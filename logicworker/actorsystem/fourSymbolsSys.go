package actorsystem

/*
	desc:四象系统
	author: twl
	time:	2023/04/27
*/

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils"
	jsoniter "github.com/json-iterator/go"
	//"runtime"
)

const fourSymbolsSlotMin = 1
const fourSymbolsSlotMid = 6
const fourSymbolsSlotMax = 12

var logArr = [4]pb3.LogId{
	pb3.LogId_LogFourSymbolsDragonEqChange,
	pb3.LogId_LogFourSymbolsTigerEqChange,
	pb3.LogId_LogFourSymbolsRosefinchEqChange,
	pb3.LogId_LogFourSymbolsTortoiseEqChange,
}

var fourSymbolDeputyMaper = map[uint32]struct{}{
	itemdef.ItemFsDragonDeputyEquip:     {},
	itemdef.ItemFsTigerDeputyEquipEquip: {},
	itemdef.ItemFsRosefinchDeputyEquip:  {},
	itemdef.ItemFsTortoiseDeputyEquip:   {},
}

var fourSymbolPrincipleMaper = map[uint32]struct{}{
	itemdef.ItemFsDragonPrincipalEquip:    {},
	itemdef.ItemFsTigerPrincipalEquip:     {},
	itemdef.ItemFsRosefinchPrincipalEquip: {},
	itemdef.ItemFsTortoisePrincipalEquip:  {},
}

var fourSymbol2BaseAttrAddRate = map[uint32]uint32{
	custom_id.FourSymbolsDragon:    uint32(attrdef.FourSymbolsDragonBaseAttrRate),
	custom_id.FourSymbolsTiger:     uint32(attrdef.FourSymbolsTigerBaseAttrRate),
	custom_id.FourSymbolsRosefinch: uint32(attrdef.FourSymbolsRosefinchBaseAttrRate),
	custom_id.FourSymbolsTortoise:  uint32(attrdef.FourSymbolsTortoiseBaseAttrRate),
}

const fsOptionTaskOn = 1      // 四象操作- 装载
const fsOptionTaskReplace = 2 //四象操作 - 替换
const fsOptionTaskOff = 3     //四象操作 - 卸载

// FourSymbolsSys 四象系统
type FourSymbolsSys struct {
	Base
	principalSuit map[uint32]*suitbase.EquipNumSuit
	deputySuit    map[uint32]*suitbase.EquipNumSuit
	data          map[uint32]*pb3.FourSymbolsInfo
}

func (sys *FourSymbolsSys) OnInit() {
	sys.init()
}

func (sys *FourSymbolsSys) OnLogin() {}

func (sys *FourSymbolsSys) OnReconnect() {}

