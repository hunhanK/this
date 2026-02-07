/**
 * @Author: lzp
 * @Date: 2023/11/18
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type FuncTrialSys struct {
	*QuestTargetBase
}

func newFuncTrialSys() iface.ISystem {
	sys := &FuncTrialSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *FuncTrialSys) OnLogin() {
	s.S2CFuncTrial()
}

func (s *FuncTrialSys) OnReconnect() {
	s.S2CFuncTrial()
}

func (s *FuncTrialSys) OnAfterLogin() {
}

func (s *FuncTrialSys) OnOpen() {
	s.init()
	s.S2CFuncTrial()
}

func (s *FuncTrialSys) PushTrialQuest(quest *pb3.QuestData) {
	msg := &pb3.S2C_7_42{}
	msg.Quest = quest
	s.SendProto3(7, 42, msg)
}

func (s *FuncTrialSys) S2CFuncTrial() {
	dataMap := s.GetData()
	s.SendProto3(7, 40, &pb3.S2C_7_40{
		TrialData: dataMap,
	})
}

func (s *FuncTrialSys) init() {
	confMgr := jsondata.GetFuncTrialMgr()
	for _, conf := range confMgr {
		s.OpenFuncTrial(conf.Id)
	}
}

func (s *FuncTrialSys) onUpdateTargetData(questId uint32) {
	_, tConf := jsondata.GetTrialQuestConf(questId)
	if tConf == nil {
		return
	}

	data := s.GetTrialData(tConf.Id)
	quest := s.GetQuestData(questId, data)
	if quest == nil {
		return
	}

	s.PushTrialQuest(quest)

	if !s.CheckFinishQuest(quest) {
		return
	}

	if !utils.SliceContainsUint32(data.PassQuestIds, questId) {
		return
	}

	data.PassQuestIds = append(data.PassQuestIds, questId)
}

func (s *FuncTrialSys) getQuestTargetConf(questId uint32) []*jsondata.QuestTargetConf {
	qConf, _ := jsondata.GetTrialQuestConf(questId)
	if qConf == nil {
		return nil
	}
	return qConf.Targets
}

func (s *FuncTrialSys) getQuestData(questId uint32) *pb3.QuestData {
	_, tConf := jsondata.GetTrialQuestConf(questId)
	if tConf == nil {
		return nil
	}
	data := s.GetTrialData(tConf.Id)
	for _, quest := range data.Quests {
		if questId == quest.GetId() {
			return quest
		}
	}
	return nil
}

func (s *FuncTrialSys) getQuestIdSet(questType uint32) map[uint32]struct{} {
	set := make(map[uint32]struct{})
	confMap := jsondata.GetFuncTrialMgr()
	if confMap == nil {
		return set
	}

	for id, conf := range confMap {
		data := s.GetTrialData(id)
		if data == nil {
			continue
		}
		for _, qConf := range conf.QuestList {
			if utils.SliceContainsUint32(data.FinishQuestIds, qConf.QuestId) {
				continue
			}
			for _, target := range qConf.Targets {
				if target.Type == questType {
					set[qConf.QuestId] = struct{}{}
				}
			}
		}
	}
	return set
}

func (s *FuncTrialSys) GetData() map[uint32]*pb3.FuncTrialData {
	binaryData := s.GetBinaryData()
	data := binaryData.TrialData
	if data == nil {
		data = make(map[uint32]*pb3.FuncTrialData)
		binaryData.TrialData = data
	}
	return data
}

func (s *FuncTrialSys) GetTrialData(id uint32) *pb3.FuncTrialData {
	dataMap := s.GetData()
	return dataMap[id]
}

func (s *FuncTrialSys) c2sRewardQuest(msg *base.Message) error {
	var req pb3.C2S_7_41
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	qConf, tConf := jsondata.GetTrialQuestConf(req.QuestId)
	if qConf == nil || tConf == nil {
		return neterror.ParamsInvalidError("quest=%d config error", req.QuestId)
	}

	data := s.GetTrialData(tConf.Id)
	if data == nil {
		return neterror.ParamsInvalidError("quest=%d not found", tConf.Id)
	}

	if utils.SliceContainsUint32(data.FinishQuestIds, req.QuestId) {
		return neterror.ParamsInvalidError("quest=%d rewards received", req.QuestId)
	}

	data.FinishQuestIds = append(data.FinishQuestIds, req.QuestId)

	// 发送奖励
	engine.GiveRewards(s.owner, qConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTrialQuestAward})

	s.owner.SendProto3(7, 41, &pb3.S2C_7_41{QuestId: req.QuestId, TrialId: tConf.Id})

	return nil
}

// 激活功能
func (s *FuncTrialSys) c2sActiveSysFunc(msg *base.Message) error {
	var req pb3.C2S_7_43
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	conf := jsondata.GetFuncTrialConf(req.TrialId)
	if conf == nil {
		return neterror.ConfNotFoundError("conf %d is nil", req.TrialId)
	}

	data := s.GetTrialData(req.TrialId)
	if data == nil {
		return neterror.ParamsInvalidError("quest=%d not found", req.TrialId)
	}

	if data.IsActive || len(data.Quests) != len(data.FinishQuestIds) {
		return neterror.ParamsInvalidError("function trialId=%d active error", req.TrialId)
	}

	data.IsActive = true

	s.owner.TriggerEvent(custom_id.AeActiveTrailFunc, req.TrialId)
	s.owner.TriggerQuestEventRange(custom_id.QttActiveFuncTrial)

	s.owner.SendProto3(7, 43, &pb3.S2C_7_43{TrialId: req.TrialId})

	engine.GiveRewards(s.owner, conf.QuestAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFuncTrialActiveAward})
	return nil
}

// CheckFuncTrial 判断某个试炼功能是否完成
func (s *FuncTrialSys) CheckFuncTrial(tid uint32) bool {
	data := s.GetTrialData(tid)
	if data == nil {
		return false
	}

	return data.IsActive
}

func (s *FuncTrialSys) GetQuestData(questId uint32, data *pb3.FuncTrialData) *pb3.QuestData {
	if data == nil {
		return nil
	}

	var quest *pb3.QuestData
	for _, v := range data.Quests {
		if v.Id == questId {
			quest = v
			break
		}
	}
	return quest
}

func (s *FuncTrialSys) OpenFuncTrial(id uint32) {
	conf := jsondata.GetFuncTrialConf(id)
	if conf == nil {
		return
	}

	data := s.GetTrialData(id)

	if data == nil {
		data = &pb3.FuncTrialData{
			TrialId:        id,
			Quests:         make([]*pb3.QuestData, 0, len(conf.QuestList)),
			FinishQuestIds: make([]uint32, 0),
			PassQuestIds:   make([]uint32, 0),
			IsActive:       false,
		}
		s.GetBinaryData().TrialData[id] = data
	}

	for _, v := range conf.QuestList {
		quest := &pb3.QuestData{}
		quest.Id = v.QuestId
		s.OnAcceptQuest(quest)
		data.Quests = append(data.Quests, quest)
		if s.CheckFinishQuest(quest) {
			data.PassQuestIds = append(data.PassQuestIds, quest.Id)
		}
	}
}

func (s *FuncTrialSys) GMReAcceptQuest(questId uint32) {
	data := s.GetData()
	for _, trialData := range data {
		for _, quest := range trialData.Quests {
			if quest.Id == questId {
				trialData.FinishQuestIds = pie.Uint32s(trialData.FinishQuestIds).Filter(func(u uint32) bool {
					return u != questId
				})
				trialData.PassQuestIds = pie.Uint32s(trialData.PassQuestIds).Filter(func(u uint32) bool {
					return u != questId
				})
				s.OnAcceptQuestAndCheckUpdateTarget(quest)
				break
			}
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiFuncTrial, newFuncTrialSys)

	net.RegisterSysProto(7, 41, sysdef.SiFuncTrial, (*FuncTrialSys).c2sRewardQuest)
	net.RegisterSysProto(7, 43, sysdef.SiFuncTrial, (*FuncTrialSys).c2sActiveSysFunc)

	engine.RegQuestTargetProgress(custom_id.QttActiveFuncTrial, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) <= 0 {
			return 0
		}
		sys := actor.GetSysObj(sysdef.SiFuncTrial)
		if sys == nil {
			return 0
		}
		s, ok := sys.(*FuncTrialSys)
		if !ok {
			return 0
		}
		isFinish := false
		for _, trialId := range ids {
			if s.CheckFuncTrial(trialId) {
				isFinish = true
			}
		}
		if isFinish {
			return 1
		}

		return 0
	})

	initFuncTrialSysGm()
}

func initFuncTrialSysGm() {
	gmevent.Register("trial.finish", func(player iface.IPlayer, args ...string) bool {
		if len(args) <= 0 {
			return false
		}

		conf := jsondata.GetFuncTrialConf(utils.AtoUint32(args[0]))
		if conf == nil {
			return false
		}

		sys, ok := player.GetSysObj(sysdef.SiFuncTrial).(*FuncTrialSys)
		if !ok {
			return false
		}

		data := sys.GetTrialData(conf.Id)

		for _, questConf := range conf.QuestList {
			questData := sys.GetQuestData(questConf.QuestId, data)
			if questData == nil {
				continue
			}
			questData.Progress = make([]uint32, 0)
			for _, v := range questConf.Targets {
				questData.Progress = append(questData.Progress, v.Count)
			}
			data.PassQuestIds = append(data.PassQuestIds, questConf.QuestId)

			sys.PushTrialQuest(questData)
		}
		return true
	}, 1)

	gmevent.Register("trialquest.finish", func(player iface.IPlayer, args ...string) bool {
		if len(args) <= 0 {
			return false
		}

		qConf, tConf := jsondata.GetTrialQuestConf(utils.AtoUint32(args[0]))

		if qConf == nil || tConf == nil {
			return false
		}

		sys, ok := player.GetSysObj(sysdef.SiFuncTrial).(*FuncTrialSys)
		if !ok {
			return false
		}

		data := sys.GetTrialData(tConf.Id)
		questData := sys.GetQuestData(qConf.QuestId, data)
		if questData == nil {
			return false
		}
		questData.Progress = make([]uint32, 0)
		for _, v := range qConf.Targets {
			questData.Progress = append(questData.Progress, v.Count)
		}
		data.PassQuestIds = append(data.PassQuestIds, qConf.QuestId)
		sys.PushTrialQuest(questData)

		return true
	}, 1)
}
