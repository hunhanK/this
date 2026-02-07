/**
 * @Author: HeXinLi
 * @Desc: 记录活动状态
 * @Date: 2021/9/26 10:12
 */

package activity

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/activitydef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"sync"

	"github.com/gzjjyz/logger"
)

type actStatusFunc func(actId uint32, status *pb3.ActStatusInfo)

var (
	once      sync.Once
	singleton *actMgr
)

type (
	actMgr struct {
		actStatus        map[uint32]*pb3.ActStatusInfo
		actStatusFuncMap map[uint32]actStatusFunc
	}
)

func ActMgr() *actMgr {
	once.Do(func() {
		singleton = &actMgr{}
		singleton.actStatus = make(map[uint32]*pb3.ActStatusInfo)
		singleton.actStatusFuncMap = make(map[uint32]actStatusFunc)
	})

	return singleton
}

func GetActStartTime(actId uint32) uint32 {
	if act, exist := ActMgr().actStatus[actId]; exist {
		return act.StartTime
	}
	return 0
}

func GetActStatus(actId uint32) uint32 {
	if act, exist := ActMgr().actStatus[actId]; exist {
		return act.Status
	}
	return activitydef.ActEnd
}

func registerActStatusEvent(actId uint32, fn actStatusFunc) {
	_, exist := ActMgr().actStatusFuncMap[actId]
	if exist {
		return
	}
	ActMgr().actStatusFuncMap[actId] = fn
}

// 同步活动状态
func onSyncActStatus(buf []byte) {
	msg := &pb3.ActStatusInfo{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	syncActStatus(msg)
}

func onC2GSyncActStatus(buf []byte) {
	st := &pb3.ActStatusInfo{}
	if err := pb3.Unmarshal(buf, st); nil != err {
		logger.LogError("onC2GSyncActStatus error:%v", err)
		return
	}
	syncActStatus(st)
}

func syncActStatus(msg *pb3.ActStatusInfo) {
	actId := msg.ActId

	conf := jsondata.GetActivityConf(actId)
	if nil == conf {
		return
	}

	ActMgr().actStatus[actId] = msg
	if fn, ok := ActMgr().actStatusFuncMap[actId]; ok {
		fn(actId, msg)
	}

	engine.Broadcast(chatdef.CIWorld, 0, 31, 1, &pb3.S2C_31_1{Info: msg}, 0)
	if gshare.GetOpenServerDay() < conf.BroadcastDay {
		return
	}

	switch msg.Status {
	case activitydef.ActPrepare:
		if conf.ReadyBroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.ReadyBroadcastId)
		}
	case activitydef.ActStart:
		if conf.StartBroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.StartBroadcastId)
		}
	case activitydef.ActEnd:
		if conf.EndBroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.EndBroadcastId)
		}
	}
}

func onC2GCloseAct(buf []byte) {
	var st pb3.CloseActivity
	if err := pb3.Unmarshal(buf, &st); nil != err {
		logger.LogError("onC2GCloseAct error:%v", err)
		return
	}

	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCloseAct, &st)
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func c2sEnterActivity(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_31_2
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return neterror.ParamsInvalidError("c2sEnterActivity error:%v", err)
	}

	actId := req.GetActId()
	conf := jsondata.GetActivityConf(actId)
	if nil == conf {
		return neterror.ParamsInvalidError("c2sEnterActivity error:conf is nil")
	}

	status := GetActStatus(actId)
	if status != activitydef.ActStart {
		return neterror.ParamsInvalidError("c2sEnterActivity error:activity is not start")
	}

	if player.GetExtraAttrU32(attrdef.Level) < conf.EnterLevel {
		player.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	if player.GetExtraAttrU32(attrdef.Circle) < conf.EnterCircle {
		player.SendTipMsg(tipmsgid.CircleNotEnough)
		return nil
	}

	enterMsg := &pb3.EnterActivity{
		ActId: actId,
	}

	if req.Arg != nil {
		enterMsg.Arg = &pb3.ExtArg{
			SceneId: req.Arg.SceneId,
			BossId:  req.Arg.BossId,
			Hdl:     req.Arg.Hdl,
		}
	}

	srvType, ok := base.ActivityConfigCrossType_MapToServerType[conf.CrossType]
	if !ok {
		return neterror.ParamsInvalidError("c2sEnterActivity error:crossType is invalid")
	}

	isActCross := gshare.GetStaticVar().IsActivityCross == 1
	// TODO: 由于传奇的跨服体系和仙侠的不一样，这里需要修改，要和策划讨论一下
	// 开启跨服后变成跨服活动
	if conf.ChangeCross && isActCross {
		client := fightworker.GetFightClient(base.SmallCrossServer)
		if client != nil {
			return player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterActivity, enterMsg)
		}
		return player.EnterFightSrv(base.LocalFightServer, fubendef.EnterActivity, enterMsg)
	}

	return player.EnterFightSrv(srvType, fubendef.EnterActivity, enterMsg)
}

func RegisterActivityCross() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CRegisterActivityCross, &pb3.RegisterActivityCross{
		PlatformId: engine.GetPfId(),
		SrvId:      engine.GetServerId(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func reqActivityInfo(player iface.IPlayer, _ ...interface{}) {
	err := c2sGetActList(player, nil)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	return
}

func SetActivityCross() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSetActivityCross, &pb3.CommonSt{
		BParam: gshare.GetStaticVar().IsActivityCross == 1,
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func SetCrossAct(flag uint32) {
	global := gshare.GetStaticVar()
	global.IsActivityCross = flag

	SetActivityCross()
	RegisterActivityCross()
}

func CloseCrossAct() {
	for id, _ := range ActMgr().actStatus {
		line := jsondata.GetActivityConf(id)
		if line.ChangeCross || line.CrossType == uint32(base.SmallCrossServer) || line.CrossType == uint32(base.MediumCrossServer) {
			engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCloseAct, &pb3.CommonSt{
				U32Param: id,
			})
		}
	}
}

func CheckIsCrossAct(actId uint32) bool {
	conf := jsondata.GetActivityConf(actId)
	if nil == conf {
		return false
	}
	isActCross := gshare.GetStaticVar().IsActivityCross == 1
	return conf.ChangeCross && isActCross
}

func init() {
	engine.RegisterSysCall(sysfuncid.FSyncActStatus, onSyncActStatus)
	engine.RegisterSysCall(sysfuncid.C2GCloseAct, onC2GCloseAct)
	engine.RegisterSysCall(sysfuncid.C2GSyncCrossActStatus, onC2GSyncActStatus)
	engine.GetActStatusFunc = GetActStatus

	event.RegActorEvent(custom_id.AeLogin, reqActivityInfo)
	event.RegActorEvent(custom_id.AeReconnect, reqActivityInfo)

	net.RegisterProto(31, 2, c2sEnterActivity)

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		RegisterActivityCross()
	})

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		SetActivityCross()
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		SetCrossAct(1)
	})

	event.RegSysEvent(custom_id.SeCrossDisconnect, func(args ...interface{}) {
		SetCrossAct(0)
		CloseCrossAct()
	})
}
