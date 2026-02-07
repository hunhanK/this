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
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"time"
)

type InviteState uint32

const (
	InvitePlayerUnKnown InviteState = iota
	InvitePlayerSuccess
	InvitePlayerFailedOffline
	InvitePlayerFailedInTeam
	InvitePlayerFailedInOtherTeam
	InvitePlayerFailedLevelLimit
	InvitePlayerFailedNotInWildFb
	InvitePlayerFailedTeamFull
	InvitePlayerFailedTeamInFb
)

const (
	TeamStateWaiting = iota // 队伍等待中
	TeamStateConsultEnterFb
	TeamStateEnterFb
	TeamStateFight
)

var CheckJoinStateMapToInviteState map[CheckJoinState]InviteState = map[CheckJoinState]InviteState{
	CheckJoinStateUnknown:           InvitePlayerUnKnown,
	CheckJoinStateSuccess:           InvitePlayerSuccess,
	CheckJoinStateFailedLevelLimit:  InvitePlayerFailedLevelLimit,
	CheckJoinStateFailedNotInWildFb: InvitePlayerFailedNotInWildFb,
}

type ApplyState uint32

const (
	ApplyTeamUnKnown ApplyState = iota
	ApplyTeamSuccess
	ApplyTeamFailedTeamLost
	ApplyTeamFailedTeamFull
	ApplyTeamFailedTeamInFb
	ApplyTeamFailedLevelLimit
	ApplyTeamFailedNeverAccept
	ApplyTeamFailedInTeam
	ApplyTeamFailedNotInWildFb
)

var CheckEnterStateMapToApplyState = map[CheckJoinState]ApplyState{
	CheckJoinStateUnknown:           ApplyTeamUnKnown,
	CheckJoinStateSuccess:           ApplyTeamSuccess,
	CheckJoinStateFailedLevelLimit:  ApplyTeamFailedLevelLimit,
	CheckJoinStateFailedTeamFull:    ApplyTeamFailedTeamFull,
	CheckJoinStateFailedInTeam:      ApplyTeamFailedInTeam,
	CheckJoinStateFailedNotInWildFb: ApplyTeamFailedNotInWildFb,
}

type CheckJoinState uint32

const (
	CheckJoinStateUnknown CheckJoinState = iota
	CheckJoinStateSuccess
	CheckJoinStateFailedLevelLimit
	CheckJoinStateFailedTeamFull
	CheckJoinStateFailedInTeam
	CheckJoinStateFailedNotInWildFb
)

type (
	ConsultEnterFbState uint32
	EnterFbState        uint32
)

const (
	ConsultEnterFbStateUnknown ConsultEnterFbState = iota
	ConsultEnterFbStateCheckConditionFailed
	ConsultEnterFbStateSure
)

const (
	EnterFbStateUnknown EnterFbState = iota
	EnterFbStateCheckFail
	EnterFbStateReady
)

const MemberMaxCount = 3

type (
	mbConsultEnterFbSt struct {
		checkTimer *time_util.Timer
		mbStates   map[uint64]ConsultEnterFbState
	}

	mbEnterFbSt struct {
		checkTimer *time_util.Timer
		mbStates   map[uint64]EnterFbState
	}

	teamSt struct {
		applyList             []uint64 // 申请列表
		team                  *pb3.TeamSt
		neverAcceptMap        map[uint64]struct{}
		createTime            int64
		applyLeadersPlayerIds map[uint64]struct{}
		teamState             uint32

		// for team fuben
		mbConsultEnterFbInfo *mbConsultEnterFbSt
		mbEnterFbInfo        *mbEnterFbSt
	}
)

func (t *teamSt) playerCanAgreeAsLeaderP(playerId uint64) bool {
	_, ok := t.applyLeadersPlayerIds[playerId]
	return ok
}

func (t *teamSt) applyLeader(playerId uint64) {
	if _, ok := t.applyLeadersPlayerIds[playerId]; !ok {
		t.applyLeadersPlayerIds[playerId] = struct{}{}
	}
}

