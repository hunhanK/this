/**
 * @Author: YangQibin
 * @Desc:
 * @Date: 2023/7/8 11:18
 */

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/random"
)

type MysteryStoreSys struct {
	Base

	data *pb3.MysteryStoreData

	timer *time_util.Timer
}

func (sys *MysteryStoreSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *MysteryStoreSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.StartTimer()
	sys.s2cInfo()
}

func (sys *MysteryStoreSys) init() bool {
	binaryData := sys.GetBinaryData()
	if nil == binaryData.MysteryStoreData {
		binaryData.MysteryStoreData = &pb3.MysteryStoreData{
			ItemInfo: make([]*pb3.MysteryStoreItemInfo, 0),
		}
	}

	sys.data = binaryData.MysteryStoreData

	if nil == sys.data.ItemInfo {
		sys.data.ItemInfo = make([]*pb3.MysteryStoreItemInfo, 0)
	}
	return true
}

func (sys *MysteryStoreSys) s2cInfo() {
	sys.SendProto3(30, 10, &pb3.S2C_30_10{
		PersonalData: sys.data,
		Records:      sys.GetBuyRecord(),
	})
}

func (sys *MysteryStoreSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *MysteryStoreSys) OnAfterLogin() {
	if sys.data.NextRefreshTime > 0 {
		sys.StartTimer()
	}
	sys.s2cInfo()
}

func (sys *MysteryStoreSys) OnLogout() {
	if nil != sys.timer {
		sys.timer.Stop()
		sys.timer = nil
	}
}

func (sys *MysteryStoreSys) onNewDay() {
	sys.data.SumRefreshCount = 0
	sys.s2cInfo()
}

func (sys *MysteryStoreSys) StartTimer() {
	nowSec := time_util.NowSec()

	conf := jsondata.GetMysteryShopConf()

	nextTime := sys.data.NextRefreshTime
	if nextTime < nowSec {
		sys.Refresh(false)

		nextTime = nowSec + conf.AutoInterval
		sys.data.NextRefreshTime = nextTime
		sys.s2cInfo()
	}

	sys.timer = sys.owner.SetTimeout(time.Duration(nextTime-nowSec)*time.Second, func() {
		sys.StartTimer()
	})
}

func (sys *MysteryStoreSys) Refresh(ismanual bool) *neterror.NetError {
	conf := jsondata.GetMysteryShopConf()
	if conf == nil {
		return neterror.InternalError("get MysteryShopConf failed")
	}

	sys.data.ItemInfo = make([]*pb3.MysteryStoreItemInfo, 0, conf.CommodityLimit)

	if sys.data.NextRefreshTime == 0 { //初次刷新
		for _, idx := range conf.InitItem {
			sys.data.ItemInfo = append(sys.data.ItemInfo, &pb3.MysteryStoreItemInfo{
				Idx:   idx,
				IsBuy: false,
			})
		}
		return nil
	}

	circle := sys.GetOwner().GetCircle()
	openDay := gshare.GetOpenServerDay()

	pool := new(random.Pool)

	type itemInfoWithIdx struct {
		Idx int
		*jsondata.MysteryShopItemConf
	}

	itemCanChoose := make([]*itemInfoWithIdx, 0)
	for idx, item := range conf.ItemList {
		if item.Rate > 0 && (circle >= item.MinCircle && circle <= item.MaxCircle) &&
			(openDay >= item.OpenDay && openDay <= item.EndDay) {
			itemCanChoose = append(itemCanChoose, &itemInfoWithIdx{
				Idx:                 idx,
				MysteryShopItemConf: item,
			})
		}
	}

	if len(itemCanChoose) == 0 {
		return neterror.InternalError("itemCanChoose is nil")
	}

	for _, foo := range itemCanChoose {
		pool.AddItem(foo, foo.Rate)
	}

	var itemChoosed map[int]*pb3.MysteryStoreItemInfo = make(map[int]*pb3.MysteryStoreItemInfo, 0)

	// maxRandCount 用来防止死循环
	for maxRandCount := 0; len(itemChoosed) < int(conf.CommodityLimit) && maxRandCount < (1000*int(conf.CommodityLimit)); maxRandCount++ {
		idx := pool.RandomOne().(*itemInfoWithIdx).Idx
		if itemChoosed[idx] != nil {
			continue
		}

		itemChoosed[idx] = &pb3.MysteryStoreItemInfo{
			Idx:   uint32(idx),
			IsBuy: false,
		}
	}

	for _, item := range itemChoosed {
		sys.data.ItemInfo = append(sys.data.ItemInfo, item)
	}

	if ismanual {
		logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRefreshMysteryStore, &pb3.LogPlayerCounter{StrArgs: "manual"})
	} else {
		logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRefreshMysteryStore, &pb3.LogPlayerCounter{StrArgs: "not manual"})
	}

	sys.LogDebug("itemChoosed is %v", itemChoosed)

	return nil
}

