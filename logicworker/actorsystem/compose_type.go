package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/composedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/logworker"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

// 背包道具合成
func (sys *ComposeSys) composeItem(conf *jsondata.ComposeConf, autoBuy bool, composeCount uint32) (newItem *itemdef.ItemParamSt) {
	if composeCount <= 0 {
		return
	}
	actor := sys.owner

	consumes := jsondata.MergeConsumeVec(conf.Consume, conf.CoinConsume)

	isBind := false
	for _, consume := range conf.Consume {
		if sys.GetOwner().CheckItemBind(consume.Id, true) {
			isBind = true
		}
	}

	origSys := actor.GetSysObj(sysdef.SiOrigin).(*OriginSys)
	origSysState := origSys.GetPlayerState()
	var alreadyTakeOnOriginItems []*pb3.ItemSt
	for _, consume := range conf.Consume {
		itemConf := jsondata.GetItemConfig(consume.Id)
		if itemConf == nil {
			continue
		}

		if itemConf.Type != itemdef.ItemTypeOrigin {
			continue
		}

		for _, item := range origSysState.PosToEquipMap {
			if item.ItemId != itemConf.Id {
				continue
			}

			alreadyTakeOnOriginItems = append(alreadyTakeOnOriginItems, item)
			break
		}
	}

	newItem = &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  int64(composeCount),
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   isBind,
	}
	if !actor.CanAddItem(newItem, true) {
		actor.SendBagFullTip(conf.ItemId)
		return
	}

	for _, item := range alreadyTakeOnOriginItems {
		origSys.takeOff(item.ItemId, false)
	}

	if !actor.ConsumeRate(consumes, int64(composeCount), autoBuy, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
		for _, item := range alreadyTakeOnOriginItems {
			origSys.takeOn(item.Handle)
		}
		// 消耗失败
		return
	}

	res := actor.AddItem(newItem)
	if len(alreadyTakeOnOriginItems) > 0 {
		item, ok := actor.GetSysObj(sysdef.SiBag).(*BagSystem).RandomItem(newItem.ItemId)
		if ok {
			origSys.takeOn(item.Handle)
		}
	}
	sys.onItemComposed(conf, composeCount)
	sys.packSend(res)

	if conf.ComposeType != composedef.ComposeTypeItem {
		logworker.LogComposeWithConsume(actor, conf, consumes, composeCount)
	}
	return
}

// 背包道具合成后
func (sys *ComposeSys) onItemComposed(conf *jsondata.ComposeConf, composeCount uint32) {
	func() {
		itemConf := jsondata.GetItemConfig(conf.ItemId)
		if itemConf == nil {
			return
		}

		if itemConf.Type != itemdef.ItemGem {
			return
		}
		sys.owner.TriggerQuestEvent(custom_id.QttGemComposeTimes, 0, int64(composeCount))
	}()
}

// 装备合成
func (sys *ComposeSys) composeEquip(successRate uint32, conf *jsondata.ComposeConf, itemHandles []uint64, exItemHandle []uint64) {
	// 检查前端发过来的handle
	composeArea := conf.ConsumeTrends.Id
	exComposeArea := conf.ConsumeTrends.ExaId
	actor := sys.owner
	var bodyHandle uint64 // 挑出那个在身上的handle
	var bodyPos uint32
	var itemId uint32
	var resHandles []uint64
	var isBind bool
	for _, handle := range itemHandles {
		item := actor.GetItemByHandle(handle)

		if nil == item {
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			// 去找身上
			equips := sys.GetMainData().ItemPool.Equips
			for _, e := range equips {
				itemId = e.ItemId
				if bodyHandle == 0 && e.Handle == handle {
					bodyHandle = handle
					bodyPos = e.Pos
					isBind = e.Bind
					break
				}
			}
		} else {
			itemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(composeArea, itemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}
	var exItemId uint32
	for _, handle := range exItemHandle {
		item := actor.GetItemByHandle(handle)
		if nil == item {
			// 去找身上
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			equips := sys.GetMainData().ItemPool.Equips
			for _, e := range equips {
				if e.Handle == handle {
					bodyHandle = handle
					bodyPos = e.Pos
					exItemId = e.ItemId
					if !isBind {
						isBind = e.Bind
					}
					break
				}
			}
		} else {
			exItemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(exComposeArea, exItemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}
	isHaveBodyHandle := bodyHandle > 0
	// 身上的装备要跟目标装备同一位置
	itemconf := jsondata.GetItemConfig(conf.ItemId)
	if isHaveBodyHandle && (itemconf.SubType != bodyPos) {
		actor.SendTipMsg(tipmsgid.TpComposePosErr)
		return
	}
	// 判断成功率
	composeSuccess := random.Hit(successRate, 10000)
	// 是否有身上的装备
	if isHaveBodyHandle {
		sys.composeEquipBody(composeSuccess, conf, bodyHandle, resHandles, isBind)
	} else {
		sys.composeEquipBag(composeSuccess, conf, append(itemHandles, exItemHandle...), isBind)
	}
	sys.sendTips(composeSuccess, conf)
}

// 装备合成 - 身上装备
func (sys *ComposeSys) composeEquipBody(composeSuccess bool, conf *jsondata.ComposeConf, bodyHandle uint64, resHandles []uint64, isBind bool) {
	// 找到固定的消耗
	actor := sys.owner
	newItemId := conf.ItemId
	// 直接替换掉身上装备的itemID
	equipLs := sys.owner.GetMainData().ItemPool.Equips
	equipSys := actor.GetSysObj(sysdef.SiEquip).(*EquipSystem)

	statics := &pb3.LogCompose{}
	flag := false

	if composeSuccess {
		// 其余的不在身上的装备走正常消耗跟固定消耗一起
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 替换自己身上的装备
			for _, eq := range equipLs {
				if eq.Handle == bodyHandle {
					// 找到替换
					oldEq := &pb3.ItemSt{
						ItemId: eq.ItemId,
						Handle: eq.Handle,
						Count:  eq.Count,
						Pos:    eq.Pos,
						Bind:   eq.Bind,
					}
					eq.ItemId = newItemId
					eq.Bind = isBind
					equipSys.AfterTakeReplace(eq, oldEq, eq.Pos)
					break
				}
			}
			flag = true
			sys.packSend(true)
		}
	} else {
		// 合成失败 身上的脱下 然后一起消耗掉 走脱下逻辑
		for _, eq := range equipLs {
			if eq.Handle == bodyHandle {
				_, err := equipSys.TakeOff(eq.Pos, false)
				if err != nil {
					return
				}
				break
			}
		}
		resHandles = append(resHandles, bodyHandle)
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
			flag = true
		}
	}
	equipSys.S2CInfo()

	if flag {
		sys.owner.TriggerQuestEventRange(custom_id.QttCompositeEquipMultiCond, newItemId)
		sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipStage)
		logworker.LogCompose(actor, conf, statics)
	}
}

// 装备合成 - 背包
func (sys *ComposeSys) composeEquipBag(composeSuccess bool, conf *jsondata.ComposeConf, itemHandles []uint64, isBind bool) {
	actor := sys.owner
	newItemId := conf.ItemId
	fixConsume := conf.Consume
	statics := &pb3.LogCompose{}
	flag := false
	if composeSuccess {
		// 直接替换背包里面的就好了
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 生成新的装备到背包
			equip := &itemdef.ItemParamSt{
				ItemId: newItemId,
				Count:  1,
				Bind:   isBind,
			}
			if actor.AddItem(equip) {
				sys.packSend(true)
			}
			flag = true
		}
	} else { // 直接消耗
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
			flag = true
		}
	}

	if flag {
		sys.owner.TriggerQuestEventRange(custom_id.QttCompositeEquipMultiCond, newItemId)
		logworker.LogCompose(actor, conf, statics)
	}
}

