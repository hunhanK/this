/**
 * @Author: lzp
 * @Date: 2024/12/20
 * @Desc:
**/

package ancienttower

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/manager"
	"math"
)

type TeamFuBenHandler struct {
	teammgr.TeamFuBenBaseHandler
}

func (t *TeamFuBenHandler) OnChange(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return
	}

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.AncientTowerTData == nil {
		fbSet.AncientTowerTData = &pb3.AncientTowerTeamData{}
	}

	aTowerSet := fbSet.AncientTowerTData
	if aTowerSet.ActorsData == nil {
		aTowerSet.ActorsData = make(map[uint64]*pb3.AncientTowerActorData)
	}

	for _, mem := range teamPb.Members {
		id := mem.PlayerInfo.Id
		_, ok := aTowerSet.ActorsData[id]
		if !ok {
			aData := &pb3.AncientTowerActorData{}
			aData.ActorId = id
			if mem.IsRobot {
				aData.LeftTimes = conf.DayTimes
			} else {
				player := manager.GetPlayerPtrById(id)
				if player != nil {
					aData.LeftTimes = player.GetExtraAttrU32(attrdef.AncientTowerTimes)
				}
			}
			aTowerSet.ActorsData[id] = aData
		}
	}

	t.updateLayer(teamId)
	t.updateFightValue(teamId)
}

func (t *TeamFuBenHandler) OnMemberChange(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	aTowerSet := getTeamSettingData(teamId)
	if nil == aTowerSet {
		return
	}

	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return
	}

	// 删除退出队伍的玩家
	var delActorIds []uint64
	for _, tActor := range aTowerSet.ActorsData {
		isDel := true
		for _, member := range teamPb.GetMembers() {
			if tActor.ActorId == member.PlayerInfo.Id {
				isDel = false
				break
			}
		}
		if isDel {
			delActorIds = append(delActorIds, tActor.ActorId)
		}
	}

	for _, key := range delActorIds {
		delete(aTowerSet.ActorsData, key)
	}

	// 增加进入队伍的玩家
	for _, mem := range teamPb.Members {
		id := mem.PlayerInfo.Id
		_, ok := aTowerSet.ActorsData[id]
		if !ok {
			aData := &pb3.AncientTowerActorData{}
			aData.ActorId = id
			if mem.IsRobot {
				aData.LeftTimes = conf.DayTimes
			} else {
				player := manager.GetPlayerPtrById(id)
				if player != nil {
					aData.LeftTimes = player.GetExtraAttrU32(attrdef.AncientTowerTimes)
				}
			}
			aTowerSet.ActorsData[id] = aData
		}
	}

	t.updateLayer(teamId)
	t.updateFightValue(teamId)
}

func (t *TeamFuBenHandler) OnConsultEnter(teamId uint64, player iface.IPlayer, args string) {
	conf := jsondata.GetAncientTowerCommonConf()
	if !player.CheckEnterFb(conf.FbId) {
		return
	}

	aTowerSet := getTeamSettingData(teamId)
	tActor := aTowerSet.ActorsData[player.GetId()]
	if tActor == nil {
		return
	}

	if player.GetExtraAttrU32(attrdef.AncientTowerTimes) < tActor.CombineTimes {
		return
	}

	err := teammgr.MbConsultCheckSuccess(player.GetId(), player.GetTeamId())
	if err != nil {
		player.LogError("MbConsultCheckSuccess failed %s", err)
	}
}

func (t *TeamFuBenHandler) OnEnterCheck(teamId uint64, player iface.IPlayer, args string) {
	teammgr.MbEnterFbReady(player.GetId(), teamId)
}

