package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
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

	"github.com/gzjjyz/srvlib/utils"
)

type GodBeastSystem struct {
	Base
}

func (s *GodBeastSystem) GetData() *pb3.GodBeastData {
	beastData := s.GetBinaryData().GodBeastData
	if beastData == nil {
		s.GetBinaryData().GodBeastData = &pb3.GodBeastData{}
		beastData = s.GetBinaryData().GodBeastData
	}
	if beastData.GodBeastMap == nil {
		beastData.GodBeastMap = make(map[uint32]*pb3.GodBeastEntry)
	}
	if beastData.BatPosMap == nil {
		beastData.BatPosMap = make(map[uint32]uint32)
	}
	return beastData
}

func (s *GodBeastSystem) OnOpen() {
	s.checkDefaultOpenBatPos()
	s.S2CInfo()
}

func (s *GodBeastSystem) ResetAllAttrs() {
	s.ResetSysAttr(attrdef.SaDragonVein)
	s.ResetSysAttr(attrdef.SaGodBeast)
}

func (s *GodBeastSystem) OnAfterLogin() {
	s.S2CInfo()
}

func (s *GodBeastSystem) OnReconnect() {
	s.S2CInfo()
}

func (s *GodBeastSystem) S2CInfo() {
	s.SendProto3(58, 20, &pb3.S2C_58_20{
		GodBeastData: s.GetData(),
	})
}

func (s *GodBeastSystem) c2sInfo(_ *base.Message) error {
	s.S2CInfo()
	return nil
}

// 请求穿戴
func (s *GodBeastSystem) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_58_21
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	owner := s.GetOwner()

	err = s.takeOn(req.Id, req.Hdl, req.BasePower, true)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}
	return nil
}

// 请求一键穿戴
func (s *GodBeastSystem) c2sFastTakeOn(msg *base.Message) error {
	var req pb3.C2S_58_28
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	owner := s.GetOwner()
	for pos, hdl := range req.TakeOnMap {
		_, _, err := s.beforeCheckTakeOnItem(req.Id, pos, hdl)
		if err != nil {
			owner.LogError("err:%v", err)
			return err
		}
	}

	for _, hdl := range req.TakeOnMap {
		err = s.takeOn(req.Id, hdl, 0, false)
		if err != nil {
			owner.LogError("err:%v", err)
			return err
		}
	}

	entry, _, err := s.getGodBeast(req.Id)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 填充基础评分
	for pos, basePower := range req.PowerMap {
		val, ok := entry.PosMap[pos]
		if !ok {
			continue
		}
		val.BasePower = basePower
	}

	s.SendProto3(58, 28, &pb3.S2C_58_28{
		Id:    req.Id,
		Entry: entry,
	})
	return nil
}

// 请求卸下
func (s *GodBeastSystem) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_58_22
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	err = s.takeOff(req.Id, req.Pos)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	s.afterTakeOff(req.Id)
	s.SendProto3(58, 22, &pb3.S2C_58_22{
		Id:  req.Id,
		Pos: req.Pos,
	})
	return nil
}

// 请求一键卸装
func (s *GodBeastSystem) c2sFastTakeOff(msg *base.Message) error {
	var req pb3.C2S_58_23
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	owner := s.GetOwner()
	mgr := jsondata.GodBeastCommonConfMgr
	if mgr == nil {
		return neterror.ConfNotFoundError("not found god beast common conf")
	}

	count := s.GetOwner().GetGodBeastBagAvailableCount()
	if count < mgr.WarnBagMinNum {
		s.GetOwner().SendTipMsg(tipmsgid.TpGodBeastBagIsFull)
		return neterror.ParamsInvalidError("bag not can use available count")
	}

	entry, _, err := s.getGodBeast(req.Id)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 一键脱
	var posList []uint32
	for pos := range entry.PosMap {
		posList = append(posList, pos)
	}
	for _, pos := range posList {
		err := s.takeOff(req.Id, pos)
		if err != nil {
			owner.LogError("err:%v", err)
			return err
		}
	}

	s.afterTakeOff(req.Id)
	s.SendProto3(58, 23, &pb3.S2C_58_23{
		Id: req.Id,
	})
	return nil
}

