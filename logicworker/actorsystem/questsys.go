/**
 * @Author: twl
 * @Desc: 任务系统
 * @Date: 2023/02/09 14:22
 */

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/srvlib/utils"
)

func newQuestSys() iface.ISystem {
	sys := &QuestSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

// QuestSys 主线任务
type QuestSys struct {
	*QuestTargetBase
	circuitQuestHelper *CircuitQuestHelper
}

func (sys *QuestSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetMirrorData() {
		binary.MirrorData = make(map[uint32]*pb3.MirrorState)
	}
	if nil == binary.GetBranchRecord() {
		binary.BranchRecord = make(map[uint32]bool)
	}

	// 注入跑环任务助手
	sys.circuitQuestHelper = NewCircuitQuestHelper(sys.owner)
	sys.circuitQuestHelper.commonCanAccept = sys.commonCanAccept
}

func (sys *QuestSys) OnAfterLogin() {
	sys.tryAddMainQuest()
	sys.tryAddBranchQuest()
	sys.s2cQuestList()
	sys.s2cQuestWindowState()
}

func (sys *QuestSys) OnReconnect() {
	sys.s2cQuestList()
	sys.s2cQuestWindowState()
}

func (sys *QuestSys) OnLevelChange() {
	sys.tryAddMainQuest()
	sys.tryAddBranchQuest()
	if sys.circuitQuestHelper.onLevelChange() {
		sys.s2cAcceptable(true)
	}
	sys.s2cQuestList()
}

func (sys *QuestSys) onNewDay() {
	sys.circuitQuestHelper.resetCircuitQuestState()
	sys.restBranchQuest()
	sys.tryAddMainQuest()
	sys.tryAddBranchQuest()
	sys.s2cQuestList()
}

func (sys *QuestSys) restBranchQuest() {
	sys.delOverdueBranchQuest()
	sys.delOverdueBranchFinishId()
}

func (sys *QuestSys) delOverdueBranchQuest() {
	binary := sys.GetBinaryData()
	var ids []uint32
	var liveQuestList []*pb3.QuestData
	for _, quest := range binary.AllQuest {
		questId := quest.GetId()
		conf := jsondata.GetQuestConf(questId)
		if conf == nil {
			continue
		}
		if sys.IsMainTask(conf) {
			liveQuestList = append(liveQuestList, quest)
		} else if sys.IsBranchTask(conf) {
			switch conf.RfType {
			case custom_id.QRfDaily:
				ids = append(ids, questId)
			case custom_id.QRfExcludeDays:
				if !conf.CheckExcludeDays(gshare.GetOpenServerDay()) {
					continue
				}
				ids = append(ids, questId)
			default:
				liveQuestList = append(liveQuestList, quest)
			}
		}
	}
	// 重新放回
	binary.AllQuest = liveQuestList
	if len(ids) > 0 {
		sys.SendProto3(7, 9, &pb3.S2C_7_9{Ids: ids})
	}
}

func (sys *QuestSys) delOverdueBranchFinishId() {
	binary := sys.GetBinaryData()
	for questId := range binary.BranchRecord {
		conf := jsondata.GetQuestConf(questId)
		if conf == nil {
			continue
		}
		if conf.RfType == custom_id.QRfDaily {
			delete(binary.BranchRecord, questId)
		}
	}
}

// 添加主线任务 - 通过父类ID连接子类
func (sys *QuestSys) tryAddMainQuest() {
	currId := sys.GetBinaryData().FinMainQuestId
	sys.checkAddQuest(currId, custom_id.QtMain, false)
}

func (sys *QuestSys) isBranchFinish(id uint32) bool {
	binary := sys.GetBinaryData()
	record := binary.GetBranchRecord()
	if isFinish, ok := record[id]; ok {
		return isFinish
	}
	return false
}

func (sys *QuestSys) tryAddBranchQuest() {
	for _, questId := range jsondata.RootTaskKv[0] {
		questConf := jsondata.GetQuestConf(questId)
		if nil == questConf {
			continue
		}
		if questConf.Type != custom_id.QtBranch || questConf.Prom.Type != custom_id.QptAuto {
			continue
		}
		//todo 接取前置条件
		if sys.isBranchFinish(questId) {
			continue
		}
		if nil != sys.GetQuest(questId) {
			continue
		}
		sys.AddQuest(questId, true, questConf)
	}
}

func (sys *QuestSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	binary := sys.GetPlayerData()
	if nil == binary {
		return nil
	}
	set := make(map[uint32]struct{})
	for _, quest := range binary.BinaryData.AllQuest {
		questId := quest.GetId()
		conf := jsondata.GetQuestConf(questId)
		if nil == conf {
			continue
		}
		for _, target := range conf.Target {
			if target.Type == qt {
				set[questId] = struct{}{}
			}
		}
	}

	idSet := sys.circuitQuestHelper.getQuestIdSet()
	for id := range idSet {
		set[id] = struct{}{}
	}

	return set
}

func (sys *QuestSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	binary := sys.GetPlayerData()
	if nil == binary {
		return nil
	}
	for _, quest := range binary.BinaryData.AllQuest {
		if quest.GetId() == id {
			return quest
		}
	}

	questData := sys.circuitQuestHelper.getUnFinishQuestData(id)
	if questData != nil {
		return questData
	}

	return nil
}

func (sys *QuestSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	if conf := jsondata.GetQuestConf(id); nil != conf {
		return conf.Target
	}
	return nil
}

func (sys *QuestSys) onUpdateTargetData(questId uint32) {
	quest := sys.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	conf := jsondata.GetQuestConf(questId)
	if nil == conf {
		return
	}

	var round uint32
	if sys.IsDailyCircuit(conf) {
		round = sys.circuitQuestHelper.getCircuitTaskRound()
	}

	resp := pb3.NewS2C_7_2()
	defer pb3.RealeaseS2C_7_2(resp)
	resp.Data = quest
	resp.Circuit = round
	sys.SendProto3(7, 2, resp)

	comp := conf.Comp
	if nil != comp && comp.Type == custom_id.QctAuto { //自动完成
		sys.Finish(argsdef.NewFinishQuestParams(questId, false, false, true))
	}
}

// GetQuest 判断任务是否已接取
func (sys *QuestSys) GetQuest(id uint32) *pb3.QuestData {
	binary := sys.GetBinaryData()
	if nil == binary {
		return nil
	}
	for _, quest := range binary.AllQuest {
		if quest.GetId() == id {
			return quest
		}
	}
	return nil
}

// IsMainTask 判断是否是主线任务
func (sys *QuestSys) IsMainTask(conf *jsondata.StdQuest) bool {
	return conf.Type == custom_id.QtMain
}

func (sys *QuestSys) IsBranchTask(conf *jsondata.StdQuest) bool {
	return conf.Type == custom_id.QtBranch
}

func (sys *QuestSys) IsDailyCircuit(conf *jsondata.StdQuest) bool {
	return conf.Type == custom_id.QtCircuit
}

func (sys *QuestSys) IsSectTask(conf *jsondata.StdQuest) bool {
	return conf.Type == custom_id.QtSect
}

// IsFinishMainQuest 是否已完成主线任务
func (sys *QuestSys) IsFinishMainQuest(id uint32) bool {
	binary := sys.GetPlayerData()
	if nil == binary {
		return false
	}
	return binary.BinaryData.GetFinMainQuestId() >= id
}

// SetFinish 设置任务完成
func (sys *QuestSys) SetFinish(id uint32) bool {
	binary := sys.GetBinaryData()
	if nil == binary {
		return false
	}

	conf := jsondata.GetQuestConf(id)
	if conf == nil {
		return false
	}

	if sys.IsDailyCircuit(conf) && !sys.circuitQuestHelper.SetFinish(id) {
		return false
	}

	for idx, quest := range binary.AllQuest {
		if quest.GetId() != id {
			continue
		}
		if sys.IsMainTask(conf) {
			binary.FinMainQuestId = id
			sys.owner.TriggerEvent(custom_id.AeFinishMainQuest, conf.ParentId, id)
			sys.owner.TriggerQuestEventRange(custom_id.QttFinishMainTask)
		}
		if sys.IsBranchTask(conf) {
			binary.BranchRecord[id] = true
		}
		last := len(binary.AllQuest) - 1
		binary.AllQuest[idx] = binary.AllQuest[last]
		binary.AllQuest[last] = nil
		binary.AllQuest = binary.AllQuest[:last]
		return true
	}

	return false
}

// GetCurMainTask 获取当前进行中的主线任务
func (sys *QuestSys) GetCurMainTask() *pb3.QuestData {
	binary := sys.GetPlayerData()
	if nil == binary {
		return nil
	}

	for _, quest := range binary.BinaryData.AllQuest {
		if conf := jsondata.GetQuestConf(quest.GetId()); nil != conf {
			if sys.IsMainTask(conf) {
				return quest
			}
		}
	}
	return nil
}

func (sys *QuestSys) commonCanAccept(conf *jsondata.StdQuest) bool {
	if !conf.CheckLevel(sys.owner.GetLevel()) {
		return false
	}

	if !conf.CheckCircle(sys.owner.GetCircle()) {
		return false
	}

	if !conf.CheckDaysCond(gshare.GetOpenServerDay()) {
		return false
	}
	return true
}

// CanAccept 能否接取任务
func (sys *QuestSys) CanAccept(conf *jsondata.StdQuest) bool {
	if !sys.commonCanAccept(conf) {
		return false
	}

	switch {
	case sys.IsMainTask(conf):
		if sys.GetCurMainTask() != nil || sys.IsFinishMainQuest(conf.Id) { //当前有主线任务
			return false
		}
	case sys.IsBranchTask(conf):
		if nil != sys.GetQuest(conf.Id) || sys.isBranchFinish(conf.Id) {
			return false
		}
	case sys.IsDailyCircuit(conf):
		return sys.circuitQuestHelper.CanAccept(conf)
	case sys.IsSectTask(conf):
		return false
	}

	return true
}

// AddQuest 接受任务
func (sys *QuestSys) AddQuest(id uint32, add bool, conf *jsondata.StdQuest) bool {
	if !sys.CanAccept(conf) {
		return false
	}

	if add {
		sys.onAccept(id, conf)
	}

	return true
}

// ChangeTargetVal 设置任务进度
func (sys *QuestSys) ChangeTargetVal(quest *pb3.QuestData, idx int, value uint32) bool {
	conf := jsondata.GetQuestConf(quest.GetId())
	if nil == conf {
		return false
	}

	len1, len2 := len(quest.Progress), len(conf.Target)
	if idx >= len2 {
		return false
	}

	for i := len1; i < len2; i++ {
		quest.Progress = append(quest.Progress, 0)
	}
	if quest.Progress[idx] >= conf.Target[idx].Count {
		return false
	}
	quest.Progress[idx] = utils.MinUInt32(value, conf.Target[idx].Count)
	return true
}

// 接受任务
func (sys *QuestSys) onAccept(id uint32, conf *jsondata.StdQuest) {
	// 接朝云悬赏
	if sys.IsDailyCircuit(conf) {
		questData := sys.circuitQuestHelper.acceptDailyCircuitQuest(id)
		if questData == nil {
			sys.owner.LogWarn("acceptDailyCircuitQuest failed id:%d", id)
			return
		}
		sys.QuestTargetBase.OnAcceptQuest(questData)
		resp := pb3.NewS2C_7_2()
		defer pb3.RealeaseS2C_7_2(resp)
		resp.Data = questData
		resp.Circuit = sys.circuitQuestHelper.getCircuitTaskRound()
		sys.SendProto3(7, 2, resp)
		logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogQuestAccept, &pb3.LogPlayerCounter{NumArgs: uint64(id)})
		return
	}

	// 接默认主线 支线
	questLs := sys.GetBinaryData().AllQuest
	for _, q := range questLs {
		if q.Id != id {
			continue
		}
		// 存在相同的 停止新增
		return
	}
	questData := &pb3.QuestData{Id: id}
	sys.GetBinaryData().AllQuest = append(sys.GetBinaryData().AllQuest, questData)
	sys.QuestTargetBase.OnAcceptQuest(questData)
	sys.onUpdateTargetData(questData.GetId())
	// 打点
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogQuestAccept, &pb3.LogPlayerCounter{NumArgs: uint64(id)})
}

