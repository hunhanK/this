package actorsystem

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/emirpasic/gods/trees/binaryheap"
	butils "github.com/emirpasic/gods/utils"

	"github.com/gzjjyz/srvlib/utils"
)

/*
	desc:背包系统
	author: ChenJunJi
	maintainer: YangQiBin
*/

type BagSystem struct {
	Base
	*miscitem.Container
	timeOutMgr *ItemStTimeOutMgr
}

const (
	ActPYYType = 1 // 个人运营活动
	ActYYType  = 2 // 全服运营活动
)

func (sys *BagSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.Bags {
		itemPool.Bags = make([]*pb3.ItemSt, 0)
	}

	if nil == sys.GetBinaryData().ItemUseTime {
		sys.GetBinaryData().ItemUseTime = make(map[uint32]uint32)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.Bags)
	container.DefaultSizeHandle = sys.DefaultSize
	container.EnlargeSizeHandle = sys.EnlargeSize
	container.GetMoneyHandle = sys.owner.GetMoneyCount
	container.AddMoneyHandle = sys.owner.AddMoney
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	container.OnUseOnGetItem = sys.OnUseOnGet
	container.OnDeleteItemPtr = sys.owner.OnDeleteItemPtr

	sys.Container = container
	sys.timeOutMgr.initItems(sys.GetOwner(), container)
}

func (sys *BagSystem) IsOpen() bool {
	return true
}

func (sys *BagSystem) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *BagSystem) OnOpen() {
	sys.S2CInfo()
}

func (sys *BagSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *BagSystem) Clear() {
	sys.Container.Clear()
	sys.S2CInfo()
}

func (sys *BagSystem) c2sInfo(msg *base.Message) error {
	sys.S2CInfo()
	return nil
}

func (sys *BagSystem) S2CInfo() {
	sys.SendProto3(4, 0, &pb3.S2C_4_0{
		Items:       sys.GetMainData().ItemPool.Bags,
		EnlargeSize: sys.EnlargeSize(),
		ItemUseTime: sys.GetBinaryData().ItemUseTime,
	})
}

func (sys *BagSystem) DefaultSize() uint32 {
	defaultCapEnlarge, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumBagCapEnlarge)
	return jsondata.GetBagDefaultSize(bagdef.BagType) + uint32(defaultCapEnlarge)
}

func (sys *BagSystem) SortBag(msg *base.Message) {
	success, updateVec, deleteVec := sys.Sort()
	defer sys.SendProto3(4, 4, &pb3.S2C_4_4{Success: success})
	if !success {
		return
	}
	if len(updateVec) > 0 {
		sys.SendProto3(4, 1, &pb3.S2C_4_1{Items: updateVec})
	}

	if len(deleteVec) > 0 {
		rsp := &pb3.S2C_4_2{}
		rsp.Handles = append(rsp.Handles, deleteVec...)

		sys.SendProto3(4, 2, rsp)
	}

}

func (sys *BagSystem) checkCd(itemConf *jsondata.ItemConf, count int64) bool {
	if itemConf.CdGroup == 0 {
		return true
	}

	cdConf := jsondata.GetItemCdGroupConf(itemConf.CdGroup)

	if cdConf == nil {
		sys.LogError("cd group %d is nil", itemConf.CdGroup)
		return false
	}

	if count > 1 {
		sys.LogError("带cd 的道具%d 不可以一次使用多个", itemConf.Id)
		return false
	}

	now := uint32(time.Now().Unix())
	lastUseTime := sys.GetBinaryData().ItemUseTime[itemConf.CdGroup]
	if lastUseTime+cdConf.Cd > now {
		sys.LogError("道具 %d 使用cd未到", itemConf.Id)
		return false
	}
	return true
}

