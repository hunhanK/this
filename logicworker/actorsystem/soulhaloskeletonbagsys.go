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

var _ iface.IBagSys = (*SoulHaloSkeletonBagSys)(nil)

type SoulHaloSkeletonBagSys struct {
	Base
	*miscitem.Container
}

func (s *SoulHaloSkeletonBagSys) IsOpen() bool {
	return true
}

func (s *SoulHaloSkeletonBagSys) OnInit() {
	mainData := s.GetMainData()

	if nil == mainData.ItemPool {
		mainData.ItemPool = &pb3.ItemPool{}
	}

	if nil == mainData.ItemPool.SoulHaloSkeletonBag {
		mainData.ItemPool.SoulHaloSkeletonBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.SoulHaloSkeletonBag)

	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr

	s.Container = container
}

func (s *SoulHaloSkeletonBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SoulHaloSkeletonBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SoulHaloSkeletonBagSys) s2cInfo() {
	s.SendProto3(67, 66, &pb3.S2C_67_66{
		Items: s.GetMainData().GetItemPool().SoulHaloSkeletonBag,
	})
}

func (s *SoulHaloSkeletonBagSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagSoulHaloSkeleton)
}

func (s *SoulHaloSkeletonBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *SoulHaloSkeletonBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(67, 67, &pb3.S2C_67_67{
		Items:   []*pb3.ItemSt{item},
		LogId:   uint32(logId),
		LogType: common.LogType[pb3.LogId(logId)],
	})

	s.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		s.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		s.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (s *SoulHaloSkeletonBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(67, 67, &pb3.S2C_67_67{
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

func (s *SoulHaloSkeletonBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(67, 68, &pb3.S2C_67_68{
		Handles: []uint64{item.GetHandle()},
	})

	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *SoulHaloSkeletonBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func init() {
	RegisterSysClass(sysdef.SiSoulHaloSkeletonBag, func() iface.ISystem {
		return &SoulHaloSkeletonBagSys{}
	})
}
