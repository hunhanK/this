/**
 * @Author: HeXinLi
 * @Desc:
 * @Date: 2022/4/2 14:54
 */

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

func loadDeleteActorIds(args ...interface{}) {
	var deleteActors []mysql.DeleteActor
	if err := db.OrmEngine.SQL("call loadDeleteActorIds()").Find(&deleteActors); nil != err {
		logger.LogError("%s", err)
		return
	}

	actorIds := make([]uint64, 0, len(deleteActors))
	for _, v := range deleteActors {
		actorIds = append(actorIds, v.ActorId)
	}
	gshare.SendGameMsg(custom_id.GMsgLoadDeletaActorIds, actorIds)
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadDeletaActorIds, loadDeleteActorIds)
	})
}