// 装备合成 - 升阶升品升星
func (sys *ComposeSys) composeEquipQuality(successRate uint32, conf *jsondata.ComposeConf) {
	// 检查前端发过来的handle
	actor := sys.owner
	itemConf := jsondata.GetItemConfig(conf.ItemId)
	var bodyHandle uint64 // 挑出那个在身上的handle
	var isBind bool
	equips := sys.GetMainData().ItemPool.Equips
	for _, e := range equips {
		if e.Pos == itemConf.SubType { // 找到了
			bodyHandle = e.Handle
			isBind = e.Bind
			break
		}
	}
	if bodyHandle == 0 {
		actor.SendTipMsg(tipmsgid.TpComposeNeedBodyEquip)
		return
	}
	// 判断成功率
	composeSuccess := random.Hit(successRate, 10000)
	// 有身上的装备
	sys.composeEquipBody(composeSuccess, conf, bodyHandle, nil, isBind)
}

// 龙装合成
func (sys *ComposeSys) composeDragonEquItem(conf *jsondata.ComposeConf) {
	equipConsume := conf.ConsumeTrends

	bodyItemId := uint32(0)

	// 身上装备
	cItem := jsondata.GetItemConfig(conf.ItemId)
	for _, equipId := range equipConsume.Id {
		itemConf := jsondata.GetItemConfig(equipId)
		if sys.checkOn(cItem.Type, cItem.SubType, itemConf.Id) && bodyItemId == 0 {
			bodyItemId = itemConf.Id
			continue
		}
	}

	// 背包道具
	mergeConsume := jsondata.MergeConsumeVec(conf.CoinConsume, conf.Consume)

	if bodyItemId > 0 && bodyItemId != conf.ItemId {
		if !sys.owner.ConsumeByConf(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
			return
		}
		err := sys.updateBodyItem(cItem.Type, cItem, uint32(pb3.LogId_LogComposeItem))
		if err != nil {
			return
		}
		sys.packSend(true)
		logworker.LogComposeWithConsume(sys.owner, conf, mergeConsume, 1)
	} else {
		sys.composeItem(conf, false, 1)
	}
}

// 域灵合成
func (sys *ComposeSys) composeDomainSoul(conf *jsondata.ComposeConf) {
	equipConsume := conf.ConsumeTrends

	bodyItemId := uint32(0)

	// 身上装备
	cItem := jsondata.GetItemConfig(conf.ItemId)
	for _, equipId := range equipConsume.Id {
		itemConf := jsondata.GetItemConfig(equipId)
		if sys.checkOn(cItem.Type, cItem.SubType, itemConf.Id) && bodyItemId == 0 {
			bodyItemId = itemConf.Id
			continue
		}
	}

	if bodyItemId == 0 || bodyItemId == conf.ItemId {
		return
	}

	// 背包道具
	mergeConsume := jsondata.MergeConsumeVec(conf.CoinConsume, conf.Consume)

	if !sys.owner.ConsumeByConf(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	err := sys.updateBodyItem(cItem.Type, cItem, uint32(pb3.LogId_LogComposeItem))
	if err != nil {
		return
	}

	sys.packSend(true)
	logworker.LogComposeWithConsume(sys.owner, conf, mergeConsume, 1)
}

// 心决装备合成
func (sys *ComposeSys) composeEdictEquip(conf *jsondata.ComposeConf) {
	equipConsume := conf.ConsumeTrends

	bodyItemId := uint32(0)

	if len(equipConsume.Id) > 1 {
		return
	}

	mergeConsume := jsondata.CopyConsumeVec(conf.CoinConsume)
	var srcItemId uint32
	// 身上装备
	if len(equipConsume.Id) > 0 {
		srcItemId = equipConsume.Id[0]
	}

	var (
		checkSrcOk   bool
		needSrcCount uint32
	)

	cItem := jsondata.GetItemConfig(conf.ItemId)

	if sys.checkOn(cItem.Type, cItem.SubType, srcItemId) && bodyItemId == 0 {
		bodyItemId = srcItemId
	}

	for _, consume := range conf.Consume {
		if consume.Id == srcItemId {
			checkSrcOk, needSrcCount = true, consume.Count
			if bodyItemId > 0 && consume.Count > 1 {
				mergeConsume = append(mergeConsume, &jsondata.Consume{
					Type:       consume.Type,
					Id:         consume.Id,
					Count:      consume.Count - 1,
					CanAutoBuy: consume.CanAutoBuy,
					Job:        consume.Job,
				})
			}
			continue
		}
		mergeConsume = append(mergeConsume, consume)
	}

	if srcItemId > 0 && !checkSrcOk {
		return
	}

	if bodyItemId > 0 {
		if !sys.owner.ConsumeByConf(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return
		}
		err := sys.updateBodyItem(cItem.Type, cItem, uint32(pb3.LogId_LogComposeItem))
		if err != nil {
			return
		}
		sys.packSend(true)
		logworker.LogComposeWithConsume(sys.owner, conf, mergeConsume, 1)
	} else {
		bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
		if !ok {
			return
		}
		var (
			itemList []uint64
			isBind   bool
		)
		if srcItemId > 0 {
			itemList = bagSys.GetItemListByItemId(srcItemId, needSrcCount)
			if uint32(len(itemList)) != needSrcCount {
				return
			}
			for _, hdl := range itemList {
				if st := bagSys.FindItemByHandle(hdl); st.GetBind() {
					isBind = true
					break
				}
			}
		}
		newItem := &itemdef.ItemParamSt{
			ItemId: conf.ItemId,
			Count:  1,
			LogId:  pb3.LogId_LogComposeItem,
			Bind:   isBind,
		}
		if !sys.owner.CanAddItem(newItem, true) {
			sys.owner.SendBagFullTip(conf.ItemId)
			return
		}
		if !sys.owner.ConsumeByConf(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return
		}
		for _, needItemHdl := range itemList {
			if !bagSys.RemoveItemByHandle(needItemHdl, pb3.LogId_LogComposeItem) {
				sys.LogError("compose fail lose compose conf %d", conf.Id)
				return
			}
		}
		res := sys.owner.AddItem(newItem)
		sys.packSend(res)
		if srcItemId > 0 {
			logworker.LogComposeWithConsume(sys.owner, conf, jsondata.MergeConsumeVec(conf.Consume, conf.CoinConsume), 1)
		}
	}
}

// 助战装备(道具) 合成
// 得到消耗的道具
// 判断消耗的道具是否在身上
func (sys *ComposeSys) composeBattleHelpItem(conf *jsondata.ComposeConf) {
	// 检查前端发过来的handle
	actor := sys.owner
	var mergeConsume []*jsondata.Consume
	copy(mergeConsume, conf.CoinConsume)
	var bodyItemId uint32 // 挑出那个在身上的道具id
	cItem := jsondata.GetItemConfig(conf.ItemId)
	for _, consume := range conf.Consume {
		conItem := jsondata.GetItemConfig(consume.Id)
		if sys.checkOn(cItem.Type, cItem.SubType, conItem.Id) && bodyItemId == 0 { // 在身上
			bodyItemId = conItem.Id
			if consume.Count > 1 {
				mergeConsume = append(mergeConsume, &jsondata.Consume{
					Type:       consume.Type,
					Id:         consume.Id,
					Count:      consume.Count - 1,
					CanAutoBuy: consume.CanAutoBuy,
					Job:        consume.Job,
				})
			}
			continue
		}
		mergeConsume = append(mergeConsume, consume)
	}
	// modify 如果合成的目标id跟身上的id一样就走else
	if bodyItemId > 0 && bodyItemId != conf.ItemId {
		if !actor.ConsumeByConf(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
			return
		} // 消耗成功
		err := sys.updateBodyItem(cItem.Type, cItem, uint32(pb3.LogId_LogComposeItem))
		if err != nil {
			return
		}
		sys.packSend(true)
		logworker.LogComposeWithConsume(actor, conf, mergeConsume, 1)
	} else { // 不在身上直接转到合成道具
		sys.composeItem(conf, false, 1)
	}
}

// 剑装合成
func (sys *ComposeSys) composeFairySwordEquip(conf *jsondata.ComposeConf) {
	owner := sys.GetOwner()
	consumes := jsondata.MergeConsumeVec(conf.Consume, conf.CoinConsume)

	// 合成的道具
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  1,
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   false,
	}
	if !owner.CanAddItem(newItem, true) {
		owner.SendBagFullTip(conf.ItemId)
		return
	}

	// 消耗道具
	if !owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
		owner.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}

	res := owner.AddItem(newItem)
	sys.packSend(res)
	itemConf := jsondata.GetItemConfig(newItem.ItemId)
	sys.owner.TriggerQuestEvent(custom_id.QttFairySwordItemByType, itemConf.Type, 1)
	sys.owner.TriggerQuestEvent(custom_id.QttFairySwordComposeItem, itemConf.Quality, 1)
	logworker.LogComposeWithConsume(sys.owner, conf, consumes, 1)
}

