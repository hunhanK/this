/**
 * @Author: LvYuMeng
 * @Date: 2024/10/30
 * @Desc:武魂神饰背包
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

var _ iface.IBagSys = (*BattleSoulGodEquipBagSys)(nil)

type BattleSoulGodEquipBagSys struct {
	Base
	*miscitem.Container
}

func (s *BattleSoulGodEquipBagSys) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.BattleSoulGodEquips {
		itemPool.BattleSoulGodEquips = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.BattleSoulGodEquips)
	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr
	s.Container = container
}

func (s *BattleSoulGodEquipBagSys) IsOpen() bool {
	return true
}

func (s *BattleSoulGodEquipBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *BattleSoulGodEquipBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BattleSoulGodEquipBagSys) s2cInfo() {
	s.SendProto3(11, 100, &pb3.S2C_11_100{
		Items: s.GetMainData().GetItemPool().BattleSoulGodEquips,
	})
}

func (s *BattleSoulGodEquipBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagBattleSoulGodEquip)
}

func (s *BattleSoulGodEquipBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *BattleSoulGodEquipBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.initGodEquip(item)

	s.SendProto3(11, 101, &pb3.S2C_11_101{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *BattleSoulGodEquipBagSys) initGodEquip(item *pb3.ItemSt) {
	if nil == item.Ext {
		item.Ext = &pb3.ItemExt{}
	}
	if item.Ext.IsInitExt {
		return
	}
	item.Ext.IsInitExt = true
	item.Union2 = 1 //默认阶级1
}

func (s *BattleSoulGodEquipBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(11, 101, &pb3.S2C_11_101{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
	s.changeStat(item, add, false, param.LogId)
	if add > 0 {
		if !param.NoTips {
			s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), add)
		}

		if checkLogId(param.LogId) {
			s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), add)
		}
	}
}

func (s *BattleSoulGodEquipBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(11, 102, &pb3.S2C_11_102{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *BattleSoulGodEquipBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiBattleSoulGodEquipBag, func() iface.ISystem {
		return &BattleSoulGodEquipBagSys{}
	})
}
