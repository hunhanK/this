/**
 * @Author: lzp
 * @Date: 2025/3/4
 * @Desc:
**/

package actorsystem

import (
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"

	"github.com/gzjjyz/random"
)

type FairySpiritEquSys struct {
	Base
}

const FairySpiritRatio = 10000

func (s *FairySpiritEquSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FairySpiritEquSys) OnLogin() {
	s.s2cInfo()
}

func (s *FairySpiritEquSys) GetData(slot uint32) *pb3.FairyEquData {
	dataMap := s.GetBinaryData().FairyEquData
	if dataMap == nil {
		s.GetBinaryData().FairyEquData = make(map[uint32]*pb3.FairyEquData)
		dataMap = s.GetBinaryData().FairyEquData
	}

	data := dataMap[slot]
	if data == nil {
		dataMap[slot] = &pb3.FairyEquData{}
		data = dataMap[slot]
	}
	return data
}

func (s *FairySpiritEquSys) getPosData(slot, pos uint32) *pb3.FairyEquPosData {
	data := s.GetData(slot)
	if data.PosData == nil {
		data.PosData = make(map[uint32]*pb3.FairyEquPosData)
	}
	posData := data.PosData[pos]
	if posData == nil {
		data.PosData[pos] = &pb3.FairyEquPosData{Pos: pos}
		posData = data.PosData[pos]
	}
	if posData.StarAttrs == nil {
		posData.StarAttrs = make(map[uint32]uint32)
	}
	return posData
}

func (s *FairySpiritEquSys) s2cInfo() {
	binData := s.GetBinaryData()
	s.SendProto3(27, 70, &pb3.S2C_27_70{Data: binData.FairyEquData})
}

func (s *FairySpiritEquSys) s2cPosData(slot, pos uint32) {
	data := s.GetData(slot)

	msg := &pb3.S2C_27_83{
		Slot: slot,
		Pos:  pos,
	}
	if data.PosData != nil {
		msg.PosData = data.PosData[pos]
	}
	if data.EquData != nil {
		msg.Equip = data.EquData[pos]
	}
	s.SendProto3(27, 83, msg)
}

func (s *FairySpiritEquSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_27_71
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Pos, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Slot, req.Pos, req.Hdl)
	return nil
}

func (s *FairySpiritEquSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_27_72
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOff(req.Slot, req.Pos); err != nil {
		return err
	}
	s.takeOff(req.Slot, req.Pos)
	return nil
}

func (s *FairySpiritEquSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_27_73
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkSlotPosHasEquip(req.Slot, req.Pos) {
		return neterror.ParamsInvalidError("slot:%d, pos:%d not equip fairy spirit", req.Slot, req.Pos)
	}

	posData := s.getPosData(req.Slot, req.Pos)
	nextLv := posData.Lv + 1
	nextLvConf := jsondata.GetFairySpiritLvConf(req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("lv: %d is nil", nextLv)
	}
	if posData.BreakLv < nextLvConf.BreakLimit {
		return neterror.ConfNotFoundError("break not satisfy")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySpiritLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Lv = nextLv
	s.s2cPosData(req.Slot, req.Pos)
	s.updateEquipPower(req.Slot, req.Pos)
	s.SendProto3(27, 73, &pb3.S2C_27_73{Slot: req.Slot, Pos: req.Pos})
	s.afterLvUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySpiritLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  req.Slot,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})
	return nil
}

func (s *FairySpiritEquSys) c2sBreak(msg *base.Message) error {
	var req pb3.C2S_27_74
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkSlotPosHasEquip(req.Slot, req.Pos) {
		return neterror.ParamsInvalidError("slot:%d, pos:%d not equip fairy spirit", req.Slot, req.Pos)
	}

	posData := s.getPosData(req.Slot, req.Pos)
	nextLv := posData.BreakLv + 1
	nextLvConf := jsondata.GetFairySpiritBreakConf(req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("breakLv: %d is nil", nextLv)
	}
	if posData.Lv < nextLvConf.LvLimit {
		return neterror.ConfNotFoundError("lv not satisfy")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySpiritBreak}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.BreakLv = nextLv
	s.s2cPosData(req.Slot, req.Pos)
	s.updateEquipPower(req.Slot, req.Pos)
	s.SendProto3(27, 74, &pb3.S2C_27_74{Slot: req.Slot, Pos: req.Pos})
	s.afterBreakUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySpiritBreak, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  req.Slot,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})

	return nil
}

