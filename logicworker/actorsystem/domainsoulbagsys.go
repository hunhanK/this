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

var _ iface.IBagSys = (*DomainSoulBagSys)(nil)

type DomainSoulBagSys struct {
	Base
	*miscitem.Container
}

func (s *DomainSoulBagSys) IsOpen() bool {
	return true
}

func (s *DomainSoulBagSys) OnInit() {
	mainData := s.GetMainData()

	if nil == mainData.ItemPool {
		mainData.ItemPool = &pb3.ItemPool{}
	}

	if nil == mainData.ItemPool.DomainSoulBag {
		mainData.ItemPool.DomainSoulBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.DomainSoulBag)

	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr

	s.Container = container
}

func (s *DomainSoulBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagDomainSoul)
}

func (s *DomainSoulBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *DomainSoulBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *DomainSoulBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DomainSoulBagSys) s2cInfo() {
	s.SendProto3(144, 45, &pb3.S2C_144_45{
		Items: s.GetMainData().GetItemPool().DomainSoulBag,
	})
}

func (s *DomainSoulBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(144, 46, &pb3.S2C_144_46{
		Items:   []*pb3.ItemSt{item},
		LogId:   uint32(logId),
		LogType: common.LogType[logId],
	})

	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *DomainSoulBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(144, 46, &pb3.S2C_144_46{
		Items: []*pb3.ItemSt{item},
		LogId: uint32(param.LogId),
	})

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

func (s *DomainSoulBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(144, 47, &pb3.S2C_144_47{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *DomainSoulBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiDomainSoulBag, func() iface.ISystem {
		return &DomainSoulBagSys{}
	})
}
