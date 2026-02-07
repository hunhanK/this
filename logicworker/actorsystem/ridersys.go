package actorsystem

import (
	"encoding/json"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/suitbase"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"math"

	"github.com/gzjjyz/srvlib/utils"
)

/**
 * @Author: YangQibin
 * @Desc: 坐骑
 * @Date: 2023/3/29
 */

type RiderSys struct {
	Base
	expUpLv       uplevelbase.ExpUpLv
	principalSuit suitbase.EquipNumSuit
	deputySuit    suitbase.EquipNumSuit
	data          *pb3.RiderData
}

func (sys *RiderSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *RiderSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.RiderData == nil {
		binaryData.RiderData = &pb3.RiderData{
			ExpLv: &pb3.ExpLvSt{},
		}
	}

	sys.data = binaryData.RiderData

	if sys.data.PrincipaleEquips == nil {
		sys.data.PrincipaleEquips = make(map[uint32]uint32)
	}

	if sys.data.DeputyEquips == nil {
		sys.data.DeputyEquips = make(map[uint32]uint32)
	}

	if sys.data.ExpLv == nil {
		sys.data.ExpLv = &pb3.ExpLvSt{}
	}

	if sys.data.Medicine == nil {
		sys.data.Medicine = make(map[uint32]*pb3.UseCounter)
	}

	sys.expUpLv = uplevelbase.ExpUpLv{
		ExpLv:            sys.data.ExpLv,
		AttrSysId:        attrdef.SaRider,
		BehavAddExpLogId: pb3.LogId_LogRiderAddExp,
		AfterUpLvCb:      sys.AfterUpLevel,
		AfterAddExpCb:    sys.AfterAddExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetRiderLvConf(lv); conf != nil {
				return &conf.ExpLvConf
			}
			return nil
		},
	}

	if err := sys.expUpLv.Init(sys.GetOwner()); err != nil {
		sys.LogError("RiderSys OnOpen expUpLv.Init err: %v", err)
		return false
	}

	commonConf := jsondata.GetRiderCommonConf()
	sys.principalSuit = suitbase.EquipNumSuit{
		Equips:             sys.data.PrincipaleEquips,
		AttrSysId:          attrdef.SaRider,
		SuitNum:            commonConf.PrincipalEqSuitCount,
		EquipTakeOnCb:      sys.onPrincipleEquipTakeOn,
		EquipTakeOffCb:     sys.onPrincipleEquipTakeOff,
		EquipReplaceCb:     sys.onPrincipleEquipReplace,
		TakeOnCheckHandler: sys.principalEquipTakeOnCheck,

		SuitActiveCb:    sys.onPrincipleSuitActive,
		SuitDisActiveCb: sys.onPrincipleSuitDisActive,
		SuitLvChangeCb:  sys.onPrincipleSuitLvChange,
	}

	if err := sys.principalSuit.Init(); err != nil {
		sys.LogError("RiderSys OnOpen principalSuit.Init err: %v", err)
		return false
	}

	sys.deputySuit = suitbase.EquipNumSuit{
		Equips:             sys.data.DeputyEquips,
		AttrSysId:          attrdef.SaRider,
		SuitNum:            commonConf.PrincipalEqSuitCount,
		EquipTakeOnCb:      sys.onDeputyEquipTakeOn,
		EquipTakeOffCb:     sys.onDeputyEquipTakeOff,
		EquipReplaceCb:     sys.onDeputyEquipReplace,
		TakeOnCheckHandler: sys.deputyEquipTakeOnCheck,

		SuitActiveCb:    sys.onDeputySuitActive,
		SuitDisActiveCb: sys.onDeputySuitDisActive,
		SuitLvChangeCb:  sys.onDeputySuitLvChange,
	}

	if err := sys.deputySuit.Init(); err != nil {
		sys.LogError("RiderSys OnOpen deputySuit.Init err: %v", err)
		return false
	}
	return true
}

