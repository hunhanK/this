/**
 * @Author: lzp
 * @Date: 2025/2/10
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"math"
)

type DragonSoulAppearSys struct {
	Base
}

func (s *DragonSoulAppearSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DragonSoulAppearSys) OnLogin() {
	s.s2cInfo()
}

func (s *DragonSoulAppearSys) OnInit() {
	binary := s.GetBinaryData()
	if binary.DragonSoulAppears == nil {
		binary.DragonSoulAppears = make(map[uint32]*pb3.DSAppearData)
	}
}

func (s *DragonSoulAppearSys) s2cInfo() {
	dataMap := s.GetBinaryData().DragonSoulAppears
	s.SendProto3(50, 15, &pb3.S2C_50_15{Data: dataMap})
}

func (s *DragonSoulAppearSys) getData(appearId uint32) *pb3.DSAppearData {
	data := s.GetBinaryData().DragonSoulAppears
	if data[appearId] == nil {
		data[appearId] = &pb3.DSAppearData{}
	}
	return data[appearId]
}

func (s *DragonSoulAppearSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_50_17
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	chipConf := jsondata.GetDragonSoulAppearChipConf(req.AppearId, req.Pos)
	if chipConf == nil {
		return neterror.ConfNotFoundError("dragonSoulAppearChip config not found, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if chipItem == nil || chipItem.Type != itemdef.ItemDragonSoulAppearChip {
		return neterror.ParamsInvalidError("chip item error, chipItem: %d", chipConf.ChipId)
	}

	chipLvConf := jsondata.GetDragonSoulAppearChipLvConf(req.AppearId, req.Pos, 1)
	if chipLvConf == nil {
		return neterror.ConfNotFoundError("dragonSoulAppearChip lv config error, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	data := s.getData(req.AppearId)
	if data.AppearChips != nil && data.AppearChips[req.Pos] > 0 {
		return neterror.ParamsInvalidError("dragonSoulAppearChip has activated, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	if !s.GetOwner().ConsumeByConf(chipLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDragonSoulAppearChipUpLv}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	if data.AppearChips == nil {
		data.AppearChips = make(map[uint32]uint32)
	}
	data.AppearChips[req.Pos] = 1
	s.ResetSysAttr(attrdef.SaDragonSoulAppear)
	s.SendProto3(50, 16, &pb3.S2C_50_16{
		AppearId: req.AppearId,
		Pos:      req.Pos,
		Lv:       1,
	})
	return nil
}

func (s *DragonSoulAppearSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_50_18
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	chipConf := jsondata.GetDragonSoulAppearChipConf(req.AppearId, req.Pos)
	if chipConf == nil {
		return neterror.ConfNotFoundError("dragonSoulAppearChip config not found, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if chipItem == nil || chipItem.Type != itemdef.ItemDragonSoulAppearChip {
		return neterror.ParamsInvalidError("chip item error, chipItem: %d", chipConf.ChipId)
	}

	data := s.getData(req.AppearId)
	if data.AppearChips == nil || data.AppearChips[req.Pos] == 0 {
		return neterror.ParamsInvalidError("chip not activate, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	nextLv := data.AppearChips[req.Pos] + 1
	chipLvConf := jsondata.GetDragonSoulAppearChipLvConf(req.AppearId, req.Pos, nextLv)
	if chipLvConf == nil {
		return neterror.ConfNotFoundError("chip lv config not found, id: %d, pos: %d", req.AppearId, req.Pos)
	}

	if !s.GetOwner().ConsumeByConf(chipLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDragonSoulAppearChipUpLv}) {
		return neterror.ConsumeFailedError("chip lev up failed")
	}

	data.AppearChips[req.Pos] = nextLv
	s.ResetSysAttr(attrdef.SaDragonSoulAppear)
	s.SendProto3(50, 16, &pb3.S2C_50_16{
		AppearId: req.AppearId,
		Pos:      req.Pos,
		Lv:       nextLv,
	})
	return nil
}

func (s *DragonSoulAppearSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_50_19
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if req.AppearId == 0 {
		s.GetOwner().TakeOffAppear(appeardef.AppearPos_DragonSoul)
		return nil
	}

	aConf := jsondata.GetDragonSoulAppearConf(req.AppearId)
	if aConf == nil {
		return neterror.ParamsInvalidError("get appear conf error appearId: %d", req.AppearId)
	}

	data := s.getData(req.AppearId)
	if len(data.AppearChips) < len(aConf.Chips) {
		return neterror.ParamsInvalidError("cannot takeOn appear, id: %d", req.AppearId)
	}

	s.GetOwner().TakeOnAppear(appeardef.AppearPos_DragonSoul, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_DragonSoul,
		AppearId: req.AppearId,
	}, true)

	s.SendProto3(50, 19, &pb3.S2C_50_19{AppearId: req.AppearId})
	return nil
}

func (s *DragonSoulAppearSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().DragonSoulAppears
	for aId, aData := range dataMap {
		aConf := jsondata.GetDragonSoulAppearConf(aId)
		if aConf == nil {
			continue
		}
		if len(aData.AppearChips) >= len(aConf.Chips) {
			lvConf := jsondata.GetDragonSoulAppearLvConf(aId, s.getAppearLv(aId))
			if lvConf != nil {
				engine.CheckAddAttrsToCalc(s.GetOwner(), calc, lvConf.Attrs)
				if lvConf.SkillId > 0 {
					s.GetOwner().LearnSkill(lvConf.SkillId, lvConf.SkillLv, true)
				}
			}
		}

		for pos, lv := range aData.AppearChips {
			chipLvConf := jsondata.GetDragonSoulAppearChipLvConf(aId, pos, lv)
			if chipLvConf == nil {
				continue
			}
			engine.CheckAddAttrsToCalc(s.GetOwner(), calc, chipLvConf.Attrs)
		}
	}
}

func (s *DragonSoulAppearSys) getAppearLv(appearId uint32) uint32 {
	data := s.getData(appearId)
	minLv := uint32(math.MaxUint32)
	for _, Lv := range data.AppearChips {
		if Lv < minLv {
			minLv = Lv
		}
	}
	return minLv
}

func init() {
	RegisterSysClass(sysdef.SiDragonSoulAppear, func() iface.ISystem {
		return &DragonSoulAppearSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaDragonSoulAppear, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		sys, ok := player.GetSysObj(sysdef.SiDragonSoulAppear).(*DragonSoulAppearSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.calcAttr(calc)
	})

	net.RegisterSysProto(50, 17, sysdef.SiDragonSoulAppear, (*DragonSoulAppearSys).c2sActivate)
	net.RegisterSysProto(50, 18, sysdef.SiDragonSoulAppear, (*DragonSoulAppearSys).c2sUpLv)
	net.RegisterSysProto(50, 19, sysdef.SiDragonSoulAppear, (*DragonSoulAppearSys).c2sTakeOn)
}
