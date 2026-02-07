/**
 * @Author: zjj
 * @Date: 2024/12/16
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type GuidelineGoalSys struct {
	*QuestTargetBase
}

func createGuidelineGoalSys() iface.ISystem {
	sys := &GuidelineGoalSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *GuidelineGoalSys) s2cInfo() {
	s.SendProto3(39, 10, &pb3.S2C_39_10{
		Data: s.getData(),
	})
}

func (s *GuidelineGoalSys) getData() *pb3.GuidelineGoalData {
	data := s.GetBinaryData().GuidelineGoalData
	if data == nil {
		s.GetBinaryData().GuidelineGoalData = &pb3.GuidelineGoalData{}
		data = s.GetBinaryData().GuidelineGoalData
	}
	if data.QuestMap == nil {
		data.QuestMap = make(map[uint32]*pb3.QuestDataWithFinishReceive)
	}
	return data
}

func (s *GuidelineGoalSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GuidelineGoalSys) OnAfterLogin() {
	s.s2cInfo()
	s.checkAcceptNewQuest()
}

func (s *GuidelineGoalSys) OnOpen() {
	s.s2cInfo()
	s.checkAcceptNewQuest()
}

func (s *GuidelineGoalSys) c2sRec(msg *base.Message) error {
	var req pb3.C2S_39_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	return s.recQuestAwards(req.QuestId)
}

func (s *GuidelineGoalSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	mgr := jsondata.GetGuidelineGoalQuestMgr()
	if mgr == nil {
		return set
	}
	for _, conf := range mgr {
		var exist bool
		for _, target := range conf.Targets {
			if target.Type != qtt {
				continue
			}
			exist = true
			break
		}
		if exist {
			set[conf.Id] = struct{}{}
		}
	}
	return set
}

func (s *GuidelineGoalSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()

	fData, ok := data.QuestMap[id]
	if !ok {
		return nil
	}

	if s.CheckFinishQuest(fData.Quest) {
		return nil
	}

	return fData.Quest
}

func (s *GuidelineGoalSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetGuidelineGoalQuestConf(id)
	if conf == nil {
		return nil
	}
	return conf.Targets
}

func (s *GuidelineGoalSys) onUpdateTargetData(id uint32) {
	data := s.getData()
	fData, ok := data.QuestMap[id]
	if !ok {
		return
	}
	if !fData.IsFinished && s.CheckFinishQuest(fData.Quest) {
		fData.IsFinished = true
	}
	s.SendProto3(39, 13, &pb3.S2C_39_13{
		Quest: fData,
	})
}

func (s *GuidelineGoalSys) checkQuestCond(cond *jsondata.GuidelineGoalCond) bool {
	owner := s.GetOwner()
	if cond.Level != 0 && owner.GetLevel() < cond.Level {
		return false
	}

	if cond.OpenServerDay != 0 && gshare.GetOpenServerDay() < cond.OpenServerDay {
		return false
	}

	if len(cond.CrossTimesAndDay) >= 2 {
		times := cond.CrossTimesAndDay[0]
		days := cond.CrossTimesAndDay[1]
		crossAllocTimes := gshare.GetCrossAllocTimes()
		smallCrossDay := gshare.GetSmallCrossDay()
		switch {
		case crossAllocTimes == times:
			if smallCrossDay < days {
				return false
			}
		case crossAllocTimes < times:
			return false
		}
	}

	if len(cond.MergeTimesAndDay) >= 2 {
		times := cond.MergeTimesAndDay[0]
		days := cond.MergeTimesAndDay[1]
		mergeTimes := gshare.GetMergeTimes()
		mergeSrvDay := gshare.GetMergeSrvDay()
		switch {
		case mergeTimes == times:
			if mergeSrvDay < days {
				return false
			}
		case mergeTimes < times:
			return false
		}
	}
	return true
}

func (s *GuidelineGoalSys) checkAcceptNewQuest() {
	owner := s.GetOwner()
	data := s.getData()
	var newIds []uint32
	jsondata.EachGuidelineGoalQuestMgr(func(id uint32, questConf *jsondata.GuidelineGoalQuestConf) {
		if _, ok := data.QuestMap[id]; ok {
			return
		}
		if questConf.Cond != nil && !s.checkQuestCond(questConf.Cond) {
			return
		}
		newIds = append(newIds, id)
	})
	if len(newIds) == 0 {
		return
	}
	var resp pb3.S2C_39_12
	for _, newId := range newIds {
		newQuest := &pb3.QuestDataWithFinishReceive{
			Quest: &pb3.QuestData{
				Id: newId,
			},
		}
		resp.Quests = append(resp.Quests, newQuest)
		data.QuestMap[newId] = newQuest
		s.OnAcceptQuestAndCheckUpdateTarget(newQuest.Quest)
	}
	owner.SendProto3(39, 12, &resp)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogGuidelineGoalAcceptNewQuest, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%v", newIds),
	})
}

func (s *GuidelineGoalSys) recQuestAwards(questId uint32) error {
	quest := jsondata.GetGuidelineGoalQuestConf(questId)
	if quest == nil {
		return neterror.ConfNotFoundError("%d not found quest conf", questId)
	}

	data := s.getData()
	fData, ok := data.QuestMap[questId]
	if !ok {
		return neterror.ConfNotFoundError("%d not accept quest data", questId)
	}

	if !fData.IsFinished {
		return neterror.ParamsInvalidError("%d quest is finish", questId)
	}

	if fData.IsReceived {
		return neterror.ParamsInvalidError("%d quest is received", questId)
	}

	owner := s.GetOwner()
	if len(quest.Awards) != 0 {
		engine.GiveRewards(owner, quest.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGuidelineGoalRecAwards})
	}

	fData.IsReceived = true
	s.SendProto3(39, 11, &pb3.S2C_39_11{
		QuestId: questId,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogGuidelineGoalRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(questId),
	})
	return nil
}

func (s *GuidelineGoalSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	fData, ok := data.QuestMap[questId]
	if !ok {
		s.owner.LogError("not found %d quest", questId)
		return
	}
	if fData.IsFinished || fData.IsReceived {
		s.GmFinishQuest(fData.Quest)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(fData.Quest)
}

func (s *GuidelineGoalSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.QuestMap, questId)

}

func init() {
	RegisterSysClass(sysdef.SiGuidelineGoal, createGuidelineGoalSys)
	net.RegisterSysProtoV2(39, 11, sysdef.SiGuidelineGoal, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GuidelineGoalSys).c2sRec
	})
	event.RegActorEvent(custom_id.AeNewDay, onEventCheckGuidelineGoal)
	event.RegActorEvent(custom_id.AeLevelUp, onEventCheckGuidelineGoal)
	event.RegActorEvent(custom_id.AeAfterMerge, onEventCheckGuidelineGoal)
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, onSysEventCheckGuidelineGoal)
	gmevent.Register("GuidelineGoalSys.finishAll", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiGuidelineGoal)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*GuidelineGoalSys)
		data := sys.getData()
		for _, receive := range data.QuestMap {
			sys.GmFinishQuest(receive.Quest)
		}
		sys.s2cInfo()
		return true
	}, 1)
}

func onSysEventCheckGuidelineGoal(_ ...interface{}) {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		onEventCheckGuidelineGoal(player)
	})
}

func onEventCheckGuidelineGoal(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiGuidelineGoal)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*GuidelineGoalSys)
	sys.checkAcceptNewQuest()
}
