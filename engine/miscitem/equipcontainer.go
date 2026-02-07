/**
 * @Author: ChenJunJi
 * @Desc: 装备容器
 * @Date: 2021/8/11 10:38
 */

package miscitem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/iface"
	"sort"
)

type EquipContainer struct {
	CheckTakeOnPosHandle func(st *pb3.ItemSt, pos uint32) bool // 穿戴部位检测
	GetItem              func(hdl uint64) *pb3.ItemSt
	DelItem              func(hdl uint64, log pb3.LogId) (result bool)
	AddItem              func(equip *pb3.ItemSt, bTip bool, log pb3.LogId) (result bool)
	AfterTakeOn          func(equip *pb3.ItemSt)
	AfterTakeOff         func(equip *pb3.ItemSt, pos uint32)
	AfterReplace         func(newEquip *pb3.ItemSt, oldEquip *pb3.ItemSt, pos uint32)
	GetBagAvailable      func() uint32
	ResetProp            func()
	SortBag              func()
	Split                func(handle uint64, count int64, logId pb3.LogId) uint64
	array                *[]*pb3.ItemSt

	TakeOnLogId  pb3.LogId
	TakeOffLogId pb3.LogId
	UpgradeLogId pb3.LogId
}

func NewEquipContainer(vec *[]*pb3.ItemSt) *EquipContainer {
	container := new(EquipContainer)
	container.array = vec

	container.TakeOnLogId = pb3.LogId_LogTakeOnEquip
	container.TakeOffLogId = pb3.LogId_LogTakeOffEquip
	container.TakeOffLogId = pb3.LogId_LogUpgradeEquip

	return container
}

func (container *EquipContainer) GetEquipPosById(itemId uint32) int {
	if nil == container.array {
		return -1
	}
	for _, st := range *container.array {
		if st.GetItemId() == itemId {
			return int(st.GetPos())
		}
	}
	return -1
}

func (container *EquipContainer) GetEquipByPos(pos uint32) (int, *pb3.ItemSt) {
	if nil == container.array {
		return 0, nil
	}
	for idx, st := range *container.array {
		if st.GetPos() == pos {
			return idx, st
		}
	}

	return 0, nil
}

func (container *EquipContainer) GetEquipByPosAndType(pos, equipType uint32) (int, *pb3.ItemSt) {
	if nil == container.array {
		return 0, nil
	}
	for idx, st := range *container.array {
		conf := jsondata.GetItemConfig(st.GetItemId())
		if nil == conf {
			continue
		}

		if st.GetPos() == pos && conf.Type == equipType {
			return idx, st
		}
	}

	return 0, nil
}
func (container *EquipContainer) GetEquipIdx(hdl uint64) (idx int) {
	if nil == container.array {
		return -1
	}
	for idx, st := range *container.array {
		if st.GetHandle() == hdl {
			return idx
		}
	}

	return -1
}

func (container *EquipContainer) GetEquipByHdl(hdl uint64) *pb3.ItemSt {
	if nil == container.array {
		return nil
	}
	for _, st := range *container.array {
		if st.GetHandle() == hdl {
			return st
		}
	}

	return nil
}

func (container *EquipContainer) Replace(hdl uint64, pos uint32) error {
	if nil == container.array {
		return neterror.InternalError("equip array is nil")
	}
	idx, oldEquip := container.GetEquipByPos(pos)
	if nil == oldEquip {
		return neterror.ParamsInvalidError("no equip in pos(%d)", pos)
	}

	if nil == container.GetItem {
		return neterror.InternalError("container GetItem func id nil")
	}

	st := container.GetItem(hdl)
	if nil == st {
		return neterror.ParamsInvalidError("no equip(%d) in bag", hdl)
	}

	if nil == container.CheckTakeOnPosHandle {
		return neterror.InternalError("container CheckTakeOnPosHandle func id nil")
	}

	if !container.CheckTakeOnPosHandle(st, pos) {
		return neterror.ParamsInvalidError("equip(%d) cond check not pass", hdl)
	}
	if !container.DelItem(hdl, container.TakeOnLogId) {
		return neterror.ParamsInvalidError("remove equip(%d),itemId(%d) from bag failed", hdl, st.GetItemId())
	}

	if result := container.DelEquip(uint32(idx), oldEquip); result != 0 {
		return neterror.ParamsInvalidError("take off equip(%d),itemId(%d) to bag failed", oldEquip.GetHandle(), oldEquip.GetItemId())
	}

	if !container.AddItem(oldEquip, true, container.TakeOffLogId) {
		return neterror.ParamsInvalidError("take off equip(%d),itemId(%d) to bag failed", oldEquip.GetHandle(), oldEquip.GetItemId())
	}
	st.Pos = pos

	*(container.array) = append(*(container.array), st)

	if nil != container.AfterTakeOff {
		container.AfterTakeOff(oldEquip, pos)
	}

	if nil != container.AfterTakeOn {
		container.AfterTakeOn(st)
	}

	if nil != container.ResetProp {
		container.ResetProp()
	}
	return nil
}

