/**
 * @Author: LvYuMeng
 * @Date: 2025/7/25
 * @Desc:
**/

package gshare

import (
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
)

type SmithRef struct {
	BagSysId                   uint32
	EquipSysId                 uint32
	SmithSysId                 uint32
	BagType                    uint32
	CalAttrSmithEqDef          uint32
	CalAttrSmithDef            uint32
	SmithLevelAttrDef          uint32
	SmithExpAttrDef            uint32
	SmithStageAttrDef          uint32
	SmithQualityAttrDef        uint32
	SmithExpMoneyType          uint32
	PrivilegeSmithAutoCastRate uint32
	SmithEquipDaTiePYYId       int // 奇匠战令-打铁
}

const (
	SmithEquipDaTiePYYId = 2830012 // 奇匠战令-打铁
)

var SmithRefArr = []*SmithRef{
	{
		BagSysId:                   sysdef.SiSmithBag,
		EquipSysId:                 sysdef.SiSmithEquip,
		SmithSysId:                 sysdef.SiSmith,
		BagType:                    bagdef.BagSmith,
		CalAttrSmithEqDef:          attrdef.SaSmithEquip,
		CalAttrSmithDef:            attrdef.SaSmith,
		SmithLevelAttrDef:          attrdef.SmithLv,
		SmithExpAttrDef:            attrdef.SmithExp,
		SmithStageAttrDef:          attrdef.SmithStage,
		SmithQualityAttrDef:        attrdef.SmithQuality,
		SmithExpMoneyType:          moneydef.SmithExp,
		PrivilegeSmithAutoCastRate: privilegedef.EnumSmithAutoCastRate,
		SmithEquipDaTiePYYId:       SmithEquipDaTiePYYId,
	},
}

type SmithRefIns struct {
}

var SmithInstance = &SmithRefIns{}

func (*SmithRefIns) EachSmithRefDo(fn func(ref *SmithRef)) {
	for _, ref := range SmithRefArr {
		fn(ref)
	}
}

// FindSmithRefByBagSysId 通过 BagSysId 查找
func (*SmithRefIns) FindSmithRefByBagSysId(sysId uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.BagSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindSmithRefByEquipSysId 通过 EquipSysId 查找
func (*SmithRefIns) FindSmithRefByEquipSysId(sysId uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.EquipSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindSmithRefBySmithSysId 通过 SmithSysId 查找
func (*SmithRefIns) FindSmithRefBySmithSysId(sysId uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.SmithSysId == sysId {
			return ref, true
		}
	}
	return nil, false
}

// FindSmithRefByBagType 通过 BagType 查找
func (*SmithRefIns) FindSmithRefByBagType(bagType uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.BagType == bagType {
			return ref, true
		}
	}
	return nil, false
}

// FindSmithRefBySaAttrDef 通过 saAttrDef 查找
func (*SmithRefIns) FindSmithRefBySaAttrDef(saAttrDef uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.CalAttrSmithEqDef == saAttrDef {
			return ref, true
		}
	}
	return nil, false
}

// FindSmithRefByExpMoneyType 通过 经验货币类型 查找
func (*SmithRefIns) FindSmithRefByExpMoneyType(mt uint32) (*SmithRef, bool) {
	for _, ref := range SmithRefArr {
		if ref.SmithExpMoneyType == mt {
			return ref, true
		}
	}
	return nil, false
}
