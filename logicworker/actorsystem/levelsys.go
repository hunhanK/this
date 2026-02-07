package actorsystem

import (
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils"
)

type LevelSys struct {
	Base
	MaxLv uint32
}

func (sys *LevelSys) IsOpen() bool {
	return true
}

func (sys *LevelSys) OnLogin() {
	actor := sys.owner

	if role := sys.GetPlayerData().GetMainData(); nil != role {
		actor.SetExtraAttr(attrdef.Level, attrdef.AttrValueAlias(role.Level))
		actor.SetExtraAttr(attrdef.Exp, role.Exp)
	}

	_, connId := actor.GetGateInfo()
	engine.PlayerLevelChange(int64(connId), actor.GetLevel())
}

func (sys *LevelSys) OnReconnect() {
	actor := sys.owner

	_, connId := actor.GetGateInfo()
	engine.PlayerLevelChange(int64(connId), actor.GetLevel())
}

func (sys *LevelSys) CalcFinalExp(exp int64, withWorldAddRate bool, addRates ...uint32) (int64, uint32) {
	// expWithOutAddRate := exp
	var totalAddRate uint32
	if withWorldAddRate {
		// 玩家等级<150 不受影响
		if sys.GetLevel() >= jsondata.GlobalUint("getExpPlayerLvLimit") {
			totalAddRate += sys.calcWorldLvExpAddRate()
		}
	}

	addRate := functional.Reduce(addRates, func(sum, cur uint32) uint32 {
		return sum + cur
	}, 0)

	totalAddRate += addRate
	exp = int64(float64(exp) * (1 + float64(totalAddRate)/10000))
	return exp, totalAddRate
}

func (sys *LevelSys) calcWorldLvExpAddRate() uint32 {
	// 世界等级对经验的加成
	var worldAddRate uint32 = 0
	worldLv := gshare.GetWorldLevel()
	if sys.GetOwner().GetLevel() < worldLv {
		worldAddRate = jsondata.GetWorldAddRateByLv(worldLv - sys.GetOwner().GetLevel())
	}

	return worldAddRate
}

func (sys *LevelSys) SetMaxLv(lv uint32) {
	if sys.owner.GetFlyCamp() == 0 {
		lv = utils.MinUInt32(jsondata.GlobalUint("flyCampMaxRoleLv"), lv)
	}
	sys.MaxLv = lv

	sys.onAfterAddExp(0, 0, pb3.LogId_LogNirvanaLvLimitChange)
}

var worldExpAddLogIds = []pb3.LogId{
	pb3.LogId_LogDailyCircuitQuestAwards,
	pb3.LogId_LogMainQuestAwards,
	pb3.LogId_LogSectTaskQuestAwards,
	pb3.LogId_LogActFairyTeacherTruthGiveAwards,
	pb3.LogId_LogActAnswerChooseCorrect,
	pb3.LogId_LogDartCarGiveAwards,
	pb3.LogId_LogAwardFastSpiritualTravel,
	pb3.LogId_LogFairyMasterTrainFbLevelReward,
	pb3.LogId_LogUnPackGiftItem,
	pb3.LogId_LogSectTask,
}

func (sys *LevelSys) WithWorldExp(logId pb3.LogId) bool {
	for _, l := range worldExpAddLogIds {
		if logId == l {
			return true
		}
	}
	return false
}

// 添加exp: 最终增加的exp 可以通过 CalcFinalExp计算得到
func (sys *LevelSys) AddExp(exp int64, logId pb3.LogId, withWorldAddRate, sendTip bool, addRates ...uint32) (finalExpAdded int64) {
	if exp <= 0 {
		return
	}

	if !withWorldAddRate {
		withWorldAddRate = sys.WithWorldExp(logId)
	}

	expAdded, totalAddRate := sys.CalcFinalExp(exp, withWorldAddRate, addRates...)
	finalExpAdded = expAdded
	exp = finalExpAdded
	if sendTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddExp, finalExpAdded, (totalAddRate+10000)/100)
	}

	sys.onAfterAddExp(exp, finalExpAdded, logId)
	return
}

func (sys *LevelSys) AddExpV2(exp int64, logId pb3.LogId, addRateByTip uint32) (finalExpAdded int64) {
	if exp <= 0 {
		return
	}
	finalExpAdded = exp
	sys.owner.SendTipMsg(tipmsgid.TpAddExp, finalExpAdded, (addRateByTip+10000)/100)
	sys.onAfterAddExp(exp, finalExpAdded, logId)
	return
}

func (sys *LevelSys) onAfterAddExp(exp, finalExpAdded int64, logId pb3.LogId) {
	curLv := sys.owner.GetLevel()
	//TODO enhance
	for {
		next := sys.GetLevel() + 1
		needExp, ok := jsondata.GetLevelConfig(sys.owner.GetLevel() + 1)
		if !ok {
			break
		}

		// 检查是否已达到最大等级限制
		if sys.GetLevel() >= jsondata.GetMaxLimitLevel() {
			sys.storeMaxLevelExp(exp)
			exp = 0
			break
		}

		if sys.MaxLv != 0 && sys.MaxLv < next {
			sys.SetExp(sys.GetExp() + exp)
			exp = 0
			break
		}

		cur := sys.GetExp()

		if cur >= needExp {
			overflowExp := cur - needExp

			sys.SetLevel(next, pb3.LogId_LogAddExp)
			sys.SetExp(0) // 先将经验清零，后面再处理溢出经验
			sys.owner.TriggerQuestEvent(custom_id.QttUpLevelTimes, 0, 1)

			if sys.GetLevel() >= jsondata.GetMaxLimitLevel() {
				sys.storeMaxLevelExp(overflowExp + exp)
				exp = 0
				break
			}

			exp += overflowExp
		} else {
			remainNeedExp := needExp - cur
			if exp >= remainNeedExp {
				exp -= remainNeedExp
				sys.SetLevel(next, pb3.LogId_LogAddExp)
				sys.SetExp(0)
				sys.owner.TriggerQuestEvent(custom_id.QttUpLevelTimes, 0, 1)

				if sys.GetLevel() >= jsondata.GetMaxLimitLevel() {
					sys.storeMaxLevelExp(exp)
					exp = 0
					break
				}
			} else {
				sys.SetExp(cur + exp)
				exp = 0
				break
			}
		}

		if exp <= 0 {
			break
		}
	}

	// 等级有变动 并且达到了最大等级
	if curLv != sys.GetLevel() && sys.MaxLv != 0 && sys.GetLevel() == sys.MaxLv {
		mailmgr.SendMailToActor(sys.GetOwner().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_ReachCurMaxActorLevel,
			Content: &mailargs.CommonMailArgs{},
		})
	}
	logworker.LogExpLv(sys.owner, logId, &pb3.LogExpLv{Exp: finalExpAdded, FromLevel: curLv})
}

