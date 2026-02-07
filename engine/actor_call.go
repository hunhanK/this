/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 10:37
 */

package engine

import (
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type ActorCallFuncType func(player iface.IPlayer, buf []byte)

var (
	actorCallMap = make(map[uint16]ActorCallFuncType)
)

func RegisterActorCallFunc(fnId uint16, fn ActorCallFuncType) {
	if nil == fn {
		logger.LogStack("注册远程调用函数为空. fnId:%d", fnId)
		return
	}
	if _, ok := actorCallMap[fnId]; ok {
		logger.LogStack("注册远程调用函数重复. fnId:%d", fnId)
		return
	}

	actorCallMap[fnId] = fn
}

func GetActorCallFunc(fnId uint16) ActorCallFuncType {
	cb, ok := actorCallMap[fnId]
	if !ok {
		logger.LogError("ActorCallFunc函数不存在. fnId:%d", fnId)
		return nil
	}
	return cb
}
