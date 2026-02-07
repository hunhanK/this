package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
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
	"jjyz/gameserver/logicworker/actorsystem/suitbase"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

/**
 * @Author: YangQibin
 * @Desc: 神兽
 * @Date: 2023/3/20
 */

type GodRiderSys struct {
	Base
	data       *pb3.GodRiderData
	riderChips map[uint32]*suitbase.ChipUpLvSuit
}

func (sys *GodRiderSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *GodRiderSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.ResetSysAttr(attrdef.SaGodRider)
	sys.SendProto3(22, 0, &pb3.S2C_22_0{Data: sys.data})
}

func (sys *GodRiderSys) OnReconnect() {
	sys.SendProto3(22, 0, &pb3.S2C_22_0{Data: sys.data})
}

func (sys *GodRiderSys) OnLogin() {
	sys.SendProto3(22, 0, &pb3.S2C_22_0{Data: sys.data})
}

func (sys *GodRiderSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.GodRiderData == nil {
		binaryData.GodRiderData = &pb3.GodRiderData{
			Riders: make(map[uint32]*pb3.GodRiderSt),
		}
	}

	sys.data = binaryData.GodRiderData
	if sys.data.Riders == nil {
		sys.data.Riders = make(map[uint32]*pb3.GodRiderSt)
	}

	sys.riderChips = make(map[uint32]*suitbase.ChipUpLvSuit)

	for _, id := range jsondata.GetGodRiderIDs() {
		riderData := sys.data.Riders[id]

		if riderData == nil {
			riderData = &pb3.GodRiderSt{
				Chips: make(map[uint32]*pb3.IdLvSt),
			}
			sys.data.Riders[id] = riderData
		}

		if riderData.Chips == nil {
			riderData.Chips = make(map[uint32]*pb3.IdLvSt)
		}

		suitIds := jsondata.GetGodRiderSuitChipIds(id)

		if suitIds == nil {
			sys.LogError("GodRiderSys init suitIds is nil, id: %v", id)
			return false
		}

		sys.riderChips[id] = &suitbase.ChipUpLvSuit{
			Chips:                  riderData.Chips,
			AttrSysId:              attrdef.SaGodRider,
			SuitNum:                uint32(len(suitIds)),
			LogId:                  pb3.LogId_LogGodRiderChipUpLv,
			GetChipIdBySlotHandler: sys.ChipIdBySlotHander(id),
			GetChipLvConfHandler:   sys.ChipLvConfHandler(id),
			AfterChipUpLvCb:        sys.AfterChipUpLvCb(id),
			AfterSuitActiveCb:      sys.AfterSuitActiveCb(id),
			AfterSuitUpLvCb:        sys.AfterSuitUpLvCb(id),
		}
	}

	for k, foo := range sys.riderChips {
		if err := foo.Init(); err != nil {
			sys.LogError("GodRiderSys init culs.Init failed, id: %v", k)
			return false
		}
	}

	return true
}

func (sys *GodRiderSys) AfterChipUpLvCb(suitId uint32) func(player iface.IPlayer, slot uint32) {
	return func(player iface.IPlayer, slot uint32) {
		chipConf := jsondata.GetGodRiderChipConf(suitId, slot)
		if chipConf == nil {
			return
		}
		sys.SendProto3(22, 1, &pb3.S2C_22_1{
			RiderId: suitId,
			Chip:    chipConf.ChipId,
			Lev:     sys.riderChips[suitId].Chips[slot].Lv,
		})
	}
}

func (sys *GodRiderSys) AfterSuitActiveCb(suitId uint32) func() {
	return func() {
		//TODO TipMsg
		//engine.BroadcastTipMsg(, "神兽套装激活")
	}
}

func (sys *GodRiderSys) AfterSuitUpLvCb(suitId uint32) func() {
	return func() {
		engine.BroadcastTipMsgById(tipmsgid.TpGodRiderSuited, sys.GetOwner().GetId(), sys.GetOwner().GetName(), suitId)
	}
}

// 根据套装id 生成根据槽位获取芯片id的函数
func (sys *GodRiderSys) ChipIdBySlotHander(suitId uint32) suitbase.ChipIdBySlotHandler {
	return func(slot uint32) uint32 {
		return jsondata.GetGodRiderChipConf(suitId, slot).ChipId
	}
}