// 武魂神饰
func (sys *ComposeSys) composeBattleSoulGodEquipMultiple(conf *jsondata.ComposeConf, composeCount uint32) {
	for i := composeCount; i > 0; i-- {
		sys.composeBattleSoulGodEquip(conf)
	}
}

// 武魂神饰合成
func (sys *ComposeSys) composeBattleSoulGodEquip(conf *jsondata.ComposeConf) {
	player := sys.owner
	var itemHandleList []uint64
	bagSys, ok := player.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
	if !ok {
		return
	}

	battleSoulGodEquipSys, ok := player.GetSysObj(sysdef.SiBattleSoulGodEquip).(*BattleSoulGodEquipSys)
	if !ok {
		return
	}

	var fixConsumeVec jsondata.ConsumeVec //固定消耗
	fixConsumeVec = append(fixConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
	itemEquipCount := make(map[uint32]uint32)
	for _, v := range conf.Consume {
		if v.Type == custom_id.ConsumeTypeItem && itemdef.IsBattleSoulGodEquipItem(jsondata.GetItemType(v.Id)) {
			itemEquipCount[v.Id] += v.Count
		} else {
			fixConsumeVec = append(fixConsumeVec, &jsondata.Consume{
				Type:       v.Type,
				Id:         v.Id,
				Count:      v.Count,
				CanAutoBuy: v.CanAutoBuy,
				Job:        v.Job,
			})
		}
	}

	for itemId, count := range itemEquipCount {
		if count == 0 {
			continue
		}
		var hasCount uint32
		needCount := count - hasCount
		hdlList := bagSys.GetItemListByItemId(itemId, needCount)
		hasCount += uint32(len(hdlList))

		if hasCount < count {
			return
		}
		itemHandleList = append(itemHandleList, hdlList...)
	}

	var removeHdlList []*pb3.ItemSt
	var maxBackLv uint64
	var maxBackLvItem *pb3.ItemSt
	var backRewardsVec []jsondata.StdRewardVec
	var allItemStList []*pb3.ItemSt
	for _, hdl := range itemHandleList {
		itemSt := bagSys.FindItemByHandle(hdl)
		if nil == itemSt {
			return
		}

		itemId := itemSt.ItemId

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}

		// 不是神饰道具 表示 hdl 有问题
		if !itemdef.IsBattleSoulGodEquipItem(itemConf.Type) {
			return
		}

		removeHdlList = append(removeHdlList, itemSt)

		allItemStList = append(allItemStList, itemSt)
		checkLv := utils.Make64(itemSt.GetUnion2(), itemSt.GetUnion1())
		if maxBackLv < checkLv {
			maxBackLv = checkLv
			maxBackLvItem = itemSt
		}
	}

	for _, itemSt := range allItemStList {
		if itemSt.GetHandle() == maxBackLvItem.GetHandle() {
			continue
		}
		backReward, _ := battleSoulGodEquipSys.packBackMaterial(itemSt)
		backRewardsVec = append(backRewardsVec, backReward)
	}

	if bagSys.AvailableCount() <= 0 {
		return
	}

	flag, removes := player.ConsumeByConfWithRet(fixConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		player.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}

	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}

	newHdl, err := series.AllocSeries()
	if err != nil {
		sys.LogError(err.Error())
		return
	}

	for _, removeItem := range removeHdlList {
		bagSys.RemoveItemByHandle(removeItem.GetHandle(), pb3.LogId_LogComposeItem)
		statics.Items[removeItem.GetItemId()] += uint32(removeItem.GetCount())
	}

	maxBackLvItem.ItemId = conf.ItemId
	maxBackLvItem.Handle = newHdl
	bagSys.AddItemPtr(maxBackLvItem, true, pb3.LogId_LogComposeItem)

	backRewards := jsondata.MergeStdReward(backRewardsVec...)
	if len(backRewards) > 0 {
		engine.GiveRewards(player, backRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogComposeItem})
	}

	sys.packSend(true)

	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}
	logworker.LogCompose(sys.owner, conf, statics)

}

