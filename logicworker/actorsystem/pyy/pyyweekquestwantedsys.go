/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type WeekQuestWantedSys struct {
	*YYQuestTargetBase
}

func createWeekQuestWantedSys() iface.IPlayerYY {
	obj := &WeekQuestWantedSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *WeekQuestWantedSys) s2cInfo() {
	s.SendProto3(8, 120, &pb3.S2C_8_120{
		ActiveId: s.Id,
		Data:     s.getData(),
	})
}

func (s *WeekQuestWantedSys) acceptQuest() {
	data := s.getData()
	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return
	}
	for _, quest := range config.Wanted.Quests {
		data.QuestMap[quest.Id] = &pb3.QuestData{
			Id:       quest.Id,
			Progress: nil,
		}
		s.OnAcceptQuest(data.QuestMap[quest.Id])
	}
}

func (s *WeekQuestWantedSys) getData() *pb3.PYYWeekQuestWantedData {
	state := s.GetYYData()
	if nil == state.WeekQuestWantedData {
		state.WeekQuestWantedData = make(map[uint32]*pb3.PYYWeekQuestWantedData)
	}
	if state.WeekQuestWantedData[s.Id] == nil {
		state.WeekQuestWantedData[s.Id] = &pb3.PYYWeekQuestWantedData{}
	}
	if state.WeekQuestWantedData[s.Id].QuestMap == nil {
		state.WeekQuestWantedData[s.Id].QuestMap = make(map[uint32]*pb3.QuestData)
	}
	return state.WeekQuestWantedData[s.Id]
}

func (s *WeekQuestWantedSys) ResetData() {
	state := s.GetYYData()
	if nil == state.WeekQuestWantedData {
		return
	}
	delete(state.WeekQuestWantedData, s.Id)
}

func (s *WeekQuestWantedSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WeekQuestWantedSys) Login() {
	s.s2cInfo()
}

func (s *WeekQuestWantedSys) OnOpen() {
	s.acceptQuest()
	s.s2cInfo()
}

func (s *WeekQuestWantedSys) OnEnd() {
	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return
	}

	data := s.getData()
	var canRecIds []uint32
	var canRecBetterIds []uint32
	for _, questData := range data.QuestMap {
		if !s.CheckFinishQuest(questData) {
			continue
		}
		if !pie.Uint32s(data.RecQuestIds).Contains(questData.Id) {
			canRecIds = append(canRecIds, questData.Id)
		}
		if data.UnLockPremiumRewardAt > 0 && !pie.Uint32s(data.RecBetterQuestIds).Contains(questData.Id) {
			canRecBetterIds = append(canRecBetterIds, questData.Id)
		}
	}

	if len(canRecIds) == 0 && len(canRecBetterIds) == 0 {
		return
	}

	var awards jsondata.StdRewardVec
	for _, questConf := range config.Wanted.Quests {
		if pie.Uint32s(canRecIds).Contains(questConf.Id) {
			data.RecQuestIds = append(data.RecQuestIds, questConf.Id)
			awards = append(awards, questConf.Awards...)
		}
		if data.UnLockPremiumRewardAt > 0 && pie.Uint32s(canRecBetterIds).Contains(questConf.Id) {
			data.RecBetterQuestIds = append(data.RecBetterQuestIds, questConf.Id)
			awards = append(awards, questConf.BetterAwards...)
		}
	}

	awards = jsondata.MergeStdReward(awards)
	if len(awards) == 0 {
		return
	}
	mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
		ConfId:  uint16(config.Wanted.Mail),
		Rewards: awards,
	})
}

func (s *WeekQuestWantedSys) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return nil
	}
	for _, quest := range config.Wanted.Quests {
		if quest.Id == id {
			return quest.Targets
		}
	}
	return nil
}

func (s *WeekQuestWantedSys) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return nil
	}
	for _, quest := range config.Wanted.Quests {
		for _, target := range quest.Targets {
			if target.Type == qtt {
				set[quest.Id] = struct{}{}
				break
			}
		}
	}
	return set
}

func (s *WeekQuestWantedSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	questData := data.QuestMap[id]
	if s.CheckFinishQuest(questData) {
		return nil
	}
	return questData
}

func (s *WeekQuestWantedSys) onUpdateTargetData(id uint32) {
	data := s.getData()
	questData := data.QuestMap[id]
	s.SendProto3(8, 125, &pb3.S2C_8_125{
		ActiveId: s.Id,
		Data:     questData,
	})
}