func (sys *FourSymbolsSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.FourSymbols == nil {
		binaryData.FourSymbols = make(map[uint32]*pb3.FourSymbolsInfo, custom_id.FourSymbolsTortoise)
		for i := custom_id.FourSymbolsDragon; i < custom_id.FourSymbolsTypeMax; i++ {
			binaryData.FourSymbols[i] = &pb3.FourSymbolsInfo{
				Type:                i,
				FourSymbolsEquipMap: make(map[uint32]uint32, fourSymbolsSlotMax),
			}
		}
	}

	sys.data = binaryData.FourSymbols

	sys.principalSuit = make(map[uint32]*suitbase.EquipNumSuit, custom_id.FourSymbolsTortoise)
	sys.deputySuit = make(map[uint32]*suitbase.EquipNumSuit, custom_id.FourSymbolsTortoise)

	for i := custom_id.FourSymbolsDragon; i < custom_id.FourSymbolsTypeMax; i++ {
		fsType := i
		principalEqMap := make(map[uint32]uint32, fourSymbolsSlotMid)
		deputyEqMap := make(map[uint32]uint32, fourSymbolsSlotMid)
		fsEquipMap := sys.data[fsType]
		if nil != fsEquipMap {
			for u, info := range fsEquipMap.FourSymbolsEquipMap { // 转换
				if u <= fourSymbolsSlotMid {
					principalEqMap[u] = info
				} else {
					deputyEqMap[u-fourSymbolsSlotMid] = info
				}
			}
		}
		principalOne := &suitbase.EquipNumSuit{ // 神装实例
			Equips:    principalEqMap,
			AttrSysId: attrdef.SaFourSymbols,
			SuitNum:   fourSymbolsSlotMid,
			EquipTakeOnCb: func(itemConf *jsondata.ItemConf) {
				sys.onPrincipleEquipTakeOn(fsType, itemConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			EquipTakeOffCb: func(itemConf *jsondata.ItemConf) {
				sys.onPrincipleEquipTakeOff(fsType, itemConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			EquipReplaceCb: func(newEquipConf, oldEquipConf *jsondata.ItemConf) {
				sys.onPrincipleEquipReplace(fsType, newEquipConf, oldEquipConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			TakeOnCheckHandler: func(itemConf *jsondata.ItemConf) error {
				return sys.principalEquipTakeOnCheck(fsType, itemConf)
			},

			SuitActiveCb: func(lv uint32) {
				sys.onPrincipleSuitActive(fsType, lv)
			},
			SuitDisActiveCb: func(lv uint32) {
				sys.onPrincipleSuitDisActive(fsType, lv)
			},
			SuitLvChangeCb: func(oldLv uint32, newLv uint32) {
				sys.onPrincipleSuitLvChange(fsType, oldLv, newLv)
			},
		}
		if err := principalOne.Init(); err != nil {
			sys.owner.LogError("FourSymbolsSys OnOpen principalSuit. type: %v Init err: %v", i, err)
			return false
		}
		sys.principalSuit[fsType] = principalOne

		deputyOne := &suitbase.EquipNumSuit{
			Equips:    deputyEqMap,
			AttrSysId: attrdef.SaFourSymbols,
			SuitNum:   fourSymbolsSlotMid,
			EquipTakeOnCb: func(itemConf *jsondata.ItemConf) {
				sys.onDeputyEquipTakeOn(fsType, itemConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			EquipTakeOffCb: func(itemConf *jsondata.ItemConf) {
				sys.onDeputyEquipTakeOff(fsType, itemConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			EquipReplaceCb: func(newEquipConf, oldEquipConf *jsondata.ItemConf) {
				sys.onDeputyEquipReplace(fsType, newEquipConf, oldEquipConf)
				sys.GetOwner().ChangeCastDragonEquip()
			},
			TakeOnCheckHandler: func(itemConf *jsondata.ItemConf) error {
				return sys.deputyEquipTakeOnCheck(fsType, itemConf)
			},
			SuitActiveCb: func(lv uint32) {
				sys.onDeputySuitActive(fsType, lv)
			},
			SuitDisActiveCb: func(lv uint32) {
				sys.onDeputySuitDisActive(fsType, lv)
			},
			SuitLvChangeCb: func(oldLv uint32, newLv uint32) {
				sys.onDeputySuitLvChange(fsType, oldLv, newLv)
			},
		}

		if err := deputyOne.Init(); err != nil {
			sys.owner.LogError("FourSymbolsSys OnOpen principalSuit. type: %v Init err: %v", i, err)
			return false
		}
		sys.deputySuit[i] = deputyOne

	}
	return true
}

func (sys *FourSymbolsSys) updateAllSlot(typeId uint32, fsOption uint32) {
	principalSuit := sys.principalSuit[typeId]
	deputySuit := sys.deputySuit[typeId]
	showEqMap := make(map[uint32]uint32, fourSymbolsSlotMax)
	for u, u2 := range principalSuit.Equips {
		showEqMap[u] = u2
	}
	for u, u2 := range deputySuit.Equips {
		showEqMap[u+fourSymbolsSlotMid] = u2
	}
	var rsp = &pb3.S2C_2_72{
		Type:                typeId,
		FourSymbolsEquipMap: showEqMap,
		Option:              fsOption,
	}
	sys.data[typeId].FourSymbolsEquipMap = showEqMap
	sys.SendProto3(2, 72, rsp)

	jsBytes, _ := jsoniter.Marshal(sys.data)
	sys.owner.LogDebug("update all slot %s", string(jsBytes))
}

// 等级条件校验
func (sys *FourSymbolsSys) checkCircleAndLevel(typ uint32) bool {
	actor := sys.GetOwner()
	fourSymbolConf := jsondata.GetFourSymbolsConf(typ)
	if fourSymbolConf == nil {
		return false
	}

	// 检查子系统开启是否正常
	mgr := actor.GetSysMgr().(*Mgr)
	if !mgr.canOpenSys(fourSymbolConf.SubSysId, nil) {
		return false
	}

	// 检查开启条件
	boundaryLv := actor.GetExtraAttrU32(attrdef.Circle)
	if fourSymbolConf.Condition > boundaryLv {
		//actor.SendTipMsg(tipmsgid.TpBoundary)
		return false
	}
	level := actor.GetLevel()
	if fourSymbolConf.ActorLevel > level {
		//actor.SendTipMsg(tipmsgid.TpLevelNotReach)
		return false
	}
	return true
}

// 神装回调
func (sys *FourSymbolsSys) onPrincipleEquipTakeOn(typeId uint32, _itemConf *jsondata.ItemConf) {
	sys.updateAllSlot(typeId, fsOptionTaskOn)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols1)
}
func (sys *FourSymbolsSys) onPrincipleEquipTakeOff(typeId uint32, _itemConf *jsondata.ItemConf) {
	sys.updateAllSlot(typeId, fsOptionTaskOff)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols1)
}
func (sys *FourSymbolsSys) onPrincipleEquipReplace(typeId uint32, _newEquipConf, _pldEquipConf *jsondata.ItemConf) {
	sys.updateAllSlot(typeId, fsOptionTaskReplace)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols1)
}
func (sys *FourSymbolsSys) principalEquipTakeOnCheck(typeId uint32, itemConf *jsondata.ItemConf) error {
	slot := itemConf.SubType
	checker := manager.CondChecker{}
	slotLimitConf := jsondata.GetFourSymbolsSlotSuitConf(typeId, slot)
	if nil == slotLimitConf {
		return neterror.ParamsInvalidError("principalEquipTakeOnCheck fourSymbolType %d slot not found: %v", typeId, slot)
	}

	if _, ok := fourSymbolPrincipleMaper[itemConf.Type]; !ok {
		return neterror.ParamsInvalidError("principalEquipTakeOnCheck fourSymbolType %d item is not principleequip: %v", typeId, itemConf.Id)
	}

	ok, err := checker.Check(sys.GetOwner(), slotLimitConf.Cond.Expr, slotLimitConf.Cond.Conf)
	if err != nil {
		return neterror.Wrap(err)
	}

	if !ok {
		return neterror.ParamsInvalidError("check failed: fourSymbolType %d itemId %d slot %d", typeId, itemConf.Id, slot)
	}
	return nil
}
func (sys *FourSymbolsSys) onPrincipleSuitActive(_typeId uint32, _lv uint32) {
	conf := jsondata.GetFourSymbolsEqSuitConf(_typeId)
	if conf == nil {
		sys.GetOwner().LogError("FourSymbolsSys failed, lv: %d", _lv)
		return
	}
	var suitConfByLv *jsondata.FourSymbolsEqSuitConf
	for _, suitConf := range conf {
		if suitConf.Level != _lv {
			continue
		}
		suitConfByLv = &suitConf
		break
	}
	if suitConfByLv == nil {
		return
	}
	if suitConfByLv.Skill != 0 {
		if !sys.GetOwner().LearnSkill(suitConfByLv.Skill, suitConfByLv.SkillLevel, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d, skillLevel: %d", suitConfByLv.Skill, suitConfByLv.SkillLevel)
		}
	}
}
func (sys *FourSymbolsSys) onPrincipleSuitDisActive(typeId uint32, lv uint32) {
	conf := jsondata.GetFourSymbolsEqSuitConf(typeId)
	if conf == nil {
		sys.GetOwner().LogError("FourSymbolsSys failed, lv: %d", lv)
		return
	}
	var suitConfByLv *jsondata.FourSymbolsEqSuitConf
	for _, suitConf := range conf {
		if suitConf.Level != lv {
			continue
		}
		suitConfByLv = &suitConf
		break
	}
	if suitConfByLv == nil {
		return
	}
	if suitConfByLv.Skill != 0 {
		sys.GetOwner().ForgetSkill(suitConfByLv.Skill, true, true, true)
	}
}
func (sys *FourSymbolsSys) onPrincipleSuitLvChange(typeId uint32, oldLv uint32, newLv uint32) {
	sys.onPrincipleSuitDisActive(typeId, oldLv)
	sys.onPrincipleSuitActive(typeId, newLv)
}

// 副装回调
func (sys *FourSymbolsSys) onDeputyEquipTakeOn(typeId uint32, _itemConf *jsondata.ItemConf) {
	sys.owner.LogTrace("onFourSymbolDeputyEquipTakeOn typeId %v itemType %d itemSubType %d", typeId, _itemConf.Type, _itemConf.SubType)
	sys.updateAllSlot(typeId, fsOptionTaskOn)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols2)
}
func (sys *FourSymbolsSys) onDeputyEquipTakeOff(typeId uint32, _itemConf *jsondata.ItemConf) {
	sys.owner.LogTrace("onFourSymbolDeputyEquipTakeOff typeId %v itemType %d itemSubType %d", typeId, _itemConf.Type, _itemConf.SubType)
	sys.updateAllSlot(typeId, fsOptionTaskOff)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols2)
}
func (sys *FourSymbolsSys) onDeputyEquipReplace(typeId uint32, _newEquipConf, _pldEquipConf *jsondata.ItemConf) {
	sys.owner.LogTrace("onFourSymbolDeputyEquipReplace typeId %v olditemType %d olditemSubType %d newItemType %d newItemSubType %d", typeId, _pldEquipConf.Type, _pldEquipConf.SubType, _newEquipConf.Type, _newEquipConf.SubType)
	sys.updateAllSlot(typeId, fsOptionTaskReplace)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols2)
}
func (sys *FourSymbolsSys) deputyEquipTakeOnCheck(typeId uint32, itemConf *jsondata.ItemConf) error {
	slot := itemConf.SubType
	checker := manager.CondChecker{}
	slotLimitConf := jsondata.GetFourSymbolsSlotSuitConf(typeId, slot)
	if nil == slotLimitConf {
		return neterror.ParamsInvalidError(" deputyEquipTakeOnCheck slot not found: %v", slot)
	}

	if _, ok := fourSymbolDeputyMaper[itemConf.Type]; !ok {
		return neterror.ParamsInvalidError(" deputyEquipTakeOnCheck item is not deputyequip: %v", itemConf.Id)
	}

	ok, err := checker.Check(sys.GetOwner(), slotLimitConf.Cond.Expr, slotLimitConf.Cond.Conf)
	if err != nil {
		return neterror.Wrap(err)
	}

	if !ok {
		return neterror.ParamsInvalidError("check failed: itemId %d slot %d", itemConf.Id, slot)
	}
	return nil
}
func (sys *FourSymbolsSys) onDeputySuitActive(typeId uint32, lv uint32)                    {}
func (sys *FourSymbolsSys) onDeputySuitDisActive(typeId uint32, lv uint32)                 {}
func (sys *FourSymbolsSys) onDeputySuitLvChange(typeId uint32, oldLv uint32, newLv uint32) {}

// 发送界面信息
func (sys *FourSymbolsSys) c2sPackSend(_msg *base.Message) {
	fourSymbolInfoMap := sys.GetBinaryData().GetFourSymbols()

	var fourSymbolInfo []*pb3.FourSymbolsInfo
	for i := custom_id.FourSymbolsDragon; i < custom_id.FourSymbolsTypeMax; i++ {
		fsInfo, ok := fourSymbolInfoMap[i]
		if !ok {
			continue
		}
		if !sys.checkCircleAndLevel(i) {
			continue
		}
		ps := sys.principalSuit[i] // 神装
		dps := sys.deputySuit[i]
		fsInfoMap := &pb3.FourSymbolsInfo{
			Type:                i,
			Level:               fsInfo.Level,
			FourSymbolsEquipMap: make(map[uint32]uint32),
		}

		for k, m := range ps.Equips {
			fsInfoMap.FourSymbolsEquipMap[k] = m
		}

		for k, m := range dps.Equips { // 转化7-12的槽位
			k += fourSymbolsSlotMid
			fsInfoMap.FourSymbolsEquipMap[k] = m
		}
		fourSymbolInfo = append(fourSymbolInfo, fsInfoMap)
	}
	var rsp = &pb3.S2C_2_70{FourSymbolsInfo: fourSymbolInfo}
	sys.SendProto3(2, 70, rsp)

}

// 四象升级
func (sys *FourSymbolsSys) c2sFourSymbolsLvUp(msg *base.Message) error {
	var req pb3.C2S_2_71
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg FourSymbolsLvUp :%v", err)
	}
	if req.Type < custom_id.FourSymbolsDragon && req.Type >= custom_id.FourSymbolsTypeMax {
		return neterror.ParamsInvalidError("UnpackPb3Msg Unknown req :%v", req.Type)
	}
	logId := logArr[req.Type-1]
	actor := sys.GetOwner()
	fourSymbolInfoMap := sys.data
	var currLv uint32
	if _, ok := fourSymbolInfoMap[req.Type]; ok {
		currLv = fourSymbolInfoMap[req.Type].Level
	}
	fourSymbolConf := jsondata.GetFourSymbolsConf(req.Type)
	// 检查开启条件
	if !sys.checkCircleAndLevel(req.Type) {
		return nil
	}

	next := currLv + 1

	max := uint32(len(fourSymbolConf.LevelCof))
	if next >= max {
		return nil // 满级
	}

	levelConf := fourSymbolConf.LevelCof[currLv] // 找的已经是下一级了
	if nil == levelConf {                        // 到顶了  没有下一级
		actor.SendTipMsg(tipmsgid.TpFourSymbolsMissLvConf)
		return nil
	}
	consume := levelConf.Consume

	if !actor.ConsumeByConf(consume, false, common.ConsumeParams{LogId: logId}) {
		return nil
	}

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogForSymbolsLvUp, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Type),
		StrArgs: fmt.Sprintf("{\"level\": %d}", next),
	})

	// 成功晋级
	fourSymbolInfoMap[req.Type].Level = next
	sys.data = fourSymbolInfoMap
	var rsp = &pb3.S2C_2_71{
		Type:  req.Type,
		Level: next,
	}
	sys.afterLvUp2Statics(req.Type, next)
	sys.SendProto3(2, 71, rsp)
	actor.GetAttrSys().ResetSysAttr(attrdef.SaFourSymbols)
	actor.TriggerQuestEvent(custom_id.QttOneFourSymbolsLv, req.Type, int64(next))
	return nil
}