func (sys *MysteryStoreSys) Buy(index uint32) error {
	if len(sys.data.ItemInfo) == 0 {
		return neterror.InternalError("sys.data.ItemInfo is nil")
	}

	if index >= uint32(len(sys.data.ItemInfo)) {
		return neterror.ParamsInvalidError("index %d is invalid maxIndex is %d", index, len(sys.data.ItemInfo))
	}

	itemInfo := sys.data.ItemInfo[index]
	if itemInfo.IsBuy {
		return neterror.ParamsInvalidError("item index %d id %d already buyed", index, itemInfo.Idx)
	}

	if nil == itemInfo {
		return neterror.ParamsInvalidError("itemInfo is nil for id %d", index)
	}

	conf := jsondata.GetMysteryShopItemConfByIdx(itemInfo.Idx)
	if nil == conf {
		return neterror.InternalError("item conf is nil")
	}

	itemConf := jsondata.GetItemConfig(conf.Id)
	if itemConf == nil {
		return neterror.InternalError("item conf is nil")
	}

	var rewards []*jsondata.StdReward
	rewards = append(rewards, &jsondata.StdReward{
		Id:    conf.Id,
		Count: int64(conf.Count),
		Bind:  conf.Bind,
	})

	if !sys.owner.DeductMoney(conf.MoneyType, int64(conf.CurrentPrice), common.ConsumeParams{LogId: pb3.LogId_LogShoppingAtMysteryStore}) {
		return neterror.ParamsInvalidError("deductMoney failed moneyType %d price %d", conf.MoneyType, conf.CurrentPrice)
	}

	itemInfo.IsBuy = true

	if !engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogShoppingAtMysteryStore}) {
		return neterror.InternalError("give rewards failed")
	}

	// 需要广播的
	if itemConf.IsRare == itemdef.ItemIsRare_Record {
		engine.BroadcastTipMsgById(tipmsgid.TpMysteryStoreShopping, sys.owner.GetId(), sys.owner.GetName(), conf.Id)
	}

	// 需要记录的
	if itemConf.IsRare == itemdef.ItemIsRare_Rare || itemConf.IsRare == itemdef.ItemIsRare_Record {
		sys.AddBuyRecord(itemInfo, sys.owner.GetId())
	}

	sys.SendProto3(30, 12, &pb3.S2C_30_12{
		Item: itemInfo,
	})

	sys.owner.TriggerQuestEvent(custom_id.QttBuyGoodsTimesFromStore, 0, int64(1))
	sys.owner.TriggerQuestEvent(custom_id.QttAchievementsBuyGoodsFromStore, 0, int64(1))
	sys.owner.TriggerQuestEvent(custom_id.QttBuyGoodsFromStoreTimes, 0, int64(1))

	return nil
}

func (sys *MysteryStoreSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_12
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	return sys.Buy(req.Index)
}

