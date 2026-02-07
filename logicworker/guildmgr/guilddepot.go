package guildmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/iface"
)

func (guild *Guild) initDepot() {
	container := miscitem.NewContainer(&guild.DepotItems)
	container.DefaultSizeHandle = guild.DefaultSize
	container.OnAddNewItem = guild.OnAddNewItem
	container.OnItemChange = guild.OnItemChange
	container.OnRemoveItem = guild.OnRemoveItem
	guild.Container = container
}

func (guild *Guild) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	guild.BroadcastProto(29, 50, &pb3.S2C_29_50{Item: item})
}

func (guild *Guild) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	guild.BroadcastProto(29, 50, &pb3.S2C_29_50{Item: item})
}

func (guild *Guild) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	guild.BroadcastProto(29, 51, &pb3.S2C_29_51{Hdl: item.GetHandle()})
}

func (guild *Guild) DefaultSize() uint32 {
	conf := jsondata.GetGuildConf()
	if nil == conf || nil == conf.Upgrade {
		return 0
	}
	if upConf := conf.Upgrade[guild.GetLevel()]; nil != upConf {
		return upConf.DepotNum
	}
	return 0
}

func (guild *Guild) checkItemPriory(itemId1 uint32, itemId2 uint32) bool {
	itemConf1 := jsondata.GetItemConfig(itemId1)
	itemConf2 := jsondata.GetItemConfig(itemId2)
	if itemConf1.Stage < itemConf2.Stage {
		return true
	}
	if itemConf1.Quality < itemConf2.Quality {
		return true
	}
	return itemConf1.Star < itemConf2.Star
}

func (guild *Guild) AutoDestroy(item *pb3.ItemSt) {
	var dIdx int
	length := len(guild.DepotItems)
	for i := 1; i < length; i++ {
		item1 := guild.DepotItems[dIdx]
		item2 := guild.DepotItems[i]
		if !guild.checkItemPriory(item1.GetItemId(), item2.GetItemId()) {
			dIdx = i
		}
	}
	dItem := guild.DepotItems[dIdx]
	dItemId := dItem.GetItemId()
	if guild.checkItemPriory(dItem.GetItemId(), item.GetItemId()) {
		guild.RemoveItemByHandle(dItem.GetHandle(), pb3.LogId_LogGuildDepotDonate)
		guild.AddItemPtr(item, false, pb3.LogId_LogGuildDepotDonate)
	} else {
		dItemId = item.GetItemId()
	}
	guild.AddEvent(custom_id.GuildEvent_DepotDestroyAuto, dItemId)
	guild.SendTip(tipmsgid.GuildDepotNoticeAutoDestroy, dItemId)

	guild.SetSaveFlag()
}

// 捐献
func (guild *Guild) Donate(actor iface.IPlayer, hdl uint64) bool {
	if nil == actor {
		return false
	}
	item := actor.GetItemByHandle(hdl)
	if nil == item || item.GetBind() {
		return false
	}
	itemId := item.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return false
	}

	if !base.CheckItemFlag(itemId, itemdef.CanContrib) {
		return false
	}
	stConf := itemConf.GetItemSettingConf()
	if stConf == nil || nil == stConf.GuildDepot {
		return false
	}

	actor.RemoveItemByHandle(hdl, pb3.LogId_LogGuildDepotDonate)

	if stConf.GuildDepot.DonateScore > 0 {
		actor.AddMoney(moneydef.GuildDepotScore, int64(stConf.GuildDepot.DonateScore), true, pb3.LogId_LogGuildDepotDonate)
	}

	guild.AddEvent(custom_id.GuildEvent_DepotDonate, actor.GetName(), item.GetItemId())
	guild.SendTip(tipmsgid.GuildDepotNoticePut, actor.GetName(), item.GetItemId())

	if guild.AvailableCount() <= 0 {
		guild.AutoDestroy(item)
	} else {
		guild.AddItemPtr(item, false, pb3.LogId_LogGuildDepotDonate)
	}
	guild.SetSaveFlag()
	return true
}

func (guild *Guild) SendTip(tipMsgId uint32, params ...interface{}) {
	guild.BroadcastProto(5, 0, common.PackMsg(tipMsgId, params...))
}

func (guild *Guild) Exchange(actor iface.IPlayer, hdl uint64) {
	item := guild.FindItemByHandle(hdl)
	if nil == item {
		return
	}
	if actor.GetBagAvailableCount() <= 0 {
		actor.SendTipMsg(tipmsgid.TpBagIsFull)
		return
	}

	itemConf := jsondata.GetItemConfig(item.GetItemId())
	if nil == itemConf {
		return
	}
	stConf := itemConf.GetItemSettingConf()
	if nil == stConf.GuildDepot {
		return
	}
	if stConf.GuildDepot.GuildDepotCircle > actor.GetCircle() {
		actor.SendTipMsg(tipmsgid.TpBoundary)
		return
	}
	if itemConf.Circle > 0 && itemConf.Circle > actor.GetCircle() {
		actor.SendTipMsg(tipmsgid.TpBoundary)
		return
	}

	if itemConf.NirvanaLevel > 0 && itemConf.NirvanaLevel > actor.GetNirvanaLevel() {
		actor.SendTipMsg(tipmsgid.CircleNotEnough)
		return
	}

	nHdl, err := series.AllocSeries()
	if err != nil {
		logger.LogError(err.Error())
		return
	}

	score := int64(stConf.GuildDepot.GuildDepotScore)
	if score > 0 {
		if !actor.DeductMoney(moneydef.GuildDepotScore, score, common.ConsumeParams{LogId: pb3.LogId_LogTakeGuildDepotItem}) {
			actor.SendTipMsg(tipmsgid.TpItemNotEnough)
			return
		}
	}

	guild.RemoveItemByHandle(hdl, pb3.LogId_LogTakeGuildDepotItem)
	item.Handle = nHdl
	actor.AddItemPtr(item, true, pb3.LogId_LogTakeGuildDepotItem)
	guild.AddEvent(custom_id.GuildEvent_DepotExchange, actor.GetName(), item.GetItemId())
	guild.SendTip(tipmsgid.GuildDepotNoticeOut, actor.GetName(), item.GetItemId())

	guild.SetSaveFlag()
}

// 手动销毁
func (guild *Guild) DestroyDepotItem(actor iface.IPlayer, hdl uint64) {
	item := guild.FindItemByHandle(hdl)
	if nil == item {
		return
	}
	itemConf := jsondata.GetItemConfig(item.GetItemId())
	if nil == itemConf {
		return
	}
	guild.RemoveItemByHandle(hdl, pb3.LogId_LogTakeGuildDepotItem)

	guild.AddEvent(custom_id.GuildEvent_DepotDestory, actor.GetName(), item.GetItemId())
	guild.SendTip(tipmsgid.GuildDepotNoticeDestroy, actor.GetName(), item.GetItemId())

	guild.SetSaveFlag()
}
