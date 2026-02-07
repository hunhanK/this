/**
 * @Author: lzp
 * @Date: 2024/12/17
 * @Desc:
**/

package teammgr

import (
	"errors"
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"time"
)

var (
	teamMgrIns = newTeamMgr()
)

const (
	TeamIdStart = 100
)

type TeamCreateSt struct {
	FbSettings *pb3.TeamFbSettings
}

type teamMgr struct {
	teamsMap       map[uint64]*teamSt
	teamIdRecycled map[uint64]struct{} // 回收的队伍id
	curMaxTeamId   uint32
}

func newTeamMgr() *teamMgr {
	temp := &teamMgr{}
	temp.Init()
	return temp
}

func (t *teamMgr) Init() {
	t.teamsMap = make(map[uint64]*teamSt)
	t.teamIdRecycled = make(map[uint64]struct{})
	t.curMaxTeamId = TeamIdStart
}

// 分配唯一队伍id
func (t *teamMgr) allocateTeamId() uint64 {
	if len(t.teamIdRecycled) != 0 {
		for k := range t.teamIdRecycled {
			delete(t.teamIdRecycled, k)
			return k
		}
	}

	t.curMaxTeamId++
	teamId := uint64(engine.GetPfId())<<40 | uint64(engine.GetServerId())<<24 | uint64(t.curMaxTeamId)
	return teamId
}

// 回收队伍id
func (t *teamMgr) recycleTeamId(teamId uint64) {
	t.teamIdRecycled[teamId] = struct{}{}
}

func (t *teamMgr) createTeam(player iface.IPlayer, arg *TeamCreateSt) (*teamSt, error) {
	teamId := t.allocateTeamId()

	teamCommonConf := jsondata.GetTeamCommonConf()

	minLvLimit := teamCommonConf.LvFilterMinLimit[0]
	maxLvLimit := teamCommonConf.LvFilterMaxLimit[1]

	fbId := arg.FbSettings.GroupFubenId
	teamFbConf := jsondata.GetTeamFubenConf(fbId)
	if teamFbConf != nil {
		minLvLimit = teamFbConf.LvFilterMinLimit[0]
		maxLvLimit = teamFbConf.LvFilterMaxLimit[1]
	}

	team := &teamSt{
		team: &pb3.TeamSt{
			TeamId:   teamId,
			LeaderId: player.GetId(),
			Settings: &pb3.TeamSettings{
				FubenSetting: arg.FbSettings,
				MinLvLimit:   minLvLimit,
				MaxLvLimit:   maxLvLimit,
				AutoAccept:   true,
			},
		},
		createTime:            time.Now().Unix(),
		neverAcceptMap:        make(map[uint64]struct{}),
		applyLeadersPlayerIds: make(map[uint64]struct{}),
	}

	t.teamsMap[teamId] = team
	team.addMember(player)
	team.onFbChange(0, fbId)
	return team, nil
}

func (t *teamMgr) destroyTeam(teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return fmt.Errorf("team not exist, teamId: %d", teamId)
	}

	for _, mb := range team.team.Members {
		if mb == nil {
			continue
		}

		player := manager.GetPlayerPtrById(mb.PlayerInfo.Id)
		if player == nil {
			continue
		}
		player.SetTeamId(0)
		player.SetExtraAttr(attrdef.TeamId, 0)
	}

	delete(t.teamsMap, teamId)
	t.recycleTeamId(teamId)
	logger.LogDebug("team destory")
	return nil
}

func (t *teamMgr) getTeamPb(teamId uint64) (*pb3.TeamSt, error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return nil, errors.New("team not exist")
	}

	return team.team, nil
}

func (t *teamMgr) getApplyList(teamId uint64) ([]uint64, error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return nil, errors.New("team not exist")
	}

	return team.applyList, nil
}

func (t *teamMgr) addMemberToTeam(teamId uint64, player iface.IPlayer, broadExceptPlayer ...uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	team.addMember(player, broadExceptPlayer...)
	return nil
}

func (t *teamMgr) addRobotToTeam(teamId uint64, player iface.IPlayer) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}
	team.addRobot()
	return nil
}

func (t *teamMgr) teamMemberNum(teamId uint64) (int, error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return 0, errors.New("team not exist")
	}

	return len(team.team.Members), nil
}

func (t *teamMgr) leaderP(player iface.IPlayer) bool {
	team, ok := t.teamsMap[player.GetTeamId()]
	if !ok {
		return false
	}

	return team.team.LeaderId == player.GetId()
}

func (t *teamMgr) sendMemberInfoToLogic(teamId uint64) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return
	}
	team.sendMemberInfoToLogic()
	return
}

func (t *teamMgr) broadCastToMember(teamId uint64, sysId, cmdId uint16, msg pb3.Message, except ...uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	team.broadCastToMember(sysId, cmdId, msg, except...)
	return nil
}

func (t *teamMgr) broadSettingInfo(teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	team.broadcastSettingInfo()
	return nil
}

