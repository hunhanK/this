/**
 * @Author: lzp
 * @Date: 2025/7/23
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

var _ iface.IBagSys = (*FeatherBagSys)(nil)

type FeatherBagSys struct {
	Base
	*miscitem.Container
}

func (s *FeatherBagSys) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.FeatherEquBag {
		itemPool.FeatherEquBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.FeatherEquBag)
	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr
	s.Container = container
}

func (s *FeatherBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *FeatherBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FeatherBagSys) IsOpen() bool {
	return true
}

func (s *FeatherBagSys) s2cInfo() {
	s.SendProto3(81, 10, &pb3.S2C_81_10{
		Items: s.GetMainData().GetItemPool().FeatherEquBag,
	})
}

func (s *FeatherBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagFeatherEqu)
}

func (s *FeatherBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *FeatherBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.initFeatherEquip(item)

	s.SendProto3(81, 11, &pb3.S2C_81_11{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *FeatherBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(81, 11, &pb3.S2C_81_11{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
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

func (s *FeatherBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(81, 12, &pb3.S2C_81_12{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *FeatherBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func (s *FeatherBagSys) initFeatherEquip(item *pb3.ItemSt) {
	if nil == item.Ext {
		item.Ext = &pb3.ItemExt{}
	}
	if item.Ext.IsInitExt {
		return
	}
	item.Ext.IsInitExt = true
	item.Union2 = 1 //默认阶级1
}

func init() {
	RegisterSysClass(sysdef.SiFeatherBag, func() iface.ISystem {
		return &FeatherBagSys{}
	})
}
