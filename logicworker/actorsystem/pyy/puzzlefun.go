/**
 * @Author: lzp
 * @Date: 2024/8/19
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
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

type PuzzleFunSys struct {
	*YYQuestTargetBase
}

func (s *PuzzleFunSys) OnOpen() {
	s.initQuestData()
	s.s2cInfo()
}

func (s *PuzzleFunSys) Login() {
	s.s2cInfo()
}

func (s *PuzzleFunSys) OnEnd() {
	s.trySendMailAwards()
}

func (s *PuzzleFunSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PuzzleFunSys) getData() *pb3.PYY_PuzzleFun {
	state := s.GetYYData()
	if state.PuzzleFun == nil {
		state.PuzzleFun = make(map[uint32]*pb3.PYY_PuzzleFun)
	}
	if state.PuzzleFun[s.Id] == nil {
		state.PuzzleFun[s.Id] = &pb3.PYY_PuzzleFun{}
	}
	data := state.PuzzleFun[s.Id]
	if data.QuestData == nil {
		data.QuestData = make(map[uint32]*pb3.QuestData)
	}
	return data
}

func (s *PuzzleFunSys) s2cInfo() {
	s.SendProto3(127, 105, &pb3.S2C_127_105{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *PuzzleFunSys) initQuestData() {
	data := s.getData()
	if len(data.QuestData) > 0 {
		return
	}

	conf := jsondata.GetPuzzleFunConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	for qId := range conf.Quests {
		qData := &pb3.QuestData{Id: qId}
		s.OnAcceptQuest(qData)
		data.QuestData[qId] = qData
	}
}

func (s *PuzzleFunSys) trySendMailAwards() {
	conf := jsondata.GetPuzzleFunConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.getData()
	var checkGiftCanFetch = func(gConf *jsondata.PuzzleFunReward) bool {
		canFetch := true
		for _, qId := range gConf.QuestIds {
			if !pie.Uint32s(data.FinishIds).Contains(qId) {
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
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(common.Mail_PYYPuzzleFunGiftAwards),
			Rewards: awards,
		})
	}
}

func (s *PuzzleFunSys) ResetData() {
	state := s.GetYYData()
	if state.PuzzleFun == nil {
		return
	}
	delete(state.PuzzleFun, s.Id)
}

func getPuzzleFunSys(player iface.IPlayer) *PuzzleFunSys {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYPuzzleFun)
	for i := range yyList {
		sys, ok := yyList[i].(*PuzzleFunSys)
		if ok && sys.IsOpen() {
			return sys
		}
	}
	return nil
}

// 任务相关回调
func (s *PuzzleFunSys) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetPuzzleFunConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	if qConf, ok := conf.Quests[id]; ok {
		return qConf.Targets
	}
	return nil
}

func (s *PuzzleFunSys) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	conf := jsondata.GetPuzzleFunConf(s.ConfName, s.ConfIdx)
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

func (s *PuzzleFunSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	if pie.Uint32s(data.FinishIds).Contains(id) {
		return nil
	}
	if qData, ok := data.QuestData[id]; ok {
		return qData
	}
	return nil
}

func (s *PuzzleFunSys) onUpdateTargetData(id uint32) {
	qData := s.getUnFinishQuestData(id)
	if qData == nil {
		return
	}

	data := s.getData()
	pbMsg := &pb3.S2C_127_108{ActId: s.GetId(), Quest: qData}
	if s.CheckFinishQuest(qData) {
		data.FinishIds = append(data.FinishIds, id)
		pbMsg.FinishQId = id
	}

	s.SendProto3(127, 108, pbMsg)
}

// 协议相关
func (s *PuzzleFunSys) c2sFetchQuestAwards(msg *base.Message) error {
	var req pb3.C2S_127_106
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	questId := req.QuestId
	qConf := jsondata.GetPuzzleFunQuestConf(s.ConfName, s.ConfIdx, questId)
	if qConf == nil {
		return neterror.ConfNotFoundError("quest not found, id:%d", questId)
	}

	data := s.getData()
	if !pie.Uint32s(data.FinishIds).Contains(questId) {
		return neterror.ParamsInvalidError("quest not finish, id:%d", questId)
	}
	if pie.Uint32s(data.ReceiveIds).Contains(questId) {
		return neterror.ParamsInvalidError("quest rewards fetched, id:%d", questId)
	}

	data.ReceiveIds = append(data.ReceiveIds, questId)
	if len(qConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), qConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYPuzzleFunQuestRewards,
		})
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYPuzzleFunQuestRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", questId),
	})

	s.SendProto3(127, 106, &pb3.S2C_127_106{
		ActId:   s.GetId(),
		QuestId: questId,
	})
	return nil
}

func (s *PuzzleFunSys) c2sFetchGiftAwards(msg *base.Message) error {
	var req pb3.C2S_127_107
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	giftId := req.GiftId
	gConf := jsondata.GetPuzzleFunGiftConf(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return neterror.ConfNotFoundError("gift not found, id:%d", giftId)
	}

	data := s.getData()
	if pie.Uint32s(data.ReceiveGiftIds).Contains(giftId) {
		return neterror.ParamsInvalidError("gift rewards fetched, id:%d", giftId)
	}
	canFetch := true
	for _, qId := range gConf.QuestIds {
		if !pie.Uint32s(data.FinishIds).Contains(qId) {
			canFetch = false
		}
	}
	if !canFetch {
		return neterror.ParamsInvalidError("gift cannot fetch, id:%d", giftId)
	}

	data.ReceiveGiftIds = append(data.ReceiveGiftIds, giftId)
	if len(gConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYPuzzleFunGiftRewards,
		})
	}
	engine.BroadcastTipMsgById(tipmsgid.PYYPuzzleFunGiftAwards, s.GetPlayer().GetId(), s.GetPlayer().GetName())

	s.SendProto3(127, 107, &pb3.S2C_127_107{
		ActId:  s.GetId(),
		GiftId: giftId,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYPuzzleFunGiftRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", giftId),
	})
	return nil
}

func (s *PuzzleFunSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	if qData, ok := data.QuestData[questId]; ok {
		if pie.Uint32s(data.FinishIds).Contains(questId) {
			s.GmFinishQuest(qData)
			return
		}
		s.OnAcceptQuestAndCheckUpdateTarget(qData)
		return
	}
}

func (s *PuzzleFunSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.QuestData, questId)
}

func newPuzzleFunSys() iface.IPlayerYY {
	obj := &PuzzleFunSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYPuzzleFun, newPuzzleFunSys)
	net.RegisterYYSysProtoV2(127, 106, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PuzzleFunSys).c2sFetchQuestAwards
	})
	net.RegisterYYSysProtoV2(127, 107, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PuzzleFunSys).c2sFetchGiftAwards
	})
	gmevent.Register("puzzle.finish", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		qId := utils.AtoUint32(args[0])
		sys := getPuzzleFunSys(player)
		if sys == nil {
			return false
		}

		qData := sys.getUnFinishQuestData(qId)
		if qData == nil {
			return false
		}
		tConfL := sys.getTargetConfFunc(qId)
		for idx, tConf := range tConfL {
			qData.Progress[idx] = tConf.Count
		}

		data := sys.getData()
		data.FinishIds = append(data.FinishIds, qId)
		sys.SendProto3(127, 108, &pb3.S2C_127_108{ActId: sys.Id, FinishQId: qId, Quest: qData})
		return true
	}, 1)
}
