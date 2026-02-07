package actorsystem

/**
 * @Author: YangQibin
 * @Desc: 神翼
 * @Date: 2023/3/20
 */

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"math"
)

type GodWingSys struct {
	Base
	data *pb3.GodWingData
}

func (sys *GodWingSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *GodWingSys) init() bool {
	if sys.GetBinaryData().GodWingData == nil {
		sys.GetBinaryData().GodWingData = &pb3.GodWingData{
			Wings: make(map[uint32]*pb3.GodWingSt),
		}
	}

	sys.data = sys.GetBinaryData().GodWingData

	if nil == sys.data.Wings {
		sys.data.Wings = make(map[uint32]*pb3.GodWingSt)
	}

	wingIds := jsondata.GetGodWingIds()
	if nil == wingIds {
		return false
	}

	for _, wingId := range wingIds {
		if _, ok := sys.data.Wings[wingId]; !ok {
			sys.data.Wings[wingId] = &pb3.GodWingSt{}
		}

		if sys.data.Wings[wingId].Chips == nil {
			sys.data.Wings[wingId].Chips = make(map[uint32]*pb3.GodWingChipSt)
		}
	}
	return true
}

func (sys *GodWingSys) OnOpen() {
	sys.init()
	sys.ResetSysAttr(attrdef.SaGodWing)
	sys.SendProto3(18, 0, &pb3.S2C_18_0{Data: sys.data})
}

func (sys *GodWingSys) OnLogin() {
	sys.SendProto3(18, 0, &pb3.S2C_18_0{Data: sys.data})
}

func (sys *GodWingSys) OnReconnect() {
	sys.SendProto3(18, 0, &pb3.S2C_18_0{Data: sys.data})
}

func (sys *GodWingSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_18_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	chipConf := jsondata.GetGodWingChipConf(req.WingId, req.Slot)

	if chipConf == nil {
		return neterror.ParamsInvalidError("get chipConf failed WingId %d slot %d", req.WingId, req.Slot)
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if nil == chipItem {
		return neterror.ParamsInvalidError("chip item %d not found", chipConf.ChipId)
	}

	if chipItem.Type != itemdef.ItemTypeGodWingChip {
		return neterror.ParamsInvalidError("chip item Id:%d invalid not GodWingChip", chipConf.ChipId)
	}

	if chipItem.SubType != req.Slot {
		return neterror.ParamsInvalidError("chip item Id:%d invalid slot not match", chipConf.ChipId)
	}

	slot := chipItem.SubType
	godWingData, ok := sys.data.Wings[req.WingId]
	if !ok {
		return neterror.ParamsInvalidError("chip on slot %d not activated", slot)
	}

	if _, ok := godWingData.Chips[slot]; ok {
		return neterror.ParamsInvalidError("chip on slot %d already activated", req.Slot)
	}

	chipLevConf := jsondata.GetGodWingChipLvConf(req.WingId, req.Slot, 1)
	if nil == chipLevConf {
		return neterror.InternalError("get chip lev conf failed WingId %d slot %d level 1", req.WingId, req.Slot)
	}

	if !sys.GetOwner().ConsumeByConf(chipLevConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGodWingChipUpLv}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	godWingData.Chips[slot] = &pb3.GodWingChipSt{
		Id:    chipConf.ChipId,
		Level: 1,
	}
	sys.onActivateChip(req.WingId, chipConf.ChipId)
	return nil
}

func (sys *GodWingSys) onActivateChip(wingId uint32, chipId uint32) {
	sys.ResetSysAttr(attrdef.SaGodWing)
	sys.SendProto3(18, 1, &pb3.S2C_18_1{
		WingId: wingId,
		Lev:    1,
		Chip:   chipId,
	})

	wingConf := jsondata.GetGodWingConf(wingId)
	if wingConf != nil {
		if len(sys.data.Wings[wingId].Chips) == len(wingConf.SuitIds) {
			engine.BroadcastTipMsgById(tipmsgid.TpGodWingSuited, sys.GetOwner().GetId(), sys.GetOwner().GetName(), wingId)
		}
	}
}

func (sys *GodWingSys) c2sChipUpLv(msg *base.Message) error {
	var req pb3.C2S_18_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	chipConf := jsondata.GetGodWingChipConf(req.WingId, req.Slot)

	if chipConf == nil {
		return neterror.ParamsInvalidError("get chipConf failed WingId %d slot %d", req.WingId, req.Slot)
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if nil == chipItem {
		return neterror.ParamsInvalidError("chip item %d not found", chipConf.ChipId)
	}
	if chipItem.Type != itemdef.ItemTypeGodWingChip {
		return neterror.ParamsInvalidError("chip item Id:%d invalid not GodWingChip", chipConf.ChipId)
	}
	slot := chipItem.SubType
	godWingData, ok := sys.data.Wings[chipItem.CommonField]
	if !ok {
		return neterror.ParamsInvalidError("chip item Id:%d invalid slot not match", chipConf.ChipId)
	}
	chip, ok := godWingData.Chips[slot]
	if !ok {
		return neterror.ParamsInvalidError("chip on slot %d not activated", slot)
	}

	nextLev := chip.Level + 1
	chipLevConf := jsondata.GetGodWingChipLvConf(req.WingId, req.Slot, nextLev)
	if nil == chipLevConf {
		return neterror.ParamsInvalidError("chip lev conf not found WingId %d, slot %d ,level %d", req.WingId, req.Slot, nextLev)
	}
	if !sys.GetOwner().ConsumeByConf(chipLevConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGodWingChipUpLv}) {
		return neterror.ConsumeFailedError("chip lev up failed")
	}

	chip.Level = nextLev
	sys.ResetSysAttr(attrdef.SaGodWing)

	sys.SendProto3(18, 1, &pb3.S2C_18_1{
		WingId: req.WingId,
		Lev:    nextLev,
		Chip:   chipConf.ChipId,
	})
	sys.owner.TriggerQuestEvent(custom_id.QttGodWingUp, 0, int64(nextLev))

	return nil
}

func (sys *GodWingSys) getWingMinLev(wingId uint32) uint32 {
	chips := sys.data.Wings[wingId].Chips
	minLev := uint32(math.MaxUint32)
	for _, gwcs := range chips {
		if gwcs.Level < uint32(minLev) {
			minLev = gwcs.Level
		}
	}
	return minLev
}

func (sys *GodWingSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_18_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed")
	}

	if req.WingId == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Wing)
		return nil
	}

	wingConf := jsondata.GetGodWingConf(req.WingId)

	if nil == wingConf {
		return neterror.ParamsInvalidError("get wing conf failed WingId %d", req.WingId)
	}

	wing := sys.data.Wings[req.WingId]
	if nil == wing {
		return neterror.ParamsInvalidError("get chips failed WingId %d", req.WingId)
	}

	if len(wing.Chips) < len(wingConf.Chips) {
		return neterror.ParamsInvalidError("suit not activated")
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Wing, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_GodWing,
		AppearId: req.WingId,
	}, true)

	return nil
}