func (sys *FourSymbolsSys) afterLvUp2Statics(typ, lv uint32) {
	var fieldStr = ""
	switch typ {
	case custom_id.FourSymbolsDragon:
		fieldStr = model.FieldFourSymbolsDragon_
	case custom_id.FourSymbolsTiger:
		fieldStr = model.FieldFourSymbolsTiger_
	case custom_id.FourSymbolsRosefinch:
		fieldStr = model.FieldFourSymbolsRoseFinch_
	case custom_id.FourSymbolsTortoise:
		fieldStr = model.FieldFourSymbolsTortoise_
	}
	if fieldStr == "" {
		return
	}
	sys.GetOwner().UpdateStatics(fieldStr, lv)
}

// 四象装备 - 装载
func (sys *FourSymbolsSys) c2sFourSymbolsEqOn(msg *base.Message) error {
	var req pb3.C2S_2_72
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg c2sFourSymbolsEqOn :%v", err)
	}
	actor := sys.GetOwner()
	if req.Type < custom_id.FourSymbolsDragon || req.Type >= custom_id.FourSymbolsTypeMax {
		actor.SendTipMsg(tipmsgid.TpFourSymbolsErrEqType)
		return nil
	}
	if !sys.checkCircleAndLevel(req.Type) {
		return nil
	}

	item := sys.GetOwner().GetItemByHandle(req.Handle)
	if item == nil {
		return neterror.ParamsInvalidError("item == nil")
	}
	itemConf := jsondata.GetItemConfig(item.ItemId)

	sys.owner.LogDebug("c2sFourSymbolsEqOn req.Type %v req.Slot %v", req.Type, req.SlotId)
	if req.SlotId >= fourSymbolsSlotMin && req.SlotId <= fourSymbolsSlotMid { // 神装
		principalSuit := sys.principalSuit[req.Type]
		sys.owner.LogDebug("c2sFourSymbolsEqOn principalSuit %v", principalSuit.Equips)
		return principalSuit.TakeOn(actor, req.Handle, itemConf, logArr[req.Type-1])
	}
	if req.SlotId > fourSymbolsSlotMid && req.SlotId <= fourSymbolsSlotMax { // 副装
		deputySuit := sys.deputySuit[req.Type]
		sys.owner.LogDebug("c2sFourSymbolsEqOn deputySuit %v", deputySuit.Equips)
		return deputySuit.TakeOn(actor, req.Handle, itemConf, logArr[req.Type-1])
	}
	return neterror.ParamsInvalidError("c2sFourSymbolsEqOn unknown Slot :%v", req.SlotId)
}

