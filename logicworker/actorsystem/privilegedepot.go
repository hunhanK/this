/**
 * @Author: LvYuMeng
 * @Date: 2025/12/26
 * @Desc: 特权仓库
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type PrivilegeDepotSystem struct {
	Base
	*miscitem.Container
}

func (sys *PrivilegeDepotSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.PrivilegeDepot {
		itemPool.PrivilegeDepot = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.PrivilegeDepot)
	container.DefaultSizeHandle = sys.DefaultSize
	container.EnlargeSizeHandle = sys.EnlargeSize
	container.GetMoneyHandle = sys.owner.GetMoneyCount
	container.AddMoneyHandle = sys.owner.AddMoney
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	sys.Container = container
}

func (sys *PrivilegeDepotSystem) OnReconnect() {
	sys.s2cInfo()
}

func (sys *PrivilegeDepotSystem) OnOpen() {
	sys.s2cInfo()
}

func (sys *PrivilegeDepotSystem) OnLogin() {
	sys.s2cInfo()
}

func (sys *PrivilegeDepotSystem) CanUsePrivilegeDepot() bool {
	var hasPrivilege bool
	t, err := sys.GetOwner().GetPrivilege(privilegedef.EnumPrivilegeDepot)
	if err == nil && t > 0 {
		hasPrivilege = true
	}

	if hasPrivilege {
		return true
	}

	return false
}

func (sys *PrivilegeDepotSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(4, 61, &pb3.S2C_4_61{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	sys.changeStat(item, int32(has), true, logId)
}

func (sys *PrivilegeDepotSystem) s2cInfo() {
	sys.SendProto3(4, 57, &pb3.S2C_4_57{
		Items: sys.GetMainData().ItemPool.PrivilegeDepot,
	})
}

func (sys *PrivilegeDepotSystem) c2sInfo(msg *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *PrivilegeDepotSystem) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagPrivilegeDepot)
}

func (sys *PrivilegeDepotSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(4, 60, &pb3.S2C_4_60{Items: []*pb3.ItemSt{item}})
	sys.changeStat(item, int32(add), false, param.LogId)
}

func (sys *PrivilegeDepotSystem) EnlargeSize() uint32 {
	return 0
}

func (sys *PrivilegeDepotSystem) SortPrivilegeDepot(msg *base.Message) {
	if !sys.CanUsePrivilegeDepot() {
		return
	}

	success, updateVec, deleteVec := sys.Sort()
	if !success {
		sys.SendProto3(4, 62, &pb3.S2C_4_62{})
		return
	}

	if len(updateVec) > 0 {
		privilegeDepotUpdateRsp := &pb3.S2C_4_60{
			Items: updateVec,
		}
		sys.SendProto3(4, 60, privilegeDepotUpdateRsp)
	}

	if len(deleteVec) > 0 {
		privilegeDepotDelItemRsp := &pb3.S2C_4_61{
			Handles: deleteVec,
		}
		sys.SendProto3(4, 61, privilegeDepotDelItemRsp)
	}
	sys.SendProto3(4, 62, &pb3.S2C_4_62{})
}

func (sys *PrivilegeDepotSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.SendProto3(4, 60, &pb3.S2C_4_60{Items: []*pb3.ItemSt{item}})
	sys.changeStat(item, int32(item.Count), false, logId)
}

func (sys *PrivilegeDepotSystem) changeStat(item *pb3.ItemSt, add int32, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  int64(add),
		LogId:  uint32(logId),
	})
}

func (sys *PrivilegeDepotSystem) c2sMergeItem(msg *base.Message) {
	if !sys.CanUsePrivilegeDepot() {
		return
	}

	var req pb3.C2S_4_63
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.MergeItem(req.FromHandle, req.ToHandle, pb3.LogId_LogPrivilegeDepotMergeItem)
}

func (sys *PrivilegeDepotSystem) c2sSplitItem(msg *base.Message) {
	if !sys.CanUsePrivilegeDepot() {
		return
	}

	var req pb3.C2S_4_64
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.Split(req.GetHandle(), int64(req.GetCount()), pb3.LogId_LogPrivilegeDepotSplitItem)
}

func (sys *PrivilegeDepotSystem) c2sToBag(msg *base.Message) {
	if !sys.CanUsePrivilegeDepot() {
		return
	}

	var req pb3.C2S_4_59
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		sys.LogError("c2sToBag failed unmarshal proto failed err: %s", err)
		return
	}

	bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return
	}

	if bagSys.AvailableCount() == 0 {
		sys.LogError("c2sToBag failed bag AvailableCount is zero")
		return
	}

	hdl := req.GetHandle()

	item := sys.FindItemByHandle(hdl)
	if nil == item {
		sys.LogError("c2sToBag find item handler: %d failed", hdl)
		return
	}

	if !sys.RemoveItemByHandle(hdl, pb3.LogId_LogPrivilegeDepotToBag) {
		sys.LogError("c2sToBag failed remove item from privilegeDepot failed")
	}

	if !bagSys.AddItemPtr(item, false, pb3.LogId_LogPrivilegeDepotToBag) {
		sys.LogError("c2sToBag failed add item to bag failed")
	}
}

func init() {
	RegisterSysClass(sysdef.SiPrivilegeDepot, func() iface.ISystem {
		return &PrivilegeDepotSystem{}
	})

	net.RegisterSysProto(4, 57, sysdef.SiPrivilegeDepot, (*PrivilegeDepotSystem).c2sInfo)
	net.RegisterSysProto(4, 59, sysdef.SiPrivilegeDepot, (*PrivilegeDepotSystem).c2sToBag)
	net.RegisterSysProto(4, 62, sysdef.SiPrivilegeDepot, (*PrivilegeDepotSystem).SortPrivilegeDepot)
	net.RegisterSysProto(4, 63, sysdef.SiPrivilegeDepot, (*PrivilegeDepotSystem).c2sMergeItem)
	net.RegisterSysProto(4, 64, sysdef.SiPrivilegeDepot, (*PrivilegeDepotSystem).c2sSplitItem)
}
