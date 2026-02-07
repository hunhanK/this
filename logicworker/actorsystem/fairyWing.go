package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/suitbase"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"math"
)

/**
 * @Author: YangQibin
 * @Desc: 仙翼
 * @Date: 2023/3/13
 */

type FairyWingSys struct {
	Base
	data *pb3.FairyWingData

	principalSuit suitbase.EquipNumSuit
	deputySuit    suitbase.EquipNumSuit

	expUpLv uplevelbase.ExpUpLv
}

func (sys *FairyWingSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *FairyWingSys) init() bool {
	binaryData := sys.GetBinaryData()
	if nil == binaryData.FairyWingData {
		binaryData.FairyWingData = &pb3.FairyWingData{
			ExpLv: &pb3.ExpLvSt{},
		}
	}

	sys.data = binaryData.FairyWingData

	if nil == sys.data.ExpLv {
		sys.data.ExpLv = &pb3.ExpLvSt{}
	}

	if nil == sys.data.PrincipaleEquips {
		sys.data.PrincipaleEquips = make(map[uint32]uint32)
	}

	if nil == sys.data.DeputyEquips {
		sys.data.DeputyEquips = make(map[uint32]uint32)
	}

	if sys.data.Medicine == nil {
		sys.data.Medicine = make(map[uint32]*pb3.UseCounter)
	}

	principaleCommonConf := jsondata.GetCommonConf("fairyWingMaequipment")
	sys.principalSuit = suitbase.EquipNumSuit{
		Equips:             sys.data.PrincipaleEquips,
		AttrSysId:          attrdef.SaFairyWing,
		SuitNum:            principaleCommonConf.U32,
		EquipTakeOnCb:      sys.onPrincipleEquipTakeOn,
		EquipTakeOffCb:     sys.onPrincipleEquipTakeOff,
		EquipReplaceCb:     sys.onPrincipleEquipReplace,
		TakeOnCheckHandler: sys.checkEquipTakeOn,

		SuitActiveCb:    sys.onPrincipleSuitActive,
		SuitDisActiveCb: sys.onPrincipleSuitDisActive,
		SuitLvChangeCb:  sys.onPrincipleSuitLvChange,
	}

	if err := sys.principalSuit.Init(); err != nil {
		sys.GetOwner().LogError("RiderSys OnOpen principalSuit.Init err: %v", err)
		return false
	}

	deputyCommonConf := jsondata.GetCommonConf("fairyWingMiequipment")
	sys.deputySuit = suitbase.EquipNumSuit{
		Equips:             sys.data.DeputyEquips,
		AttrSysId:          attrdef.SaFairyWing,
		SuitNum:            deputyCommonConf.U32,
		EquipTakeOnCb:      sys.onDeputyEquipTakeOn,
		EquipTakeOffCb:     sys.onDeputyEquipTakeOff,
		EquipReplaceCb:     sys.onDeputyEquipReplace,
		TakeOnCheckHandler: sys.checkEquipTakeOn,

		SuitActiveCb:    sys.onDeputySuitActive,
		SuitDisActiveCb: sys.onDeputySuitDisActive,
		SuitLvChangeCb:  sys.onDeputySuitLvChange,
	}

	if err := sys.deputySuit.Init(); err != nil {
		sys.GetOwner().LogError("RiderSys OnOpen deputySuit.Init err: %v", err)
		return false
	}

	sys.expUpLv = uplevelbase.ExpUpLv{
		ExpLv:            sys.data.ExpLv,
		AttrSysId:        attrdef.SaFairyWing,
		BehavAddExpLogId: pb3.LogId_LogFairyWingAddExp,
		AfterUpLvCb:      sys.AfterUpLevel,
		AfterAddExpCb:    sys.AfterAddExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetFairyWingLvConf(lv); conf != nil {
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

func (sys *FairyWingSys) AfterUpLevel(oldLv uint32) {
	nextLv := sys.data.ExpLv.Lv
	sys.owner.SetExtraAttr(attrdef.FairyWingLv, int64(nextLv))
	sys.ResetSysAttr(attrdef.SaFairyWing)
	stageConf := jsondata.GetFairyWingStageConfByLv(nextLv)
	sys.owner.TriggerQuestEvent(custom_id.QttFairyWingUpLv, 0, int64(nextLv))
	sys.GetOwner().UpdateStatics(model.FieldFairyWingLv_, nextLv)

	if stageConf != nil {
		if stageConf.ReqLv == nextLv {
			sys.GetOwner().SendTipMsg(tipmsgid.TpFairyWingUpStage, stageConf.Stage)
		}
	}
}

func (sys *FairyWingSys) AfterAddExp() {
	sys.SendProto3(15, 25, &pb3.S2C_15_25{ExpLv: sys.data.ExpLv})
}

func (sys *FairyWingSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.expUpLv.ExpLv.Lv = 1
	sys.owner.TriggerQuestEvent(custom_id.QttFairyWingUpLv, 0, int64(sys.GetLevel()))
	sys.GetOwner().SetExtraAttr(attrdef.FairyWingLv, int64(sys.GetLevel()))

	sys.SendProto3(15, 1, &pb3.S2C_15_1{Data: sys.data})

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Wing, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_FairyWing,
		AppearId: 1,
	}, true)
	sys.ResetSysAttr(attrdef.SaFairyWing)
}

func (sys *FairyWingSys) OnLogin() {
	sys.GetOwner().SetExtraAttr(attrdef.FairyWingLv, int64(sys.GetLevel()))
	sys.SendProto3(15, 1, &pb3.S2C_15_1{Data: sys.data})
}

func (sys *FairyWingSys) OnReconnect() {
	sys.SendProto3(15, 1, &pb3.S2C_15_1{Data: sys.data})
}

func (sys *FairyWingSys) GetLevel() uint32 {
	if !sys.IsOpen() {
		return 0
	}
	return sys.expUpLv.ExpLv.Lv
}

func (sys *FairyWingSys) checkEquipTakeOn(itemConf *jsondata.ItemConf) error {
	slotLimitConf := jsondata.GetFairyEquipSlotConf(itemConf.SubType)

	if nil == slotLimitConf {
		return neterror.ParamsInvalidError("slot not found: %v", itemConf.SubType)
	}

	if sys.GetOwner().GetCircle() < slotLimitConf.ReqRan {
		return neterror.ParamsInvalidError("circle not enough: %v", slotLimitConf.ReqRan)
	}

	if sys.GetMainData().Level < slotLimitConf.ReqLv {
		return neterror.ParamsInvalidError("level not enough: %v", slotLimitConf.ReqLv)
	}

	if sys.GetMainData().Level < itemConf.Level {
		return neterror.ParamsInvalidError("level not enough for item : %v", itemConf.Level)
	}

	if sys.GetOwner().GetCircle() < itemConf.Circle {
		return neterror.ParamsInvalidError("circle not enough for item : %v", itemConf.Circle)
	}

	//涅槃转生
	if itemConf.NirvanaLevel > 0 && itemConf.NirvanaLevel > sys.GetOwner().GetNirvanaLevel() {
		return neterror.ParamsInvalidError("nirvana not enough for item : %v", itemConf.NirvanaLevel)
	}

	wingStageConf := jsondata.GetFairyWingStageConfByLv(sys.GetLevel())

	if nil == wingStageConf {
		return neterror.ParamsInvalidError("stage not found wingLv: %v", sys.GetLevel())
	}

	if wingStageConf.Stage < itemConf.CommonField {
		return neterror.ParamsInvalidError("stage not enough: %v", itemConf.CommonField)
	}

	return nil
}

func (sys *FairyWingSys) c2sTakeEquipOn(msg *base.Message) error {
	var req pb3.C2S_15_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	item := sys.GetOwner().GetItemByHandle(req.EquipHandle)
	if nil == item {
		return neterror.ParamsInvalidError("item not found: %d", req.EquipHandle)
	}

	itemConf := jsondata.GetItemConfig(item.ItemId)
	if nil == itemConf {
		return neterror.ParamsInvalidError("itemconf not found: %v", item.ItemId)
	}

	if itemConf.Type == itemdef.ItemTypeFairyWingPrincipaleEquip {
		return sys.principalSuit.TakeOn(sys.GetOwner(), req.EquipHandle, itemConf, pb3.LogId_LogFairyWingEquipTakeOn)
	}

	if itemConf.Type == itemdef.ItemTypeFairyWingDeputyEquip {
		return sys.deputySuit.TakeOn(sys.GetOwner(), req.EquipHandle, itemConf, pb3.LogId_LogFairyWingEquipTakeOn)
	}

	return nil
}

func (sys *FairyWingSys) c2sTakeEquipOff(msg *base.Message) error {
	var req pb3.C2S_15_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if req.Type == itemdef.ItemTypeFairyWingPrincipaleEquip {
		return sys.principalSuit.TakeOff(sys.GetOwner(), req.Pos, pb3.LogId_LogFairyWingEquipTakeOff)
	}

	if req.Type == itemdef.ItemTypeFairyWingDeputyEquip {
		return sys.deputySuit.TakeOff(sys.GetOwner(), req.Pos, pb3.LogId_LogFairyWingEquipTakeOff)
	}
	return neterror.ParamsInvalidError("req.Type %v incorrect", req.Type)
}

// 仙翼-仙翼培养-幻化
func (sys *FairyWingSys) c2sChangeAppear(msg *base.Message) error {
	var req pb3.C2S_15_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if req.Stage == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Wing)
		return nil
	}

	stageConf := jsondata.GetFairyWingStageConf(req.Stage)
	if nil == stageConf {
		return neterror.ParamsInvalidError("stage not found: %v", req.Stage)
	}

	curStageConf := jsondata.GetFairyWingStageConfByLv(sys.GetLevel())

	if nil == curStageConf {
		return neterror.ParamsInvalidError("stage not found: %v", sys.GetLevel())
	}

	if req.Stage > curStageConf.Stage {
		return neterror.ParamsInvalidError("stage not enough: %v", req.Stage)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Wing, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_FairyWing,
		AppearId: req.Stage,
	}, true)
	return nil
}

