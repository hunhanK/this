package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

func newFairyDynastyQuestSys() iface.ISystem {
	sys := &FairyDynastyQuestSys{}
	sys.QuestTargetBase = &QuestTargetBase{}
	sys.GetTargetConfFunc = sys.getTargetConfFunc
	sys.GetQuestIdSetFunc = sys.getQuestIdSet
	sys.GetUnFinishQuestDataFunc = sys.getUnFinishQuestData
	sys.OnUpdateTargetDataFunc = sys.onUpdateTargetData
	sys.IsSysOpenFunc = sys.IsOpen

	return sys
}

type FairyDynastyQuestSys struct {
	*QuestTargetBase
	data *pb3.FairyDynastyQuestData
}

func (sys *FairyDynastyQuestSys) init() {
	if nil == sys.GetBinaryData().FairyDynastyQuest {
		sys.refresh()

	}
	sys.data = sys.GetBinaryData().FairyDynastyQuest

	if sys.data.Quests == nil || sys.data.Progresses == nil {
		sys.refresh()
	}

	sys.sendInfo()
}

func (sys *FairyDynastyQuestSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *FairyDynastyQuestSys) OnOpen() {
	sys.init()
	sys.data.SysOpenAt = time_util.NowSec()
}

func (sys *FairyDynastyQuestSys) sendInfo() {
	sys.SendProto3(43, 0, &pb3.S2C_43_0{Data: sys.data})
}

func (sys *FairyDynastyQuestSys) OnReconnect() {
	sys.sendInfo()
}

func (sys *FairyDynastyQuestSys) OnAfterLogin() {
	sys.sendInfo()
}

// 补发昨天的奖励
func (sys *FairyDynastyQuestSys) supplyLastDayReward() {
	var rewardVecs []jsondata.StdRewardVec

	// 未领取的任务奖励
	for id, quest := range sys.data.Quests {
		if quest.Rewarded {
			continue
		}

		if !sys.CheckFinishQuest(quest.QuestInfo) {
			continue
		}

		questConf := jsondata.GetFairyDynastyQuestById(id)
		if questConf == nil {
			continue
		}

		rewardVecs = append(rewardVecs, questConf.Award)
	}

	// 未领取的进度奖励
	for idx, rewarded := range sys.data.Progresses {
		if rewarded {
			continue
		}

		progressConf := jsondata.GetFairyDynastyProgress(idx)
		if progressConf == nil {
			continue
		}

		if sys.data.QuestPoint < progressConf.ReqPoint {
			break
		}

		rewardVecs = append(rewardVecs, progressConf.Award)
	}

	// 合并奖励
	rewards := jsondata.MergeStdReward(rewardVecs...)
	if len(rewards) > 0 {
		sys.GetOwner().SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_FairyDynastySupplyLastDayReward,
			Rewards: rewards,
		})
	}
}

func (sys *FairyDynastyQuestSys) refresh() {
	// 清理数据
	sys.GetBinaryData().FairyDynastyQuest = &pb3.FairyDynastyQuestData{
		Quests: make(map[uint32]*pb3.FairyDynastyQuest),
	}
	sys.data = sys.GetBinaryData().FairyDynastyQuest

	// 初始化新的任务
	for id, questConf := range jsondata.GetFairyDynastyQuests() {
		if questConf.MinOpenDayCanTrigger > gshare.GetOpenServerDay() {
			continue
		}

		sys.data.Quests[id] = &pb3.FairyDynastyQuest{
			QuestInfo: &pb3.QuestData{
				Id: id,
			},
			Rewarded: false,
		}
	}

	// 刷新的时候初始化一下任务的进度
	for _, quest := range sys.data.Quests {
		sys.QuestTargetBase.OnAcceptQuest(quest.QuestInfo)
	}

	// 初始化新的积分进度
	sys.data.Progresses = make([]bool, len(jsondata.GetFairyDynastyProgresses()))
	sys.data.QuestPoint = 0
	sys.data.DailyRenown = 0
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttFairyDynastyQuestAddRenown)
}

func (sys *FairyDynastyQuestSys) onNewDay() {
	sys.supplyLastDayReward()
	sys.refresh()

	sys.sendInfo()
}

func (sys *FairyDynastyQuestSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	rawQuests := jsondata.GetFairyDynastyQuests()
	if nil == rawQuests {
		return nil
	}

	ret := make(map[uint32]struct{})
	for questId, questConf := range rawQuests {
		if questConf.MinOpenDayCanTrigger > gshare.GetOpenServerDay() {
			continue
		}

		for _, questTargetConf := range questConf.Targets {
			if questTargetConf.Type != qt {
				continue
			}
			ret[questId] = struct{}{}
		}
	}

	return ret
}

func (sys *FairyDynastyQuestSys) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	rawQuests := jsondata.GetFairyDynastyQuests()
	if rawQuests == nil {
		return nil
	}

	questConf, ok := rawQuests[questId]
	if !ok {
		return nil
	}

	if questConf.MinOpenDayCanTrigger > gshare.GetOpenServerDay() {
		return nil
	}

	return questConf.Targets
}

func (sys *FairyDynastyQuestSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	for qid, quest := range sys.data.Quests {
		if qid == id {
			return quest.QuestInfo
		}
	}

	return nil
}