func (s *FairySpiritEquSys) c2sEvolve(msg *base.Message) error {
	var req pb3.C2S_27_75
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkSlotPosHasEquip(req.Slot, req.Pos) {
		return neterror.ParamsInvalidError("slot:%d, pos:%d not equip fairy spirit", req.Slot, req.Pos)
	}

	equip := s.getSlotPosEquip(req.Slot, req.Pos)
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	starConf := jsondata.GetFairySpiritStarConf(req.Pos, itemConf.Quality, itemConf.Stage)
	if starConf == nil {
		return neterror.ParamsInvalidError("pos:%d quality:%d not found config", req.Pos, itemConf.Quality)
	}

	posData := s.getPosData(req.Slot, req.Pos)
	if posData.Star >= starConf.StarLimit {
		return neterror.ParamsInvalidError("slot:%d, pos:%d star limit", req.Slot, req.Pos)
	}

	nextLv := posData.Star + 1
	nextLvConf := jsondata.GetFairySpiritStarLvConf(req.Pos, nextLv)
	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySpiritEvolve}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Star = nextLv

	attrLibs := jsondata.GetFairySpiritAttrLib(req.Pos, itemConf.Quality, itemConf.Stage)
	pool := new(random.Pool)
	for _, v := range attrLibs {
		if _, ok := posData.StarAttrs[v.Id]; ok {
			continue
		}
		pool.AddItem(v, v.Weight)
	}
	ret := pool.RandomOne()
	if attrLib, ok := ret.(*jsondata.FairySpiritEquAttr); ok {
		posData.StarAttrs[attrLib.Id] = attrLib.Value
	}

	s.s2cPosData(req.Slot, req.Pos)
	s.updateEquipPower(req.Slot, req.Pos)
	s.SendProto3(27, 75, &pb3.S2C_27_75{Slot: req.Slot, Pos: req.Pos})
	s.afterEvolveUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySpiritEvolve, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  req.Slot,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})

	return nil
}

func (s *FairySpiritEquSys) c2sAwaken(msg *base.Message) error {
	var req pb3.C2S_27_76
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkSlotPosHasEquip(req.Slot, req.Pos) {
		return neterror.ParamsInvalidError("slot:%d, pos:%d not equip fairy spirit", req.Slot, req.Pos)
	}

	posData := s.getPosData(req.Slot, req.Pos)
	nextLv := posData.AwakenLv + 1
	nextLvConf := jsondata.GetFairySpiritAwakenConf(req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("awakenLv: %d is nil", nextLv)
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySpiritAwaken}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.AwakenLv = nextLv
	s.s2cPosData(req.Slot, req.Pos)
	s.updateEquipPower(req.Slot, req.Pos)
	s.SendProto3(27, 76, &pb3.S2C_27_76{Slot: req.Slot, Pos: req.Pos})
	s.afterAwakenUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySpiritAwaken, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"slot":  req.Slot,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})

	return nil
}

func (s *FairySpiritEquSys) calcFairySpiritAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().FairyEquData
	for slot, data := range dataMap {
		for pos := range data.PosData {
			s.calcSlotPosAttr(slot, pos, calc)
		}
	}
}

func (s *FairySpiritEquSys) calcSlotPosAttr(slot, pos uint32, calc *attrcalc.FightAttrCalc) {
	posData := s.getPosData(slot, pos)
	equip := s.getSlotPosEquip(slot, pos)
	if equip == nil {
		return
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return
	}

	// 觉醒属性
	awakenConf := jsondata.GetFairySpiritAwakenConf(posData.Pos, posData.AwakenLv)
	var baseRatio, lRatio, bRatio, sRatio uint32
	if awakenConf != nil {
		baseRatio = awakenConf.BaseRatio
		lRatio = awakenConf.LvRatio
		bRatio = awakenConf.BreakRatio
		sRatio = awakenConf.StarRatio
	}

	// 等级属性
	lvConf := jsondata.GetFairySpiritLvConf(posData.Pos, posData.Lv)
	if lvConf != nil {
		engine.CheckAddAttrsRate(s.owner, calc, lvConf.Attrs, lRatio+FairySpiritRatio)
	}

	// 突破属性
	breakConf := jsondata.GetFairySpiritBreakConf(posData.Pos, posData.BreakLv)
	if breakConf != nil {
		engine.CheckAddAttrsRate(s.owner, calc, breakConf.Attrs, bRatio+FairySpiritRatio)
	}

	// 进化属性
	attrs := s.getStarAttrs(equip, posData)
	if attrs != nil {
		engine.CheckAddAttrsRate(s.owner, calc, attrs, sRatio+FairySpiritRatio)
	}

	// 基础属性
	baseRatio += FairySpiritRatio
	engine.CheckAddAttrsRate(s.owner, calc, itemConf.StaticAttrs, baseRatio)
	engine.CheckAddAttrsRate(s.owner, calc, itemConf.PremiumAttrs, baseRatio)
	engine.CheckAddAttrsRate(s.owner, calc, itemConf.SuperAttrs, baseRatio)
}

