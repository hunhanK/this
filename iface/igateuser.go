/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/9/29 19:42
 */

package iface

import (
	"jjyz/base/pb3"
)

type IGateUser interface {
	GetPlayerId() uint64
	Reset()
	SendProto3(protoH, protoL uint16, msg pb3.Message)
}