// Finish 完成任务
func (sys *QuestSys) Finish(params *argsdef.FinishQuestParams) bool {
	id, imme, forceAutoAcceptNext, sendTick := params.QuestId, params.Imme, params.ForceAutoAcceptNext, params.SendTick
	conf := jsondata.GetQuestConf(id)
	if nil == conf {
		return false
	}

	var quest *pb3.QuestData
	switch {
	case sys.IsDailyCircuit(conf):
		quest = sys.circuitQuestHelper.GetQuest(id)
	default:
		quest = sys.GetQuest(id)
	}
	if nil == quest {
		return false
	}

	// 是否需要检查任务是否可以完成
	if !imme && !sys.canFinish(quest) {
		return false
	}

	// 日常任务双倍校验
	if params.IsDoubleCircuitQuestAwards && sys.IsDailyCircuit(conf) && !sys.circuitQuestHelper.doubleRecCircuitQuestAwardsConsumes(params.NeedQuickConsume) {
		return false
	}

	sys.s2cFinishQuest(id, sendTick)

	sys.SetFinish(id)

	questLog := func(questConf *jsondata.StdQuest) pb3.LogId {
		if sys.IsDailyCircuit(conf) {
			return pb3.LogId_LogDailyCircuitQuestAwards
		}

		if sys.IsMainTask(conf) {
			return pb3.LogId_LogMainQuestAwards
		}

		if sys.IsSectTask(conf) {
			return pb3.LogId_LogSectTaskQuestAwards
		}
		return pb3.LogId_LogQuestAward
	}
	if len(conf.Awards) > 0 {
		var awards jsondata.StdRewardVec
		for _, award := range conf.Awards {
			cpAward := &jsondata.StdReward{
				Id:           award.Id,
				Count:        award.Count,
				Bind:         award.Bind,
				Job:          award.Job,
				Sex:          award.Sex,
				OpenDay:      award.OpenDay,
				Weight:       award.Weight,
				Group:        award.Group,
				Broadcast:    award.Broadcast,
				Extra:        award.Extra,
				Quality:      award.Quality,
				QualityAttrs: award.QualityAttrs,
			}
			if params.IsDoubleCircuitQuestAwards {
				cpAward.Count = cpAward.Count * 2
			}
			awards = append(awards, cpAward)
		}

		if sys.IsDailyCircuit(conf) {
			addRate := sys.owner.GetFightAttr(attrdef.CircuitQuestAwardAdd)
			awards = jsondata.CalcStdRewardByRate(awards, addRate)
		}
		engine.GiveRewards(sys.owner, awards, common.EngineGiveRewardParam{LogId: questLog(conf)})
	}

	if !sys.onAfterFinishQuest(conf) {
		return false
	}

	// 主动检测承接下一个任务
	sys.checkAddQuest(id, conf.Type, forceAutoAcceptNext)

	for _, target := range conf.Target {
		if target.Type != custom_id.QttPassMirrorFb {
			break
		}
		binary := sys.GetBinaryData()
		for _, tar := range target.Ids {
			delete(binary.MirrorData, tar)
		}
	}
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogQuestFinish, &pb3.LogPlayerCounter{NumArgs: uint64(id)})
	return true
}

