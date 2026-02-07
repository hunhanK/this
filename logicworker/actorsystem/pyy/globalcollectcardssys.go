/**
 * @Author: zjj
 * @Date: 2024/8/5
 * @Desc: 全民集卡
**/

package pyy

import (
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

type GlobalCollectCardsSys struct {
	*YYQuestTargetBase
}

func (s *GlobalCollectCardsSys) getData() *pb3.PYYGlobalCollectCards {
	state := s.GetYYData()
	if nil == state.GlobalCollectCardsMap {
		state.GlobalCollectCardsMap = make(map[uint32]*pb3.PYYGlobalCollectCards)
	}
	if nil == state.GlobalCollectCardsMap[s.Id] {
		state.GlobalCollectCardsMap[s.Id] = &pb3.PYYGlobalCollectCards{}
	}
	data := state.GlobalCollectCardsMap[s.Id]
	if data.Day == 0 {
		data.Day = s.GetOpenDay()
	}
	if data.QuestData == nil {
		data.QuestData = make(map[uint32]*pb3.PYYGlobalCollectCardsQuest)
	}
	if data.ExchangeMap == nil {
		data.ExchangeMap = make(map[uint32]uint32)
	}
	return data
}

func (s *GlobalCollectCardsSys) getDailyQuestData(day uint32) *pb3.PYYGlobalCollectCardsQuest {
	data := s.getData()
	if day == 0 {
		day = data.Day
	}
	questData, ok := data.QuestData[day]
	if !ok {
		data.QuestData[day] = &pb3.PYYGlobalCollectCardsQuest{}
		questData = data.QuestData[day]
	}
	if questData.QuestMap == nil {
		questData.QuestMap = make(map[uint32]*pb3.QuestData)
	}
	return questData
}
func (s *GlobalCollectCardsSys) resetDailyQuestData(day uint32) {
	data := s.getData()
	if day == 0 {
		day = data.Day
	}
	data.QuestData[day] = &pb3.PYYGlobalCollectCardsQuest{}
	return
}

func (s *GlobalCollectCardsSys) getQuest(questId uint32) *pb3.QuestData {
	questData := s.getDailyQuestData(0)
	data, ok := questData.QuestMap[questId]
	if !ok {
		return nil
	}
	return data
}

func (s *GlobalCollectCardsSys) s2cInfo() {
	s.GetPlayer().SendProto3(61, 50, &pb3.S2C_61_50{
		ActiveId: s.GetId(),
		Data:     s.getData(),
		CardData: s.getCardData(0),
	})
}

func (s *GlobalCollectCardsSys) ResetData() {
	s.clearCardData()
	state := s.GetYYData()
	if nil == state.GlobalCollectCardsMap {
		return
	}
	delete(state.GlobalCollectCardsMap, s.Id)
}

func (s *GlobalCollectCardsSys) initQuestData() {
	data := s.getData()
	questData := s.getDailyQuestData(0)
	if len(questData.QuestMap) != 0 {
		return
	}

	dailyQuestConf, ok := jsondata.GetGlobalCollectDailyQuestConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	// 初始化今天的任务
	questConf, ok := dailyQuestConf[data.Day]
	if !ok {
		return
	}
	for _, qConf := range questConf.QuestMap {
		info := &pb3.QuestData{
			Id: qConf.Id,
		}
		questData.QuestMap[qConf.Id] = info
		s.OnAcceptQuestAndCheckUpdateTarget(info)
	}
}

func (s *GlobalCollectCardsSys) OnOpen() {
	s.s2cInfo()
	s.initQuestData()
}

func (s *GlobalCollectCardsSys) Login() {
	s.reissueLogWorker()
}

func (s *GlobalCollectCardsSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *GlobalCollectCardsSys) OnEnd() {
	s.reissueProcessAwards()
}

func (s *GlobalCollectCardsSys) OnReconnect() {
	s.s2cInfo()
}

// 先补发昨天未领取的任务奖励
func (s *GlobalCollectCardsSys) reissueQuestAwards() {
	day := s.getData().Day
	dailyQuestData := s.getDailyQuestData(day)
	if len(dailyQuestData.FinishIds) == len(dailyQuestData.ReceiveIds) {
		return
	}

	var receiveIdSet = make(map[uint32]struct{})
	for _, id := range dailyQuestData.ReceiveIds {
		receiveIdSet[id] = struct{}{}
	}

	var awards jsondata.StdRewardVec
	for _, id := range dailyQuestData.FinishIds {
		if _, ok := receiveIdSet[id]; ok {
			continue
		}
		conf := s.getQuestConf(day, id)
		if conf == nil {
			continue
		}
		awards = append(awards, conf.Awards...)
		dailyQuestData.ReceiveIds = append(dailyQuestData.ReceiveIds, id)
	}

	// 补发奖励
	if len(awards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(common.Mail_GlobalCollectCardsReissueQuestAwards),
			Rewards: awards,
		})
	}
}

