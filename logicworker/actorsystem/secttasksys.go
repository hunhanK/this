package actorsystem

import (
	"jjyz/base"
	"jjyz/base/actsweepmgr"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
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

	"github.com/gzjjyz/srvlib/utils"

	"github.com/gzjjyz/random"
)

var (
	SectQuestTargetMap map[uint32]map[uint32]struct{}
)

type SectTaskSys struct {
	*QuestTargetBase
}

func newSectTaskSys() iface.ISystem {
	sys := &SectTaskSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (sys *SectTaskSys) s2cInfo() {
	sys.SendProto3(46, 0, &pb3.S2C_46_0{Task: sys.GetData()})
}

func (sys *SectTaskSys) OnOpen() {
	if err := sys.refresh(true); nil != err {
		sys.LogError("sect refresh err:%v", err)
	}
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepBySectTask, sys.GetOwner().GetId())
}

func (sys *SectTaskSys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *SectTaskSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *SectTaskSys) GetData() *pb3.SectTask {
	binary := sys.GetBinaryData()
	if nil == binary.SectTask {
		binary.SectTask = &pb3.SectTask{}
	}
	return binary.GetSectTask()
}

func (sys *SectTaskSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := SectQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

const (
	sectTaskNoAccept = 0 //未接取
	sectTaskRunning  = 1 //进行中
	sectTaskGiveUp   = 2 //已放弃
	sectTaskCommit   = 3 //已完成
)

func (sys *SectTaskSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	sectTask := sys.GetData()
	for _, task := range sectTask.TaskList {
		if task.QuestId == id && task.Status == sectTaskRunning {
			return task.Quest
		}
	}
	return nil
}

func (sys *SectTaskSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	if conf := jsondata.GetSectQuestTaskConf(id); nil != conf {
		if qConf := jsondata.GetQuestConf(id); nil != qConf {
			return qConf.Target
		}
	}
	return nil
}

func (sys *SectTaskSys) onUpdateTargetData(questId uint32) {
	quest := sys.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	//更新任务
	sys.SendProto3(46, 3, &pb3.S2C_46_3{Quest: quest})
}

func onAfterReloadSectTaskConf(args ...interface{}) {
	tmp := make(map[uint32]map[uint32]struct{})
	if nil == jsondata.SectTaskConfMgr || nil == jsondata.SectTaskConfMgr.TaskPool {
		return
	}
	taskList := jsondata.SectTaskConfMgr.TaskPool
	for id := range taskList {
		if conf := jsondata.GetQuestConf(id); nil != conf {
			for _, target := range conf.Target {
				if _, ok := tmp[target.Type]; !ok {
					tmp[target.Type] = make(map[uint32]struct{})
				}
				tmp[target.Type][id] = struct{}{}
			}
		}
	}
	SectQuestTargetMap = tmp
}

const (
	sectTaskOp_GiveUp = 1
	sectTaskOp_Accept = 2
	sectTaskOp_Commit = 3
)

func (sys *SectTaskSys) giveUpTask(id uint32) error {
	data := sys.GetData()
	for _, v := range data.TaskList {
		if v.QuestId == id && v.Status == sectTaskRunning {
			v.Status = sectTaskGiveUp
			v.Quest = nil
			event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepBySectTask, sys.GetOwner().GetId())
			return nil
		}
	}
	return neterror.ParamsInvalidError("no running task(%d)", id)
}

func (sys *SectTaskSys) acceptTask(id uint32) error {
	data := sys.GetData()
	conf := jsondata.GetSectTaskConf()
	if nil == conf {
		return neterror.ConfNotFoundError("no sect task conf")
	}

	taskConf := jsondata.GetSectQuestTaskConf(id)
	if nil == taskConf {
		return neterror.ConfNotFoundError("no sect task(%d)", id)
	}

	var runningNum uint32
	for _, v := range data.TaskList {
		if v.Status == sectTaskRunning {
			runningNum++
		}
	}
	if conf.CanTakeNum <= (data.CompleteTimes + runningNum) {
		return neterror.ParamsInvalidError("sect task take limit")
	}
	for _, v := range data.TaskList {
		if v.QuestId == id && v.Status == sectTaskNoAccept {
			v.Status = sectTaskRunning
			quest := &pb3.QuestData{
				Id: id,
			}
			v.Quest = quest
			sys.QuestTargetBase.OnAcceptQuest(quest)
			event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepBySectTask, sys.GetOwner().GetId())
			return nil
		}
	}
	return neterror.ParamsInvalidError("no can accept task(%d)", id)
}