func (t *teamSt) removeLeaderApply(playerId uint64) (exists bool) {
	_, exists = t.applyLeadersPlayerIds[playerId]
	delete(t.applyLeadersPlayerIds, playerId)
	return
}

func (t *teamSt) broadCastToMember(sysId uint16, cmdId uint16, msg pb3.Message, except ...uint64) {
	for _, member := range t.team.Members {
		if utils.SliceContainsUint64(except, member.PlayerInfo.Id) {
			continue
		}

		player := manager.GetPlayerPtrById(member.PlayerInfo.Id)
		if player == nil {
			continue
		}

		player.SendProto3(sysId, cmdId, msg)
	}
}

func (t *teamSt) noLevelLimitP() bool {
	return t.team.Settings.MinLvLimit == 0 && t.team.Settings.MaxLvLimit == 0
}

func (t *teamSt) checkLevelLimit(entity iface.IEntity) bool {
	if t.noLevelLimitP() {
		return true
	}

	if entity.GetLevel() < t.team.Settings.MinLvLimit {
		return false
	}

	if entity.GetLevel() > t.team.Settings.MaxLvLimit {
		return false
	}

	return true
}

func (t *teamSt) checkJoin(player iface.IPlayer) CheckJoinState {
	if player.GetTeamId() != 0 {
		return CheckJoinStateFailedInTeam
	}

	commonConf := jsondata.GetTeamCommonConf()
	if commonConf == nil {
		logger.LogError("team common conf is nil")
		return CheckJoinStateUnknown
	}

	if !t.checkLevelLimit(player) {
		return CheckJoinStateFailedLevelLimit
	}

	maxMemberNum := int(commonConf.MaxMember)
	if t.team.Settings.FubenSetting.GroupFubenId != 0 {
		fbConf := jsondata.GetTeamFubenConf(t.team.Settings.FubenSetting.GroupFubenId)
		if fbConf != nil {
			maxMemberNum = int(fbConf.MemberLimit)
		}
	}

	if len(t.team.Members) >= maxMemberNum {
		return CheckJoinStateFailedTeamFull
	}

	return CheckJoinStateSuccess
}

// 加入玩家
func (t *teamSt) addMember(player iface.IPlayer, broadExceptPlayer ...uint64) {
	player.TriggerEvent(custom_id.AePlayerJoinTeam)
	member := PackTeamMemberSt(player)
	teamId := t.team.TeamId
	player.SetTeamId(teamId)
	player.SetExtraAttr(attrdef.TeamId, attrdef.AttrValueAlias(teamId))
	t.team.Members = append(t.team.Members, member)
	t.onAddMember(member, broadExceptPlayer...)
}

// 加入机器人
func (t *teamSt) addRobot() {
	leader := t.getLeader()
	if leader == nil {
		return
	}

	// 随机等级
	leaderLevel := leader.GetLevel()
	minLv, maxLv := uint32(0), uint32(0)
	if leaderLevel <= 5 {
		minLv = 1
	} else {
		minLv = leaderLevel - 5
	}
	maxLv = leaderLevel + 5
	randomLevel := random.IntervalUU(minLv, maxLv)

	var ids []uint32
	for _, mem := range t.team.Members {
		if !mem.IsRobot {
			continue
		}
		ids = append(ids, uint32(mem.PlayerInfo.Id))
	}

	conf := jsondata.RandomOneAssistFightRobot(t.getFbId(), ids)
	if conf == nil {
		return
	}

	var inheritRate = conf.InheritRate
	if inheritRate == 0 {
		inheritRate = 5000
	}

	robot := &pb3.TeamMemberSt{
		IsRobot: true,
		PlayerInfo: &pb3.PlayerDataBase{
			Id:             conf.Id,
			Job:            conf.Job,
			Sex:            conf.Sex,
			Skills:         make(map[uint32]uint32),
			AppearInfo:     make(map[uint32]*pb3.SysAppearSt),
			Lv:             randomLevel,
			Circle:         leader.GetCircle(),
			SmallCrossCamp: int32(leader.GetSmallCrossCamp()),
			Name:           conf.Name,
			Power:          uint64(leader.GetExtraAttr(attrdef.FightValue) * inheritRate / 10000),
		},
	}

	t.team.Members = append(t.team.Members, robot)
	t.onAddMember(robot)
}

