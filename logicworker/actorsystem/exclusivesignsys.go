/**
 * @Author: LvYuMeng
 * @Date: 2025/12/22
 * @Desc: 专属签名
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type ExclusiveSignSys struct {
	Base
}

func (s *ExclusiveSignSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *ExclusiveSignSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ExclusiveSignSys) OnOpen() {
	s.s2cInfo()
}

func (s *ExclusiveSignSys) s2cInfo() {
	s.SendProto3(11, 185, &pb3.S2C_11_185{Data: s.getData()})
}

func (s *ExclusiveSignSys) getData() *pb3.ExclusiveSign {
	binary := s.GetBinaryData()
	if binary.ExclusiveSign == nil {
		binary.ExclusiveSign = new(pb3.ExclusiveSign)
	}
	if binary.ExclusiveSign.Lv == nil {
		binary.ExclusiveSign.Lv = make(map[uint32]uint32)
	}
	return binary.ExclusiveSign
}

func (s *ExclusiveSignSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_11_186
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return err
	}
	var equipSet bool
	if equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok && equipSys.IsOpen() {
		_, eq := equipSys.GetEquipByPos(req.Pos)
		equipSet = eq != nil
	}
	if !equipSet {
		return neterror.ParamsInvalidError("no equip")
	}
	data := s.getData()
	nextLv := data.Lv[req.Pos] + 1
	nextConf := jsondata.GetExclusiveSignLvConf(req.Pos, nextLv)
	if nextConf == nil {
		return neterror.ConfNotFoundError("pos %d lv %d conf nil", req.Pos, nextLv)
	}
	if !s.owner.ConsumeByConf(nextConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogExclusiveSignLvUpConsume}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Lv[req.Pos] = nextLv
	s.SendProto3(11, 186, &pb3.S2C_11_186{Pos: req.Pos, Lv: nextLv})
	engine.BroadcastTipMsgById(tipmsgid.TipExclusiveSign, s.owner.GetId(), s.owner.GetName())
	s.ResetSysAttr(attrdef.SaExclusiveSign)

	return nil
}

func (s *ExclusiveSignSys) calcAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok || !equipSys.IsOpen() {
		return
	}
	for pos, lv := range data.Lv {
		_, eq := equipSys.GetEquipByPos(pos)
		if eq == nil {
			continue
		}
		conf := jsondata.GetExclusiveSignLvConf(pos, lv)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, conf.Attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiExclusiveSign, func() iface.ISystem {
		return &ExclusiveSignSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaExclusiveSign, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		if sys, ok := player.GetSysObj(sysdef.SiExclusiveSign).(*ExclusiveSignSys); ok && sys.IsOpen() {
			sys.calcAttrs(calc)
		}
	})

	net.RegisterSysProtoV2(11, 186, sysdef.SiExclusiveSign, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ExclusiveSignSys).c2sLvUp
	})
}
