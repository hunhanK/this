package gm

import (
	"fmt"
	"jjyz/base/cmd"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/iface/iactorsys"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/entity"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gmevent"
	"math"
	"runtime/debug"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

// 发公告
func sendNotice(actor iface.IPlayer, args ...string) bool {
	if len(args) < 5 {
		return false
	}
	param := args[0]
	tipMsgId := utils.AtoUint32(args[1])
	sTime := utils.AtoUint32(args[2])
	eTime := utils.AtoUint32(args[3])
	interval := utils.AtoUint32(args[4])

	addNotice(param, tipMsgId, sTime, eTime, interval)
	return true
}

// 设置等级
func setLevel(actor iface.IPlayer, args ...string) bool {
	if len(args) <= 0 {
		return false
	}
	if sys := actor.GetSysObj(sysdef.SiLevel).(iactorsys.ILevelSys); nil != sys {
		lv := utils.AtoUint32(args[0])
		conf := jsondata.GetVocationConf(actor.GetJob())
		if conf == nil {
			return false
		}
		lv = utils.MinUInt32(lv, uint32(len(conf.LevelAttr)))
		sys.SetLevel(lv, pb3.LogId_LogGm)
		return true
	}
	return false
}

// 加经验
func addExp(actor iface.IPlayer, args ...string) bool {
	if len(args) <= 0 {
		return false
	}
	actor.AddExp(utils.AtoInt64(args[0]), pb3.LogId_LogGm, false)
	return true
}

// 玩家属性
func prop(actor iface.IPlayer, args ...string) bool {
	count := len(args)
	if count <= 0 {
		return false
	}

	var id, value uint32

	isSet := false

	id = utils.AtoUint32(args[0])
	if len(args) >= 2 {
		value = utils.AtoUint32(args[1])
		isSet = true
	}

	if isSet {
		actor.SetExtraAttr(id, attrdef.AttrValueAlias(value))
		return true
	}
	logger.LogInfo("%s %d:%d", actor.GetName(), id, value)
	actor.SendTipMsg(tipmsgid.TpStr, actor.GetExtraAttr(id))
	return true
}

// 加邮件
func addMail(actor iface.IPlayer, args ...string) bool {
	//TODO: YangQiBin 邮件
	// argsCount := len(args)
	// if argsCount < 2 {
	// 	return false
	// }
	// title, content := args[0], args[1]

	// var rewards []*json.StdReward
	// if argsCount >= 4 {
	// 	rewards = []*json.StdReward{{Id: utils.AtoUint32(args[2]), Count: utils.AtoInt64(args[3]), Bind: true}}
	// }

	// var times uint32 = 1
	// if argsCount >= 5 {
	// 	times = utils.AtoUint32(args[4])
	// }

	// for i := uint32(0); i < times; i++ {
	//	mailmgr.SendMailToActor(actor.GetId(), title, content, rewards)
	// }
	return true
}

// 充值
func charge(actor iface.IPlayer, args ...string) bool {
	if len(args) <= 0 {
		return false
	}
	sys, ok := actor.GetSysObj(sysdef.SiCharge).(*actorsystem.ChargeSys)
	if !ok {
		return false
	}

	var cashNum uint32
	chargeId := utils.AtoUint32(args[0])
	conf := jsondata.GetChargeConf(chargeId)
	if nil != conf {
		cashNum = conf.CashCent
	} else {
		cashNum = utils.AtoUint32(args[1])
	}
	var params = &pb3.OnChargeParams{
		ChargeId:           chargeId,
		CashCent:           cashNum,
		SkipLogFirstCharge: true,
	}
	sys.OnCharge(params, pb3.LogId_LogGm)
	return true
}

// 跨周
func newWeek(actor iface.IPlayer, args ...string) bool {
	actor.NewWeekArrive()

	return true
}

func mockTime(actor iface.IPlayer, args ...string) bool {
	time_util.MockCurTime(time.Unix(int64(utils.AtoUint32(args[0])), 0))
	return true
}

func cancelMockTime(actor iface.IPlayer, args ...string) bool {
	time_util.CancelMockTime()
	return true
}

func sendTips(actor iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}
	tipsId := utils.AtoUint32(args[0])
	var tipArgs []interface{}
	for _, arg := range args[1:] {
		tipArgs = append(tipArgs, arg)
	}
	actor.SendTipMsg(tipsId, tipArgs...)
	return true
}