func (sys *BagSystem) UseItem(handle uint64, count int64, params []uint32) {
	item := sys.FindItemByHandle(handle)
	if count <= 0 || nil == item || item.GetCount() < count {
		return
	}
	itemId := item.GetItemId()
	conf := jsondata.GetItemConfig(itemId)
	if conf == nil {
		return
	}

	if !sys.checkCd(conf, count) {
		sys.owner.SendTipMsg(tipmsgid.TpUseItemFailed)
		return
	}

	if !sys.owner.CheckItemCond(conf) {
		sys.owner.SendTipMsg(tipmsgid.TpUseItemFailed)
		return
	}

	if CheckTimeOut(item) {
		sys.owner.SendTipMsg(tipmsgid.TpUseItemFailed)
		return
	}

	rsp := &pb3.S2C_4_3{
		Handle: item.Handle,
		ItemId: itemId,
		Count:  count,
		Ret:    false,
	}

	if fn := miscitem.GetItemUseHandle(itemId); nil != fn {
		if !sys.checkItemUseConsume(itemId, count) {
			return
		}

		st := &miscitem.UseItemParamSt{Handle: item.GetHandle(), ItemId: itemId, Count: count, Params: params}
		if success, del, realUseCnt := fn(sys.owner, st); success {
			if del {
				if sys.DeleteItemPtr(item, realUseCnt, pb3.LogId_LogUseItem) {
					sys.owner.TriggerQuestEvent(custom_id.QttConsumeItemNum, itemId, count)
				}
			}
			rsp.Ret = true

			if conf.CdGroup != 0 {
				sys.GetBinaryData().ItemUseTime[conf.CdGroup] = uint32(time.Now().Unix())
			}
		} else {
			sys.LogWarn("%s使用道具失败:%s,%d,%d %d", sys.owner.GetName(), conf.Name, itemId, item.GetHandle(), count)
		}
	} else {
		sys.LogWarn("未注册的道具使用handle. item:%d %s", itemId, conf.Name)
	}

	sys.SendProto3(4, 3, rsp)
}

func (sys *BagSystem) checkItemUseConsume(itemId uint32, count int64) bool {
	conf := jsondata.GetItemUseConsumeConf(itemId)
	if nil == conf {
		return true
	}

	if !sys.owner.CheckConsumeByConf(conf.Consume, false, 0) {
		sys.GetOwner().SendTipMsg(tipmsgid.TpItemNotEnough)
		return false
	}

	return true
}

// 根据道具表id获取所有道具Id
func (sys *BagSystem) GetAllItemHandleByItemId(itemId uint32) (ret []uint64) {
	mainData := sys.GetMainData()

	if nil == mainData.ItemPool {
		return nil
	}

	ret = make([]uint64, 0)
	for _, item := range mainData.ItemPool.Bags {
		if item.GetItemId() == itemId {
			ret = append(ret, item.GetHandle())
		}
	}
	return
}

// 根据道具表id和品质 获取所有道具Id
func (sys *BagSystem) GetAllItemHandleByItemIdQuality(itemId, quality uint32) (ret []uint64, num int64) {
	mainData := sys.GetMainData()

	if nil == mainData.ItemPool {
		return
	}

	ret = make([]uint64, 0)
	for _, item := range mainData.ItemPool.Bags {
		if item.GetItemId() == itemId && item.GetUnion2() <= quality {
			ret = append(ret, item.GetHandle())
			num += item.GetCount()
		}
	}
	return
}

// 根据物品类型获取item对象
func (sys *BagSystem) GetItemByItemType(pType uint32, sType uint32) []*pb3.ItemSt {
	mainData := sys.GetMainData()
	var ret []*pb3.ItemSt
	for _, item := range mainData.ItemPool.Bags {
		itemCfg := jsondata.GetItemConfig(item.GetItemId())
		if sType == itemCfg.SubType && itemCfg.Type == pType {
			ret = append(ret, item)
		}
	}
	return ret
}

