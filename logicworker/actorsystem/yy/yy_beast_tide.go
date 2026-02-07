/**
 * @Author:
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-兽潮来袭
**/

package yy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

const (
	BTEffectTypeBossRefresh      = 1 // boss刷新
	BTEffectTypeItemAwards       = 2 // 全服奖励
	BTEffectTypeDoubleExperience = 3 // 双倍经验
	BTEffectTypeDoubleMaterial   = 4 // 双倍材料
)

const (
	OneCount = 1
	TenCount = 10
)

const (
	EffectStatus0 = 0 // 未生效
	EffectStatus1 = 1 // 正生效
	EffectStatus2 = 2 // 已生效
)

const HourSec = 60 * 60

type YYBeastTide struct {
	YYBase
}

func (s *YYBeastTide) OnInit() {
	s.addMonExtraDrops()
	s.sendAllPlayers()
}

func (s *YYBeastTide) NewDay() {
	data := s.GetData()
	data.Value = 0
	data.Effects = make(map[uint32]*pb3.YYBeastTideEffect)
	s.sendAllPlayers()
}

func (s *YYBeastTide) OnEnd() {
	s.delMonExtraDrops()

	// 发送玩家未领取的个人捐献奖励
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	data := s.GetData()

	getUnFetchRewards := func(playerId uint64, value uint32) jsondata.StdRewardVec {
		var rewards jsondata.StdRewardVec
		for id, rConf := range conf.PDonate {
			rewardsBit := data.PlayerRewards[playerId]
			if utils.IsSetBit(rewardsBit, id) {
				continue
			}
			if value >= rConf.Count {
				rewards = append(rewards, rConf.Rewards...)
			}
		}
		return rewards
	}

	for k, v := range data.PlayerMap {
		rewards := getUnFetchRewards(k, v)
		if len(rewards) > 0 {
			mailmgr.SendMailToActor(k, &mailargs.SendMailSt{
				ConfId:  common.Mail_YYBeastTidePersonUnFetchRewards,
				Rewards: rewards,
			})
		}
	}
}

func (s *YYBeastTide) PlayerLogin(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *YYBeastTide) PlayerReconnect(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *YYBeastTide) GetData() *pb3.YYBeastTide {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.BeastTideData == nil {
		globalVar.YyDatas.BeastTideData = make(map[uint32]*pb3.YYBeastTide)
	}
	if globalVar.YyDatas.BeastTideData[s.Id] == nil {
		globalVar.YyDatas.BeastTideData[s.Id] = &pb3.YYBeastTide{}
	}

	bTData := globalVar.YyDatas.BeastTideData[s.Id]
	if bTData.Effects == nil {
		bTData.Effects = make(map[uint32]*pb3.YYBeastTideEffect)
	}
	if bTData.PlayerMap == nil {
		bTData.PlayerMap = make(map[uint64]uint32)
	}
	if bTData.PlayerRewards == nil {
		bTData.PlayerRewards = make(map[uint64]uint32)
	}

	return bTData
}
func (s *YYBeastTide) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.BeastTideData == nil {
		return
	}
	delete(globalVar.YyDatas.BeastTideData, s.Id)
}

// 获得全服效果加成
func (s *YYBeastTide) GetBeastTideAddition(effType uint32) int64 {
	effData := s.getBeastTideByEffType(effType)
	if effData == nil {
		return 0
	}
	return int64(effData.Ext)
}

func (s *YYBeastTide) AddDonate(player iface.IPlayer, num uint32) {
	data := s.GetData()
	preVal := data.Value

	data.Value += num
	data.PlayerMap[player.GetId()] += num

	s.CheckUpServerValue(preVal, data.Value)
}

func (s *YYBeastTide) AutoDonateBeastTide() {
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	hour := time_util.Now().Hour()
	hConf, ok := conf.DonateGrow[uint32(hour)]
	if !ok || len(hConf.DonateValues) < 2 {
		return
	}
	min := hConf.DonateValues[0]
	max := hConf.DonateValues[1]
	count := random.IntervalUU(min, max)

	data := s.GetData()
	preVal := data.Value
	data.Value += count

	s.CheckUpServerValue(preVal, data.Value)
}

func (s *YYBeastTide) CheckUpServerValue(preVal, newVal uint32) {
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	for _, v := range conf.SDonate {
		// 保证执行一次
		if preVal < v.Count && newVal >= v.Count {
			s.DoSeverEffect(v)
		}
	}
}