func (sys *FairyWingSys) onPrincipleEquipTakeOff(itemConf *jsondata.ItemConf) {
	sys.SendProto3(15, 3, &pb3.S2C_15_3{
		Type: itemConf.Type,
		Pos:  itemConf.SubType,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *FairyWingSys) onPrincipleEquipTakeOn(itemConf *jsondata.ItemConf) {
	sys.SendProto3(15, 2, &pb3.S2C_15_2{
		Id: itemConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *FairyWingSys) onPrincipleEquipReplace(newEquipConf, pldEquipConf *jsondata.ItemConf) {
	sys.SendProto3(15, 2, &pb3.S2C_15_2{
		Id: newEquipConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *FairyWingSys) onPrincipleSuitActive(lv uint32) {
	conf := jsondata.GetFairyWingPrincipalEqSuitLvConf(lv)
	if conf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d, skillLevel: %d", conf.SkillId, conf.SkillLevel)
		}
	}
}

func (sys *FairyWingSys) onPrincipleSuitDisActive(lv uint32) {
	conf := jsondata.GetFairyWingPrincipalEqSuitLvConf(lv)
	if conf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(conf.SkillId, true, true, true)
	}
}

func (sys *FairyWingSys) onPrincipleSuitLvChange(oldLv uint32, newLv uint32) {
	oldLvConf := jsondata.GetFairyWingPrincipalEqSuitLvConf(oldLv)
	if oldLvConf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", oldLv)
		return
	}

	if oldLvConf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(oldLvConf.SkillId, true, true, true)
	}

	newLvConf := jsondata.GetFairyWingPrincipalEqSuitLvConf(newLv)
	if newLvConf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", newLv)
		return
	}

	if newLvConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(newLvConf.SkillId, newLvConf.SkillLevel, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d, skillLevel: %d", newLvConf.SkillId, newLvConf.SkillLevel)
		}
	}
	manager.TriggerCalcPowerRushRankByType(sys.GetOwner(), ranktype.PowerRushRankTypeWing)
}

