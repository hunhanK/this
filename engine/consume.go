package engine

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"golang.org/x/exp/maps"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/gameserver/iface"
	"math"
)

type Consumer struct {
	owner iface.IPlayer
}

func NewConsumer(owner iface.IPlayer) *Consumer {
	return &Consumer{owner: owner}
}

func calcBuyItemCost(itemId uint32, buyCount int64, haveMoney map[uint32]int64) (cost map[uint32]int64, flag bool) {
	cost = make(map[uint32]int64)
	if _, ok := haveMoney[moneydef.BindDiamonds]; !ok {
		haveMoney[moneydef.BindDiamonds] = 0
	}

	for mt, haveCount := range haveMoney {
		price := int64(jsondata.GetAutoBuyItemPrice(itemId, mt))
		if price <= 0 {
			continue
		}

		need := price * buyCount

		// 如果货币充足，直接退出
		if need <= haveCount {
			buyCount = 0
			cost[mt] += need
			break
		}

		// 货币不足，先尽可能用该货币购买
		canBuyCount := int64(math.Floor(float64(haveCount)) / float64(price))
		cost[mt] = canBuyCount * price
		buyCount -= canBuyCount
	}

	// 货币不足，但是该货币试绑钻，可以尝试用钻石补充
	if buyCount > 0 {
		if price := jsondata.GetAutoBuyItemPrice(itemId, moneydef.BindDiamonds); price > 0 {
			more := price * buyCount
			left := haveMoney[moneydef.BindDiamonds] - cost[moneydef.BindDiamonds]
			if left < 0 {
				flag = false
				return
			}

			cost[moneydef.Diamonds] += more - left
			cost[moneydef.BindDiamonds] += left
			if cost[moneydef.Diamonds] > haveMoney[moneydef.Diamonds] {
				flag = false
				return
			}
			buyCount = 0
		}
	}

	return cost, buyCount <= 0
}

func (c *Consumer) CheckEnoughByRemoveMaps(remove argsdef.RemoveMaps) bool {
	for itemId, count := range remove.ItemMap {
		if count < 0 || c.owner.GetItemCount(itemId, -1) < count {
			return false
		}
	}

	for moneyType, moneyCount := range remove.MoneyMap {
		if moneyCount < 0 || c.owner.GetMoneyCount(moneyType) < moneyCount {
			return false
		}
	}

	return true
}

// filterConsumeFromConf 非拷贝安全，不要暴漏给外部访问
func (c *Consumer) filterConsumeFromConf(consumes jsondata.ConsumeVec) (ret jsondata.ConsumeVec) {
	for _, consume := range consumes {
		if consume.Job > 0 && consume.Job != c.owner.GetJob() {
			continue
		}
		ret = append(ret, consume)
	}
	return ret
}

func (c *Consumer) Consume(normalizedConsumes argsdef.NormalizeConsumesSt, params common.ConsumeParams) (bool, argsdef.RemoveMaps) {
	removeMaps, buyItemMap, valid := c.CalcRemoveAndBuyItem(normalizedConsumes, params)
	if !valid {
		return valid, removeMaps
	}

	if !c.CheckEnoughByRemoveMaps(removeMaps) {
		return false, removeMaps
	}

	if !c.RemoveResource(removeMaps, params) {
		return false, removeMaps
	}

	c.TriggerConsumeEvent(removeMaps, normalizedConsumes.AutoBuyItemMap, buyItemMap)
	return true, removeMaps
}

func (c *Consumer) RemoveResource(remove argsdef.RemoveMaps, params common.ConsumeParams) bool {
	// 消耗道具
	for itemId, itemCount := range remove.ItemMap { //
		if !c.owner.DeleteItemById(itemId, itemCount, params.LogId) {
			return false
		}
	}

	// 消耗钱币
	for moneyType, moneyCount := range remove.MoneyMap {
		if !c.owner.DeductMoney(moneyType, moneyCount, params) {
			return false
		}
	}

	return true
}

func (c *Consumer) TriggerConsumeEvent(remove argsdef.RemoveMaps, autoBuyItemMap map[uint32]int64, buyItemMap map[uint32]int64) {
	for mt, _ := range remove.MoneyMap {
		if mt == moneydef.BindDiamonds || mt == moneydef.Diamonds {
			c.owner.TriggerQuestEvent(custom_id.QttDiamondShop, 0, 1)
		}
	}

	for itemId, itemCount := range autoBuyItemMap {
		c.owner.TriggerEvent(custom_id.AeAutoBuy, itemId, itemCount)

	}
	for itemId, itemCount := range remove.ItemMap {
		c.owner.TriggerQuestEvent(custom_id.QttConsumeItemNum, itemId, itemCount)
	}
}

