package event

import (
	"jjyz/base/custom_id"
	"reflect"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

/*
desc:服务器事件
author:ChenJunJi
*/

type SrvEventCallBack func(args ...interface{})

var (
	SrvEventMap [custom_id.SeMax][TriggerLevelMax][]SrvEventCallBack
)

// RegServerEvent 注册玩家事件回调函数
func regSysEvent(level int) func(id int, fn SrvEventCallBack) {
	if level < 0 || level >= TriggerLevelMax {
		logger.LogFatal("注册服务器事件优先级错误")
	}
	return func(id int, fn SrvEventCallBack) {
		if id < 0 || id >= custom_id.SeMax {
			logger.LogFatal("注册服务器事件id超出定义范围")
		}
		if fn == nil {
			logger.LogFatal("注册玩家事件回调函数为空")
		}

		sets := SrvEventMap[id][level]
		pointer := reflect.ValueOf(fn).Pointer()
		for _, one := range sets {
			if reflect.ValueOf(one).Pointer() == pointer {
				logger.LogFatal("regServerEvent fn repeated. eventId:%d", id)
				return
			}
		}
		SrvEventMap[id][level] = append(SrvEventMap[id][level], fn)
	}
}

// TriggerServerEvent 触发服务器事件
func TriggerSysEvent(id int, args ...interface{}) {
	for _, fnSet := range SrvEventMap[id] {
		for _, fn := range fnSet {
			utils.ProtectRun(func() {
				fn(args...)
			})
		}
	}
}

var (
	RegSysEventH = regSysEvent(HIGH)
	RegSysEvent  = regSysEvent(NORMAL)
	RegSysEventL = regSysEvent(LOW)
)

func init() {
	RegSysEvent(custom_id.SePreReloadJson, func(args ...interface{}) {
		monList := []int{custom_id.SeMonsterDie, custom_id.SeMonsterDamage, custom_id.SeMonsterLiveTimeOut}
		for _, v := range monList {
			for k, _ := range SrvEventMap[v] {
				SrvEventMap[v][k] = []SrvEventCallBack{}
			}
		}
	})
}