// 请求强化
func (s *GodBeastSystem) c2sEnhance(msg *base.Message) error {
	var req pb3.C2S_58_24
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	beast, _, err := s.getGodBeast(req.Id)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	simpleGodBeastItemSt := beast.PosMap[req.Pos]
	if simpleGodBeastItemSt == nil {
		return neterror.ParamsInvalidError("not found pos %d", &req.Pos)
	}

	itemConf := jsondata.GetItemConfig(simpleGodBeastItemSt.ItemId)
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId %d not found conf", simpleGodBeastItemSt.ItemId)
	}

	equipmentConf, ok := jsondata.GetGodBeastEquipmentConf(itemConf.Quality)
	if !ok {
		return neterror.ConfNotFoundError("quality %d not found conf", itemConf.Quality)
	}

	// 等级校验
	lvConf := equipmentConf.EnhanceLvs[len(equipmentConf.EnhanceLvs)-1]
	if simpleGodBeastItemSt.EnhanceLv >= lvConf.Lv {
		s.owner.SendTipMsg(tipmsgid.TpGodBeastEnhanceIsFull)
		return neterror.ParamsInvalidError("enhanceLv is full, lv %d , conf Lv %d", simpleGodBeastItemSt.EnhanceLv, lvConf.Lv)
	}

	fullLvExp := jsondata.GetGodBeastEquipmentFullLvExp(equipmentConf.Quality)
	baseExp, curExp := s.getCurLvAllExp(simpleGodBeastItemSt)

	// 获取这些装备可以提供的经验
	offerExp, hdlList := s.getOfferExp(req.HdlMap, fullLvExp, baseExp+curExp)
	if len(hdlList) == 0 {
		s.owner.LogTrace("not found consumeVec")
		return nil
	}

	if !s.checkCanAttachExp2Enhance(hdlList) {
		return neterror.ParamsInvalidError("checkCanAttachExp2Enhance failed")
	}

	for _, hdl := range hdlList {
		count := req.HdlMap[hdl]
		for i := uint32(0); i < count; i++ {
			if !s.GetOwner().RemoveGodBeastItemByHandle(hdl, pb3.LogId_LogGodBeastEnhanceConsume) {
				s.owner.LogWarn("enhance hdl remove failed , hdl %d", hdl)
			}
		}
	}
	s.owner.TriggerQuestEvent(custom_id.QttGodBeastEnhance, 0, 1)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastEnhanceOpt, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Id),
		StrArgs: fmt.Sprintf("{\"pos\":%d}", req.Pos),
	})

	err = s.doUpLv(req.Id, itemConf.Quality, offerExp, simpleGodBeastItemSt)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	s.SendProto3(58, 24, &pb3.S2C_58_24{
		Id:     req.Id,
		Pos:    req.Pos,
		ItemSt: simpleGodBeastItemSt,
	})
	return nil
}

func (s *GodBeastSystem) doUpLv(godBeastId, itemQuality uint32, offerExp uint64, simpleGodBeastItemSt *pb3.SimpleGodBeastItemSt) error {
	lvConfMap, ok := jsondata.GetGodBeastEquipmentLvMgr(itemQuality)
	if !ok {
		return neterror.ConfNotFoundError("lvConfMap not found")
	}

	simpleGodBeastItemSt.Exp += uint32(offerExp)

	// 防止死循环 理论上不会死循环。 不用 for{} 来写
	for i := 0; i < len(lvConfMap); i++ {
		lvConf, ok := lvConfMap[simpleGodBeastItemSt.EnhanceLv+1]
		if !ok {
			// 没有下一级 经验置空
			simpleGodBeastItemSt.Exp = 0
			break
		}

		if simpleGodBeastItemSt.Exp < lvConf.ConsumeExp {
			break
		}

		simpleGodBeastItemSt.EnhanceLv += 1
		simpleGodBeastItemSt.Exp -= lvConf.ConsumeExp
	}

	// 上阵
	if godBeastId == 0 {
		return nil
	}

	data := s.GetData()
	var toBat bool
	for _, gId := range data.BatPosMap {
		if gId != godBeastId {
			continue
		}
		toBat = true
		break
	}
	if toBat {
		s.ResetAllAttrs()
	}
	return nil
}

// 请求解锁槽位
func (s *GodBeastSystem) c2sUnlockBatPos(msg *base.Message) error {
	var req pb3.C2S_58_25
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	data := s.GetData()
	owner := s.GetOwner()
	_, ok := data.BatPosMap[req.Idx]
	if ok {
		owner.LogTrace("already locked, req pos is %d", req.Idx)
		return nil
	}

	if req.Idx != uint32(len(data.BatPosMap)+1) {
		owner.LogTrace("not opened in order, req pos is %d", req.Idx)
		return nil
	}

	mgr := jsondata.GodBeastCommonConfMgr
	if mgr == nil {
		return neterror.ConfNotFoundError("GodBeastCommonConfMgr is nil")
	}

	posSt, ok := mgr.BatPosMgr[fmt.Sprintf("%d", req.Idx)]
	if !ok {
		return neterror.ConfNotFoundError("BatPosMgr pos %d is not exist", req.Idx)
	}

	// 境界等级校验
	if posSt.Circle > 0 && owner.GetCircle() < posSt.Circle {
		return neterror.ConsumeFailedError("circle %d , owner circle %d", posSt.Circle, owner.GetCircle())
	}

	if posSt.SponsorGiftId > 0 {
		return neterror.ConsumeFailedError("SponsorGift %d ,auto un lock", posSt.SponsorGiftId)
	}

	if len(posSt.Consume) != 0 && !owner.ConsumeByConf(posSt.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGodBeastUnlockBatPosConsume}) {
		return neterror.ConsumeFailedError("pos %d is ConsumeFailed", req.Idx)
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastUnlockBatPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Idx),
	})

	data.BatPosMap[posSt.Idx] = 0
	s.SendProto3(58, 25, &pb3.S2C_58_25{
		Idx: posSt.Idx,
	})
	return nil
}

// 请求上阵
func (s *GodBeastSystem) c2sToBatPos(msg *base.Message) error {
	var req pb3.S2C_58_26
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	data := s.GetData()
	owner := s.GetOwner()
	gId, ok := data.BatPosMap[req.Idx]
	if !ok || gId != 0 {
		owner.LogTrace("already to bat pos, req idx is %d", req.Idx)
		return nil
	}

	// 重复上阵
	if gId > 0 {
		return neterror.ParamsInvalidError("already to bat idx %d , old gId %d", req.Idx, gId)
	}

	entry, conf, err := s.getGodBeast(req.Id)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	if !entry.IsActive {
		return neterror.ParamsInvalidError("god beast %d un active", req.Id)
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastToBatPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Idx),
		StrArgs: fmt.Sprintf("{\"Idx\":%d}", req.Id),
	})

	data.BatPosMap[req.Idx] = entry.Id

	// 学技能
	for _, skill := range conf.Skills {
		owner.LearnSkill(skill.Id, skill.Lv, true)
	}

	s.SendProto3(58, 26, &pb3.S2C_58_26{
		Id:  req.Id,
		Idx: req.Idx,
	})
	s.ResetAllAttrs()
	return nil
}

