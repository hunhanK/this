/**
 * @Author: HeXinLi
 * @Desc: 逻辑服调用战斗服
 * @Date: 2021/9/6 15:51
 */

package engine

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
)

type SysCallFunc func(buf []byte)

var fightSrvCallMap = make(map[uint16]SysCallFunc)

// RegisterSysCall 注册给战斗服调用的函数
func RegisterSysCall(fnId uint16, fn SysCallFunc) {
	if nil == fn {
		return
	}

	if _, ok := fightSrvCallMap[fnId]; ok {
		return
	}

	fightSrvCallMap[fnId] = fn
}

func GetSysCall(fnId uint16) SysCallFunc {
	return fightSrvCallMap[fnId]
}

// CallFightSrvFunc 逻辑服调用战斗服函数
func CallFightSrvFunc(ftype base.ServerType, fnId uint16, msg pb3.Message) error {
	st := pb3.CallFightSrvFunc{
		FnId: uint32(fnId),
	}
	if nil != msg {
		data, err := pb3.Marshal(msg)
		if nil != err {
			logger.LogError("OnCallFightSrv error:%v", err)
			return err
		}
		st.Data = data
	}
	err := SendToFight(ftype, cmd.GFCallFightSrvFunc, &st)
	if err != nil {
		return err
	}
	if msg == nil {
		logger.LogTrace("[RPC.CallFightSrvFunc] fnId:%d", fnId)
		return nil
	}
	logger.LogTrace("[RPC.CallFightSrvFunc] req:%s{%+v}", msg.ProtoReflect().Descriptor().FullName(), msg)
	return nil
}

// OnFightSrvCreateMonster 调用战斗服创建怪物
func OnFightSrvCreateMonster(ftype base.ServerType, fbId, sceneId, monId uint32, args ...uint32) {
	msg := &pb3.CreateMonster_Common{
		FbId:    fbId,
		SceneId: sceneId,
		MonId:   monId,
	}

	if len(args) > 0 {
		for i := 0; i < len(args); i++ {
			msg.ExtraData = append(msg.ExtraData, args[i])
		}
	}
	//logger.LogDebug("逻辑服通知战斗服创建怪物 fbId:%v, 场景id:%v, 怪物id:%v", fbId, sceneId, monId)
	CallFightSrvFunc(ftype, sysfuncid.GFCreateMonster, msg)
}
