/**
 * @Author: yangqibin
 * @Desc: 无极仙宫公共数据管理
 * @Date: 2023/12/5 10:12
 */

package inffairyplacemgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"

	"golang.org/x/exp/slices"

	"github.com/gzjjyz/logger"
)

var localInfFairyPlaceMgrIns = &LocalInfFairyPlaceMgr{}

type LocalInfFairyPlaceMgr struct {
	localInfo *pb3.LocalInfFairyPlaceInfo
}

var crossInfFairyPlaceJobInfo = make(map[uint32]uint64)

func localInfFairyPlaceMgrOnServerInit(args ...interface{}) {
	info := gshare.GetStaticVar().LocalInfFairyPlaceInfo
	if info == nil {
		info = &pb3.LocalInfFairyPlaceInfo{}
		gshare.GetStaticVar().LocalInfFairyPlaceInfo = info
	}
	GetLocalInfFairyPlaceMgr().localInfo = info

	if info.JobInfo == nil {
		info.JobInfo = make(map[uint32]uint64)
	}

	if info.JobRewardInfo == nil {
		info.JobRewardInfo = make(map[uint32]bool)
	}

	if info.SignatureInfo == nil {
		info.SignatureInfo = make(map[uint32]string)
	}

	if info.AttrInfo == nil {
		info.AttrInfo = make(map[uint32]uint64)
	}

	js, _ := json.Marshal(info)
	logger.LogDebug("localInfFairyPlaceMgrOnServerInit %s", string(js))
}

func GetLocalInfFairyPlaceMgr() *LocalInfFairyPlaceMgr {
	if localInfFairyPlaceMgrIns == nil {
		localInfFairyPlaceMgrIns = &LocalInfFairyPlaceMgr{}
	}
	return localInfFairyPlaceMgrIns
}

func (mgr *LocalInfFairyPlaceMgr) GetLocalInfo() *pb3.LocalInfFairyPlaceInfo {
	return mgr.localInfo
}

func (mgr *LocalInfFairyPlaceMgr) SetLocalInfo(info *pb3.LocalInfFairyPlaceInfo) {
	mgr.localInfo = info
}

func (mgr *LocalInfFairyPlaceMgr) Refresh() {
	mgr.localInfo = &pb3.LocalInfFairyPlaceInfo{
		JobInfo:       make(map[uint32]uint64),
		JobRewardInfo: make(map[uint32]bool),
		SignatureInfo: make(map[uint32]string),
		AttrInfo:      make(map[uint32]uint64),
	}

	gshare.GetStaticVar().LocalInfFairyPlaceInfo = mgr.localInfo
}

func (mgr *LocalInfFairyPlaceMgr) SendJobReward(jobId uint32) error {
	jobInfoByActorId := mgr.localInfo.JobInfo[jobId]
	if jobInfoByActorId == 0 {
		return fmt.Errorf("jobInfo is nil for jobId %d", jobId)
	}

	jobConf := jsondata.GetLocalInfFairyPlaceJobConfig(jobId)
	if jobConf == nil {
		return errors.New("jobConf is nil")
	}

	if len(jobConf.Rewards) == 0 {
		return errors.New("rewards is nil")
	}

	rewardState := GetLocalInfFairyPlaceMgr().GetLocalInfo().JobRewardInfo[jobId]
	if rewardState {
		return errors.New("already rewarded")
	}

	GetLocalInfFairyPlaceMgr().GetLocalInfo().JobRewardInfo[jobId] = true

	mailmgr.SendMailToActor(jobInfoByActorId, &mailargs.SendMailSt{
		ConfId:  common.Mail_LocalInfFairyPlaceJobReward,
		Rewards: jobConf.Rewards,
	})
	return nil
}

func (mgr *LocalInfFairyPlaceMgr) GetMasterLeaderId() uint64 {
	localInfo := mgr.GetLocalInfo()
	if localInfo == nil {
		return 0
	}

	jobInfo := localInfo.GetJobInfo()
	if jobInfo == nil {
		return 0
	}

	masterActorId, ok := jobInfo[custom_id.LocalInfFairyPlaceJob_Master]
	if !ok {
		return 0
	}

	return masterActorId
}

func (mgr *LocalInfFairyPlaceMgr) PackLocalInfFairyPlaceInfoToClient() *pb3.LocalInfFairyPlaceInfoToClient {
	localInfo := mgr.localInfo
	var jobInfoMap = make(map[uint32]*pb3.PlayerDataBase)
	var attrInfoMap = make(map[uint32]*pb3.OfflineProperty)
	for k, actorId := range localInfo.JobInfo {
		dataBase := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		offlineProperty := manager.GetData(actorId, gshare.ActorDataProperty).(*pb3.OfflineProperty)
		jobInfoMap[k] = dataBase
		attrInfoMap[k] = offlineProperty
	}
	return &pb3.LocalInfFairyPlaceInfoToClient{
		JobInfo:       jobInfoMap,
		JobRewardInfo: localInfo.JobRewardInfo,
		SignatureInfo: localInfo.SignatureInfo,
		AttrInfo:      attrInfoMap,
	}
}