func (s *GlobalCollectCardsSys) onNewDay() {
	// 先补发昨天的奖励
	s.reissueQuestAwards()
	// 更新天数
	s.getData().Day = s.GetOpenDay()
	// 更新赠送次数
	s.getCardData(s.GetPlayer().GetId()).TodayGiftCount = 0
	// 重新接任务
	s.initQuestData()
	s.s2cInfo()
}

func (s *GlobalCollectCardsSys) getDailyQuestConfMap(day uint32) map[uint32]*jsondata.PYYGlobalCollectCardQuestConf {
	conf, ok := jsondata.GetGlobalCollectDailyQuestConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	if day == 0 {
		day = s.getData().Day
	}
	questConf := conf[day]
	if questConf == nil {
		return nil
	}
	return questConf.QuestMap
}

func (s *GlobalCollectCardsSys) getQuestConf(day, id uint32) *jsondata.PYYGlobalCollectCardQuestConf {
	dailyQuestMap := s.getDailyQuestConfMap(day)
	if dailyQuestMap == nil {
		return nil
	}
	cardQuestConf := dailyQuestMap[id]
	if cardQuestConf == nil {
		return nil
	}
	return cardQuestConf
}

func (s *GlobalCollectCardsSys) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	questConf := s.getQuestConf(0, id)
	if questConf == nil {
		return nil
	}
	return questConf.Targets
}

func (s *GlobalCollectCardsSys) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	questMap := s.getDailyQuestConfMap(0)
	if questMap == nil {
		return nil
	}
	var idSet = make(map[uint32]struct{})
	for _, conf := range questMap {
		for _, target := range conf.Targets {
			if target.Type != qtt {
				continue
			}
			idSet[conf.Id] = struct{}{}
			break
		}
	}
	return idSet
}

func (s *GlobalCollectCardsSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getDailyQuestData(0)
	questData, ok := data.QuestMap[id]
	if !ok {
		return nil
	}
	if s.CheckFinishQuest(questData) {
		return nil
	}
	return questData
}

func (s *GlobalCollectCardsSys) onUpdateTargetData(id uint32) {
	data := s.getData()
	questData := s.getDailyQuestData(0)
	quest := questData.QuestMap[id]
	if !pie.Uint32s(questData.FinishIds).Contains(id) && s.CheckFinishQuest(quest) {
		questData.FinishIds = append(questData.FinishIds, id)
	}
	//下发任务进度任务
	s.SendProto3(61, 52, &pb3.S2C_61_52{Quest: quest, Day: data.Day, ActiveId: s.Id})
}

func (s *GlobalCollectCardsSys) addPoint(point uint32, logId pb3.LogId) {
	oldPoint := s.getData().TotalPoint
	newPoint := oldPoint + point
	s.getData().TotalPoint = newPoint
	s.SendProto3(61, 53, &pb3.S2C_61_53{
		ActiveId:   s.Id,
		TotalPoint: s.getData().TotalPoint,
	})
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeGlobalCollectCards, s.GetPlayer(), int64(s.getData().TotalPoint), false, 0)
	logworker.LogPlayerBehavior(s.GetPlayer(), logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d", point, oldPoint, newPoint),
	})
}

