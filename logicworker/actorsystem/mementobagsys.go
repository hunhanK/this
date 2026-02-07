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
	"time"
)

var _ iface.IBagSys = (*MementoBagSystem)(nil)

// 古宝背包

type MementoBagSystem struct {
	Base
	*miscitem.Container
}

func (sys *MementoBagSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}

	if nil == itemPool.MementoBag {
		itemPool.MementoBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.MementoBag)
	container.DefaultSizeHandle = sys.DefaultSize
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	container.OnUseOnGetItem = sys.OnUseOnGet
	container.OnDeleteItemPtr = sys.owner.OnDeleteItemPtr
	sys.Container = container
}

func (sys *MementoBagSystem) OnOpen() {
	sys.S2CInfo()
}

func (sys *MementoBagSystem) OnLogin() {
	sys.S2CInfo()
}

func (sys *MementoBagSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *MementoBagSystem) S2CInfo() {
	sys.SendProto3(130, 30, &pb3.S2C_130_30{
		Items: sys.GetMainData().ItemPool.MementoBag,
	})
}

func (sys *MementoBagSystem) OnUseOnGet(hdl uint64, count int64) {
	sys.UseItem(hdl, count, nil)
}

func (sys *MementoBagSystem) checkCd(itemConf *jsondata.ItemConf, count int64) bool {
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

func (sys *MementoBagSystem) checkItemUseConsume(itemId uint32, count int64) bool {
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

func (sys *MementoBagSystem) UseItem(handle uint64, count int64, params []uint32) {
	item := sys.FindItemByHandle(handle)
	if nil == item || item.GetCount() < count {
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

func (sys *MementoBagSystem) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagMemento)
}

func (sys *MementoBagSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.SendProto3(130, 31, &pb3.S2C_130_31{
		Items: []*pb3.ItemSt{item},
	})

	sys.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (sys *MementoBagSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(130, 32, &pb3.S2C_130_32{
		Items: []*pb3.ItemSt{item},
	})
	sys.changeStat(item, item.GetCount(), false, param.LogId)
	if add > 0 {
		if !param.NoTips {
			sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), add)
		}

		if checkLogId(param.LogId) {
			sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), add)
		}
	}
}

func (sys *MementoBagSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(130, 33, &pb3.S2C_130_33{
		Handles: []uint64{item.Handle},
	})
	has := item.GetCount()
	sys.changeStat(item, 0-has, true, logId)
}

func (sys *MementoBagSystem) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   sys.GetItemCount(item.GetItemId(), -1),
	})
}

func (sys *MementoBagSystem) c2sInfo(_ *base.Message) error {
	sys.S2CInfo()
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiMementoBag, func() iface.ISystem {
		return &MementoBagSystem{}
	})
	net.RegisterSysProtoV2(130, 30, sysdef.SiMementoBag, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MementoBagSystem).c2sInfo
	})
}