func (t *TeamFuBenHandler) OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error {
	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("ancient tower common conf not found")
	}
	teamPb, err := teammgr.GetTeamPb(teamId)
	if teamPb == nil {
		return neterror.InternalError("team get err: %v", err)
	}

	aTowerSet := getTeamSettingData(teamId)
	canMatch := false
	for _, mem := range teamPb.Members {
		if mem.IsRobot {
			continue
		}
		playerId := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}
		tActor, ok := aTowerSet.ActorsData[playerId]
		if !ok {
			continue
		}
		if player.GetExtraAttrU32(attrdef.AncientTowerTimes) < tActor.CombineTimes {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TimesNotEnough))
			return neterror.ParamsInvalidError("combine times limit")
		}
		if player.GetExtraAttrU32(attrdef.AncientTowerTimes) > 0 {
			canMatch = true
		}
	}
	if !canMatch {
		teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TimesNotEnough))
		return neterror.ParamsInvalidError("times limit")
	}

	t.updateLayer(teamId)
	t.updateFightValue(teamId)

	retArgs.ATowerFbArgs = &pb3.ATowerFbTeamArgs{}
	retArgs.ATowerFbArgs.Actors = make(map[uint64]*pb3.AncientTowerActorData)
	for _, mem := range teamPb.Members {
		playerId := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}
		retArgs.ATowerFbArgs.Actors[playerId] = &pb3.AncientTowerActorData{
			ActorId:   playerId,
			LeftTimes: player.GetExtraAttrU32(attrdef.AncientTowerTimes),
		}
	}
	return nil
}

func (t *TeamFuBenHandler) updateLayer(teamId uint64) {
	aTowerSet := getTeamSettingData(teamId)
	if aTowerSet == nil {
		return
	}
	layer := getTeamAncientTowerLayer(teamId)
	aTowerSet.Layer = layer
}

func (t *TeamFuBenHandler) updateFightValue(teamId uint64) {
	aTowerSet := getTeamSettingData(teamId)
	if aTowerSet == nil {
		return
	}
	fightValue := getTeamFightValue(teamId)
	aTowerSet.FightValue = fightValue
}

func getTeamAncientTowerLayer(teamId uint64) uint32 {
	aTowerSet := getTeamSettingData(teamId)

	minLayer := uint32(math.MaxUint32)
	for actorId := range aTowerSet.ActorsData {
		player := manager.GetPlayerPtrById(actorId)
		if player == nil {
			continue
		}
		layer := player.GetExtraAttrU32(attrdef.AncientTowerLayer)
		if layer < minLayer {
			minLayer = layer
		}
	}
	layer := utils.MinUInt32(jsondata.GetAncientTowerMaxLayer(), minLayer+1)
	return layer
}

func getTeamFightValue(teamId uint64) int64 {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return 0
	}

	var maxPower int64
	for _, mem := range teamPb.Members {
		if mem.IsRobot {
			continue
		}

		id := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(id)
		if player == nil {
			continue
		}
		power := player.GetExtraAttr(attrdef.FightValue)
		if power > maxPower {
			maxPower = power
		}
	}

	var sumPower float64
	for _, mem := range teamPb.Members {
		if mem.IsRobot {
			sumPower += float64(maxPower) * 0.25
		} else {
			player := manager.GetPlayerPtrById(mem.PlayerInfo.Id)
			if player != nil {
				power := player.GetExtraAttr(attrdef.FightValue)
				sumPower += float64(power)
			}
		}
	}

	return int64(sumPower / float64(len(teamPb.Members)))
}

func getTeamSettingData(teamId uint64) *pb3.AncientTowerTeamData {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if nil == fbSet {
		return nil
	}
	if nil == fbSet.AncientTowerTData {
		fbSet.AncientTowerTData = &pb3.AncientTowerTeamData{}
	}
	if nil == fbSet.AncientTowerTData.ActorsData {
		fbSet.AncientTowerTData.ActorsData = make(map[uint64]*pb3.AncientTowerActorData)
	}
	return fbSet.AncientTowerTData
}

func afterReloadConf(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return
	}

	isReload := args[0].(bool)
	if isReload {
		return
	}
	teammgr.RegTeamFbHandler(conf.FbId, &TeamFuBenHandler{})
}

func init() {
	event.RegSysEvent(custom_id.SeReloadJson, afterReloadConf)
}
