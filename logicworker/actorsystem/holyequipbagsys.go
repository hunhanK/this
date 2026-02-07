/**
 * @Author: lzp
 * @Date: 2025/3/10
 * @Desc:
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

var _ iface.IBagSys = (*HolyEquipBagSys)(nil)

type HolyEquipBagSys struct {
	Base
	*miscitem.Container
}

func (s *HolyEquipBagSys) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.HolyEquips {
		itemPool.HolyEquips = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.HolyEquips)
	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr
	s.Container = container
}

func (s *HolyEquipBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *HolyEquipBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HolyEquipBagSys) IsOpen() bool {
	return true
}

func (s *HolyEquipBagSys) s2cInfo() {
	s.SendProto3(76, 6, &pb3.S2C_76_6{
		Items: s.GetMainData().GetItemPool().HolyEquips,
	})
}

func (s *HolyEquipBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagHolyEquip)
}

func (s *HolyEquipBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *HolyEquipBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(76, 7, &pb3.S2C_76_7{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *HolyEquipBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(76, 7, &pb3.S2C_76_7{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
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

func (s *HolyEquipBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(76, 8, &pb3.S2C_76_8{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *HolyEquipBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiHolyEquipBag, func() iface.ISystem {
		return &HolyEquipBagSys{}
	})
}