func (sys *BagSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.SetTimeOut(item)
	sys.owner.TriggerEvent(custom_id.AeAddNewBagItem, item)

	sys.SendProto3(4, 1, &pb3.S2C_4_1{Items: []*pb3.ItemSt{item}, LogId: uint32(logId), LogType: common.LogType[pb3.LogId(logId)]})
	sys.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	// 特权25可自动使用道具
	if base.CheckItemFlag(item.ItemId, itemdef.PrivilegeAutoUse) &&
		sys.owner.HasPrivilege(privilegedef.EnumItemAutoUse) {
		sys.OnUseOnGetItem(item.GetHandle(), item.GetCount())
		bTip = false
	}

	if bTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}

	sys.timeOutMgr.Add(sys.GetOwner(), item)
}

func (sys *BagSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(4, 1, &pb3.S2C_4_1{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
	sys.changeStat(item, add, false, param.LogId)
	if add > 0 {
		if !param.NoTips {
			sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), add)
		}

		if checkLogId(param.LogId) {
			sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), add)
		}
	}
}

func (sys *BagSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(4, 2, &pb3.S2C_4_2{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	sys.changeStat(item, 0-has, true, logId)
}

func (sys *BagSystem) OnUseOnGet(hdl uint64, count int64) {
	sys.UseItem(hdl, count, nil)
}

func (sys *BagSystem) SetTimeOut(item *pb3.ItemSt) {
	if nil == item || item.GetTimeOut() > 0 {
		return
	}

	itemId := item.GetItemId()
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return
	}
	nowSec := time_util.NowSec()
	if len(conf.ActArgs) >= 2 {
		actType := conf.ActArgs[0]
		item.TimeOut = nowSec
		for i := 1; i < len(conf.ActArgs); i++ {
			actId := conf.ActArgs[i]
			if actType == ActPYYType {
				obj := sys.owner.GetPYYObj(actId)
				if obj != nil && obj.IsOpen() {
					item.TimeOut = obj.GetEndTime()
					break
				}
			} else if actType == ActYYType {
				obj := sys.owner.GetYYObj(actId)
				if obj != nil && obj.IsOpen() {
					item.TimeOut = obj.GetEndTime()
					break
				}
			}
		}
		return
	}

	if conf.Timeout > 0 {
		item.TimeOut = time_util.NowSec() + conf.Timeout
	}
}

func CheckTimeOut(item *pb3.ItemSt) (ret bool) {
	if nil == item || item.GetTimeOut() == 0 {
		return
	}

	nowSec := time_util.NowSec()

	if nowSec < item.GetTimeOut() {
		return
	}

	return true
}

func (sys *BagSystem) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   sys.GetItemCount(item.GetItemId(), -1),
	})
}

// 使用道具
func (sys *BagSystem) c2sUseItem(msg *base.Message) {
	var req pb3.C2S_4_3
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.UseItem(req.Handle, req.Count, req.Params)
}

// 使用道具
func (sys *BagSystem) c2sBatchUseItem(msg *base.Message) {
	var req pb3.C2S_4_26
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	for _, st := range req.UseList {
		utils.ProtectRun(func() {
			sys.UseItem(st.Handle, st.Count, st.Params)
		})
	}
}

// 合并道具
func (sys *BagSystem) c2sMergeItem(msg *base.Message) {
	var req pb3.C2S_4_5
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.MergeItem(req.FromHandle, req.ToHandle, pb3.LogId_LogMergeItem)
}

func (sys *BagSystem) c2sBag2PrivilegeDepot(msg *base.Message) error {
	depotSys, ok := sys.owner.GetSysObj(sysdef.SiPrivilegeDepot).(*PrivilegeDepotSystem)
	if !ok || !depotSys.IsOpen() {
		return neterror.SysNotExistError("sys %d is not open", sysdef.SiPrivilegeDepot)
	}
	if !depotSys.CanUsePrivilegeDepot() {
		return neterror.SysNotExistError("no privilege")
	}

	var req pb3.C2S_4_58
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return err
	}
	hdl := req.GetHandle()

	item := sys.FindItemByHandle(hdl)
	if nil == item {
		return neterror.ParamsInvalidError("c2sBag2Depot find item by handler %d failed", hdl)
	}

	if depotSys.AvailableCount() <= 0 {
		return neterror.ParamsInvalidError("depot is full")
	}

	if base.CheckItemFlag(item.GetItemId(), itemdef.DenyStorage) {
		return neterror.ParamsInvalidError("c2sBag2Depot failed item %d can`t be put into DepotSys", item.ItemId)
	}

	sys.RemoveItemByHandle(hdl, pb3.LogId_LogBagToPrivilegeDepot)
	depotSys.AddItemPtr(item, false, pb3.LogId_LogBagToPrivilegeDepot)

	return nil
}

