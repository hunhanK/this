/**
 * @Author: ChenJunJi
 * @Date: 2023/11/06
 * @Desc:
**/

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"
)

func updateActorStatics(actorId uint64, rawData map[string]interface{}) {
	if len(rawData) <= 0 {
		return
	}

	exist, err := db.OrmEngine.Exist(&model.ActorStatics{ActorId: actorId})
	if nil == err {
		if exist {
			_, err = db.OrmEngine.Table("actor_statics").
				Where("actor_id = ?", actorId).Update(rawData)
		} else {
			rawData["actor_id"] = actorId
			_, err = db.OrmEngine.Table("actor_statics").Insert(rawData)
		}
	}

	if nil != err {
		logger.LogError("update actor statics error. data:%v, error %v", rawData, err)
		return
	}
	return
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgPlayerStatics, func(param ...interface{}) {
			updateActorStatics(param[0].(uint64), param[1].(map[string]interface{}))
		})
	})
}
