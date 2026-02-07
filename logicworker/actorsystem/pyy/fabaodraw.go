package pyy

/**
 * @Author: LvYuMeng
 * @Date: 2023/10/31
 * @Desc: 法宝抽奖
**/

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/reachconddef"
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
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"os"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/srvlib/utils"
)

type FaBaoDrawSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *FaBaoDrawSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *FaBaoDrawSys) GetData() *pb3.PYY_EvolutionFaBaoDraw {
	yyData := s.GetYYData()
	//抽奖数据
	if nil == yyData.EvolutionFaBaoDraw {
		yyData.EvolutionFaBaoDraw = make(map[uint32]*pb3.PYY_EvolutionFaBaoDraw)
	}
	if nil == yyData.EvolutionFaBaoDraw[s.Id] {
		yyData.EvolutionFaBaoDraw[s.Id] = &pb3.PYY_EvolutionFaBaoDraw{}
	}
	sData := yyData.EvolutionFaBaoDraw[s.Id]

	//抽奖数据
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	//法宝活动数据
	if nil == sData.FaBaoDraw {
		sData.FaBaoDraw = &pb3.PYY_FaBaoDraw{}
	}
	s.InitData(sData.FaBaoDraw)
	return sData
}

func (s *FaBaoDrawSys) InitData(data *pb3.PYY_FaBaoDraw) {
	if nil == data.Exchange {
		data.Exchange = make(map[uint32]uint32)
	}
	if nil == data.Record {
		data.Record = &pb3.FaBaoDrawRecord{}
	}
	if nil == data.SumAward {
		data.SumAward = make(map[uint32]bool)
	}
	if nil == data.ItemCount {
		data.ItemCount = make(map[uint32]uint32)
	}
	if nil == data.LibHit {
		data.LibHit = make(map[uint32]*pb3.FaBaoDrawLibHit)
	}
	return
}

func (s *FaBaoDrawSys) GetHitItemNums() uint32 {
	data := s.GetData()
	return uint32(len(data.FaBaoDraw.ItemCount))
}

func (s *FaBaoDrawSys) s2cInfo() {
	conf, err := s.GetConf()
	if nil != err {
		return
	}
	data := s.GetData()
	s.SendProto3(47, 0, &pb3.S2C_47_0{
		ActiveId: s.Id,
		Info:     data.FaBaoDraw,
		Lottery: &pb3.FaBaoLotteryInfo{
			DrawTimes:         data.LotteryData.Times,
			Lucky:             data.LotteryData.StageCount,
			PlayerLuckyLibId:  s.lottery.GetLibId(conf.SmallLibId),
			PlayerLucky:       s.lottery.GetGuaranteeCount(conf.SmallLibId),
			DestinyLuckyLibId: s.lottery.GetLibId(conf.SuperLibId),
			DestinyLucky:      s.lottery.GetGuaranteeCount(conf.SuperLibId),
		},
	})
	s.SendProto3(47, 5, &pb3.S2C_47_5{ActiveId: s.Id, Record: s.getGlobalRecord(), Type: fabaoDrawRecordGlobalType})
}

func (s *FaBaoDrawSys) GetLuckTimes() uint16 {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	return conf.LuckTimes
}

func (s *FaBaoDrawSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	return conf.LuckyValEx
}

func (s *FaBaoDrawSys) RawData() *pb3.LotteryData {
	data := s.GetData()
	return data.LotteryData
}

func (s *FaBaoDrawSys) Login() {
	s.s2cInfo()
}

func (s *FaBaoDrawSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FaBaoDrawSys) OnOpen() {
	s.remindInit()
	s.s2cInfo()
}

func (s *FaBaoDrawSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionFaBaoDraw {
		return
	}
	delete(state.EvolutionFaBaoDraw, s.Id)
}

func (s *FaBaoDrawSys) remindInit() {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	for _, v := range conf.Exchange {
		if v.Remind == 1 {
			data.FaBaoDraw.ExchangeRemind = append(data.FaBaoDraw.ExchangeRemind, v.ID)
		}
	}
}

