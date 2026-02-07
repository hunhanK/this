/**
 * @Author: zjj
 * @Date: 2024/12/19
 * @Desc:
**/

package teammgr

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

var _ TeamFuBenHandler = (*TeamFuBenBaseHandler)(nil)

type TeamFuBenBaseHandler struct {
	GetSrvType func() base.ServerType
}

func NewTeamFuBenBaseHandler(opts ...Option) *TeamFuBenBaseHandler {
	t := &TeamFuBenBaseHandler{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

type Option func(b *TeamFuBenBaseHandler)

func WithGetSrvType(f func() base.ServerType) Option {
	return func(b *TeamFuBenBaseHandler) {
		b.GetSrvType = f
	}
}

func (t *TeamFuBenBaseHandler) OnConsultEnter(teamId uint64, player iface.IPlayer, args string) {
}

func (t *TeamFuBenBaseHandler) OnEnterCheck(teamId uint64, player iface.IPlayer, args string) {
}

func (t *TeamFuBenBaseHandler) OnEnterCheckFailed(fbId uint32, unReadyList []iface.IPlayer, readyList []iface.IPlayer) {

}
func (t *TeamFuBenBaseHandler) OnChange(teamId uint64) {

}

func (t *TeamFuBenBaseHandler) OnMemberChange(teamId uint64) {

}

func (t *TeamFuBenBaseHandler) OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error {
	return nil
}

func (t *TeamFuBenBaseHandler) OnCreateRet(teamId uint64, fbHdl uint64, sceneId uint32) {
	teamPb, err := GetTeamPb(teamId)
	if err != nil {
		return
	}

	fbId := teamPb.GetSettings().FubenSetting.GroupFubenId
	todo := &pb3.EnterFubenHdl{}
	todo.FbHdl = fbHdl
	todo.SceneId = sceneId

	srvType := t.OnGetTeamFuBenSrvType()

	// 拉取玩家进入
	for _, mem := range teamPb.GetMembers() {
		player := manager.GetPlayerPtrById(mem.PlayerInfo.Id)
		if player == nil {
			continue
		}
		err := player.EnterFightSrv(srvType, fubendef.EnterTeamFb, todo)
		if err != nil {
			PlayerExitTeam(player.GetTeamId(), player.GetId())
			continue
		}
		player.TriggerEvent(custom_id.AeEnterTeamFb, fbId)
	}

}

func (t *TeamFuBenBaseHandler) OnGetTeamFuBenSrvType() base.ServerType {
	if t.GetSrvType != nil {
		return t.GetSrvType()
	}
	return base.LocalFightServer
}
