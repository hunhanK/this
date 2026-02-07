package actorsystem

import (
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/commontimesconter"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type WorldBossSys struct {
	Base
	data    *pb3.WorldBossPersonalInfo
	counter *commontimesconter.CommonTimesCounter
}

func (sys *WorldBossSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *WorldBossSys) init() {
	data := sys.GetBinaryData().WorldBossPersonalInfo
	if data == nil {
		data = &pb3.WorldBossPersonalInfo{}
		sys.GetBinaryData().WorldBossPersonalInfo = data
	}

	if data.LayersInfo == nil {
		data.LayersInfo = make(map[uint32]*pb3.WorldBossLayerPersonalInfo)
	}

	if data.TimesCounter == nil {
		data.TimesCounter = commontimesconter.NewCommonTimesCounterData()
	}

	if data.DailyKillMons == nil {
		data.DailyKillMons = make(map[uint32]*pb3.WordBossKillMon)
	}

	sys.data = data

	// 初始化计数器
	sys.counter = commontimesconter.NewCommonTimesCounter(
		sys.data.TimesCounter,
		commontimesconter.WithOnGetFreeTimes(func() uint32 {
			return jsondata.GetWorldBossCommonConf().DailyRewardTimes
		}),
		commontimesconter.WithOnGetOtherAddFreeTimes(func() uint32 {
			privilegeFreeTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumWorldBossFreeTimes)
			attrAddTimes := sys.GetOwner().GetFightAttr(attrdef.WorldBossTimesAdd)
			return uint32(privilegeFreeTimes + attrAddTimes)
		}),
		commontimesconter.WithOnGetDailyBuyTimesUpLimit(func() uint32 {
			canBuyTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumWorldBossBuyTimes)
			return uint32(canBuyTimes)
		}),
		commontimesconter.WithOnUpdateCanUseTimes(func(canUseTimes uint32) {
			sys.owner.SetExtraAttr(attrdef.WorldBossCanUseTimes, attrdef.AttrValueAlias(canUseTimes))
		}),
		commontimesconter.WithOnUpdateRetrievalTimes(func(retrievalTimes uint32) {
			sys.owner.SetExtraAttr(attrdef.WorldBossRetrievalTimes, attrdef.AttrValueAlias(retrievalTimes))
			sys.sendPersonalInfo()
		}),
		commontimesconter.WithOnGetLogoutDays(func() uint32 {
			logoutDay := sys.owner.GetLogoutDay()
			return logoutDay
		}),

		commontimesconter.WithOnGetRetrievalDays(func() uint32 {
			retrievalDay := uint32(1)
			addRetrievalDay, _ := sys.owner.GetPrivilege(privilegedef.EnumWorldBossRetrievalDays)
			retrievalDay += uint32(addRetrievalDay)
			if sys.GetOwner().GetCreateDay()-1 < retrievalDay {
				retrievalDay = sys.GetOwner().GetCreateDay() - 1
			}
			return retrievalDay
		}),
	)
	err := sys.counter.Init()
	if err != nil {
		sys.GetOwner().LogError("init counter failed")
		return
	}
}

func (sys *WorldBossSys) OnOpen() {
	sys.init()
	sys.sendPersonalInfo()
	sys.sendPubInfo()
	sys.syncPersonalToFightActor()
}

func (sys *WorldBossSys) OnLogin() {
	sys.owner.SetExtraAttr(attrdef.WorldBossRetrievalTimes, attrdef.AttrValueAlias(sys.counter.GetRetrievalTimes()))
	sys.sendPersonalInfo()
	sys.sendPubInfo()
}

func (sys *WorldBossSys) OnLoginFight() {
	if !sys.GetOwner().InLocalFightSrv() {
		return
	}

	sys.syncPersonalToFightActor()
}

func (sys *WorldBossSys) OnReconnect() {
	sys.sendPersonalInfo()
	sys.sendPubInfo()
	sys.syncPersonalToFightActor()
}

func (sys *WorldBossSys) sendPersonalInfo() {
	sys.SendProto3(161, 1, &pb3.S2C_161_1{
		Info: sys.data,
	})
}

func (sys *WorldBossSys) sendPubInfo() {
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FPackWorldBossInfoReq, &pb3.WorldBossPackInfoReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: sys.GetOwner().GetId(),
	})
}

