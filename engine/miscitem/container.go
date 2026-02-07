package miscitem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine/series"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

// 由于该项目使用的是protobuf2,协议不能使用map类型
type (
	Container struct {
		DefaultSizeHandle func() uint32
		EnlargeSizeHandle func() uint32
		GetMoneyHandle    func(moneyType uint32) int64
		AddMoneyHandle    func(moneyType uint32, count int64, btip bool, logId pb3.LogId) bool
		OnAddNewItem      func(item *pb3.ItemSt, bTip bool, logId pb3.LogId)
		OnItemChange      func(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam)
		OnRemoveItem      func(item *pb3.ItemSt, logId pb3.LogId)
		OnUseOnGetItem    func(hdl uint64, count int64)
		AppendNewAddItem  func(*pb3.ItemSt)
		itemVec           *[]*pb3.ItemSt
		itemMap           map[uint64]*pb3.ItemSt
		needSameOwner     bool
		OnDeleteItemPtr   func(item *pb3.ItemSt, count int64, logId pb3.LogId)
	}
)

func NewContainer(vec *[]*pb3.ItemSt) *Container {
	container := new(Container)
	container.itemVec = vec
	container.itemMap = make(map[uint64]*pb3.ItemSt)

	for _, item := range *vec {
		container.itemMap[item.GetHandle()] = item
	}

	return container
}

func (container *Container) GetAllItemMap() map[uint64]*pb3.ItemSt {
	return container.itemMap
}

func (container *Container) Clear() {
	if nil != container.itemVec {
		*(container.itemVec) = make([]*pb3.ItemSt, 0)
	}
	container.itemMap = make(map[uint64]*pb3.ItemSt)
}

func (container *Container) Capacity() uint32 {
	var count uint32 = 0
	if nil != container.DefaultSizeHandle {
		count += container.DefaultSizeHandle()
	}
	if nil != container.EnlargeSizeHandle {
		count += container.EnlargeSizeHandle()
	}
	return count
}

func (container *Container) AvailableCount() uint32 {
	cap := container.Capacity()
	size := uint32(len(*container.itemVec))
	if cap <= size {
		return 0
	}
	return cap - size
}

func (container *Container) GetAddItemNeedGridCount(param *itemdef.ItemParamSt, overlay bool) int64 {
	if nil == param {
		return 0
	}
	conf := jsondata.GetItemConfig(param.ItemId)
	if nil == conf {
		return 0
	}
	if conf.Type == itemdef.ItemTypeMoney {
		return 0
	}
	dup := int64(conf.Dup)
	if conf.Dup <= 0 {
		dup = 1
	}
	if overlay && !param.AutoUse && base.CheckItemFlag(param.ItemId, itemdef.UseOnGet) {
		overlay = false
	}
	var bind int8 = 0
	if param.Bind {
		bind = 1
	}
	remain := int64(param.Count)
	if dup > 1 {
		if overlay { //允许和现有的叠加
			// 当前有多少个这样的物品
			if tmp := container.GetItemCountByOwner(param.ItemId, bind, param.OwnerId); tmp > 0 {
				// 如果能够叠加，就去掉叠加的数量
				if mod := (dup - tmp%dup) % dup; remain > mod {
					remain -= mod
				} else {
					remain = 0
				}
			}
		}
		if remain <= 0 {
			return 0
		}

		if (remain % dup) > 0 { // 比如2个物品叠到最大叠加为3的格子上，需要消耗1个格子
			return remain/dup + 1
		} else {
			return remain / dup // 刚好放下，那就返回除的结果
		}
	}
	return remain
}

func (container *Container) CanAddItem(param *itemdef.ItemParamSt, overlay bool) bool {
	if nil == param {
		return false
	}

	conf := jsondata.GetItemConfig(param.ItemId)
	if nil == conf {
		logger.LogWarn("not found item config(id:%d)", param.ItemId)
		return false
	}

	available := int64(container.AvailableCount())
	need := container.GetAddItemNeedGridCount(param, overlay)

	return available >= need
}