func (t *teamSt) onAddMember(member *pb3.TeamMemberSt, broadExceptPlayer ...uint64) {
	t.broadCastToMember(28, 83, &pb3.S2C_28_83{
		Player: member,
	}, broadExceptPlayer...)

	if t.mbConsultEnterFbInfo == nil {
		t.mbConsultEnterFbInfo = &mbConsultEnterFbSt{}
	}
	consultInfo := t.mbConsultEnterFbInfo
	if consultInfo.mbStates == nil {
		consultInfo.mbStates = make(map[uint64]ConsultEnterFbState)
	}
	t.mbConsultEnterFbInfo.mbStates[member.PlayerInfo.Id] = ConsultEnterFbStateUnknown

	t.onTeamMemberChange()
	t.sendMemberInfoToLogic()
}

func (t *teamSt) onTeamMemberChange() {
	fbId := t.team.GetSettings().FubenSetting.GroupFubenId
	handler, err := GetTeamFbHandler(fbId)
	if err != nil {
		return
	}
	handler.OnMemberChange(t.team.TeamId)
	event.TriggerSysEvent(custom_id.SeTeamMemberChange, t.team.TeamId)
}

// @todo
func (t *teamSt) sendMemberInfoToLogic() {
}

func (t *teamSt) join(player iface.IPlayer) {
	if t.checkJoin(player) != CheckJoinStateSuccess {
		return
	}

	if !utils.SliceContainsUint64(t.applyList, player.GetId()) {
		t.applyList = append(t.applyList, player.GetId())
	}
	t.onApply(player)
}

func (t *teamSt) playerExit(playerId uint64) {
	originMembers := t.team.Members
	t.team.Members = make([]*pb3.TeamMemberSt, 0, len(originMembers)-1)

	var kickMember *pb3.TeamMemberSt
	for _, member := range originMembers {
		if member.PlayerInfo.Id == playerId {
			kickMember = member
			continue
		}
		t.team.Members = append(t.team.Members, member)
	}

	if kickMember == nil {
		return
	}

	// 处理机器人
	if kickMember.IsRobot {
	} else {
		targetPlayer := manager.GetPlayerPtrById(playerId)
		if targetPlayer != nil {
			targetPlayer.SetTeamId(0)
			targetPlayer.SetExtraAttr(attrdef.TeamId, 0)
		}
		logger.LogDebug("team kick actor")
	}

	// 处理team actor 状态
	if t.mbConsultEnterFbInfo != nil {
		delete(t.mbConsultEnterFbInfo.mbStates, playerId)
	}

	t.onPlayerExit(playerId)

	if playerId == t.team.LeaderId {
		t.onLeaderExit()
	}
}

func (t *teamSt) onPlayerExit(playerId uint64) {
	t.broadCastToMember(28, 82, &pb3.S2C_28_82{
		PlayerId: playerId,
	})

	kickPlayer := manager.GetPlayerPtrById(playerId)
	if kickPlayer != nil {
		kickPlayer.SendProto3(28, 84, &pb3.S2C_28_84{})
		err := kickPlayer.CallActorFunc(actorfuncid.G2FActorExitTeam, &pb3.CommonSt{U64Param: t.team.TeamId})
		if err != nil {
			logger.LogError("err:%v", err)
		}
	}

	t.onTeamMemberChange()
	t.sendMemberInfoToLogic()

	if len(t.team.Members) == 0 {
		teamMgrIns.destroyTeam(t.team.TeamId)
		// 队伍解散发tips
		if kickPlayer != nil {
			kickPlayer.SendTipMsg(tipmsgid.TeamLeaderDissolve)
			kickPlayer.SendProto3(28, 2, &pb3.S2C_28_2{
				Teams: TeamList(),
			})
		}
	}
}

