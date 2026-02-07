package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

/*
desc:商城系统
author: ChenJunJi
*/

type StoreSys struct {
	Base
}

func (sys *StoreSys) OnInit() {
	binary := sys.GetBinaryData()
	data := binary.StoreData
	if nil == data {
		binary.StoreData = &pb3.StoreData{}
		data = binary.StoreData
	}

	if nil == data.TotalBuy {
		data.TotalBuy = make(map[uint32]uint32)
	}

	if nil == data.WeekBuy {
		data.WeekBuy = make(map[uint32]uint32)
	}

	if nil == data.DailyBuy {
		data.DailyBuy = make(map[uint32]uint32)
	}

	if nil == data.SpecCycleBuy {
		data.SpecCycleBuy = make(map[uint32]uint32)
	}

	if nil == data.QuestRecords {
		data.QuestRecords = make(map[uint64]uint32)
	}
}

func (sys *StoreSys) OnNewDay() {
	if data := sys.GetBinaryData(); nil != data {
		data.StoreData.DailyBuy = make(map[uint32]uint32)
	}
	sys.s2cDailyLimitInfo(nil)
}

func (sys *StoreSys) OnNewWeek() {
	if data := sys.GetBinaryData(); nil != data {
		data.StoreData.WeekBuy = make(map[uint32]uint32)
	}
	sys.s2cWeekLimitInfo(nil)
}

func (sys *StoreSys) OnReconnect() {
	sys.s2cLimitInfo(nil)
}

// 下发限购购买信息
func (sys *StoreSys) s2cLimitInfo(_ *base.Message) {
	sys.SendProto3(30, 1, &pb3.S2C_30_1{Data: sys.GetBinaryData().StoreData})
}

func (sys *StoreSys) s2cDailyLimitInfo(_ *base.Message) {
	sys.SendProto3(30, 4, &pb3.S2C_30_4{})
}

func (sys *StoreSys) s2cWeekLimitInfo(_ *base.Message) {
	sys.SendProto3(30, 5, &pb3.S2C_30_5{})
}

