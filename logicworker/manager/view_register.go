package manager

import (
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type ViewOtherPlayerTypeFunc func(targetPlayer iface.IPlayer, info *pb3.DetailedRoleInfo)

var ViewFuncMap = make(map[uint32]ViewOtherPlayerTypeFunc)

func RegisterViewFunc(viewType uint32, function ViewOtherPlayerTypeFunc) {
	if _, ok := ViewFuncMap[viewType]; ok {
		logger.LogStack("重复注册查看玩家信息函数. 类型 Type ：%d", viewType)
		return
	}

	ViewFuncMap[viewType] = function
}

func GetViewFunc(viewType uint32) ViewOtherPlayerTypeFunc {
	if function, ok := ViewFuncMap[viewType]; ok {
		return function
	}

	logger.LogStack("查看类型错误. 类型 Type ：%d", viewType)
	return nil
}

func TriggerViewFunc(viewType uint32, targetPlayer iface.IPlayer, info *pb3.DetailedRoleInfo) {
	if fun := GetViewFunc(viewType); nil != fun {
		fun(targetPlayer, info)
	}
}
