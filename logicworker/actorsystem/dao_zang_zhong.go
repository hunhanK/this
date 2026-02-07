/**
 * @Author: yzh
 * @Date:
 * @Desc: 道藏塚
 * @Modify：
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type DaoZangZhongSys struct {
	Base
}

func (s *DaoZangZhongSys) OnLogin() {
	s.NotifyCurLayer()
}

func (s *DaoZangZhongSys) OnReconnect() {
	s.NotifyCurLayer()
}

func (s *DaoZangZhongSys) NotifyCurLayer() {
	nextLayer := s.GetBinaryData().PassDaoZangZhongLayer + 1
	if nextLayer > uint32(len(jsondata.GetDaoZangZhongConf().LayerConf)) {
		nextLayer = 0
	}
	s.SendProto3(17, 101, &pb3.S2C_17_101{
		CurLayer: nextLayer,
	})
}

func (s *DaoZangZhongSys) C2SEnterLayer(_ *base.Message) {
	if !s.IsOpen() {
		s.owner.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}

	if s.GetOwner().InDartCar() {
		s.GetOwner().SendTipMsg(tipmsgid.Tpindartcar)
		return
	}

	s.EnterLayer()
}

func (s *DaoZangZhongSys) EnterLayer() {
	enterLayer := s.GetBinaryData().PassDaoZangZhongLayer + 1
	if conf := jsondata.GetDaoZangZhongLayerConf(enterLayer); conf == nil {
		s.LogWarn("pass layer is max")
		return
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogDaoZZEnter, &pb3.LogPlayerCounter{
		NumArgs: uint64(enterLayer),
	})

	err := s.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterDaoZangZhong, &pb3.EnterDaoZangZhongReq{
		Layer: enterLayer,
	})
	if err != nil {
		s.LogError("Enter DaoZangZhong failed level %d err: %s", enterLayer, err)
		return
	}
}

func (s *DaoZangZhongSys) PassLayer(layer uint32) {
	if s.owner.GetBinaryData().PassDaoZangZhongLayer > layer {
		return
	}
	s.GetBinaryData().PassDaoZangZhongLayer++
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogPassDaoZangZhong, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetBinaryData().PassDaoZangZhongLayer),
	})
	s.owner.TriggerQuestEvent(custom_id.QttUpgradeTianGuanLayer, 0, int64(s.GetBinaryData().PassDaoZangZhongLayer))
}

func PassLayer(player iface.IPlayer, buf []byte) {
	var msg pb3.PassDaoZangZhongLayerMsg

	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	if msg.MasterActorId != player.GetId() {
		player.TriggerQuestEvent(custom_id.QttHelpPassDZZFb, 0, 1)
		return
	}
	player.GetSysObj(sysdef.SiDaoZangzhong).(*DaoZangZhongSys).PassLayer(msg.Layer)

	player.TriggerQuestEvent(custom_id.QttPassDZZFbLevel, 0, int64(msg.Layer))
	player.SendProto3(17, 103, &pb3.S2C_17_103{
		Layer:     msg.Layer,
		AmIHelper: msg.MasterActorId != player.GetId(),
		HasDrop:   msg.HasDrop,
	})
}

func Exit(player iface.IPlayer, buf []byte) {
	var req pb3.ExitDaoZangZhongReq

	if err := pb3.Unmarshal(buf, &req); err != nil {
		return
	}

	if req.MasterActorId != player.GetId() {
		return
	}

	player.GetSysObj(sysdef.SiDaoZangzhong).(*DaoZangZhongSys).NotifyCurLayer()
}

func AskHelperToWorld(player iface.IPlayer, buf []byte) {
	var req pb3.AskHelpForDaoZangZhongToGuildReq

	if err := pb3.Unmarshal(buf, &req); err != nil {
		return
	}

	engine.Broadcast(chatdef.CIGuild, 0, 17, 102, &pb3.S2C_17_102{
		ActorId: player.GetId(),
		Layer:   req.Layer,
	}, 0)

	logworker.LogPlayerBehavior(player, pb3.LogId_LogBehavAskHelperInDaoZangZhong, &pb3.LogPlayerCounter{
		NumArgs: uint64(player.GetBinaryData().PassDaoZangZhongLayer),
	})
}

func EnterLayerSucc(player iface.IPlayer, buf []byte) {
	var msg pb3.EnterDaoZangZhongLayerSuccMsg

	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}
	amIHelper := msg.MasterActorId != player.GetId()
	player.SendProto3(17, 104, &pb3.S2C_17_104{
		CurLayer:  msg.Layer,
		AmIHelper: amIHelper,
	})
}

func init() {
	RegisterSysClass(sysdef.SiDaoZangzhong, func() iface.ISystem {
		return &DaoZangZhongSys{}
	})

	net.RegisterSysProto(17, 101, sysdef.SiDaoZangzhong, (*DaoZangZhongSys).C2SEnterLayer)

	engine.RegisterActorCallFunc(playerfuncid.PassDaoZangZhongLayer, PassLayer)
	engine.RegisterActorCallFunc(playerfuncid.ExitDaoZangZhong, Exit)
	engine.RegisterActorCallFunc(playerfuncid.EnteredDaoZangZhongLayer, EnterLayerSucc)
	engine.RegisterActorCallFunc(playerfuncid.AskHelpForDaoZangZhongToGuild, AskHelperToWorld)

	gmevent.Register("DaoZangZhongSys.setLayer", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiDaoZangzhong)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*DaoZangZhongSys)
		sys.GetBinaryData().PassDaoZangZhongLayer = utils.AtoUint32(args[0])
		sys.NotifyCurLayer()
		return true
	}, 1)
}