func (sys *WorldBossSys) syncPersonalToFightActor() {
	if !sys.GetOwner().InLocalFightSrv() {
		return
	}

	info := &pb3.WorldBossPersonalInfoOnFight{
		LayersInfo: make(map[uint32]*pb3.WorldBossLayerPersonalInfoOnFight),
	}

	for sceneId, layer := range sys.data.LayersInfo {
		layerToSync := pb3.WorldBossLayerPersonalInfoOnFight{
			Monster: make(map[uint32]*pb3.WorldBossMonsterPersonalOnFight),
		}

		for monId, mon := range layer.Monster {
			layerToSync.Monster[monId] = &pb3.WorldBossMonsterPersonalOnFight{
				MonsterId:  monId,
				FirstBlood: mon.FirstBlood,
			}
		}

		info.LayersInfo[sceneId] = &layerToSync
	}

	err := sys.GetOwner().CallActorFunc(actorfuncid.WorldBossSyncPersonalInfoReq, info)
	if err != nil {
		sys.LogError("err:%v", err)
	}
}

func (sys *WorldBossSys) reqFightSyncMonsterInfo(sceneId, monId uint32) {
	if !sys.GetOwner().InLocalFightSrv() {
		return
	}

	monInfoToSync := &pb3.WorldBossMonsterPersonalOnFight{}

	layerInfo, ok := sys.data.LayersInfo[sceneId]
	if ok {
		mon, ok := layerInfo.Monster[monId]
		if ok {
			monInfoToSync.FirstBlood = mon.FirstBlood
			monInfoToSync.MonsterId = monId
		}
	}

	sys.GetOwner().CallActorFunc(actorfuncid.WorldBossSyncPersonalLayerInfoReq, &pb3.WorldBossSyncPersonalMonsterInfoReq{
		SceneId: sceneId,
		Mon:     monInfoToSync,
	})
}

func (sys *WorldBossSys) c2sBuyRewardTimes(msg *base.Message) error {
	var req pb3.C2S_161_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	if !sys.counter.CheckCanBuyDailyAddTimes(req.Times) {
		return neterror.ParamsInvalidError("buy daily times reach limit")
	}

	dailyBuyTimes := sys.counter.GetDailyBuyTimes()
	var consumes []*jsondata.Consume
	for i := uint32(0); i < req.Times; i++ {
		consumes = append(consumes, sys.getConsumes(dailyBuyTimes)...)
		dailyBuyTimes++
	}

	consumeState := sys.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogWorldBossBuyRewardTimes})
	if !consumeState {
		return neterror.ParamsInvalidError("consume failed")
	}

	sys.counter.AddBuyDailyAddTimes(req.Times)
	sys.sendRewardTimesInfo()
	return nil
}

func (sys *WorldBossSys) getConsumes(buyTimes uint32) []*jsondata.Consume {
	var maxTimes uint32
	for _, v := range jsondata.GetWorldBossCommonConf().BuyConsume {
		if v.Times > maxTimes {
			maxTimes = v.Times
		}
	}

	if buyTimes > maxTimes {
		buyTimes = maxTimes
	}

	var consumes []*jsondata.Consume
	for _, v := range jsondata.GetWorldBossCommonConf().BuyConsume {
		if v.Times == buyTimes {
			consumes = append(consumes, v.Consume)
		}
	}

	return consumes
}

func (sys *WorldBossSys) sendRewardTimesInfo() {
	sys.SendProto3(161, 2, &pb3.S2C_161_2{
		TimesCounter: sys.data.TimesCounter,
	})
}

func (sys *WorldBossSys) c2sEnterWorldBoss(msg *base.Message) error {
	var req pb3.C2S_161_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	return sys.enter(req.SceneId)
}

func (sys *WorldBossSys) enter(sceneId uint32) error {
	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", sceneId)
	}

	if sys.GetOwner().GetCircle() < layerConf.BoundaryLevel {
		return neterror.ParamsInvalidError("boundary level limit current %d needed %d", sys.GetOwner().GetCircle(), layerConf.BoundaryLevel)
	}

	if sys.GetOwner().GetNirvanaLevel() < layerConf.NirvanaLv {
		return neterror.ParamsInvalidError("nirvana level limit current %d needed %d", sys.GetOwner().GetNirvanaLevel(), layerConf.NirvanaLv)
	}

	if sys.GetOwner().GetLevel() < layerConf.PlayerLv {
		return neterror.ParamsInvalidError("player level limit current %d needed %d", sys.GetOwner().GetLevel(), layerConf.PlayerLv)
	}

	err := sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterWorldBoss, &pb3.AttackWorldBoss{
		SceneId: sceneId,
	})
	if err != nil {
		return neterror.InternalError("enter fight srv failed err: %s", err)
	}
	return nil
}

