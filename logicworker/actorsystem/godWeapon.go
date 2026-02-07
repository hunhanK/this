package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
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
	"jjyz/gameserver/logicworker/actorsystem/suitbase"
	"jjyz/gameserver/net"
)

/**
 * @Author: YangQibin
 * @Desc: 神兽
 * @Date: 2023/3/20
 */

type GodWeaponSys struct {
	Base
	data        *pb3.GodWeaponData
	weaponChips map[uint32]*suitbase.ChipUpLvSuit
}

func (sys *GodWeaponSys) GetFashionQuality(weaponId uint32) uint32 {
	conf := jsondata.GetGodWeaponConf(weaponId)
	if nil == conf {
		return 0
	}
	return conf.Quality
}

func (sys *GodWeaponSys) GetFashionBaseAttr(weaponId uint32) jsondata.AttrVec {
	_, ok := sys.data.Weapons[weaponId]
	if !ok {
		return nil
	}
	chipUpLvER, ok := sys.weaponChips[weaponId]
	if !ok {
		return nil
	}
	if chipUpLvER.SuitActivated() {
		weaponLvConf := jsondata.GetGodWeaponLvConf(weaponId, chipUpLvER.GetSuitLv())
		if nil != weaponLvConf {
			return weaponLvConf.Attrs
		}
	}
	return nil
}

func (sys *GodWeaponSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *GodWeaponSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.ResetSysAttr(attrdef.SaGodWeapon)
	sys.SendProto3(25, 0, &pb3.S2C_25_0{Data: sys.data})
}

func (sys *GodWeaponSys) OnLogin() {
	sys.SendProto3(25, 0, &pb3.S2C_25_0{Data: sys.data})
}

func (sys *GodWeaponSys) OnReconnect() {
	sys.SendProto3(25, 0, &pb3.S2C_25_0{Data: sys.data})
}

func (sys *GodWeaponSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.GodWeaponData == nil {
		binaryData.GodWeaponData = &pb3.GodWeaponData{
			Weapons: make(map[uint32]*pb3.GodWeaponSt),
		}
	}

	sys.data = binaryData.GodWeaponData
	if sys.data.Weapons == nil {
		sys.data.Weapons = make(map[uint32]*pb3.GodWeaponSt)
	}

	sys.weaponChips = make(map[uint32]*suitbase.ChipUpLvSuit)

	for _, id := range jsondata.GetGodWeaponIDs() {
		weaponData := sys.data.Weapons[id]

		if weaponData == nil {
			weaponData = &pb3.GodWeaponSt{
				Chips: make(map[uint32]*pb3.IdLvSt),
			}
			sys.data.Weapons[id] = weaponData
		}

		if weaponData.Chips == nil {
			weaponData.Chips = make(map[uint32]*pb3.IdLvSt)
		}

		suitIds := jsondata.GetGodWeaponSuitChipIds(id)

		if suitIds == nil {
			sys.LogError("GodWeaponSys init suitIds is nil, id: %v", id)
			return false
		}

		sys.weaponChips[id] = &suitbase.ChipUpLvSuit{
			Chips:                  weaponData.Chips,
			AttrSysId:              attrdef.SaGodWeapon,
			SuitNum:                uint32(len(suitIds)),
			LogId:                  pb3.LogId_LogGodWeaponChipUpLv,
			GetChipIdBySlotHandler: sys.ChipIdBySlotHander(id),
			GetChipLvConfHandler:   sys.ChipLvConfHandler(id),
			AfterChipUpLvCb:        sys.AfterChipUpLvCb(id),
			AfterSuitActiveCb:      sys.AfterSuitActiveCb(id),
			AfterSuitUpLvCb:        sys.AfterSuitUpLvCb(id),
		}
	}

	for k, foo := range sys.weaponChips {
		if err := foo.Init(); err != nil {
			sys.LogError("GodWeaponSys init culs.Init failed, id: %v", k)
			return false
		}
	}

	return true
}

func (sys *GodWeaponSys) AfterChipUpLvCb(suitId uint32) func(player iface.IPlayer, slot uint32) {
	return func(player iface.IPlayer, slot uint32) {
		chipConf := jsondata.GetGodWeaponChipConf(suitId, slot)
		if chipConf == nil {
			return
		}
		sys.SendProto3(25, 1, &pb3.S2C_25_1{
			WeaponId: suitId,
			Chip:     chipConf.ChipId,
			Lev:      sys.weaponChips[suitId].Chips[slot].Lv,
		})
	}
}

func (sys *GodWeaponSys) AfterSuitActiveCb(suitId uint32) func() {
	return func() {
		conf := jsondata.GetGodWeaponConf(suitId)
		sys.GetOwner().TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
			SetId:     conf.SetId,
			FType:     conf.FType,
			FashionId: suitId,
		})
		engine.BroadcastTipMsgById(tipmsgid.TpGodWeaponSuited, sys.GetOwner().GetId(), sys.GetOwner().GetName(), suitId)
	}
}

func (sys *GodWeaponSys) AfterSuitUpLvCb(suitId uint32) func() {
	return func() {
	}
}

