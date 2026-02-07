package newjingjiemgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

func RunOne() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.TriggerEvent(custom_id.AeNewJingJieXiuLianPush, time_util.NowSec())
	})
}
