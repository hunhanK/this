/**
 * @Author: lzp
 * @Date: 2025/7/4
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

var _ iface.IBagSys = (*BloodEquBagSys)(nil)

type BloodEquBagSys struct {
	Base
	*miscitem.Container
}

func (s *BloodEquBagSys) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.BloodEquBag {
		itemPool.BloodEquBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.BloodEquBag)
	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr
	s.Container = container
}

func (s *BloodEquBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *BloodEquBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BloodEquBagSys) IsOpen() bool {
	return true
}

func (s *BloodEquBagSys) s2cInfo() {
	s.SendProto3(79, 30, &pb3.S2C_79_30{
		Items: s.GetMainData().GetItemPool().BloodEquBag,
	})
}

func (s *BloodEquBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagBloodEqu)
}

func (s *BloodEquBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *BloodEquBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(79, 31, &pb3.S2C_79_31{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *BloodEquBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(79, 31, &pb3.S2C_79_31{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
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

func (s *BloodEquBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(79, 32, &pb3.S2C_79_32{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *BloodEquBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiBloodEquBag, func() iface.ISystem {
		return &BloodEquBagSys{}
	})
}
