package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"golang.org/x/exp/slices"

	"github.com/gzjjyz/srvlib/utils"
)

/**
 * @Author: YangQibin
 * @Desc: 武器
 * @Date: 2024/3/31
 */

type WeaponSys struct {
	Base
	expUpLv uplevelbase.ExpUpLv
	data    *pb3.WeaponData
}

func (sys *WeaponSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *WeaponSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.WeaponData == nil {
		binaryData.WeaponData = &pb3.WeaponData{
			ExpLv: &pb3.ExpLvSt{},
		}
	}

	sys.data = binaryData.WeaponData

	if sys.data.ExpLv == nil {
		sys.data.ExpLv = &pb3.ExpLvSt{}
	}

	if sys.data.Medicine == nil {
		sys.data.Medicine = make(map[uint32]*pb3.UseCounter)
	}

	sys.expUpLv = uplevelbase.ExpUpLv{
		ExpLv:            sys.data.ExpLv,
		AttrSysId:        attrdef.SaWeapon,
		BehavAddExpLogId: pb3.LogId_LogWeaponAddExp,
		AfterUpLvCb:      sys.AfterUpLevel,
		AfterAddExpCb:    sys.AfterAddExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetWeaponLvConf(lv); conf != nil {
				return &conf.ExpLvConf
			}
			return nil
		},
	}

	if err := sys.expUpLv.Init(sys.GetOwner()); err != nil {
		sys.LogError("WeaponSys OnOpen expUpLv.Init err: %v", err)
		return false
	}

	return true
}

func (sys *WeaponSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.expUpLv.ExpLv.Lv = 1
	sys.ResetSysAttr(attrdef.SaWeapon)

	sys.SendProto3(24, 1, &pb3.S2C_24_1{Data: sys.data})

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Weapon, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_Weapon,
		AppearId: 1,
	}, true)
}

func (sys *WeaponSys) OnLogin() {
	sys.SendProto3(24, 1, &pb3.S2C_24_1{Data: sys.data})
}

func (sys *WeaponSys) OnReconnect() {
	sys.SendProto3(24, 1, &pb3.S2C_24_1{Data: sys.data})
}

func (sys *WeaponSys) GetLevel() uint32 {
	if !sys.IsOpen() {
		return 0
	}

	return sys.expUpLv.ExpLv.Lv
}

func (sys *WeaponSys) c2sInfo(msg *base.Message) error {
	sys.SendProto3(24, 1, &pb3.S2C_24_1{Data: sys.data})
	return nil
}

func (sys *WeaponSys) c2sAddExp(msg *base.Message) error {
	var req pb3.C2S_24_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	if req.ItemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	lvConf := sys.expUpLv.GetLvConfHandler(sys.expUpLv.ExpLv.Lv + 1)
	if lvConf == nil {
		return neterror.ParamsInvalidError("lvConf == nil")
	}

	levelUpItem := jsondata.GetWeaponCommonConf().LevelUpItem

	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}

		if !utils.SliceContainsUint32(levelUpItem, item.ItemId) {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	expToAdd := uint64(0)

	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(uint64(entry.Key))

		itemConf := jsondata.GetItemConfig(item.ItemId)

		expToAdd += uint64(itemConf.CommonField * entry.Value)

		if !sys.GetOwner().DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogWeaponUpLv) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	return sys.expUpLv.AddExp(sys.GetOwner(), expToAdd)
}

func (sys *WeaponSys) AfterUpLevel(oldLv uint32) {
	sys.owner.TriggerQuestEvent(custom_id.QttWeaponLv, 0, int64(sys.expUpLv.ExpLv.Lv))
}
func (sys *WeaponSys) AfterAddExp() {
	sys.SendProto3(24, 2, &pb3.S2C_24_2{ExpLv: sys.data.ExpLv})
}

func (sys *WeaponSys) c2sChangeAppear(msg *base.Message) error {
	var req pb3.C2S_24_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.InternalError("UnPackPb3Msg failed, err: %v", err)
	}

	if req.Lv == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Weapon)
		return nil
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Weapon, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_Weapon,
		AppearId: 1,
	}, true)
	return nil
}

func (sys *WeaponSys) c2sLearnSkill(msg *base.Message) error {
	var req pb3.C2S_24_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.InternalError("UnPackPb3Msg failed, err: %v", err)
	}

	curConf := jsondata.GetWeaponLvConf(sys.data.ExpLv.Lv)
	if nil == curConf {
		return neterror.InternalError("GetWeaponLvConf failed, lv: %d", sys.data.ExpLv.Lv)
	}

	if sys.data.ExpLv.Lv < req.Lv {
		return neterror.InternalError("sys.data.ExpLv.Lv < req.Lv")
	}

	reqLvConf := jsondata.GetWeaponLvConf(req.Lv)
	if reqLvConf == nil {
		return neterror.InternalError("GetWeaponLvConf failed, lv: %d", req.Lv)
	}

	if reqLvConf.SkillId == 0 {
		return neterror.InternalError("reqLvConf.SkillId == 0")
	}

	skillConf := jsondata.GetSkillConfig(reqLvConf.SkillId)
	if skillConf == nil {
		return neterror.InternalError("GetSkillConfig failed, skillId: %d", reqLvConf.SkillId)
	}

	skillInfo := sys.GetOwner().GetSkillInfo(reqLvConf.SkillId)
	initLv := uint32(1)
	var skillLvConf *jsondata.SkillLevelConf
	if skillInfo != nil {
		return neterror.InternalError("skillInfo != nil")
	}

	skillLvConf = skillConf.LevelConf[initLv]
	if skillLvConf == nil {
		return neterror.InternalError("GetSkillLevelConf failed, skillId: %d, lv: %d", reqLvConf.SkillId, initLv)
	}

	if !sys.GetOwner().LearnSkill(reqLvConf.SkillId, initLv, true) {
		return neterror.InternalError("LearnSkill failed, skillId: %d, lv: %d", reqLvConf.SkillId, initLv)
	}
	return nil
}