// 龙脉合成
func (sys *ComposeSys) composeDragonVeinEquip(conf *jsondata.ComposeConf, itemHandleList []uint64) {
	player := sys.owner
	var needConsumeVec jsondata.ConsumeVec
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.Consume)...)
	consumeTrends := conf.ConsumeTrends

	var needItemIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.Id {
		needItemIdSet[id] = struct{}{}
	}

	if consumeTrends.Count != 1 {
		return
	}

	if uint32(len(itemHandleList)) != consumeTrends.Count { //身上穿的
		return
	}

	godBeastSys, ok := player.GetSysObj(sysdef.SiGodBeast).(*GodBeastSystem)
	if !ok {
		return
	}

	bodyItemSt, bodyPosId := godBeastSys.FindDragonVeinEquipInSlot(itemHandleList[0])
	if bodyPosId == 0 {
		return
	}

	itemId := bodyItemSt.ItemId
	_, ok = needItemIdSet[itemId]
	if !ok {
		return
	}

	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return
	}

	if !itemdef.IsItemTypeDragonVeinEquip(itemConf.Type) {
		return
	}

	entry, _, _ := godBeastSys.getGodBeast(bodyPosId)
	if nil == entry {
		return
	}

	bodyItemSt = entry.DragonVeinEquip
	if nil == bodyItemSt {
		return
	}

	flag, removes := player.ConsumeByConfWithRet(needConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		player.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}

	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}

	newHdl, err := series.AllocSeries()
	if err != nil {
		sys.LogError(err.Error())
		return
	}

	bodyItemSt.Handle = newHdl
	bodyItemSt.ItemId = conf.ItemId
	godBeastSys.afterDragonVeinTakeOn(bodyPosId)

	sys.packSend(true)

	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}
	logworker.LogCompose(sys.owner, conf, statics)

}

// 神兽装备合成
func (sys *ComposeSys) composeGodBeastItem(conf *jsondata.ComposeConf, itemHandleList []uint64) {
	owner := sys.GetOwner()
	var needConsumeVec jsondata.ConsumeVec
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.Consume)...)
	consumeTrends := conf.ConsumeTrends

	var needItemIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.Id {
		needItemIdSet[id] = struct{}{}
	}

	if uint32(len(itemHandleList)) != consumeTrends.Count {
		return
	}

	var bagItemList []*pb3.SimpleGodBeastItemSt
	for _, hdl := range itemHandleList {
		itemSt := owner.GetGodBeastItemByHandle(hdl)
		if itemSt == nil {
			return
		}

		itemId := itemSt.ItemId
		_, ok := needItemIdSet[itemId]
		if !ok {
			return
		}

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}

		// 不是神兽道具 表示 hdl 有问题
		if !itemdef.IsGodBeastBagItem(itemConf.Type) {
			return
		}

		// itemHandleList 默认让客户端只传入装备的hdl, 消耗的材料服务端自己从合成表读取, 做一个兜底 防止错误
		// 不是神兽装备
		if !itemdef.IsGodBeastEquip(itemConf.Type) {
			return
		}

		bagItemList = append(bagItemList, itemSt.ToSimpleGodBeastItemSt())
	}

	// 道具都符合
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  1,
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   false,
	}
	if !owner.CanAddItem(newItem, true) {
		owner.SendBagFullTip(conf.ItemId)
		return
	}

	// 移除道具
	flag, removes := owner.ConsumeByConfWithRet(needConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		owner.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}
	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}

	for _, st := range bagItemList {
		owner.RemoveGodBeastItemByHandle(st.Handle, pb3.LogId_LogComposeItem)
		statics.Items[st.GetItemId()] += uint32(st.GetCount())
	}
	res := owner.AddItem(newItem)
	sys.packSend(res)
	sys.owner.TriggerEvent(custom_id.AeComposeGodBeastItem, newItem, bagItemList)

	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}
	logworker.LogCompose(sys.owner, conf, statics)
}

// 仙灵灵装合成
func (sys *ComposeSys) composeFairySpiritEqu(conf *jsondata.ComposeConf, itemHandleList []uint64) {
	owner := sys.GetOwner()
	var needConsumeVec jsondata.ConsumeVec
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
	needConsumeVec = append(needConsumeVec, jsondata.CopyConsumeVec(conf.Consume)...)
	consumeTrends := conf.ConsumeTrends

	var needItemIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.Id {
		needItemIdSet[id] = struct{}{}
	}

	if uint32(len(itemHandleList)) != consumeTrends.Count {
		return
	}

	var bagItemList []*pb3.ItemSt
	for _, hdl := range itemHandleList {
		itemSt := owner.GetFairySpiritItemByHandle(hdl)
		if itemSt == nil {
			return
		}

		itemId := itemSt.ItemId
		_, ok := needItemIdSet[itemId]
		if !ok {
			return
		}

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}

		// 不是仙灵灵装道具 表示 hdl 有问题
		if !itemdef.IsFairySpiritItem(itemConf.Type) {
			return
		}

		bagItemList = append(bagItemList, itemSt)
	}

	// 道具都符合
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  1,
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   false,
	}
	if !owner.CanAddItem(newItem, true) {
		owner.SendBagFullTip(conf.ItemId)
		return
	}

	// 移除道具
	flag, removes := owner.ConsumeByConfWithRet(needConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		owner.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}
	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}

	for _, st := range bagItemList {
		owner.RemoveFairySpiritItemByHandle(st.Handle, pb3.LogId_LogComposeItem)
		statics.Items[st.GetItemId()] += uint32(st.GetCount())
	}
	res := owner.AddItem(newItem)
	sys.packSend(res)

	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}
	logworker.LogCompose(sys.owner, conf, statics)
}

// 飞剑玉符
func (sys *ComposeSys) composeFlyingSwordEquipMultiple(conf *jsondata.ComposeConf, composeCount uint32) {
	for i := composeCount; i > 0; i-- {
		sys.composeFlyingSwordEquip(conf)
	}
}

