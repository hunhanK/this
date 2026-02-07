/**
 * @Author: zjj
 * @Date: 2024/12/19
 * @Desc:
**/

package teammgr

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

// TeamFuBenHandler 副本处理器
type TeamFuBenHandler interface {
	OnConsultEnter(teamId uint64, player iface.IPlayer, args string)
	OnEnterCheck(teamId uint64, player iface.IPlayer, args string)
	OnEnterCheckFailed(fbId uint32, unReadyList []iface.IPlayer, readyList []iface.IPlayer)
	OnChange(teamId uint64)
	OnMemberChange(teamId uint64)
	OnMatch(teamId uint64, args *pb3.EnterTeamFbArgs, retArgs *pb3.EnterTeamFbRetArgs) error
	OnCreateRet(teamId uint64, fbHdl uint64, sceneId uint32)
	OnGetTeamFuBenSrvType() base.ServerType
}

var teamFuBenHandlerMap = make(map[uint32]TeamFuBenHandler)

func RegTeamFbHandler(teamFbId uint32, handler TeamFuBenHandler) {
	_, ok := teamFuBenHandlerMap[teamFbId]
	if ok {
		panic(fmt.Sprintf("already reg %d fb id handler", teamFbId))
		return
	}
	teamFuBenHandlerMap[teamFbId] = handler
}

func GetTeamFbHandler(teamFbId uint32) (TeamFuBenHandler, error) {
	val, ok := teamFuBenHandlerMap[teamFbId]
	if !ok {
		return nil, neterror.ParamsInvalidError("%d not reg handler", teamFbId)
	}
	return val, nil
}