func (sys *WorldBossSys) c2sFirstBloodReward(msg *base.Message) error {
	var req pb3.C2S_161_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", req.SceneId)
	}

	layerInfo, ok := sys.data.LayersInfo[req.SceneId]
	if !ok {
		return neterror.ParamsInvalidError("layer info is nil sceneId %d", req.SceneId)
	}

	monConf := jsondata.GetWorldBossLayerBossConfigBySceneId(req.SceneId, req.MonId)
	if monConf == nil {
		return neterror.ConfNotFoundError("mon conf is nil sceneId %d monId %d", req.SceneId, req.MonId)
	}

	monInfo, ok := layerInfo.Monster[req.MonId]
	if !ok {
		return neterror.ParamsInvalidError("monster info is nil sceneId %d monId %d", req.SceneId, req.MonId)
	}

	if !monInfo.FirstBlood {
		return neterror.ParamsInvalidError("first blood is false sceneId %d monId %d", req.SceneId, req.MonId)
	}

	if monInfo.FirstBloodRewarded {
		return neterror.ParamsInvalidError("first blood is rewarded sceneId %d monId %d", req.SceneId, req.MonId)
	}

	monInfo.FirstBloodRewarded = true

	state := engine.GiveRewards(sys.GetOwner(), monConf.FirstKillReward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogWorldBossFirstBloodReward,
	})

	if !state {
		return neterror.InternalError("give first blood reward failed")
	}

	sys.SendProto3(161, 5, &pb3.S2C_161_5{
		SceneId:     req.SceneId,
		MonsterInfo: monInfo,
	})

	sys.SendProto3(161, 7, &pb3.S2C_161_7{
		SceneId: req.SceneId,
		MonId:   req.MonId,
	})

	return nil
}

func (sys *WorldBossSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_161_8
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(req.SceneId)
	if layerConf == nil {
		return neterror.ConfNotFoundError("layer conf is nil sceneId %d", req.SceneId)
	}

	layerInfo, ok := sys.data.LayersInfo[req.SceneId]
	if !ok {
		layerInfo = &pb3.WorldBossLayerPersonalInfo{
			Monster: make(map[uint32]*pb3.WorldBossMonsterPersonal),
		}
		sys.data.LayersInfo[req.SceneId] = layerInfo
	}

	monInfo, ok := layerInfo.Monster[req.MonId]
	if !ok {
		monInfo = &pb3.WorldBossMonsterPersonal{
			MonsterId: req.MonId,
		}
		layerInfo.Monster[req.MonId] = monInfo
	}

	monInfo.NeedFollow = req.NeedFollow
	sys.SendProto3(161, 5, &pb3.S2C_161_5{
		SceneId:     req.SceneId,
		MonsterInfo: monInfo,
	})

	return nil
}

func (sys *WorldBossSys) c2sQuickAttack(msg *base.Message) error {
	var req pb3.C2S_161_9
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}

	leftTimes := sys.counter.GetLeftTimes()
	if leftTimes <= 0 {
		return neterror.ParamsInvalidError("times limit")
	}

	killData, ok := sys.data.DailyKillMons[req.Id]
	if !ok || killData.IsFinish {
		return neterror.ParamsInvalidError("cannot quick attack")
	}

	err := sys.GetOwner().CallActorFunc(actorfuncid.G2FWorldBossQuickReward, &pb3.WorldBossQuickRewardReq{
		Id:      req.Id,
		SceneId: killData.SceneId,
		MonId:   killData.MonId,
	})
	if err != nil {
		sys.LogError("err: %v", err)
	}
	return nil
}

func (sys *WorldBossSys) getLayerInfo(sceneId uint32) (*pb3.WorldBossLayerPersonalInfo, error) {
	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
	if layerConf == nil {
		return nil, fmt.Errorf("layerConf for sceneId %d is nil", sceneId)
	}

	layerInfo, ok := sys.data.LayersInfo[sceneId]
	if !ok {
		layerInfo = &pb3.WorldBossLayerPersonalInfo{
			Monster: make(map[uint32]*pb3.WorldBossMonsterPersonal),
		}

		sys.data.LayersInfo[sceneId] = layerInfo
	}

	return layerInfo, nil
}

