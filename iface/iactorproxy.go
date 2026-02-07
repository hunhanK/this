/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/26 11:11
 */

package iface

import (
	"jjyz/base"
	"jjyz/base/pb3"
)

type IActorProxy interface {
	OnRawProto(msg *base.Message)
	GetProxyType() base.ServerType
	InitEnter(player IPlayer, todoId uint32, todoBuf []byte) error
	ToDo(todoId uint32, msg pb3.Message) error
	SwitchFtype(ftype base.ServerType) error
	Exit()

	CallActorFunc(fnId uint16, msg pb3.Message) error
	CallActorSysFn(ftype base.ServerType, fnId uint16, msg pb3.Message) error
}