func (sys *WeaponSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	lv := sys.GetBinaryData().WeaponData.ExpLv.Lv

	lvConf := jsondata.GetWeaponLvConf(lv)
	if lvConf == nil {
		return
	}
	owner := sys.GetOwner()
	engine.CheckAddAttrsToCalc(owner, calc, lvConf.Attrs)

	// 丹药对基础数值的加成比例
	for id, medicine := range sys.data.Medicine {
		medicineConf := jsondata.GetWeaponCommonConf().Medicine[id]
		if medicineConf == nil {
			continue
		}

		// 基本属性百分比加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.RateAttrs, medicine.Count)

		// 计算丹药的固定数值加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.Attrs, medicine.Count)
	}
}

func (sys *WeaponSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	lv := sys.GetBinaryData().WeaponData.ExpLv.Lv
	lvConf := jsondata.GetWeaponLvConf(lv)
	if lvConf == nil {
		return
	}
	owner := sys.GetOwner()
	medicineAddRate := uint32(totalSysCalc.GetValue(attrdef.WeaponAttrRate)) + uint32(totalSysCalc.GetValue(attrdef.HelpFightingBaseAttrRate))
	if medicineAddRate == 0 {
		return
	}
	engine.CheckAddAttrsRateRoundingUp(owner, calc, lvConf.Attrs, medicineAddRate)
}

func (sys *WeaponSys) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	medicineConf := jsondata.GetWeaponCommonConf().Medicine[conf.ItemId]
	if medicineConf == nil {
		return false, false, 0
	}

	medicine, ok := sys.data.Medicine[conf.ItemId]
	if !ok {
		medicine = &pb3.UseCounter{
			Id: conf.ItemId,
		}
		sys.data.Medicine[conf.ItemId] = medicine
	}

	var limitConf *jsondata.MedicineUseLimit

	for _, mul := range medicineConf.UseLimit {
		if sys.GetOwner().GetLevel() <= mul.LevelLimit {
			limitConf = mul
			break
		}
	}

	if limitConf == nil && len(medicineConf.UseLimit) > 0 {
		limitConf = medicineConf.UseLimit[len(medicineConf.UseLimit)-1]
	}

	if medicine.Count+uint32(param.Count) > uint32(limitConf.Limit) {
		sys.GetOwner().LogError("useMedicine failed, medicine.Count >= limitConf.Limit, medicine.Count: %d, limitConf.Limit: %d", medicine.Count, limitConf.Limit)
		return false, false, 0
	}

	medicine.Count += uint32(param.Count)

	sys.ResetSysAttr(attrdef.SaWeapon)
	sys.SendProto3(24, 5, &pb3.S2C_24_5{
		Medicines: sys.data.Medicine,
	})
	return true, true, int64(param.Count)
}

func (sys *WeaponSys) CheckFashionActive(_ uint32) bool {
	return true
}

func weaponUseMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	weaponSys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
	if !ok || !weaponSys.IsOpen() {
		return false, false, 0
	}
	return weaponSys.useMedicine(param, conf)
}

func weaponAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}
func weaponAttrCalcFnAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttrAddRate(totalSysCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiWeapon, func() iface.ISystem {
		return &WeaponSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemWeaponMedicine, weaponUseMedicine)

	event.RegActorEvent(custom_id.AeLearnSysSkill, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
		if !ok || !sys.IsOpen() {
			return
		}

		skillId, ok := args[0].(uint32)
		if !ok {
			return
		}

		commonConf := jsondata.GetWeaponCommonConf()
		if !slices.Contains(commonConf.SkillList, skillId) {
			return
		}

		sys.ResetSysAttr(attrdef.SaWeapon)
	})

	engine.RegAttrCalcFn(attrdef.SaWeapon, weaponAttrCalcFn)
	engine.RegAttrAddRateCalcFn(attrdef.SaWeapon, weaponAttrCalcFnAddRate)
	net.RegisterSysProto(24, 1, sysdef.SiWeapon, (*WeaponSys).c2sInfo)
	net.RegisterSysProto(24, 2, sysdef.SiWeapon, (*WeaponSys).c2sAddExp)
	net.RegisterSysProto(24, 3, sysdef.SiWeapon, (*WeaponSys).c2sChangeAppear)
	net.RegisterSysProto(24, 4, sysdef.SiWeapon, (*WeaponSys).c2sLearnSkill)
	engine.RegQuestTargetProgress(custom_id.QttWeaponLv, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		weaponSys, ok := actor.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
		if !ok || !weaponSys.IsOpen() {
			return 0
		}

		return weaponSys.expUpLv.ExpLv.Lv
	})
	gmevent.Register("WeaponSys.upLevel", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiWeapon).(*WeaponSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		exp := utils.AtoUint64(args[0])
		sys.expUpLv.AddExp(sys.GetOwner(), exp)
		return true
	}, 1)
}
