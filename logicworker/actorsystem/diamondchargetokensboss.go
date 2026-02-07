/**
 * @Author: LvYuMeng
 * @Date: 2025/12/15
 * @Desc: 真充升级
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type DiamondChargeTokensBoss struct {
	Base
}

func (s *DiamondChargeTokensBoss) getData() *pb3.DiamondChargeTokensBoss {
	binary := s.GetBinaryData()
	if binary.DiamondChargeTokensBoss == nil {
		binary.DiamondChargeTokensBoss = &pb3.DiamondChargeTokensBoss{}
	}
	return binary.DiamondChargeTokensBoss
}

func (s *DiamondChargeTokensBoss) OnAfterLogin() {
	s.s2cInfo()
}

func (s *DiamondChargeTokensBoss) OnReconnect() {
	s.s2cInfo()
}

func (s *DiamondChargeTokensBoss) OnOpen() {
	s.s2cInfo()
}

func (s *DiamondChargeTokensBoss) s2cInfo() {
	s.SendProto3(36, 9, &pb3.S2C_36_9{Data: s.getData()})
}

func (s *DiamondChargeTokensBoss) onBossKill(count uint32) {
	data := s.getData()
	if !data.IsOpen {
		return
	}

	conf := jsondata.GetDiamondChargeTokensBossConf()
	if nil == conf || conf.SingleLimit == 0 {
		return
	}

	limit := conf.AddLimit / conf.SingleLimit
	if data.Count >= limit {
		return
	}

	if data.Count+count >= limit {
		data.Count = limit
	} else {
		data.Count += count
	}

	s.s2cInfo()
	s.ResetSysAttr(attrdef.SaDiamondChargeTokensBoss)
	return
}

func (s *DiamondChargeTokensBoss) c2sOpen(msg *base.Message) error {
	var req pb3.C2S_36_9
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	data := s.getData()
	if data.IsOpen {
		return neterror.ParamsInvalidError("opened")
	}
	data.IsOpen = true
	s.s2cInfo()
	return nil
}

func (s *DiamondChargeTokensBoss) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	conf := jsondata.GetDiamondChargeTokensBossConf()
	if nil == conf {
		return
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, jsondata.AttrVec{
		&jsondata.Attr{
			Type:  attrdef.DiamondChargeTokensPermanentLimit,
			Value: data.Count * conf.SingleLimit,
		},
	})
}

func onKillDiamondChargeTokensBoss(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}

	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	if jsondata.GetMonsterType(monId) != custom_id.MtBoss {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiDiamondChargeTokensBoss).(*DiamondChargeTokensBoss)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.onBossKill(count)
}

func calcDiamondChargeTokensBoss(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiDiamondChargeTokensBoss)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys, ok := obj.(*DiamondChargeTokensBoss)
	if !ok {
		return
	}

	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiDiamondChargeTokensBoss, func() iface.ISystem {
		return &DiamondChargeTokensBoss{}
	})

	engine.RegAttrCalcFn(attrdef.SaDiamondChargeTokensBoss, calcDiamondChargeTokensBoss)

	net.RegisterSysProtoV2(36, 9, sysdef.SiDiamondChargeTokensBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DiamondChargeTokensBoss).c2sOpen
	})

	event.RegActorEvent(custom_id.AeKillMon, onKillDiamondChargeTokensBoss)
}