// 请求下阵
func (s *GodBeastSystem) c2sDownBatPos(msg *base.Message) error {
	var req pb3.C2S_58_27
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	s.downBatPos(req.Idx)
	return nil
}

func (s *GodBeastSystem) checkDefaultOpenBatPos() {
	data := s.GetData()
	mgr := jsondata.GodBeastCommonConfMgr
	if mgr == nil {
		s.GetOwner().LogError("not found common mgr conf")
		return
	}
	for _, batPos := range mgr.BatPosMgr {
		_, ok := data.BatPosMap[batPos.Idx]
		if ok {
			continue
		}
		if len(batPos.Consume) != 0 {
			continue
		}
		if batPos.SponsorGiftId > 0 {
			continue
		}
		if batPos.Circle > 0 {
			continue
		}
		data.BatPosMap[batPos.Idx] = 0
	}
}

// 下阵
func (s *GodBeastSystem) downBatPos(idx uint32) {
	data := s.GetData()
	owner := s.GetOwner()
	gId, ok := data.BatPosMap[idx]
	if !ok || gId == 0 {
		owner.LogTrace("already down locked, req idx is %d", idx)
		return
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastDownBatPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(idx),
	})

	entry, conf, err := s.getGodBeast(gId)
	if err != nil {
		owner.LogError("err:%v", err)
	} else {
		// 遗忘技能
		for _, skill := range conf.Skills {
			owner.ForgetSkill(skill.Id, true, true, true)
		}
	}

	data.BatPosMap[idx] = 0
	s.SendProto3(58, 27, &pb3.S2C_58_27{
		Idx: idx,
	})

	if nil != entry.DragonVeinEquip {
		err := s.dragonVeinTakeOff(gId)
		if err != nil {
			s.GetOwner().LogError("take off dragon vein equip failed on beastId %d", gId)
		}
	}

	s.ResetAllAttrs()
}

// 获取神兽信息
func (s *GodBeastSystem) getGodBeast(godBeastId uint32) (*pb3.GodBeastEntry, *jsondata.GodBeastConfValue, error) {
	conf, ok := jsondata.GetGodBeastConf(godBeastId)
	if !ok {
		return nil, nil, neterror.ConfNotFoundError("god beast config is nil , id %d", godBeastId)
	}

	data := s.GetData()
	// 获取神兽
	entry, ok := data.GodBeastMap[godBeastId]
	if !ok || entry == nil {
		data.GodBeastMap[godBeastId] = &pb3.GodBeastEntry{
			Id:       godBeastId,
			IsActive: false,
		}
		entry = data.GodBeastMap[godBeastId]
	}
	if entry.PosMap == nil {
		entry.PosMap = make(map[uint32]*pb3.SimpleGodBeastItemSt)
	}
	return entry, conf, nil
}

// 前置检查装备
// pos 不传表示不校验
func (s *GodBeastSystem) beforeCheckTakeOnItem(godBeastId uint32, pos uint32, hdl uint64) (*jsondata.ItemConf, *pb3.ItemSt, error) {
	owner := s.GetOwner()
	st := owner.GetGodBeastItemByHandle(hdl)
	if st == nil {
		return nil, nil, neterror.ParamsInvalidError("not found item , hdl %d", hdl)
	}

	itemConf := jsondata.GetItemConfig(st.ItemId)
	if itemConf == nil {
		return nil, nil, neterror.ConfNotFoundError("not found item , id %d", st.ItemId)
	}

	if !itemdef.IsGodBeastEquip(itemConf.Type) {
		return nil, nil, neterror.ParamsInvalidError("item not equal, type %d", itemConf.Type)
	}

	if !itemdef.IsGodBeastEquipmentType(itemConf.SubType) {
		return nil, nil, neterror.ParamsInvalidError("SubType %d not god beast", itemConf.SubType)
	}

	if !owner.CheckItemCond(itemConf) {
		return nil, nil, neterror.ParamsInvalidError("item not reach use cond %d", itemConf.Id)
	}

	if pos > 0 && itemConf.SubType != pos {
		return nil, nil, neterror.ParamsInvalidError("item not equal, subType %d,pos %d", itemConf.SubType, pos)
	}

	_, godBeastConf, err := s.getGodBeast(godBeastId)
	if err != nil {
		owner.LogError("err:%v", err)
		return nil, nil, err
	}

	// 检查神兽品质 是否可以操作
	qualityConf := jsondata.GetGodBeastQualityConf(godBeastConf.Quality)
	if qualityConf == nil {
		return nil, nil, neterror.ConfNotFoundError("not found god beast quality conf , quality %d", godBeastConf.Quality)
	}
	if qualityConf.Quality != 0 && owner.GetCircle() < qualityConf.Quality {
		return nil, nil, neterror.ParamsInvalidError("circle %d < quality circle %d", owner.GetCircle(), qualityConf.Circle)
	}
	if qualityConf.CrossTimes != 0 && gshare.GetCrossAllocTimes() < qualityConf.CrossTimes {
		return nil, nil, neterror.ParamsInvalidError("crossTimes %d < quality crossTimes %d", gshare.GetCrossAllocTimes(), qualityConf.CrossTimes)
	}
	if qualityConf.MinOpenDay != 0 && gshare.GetOpenServerDay() < qualityConf.MinOpenDay {
		return nil, nil, neterror.ParamsInvalidError("openDay %d < quality minOpenDay %d", gshare.GetOpenServerDay(), qualityConf.MinOpenDay)
	}

	// 品质是否足够
	if uint32(len(godBeastConf.MinQualityList)) != itemdef.GetMaxGodBeastEquipmentCount() {
		return nil, nil, neterror.ConfNotFoundError("MinQualityList %v not enough", godBeastConf.MinQualityList)
	}

	idx := itemConf.SubType - 1
	minQuality := godBeastConf.MinQualityList[idx]
	// 装备最低品质校验
	if itemConf.Quality < minQuality {
		return nil, nil, neterror.ParamsInvalidError("itemConf.Quality %d < minQuality %d , idx %d", itemConf.Quality, minQuality, idx)
	}

	return itemConf, st, nil
}