func (s *WeekQuestWantedSys) c2sRecQuest(msg *base.Message) error {
	var req pb3.C2S_8_122
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	id := req.QuestId
	data := s.getData()

	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	var questConf *jsondata.WeekQuestWantedQuests
	for _, quest := range config.Wanted.Quests {
		if quest.Id != id {
			continue
		}
		questConf = quest
		break
	}

	if questConf == nil {
		return neterror.ConfNotFoundError("%s %d quest not found", s.GetPrefix(), id)
	}

	if !s.CheckFinishQuest(data.QuestMap[id]) {
		return neterror.ParamsInvalidError("%s quest %d not finish", s.GetPrefix(), id)
	}

	var resp pb3.S2C_8_122
	resp.ActiveId = s.Id
	var awards jsondata.StdRewardVec
	if !pie.Uint32s(data.RecQuestIds).Contains(id) {
		resp.QuestId = id
		data.RecQuestIds = append(data.RecQuestIds, id)
		awards = append(awards, questConf.Awards...)
	}

	if data.UnLockPremiumRewardAt > 0 && !pie.Uint32s(data.RecBetterQuestIds).Contains(id) {
		resp.BetterQuestId = id
		data.RecBetterQuestIds = append(data.RecBetterQuestIds, id)
		awards = append(awards, questConf.BetterAwards...)
	}
	if len(awards) == 0 {
		return neterror.ParamsInvalidError("%s %d quest not can rec awards", s.GetPrefix(), id)
	}

	player := s.GetPlayer()
	player.SendProto3(8, 122, &resp)
	awards = jsondata.MergeStdReward(awards)
	if len(awards) > 0 {
		engine.GiveRewards(player, awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYWeekQuestWantedRecAwards,
		})
		player.SendShowRewardsPop(awards)
	}
	player.TriggerQuestEventRange(custom_id.QttCompleteWeekQuestWantedQuest)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYWeekQuestWantedRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d", id, resp.QuestId, resp.BetterQuestId),
	})
	return nil
}

func (s *WeekQuestWantedSys) c2sQuickRec(msg *base.Message) error {
	var req pb3.C2S_8_123
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.getData()
	if data.UnLockPremiumRewardAt == 0 {
		return neterror.ParamsInvalidError("%s not unlock better awards", s.GetPrefix())
	}

	var canRecIds []uint32
	var canRecBetterIds []uint32
	for _, questData := range data.QuestMap {
		if !s.CheckFinishQuest(questData) {
			continue
		}
		if !pie.Uint32s(data.RecQuestIds).Contains(questData.Id) {
			canRecIds = append(canRecIds, questData.Id)
		}
		if !pie.Uint32s(data.RecBetterQuestIds).Contains(questData.Id) {
			canRecBetterIds = append(canRecBetterIds, questData.Id)
		}
	}

	if len(canRecIds) == 0 && len(canRecBetterIds) == 0 {
		return neterror.ParamsInvalidError("%s all quests rec", s.GetPrefix())
	}

	var awards jsondata.StdRewardVec
	for _, questConf := range config.Wanted.Quests {
		if pie.Uint32s(canRecIds).Contains(questConf.Id) {
			data.RecQuestIds = append(data.RecQuestIds, questConf.Id)
			awards = append(awards, questConf.Awards...)
		}
		if pie.Uint32s(canRecBetterIds).Contains(questConf.Id) {
			data.RecBetterQuestIds = append(data.RecBetterQuestIds, questConf.Id)
			awards = append(awards, questConf.BetterAwards...)
		}
	}

	player := s.GetPlayer()
	awards = jsondata.MergeStdReward(awards)
	if len(awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYWeekQuestWantedQuickRec,
		})
		player.SendShowRewardsPop(awards)
	}
	s.SendProto3(8, 123, &pb3.S2C_8_123{
		ActiveId:          s.Id,
		RecQuestIds:       data.RecQuestIds,
		RecBetterQuestIds: data.RecBetterQuestIds,
	})
	player.TriggerQuestEventRange(custom_id.QttCompleteWeekQuestWantedQuest)

	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYWeekQuestWantedQuickRec, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("normal:%v,better:%v", canRecIds, canRecBetterIds),
	})
	return nil
}

