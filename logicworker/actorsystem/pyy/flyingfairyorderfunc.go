/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 飞仙令
**/

package pyy

import (
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

// 飞仙令

type FlyingFairyOrderFuncSys struct {
	*YYQuestTargetBase
}

func createYYFlyingFairyOrderFuncSys() iface.IPlayerYY {
	obj := &FlyingFairyOrderFuncSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *FlyingFairyOrderFuncSys) OnOpen() {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if conf.StoreSubType != 0 {
		s.GetPlayer().ResetSpecCycleBuy(custom_id.StoreTypeFlyingFairyOrderFunc, conf.StoreSubType)
	}

	s.initData() // 初始化赛季数据
	s.S2CInfo()
}

func (s *FlyingFairyOrderFuncSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	openDay := s.GetOpenDay()
	data := s.GetData()
	data.CurYyDay = openDay
	_, ok := data.DailyQuestMap[openDay]
	if !ok {
		data.DailyQuestMap[openDay] = s.acceptDailyQuestsConf(openDay) // 初始化日常任务
	}

	s.S2CInfo()
}

func (s *FlyingFairyOrderFuncSys) Login() {
	if !s.IsOpen() {
		return
	}

	openDay := s.GetOpenDay()
	data := s.GetData()
	data.CurYyDay = openDay
	_, ok := data.DailyQuestMap[openDay]
	if !ok {
		data.DailyQuestMap[openDay] = s.acceptDailyQuestsConf(openDay) // 初始化日常任务
	}

	s.S2CInfo()
}

func (s *FlyingFairyOrderFuncSys) OnEnd() {
	// 活动结束
	data := s.GetData()
	curLv := int64(data.GetCurLv())
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	// 检查下还有哪些奖励没领
	var rewards jsondata.StdRewardVec
	for _, level := range conf.Level {
		if level.Lv > curLv {
			continue
		}

		if !pie.Uint32s(data.NormalRecLvs).Contains(uint32(level.Lv)) {
			rewards = append(rewards, level.Awards...)
		}

		if !s.IsUnLockHeight() {
			continue
		}

		if !pie.Uint32s(data.HighRecLvs).Contains(uint32(level.Lv)) {
			rewards = append(rewards, level.HAwards...)
		}
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YyFlyingFairyOrderFunc,
			Rewards: rewards,
		})
	}
}

func (s *FlyingFairyOrderFuncSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetPlayer().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%s", coreNumData, string(bytes)),
	})
}

func (s *FlyingFairyOrderFuncSys) IsUnLockHeight() bool {
	return s.GetData().UnLockChargeId > 0
}

// 初始化数据
func (s *FlyingFairyOrderFuncSys) initData() {
	data := s.GetData()
	if data.DailyQuestMap == nil {
		data.DailyQuestMap = make(map[uint32]*pb3.YYFlyingFairyOrderFuncQuests)
	}
	if data.SeasonQuest == nil {
		data.SeasonQuest = new(pb3.YYFlyingFairyOrderFuncQuests)
	}
	openDay := s.GetOpenDay()
	data.CurYyDay = openDay

	// 初始化任务
	_, ok := data.DailyQuestMap[openDay]
	if !ok {
		data.DailyQuestMap[openDay] = s.acceptDailyQuestsConf(openDay)
	}
	if len(data.SeasonQuest.Quests) == 0 {
		data.SeasonQuest = s.acceptSeasonQuestConf()
	}
}

// 今日任务
func (s *FlyingFairyOrderFuncSys) acceptDailyQuestsConf(day uint32) *pb3.YYFlyingFairyOrderFuncQuests {
	var quests pb3.YYFlyingFairyOrderFuncQuests
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return &quests
	}
	for i := range conf.DailyTask {
		dailyTaskConf := conf.DailyTask[i]
		if dailyTaskConf.Day != int64(day) {
			continue
		}

		if dailyTaskConf.QuestConf == nil {
			break
		}

		for k := range dailyTaskConf.QuestConf {
			quests.Quests = append(quests.Quests, &pb3.QuestData{
				Id:       uint32(dailyTaskConf.QuestConf[k].Id),
				Progress: nil,
			})
		}
		break
	}

	// 接任务
	for i := range quests.Quests {
		questData := quests.Quests[i]
		s.OnAcceptQuestAndCheckUpdateTarget(questData)
	}

	return &quests
}

