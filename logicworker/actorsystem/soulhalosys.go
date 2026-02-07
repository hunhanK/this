/**
 * @Author: LvYuMeng
 * @Date: 2024/5/13
 * @Desc: 魂环
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
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
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
)

const (
	UpdateSoulHaloInSlotDefault    = 0
	UpdateSoulHaloInSlotTakeOn     = 1
	UpdateSoulHaloInSlotLevelUp    = 2
	UpdateSoulHaloInSlotBreak      = 3
	UpdateSoulHaloInSlotRefine     = 4
	UpdateSoulHaloInSlotRefineSave = 5
)

const (
	UpdateSoulBoneInSlotDefault = 0
	UpdateSoulBoneInSlotTakeOn  = 1
	UpdateSoulBoneInSlotStageUp = 2
)

type SoulHaloSys struct {
	Base

	expUpLvs map[uint32]*uplevelbase.ExpUpLv
}

func (s *SoulHaloSys) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.initExpUpLv()
}

func (s *SoulHaloSys) OnLogin() {
	s.checkRefineSuit(true)
}

func (s *SoulHaloSys) onJobChange(job uint32) {
	data := s.data()
	modifyItemSt := func(itemSt *pb3.ItemSt) {
		if nil == itemSt {
			return
		}
		newItemId := jsondata.GetJobChangeItemConfByIdAndJob(itemSt.ItemId, job)
		if newItemId == 0 {
			return
		}
		s.owner.LogInfo("change item %d to %d", itemSt.ItemId, newItemId)
		itemSt.ItemId = newItemId
	}

	for slot, slotInfo := range data.SoltInfo {
		if nil != slotInfo.SoulHalo {
			s.cancelActiveSuit(slot, slotInfo.SoulHalo.GetItemId())
			modifyItemSt(slotInfo.SoulHalo)
		}

		for _, soulBone := range slotInfo.SoulBone {
			modifyItemSt(soulBone)
		}

		s.appearActive(slot, false)
		s.checkActiveSuit(slot, false)
	}

	s.checkRefineSuit(false)
}

func (s *SoulHaloSys) ResetAttrs() {
	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.ResetSysAttr(attrdef.SaSoulBone)
}

func (s *SoulHaloSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SoulHaloSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SoulHaloSys) OnOpen() {
	s.initSoulHaloQi()
	s.initExpUpLv()
}

func (s *SoulHaloSys) GetData() *pb3.SoulHaloData {
	return s.data()
}

func (s *SoulHaloSys) data() *pb3.SoulHaloData {
	binary := s.GetBinaryData()
	if nil == binary.SoulHaloData {
		binary.SoulHaloData = &pb3.SoulHaloData{}
	}
	if nil == binary.SoulHaloData.SoltInfo {
		binary.SoulHaloData.SoltInfo = make(map[uint32]*pb3.SoulHaloInfo)
	}
	if binary.SoulHaloData.SoulHaloQiInfo == nil {
		binary.SoulHaloData.SoulHaloQiInfo = make(map[uint32]*pb3.SoulHaloQi)
	}
	return binary.SoulHaloData
}

func (s *SoulHaloSys) getSoulHaloInfoBySlot(slot uint32) *pb3.SoulHaloInfo {
	data := s.data()
	v, ok := data.SoltInfo[slot]
	if !ok {
		return nil
	}

	if nil == v.SoulBone {
		v.SoulBone = make(map[uint32]*pb3.ItemSt)
	}
	return v
}

func (s *SoulHaloSys) getSoulHaloBySlot(slot uint32) (*pb3.ItemSt, error) {
	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil == slotInfo {
		return nil, neterror.ParamsInvalidError("slot(%d) is nil", slot)
	}
	if nil == slotInfo.SoulHalo {
		return nil, neterror.ParamsInvalidError("soul halo in slot(%d) is empty", slot)
	}
	return slotInfo.SoulHalo, nil
}

func (s *SoulHaloSys) openSlot(slot uint32) {
	data := s.data()
	if nil == data.SoltInfo[slot] {
		data.SoltInfo[slot] = &pb3.SoulHaloInfo{}
	}
	if nil == data.SoltInfo[slot].SoulBone {
		data.SoltInfo[slot].SoulBone = make(map[uint32]*pb3.ItemSt)
	}
}

func (s *SoulHaloSys) s2cInfo() {
	s.SendProto3(67, 0, &pb3.S2C_67_0{Data: s.data()})
}

func (s *SoulHaloSys) isSlotLock(slot uint32) bool {
	data := s.data()
	if _, ok := data.SoltInfo[slot]; ok {
		return true
	}

	conf := jsondata.GetSoulHaloSlotConf(slot)
	if nil == conf {
		return false
	}

	if tabConf := jsondata.GetSoulHaloTabConf(conf.TabId); nil != tabConf {
		if tabConf.MergeTimes > 0 && tabConf.MergeTimes > gshare.GetMergeTimes() {
			return false
		}
		if tabConf.SrvOpenDay > 0 && tabConf.SrvOpenDay > gshare.GetOpenServerDay() {
			return false
		}
	}

	if len(conf.MustConsume) > 0 {
		return false
	}

	//check auto
	if len(conf.Cond) == 0 {
		if len(conf.Consume) > 0 { //道具解锁
			return false
		} else { //默认开
			s.openSlot(slot)
			return true
		}
	}

	for _, v := range conf.Cond {
		if CheckReach(s.owner, v.Type, v.Val) {
			s.openSlot(slot)
			return true
		}
	}
	return false
}

func (s *SoulHaloSys) c2sUnLock(msg *base.Message) error {
	var req pb3.C2S_67_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	slot := req.Solt

	conf := jsondata.GetSoulHaloSlotConf(slot)
	if nil == conf {
		return neterror.ConfNotFoundError("soul halo slot(%d) conf is nil", slot)
	}

	if s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is unlock", slot)
	}

	if len(conf.Consume) == 0 {
		return neterror.ParamsInvalidError("soul halo slot(%d) not allow use item unlock", slot)
	}

	consume := jsondata.MergeConsumeVec(conf.Consume, conf.MustConsume)
	if !s.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUnLockSoulHaloSlot}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.openSlot(slot)

	s.SendProto3(67, 7, &pb3.S2C_67_7{Solt: slot})
	return nil
}

func (s *SoulHaloSys) checkTakeOnSlotHandle(equip *pb3.ItemSt, slot uint32) (bool, error) {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return false, neterror.ConfNotFoundError("item itemConf(%d) nil", equip.GetItemId())
	}

	if !itemdef.IsSoulHalo(itemConf.Type) {
		return false, neterror.SysNotExistError("not soul halo item")
	}

	_, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, neterror.SysNotExistError("bag sys get err")
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}

	slotConf := jsondata.GetSoulHaloSlotConf(slot)
	if nil == slotConf {
		return false, neterror.ParamsInvalidError("soul halo slot(%d) conf is nil", slot)
	}

	if slotConf.Quality != 0 && itemConf.Stage != slotConf.Quality {
		return false, neterror.ParamsInvalidError("soul halo quality not match")
	}

	if !s.getSoulHaloExtData(equip).IsIdentify {
		return false, neterror.ParamsInvalidError("not identify")
	}

	conf := jsondata.GetSoulHaloConf()
	for _, v := range conf.SlotConf {
		if v.SlotId == slot {
			continue
		}
		otherSlot := s.getSoulHaloInfoBySlot(v.SlotId)
		if nil == otherSlot || nil == otherSlot.SoulHalo {
			continue
		}
		if otherSlot.SoulHalo.GetItemId() == equip.GetItemId() {
			return false, neterror.ParamsInvalidError("has same soul halo in slot")
		}
	}

	return true, nil
}

func (s *SoulHaloSys) takeOff(slot uint32) error {
	if !s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is not unlock", slot)
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)

	oldEquip := slotInfo.SoulHalo
	if nil == oldEquip {
		return neterror.ParamsInvalidError("soul halo slot conf is nil")
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	if succ := bagSys.AddItemPtr(oldEquip, true, pb3.LogId_LogSoulHaloTakeOff); !succ {
		return neterror.InternalError("soul halo take off(%d) err", slot)
	}

	slotInfo.SoulHalo = nil
	s.SendProto3(67, 2, &pb3.S2C_67_2{Solt: slot})

	s.afterTakeOff(slot, oldEquip)
	return nil
}

func (s *SoulHaloSys) takeOn(hdl uint64, slot uint32) error {
	if !s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is not unlock", slot)
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	equip := bagSys.FindItemByHandle(hdl)
	if nil == equip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	ok, err := s.checkTakeOnSlotHandle(equip, slot)
	if !ok {
		return err
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil != slotInfo.SoulHalo {
		if err := s.takeOff(slot); err != nil {
			return err
		}
	}

	if removeSucc := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogSoulHaloTakeOn); !removeSucc {
		return neterror.InternalError("remove soul halo hdl:%d item:%d failed", equip.GetHandle(), equip.GetItemId())
	}

	equip.Bind = true
	slotInfo.SoulHalo = equip
	s.updateSoulHaloInSlot(slot, UpdateSoulHaloInSlotTakeOn)

	s.afterTakeOn(slot)
	s.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXYearSoulRingNum)
	return nil
}

func (s *SoulHaloSys) updateSoulHaloInSlot(slot, event uint32) {
	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil == slotInfo {
		return
	}
	s.SendProto3(67, 1, &pb3.S2C_67_1{Solt: slot, SoulHalo: slotInfo.SoulHalo, Event: event})
}

func (s *SoulHaloSys) updateSoulBoneInSlot(slot, pos, event uint32) {
	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil == slotInfo {
		return
	}
	s.SendProto3(67, 3, &pb3.S2C_67_3{Solt: slot, SoulBone: slotInfo.SoulBone[pos], Event: event, Pos: pos})
}

func (s *SoulHaloSys) checkActiveSuit(slot uint32, sendTips bool) {
	conf := jsondata.GetSoulHaloConf()
	if nil == conf {
		return
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)
	equip := slotInfo.SoulHalo

	if nil == equip {
		return
	}

	haloConf := conf.HaloConf[equip.GetItemId()]
	if nil == haloConf {
		return
	}

	if len(slotInfo.SoulBone) < len(haloConf.Suits) {
		return
	}

	isSet := make(map[uint32]struct{}, len(haloConf.Suits))
	for _, itemId := range haloConf.Suits {
		isSet[itemId] = struct{}{}
	}

	var count int
	for _, bone := range slotInfo.SoulBone {
		if _, ok := isSet[bone.GetItemId()]; ok {
			count++
		}
	}

	if count != len(haloConf.Suits) {
		return
	}

	skill := make(map[uint32]uint32)
	breakLv := s.getSoulHaloBreakLv(equip)
	for _, sk := range haloConf.Skill {
		if sk.BreakLv > breakLv {
			continue
		}
		skill[sk.SkillId] = utils.MaxUInt32(skill[sk.SkillId], sk.SkillLv)
	}

	for skillId, skillLv := range skill {
		s.owner.LearnSkill(skillId, skillLv, sendTips)
	}
}

func (s *SoulHaloSys) cancelActiveSuit(slot, soulHaloItemId uint32) {
	conf := jsondata.GetSoulHaloConf()
	if nil == conf {
		return
	}

	haloConf := conf.HaloConf[soulHaloItemId]
	if nil == haloConf {
		return
	}

	for _, sk := range haloConf.Skill {
		s.owner.ForgetSkill(sk.SkillId, true, true, true)
	}
}

func (s *SoulHaloSys) appearActive(slot uint32, isTip bool) {
	slotInfo := s.getSoulHaloInfoBySlot(slot)

	equip := slotInfo.SoulHalo
	if nil == equip {
		return
	}

	s.GetOwner().TakeOnAppear(appeardef.AppearSoulHaloPos[slot], &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_SoulHalo,
		AppearId: equip.GetItemId(),
	}, isTip)
}

func (s *SoulHaloSys) appearCancel(slot uint32) {
	s.GetOwner().TakeOffAppear(appeardef.AppearSoulHaloPos[slot])
}

func (s *SoulHaloSys) afterTakeOn(slot uint32) {
	s.appearActive(slot, true)
	s.checkActiveSuit(slot, true)
	s.checkRefineSuit(true)
	s.ResetSysAttr(attrdef.SaSoulHalo)
}

func (s *SoulHaloSys) afterTakeOff(slot uint32, oldSoulHalo *pb3.ItemSt) {
	s.appearCancel(slot)
	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.checkRefineSuit(true)
	s.cancelActiveSuit(slot, oldSoulHalo.GetItemId())
}

func (s *SoulHaloSys) afterTakeOnBone(slot, _ uint32) {
	s.checkActiveSuit(slot, true)
	s.GetOwner().TriggerQuestEventRange(custom_id.QttSoulHalosTakeOnBone)
	s.ResetSysAttr(attrdef.SaSoulBone)
}

func (s *SoulHaloSys) afterTakeOffBone(slot, _ uint32, _ *pb3.ItemSt) {
	s.ResetSysAttr(attrdef.SaSoulBone)
	s.GetOwner().TriggerQuestEventRange(custom_id.QttSoulHalosTakeOnBone)
	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil != slotInfo && nil != slotInfo.SoulHalo {
		s.cancelActiveSuit(slot, slotInfo.SoulHalo.GetItemId())
	}
}

func (s *SoulHaloSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_67_1
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	return s.takeOn(req.Hdl, req.Solt)
}

func (s *SoulHaloSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_67_2
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	return s.takeOff(req.Solt)
}

func (s *SoulHaloSys) checkTakeOnBoneSlotHandle(equip *pb3.ItemSt, slot, pos uint32) (bool, error) {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return false, neterror.ConfNotFoundError("item itemConf(%d) nil", equip.GetItemId())
	}

	if !itemdef.IsSoulBone(itemConf.Type, itemConf.SubType) {
		return false, neterror.SysNotExistError("not soul bone item")
	}

	if itemConf.SubType != pos {
		return false, neterror.ParamsInvalidError("soul bone take pos is not equal")
	}

	slotConf := jsondata.GetSoulHaloSlotConf(slot)
	if nil == slotConf {
		return false, neterror.ParamsInvalidError("soul halo slot(%d) conf is nil", slot)
	}

	if slotConf.Quality != 0 && itemConf.Stage != slotConf.Quality {
		return false, neterror.ParamsInvalidError("soul halo quality not match")
	}

	_, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, neterror.SysNotExistError("bag sys get err")
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}

	return true, nil
}

func (s *SoulHaloSys) takeOnBone(hdl uint64, slot, pos uint32) error {
	if !s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is not unlock", slot)
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	equip := bagSys.FindItemByHandle(hdl)
	if nil == equip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	ok, err := s.checkTakeOnBoneSlotHandle(equip, slot, pos)
	if !ok {
		return err
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if _, ok := slotInfo.SoulBone[pos]; ok {
		if err := s.takeOffBone(slot, pos); err != nil {
			return err
		}
	}

	if removeSucc := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogSoulBoneTakeOn); !removeSucc {
		return neterror.InternalError("remove soul halo hdl:%d item:%d failed", equip.GetHandle(), equip.GetItemId())
	}

	equip.Bind = true
	slotInfo.SoulBone[pos] = equip
	s.updateSoulBoneInSlot(slot, pos, UpdateSoulBoneInSlotTakeOn)

	s.afterTakeOnBone(slot, pos)

	return nil
}

func (s *SoulHaloSys) takeOffBone(slot, pos uint32) error {
	if !s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is not unlock", slot)
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)

	oldEquip := slotInfo.SoulBone[pos]
	if nil == oldEquip {
		return neterror.ParamsInvalidError("soul halo slot conf is nil")
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	if succ := bagSys.AddItemPtr(oldEquip, true, pb3.LogId_LogSoulBoneTakeOff); !succ {
		return neterror.InternalError("soul halo take off(%d) err", slot)
	}

	delete(slotInfo.SoulBone, pos)
	s.SendProto3(67, 4, &pb3.S2C_67_4{
		Solt: slot,
		Pos:  pos,
	})

	s.afterTakeOffBone(slot, pos, oldEquip)

	return nil
}

func (s *SoulHaloSys) c2sTakeOnBone(msg *base.Message) error {
	var req pb3.C2S_67_3
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	return s.takeOnBone(req.Hdl, req.Solt, req.Pos)
}

func (s *SoulHaloSys) c2sTakeOffBone(msg *base.Message) error {
	var req pb3.C2S_67_4
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	return s.takeOffBone(req.Solt, req.Pos)
}

func (s *SoulHaloSys) getSoulHaloLv(equip *pb3.ItemSt) uint32 {
	if nil == equip {
		return 0
	}
	return equip.Union1
}

func (s *SoulHaloSys) setSoulHaloLv(equip *pb3.ItemSt, lv uint32) {
	if nil == equip {
		return
	}
	equip.Union1 = lv
}

func (s *SoulHaloSys) getSoulHaloBreakLv(equip *pb3.ItemSt) uint32 {
	if nil == equip {
		return 0
	}
	return equip.Union2
}

func (s *SoulHaloSys) setSoulHaloBreakLv(equip *pb3.ItemSt, lv uint32) {
	if nil == equip {
		return
	}
	equip.Union2 = lv
}

func (s *SoulHaloSys) getSoulBoneStage(equip *pb3.ItemSt) uint32 {
	if nil == equip {
		return 0
	}
	return equip.Union1
}

func (s *SoulHaloSys) setSoulBoneStage(equip *pb3.ItemSt, stage uint32) {
	if nil == equip {
		return
	}
	equip.Union1 = stage
}

func (s *SoulHaloSys) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_67_5
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	slot := req.Solt

	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil == slotInfo {
		return neterror.ParamsInvalidError("soul halo slot(%d) is nil", slot)
	}

	equip := slotInfo.SoulHalo
	if nil == equip {
		return neterror.ParamsInvalidError("soul halo slot(%d) is empty", slot)
	}

	conf := jsondata.GetSoulHaloConfByItemId(equip.GetItemId())
	if nil == conf {
		return neterror.ParamsInvalidError("soul halo conf is nil")
	}

	lv := s.getSoulHaloLv(equip)
	ntLv := lv + 1

	if lv >= uint32(len(conf.LvConf)) {
		return neterror.ConfNotFoundError("soul halo lv conf(%d) is nil", lv)
	}

	lvConf := conf.LvConf[ntLv-1]
	if lvConf.BreakLv > s.getSoulHaloBreakLv(equip) {
		return neterror.ConfNotFoundError("soul halo break lv is not enough")
	}

	if !s.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.setSoulHaloLv(equip, ntLv)
	s.updateSoulHaloInSlot(slot, UpdateSoulHaloInSlotLevelUp)
	s.checkRefineSuit(true)
	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.owner.TriggerQuestEvent(custom_id.QttAchievementsSoulHalosLevelUp, 0, 1)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSoulHaloLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"level":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *SoulHaloSys) c2sBreak(msg *base.Message) error {
	var req pb3.C2S_67_6
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	slot := req.Solt

	slotInfo := s.getSoulHaloInfoBySlot(slot)
	if nil == slotInfo {
		return neterror.ParamsInvalidError("soul halo slot(%d) is nil", slot)
	}

	equip := slotInfo.SoulHalo
	if nil == equip {
		return neterror.ParamsInvalidError("soul halo slot(%d) is empty", slot)
	}

	conf := jsondata.GetSoulHaloConfByItemId(equip.GetItemId())
	if nil == conf {
		return neterror.ParamsInvalidError("soul halo conf is nil")
	}

	breakLv := s.getSoulHaloBreakLv(equip)
	ntBreakLv := breakLv + 1
	if ntBreakLv >= uint32(len(conf.BreakConf)) {
		return neterror.ConfNotFoundError("soul halo break conf(%d) is nil", ntBreakLv)
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	breakConf := conf.BreakConf[ntBreakLv]

	var backAwardVec []jsondata.StdRewardVec
	itemHandle := pie.Uint64s(req.SwallowList).Unique()
	for _, hdl := range itemHandle {
		item := bagSys.FindItemByHandle(hdl)
		if nil == item {
			return neterror.ParamsInvalidError("not found item(%d) in bag", hdl)
		}
		if !utils.SliceContainsUint32(breakConf.SwallowItem, item.GetItemId()) {
			return neterror.ParamsInvalidError("item(%d) is not in soul halo break swallow item", hdl)
		}
		if refineItemConf := jsondata.GetSoulHaloRefineConfByItemId(item.GetItemId()); nil != refineItemConf {
			times := s.getSoulHaloExtData(item).RefineTimes
			if times > 0 {
				backAwardVec = append(backAwardVec, refineItemConf.GetRefineBackAwards(times))
			}
		}
	}

	if uint32(len(itemHandle)) != breakConf.SwallowCount {
		return neterror.ParamsInvalidError("soul halo break swallow item not equal")
	}

	if !s.owner.ConsumeByConf(breakConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloBreakUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	for _, hdl := range itemHandle {
		if !bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogSoulHaloBreakUp) {
			return neterror.InternalError("扣除道具失败 hdl:%d,扣除列表:%v", hdl, itemHandle)
		}
	}

	if itemConf := jsondata.GetItemConfig(equip.GetItemId()); nil != itemConf {
		engine.BroadcastTipMsgById(tipmsgid.SoulHaloBreakSuccess, s.owner.GetName(), itemConf.Name, ntBreakLv)
	}

	backAwards := jsondata.MergeStdReward(backAwardVec...)
	if len(backAwards) > 0 {
		engine.GiveRewards(s.owner, backAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSoulHaloRefineBackAward,
		})
	}

	s.setSoulHaloBreakLv(equip, ntBreakLv)
	s.updateSoulHaloInSlot(slot, UpdateSoulHaloInSlotBreak)
	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.GetOwner().TriggerQuestEvent(custom_id.QttAchievementsSoulHalosBreak, 0, 1)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSoulHaloBreakUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId":  equip.GetItemId(),
			"handle":  equip.GetHandle(),
			"breakLv": equip.GetUnion2(),
		}),
	})
	return nil
}

func (s *SoulHaloSys) c2sBoneStageUp(msg *base.Message) error {
	var req pb3.C2S_67_8
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	slot := req.Solt
	pos := req.Pos

	if !s.isSlotLock(slot) {
		return neterror.ParamsInvalidError("soul halo slot(%d) is not unlock", slot)
	}

	slotInfo := s.getSoulHaloInfoBySlot(slot)

	equip := slotInfo.SoulBone[pos]
	if nil == equip {
		return neterror.ParamsInvalidError("soul bone slot conf is nil")
	}

	conf := jsondata.GetSoulBoneConfByItemId(equip.GetItemId())
	if nil == conf {
		return neterror.ParamsInvalidError("soul bone conf is nil")
	}

	stage := s.getSoulBoneStage(equip)
	ntStage := stage + 1

	stageConf := jsondata.GetSoulBoneStageConf(equip.GetItemId(), ntStage)
	if nil == stageConf {
		return neterror.ConfNotFoundError("soul bone stage conf(%d) is nil", ntStage)
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	itemHandle := pie.Uint64s(req.SwallowList).Unique()
	for _, hdl := range itemHandle {
		item := bagSys.FindItemByHandle(hdl)
		if nil == item {
			return neterror.ParamsInvalidError("not found item(%d) in bag", hdl)
		}
		if !utils.SliceContainsUint32(stageConf.SwallowItem, item.GetItemId()) {
			return neterror.ParamsInvalidError("item(%d) is not in soul halo break swallow item", hdl)
		}
	}

	if uint32(len(itemHandle)) != stageConf.SwallowCount {
		return neterror.ParamsInvalidError("soul halo break swallow item not equal")
	}

	if !s.owner.ConsumeByConf(stageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogId_LogSoulBoneStageUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	for _, hdl := range itemHandle {
		if !bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogId_LogSoulBoneStageUp) {
			return neterror.InternalError("扣除道具失败 hdl:%d,扣除列表:%v", hdl, itemHandle)
		}
	}

	if itemConf := jsondata.GetItemConfig(equip.GetItemId()); nil != itemConf {
		engine.BroadcastTipMsgById(tipmsgid.SoulHaloBreakSuccess, s.owner.GetName(), itemConf.Name, ntStage)
	}

	s.setSoulBoneStage(equip, ntStage)
	s.updateSoulBoneInSlot(slot, pos, UpdateSoulBoneInSlotStageUp)
	s.ResetSysAttr(attrdef.SaSoulBone)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogId_LogSoulBoneStageUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"stage":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *SoulHaloSys) calSoulHaloRecycleRewards(itemSt *pb3.ItemSt) jsondata.StdRewardVec {
	conf := jsondata.GetSoulHaloConfByItemId(itemSt.GetItemId())
	var rewardsVec []jsondata.StdRewardVec

	lv := s.getSoulHaloLv(itemSt)
	if lv > 0 && lv <= uint32(len(conf.LvConf)) && len(conf.LvConf[lv-1].RecycleRewards) > 0 {
		rewardsVec = append(rewardsVec, conf.LvConf[lv-1].RecycleRewards)
	}

	breakLv := s.getSoulHaloBreakLv(itemSt)
	if breakLv < uint32(len(conf.BreakConf)) && len(conf.BreakConf[breakLv].RecycleRewards) > 0 {
		rewardsVec = append(rewardsVec, conf.BreakConf[breakLv].RecycleRewards)
	}

	if refineItemConf := jsondata.GetSoulHaloRefineConfByItemId(itemSt.GetItemId()); nil != refineItemConf {
		times := s.getSoulHaloExtData(itemSt).RefineTimes
		if times > 0 {
			rewardsVec = append(rewardsVec, refineItemConf.GetRefineBackAwards(times))
		}
	}

	rewards := jsondata.MergeStdReward(rewardsVec...)

	return rewards
}

func (s *SoulHaloSys) calSoulBoneRecycleRewards(soulBone *pb3.ItemSt) jsondata.StdRewardVec {
	stageConf := jsondata.GetSoulBoneStageConf(soulBone.GetItemId(), s.getSoulBoneStage(soulBone))
	if nil == stageConf || len(stageConf.RecycleRewards) == 0 {
		return nil
	}

	var rewardsVec []jsondata.StdRewardVec

	rewardsVec = append(rewardsVec, stageConf.RecycleRewards)

	rewards := jsondata.MergeStdReward(rewardsVec...)

	return rewards
}

func (s *SoulHaloSys) getSuitMaxQuality() uint32 {
	data := s.data()
	var quality uint32
	for _, slotInfo := range data.SoltInfo {
		if nil == slotInfo.SoulHalo {
			continue
		}
		itemConf := jsondata.GetItemConfig(slotInfo.SoulHalo.GetItemId())
		if nil == itemConf {
			continue
		}
		if len(slotInfo.SoulBone) < itemdef.SoulBonePosEnd {
			continue
		}
		thisQuality := itemConf.Stage
		for _, bone := range slotInfo.SoulBone {
			boneItemConf := jsondata.GetItemConfig(bone.GetItemId())
			thisQuality = utils.MinUInt32(boneItemConf.Stage, thisQuality)
		}
		quality = utils.MaxUInt32(thisQuality, quality)
	}
	return quality
}

func (s *SoulHaloSys) calcAttrSoulHalo(calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetSoulHaloConf()
	suitInfo := make(map[uint32]map[uint32]uint32)
	for _, slotConf := range conf.SlotConf {
		slot := slotConf.SlotId
		slotInfo := s.getSoulHaloInfoBySlot(slot)
		if nil == slotInfo {
			continue
		}
		soulHalo := slotInfo.SoulHalo
		if nil == soulHalo {
			continue
		}

		soulHaloConf := jsondata.GetSoulHaloConfByItemId(soulHalo.GetItemId())
		//魂环基础属性
		itemConf := jsondata.GetItemConfig(soulHalo.GetItemId())
		if nil == itemConf {
			s.LogError("soul halo item conf(%d) is nil", soulHalo.GetItemId())
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)

		//魂环等级属性
		lv := s.getSoulHaloLv(soulHalo)
		if lv > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, soulHaloConf.LvConf[lv-1].Attrs)
		}

		//魂环等级特性属性
		for _, featureConf := range soulHaloConf.SoulHaloLvFeature {
			if featureConf.Level > lv {
				continue
			}
			engine.CheckAddAttrsToCalc(s.owner, calc, featureConf.Attrs)
		}

		//魂环突破属性
		if len(soulHaloConf.BreakConf) > 0 {
			breakLv := s.getSoulHaloBreakLv(soulHalo)
			engine.CheckAddAttrsToCalc(s.owner, calc, soulHaloConf.BreakConf[breakLv].Attrs)
		}

		ext := s.getSoulHaloExtData(soulHalo)

		for groupId, entry := range ext.RefineData {
			if nil == entry {
				continue
			}
			refineConf := jsondata.GetSoulHaloRefineConfByItemId(soulHalo.GetItemId())
			if nil == refineConf {
				continue
			}
			groupConf, ok := refineConf.RefineGroup[groupId]
			if !ok {
				continue
			}
			if groupConf.ActiveSoulHaloLv > lv {
				continue
			}
			//魂环洗练追加属性
			if entry.Type == custom_id.SoulHaloRefineEntryTypeRand {
				engine.CheckAddAttrsToCalc(s.owner, calc, jsondata.AttrVec{{Type: entry.AttrType, Value: entry.AttrVal}})
			} else if entry.Type == custom_id.SoulHaloRefineEntryTypeSuit {
				if _, ok := suitInfo[entry.SuitId]; !ok {
					suitInfo[entry.SuitId] = make(map[uint32]uint32)
				}
				for star := uint32(1); star <= entry.SuitStar; star++ {
					suitInfo[entry.SuitId][star]++
				}
			}
		}

	}
	//魂环洗练套装属性
	suitConfList := jsondata.GetSoulHaloRefineConf().SuitConf
	for suitId, suitConf := range suitConfList {
		if _, ok := suitInfo[suitId]; !ok {
			continue
		}
		var maxStar uint32
		for _, starConf := range suitConf {
			if suitInfo[suitId][starConf.SuitStar] < starConf.ActiveNum {
				continue
			}
			if maxStar < starConf.SuitStar {
				maxStar = starConf.SuitStar
			}
		}
		if maxStar > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, suitConfList[suitId][maxStar].Attrs)
		}
	}
}

func (s *SoulHaloSys) calcAttrSoulHaloAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetSoulHaloConf()
	addRate := totalSysCalc.GetValue(attrdef.SoulHaloBaseAttrRate)
	if addRate == 0 {
		return
	}
	for _, slotConf := range conf.SlotConf {
		slot := slotConf.SlotId
		slotInfo := s.getSoulHaloInfoBySlot(slot)
		if nil == slotInfo {
			continue
		}
		soulHalo := slotInfo.SoulHalo
		if nil == soulHalo {
			continue
		}
		soulHaloConf := jsondata.GetSoulHaloConfByItemId(soulHalo.GetItemId())
		//魂环等级属性
		lv := s.getSoulHaloLv(soulHalo)
		if lv > 0 {
			if addRate > 0 {
				engine.CheckAddAttrsRateRoundingUp(s.owner, calc, soulHaloConf.LvConf[lv-1].Attrs, uint32(addRate))
			}
		}
	}
}

func (s *SoulHaloSys) calcAttrSoulBone(calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetSoulHaloConf()

	for _, slotConf := range conf.SlotConf {
		slot := slotConf.SlotId
		slotInfo := s.getSoulHaloInfoBySlot(slot)
		if nil == slotInfo {
			continue
		}
		//魂骨基础属性
		for _, soulBone := range slotInfo.SoulBone {
			itemConf := jsondata.GetItemConfig(soulBone.GetItemId())
			if nil == itemConf {
				continue
			}
			engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)

			if stageConf := jsondata.GetSoulBoneStageConf(soulBone.GetItemId(), s.getSoulBoneStage(soulBone)); nil != stageConf {
				engine.CheckAddAttrsToCalc(s.owner, calc, stageConf.Attrs)
			}
		}
	}
}

func (s *SoulHaloSys) getSoulHaloExtData(itemSt *pb3.ItemSt) *pb3.ItemExtSoulHalo {
	if nil == itemSt.Ext {
		itemSt.Ext = &pb3.ItemExt{}
	}
	if nil == itemSt.Ext.SoulHalo {
		itemSt.Ext.SoulHalo = &pb3.ItemExtSoulHalo{}
	}
	ext := itemSt.Ext.SoulHalo
	if nil == ext.RefineData {
		ext.RefineData = make(map[uint32]*pb3.SoulHaloRefineSt)
	}
	if nil == ext.WaitSaveData {
		ext.WaitSaveData = make(map[uint32]*pb3.SoulHaloRefineSt)
	}
	return ext
}

func (s *SoulHaloSys) refineSoulHalo(soulHalo *pb3.ItemSt, skipIds []uint32) error {
	conf := jsondata.GetSoulHaloRefineConfByItemId(soulHalo.GetItemId())
	if nil == conf {
		return neterror.InternalError("soul halo %d refine conf is nil", soulHalo.GetItemId())
	}

	var (
		soulHaloExt = s.getSoulHaloExtData(soulHalo)
		soulHaloLv  = s.getSoulHaloLv(soulHalo)
		isRefine    = soulHaloExt.IsIdentify
	)

	if isRefine && !conf.CanRefine {
		return neterror.ParamsInvalidError("soul halo %d not allow refine", soulHalo.GetItemId())
	}

	if !soulHaloExt.IsIdentify {
		soulHaloExt.IsIdentify = true
	}

	waitSaveData := make(map[uint32]*pb3.SoulHaloRefineSt)
	for k, v := range soulHaloExt.RefineData {
		if nil == v {
			continue
		}
		waitSaveData[k] = &pb3.SoulHaloRefineSt{
			GroupId:  v.GroupId,
			BindId:   v.BindId,
			Type:     v.Type,
			AttrType: v.AttrType,
			AttrVal:  v.AttrVal,
			SuitId:   v.SuitId,
			SuitStar: v.SuitStar,
			Quality:  v.Quality,
		}
	}

	for groupId, groupConf := range conf.RefineGroup {
		if pie.Uint32s(skipIds).Contains(groupId) {
			continue
		}
		if groupConf.IsFix && isRefine { //特定属性只初始化1次
			continue
		}

		delete(waitSaveData, groupId) //非特定和锁定属性都会被洗掉

		if groupConf.EnterSoulHaloLv > soulHaloLv {
			continue
		}

		rate := utils.Ternary(isRefine, groupConf.RefineRate, groupConf.InitRate).(uint32)
		if !random.Hit(rate, 10000) {
			continue
		}

		if entry := s.refineRandomOne(groupId, groupConf, custom_id.SoulHaloRefineCond{
			RefineTimes: soulHaloExt.RefineTimes,
			SoulHaloLv:  soulHaloLv,
			IsRefine:    isRefine,
		}); nil != entry {
			waitSaveData[groupId] = entry
		}
	}

	if !isRefine {
		soulHaloExt.RefineData = waitSaveData
	} else {
		soulHaloExt.WaitSaveData = waitSaveData
	}

	return nil
}

func (s *SoulHaloSys) refineRandomOne(groupId uint32, groupConf *jsondata.SoulHaloRefineGroupConf, cond custom_id.SoulHaloRefineCond) *pb3.SoulHaloRefineSt {
	pool := new(random.Pool)
	for _, libConf := range groupConf.Libs {
		if cond.SoulHaloLv < libConf.SoulHaloLv {
			continue
		}
		if cond.RefineTimes < libConf.RefineMinTimes || cond.RefineTimes > libConf.RefineMaxTimes {
			continue
		}
		rate := utils.Ternary(cond.IsRefine, libConf.RefineWeight, libConf.InitWeight).(uint32)
		pool.AddItem(libConf, rate)
	}
	if pool.Size() == 0 {
		return nil
	}
	randConf := pool.RandomOne().(*jsondata.SoulHaloRefineLibsConf)
	refineEntry := &pb3.SoulHaloRefineSt{
		GroupId:  groupId,
		BindId:   randConf.ID,
		Type:     randConf.Type,
		AttrType: randConf.AttrType,
		AttrVal:  randConf.GetAttrVal(),
		SuitId:   randConf.SuitId,
		SuitStar: randConf.SuitStar,
		Quality:  randConf.Quality,
	}
	return refineEntry
}

func (s *SoulHaloSys) initSoulHalo(itemSt *pb3.ItemSt) {
	if !itemdef.IsSoulHalo(jsondata.GetItemType(itemSt.GetItemId())) {
		return
	}
	soulHaloExt := s.getSoulHaloExtData(itemSt)
	if soulHaloExt.IsInit {
		return
	}

	if soulHaloExt.IsIdentify {
		soulHaloExt.IsInit = true
		s.LogError("item %d refined but not init before", itemSt.GetItemId())
		return
	}

	conf := jsondata.GetSoulHaloRefineConfByItemId(itemSt.GetItemId())
	if nil == conf {
		s.LogError("soul halo %d refine conf is nil", itemSt.GetItemId())
		return
	}

	soulHaloExt.IsInit = true

	if soulHaloExt.IsIdentify {
		return
	}

	if conf.NeedIdentify {
		return
	}

	err := s.refineSoulHalo(itemSt, nil)
	if nil != err {
		s.LogError("err:%v", err)
		return
	}
}

func (s *SoulHaloSys) isRefineFuncOpen() bool {
	conf := jsondata.GetSoulHaloRefineConf()
	if nil == conf {
		return false
	}
	return conf.RefineOpenDay <= gshare.GetOpenServerDay()
}

func (s *SoulHaloSys) getRefineScore(v *pb3.SoulHaloRefineSt) int64 {
	switch v.Type {
	case custom_id.SoulHaloRefineEntryTypeRand:
		singleCalc := attrcalc.GetSingleCalc()
		defer func() {
			singleCalc.Reset()
		}()
		engine.CheckAddAttrsToCalc(s.owner, singleCalc, []*jsondata.Attr{
			{
				Type:  v.AttrType,
				Value: v.AttrVal,
			},
		})
		fightVal := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(s.owner.GetJob()))
		return fightVal
	case custom_id.SoulHaloRefineEntryTypeSuit:
		return int64(jsondata.GetSoulHaloSuitEntryFightValue(v.SuitId, v.SuitStar))
	}
	return 0
}

func (s *SoulHaloSys) c2sRefine(msg *base.Message) error {
	var req pb3.C2S_67_50
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.isRefineFuncOpen() {
		s.owner.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.ParamsInvalidError("bag sys not open")
	}

	slot := req.Solt

	var equip *pb3.ItemSt
	if slot > 0 {
		equip, err = s.getSoulHaloBySlot(slot)
		if nil != err {
			return err
		}
	} else {
		equip = bagSys.FindItemByHandle(req.GetHdl())
	}

	if nil == equip {
		return neterror.ParamsInvalidError("not found soul halo slot:%d,hdl:%d", req.GetSolt(), req.GetHdl())
	}

	refineItemConf := jsondata.GetSoulHaloRefineConfByItemId(equip.GetItemId())
	if nil == refineItemConf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	ext := s.getSoulHaloExtData(equip)

	if !req.IsSkip {
		fn := func(refineDara map[uint32]*pb3.SoulHaloRefineSt) (score int64) {
			for _, v := range refineDara {
				if v == nil {
					continue
				}
				score += s.getRefineScore(v)
			}
			return
		}
		refineScore, waitScore := fn(ext.RefineData), fn(ext.WaitSaveData)
		s.LogDebug("refineScore:%d,waitScore:%d", refineScore, waitScore)
		if refineScore > waitScore {
			return neterror.ParamsInvalidError("refine data score high and not skip")
		}
	}

	isIdentify := !ext.IsIdentify

	if isIdentify && len(req.LockIds) > 0 {
		return neterror.ParamsInvalidError("init not allow lock")
	}

	if !isIdentify && slot == 0 { //洗练的时候要在位置上
		return neterror.ParamsInvalidError("refine need in slot")
	}

	for _, lockId := range req.LockIds {
		groupConf, ok := refineItemConf.RefineGroup[lockId]
		if !ok {
			return neterror.ParamsInvalidError("lock conf %d is nil", lockId)
		}
		if groupConf.IsFix {
			return neterror.ParamsInvalidError("fix not allow lock %d", lockId)
		}
		if !groupConf.CanLock {
			return neterror.ParamsInvalidError("not allow lock %d", lockId)
		}
	}

	if !isIdentify {
		consumeVec, ok := refineItemConf.GetRefineConsume(uint32(len(req.LockIds)))
		if !ok {
			return neterror.ConfNotFoundError("consume get nil")
		}
		if !s.owner.ConsumeByConf(consumeVec, false, common.ConsumeParams{
			LogId: pb3.LogId_LogSoulHaloRefineConsume,
		}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		ext.RefineTimes++
	}

	err = s.refineSoulHalo(equip, req.LockIds)
	if nil != err {
		return err
	}

	if isIdentify {
		s.checkRefineSuit(true)
		s.ResetSysAttr(attrdef.SaSoulHalo)
	}

	if slot > 0 {
		s.updateSoulHaloInSlot(slot, UpdateSoulHaloInSlotRefine)
	} else {
		bagSys.OnItemChange(equip, 0, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSoulHaloRefineIdentify,
		})
	}

	if !isIdentify {
		s.owner.TriggerQuestEvent(custom_id.QttAchievementsSoulHalosRefine, 0, 1)
	}
	return nil
}

func (s *SoulHaloSys) c2sRefineSave(msg *base.Message) error {
	var req pb3.C2S_67_51
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.isRefineFuncOpen() {
		s.owner.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	slot := req.Solt

	equip, err := s.getSoulHaloBySlot(slot)
	if nil != err {
		return err
	}

	ext := s.getSoulHaloExtData(equip)
	if len(ext.WaitSaveData) == 0 {
		return neterror.ParamsInvalidError("wait save data nil")
	}

	ext.RefineData = ext.WaitSaveData
	ext.WaitSaveData = nil

	s.checkRefineSuit(true)
	s.updateSoulHaloInSlot(slot, UpdateSoulHaloInSlotRefineSave)
	s.ResetSysAttr(attrdef.SaSoulHalo)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSoulHaloRefineSave, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"refine": ext.GetRefineData(),
		}),
	})

	return nil
}

func (s *SoulHaloSys) checkRefineSuit(sendTips bool) {
	conf := jsondata.GetSoulHaloConf()
	suitInfo := make(map[uint32]map[uint32]uint32)
	for _, slotConf := range conf.SlotConf {
		slot := slotConf.SlotId
		soulHalo, _ := s.getSoulHaloBySlot(slot)
		if nil == soulHalo {
			continue
		}
		lv := s.getSoulHaloLv(soulHalo)
		ext := s.getSoulHaloExtData(soulHalo)
		for groupId, entry := range ext.RefineData {
			if nil == entry {
				continue
			}
			refineConf := jsondata.GetSoulHaloRefineConfByItemId(soulHalo.GetItemId())
			if nil == refineConf {
				continue
			}
			groupConf, ok := refineConf.RefineGroup[groupId]
			if !ok {
				continue
			}
			if groupConf.ActiveSoulHaloLv > lv {
				continue
			}
			if entry.Type != custom_id.SoulHaloRefineEntryTypeSuit {
				continue
			}
			if _, ok := suitInfo[entry.SuitId]; !ok {
				suitInfo[entry.SuitId] = make(map[uint32]uint32)
			}
			for star := uint32(1); star <= entry.SuitStar; star++ {
				suitInfo[entry.SuitId][star]++
			}
		}
	}

	suitConfList := jsondata.GetSoulHaloRefineConf().SuitConf
	for suitId, suitConf := range suitConfList {
		var maxStar, skillId, skillLv uint32
		for _, starConf := range suitConf {
			skillId = starConf.SkillId
			if _, ok := suitInfo[suitId]; !ok {
				break
			}
			if suitInfo[suitId][starConf.SuitStar] < starConf.ActiveNum {
				continue
			}
			if maxStar < starConf.SuitStar {
				maxStar = starConf.SuitStar
				skillLv = starConf.SkillLv
			}
		}

		s.owner.ForgetSkill(skillId, true, sendTips, true)

		if maxStar > 0 {
			s.owner.LearnSkill(skillId, skillLv, sendTips)
		}
	}
}

func (s *SoulHaloSys) getSysPower() (power int64) {
	power += s.owner.GetAttrSys().GetSysPower(attrdef.SaSoulHalo)
	power += s.owner.GetAttrSys().GetSysPower(attrdef.SaSoulBone)

	return
}

func calcSoulHaloAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrSoulHalo(calc)
	s.calcAttrSoulHaloQi(calc)
}

func calcSoulHaloAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrSoulHaloAddRate(totalSysCalc, calc)
	s.calcAttrSoulHaloQiAddRate(totalSysCalc, calc)
}

func calcSoulBoneAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrSoulBone(calc)
}

func handleQttTakeOnXYearSoulRingNum(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return 0
	}
	if len(ids) != 1 {
		return 0
	}
	state := ids[0]
	var count uint32
	data := s.data()
	for _, info := range data.SoltInfo {
		if info == nil || info.SoulHalo == nil {
			continue
		}
		itemId := info.SoulHalo.ItemId
		conf := jsondata.GetItemConfig(itemId)
		if conf == nil {
			continue
		}
		if conf.Stage >= state {
			count++
		}
	}
	return count
}

func handleQttSoulHalosTakeOnBone(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !s.IsOpen() {
		return 0
	}
	if len(ids) != 1 {
		return 0
	}
	state := ids[0]
	var count uint32
	data := s.data()
	for _, info := range data.SoltInfo {
		if info == nil || info.SoulHalo == nil {
			continue
		}
		for _, st := range info.SoulBone {
			conf := jsondata.GetItemConfig(st.ItemId)
			if conf == nil {
				continue
			}
			if conf.Stage >= state {
				count++
			}
		}
	}
	return count
}

func soulHaloOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}

	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaSoulHalo, attrdef.SaSoulBone, attrdef.SaImmortalSoul}
	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}

	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeSoulHalo, player, sumPower, false, 0)
}

func soulHaloInit(player iface.IPlayer, args ...interface{}) {
	s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok {
		return
	}
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	itemSt, ok := args[0].(*pb3.ItemSt)
	if !ok {
		player.LogError("args not *pb.itemSt type")
		return
	}
	s.initSoulHalo(itemSt)
}

func handleJobChangeSoulHalo(player iface.IPlayer, job uint32) bool {
	sys, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !sys.IsOpen() {
		return true
	}
	sys.onJobChange(job)
	return true
}

func init() {
	RegisterSysClass(sysdef.SiSoulHalo, func() iface.ISystem {
		return &SoulHaloSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSoulHalo, calcSoulHaloAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaSoulHalo, calcSoulHaloAttrAddRate)
	engine.RegAttrCalcFn(attrdef.SaSoulBone, calcSoulBoneAttr)

	engine.RegQuestTargetProgress(custom_id.QttTakeOnXYearSoulRingNum, handleQttTakeOnXYearSoulRingNum)
	engine.RegQuestTargetProgress(custom_id.QttSoulHalosTakeOnBone, handleQttSoulHalosTakeOnBone)

	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, soulHaloOnUpdateSysPowerMap)
	event.RegActorEvent(custom_id.AeAddNewBagItem, soulHaloInit)

	net.RegisterSysProtoV2(67, 7, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sUnLock
	})

	net.RegisterSysProtoV2(67, 1, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(67, 2, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sTakeOff
	})

	net.RegisterSysProtoV2(67, 3, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sTakeOnBone
	})
	net.RegisterSysProtoV2(67, 4, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sTakeOffBone
	})

	net.RegisterSysProtoV2(67, 5, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sLevelUp
	})
	net.RegisterSysProtoV2(67, 6, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sBreak
	})

	net.RegisterSysProtoV2(67, 8, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sBoneStageUp
	})

	net.RegisterSysProtoV2(67, 50, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sRefine
	})
	net.RegisterSysProtoV2(67, 51, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sRefineSave
	})

	jobchange.RegJobChangeFunc(jobchange.SoulHalo, &jobchange.Fn{Fn: handleJobChangeSoulHalo})
}