func (sys *QuestSys) s2cHasMasterQuest() {
	var hasMasterQuest bool
	for _, data := range sys.GetBinaryData().AllQuest {
		questConf := jsondata.GetQuestConf(data.Id)
		if questConf == nil {
			continue
		}
		if !sys.IsMainTask(questConf) {
			continue
		}
		hasMasterQuest = true
		break
	}
	var finMainQuestId uint32
	if !hasMasterQuest {
		finMainQuestId = sys.GetBinaryData().FinMainQuestId
	}
	sys.SendProto3(7, 12, &pb3.S2C_7_12{
		HasMasterQuest: hasMasterQuest,
		FinMainQuestId: finMainQuestId,
	})
}

func (sys *QuestSys) onAfterFinishQuest(conf *jsondata.StdQuest) bool {
	var ret bool
	switch {
	case sys.IsDailyCircuit(conf):
		var extAwards []*pb3.StdAward
		ret, extAwards = sys.circuitQuestHelper.onAfterFinishQuest(conf)
		if ret && len(extAwards) > 0 {
			addRate := sys.owner.GetFightAttr(attrdef.CircuitQuestAwardAdd)
			vec := jsondata.Pb3RewardVecToStdRewardVec(extAwards)
			vec = jsondata.CalcStdRewardByRate(vec, addRate)
			engine.GiveRewards(sys.owner, vec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQuestAward})
			sys.SendProto3(7, 26, &pb3.S2C_7_26{
				Rewards: extAwards,
			})
		}
	default:
		ret = true
	}
	return ret
}