// 四象装备 - 卸载
func (sys *FourSymbolsSys) c2sFourSymbolsEqOff(msg *base.Message) error {
	var req pb3.C2S_2_73
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg c2sFourSymbolsEqOff :%v", err)
	}
	actor := sys.GetOwner()

	sys.owner.LogDebug("c2sFourSymbolsEqOff req.Type %v req.Slot %v", req.Type, req.SlotId)
	if req.Type < custom_id.FourSymbolsDragon || req.Type >= custom_id.FourSymbolsTypeMax {
		actor.SendTipMsg(tipmsgid.TpFourSymbolsErrEqType)
		return nil
	}
	if !sys.checkCircleAndLevel(req.Type) {
		return nil
	}

	if req.SlotId >= fourSymbolsSlotMin && req.SlotId <= fourSymbolsSlotMid { // 神装
		principalSuit := sys.principalSuit[req.Type]
		sys.owner.LogDebug("c2sFourSymbolsEqOff principalSuit %v", principalSuit.Equips)
		return principalSuit.TakeOff(actor, req.SlotId, logArr[req.Type-1])
	}
	if req.SlotId > fourSymbolsSlotMid && req.SlotId <= fourSymbolsSlotMax { // 副装
		deputySuit := sys.deputySuit[req.Type]
		sys.owner.LogDebug("c2sFourSymbolsEqOff deputySuit %v", deputySuit.Equips)
		return deputySuit.TakeOff(actor, req.SlotId-fourSymbolsSlotMid, logArr[req.Type-1])
	}
	return neterror.ParamsInvalidError("c2sFourSymbolsEqOff unknown Slot :%v", req.SlotId)
}

