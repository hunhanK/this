/**
 * @Author: LvYuMeng
 * @Date: 2024/11/27
 * @Desc: 潜能
**/

package actorsystem

import (
	"errors"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
)

type PotentialSys struct {
	*QuestTargetBase
}

var (
	potentialQuestTargetMap = map[uint32]map[uint32]struct{}{} // 境界任务事件对应的id
)

func newPotentialSys() iface.ISystem {
	sys := &PotentialSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (s *PotentialSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := potentialQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

func (s *PotentialSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf, err := jsondata.GetPotentialQuestConf(id)
	if nil != err {
		return nil
	}
	return conf.Targets
}

func (s *PotentialSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	questConf, err := jsondata.GetPotentialQuestConf(id)
	if nil != err {
		return nil
	}
	data := s.getData()
	state, ok := data.QuestState[id]
	if !ok {
		return nil
	}
	if state.DailyTimes >= questConf.Count {
		return nil
	}
	quest, ok := data.Quests[id]
	if !ok {
		return nil
	}
	return quest
}

func (s *PotentialSys) onUpdateTargetData(questId uint32) {
	quest := s.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	s.checkTaskComplete(questId)
	data := s.getData()
	s.SendProto3(74, 12, &pb3.S2C_74_12{
		State: data.QuestState[questId],
		Quest: quest,
	})
}

func (s *PotentialSys) checkResetQuest() {
	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return
	}
	data := s.getData()
	for id, questConf := range conf.Quest {
		if _, ok := data.QuestState[id]; !ok {
			data.QuestState[id] = &pb3.PotentialQuestState{Id: id}
		}
		state := data.QuestState[id]
		if state.DailyTimes >= questConf.Count {
			continue
		}
		_, ok := data.Quests[id]
		if ok {
			continue
		}
		quest := &pb3.QuestData{
			Id: questConf.Id,
		}
		data.Quests[id] = quest
		s.OnAcceptQuestAndCheckUpdateTarget(quest)
	}
}

func (s *PotentialSys) checkTaskComplete(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}
	if !s.CheckFinishQuest(quest) {
		return
	}

	data := s.getData()
	questConf, err := jsondata.GetPotentialQuestConf(id)
	if nil != err {
		return
	}

	state, ok := data.QuestState[id]
	if !ok {
		return
	}
	if questConf.Count <= state.DailyTimes {
		return
	}

	state.DailyTimes++

	engine.GiveRewards(s.owner, questConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPotentialQuestFinish,
	})

	delete(data.Quests, id)
	s.checkResetQuest()
}

func (s *PotentialSys) getData() *pb3.PotentialData {
	binary := s.GetBinaryData()
	if nil == binary.PotentialData {
		binary.PotentialData = &pb3.PotentialData{}
	}
	if nil == binary.PotentialData.Plans {
		binary.PotentialData.Plans = make(map[uint32]*pb3.PotentialPlan)
	}
	if nil == binary.PotentialData.Quests {
		binary.PotentialData.Quests = make(map[uint32]*pb3.QuestData)
	}
	if nil == binary.PotentialData.QuestState {
		binary.PotentialData.QuestState = make(map[uint32]*pb3.PotentialQuestState)
	}
	return binary.PotentialData
}

func (s *PotentialSys) getCurrentOpenTime() (uint32, uint32) {
	now := time.Now()
	year, month, day := now.Date()

	var startTime, endTime time.Time

	if day <= 15 {
		startTime = time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		endTime = time.Date(year, month, 15, 23, 59, 59, 0, now.Location())
	} else {
		startTime = time.Date(year, month, 16, 0, 0, 0, 0, now.Location())
		lastDay := time.Date(year, month+1, 1, 0, 0, -1, 0, now.Location())
		endTime = lastDay
	}

	return uint32(startTime.Unix()), uint32(endTime.Unix())
}

func (s *PotentialSys) OnLogin() {
	s.checkResetQuest()
	s.setLvExpAttr()
	s.setPotentialMainTypeAttr()
}

func (s *PotentialSys) setPotentialMainTypeAttr() {
	s.owner.SetExtraAttr(attrdef.PotentialMainType, int64(s.getUsingPlanMainType()))
}

func (s *PotentialSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *PotentialSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PotentialSys) OnOpen() {
	s.checkOpen()
}

