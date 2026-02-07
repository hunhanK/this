/**
 * @Author: lzp
 * @Date: 2024/8/5
 * @Desc:仙灵装备
**/

package actorsystem

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FairyEquipSys struct {
	Base
	*miscitem.Container
}

func (sys *FairyEquipSys) OnInit() {
	mainData := sys.GetMainData()

	itemPool := mainData.ItemPool
	if itemPool == nil {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}

	if itemPool.FairyEquips == nil {
		itemPool.FairyEquips = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewContainer(&mainData.ItemPool.FairyEquips)
	container.DefaultSizeHandle = sys.DefaultSize
	container.OnAddNewItem = sys.OnAddNewItem
	container.OnItemChange = sys.OnItemChange
	container.OnRemoveItem = sys.OnRemoveItem
	container.OnDeleteItemPtr = sys.owner.OnDeleteItemPtr
	sys.Container = container
}

func (sys *FairyEquipSys) IsOpen() bool {
	return true
}

func (sys *FairyEquipSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *FairyEquipSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *FairyEquipSys) S2CInfo() {
	sys.SendProto3(27, 35, &pb3.S2C_27_35{
		Items: sys.GetMainData().ItemPool.FairyEquips,
	})
}

func (sys *FairyEquipSys) GetData() map[uint64]*pb3.FairyEquipData {
	binary := sys.GetBinaryData()
	if binary.FairyEquips == nil {
		binary.FairyEquips = make(map[uint64]*pb3.FairyEquipData)
	}
	return binary.FairyEquips
}

func (sys *FairyEquipSys) GetFairyEquipData(hdl uint64) *pb3.FairyEquipData {
	data := sys.GetData()
	eData, ok := data[hdl]
	if !ok {
		data[hdl] = &pb3.FairyEquipData{
			PosData: make(map[uint32]uint64),
		}
		eData = data[hdl]
	}

	return eData
}

func (sys *FairyEquipSys) DefaultSize() uint32 {
	return jsondata.GetBagDefaultSize(bagdef.BagFairyEquipType)
}

func (sys *FairyEquipSys) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	sys.genAttrs(item)
	sys.SendProto3(27, 36, &pb3.S2C_27_36{Items: []*pb3.ItemSt{item}, LogId: uint32(logId)})
	sys.changeStat(item, item.GetCount(), false, logId)
	if bTip {
		sys.owner.SendTipMsg(tipmsgid.TpAddItem, common.PackUserItem(item), item.GetCount(), uint32(logId))
	}
}

func (sys *FairyEquipSys) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	sys.SendProto3(27, 36, &pb3.S2C_27_36{Items: []*pb3.ItemSt{item}, LogId: uint32(param.LogId)})
	sys.changeStat(item, add, false, param.LogId)
}

func (sys *FairyEquipSys) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	sys.SendProto3(27, 37, &pb3.S2C_27_37{Handles: []uint64{item.GetHandle()}})
	has := item.GetCount()
	sys.changeStat(item, 0-has, true, logId)
}

func (sys *FairyEquipSys) SyncItemChange(itemSt *pb3.ItemSt, logId pb3.LogId) {
	singleCalc := attrcalc.GetSingleCalc()
	defer func() {
		singleCalc.Reset()
	}()
	sys.calcFairyEquipAttrCalc(itemSt, singleCalc)
	job := sys.GetOwner().GetJob()
	power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(job))
	itemSt.Ext.Power = uint64(power)
	sys.OnItemChange(itemSt, 0, common.EngineGiveRewardParam{LogId: logId, NoTips: false})
	singleCalc.Reset()
}

func (sys *FairyEquipSys) calcFairyEquipAttrCalc(itemSt *pb3.ItemSt, calc *attrcalc.FightAttrCalc) {
	//主属性
	for _, attr := range itemSt.Attrs {
		calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
	}
	// 副属性
	for _, attr := range itemSt.Attrs2 {
		calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
	}
}

