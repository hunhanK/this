/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2022/10/31 13:35
 */

package gshare

import (
	"github.com/gzjjyz/safeworker"
)

var (
	RegisterSDKMsgHandler func(msgId safeworker.MsgIdType, hdl safeworker.MsgHdlType)
	SendSDkMsg            func(id safeworker.MsgIdType, params ...interface{})
)