func (s *GlobalCollectCardsSys) c2sProcessAwards(msg *base.Message) error {
	var req pb3.C2S_61_61
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	data := s.getData()
	owner := s.GetPlayer()
	process := req.Process
	if pie.Uint32s(data.RecProcesses).Contains(process) {
		owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf, ok := jsondata.GetGlobalCollectProcessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	processConf, ok := conf[process]
	if !ok {
		return neterror.ConfNotFoundError("%s %d  not found conf", s.GetPrefix(), process)
	}

	if data.TotalPoint < process {
		return neterror.ParamsInvalidError("%s %d  > %d", s.GetPrefix(), process, data.TotalPoint)
	}

	data.RecProcesses = append(data.RecProcesses, process)

	if len(processConf.Awards) > 0 {
		engine.GiveRewards(owner, processConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYGlobalCollectCardsProcessAwards,
		})
	}

	owner.SendProto3(61, 61, &pb3.S2C_61_61{
		ActiveId: s.GetId(),
		Process:  process,
	})

	engine.BroadcastTipMsgById(tipmsgid.PYYGlobalCollectCardsRecProcessAwardsBro, owner.GetId(), owner.GetName())
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsProcessAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", process),
	})

	return nil
}

func (s *GlobalCollectCardsSys) c2sDailyQuestAwards(msg *base.Message) error {
	var req pb3.C2S_61_62
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	questId := req.QuestId
	questConfMap := s.getDailyQuestConfMap(0)
	if questConfMap == nil {
		return neterror.ConfNotFoundError("%s not found quest conf", s.GetPrefix())
	}

	questConf, ok := questConfMap[questId]
	if !ok {
		return neterror.ConfNotFoundError("%s not found %d quest conf", s.GetPrefix(), questId)
	}

	quest := s.getQuest(questId)
	if quest == nil {
		return neterror.ParamsInvalidError("not found %d quest data", questId)
	}

	questData := s.getDailyQuestData(0)
	if pie.Uint32s(questData.ReceiveIds).Contains(questId) {
		return neterror.ParamsInvalidError("already received %d", questId)
	}

	if !pie.Uint32s(questData.FinishIds).Contains(questId) {
		return neterror.ParamsInvalidError("%d quest not finish", questId)
	}

	owner := s.GetPlayer()

	// 领奖
	questData.ReceiveIds = append(questData.ReceiveIds, questId)
	if len(questConf.Awards) > 0 {
		engine.GiveRewards(owner, questConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYGlobalCollectCardsQuestAwards,
		})
	}

	day := s.getData().Day
	owner.SendProto3(61, 62, &pb3.S2C_61_62{
		ActiveId: s.GetId(),
		QuestId:  questId,
		Day:      day,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsQuestAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", day, questId),
	})
	return nil
}

func (s *GlobalCollectCardsSys) c2sActiveCard(msg *base.Message) error {
	var req pb3.C2S_61_63
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	owner := s.GetPlayer()
	itemId := req.ItemId
	conf, ok := jsondata.GetGlobalCollectCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	cardConf, ok := conf[itemId]
	if !ok {
		return neterror.ParamsInvalidError("%s not found %d conf", s.GetPrefix(), itemId)
	}

	cardData := s.getCardData(owner.GetId())
	if count := cardData.CollectCardMap[itemId]; count < 1 {
		return neterror.ParamsInvalidError("%s not found %d data", s.GetPrefix(), itemId)
	}

	if _, ok := cardData.ActiveCollectCardMap[itemId]; ok {
		return neterror.ParamsInvalidError("%s %d already active", s.GetPrefix(), itemId)
	}

	cardData.ActiveCollectCardMap[itemId] = 1
	s.addPoint(cardConf.GainPoint, pb3.LogId_LogPYYGlobalCollectCardsAddPointByActiveCard)

	owner.SendProto3(61, 63, &pb3.S2C_61_63{
		ActiveId: s.GetId(),
		ItemId:   itemId,
	})

	s.afterCheckActiveCardSuit(itemId)

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsActiveCard, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", itemId),
	})
	return nil
}

