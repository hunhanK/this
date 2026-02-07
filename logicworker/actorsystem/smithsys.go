/**
 * @Author: LvYuMeng
 * @Date: 2025/7/25
 * @Desc: 铁匠
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"golang.org/x/exp/maps"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type SmithSys struct {
	*QuestTargetBase
	qualityUpTimer *time_util.Timer
}

var (
	errSmithSysNil  = neterror.SysNotExistError("smith sys err is nil")
	errSmithConfNil = neterror.ConfNotFoundError("smith conf is nil")
	errSmithRefNil  = neterror.InternalError("smith ref is nil")
)

const (
	smithQuestTypeDaily = 1
	smithQuestTypeStage = 2
)

func newSmithSys() iface.ISystem {
	sys := &SmithSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *SmithSys) GetSysData() *pb3.Smith {
	binary := s.GetBinaryData()

	if nil == binary.SmithData {
		binary.SmithData = &pb3.SmithData{}
	}

	if nil == binary.SmithData.SysData {
		binary.SmithData.SysData = make(map[uint32]*pb3.Smith)
	}

	sysId := s.GetSysId()
	sysData, ok := binary.SmithData.SysData[sysId]
	if !ok {
		sysData = &pb3.Smith{}
		binary.SmithData.SysData[sysId] = sysData
	}

	if nil == sysData.StageUpQuest {
		sysData.StageUpQuest = map[uint32]*pb3.SmithQuest{}
	}

	if nil == sysData.DailyQuest {
		sysData.DailyQuest = map[uint32]*pb3.SmithQuest{}
	}

	return sysData
}

func (s *SmithSys) OnLogin() {
	s.SetExAttr()
	s.acceptStageQuest()
	s.setQualityUpTimer()
}

func (s *SmithSys) onNewDay() {
	sysData := s.GetSysData()
	s.compensateDailyMail()
	sysData.DailyQuest = nil
	s.acceptDailyQuest()
	s.s2cInfo()
}

func (s *SmithSys) compensateDailyMail() {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	sysData := s.GetSysData()
	var rewardsVec []jsondata.StdRewardVec
	for _, v := range sysData.DailyQuest {
		if v.IsRev {
			continue
		}
		if !s.CheckFinishQuest(v.Quest) {
			continue
		}
		dqConf := conf.GetDailyQuestConf(v.ConfId)
		if nil == dqConf {
			continue
		}
		v.IsRev = true
		rewardsVec = append(rewardsVec, dqConf.Rewards)
	}

	rewards := jsondata.AppendStdReward(rewardsVec...)

	if len(rewards) > 0 {
		s.owner.SendMail(&mailargs.SendMailSt{
			ConfId:  conf.DailyTaskMailId,
			Rewards: rewards,
		})
	}
}

func (s *SmithSys) OnOpen() {
	s.SetLevel(1)
	s.SetStage(1)
	s.acceptStageQuest()
	s.acceptDailyQuest()
	s.reCalAttr()
	s.s2cInfo()
}

const smithDailyQuestId = 1
const smithStageQuestId = 1000

func (s *SmithSys) acceptDailyQuest() {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	sysData := s.GetSysData()
	if len(sysData.DailyQuest) > 0 {
		return
	}

	for idx, v := range conf.DailyTask {
		id := smithDailyQuestId + idx + 1
		quest := &pb3.SmithQuest{
			Id:     id,
			Type:   smithQuestTypeDaily,
			ConfId: v.QuestId,
			Quest: &pb3.QuestData{
				Id: id,
			},
		}
		sysData.DailyQuest[id] = quest
		s.OnAcceptQuestAndCheckUpdateTarget(sysData.DailyQuest[id].Quest)
		s.updateQuest(quest)
	}
}

func (s *SmithSys) updateQuest(q *pb3.SmithQuest) {
	s.SendProto3(80, 34, &pb3.S2C_80_34{
		SysId: s.GetSysId(),
		Quest: q,
	})
}

func (s *SmithSys) acceptStageQuest() {
	sysData := s.GetSysData()
	if len(sysData.StageUpQuest) > 0 {
		return
	}

	conf := s.GetConf()
	if nil == conf {
		return
	}

	stageConf, err := conf.GetSmithStageConf(sysData.Stage + 1)
	if nil != err {
		return
	}

	for idx, v := range stageConf.Quests {
		id := smithStageQuestId + idx + 1
		quest := &pb3.SmithQuest{
			Id:     id,
			Type:   smithQuestTypeStage,
			ConfId: v.QuestId,
			Quest:  &pb3.QuestData{Id: id},
			IsRev:  false,
		}
		sysData.StageUpQuest[id] = quest
		s.OnAcceptQuestAndCheckUpdateTarget(sysData.StageUpQuest[id].Quest)
		s.updateQuest(quest)
	}
}

func (s *SmithSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SmithSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SmithSys) s2cInfo() {
	s.SendProto3(80, 30, &pb3.S2C_80_30{
		SysId: s.GetSysId(),
		Data:  s.GetSysData(),
	})
}

func (s *SmithSys) GetConf() *jsondata.SmithConfig {
	return jsondata.GetSmithSysConf(s.GetSysId())
}

func (s *SmithSys) findQuestById(id uint32) *pb3.SmithQuest {
	sysData := s.GetSysData()
	if sq, ok := sysData.StageUpQuest[id]; ok {
		return sq
	}
	if sq, ok := sysData.DailyQuest[id]; ok {
		return sq
	}

	return nil
}

func (s *SmithSys) GetRef() (*gshare.SmithRef, error) {
	ref, ok := gshare.SmithInstance.FindSmithRefBySmithSysId(s.GetSysId())
	if !ok {
		return nil, errSmithRefNil
	}

	return ref, nil
}

func (s *SmithSys) AddExp(exp int64, logId pb3.LogId) {
	if exp <= 0 && logId != pb3.LogId_LogSmithStageUpLift {
		return
	}

	conf := s.GetConf()
	if nil == conf {
		return
	}

	data := s.GetSysData()
	maxLv := uint32(len(conf.LvConf))
	oldLv := data.GetLv()

	myExp := data.GetExp() + exp

	lvLimit := conf.GetLvLimitByStage(data.Stage)

	for nextLv := data.GetLv() + 1; nextLv < maxLv; nextLv++ {
		if nextLv > lvLimit {
			s.SetExp(myExp)
			break
		}
		nextLvConf := conf.LvConf[nextLv-1]
		if myExp >= nextLvConf.Exp {
			myExp -= nextLvConf.Exp
			s.SetExp(0)
			s.SetLevel(nextLv)
		} else {
			s.SetExp(myExp)
			break
		}
	}

	if oldLv != data.GetLv() {
		s.reCalAttr()
	}
	logworker.LogExpLv(s.owner, logId, &pb3.LogExpLv{Exp: exp, FromLevel: oldLv})
}

func (s *SmithSys) SetLevel(level uint32) {
	ref, err := s.GetRef()
	if err != nil {
		return
	}

	data := s.GetSysData()
	data.Lv = level
	s.owner.SetExtraAttr(ref.SmithLevelAttrDef, int64(data.Lv))

	s.owner.TriggerQuestEvent(custom_id.QttSmithLv, s.GetSysId(), int64(data.Lv))
}

func (s *SmithSys) SetExp(exp int64) {
	ref, err := s.GetRef()
	if err != nil {
		return
	}

	data := s.GetSysData()
	data.Exp = exp
	s.owner.SetExtraAttr(ref.SmithExpAttrDef, data.Exp)
}

func (s *SmithSys) SetStage(stage uint32) {
	ref, err := s.GetRef()
	if err != nil {
		return
	}

	data := s.GetSysData()
	data.Stage = stage
	s.owner.SetExtraAttr(ref.SmithStageAttrDef, int64(data.Stage))
}

func (s *SmithSys) SetQuality(quality uint32) {
	ref, err := s.GetRef()
	if err != nil {
		return
	}

	data := s.GetSysData()
	data.Quality = quality
	s.owner.SetExtraAttr(ref.SmithQualityAttrDef, int64(data.Quality))

	s.owner.TriggerQuestEvent(custom_id.QttSmithQuality, s.GetSysId(), int64(data.Quality))
}

func (s *SmithSys) SetExAttr() {
	ref, err := s.GetRef()
	if err != nil {
		return
	}

	data := s.GetSysData()
	s.owner.SetExtraAttr(ref.SmithLevelAttrDef, int64(data.Lv))
	s.owner.SetExtraAttr(ref.SmithExpAttrDef, data.Exp)
	s.owner.SetExtraAttr(ref.SmithStageAttrDef, int64(data.Stage))
	s.owner.SetExtraAttr(ref.SmithQualityAttrDef, int64(data.Quality))
}

func (s *SmithSys) getQuestIdSet(qtt uint32) (set map[uint32]struct{}) {
	tMap, ok := smithQuestTargetMap[s.GetSysId()]
	if !ok {
		return
	}

	ids, ok := tMap[qtt]
	if !ok {
		return nil
	}

	data := s.GetSysData()

	set = make(map[uint32]struct{})
	for _, pri := range data.StageUpQuest {
		if _, exist := ids[pri.ConfId]; exist {
			set[pri.Id] = struct{}{}
		}
	}
	for _, pri := range data.DailyQuest {
		if _, exist := ids[pri.ConfId]; exist {
			set[pri.Id] = struct{}{}
		}
	}

	return
}

func (s *SmithSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	quest := s.findQuestById(id)
	if nil == quest {
		return nil
	}

	if quest.IsRev {
		return nil
	}

	return quest.Quest
}

func (s *SmithSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	questConf := s.getQuestConf(id)
	if nil == questConf {
		return nil
	}

	return questConf.Targets
}

func (s *SmithSys) getQuestConf(id uint32) *jsondata.SmithQuests {
	quest := s.findQuestById(id)
	if nil == quest {
		return nil
	}

	nextStage := s.GetSysData().Stage + 1

	switch quest.Type {
	case smithQuestTypeDaily:
		return s.GetConf().GetDailyQuestConf(quest.ConfId)
	case smithQuestTypeStage:
		return s.GetConf().GetStageQuestConf(nextStage, quest.ConfId)
	}

	return nil
}

func (s *SmithSys) onUpdateTargetData(id uint32) {
	q := s.getUnFinishQuestData(id)
	if nil == q {
		return
	}

	quest := s.findQuestById(id)
	if nil == quest {
		return
	}

	s.updateQuest(quest)
}

func (s *SmithSys) GetBagSys() (*SmithBagSys, error) {
	ref, err := s.GetRef()
	if nil != err {
		return nil, err
	}

	bagSys, ok := s.owner.GetSysObj(ref.BagSysId).(*SmithBagSys)
	if !ok {
		return nil, neterror.SysNotExistError("SmithBagSys get err")
	}

	return bagSys, nil
}

func (s *SmithSys) cast(count uint32, isAuto bool) error {
	conf := s.GetConf()
	if nil == conf {
		return errSmithConfNil
	}

	if !isAuto && count > 1 {
		return neterror.ParamsInvalidError("need auto")
	}

	ref, err := s.GetRef()
	if nil != err {
		return err
	}

	bagSys, err := s.GetBagSys()
	if err != nil {
		return err
	}

	if bagSys.AvailableCount() < count {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	sysData := s.GetSysData()

	lvConf, err := conf.GetSmithLvConf(sysData.Lv)
	if nil != err {
		return err
	}

	addAutoRate, _ := s.owner.GetPrivilege(privilegedef.PrivilegeType(ref.PrivilegeSmithAutoCastRate))
	if count > uint32(addAutoRate)+1 {
		return neterror.ParamsInvalidError("count limit")
	}

	if !s.owner.ConsumeRate(conf.CastConsume, int64(count), false, common.ConsumeParams{LogId: pb3.LogId_LogSmithCastConsume}) {
		return neterror.ParamsInvalidError("consume failed")
	}

	rewards := conf.GetCastAwards(sysData.Stage, sysData.Quality, count)
	rewards = jsondata.AppendStdReward(rewards, jsondata.StdRewardMulti(lvConf.CastRewards, int64(count)))

	engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSmithCastAwards})

	sysId := s.GetSysId()
	s.owner.TriggerEvent(custom_id.AeAddActivityHandBookExp, ref.SmithEquipDaTiePYYId, uint32(count))
	s.owner.TriggerQuestEvent(custom_id.QttSmithTimes, sysId, int64(count))
	s.owner.TriggerQuestEvent(custom_id.QttHistorySmithTimes, sysId, int64(count))

	return nil
}

func (s *SmithSys) dailyTaskCommit(id uint32) error {
	conf := s.GetConf()
	if nil == conf {
		return errSmithConfNil
	}

	quest := s.findQuestById(id)
	if nil == quest {
		return neterror.ParamsInvalidError("quest not exist")
	}

	dqConf := conf.GetDailyQuestConf(quest.ConfId)
	if nil == dqConf {
		return neterror.ParamsInvalidError("quest conf is nil")
	}

	if quest.IsRev {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	if !s.CheckFinishQuest(quest.Quest) {
		s.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	sysData := s.GetSysData()
	quest.IsRev = true
	sysData.DailyCompletTimes++

	engine.GiveRewards(s.owner, dqConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSmithDailyTaskAwards,
	})

	s.SendProto3(80, 32, &pb3.S2C_80_32{
		SysId:             s.GetSysId(),
		DailyCompletTimes: sysData.DailyCompletTimes,
	})

	s.updateQuest(quest)

	s.acceptDailyQuest()

	return nil
}

func (s *SmithSys) stageTaskCommit(id uint32) error {
	conf := s.GetConf()
	if nil == conf {
		return errSmithConfNil
	}

	quest := s.findQuestById(id)
	if nil == quest {
		return neterror.ParamsInvalidError("quest not exist")
	}

	if !s.CheckFinishQuest(quest.Quest) {
		return neterror.ParamsInvalidError("not complete")
	}

	sysData := s.GetSysData()

	sqConf := conf.GetStageQuestConf(sysData.Stage+1, quest.ConfId)
	if nil == sqConf {
		return neterror.ParamsInvalidError("quest conf is nil")
	}

	if quest.IsRev {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	quest.IsRev = true
	engine.GiveRewards(s.owner, sqConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSmithStageUpTaskAwards,
	})

	s.SendProto3(80, 33, &pb3.S2C_80_33{
		SysId: s.GetSysId(),
		Id:    id,
	})

	s.updateQuest(quest)

	return nil
}

func (s *SmithSys) updateStage(stage uint32) error {
	sysData := s.GetSysData()
	s.SetStage(stage)
	s.SendProto3(80, 35, &pb3.S2C_80_35{
		SysId: s.GetSysId(),
		Stage: sysData.Stage,
	})

	s.reCalAttr()

	ids := maps.Keys(sysData.StageUpQuest)
	sysData.StageUpQuest = nil

	s.SendProto3(80, 39, &pb3.S2C_80_39{
		SysId: s.GetSysId(),
		Ids:   ids,
	})

	s.acceptStageQuest()

	s.AddExp(0, pb3.LogId_LogSmithStageUpLift)
	return nil
}

func (s *SmithSys) stageUp() error {
	conf := s.GetConf()
	if nil == conf {
		return errSmithConfNil
	}

	sysData := s.GetSysData()

	nextStageConf, err := conf.GetSmithStageConf(sysData.Stage + 1)
	if err != nil {
		return err
	}

	for _, v := range nextStageConf.Quests {
		qOver := false
		for _, q := range sysData.StageUpQuest {
			if q.ConfId == v.QuestId && q.IsRev {
				qOver = true
				break
			}
		}
		if !qOver {
			return neterror.ParamsInvalidError("quest is nil or not rev")
		}
	}

	err = s.updateStage(sysData.Stage + 1)
	if err != nil {
		return err
	}
	return nil
}

func (s *SmithSys) enterQualityUp() error {
	sysData := s.GetSysData()

	nextQualityConf, err := s.GetConf().GetSmithQualityConf(sysData.Quality + 1)
	if nil != err {
		return err
	}

	if nextQualityConf.NeedStage > sysData.Stage {
		return neterror.ParamsInvalidError("stage not reach")
	}

	if sysData.QualityUpTimeOut > 0 {
		return neterror.ParamsInvalidError("is in countdown")
	}

	if !s.owner.ConsumeByConf(nextQualityConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogSmithQualityUpConsume,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	nowSec := time_util.NowSec()
	timeOut := nextQualityConf.Cd + nowSec
	sysData.QualityUpTimeOut = timeOut

	s.SendProto3(80, 37, &pb3.S2C_80_37{
		SysId:            s.GetSysId(),
		QualityUpTimeOut: timeOut,
	})

	s.setQualityUpTimer()
	return nil
}

func (s *SmithSys) setQualityUpTimer() {
	if nil != s.qualityUpTimer {
		s.qualityUpTimer.Stop()
	}

	nowSec := time_util.NowSec()
	sysData := s.GetSysData()

	if sysData.QualityUpTimeOut == 0 {
		return
	}

	var sec uint32 = 1

	if sysData.QualityUpTimeOut > nowSec {
		sec = sysData.QualityUpTimeOut - nowSec
	}

	tm := timer.SetTimeout(time.Duration(sec)*time.Second, func() {
		s.qualityUp()
	})

	s.qualityUpTimer = tm
}

func (s *SmithSys) qualityUp() {
	sysData := s.GetSysData()
	if sysData.QualityUpTimeOut == 0 {
		return
	}

	nowSec := time_util.NowSec()
	if sysData.QualityUpTimeOut > nowSec {
		return
	}

	if nil != s.qualityUpTimer {
		s.qualityUpTimer.Stop()
		s.qualityUpTimer = nil
	}

	sysData.QualityUpTimeOut = 0
	s.SendProto3(80, 37, &pb3.S2C_80_37{
		SysId:            s.GetSysId(),
		QualityUpTimeOut: sysData.QualityUpTimeOut,
	})

	s.SetQuality(sysData.Quality + 1)
	s.SendProto3(80, 38, &pb3.S2C_80_38{
		SysId:   s.GetSysId(),
		Quality: sysData.Quality,
	})
}

func (s *SmithSys) qualityUpSpeedUp() error {
	sysData := s.GetSysData()

	nextQualityConf, err := s.GetConf().GetSmithQualityConf(sysData.Quality + 1)
	if nil != err {
		return err
	}

	nowSec := time_util.NowSec()

	if sysData.QualityUpTimeOut == 0 || sysData.QualityUpTimeOut <= nowSec {
		return neterror.ParamsInvalidError("is over or not start")
	}

	if nextQualityConf.Cd > 0 {
		leftSec := sysData.QualityUpTimeOut - nowSec
		count := nextQualityConf.SpeedUpCount * leftSec / nextQualityConf.Cd
		if !s.owner.ConsumeByConf(jsondata.ConsumeVec{&jsondata.Consume{
			Id:    nextQualityConf.SpeedUpItemId,
			Count: count,
		}}, false, common.ConsumeParams{LogId: pb3.LogId_LogSmithQualityUpSpeedUpConsume}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	}

	sysData.QualityUpTimeOut = nowSec
	s.qualityUp()
	return nil
}

func (s *SmithSys) GMReAcceptQuest(questId uint32) {
	sysData := s.GetSysData()
	for _, v := range sysData.StageUpQuest {
		if v.ConfId != questId {
			continue
		}
		if v.IsRev {
			s.GmFinishQuest(v.Quest)
			continue
		}
		s.OnAcceptQuestAndCheckUpdateTarget(v.Quest)
	}
}

func (s *SmithSys) GMDelQuest(questId uint32) {
	sysData := s.GetSysData()
	var delIds []uint32
	for _, v := range sysData.StageUpQuest {
		if v.ConfId != questId {
			continue
		}
		delIds = append(delIds, v.Id)
	}

	for _, delId := range delIds {
		delete(sysData.StageUpQuest, delId)
	}

	s.SendProto3(80, 39, &pb3.S2C_80_39{
		SysId: s.GetSysId(),
		Ids:   delIds,
	})
}

func (s *SmithSys) reCalAttr() {
	ref, err := s.GetRef()
	if nil != err {
		return
	}
	s.ResetSysAttr(ref.CalAttrSmithDef)
}

func (s *SmithSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	sysData := s.GetSysData()

	if lvConf, err := conf.GetSmithLvConf(sysData.Lv); nil == err {
		engine.CheckAddAttrsToCalc(s.owner, calc, lvConf.Attrs)
	}

	if stageConf, err := conf.GetSmithStageConf(sysData.Stage); nil == err {
		engine.CheckAddAttrsToCalc(s.owner, calc, stageConf.Attrs)
	}
}

func regSmithSys() {
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		RegisterSysClass(ref.SmithSysId, func() iface.ISystem {
			return newSmithSys()
		})

		engine.RegAttrCalcFn(ref.CalAttrSmithDef, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
			s, ok := player.GetSysObj(ref.SmithSysId).(*SmithSys)
			if !ok || !s.IsOpen() {
				return
			}

			s.calcAttr(calc)
		})
	})
}

func GetSmithSys(player iface.IPlayer, sysId uint32) (*SmithSys, bool) {
	obj := player.GetSysObj(sysId)
	if obj == nil || !obj.IsOpen() {
		return nil, false
	}
	sys, ok := obj.(*SmithSys)
	if !ok || !sys.IsOpen() {
		return nil, false
	}
	return sys, true
}

func c2sCast(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_31
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.cast(req.Count, req.IsAuto)
	if err != nil {
		return err
	}
	return nil
}

func c2sDailyTaskCommit(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_32
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.dailyTaskCommit(req.Id)
	if err != nil {
		return err
	}
	return nil
}

func c2sStageTaskCommit(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_33
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.stageTaskCommit(req.Id)
	if err != nil {
		return err
	}
	return nil
}

func c2sStageUp(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_35
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.stageUp()
	if err != nil {
		return err
	}
	return nil
}

func c2sQualityUp(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_36
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.enterQualityUp()
	if err != nil {
		return err
	}
	return nil
}

func c2sQualityUpSpeedUp(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_37
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithSys(player, req.GetSysId())
	if !ok {
		return errSmithSysNil
	}

	err = sys.qualityUpSpeedUp()
	if err != nil {
		return err
	}
	return nil
}

var (
	smithQuestTargetMap = map[uint32]map[uint32]map[uint32]struct{}{} // 境界任务事件对应的id
)

func onAfterReloadSmithConf(args ...interface{}) {
	tmps := make(map[uint32]map[uint32]map[uint32]struct{})

	for _, conf := range jsondata.SmithConfigMgr {
		tmp := make(map[uint32]map[uint32]struct{})

		for _, v := range conf.DailyTask {
			for _, target := range v.Targets {
				if _, ok := tmp[target.Type]; !ok {
					tmp[target.Type] = make(map[uint32]struct{})
				}
				tmp[target.Type][v.QuestId] = struct{}{}
			}
		}

		for _, stageConf := range conf.StageConf {
			for _, v := range stageConf.Quests {
				for _, target := range v.Targets {
					if _, ok := tmp[target.Type]; !ok {
						tmp[target.Type] = make(map[uint32]struct{})
					}
					tmp[target.Type][v.QuestId] = struct{}{}
				}
			}
		}
		tmps[conf.SysId] = tmp
	}

	smithQuestTargetMap = tmps
}

func onSmithNewDay(player iface.IPlayer, args ...interface{}) {
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		s, ok := player.GetSysObj(ref.SmithSysId).(*SmithSys)
		if !ok || !s.IsOpen() {
			return
		}
		s.onNewDay()
	})
}

func handleQttSmithLv(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	sysId := ids[0]
	var lv uint32
	var find bool
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		if find {
			return
		}
		if ref.SmithSysId != sysId {
			return
		}
		sys, ok := GetSmithSys(actor, ref.SmithSysId)
		if !ok {
			return
		}
		lv = sys.GetSysData().Lv
		find = true
		return
	})
	return lv
}

func handleQttSmithQuality(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	sysId := ids[0]
	var quality uint32
	var find bool
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		if find {
			return
		}
		if ref.SmithSysId != sysId {
			return
		}
		sys, ok := GetSmithSys(actor, ref.SmithSysId)
		if !ok {
			return
		}
		quality = sys.GetSysData().Quality
		find = true
		return
	})
	return quality
}

func init() {
	regSmithSys()

	event.RegActorEvent(custom_id.AeNewDay, onSmithNewDay)

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadSmithConf)

	net.RegisterProto(80, 31, c2sCast)
	net.RegisterProto(80, 32, c2sDailyTaskCommit)
	net.RegisterProto(80, 33, c2sStageTaskCommit)
	net.RegisterProto(80, 35, c2sStageUp)
	net.RegisterProto(80, 36, c2sQualityUp)
	net.RegisterProto(80, 37, c2sQualityUpSpeedUp)
	engine.RegQuestTargetProgress(custom_id.QttSmithLv, handleQttSmithLv)
	engine.RegQuestTargetProgress(custom_id.QttSmithQuality, handleQttSmithQuality)

	gmevent.Register("smith.finishStageQuest", func(player iface.IPlayer, args ...string) bool {
		gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
			sys, ok := GetSmithSys(player, ref.SmithSysId)
			if !ok {
				return
			}
			for _, v := range sys.GetSysData().StageUpQuest {
				sys.GmFinishQuest(v.Quest)
			}
		})
		return true
	}, 1)
	gmevent.Register("smith.finishDailyQuest", func(player iface.IPlayer, args ...string) bool {
		gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
			sys, ok := GetSmithSys(player, ref.SmithSysId)
			if !ok {
				return
			}
			for _, v := range sys.GetSysData().DailyQuest {
				sys.GmFinishQuest(v.Quest)
			}
		})
		return true
	}, 1)
	gmevent.Register("smith.stage", func(player iface.IPlayer, args ...string) bool {
		stage := utils.AtoUint32(args[0])
		gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
			s, ok := GetSmithSys(player, ref.SmithSysId)
			if !ok {
				return
			}
			err := s.updateStage(stage)
			if err != nil {
				return
			}
		})
		return true
	}, 1)
}