// 飞剑玉符合成
func (sys *ComposeSys) composeFlyingSwordEquip(conf *jsondata.ComposeConf) {
	player := sys.owner
	var itemHandleList []uint64

	var fixConsumeVec jsondata.ConsumeVec //固定消耗
	fixConsumeVec = append(fixConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
	itemEquipCount := make(map[uint32]uint32)
	for _, v := range conf.Consume {
		// 飞剑玉符
		if v.Type == custom_id.ConsumeTypeItem && itemdef.IsFlyingSwordEquipItem(jsondata.GetItemType(v.Id)) {
			itemEquipCount[v.Id] += v.Count
		} else {
			fixConsumeVec = append(fixConsumeVec, &jsondata.Consume{
				Type:       v.Type,
				Id:         v.Id,
				Count:      v.Count,
				CanAutoBuy: v.CanAutoBuy,
				Job:        v.Job,
			})
		}
	}

	bagSys, ok := player.GetSysObj(sysdef.SiFlyingSwordEquipBag).(*FlyingSwordEquipBagSys)
	if !ok {
		return
	}
	for itemId, count := range itemEquipCount {
		if count == 0 {
			continue
		}
		var hasCount uint32
		needCount := count - hasCount
		hdlList := bagSys.GetItemListByItemId(itemId, needCount)
		hasCount += uint32(len(hdlList))
		if hasCount < count {
			return
		}
		itemHandleList = append(itemHandleList, hdlList...)
	}

	var removeHdlList []*pb3.ItemSt
	var allItemStList []*pb3.ItemSt
	for _, hdl := range itemHandleList {
		itemSt := bagSys.FindItemByHandle(hdl)
		if nil == itemSt {
			return
		}
		itemId := itemSt.ItemId
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}
		// 不是飞剑玉符道具 表示 hdl 有问题
		if !itemdef.IsFlyingSwordEquipItem(itemConf.Type) {
			return
		}
		removeHdlList = append(removeHdlList, itemSt)
		allItemStList = append(allItemStList, itemSt)
	}

	if bagSys.AvailableCount() <= 0 {
		return
	}

	flag, removes := player.ConsumeByConfWithRet(fixConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		player.LogWarn("consume failed, conf id %d", conf.Id)
		return
	}

	// 移除和打点
	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}
	for _, removeItem := range removeHdlList {
		bagSys.RemoveItemByHandle(removeItem.GetHandle(), pb3.LogId_LogComposeItem)
		statics.Items[removeItem.GetItemId()] += uint32(removeItem.GetCount())
	}
	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}

	// 加新道具
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  int64(1),
		LogId:  pb3.LogId_LogComposeItem,
	}
	bagSys.AddItem(newItem)
	sys.packSend(true)

	logworker.LogCompose(sys.owner, conf, statics)
}

// 源魂装备合成
func (sys *ComposeSys) composeSourceSoulEquip(successRate uint32, conf *jsondata.ComposeConf, itemHandles []uint64, exItemHandle []uint64) {
	// 检查前端发过来的handle
	composeArea := conf.ConsumeTrends.Id
	exComposeArea := conf.ConsumeTrends.ExaId
	actor := sys.owner
	var bodyHandle uint64 // 挑出那个在身上的handle
	var bodyPos uint32
	var itemId uint32
	var resHandles []uint64
	var isBind bool
	for _, handle := range itemHandles {
		item := actor.GetSourceSoulEquipItemByHandle(handle)
		if nil == item {
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			// 去找身上
			data := sys.GetBinaryData().SourceSoulEquipData
			if data == nil || data.TakeOnEquip == nil {
				return
			}
			for pos, e := range data.TakeOnEquip {
				itemId = e.ItemId
				if bodyHandle == 0 && e.Handle == handle {
					bodyHandle = handle
					bodyPos = pos
					isBind = e.Bind
					break
				}
			}
		} else {
			itemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(composeArea, itemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}
	var exItemId uint32
	for _, handle := range exItemHandle {
		item := actor.GetSourceSoulEquipItemByHandle(handle)
		if nil == item {
			// 去找身上
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			// 去找身上
			data := sys.GetBinaryData().SourceSoulEquipData
			if data == nil || data.TakeOnEquip == nil {
				return
			}
			for pos, e := range data.TakeOnEquip {
				if e.Handle == handle {
					bodyHandle = handle
					bodyPos = pos
					exItemId = e.ItemId
					if !isBind {
						isBind = e.Bind
					}
					break
				}
			}
		} else {
			exItemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(exComposeArea, exItemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}
	isHaveBodyHandle := bodyHandle > 0
	// 身上的装备要跟目标装备同一位置
	itemConf := jsondata.GetItemConfig(conf.ItemId)
	if isHaveBodyHandle && (itemConf.SubType != bodyPos) {
		actor.SendTipMsg(tipmsgid.TpComposePosErr)
		return
	}
	// 判断成功率
	composeSuccess := random.Hit(successRate, 10000)
	// 是否有身上的装备
	if isHaveBodyHandle {
		sys.composeSourceSoulEquipBody(composeSuccess, conf, bodyHandle, resHandles, isBind)
	} else {
		sys.composeSourceSoulEquipBag(composeSuccess, conf, append(itemHandles, exItemHandle...), isBind)
	}
	sys.sendTips(composeSuccess, conf)
}

// 源魂装备合成 - 身上装备
func (sys *ComposeSys) composeSourceSoulEquipBody(composeSuccess bool, conf *jsondata.ComposeConf, bodyHandle uint64, resHandles []uint64, isBind bool) {
	// 找到固定的消耗
	actor := sys.owner
	newItemId := conf.ItemId
	// 直接替换掉身上装备的itemID
	data := sys.GetBinaryData().SourceSoulEquipData
	if data == nil || data.TakeOnEquip == nil {
		return
	}
	equipLs := data.TakeOnEquip
	equipSys := actor.GetSysObj(sysdef.SiSourceSoulEquip).(*SourceSoulEquipSys)

	statics := &pb3.LogCompose{}
	if composeSuccess {
		// 其余的不在身上的装备走正常消耗跟固定消耗一起
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 替换自己身上的装备
			for _, eq := range equipLs {
				if eq.Handle == bodyHandle {
					// 找到替换
					oldEq := &pb3.ItemSt{
						ItemId: eq.ItemId,
						Handle: eq.Handle,
						Count:  eq.Count,
						Pos:    eq.Pos,
						Bind:   eq.Bind,
					}
					eq.ItemId = newItemId
					eq.Bind = isBind
					equipSys.AfterComposeTakeReplace(eq, oldEq, eq.Pos)
					break
				}
			}
			sys.packSend(true)
		}
	} else {
		// 合成失败 身上的脱下 然后一起消耗掉 走脱下逻辑
		for _, eq := range equipLs {
			if eq.Handle == bodyHandle {
				_, err := equipSys.TakeOff(eq.Pos, false)
				if err != nil {
					return
				}
				break
			}
		}
		resHandles = append(resHandles, bodyHandle)
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
		}
	}
}

// 源魂装备合成 - 背包
func (sys *ComposeSys) composeSourceSoulEquipBag(composeSuccess bool, conf *jsondata.ComposeConf, itemHandles []uint64, isBind bool) {
	actor := sys.owner
	newItemId := conf.ItemId
	fixConsume := conf.Consume
	statics := &pb3.LogCompose{}
	if composeSuccess {
		// 直接替换背包里面的就好了
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 生成新的装备到背包
			equip := &itemdef.ItemParamSt{
				ItemId: newItemId,
				Count:  1,
				Bind:   isBind,
			}
			if actor.AddItem(equip) {
				sys.packSend(true)
			}
		}
	} else { // 直接消耗
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
		}
	}
}

