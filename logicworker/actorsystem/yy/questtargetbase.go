/**
 * @Author: PengZiMing
 * @Desc: 活动任务基类
 * @Date: 2022/6/6 19:43
 */

package yy

import (
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/quest"

	"github.com/gzjjyz/logger"
)

// YYQuestTargetBase 运营任务数据基类
type YYQuestTargetBase struct {
	YYBase
	quest.ProgressBase
	GetQuestIdSetFunc        func(qtt uint32) map[uint32]struct{}
	GetUnFinishQuestDataFunc func(id uint32) *pb3.QuestData
	GetTargetConfFunc        func(id uint32) []*jsondata.QuestTargetConf
	OnUpdateTargetDataFunc   func(id uint32)
	CheckQuestFinishFunc     func(id uint32) bool
}

func (sys *YYQuestTargetBase) isInitFinish() bool {
	if !sys.IsOpen() {
		return false
	}

	if nil == sys.GetQuestIdSetFunc || nil == sys.GetTargetConfFunc || nil == sys.GetUnFinishQuestDataFunc {
		logger.LogDebug("sys quest fun is nil yyid = %d", sys.Id)
		return false
	}
	return true
}

func (sys *YYQuestTargetBase) GmFinishQuest(data *pb3.QuestData) {
	if nil == data {
		return
	}

	id := data.GetId()
	if nil == sys.GetTargetConfFunc {
		return
	}

	targets := sys.GetTargetConfFunc(id)
	if len(targets) <= 0 {
		return
	}

	data.Progress = make([]uint32, len(targets))
	for idx, target := range targets {
		data.Progress[idx] = target.Count
	}

	if nil != sys.CheckQuestFinishFunc {
		sys.CheckQuestFinishFunc(data.GetId())
	}

	if nil != sys.OnUpdateTargetDataFunc {
		sys.OnUpdateTargetDataFunc(data.GetId())
	}
}

func (sys *YYQuestTargetBase) OnAcceptQuest(data *pb3.QuestData) {
	if nil == data {
		return
	}

	id := data.GetId()
	if nil == sys.GetTargetConfFunc {
		return
	}

	targets := sys.GetTargetConfFunc(id)
	if len(targets) <= 0 {
		return
	}
	// todo 全服任务不存在累积进度
	// data.Progress = sys.AcceptQuestInitProgress(sys.GetPlayer(), data.Progress, targets)
}

// CheckFinishQuest 判断任务能否完成
func (sys *YYQuestTargetBase) CheckFinishQuest(data *pb3.QuestData) bool {
	if nil == data {
		return false
	}

	id := data.GetId()
	if nil == sys.GetTargetConfFunc {
		return false
	}

	targets := sys.GetTargetConfFunc(id)
	return sys.CheckProgress(data.Progress, targets)
}

// OnQuestEvent 任务事件
func (sys *YYQuestTargetBase) OnQuestEvent(actor iface.IPlayer, qt uint32, id, count uint32, add bool) {
	if !sys.isInitFinish() {
		return
	}

	ids := sys.GetQuestIdSetFunc(qt)
	if len(ids) <= 0 {
		return
	}

	for questId := range ids {
		targets := sys.GetTargetConfFunc(questId)
		if nil == targets {
			continue
		}
		data := sys.GetUnFinishQuestDataFunc(questId)
		if nil == data {
			continue
		}

		change, checkFinish := false, false
		data.Progress, change, checkFinish = sys.QuestEventProgress(data.Progress, targets, qt, id, count, add)

		if checkFinish && nil != sys.CheckQuestFinishFunc {
			if sys.CheckQuestFinishFunc(questId) {
				change = false
			}
		}
		if change && nil != sys.OnUpdateTargetDataFunc {
			sys.OnUpdateTargetDataFunc(questId)
		}
	}
}

// CalcQuestTargetByRange2 直接去拿注册函数再统计一遍
func (sys *YYQuestTargetBase) CalcQuestTargetByRange2(actor iface.IPlayer, qtt uint32, args ...interface{}) {
	if !sys.isInitFinish() {
		return
	}

	idSet := sys.GetQuestIdSetFunc(qtt)
	for id := range idSet {
		targets := sys.GetTargetConfFunc(id)
		if nil == targets {
			continue
		}

		data := sys.GetUnFinishQuestDataFunc(id)
		if nil == data {
			continue
		}

		change, checkFinish := false, false
		data.Progress, change, checkFinish = sys.QuestEventRangeProgress2(data.Progress, targets, actor, qtt, args...)

		if checkFinish && nil != sys.CheckQuestFinishFunc {
			if sys.CheckQuestFinishFunc(id) {
				change = false
			}
		}

		if change && nil != sys.OnUpdateTargetDataFunc {
			sys.OnUpdateTargetDataFunc(id)
		}
	}
}

func (sys *YYQuestTargetBase) CalcQuestTargetByRange(actor iface.IPlayer, qtt, tVal, preVal, qtype uint32) {
	if !sys.isInitFinish() {
		return
	}

	idSet := sys.GetQuestIdSetFunc(qtt)
	for id := range idSet {
		targets := sys.GetTargetConfFunc(id)
		if nil == targets {
			continue
		}
		data := sys.GetUnFinishQuestDataFunc(id)
		if nil == data {
			continue
		}

		change, checkFinish := false, false
		data.Progress, change, checkFinish = sys.QuestEventRangeProgress(data.Progress, targets, qtt, tVal, preVal, qtype)
		if checkFinish && nil != sys.CheckQuestFinishFunc {
			if sys.CheckQuestFinishFunc(id) {
				change = false
			}
		}
		if change && nil != sys.OnUpdateTargetDataFunc {
			sys.OnUpdateTargetDataFunc(id)
		}
	}
}
