/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/29 9:49
 */

package gshare

import (
	"github.com/gzjjyz/safeworker"
)

var (
	RegisterGameMsgHandler func(msgId safeworker.MsgIdType, hdl safeworker.MsgHdlType)
	SendGameMsg            func(id safeworker.MsgIdType, params ...interface{})
	IsActorInThisServer    func(actorId uint64) bool
	AddMoneyOffline        func(actorId uint64, moneyType, value uint32, bTip bool, logId uint32)
)