func (sys *GodWingSys) c2sUpSkillLevel(msg *base.Message) error {
	var req pb3.C2S_18_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed")
	}

	wing := sys.data.Wings[req.WingId]
	if nil == wing {
		return neterror.ParamsInvalidError("wing not found %d", req.WingId)
	}

	wingLv := sys.getWingMinLev(req.WingId)
	wingLvConf := jsondata.GetGodWingLvConf(req.WingId, wingLv)
	if nil == wingLvConf {
		return neterror.ParamsInvalidError("wing %d lev %d conf not found", req.WingId, wingLv)
	}

	skill := sys.GetOwner().GetSkillInfo(wingLvConf.Skill)
	if nil == skill {
		if !sys.GetOwner().LearnSkill(wingLvConf.Skill, 1, true) {
			return neterror.InternalError("learn skill %d failed", wingLvConf.Skill)
		}
		return nil
	}

	if skill.Level >= wingLvConf.SkillLevel {
		return neterror.ParamsInvalidError("skill %d already max level for now", wingLvConf.Skill)
	}

	if !sys.GetOwner().LearnSkill(wingLvConf.Skill, skill.Level+1, true) {
		return neterror.InternalError("learn skill %d failed", wingLvConf.Skill)
	}

	return nil
}

func (sys *GodWingSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for wingId, wing := range sys.data.Wings {
		wingConf := jsondata.GetGodWingConf(wingId)
		// 激活的套装数量大于等于套装配置数量，才计算套装属性
		if len(wing.Chips) >= len(wingConf.Chips) {
			wingLvConf := jsondata.GetGodWingLvConf(wingId, sys.getWingMinLev(wingId))
			if nil != wingLvConf {
				engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, wingLvConf.Attrs)
			}
		}

		for slot, chip := range wing.Chips {
			chipLvConf := jsondata.GetGodWingChipLvConf(wingId, slot, chip.Level)
			if nil == chipLvConf {
				continue
			}
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, chipLvConf.Attrs)
		}
	}

}

func (sys *GodWingSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Wings[fashionId]
	return ok
}

func godWingAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	godWingSys, ok := player.GetSysObj(sysdef.SiGodWing).(*GodWingSys)
	if !ok || !godWingSys.IsOpen() {
		return
	}
	godWingSys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiGodWing, func() iface.ISystem {
		return &GodWingSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaGodWing, godWingAttrCalcFn)

	net.RegisterSysProto(18, 1, sysdef.SiGodWing, (*GodWingSys).c2sActivate)
	net.RegisterSysProto(18, 2, sysdef.SiGodWing, (*GodWingSys).c2sChipUpLv)
	net.RegisterSysProto(18, 3, sysdef.SiGodWing, (*GodWingSys).c2sTakeOn)
	net.RegisterSysProto(18, 4, sysdef.SiGodWing, (*GodWingSys).c2sUpSkillLevel)
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeSaGodWing, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaGodWing)
	})
}