func (sys *BagSystem) c2sBag2Depot(msg *base.Message) {
	depotSys, ok := sys.owner.GetSysObj(sysdef.SiDepot).(*DepotSystem)
	if !ok {
		return
	}
	if !depotSys.CanUseDepot() {
		return
	}

	var req pb3.C2S_4_7
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	hdl := req.GetHandle()

	item := sys.FindItemByHandle(hdl)
	if nil == item {
		sys.LogError("c2sBag2Depot find item by handler %d failed", hdl)
		return
	}

	if depotSys.AvailableCount() <= 0 {
		return
	}

	if base.CheckItemFlag(item.GetItemId(), itemdef.DenyStorage) {
		sys.LogError("c2sBag2Depot failed item %d can`t be put into DepotSys", item.ItemId)
		return
	}

	sys.RemoveItemByHandle(hdl, pb3.LogId_LogBagToDepot)
	depotSys.AddItemPtr(item, false, pb3.LogId_LogBagToDepot)
}

func c2sRecycle(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_4_19
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	err = recycleAndDecompose(player, req.Handles, pb3.LogId_LogRecycle)
	if err != nil {
		return neterror.Wrap(err)
	}

	player.SendProto3(4, 19, &pb3.S2C_4_19{Ret: true})
	player.TriggerQuestEvent(custom_id.QttItemRecycle, 0, 1)
	return nil
}

func c2sDecompose(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_4_20
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	err = recycleAndDecompose(player, req.Handles, pb3.LogId_LogDecompose)
	if err != nil {
		return neterror.Wrap(err)
	}

	player.SendProto3(4, 20, &pb3.S2C_4_20{Ret: true})
	player.TriggerQuestEvent(custom_id.QttItemDecompose, 0, 1)
	return nil
}

func (sys *BagSystem) OnEnlargeSize(size uint32) {
	binaryData := sys.GetBinaryData()
	binaryData.BagEnlargeSize += size
	sys.SendProto3(4, 8, &pb3.S2C_4_8{EnlargeSize: sys.EnlargeSize()})
}

func (sys *BagSystem) EnlargeSize() uint32 {
	binaryData := sys.GetBinaryData()

	return binaryData.BagEnlargeSize
}

func GmMakeItem(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length <= 0 {
		return false
	}

	ids := jsondata.GetItemIdByName(args[0])
	if ids == nil {
		return false
	}
	var count int64 = 1
	bind := false

	if length >= 3 {
		bind = utils.AtoUint32(args[2]) == 1
		count = utils.AtoInt64(args[1])
	} else if length >= 2 {
		count = utils.AtoInt64(args[1])
	}

	for _, id := range ids {
		st := &itemdef.ItemParamSt{
			ItemId: id,
			Count:  count,
			Bind:   bind,
			LogId:  pb3.LogId_LogGm,
		}
		if actor.CanAddItem(st, true) {
			actor.AddItem(st)
		}
	}
	return true
}

func GmAddItem(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length <= 0 {
		return false
	}

	id := utils.AtoUint32(args[0])
	if id <= 0 {
		return false
	}
	var count int64 = 1
	bind := false

	if length >= 3 {
		bind = utils.AtoUint32(args[2]) == 1
		count = utils.AtoInt64(args[1])
	} else if length >= 2 {
		count = utils.AtoInt64(args[1])
	}

	rewards := jsondata.StdRewardVec{}

	rewards = append(rewards, &jsondata.StdReward{
		Id:    id,
		Count: count,
		Bind:  bind,
	})

	state := engine.GiveRewards(actor, rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGm,
	})

	return state
}