func (sys *WorldBossSys) getLayerMonsterInfo(sceneId, monId uint32) (*pb3.WorldBossMonsterPersonal, error) {
	layerInfo, err := sys.getLayerInfo(sceneId)
	if err != nil {
		return nil, err
	}

	mon, ok := layerInfo.Monster[monId]
	if !ok {
		mon = &pb3.WorldBossMonsterPersonal{
			MonsterId: monId,
		}

		layerInfo.Monster[monId] = mon
	}

	return mon, nil
}

func (sys *WorldBossSys) onNewDay() {
	// 先加上 下个版本移除
	sys.data.UsedRewardTimes = 0
	sys.data.HelpTimes = 0
	sys.data.ItemAddRewardTimes = 0
	sys.data.BuyedRewardTimes = 0

	sys.counter.NewDay()
	sys.resetKillMon()
	sys.sendPersonalInfo()
}

func (sys *WorldBossSys) recordKillMon(sceneId, monId uint32) {
	Id := uint32(len(sys.data.DailyKillMons)) + 1
	sys.data.DailyKillMons[Id] = &pb3.WordBossKillMon{
		Id:        Id,
		SceneId:   sceneId,
		MonId:     monId,
		Timestamp: time_util.NowSec(),
	}
}

func (sys *WorldBossSys) updateKillMon(Id uint32) {
	killData, ok := sys.data.DailyKillMons[Id]
	if !ok {
		return
	}
	killData.IsFinish = true
}

func (sys *WorldBossSys) resetKillMon() {
	sys.data.DailyKillMons = make(map[uint32]*pb3.WordBossKillMon)
}

// 触发找回
func (sys *WorldBossSys) triggerRetrieval(sceneId, monId uint32) {
	monConf := jsondata.GetWorldBossLayerBossConfigBySceneId(sceneId, monId)
	if monConf == nil {
		return
	}

	conf := jsondata.GetWorldBossCommonConf()
	if conf == nil {
		return
	}

	// 开服天数不满足
	openDay := gshare.GetOpenServerDay()
	if openDay < conf.RetrievalDay {
		return
	}

	// 新手层不触发
	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
	if !layerConf.NeedDeductTimes {
		return
	}

	retrievalTimes := sys.owner.GetExtraAttrU32(attrdef.WorldBossRetrievalTimes)
	if retrievalTimes > 0 && len(monConf.RetrievalReward) > 0 {
		engine.GiveRewards(sys.owner, monConf.RetrievalReward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogWorldBossRetrievalReward})
		sys.counter.AddRetrievalUsedTimes(1)
	}

	// 记录当日击杀数据
	sys.recordKillMon(sceneId, monId)
	sys.SendProto3(161, 10, &pb3.S2C_161_10{DailyKillMons: sys.data.DailyKillMons, LeftTimes: sys.counter.GetLeftTimes()})
}

func (sys *WorldBossSys) onUsedRewardTimes() {
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogWorldBossUseRewardTimes, &pb3.LogPlayerCounter{
		NumArgs: uint64(sys.counter.GetDailyUseTimes()),
	})

	sys.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionKillWorldBoss)
	sys.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiWorldBoss, 1)
	sys.owner.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
		Cond:  custom_id.FaBaoTalentCondWorldBoss,
		Count: 1,
	})
	sys.owner.TriggerQuestEvent(custom_id.QttConsumeTimesKillWorldBossTimes, 0, 1)
}