func (sys *FairyWingSys) onDeputyEquipTakeOff(itemConf *jsondata.ItemConf) {
	sys.SendProto3(15, 3, &pb3.S2C_15_3{
		Type: itemConf.Type,
		Pos:  itemConf.SubType,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}
func (sys *FairyWingSys) onDeputyEquipTakeOn(itemConf *jsondata.ItemConf) {
	sys.SendProto3(15, 2, &pb3.S2C_15_2{
		Id: itemConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}
func (sys *FairyWingSys) onDeputyEquipReplace(newEquipConf, pldEquipConf *jsondata.ItemConf) {
	sys.SendProto3(15, 2, &pb3.S2C_15_2{
		Id: newEquipConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *FairyWingSys) onDeputySuitActive(lv uint32) {
	conf := jsondata.GetFairyWingPrincipalEqSuitLvConf(lv)
	if conf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d, skillLevel: %d", conf.SkillId, conf.SkillLevel)
		}
	}
}

func (sys *FairyWingSys) onDeputySuitDisActive(lv uint32) {
	conf := jsondata.GetFairyWingPrincipalEqSuitLvConf(lv)
	if conf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(conf.SkillId, true, true, true)
	}
}

func (sys *FairyWingSys) onDeputySuitLvChange(oldLv uint32, newLv uint32) {
	oldLvConf := jsondata.GetFairyWingPrincipalEqSuitLvConf(oldLv)
	if oldLvConf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", oldLv)
		return
	}

	if oldLvConf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(oldLvConf.SkillId, true, true, true)
	}

	newLvConf := jsondata.GetFairyWingPrincipalEqSuitLvConf(newLv)
	if newLvConf == nil {
		sys.GetOwner().LogError("GetFairyWingPrincipalEqSuitLvConf failed, lv: %d", newLv)
		return
	}

	if newLvConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(newLvConf.SkillId, newLvConf.SkillLevel, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d, skillLevel: %d", newLvConf.SkillId, newLvConf.SkillLevel)
		}
	}
}

