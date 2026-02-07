/**
 * @Author: yzh
 * @Date:
 * @Desc: 新法宝
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type FaBaoSys struct {
	Base
}

func (s *FaBaoSys) OnLogin() {
	if !s.IsOpen() {
		return
	}

	s.S2CInfo()
}

func (s *FaBaoSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)
	s.S2CInfo()
}

func (s *FaBaoSys) OnOpen() {
	s.S2CInfo()
	conf := jsondata.GetNewFaBaoConf()
	if conf == nil {
		return
	}
	for i := uint32(0); i < conf.SlotNum; i++ {
		err := s.unLockSlot(i + 1)
		if err != nil {
			s.GetOwner().LogWarn("err:%v", err)
		}
	}
}

func (s *FaBaoSys) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *FaBaoSys) S2CInfo() {
	s.SendProto3(158, 0, &pb3.S2C_158_0{
		State: s.state(),
	})
}

// 低16：位Id；高16：阵Id
func (s *FaBaoSys) getSlotIdx(zId, idx uint32) uint32 {
	return utils.Make32(utils.High16(zId), utils.Low16(idx))
}

func (s *FaBaoSys) splitSlotIdx(slotIdx uint32) (uint32, uint32) {
	zId := uint32(utils.High16(slotIdx))
	idx := uint32(utils.Low16(slotIdx))
	return zId, idx
}

func (s *FaBaoSys) GetTotalLv() uint32 {
	state := s.state()
	var totalLv uint32
	for _, bao := range state.FaBaoMap {
		totalLv += bao.Lv
	}
	return totalLv
}

func (s *FaBaoSys) afterActive(itemId uint32) {
	conf := jsondata.GetNewFaBaoConf()
	faBaoConf, ok := conf.FaBaos[itemId]
	if !ok {
		s.GetOwner().LogWarn("fa bao conf not found")
		return
	}

	state := s.state()
	for _, suit := range state.Suits {
		if !pie.Uint32s(suit.FaBaoItemIds).Contains(itemId) {
			continue
		}
		s.GetOwner().LogInfo("suit already active")
		return
	}

	var suit *jsondata.NewFaBaoSuitConf
	if len(faBaoConf.Suits) > 0 {
		suit = faBaoConf.Suits[0]
	}
	if suit == nil {
		s.GetOwner().LogWarn("fa bao suit conf not found")
		return
	}

	active := true
	for _, suitItemId := range suit.ItemIds {
		_, ok := state.FaBaoMap[suitItemId]
		if ok {
			continue
		}
		active = false
		break
	}

	// 不满足激活条件
	if !active {
		s.GetOwner().LogInfo("active failed, active faBao is %v", suit.ItemIds)
		return
	}

	s.LogPlayerBehavior(0, map[string]interface{}{
		"itemIds": suit.ItemIds,
	}, pb3.LogId_LogNewFaBaoActiveSuitsBehavior)

	state.Suits = append(state.Suits, &pb3.NewFaBaoSuit{
		FaBaoItemIds: suit.ItemIds,
	})
}

func (s *FaBaoSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: coreNumData,
		StrArgs: string(bytes),
	})
}

func (s *FaBaoSys) active(itemId uint32) error {

	conf := jsondata.GetNewFaBaoConf()

	faBaoConf, ok := conf.FaBaos[itemId]
	if !ok {
		err := neterror.ConfNotFoundError("fa bao conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	state := s.state()
	for _, faBao := range state.FaBaoMap {
		if faBao.Id != itemId {
			continue
		}
		err := neterror.ParamsInvalidError("fa bao already exist")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	// 激活法宝的消耗
	if !s.owner.ConsumeByConf(jsondata.ConsumeVec{{
		Id:    itemId,
		Count: 1,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveNewFaBao}) {
		err := neterror.ConsumeFailedError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if len(faBaoConf.Qualities) == 0 {
		err := neterror.ConfNotFoundError("not quality config")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	qualityConf := faBaoConf.Qualities[0]
	if !s.owner.ConsumeByConf(qualityConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveNewFaBao}) {
		err := neterror.ConsumeFailedError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	s.LogPlayerBehavior(uint64(itemId), map[string]interface{}{}, pb3.LogId_LogNewFaBaoActiveBehavior)

	state.FaBaoMap[itemId] = &pb3.NewFaBao{
		Id:      itemId,
		Quality: 1,
		Lv:      1,
	}

	s.owner.TriggerEvent(custom_id.AeActiveNewFaBao, itemId)

	s.afterActive(itemId)
	s.initTalent(itemId)

	s.ResetSysAttr(attrdef.SaNewFaBao)

	s.owner.TriggerQuestEventRange(custom_id.QttActiveFaBao)

	s.SendProto3(158, 1, &pb3.S2C_158_1{
		FaBao: state.FaBaoMap[itemId],
	})
	s.SendProto3(158, 12, &pb3.S2C_158_12{
		TalentData: state.FaBaoTalent[itemId],
	})
	return nil
}

func (s *FaBaoSys) upLv(itemId uint32) error {
	conf := jsondata.GetNewFaBaoConf()

	faBaoConf, ok := conf.FaBaos[itemId]
	if !ok {
		err := neterror.ConfNotFoundError("fa bao conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	state := s.state()
	faBao, ok := state.FaBaoMap[itemId]
	if !ok {
		err := neterror.ParamsInvalidError("fa bao not active")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	var qualityConf *jsondata.NewFaBaoQualityConf
	for _, qConf := range faBaoConf.Qualities {
		if qConf.Quality != faBao.Quality {
			continue
		}
		qualityConf = qConf
		break
	}
	if qualityConf == nil {
		err := neterror.ConfNotFoundError("fa bao next quality not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	var nextLvConf *jsondata.NewFaBaoLvConf
	for _, lvC := range qualityConf.Lvs {
		if lvC.Lv != faBao.Lv+1 {
			continue
		}
		nextLvConf = lvC
		break
	}
	if nextLvConf == nil {
		err := neterror.ConfNotFoundError("fa bao next lv conf not found , quality is %d , next lv is %d", faBao.Quality, faBao.Lv+1)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if len(nextLvConf.Consume) > 0 && !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvNewFaBao}) {
		err := neterror.ConsumeFailedError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	var logSt pb3.LogExpLv
	logSt.FromLevel = faBao.Lv
	faBao.Lv = nextLvConf.Lv
	logSt.ToLevel = faBao.Lv
	logworker.LogExpLv(s.GetOwner(), pb3.LogId_LogNewFaBaoUpLvExp, &logSt)

	s.ResetSysAttr(attrdef.SaNewFaBao)

	s.owner.TriggerQuestEventRange(custom_id.QttNewFaBaoAnyUpTo)

	s.SendProto3(158, 2, &pb3.S2C_158_2{
		FaBao: faBao,
	})
	return nil
}

func (s *FaBaoSys) upStar(itemId uint32) error {
	conf := jsondata.GetNewFaBaoConf()
	faBaoConf, ok := conf.FaBaos[itemId]
	if !ok {
		err := neterror.ConfNotFoundError("fa bao conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	state := s.state()
	faBao, ok := state.FaBaoMap[itemId]
	if !ok {
		err := neterror.ParamsInvalidError("fa bao not active")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	var nextStarConf *jsondata.NewFaBaoStarConf
	for _, star := range faBaoConf.Stars {
		if star.Star != faBao.Star+1 {
			continue
		}
		nextStarConf = star
		break
	}
	if nextStarConf == nil {
		err := neterror.ConfNotFoundError("fa bao conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if !s.owner.ConsumeByConf(nextStarConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvNewFaBao}) {
		err := neterror.ConsumeFailedError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	s.LogPlayerBehavior(uint64(faBao.Id), map[string]interface{}{
		"oldStar": faBao.Star,
		"newStar": faBao.Star + 1,
	}, pb3.LogId_LogNewFaBaoUpStarSuccess)

	faBao.Star++
	if s.isOnShelf(itemId) && nextStarConf.SkillId > 0 {
		s.owner.LearnSkill(nextStarConf.SkillId, nextStarConf.SkillLv, true)
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)
	s.SendProto3(158, 3, &pb3.S2C_158_3{
		FaBao: faBao,
	})
	return nil
}

func (s *FaBaoSys) isActive(itemId uint32) bool {
	for _, suit := range s.state().Suits {
		if pie.Uint32s(suit.FaBaoItemIds).Contains(itemId) {
			return true
		}
	}
	return false
}

func (s *FaBaoSys) isOnShelf(itemId uint32) bool {
	for _, faBao := range s.state().BattleSlots {
		if faBao.Id == itemId {
			return true
		}
	}
	return false
}

func (s *FaBaoSys) upQuality(itemId uint32) error {
	conf := jsondata.GetNewFaBaoConf()
	faBaoConf, ok := conf.FaBaos[itemId]
	if !ok {
		s.GetOwner().LogWarn("fa bao conf not found")
		return nil
	}

	state := s.state()
	faBao, ok := state.FaBaoMap[itemId]
	if !ok {
		err := neterror.ConfNotFoundError("fa bao not active")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if faBao.Quality >= uint32(len(faBaoConf.Qualities)) {
		err := neterror.ConfNotFoundError("quality is overflow")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	// 取下一阶的品质
	var nextQualityConf *jsondata.NewFaBaoQualityConf
	for _, qConf := range faBaoConf.Qualities {
		if qConf.Quality != faBao.Quality+1 {
			continue
		}
		nextQualityConf = qConf
		break
	}
	if nextQualityConf == nil {
		err := neterror.ConfNotFoundError("fa bao next quality not found , next is %d", faBao.Quality+1)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if !s.owner.ConsumeByConf(nextQualityConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvNewFaBao}) {
		err := neterror.ConsumeFailedError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	s.LogPlayerBehavior(uint64(faBao.Id), map[string]interface{}{
		"oldQuality": faBao.Quality,
		"newQuality": faBao.Quality + 1,
	}, pb3.LogId_LogNewFaBaoUpQualitySuccess)

	faBao.Quality++

	s.ResetSysAttr(attrdef.SaNewFaBao)

	s.SendProto3(158, 3, &pb3.S2C_158_3{
		FaBao: faBao,
	})
	return nil
}

func (s *FaBaoSys) activeSuitSkill(itemId, skillLv uint32) error {
	state := s.state()
	var suit *pb3.NewFaBaoSuit
	for _, one := range state.Suits {
		if !pie.Uint32s(one.FaBaoItemIds).Contains(itemId) {
			continue
		}
		suit = one
		break
	}

	if suit == nil {
		err := neterror.ParamsInvalidError("suit not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	for _, itemId := range suit.FaBaoItemIds {
		faBao, ok := state.FaBaoMap[itemId]
		if !ok {
			err := neterror.ParamsInvalidError("fa bao(item id:%d) not found", faBao.Id)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}

		if faBao.Star < skillLv {
			err := neterror.ParamsInvalidError("fa bao(item id:%d) skill lv < skill lv", faBao.Id)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}
	}

	s.LogPlayerBehavior(uint64(itemId), map[string]interface{}{
		"suit":    suit,
		"skillLv": skillLv,
	}, pb3.LogId_LogNewFaBaoActiveSuitSkillSuccess)

	suit.Lv = skillLv

	battleFaBaoMap := map[uint32]*pb3.NewFaBao{}
	for _, faBao := range state.BattleSlots {
		battleFaBaoMap[faBao.Id] = faBao
	}
	conf := jsondata.GetNewFaBaoConf()
	for _, itemId := range suit.FaBaoItemIds {
		_, ok := battleFaBaoMap[itemId]
		if !ok {
			continue
		}
		faBaoConf, ok := conf.FaBaos[itemId]
		if !ok {
			continue
		}
		starConf := faBaoConf.Stars[skillLv-1]
		if starConf.SuitSkillId > 0 {
			s.owner.LearnSkill(starConf.SuitSkillId, starConf.SkillLv, true)
		}
	}

	s.SendProto3(158, 9, &pb3.S2C_158_9{
		Suit: suit,
	})
	return nil
}

func (s *FaBaoSys) state() *pb3.NewFaBaoState {
	if s.GetBinaryData().FaBaoState == nil {
		s.GetBinaryData().FaBaoState = &pb3.NewFaBaoState{}
	}
	if s.GetBinaryData().FaBaoState.FaBaoMap == nil {
		s.GetBinaryData().FaBaoState.FaBaoMap = make(map[uint32]*pb3.NewFaBao)
	}
	if s.GetBinaryData().FaBaoState.BattleSlots == nil {
		s.GetBinaryData().FaBaoState.BattleSlots = make(map[uint32]*pb3.NewFaBao)
	}
	if s.GetBinaryData().FaBaoState.FaBaoTalent == nil {
		s.GetBinaryData().FaBaoState.FaBaoTalent = make(map[uint32]*pb3.NewFaBaoTalent)
	}
	return s.GetBinaryData().FaBaoState
}

// 解锁槽位
func (s *FaBaoSys) unLockSlot(slotIdx uint32) error {
	data := s.state()
	conf := jsondata.GetNewFaBaoConf()

	if conf == nil {
		err := neterror.ConfNotFoundError("FaBaoSys conf is nil")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	zId, id := s.splitSlotIdx(slotIdx)
	if zId == 0 || id == 0 {
		s.GetOwner().LogWarn("slotIdx not zero")
		return nil
	}

	if id > uint32(len(conf.UnLockSlotJJLvs)) {
		err := neterror.ConfNotFoundError("FaBaoSys lock slot jj lvs conf is nil , idx is %d", slotIdx)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	zConf, ok := conf.FaBaoZhen[zId]
	if !ok {
		return neterror.ConfNotFoundError("FaBaoSys lock zheng conf is nil, idx is %d", slotIdx)
	}

	if zConf.MergeTimes > 0 {
		if gshare.GetMergeTimes() < zConf.MergeTimes {
			return neterror.ConfNotFoundError("FaBaoSys zheng is lock mergeTimes limit, idx is %d", slotIdx)
		}
		if gshare.GetMergeTimes() == zConf.MergeTimes {
			if gshare.GetMergeSrvDay() < zConf.MergeDay {
				return neterror.ConfNotFoundError("FaBaoSys zheng is lock mergeDay limit, idx is %d", slotIdx)
			}
		}
	}

	// 重复解锁
	if pie.Uint32s(data.BattleSlotIdxVec).Contains(slotIdx) {
		s.GetOwner().LogWarn("data is %d , already un slot idx , val is %d", data.BattleSlotIdxVec, slotIdx)
		return nil
	}

	sum := conf.SlotNum * uint32(len(conf.FaBaoZhen))
	if uint32(len(data.BattleSlotIdxVec)) >= sum {
		s.GetOwner().LogWarn("battle slot number %d > conf slot number %d", uint32(len(data.BattleSlotIdxVec)), conf.SlotNum)
		return nil
	}

	jjSys, ok := s.GetOwner().GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok {
		err := neterror.SysNotExistError("NewJingJieSys is nil")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	jjData := jjSys.GetData()
	if jjData == nil {
		err := neterror.SysNotExistError("NewJingJieSys data is nil")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	jjCondLv := conf.UnLockSlotJJLvs[id-1]
	if jjCondLv > jjSys.GetData().Level {
		s.GetOwner().LogWarn("jj lv not enough ,cand is %d , jjlv is %d", jjCondLv, jjSys.GetData().Level)
		return nil
	}

	s.LogPlayerBehavior(uint64(slotIdx), map[string]interface{}{}, pb3.LogId_LogNewFaBaoUnLockSuccess)

	// 满足条件 解锁
	data.BattleSlotIdxVec = append(data.BattleSlotIdxVec, slotIdx)
	s.GetOwner().SendProto3(158, 5, &pb3.S2C_158_5{
		SlotIdx: slotIdx,
	})
	return nil
}

// 上阵
func (s *FaBaoSys) onShelf(slotIdx uint32, itemId uint32) error {
	data := s.state()
	conf := jsondata.GetNewFaBaoConf()

	if conf == nil {
		err := neterror.ConfNotFoundError("FaBaoSys conf is nil")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if slotIdx == 0 {
		s.GetOwner().LogWarn("slotIdx not zero")
		return nil
	}

	itemSt := conf.FaBaos[itemId]

	// 没解锁
	if !pie.Uint32s(data.BattleSlotIdxVec).Contains(slotIdx) {
		s.GetOwner().LogWarn("battle slot num is %d , slot idx is %d", uint32(len(data.BattleSlotIdxVec)), slotIdx)
		return nil
	}

	// 拿信息
	newFaBao, ok := data.FaBaoMap[itemSt.ItemId]
	if !ok {
		s.GetOwner().LogWarn("fa bao un active , item id is %d", itemSt.ItemId)
		return nil
	}

	// 重复上阵
	faBao, ok := data.BattleSlots[slotIdx]
	if ok {
		if faBao.Id == newFaBao.Id {
			s.GetOwner().LogWarn("already on shelf slot , item is %d", itemSt.ItemId)
			return nil
		}
	}

	// 其他槽位
	var otherIdx uint32
	for idx, otherFaBao := range data.BattleSlots {
		if otherFaBao.Id != itemId {
			continue
		}
		otherIdx = idx
		break
	}

	// 先下再上
	if otherIdx > 0 {
		err := s.offShelf(otherIdx)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return err
		}
	}

	s.LogPlayerBehavior(uint64(newFaBao.Id), map[string]interface{}{
		"slotIdx": slotIdx,
	}, pb3.LogId_LogNewFaBaoUnLockSuccess)

	data.BattleSlots[slotIdx] = newFaBao

	sks, err := s.getCurFaBaoSkills(newFaBao)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}

	for _, sk := range sks {
		if len(sk) != 2 {
			continue
		}
		s.GetOwner().LearnSkill(sk[0], sk[1], true)
	}

	s.GetOwner().SendProto3(158, 6, &pb3.S2C_158_6{
		SlotIdx: slotIdx,
		FaBao:   newFaBao,
	})
	return nil
}

// 下场
func (s *FaBaoSys) offShelf(slotIdx uint32) error {
	data := s.state()

	if slotIdx == 0 {
		s.GetOwner().LogWarn("slotIdx not zero")
		return nil
	}

	// 已上阵
	faoBao, ok := data.BattleSlots[slotIdx]
	if !ok {
		s.GetOwner().LogWarn("not found battle slot fa bao  , idx is %d", slotIdx)
		return nil
	}

	s.LogPlayerBehavior(uint64(faoBao.Id), map[string]interface{}{
		"slotIdx": slotIdx,
	}, pb3.LogId_LogNewFaBaoOffShelfSuccess)

	delete(data.BattleSlots, slotIdx)

	sks, err := s.getCurFaBaoSkills(faoBao)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	for _, sk := range sks {
		s.GetOwner().ForgetSkill(sk[0], true, true, true)
	}

	// 下发下阵
	s.GetOwner().SendProto3(158, 8, &pb3.S2C_158_8{
		SlotIdx: slotIdx,
	})
	return nil
}

// 替换
func (s *FaBaoSys) replace(slotIdx, itemId uint32) error {
	if slotIdx == 0 {
		s.GetOwner().LogWarn("slotIdx not zero")
		return nil
	}

	data := s.state()

	// 下当前槽位的法宝
	curFb := data.BattleSlots[slotIdx]
	if curFb != nil {
		err := s.offShelf(slotIdx)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return err
		}
	}

	// 其他槽位
	var otherIdx uint32
	for idx, otherFaBao := range data.BattleSlots {
		if otherFaBao.Id != itemId {
			continue
		}
		otherIdx = idx
		break
	}

	// 下其他槽位的法宝
	if otherIdx > 0 {
		err := s.offShelf(otherIdx)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return err
		}
	}

	// 上阵
	err := s.onShelf(slotIdx, itemId)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	// 上阵
	if curFb != nil {
		err = s.onShelf(otherIdx, curFb.Id)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return err
		}
	}

	s.LogPlayerBehavior(uint64(itemId), map[string]interface{}{
		"SlotIdx": slotIdx,
	}, pb3.LogId_LogNewFaBaoReplaceEnd)

	s.SendProto3(158, 10, &pb3.S2C_158_10{
		SlotIdx: slotIdx,
		FaBao:   s.state().FaBaoMap[itemId],
	})

	return nil
}

func (s *FaBaoSys) getFaBaoStarConf(faBao *pb3.NewFaBao, star uint32) (*jsondata.NewFaBaoStarConf, error) {
	conf := jsondata.GetNewFaBaoConf()

	if conf == nil {
		return nil, neterror.ConfNotFoundError("FaBaoSys conf is nil")
	}

	// 法宝配制
	faBaoConf, ok := conf.FaBaos[faBao.Id]
	if !ok {
		return nil, neterror.ConfNotFoundError("not found fa bao conf item id is %d", faBao.Id)
	}

	// 星级技能
	var faBaoStar *jsondata.NewFaBaoStarConf
	for _, faBaoC := range faBaoConf.Stars {
		if faBaoC.Star != star {
			continue
		}
		faBaoStar = faBaoC
		break
	}
	if faBaoStar == nil {
		return nil, neterror.ConfNotFoundError("faBaoStar conf is nil star: %d", star)
	}
	return faBaoStar, nil
}

func (s *FaBaoSys) getCurFaBaoSkills(faBao *pb3.NewFaBao) ([][]uint32, error) {
	var skills [][]uint32
	faBaoStarConf, err := s.getFaBaoStarConf(faBao, faBao.Star)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return nil, err
	}

	skills = append(skills, []uint32{faBaoStarConf.SkillId, faBaoStarConf.SkillLv})

	state := s.state()

	for i := range state.Suits {
		suit := state.Suits[i]
		if !pie.Uint32s(suit.FaBaoItemIds).Contains(faBao.Id) {
			s.GetOwner().LogWarn("suit idx %d not found itemId %d", i, faBao.Id)
			continue
		}

		faBaoStarConf, err := s.getFaBaoStarConf(faBao, suit.Lv)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return nil, err
		}

		if faBaoStarConf.SuitSkillId == 0 {
			break
		}

		skills = append(skills, []uint32{faBaoStarConf.SuitSkillId, suit.Lv})
	}

	return skills, nil
}

func (s *FaBaoSys) c2sActiveFaBao(msg *base.Message) error {
	var req pb3.C2S_158_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return s.active(req.ItemId)
}

func (s *FaBaoSys) c2sUpLvFaBao(msg *base.Message) error {
	var req pb3.C2S_158_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return s.upLv(req.ItemId)
}

func (s *FaBaoSys) c2sUpQuality(msg *base.Message) error {
	var req pb3.C2S_158_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return s.upQuality(req.ItemId)
}

func (s *FaBaoSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_158_4
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return s.upStar(req.ItemId)
}

func (s *FaBaoSys) c2sUnLock(msg *base.Message) error {
	var req pb3.C2S_158_5
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	err := s.unLockSlot(req.SlotIdx)
	if err != nil {
		return err
	}
	return nil
}

func (s *FaBaoSys) c2sOnShelf(msg *base.Message) error {
	var req pb3.C2S_158_6
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if !s.canOptFaBao() {
		s.GetOwner().LogWarn("not opt fa bao")
		s.GetOwner().SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	err := s.onShelf(req.SlotIdx, req.ItemId)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)

	return nil
}

func (s *FaBaoSys) c2sOffShelf(msg *base.Message) error {
	var req pb3.C2S_158_8
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if !s.canOptFaBao() {
		s.GetOwner().LogWarn("not opt fa bao")
		s.GetOwner().SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	err := s.offShelf(req.SlotIdx)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)

	return nil
}

func (s *FaBaoSys) c2sReplace(msg *base.Message) error {
	var req pb3.C2S_158_10
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if !s.canOptFaBao() {
		s.GetOwner().LogWarn("not opt fa bao")
		s.GetOwner().SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	err := s.replace(req.SlotIdx, req.ItemId)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)

	return nil
}

func (s *FaBaoSys) c2sActiveSuitSkill(msg *base.Message) error {
	var req pb3.C2S_158_9
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	err := s.activeSuitSkill(req.ItemId, req.ActiveSuitSkillLv)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	s.ResetSysAttr(attrdef.SaNewFaBao)

	return nil
}

func (s *FaBaoSys) c2sNewFaBaoBattle(msg *base.Message) error {
	var req pb3.C2S_158_11
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if s.isActorDeath() {
		s.GetOwner().LogWarn("actor is dead")
		s.GetOwner().SendTipMsg(tipmsgid.TpFightingLimit)
		return nil
	}

	s.SendProto3(158, 11, &pb3.S2C_158_11{
		ItemId: req.ItemId,
	})
	return nil
}

func (s *FaBaoSys) c2sUpgradeNewFaBaoTalent(msg *base.Message) error {
	var req pb3.C2S_158_12
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	data := s.state()
	talent, ok := data.FaBaoTalent[req.ItemId]
	faBao, ok2 := data.FaBaoMap[req.ItemId]

	if !ok || !ok2 {
		err := neterror.ParamsInvalidError("fa bao not active")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	newTalentLv := talent.Lv + 1
	tLvConf := jsondata.GetNewFaBaoTalentLvConf(req.ItemId, newTalentLv)
	if tLvConf == nil {
		err := neterror.ConfNotFoundError("fa bao conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	tStarConf, err := s.getFaBaoStarConf(faBao, faBao.Star)
	if err != nil {
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}
	if newTalentLv > tStarConf.TalentMaxLv {
		err := neterror.ParamsInvalidError("fa bao star limit")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if talent.Count < tLvConf.Count {
		err := neterror.ParamsInvalidError("fa bao talent upgrade error")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	talent.Lv = newTalentLv
	s.ResetSysAttr(attrdef.SaNewFaBao)

	s.SendProto3(158, 12, &pb3.S2C_158_12{
		TalentData: talent,
	})

	return nil
}

func (s *FaBaoSys) isActorDeath() bool {
	if s.GetOwner().HasState(custom_id.ESDeath) {
		s.GetOwner().LogInfo("actor is death")
		return true
	}
	return false
}

func (s *FaBaoSys) canOptFaBao() bool {
	if s.isActorDeath() {
		s.GetOwner().LogInfo("actor is death")
		return false
	}

	if s.GetOwner().GetFightStatus() {
		s.GetOwner().LogInfo("actor is fight status")
		return false
	}

	return true
}

// 法宝天赋
func (s *FaBaoSys) initTalent(itemId uint32) {
	tConf := jsondata.GetNewFaBaoTalentConf(itemId)
	if tConf == nil {
		s.GetOwner().LogWarn("fa bao conf not found")
		return
	}

	state := s.state()
	if _, ok := state.FaBaoTalent[itemId]; ok {
		return
	}
	state.FaBaoTalent[itemId] = &pb3.NewFaBaoTalent{
		ItemId: itemId,
		Lv:     1,
		Cond:   tConf.Cond,
	}
}

// 处理法宝天赋
func (s *FaBaoSys) handleNewFaBaoTalent(param *custom_id.FaBaoTalentEvent) {
	fConf := jsondata.GetNewFaBaoTalentConfByCond(param.Cond)
	if fConf == nil {
		return
	}

	state := s.state()
	talent, ok := state.FaBaoTalent[fConf.ItemId]
	if !ok {
		return
	}

	isAdd := false
	if len(fConf.Params) == 0 {
		isAdd = true
	} else if len(fConf.Params) == 1 {
		if param.Param0 >= fConf.Params[0] {
			isAdd = true
		}
	} else if len(fConf.Params) == 2 {
		if param.Param0 >= fConf.Params[0] &&
			param.Param1 >= fConf.Params[1] {
			isAdd = true
		}
	}

	if isAdd {
		talent.Count += param.Count
	}

	// 推送天赋变化
	s.SendProto3(158, 12, &pb3.S2C_158_12{TalentData: talent})
}

func calcFaBaoSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok {
		return
	}

	newFaBaoConf := jsondata.GetNewFaBaoConf()
	if newFaBaoConf == nil {
		return
	}

	data := sys.state()
	var attrs jsondata.AttrVec
	for _, faBao := range data.FaBaoMap {
		newFaBao := newFaBaoConf.FaBaos[faBao.Id]

		var fbQuality *jsondata.NewFaBaoQualityConf
		for i := range newFaBao.Qualities {
			qualityConf := newFaBao.Qualities[i]
			if qualityConf.Quality != faBao.Quality {
				continue
			}
			fbQuality = qualityConf
			break
		}
		if fbQuality == nil {
			continue
		}

		var lvConf *jsondata.NewFaBaoLvConf
		for i := range fbQuality.Lvs {
			lv := fbQuality.Lvs[i]
			if faBao.Lv != lv.Lv {
				continue
			}
			lvConf = lv
			break
		}
		if lvConf != nil {
			// 等级加成
			attrs = append(attrs, lvConf.Attr...)
			attrs = append(attrs, lvConf.DestinedAttrVes...)
		}

		// 升星加成
		var starC *jsondata.NewFaBaoStarConf
		for i := range newFaBao.Stars {
			starConf := newFaBao.Stars[i]
			if starConf.Star != faBao.Star {
				continue
			}
			starC = starConf
			break
		}
		if starC != nil {
			attrs = append(attrs, starC.Attr...)
			attrs = append(attrs, starC.DestinedAttrVes...)
		}

		// 天赋加成
		talent := data.FaBaoTalent[faBao.Id]
		if talent != nil {
			tLvConf := jsondata.GetNewFaBaoTalentLvConf(faBao.Id, talent.Lv)
			if tLvConf != nil {
				attrs = append(attrs, tLvConf.Attr...)
			}
		}

		attrs = jsondata.MergeAttrVec(attrs...)
	}

	if len(attrs) > 0 {
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func qttNewFaBaoAnyUpTo(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	obj := actor.GetSysObj(sysdef.SiNewFabao)
	if obj == nil {
		return 0
	}
	if !obj.IsOpen() {
		return 0
	}
	s := obj.(*FaBaoSys)
	var maxLv uint32
	for _, bao := range s.state().FaBaoMap {
		if maxLv >= bao.Lv {
			continue
		}
		maxLv = bao.Lv
	}
	return maxLv
}

func qttNewFaBaoActiveNum(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	obj := actor.GetSysObj(sysdef.SiNewFabao)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	s := obj.(*FaBaoSys)
	return uint32(len(s.state().FaBaoMap))
}

func newFaBaoOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaNewFaBao}

	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}
	player.SetRankValue(gshare.RankTypeFaBao, sumPower)
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeFaBao, player.GetId(), sumPower)
}

func newFaBaoUpdateTalentData(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 1 {
		return
	}

	eData, ok := args[0].(*custom_id.FaBaoTalentEvent)
	if !ok {
		return
	}

	sys.handleNewFaBaoTalent(eData)
}

func newFaBaoHandleKillMon(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 4 {
		return
	}

	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	conf := jsondata.GetMonsterConf(monId)
	if conf == nil {
		return
	}

	cond := 0
	switch conf.SubType {
	case custom_id.MstWorldBoss:
		cond = custom_id.FaBaoTalentCondKillWorldBoss
	case custom_id.MstSuitBoss:
		cond = custom_id.FaBaoTalentCondKillSuitBoss
	case custom_id.MstCrossFairyBoss:
		cond = custom_id.FaBaoTalentCondKillFairyLandBoss
	case custom_id.MstSelfBoss:
		cond = custom_id.FaBaoTalentCondSelfBoss
	case custom_id.MstQiMenBoss:
		cond = custom_id.FaBaoTalentCondQiMenBoss
	case custom_id.MstGodBeastBoss:
		cond = custom_id.FaBaoTalentCondGodBeastBoss
	case custom_id.MstDivineRealmBoss:
		cond = custom_id.FaBaoTalentDivineRealmBoss
	case custom_id.MstFaShenBoss:
		cond = custom_id.FaBaoTalentFaShenBoss
	case custom_id.MstSectHuntingBoss:
		cond = custom_id.FaBaoTalentSectHuntingBoss
	}

	if cond > 0 {
		sys.handleNewFaBaoTalent(&custom_id.FaBaoTalentEvent{
			Cond:   uint32(cond),
			Count:  count,
			Param0: conf.Quality,
		})
	}
}

func handlePowerRushRankSubTypeFaBao(player iface.IPlayer) (score int64) {
	attrSys := player.GetAttrSys()
	if attrSys == nil {
		return 0
	}
	var totalPower int64
	var sysIds = []uint32{
		attrdef.SaNewFaBao,
		attrdef.SaFaBaoGift,
		attrdef.SaDestinedFaBao,
		attrdef.AtDestinedFaBaoEngrave,
	}
	for _, sysId := range sysIds {
		totalPower += attrSys.GetSysPower(sysId)
	}
	return totalPower
}

func init() {
	RegisterSysClass(sysdef.SiNewFabao, func() iface.ISystem {
		return &FaBaoSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaNewFaBao, calcFaBaoSysAttr)
	engine.RegQuestTargetProgress(custom_id.QttNewFaBaoAnyUpTo, qttNewFaBaoAnyUpTo)
	engine.RegQuestTargetProgress(custom_id.QttActiveFaBao, qttNewFaBaoActiveNum)

	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, newFaBaoOnUpdateSysPowerMap)
	event.RegActorEvent(custom_id.AeFaBaoTalentEvent, newFaBaoUpdateTalentData)
	event.RegActorEvent(custom_id.AeKillMon, newFaBaoHandleKillMon)

	net.RegisterSysProtoV2(158, 1, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sActiveFaBao
	})
	net.RegisterSysProtoV2(158, 2, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sUpLvFaBao
	})
	net.RegisterSysProtoV2(158, 3, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sUpQuality
	})
	net.RegisterSysProtoV2(158, 4, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sUpStar
	})
	net.RegisterSysProtoV2(158, 5, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sUnLock
	})
	net.RegisterSysProtoV2(158, 6, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sOnShelf
	})
	net.RegisterSysProtoV2(158, 8, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sOffShelf
	})
	net.RegisterSysProtoV2(158, 9, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sActiveSuitSkill
	})
	net.RegisterSysProtoV2(158, 10, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sReplace
	})
	net.RegisterSysProtoV2(158, 11, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sNewFaBaoBattle
	})
	net.RegisterSysProtoV2(158, 12, sysdef.SiNewFabao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoSys).c2sUpgradeNewFaBaoTalent
	})

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeFaBao, handlePowerRushRankSubTypeFaBao)

	gmevent.Register("fbao.talent", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiNewFabao)
		if obj == nil {
			return false
		}
		if len(args) < 2 {
			return false
		}
		cond := utils.AtoUint32(args[0])
		count := utils.AtoUint32(args[1])
		param := &custom_id.FaBaoTalentEvent{
			Cond:  cond,
			Count: count,
		}
		switch cond {
		case custom_id.FaBaoTalentCondWorldBoss:
		case custom_id.FaBaoTalentCondAlchemyItem:
			param.Param0 = 5
		case custom_id.FaBaoTalentCondBattleArena:
		}
		player.TriggerEvent(custom_id.AeFaBaoTalentEvent, param)
		return true
	}, 1)
}