func onWorldBossFirstBlood(player iface.IPlayer, buf []byte) {
	var req pb3.WorldbossFirstBlood
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("onWorldBossFirstBlood Unmarshal failed err: %s", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		player.LogError("onWorldBossFirstBlood sys is not WorldBossSys or sys is not open")
		return
	}

	sceneId, monId := req.SceneId, req.MonId

	monInfo, err := sys.getLayerMonsterInfo(sceneId, monId)
	if err != nil {
		player.LogError("onWorldBossFirstBlood getLayerMonsterInfo failed err: %s", err)
		return
	}

	monInfo.FirstBlood = true

	// 触发找回
	sys.triggerRetrieval(sceneId, monId)

	logArg := map[string]any{
		"SceneId": sceneId,
		"MonId":   monId,
	}
	logArgBytes, _ := json.Marshal(logArg)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogWorldBossFirstBlood, &pb3.LogPlayerCounter{
		StrArgs: string(logArgBytes),
	})

	player.SendProto3(161, 5, &pb3.S2C_161_5{
		SceneId:     req.SceneId,
		MonsterInfo: monInfo,
	})
	sys.sendPersonalInfo()

	sys.reqFightSyncMonsterInfo(req.SceneId, req.MonId)

	player.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionKillWorldBoss)
	player.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
		Cond:  custom_id.FaBaoTalentCondWorldBoss,
		Count: 1,
	})
}

func recordLatestFirstBlood(player iface.IPlayer, buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("onWorldBossFirstBlood Unmarshal failed err: %s", err)
		return
	}

	var (
		sceneId = req.GetU32Param()
		monId   = req.GetU32Param2()
	)

	storedPubMonInfo, err := manager.GetWorldBossMonsterPubStoreInfo(sceneId, monId)
	if err != nil {
		player.LogError("onWorldBossFirstBlood GetWorldBossMonsterPubStoreInfo failed err: %s", err)
		return
	}

	storedPubMonInfo.FirstBloodActorId = player.GetId()
}

func onWorldBossDeductRewardTimes(player iface.IPlayer, buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("onWorldBossDeductRewardTimes Unmarshal failed err: %s", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys is not open")
		return
	}

	leftRewardTimes := sys.counter.GetLeftTimes()
	if leftRewardTimes <= 0 {
		return
	}

	// 扣除挑战次数
	if !sys.counter.DeductTimes(1) {
		player.LogWarn("times not enough")
		return
	}
	// 触发找回
	sceneId, monId := req.U32Param, req.U32Param2
	sys.triggerRetrieval(sceneId, monId)
	sys.onUsedRewardTimes()
	sys.sendPersonalInfo()
}

func worldBossItemAddRewardTimes(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("worldBossItemAddRewardTimes param len < 1")
		return false, false, 0
	}

	addTimesPer := conf.Param[0]

	sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		player.LogError("worldBossItemAddRewardTimes sys is not WorldBossSys or sys is not open")
		return false, false, 0
	}

	logArgs := map[string]any{}
	player.LogDebug("worldBossItemAddRewardTimes before %d", sys.counter.GetDailyItemAddTimes())
	sys.counter.AddItemDailyAddTimes(addTimesPer * uint32(param.Count))
	player.LogDebug("worldBossItemAddRewardTimes after %d", sys.counter.GetDailyItemAddTimes())
	logArgs["param.Count"] = param.Count
	logArgs["addTimesPer"] = addTimesPer

	argBytes, _ := json.Marshal(logArgs)

	logworker.LogPlayerBehavior(player, pb3.LogId_LogWorldBossItemAddRewardTimes, &pb3.LogPlayerCounter{
		StrArgs: string(argBytes),
		NumArgs: uint64(sys.counter.GetDailyItemAddTimes()),
	})

	sys.sendRewardTimesInfo()
	return true, true, param.Count
}

func onWorldBossQuickReward(player iface.IPlayer, buf []byte) {
	var req pb3.WorldBossQuickRewardRet
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("onWorldBossQuickReward Unmarshal failed err: %s", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys is not open")
		return
	}

	commConf := jsondata.GetWorldBossCommonConf()
	if commConf == nil {
		player.LogError("worldBoss common config is not exits")
		return
	}

	leftRewardTimes := sys.counter.GetLeftTimes()
	if leftRewardTimes <= 0 {
		return
	}

	id, sceneId, monId := req.Id, req.SceneId, req.MonId
	rewards := jsondata.Pb3RewardVecToStdRewardVec(req.Rewards)

	monConf := jsondata.GetWorldBossLayerBossConfigBySceneId(sceneId, monId)
	if monConf == nil {
		return
	}

	// 扣除挑战次数
	if !sys.counter.DeductTimes(1) {
		player.LogWarn("times not enough")
		return
	}

	// 扣除找回次数
	retrievalTimes := sys.owner.GetExtraAttrU32(attrdef.WorldBossRetrievalTimes)
	if retrievalTimes > 0 && len(monConf.RetrievalReward) > 0 {
		rewards = append(rewards, monConf.RetrievalReward...)
		sys.counter.AddRetrievalUsedTimes(1)
	}

	rewards = jsondata.MergeStdReward(rewards)
	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogWorldBossQuickReward})

	sys.updateKillMon(id)
	sys.onUsedRewardTimes()
	sys.owner.FinishBossQuickAttack(monId, sceneId, commConf.FbId)

	sys.SendProto3(161, 9, &pb3.S2C_161_9{Id: id})
	sys.owner.SendShowRewardsPop(rewards)
	sys.sendPersonalInfo()
}

