/**
 * @Author: zjj
 * @Date: 2024/8/2
 * @Desc:
**/

package yymgr

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
)

func init() {
	gmevent.Register("yy.open", func(actor iface.IPlayer, args ...string) bool {
		yyId := utils.AtoUint32(args[0])
		durationDay := utils.AtoUint32(args[1])
		sTime := time_util.NowSec()
		eTime := time_util.GetDaysZeroTime(durationDay)
		confIdx := uint32(1)
		if len(args) == 3 {
			index := utils.AtoUint32(args[2])
			if index > 0 {
				confIdx = index
			}
		}
		GmOpenYY(yyId, sTime, eTime, confIdx, true)
		return true
	}, 1)

	gmevent.Register("yy.end", func(actor iface.IPlayer, args ...string) bool {
		GmEndYY(args[0])
		return true
	}, 1)

	gmevent.Register("yy.endall", func(actor iface.IPlayer, args ...string) bool {
		for id, obj := range GetYYMgr().objMap {
			if obj != nil {
				utils.ProtectRun(func() {
					GetYYMgr().CloseYY(id, false)
				})
			}
		}
		return true
	}, 1)
}
