package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/reachconddef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

const (
	LimitTargeCannot uint32 = 0 // 不可领取
	LimitTargeCan    uint32 = 1 // 可领取
	LimitTargeIsGet  uint32 = 2 // 已领取
)

type LimitTargetSys struct {
	Base
	timer *time_util.Timer
}

func (sys *LimitTargetSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.LimitTarget {
		binary.LimitTarget = &pb3.LimitTarget{}
	}
}

func (sys *LimitTargetSys) OnOpen() {
	sys.checkLimitTargetLv(sys.owner.GetLevel(), sys.owner.GetLevel())
}

func (sys *LimitTargetSys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *LimitTargetSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *LimitTargetSys) onNewDay() {
	// 清空上一期的开服限时目标
	sys.clearBeforeLimitTimeOpenData()

	// 检测是否有新一期的开服目标
	sys.checkLimitTimeOpenStart()

	// 跨天通知下前端数据
	sys.s2cInfo()
}

func (sys *LimitTargetSys) GetLimitTimeLvData() *pb3.LimitTarget {
	binary := sys.GetBinaryData()
	if nil == binary.LimitTarget {
		binary.LimitTarget = &pb3.LimitTarget{}
	}
	return binary.LimitTarget
}

func (sys *LimitTargetSys) logFinish(openDay uint32, condType uint32, condVal []int64) {
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogLimitTargetFinish, &pb3.LogPlayerCounter{
		NumArgs: uint64(condType),
		StrArgs: logworker.ConvertJsonStr(map[string]any{
			"finishId": openDay,
			"condType": condType,
			"condVal":  condVal,
		}),
	})
}

func (sys *LimitTargetSys) clearBeforeLimitTimeOpenData() {
	nowSec := time_util.NowSec()
	data := sys.GetLimitTimeLvData()
	if data.Id <= 0 {
		return
	}
	conf := jsondata.GetLimitTargetConfById(data.Id)
	if nil == conf {
		return
	}
	if data.GetStartTime() > 0 && (data.Flag == LimitTargeIsGet || data.GetEndTime() <= nowSec) {
		if nil != data.LimitData && data.LimitData.OpenDay > 0 { //已开启过的
			if dayConf, ok := conf.OpenLimit[data.LimitData.OpenDay]; ok {
				if CheckReach(sys.owner, dayConf.CondType, dayConf.CondVal) && !data.LimitData.Flag {
					data.LimitData.Flag = true
					mailmgr.SendMailToActor(sys.owner.GetId(),
						&mailargs.SendMailSt{
							ConfId:  common.Mail_OpenDayLimitTimeTitle,
							Rewards: dayConf.FinishAward,
						})
					sys.logFinish(data.LimitData.OpenDay, dayConf.CondType, dayConf.CondVal)
				}
			}
		}
	}
	data.LimitData = nil

}

// 检测是否开启开服限时目标
func (sys *LimitTargetSys) checkLimitTimeOpenStart() {
	nowSec := time_util.NowSec()
	data := sys.GetLimitTimeLvData()
	if data.Id <= 0 {
		return
	}
	conf := jsondata.GetLimitTargetConfById(data.GetId())
	if data.GetStartTime() > 0 && (data.Flag == LimitTargeIsGet || data.GetEndTime() <= nowSec) {
		day := sys.getOpenLimitDayByTime(data.GetStartTime())
		if nil != data.LimitData && data.LimitData.OpenDay > 0 {
			return
		}
		if _, ok := conf.OpenLimit[day]; ok {
			data.LimitData = &pb3.LimitTimeOpenData{
				OpenDay: day,
				Flag:    false,
			}
		}
	}
}

func (sys *LimitTargetSys) OnLogout() {
	if nil != sys.timer {
		sys.timer.Stop()
		sys.timer = nil
	}
}

func (sys *LimitTargetSys) OnLogin() {
	sys.checkLimitTargetLv(sys.owner.GetLevel(), sys.owner.GetLevel())
}

func (sys *LimitTargetSys) onActorLevelUp(oldLv, newLv uint32) {
	// 检测是否开启限时等级
	sys.checkLimitTargetLv(oldLv, newLv)
}