func (t *teamMgr) checkJoin(teamId uint64, player iface.IPlayer) (CheckJoinState, error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return CheckJoinStateUnknown, errors.New("team not exist")
	}
	return team.checkJoin(player), nil
}

func (t *teamMgr) applyJoinTeam(teamId uint64, player iface.IPlayer) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	team.join(player)
	return nil
}

func (t *teamMgr) playerExitTeam(teamId uint64, playerId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}
	team.playerExit(playerId)
	return nil
}

func (t *teamMgr) changeLeader(teamId uint64, playerId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.changeLeader(playerId)
}

func (t *teamMgr) setting(teamId uint64, setting *pb3.TeamSettings) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.setting(setting)
}

func (t *teamMgr) addNeverAccept(teamId uint64, playerId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}
	team.neverAcceptMap[playerId] = struct{}{}
	return nil
}

func (t *teamMgr) checkApplicable(teamId uint64, player iface.IPlayer) ApplyState {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return ApplyTeamFailedTeamLost
	}

	if team.getTeamState() == TeamStateEnterFb {
		return ApplyTeamFailedTeamFull
	}

	if team.getTeamState() == TeamStateFight {
		return ApplyTeamFailedTeamInFb
	}

	_, ok = team.neverAcceptMap[player.GetId()]
	if ok {
		return ApplyTeamFailedNeverAccept
	}

	if player.GetTeamId() != 0 {
		return ApplyTeamFailedInTeam
	}

	CheckEnterState := team.checkJoin(player)

	return CheckEnterStateMapToApplyState[CheckEnterState]

}

func (t *teamMgr) checkInvitable(teamId uint64, player iface.IPlayer) InviteState {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return InvitePlayerUnKnown
	}

	if team.getTeamState() == TeamStateEnterFb {
		return InvitePlayerFailedTeamFull
	}

	if team.getTeamState() == TeamStateFight {
		return InvitePlayerFailedTeamInFb
	}

	if player.GetTeamId() != 0 && player.GetTeamId() != teamId {
		return InvitePlayerFailedInOtherTeam
	}

	CheckEnterState := team.checkJoin(player)
	return CheckJoinStateMapToInviteState[CheckEnterState]
}

func (t *teamMgr) teamList() []*pb3.TeamSt {
	teams := make([]*pb3.TeamSt, 0, len(t.teamsMap))
	for _, team := range t.teamsMap {
		teams = append(teams, team.team)
	}
	return teams
}

func (t *teamMgr) playerInTeam(teamId uint64, playerId uint64) bool {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return false
	}

	return team.memberP(playerId)
}

func (t *teamMgr) playerCanAgreeAsLeaderP(teamId uint64, playerId uint64) bool {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return false
	}
	return team.playerCanAgreeAsLeaderP(playerId)
}

func (t *teamMgr) applyLeader(teamId uint64, playerId uint64) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return
	}
	team.applyLeader(playerId)
}

func (t *teamMgr) removeLeaderApply(teamId uint64, playerId uint64) (exists bool) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return
	}
	exists = team.removeLeaderApply(playerId)
	return
}

func (t *teamMgr) requireEnterFb(teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.requireEnterFb()
}

func (t *teamMgr) sureConsultEnterFb(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.sureConsultEnterFb(playerId)
}

func (t *teamMgr) refuseConsultEnterFb(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.refuseConsultEnterFb(playerId)
}

func (t *teamMgr) mbConsultCheckFailed(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.mbConsultCheckFailed(playerId)
}

func (t *teamMgr) mbConsultCheckSuccess(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.mbConsultCheckSuccess(playerId)
}

func (t *teamMgr) mbEnterFbCheckFail(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.mbEnterFbCheckFail(playerId)
}

func (t *teamMgr) mbEnterFbReady(playerId uint64, teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.mbEnterFbReady(playerId)
}

func (t *teamMgr) onTeamExitFuben(teamId uint64) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	return team.onTeamExitFb()
}

func (t *teamMgr) getTeamState(teamId uint64) (state uint32, err error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return 0, errors.New("team not exist")
	}

	return team.getTeamState(), nil
}

func (t *teamMgr) getActorNum(teamId uint64) uint32 {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return 0
	}
	return team.getActorNum()
}

func (t *teamMgr) getFbId(teamId uint64) uint32 {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return 0
	}
	return team.getFbId()
}

func (t *teamMgr) getFbSetting(teamId uint64) *pb3.TeamFbSettings {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return nil
	}
	return team.getFbSetting()
}

func (t *teamMgr) updateMemberInfo(teamId uint64, info *pb3.TeamMemberSt) (err error) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return errors.New("team not exist")
	}

	team.updateMemberInfo(info)
	return nil
}

func (t *teamMgr) onTeamMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArg *pb3.EnterTeamFbRetArgs) error {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return nil
	}
	return team.onTeamMatch(args, retArg)
}

func (t *teamMgr) kickRobot(teamId uint64) {
	team, ok := t.teamsMap[teamId]
	if !ok {
		return
	}
	team.kickRobot()
}
