/**
 * @Author: DaiGuanYu
 * @Desc:
 * @Date: 2021/12/28 16:12
 */

package disableproto

import (
	"jjyz/gameserver/logicworker/gm/gmflag"
)

func getDisableProtos() map[uint32]bool {
	if nil == gmflag.GetGmCmdData().DisableProto {
		gmflag.GetGmCmdData().DisableProto = make(map[uint32]bool)
	}
	return gmflag.GetGmCmdData().DisableProto
}

func SetDisableProto(proto uint32) {
	protoMap := getDisableProtos()
	if _, ok := protoMap[proto]; ok {
		return
	}

	protoMap[proto] = true
}

func IsDisableProto(proto uint32) bool {
	protoMap := getDisableProtos()
	if _, ok := protoMap[proto]; ok {
		return true
	}

	return false
}

func DelDisableProto(proto uint32) {
	protoMap := getDisableProtos()
	if IsDisableProto(proto) {
		delete(protoMap, proto)
	}
}
