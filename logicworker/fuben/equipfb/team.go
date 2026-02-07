/**
 * @Author: lzp
 * @Date: 2024/12/19
 * @Desc:
**/

package equipfb

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/manager"
)

type TeamFuBenHandler struct {
	teammgr.TeamFuBenBaseHandler
}

func (t *TeamFuBenHandler) OnChange(teamId uint64) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.EquipFbTData == nil {
		fbSet.EquipFbTData = &pb3.EquipFbTeamData{}
	}
}

func (t *TeamFuBenHandler) OnMemberChange(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.EquipFbTData == nil {
		fbSet.EquipFbTData = &pb3.EquipFbTeamData{}
	}

	equipFbTData := fbSet.EquipFbTData
	if equipFbTData.EFbActorData == nil {
		equipFbTData.EFbActorData = make(map[uint64]*pb3.EquipFbTeamActorData)
	}

	// 删除退出队伍的玩家
	var delActorIds []uint64
	for _, tActor := range equipFbTData.EFbActorData {
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
		delete(equipFbTData.EFbActorData, key)
	}

	// 增加进入队伍的玩家
	for _, member := range teamPb.GetMembers() {
		actorId := member.PlayerInfo.Id
		if _, ok := equipFbTData.EFbActorData[actorId]; !ok {
			equipFbTData.EFbActorData[actorId] = &pb3.EquipFbTeamActorData{
				ActorId: actorId,
			}
		}
	}
}

func (t *TeamFuBenHandler) OnConsultEnter(teamId uint64, player iface.IPlayer, args string) {
	err := teammgr.MbConsultCheckSuccess(player.GetId(), teamId)
	if err != nil {
		player.LogError("MbConsultCheckSuccess failed %s", err)
	}
}

func (t *TeamFuBenHandler) OnEnterCheck(teamId uint64, player iface.IPlayer, args string) {
	teammgr.MbEnterFbReady(player.GetId(), teamId)
}

func (t *TeamFuBenHandler) OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	fbId := teammgr.GetTeamFbSetting(teamId).GroupFubenId
	if fbId == 0 {
		return neterror.InternalError("not select fuBen")
	}

	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("equip fb common config not found")
	}

	fbSet := teammgr.GetTeamFbSetting(teamId)
	eData := fbSet.EquipFbTData
	if eData == nil {
		return neterror.InternalError("not equip fb team data")
	}

	canMatch := false
	for _, member := range teamPb.GetMembers() {
		playerId := member.PlayerInfo.Id
		mPlayer := manager.GetPlayerPtrById(playerId)
		if mPlayer == nil {
			continue
		}
		if mPlayer.GetExtraAttrU32(attrdef.IsLost) == 1 {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder6, mPlayer.GetName()))
			return neterror.InternalError("team member is lost")
		}
		if !mPlayer.GetSysOpen(sysdef.SiEquipFuBen) {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder))
			return neterror.InternalError("team member equip fuBen is not open")
		}
		if !mPlayer.CheckEnterFb(conf.FbId) {
			fbConf := jsondata.GetFbConf(conf.FbId)
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder5, mPlayer.GetName(), fbConf.FbName))
			return neterror.InternalError("team member cannot enter equipFuBen")
		}
		if mPlayer.GetExtraAttrU32(attrdef.EquipFbTimes) > 0 {
			canMatch = true
		}
	}
	if !canMatch {
		teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder3))
		return neterror.InternalError("team member times not enough")
	}

	pkgEquipFbArgs(teamId, retArgs)
	return nil
}

func pkgEquipFbArgs(teamId uint64, retArgs *pb3.EnterTeamFbRetArgs) {
	teamPb, _ := teammgr.GetTeamPb(teamId)
	retArgs.EquipFbArgs = &pb3.EquipFbTeamArgs{}
	retArgs.EquipFbArgs.Actors = make(map[uint64]*pb3.EquipFbTeamActorData)
	for _, mem := range teamPb.Members {
		playerId := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}
		retArgs.EquipFbArgs.Actors[playerId] = &pb3.EquipFbTeamActorData{
			LeftTimes: player.GetExtraAttrU32(attrdef.EquipFbTimes),
		}
	}
	return
}

func afterReloadConf(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	conf := jsondata.GetEquipFbCommonConf()
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