func (s *PotentialSys) s2cInfo() {
	s.SendProto3(74, 0, &pb3.S2C_74_0{Data: s.getData()})
}

func (s *PotentialSys) checkOpen() {
	data := s.getData()
	if data.EndTime >= time_util.NowSec() {
		return
	}
	s.resetLoop()
}

func (s *PotentialSys) resetLoop() {
	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return
	}

	startTime, endTime := s.getCurrentOpenTime()
	oldData := s.getData()

	//遗忘旧赛季技能
	for _, plan := range oldData.Plans {
		forgetSkillIds := s.getAllLearnedSkillByPlan(plan)
		for _, st := range forgetSkillIds {
			s.owner.ForgetSkill(st.Key, true, true, true)
		}
	}

	newData := &pb3.PotentialData{
		Plans:         make(map[uint32]*pb3.PotentialPlan),
		UnlockPlanNum: utils.MaxUInt32(oldData.UnlockPlanNum, conf.DefaultPlanOpen),
		PlanId:        1,
		StartTime:     startTime,
		EndTime:       endTime,
		Lv:            1,
	}
	//保存旧页签名
	for idx := uint32(1); idx <= newData.UnlockPlanNum; idx++ {
		name := ""
		if oldPlan, ok := oldData.Plans[idx]; ok {
			name = oldPlan.Name
		}
		newData.Plans[idx] = s.createNewPlan(idx, name)
	}
	s.GetBinaryData().PotentialData = newData
	s.s2cInfo()
	s.setLvExpAttr()
	s.setPotentialMainTypeAttr()
	s.checkResetQuest()
}

func (s *PotentialSys) getAllLearnedSkillByPlan(plan *pb3.PotentialPlan) []*pb3.KeyValue {
	var skillIds []*pb3.KeyValue
	for _, mySeries := range plan.Pos2Sub {
		if mySeries.MainSkillId > 0 && mySeries.MainLv > 0 {
			skillIds = append(skillIds, &pb3.KeyValue{
				Key:   mySeries.MainSkillId,
				Value: mySeries.MainLv,
			})
		}
		for branchId, branchSkillId := range mySeries.BranchDevelop {
			if branchSkillId > 0 && mySeries.BranchLv[branchId] > 0 {
				skillIds = append(skillIds, &pb3.KeyValue{
					Key:   branchSkillId,
					Value: mySeries.BranchLv[branchId],
				})
			}
		}
	}
	return skillIds
}

func (s *PotentialSys) initPlan(plan *pb3.PotentialPlan) {
	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return
	}

	//解锁无消耗孔位
	for _, posConf := range conf.PotentialPos {
		if posConf.UnlockPoint == 0 && !pie.Uint32s(plan.UnlockPos).Contains(posConf.Type) {
			plan.UnlockPos = append(plan.UnlockPos, posConf.Type)
		}
	}

	specialSeriesConf, err := jsondata.GetPotentialSpecialSeriesConf()
	if err != nil {
		s.LogError("not found special series conf,err:%v", err)
		return
	}

	plan.Pos2Sub[potentialPosTypeSpecial] = &pb3.PotentialBranch{
		SubType:       specialSeriesConf.SubType,
		BranchDevelop: map[uint32]uint32{},
		BranchLv:      map[uint32]uint32{},
	}
}

func (s *PotentialSys) setLvExpAttr() {
	data := s.getData()
	s.owner.SetExtraAttr(attrdef.PotentialLevel, int64(data.Lv))
	s.owner.SetExtraAttr(attrdef.PotentialExp, data.Exp)
}

func (s *PotentialSys) getUsingPlanMainType() uint32 {
	data := s.getData()
	plan, err := s.getPlanInfo(data.PlanId)
	if nil != err {
		return 0
	}
	series, ok := s.getSeries(plan, potentialPosTypeMain)
	if !ok {
		return 0
	}
	return series.SubType
}

func (s *PotentialSys) onNewDay() {
	s.checkOpen()
	s.dailyQuestReset()
	s.s2cInfo()
}

func (s *PotentialSys) dailyQuestReset() {
	data := s.getData()
	data.Quests = make(map[uint32]*pb3.QuestData)
	data.QuestState = make(map[uint32]*pb3.PotentialQuestState)
	s.checkResetQuest()
}