func (sys *FourSymbolsSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	sys.calcSingleTypAttr(custom_id.FourSymbolsDragon, calc)
	sys.calcSingleTypAttr(custom_id.FourSymbolsTiger, calc)
	sys.calcSingleTypAttr(custom_id.FourSymbolsRosefinch, calc)
	sys.calcSingleTypAttr(custom_id.FourSymbolsTortoise, calc)
}

func (sys *FourSymbolsSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys.calcSingleTypAttrAddRate(custom_id.FourSymbolsDragon, totalSysCalc, calc)
	sys.calcSingleTypAttrAddRate(custom_id.FourSymbolsTiger, totalSysCalc, calc)
	sys.calcSingleTypAttrAddRate(custom_id.FourSymbolsRosefinch, totalSysCalc, calc)
	sys.calcSingleTypAttrAddRate(custom_id.FourSymbolsTortoise, totalSysCalc, calc)
}

func (sys *FourSymbolsSys) calcSingleTypAttr(typ uint32, calc *attrcalc.FightAttrCalc) {
	// 等级属性
	func() {
		fsInfo := sys.data[typ]
		if fsInfo == nil {
			return
		}
		fsConf := jsondata.GetFourSymbolsConf(typ)
		if nil == fsConf || fsInfo.Level <= 0 {
			return
		}
		lvConf := fsConf.LevelCof[fsInfo.Level]
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)
	}()

	// 镶嵌的神装基础属性
	func() {
		principalSuit := sys.principalSuit[typ]
		if principalSuit == nil {
			return
		}
		for _, itemId := range principalSuit.Equips {
			itemConf := jsondata.GetItemConfig(itemId)
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, itemConf.StaticAttrs)
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, itemConf.PremiumAttrs)
		}
		suitConf := jsondata.GetFourSymbolsEqSuitConf(typ)
		if suitConf == nil {
			return
		}
		suitLv := principalSuit.GetSuitLv()
		if suitLv < 1 || suitLv > uint32(len(suitConf)) {
			return
		}
		conf := suitConf[suitLv-1]
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}()

	// 镶嵌的副装基础属性
	func() {
		deputySuit := sys.deputySuit[typ]
		if deputySuit == nil {
			return
		}
		for _, itemId := range deputySuit.Equips {
			itemConf := jsondata.GetItemConfig(itemId)
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, itemConf.StaticAttrs)
		}
		suitConf := jsondata.GetFourSymbolsEqSupSuitConf(typ)
		if suitConf == nil {
			return
		}
		suitLv := deputySuit.GetSuitLv()
		if suitLv < 1 || suitLv > uint32(len(suitConf)) {
			return
		}
		conf := suitConf[suitLv-1]
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}()
}

