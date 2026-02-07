/**
 * @Author: LvYuMeng
 * @Date: 2024/11/20
 * @Desc: 剑装抽奖
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type SwordEquipDrawSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *SwordEquipDrawSys) Login() {
	s.s2cInfo()
}

func (s *SwordEquipDrawSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SwordEquipDrawSys) OnOpen() {
	s.clearRecord()
	s.clearLucky()
	s.s2cInfo()
}

func (s *SwordEquipDrawSys) clearLucky() {
	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if conf.ClearLucky {
		s.RawData().PlayerLucky = 0
	}
}

func (s *SwordEquipDrawSys) s2cInfo() {
	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.getData()
	s.SendProto3(56, 20, &pb3.S2C_56_20{
		ActiveId: s.GetId(),
		Info:     data.GetSwordEquipDraw(),
		LotteryInfo: &pb3.SwordEquipDrawLotteryInfo{
			PlayerLucky: s.lottery.GetGuaranteeCount(conf.MaxAwardLibId),
		},
	})
}

func (s *SwordEquipDrawSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.SwordEquipDrawRecords, s.GetId())
	}
}

func (s *SwordEquipDrawSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionSwordEquipDraw {
		return
	}

	oldLucky := s.RawData().PlayerLucky

	state.EvolutionSwordEquipDraw[s.GetId()] = &pb3.PYY_EvolutionSwordEquipDraw{
		LotteryData: &pb3.LotteryData{PlayerLucky: oldLucky},
	}
}

func (s *SwordEquipDrawSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *SwordEquipDrawSys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.LuckTimes
}

func (s *SwordEquipDrawSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *SwordEquipDrawSys) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *SwordEquipDrawSys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	consumeConf := conf.GetDrawConsume(1)
	singlePrice := jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

const (
	SwordEquipDrawRecordGType = 1
	SwordEquipDrawRecordPType = 2
)

func (s *SwordEquipDrawSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().GetSwordEquipDraw()
	if _, ok := data.LibHit[libId]; !ok {
		data.LibHit[libId] = &pb3.SwordEquipDrawLibHit{}
	}
	if nil == data.LibHit[libId].AwardPoolId {
		data.LibHit[libId].AwardPoolId = make(map[uint32]uint32)
	}
	data.LibHit[libId].AwardPoolId[awardPoolConf.Id]++

	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.SwordEquipDrawRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()

	if pie.Uint32s(conf.RecordSuperLibs).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.RecordNum))
		s.record(&data.SuperRecords, record, int(conf.RecordNum))
	} else {
		s.record(&gData.Records, record, int(conf.RecordNum))
		s.record(&data.Records, record, int(conf.RecordNum))
	}
}

func (s *SwordEquipDrawSys) record(records *[]*pb3.SwordEquipDrawRecord, record *pb3.SwordEquipDrawRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SwordEquipDrawSys) globalRecord() *pb3.SwordEquipDrawRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.SwordEquipDrawRecords == nil {
		globalVar.PyyDatas.SwordEquipDrawRecords = make(map[uint32]*pb3.SwordEquipDrawRecordsList)
	}
	if globalVar.PyyDatas.SwordEquipDrawRecords[s.Id] == nil {
		globalVar.PyyDatas.SwordEquipDrawRecords[s.Id] = &pb3.SwordEquipDrawRecordsList{}
	}
	if globalVar.PyyDatas.SwordEquipDrawRecords[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SwordEquipDrawRecords[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SwordEquipDrawRecords[s.Id]
}

func (s *SwordEquipDrawSys) getData() *pb3.PYY_EvolutionSwordEquipDraw {
	state := s.GetYYData()
	if nil == state.EvolutionSwordEquipDraw {
		state.EvolutionSwordEquipDraw = make(map[uint32]*pb3.PYY_EvolutionSwordEquipDraw)
	}
	if state.EvolutionSwordEquipDraw[s.Id] == nil {
		state.EvolutionSwordEquipDraw[s.Id] = &pb3.PYY_EvolutionSwordEquipDraw{}
	}
	sData := state.EvolutionSwordEquipDraw[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.SwordEquipDraw {
		sData.SwordEquipDraw = &pb3.PYY_SwordEquipDraw{}
	}
	if nil == sData.SwordEquipDraw.ExchangeRemind {
		sData.SwordEquipDraw.ExchangeRemind = make(map[uint32]bool)
	}
	if nil == sData.SwordEquipDraw.Exchange {
		sData.SwordEquipDraw.Exchange = make(map[uint32]uint32)
	}
	if nil == sData.SwordEquipDraw.LibHit {
		sData.SwordEquipDraw.LibHit = make(map[uint32]*pb3.SwordEquipDrawLibHit)
	}
	return sData
}

func (s *SwordEquipDrawSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_56_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SwordEquipDrawSys conf is nil")
	}

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("SwordEquipDrawSys consumes conf is nik")
	}

	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogSwordEquipDrawConsume})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()
	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}

	bundledAwards := jsondata.StdRewardMulti(conf.DrawScore, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSwordEquipDrawAward})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogSwordEquipDrawAward,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sData := s.getData().GetSwordEquipDraw()
	sData.TotalTimes += req.GetTimes()
	s.GetPlayer().TriggerQuestEvent(custom_id.QttFairySwordDrawDrawTimes, 0, int64(req.Times))
	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActSwordEquip,
		ActId:   s.Id,
		Times:   req.Times,
	})

	rsp := &pb3.S2C_56_21{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
		Lucky:      s.lottery.GetGuaranteeCount(conf.MaxAwardLibId),
		LibHit:     sData.GetLibHit(),
	}
	for _, v := range result.LibResult {
		rewards := engine.FilterRewardByPlayer(s.GetPlayer(), v.AwardPoolConf.Awards)
		rsp.Result = append(rsp.Result, &pb3.SwordEquipDrawSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
			ItemId:      rewards[0].Id,
			Count:       uint32(rewards[0].Count),
		})
	}

	s.SendProto3(56, 21, rsp)
	return nil

}

func (s *SwordEquipDrawSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_56_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_56_23{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case SwordEquipDrawRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case SwordEquipDrawRecordPType:
		data := s.getData().GetSwordEquipDraw()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(56, 23, rsp)
	return nil
}

func (s *SwordEquipDrawSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_56_24
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("conf not exist")
	}

	data := s.getData().GetSwordEquipDraw()
	exConf := conf.Exchange[req.Id]

	if exConf.ExchangeTimes > 0 && (data.Exchange[req.GetId()]+req.Count) > exConf.ExchangeTimes {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	if !s.player.ConsumeRate(exConf.Consume, int64(req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogSwordEquipDrawExchange}) {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Exchange[req.Id] += req.Count

	rewards := jsondata.StdRewardMulti(exConf.Rewards, int64(req.Count))
	if len(rewards) > 0 {
		engine.GiveRewards(s.player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSwordEquipDrawExchange})
	}

	s.player.SendProto3(56, 24, &pb3.S2C_56_24{
		ActiveId: s.Id,
		Id:       req.Id,
		Times:    data.Exchange[req.Id],
	})
	return nil
}

func (s *SwordEquipDrawSys) c2sRemind(msg *base.Message) error {
	var req pb3.C2S_56_25
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYSwordEquipDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("conf is nil")
	}

	data := s.getData().GetSwordEquipDraw()

	if req.GetNeed() {
		if _, ok := conf.Exchange[req.GetId()]; !ok {
			return neterror.ConfNotFoundError("Exchange conf %d not found", req.GetId())
		}
	} else {
		delete(data.ExchangeRemind, req.GetId())
	}

	s.SendProto3(56, 25, &pb3.S2C_56_25{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
		Need:     req.GetNeed(),
	})

	return nil
}

func (s *SwordEquipDrawSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSwordEquipDraw, func() iface.IPlayerYY {
		return &SwordEquipDrawSys{}
	})

	net.RegisterYYSysProtoV2(56, 21, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SwordEquipDrawSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(56, 23, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SwordEquipDrawSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(56, 24, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SwordEquipDrawSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(56, 25, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SwordEquipDrawSys).c2sRemind
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.SwordEquipDrawTip, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，道具
	})
}
