/**
 * @Author: yzh
 * @Date:
 * @Desc: 纪念品
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
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type MementoSys struct {
	Base
}

func (s *MementoSys) OnLogin() {
	s.reLearnSeriesSuitSkill(0)
	s.s2cInfo()
}

func (s *MementoSys) OnOpen() {
	s.s2cInfo()
}

func (s *MementoSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MementoSys) s2cInfo() {
	s.SendProto3(130, 1, &pb3.S2C_130_1{
		State: s.state(),
	})
}

func (s *MementoSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: coreNumData,
		StrArgs: string(bytes),
	})
}

func (s *MementoSys) state() *pb3.CollectedMementoState {
	state := s.GetBinaryData().CollectedMementoState
	if state == nil {
		state = &pb3.CollectedMementoState{
			QuantityCollected: map[uint32]*pb3.CollectedMementoSeriesState{},
		}
		s.GetBinaryData().CollectedMementoState = state
	}
	if state.QuantityCollected == nil {
		state.QuantityCollected = map[uint32]*pb3.CollectedMementoSeriesState{}
	}
	return state
}

func mementoConvertDebris(owner iface.IPlayer, itemId uint32, count int64) {
	memento, ok := jsondata.MementoKeyByItemIdMap[itemId]
	if !ok {
		return
	}

	var giveAwards jsondata.StdRewardVec
	for i := int64(0); i < count; i++ {
		giveAwards = append(giveAwards, jsondata.CopyStdRewardVec(memento.ConvertDebris)...)
	}

	if len(giveAwards) > 0 {
		engine.GiveRewards(owner, giveAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogMementoConvertDebris,
		})
	}
}

func (s *MementoSys) activeMemento(itemId uint32, count int64, autoActive bool) {
	owner := s.GetOwner()
	valList := jsondata.Memento2QuantityAndSeriesMap[itemId]
	if len(valList) != 2 {
		return
	}

	quantityId, seriesId := valList[0], valList[1]
	memento, ok := jsondata.MementoKeyByItemIdMap[itemId]
	if !ok {
		owner.LogWarn("not such memento(item id:%d)", itemId)
		return
	}

	// 开服天数不满足
	if memento.OpenDay != 0 && memento.OpenDay > gshare.GetOpenServerDay() {
		mementoConvertDebris(s.GetOwner(), itemId, count)
		return
	}

	// 系列不满足
	if !s.checkSeriesOpen(quantityId, seriesId) {
		mementoConvertDebris(s.GetOwner(), itemId, count)
		return
	}

	// 已经激活 直接转换碎片
	if s.getMemento(quantityId, seriesId, itemId) != nil {
		mementoConvertDebris(s.GetOwner(), itemId, count)
		return
	}

	// 自动激活
	if autoActive {
		count -= 1
	}

	// 非自动激活会强制消耗碎片激活
	if !autoActive && !s.owner.ConsumeByConf(memento.ActiveConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveMemento}) {
		owner.LogWarn("active consume not enough")
		return
	}

	item, err := s.initMementoItem(memento)
	if err != nil {
		owner.LogError(err.Error())
		return
	}

	s.LogPlayerBehavior(uint64(itemId), map[string]interface{}{
		"quantityId": quantityId,
		"seriesId":   seriesId,
	}, pb3.LogId_LogMementoActiveSuccess)

	seriesState := s.getSeriesCollected(quantityId, seriesId)
	seriesState.Mementos = append(seriesState.Mementos, item)

	// 转换碎片
	if count > 0 {
		s.convertDebris(itemId, count)
	}

	quantityCollected := s.getQuantityCollected(quantityId)
	quantityCollected.SeriesActiveStateMap[seriesId] = s.calcSeriesActiveState(quantityId, seriesId)
	quantityCollected.SeriesStarMap[seriesId] = s.calcSeriesStar(quantityId, seriesId)
	newSeriesId, minStar := s.calcNewSeriesStar(itemId)
	if newSeriesId > 0 {
		s.setNewSeriesMinStar(newSeriesId, minStar)
		s.owner.TriggerQuestEventRange(custom_id.QttMementoActiveSuitNum)
	}

	s.SendProto3(130, 4, &pb3.S2C_130_4{
		Quantity: quantityId,
		Series:   seriesId,
		Item:     item,
	})
	s.reCalcSeriesSuit(quantityId)
	s.owner.TriggerQuestEventRange(custom_id.QttMementoActive)
}

func (s *MementoSys) getQuantityCollected(quantityId uint32) *pb3.CollectedMementoSeriesState {
	state := s.state()
	seriesState, ok := state.QuantityCollected[quantityId]
	if !ok {
		state.QuantityCollected[quantityId] = &pb3.CollectedMementoSeriesState{}
		seriesState = state.QuantityCollected[quantityId]
	}
	if seriesState.SeriesCollected == nil {
		seriesState.SeriesCollected = map[uint32]*pb3.Items{}
	}
	if seriesState.SeriesStarMap == nil {
		seriesState.SeriesStarMap = map[uint32]uint32{}
	}
	if seriesState.SeriesStarOfLearnedSkillMap == nil {
		seriesState.SeriesStarOfLearnedSkillMap = map[uint32]uint32{}
	}
	if seriesState.SeriesSuitMap == nil {
		seriesState.SeriesSuitMap = map[uint32]*pb3.MementoSeriesSuit{}
	}
	if seriesState.SeriesActiveStateMap == nil {
		seriesState.SeriesActiveStateMap = map[uint32]bool{}
	}
	return seriesState
}
func (s *MementoSys) setNewSeriesMinStar(id, minStar uint32) {
	state := s.state()
	if state.NewSeriesMinStarMap == nil {
		state.NewSeriesMinStarMap = map[uint32]uint32{}
	}
	state.NewSeriesMinStarMap[id] = minStar
	s.SendProto3(130, 6, &pb3.S2C_130_6{
		NewSeriesId:   id,
		NewSeriesStar: minStar,
	})
}

func (s *MementoSys) getSeriesCollected(quantityId, seriesId uint32) *pb3.Items {
	quantityCollected := s.getQuantityCollected(quantityId)
	seriesState, ok := quantityCollected.SeriesCollected[seriesId]
	if !ok {
		quantityCollected.SeriesCollected[seriesId] = &pb3.Items{}
		seriesState = quantityCollected.SeriesCollected[seriesId]
	}
	return seriesState
}

func (s *MementoSys) upMementoLv(hdl uint64) {
	quantityId, seriesId, mementoItem, ok := s.getCollectedMementoItem(hdl)
	if !ok {
		s.GetOwner().LogWarn("memento item(handler:%d) not found", hdl)
		return
	}

	mementoConf, ok := jsondata.MementoKeyByItemIdMap[mementoItem.ItemId]
	if !ok {
		s.GetOwner().LogWarn("quantity:%d series:%d mementoConf(item id:%d) not found", quantityId, seriesId, mementoConf.ItemId)
		return
	}

	nextLv := mementoItem.Union1 + 1

	if nextLv >= uint32(len(mementoConf.LevelConfs)) {
		s.GetOwner().LogWarn("quantity:%d series:%d mementoConf(item id:%d) overflow max level", quantityId, seriesId, mementoConf.ItemId)
		return
	}

	nextLvConf := mementoConf.LevelConfs[nextLv]

	if nextLvConf.NeedJingjieLv > 0 {
		jingjieLv := s.owner.GetExtraAttrU32(attrdef.Circle)

		if jingjieLv < nextLvConf.NeedJingjieLv {
			s.GetOwner().LogWarn("boundary lv is not enough, need:%d current:%d", nextLvConf.NeedJingjieLv, jingjieLv)
			return
		}
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvMemento}) {
		s.GetOwner().LogWarn("consume not enough")
		return
	}

	s.LogPlayerBehavior(uint64(mementoItem.ItemId), map[string]interface{}{
		"quantityId": quantityId,
		"seriesId":   seriesId,
		"oldLv":      mementoItem.Union1,
		"newLv":      nextLvConf.Level,
	}, pb3.LogId_LogMementoUpLvSuccess)

	mementoItem.Union1 = nextLvConf.Level
	s.ResetSysAttr(attrdef.SaMemento)
	s.owner.TriggerQuestEventRange(custom_id.QttMementoLevelReach)

	s.SendProto3(130, 2, &pb3.S2C_130_2{
		Quantity: quantityId,
		Series:   seriesId,
		Item:     mementoItem,
	})
}

func (s *MementoSys) upMementoStar(hdl uint64, consumeMgr map[uint32]*pb3.MementoStarConsume) error {
	quantityId, seriesId, mementoItem, ok := s.getCollectedMementoItem(hdl)
	if !ok {
		err := neterror.ConfNotFoundError("memento item(handler:%d) not found", hdl)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	mementoConf, ok := jsondata.MementoKeyByItemIdMap[mementoItem.ItemId]
	if !ok {
		err := neterror.ConfNotFoundError("quantity:%d series:%d mementoConf(item id:%d) not found", quantityId, seriesId, mementoConf.ItemId)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	nextStar := mementoItem.Union2 + 1
	if nextStar >= uint32(len(mementoConf.StarConfs)) {
		err := neterror.ConfNotFoundError("quantity:%d series:%d mementoConf(item id:%d) overflow max star", quantityId, seriesId, mementoConf.ItemId)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	// 拿到下一星级的配制
	nextStarConf := mementoConf.StarConfs[nextStar]
	var finalConsume jsondata.ConsumeVec
	finalConsume = append(finalConsume, nextStarConf.Consume...)

	var checkMementoItemIdsContain = func(debrisList []*jsondata.MementoStarExtConsumeDebris, itemId uint32) bool {
		for _, debris := range debrisList {
			if debris.ItemId != itemId {
				continue
			}
			return true
		}
		return false
	}

	// 万能碎片
	var commonSpList = pie.Uint32s([]uint32{41030001, 41030002, 41030003})
	var checkExtConsumeDebrisCanUse = func(srcQuantityId uint32, debrisItemId uint32) error {
		if commonSpList.Contains(debrisItemId) {
			return nil
		}

		quantityConf, ok := jsondata.GetMementoQuantityConf(srcQuantityId)
		if !ok {
			err := neterror.ConfNotFoundError("quantityConf %d not found", srcQuantityId)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}

		mementoConf, ok := jsondata.MementoKeyByDebrisMap[debrisItemId]
		if !ok {
			err := neterror.ConfNotFoundError("Memento debrisItemId %d not found", debrisItemId)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}

		quantityAndSeries := jsondata.Memento2QuantityAndSeriesMap[mementoConf.ItemId]
		quantityId, seriesId := quantityAndSeries[0], quantityAndSeries[1]

		// 星级不满足
		mementoItemSt := s.getMemento(quantityId, seriesId, mementoConf.ItemId)
		if mementoItemSt == nil {
			err := neterror.ConfNotFoundError("mementoItemSt not found, itemId is %d", debrisItemId)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}

		if mementoItemSt.Union2 < quantityConf.StarLimit {
			err := neterror.ConfNotFoundError("Union2 %d < StarLimit %d", mementoItemSt.Union2, quantityConf.StarLimit)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}
		return nil
	}

	// 看下额外消耗
	var extConsume jsondata.ConsumeVec
	if len(nextStarConf.ExtConsumeVec) != len(consumeMgr) {
		err := neterror.ConfNotFoundError("need other consumeVec size is %d, c2s consumeVec size is %d", len(nextStarConf.ExtConsumeVec), len(consumeMgr))
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	for _, conf := range nextStarConf.ExtConsumeVec {
		c, ok := consumeMgr[conf.Id]
		if !ok {
			err := neterror.ConfNotFoundError("extConsume conf not found, id is %d", conf.Id)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}

		// 校验客户端参数
		var c2sTotal uint32
		for _, item := range c.Items {
			// 找不到
			if !checkMementoItemIdsContain(conf.MementoItemIds, item.ItemId) {
				err := neterror.ConfNotFoundError("mementoItemIds conf not found, itemId is %d", item.ItemId)
				s.GetOwner().LogWarn("err:%v", err)
				return err
			}

			// 不满足
			err := checkExtConsumeDebrisCanUse(quantityId, item.ItemId)
			if err != nil {
				return neterror.Wrap(err)
			}

			// 满足加入条件 等待消耗
			extConsume = append(extConsume, &jsondata.Consume{
				Id:    item.ItemId,
				Count: uint32(item.Count),
			})
			c2sTotal += uint32(item.Count)
		}

		if c2sTotal < conf.Total {
			err := neterror.ConfNotFoundError("c2sTotal %d conf.Total %d", c2sTotal, conf.Total)
			s.GetOwner().LogWarn("err:%v", err)
			return err
		}
	}

	finalConsume = append(finalConsume, extConsume...)

	if !s.owner.ConsumeByConf(finalConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpStarMemento}) {
		err := neterror.ConfNotFoundError("consume not enough")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	s.LogPlayerBehavior(uint64(mementoItem.ItemId), map[string]interface{}{
		"quantityId": quantityId,
		"seriesId":   seriesId,
		"oldStar":    mementoItem.Union2,
		"newStar":    nextStarConf.Star,
	}, pb3.LogId_LogMementoUpStarSuccess)

	mementoItem.Union2 = nextStarConf.Star
	// 重算总星级
	quantityCollected := s.getQuantityCollected(quantityId)
	quantityCollected.SeriesStarMap[seriesId] = s.calcSeriesStar(quantityId, seriesId)
	quantityCollected.SeriesActiveStateMap[seriesId] = s.calcSeriesActiveState(quantityId, seriesId)
	newSeriesId, minStar := s.calcNewSeriesStar(mementoItem.ItemId)
	if newSeriesId > 0 && minStar > 0 {
		s.setNewSeriesMinStar(newSeriesId, minStar)
		s.owner.TriggerQuestEventRange(custom_id.QttMementoActiveSuitNum)
	}
	s.owner.TriggerQuestEventRange(custom_id.QttMementoStarReach)
	s.reCalcSeriesSuit(quantityId)
	s.ResetSysAttr(attrdef.SaMemento)
	s.SendProto3(130, 3, &pb3.S2C_130_3{
		Quantity:      quantityId,
		Series:        seriesId,
		Item:          mementoItem,
		SeriesStarMap: quantityCollected.SeriesStarMap,
	})
	//s.s2cInfo()
	return nil
}

func (s *MementoSys) tryLearnSeriesSuitSkill(quantityId, seriesId, star uint32) {
	seriesConf, ok := jsondata.GetMementoSeriesConf(quantityId, seriesId)
	if !ok {
		s.GetOwner().LogWarn("quantity:%d series:%d not found", quantityId, seriesId)
		return
	}

	if len(seriesConf.StarConfs) == 0 {
		s.GetOwner().LogWarn("seriesConf.StarConf is empty")
		return
	}

	seriesState := s.getSeriesCollected(quantityId, seriesId)
	if len(seriesState.Mementos) < len(seriesConf.Mementos) {
		s.GetOwner().LogWarn("series collected not enough. need:%d, current:%d", len(seriesConf.Mementos), len(seriesState.Mementos))
		return
	}

	quantityCollected := s.getQuantityCollected(quantityId)
	starMap := quantityCollected.SeriesStarMap
	if starMap == nil {
		s.GetOwner().LogWarn("series star not enough. need:%d, current:%d", 0, star)
		return
	}

	minStar := starMap[seriesId]

	if minStar < star {
		s.GetOwner().LogWarn("series star not enough. need:%d, current:%d", minStar, star)
		return
	}

	var hitStarConf *jsondata.MementoSeriesStarConf
	for _, starConf := range seriesConf.StarConfs {
		if starConf.Star != star {
			continue
		}
		hitStarConf = starConf
	}

	if hitStarConf == nil {
		s.GetOwner().LogWarn("seriesConf.StarConf is nil")
		return
	}

	s.LogPlayerBehavior(uint64(star), map[string]interface{}{
		"QuantityId": quantityId,
		"SeriesId":   seriesId,
	}, pb3.LogId_LogMementoLearnSkillSuccess)

	quantityCollected.SeriesStarOfLearnedSkillMap[seriesId] = hitStarConf.Star

	s.owner.SendProto3(130, 5, &pb3.S2C_130_5{
		Quantity:   quantityId,
		Series:     seriesId,
		BelongStar: star,
	})
}

func (s *MementoSys) calcSeriesStar(quantityId, seriesId uint32) uint32 {
	seriesState := s.getSeriesCollected(quantityId, seriesId)

	// 拿到 json 的系列配制
	seriesConf, ok := jsondata.GetMementoSeriesConf(quantityId, seriesId)
	if !ok {
		s.GetOwner().LogWarn("quantity:%d series:%d not found", quantityId, seriesId)
		return 0
	}

	var minStar uint32
	// 没有全部激活系列套装 那么就是 0 星
	if len(seriesState.Mementos) < len(seriesConf.Mementos) {
		return minStar
	}

	//
	var initMinStarEnd bool
	for _, mementos := range seriesState.Mementos {
		if !initMinStarEnd && minStar == 0 {
			minStar = mementos.Union2
			initMinStarEnd = true
			continue
		}

		if minStar > mementos.Union2 {
			minStar = mementos.Union2
		}
	}

	return minStar
}

func (s *MementoSys) calcNewSeriesStar(itemId uint32) (newSeriesId uint32, minStar uint32) {
	if jsondata.MementoNewSeriesMap == nil {
		return
	}
	var newSeriesConf *jsondata.MementoNewSeriesConf
	for _, conf := range jsondata.MementoNewSeriesMap {
		if !pie.Uint32s(conf.ItemIds).Contains(itemId) {
			continue
		}
		newSeriesConf = conf
		break
	}

	if newSeriesConf == nil {
		return
	}

	// 拿到配制的最大星级
	var confMaxStar = uint32(0)
	for _, val := range newSeriesConf.StarAttrs {
		if val.MinStar > confMaxStar {
			confMaxStar = val.MinStar
		}
	}

	var m uint32 = 999
	for _, id := range newSeriesConf.ItemIds {
		valList := jsondata.Memento2QuantityAndSeriesMap[id]
		if len(valList) != 2 {
			return
		}
		quantityId, seriesId := valList[0], valList[1]
		itemSt := s.getMemento(quantityId, seriesId, id)
		if itemSt == nil {
			return
		}
		if m > itemSt.Union2 {
			m = itemSt.Union2
		}
	}
	if m == 999 {
		return
	}
	minStar = m
	if minStar > confMaxStar {
		minStar = confMaxStar
	}
	newSeriesId = newSeriesConf.SeriesId
	return
}

func (s *MementoSys) calcSeriesActiveState(quantityId, seriesId uint32) bool {
	seriesState := s.getSeriesCollected(quantityId, seriesId)

	// 拿到 json 的系列配制
	seriesConf, ok := jsondata.GetMementoSeriesConf(quantityId, seriesId)
	if !ok {
		s.GetOwner().LogWarn("quantity:%d series:%d not found", quantityId, seriesId)
		return false
	}

	// 没有全部激活系列套装 那么就是 0 星
	if len(seriesState.Mementos) < len(seriesConf.Mementos) {
		return false
	}
	return true
}

func (s *MementoSys) reCalcSeriesSuit(quantityId uint32) {
	// 获取已激活的系列
	quantityCollected := s.getQuantityCollected(quantityId)
	conf, ok := jsondata.GetMementoQuantityConf(quantityId)
	if !ok {
		return
	}

	activeStateMap := quantityCollected.SeriesActiveStateMap
	seriesStarMap := quantityCollected.SeriesStarMap
	suitMap := quantityCollected.SeriesSuitMap
	for _, suitConf := range conf.SeriesSuitList {
		suitId := suitConf.Id
		var canSuit = true
		for _, seriesId := range suitConf.SeriesIds {
			if !activeStateMap[seriesId] {
				canSuit = false
				break
			}
		}

		if !canSuit {
			continue
		}

		var minStar uint32 = 999
		for _, star := range seriesStarMap {
			if minStar < star {
				continue
			}
			minStar = star
		}

		suitStarLevel, ok := suitConf.LevelMap[minStar]
		if !ok {
			continue
		}

		suit, ok := suitMap[suitId]
		if !ok {
			suitMap[suitId] = &pb3.MementoSeriesSuit{}
			suit = suitMap[suitId]
		}

		suit.SeriesIds = suitConf.SeriesIds
		suit.SkillId = suitConf.SkillId
		suit.SkillLv = suitStarLevel.SkillLv
	}

	if len(suitMap) == 0 {
		return
	}

	s.reLearnSeriesSuitSkill(quantityId)
	s.SendProto3(130, 20, &pb3.S2C_130_20{
		SeriesSuitMap: suitMap,
		QuantityId:    quantityId,
	})
}

func (s *MementoSys) reLearnSeriesSuitSkill(quantityId uint32) {
	if quantityId == 0 {
		for _, quantityState := range s.state().QuantityCollected {
			for _, suit := range quantityState.SeriesSuitMap {
				s.GetOwner().LearnSkill(suit.SkillId, suit.SkillLv, true)
			}
		}
		return
	}
	quantityState := s.getQuantityCollected(quantityId)
	for _, suit := range quantityState.SeriesSuitMap {
		s.GetOwner().LearnSkill(suit.SkillId, suit.SkillLv, true)
	}
}

func (s *MementoSys) getCollectedMementoItem(hdl uint64) (uint32, uint32, *pb3.ItemSt, bool) {
	state := s.state()
	var (
		mementoItem          *pb3.ItemSt
		quantityId, seriesId uint32
	)
	for theQuantityId, quantityState := range state.QuantityCollected {
		for theSeriesId, seriesState := range quantityState.SeriesCollected {
			for _, item := range seriesState.Mementos {
				if item.Handle != hdl {
					continue
				}

				mementoItem = item
				quantityId = theQuantityId
				seriesId = theSeriesId
				break
			}

			if mementoItem != nil {
				break
			}
		}

		if mementoItem != nil {
			break
		}
	}

	if mementoItem == nil {
		return 0, 0, nil, false
	}

	return quantityId, seriesId, mementoItem, true
}

func (s *MementoSys) initMementoItem(memento *jsondata.Memento) (*pb3.ItemSt, error) {
	hdl, err := series.AllocSeries()
	if err != nil {
		s.GetOwner().LogError(err.Error())
		return nil, err
	}

	return &pb3.ItemSt{
		Handle: hdl,
		ItemId: memento.ItemId,
		Count:  1,
		Bind:   true,
		Union1: 1,
	}, nil
}

// 获取纪念品
func (s *MementoSys) getMemento(quantityId uint32, seriesId uint32, itemId uint32) *pb3.ItemSt {
	var itemSt *pb3.ItemSt
	seriesState := s.getSeriesCollected(quantityId, seriesId)
	for _, item := range seriesState.Mementos {
		if item.ItemId != itemId {
			continue
		}
		itemSt = item
		break
	}
	return itemSt
}

// 碎片转换
func (s *MementoSys) convertDebris(itemId uint32, count int64) {
	memento, ok := jsondata.MementoKeyByItemIdMap[itemId]
	if !ok {
		return
	}

	var giveAwards jsondata.StdRewardVec
	for i := int64(0); i < count; i++ {
		giveAwards = append(giveAwards, jsondata.CopyStdRewardVec(memento.ConvertDebris)...)
	}

	if len(giveAwards) > 0 {
		engine.GiveRewards(s.GetOwner(), giveAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogMementoConvertDebris,
		})
	}
}

// todo 需要优化
func calcMementoSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiMemento).(*MementoSys)
	if !sys.IsOpen() {
		return
	}

	state := sys.state()

	var calcMementoNewSeriesBaseAttr = func() int64 {
		for id, minStar := range state.NewSeriesMinStarMap {
			seriesConf := jsondata.MementoNewSeriesMap[id]
			if seriesConf == nil {
				continue
			}
			attrs := seriesConf.StarAttrs[minStar]
			if attrs == nil {
				continue
			}
			engine.CheckAddAttrsToCalc(player, calc, attrs.Attrs)
		}
		return calc.GetValue(attrdef.MementoSelfBaseAddRate)
	}

	addRate := calcMementoNewSeriesBaseAttr()

	var calMementoSeriesAttr = func(quantityId, seriesId uint32) {
		quantityCollectMgr := sys.getQuantityCollected(quantityId)
		seriesStarMgr := quantityCollectMgr.SeriesStarMap
		seriesActiveStateMap := quantityCollectMgr.SeriesActiveStateMap
		items := sys.getSeriesCollected(quantityId, seriesId)

		// 获取最低星级
		star := seriesStarMgr[seriesId]

		// 拿到对应的纪念品系列
		seriesConf, ok := jsondata.GetMementoSeriesConf(quantityId, seriesId)
		if !ok {
			player.LogWarn("quantity:%d series:%d not found", quantityId, seriesId)
			return
		}

		itemMap := map[uint32]*pb3.ItemSt{}
		for _, item := range items.Mementos {
			itemMap[item.ItemId] = item
		}

		// 拿到命中的套装星级
		var hitStarConf *jsondata.MementoSeriesStarConf
		for _, starConf := range seriesConf.StarConfs {
			if starConf.Star != star {
				continue
			}
			hitStarConf = starConf
			break
		}

		// 命中的套装星级 有属性 需要重新加一下 需要判断套装是否激活
		if hitStarConf != nil && len(hitStarConf.Attrs) > 0 && seriesActiveStateMap[seriesId] {
			engine.AddAttrsToCalc(player, calc, hitStarConf.Attrs)
		}

		// 单个纪念品的属性 需要加一下
		for _, mementoConf := range seriesConf.Mementos {
			item, ok := itemMap[mementoConf.ItemId]
			if !ok {
				continue
			}

			// 升级 拿一下升级的属性
			var lvConf *jsondata.MementoLvConf
			if item.Union1 < uint32(len(mementoConf.LevelConfs)) {
				lvConf = mementoConf.LevelConfs[item.Union1]
			}

			// 升星 拿一下升星的数额比例
			var starConf *jsondata.MementoStarConf
			if item.Union2 <= uint32(len(mementoConf.StarConfs)) {
				starConf = mementoConf.StarConfs[item.Union2]
			}

			// 升星属性没有找到 那么就按升级属性去累加即可
			if starConf == nil {
				player.LogWarn("not found star conf , init start conf")
				starConf = &jsondata.MementoStarConf{}
			}

			// 得到最终的属性提升
			// 计算公式 = 纪念品 升级后(或不升级)基础属性 + (  升级后(或不升级)基础属性 * 升星提升百分比 )
			var attrs jsondata.AttrVec
			if lvConf != nil {
				for i := range lvConf.Attrs {
					attrs = append(attrs, &jsondata.Attr{
						Type:           lvConf.Attrs[i].Type,
						Value:          lvConf.Attrs[i].Value + ((lvConf.Attrs[i].Value * (starConf.AttrsAddRate + uint32(addRate))) / 10000),
						Job:            lvConf.Attrs[i].Job,
						EffectiveLimit: lvConf.Attrs[i].EffectiveLimit,
					})
				}
			}
			if starConf != nil {
				for i := range starConf.Attrs {
					attrs = append(attrs, &jsondata.Attr{
						Type:           starConf.Attrs[i].Type,
						Value:          starConf.Attrs[i].Value,
						Job:            starConf.Attrs[i].Job,
						EffectiveLimit: starConf.Attrs[i].EffectiveLimit,
					})
				}
			}

			if len(attrs) > 0 {
				engine.AddAttrsToCalc(player, calc, attrs)
			}
		}
	}

	for quantityId := range state.QuantityCollected {
		quantityCollectMgr := sys.getQuantityCollected(quantityId)
		for seriesId := range quantityCollectMgr.SeriesCollected {
			calMementoSeriesAttr(quantityId, seriesId)
		}
	}
}

func (s *MementoSys) c2sUpLv(msg *base.Message) {
	var req pb3.C2S_130_1
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}
	s.upMementoLv(req.Hdl)
}

func (s *MementoSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_130_2
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	var mgr = make(map[uint32]*pb3.MementoStarConsume)
	for _, consume := range req.Consumes {
		mgr[consume.Id] = consume
	}

	err = s.upMementoStar(req.Hdl, mgr)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *MementoSys) c2sLearnSkill(msg *base.Message) {
	var req pb3.C2S_130_4
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}
	s.tryLearnSeriesSuitSkill(req.Quantity, req.Series, req.BelongStar)
}

func (s *MementoSys) c2sActive(msg *base.Message) {
	var req pb3.C2S_130_3
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}
	s.activeMemento(req.ItemId, 0, false)
	s.ResetSysAttr(attrdef.SaMemento)
}

func (s *MementoSys) checkSeriesOpen(quantityId uint32, seriesId uint32) bool {
	conf, ok := jsondata.GetMementoSeriesConf(quantityId, seriesId)
	if !ok {
		return false
	}

	// 开服天数满足
	if conf.OpenDay != 0 && conf.OpenDay <= gshare.GetOpenServerDay() {
		return true
	}

	if len(conf.OpenSeriesIds) != len(conf.OpenSeriesMinStars) {
		return false
	}

	// 没有前置条件
	if len(conf.OpenSeriesIds) == 0 {
		return true
	}

	state := s.state()
	seriesState, ok := state.QuantityCollected[quantityId]
	// 有前置条件但是没激活
	if !ok {
		return false
	}

	ok = true
	for idx, id := range conf.OpenSeriesIds {
		// 这个系列没激活 直接结束
		if seriesState.SeriesStarMap == nil {
			ok = false
			break
		}
		minStar := seriesState.SeriesStarMap[id]
		minStarConf := conf.OpenSeriesMinStars[idx]
		if minStar >= minStarConf {
			continue
		}
		ok = false
		break
	}
	return ok
}

// 使用纪念品
func handleUseItemMemento(player iface.IPlayer, param *miscitem.UseItemParamSt, _ *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	itemId, count := param.ItemId, param.Count
	obj := player.GetSysObj(sysdef.SiMemento)
	if obj == nil || !obj.IsOpen() {
		mementoConvertDebris(player, itemId, count)
		return true, true, count
	}
	sys, ok := obj.(*MementoSys)
	if !ok {
		mementoConvertDebris(player, itemId, count)
		return true, true, count
	}
	sys.activeMemento(itemId, count, true)
	sys.ResetSysAttr(attrdef.SaMemento)
	return true, true, count
}

func handleQttMementoActive(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	if jsondata.MementoConfMap == nil {
		return 0
	}
	quality := ids[0]
	obj := actor.GetSysObj(sysdef.SiMemento)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*MementoSys)
	if !ok {
		return 0
	}
	var count uint32
	for _, v1 := range sys.state().QuantityCollected {
		for _, v2 := range v1.SeriesCollected {
			for _, memento := range v2.Mementos {
				mementoConf := jsondata.MementoKeyByItemIdMap[memento.ItemId]
				if mementoConf == nil {
					continue
				}
				if mementoConf.Quality >= quality {
					count++
				}
			}
		}
	}
	return count
}

func handleQttMementoLevelReach(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	obj := actor.GetSysObj(sysdef.SiMemento)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*MementoSys)
	if !ok {
		return 0
	}
	var maxLevel = uint32(0)
	for _, v1 := range sys.state().QuantityCollected {
		for _, v2 := range v1.SeriesCollected {
			for _, memento := range v2.Mementos {
				if memento.Union1 > maxLevel {
					maxLevel = memento.Union1
				}
			}
		}
	}
	return maxLevel
}

func handleQttMementoStarReach(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	obj := actor.GetSysObj(sysdef.SiMemento)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*MementoSys)
	if !ok {
		return 0
	}
	var maxStar = uint32(0)
	for _, v1 := range sys.state().QuantityCollected {
		for _, v2 := range v1.SeriesCollected {
			for _, memento := range v2.Mementos {
				if memento.Union2 >= maxStar {
					maxStar = memento.Union2
				}
			}
		}
	}
	return maxStar
}

func handleQttMementoActiveSuitNum(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	obj := actor.GetSysObj(sysdef.SiMemento)
	if obj == nil || !obj.IsOpen() {
		return 0
	}
	sys, ok := obj.(*MementoSys)
	if !ok {
		return 0
	}

	return uint32(len(sys.state().NewSeriesMinStarMap))
}

func init() {
	RegisterSysClass(sysdef.SiMemento, func() iface.ISystem {
		return &MementoSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemMemento, handleUseItemMemento)
	engine.RegQuestTargetProgress(custom_id.QttMementoActive, handleQttMementoActive)
	engine.RegQuestTargetProgress(custom_id.QttMementoLevelReach, handleQttMementoLevelReach)
	engine.RegQuestTargetProgress(custom_id.QttMementoStarReach, handleQttMementoStarReach)
	engine.RegQuestTargetProgress(custom_id.QttMementoActiveSuitNum, handleQttMementoActiveSuitNum)

	engine.RegAttrCalcFn(attrdef.SaMemento, calcMementoSysAttr)

	net.RegisterSysProto(130, 1, sysdef.SiMemento, (*MementoSys).c2sUpLv)
	net.RegisterSysProtoV2(130, 2, sysdef.SiMemento, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MementoSys).c2sUpStar
	})
	net.RegisterSysProto(130, 3, sysdef.SiMemento, (*MementoSys).c2sActive)
	net.RegisterSysProto(130, 4, sysdef.SiMemento, (*MementoSys).c2sLearnSkill)
}
