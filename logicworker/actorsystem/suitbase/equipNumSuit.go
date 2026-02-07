package suitbase

import (
	"errors"
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"math"
)

// EquipNumSuit 装备数量套装
// 填入装备后，根据装备数量判断是否激活套装
// 套装激活后的等级由填入装备的最小阶决定
// 会自动要求属性系统重新计算属性
type EquipNumSuit struct {
	Equips    map[uint32]uint32 // key: 装备位置 value: 装备id
	AttrSysId uint32

	TakeOnCheckHandler func(itemConf *jsondata.ItemConf) error // 装备穿戴检查

	EquipTakeOffCb func(itemConf *jsondata.ItemConf)                   // 装备卸下后回调
	EquipTakeOnCb  func(itemConf *jsondata.ItemConf)                   // 装备穿上后回调
	EquipReplaceCb func(newEquipConf, oldEquipConf *jsondata.ItemConf) // 装备替换后回调

	SuitActiveCb    func(lv uint32)                  // 套装激活后回调
	SuitDisActiveCb func(lv uint32)                  // 套装失效后回调
	SuitLvChangeCb  func(oldLv uint32, newLv uint32) // 套装等级变化后回调

	SuitNum uint32 // 套装需要的装备数量
}

func (e *EquipNumSuit) Init() error {
	if e.Equips == nil {
		return errors.New("Equips is nil")
	}

	if e.TakeOnCheckHandler == nil {
		return errors.New("TakeOnCheckHandler is nil")
	}

	if e.AttrSysId <= 0 {
		return errors.New("AttrSysId incorrect")
	}

	if e.SuitNum <= 0 {
		return errors.New("SuitNum incorrect")
	}

	if e.EquipTakeOffCb == nil {
		return errors.New("EquipTakeOffCb is nil")
	}

	if e.EquipTakeOnCb == nil {
		return errors.New("EquipTakeOnCb is nil")
	}

	if e.EquipReplaceCb == nil {
		return errors.New("EquipReplaceCb is nil")
	}

	if e.SuitActiveCb == nil {
		return errors.New("SuitActiveCb is nil")
	}

	if e.SuitDisActiveCb == nil {
		return errors.New("SuitDisActiveCb is nil")
	}

	if e.SuitLvChangeCb == nil {
		return errors.New("SuitLvChangeCb is nil")
	}

	return nil
}

func (e *EquipNumSuit) TakeOn(player iface.IPlayer, itemHdl uint64, itemConf *jsondata.ItemConf, logId pb3.LogId) error {
	item := player.GetItemByHandle(itemHdl)
	if item == nil {
		return neterror.ParamsInvalidError("item not found")
	}

	err := e.TakeOnCheckHandler(itemConf)
	if nil != err {
		return err
	}

	_, ok := e.Equips[itemConf.SubType]
	if ok {
		return e.Replace(player, itemHdl, itemConf, logId)
	}

	if !player.DeleteItemPtr(item, 1, logId) {
		return neterror.InternalError("item not enough")
	}

	e.Equips[itemConf.SubType] = itemConf.Id
	e.onTakeOn(player, itemConf)

	if len(e.Equips) >= int(e.SuitNum) {
		suitLv := e.GetSuitLv()
		e.onSuitActive(player, suitLv)
	}

	return nil
}

func (e *EquipNumSuit) EquipOnP(itemId uint32) bool {
	for _, v := range e.Equips {
		if v == itemId {
			return true
		}
	}

	return false
}

func (e *EquipNumSuit) TakeOnWithItemConfAndWithoutRewardToBag(player iface.IPlayer, itemConf *jsondata.ItemConf, logId uint32) error {
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemConf is nil")
	}

	err := e.TakeOnCheckHandler(itemConf)
	if nil != err {
		return err
	}

	e.Equips[itemConf.SubType] = itemConf.Id
	e.onTakeOn(player, itemConf)

	if len(e.Equips) >= int(e.SuitNum) {
		suitLv := e.GetSuitLv()
		e.onSuitActive(player, suitLv)
	}

	return nil
}

func (e *EquipNumSuit) onSuitActive(player iface.IPlayer, suitLv uint32) {
	e.SuitActiveCb(suitLv)
	return
}

func (e *EquipNumSuit) onTakeOn(player iface.IPlayer, itemConf *jsondata.ItemConf) {
	e.EquipTakeOnCb(itemConf)
	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	return
}