// 血脉合成
func (sys *ComposeSys) composeBlood(conf *jsondata.ComposeConf, consumeHdl uint64, itemHandle, exItemHandle []uint64) {
	bloodSys, ok := sys.owner.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !bloodSys.IsOpen() {
		return
	}

	owner := sys.GetOwner()
	consumeItemSt := owner.GetBloodItemByHandle(consumeHdl)
	if consumeItemSt == nil {
		return
	}

	var consumeItemId uint32
	if len(conf.Consume) > 0 {
		consumeItemId = conf.Consume[0].Id
	}

	// 配置的固定消耗不满足
	if consumeItemId == 0 || consumeItemSt.ItemId != consumeItemId {
		return
	}

	consumeTrends := conf.ConsumeTrends
	if uint32(len(itemHandle)) != consumeTrends.Count {
		return
	}
	if uint32(len(exItemHandle)) != consumeTrends.ExaCount {
		return
	}

	var needItemIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.Id {
		needItemIdSet[id] = struct{}{}
	}

	var needItemExtIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.ExaId {
		needItemExtIdSet[id] = struct{}{}
	}

	var bagItemList []*pb3.ItemSt
	for _, hdl := range itemHandle {
		itemSt := owner.GetBloodItemByHandle(hdl)
		if itemSt == nil {
			return
		}

		itemId := itemSt.ItemId
		_, ok := needItemIdSet[itemId]
		if !ok {
			return
		}

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}

		// 不是血脉
		if !itemdef.IsBloodItem(itemConf.Type) {
			return
		}

		bagItemList = append(bagItemList, itemSt)
	}

	for _, hdl := range exItemHandle {
		itemSt := owner.GetBloodItemByHandle(hdl)
		if itemSt == nil {
			return
		}

		itemId := itemSt.ItemId
		_, ok := needItemExtIdSet[itemId]
		if !ok {
			return
		}

		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return
		}

		// 不是血脉
		if !itemdef.IsBloodItem(itemConf.Type) {
			return
		}

		bagItemList = append(bagItemList, itemSt)
	}

	// 道具都符合
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  1,
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   false,
	}
	if !owner.CanAddItem(newItem, true) {
		owner.SendBagFullTip(conf.ItemId)
		return
	}

	// 移除道具
	statics := &pb3.LogCompose{
		Items: make(map[uint32]uint32),
	}

	for _, st := range bagItemList {
		owner.RemoveBloodItemByHandle(st.Handle, pb3.LogId_LogComposeItem)
		statics.Items[st.GetItemId()] += uint32(st.GetCount())
	}

	// 移除固定装备
	slot := consumeItemSt.Pos
	if slot == 0 {
		owner.RemoveBloodItemByHandle(consumeItemSt.Handle, pb3.LogId_LogComposeItem)
	} else {
		bloodSys.TakeOffByCompose(slot)
	}
	statics.Items[consumeItemSt.GetItemId()] += uint32(consumeItemSt.GetCount())

	// 添加道具
	res := owner.AddItem(newItem)
	sys.packSend(res)

	// 穿配装备
	if slot > 0 {
		bloodSys.TakeOnByCompose(slot, newItem.AddItemAfterHdl)
	}

	sys.owner.TriggerQuestEvent(custom_id.QttBloodCompose, 0, 1)
	logworker.LogCompose(sys.owner, conf, statics)
}

// 血玉合成
func (sys *ComposeSys) composeFeatherGen(conf *jsondata.ComposeConf, autoBuy bool, composeCount uint32) (newItem *itemdef.ItemParamSt) {
	if composeCount <= 0 {
		return
	}
	actor := sys.owner

	consumes := jsondata.MergeConsumeVec(conf.Consume, conf.CoinConsume)

	isBind := false
	for _, consume := range conf.Consume {
		if sys.GetOwner().CheckItemBind(consume.Id, true) {
			isBind = true
		}
	}

	newItem = &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  int64(composeCount),
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   isBind,
	}
	if !actor.CanAddItem(newItem, true) {
		actor.SendBagFullTip(conf.ItemId)
		return
	}

	if !actor.ConsumeRate(consumes, int64(composeCount), autoBuy, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem}) {
		// 消耗失败
		return
	}

	res := actor.AddItem(newItem)
	sys.onItemComposed(conf, composeCount)
	sys.packSend(res)

	logworker.LogComposeWithConsume(actor, conf, consumes, composeCount)
	return
}

// 羽装合成
func (sys *ComposeSys) composeFeatherEqu(conf *jsondata.ComposeConf, composeCount uint32) {
	for i := composeCount; i > 0; i-- {
		player := sys.owner
		var itemHandleList []uint64
		bagSys, ok := player.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
		if !ok {
			return
		}

		featherSys, ok := player.GetSysObj(sysdef.SiFeather).(*FeatherSys)
		if !ok {
			return
		}

		var fixConsumeVec jsondata.ConsumeVec //固定消耗
		fixConsumeVec = append(fixConsumeVec, jsondata.CopyConsumeVec(conf.CoinConsume)...)
		itemEquipCount := make(map[uint32]uint32)
		for _, v := range conf.Consume {
			if v.Type == custom_id.ConsumeTypeItem && itemdef.IsFeatherEqu(jsondata.GetItemType(v.Id)) {
				itemEquipCount[v.Id] += v.Count
			} else {
				fixConsumeVec = append(fixConsumeVec, &jsondata.Consume{
					Type:       v.Type,
					Id:         v.Id,
					Count:      v.Count,
					CanAutoBuy: v.CanAutoBuy,
					Job:        v.Job,
				})
			}
		}

		for itemId, count := range itemEquipCount {
			if count == 0 {
				continue
			}
			var hasCount uint32
			needCount := count - hasCount
			hdlList := bagSys.GetItemListByItemId(itemId, needCount)
			hasCount += uint32(len(hdlList))

			if hasCount < count {
				return
			}
			itemHandleList = append(itemHandleList, hdlList...)
		}

		var removeHdlList []*pb3.ItemSt
		var maxBackLv uint64
		var maxBackLvItem *pb3.ItemSt
		var backRewardsVec []jsondata.StdRewardVec
		var allItemStList []*pb3.ItemSt
		for _, hdl := range itemHandleList {
			itemSt := bagSys.FindItemByHandle(hdl)
			if nil == itemSt {
				return
			}

			itemId := itemSt.ItemId

			itemConf := jsondata.GetItemConfig(itemId)
			if itemConf == nil {
				return
			}

			// 不是羽装道具 表示 hdl 有问题
			if !itemdef.IsFeatherEqu(itemConf.Type) {
				return
			}

			removeHdlList = append(removeHdlList, itemSt)

			allItemStList = append(allItemStList, itemSt)
			checkLv := utils.Make64(itemSt.GetUnion2(), itemSt.GetUnion1())
			if maxBackLv < checkLv {
				maxBackLv = checkLv
				maxBackLvItem = itemSt
			}
		}

		for _, itemSt := range allItemStList {
			if itemSt.GetHandle() == maxBackLvItem.GetHandle() {
				continue
			}
			backReward := featherSys.packBackMaterial(itemSt)
			backRewardsVec = append(backRewardsVec, backReward)
		}

		if bagSys.AvailableCount() <= 0 {
			return
		}

		flag, removes := player.ConsumeByConfWithRet(fixConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
		if !flag {
			player.LogWarn("consume failed, conf id %d", conf.Id)
			return
		}

		statics := &pb3.LogCompose{
			Items:  make(map[uint32]uint32),
			Moneys: make(map[uint32]uint32),
		}

		newHdl, err := series.AllocSeries()
		if err != nil {
			sys.LogError(err.Error())
			return
		}

		for _, removeItem := range removeHdlList {
			bagSys.RemoveItemByHandle(removeItem.GetHandle(), pb3.LogId_LogComposeItem)
			statics.Items[removeItem.GetItemId()] += uint32(removeItem.GetCount())
		}

		maxBackLvItem.ItemId = conf.ItemId
		maxBackLvItem.Handle = newHdl
		bagSys.AddItemPtr(maxBackLvItem, true, pb3.LogId_LogComposeItem)

		backRewards := jsondata.MergeStdReward(backRewardsVec...)
		if len(backRewards) > 0 {
			engine.GiveRewards(player, backRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogComposeItem})
		}

		sys.packSend(true)

		for id, count := range removes.ItemMap {
			statics.Items[id] += uint32(count)
		}
		for id, count := range removes.MoneyMap {
			statics.Moneys[id] += uint32(count)
		}
		logworker.LogCompose(sys.owner, conf, statics)
	}
}

