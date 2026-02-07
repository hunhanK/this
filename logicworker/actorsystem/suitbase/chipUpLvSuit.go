package suitbase

import (
	"errors"
	"fmt"
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"math"
)

// ChipUpLvSuit 碎片升级套装
// 填入碎片后，根据碎片数量判断是否激活套装
// 套装的每个部位需要消耗道具进行激活和升级 已经激活的道具无法再卸下
// 套装的等级由碎片的最小等级决定
// 会自动要求属性系统重新计算属性
type ChipIdBySlotHandler func(slot uint32) uint32
type ChipLvConfHandler func(slot uint32, lv uint32) *jsondata.ConsumeUpLvConf
type AfterChipUpLvCb func(player iface.IPlayer, slot uint32)

type ChipUpLvSuit struct {
	Chips     map[uint32]*pb3.IdLvSt // key: 碎片位置 value: 碎片信息
	AttrSysId uint32

	LogId pb3.LogId // 日志id

	GetChipIdBySlotHandler func(slot uint32) uint32 // 获取碎片id
	GetChipLvConfHandler   func(slot uint32, lv uint32) *jsondata.ConsumeUpLvConf

	AfterChipUpLvCb   func(player iface.IPlayer, slot uint32) // 碎片升级后回调
	AfterSuitActiveCb func()                                  // 套装激活后回调
	AfterSuitUpLvCb   func()                                  // 套装等级变化后回调

	SuitNum uint32 // 套装需要的装备数量
}

func (e *ChipUpLvSuit) Init() error {
	if e.Chips == nil {
		return errors.New("Chips is nil")
	}

	if e.AttrSysId <= 0 {
		return errors.New("AttrSysId incorrect")
	}

	if e.SuitNum <= 0 {
		return errors.New("SuitNum incorrect")
	}

	if e.AfterSuitActiveCb == nil {
		return errors.New("SuitActiveCb is nil")
	}

	return nil
}

func (e *ChipUpLvSuit) ChipUpLv(player iface.IPlayer, slot uint32, autoBuy bool) error {
	chipId := uint64(e.GetChipIdBySlotHandler(slot))
	chip := e.Chips[slot]

	suitStateBeforeUpLv := e.SuitActivated()

	if chip == nil {
		chip = &pb3.IdLvSt{
			Id: chipId,
		}
	}
	nextLv := chip.Lv + 1
	chipLvConf := e.GetChipLvConfHandler(slot, nextLv)
	if chipLvConf == nil {
		return fmt.Errorf("chipLvConf is nil slot:%v lv:%v", slot, nextLv)
	}

	oldSuitLv := e.GetSuitLv()

	if !player.ConsumeByConf(chipLvConf.Consume, autoBuy, common.ConsumeParams{LogId: e.LogId}) {
		return fmt.Errorf("consume failed %v", chipLvConf.Consume)
	}

	chip.Lv = nextLv
	e.Chips[slot] = chip

	if !suitStateBeforeUpLv && e.SuitActivated() {
		e.onSuitActive(player, e.GetSuitLv())
	}

	if oldSuitLv < e.GetSuitLv() && suitStateBeforeUpLv {
		e.onSuitUpLv(player, oldSuitLv, e.GetSuitLv())
	}

	e.onChipUpLv(player, slot)
	return nil
}

func (e *ChipUpLvSuit) onChipUpLv(player iface.IPlayer, slot uint32) {
	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	e.AfterChipUpLvCb(player, slot)
	return
}

func (e *ChipUpLvSuit) onSuitActive(player iface.IPlayer, suitLv uint32) {
	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	e.AfterSuitActiveCb()
	return
}

func (e *ChipUpLvSuit) SuitActivated() bool {
	return len(e.Chips) >= int(e.SuitNum)
}

func (e *ChipUpLvSuit) GetSuitLv() uint32 {
	if len(e.Chips) < int(e.SuitNum) {
		return 0
	}

	minLv := uint32(math.MaxUint32)

	for _, ils := range e.Chips {
		if ils.Lv < minLv {
			minLv = ils.Lv
		}
	}

	if minLv == math.MaxUint32 {
		return 0
	}

	return minLv
}

func (e *ChipUpLvSuit) onSuitUpLv(player iface.IPlayer, oldLv uint32, newLv uint32) {
	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	e.AfterSuitUpLvCb()
	return
}
