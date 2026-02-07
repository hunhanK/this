package actorsystem

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

var _ iface.IBagSys = (*GodBeastBagSystem)(nil)

type GodBeastBagSystem struct {
	Base
	*miscitem.Container
}

func (sys *GodBeastBagSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}

	if nil == itemPool.GodBeastBag {
		itemPool.GodBeastBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.GodBeastBag)
	container.DefaultSizeHandle = sys.DefaultSize
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	container.OnDeleteItemPtr = sys.owner.OnDeleteItemPtr
	sys.Container = container
}

func (sys *GodBeastBagSystem) OnOpen() {
	sys.S2CInfo()
}

func (sys *GodBeastBagSystem) OnLogin() {
	sys.S2CInfo()
}

func (sys *GodBeastBagSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *GodBeastBagSystem) S2CInfo() {
	sys.SendProto3(58, 1, &pb3.S2C_58_1{
		Items: pb3.BatchConvertItemStToSimpleGodBeastItemSt(sys.GetMainData().ItemPool.GodBeastBag),
	})
}

func (sys *GodBeastBagSystem) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagGodGodBeastType)
}

func (sys *GodBeastBagSystem) genRandomAttr(item *pb3.ItemSt) {
	owner := sys.GetOwner()
	// 已存在随机属性
	if len(item.Attrs) != 0 {
		return
	}

	itemConf := jsondata.GetItemConfig(item.ItemId)
	if itemConf == nil {
		return
	}

	if !itemdef.IsGodBeastEquip(itemConf.Type) {
		return
	}

	if !itemdef.IsGodBeastEquipmentType(itemConf.SubType) {
		return
	}

	// 随机属性工厂
	var randomAttrFactory = func(number int32, attrLib []*jsondata.GodBeastRandAttr, attrSet map[uint32]struct{}) {
		// 进行属性不放回 暴力点 每拿一个就循环一遍 避免极限情况下出现同一个随机库一直出相同的属性
		for i := int32(0); i < number; i++ {
			pool := new(random.Pool)
			for _, attr := range attrLib {
				val := attr
				_, ok := attrSet[val.Type]
				if ok {
					continue
				}
				pool.AddItem(val, val.Weight)
			}

			// 出现问题直接让它抛出
			randomOne := pool.RandomOne()
			if randomOne == nil {
				owner.LogWarn("not random one attr,val is nil")
				continue
			}

			randomAttr, ok := randomOne.(*jsondata.GodBeastRandAttr)
			if !ok {
				owner.LogWarn("random one convert failed %v", randomOne)
				continue
			}

			attrSet[randomAttr.Type] = struct{}{}
			item.Attrs = append(item.Attrs, &pb3.AttrSt{
				Type:  randomAttr.Type,
				Value: random.IntervalUU(randomAttr.ValueMin, randomAttr.ValueMax),
			})
		}
	}

	lib, ok := jsondata.GetGodBeastRandAttrLib(itemConf.Quality, itemConf.Star)
	if !ok {
		return
	}

	var attrSet = make(map[uint32]struct{}, lib.NormalAttrNum+lib.ExcellentAttrNum)
	if lib.NormalAttrNum > 0 {
		randomAttrFactory(lib.NormalAttrNum, lib.NormalAttr, attrSet)
	}

	if lib.ExcellentAttrNum > 0 {
		randomAttrFactory(lib.ExcellentAttrNum, lib.ExcellentAttr, attrSet)
	}
}

func (sys *GodBeastBagSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	// sys.genRandomAttr(item)
	sys.SendProto3(58, 11, &pb3.S2C_58_11{
		Items: []*pb3.SimpleGodBeastItemSt{item.ToSimpleGodBeastItemSt()},
	})
	sys.changeStat(item, item.GetCount(), false, logId)

	if checkLogId(logId) {
		sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	if bTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (sys *GodBeastBagSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(58, 12, &pb3.S2C_58_12{
		Items: []*pb3.SimpleGodBeastItemSt{item.ToSimpleGodBeastItemSt()},
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

func (sys *GodBeastBagSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(58, 13, &pb3.S2C_58_13{
		Handles: []uint64{item.Handle},
	})
	has := item.GetCount()
	sys.changeStat(item, 0-has, true, logId)
}

func (sys *GodBeastBagSystem) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   sys.GetItemCount(item.GetItemId(), -1),
	})
}

func (sys *GodBeastBagSystem) c2sInfo(_ *base.Message) error {
	sys.S2CInfo()
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiGodBeastBag, func() iface.ISystem {
		return &GodBeastBagSystem{}
	})
	net.RegisterSysProtoV2(58, 1, sysdef.SiGodBeastBag, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodBeastBagSystem).c2sInfo
	})
}