// 穿
func (s *GodBeastSystem) takeOn(godBeastId uint32, itemHdl uint64, basePower uint32, sendProto bool) error {
	owner := s.GetOwner()

	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 前置校验
	itemConf, itemSt, err := s.beforeCheckTakeOnItem(godBeastId, 0, itemHdl)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	pos := itemConf.SubType

	// 有穿戴 先卸下
	sItemSt := entry.PosMap[pos]
	if sItemSt != nil {
		count := owner.GetGodBeastBagAvailableCount()
		if count == 0 {
			owner.SendTipMsg(tipmsgid.TpGodBeastBagIsFull)
			return nil
		}
		err := s.takeOff(godBeastId, pos)
		if err != nil {
			owner.LogError("err:%v", err)
			return err
		}
	}

	// 穿戴
	godBeastItemSt := itemSt.ToSimpleGodBeastItemSt()
	godBeastItemSt.BasePower = basePower
	entry.PosMap[pos] = godBeastItemSt
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastTakeOn, &pb3.LogPlayerCounter{
		NumArgs: uint64(godBeastId),
		StrArgs: fmt.Sprintf("{\"pos\":%d,\"itemId\":%d}", pos, itemSt.ItemId),
	})
	owner.DeleteItemPtr(itemSt, 1, pb3.LogId_LogGodBeastTakeOnConsume)

	// 穿戴后
	s.afterTakeOn(godBeastId)

	if sendProto {
		s.SendProto3(58, 21, &pb3.S2C_58_21{
			Id:     godBeastId,
			Pos:    pos,
			ItemSt: godBeastItemSt,
		})
	}
	return nil
}

// 穿之后
func (s *GodBeastSystem) afterTakeOn(godBeastId uint32) {
	data := s.GetData()
	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}

	// 装备没穿够 不进行处理
	if itemdef.GetMaxGodBeastEquipmentCount() != uint32(len(entry.PosMap)) {
		return
	}

	// 修改激活状态
	if !entry.IsActive {
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastActive, &pb3.LogPlayerCounter{
			NumArgs: uint64(godBeastId),
		})

		entry.IsActive = true
		s.SendProto3(58, 40, &pb3.S2C_58_40{
			Id:       godBeastId,
			IsActive: true,
		})
		s.owner.TriggerQuestEventRange(custom_id.QttGodBeastUnLockByQuality)
	}

	if !entry.IsActive {
		return
	}

	// 上阵
	var toBat bool
	for _, gId := range data.BatPosMap {
		if gId != godBeastId {
			continue
		}
		toBat = true
		break
	}
	if toBat {
		s.ResetAllAttrs()
	}
}

// 卸
func (s *GodBeastSystem) takeOff(godBeastId uint32, pos uint32) error {
	owner := s.GetOwner()
	count := owner.GetGodBeastBagAvailableCount()
	if count == 0 {
		owner.SendTipMsg(tipmsgid.TpGodBeastBagIsFull)
		return neterror.ParamsInvalidError("bag not can use available count")
	}

	_, ok := jsondata.GetGodBeastConf(godBeastId)
	if !ok {
		return neterror.ConfNotFoundError("god beast config is nil , id %d", godBeastId)
	}

	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	simpleGodBeastItemSt := entry.PosMap[pos]
	if simpleGodBeastItemSt == nil {
		return neterror.ParamsInvalidError("not found pos %d", pos)
	}
	itemSt := simpleGodBeastItemSt.ToItemSt()
	owner.AddItemPtr(itemSt, true, pb3.LogId_LogGodBeastTakeOffAward)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastTakeOff, &pb3.LogPlayerCounter{
		NumArgs: uint64(godBeastId),
		StrArgs: fmt.Sprintf("{\"itemId\":%d,\"pos\":%d}", simpleGodBeastItemSt.ItemId, pos),
	})
	delete(entry.PosMap, pos)
	return nil
}