func (sys *LimitTargetSys) checkLvTask(oldLv, newLv uint32, data *pb3.LimitTarget) {
	nowSec := time_util.NowSec()
	ltConf := jsondata.GetLimitTargetConf()
	if nil == ltConf {
		return
	}
	if data.GetId() > 0 || data.GetStartTime() > 0 {
		return
	}
	for k, conf := range ltConf {
		if utils.SliceContainsUint32(data.FinishId, k) {
			continue
		}
		if oldLv <= conf.Lv && newLv >= conf.Lv { //登录时刚好相等，或者刚升上来的
			data.Id = k
			data.StartTime = nowSec
			data.EndTime = nowSec + conf.FinishHour*60*60
			data.Flag = LimitTargeCannot
			sys.changeTaskStatus(data) //触发时完成条件
			return
		}
	}
}

func (sys *LimitTargetSys) sendLvAwardMail(data *pb3.LimitTarget, conf *jsondata.LimitTargetConf) {
	if data.Flag == LimitTargeCan {
		data.Flag = LimitTargeIsGet
		mailmgr.SendMailToActor(sys.owner.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_OpenDayLimitTimeTitle,
			Rewards: conf.LvAward,
		})
	}
}

func (sys *LimitTargetSys) isFinishLvTask(data *pb3.LimitTarget) bool {
	if data.Id <= 0 || data.StartTime <= 0 {
		sys.LogWarn("the limit target task(%d) not start", data.Id)
		return false
	}
	nowSec := time_util.NowSec()
	if data.EndTime <= nowSec || data.Flag == LimitTargeIsGet {
		return true
	}
	return false
}

func (sys *LimitTargetSys) getOpenLimitDayByTime(startTime uint32) uint32 {
	var day uint32
	stZero := time_util.GetZeroTime(startTime)
	todayZero := time_util.GetDaysZeroTime(0)
	if todayZero >= stZero {
		day = todayZero/86400 - stZero/86400 + 1
	}
	return day
}

func (sys *LimitTargetSys) isFinishTask(data *pb3.LimitTarget) bool {
	//等级
	if !sys.isFinishLvTask(data) {
		return false
	}
	//限时目标
	conf := jsondata.GetLimitTargetConfById(data.GetId())
	if nil == conf {
		sys.LogError("the limit target task(%d) conf nil", data.GetId())
		return false
	}
	day := sys.getOpenLimitDayByTime(data.GetStartTime())
	for openDay := range conf.OpenLimit {
		if day <= openDay { //天数没走完
			return false
		}
	}
	return true
}

func (sys *LimitTargetSys) changeTaskStatus(data *pb3.LimitTarget) {
	if data.Id <= 0 {
		return
	}
	conf := jsondata.GetLimitTargetConfById(data.Id)
	if nil == conf {
		return
	}
	nowSec := time_util.NowSec()
	if data.GetStartTime() > 0 {
		if data.GetEndTime() > nowSec { //未结束
			flag := data.GetFlag()
			if flag == LimitTargeCannot {
				if sys.owner.GetLevel() >= conf.FinishLv {
					data.Flag = LimitTargeCan
					sys.logFinish(1, reachconddef.ReachStandard_Level, []int64{int64(conf.FinishLv)})
				}
			}
			if data.Flag == LimitTargeCan { //可领取状态
				if nil != sys.timer {
					sys.timer.Stop()
				}
				sys.timer = sys.owner.SetTimeout(time.Duration(data.GetEndTime()-nowSec)*time.Second, func() {
					sys.sendLvAwardMail(data, conf)
					sys.checkNextLoop()
				})
			}
		} else {
			//超时直接结束发邮件
			sys.sendLvAwardMail(data, conf)
			sys.checkNextLoop()
		}
	}
}

func (sys *LimitTargetSys) checkLimitTargetLv(oldLv, newLv uint32) {
	ltConf := jsondata.GetLimitTargetConf()
	if nil == ltConf {
		return
	}
	data := sys.GetLimitTimeLvData()
	sys.changeTaskStatus(data)
	if data.Id > 0 {
		if sys.isFinishTask(data) {
			data.FinishId = append(data.FinishId, data.Id)
			data = &pb3.LimitTarget{
				FinishId: data.FinishId,
			}
		}
	}
	sys.checkLvTask(oldLv, newLv, data)
	sys.s2cInfo()
}

func (sys *LimitTargetSys) s2cInfo() {
	sys.SendProto3(44, 0, &pb3.S2C_44_0{LimitTarget: sys.GetBinaryData().LimitTarget})
}

