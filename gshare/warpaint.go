/**
 * @Author: LvYuMeng
 * @Date: 2025/12/22
 * @Desc:
**/

package gshare

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/pb3"
)

type WarPaintRef struct {
	BagSysId                    uint32
	WarPaintSysId               uint32
	WarPaintRefineSysId         uint32
	BagType                     uint32
	CalAttrWarPaintEqDef        uint32
	CalAttrWarPaintRefineDef    uint32
	WarPaintEquipBaseAddRate    uint32
	WarPaintEquipQualityAddRate uint32
	BindSysId                   uint32
	EquipJudge                  func(itemType uint32) bool

	LogWarPaintEquipTakeOn             pb3.LogId
	LogWarPaintEquipTakeOff            pb3.LogId
	LogWarPaintEquipTakeOnRiderFashion pb3.LogId
	LogWarPaintEquipRefine             pb3.LogId
	LogWarPaintEquipRefineUpLv         pb3.LogId

	SeOptWarPaintEquip int
}

var WarPaintRefArr = []*WarPaintRef{
	{
		BagSysId:                           sysdef.SiWarPaintFairyWingBag,
		WarPaintSysId:                      sysdef.SiWarPaintFairyWing,
		WarPaintRefineSysId:                sysdef.SiWarPaintRefineFairyWing,
		BagType:                            bagdef.BagWarPaintFairyWing,
		CalAttrWarPaintEqDef:               attrdef.SaWarPaintFairyWingEquip,
		CalAttrWarPaintRefineDef:           attrdef.SaWarPaintFairyWingRefine,
		WarPaintEquipBaseAddRate:           attrdef.WarPaintEquipBaseAddRateFairyWing,
		WarPaintEquipQualityAddRate:        attrdef.WarPaintEquipQualityAddRateFairyWing,
		BindSysId:                          sysdef.SiFairyWingFashion,
		EquipJudge:                         itemdef.IsWarPaintFairyWing,
		LogWarPaintEquipTakeOn:             pb3.LogId_LogFairyWingWarPaintEquipTakeOn,
		LogWarPaintEquipTakeOff:            pb3.LogId_LogFairyWingWarPaintEquipTakeOff,
		LogWarPaintEquipTakeOnRiderFashion: pb3.LogId_LogFairyWingWarPaintEquipTakeOnRiderFashion,
		LogWarPaintEquipRefine:             pb3.LogId_LogFairyWingWarPaintEquipRefine,
		LogWarPaintEquipRefineUpLv:         pb3.LogId_LogFairyWingWarPaintEquipRefineUpLv,
		SeOptWarPaintEquip:                 custom_id.SeOptWarPaintEquipFairyWing,
	},
	{
		BagSysId:                           sysdef.SiWarPaintGodWeaponBag,
		WarPaintSysId:                      sysdef.SiWarPaintGodWeapon,
		WarPaintRefineSysId:                sysdef.SiWarPaintRefineGodWeapon,
		BagType:                            bagdef.BagWarPaintGodWeapon,
		CalAttrWarPaintEqDef:               attrdef.SaWarPaintGodWeaponEquip,
		CalAttrWarPaintRefineDef:           attrdef.SaWarPaintGodWeaponRefine,
		WarPaintEquipBaseAddRate:           attrdef.WarPaintEquipBaseAddRateGodWeapon,
		WarPaintEquipQualityAddRate:        attrdef.WarPaintEquipQualityAddRateGodWeapon,
		BindSysId:                          sysdef.SiGodWeapon,
		EquipJudge:                         itemdef.IsWarPaintGodWeapon,
		LogWarPaintEquipTakeOn:             pb3.LogId_LogGodWeaponWarPaintEquipTakeOn,
		LogWarPaintEquipTakeOff:            pb3.LogId_LogGodWeaponWarPaintEquipTakeOff,
		LogWarPaintEquipTakeOnRiderFashion: pb3.LogId_LogGodWeaponWarPaintEquipTakeOnRiderFashion,
		LogWarPaintEquipRefine:             pb3.LogId_LogGodWeaponWarPaintEquipRefine,
		LogWarPaintEquipRefineUpLv:         pb3.LogId_LogGodWeaponWarPaintEquipRefineUpLv,
		SeOptWarPaintEquip:                 custom_id.SeOptWarPaintEquipGodWeapon,
	},
	{
		BagSysId:                           sysdef.SiWarPaintBattleShieldBag,
		WarPaintSysId:                      sysdef.SiWarPaintBattleShield,
		WarPaintRefineSysId:                sysdef.SiWarPaintRefineBattleShield,
		BagType:                            bagdef.BagWarPaintBattleShield,
		CalAttrWarPaintEqDef:               attrdef.SaWarPaintBattleShieldEquip,
		CalAttrWarPaintRefineDef:           attrdef.SaWarPaintBattleShieldRefine,
		WarPaintEquipBaseAddRate:           attrdef.WarPaintEquipBaseAddRateBattleShield,
		WarPaintEquipQualityAddRate:        attrdef.WarPaintEquipQualityAddRateBattleShield,
		BindSysId:                          sysdef.SiBattleShieldTransform,
		EquipJudge:                         itemdef.IsWarPaintBattleShield,
		LogWarPaintEquipTakeOn:             pb3.LogId_LogBattleShieldWarPaintEquipTakeOn,
		LogWarPaintEquipTakeOff:            pb3.LogId_LogBattleShieldWarPaintEquipTakeOff,
		LogWarPaintEquipTakeOnRiderFashion: pb3.LogId_LogBattleShieldWarPaintEquipTakeOnRiderFashion,
		LogWarPaintEquipRefine:             pb3.LogId_LogBattleShieldWarPaintEquipRefine,
		LogWarPaintEquipRefineUpLv:         pb3.LogId_LogBattleShieldWarPaintEquipRefineUpLv,
		SeOptWarPaintEquip:                 custom_id.SeOptWarPaintEquipBattleShield,
	},
}

type WarPaintRefIns struct {
}

var WarPaintInstance = &WarPaintRefIns{}

func (*WarPaintRefIns) EachWarPaintRefDo(fn func(ref *WarPaintRef)) {
	for _, ref := range WarPaintRefArr {
		fn(ref)
	}
}

// FindWarPaintRefByBagSysId 通过 BagSysId 查找
func (*WarPaintRefIns) FindWarPaintRefByBagSysId(sysId uint32) (*WarPaintRef, bool) {
	for _, ref := range WarPaintRefArr {
		if ref.BagSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindWarPaintRefByWarPaintSysId 通过 WarPaintSysId 查找
func (*WarPaintRefIns) FindWarPaintRefByWarPaintSysId(sysId uint32) (*WarPaintRef, bool) {
	for _, ref := range WarPaintRefArr {
		if ref.WarPaintSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindWarPaintRefByWarPaintRefineSysId 通过 WarPaintRefineSysId 查找
func (*WarPaintRefIns) FindWarPaintRefByWarPaintRefineSysId(sysId uint32) (*WarPaintRef, bool) {
	for _, ref := range WarPaintRefArr {
		if ref.WarPaintRefineSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindWarPaintRefByBagType 通过 BagType 查找
func (*WarPaintRefIns) FindWarPaintRefByBagType(bagType uint32) (*WarPaintRef, bool) {
	for _, ref := range WarPaintRefArr {
		if ref.BagType == bagType {
			return ref, true
		}
	}
	return nil, false
}
