/**
 * @Author: lzp
 * @Date: 2023/11/22
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type SuitBossSys struct {
	Base
	data *pb3.SuitBossPlayerInfo
}

func (sys *SuitBossSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *SuitBossSys) OnOpen() {
	sys.init()
	sys.pushPubInfo()
	sys.pushPlayerInfo()
}

func (sys *SuitBossSys) OnLogin() {
	sys.pushPubInfo()
	sys.pushPlayerInfo()
}

func (sys *SuitBossSys) OnReconnect() {
	sys.pushPubInfo()
	sys.pushPlayerInfo()
}

func (sys *SuitBossSys) onNewDay() {
	sys.data.UsedTimes = 0
	sys.pushPlayerInfo()
}

func (sys *SuitBossSys) init() {
	data := sys.GetBinaryData().SuitBossInfo
	if data == nil {
		data = &pb3.SuitBossPlayerInfo{}
		sys.GetBinaryData().SuitBossInfo = data
	}

	sys.data = data
}

// 公共信息战斗服发送
func (sys *SuitBossSys) pushPubInfo() {
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSuitBossInfoReq, &pb3.SuitBossInfoReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: sys.GetOwner().GetId(),
	})
}

// 玩家自己数据逻辑服发送
func (sys *SuitBossSys) pushPlayerInfo() {
	sys.SendProto3(168, 5, &pb3.S2C_168_5{
		Info: sys.data,
	})
}

// 获取剩余次数
func (sys *SuitBossSys) GetLeftUsedTimes() uint32 {
	privilegeAddTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumSuitBossFreeTimes)
	dailyTimes := jsondata.GetSuitBossCommonConf().DailyRewardTimes
	addTimes := sys.GetOwner().GetFightAttr(attrdef.SuitBossTimesAdd)
	leftTimes := utils.MaxUInt32(0, uint32(dailyTimes)+uint32(privilegeAddTimes)+uint32(addTimes)-sys.data.UsedTimes)
	return leftTimes
}

// C2S
func (sys *SuitBossSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_168_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	conf := jsondata.GetSuitBossCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("suitBoss common conf is nil")
	}

	layerConf := jsondata.GetSuitBossLayerConf(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("suitBoss layer conf is nil sceneId %d", req.SceneId)
	}

	if sys.GetOwner().GetCircle() < layerConf.BoundaryLevel {
		return neterror.ParamsInvalidError("suitBoss boundary level limit current %d needed %d", sys.GetOwner().GetCircle(), layerConf.BoundaryLevel)
	}

	if sys.GetOwner().GetLevel() < layerConf.PlayerLv {
		return neterror.ParamsInvalidError("suitBoss player level limit current %d needed %d", sys.GetOwner().GetLevel(), layerConf.PlayerLv)
	}

	if sys.GetOwner().GetSceneId() == req.SceneId {
		return neterror.ConfNotFoundError("suitBoss player is in fuBen %d", req.SceneId)
	}

	if sys.GetOwner().InDartCar() {
		sys.GetOwner().SendTipMsg(tipmsgid.Tpindartcar)
		return nil
	}

	times := req.MergeTimes
	if times == 0 {
		times = 1
	}
	if sys.GetLeftUsedTimes() <= 0 || sys.GetLeftUsedTimes() < times {
		return neterror.ConfNotFoundError("suitBoss enter times error %d", req.SceneId)
	}

	var totalConsumes jsondata.ConsumeVec
	for i := uint32(1); i <= times; i++ {
		next := sys.data.UsedTimes + i
		consumes := jsondata.GetSuitBossConsumeConf(req.SceneId, next)
		if consumes == nil {
			return neterror.ParamsInvalidError("suit boss consume config err")
		}
		totalConsumes = append(totalConsumes, consumes...)
	}

	if !sys.GetOwner().CheckConsumeByConf(totalConsumes, false, pb3.LogId_LogCrossFairyBossEnterConsume) {
		return neterror.ConsumeFailedError("consume failed")
	}

	err := sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterSuitBoss,
		&pb3.AttackSuitBoss{
			SceneId: req.SceneId,
		},
		&argsdef.ConsumesSt{
			Consumes: totalConsumes,
			LogId:    pb3.LogId_LogSuitBossEnterConsume,
		})
	if err != nil {
		return neterror.InternalError("suitBoss enter fight srv failed err: %s", err)
	}

	err = sys.GetOwner().MergeTimesChallengeBoss(base.LocalFightServer, fubendef.EnterSuitBoss, times)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	// 加次数
	sys.data.UsedTimes += times
	sys.pushPlayerInfo()
	sys.owner.TriggerQuestEvent(custom_id.QttEnterSuitBossTimes, 0, 1)
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogSuitBossEnterConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(sys.data.UsedTimes),
	})
	return nil

}

func (sys *SuitBossSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_168_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	lConf := jsondata.GetSuitBossLayerConf(req.SceneId)
	if lConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", req.SceneId)
	}

	if sys.data.Layers == nil {
		sys.data.Layers = make(map[uint32]*pb3.SuitBossLayerPlayerInfo)
	}

	layerInfo, ok := sys.data.Layers[req.SceneId]
	if !ok {
		layerInfo = &pb3.SuitBossLayerPlayerInfo{
			Monster: make(map[uint32]*pb3.SuitBossMonsterPlayer),
		}
		sys.data.Layers[req.SceneId] = layerInfo
	}

	monInfo, ok := layerInfo.Monster[req.MonId]
	if !ok {
		monInfo = &pb3.SuitBossMonsterPlayer{
			MonsterId: req.MonId,
		}
		layerInfo.Monster[req.MonId] = monInfo
	}

	monInfo.NeedFollow = req.NeedFollow

	sys.pushPlayerInfo()

	return nil
}

func onSuitBossNewDay(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiSuitBoss).(*SuitBossSys); ok && sys.IsOpen() {
		sys.onNewDay()
	}
}

// F2G
func onF2GSuitBossReliveInfo(buf []byte) {
	var req pb3.S2C_168_7
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onF2GSuitBossReliveInfo Unmarshal failed err: %s", err)
		return
	}
	engine.Broadcast(chatdef.CIWorld, 0, 168, 7, &req, 0)
}

func onF2GSuitBossDeadInfo(buf []byte) {
	var req pb3.S2C_168_7
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onF2GSuitBossDeadInfo Unmarshal failed err: %s", err)
		return
	}
	engine.Broadcast(chatdef.CIWorld, 0, 168, 7, &req, 0)
}

func onF2GSuitBossInfoRes(buf []byte) {
	var req pb3.SuitBossInfoRes
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onF2GSuitBossInfoRes Unmarshal failed err: %s", err)
		return
	}

	player := manager.GetPlayerPtrById(req.ActorId)
	if player == nil {
		return
	}

	player.SendProto3(168, 6, &pb3.S2C_168_6{
		Info: req.PubInfo,
	})
}

func init() {
	RegisterSysClass(sysdef.SiSuitBoss, func() iface.ISystem {
		return &SuitBossSys{}
	})

	// c2s
	net.RegisterSysProto(168, 1, sysdef.SiSuitBoss, (*SuitBossSys).c2sEnter)
	net.RegisterSysProto(168, 2, sysdef.SiSuitBoss, (*SuitBossSys).c2sFollowBoss)

	// f2g
	engine.RegisterSysCall(sysfuncid.F2GSuitBossReliveInfo, onF2GSuitBossReliveInfo)
	engine.RegisterSysCall(sysfuncid.F2GSuitBossDeadInfo, onF2GSuitBossDeadInfo)
	engine.RegisterSysCall(sysfuncid.F2GSuitBossInfoRes, onF2GSuitBossInfoRes)

	event.RegActorEvent(custom_id.AeNewDay, onSuitBossNewDay)
	event.RegActorEvent(custom_id.AeKillMon, onKillSuitBoss)
}

func onKillSuitBoss(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}

	monId, ok := args[0].(uint32)
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

	sys, ok := actor.GetSysObj(sysdef.SiSuitBoss).(*SuitBossSys)
	if !ok || !sys.IsOpen() {
		return
	}

	layerConf := jsondata.GetSuitBossLayerConf(sceneId)
	if layerConf == nil {
		return
	}

	bossMgr := layerConf.Boss
	if bossMgr == nil {
		return
	}

	_, ok = bossMgr[monId]
	if !ok {
		return
	}
	actor.TriggerQuestEvent(custom_id.QttKillLayerSuitBoss, layerConf.LayerId, int64(count))
}
