/**
 * @Author: DaiGuanYu
 * @Desc: 物品id容器
 * @Date: 2021/8/17 19:47
 */

package miscitem

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/iface"
)

type (
	ItemIdContainer struct {
		CheckTakeOnPosHandle func(itemId uint32, pos uint32, job uint32) bool // 穿戴部位检测
		GetItem              func(hdl uint64) *pb3.ItemSt
		DelItem              func(hdl uint64, log uint32) (result bool)
		AddItem              func(equip *pb3.ItemSt, bTip bool, log uint32) (result bool)
		GetBagAvailable      func() uint32
		ResetProp            func()
		array                *[]*pb3.SimpleItem
		SuitLv               uint32
		LogId                uint32
	}
)

func NewItemIdContainer(vec *[]*pb3.SimpleItem) *ItemIdContainer {
	container := new(ItemIdContainer)
	container.array = vec
	return container
}

func (container *ItemIdContainer) GetEquipVec() []*pb3.SimpleItem {
	return *container.array
}

func (container *ItemIdContainer) GetEquipPosById(itemId uint32) uint32 {
	if nil == container.array {
		return 1
	}
	for _, st := range *container.array {
		if st.GetItemId() == itemId {
			return uint32(st.GetPos())
		}
	}
	return 2
}

func (container *ItemIdContainer) GetEquipByPos(pos uint32) (uint32, *pb3.SimpleItem) {
	if nil == container.array {
		return 1, nil
	}
	for idx, st := range *container.array {
		if st.GetPos() == pos {
			return uint32(idx), st
		}
	}

	return 2, nil
}

func (container *ItemIdContainer) GetEquipIdx(itemId uint32) (idx uint32) {
	if nil == container.array {
		return 1
	}
	for idx, st := range *container.array {
		if st.GetItemId() == itemId {
			return uint32(idx)
		}
	}

	return 1
}

func (container *ItemIdContainer) TakeOn(hdl uint64) (uint32, *pb3.SimpleItem) {
	if nil == container.array {
		return 1, nil
	}

	if nil == container.GetItem {
		return 2, nil
	}

	st := container.GetItem(hdl)
	if nil == st {
		return 3, nil
	}

	if nil == container.CheckTakeOnPosHandle {
		return 4, nil
	}

	itemId := st.GetItemId()
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return 5, nil
	}

	pos := conf.SubType

	if !container.CheckTakeOnPosHandle(itemId, pos, uint32(conf.Job)) {
		return 6, nil
	}

	_, equip := container.GetEquipByPos(pos)
	if nil != equip {
		if errCode := container.TakeOff(pos); errCode != 0 {
			return 7, nil
		}
	}

	if !container.DelItem(hdl, container.LogId) {
		return 8, nil
	}

	equip = &pb3.SimpleItem{
		ItemId: itemId,
		Pos:    pos,
	}

	*(container.array) = append(*(container.array), equip)

	if nil != container.ResetProp {
		container.ResetProp()
	}

	return 0, equip
}

func (container *ItemIdContainer) TakeOnByPos(hdl uint64, pos uint32) (uint32, *pb3.SimpleItem) {
	if nil == container.array {
		return 1, nil
	}

	if nil == container.GetItem {
		return 2, nil
	}

	st := container.GetItem(hdl)
	if nil == st {
		return 3, nil
	}

	if nil == container.CheckTakeOnPosHandle {
		return 4, nil
	}

	itemId := st.GetItemId()
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return 5, nil
	}

	if !container.CheckTakeOnPosHandle(itemId, pos, uint32(conf.Job)) {
		return 6, nil
	}

	_, equip := container.GetEquipByPos(pos)
	if nil != equip {
		if errCode := container.TakeOff(pos); errCode != 0 {
			return 7, nil
		}
	}

	if !container.DelItem(hdl, container.LogId) {
		return 8, nil
	}

	equip = &pb3.SimpleItem{
		ItemId: itemId,
		Pos:    pos,
	}

	*(container.array) = append(*(container.array), equip)

	if nil != container.ResetProp {
		container.ResetProp()
	}

	return 0, equip
}

func (container *ItemIdContainer) TakeOff(pos uint32) uint32 {
	idx, equip := container.GetEquipByPos(pos)
	if equip != nil {
		if nil != container.GetBagAvailable {
			if container.GetBagAvailable() <= 0 {
				return 1
			}
		}

		if result := container.DelEquip(idx, equip); result != 0 {
			return 2
		}

		hdl, err := series.AllocSeries()
		if err != nil {
			logger.LogError(err.Error())
			return 2
		}

		var item = &pb3.ItemSt{
			ItemId: equip.GetItemId(),
			Count:  1,
			Bind:   true,
			Handle: hdl,
		}

		container.AddItem(item, true, container.LogId)

		if nil != container.ResetProp {
			container.ResetProp()
		}
	} else {
		return custom_id.Success
	}

	return custom_id.Success
}

func (container *ItemIdContainer) DelEquip(idx uint32, equip *pb3.SimpleItem) (result int) {
	if nil == container.array {
		return 1
	}

	size := len(*container.array)
	if size <= 0 {
		return 2
	}

	last := size - 1
	if idx != uint32(last) {
		(*container.array)[idx] = (*container.array)[last]
	}
	*container.array = (*container.array)[:last]

	equip.Pos = 0

	return 0
}

func (container *ItemIdContainer) UpGradeEquip(owner iface.IPlayer,
	pos uint32, takeOnId uint32, targetId uint32, consume []*jsondata.Consume, autoBuy bool) *pb3.SimpleItem {
	if nil == owner {
		return nil
	}
	_, st := container.GetEquipByPos(pos)
	if takeOnId > 0 {
		if nil == st || st.GetItemId() != takeOnId {
			return nil
		}
	}
	if !owner.ConsumeByConf(consume, autoBuy, common.ConsumeParams{LogId: pb3.LogId_LogUpgradeEquip}) {
		return nil
	}

	job := owner.GetJob()
	if nil != container.CheckTakeOnPosHandle && !container.CheckTakeOnPosHandle(targetId, pos, job) || (takeOnId == 0 && st != nil) {
		// 添加到背包
		var reward []*jsondata.StdReward
		reward = append(reward, &jsondata.StdReward{
			Id:    targetId,
			Count: 1,
			Bind:  true,
			Job:   job,
		})

		engine.GiveRewards(owner, reward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogUpgradeEquip})
	} else {

		if nil != st {
			st.ItemId = targetId
		} else {
			st = &pb3.SimpleItem{
				ItemId: targetId,
				Pos:    pos,
			}
			*(container.array) = append(*(container.array), st)
		}
		container.ResetProp()
	}
	return st
}
