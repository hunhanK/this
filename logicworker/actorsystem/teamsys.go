/**
 * @Author: lzp
 * @Date: 2024/12/17
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"
)

type TeamSys struct {
	Base
}

func (s *TeamSys) OnLogin() {
	s.s2cInfo()
	s.owner.SetExtraAttr(attrdef.TeamId, attrdef.AttrValueAlias(s.owner.GetTeamId()))
}

func (s *TeamSys) OnReconnect() {
	s.s2cInfo()
}

func (s *TeamSys) OnOpen() {
	s.s2cInfo()
}

func (s *TeamSys) s2cInfo() {
	teamId := s.GetOwner().GetTeamId()
	if teamId == 0 {
		return
	}
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}
	s.SendProto3(28, 0, &pb3.S2C_28_0{
		Team: teamPb,
	})
}

func (s *TeamSys) c2sCreateTeam(msg *base.Message) error {
	var req pb3.C2S_28_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()

	if teamId != 0 {
		return neterror.ParamsInvalidError("already in team")
	}

	if !checkCanChangeTeamFb(player, req.Settings.GroupFubenId) {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	createArg := &teammgr.TeamCreateSt{
		FbSettings: req.Settings,
	}

	teamId, err := teammgr.CreateTeam(player, createArg)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	player.SendProto3(28, 1, &pb3.S2C_28_1{
		Team: teamPb,
	})
	return nil
}

func (s *TeamSys) c2sTeamList(msg *base.Message) error {
	teamList := teammgr.TeamList()
	player := s.GetOwner()
	player.SendProto3(28, 2, &pb3.S2C_28_2{
		Teams: teamList,
	})
	return nil
}

// ************************** 邀请组队 **************************

// 邀请组队
func (s *TeamSys) c2sInvite(msg *base.Message) error {
	var req pb3.C2S_28_40
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	targetPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if targetPlayer == nil {
		return nil
	}

	state := teammgr.CheckInvitable(teamId, targetPlayer)
	switch state {
	case teammgr.InvitePlayerFailedInOtherTeam:
		player.SendTipMsg(tipmsgid.TpTeamInviteFailed_InOtherTeam)
	case teammgr.InvitePlayerFailedInTeam:
		player.SendTipMsg(tipmsgid.TpTeamInviteFailed_InCurrentTeam)
	case teammgr.InvitePlayerFailedNotInWildFb:
		player.SendTipMsg(tipmsgid.TpTeamInviteFailed_InFuben)
	case teammgr.InvitePlayerFailedLevelLimit:
		player.SendTipMsg(tipmsgid.TpTeamInviteFailed_LevelLimit)
	case teammgr.InvitePlayerFailedTeamFull:
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_TeamFull)
	case teammgr.InvitePlayerFailedTeamInFb:
		player.SendTipMsg(tipmsgid.TpTeamApplyJoinTeamFailed_TeamInFuben)
	}

	if state == teammgr.InvitePlayerSuccess {
		targetPlayer.SendProto3(28, 41, &pb3.S2C_28_41{
			Team:      teamPb,
			Timestamp: time.Now().Unix(),
			Name:      player.GetName(),
		})
		player.SendTipMsg(tipmsgid.TpTeamInviteSuccess)
	}

	return nil
}

func (s *TeamSys) c2sQuickInvite(msg *base.Message) error {
	var req pb3.C2S_28_43
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	var failedPlayerIds []uint64
	for _, playerId := range req.PlayerIds {
		targetPlayer := manager.GetPlayerPtrById(uint64(playerId))
		if targetPlayer == nil {
			failedPlayerIds = append(failedPlayerIds, playerId)
			continue
		}

		targetPlayer.SendProto3(28, 41, &pb3.S2C_28_41{
			Team:      teamPb,
			Timestamp: time.Now().Unix(),
			Name:      player.GetName(),
		})
	}

	player.SendProto3(28, 43, &pb3.S2C_28_43{
		PlayerIds: failedPlayerIds,
	})

	return nil
}

func (s *TeamSys) c2sOnceInvite(msg *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	commonConf := jsondata.GetTeamCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	if len(teamPb.GetMembers()) == int(commonConf.MaxMember) {
		return neterror.ParamsInvalidError("members is full")
	}

	checkInTeam := func(team *pb3.TeamSt, player iface.IPlayer) bool {
		members := team.GetMembers()
		for _, mem := range members {
			if mem.PlayerInfo.Id == player.GetId() {
				return true
			}
		}
		return false
	}

	manager.AllOnlinePlayerDo(func(tmpPlayer iface.IPlayer) {
		if tmpPlayer.GetId() == player.GetId() {
			return
		}
		if checkInTeam(teamPb, tmpPlayer) {
			return
		}
		tmpPlayer.SendProto3(28, 41, &pb3.S2C_28_41{
			Team:      teamPb,
			Timestamp: time.Now().Unix(),
			Name:      player.GetName(),
		})
	})

	channel := chatdef.CITeamUp

	message := fmt.Sprintf(jsondata.GlobalString("teamShout2"), player.GetName(), "%s")
	if fbId := teammgr.GetTeamFbId(teamId); fbId > 0 {
		fbConf := jsondata.GetFbConf(fbId)
		if fbConf != nil {
			message = fmt.Sprintf(jsondata.GlobalString("teamShout1"), player.GetName(), fbConf.FbName, "%s")
		}
	}

	chatMsg := &pb3.C2S_5_1{
		Msg:     message,
		Channel: uint32(channel),
		Params:  fmt.Sprintf("6~%d", teamId),
	}
	player.ChannelChat(chatMsg, false)

	return nil
}

// 获取附近的玩家作为邀请玩家
func (s *TeamSys) c2sNearByInviteList(msg *base.Message) error {
	var req pb3.C2S_28_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()

	err := player.CallActorFunc(actorfuncid.G2FNearByInviteListReq, nil)
	if err != nil {
		player.LogError("err: %v", err)
	}
	return nil
}

// 玩家同意组队邀请
func (s *TeamSys) c2sAgreeInvite(msg *base.Message) error {
	var req pb3.C2S_28_42
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamCreateTime, err := teammgr.GetTeamCreateTime(req.TeamId)
	if err != nil || req.Timestamp < teamCreateTime {
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_TeamDestroyed)
		return nil
	}

	state, _ := teammgr.CheckJoin(req.TeamId, player)

	switch state {
	case teammgr.CheckJoinStateFailedLevelLimit:
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_LevelLimit)
		return nil
	case teammgr.CheckJoinStateFailedInTeam:
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_InOtherTeam)
		return nil
	case teammgr.CheckJoinStateFailedTeamFull:
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_TeamFull)
		return nil
	case teammgr.CheckJoinStateFailedNotInWildFb:
		player.SendTipMsg(tipmsgid.TpTeamAgreeInviteFailed_FubenForbitJoin)
		return nil
	}

	if state != teammgr.CheckJoinStateSuccess {
		return neterror.InternalError("check enter state failed")
	}

	teamPb, _ := teammgr.GetTeamPb(req.TeamId)
	if !checkCanChangeTeamFb(player, teamPb.Settings.FubenSetting.GroupFubenId) {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	tState, err := teammgr.GetTeamState(req.TeamId)
	if nil != err {
		return neterror.Wrap(err)
	}
	if tState != teammgr.TeamStateWaiting {
		player.SendTipMsg(tipmsgid.TpTeamApplyJoinTeamFailed_TeamInFuben)
		return nil
	}

	err = teammgr.AddMemberToTeam(req.TeamId, player, player.GetId())
	if err != nil {
		return neterror.InternalError("team add member err: %v", err)
	}

	player.SendProto3(28, 23, &pb3.S2C_28_23{
		Team: teamPb,
	})
	return nil
}

// ************************** 申请入队 **************************
// 申请入队
func (s *TeamSys) c2sApplyJoinTeam(msg *base.Message) error {
	var req pb3.C2S_28_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamPb, err := teammgr.GetTeamPb(req.TeamId)
	if err != nil {
		player.SendTipMsg(tipmsgid.TpTeamApplyJoinTeamFailed_TeamDestroyed)
		return nil
	}

	if !checkCanChangeTeamFb(player, teamPb.Settings.FubenSetting.GroupFubenId) {
		player.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	state := teammgr.CheckApplicable(req.TeamId, player)
	player.SendProto3(28, 22, &pb3.S2C_28_22{
		State:   uint32(state),
		Setting: teamPb.Settings,
		TeamId:  teamPb.TeamId,
	})

	if state != teammgr.ApplyTeamSuccess {
		return nil
	}

	if teamPb.Settings.AutoAccept {
		if state != teammgr.ApplyTeamSuccess {
			return nil
		}

		if err := teammgr.AddMemberToTeam(teamPb.TeamId, player); err != nil {
			return neterror.InternalError(err.Error())
		}

		player.SendProto3(28, 23, &pb3.S2C_28_23{
			Team: teamPb,
		})
		player.SendTipMsg(tipmsgid.TpTeamApplyJoinTeamFailed_InTeam)

		return nil
	}

	if err := teammgr.ApplyEnterTeam(req.TeamId, player); err != nil {
		return neterror.InternalError(err.Error())
	}
	return nil
}

// 接受申请
func (s *TeamSys) c2sAgreeEnterTeam(msg *base.Message) error {
	var req pb3.C2S_28_25
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	targetPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if targetPlayer == nil {
		player.SendTipMsg(tipmsgid.TpTeamAgreeApplyFailed_PlayerOffline)
		return nil
	}

	state, _ := teammgr.CheckJoin(teamId, targetPlayer)
	switch state {
	case teammgr.CheckJoinStateFailedLevelLimit:
		player.SendTipMsg(tipmsgid.TpTeamAgreeApplyFailed_LevelLimit)
		return nil
	case teammgr.CheckJoinStateFailedInTeam:
		player.SendTipMsg(tipmsgid.TpTeamAgreeApplyFailed_InOtherTeam)
		return nil
	case teammgr.CheckJoinStateFailedTeamFull:
		player.SendTipMsg(tipmsgid.TpTeamAgreeApplyFailed_TeamFull)
		return nil
	case teammgr.CheckJoinStateFailedNotInWildFb:
		player.SendTipMsg(tipmsgid.TpTeamAgreeApplyFailed_FubenForbitJoin)
		return nil
	}

	if state != teammgr.CheckJoinStateSuccess {
		return neterror.InternalError("check enter state failed")
	}

	if err := teammgr.AddMemberToTeam(teamId, targetPlayer, req.PlayerId); err != nil {
		return neterror.InternalError(err.Error())
	}

	targetPlayer.SendProto3(28, 23, &pb3.S2C_28_23{
		Team: teamPb,
	})
	return nil
}

// 拒绝入队申请
func (s *TeamSys) c2sDisAgreeEnterTeam(msg *base.Message) error {
	var req pb3.C2S_28_26
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	targetPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if targetPlayer == nil {
		return nil
	}

	targetPlayer.SendTipMsg(tipmsgid.TpTeamDisAgreeEnterTeamApply, player.GetName())

	if req.NeverAccept {
		err = teammgr.AddNeverAccept(teamId, req.PlayerId)
		if err != nil {
			return neterror.InternalError("add accept err: %v", err)
		}
	}

	return nil
}

func (s *TeamSys) c2sRefuseInvite(msg *base.Message) error {
	var req pb3.C2S_28_27
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	teamCreateTime, err := teammgr.GetTeamCreateTime(req.TeamId)
	if err != nil || req.Timestamp < teamCreateTime {
		return nil
	}

	player := s.GetOwner()
	teamPb, _ := teammgr.GetTeamPb(req.TeamId)
	leader := manager.GetPlayerPtrById(teamPb.LeaderId)
	if leader == nil {
		return nil
	}

	leader.SendTipMsg(tipmsgid.TpTeamDisAgreeEnterTeamInvite, player.GetName())
	return nil
}

// ************************************ 队伍变更 ************************************

// 踢出队伍
func (s *TeamSys) c2sKickOut(msg *base.Message) error {
	var req pb3.C2S_28_84
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	state, err := teammgr.GetTeamState(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	if state == teammgr.TeamStateFight {
		player.SendTipMsg(tipmsgid.TeamReminder2)
		return neterror.ParamsInvalidError("in fight not kick out member")
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	if err := teammgr.PlayerExitTeam(teamId, req.PlayerId); err != nil {
		return neterror.InternalError(err.Error())
	}

	targetPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if targetPlayer != nil {
		targetPlayer.SendTipMsg(tipmsgid.TpTeamRemovedFromTeam)
		targetPlayer.SendProto3(28, 84, &pb3.S2C_28_84{})
	}

	return nil
}

// 退出队伍
func (s *TeamSys) c2sExitTeam(msg *base.Message) error {
	var req pb3.C2S_28_85
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	state, err := teammgr.GetTeamState(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}
	if state == teammgr.TeamStateFight {
		player.SendTipMsg(tipmsgid.TeamReminder2)
		return neterror.ParamsInvalidError("in fight not exit team")
	}

	if err := teammgr.PlayerExitTeam(teamId, player.GetId()); err != nil {
		return neterror.InternalError(err.Error())
	}

	player.SendTipMsg(tipmsgid.TpTeamPlayerExitTeam)
	player.SendProto3(28, 85, &pb3.S2C_28_85{})

	return nil
}

// ************************ 队长移交/申请 ***************************************
// 向队员移交队长身份
func (s *TeamSys) c2sGrantLeader(msg *base.Message) error {
	var req pb3.C2S_28_90
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	targetPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if targetPlayer == nil {
		player.SendTipMsg(tipmsgid.TpTeamGrantLeaderPlayerOffline)
		return nil
	}

	if err := teammgr.ChangeLeader(teamId, req.PlayerId); err != nil {
		return neterror.InternalError(err.Error())
	}

	return nil
}

func (s *TeamSys) c2sApplyLeader(msg *base.Message) error {
	var req pb3.C2S_28_93
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if isLeader {
		return neterror.ParamsInvalidError("already leader")
	}

	leader := manager.GetPlayerPtrById(teamPb.LeaderId)
	if leader == nil {
		return nil
	}

	teammgr.ApplyLeader(teamId, player.GetId())
	leader.SendProto3(28, 94, &pb3.S2C_28_94{
		PlayerId: player.GetId(),
	})
	return nil
}

func (s *TeamSys) c2sProcessApplyLeaderMsg(msg *base.Message) error {
	var req pb3.C2S_28_95
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}

	applyPlayer := manager.GetPlayerPtrById(req.PlayerId)
	if applyPlayer == nil {
		player.SendTipMsg(tipmsgid.TpTeamProcessLeaderApplyPlayerOffline)
		return nil
	}

	switch req.IsAgree {
	case true:
		isLeader := teammgr.LeaderP(applyPlayer)
		if isLeader {
			return neterror.ParamsInvalidError("already leader")
		}

		if !teammgr.PlayerCanAgreeAsLeaderP(teamId, req.PlayerId) {
			player.SendTipMsg(tipmsgid.TpTeamProcessLeaderApplyInvalid)
			return nil
		}

		if err := teammgr.ChangeLeader(teamId, req.PlayerId); err != nil {
			return neterror.InternalError(err.Error())
		}

	case false:
		exists := teammgr.RemoveLeaderApply(teamId, req.PlayerId)
		if exists {
			applyPlayer.SendTipMsg(tipmsgid.TpTeamProcessLeaderApplyRefuseApply, player.GetName())
		}
	}
	return nil
}

// *************************** 队伍设置 *****************************************
// 队伍设置
func (s *TeamSys) c2sSetting(msg *base.Message) error {
	var req pb3.C2S_28_100
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	if !checkTeamSettingChange(teamPb, req.Settings.FubenSetting.GroupFubenId) {
		player.SendTipMsg(tipmsgid.TeamReminder)
		return neterror.ParamsInvalidError("team member system not open")
	}

	state, err := teammgr.GetTeamState(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}
	if state == teammgr.TeamStateFight {
		return neterror.ParamsInvalidError("in team fuBen")
	}

	if err := teammgr.Setting(teamId, req.Settings); err != nil {
		return neterror.InternalError(err.Error())
	}
	return nil
}

func (s *TeamSys) c2sAskAssistFightRobotToTeam(_ *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		player.LogError("err: %v", err)
		return err
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	if err := teammgr.AddRobotToTeam(teamId, player); err != nil {
		player.LogError("err: %v", err)
		return err
	}

	return nil
}

// ************************************ 团队副本相关 ********************************
func (s *TeamSys) c2sEnterFb(_ *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	err = teammgr.RequireEnterFb(teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}
	return nil
}

func (s *TeamSys) c2sSureEnter(_ *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	err = teammgr.SureEnterFb(player.GetId(), teamId)
	if err != nil {
		return neterror.InternalError("sure enter failed %s", err.Error())
	}

	return nil
}

func (s *TeamSys) c2sRefuseEnter(_ *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	err = teammgr.RefuseEnterFb(player.GetId(), teamId)
	if err != nil {
		return neterror.InternalError(err.Error())
	}
	return nil
}

func (s *TeamSys) c2sClose(_ *base.Message) error {
	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.GroupFubenId == 0 {
		return neterror.ParamsInvalidError("not select fuBen")
	}

	for _, member := range teamPb.GetMembers() {
		if member.IsRobot {
			err = teammgr.PlayerExitTeam(teamId, member.PlayerInfo.Id)
			if err != nil {
				player.LogError("robot exit team err: %v", err)
			}
		}
	}

	teammgr.BroadCastToMember(teamId, 28, 204, &pb3.S2C_28_204{
		ActorId: player.GetId(),
	})
	return nil
}

func (s *TeamSys) c2sTeamMatch(msg *base.Message) error {
	var req pb3.C2S_28_205
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return neterror.ParamsInvalidError(err.Error())
	}

	isLeader := teammgr.LeaderP(player)
	if !isLeader {
		return neterror.ParamsInvalidError("not leader")
	}

	fbId := teamPb.Settings.FubenSetting.GroupFubenId
	if req.GetFbId() != fbId {
		return neterror.ParamsInvalidError("target fb not equal")
	}

	for _, member := range teamPb.GetMembers() {
		playerId := member.PlayerInfo.Id
		mPlayer := manager.GetPlayerPtrById(playerId)
		if mPlayer == nil {
			continue
		}
		if mPlayer.GetExtraAttrU32(attrdef.IsLost) == 1 {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder6, mPlayer.GetName()))
			return nil
		}

		if !checkCanChangeTeamFb(player, fbId) {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder))
			return nil
		}

		fbConf := jsondata.GetFbConf(fbId)

		if !mPlayer.CheckEnterFb(fbId) {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder5, mPlayer.GetName(), fbConf.FbName))
			return nil
		}

		mPlayer.GetSceneId()
		if mPlayer.IsInPrison() {
			teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TeamReminder5, mPlayer.GetName(), fbConf.FbName))
			return nil
		}
	}

	ret := &pb3.EnterTeamFbRetArgs{}
	err = teammgr.OnTeamMatch(teamId, req.Args, ret)
	if nil != err {
		return err
	}

	teammgr.BroadCastToMember(teamId, 28, 205, &pb3.S2C_28_205{FbId: req.FbId, Args: ret})
	return nil
}

func checkCanChangeTeamFb(player iface.IPlayer, fbId uint32) bool {
	if fbId == 0 {
		return true
	}
	teamFbConf := jsondata.GetTeamFubenConf(fbId)
	if teamFbConf != nil {
		sysId := teamFbConf.SysId
		if sysId == 0 {
			return true
		}
		return player.GetSysOpen(sysId)
	}
	return false
}

func checkTeamSettingChange(team *pb3.TeamSt, fbId uint32) bool {
	for _, teamMember := range team.GetMembers() {
		if teamMember.IsRobot {
			continue
		}
		mActor := manager.GetPlayerPtrById(teamMember.PlayerInfo.Id)
		if mActor != nil && !checkCanChangeTeamFb(mActor, fbId) {
			return false
		}
	}
	return true
}

func updateTeamMemberInfo(player iface.IPlayer) {
	teamId := player.GetTeamId()
	if teamId == 0 {
		return
	}

	memberInfo := teammgr.PackTeamMemberSt(player)

	err := teammgr.BroadCastToMember(teamId, 28, 86, &pb3.S2C_28_86{
		Player: memberInfo,
	})
	if err != nil {
		player.LogError("err:%v", err)
	}

	err = teammgr.UpdateMemberInfo(teamId, memberInfo)
	if err != nil {
		player.LogError("err:%v", err)
	}
}

func handleTeamMemLevelUp(player iface.IPlayer, _ ...interface{}) {
	updateTeamMemberInfo(player)
}

func handleTeamMemLogout(player iface.IPlayer, _ ...interface{}) {
	teamId := player.GetTeamId()
	if teamId == 0 {
		return
	}

	err := teammgr.PlayerExitTeam(teamId, player.GetId())
	if err != nil {
		player.LogError("err: %v", err)
	}
}

func init() {
	RegisterSysClass(sysdef.SiTeam, func() iface.ISystem {
		return &TeamSys{}
	})

	event.RegActorEvent(custom_id.AeLevelUp, handleTeamMemLevelUp)
	event.RegActorEvent(custom_id.AeLogout, handleTeamMemLogout)

	net.RegisterSysProto(28, 1, sysdef.SiTeam, (*TeamSys).c2sCreateTeam)
	net.RegisterSysProto(28, 2, sysdef.SiTeam, (*TeamSys).c2sTeamList)

	net.RegisterSysProto(28, 22, sysdef.SiTeam, (*TeamSys).c2sApplyJoinTeam)
	net.RegisterSysProto(28, 25, sysdef.SiTeam, (*TeamSys).c2sAgreeEnterTeam)
	net.RegisterSysProto(28, 26, sysdef.SiTeam, (*TeamSys).c2sDisAgreeEnterTeam)
	net.RegisterSysProto(28, 27, sysdef.SiTeam, (*TeamSys).c2sRefuseInvite)
	net.RegisterSysProto(28, 28, sysdef.SiTeam, (*TeamSys).c2sAskAssistFightRobotToTeam)

	net.RegisterSysProto(28, 40, sysdef.SiTeam, (*TeamSys).c2sInvite)
	net.RegisterSysProto(28, 42, sysdef.SiTeam, (*TeamSys).c2sAgreeInvite)
	net.RegisterSysProto(28, 43, sysdef.SiTeam, (*TeamSys).c2sQuickInvite)
	net.RegisterSysProto(28, 45, sysdef.SiTeam, (*TeamSys).c2sOnceInvite)
	net.RegisterSysProto(28, 61, sysdef.SiTeam, (*TeamSys).c2sNearByInviteList)

	net.RegisterSysProto(28, 84, sysdef.SiTeam, (*TeamSys).c2sKickOut)
	net.RegisterSysProto(28, 85, sysdef.SiTeam, (*TeamSys).c2sExitTeam)
	net.RegisterSysProto(28, 90, sysdef.SiTeam, (*TeamSys).c2sGrantLeader)
	net.RegisterSysProto(28, 93, sysdef.SiTeam, (*TeamSys).c2sApplyLeader)
	net.RegisterSysProto(28, 95, sysdef.SiTeam, (*TeamSys).c2sProcessApplyLeaderMsg)
	net.RegisterSysProto(28, 100, sysdef.SiTeam, (*TeamSys).c2sSetting)

	net.RegisterSysProto(28, 200, sysdef.SiTeam, (*TeamSys).c2sEnterFb)
	net.RegisterSysProto(28, 201, sysdef.SiTeam, (*TeamSys).c2sSureEnter)
	net.RegisterSysProto(28, 202, sysdef.SiTeam, (*TeamSys).c2sRefuseEnter)
	net.RegisterSysProto(28, 204, sysdef.SiTeam, (*TeamSys).c2sClose)
	net.RegisterSysProto(28, 205, sysdef.SiTeam, (*TeamSys).c2sTeamMatch)
}
