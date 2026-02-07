/**
 * @Author: zjj
 * @Date: 2025/7/30
 * @Desc:
**/

package miscitem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/pb3"
)

// BatchAtomicResult 批处理结果结构
type BatchAtomicResult struct {
	Success bool
	Failed  []*itemdef.ItemParamSt
}

// BatchAddItemsAtomic 原子批量添加道具
func (container *Container) BatchAddItemsAtomic(params []*itemdef.ItemParamSt) BatchAtomicResult {
	result := BatchAtomicResult{Success: true}

	// 快照当前状态
	originalVec := make([]*pb3.ItemSt, len(*container.itemVec))
	copy(originalVec, *container.itemVec)

	originalMap := make(map[uint64]*pb3.ItemSt)
	for k, v := range container.itemMap {
		originalMap[k] = v
	}

	// 临时禁用回调，避免中间状态污染
	oldOnAdd := container.OnAddNewItem
	oldOnChange := container.OnItemChange
	oldOnUse := container.OnUseOnGetItem

	container.OnAddNewItem = nil
	container.OnItemChange = nil
	container.OnUseOnGetItem = nil

	var newHandles []uint64

	defer func() {
		// 回滚
		if !result.Success {
			*container.itemVec = originalVec
			container.itemMap = originalMap
		}
		// 恢复回调
		container.OnAddNewItem = oldOnAdd
		container.OnItemChange = oldOnChange
		container.OnUseOnGetItem = oldOnUse
	}()

	for _, param := range params {
		originalAddItemAfter := param.AddItemAfterHdl

		if !container.AddItem(param) {
			result.Success = false
			result.Failed = append(result.Failed, param)
			break
		}
		newHandles = append(newHandles, param.AddItemAfterHdl)
		param.AddItemAfterHdl = originalAddItemAfter
	}

	if result.Success {
		for _, handle := range newHandles {
			if item, ok := container.itemMap[handle]; ok && oldOnAdd != nil {
				oldOnAdd(item, true, 0)
			}
		}
	}

	return result
}

// BatchDeleteItemsAtomic 原子批量删除道具
func (container *Container) BatchDeleteItemsAtomic(params []*itemdef.ItemParamSt) BatchAtomicResult {
	result := BatchAtomicResult{Success: true}

	// 快照状态
	originalVec := make([]*pb3.ItemSt, len(*container.itemVec))
	copy(originalVec, *container.itemVec)

	originalMap := make(map[uint64]*pb3.ItemSt)
	for k, v := range container.itemMap {
		originalMap[k] = v
	}

	oldOnRemove := container.OnRemoveItem
	oldOnChange := container.OnItemChange

	container.OnRemoveItem = nil
	container.OnItemChange = nil

	defer func() {
		if !result.Success {
			*container.itemVec = originalVec
			container.itemMap = originalMap
		}
		container.OnRemoveItem = oldOnRemove
		container.OnItemChange = oldOnChange
	}()

	for _, param := range params {
		if !container.DeleteItem(param) {
			result.Success = false
			result.Failed = append(result.Failed, param)
			break
		}
	}

	// 成功后补发回调
	if result.Success && oldOnChange != nil {
		for _, param := range params {
			oldOnChange(nil, -param.Count, common.EngineGiveRewardParam{
				LogId:  param.LogId,
				NoTips: false,
			})
		}
	}

	return result
}