// 赛季任务
func (s *FlyingFairyOrderFuncSys) acceptSeasonQuestConf() *pb3.YYFlyingFairyOrderFuncQuests {
	var quests pb3.YYFlyingFairyOrderFuncQuests
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return &quests
	}

	if conf.SeasonQuest == nil {
		return &quests
	}

	for k := range conf.SeasonQuest {
		quests.Quests = append(quests.Quests, &pb3.QuestData{
			Id:       uint32(conf.SeasonQuest[k].Id),
			Progress: nil,
		})
	}

	// 接任务
	for i := range quests.Quests {
		questData := quests.Quests[i]
		s.OnAcceptQuestAndCheckUpdateTarget(questData)
	}

	return &quests
}

func (s *FlyingFairyOrderFuncSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	//下发任务进度任务
	s.SendProto3(145, 5, &pb3.S2C_145_5{Quest: quest, ActiveId: s.Id})
}

func (s *FlyingFairyOrderFuncSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.GetData()

	// 赛季任务已经完成
	if pie.Uint32s(data.SeasonQuest.FinishQuestIds).Contains(id) {
		return nil
	}
	for i := range data.SeasonQuest.Quests {
		if data.SeasonQuest.Quests[i].Id == id {
			s.GetPlayer().LogInfo("found season quest is %v", data.SeasonQuest.Quests[i])
			return data.SeasonQuest.Quests[i]
		}
	}

	// 日常任务 - 只拿今天的
	quests, ok := data.DailyQuestMap[s.GetOpenDay()]
	if !ok {
		return nil
	}

	if pie.Uint32s(quests.FinishQuestIds).Contains(id) {
		return nil
	}

	for i := range quests.Quests {
		if quests.Quests[i].Id == id {
			return quests.Quests[i]
		}
	}

	return nil
}

func (s *FlyingFairyOrderFuncSys) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	// 前置校验一下是不是空
	if conf.SeasonQuest != nil {
		questConf, ok := conf.SeasonQuest[fmt.Sprintf("%d", questId)]
		if ok {
			return questConf.Targets
		}
	}

	var todayQuests *jsondata.YyFlyingFairyOrderFuncDailyTask
	openDay := int64(s.GetOpenDay())
	for i := range conf.DailyTask {
		if conf.DailyTask[i].Day != openDay {
			continue
		}
		todayQuests = conf.DailyTask[i]
		break
	}

	if todayQuests.QuestConf == nil {
		return nil
	}

	questConf, ok := todayQuests.QuestConf[fmt.Sprintf("%d", questId)]
	if ok {
		return questConf.Targets
	}
	return nil
}

func (s *FlyingFairyOrderFuncSys) getQuestIdSetFunc(qt uint32) map[uint32]struct{} {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	set := make(map[uint32]struct{})

	if conf.SeasonQuest != nil {
		for _, v := range conf.SeasonQuest {
			for _, target := range v.Targets {
				if target.Type == qt {
					set[uint32(v.Id)] = struct{}{}
				}
			}
		}
	}

	// 只拿今日任务的ID 非今日会过滤
	var todayQuestConf *jsondata.YyFlyingFairyOrderFuncDailyTask
	openDay := int64(s.GetOpenDay())
	for i := range conf.DailyTask {
		taskConf := conf.DailyTask[i]

		if taskConf.Day != openDay {
			continue
		}
		todayQuestConf = taskConf
		break
	}

	if todayQuestConf == nil {
		return set
	}

	if todayQuestConf.QuestConf == nil {
		return set
	}

	for k := range todayQuestConf.QuestConf {
		v := todayQuestConf.QuestConf[k]
		for _, target := range v.Targets {
			if target.Type != qt {
				continue
			}
			set[uint32(v.Id)] = struct{}{}
		}
	}
	return set
}

