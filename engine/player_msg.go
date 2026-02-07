/**
 * @Author: ChenJunJi
 * @Desc: 玩家消息
 * @Date: 2021/9/18 10:11
 */

package engine

import (
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type (
	ActorMsgFnType   func(actor iface.IPlayer, msg pb3.Message)
	ActorMsgRegister struct {
		Pb3Creator func() pb3.Message // 生成对应的pb3结构
		CallBack   ActorMsgFnType
	}
)

var (
	actorMsgFnMap     = make(map[int]*ActorMsgRegister)
	SendPlayerMessage func(playerId uint64, msgType int, msg pb3.Message)
)

// RegisterMessage 注册玩家消息处理函数
func RegisterMessage(msgType int, pb3Creator func() pb3.Message, cb ActorMsgFnType) {
	if _, repeat := actorMsgFnMap[msgType]; repeat {
		logger.LogStack("玩家消息处理handle重复注册. msgType:%d", msgType)
		return
	}
	actorMsgFnMap[msgType] = &ActorMsgRegister{
		Pb3Creator: pb3Creator,
		CallBack:   cb,
	}
}

// GetMessagePb3 获取玩家消息类型对应的pb3结构
func GetMessagePb3(msgType int) pb3.Message {
	if st, ok := actorMsgFnMap[msgType]; ok {
		if nil != st.Pb3Creator {
			return st.Pb3Creator()
		}
	}
	return nil
}

// GetMessageCallback 获取玩家消息处理函数
func GetMessageCallback(msgType int) ActorMsgFnType {
	if st, ok := actorMsgFnMap[msgType]; ok {
		return st.CallBack
	}
	return nil
}