func (s *PotentialSys) SetLevel(level uint32) {
	data := s.getData()
	data.Lv = level
	s.owner.SetExtraAttr(attrdef.PotentialLevel, int64(data.Lv))
}

func (s *PotentialSys) SetExp(exp int64) {
	data := s.getData()
	data.Exp = exp
	s.owner.SetExtraAttr(attrdef.PotentialExp, data.Exp)
}

func (s *PotentialSys) GetOpenDay() uint32 {
	data := s.getData()
	if data.StartTime == 0 {
		return 0
	}
	return time_util.TimestampSubDays(data.StartTime, time_util.NowSec()) + 1
}

func (s *PotentialSys) AddExp(exp int64, logId pb3.LogId) {
	if exp <= 0 {
		return
	}

	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return
	}

	data := s.getData()
	openDay := s.GetOpenDay()
	oldLv := data.GetLv()
	maxLv := uint32(len(conf.LvConf))
	var addRate int64
	for _, v := range conf.RateConf {
		if openDay < v.MinDay || openDay > v.MaxDay {
			continue
		}
		if oldLv < v.MinLevel || oldLv > v.MaxLevel {
			continue
		}
		addRate = int64(v.Rate)
		break
	}
	exp = exp + utils.CalcMillionRate64(exp, addRate)
	myExp := data.GetExp() + exp

	for nextLv := data.GetLv() + 1; nextLv < maxLv; nextLv++ {
		nextLvConf := conf.LvConf[nextLv-1]
		if myExp >= nextLvConf.Exp {
			myExp -= nextLvConf.Exp
			s.SetExp(0)
			s.SetLevel(nextLv)
		} else {
			//s.SetLevel(nextLv)
			s.SetExp(myExp)
			break
		}
	}

	logworker.LogExpLv(s.owner, logId, &pb3.LogExpLv{Exp: exp, FromLevel: oldLv})
}

func (s *PotentialSys) isTabOpen(idx uint32) bool {
	data := s.getData()
	return data.UnlockPlanNum >= idx
}

// 1:激活 2:阵位解锁 3:子系列切换 4:分支解锁 5:培养点切换 6:回退 7 主潜能升级 8 副潜能升级
const (
	potentialUpdateTypeUnlockTab            = 1
	potentialUpdateTypeUnlockPos            = 2
	potentialUpdateTypeSeriesToPos          = 3
	potentialUpdateTypeUnlockSeriesBranch   = 4
	potentialUpdateTypeChangeBranchSkill    = 5
	potentialUpdateTypeBackUsePoint         = 6
	potentialUpdateTypeUpdateMainSkill      = 7
	potentialUpdateTypeUpdateBranchSkill    = 8
	potentialUpdateTypeUpdateRecommendReset = 9
)

const (
	potentialPosTypeMain    = 1
	potentialPosTypeSub     = 2
	potentialPosTypeSpecial = 3
)

func (s *PotentialSys) getPlanInfo(planId uint32) (*pb3.PotentialPlan, error) {
	if !s.isTabOpen(planId) {
		return nil, neterror.ParamsInvalidError("plan %d not open", planId)
	}
	data := s.getData()
	plan, ok := data.Plans[planId]
	if !ok {
		return nil, neterror.ParamsInvalidError("plan %d data not init", planId)
	}
	if nil == plan.Pos2Sub {
		plan.Pos2Sub = map[uint32]*pb3.PotentialBranch{}
	}
	return plan, nil
}

func (s *PotentialSys) onUpdatePlanInfo(plan *pb3.PotentialPlan, updateType uint32) {
	s.setPotentialMainTypeAttr()
	s.SendProto3(74, 1, &pb3.S2C_74_1{
		Idx:  plan.Id,
		Plan: plan,
		Type: updateType,
	})
}