func (s *GlobalCollectCardsSys) afterCheckActiveCardSuit(itemId uint32) {
	conf, ok := jsondata.GetGlobalCollectSuitConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var suitConf *jsondata.PYYGlobalCollectCardSuitConf
	for _, sConf := range conf {
		if !pie.Uint32s(sConf.ItemIds).Contains(itemId) {
			continue
		}
		suitConf = sConf
		break
	}
	if suitConf == nil {
		return
	}
	err := s.activeCardSuit(suitConf.SuitId)
	if err != nil {
		s.LogDebug("err:%v", err)
		return
	}
}

func (s *GlobalCollectCardsSys) activeCardSuit(suitId uint32) error {
	conf, ok := jsondata.GetGlobalCollectSuitConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found suit conf", s.GetPrefix())
	}
	data := s.getData()
	if pie.Uint32s(data.SuitIds).Contains(suitId) {
		return neterror.ParamsInvalidError("%d already active", suitId)
	}

	suitConf, ok := conf[suitId]
	if !ok {
		return neterror.ParamsInvalidError("%s %d not found suit conf", s.GetPrefix(), suitId)
	}

	owner := s.GetPlayer()
	cardData := s.getCardData(owner.GetId())
	for _, id := range suitConf.ItemIds {
		_, ok := cardData.ActiveCollectCardMap[id]
		if !ok {
			return neterror.ParamsInvalidError("%d not active", id)
		}
	}

	data.SuitIds = append(data.SuitIds, suitId)
	s.addPoint(conf[suitId].GainPoint, pb3.LogId_LogPYYGlobalCollectCardsAddPointByActiveSuit)

	owner.SendProto3(61, 64, &pb3.S2C_61_64{
		ActiveId: s.GetId(),
		SuitId:   suitId,
	})
	engine.BroadcastTipMsgById(tipmsgid.PYYGlobalCollectCardsHaveCardBro, owner.GetId(), owner.GetName(), suitConf.Name)
	owner.TriggerQuestEvent(custom_id.QttGlobalCollectCardsXSuit, 0, int64(len(data.SuitIds)))
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsActiveCard, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", suitId),
	})
	return nil
}

func (s *GlobalCollectCardsSys) c2sActiveCardSuit(msg *base.Message) error {
	var req pb3.C2S_61_64
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	err := s.activeCardSuit(req.SuitId)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	return nil
}

func (s *GlobalCollectCardsSys) c2sRecycle(msg *base.Message) error {
	var req pb3.C2S_61_65
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	cardConfMap, ok := jsondata.GetGlobalCollectCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	itemCountMap := req.ItemCountMap
	if len(itemCountMap) == 0 {
		return neterror.ParamsInvalidError("item count map is empty")
	}

	owner := s.GetPlayer()
	cardData := s.getCardData(owner.GetId())

	var realCanDeductMap = make(map[uint32]uint32)

	for itemId, consumeCount := range itemCountMap {
		if consumeCount == 0 {
			continue
		}

		haveCount := cardData.CollectCardMap[itemId]
		if haveCount < 2 || haveCount < consumeCount {
			continue
		}

		if haveCount == consumeCount {
			realCanDeductMap[itemId] = consumeCount - 1
			continue
		}

		realCanDeductMap[itemId] = consumeCount
	}

	if len(realCanDeductMap) == 0 {
		return neterror.ConsumeFailedError("not can consume card")
	}

	var needGiveAwards jsondata.StdRewardVec
	var rsp = pb3.S2C_61_65{
		ActiveId:     s.GetId(),
		ItemCountMap: make(map[uint32]uint32),
	}
	for itemId, count := range realCanDeductMap {
		cardConf := cardConfMap[itemId]
		if cardConf == nil {
			continue
		}

		realCount := s.subCard(itemId, count)
		rsp.ItemCountMap[itemId] = realCount

		vec := jsondata.StdRewardVecToPb3RewardVec(cardConf.Awards)
		for _, stdAward := range vec {
			stdAward.Count *= int64(realCount)
		}
		needGiveAwards = append(needGiveAwards, jsondata.Pb3RewardVecToStdRewardVec(vec)...)
	}

	if len(needGiveAwards) > 0 {
		engine.GiveRewards(owner, needGiveAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYGlobalCollectCardsRecycleCardGiveAwards,
		})
	}

	owner.SendProto3(61, 65, &rsp)

	val, _ := json.Marshal(&rsp)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsRecycleCardGiveAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%s", string(val)),
	})
	return nil
}

