/**
 * @Author: zjj
 * @Date: 2024/8/7
 * @Desc: 仙灵幻境
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type CrossFairyBossSys struct {
	Base
}

func (sys *CrossFairyBossSys) getData() *pb3.CrossFairyBossPlayerInfo {
	data := sys.GetBinaryData().CrossFairyBossInfo
	if data == nil {
		sys.GetBinaryData().CrossFairyBossInfo = &pb3.CrossFairyBossPlayerInfo{}
		data = sys.GetBinaryData().CrossFairyBossInfo
	}
	if data.Layers == nil {
		data.Layers = make(map[uint32]*pb3.CrossFairyBossLayerPlayerInfo)
	}
	return data
}

func (sys *CrossFairyBossSys) OnOpen() {
	sys.g2fCrossFairyBossInfoReq()
	sys.s2cInfo()
}

func (sys *CrossFairyBossSys) OnLogin() {
	sys.g2fCrossFairyBossInfoReq()
	sys.s2cInfo()
}

func (sys *CrossFairyBossSys) OnReconnect() {
	sys.g2fCrossFairyBossInfoReq()
	sys.s2cInfo()
}

func (sys *CrossFairyBossSys) onNewDay() {
	sys.getData().UsedTimes = 0
	sys.s2cInfo()
}

// 公共信息战斗服发送
func (sys *CrossFairyBossSys) g2fCrossFairyBossInfoReq() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FCrossFairyBossInfoReq, &pb3.CrossFairyBossInfoReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: sys.GetOwner().GetId(),
	})
	if err != nil {
		sys.GetOwner().LogError("CrossFairyBossSys pushPubInfo err:%v", err)
	}
}

// 玩家自己数据逻辑服发送
func (sys *CrossFairyBossSys) s2cInfo() {
	sys.SendProto3(168, 10, &pb3.S2C_168_10{
		Info: sys.getData(),
	})
}

// 获取剩余次数
func (sys *CrossFairyBossSys) getLeftUsedTimes() uint32 {
	privilegeAddTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumCrossFairyBossFreeTimes)
	dailyTimes := jsondata.GetCrossFairyBossCommonConf().DailyRewardTimes

	leftTimes := utils.MaxUInt32(0, uint32(dailyTimes)+uint32(privilegeAddTimes)-sys.getData().UsedTimes)
	return leftTimes
}

// C2S
func (sys *CrossFairyBossSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_168_14
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	conf := jsondata.GetCrossFairyBossCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("crossFairyBoss common conf is nil")
	}

	layerConf := jsondata.GetCrossFairyBossLayerConf(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("crossFairyBoss layer conf is nil sceneId %d", req.SceneId)
	}

	owner := sys.GetOwner()

	// 校验仙灵条件
	star := layerConf.FairyStar
	color := layerConf.FairyColor
	count := layerConf.FairyCount
	statCount := statFairyByStarAndColor(owner, color, star)
	if statCount < count {
		return neterror.ParamsInvalidError("not reach into cand color %d star %d count %d", color, star, count)
	}

	if layerConf.OpenDayLimit > 0 && layerConf.OpenDayLimit > gshare.GetOpenServerDay() {
		return neterror.ParamsInvalidError("not reach into cand OpenDayLimit %d %d", layerConf.OpenDayLimit, gshare.GetOpenServerDay())
	}

	statFairyFightVal(owner)
	if layerConf.FairyPowerLimit > 0 {
		fightVal := statFairyFightVal(owner)
		if layerConf.FairyPowerLimit > fightVal {
			return neterror.ParamsInvalidError("crossFairyBoss enter power limit %d %d", layerConf.FairyPowerLimit, fightVal)
		}
	}

	if owner.GetSceneId() == req.SceneId {
		return neterror.ConfNotFoundError("crossFairyBoss player is in fuBen %d", req.SceneId)
	}

	times := req.MergeTimes
	if times == 0 {
		times = 1
	}
	if sys.getLeftUsedTimes() <= 0 || sys.getLeftUsedTimes() < times {
		return neterror.ConfNotFoundError("crossFairyBoss enter times error %d", req.SceneId)
	}

	data := sys.getData()
	var totalConsumes jsondata.ConsumeVec
	for i := uint32(1); i <= times; i++ {
		next := data.UsedTimes + i
		consumes := jsondata.GetCrossFairyBossConsumeConf(req.SceneId, next)
		if consumes == nil {
			return neterror.ParamsInvalidError("cross fairy boss consume config err")
		}
		totalConsumes = append(totalConsumes, consumes...)
	}

	if owner.InDartCar() {
		owner.SendTipMsg(tipmsgid.Tpindartcar)
		return nil
	}

	if !owner.CheckConsumeByConf(totalConsumes, false, pb3.LogId_LogCrossFairyBossEnterConsume) {
		return neterror.ConsumeFailedError("consume failed")
	}

	err := owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterCrossFairyBoss,
		&pb3.AttackCrossFairyBoss{
			SceneId: req.SceneId,
		},
		&argsdef.ConsumesSt{
			Consumes: totalConsumes,
			LogId:    pb3.LogId_LogCrossFairyBossEnterConsume,
		})
	if err != nil {
		return neterror.InternalError("crossFairyBoss enter fight srv failed err: %s", err)
	}

	err = owner.MergeTimesChallengeBoss(base.SmallCrossServer, fubendef.EnterCrossFairyBoss, times)
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	// 加次数
	data.UsedTimes += times
	sys.s2cInfo()
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCrossFairyBossEnterConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.UsedTimes),
	})
	return nil

}

func (sys *CrossFairyBossSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_168_15
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	lConf := jsondata.GetCrossFairyBossLayerConf(req.SceneId)
	if lConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", req.SceneId)
	}

	data := sys.getData()
	layerInfo, ok := data.Layers[req.SceneId]
	if !ok {
		layerInfo = &pb3.CrossFairyBossLayerPlayerInfo{
			Monster: make(map[uint32]*pb3.CrossFairyBossMonsterPlayer),
		}
		data.Layers[req.SceneId] = layerInfo
	}

	monInfo, ok := layerInfo.Monster[req.MonId]
	if !ok {
		monInfo = &pb3.CrossFairyBossMonsterPlayer{
			MonsterId: req.MonId,
		}
		layerInfo.Monster[req.MonId] = monInfo
	}

	monInfo.NeedFollow = req.NeedFollow

	sys.s2cInfo()

	return nil
}

func onCrossFairyBossNewDay(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiCrossFairyBoss).(*CrossFairyBossSys); ok && sys.IsOpen() {
		sys.onNewDay()
	}
}

func onF2GCrossFairyBossInfoRes(buf []byte) {
	var req pb3.CrossFairyBossInfoRes
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onF2GCrossFairyBossInfoRes Unmarshal failed err: %s", err)
		return
	}

	player := manager.GetPlayerPtrById(req.ActorId)
	if player == nil {
		return
	}

	player.SendProto3(168, 11, &pb3.S2C_168_11{
		Info: req.PubInfo,
	})
}

func statFairyByStarAndColor(actor iface.IPlayer, color, star uint32) uint32 {
	if color == 0 && star == 0 {
		return 0
	}
	var count uint32
	for _, itemSt := range actor.GetMainData().ItemPool.FairyBag {
		conf := jsondata.GetFairyConf(itemSt.ItemId)
		if conf == nil {
			continue
		}
		if color != 0 && conf.Color < color {
			continue
		}
		if star != 0 && itemSt.Union2 < star {
			continue
		}
		count++
	}
	return count
}

func statFairyFightVal(actor iface.IPlayer) int64 {
	fairySys, ok := actor.GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !fairySys.IsOpen() {
		return 0
	}

	if fairySys.battlePos == 0 {
		return 0
	}

	data := fairySys.GetData()
	var fightValue int64
	for _, val := range data.BattleFairy {
		fairy, err := fairySys.GetFairy(val)
		if err != nil {
			actor.LogError("err:%v", err)
			continue
		}
		if fairy == nil || fairy.Ext == nil {
			continue
		}
		fightValue += int64(fairy.Ext.Power)
	}
	return fightValue
}

func init() {
	RegisterSysClass(sysdef.SiCrossFairyBoss, func() iface.ISystem {
		return &CrossFairyBossSys{}
	})

	// c2s
	net.RegisterSysProto(168, 14, sysdef.SiCrossFairyBoss, (*CrossFairyBossSys).c2sEnter)
	net.RegisterSysProto(168, 15, sysdef.SiCrossFairyBoss, (*CrossFairyBossSys).c2sFollowBoss)

	// f2g
	engine.RegisterSysCall(sysfuncid.F2GCrossFairyBossInfoRes, onF2GCrossFairyBossInfoRes)

	event.RegActorEvent(custom_id.AeNewDay, onCrossFairyBossNewDay)
}