func (s *PotentialSys) subPoint(plan *pb3.PotentialPlan, score uint32) bool {
	size := s.getTotalPointSize()
	if plan.UsePoinit+score > size {
		return false
	}
	plan.UsePoinit += score
	s.SendProto3(74, 7, &pb3.S2C_74_7{
		Idx:      plan.Id,
		UsePoint: plan.UsePoinit,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogPotentialPointUse, &pb3.LogPlayerCounter{
		NumArgs: uint64(score),
	})
	return true
}

func (s *PotentialSys) backPoint(plan *pb3.PotentialPlan, score uint32) bool {
	if plan.UsePoinit < score {
		s.owner.LogStack("plan %d occupy back exceed! back %d, use %d", plan.Id, score, plan.UsePoinit)
		plan.UsePoinit = 0
	} else {
		plan.UsePoinit -= score
	}
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogPotentialPointBack, &pb3.LogPlayerCounter{
		NumArgs: uint64(score),
	})
	return true
}

func (s *PotentialSys) unlockPos(plan *pb3.PotentialPlan, pos uint32) (bool, error) {
	if pie.Uint32s(plan.UnlockPos).Contains(pos) {
		return true, nil
	}
	posConf, err := jsondata.GetPotentialPosConf(pos)
	if nil != err {
		return false, err
	}
	if !s.subPoint(plan, posConf.UnlockPoint) {
		s.owner.SendTipMsg(tipmsgid.SkillPointNotEnough)
		return false, nil
	}
	plan.UnlockPos = append(plan.UnlockPos, pos)
	return true, nil
}

func (s *PotentialSys) getSeries(plan *pb3.PotentialPlan, pos uint32) (*pb3.PotentialBranch, bool) {
	if nil == plan {
		return nil, false
	}
	series, ok := plan.Pos2Sub[pos]
	if !ok {
		return nil, false
	}
	if nil == series.BranchDevelop {
		series.BranchDevelop = map[uint32]uint32{}
	}
	if nil == series.BranchLv {
		series.BranchLv = map[uint32]uint32{}
	}
	return series, true
}

func (s *PotentialSys) checkSeriesToPos(plan *pb3.PotentialPlan, pos, newSeriesId uint32) (bool, error) {
	if !pie.Uint32s(plan.UnlockPos).Contains(pos) {
		return false, neterror.ParamsInvalidError("pos not unlock")
	}
	if pos == potentialPosTypeSpecial {
		return false, neterror.ParamsInvalidError("special pos cant choose")
	}
	newSeriesConf, err := jsondata.GetPotentialSeriesConf(newSeriesId)
	if err != nil {
		return false, err
	}
	if newSeriesConf.IsSpecial {
		return false, neterror.ParamsInvalidError("is special series")
	}
	oldSeries, isTake := s.getSeries(plan, pos)
	if isTake && oldSeries.SubType == newSeriesId {
		return false, neterror.ParamsInvalidError("has take")
	}

	if pos == potentialPosTypeSub { //副的不能选主位已选系列
		mainSeries, ok := s.getSeries(plan, potentialPosTypeMain)
		if !ok {
			return false, neterror.ParamsInvalidError("main series is nil")
		}
		if mainSeries.SubType == newSeriesId {
			return false, neterror.ParamsInvalidError("main series is use")
		}
	}
	return true, nil
}

func (s *PotentialSys) c2sUnlockTab(msg *base.Message) error {
	var req pb3.C2S_74_1
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	if s.isTabOpen(req.GetIdx()) {
		return neterror.ParamsInvalidError("plan tab repeated unlock")
	}

	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	data := s.getData()
	if data.UnlockPlanNum >= conf.PlanNum {
		return neterror.ParamsInvalidError("plan tab unlock num limit")
	}

	if !s.owner.ConsumeByConf(conf.PlanConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogPotentialTabUnlock}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	newIdx := data.UnlockPlanNum + 1
	data.UnlockPlanNum = newIdx
	data.Plans[newIdx] = s.createNewPlan(newIdx, req.GetName())

	s.onUpdatePlanInfo(data.Plans[newIdx], potentialUpdateTypeUnlockTab)

	return nil
}

func (s *PotentialSys) createNewPlan(idx uint32, name string) *pb3.PotentialPlan {
	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return nil
	}

	if name == "" {
		name = fmt.Sprintf("%s%d", conf.DefaultPlanName, idx)
	}

	newPlan := &pb3.PotentialPlan{
		Id:      idx,
		Name:    name,
		Pos2Sub: map[uint32]*pb3.PotentialBranch{},
	}

	s.initPlan(newPlan)

	return newPlan
}

const potentialTabNameLen = 4

