package net

import (
	"errors"
	"jjyz/base"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net/internal"
	"log"
	"reflect"
)

const (
	ProtoType_Unknown  = iota
	ProtoType_PSys     // 个人系统协议
	ProtoType_Pyy      // 个人运营活动协议
	ProtoType_GlobalYy // 全服运营活动协议
)

type protoFunc func(player iface.IPlayer, msg *base.Message) error

var FightProtoList map[uint16]struct{}
var DirectTbl map[uint16]base.ServerType

func RegisterSysProtoV2(protoIdH, protoIdL uint16, sysId int16, fn internal.SysHdlFunc) {
	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		SysId:      sysId,
		Type:       ProtoType_PSys,
		SysHdlFunc: fn,
	})
}

// 注册系统协议
func RegisterSysProto(protoIdH, protoIdL uint16, sysId int16, fn interface{}) {
	if nil == fn {
		log.Panicf("(%d, %d) fn is nil", protoIdH, protoIdL)
		return
	}
	f := reflect.ValueOf(fn)
	if f.Kind() != reflect.Func {
		log.Panicf("(%d, %d) fn non-func type", protoIdH, protoIdL)
		return
	}

	ft := f.Type()
	numIn := ft.NumIn()
	if numIn != 2 {
		log.Panicf("(%d, %d) func args's num mismatch", protoIdH, protoIdL)
		return
	}
	//todo 类型判断
	args1 := f.Type().In(0)
	if args1.Kind() != reflect.Ptr {
		log.Panicf("func args[0]'s type mismatch")
		return
	}

	creator, ok := engine.ClassSet[uint32(sysId)]
	if !ok {
		log.Panicf("creator is nil sysId %d", sysId)
	}

	if !args1.ConvertibleTo(reflect.TypeOf(creator())) {
		log.Panicf("args[0] expect:%v, got:%v", reflect.TypeOf(creator()), args1)
	}

	args2 := f.Type().In(1)
	if args2.Kind() != reflect.Ptr {
		log.Panicf("func args[1]'s num mismatch 1")
		return
	}
	if !args2.ConvertibleTo(reflect.TypeOf(&base.Message{})) {
		log.Panicf("args[1] expect: *base.Message, got:%v", args2)
		return
	}

	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		SysId: sysId,
		Type:  ProtoType_PSys,
		Caller: &internal.Caller{
			FnValue: f,
			FnNumIn: numIn,
		},
	})
}

// RegisterYYSysProtoV2 v2 avoid reflection to waste system resource, priority to use it
func RegisterYYSysProtoV2(protoIdH, protoIdL uint16, fn internal.YYSysHdlFunc) {
	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		Type:         ProtoType_Pyy,
		YYSysHdlFunc: fn,
	})
}

func RegisterGlobalYYSysProto(protoIdH, protoIdL uint16, fn internal.GlobalYYSysHdlFunc) {
	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		Type:               ProtoType_GlobalYy,
		GlobalYYSysHdlFunc: fn,
	})
}

// 注册运营协议
func RegisterYYSysProto(protoIdH, protoIdL uint16, fn interface{}) {
	if nil == fn {
		log.Panicf("(%d, %d) fn is nil", protoIdH, protoIdL)
		return
	}

	f := reflect.ValueOf(fn)
	if f.Kind() != reflect.Func {
		log.Panicf("(%d, %d) fn non-func type", protoIdH, protoIdL)
		return
	}

	ft := f.Type()
	numIn := ft.NumIn()
	if numIn != 2 {
		log.Panicf("(%d, %d) func args's num mismatch", protoIdH, protoIdL)
		return
	}

	//todo 类型判断
	args1 := f.Type().In(0)
	if args1.Kind() != reflect.Ptr {
		log.Panicf("func args[0]'s type mismatch")
		return
	}

	args2 := f.Type().In(1)
	if args2.Kind() != reflect.Ptr {
		log.Panicf("func args[1]'s num mismatch 1")
		return
	}

	if !args2.ConvertibleTo(reflect.TypeOf(&base.Message{})) {
		log.Panicf("args[1] expect: *base.Message, got:%v", args2)
		return
	}

	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		Type: ProtoType_Pyy,
		Caller: &internal.Caller{
			FnValue: f,
			FnNumIn: numIn,
		},
	})
}

func RegisterProto(protoIdH, protoIdL uint16, fn protoFunc) {
	internal.ActualRegister(protoIdH, protoIdL, &internal.ProtoInfo{
		Type: ProtoType_Unknown,
		Func: fn,
	})
}

func GetRegister(sysId, cmdId uint16) *internal.ProtoInfo {
	protoId := sysId<<8 | cmdId
	if info, ok := internal.ProtoTbl[protoId]; ok {
		return info
	}
	return nil
}

func Call(caller *internal.Caller, args ...interface{}) ([]reflect.Value, error) {
	if nil == caller {
		return nil, errors.New("caller is nil")
	}
	size := len(args)
	if size != caller.FnNumIn {
		return nil, errors.New("the number of input args not match")
	}
	in := make([]reflect.Value, size)
	for idx, obj := range args {
		in[idx] = reflect.ValueOf(obj)
	}

	retValues := caller.FnValue.Call(in)

	if len(retValues) == 0 {
		return retValues, nil
	}

	if retValues[0].CanInterface() {
		switch err := retValues[0].Interface().(type) {
		case error:
			return retValues, err
		default:
			return retValues, nil
		}
	}

	return retValues, nil
}

func onRegisterFightProto(buf []byte) {
	msg := &pb3.FightProtoList{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	list := make(map[uint16]struct{}, len(msg.Protos))
	for _, protoId := range msg.Protos {
		list[uint16(protoId)] = struct{}{}
	}

	FightProtoList = list

	tbl := make(map[uint16]base.ServerType)
	for protoId, serverType := range msg.DirectTbl {
		tbl[uint16(protoId)] = base.ServerType(serverType)
	}

	DirectTbl = tbl
}

func init() {
	engine.RegisterSysCall(sysfuncid.FGRegistFightProto, onRegisterFightProto)
}
