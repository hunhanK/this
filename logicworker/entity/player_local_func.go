/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/25 17:42
 */

package entity

import (
	"github.com/gzjjyz/logger"
	"golang.org/x/exp/slices"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"time"
)

// onActorCallAddPlayer 加经验
func onActorCallAddPlayer(actor iface.IPlayer, buf []byte) {
	var st pb3.FActorAddExpSt
	if err := pb3.Unmarshal(buf, &st); nil != err {
		logger.LogError("onActorCallAddPlayer error:%v", err)
		return
	}
	actor.AddExp(int64(st.GetExp()), pb3.LogId(st.GetLogId()), st.WithWorldAddRate, st.AddRate)
}

func onCheckLootUserItem(actor iface.IPlayer, buf []byte) {
	proxy := actor.GetActorProxy()
	if nil == proxy {
		return
	}

	var req pb3.FActorCheckLootUserItemSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onCheckLootItem error:%v", err)
		return
	}

	// 检查元神碎片是否可以拾取
	if !actor.CheckBossSpirit(req.ItemId) {
		return
	}

	availableCount, tipId := getAvailableCountAndTipId(actor, req.ItemId)
	if availableCount <= 0 {
		actor.SendTipMsg(tipId)
		return
	}
	err := proxy.CallActorFunc(actorfuncid.SureLootUserItem, &req)
	if err != nil {
		actor.LogError("err:%v", err)
	}
}

func onCheckLootItem(actor iface.IPlayer, buf []byte) {
	proxy := actor.GetActorProxy()
	if nil == proxy {
		return
	}

	var req pb3.FActorLootItemSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onCheckLootItem error:%v", err)
		return
	}
	st := &itemdef.ItemParamSt{}
	st.ItemId = req.GetItemId()
	st.Count = int64(req.GetCount())
	st.Bind = req.GetBind()
	if !actor.CanAddItem(st, true) {
		_, tipId := getAvailableCountAndTipId(actor, st.ItemId)
		actor.SendTipMsg(tipId)
		return
	}

	// 检查元神碎片是否可以拾取
	if !actor.CheckBossSpirit(req.ItemId) {
		return
	}

	err := proxy.CallActorFunc(actorfuncid.SureLootItem, &pb3.FActorCheckLootItemSt{
		Hdl: req.DropHdl})
	if err != nil {
		actor.LogError("err:%v", err)
	}
}

func onLootItem(actor iface.IPlayer, buf []byte) {
	proxy := actor.GetActorProxy()
	if nil == proxy {
		return
	}

	var req pb3.FActorLootItemSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onCheckLootItem error:%v", err)
		return
	}

	itemId := req.ItemId
	conf := jsondata.GetItemConfig(itemId)
	if nil == conf {
		return
	}

	st := itemdef.ItemParamSt{
		ItemId:  req.ItemId,
		Count:   int64(req.Count),
		Bind:    req.Bind,
		LogId:   pb3.LogId_LogLootItem,
		Quality: req.Quality,
	}

	rewards := []*jsondata.StdReward{
		{
			Id:      req.GetItemId(),
			Count:   int64(req.GetCount()),
			Bind:    req.GetBind(),
			Quality: req.Quality,
		},
	}
	engine.GiveRewards(actor, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLootItem})

	actor.TriggerEvent(custom_id.AeLootItem, st, req.FbId)
}

func onLootUserItem(actor iface.IPlayer, buf []byte) {
	proxy := actor.GetActorProxy()
	if nil == proxy {
		return
	}

	var req pb3.FActorLootUserItemSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}
	if nil == req.Item {
		return
	}
	if actor.GetBagAvailableCount() > 0 {
		actor.AddItemPtr(req.Item, true, pb3.LogId_LogLootItem)
	} else {
		mailmgr.SendMailToActor(actor.GetId(), &mailargs.SendMailSt{
			ConfId: common.Mail_BagInsufficient,
			UserItems: []*pb3.ItemSt{
				req.Item,
			},
		})
	}
}

