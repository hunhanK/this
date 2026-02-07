/**
 * @Author: LvYuMeng
 * @Date: 2024/1/29
 * @Desc:

**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type DragonDrawSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *DragonDrawSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *DragonDrawSys) GetLuckTimes() uint16 {
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	return conf.LuckTimes
}

func (s *DragonDrawSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	return conf.LuckyValEx
}

func (s *DragonDrawSys) RawData() *pb3.LotteryData {
	data := s.GetData()
	return data.LotteryData
}

func (s *DragonDrawSys) GetSingleDiamondPrice() uint32 {
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	itemId := conf.DrawConsume[0].Id
	singlePrice := jsondata.GetAutoBuyItemPrice(itemId, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(itemId, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *DragonDrawSys) GetData() *pb3.PYY_EvolutionDragonDraw {
	state := s.GetYYData()
	if nil == state.EvolutionDragonDraw {
		state.EvolutionDragonDraw = make(map[uint32]*pb3.PYY_EvolutionDragonDraw)
	}
	if state.EvolutionDragonDraw[s.Id] == nil {
		state.EvolutionDragonDraw[s.Id] = &pb3.PYY_EvolutionDragonDraw{}
	}
	sData := state.EvolutionDragonDraw[s.Id]
	//抽奖数据
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	//龙装部分数据
	if nil == sData.DragonDraw {
		sData.DragonDraw = &pb3.DragonDraw{}
	}
	s.InitData(sData.DragonDraw)
	return sData
}

func (s *DragonDrawSys) InitData(data *pb3.DragonDraw) {
	if nil == data.ExchangeRemind {
		data.ExchangeRemind = make(map[uint32]bool)
	}
	if nil == data.Exchange {
		data.Exchange = make(map[uint32]uint32)
	}
	if nil == data.SumAward {
		data.SumAward = make(map[uint32]bool)
	}
}

func (s *DragonDrawSys) s2cInfo() {
	data := s.GetData()
	s.SendProto3(56, 0, &pb3.S2C_56_0{
		ActiveId: s.Id,
		Info: &pb3.PYY_DragonDraw{
			DrawTimes:      data.LotteryData.Times,
			Lucky:          data.LotteryData.PlayerLucky,
			ExchangeRemind: data.DragonDraw.ExchangeRemind,
			Exchange:       data.DragonDraw.Exchange,
			SumAward:       data.DragonDraw.SumAward,
			Record:         data.DragonDraw.Record,
			LastResetKey:   data.DragonDraw.LastResetKey,
			HighPoolId:     data.DragonDraw.HighPoolId,
		},
	})
}

func (s *DragonDrawSys) Login() {
	s.checkDefault()
	s.s2cInfo()
}

func (s *DragonDrawSys) checkDefault() {
	yyList := pyymgr.GetPlayerAllYYObj(s.GetPlayer(), yydefine.YYAffordableGift)
	for _, obj := range yyList {
		if sys, ok := obj.(*AffordableGiftSys); ok && sys.IsOpen() {
			return
		}
	}
	s.SetHighPoolId(0, false)
}

func (s *DragonDrawSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionDragonDraw {
		state.EvolutionDragonDraw = make(map[uint32]*pb3.PYY_EvolutionDragonDraw)
	}
	delete(s.GetYYData().EvolutionDragonDraw, s.GetId())
	s.SetHighPoolId(0, false)
}

func (s *DragonDrawSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DragonDrawSys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *DragonDrawSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_56_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("dragon draw conf is nil")
	}

	cosConf, ok := conf.Cos[req.Times]
	if !ok {
		return neterror.ConfNotFoundError("dragon draw cos times not exist")
	}

	data := s.GetData()
	libIds := conf.Pool[data.DragonDraw.HighPoolId].LibIds
	if len(libIds) <= 0 {
		return neterror.InternalError("dragon draw no libs")
	}

	consumes := jsondata.ConsumeMulti(conf.DrawConsume, cosConf.Count)
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogDragonDraw})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	//每抽赠送积分
	bundledAwards := jsondata.StdRewardMulti(conf.DrawScore, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDragonDraw})
	}

	if len(conf.ReachScore) >= 2 {
		s.GetPlayer().TriggerEvent(custom_id.AeGetDrawReachScore, conf.ReachScore[0], conf.ReachScore[1]*req.Times)
	}

	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()
	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, libIds)
	if len(result.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
			LogId:        pb3.LogId_LogDragonDraw,
			NoTips:       true,
			BroadcastExt: []interface{}{s.Id},
		})
	}

	var ids []*pb3.DragonItem
	for _, v := range result.LibResult {
		rewards := engine.FilterRewardByPlayer(s.GetPlayer(), v.AwardPoolConf.Awards)
		st := &pb3.DragonItem{
			ItemId: rewards[0].Id,
			Count:  uint32(rewards[0].Count),
		}
		if utils.SliceContainsUint32(conf.SuperLibs, v.LibId) {
			st.PoolType = dragonDrawHighPoolType
		}
		ids = append(ids, st)
	}
	s.GetPlayer().TriggerQuestEvent(custom_id.QttDragonDrawTime, 0, int64(req.Times))
	s.GetPlayer().TriggerQuestEvent(custom_id.QttAchievementsDragonDrawTime, 0, int64(req.Times))
	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActDrawTypeDragon,
		ActId:   s.Id,
		Times:   req.Times,
	})

	s.SendProto3(56, 1, &pb3.S2C_56_1{
		ActiveId:  s.Id,
		Times:     req.Times,
		Items:     ids,
		Lucky:     data.LotteryData.PlayerLucky,
		DrawTimes: data.LotteryData.Times,
	})

	return nil
}

func (s *DragonDrawSys) record(records *[]*pb3.ItemGetRecord, record *pb3.ItemGetRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *DragonDrawSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.GetData()

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("dragon draw conf is nil")
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		s.LogError("dragon draw pool(%d) nothing can record", awardPoolConf.Id)
		return
	}

	nowSec := time_util.NowSec()
	record := &pb3.ItemGetRecord{
		ActorId:   s.player.GetId(),
		ActorName: s.player.GetName(),
		ItemId:    rewards[0].Id,
		Count:     uint32(rewards[0].Count),
		TimeStamp: nowSec,
	}

	s.record(&data.DragonDraw.Record, record, dragonDrawPersonalRecord)

	globalRecord := s.globalRecord()

	if utils.SliceContainsUint32(conf.RecordSuperLibs, libId) {
		s.record(&globalRecord.HighRecord, record, dragonDrawGlobalHighRecord)
	} else {
		s.record(&globalRecord.NormalRecord, record, dragonDrawGlobalNormalRecord)
	}
}

func (s *DragonDrawSys) c2sSumDraw(msg *base.Message) error {
	var req pb3.C2S_56_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("dragon draw conf is nil")
	}

	sumConf := conf.SumDraw[req.Id]
	data := s.GetData()

	if sumConf.DrawTimes > data.LotteryData.Times || data.DragonDraw.SumAward[req.Id] {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	rewards := conf.SumDraw[req.Id].Rewards

	bagSys := s.player.GetSysObj(sysdef.SiBag).(*actorsystem.BagSystem)
	canAdd := engine.CheckRewards(s.GetPlayer(), rewards)
	if !canAdd && bagSys.AvailableCount() <= 10 {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	data.DragonDraw.SumAward[req.Id] = true

	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDragonDrawSumDraw})
	}

	s.SendProto3(56, 2, &pb3.S2C_56_2{
		ActiveId: s.Id,
		Id:       req.Id,
	})
	return nil
}

const (
	dragonDrawRecordGlobalType   = 1
	dragonDrawRecordPersonalType = 2

	dragonDrawHighPoolType   = 1
	dragonDrawNormalPoolType = 2

	dragonDrawGlobalHighRecord   = 50
	dragonDrawGlobalNormalRecord = 150
	dragonDrawPersonalRecord     = 100
)

func (s *DragonDrawSys) globalRecord() *pb3.DragonDrawRecord {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if nil == globalVar.PyyDatas.DragonDrawRecord {
		globalVar.PyyDatas.DragonDrawRecord = make(map[uint32]*pb3.DragonDrawRecord)
	}
	if nil == globalVar.PyyDatas.DragonDrawRecord[s.Id] {
		globalVar.PyyDatas.DragonDrawRecord[s.Id] = &pb3.DragonDrawRecord{}
	}
	if globalVar.PyyDatas.DragonDrawRecord[s.Id].StartTime == 0 {
		globalVar.PyyDatas.DragonDrawRecord[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.DragonDrawRecord[s.Id]
}

func (s *DragonDrawSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.DragonDrawRecord, s.GetId())
	}
}

func (s *DragonDrawSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_56_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("dragon draw conf not exist")
	}

	rsp := &pb3.S2C_56_3{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case dragonDrawRecordGlobalType:
		rsp.Record = s.globalRecord()
	case dragonDrawRecordPersonalType:
		rsp.Record = &pb3.DragonDrawRecord{NormalRecord: s.GetData().DragonDraw.Record}
	}

	s.SendProto3(56, 3, rsp)
	return nil
}

func (s *DragonDrawSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_56_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("fabao draw conf not exist")
	}

	data := s.GetData().DragonDraw
	exConf := conf.Exchange[req.Id]

	if exConf.ExchangeTimes > 0 && (data.Exchange[req.GetId()]+req.Count) > exConf.ExchangeTimes {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	if !s.player.ConsumeRate(exConf.Consume, int64(req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogDragonDrawExchange}) {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Exchange[req.Id] += req.Count

	rewards := jsondata.StdRewardMultiRate(exConf.Rewards, float64(req.Count))
	if len(rewards) > 0 {
		engine.GiveRewards(s.player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDragonDrawExchange})
	}

	s.player.SendProto3(56, 4, &pb3.S2C_56_4{
		ActiveId: s.Id,
		Id:       req.Id,
		Times:    data.Exchange[req.Id],
	})
	return nil
}

func (s *DragonDrawSys) c2sExchangeRemind(msg *base.Message) error {
	var req pb3.C2S_56_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("dragon draw conf not exist")
	}

	data := s.GetData().DragonDraw
	if req.Need {
		data.ExchangeRemind[req.Id] = true
	} else {
		delete(data.ExchangeRemind, req.Id)
	}
	s.SendProto3(56, 5, &pb3.S2C_56_5{
		ActiveId: s.Id,
		Id:       req.Id,
		Need:     data.ExchangeRemind[req.Id],
	})
	return nil
}

func (s *DragonDrawSys) SetHighPoolId(id uint32, isSend bool) {
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("dragon draw conf is nil")
		return
	}
	if id == 0 {
		id = conf.DefaultLibId
	}
	data := s.GetData()
	if nil == conf.Pool[id] {
		s.LogError("dragon draw cant find pool(%d)", id)
		return
	}
	data.DragonDraw.HighPoolId = id
	if isSend {
		s.SendProto3(56, 6, &pb3.S2C_56_6{ActiveId: s.Id, Id: data.DragonDraw.HighPoolId})
	}
}

func (s *DragonDrawSys) OnEnd() {
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	var rewards jsondata.StdRewardVec
	for _, v := range conf.SumDraw {
		if data.DragonDraw.SumAward[v.ID] || v.DrawTimes > data.LotteryData.Times {
			continue
		}
		data.DragonDraw.SumAward[v.ID] = true
		rewards = jsondata.MergeStdReward(rewards, v.Rewards)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_DragonDraw,
			Rewards: rewards,
		})
	}
}

func (s *DragonDrawSys) ResetDrawTimes(id, openTime uint32) {
	data := s.GetData()
	key := utils.Make64(openTime, id)
	if data.DragonDraw.LastResetKey > 0 && data.DragonDraw.LastResetKey != key {
		var rewards jsondata.StdRewardVec
		if conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx); nil != conf {
			for _, v := range conf.SumDraw {
				if data.DragonDraw.SumAward[v.ID] || v.DrawTimes > data.LotteryData.Times {
					continue
				}
				data.DragonDraw.SumAward[v.ID] = true
				rewards = jsondata.MergeStdReward(rewards, v.Rewards)
			}
		}
		if len(rewards) > 0 {
			mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
				ConfId:  common.Mail_DragonDraw,
				Rewards: rewards,
			})
		}
		//reset
		data.LotteryData.Times = 0
		data.DragonDraw.SumAward = make(map[uint32]bool)
	}
	data.DragonDraw.LastResetKey = key
	s.s2cInfo()
}

func (s *DragonDrawSys) NewDay() {
	s.lottery.OnLotteryNewDay()
	conf := jsondata.GetYYDragonDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	nowZero := time_util.GetZeroTime(time_util.NowSec())
	interval := nowZero - time_util.GetZeroTime(s.OpenTime)
	if interval > 0 && interval%conf.ResetExchangeTimeSec == 0 {
		s.ResetDrawTimes(s.Id, nowZero)
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDragonDraw, func() iface.IPlayerYY {
		return &DragonDrawSys{}
	})

	net.RegisterYYSysProtoV2(56, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonDrawSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(56, 2, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonDrawSys).c2sSumDraw
	})
	net.RegisterYYSysProtoV2(56, 3, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonDrawSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(56, 4, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonDrawSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(56, 5, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonDrawSys).c2sExchangeRemind
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.DragonHuntDrawTips1, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，活动id，道具
	})
	engine.RegRewardsBroadcastHandler(tipmsgid.DragonHuntDrawTips2, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，活动id，道具
	})
}
