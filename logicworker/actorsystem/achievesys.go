package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

var (
	AchieveQuestTargetMap map[uint32]map[uint32]struct{} // 境界任务事件对应的id
)

type AchieveSys struct {
	*QuestTargetBase
}

func newAchieveSys() iface.ISystem {
	sys := &AchieveSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (sys *AchieveSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetAchieve() {
		binary.Achieve = &pb3.AchieveData{}
	}
	achieve := binary.GetAchieve()
	if nil == achieve.AchieveRecord {
		achieve.AchieveRecord = make(map[uint32]uint32)
	}
	if nil == achieve.Quests {
		achieve.Quests = make(map[uint32]*pb3.QuestData)
	}
}

func (sys *AchieveSys) GetData() *pb3.AchieveData {
	binary := sys.GetBinaryData()
	return binary.GetAchieve()
}

func (sys *AchieveSys) OnOpen() {
	achieve := sys.GetData()
	achieve.Level = 1
	sys.CheckResetQuest()
	sys.SendProto3(39, 0, &pb3.S2C_39_0{Achieve: achieve})
}

func (sys *AchieveSys) OnLogin() {
	sys.CheckResetQuest()
}

func (sys *AchieveSys) OnAfterLogin() {
	sys.SendProto3(39, 0, &pb3.S2C_39_0{Achieve: sys.GetData()})
}

func (sys *AchieveSys) OnReconnect() {
	sys.SendProto3(39, 0, &pb3.S2C_39_0{Achieve: sys.GetData()})
}

func (sys *AchieveSys) CheckResetQuest() {
	achieve := sys.GetData()
	taskList := jsondata.AchieveTaskConfMgr
	for id := range taskList {
		if _, ok := achieve.Quests[id]; !ok && achieve.AchieveRecord[id] == 0 {
			quest := &pb3.QuestData{
				Id: id,
			}
			achieve.Quests[id] = quest
			sys.QuestTargetBase.OnAcceptQuest(quest)
		}
	}
}

func (sys *AchieveSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := AchieveQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

func (sys *AchieveSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	achieve := sys.GetData()
	if achieve.AchieveRecord[id] > 0 { //已经完成的直接返回
		return nil
	}
	if task, ok := achieve.Quests[id]; ok { //进行中的
		return task
	}
	return nil
}

func (sys *AchieveSys) onUpdateTargetData(questId uint32) {
	quest := sys.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	//更新任务
	sys.SendProto3(39, 1, &pb3.S2C_39_1{Quest: quest})

}

func (sys *AchieveSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	if conf := jsondata.GetAchieveTaskConf(id); nil != conf {
		return conf.Targets
	}
	return nil
}

func (sys *AchieveSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_39_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	achieve := sys.GetData()
	ids := req.Ids
	if len(ids) == 0 {
		return neterror.ParamsInvalidError("ids is nil")
	}
	ts := time_util.NowSec()
	var awards jsondata.StdRewardVec
	var addExpSum int64
	var logId pb3.LogId
	var okIds []uint32
	owner := sys.GetOwner()
	for _, id := range ids {
		if req.GetType() == custom_id.AchieveAward {
			conf := jsondata.GetAchieveTaskConf(id)
			if nil == conf {
				owner.LogError("achieve task conf(%d) is nil", id)
				continue
			}
			quest := achieve.Quests[id]
			if nil == quest {
				owner.LogError("no accept achieve task(%d)", id)
				continue
			}
			if !sys.CheckFinishQuest(quest) {
				owner.LogError("%d AwardCondNotEnough", quest.Id)
				continue
			}
			if achieve.AchieveRecord[id] > 0 {
				owner.LogError("%d TpAwardIsReceive", id)
				continue

			}
			achieve.AchieveRecord[id] = ts
			if conf.AddExp > 0 {
				addExpSum += int64(conf.AddExp)
			}
			if len(conf.Award) > 0 {
				awards = append(awards, conf.Award...)
				logId = pb3.LogId_LogAchieveTaskFinish
			}
			okIds = append(okIds, id)
		} else if req.GetType() == custom_id.AchieveLevelAward {
			if id <= 0 {
				owner.LogError("achieve param 0 err")
				continue
			}
			conf := jsondata.GetAchieveConf(id)
			if nil == conf {
				owner.LogError("achieve conf(%d) is nil", id)
				continue
			}
			if achieve.GetLevel() < conf.Level {
				owner.LogError("%d %d AwardCondNotEnough", achieve.GetLevel(), conf.Level)
				continue

			}
			if utils.SliceContainsUint32(achieve.LevelAwardRecord, id) {
				owner.LogError("%d TpAwardIsReceive", id)
				continue
			}
			achieve.LevelAwardRecord = append(achieve.LevelAwardRecord, id)
			if len(conf.Award) > 0 {
				awards = append(awards, conf.Award...)
				logId = pb3.LogId_LogAchieveLevel
			}
			okIds = append(okIds, id)
		}
	}
	if addExpSum > 0 {
		sys.owner.AddMoney(moneydef.AchievePoint, addExpSum, false, pb3.LogId_LogAchieveTaskFinish)
	}
	if len(awards) > 0 {
		engine.GiveRewards(sys.owner, awards, common.EngineGiveRewardParam{LogId: logId})
	}
	sys.SendProto3(39, 2, &pb3.S2C_39_2{
		Type:      req.GetType(),
		Timestamp: ts,
		Ids:       okIds,
	})
	return nil
}

func (sys *AchieveSys) AddAchievePoint(addPoint int64, logId pb3.LogId) {
	if addPoint <= 0 {
		return
	}
	achieve := sys.GetData()
	lv := achieve.GetLevel()
	point := achieve.GetPoint()
	point += addPoint
	for {
		ntConf := jsondata.GetAchieveConf(lv + 1)
		if nil == ntConf {
			break
		}
		if point >= int64(ntConf.Exp) {
			lv = lv + 1
			point = point - int64(ntConf.Exp)
		} else {
			break
		}
	}
	achieve.Level = lv
	achieve.Point = point
	sys.SendProto3(39, 3, &pb3.S2C_39_3{
		Level: achieve.Level,
		Point: achieve.Point,
	})
}

func onAfterReloadAchieveConf(args ...interface{}) {
	tmp := make(map[uint32]map[uint32]struct{})
	if nil == jsondata.AchieveTaskConfMgr {
		return
	}
	for id, quest := range jsondata.AchieveTaskConfMgr {
		for _, target := range quest.Targets {
			if _, ok := tmp[target.Type]; !ok {
				tmp[target.Type] = make(map[uint32]struct{})
			}
			tmp[target.Type][id] = struct{}{}
		}
	}
	AchieveQuestTargetMap = tmp
}

func init() {
	RegisterSysClass(sysdef.SiAchieve, newAchieveSys)

	net.RegisterSysProto(39, 2, sysdef.SiAchieve, (*AchieveSys).c2sAward)

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadAchieveConf)
}