func onLootItems(actor iface.IPlayer, buf []byte) {
	proxy := actor.GetActorProxy()
	if nil == proxy {
		return
	}

	var req pb3.FActorLootItems
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	var normalBagItemList, godBeastBagItemList []*pb3.FActorLootItemSt
	var normalBagUserItems, godBeastBagUserItems []*pb3.ItemSt
	for _, item := range req.Items {
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if itemConf != nil && itemdef.IsGodBeastBagItem(itemConf.Type) {
			godBeastBagItemList = append(godBeastBagItemList, item)
			continue
		}
		normalBagItemList = append(normalBagItemList, item)
	}
	for _, item := range req.UserItems {
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if itemConf != nil && itemdef.IsGodBeastBagItem(itemConf.Type) {
			godBeastBagUserItems = append(godBeastBagUserItems, item)
			continue
		}
		normalBagUserItems = append(normalBagUserItems, item)
	}

	lootItemsByBag(actor, actor.GetBagAvailableCount(), normalBagItemList, normalBagUserItems)
	lootItemsByBag(actor, actor.GetGodBeastBagAvailableCount(), godBeastBagItemList, godBeastBagUserItems)
}

func lootItemsByBag(actor iface.IPlayer, availableCount uint32, itemList []*pb3.FActorLootItemSt, userItems []*pb3.ItemSt) {
	needCount := uint32(len(itemList) + len(userItems))
	if needCount == 0 {
		return
	}

	lootItemToReward := func(item *pb3.FActorLootItemSt) *jsondata.StdReward {
		return &jsondata.StdReward{
			Id:      item.ItemId,
			Count:   int64(item.Count),
			Bind:    item.Bind,
			Quality: item.Quality,
		}
	}
	items := functional.Map(itemList, lootItemToReward)

	if needCount >= availableCount {
		mailmgr.SendMailToActor(actor.GetId(), &mailargs.SendMailSt{
			ConfId:    common.Mail_BagInsufficient,
			Rewards:   items,
			UserItems: userItems,
		})
		return
	} else {
		if len(items) > 0 {
			engine.GiveRewards(actor, items, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLootItem})
		}

		for idx, item := range items {
			st := itemdef.ItemParamSt{
				ItemId:  item.Id,
				Count:   item.Count,
				Bind:    item.Bind,
				LogId:   pb3.LogId_LogLootItem,
				Quality: item.Quality,
			}
			actor.TriggerEvent(custom_id.AeLootItem, st, itemList[idx].FbId)
		}

		for _, item := range userItems {
			actor.AddItemPtr(item, true, pb3.LogId_LogLootItem)
		}
	}

}

func onDropUserItem(actor iface.IPlayer, buf []byte) {
	var req pb3.FActorDropItemSt
	if nil != pb3.Unmarshal(buf, &req) {
		return
	}

	item := actor.GetItemByHandle(req.GetHdl())
	if nil == item {
		return
	}
	if base.CheckItemFlag(item.GetItemId(), itemdef.DenyDrop) {
		return
	}

	if actorsystem.CheckTimeOut(item) {
		return
	}

	ret := actor.DeleteItemPtr(item, item.GetCount(), pb3.LogId_LogDropItem)
	if ret && !item.GetBind() {
		proxy := actor.GetActorProxy()
		if nil == proxy {
			return
		}

		proxy.CallActorFunc(actorfuncid.DropUserItem, &pb3.FActorDropItemSt{
			Item: item,
		})
	}
}

func OnTransferCheck(actor iface.IPlayer, buf []byte) {
	if len(buf) < 0 {
		return
	}
	var req pb3.ProtoByteArray
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTransferCheck error1: %v", err)
		return
	}

	var realReq pb3.C2S_128_12
	if err := pb3.Unmarshal(req.Buff, &realReq); nil != err {
		logger.LogError("onTransferCheck error: %v", err)
		return
	}
	conf := jsondata.GetTransferConfig(realReq.TransferId)
	if nil == conf {
		return
	}
	if len(conf.Consume) > 0 {
		if !actor.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogTransferConsume}) {
			return
		}
	}
	actor.CallActorFunc(actorfuncid.TransferCheck, &req)
}

func onAddMoney(actor iface.IPlayer, buf []byte) {
	msg := &pb3.FActorAddMoney{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onAddMoney error:%v", err)
		return
	}

	actor.AddMoney(msg.Type, msg.Count, true, pb3.LogId(msg.LogId))
}

func onDeleteItem(actor iface.IPlayer, buf []byte) {
	msg := &pb3.FDeleteItemInBag{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onDeleteItem error:%v", err)
		return
	}

	actor.DeleteItemById(msg.ItemId, msg.Count, pb3.LogId(msg.LogId))
}