func (s *FlyingFairyOrderFuncSys) GetData() *pb3.YYFlyingFairyOrderFunc {
	yyData := s.GetYYData()
	if nil == yyData.FlyingFairyOrderFuncDataMap {
		yyData.FlyingFairyOrderFuncDataMap = make(map[uint32]*pb3.YYFlyingFairyOrderFunc)
	}
	if nil == yyData.FlyingFairyOrderFuncDataMap[s.Id] {
		yyData.FlyingFairyOrderFuncDataMap[s.Id] = &pb3.YYFlyingFairyOrderFunc{}
	}
	orderFunc := yyData.FlyingFairyOrderFuncDataMap[s.Id]
	if orderFunc.DailyQuestMap == nil {
		orderFunc.DailyQuestMap = make(map[uint32]*pb3.YYFlyingFairyOrderFuncQuests)
	}
	if orderFunc.SeasonQuest == nil {
		orderFunc.SeasonQuest = &pb3.YYFlyingFairyOrderFuncQuests{}
	}
	return orderFunc
}

func (s *FlyingFairyOrderFuncSys) S2CInfo() {
	s.SendProto3(145, 1, &pb3.S2C_145_1{
		ActiveId: s.Id,
		State:    s.GetData(),
	})
}

// NewDay 跨天 每天0点解锁第二天的每日任务并结束当天的每日任务
func (s *FlyingFairyOrderFuncSys) NewDay() {
	if !s.IsOpen() {
		return
	}
	openDay := s.GetOpenDay()
	data := s.GetData()
	data.DailyQuestMap[openDay] = s.acceptDailyQuestsConf(openDay) // 跨天初始化日常任务
	data.CurYyDay = openDay                                        // 初始一下活动天

	s.S2CInfo()
}

// 领取奖励
func (s *FlyingFairyOrderFuncSys) c2sReceiveReward(msg *base.Message) error {
	var req pb3.C2S_145_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}
	iPlayer := s.GetPlayer()
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found yy flying fairy order func conf", s.GetPrefix())
	}
	data := s.GetData()

	// 等级
	if data.CurLv < req.Lv {
		iPlayer.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	// 历史领取
	var historyRecLvs = data.NormalRecLvs
	if req.IsH {
		historyRecLvs = data.HighRecLvs
	}
	if pie.Uint32s(historyRecLvs).Contains(req.Lv) {
		iPlayer.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	var level *jsondata.YyFlyingFairyOrderFuncLevel
	for i := range conf.Level {
		if conf.Level[i].Lv != int64(req.Lv) {
			continue
		}
		level = conf.Level[i]
		break
	}
	if level == nil {
		return neterror.ConfNotFoundError("%s not found yy flying fairy order func level conf, req lv is %v", s.GetPrefix(), req.Lv)
	}
	s.LogPlayerBehavior(uint64(level.Lv), map[string]interface{}{
		"isHigh": req.IsH,
	}, pb3.LogId_LogFlyingFairyOrderFuncReceiveReward)

	// 领取该等级的奖励
	var awards = level.Awards
	if req.IsH {
		awards = level.HAwards
	}

	// 记录领取等级
	if req.IsH {
		data.HighRecLvs = append(data.HighRecLvs, uint32(level.Lv))
	} else {
		data.NormalRecLvs = append(data.NormalRecLvs, uint32(level.Lv))
	}

	// 下发奖励
	engine.GiveRewards(iPlayer, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogChargeFlyingFairyOrderRecWards})

	// 下发一键领取
	s.SendProto3(145, 2, &pb3.S2C_145_2{
		ActiveId:     s.Id,
		NormalRecLvs: data.NormalRecLvs,
		HighRecLvs:   data.HighRecLvs,
	})
	return nil
}

