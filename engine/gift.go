package engine

import (
	"github.com/gzjjyz/random"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
)

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

	if flag := CheckRewards(player, vec); !flag {
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

	GiveRewards(player, vec, common.EngineGiveRewardParam{LogId: logId})
	for _, st := range broadcastVec {
		BroadcastTipMsgById(st.tipMsgId, player.GetId(), player.GetName(), st.itemId, st.count)
	}
	return custom_id.Success
}
