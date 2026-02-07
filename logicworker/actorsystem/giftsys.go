package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

/*
	desc:礼包系统
	author: twl
*/

type (
	UnpackGiftArgs struct {
		GiftId  uint32
		Count   int64
		LogId   pb3.LogId
		AutoBuy bool
	}

	broadcastSt struct {
		tipMsgId uint32
		itemId   uint32
		count    int64
	}
)

var (
	broadcastVec = make([]*broadcastSt, 0, 100)
)

// GiftSys 礼包系统
type GiftSys struct {
	Base
}

// CheckGift 检查是否满足条件
func CheckGift(actor iface.IPlayer, conf *jsondata.StdReward) bool {
	if conf.Sex > 0 && actor.GetSex() != conf.Sex {
		return false
	}
	if conf.Job > 0 && actor.GetJob() != conf.Job {
		return false
	}
	return true
}

func (sys *GiftSys) OnReconnect() {}

// UnpackItem 拆随机礼包
func (sys *GiftSys) UnpackItem(st *miscitem.UseItemParamSt) (success, del bool, cnt int64) {
	success, del = false, false
	actor := sys.owner
	bagSys := actor.GetSysObj(sysdef.SiBag)
	if nil == bagSys {
		return
	}

	item := actor.GetItemByHandle(st.Handle)
	if nil == item {
		return
	}

	conf := jsondata.GetRandGiftConf(item.GetItemId())
	if nil == conf {
		return
	}

	itemConf := jsondata.GetItemConfig(item.GetItemId())
	if itemConf == nil {
		return
	}

	itemTips := make(map[uint32]int64)
	broadcast := make([]uint32, 0)
	vec := make([]*jsondata.StdReward, 0)
	addReward := func(conf *jsondata.RandGiftRewardConf) {
		vec = append(vec, &conf.StdReward)
		if conf.Broadcast > 0 {
			broadcast = append(broadcast, conf.Id)
		}
		if conf.TipMsg {
			itemTips[conf.Id] += conf.Count
		}
	}
	openDay := gshare.GetOpenServerDay()
	for group, line := range conf.Rewards {
		if group == 0 {
			for _, reward := range line {
				if openDay >= reward.OpenDay && CheckGift(actor, &reward.StdReward) {
					for i := st.Count; i > 0; i-- {
						addReward(reward)
					}
				}
			}
		} else {
			pool := new(random.Pool)
			for i := int64(0); i < st.Count; i++ {
				pool.Clear()
				for _, reward := range line {
					if reward.OpenDay > openDay {
						continue
					}
					if !CheckGift(actor, &reward.StdReward) {
						continue
					}
					pool.AddItem(reward, reward.Weight)
				}

				ret := pool.RandomOne()
				if reward, ok := ret.(*jsondata.RandGiftRewardConf); ok {
					addReward(reward)
				}
			}
		}
	}

	if flag := engine.CheckRewards(actor, vec); !flag {
		actor.SendTipMsg(tipmsgid.TpBagIsFull)
		return
	}

	engine.GiveRewards(actor, vec, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogUnPackGiftItem,
		BroadcastExt: []interface{}{itemConf.Name},
	})
	for itemId, count := range itemTips {
		curCount := actor.GetItemCount(itemId, -1)
		actor.SendTipMsg(tipmsgid.TpUseItem1, count, itemId, curCount, itemId)
	}

	return true, true, st.Count
}

// UnPackChoiceGift 拆选择礼包
func (sys *GiftSys) UnPackChoiceGift(handle uint64, index uint32, count uint32) bool {
	actor := sys.owner

	item := actor.GetItemByHandle(handle)
	if nil == item || int64(count) > item.Count {
		return false
	}

	// 拆数量不能超过 selectedGiftOpenMaxNum, 太大会炸
	selectedGiftOpenMaxNum := jsondata.GlobalUint("selectedGiftOpenMaxNum")
	if selectedGiftOpenMaxNum == 0 {
		return false
	}
	if count > selectedGiftOpenMaxNum {
		return false
	}

	itemId := item.GetItemId()

	conf := jsondata.GetChoiceGiftConf(itemId)
	if nil == conf {
		return false
	}

	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return false
	}

	vec := make([]*jsondata.StdReward, 0)
	if index <= 0 || int(index) > len(conf.Rewards) {
		return false
	}

	consume := make([]*jsondata.Consume, 0)
	consume = append(consume, &jsondata.Consume{
		Id:    itemId,
		Count: count,
	})
	if !actor.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUnPackGiftItem}) {
		return false
	}

	var broadcast []uint32
	for i := 1; i <= int(count); i++ {
		reward := conf.Rewards[index-1]
		if !CheckGift(actor, &reward.StdReward) {
			return false
		}
		vec = append(vec, &jsondata.StdReward{
			Id:    reward.Id,
			Count: reward.Count,
			Bind:  reward.Bind,
		})
		if reward.Broadcast {
			broadcast = append(broadcast, reward.Id)
		}
	}

	if flag := engine.CheckRewards(actor, vec); !flag {
		actor.SendTipMsg(tipmsgid.TpBagIsFull)
	}

	engine.GiveRewards(actor, vec, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogUnPackGiftItem,
		BroadcastExt: []interface{}{itemConf.Name},
	})
	return true
}

func (sys *GiftSys) GetGiftUsedTimes(itemId uint32) uint32 {
	if sys.GetBinaryData().GiftUseTimes == nil {
		return 0
	}
	return sys.GetBinaryData().GiftUseTimes[itemId]
}

