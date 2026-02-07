/**
 * @Author: LvYuMeng
 * @Date: 2024/10/30
 * @Desc:
**/

package gshare

import (
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
)

var BagTypeToSysMap = map[uint32]uint32{
	bagdef.BagType:                 sysdef.SiBag,
	bagdef.BagDepotType:            sysdef.SiDepot,
	bagdef.BagFairyType:            sysdef.SiFairyBag,
	bagdef.BagGodGodBeastType:      sysdef.SiGodBeastBag,
	bagdef.BagFairyEquipType:       sysdef.SiFairyEquip,
	bagdef.BagBattleSoulGodEquip:   sysdef.SiBattleSoulGodEquipBag,
	bagdef.BagMemento:              sysdef.SiMementoBag,
	bagdef.BagFairySword:           sysdef.SiFairySwordBag,
	bagdef.BagFairySpirit:          sysdef.SiFairySpiritEquBag,
	bagdef.BagHolyEquip:            sysdef.SiHolyEquipBag,
	bagdef.BagFlyingSword:          sysdef.SiFlyingSwordEquipBag,
	bagdef.BagSourceSoul:           sysdef.SiSourceSoulBag,
	bagdef.BagBlood:                sysdef.SiBloodBag,
	bagdef.BagBloodEqu:             sysdef.SiBloodEquBag,
	bagdef.BagSmith:                sysdef.SiSmithBag,
	bagdef.BagFeatherEqu:           sysdef.SiFeatherBag,
	bagdef.BagDomainSoul:           sysdef.SiDomainSoulBag,
	bagdef.BagDomainEye:            sysdef.SiDomainEyeBag,
	bagdef.BagWarPaintFairyWing:    sysdef.SiWarPaintFairyWingBag,
	bagdef.BagWarPaintGodWeapon:    sysdef.SiWarPaintGodWeaponBag,
	bagdef.BagWarPaintBattleShield: sysdef.SiWarPaintBattleShieldBag,
	bagdef.BagSoulHaloSkeleton:     sysdef.SiSoulHaloSkeletonBag,
}

func CheckSysBelongBagType(sysId uint32) bool {
	for _, v := range BagTypeToSysMap {
		if v != sysId {
			continue
		}
		return true
	}
	return false
}

type BagCheckDef struct {
	Bag   uint32
	Check func(uint32) bool
}

var SpBagRules = []BagCheckDef{
	{Bag: bagdef.BagGodGodBeastType, Check: itemdef.IsGodBeastBagItem},
	{Bag: bagdef.BagFairyType, Check: itemdef.IsFairy},
	{Bag: bagdef.BagFairyEquipType, Check: itemdef.IsFairyEquipBagItem},
	{Bag: bagdef.BagBattleSoulGodEquip, Check: itemdef.IsBattleSoulGodEquipItem},
	{Bag: bagdef.BagMemento, Check: itemdef.IsMementoItem},
	{Bag: bagdef.BagFairySword, Check: itemdef.IsFairySwordItem},
	{Bag: bagdef.BagFairySpirit, Check: itemdef.IsFairySpiritItem},
	{Bag: bagdef.BagHolyEquip, Check: itemdef.IsHolyItem},
	{Bag: bagdef.BagBlood, Check: itemdef.IsBloodItem},
	{Bag: bagdef.BagBloodEqu, Check: itemdef.IsBloodEquItem},
	{Bag: bagdef.BagSmith, Check: itemdef.IsSmithEquipItem},
	{Bag: bagdef.BagFlyingSword, Check: itemdef.IsFlyingSwordBagItem},
	{Bag: bagdef.BagSourceSoul, Check: itemdef.IsItemTypeSourceSoulEquipItem},
	{Bag: bagdef.BagFeatherEqu, Check: itemdef.IsFeatherEqu},
	{Bag: bagdef.BagDomainSoul, Check: itemdef.IsDomainSoulItem},
	{Bag: bagdef.BagDomainEye, Check: itemdef.IsDomainEyeItem},
	{Bag: bagdef.BagWarPaintFairyWing, Check: itemdef.IsWarPaintFairyWing},
	{Bag: bagdef.BagWarPaintGodWeapon, Check: itemdef.IsWarPaintGodWeapon},
	{Bag: bagdef.BagWarPaintBattleShield, Check: itemdef.IsWarPaintBattleShield},
	{Bag: bagdef.BagSoulHaloSkeleton, Check: itemdef.IsSoulHaloSkeleton},
}