func (s *PotentialSys) c2sChangeTabName(msg *base.Message) error {
	var req pb3.C2S_74_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	_, err = s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}
	if len([]rune(req.Name)) > potentialTabNameLen {
		return neterror.ParamsInvalidError("name limit")
	}
	engine.SendWordMonitor(wordmonitor.Name, wordmonitor.PotentialPlanTabName, req.GetName(),
		wordmonitoroption.WithPlayerId(s.owner.GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(s.owner.BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(s.owner.GetExtraAttrU32(attrdef.DitchId)),
	)

	return nil
}

func (s *PotentialSys) changeTabName(req *pb3.C2S_74_2) {
	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		s.owner.LogError("plan is nil")
		return
	}
	plan.Name = req.GetName()
	s.SendProto3(74, 2, &pb3.S2C_74_2{
		Idx:  plan.GetId(),
		Name: plan.GetName(),
	})
}

func (s *PotentialSys) getTotalPointSize() uint32 {
	data := s.getData()
	lvConf, err := jsondata.GetPotentialLvConf(data.GetLv())
	if nil != err {
		return 0
	}
	return lvConf.Point
}

func (s *PotentialSys) c2sUnlockPos(msg *base.Message) error {
	var req pb3.C2S_74_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	success, err := s.unlockPos(plan, req.GetType())
	if !success {
		return err
	}

	s.onUpdatePlanInfo(plan, potentialUpdateTypeUnlockPos)
	return nil
}

func (s *PotentialSys) isValidPlan(idx uint32) (bool, error) {
	if idx <= 0 {
		return false, neterror.ParamsInvalidError("idx is 0")
	}
	data := s.getData()
	if data.PlanId != idx {
		return false, neterror.ParamsInvalidError("plan not use")
	}
	return true, nil
}

func (s *PotentialSys) createNewSeries(newSeriesId uint32) (*pb3.PotentialBranch, error) {
	series := &pb3.PotentialBranch{
		SubType:       newSeriesId,
		BranchDevelop: map[uint32]uint32{},
		BranchLv:      map[uint32]uint32{},
		MainLv:        0,
	}

	newSeriesConf, err := jsondata.GetPotentialSeriesConf(newSeriesId)
	if err != nil {
		return nil, err
	}

	if newSeriesConf.MainSkillID > 0 {
		series.MainLv = 1
		series.MainSkillId = newSeriesConf.MainSkillID
	}
	return series, nil
}

func (s *PotentialSys) c2sSeriesToPos(msg *base.Message) error {
	var req pb3.C2S_74_4
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos, newSeriesId := req.GetType(), req.GetSubType()

	success, err := s.checkSeriesToPos(plan, pos, newSeriesId)
	if !success {
		return err
	}

	newSeries, err := s.createNewSeries(newSeriesId)
	if nil != err {
		return err
	}

	oldSeries, needSwitch := s.getSeries(plan, pos)

	if needSwitch {
		var backPoint uint32
		seriesConf, err := jsondata.GetPotentialSeriesConf(oldSeries.SubType)
		if nil != err {
			return err
		}
		var forgerSkillId []uint32
		//主技能点返还
		backPoint += seriesConf.SkillLvConf.CalcSkillBackPoint(oldSeries.MainLv)
		if seriesConf.MainSkillID > 0 {
			forgerSkillId = append(forgerSkillId, seriesConf.MainSkillID)
		}
		//分支技能点返还
		for branchId, branchLv := range oldSeries.BranchLv {
			if branchConf, err := jsondata.GetPotentialBranchConf(oldSeries.SubType, branchId); nil != err {
				return err
			} else {
				backPoint += branchConf.SkillLvConf.CalcSkillBackPoint(branchLv)
			}
		}
		for _, skillId := range oldSeries.BranchDevelop {
			forgerSkillId = append(forgerSkillId, skillId)
		}
		for _, skillId := range forgerSkillId {
			s.owner.ForgetSkill(skillId, true, true, true)
		}
		delete(plan.Pos2Sub, pos)
		s.backPoint(plan, backPoint)
	}

	plan.Pos2Sub[pos] = newSeries

	if newSeries.MainSkillId > 0 {
		if !s.owner.LearnSkill(newSeries.MainSkillId, newSeries.MainLv, true) {
			s.owner.LogError("skill %d learn failed", newSeries.MainSkillId)
		}
	}

	s.onUpdatePlanInfo(plan, potentialUpdateTypeSeriesToPos)

	return nil
}

