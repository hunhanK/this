/**
 * @Author: lzp
 * @Date: 2025/7/2
 * @Desc:
**/

package actorsystem

import (
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
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type BloodSys struct {
	Base
}

func (s *BloodSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BloodSys) OnLogin() {
	s.s2cInfo()
}

func (s *BloodSys) OnOpen() {
	s.openSlot()
	s.s2cInfo()
}

func (s *BloodSys) OnNewDay() {
	s.openSlot()
	s.s2cInfo()
}

func (s *BloodSys) GetData() map[uint32]*pb3.Blood {
	binary := s.GetBinaryData()
	if binary.BloodData == nil {
		binary.BloodData = make(map[uint32]*pb3.Blood)
	}
	return binary.BloodData
}

func (s *BloodSys) LearnSkill(slot uint32) {
	slotData := s.getSlotData(slot)
	if slotData == nil || slotData.Blood == nil {
		return
	}
	itemId := slotData.Blood.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf != nil && itemConf.Type == itemdef.ItemTypeBlood &&
		itemConf.EquipSkill > 0 && itemConf.EquipSkillLv > 0 {
		s.owner.LearnSkill(itemConf.EquipSkill, itemConf.EquipSkillLv, true)
	}
}

func (s *BloodSys) CheckCanLearnSkill(slot uint32) bool {
	slotData := s.getSlotData(slot)
	if slotData == nil || slotData.Blood == nil {
		return false
	}
	itemId := slotData.Blood.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf != nil && itemConf.Type == itemdef.ItemTypeBlood && itemConf.EquipSkill > 0 && itemConf.EquipSkillLv > 0 {
		return true
	}
	return false
}

func (s *BloodSys) ForgetSkill(slot uint32) {
	slotData := s.getSlotData(slot)
	if slotData == nil || slotData.Blood == nil {
		return
	}
	itemId := slotData.Blood.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf != nil && itemConf.Type == itemdef.ItemTypeBlood &&
		itemConf.EquipSkill > 0 && itemConf.EquipSkillLv > 0 {
		s.owner.ForgetSkill(itemConf.EquipSkill, true, true, true)
	}
}

func (s *BloodSys) TakeOffByCompose(slot uint32) {
	slotData := s.getSlotData(slot)
	if slotData == nil || slotData.Blood == nil {
		return
	}
	bloodItem := slotData.Blood
	bloodItem.Pos = 0
	s.s2cInfo()
	s.owner.RemoveBloodItemByHandle(bloodItem.Handle, pb3.LogId_LogComposeItem)
}

func (s *BloodSys) TakeOnByCompose(slot uint32, hdl uint64) {
	slotData := s.getSlotData(slot)
	if slotData == nil {
		return
	}
	bloodItem := s.getBloodItem(hdl)
	bloodItem.Pos = slot
	slotData.Blood = bloodItem
	s.ResetSysAttr(attrdef.SaBlood)
	s.s2cInfo()
}

func (s *BloodSys) getSlotData(slot uint32) *pb3.Blood {
	data := s.GetData()
	sData, ok := data[slot]
	if !ok {
		return nil
	}
	return sData
}

func (s *BloodSys) openSlot() {
	data := s.GetData()
	for _, conf := range jsondata.BloodConfMgr {
		if gshare.GetMergeSrvDayByTimes(conf.MergeTimes) < conf.MergeDays {
			continue
		}
		_, ok := data[conf.SlotId]
		if ok {
			continue
		}
		data[conf.SlotId] = &pb3.Blood{
			Slot: conf.SlotId,
		}
	}
}

func (s *BloodSys) s2cInfo() {
	s.SendProto3(79, 0, &pb3.S2C_79_0{BloodData: s.GetData()})
}

func (s *BloodSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_79_1
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Slot, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Slot, req.Hdl)
	s.SendProto3(79, 1, &pb3.S2C_79_1{Hdl: req.Hdl, Slot: req.Slot})
	return nil
}

