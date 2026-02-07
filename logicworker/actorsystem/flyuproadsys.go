/**
 * @Author: zjj
 * @Date: 2023年11月22日
 * @Desc: 飞升之路
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils/pie"
)

const (
	// 天地元力状态: 0 未完成, 1 已完成
	YuanPowerStateUnOver = 0
	YuanPowerStateOver   = 1
	// 飞升上界状态: 0 未开启, 1 已开启, 2 已完成所有任务, 3 已飞升
	FlyUpWorldStateUnOpen   = 0
	FlyUpWorldStateOpen     = 1
	FlyUpWorldStateComplete = 2
	FlyUpWorldStateFlyOver  = 3
)

type FlyUpRoadSys struct {
	*QuestTargetBase
	flyUpQuestTypeChangeHandleMap map[uint32]func(questId uint32)
}

func (s *FlyUpRoadSys) OnOpen() {
	s.acceptReachCondQuest()
	s.changeFlyUpWorldState(FlyUpWorldStateOpen)
	s.checkFlyUpWorldFinishProcess()
	s.s2cInfo()
}

func (s *FlyUpRoadSys) OnLogin() {
	s.GetOwner().TriggerQuestEventRange(custom_id.QttFirstOrBuyXSponsorGift)
	checkPlayerOnlineFlyUpWorldRank(s.GetOwner(), false)
	s.checkFlyUpWorldFinishProcess()
	s.reLearnSkill()
}

func (s *FlyUpRoadSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *FlyUpRoadSys) OnReconnect() {
	checkPlayerOnlineFlyUpWorldRank(s.GetOwner(), false)
	s.s2cInfo()
}

func (s *FlyUpRoadSys) NewDay() {
	s.acceptReachCondQuest()
	s.s2cInfo()
}

func (s *FlyUpRoadSys) GetYuanPowerState() uint32 {
	return s.getData().GetYuanPowerState()
}

func (s *FlyUpRoadSys) IsYuanPowerStateUnOver() bool {
	return s.GetYuanPowerState() == YuanPowerStateUnOver
}

func (s *FlyUpRoadSys) IsYuanPowerStateOver() bool {
	return s.GetYuanPowerState() == YuanPowerStateOver
}

func (s *FlyUpRoadSys) changeYuanPowerState(state uint32) {
	s.getData().YuanPowerState = state
}

func (s *FlyUpRoadSys) GetFlyUpWorldState() uint32 {
	return s.getData().GetFlyUpWorldState()
}

func (s *FlyUpRoadSys) changeFlyUpWorldState(state uint32) {
	s.getData().FlyUpWorldState = state
}

func (s *FlyUpRoadSys) IsFlyUpWorldStateOpen() bool {
	return s.GetFlyUpWorldState() == FlyUpWorldStateOpen
}

func (s *FlyUpRoadSys) IsFlyUpWorldStateComplete() bool {
	return s.GetFlyUpWorldState() == FlyUpWorldStateComplete
}

func (s *FlyUpRoadSys) IsFlyUpWorldStateUnOpen() bool {
	return s.GetFlyUpWorldState() == FlyUpWorldStateUnOpen
}

func (s *FlyUpRoadSys) getData() *pb3.FlyUpRoadData {
	if s.GetBinaryData().FlyUpRoadData == nil {
		s.GetBinaryData().FlyUpRoadData = &pb3.FlyUpRoadData{}
	}
	data := s.GetBinaryData().FlyUpRoadData
	if data.GroupQuestCompleteCountMap == nil {
		data.GroupQuestCompleteCountMap = make(map[uint32]uint32)
	}
	if data.SubGroupQuestCompleteCountMap == nil {
		data.SubGroupQuestCompleteCountMap = make(map[uint32]uint32)
	}
	if data.QuestMap == nil {
		data.QuestMap = make(map[uint32]*pb3.QuestData)
	}
	if data.SubGroupGiftBuyTimes == nil {
		data.SubGroupGiftBuyTimes = make(map[uint32]uint32)
	}
	if data.FlyUpRoadSubGroupRecAwards == nil {
		data.FlyUpRoadSubGroupRecAwards = make(map[uint32]*pb3.FlyUpRoadSubGroupRecAwards)
	}
	return data
}

func (s *FlyUpRoadSys) s2cYuanPowerState() {
	s.SendProto3(166, 13, &pb3.S2C_166_13{
		YuanPowerState: s.GetYuanPowerState(),
	})
}

func (s *FlyUpRoadSys) s2cFlyUpWorldState() {
	s.SendProto3(166, 14, &pb3.S2C_166_14{
		FlyUpWorldState: s.GetFlyUpWorldState(),
	})
}

func (s *FlyUpRoadSys) s2cInfo() {
	s.SendProto3(166, 0, &pb3.S2C_166_0{
		State: s.getData(),
	})
}

// 接任务
func (s *FlyUpRoadSys) acceptQuestByFlyUpQuestType(typ, parentId uint32) bool {
	data := s.getData()
	owner := s.GetOwner()
	openServerDay := gshare.GetOpenServerDay()
	mgr, err := jsondata.GetFlyUpRoadQuestMgr()
	if err != nil {
		owner.LogError("err:%v", err)
		return false
	}

	var acceptQuestList []*pb3.QuestData
	var acceptNewQuest bool
	for _, conf := range mgr {
		if conf.Typ != typ {
			continue
		}
		_, ok := data.QuestMap[conf.Id]
		if ok {
			continue
		}

		lv := conf.Lv
		circle := conf.Circle
		openDay := conf.OpenDay
		completeQuest := conf.CompleteQuest
		completeSubGroupQuest := conf.CompleteSubGroupQuest

		if circle > 0 && owner.GetCircle() < circle {
			continue
		}

		if lv > 0 && owner.GetLevel() < lv {
			continue
		}

		if openDay > 0 && openServerDay < openDay {
			continue
		}

		if len(completeQuest) == 2 {
			count, ok := data.GroupQuestCompleteCountMap[completeQuest[0]]
			if !ok {
				continue
			}
			if count < completeQuest[1] {
				continue
			}
		}

		if len(completeSubGroupQuest) >= 3 {
			count, ok := data.SubGroupQuestCompleteCountMap[completeSubGroupQuest[1]]
			if !ok {
				continue
			}
			if count < completeSubGroupQuest[2] {
				continue
			}
		}

		if conf.CreateActorDay != 0 && owner.GetCreateDay() < conf.CreateActorDay {
			continue
		}

		if conf.ParentId != 0 && parentId != conf.ParentId {
			continue
		}

		acceptNewQuest = true
		data.QuestMap[conf.Id] = &pb3.QuestData{
			Id: conf.Id,
		}

		acceptQuestList = append(acceptQuestList, data.QuestMap[conf.Id])
	}

	for _, quest := range acceptQuestList {
		s.OnAcceptQuestAndCheckUpdateTarget(quest)
	}
	return acceptNewQuest
}

func (s *FlyUpRoadSys) acceptReachCondQuest() {
	data := s.getData()
	for _, questId := range data.RecQuestIds {
		s.acceptYuanPowerQuest(questId)
		s.acceptFlyUpWorldQuest(questId)
	}
	s.acceptYuanPowerQuest(0)
	s.acceptFlyUpWorldQuest(0)
}

// 接取天地元力的任务
func (s *FlyUpRoadSys) acceptYuanPowerQuest(parentId uint32) bool {
	return s.acceptQuestByFlyUpQuestType(jsondata.FlyUpQuestTypeYuanPower, parentId)
}

// 接取飞升上界的任务
func (s *FlyUpRoadSys) acceptFlyUpWorldQuest(parentId uint32) bool {
	return s.acceptQuestByFlyUpQuestType(jsondata.FlyUpQuestTypeFlyToWorld, parentId)
}

func (s *FlyUpRoadSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	owner := s.GetOwner()
	mgr, err := jsondata.GetFlyUpRoadQuestMgr()
	if err != nil {
		owner.LogError("err:%v", err)
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

func (s *FlyUpRoadSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	questData := data.QuestMap[id]
	if s.CheckFinishQuest(questData) {
		return nil
	}
	return questData
}

func (s *FlyUpRoadSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf, err := jsondata.GetFlyUpRoadQuest(id)
	owner := s.GetOwner()
	if err != nil {
		owner.LogError("err:%d", err)
		return nil
	}
	return conf.Targets
}

func (s *FlyUpRoadSys) onUpdateTargetData(id uint32) {
	owner := s.GetOwner()
	data := s.getData()
	err := s.triggerChange(id)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}
	questData := data.QuestMap[id]
	if !pie.Uint32s(data.FinishQuestIds).Contains(id) && s.CheckFinishQuest(questData) {
		data.FinishQuestIds = append(data.FinishQuestIds, id)
	}
}

// 领取天地元力任务奖励
func (s *FlyUpRoadSys) c2sRecYuanPowerQuest(msg *base.Message) error {
	var req pb3.C2S_166_1
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	err = s.recQuestAward(req.QuestId, pb3.LogId_LogGiveRewardsByRecYuanPowerQuest)
	if err != nil {
		return neterror.Wrap(err)
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogCompleteYuanPowerQuest, &pb3.LogPlayerCounter{
		Timestamp: time_util.NowSec(),
		NumArgs:   uint64(req.QuestId),
	})

	s.SendProto3(166, 1, &pb3.S2C_166_1{
		LayerId: req.LayerId,
		QuestId: req.QuestId,
	})

	if s.acceptYuanPowerQuest(req.QuestId) {
		s.s2cInfo()
	}

	s.checkYuanPowerFinishProcess()
	return nil
}

// 领取飞升上界任务奖励
func (s *FlyUpRoadSys) c2sRecFlyUpWorldQuest(msg *base.Message) error {
	var req pb3.C2S_166_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	err = s.recQuestAward(req.QuestId, pb3.LogId_LogGiveRewardsByRecFlyUpWorldQuest)
	if err != nil {
		return neterror.Wrap(err)
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogCompleteFlyUpWorldQuest, &pb3.LogPlayerCounter{
		Timestamp: time_util.NowSec(),
		NumArgs:   uint64(req.QuestId),
	})

	s.SendProto3(166, 2, &pb3.S2C_166_2{
		LayerId: req.LayerId,
		QuestId: req.QuestId,
	})

	if s.acceptFlyUpWorldQuest(req.QuestId) {
		s.s2cInfo()
	}

	s.checkFlyUpWorldFinishProcess()
	return nil
}

func (s *FlyUpRoadSys) recQuestAward(questId uint32, logId pb3.LogId) error {

	data := s.getData()
	owner := s.GetOwner()

	quest, err := jsondata.GetFlyUpRoadQuest(questId)
	if err != nil {
		return err
	}

	finishQuestIds := data.FinishQuestIds
	recQuestIds := data.RecQuestIds
	if !pie.Uint32s(finishQuestIds).Contains(questId) {
		return neterror.ParamsInvalidError("not found quest, id is %d", questId)
	}

	if pie.Uint32s(recQuestIds).Contains(questId) {
		return neterror.ParamsInvalidError("already rec quest awards, id is %d", questId)
	}

	recQuestIds = append(recQuestIds, questId)
	data.RecQuestIds = recQuestIds
	s.addHistoryYuanPowerNum(quest.YuanPowerNum)

	s.GetOwner().TriggerQuestEventRange(custom_id.QttFlyUpWorldYuanPower)
	// 下发奖励
	if len(quest.Awards) > 0 {
		engine.GiveRewards(owner, quest.Awards, common.EngineGiveRewardParam{
			LogId: logId,
		})
	}
	data.GroupQuestCompleteCountMap[quest.GroupId] += 1
	data.SubGroupQuestCompleteCountMap[quest.SubGroupId] += 1
	owner.TriggerQuestEventRange(custom_id.QttCompleteFlyUpRoadSubGroupQuest)
	owner.TriggerQuestEvent(custom_id.QttCompositeFlyUpRoadQuestNum, 0, 1)
	owner.TriggerQuestEvent(custom_id.QttAchievementsCompositeFlyUpRoadQuestNum, 0, 1)
	return nil
}

// 激活飞升上界
func (s *FlyUpRoadSys) c2sActivateFlyUpWorld(_ *base.Message) error {
	owner := s.GetOwner()

	if s.IsYuanPowerStateOver() {
		return neterror.ParamsInvalidError("yuan power state is %d", s.GetYuanPowerState())
	}

	if !s.IsFlyUpWorldStateUnOpen() {
		return neterror.ParamsInvalidError("already activate fly up to world state is %d", s.GetFlyUpWorldState())
	}

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActivateFlyUpWorld, &pb3.LogPlayerCounter{
		Timestamp: time_util.NowSec(),
	})
	s.acceptFlyUpWorldQuest(0)

	s.changeFlyUpWorldState(FlyUpWorldStateOpen)
	s.SendProto3(166, 3, &pb3.S2C_166_3{})
	s.s2cInfo()
	return nil
}

func (s *FlyUpRoadSys) c2sJoinFlyUpWorldRank(_ *base.Message) error {
	day := gshare.GetOpenServerDay()
	owner := s.GetOwner()
	data := s.getData()
	mgr, err := jsondata.GetFlyUpRoadConfMgr()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	if day < mgr.ServerOpenDay {
		return neterror.ParamsInvalidError("server open day is %d,conf day is %d", day, mgr.ServerOpenDay)
	}

	if owner.GetLevel() < mgr.LevelLimit {
		return neterror.ParamsInvalidError("player level limit")
	}

	if owner.GetExtraAttrU32(attrdef.Circle) < mgr.Circle {
		return neterror.ParamsInvalidError("player circle limit")
	}

	// 消耗道具
	if len(mgr.ActivateFlyUpWorld) > 0 && !owner.ConsumeByConf(mgr.ActivateFlyUpWorld, false, common.ConsumeParams{LogId: pb3.LogId_LogActivateFlyUpWorldConsume}) {
		return neterror.ConsumeFailedError("activate fly up to world fail")
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeFlyUpRoad)
	if rank == nil {
		return neterror.ParamsInvalidError("not found rank, type is %d", gshare.RankTypeFlyUpRoad)
	}

	rankById := rank.GetRankById(s.GetOwner().GetId())
	if rankById > 0 {
		return neterror.ParamsInvalidError("already join rank")
	}

	if data.CompleteFlyUpWorldQuestTimeAt < owner.GetCreateTime() {
		return neterror.ParamsInvalidError("complete fly up world quest time at %d < create time %d", data.CompleteFlyUpWorldQuestTimeAt, owner.GetCreateTime())
	}

	var bastAt = gshare.GetFlyUpRoadBastTimeAt()
	diff := bastAt - data.CompleteFlyUpWorldQuestTimeAt
	owner.SetRankValue(gshare.RankTypeFlyUpRoad, int64(diff))
	ok := rank.Update(owner.GetId(), int64(diff))
	if !ok {
		return neterror.ParamsInvalidError("update rank false, id %d ,score %d", owner.GetId(), diff)
	}

	rankById = rank.GetRankById(s.GetOwner().GetId())
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogJoinFlyUpWorldRank, &pb3.LogPlayerCounter{
		Timestamp: time_util.NowSec(),
		NumArgs:   uint64(rankById),
	})

	data.FlyUpToWorldAt = time_util.NowSec()
	s.changeFlyUpWorldState(FlyUpWorldStateFlyOver)
	data.Rank = rankById
	owner.SetExtraAttr(attrdef.FlyUpWorldQuestRank, attrdef.AttrValueAlias(data.Rank))
	owner.TriggerQuestEvent(custom_id.QttFlyUpToWorld, 0, 1)
	s.SendProto3(166, 4, &pb3.S2C_166_4{
		Rank: rankById,
	})
	s.s2cFlyUpWorldState()
	s.owner.TriggerEvent(custom_id.AeFlyUpWorldFinish)
	event.TriggerSysEvent(custom_id.SeJoinFlyUpWorldRank)
	return nil
}

// 触发变动
func (s *FlyUpRoadSys) triggerChange(questId uint32) error {
	q, err := jsondata.GetFlyUpRoadQuest(questId)
	if err != nil {
		return err
	}

	handle, ok := s.flyUpQuestTypeChangeHandleMap[q.Typ]
	if !ok {
		return nil
	}

	handle(questId)
	return nil
}

// 天地元力任务
func (s *FlyUpRoadSys) handleFlyUpQuestTypeChangeYuanPower(questId uint32) {
	data := s.getData()
	owner := s.GetOwner()
	q, err := jsondata.GetFlyUpRoadQuest(questId)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	qData, ok := data.QuestMap[questId]
	if !ok {
		owner.LogError("not found %d quest data", questId)
		return
	}

	owner.SendProto3(166, 10, &pb3.S2C_166_10{
		LayerId: q.Typ,
		Quest:   qData,
	})
}

func (s *FlyUpRoadSys) checkYuanPowerFinishProcess() {
	data := s.getData()
	// 检查天地元力的任务状态
	oldState := data.GetYuanPowerState()
	newState := s.checkYuanPowerState()
	if oldState == newState {
		return
	}
	s.changeYuanPowerState(newState)
	s.s2cYuanPowerState()
}

// 飞升上界任务
func (s *FlyUpRoadSys) handleFlyUpQuestTypeChangeFlyToWorld(questId uint32) {
	data := s.getData()
	owner := s.GetOwner()
	q, err := jsondata.GetFlyUpRoadQuest(questId)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	qData, ok := data.QuestMap[questId]
	if !ok {
		owner.LogError("not found %d quest data", questId)
		return
	}

	owner.SendProto3(166, 11, &pb3.S2C_166_11{
		LayerId: q.GroupId,
		Quest:   qData,
	})
}

func (s *FlyUpRoadSys) checkFlyUpWorldFinishProcess() {
	data := s.getData()
	oldState := data.GetFlyUpWorldState()
	newState := s.checkFlyUpWorldState()
	if oldState == newState {
		return
	}
	s.changeFlyUpWorldState(newState)
	if s.IsFlyUpWorldStateComplete() {
		data.CompleteFlyUpWorldQuestTimeAt = time_util.NowSec()
	}
	s.s2cFlyUpWorldState()
}

// 检查天地元力的任务状态
func (s *FlyUpRoadSys) checkYuanPowerState() uint32 {
	data := s.getData()
	var idSet = make(map[uint32]struct{})
	for _, id := range data.RecQuestIds {
		idSet[id] = struct{}{}
	}

	var state = YuanPowerStateOver
	for _, quest := range data.QuestMap {
		if state != YuanPowerStateOver {
			break
		}

		questConf, err := jsondata.GetFlyUpRoadQuest(quest.Id)
		if err != nil {
			s.LogError("err:%v", err)
			continue
		}

		if questConf.Typ != jsondata.FlyUpQuestTypeYuanPower {
			continue
		}

		if _, ok := idSet[quest.Id]; ok {
			continue
		}

		state = YuanPowerStateUnOver
		break
	}
	return uint32(state)
}

// 检查飞升上界的任务状态
func (s *FlyUpRoadSys) checkFlyUpWorldState() uint32 {
	data := s.getData()

	// 不是开启状态 直接结束
	if !s.IsFlyUpWorldStateOpen() {
		return s.GetFlyUpWorldState()
	}

	var idSet = make(map[uint32]struct{})
	for _, id := range data.RecQuestIds {
		idSet[id] = struct{}{}
	}

	var state = FlyUpWorldStateComplete
	for _, quest := range data.QuestMap {
		if state != FlyUpWorldStateComplete {
			break
		}

		questConf, err := jsondata.GetFlyUpRoadQuest(quest.Id)
		if err != nil {
			s.LogError("err:%v", err)
			continue
		}

		if questConf.Typ != jsondata.FlyUpQuestTypeFlyToWorld {
			continue
		}

		if _, ok := idSet[quest.Id]; ok {
			continue
		}

		state = FlyUpWorldStateOpen
		break
	}

	return uint32(state)
}

// 记录一波状态
func (s *FlyUpRoadSys) c2sPlayBackFlyToWorldAt(_ *base.Message) error {
	data := s.getData()
	if data.PlayBackFlyToWorldAt != 0 {
		return nil
	}
	data.PlayBackFlyToWorldAt = time_util.NowSec()
	s.SendProto3(166, 5, &pb3.S2C_166_5{
		PlayBackFlyToWorldAt: data.PlayBackFlyToWorldAt,
	})
	return nil
}

// 领取天地元力进度奖励
func (s *FlyUpRoadSys) c2sRecYuanPowerNumAwards(msg *base.Message) error {
	var req pb3.C2S_166_6
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	mgr, err := jsondata.GetFlyUpRoadConfMgr()
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()

	if pie.Uint32s(data.RecYuanPowerNumAwardsIdxList).Contains(req.Idx) {
		return neterror.ParamsInvalidError("already rec , idx is %d", req.Idx)
	}

	if uint32(len(mgr.YuanTargetsAwards)) <= req.Idx {
		return neterror.ParamsInvalidError("too long, idx is %d", req.Idx)
	}

	targetAwards := mgr.YuanTargetsAwards[req.Idx]
	if data.HistoryYuanPowerNum < targetAwards.Count {
		return neterror.ParamsInvalidError("HistoryYuanPowerNum %d, target count is %d", data.HistoryYuanPowerNum, targetAwards.Count)
	}

	data.RecYuanPowerNumAwardsIdxList = append(data.RecYuanPowerNumAwardsIdxList, req.Idx)
	if len(targetAwards.Awards) > 0 {
		engine.GiveRewards(s.GetOwner(), targetAwards.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogRecHistoryYuanPowerNumAwards,
		})
	}
	s.SendProto3(166, 6, &pb3.S2C_166_6{
		Idx: req.Idx,
	})
	return nil
}

func (s *FlyUpRoadSys) c2sBuySubGroupGift(msg *base.Message) error {
	owner := s.GetOwner()
	var req pb3.C2S_166_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	groupId := req.GroupId
	subGroupId := req.SubGroupId
	groupConf, err := jsondata.GetFlyUpRoadGroup(groupId)
	if err != nil {
		return neterror.Wrap(err)
	}

	subGroupConf := groupConf.SubGroupConf[subGroupId]
	if subGroupConf == nil {
		return neterror.ConfNotFoundError("group:%d subGroup:%d not found", groupId, subGroupId)
	}

	data := s.getData()
	count := data.SubGroupGiftBuyTimes[subGroupId]
	if subGroupConf.GiftLimit != 0 && count >= subGroupConf.GiftLimit {
		return neterror.ParamsInvalidError("buy limit %d", count)
	}

	if len(subGroupConf.GiftConsume) != 0 && !owner.ConsumeByConf(subGroupConf.GiftConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyFlyUpRoadSubGroupGift}) {
		return neterror.ConfNotFoundError("group:%d subGroup:%d consume failed", groupId, subGroupId)
	}

	if len(subGroupConf.GiftAwards) > 0 {
		engine.GiveRewards(owner, subGroupConf.GiftAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogGiveAwardsFlyUpRoadSubGroupGift,
		})
	}

	count += 1
	data.SubGroupGiftBuyTimes[subGroupId] = count
	s.addHistoryYuanPowerNum(subGroupConf.YuanPowerNum)

	s.SendProto3(166, 7, &pb3.S2C_166_7{
		GroupId:      groupId,
		SubGroupId:   subGroupId,
		GiftBuyTimes: count,
	})
	return nil
}

func (s *FlyUpRoadSys) addHistoryYuanPowerNum(number uint32) {
	data := s.getData()
	data.HistoryYuanPowerNum += number
	s.GetOwner().TriggerQuestEvent(custom_id.QttHistoryYuanPowerNum, 0, int64(data.HistoryYuanPowerNum))
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeFlyUpLoadItem, s.GetOwner(), int64(data.HistoryYuanPowerNum), false, 0)
}

func (s *FlyUpRoadSys) reLearnSkill() {
	data := s.getData()
	owner := s.GetOwner()
	for _, id := range data.SkillIds {
		owner.LearnSkill(id, 1, true)
	}
}

func (s *FlyUpRoadSys) checkLearnSkill(groupId uint32) error {
	data := s.getData()
	owner := s.GetOwner()

	count := jsondata.GetFlyUpRoadGroupQuestCount(groupId)
	if count == 0 {
		return nil
	}

	cCount := data.GroupQuestCompleteCountMap[groupId]
	if cCount != count {
		return nil
	}

	group, err := jsondata.GetFlyUpRoadGroup(groupId)
	if err != nil {
		return neterror.Wrap(err)
	}

	skillId := group.SkillId
	if skillId == 0 {
		return nil
	}

	skillIds := pie.Uint32s(data.SkillIds)
	if skillIds.Contains(skillId) {
		return nil
	}

	data.SkillIds = skillIds.Append(skillId).Unique()
	if !owner.LearnSkill(skillId, 1, true) {
		return neterror.InternalError("fly up road learn skill failed. Id:%d", skillId)
	}

	owner.SendProto3(166, 8, &pb3.S2C_166_8{
		GroupId: groupId,
		SkillId: skillId,
	})
	return nil
}

func (s *FlyUpRoadSys) c2sLearnSkill(msg *base.Message) error {
	var req pb3.C2S_166_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	err = s.checkLearnSkill(req.GroupId)
	return err
}

func (s *FlyUpRoadSys) c2sRecSubGroupAwards(msg *base.Message) error {
	var req pb3.C2S_166_9
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	err = s.recSubGroupAwards(req.GroupId, req.SubGroupId, req.Target)
	return err
}

func (s *FlyUpRoadSys) recSubGroupAwards(groupId, subGroupId, target uint32) error {
	data := s.getData()
	owner := s.GetOwner()

	groupConf, err := jsondata.GetFlyUpRoadGroup(groupId)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	subGroupConf := groupConf.SubGroupConf[subGroupId]
	if subGroupConf == nil {
		return neterror.ConfNotFoundError("%d %d not found conf", groupId, subGroupId)
	}

	var subGroupAwards *jsondata.FlyUpRoadGroupAwardConf
	for _, award := range subGroupConf.SubGroupAwards {
		if award.StageTarget == target {
			subGroupAwards = award
			break
		}
	}

	if subGroupAwards == nil {
		return neterror.ConfNotFoundError("%d %d %d not found conf", groupId, subGroupId, target)
	}

	cCount := data.SubGroupQuestCompleteCountMap[subGroupId]
	if cCount < target {
		return nil
	}

	subGroupRecAwards, ok := data.FlyUpRoadSubGroupRecAwards[subGroupId]
	if !ok {
		data.FlyUpRoadSubGroupRecAwards[subGroupId] = &pb3.FlyUpRoadSubGroupRecAwards{
			SubGroupId: subGroupId,
		}
		subGroupRecAwards = data.FlyUpRoadSubGroupRecAwards[subGroupId]
	}
	ids := pie.Uint32s(subGroupRecAwards.RecTargetList)
	if ids.Contains(target) {
		return nil
	}

	subGroupRecAwards.RecTargetList = ids.Append(target).Unique()
	if len(subGroupAwards.StageAwards) != 0 {
		engine.GiveRewards(owner, subGroupAwards.StageAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogFlyUpLoadRecSubGroupAwards,
		})
		engine.BroadcastTipMsgById(tipmsgid.FlyuproadquestStageAwardsShow, owner.GetName(), subGroupConf.Name, engine.StdRewardToBroadcast(s.owner, subGroupAwards.StageAwards))
	}

	owner.SendProto3(166, 9, &pb3.S2C_166_9{
		GroupId:    groupId,
		SubGroupId: subGroupId,
		Target:     target,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFlyUpLoadRecSubGroupAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(target),
		StrArgs: fmt.Sprintf("%d_%d_%d", groupId, subGroupId, target),
	})

	return nil
}

func (s *FlyUpRoadSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	questData, ok := data.QuestMap[questId]
	if !ok {
		return
	}
	if pie.Uint32s(data.RecQuestIds).Contains(questId) || pie.Uint32s(data.FinishQuestIds).Contains(questId) {
		s.GmFinishQuest(questData)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(questData)
}

func (s *FlyUpRoadSys) GMDelQuest(questId uint32) {
	data := s.getData()
	data.FinishQuestIds = pie.Uint32s(data.FinishQuestIds).Filter(func(u uint32) bool {
		return u != questId
	})
	data.RecQuestIds = pie.Uint32s(data.RecQuestIds).Filter(func(u uint32) bool {
		return u != questId
	})
	delete(data.QuestMap, questId)

}

func createFlyUpRoadSys() iface.ISystem {
	sys := &FlyUpRoadSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	sys.flyUpQuestTypeChangeHandleMap = map[uint32]func(questId uint32){
		jsondata.FlyUpQuestTypeYuanPower:  sys.handleFlyUpQuestTypeChangeYuanPower,
		jsondata.FlyUpQuestTypeFlyToWorld: sys.handleFlyUpQuestTypeChangeFlyToWorld,
	}
	return sys
}

func checkPlayerOnlineFlyUpWorldRank(player iface.IPlayer, sendSelf bool) {
	obj := player.GetSysObj(sysdef.SiFlyUpRoad)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FlyUpRoadSys)
	if !ok {
		return
	}
	data := sys.getData()
	if data.Rank == 0 {
		return
	}
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeFlyUpRoad)
	if rank == nil {
		return
	}
	data.Rank = rank.GetRankById(player.GetId())
	player.SetExtraAttr(attrdef.FlyUpWorldQuestRank, attrdef.AttrValueAlias(data.Rank))
	if sendSelf {
		player.SendProto3(166, 4, &pb3.S2C_166_4{
			Rank: data.Rank,
		})
	}
}

func updateAllOnlineFlyUpWorldRank(_ ...interface{}) {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		checkPlayerOnlineFlyUpWorldRank(player, true)
	})
}

func handleQttFlyUpToWorld(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	if actor.IsFlyUpToWorld() {
		return 1
	}
	return 0
}

func handleQttFlyUpWorldYuanPower(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiFlyUpRoad).(*FlyUpRoadSys)
	if !ok || !s.IsOpen() {
		return 0
	}

	data := s.getData()

	return data.HistoryYuanPowerNum
}

func handleQttCompleteFlyUpRoadSubGroupQuest(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) == 0 {
		return 0
	}
	subGroupId := ids[0]
	var count uint32
	obj := player.GetSysObj(sysdef.SiFlyUpRoad)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	questIds := obj.(*FlyUpRoadSys).getData().RecQuestIds
	for _, id := range questIds {
		quest, err := jsondata.GetFlyUpRoadQuest(id)
		if err != nil {
			continue
		}
		if quest.SubGroupId != subGroupId {
			continue
		}
		count += 1
	}
	return count
}

func handleFlyUpRoadAeCircleOrLevelChange(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiFlyUpRoad)
	if obj == nil || !obj.IsOpen() {
		return
	}
	obj.(*FlyUpRoadSys).acceptReachCondQuest()
	obj.(*FlyUpRoadSys).s2cInfo()
}

func handleFlyUpRoadAeNewDay(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiFlyUpRoad)
	if obj == nil || !obj.IsOpen() {
		return
	}
	obj.(*FlyUpRoadSys).NewDay()
}

func init() {
	RegisterSysClass(sysdef.SiFlyUpRoad, func() iface.ISystem {
		return createFlyUpRoadSys()
	})
	net.RegisterSysProtoV2(166, 1, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sRecYuanPowerQuest
	})
	net.RegisterSysProtoV2(166, 2, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sRecFlyUpWorldQuest
	})
	net.RegisterSysProtoV2(166, 3, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sActivateFlyUpWorld
	})
	net.RegisterSysProtoV2(166, 4, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sJoinFlyUpWorldRank
	})
	net.RegisterSysProtoV2(166, 5, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sPlayBackFlyToWorldAt
	})
	net.RegisterSysProtoV2(166, 6, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sRecYuanPowerNumAwards
	})
	net.RegisterSysProtoV2(166, 7, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sBuySubGroupGift
	})
	net.RegisterSysProtoV2(166, 8, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sLearnSkill
	})
	net.RegisterSysProtoV2(166, 9, sysdef.SiFlyUpRoad, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyUpRoadSys).c2sRecSubGroupAwards
	})
	event.RegActorEventH(custom_id.AeCircleChange, handleFlyUpRoadAeCircleOrLevelChange)
	event.RegActorEventH(custom_id.AeLevelUp, handleFlyUpRoadAeCircleOrLevelChange)
	event.RegActorEvent(custom_id.AeNewDay, handleFlyUpRoadAeNewDay)
	// 触发有人加入排行榜 检查一下在线玩家的排名
	event.RegSysEvent(custom_id.SeJoinFlyUpWorldRank, updateAllOnlineFlyUpWorldRank)
	engine.RegQuestTargetProgress(custom_id.QttFlyUpToWorld, handleQttFlyUpToWorld)
	engine.RegQuestTargetProgress(custom_id.QttFlyUpWorldYuanPower, handleQttFlyUpWorldYuanPower)
	engine.RegQuestTargetProgress(custom_id.QttCompleteFlyUpRoadSubGroupQuest, handleQttCompleteFlyUpRoadSubGroupQuest)
	initFlyToWorldGm()
}

func initFlyToWorldGm() {
	gmevent.Register("FlyUpRoadSys.gmQuest", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyUpRoad)
		if obj == nil {
			return false
		}
		if !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*FlyUpRoadSys)
		if !ok {
			return false
		}
		data := sys.getData()
		for _, quest := range data.QuestMap {
			sys.GmFinishQuest(quest)
		}
		return true
	}, 1)
	gmevent.Register("FlyUpRoadSys.quickRecQuest", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyUpRoad)
		if obj == nil {
			return false
		}
		if !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*FlyUpRoadSys)
		if !ok {
			return false
		}
		data := sys.getData()

		for _, quest := range data.QuestMap {
			questConf, err := jsondata.GetFlyUpRoadQuest(quest.Id)
			if err != nil {
				player.LogError("err:%v", err)
				continue
			}
			msg := base.NewMessage()
			switch questConf.Typ {
			case jsondata.FlyUpQuestTypeYuanPower:
				msg.SetCmd(166<<8 | 1)
				err = msg.PackPb3Msg(&pb3.C2S_166_1{
					LayerId: questConf.GroupId,
					QuestId: quest.Id,
				})
				if err != nil {
					player.LogError(err.Error())
				}
				player.DoNetMsg(166, 1, msg)
			case jsondata.FlyUpQuestTypeFlyToWorld:
				msg.SetCmd(166<<8 | 2)
				err := msg.PackPb3Msg(&pb3.C2S_166_2{
					LayerId: questConf.GroupId,
					QuestId: quest.Id,
				})
				if err != nil {
					player.LogError(err.Error())
				}
				player.DoNetMsg(166, 2, msg)
			}
		}
		return true
	}, 1)
	gmevent.Register("FlyUpRoadSys.gmSubGroupGift", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyUpRoad)
		if obj == nil {
			return false
		}
		if !obj.IsOpen() {
			return false
		}
		mgr, err := jsondata.GetFlyUpRoadGroupMgr()
		if err != nil {
			player.LogError("err:%v", err)
			return false
		}
		for _, conf := range mgr {
			for _, groupConf := range conf.SubGroupConf {
				msg := base.NewMessage()
				msg.SetCmd(166<<8 | 1)
				var req = pb3.C2S_166_7{
					GroupId:    conf.Id,
					SubGroupId: groupConf.Id,
				}
				err = msg.PackPb3Msg(&req)
				if err != nil {
					player.LogError(err.Error())
				}
				player.DoNetMsg(166, 7, msg)
			}
		}
		return true
	}, 1)
	gmevent.Register("FlyUpRoadSys.gmSetMaxPowerNumb", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyUpRoad)
		if obj == nil {
			return false
		}
		if !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*FlyUpRoadSys)
		if !ok {
			return false
		}
		data := sys.getData()
		data.HistoryYuanPowerNum = utils.AtoUint32(args[0])
		sys.addHistoryYuanPowerNum(0)
		sys.s2cInfo()
		return true
	}, 1)
	gmevent.Register("FlyUpRoadSys.RankTypeFlyUpRoad", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyUpRoad)
		if obj == nil {
			return false
		}
		if !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*FlyUpRoadSys)
		if !ok {
			return false
		}
		data := sys.getData()
		rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeFlyUpRoad)
		if rank == nil {
			return false
		}
		var bastAt = gshare.GetFlyUpRoadBastTimeAt()
		diff := bastAt - data.CompleteFlyUpWorldQuestTimeAt
		player.SetRankValue(gshare.RankTypeFlyUpRoad, int64(diff))
		rank.Update(player.GetId(), int64(diff))
		data.Rank = rank.GetRankById(player.GetId())
		checkPlayerOnlineFlyUpWorldRank(player, false)
		player.TriggerEvent(custom_id.AeFlyUpWorldFinish)
		sys.s2cInfo()
		return true
	}, 1)
}