// 检查添加下一个任务
func (sys *QuestSys) checkAddQuest(currFinId uint32, questType uint32, forceAccept bool) bool {
	conf := jsondata.GetQuestConf(currFinId)
	if currFinId > 0 && conf == nil {
		return false
	}

	// 有一种可能前端请求完成任务升级,在升级的时候触发事件已经接过 currFinId 接下来的任务了, 此时再处理 c2s_7_5 传过来的 currFinId 就有问题了.
	if sys.GetBinaryData().FinMainQuestId > 0 && sys.IsMainTask(conf) && currFinId != sys.GetBinaryData().FinMainQuestId {
		return false
	}

	if nextVec, ok := jsondata.RootTaskKv[currFinId]; ok {
		for _, nextId := range nextVec { // 遍历接受所有子任务
			nextConf := jsondata.GetQuestConf(nextId)
			if nil == nextConf {
				continue
			}

			//
			//if nextConf.Type != questType {
			//	continue
			//}

			// 判断能否接子任务
			if !sys.CanAccept(nextConf) {
				continue
			}

			if nextConf.Prom == nil {
				continue
			}

			if nextConf.Prom.Type != custom_id.QptAuto && !forceAccept {
				continue
			}

			if nextConf.ParentId == currFinId {
				sys.onAccept(nextId, nextConf)
			}
		}
	}
	return true
}

// 判断任务能否完成
func (sys *QuestSys) canFinish(quest *pb3.QuestData) bool {
	conf := jsondata.GetQuestConf(quest.GetId())
	if nil == conf {
		return false
	}
	len1, len2 := len(quest.Progress), len(conf.Target)
	if len1 < len2 {
		return false
	}
	for idx, target := range conf.Target {
		if quest.Progress[idx] < target.Count {
			return false
		}
	}
	return true
}

func (sys *QuestSys) allowDirectAccept(conf *jsondata.StdQuest) bool {
	if sys.IsMainTask(conf) || sys.IsBranchTask(conf) {
		return true
	}
	return false
}