func (s *GlobalCollectCardsSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_61_66
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetGlobalCollectExchangeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	itemId := req.ItemId
	exchangeConf, ok := conf[itemId]
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix(), itemId)
	}

	data := s.getData()
	if exchangeConf.OpenDay != 0 && exchangeConf.OpenDay > data.Day {
		return neterror.ParamsInvalidError("%d %d day not reached", exchangeConf.OpenDay, data.Day)
	}

	count := data.ExchangeMap[itemId]
	if exchangeConf.Limit != 0 && count >= exchangeConf.Limit {
		return neterror.ParamsInvalidError("%d limit %d", itemId, exchangeConf.Limit)
	}

	owner := s.GetPlayer()
	if len(exchangeConf.Consume) != 0 && !owner.ConsumeByConf(exchangeConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYGlobalCollectCardsExchangeItem}) {
		return neterror.ConsumeFailedError("%d consume failed", itemId)
	}

	// 计数
	data.ExchangeMap[itemId] += 1

	// 下发奖励
	if len(exchangeConf.Awards) > 0 {
		engine.GiveRewards(owner, exchangeConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYGlobalCollectCardsExchangeItem})
	}

	owner.SendProto3(61, 66, &pb3.S2C_61_66{
		ActiveId: s.GetId(),
		ItemId:   itemId,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsExchangeItem, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", itemId),
	})

	return nil
}

// 补发进度奖励
func (s *GlobalCollectCardsSys) reissueProcessAwards() {
	data := s.getData()
	conf, ok := jsondata.GetGlobalCollectProcessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var awards jsondata.StdRewardVec
	for _, processConf := range conf {
		if pie.Uint32s(data.RecProcesses).Contains(processConf.Process) {
			continue
		}
		if processConf.Process != 0 && processConf.Process > data.TotalPoint {
			continue
		}
		data.RecProcesses = append(data.RecProcesses, processConf.Process)
		awards = append(awards, processConf.Awards...)
	}
	if len(awards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(common.Mail_GlobalCollectCardsReissueProcessAwards),
			Rewards: awards,
		})
	}
}

func (s *GlobalCollectCardsSys) GMReAcceptQuest(questId uint32) {
	quest := s.getQuest(questId)
	if quest == nil {
		s.GetPlayer().LogWarn("not found %d quest", questId)
		return
	}
	data := s.getDailyQuestData(0)
	if pie.Uint32s(data.ReceiveIds).Contains(questId) || pie.Uint32s(data.FinishIds).Contains(questId) {
		s.GmFinishQuest(quest)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(quest)
}

func (s *GlobalCollectCardsSys) GMDelQuest(questId uint32) {
	data := s.getDailyQuestData(0)
	delete(data.QuestMap, questId)
	data.FinishIds = pie.Uint32s(data.FinishIds).Filter(func(u uint32) bool {
		return u != questId
	})
	data.ReceiveIds = pie.Uint32s(data.ReceiveIds).Filter(func(u uint32) bool {
		return u != questId
	})
}

func createGlobalCollectCardsSys() iface.IPlayerYY {
	obj := &GlobalCollectCardsSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

// 一期只会有一个这个模板 有多个就会有问题
func getGlobalCollectCardsSys(player iface.IPlayer) (*GlobalCollectCardsSys, bool) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYGlobalCollectCards)
	if len(yyList) == 0 {
		return nil, false
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*GlobalCollectCardsSys)
		if !ok || !sys.IsOpen() {
			return nil, false
		}
		return sys, true
	}

	return nil, false
}

