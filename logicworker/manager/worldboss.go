package manager

import (
	"fmt"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"

	"github.com/gzjjyz/logger"
)

func GetWorldBossPubInfo() *pb3.WorldBossPubInfoStore {
	if gshare.GetStaticVar().WorldBossPubInfo == nil {
		gshare.GetStaticVar().WorldBossPubInfo = &pb3.WorldBossPubInfoStore{
			Layers: make(map[uint32]*pb3.WorldBossLayerPubInfoStore),
		}
	}

	return gshare.GetStaticVar().WorldBossPubInfo
}

func GetWorldBossPubLayerInfo(sceneId uint32) (*pb3.WorldBossLayerPubInfoStore, error) {
	layerConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
	if layerConf == nil {
		return nil, fmt.Errorf("layerConf for sceneId %d is nil", sceneId)
	}

	layerInfo, ok := GetWorldBossPubInfo().Layers[sceneId]
	if !ok {
		layerInfo = &pb3.WorldBossLayerPubInfoStore{
			SceneId: sceneId,
			Monster: make(map[uint32]*pb3.WorldBossMonsterPubStore),
		}

		GetWorldBossPubInfo().Layers[sceneId] = layerInfo
	}

	if layerInfo.Monster == nil {
		layerInfo.Monster = make(map[uint32]*pb3.WorldBossMonsterPubStore)
	}

	return layerInfo, nil
}

func GetWorldBossMonsterPubStoreInfo(sceneId, monsterId uint32) (*pb3.WorldBossMonsterPubStore, error) {
	layerInfo, err := GetWorldBossPubLayerInfo(sceneId)
	if err != nil {
		return nil, err
	}

	monsterInfo, ok := layerInfo.Monster[monsterId]
	if !ok {
		monsterInfo = &pb3.WorldBossMonsterPubStore{
			MonsterId: monsterId,
		}
		layerInfo.Monster[monsterId] = monsterInfo
	}

	return monsterInfo, nil
}

func OnF2GPackAndBroadcastWorldBossDeadInfo(buf []byte) {
	var req pb3.S2C_161_4
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("OnF2GPackAndBroadcastWorldBossDeadInfo Unmarshal failed err: %s", err)
		return
	}

	monInfo, err := GetWorldBossMonsterPubStoreInfo(req.SceneId, req.MonsterInfo.MonsterId)
	if err != nil {
		logger.LogError("OnF2GPackAndBroadcastWorldBossDeadInfo GetWorldBossMonsterPubStoreInfo failed err: %s", err)
		return
	}

	playerBase, ok := GetData(monInfo.FirstBloodActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if ok {
		req.MonsterInfo.FirstBloodActorName = playerBase.Name
	}
	engine.Broadcast(chatdef.CIWorld, 0, 161, 4, &req, 0)
}

func OnF2GPackAndBroadcastWorldBossReliveInfo(buf []byte) {
	var req pb3.S2C_161_3
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("OnF2GPackAndBroadcastWorldBossDeadInfo Unmarshal failed err: %s", err)
		return
	}

	monInfo, err := GetWorldBossMonsterPubStoreInfo(req.SceneId, req.MonsterInfo.MonsterId)
	if err != nil {
		logger.LogError("OnF2GPackAndBroadcastWorldBossDeadInfo GetWorldBossMonsterPubStoreInfo failed err: %s", err)
		return
	}

	playerBase, ok := GetData(monInfo.FirstBloodActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if ok {
		req.MonsterInfo.FirstBloodActorName = playerBase.Name
	}
	engine.Broadcast(chatdef.CIWorld, 0, 161, 3, &req, 0)
}

func OnF2GPackWorldBossInfoRes(buf []byte) {
	var req pb3.WorldBossPackInfoRes
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("OnF2GPackWorldBossInfoRes Unmarshal failed err: %s", err)
		return
	}

	player := GetPlayerPtrById(req.ActorId)
	if player == nil {
		return
	}

	fightWorldBossPubInfo := req.PubInfo
	gameWorldBossPubInfo := gshare.GetStaticVar().WorldBossPubInfo
	if gameWorldBossPubInfo == nil {
		gameWorldBossPubInfo = &pb3.WorldBossPubInfoStore{}
	}

	for sceneId, flMonsters := range fightWorldBossPubInfo.Layers {
		if gameWorldBossPubInfo.Layers == nil {
			gameWorldBossPubInfo.Layers = make(map[uint32]*pb3.WorldBossLayerPubInfoStore)
		}

		glayer, ok := gameWorldBossPubInfo.Layers[sceneId]
		if !ok {
			glayer = &pb3.WorldBossLayerPubInfoStore{
				SceneId: sceneId,
				Monster: make(map[uint32]*pb3.WorldBossMonsterPubStore),
			}
			gameWorldBossPubInfo.Layers[sceneId] = glayer
		}

		for fmId, fmonster := range flMonsters.Monster {
			glMonster, ok := glayer.Monster[fmId]
			if ok && glMonster.FirstBloodActorId != 0 {
				playerBaseDataPb := GetData(glMonster.FirstBloodActorId, gshare.ActorDataBase)

				playerBaseData, ok := playerBaseDataPb.(*pb3.PlayerDataBase)
				if !ok {
					continue
				}
				fmonster.FirstBloodActorName = playerBaseData.Name
			}
		}
	}

	player.SendProto3(161, 0, &pb3.S2C_161_0{
		Info: fightWorldBossPubInfo,
	})
}

func init() {
	engine.RegisterSysCall(sysfuncid.F2GPackWorldBossInfoRes, OnF2GPackWorldBossInfoRes)
	engine.RegisterSysCall(sysfuncid.F2GPackAndBroadcastWorldBossDeadInfo, OnF2GPackAndBroadcastWorldBossDeadInfo)
	engine.RegisterSysCall(sysfuncid.F2GPackAndBroadcastWorldBossReliveInfo, OnF2GPackAndBroadcastWorldBossReliveInfo)
}