func c2sGetLimitTimeAward(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		s := sys.(*LimitTargetSys)
		return s.getLimitTimeAward(msg)
	}
}

func (sys *LimitTargetSys) getLimitTimeAward(msg *base.Message) error {
	data := sys.GetLimitTimeLvData()
	conf := jsondata.GetLimitTargetConfById(data.GetId())
	if nil == conf {
		return neterror.ConfNotFoundError("no limittarget conf(%d)", data.GetId())
	}

	if data.GetStartTime() > 0 && (data.GetEndTime() > time_util.NowSec() && data.GetFlag() != LimitTargeIsGet) { //活动时限内且未领取视为未结束等级活动
		if data.GetFlag() != LimitTargeCan {
			return neterror.ParamsInvalidError("limittarget cond not meet")
		}
		data.Flag = LimitTargeIsGet
		if !engine.GiveRewards(sys.owner, conf.LvAward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLimitTargetLvReward}) {
			sys.LogError("limit target give award err")
		}
		engine.BroadcastTipMsgById(conf.TipId, sys.owner.GetId(), sys.owner.GetName(), engine.StdRewardToBroadcast(sys.owner, conf.LvAward))
		sys.checkNextLoop()
	} else {
		if nil == data.LimitData || data.LimitData.OpenDay <= 0 {
			return nil
		}
		dayConf := jsondata.GetLimitOpenTargetConfById(data.GetId(), data.LimitData.OpenDay)
		if dayConf == nil {
			return neterror.ConfNotFoundError("limittarget conf(%d) day(%d) is nil", data.GetId(), data.LimitData.OpenDay)
		}

		if data.LimitData.Flag {
			sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}

		if !CheckReach(sys.owner, dayConf.CondType, dayConf.CondVal) {
			sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
			return nil
		}

		data.LimitData.Flag = true
		engine.GiveRewards(sys.owner, dayConf.FinishAward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLimitTargetOpenReward})
		engine.BroadcastTipMsgById(dayConf.TipId, sys.owner.GetId(), sys.owner.GetName(), engine.StdRewardToBroadcast(sys.owner, dayConf.FinishAward))
		sys.logFinish(data.LimitData.OpenDay, dayConf.CondType, dayConf.CondVal)
	}

	sys.s2cInfo()
	return nil
}

func (sys *LimitTargetSys) checkNextLoop() {
	data := sys.GetLimitTimeLvData()
	if data.GetStartTime() == 0 {
		return
	}
	nowSec := time_util.NowSec()
	if data.Flag != LimitTargeIsGet && data.GetEndTime() > nowSec {
		return
	}
	if time_util.IsSameDay(data.GetStartTime(), data.GetEndTime()) {
		return
	}
	if nil != data.LimitData && data.LimitData.OpenDay > 0 {
		return
	}
	sys.checkLimitTimeOpenStart()
	sys.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiLimitTarget, func() iface.ISystem {
		return &LimitTargetSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiLimitTarget).(*LimitTargetSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})
	event.RegActorEvent(custom_id.AeLevelUp, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) <= 0 {
			return
		}
		oldLv, ok := args[0].(uint32)
		if !ok {
			return
		}
		newLv, ok := args[1].(uint32)
		if !ok {
			return
		}
		if sys, ok := actor.GetSysObj(sysdef.SiLimitTarget).(*LimitTargetSys); ok {
			sys.onActorLevelUp(oldLv, newLv)
		}
	})

	net.RegisterSysProtoV2(44, 1, sysdef.SiLimitTarget, c2sGetLimitTimeAward)

	gmevent.Register("changeLimitTarget", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiLimitTarget).(*LimitTargetSys)
		if !ok {
			return false
		}

		day := utils.AtoUint32(args[0])
		conf := jsondata.GetLimitTargetConfById(1)
		if _, ok := conf.OpenLimit[day]; !ok {
			return false
		}

		data := sys.GetLimitTimeLvData()
		if data.EndTime > time_util.NowSec() {
			data.EndTime = time_util.NowSec()
		}

		sys.clearBeforeLimitTimeOpenData()
		data.LimitData = &pb3.LimitTimeOpenData{
			OpenDay: day,
			Flag:    false,
		}
		sys.s2cInfo()

		return true
	}, 1)
}
