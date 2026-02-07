package event

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/iface"
	"reflect"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type ActorEventCallBack func(player iface.IPlayer, args ...interface{})

const (
	HIGH   = iota // 触发优先级 高
	NORMAL        // 触发优先级 普通
	LOW           // 触发优先级 低
	TriggerLevelMax
)

var (
	aeFuncVec [custom_id.AeMax][TriggerLevelMax][]ActorEventCallBack
)

func TriggerEvent(actor iface.IPlayer, id int, args ...interface{}) {
	for _, fnSet := range aeFuncVec[id] {
		for _, fn := range fnSet {
			utils.ProtectRun(func() {
				fn(actor, args...)
			})
		}
	}
}

func regAEFunc(level int) func(id int, fn ActorEventCallBack) {
	if level < 0 || level >= TriggerLevelMax {
		logger.LogFatal("注册服务器事件优先级错误")
	}
	return func(id int, fn ActorEventCallBack) {
		if id < 0 || id >= custom_id.AeMax {
			logger.LogFatal("注册服务器事件id超出定义范围")
		}
		if fn == nil {
			logger.LogFatal("注册玩家事件回调函数为空")
		}

		sets := aeFuncVec[id][level]
		pointer := reflect.ValueOf(fn).Pointer()
		for _, one := range sets {
			if reflect.ValueOf(one).Pointer() == pointer {
				logger.LogFatal("RegActorEvent fn repeated. eventId:%d", id)
				return
			}
		}
		aeFuncVec[id][level] = append(aeFuncVec[id][level], fn)
	}
}

var (
	RegActorEventH = regAEFunc(HIGH)
	RegActorEvent  = regAEFunc(NORMAL)
	RegActorEventL = regAEFunc(LOW)
)