// 下发可接任务列表
func (sys *QuestSys) s2cAcceptable(resetRecCircuitQuest bool) {
	binary := sys.GetPlayerData()
	if nil == binary {
		return
	}

	finId := binary.BinaryData.GetFinMainQuestId()
	childVec := jsondata.RootTaskKv[finId]
	//下发可接任务  去父ID组里面找
	var ids []uint32
	for _, id := range childVec {
		conf := jsondata.GetQuestConf(id)
		if conf == nil {
			continue
		}
		if !sys.allowDirectAccept(conf) {
			continue
		}
		if !sys.CanAccept(conf) {
			continue
		}
		ids = append(ids, id)
	}

	circuitQuestIds := sys.circuitQuestHelper.getAcceptableCircuitQuestIds(resetRecCircuitQuest)
	ids = append(ids, circuitQuestIds...)

	if len(ids) <= 0 {
		return
	}

	msg := &pb3.S2C_7_3{Ids: ids}
	if round := sys.circuitQuestHelper.getCircuitTaskRound(); round > 0 {
		msg.CircuitTaskRound = round
	}
	sys.SendProto3(7, 3, msg)

}

// 下发新增任务
func (sys *QuestSys) s2cUpdateQuest(quest *pb3.QuestData) {
	resp := pb3.NewS2C_7_2()
	defer pb3.RealeaseS2C_7_2(resp)
	resp.Data = quest
	sys.SendProto3(7, 2, resp)
}

// 下发任务列表
func (sys *QuestSys) s2cQuestList() {
	if binary := sys.GetPlayerData(); nil != binary {
		msg := &pb3.S2C_7_1{
			Datas: binary.BinaryData.AllQuest,
		}
		sys.circuitQuestHelper.fillIn(msg)
		sys.SendProto3(7, 1, msg)
	}
	sys.s2cAcceptable(false)
}

// 玩家手动接取任务
func (sys *QuestSys) c2sAccept(msg *base.Message) {
	var req pb3.C2S_7_6
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		return
	}
	id := req.GetId()
	conf := jsondata.GetQuestConf(id)
	if nil == conf {
		return
	}
	prom := conf.Prom
	if nil == prom {
		return
	}
	sys.AddQuest(id, true, conf)
}

// 请求从npc接取任务
func (sys *QuestSys) c2sNpcAccept(msg *base.Message) {
	var req pb3.C2S_7_4
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		return
	}
	id := req.GetId()
	conf := jsondata.GetQuestConf(id)
	if nil == conf {
		return
	}

	prom := conf.Prom
	if prom == nil || prom.Type != custom_id.QptNpc {
		return //不是从npc上接取的任务
	}

	sys.AddQuest(id, true, conf)
}

// 玩家手动提交任务
func (sys *QuestSys) c2sFinish(msg *base.Message) {
	var req pb3.C2S_7_7
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		return
	}
	questParams := argsdef.NewFinishQuestParams(req.GetId(), false, false, true)
	questParams.IsDoubleCircuitQuestAwards = req.IsDoubleCircuitQuestAwards
	sys.Finish(questParams)
	sys.s2cAcceptable(false)
}

// 请求从npc提交任务
func (sys *QuestSys) c2sNpcFinish(msg *base.Message) {
	var req pb3.C2S_7_5
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		return
	}
	id := req.GetId()
	conf := jsondata.GetQuestConf(id)
	if nil == conf {
		return
	}

	comp := conf.Comp
	if comp == nil || comp.Type != custom_id.QctNpc {
		return //不是从npc上提交的任务
	}

	questParams := argsdef.NewFinishQuestParams(req.GetId(), false, false, true)
	questParams.IsDoubleCircuitQuestAwards = req.IsDoubleCircuitQuestAwards
	sys.Finish(questParams)

	if sys.IsDailyCircuit(conf) || sys.IsMainTask(conf) {
		sys.s2cAcceptable(false)
	}
}

// 下发完成任务
func (sys *QuestSys) s2cFinishQuest(id uint32, sendTick bool) {
	sys.SendProto3(7, 8, &pb3.S2C_7_8{Id: id, NeedTick: sendTick})
}

func (sys *QuestSys) c2sQuickFinishDailyQuest(msg *base.Message) {
	var req pb3.C2S_7_27
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		sys.LogError(err.Error())
		return
	}

	// 用跑环任务的数量来做循环批量完成
	state := sys.circuitQuestHelper.circuitQuestState()
	conf, ok := jsondata.GetCircuitQuestConfByRootQuestId(state.RootQuestId)
	if !ok {
		return
	}
	count := len(conf.RoundConfs)

	reqRound, ids, err := sys.circuitQuestHelper.batchGetNextCircuitQuestIds(req.Round, req.IsDoubleCircuitQuestAwards)
	if err != nil {
		sys.LogError("err:%v", err)
		return
	}

	for _, id := range ids {
		sys.onAccept(id, jsondata.GetQuestConf(id))
	}

	questParams := argsdef.NewFinishQuestParams(state.Quest.Id, true, true, true)
	questParams.IsDoubleCircuitQuestAwards = req.IsDoubleCircuitQuestAwards
	questParams.NeedQuickConsume = true
	finish := sys.Finish(questParams)
	if !finish {
		return
	}

	for i := 0; i < count; i++ {
		if state.FinishQuestId == state.Quest.Id || state.Quest == nil || state.Round > reqRound {
			break
		}
		questParams = argsdef.NewFinishQuestParams(state.Quest.Id, true, true, false)
		questParams.IsDoubleCircuitQuestAwards = req.IsDoubleCircuitQuestAwards
		questParams.NeedQuickConsume = true
		finish := sys.Finish(questParams)
		if !finish {
			break
		}
	}
}