func (sys *SectTaskSys) commitTask(id uint32) error {
	data := sys.GetData()
	conf := jsondata.GetSectTaskConf()
	taskConf := jsondata.GetSectQuestTaskConf(id)
	if nil == conf || nil == taskConf {
		return neterror.ConfNotFoundError("no sect task conf(%d)", id)
	}
	if conf.CanTakeNum <= data.CompleteTimes {
		return neterror.ParamsInvalidError("today sect commit limit")
	}
	for _, v := range data.TaskList {
		if v.QuestId == id && v.Status == sectTaskRunning {
			if sys.CheckFinishQuest(v.Quest) {
				v.Status = sectTaskCommit
				data.CompleteTimes++
				engine.GiveRewards(sys.owner, taskConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectTask})
				logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogSectTask, &pb3.LogPlayerCounter{
					NumArgs: uint64(data.CompleteTimes),
				})
				sys.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiSectTask, 1)
				sys.GetOwner().TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionGuildQuestSect)
				sys.GetOwner().TriggerQuestEvent(custom_id.QttSectTaskNum, 0, 1)
				sys.GetOwner().TriggerQuestEvent(custom_id.QttSectTask, 0, 1)
				event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepBySectTask, sys.GetOwner().GetId())
				return nil
			}
			break
		}
	}
	sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
	return neterror.ParamsInvalidError("no can commit task(%d)", id)
}

func c2sTask(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		var req pb3.C2S_46_1
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		s := sys.(*SectTaskSys)
		if req.Op < sectTaskOp_GiveUp || req.Op > sectTaskOp_Commit {
			return neterror.ParamsInvalidError("no option for task")
		}
		switch req.Op {
		case sectTaskOp_GiveUp:
			err = s.giveUpTask(req.QuestId)
		case sectTaskOp_Accept:
			err = s.acceptTask(req.QuestId)
		case sectTaskOp_Commit:
			err = s.commitTask(req.QuestId)
		}
		if nil != err {
			return err
		}
		s.SendProto3(46, 1, &pb3.S2C_46_1{
			Op:      req.Op,
			QuestId: req.QuestId,
		})
		return nil
	}
}

const (
	SectTaskPerfectQuality = 5
)

func (sys *SectTaskSys) refresh(isClear bool) error {
	conf := jsondata.GetSectTaskConf()
	if nil == conf {
		return neterror.ConfNotFoundError("no sect task conf")
	}
	var tlv, num uint32
	if s, ok := sys.owner.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys); ok {
		txt := s.getData()
		if nil != txt {
			tlv = txt.Level
		}
	}
	data := sys.GetData()
	exclude := make(map[uint32]struct{})
	var newTaskList []*pb3.SectTaskStatus
	for i := 0; i < int(conf.CanShowNum); i++ {
		newTaskList = append(newTaskList, &pb3.SectTaskStatus{})
	}
	if !isClear {
		for i, v := range data.TaskList {
			if i >= int(conf.CanShowNum) {
				break
			}
			if v.Status == sectTaskRunning {
				exclude[v.QuestId] = struct{}{}
				newTaskList[i] = v
				num++
			}
		}
	}
	data.TaskList = newTaskList

	privilegeOnce, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumSectTaskPerfectTask)

	pool := new(random.Pool)
	var taskList []uint32
	var perfectTask []uint32
	for questId, quest := range conf.TaskPool {
		if _, ok := exclude[questId]; ok {
			continue
		}
		questConf := jsondata.GetQuestConf(questId)
		if nil == questConf {
			continue
		}
		if questConf.Type != custom_id.QtSect {
			sys.LogError("quest(%d) not a sect task!!!", questId)
			continue
		}
		if quest.TaXianTu != tlv {
			continue
		}

		if quest.OpenDay != 0 && quest.OpenDay > gshare.GetOpenServerDay() {
			continue
		}

		if quest.OpenLv != 0 && quest.OpenDay > sys.GetOwner().GetLevel() {
			continue
		}

		if quest.OpenCircle != 0 && quest.OpenCircle > sys.GetOwner().GetCircle() {
			continue
		}

		if quest.Quality == SectTaskPerfectQuality && privilegeOnce > 0 {
			perfectTask = append(perfectTask, questId)
		}
		taskList = append(taskList, questId)
	}

	var mustTask uint32
	if len(perfectTask) > 0 {
		mustTask = perfectTask[random.Interval(0, len(perfectTask)-1)]
	}
	for _, questId := range taskList {
		if questId == mustTask {
			continue
		}
		pool.AddItem(questId, conf.TaskPool[questId].Weight)
	}

	if pool.Size() == 0 {
		return neterror.ParamsInvalidError("no task can refresh")
	}

	if conf.CanShowNum > num {
		num = conf.CanShowNum - num
	} else {
		return nil
	}

	ret := pool.RandomMany(num)
	if mustTask > 0 {
		ret[0] = mustTask
	}
	idx := 0
	for _, v := range data.TaskList {
		if idx >= len(ret) {
			break
		}
		if v.QuestId == 0 {
			v.QuestId = ret[idx].(uint32)
			idx++
		}
	}
	sys.s2cInfo()
	return nil
}

