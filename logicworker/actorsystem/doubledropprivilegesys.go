/**
 * @Author: LvYuMeng
 * @Date: 2025/12/15
 * @Desc: 双倍特权
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type DoubleDropPrivilegeSys struct {
	Base
}

func (s *DoubleDropPrivilegeSys) OnLogin() {
	s.setOpen()
}

func (s *DoubleDropPrivilegeSys) setOpen() {
	data := s.getData()
	var status int64
	if data.ChargeTime > 0 && data.IsOpen {
		status = 1
	}
	s.owner.SetExtraAttr(attrdef.DoubleDropPrivilege, status)
}

func (s *DoubleDropPrivilegeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DoubleDropPrivilegeSys) OnAfterLogin() {
	s.s2cInfo()
	s.owner.SendProto3(37, 36, &pb3.S2C_37_36{UseCount: s.getData().Times})
}

func (s *DoubleDropPrivilegeSys) getData() *pb3.DoubleDropPrivilege {
	binary := s.GetBinaryData()
	if binary.DoubleDropPrivilege == nil {
		binary.DoubleDropPrivilege = &pb3.DoubleDropPrivilege{}
	}
	if binary.DoubleDropPrivilege.Times == nil {
		binary.DoubleDropPrivilege.Times = make(map[uint32]uint32)
	}
	return binary.DoubleDropPrivilege
}

func (s *DoubleDropPrivilegeSys) s2cInfo() {
	data := s.getData()
	s.SendProto3(37, 35, &pb3.S2C_37_35{
		IsOpen:     data.IsOpen,
		ChargeTime: data.ChargeTime,
	})
}

func (s *DoubleDropPrivilegeSys) chargeCheck(chargeId uint32) bool {
	conf := jsondata.GetDoubleDropPrivilegeConf()
	if nil == conf {
		return false
	}
	if conf.ChargeId != chargeId {
		return false
	}
	data := s.getData()
	if data.ChargeTime > 0 {
		return false
	}
	return true
}

func (s *DoubleDropPrivilegeSys) chargeBack(chargeId uint32) bool {
	if !s.chargeCheck(chargeId) {
		return false
	}
	data := s.getData()
	data.IsOpen = true
	data.ChargeTime = time_util.NowSec()
	s.setOpen()
	s.s2cInfo()
	engine.BroadcastTipMsgById(tipmsgid.TipDoubleDropPrivilege, s.owner.GetId(), s.owner.GetName())
	s.ResetSysAttr(attrdef.SaDoubleDropPrivilege)
	return true
}

func (s *DoubleDropPrivilegeSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	if data.ChargeTime == 0 {
		return
	}
	conf := jsondata.GetDoubleDropPrivilegeConf()
	if nil == conf {
		return
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, conf.Attrs)
}

func (s *DoubleDropPrivilegeSys) c2sSetOpen(msg *base.Message) error {
	var req pb3.C2S_37_38
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	data := s.getData()
	if data.ChargeTime == 0 {
		return neterror.ParamsInvalidError("not active")
	}
	data.IsOpen = req.IsOpen
	s.setOpen()
	s.SendProto3(37, 38, &pb3.S2C_37_38{IsOpen: data.IsOpen})
	return nil
}

func (s *DoubleDropPrivilegeSys) SaveTimes(times map[uint32]uint32) {
	data := s.getData()
	data.Times = times
}

func (s *DoubleDropPrivilegeSys) onNewDay() {
	data := s.getData()
	data.Times = nil
	err := s.owner.CallActorFunc(actorfuncid.G2FClearDoubleDropPrivilege, &pb3.CommonSt{
		BParam: data.IsOpen,
	})
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *DoubleDropPrivilegeSys) PackCreateData(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}
	createData.DoubleDropPrivilegeInfo = &pb3.DoubleDropPrivilegeInfo{Times: s.getData().Times}
}

func doubleDropPrivilegeChargeChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiDoubleDropPrivilege).(*DoubleDropPrivilegeSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeCheck(chargeConf.ChargeId)
}

func doubleDropPrivilegeChargeChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiDoubleDropPrivilege).(*DoubleDropPrivilegeSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeBack(chargeConf.ChargeId)
}

func calcDoubleDropPrivilege(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDoubleDropPrivilege).(*DoubleDropPrivilegeSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiDoubleDropPrivilege, func() iface.ISystem {
		return &DoubleDropPrivilegeSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaDoubleDropPrivilege, calcDoubleDropPrivilege)
	engine.RegChargeEvent(chargedef.DoubleDropPrivilege, doubleDropPrivilegeChargeChargeCheck, doubleDropPrivilegeChargeChargeBack)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiDoubleDropPrivilege).(*DoubleDropPrivilegeSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	net.RegisterSysProtoV2(37, 38, sysdef.SiDoubleDropPrivilege, func(s iface.ISystem) func(*base.Message) error {
		return s.(*DoubleDropPrivilegeSys).c2sSetOpen
	})
}