func (sys *FairyEquipSys) c2sEnhance(msg *base.Message) error {
	var req pb3.C2S_27_33
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}

	fairyData := sys.getFairyData()
	equipData, ok := fairyData.PosEquip[req.Slot]
	if !ok {
		return neterror.ParamsInvalidError("fairy equip not found slot:%d, pos:%d", req.Slot, req.Pos)
	}
	hdl, ok := equipData.PosData[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("fairy equip not found slot:%d, pos:%d", req.Slot, req.Pos)
	}

	itemSt := sys.getFairyEquipItem(hdl)
	if itemSt == nil {
		return neterror.ParamsInvalidError("fairy equip not found hdl:%d", hdl)
	}

	conf := jsondata.GetFEquipConf(itemSt.ItemId)
	if conf == nil {
		return neterror.ParamsInvalidError("item not found itemId:%d", itemSt.ItemId)
	}

	// 等级校验
	lvConf := conf.EnhanceConf[len(conf.EnhanceConf)-1]
	if itemSt.Union1 >= lvConf.Lv {
		return neterror.ParamsInvalidError("enhance lv is full, id:%d", itemSt.ItemId)
	}

	addExp, hldL := sys.getCalcExp(req.HdlMap)

	// 消耗
	for _, hdl := range hldL {
		count := req.HdlMap[hdl]
		consumeItem := sys.getFairyEquipItem(hdl)
		if !sys.GetOwner().DeleteItemPtr(consumeItem, int64(count), pb3.LogId_LogFairyEquipEnhance) {
			sys.GetOwner().LogWarn("enhance consumes failed")
		}
	}

	// 加经验
	sys.addExp(itemSt, addExp)

	sys.SendProto3(27, 33, &pb3.S2C_27_33{
		Slot:   req.Slot,
		Pos:    req.Pos,
		ItemSt: itemSt,
	})

	return nil
}

func (sys *FairyEquipSys) addExp(itemSt *pb3.ItemSt, addExp uint32) {
	itemId := itemSt.ItemId

	lConf := jsondata.GetFEquipEnhanceConf(itemId, itemSt.Union1+1)
	if lConf == nil {
		return
	}

	itemSt.Union2 += addExp
	oldLv := itemSt.Union1

	for lConf != nil && lConf.ConsumeExp != 0 && itemSt.Union2 >= lConf.ConsumeExp {
		itemSt.Union2 -= lConf.ConsumeExp
		itemSt.Union1 += 1
		lConf = jsondata.GetFEquipEnhanceConf(itemId, itemSt.Union1+1)
	}

	newLv := itemSt.Union1
	if oldLv < newLv {
		sys.afterLvUp(oldLv, itemSt)
	}
}

func (sys *FairyEquipSys) afterLvUp(oldLv uint32, itemSt *pb3.ItemSt) {
	lvConf := jsondata.GetFEquipEnhanceConf(itemSt.ItemId, itemSt.Union1)
	if lvConf == nil {
		return
	}

	conf := jsondata.GetFEquipConf(itemSt.ItemId)
	if conf == nil {
		return
	}

	job := sys.GetOwner().GetJob()
	// 主属性加成
	for _, attrConf := range lvConf.Attrs {
		if attrConf.Job <= 0 || attrConf.Job == job {
			for _, attr := range itemSt.Attrs {
				if attrConf.Type == attr.Type {
					attr.Value += attrConf.Value
				}
			}
			itemSt.Attrs = append(itemSt.Attrs, &pb3.AttrSt{Type: attrConf.Type, Value: attrConf.Value})
		}
	}

	// 副属性加成
	for tmpLv := oldLv + 1; tmpLv <= itemSt.Union1; tmpLv++ {
		tmpConf := jsondata.GetFEquipEnhanceConf(itemSt.ItemId, tmpLv)
		if tmpConf == nil {
			continue
		}
		if len(tmpConf.Rates) > 0 {
			if len(itemSt.Attrs2) < int(conf.MinorAttrMaxNum) {
				// 新增副属性
				sys.addMinorAttrs(itemSt)
				continue
			}

			// 副属性满之后，随机一条加成
			if len(lvConf.Rates) < 2 {
				continue
			}
			idx := random.Interval(0, len(itemSt.Attrs2)-1)
			attr := itemSt.Attrs2[idx]
			minRate, maxRate := lvConf.Rates[0], lvConf.Rates[1]
			rate := random.IntervalUU(minRate, maxRate)
			if itemSt.Ext.FEquipAttrRatioMap == nil {
				itemSt.Ext.FEquipAttrRatioMap = make(map[uint32]uint32)
			}
			itemSt.Ext.FEquipAttrRatioMap[attr.Type] += rate
		}
	}
	sys.ResetSysAttr(attrdef.SaFairyEquip)
	sys.SyncItemChange(itemSt, pb3.LogId_LogFairyEquipEnhance)
}