func (s *BloodSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_79_2
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	slotData := s.getSlotData(req.Slot)
	if slotData == nil {
		return neterror.ParamsInvalidError("slot:%d not open", req.Slot)
	}
	if slotData.Blood == nil {
		return neterror.ParamsInvalidError("slot:%d not equip blood", req.Slot)
	}

	nextLv := slotData.Lv + 1
	nextLvConf := jsondata.GetBloodLvConf(req.Slot, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("lv: %d config is nil", nextLv)
	}

	if slotData.BreakLv < nextLvConf.BreakLvLimit {
		return neterror.ParamsInvalidError("break lv limit")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBloodLvUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	slotData.Lv = nextLv
	s.afterLvUp()
	s.owner.TriggerQuestEvent(custom_id.QttBloodUpgrade, 0, 1)
	s.logPlayerBehavior(req.Slot, pb3.LogId_LogBloodLvUp, nextLv)
	s.SendProto3(79, 2, &pb3.S2C_79_2{Slot: req.Slot, Lv: nextLv})
	s.s2cInfo()
	return nil
}

func (s *BloodSys) c2sBreakUp(msg *base.Message) error {
	var req pb3.C2S_79_3
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	slotData := s.getSlotData(req.Slot)
	if slotData == nil {
		return neterror.ParamsInvalidError("slot:%d not open", req.Slot)
	}
	if slotData.Blood == nil {
		return neterror.ParamsInvalidError("slot:%d not equip blood", req.Slot)
	}

	nextBreak := slotData.BreakLv + 1
	nextBreakConf := jsondata.GetBloodBreakConf(req.Slot, nextBreak)
	if nextBreakConf == nil {
		return neterror.ConfNotFoundError("break: %d config is nil", nextBreak)
	}

	if slotData.Lv < nextBreakConf.LvLimit {
		return neterror.ParamsInvalidError("lv not satisfy")
	}

	if !s.owner.ConsumeByConf(nextBreakConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBloodBreakUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	slotData.BreakLv = nextBreak

	s.afterBreak()
	s.owner.TriggerQuestEvent(custom_id.QttBloodBreak, 0, 1)
	s.logPlayerBehavior(req.Slot, pb3.LogId_LogBloodBreakUp, nextBreak)
	s.SendProto3(79, 3, &pb3.S2C_79_3{Slot: req.Slot, BreakLv: nextBreak})
	s.s2cInfo()
	return nil
}

func (s *BloodSys) checkTakeOn(slot uint32, hdl uint64) error {
	equip := s.getBloodItem(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	slotData := s.getSlotData(slot)
	if slotData == nil {
		return neterror.ParamsInvalidError("slot:%d not open", slot)
	}

	conf := jsondata.GetBloodConf(slot)
	if conf == nil {
		return neterror.ConfNotFoundError("blood slot:%d config not found", slot)
	}
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf.Type != itemdef.ItemTypeBlood && itemConf.SubType != slot {
		return neterror.ParamsInvalidError("item not fairy sword")
	}

	return nil
}

func (s *BloodSys) takeOn(slot uint32, hdl uint64) {
	slotData := s.getSlotData(slot)
	oldBlood := slotData.Blood
	if oldBlood != nil {
		s.takeOff(slot)
	}

	bloodItem := s.getBloodItem(hdl)
	bloodItem.Pos = slot
	slotData.Blood = bloodItem

	// 遗忘旧的技能
	if oldBlood != nil {
		itemConf := jsondata.GetItemConfig(oldBlood.ItemId)
		if itemConf != nil {
			s.owner.ForgetSkill(itemConf.EquipSkill, true, false, true)
		}
	}

	s.owner.TriggerQuestEventRange(custom_id.QttBloodItemEquip)
	s.owner.TriggerEvent(custom_id.AeBloodTakeOn, slot)
	s.afterTakeOn()
	s.s2cInfo()
}

func (s *BloodSys) takeOff(slot uint32) {
	slotData := s.getSlotData(slot)
	if slotData.Blood == nil {
		return
	}

	bloodItem := slotData.Blood
	bloodItem.Pos = 0
}

func (s *BloodSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaBlood)
}

func (s *BloodSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaBlood)
}

func (s *BloodSys) afterLvUp() {
	s.ResetSysAttr(attrdef.SaBlood)
}

func (s *BloodSys) afterBreak() {
	s.ResetSysAttr(attrdef.SaBlood)
}

func (s *BloodSys) logPlayerBehavior(slot uint32, logId pb3.LogId, value uint32) {
	logworker.LogPlayerBehavior(s.owner, logId, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  slot,
			"level": value,
		}),
	})
}

func (s *BloodSys) getBloodItem(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiBloodBag).(*BloodBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *BloodSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	var attrs jsondata.AttrVec
	for _, slotData := range data {
		if slotData.Blood == nil {
			continue
		}

		// 装备本身属性
		itemConf := jsondata.GetItemConfig(slotData.Blood.ItemId)
		if itemConf == nil {
			continue
		}
		attrs = append(attrs, itemConf.StaticAttrs...)
		attrs = append(attrs, itemConf.PremiumAttrs...)
		attrs = append(attrs, itemConf.SuperAttrs...)

		// 等级属性
		lvConf := jsondata.GetBloodLvConf(slotData.Slot, slotData.Lv)
		if lvConf != nil {
			attrs = append(attrs, lvConf.Attrs...)
		}

		// 突破属性
		breakConf := jsondata.GetBloodBreakConf(slotData.Slot, slotData.BreakLv)
		if breakConf != nil {
			attrs = append(attrs, breakConf.Attrs...)
		}
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
}

func calcBloodAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiBlood, func() iface.ISystem {
		return &BloodSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaBlood, calcBloodAttr)
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiBlood).(*BloodSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.OnNewDay()
	})
	engine.RegQuestTargetProgress(custom_id.QttBloodItemEquip, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		s, ok := player.GetSysObj(sysdef.SiBlood).(*BloodSys)
		if !ok || !s.IsOpen() {
			return 0
		}

		if len(ids) != 3 {
			return 0
		}

		spGrade, spQuality, spStar := ids[0], ids[1], ids[2]
		data := s.GetData()
		var count uint32
		for _, sData := range data {
			if sData.Blood == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(sData.Blood.ItemId)
			if itemConf == nil {
				continue
			}
			if itemConf.Grade >= spGrade && itemConf.Quality >= spQuality && itemConf.Star >= spStar {
				count += 1
			}
		}
		return count
	})

	net.RegisterSysProtoV2(79, 1, sysdef.SiBlood, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(79, 2, sysdef.SiBlood, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodSys).c2sLvUp
	})
	net.RegisterSysProtoV2(79, 3, sysdef.SiBlood, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodSys).c2sBreakUp
	})
}