// 根据套装id 生成根据槽位获取芯片id的函数
func (sys *GodWeaponSys) ChipIdBySlotHander(suitId uint32) suitbase.ChipIdBySlotHandler {
	return func(slot uint32) uint32 {
		return jsondata.GetGodWeaponChipConf(suitId, slot).ChipId
	}
}

// 根据套装id 生成根据槽位获取芯片升级配置的函数
func (sys *GodWeaponSys) ChipLvConfHandler(id uint32) suitbase.ChipLvConfHandler {
	return func(slot uint32, lv uint32) *jsondata.ConsumeUpLvConf {
		return jsondata.GetGodWeaponChipLvConf(id, slot, lv)
	}
}

func (sys *GodWeaponSys) c2sUpChipLv(msg *base.Message) error {
	var req pb3.C2S_25_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	chipConf := jsondata.GetGodWeaponChipConf(req.WeaponId, req.Slot)
	if nil == chipConf {
		return neterror.ParamsInvalidError("chipConf is nil")
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if nil == chipItem {
		return neterror.ParamsInvalidError("chipItem is nil")
	}

	if chipItem.Type != itemdef.ItemTypeGodWeaponChip {
		return neterror.ParamsInvalidError("chipItem type err")
	}

	_, ok := sys.data.Weapons[req.WeaponId]
	if !ok {
		return neterror.ParamsInvalidError("weaponData is nil")
	}

	chipUpLvER, ok := sys.weaponChips[req.WeaponId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if err := chipUpLvER.ChipUpLv(sys.GetOwner(), req.Slot, true); err != nil {
		return neterror.ParamsInvalidError("chipUpLvER.ChipUpLv err: %v", err)
	}
	return nil
}

func (sys *GodWeaponSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_25_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	if req.WeaponId == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Weapon)
		return nil
	}

	weapon := sys.data.Weapons[req.WeaponId]
	if nil == weapon {
		return neterror.ParamsInvalidError("weapon is nil")
	}

	chipUpLvER, ok := sys.weaponChips[req.WeaponId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if !chipUpLvER.SuitActivated() {
		return neterror.ParamsInvalidError("weapon suit not activated")
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Weapon, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_GodWeapon,
		AppearId: req.WeaponId,
	}, true)

	return nil
}

func (sys *GodWeaponSys) c2sUpSkillLevel(msg *base.Message) error {
	var req pb3.C2S_25_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	weapon := sys.data.Weapons[req.WeaponId]
	if nil == weapon {
		return neterror.ParamsInvalidError("weapon is nil")
	}

	chipUpLvER, ok := sys.weaponChips[req.WeaponId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	weaponLvConf := jsondata.GetGodWeaponLvConf(req.WeaponId, chipUpLvER.GetSuitLv())
	if weaponLvConf == nil {
		return neterror.ParamsInvalidError("weaponLvConf is nil")
	}

	if weaponLvConf.SkillId == 0 {
		return neterror.ParamsInvalidError("weaponLvConf.SkillId != 0")
	}

	skill := sys.GetOwner().GetSkillInfo(uint32(weaponLvConf.SkillId))
	if skill != nil && weaponLvConf.SkillLv <= skill.Level {
		return neterror.ParamsInvalidError("skill level is max")
	}

	if !sys.GetOwner().LearnSkill(uint32(weaponLvConf.SkillId), uint32(weaponLvConf.SkillLv), true) {
		return neterror.ParamsInvalidError("LearnSkill err")
	}
	return nil
}

func (sys *GodWeaponSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for weaponId, weapon := range sys.data.Weapons {
		chipUpLvER, ok := sys.weaponChips[weaponId]
		if !ok {
			sys.LogError("get chipUplvER failed weaponId %d", weaponId)
			continue
		}

		if chipUpLvER.SuitActivated() {
			weaponLvConf := jsondata.GetGodWeaponLvConf(weaponId, chipUpLvER.GetSuitLv())
			if nil != weaponLvConf {
				engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, weaponLvConf.Attrs)
			}
		}

		for slot, chip := range weapon.Chips {
			chipLvConf := jsondata.GetGodWeaponChipLvConf(weaponId, slot, chip.Lv)
			if nil == chipLvConf {
				continue
			}
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, chipLvConf.Attrs)
		}
	}
}

func (sys *GodWeaponSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Weapons[fashionId]
	return ok
}

func godWeaponAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	godWeaponSys, ok := player.GetSysObj(sysdef.SiGodWeapon).(*GodWeaponSys)
	if !ok || !godWeaponSys.IsOpen() {
		return
	}
	godWeaponSys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiGodWeapon, func() iface.ISystem {
		return &GodWeaponSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaGodWeapon, godWeaponAttrCalcFn)

	net.RegisterSysProto(25, 2, sysdef.SiGodWeapon, (*GodWeaponSys).c2sUpChipLv)
	net.RegisterSysProto(25, 3, sysdef.SiGodWeapon, (*GodWeaponSys).c2sDress)
	net.RegisterSysProto(25, 4, sysdef.SiGodWeapon, (*GodWeaponSys).c2sUpSkillLevel)
}
