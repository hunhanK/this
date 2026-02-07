/**
 * @Author: lzp
 * @Date: 2025/12/12
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math/rand"
	"time"
)

const (
	GoldenPigGiftType1 = 1 // 单个购买
	GoldenPigGiftType2 = 2 // 一次购买
)

const (
	DrawType1 = 1 // 普通抽取
	DrawType2 = 2 // 一键抽取
)

const RecordMaxNum = 50

type GoldenPigTreasureSys struct {
	Base
}

func (s *GoldenPigTreasureSys) OnLogin() {
	s.s2cInfo()
}

func (s *GoldenPigTreasureSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GoldenPigTreasureSys) OnOpen() {
	s.s2cInfo()
}

func (s *GoldenPigTreasureSys) OnNewDay() {
	data := s.GetData()
	data.Gifts = make(map[uint32]uint32)
	data.IsFetchDayRewards = false
	data.DrawCount = 0
	data.HasDrawCount = 0
	s.s2cInfo()
}

func (s *GoldenPigTreasureSys) GetData() *pb3.GoldenPigData {
	data := s.GetBinaryData().GoldenPigData
	if data == nil {
		data = &pb3.GoldenPigData{}
		s.GetBinaryData().GoldenPigData = data
	}

	if data.Gifts == nil {
		data.Gifts = make(map[uint32]uint32)
	}
	return data
}

func (s *GoldenPigTreasureSys) GetGlobal() *pb3.GoldenPigGlobalData {
	globalVar := gshare.GetStaticVar()
	if globalVar.GoldenPigGlobalData == nil {
		globalVar.GoldenPigGlobalData = &pb3.GoldenPigGlobalData{}
	}
	if globalVar.GoldenPigGlobalData.GoldenPigDrawRecords == nil {
		globalVar.GoldenPigGlobalData.GoldenPigDrawRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return globalVar.GoldenPigGlobalData
}

func (s *GoldenPigTreasureSys) CheckBoughtAllGifts() bool {
	conf := jsondata.GoldenPigTreasureConfMgr
	if conf == nil {
		return false
	}
	data := s.GetData()
	for _, gConf := range conf.Gifts {
		if gConf.GiftType == GoldenPigGiftType2 {
			continue
		}
		if data.Gifts[gConf.GiftId] < gConf.Count {
			return false
		}
	}
	return true
}

func (s *GoldenPigTreasureSys) addRecord(rd *pb3.ItemGetRecord) {
	// 全服记录
	globalData := s.GetGlobal()
	records := globalData.GoldenPigDrawRecords
	records = append(records, rd)
	if len(records) > RecordMaxNum {
		globalData.GoldenPigDrawRecords = records[len(records)-RecordMaxNum:]
	} else {
		globalData.GoldenPigDrawRecords = records
	}

	// 个人记录
	data := s.GetData()
	personalRecords := data.PersonalRecords
	personalRecords = append(personalRecords, rd)
	if len(personalRecords) > RecordMaxNum {
		data.PersonalRecords = personalRecords[len(personalRecords)-RecordMaxNum:]
	} else {
		data.PersonalRecords = personalRecords
	}
}

func (s *GoldenPigTreasureSys) s2cGoldenData() {
	s.SendProto3(83, 3, &pb3.S2C_83_3{Data: s.GetData()})
}

func (s *GoldenPigTreasureSys) s2cInfo() {
	s.SendProto3(83, 0, &pb3.S2C_83_0{
		Data:       s.GetData(),
		GlobalData: s.GetGlobal(),
	})
}

func (s *GoldenPigTreasureSys) c2sFetchDayRewards(msg *base.Message) error {
	var req pb3.C2S_83_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GoldenPigTreasureConfMgr
	if conf == nil {
		return neterror.ConfNotFoundError("GoldenPigTreasureConfMgr nil")
	}

	data := s.GetData()
	if data.IsFetchDayRewards {
		return neterror.ParamsInvalidError("already fetched")
	}

	data.IsFetchDayRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGoldenPigGiftDayReward})
	}
	s.s2cGoldenData()
	return nil
}

func (s *GoldenPigTreasureSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_83_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GoldenPigTreasureConfMgr
	if conf == nil {
		return neterror.ConfNotFoundError("GoldenPigTreasureConfMgr nil")
	}

	data := s.GetData()
	globalData := s.GetGlobal()

	retCount := data.DrawCount - data.HasDrawCount
	if retCount == 0 {
		return neterror.ParamsInvalidError("no draw count")
	}

	drawCount := uint32(1)
	consumeNum := jsondata.GetGoldenPigDrawConsumeNum(data.HasDrawCount + drawCount)
	if s.owner.GetMoneyCount(conf.ConsumeMoney) < int64(consumeNum) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	if req.GetDrawType() == DrawType2 {
		for i := drawCount + 1; i <= retCount; i++ {
			tmpNum := jsondata.GetGoldenPigDrawConsumeNum(data.HasDrawCount + i)
			consumeNum += tmpNum
			if s.owner.GetMoneyCount(conf.ConsumeMoney) < int64(consumeNum) {
				consumeNum -= tmpNum
				break
			}
			drawCount++
		}
	}

	// 消耗
	if !s.owner.DeductMoney(conf.ConsumeMoney, int64(consumeNum), common.ConsumeParams{
		LogId: pb3.LogId_LogGoldenPigGiftDrawConsume,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	if conf.AddCountRate > 0 {
		globalData.PoolNum += consumeNum * conf.AddCountRate / 100
	}

	// 奖励
	getRatio := func() uint32 {
		randPool := new(random.Pool)
		for _, rConf := range conf.RatioPool {
			randPool.AddItem(rConf, rConf.Weight)
		}
		if randPool.Size() == 0 {
			return 0
		}
		randItem := randPool.RandomOne().(*jsondata.GoldenPigTreasureRatio)
		return randItem.Rate
	}

	player := s.owner
	getMoneyNum, lastGetMoneyNum := uint32(0), uint32(0)

	for i := uint32(1); i <= drawCount; i++ {
		ratio := getRatio()
		if ratio == 0 {
			continue
		}
		diamondNum := ratio * jsondata.GetGoldenPigDrawConsumeNum(data.HasDrawCount+i)

		if diamondNum > 0 {
			s.addRecord(&pb3.ItemGetRecord{
				ActorId:   player.GetId(),
				ActorName: player.GetName(),
				ItemId:    itemdef.ItemDiamond,
				Count:     diamondNum,
				TimeStamp: time_util.NowSec(),
			})
			if i == drawCount {
				lastGetMoneyNum = diamondNum
			}
			getMoneyNum += diamondNum
		}
	}

	data.HasDrawCount += drawCount
	if getMoneyNum > 0 {
		player.AddMoney(conf.RewardMoney, int64(getMoneyNum), true, pb3.LogId_LogGoldenPigGiftDrawAwards)
		if !utils.SliceContainsUint64(globalData.PlayerIds, player.GetId()) {
			globalData.PlayerIds = append(globalData.PlayerIds, player.GetId())
		}
	}

	s.SendProto3(83, 2, &pb3.S2C_83_2{
		DrawType:     req.DrawType,
		MoneyNum:     getMoneyNum,
		LastMoneyNum: lastGetMoneyNum,
	})
	s.s2cInfo()
	return nil
}

func goldenPigGiftCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiGoldenPigTreasure).(*GoldenPigTreasureSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	gConf := jsondata.GetGoldenPigGiftConf(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	if gConf.GiftType == GoldenPigGiftType1 {
		if data.Gifts[gConf.GiftId] >= gConf.Count {
			return false
		}
	}
	if gConf.GiftType == GoldenPigGiftType2 {
		for _, count := range data.Gifts {
			if count > 0 {
				return false
			}
		}
	}
	return true
}

func goldenPigGiftChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiGoldenPigTreasure).(*GoldenPigTreasureSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	gConf := jsondata.GetGoldenPigGiftConf(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	var rewards jsondata.StdRewardVec
	if gConf.GiftType == GoldenPigGiftType1 {
		data.Gifts[gConf.GiftId] += 1
		rewards = gConf.Rewards
	}

	if gConf.GiftType == GoldenPigGiftType2 {
		for _, vConf := range jsondata.GoldenPigTreasureConfMgr.Gifts {
			data.Gifts[vConf.GiftId] += vConf.Count
			rewardsMulti := jsondata.StdRewardMulti(vConf.Rewards, int64(vConf.Count))
			rewards = append(rewards, rewardsMulti...)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGoldenPigGiftBuyPayReward})
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogGoldenPigGiftBuyPayReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(gConf.GiftId),
	})
	player.SendShowRewardsPop(rewards)
	engine.BroadcastTipMsgById(gConf.BroadcastId, player.GetId(), player.GetName(), gConf.GiftName, engine.StdRewardToBroadcast(player, rewards))

	sys.s2cGoldenData()
	return true
}

func useGoldenPigDrawItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiGoldenPigTreasure).(*GoldenPigTreasureSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	data.DrawCount += uint32(param.Count)
	sys.s2cGoldenData()
	return true, true, param.Count
}

func handleGoldenPigTreasureSettle() {
	conf := jsondata.GoldenPigTreasureConfMgr

	globalVar := gshare.GetStaticVar()
	goldenGlobal := globalVar.GoldenPigGlobalData
	if goldenGlobal == nil {
		return
	}

	playerIds := append([]uint64(nil), goldenGlobal.PlayerIds...)
	if len(playerIds) == 0 {
		return
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(playerIds), func(i, j int) {
		playerIds[i], playerIds[j] = playerIds[j], playerIds[i]
	})

	allocatedNum := uint32(0)
	allocatedRatio := uint32(0)

	cursor := 0
	for _, rConf := range conf.RewardsPool {
		if cursor >= len(playerIds) {
			break
		}
		num := rConf.PlayerNum
		remain := len(playerIds) - cursor
		num = utils.MinUInt32(num, uint32(remain))

		allocatedRatio += rConf.Ratio * num

		for i := uint32(0); i < num; i++ {
			playerId := playerIds[cursor]

			if rConf.Ratio == 0 {
				continue
			}
			moneyCount := goldenGlobal.PoolNum * rConf.Ratio / 100
			allocatedNum += moneyCount
			mailmgr.SendMailToActor(playerId, &mailargs.SendMailSt{
				ConfId: uint16(conf.MailId),
				Content: &mailargs.CommonMailArgs{
					Str1:   rConf.RewardName,
					Digit1: int64(moneyCount),
				},
				Rewards: jsondata.StdRewardVec{
					{Id: itemdef.ItemDiamond, Count: int64(moneyCount)},
				},
			})
			cursor++
		}
	}

	// 剩余玩家分配剩余池子
	playerIds = playerIds[cursor:]
	if len(playerIds) > 0 {
		remainRatio := uint32(100) - allocatedRatio
		if remainRatio > 0 {
			remainNum := goldenGlobal.PoolNum * remainRatio / 100
			moneyCount := remainNum / uint32(len(playerIds))
			moneyCount = utils.MinUInt32(moneyCount, goldenGlobal.PoolNum*conf.RewardsPoolRatio/100)
			if moneyCount > 0 {
				for _, playerId := range playerIds {
					mailmgr.SendMailToActor(playerId, &mailargs.SendMailSt{
						ConfId: uint16(conf.MailId),
						Content: &mailargs.CommonMailArgs{
							Str1:   conf.RewardName,
							Digit1: int64(moneyCount),
						},
						Rewards: jsondata.StdRewardVec{
							{Id: itemdef.ItemDiamond, Count: int64(moneyCount)},
						},
					})
				}
			}
		}
	}
}

func initPool() {
	conf := jsondata.GoldenPigTreasureConfMgr
	if conf == nil {
		return
	}
	globalVar := gshare.GetStaticVar()
	if globalVar.GoldenPigGlobalData == nil {
		globalVar.GoldenPigGlobalData = &pb3.GoldenPigGlobalData{}
		globalData := globalVar.GoldenPigGlobalData
		globalData.PoolNum = conf.InitNum
	}
}

func resetPool() {
	conf := jsondata.GoldenPigTreasureConfMgr
	if conf == nil {
		return
	}
	globalVar := gshare.GetStaticVar()
	if globalVar.GoldenPigGlobalData != nil {
		globalData := globalVar.GoldenPigGlobalData
		globalData.PoolNum = conf.InitNum
		globalData.PlayerIds = globalData.PlayerIds[:0]
	}
}

func init() {
	RegisterSysClass(sysdef.SiGoldenPigTreasure, func() iface.ISystem {
		return &GoldenPigTreasureSys{}
	})

	net.RegisterSysProtoV2(83, 1, sysdef.SiGoldenPigTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GoldenPigTreasureSys).c2sFetchDayRewards
	})
	net.RegisterSysProtoV2(83, 2, sysdef.SiGoldenPigTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GoldenPigTreasureSys).c2sDraw
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiGoldenPigTreasure).(*GoldenPigTreasureSys); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		initPool()
	})
	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		handleGoldenPigTreasureSettle()
		resetPool()
	})

	engine.RegChargeEvent(chargedef.GoldenPigGift, goldenPigGiftCheck, goldenPigGiftChargeBack)
	miscitem.RegCommonUseItemHandle(itemdef.UseGoldenPigDrawItem, useGoldenPigDrawItem)
}
