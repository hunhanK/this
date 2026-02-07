/**
 * @Author: LvYuMeng
 * @Date: 2024/11/7
 * @Desc: 讨伐令
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/internal/timer"
	"math"
	"time"
)

type KillTokenSys struct {
	Base
	recoverTimer *time_util.Timer
}

func (sys *KillTokenSys) OnInit() {

}

func (sys *KillTokenSys) GetKillTokenLimit() uint32 {
	conf := jsondata.GetKillTokenConf()
	if conf == nil {
		sys.LogWarn("conf not found")
		return 0
	}
	data := sys.getData()
	return conf.Limit + data.AddKillTotenLimit
}

func (sys *KillTokenSys) InitData() {
	sys.onLoginTrySetTimer()
	sys.owner.SetExtraAttr(attrdef.KillTokenLimit, int64(sys.GetKillTokenLimit()))
	sys.owner.SetExtraAttr(attrdef.KillToken, int64(sys.getData().KillToken))
}

func (sys *KillTokenSys) getData() *pb3.KillTokenData {
	binary := sys.owner.GetBinaryData()
	if nil == binary.KillToken {
		binary.KillToken = &pb3.KillTokenData{}
	}
	return binary.KillToken
}

func (sys *KillTokenSys) OnOpen() {
	conf := jsondata.GetKillTokenConf()
	if conf == nil {
		sys.LogWarn("conf not found")
		return
	}
	data := sys.getData()
	data.KillToken = conf.Origin
	sys.InitData()
}

func (sys *KillTokenSys) OnLogin() {
	sys.InitData()
}

func (sys *KillTokenSys) OnAfterLogin() {
}

func (sys *KillTokenSys) OnReconnect() {
}

func (sys *KillTokenSys) OnDestroy() {
	if sys.recoverTimer != nil {
		sys.recoverTimer.Stop()
		sys.recoverTimer = nil
	}
}

func (sys *KillTokenSys) onLoginTrySetTimer() {
	if sys.recoverTimer != nil {
		return
	}

	conf := jsondata.GetKillTokenConf()
	if conf == nil || conf.Time == 0 {
		sys.LogWarn("conf not found")
		return
	}

	data := sys.getData()
	if data.GetKillToken() >= sys.GetKillTokenLimit() {
		return
	}

	nextAddTime := data.GetNextAddKillTokenTime()
	if nextAddTime <= 0 {
		sys.trySetTimer()
		return
	}

	now := time_util.NowSec()
	if nextAddTime < now {
		// 算出离线增加多少讨伐令
		addKillToken := (now-nextAddTime)/conf.Time + 1
		sys.AddKillToken(addKillToken)

		// 算出下次增加的讨伐令的时间
		residueTime := (now - nextAddTime) % conf.Time
		nextAddTime = now + conf.Time - residueTime
	}

	data.NextAddKillTokenTime = nextAddTime
	sys.recoverTimer = timer.SetTimeout(time.Duration(nextAddTime-now)*time.Second, func() {
		sys.recoverTimer.Stop()
		sys.recoverTimer = nil
		if sys.AddKillToken(conf.Num) {
			sys.trySetTimer()
		}
	})
}

func (sys *KillTokenSys) trySetTimer() {
	if sys.recoverTimer != nil {
		return
	}
	conf := jsondata.GetKillTokenConf()
	if conf == nil || conf.Time == 0 {
		sys.LogWarn("conf not found")
		return
	}

	data := sys.getData()
	if data.GetKillToken() >= sys.GetKillTokenLimit() {
		return
	}
	now := time_util.NowSec()
	nextAddTime := now + conf.Time
	data.NextAddKillTokenTime = nextAddTime
	sys.recoverTimer = timer.SetTimeout(time.Duration(nextAddTime-now)*time.Second, func() {
		sys.recoverTimer.Stop()
		sys.recoverTimer = nil
		if sys.AddKillToken(conf.Num) {
			sys.trySetTimer()
		}
	})
}

func (sys *KillTokenSys) AddKillToken(num uint32) bool {
	data := sys.getData()
	cur := data.GetKillToken()
	limit := sys.GetKillTokenLimit()
	if cur >= limit {
		return false
	}
	newNum := cur + num
	if newNum > limit {
		newNum = limit
	}
	data.KillToken = newNum
	sys.owner.SetExtraAttr(attrdef.KillToken, int64(data.GetKillToken()))
	return true
}

func (sys *KillTokenSys) DecKillToken(num uint32) bool {
	data := sys.getData()
	cur := data.GetKillToken()
	if cur < num {
		return false
	}
	cur -= num
	data.KillToken = cur
	sys.owner.SetExtraAttr(attrdef.KillToken, int64(cur))
	sys.trySetTimer()

	curUse := data.GetDailyUseKillToken()
	data.DailyUseKillToken = curUse + num

	return true
}

func (sys *KillTokenSys) onNewDay() {
	data := sys.getData()
	data.DailyUseKillToken = 0
}

func (sys *KillTokenSys) AddKillTokenLimit(add uint32) {
	conf := jsondata.GetKillTokenConf()
	if conf == nil || conf.Time == 0 {
		sys.LogWarn("conf not found")
		return
	}
	data := sys.getData()
	data.AddKillTotenLimit += add
	limit := conf.KillTokenMax
	if limit < data.AddKillTotenLimit {
		data.AddKillTotenLimit = limit
	}

	sys.owner.SetExtraAttr(attrdef.KillTokenLimit, int64(sys.GetKillTokenLimit()))

	sys.AddKillToken(add)
}

func (sys *KillTokenSys) BossSweepChecker(monId uint32) bool {
	conf := jsondata.GetMonsterConf(monId)
	if conf == nil {
		return false
	}
	token := conf.KillToken
	data := sys.getData()
	cur := data.GetKillToken()
	if cur < token {
		return false
	}
	return true
}

func (sys *KillTokenSys) BossSweepSettle(monId, sceneId uint32, rewards jsondata.StdRewardVec) bool {
	conf := jsondata.GetMonsterConf(monId)
	if conf == nil {
		return false
	}
	token := conf.KillToken
	if !sys.DecKillToken(token) {
		return false
	}
	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiMenBossSweepAwards})
	sys.owner.SendShowRewardsPop(rewards)
	return true
}

func onDecKillToken(actor iface.IPlayer, buf []byte) {
	msg := &pb3.DecKillton{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiKillToken).(*KillTokenSys)
	if ok {
		sys.DecKillToken(msg.DecKillToken)
	}
}

// 使用物品恢复讨伐令
func useItemAddKillToken(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiKillToken).(*KillTokenSys)
	if !ok || !sys.IsOpen() {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return false, false, 0
	}

	addNum := conf.Param[0]

	limitNum := sys.GetKillTokenLimit()
	data := sys.getData()
	cur := data.GetKillToken()
	if cur+addNum > limitNum {
		return
	}

	residueNum := limitNum - cur
	useItemCount := int64(math.Floor(float64(residueNum) / float64(addNum)))
	if useItemCount > param.Count {
		useItemCount = param.Count
	}

	add := addNum * uint32(useItemCount)
	if sys.AddKillToken(add) {
		return true, true, useItemCount
	}

	return false, false, 0
}

// 使用道具增加讨伐令上限
func useItemAddKillTokenLimit(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiKillToken).(*KillTokenSys)
	if !ok || !sys.IsOpen() {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return false, false, 0
	}

	data := sys.getData()
	curAdd := data.AddKillTotenLimit
	limit := jsondata.GetKillTokenConf().KillTokenMax
	if limit > curAdd {
		return false, false, 0
	}
	dif := limit - curAdd
	canUse := int64(dif / conf.Param[0])
	use := param.Count
	if canUse < use {
		use = canUse
	}

	if use == 0 {
		return
	}

	addValue := use * int64(conf.Param[0])
	sys.AddKillTokenLimit(uint32(addValue))
	return true, true, use
}

func init() {
	RegisterSysClass(sysdef.SiKillToken, func() iface.ISystem {
		return &KillTokenSys{}
	})
	engine.RegisterActorCallFunc(playerfuncid.DecKillToken, onDecKillToken)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddKillToken, useItemAddKillToken)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddKillTokenLimit, useItemAddKillTokenLimit)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiKillToken).(*KillTokenSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})
}
