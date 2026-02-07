/**
 * @Author: PengZiMing
 * @Desc: 处理任务事件的逻辑
 * @Date: 2022/6/6 13:32
 */

package quest

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

// 任务类的系统
var AllQuestTargetSysIds = []uint32{
	sysdef.SiQuest,
	sysdef.SiFreeVip,
	sysdef.SiBoundary,
	sysdef.SiAchieve,
	sysdef.SiFairyDynastyQuestSys,
	sysdef.SiNewJingJie,
	sysdef.SiSectTask,
	sysdef.SiTimeLimitChargePack,
	sysdef.SiTimeLimitMoneyPack,
	sysdef.SiFuncTrial,
	sysdef.SiFlyUpRoad,
	sysdef.SiWeaponSoul,
	sysdef.SiAskHelp,
	sysdef.SiNirvana,
	sysdef.SiGuidelineGoal,
	sysdef.SiPotential,
	sysdef.SiMagicWeapon,
	sysdef.SiCollectCard,
	sysdef.SiGMBenefit,
	sysdef.SiSmith,
	sysdef.SiDemonKingSecKill,
	sysdef.SiSponsorshipQuestLayer,
}

// 任务类的运营活动
var AllQuestTargetYYType = []uint32{
	yydefine.YYSiGodWeaponCome,
	yydefine.YYSiFlyingFairyOrderFunc,
	yydefine.YYFreeFashion,
	yydefine.YYTurntableDraw,
	yydefine.YYGodWeaponCeremony,
	yydefine.PYYGlobalCollectCards,
	yydefine.PYYPuzzleFun,
	yydefine.PYYFairySwordGoal,
	yydefine.PYYWeekQuestWanted,
	yydefine.PYYCelebratory,
	yydefine.PYYQiXiPuzzle,
	yydefine.PYYThreeRealmTrial,
}

// 任务类的全服运营活动
var AllQuestTargeGlobalYYType = []uint32{}

// todo 或许有更好的方法 后面再优化下
func allQuestSysDo(actor iface.IPlayer, fn func(sys iface.IQuestTargetSys)) {
	// 遍历任务类系统
	for _, sysId := range AllQuestTargetSysIds {
		obj := actor.GetSysObj(sysId)
		if obj == nil {
			continue
		}
		sys, ok := obj.(iface.IQuestTargetSys)
		if !ok {
			logger.LogWarn("系统 %d 未实现任务目标基类", sysId)
			continue
		}
		fn(sys)
	}
	// 遍历任务类的运营活动
	for _, yyType := range AllQuestTargetYYType {
		actor.TriggerEvent(custom_id.AeYYQuest, yyType, fn)
	}
	// 遍历任务类的运营活动
	for _, yyType := range AllQuestTargeGlobalYYType {
		actor.TriggerEvent(custom_id.AeGlobalYYQuest, yyType, fn)
	}
}

// 普通任务事件
func onQuestTargetEvent(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}

	qt, ok := args[0].(uint32)
	if !ok {
		return
	}

	id, ok := args[1].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(int64)
	if !ok {
		return
	}

	if qt <= 0 || qt > custom_id.QttMax {
		logger.LogError("%s %d 任务类型 %d 不在 QttMap 中", actor.GetName(), actor.GetId(), qt)
		logger.LogError("========================*********************========================")
		return
	}

	def := custom_id.QttVec[qt]
	if def.IsRecord {
		actor.TaskRecord(qt, id, count, def.IsAdd)
	}

	allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
		sys.OnQuestEvent(actor, qt, id, uint32(count), def.IsAdd)
	})
}

// 任务事件范围
func onQuestTargetEventRange(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	qt, ok := args[0].(uint32)
	if !ok {
		return
	}

	if qt <= 0 || qt > custom_id.QttMax {
		logger.LogError("任务类型 %d 不在 QttMap 中", qt)
		logger.LogError("========================*********************========================")
		return
	}

	allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
		sys.CalcQuestTargetByRange2(actor, qt, args[1:]...)
	})
}

