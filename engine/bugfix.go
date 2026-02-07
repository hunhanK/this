/**
 * @Author: HeXinLi
 * @Desc:
 * @Date: 2022/2/23 11:45
 */

package engine

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

func SetBugFixVersion(player iface.IPlayer, version uint32) {
	binary := player.GetBinaryData()

	bugFix := binary.BugFixVersion
	binary.BugFixVersion = utils.SetBit(bugFix, version)
}

func init() {
	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		for i := uint32(custom_id.BugFixMin); i <= custom_id.BugFixMax; i++ {
			SetBugFixVersion(player, i)
		}
	})
}
