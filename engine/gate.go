/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/29 22:48
 */

package engine

import (
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

var (
	Broadcast         func(channelId uint32, param int64, sysId, cmdId uint16, msg pb3.Message, level uint32)
	BroadcastBuf      func(channelId uint32, param int64, sysId, cmdId uint16, buf []byte, level uint32)
	PlayerLevelChange func(actorId int64, level uint32)
	CloseGateUser     func(user iface.IGateUser, reason uint16)
)