func onReliveConsume(player iface.IPlayer, buf []byte) {
	bagSys := player.(*Player).GetBagSys()
	if nil == bagSys {
		return
	}
	var itemId uint32
	vec := jsondata.GlobalU32Vec("reviveCost")
	size := len(vec)
	if size >= 1 {
		itemId = vec[0]
	} else {
		return
	}
	if bagSys.GetItemCount(itemId, -1) <= 0 {
		return
	}
	bagSys.DeleteItem(&itemdef.ItemParamSt{ItemId: itemId, Count: 1, LogId: pb3.LogId_LogReliveConsume})
	player.CallActorFunc(actorfuncid.ReliveConsumeRet, nil)
}

func onDropInfo(actor iface.IPlayer, buf []byte) {
	var req = &pb3.DropInfos{}
	if err := pb3.Unmarshal(buf, req); err != nil {
		return
	}
	actor.TriggerEvent(custom_id.AeDropInfo, req)

	logDropInfo(actor, req)
}

func logDropInfo(player iface.IPlayer, info *pb3.DropInfos) {
	if len(info.DropList) == 0 {
		return
	}
	conf := jsondata.GetMonsterConf(info.MonsterId)
	if conf == nil || conf.Type != custom_id.MtBoss {
		return
	}
	st := pb3.LogBossDrop{
		BossType:  conf.SubType,
		BossId:    info.MonsterId,
		BossLevel: info.Level,
		Timestamp: uint32(time.Now().Unix()),
		Equips:    make(map[uint32]uint32),
		Items:     make(map[uint32]uint32),
	}
	if sceneConf := jsondata.GetSceneConf(info.SceneId); nil != sceneConf {
		st.Scene = sceneConf.Name
	}

	for _, drop := range info.DropList {
		if itemConf := jsondata.GetItemConfig(drop.ItemId); nil != itemConf {
			if slices.Contains(jsondata.CommonStConfMgr.EquipTypes, itemConf.Type) {
				st.Equips[drop.ItemId] += drop.Count
			} else {
				st.Items[drop.ItemId] += drop.Count
			}
		}
	}

	logworker.LogDrop(player, &st)
}

func EnterActivity(actor iface.IPlayer, buf []byte) {
	//var st pb3.EnterActivityInfo
	//if err := pb3.Unmarshal(buf, &st); nil != err {
	//	logger.LogError("onEnterActivity error:%v", err)
	//	return
	//}
	//
	//logworker.LogActivity(actor, &pb.LogActivity{
	//	Counter: proto.String("enterActivity"),
	//	Value:   proto.Int32(int32(st.ActId)),
	//})
}

func syncStatus(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if err := pb3.Unmarshal(buf, &st); nil != err {
		return
	}

	p, ok := player.(*Player)
	if !ok {
		return
	}

	if st.BParam {
		p.StateBitSet.Set(st.U32Param)
	} else {
		p.StateBitSet.Unset(st.U32Param)
	}
}

