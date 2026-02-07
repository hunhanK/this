/**
 * @Author: lzp
 * @Date: 2023/12/11
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type WeddingBossSys struct {
	Base
}

func (sys *WeddingBossSys) OnOpen() {
	sys.S2CInfo()
}

func (sys *WeddingBossSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *WeddingBossSys) OnNewDay() {
	data := sys.GetData()
	for _, v := range data.Layers {
		v.UsedTimes = 0
	}
	sys.S2CInfo()
}

func (sys *WeddingBossSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *WeddingBossSys) S2CInfo() {
	pbMsg := &pb3.S2C_53_52{
		Info: sys.GetData(),
	}
	sys.SendProto3(53, 52, pbMsg)
}

func (sys *WeddingBossSys) PushLayerInfo(sceneId uint32) {
	layers := make(map[uint32]*pb3.WeddingBossLayerPlayerInfo)
	layers[sceneId] = sys.GetLayerData(sceneId)
	pbMsg := &pb3.S2C_53_53{
		Info: &pb3.WeddingBossPlayerInfo{Layers: layers},
	}
	sys.SendProto3(53, 53, pbMsg)
}

func (sys *WeddingBossSys) GetData() *pb3.WeddingBossPlayerInfo {
	data := sys.GetBinaryData().WeddingBossInfo
	if data == nil {
		data = &pb3.WeddingBossPlayerInfo{
			Layers: map[uint32]*pb3.WeddingBossLayerPlayerInfo{},
		}
		sys.GetBinaryData().WeddingBossInfo = data
	}

	if data.Layers == nil {
		data.Layers = map[uint32]*pb3.WeddingBossLayerPlayerInfo{}
	}
	return data
}

func (sys *WeddingBossSys) GetLayerData(sceneId uint32) *pb3.WeddingBossLayerPlayerInfo {
	data := sys.GetData()
	if data.Layers[sceneId] == nil {
		data.Layers[sceneId] = &pb3.WeddingBossLayerPlayerInfo{
			UsedTimes: 0,
			Monsters:  make(map[uint32]*pb3.WeddingBossMonsterPlayer),
		}
	}
	if data.Layers[sceneId].Monsters == nil {
		data.Layers[sceneId].Monsters = make(map[uint32]*pb3.WeddingBossMonsterPlayer)
	}
	return data.Layers[sceneId]
}

func (sys *WeddingBossSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_53_50
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}
	conf := jsondata.GetWeddingBossCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("wedding boss common conf is nil ")
	}

	layerConf := jsondata.GetWeddingBossLayerConf(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("wedding boss layer conf is nil sceneId %d", req.SceneId)
	}

	if !sys.CheckWeddingRingLv(req.SceneId) {
		return neterror.ParamsInvalidError("ring lv is limit")
	}

	if sys.GetOwner().GetFbId() == jsondata.GetWeddingBossCommonConf().FbId {
		return neterror.ParamsInvalidError("wedding boss player is in fuBen %d", req.SceneId)
	}

	if sys.GetOwner().GetSceneId() == req.SceneId {
		return neterror.ParamsInvalidError("wedding boss player is in fuBen %d", req.SceneId)
	}

	if !sys.CheckSceneBoss(req.SceneId) {
		return neterror.ConfNotFoundError("wedding boss=%d enter error %d", layerConf.BossType, req.SceneId)
	}

	consumes := jsondata.GetWeddingBossConsumeConf(req.SceneId)
	if consumes == nil {
		return neterror.ConsumeFailedError("wedding boss enter consume failed")
	}

	// 单人boss
	err := sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterWeddingBoss,
		&pb3.AttackWeddingBoss{
			SceneId: req.SceneId,
		},
		&argsdef.ConsumesSt{
			Consumes: consumes,
			LogId:    pb3.LogId_LogWeddingBossEnterConsume,
		})
	if err != nil {
		return neterror.InternalError("wedding boss enter fight srv failed err: %s", err)
	}

	return nil
}

func (sys *WeddingBossSys) c2sEnterReq(msg *base.Message) error {
	var req pb3.C2S_53_54
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	conf := jsondata.GetWeddingBossCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("wedding boss common conf is nil ")
	}

	layerConf := jsondata.GetWeddingBossLayerConf(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("wedding boss layer conf is nil sceneId %d", req.SceneId)
	}

	if !sys.CheckWeddingRingLv(req.SceneId) {
		return neterror.ParamsInvalidError("ring lv is limit")
	}

	if sys.GetOwner().GetSceneId() == req.SceneId {
		return neterror.ParamsInvalidError("wedding boss player is in fuBen %d", req.SceneId)
	}

	if sys.GetLeftUsedTimes(req.SceneId) <= 0 {
		return neterror.ConfNotFoundError("wedding boss=%d enter times error %d", layerConf.BossType, req.SceneId)
	}

	if !sys.CheckSceneBoss(req.SceneId) {
		return neterror.ConfNotFoundError("wedding boss=%d enter error %d", layerConf.BossType, req.SceneId)
	}

	if fbConf := jsondata.GetFbConf(sys.GetOwner().GetFbId()); fbConf != nil {
		if !utils.SliceContainsUint32(fbConf.CanEnterHdlIds, 0) {
			sys.owner.SendTipMsg(tipmsgid.TpCantEnterOtherFb)
			return nil
		}
	}

	// 双人boss
	mData := sys.GetBinaryData().MarryData
	if mData == nil || !friendmgr.IsExistStatus(mData.CommonId, custom_id.FsMarry) {
		return neterror.ParamsInvalidError("wedding boss sceneId=%d marry limit", req.SceneId)
	}

	mPlayer := manager.GetPlayerPtrById(mData.MarryId)
	if mPlayer == nil {
		return neterror.ParamsInvalidError("wedding boss sceneId=%d mate offline", req.SceneId)
	}

	if mSys, ok := mPlayer.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && mSys.IsOpen() {
		if mSys.GetLeftUsedTimes(req.SceneId) <= 0 {
			sys.owner.SendTipMsg(tipmsgid.WeddingBossTimesLimit)
			return nil
		}
	}

	fbId := mPlayer.GetFbId()
	if fbConf := jsondata.GetFbConf(fbId); fbConf != nil {
		if !utils.SliceContainsUint32(fbConf.CanEnterHdlIds, 0) {
			sys.owner.SendTipMsg(tipmsgid.WeddingBossMateInOtherFb)
			return nil
		}
	}

	mPlayer.SendProto3(53, 54, &pb3.S2C_53_54{
		SceneId: req.SceneId,
	})

	return nil
}

func (sys *WeddingBossSys) c2sEnterResp(msg *base.Message) error {
	var req pb3.C2S_53_55
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	layerConf := jsondata.GetWeddingBossLayerConf(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("wedding boss layer conf is nil sceneId %d", req.SceneId)
	}

	if !sys.CheckWeddingRingLv(req.SceneId) {
		return neterror.ParamsInvalidError("ring lv is limit")
	}

	if sys.GetLeftUsedTimes(req.SceneId) <= 0 {
		return neterror.ConfNotFoundError("wedding boss mate enter times error %d", req.SceneId)
	}

	if !sys.CheckSceneBoss(req.SceneId) {
		return neterror.ConfNotFoundError("wedding boss=%d enter error %d", layerConf.BossType, req.SceneId)
	}

	if fbConf := jsondata.GetFbConf(sys.GetOwner().GetFbId()); fbConf != nil {
		if !utils.SliceContainsUint32(fbConf.CanEnterHdlIds, 0) {
			sys.owner.SendTipMsg(tipmsgid.TpCantEnterOtherFb)
			return nil
		}
	}

	mData := sys.GetBinaryData().MarryData
	if mData == nil || !friendmgr.IsExistStatus(mData.CommonId, custom_id.FsMarry) {
		return neterror.ParamsInvalidError("wedding boss sceneId=%d marry limit", req.SceneId)
	}

	mPlayer := manager.GetPlayerPtrById(mData.MarryId)
	if mPlayer == nil {
		return neterror.ParamsInvalidError("wedding boss sceneId=%d mate offline", req.SceneId)
	}

	if mSys, ok := mPlayer.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && mSys.IsOpen() {
		if mSys.GetLeftUsedTimes(req.SceneId) <= 0 {
			sys.owner.SendTipMsg(tipmsgid.WeddingBossTimesLimit)
			return nil
		}
	}

	if fbConf := jsondata.GetFbConf(mPlayer.GetFbId()); fbConf != nil {
		if !utils.SliceContainsUint32(fbConf.CanEnterHdlIds, 0) {
			sys.owner.SendTipMsg(tipmsgid.WeddingBossMateInOtherFb)
			return nil
		}
	}

	// 将双方拉进副本
	err := sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterWeddingBoss, &pb3.AttackWeddingBoss{
		SceneId: req.SceneId,
		MarryId: mData.MarryId,
	})
	if err != nil {
		return neterror.InternalError("wedding boss enter fight srv failed err: %s", err)
	}

	return nil
}

func (sys *WeddingBossSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_53_51
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	lConf := jsondata.GetWeddingBossLayerConf(req.SceneId)
	if lConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", req.SceneId)
	}

	data := sys.GetLayerData(req.SceneId)

	monInfo, ok := data.Monsters[req.MonId]
	if !ok {
		monInfo = &pb3.WeddingBossMonsterPlayer{
			MonsterId: req.MonId,
		}
		data.Monsters[req.MonId] = monInfo
	}

	monInfo.NeedFollow = req.NeedFollow

	sys.PushLayerInfo(req.SceneId)

	return nil
}

func (sys *WeddingBossSys) AddUsedTimes(sceneId uint32) {
	data := sys.GetLayerData(sceneId)
	data.UsedTimes += 1
}

func (sys *WeddingBossSys) GetLeftUsedTimes(sceneId uint32) int {
	dayTimes := 0
	layerConf := jsondata.GetWeddingBossLayerConf(sceneId)
	if layerConf != nil {
		dayTimes = int(layerConf.DayTimes)
	}
	data := sys.GetLayerData(sceneId)
	leftTimes := utils.MaxInt(0, dayTimes-int(data.UsedTimes))
	return leftTimes
}

func (sys *WeddingBossSys) CheckSceneBoss(sceneId uint32) bool {
	data := sys.GetLayerData(sceneId)
	now := time_util.NowSec()
	if len(data.Monsters) == 0 {
		return true
	}

	for _, monInfo := range data.Monsters {
		if monInfo.ReliveTime == 0 || monInfo.ReliveTime < now {
			return true
		}
	}
	return false
}

func (sys *WeddingBossSys) UpdateWeddingBossInfo(msg *pb3.KillWeddingBoss) {
	data := sys.GetLayerData(msg.SceneId)
	monInfo, ok := data.Monsters[msg.MonsterId]
	if ok {
		monInfo.ReliveTime = msg.ReliveTime
	} else {
		data.Monsters[msg.MonsterId] = &pb3.WeddingBossMonsterPlayer{
			MonsterId:  msg.MonsterId,
			ReliveTime: msg.ReliveTime,
		}
	}
	sys.PushLayerInfo(msg.SceneId)
}

func (sys *WeddingBossSys) CheckWeddingRingLv(sceneId uint32) bool {
	layerConf := jsondata.GetWeddingBossLayerConf(sceneId)
	if layerConf == nil {
		return false
	}

	rSys, ok := sys.owner.GetSysObj(sysdef.SiWeddingRing).(*WeddingRingSys)
	if !ok {
		return false
	}

	if rSys.GetWeddingRingLv() < layerConf.RingLv {
		return false
	}
	return true
}

func onWeddingBossNewDay(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && sys.IsOpen() {
		sys.OnNewDay()
	}
}

func onF2GPullMateIntoFb(buf []byte) {
	msg := &pb3.AttackWeddingBoss{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		logger.LogError("onF2GPullMateIntoFb %v", err)
		return
	}

	player := manager.GetPlayerPtrById(msg.MarryId)
	if player == nil {
		return
	}

	if sys, ok := player.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && sys.IsOpen() {
		err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterWeddingBoss, msg)
		if err != nil {
			player.LogError("wedding boss mate enter fight srv failed err: %s", err)
			return
		}
	}
}

func onF2GUpdateUsedTimes(buf []byte) {
	msg := &pb3.KillWeddingBoss{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		logger.LogError("onF2GUpdateUsedTimes %v", err)
		return
	}
	engine.SendPlayerMessage(msg.ActorId, gshare.OfflineWBossUsedTimes, msg)
}

func onF2GKillBoss(buf []byte) {
	msg := &pb3.KillWeddingBoss{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		logger.LogError("onF2GKillBoss %v", err)
		return
	}

	player := manager.GetPlayerPtrById(msg.ActorId)
	if player == nil {
		return
	}

	if sys, ok := player.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && sys.IsOpen() {
		sys.UpdateWeddingBossInfo(msg)
	}
}

func addWeddingBossUsedTimes(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.KillWeddingBoss)
	if !ok {
		return
	}
	setWeddingBossUsedTimes(player, st)
}

func setWeddingBossUsedTimes(player iface.IPlayer, msg *pb3.KillWeddingBoss) {
	if sys, ok := player.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys); ok && sys.IsOpen() {
		sys.AddUsedTimes(msg.SceneId)
		sys.UpdateWeddingBossInfo(msg)
	}
}

func init() {
	RegisterSysClass(sysdef.SiWeddingBoss, func() iface.ISystem {
		return &WeddingBossSys{}
	})

	//C2S
	net.RegisterSysProto(53, 50, sysdef.SiWeddingBoss, (*WeddingBossSys).c2sEnter)
	net.RegisterSysProto(53, 51, sysdef.SiWeddingBoss, (*WeddingBossSys).c2sFollowBoss)
	net.RegisterSysProto(53, 54, sysdef.SiWeddingBoss, (*WeddingBossSys).c2sEnterReq)
	net.RegisterSysProto(53, 55, sysdef.SiWeddingBoss, (*WeddingBossSys).c2sEnterResp)

	//f2g
	engine.RegisterSysCall(sysfuncid.F2GWeddingBossPullMateIntoFb, onF2GPullMateIntoFb)
	engine.RegisterSysCall(sysfuncid.F2GWeddingBossKillBoss, onF2GKillBoss)
	engine.RegisterSysCall(sysfuncid.F2GWeddingBossUpdateUsedTimes, onF2GUpdateUsedTimes)

	event.RegActorEvent(custom_id.AeNewDay, onWeddingBossNewDay)

	engine.RegisterMessage(gshare.OfflineWBossUsedTimes, func() pb3.Message {
		return &pb3.KillWeddingBoss{}
	}, addWeddingBossUsedTimes)

	gmevent.Register("wbRefreshTimes", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiWeddingBoss).(*WeddingBossSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		data := sys.GetData()
		for _, layerData := range data.Layers {
			layerData.UsedTimes = 0
			layerData.Monsters = make(map[uint32]*pb3.WeddingBossMonsterPlayer)
		}
		sys.S2CInfo()
		return true
	}, 1)
}
