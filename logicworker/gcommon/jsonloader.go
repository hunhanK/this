package gcommon

import (
	"jjyz/base/confmgr"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare/event"
	"strings"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func LoadJsonData(reload bool) {
	start := time.Now()
	logger.LogWarn("开始加载配置文件")

	event.TriggerSysEvent(custom_id.SePreReloadJson, reload)
	for k, loader := range confmgr.ConfVec {
		if nil == loader {
			continue
		}
		if !loader() {
			logger.LogError("load %d failed", k)
		}
	}
	event.TriggerSysEvent(custom_id.SeReloadJson, reload)
	logger.LogWarn("加载配置文件完成, 耗时:%v", time.Since(start))
}

func LoadSomeJsonData(idsString string) {
	start := time.Now()
	logger.LogWarn("开始热更新配置文件")

	event.TriggerSysEvent(custom_id.SePreReloadJson, true)
	splitStr := strings.Split(idsString, ",")
	for _, numStr := range splitStr {
		id := utils.Atoi(numStr)
		if loader := confmgr.ConfVec[id]; loader != nil {
			loader()
			logger.LogWarn("热更新配置：%d", id)
		}
	}
	event.TriggerSysEvent(custom_id.SeReloadJson, true)

	logger.LogWarn("热更新配置完成, 耗时:%v", time.Since(start))
}