func (sys *FourSymbolsSys) calcSingleTypAttrAddRate(typ uint32, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	// 等级属性
	func() {
		fsInfo := sys.data[typ]
		if fsInfo == nil {
			return
		}
		fsConf := jsondata.GetFourSymbolsConf(typ)
		if nil == fsConf || fsInfo.Level <= 0 {
			return
		}
		lvConf := fsConf.LevelCof[fsInfo.Level]
		attrType := fourSymbol2BaseAttrAddRate[typ]
		addRate := uint32(totalSysCalc.GetValue(attrType)) + uint32(totalSysCalc.GetValue(attrdef.FourSymbolsBaseAttrRate))
		if addRate > 0 && lvConf != nil {
			engine.CheckAddAttrsRateRoundingUp(sys.GetOwner(), calc, lvConf.Attrs, addRate)
		}
	}()
}

func fourSymbolsProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return
	}
	fourSymbolsSys, ok := obj.(*FourSymbolsSys)
	if !ok {
		return
	}
	fourSymbolsSys.calcAttr(calc)
}

func fourSymbolsPropertyAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return
	}
	fourSymbolsSys, ok := obj.(*FourSymbolsSys)
	if !ok {
		return
	}
	fourSymbolsSys.calcAttrAddRate(totalSysCalc, calc)
}

