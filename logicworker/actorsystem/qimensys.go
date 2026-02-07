package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/fuben/qimenmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type QiMenSys struct {
	Base
	timer *time_util.Timer
}

func (sys *QiMenSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.QiMen {
		binary.QiMen = &pb3.QiMen{}
	}
}

func (sys *QiMenSys) GetData() *pb3.QiMen {
	return sys.GetBinaryData().GetQiMen()
}

func (sys *QiMenSys) RecoverEnergy() {
	if nil != sys.timer {
		sys.timer.Stop()
	}

	arr := jsondata.GlobalU32Vec("qimenRecover")
	second, energy := arr[0], arr[1]
	if second == 0 {
		sys.LogError("energy recover time cant use zero")
		return
	}

	data := sys.GetData()
	nowSec := time_util.NowSec()

	if data.NextRecoverTime == 0 {
		data.NextRecoverTime = nowSec + second
	}

	var addVal uint32
	if data.NextRecoverTime <= nowSec {
		round := (nowSec - data.NextRecoverTime) / second
		addVal = (round + 1) * energy
		data.NextRecoverTime += (round + 1) * second
	}

	limit := jsondata.GlobalUint("musclemax")
	if data.Energy < limit {
		addVal = utils.Ternary((data.Energy+addVal) > limit, limit-data.Energy, addVal).(uint32)
		sys.AddEnergy(addVal)
	}

	sys.timer = sys.owner.SetTimeout(time.Duration(data.NextRecoverTime-nowSec)*time.Second, func() {
		sys.RecoverEnergy()
	})
}

func (sys *QiMenSys) notifyEnergy() {
	qimen := sys.GetData()
	sys.SendProto3(17, 52, &pb3.S2C_17_52{Energy: qimen.Energy})
}

// 系统开启，设置初始体力值
func (sys *QiMenSys) OnOpen() {
	energy := jsondata.GlobalUint("initialmuscle")
	sys.AddEnergy(energy)
	sys.RecoverEnergy()
	sys.s2cFbInfo()
}

func (sys *QiMenSys) OnAfterLogin() {
	sys.RecoverEnergy()
	sys.s2cFbInfo()
	sys.notifyEnergy()
}

func (sys *QiMenSys) OnLogout() {
	if nil != sys.timer {
		sys.timer.Stop()
		sys.timer = nil
	}
}

func (sys *QiMenSys) OnReconnect() {
	sys.s2cFbInfo()
	sys.notifyEnergy()
}

// 请求进入副本
func (sys *QiMenSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_17_50
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	layer := req.GetLayer()
	typeConf := qimenmgr.GetCurTypeConf()
	layerConf := typeConf.LayerConf[layer]
	if nil == layerConf {
		return neterror.ParamsInvalidError("qimenmgr not found failed layer(%d)", layer)
	}
	checker := new(manager.CondChecker)
	fbId := qimenmgr.GetFbId()
	cond := layerConf.Cond
	successed, err := checker.Check(sys.GetOwner(), layerConf.Cond.Expr, map[string]uint32{"Level": cond.Level, cond.Ref.Key: cond.Ref.Value}, fbId)
	if err != nil {
		return neterror.ParamsInvalidError("%s", err)
	}
	if !successed {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterQiMen, &pb3.EnterQiMen{
		SceneId: layerConf.SceneId,
	})
	return nil
}

func (sys *QiMenSys) s2cFbInfo() {
	fbInfo := qimenmgr.GetQiMenInfo()
	rsp := &pb3.S2C_17_51{}
	if !fbInfo.Lock {
		rsp.FbType = fbInfo.QiMenType
		rsp.Loop = fbInfo.QiMenLoop[fbInfo.QiMenType]
		rsp.EndTimeStamp = fbInfo.QiMenEndTimeStamp
	}
	sys.SendProto3(17, 51, rsp)
}

func (sys *QiMenSys) c2sFbInfo(msg *base.Message) {
	sys.s2cFbInfo()
}

func (sys *QiMenSys) syncEnergyToFight() {
	msg := &pb3.CommonSt{U32Param: sys.GetData().Energy}
	err := sys.owner.CallActorFunc(actorfuncid.G2FSyncQiMenEnergy, msg)
	if err != nil {
		sys.LogError("err:%v", err)
		return
	}
}

func (sys *QiMenSys) AddEnergy(addEnergy uint32) bool {
	qimen := sys.GetData()
	oldEnergy := qimen.Energy
	limit := jsondata.GlobalUint("musclemax")
	if oldEnergy+addEnergy >= limit {
		addEnergy = limit - qimen.Energy
		qimen.Energy = limit
	} else {
		qimen.Energy += addEnergy
	}
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldEnergy": oldEnergy,
		"newEnergy": qimen.Energy,
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogQiMenAddEnergy, &pb3.LogPlayerCounter{
		NumArgs: uint64(addEnergy),
		StrArgs: string(logArg)})
	sys.notifyEnergy()
	sys.syncEnergyToFight()
	return true
}

func (sys *QiMenSys) DecEnergy(subEnergy uint32) bool {
	qimen := sys.GetData()
	oldEnergy := qimen.Energy
	if qimen.Energy >= subEnergy {
		qimen.Energy -= subEnergy
	} else {
		sys.LogError("player:%d,energy sub exceed old:energy:%d,cos:%d", sys.owner.GetId(), qimen.Energy, subEnergy)
		subEnergy = qimen.Energy
		qimen.Energy = 0
	}
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldEnergy": oldEnergy,
		"newEnergy": qimen.Energy,
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogQiMenSubEnergy, &pb3.LogPlayerCounter{
		NumArgs: uint64(subEnergy),
		StrArgs: string(logArg),
	})
	sys.notifyEnergy()
	sys.syncEnergyToFight()
	sys.owner.TriggerQuestEvent(custom_id.QttQiMenConsumeEnergyHistory, 0, int64(subEnergy))
	return true
}