func (t *teamSt) onLeaderExit() {
	if len(t.team.Members) <= 0 {
		return
	}

	var newLeaderId uint64
	var kickRobotIds []uint64
	for _, member := range t.team.Members {
		if member.IsRobot {
			kickRobotIds = append(kickRobotIds, member.PlayerInfo.Id)
			continue
		}
		if newLeaderId != 0 {
			continue
		}
		newLeaderId = member.PlayerInfo.Id
	}

	for _, robotId := range kickRobotIds {
		t.playerExit(robotId)
	}

	if len(t.team.Members) <= 0 || newLeaderId == 0 {
		return
	}

	t.team.LeaderId = newLeaderId
	t.broadCastToMember(28, 81, &pb3.S2C_28_81{
		LeaderId: newLeaderId,
	})

	leader := manager.GetPlayerPtrById(newLeaderId)
	if leader == nil {
		return
	}

	leader.SendProto3(28, 100, &pb3.S2C_28_100{
		Settings: t.team.Settings,
	})
}

func (t *teamSt) onApply(player iface.IPlayer) {
	leader := manager.GetPlayerPtrById(t.team.LeaderId)
	if leader == nil {
		logger.LogError("leader is nil, leaderId:%d", t.team.LeaderId)
		return
	}

	foo := player.ToPlayerDataBase()

	leader.SendProto3(28, 24, &pb3.S2C_28_24{
		Player: foo,
	})
}

func (t *teamSt) changeLeader(playerId uint64) error {
	if t.team.LeaderId == playerId {
		return errors.New("already leader")
	}

	newLeader := manager.GetPlayerPtrById(playerId)
	if newLeader == nil {
		return errors.New("user to be granted is offline")
	}

	t.team.LeaderId = playerId
	t.broadCastToMember(28, 81, &pb3.S2C_28_81{
		LeaderId: playerId,
	})

	return nil
}

func (t *teamSt) settingChangebleP(settings *pb3.TeamSettings) bool {
	if settings.FubenSetting.GroupFubenId != 0 {
		fubenConf := jsondata.GetTeamFubenConf(settings.FubenSetting.GroupFubenId)
		if fubenConf == nil {
			logger.LogError("fuben conf is nil, fubenId:%d", settings.FubenSetting.GroupFubenId)
			return false
		}

		if fubenConf.MemberLimit != 0 && len(t.team.Members) > int(fubenConf.MemberLimit) {
			return false
		}
	}
	return true
}

func (t *teamSt) broadcastSettingInfo() {
	t.broadCastToMember(28, 100, &pb3.S2C_28_100{
		Settings: t.team.Settings,
	})
}

func (t *teamSt) setting(settings *pb3.TeamSettings) error {
	if !t.settingChangebleP(settings) {
		return errors.New("setting is not changeble")
	}

	originFubenId := t.team.Settings.FubenSetting.GroupFubenId
	t.team.Settings = settings
	if originFubenId != settings.FubenSetting.GroupFubenId {
		t.onFbChange(originFubenId, settings.FubenSetting.GroupFubenId)
	}

	t.broadcastSettingInfo()
	return nil
}