func (s *FairySpiritEquSys) calcFairySpiritSuitAttr(calc *attrcalc.FightAttrCalc) {
	doCalcSuit := func(data *pb3.FairyEquData) {
		// 按照装备品质大->小排序
		var itemList []*pb3.ItemSt
		for _, equip := range data.EquData {
			if equip == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(equip.GetItemId())
			if itemConf == nil {
				continue
			}
			itemList = append(itemList, equip)
		}
		sort.Slice(itemList, func(i, j int) bool {
			itemConf1 := jsondata.GetItemConfig(itemList[i].ItemId)
			itemConf2 := jsondata.GetItemConfig(itemList[j].ItemId)
			return itemConf1.Quality > itemConf2.Quality
		})

		suitMap := make(map[uint32][]*pb3.ItemSt)
		for id := range jsondata.FairySpiritSuitConfMgr {
			suitMap[id] = make([]*pb3.ItemSt, 0)
		}

		for _, equip := range data.EquData {
			if equip == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(equip.GetItemId())
			if itemConf == nil {
				continue
			}
			for id := range suitMap {
				if id <= itemConf.Quality {
					suitMap[id] = append(suitMap[id], equip)
				}
			}
		}

		for _, equipL := range suitMap {
			sort.Slice(equipL, func(i, j int) bool {
				conf1 := jsondata.GetItemConfig(itemList[i].ItemId)
				conf2 := jsondata.GetItemConfig(itemList[j].ItemId)
				return conf1.Stage > conf2.Stage
			})
		}

		length := len(itemList)
		for i := 1; i <= length; i++ {
			itemId := itemList[i-1].ItemId
			quality := jsondata.GetItemConfig(itemId).Quality
			equipL := suitMap[quality]
			if len(equipL) <= 0 {
				continue
			}
			item := equipL[i-1]
			stage := jsondata.GetItemConfig(item.ItemId).Stage

			suitConf := jsondata.GetFairySpiritSuitConf(quality, uint32(i), stage)
			if suitConf == nil {
				continue
			}

			// 套装效果
			engine.CheckAddAttrsToCalc(s.owner, calc, suitConf.Attrs)
			if suitConf.SkillId > 0 {
				s.owner.LearnSkill(suitConf.SkillId, suitConf.SkillLv, true)
			}
		}
	}

	dataMap := s.GetBinaryData().FairyEquData
	for _, data := range dataMap {
		doCalcSuit(data)
	}
}

func (s *FairySpiritEquSys) getStarAttrs(equip *pb3.ItemSt, posData *pb3.FairyEquPosData) jsondata.AttrVec {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return nil
	}

	var attrs jsondata.AttrVec
	for id, value := range posData.StarAttrs {
		attrConf := jsondata.GetFairySpiritAttrConf(posData.Pos, id)
		if attrConf == nil {
			continue
		}
		if itemConf.Quality < attrConf.QualityLimit {
			continue
		}
		if itemConf.Stage < attrConf.StageLimit {
			continue
		}
		attrs = append(attrs, &jsondata.Attr{Type: attrConf.Type, Value: value})
	}
	return attrs
}

func (s *FairySpiritEquSys) getSlotPosEquip(slot, pos uint32) *pb3.ItemSt {
	data := s.GetData(slot)
	if data.EquData == nil {
		return nil
	}
	if equip, ok := data.EquData[pos]; ok {
		return equip
	}
	return nil
}

func (s *FairySpiritEquSys) checkSlotPosHasEquip(slot, pos uint32) bool {
	data := s.GetData(slot)
	if data.EquData == nil {
		return false
	}
	_, ok := data.EquData[pos]
	return ok
}