// 卸之后
func (s *GodBeastSystem) afterTakeOff(godBeastId uint32) {
	data := s.GetData()
	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}

	// 装备穿够 不进行处理
	if itemdef.GetMaxGodBeastEquipmentCount() == uint32(len(entry.PosMap)) {
		return
	}

	// 修改激活状态
	if entry.IsActive {
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastCancelActive, &pb3.LogPlayerCounter{
			NumArgs: uint64(godBeastId),
		})
		entry.IsActive = false
		s.SendProto3(58, 40, &pb3.S2C_58_40{
			Id:       godBeastId,
			IsActive: false,
		})
	}

	// 下阵
	var pos uint32
	for idx, gId := range data.BatPosMap {
		if gId != godBeastId {
			continue
		}
		pos = idx
		break
	}
	if pos == 0 {
		return
	}
	s.downBatPos(pos)
}

func (s *GodBeastSystem) checkCanAttachExp2Enhance(hdlList []uint64) bool {
	for _, hdl := range hdlList {
		godBeastItemSt := s.owner.GetGodBeastItemByHandle(hdl)
		if godBeastItemSt == nil {
			s.owner.LogWarn("hdl %d not found item", hdl)
			return false
		}
		itemConf := jsondata.GetItemConfig(godBeastItemSt.ItemId)
		if itemConf == nil {
			return false
		}
		// 粉装以及以上不让参与强化
		if itemConf.Quality >= 7 {
			s.owner.LogWarn("hdl %d itemId is %d, un can attach exp enhance", hdl, itemConf.Id)
			return false
		}
	}
	return true
}

// 计算能提供多少经验
func (s *GodBeastSystem) getOfferExp(hdlMap map[uint64]uint32, fullLvExp uint64, curExp uint64) (uint64, []uint64) {
	owner := s.GetOwner()
	var diffExp uint64
	if fullLvExp > curExp {
		diffExp = fullLvExp - curExp
	}
	if diffExp == 0 {
		return 0, nil
	}

	// 背包消耗品
	// 神兽装备消耗品
	var hdlList []uint64
	var offerExp uint64
	for hdl, count := range hdlMap {
		if offerExp > diffExp {
			break
		}

		godBeastItemSt := s.owner.GetGodBeastItemByHandle(hdl)
		if godBeastItemSt == nil {
			owner.LogWarn("hdl %d not found item", hdl)
			return 0, nil
		}
		itemConf := jsondata.GetItemConfig(godBeastItemSt.ItemId)
		switch itemConf.Type {
		case itemdef.ItemTypeGodBeastEquipMaterials:
			var actualCount uint32
			for ; actualCount < count; actualCount++ {
				offerExp += uint64(itemConf.CommonField)
				if offerExp < diffExp {
					continue
				}
				break
			}
			hdlList = append(hdlList, hdl)
		case itemdef.ItemTypeGodBeastEquip:
			conf, ok := jsondata.GetGodBeastEquipmentConf(itemConf.Quality)
			if !ok {
				owner.LogWarn("quality %d not found conf", itemConf.Quality)
				return 0, nil
			}
			baseExp, oneExp := s.getCurLvAllExp(godBeastItemSt.ToSimpleGodBeastItemSt())
			rateExp := utils.CalcMillionRate64(int64(oneExp), int64(conf.Rate))
			offerExp += uint64(rateExp) + baseExp
			hdlList = append(hdlList, hdl)
		default:
			owner.LogWarn("item type %d not found", itemConf.Type)
		}

	}

	return offerExp, hdlList
}

// 当前所拥有的经验(包含历史等级的经验)
func (s *GodBeastSystem) getCurLvAllExp(st *pb3.SimpleGodBeastItemSt) (uint64, uint64) {
	itemConf := jsondata.GetItemConfig(st.ItemId)
	if itemConf == nil {
		s.GetOwner().LogWarn("itemId %d not found conf", st.ItemId)
		return 0, 0
	}

	conf, ok := jsondata.GetGodBeastEquipmentConf(itemConf.Quality)
	if !ok {
		s.owner.LogTrace("quality %d not found conf", itemConf.Quality)
		return 0, 0
	}
	var totalExp uint64
	for _, lvConf := range conf.EnhanceLvs {
		if lvConf.Lv >= st.EnhanceLv {
			break
		}
		totalExp += uint64(lvConf.ConsumeExp)
	}
	return uint64(conf.BaseExp), totalExp + uint64(st.Exp)
}

// 请求穿戴
func (s *GodBeastSystem) c2sDragonVeinTakeOn(msg *base.Message) error {
	var req pb3.C2S_58_51
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	err = s.dragonVeinTakeOn(req.Id, req.Hdl, req.BasePower)
	if err != nil {
		return err
	}
	return nil
}

func (s *GodBeastSystem) beforeCheckTakeOnItemDragonVein(godBeastId uint32, hdl uint64) (*jsondata.ItemConf, *pb3.ItemSt, error) {
	owner := s.GetOwner()
	st := owner.GetGodBeastItemByHandle(hdl)
	if st == nil {
		return nil, nil, neterror.ParamsInvalidError("not found item , hdl %d", hdl)
	}

	itemConf := jsondata.GetItemConfig(st.ItemId)
	if itemConf == nil {
		return nil, nil, neterror.ConfNotFoundError("not found item , id %d", st.ItemId)
	}

	if !itemdef.IsItemTypeDragonVeinEquip(itemConf.Type) {
		return nil, nil, neterror.ParamsInvalidError("item not equal, type %d", itemConf.Type)
	}

	if !s.owner.CheckItemCond(itemConf) {
		return nil, nil, neterror.ParamsInvalidError("item conf %d cond not meet, id", st.ItemId)
	}

	var isToBat bool
	data := s.GetData()
	for _, gid := range data.BatPosMap {
		if gid == godBeastId {
			isToBat = true
			break
		}
	}
	if !isToBat {
		return nil, nil, neterror.ParamsInvalidError("god beast not bat, id %d", godBeastId)
	}

	return itemConf, st, nil
}