func (container *EquipContainer) TakeOn(hdl uint64, pos uint32) (error, *pb3.ItemSt) {
	if nil == container.array {
		return neterror.InternalError("equip array is nil"), nil
	}

	if nil == container.GetItem {
		return neterror.InternalError("container GetItem func id nil"), nil
	}

	st := container.GetItem(hdl)
	if nil == st {
		return neterror.ParamsInvalidError("no equip(%d) in bag", hdl), nil
	}

	if nil == container.CheckTakeOnPosHandle {
		return neterror.InternalError("container CheckTakeOnPosHandle func id nil"), nil
	}

	if !container.CheckTakeOnPosHandle(st, pos) {
		return neterror.ParamsInvalidError("equip(%d) cond check not pass", hdl), nil
	}

	_, equip := container.GetEquipByPos(pos)
	if nil != equip {
		if err := container.TakeOff(pos); err != nil {
			return err, nil
		}
	}

	if st.GetCount() > 1 && nil != container.Split {
		if newHdl := container.Split(hdl, 1, container.TakeOnLogId); newHdl > 0 {
			hdl = newHdl
			st = container.GetItem(hdl)
		}
	}

	if !container.DelItem(hdl, container.TakeOnLogId) {
		return neterror.ParamsInvalidError("remove equip(%d),itemId(%d) from bag failed", hdl, st.GetItemId()), nil
	}

	st.Pos = pos

	*(container.array) = append(*(container.array), st)

	if nil != container.AfterTakeOn {
		container.AfterTakeOn(st)
	}

	if nil != container.ResetProp {
		container.ResetProp()
	}

	return nil, st
}

func (container *EquipContainer) TakeOff(pos uint32) error {
	idx, equip := container.GetEquipByPos(pos)
	if equip != nil && idx >= 0 {
		if container.GetBagAvailable == nil {
			return neterror.InternalError("container GetBagAvailable func is nil")
		}

		if container.GetBagAvailable() <= 0 {
			return neterror.ParamsInvalidError("container GetBagAvailable func is nil")
		}

		if result := container.DelEquip(uint32(idx), equip); result != 0 {
			return neterror.ParamsInvalidError("take off equip(%d),itemId(%d) to bag failed", equip.GetHandle(), equip.GetItemId())
		}

		equip.Bind = true

		if !container.AddItem(equip, true, container.TakeOffLogId) {
			return neterror.ParamsInvalidError("take off equip(%d),itemId(%d) to bag failed", equip.GetHandle(), equip.GetItemId())
		}

		if nil != container.AfterTakeOff {
			container.AfterTakeOff(equip, pos)
		}

		if nil != container.SortBag {
			container.SortBag()
		}

		if nil != container.ResetProp {
			container.ResetProp()
		}
	} else {
		return nil
	}

	return nil
}

func (container *EquipContainer) DelEquip(idx uint32, equip *pb3.ItemSt) (result int) {
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

func (container *EquipContainer) UpGradeEquip(owner iface.IPlayer,
	pos uint32, takeOnId uint32, targetId uint32, consume []*jsondata.Consume, autoBuy bool) *pb3.ItemSt {
	if nil == owner {
		return nil
	}
	_, st := container.GetEquipByPos(pos)
	if takeOnId > 0 {
		if nil == st || st.GetItemId() != takeOnId {
			return nil
		}
	}
	if !owner.ConsumeByConf(consume, autoBuy, common.ConsumeParams{LogId: container.UpgradeLogId}) {
		return nil
	}

	if nil != st {
		st.ItemId = targetId
	} else {
		hdl, err := series.AllocSeries()
		if err != nil {
			logger.LogError(err.Error())
			return nil
		}
		st = &pb3.ItemSt{
			ItemId: targetId,
			Count:  1,
			Bind:   true,
			Handle: hdl,
			Pos:    pos,
		}
		*(container.array) = append(*(container.array), st)
	}

	if container.ResetProp != nil {
		container.ResetProp()
	}
	return st
}

func (container *EquipContainer) GetEquipVec() []*pb3.ItemSt {
	return *container.array
}

func (container *EquipContainer) GetSortedStageVec(pos []uint32) []uint32 {
	return GetSortedStageVec(container.GetEquipVec(), pos)
}

func GetSortedStageVec(arr []*pb3.ItemSt, pos []uint32) []uint32 {
	var stageVec = make([]uint32, 0, 16)
	for _, equip := range arr {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}
		if len(pos) > 0 && !utils.SliceContainsUint32(pos, conf.SubType) {
			continue
		}
		stageVec = append(stageVec, conf.Stage)
	}

	sort.Slice(stageVec, func(i, j int) bool {
		return stageVec[i] > stageVec[j]
	})

	return stageVec
}