func (s *PotentialSys) c2sUnlockSeriesBranch(msg *base.Message) error {
	var req pb3.C2S_74_5
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos, branchId := req.GetType(), req.GetBranchId()

	mySeries, ok := s.getSeries(plan, pos)
	if !ok {
		return neterror.ParamsInvalidError("series not choose")
	}

	if mySeries.BranchLv[branchId] > 0 {
		return neterror.ParamsInvalidError("has unlock")
	}

	branchConf, err := jsondata.GetPotentialBranchConf(mySeries.SubType, branchId)
	if nil != err {
		return err
	}

	data := s.getData()
	//解锁条件判断
	if branchConf.BranchPotentialLv > data.Lv {
		return neterror.ParamsInvalidError("BranchPotentialLv not enough")
	}

	if branchConf.NeedInMain && pos != potentialPosTypeMain {
		return neterror.ParamsInvalidError("pos not main")
	}

	var initLevel uint32 = 1
	lvConf, ok := branchConf.SkillLvConf[initLevel]
	if !ok {
		return neterror.ParamsInvalidError("branch level conf %d is nil", initLevel)
	}

	if !s.subPoint(plan, lvConf.Point) {
		s.owner.SendTipMsg(tipmsgid.SkillPointNotEnough)
		return nil
	}

	mySeries.BranchLv[branchId] = initLevel

	s.onUpdatePlanInfo(plan, potentialUpdateTypeUnlockSeriesBranch)

	return nil
}

func (s *PotentialSys) c2sChangeBranchSkill(msg *base.Message) error {
	var req pb3.C2S_74_6
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos, branchId, skillId := req.GetType(), req.GetBranchId(), req.GetDevelopId()

	mySeries, ok := s.getSeries(plan, pos)
	if !ok {
		return neterror.ParamsInvalidError("series not choose")
	}

	if mySeries.BranchLv[branchId] == 0 {
		return neterror.ParamsInvalidError("not unlock")
	}

	branchConf, err := jsondata.GetPotentialBranchConf(mySeries.SubType, branchId)
	if err != nil {
		return err
	}

	if !pie.Uint32s(branchConf.BranchSkillID).Contains(req.GetDevelopId()) {
		return neterror.ParamsInvalidError("skillId %d not exist", skillId)
	}

	if oldSkillId := mySeries.BranchDevelop[branchId]; oldSkillId > 0 {
		s.owner.ForgetSkill(oldSkillId, true, true, true)
		delete(mySeries.BranchDevelop, branchId)
	}

	mySeries.BranchDevelop[branchId] = skillId
	if !s.owner.LearnSkill(skillId, mySeries.BranchLv[branchId], true) {
		s.owner.LogError("skill %d learn failed", skillId)
	}

	s.onUpdatePlanInfo(plan, potentialUpdateTypeChangeBranchSkill)

	return nil
}

func (s *PotentialSys) c2sBackUsePoint(msg *base.Message) error {
	var req pb3.C2S_74_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos, branchId := req.GetType(), req.GetBranchId()
	isMain := branchId == 0

	mySeries, ok := s.getSeries(plan, pos)
	if !ok {
		return neterror.ParamsInvalidError("series not choose")
	}

	var backPoint uint32

	if isMain {
		seriesConf, err := jsondata.GetPotentialSeriesConf(mySeries.SubType)
		if nil != err {
			return err
		}
		backPoint = seriesConf.SkillLvConf.CalcSkillBackPoint(mySeries.MainLv)
		s.owner.ForgetSkill(mySeries.MainSkillId, true, true, true)
		mySeries.MainLv = 0

		if seriesConf.MainSkillID > 0 {
			mySeries.MainLv = 1
			mySeries.MainSkillId = seriesConf.MainSkillID
			if !s.owner.LearnSkill(mySeries.MainSkillId, mySeries.MainLv, true) {
				s.owner.LogError("skill %d learn failed", mySeries.MainSkillId)
			}
		}
	} else {
		branchConf, err := jsondata.GetPotentialBranchConf(mySeries.SubType, branchId)
		if nil != err {
			return err
		}
		backPoint = branchConf.SkillLvConf.CalcSkillBackPoint(mySeries.BranchLv[branchId])
		s.owner.ForgetSkill(mySeries.BranchDevelop[branchId], true, true, true)
		delete(mySeries.BranchLv, branchId)
		delete(mySeries.BranchDevelop, branchId)
	}

	s.backPoint(plan, backPoint)

	s.onUpdatePlanInfo(plan, potentialUpdateTypeBackUsePoint)
	return nil
}