func (container *Container) AddItem(param *itemdef.ItemParamSt) bool {
	if param.Count <= 0 {
		return false
	}
	conf := jsondata.GetItemConfig(param.ItemId)
	if nil == conf {
		return false
	}

	// 获得资源
	if itemdef.IsMoney(conf.Type, conf.SubType) {
		if nil != container.AddMoneyHandle {
			return container.AddMoneyHandle(conf.SubType, int64(param.Count), !param.NoTips, param.LogId)
		}
	}

	if !container.CanAddItem(param, true) {
		return false
	}
	remain := param.Count
	overlap := utils.MaxInt64(conf.Dup, 1)

	useOnGet := param.AutoUse
	if !useOnGet {
		useOnGet = base.CheckItemFlag(param.ItemId, itemdef.UseOnGet)
	}
	doOverLap := !useOnGet && overlap > 1

	if doOverLap {
		overlapCount := int64(container.overlapToExists(param, remain))
		if remain > overlapCount {
			remain -= overlapCount
		} else {
			remain = 0
		}
	}

	for remain > 0 && container.AvailableCount() > 0 {
		var count int64
		if remain > overlap {
			count = overlap
			remain -= overlap
		} else {
			count = remain
			remain = 0
		}

		item := &pb3.ItemSt{
			ItemId: param.ItemId,
			Count:  count,
			Bind:   param.Bind,
			Ext: &pb3.ItemExt{
				OwnerId: param.OwnerId,
			},
		}

		bTip := !param.NoTips
		if _, err := container.addNewItem(item, bTip, param.LogId); err != nil {
			logger.LogError(err.Error())
			return false
		}

		if useOnGet && nil != container.OnUseOnGetItem {
			container.OnUseOnGetItem(item.GetHandle(), count)
		}
		param.AddItemAfterHdl = item.GetHandle()
	}

	return true
}

func (container *Container) AddItemPtr(item *pb3.ItemSt, bTip bool, logId pb3.LogId) bool {
	if item.GetCount() <= 0 {
		return false
	}
	conf := jsondata.GetItemConfig(item.GetItemId())
	if nil == conf {
		return false
	}

	if container.AvailableCount() <= 0 {
		return false
	}

	*container.itemVec = append(*container.itemVec, item)
	container.itemMap[item.GetHandle()] = item
	if nil != container.OnAddNewItem {
		container.OnAddNewItem(item, bTip, logId)
	}

	return true
}

func (container *Container) DeleteItem(param *itemdef.ItemParamSt) bool {
	if param.Count <= 0 {
		return true
	}

	conf := jsondata.GetItemConfig(param.ItemId)
	if nil == conf {
		return false
	}

	temp := param.Count
	costMap := map[uint64]int64{}

	calc := func(flag bool) {
		if temp <= 0 {
			return
		}
		for _, v := range *container.itemVec {
			if v.GetItemId() == param.ItemId && v.GetBind() == flag {
				if hdl, cnt := v.GetHandle(), v.GetCount(); cnt >= temp {
					costMap[hdl] = temp
					temp = 0
					return
				} else {
					costMap[hdl] = cnt
					temp -= cnt
				}
			}
		}
	}
	//优先消耗绑定的
	calc(true)
	calc(false)

	//道具数量不足
	if temp > 0 {
		return false
	}

	for handle, count := range costMap {
		if item, ok := container.itemMap[handle]; ok {
			container.DeleteItemPtr(item, count, param.LogId)
		}
	}

	return true
}

func (container *Container) DeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId) bool {
	for idx, iterator := range *container.itemVec {
		if iterator == item {
			has := iterator.GetCount()
			if has < count {
				return false
			} else if has > count {
				item.Count = has - count
				if nil != container.OnItemChange {
					container.OnItemChange(item, 0-count, common.EngineGiveRewardParam{LogId: logId, NoTips: false})
				}
				if nil != container.OnDeleteItemPtr {
					container.OnDeleteItemPtr(item, count, logId)
				}
				return true
			} else {
				(*container.itemVec)[idx] = nil
				*container.itemVec = append((*container.itemVec)[:idx], (*container.itemVec)[idx+1:]...)
				delete(container.itemMap, item.GetHandle())
				if nil != container.OnRemoveItem {
					container.OnRemoveItem(item, logId)
				}
				if nil != container.OnDeleteItemPtr {
					container.OnDeleteItemPtr(item, count, logId)
				}
			}
			return true
		}
	}
	return false
}