func (s *FaBaoDrawSys) OnEnd() {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	var rewards jsondata.StdRewardVec
	var actName string
	if pyyConf := jsondata.GetPlayerYYConf(s.GetId()); nil != pyyConf {
		actName = pyyConf.Name
	}
	var addScore int64
	drawTime := data.LotteryData.Times
	for k, v := range conf.SumDraw {
		if v.DrawTimes <= drawTime && !data.FaBaoDraw.SumAward[k] {
			rewards = jsondata.MergeStdReward(rewards, v.Rewards)
			data.FaBaoDraw.SumAward[k] = true
			addScore += int64(v.Score)
		}
	}
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_FaBaoDrawSumAward,
			Rewards: rewards,
			Content: &mailargs.PYYNameArgs{Name: actName},
		})
	}
	s.addScore(addScore, pb3.LogId_LogFaBaoDrawEndSwitch, false)

	amount := data.FaBaoDraw.GetScore()
	data.FaBaoDraw.Score = 0
	s.logScoreChange(amount, data.FaBaoDraw.Score, pb3.LogId_LogFaBaoDrawEndSwitch)
	if amount > 0 {
		var switchAward jsondata.StdRewardVec
		for _, v := range conf.Switch {
			switchAward = append(switchAward, &jsondata.StdReward{
				Id:    v.Id,
				Count: v.Count * amount,
				Bind:  v.Bind,
				Job:   v.Job,
			})
		}
		if len(switchAward) > 0 {
			itemConf := jsondata.GetItemConfig(switchAward[0].Id)
			mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
				ConfId:  common.Mail_FaBaoDrawEndSwitch,
				Rewards: switchAward,
				Content: &mailargs.FaBaoDrawSwitchArgs{
					ActName:     actName,
					ScoreName:   conf.ScoreName,
					ScoreCount:  amount,
					SwitchName:  itemConf.Name,
					SwitchCount: switchAward[0].Count,
				},
			})
		}
	}

	s.clearRecord(true)
}

func (s *FaBaoDrawSys) clearRecord(isEnd bool) {
	record := s.getGlobalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().FaBaoDrawRecord, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().FaBaoDrawRecord, s.Id)
	}
}

func (s *FaBaoDrawSys) NewDay() {
	s.lottery.OnLotteryNewDay()
	s.s2cInfo()
}

const (
	oneFaBaoDraw   = 1
	tenFaBaoDraw   = 10
	fiftyFaBaoDraw = 50

	fabaoDrawRecordHighSave   = 50
	fabaoDrawRecordMiddleSave = 50

	fabaoDrawRecordGlobalType   = 1
	fabaoDrawRecordPersonalType = 2
)

var faoBaoDrawFixTimes = []uint32{oneFaBaoDraw, tenFaBaoDraw, fiftyFaBaoDraw}

func (s *FaBaoDrawSys) getGlobalRecord() *pb3.FaBaoDrawRecord {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.FaBaoDrawRecord {
		globalVar.FaBaoDrawRecord = make(map[uint32]*pb3.FaBaoDrawRecord)
	}
	if nil == globalVar.FaBaoDrawRecord[s.Id] {
		globalVar.FaBaoDrawRecord[s.Id] = &pb3.FaBaoDrawRecord{}
	}
	if globalVar.FaBaoDrawRecord[s.Id].StartTime == 0 {
		globalVar.FaBaoDrawRecord[s.Id].StartTime = s.GetOpenTime()
	}
	if globalVar.FaBaoDrawRecord[s.Id].StartTime < s.GetOpenTime() {
		globalVar.FaBaoDrawRecord[s.Id] = &pb3.FaBaoDrawRecord{StartTime: s.OpenTime}
	}
	return globalVar.FaBaoDrawRecord[s.Id]
}

func (s *FaBaoDrawSys) consumeByTimes(conf *jsondata.FaBaoDrawConf, times uint32, autoBuy bool) (bool, uint32) {
	var consume jsondata.ConsumeVec
	switch times {
	case oneFaBaoDraw:
		consume = conf.Cos1
	case tenFaBaoDraw:
		consume = conf.Cos10
	case fiftyFaBaoDraw:
		consume = conf.Cos50
	}
	success, remove := s.GetPlayer().ConsumeByConfWithRet(consume, autoBuy, common.ConsumeParams{
		LogId:   pb3.LogId_LogFaBaoDrawAward,
		SubType: s.GetId(),
	})
	if !success {
		return false, 0
	}
	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()
	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}
	return success, useDiamondCount
}