// GetFourSymbolsLvByType 获取指定四象的等级
func GetFourSymbolsLvByType(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	needType := ids[0]
	if needType < custom_id.FourSymbolsDragon && needType >= custom_id.FourSymbolsTypeMax {
		return 0
	}
	obj := actor.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	fourSymbolsSys := obj.(*FourSymbolsSys)
	return fourSymbolsSys.data[needType].Level
}

func handleQttTakeOnATypeXTypeYStageZEquipByFourSymbols1(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 2 {
		return 0
	}
	obj := actor.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	fourSymbolsSys, ok := obj.(*FourSymbolsSys)
	if !ok {
		return 0
	}
	typ := ids[0]
	typ = GetFourSymbolsTypeByEqType(typ)
	if typ == 0 {
		return 0
	}
	nedStage := ids[1]
	return fourSymbolsSys.principalSuit[typ].GetCountMoreThanTheStage(nedStage)
}

func handleQttTakeOnATypeXTypeYStageZEquipByFourSymbols2(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 2 {
		return 0
	}
	obj := actor.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	fourSymbolsSys, ok := obj.(*FourSymbolsSys)
	if !ok {
		return 0
	}
	typ := ids[0]
	if typ == 0 {
		return 0
	}
	nedStage := ids[1]
	return fourSymbolsSys.deputySuit[typ].GetCountMoreThanTheStage(nedStage)
}

