/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:54
 */

package gshare

import (
	"github.com/gzjjyz/safeworker"
	"jjyz/base/custom_id"
)

var (
	RegisterDBMsgHandler func(msgId safeworker.MsgIdType, hdl safeworker.MsgHdlType)
	SendDBMsg            func(id safeworker.MsgIdType, params ...interface{})
)

func SendPlayerStaticsMsg(playerId uint64, raw map[string]interface{}) {
	SendDBMsg(custom_id.GMsgPlayerStatics, playerId, raw)
}