func addMoney(actor iface.IPlayer, args ...string) bool {
	if len(args) < 2 {
		return false
	}

	actor.AddMoney(utils.AtoUint32(args[0]), utils.AtoInt64(args[1]), true, pb3.LogId_LogGm)
	return true
}

func freeMemory(actor iface.IPlayer, args ...string) bool {
	debug.FreeOSMemory()
	return true
}

func newDay(player iface.IPlayer, args ...string) bool {
	player.TriggerEvent(custom_id.AeBeforeNewDay)

	player.GetPlayerData().MainData.DayOnlineTime = 0
	player.GetPlayerData().MainData.NewDayResetTime = time_util.NowSec()

	player.NewDayArrive()
	return true
}

// func enterCross(actor iface.IPlayer, args ...string) bool {
// 	actor.EnterCrossMainScene()

// 	return true
// }

// func exitCross(actor iface.IPlayer, args ...string) bool {
// 	if et, ok := actor.(*entity.Player); ok {
// 		et.EnterLastFb()
// 	}
// 	return true
// }

func actorCache(actor iface.IPlayer, args ...string) bool {
	fileName := args[0]
	actorId := actor.GetId()
	if et, ok := actor.(*entity.Player); ok {
		et.ReloadCacheKick = true
	}

	gshare.SendDBMsg(custom_id.GMsgLoadActorCache, actorId, fileName)
	return true
}

func cacheGlobalVar(actor iface.IPlayer, args ...string) bool {
	fileName := args[0]
	gshare.SendDBMsg(custom_id.GMsgGMLoadGlobalVarByLocalFile, fileName)
	return true
}

func deleteActor(actor iface.IPlayer, args ...string) bool {
	gshare.SendDBMsg(custom_id.GMsgDeletePlayer, utils.AtoUint64(args[0]), utils.AtoUint32(args[1]))
	return true
}

func recoverActor(actor iface.IPlayer, args ...string) bool {
	recoveryPlayer(utils.AtoUint64(args[0]), utils.AtoUint32(args[1]))
	return true
}

func learnSkill(actor iface.IPlayer, args ...string) bool {
	return actor.LearnSkill(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]), true)
}

func forgetSkill(actor iface.IPlayer, args ...string) bool {
	actor.ForgetSkill(utils.AtoUint32(args[0]), true, true, true)
	return true
}

func showOpenDay(actor iface.IPlayer, args ...string) bool {
	logger.LogDebug("开服天数 %d", gshare.GetOpenServerDay())
	actor.SendTipMsg(tipmsgid.TpStr, gshare.GetOpenServerDay())
	return true
}

func showSmallCrossDay(actor iface.IPlayer, args ...string) bool {
	crossDay := gshare.GetSmallCrossDay()
	logger.LogDebug("小跨服天数 %d", crossDay)
	actor.SendTipMsg(tipmsgid.TpStr, fmt.Sprintf("小跨服天数 %d", crossDay))
	return true
}

func showMergeDay(actor iface.IPlayer, args ...string) bool {
	mergeDay := gshare.GetMergeSrvDay()
	logger.LogDebug("合服天数 %d", mergeDay)
	actor.SendTipMsg(tipmsgid.TpStr, mergeDay)
	return true
}

func showMergeTimes(actor iface.IPlayer, args ...string) bool {
	mergeTimes := gshare.GetMergeTimes()
	logger.LogDebug("合服次数 %d", mergeTimes)
	actor.SendTipMsg(tipmsgid.TpStr, mergeTimes)
	return true
}

func setVip(player iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}

	vipLv := utils.AtoUint32(args[0])
	conf := jsondata.GetVipLevelConfByLevel(vipLv)
	if conf.NeedScore > uint32(player.GetExtraAttr(attrdef.VipExp)) {
		exp := conf.NeedScore - uint32(player.GetExtraAttr(attrdef.VipExp))
		player.GetSysObj(sysdef.SiVip).(*actorsystem.VipSys).AddExp(exp)
	}
	return true
}