func handleGlobalCollectCardsAeNewDay(player iface.IPlayer, _ ...interface{}) {
	sys, ok := getGlobalCollectCardsSys(player)
	if !ok {
		return
	}
	sys.onNewDay()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYGlobalCollectCards, createGlobalCollectCardsSys)
	event.RegActorEvent(custom_id.AeNewDay, handleGlobalCollectCardsAeNewDay)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemByCollectCard, handleUseItemByCollectCard)

	net.RegisterYYSysProtoV2(61, 61, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sProcessAwards
	})
	net.RegisterYYSysProtoV2(61, 62, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sDailyQuestAwards
	})
	net.RegisterYYSysProtoV2(61, 63, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sActiveCard
	})
	net.RegisterYYSysProtoV2(61, 64, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sActiveCardSuit
	})
	net.RegisterYYSysProtoV2(61, 65, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sRecycle
	})
	net.RegisterYYSysProtoV2(61, 66, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sExchange
	})
	net.RegisterYYSysProtoV2(61, 67, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sAskHelp
	})
	net.RegisterYYSysProtoV2(61, 68, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sToHelp
	})
	net.RegisterYYSysProtoV2(61, 71, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GlobalCollectCardsSys).c2sGetAskRecord
	})
	manager.RegPlayScoreExtValueGetter(ranktype.PlayScoreRankTypeGlobalCollectCards, handlePlayScoreRankTypeGlobalCollectCards)
	initGlobalCollectCardsGm()
}

func handlePlayScoreRankTypeGlobalCollectCards(player iface.IPlayer) *pb3.YYFightValueRushRankExt {
	_, ok := getGlobalCollectCardsSys(player)
	if !ok {
		return nil
	}
	return &pb3.YYFightValueRushRankExt{
		GlobalCollectCard: &pb3.YYFightValueRushRankExtGlobalCollectCard{
			UpdateAt: time_util.NowSec(),
		},
	}
}

func initGlobalCollectCardsGm() {
	gmevent.Register("GlobalCollectCardsSys.completedTodayQuest", func(player iface.IPlayer, args ...string) bool {
		sys, ok := getGlobalCollectCardsSys(player)
		if !ok {
			return false
		}
		dailyQuestData := sys.getDailyQuestData(0)
		if dailyQuestData == nil {
			return false
		}
		for _, data := range dailyQuestData.QuestMap {
			sys.GmFinishQuest(data)
		}
		return true
	}, 0)
	gmevent.Register("GlobalCollectCardsSys.addCard", func(player iface.IPlayer, args ...string) bool {
		sys, ok := getGlobalCollectCardsSys(player)
		if !ok {
			return false
		}
		sys.addCard(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]), pb3.LogId_LogGm)
		return true
	}, 0)
	gmevent.Register("GlobalCollectCardsSys.addPoint", func(player iface.IPlayer, args ...string) bool {
		sys, ok := getGlobalCollectCardsSys(player)
		if !ok {
			return false
		}
		sys.addPoint(utils.AtoUint32(args[0]), pb3.LogId_LogGm)
		return true
	}, 0)
	gmevent.Register("GlobalCollectCardsSys.clearCard", func(player iface.IPlayer, args ...string) bool {
		sys, ok := getGlobalCollectCardsSys(player)
		if !ok {
			return false
		}
		data := sys.getCardData(0)
		data.CollectCardMap = make(map[uint32]uint32)
		data.ActiveCollectCardMap = make(map[uint32]uint32)
		sys.getData().SuitIds = []uint32{}
		sys.s2cInfo()
		return true
	}, 0)
}