func (s *GodBeastSystem) dragonVeinTakeOn(godBeastId uint32, itemHdl uint64, basePower uint32) error {
	owner := s.GetOwner()

	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		return err
	}

	// 前置校验
	_, itemSt, err := s.beforeCheckTakeOnItemDragonVein(godBeastId, itemHdl)
	if err != nil {
		return err
	}

	// 有穿戴 先卸下
	sItemSt := entry.DragonVeinEquip
	if sItemSt != nil {
		err := s.dragonVeinTakeOff(godBeastId)
		if err != nil {
			return err
		}
	}

	owner.DeleteItemPtr(itemSt, 1, pb3.LogId_LogDragonVeinEquipTakeOn)

	// 穿戴
	godBeastItemSt := itemSt.ToSimpleGodBeastItemSt()
	godBeastItemSt.BasePower = basePower
	entry.DragonVeinEquip = godBeastItemSt

	// 穿戴后
	s.afterDragonVeinTakeOn(godBeastId)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogDragonVeinEquipTakeOn, &pb3.LogPlayerCounter{
		NumArgs: uint64(godBeastId),
		StrArgs: fmt.Sprintf("{\"itemId\":%d}", itemSt.ItemId),
	})
	return nil
}

// 请求卸下
func (s *GodBeastSystem) c2sDragonVeinTakeOff(msg *base.Message) error {
	var req pb3.C2S_58_52
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	godBeastId := req.GetId()

	err = s.dragonVeinTakeOff(godBeastId)
	if err != nil {
		return err
	}

	return nil
}

func (s *GodBeastSystem) dragonVeinTakeOff(godBeastId uint32) error {
	owner := s.GetOwner()
	count := owner.GetGodBeastBagAvailableCount()
	if count == 0 {
		owner.SendTipMsg(tipmsgid.TpGodBeastBagIsFull)
		return neterror.ParamsInvalidError("bag not can use available count")
	}

	_, ok := jsondata.GetGodBeastConf(godBeastId)
	if !ok {
		return neterror.ConfNotFoundError("god beast config is nil , id %d", godBeastId)
	}

	entry, _, err := s.getGodBeast(godBeastId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	simpleGodBeastItemSt := entry.DragonVeinEquip
	if simpleGodBeastItemSt == nil {
		return neterror.ParamsInvalidError("not found equip")
	}

	entry.DragonVeinEquip = nil

	itemSt := simpleGodBeastItemSt.ToItemSt()
	owner.AddItemPtr(itemSt, true, pb3.LogId_LogDragonVeinEquipTakeOff)

	s.afterDragonVeinTakeOff(godBeastId)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGodBeastTakeOff, &pb3.LogPlayerCounter{
		NumArgs: uint64(godBeastId),
		StrArgs: fmt.Sprintf("{\"itemId\":%d}", simpleGodBeastItemSt.ItemId),
	})
	return nil
}

func (s *GodBeastSystem) afterDragonVeinTakeOff(godBeastId uint32) {
	s.SendProto3(58, 52, &pb3.S2C_58_52{
		Id: godBeastId,
	})
	s.ResetAllAttrs()
}

func (s *GodBeastSystem) afterDragonVeinTakeOn(godBeastId uint32) {
	entry, _, err := s.getGodBeast(godBeastId)
	if nil != err {
		return
	}
	s.SendProto3(58, 51, &pb3.S2C_58_51{
		Id:     godBeastId,
		ItemSt: entry.DragonVeinEquip,
	})
	s.ResetAllAttrs()
}

func (s *GodBeastSystem) FindDragonVeinEquipInSlot(findHdl uint64) (st *pb3.SimpleGodBeastItemSt, godBeastId uint32) {
	data := s.GetData()
	for _, entryId := range data.BatPosMap {
		entry, _, _ := s.getGodBeast(entryId)
		if nil == entry {
			continue
		}
		if nil == entry.DragonVeinEquip {
			continue
		}
		if entry.DragonVeinEquip.GetHandle() == findHdl {
			return entry.DragonVeinEquip, entryId
		}
	}

	return
}

