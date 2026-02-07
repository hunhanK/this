package event

import (
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"reflect"

	"github.com/gzjjyz/logger"
)

type RoleRecEventCallBack func(player iface.IPlayer, id uint32) *pb3.Scroll

var (
	recFuncMap map[uint32]RoleRecEventCallBack
)

func GetRecScroll(actor iface.IPlayer, id uint32) *pb3.Scroll {
	if fun := recFuncMap[id]; nil != fun {
		return fun(actor, id)
	}
	return nil
}

// 注册玩家九州绘卷记录事件
func RegRecFunc(id uint32, fun RoleRecEventCallBack) {
	if id < custom_id.RoleRec1001 && id >= custom_id.RoleRecMax {
		logger.LogFatal("注册玩家生涯事件id超出定义范围")
	}
	if fun == nil {
		logger.LogFatal("注册玩家生涯事件回调函数为空")
	}
	if nil == recFuncMap {
		recFuncMap = make(map[uint32]RoleRecEventCallBack)
	}
	pointer := reflect.ValueOf(fun).Pointer()
	for _, one := range recFuncMap {
		if reflect.ValueOf(one).Pointer() == pointer {
			logger.LogFatal("RegActorEvent fun repeated. eventId:%d", id)
			return
		}
	}
	recFuncMap[id] = fun
}
