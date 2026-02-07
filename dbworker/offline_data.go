/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2022/6/18 1:12
 */

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/model"

	"github.com/gzjjyz/logger"
)

// 保存玩家数据，如果保存失败，需要通知逻辑协程
func saveOfflineData(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveOfflineData", 1, len(args)) {
		return
	}

	m, ok := args[0].(model.OfflineData)
	if !ok {
		logger.LogError("保存玩家离线数据失败, 参数1不是model.OfflineData")
		return
	}

	exist, err := db.OrmEngine.Exist(&model.OfflineData{
		ActorId: m.ActorId,
		SysId:   m.SysId,
	})

	if nil == err {
		if exist {
			_, err = db.OrmEngine.ID([]interface{}{m.ActorId, m.SysId}).Update(&m)
		} else {
			_, err = db.OrmEngine.Insert(&m)
		}
	}
	if nil != err {
		gshare.SendGameMsg(custom_id.GMsgSaveOfflineDataFailed, m.ActorId, m.SysId)
		logger.LogError("save player data error!!! playerId=%d, dataId=%d, err=%v", m.ActorId, m.SysId, err)
		return
	}
}
func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveOfflineData, saveOfflineData)
	})
}