func (s *WeekQuestWantedSys) c2sQuickComplete(msg *base.Message) error {
	var req pb3.C2S_8_124
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	config := jsondata.GetWeekQuestWantedConfig(s.ConfName, s.ConfIdx)
	if config == nil || config.Wanted == nil {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.getData()
	if data.UnLockPremiumRewardAt == 0 {
		return neterror.ParamsInvalidError("%s not unlock better awards", s.GetPrefix())
	}

	var quickIds []uint32
	var totalConsume jsondata.ConsumeVec
	for _, questConf := range config.Wanted.Quests {
		if len(questConf.Consume) == 0 {
			continue
		}
		if pie.Uint32s(data.RecQuestIds).Contains(questConf.Id) {
			continue
		}
		if s.CheckFinishQuest(data.QuestMap[questConf.Id]) {
			continue
		}
		quickIds = append(quickIds, questConf.Id)
		totalConsume = append(totalConsume, questConf.Consume...)
	}
	if len(quickIds) == 0 {
		return neterror.ParamsInvalidError("%s all quests complete", s.GetPrefix())
	}

	player := s.GetPlayer()
	if len(totalConsume) == 0 || !player.ConsumeByConf(totalConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYWeekQuestWantedQuickCompile}) {
		return neterror.ConsumeFailedError("consume not enough")
	}

	for _, id := range quickIds {
		questData := data.QuestMap[id]
		s.GmFinishQuest(questData)
	}
	s.SendProto3(8, 124, &pb3.S2C_8_124{
		ActiveId: s.Id,
		Data:     data,
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYWeekQuestWantedQuickCompile, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%v", quickIds),
	})
	return nil
}

func (s *WeekQuestWantedSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	questData := data.QuestMap[questId]
	if questData == nil {
		return
	}
	if pie.Uint32s(data.RecQuestIds).Contains(questId) {
		s.GmFinishQuest(questData)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(questData)
}

func (s *WeekQuestWantedSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.QuestMap, questId)
	data.RecQuestIds = pie.Uint32s(data.RecQuestIds).Filter(func(u uint32) bool {
		return u != questId
	})
}

func checkWeekQuestWantedHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYWeekQuestWanted, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*WeekQuestWantedSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		config := jsondata.GetWeekQuestWantedConfig(sys.ConfName, sys.ConfIdx)
		if config == nil || config.Wanted == nil {
			return
		}
		if config.Wanted.ChargeId != conf.ChargeId {
			return
		}
		if sys.getData().UnLockPremiumRewardAt > 0 {
			return
		}
		ret = true
		return
	})
	return ret
}

func wekQuestWantedChargeHandler(actor iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	var ret bool
	if !checkWeekQuestWantedHandler(actor, chargeConf) {
		return false
	}
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYWeekQuestWanted, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*WeekQuestWantedSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		config := jsondata.GetWeekQuestWantedConfig(sys.ConfName, sys.ConfIdx)
		if config == nil || config.Wanted == nil {
			return
		}
		if config.Wanted.ChargeId != chargeConf.ChargeId {
			return
		}
		if sys.getData().UnLockPremiumRewardAt > 0 {
			return
		}
		sys.getData().UnLockPremiumRewardAt = time_util.NowSec()
		sys.SendProto3(8, 121, &pb3.S2C_8_121{
			ActiveId:              sys.Id,
			UnLockPremiumRewardAt: sys.getData().UnLockPremiumRewardAt,
		})
		if len(config.Wanted.Rewards) > 0 {
			engine.GiveRewards(actor, config.Wanted.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYWeekQuestWantedUnLockAwards})
			actor.SendShowRewardsPop(config.Wanted.Rewards)
		}
		if config.Wanted.BroadcastId > 0 {
			engine.BroadcastTipMsgById(config.Wanted.BroadcastId, actor.GetId(), actor.GetName())
		}
		ret = true
		return
	})
	return ret
}

func handleQttCompleteWeekQuestWantedQuest(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	pyyId := ids[0]
	var count uint32
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYWeekQuestWanted, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*WeekQuestWantedSys)
		if !ok {
			return
		}
		if sys.Id != pyyId {
			return
		}
		count = uint32(len(sys.getData().RecQuestIds))
	})
	return count
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYWeekQuestWanted, func() iface.IPlayerYY {
		return createWeekQuestWantedSys()
	})
	engine.RegChargeEvent(chargedef.PYYWeekQuestWanted, checkWeekQuestWantedHandler, wekQuestWantedChargeHandler)
	engine.RegQuestTargetProgress(custom_id.QttCompleteWeekQuestWantedQuest, handleQttCompleteWeekQuestWantedQuest)
	net.RegisterYYSysProtoV2(8, 122, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*WeekQuestWantedSys).c2sRecQuest
	})
	net.RegisterYYSysProtoV2(8, 123, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*WeekQuestWantedSys).c2sQuickRec
	})
	net.RegisterYYSysProtoV2(8, 124, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*WeekQuestWantedSys).c2sQuickComplete
	})
	gmevent.Register("WeekQuestWantedSys.QuickComplete", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYWeekQuestWanted, func(obj iface.IPlayerYY) {
			sys := obj.(*WeekQuestWantedSys)
			data := sys.getData()
			for _, questData := range data.QuestMap {
				sys.GmFinishQuest(questData)
			}
			sys.s2cInfo()
		})
		return true
	}, 1)
}
