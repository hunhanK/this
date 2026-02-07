/**
 * @Author: LvYuMeng
 * @Date: 2024/8/20
 * @Desc: 惊喜盲盒
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/moneydef"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type SurpriseMysteryBoxSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *SurpriseMysteryBoxSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *SurpriseMysteryBoxSys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.LuckTimes
}

func (s *SurpriseMysteryBoxSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *SurpriseMysteryBoxSys) RawData() *pb3.LotteryData {
	data := s.data()
	return data.LotteryData
}

func (s *SurpriseMysteryBoxSys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
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

func (s *SurpriseMysteryBoxSys) IsDrawFull() bool {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return true
	}

	if len(conf.LimitLibIds) == 0 {
		return false
	}

	for _, libId := range conf.LimitLibIds {
		if s.lottery.CanLibUse(libId, true) {
			return false
		}
	}

	return true
}

func (s *SurpriseMysteryBoxSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	sData := s.data().GetSurpriseMysteryBoxes()
	if nil == sData.LibHit[libId] {
		sData.LibHit[libId] = &pb3.SurpriseMysteryBoxesLibHit{}
	}
	if nil == sData.LibHit[libId].AwardPoolId {
		sData.LibHit[libId].AwardPoolId = make(map[uint32]uint32)
	}
	sData.LibHit[libId].AwardPoolId[awardPoolConf.Id]++

	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.SurpriseMysteryBoxesRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()

	if pie.Uint32s(conf.SpRecordIds).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.GlobalRecordCount))
		s.record(&sData.SuperRecords, record, int(conf.PersonalRecordCount))
		s.SendProto3(69, 197, &pb3.S2C_69_197{
			ActiveId: s.GetId(),
			Type:     SurpriseMysteryBoxRecordPType,
			Record:   record,
		})
		engine.Broadcast(chatdef.CIWorld, 0, 69, 197, &pb3.S2C_69_197{
			ActiveId: s.GetId(),
			Type:     SurpriseMysteryBoxRecordGType,
			Record:   record,
		}, 0)
	} else {
		s.record(&gData.Records, record, int(conf.GlobalRecordCount))
		s.record(&sData.Records, record, int(conf.PersonalRecordCount))
	}
}

func (s *SurpriseMysteryBoxSys) record(records *[]*pb3.SurpriseMysteryBoxesRecord, record *pb3.SurpriseMysteryBoxesRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SurpriseMysteryBoxSys) globalRecord() *pb3.SurpriseMysteryBoxesRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.SupriseMysteryBoxesRecord == nil {
		globalVar.PyyDatas.SupriseMysteryBoxesRecord = make(map[uint32]*pb3.SurpriseMysteryBoxesRecordsList)
	}
	if globalVar.PyyDatas.SupriseMysteryBoxesRecord[s.Id] == nil {
		globalVar.PyyDatas.SupriseMysteryBoxesRecord[s.Id] = &pb3.SurpriseMysteryBoxesRecordsList{}
	}
	if globalVar.PyyDatas.SupriseMysteryBoxesRecord[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SupriseMysteryBoxesRecord[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SupriseMysteryBoxesRecord[s.Id]
}

func (s *SurpriseMysteryBoxSys) data() *pb3.PYY_EvolutionSurpriseMysteryBoxes {
	state := s.GetYYData()
	if nil == state.EvolutionSurpriseMysteryBoxes {
		state.EvolutionSurpriseMysteryBoxes = make(map[uint32]*pb3.PYY_EvolutionSurpriseMysteryBoxes)
	}
	if state.EvolutionSurpriseMysteryBoxes[s.Id] == nil {
		state.EvolutionSurpriseMysteryBoxes[s.Id] = &pb3.PYY_EvolutionSurpriseMysteryBoxes{}
	}
	sData := state.EvolutionSurpriseMysteryBoxes[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.SurpriseMysteryBoxes {
		sData.SurpriseMysteryBoxes = &pb3.PYY_SurpriseMysteryBoxes{}
	}
	if nil == sData.SurpriseMysteryBoxes.Exchange {
		sData.SurpriseMysteryBoxes.Exchange = make(map[uint32]uint32)
	}
	if nil == sData.SurpriseMysteryBoxes.LibHit {
		sData.SurpriseMysteryBoxes.LibHit = make(map[uint32]*pb3.SurpriseMysteryBoxesLibHit)
	}
	return sData
}

func (s *SurpriseMysteryBoxSys) Login() {
	s.callCrossOpen()
	s.s2cInfo()
	s.s2cCrossInfo()
}

func (s *SurpriseMysteryBoxSys) OnReconnect() {
	s.callCrossOpen()
	s.s2cInfo()
	s.s2cCrossInfo()
}

func (s *SurpriseMysteryBoxSys) s2cInfo() {
	data := s.data()
	s.SendProto3(69, 190, &pb3.S2C_69_190{
		ActiveId: s.GetId(),
		Info:     data.GetSurpriseMysteryBoxes(),
		LotteryInfo: &pb3.SurpriseMysteryBoxesLotteryInfo{
			StageCount:  data.GetLotteryData().GetStageCount(),
			PlayerLucky: data.GetLotteryData().GetPlayerLucky(),
			IsFull:      s.IsDrawFull(),
		},
	})
}

func (s *SurpriseMysteryBoxSys) s2cCrossInfo() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYAddSurpriseMysteryBoxReq, &pb3.CommonSt{
		U64Param:  s.GetPlayer().GetId(),
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
		U32Param3: s.GetId(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *SurpriseMysteryBoxSys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
	s.callCrossOpen()
	s.s2cCrossInfo()
}

func (s *SurpriseMysteryBoxSys) OnEnd() {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var rewardsVec []jsondata.StdRewardVec
	sData := s.data().GetSurpriseMysteryBoxes()
	for _, v := range conf.SumAwards {
		if pie.Uint32s(sData.RevSumAwards).Contains(v.Id) {
			continue
		}
		if sData.TotalTimes < v.Times {
			continue
		}
		sData.RevSumAwards = append(sData.RevSumAwards, v.Id)
		rewardsVec = append(rewardsVec, v.Rewards)
	}
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *SurpriseMysteryBoxSys) callCrossOpen() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYAddSurpriseMysteryBoxActInfoSync, &pb3.G2CSyncSurpriseMysteryBoxesInfo{
		Id:        s.GetId(),
		StartTime: s.GetOpenTime(),
		EndTime:   s.GetEndTime(),
		ConfIdx:   s.GetConfIdx(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *SurpriseMysteryBoxSys) addDegree(score uint32) {
	s.callCrossOpen()
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYAddSurpriseMysteryBoxDegree, &pb3.CommonSt{
		U32Param:  s.GetId(),
		U32Param2: score,
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *SurpriseMysteryBoxSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.SupriseMysteryBoxesRecord, s.GetId())
	}
}

func (s *SurpriseMysteryBoxSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionSurpriseMysteryBoxes {
		return
	}
	delete(state.EvolutionSurpriseMysteryBoxes, s.GetId())
}

func (s *SurpriseMysteryBoxSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_191
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys conf is nil")
	}

	if s.IsDrawFull() {
		return neterror.ParamsInvalidError("draw full")
	}

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys consumes conf is nik")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogSurpriseMysteryBoxesDrawConsume})
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

	bundledAwards := jsondata.StdRewardMulti(conf.GiveRewards, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSurpriseMysteryBoxesDrawAwards})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogSurpriseMysteryBoxesDrawAwards,
		NoTips: true,
	})
	s.addDegree(conf.OnceIncDegree * req.Times)

	sData := s.data().GetSurpriseMysteryBoxes()
	lData := s.data().GetLotteryData()
	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_69_191{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
		Lucky:      lData.GetPlayerLucky(),
		LibHit:     sData.GetLibHit(),
		IsFull:     s.IsDrawFull(),
	}

	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.SurpriseMysteryBoxesSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}

	s.SendProto3(69, 191, rsp)
	return nil

}

const (
	SurpriseMysteryBoxRecordGType = 1
	SurpriseMysteryBoxRecordPType = 2
)

func (s *SurpriseMysteryBoxSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_192
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_192{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case SurpriseMysteryBoxRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case SurpriseMysteryBoxRecordPType:
		data := s.data().GetSurpriseMysteryBoxes()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(69, 192, rsp)
	return nil
}

func (s *SurpriseMysteryBoxSys) c2sSumAward(msg *base.Message) error {
	var req pb3.C2S_69_193
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys conf is nil")
	}
	sumAwardConf, ok := conf.SumAwards[req.GetId()]
	if !ok {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys SumAwards conf %d is nil", req.GetId())
	}

	sData := s.data().GetSurpriseMysteryBoxes()
	if pie.Uint32s(sData.RevSumAwards).Contains(req.GetId()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	if sumAwardConf.Times > sData.GetTotalTimes() {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	sData.RevSumAwards = append(sData.RevSumAwards, req.GetId())
	engine.GiveRewards(s.GetPlayer(), sumAwardConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSurpriseMysteryBoxesSumAwards,
	})

	s.SendProto3(69, 193, &pb3.S2C_69_193{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
	})
	return nil
}

func (s *SurpriseMysteryBoxSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_69_194
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys conf is nil")
	}

	sData := s.data().GetSurpriseMysteryBoxes()
	exConf := conf.Exchange[req.Id]

	if exConf.ExchangeTimes > 0 && (sData.Exchange[req.GetId()]+req.Count) > exConf.ExchangeTimes {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	if !s.player.ConsumeRate(exConf.Consume, int64(req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogSurpriseMysteryBoxesExchangeConsume}) {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	sData.Exchange[req.Id] += req.Count

	rewards := jsondata.StdRewardMultiRate(exConf.Rewards, float64(req.Count))
	if len(rewards) > 0 {
		engine.GiveRewards(s.player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSurpriseMysteryBoxesExchangeAwards})
	}

	s.player.SendProto3(69, 194, &pb3.S2C_69_194{
		ActiveId: s.Id,
		Id:       req.Id,
		Times:    sData.Exchange[req.Id],
	})
	return nil
}

func (s *SurpriseMysteryBoxSys) c2sRemind(msg *base.Message) error {
	var req pb3.C2S_69_195
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SurpriseMysteryBoxSys conf is nil")
	}
	if _, ok := conf.Exchange[req.GetId()]; !ok {
		return neterror.ParamsInvalidError("exchange conf %d is nil", req.GetId())
	}

	sData := s.data().GetSurpriseMysteryBoxes()
	if req.Need {
		if !pie.Uint32s(sData.ExchangeRemind).Contains(req.GetId()) {
			sData.ExchangeRemind = append(sData.ExchangeRemind, req.GetId())
		}
	} else {
		sData.ExchangeRemind = pie.Uint32s(sData.ExchangeRemind).Filter(func(u uint32) bool {
			return u != req.GetId()
		})
	}
	s.SendProto3(69, 195, &pb3.S2C_69_195{
		ActiveId: s.Id,
		Id:       req.GetId(),
		Need:     req.GetNeed(),
	})
	return nil
}

func (s *SurpriseMysteryBoxSys) c2sDegreeAward(msg *base.Message) error {
	var req pb3.C2S_69_198
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	for _, v := range req.GetId() {
		if ok, err := s.canRvDegreeAward(v); !ok {
			return err
		}
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYAddSurpriseMysteryBoxAwardReq, &pb3.G2CSurpriseMysteryBoxesAwardsReq{
		ActiveId: s.GetId(),
		AwardId:  req.GetId(),
		ActorId:  s.GetPlayer().GetId(),
		PfId:     engine.GetPfId(),
		SrvId:    engine.GetServerId(),
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SurpriseMysteryBoxSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func (s *SurpriseMysteryBoxSys) canRvDegreeAward(id uint32) (bool, error) {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false, neterror.ConfNotFoundError("SurpriseMysteryBoxSys conf is nil")
	}
	if _, ok := conf.DegreeAwards[id]; !ok {
		return false, neterror.ConfNotFoundError("SurpriseMysteryBoxSys degree award conf is nil")
	}
	sData := s.data().GetSurpriseMysteryBoxes()
	if pie.Uint32s(sData.DegreeAward).Contains(id) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return false, nil
	}
	if sData.TotalTimes < conf.DegreeAwards[id].DrawTimes {
		return false, neterror.ParamsInvalidError("total draw times not enough")
	}
	return true, nil
}

func (s *SurpriseMysteryBoxSys) revDegreeAward(ids []uint32, degree uint32) {
	conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var revIds []uint32
	var rewardVec []jsondata.StdRewardVec
	for _, id := range ids {
		if ok, _ := s.canRvDegreeAward(id); !ok {
			continue
		}
		if degree < conf.DegreeAwards[id].Degree {
			continue
		}
		sData := s.data().GetSurpriseMysteryBoxes()
		sData.DegreeAward = append(sData.DegreeAward, id)
		revIds = append(revIds, id)
		rewardVec = append(rewardVec, conf.DegreeAwards[id].Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardVec...)
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSurpriseMysteryBoxesDegreeAwards,
		})
	}
	if len(revIds) > 0 {
		s.SendProto3(69, 198, &pb3.S2C_69_198{
			ActiveId: s.GetId(),
			Id:       revIds,
		})
	}
}

func onSurpriseMysteryBoxAwardRet(buf []byte) {
	msg := &pb3.C2GSurpriseMysteryBoxesAwardsRet{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	player := manager.GetPlayerPtrById(msg.GetActorId())
	if nil == player {
		return
	}

	yy := pyymgr.GetPlayerYYObj(player, msg.GetActiveId())
	if nil == yy || !yy.IsOpen() {
		return
	}
	myYY, ok := yy.(*SurpriseMysteryBoxSys)
	if !ok {
		return
	}
	myYY.revDegreeAward(msg.GetAwardId(), msg.GetDegree())
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSurpriseMysteryBox, func() iface.IPlayerYY {
		return &SurpriseMysteryBoxSys{}
	})

	engine.RegisterSysCall(sysfuncid.C2FYYAddSurpriseMysteryBoxAwardRet, onSurpriseMysteryBoxAwardRet)

	net.RegisterYYSysProtoV2(69, 191, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(69, 192, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(69, 193, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sSumAward
	})
	net.RegisterYYSysProtoV2(69, 194, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(69, 195, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sRemind
	})
	net.RegisterYYSysProtoV2(69, 198, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SurpriseMysteryBoxSys).c2sDegreeAward
	})

	gmevent.Register("mysteryBox.degree", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		score := utils.AtoUint32(args[1])
		yy := pyymgr.GetPlayerYYObj(player, yyId)
		if nil == yy || !yy.IsOpen() {
			return false
		}
		myYY, ok := yy.(*SurpriseMysteryBoxSys)
		if !ok {
			return false
		}
		myYY.addDegree(score)
		return true
	}, 1)

	gmevent.Register("mysteryBox", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		count := utils.AtoUint32(args[1])
		yy := pyymgr.GetPlayerYYObj(player, yyId)
		if nil == yy || !yy.IsOpen() {
			return false
		}
		myYY, ok := yy.(*SurpriseMysteryBoxSys)
		if !ok {
			return false
		}
		conf, ok := jsondata.GetYYSurpriseMysteryBoxConf(myYY.ConfName, myYY.ConfIdx)
		if !ok {
			return false
		}
		myYY.lottery.GmDraw(count, 0, conf.LibIds)
		return true
	}, 1)
}