func (sys *FairyWingSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	// level attr
	lv := sys.GetLevel()

	owner := sys.GetOwner()
	lvConf := jsondata.GetFairyWingLvConf(lv)
	if lvConf != nil {
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)
	}
	// 丹药对基础数值的加成比例
	for id, medicine := range sys.data.Medicine {
		medicineConf := jsondata.GetFairyWingCommonMgr().Medicine[id]
		if medicineConf == nil {
			continue
		}

		// 基本属性百分比加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.RateAttrs, medicine.Count)

		// 计算丹药的固定数值加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.Attrs, medicine.Count)
	}

	// deputy equips attr
	deputyMinStage := uint32(math.MaxUint32)
	for _, itemId := range sys.data.DeputyEquips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(owner, calc, itemConf.StaticAttrs)
		if itemConf.Stage < deputyMinStage {
			deputyMinStage = itemConf.Stage
		}
	}
	deputyCommonConf := jsondata.GetCommonConf("fairyWingMiequipment")
	if deputyCommonConf != nil && len(sys.data.DeputyEquips) >= int(deputyCommonConf.U32) && deputyCommonConf.U32 != 0 {
		deputySuitConf := jsondata.GetFairyWingDeputyEqSuitLvConf(deputyMinStage)
		if nil != deputySuitConf {
			engine.CheckAddAttrsToCalc(owner, calc, deputySuitConf.Attrs)
		}
	}

	// principale equips attr
	principaleMinStage := uint32(math.MaxUint32)
	for _, itemId := range sys.data.PrincipaleEquips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(owner, calc, itemConf.StaticAttrs)
		engine.CheckAddAttrsToCalc(owner, calc, itemConf.PremiumAttrs)
		if itemConf.Stage < deputyMinStage {
			principaleMinStage = itemConf.Stage
		}
	}
	principaleCommonConf := jsondata.GetCommonConf("fairyWingMaequipment")
	if principaleCommonConf != nil && len(sys.data.PrincipaleEquips) >= int(principaleCommonConf.U32) && principaleCommonConf.U32 != 0 {
		principaleSuitConf := jsondata.GetFairyWingDeputyEqSuitLvConf(principaleMinStage)
		if nil != principaleSuitConf {
			engine.CheckAddAttrsToCalc(owner, calc, principaleSuitConf.Attrs)
		}
	}

}

func (sys *FairyWingSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	lv := sys.GetLevel()
	owner := sys.GetOwner()
	lvConf := jsondata.GetFairyWingLvConf(lv)
	medicineAddRate := uint32(totalSysCalc.GetValue(attrdef.FairyWingBaseAttrRate)) + uint32(totalSysCalc.GetValue(attrdef.HelpFightingBaseAttrRate))
	if lvConf != nil && medicineAddRate > 0 {
		engine.CheckAddAttrsRateRoundingUp(owner, calc, lvConf.Attrs, medicineAddRate)
	}
}

