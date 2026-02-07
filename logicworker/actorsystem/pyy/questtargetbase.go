/**
 * @Author: PengZiMing
 * @Desc: 活动任务基类
 * @Date: 2022/6/6 19:43
 */

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/quest"
	"jjyz/gameserver/logworker"
	"strings"
)

// QuestTargetBase 运营任务数据基类
type YYQuestTargetBase struct {
	PlayerYYBase
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
		sys.LogDebug("sys quest fun is nil yyid = %d", sys.Id)
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
	data.Progress = sys.AcceptQuestInitProgress(sys.GetPlayer(), data.Progress, targets)
}

func (sys *YYQuestTargetBase) OnAcceptQuestAndCheckUpdateTarget(data *pb3.QuestData) {
	sys.OnAcceptQuest(data)
	if sys.OnUpdateTargetDataFunc != nil {
		sys.OnUpdateTargetDataFunc(data.GetId())
	}
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

// 直接去拿注册函数再统计一遍
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

func (sys *YYQuestTargetBase) GMReAcceptQuest(questId uint32) {}

func (sys *YYQuestTargetBase) GMDelQuest(questId uint32) {}

func offlineGMReAcceptPYYQuest(player iface.IPlayer, msg pb3.Message) {
	var doLogic = func(gmQuest iface.IQuestGM, splitStr []string, opt uint32) {
		for _, numStr := range splitStr {
			questId := utils.AtoUint32(numStr)
			if questId == 0 {
				player.LogError("offlineGMReAcceptPYYQuest convert %s to questId failed", numStr)
				continue
			}
			switch opt {
			case 1: // 重新接任务
				utils.ProtectRun(func() {
					gmQuest.GMReAcceptQuest(questId)
				})
			case 2: // 删除任务
				utils.ProtectRun(func() {
					gmQuest.GMDelQuest(questId)
				})
			}
		}
	}

	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		player.LogError("offlineGMReAcceptPYYQuest convert CommonSt failed")
		return
	}

	player.LogInfo("handle offlineGMReAcceptPYYQuest:%s", st.String())

	logworker.LogPlayerBehavior(player, pb3.LogId_LogGm, &pb3.LogPlayerCounter{
		StrArgs: "offlineGMReAcceptPYYQuest:" + st.String(),
	})

	pyyClassId := st.U32Param
	opt := st.U32Param2
	idsString := st.StrParam
	splitStr := strings.Split(idsString, ",")
	if len(splitStr) == 0 {
		player.LogError("offlineGMReAcceptPYYQuest ids id zero")
		return
	}

	yyList := pyymgr.GetPlayerAllYYObj(player, pyyClassId)
	if len(yyList) == 0 {
		return
	}

	for i := range yyList {
		v := yyList[i]
		if v == nil || !v.IsOpen() {
			continue
		}
		gmQuest, ok := v.(iface.IQuestGM)
		if !ok {
			player.LogError("offlineGMReAcceptPYYQuest convert %d IQuestGM failed", pyyClassId)
			return
		}
		doLogic(gmQuest, splitStr, opt)
	}

}

func init() {
	engine.RegisterMessage(gshare.OfflineGMReAcceptPYYQuest, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineGMReAcceptPYYQuest)
}