func (sys *GiftSys) AddGiftUsedTimes(itemId, count uint32) {
	if sys.GetBinaryData().GiftUseTimes == nil {
		sys.GetBinaryData().GiftUseTimes = make(map[uint32]uint32)
	}
	sys.GetBinaryData().GiftUseTimes[itemId] += count
}

// 开礼包
func (sys *GiftSys) c2sUnpackGift(msg *base.Message) {
	var req pb3.C2S_2_41
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return
	}
	player := sys.owner
	itemSt := player.GetItemByHandle(req.Id)
	if itemSt == nil || int64(req.Count) > itemSt.Count {
		return
	}
	isSuccess := sys.UnPackChoiceGift(req.Id, req.Index, req.Count)
	player.SendProto3(2, 41, &pb3.S2C_2_41{
		IsSucess: isSuccess,
		Id:       req.Id,
	})
}

// UnpackGift 开礼包
func UnpackGift(player iface.IPlayer, args UnpackGiftArgs) int32 {
	giftId := args.GiftId
	conf := jsondata.GetGiftConf(giftId)
	if nil == conf {
		return custom_id.NoConfig
	}

	if args.LogId == 0 {
		args.LogId = pb3.LogId(conf.LogId)
	}

	unpackCount := args.Count

	itemTips := make(map[uint32]int64)
	broadcastVec = broadcastVec[:0]
	vec := make([]*jsondata.StdReward, 0)
	addReward := func(conf *jsondata.RandGiftRewardConf) {
		vec = append(vec, &conf.StdReward)
		if conf.Broadcast > 0 {
			broadcastVec = append(broadcastVec, &broadcastSt{
				tipMsgId: conf.Broadcast,
				itemId:   conf.Id,
				count:    conf.Count,
			})
		}
		if conf.TipMsg {
			itemTips[conf.Id] += conf.Count
		}
	}
	openDay := gshare.GetOpenServerDay()
	for group, line := range conf.Rewards {
		if group == 0 {
			for _, reward := range line {
				if openDay >= reward.OpenDay && CheckGift(player, &reward.StdReward) {
					for i := unpackCount; i > 0; i-- {
						addReward(reward)
					}
				}
			}
		} else {
			pool := new(random.Pool)
			for i := int64(0); i < unpackCount; i++ {
				pool.Clear()
				for _, reward := range line {
					if reward.OpenDay > openDay {
						continue
					}
					if !CheckGift(player, &reward.StdReward) {
						continue
					}
					pool.AddItem(reward, reward.Weight)
				}

				ret := pool.RandomOne()
				if reward, ok := ret.(*jsondata.RandGiftRewardConf); ok {
					addReward(reward)
				}
			}
		}
	}

	if flag := engine.CheckRewards(player, vec); !flag {
		player.SendTipMsg(tipmsgid.TpBagIsFull)
		return custom_id.BagNotEnoughGrid
	}

	logId := pb3.LogId_LogUnPackGiftItem
	if args.LogId > 0 {
		logId = args.LogId
	}

	if !player.ConsumeRate(conf.Consume, unpackCount, args.AutoBuy, common.ConsumeParams{LogId: logId}) {
		return custom_id.ConsumeFailed
	}

	engine.GiveRewards(player, vec, common.EngineGiveRewardParam{LogId: logId})
	for _, st := range broadcastVec {
		engine.BroadcastTipMsgById(st.tipMsgId, player.GetName(), st.itemId, st.count)
	}
	return custom_id.Success
}

func gmUnPackGift(player iface.IPlayer, args ...string) bool {
	if len(args) < 2 {
		return false
	}
	id := utils.AtoUint32(args[0])
	count := utils.AtoInt64(args[1])
	err := UnpackGift(player, UnpackGiftArgs{
		GiftId: id,
		Count:  count,
	})
	return err == custom_id.Success
}

// 使用礼包
func useItemGift(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	giftSys := player.GetSysObj(sysdef.SiGiftSys).(*GiftSys)
	return giftSys.UnpackItem(param)
}

// 使用龙珠礼包
func useDragonBallGift(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	// 不支持批量使用
	if param.Count > 1 {
		return false, false, 0
	}

	giftSys := player.GetSysObj(sysdef.SiGiftSys).(*GiftSys)
	if giftSys == nil {
		return false, false, 0
	}

	usedTimes := giftSys.GetGiftUsedTimes(param.ItemId)
	tConf := jsondata.GetGiftUsedTimesConf(param.ItemId, usedTimes+1)
	if tConf == nil {
		return false, false, 0
	}

	giftSys.AddGiftUsedTimes(param.ItemId, 1)
	engine.GiveRewards(player, jsondata.StdRewardVec{&jsondata.StdReward{
		Id:    tConf.GiftId,
		Count: param.Count,
	}}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogUnPackGiftItem})

	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiGiftSys, func() iface.ISystem {
		return &GiftSys{}
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemGift, useItemGift)
	miscitem.RegCommonUseItemHandle(itemdef.UseDragonBallGift, useDragonBallGift)

	net.RegisterSysProto(2, 41, sysdef.SiGiftSys, (*GiftSys).c2sUnpackGift)
	gmevent.Register("gift", gmUnPackGift, 1)

	engine.RegRewardsBroadcastHandler(tipmsgid.TpGiftBroadcast, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，礼包名称，道具
	})
}
