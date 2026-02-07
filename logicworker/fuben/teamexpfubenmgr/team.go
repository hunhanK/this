/**
 * @Author: LvYuMeng
 * @Date: 2024/12/19
 * @Desc:
**/

package teamexpfubenmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/manager"
)

type TeamFuBenHandler struct {
	teammgr.TeamFuBenBaseHandler
}

func (t *TeamFuBenHandler) OnChange(teamId uint64) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if nil == fbSet {
		logger.LogError("team %d setting nil", teamId)
		return
	}
	if fbSet.ExpFbTeamData == nil {
		fbSet.ExpFbTeamData = &pb3.ExpFbTeamData{}
	}
	updateSetting(teamId)
	return
}

func (t *TeamFuBenHandler) OnMemberChange(teamId uint64) {
	updateSetting(teamId)
}

func updateSetting(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	settingData, ok := getTeamSettingData(teamId)
	if !ok {
		return
	}

	oldSetting := settingData.Data
	settingData.Data = make(map[uint64]*pb3.ExpFbTeamActorData)

	for _, member := range teamPb.GetMembers() {
		playerId := member.GetPlayerInfo().GetId()
		player := manager.GetPlayerPtrById(playerId)
		if nil == player {
			continue
		}
		tActor := &pb3.ExpFbTeamActorData{}
		if oldSet, ok := oldSetting[playerId]; ok {
			tActor.CombineTimes = oldSet.GetCombineTimes()
			tActor.CanGetExp = oldSet.CanGetExp
		}
		tActor.LeftTimes = player.GetExtraAttrU32(attrdef.ExpCanChangeTimes)
		settingData.Data[playerId] = tActor
	}

	err = teammgr.BroadSettingInfo(teamId)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func getTeamSettingData(teamId uint64) (*pb3.ExpFbTeamData, bool) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if nil == fbSet {
		return nil, false
	}
	if nil == fbSet.ExpFbTeamData {
		fbSet.ExpFbTeamData = &pb3.ExpFbTeamData{}
	}
	if nil == fbSet.ExpFbTeamData.Data {
		fbSet.ExpFbTeamData.Data = make(map[uint64]*pb3.ExpFbTeamActorData)
	}
	return fbSet.ExpFbTeamData, true
}

func getTeamTeamPlayerSetData(teamId, playerId uint64) *pb3.ExpFbTeamActorData {
	if setData, ok := getTeamSettingData(teamId); ok {
		actorData, ok := setData.Data[playerId]
		if ok {
			return actorData
		}
	}
	return nil
}

func (t *TeamFuBenHandler) OnConsultEnter(teamId uint64, player iface.IPlayer, args string) {
	if !player.CheckBagCount(fubendef.EnterTeamFb) {
		player.SendTipMsg(tipmsgid.TpBagIsFull)
		err := teammgr.RefuseEnterFb(player.GetId(), teamId)
		if err != nil {
			player.LogError("bag is full")
		}
		return
	}
	ok := func() bool {
		expFbSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*actorsystem.ExpFbSys)
		if !ok || !expFbSys.IsOpen() {
			logger.LogError("expFubenConsultEnterCheckTimes sys is not open")
			return false
		}
		return true
	}()

	if ok {
		err := teammgr.MbConsultCheckSuccess(player.GetId(), player.GetTeamId())
		if err != nil {
			player.LogError("MbConsultCheckSuccess failed %s", err)
		}
	} else {
		err := teammgr.MbConsultCheckFailed(player.GetId(), player.GetTeamId())
		if err != nil {
			player.LogError("MbEnterFubenCheckFailed failed %s", err)
		}
	}
}

func (t *TeamFuBenHandler) OnEnterCheck(teamId uint64, player iface.IPlayer, _ string) {
	ok := func() bool {
		expFbSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*actorsystem.ExpFbSys)
		if !ok || !expFbSys.IsOpen() {
			logger.LogError("expFubenConsultEnterCheckTimes sys is not open")
			return false
		}
		consumeRet := expFbSys.TeamDoEnterConsume()
		if consumeRet {
			// 标记下可以获取经验
			actorData := getTeamTeamPlayerSetData(teamId, player.GetId())
			if actorData != nil {
				actorData.CanGetExp = true
			}
		}
		return true
	}()

	if ok {
		err := teammgr.MbEnterFbReady(player.GetId(), player.GetTeamId())
		if err != nil {
			player.LogError("MbEnterFbReady failed %s", err)
		}
	} else {
		err := teammgr.MbEnterFbCheckFail(player.GetId(), player.GetTeamId())
		if err != nil {
			player.LogError("MbEnterFbCheckFail failed %s", err)
		}
	}
}

func (t *TeamFuBenHandler) OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if nil == teamPb {
		return neterror.InternalError("team get err:%v", err)
	}

	conf := jsondata.GetExpFubenConf()
	if nil == conf {
		return neterror.ConfNotFoundError("exp fb common config not found")
	}

	retArgs.ExpFubenArgs = &pb3.ExpFubenTeamFbArgs{}

	canMatch := false
	for _, member := range teamPb.Members {
		if member.IsRobot {
			continue
		}
		playerId := member.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if nil == player {
			return neterror.ParamsInvalidError("player offline")
		}
		expFbSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*actorsystem.ExpFbSys)
		if !ok || !expFbSys.IsOpen() {
			return neterror.ParamsInvalidError("player exp sys not open")
		}
		timeEnough := expFbSys.TeamCheckEnterConsume(teamId)
		if timeEnough {
			canMatch = true
		}
	}

	if !canMatch {
		teammgr.BroadCastToMember(teamId, 5, 0, common.PackMsg(tipmsgid.TimesNotEnough))
		return neterror.ParamsInvalidError("cosTimes not enough")
	}

	for _, member := range teamPb.Members {
		playerId := member.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}
		retArgs.ExpFubenArgs.Actors = append(retArgs.ExpFubenArgs.Actors, &pb3.ExpFubenTeamFbActorArgs{
			LeftTimes: player.GetExtraAttrU32(attrdef.ExpCanChangeTimes),
			ActorId:   playerId,
		})
	}
	return nil
}

func (t *TeamFuBenHandler) OnCreateRet(teamId uint64, fbHdl uint64, sceneId uint32) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	todo := &pb3.EnterFubenHdl{}
	todo.FbHdl = fbHdl
	todo.SceneId = sceneId

	for _, mem := range teamPb.GetMembers() {
		player := manager.GetPlayerPtrById(mem.PlayerInfo.Id)
		if player == nil {
			continue
		}
		err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterTeamFb, todo)
		if err != nil {
			player.LogError("err:%v", err)
			teammgr.PlayerExitTeam(player.GetTeamId(), player.GetId())
			continue
		}
	}
}

func afterReloadConf(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	fConf := jsondata.GetExpFubenConf()
	if fConf == nil {
		return
	}
	isReload := args[0].(bool)
	if isReload {
		return
	}
	teammgr.RegTeamFbHandler(fConf.FbId, &TeamFuBenHandler{})
}

func init() {
	event.RegSysEvent(custom_id.SeReloadJson, afterReloadConf)

	event.RegActorEvent(custom_id.AeExpFbUseTimeChange, func(actor iface.IPlayer, args ...interface{}) {
		teamId := actor.GetTeamId()
		if teamId == 0 {
			return
		}

		expFbConf := jsondata.GetExpFubenConf()
		if nil == expFbConf {
			return
		}

		fbId := teammgr.GetTeamFbId(teamId)
		if fbId != expFbConf.FbId {
			logger.LogError("队伍目标不是经验副本")
			return
		}
		updateSetting(teamId)
	})
}