func GmAddItemEx(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length <= 2 {
		return false
	}

	id := utils.AtoUint32(args[0])
	idEnd := utils.AtoUint32(args[1])
	count := utils.AtoUint32(args[2])
	if id <= 0 || idEnd <= 0 {
		return false
	}

	for i := id; i <= idEnd; i++ {
		st := &itemdef.ItemParamSt{
			ItemId: i,
			Count:  int64(count),
			Bind:   false,
			LogId:  pb3.LogId_LogGm,
		}
		if actor.CanAddItem(st, true) {
			actor.AddItem(st)
		}
	}

	return true
}

func checkLogId(logId pb3.LogId) bool {
	switch logId {
	case pb3.LogId_LogSplitItem, pb3.LogId_LogMergeItem, pb3.LogId_LogDepotToBag:
		return false
	}

	return true
}

func GmClearBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiBag).(*BagSystem); ok {
		sys.Clear()
		return true
	}
	return false
}
func GmClearDepot(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiDepot).(*DepotSystem); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}
func GmClearFairy(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem); ok {
		sys.Clear()
		sys.S2CInfo()
		return true
	}
	return false
}
func GmClearGodBeast(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiGodBeastBag).(*GodBeastBagSystem); ok {
		sys.Clear()
		sys.S2CInfo()
		return true
	}
	return false
}
func GmClearFairyEquip(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFairyEquip).(*FairyEquipSys); ok {
		sys.Clear()
		sys.S2CInfo()
		return true
	}
	return false
}
func GmClearGodEquip(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}
func GmClearMemento(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiMementoBag).(*MementoBagSystem); ok {
		sys.Clear()
		sys.S2CInfo()
		return true
	}
	return false
}
func GmClearFairySword(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFairySwordBag).(*FairySwordBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearFairySpirit(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFairySpiritEquBag).(*FairySpiritEquBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearHolyBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiHolyEquipBag).(*HolyEquipBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}
func GmClearFlyingSwordBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFlyingSwordEquipBag).(*FlyingSwordEquipBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearSourceSoulBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiSourceSoulBag).(*SourceSoulBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearBloodBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiBloodBag).(*BloodBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearBloodEquBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiBloodEquBag).(*BloodEquBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearSmithBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiSmithBag).(*SmithBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearFeatherEquBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearDomainEyeBag(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiDomainEyeBag).(*DomainEyeBagSys); ok {
		sys.Clear()
		sys.s2cInfo()
		return true
	}
	return false
}

func GmClearBagByType(actor iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}

	bagType := utils.AtoUint32(args[0])
	switch bagType {
	case bagdef.BagTempType:
	case bagdef.BagDepotType:
		return GmClearDepot(actor, args...)
	case bagdef.BagFairyType:
		return GmClearFairy(actor, args...)
	case bagdef.BagGodGodBeastType:
		return GmClearGodBeast(actor, args...)
	case bagdef.BagFairyEquipType:
		return GmClearFairyEquip(actor, args...)
	case bagdef.BagBattleSoulGodEquip:
		return GmClearGodEquip(actor, args...)
	case bagdef.BagMemento:
		return GmClearMemento(actor, args...)
	case bagdef.BagFairySword:
		return GmClearFairySword(actor, args...)
	case bagdef.BagFairySpirit:
		return GmClearFairySpirit(actor, args...)
	case bagdef.BagHolyEquip:
		return GmClearHolyBag(actor, args...)
	case bagdef.BagFlyingSword:
		return GmClearFlyingSwordBag(actor, args...)
	case bagdef.BagSourceSoul:
		return GmClearSourceSoulBag(actor, args...)
	case bagdef.BagBlood:
		return GmClearBloodBag(actor, args...)
	case bagdef.BagBloodEqu:
		return GmClearBloodEquBag(actor, args...)
	case bagdef.BagSmith:
		return GmClearSmithBag(actor, args...)
	case bagdef.BagFeatherEqu:
		return GmClearFeatherEquBag(actor, args...)
	case bagdef.BagDomainEye:
		return GmClearDomainEyeBag(actor, args...)
	default:
		return GmClearBag(actor, args...)
	}
	return false
}

