/**
 * @Author: lzp
 * @Date: 2024/12/18
 * @Desc:
**/

package beastrampantmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/manager"
)

type TeamFuBenHandler struct {
	*teammgr.TeamFuBenBaseHandler
}

func (t *TeamFuBenHandler) OnChange(teamId uint64) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.BRFbTData == nil {
		fbSet.BRFbTData = &pb3.BRFbTeamData{}
	}
}

// OnMemberChange 到战斗服推送活动数据
func (t *TeamFuBenHandler) OnMemberChange(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		client := fightworker.GetFightClient(base.SmallCrossServer)
		if client != nil {
			err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FBeastRampantTeamMemChange, teamPb)
			if err != nil {
				logger.LogError("onConnectFightSrv err: %v", err)
			}
		}
		return
	}

	err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FBeastRampantTeamMemChange, teamPb)
	if err != nil {
		logger.LogError("onConnectFightSrv err: %v", err)
	}
}

func (t *TeamFuBenHandler) OnConsultEnter(teamId uint64, player iface.IPlayer, args string) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet != nil && fbSet.BRFbTData.Diff == 0 {
		return
	}

	err := teammgr.MbConsultCheckSuccess(player.GetId(), player.GetTeamId())
	if err != nil {
		player.LogError("MbConsultCheckSuccess failed %s", err)
	}
}

func (t *TeamFuBenHandler) OnEnterCheck(teamId uint64, player iface.IPlayer, _ string) {
	teammgr.MbEnterFbReady(player.GetId(), teamId)
}

func (t *TeamFuBenHandler) OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if teamPb == nil {
		return neterror.InternalError("team get err:%v", err)
	}

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.BRFbTData.Diff == 0 {
		return neterror.InternalError("not sel diff")
	}

	fbSet.BRFbTData.Hdl = args.Hdl

	retArgs.BRFbArgs = &pb3.BRFbTeamArgs{}
	retArgs.BRFbArgs.Hdl = args.Hdl
	retArgs.BRFbArgs.Actors = make(map[uint64]*pb3.BeastRampantPlayerInfo)
	for _, mem := range teamPb.Members {
		playerId := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}
		retArgs.BRFbArgs.Actors[playerId] = &pb3.BeastRampantPlayerInfo{
			LeftTimes: player.GetExtraAttrU32(attrdef.BeastRampantTimes),
			Diff:      fbSet.BRFbTData.Diff,
		}
	}
	return nil
}

func handleBeastRampantSelDiff(buf []byte) {
	var msg pb3.CommonSt
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		logger.LogError("unmarshal err: %v", err)
		return
	}

	teamId := msg.U64Param
	diff := msg.U32Param2

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet == nil {
		return
	}

	if fbSet.BRFbTData == nil {
		fbSet.BRFbTData = &pb3.BRFbTeamData{}
	}
	fbSet.BRFbTData.Diff = diff
}

func afterReloadConf(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	fConf := jsondata.GetBRFbConf()
	if fConf == nil {
		return
	}

	isReload := args[0].(bool)
	if isReload {
		return
	}
	handler := &TeamFuBenHandler{
		TeamFuBenBaseHandler: teammgr.NewTeamFuBenBaseHandler(
			teammgr.WithGetSrvType(func() base.ServerType {
				if engine.FightClientExistPredicate(base.SmallCrossServer) {
					client := fightworker.GetFightClient(base.SmallCrossServer)
					if client != nil {
						return base.SmallCrossServer
					}
				}
				return base.LocalFightServer
			}),
		)}

	teammgr.RegTeamFbHandler(fConf.FbId, handler)
}

func init() {
	event.RegSysEvent(custom_id.SeReloadJson, afterReloadConf)
	engine.RegisterSysCall(sysfuncid.F2GBeastRampantSelDiffReq, handleBeastRampantSelDiff)
}