func (sys *RiderSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.expUpLv.ExpLv.Lv = 1
	sys.ResetSysAttr(attrdef.SaRider)

	sys.SendProto3(21, 1, &pb3.S2C_21_1{Data: sys.data})

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Rider, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_Rider,
		AppearId: 1,
	}, true)
}

func (sys *RiderSys) OnLogin() {
	sys.SendProto3(21, 1, &pb3.S2C_21_1{Data: sys.data})
}

func (sys *RiderSys) OnReconnect() {
	sys.SendProto3(21, 1, &pb3.S2C_21_1{Data: sys.data})
}

func (sys *RiderSys) c2sInfo(msg *base.Message) error {
	sys.SendProto3(21, 1, &pb3.S2C_21_1{Data: sys.data})
	return nil
}

func (sys *RiderSys) GetLevel() uint32 {
	if !sys.IsOpen() {
		return 0
	}
	return sys.expUpLv.ExpLv.Lv
}

func (sys *RiderSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_21_2

	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	item := sys.GetOwner().GetItemByHandle(req.EquipHandle)
	if item == nil {
		return neterror.ParamsInvalidError("item == nil")
	}

	itemConf := jsondata.GetItemConfig(item.ItemId)

	if itemConf.Type == itemdef.ItemRiderPrincipaleEquip {
		return sys.principalSuit.TakeOn(sys.GetOwner(), req.EquipHandle, itemConf, pb3.LogId_LogRiderEquipTakeOn)
	}

	if itemConf.Type == itemdef.ItemRiderDeputyEquip {
		return sys.deputySuit.TakeOn(sys.GetOwner(), req.EquipHandle, itemConf, pb3.LogId_LogRiderEquipTakeOn)
	}
	return nil
}

func (sys *RiderSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_21_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	if req.Type == itemdef.ItemRiderPrincipaleEquip {
		return sys.principalSuit.TakeOff(sys.GetOwner(), req.Pos, pb3.LogId_LogRiderEquipTakeOff)
	}

	if req.Type == itemdef.ItemRiderDeputyEquip {
		return sys.deputySuit.TakeOff(sys.GetOwner(), req.Pos, pb3.LogId_LogRiderEquipTakeOff)
	}
	return neterror.ParamsInvalidError("req.Type %v incorrect", req.Type)
}

func (sys *RiderSys) c2sAddExp(msg *base.Message) error {
	var req pb3.C2S_21_5
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

	levelUpItem := jsondata.GetRiderCommonConf().LevelUpItem

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

		if !sys.GetOwner().DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogRiderAddExp) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	return sys.expUpLv.AddExp(sys.GetOwner(), expToAdd)
}

func (sys *RiderSys) AfterUpLevel(oldLv uint32) {
	sys.GetOwner().UpdateStatics(model.FieldRiderLv_, sys.GetLevel())
	sys.owner.TriggerQuestEvent(custom_id.QttRiderLv, 0, int64(sys.expUpLv.ExpLv.Lv)) // 触发一下任务
	sys.owner.TriggerQuestEvent(custom_id.QttRiderLvTimes, 0, 1)                      // 触发一下任务
}

func (sys *RiderSys) AfterAddExp() {
	sys.SendProto3(21, 5, &pb3.S2C_21_5{ExpLv: sys.data.ExpLv})
}

func (sys *RiderSys) deputyEquipTakeOnCheck(itemConf *jsondata.ItemConf) error {
	slot := itemConf.SubType
	checker := manager.CondChecker{}
	slotLimitConf := jsondata.GetRiderEquipSlotConf(slot)
	if nil == slotLimitConf {
		return neterror.ParamsInvalidError("slot not found: %v", slot)
	}

	ok, err := checker.Check(sys.GetOwner(), slotLimitConf.Cond.Expr, slotLimitConf.Cond.Conf)
	if err != nil {
		return err
	}

	if !ok {
		return neterror.ParamsInvalidError("check failed: itemId %d slot %d", itemConf.Id, slot)
	}
	return nil
}

