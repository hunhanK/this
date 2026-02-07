/**
 * @Author: lzp
 * @Date: 2024/12/16
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FairySwordGoal struct {
	*YYQuestTargetBase
}

func (s *FairySwordGoal) OnOpen() {
	s.initQuestData()
	s.s2cInfo()
}

func (s *FairySwordGoal) Login() {
	s.s2cInfo()
}

func (s *FairySwordGoal) OnEnd() {
	s.trySendMailRewards()
}

func (s *FairySwordGoal) OnReconnect() {
	s.s2cInfo()
}

func (s *FairySwordGoal) initQuestData() {
	data := s.getData()
	if len(data.QuestData) > 0 {
		return
	}

	conf := jsondata.GetFairySwordGoalConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	for qId := range conf.Quests {
		qData := &pb3.QuestData{Id: qId}
		s.OnAcceptQuest(qData)
		data.QuestData[qId] = qData
		s.onUpdateTargetData(qData.Id)
	}
}

func (s *FairySwordGoal) trySendMailRewards() {
	conf := jsondata.GetFairySwordGoalConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.getData()

	var awards jsondata.StdRewardVec

	var checkCanRecQuestRewards = func(qConf *jsondata.FairySwordGoalQuest) bool {
		data := s.getData()
		if pie.Uint32s(data.FinishIds).Contains(qConf.Id) {
			return true
		}
		return false
	}

	var checkCanRecPointRewards = func(pConf *jsondata.FairySwordGoalPoint) bool {
		data := s.getData()
		if data.Point >= pConf.Point {
			return true
		}
		return false
	}

	// 没领取的任务奖励
	for _, qConf := range conf.Quests {
		if !pie.Uint32s(data.ReceiveIds).Contains(qConf.Id) && checkCanRecQuestRewards(qConf) {
			awards = append(awards, qConf.Awards...)
			data.RecRewardIds = append(data.ReceiveIds, qConf.Id)
			data.Point += qConf.Point
		}
	}

	// 没领取的积分奖励
	for _, pConf := range conf.PointRewards {
		if !pie.Uint32s(data.RecRewardIds).Contains(pConf.Point) && checkCanRecPointRewards(pConf) {
			awards = append(awards, pConf.Awards...)
			data.RecRewardIds = append(data.ReceiveIds, pConf.Point)
		}
	}

	// 邮件补发礼包奖励
	if len(awards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(common.Mail_PYYFairySwordGoalAwards),
			Rewards: awards,
		})
	}
}

func (s *FairySwordGoal) getData() *pb3.PYY_FairySwordGoal {
	state := s.GetYYData()
	if state.FairySwordGoal == nil {
		state.FairySwordGoal = make(map[uint32]*pb3.PYY_FairySwordGoal)
	}
	if state.FairySwordGoal[s.Id] == nil {
		state.FairySwordGoal[s.Id] = &pb3.PYY_FairySwordGoal{}
	}
	data := state.FairySwordGoal[s.Id]
	if data.QuestData == nil {
		data.QuestData = make(map[uint32]*pb3.QuestData)
	}
	return data
}

func (s *FairySwordGoal) s2cInfo() {
	s.SendProto3(127, 120, &pb3.S2C_127_120{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *FairySwordGoal) ResetData() {
	state := s.GetYYData()
	if state.FairySwordGoal == nil {
		return
	}
	delete(state.FairySwordGoal, s.Id)
}

// 任务相关回调
func (s *FairySwordGoal) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetFairySwordGoalConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	if qConf, ok := conf.Quests[id]; ok {
		return qConf.Targets
	}
	return nil
}

func (s *FairySwordGoal) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	conf := jsondata.GetFairySwordGoalConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	idSet := make(map[uint32]struct{})
	for _, qConf := range conf.Quests {
		for _, tConf := range qConf.Targets {
			if tConf.Type != qtt {
				continue
			}
			idSet[qConf.Id] = struct{}{}
			break
		}
	}
	return idSet
}

func (s *FairySwordGoal) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	if pie.Uint32s(data.FinishIds).Contains(id) {
		return nil
	}
	if qData, ok := data.QuestData[id]; ok {
		return qData
	}
	return nil
}

func (s *FairySwordGoal) onUpdateTargetData(id uint32) {
	qData := s.getUnFinishQuestData(id)
	if qData == nil {
		return
	}

	data := s.getData()
	pbMsg := &pb3.S2C_127_123{ActId: s.GetId(), Quest: qData}
	if s.CheckFinishQuest(qData) {
		data.FinishIds = append(data.FinishIds, id)
		pbMsg.FinishQId = id
	}

	s.SendProto3(127, 123, pbMsg)
}

// 协议相关
func (s *FairySwordGoal) c2sFetchPointAwards(msg *base.Message) error {
	var req pb3.C2S_127_121
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	pointId := req.PointId
	pConf := jsondata.GetFairySwordGoalPConf(s.ConfName, s.ConfIdx, pointId)
	if pConf == nil {
		return neterror.ConfNotFoundError("point config not found, id:%d", pointId)
	}

	data := s.getData()
	if pie.Uint32s(data.RecRewardIds).Contains(pointId) {
		return neterror.ParamsInvalidError("point rewards fetched, id:%d", pointId)
	}
	if data.Point < pConf.Point {
		return neterror.ParamsInvalidError("point not satisfy, id:%d", pointId)
	}

	data.RecRewardIds = append(data.RecRewardIds, pointId)
	if len(pConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), pConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYFairySwordGoalPointRewards,
		})
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYFairySwordGoalPointRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", pointId),
	})

	s.SendProto3(127, 121, &pb3.S2C_127_121{
		ActId:   s.GetId(),
		PointId: pointId,
	})

	return nil
}

func (s *FairySwordGoal) c2sFetchQuestAwards(msg *base.Message) error {
	var req pb3.C2S_127_122
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	questId := req.QId
	qConf := jsondata.GetFairySwordGoalQConf(s.ConfName, s.ConfIdx, questId)
	if qConf == nil {
		return neterror.ConfNotFoundError("quest config not found, id:%d", questId)
	}

	data := s.getData()
	if !pie.Uint32s(data.FinishIds).Contains(questId) {
		return neterror.ParamsInvalidError("quest not finish, id:%d", questId)
	}
	if pie.Uint32s(data.ReceiveIds).Contains(questId) {
		return neterror.ParamsInvalidError("quest rewards fetched, id:%d", questId)
	}

	data.ReceiveIds = append(data.ReceiveIds, questId)
	data.Point += qConf.Point
	if len(qConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), qConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYFairySwordGoalQuestRewards,
		})
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYFairySwordGoalQuestRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", questId),
	})

	s.SendProto3(127, 122, &pb3.S2C_127_122{
		ActId: s.GetId(),
		QId:   questId,
		Point: data.Point,
	})

	return nil
}

func newFairySwordGoal() iface.IPlayerYY {
	obj := &FairySwordGoal{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYFairySwordGoal, newFairySwordGoal)
	net.RegisterYYSysProtoV2(127, 121, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FairySwordGoal).c2sFetchPointAwards
	})
	net.RegisterYYSysProtoV2(127, 122, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FairySwordGoal).c2sFetchQuestAwards
	})
	gmevent.Register("fairySwordGoal.completeQuest", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYFairySwordGoal, func(obj iface.IPlayerYY) {
			goal := obj.(*FairySwordGoal)
			if goal == nil {
				return
			}
			data := goal.getData()
			for _, questData := range data.QuestData {
				goal.GmFinishQuest(questData)
				goal.onUpdateTargetData(questData.Id)
			}
			goal.s2cInfo()
		})
		return true
	}, 1)
}
