/**
 * @Author: lzp
 * @Date: 2024/4/26
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type TreasurePavSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

const (
	TreasurePavRecordGType = 1
	TreasurePavRecordPType = 2

	TreasurePavRecordGCount = 50
	TreasurePavRecordPCount = 50
)

var ErrorConfNotFound = neterror.ConfNotFoundError("treasurepav draw conf is nil")

func (s *TreasurePavSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}

	// 初始化累抽
	aData := s.GetTreasureAccInfo()
	if aData.AccRewardRound == 0 {
		curRound := jsondata.GetYYTreasureAccRewardFirstRound()
		aData.AccRewardRound = curRound
		aData.AccAwards[curRound] = &pb3.TreasureAccReward{
			Round: curRound,
			Ids:   make([]uint32, 0),
		}
	}
}

func (s *TreasurePavSys) Login() {
	s.S2CInfo()
}

func (s *TreasurePavSys) OnReconnect() {
	s.S2CInfo()
}

func (s *TreasurePavSys) OnOpen() {
	s.S2CInfo()
}

func (s *TreasurePavSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func (s *TreasurePavSys) ResetData() {
	state := s.GetYYData()
	if state.TreasurePavDraw == nil {
		return
	}
	delete(state.TreasurePavDraw, s.Id)
}

func (s *TreasurePavSys) S2CInfo() {
	data := s.GetData()
	aData := s.GetTreasureAccInfo()
	s.SendProto3(63, 0, &pb3.S2C_63_0{
		ActiveId:  s.Id,
		DrawTimes: aData.DrawTimes,
		AccRound:  aData.AccRewardRound,
		SumAward:  aData.AccAwards,
		Record:    data.TreasureDraw.Record,
	})
}

func (s *TreasurePavSys) GetData() *pb3.PYY_TreasurePavDraw {
	state := s.GetYYData()
	if state.TreasurePavDraw == nil {
		state.TreasurePavDraw = make(map[uint32]*pb3.PYY_TreasurePavDraw)
	}
	if state.TreasurePavDraw[s.Id] == nil {
		state.TreasurePavDraw[s.Id] = &pb3.PYY_TreasurePavDraw{}
	}
	sData := state.TreasurePavDraw[s.Id]
	if sData.LotteryData == nil {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if sData.TreasureDraw == nil {
		sData.TreasureDraw = &pb3.TreasureDraw{}
	}
	return sData
}

func (s *TreasurePavSys) GetTreasureAccInfo() *pb3.TreasureAccDrawInfo {
	binary := s.player.GetBinaryData()
	if binary.TreasureAccDrawInfo == nil {
		binary.TreasureAccDrawInfo = &pb3.TreasureAccDrawInfo{
			AccRewardRound: 0,
			AccAwards:      make(map[uint32]*pb3.TreasureAccReward),
		}
	}
	return binary.TreasureAccDrawInfo
}

func (s *TreasurePavSys) GetGlobalRecord() *pb3.TreasureDrawRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.TreasureDrawRecord == nil {
		globalVar.TreasureDrawRecord = make(map[uint64]*pb3.TreasureDrawRecord)
	}
	idx := utils.Make64(s.OpenTime, s.Id)
	if globalVar.TreasureDrawRecord[idx] == nil {
		globalVar.TreasureDrawRecord[idx] = &pb3.TreasureDrawRecord{}
	}
	return globalVar.TreasureDrawRecord[idx]
}

func (s *TreasurePavSys) GetLuckTimes() uint16 {
	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	return conf.LuckTimes
}

func (s *TreasurePavSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	return conf.LuckyValEx
}

func (s *TreasurePavSys) RawData() *pb3.LotteryData {
	data := s.GetData()
	return data.LotteryData
}

func (s *TreasurePavSys) GetSingleDiamondPrice() uint32 {
	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	itemId := conf.DrawConsume[0].Id
	singlePrice := jsondata.GetAutoBuyItemPrice(itemId, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(itemId, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *TreasurePavSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	tData := s.GetData().TreasureDraw
	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	rd := &pb3.ItemGetRecord{
		ActorId:   s.player.GetId(),
		ActorName: s.player.GetName(),
		ItemId:    rewards[0].Id,
		Count:     uint32(rewards[0].Count),
		TimeStamp: time_util.NowSec(),
	}

	// 个人记录
	s.DrawRecord(&tData.Record, rd, TreasurePavRecordPCount)

	// 全服记录
	gRecord := s.GetGlobalRecord()
	if utils.SliceContainsUint32(conf.RecordSuperLibs, libId) {
		s.DrawRecord(&gRecord.Records, rd, TreasurePavRecordGCount)
	}
}

func (s *TreasurePavSys) DrawRecord(rds *[]*pb3.ItemGetRecord, record *pb3.ItemGetRecord, recordLimit int) {
	*rds = append(*rds, record)
	if len(*rds) > recordLimit {
		*rds = (*rds)[1:]
	}
}

func (s *TreasurePavSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_63_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("treasurepav draw conf is nil")
	}

	cosConf, ok := conf.Cos[req.Times]
	if !ok {
		return neterror.ConfNotFoundError("treasurepav draw cos times not exist")
	}
	if len(conf.LibIds) <= 0 {
		return neterror.InternalError("treasurepav draw no libs")
	}

	consumes := jsondata.ConsumeMulti(conf.DrawConsume, cosConf.Count)
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogTreasurePavDraw})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 记录累抽次数
	aData := s.GetTreasureAccInfo()
	aData.DrawTimes += req.Times

	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()

	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	if len(result.Awards) > 0 {
		if !req.IsSkip {
			dur := time.Second * time.Duration(int64(conf.TurnTime))
			s.GetPlayer().SetTimeout(dur, func() {
				engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
					LogId:        pb3.LogId_LogTreasurePavDraw,
					BroadcastExt: []interface{}{s.Id},
				})
			})
		} else {
			engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
				LogId:        pb3.LogId_LogTreasurePavDraw,
				BroadcastExt: []interface{}{s.Id},
			})
		}
	}

	// 打包奖励
	var ids []*pb3.KeyValue
	for _, v := range result.LibResult {
		ids = append(ids, &pb3.KeyValue{
			Key:   v.LibId,
			Value: v.AwardPoolConf.Id,
		})
	}

	s.SendProto3(63, 1, &pb3.S2C_63_1{
		ActiveId:  s.Id,
		Times:     req.Times,
		DrawTimes: aData.DrawTimes,
		Items:     ids,
	})
	return nil
}

func (s *TreasurePavSys) c2sFetchAccReward(msg *base.Message) error {
	var req pb3.C2S_63_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	aData := s.GetTreasureAccInfo()
	accConf := jsondata.GetYYTreasureAccRewardConf(aData.AccRewardRound)
	if accConf == nil {
		return neterror.ConfNotFoundError("treasurepav accdrawconfig is nil")
	}

	rConf, ok := accConf.AccRewards[req.Id]
	if !ok {
		return neterror.ConfNotFoundError("treasurepav accdrawconfig(%d, %d) not exist", aData.AccRewardRound, req.Id)
	}

	if rConf.DrawTimes > aData.DrawTimes {
		return neterror.ConfNotFoundError("treasurepav accdrawconfig fetch limit")
	}

	fetchReward, ok := aData.AccAwards[aData.AccRewardRound]
	if ok && utils.SliceContainsUint32(fetchReward.Ids, req.Id) {
		return neterror.ParamsInvalidError("treasurepav accdrawconfig is received")
	}

	rewards := rConf.Rewards
	bagSys := s.player.GetSysObj(sysdef.SiBag).(*actorsystem.BagSystem)
	canAdd := engine.CheckRewards(s.GetPlayer(), rewards)
	if !canAdd && bagSys.AvailableCount() <= 10 {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	fetchReward.Ids = append(fetchReward.Ids, req.Id)
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTreasurePavAccReward})
	}

	if s.checkAllReceived() {
		// 领取完, 下一轮
		s.newAccRewardRound()
	}
	s.SendProto3(63, 2, &pb3.S2C_63_2{
		ActiveId:  s.Id,
		Id:        req.Id,
		AccRound:  aData.AccRewardRound,
		DrawTimes: aData.DrawTimes,
	})
	return nil
}

func (s *TreasurePavSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_63_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYTreasureDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return ErrorConfNotFound
	}

	rsp := &pb3.S2C_63_3{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case TreasurePavRecordGType:
		rsp.Record = s.GetGlobalRecord()
	case TreasurePavRecordPType:
		rsp.Record = &pb3.TreasureDrawRecord{Records: s.GetData().TreasureDraw.Record}
	}

	s.SendProto3(63, 3, rsp)
	return nil
}

// 检查该轮次累抽奖励是否全部领取完
func (s *TreasurePavSys) checkAllReceived() bool {
	aData := s.GetTreasureAccInfo()
	curRound := aData.AccRewardRound
	accReward, ok := aData.AccAwards[curRound]
	if !ok || len(accReward.Ids) == 0 {
		return false
	}
	aConf := jsondata.GetYYTreasureAccRewardConf(curRound)
	for id := range aConf.AccRewards {
		if !utils.SliceContainsUint32(accReward.Ids, id) {
			return false
		}
	}
	return true
}

func (s *TreasurePavSys) newAccRewardRound() {
	aData := s.GetTreasureAccInfo()
	curRound := aData.AccRewardRound
	aConf := jsondata.GetYYTreasureAccRewardConf(curRound)
	if aConf == nil {
		return
	}

	nextRound := aConf.NextRound
	if jsondata.GetYYTreasureAccRewardConf(nextRound) == nil {
		return
	}

	aData.AccRewardRound = nextRound
	aData.AccAwards[nextRound] = &pb3.TreasureAccReward{
		Ids: []uint32{},
	}
	sumTimes := aConf.AccRewards[uint32(len(aConf.AccRewards))].DrawTimes
	aData.DrawTimes = utils.MaxUInt32(0, aData.DrawTimes-sumTimes)
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTreasurePav, func() iface.IPlayerYY {
		return &TreasurePavSys{}
	})

	net.RegisterYYSysProtoV2(63, 1, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*TreasurePavSys).c2sDraw
	})

	net.RegisterYYSysProtoV2(63, 2, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*TreasurePavSys).c2sFetchAccReward
	})

	net.RegisterYYSysProtoV2(63, 3, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TreasurePavSys).c2sRecord
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.TreasurePavDrawTip, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, id}, true
	})

	gmevent.Register("tDraw", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		objs := sys.GetAllObj(yydefine.YYTreasurePav)
		if len(objs) == 0 {
			return false
		}
		if objs[0].GetClass() != yydefine.YYTreasurePav {
			return false
		}

		obj := objs[0]
		msg := base.NewMessage()
		msg.SetCmd(63<<8 | 1)
		err := msg.PackPb3Msg(&pb3.C2S_63_1{
			Base: &pb3.YYBase{
				ActiveId: 777001,
			},
			Times: utils.AtoUint32(args[0]),
		})
		if err != nil {
			return false
		}

		s := obj.(*TreasurePavSys)
		err = s.c2sDraw(msg)
		if err != nil {
			s.LogError("gm c2sDraw err:%v", err)
			return false
		}

		return true
	}, 1)

	gmevent.Register("tRevAcc", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		objs := sys.GetAllObj(yydefine.YYTreasurePav)
		if len(objs) == 0 {
			return false
		}

		obj := objs[0]
		if obj.GetClass() != yydefine.YYTreasurePav {
			return false
		}

		s := obj.(*TreasurePavSys)
		id := utils.AtoUint32(args[0])

		sendFunc := func(id uint32) error {
			msg := base.NewMessage()
			msg.SetCmd(63<<8 | 2)
			err := msg.PackPb3Msg(&pb3.C2S_63_2{
				Base: &pb3.YYBase{
					ActiveId: 777001,
				},
				Id: id,
			})
			if err != nil {
				return err
			}

			return s.c2sFetchAccReward(msg)
		}

		if id == 0 {
			accRound := s.GetTreasureAccInfo().AccRewardRound
			aConf := jsondata.GetYYTreasureAccRewardConf(accRound)
			if aConf == nil {
				return false
			}
			for _, aRewardConf := range aConf.AccRewards {
				if sendFunc(aRewardConf.Id) != nil {
					return false
				}
			}
			return true
		}

		if sendFunc(id) != nil {
			s.LogError("gm c2sFetchAccReward error")
			return false
		}

		return true
	}, 1)

	gmevent.Register("tSetAccTimes", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		objs := sys.GetAllObj(yydefine.YYTreasurePav)
		if len(objs) == 0 {
			return false
		}
		if objs[0].GetClass() != yydefine.YYTreasurePav {
			return false
		}

		s := objs[0].(*TreasurePavSys)
		s.GetTreasureAccInfo().DrawTimes = utils.AtoUint32(args[0])
		s.S2CInfo()
		return true
	}, 1)
}