func (sys *RiderSys) principalEquipTakeOnCheck(itemConf *jsondata.ItemConf) error {
	slot := itemConf.SubType
	checker := manager.CondChecker{}
	slotLimitConf := jsondata.GetRiderEquipSlotConf(slot)
	if nil == slotLimitConf {
		return neterror.ParamsInvalidError("slot not found: %v", slot)
	}

	ok, err := checker.Check(sys.GetOwner(), slotLimitConf.Cond.Expr, slotLimitConf.Cond.Conf)
	if err != nil {
		return err
	}

	if !ok {
		return neterror.ParamsInvalidError("check failed: itemId %d slot %d", itemConf.Id, slot)
	}
	return nil
}

func (sys *RiderSys) onPrincipleEquipTakeOff(itemConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider1)
	sys.SendProto3(21, 3, &pb3.S2C_21_3{
		Type: itemConf.Type,
		Pos:  itemConf.SubType,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onPrincipleEquipTakeOn(itemConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider1)
	sys.SendProto3(21, 2, &pb3.S2C_21_2{
		Id: itemConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onPrincipleEquipReplace(newEquipConf, pldEquipConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider1)
	sys.SendProto3(21, 2, &pb3.S2C_21_2{
		Id: newEquipConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onPrincipleSuitActive(lv uint32) {
	conf := jsondata.GetRiderPrincipalEqSuitConf(lv)
	if conf == nil {
		sys.LogError("GetRiderPrincipalEqSuitConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true) {
			sys.LogError("LearnSkill failed, skillId: %d, skillLevel: %d", conf.SkillId, conf.SkillLevel)
		}
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"suitType": "principle",
	})

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: string(logArg),
	})
}

func (sys *RiderSys) onPrincipleSuitDisActive(lv uint32) {
	conf := jsondata.GetRiderPrincipalEqSuitConf(lv)
	if conf == nil {
		sys.LogError("GetRiderPrincipalEqSuitConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(conf.SkillId, true, true, true)
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"suitType": "principle",
	})

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitDisactive, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: string(logArg),
	})
}

func (sys *RiderSys) onPrincipleSuitLvChange(oldLv uint32, newLv uint32) {
	oldLvConf := jsondata.GetRiderPrincipalEqSuitConf(oldLv)
	if oldLvConf == nil {
		sys.LogError("GetRiderPrincipalEqSuitConf failed, lv: %d", oldLv)
		return
	}

	if oldLvConf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(oldLvConf.SkillId, true, true, true)
	}

	newLvConf := jsondata.GetRiderPrincipalEqSuitConf(newLv)
	if newLvConf == nil {
		sys.LogError("GetRiderPrincipalEqSuitConf failed, lv: %d", newLv)
		return
	}

	if newLvConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(newLvConf.SkillId, newLvConf.SkillLevel, true) {
			sys.LogError("LearnSkill failed, skillId: %d, skillLevel: %d", newLvConf.SkillId, newLvConf.SkillLevel)
		}
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"oldLv":    oldLv,
		"newLv":    newLv,
		"suitType": "principle",
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitLvChange, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
}

