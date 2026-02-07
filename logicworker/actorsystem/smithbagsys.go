/**
 * @Author: LvYuMeng
 * @Date: 2025/7/24
 * @Desc: 铁匠装备背包
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

var _ iface.IBagSys = (*SmithBagSys)(nil)

type SmithBagSys struct {
	Base
	*miscitem.Container
}

func (s *SmithBagSys) IsOpen() bool {
	return true
}

func (s *SmithBagSys) GetSysData() *[]*pb3.ItemSt {
	mainData := s.GetMainData()

	if nil == mainData.ItemPool {
		mainData.ItemPool = &pb3.ItemPool{}
	}

	var target *[]*pb3.ItemSt

	switch s.GetSysId() {
	case sysdef.SiSmithBag:
		target = &mainData.ItemPool.SmithBag
	default:
		return nil
	}

	if *target == nil {
		*target = make([]*pb3.ItemSt, 0)
	}

	return target
}

func (s *SmithBagSys) OnInit() {
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

func (s *SmithBagSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SmithBagSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SmithBagSys) s2cInfo() {
	s.SendProto3(80, 0, &pb3.S2C_80_0{
		SysId: s.GetSysId(),
		Items: *s.GetSysData(),
	})
}

func (s *SmithBagSys) DefaultSize() uint32 {
	ref, ok := gshare.SmithInstance.FindSmithRefByBagSysId(s.GetSysId())
	if !ok {
		return 0
	}
	bagSize := int(ref.BagType)
	return jsondata.GetBagDefaultSize(bagSize)
}

func (s *SmithBagSys) EnlargeSize() uint32 {
	return 0
}

func (s *SmithBagSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.initEquip(item)

	s.SendProto3(80, 1, &pb3.S2C_80_1{
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

func (s *SmithBagSys) initEquip(item *pb3.ItemSt) {
	if nil != item.Ext && item.Ext.IsInitExt {
		return
	}

	ref, ok := gshare.SmithInstance.FindSmithRefByBagSysId(s.GetSysId())
	if !ok {
		s.owner.LogError("道具初始化失败：%d", item.GetHandle())
		return
	}

	smithLv := s.owner.GetExtraAttrU32(attrdef.SmithLv)

	item.Union1 = jsondata.GetSmithSysConf(ref.SmithSysId).GetSmithRandEquipLv(smithLv)

	item.Attrs = jsondata.GetSmithEquipAttrConf(s.GetSysId()).GetSmithEntryAttrs(item.ItemId)

	if nil == item.Ext {
		item.Ext = &pb3.ItemExt{}
	}
	item.Ext.IsInitExt = true

	return
}

func (s *SmithBagSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendProto3(80, 1, &pb3.S2C_80_1{
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

func (s *SmithBagSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendProto3(80, 2, &pb3.S2C_80_2{
		SysId:   s.GetSysId(),
		Handles: []uint64{item.GetHandle()},
	})

	has := item.GetCount()
	s.changeStat(item, 0-has, true, logId)
}

func (s *SmithBagSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   s.GetItemCount(item.GetItemId(), -1),
	})
}

func regSmithBagSys() {
	gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
		RegisterSysClass(ref.BagSysId, func() iface.ISystem {
			return &SmithBagSys{}
		})
	})
}

func init() {
	regSmithBagSys()
}