// 购买商品
func (sys *StoreSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	id, count := req.GetId(), req.GetCount()
	conf := jsondata.GetStoreConf(id)
	if nil == conf {
		return neterror.ConfNotFoundError("not found store id %d", id)
	}

	storeBuyLimit := jsondata.GlobalUint("storeBuyLimit")
	if storeBuyLimit > 0 && count > storeBuyLimit {
		return neterror.ParamsInvalidError("storeBuyLimit %d , count %d", storeBuyLimit, count)
	}

	//开服天数
	if conf.OpenServerDay > 0 && conf.OpenServerDay > gshare.GetOpenServerDay() {
		return neterror.ParamsInvalidError("conf.OpenServerDay %d , OpenServerDay %d", conf.OpenServerDay, gshare.GetOpenServerDay())
	}

	//合服次数
	if conf.MergeTimes > 0 && conf.MergeTimes > gshare.GetMergeTimes() {
		return neterror.ParamsInvalidError("conf.MergeTimes %d , MergeTimes %d", conf.MergeTimes, gshare.GetMergeTimes())
	}

	vipLevel := sys.owner.GetExtraAttrU32(attrdef.VipLevel)
	if conf.Vip > vipLevel {
		return neterror.ParamsInvalidError("conf.Vip %d , vipLevel %d", conf.Vip, vipLevel)
	}

	if conf.Level > sys.owner.GetLevel() {
		return neterror.ParamsInvalidError("conf.Level %d , sys.owner.GetLevel %d", conf.Level, sys.owner.GetLevel())
	}

	if conf.FreeVipLevel > 0 {
		obj := sys.GetOwner().GetSysObj(sysdef.SiFreeVip)
		if obj == nil {
			return neterror.ParamsInvalidError("conf.FreeVipLevel %d , sys.owner.FreeVip is nil", conf.FreeVipLevel)
		}
		vipSys, ok := obj.(*FreeVipSys)
		if !ok {
			return neterror.ParamsInvalidError("conf.FreeVipLevel %d , sys.owner.FreeVip convert failed ,lv %d", conf.FreeVipLevel, 0)
		}
		if conf.FreeVipLevel > vipSys.getData().GetLevel() {
			return neterror.ParamsInvalidError("conf.FreeVipLevel %d , sys.owner.FreeVipLevel %d", conf.FreeVipLevel, vipSys.getData().GetLevel())
		}
	}

	if conf.Circle > 0 {
		if conf.Circle > sys.owner.GetCircle() {
			return neterror.ParamsInvalidError("conf.Circle %d , sys.owner.Circle %d", conf.Circle, sys.owner.GetCircle())
		}
	}

	data := sys.GetBinaryData().StoreData
	if nil == data {
		return neterror.ParamsInvalidError("ys.GetBinaryData().StoreData is nil")
	}

	// 需要购买前置商品
	if len(conf.GoodsIds) > 0 {
		sys.LogDebug("conf.GoodsIds  %d", conf.GoodsIds)
		for _, goodsId := range conf.GoodsIds {
			if data.TotalBuy[goodsId] > 0 {
				continue
			}
			return neterror.ParamsInvalidError("data.TotalBuy[%d] <= 0", goodsId)
		}
	}

	var vipAddLimit uint32
	if vipLevel > 0 {
		if len(conf.VipAddLimit) > int(vipLevel) {
			vipAddLimit = conf.VipAddLimit[int(vipLevel)-1]
		}
	}
	var totalBuy, weekBuy, dailyBuy, specCycleBuy uint32

	if conf.Limit > 0 {
		totalBuy = data.TotalBuy[id] + count
		if totalBuy > conf.Limit+vipAddLimit {
			return neterror.ParamsInvalidError("totalBuy > conf.Limit+vipAddLimit")
		}
	}

	if conf.WeeklyLimit > 0 {
		weekBuy = data.WeekBuy[id] + count
		if weekBuy > conf.WeeklyLimit+vipAddLimit {
			return neterror.ParamsInvalidError("weekBuy > conf.WeeklyLimit+vipAddLimit")
		}
	}

	if conf.DailyLimit > 0 {
		dailyBuy = data.DailyBuy[id] + count
		if dailyBuy > conf.DailyLimit+vipAddLimit {
			return neterror.ParamsInvalidError("dailyBuy > conf.DailyLimit+vipAddLimit")
		}
	}

	if conf.SpecCycleLimit > 0 {
		specCycleBuy = data.SpecCycleBuy[id] + count
		if specCycleBuy > conf.SpecCycleLimit+vipAddLimit {
			return neterror.ParamsInvalidError("specCycleBuy > conf.SpecCycleLimit+vipAddLimit")
		}
	}

	// 生效商城道具购买消耗折扣
	var consumeVec = conf.Consume
	if conf.IsEffectDiscount() {
		valueAlias := sys.GetOwner().GetFightAttr(attrdef.StoreSubConsumeRate)
		consumeVec = jsondata.CalcConsumeDiscount(conf.Consume, valueAlias)
	}
	if len(consumeVec) > 0 {
		if !sys.owner.ConsumeRate(consumeVec, int64(count), false, common.ConsumeParams{
			LogId:   pb3.LogId_LogStoreBuyItem,
			SubType: conf.Type,
			RefId:   conf.SubType,
		}) {
			sys.GetOwner().SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ParamsInvalidError("ConsumeRate failed")
		}
	} else {
		totalCost := conf.MoneyCount * count
		consume := jsondata.ConsumeVec{
			{Type: custom_id.ConsumeTypeMoney, Id: conf.MoneyType, Count: totalCost},
		}
		if conf.IsEffectDiscount() {
			valueAlias := sys.GetOwner().GetFightAttr(attrdef.StoreSubConsumeRate)
			consume = jsondata.CalcConsumeDiscount(consume, valueAlias)
		}
		if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{
			LogId:   pb3.LogId_LogStoreBuyItem,
			SubType: conf.Type,
			RefId:   conf.SubType,
		}) {
			sys.GetOwner().SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ParamsInvalidError("ConsumeByConf failed")
		}
	}

	if totalBuy > 0 {
		data.TotalBuy[id] = totalBuy
	}

	if weekBuy > 0 {
		data.WeekBuy[id] = weekBuy
	}

	if dailyBuy > 0 {
		data.DailyBuy[id] = dailyBuy
	}

	if specCycleBuy > 0 {
		data.SpecCycleBuy[id] = specCycleBuy
	}

	engine.GiveRewards(sys.owner, []*jsondata.StdReward{{Id: conf.ItemId, Count: int64(conf.ItemCount * count), Bind: conf.Bind}},
		common.EngineGiveRewardParam{LogId: pb3.LogId_LogStoreBuyItem})

	sys.SendProto3(30, 2, &pb3.S2C_30_2{Id: req.Id, Count: req.Count, TodoId: req.TodoId})

	if totalBuy > 0 || weekBuy > 0 || dailyBuy > 0 || specCycleBuy > 0 {
		sys.SendProto3(30, 3, &pb3.S2C_30_3{Id: req.Id, TotalBuy: totalBuy, WeekBuy: weekBuy, DailyBuy: dailyBuy, SpecCycleBuy: specCycleBuy})
	}

	if conf.MoneyType == moneydef.Diamonds || conf.MoneyType == moneydef.BindDiamonds {
		sys.owner.TriggerQuestEvent(custom_id.QttDiamondShop, 0, 1)
		sys.owner.TriggerEvent(custom_id.AeStoreDiamondsBuy, conf.ItemId, conf.ItemCount*count)
	}

	sys.owner.TriggerQuestEvent(custom_id.QttBuyGoodsFromStore, id, int64(count))
	sys.owner.TriggerQuestEvent(custom_id.QttBuyGoodsFromStoreTimes, 0, int64(count))
	sys.owner.TriggerQuestEvent(custom_id.QttAchievementsBuyGoodsFromType, conf.Type, int64(count))
	data.QuestRecords[utils.Make64(conf.Type, id)] += 1
	sys.owner.TriggerQuestEventRange(custom_id.QttSpecStoreByIdGoods)
	return nil
}