func (sys *RiderSys) onDeputyEquipTakeOff(itemConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider2)
	sys.SendProto3(21, 3, &pb3.S2C_21_3{
		Type: itemConf.Type,
		Pos:  itemConf.SubType,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onDeputyEquipTakeOn(itemConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider2)
	sys.SendProto3(21, 2, &pb3.S2C_21_2{
		Id: itemConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onDeputyEquipReplace(newEquipConf, pldEquipConf *jsondata.ItemConf) {
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByRider2)
	sys.SendProto3(21, 2, &pb3.S2C_21_2{
		Id: newEquipConf.Id,
	})
	sys.GetOwner().ChangeCastDragonEquip()
}

func (sys *RiderSys) onDeputySuitActive(lv uint32) {
	conf := jsondata.GetRiderDeputyEqSuitConf(lv)
	if conf == nil {
		sys.LogError("GetRiderDeputyEqSuitConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true) {
			sys.LogError("LearnSkill failed, skillId: %d, skillLevel: %d", conf.SkillId, conf.SkillLevel)
		}
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"suitType": "deputy",
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: string(logArg),
	})
}

func (sys *RiderSys) onDeputySuitDisActive(lv uint32) {
	conf := jsondata.GetRiderDeputyEqSuitConf(lv)
	if conf == nil {
		sys.LogError("GetRiderDeputyEqSuitConf failed, lv: %d", lv)
		return
	}

	if conf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(conf.SkillId, true, true, true)
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"suitType": "deputy",
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitDisactive, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: string(logArg),
	})
}

func (sys *RiderSys) onDeputySuitLvChange(oldLv uint32, newLv uint32) {
	oldLvConf := jsondata.GetRiderDeputyEqSuitConf(oldLv)
	if oldLvConf == nil {
		sys.LogError("GetRiderDeputyEqSuitConf failed, lv: %d", oldLv)
		return
	}

	if oldLvConf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(oldLvConf.SkillId, true, true, true)
	}

	newLvConf := jsondata.GetRiderDeputyEqSuitConf(newLv)
	if newLvConf == nil {
		sys.LogError("GetRiderDeputyEqSuitConf failed, lv: %d", newLv)
		return
	}

	if newLvConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(newLvConf.SkillId, newLvConf.SkillLevel, true) {
			sys.LogError("LearnSkill failed, skillId: %d, skillLevel: %d", newLvConf.SkillId, newLvConf.SkillLevel)
			return
		}
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"oldLv":    oldLv,
		"newLv":    newLv,
		"suitType": "deputy",
	})

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderSuitLvChange, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	manager.TriggerCalcPowerRushRankByType(sys.GetOwner(), ranktype.PowerRushRankTypeRider)
}

// 助战-飞剑培养
func (sys *RiderSys) c2sChangeAppear(msg *base.Message) error {
	var req pb3.C2S_21_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.InternalError("UnPackPb3Msg failed, err: %v", err)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Rider, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_Rider,
		AppearId: 1,
	}, true)
	return nil
}

func (sys *RiderSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	// level attr
	lv := sys.GetBinaryData().RiderData.ExpLv.Lv

	lvConf := jsondata.GetRiderLvConf(lv)
	if lvConf != nil {
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)
	}
	owner := sys.GetOwner()
	// 丹药对基础数值的加成比例
	for id, medicine := range sys.data.Medicine {
		medicineConf := jsondata.GetRiderCommonConf().Medicine[id]
		if medicineConf == nil {
			continue
		}

		// 基本属性百分比加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.RateAttrs, medicine.Count)

		// 计算丹药的固定数值加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.Attrs, medicine.Count)
	}

	// deputy equips attr
	deputyMinLv := uint32(math.MaxUint32)
	for _, itemId := range sys.data.DeputyEquips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(owner, calc, itemConf.StaticAttrs)
		if itemConf.Stage < deputyMinLv {
			deputyMinLv = itemConf.Stage
		}
	}
	if sys.deputySuit.SuitActivated() {
		deputySuitConf := jsondata.GetRiderDeputyEqSuitConf(deputyMinLv)
		if nil != deputySuitConf {
			engine.CheckAddAttrsToCalc(owner, calc, deputySuitConf.Attrs)
		}
	}

	// principale equips attr
	principaleMinLv := uint32(math.MaxUint32)
	for _, itemId := range sys.data.PrincipaleEquips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(owner, calc, itemConf.StaticAttrs)
		if itemConf.Stage < principaleMinLv {
			principaleMinLv = itemConf.Stage
		}
	}
	if sys.principalSuit.SuitActivated() {
		principaleSuitConf := jsondata.GetRiderPrincipalEqSuitConf(principaleMinLv)
		if nil != principaleSuitConf {
			engine.CheckAddAttrsToCalc(owner, calc, principaleSuitConf.Attrs)
		}
	}

}

