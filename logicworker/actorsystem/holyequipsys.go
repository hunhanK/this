/**
 * @Author: lzp
 * @Date: 2025/3/10
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
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
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
)

const (
	HolyLv   = 1 // 强化大师-等级
	HolyStar = 2 // 强化大师-星级
)

var HolyEquipPosAttrAddRateMap = map[uint32]uint32{
	1:  attrdef.HolyEquipSuit1,
	2:  attrdef.HolyEquipSuit2,
	3:  attrdef.HolyEquipSuit3,
	4:  attrdef.HolyEquipSuit4,
	5:  attrdef.HolyEquipSuit5,
	6:  attrdef.HolyEquipSuit6,
	7:  attrdef.HolyEquipSuit7,
	8:  attrdef.HolyEquipSuit8,
	9:  attrdef.HolyEquipSuit9,
	10: attrdef.HolyEquipSuit10,
	11: attrdef.HolyEquipSuit11,
	12: attrdef.HolyEquipSuit12,
}

type HolyEquipSys struct {
	Base
}

func (s *HolyEquipSys) OnReconnect() {
}

func (s *HolyEquipSys) OnLogin() {
}

func (s *HolyEquipSys) OnOpen() {
}

func (s *HolyEquipSys) getData(id uint32) *pb3.HolyEquData {
	dataMap := s.GetBinaryData().HolyEquData
	if dataMap == nil {
		s.GetBinaryData().HolyEquData = make(map[uint32]*pb3.HolyEquData)
		dataMap = s.GetBinaryData().HolyEquData
	}
	data := dataMap[id]
	if data == nil {
		dataMap[id] = &pb3.HolyEquData{}
		data = dataMap[id]
	}
	return data
}

func (s *HolyEquipSys) getPosData(id, pos uint32) *pb3.HolyEquPosData {
	data := s.getData(id)
	if data.PosData == nil {
		data.PosData = make(map[uint32]*pb3.HolyEquPosData)
	}

	posData := data.PosData[pos]
	if posData == nil {
		posData = &pb3.HolyEquPosData{}
		data.PosData[pos] = posData
	}
	if posData.StarAttrs == nil {
		posData.StarAttrs = make(map[uint32]uint32)
	}
	return posData
}

func (s *HolyEquipSys) s2cIdData(id uint32) {
	data := s.getData(id)
	s.SendProto3(76, 0, &pb3.S2C_76_0{Id: id, Data: data})
}

func (s *HolyEquipSys) s2cPosData(id, pos uint32) {
	posData := s.getPosData(id, pos)
	s.SendProto3(76, 9, &pb3.S2C_76_9{
		Id:      id,
		Pos:     pos,
		PosData: posData,
	})
}

func (s *HolyEquipSys) c2sInfo(msg *base.Message) error {
	var req pb3.C2S_76_0
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	s.s2cIdData(req.Id)
	return nil
}

func (s *HolyEquipSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_76_1
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Id, req.Pos, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Id, req.Pos, req.Hdl)
	return nil
}

func (s *HolyEquipSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_76_2
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOff(req.Id, req.Pos); err != nil {
		return err
	}
	s.takeOff(req.Id, req.Pos)
	return nil
}

func (s *HolyEquipSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_76_3
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkIdPosHasEquip(req.Id, req.Pos) {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip holy", req.Id, req.Pos)
	}

	posData := s.getPosData(req.Id, req.Pos)
	nextLv := posData.Lv + 1
	nextLvConf := jsondata.GetHolyEquLvConf(req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("lv: %d is nil", nextLv)
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogHolyEquipLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Lv = nextLv
	s.s2cPosData(req.Id, req.Pos)
	s.SendProto3(76, 3, &pb3.S2C_76_3{Id: req.Id, Pos: req.Pos})
	s.afterLvUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogHolyEquipLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"id":    req.Id,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})
	s.GetOwner().TriggerQuestEvent(custom_id.QttHolyEquipUpgrade, 0, 1)
	return nil
}

func (s *HolyEquipSys) c2sEvolve(msg *base.Message) error {
	var req pb3.C2S_76_4
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	if !s.checkIdPosHasEquip(req.Id, req.Pos) {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip holy", req.Id, req.Pos)
	}

	posData := s.getPosData(req.Id, req.Pos)
	equip := posData.Equip
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemId: %d config not exits", equip.GetItemId())
	}
	starConf := jsondata.GetHolyEquStarConf(req.Id, req.Pos, itemConf.Quality)
	if starConf == nil {
		return neterror.ParamsInvalidError("pos:%d quality:%d not found config", req.Pos, itemConf.Quality)
	}
	if posData.Star >= starConf.StarLimit {
		return neterror.ParamsInvalidError("id:%d, pos:%d star limit", req.Id, req.Pos)
	}

	nextLv := posData.Star + 1
	nextLvConf := jsondata.GetHolyEquStarLvConf(req.Id, req.Pos, nextLv)

	for _, value := range req.ItemMap {
		item := s.owner.GetHolyEquipItemByHandle(value.GetKey())
		if item == nil {
			return neterror.ParamsInvalidError("item not exist, hdl:%d", value.GetKey())
		}
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if itemConf == nil {
			return neterror.ParamsInvalidError("item config not found, itemId:%d", item.ItemId)
		}
		if itemConf.Type != itemdef.ItemTypeHolyEquip {
			return neterror.ParamsInvalidError("item not holy equip, itemId:%d", itemConf.Id)
		}
		if uint32(item.Count) < value.GetValue() {
			return neterror.ParamsInvalidError("item count limit, itemId:%d", itemConf.Id)
		}
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogHolyEquipEvolve}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 获取额外概率加成
	addRate := s.getAddEvolveRate(req.ItemMap)

	// 扣除添加的道具
	for _, entry := range req.ItemMap {
		item := s.owner.GetHolyEquipItemByHandle(entry.Key)
		if !s.GetOwner().DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogHolyEquipEvolve) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	// 失败
	if !s.checkEvolveSuccess(nextLvConf.SuccRate+addRate, nextLvConf.MaxRate) {
		s.owner.SendTipMsg(tipmsgid.HolyEquipEvolutionLose)
		s.SendProto3(76, 4, &pb3.S2C_76_4{Result: false})
		return nil
	}

	posData.Star = nextLv

	attrLibs := jsondata.GetHolyEquAttrLibConf(req.Id, req.Pos, itemConf.Quality, itemConf.Stage)
	pool := new(random.Pool)
	for _, v := range attrLibs {
		if _, ok := posData.StarAttrs[v.Id]; ok {
			continue
		}
		pool.AddItem(v, v.Weight)
	}
	ret := pool.RandomOne()
	if attrLib, ok := ret.(*jsondata.HolyEquAttr); ok {
		posData.StarAttrs[attrLib.Id] = attrLib.Value
	}

	s.s2cPosData(req.Id, req.Pos)
	s.SendProto3(76, 4, &pb3.S2C_76_4{Result: true, Id: req.Id, Pos: req.Pos})
	s.afterEvolveUp()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogHolyEquipEvolve, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"id":    req.Id,
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})
	s.GetOwner().TriggerQuestEvent(custom_id.QttHolyEquipEvolve, 0, 1)
	return nil
}

func (s *HolyEquipSys) c2sSuitActive(msg *base.Message) error {
	var req pb3.C2S_76_5
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData(req.Id)
	oldLv := s.getHolyStrengthenLv(req.Id, req.Type)
	ntConf := jsondata.GetHolyStrengthenSuitNextConf(req.Type, oldLv)
	if ntConf == nil {
		return neterror.ParamsInvalidError("holy equip strengthen conf nil id:%d, type:%d", req.Id, req.Type)
	}

	suitLv := s.calcHolyStrengthenLv(req.Id, req.Type)
	if suitLv < ntConf.Lv {
		return nil
	}
	if req.Type == HolyLv {
		data.SuitLv = ntConf.Lv
	} else if req.Type == HolyStar {
		data.SuitStar = ntConf.Lv
	}

	s.s2cIdData(req.Id)
	s.SendProto3(76, 5, &pb3.S2C_76_5{Id: req.Id, Type: req.Type, Lv: ntConf.Lv})
	s.afterSuitActive()
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogHolySuitStrengthen, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"id":    req.Id,
			"type":  req.Type,
			"preLv": oldLv,
			"level": ntConf.Lv,
		}),
	})
	return nil
}

func (s *HolyEquipSys) c2sCompose(msg *base.Message) error {
	var req pb3.C2S_76_10
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	equip := s.getIdPosEquip(req.Id, req.Pos)
	if equip == nil {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip holy", req.Id, req.Pos)
	}
	composeConf := jsondata.GetHolyComposeConf(equip.ItemId)
	if composeConf == nil {
		return neterror.ParamsInvalidError("itemId:%d compose config nil", equip.ItemId)
	}

	// 消耗
	if !s.owner.ConsumeByConf(composeConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogHolyEquipCompose,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 获得新装备
	itemId := composeConf.NewItemId
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return nil
	}

	engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    itemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogHolyEquipCompose})

	bagSys, _ := s.owner.GetSysObj(sysdef.SiHolyEquipBag).(*HolyEquipBagSys)
	itemList := bagSys.GetItemListByItemId(itemId, 1)
	itemHdl := itemList[random.Interval(0, len(itemList)-1)]
	s.takeOn(req.Id, req.Pos, itemHdl)

	// 删除返回的旧装备
	consumes := jsondata.ConsumeVec{
		{Id: equip.ItemId, Count: 1},
	}
	s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogHolyEquipCompose})
	s.SendProto3(76, 10, &pb3.S2C_76_10{Id: req.Id, Pos: req.Pos})
	s.GetOwner().TriggerQuestEvent(custom_id.QttHolyEquipCompose, 0, 1)
	return nil
}

// 装配槽属性重算
func (s *HolyEquipSys) calcHolyEquipAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().HolyEquData
	for id, data := range dataMap {
		for pos := range data.PosData {
			s.calcIdPosAttr(id, pos, calc)
		}
	}
}

// 装备套装属性重算
func (s *HolyEquipSys) calcHolyEquipSuitAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().HolyEquData
	for _, data := range dataMap {
		s.calcIdHolyEquipSuitAttr(data, calc)
	}
}

// 装备强化大师属性重算
func (s *HolyEquipSys) calcHolyEquipMasterAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().HolyEquData
	for _, data := range dataMap {
		sConf1 := jsondata.GetHolyStrengthenSuitConf(HolyLv, data.SuitLv)
		if sConf1 != nil {
			engine.CheckAddAttrsToCalc(s.owner, calc, sConf1.Attrs)
		}
		sConf2 := jsondata.GetHolyStrengthenSuitConf(HolyStar, data.SuitStar)
		if sConf2 != nil {
			engine.CheckAddAttrsToCalc(s.owner, calc, sConf2.Attrs)
		}
	}
}

// 基础属性加成
func (s *HolyEquipSys) calcHolyEquipAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetBinaryData().HolyEquData
	for _, data := range dataMap {
		for pos, posData := range data.PosData {
			item := posData.Equip
			if item == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(item.GetItemId())
			if itemConf == nil {
				continue
			}
			addRateAttrId := HolyEquipPosAttrAddRateMap[pos]
			if addRateAttrId == 0 {
				continue
			}
			addRate := totalSysCalc.GetValue(addRateAttrId)
			if addRate == 0 {
				continue
			}
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, itemConf.StaticAttrs, uint32(addRate))
		}
	}
}

func (s *HolyEquipSys) calcIdHolyEquipSuitAttr(data *pb3.HolyEquData, calc *attrcalc.FightAttrCalc) {
	// 按照装备阶级大->小排序
	var itemList []*pb3.ItemSt
	for _, posData := range data.PosData {
		if posData.Equip == nil {
			continue
		}
		itemList = append(itemList, posData.Equip)
	}
	sort.Slice(itemList, func(i, j int) bool {
		itemConf1 := jsondata.GetItemConfig(itemList[i].ItemId)
		itemConf2 := jsondata.GetItemConfig(itemList[j].ItemId)
		return itemConf1.Stage > itemConf2.Stage
	})

	suitMap := make(map[uint32][]*pb3.ItemSt)
	for id := range jsondata.HolyEquSuitConfMgr {
		suitMap[id] = make([]*pb3.ItemSt, 0)
	}

	for _, posData := range data.PosData {
		equip := posData.Equip
		if equip == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(equip.GetItemId())
		if itemConf == nil {
			continue
		}
		for id := range suitMap {
			if id <= itemConf.Stage {
				suitMap[id] = append(suitMap[id], equip)
			}
		}
	}

	for _, equipL := range suitMap {
		sort.Slice(equipL, func(i, j int) bool {
			conf1 := jsondata.GetItemConfig(itemList[i].ItemId)
			conf2 := jsondata.GetItemConfig(itemList[j].ItemId)
			return conf1.Quality > conf2.Quality
		})
	}

	length := len(itemList)
	for i := 1; i <= length; i++ {
		itemId := itemList[i-1].ItemId
		stage := jsondata.GetItemConfig(itemId).Stage
		equipL := suitMap[stage]
		if len(equipL) <= 0 {
			continue
		}

		item := equipL[i-1]
		quality := jsondata.GetItemConfig(item.ItemId).Quality
		suitConf := jsondata.GetHolyEquSuitConf(stage, uint32(i), quality)
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

func (s *HolyEquipSys) calcIdPosAttr(id, pos uint32, calc *attrcalc.FightAttrCalc) {
	posData := s.getPosData(id, pos)
	equip := posData.Equip
	if equip == nil {
		return
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return
	}

	// 基础属性
	engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)
	engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.PremiumAttrs)
	engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.SuperAttrs)

	// 等级属性
	lvConf := jsondata.GetHolyEquLvConf(posData.Pos, posData.Lv)
	if lvConf != nil {
		engine.CheckAddAttrsToCalc(s.owner, calc, lvConf.Attrs)
	}

	// 进化属性
	attrs := s.getStarAttrs(equip, id, posData)
	if attrs != nil {
		engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
	}
}

func (s *HolyEquipSys) getStarAttrs(equip *pb3.ItemSt, id uint32, posData *pb3.HolyEquPosData) jsondata.AttrVec {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return nil
	}

	var attrs jsondata.AttrVec
	for attrId, value := range posData.StarAttrs {
		attrConf := jsondata.GetHolyEquAttrConf(id, posData.Pos, attrId)
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

func (s *HolyEquipSys) checkEvolveSuccess(rate, maxRate uint32) bool {
	if rate >= maxRate {
		return true
	}
	if !random.Hit(rate, maxRate) {
		return false
	}
	return true
}

func (s *HolyEquipSys) getHolyStrengthenLv(id, typ uint32) uint32 {
	data := s.getData(id)
	if typ == HolyLv {
		return data.SuitLv
	} else if typ == HolyStar {
		return data.SuitStar
	}
	return 0
}

func (s *HolyEquipSys) calcHolyStrengthenLv(id, typ uint32) uint32 {
	data := s.getData(id)
	var lv uint32
	if typ == HolyLv {
		for _, posData := range data.PosData {
			lv += posData.Lv
		}
	} else if typ == HolyStar {
		for _, posData := range data.PosData {
			lv += posData.Star
		}
	}
	return lv
}

func (s *HolyEquipSys) getAddEvolveRate(itemMap []*pb3.Key64Value) uint32 {
	var rate uint32
	for _, itemVal := range itemMap {
		itemSt := s.getHolyEquip(itemVal.Key)
		if itemSt == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(itemSt.GetItemId())
		if itemConf == nil {
			continue
		}
		rate = rate + itemConf.CommonField*itemVal.Value
	}
	return rate
}

func (s *HolyEquipSys) takeOn(id, pos uint32, hdl uint64) {
	posData := s.getPosData(id, pos)
	oldEquip := posData.Equip
	if oldEquip != nil {
		s.takeOff(id, pos)
	}

	equip := s.getHolyEquip(hdl)
	// 删除装备
	if !s.owner.RemoveHolyItemByHandle(hdl, pb3.LogId_LogHolyEquipTakeOn) {
		return
	}

	// 穿戴装备
	posData.Equip = equip
	posData.Pos = pos

	s.afterTakeOn()
	s.s2cPosData(id, pos)
	s.GetOwner().TriggerQuestEventRange(custom_id.QttHolyEquipTakeOn)
	s.SendProto3(76, 1, &pb3.S2C_76_1{Id: id, Pos: pos})
}

func (s *HolyEquipSys) takeOff(id, pos uint32) {
	posData := s.getPosData(id, pos)
	if posData.Equip == nil {
		return
	}

	oldEquip := posData.Equip
	posData.Equip = nil
	if !engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    oldEquip.ItemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogHolyEquipTakeOff}) {
		return
	}

	s.afterTakeOff()
	s.s2cPosData(id, pos)
	s.GetOwner().TriggerQuestEventRange(custom_id.QttHolyEquipTakeOn)
	s.SendProto3(76, 2, &pb3.S2C_76_2{Id: id, Pos: pos})
}

func (s *HolyEquipSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaHolyEquip)
	s.ResetSysAttr(attrdef.SaHolyEquipSuit)
}

func (s *HolyEquipSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaHolyEquip)
	s.ResetSysAttr(attrdef.SaHolyEquipSuit)
}

func (s *HolyEquipSys) afterLvUp() {
	s.ResetSysAttr(attrdef.SaHolyEquip)
}

func (s *HolyEquipSys) afterEvolveUp() {
	s.ResetSysAttr(attrdef.SaHolyEquip)
}

func (s *HolyEquipSys) afterSuitActive() {
	s.ResetSysAttr(attrdef.SaHolyEquipStrengthen)
}

func (s *HolyEquipSys) checkTakeOn(id, pos uint32, hdl uint64) error {
	equip := s.getHolyEquip(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemId:%d config not exist", equip.ItemId)
	}

	conf := jsondata.GetHolyEquConf(id)
	if conf == nil {
		return neterror.ParamsInvalidError("id:%d config not exist", id)
	}
	sLv := s.owner.GetSpiritLv()
	if sLv < conf.SpiritLvLimit {
		return neterror.ParamsInvalidError("id:%d spiritLv not satisfy", id)
	}
	if itemConf.Stage != conf.StageLimit {
		return neterror.ParamsInvalidError("id:%d stage not satisfy", id)
	}

	posConf := jsondata.GetHolyEquPosConf(pos)
	if posConf == nil {
		return neterror.ParamsInvalidError("pos:%d config not exist", pos)
	}
	if posConf.Type != itemConf.Type || posConf.SubType != itemConf.SubType {
		return neterror.ParamsInvalidError("item not holy equip")
	}

	return nil
}

func (s *HolyEquipSys) checkTakeOff(id, pos uint32) error {
	if !s.checkIdPosHasEquip(id, pos) {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip holy", id, pos)
	}
	return nil
}

func (s *HolyEquipSys) checkIdPosHasEquip(id, pos uint32) bool {
	posData := s.getPosData(id, pos)
	return posData.Equip != nil
}

func (s *HolyEquipSys) getIdPosEquip(id, pos uint32) *pb3.ItemSt {
	posData := s.getPosData(id, pos)
	return posData.Equip
}

func (s *HolyEquipSys) getHolyEquip(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiHolyEquipBag).(*HolyEquipBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *HolyEquipSys) chargeCheck(chargeId uint32) bool {
	id := jsondata.GetHolyEquIdByChargeId(chargeId)
	if id <= 0 {
		return false
	}
	data := s.getData(id)
	if len(data.GiftIds) > 0 && utils.SliceContainsUint32(data.GiftIds, chargeId) {
		return false
	}
	return true
}

func (s *HolyEquipSys) chargeBack(chargeId uint32) bool {
	id, gConf := jsondata.GetHolyGiftConf(chargeId)
	if id == 0 || gConf == nil {
		return false
	}
	data := s.getData(id)
	data.GiftIds = pie.Uint32s(data.GiftIds).Append(chargeId).Unique()

	engine.GiveRewards(s.owner, gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogHolyGiftBuy})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogHolyGiftBuy, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"id":     id,
			"giftId": gConf.GiftId,
		}),
	})
	s.s2cIdData(id)
	return true
}

func calcHolyEquip(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcHolyEquipAttr(calc)
}

func calcHolyEquipSuit(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcHolyEquipSuitAttr(calc)
}

func calcHolyEquipStrengthen(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcHolyEquipMasterAttr(calc)
}

func calcHolyEquipAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcHolyEquipAttrAddRate(totalSysCalc, calc)
}

// 任务统计
func holyEquipTakeOnCount(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	s, ok := actor.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys)
	if !ok || !s.IsOpen() {
		return 0
	}
	dataMap := s.GetBinaryData().HolyEquData
	var count uint32
	needStage := ids[0]
	for _, data := range dataMap {
		for _, posData := range data.PosData {
			if posData.Equip == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(posData.Equip.GetItemId())
			if itemConf.Stage >= needStage {
				count++
			}
		}
	}
	return count
}

func giftHolyChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	if s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys); ok && s.IsOpen() {
		if s.chargeCheck(conf.ChargeId) {
			return true
		}
	}
	return false
}

func giftHolyChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if s, ok := player.GetSysObj(sysdef.SiHolyEquip).(*HolyEquipSys); ok && s.IsOpen() {
		if s.chargeBack(conf.ChargeId) {
			return true
		}
	}
	return false
}

func init() {
	RegisterSysClass(sysdef.SiHolyEquip, func() iface.ISystem {
		return &HolyEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaHolyEquip, calcHolyEquip)
	engine.RegAttrCalcFn(attrdef.SaHolyEquipSuit, calcHolyEquipSuit)
	engine.RegAttrCalcFn(attrdef.SaHolyEquipStrengthen, calcHolyEquipStrengthen)

	engine.RegAttrAddRateCalcFn(attrdef.SaHolyEquip, calcHolyEquipAttrAddRate)

	engine.RegQuestTargetProgress(custom_id.QttHolyEquipTakeOn, holyEquipTakeOnCount)

	engine.RegChargeEvent(chargedef.HolyEquipGift, giftHolyChargeCheck, giftHolyChargeBack)

	net.RegisterSysProto(76, 0, sysdef.SiHolyEquip, (*HolyEquipSys).c2sInfo)
	net.RegisterSysProto(76, 1, sysdef.SiHolyEquip, (*HolyEquipSys).c2sTakeOn)
	net.RegisterSysProto(76, 2, sysdef.SiHolyEquip, (*HolyEquipSys).c2sTakeOff)
	net.RegisterSysProto(76, 3, sysdef.SiHolyEquip, (*HolyEquipSys).c2sUpgrade)
	net.RegisterSysProto(76, 4, sysdef.SiHolyEquip, (*HolyEquipSys).c2sEvolve)
	net.RegisterSysProto(76, 5, sysdef.SiHolyEquip, (*HolyEquipSys).c2sSuitActive)
	net.RegisterSysProto(76, 10, sysdef.SiHolyEquip, (*HolyEquipSys).c2sCompose)
}