func (s *FaBaoDrawSys) GetSingleDiamondPrice() uint32 {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	itemId := conf.Cos1[0].Id
	singlePrice := jsondata.GetAutoBuyItemPrice(itemId, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(itemId, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *FaBaoDrawSys) addScore(score int64, logId pb3.LogId, send bool) {
	if score <= 0 {
		s.LogWarn("player:%d add fabao draw score err which score below 0", s.player.GetId())
		return
	}
	data := s.GetData()
	oldScore := score
	data.FaBaoDraw.Score += score
	s.logScoreChange(oldScore, score, logId)
	if send {
		s.SendProto3(47, 10, &pb3.S2C_47_10{ActiveId: s.GetId(), Score: data.FaBaoDraw.Score})
		conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
		if nil == conf {
			return
		}
		item := &pb3.ItemSt{
			ItemId: conf.ScoreItemId,
			Count:  score,
		}
		s.GetPlayer().SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))

		return
	}
}

func (s *FaBaoDrawSys) logScoreChange(oldScore, score int64, logId pb3.LogId) {
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldScore": oldScore,
		"newScore": score,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArg),
	})
}

func (s *FaBaoDrawSys) record(records *[]*pb3.ItemGetRecord, record *pb3.ItemGetRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *FaBaoDrawSys) recordHit(libId uint32, awardPoolId uint32) {
	data := s.GetData().FaBaoDraw
	if nil == data.LibHit[libId] {
		data.LibHit[libId] = &pb3.FaBaoDrawLibHit{}
	}
	if nil == data.LibHit[libId].AwardPoolId {
		data.LibHit[libId].AwardPoolId = make(map[uint32]uint32)
	}
	data.LibHit[libId].AwardPoolId[awardPoolId]++
}

func (s *FaBaoDrawSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	if len(oneAward) <= 0 {
		return
	}

	conf, err := s.GetConf()
	if nil != err {
		return
	}

	nowSec := time_util.NowSec()
	data := s.GetData().FaBaoDraw
	gData := s.getGlobalRecord()

	s.recordHit(libId, awardPoolConf.Id)

	record := &pb3.ItemGetRecord{
		Type:      libId,
		ActorId:   s.GetPlayer().GetId(),
		ActorName: s.GetPlayer().GetName(),
		TimeStamp: nowSec,
		ItemId:    oneAward[0].Id,
		Count:     uint32(oneAward[0].Count),
	}

	if utils.SliceContainsUint32(conf.RecordSuperLibs, libId) {
		s.record(&gData.HighRecord, record, fabaoDrawRecordHighSave)
		s.record(&data.Record.HighRecord, record, fabaoDrawRecordHighSave)
	} else if utils.SliceContainsUint32(conf.RecordNormalLibs, libId) {
		s.record(&gData.MiddleRecord, record, fabaoDrawRecordMiddleSave)
		s.record(&data.Record.MiddleRecord, record, fabaoDrawRecordMiddleSave)
	} else {
		return
	}

	if awardPoolConf.IsRecord {
		engine.Broadcast(chatdef.CIWorld, 0, 47, 6, &pb3.S2C_47_6{Record: []*pb3.ItemGetRecord{record}}, 0)
	}
}