func (sys *FairyDynastyQuestSys) onUpdateTargetData(id uint32) {
	quest := sys.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	sys.SendProto3(43, 1, &pb3.S2C_43_1{Quest: &pb3.FairyDynastyQuest{
		QuestInfo: quest,
		Rewarded:  false,
	}})
}

func (sys *FairyDynastyQuestSys) c2sQuestReward(msg *base.Message) error {
	var req pb3.C2S_43_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	questData, ok := sys.data.Quests[req.QuestId]
	if !ok {
		return neterror.ParamsInvalidError("questData for id %d not found", req.QuestId)
	}
	if questData.Rewarded {
		return neterror.ParamsInvalidError("quest %d already rewarded", req.QuestId)
	}

	if !sys.CheckFinishQuest(questData.QuestInfo) {
		return neterror.ParamsInvalidError("quest %d not finished", req.QuestId)
	}

	questConf := jsondata.GetFairyDynastyQuestById(req.QuestId)
	if questConf == nil {
		return neterror.InternalError("quest conf for id %d not found", req.QuestId)
	}

	questData.Rewarded = true
	sys.data.QuestPoint += questConf.Point

	// 发奖
	state := engine.GiveRewards(sys.GetOwner(), questConf.Award, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyDynastyReward,
	})

	if !state {
		return neterror.InternalError("give reward failed")
	}
	sys.SendProto3(43, 2, &pb3.S2C_43_2{
		Quest: questData,
	})
	sys.SendProto3(43, 4, &pb3.S2C_43_4{QuestPoint: sys.data.QuestPoint})

	// 添加名望
	fairyDynastyJobSys, ok := sys.GetOwner().GetSysObj(sysdef.SiFairyDynastyJobSys).(*FairyDynastyJobSys)
	if ok && fairyDynastyJobSys.IsOpen() {
		fairyDynastyJobSys.AddPoint(int64(questConf.Renown))
	}
	sys.data.DailyRenown += questConf.Renown
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttFairyDynastyQuestAddRenown)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsFairyDynastyQuestAddRenown, 0, int64(questConf.Renown))
	sys.GetOwner().TriggerEvent(custom_id.AeAddFlyingFairyOrderFuncExp, sysdef.SiFairyDynastyQuestSys, uint32(questConf.Renown))

	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsTypeFairyDynastyQuest, questConf.Type, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttTypeFairyDynastyQuest, questConf.Type, 1)
	return nil
}

func (sys *FairyDynastyQuestSys) c2sProgressReward(msg *base.Message) error {
	var req pb3.C2S_43_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if req.Idx >= uint32(len(sys.data.Progresses)) {
		return neterror.ParamsInvalidError("idx %d out of range", req.Idx)
	}

	if sys.data.Progresses[req.Idx] {
		return neterror.ParamsInvalidError("progress %d already rewarded", req.Idx)
	}

	progressConf := jsondata.GetFairyDynastyProgress(int(req.Idx))
	if progressConf == nil {
		return neterror.InternalError("conf for idx %d not found", req.Idx)
	}

	if sys.data.QuestPoint < progressConf.ReqPoint {
		return neterror.ParamsInvalidError("current point %d < reqPoint %d", sys.data.QuestPoint, progressConf.ReqPoint)
	}

	sys.data.Progresses[req.Idx] = true

	// 发奖
	state := engine.GiveRewards(sys.GetOwner(), progressConf.Award, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyDynastyProgressReward,
	})

	if !state {
		return neterror.InternalError("give reward failed")
	}
	sys.SendProto3(43, 3, &pb3.S2C_43_3{
		Progresses: sys.data.Progresses,
		Idx:        req.Idx,
	})
	return nil
}

func (sys *FairyDynastyQuestSys) GMReAcceptQuest(questId uint32) {
	for qid, quest := range sys.data.Quests {
		if qid != questId {
			continue
		}
		sys.OnAcceptQuestAndCheckUpdateTarget(quest.QuestInfo)
		break
	}

}

func (sys *FairyDynastyQuestSys) GMDelQuest(questId uint32) {
	delete(sys.data.Quests, questId)
}

func init() {
	RegisterSysClass(sysdef.SiFairyDynastyQuestSys, newFairyDynastyQuestSys)
	net.RegisterSysProtoV2(43, 2, sysdef.SiFairyDynastyQuestSys,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(msg *base.Message) error {
				return sys.(*FairyDynastyQuestSys).c2sQuestReward(msg)
			}
		})

	net.RegisterSysProtoV2(43, 3, sysdef.SiFairyDynastyQuestSys,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(msg *base.Message) error {
				return sys.(*FairyDynastyQuestSys).c2sProgressReward(msg)
			}
		})

	event.RegActorEventL(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys := player.GetSysObj(sysdef.SiFairyDynastyQuestSys).(*FairyDynastyQuestSys)
		if sys == nil || !sys.IsOpen() {
			return
		}
		if time_util.IsSameDay(sys.data.SysOpenAt, time_util.NowSec()) {
			return
		}
		sys.onNewDay()
	})

	engine.RegQuestTargetProgress(custom_id.QttFairyDynastyQuestAddRenown, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		sys := player.GetSysObj(sysdef.SiFairyDynastyQuestSys).(*FairyDynastyQuestSys)
		if sys == nil || !sys.IsOpen() {
			return 0
		}
		return sys.data.DailyRenown
	})
}
