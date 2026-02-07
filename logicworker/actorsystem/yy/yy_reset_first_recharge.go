/**
 * @Author: zjj
 * @Date: 2024/8/1
 * @Desc: 屠龙BOSS
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
)

type ResetFirstRecharge struct {
	YYBase
}

func (k *ResetFirstRecharge) OnOpen() {
	logger.LogInfo("YY ResetFirstRecharge Open, Send GMsgResetFirstRecharge Msg")
	gshare.SendGameMsg(custom_id.GMsgResetFirstRecharge)
}

func init() {
	yymgr.RegisterYYType(yydefine.YYResetFirstRecharge, func() iface.IYunYing {
		return &ResetFirstRecharge{}
	})
	gmevent.Register("resetfirstrecharge", func(player iface.IPlayer, args ...string) bool {
		gshare.SendGameMsg(custom_id.GMsgResetFirstRecharge)
		return true
	}, 1)
}