// 一键领取奖励
func (s *FlyingFairyOrderFuncSys) c2sReceiveAllReward(msg *base.Message) error {
	var req pb3.C2S_145_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found yy flying fairy order func conf", s.GetPrefix())
	}
	data := s.GetData()

	var rewards jsondata.StdRewardVec
	var normalRecLvs, highRecLvs []uint32
	for _, level := range conf.Level {
		if level.Lv > int64(data.CurLv) {
			continue
		}

		if !pie.Uint32s(data.NormalRecLvs).Contains(uint32(level.Lv)) {
			rewards = append(rewards, level.Awards...)
			normalRecLvs = append(normalRecLvs, uint32(level.Lv))
		}

		if !s.IsUnLockHeight() {
			continue
		}

		if !pie.Uint32s(data.HighRecLvs).Contains(uint32(level.Lv)) {
			rewards = append(rewards, level.HAwards...)
			highRecLvs = append(highRecLvs, uint32(level.Lv))
		}
	}

	if len(normalRecLvs) > 0 {
		// 下发奖励
		data.NormalRecLvs = append(data.NormalRecLvs, normalRecLvs...)
	}
	if len(highRecLvs) > 0 {
		data.HighRecLvs = append(data.HighRecLvs, highRecLvs...)
	}

	s.LogPlayerBehavior(uint64(data.CurLv), map[string]interface{}{
		"normalRecLvs": normalRecLvs,
		"highRecLvs":   highRecLvs,
	}, pb3.LogId_LogFlyingFairyOrderFuncReceiveAllReward)

	// 下发奖励
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogChargeFlyingFairyOrderRecWards})
	}

	// 下发领取成功奖励
	s.SendProto3(145, 2, &pb3.S2C_145_2{
		ActiveId:     s.Id,
		NormalRecLvs: data.NormalRecLvs,
		HighRecLvs:   data.HighRecLvs,
	})

	return nil
}

// 等级购买
func (s *FlyingFairyOrderFuncSys) levelBuy(msg *base.Message) error {
	var req pb3.C2S_145_3
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found yy flying fairy order func conf", s.GetPrefix())
	}
	data := s.GetData()
	iPlayer := s.GetPlayer()
	// 等级是否溢出
	if req.TargetLv < data.CurLv {
		iPlayer.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}
	if req.TargetLv > uint32(len(conf.Level)) {
		iPlayer.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	// 开始拿材料
	var consumes jsondata.ConsumeVec
	for i := range conf.LevelBuy {
		levelBuy := conf.LevelBuy[i]
		if data.CurLv < uint32(levelBuy.Lv) && uint32(levelBuy.Lv) <= req.TargetLv {
			consumes = append(consumes, levelBuy.Consume...)
		}
	}

	// 消费物品升级
	if len(consumes) > 0 {
		if !iPlayer.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogChargeFlyingFairyOrderBuyLevel}) {
			iPlayer.SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ConsumeFailedError("%s Consume Failed", s.GetPrefix())
		}
	}

	s.LogPlayerBehavior(uint64(req.TargetLv), map[string]interface{}{}, pb3.LogId_LogFlyingFairyOrderToBuyLv)

	if req.TargetLv == uint32(len(conf.Level)) {
		data.CurExp = 0
	}
	data.CurLv = req.TargetLv

	s.SendProto3(145, 3, &pb3.S2C_145_3{
		ActiveId: s.Id,
		TargetLv: req.TargetLv,
		CurExp:   data.CurExp,
	})
	return nil
}

func (s *FlyingFairyOrderFuncSys) completeQuest(ids ...uint32) []uint32 {
	var oks []uint32
	seasonQuest := s.GetData().SeasonQuest
	dailyQuest := s.GetData().DailyQuestMap[s.GetOpenDay()]
	for i := range ids {
		if seasonQuest != nil {
			if pie.Uint32s(seasonQuest.FinishQuestIds).Contains(ids[i]) {
				continue
			}
			for qi := range seasonQuest.Quests {
				if seasonQuest.Quests[qi].Id == ids[i] {
					oks = append(oks, ids[i])
					seasonQuest.FinishQuestIds = append(seasonQuest.FinishQuestIds, ids[i])
					continue
				}
			}
		}

		if dailyQuest != nil {
			if pie.Uint32s(dailyQuest.FinishQuestIds).Contains(ids[i]) {
				continue
			}
			for qi := range dailyQuest.Quests {
				if dailyQuest.Quests[qi].Id == ids[i] {
					oks = append(oks, ids[i])
					dailyQuest.FinishQuestIds = append(dailyQuest.FinishQuestIds, ids[i])
					continue
				}
			}
		}

	}
	return oks
}