func (s *YYBeastTide) DoSeverEffect(conf *jsondata.BeastTideDonate) {
	rConf := conf.ServerReward
	if rConf == nil {
		return
	}

	// 之前的同类型buff直接置为已生效
	effData := s.getBeastTideByEffType(rConf.RType)
	if effData != nil {
		effData.Status = EffectStatus2
	}

	data := s.GetData()
	if _, ok := data.Effects[conf.Id]; !ok {
		data.Effects[conf.Id] = &pb3.YYBeastTideEffect{Id: conf.Id, EffectType: rConf.RType}
	}
	effect, _ := data.Effects[conf.Id]

	switch rConf.RType {
	case BTEffectTypeBossRefresh:
		if effect.Status == EffectStatus2 {
			return
		}
		effect.Status = EffectStatus2
		s.callFightSrv()
		engine.BroadcastTipMsgById(tipmsgid.YYBeastTideActiveBuffTip, rConf.Name)
	case BTEffectTypeItemAwards:
		if effect.Status == EffectStatus2 {
			return
		}
		effect.Status = EffectStatus2
		mailmgr.AddSrvMailStr(common.Mail_YYBeastTideServerAwards, "", rConf.Rewards)
		engine.BroadcastTipMsgById(tipmsgid.YYBeastTideServerRewardsTip)
	case BTEffectTypeDoubleExperience:
		now := time_util.NowSec()
		if now >= effect.StartTime && now < effect.EndTime {
			return
		}

		if len(rConf.Params) < 2 {
			return
		}

		ratio, hour := rConf.Params[0], rConf.Params[1]
		effect.EndTime = time_util.NowSec() + hour*HourSec
		effect.StartTime = time_util.NowSec()
		effect.Status = EffectStatus1
		effect.Ext = ratio
		engine.BroadcastTipMsgById(tipmsgid.YYBeastTideActiveBuffTip, rConf.Name)
	case BTEffectTypeDoubleMaterial:
		now := time_util.NowSec()
		if now >= effect.StartTime && now < effect.EndTime {
			return
		}

		if len(rConf.Params) < 2 {
			return
		}

		ratio, hour := rConf.Params[0], rConf.Params[1]
		effect.EndTime = time_util.NowSec() + hour*HourSec
		effect.StartTime = time_util.NowSec()
		effect.Status = EffectStatus1
		effect.Ext = ratio
		engine.BroadcastTipMsgById(tipmsgid.YYBeastTideActiveBuffTip, rConf.Name)
	}

	// 全服推送
	s.sendAllPlayers()
}

func (s *YYBeastTide) getBeastTideByEffType(effType uint32) *pb3.YYBeastTideEffect {
	data := s.GetData()
	now := time_util.NowSec()
	for _, effect := range data.Effects {
		if effect.EffectType != effType {
			continue
		}
		if effect.Status != EffectStatus1 {
			continue
		}
		if now >= effect.StartTime && now < effect.EndTime {
			return effect
		}
	}
	return nil
}

func (s *YYBeastTide) addMonExtraDrops() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FYYActAddMonExtraDrops, &pb3.G2FYYActAddMonExtraDrops{
		ActivityId: s.GetId(),
		Data:       s.packMonDropData(),
	})
	if err != nil {
		s.LogError("err: %v", err)
	}
}

func (s *YYBeastTide) delMonExtraDrops() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FYYActDelMonExtraDrops, &pb3.G2FYYActDelMonExtraDrops{
		ActivityId: s.GetId(),
	})
	if err != nil {
		s.LogError("err: %v", err)
	}
}

func (s *YYBeastTide) packMonDropData() map[uint64]*pb3.YYActMonExtraDrops {
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	pkgMsg := make(map[uint64]*pb3.YYActMonExtraDrops)
	for _, mon := range conf.Monsters {
		pkgMsg[mon.MonId] = &pb3.YYActMonExtraDrops{DropIds: mon.Drops}
	}

	return pkgMsg
}

func (s *YYBeastTide) sendPlayerData(player iface.IPlayer) {
	if player == nil {
		return
	}
	player.SendProto3(69, 130, &pb3.S2C_69_130{
		ActiveId: s.GetId(),
		Data:     s.packPlayerData(player),
	})
}

func (s *YYBeastTide) packPlayerData(player iface.IPlayer) *pb3.YYBeastTidePlayerInfo {
	data := s.GetData()
	return &pb3.YYBeastTidePlayerInfo{
		TotalVal:      data.GetValue(),
		PerVal:        data.PlayerMap[player.GetId()],
		PerGetRewards: data.PlayerRewards[player.GetId()],
		Effects:       data.Effects,
	}
}