func (sys *FairyEquipSys) getCalcExp(hdlMap map[uint64]uint32) (uint32, []uint64) {
	var exp uint32
	var hdlL []uint64
	for hdl, count := range hdlMap {
		itemSt := sys.getFairyEquipItem(hdl)
		if itemSt == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(itemSt.ItemId)
		switch itemConf.Type {
		case itemdef.ItemTypeFairyEquip:
			conf := jsondata.GetFEquipConf(itemSt.ItemId)
			if conf == nil {
				continue
			}
			exp += conf.BaseExp
			hdlL = append(hdlL, hdl)
			lConf := jsondata.GetFEquipEnhanceConf(itemSt.ItemId, itemSt.Union1)
			if lConf != nil {
				rateExp := utils.CalcMillionRate64(int64(lConf.ConsumeExp), int64(conf.Rate))
				exp += uint32(rateExp)
			}
		case itemdef.ItemTypeFairyEquipMaterials:
			exp += itemConf.CommonField * count
			hdlL = append(hdlL, hdl)
		}
	}

	return exp, hdlL
}

func (sys *FairyEquipSys) afterTakeOn() {
	sys.ResetSysAttr(attrdef.SaFairyEquip)
}

func (sys *FairyEquipSys) afterTakeOff() {
	sys.ResetSysAttr(attrdef.SaFairyEquip)
}

func (sys *FairyEquipSys) genAttrs(item *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(item.ItemId)
	if itemConf == nil {
		return
	}

	if !itemdef.IsFairyEquip(itemConf.Type) {
		return
	}

	if !itemdef.IsFairyEquipSubType(itemConf.SubType) {
		return
	}

	conf := jsondata.GetFEquipConf(itemConf.Id)
	if conf == nil {
		return
	}

	makeAttrs := func(counts []uint32, attrs []*jsondata.FairyEquipAttr) []*pb3.AttrSt {
		if len(counts) < 2 {
			return nil
		}
		count := random.IntervalUU(counts[0], counts[1])
		pool := new(random.Pool)
		for _, v := range attrs {
			pool.AddItem(v, v.Weight)
		}
		rets := pool.RandomMany(count)

		var tmpAttrs []*pb3.AttrSt
		for _, ret := range rets {
			attrConf := ret.(*jsondata.FairyEquipAttr)
			tmpAttrs = append(tmpAttrs, &pb3.AttrSt{
				Type:  attrConf.Type,
				Value: random.IntervalUU(attrConf.ValueMin, attrConf.ValueMax),
			})
		}
		return tmpAttrs
	}

	if len(conf.MainAttrNum) > 0 && len(conf.MainAttr) > 0 {
		attrs := makeAttrs(conf.MainAttrNum, conf.MainAttr)
		item.Attrs = attrs
	}

	if len(conf.MinorAttrNum) > 0 && len(conf.MinorAttr) > 0 {
		attrs := makeAttrs(conf.MinorAttrNum, conf.MinorAttr)
		item.Attrs2 = attrs
	}

	if item.Ext.FEquipAttrRatioMap == nil {
		item.Ext.FEquipAttrRatioMap = make(map[uint32]uint32)
	}

	// 初始化战力
	singleCalc := attrcalc.GetSingleCalc()
	sys.calcFairyEquipAttrCalc(item, singleCalc)
	job := sys.GetOwner().GetJob()
	power := attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(job))
	item.Ext.Power = uint64(power)
	singleCalc.Reset()
}