func (sys *MysteryStoreSys) c2sRefresh(_ *base.Message) error {
	conf := jsondata.GetMysteryShopConf()
	if conf == nil {
		return neterror.InternalError("conf is nil")
	}

	sumRefreshCount := sys.data.SumRefreshCount
	if sumRefreshCount >= conf.ManualLimit {
		return neterror.ParamsInvalidError("refresh time can use is zero")
	}

	if sumRefreshCount >= conf.FreeCount {
		times := sumRefreshCount - conf.FreeCount + 1
		consume := jsondata.GetMysteryShopRefreshConsumeByTimes(times)
		if nil == consume {
			return neterror.ConsumeFailedError("mystery store consume conf is nil")
		}
		if !sys.GetOwner().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogRefreshMysteryStore}) {
			return neterror.ConsumeFailedError("消耗失败")
		}
	}

	sys.data.SumRefreshCount++

	if err := sys.Refresh(true); err != nil {
		return err
	}

	sys.SendProto3(30, 11, &pb3.S2C_30_11{
		PersonalData: sys.data,
	})
	return nil
}

func (sys *MysteryStoreSys) AddBuyRecord(itemInfo *pb3.MysteryStoreItemInfo, actorId uint64) {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.MysteryStoreBuyRecords {
		globalVar.MysteryStoreBuyRecords = make([]*pb3.MysteryStoreBuyRecord, 0)
	}

	conf := jsondata.GetMysteryShopConf()
	size := uint32(len(globalVar.MysteryStoreBuyRecords))

	lineConf := jsondata.GetMysteryShopItemConfByIdx(itemInfo.Idx)
	if nil == lineConf {
		return
	}

	var first uint32
	if size >= conf.LogCount {
		first = size - conf.LogCount + 1
	}

	if first > 0 {
		globalVar.MysteryStoreBuyRecords = globalVar.MysteryStoreBuyRecords[first:]
	}

	actorName := sys.owner.GetName()

	record := &pb3.MysteryStoreBuyRecord{
		ActorName: actorName,
		ItemId:    lineConf.Id,
		Count:     lineConf.Count,
		MoneyType: lineConf.MoneyType,
		Money:     lineConf.CurrentPrice,
		ActorId:   actorId,
		Time:      uint32(time.Now().Unix()),
	}

	globalVar.MysteryStoreBuyRecords = append(globalVar.MysteryStoreBuyRecords, record)

	engine.Broadcast(chatdef.CIWorld, 0, 30, 14, &pb3.S2C_30_14{
		Log: record,
	}, 0)
}

// SendBuyRecord 发送全部
func (sys *MysteryStoreSys) GetBuyRecord() []*pb3.MysteryStoreBuyRecord {
	conf := jsondata.GetMysteryShopConf()

	globalVar := gshare.GetStaticVar()

	var res []*pb3.MysteryStoreBuyRecord

	var i uint32 = 0
	for _, line := range globalVar.MysteryStoreBuyRecords {

		if i >= conf.LogCount {
			break
		}

		res = append(res, &pb3.MysteryStoreBuyRecord{
			ActorName: line.ActorName,
			ItemId:    line.ItemId,
			Count:     line.Count,
			MoneyType: line.MoneyType,
			Money:     line.Money,
			ActorId:   line.ActorId,
			Time:      line.Time,
		})

		i++
	}
	return res
}

func init() {
	RegisterSysClass(sysdef.SiMysteryStoreSys, func() iface.ISystem {
		return &MysteryStoreSys{}
	})

	net.RegisterSysProto(30, 12, sysdef.SiMysteryStoreSys, (*MysteryStoreSys).c2sBuy)
	net.RegisterSysProto(30, 11, sysdef.SiMysteryStoreSys, (*MysteryStoreSys).c2sRefresh)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiMysteryStoreSys).(*MysteryStoreSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})
}