func (container *Container) RemoveItemByHandle(handle uint64, logId pb3.LogId) bool {
	if item := container.FindItemByHandle(handle); nil != item {
		return container.DeleteItemPtr(item, item.GetCount(), logId)
	}
	return false
}

func (container *Container) GetItemCount(itemId uint32, bind int8) (count int64) {
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return
	}

	////判断是否是货币资源
	//if custom_id.IsMoney(conf.Type, conf.SubType) {
	//	if nil != container.GetMoneyHandle {
	//		return container.GetMoneyHandle(conf.SubType)
	//	}
	//}

	now := time_util.NowSec()
	for _, v := range *container.itemVec {
		if v.GetItemId() != itemId {
			continue
		}
		if bind != -1 {
			if v.GetBind() != (bind == 1) {
				continue
			}
		}

		timeout := v.GetTimeOut()
		if timeout > 0 && timeout <= now {
			continue
		}

		count += int64(v.GetCount())
	}

	return count
}

func (container *Container) GetItemCountByOwner(itemId uint32, bind int8, ownerId uint64) (count int64) {
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return
	}

	//判断是否是货币资源
	if itemdef.IsMoney(conf.Type, conf.SubType) {
		if nil != container.GetMoneyHandle {
			return container.GetMoneyHandle(conf.SubType)
		}
	}

	now := time_util.NowSec()
	for _, v := range *container.itemVec {
		if v.GetItemId() != itemId {
			continue
		}
		if bind != -1 {
			if v.GetBind() != (bind == 1) {
				continue
			}
		}

		timeout := v.GetTimeOut()
		if timeout > 0 && timeout <= now {
			continue
		}

		if container.needSameOwner && !container.isSameOwner(v.Ext, ownerId) {
			continue
		}

		count += int64(v.GetCount())
	}

	return count
}

func (container *Container) FindItemByHandle(handle uint64) *pb3.ItemSt {
	if item, ok := container.itemMap[handle]; ok {
		return item
	}
	return nil
}

func (container *Container) GetItemListByItemId(itemId, count uint32) []uint64 {
	var result []uint64
	for _, item := range *container.itemVec {
		if item.ItemId != itemId {
			continue
		}
		if count > 0 && uint32(len(result)) >= count {
			break
		}
		result = append(result, item.GetHandle())
	}
	return result
}

func (container *Container) RandomItem(itemId uint32) (*pb3.ItemSt, bool) {
	for _, item := range *container.itemVec {
		if item.ItemId != itemId {
			continue
		}
		return item, true
	}
	return nil, false
}

func (container *Container) NeedSameOwner(need bool) {
	container.needSameOwner = need
}

// 堆叠到已存在的道具
func (container *Container) overlapToExists(param *itemdef.ItemParamSt, remainCount int64) int64 {
	conf := jsondata.GetItemConfig(param.ItemId)
	if nil == conf || itemdef.CannotOverlap(conf.Type) {
		return 0
	}

	var ret int64
	for _, info := range *container.itemVec {
		if remainCount <= 0 {
			break
		}
		if info.GetItemId() != param.ItemId || info.GetBind() != param.Bind {
			continue
		}
		cur := info.GetCount()
		if cur >= conf.Dup {
			continue
		}

		if container.needSameOwner && !container.isSameOwner(info.Ext, param.OwnerId) {
			continue
		}

		space := conf.Dup - cur
		if space > remainCount {
			space = remainCount
		}
		info.Count = space + cur
		remainCount -= space
		ret += space

		if nil != container.OnItemChange {
			container.OnItemChange(info, space, common.EngineGiveRewardParam{LogId: param.LogId, NoTips: param.NoTips})
		}
	}

	return ret
}

// isSameOwner 判断是否该道具是属于该玩家的
func (container *Container) isSameOwner(ext *pb3.ItemExt, ownerId uint64) bool {
	return ext == nil || ext.OwnerId == 0 || ext.OwnerId == ownerId
}