// 根据套装id 生成根据槽位获取芯片升级配置的函数
func (sys *GodRiderSys) ChipLvConfHandler(id uint32) suitbase.ChipLvConfHandler {
	return func(slot uint32, lv uint32) *jsondata.ConsumeUpLvConf {
		return jsondata.GetGodRiderChipLvConf(id, slot, lv)
	}
}

func (sys *GodRiderSys) c2sUpChipLv(msg *base.Message) error {
	var req pb3.C2S_22_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	chipConf := jsondata.GetGodRiderChipConf(req.RiderId, req.Slot)
	if nil == chipConf {
		return neterror.ParamsInvalidError("chipConf is nil")
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if nil == chipItem {
		return neterror.ParamsInvalidError("chipItem is nil")
	}

	if chipItem.Type != itemdef.ItemTypeGodRiderChip {
		return neterror.ParamsInvalidError("chipItem type err")
	}

	_, ok := sys.data.Riders[req.RiderId]
	if !ok {
		return neterror.ParamsInvalidError("riderData is nil")
	}

	chipUpLvER, ok := sys.riderChips[req.RiderId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if err := chipUpLvER.ChipUpLv(sys.GetOwner(), req.Slot, true); err != nil {
		return neterror.ParamsInvalidError("chipUpLvER.ChipUpLv err: %v", err)
	}
	return nil
}

func (sys *GodRiderSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_22_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	if req.RiderId == 0 {
		return nil
	}

	rider := sys.data.Riders[req.RiderId]
	if nil == rider {
		return neterror.ParamsInvalidError("rider is nil")
	}

	chipUpLvER, ok := sys.riderChips[req.RiderId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if !chipUpLvER.SuitActivated() {
		return neterror.ParamsInvalidError("rider suit not activated")
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Rider, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_GodRider,
		AppearId: req.RiderId,
	}, true)

	return nil
}

func (sys *GodRiderSys) c2sUpSkillLevel(msg *base.Message) error {
	var req pb3.C2S_22_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	rider := sys.data.Riders[req.RiderId]
	if nil == rider {
		return neterror.ParamsInvalidError("rider is nil")
	}

	chipUpLvER, ok := sys.riderChips[req.RiderId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	riderLvConf := jsondata.GetGodRiderLvConf(req.RiderId, chipUpLvER.GetSuitLv())
	if riderLvConf == nil {
		return neterror.ParamsInvalidError("riderLvConf is nil")
	}

	if riderLvConf.SkillId == 0 {
		return neterror.ParamsInvalidError("riderLvConf.SkillId != 0")
	}

	skill := sys.GetOwner().GetSkillInfo(riderLvConf.SkillId)
	if skill != nil && riderLvConf.SkillLv <= skill.Level {
		return neterror.ParamsInvalidError("skill level is max")
	}

	if !sys.GetOwner().LearnSkill(riderLvConf.SkillId, riderLvConf.SkillLv, true) {
		return neterror.ParamsInvalidError("LearnSkill err")
	}
	return nil
}

func (sys *GodRiderSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for riderId, rider := range sys.data.Riders {
		chipUpLvER, ok := sys.riderChips[riderId]
		if !ok {
			sys.LogError("get chipUplvER failed riderId %d", riderId)
			continue
		}

		if chipUpLvER.SuitActivated() {
			riderLvConf := jsondata.GetGodRiderLvConf(riderId, chipUpLvER.GetSuitLv())
			if nil != riderLvConf {
				engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, riderLvConf.Attrs)
			}
		}

		for slot, chip := range rider.Chips {
			chipLvConf := jsondata.GetGodRiderChipLvConf(riderId, slot, chip.Lv)
			if nil == chipLvConf {
				continue
			}
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, chipLvConf.Attrs)
		}
	}
}

func godRiderAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	godRiderSys, ok := player.GetSysObj(sysdef.SiGodRider).(*GodRiderSys)
	if !ok || !godRiderSys.IsOpen() {
		return
	}
	godRiderSys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiGodRider, func() iface.ISystem {
		return &GodRiderSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaGodRider, godRiderAttrCalcFn)

	net.RegisterSysProto(22, 2, sysdef.SiGodRider, (*GodRiderSys).c2sUpChipLv)
	net.RegisterSysProto(22, 3, sysdef.SiGodRider, (*GodRiderSys).c2sDress)
	net.RegisterSysProto(22, 4, sysdef.SiGodRider, (*GodRiderSys).c2sUpSkillLevel)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeGodRider, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaGodRider)
	})
}