// 玩家等级变动
func onQuestSysLevelChange(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiQuest).(*QuestSys); ok {
		sys.OnLevelChange()
	}
}

func onQuestSysNewDay(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiQuest).(*QuestSys); ok && sys.IsOpen() {
		sys.onNewDay()
	}
}

func GmQuest(actor iface.IPlayer, args ...string) bool {
	sys, ok := actor.GetSysObj(sysdef.SiQuest).(*QuestSys)
	if !ok {
		return false
	}
	size := len(args)
	if size <= 0 {
		return false
	}
	switch args[0] {
	case "finish":
		if quest := sys.GetCurMainTask(); nil != quest {
			sys.Finish(argsdef.NewFinishQuestParams(quest.GetId(), true, false, true))
		}
	case "finishAll":
		for {
			if quest := sys.GetCurMainTask(); nil != quest {
				sys.Finish(argsdef.NewFinishQuestParams(quest.GetId(), true, false, true))
			} else {
				break
			}
		}
	case "finish.taskId":
		if size <= 1 {
			return false
		}
		id := utils.AtoUint32(args[1])
		sys.Finish(argsdef.NewFinishQuestParams(id, true, false, true))
	case "to":
		if size <= 1 {
			return false
		}
		id := utils.AtoUint32(args[1])
		if quest := sys.GetCurMainTask(); nil != quest {
			sys.SetFinish(quest.GetId())
			sys.s2cFinishQuest(quest.GetId(), true)
		}
		conf := jsondata.GetQuestConf(id)
		if nil == conf {
			return false
		}
		if sys.IsMainTask(conf) {
			if binary := sys.GetPlayerData(); nil != binary {
				actor.TriggerEvent(custom_id.AeFinishMainQuest, conf.ParentId, id)
			}
		}
		sys.onAccept(id, conf)
	}

	return true
}

// 请求进入副本
func (sys *QuestSys) c2sEnterMirrorFb(msg *base.Message) error {
	var req pb3.C2S_17_251
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	id := req.GetQuestId()
	conf := jsondata.GetQuestConf(id)
	if nil == conf {
		return neterror.ParamsInvalidError("quest conf(%d) is nil", id)
	}
	hasAccept := false
	questList := sys.GetBinaryData().GetAllQuest()
	for _, quest := range questList {
		if quest.GetId() == id {
			hasAccept = true
			break
		}
	}

	if !sys.circuitQuestHelper.canEnterMirrorFb(id, hasAccept) {
		return nil
	}

	mirrorId := req.GetMirrorId()
	canEnter := false
	for _, target := range conf.Target {
		if target.Type != custom_id.QttPassMirrorFb {
			break
		}
		for _, tar := range target.Ids {
			if tar == mirrorId {
				canEnter = true
				break
			}
		}
	}
	if !canEnter { //不能进未指定的镜像副本
		return nil
	}
	reqEnter := &pb3.EnterMirrorFb{MirrorId: mirrorId, X: req.GetX(), Y: req.GetY()}
	binary := sys.GetBinaryData()
	if mirror, ok := binary.MirrorData[mirrorId]; ok {
		reqEnter.Order = mirror.Order
		reqEnter.Progress = mirror.Progress
	} else {
		binary.MirrorData[mirrorId] = &pb3.MirrorState{}
	}
	err = sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterMirror, reqEnter)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
	}
	return nil
}

func (sys *QuestSys) getQuestWindowState() *pb3.QuestWindow {
	if sys.GetBinaryData().QuestWindowState == nil {
		sys.GetBinaryData().QuestWindowState = &pb3.QuestWindow{}
	}
	return sys.GetBinaryData().QuestWindowState
}

func (sys *QuestSys) s2cQuestWindowState() {
	sys.SendProto3(7, 30, &pb3.S2C_7_30{
		State: sys.getQuestWindowState(),
	})
}