// 添加一条副属性
func (sys *FairyEquipSys) addMinorAttrs(item *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(item.ItemId)
	if itemConf == nil {
		return
	}

	conf := jsondata.GetFEquipConf(itemConf.Id)
	if conf == nil {
		return
	}

	var checkHasAttr = func(attrId uint32, attrs []*pb3.AttrSt) bool {
		for _, attr := range attrs {
			if attr.Type == attrId {
				return true
			}
		}
		return false
	}

	pool := new(random.Pool)
	for _, v := range conf.MinorAttr {
		if checkHasAttr(v.Type, item.Attrs2) {
			continue
		}
		pool.AddItem(v, v.Weight)
	}

	ret := pool.RandomOne()
	attrConf := ret.(*jsondata.FairyEquipAttr)
	item.Attrs2 = append(item.Attrs2, &pb3.AttrSt{
		Type:  attrConf.Type,
		Value: random.IntervalUU(attrConf.ValueMin, attrConf.ValueMax),
	})
}

func (sys *FairyEquipSys) changeStat(item *pb3.ItemSt, add int64, remove bool, logId pb3.LogId) {
	logworker.LogItem(sys.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  add,
		LogId:  uint32(logId),
		Left:   sys.GetItemCount(item.GetItemId(), -1),
	})
}

func (sys *FairyEquipSys) getFairy(hdl uint64) *pb3.ItemSt {
	if fairyBag, ok := sys.owner.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem); ok {
		item := fairyBag.FindItemByHandle(hdl)
		if item != nil {
			return item
		}
	}
	return nil
}

func (sys *FairyEquipSys) getFairyData() *pb3.FairyData {
	if fairySys, ok := sys.owner.GetSysObj(sysdef.SiFairy).(*FairySystem); ok {
		return fairySys.GetData()
	}
	return nil
}

func (sys *FairyEquipSys) getFairyEquipItem(hdl uint64) *pb3.ItemSt {
	item := sys.FindItemByHandle(hdl)
	if item != nil {
		return item
	}
	return nil
}

func (sys *FairyEquipSys) getFairyEquipOnNum() uint32 {
	fData := sys.getFairyData()
	var count uint32
	for _, fEquip := range fData.PosEquip {
		for _, eHdl := range fEquip.PosData {
			if eHdl > 0 {
				count++
			}
		}
	}
	return count
}