func (t *teamSt) onFbChange(originFubenId uint32, newFubenId uint32) {
	newFubenConf := jsondata.GetTeamFubenConf(newFubenId)
	if newFubenConf == nil {
		return
	}
	// 将队伍的等级限制设为不限制
	t.team.Settings.MinLvLimit = newFubenConf.LvFilterMinLimit[0]
	t.team.Settings.MaxLvLimit = newFubenConf.LvFilterMaxLimit[1]

	handler, err := GetTeamFbHandler(newFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	handler.OnChange(t.team.TeamId)
}

func (t *teamSt) memberP(playerId uint64) bool {
	for _, member := range t.team.Members {
		if member.PlayerInfo.Id == playerId {
			return true
		}
	}
	return false
}

func (t *teamSt) onConsultEnterCheckAllSure() {
	t.startEnterFbCheck()
	handler, err := GetTeamFbHandler(t.team.Settings.FubenSetting.GroupFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	status := true
	var playerList []iface.IPlayer
	for _, mb := range t.team.Members {
		id := mb.PlayerInfo.Id
		if mb.IsRobot {
			err := t.mbEnterFbReady(id)
			if err != nil {
				logger.LogError("err:%v", err)
				return
			}
			continue
		}
		player := manager.GetPlayerPtrById(id)
		if player == nil {
			status = false
			t.playerExit(id)
			logger.LogError("onConsultEnterCheckAllSure error:entity %d is not exist in team!", id)
			continue
		}
		playerList = append(playerList, player)
	}

	if !status {
		t.kickRobot()
		t.teamState = TeamStateWaiting
		for key := range t.mbConsultEnterFbInfo.mbStates {
			t.mbConsultEnterFbInfo.mbStates[key] = ConsultEnterFbStateUnknown
		}
		logger.LogError("onConsultEnterCheckAllSure status error!")
		return
	}

	for _, player := range playerList {
		handler.OnEnterCheck(t.team.TeamId, player, t.team.Settings.FubenSetting.JsonArgs)
	}

	logger.LogInfo("onConsultEnterCheckAllSure all sure")
}

func (t *teamSt) startConsultEnterCheck() {
	allSure :=
		functional.ReduceMap(t.mbConsultEnterFbInfo.mbStates,
			func(reduce bool, actorId uint64, state ConsultEnterFbState) bool {
				if !reduce {
					return reduce
				}

				return reduce && state == ConsultEnterFbStateSure
			}, true)

	// 如果不需要加机器人, 不用满足3人的判断
	if t.checkAutoAddRobot() {
		if len(t.team.GetMembers()) == 3 && allSure {
			t.onConsultEnterCheckAllSure()
		}
	} else {
		if allSure {
			t.onConsultEnterCheckAllSure()
		}
	}
}

func (t *teamSt) onEnterFbCheckTimeOut() {
	t.onEnterCheckFailed()
}

func (t *teamSt) onEnterFbCheckAllMemberReady() {
	t.mbEnterFbInfo.checkTimer.Stop()
	t.teamState = TeamStateFight
	t.mbEnterFbInfo = nil
	handler, err := GetTeamFbHandler(t.team.Settings.FubenSetting.GroupFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	srvType := handler.OnGetTeamFuBenSrvType()
	teamPb, err := GetTeamPb(t.team.TeamId)
	if err := engine.CallFightSrvFunc(srvType, sysfuncid.G2FTeamFbCreate, teamPb); err != nil {
		logger.LogError("err: %v", err)
		return
	}
}

func (t *teamSt) startEnterFbCheck() {
	// change teamState to TeamState_EnterFb
	t.teamState = TeamStateEnterFb

	// initial mbEnterFbInfo
	t.mbEnterFbInfo = &mbEnterFbSt{
		mbStates: make(map[uint64]EnterFbState),
	}
	for _, mb := range t.team.Members {
		t.mbEnterFbInfo.mbStates[mb.PlayerInfo.Id] = EnterFbStateUnknown
	}

	// check loop deadline
	endTime := time.Now().Add(30 * time.Second)

	mbEnterFbChecker := func() {
		allReady :=
			functional.ReduceMap(t.mbEnterFbInfo.mbStates,
				func(reduce bool, actorId uint64, state EnterFbState) bool {
					if !reduce {
						return reduce
					}

					return reduce && state == EnterFbStateReady && len(t.mbEnterFbInfo.mbStates) > 0
				}, true)

		if time.Now().After(endTime) {
			t.onEnterFbCheckTimeOut()
			return
		}

		if allReady {
			t.onEnterFbCheckAllMemberReady()
		}
		logger.LogInfo("mbEnterFbChecker checking")
	}
	if t.mbEnterFbInfo.checkTimer != nil {
		t.mbEnterFbInfo.checkTimer.Stop()
	}
	t.mbEnterFbInfo.checkTimer = timer.SetInterval(1*time.Second, mbEnterFbChecker)
}

func (t *teamSt) requireEnterFb() error {
	if t.teamState != TeamStateWaiting {
		return errors.New("not in waiting state")
	}

	// 设置副本状态
	t.teamState = TeamStateConsultEnterFb

	if t.checkAutoAddRobot() {
		t.autoAddRobot()
	}

	t.broadCastToMember(28, 200, &pb3.S2C_28_200{})
	return nil
}

func (t *teamSt) autoAddRobot() {
	num := len(t.team.Members)
	for i := num; i < MemberMaxCount; i++ {
		t.addRobot()
	}

	// 机器人自动准备
	for _, member := range t.team.Members {
		if member.IsRobot {
			err := MbConsultCheckSuccess(member.PlayerInfo.Id, t.team.TeamId)
			if err != nil {
				logger.LogError("err: %v", err)
			}
		}
	}
}

func (t *teamSt) checkAutoAddRobot() bool {
	if t.getFbId() > 0 {
		fbConf := jsondata.GetFbConf(t.getFbId())
		if fbConf != nil && fbConf.IsTeamFb && fbConf.AutoAddRobot {
			return true
		}
	}
	return false
}

func (t *teamSt) getFbId() uint32 {
	fbSet := t.team.GetSettings().FubenSetting
	if fbSet == nil {
		return 0
	}
	return fbSet.GroupFubenId
}

func (t *teamSt) getFbSetting() *pb3.TeamFbSettings {
	return t.team.GetSettings().FubenSetting
}

func (t *teamSt) sureConsultEnterFb(playerId uint64) error {
	if t.teamState != TeamStateConsultEnterFb {
		return errors.New("not in TeamState_ConsultEnterFb")
	}

	handler, err := GetTeamFbHandler(t.team.Settings.FubenSetting.GroupFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}

	surer := manager.GetPlayerPtrById(playerId)
	handler.OnConsultEnter(t.team.TeamId, surer, t.team.Settings.FubenSetting.JsonArgs)

	return nil
}

func (t *teamSt) onTeamExitFb() error {
	if t.teamState != TeamStateFight {
		return errors.New("not in fight state")
	}

	t.teamState = TeamStateWaiting

	// 踢出机器人
	t.kickRobot()

	if t.mbConsultEnterFbInfo == nil {
		return nil
	}
	for id := range t.mbConsultEnterFbInfo.mbStates {
		t.mbConsultEnterFbInfo.mbStates[id] = ConsultEnterFbStateUnknown
	}

	return nil
}

func (t *teamSt) refuseConsultEnterFb(playerId uint64) error {
	if t.teamState != TeamStateConsultEnterFb {
		return errors.New("not in TeamStateConsultEnterFb state")
	}

	t.teamState = TeamStateWaiting
	for key := range t.mbConsultEnterFbInfo.mbStates {
		t.mbConsultEnterFbInfo.mbStates[key] = ConsultEnterFbStateUnknown
	}
	t.broadCastToMember(28, 202, &pb3.S2C_28_202{
		RefuserId: playerId,
	})

	t.kickRobot()
	return nil
}

func (t *teamSt) mbConsultCheckFailed(playerId uint64) error {
	if t.teamState != TeamStateConsultEnterFb {
		return errors.New("not in TeamState_ConsultEnterFuben")
	}

	// 队员状态改为初始状态
	if t.mbConsultEnterFbInfo != nil {
		for id := range t.mbConsultEnterFbInfo.mbStates {
			t.mbConsultEnterFbInfo.mbStates[id] = ConsultEnterFbStateUnknown
		}
	}
	t.teamState = TeamStateWaiting

	t.broadCastToMember(28, 203, &pb3.S2C_28_203{
		UserId: playerId,
	})

	t.kickRobot()
	return nil
}

func (t *teamSt) mbConsultCheckSuccess(playerId uint64) error {
	if t.teamState != TeamStateConsultEnterFb {
		return errors.New("not in TeamState_ConsultEnterFuben")
	}

	t.mbConsultEnterFbInfo.mbStates[playerId] = ConsultEnterFbStateSure

	var sureMembers []uint64
	for mbId, state := range t.mbConsultEnterFbInfo.mbStates {
		if state == ConsultEnterFbStateSure {
			sureMembers = append(sureMembers, mbId)
		}
	}

	t.broadCastToMember(28, 201, &pb3.S2C_28_201{
		PlayerId: sureMembers,
	})

	t.startConsultEnterCheck()

	return nil
}

func (t *teamSt) onEnterCheckFailed() {
	t.mbEnterFbInfo.checkTimer.Stop()
	t.teamState = TeamStateWaiting
	fbId := t.team.Settings.FubenSetting.GroupFubenId

	var readyList []iface.IPlayer
	for k, efs := range t.mbEnterFbInfo.mbStates {
		if efs == EnterFbStateReady {
			player := manager.GetPlayerPtrById(k)
			if player == nil {
				continue
			}
			readyList = append(readyList, player)
		}
	}

	var unReadyList []iface.IPlayer
	for k, efs := range t.mbEnterFbInfo.mbStates {
		if efs != EnterFbStateReady {
			continue
		}

		player := manager.GetPlayerPtrById(k)
		if player == nil {
			continue
		}
		unReadyList = append(unReadyList, player)
	}

	handler, err := GetTeamFbHandler(t.team.Settings.FubenSetting.GroupFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	handler.OnEnterCheckFailed(fbId, unReadyList, readyList)

	t.mbEnterFbInfo = nil

	t.kickRobot()
}

func (t *teamSt) mbEnterFbCheckFail(playerId uint64) error {
	if t.teamState != TeamStateEnterFb {
		return errors.New("not in TeamState_EnterFb")
	}

	t.mbEnterFbInfo.mbStates[playerId] = EnterFbStateCheckFail

	t.onEnterCheckFailed()
	return nil
}

func (t *teamSt) mbEnterFbReady(playerId uint64) error {
	if t.teamState != TeamStateEnterFb {
		return errors.New("not in TeamState_EnterFb")
	}

	t.mbEnterFbInfo.mbStates[playerId] = EnterFbStateReady
	return nil
}

func (t *teamSt) getTeamState() uint32 {
	return t.teamState
}

func (t *teamSt) getLeader() iface.IPlayer {
	leaderId := t.team.LeaderId
	return manager.GetPlayerPtrById(leaderId)
}

func (t *teamSt) getActorNum() uint32 {
	var actorNum uint32
	for _, v := range t.team.Members {
		if v.IsRobot {
			continue
		}
		actorNum++
	}
	return actorNum
}

func (t *teamSt) updateMemberInfo(info *pb3.TeamMemberSt) {
	if info == nil {
		return
	}

	if len(t.team.Members) == 0 {
		return
	}

	var idx int = -1
	for i, tms := range t.team.Members {
		if tms.PlayerInfo.Id == info.PlayerInfo.Id {
			idx = i
			break
		}
	}

	if idx == -1 {
		return
	}

	t.team.Members[idx] = info
}

func (t *teamSt) kickRobot() {
	for _, member := range t.team.GetMembers() {
		if member.IsRobot {
			t.playerExit(member.PlayerInfo.Id)
		}
	}
}

func (t *teamSt) onTeamMatch(args *pb3.EnterTeamFbArgs, retArg *pb3.EnterTeamFbRetArgs) error {
	handler, err := GetTeamFbHandler(t.team.GetSettings().FubenSetting.GroupFubenId)
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}
	return handler.OnMatch(t.team.TeamId, args, retArg)
}

func PackTeamMemberSt(player iface.IPlayer) *pb3.TeamMemberSt {
	foo := player.ToPlayerDataBase()

	return &pb3.TeamMemberSt{
		PlayerInfo: foo,
	}
}

func handleTeamFbCreateRet(buf []byte) {
	var ret pb3.CreateTeamFbArgsRet
	if err := pb3.Unmarshal(buf, &ret); err != nil {
		logger.LogError("unmarshal failed")
		return
	}

	teamFbId := ret.TeamFbId

	handler, err := GetTeamFbHandler(teamFbId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	handler.OnCreateRet(ret.TeamId, ret.FbHdl, ret.SceneId)

	// 拉机器人进副本
	callAssistFightToFuBen(teamFbId, ret.TeamId, ret.FbHdl, ret.SceneId)
}

func callAssistFightToFuBen(teamFbId uint32, teamId uint64, fbHdl uint64, sceneId uint32) {
	handler, err := GetTeamFbHandler(teamFbId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	srvType := handler.OnGetTeamFuBenSrvType()
	team, err := GetTeamPb(teamId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	createData := robotmgr.CopyRealActorMirrorRobotData(team.LeaderId, &custom_id.MirrorRobotParam{
		RobotType: custom_id.ActorRobotTypeAssistFight,
	})

	for _, member := range team.Members {
		if !member.IsRobot {
			continue
		}
		createData.RobotConfigId = member.PlayerInfo.Id
		createData.Level = member.PlayerInfo.Lv
		createData.Circle = member.PlayerInfo.Circle
		err = engine.CallFightSrvFunc(srvType, sysfuncid.G2FTeamFbCreateAssistFightRobotReq, &pb3.CreateAssistFightRobotReq{
			TeamId:         teamId,
			SceneId:        sceneId,
			SmallCrossCamp: uint32(member.PlayerInfo.SmallCrossCamp),
			FbHdl:          fbHdl,
			FbId:           teamFbId,
			MirrorData:     createData,
		})
		if err != nil {
			logger.LogError("G2FTeamFbCreateAssistFightRobotReq [%d] err:%v", createData.RobotConfigId, err)
		}
	}
}

func operationTeam(req *pb3.OperationTeam) error {
	teamId := req.TeamId
	operation := req.Opeation
	switch operation {
	case custom_id.TeamOperation_PlayerExit:
		arg, ok := req.Arg.(*pb3.OperationTeam_PlayerExitArg)
		if !ok {
			return fmt.Errorf("not OperationTeam_PlayerExitArgs type")
		}
		err := teamMgrIns.playerExitTeam(teamId, arg.PlayerExitArg.ActorId)
		if err != nil {
			return err
		}
	case custom_id.TeamOperation_TeamExitFb:
		err := teamMgrIns.onTeamExitFuben(teamId)
		if err != nil {
			return err
		}
	case custom_id.TeamOperation_KickRobot:
		teamMgrIns.kickRobot(teamId)
	}
	return nil
}

func handleOperationTeam(buf []byte) {
	var req pb3.OperationTeam
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("unmarshal failed")
		return
	}

	err := operationTeam(&req)
	if nil != err {
		logger.LogError("err:%v", err)
	}
}

func handleDissolveTeam(buf []byte) {
	var req pb3.DissolveTeamReq
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("unmarshal failed")
		return
	}

	err := DissolveTeam(req.TeamId)
	if err != nil {
		logger.LogError("err: %v", err)
	}
}

func init() {
	engine.RegisterSysCall(sysfuncid.F2GTeamFbCreateRet, handleTeamFbCreateRet)
	engine.RegisterSysCall(sysfuncid.F2GOperationTeam, handleOperationTeam)
	engine.RegisterSysCall(sysfuncid.C2GDissolveTeam, handleDissolveTeam)
}