// 存储满级经验，并限制最大存储上限
func (sys *LevelSys) storeMaxLevelExp(exp int64) {
	maxExpStorage := jsondata.GetMaxExpStorage()
	currentExp := sys.GetExp()
	// 计算存储后的总经验
	totalExp := currentExp + exp
	// 限制最大存储上限
	if totalExp > maxExpStorage {
		totalExp = maxExpStorage
	}
	sys.SetExp(totalExp)
}

func (sys *LevelSys) DeductExp(exp int64, logId uint32) bool {
	if exp <= 0 {
		return false
	}
	role := sys.GetPlayerData().MainData
	cur := role.GetExp()
	if cur < exp {
		return false
	}
	role.Exp -= exp
	sys.owner.SetExtraAttr(attrdef.Exp, role.GetExp())
	return true
}

func (sys *LevelSys) GetLevel() uint32 {
	actor := sys.owner
	return actor.GetLevel()
}

func (sys *LevelSys) SetLevel(level uint32, logId pb3.LogId) {
	player := sys.owner

	old := player.GetLevel()
	actorData := sys.GetPlayerData()
	if old == level || actorData == nil {
		return
	}

	// 设置总等级
	actorData.MainData.Level = level
	player.SendActorData()
	player.SetExtraAttr(attrdef.Level, attrdef.AttrValueAlias(level))

	// 重算属性,触发任务事件
	sys.ResetSysAttr(attrdef.SaLevel)
	sys.owner.TriggerQuestEvent(custom_id.QttLevel, 0, int64(level))

	// 等级变化
	player.LogInfo("%s,%d 等级变化:level:%d->%d", player.GetName(), player.GetId(), old, level)
	if level > old {
		player.TriggerEvent(custom_id.AeLevelUp, old, level)
		player.TriggerQuestEvent(custom_id.QttLevel, 0, int64(level))
	} else {
		player.TriggerEvent(custom_id.AeLevelDown, old, level)
	}

	player.CallActorFunc(actorfuncid.OnPlayerLevelChange, &pb3.AeLevelUpArg{
		Old:   old,
		Level: level,
	})

	// 同步网关
	_, connId := player.GetGateInfo()
	engine.PlayerLevelChange(int64(connId), level)
}

func (sys *LevelSys) SetExp(exp int64) {
	if exp < 0 {
		return
	}
	role := sys.GetPlayerData().MainData
	role.Exp = exp
	sys.owner.SetExtraAttr(attrdef.Exp, exp)
}

func (sys *LevelSys) GetExp() int64 {
	return sys.GetPlayerData().MainData.Exp
}

func (sys *LevelSys) IsReachMaxLevel() bool {
	return sys.MaxLv == sys.GetLevel()
}

func calcLevelSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if jData := jsondata.GetVocationConf(player.GetJob()); nil != jData {
		if nil != jData.InitAttr {
			for _, conf := range jData.InitAttr {
				calc.AddValue(conf.Type, attrdef.AttrValueAlias(conf.Value))
			}
		}

		actorData := player.GetPlayerData().MainData
		if lvConf := jData.LevelAttr[actorData.Level]; nil != lvConf {
			engine.CheckAddAttrsToCalc(player, calc, lvConf)
		}
	}
}

func resumeHpAfterLevelUp(player iface.IPlayer, args ...interface{}) {
	if player == nil {
		return
	}
	player.FullHp()
}

func init() {
	RegisterSysClass(sysdef.SiLevel, func() iface.ISystem {
		return &LevelSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaLevel, calcLevelSysAttr)
	engine.RegQuestTargetProgress(custom_id.QttLevel, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		return actor.GetLevel()
	})

	event.RegActorEvent(custom_id.AeLevelUp, resumeHpAfterLevelUp)
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeLevel, func(actor iface.IPlayer) (score int64) {
		return int64(actor.GetLevel())
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemActorLevelUpElixir, handleUseItemActorLevelUpElixir)
}

func handleUseItemActorLevelUpElixir(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) == 0 {
		return
	}

	if conf.Param[0] == 0 {
		return
	}

	var giveExpMaxLevel = player.GetLevel()
	if giveExpMaxLevel > conf.Param[0] {
		giveExpMaxLevel = conf.Param[0]
	}

	needExp, ok := jsondata.GetLevelConfig(giveExpMaxLevel + 1)
	if !ok {
		return
	}
	for i := int64(0); i < param.Count; i++ {
		player.AddExp(needExp, pb3.LogId_LogUseItemActorLevelUpElixir, false)
	}
	return true, true, param.Count
}