func GmUseItem(actor iface.IPlayer, args ...string) bool {
	if sys, ok := actor.GetSysObj(sysdef.SiBag).(*BagSystem); ok {
		itemIds := sys.GetAllItemHandleByItemId(utils.AtoUint32(args[0]))
		for i := range itemIds {
			sys.UseItem(itemIds[i], 1, []uint32{})
		}
		return true
	}
	return false
}

func addBagSize(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	addSizePerStep := conf.Param[0]

	bagSys, ok := player.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, false, 0
	}

	bagConf := jsondata.GetBagConf(bagdef.BagType)
	if nil == bagConf {
		player.LogError("useItem:%d, GetBagConf failed", param.ItemId)
		return false, false, 0
	}

	maxBagCells := bagConf.MaxCells

	defaultCapEnlarge, _ := player.GetPrivilege(privilegedef.EnumBagCapEnlarge)
	maxBagCells += uint32(defaultCapEnlarge)

	sumAddSize := uint32(0)
	for i := 0; i < int(param.Count); i++ {
		sumAddSize += addSizePerStep
	}

	if sumAddSize+bagSys.EnlargeSize()+bagSys.DefaultSize() > maxBagCells {
		player.LogError("useItem:%d, max bag capacity limit", param.ItemId)
		return
	}

	if sumAddSize > 0 {
		bagSys.OnEnlargeSize(uint32(sumAddSize))
	}

	return true, true, param.Count
}

func useItemDefault(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	_, ok := player.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, false, 0
	}
	return true, true, param.Count
}

var _ iface.IBagSys = (*BagSystem)(nil)

func init() {
	RegisterSysClass(sysdef.SiBag, func() iface.ISystem {
		return &BagSystem{
			timeOutMgr: NewItemStTimeOutMgr(),
		}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddBagSize, addBagSize)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemDefault, useItemDefault)

	net.RegisterSysProto(4, 0, sysdef.SiBag, (*BagSystem).c2sInfo)
	net.RegisterSysProto(4, 3, sysdef.SiBag, (*BagSystem).c2sUseItem)
	net.RegisterSysProto(4, 26, sysdef.SiBag, (*BagSystem).c2sBatchUseItem)
	net.RegisterSysProto(4, 4, sysdef.SiBag, (*BagSystem).SortBag)
	net.RegisterSysProto(4, 5, sysdef.SiBag, (*BagSystem).c2sMergeItem)

	net.RegisterSysProto(4, 7, sysdef.SiBag, (*BagSystem).c2sBag2Depot)
	net.RegisterSysProtoV2(4, 58, sysdef.SiBag, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BagSystem).c2sBag2PrivilegeDepot
	})

	net.RegisterProto(4, 19, c2sRecycle)
	net.RegisterProto(4, 20, c2sDecompose)
	gmevent.Register("make", GmMakeItem, 1)
	gmevent.Register("additem", GmAddItem, 1)
	gmevent.Register("additem2", GmAddItemEx, 1)
	gmevent.Register("clearbag", GmClearBag, 1)
	gmevent.Register("useItem", GmUseItem, 1)
	gmevent.Register("clearbagtype", GmClearBagByType, 1)
}

type ItemStTimeOutMgr struct {
	timeOutHeap       *binaryheap.Heap
	fastTimeOutItemSt *pb3.ItemSt
	timer             *time_util.Timer
}

func NewItemStTimeOutMgr() *ItemStTimeOutMgr {
	timeOutHeap := binaryheap.NewWith(func(a, b interface{}) int {
		aSt := a.(*pb3.ItemSt)
		bSt := b.(*pb3.ItemSt)
		return butils.IntComparator(int(aSt.GetTimeOut()), int(bSt.GetTimeOut()))
	})
	return &ItemStTimeOutMgr{timeOutHeap: timeOutHeap}
}