// 领域符文合成
func (sys *ComposeSys) composeDomainEyeRune(conf *jsondata.ComposeConf, consumeHdl uint64) {
	owner := sys.GetOwner()

	consumeTrends := conf.ConsumeTrends
	var consumeIdSet = make(map[uint32]struct{})
	for _, id := range consumeTrends.Id {
		consumeIdSet[id] = struct{}{}
	}

	checkItemValid := func(hdl uint64) bool {
		itemSt := owner.GetDomainEyeRuneItemByHandle(hdl)
		if itemSt == nil {
			return false
		}
		itemId := itemSt.ItemId
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return false
		}
		if !itemdef.IsDomainEyeItem(itemConf.Type) {
			return false
		}
		return true
	}

	// 检查传入的hdl是否是穿戴中的符文
	if !checkItemValid(consumeHdl) {
		return
	}
	consumeItemSt := owner.GetDomainEyeRuneItemByHandle(consumeHdl)
	_, ok := consumeIdSet[consumeItemSt.ItemId]
	if !ok {
		return
	}
	if consumeItemSt.Ext.OwnerId == 0 || consumeItemSt.Pos == 0 {
		return
	}

	// 道具都符合
	newItem := &itemdef.ItemParamSt{
		ItemId: conf.ItemId,
		Count:  1,
		LogId:  pb3.LogId_LogComposeItem,
		Bind:   false,
	}
	if !owner.CanAddItem(newItem, true) {
		owner.SendBagFullTip(conf.ItemId)
		return
	}

	// 背包道具
	mergeConsume := jsondata.MergeConsumeVec(conf.CoinConsume, conf.Consume)
	flag, removes := sys.owner.ConsumeByConfWithRet(mergeConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogComposeItem})
	if !flag {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	// 日志
	statics := &pb3.LogCompose{
		Items:  make(map[uint32]uint32),
		Moneys: make(map[uint32]uint32),
	}
	statics.Items[consumeItemSt.GetItemId()] += uint32(consumeItemSt.GetCount())
	for id, count := range removes.ItemMap {
		statics.Items[id] += uint32(count)
	}
	for id, count := range removes.MoneyMap {
		statics.Moneys[id] += uint32(count)
	}

	// 获得新的符文
	res := owner.AddItem(newItem)
	sys.packSend(res)

	// 镶嵌符文
	domainEyeSys, ok := sys.owner.GetSysObj(sysdef.SiDomainEye).(*DomainEyeSys)
	if ok && domainEyeSys.IsOpen() {
		id, slot := uint32(consumeItemSt.Ext.OwnerId), consumeItemSt.Pos

		domainEyeSys.TakeOffRuneAfterCompose(id, slot)
		domainEyeSys.TakeOnRuneAfterCompose(id, slot, newItem.AddItemAfterHdl)
	}

	logworker.LogCompose(sys.owner, conf, statics)
}