// 重算属性
func calcGodBeastSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !s.IsOpen() {
		return
	}
	data := s.GetData()
	owner := s.GetOwner()

	var calcGodBeastItemStAttr = func(st *pb3.SimpleGodBeastItemSt, calc *attrcalc.FightAttrCalc) {
		itemConf := jsondata.GetItemConfig(st.ItemId)
		if itemConf == nil {
			return
		}

		// 基础属性
		engine.CheckAddAttrsToCalc(owner, calc, itemConf.StaticAttrs)

		// 装备星级属性
		engine.CheckAddAttrsToCalc(owner, calc, itemConf.PremiumAttrs)

		// 装备品质属性
		engine.CheckAddAttrsToCalc(owner, calc, itemConf.SuperAttrs)

		// 随机属性
		for _, attr := range st.Attrs {
			calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
		}

		// 强化属性
		if st.EnhanceLv == 0 {
			return
		}
		lvMgr, ok := jsondata.GetGodBeastEquipmentLvMgr(itemConf.Quality)
		if !ok {
			return
		}
		lvConf := lvMgr[st.EnhanceLv]
		if lvConf == nil {
			return
		}
		if lvConf.PosAttr != nil {
			attrVal := lvConf.PosAttr[fmt.Sprintf("%d", itemConf.SubType)]
			if attrVal != nil {
				engine.CheckAddAttrsToCalc(owner, calc, attrVal.AttrVec)
			}
		}
	}

	var calcGodBeastAttr = func(entry *pb3.GodBeastEntry, calc *attrcalc.FightAttrCalc) {
		// 神兽基础属性
		godBeastConf, ok := jsondata.GetGodBeastConf(entry.Id)
		if ok {
			engine.CheckAddAttrsToCalc(owner, calc, godBeastConf.BaseAttr)
		}
		// 装备属性
		for _, st := range entry.PosMap {
			calcGodBeastItemStAttr(st, calc)
		}
		// 技能属性
		for _, sk := range godBeastConf.Skills {
			engine.CheckAddAttrsToCalc(owner, calc, sk.Attrs)
		}
	}

	for _, gId := range data.BatPosMap {
		entry := data.GodBeastMap[gId]
		if entry == nil {
			continue
		}
		tmpCalc := &attrcalc.FightAttrCalc{}
		calcGodBeastAttr(entry, tmpCalc)
		calcGodBeastSysRateAttr(entry, tmpCalc)
		calc.AddCalc(tmpCalc)
	}
	calcAllGodBeastSysRateAttr(calc)
}

func calcGodBeastSysAttrRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !s.IsOpen() {
		return
	}
	data := s.GetData()
	addRate := totalSysCalc.GetValue(attrdef.GodRiderBaseAttrRate)

	var attrs jsondata.AttrVec
	for _, gId := range data.BatPosMap {
		entry := data.GodBeastMap[gId]
		if entry == nil {
			continue
		}
		// 神兽基础属性
		godBeastConf, ok := jsondata.GetGodBeastConf(entry.Id)
		if ok {
			attrs = append(attrs, godBeastConf.BaseAttr...)
		}

		// 装备属性
		for _, st := range entry.PosMap {
			itemConf := jsondata.GetItemConfig(st.ItemId)
			if itemConf == nil {
				continue
			}
			// 基础属性
			attrs = append(attrs, itemConf.StaticAttrs...)
		}
	}
	if addRate > 0 && len(attrs) > 0 {
		engine.CheckAddAttrsRateRoundingUp(s.GetOwner(), calc, attrs, uint32(addRate))
	}
}

func calcSaDragonVeinSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !s.IsOpen() {
		return
	}

	var calcDragonVeinItemStAttr = func(st *pb3.SimpleGodBeastItemSt, calc *attrcalc.FightAttrCalc) {
		itemConf := jsondata.GetItemConfig(st.ItemId)
		if itemConf == nil {
			return
		}

		// 基础属性
		engine.CheckAddAttrsToCalc(player, calc, itemConf.StaticAttrs)

		// 极品属性
		engine.CheckAddAttrsToCalc(player, calc, itemConf.PremiumAttrs)
	}

	data := s.GetData()
	for _, gId := range data.BatPosMap {
		entry := data.GodBeastMap[gId]
		if entry == nil {
			continue
		}
		if nil != entry.DragonVeinEquip {
			calcDragonVeinItemStAttr(entry.DragonVeinEquip, calc)
		}
	}
}

// 计算神兽单独加成属性
func calcGodBeastSysRateAttr(entry *pb3.GodBeastEntry, calc *attrcalc.FightAttrCalc) {
	calcDragonVeinRate := func() int64 {
		if nil == entry.DragonVeinEquip {
			return 0
		}
		itemConf := jsondata.GetItemConfig(entry.DragonVeinEquip.GetItemId())
		if nil == itemConf {
			return 0
		}
		return int64(itemConf.CommonField)
	}
	dragonVeinRate := calcDragonVeinRate()
	if value1 := calc.GetValue(attrdef.GodBeastDefAddRate) + dragonVeinRate; value1 > 0 {
		calc.AddValueByRate(attrdef.DefWu, value1)
		calc.AddValueByRate(attrdef.DefMo, value1)
		calc.AddValueByRate(attrdef.Defend, value1)
	}
	if value2 := calc.GetValue(attrdef.GodBeastHpAddRate) + dragonVeinRate; value2 > 0 {
		calc.AddValueByRate(attrdef.MaxHp, value2)
	}
	if value3 := calc.GetValue(attrdef.GodBeastAtkAddRate) + dragonVeinRate; value3 > 0 {
		calc.AddValueByRate(attrdef.AttackWu, value3)
		calc.AddValueByRate(attrdef.AttackMo, value3)
		calc.AddValueByRate(attrdef.Attack, value3)
	}
}

// 计算上阵所有神兽加成属性
func calcAllGodBeastSysRateAttr(calc *attrcalc.FightAttrCalc) {
	if value1 := calc.GetValue(attrdef.GodBeastAllDefAddRate); value1 > 0 {
		calc.AddValueByRate(attrdef.DefWu, value1)
		calc.AddValueByRate(attrdef.DefMo, value1)
		calc.AddValueByRate(attrdef.Defend, value1)
	}
	if value2 := calc.GetValue(attrdef.GodBeastAllHpAddRate); value2 > 0 {
		calc.AddValueByRate(attrdef.Hp, value2)
	}
	if value3 := calc.GetValue(attrdef.GodBeastAllAtkAddRate); value3 > 0 {
		calc.AddValueByRate(attrdef.AttackWu, value3)
		calc.AddValueByRate(attrdef.AttackMo, value3)
		calc.AddValueByRate(attrdef.Attack, value3)
	}
	if value4 := calc.GetValue(attrdef.GodBeastAllAttrAddRate); value4 > 0 {
		rate := float64(value4) / 10000
		calc.AddRateExcludeAttrType(rate, attrdef.GodBeastAllAttrAddRate)
	}
}