func (sys *RiderSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	lv := sys.GetBinaryData().RiderData.ExpLv.Lv
	lvConf := jsondata.GetRiderLvConf(lv)
	medicineAddRate := uint32(totalSysCalc.GetValue(attrdef.RiderBaseAttrRate)) + uint32(totalSysCalc.GetValue(attrdef.HelpFightingBaseAttrRate))
	if lvConf != nil && medicineAddRate != 0 {
		engine.CheckAddAttrsRateRoundingUp(sys.GetOwner(), calc, lvConf.Attrs, medicineAddRate)
	}
}

func (sys *RiderSys) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (bool, bool, int64) {
	medicineConf := jsondata.GetRiderCommonConf().Medicine[conf.ItemId]
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

	sys.ResetSysAttr(attrdef.SaRider)
	sys.SendProto3(21, 6, &pb3.S2C_21_6{
		Medicines: sys.data.Medicine,
	})
	return true, true, int64(param.Count)
}

func (sys *RiderSys) CheckFashionActive(fashionId uint32) bool {
	return true
}

func RiderProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}
func RiderPropertyAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttrAddRate(totalSysCalc, calc)
}

// 坐骑达到x级
func QuestRiderLv(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	riderSys := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
	return riderSys.expUpLv.ExpLv.Lv
}

func riderUseMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	riderSys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok || !riderSys.IsOpen() {
		return false, false, 0
	}
	return riderSys.useMedicine(param, conf)
}

func handleQttTakeOnXTypeYStageZEquipByRider1(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	riderSys, ok := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok {
		return 0
	}
	nedStage := ids[0]
	return riderSys.principalSuit.GetCountMoreThanTheStage(nedStage)
}

func handleQttTakeOnXTypeYStageZEquipByRider2(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	riderSys, ok := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
	if !ok {
		return 0
	}
	nedStage := ids[0]
	return riderSys.deputySuit.GetCountMoreThanTheStage(nedStage)
}

func handlePowerRushRankSubTypeRiderLv(player iface.IPlayer) (score int64) {
	return player.GetAttrSys().GetSysPower(attrdef.SaRider)
}

func init() {
	RegisterSysClass(sysdef.SiRider, func() iface.ISystem {
		return &RiderSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaRider, RiderProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaRider, RiderPropertyAddRate)
	net.RegisterSysProto(21, 1, sysdef.SiRider, (*RiderSys).c2sInfo)
	net.RegisterSysProto(21, 2, sysdef.SiRider, (*RiderSys).c2sTakeOn)
	net.RegisterSysProto(21, 3, sysdef.SiRider, (*RiderSys).c2sTakeOff)
	net.RegisterSysProto(21, 5, sysdef.SiRider, (*RiderSys).c2sAddExp)
	net.RegisterSysProto(21, 6, sysdef.SiRider, (*RiderSys).c2sChangeAppear)
	engine.RegQuestTargetProgress(custom_id.QttRiderLv, QuestRiderLv)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnXTypeYStageZEquipByRider1, handleQttTakeOnXTypeYStageZEquipByRider1)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnXTypeYStageZEquipByRider2, handleQttTakeOnXTypeYStageZEquipByRider2)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemRiderMedicine, riderUseMedicine)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeRider, handlePowerRushRankSubTypeRiderLv)

	gmevent.Register("riderUp", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiRider).(*RiderSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		exp := utils.AtoUint64(args[0])
		sys.expUpLv.AddExp(sys.GetOwner(), exp)
		return true
	}, 1)
}
