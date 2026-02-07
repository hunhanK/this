/**
 * @Author: LvYuMeng
 * @Date: 2025/7/28
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
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
	"sort"
)

const (
	SmithEquipRecycleCallTypeRandom = 1 // 随机概率
	SmithEquipRecycleCallTypeWeight = 2 // 权重
)

const (
	SmithEquipRecyclePYYId = 2830013 // 奇匠战令-分解
)

type SmithEquipSys struct {
	Base
}

func (s *SmithEquipSys) GetSysData() *pb3.SmithEquip {
	binary := s.GetBinaryData()

	if nil == binary.SmithEquipData {
		binary.SmithEquipData = &pb3.SmithEquipData{}
	}

	if nil == binary.SmithEquipData.SysData {
		binary.SmithEquipData.SysData = make(map[uint32]*pb3.SmithEquip)
	}

	sysId := s.GetSysId()
	sysData, ok := binary.SmithEquipData.SysData[sysId]
	if !ok {
		sysData = &pb3.SmithEquip{}
		binary.SmithEquipData.SysData[sysId] = sysData
	}

	if nil == sysData.TakeOnEquip {
		sysData.TakeOnEquip = make(map[uint32]*pb3.ItemSt)
	}

	if nil == sysData.ActiveSuit {
		sysData.ActiveSuit = make(map[uint64]uint32)
	}

	if nil == sysData.DailyRecycleNum {
		sysData.DailyRecycleNum = make(map[uint32]int64)
	}

	return sysData
}

func (s *SmithEquipSys) OnLogin() {
	s.checkSuit()
}

func (s *SmithEquipSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SmithEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SmithEquipSys) s2cInfo() {
	s.SendProto3(80, 11, &pb3.S2C_80_11{
		SysId: s.GetSysId(),
		Data:  s.GetSysData(),
	})
}

func (s *SmithEquipSys) getSmithEquipBySlot(slot uint32) *pb3.ItemSt {
	data := s.GetSysData()
	v, ok := data.TakeOnEquip[slot]
	if !ok {
		return nil
	}
	return v
}

func (s *SmithEquipSys) checkTakeOnSlotHandle(equip *pb3.ItemSt, slot uint32) (bool, error) {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return false, neterror.ConfNotFoundError("item itemConf(%d) nil", equip.GetItemId())
	}

	if !itemdef.IsSmithEquipItem(itemConf.Type) {
		return false, neterror.SysNotExistError("not SmithEquip item")
	}

	if itemConf.SubType != slot {
		return false, neterror.ParamsInvalidError("smith equip take pos is not equal")
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}

	if nil == jsondata.GetSmithEquipConf(s.GetSysId()).GetSlotConf(slot) {
		return false, neterror.ParamsInvalidError("SmithEquip(%d) conf is nil", slot)
	}

	return true, nil
}

func (s *SmithEquipSys) GetRef() (*gshare.SmithRef, error) {
	ref, ok := gshare.SmithInstance.FindSmithRefByEquipSysId(s.GetSysId())
	if !ok {
		return nil, neterror.InternalError("ref not found %d", s.GetSysId())
	}

	return ref, nil
}

func (s *SmithEquipSys) GetBagSys() (*SmithBagSys, error) {
	ref, err := s.GetRef()
	if nil != err {
		return nil, err
	}

	bagSys, ok := s.owner.GetSysObj(ref.BagSysId).(*SmithBagSys)
	if !ok {
		return nil, neterror.SysNotExistError("SmithBagSys get err")
	}

	return bagSys, nil
}

func (s *SmithEquipSys) takeOn(slot uint32, hdl uint64) error {
	bagSys, err := s.GetBagSys()
	if nil != err {
		return err
	}

	newEquip := bagSys.FindItemByHandle(hdl)
	if nil == newEquip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	ok, err := s.checkTakeOnSlotHandle(newEquip, slot)
	if !ok {
		return err
	}

	oldEquip := s.getSmithEquipBySlot(slot)
	if nil != oldEquip {
		if err := s.takeOff(slot, pb3.LogId_LogSmithEquipTakeRepkace, false); err != nil {
			return err
		}
	}

	if removeSucc := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogSmithEquipTakeOn); !removeSucc {
		return neterror.InternalError("remove SmithEquip hdl:%d item:%d failed", newEquip.GetHandle(), newEquip.GetItemId())
	}

	s.GetSysData().TakeOnEquip[slot] = newEquip

	s.SendProto3(80, 12, &pb3.S2C_80_12{
		SysId: s.GetSysId(),
		Pos:   slot,
		Item:  newEquip,
	})

	s.afterTakeOn(slot)

	return nil
}

func (s *SmithEquipSys) afterTakeOn(slot uint32) {
	s.checkSuit()
	s.reCalAttr()
}

func (s *SmithEquipSys) reCalAttr() {
	ref, err := s.GetRef()
	if nil != err {
		return
	}
	s.ResetSysAttr(ref.CalAttrSmithEqDef)
}

func (s *SmithEquipSys) checkSuit() {
	conf := jsondata.GetSmithEquipConf(s.GetSysId())
	if nil == conf {
		return
	}

	sysData := s.GetSysData()
	change := false

	type SortSt struct {
		Stage, Quality uint32
	}

	for suitType, suitConf := range conf.Suits {
		var sortSt []*SortSt
		for _, slot := range suitConf.Pos {
			eq, ok := sysData.TakeOnEquip[slot]
			if !ok {
				continue
			}

			itemConf := jsondata.GetItemConfig(eq.GetItemId())
			if nil == itemConf {
				continue
			}
			sortSt = append(sortSt, &SortSt{Stage: itemConf.Stage, Quality: itemConf.Quality})
		}

		sort.Slice(sortSt, func(i, j int) bool {
			if sortSt[i].Stage == sortSt[j].Stage {
				return sortSt[i].Quality > sortSt[j].Quality
			}
			return sortSt[i].Stage > sortSt[j].Stage
		})

		for nums, stageConf := range suitConf.SuitNums {
			if nums <= 0 || nums > uint32(len(sortSt)) {
				continue
			}

			sortL := uint32(len(sortSt))

			var maxStage uint32

			for _, v := range stageConf.Suits {
				var stage uint32
				for i := nums - 1; i < sortL; i++ {
					if sortSt[i].Quality >= v.QualityLimit {
						stage = sortSt[i].Stage
						break
					}
				}

				if stage == 0 {
					continue
				}

				if v.Stage > stage || maxStage > v.Stage {
					continue
				}

				maxStage = v.Stage
			}

			key := utils.Make64(nums, suitType)

			if sysData.ActiveSuit[key] < maxStage {
				sysData.ActiveSuit[key] = maxStage
				change = true
			}
		}
	}

	if change {
		s.SendProto3(80, 14, &pb3.S2C_80_14{
			SysId:      s.GetSysId(),
			ActiveSuit: sysData.GetActiveSuit(),
		})

		var skillLv uint32
		for key, stage := range sysData.ActiveSuit {
			eqType, nums := utils.High32(key), utils.Low32(key)
			if suitConf := conf.GetSuitConf(eqType, nums, stage); nil != suitConf {
				if suitConf.SkillLv > skillLv {
					skillLv = suitConf.SkillLv
				}
			}
		}
		if skillLv > 0 {
			s.owner.LearnSkill(conf.SuitSkillId, skillLv, true)
		}
	}
}

func (s *SmithEquipSys) takeOff(slot uint32, logId pb3.LogId, isSend bool) error {
	oldItem := s.getSmithEquipBySlot(slot)
	if nil == oldItem {
		return neterror.ParamsInvalidError("SmithEquip is nil")
	}

	bagSys, err := s.GetBagSys()
	if nil != err {
		return err
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	if succ := bagSys.AddItemPtr(oldItem, true, logId); !succ {
		return neterror.InternalError("SmithEquip take off(%d) err", slot)
	}

	delete(s.GetSysData().TakeOnEquip, slot)

	if isSend {
		s.SendProto3(80, 13, &pb3.S2C_80_13{
			SysId: s.GetSysId(),
			Pos:   slot,
		})
	}

	s.afterTakeOff(slot)

	return nil
}

func (s *SmithEquipSys) afterTakeOff(slot uint32) {
	s.reCalAttr()
}

func (s *SmithEquipSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetSmithEquipConf(s.GetSysId())
	if nil == conf {
		return
	}

	sysData := s.GetSysData()

	for slot, equip := range sysData.TakeOnEquip {
		itemConf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.PremiumAttrs)

		if slotLvConf := conf.GetSlotLvConf(slot, equip.GetUnion1()); nil != slotLvConf {
			engine.CheckAddAttrsToCalc(s.owner, calc, slotLvConf.Attrs)
		}

		for _, attr := range equip.Attrs {
			calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
		}
	}

	for key, stage := range sysData.ActiveSuit {
		eqType, nums := utils.High32(key), utils.Low32(key)
		if suitConf := conf.GetSuitConf(eqType, nums, stage); nil != suitConf {
			engine.CheckAddAttrsToCalc(s.owner, calc, suitConf.Attrs)
		}
	}
}

func c2sTakeOn(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_12
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys, ok := GetSmithEquipSys(player, req.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("sys %d not found", req.GetSysId())
	}

	err = sys.takeOn(req.GetPos(), req.GetHandle())
	if err != nil {
		return err
	}

	return nil
}

func c2sTakeOff(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_80_13
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	sys, ok := GetSmithEquipSys(player, req.GetSysId())
	if !ok {
		return neterror.ParamsInvalidError("sys %d not found", req.GetSysId())
	}
	err = sys.takeOff(req.GetPos(), pb3.LogId_LogSmithEquipTakeOff, true)
	if err != nil {
		return err
	}
	return nil
}

func (s *SmithEquipSys) calRecycleRewards(_ *pb3.ItemSt) jsondata.StdRewardVec {
	sysId := s.GetSysId()
	equipSysConf := jsondata.GetSmithEquipConf(sysId)
	if equipSysConf == nil {
		return nil
	}
	// 装备等级
	var equipLv uint32
	stdRewardVec := s.getRecycleAwards(equipLv, equipSysConf.Recycle)
	owner := s.GetOwner()
	total, _ := owner.GetPrivilege(privilegedef.EnumSmithDecompositionDropItemRate)
	if random.Hit(uint32(total), 10000) {
		awards := s.getRecycleAwards(equipLv, equipSysConf.RecycleExtra)
		if len(awards) > 0 {
			stdRewardVec = append(stdRewardVec, awards...)
		}
	}
	return stdRewardVec
}

func (s *SmithEquipSys) getRecycleAwards(equipLv uint32, equipSysConf []*jsondata.SmithEquipRecycle) jsondata.StdRewardVec {
	var recycleConf *jsondata.SmithEquipRecycle
	for _, recycle := range equipSysConf {
		if recycle.MaxLevel > equipLv {
			recycleConf = recycle
		}
	}
	if recycleConf == nil {
		return nil
	}
	var awards jsondata.StdRewardVec
	switch recycleConf.CalType {
	case SmithEquipRecycleCallTypeRandom:
		for _, entry := range recycleConf.RecycleEntry {
			recycleEntryConf := entry
			if !random.Hit(recycleEntryConf.Weight, 10000) {
				continue
			}
			awards = append(awards, recycleEntryConf.Rewards...)
		}
	case SmithEquipRecycleCallTypeWeight:
		var randomPool = new(random.Pool)
		for _, entry := range recycleConf.RecycleEntry {
			recycleEntryConf := entry
			randomPool.AddItem(recycleEntryConf, recycleEntryConf.Weight)
		}
		if randomPool.Size() == 0 {
			return nil
		}
		randomOne := randomPool.RandomOne().(*jsondata.SmithEquipRecycleEntry)
		awards = append(awards, randomOne.Rewards...)
		randomPool.Clear()
	}
	return awards
}

func (s *SmithEquipSys) calcUpLimitCount(itemId uint32, count int64) int64 {
	sysId := s.GetSysId()
	owner := s.GetOwner()
	equipSysConf := jsondata.GetSmithEquipConf(sysId)
	if equipSysConf == nil || equipSysConf.RecycleLimitItem == nil {
		return count
	}
	upItem, ok := equipSysConf.RecycleLimitItem[itemId]
	if !ok {
		return count
	}
	upLimit := upItem.Count
	data := s.GetSysData()
	dailyNum := data.DailyRecycleNum[itemId]
	obj := owner.GetSysObj(sysdef.SiSmithPrivilege)
	if obj != nil && obj.IsOpen() {
		sys, ok := obj.(*SmithPrivilegeSys)
		if ok {
			upLimit += sys.GetRecycleExtCount(itemId)
		}
	}
	dailyUpLimit := int64(upLimit)
	if dailyNum >= dailyUpLimit {
		return 0
	}

	var canGet = count
	if dailyNum+count > dailyUpLimit {
		// 确保不会出现负数结果
		if dailyUpLimit > dailyNum {
			canGet = dailyUpLimit - dailyNum
		} else {
			canGet = 0
		}
	}
	data.DailyRecycleNum[itemId] += canGet
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSmithEquipRecycle, &pb3.LogPlayerCounter{NumArgs: uint64(sysId), StrArgs: fmt.Sprintf("%d_%d_%d_%d", itemId, dailyNum, canGet, data.DailyRecycleNum[itemId])})
	return canGet
}

func (s *SmithEquipSys) handleAeNewDay() {
	data := s.GetSysData()
	data.DailyRecycleNum = make(map[uint32]int64)
	s.s2cInfo()
}

func regSmithEquipSys() {
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		RegisterSysClass(ref.EquipSysId, func() iface.ISystem {
			return &SmithEquipSys{}
		})

		engine.RegAttrCalcFn(ref.CalAttrSmithEqDef, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
			s, ok := player.GetSysObj(ref.EquipSysId).(*SmithEquipSys)
			if !ok || !s.IsOpen() {
				return
			}

			s.calcAttr(calc)
		})
	})
}

func GetSmithEquipSys(player iface.IPlayer, sysId uint32) (*SmithEquipSys, bool) {
	obj := player.GetSysObj(sysId)
	if obj == nil || !obj.IsOpen() {
		return nil, false
	}
	sys, ok := obj.(*SmithEquipSys)
	if !ok || !sys.IsOpen() {
		return nil, false
	}
	return sys, true
}

func handleSmithEquipRecycleItemTypeNum(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	itemType := args[0].(uint32)
	itemNum := args[1].(int64)
	switch itemType {
	case itemdef.ItemTypeSmithEquip:
		if s, ok := GetSmithEquipSys(player, sysdef.SiSmithEquip); ok {
			recycleNum := s.GetSysData().DailyRecycleNum
			sysId := s.GetSysId()
			s.SendProto3(80, 15, &pb3.S2C_80_15{
				SysId:           sysId,
				DailyRecycleNum: recycleNum,
			})
			player.TriggerEvent(custom_id.AeAddActivityHandBookExp, SmithEquipRecyclePYYId, uint32(itemNum))
			player.TriggerQuestEvent(custom_id.QttSmithRecycleTimes, sysId, itemNum)
			player.TriggerQuestEvent(custom_id.QttHistorySmithRecycleTimes, sysId, itemNum)
		}
	}
}

func handleSmithEquipAeNewDay(player iface.IPlayer, _ ...interface{}) {
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		obj := player.GetSysObj(ref.EquipSysId)
		if obj == nil || !obj.IsOpen() {
			return
		}
		sys := obj.(*SmithEquipSys)
		if sys == nil {
			return
		}
		sys.handleAeNewDay()
	})
}

func init() {
	regSmithEquipSys()

	net.RegisterProto(80, 12, c2sTakeOn)
	net.RegisterProto(80, 13, c2sTakeOff)
	event.RegActorEvent(custom_id.AeRecycleItemTypeNum, handleSmithEquipRecycleItemTypeNum)
	event.RegActorEventL(custom_id.AeNewDay, handleSmithEquipAeNewDay)
}