func onComposeGodBeastItem(player iface.IPlayer, args ...interface{}) {
	s := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !s.IsOpen() {
		return
	}
	owner := s.GetOwner()
	if len(args) != 2 {
		return
	}
	paramSt := args[0].(*itemdef.ItemParamSt)
	itemSts := args[1].([]*pb3.SimpleGodBeastItemSt) // 已经移除掉的装备
	itemSt := owner.GetGodBeastItemByHandle(paramSt.AddItemAfterHdl)

	// 表示合成失败 找不到最后的装备
	if itemSt == nil {
		owner.LogWarn("not found after item , hdl is %d", paramSt.AddItemAfterHdl)
		return
	}

	itemConf := jsondata.GetItemConfig(paramSt.ItemId)
	if itemConf == nil {
		owner.LogWarn("not found item conf, itemId is %d", paramSt.ItemId)
		return
	}

	var totalExp uint64
	for _, beastItemSt := range itemSts {
		_, offerExp := s.getCurLvAllExp(beastItemSt)
		//totalExp += baseExp
		totalExp += offerExp
	}

	simpleGodBeastItemSt := itemSt.ToSimpleGodBeastItemSt()
	err := s.doUpLv(0, itemConf.Quality, totalExp, simpleGodBeastItemSt)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	itemSt.Union1 = simpleGodBeastItemSt.EnhanceLv
	itemSt.Union2 = simpleGodBeastItemSt.Exp
	if sys, ok := player.GetSysObj(sysdef.SiGodBeastBag).(*GodBeastBagSystem); ok {
		sys.OnItemChange(itemSt, 1, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogGodBeastCompose,
			NoTips: true,
		})
	}

}

func onGodBeastAeReceiveSponsorGift(player iface.IPlayer, args ...interface{}) {
	s := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !s.IsOpen() {
		return
	}
	flag, err := player.GetPrivilege(privilegedef.EnumGodBeastUnlockBatPosX)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	if flag == 0 {
		return
	}
	data := s.GetData()
	mgr := jsondata.GodBeastCommonConfMgr
	if mgr == nil {
		return
	}
	for _, godBeastBatPos := range mgr.BatPosMgr {
		if godBeastBatPos.SponsorGiftId == 0 {
			continue
		}
		_, ok := data.BatPosMap[godBeastBatPos.Idx]
		if ok {
			return
		}
		if !utils.IsSetBit64(uint64(flag), godBeastBatPos.Idx) {
			continue
		}
		data.BatPosMap[godBeastBatPos.Idx] = 0
		s.SendProto3(58, 25, &pb3.S2C_58_25{
			Idx: godBeastBatPos.Idx,
		})
	}

}

func init() {
	RegisterSysClass(sysdef.SiGodBeast, func() iface.ISystem {
		return &GodBeastSystem{}
	})

	event.RegActorEvent(custom_id.AeComposeGodBeastItem, onComposeGodBeastItem)
	event.RegActorEventL(custom_id.AeReceiveSponsorGift, onGodBeastAeReceiveSponsorGift)

	engine.RegAttrCalcFn(attrdef.SaGodBeast, calcGodBeastSysAttr)
	engine.RegAttrCalcFn(attrdef.SaDragonVein, calcSaDragonVeinSysAttr)

	engine.RegAttrAddRateCalcFn(attrdef.SaGodBeast, calcGodBeastSysAttrRate)

	engine.RegQuestTargetProgress(custom_id.QttGodBeastUnLockByQuality, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) < 1 {
			return 0
		}
		quality := ids[0]
		s := actor.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
		if !s.IsOpen() {
			return 0
		}
		data := s.GetData()
		var count uint32
		for _, entry := range data.GodBeastMap {
			if !entry.IsActive {
				continue
			}
			godBeastConf, ok := jsondata.GetGodBeastConf(entry.Id)
			if !ok {
				continue
			}
			if godBeastConf.Quality >= quality {
				count += 1
			}
		}
		return count
	})
	net.RegisterSysProtoV2(58, 20, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sInfo
	})
	net.RegisterSysProtoV2(58, 21, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sTakeOn
	})
	net.RegisterSysProtoV2(58, 22, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sTakeOff
	})
	net.RegisterSysProtoV2(58, 23, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sFastTakeOff
	})
	net.RegisterSysProtoV2(58, 24, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sEnhance
	})
	net.RegisterSysProtoV2(58, 25, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sUnlockBatPos
	})
	net.RegisterSysProtoV2(58, 26, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sToBatPos
	})
	net.RegisterSysProtoV2(58, 27, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sDownBatPos
	})
	net.RegisterSysProtoV2(58, 28, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sFastTakeOn
	})
	net.RegisterSysProtoV2(58, 51, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sDragonVeinTakeOn
	})
	net.RegisterSysProtoV2(58, 52, sysdef.SiGodBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastSystem).c2sDragonVeinTakeOff
	})
}