func (s *PotentialSys) c2sChangeTab(msg *base.Message) error {
	var req pb3.C2S_74_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	changePlan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	data := s.getData()
	currentPlanId := data.PlanId
	currentPlan, err := s.getPlanInfo(currentPlanId)
	if nil != err {
		return err
	}

	forgetSkillIds := s.getAllLearnedSkillByPlan(currentPlan)
	for _, st := range forgetSkillIds {
		s.owner.ForgetSkill(st.Key, true, true, true)
	}

	data.PlanId = changePlan.Id
	learnSkillIds := s.getAllLearnedSkillByPlan(changePlan)
	for _, st := range learnSkillIds {
		s.owner.LearnSkill(st.Key, st.Value, true)
	}

	s.SendProto3(74, 8, &pb3.S2C_74_8{Idx: changePlan.Id})
	return nil
}

func (s *PotentialSys) c2sUpdateMainSkill(msg *base.Message) error {
	var req pb3.C2S_74_9
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos := req.GetType()

	mySeries, ok := s.getSeries(plan, pos)
	if !ok {
		return neterror.ParamsInvalidError("series not choose")
	}

	if mySeries.MainSkillId == 0 {
		return neterror.ParamsInvalidError("series cant lv up")
	}

	seriesConf, err := jsondata.GetPotentialSeriesConf(mySeries.SubType)
	if err != nil {
		return err
	}

	nextLv := mySeries.MainLv + 1
	nextLvConf, err := seriesConf.SkillLvConf.GetLevelConf(nextLv)
	if nil != err {
		return err
	}

	if !s.subPoint(plan, nextLvConf.Point) {
		s.owner.SendTipMsg(tipmsgid.SkillPointNotEnough)
		return nil
	}
	mySeries.MainLv = nextLv
	if !s.owner.LearnSkill(mySeries.MainSkillId, mySeries.MainLv, true) {
		s.LogError("skill %d learn failed", mySeries.MainSkillId)
	}

	s.onUpdatePlanInfo(plan, potentialUpdateTypeUpdateMainSkill)
	return nil
}

func (s *PotentialSys) c2sUpdateBranchSkill(msg *base.Message) error {
	var req pb3.C2S_74_10
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	pos, branchId := req.GetType(), req.GetBranchId()

	mySeries, ok := s.getSeries(plan, pos)
	if !ok {
		return neterror.ParamsInvalidError("series not choose")
	}

	if mySeries.BranchLv[branchId] == 0 {
		return neterror.ParamsInvalidError("branch not active")
	}

	branchConf, err := jsondata.GetPotentialBranchConf(mySeries.SubType, branchId)
	if err != nil {
		return err
	}

	nextLv := mySeries.BranchLv[branchId] + 1
	nextLvConf, err := branchConf.SkillLvConf.GetLevelConf(nextLv)
	if nil != err {
		return err
	}

	if !s.subPoint(plan, nextLvConf.Point) {
		s.owner.SendTipMsg(tipmsgid.SkillPointNotEnough)
		return nil
	}

	mySeries.BranchLv[branchId] = nextLv

	skillId := mySeries.BranchDevelop[branchId]
	if skillId > 0 {
		if !s.owner.LearnSkill(skillId, mySeries.BranchLv[branchId], true) {
			s.LogError("skill %d learn failed", mySeries.MainSkillId)
		}
	}

	s.onUpdatePlanInfo(plan, potentialUpdateTypeUpdateBranchSkill)
	return nil
}

