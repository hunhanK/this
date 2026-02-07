/**
 * @Author: lzp
 * @Date: 2025/3/5
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
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
	"jjyz/gameserver/net"
)

var _ iface.IBagSys = (*SourceSoulBagSys)(nil)

type SourceSoulBagSys struct {
	Base
	*miscitem.Container
}

func (s *SourceSoulBagSys) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.SourceSouls {
		itemPool.SourceSouls = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.SourceSouls)
	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr
	s.Container = container
}

func (s *SourceSoulBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SourceSoulBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SourceSoulBagSys) IsOpen() bool {
	return true
}

func (s *SourceSoulBagSys) s2cInfo() {
	s.SendProto3(4, 40, &pb3.S2C_4_40{
		Items: s.GetMainData().ItemPool.SourceSouls,
	})
}

func (s *SourceSoulBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagSourceSoul)
}

func (s *SourceSoulBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *SourceSoulBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(4, 41, &pb3.S2C_4_41{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *SourceSoulBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(4, 41, &pb3.S2C_4_41{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
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

func (s *SourceSoulBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(4, 42, &pb3.S2C_4_42{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *SourceSoulBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiSourceSoulBag, func() iface.ISystem {
		return &SourceSoulBagSys{}
	})
	net.RegisterSysProtoV2(4, 40, sysdef.SiSourceSoulBag, func(sys iface.ISystem) func(*base.Message) error {
		return func(message *base.Message) error {
			sys.(*SourceSoulBagSys).s2cInfo()
			return nil
		}
	})
}
