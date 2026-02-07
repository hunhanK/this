package dbworker

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/objversionworker"
	"os"

	"github.com/gzjjyz/logger"
)

const kiloByte = 1024

func loadGlobalVar(args ...interface{}) {
	var datas []mysql.GlobalVar
	if err := db.OrmEngine.SQL("call loadGlobalVar(?)", gshare.GameConf.SrvId).Find(&datas); nil != err {
		logger.LogError("load global error! %v", err)
		return
	}
	if len(datas) <= 0 {
		return
	}

	data := datas[0]

	global := &pb3.GlobalVar{}
	if err := pb3.Unmarshal(pb3.UnCompress(data.BinaryData), global); nil != err {
		logger.LogError("load global error! %v", err)
		return
	}

	gshare.SendGameMsg(custom_id.GMsgLoadGlobalVarRet, global)
}

func saveGlobalVar(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveGlobalVar", 1, len(args)) {
		return
	}
	blob, ok := args[0].([]byte)
	if !ok {
		return
	}

	if len(blob)/kiloByte > 60 {
		logger.LogWarn("saveGlobalVar 玩家binary data 大小: %dk 即将到达临界值65k", len(blob)/kiloByte)
	}

	_, err := db.OrmEngine.Exec("call saveGlobalVar(?,?)", gshare.GameConf.SrvId, blob)
	if nil != err {
		logger.LogError("save global var error! err:%v", err)
		return
	}
	objversionworker.PostObjVersionGlobalVarData(gshare.GameConf.SrvId, blob)
}

// LoadGlobalVarCacheFromFile 从文件加载全局变量缓存
func LoadGlobalVarCacheFromFile(fileName string) *pb3.GlobalVar {
	data, err := os.ReadFile(utils.GetCurrentDir() + fileName)
	if nil != err && os.IsNotExist(err) {
		logger.LogError("load actor cache from file error!! %v", err)
		return nil
	}

	if len(data) <= 0 {
		logger.LogError("load actor cache from file error!! buffer is nil")
		return nil
	}

	global := &pb3.GlobalVar{}
	if err := pb3.Unmarshal(pb3.UnCompress(data), global); nil != err {
		logger.LogError("load global data error! %v", err)
		return nil
	}
	return global
}

func handleGMsgGMLoadGlobalVarByLocalFile(param ...interface{}) {
	if len(param) < 1 {
		return
	}
	fileName := param[0].(string)
	global := LoadGlobalVarCacheFromFile(fileName)
	if global == nil {
		return
	}
	gshare.SendGameMsg(custom_id.GMsgLoadGlobalVarRet, global)
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveGlobalVar, saveGlobalVar)
		gshare.RegisterDBMsgHandler(custom_id.GMsgGMLoadGlobalVarByLocalFile, handleGMsgGMLoadGlobalVarByLocalFile)
	})
}