func (sys *StoreSys) ResetSpecCycleBuy(typ uint32, subType uint32) error {
	storeConfs := jsondata.GetStoreConfByTypeAndSubType(typ, subType)
	data := sys.GetBinaryData().StoreData
	if len(data.SpecCycleBuy) == 0 {
		return nil
	}
	for _, id := range storeConfs {
		delete(data.SpecCycleBuy, id)
	}
	sys.s2cLimitInfo(nil)
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiStore, func() iface.ISystem {
		return &StoreSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiStore).(*StoreSys); ok && sys.IsOpen() {
			sys.OnNewDay()
		}
	})
	event.RegActorEvent(custom_id.AeNewWeek, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiStore).(*StoreSys); ok {
			sys.OnNewWeek()
		}
	})
	engine.RegQuestTargetProgress(custom_id.QttSpecStoreByIdGoods, handleQttSpecStoreByIdGoods)

	net.RegisterSysProto(30, 1, sysdef.SiStore, (*StoreSys).s2cLimitInfo)
	net.RegisterSysProto(30, 2, sysdef.SiStore, (*StoreSys).c2sBuy)
}

func handleQttSpecStoreByIdGoods(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 2 {
		return 0
	}
	if sys, ok := actor.GetSysObj(sysdef.SiStore).(*StoreSys); ok && sys.IsOpen() {
		if data := sys.GetBinaryData(); nil != data {
			return data.StoreData.QuestRecords[utils.Make64(ids[0], ids[1])]
		}
	}
	return 0
}
