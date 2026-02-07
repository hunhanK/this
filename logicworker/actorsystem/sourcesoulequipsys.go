/**
 * @Author: zjj
 * @Date:
 * @Desc: 源魂
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SourceSoulEquipSys struct {
	Base
}

func (s *SourceSoulEquipSys) s2cInfo() {
	s.SendProto3(8, 190, &pb3.S2C_8_190{
		Data: s.getData(),
	})
}

func (s *SourceSoulEquipSys) getData() *pb3.SourceSoulEquipData {
	data := s.GetBinaryData().SourceSoulEquipData
	if data == nil {
		s.GetBinaryData().SourceSoulEquipData = &pb3.SourceSoulEquipData{}
		data = s.GetBinaryData().SourceSoulEquipData
	}
	if data.TakeOnEquip == nil {
		data.TakeOnEquip = make(map[uint32]*pb3.ItemSt)
	}
	if data.SuitMaxIdxMap == nil {
		data.SuitMaxIdxMap = make(map[uint64]uint32)
	}
	return data
}

func (s *SourceSoulEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SourceSoulEquipSys) OnLogin() {
	s.s2cInfo()
}

func (s *SourceSoulEquipSys) OnOpen() {
	s.s2cInfo()
}

func (s *SourceSoulEquipSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_8_191
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	pos := req.Pos
	if !jsondata.IsSourceSoulEquipPos(pos) {
		return neterror.ParamsInvalidError("%d not conf pos", pos)
	}
	owner := s.GetOwner()
	itemSt := owner.GetSourceSoulEquipItemByHandle(req.Handle)
	if itemSt == nil {
		return neterror.ParamsInvalidError("%d not found", req.Handle)
	}

	itemConf := jsondata.GetItemConfig(itemSt.ItemId)
	if itemConf == nil {
		return neterror.ConfNotFoundError("%d not found item conf", itemSt.ItemId)
	}

	if !itemdef.IsIstSourceSoulEquip(itemConf.SubType) {
		return neterror.ParamsInvalidError("%d not source soul item", itemConf.Id)
	}

	if !owner.CheckItemCond(itemConf) {
		return neterror.ParamsInvalidError("%d not reach take on cond", itemConf.Id)
	}

	data := s.getData()
	st := data.TakeOnEquip[pos]
	// 先卸下
	if st != nil {
		availableCount := owner.GetSourceSoulBagAvailableCount()
		if availableCount == 0 {
			return neterror.ParamsInvalidError("bag limit")
		}
		if !owner.AddItemPtr(st, false, pb3.LogId_LogSourceSoulEquipTakeOff) {
			return neterror.ParamsInvalidError("bag limit")
		}
		s.ForgetSkill(st)
	}

	if !owner.DeleteItemPtr(itemSt, 1, pb3.LogId_LogSourceSoulEquipTakeOn) {
		return neterror.ParamsInvalidError("%d %d item remove failed", itemSt.ItemId, itemSt.ItemId)
	}
	s.LearnSkill(itemSt)
	data.TakeOnEquip[pos] = itemSt
	s.SendProto3(8, 191, &pb3.S2C_8_191{
		Pos:  pos,
		Item: itemSt,
	})
	s.ResetSysAttr(attrdef.SaSourceSoulEquip)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSourceSoulEquipTakeOn, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", itemSt.ItemId),
	})
	return nil
}

func (s *SourceSoulEquipSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_8_192
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	pos := req.Pos
	_, err = s.TakeOff(pos, true)
	if err != nil {
		return err
	}
	return nil
}

func (s *SourceSoulEquipSys) c2sActiveSuit(msg *base.Message) error {
	var req pb3.C2S_8_193
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	stageSuitConf := jsondata.GetSourceSoulEquipStageSuit(req.StageSuitIdx)
	if stageSuitConf == nil {
		return neterror.ConfNotFoundError("%d not found stageSuitConf", req.StageSuitIdx)
	}
	var subStageSuitConf *jsondata.SourceSoulEquipSubStageSuit
	for _, subStageSuit := range stageSuitConf.SubStageSuit {
		if subStageSuit.Idx == req.SubStageSuitIdx {
			subStageSuitConf = subStageSuit
			break
		}
	}
	if subStageSuitConf == nil {
		return neterror.ParamsInvalidError("%d not found subStageSuitConf", req.SubStageSuitIdx)
	}
	data := s.getData()
	var equipSuit *pb3.SourceSoulEquipSuit
	for _, suit := range data.ActiveSuit {
		if suit.StageSuitIdx == req.StageSuitIdx && suit.SubStageSuitIdx == req.SubStageSuitIdx {
			equipSuit = suit
			break
		}
	}
	if equipSuit == nil {
		equipSuit = &pb3.SourceSoulEquipSuit{
			StageSuitIdx:    req.StageSuitIdx,
			SubStageSuitIdx: req.SubStageSuitIdx,
			SuitActiveFlag:  0,
		}
		data.ActiveSuit = append(data.ActiveSuit, equipSuit)
	}

	// 一次只激活一个
	var suitAttrsConf *jsondata.SourceSoulEquipSuitAttrs
	for _, suitAttrs := range subStageSuitConf.SuitAttrs {
		if utils.IsSetBit(equipSuit.SuitActiveFlag, suitAttrs.Idx) {
			continue
		}
		suitAttrsConf = suitAttrs
		break
	}
	if suitAttrsConf == nil {
		return neterror.ParamsInvalidError("%d %d not found can active suitAttrsConf", req.StageSuitIdx, req.SubStageSuitIdx)
	}

	var reachCount uint32
	for _, itemSt := range data.TakeOnEquip {
		config := jsondata.GetItemConfig(itemSt.ItemId)
		if config == nil {
			return neterror.ConfNotFoundError("%d not found conf", itemSt.ItemId)
		}
		if !jsondata.IsSourceSoulEquipType(config.SubType, suitAttrsConf.Type) {
			continue
		}
		if suitAttrsConf.MinQuality > config.Quality {
			continue
		}
		if suitAttrsConf.MinStar > config.Star {
			continue
		}
		if suitAttrsConf.MinStage > config.Stage {
			continue
		}
		reachCount += 1
	}

	if suitAttrsConf.Num > reachCount {
		return neterror.ParamsInvalidError("%d %d not reach %d suitAttrsConf", req.StageSuitIdx, req.SubStageSuitIdx, suitAttrsConf.Idx)
	}

	equipSuit.SuitActiveFlag = utils.SetBit(equipSuit.SuitActiveFlag, suitAttrsConf.Idx)
	s.updateSuitMaxIdxMap()
	s.SendProto3(8, 193, &pb3.S2C_8_193{
		EquipSuit:     equipSuit,
		SuitMaxIdxMap: data.SuitMaxIdxMap,
	})
	s.ResetSysAttr(attrdef.SaSourceSoulEquip)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSourceSoulEquipActiveSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.StageSuitIdx),
		StrArgs: fmt.Sprintf("%d_%d", req.SubStageSuitIdx, suitAttrsConf.Idx),
	})
	return nil
}

func (s *SourceSoulEquipSys) LearnSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		s.owner.LearnSkill(itemConf.EquipSkill, itemConf.EquipSkillLv, true)
	}
}

func (s *SourceSoulEquipSys) ForgetSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		s.owner.ForgetSkill(itemConf.EquipSkill, true, true, true)
	}
}

func (s *SourceSoulEquipSys) calcAttrs(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()

	// 装备属性
	for _, itemSt := range data.TakeOnEquip {
		itemConfig := jsondata.GetItemConfig(itemSt.ItemId)
		if itemConfig == nil {
			continue
		}
		//基础属性
		engine.CheckAddAttrsToCalc(owner, calc, itemConfig.StaticAttrs)
		//极品属性
		engine.CheckAddAttrsToCalc(owner, calc, itemConfig.PremiumAttrs)
		//品质属性
		engine.CheckAddAttrsSelectQualityToCalc(owner, calc, itemConfig.SuperAttrs, itemConfig.Quality)
	}

	// 套装属性
	for _, equipSuit := range data.ActiveSuit {
		if equipSuit.SuitActiveFlag == 0 || !equipSuit.ClacAttr {
			continue
		}
		stageSuitConf := jsondata.GetSourceSoulEquipStageSuit(equipSuit.StageSuitIdx)
		if stageSuitConf == nil {
			continue
		}
		var subStageSuitConf *jsondata.SourceSoulEquipSubStageSuit
		for _, subStageSuit := range stageSuitConf.SubStageSuit {
			if subStageSuit.Idx == equipSuit.SubStageSuitIdx {
				subStageSuitConf = subStageSuit
				break
			}
		}
		if subStageSuitConf == nil {
			continue
		}
		for _, suitAttr := range subStageSuitConf.SuitAttrs {
			if !utils.IsSetBit(equipSuit.SuitActiveFlag, suitAttr.Idx) {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, suitAttr.Attrs)
		}
	}

	var maxStageSuitIdxSet = make(map[uint32]struct{})
	suitMaxIdxMap := data.SuitMaxIdxMap
	for _, v := range data.SuitMaxIdxMap {
		maxStageSuitIdxSet[v] = struct{}{}
	}

	for _, equipSuit := range data.ActiveSuit {
		if _, ok := maxStageSuitIdxSet[equipSuit.StageSuitIdx]; !ok {
			continue
		}
		stageSuitConf := jsondata.GetSourceSoulEquipStageSuit(equipSuit.StageSuitIdx)
		if stageSuitConf == nil {
			continue
		}
		var subStageSuitConf *jsondata.SourceSoulEquipSubStageSuit
		for _, subStageSuit := range stageSuitConf.SubStageSuit {
			if subStageSuit.Idx == equipSuit.SubStageSuitIdx {
				subStageSuitConf = subStageSuit
				break
			}
		}
		if subStageSuitConf == nil {
			continue
		}
		for _, suitAttr := range subStageSuitConf.SuitAttrs {
			key := utils.Make64(suitAttr.Type, suitAttr.Idx)
			if suitMaxIdx, ok := suitMaxIdxMap[key]; ok && suitMaxIdx == equipSuit.StageSuitIdx {
				engine.CheckAddAttrsToCalc(owner, calc, suitAttr.Attrs)
			}
		}
	}
}

func (s *SourceSoulEquipSys) updateSuitMaxIdxMap() {
	data := s.getData()
	var suitMaxIdxMap = make(map[uint64]uint32)
	for _, equipSuit := range data.ActiveSuit {
		stageSuitConf := jsondata.GetSourceSoulEquipStageSuit(equipSuit.StageSuitIdx)
		if stageSuitConf == nil {
			return
		}
		var subStageSuitConf *jsondata.SourceSoulEquipSubStageSuit
		for _, subStageSuit := range stageSuitConf.SubStageSuit {
			if subStageSuit.Idx == equipSuit.SubStageSuitIdx {
				subStageSuitConf = subStageSuit
				break
			}
		}
		if subStageSuitConf == nil {
			return
		}
		// 一次只激活一个
		newMaxId := stageSuitConf.Idx
		for _, suitAttrs := range subStageSuitConf.SuitAttrs {
			if !utils.IsSetBit(equipSuit.SuitActiveFlag, suitAttrs.Idx) {
				continue
			}
			key := utils.Make64(suitAttrs.Type, suitAttrs.Idx)
			maxId := suitMaxIdxMap[key]
			if maxId >= newMaxId {
				continue
			}
			suitMaxIdxMap[key] = newMaxId
			continue
		}
	}
	data.SuitMaxIdxMap = suitMaxIdxMap
}

func (s *SourceSoulEquipSys) TakeOff(pos uint32, send bool) (*pb3.ItemSt, error) {
	data := s.getData()
	owner := s.GetOwner()
	st := data.TakeOnEquip[pos]
	if st == nil {
		return nil, neterror.ParamsInvalidError("%d not can take off equip", pos)
	}
	availableCount := owner.GetSourceSoulBagAvailableCount()
	if availableCount == 0 {
		return nil, neterror.ParamsInvalidError("bag limit")
	}
	if !owner.AddItemPtr(st, false, pb3.LogId_LogSourceSoulEquipTakeOff) {
		return nil, neterror.ParamsInvalidError("bag limit")
	}
	delete(data.TakeOnEquip, pos)
	s.ForgetSkill(st)
	if send {
		s.SendProto3(8, 192, &pb3.S2C_8_192{
			Pos: pos,
		})
	}
	s.ResetSysAttr(attrdef.SaSourceSoulEquip)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSourceSoulEquipTakeOff, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", st.ItemId),
	})
	return st, nil
}

func (s *SourceSoulEquipSys) AfterComposeTakeReplace(equip *pb3.ItemSt, oldEquip *pb3.ItemSt, pos uint32) {
	s.ResetSysAttr(attrdef.SaSourceSoulEquip)
	if oldEquip != nil {
		s.ForgetSkill(oldEquip)
	}
	if equip != nil {
		s.LearnSkill(equip)
	}
	s.s2cInfo()
}

func calcSourceSoulEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiSourceSoulEquip)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*SourceSoulEquipSys)
	if sys == nil {
		return
	}
	sys.calcAttrs(calc)
}

func init() {
	RegisterSysClass(sysdef.SiSourceSoulEquip, func() iface.ISystem {
		return &SourceSoulEquipSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaSourceSoulEquip, calcSourceSoulEquipAttr)
	net.RegisterSysProtoV2(8, 191, sysdef.SiSourceSoulEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SourceSoulEquipSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(8, 192, sysdef.SiSourceSoulEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SourceSoulEquipSys).c2sTakeOff
	})
	net.RegisterSysProtoV2(8, 193, sysdef.SiSourceSoulEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SourceSoulEquipSys).c2sActiveSuit
	})
}
