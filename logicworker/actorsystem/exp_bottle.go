/**
 * @Author: yzh
 * @Date:
 * @Desc: 经验瓶
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type ExpBottleSys struct {
	Base
}

func (s *ExpBottleSys) OnLvUp() {
	s.sendState()
}

func (s *ExpBottleSys) OnOpen() {
	s.sendState()
}

func (s *ExpBottleSys) OnNewDay() {
	s.sendState()
}

func (s *ExpBottleSys) OnLogin() {
	s.GetOwner().LogDebug("exp bottle on login")
	s.sendState()
}

func (s *ExpBottleSys) OnReconnect() {
	s.sendState()
}

func (s *ExpBottleSys) sendState() {
	s.SendProto3(141, 1, &pb3.S2C_141_1{
		State: s.state(),
	})
}

func (s *ExpBottleSys) state() *pb3.ExpBottleState {
	conf := jsondata.GetExpBottleConf()
	binary := s.owner.GetBinaryData()
	state := binary.ExpBottleState

	if state == nil {
		state = &pb3.ExpBottleState{
			ResetAt: time_util.NowSec(),
		}
		binary.ExpBottleState = state
	}

	if !time_util.IsSameDay(state.ResetAt, time_util.NowSec()) {
		state.ResetAt = time_util.NowSec()
		state.KilledWildBoosToday = 0
		state.ReceivableBottleNum = 0
		state.ReceivedBottleNumToday = 0
	}

	s.calcReceivableExp(state)

	if state.ReceivedBottleNumToday+state.ReceivableBottleNum < s.getMaxReceivableBottleNumDaily() {
		state.ReceivableBottleNum = state.KilledWildBoosToday/conf.NeedKillWildBossNumPerBottle - state.ReceivedBottleNumToday
	}

	return state
}

func (s *ExpBottleSys) getMaxReceivableBottleNumDaily() uint32 {
	conf := jsondata.GetExpBottleConf()
	if conf == nil {
		s.GetOwner().LogWarn("revExpBottle not found expBottle conf")
		return 0
	}
	var maxReceivableBottleNumDaily = conf.InitRevBottleNum + uint32(s.owner.GetFightAttr(attrdef.ExpBottleDailyLimitAdd))
	s.GetOwner().LogInfo("MaxReceivableBottleNumDaily is %d = %d + %d", maxReceivableBottleNumDaily, conf.InitRevBottleNum, uint32(s.owner.GetFightAttr(attrdef.ExpBottleDailyLimitAdd)))
	return maxReceivableBottleNumDaily
}

func (s *ExpBottleSys) revExpBottle(agreeBuyExpMultipleRatePackage bool) {
	state := s.state()

	if state.ReceivableBottleNum == 0 {
		s.GetOwner().LogWarn("not receivable ")
		return
	}

	if state.ReceivedBottleNumToday >= s.getMaxReceivableBottleNumDaily() {
		s.GetOwner().LogWarn("recv cnt over max")
		return
	}

	expConf, ok := s.getExpConf()
	if !ok {
		s.GetOwner().LogWarn("not found exp conf")
		return
	}

	rate := expConf.MultipleRate
	if agreeBuyExpMultipleRatePackage {
		if !s.owner.ConsumeByConf(expConf.ExpMultipleRatePackage.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyExpBottleMultipleRate}) {
			s.GetOwner().LogWarn("not enough consume")
			return
		}
		rate += expConf.ExpMultipleRatePackage.AddRate
	}

	exp := state.BasicExp
	var multiple uint32 = 1
	if random.Hit(rate, 10000) {
		expBottleMultipleWeightPool := new(random.Pool)
		for i, weight := range expConf.ExpMultiple.Weights {
			expBottleMultipleWeightPool.AddItem(expConf.ExpMultiple.Multiple[i], weight)
		}
		multiple = expBottleMultipleWeightPool.RandomOne().(uint32)
		exp *= multiple
	}

	// 会自动加成世界等级经验
	finalExp := s.owner.GetSysObj(sysdef.SiLevel).(*LevelSys).AddExp(int64(exp), pb3.LogId_LogAddExp, true, false)

	state.ReceivedBottleNumToday += 1
	state.ReceivableBottleNum -= 1

	logArg, _ := json.Marshal(map[string]interface{}{
		"isBuy":         agreeBuyExpMultipleRatePackage,
		"multiple":      multiple,
		"exp":           exp,
		"dayReceiveNum": state.ReceivedBottleNumToday,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogExpBottleReceive, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	s.owner.TriggerQuestEvent(custom_id.QttRevExpBottleCnt, 0, 1)
	s.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiExpBottle, 1) // 触发资源找回事件

	s.SendProto3(141, 2, &pb3.S2C_141_2{
		Exp:      uint32(finalExp),
		Multiple: multiple,
		State:    state,
	})
}

func (s *ExpBottleSys) getExpConf() (*jsondata.ExpBottleExp, bool) {
	curLv := s.owner.GetLevel()
	conf := jsondata.GetExpBottleConf()
	if conf == nil {
		return nil, false
	}
	var expConf *jsondata.ExpBottleExp
	for _, expCfg := range conf.ExpConfs {
		if expCfg.MinLv <= curLv && expCfg.MaxLv >= curLv {
			expConf = expCfg
			break
		}
	}
	if expConf == nil {
		return nil, false
	}
	return expConf, true
}

func (s *ExpBottleSys) OnLevelChange() {
	s.sendState()
}

func (s *ExpBottleSys) calcReceivableExp(state *pb3.ExpBottleState) {
	expConf, ok := s.getExpConf()
	if !ok {
		s.GetOwner().LogWarn("not found exp conf")
		return
	}

	state.BasicExp = expConf.BasicExp

	// 世界等级对经验的加成
	var worldAddRate uint32 = 0
	worldLv := gshare.GetWorldLevel()
	if s.GetOwner().GetLevel() < worldLv {
		worldAddRate = jsondata.GetWorldAddRateByLv(worldLv - s.GetOwner().GetLevel())
	}
	state.WorldLvExp = state.BasicExp * (worldAddRate / 10000)
}

func onExpBottleSysCrossDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiExpBottle).(*ExpBottleSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.OnNewDay()
}

func c2sRevExpBottle(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		var req pb3.C2S_141_1
		if err := msg.UnpackagePbmsg(&req); err != nil {
			return neterror.Wrap(err)
		}
		sys.(*ExpBottleSys).revExpBottle(req.AgreeBuyExpMultipleRatePackage)
		return nil
	}
}

// 玩家等级变动
func onExpBottleSysLevelChange(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiExpBottle).(*ExpBottleSys); ok {
		if !sys.IsOpen() {
			return
		}
		sys.OnLevelChange()
	}
}

func init() {
	RegisterSysClass(sysdef.SiExpBottle, func() iface.ISystem {
		return &ExpBottleSys{}
	})

	event.RegActorEvent(custom_id.AeLevelUp, onExpBottleSysLevelChange)

	net.RegisterSysProtoV2(141, 1, sysdef.SiExpBottle, c2sRevExpBottle)

	event.RegActorEvent(custom_id.AeNewDay, onExpBottleSysCrossDay)
	event.RegActorEvent(custom_id.AeKillMon, expBottleHandleKillMon)
}

func expBottleHandleKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}

	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}
	conf := jsondata.GetExpBottleConf()

	if conf == nil {
		return
	}
	if len(conf.KillMonSubTypes) == 0 {
		return
	}

	monConf := jsondata.GetMonsterConf(monId)
	if monConf == nil {
		return
	}

	if !pie.Uint32s(conf.KillMonSubTypes).Contains(monConf.SubType) {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiExpBottle).(*ExpBottleSys)
	if !ok || !sys.IsOpen() {
		return
	}

	state := sys.state()

	state.KilledWildBoosToday += count

	if state.ReceivedBottleNumToday+state.ReceivableBottleNum < sys.getMaxReceivableBottleNumDaily() {
		state.ReceivableBottleNum = state.KilledWildBoosToday/conf.NeedKillWildBossNumPerBottle - state.ReceivedBottleNumToday
	}

	sys.sendState()
}