func setCircleLev(player iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*actorsystem.NewJingJieSys)
	if !ok {
		return false
	}
	circleLev := utils.AtoUint32(args[0])

	data := sys.GetData()
	oldLv := data.GetLevel()
	data.Level = circleLev
	player.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Level))
	player.TriggerQuestEvent(custom_id.QttCircle, 0, int64(data.Level))
	player.TriggerEvent(custom_id.AeCircleChange, oldLv, data.Level)
	player.TriggerQuestEventRange(custom_id.QttNewJingJieUpLv)
	sys.ResetSysAttr(attrdef.SaNewJingJie)

	sys.SendProto3(153, 2, &pb3.S2C_153_2{
		Ret:         true,
		Level:       data.Level,
		LoseTimes:   data.LoseTimes,
		RecoverTime: data.RecoverTime,
	})
	return true
}

func kickSelf(player iface.IPlayer, args ...string) bool {
	if player == nil {
		return false
	}

	player.ClosePlayer(cmd.DCRKick)
	return true
}

func rich(player iface.IPlayer, args ...string) bool {
	if player == nil {
		return false
	}

	for i := moneydef.MoneyUnknown + 1; i < moneydef.MoneyEnd; i++ {
		moneySys := player.GetSysObj(sysdef.SiMoney).(*actorsystem.MoneySys)
		if moneySys == nil {
			logger.LogError("rich moneySys is nil")
			return false
		}
		moneySys.SetMoney(uint32(i), math.MaxInt16, true, pb3.LogId_LogGmRich)
	}

	return true
}

func poor(player iface.IPlayer, args ...string) bool {
	if player == nil {
		return false
	}

	for i := moneydef.MoneyStart; i <= moneydef.MoneyEnd; i++ {
		moneySys := player.GetSysObj(sysdef.SiMoney).(*actorsystem.MoneySys)
		if moneySys == nil {
			logger.LogError("poor moneySys is nil")
			return false
		}
		moneySys.SetMoney(uint32(i), 0, true, pb3.LogId_LogGmRich)
	}

	return true
}

func init() {
	gmevent.Register("rsf", func(actor iface.IPlayer, args ...string) bool {
		gcommon.LoadJsonData(true)
		return true
	}, 1)

	gmevent.Register("rich", rich, 1)
	gmevent.Register("poor", poor, 1)
	gmevent.Register("addmoney", addMoney, 1)
	gmevent.Register("sendNotice", sendNotice, 1)
	gmevent.Register("level", setLevel, 1)
	gmevent.Register("addexp", addExp, 1)
	gmevent.Register("prop", prop, 1)
	gmevent.Register("addmail", addMail, 1)
	gmevent.Register("charge", charge, 1)
	gmevent.Register("newweek", newWeek, 1)
	gmevent.Register("freemem", freeMemory, 10)
	gmevent.Register("newday", newDay, 1)
	gmevent.Register("srvNewDay", func(player iface.IPlayer, args ...string) bool {
		event.TriggerSysEvent(custom_id.SeNewDayArrive)
		return true
	}, 1)
	gmevent.Register("cache", actorCache, 1)
	gmevent.Register("delete", deleteActor, 1)
	gmevent.Register("recovery", recoverActor, 1)
	gmevent.Register("skill", learnSkill, 1)
	gmevent.Register("fskill", forgetSkill, 1)
	gmevent.Register("openday", showOpenDay, 1)
	gmevent.Register("smallcrossday", showSmallCrossDay, 1)
	gmevent.Register("mergeday", showMergeDay, 1)
	gmevent.Register("mergetimes", showMergeTimes, 1)
	gmevent.Register("setvip", setVip, 1)
	gmevent.Register("circle", setCircleLev, 1)
	gmevent.Register("kickself", kickSelf, 1)
	gmevent.Register("mockTime", mockTime, 1)
	gmevent.Register("cancelMockTime", cancelMockTime, 1)
	gmevent.Register("tips", sendTips, 1)
	gmevent.Register("cacheglobalvar", cacheGlobalVar, 1)
	gmevent.Register("replacePYY", func(player iface.IPlayer, args ...string) bool {
		actorId := utils.AtoUint64(args[0])
		yy := gshare.GetStaticVar().GlobalPlayerYY[actorId]
		if yy != nil {
			gshare.GetStaticVar().GlobalPlayerYY[player.GetId()] = yy
		}
		return true
	}, 1)
	gmevent.Register("gmSet360Wan", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		engine.SetGM360Wan(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]))
		return true
	}, 1)
	gmevent.Register("setDitchId", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}

		ditchId := utils.AtoUint32(args[0])
		player.SetExtraAttr(attrdef.DitchId, attrdef.AttrValueAlias(ditchId))
		return true
	}, 1)
}
