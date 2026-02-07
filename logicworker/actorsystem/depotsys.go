package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

/*
	desc:仓库系统
	author: ChenJunJi
	maintainer: YangQiBin
*/

type DepotSystem struct {
	Base
	*miscitem.Container
}

func (sys *DepotSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.DepotItems {
		itemPool.DepotItems = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.DepotItems)
	container.DefaultSizeHandle = sys.DefaultSize
	container.EnlargeSizeHandle = sys.EnlargeSize
	container.GetMoneyHandle = sys.owner.GetMoneyCount
	container.AddMoneyHandle = sys.owner.AddMoney
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	sys.Container = container
}

func (sys *DepotSystem) OnReconnect() {
	sys.s2cInfo()
}

func (sys *DepotSystem) OnOpen() {
	sys.s2cInfo()
}

func (sys *DepotSystem) OnLogin() {
	sys.s2cInfo()
}

func (sys *DepotSystem) CanUseDepot() bool {
	var hasPrivilege bool
	t, err := sys.GetOwner().GetPrivilege(privilegedef.EnumPortableDepot)
	if err == nil && t > 0 {
		hasPrivilege = true
	}

	if hasPrivilege {
		return true
	}

	cc := jsondata.GlobalU32Vec("depotNpcId")
	if len(cc) != 2 {
		sys.LogWarn("没有配置随身仓库通用配置 depotNpcId, 已允许所有随身仓库请求")
		return true
	}

	depotNpcId, maxGrid := cc[0], cc[1]

	// 没有随身仓库特权，需要验证玩家距离仓库管理员的 npc 距离 <= 5 格
	sceneId := sys.GetOwner().GetSceneId()
	cfgs := jsondata.GetNpcLocationConf(sceneId)
	var npcX, npcY int32
	var has bool
	for _, cfg := range cfgs {
		if cfg.NpcId == depotNpcId {
			npcX, npcY = cfg.Posx, cfg.Posy
			has = true
		}
	}
	if !has {
		return false
	}

	pos := sys.GetOwner().GetBinaryData().GetPos()
	if pos == nil {
		return false
	}

	playerX, playerY := pos.PosX, pos.PosY

	return uint32(base.GetDistance(uint32(npcX), uint32(npcY), uint32(playerX), uint32(playerY))) <= maxGrid
}

func (sys *DepotSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(4, 14, &pb3.S2C_4_14{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	sys.changeStat(item, int32(has), true, logId)
}

func (sys *DepotSystem) s2cInfo() {
	binaryData := sys.GetBinaryData()

	sys.SendProto3(4, 11, &pb3.S2C_4_11{
		Items:       sys.GetMainData().ItemPool.DepotItems,
		EnlargeSize: binaryData.DepotEnlargeSize})
}

func (sys *DepotSystem) c2sInfo(msg *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *DepotSystem) DefaultSize() uint32 {
	extraCap, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumDepotCapEnlarge)
	return jsondata.GetBagDefaultSize(bagdef.BagDepotType) + uint32(extraCap)
}

func (sys *DepotSystem) GetEnlargeSize() uint32 {
	binaryData := sys.GetBinaryData()

	return binaryData.DepotEnlargeSize
}

func (sys *DepotSystem) OnEnlargeSize(size uint32) {
	binaryData := sys.GetBinaryData()

	binaryData.DepotEnlargeSize += size

	sys.SendProto3(4, 18, &pb3.S2C_4_18{CurSize: binaryData.DepotEnlargeSize})
}

func (sys *DepotSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(4, 13, &pb3.S2C_4_13{Items: []*pb3.ItemSt{item}})
	sys.changeStat(item, int32(add), false, param.LogId)
}

func (sys *DepotSystem) EnlargeSize() uint32 {
	binaryData := sys.GetBinaryData()

	return binaryData.DepotEnlargeSize
}

func (sys *DepotSystem) SortDepot(msg *base.Message) {
	if !sys.CanUseDepot() {
		return
	}

	success, updateVec, deleteVec := sys.Sort()
	if !success {
		sys.SendProto3(4, 15, &pb3.S2C_4_15{})
		return
	}

	if len(updateVec) > 0 {
		depotUpdateRsp := &pb3.S2C_4_13{
			Items: updateVec,
		}
		sys.SendProto3(4, 13, depotUpdateRsp)
	}

	if len(deleteVec) > 0 {
		depotDelItemRsp := &pb3.S2C_4_14{
			Handles: deleteVec,
		}
		sys.SendProto3(4, 14, depotDelItemRsp)
	}
	sys.SendProto3(4, 15, &pb3.S2C_4_15{})
}

func (sys *DepotSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.SendProto3(4, 13, &pb3.S2C_4_13{Items: []*pb3.ItemSt{item}})
	sys.changeStat(item, int32(item.Count), false, logId)
}

func (sys *DepotSystem) changeStat(item *pb3.ItemSt, add int32, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  int64(add),
		LogId:  uint32(logId),
	})
}

