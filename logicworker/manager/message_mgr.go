package manager

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
)

// SendPlayerMessage 给玩家发消息
func sendPlayerMessage(playerId uint64, msgType int, msg pb3.Message) {
	if playerId <= 0 {
		logger.LogError("Add offline message wrong playerId:%d type:%d", playerId, msgType)
		return
	}
	player := GetPlayerPtrById(playerId)
	if nil != player && !player.GetLost() {
		player.OnRecvMessage(msgType, msg)
		return
	}
	buf, err := pb3.Marshal(msg)
	if nil != err {
		logger.LogError("SendPlayerMessage error! err:%v", err)
		return
	}
	if len(buf) > 255 {
		logger.LogError("SendPlayerMessage error! the size of msg buf is too large")
		return
	}
	gshare.SendDBMsg(custom_id.GMsgAddActorMsg, playerId, msgType, buf)
}

func init() {
	engine.SendPlayerMessage = sendPlayerMessage
}