func (s *YYBeastTide) sendAllPlayers() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.sendPlayerData(player)
	})
}

func (s *YYBeastTide) callFightSrv() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FRelieveSceneMonsters, nil)
	if err != nil {
		s.LogError("err: %v", err)
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FRelieveSceneMonsters, nil)
	if err != nil {
		s.LogError("err: %v", err)
	}
}

func (s *YYBeastTide) c2sDonate(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_131
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("conf not found")
	}

	consumes := conf.OnceConsume
	addCount := OneCount
	if req.Ref == 2 {
		consumes = conf.TenConsume
		addCount = TenCount
	}
	if !player.ConsumeByConf(consumes, false, common.ConsumeParams{
		LogId: pb3.LogId_LogYYBeastTideDonateConsume,
	}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.AddDonate(player, uint32(addCount))
	player.SendProto3(69, 131, &pb3.S2C_69_131{
		ActId: s.GetId(),
		Data:  s.packPlayerData(player),
	})

	return nil
}

func (s *YYBeastTide) c2sFetchRewards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_132
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYBeastTideConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("conf not found")
	}

	pConf, ok := conf.PDonate[req.Idx]
	if !ok {
		return neterror.ParamsInvalidError("conf pDonate(%d) not found", req.Idx)
	}

	data := s.GetData()
	playerId := player.GetId()
	if data.PlayerMap[playerId] < pConf.Count {
		return neterror.ParamsInvalidError("conf pDonate(%d) donation limit", req.Idx)
	}

	rewardsBit := data.PlayerRewards[playerId]
	if utils.IsSetBit(rewardsBit, req.Idx) {
		return neterror.ParamsInvalidError("conf pDonate(%d) rewards has fetch", req.Idx)
	}

	// 设置标志位
	rewardsBit = utils.SetBit(rewardsBit, req.Idx)
	data.PlayerRewards[playerId] = rewardsBit

	// 发送奖励
	engine.GiveRewards(player, pConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogYYBeastTidePlayerDonateAwards,
	})

	player.SendProto3(69, 132, &pb3.S2C_69_132{ActId: s.Id, Idx: req.Idx})
	return nil
}

func init() {
	yymgr.RegisterYYType(yydefine.YYBeastTide, func() iface.IYunYing {
		return &YYBeastTide{}
	})

	net.RegisterGlobalYYSysProto(69, 131, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYBeastTide).c2sDonate
	})

	net.RegisterGlobalYYSysProto(69, 132, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYBeastTide).c2sFetchRewards
	})

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		yyList := yymgr.GetAllYY(yydefine.YYBeastTide)
		for _, yy := range yyList {
			if yy.IsOpen() {
				yy.(*YYBeastTide).addMonExtraDrops()
			} else {
				yy.(*YYBeastTide).delMonExtraDrops()
			}
		}
	})

	gmevent.Register("bt.c2sDonate", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		ref := utils.AtoUint32(args[0])

		allYY := yymgr.GetAllYY(yydefine.YYBeastTide)
		for _, iYY := range allYY {
			if sys, ok := iYY.(*YYBeastTide); ok && sys.IsOpen() {
				msg := base.NewMessage()
				msg.SetCmd(69<<8 | 131)
				err := msg.PackPb3Msg(&pb3.C2S_69_131{
					Base: &pb3.YYBase{ActiveId: sys.Id},
					Ref:  ref,
				})
				if err != nil {
					return false
				}
				err = sys.c2sDonate(player, msg)
				if err != nil {
					return false
				}
				break
			}
		}
		return true
	}, 1)

	gmevent.Register("bt.add", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		num := utils.AtoUint32(args[0])

		allYY := yymgr.GetAllYY(yydefine.YYBeastTide)
		for _, iYY := range allYY {
			if sys, ok := iYY.(*YYBeastTide); ok && sys.IsOpen() {
				sys.AddDonate(player, num)
				sys.sendPlayerData(player)
				break
			}
		}
		return true
	}, 1)

	gmevent.Register("bt.clear", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYBeastTide)
		for _, iYY := range allYY {
			if sys, ok := iYY.(*YYBeastTide); ok && sys.IsOpen() {
				data := sys.GetData()
				data.Value = 0
				data.Effects = make(map[uint32]*pb3.YYBeastTideEffect)
				data.PlayerMap = make(map[uint64]uint32)
				data.PlayerRewards = make(map[uint64]uint32)
				sys.sendPlayerData(player)
				break
			}
		}
		return true
	}, 1)
}
