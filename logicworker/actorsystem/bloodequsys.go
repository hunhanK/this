/**
 * @Author: lzp
 * @Date: 2025/7/3
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
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
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"
)

type BloodEquSys struct {
	Base
}

func (s *BloodEquSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BloodEquSys) OnLogin() {
	s.s2cInfo()
}

func (s *BloodEquSys) OnOpen() {
	s.s2cInfo()
}

func (s *BloodEquSys) GetData() map[uint32]*pb3.BloodEqu {
	binary := s.GetBinaryData()
	if binary.BloodEquData == nil {
		binary.BloodEquData = make(map[uint32]*pb3.BloodEqu)
	}
	return binary.BloodEquData
}

func (s *BloodEquSys) LearnSkill(slot uint32) {
	data := s.GetData()
	sData, ok := data[slot]
	if !ok {
		return
	}
	for _, skill := range sData.SkillData {
		s.owner.LearnSkill(skill.Id, skill.Level, true)
		sData.SuitSkillIds = pie.Uint32s(sData.SuitSkillIds).Append(skill.Id).Unique()
	}
}

func (s *BloodEquSys) ForgetSkill(slot uint32) {
	data := s.GetData()
	sData, ok := data[slot]
	if !ok {
		return
	}
	for _, skill := range sData.SkillData {
		s.owner.ForgetSkill(skill.Id, true, true, true)
	}
}

func (s *BloodEquSys) getSlotPosData(slot, pos uint32) *pb3.BloodEquPos {
	data := s.GetData()
	sData := data[slot]
	if sData == nil {
		sData = &pb3.BloodEqu{}
		sData.Slot = slot
		data[slot] = sData
	}
	if sData.PosData == nil {
		sData.PosData = make(map[uint32]*pb3.BloodEquPos)
	}
	pData := sData.PosData[pos]
	if pData == nil {
		pData = &pb3.BloodEquPos{}
		pData.Pos = pos
		pData.Awaken = &pb3.BloodEquAwaken{}
		sData.PosData[pos] = pData
	}
	return pData
}

func (s *BloodEquSys) s2cInfo() {
	s.SendProto3(79, 20, &pb3.S2C_79_20{BloodEquData: s.GetData()})
}

func (s *BloodEquSys) s2cSlotPos(slot, pos uint32) {
	s.SendProto3(79, 26, &pb3.S2C_79_26{
		Slot: slot,
		Pos:  pos,
		Data: s.getSlotPosData(slot, pos),
	})
}

func (s *BloodEquSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_79_21
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Slot, req.Pos, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Slot, req.Pos, req.Hdl)
	s.SendProto3(79, 21, &pb3.S2C_79_21{Slot: req.Slot, Pos: req.Pos, Hdl: req.Hdl})
	return nil
}

func (s *BloodEquSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_79_22
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetBloodEquSlotAwakeConf(req.Slot, req.Pos)
	if conf == nil {
		return neterror.ConfNotFoundError("slot:%d, pos:%d config not found", req.Slot, req.Pos)
	}

	pData := s.getSlotPosData(req.Slot, req.Pos)
	if pData.BloodEqu == nil {
		return neterror.ParamsInvalidError("slot:%d not equip blood", req.Slot)
	}

	nextLv := pData.Lv + 1
	nextLvConf := jsondata.GetBloodEquSlotLvConf(req.Slot, req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("lv: %d config is nil", nextLv)
	}

	if pData.Stage < nextLvConf.StageLimit {
		return neterror.ParamsInvalidError("stage lv limit")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBloodEquLvUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	pData.Lv = nextLv
	s.afterLvUp()

	s.owner.TriggerQuestEvent(custom_id.QttBloodEquUpgrade, 0, 1)
	s.logPlayerBehavior(req.Slot, req.Pos, pb3.LogId_LogBloodEquLvUp, nextLv)
	s.SendProto3(79, 22, &pb3.S2C_79_22{Slot: req.Slot, Pos: req.Pos, Lv: nextLv})
	s.s2cSlotPos(req.Slot, req.Pos)
	return nil
}

func (s *BloodEquSys) c2sStageUp(msg *base.Message) error {
	var req pb3.C2S_79_23
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetBloodEquSlotConf(req.Slot, req.Pos)
	if conf == nil {
		return neterror.ParamsInvalidError("slot:%d, pos:%d config not found", req.Slot, req.Pos)
	}

	pData := s.getSlotPosData(req.Slot, req.Pos)
	if pData.BloodEqu == nil {
		return neterror.ParamsInvalidError("slot:%d not equip blood", req.Slot)
	}

	nextStage := pData.Stage + 1
	nextStageConf := jsondata.GetBloodEquSlotStageConf(req.Slot, req.Pos, nextStage)
	if nextStageConf == nil {
		return neterror.ConfNotFoundError("stage: %d config is nil", nextStage)
	}

	if pData.Lv < nextStageConf.LvLimit {
		return neterror.ParamsInvalidError("lv limit")
	}

	if !s.owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBloodEquStageUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	pData.Stage = nextStage
	s.afterStageUp()
	s.logPlayerBehavior(req.Slot, req.Pos, pb3.LogId_LogBloodEquStageUp, nextStage)
	s.SendProto3(79, 23, &pb3.S2C_79_23{Slot: req.Slot, Pos: req.Pos, Stage: nextStage})
	s.s2cSlotPos(req.Slot, req.Pos)
	return nil
}

func (s *BloodEquSys) c2sAwake(msg *base.Message) error {
	var req pb3.C2S_79_24
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetBloodEquSlotAwakeConf(req.Slot, req.Pos)
	if conf == nil {
		return neterror.ConfNotFoundError("slot:%d, pos:%d config not found", req.Slot, req.Pos)
	}

	pData := s.getSlotPosData(req.Slot, req.Pos)
	if pData.BloodEqu == nil {
		return neterror.ParamsInvalidError("slot:%d not equip blood", req.Slot)
	}

	itemConf := jsondata.GetItemConfig(pData.BloodEqu.ItemId)
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemId:%d config not found", pData.BloodEqu.ItemId)
	}

	if len(conf.AwakeConsume) > 0 && !s.owner.ConsumeByConf(conf.AwakeConsume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBloodEquAwaken,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	pData.IsAwaken = true

	s.afterAwaken()
	player := s.GetOwner()
	engine.BroadcastTipMsgById(tipmsgid.BloodEquipAwakeTip, player.GetId(), player.GetName(), itemConf.Name)
	player.TriggerQuestEvent(custom_id.QttBloodEquAwaken, 0, 1)
	s.SendProto3(79, 24, &pb3.S2C_79_24{Slot: req.Slot, Pos: req.Pos, IsSuccess: true})
	s.s2cSlotPos(req.Slot, req.Pos)
	return nil
}

func (s *BloodEquSys) c2sCompose(msg *base.Message) error {
	var req pb3.C2S_79_25
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if len(req.CList) > int(jsondata.GlobalUint("bloodEquBatchComposeNumber")) {
		return neterror.ParamsInvalidError("compose num limit")
	}
	for _, v := range req.CList {
		s.compose(v.Hdl, v.Consumes1, v.Consumes2)
	}

	s.ResetSysAttr(attrdef.SaBloodEqu)
	s.owner.TriggerQuestEvent(custom_id.QttBloodEquCompose, 0, 1)
	s.SendProto3(79, 25, &pb3.S2C_79_25{IsSuccess: true})
	s.s2cInfo()
	return nil
}

func (s *BloodEquSys) compose(hdl uint64, consumes1, consumes2 []uint64) {
	itemSt := s.getBloodEquItem(hdl)
	if itemSt == nil {
		return
	}

	conf := jsondata.GetBloodEquComposeConf(itemSt.ItemId)
	if conf == nil {
		return
	}

	itemConf := jsondata.GetItemConfig(conf.NewItemId)
	if itemConf == nil {
		return
	}

	consumes1 = pie.Uint64s(consumes1).Unique()
	consumes2 = pie.Uint64s(consumes2).Unique()

	if len(consumes1) > 0 {
		if len(conf.ComposeConf) < 1 {
			return
		}
		if !s.checkComposeConf(conf.ItemId, conf.ComposeConf[0], consumes1) {
			logger.LogError("bloodEqu itemId: %d compose consume1 error", conf.ItemId)
			return
		}
	}

	if len(consumes2) > 0 {
		if len(conf.ComposeConf) < 2 {
			return
		}
		if !s.checkComposeConf(conf.ItemId, conf.ComposeConf[1], consumes2) {
			logger.LogError("bloodEqu itemId: %d compose consume2 error", conf.ItemId)
			return
		}
	}

	player := s.GetOwner()

	// 移除道具
	var consumes []uint64
	consumes = append(consumes, consumes1...)
	consumes = append(consumes, consumes2...)
	for _, tmpHdl := range consumes {
		player.RemoveBloodEquItemByHandle(tmpHdl, pb3.LogId_LogBloodEquCompose)
	}

	var slot, pos uint32
	if s.isEquip(hdl) {
		slot = uint32(itemSt.Ext.OwnerId)
		pos = itemSt.Pos
	}

	// 移除自己
	player.RemoveBloodEquItemByHandle(hdl, pb3.LogId_LogBloodEquCompose)

	// 获得新的道具
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.NewItemId,
		Count:  1,
		LogId:  pb3.LogId_LogBloodEquCompose,
		Bind:   false,
	}
	player.AddItem(newItem)
	engine.BroadcastTipMsgById(tipmsgid.BloodEquipComposeTip, player.GetId(), player.GetName(), itemConf.Name, itemConf.Star)

	// 替换上
	if slot > 0 && pos > 0 {
		newItemSt := s.getBloodEquItem(newItem.AddItemAfterHdl)
		newItemSt.Ext.OwnerId = uint64(slot)
		newItemSt.Pos = pos

		pData := s.getSlotPosData(slot, pos)
		pData.BloodEqu = newItemSt
	}
}

func (s *BloodEquSys) checkComposeConf(itemId uint32, conf *jsondata.BloodEquCompose, consumes []uint64) bool {
	switch conf.Type {
	case 0:
		var count uint32
		for _, hdl := range consumes {
			itemSt := s.getBloodEquItem(hdl)
			if itemSt.ItemId == itemId {
				count++
			}
		}
		if count == conf.ItemNum {
			return true
		}
	case 1:
		var count uint32
		for _, hdl := range consumes {
			itemSt := s.getBloodEquItem(hdl)
			itemConf := jsondata.GetItemConfig(itemSt.ItemId)
			if itemConf == nil {
				continue
			}
			if (conf.SpSubType == 0 || conf.SpSubType == itemConf.SubType) &&
				conf.SpType == itemConf.Type && conf.SpStar == itemConf.Star &&
				conf.SpGrade == itemConf.Grade {
				count++
			}
			if count == conf.ItemNum {
				return true
			}
		}
	default:
		return false
	}
	return false
}

func (s *BloodEquSys) isEquip(hdl uint64) bool {
	itemSt := s.getBloodEquItem(hdl)
	if itemSt == nil {
		return false
	}
	if itemSt.Pos > 0 || itemSt.Ext.OwnerId > 0 {
		return true
	}
	return false
}

func (s *BloodEquSys) triggerSuitEffect(slot uint32) {
	data := s.GetData()
	sData, ok := data[slot]
	if !ok {
		return
	}

	// 遗忘已学习技能
	for _, skillId := range sData.SuitSkillIds {
		s.owner.ForgetSkill(skillId, true, false, true)
	}
	sData.SuitSkillIds = sData.SuitSkillIds[:]

	var itemList []*pb3.ItemSt
	for _, pData := range sData.PosData {
		itemSt := pData.BloodEqu
		if itemSt == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(itemSt.ItemId)
		if itemConf == nil {
			continue
		}
		itemList = append(itemList, itemSt)
	}
	sort.Slice(itemList, func(i, j int) bool {
		itemConf1 := jsondata.GetItemConfig(itemList[i].ItemId)
		itemConf2 := jsondata.GetItemConfig(itemList[j].ItemId)
		return itemConf1.Star > itemConf2.Star
	})

	sConfL := jsondata.GetBloodEquSuitConfL(slot)
	for _, sConf := range sConfL {
		var tmpItems []*pb3.ItemSt
		for _, itemSt := range itemList {
			itemConf := jsondata.GetItemConfig(itemSt.ItemId)
			if itemConf.Grade < sConf.Grade {
				continue
			}
			tmpItems = append(tmpItems, itemSt)
		}

		// 套装数量不满足
		if len(tmpItems) < int(sConf.SuitNum) {
			continue
		}

		tmpItemSt := tmpItems[sConf.SuitNum-1]
		tmpItemConf := jsondata.GetItemConfig(tmpItemSt.ItemId)

		var suitLvConf *jsondata.BloodEquSuitLv
		for _, v := range sConf.SuitLvConf {
			if tmpItemConf.Star >= v.MinStar && tmpItemConf.Star <= v.MaxStar {
				suitLvConf = v
				break
			}
		}

		// 没有此套装
		if suitLvConf == nil {
			continue
		}

		if len(suitLvConf.Skill) <= 0 || len(suitLvConf.SkillLv) <= 0 ||
			len(suitLvConf.Skill) != len(suitLvConf.SkillLv) {
			continue
		}

		// 学习套装技能
		if suitLvConf.ISFinalSkill {
			if sData.SkillData == nil {
				sData.SkillData = make(map[uint32]*pb3.Skill)
			}
			for i := range suitLvConf.Skill {
				skillId, skillLv := suitLvConf.Skill[i], suitLvConf.SkillLv[i]
				sData.SkillData[skillId] = &pb3.Skill{Id: skillId, Level: skillLv}
			}
		} else {
			for i := range suitLvConf.Skill {
				skillId, skillLv := suitLvConf.Skill[i], suitLvConf.SkillLv[i]
				s.owner.LearnSkill(skillId, skillLv, true)
				sData.SuitSkillIds = pie.Uint32s(sData.SuitSkillIds).Append(skillId).Unique()
			}
		}

		if sConf.NoticeId > 0 && !utils.SliceContainsUint32(sData.SuitIds, sConf.SuitId) {
			sData.SuitIds = append(sData.SuitIds, sConf.SuitId)
			player := s.GetOwner()
			bloodConf := jsondata.GetBloodConf(slot)
			if bloodConf != nil {
				engine.BroadcastTipMsgById(sConf.NoticeId, player.GetId(), player.GetName(), sConf.SuitNum, bloodConf.BloodName)
			}
		}
	}
}

func (s *BloodEquSys) checkTakeOn(slot, pos uint32, hdl uint64) error {
	bloodSys, ok := s.owner.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !bloodSys.IsOpen() {
		return nil
	}
	bloodEquip := s.getBloodEquItem(hdl)
	if bloodEquip == nil {
		return neterror.ParamsInvalidError("not blood equip")
	}
	itemConf := jsondata.GetItemConfig(bloodEquip.GetItemId())
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId:%d config not found", itemConf.Id)
	}
	conf := jsondata.GetBloodEquSlotConf(slot, pos)
	if conf == nil {
		return neterror.ConfNotFoundError("slot:%d, pos:%d config not found", slot, pos)
	}
	if itemConf.Type != conf.Type || itemConf.SubType != conf.SubType {
		return neterror.ParamsInvalidError("can not equip")
	}
	return nil
}

func (s *BloodEquSys) takeOn(slot, pos uint32, hdl uint64) {
	pData := s.getSlotPosData(slot, pos)
	if pData.BloodEqu != nil {
		s.takeOff(slot, pos)
	}

	bloodEquItem := s.getBloodEquItem(hdl)
	bloodEquItem.Pos = pos
	bloodEquItem.Ext.OwnerId = uint64(slot)
	pData.BloodEqu = bloodEquItem

	s.afterTakeOn()
	s.triggerSuitEffect(slot)
	s.owner.TriggerEvent(custom_id.AeBloodEquTakeOn, slot)
	s.owner.TriggerQuestEventRange(custom_id.QttBloodEquItemEquip)
	s.s2cSlotPos(slot, pos)
}

func (s *BloodEquSys) takeOff(slot, pos uint32) {
	pData := s.getSlotPosData(slot, pos)
	if pData.BloodEqu == nil {
		return
	}

	bloodEquItem := pData.BloodEqu
	bloodEquItem.Pos = 0
	bloodEquItem.Ext.OwnerId = 0

	s.afterTakeOff()
}

func (s *BloodEquSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaBloodEqu)
}

func (s *BloodEquSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaBloodEqu)
}

func (s *BloodEquSys) afterLvUp() {
	s.ResetSysAttr(attrdef.SaBloodEqu)
}

func (s *BloodEquSys) afterStageUp() {
	s.ResetSysAttr(attrdef.SaBloodEqu)
}

func (s *BloodEquSys) afterAwaken() {
	s.ResetSysAttr(attrdef.SaBloodEqu)
}

func (s *BloodEquSys) logPlayerBehavior(slot, pos uint32, logId pb3.LogId, value uint32) {
	logworker.LogPlayerBehavior(s.owner, logId, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  slot,
			"pos":   pos,
			"level": value,
		}),
	})
}

func (s *BloodEquSys) getBloodEquItem(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiBloodEquBag).(*BloodEquBagSys)
	if !ok || !bagSys.IsOpen() {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *BloodEquSys) handleUpgradeAwake(slot, pos uint32, param *custom_id.BloodEquAwakeParam) {
	aConf := jsondata.GetBloodEquSlotAwakeConf(slot, pos)
	if aConf == nil {
		return
	}

	pData := s.getSlotPosData(slot, pos)
	lConf := jsondata.GetBloodEquSlotAwakeLvConf(slot, pos, pData.Awaken.Lv)
	if lConf == nil {
		return
	}

	isAdd := false
	switch aConf.Cond {
	case custom_id.BloodEquAwakeCondKillMon:
		sceneId := param.Param0
		if utils.SliceContainsUint32(aConf.Params, sceneId) {
			isAdd = true
		}
	default:
		isAdd = false
	}

	if isAdd {
		pData.Awaken.Count += param.Count
		isUp := false
		for {
			next := pData.Awaken.Lv + 1
			lConf := jsondata.GetBloodEquSlotAwakeLvConf(slot, pos, next)
			if lConf == nil {
				break
			}

			if pData.Awaken.Count >= lConf.Count {
				pData.Awaken.Lv = next
				pData.Awaken.Count -= lConf.Count
				isUp = true
			} else {
				break
			}
		}
		s.s2cSlotPos(slot, pos)
		if isUp {
			s.ResetSysAttr(attrdef.SaBloodEqu)
		}
	}
}

func (s *BloodEquSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	var attrs jsondata.AttrVec
	for slot, sData := range data {
		var subAttrs jsondata.AttrVec
		for pos, pData := range sData.PosData {
			if pData.BloodEqu == nil {
				continue
			}

			// 装备本身属性
			itemConf := jsondata.GetItemConfig(pData.BloodEqu.ItemId)
			if itemConf == nil {
				continue
			}
			subAttrs = append(subAttrs, itemConf.StaticAttrs...)
			subAttrs = append(subAttrs, itemConf.PremiumAttrs...)
			subAttrs = append(subAttrs, itemConf.SuperAttrs...)

			// 等级属性
			lvConf := jsondata.GetBloodEquSlotLvConf(slot, pos, pData.Lv)
			if lvConf != nil {
				subAttrs = append(subAttrs, lvConf.Attrs...)
			}
			// 进阶属性
			stageConf := jsondata.GetBloodEquSlotStageConf(slot, pos, pData.Stage)
			if stageConf != nil {
				subAttrs = append(subAttrs, stageConf.Attrs...)
			}

			// 觉醒属性
			if pData.IsAwaken {
				if pData.Awaken == nil {
					pData.Awaken = &pb3.BloodEquAwaken{}
				}
				awaken := pData.Awaken
				aConf := jsondata.GetBloodEquSlotAwakeLvConf(slot, pos, awaken.Lv)
				if aConf != nil {
					subAttrs = append(subAttrs, aConf.Attrs...)
				}
			}
		}
		attrs = append(attrs, subAttrs...)
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
}

func calcBloodEquAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiBloodEqu).(*BloodEquSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttr(calc)
}

func bloodEquHandleKillMon(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBloodEqu).(*BloodEquSys)
	if !ok || !sys.IsOpen() {
		return
	}
	if len(args) < 4 {
		return
	}
	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}
	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	data := sys.GetData()
	for slot, sData := range data {
		for pos := range sData.PosData {
			sys.handleUpgradeAwake(slot, pos, &custom_id.BloodEquAwakeParam{
				Cond:   custom_id.BloodEquAwakeCondKillMon,
				Param0: sceneId,
				Count:  count,
			})
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiBloodEqu, func() iface.ISystem {
		return &BloodEquSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaBloodEqu, calcBloodEquAttr)
	event.RegActorEvent(custom_id.AeKillMon, bloodEquHandleKillMon)
	engine.RegQuestTargetProgress(custom_id.QttBloodEquItemEquip, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		s, ok := player.GetSysObj(sysdef.SiBloodEqu).(*BloodEquSys)
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
			for _, pData := range sData.PosData {
				if pData.BloodEqu == nil {
					continue
				}
				itemConf := jsondata.GetItemConfig(pData.BloodEqu.ItemId)
				if itemConf == nil {
					continue
				}
				if itemConf.Grade >= spGrade && itemConf.Quality >= spQuality && itemConf.Star >= spStar {
					count += 1
				}
			}
		}
		return count
	})

	net.RegisterSysProtoV2(79, 21, sysdef.SiBloodEqu, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodEquSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(79, 22, sysdef.SiBloodEqu, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodEquSys).c2sLvUp
	})
	net.RegisterSysProtoV2(79, 23, sysdef.SiBloodEqu, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodEquSys).c2sStageUp
	})
	net.RegisterSysProtoV2(79, 24, sysdef.SiBloodEqu, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodEquSys).c2sAwake
	})
	net.RegisterSysProtoV2(79, 25, sysdef.SiBloodEqu, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BloodEquSys).c2sCompose
	})
}
