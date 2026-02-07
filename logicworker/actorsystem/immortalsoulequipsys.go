/**
 * @Author: lzp
 * @Date: 2024/10/29
 * @Desc: 武魂神器
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type ImmortalSoulEquipSys struct {
	Base
}

func (s *ImmortalSoulEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ImmortalSoulEquipSys) OnLogin() {
	s.s2cInfo()
}

func (s *ImmortalSoulEquipSys) GetData() map[uint32]*pb3.BattleSoulEquipData {
	binary := s.GetBinaryData()
	if binary.BattleSoulEData == nil {
		binary.BattleSoulEData = make(map[uint32]*pb3.BattleSoulEquipData)
	}
	return binary.BattleSoulEData
}

func (s *ImmortalSoulEquipSys) s2cInfo() {
	data := s.GetData()
	s.SendProto3(11, 87, &pb3.S2C_11_87{Data: data})
}

func (s *ImmortalSoulEquipSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_11_85
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetImmortalSoulEquipStarConf(req.Id, 0)
	if conf == nil {
		return neterror.ConfNotFoundError("immortal soul equip=%d not found", req.Id)
	}

	data := s.GetData()
	_, ok := data[req.Id]
	if ok {
		return neterror.ParamsInvalidError("immortal soul equip=%d has activated", req.Id)
	}

	if !s.GetOwner().ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogImmortalSoulEquipUpStar}) {
		return neterror.ParamsInvalidError("immortal soul equip active consume not enough: %v", req.Id)
	}

	data[req.Id] = &pb3.BattleSoulEquipData{Id: req.Id, Star: 0}
	s.onStarChange(req.Id)
	s.SendProto3(11, 85, &pb3.S2C_11_85{Id: req.Id})
	return nil
}

func (s *ImmortalSoulEquipSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_11_86
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	iData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("immortal soul equip not activated: %d", req.Id)
	}

	nextStar := iData.Star + 1
	conf := jsondata.GetImmortalSoulEquipStarConf(req.Id, nextStar)
	if conf == nil {
		return neterror.ConfNotFoundError("immortal soul equip star conf not found: %d, %d", req.Id, nextStar)
	}

	if !s.GetOwner().ConsumeByConf(conf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogImmortalSoulEquipUpStar}) {
		return neterror.ParamsInvalidError("immortal soul equip star consume not enough: %v", req.Id)
	}

	iData.Star = nextStar
	s.onStarChange(req.Id)
	s.SendProto3(11, 86, &pb3.S2C_11_86{Id: req.Id, Star: nextStar})
	return nil
}

func (s *ImmortalSoulEquipSys) onStarChange(id uint32) {
	data := s.GetData()
	iData, ok := data[id]
	if !ok {
		return
	}
	conf := jsondata.GetImmortalSoulEquipStarConf(id, iData.Star)
	if conf == nil {
		return
	}

	if conf.SkillId > 0 {
		s.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true)
	}

	s.ResetSysAttr(attrdef.SaImmortalSoulEquip)
}

func (s *ImmortalSoulEquipSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for _, v := range data {
		conf := jsondata.GetImmortalSoulEquipStarConf(v.Id, v.Star)
		if conf == nil {
			continue
		}

		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, conf.Attrs)
	}
}

func immortalSoulEquipProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoulEquip).(*ImmortalSoulEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiImmortalSoulEquip, func() iface.ISystem {
		return &ImmortalSoulEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaImmortalSoulEquip, immortalSoulEquipProperty)

	net.RegisterSysProto(11, 85, sysdef.SiImmortalSoulEquip, (*ImmortalSoulEquipSys).c2sActivate)
	net.RegisterSysProto(11, 86, sysdef.SiImmortalSoulEquip, (*ImmortalSoulEquipSys).c2sUpStar)
}
