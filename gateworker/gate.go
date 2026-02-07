/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/28 10:15
 */

package gateworker

import (
	"encoding/binary"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

func onGateMsg(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	gateIdx, ok := args[0].(uint32)
	if !ok {
		return
	}
	data, ok := args[1].([]byte)
	if !ok {
		return
	}

	st := GetGateConn(gateIdx)
	if nil == st {
		return
	}

	if len(data) < 5 {
		return
	}

	cmdId := data[0]
	connId := binary.LittleEndian.Uint32(data[1:5])
	switch cmdId {
	case base.GW_OPEN:
		st.OpenNewUser(connId, string(data[5:]))
	case base.GW_CLOSE:
		st.CloseUser(connId)
	case base.GW_DATA:
		st.OnUserMsg(connId, data[5:])
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgRecvGateMsg, onGateMsg)
	})
}
