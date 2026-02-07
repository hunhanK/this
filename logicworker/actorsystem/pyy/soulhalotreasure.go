/**
 * @Author: LvYuMeng
 * @Date: 2024/8/5
 * @Desc: 魂环秘宝
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
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
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SoulHaloTreasureSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *SoulHaloTreasureSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *SoulHaloTreasureSys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.LuckTimes
}

func (s *SoulHaloTreasureSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *SoulHaloTreasureSys) RawData() *pb3.LotteryData {
	data := s.data()
	return data.LotteryData
}

func (s *SoulHaloTreasureSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.SoulHaloTreasureRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()
	sData := s.data().GetSoulHaloTreasure()

	if pie.Uint32s(conf.SpRecordIds).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.GlobalRecordCount))
		s.record(&sData.SuperRecords, record, int(conf.PersonalRecordCount))
	} else {
		s.record(&gData.Records, record, int(conf.GlobalRecordCount))
		s.record(&sData.Records, record, int(conf.PersonalRecordCount))
	}
}

func (s *SoulHaloTreasureSys) record(records *[]*pb3.SoulHaloTreasureRecord, record *pb3.SoulHaloTreasureRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SoulHaloTreasureSys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
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

func (s *SoulHaloTreasureSys) data() *pb3.PYY_EvolutionSoulHaloTreasure {
	state := s.GetYYData()
	if nil == state.EvolutionSoulHaloTreasure {
		state.EvolutionSoulHaloTreasure = make(map[uint32]*pb3.PYY_EvolutionSoulHaloTreasure)
	}
	if state.EvolutionSoulHaloTreasure[s.Id] == nil {
		state.EvolutionSoulHaloTreasure[s.Id] = &pb3.PYY_EvolutionSoulHaloTreasure{}
	}
	sData := state.EvolutionSoulHaloTreasure[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.SoulHaloTreasure {
		sData.SoulHaloTreasure = &pb3.PYY_SoulHaloTreasure{}
	}
	if nil == sData.SoulHaloTreasure.Box {
		sData.SoulHaloTreasure.Box = make(map[uint32]*pb3.SoulHaloTreasureBox)
	}
	return sData
}

func (s *SoulHaloTreasureSys) getBoxDataById(boxId uint32) *pb3.SoulHaloTreasureBox {
	sData := s.data().GetSoulHaloTreasure()
	if nil == sData.Box[boxId] {
		sData.Box[boxId] = &pb3.SoulHaloTreasureBox{}
	}
	if nil == sData.Box[boxId].Exchange {
		sData.Box[boxId].Exchange = make(map[uint32]uint32)
	}
	return sData.Box[boxId]
}

func (s *SoulHaloTreasureSys) Login() {
	s.s2cInfo()
}

func (s *SoulHaloTreasureSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SoulHaloTreasureSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.SoulHaloTreasureRecord, s.GetId())
	}
}

func (s *SoulHaloTreasureSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionSoulHaloTreasure {
		return
	}
	delete(state.EvolutionSoulHaloTreasure, s.GetId())
}

func (s *SoulHaloTreasureSys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *SoulHaloTreasureSys) OnEnd() {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var rewardsVec []jsondata.StdRewardVec
	sData := s.data().GetSoulHaloTreasure()
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

func (s *SoulHaloTreasureSys) s2cInfo() {
	s.SendProto3(69, 170, &pb3.S2C_69_170{
		ActiveId: s.GetId(),
		Data:     s.data().GetSoulHaloTreasure(),
	})
}

func (s *SoulHaloTreasureSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func (s *SoulHaloTreasureSys) globalRecord() *pb3.SoulHaloTreasureRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.SoulHaloTreasureRecord == nil {
		globalVar.PyyDatas.SoulHaloTreasureRecord = make(map[uint32]*pb3.SoulHaloTreasureRecordList)
	}
	if globalVar.PyyDatas.SoulHaloTreasureRecord[s.Id] == nil {
		globalVar.PyyDatas.SoulHaloTreasureRecord[s.Id] = &pb3.SoulHaloTreasureRecordList{}
	}
	if globalVar.PyyDatas.SoulHaloTreasureRecord[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SoulHaloTreasureRecord[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SoulHaloTreasureRecord[s.Id]
}

func (s *SoulHaloTreasureSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_171
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys conf is nil")
	}

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys consumes conf is nik")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloTreasureConsume})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSoulHaloTreasureAward})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogSoulHaloTreasureAward,
		NoTips: true,
	})

	sData := s.data().GetSoulHaloTreasure()
	sData.TotalTimes += req.GetTimes()

	boxScore := s.randBoxByTimes(req.GetTimes())
	for boxId, score := range boxScore {
		s.updateScore(boxId, score, true)
	}

	rsp := &pb3.S2C_69_171{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
	}
	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.SoulHaloTreasureSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}
	for boxId, score := range boxScore {
		rsp.Box = append(rsp.Box, &pb3.SoulHaloTreasureBoxSt{
			Id:       boxId,
			AddScore: score,
			Score:    s.getBoxDataById(boxId).GetScore(),
		})
	}
	s.GetPlayer().TriggerQuestEvent(custom_id.QttSoulHaloTreasureDrawTime, 0, int64(req.Times))
	s.GetPlayer().TriggerQuestEvent(custom_id.QttAchievementsSoulHaloTreasureDrawTime, 0, int64(req.Times))
	s.SendProto3(69, 171, rsp)
	return nil
}

func (s *SoulHaloTreasureSys) randBoxByTimes(times uint32) map[uint32]uint32 {
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	result := make(map[uint32]uint32)
	for i := uint32(1); i <= times; i++ {
		for _, boxConf := range conf.Treasure {
			if !random.Hit(boxConf.Rate, 10000) {
				continue
			}
			result[boxConf.ID] += boxConf.GetDropScore()
		}
	}

	return result
}

const (
	SoulHaloTreasureRecordGType = 1
	SoulHaloTreasureRecordPType = 2
)

func (s *SoulHaloTreasureSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_173
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_173{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case SoulHaloTreasureRecordGType:
		gData := s.globalRecord()
		rsp.Record = make([]*pb3.SoulHaloTreasureRecord, 0, len(gData.Records)+len(gData.SuperRecords))
		rsp.Record = append(rsp.Record, gData.Records...)
		rsp.Record = append(rsp.Record, gData.SuperRecords...)
	case SoulHaloTreasureRecordPType:
		data := s.data().GetSoulHaloTreasure()
		rsp.Record = make([]*pb3.SoulHaloTreasureRecord, 0, len(data.Records)+len(data.SuperRecords))
		rsp.Record = append(rsp.Record, data.Records...)
		rsp.Record = append(rsp.Record, data.SuperRecords...)
	}

	s.SendProto3(69, 173, rsp)

	return nil
}

func (s *SoulHaloTreasureSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_69_174
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys conf is nil")
	}
	boxConf, ok := conf.Treasure[req.GetBoxId()]
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys box conf %d is nil", req.GetBoxId())
	}
	exchangeConf, ok := conf.GetExchangeConf(req.GetBoxId(), req.GetGoodsId())
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys box %d exchange %d conf is nil", req.GetBoxId(), req.GetGoodsId())
	}

	boxData := s.getBoxDataById(req.GetBoxId())
	count := boxData.Exchange[req.GetGoodsId()]
	if (count + req.GetCount()) > exchangeConf.Count {
		return neterror.ParamsInvalidError("exchange times not enough")
	}

	if !s.updateScore(req.GetBoxId(), boxConf.OpenScore*req.GetCount(), false) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	boxData.Exchange[req.GetGoodsId()] += req.GetCount()
	engine.GiveRewards(s.GetPlayer(), jsondata.StdRewardMulti(exchangeConf.Rewards, int64(req.GetCount())), common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSoulHaloTreasureExchange,
	})

	s.SendProto3(69, 174, &pb3.S2C_69_174{
		ActiveId: s.GetId(),
		BoxId:    req.GetBoxId(),
		GoodsId:  req.GetGoodsId(),
		Count:    boxData.Exchange[req.GetGoodsId()],
	})
	return nil
}

func (s *SoulHaloTreasureSys) updateScore(boxId, score uint32, isAdd bool) bool {
	box := s.getBoxDataById(boxId)
	if isAdd {
		box.Score += score
	} else {
		if box.Score < score {
			return false
		}
		box.Score -= score
	}
	s.SendProto3(69, 172, &pb3.S2C_69_172{
		ActiveId: s.GetId(),
		BoxId:    boxId,
		Score:    box.Score,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSoulHaloTreasureBoxScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"boxId": boxId,
			"score": box.Score,
		}),
	})
	return true
}

func (s *SoulHaloTreasureSys) c2sSumAward(msg *base.Message) error {
	var req pb3.C2S_69_175
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYSoulHaloTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys conf is nil")
	}
	sumAwardConf, ok := conf.SumAwards[req.GetId()]
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloTreasureSys SumAwards conf %d is nil", req.GetId())
	}
	sData := s.data().GetSoulHaloTreasure()
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
		LogId: pb3.LogId_LogSoulHaloTreasureSumAward,
	})
	s.SendProto3(69, 175, &pb3.S2C_69_175{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSoulHaloTreasure, func() iface.IPlayerYY {
		return &SoulHaloTreasureSys{}
	})

	net.RegisterYYSysProtoV2(69, 171, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SoulHaloTreasureSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(69, 173, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SoulHaloTreasureSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(69, 174, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SoulHaloTreasureSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(69, 175, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SoulHaloTreasureSys).c2sSumAward
	})
}