func (sys *QuestSys) c2sRecQuestWindowAwards(msg *base.Message) error {
	var req pb3.C2S_7_30
	err := pb3.Unmarshal(msg.Data, &req)
	if nil != err {
		return neterror.Wrap(err)
	}
	finMainQuestId := sys.GetBinaryData().GetFinMainQuestId()
	questPreConf, ok := jsondata.GetQuestPreConf()
	if !ok {
		return neterror.ConfNotFoundError("quest pre conf not found")
	}
	state := sys.getQuestWindowState()

	if pie.Uint32s(state.RecQuestIds).Contains(req.QuestId) {
		return neterror.ParamsInvalidError("already rec quest pre awards , quest id is %d", req.QuestId)
	}

	if finMainQuestId < req.QuestId {
		return neterror.ParamsInvalidError("already rec quest pre awards , quest id is %d , main fin is %d", req.QuestId, finMainQuestId)
	}

	stepList, ok := questPreConf[fmt.Sprintf("%d", req.Id)]
	if !ok {
		return neterror.ConfNotFoundError("quest pre step conf not found , id is %d", req.Id)
	}

	var recStep *jsondata.QuestPreConfStep
	for _, step := range stepList.Step {
		if step.QuestId != req.QuestId {
			continue
		}
		recStep = step
		break
	}
	if recStep == nil {
		return neterror.ConfNotFoundError("quest pre step conf not found , id is %d , quest id %d", req.Id, req.QuestId)
	}

	sys.GetOwner().TriggerQuestEvent(custom_id.QttRecQuestWindowAwards, recStep.QuestId, 1)
	sys.getQuestWindowState().RecQuestIds = append(sys.getQuestWindowState().RecQuestIds, recStep.QuestId)
	if len(recStep.Award) > 0 {
		engine.GiveRewards(sys.GetOwner(), recStep.Award, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogQuestWindowsRecAwards,
		})
	}

	sys.s2cQuestWindowState()
	return nil
}

func (sys *QuestSys) GMReAcceptQuest(questId uint32) {
	if binary := sys.GetPlayerData(); nil != binary {
		for _, data := range binary.BinaryData.AllQuest {
			if data.Id != questId {
				continue
			}
			sys.OnAcceptQuestAndCheckUpdateTarget(data)
		}
	}
}

func syncMirrorProgress(player iface.IPlayer, buf []byte) {
	msg := &pb3.SyncMirrorProgress{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}
	conf := jsondata.GetMirrorFbConf(msg.MirrorId)
	if nil == conf {
		return
	}
	binary := player.GetBinaryData()
	mirrorData := binary.GetMirrorData()
	if _, ok := mirrorData[msg.MirrorId]; !ok {
		return
	}
	mirror := mirrorData[msg.MirrorId]
	if mirror.Order != msg.Order {
		mirror.Order = msg.Order
		mirror.Progress = nil
	}

	if nil == msg.Progress {
		return
	}

	mirror.Progress = msg.Progress
}

func useItemQuickUpLvAndQuest(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) != 2 {
		return
	}
	maxLv := conf.Param[0]
	targetQuestId := conf.Param[1]
	if maxLv > 200 {
		maxLv = 200
	}
	var canUse bool
	if player.GetLevel() < maxLv {
		canUse = true
		sys, ok := player.GetSysObj(sysdef.SiQuest).(*QuestSys)
		if !ok {
			return
		}
		mainTask := sys.GetCurMainTask()
		mainTaskId := mainTask.Id
		for mainTaskId < targetQuestId {
			sys.Finish(argsdef.NewFinishQuestParams(mainTaskId, true, true, true))
			questData := sys.GetCurMainTask()
			mainTaskId = questData.Id
		}
	}
	for player.GetLevel() < maxLv {
		needExp, ok := jsondata.GetLevelConfig(player.GetLevel() + 1)
		if !ok {
			return
		}
		player.AddExp(needExp, pb3.LogId_LogUseItemQuickUpLvAndQuest, false)
	}
	if canUse {
		// 前端会连续请求五次升级境界 这边先暴力处理
		circle := player.GetCircle()
		player.SetExtraAttr(attrdef.Circle, 5)
		player.TriggerEvent(custom_id.AeAfterUseItemQuickUpLvAndQuest)
		player.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(circle))
	}
	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiQuest, newQuestSys)
	net.RegisterSysProto(7, 4, sysdef.SiQuest, (*QuestSys).c2sNpcAccept)
	net.RegisterSysProto(7, 5, sysdef.SiQuest, (*QuestSys).c2sNpcFinish)
	net.RegisterSysProto(7, 6, sysdef.SiQuest, (*QuestSys).c2sAccept)
	net.RegisterSysProto(7, 7, sysdef.SiQuest, (*QuestSys).c2sFinish)
	net.RegisterSysProto(17, 251, sysdef.SiQuest, (*QuestSys).c2sEnterMirrorFb)
	net.RegisterSysProto(7, 27, sysdef.SiQuest, (*QuestSys).c2sQuickFinishDailyQuest)
	net.RegisterSysProtoV2(7, 30, sysdef.SiQuest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*QuestSys).c2sRecQuestWindowAwards
	})
	event.RegActorEvent(custom_id.AeLevelUp, onQuestSysLevelChange)
	event.RegActorEvent(custom_id.AeCircleChange, onQuestSysLevelChange)
	event.RegActorEvent(custom_id.AeNewDay, onQuestSysNewDay)

	engine.RegisterActorCallFunc(playerfuncid.SyncMirrorProgress, syncMirrorProgress)

	engine.RegisterActorCallFunc(playerfuncid.TriggerQuestEvent, func(player iface.IPlayer, buf []byte) {
		var msg pb3.CommonSt
		if err := pb3.Unmarshal(buf, &msg); err != nil {
			return
		}

		player.TriggerQuestEvent(msg.U32Param, msg.U32Param2, msg.I64Param)
	})

	engine.RegQuestTargetProgress(custom_id.QttDailyCircuitQuestRound, handleDailyCircuitQuestRound)
	engine.RegQuestTargetProgress(custom_id.QttRecQuestWindowAwards, handleRecQuestWindowAwards)
	engine.RegQuestTargetProgress(custom_id.QttFinishMainTask, handleFinishMainTask)
	initQuestGm()
	miscitem.RegCommonUseItemHandle(itemdef.UseItemQuickUpLvAndQuest, useItemQuickUpLvAndQuest)
}

