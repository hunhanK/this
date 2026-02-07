package entity

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
)

type ActorProxy struct {
	fightType base.ServerType
	ActorId   uint64
	Handle    uint64
}

func NewActorProxy(player iface.IPlayer, fightType base.ServerType) *ActorProxy {
	proxy := &ActorProxy{}
	proxy.fightType = fightType
	proxy.ActorId = player.GetId()
	return proxy
}

func (proxy *ActorProxy) GetProxyType() base.ServerType {
	return proxy.fightType
}

func (proxy *ActorProxy) ToDo(todoId uint32, msg pb3.Message) error {
	data, err := pb3.Marshal(msg)
	if nil != err {
		return fmt.Errorf("ActorProxy ToDo failed %s", err)
	}

	todo := &pb3.ToDo{
		ActorId: proxy.ActorId,
		TodoId:  todoId,
		TodoBuf: data,
	}

	return engine.SendToFight(proxy.fightType, cmd.GFToDo, todo)
}

func (proxy *ActorProxy) SwitchFtype(ftype base.ServerType) error {
	if !ftype.IsFightSrv() {
		return fmt.Errorf("fight srv type(%d) is invalid", ftype)
	}
	proxy.fightType = ftype
	return nil
}

func (proxy *ActorProxy) InitEnter(player iface.IPlayer, todoId uint32, todoBuf []byte) error {
	req := &pb3.EnterFight{
		PfId:    gshare.GameConf.PfId,
		SrvId:   gshare.GameConf.SrvId,
		ActorId: proxy.ActorId,
		TodoId:  todoId,
		TodoBuf: todoBuf,
	}

	req.CreateData = player.PackCreateData()
	if data, ok := manager.GetData(proxy.ActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		req.BaseData = data
	}

	return engine.SendToFight(proxy.fightType, cmd.G2FInitEnter, req)
}

func (proxy *ActorProxy) PackCallActorFuncProto(fnId uint16, msg pb3.Message) (pb3.Message, error) {
	st := &pb3.CallActorFunc{}
	st.FnId = uint32(fnId)
	st.ActorId = proxy.ActorId
	if nil != msg {
		data, err := pb3.Marshal(msg)
		if nil != err {
			return nil, fmt.Errorf("PackCallActorFuncProtoExpire error:%v", err)
		}

		st.Data = data
	}

	return st, nil
}

func (proxy *ActorProxy) PackActorCallSysFnProto(fnId uint16, msg pb3.Message) (pb3.Message, error) {
	st := &pb3.CallActorSmallCrossFunc{}
	st.FnId = uint32(fnId)
	st.ActorId = proxy.ActorId
	st.PfId = engine.GetPfId()
	st.SrvId = engine.GetServerId()
	if nil != msg {
		data, err := pb3.Marshal(msg)
		if nil != err {
			return nil, fmt.Errorf("PackCallActorFuncProtoExpire error:%v", err)
		}

		st.Data = data
	}
	return st, nil
}

func (proxy *ActorProxy) Exit() {
	st := pb3.ExitFight{
		ActorId: proxy.ActorId,
		Handle:  proxy.Handle,
	}

	if err := engine.CallFightSrvFunc(proxy.fightType, sysfuncid.GFExitFight, &st); nil != err {
		logger.LogError("ActorProxy Exit failed %v", err)
	}
}

func (proxy *ActorProxy) CallActorFunc(fnId uint16, msg pb3.Message) error {
	st, err := proxy.PackCallActorFuncProto(fnId, msg)
	if nil != err {
		return err
	}
	return engine.SendToFight(proxy.fightType, cmd.GFCallActorFunc, st)
}

// CallActorSysFn 先加上 如果有第三个出现 可以考虑抽出来做一个工厂方法
func (proxy *ActorProxy) CallActorSysFn(ftype base.ServerType, fnId uint16, msg pb3.Message) error {
	st, err := proxy.PackActorCallSysFnProto(fnId, msg)
	if nil != err {
		return err
	}
	return engine.SendToFight(ftype, cmd.GFActorCallSysFn, st)
}

func (proxy *ActorProxy) OnRawProto(msg *base.Message) {
	st := &pb3.ForwardClientMsgToFight{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: proxy.ActorId,
		Header:  msg.Header,
		Data:    msg.Data,
	}
	err := engine.SendToFight(proxy.fightType, cmd.GFToFightClientMsg, st)
	if err != nil {
		logger.LogError("err:%v", err)
	}
}