// 领取任务奖励
func (s *FlyingFairyOrderFuncSys) c2sRewardQuest(msg *base.Message) error {
	var req pb3.C2S_145_4
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	quest := s.getUnFinishQuestData(req.QuestId)
	if quest == nil {
		return neterror.ConfNotFoundError("%s get un finish quest ,not found yy flying fairy order func quest conf,quest id is %d", s.GetPrefix(), req.QuestId)
	}

	if !s.CheckFinishQuest(quest) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	questConf, ok := s.getQuestConf(req.QuestId)
	if !ok {
		return neterror.ConfNotFoundError("%s get quest conf fail, not found yy flying fairy order func quest conf,quest id is %d", s.GetPrefix(), req.QuestId)
	}

	s.LogPlayerBehavior(uint64(req.QuestId), map[string]interface{}{}, pb3.LogId_LogFlyingFairyOrderFuncRewardQuest)

	// 完成任务
	completeQuest := s.completeQuest(req.QuestId)

	// 加经验
	s.addExp(questConf.Exp)

	// 加等级
	s.addLevel()

	s.SendProto3(145, 4, &pb3.S2C_145_4{
		ActiveId:       s.Id,
		FinishQuestIds: completeQuest,
	})
	return nil
}

// 一键领取任务奖励
func (s *FlyingFairyOrderFuncSys) c2sAllRewardQuest(msg *base.Message) error {
	var req pb3.C2S_145_5
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	var totalExp int64
	var finishQuestIds []uint32
	if s.GetData() != nil && s.GetData().SeasonQuest != nil {
		seasonQuest := s.GetData().SeasonQuest
		for i := range seasonQuest.Quests {
			quest := seasonQuest.Quests[i]
			if pie.Uint32s(seasonQuest.FinishQuestIds).Contains(quest.Id) {
				continue
			}
			if !s.CheckFinishQuest(quest) {
				continue
			}

			questConf, ok := s.getQuestConf(quest.Id)
			if !ok {
				continue
			}
			finishQuestIds = append(finishQuestIds, quest.Id)
			totalExp += questConf.Exp
		}
	}

	dailyQuest, ok := s.GetData().DailyQuestMap[s.GetOpenDay()]
	if ok {
		for i := range dailyQuest.Quests {
			quest := dailyQuest.Quests[i]
			if pie.Uint32s(dailyQuest.FinishQuestIds).Contains(quest.Id) {
				continue
			}
			if !s.CheckFinishQuest(quest) {
				continue
			}
			questConf, ok := s.getQuestConf(quest.Id)
			if !ok {
				continue
			}
			finishQuestIds = append(finishQuestIds, quest.Id)
			totalExp += questConf.Exp
		}
	}

	if len(finishQuestIds) == 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	s.LogPlayerBehavior(0, map[string]interface{}{
		"finishQuestIds": finishQuestIds,
	}, pb3.LogId_LogFlyingFairyOrderFuncAllRewardQuest)

	// 加经验
	s.addExp(totalExp)

	// 加等级
	s.addLevel()

	// 完成任务
	completeQuest := s.completeQuest(finishQuestIds...)

	s.SendProto3(145, 4, &pb3.S2C_145_4{
		ActiveId:       s.Id,
		FinishQuestIds: completeQuest,
	})

	return nil
}

// 检查是否可以升级
func (s *FlyingFairyOrderFuncSys) checkCanUpLevel() bool {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false
	}
	data := s.GetData()

	// 顶级不用升级了
	if data.CurLv == uint32(len(conf.Level)) {
		return false
	}

	for i := range conf.Level {
		if conf.Level[i].Lv == int64(data.CurLv+1) {
			return conf.Level[i].Exp <= int64(data.CurExp)
		}
	}
	return false
}