func GetFourSymbolsTypeByEqType(eqType uint32) uint32 {
	switch eqType {
	case itemdef.ItemFsDragonPrincipalEquip, itemdef.ItemFsDragonDeputyEquip:
		return custom_id.FourSymbolsDragon
	case itemdef.ItemFsTigerPrincipalEquip, itemdef.ItemFsTigerDeputyEquipEquip:
		return custom_id.FourSymbolsTiger
	case itemdef.ItemFsRosefinchPrincipalEquip, itemdef.ItemFsRosefinchDeputyEquip:
		return custom_id.FourSymbolsRosefinch
	case itemdef.ItemFsTortoisePrincipalEquip, itemdef.ItemFsTortoiseDeputyEquip:
		return custom_id.FourSymbolsTortoise
	default:
		return 0
	}
}

func gmFourSymbolsLvUp(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys); ok {
		id := utils.AtoUint32(args[0])
		lv := utils.AtoUint32(args[1])
		fourSymbolInfoMap := sys.data
		fourSymbolInfoMap[id].Level = lv
		var rsp = &pb3.S2C_2_71{Type: id, Level: lv}
		sys.SendProto3(2, 71, rsp)
		actor.GetAttrSys().ResetSysAttr(attrdef.SaFourSymbols)
		actor.TriggerQuestEvent(custom_id.QttOneFourSymbolsLv, id, int64(lv))
		return true
	}
	return false
}

func handlePowerRushRankSubTypeByFourSymbols(player iface.IPlayer, typ uint32) int64 {
	obj := player.GetSysObj(sysdef.SiFourSymbols)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*FourSymbolsSys)
	if !ok {
		return 0
	}
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()
	sys.calcSingleTypAttr(typ, singleCalc)
	return attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(player.GetJob()))
}

func init() {
	RegisterSysClass(sysdef.SiFourSymbols, func() iface.ISystem {
		f := &FourSymbolsSys{}
		return f
	})
	engine.RegAttrCalcFn(attrdef.SaFourSymbols, fourSymbolsProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaFourSymbols, fourSymbolsPropertyAddRate)
	net.RegisterSysProto(2, 70, sysdef.SiFourSymbols, (*FourSymbolsSys).c2sPackSend)
	net.RegisterSysProto(2, 71, sysdef.SiFourSymbols, (*FourSymbolsSys).c2sFourSymbolsLvUp)
	net.RegisterSysProto(2, 72, sysdef.SiFourSymbols, (*FourSymbolsSys).c2sFourSymbolsEqOn)
	net.RegisterSysProto(2, 73, sysdef.SiFourSymbols, (*FourSymbolsSys).c2sFourSymbolsEqOff)
	engine.RegQuestTargetProgress(custom_id.QttOneFourSymbolsLv, GetFourSymbolsLvByType)
	gmevent.Register("FourSymbolsLv", gmFourSymbolsLvUp, 1)
	//gmevent.Register("compose", gmCompose, 1)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFourSymbolsDragon, func(player iface.IPlayer) (score int64) {
		return handlePowerRushRankSubTypeByFourSymbols(player, custom_id.FourSymbolsDragon)
	})
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFourSymbolsTiger, func(player iface.IPlayer) (score int64) {
		return handlePowerRushRankSubTypeByFourSymbols(player, custom_id.FourSymbolsTiger)
	})
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFourSymbolsRoseFinch, func(player iface.IPlayer) (score int64) {
		return handlePowerRushRankSubTypeByFourSymbols(player, custom_id.FourSymbolsRosefinch)
	})
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFourSymbolsTortoise, func(player iface.IPlayer) (score int64) {
		return handlePowerRushRankSubTypeByFourSymbols(player, custom_id.FourSymbolsTortoise)
	})

	engine.RegQuestTargetProgress(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols1, handleQttTakeOnATypeXTypeYStageZEquipByFourSymbols1)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnATypeXTypeYStageZEquipByFourSymbols2, handleQttTakeOnATypeXTypeYStageZEquipByFourSymbols2)
}
