/**
 * @Author: DaiGuanYu
 * @Desc:
 * @Date: 2022/1/10 15:00
 */

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"
)

func updateServerInfo(args ...interface{}) {
	if len(args) <= 1 {
		return
	}

	serverId, ok := args[0].(uint32)
	if !ok {
		return
	}

	flag, ok := args[1].(bool)
	if !ok {
		return
	}

	var createFlag uint32
	if flag {
		createFlag = 1
	}

	//serverInfo := &pb3.ServerInfo{}
	//pb3.Unmarshal(buf, serverInfo)
	_, err := db.OrmEngine.Table(model.ServerInfo{}.TableName()).Where("`server_id` = ?", serverId).Update(map[string]interface{}{
		"forbid_create_flag": createFlag,
	})
	if nil != err {
		logger.LogError("%v", err)
		return
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateServerInfo, updateServerInfo)
	})
}