func (s *PotentialSys) c2sUseRecommendPlan(msg *base.Message) error {
	var req pb3.C2S_74_11
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	isValid, err := s.isValidPlan(req.GetIdx())
	if !isValid {
		return err
	}

	plan, err := s.getPlanInfo(req.GetIdx())
	if nil != err {
		return err
	}

	recommendConf, err := jsondata.GetPotentialRecommendConf(req.GetRecommendId())
	if nil != err {
		return err
	}

	if recommendConf.MainUse == recommendConf.SubUse {
		return neterror.ParamsInvalidError("recommend series is equal")
	}

	newPlan := s.createNewPlan(plan.Id, plan.Name)

	addSeries := func(plan *pb3.PotentialPlan, pos, newSeriesId uint32) bool {
		isUnlock, _ := s.unlockPos(newPlan, pos)
		if !isUnlock {
			return false
		}
		can, _ := s.checkSeriesToPos(plan, potentialPosTypeMain, newSeriesId)
		if !can {
			return false
		}
		newSeries, err := s.createNewSeries(newSeriesId)
		if nil != err {
			return false
		}
		plan.Pos2Sub[pos] = newSeries
		return true
	}

	addSeries(newPlan, potentialPosTypeMain, recommendConf.MainUse)
	addSeries(newPlan, potentialPosTypeSub, recommendConf.SubUse)

	forgetSkillIds := s.getAllLearnedSkillByPlan(plan)
	for _, st := range forgetSkillIds {
		s.owner.ForgetSkill(st.Key, true, true, true)
	}

	data := s.getData()
	data.Plans[plan.Id] = newPlan

	newSkillIds := s.getAllLearnedSkillByPlan(plan)
	for _, st := range newSkillIds {
		s.owner.LearnSkill(st.Key, st.Value, true)
	}

	s.onUpdatePlanInfo(data.Plans[plan.Id], potentialUpdateTypeUpdateRecommendReset)

	return nil
}

func onAfterReloadPotentialConf(args ...interface{}) {
	conf := jsondata.GetPotentialConf()
	if nil == conf {
		return
	}
	tmp := make(map[uint32]map[uint32]struct{})
	for id, quest := range conf.Quest {
		for _, target := range quest.Targets {
			if _, ok := tmp[target.Type]; !ok {
				tmp[target.Type] = make(map[uint32]struct{})
			}
			tmp[target.Type][id] = struct{}{}
		}
	}
	potentialQuestTargetMap = tmp
}

func init() {
	RegisterSysClass(sysdef.SiPotential, func() iface.ISystem {
		return newPotentialSys()
	})

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadPotentialConf)

	net.RegisterSysProtoV2(74, 1, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUnlockTab
	})
	net.RegisterSysProtoV2(74, 2, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sChangeTabName
	})
	net.RegisterSysProtoV2(74, 3, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUnlockPos
	})
	net.RegisterSysProtoV2(74, 4, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sSeriesToPos
	})
	net.RegisterSysProtoV2(74, 5, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUnlockSeriesBranch
	})
	net.RegisterSysProtoV2(74, 6, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sChangeBranchSkill
	})
	net.RegisterSysProtoV2(74, 7, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sBackUsePoint
	})
	net.RegisterSysProtoV2(74, 8, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sChangeTab
	})
	net.RegisterSysProtoV2(74, 9, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUpdateMainSkill
	})
	net.RegisterSysProtoV2(74, 10, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUpdateBranchSkill
	})
	net.RegisterSysProtoV2(74, 11, sysdef.SiPotential, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PotentialSys).c2sUseRecommendPlan
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiPotential).(*PotentialSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	engine.RegWordMonitorOpCodeHandler(wordmonitor.PotentialPlanTabName, func(word *wordmonitor.Word) error {
		player := manager.GetPlayerPtrById(word.PlayerId)
		if nil == player {
			return nil
		}
		if word.Ret != wordmonitor2.Success {
			player.SendTipMsg(tipmsgid.TpSensitiveWord)
			return nil
		}
		req, ok := word.Data.(*pb3.C2S_74_2)
		if !ok {
			return errors.New("not *pb3.C2S_74_2")
		}
		sys, ok := player.GetSysObj(sysdef.SiPotential).(*PotentialSys)
		if !ok || !sys.IsOpen() {
			return errors.New("sys not open")
		}
		sys.changeTabName(req)
		return nil
	})

	gmevent.Register("potential.season", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiPotential).(*PotentialSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		sys.resetLoop()
		return true
	}, 1)
}
