package internal

import (
	"jjyz/base"
	"jjyz/gameserver/iface"
	"reflect"

	"github.com/gzjjyz/logger"
)

// 协议注册表
var (
	ProtoTbl = make(map[uint16]*ProtoInfo)
)

type (
	//协议信息
	ProtoInfo struct {
		SysId  int16 //系统id
		ActId  int16 //活动id
		Type   int8  //协议类型
		Func   func(actor iface.IPlayer, msg *base.Message) error
		Caller *Caller // declared
		YYSysHdlFunc
		SysHdlFunc
		GlobalYYSysHdlFunc
	}

	SysHdlFunc         func(sys iface.ISystem) func(*base.Message) error
	YYSysHdlFunc       func(sys iface.IPlayerYY) func(*base.Message) error
	GlobalYYSysHdlFunc func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error

	//协议绑定函数
	Caller struct {
		FnValue reflect.Value //函数的reflect.value
		FnNumIn int           //函数的输入参数数量
	}
)

func ActualRegister(protoIdH, protoIdL uint16, obj *ProtoInfo) {
	protoId := protoIdH<<8 | protoIdL
	if _, ok := ProtoTbl[protoId]; ok {
		logger.LogStack("proto sysId:%d, cmdId:%d register repeat.", protoIdH, protoIdL)
		return
	}
	ProtoTbl[protoId] = obj
}