func worldBossUseBox(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys is not open")
		return false, false, 0
	}

	if len(conf.Param) < 1 {
		player.LogError("worldBossUseBox param len < 1")
		return false, false, 0
	}

	monId := conf.Param[0]
	err := sys.GetOwner().CallActorFunc(actorfuncid.G2FUseWorldBossBox, &pb3.CommonSt{
		U32Param: monId,
	})
	if err != nil {
		sys.LogError("err: %v", err)
	}

	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiWorldBoss, func() iface.ISystem {
		return &WorldBossSys{}
	})

	net.RegisterSysProtoV2(161, 2, sysdef.SiWorldBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WorldBossSys).c2sBuyRewardTimes
	})

	net.RegisterSysProtoV2(161, 6, sysdef.SiWorldBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WorldBossSys).c2sEnterWorldBoss
	})

	net.RegisterSysProtoV2(161, 7, sysdef.SiWorldBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WorldBossSys).c2sFirstBloodReward
	})

	net.RegisterSysProtoV2(161, 8, sysdef.SiWorldBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WorldBossSys).c2sFollowBoss
	})

	net.RegisterSysProtoV2(161, 9, sysdef.SiWorldBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WorldBossSys).c2sQuickAttack
	})

	engine.RegisterActorCallFunc(playerfuncid.WorldBossFirstBlood, onWorldBossFirstBlood)

	engine.RegisterActorCallFunc(playerfuncid.WorldBossRecordLatestFirstBlood, recordLatestFirstBlood)

	engine.RegisterActorCallFunc(playerfuncid.WorldBossDeductRewardTimes, onWorldBossDeductRewardTimes)

	engine.RegisterActorCallFunc(playerfuncid.WorldBossQuickReward, onWorldBossQuickReward)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
		if !ok || !sys.IsOpen() {
			sys.LogError("sys is not open")
			return
		}
		sys.onNewDay()
	})

	event.RegActorEvent(custom_id.AeActiveActSweep, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
		if !ok || !sys.IsOpen() {
			sys.LogError("sys is not open")
			return
		}

		commonConf := jsondata.GetActSweepCommonConf()
		if commonConf == nil {
			return
		}
		sys.counter.AddRetrievalDailyAddTimes(commonConf.WorldBossRetrievalCount)
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddWorldBossRewardTimes, worldBossItemAddRewardTimes)
	miscitem.RegCommonUseItemHandle(itemdef.UseWorldBossBox, worldBossUseBox)

	event.RegActorEvent(custom_id.AeKillMon, onKillWorldBoss)

	event.RegActorEvent(custom_id.AeRegFightAttrChange, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		attrType, ok := args[0].(uint32)
		if !ok {
			return
		}
		if attrType != attrdef.WorldBossTimesAdd {
			return
		}
		sys, ok := actor.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.counter.ReCalcTimes()
	})

	gmevent.Register("woldboss.setUsedTimes", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
		if !ok || !sys.IsOpen() {
			return false
		}

		if len(args) < 1 {
			return false
		}
		sys.counter.NewDay()
		sys.syncPersonalToFightActor()
		sys.sendPersonalInfo()
		return true
	}, 1)
}

func onKillWorldBoss(actor iface.IPlayer, args ...interface{}) {
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

	sys, ok := actor.GetSysObj(sysdef.SiWorldBoss).(*WorldBossSys)
	if !ok || !sys.IsOpen() {
		return
	}

	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
	if nil == layerConf {
		return
	}

	if layerConf.Boss == nil {
		return
	}

	_, ok = layerConf.Boss[monId]
	if !ok {
		return
	}

	actor.TriggerQuestEvent(custom_id.QttKillXLayerWorldBoss, layerConf.LayerId, int64(count))
}