func (sys *FairyEquipSys) calcFairyEquipAttr(calc *attrcalc.FightAttrCalc) {
	fairyData := sys.getFairyData()
	if fairyData == nil || fairyData.PosEquip == nil {
		return
	}

	// 仙灵装备属性
	var calcFairyEquip = func(data *pb3.FairyEquipData) jsondata.AttrVec {
		var attrs jsondata.AttrVec
		for _, hdl := range data.PosData {
			// 找到对应的装备ItemSt
			itemSt := sys.GetOwner().GetFairyEquipItemByHandle(hdl)
			if itemSt == nil {
				continue
			}
			// 主属性
			for _, attr := range itemSt.Attrs {
				attrs = append(attrs, &jsondata.Attr{Type: attr.Type, Value: attr.Value})
			}
			// 副属性
			for _, attr := range itemSt.Attrs2 {
				k := attr.Type
				v := attr.Value

				ratio := itemSt.Ext.FEquipAttrRatioMap[k]
				v += utils.CalcMillionRate(attr.Value, ratio)

				attrs = append(attrs, &jsondata.Attr{Type: k, Value: v})
			}
		}
		return attrs
	}

	// 仙灵装备套装属性
	var calcFairyEquipSuit = func(data *pb3.FairyEquipData) jsondata.AttrVec {
		suitMap := make(map[uint32]uint32)
		for _, hdl := range data.PosData {
			itemSt := sys.GetOwner().GetFairyEquipItemByHandle(hdl)
			if itemSt == nil {
				continue
			}
			conf := jsondata.GetFEquipConf(itemSt.ItemId)
			if conf == nil {
				continue
			}
			suitMap[conf.SuitId] += 1
		}
		var attrs jsondata.AttrVec
		for sId, num := range suitMap {
			conf := jsondata.GetFEquipSuitConf(sId)
			if conf == nil {
				continue
			}
			for _, sConf := range conf.Suits {
				if num >= sConf.Num {
					attrs = append(attrs, sConf.Attrs...)
				}
			}
		}
		return attrs
	}

	// 孔位上没有上阵仙灵的不算
	for slot, equipData := range fairyData.PosEquip {
		fHdl := fairyData.BattleFairy[slot]
		if fHdl == 0 {
			continue
		}
		attrs1 := calcFairyEquip(equipData)
		attrs2 := calcFairyEquipSuit(equipData)
		engine.CheckAddAttrsToCalc(sys.owner, calc, attrs1)
		engine.CheckAddAttrsToCalc(sys.owner, calc, attrs2)
	}
}

func calcFairyEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if sys, ok := player.GetSysObj(sysdef.SiFairyEquip).(*FairyEquipSys); ok && sys.IsOpen() {
		sys.calcFairyEquipAttr(calc)
	}
}

func fairyEquipSuitQuestTarget(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	sys, ok := actor.GetSysObj(sysdef.SiFairyEquip).(*FairyEquipSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	fairyData := sys.getFairyData()
	// 仙灵装备套装属性
	var calcFairyEquipSuit = func(data *pb3.FairyEquipData) uint32 {
		var count uint32
		var suitMap = make(map[uint32]uint32)
		for _, hdl := range data.PosData {
			itemSt := sys.GetOwner().GetFairyEquipItemByHandle(hdl)
			if itemSt == nil {
				continue
			}
			conf := jsondata.GetFEquipConf(itemSt.ItemId)
			if conf == nil {
				continue
			}
			suitMap[conf.SuitId] += 1
		}
		for sId, num := range suitMap {
			conf := jsondata.GetFEquipSuitConf(sId)
			if conf == nil {
				continue
			}
			for _, sConf := range conf.Suits {
				if num >= sConf.Num {
					count += 1
				}
			}
		}
		return count
	}
	var count uint32
	// 孔位上没有上阵仙灵的不算
	for slot, equipData := range fairyData.PosEquip {
		fHdl := fairyData.BattleFairy[slot]
		if fHdl == 0 {
			continue
		}
		count += calcFairyEquipSuit(equipData)
	}
	return count
}

func init() {
	RegisterSysClass(sysdef.SiFairyEquip, func() iface.ISystem {
		return &FairyEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairyEquip, calcFairyEquipAttr)
	engine.RegQuestTargetProgress(custom_id.QttFairyEquipSuitNum, fairyEquipSuitQuestTarget)

	//engine.RegQuestTargetProgress(custom_id.QttFairyEquipNum, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	//	obj := actor.GetSysObj(gshare.SiFairyEquip)
	//	if obj == nil || !obj.IsOpen() {
	//		return 0
	//	}
	//	sys, ok := obj.(*FairyEquipSys)
	//	if !ok {
	//		return 0
	//	}
	//	return sys.getFairyEquipOnNum()
	//})

	net.RegisterSysProtoV2(27, 33, sysdef.SiFairyEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FairyEquipSys).c2sEnhance
	})
}