func (sys *QiMenSys) c2sReqBossList(msg *base.Message) {
	qimenmgr.SendQiMenAllBossList(sys.owner, 0)
}

// 使用奇门体力丹: 增加八方奇门副本体力
func useQiMenEnergy(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	qmSys, ok := player.GetSysObj(sysdef.SiQiMen).(*QiMenSys)
	if !ok || !qmSys.IsOpen() {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return false, false, 0
	}
	energy := qmSys.GetData().GetEnergy()
	allowUse := param.Count
	limit := jsondata.GlobalUint("musclemax")
	if limit <= energy {
		return false, false, 0
	}
	if param.Count > 1 {
		allowUse = utils.MinInt64(int64((limit-energy)/conf.Param[0]), param.Count)
	}
	ok = qmSys.AddEnergy(uint32(allowUse) * conf.Param[0])
	if !ok {
		return false, false, 0
	}
	return true, true, allowUse
}

// 扣除体力
func OnDecEnergy(actor iface.IPlayer, buf []byte) {
	msg := &pb3.QiMenDecEnergy{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		actor.LogError("【奇门八方】同步体力出错 %v", err)
		return
	}

	sys, ok := actor.GetSysObj(sysdef.SiQiMen).(*QiMenSys)
	if ok {
		sys.DecEnergy(msg.DecEnergy)
	}
}

func OnSendQiMenBossStatus(actor iface.IPlayer, buf []byte) {
	msg := &pb3.SendQiMenBossStatus{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		actor.LogError("重连下发boss状态出错 %v", err)
		return
	}
	sceneId := msg.GetSceneId()
	layerConf := jsondata.GetQiMenLayerConf(qimenmgr.GetQiMenInfo().QiMenType, sceneId)

	actor.SendProto3(17, 55, &pb3.S2C_17_55{Layer: layerConf.Layer})
	qimenmgr.SendQiMenAllBossList(actor, layerConf.Layer)
}

func c2sEnterCrossFb(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_17_180
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	layerConf, conf := jsondata.GetCrossQiMenConfByScene(req.GetId())
	if nil == conf {
		return neterror.ConfNotFoundError("no cross qimen single conf(%d)", req.GetId())
	}

	for _, cond := range layerConf.Cond {
		if !CheckReach(player, cond.Type, cond.Value) {
			player.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
			return nil
		}
	}

	if !CheckReach(player, conf.CondType, conf.CondVal) {
		player.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterCrossQiMen, &pb3.CommonSt{
		U32Param:  layerConf.Id,
		U32Param2: req.GetId(),
	})
	return nil
}

func (sys *QiMenSys) c2sEnterResidentFb(msg *base.Message) error {
	var req pb3.C2S_17_71
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	fbConf := jsondata.GetResidentQiMenFbConf(req.FbType)
	if nil == fbConf {
		return neterror.ConfNotFoundError("resident qimen fbConf(%d) not exist", req.FbType)
	}

	serverDay := gshare.GetOpenServerDay()
	if serverDay < fbConf.OpenDay {
		return neterror.ConfNotFoundError("resident qimen not open")
	}

	layerConf, ok := fbConf.Layer[req.Layer]
	if !ok {
		return neterror.ConfNotFoundError("resident qimen layerConf not exist")
	}

	checker := new(manager.CondChecker)
	if success, err := checker.Check(sys.owner, layerConf.Cond.Expr, map[string]uint32{"Level": layerConf.Cond.Level, layerConf.Cond.Ref.Key: layerConf.Cond.Ref.Value}); !success {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return err
	}
	return sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterResidentQiMen, &pb3.CommonSt{
		U32Param: layerConf.SceneId,
	})
}

func (sys *QiMenSys) BossSweepChecker(monId uint32) bool {
	energy, ok := jsondata.GetQiMenBossEnergy(monId)
	if !ok {
		return false
	}

	if sys.GetData().Energy < energy {
		return false
	}

	return true
}

func (sys *QiMenSys) BossSweepSettle(monId, sceneId uint32, rewards jsondata.StdRewardVec) bool {
	energy, ok := jsondata.GetQiMenBossEnergy(monId)
	if !ok {
		return false
	}

	if sys.GetData().Energy < energy {
		return false
	}

	sys.DecEnergy(energy)

	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiMenBossSweepAwards})

	sys.owner.SendShowRewardsPop(rewards)

	return true
}

func (sys *QiMenSys) onNewDay() {
	sys.AddEnergy(jsondata.GlobalUint("qimenDailyRecover"))
}

func init() {
	RegisterSysClass(sysdef.SiQiMen, func() iface.ISystem {
		return &QiMenSys{}
	})

	net.RegisterSysProto(17, 50, sysdef.SiQiMen, (*QiMenSys).c2sEnterFb)
	net.RegisterSysProto(17, 51, sysdef.SiQiMen, (*QiMenSys).c2sFbInfo)
	net.RegisterSysProto(17, 53, sysdef.SiQiMen, (*QiMenSys).c2sReqBossList)
	net.RegisterSysProto(17, 71, sysdef.SiQiMen, (*QiMenSys).c2sEnterResidentFb)

	engine.RegisterActorCallFunc(playerfuncid.DeQiMenEnergy, OnDecEnergy)
	engine.RegisterActorCallFunc(playerfuncid.SendQiMenBossStatus, OnSendQiMenBossStatus)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemQiMenEnergy, useQiMenEnergy)

	net.RegisterProto(17, 180, c2sEnterCrossFb)
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiQiMen).(*QiMenSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})
}
