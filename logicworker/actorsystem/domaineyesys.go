/**
 * @Author: lzp
 * @Date: 2025/8/22
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"math"
)

type DomainEyeSys struct {
	Base
	expUpLvs      map[uint32]*uplevelbase.ExpUpLv
	runesExpUpLvs map[uint32]map[uint32]*uplevelbase.ExpUpLv
}

const (
	RuneType1 = 1 // 低级
	RuneType2 = 2 // 中级
	RuneType3 = 3 // 高级
)

func (s *DomainEyeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DomainEyeSys) OnLogin() {
	s.initExpUpLv()
	s.s2cInfo()
}

func (s *DomainEyeSys) OnOpen() {
	for id, conf := range jsondata.DomainEyeConfMgr {
		if conf.UnLock == 0 {
			s.unLockDomainEye(id)
		}
	}
	s.s2cInfo()
}

func (s *DomainEyeSys) GetData() map[uint32]*pb3.DomainEyeData {
	binData := s.GetBinaryData()
	if binData.DomainEyeData == nil {
		binData.DomainEyeData = map[uint32]*pb3.DomainEyeData{}
	}
	return binData.DomainEyeData
}

func (s *DomainEyeSys) TakeOnRuneAfterCompose(id, slot uint32, hdl uint64) {
	sData := s.getDomainEyeSlotData(id, slot)
	if sData == nil {
		return
	}
	runeItem := s.getDomainEyeRuneItem(hdl)
	runeItem.Ext.OwnerId = uint64(id)
	runeItem.Pos = slot

	sData.ItemSt = runeItem
	s.ResetSysAttr(attrdef.SaDomainEye)
	s.s2cDomainEye(id)
	s.s2cItemUpdate(runeItem, pb3.LogId_LogDomainEyeRunesCompose)
}

func (s *DomainEyeSys) TakeOffRuneAfterCompose(id, slot uint32) {
	sData := s.getDomainEyeSlotData(id, slot)
	if sData == nil || sData.ItemSt == nil {
		return
	}

	runeItemSt := sData.ItemSt
	runeItemSt.Ext.OwnerId = 0
	runeItemSt.Pos = 0
	s.owner.RemoveDomainEyeRuneItemByHandle(runeItemSt.Handle, pb3.LogId_LogComposeItem)

	sData.ItemSt = nil
}

func (s *DomainEyeSys) getDomainEyeData(id uint32) *pb3.DomainEyeData {
	data := s.GetData()
	dData, ok := data[id]
	if !ok {
		return nil
	}
	return dData
}

func (s *DomainEyeSys) getDomainEyeSlotData(id, slot uint32) *pb3.DomainEyeSlot {
	dData := s.getDomainEyeData(id)
	if dData == nil {
		return nil
	}

	return dData.SlotData[slot]
}

func (s *DomainEyeSys) s2cInfo() {
	s.SendProto3(82, 1, &pb3.S2C_82_1{Data: s.GetData()})
}

func (s *DomainEyeSys) s2cDomainEye(id uint32) {
	s.SendProto3(82, 6, &pb3.S2C_82_6{Data: s.getDomainEyeData(id)})
}

func (s *DomainEyeSys) s2cDomainEyeRune(id, slot uint32) {
	s.SendProto3(82, 11, &pb3.S2C_82_11{Id: id, Slot: slot, Data: s.getDomainEyeSlotData(id, slot)})
}

func (s *DomainEyeSys) s2cItemUpdate(item *pb3.ItemSt, logId pb3.LogId) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiDomainEyeBag).(*DomainEyeBagSys)
	if ok && bagSys.IsOpen() {
		bagSys.OnItemChange(item, 0, common.EngineGiveRewardParam{LogId: logId})
	}
}

func (s *DomainEyeSys) unLockDomainEye(id uint32) {
	conf := jsondata.GetDomainEyeConf(id)
	if conf == nil {
		return
	}

	data := s.GetData()
	_, ok := data[id]
	if !ok {
		data[id] = &pb3.DomainEyeData{Id: id, Stage: 1}
	}

	dData := data[id]
	if dData.ExpLv == nil {
		dData.ExpLv = &pb3.ExpLvSt{}
	}

	if dData.Attrs == nil {
		sConf := jsondata.GetDomainEyeStageConf(dData.Id, dData.Stage)
		if sConf != nil {
			for i := range sConf.Attrs {
				attr := sConf.Attrs[i]
				dData.Attrs = append(dData.Attrs, &pb3.AttrSt{Type: attr.Type})
			}
		}
	}

	if dData.SlotData == nil {
		dData.SlotData = make(map[uint32]*pb3.DomainEyeSlot)
	}

	for slot := range conf.SlotConf {
		_, ok := dData.SlotData[slot]
		if !ok {
			dData.SlotData[slot] = &pb3.DomainEyeSlot{Slot: slot}
		}
		slotData := dData.SlotData[slot]
		if slotData.ExpLv == nil {
			slotData.ExpLv = &pb3.ExpLvSt{}
			dData.SlotData[slot] = slotData
		}
	}

	s.initExpUpLvById(id)
}

func (s *DomainEyeSys) afterAddExp() {
}

func (s *DomainEyeSys) afterLvUp(_ uint32) {
}

func (s *DomainEyeSys) afterSlotAddExp() {
}

func (s *DomainEyeSys) afterSlotLvUp(_ uint32) {
}

func (s *DomainEyeSys) initExpUpLv() {
	data := s.GetData()
	s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)
	s.runesExpUpLvs = make(map[uint32]map[uint32]*uplevelbase.ExpUpLv)

	for id := range data {
		s.initExpUpLvById(id)
	}
}

func (s *DomainEyeSys) initExpUpLvById(id uint32) {
	data := s.getDomainEyeData(id)
	if data == nil {
		return
	}

	conf := jsondata.GetDomainEyeConf(id)
	if conf == nil {
		return
	}

	if s.expUpLvs == nil {
		s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)
	}

	s.expUpLvs[id] = &uplevelbase.ExpUpLv{
		ExpLv:            data.ExpLv,
		AttrSysId:        attrdef.SaDomainEye,
		BehavAddExpLogId: pb3.LogId_LogDomainEyeUpLv,
		AfterAddExpCb:    s.afterAddExp,
		AfterUpLvCb:      s.afterLvUp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetDomainEyeLvConf(id, lv); conf != nil {
				return conf.ExpLvConf
			}
			return nil
		},
	}

	if s.runesExpUpLvs == nil {
		s.runesExpUpLvs = make(map[uint32]map[uint32]*uplevelbase.ExpUpLv)
	}

	_, ok := s.runesExpUpLvs[id]
	if !ok {
		s.runesExpUpLvs[id] = make(map[uint32]*uplevelbase.ExpUpLv)
	}

	for slot := range conf.SlotConf {
		slotData, ok := data.SlotData[slot]
		if !ok {
			continue
		}
		s.runesExpUpLvs[id][slot] = &uplevelbase.ExpUpLv{
			ExpLv:            slotData.ExpLv,
			AttrSysId:        attrdef.SaDomainEye,
			BehavAddExpLogId: pb3.LogId_LogDomainEyeRunesUpLv,
			AfterAddExpCb:    s.afterSlotAddExp,
			AfterUpLvCb:      s.afterSlotLvUp,
			GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
				if conf := jsondata.GetDomainEyeSlotLvConf(id, slot, lv); conf != nil {
					return conf.ExpLvConf
				}
				return nil
			},
		}

	}
}

func (s *DomainEyeSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_82_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	id, itemMap := req.GetId(), req.GetItemMap()
	if itemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	data := s.getDomainEyeData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", id)
	}

	conf := jsondata.GetDomainEyeConf(id)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", id)
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !utils.SliceContainsUint32(conf.LevelUpItem, item.ItemId) {
			return neterror.ParamsInvalidError("item:%d not in LevelUpItem", item.ItemId)
		}
		if item.Count < int64(entry.Value) {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	addExp := uint64(0)
	for _, entry := range req.ItemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		addExp += uint64(itemConf.CommonField * entry.Value)
	}

	nextLv := data.ExpLv.Lv + 1
	lvConf := jsondata.GetDomainEyeLvConf(id, nextLv)
	if lvConf != nil && data.ExpLv.Exp+addExp >= lvConf.RequiredExp {
		if data.BreakLv < lvConf.BreakLimit {
			return neterror.ParamsInvalidError("id:%d stage limit", req.Id)
		}
	}

	for _, entry := range req.ItemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		s.owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogDomainEyeUpLv)
	}

	err := s.expUpLvs[id].AddExp(s.GetOwner(), addExp)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaDomainEye)
	s.SendProto3(82, 2, &pb3.S2C_82_2{Id: id, ExpLv: s.getDomainEyeData(id).ExpLv})
	return nil
}

func (s *DomainEyeSys) c2sUpBreak(msg *base.Message) error {
	var req pb3.C2S_82_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getDomainEyeData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", req.Id)
	}

	conf := jsondata.GetDomainEyeConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", req.Id)
	}

	nextBreakLv := data.BreakLv + 1
	bConf := jsondata.GetDomainEyeBreakConf(req.Id, nextBreakLv)
	if bConf == nil {
		return neterror.ParamsInvalidError("id: %d breakLv: %d config not found", req.Id, nextBreakLv)
	}

	if data.ExpLv.Lv < bConf.LvLimit {
		return neterror.ParamsInvalidError("id: %d lv limit", req.Id)
	}

	if !s.owner.ConsumeByConf(bConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogDomainEyeUpBreak,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data.BreakLv = nextBreakLv
	s.ResetSysAttr(attrdef.SaDomainEye)
	s.SendProto3(82, 3, &pb3.S2C_82_3{Id: req.Id, BreakLv: nextBreakLv})
	return nil
}

func (s *DomainEyeSys) c2sUpAttrs(msg *base.Message) error {
	var req pb3.C2S_82_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getDomainEyeData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", req.Id)
	}

	sConf := jsondata.GetDomainEyeStageConf(req.Id, data.Stage)
	if sConf == nil {
		return neterror.ConfNotFoundError("id: %d, stage: %d config not found", req.Id, data.Stage)
	}

	if s.checkCanUpStage(req.Id) {
		return neterror.ParamsInvalidError("cannot up attrs")
	}

	if len(sConf.AttrCounts) < 2 || len(sConf.AttrValues) < 2 {
		return neterror.ConfNotFoundError("id: %d config error", req.Id)
	}

	if !s.owner.ConsumeByConf(sConf.UpgradeConsume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogDomainEyeUpUpgrade,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}

	count := random.Interval(int(sConf.AttrCounts[0]), int(sConf.AttrCounts[1]))
	idxL := random.RandMany(0, uint32(len(data.Attrs)-1), uint32(count))

	preStageConf := jsondata.GetDomainEyeStageConf(req.Id, data.Stage-1)
	for _, idx := range idxL {
		ratio := random.IntervalUU(sConf.AttrValues[0], sConf.AttrValues[1])
		attrId := data.Attrs[idx].Type

		addValue := s.calcAddValue(preStageConf, sConf, attrId, ratio)
		data.Attrs[idx].Value += addValue
		maxValue := jsondata.GetDomainEyeStageAttrValue(data.Id, data.Stage, attrId)
		data.Attrs[idx].Value = utils.MinUInt32(data.Attrs[idx].Value, maxValue)
	}

	s.s2cDomainEye(req.Id)
	s.ResetSysAttr(attrdef.SaDomainEye)
	return nil
}

func (s *DomainEyeSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_82_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getDomainEyeData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", req.Id)
	}

	conf := jsondata.GetDomainEyeConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", req.Id)
	}

	if !s.checkCanUpStage(req.Id) {
		return neterror.ParamsInvalidError("id: %d cannot up stage", req.Id)
	}

	stage := data.Stage
	sConf := jsondata.GetDomainEyeStageConf(req.Id, stage)
	if sConf == nil {
		return neterror.ConfNotFoundError("id: %d, stage: %d config not found", req.Id, stage)
	}

	if !s.owner.ConsumeByConf(sConf.StageConsume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogDomainEyeUpStage,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data.Stage += 1
	s.ResetSysAttr(attrdef.SaDomainEye)
	s.SendProto3(82, 5, &pb3.S2C_82_5{Id: req.Id, Stage: data.Stage})
	return nil
}

func (s *DomainEyeSys) c2sUnLock(msg *base.Message) error {
	var req pb3.C2S_82_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetDomainEyeConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", req.Id)
	}
	if conf.UnLock == 0 {
		return neterror.ParamsInvalidError("id: %d unlock=0", req.Id)
	}

	data := s.getDomainEyeData(req.Id)
	if data != nil {
		return neterror.ParamsInvalidError("id: %d unlock", req.Id)
	}

	preId := req.Id - 1
	preData := s.getDomainEyeData(preId)

	if preData == nil || (preData != nil && preData.Stage < conf.UnLock) {
		return neterror.ParamsInvalidError("id: %d cannot unlock", req.Id)
	}

	s.unLockDomainEye(req.Id)
	s.SendProto3(82, 7, &pb3.S2C_82_7{Id: req.Id})
	s.s2cDomainEye(req.Id)
	return nil
}

func (s *DomainEyeSys) c2sSlotUpLv(msg *base.Message) error {
	var req pb3.C2S_82_8
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	id, slot := req.GetId(), req.GetSlot()
	itemMap, runeMap := req.GetItemMap(), req.GetRunesMap()

	if itemMap == nil && runeMap == nil {
		return neterror.ParamsInvalidError("params error")
	}

	data := s.getDomainEyeData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", id)
	}

	sConf := jsondata.GetDomainEyeSlotConf(id, slot)
	if sConf == nil {
		return neterror.ConfNotFoundError("id: %d slot: %d config not found", id, slot)
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item nil")
		}

		itemConf := jsondata.GetItemConfig(item.GetItemId())
		if itemConf == nil {
			return neterror.ParamsInvalidError("itemConf nil")
		}

		itemIdL := jsondata.GlobalU32Vec("domainEyeSlotMaterialItem")
		if !utils.SliceContainsUint32(itemIdL, itemConf.Id) {
			return neterror.ParamsInvalidError("not rune material")
		}

		if item.Count < int64(entry.Value) {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	for _, entry := range runeMap {
		item := s.getDomainEyeRuneItem(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item nil")
		}

		itemConf := jsondata.GetItemConfig(item.GetItemId())
		if itemConf == nil {
			return neterror.ParamsInvalidError("itemConf nil")
		}
		if !itemdef.IsDomainEyeItem(itemConf.Type) {
			return neterror.ParamsInvalidError("item type error")
		}

		if item.Count < int64(entry.Value) {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	addExp := uint64(0)
	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		addExp += uint64(itemConf.CommonField * entry.Value)
		if !s.owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogDomainEyeUpLv) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	for _, entry := range runeMap {
		item := s.getDomainEyeRuneItem(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		addExp += uint64(itemConf.CommonField * entry.Value)
		if !s.owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogDomainEyeUpLv) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	sData := s.getDomainEyeSlotData(id, slot)
	oldLv := sData.ExpLv.Lv

	err := s.runesExpUpLvs[id][slot].AddExp(s.GetOwner(), addExp)
	if err != nil {
		return err
	}

	newLv := sData.ExpLv.Lv
	for lv := oldLv + 1; lv <= newLv; lv++ {
		s.unLockSlotAttrs(id, slot, lv)
	}

	s.ResetSysAttr(attrdef.SaDomainEye)
	s.SendProto3(82, 8, &pb3.S2C_82_8{Id: id, Slot: slot, ExpLv: sData.ExpLv})
	s.s2cDomainEyeRune(id, slot)
	return nil
}

func (s *DomainEyeSys) c2sRunesTakeOn(msg *base.Message) error {
	var req pb3.C2S_82_9
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Id, req.Slot, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Id, req.Slot, req.Hdl)
	s.SendProto3(82, 9, &pb3.S2C_82_9{Id: req.Id, Slot: req.Slot, Hdl: req.Hdl})
	return nil
}

func (s *DomainEyeSys) c2sRunesTakeOff(msg *base.Message) error {
	var req pb3.C2S_82_10
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if err := s.checkTakeOff(req.Id, req.Slot); err != nil {
		return err
	}

	s.takeOff(req.Id, req.Slot)
	s.SendProto3(82, 10, &pb3.S2C_82_10{Id: req.Id, Slot: req.Slot})
	s.s2cDomainEye(req.Id)
	return nil
}

func (s *DomainEyeSys) checkTakeOn(id, slot uint32, hdl uint64) error {
	data := s.getDomainEyeData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", id)
	}

	runeItem := s.getDomainEyeRuneItem(hdl)
	if runeItem == nil {
		return neterror.ParamsInvalidError("not domainEye rune")
	}

	itemConf := jsondata.GetItemConfig(runeItem.ItemId)
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId: %d config not found", runeItem.ItemId)
	}
	if itemConf.SubType != slot {
		return neterror.ParamsInvalidError("itemId: %d cannot equip", runeItem.ItemId)
	}
	return nil
}

func (s *DomainEyeSys) takeOn(id, slot uint32, hdl uint64) {
	sData := s.getDomainEyeSlotData(id, slot)
	if sData.ItemSt != nil {
		s.takeOff(id, slot)
	}

	runeItem := s.getDomainEyeRuneItem(hdl)
	runeItem.Pos = slot
	runeItem.Ext.OwnerId = uint64(id)
	sData.ItemSt = runeItem

	s.s2cDomainEyeRune(id, slot)
	s.s2cItemUpdate(runeItem, pb3.LogId_LogDomainEyeRunesTakeOn)
	s.afterTakeOn()
}

func (s *DomainEyeSys) checkTakeOff(id, slot uint32) error {
	data := s.getDomainEyeData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id: %d lock", id)
	}
	sData := s.getDomainEyeSlotData(id, slot)
	if sData.ItemSt == nil {
		return neterror.ParamsInvalidError("id: %d, slot: %d not equip rune", id, slot)
	}
	return nil
}

func (s *DomainEyeSys) takeOff(id, slot uint32) {
	sData := s.getDomainEyeSlotData(id, slot)
	if sData.ItemSt == nil {
		return
	}

	runeItem := sData.ItemSt
	runeItem.Pos = 0
	runeItem.Ext.OwnerId = 0
	sData.ItemSt = nil

	s.s2cDomainEyeRune(id, slot)
	s.s2cItemUpdate(runeItem, pb3.LogId_LogDomainEyeRunesTakeOn)
	s.afterTakeOff()
}

func (s *DomainEyeSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaDomainEye)
}

func (s *DomainEyeSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaDomainEye)
}

func (s *DomainEyeSys) calcAddValue(conf1, conf2 *jsondata.DomainEyeStage, attrId, ratio uint32) uint32 {
	getValue := func(conf *jsondata.DomainEyeStage, attrId uint32) uint32 {
		for _, v := range conf.Attrs {
			if v.Type == attrId {
				return v.Value
			}
		}
		return 0
	}

	var value1, value2 uint32
	if conf1 != nil {
		value1 = getValue(conf1, attrId)
	}

	if conf2 != nil {
		value2 = getValue(conf2, attrId)
	}

	addValue := value2 - value1
	if addValue > 0 {
		addValue = utils.CalcMillionRate(addValue, ratio)
	}

	return addValue
}

func (s *DomainEyeSys) checkCanUpStage(id uint32) bool {
	data := s.getDomainEyeData(id)
	if data == nil {
		return false
	}

	sConf := jsondata.GetDomainEyeStageConf(id, data.Stage)
	if sConf == nil {
		return false
	}

	var count uint32
	for i := range data.Attrs {
		for j := range sConf.Attrs {
			if data.Attrs[i].Type == sConf.Attrs[j].Type &&
				data.Attrs[i].Value == sConf.Attrs[j].Value {
				count++
			}
		}
	}

	return count >= sConf.Cond
}

func (s *DomainEyeSys) unLockSlotAttrs(id, slot uint32, lv uint32) {
	slotData := s.getDomainEyeSlotData(id, slot)
	if slotData == nil {
		return
	}

	slotConf := jsondata.GetDomainEyeSlotConf(id, slot)
	if slotConf == nil {
		return
	}

	lvConf := jsondata.GetDomainEyeSlotLvConf(id, slot, lv)
	if lvConf == nil {
		return
	}

	if lvConf.IsUnLockAttr {
		pool := new(random.Pool)
		for id, v := range slotConf.UnLockAttrs {
			if !utils.SliceContainsUint32(slotData.AttrIdL, id) {
				pool.AddItem(v, v.Weight)
			}
		}
		if pool.Size() == 0 {
			return
		}
		attrConf := pool.RandomOne().(*jsondata.DomainEyeSlotAttr)
		slotData.AttrIdL = append(slotData.AttrIdL, attrConf.Id)
	}
}

func (s *DomainEyeSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	s.calcEyeAttr(calc)
	s.calcRuneAttr(calc)
}

func (s *DomainEyeSys) calcEyeAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()

	for id := range data {
		attrs := s.getDomainEyeAttrs(id)
		if len(attrs) > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
		}
	}

	s.calcSuitAttr(calc)
}

func (s *DomainEyeSys) calcRuneAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for id, v := range data {
		var attrs jsondata.AttrVec
		for slot := range v.SlotData {
			attrs = append(attrs, s.getDomainEyeRuneAttrs(id, slot)...)
		}
		if len(attrs) > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
		}
	}
	s.calcRuneSuitAttr(calc)
}

func (s *DomainEyeSys) calcRuneSuitAttr(calc *attrcalc.FightAttrCalc) {
	for id, conf := range jsondata.DomainEyeRunesSuitConfMgr {
		countMap, sumCount := s.countRunesById(id)
		confCountMap, confSumCount := s.countSlotTypeCountMapConf(id)
		if countMap == nil || sumCount == 0 {
			continue
		}
		if confCountMap == nil || confSumCount == 0 {
			continue
		}

		for _, vConf := range conf.GroupSuit {
			if vConf.SlotType == 0 {
				if sumCount < confSumCount {
					continue
				}
			} else {
				if countMap[vConf.SlotType] < confCountMap[vConf.SlotType] {
					continue
				}
			}

			minQuality := s.getSlotRuneMinQualityById(id, vConf.SlotType)
			for _, suitConf := range vConf.RunesSuit {
				if suitConf.Quality != minQuality {
					continue
				}
				engine.CheckAddAttrsToCalc(s.owner, calc, suitConf.Attrs)
			}
		}
	}
}

func (s *DomainEyeSys) calcSuitAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()

	// 确定套装数量
	suitMap := make(map[uint32]uint32) // k:id v:num
	for id, v := range jsondata.DomainEyeSuitConfMgr {
		for _, suitConf := range v.SuitConf {
			_, ok := data[suitConf.Id]
			if ok {
				suitMap[id] += 1
			}
		}
	}

	for id, num := range suitMap {
		conf := jsondata.DomainEyeSuitConfMgr[id]

		var attrs jsondata.AttrVec
		switch num {
		case 1:
			var stage uint32
			for _, suitConf := range conf.SuitConf {
				dData, ok := data[suitConf.Id]
				if ok {
					stage = dData.Stage
					break
				}
			}
			tmpConf := jsondata.GetDomainEyeSuitAttrsByStage(id, num, stage)
			if tmpConf != nil {
				attrs = append(attrs, tmpConf.Attrs...)
			}
		case 2:
			suiConf1, suitConf2 := conf.SuitConf[0], conf.SuitConf[1]
			dData1, dData2 := data[suiConf1.Id], data[suitConf2.Id]

			minStage := utils.MinUInt32(dData1.Stage, dData2.Stage)
			maxStage := utils.MaxUInt32(dData1.Stage, dData2.Stage)

			tmpConf1 := jsondata.GetDomainEyeSuitAttrsByStage(id, 1, maxStage)
			if tmpConf1 != nil {
				attrs = append(attrs, tmpConf1.Attrs...)
			}
			tmpConf2 := jsondata.GetDomainEyeSuitAttrsByStage(id, 2, minStage)
			if tmpConf2 != nil {
				attrs = append(attrs, tmpConf2.Attrs...)
			}
		}
		if len(attrs) > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
		}
	}
}

func (s *DomainEyeSys) calcDomainEyeAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	value := totalSysCalc.GetValue(attrdef.DomainEyeAllAttrRate)
	data := s.GetData()
	for id := range data {
		var attrs jsondata.AttrVec

		for _, attrConf := range s.getDomainEyeAttrs(id) {
			prpConf := jsondata.AttrFightArray[attrConf.Type]
			if prpConf.FormatType > 0 {
				continue
			}
			attrs = append(attrs, attrConf)
		}

		if len(attrs) > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, attrs, uint32(value))
		}
	}
}

func (s *DomainEyeSys) calcDomainRuneAttrAddRate(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	slotCalc := new(attrcalc.FightAttrCalc)

	for id, v := range data {
		for slot := range v.SlotData {
			attrs := s.getDomainEyeRuneAttrs(id, slot)
			if len(attrs) <= 0 {
				continue
			}
			engine.CheckAddAttrsToCalc(s.owner, slotCalc, attrs)

			var rateAttrs jsondata.AttrVec
			hpRate := slotCalc.GetValue(attrdef.DomainEyeRuneHpAttrRate)
			if hpRate > 0 {
				value := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.MaxHp), hpRate)
				if value > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.MaxHp, Value: uint32(value)})
				}
			}
			attackRate := slotCalc.GetValue(attrdef.DomainEyeRuneAttackAttrRate)
			if attackRate > 0 {
				value1 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.AttackWu), attackRate)
				value2 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.AttackMo), attackRate)
				value3 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.Attack), attackRate)
				if value1 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.AttackWu, Value: uint32(value1)})
				}
				if value2 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.AttackMo, Value: uint32(value2)})
				}
				if value3 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.Attack, Value: uint32(value3)})
				}
			}
			defRate := slotCalc.GetValue(attrdef.DomainEyeRuneDefAttrRate)
			if defRate > 0 {
				value1 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.DefWu), defRate)
				value2 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.DefMo), defRate)
				value3 := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.Defend), defRate)
				if value1 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.DefWu, Value: uint32(value1)})
				}
				if value2 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.DefMo, Value: uint32(value2)})
				}
				if value3 > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.Defend, Value: uint32(value3)})
				}
			}
			breakRate := slotCalc.GetValue(attrdef.DomainEyeRuneArmorBreakAttrRate)
			if breakRate > 0 {
				value := utils.CalcMillionRate64(slotCalc.GetValue(attrdef.ArmorBreak), breakRate)
				if value > 0 {
					rateAttrs = append(rateAttrs, &jsondata.Attr{Type: attrdef.ArmorBreak, Value: uint32(value)})
				}
			}

			engine.CheckAddAttrsToCalc(s.owner, calc, rateAttrs)
			slotCalc.Reset()
		}
	}
}

func (s *DomainEyeSys) getDomainEyeAttrs(id uint32) jsondata.AttrVec {
	var attrs jsondata.AttrVec

	data := s.getDomainEyeData(id)

	lv := data.ExpLv.Lv
	lvConf := jsondata.GetDomainEyeLvConf(id, lv)
	if lvConf != nil {
		attrs = append(attrs, lvConf.Attrs...)
	}

	for _, attr := range data.Attrs {
		attrs = append(attrs, &jsondata.Attr{Type: attr.Type, Value: attr.Value})
	}

	breakLv := data.BreakLv
	breakConf := jsondata.GetDomainEyeBreakConf(id, breakLv)
	if breakConf != nil {
		attrs = append(attrs, breakConf.Attrs...)
	}

	return attrs
}

func (s *DomainEyeSys) getDomainEyeRuneAttrs(id, slot uint32) jsondata.AttrVec {
	slotData := s.getDomainEyeSlotData(id, slot)
	if slotData == nil {
		return nil
	}

	var attrs jsondata.AttrVec

	if slotData.ItemSt != nil {
		// 等级属性
		lv := slotData.ExpLv.Lv
		lvConf := jsondata.GetDomainEyeSlotLvConf(id, slot, lv)
		if lvConf != nil && len(lvConf.Attrs) > 0 {
			attrs = append(attrs, lvConf.Attrs...)
		}

		// 符文属性
		itemConf := jsondata.GetItemConfig(slotData.ItemSt.ItemId)
		if itemConf == nil {
			return attrs
		}
		attrs = append(attrs, itemConf.StaticAttrs...)
		attrs = append(attrs, itemConf.PremiumAttrs...)
		attrs = append(attrs, itemConf.SuperAttrs...)

		// 解锁属性
		for _, v := range slotData.AttrIdL {
			attrConf := jsondata.GetDomainEyeSlotAttrConf(id, slot, v)
			if attrConf == nil {
				continue
			}
			if itemConf.Quality < attrConf.RequireQuality {
				continue
			}
			attrs = append(attrs, attrConf.Attrs...)
		}
	}
	return attrs
}

func (s *DomainEyeSys) getDomainEyeRuneItem(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiDomainEyeBag).(*DomainEyeBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *DomainEyeSys) countSlotTypeCountMapConf(id uint32) (map[uint32]uint32, uint32) {
	conf := jsondata.GetDomainEyeConf(id)
	if conf == nil {
		return nil, 0
	}

	countMap := make(map[uint32]uint32)
	sumCount := uint32(0)
	for _, slotConf := range conf.SlotConf {
		countMap[slotConf.SlotType] += 1
		sumCount++
	}
	return countMap, sumCount
}

func (s *DomainEyeSys) getSlotRuneMinQualityById(id, slotType uint32) uint32 {
	data := s.getDomainEyeData(id)
	if data == nil {
		return 0
	}

	minQuality := uint32(math.MaxUint32)
	for slot, sData := range data.SlotData {
		if sData.ItemSt == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(sData.ItemSt.ItemId)
		if itemConf == nil {
			continue
		}

		slotConf := jsondata.GetDomainEyeSlotConf(id, slot)
		if slotConf == nil {
			continue
		}

		if slotType == 0 || slotType == slotConf.SlotType {
			if itemConf.Quality < minQuality {
				minQuality = itemConf.Quality
			}
		}
	}
	return minQuality
}

func calcDomainEyeAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiDomainEye).(*DomainEyeSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttr(calc)
}

func calcDomainEyeAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiDomainEye).(*DomainEyeSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcDomainEyeAttrAddRate(totalSysCalc, calc)
	s.calcDomainRuneAttrAddRate(calc)
}

func (s *DomainEyeSys) countRunesById(id uint32) (map[uint32]uint32, uint32) {
	data := s.getDomainEyeData(id)
	if data == nil {
		return nil, 0
	}

	countMap := make(map[uint32]uint32)
	sumCount := uint32(0)
	for slot, v := range data.SlotData {
		if v.ItemSt == nil {
			continue
		}
		slotConf := jsondata.GetDomainEyeSlotConf(id, slot)
		if slotConf == nil {
			continue
		}
		countMap[slotConf.SlotType] += 1
		sumCount++
	}
	return countMap, sumCount
}

func init() {
	RegisterSysClass(sysdef.SiDomainEye, func() iface.ISystem {
		return &DomainEyeSys{}
	})

	net.RegisterSysProto(82, 2, sysdef.SiDomainEye, (*DomainEyeSys).c2sUpLv)
	net.RegisterSysProto(82, 3, sysdef.SiDomainEye, (*DomainEyeSys).c2sUpBreak)
	net.RegisterSysProto(82, 4, sysdef.SiDomainEye, (*DomainEyeSys).c2sUpAttrs)
	net.RegisterSysProto(82, 5, sysdef.SiDomainEye, (*DomainEyeSys).c2sUpStage)
	net.RegisterSysProto(82, 7, sysdef.SiDomainEye, (*DomainEyeSys).c2sUnLock)

	net.RegisterSysProto(82, 8, sysdef.SiDomainEye, (*DomainEyeSys).c2sSlotUpLv)
	net.RegisterSysProto(82, 9, sysdef.SiDomainEye, (*DomainEyeSys).c2sRunesTakeOn)
	net.RegisterSysProto(82, 10, sysdef.SiDomainEye, (*DomainEyeSys).c2sRunesTakeOff)

	engine.RegAttrCalcFn(attrdef.SaDomainEye, calcDomainEyeAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaDomainEye, calcDomainEyeAttrAddRate)

	gmevent.Register("upDomainEyeAttrs", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		id := utils.AtoUint32(args[0])
		sys, ok := player.GetSysObj(sysdef.SiDomainEye).(*DomainEyeSys)
		if ok && sys.IsOpen() {
			data := sys.getDomainEyeData(id)
			if data == nil {
				return false
			}
			length := len(data.Attrs)
			for i := 0; i < length; i++ {
				attrId := data.Attrs[i].Type
				attrValue := jsondata.GetDomainEyeStageAttrValue(data.Id, data.Stage, attrId)
				data.Attrs[i].Value = attrValue
			}
			sys.s2cDomainEye(id)
			sys.ResetSysAttr(attrdef.SaDomainEye)
			return true
		}
		return false
	}, 1)
}