func (mgr *LocalInfFairyPlaceMgr) OnMarry(playerId, marryId uint64) {
	localInfo := mgr.GetLocalInfo()
	if localInfo == nil {
		return
	}
	jobInfo := localInfo.GetJobInfo()
	if jobInfo == nil {
		return
	}

	masterActorId, ok := jobInfo[custom_id.LocalInfFairyPlaceJob_Master]
	if !ok {
		return
	}
	if masterActorId != playerId {
		return
	}

	attrInfo := localInfo.GetAttrInfo()
	if attrInfo == nil {
		return
	}
	attrInfo[custom_id.LocalInfFairyPlaceJob_Master] = masterActorId

	// 伴侣基础属性和外观属性
	if _, ok := jobInfo[custom_id.LocalInfFairyPlaceJob_Emperor]; ok {
		return
	}

	emperor, ok := manager.GetData(marryId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if ok || emperor != nil {
		jobInfo[custom_id.LocalInfFairyPlaceJob_Emperor] = marryId
		attrInfo[custom_id.LocalInfFairyPlaceJob_Emperor] = marryId
	}

	engine.Broadcast(chatdef.CIWorld, 0, 169, 0, &pb3.S2C_169_0{
		LocalInf: mgr.PackLocalInfFairyPlaceInfoToClient(),
	}, 0)
}

func (mgr *LocalInfFairyPlaceMgr) OnDivorce(playerId uint64) {
	localInfo := mgr.GetLocalInfo()
	if localInfo == nil {
		return
	}

	jobInfo := localInfo.GetJobInfo()
	attrInfo := localInfo.GetAttrInfo()
	if jobInfo == nil || attrInfo == nil {
		return
	}

	masterActorId, ok := jobInfo[custom_id.LocalInfFairyPlaceJob_Master]
	if !ok || masterActorId != playerId {
		return
	}

	if _, ok := jobInfo[custom_id.LocalInfFairyPlaceJob_Emperor]; !ok {
		return
	}

	delete(jobInfo, custom_id.LocalInfFairyPlaceJob_Emperor)
	delete(attrInfo, custom_id.LocalInfFairyPlaceJob_Emperor)

	engine.Broadcast(chatdef.CIWorld, 0, 169, 0, &pb3.S2C_169_0{
		LocalInf: mgr.PackLocalInfFairyPlaceInfoToClient(),
	}, 0)
}

func onF2GInfFairyPlaceSyncPlayerInfoReq(buf []byte) {
	var req pb3.F2GInfFairyPlaceSyncPlayerInfoReq
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onF2GInfFairyPlaceSyncPlayerInfoReq unmarshal failed err: %s", err)
		return
	}

	res := &pb3.G2FInfFairyPlaceSyncPlayerInfoRes{
		JobId: req.JobId,
	}
	playerInfo, ok := manager.GetData(req.PlayerId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		logger.LogError("onF2GInfFairyPlaceSyncPlayerInfoReq get player data for %d failed", req.PlayerId)
		return
	}

	res.BaseData = &pb3.CrossFightActorBaseData{
		PfId:     engine.GetPfId(),
		SrvId:    engine.GetServerId(),
		ActorId:  req.PlayerId,
		BaseData: playerInfo,
	}

	if req.JobId == custom_id.CrossInfFairyPlaceJob_Master {
		emperorPlayer, ok := manager.GetData(playerInfo.MarryId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			return
		}

		res.Cp = &pb3.CrossFightActorBaseData{
			PfId:     engine.GetPfId(),
			SrvId:    engine.GetServerId(),
			ActorId:  emperorPlayer.MarryId,
			BaseData: emperorPlayer,
		}
	}

	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FInfFairyPlaceSyncPlayerInfoRes, res)
}

