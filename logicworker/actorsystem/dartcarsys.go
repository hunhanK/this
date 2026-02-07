/**
 * @Author: zjj
 * @Date: 2023/11/27
 * @Desc: 护送活动(押镖)
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/actsweepmgr"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/operatelock"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/dartcarmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type DartCarSys struct {
	Base
}

func (sys *DartCarSys) getData() *pb3.DartCarTimes {
	if sys.GetBinaryData().DartCarTimes == nil {
		sys.GetBinaryData().DartCarTimes = &pb3.DartCarTimes{}
	}
	if sys.GetBinaryData().DartCarTimes.LastResetTimeAt == 0 {
		sys.GetBinaryData().DartCarTimes.LastResetTimeAt = time_util.NowSec()
	}
	return sys.GetBinaryData().DartCarTimes
}

func (sys *DartCarSys) OnOpen() {
	sys.resetDate()
	sys.s2cDartCarTimes()
}

func (sys *DartCarSys) OnAfterLogin() {
	sys.reLogin()
}

func (sys *DartCarSys) OnReconnect() {
	sys.reLogin()
}

func (sys *DartCarSys) resetDate() {
	data := sys.getData()
	nowSec := time_util.NowSec()
	if !time_util.IsSameDay(nowSec, data.LastResetTimeAt) {
		data.Times = 0
		data.KillTimes = 0
		data.LastResetTimeAt = nowSec
	}
}

func (sys *DartCarSys) reLogin() {
	sys.resetDate()
	carData := dartcarmgr.GDartCarMgrIns.GetDartCarData(sys.owner.GetId())
	var catType = 0
	if carData != nil {
		catType = int(carData.GetCarType())
	}
	sys.owner.SetExtraAttr(attrdef.DartCarType, attrdef.AttrValueAlias(catType))
	sys.owner.SetExtraAttr(attrdef.KillDartCarTimes, attrdef.AttrValueAlias(sys.getData().KillTimes))
	sys.s2cDartCarTimes()
	dartcarmgr.GDartCarMgrIns.SendDartCarData(sys.owner)
}

func (sys *DartCarSys) s2cDartCarTimes() {
	data := sys.getData()
	sys.SendProto3(52, 1, &pb3.S2C_52_1{
		Times:     data.GetTimes(),
		KillTimes: data.GetKillTimes(),
	})
}

func (sys *DartCarSys) onNewDay() {
	sys.resetDate()
	sys.s2cDartCarTimes()
}

func (sys *DartCarSys) GetDartCarTimes() uint32 {
	return sys.getData().Times
}

func (sys *DartCarSys) SetDartCarTimes(times uint32) {
	sys.getData().Times = times
}

func (sys *DartCarSys) c2sDartCarTimes(_ *base.Message) error {
	sys.s2cDartCarTimes()
	return nil
}

func (sys *DartCarSys) c2sAcceptDartCar(msg *base.Message) error {
	player := sys.owner
	if player.IsOperateLock(operatelock.AcceptDartCar) {
		return nil
	}

	mgr, err := jsondata.GetDartCarCommonConfMgr()
	if err != nil {
		return neterror.Wrap(err)
	}

	curType := player.GetExtraAttr(attrdef.DartCarType)
	if curType != 0 {
		return nil
	}

	carData := dartcarmgr.GDartCarMgrIns.GetDartCarData(player.GetId())
	if carData != nil {
		return nil
	}

	// 巅峰匹配中
	if player.GetExtraAttrU32(attrdef.MatchingTopFight) > 0 {
		return nil
	}

	var req pb3.C2S_52_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.Wrap(err)
	}

	// 普通拉镖次数检测
	if sys.GetDartCarTimes() >= mgr.Times {
		return nil
	}

	carType := req.CarType

	conf, err := jsondata.GetDartCarInfoConfByCtAndSt(carType)
	if err != nil {
		return neterror.Wrap(err)
	}

	// 校验跨服
	owner := sys.GetOwner()
	isCross := conf.IsCross
	if owner.GetActorProxy().GetProxyType().IsCrossSrv() && !isCross {
		return neterror.ParamsInvalidError("is cross, but dart car type is failed, req: %d,%d", carType, req.NpcId)
	}

	// 校验跨服押镖的功能是否开启
	refSysOpenId := conf.RefSysOpenId
	if refSysOpenId != 0 {
		if sys.GetOwner().GetSysMgr() == nil {
			return neterror.InternalError("not found sys mgr %d", refSysOpenId)
		}
		mgr, ok := sys.GetOwner().GetSysMgr().(*Mgr)
		if !ok {
			return neterror.InternalError("sys mgr convert failed %d", refSysOpenId)
		}
		if !mgr.canOpenSys(refSysOpenId, nil) {
			return neterror.InternalError("sys mgr convert failed %d", refSysOpenId)
		}
	}

	// 校验接镖的npcId
	if isCross {
		camp := fightworker.GetHostInfo(sys.GetOwner().GetActorProxy().GetProxyType()).Camp
		posInfo := mgr.CrossScenePosInfoMgr[fmt.Sprintf("%d", camp)]
		if posInfo == nil || posInfo.Npc != req.NpcId {
			player.SendTipMsg(tipmsgid.TpReceiveDartCarToLongDistance)
			return nil
		}
	}

	// 如果consume2有配, 则优先消耗,
	consumeConf, err := jsondata.GetDartCarConsumeConf(gshare.GetOpenServerDay(), conf.ConsumeList)
	if err != nil {
		return neterror.Wrap(err)
	}

	// 消耗
	var consume jsondata.ConsumeVec
	if len(consumeConf.Consume2) > 0 {
		consume = consumeConf.Consume2
	} else {
		consume = consumeConf.Consume
	}

	if mgr.Lv < sys.GetOwner().GetLevel() {
		var newConsumeVec jsondata.ConsumeVec
		for _, c := range consume {
			if c.Type == custom_id.ConsumeTypeMoney && c.Id == moneydef.BindDiamonds {
				newConsumeVec = append(newConsumeVec, &jsondata.Consume{
					Type:       c.Type,
					Id:         moneydef.Diamonds,
					Count:      c.Count,
					CanAutoBuy: c.CanAutoBuy,
					Job:        c.Job,
				})
			}
		}
		if len(newConsumeVec) > 0 {
			consume = newConsumeVec
		}
	}

	if len(consume) > 0 && !sys.GetOwner().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDartCarStartConsume}) {
		return neterror.ConsumeFailedError("start dart car failed")
	}

	logworker.LogPlayerBehavior(player, pb3.LogId_LogDartCarStart, &pb3.LogPlayerCounter{
		NumArgs: uint64(carType),
	})

	// 更新次数
	sys.SetDartCarTimes(sys.GetDartCarTimes() + 1)
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByCrossDartCar, sys.GetOwner().GetId())
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAcceptDartCarXTimes, 0, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAcceptXDartCarYTimes, carType, 1)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsAcceptXDartCarYTimes, carType, 1)

	// 告诉战斗服要开始押镖
	player.LockOperate(operatelock.AcceptDartCar, 3000)
	err = player.CallActorFunc(actorfuncid.BeginDartCar, &pb3.BeginDartCar{DartCarType: carType, OpenServerDay: gshare.GetOpenServerDay()})
	if err != nil {
		player.LogError("err:%v", err)
	}
	return nil
}

func (sys *DartCarSys) c2sCarPos(msg *base.Message) error {
	var req pb3.C2S_52_3
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.Wrap(err)
	}

	carData := dartcarmgr.GDartCarMgrIns.GetDartCarData(sys.owner.GetId())
	if carData == nil {
		return nil
	}

	st := &pb3.CommonSt{}
	st.U64Param = sys.owner.GetId()
	st.StrParam = req.Arg
	proxy := sys.owner.GetActorProxy()
	proxyType := proxy.GetProxyType()
	var err error
	if custom_id.IsDartCarTypeCross(carData.CarType) {
		err = proxy.SwitchFtype(base.SmallCrossServer)
		if err != nil {
			return neterror.Wrap(err)
		}
		err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncCrossFindCarPos, st)
	} else {
		err = proxy.SwitchFtype(base.LocalFightServer)
		if err != nil {
			return neterror.Wrap(err)
		}
		err = proxy.CallActorFunc(actorfuncid.FindCarPos, st)
	}
	if err != nil {
		sys.LogError("err:%v", err)
	}
	err = proxy.SwitchFtype(proxyType)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (sys *DartCarSys) SetKillDartCarTimes(times uint32) {
	sys.getData().KillTimes = times
}

func (sys *DartCarSys) enterFb(msg *base.Message) error {
	var req pb3.C2S_17_220
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	dartCarType := sys.GetOwner().GetExtraAttrU32(attrdef.DartCarType)
	if dartCarType > 0 {
		ctAndSt, err := jsondata.GetDartCarInfoConfByCtAndSt(dartCarType)
		if err != nil {
			return neterror.Wrap(err)
		}
		if !ctAndSt.IsCross {
			sys.LogWarn("已经接取本服镖车,不能进入跨服场景")
			return nil
		}
	}

	err = sys.GetOwner().EnterFightSrv(base.SmallCrossServer, fubendef.EnterCrossDartCar, &pb3.EnterFubenHdl{
		FbHdl:   0,
		SceneId: req.SceneId,
		PosX:    int32(req.X),
		PosY:    int32(req.Y),
		Param:   req.Param,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

// 劫镖成功
func onKillDartCarSucceed(player iface.IPlayer, buf []byte) {
	obj := player.GetSysObj(sysdef.SiDartCar)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*DartCarSys)
	newTimes := sys.getData().KillTimes + 1
	sys.SetKillDartCarTimes(newTimes)

	player.SetExtraAttr(attrdef.KillDartCarTimes, attrdef.AttrValueAlias(newTimes))
	player.SendProto3(52, 5, &pb3.S2C_52_5{
		KillTimes: newTimes,
	})
}

var singleDartCarController = &DartCarController{}

type DartCarController struct {
	actsweepmgr.Base
}

func (receiver *DartCarController) GetUseTimes(id uint32, playerId uint64) (useTimes uint32, ret bool) {
	player := manager.GetPlayerPtrById(playerId)
	sys := player.GetSysObj(sysdef.SiDartCar)
	if sys != nil && !sys.IsOpen() {
		return
	}
	s, ok := sys.(*DartCarSys)
	if !ok {
		return
	}
	return s.GetDartCarTimes(), true
}

func (receiver *DartCarController) AddUseTimes(id, times uint32, playerId uint64) {
	player := manager.GetPlayerPtrById(playerId)
	sys := player.GetSysObj(sysdef.SiDartCar)
	if sys != nil && !sys.IsOpen() {
		return
	}
	s, ok := sys.(*DartCarSys)
	if !ok {
		return
	}
	s.SetDartCarTimes(s.GetDartCarTimes() + times)
}

func init() {
	RegisterSysClass(sysdef.SiDartCar, func() iface.ISystem {
		return &DartCarSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys := player.GetSysObj(sysdef.SiDartCar)
		if sys != nil && !sys.IsOpen() {
			return
		}
		s, ok := sys.(*DartCarSys)
		if !ok {
			return
		}
		s.onNewDay()
	})

	net.RegisterSysProtoV2(52, 1, sysdef.SiDartCar, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DartCarSys).c2sDartCarTimes
	})
	net.RegisterSysProtoV2(52, 2, sysdef.SiDartCar, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DartCarSys).c2sAcceptDartCar
	})
	net.RegisterSysProtoV2(52, 3, sysdef.SiDartCar, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DartCarSys).c2sCarPos
	})
	net.RegisterSysProtoV2(17, 220, sysdef.SiDartCar, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DartCarSys).enterFb
	})

	engine.RegisterActorCallFunc(playerfuncid.KillDartCarSucceed, onKillDartCarSucceed)

	actsweepmgr.Reg(actsweepmgr.ActSweepByDartCar, singleDartCarController)
	actsweepmgr.Reg(actsweepmgr.ActSweepByCrossDartCar, singleDartCarController)

	gmevent.Register("DartCarSys.reset", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiDartCar)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*DartCarSys)
		sys.getData().Times = 0
		sys.getData().KillTimes = 0
		sys.getData().LastResetTimeAt = time_util.NowSec()
		sys.owner.SetExtraAttr(attrdef.KillDartCarTimes, attrdef.AttrValueAlias(0))
		sys.s2cDartCarTimes()
		return true
	}, 1)
}
