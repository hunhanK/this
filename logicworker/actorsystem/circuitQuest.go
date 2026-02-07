/**
 * @Author: zjj
 * @Date: 2024/4/25
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

type CircuitQuestHelper struct {
	owner iface.IPlayer

	commonCanAccept func(conf *jsondata.StdQuest) bool
}

func NewCircuitQuestHelper(owner iface.IPlayer) *CircuitQuestHelper {
	return &CircuitQuestHelper{owner: owner}
}

func (sys *CircuitQuestHelper) GetBinaryData() *pb3.PlayerBinaryData {
	return sys.owner.GetBinaryData()
}

func (sys *CircuitQuestHelper) getCircuitTaskRound() uint32 {
	return sys.circuitQuestState().Round
}

func (sys *CircuitQuestHelper) IsDailyCircuit(conf *jsondata.StdQuest) bool {
	return conf.Type == custom_id.QtCircuit
}

// 等级变动
func (sys *CircuitQuestHelper) onLevelChange() (resetRecCircuitQuest bool) {
	binary := sys.owner.GetBinaryData()
	if nil == binary {
		return
	}

	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.Quest != nil {
		return
	}

	// 能接根任务就不重置
	questConf := jsondata.GetQuestConf(circuitQuestState.RootQuestId)
	if questConf != nil && sys.CanAccept(questConf) {
		return
	}

	sys.owner.SendProto3(7, 31, &pb3.S2C_7_31{})
	return true
}

func (sys *CircuitQuestHelper) circuitQuestState() *pb3.CircuitQuestState {
	state := sys.owner.GetBinaryData().CircuitQuestState
	if state == nil {
		state = &pb3.CircuitQuestState{}
		sys.owner.GetBinaryData().CircuitQuestState = state
	}
	return state
}

func (sys *CircuitQuestHelper) resetCircuitQuestState() {
	binary := sys.GetBinaryData()
	if nil == binary {
		return
	}

	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.Quest == nil {
		// 清理未接取的朝云悬赏
		sys.owner.SendProto3(7, 31, &pb3.S2C_7_31{})
		return
	}

	for idx, quest := range binary.AllQuest {
		if quest.GetId() != circuitQuestState.Quest.Id {
			continue
		}
		last := len(binary.AllQuest) - 1
		binary.AllQuest[idx] = binary.AllQuest[last]
		binary.AllQuest[last] = nil
		binary.AllQuest = binary.AllQuest[:last]
		break
	}

	// 通知客户端需要清理的任务
	var ids []uint32
	if circuitQuestState.Quest != nil { // 接了任务 直接删
		ids = append(ids, circuitQuestState.Quest.Id)
	} else { // 没接任务 需要删那些可以接的那些任务
		childVec := jsondata.RootTaskKv[circuitQuestState.FinishQuestId]
		for _, id := range childVec {
			conf := jsondata.GetQuestConf(id)
			if conf == nil {
				continue
			}
			if !sys.IsDailyCircuit(conf) {
				continue
			}
			ids = append(ids, conf.Id)
		}
	}

	if len(ids) > 0 {
		sys.owner.SendProto3(7, 9, &pb3.S2C_7_9{
			Ids: ids,
		})
	}

	circuitQuestState.Quest = nil
	circuitQuestState.RootQuestId = 0
	circuitQuestState.FinishQuestId = 0
	circuitQuestState.SpecRootQuestAt = 0
	circuitQuestState.AcceptedAt = 0
	circuitQuestState.Round = 0
}

func (sys *CircuitQuestHelper) getQuestIdSet() map[uint32]struct{} {
	circuitQuestState := sys.circuitQuestState()
	set := make(map[uint32]struct{})
	if time_util.IsSameDay(circuitQuestState.AcceptedAt, time_util.NowSec()) &&
		circuitQuestState.FinishQuestId != circuitQuestState.Quest.Id {
		set[circuitQuestState.Quest.Id] = struct{}{}
	}
	return set
}

func (sys *CircuitQuestHelper) getUnFinishQuestData(id uint32) *pb3.QuestData {
	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.FinishQuestId != id && circuitQuestState.Quest.Id == id {
		return circuitQuestState.Quest
	}
	return nil
}

func (sys *CircuitQuestHelper) SetFinish(id uint32) bool {
	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.FinishQuestId == id {
		logger.LogWarn("already finish")
		return false
	}
	if circuitQuestState.Quest.Id != id {
		logger.LogWarn("current quest incorrect, expect id %d given id %d", circuitQuestState.Quest.Id, id)
		return false
	}
	circuitQuestState.FinishQuestId = id

	sys.owner.TriggerQuestEventRange(custom_id.QttDailyCircuitQuestRound)
	if sys.isFinishEndRoundQuest() {
		sys.owner.TriggerQuestEvent(custom_id.QttCompleteDailyCircuitQuestRound, 0, 1)
	}
	sys.owner.TriggerQuestEvent(custom_id.QttCompleteDailyCircuitQuest, 0, 1)
	sys.owner.TriggerQuestEvent(custom_id.QttCompleteDailyCircuitQuestRecord, 0, 1)

	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogQuestCircuitQuest, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("{\"round\":%d}", circuitQuestState.Round),
	})
	return true
}

func (sys *CircuitQuestHelper) CanAccept(conf *jsondata.StdQuest) bool {
	if !sys.commonCanAccept(conf) {
		return false
	}

	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.Quest != nil && circuitQuestState.FinishQuestId != circuitQuestState.Quest.Id {
		return false
	}

	circuitQuestConf, ok := jsondata.GetCircuitQuestConfByRootQuestId(circuitQuestState.RootQuestId)
	if !ok {
		return false
	}

	var curRoundQuests *jsondata.CircuitQuest
	if circuitQuestState.Round > 0 {
		curRoundQuests = circuitQuestConf.RoundConfs[circuitQuestState.Round-1]
	} else {
		curRoundQuests = circuitQuestConf.RoundConfs[0]
	}
	if circuitQuestState.FinishQuestId > 0 {
		if circuitQuestState.FinishQuestId == curRoundQuests.QuestIds[len(curRoundQuests.QuestIds)-1] {
			return circuitQuestConf.RoundConfs[circuitQuestState.Round].QuestIds[0] == conf.Id
		}

		var nextQuestIdx int
		for idx, questId := range curRoundQuests.QuestIds {
			if questId != circuitQuestState.FinishQuestId {
				continue
			}
			nextQuestIdx = idx
			break
		}
		return curRoundQuests.QuestIds[nextQuestIdx] == conf.Id
	}

	return curRoundQuests.QuestIds[0] == conf.Id
}

// reRecCircuitQuest 是否重置
// specRootQuestAt 派发任务时间
func (sys *CircuitQuestHelper) getAcceptableCircuitQuestIds(resetRecCircuitQuest bool) []uint32 {
	var ids []uint32
	circuitQuestState := sys.circuitQuestState()
	now := time_util.NowSec()
	switch {
	case !resetRecCircuitQuest && time_util.IsSameDay(circuitQuestState.SpecRootQuestAt, now):
		ids = sys.sameSpecRootQuestDayGetAcceptableCircuitQuestIds()
	default:
		id := sys.resetAndGetAcceptableCircuitQuestId()
		if id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// 初始化并且获取一个跑环任务
func (sys *CircuitQuestHelper) resetAndGetAcceptableCircuitQuestId() uint32 {
	var (
		circuitQuestIds  []uint32
		lastCircuitQuest *jsondata.StdQuest // 兜底跑环任务
	)

	// 遍历跑环任务 拿到跑环任务的ID
	for id, conf := range jsondata.StdQuestConfMgr {
		if conf.Type != custom_id.QtCircuit {
			continue
		}
		if conf.ParentId != 0 {
			continue
		}

		lastCircuitQuest = conf

		if !conf.CheckLevel(sys.owner.GetLevel()) {
			continue
		}

		if !conf.CheckCircle(sys.owner.GetCircle()) {
			continue
		}

		if !conf.CheckDaysCond(gshare.GetOpenServerDay()) {
			continue
		}
		circuitQuestIds = append(circuitQuestIds, id)
	}

	// 如果跑环任务遍历的时候都不符合条件 那么就取最后一个符合条件的跑环任务
	if len(circuitQuestIds) == 0 && lastCircuitQuest != nil && lastCircuitQuest.Level < sys.owner.GetLevel() {
		circuitQuestIds = append(circuitQuestIds, lastCircuitQuest.Id)
		logger.LogDebug("lastCircuitQuestId:%d", lastCircuitQuest.Id)
	}

	// 实在找不到跑环任务
	if len(circuitQuestIds) == 0 {
		logger.LogDebug("not found meet the conditions circuit quest")
		return 0
	}

	sys.resetCircuitQuestState()

	// 在符合条件的跑环任务随机取一个
	circuitQuestIdx := random.Interval(0, len(circuitQuestIds)-1)
	circuitQuestId := circuitQuestIds[circuitQuestIdx]

	circuitQuestState := sys.circuitQuestState()
	circuitQuestState.RootQuestId = circuitQuestId
	circuitQuestState.SpecRootQuestAt = time_util.NowSec()

	return circuitQuestId
}

// 下发的任务是同一天的
func (sys *CircuitQuestHelper) sameSpecRootQuestDayGetAcceptableCircuitQuestIds() []uint32 {
	circuitQuestState := sys.circuitQuestState()
	now := time_util.NowSec()

	// 接任务时间不是同一天
	// 但是这个任务又符合派发的 那就从根任务开始重新做
	if !time_util.IsSameDay(circuitQuestState.AcceptedAt, now) {
		return []uint32{circuitQuestState.RootQuestId}
	}

	// 当前跑环任务完成的任务和当前的任务不一样
	finishQuestId := circuitQuestState.FinishQuestId
	curQuestId := circuitQuestState.Quest.Id
	if finishQuestId != curQuestId {
		return nil
	}

	//下发可接任务  去父ID组里面找
	var ids []uint32
	childVec := jsondata.RootTaskKv[finishQuestId]
	for _, id := range childVec {
		conf := jsondata.GetQuestConf(id)
		if conf == nil {
			continue
		}
		if !(conf.Prom.Type == custom_id.QptNpc || conf.Prom.Type == custom_id.QptManual) {
			continue
		}
		if !sys.CanAccept(conf) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// 接朝云悬赏任务
func (sys *CircuitQuestHelper) acceptDailyCircuitQuest(id uint32) *pb3.QuestData {
	var questData *pb3.QuestData
	circuitQuestState := sys.circuitQuestState()
	switch {
	case time_util.IsSameDay(circuitQuestState.AcceptedAt, time_util.NowSec()): // 同一天
		if circuitQuestState.FinishQuestId != circuitQuestState.Quest.Id {
			logger.LogWarn("exist circuit quest not finish")
			return nil
		}
		if !sys.isFinishEndRoundQuest() {
			circuitQuestState.Round++
			sys.owner.TriggerQuestEventRange(custom_id.QttDailyCircuitQuestRound)
		}
		questData = &pb3.QuestData{Id: id}
	default: // 跨天
		if circuitQuestState.RootQuestId == 0 {
			circuitQuestState.RootQuestId = id
		}
		circuitQuestState.AcceptedAt = time_util.NowSec()
		circuitQuestState.Round = 1
		questData = &pb3.QuestData{Id: id}
	}

	circuitQuestState.Quest = questData
	return questData
}

// 检查这一环最后一个跑环任务
func (sys *CircuitQuestHelper) isFinishEndRoundQuest() bool {
	circuitQuestState := sys.circuitQuestState()
	circuitQuestConf, ok := jsondata.GetCircuitQuestConfByRootQuestId(circuitQuestState.RootQuestId)
	if !ok {
		logger.LogWarn("circuit quest tree not exist")
		return false
	}
	curRoundQuests := circuitQuestConf.RoundConfs[len(circuitQuestConf.RoundConfs)-1]
	if circuitQuestState.FinishQuestId == curRoundQuests.QuestIds[len(curRoundQuests.QuestIds)-1] {
		if circuitQuestState.Round >= uint32(len(circuitQuestConf.RoundConfs)) {
			logger.LogWarn("daily circuit round over max")
			return false
		}
		return true
	}
	return false
}

func (sys *CircuitQuestHelper) GetQuest(id uint32) *pb3.QuestData {
	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.FinishQuestId == id {
		logger.LogWarn("already finish")
		return nil
	}
	if circuitQuestState.Quest == nil {
		return nil
	}
	if circuitQuestState.Quest.Id != id {
		logger.LogWarn("current circuit quest is incorrect, expect %d given %d", circuitQuestState.Quest.Id, id)
		return nil
	}
	return circuitQuestState.Quest
}

func (sys *CircuitQuestHelper) onAfterFinishQuest(conf *jsondata.StdQuest) (bool, []*pb3.StdAward) {
	circuitQuestState := sys.circuitQuestState()
	circuitQuestConf, ok := jsondata.GetCircuitQuestConfByRootQuestId(circuitQuestState.RootQuestId)
	if !ok {
		return false, nil
	}

	if circuitQuestState.Round == 0 || circuitQuestState.Round > uint32(len(circuitQuestConf.RoundConfs)) {
		return false, nil
	}

	// 日常任务
	roundConf := circuitQuestConf.RoundConfs[circuitQuestState.Round-1]
	if conf.Id == roundConf.QuestIds[len(roundConf.QuestIds)-1] && roundConf.DailyMissionId > 0 {
		sys.owner.TriggerEvent(custom_id.AeDailyMissionComplete, roundConf.DailyMissionId)
	}

	// 满环才有额外奖励
	var extAwards []*pb3.StdAward
	if int(roundConf.Round) >= len(circuitQuestConf.RoundConfs) && len(circuitQuestConf.ExtraAwards) > 0 {
		for _, award := range circuitQuestConf.ExtraAwards {
			stdAward := &pb3.StdAward{
				Id:        award.Id,
				Count:     award.Count,
				Bind:      award.Bind,
				Weight:    award.Weight,
				Job:       award.Job,
				Sex:       award.Sex,
				OpenDay:   award.OpenDay,
				Group:     award.Group,
				Broadcast: award.Broadcast,
			}
			extAwards = append(extAwards, stdAward)
		}
	}

	sys.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiDailyQuest, 1)
	return true, extAwards
}

func (sys *CircuitQuestHelper) fillIn(msg *pb3.S2C_7_1) {
	circuitQuestState := sys.circuitQuestState()
	if circuitQuestState.AcceptedAt > 0 &&
		time_util.IsSameDay(time_util.NowSec(), circuitQuestState.AcceptedAt) &&
		circuitQuestState.FinishQuestId != circuitQuestState.Quest.Id {
		msg.Circuit = circuitQuestState.Round
		msg.CircuitQuest = circuitQuestState.Quest
	}
}

func (sys *CircuitQuestHelper) batchGetNextCircuitQuestIds(reqRound uint32, isDouble bool) (uint32, []uint32, error) {
	var ids []uint32
	state := sys.circuitQuestState()
	conf, ok := jsondata.GetCircuitQuestConfByRootQuestId(state.RootQuestId)
	if !ok {
		return 0, nil, nil
	}

	rounds := len(conf.RoundConfs)
	if reqRound == 0 {
		reqRound = uint32(rounds)
	}

	var roundConfIdx int
	if state.Round > 0 {
		roundConfIdx = int(state.Round - 1)
	}
	curRoundQuests := conf.RoundConfs[roundConfIdx]

	if time_util.IsSameDay(state.AcceptedAt, time_util.NowSec()) {
		// 最后一环
		if state.Round == uint32(rounds) && state.FinishQuestId == curRoundQuests.QuestIds[len(curRoundQuests.QuestIds)-1] {
			logger.LogWarn("already finish all")
			return 0, nil, nil
		}
	}

	minRoundNotFinish := state.Round
	if state.FinishQuestId == curRoundQuests.QuestIds[len(curRoundQuests.QuestIds)-1] {
		minRoundNotFinish++
	}

	vipLevel := sys.owner.GetVipLevel()
	costConf := jsondata.GetCircuitQuestVipCostConf(vipLevel)
	var consumes jsondata.ConsumeVec
	if isDouble {
		consumes = append(consumes, costConf.DoubleCost...)
	}
	consumes = append(consumes, costConf.Consume...)

	if len(consumes) != 0 && !sys.owner.CheckConsumeByConf(consumes, false, 0) {
		return 0, nil, neterror.ConsumeFailedError("active consume not enough")
	}

	// 没接过任务
	if state.Quest == nil {
		ids = append(ids, state.RootQuestId)
		return reqRound, ids, nil
	}

	// 接了任务 但是没完成
	if state.Quest.Id != state.FinishQuestId {
		return 0, nil, nil
	}

	// 遍历接受所有子任务
	nextVec, ok := jsondata.RootTaskKv[state.FinishQuestId]
	if !ok {
		return 0, nil, nil
	}

	for _, nextId := range nextVec {
		nextConf := jsondata.GetQuestConf(nextId)
		if nil == nextConf {
			continue
		}

		if !nextConf.CheckLevel(sys.owner.GetLevel()) {
			continue
		}
		if !nextConf.CheckCircle(sys.owner.GetCircle()) {
			continue
		}

		if !nextConf.CheckDaysCond(gshare.GetOpenServerDay()) {
			continue
		}

		if nextConf.ParentId == state.FinishQuestId {
			ids = append(ids, nextId)
		}
	}
	return reqRound, ids, nil
}

func (sys *CircuitQuestHelper) canEnterMirrorFb(id uint32, hasAccept bool) bool {
	circuitQuestState := sys.circuitQuestState()
	if !hasAccept && nil != circuitQuestState && nil != circuitQuestState.Quest && circuitQuestState.Quest.Id == id {
		hasAccept = true
	}
	if !hasAccept { //未接受状态不可进副本
		return false
	}
	return true
}

func (sys *CircuitQuestHelper) doubleRecCircuitQuestAwardsConsumes(needConsumeQuicklyConsume bool) bool {
	vipLevel := sys.owner.GetVipLevel()
	costConf := jsondata.GetCircuitQuestVipCostConf(vipLevel)
	// 不可以双倍领取
	if costConf == nil || !costConf.CanDouble {
		sys.owner.SendTipMsg(tipmsgid.TpCircuitQuestNotCanDouble)
		return false
	}
	var cost jsondata.ConsumeVec
	if needConsumeQuicklyConsume {
		cost = append(cost, costConf.Consume...)
	}
	if len(costConf.DoubleCost) != 0 {
		cost = append(cost, costConf.DoubleCost...)
	}
	if len(cost) == 0 {
		return true
	}
	return sys.owner.ConsumeByConf(cost, false, common.ConsumeParams{LogId: pb3.LogId_LogDoubleRecCircuitQuestAwards})
}