func (c *Consumer) CalcRemoveAndBuyItem(normalizedConsumes argsdef.NormalizeConsumesSt, params common.ConsumeParams) (remove argsdef.RemoveMaps, buyItemMap map[uint32]int64, valid bool) {
	remove = argsdef.RemoveMaps{
		MoneyMap: make(map[uint32]int64),
		ItemMap:  maps.Clone(normalizedConsumes.ItemMap),
	}
	buyItemMap = make(map[uint32]int64)

	// 玩家当前拥有的货币
	var hadMoneys = c.owner.CopyMoneys()

	for moneyType, moneyCount := range normalizedConsumes.MoneyMap {
		haveCount := hadMoneys[moneyType]
		if haveCount < moneyCount {
			if moneyType == moneydef.BindDiamonds {
				if haveCount+hadMoneys[moneydef.Diamonds] >= moneyCount {
					remove.MoneyMap[moneydef.Diamonds] += moneyCount - haveCount
					remove.MoneyMap[moneyType] += haveCount
					continue
				}
				return
			}
			return
		}
		remove.MoneyMap[moneyType] += moneyCount
	}

	// 先统计配置的货币消耗
	for mt, cost := range remove.MoneyMap {
		left := hadMoneys[mt] - cost
		if left < 0 {
			return
		}
		hadMoneys[mt] = left
	}

	// 统计需要自动购买的道具 和需要消耗的道具
	for itemId, itemCount := range normalizedConsumes.AutoBuyItemMap {
		has := c.owner.GetItemCount(itemId, -1) - normalizedConsumes.ItemMap[itemId]
		if has < itemCount {
			buyItemMap[itemId] += itemCount - has
			remove.ItemMap[itemId] += has
		} else {
			remove.ItemMap[itemId] += itemCount
		}
	}

	// 自动购买道具转换成货币
	for id, count := range buyItemMap {
		// 计算购买道具需要的货币
		cost, flag := calcBuyItemCost(id, count, hadMoneys)
		if !flag {
			return
		}

		// 统计需要消耗的货币
		for mt, cost := range cost {
			left := hadMoneys[mt] - cost
			if left < 0 {
				return
			}
			hadMoneys[mt] = left

			remove.MoneyMap[mt] += cost
		}
	}

	// 在最后计算减免的仙玉 绑玉
	for k, v := range remove.MoneyMap {
		remove.MoneyMap[k] = jsondata.CalcConsumeDiamondsDiscount(int(k), v, params.DrawAwardsSubDiamondAddRate, uint32(params.LogId))
	}

	return remove, buyItemMap, true
}

func (c *Consumer) NormalizeConsumeConf(consumes jsondata.ConsumeVec, autoBuy bool) (ret argsdef.NormalizeConsumesSt, valid bool) {
	ret = argsdef.NormalizeConsumesSt{
		ItemMap:        make(map[uint32]int64),
		MoneyMap:       make(map[uint32]int64),
		AutoBuyItemMap: make(map[uint32]int64),
	}

	consumes = c.filterConsumeFromConf(consumes)

	valid = functional.Reduce(consumes, func(valid bool, cur *jsondata.Consume) bool {
		if valid == false {
			return false
		}

		if cur.Type < custom_id.ConsumeTypeStart || cur.Type > custom_id.ConsumeTypeEnd {
			return false
		}

		return true
	}, true)

	if !valid {
		return
	}

	autoBuyItemSumFn := func(sumMap map[uint32]int64, consume *jsondata.Consume) map[uint32]int64 {
		if consume.Type == custom_id.ConsumeTypeItem {
			if autoBuy && consume.CanAutoBuy {
				sumMap[consume.Id] += int64(consume.Count)
			}
		}

		return sumMap
	}

	itemMapSumFn := func(sumMap map[uint32]int64, consume *jsondata.Consume) map[uint32]int64 {
		if consume.Type == custom_id.ConsumeTypeItem {
			if !(autoBuy && consume.CanAutoBuy) {
				sumMap[consume.Id] += int64(consume.Count)
			}
		}
		return sumMap
	}

	moneyMapSumFn := func(sumMap map[uint32]int64, consume *jsondata.Consume) map[uint32]int64 {
		if consume.Type == custom_id.ConsumeTypeMoney {
			sumMap[consume.Id] += int64(consume.Count)
		}
		return sumMap
	}

	functional.Reduces(consumes, []map[uint32]int64{
		ret.AutoBuyItemMap,
		ret.ItemMap,
		ret.MoneyMap,
	},
		autoBuyItemSumFn,
		itemMapSumFn,
		moneyMapSumFn,
	)
	return
}

type ConsumeBehaviorCheckerFunc func(params common.ConsumeParams, subType uint32, ext []uint32) bool

var ConsumeBehaviorCheckerMap = make(map[uint32]ConsumeBehaviorCheckerFunc)

func RegisterConsumeBehaviorChecker(logId uint32, fn ConsumeBehaviorCheckerFunc) {
	if nil == fn {
		return
	}

	if _, ok := ConsumeBehaviorCheckerMap[logId]; ok {
		return
	}

	ConsumeBehaviorCheckerMap[logId] = fn
}

const defaultConsumeMoneyCheckerId = 0

func CheckConsumeBehavior(params common.ConsumeParams, logId uint32, subType uint32, ext []uint32) bool {
	if fn, ok := ConsumeBehaviorCheckerMap[logId]; ok {
		return fn(params, subType, ext)
	}

	fn := ConsumeBehaviorCheckerMap[defaultConsumeMoneyCheckerId]
	return fn(params, subType, ext)
}

func init() {
	RegisterConsumeBehaviorChecker(defaultConsumeMoneyCheckerId, func(params common.ConsumeParams, subType uint32, ext []uint32) bool {
		if subType > 0 && params.SubType != subType {
			return false
		}
		if len(ext) > 0 && !pie.Uint32s(ext).Contains(params.RefId) {
			return false
		}
		return true
	})
}
