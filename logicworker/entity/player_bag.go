/**
 * @Author: zjj
 * @Date: 2025/3/26
 * @Desc:
**/

package entity

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
)

func (player *Player) GetBagSysByBagType(bagType uint32) iface.IBagSys {
	sysId, ok := gshare.BagTypeToSysMap[bagType]
	if !ok {
		return nil
	}
	sys := player.GetSysObj(sysId)
	if sys == nil {
		return nil
	}
	objSys, ok := sys.(iface.IBagSys)
	if !ok {
		return nil
	}
	return objSys
}

func (player *Player) GetBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagType)
}

func (player *Player) GetFairyBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFairyType)
}

func (player *Player) GetGodBeastBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagGodGodBeastType)
}

func (player *Player) GetFairyEquipBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFairyEquipType)
}

func (player *Player) GetBattleSoulGodEquipBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagBattleSoulGodEquip)
}

func (player *Player) GetSmithBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagSmith)
}

func (player *Player) GetMementoBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagMemento)
}

func (player *Player) GetFairySwordBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFairySword)
}

func (player *Player) GetFairySpiritEquBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFairySpirit)
}

func (player *Player) GetHolyEquBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagHolyEquip)
}

func (player *Player) GetBloodBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagBlood)
}

func (player *Player) GetBloodEquBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagBloodEqu)
}

func (player *Player) GetFeatherEquBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFeatherEqu)
}

func (player *Player) GetFlyingSwordBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagFlyingSword)
}

func (player *Player) GetSourceSoulBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagSourceSoul)
}

func (player *Player) GetDomainEyeBagSys() iface.IBagSys {
	return player.GetBagSysByBagType(bagdef.BagDomainEye)
}

// GetItemByHandle 根据handle获取背包道具
func (player *Player) GetItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetBagSys(); nil != sys {
		if item := sys.FindItemByHandle(hdl); nil != item {
			return item
		}
	}
	return nil
}

func (player *Player) RemoveItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

// GetFairyByHandle 根据handle获取仙灵道具
func (player *Player) GetFairyByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetFairyBagSys(); nil != sys {
		if item := sys.FindItemByHandle(hdl); nil != item {
			return item
		}
	}
	return nil
}

// RemoveFairyByHandle 要确认不能移除出战中的宠物
func (player *Player) RemoveFairyByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetFairyBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

// GetGodBeastItemByHandle 根据handle获取神兽装备
func (player *Player) GetGodBeastItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetGodBeastBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveGodBeastItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetGodBeastBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetFairyEquipItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetFairyEquipBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveFairyEquipItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetFairyEquipBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetFairySwordItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetFairySwordBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveFairySwordItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetFairySwordBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetFairySpiritItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetFairySpiritEquBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveFairySpiritItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetFairySpiritEquBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetHolyEquipItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetHolyEquBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveHolyItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetHolyEquBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetFlyingSwordEquipItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetFlyingSwordBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveFlyingSwordEquipItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetFlyingSwordBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetSourceSoulEquipItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetSourceSoulBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveSourceSoulEquipItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetSourceSoulBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetBloodItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetBloodBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveBloodItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetBloodBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) RemoveBloodEquItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetBloodEquBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetDomainEyeRuneItemByHandle(hdl uint64) *pb3.ItemSt {
	if sys := player.GetDomainEyeBagSys(); nil != sys {
		return sys.FindItemByHandle(hdl)
	}
	return nil
}

func (player *Player) RemoveDomainEyeRuneItemByHandle(hdl uint64, logId pb3.LogId) bool {
	if sys := player.GetDomainEyeBagSys(); nil != sys {
		return sys.RemoveItemByHandle(hdl, logId)
	}
	return false
}

func (player *Player) GetItemByHandleWithBagType(bagType uint32, hdl uint64) *pb3.ItemSt {
	sys := player.GetBagSysByBagType(bagType)
	if nil == sys {
		return nil
	}
	return sys.FindItemByHandle(hdl)
}

func (player *Player) RemoveItemByHandleWithBagType(bagType uint32, hdl uint64, logId pb3.LogId) bool {
	sys := player.GetBagSysByBagType(bagType)
	if nil == sys {
		return false
	}
	return sys.RemoveItemByHandle(hdl, logId)
}

func (player *Player) GetBagByItemType(itemId uint32) iface.IBagSys {
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return nil
	}

	for _, v := range gshare.SpBagRules {
		if v.Check(itemConf.Type) {
			return player.GetBagSysByBagType(v.Bag)
		}
	}
	return player.GetBagSys()
}

func (player *Player) GetBagType(itemId uint32) uint32 {
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return bagdef.BagType
	}

	for _, v := range gshare.SpBagRules {
		if v.Check(itemConf.Type) {
			return v.Bag
		}
	}

	return bagdef.BagType
}

func (player *Player) SendBagFullTip(itemId uint32) {
	bagType := player.GetBagType(itemId)
	conf := jsondata.GetBagConf(int(bagType))
	if conf != nil {
		player.SendProto3(17, 250, &pb3.S2C_17_250{
			Result:  false,
			BagType: conf.Id,
		})
	}
	return
}

func (player *Player) AddItemPtr(item *pb3.ItemSt, bTip bool, logId pb3.LogId) bool {
	sysObj := player.GetBagByItemType(item.ItemId)
	if sysObj != nil {
		return sysObj.AddItemPtr(item, bTip, logId)
	}
	return false
}

func (player *Player) DeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId) bool {
	sysObj := player.GetBagByItemType(item.ItemId)
	if sysObj != nil {
		return sysObj.DeleteItemPtr(item, count, logId)
	}
	return false
}

func (player *Player) OnDeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId) {
	player.TriggerEvent(custom_id.AeConsumeItem, item.GetItemId(), count, logId)
}

func (player *Player) DeleteItemById(itemId uint32, count int64, logId pb3.LogId) bool {
	sysObj := player.GetBagByItemType(itemId)
	if sysObj != nil {
		return sysObj.DeleteItem(&itemdef.ItemParamSt{ItemId: itemId, Count: count, LogId: logId})
	}

	return false
}

func (player *Player) GetItemCount(itemId uint32, bind int8) int64 {
	sysObj := player.GetBagByItemType(itemId)
	if sysObj != nil {
		return sysObj.GetItemCount(itemId, bind)
	}
	return 0
}

func (player *Player) CheckItemBind(itemId uint32, isBind bool) bool {
	if sys := player.GetBagSys(); nil != sys {
		return sys.CheckItemBind(itemId, isBind)
	}
	return false
}

func (player *Player) GetBelongBagSysByHandle(hdl uint64) iface.ISystem {
	for _, sysId := range gshare.BagTypeToSysMap {
		objSys := player.GetSysObj(sysId)
		if nil == objSys {
			continue
		}
		bagSys, ok := objSys.(iface.IBagSys)
		if !ok {
			continue
		}
		if nil == bagSys.FindItemByHandle(hdl) {
			continue
		}
		return objSys
	}

	return nil
}