func (s *FairySpiritEquSys) checkTakeOn(pos uint32, hdl uint64) error {
	equip := s.getFairySpirit(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	conf := jsondata.GetFairySpiritEquConf(pos)
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if conf.Type != itemConf.Type || conf.SubType != itemConf.SubType {
		return neterror.ParamsInvalidError("item not fairy spirit")
	}
	return nil
}

func (s *FairySpiritEquSys) checkTakeOff(slot, pos uint32) error {
	if !s.checkSlotPosHasEquip(slot, pos) {
		return neterror.ParamsInvalidError("slot:%d, pos:%d not equip fairy spirit", slot, pos)
	}
	return nil
}

func (s *FairySpiritEquSys) takeOn(slot, pos uint32, hdl uint64) {
	data := s.GetData(slot)
	if data.EquData == nil {
		data.EquData = make(map[uint32]*pb3.ItemSt)
	}
	if data.EquData[pos] != nil {
		s.takeOff(slot, pos)
	}

	equip := s.getFairySpirit(hdl)

	// 删除装备
	if !s.owner.RemoveFairySpiritItemByHandle(hdl, pb3.LogId_LogFairySpiritTakeOn) {
		return
	}

	// 穿戴装备
	data.EquData[pos] = equip

	s.s2cPosData(slot, pos)
	s.updateEquipPower(slot, pos)
	s.afterTakeOn()
	s.SendProto3(27, 71, &pb3.S2C_27_71{Slot: slot, Pos: pos})
}

func (s *FairySpiritEquSys) takeOff(slot, pos uint32) {
	data := s.GetData(slot)
	if data.EquData == nil {
		data.EquData = make(map[uint32]*pb3.ItemSt)
	}
	if data.EquData[pos] == nil {
		return
	}
	oldEquip := data.EquData[pos]
	data.EquData[pos] = nil
	if !engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    oldEquip.ItemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairySpiritTakeOff}) {
		return
	}
	s.afterTakeOff()
	s.resetEquipPower(slot, pos)
	s.s2cPosData(slot, pos)
	s.SendProto3(27, 72, &pb3.S2C_27_72{Slot: slot, Pos: pos})
}

func (s *FairySpiritEquSys) updateEquipPower(slot, pos uint32) {
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()

	equip := s.getSlotPosEquip(slot, pos)
	if equip == nil {
		return
	}

	// 装备战力
	s.calcSlotPosAttr(slot, pos, singleCalc)
	job := s.GetOwner().GetJob()
	power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(job))

	equip.Ext.Power = uint64(power)
}

func (s *FairySpiritEquSys) resetEquipPower(slot, pos uint32) {
	equip := s.getSlotPosEquip(slot, pos)
	if equip == nil {
		return
	}

	equip.Ext.Power = 0
	if bagSys := s.getFairySpiritEquBagSys(); bagSys != nil {
		bagSys.OnItemChange(equip, 0, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogFairySpiritTakeOff,
		})
	}
}

func (s *FairySpiritEquSys) getFairySpiritEquBagSys() *FairySpiritEquBagSys {
	if sys, ok := s.owner.GetSysObj(sysdef.SiFairySpiritEquBag).(*FairySpiritEquBagSys); ok {
		return sys
	}
	return nil
}

func (s *FairySpiritEquSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
	s.ResetSysAttr(attrdef.SaFairySpiritSuit)
	s.owner.TriggerQuestEventRange(custom_id.QttFairyEquipNum)
}

func (s *FairySpiritEquSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
	s.ResetSysAttr(attrdef.SaFairySpiritSuit)
	s.owner.TriggerQuestEventRange(custom_id.QttFairyEquipNum)
}

func (s *FairySpiritEquSys) afterLvUp() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
}

func (s *FairySpiritEquSys) afterBreakUp() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
}

func (s *FairySpiritEquSys) afterEvolveUp() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
}

func (s *FairySpiritEquSys) afterAwakenUp() {
	s.ResetSysAttr(attrdef.SaFairySpirit)
}

func (s *FairySpiritEquSys) getFairySpirit(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFairySpiritEquBag).(*FairySpiritEquBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func calcFairySpirit(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySpiritEqu).(*FairySpiritEquSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySpiritAttr(calc)
}

func calcFairySpiritSuit(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySpiritEqu).(*FairySpiritEquSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySpiritSuitAttr(calc)
}

// 任务统计
func fairySpiritTakeOnCount(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiFairySpiritEqu).(*FairySpiritEquSys)
	if !ok || !s.IsOpen() {
		return 0
	}
	dataMap := s.GetBinaryData().FairyEquData
	if dataMap == nil {
		return 0
	}
	var count uint32
	for _, data := range dataMap {
		if data.EquData == nil {
			return 0
		}
		for _, eData := range data.EquData {
			if eData != nil {
				count++
			}
		}
	}
	return count
}

func init() {
	RegisterSysClass(sysdef.SiFairySpiritEqu, func() iface.ISystem {
		return &FairySpiritEquSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairySpirit, calcFairySpirit)
	engine.RegAttrCalcFn(attrdef.SaFairySpiritSuit, calcFairySpiritSuit)

	engine.RegQuestTargetProgress(custom_id.QttFairyEquipNum, fairySpiritTakeOnCount)

	net.RegisterSysProto(27, 71, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sTakeOn)
	net.RegisterSysProto(27, 72, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sTakeOff)
	net.RegisterSysProto(27, 73, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sUpgrade)
	net.RegisterSysProto(27, 74, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sBreak)
	net.RegisterSysProto(27, 75, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sEvolve)
	net.RegisterSysProto(27, 76, sysdef.SiFairySpiritEqu, (*FairySpiritEquSys).c2sAwaken)
}