// 加等级
func (s *FlyingFairyOrderFuncSys) addLevel() bool {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false
	}
	data := s.GetData()
	for i := 0; i < len(conf.Level); i++ {
		if !s.checkCanUpLevel() {
			break
		}
		var level *jsondata.YyFlyingFairyOrderFuncLevel
		for i := range conf.Level {
			if conf.Level[i].Lv == int64(data.CurLv+1) {
				level = conf.Level[i]
			}
		}

		data.CurLv = uint32(level.Lv)
		data.CurExp = data.CurExp - uint32(level.Exp) // 更新一下经验
	}
	// 下发最新的等级 经验信息
	s.SendProto3(145, 7, &pb3.S2C_145_7{ActiveId: s.Id, NewLevel: data.CurLv, NewExp: data.CurExp})
	s.LogPlayerBehavior(uint64(data.CurLv), map[string]interface{}{}, pb3.LogId_LogFlyingFairyOrderToLv)
	return true
}

// 加经验
func (s *FlyingFairyOrderFuncSys) addExp(exp int64) int64 {
	addRate := s.GetPlayer().GetFightAttr(attrdef.FlyingFairyOrderFuncAllExpAddRate)
	exp = exp * (10000 + addRate)
	exp = exp / 10000
	s.GetData().CurExp += uint32(exp)
	return exp
}

// 充值奖励发放
func (s *FlyingFairyOrderFuncSys) charge(chargeId uint32) error {
	data := s.GetData()
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	var find bool
	var awards jsondata.StdRewardVec
	for i := range conf.Charge {
		if find {
			break
		}
		charge := conf.Charge[i]
		if charge.ChargeId == int64(chargeId) {
			find = true
			awards = charge.TitleAwards
		}
	}

	if !find {
		return neterror.ParamsInvalidError("not found %d charge id %d", chargeId)
	}

	if len(awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogChargeFlyingFairyOrderFuncHigh})
	}

	data.UnLockChargeId = chargeId
	// 告诉前端充值成功
	s.SendProto3(145, 6, &pb3.S2C_145_6{
		ActiveId:       s.Id,
		CurLevel:       data.CurLv,
		UnLockChargeId: data.UnLockChargeId,
	})
	s.LogPlayerBehavior(uint64(chargeId), map[string]interface{}{}, pb3.LogId_LogFlyingFairyOrderFuncUnLockChargeId)

	return nil
}

// 拿到今日的日常任务
func (s *FlyingFairyOrderFuncSys) getToDayQuests() (*pb3.YYFlyingFairyOrderFuncQuests, bool) {
	data := s.GetData()
	quests, ok := data.DailyQuestMap[s.GetOpenDay()]
	return quests, ok
}

// 拿到任务
func (s *FlyingFairyOrderFuncSys) getQuest(id uint32) (*pb3.QuestData, bool) {
	data := s.GetData()

	// 赛季任务已经完成
	for i := range data.SeasonQuest.Quests {
		if data.SeasonQuest.Quests[i].Id == id {
			s.GetPlayer().LogInfo("found season quest is %v", data.SeasonQuest.Quests[i])
			return data.SeasonQuest.Quests[i], true
		}
	}

	// 日常任务 - 只拿今天的
	quests, ok := data.DailyQuestMap[s.GetOpenDay()]
	if !ok {
		return nil, false
	}
	for i := range quests.Quests {
		if quests.Quests[i].Id == id {
			s.GetPlayer().LogInfo("found daily quest is %v", quests.Quests[i])
			return quests.Quests[i], true
		}
	}
	return nil, false
}

// 拿到任务配制
func (s *FlyingFairyOrderFuncSys) getQuestConf(id uint32) (*jsondata.YyFlyingFairyOrderFuncQuestConf, bool) {
	conf, ok := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil, false
	}
	if conf.SeasonQuest != nil {
		questConf, ok := conf.SeasonQuest[fmt.Sprintf("%d", id)]
		if ok {
			return questConf, true
		}
	}

	// 日常任务 - 只拿今天的
	for i := range conf.DailyTask {
		if conf.DailyTask[i].QuestConf == nil {
			continue
		}
		questConf, ok := conf.DailyTask[i].QuestConf[fmt.Sprintf("%d", id)]
		if ok {
			return questConf, true
		}
	}
	return nil, false
}

func (s *FlyingFairyOrderFuncSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.FlyingFairyOrderFuncDataMap {
		return
	}
	delete(yyData.FlyingFairyOrderFuncDataMap, s.Id)
}

// 检查是否可以充值
func checkFlyingFairyOrderFuncHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	if conf.ChargeType != chargedef.FlyingFairyOrderFunc {
		return false
	}
	var checkRet bool
	var canCharge bool
	rangeFlyingFairyOrderFuncSys(player, func(s *FlyingFairyOrderFuncSys) {
		if checkRet {
			return
		}
		config, ok1 := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
		if !ok1 {
			return
		}
		for _, charge := range config.Charge {
			if uint32(charge.ChargeId) == conf.ChargeId {
				checkRet = true
				break
			}
		}
		if !checkRet {
			return
		}
		canCharge = s.GetData().UnLockChargeId == 0
	})
	if !checkRet {
		return false
	}

	// 没充过就行了 目前是要互斥的 普通不能升高级
	return canCharge
}

// 充值回调
func flyingFairyOrderFuncChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !checkFlyingFairyOrderFuncHandler(player, conf) {
		return false
	}

	var ret bool
	rangeFlyingFairyOrderFuncSys(player, func(sys *FlyingFairyOrderFuncSys) {
		if ret {
			return
		}
		err := sys.charge(conf.ChargeId)
		if err != nil {
			player.LogError("privilegeCardMonthChargeHandler: %s", err.Error())
			return
		}
		ret = true
		player.SendTipMsg(tipmsgid.TpFlyingFairyOrderFuncCharge, player.GetId(), player.GetName())
	})
	return ret
}

func rangeFlyingFairyOrderFuncSys(player iface.IPlayer, doLogic func(sys *FlyingFairyOrderFuncSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiFlyingFairyOrderFunc)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*FlyingFairyOrderFuncSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		doLogic(sys)
	}
}

func handleAddFlyingFairyOrderFuncExp(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	sysId := args[0].(int)
	exp := args[1].(uint32)
	rangeFlyingFairyOrderFuncSys(player, func(s *FlyingFairyOrderFuncSys) {
		config, ok1 := jsondata.GetYyFlyingFairyOrderFuncConf(s.ConfName, s.ConfIdx)
		if !ok1 {
			return
		}
		if config.OtherProvideExpSysId != 0 && config.OtherProvideExpSysId == sysId {
			finalExp := s.addExp(int64(exp))
			s.addLevel()
			s.LogPlayerBehavior(uint64(finalExp), map[string]interface{}{"sysId": sysId}, pb3.LogId_LogAddFlyingFairyOrderFuncExp)
		}
	})
}

func init() {
	engine.RegChargeEvent(chargedef.FlyingFairyOrderFunc, checkFlyingFairyOrderFuncHandler, flyingFairyOrderFuncChargeHandler)

	pyymgr.RegPlayerYY(yydefine.YYSiFlyingFairyOrderFunc, createYYFlyingFairyOrderFuncSys)

	event.RegActorEvent(custom_id.AeAddFlyingFairyOrderFuncExp, handleAddFlyingFairyOrderFuncExp)

	net.RegisterYYSysProtoV2(145, 1, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FlyingFairyOrderFuncSys).c2sReceiveReward
	})
	net.RegisterYYSysProtoV2(145, 2, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FlyingFairyOrderFuncSys).c2sReceiveAllReward
	})
	net.RegisterYYSysProtoV2(145, 3, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FlyingFairyOrderFuncSys).levelBuy
	})
	net.RegisterYYSysProtoV2(145, 4, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FlyingFairyOrderFuncSys).c2sRewardQuest
	})
	net.RegisterYYSysProtoV2(145, 5, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FlyingFairyOrderFuncSys).c2sAllRewardQuest
	})
}
