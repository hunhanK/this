package actorsystem

import (
	"jjyz/base/attrcalc"
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
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"

	"github.com/gzjjyz/srvlib/utils"
)

type FairyBagSystem struct {
	Base
	*miscitem.Container
}

func (sys *FairyBagSystem) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.FairyBag {
		itemPool.FairyBag = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.FairyBag)
	container.DefaultSizeHandle = sys.DefaultSize
	container.EnlargeSizeHandle = sys.EnlargeSize
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	container.OnDeleteItemPtr = sys.owner.OnDeleteItemPtr
	sys.Container = container
}

func (sys *FairyBagSystem) OnLogin() {
	sys.S2CInfo()
}

func (sys *FairyBagSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *FairyBagSystem) S2CInfo() {
	sys.SendProto3(27, 0, &pb3.S2C_27_0{
		Items: sys.GetMainData().ItemPool.FairyBag,
	})
}

func (sys *FairyBagSystem) IsOpen() bool {
	return true
}

func (sys *FairyBagSystem) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagFairyType)
}

func (sys *FairyBagSystem) EnlargeSize() uint32 {
	return 0
}

func (sys *FairyBagSystem) initFairyExt(fairy *pb3.ItemSt) {
	itemId := fairy.GetItemId()
	fairyConf := jsondata.GetFairyConf(itemId)
	if nil == fairyConf {
		return
	}
	fairy.Union1 = 1                                                                        //初始化等级
	fairy.Union2 = fairyConf.Star                                                           //初始化星级
	fairy.Ext = &pb3.ItemExt{FairyBackNum: 0, FairyBreakLv: 1, FairyGrade: fairyConf.Grade} //突破等级默认1
	var slotNum uint32
	if fairyConf.Grade <= custom_id.FairyGradeSpBegin {
		slotNum = jsondata.GlobalUint("normalFairySkillNum")
	} else {
		slotNum = jsondata.GlobalUint("srcGodFairySkillNum")
	}
	fairy.Ext.FairySkillSoltNum = slotNum
	fairy.Ext.FairySkill = make(map[uint32]*pb3.Skill)
	fairy.Ext.StarConsume = make(map[uint32]uint32)
	for i, skill := range fairyConf.SkillGive {
		if skill == 0 {
			continue
		}
		fairy.Ext.FairySkill[uint32(i)] = &pb3.Skill{
			Id:    skill,
			Level: 1,
		}
	}
	fairySys := sys.GetOwner().GetSysObj(sysdef.SiFairy).(*FairySystem)
	if fairySys != nil && fairySys.IsOpen() {
		singleCalc := attrcalc.GetSingleCalc()
		fairySys.calcFairyAttrCalc(fairy, singleCalc)
		player := sys.GetOwner()
		power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(player.GetJob()))
		fairy.Ext.Power = uint64(power)
		singleCalc.Reset()
	}
}

func (sys *FairyBagSystem) InitFairyItem(item *pb3.ItemSt) {
	itemId := item.GetItemId()
	sys.initFairyExt(item)
	binary := sys.GetBinaryData()
	fairyData := binary.GetFairyData()
	fairyData.HistoricalStar[itemId] = utils.MaxUInt32(fairyData.HistoricalStar[itemId], item.Union2)
	onFairyLvChangeQuest(sys.owner, itemId)
	onFairyStarChangeQuest(sys.owner, itemId)
	sys.owner.SendProto3(27, 22, &pb3.S2C_27_22{
		ConfId: itemId,
		Star:   item.Union2,
	})
}

func onFairyLvChangeQuest(player iface.IPlayer, itemId uint32) {
	var maxFairyLv uint32
	var appointFairyLv uint32
	for _, itemSt := range player.GetMainData().ItemPool.FairyBag {
		maxFairyLv = utils.MaxUInt32(maxFairyLv, itemSt.Union1)
		if itemSt.ItemId == itemId {
			appointFairyLv = utils.MaxUInt32(appointFairyLv, itemSt.Union1)
		}
	}
	player.TriggerQuestEventRange(custom_id.QttFairyLvMax)
}

func onFairyStarChangeQuest(player iface.IPlayer, itemId uint32) {
	var maxFairyStar uint32
	var appointFairyStar uint32
	for _, itemSt := range player.GetMainData().ItemPool.FairyBag {
		maxFairyStar = utils.MaxUInt32(maxFairyStar, itemSt.Union2)
		if itemSt.ItemId == itemId {
			appointFairyStar = utils.MaxUInt32(appointFairyStar, itemSt.Union2)
		}
	}
	player.TriggerQuestEventRange(custom_id.QttFairyStarMax)
	player.TriggerQuestEventRange(custom_id.QttFairyStarNum)
}

func (sys *FairyBagSystem) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.InitFairyItem(item)

	sys.SendProto3(27, 1, &pb3.S2C_27_1{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	sys.changeStat(item, item.GetCount(), false, logId)

	if bTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}

	if logId == pb3.LogId_LogDrawDramaRewardFirst {
		sys.owner.TriggerEvent(custom_id.AeGetFirstFairy, item.GetHandle())
	}

	if checkLogId(logId) {
		sys.owner.TriggerQuestEvent(custom_id.QttGetItemNum, item.GetItemId(), item.GetCount())
	}

	sys.owner.TriggerEvent(custom_id.AeGetNewFairy, item.GetHandle())
}

func (sys *FairyBagSystem) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(27, 1, &pb3.S2C_27_1{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
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

func (sys *FairyBagSystem) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	onFairyLvChangeQuest(sys.owner, item.GetItemId())
	onFairyStarChangeQuest(sys.owner, item.GetItemId())
	sys.SendProto3(27, 2, &pb3.S2C_27_2{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	sys.changeStat(item, 0-has, true, logId)
}

func (sys *FairyBagSystem) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   sys.GetItemCount(item.GetItemId(), -1),
	})
}

var _ iface.IBagSys = (*FairyBagSystem)(nil)

func AddAllFairy(actor iface.IPlayer, args ...string) bool {
	for itemId := range jsondata.FairyConfMgr {
		st := &itemdef.ItemParamSt{
			ItemId: itemId,
			Count:  1,
			Bind:   true,
			LogId:  pb3.LogId_LogGm,
		}
		if actor.CanAddItem(st, true) {
			actor.AddItem(st)
		}
	}
	return true
}

func init() {
	gmevent.Register("addFairy", AddAllFairy, 1)

	RegisterSysClass(sysdef.SiFairyBag, func() iface.ISystem {
		return &FairyBagSystem{}
	})
}
