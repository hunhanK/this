/**
 * @Author: LvYuMeng
 * @Date: 2025/12/22
 * @Desc: 战纹背包
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

var _ iface.IBagSys = (*WarPaintBagSys)(nil)

type WarPaintBagSys struct {
	Base
	*miscitem.Container
}

func (s *WarPaintBagSys) IsOpen() bool {
	return true
}

func (s *WarPaintBagSys) GetSysData() *[]*pb3.ItemSt {
	mainData := s.GetMainData()

	if nil == mainData.ItemPool {
		mainData.ItemPool = &pb3.ItemPool{}
	}

	var target *[]*pb3.ItemSt

	switch s.GetSysId() {
	case sysdef.SiWarPaintFairyWingBag:
		target = &mainData.ItemPool.WarPaintFairyWingBag
	case sysdef.SiWarPaintGodWeaponBag:
		target = &mainData.ItemPool.WarPaintGodWeaponBag
	case sysdef.SiWarPaintBattleShieldBag:
		target = &mainData.ItemPool.WarPaintBattleShieldBag
	default:
		return nil
	}

	if *target == nil {
		*target = make([]*pb3.ItemSt, 0)
	}

	return target
}

func (s *WarPaintBagSys) OnInit() {
	sysData := s.GetSysData()

	container := miscitem.NewContainer(sysData)

	container.DefaultSizeHandle = s.DefaultSize
	container.EnlargeSizeHandle = s.EnlargeSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.OnDeleteItemPtr = s.owner.OnDeleteItemPtr

	s.Container = container
}

func (s *WarPaintBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *WarPaintBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WarPaintBagSys) s2cInfo() {
	s.SendProto3(15, 30, &pb3.S2C_15_30{
		SysId: s.GetSysId(),
		Items: *s.GetSysData(),
	})
}

func (s *WarPaintBagSys) DefaultSize() uint32 {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByBagSysId(s.GetSysId())
	if !ok {
		return 0
	}
	bagSize := int(ref.BagType)
	return jsondata.GetBagDefaultSize(bagSize)
}

func (s *WarPaintBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *WarPaintBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendProto3(15, 31, &pb3.S2C_15_31{
		SysId:   s.GetSysId(),
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

func (s *WarPaintBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(15, 31, &pb3.S2C_15_31{
		SysId: s.GetSysId(),
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

func (s *WarPaintBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(15, 32, &pb3.S2C_15_32{
		SysId:   s.GetSysId(),
		Handles: []uint64{item.GetHandle()},
	})

	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *WarPaintBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func regWarPaintBagSys() {
	gshare.WarPaintInstance.EachWarPaintRefDo(func(ref *gshare.WarPaintRef) {
		RegisterSysClass(ref.BagSysId, func() iface.ISystem {
			return &WarPaintBagSys{}
		})
	})
}

func init() {
	regWarPaintBagSys()
}