func (sys *SectTaskSys) refreshTask() error {
	data := sys.GetData()
	conf := jsondata.GetSectTaskConf()
	if nil == conf {
		return neterror.ConfNotFoundError("no sect task conf")
	}
	if data.CompleteTimes >= conf.CanTakeNum {
		return neterror.ParamsInvalidError("today sect task complete limit")
	}
	var consume jsondata.ConsumeVec
	for _, v := range conf.Refresh {
		if data.RefreshTimes >= v.RefreshTimes-1 {
			consume = v.Consume
		}
	}
	num := 0
	for _, v := range data.TaskList {
		if v.Status == sectTaskRunning {
			num++
		}
	}
	if num == len(data.TaskList) {
		return neterror.ParamsInvalidError("all sect task in running")
	}
	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogRefreshSectTask}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	data.RefreshTimes++
	return sys.refresh(false)
}

func c2sRefresh(sys iface.ISystem) func(*base.Message) error {
	return func(_ *base.Message) error {
		return sys.(*SectTaskSys).refreshTask()
	}
}

func onSectTaskNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSectTask).(*SectTaskSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.GetData().RefreshTimes = 0
	sys.GetData().CompleteTimes = 0
	sys.s2cInfo()
	err := sys.refresh(false)
	if err != nil {
		sys.LogError("sect refresh err:%v", err)
		return
	}
}

func onSectTaskCondChange(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSectTask).(*SectTaskSys)
	if !ok || !sys.IsOpen() {
		return
	}
	err := sys.refresh(false)
	if err != nil {
		sys.LogError("sect refresh err:%v", err)
		return
	}
}

var singleSectTaskController = &SectTaskController{}

type SectTaskController struct {
	actsweepmgr.Base
}

func (receiver *SectTaskController) AddUseTimes(_ uint32, times uint32, playerId uint64) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiSectTask)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys, ok := obj.(*SectTaskSys)
	if !ok {
		return
	}

	conf := jsondata.GetSectTaskConf()
	if nil == conf {
		return
	}
	data := sys.GetData()
	data.CompleteTimes += times
	sys.GetOwner().TriggerQuestEvent(custom_id.QttSectTaskNum, 0, int64(times))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttSectTask, 0, int64(times))
	sys.s2cInfo()
}

func (receiver *SectTaskController) GetCanUseTimes(id uint32, playerId uint64) (canUseTimes uint32) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiSectTask)
	if obj == nil || !obj.IsOpen() {
		return
	}

	_, ok := obj.(*SectTaskSys)
	if !ok {
		return
	}

	conf := jsondata.GetSectTaskConf()
	if nil == conf {
		return
	}

	return conf.CanTakeNum
}

func (receiver *SectTaskController) GetUseTimes(id uint32, playerId uint64) (useTimes uint32, ret bool) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiSectTask)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys, ok := obj.(*SectTaskSys)
	if !ok {
		return
	}

	data := sys.GetData()

	var runningTimes uint32
	for _, v := range data.TaskList {
		if v.Status == sectTaskRunning {
			runningTimes += 1
		}
	}

	return sys.GetData().CompleteTimes + runningTimes, true
}

func init() {
	RegisterSysClass(sysdef.SiSectTask, newSectTaskSys)

	net.RegisterSysProtoV2(46, 1, sysdef.SiSectTask, c2sTask)
	net.RegisterSysProtoV2(46, 2, sysdef.SiSectTask, c2sRefresh)

	event.RegActorEvent(custom_id.AeNewDay, onSectTaskNewDay)
	event.RegActorEvent(custom_id.AeFreeUpLv, onSectTaskCondChange)

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadSectTaskConf)
	actsweepmgr.Reg(actsweepmgr.ActSweepBySectTask, singleSectTaskController)
	gmevent.Register("sectTask", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiSectTask).(*SectTaskSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		taskId := utils.AtoUint32(args[0])
		data := sys.GetData()
		data.TaskList[0] = &pb3.SectTaskStatus{
			QuestId: taskId,
			Status:  0,
			Quest:   nil,
		}
		sys.s2cInfo()
		return true
	}, 1)

	engine.RegisterMessage(gshare.OfflineGMSectTaskRefresh, func() pb3.Message {
		return &pb3.CommonSt{}
	}, func(player iface.IPlayer, msg pb3.Message) {
		sys, ok := player.GetSysObj(sysdef.SiSectTask).(*SectTaskSys)
		if !ok || !sys.IsOpen() {
			return
		}
		err := sys.refresh(true)
		if err != nil {
			player.LogError("err:%v", err)
			return
		}
	})
}