func initQuestGm() {
	gmevent.Register("task", GmQuest, 1)

	gmevent.Register("mirror", func(actor iface.IPlayer, args ...string) bool {
		if len(args) <= 0 {
			return false
		}
		mirrorId := utils.AtoUint32(args[0])
		x := utils.AtoInt32(args[1])
		y := utils.AtoInt32(args[2])
		reqEnter := &pb3.EnterMirrorFb{MirrorId: mirrorId, X: x, Y: y}
		actor.EnterFightSrv(base.LocalFightServer, fubendef.EnterMirror, reqEnter)
		return true
	}, 1)

	gmevent.Register("quest.c2sRecQuestWindowAwards", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(7<<8 | 30)
		err := msg.PackPb3Msg(&pb3.C2S_7_30{
			Id:      utils.AtoUint32(args[0]),
			QuestId: utils.AtoUint32(args[1]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(7, 30, msg)
		return true
	}, 1)

	gmevent.Register("quest.resetCircuitQuestState", func(player iface.IPlayer, args ...string) bool {
		var ids []uint32

		questSys := player.GetSysObj(sysdef.SiQuest).(*QuestSys)
		sys := questSys.circuitQuestHelper
		sys.resetCircuitQuestState()
		circuitQuestId := utils.AtoUint32(args[0])
		circuitQuestState := sys.circuitQuestState()
		circuitQuestState.RootQuestId = circuitQuestId
		circuitQuestState.SpecRootQuestAt = time_util.NowSec()
		ids = append(ids, circuitQuestId)
		binary := questSys.GetPlayerData()
		if nil == binary {
			return false
		}
		finId := binary.BinaryData.GetFinMainQuestId()
		childVec := jsondata.RootTaskKv[finId]

		//下发可接任务  去父ID组里面找
		if nil != childVec {
			for _, id := range childVec {
				conf := jsondata.GetQuestConf(id)
				if nil != conf && questSys.allowDirectAccept(conf) {
					if sys.CanAccept(conf) {
						ids = append(ids, id)
					}
				}
			}
		}
		if len(ids) <= 0 {
			return false
		}

		msg := &pb3.S2C_7_3{Ids: ids}
		player.SendProto3(7, 3, msg)
		return true
	}, 1)

	gmevent.Register("s2cUpdateQuest", func(player iface.IPlayer, args ...string) bool {
		questSys := player.GetSysObj(sysdef.SiQuest).(*QuestSys)
		circuitQuestId := utils.AtoUint32(args[0])
		for _, data := range questSys.GetBinaryData().AllQuest {
			if data.Id == circuitQuestId {
				questSys.s2cUpdateQuest(data)
			}
		}
		return true
	}, 1)
}

func handleRecQuestWindowAwards(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	questId := ids[0]
	if sys, ok := actor.GetSysObj(sysdef.SiQuest).(*QuestSys); ok {
		state := sys.getQuestWindowState()
		if pie.Uint32s(state.RecQuestIds).Contains(questId) {
			return 1
		}
	}
	return 0
}

func handleFinishMainTask(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	questId := ids[0]
	sys, ok := actor.GetSysObj(sysdef.SiQuest).(*QuestSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	if !sys.IsFinishMainQuest(questId) {
		return 0
	}
	return 1
}

func handleDailyCircuitQuestRound(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	if sys, ok := actor.GetSysObj(sysdef.SiQuest).(*QuestSys); ok {
		return sys.circuitQuestHelper.getCircuitTaskRound()
	}
	return 0
}