// func onQuestTargetActorGemChange(qtype uint32) func(actor iface.IPlayer, args ...interface{}) {
// 	return func(actor iface.IPlayer, args ...interface{}) {
// 		if len(args) <= 0 {
// 			return
// 		}
// 		gemId, ok := args[0].(uint32)
// 		if !ok {
// 			return
// 		}

// 		conf := jsondata.GetItemConfig(gemId)
// 		if nil == conf {
// 			return
// 		}

// 		allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
// 			sys.CalcQuestTargetByRange(actor, custom_id.QttMountGemLevel, conf.GemLevel, 0, qtype)
// 		})
// 	}
// }

func onQuestTargetActorLearnVocationSkill(actor iface.IPlayer, args ...interface{}) {
	if len(args) <= 0 {
		return
	}

	level, ok := args[0].(uint32)
	if !ok {
		return
	}

	allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
		sys.CalcQuestTargetByRange(actor, custom_id.QttSkillCount, level, 0, custom_id.QTYPE_ADD)
	})
}

func onQuestKillMon(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}

	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	fbId, ok := args[3].(uint32)
	if !ok {
		return
	}

	if mConf := jsondata.GetMonsterConf(monsterId); mConf != nil {
		actor.TriggerQuestEvent(custom_id.QttMonsterId, monsterId, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterType, mConf.Type, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterTypeHistory, mConf.Type, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterSupType, mConf.SubType, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterSupTypeHistory, mConf.SubType, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterQuality, mConf.Quality, int64(count))
		actor.TriggerQuestEvent(custom_id.QttMonsterCount, uint32(0), int64(count))
		actor.TriggerQuestEvent(custom_id.QttSceneMonsterCount, sceneId, int64(count))
		actor.TriggerQuestEvent(custom_id.QttAchievementsSceneMonsterCount, sceneId, int64(count))
		actor.TriggerQuestEvent(custom_id.QttKillFbMonster, fbId, int64(count))
		actor.TriggerQuestEvent(custom_id.QttAchievementsMonsterId, monsterId, int64(count))

		allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
			sys.CalcQuestTargetByRange(actor, custom_id.QttMonsterLv, mConf.Level, 0, custom_id.QTYPE_ADD)

			// 击杀指定类型 X 级以上的怪物
			var specSubTypeLevelQtt uint32
			switch mConf.SubType {
			case custom_id.MstQiMenBoss:
				specSubTypeLevelQtt = custom_id.QttQiMenMonsterLv
			case custom_id.MstSuitBoss:
				specSubTypeLevelQtt = custom_id.QttSuitMonsterLv
			}
			if specSubTypeLevelQtt > 0 {
				sys.CalcQuestTargetByRange(actor, specSubTypeLevelQtt, mConf.Level, 0, custom_id.QTYPE_ADD)
			}

			// 击杀指定类型 X 阶以上的怪物
			specSubTypeLevelQtt = 0
			switch mConf.SubType {
			case custom_id.MstWorldBoss:
				specSubTypeLevelQtt = custom_id.QttWorldBossMonsterQuality
			case custom_id.MstSuitBoss:
				specSubTypeLevelQtt = custom_id.QttSuitMonsterQuality
			case custom_id.MstNoblemanBoss:
				specSubTypeLevelQtt = custom_id.QttNoblemanMonsterQuality
			}
			if specSubTypeLevelQtt > 0 {
				sys.CalcQuestTargetByRange(actor, specSubTypeLevelQtt, mConf.Quality, 0, custom_id.QTYPE_ADD)
			}

		})

	}
}

// 任务事件范围1
func onQuestTargetEventByRange(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}
	var params []uint32
	for i := 0; i < 4; i++ {
		if val, ok := args[i].(uint32); !ok {
			return
		} else {
			params = append(params, val)
		}
	}

	qt := params[0]
	if qt <= 0 || qt > custom_id.QttMax {
		logger.LogError("任务类型 %d 不在 QttMap 中", qt)
		logger.LogError("========================*********************========================")
		return
	}

	allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
		sys.CalcQuestTargetByRange(actor, params[0], params[1], params[2], params[3])
	})
}