func (e *EquipNumSuit) GetSuitLv() uint32 {
	if len(e.Equips) < int(e.SuitNum) {
		return 0
	}

	minLv := uint32(math.MaxUint32)

	for _, itemId := range e.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf == nil {
			return 0
		}

		if itemConf.Stage < minLv {
			minLv = itemConf.Stage
		}
	}

	if minLv == math.MaxUint32 {
		return 0
	}
	return minLv
}

func (e *EquipNumSuit) Replace(player iface.IPlayer, itemHdl uint64, itemConf *jsondata.ItemConf, logId pb3.LogId) error {
	item := player.GetItemByHandle(itemHdl)
	if item == nil {
		return neterror.ParamsInvalidError("item not found")
	}

	err := e.TakeOnCheckHandler(itemConf)
	if nil != err {
		return err
	}

	oldEquip, ok := e.Equips[itemConf.SubType]
	if !ok {
		return neterror.ParamsInvalidError("item not found")
	}

	oldSuitLv := e.GetSuitLv()

	e.Equips[itemConf.SubType] = itemConf.Id
	if !player.DeleteItemPtr(item, 1, logId) {
		return neterror.InternalError("item not enough")
	}

	oldEquipConf := jsondata.GetItemConfig(oldEquip)

	if oldEquipConf == nil {
		return neterror.InternalError("item not found")
	}

	reward := &jsondata.StdReward{
		Id:    oldEquip,
		Count: 1,
	}

	param := common.EngineGiveRewardParam{
		LogId:  logId,
		NoTips: true,
	}
	if !engine.GiveRewards(player, []*jsondata.StdReward{reward}, param) {
		return neterror.InternalError("give reward failed")
	}

	if len(e.Equips) >= int(e.SuitNum) {
		newSuitLv := e.GetSuitLv()
		if oldSuitLv != newSuitLv {
			e.onSuitLvChange(player, oldSuitLv, newSuitLv)
		}
	}

	e.onReplace(player, itemConf, oldEquipConf)
	return nil
}

func (e *EquipNumSuit) onSuitLvChange(player iface.IPlayer, oldLv uint32, newLv uint32) {
	e.SuitLvChangeCb(oldLv, newLv)
	return
}

func (e *EquipNumSuit) onReplace(player iface.IPlayer, newEquipConf, oldEquipConf *jsondata.ItemConf) {
	e.EquipReplaceCb(newEquipConf, oldEquipConf)

	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	return
}

func (e *EquipNumSuit) TakeOff(player iface.IPlayer, pos uint32, logId pb3.LogId) error {
	itemId, ok := e.Equips[pos]
	if !ok {
		return neterror.ParamsInvalidError("item not found")
	}

	suitLv := e.GetSuitLv()
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return neterror.InternalError("itemConf not found")
	}

	rewardItem := &jsondata.StdReward{
		Id:    itemId,
		Count: 1,
	}
	param := common.EngineGiveRewardParam{
		LogId:  logId,
		NoTips: true,
	}
	if !engine.GiveRewards(player, []*jsondata.StdReward{rewardItem}, param) {
		return neterror.InternalError("give reward failed")
	}
	delete(e.Equips, pos)

	e.onTakeOff(player, itemConf)

	if len(e.Equips) == int(e.SuitNum)-1 {
		e.onSuitDisActive(player, suitLv)
	}

	return nil
}

func (e *EquipNumSuit) onSuitDisActive(player iface.IPlayer, lv uint32) {
	e.SuitDisActiveCb(lv)
	return
}

func (e *EquipNumSuit) onTakeOff(player iface.IPlayer, itemConf *jsondata.ItemConf) {
	e.EquipTakeOffCb(itemConf)
	player.GetAttrSys().ResetSysAttr(e.AttrSysId)
	return
}

func (e *EquipNumSuit) SuitActivated() bool {
	if len(e.Equips) >= int(e.SuitNum) {
		return true
	}
	return false
}

func (e *EquipNumSuit) GetCountMoreThanTheStage(stage uint32) uint32 {
	var count uint32
	for _, itemId := range e.Equips {
		if itemConf := jsondata.GetItemConfig(itemId); nil != itemConf {
			if itemConf.Stage >= stage {
				count++
			}
		}
	}
	return count
}