func (sys *FairyWingSys) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (bool, bool, int64) {
	medicineConf := jsondata.GetFairyWingCommonMgr().Medicine[conf.ItemId]
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

	if limitConf == nil {
		return false, false, 0
	}

	if medicine.Count+uint32(param.Count) > uint32(limitConf.Limit) {
		sys.GetOwner().LogError("useMedicine failed, medicine.Count >= limitConf.Limit, medicine.Count: %d, limitConf.Limit: %d", medicine.Count, limitConf.Limit)
		return false, false, 0
	}

	medicine.Count += uint32(param.Count)

	sys.ResetSysAttr(attrdef.SaFairyWing)
	sys.SendProto3(15, 6, &pb3.S2C_15_6{
		Medicines: sys.data.Medicine,
	})
	return true, true, int64(param.Count)
}

func (sys *FairyWingSys) c2sAddExp(msg *base.Message) error {
	var req pb3.C2S_15_25
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

	expToAdd := uint64(0)

	levelUpItem := jsondata.GetFairyWingCommonMgr().LevelUpItem

	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}

		if !utils.SliceContainsUint32(levelUpItem, uint32(item.ItemId)) {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count %d < count %d", item.Count, entry.Value)
		}
	}

	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(uint64(entry.Key))

		itemConf := jsondata.GetItemConfig(item.ItemId)

		expToAdd += uint64(itemConf.CommonField * entry.Value)

		if !sys.GetOwner().DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogFairyWingAddExp) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	return sys.expUpLv.AddExp(sys.GetOwner(), expToAdd)
}

func (sys *FairyWingSys) CheckFashionActive(fashionId uint32) bool {
	return true
}

func fairyWingProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.calcAttr(calc)
}

func fairyWingPropertyAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttrAddRate(totalSysCalc, calc)
}

func GetFairyWingLv(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	sys, ok := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	return sys.GetLevel()
}

func handleQttTakeOnXTypeYStageZEquipByFairyWing1(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	fairyWingSys, ok := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok {
		return 0
	}
	nedStage := ids[0]
	return fairyWingSys.principalSuit.GetCountMoreThanTheStage(nedStage)
}

func handleQttTakeOnXTypeYStageZEquipByFairyWing2(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	fairyWingSys, ok := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok {
		return 0
	}
	nedStage := ids[0]
	return fairyWingSys.deputySuit.GetCountMoreThanTheStage(nedStage)
}

func fairyWingUseMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	fairyWingSys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
	if !ok || !fairyWingSys.IsOpen() {
		return false, false, 0
	}
	return fairyWingSys.useMedicine(param, conf)
}

func handlePowerRushRankSubTypeWingLv(player iface.IPlayer) (score int64) {
	return player.GetAttrSys().GetSysPower(attrdef.SaFairyWing)
}

func init() {
	RegisterSysClass(sysdef.SiFairyWing, func() iface.ISystem {
		return &FairyWingSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairyWing, fairyWingProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaFairyWing, fairyWingPropertyAddRate)

	net.RegisterSysProto(15, 2, sysdef.SiFairyWing, (*FairyWingSys).c2sTakeEquipOn)
	net.RegisterSysProto(15, 3, sysdef.SiFairyWing, (*FairyWingSys).c2sTakeEquipOff)
	net.RegisterSysProto(15, 6, sysdef.SiFairyWing, (*FairyWingSys).c2sChangeAppear)
	net.RegisterSysProto(15, 25, sysdef.SiFairyWing, (*FairyWingSys).c2sAddExp)
	engine.RegQuestTargetProgress(custom_id.QttFairyWingFashion, GetFairyWingLv)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnXTypeYStageZEquipByFairyWing1, handleQttTakeOnXTypeYStageZEquipByFairyWing1)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnXTypeYStageZEquipByFairyWing2, handleQttTakeOnXTypeYStageZEquipByFairyWing2)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemFairyWingMedicine, fairyWingUseMedicine)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeWing, handlePowerRushRankSubTypeWingLv)

	gmevent.Register("wingup", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		exp := utils.AtoUint64(args[0])
		sys.expUpLv.AddExp(sys.GetOwner(), exp)
		return true
	}, 1)
}