func (s *FaBaoDrawSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_47_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if !pie.Uint32s(faoBaoDrawFixTimes).Contains(req.Times) {
		return neterror.ParamsInvalidError("fabao draw times(%d) err", req.Times)
	}

	conf, err := s.GetConf()
	if nil != err {
		return err
	}

	data := s.GetData()
	success, useDiamondCount := s.consumeByTimes(conf, req.Times, req.AutoBuy)
	if !success {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.addScore(int64(conf.GiveScore*req.Times), pb3.LogId_LogFaBaoDrawAward, true)

	bundledAwards := jsondata.StdRewardMulti(conf.GiveRewards, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFaBaoDrawAward})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	if len(result.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
			LogId:        pb3.LogId_LogFaBaoDrawAward,
			NoTips:       true,
			BroadcastExt: []interface{}{s.Id},
		})
	}

	switch req.Times {
	case oneFaBaoDraw:
		s.GetPlayer().TriggerQuestEvent(custom_id.QttFaBaoDrawAnyOneFaBaoDraw, s.Id, 1)
	case tenFaBaoDraw:
		s.GetPlayer().TriggerQuestEvent(custom_id.QttFaBaoDrawAnyTenFaBaoDraw, s.Id, 1)
	}
	s.GetPlayer().TriggerQuestEvent(custom_id.QttFaBaoDrawDrawTimes, s.Id, int64(req.Times))
	s.GetPlayer().TriggerQuestEvent(custom_id.QttFaoBaoDrawTime, s.Id, int64(req.Times))

	if len(conf.ReachScore) >= 2 {
		s.GetPlayer().TriggerEvent(custom_id.AeGetDrawReachScore, conf.ReachScore[0], conf.ReachScore[1]*req.Times)
	}

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActFaBaoDraw,
		ActId:   s.Id,
		Times:   req.Times,
	})

	rsp := &pb3.S2C_47_1{
		ActiveId:          s.Id,
		Times:             data.LotteryData.Times,
		Lucky:             data.LotteryData.StageCount,
		LibHit:            data.FaBaoDraw.LibHit,
		PlayerLuckyLibId:  s.lottery.GetLibId(conf.SmallLibId),
		PlayerLucky:       s.lottery.GetGuaranteeCount(conf.SmallLibId),
		DestinyLuckyLibId: s.lottery.GetLibId(conf.SuperLibId),
		DestinyLucky:      s.lottery.GetGuaranteeCount(conf.SuperLibId),
	}

	for _, v := range result.LibResult {
		st := &pb3.FaBaoDrawSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		}

		if oneAwards := engine.FilterRewardByPlayer(s.GetPlayer(), v.OneAwards); len(oneAwards) > 0 {
			st.ItemId = oneAwards[0].Id
			st.Count = uint32(oneAwards[0].Count)
		}

		rsp.Result = append(rsp.Result, st)

		if data.FaBaoDraw.ItemCount[st.ItemId] == 0 && pie.Uint32s(conf.HitItems).Contains(st.ItemId) {
			data.FaBaoDraw.ItemCount[st.ItemId]++
			s.GetPlayer().TriggerEvent(custom_id.AeReachStandardQuest, &custom_id.ActReachStandardEvent{
				ReachType: reachconddef.ReachStandard_CountProgress,
				Key:       s.Id,
				Val:       st.ItemId,
			})
		}
	}

	rsp.ItemCount = data.FaBaoDraw.ItemCount

	s.SendProto3(47, 1, rsp)
	return nil
}

func (s *FaBaoDrawSys) c2sSumDrawAward(msg *base.Message) error {
	var req pb3.C2S_47_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf || nil == conf.SumDraw {
		return neterror.ConfNotFoundError("fabao draw conf not exist")
	}

	data := s.GetData()
	drawTimes := data.LotteryData.Times

	var ids []uint32
	var rewardVec []jsondata.StdRewardVec
	for id, v := range conf.SumDraw {
		if v.DrawTimes > drawTimes {
			continue
		}
		if data.FaBaoDraw.SumAward[id] {
			continue
		}
		ids = append(ids, id)
		rewardVec = append(rewardVec, v.Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardVec...)
	if len(rewards) <= 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}
	if ok := engine.CheckRewards(s.GetPlayer(), rewards); !ok { //背包空间不足
		s.GetPlayer().SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	for _, id := range ids {
		data.FaBaoDraw.SumAward[id] = true
		if conf.SumDraw[id].Score > 0 {
			s.addScore(int64(conf.SumDraw[id].Score), pb3.LogId_LogFaBaoDrawSumAward, true)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFaBaoDrawSumAward, BroadcastExt: []interface{}{s.Id}})
	}

	s.SendProto3(47, 3, &pb3.S2C_47_3{
		ActiveId: s.Id,
		Ids:      ids,
	})
	return nil
}

func (s *FaBaoDrawSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_47_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("fabao draw conf not exist")
	}
	rsp := &pb3.S2C_47_5{ActiveId: s.Id, Type: req.GetType()}
	switch req.Type {
	case fabaoDrawRecordGlobalType:
		rsp.Record = s.getGlobalRecord()
	case fabaoDrawRecordPersonalType:
		rsp.Record = s.GetData().FaBaoDraw.Record
	}
	s.SendProto3(47, 5, rsp)
	return nil
}

func (s *FaBaoDrawSys) GetConf() (*jsondata.FaBaoDrawConf, error) {
	conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil, neterror.ConfNotFoundError("fabao draw conf not exist")
	}
	return conf, nil
}