func onActorTriggerEvent(player iface.IPlayer, buf []byte) {
	var req pb3.FTriggerPlayerEventSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTriggerPlayerEvent error:%v", err)
		return
	}

	eventId := int(req.PlayerEventId)
	switch eventId {
	case custom_id.AeKillMon, custom_id.AeVestKillMon:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_KillMonArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_KillMonArg")
			return
		}
		player.TriggerEvent(eventId,
			arg.KillMonArg.MonsterId,
			arg.KillMonArg.SceneId,
			arg.KillMonArg.Count,
			arg.KillMonArg.FbId,
			arg.KillMonArg.MonterLv)

	case custom_id.AeKillByActor:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_KillByActorArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_KillByActorArg")
			return
		}
		player.TriggerEvent(eventId, arg.KillByActorArg.OtherActorId, arg.KillByActorArg.SceneId)
	case custom_id.AeKillOtherActor:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_KillOtherActorArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_KillOtherActorArg")
			return
		}
		player.TriggerEvent(eventId, arg.KillOtherActorArg.OtherActorId, arg.KillOtherActorArg.SceneId, arg.KillOtherActorArg.Camp)
	case custom_id.AePassFb:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_PassFbArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_PassFbArg")
			return
		}
		player.TriggerEvent(eventId, arg.PassFbArg.FbId, arg.PassFbArg.Count)
	case custom_id.AeGather:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_GatherArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_GatherArg")
			return
		}
		player.TriggerEvent(eventId, arg.GatherArg.GatherId)
	case custom_id.AeEnterFb:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_EnterFbArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_EnterFbArg")
			return
		}
		player.TriggerEvent(eventId, arg.EnterFbArg.FbId)

	case custom_id.AeRespondHelp:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_RespondHelpArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_RespondHelpArg")
			return
		}
		player.TriggerEvent(eventId, arg.RespondHelpArg.OtherActorId)
	case custom_id.AeAttackedByActor /*, common.AeDeath*/ :
		_, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_AttackedByActorArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_AttackedByActorArg")
			return
		}

		player.TriggerEvent(eventId)
	case custom_id.AePassMirrorFb:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_PassMirrorFbArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_PassMirrorFbArg")
			return
		}
		player.TriggerEvent(eventId, arg.PassMirrorFbArg.MirrorId)
	case custom_id.AeParticipateNiEnBeast:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_NiEnBeastArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_NiEnBeastArg")
			return
		}
		player.TriggerEvent(eventId, arg.NiEnBeastArg.MonsterId)
	case custom_id.AeParticipateGroupOfNiEnBeast:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_GroupOfNiEnBeastArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_GroupOfNiEnBeastArg")
			return
		}
		player.TriggerEvent(eventId, arg.GroupOfNiEnBeastArg.MonsterId)
	case custom_id.AeDailyMissionComplete:
		arg, ok := req.Arg.(*pb3.FTriggerPlayerEventSt_DailyMissionArg)
		if !ok {
			logger.LogError("onTriggerPlayerEvent failed to convert arg to *pb3.FTriggerPlayerEventSt_PassMirrorFbArg")
			return
		}
		player.TriggerEvent(eventId, arg.DailyMissionArg.TaskId)
	case custom_id.AeJoinGodAreaRaceAct, custom_id.AePassFbByHelper:
		player.TriggerEvent(eventId, nil)
	default:
		logger.LogError("战斗服调用玩家事件 %d", eventId)
	}
}

func onSyncDropTimes(et iface.IPlayer, buf []byte) {
	msg := &pb3.PlayerDropData{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	player, ok := et.(*Player)
	if !ok {
		return
	}
	binaryData := player.GetBinaryData()
	binaryData.DropData = msg
}

func getAvailableCountAndTipId(actor iface.IPlayer, itemId uint32) (uint32, uint32) {
	// 默认取正常背包
	availableCount := actor.GetBagAvailableCount()
	tipId := tipmsgid.BagIsFullLootFail
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf != nil && itemdef.IsGodBeastBagItem(itemConf.Type) {
		availableCount = actor.GetGodBeastBagAvailableCount()
		tipId = tipmsgid.TpGodBeastBagIsFull
	}
	return availableCount, uint32(tipId)
}

func init() {
	engine.RegisterActorCallFunc(playerfuncid.ActorAddExp, onActorCallAddPlayer)
	engine.RegisterActorCallFunc(playerfuncid.DropUserItem, onDropUserItem)
	engine.RegisterActorCallFunc(playerfuncid.CheckLootUserItem, onCheckLootUserItem)
	engine.RegisterActorCallFunc(playerfuncid.CheckLootItem, onCheckLootItem)
	engine.RegisterActorCallFunc(playerfuncid.LootItem, onLootItem)
	engine.RegisterActorCallFunc(playerfuncid.LootUserItem, onLootUserItem)
	engine.RegisterActorCallFunc(playerfuncid.LootItems, onLootItems)
	engine.RegisterActorCallFunc(playerfuncid.TransferCheck, OnTransferCheck)
	engine.RegisterActorCallFunc(playerfuncid.AddMoney, onAddMoney)
	engine.RegisterActorCallFunc(playerfuncid.DeleteItem, onDeleteItem)
	engine.RegisterActorCallFunc(playerfuncid.ReliveConsume, onReliveConsume)
	engine.RegisterActorCallFunc(playerfuncid.SyncDropInfo, onDropInfo)
	engine.RegisterActorCallFunc(playerfuncid.PlayerEnterActivity, EnterActivity)
	engine.RegisterActorCallFunc(playerfuncid.SyncStatus, syncStatus)
	engine.RegisterActorCallFunc(playerfuncid.TriggerEvent, onActorTriggerEvent)
	engine.RegisterActorCallFunc(playerfuncid.SyncDropTimes, onSyncDropTimes)
}