func onQuestPassFb(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	fbId, ok := args[0].(uint32)
	if !ok {
		return
	}

	var count int64 = 1
	if len(args) > 1 {
		if passTimes, ok := args[1].(uint32); ok {
			count = int64(passTimes)
		}
	}

	actor.TriggerQuestEventRangeTimes(custom_id.QttPassFbTimes, fbId, count)
	actor.TriggerQuestEvent(custom_id.QttAchievementsPassFbTimes, fbId, count)
}

func onQuestPassDzzFbLevel(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	level, ok := args[0].(int64)
	if !ok {
		return
	}
	actor.TriggerQuestEvent(custom_id.QttPassDZZFbLevel, 0, level)
}

func onQuestEnterFb(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	fbId, ok := args[0].(uint32)
	if !ok {
		return
	}

	actor.TriggerQuestEvent(custom_id.QttEnterFbTimes, fbId, 1)
	actor.TriggerQuestEvent(custom_id.QttAchievementsEnterFbTimes, fbId, 1)
}

func onQuestGather(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	gatherId, ok := args[0].(uint32)
	if !ok {
		return
	}

	actor.TriggerQuestEvent(custom_id.QttGatherId, gatherId, 1)
}

func onQuestPassMirrorFb(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	mirrorId, ok := args[0].(uint32)
	if !ok {
		return
	}

	actor.TriggerQuestEvent(custom_id.QttPassMirrorFb, mirrorId, 1)
}

func onParticipateNiEnBeast(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}
	actor.TriggerQuestEvent(custom_id.QttParticipateNiEnBeast, monsterId, 1)
}

func onParticipateGroupOfNiEnBeast(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}
	actor.TriggerQuestEvent(custom_id.QttParticipateGroupOfNiEnBeast, monsterId, 1)
}

func onQuestWearItem(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}

	targetType, ok := args[0].(uint32)
	if !ok {
		return
	}

	qtype, ok := args[1].(int)
	if !ok {
		return
	}

	curItemId, ok := args[2].(uint32)
	if !ok {
		return
	}

	lastItemId, ok := args[3].(uint32)
	if !ok {
		return
	}

	curItemConf := jsondata.GetItemConfig(curItemId)
	if curItemConf == nil {
		return
	}

	curStage := curItemConf.Stage
	var lastStage uint32
	if lastItemId > 0 {
		itemConf := jsondata.GetItemConfig(lastItemId)
		lastStage = itemConf.Stage
	}

	allQuestSysDo(actor, func(sys iface.IQuestTargetSys) {
		sys.CalcQuestTargetByRange(actor, targetType, curStage, lastStage, uint32(qtype))
	})
}

func init() {
	event.RegActorEvent(custom_id.AeQuestEvent, onQuestTargetEvent)
	event.RegActorEvent(custom_id.AeQuestEventRange, onQuestTargetEventRange)
	event.RegActorEvent(custom_id.AeQuestEventByRange, onQuestTargetEventByRange)
	event.RegActorEvent(custom_id.AeLearnVocationSkill, onQuestTargetActorLearnVocationSkill)
	event.RegActorEvent(custom_id.AeKillMon, onQuestKillMon)
	event.RegActorEvent(custom_id.AePassFb, onQuestPassFb)
	event.RegActorEvent(custom_id.AePassDzzFb, onQuestPassDzzFbLevel)
	event.RegActorEvent(custom_id.AeEnterFb, onQuestEnterFb)
	event.RegActorEvent(custom_id.AeGather, onQuestGather)
	event.RegActorEvent(custom_id.AeWearItemQuestEvent, onQuestWearItem)
	event.RegActorEvent(custom_id.AePassMirrorFb, onQuestPassMirrorFb)
	event.RegActorEvent(custom_id.AeParticipateNiEnBeast, onParticipateNiEnBeast)
	event.RegActorEvent(custom_id.AeParticipateGroupOfNiEnBeast, onParticipateGroupOfNiEnBeast)
}