func (s *FaBaoDrawSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_47_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, err := s.GetConf()
	if nil != err {
		return err
	}

	exConf := conf.GetExchangeConfById(req.Id)
	if nil == exConf {
		return neterror.ConfNotFoundError("fabao draw exchangeConf not exist")
	}

	data := s.GetData().FaBaoDraw

	if int64(conf.ExLayerCos[exConf.Layer-1]) > data.Cost {
		return neterror.ParamsInvalidError("cant buy gift which not open")
	}
	if data.Exchange[req.GetId()] >= exConf.ExchangeTimes {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}
	if data.Score < int64(exConf.NeedMoney) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	oldScore := data.Score
	data.Score -= int64(exConf.NeedMoney)
	s.logScoreChange(oldScore, data.Score, pb3.LogId_LogFaBaoExchange)
	s.SendProto3(47, 10, &pb3.S2C_47_10{ActiveId: s.GetId(), Score: data.Score})
	data.Cost += int64(exConf.NeedMoney)
	s.SendProto3(47, 9, &pb3.S2C_47_9{ActiveId: s.GetId(), Cost: data.Cost})
	data.Exchange[req.GetId()]++
	if len(exConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), exConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFaBaoExchange, BroadcastExt: []interface{}{s.Id}})
	}
	s.SendProto3(47, 7, &pb3.S2C_47_7{ActiveId: s.GetId(), Id: req.GetId(), Times: data.Exchange[req.GetId()]})
	return nil
}

func (s *FaBaoDrawSys) c2sExchangeRemind(msg *base.Message) error {
	var req pb3.C2S_47_8
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, err := s.GetConf()
	if nil != err {
		return err
	}

	if nil == conf.GetExchangeConfById(req.GetId()) {
		return neterror.ConfNotFoundError("exchangeConf nil")
	}

	data := s.GetData().FaBaoDraw
	if req.GetNeed() {
		if !utils.SliceContainsUint32(data.ExchangeRemind, req.GetId()) {
			data.ExchangeRemind = append(data.ExchangeRemind, req.GetId())
		}
	} else {
		for pos, v := range data.ExchangeRemind {
			if v == req.GetId() {
				data.ExchangeRemind = append(data.ExchangeRemind[:pos], data.ExchangeRemind[pos+1:]...)
				break
			}
		}
	}

	s.SendProto3(47, 8, &pb3.S2C_47_8{
		ActiveId: s.Id,
		Id:       req.GetId(),
		Need:     req.GetNeed(),
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYFaBaoDraw, func() iface.IPlayerYY {
		return &FaBaoDrawSys{}
	})

	net.RegisterYYSysProtoV2(47, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FaBaoDrawSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(47, 3, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FaBaoDrawSys).c2sSumDrawAward
	})

	net.RegisterYYSysProtoV2(47, 5, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FaBaoDrawSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(47, 7, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FaBaoDrawSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(47, 8, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FaBaoDrawSys).c2sExchangeRemind
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.FabaoDrawTips, engine.CommonYYDrawBroadcast)

	engine.RegRewardsBroadcastHandler(tipmsgid.FabaoDrawTips2, engine.CommonYYDrawBroadcast)

	engine.RegRewardsBroadcastHandler(tipmsgid.FireworksCelebrationTips, engine.CommonYYDrawBroadcast)

	engine.RegRewardsBroadcastHandler(tipmsgid.QiXiDrawTips, engine.CommonYYDrawBroadcast)

	gmevent.Register("fabaodraw", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		totalTimes := utils.AtoInt(args[1])
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}
		obj := sys.GetObjById(yyId)
		if nil == obj || obj.GetClass() != yydefine.YYFaBaoDraw {
			return false
		}
		s := obj.(*FaBaoDrawSys)
		conf := jsondata.GetYYFaBaoDrawConf(s.ConfName, s.ConfIdx)
		if conf == nil {
			return false
		}
		result := s.lottery.DoDraw(uint32(totalTimes), uint32(totalTimes), conf.LibIds)
		if len(result.Awards) > 0 {
			engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
				LogId:        pb3.LogId_LogFaBaoDrawAward,
				NoTips:       true,
				BroadcastExt: []interface{}{s.Id},
			})
		}

		fp, err := os.Create(utils.GetCurrentDir() + fmt.Sprintf("法宝抽奖-%d.csv", time_util.NowSec()))
		defer fp.Close()
		if nil != err {
			s.LogError("err:%v", err)
			return false
		}

		writer := csv.NewWriter(fp)
		var header = []string{"awardID"}
		writer.Write(header)
		for _, v := range result.LibResult {
			writer.Write([]string{utils.I32toa(v.LibId)})
		}

		writer.Flush()
		if err = writer.Error(); err != nil {
			s.LogError("err:%v", err)
			return false
		}
		return true
	}, 1)
}
