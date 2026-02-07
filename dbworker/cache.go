/**
 * @Author: zjj
 * @Date: 2023/12/1
 * @Desc:
**/

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveCacheImme, func(_ ...interface{}) {
			saveCacheImme()
		})
	})
}
