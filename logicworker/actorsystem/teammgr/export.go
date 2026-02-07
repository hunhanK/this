/**
 * @Author: lzp
 * @Date: 2024/12/17
 * @Desc:
**/

package teammgr

import (
	"errors"
	"github.com/gzjjyz/logger"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

func CreateTeam(player iface.IPlayer, arg *TeamCreateSt) (uint64, error) {
	team, err := teamMgrIns.createTeam(player, arg)
	if err != nil {
		return 0, err
	}

	return team.team.TeamId, nil
}

func GetTeamPb(teamId uint64) (*pb3.TeamSt, error) {
	return teamMgrIns.getTeamPb(teamId)
}

func GetApplyList(teamId uint64) ([]uint64, error) {
	return teamMgrIns.getApplyList(teamId)
}

func LeaderP(player iface.IPlayer) bool {
	return teamMgrIns.leaderP(player)
}

func IsSameTeam(player, another iface.IPlayer) bool {
	if player.GetTeamId() == 0 {
		return false
	}
	if player.GetTeamId() != another.GetTeamId() {
		return false
	}
	return true
}

func AddMemberToTeam(teamId uint64, player iface.IPlayer, broadExceptPlayer ...uint64) error {
	return teamMgrIns.addMemberToTeam(teamId, player, broadExceptPlayer...)
}

func AddRobotToTeam(teamId uint64, player iface.IPlayer) error {
	return teamMgrIns.addRobotToTeam(teamId, player)
}

func GetTeamCreateTime(teamId uint64) (int64, error) {
	team := teamMgrIns.teamsMap[teamId]
	if team == nil {
		return 0, errors.New("team not exist")
	}

	return team.createTime, nil
}

func BroadCastToMember(teamId uint64, sysId, cmdId uint16, msg pb3.Message, except ...uint64) error {
	return teamMgrIns.broadCastToMember(teamId, sysId, cmdId, msg, except...)
}

func CheckJoin(teamId uint64, player iface.IPlayer) (CheckJoinState, error) {
	return teamMgrIns.checkJoin(teamId, player)
}

func ApplyEnterTeam(teamId uint64, player iface.IPlayer) error {
	return teamMgrIns.applyJoinTeam(teamId, player)
}

func PlayerExitTeam(teamId uint64, playerId uint64) error {
	return teamMgrIns.playerExitTeam(teamId, playerId)
}

func ChangeLeader(teamId uint64, playerId uint64) error {
	return teamMgrIns.changeLeader(teamId, playerId)
}

func Setting(teamId uint64, setting *pb3.TeamSettings) error {
	return teamMgrIns.setting(teamId, setting)
}

func BroadSettingInfo(teamId uint64) error {
	return teamMgrIns.broadSettingInfo(teamId)
}

func AddNeverAccept(teamId uint64, playerId uint64) error {
	return teamMgrIns.addNeverAccept(teamId, playerId)
}

func CheckApplicable(teamId uint64, player iface.IPlayer) ApplyState {
	return teamMgrIns.checkApplicable(teamId, player)
}

func CheckInvitable(teamId uint64, player iface.IPlayer) InviteState {
	return teamMgrIns.checkInvitable(teamId, player)
}

func TeamList() []*pb3.TeamSt {
	return teamMgrIns.teamList()
}

func PlayerInTeam(teamId uint64, playerId uint64) bool {
	return teamMgrIns.playerInTeam(teamId, playerId)
}

func PlayerCanAgreeAsLeaderP(teamId uint64, playerId uint64) bool {
	return teamMgrIns.playerCanAgreeAsLeaderP(teamId, playerId)
}

func ApplyLeader(teamId uint64, playerId uint64) {
	teamMgrIns.applyLeader(teamId, playerId)
}

func RemoveLeaderApply(teamId uint64, playerId uint64) (exists bool) {
	return teamMgrIns.removeLeaderApply(teamId, playerId)
}

func GetTeamState(teamId uint64) (state uint32, err error) {
	return teamMgrIns.getTeamState(teamId)
}

func GetTeamFbId(teamId uint64) uint32 {
	return teamMgrIns.getFbId(teamId)
}

func GetTeamFbSetting(teamId uint64) *pb3.TeamFbSettings {
	return teamMgrIns.getFbSetting(teamId)
}

func UpdateMemberInfo(teamId uint64, info *pb3.TeamMemberSt) (err error) {
	return teamMgrIns.updateMemberInfo(teamId, info)
}

func RequireEnterFb(teamId uint64) error {
	return teamMgrIns.requireEnterFb(teamId)
}

func SureEnterFb(playerId uint64, teamId uint64) error {
	return teamMgrIns.sureConsultEnterFb(playerId, teamId)
}

func RefuseEnterFb(playerId uint64, teamId uint64) error {
	return teamMgrIns.refuseConsultEnterFb(playerId, teamId)
}

func OnTeamMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArg *pb3.EnterTeamFbRetArgs) error {
	return teamMgrIns.onTeamMatch(teamId, args, retArg)
}

func MbConsultCheckFailed(playerId uint64, teamId uint64) error {
	return teamMgrIns.mbConsultCheckFailed(playerId, teamId)
}

func MbConsultCheckSuccess(playerId uint64, teamId uint64) error {
	return teamMgrIns.mbConsultCheckSuccess(playerId, teamId)
}

func MbEnterFbCheckFail(playerId uint64, teamId uint64) error {
	return teamMgrIns.mbEnterFbCheckFail(playerId, teamId)
}

func MbEnterFbReady(playerId uint64, teamId uint64) error {
	return teamMgrIns.mbEnterFbReady(playerId, teamId)
}

func DissolveTeam(teamId uint64) error {
	teamPb, err := GetTeamPb(teamId)
	if err != nil {
		return errors.New("team not found")
	}

	for _, teamMem := range teamPb.Members {
		if !teamMem.IsRobot {
			playerId := teamMem.PlayerInfo.Id
			err := PlayerExitTeam(teamId, playerId)
			if err != nil {
				logger.LogError("err: %v", err)
			}
		}
	}
	return nil
}
