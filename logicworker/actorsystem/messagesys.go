/**
 * @Author: ChenJunJi
 * @Desc: 玩家消息
 * @Date: 2021/9/18 10:04
 */

package actorsystem

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/manager"
)

type MessageSys struct {
	Base
}

func (sys *MessageSys) IsOpen() bool {
	return true
}

// OnInit 系统初始化
func (sys *MessageSys) OnInit() {
	sys.loadMsgFromDB(0)
}

func (sys *MessageSys) OnReconnect() {
	sys.loadMsgFromDB(0)
}

// db加载消息
func (sys *MessageSys) loadMsgFromDB(msgId int64) {
	gshare.SendDBMsg(custom_id.GMsgLoadActorMsg, sys.owner.GetId(), msgId)
}

func (sys *MessageSys) onLoadMsgFromDB(msgs []*gshare.MsgSt) {
	for _, line := range msgs {
		st := engine.GetMessagePb3(line.MsgType)
		if nil != st {
			if err := pb3.Unmarshal(line.Msg, st); nil != err {
				sys.LogError("onLoadMsgFromDB error!!! %v", err)
				continue
			}
		}
		sys.OnRecvMessage(line.MsgType, st)
		sys.deleteDBMsg(line.MsgId)
	}
}

func (sys *MessageSys) deleteDBMsg(msgId int64) {
	gshare.SendDBMsg(custom_id.GMsgDeleteActorMsg, sys.owner.GetId(), msgId)
}

// OnRecvMessage 收到消息
func (sys *MessageSys) OnRecvMessage(msgType int, msg pb3.Message) {
	if fn := engine.GetMessageCallback(msgType); nil != fn {
		fn(sys.owner, msg)
	} else {
		sys.LogWarn("收到未注册的玩家消息类型. msgType=%d", msgType)
	}
}

// 玩家新增消息返回
func onAddActorMsgRet(param ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadActorMsgRet", 2, len(param)) {
		return
	}
	playerId, ok := param[0].(uint64)
	if !ok {
		return
	}
	player := manager.GetPlayerPtrById(playerId)
	if nil == player || player.GetLost() {
		return
	}
	msgId, ok := param[1].(int64)
	if !ok {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiMessage).(*MessageSys)
	if ok {
		sys.loadMsgFromDB(msgId)
	}
}

func onLoadActorMsgRet(param ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadActorMsgRet", 2, len(param)) {
		return
	}
	playerId, ok := param[0].(uint64)
	if !ok {
		return
	}
	player := manager.GetPlayerPtrById(playerId)
	if nil == player {
		return
	}
	rets, ok := param[1].([]*gshare.MsgSt)
	if !ok {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiMessage).(*MessageSys)
	if !ok {
		return
	}
	sys.onLoadMsgFromDB(rets)
}

func init() {
	RegisterSysClass(sysdef.SiMessage, func() iface.ISystem {
		return &MessageSys{}
	})
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgAddActorMsg, onAddActorMsgRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadActorMsg, onLoadActorMsgRet)
	})
}
