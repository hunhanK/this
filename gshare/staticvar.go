/**
* @Author: ChenJunJi
* @Desc: 系统静态变量
* @Date: 2021/7/14 13:53
 */

package gshare

import (
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"

	"github.com/gzjjyz/logger"
)

var (
	systemVar = new(pb3.GlobalVar)
)

func SaveStaticVar() {
	if nil == systemVar {
		return
	}

	event.TriggerSysEvent(custom_id.SeBeforeSaveGlobalVar)

	blob := pb3.CompressByte(systemVar)
	if len(blob) <= 0 {
		return
	}

	SendDBMsg(custom_id.GMsgSaveGlobalVar, blob)
}

func GetStaticVar() *pb3.GlobalVar {
	return systemVar
}

func SetStaticVar(sysVar *pb3.GlobalVar) {
	systemVar = sysVar
}

func onLoadGlobalVar(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadGlobalVar", 1, len(args)) {
		return
	}
	st, ok := args[0].(*pb3.GlobalVar)
	if !ok {
		logger.LogError("on load global var args type error!")
		return
	}
	systemVar = st
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		RegisterGameMsgHandler(custom_id.GMsgLoadGlobalVarRet, onLoadGlobalVar)
	})
}