func (container *Container) Sort() (success bool, updateVec []*pb3.ItemSt, deleteVec []uint64) {
	size := len(*container.itemVec)
	if 0 >= size {
		return
	}
	updateMap := make(map[uint64]bool)
	deleteMap := make(map[uint64]bool)

	itemVec := container.itemVec
	for end := len(*itemVec) - 1; end > 0; end-- {
		item := (*itemVec)[end]
		conf := jsondata.GetItemConfig(item.GetItemId())
		if nil == conf {
			continue
		}
		for begin := 0; begin <= end-1; begin++ {
			src := (*itemVec)[end]
			dst := (*itemVec)[begin]

			srcId, dstId := src.GetItemId(), dst.GetItemId()
			if srcId != dstId {
				continue
			}

			srcBind, dstBind := src.GetBind(), dst.GetBind()
			if srcBind != dstBind {
				continue
			}

			srcHandle, dstHandle := src.GetHandle(), dst.GetHandle()
			srcCount, dstCount := src.GetCount(), dst.GetCount()

			del := false
			if dstCount < conf.Dup {
				overlap := conf.Dup - dstCount
				if srcCount >= overlap {
					src.Count = srcCount - overlap
					updateMap[srcHandle] = true
				} else {
					overlap = srcCount
					deleteMap[srcHandle] = true
					(*itemVec)[end] = nil
					*itemVec = append((*itemVec)[:end], (*itemVec)[end+1:]...)
					del = true
					delete(updateMap, srcHandle)
					delete(container.itemMap, src.GetHandle())
				}
				dst.Count = dstCount + overlap
				updateMap[dstHandle] = true
				success = true
				if del {
					break
				}
			}
		}
	}

	for handle := range updateMap {
		if item, ok := container.itemMap[handle]; ok {
			updateVec = append(updateVec, item)
		}
	}

	for handle := range deleteMap {
		deleteVec = append(deleteVec, handle)
	}
	return
}

func (container *Container) Split(handle uint64, count int64, logId pb3.LogId) uint64 {
	if container.AvailableCount() <= 0 {
		return 0
	}

	item := container.FindItemByHandle(handle)
	if nil == item || item.GetCount() <= count {
		return 0
	}

	item.Count -= count
	if nil != container.OnItemChange {
		container.OnItemChange(item, 0-count, common.EngineGiveRewardParam{LogId: logId, NoTips: false})
	}

	newItem := item.Copy()
	newItem.Count = count

	if _, err := container.addNewItem(newItem, false, logId); err != nil {
		logger.LogError(err.Error())
		return 0
	}

	return newItem.GetHandle()
}

func (container *Container) MergeItem(fromHandle, toHandle uint64, logId pb3.LogId) {
	from := container.FindItemByHandle(fromHandle)
	to := container.FindItemByHandle(toHandle)
	if nil == from || nil == to {
		return
	}
	itemId := from.GetItemId()
	if itemId != to.GetItemId() {
		return
	}
	if from.GetBind() != to.GetBind() {
		return
	}
	if from.GetTimeOut() != to.GetTimeOut() {
		return
	}
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return
	}
	tCount, fCount := to.GetCount(), from.GetCount()
	if tCount >= conf.Dup {
		return
	}

	diff := conf.Dup - tCount
	if diff >= from.GetCount() {
		to.Count = tCount + fCount
		if nil != container.OnItemChange {
			container.OnItemChange(to, fCount, common.EngineGiveRewardParam{LogId: logId, NoTips: false})
		}
		container.DeleteItemPtr(from, fCount, logId)
	} else {
		to.Count = tCount + diff
		if nil != container.OnItemChange {
			container.OnItemChange(to, diff, common.EngineGiveRewardParam{LogId: logId, NoTips: false})
		}
		container.DeleteItemPtr(from, diff, logId)
	}
}

func (container *Container) addNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) (uint64, error) {
	var err error
	item.Handle, err = series.AllocSeries()
	if err != nil {
		logger.LogError(err.Error())
		return 0, err
	}
	logger.LogDebug("new add handler:%d", item.Handle)
	*container.itemVec = append(*container.itemVec, item)
	container.itemMap[item.GetHandle()] = item
	if nil != container.OnAddNewItem {
		container.OnAddNewItem(item, bTip, logId)
	}

	if nil != container.AppendNewAddItem {
		container.AppendNewAddItem(item)
	}
	return item.Handle, nil
}

func (container *Container) CheckItemBind(itemId uint32, isBind bool) bool {
	for _, v := range *container.itemVec {
		if v.GetItemId() == itemId && v.GetBind() == isBind {
			return true
		}
	}
	return false
}