func infFairyPlaceOnGuildSecretOver(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	GetLocalInfFairyPlaceMgr().Refresh()

	leaderGuildId := args[0].(uint64)

	guildInfo := guildmgr.GetGuildById(leaderGuildId)
	if guildInfo == nil {
		logger.LogError("guildInfo is nil")
		return
	}

	localInfo := GetLocalInfFairyPlaceMgr().GetLocalInfo()

	// LocalInfFairyPlaceJob_Master
	guildLeaderId := guildInfo.BasicInfo.LeaderId
	master, ok := manager.GetData(guildLeaderId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok || master == nil {
		logger.LogError("master is nil")
		return
	}

	localInfo.JobInfo[custom_id.LocalInfFairyPlaceJob_Master] = master.GetId()
	localInfo.AttrInfo[custom_id.LocalInfFairyPlaceJob_Master] = guildLeaderId

	// LocalInfFairyPlaceJob_Emperor
	emperor, ok := manager.GetData(master.MarryId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if ok || emperor != nil {
		localInfo.JobInfo[custom_id.LocalInfFairyPlaceJob_Emperor] = emperor.GetId()
		localInfo.AttrInfo[custom_id.LocalInfFairyPlaceJob_Emperor] = emperor.GetId()
	}

	rankItem := manager.GRankMgrIns.GetRankByType(gshare.RankTypePower).GetList(1, 10)

	rankItem = slices.DeleteFunc(rankItem, func(item *pb3.OneRankItem) bool {
		if item.Id == guildLeaderId {
			return true
		}

		if emperor != nil && emperor.Id == item.Id {
			return true
		}

		return false
	})

	for jobId, rank := range custom_id.LocalInfFairyPlaceJobMapToRank {
		if len(rankItem) < int(rank) {
			continue
		}

		player, ok := manager.GetData(rankItem[rank-1].Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if ok || player != nil {
			localInfo.JobInfo[jobId] = player.GetId()
			localInfo.AttrInfo[jobId] = player.GetId()
		}
	}

	if err := GetLocalInfFairyPlaceMgr().SendJobReward(custom_id.LocalInfFairyPlaceJob_Master); err != nil {
		logger.LogError("SendJobReward failed err: %s", err)
	}

	engine.Broadcast(chatdef.CIWorld, 0, 169, 0, &pb3.S2C_169_0{
		LocalInf: GetLocalInfFairyPlaceMgr().PackLocalInfFairyPlaceInfoToClient(),
	}, 0)
}

func GetCrossInfFairyPlaceJobInfo() map[uint32]uint64 {
	return crossInfFairyPlaceJobInfo
}

func clearCrossInfFairyPlaceJobInfo(buf []byte) {
	crossInfFairyPlaceJobInfo = make(map[uint32]uint64)
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, localInfFairyPlaceMgrOnServerInit)
	event.RegSysEvent(custom_id.SeGuildSecretOver, infFairyPlaceOnGuildSecretOver)

	gmevent.Register("localInfFairyPlace.FakeData", func(player iface.IPlayer, _ ...string) bool {

		GetLocalInfFairyPlaceMgr().Refresh()

		rankItem := manager.GRankMgrIns.GetRankByType(gshare.RankTypePower).GetList(1, 10)
		if len(rankItem) == 0 {
			return false
		}

		// LocalInfFairyPlaceJob_Master
		leaderId := rankItem[0].Id
		master, ok := manager.GetData(leaderId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok || master == nil {
			logger.LogError("master is nil")
			return false
		}
		GetLocalInfFairyPlaceMgr().GetLocalInfo().JobInfo[custom_id.LocalInfFairyPlaceJob_Master] = leaderId

		// LocalInfFairyPlaceJob_Emperor
		emperor, ok := manager.GetData(master.MarryId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if ok || emperor != nil {
			GetLocalInfFairyPlaceMgr().GetLocalInfo().JobInfo[custom_id.LocalInfFairyPlaceJob_Emperor] = emperor.GetId()
		}

		rankItem = slices.DeleteFunc(rankItem, func(item *pb3.OneRankItem) bool {
			if item.Id == leaderId {
				return true
			}

			if emperor != nil && emperor.Id == item.Id {
				return true
			}

			return false
		})

		for jobId, rank := range custom_id.LocalInfFairyPlaceJobMapToRank {
			if len(rankItem) < int(rank) {
				continue
			}

			player, ok := manager.GetData(rankItem[rank-1].Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
			if !ok || player == nil {
				logger.LogError("player is nil")
				continue
			}

			GetLocalInfFairyPlaceMgr().GetLocalInfo().JobInfo[jobId] = rankItem[rank-1].Id
		}

		gshare.GetStaticVar().LocalInfFairyPlaceInfo = localInfFairyPlaceMgrIns.GetLocalInfo()

		engine.Broadcast(chatdef.CIWorld, 0, 169, 0, &pb3.S2C_169_0{
			LocalInf: GetLocalInfFairyPlaceMgr().PackLocalInfFairyPlaceInfoToClient(),
		}, 0)
		return true
	}, 1)

	gmevent.Register("crossInfFairyPlace.FakeData", func(player iface.IPlayer, args ...string) bool {
		engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FCrossInfFairyPlaceFakeDataReq, nil)
		return true
	}, 1)

	engine.RegisterSysCall(sysfuncid.C2GSyncCrossInfFairyPlace, onF2GInfFairyPlaceSyncPlayerInfoReq)

	engine.RegisterSysCall(sysfuncid.FCDisconnectCross, clearCrossInfFairyPlaceJobInfo)
}