// 新守护合成
func (sys *ComposeSys) composeTypeSaNewSpirit(successRate uint32, conf *jsondata.ComposeConf, itemHandles []uint64, exItemHandle []uint64) {
	// 检查前端发过来的handle
	composeArea := conf.ConsumeTrends.Id
	exComposeArea := conf.ConsumeTrends.ExaId
	actor := sys.owner
	var bodyHandle uint64 // 挑出那个在身上的handle
	var bodyItemId uint32
	var itemId uint32
	var resHandles []uint64
	var isBind bool
	obj := actor.GetSysObj(sysdef.SiNewSpirit)
	if obj == nil || !obj.IsOpen() {
		actor.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}
	nsSys := obj.(*NewSpiritSys)
	if nsSys == nil {
		actor.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}
	newSpiritData := nsSys.getData()
	for _, handle := range itemHandles {
		item := actor.GetItemByHandle(handle)
		if nil == item {
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			if newSpiritData == nil {
				continue
			}
			// 去找身上
			equips := newSpiritData.PosSpirit
			for _, e := range equips {
				itemId = e.ItemId
				if bodyHandle == 0 && e.Handle == handle {
					bodyHandle = handle
					bodyItemId = e.ItemId
					isBind = e.Bind
					break
				}
			}
		} else {
			itemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(composeArea, itemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}

	var exItemId uint32
	for _, handle := range exItemHandle {
		item := actor.GetItemByHandle(handle)
		if nil == item {
			// 去找身上
			if bodyHandle > 0 { // 已经有身上的装备了
				actor.SendTipMsg(tipmsgid.TpComposeBodyEquipTooMuch)
				return
			}
			if newSpiritData == nil {
				continue
			}
			// 去找身上
			equips := newSpiritData.PosSpirit
			for _, e := range equips {
				if e.Handle == handle {
					bodyHandle = handle
					bodyItemId = e.ItemId
					exItemId = e.ItemId
					if !isBind {
						isBind = e.Bind
					}
					break
				}
			}
		} else {
			exItemId = item.ItemId
			resHandles = append(resHandles, handle)
		}
		// 前端发送过来的是否在范围内
		if !utils.SliceContainsUint32(exComposeArea, exItemId) { // 装备ID是否合法 tips
			actor.SendTipMsg(tipmsgid.TpComposeConfNil)
			return
		}
	}

	isHaveBodyHandle := bodyHandle > 0
	// 身上的装备要跟目标装备同一位置
	itemConf := jsondata.GetItemConfig(conf.ItemId)
	bodyItemConf := jsondata.GetItemConfig(bodyItemId)
	if isHaveBodyHandle && itemConf != nil && bodyItemConf != nil && (itemConf.SubType != bodyItemConf.SubType) {
		actor.SendTipMsg(tipmsgid.TpComposePosErr)
		return
	}

	// 判断成功率
	composeSuccess := random.Hit(successRate, 10000)
	// 是否有身上的装备
	if isHaveBodyHandle {
		sys.composeTypeSaNewSpiritBody(composeSuccess, conf, bodyHandle, resHandles, isBind)
	} else {
		sys.composeTypeSaNewSpiritBag(composeSuccess, conf, append(itemHandles, exItemHandle...), isBind)
	}
	sys.sendTips(composeSuccess, conf)
}

// 新守护合成 - 身上装备
func (sys *ComposeSys) composeTypeSaNewSpiritBody(composeSuccess bool, conf *jsondata.ComposeConf, bodyHandle uint64, resHandles []uint64, isBind bool) {
	// 找到固定的消耗
	actor := sys.owner
	newItemId := conf.ItemId
	itemConf := jsondata.GetItemConfig(newItemId)
	// 直接替换掉身上装备的itemID
	obj := actor.GetSysObj(sysdef.SiNewSpirit)
	if obj == nil || !obj.IsOpen() {
		actor.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}
	nsSys := obj.(*NewSpiritSys)
	if nsSys == nil {
		actor.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}
	data := nsSys.getData()
	equipLs := data.PosSpirit
	statics := &pb3.LogCompose{}
	flag := false

	if composeSuccess {
		// 其余的不在身上的装备走正常消耗跟固定消耗一起
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 替换自己身上的装备
			for pos, eq := range equipLs {
				if eq.Handle != bodyHandle {
					continue
				}
				// 找到替换
				oldEq := &pb3.ItemSt{
					ItemId: eq.ItemId,
					Handle: eq.Handle,
					Count:  eq.Count,
					Pos:    eq.Pos,
					Bind:   eq.Bind,
					Union1: eq.Union1,
				}
				eq.ItemId = newItemId
				eq.Bind = isBind
				eq.Union1 = itemConf.CommonField
				nsSys.AfterTakeReplace(pos, eq, oldEq)
				break
			}
			flag = true
			sys.packSend(true)
		}
	} else {
		// 合成失败 身上的脱下 然后一起消耗掉 走脱下逻辑
		for pos, eq := range equipLs {
			if eq.Handle != bodyHandle {
				continue
			}
			nsSys.takeOff(pos, eq)
			break
		}
		resHandles = append(resHandles, bodyHandle)
		if sys.consumeWithHandler(conf.Consume, resHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
			flag = true
		}
	}
	nsSys.s2cInfo()

	if flag {
		logworker.LogCompose(actor, conf, statics)
	}
}

// 新守护合成 - 背包
func (sys *ComposeSys) composeTypeSaNewSpiritBag(composeSuccess bool, conf *jsondata.ComposeConf, itemHandles []uint64, isBind bool) {
	actor := sys.owner
	newItemId := conf.ItemId
	fixConsume := conf.Consume
	statics := &pb3.LogCompose{}
	flag := false
	if composeSuccess {
		// 直接替换背包里面的就好了
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			// 消耗成功 生成新的装备到背包
			equip := &itemdef.ItemParamSt{
				ItemId: newItemId,
				Count:  1,
				Bind:   isBind,
			}
			if actor.AddItem(equip) {
				sys.packSend(true)
			}
			flag = true
		}
	} else { // 直接消耗
		if sys.consumeWithHandler(fixConsume, itemHandles, pb3.LogId_LogComposeItem, statics) {
			sys.packSend(false)
			flag = true
		}
	}

	if flag {
		logworker.LogCompose(actor, conf, statics)
	}
}

func (sys *ComposeSys) composeTypeSoulHaloSkeleton(conf *jsondata.ComposeConf, itemHandles []uint64, isBody bool) {
	actor := sys.owner
	cItem := jsondata.GetItemConfig(conf.ItemId)
	if cItem == nil {
		return
	}
	skeletonSys := actor.GetSysObj(sysdef.SiSoulHaloSkeleton).(*SoulHaloSkeletonSys)
	if skeletonSys == nil || !skeletonSys.IsOpen() {
		actor.SendTipMsg(tipmsgid.TpSySNotOpen)
		return
	}
	if isBody {
		sys.composeSoulHaloSkeletonOnBody(conf, cItem, skeletonSys, itemHandles)
	} else {
		sys.composeSoulHaloSkeletonInBag(conf, cItem, skeletonSys, itemHandles)
	}
}

// 处理身上器骸装备的合成
func (sys *ComposeSys) composeSoulHaloSkeletonOnBody(conf *jsondata.ComposeConf, cItem *jsondata.ItemConf, skeletonSys *SoulHaloSkeletonSys, itemHandles []uint64) {
	actor := sys.owner
	bodySkeletonItem := uint64(0)
	// 遍历身上的器骸装备，找到匹配的装备
	for _, skeleton := range skeletonSys.GetSoulHaloSkeletonData().Skeleton {
		if skeleton.ItemId == 0 {
			continue
		}
		for i := 0; i < len(itemHandles); i++ {
			if itemHandles[i] == skeleton.Handle {
				bodySkeletonItem = skeleton.Handle
				itemHandles = append(itemHandles[:i], itemHandles[i+1:]...)
				break
			}

		}
	}
	if bodySkeletonItem == 0 {
		return
	}
	statics := &pb3.LogCompose{}

	if sys.consumeWithHandler(conf.Consume, itemHandles, pb3.LogId_LogComposeItem, statics) { // 直接替换身上的器骸装备
		err := skeletonSys.TakeOnWithItemConfAndWithoutRewardToBag(actor, cItem, uint32(pb3.LogId_LogComposeItem), itemHandles, bodySkeletonItem)
		if err != nil {
			return
		}
	}
	sys.packSend(true)
	logworker.LogCompose(actor, conf, statics)
}

// 处理背包器骸装备的合成
func (sys *ComposeSys) composeSoulHaloSkeletonInBag(conf *jsondata.ComposeConf, cItem *jsondata.ItemConf, skeletonSys *SoulHaloSkeletonSys, itemHandles []uint64) {
	actor := sys.owner
	// 检查背包中是否有对应的器骸装备
	hasRequiredItem := false
	for _, handle := range itemHandles {
		item := actor.GetItemByHandle(handle)
		if item != nil {
			itemConf := jsondata.GetItemConfig(item.ItemId)
			if itemConf != nil && itemConf.Type == itemdef.ItemTypeSoulHaloSkeleton {
				hasRequiredItem = true
				break
			}
		}
	}
	if !hasRequiredItem {
		return
	}
	statics := &pb3.LogCompose{}
	if !sys.consumeWithHandler(conf.Consume, itemHandles, pb3.LogId_LogComposeItem, statics) {
		return
	}

	// 生成新的 到背包
	equip := &itemdef.ItemParamSt{
		ItemId: cItem.Id,
		Count:  1,
	}
	if actor.AddItem(equip) {
		sys.packSend(true)
		logworker.LogCompose(actor, conf, statics)
	}
}
