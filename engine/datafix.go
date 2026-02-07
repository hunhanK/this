/**
 * @Author: HeXinLi
 * @Desc: 老号数据修复版本
 * @Date: 2022/3/31 11:54
 */

package engine

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

func SetDataFixVersion(player iface.IPlayer, version uint32) {
	binary := player.GetBinaryData()
	dataFix := binary.DataFixVersion
	binary.DataFixVersion = utils.SetBit(dataFix, version)
}

func init() {
	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		for i := uint32(custom_id.DataFixMin); i <= custom_id.DataFixMax; i++ {
			SetDataFixVersion(player, i)
		}
	})
}
