/**
 * @Author: zjj
 * @Date: 2025年8月6日
 * @Desc: 七夕拼图
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

type QiXiPuzzleSys struct {
	*YYQuestTargetBase
}

func (s *QiXiPuzzleSys) OnOpen() {
	s.initQuestData()
	s.s2cInfo()
}

func (s *QiXiPuzzleSys) Login() {
	s.s2cInfo()
}

func (s *QiXiPuzzleSys) OnEnd() {
	s.trySendMailAwards()
}

func (s *QiXiPuzzleSys) OnReconnect() {
	s.s2cInfo()
}

func (s *QiXiPuzzleSys) getData() *pb3.PYYQiXiPuzzle {
	state := s.GetYYData()
	if state.QiXiPuzzle == nil {
		state.QiXiPuzzle = make(map[uint32]*pb3.PYYQiXiPuzzle)
	}
	if state.QiXiPuzzle[s.Id] == nil {
		state.QiXiPuzzle[s.Id] = &pb3.PYYQiXiPuzzle{}
	}
	data := state.QiXiPuzzle[s.Id]
	if data.QuestData == nil {
		data.QuestData = make(map[uint32]*pb3.QuestData)
	}
	return data
}

func (s *QiXiPuzzleSys) s2cInfo() {
	s.SendProto3(9, 90, &pb3.S2C_9_90{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *QiXiPuzzleSys) initQuestData() {
	data := s.getData()
	if len(data.QuestData) > 0 {
		return
	}

	conf := jsondata.GetPYYQiXiPuzzleConfig(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	for qId := range conf.Quests {
		qData := &pb3.QuestData{Id: qId}
		s.OnAcceptQuest(qData)
		data.QuestData[qId] = qData
	}
}

func (s *QiXiPuzzleSys) trySendMailAwards() {
	conf := jsondata.GetPYYQiXiPuzzleConfig(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.getData()
	var checkGiftCanFetch = func(gConf *jsondata.QiXiPuzzleGiftRewards) bool {
		canFetch := true
		for _, qId := range gConf.GridIds {
			if !pie.Uint32s(data.GridIds).Contains(qId) {
				canFetch = false
				break
			}
		}
		return canFetch
	}

	var awards jsondata.StdRewardVec
	for _, gConf := range conf.GiftRewards {
		if !pie.Uint32s(data.ReceiveGiftIds).Contains(gConf.Id) && checkGiftCanFetch(gConf) {
			awards = append(awards, gConf.Awards...)
			data.ReceiveGiftIds = append(data.ReceiveGiftIds, gConf.Id)
		}
	}

	// 邮件补发礼包奖励
	if len(awards) > 0 {
		awards = jsondata.MergeStdReward(awards)
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: awards,
		})
	}
}

func (s *QiXiPuzzleSys) ResetData() {
	state := s.GetYYData()
	if state.QiXiPuzzle == nil {
		return
	}
	delete(state.QiXiPuzzle, s.Id)
}

// 任务相关回调
func (s *QiXiPuzzleSys) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetPYYQiXiPuzzleConfig(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	if qConf, ok := conf.Quests[id]; ok {
		return qConf.Targets
	}
	return nil
}

func (s *QiXiPuzzleSys) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	conf := jsondata.GetPYYQiXiPuzzleConfig(s.ConfName, s.ConfIdx)
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

func (s *QiXiPuzzleSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	qData, ok := data.QuestData[id]
	if !ok {
		return nil
	}
	if !s.CheckFinishQuest(qData) {
		return qData
	}
	return nil
}

func (s *QiXiPuzzleSys) onUpdateTargetData(id uint32) {
	qData := s.getData().QuestData[id]
	if qData == nil {
		return
	}
	s.SendProto3(9, 94, &pb3.S2C_9_94{ActId: s.GetId(), Quest: qData})
}

// 协议相关
func (s *QiXiPuzzleSys) c2sFetchQuestAwards(msg *base.Message) error {
	var req pb3.C2S_9_91
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	questId := req.QuestId
	qConf := jsondata.GetPYYQiXiPuzzleQuestConf(s.ConfName, s.ConfIdx, questId)
	if qConf == nil {
		return neterror.ConfNotFoundError("quest not found, id:%d", questId)
	}

	data := s.getData()
	questData, ok := data.QuestData[questId]
	if !ok {
		return neterror.ParamsInvalidError("%d quest not accept", questId)
	}

	if !s.CheckFinishQuest(questData) {
		return neterror.ParamsInvalidError("quest not finish, id:%d", questId)
	}

	if pie.Uint32s(data.ReceiveIds).Contains(questId) {
		return neterror.ParamsInvalidError("quest rewards fetched, id:%d", questId)
	}

	data.ReceiveIds = append(data.ReceiveIds, questId)
	if len(qConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), qConf.Awards, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogQiXiPuzzleQuestRewards,
			NoTips: true,
		})
		s.GetPlayer().SendShowRewardsPop(qConf.Awards)
	}

	s.SendProto3(9, 91, &pb3.S2C_9_91{
		ActId:   s.GetId(),
		QuestId: questId,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogQiXiPuzzleQuestRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", questId),
	})
	return nil
}

func (s *QiXiPuzzleSys) c2sFetchGiftAwards(msg *base.Message) error {
	var req pb3.C2S_9_92
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	giftId := req.GiftId
	gConf := jsondata.GetPYYQiXiPuzzleGiftConf(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return neterror.ConfNotFoundError("gift not found, id:%d", giftId)
	}

	data := s.getData()
	if pie.Uint32s(data.ReceiveGiftIds).Contains(giftId) {
		return neterror.ParamsInvalidError("gift rewards fetched, id:%d", giftId)
	}

	canFetch := true
	for _, qId := range gConf.GridIds {
		if !pie.Uint32s(data.GridIds).Contains(qId) {
			canFetch = false
		}
	}
	if !canFetch {
		return neterror.ParamsInvalidError("gift cannot fetch, id:%d", giftId)
	}

	data.ReceiveGiftIds = append(data.ReceiveGiftIds, giftId)
	if len(gConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Awards, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogQiXiPuzzleGiftRewards,
			NoTips: true,
		})
		s.GetPlayer().SendShowRewardsPop(gConf.Awards)
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogQiXiPuzzleGiftRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", giftId),
	})

	s.SendProto3(9, 92, &pb3.S2C_9_92{
		ActId:  s.GetId(),
		GiftId: giftId,
	})
	return nil
}

func (s *QiXiPuzzleSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	if qData, ok := data.QuestData[questId]; ok {
		if s.CheckFinishQuest(qData) {
			s.GmFinishQuest(qData)
			return
		}
		s.OnAcceptQuestAndCheckUpdateTarget(qData)
		return
	}
}

func (s *QiXiPuzzleSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.QuestData, questId)
}

func (s *QiXiPuzzleSys) c2sFetchGrid(msg *base.Message) error {
	var req pb3.C2S_9_93
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}
	gridId := req.GridId

	gConf := jsondata.GetPYYQiXiPuzzleGridConf(s.ConfName, s.ConfIdx, gridId)
	if gConf == nil {
		return neterror.ConfNotFoundError("grid not found, id:%d", gridId)
	}

	data := s.getData()
	if pie.Uint32s(data.GridIds).Contains(gridId) {
		return neterror.ParamsInvalidError("grid rewards fetched, id:%d", gridId)
	}

	player := s.GetPlayer()
	if len(gConf.Consume) == 0 || !player.ConsumeByConf(gConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogQiXiPuzzleGrid}) {
		return neterror.ConsumeFailedError("consume failed %d", gridId)
	}

	data.GridIds = append(data.GridIds, gridId)
	if len(gConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogQiXiPuzzleGrid,
		})
		s.GetPlayer().SendShowRewardsPop(gConf.Awards)
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogQiXiPuzzleGrid, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", gridId),
	})

	s.SendProto3(9, 93, &pb3.S2C_9_93{
		ActId:  s.GetId(),
		GridId: gridId,
	})
	return nil
}

func newQiXiPuzzleSys() iface.IPlayerYY {
	obj := &QiXiPuzzleSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYQiXiPuzzle, newQiXiPuzzleSys)
	net.RegisterYYSysProtoV2(9, 91, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*QiXiPuzzleSys).c2sFetchQuestAwards
	})
	net.RegisterYYSysProtoV2(9, 92, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*QiXiPuzzleSys).c2sFetchGiftAwards
	})
	net.RegisterYYSysProtoV2(9, 93, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*QiXiPuzzleSys).c2sFetchGrid
	})
	gmevent.Register("PYYQiXiPuzzle.finishAll", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYQiXiPuzzle, func(obj iface.IPlayerYY) {
			sys := obj.(*QiXiPuzzleSys)
			if sys == nil {
				return
			}
			data := sys.getData()
			for _, questData := range data.QuestData {
				sys.GmFinishQuest(questData)
			}
			sys.s2cInfo()
		})
		return true
	}, 1)
}