func (sys *DepotSystem) c2sMergeItem(msg *base.Message) {
	if !sys.CanUseDepot() {
		return
	}

	var req pb3.C2S_4_16
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.MergeItem(req.FromHandle, req.ToHandle, pb3.LogId_LogDepotMergeItem)
}

func (sys *DepotSystem) c2sSplitItem(msg *base.Message) {
	if !sys.CanUseDepot() {
		return
	}

	var req pb3.C2S_4_17
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.Split(req.GetHandle(), int64(req.GetCount()), pb3.LogId_LogDepotSplitItem)
}

func (sys *DepotSystem) c2sToBag(msg *base.Message) {
	if !sys.CanUseDepot() {
		return
	}

	var req pb3.C2S_4_12
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

	if !sys.RemoveItemByHandle(hdl, pb3.LogId_LogDepotToBag) {
		sys.LogError("c2sToBag failed remove item from depot failed")
	}

	if !bagSys.AddItemPtr(item, false, pb3.LogId_LogDepotToBag) {
		sys.LogError("c2sToBag failed add item to bag failed")
	}
}

func addDepotSize(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	addSizePerStep := conf.Param[0]

	depotSys, ok := player.GetSysObj(sysdef.SiDepot).(*DepotSystem)
	if !ok {
		return false, false, 0
	}

	depotConf := jsondata.GetBagConf(bagdef.BagDepotType)
	if nil == depotConf {
		player.LogError("useItem:%d, GetBagConf failed", param.ItemId)
		return false, false, 0
	}

	sumAddSize := 0
	for i := 0; i < int(param.Count); i++ {
		sumAddSize += int(addSizePerStep)
	}

	extraCap, _ := player.GetPrivilege(privilegedef.EnumDepotCapEnlarge)

	if sumAddSize+int(depotSys.EnlargeSize())+int(depotSys.DefaultSize()) > int(depotConf.MaxCells)+int(extraCap) {
		player.LogError("useItem:%d, max depot capacity limit", param.ItemId)
		return
	}

	if sumAddSize > 0 {
		depotSys.OnEnlargeSize(uint32(sumAddSize))

		player.SendTipMsg(conf.Tip, sumAddSize)

	}

	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiDepot, func() iface.ISystem {
		return &DepotSystem{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddDepotSize, addDepotSize)
	net.RegisterSysProto(4, 11, sysdef.SiDepot, (*DepotSystem).c2sInfo)
	net.RegisterSysProto(4, 12, sysdef.SiDepot, (*DepotSystem).c2sToBag)
	net.RegisterSysProto(4, 15, sysdef.SiDepot, (*DepotSystem).SortDepot)
	net.RegisterSysProto(4, 16, sysdef.SiDepot, (*DepotSystem).c2sMergeItem)
	net.RegisterSysProto(4, 17, sysdef.SiDepot, (*DepotSystem).c2sSplitItem)
	//net.RegisterSysProto(4, 18, gshare.SiDepot, (*DepotSystem).c2sEnlargeSize)
}