func (to *ItemStTimeOutMgr) handleTimeOut(player iface.IPlayer) {
	if to.timeOutHeap.Size() == 0 {
		return
	}

	if to.fastTimeOutItemSt == nil {
		return
	}

	// 过期检查
	if !CheckTimeOut(to.fastTimeOutItemSt) {
		return
	}

	// 出栈
	value, _ := to.timeOutHeap.Pop()
	st := value.(*pb3.ItemSt)

	// 已无道具
	itemSt := player.GetItemByHandle(st.Handle)
	if itemSt == nil {
		// 更换期限道具
		to.changeFastTimeOutItemSt(player)
		return
	}

	// 移除道具
	player.DeleteItemPtr(itemSt, itemSt.GetCount(), pb3.LogId_LogItemStTimeOutDel)

	// 更换期限道具
	to.changeFastTimeOutItemSt(player)

	// 发送过期补偿奖励
	itemConf := jsondata.GetItemConfig(itemSt.ItemId)

	rewardConf := jsondata.GetRecycleConf(itemSt.ItemId)
	if rewardConf == nil {
		logger.LogWarn("不存在的回收配置:%d", itemSt.ItemId)
		return
	}

	rewards := jsondata.StdRewardMulti(rewardConf.Rewards, itemSt.Count)
	mailArgs := &mailargs.SendMailSt{
		Rewards: rewards,
	}
	if len(itemConf.ActArgs) >= 2 {
		actType := itemConf.ActArgs[0]
		actId := itemConf.ActArgs[1]

		var actName string
		if actType == ActPYYType {
			if actConf := jsondata.GetPlayerYYConf(actId); actConf != nil {
				actName = actConf.Name
			}
		} else if actType == ActYYType {
			if actConf := jsondata.GetYunYingConf(actId); actConf != nil {
				actName = actConf.Name
			}
		}

		mailArgs.ConfId = common.Mail_ActItemStTimeOutGiveAward
		mailArgs.Content = &mailargs.CommonMailArgs{
			Str1: actName,
			Str2: itemConf.Name,
		}
	} else {
		mailArgs.ConfId = common.Mail_ItemStTimeOutGiveAward
		mailArgs.Content = &mailargs.CommonMailArgs{
			Str1: itemConf.Name,
		}
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(player.GetId(), mailArgs)
	}
}

func (to *ItemStTimeOutMgr) initItems(player iface.IPlayer, container *miscitem.Container) {
	if to.timer != nil {
		to.timer.Stop()
	}
	to.timeOutHeap.Clear()
	for _, itemSt := range container.GetAllItemMap() {
		if itemSt.GetTimeOut() == 0 {
			continue
		}
		to.Add(nil, itemSt)
	}
	to.changeFastTimeOutItemSt(player)
}

func (to *ItemStTimeOutMgr) Add(player iface.IPlayer, itemSt *pb3.ItemSt) {
	if itemSt == nil {
		return
	}
	if itemSt.GetTimeOut() == 0 {
		return
	}

	to.timeOutHeap.Push(itemSt)

	if to.fastTimeOutItemSt != nil && to.fastTimeOutItemSt.GetTimeOut() < itemSt.GetTimeOut() {
		return
	}

	if player != nil {
		to.changeFastTimeOutItemSt(player)
	}
}

func (to *ItemStTimeOutMgr) changeFastTimeOutItemSt(player iface.IPlayer) {
	to.fastTimeOutItemSt = nil
	if to.timeOutHeap.Size() == 0 {
		return
	}
	peek, ok := to.timeOutHeap.Peek()
	if !ok {
		return
	}

	if to.timer != nil {
		to.timer.Stop()
	}
	to.fastTimeOutItemSt = peek.(*pb3.ItemSt)

	var diff uint32
	if to.fastTimeOutItemSt.GetTimeOut() > time_util.NowSec() {
		diff = to.fastTimeOutItemSt.GetTimeOut() - time_util.NowSec()
	}

	to.timer = player.SetTimeout(time.Duration(diff)*time.Second, func() {
		to.handleTimeOut(player)
	})
}
