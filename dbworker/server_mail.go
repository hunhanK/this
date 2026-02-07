/**
 * @Author: ChenJunJi
 * @Date: 2023/11/09
 * @Desc:
**/

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/model"
)

func loadServerMail(args ...interface{}) {
	list, err := model.LoadServerMail()
	if nil != err {
		logger.LogError("load server mail error. %v", err)
		return
	}

	gshare.SendGameMsg(custom_id.GMsgLoadSrvMailRet, list)
}

func addServerMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("addServerMail", 1, len(args)) {
		return
	}

	st, ok := args[0].(*model.ServerMail)
	if !ok {
		logger.LogError("addServerMail Param Error!!!")
		return
	}

	if _, err := db.OrmEngine.Insert(st); nil != err {
		logger.LogError("add server mail error! mail:{%v} err:%v", st, err)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadSrvMail, loadServerMail)
		gshare.RegisterDBMsgHandler(custom_id.GMsgAddServerMail, addServerMail)
	})
}
