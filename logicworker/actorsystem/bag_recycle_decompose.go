package actorsystem

import (
	"github.com/gzjjyz/random"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

func getRecycleRandRewards(_ iface.IPlayer, randRewards []*jsondata.RandomReward, count int64) jsondata.StdRewardVec {
	var items jsondata.StdRewardVec
	pool := new(random.Pool)
	for _, randRewardConf := range randRewards {
		pool.Clear()
		for _, id := range randRewardConf.Ids {
			pool.AddItem(id, 1)
		}
		ids := pool.RandomMany(randRewardConf.Count)
		for _, v := range ids {
			itemId := v.(uint32)
			itemConf := jsondata.GetItemConfig(itemId)
			if itemConf == nil {
				continue
			}
			items = append(items, &jsondata.StdReward{
				Id:    itemId,
				Count: 1,
				Bind:  randRewardConf.Bind,
			})
		}
	}
	items = jsondata.StdRewardMulti(items, count)
	return items
}

func getDecomposeExRewards(player iface.IPlayer, itemSt *pb3.ItemSt, itemConf *jsondata.ItemConf) jsondata.StdRewardVec {
	switch itemConf.Type {
	case itemdef.ItemTypeSoulHalo:
		if s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys); ok {
			return s.calSoulHaloRecycleRewards(itemSt)
		}
	case itemdef.ItemTypeBattleSoulGodEquip:
		if s, ok := player.GetSysObj(sysdef.SiBattleSoulGodEquip).(*BattleSoulGodEquipSys); ok {
			rewards, _ := s.packBackMaterial(itemSt)
			return rewards
		}
	case itemdef.ItemTypeSoulBone:
		if s, ok := player.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys); ok {
			return s.calSoulBoneRecycleRewards(itemSt)
		}
	case itemdef.ItemTypeSmithEquip:
		if s, ok := GetSmithEquipSys(player, sysdef.SiSmithEquip); ok {
			return s.calRecycleRewards(itemSt)
		}
	}
	return nil
}

func canDecomposeFn(player iface.IPlayer, itemConf *jsondata.ItemConf) (bool, error) {
	switch itemConf.Type {
	case itemdef.ItemTypeMountainsAndSea:
		data := player.GetBinaryData().GetMountainsAndSeas()
		id := jsondata.GetMountainsAndSeasIdByItem(itemConf.Id)
		if _, isNotActive := data.Data[id]; !isNotActive && id > 0 {
			return false, neterror.ParamsInvalidError("cant decompose mountrainandseas not active")
		}
	}
	return true, nil
}

func filterUpLimitRewards(player iface.IPlayer, rewards jsondata.StdRewardVec) jsondata.StdRewardVec {
	copyStdRewardVec := jsondata.CopyStdRewardVec(rewards)
	var newRewards jsondata.StdRewardVec
	for _, reward := range copyStdRewardVec {
		itemConf := jsondata.GetItemConfig(reward.Id)
		if itemConf == nil {
			continue
		}
		switch itemConf.Type {
		case itemdef.ItemTypeSmithEquipMaterial:
			if s, ok := GetSmithEquipSys(player, sysdef.SiSmithEquip); ok {
				reward.Count = s.calcUpLimitCount(reward.Id, reward.Count)
			}
		}
		if reward.Count == 0 {
			continue
		}
		newRewards = append(newRewards, reward)
	}
	return newRewards
}

func recycleAndDecompose(player iface.IPlayer, handles []uint64, logId pb3.LogId) error {
	itemHandleToRemove := make([]uint64, 0)
	normalRewardVecs := make([]jsondata.StdRewardVec, 0)
	var itemTypeNumMap = make(map[uint32]uint32)
	for _, handle := range handles {
		objSys := player.GetBelongBagSysByHandle(handle)
		if nil == objSys {
			return neterror.ParamsInvalidError("item handle %d not found in bag", handle)
		}
		sys := objSys.(iface.IBagSys)
		item := sys.FindItemByHandle(handle)
		if item == nil {
			return neterror.ParamsInvalidError("item handle %d not found in bag", handle)
		}

		cycleConf := jsondata.GetRecycleConf(item.ItemId)
		if cycleConf == nil {
			return neterror.InternalError("item %d recycle config not found", item.ItemId)
		}

		itemConf := jsondata.GetItemConfig(item.ItemId)
		if itemConf == nil {
			continue
		}

		if ok, err := canDecomposeFn(player, itemConf); !ok {
			return neterror.Wrap(err)
		}

		itemHandleToRemove = append(itemHandleToRemove, handle)
		normalRewardVecs = append(normalRewardVecs, jsondata.StdRewardMulti(cycleConf.Rewards, item.Count))

		// 随机奖励
		if randRewards := getRecycleRandRewards(player, cycleConf.RandomRewards, item.Count); len(randRewards) > 0 {
			normalRewardVecs = append(normalRewardVecs, randRewards)
		}

		// 额外奖励
		if exRewards := getDecomposeExRewards(player, item, itemConf); len(exRewards) > 0 {
			normalRewardVecs = append(normalRewardVecs, exRewards)
		}
		itemTypeNumMap[itemConf.Type] += 1
	}

	normalRewards := jsondata.MergeStdReward(normalRewardVecs...)

	// 过滤每日获得上限的道具
	normalRewards = filterUpLimitRewards(player, normalRewards)

	// 特权的道具回收收益加成
	yieldAddRate, _ := player.GetPrivilege(privilegedef.EnumNormalEquipRecycleYield)
	normalYieldRate := 10000 + yieldAddRate

	for _, v := range normalRewards {
		itemConf := jsondata.GetItemConfig(v.Id)
		if nil == itemConf {
			continue
		}
		if itemConf.Type != itemdef.ItemTypeMoney {
			continue
		}
		if itemConf.SubType != moneydef.YuanBao && itemConf.SubType != moneydef.FairyStone {
			continue
		}
		v.Count = int64(float64(v.Count) * float64(normalYieldRate) / 10000)
	}

	for _, handle := range itemHandleToRemove {
		sys := player.GetBelongBagSysByHandle(handle).(iface.IBagSys)
		sys.RemoveItemByHandle(handle, logId)
	}

	if len(normalRewards) > 0 && !engine.GiveRewards(player, normalRewards, common.EngineGiveRewardParam{LogId: logId}) {
		return neterror.InternalError("decompose give rewards failed")
	}

	// 触发任务
	for typ, num := range itemTypeNumMap {
		if num == 0 || typ == 0 {
			continue
		}
		player.TriggerQuestEvent(custom_id.QttRecycleAndDecomposeItemType, typ, int64(num))
		player.TriggerEvent(custom_id.AeRecycleItemTypeNum, typ, int64(num))
	}

	return nil
}
