package engine

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/iface"
)

type QuestTargetProgressHandle func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32

var (
	QTPMap = make(map[uint32]QuestTargetProgressHandle)
)

// 注册任务目标对应的累计进度
func RegQuestTargetProgress(qtt uint32, fn QuestTargetProgressHandle) {
	QTPMap[qtt] = fn
}

// 获取任务目标进度
func GetQuestTargetProgress(actor iface.IPlayer, qtt uint32, ids []uint32, args ...interface{}) (uint32, bool) {
	if fn, ok := QTPMap[qtt]; ok {
		return fn(actor, ids, args...), true
	}
	return 0, false
}

func init() {
	RegQuestTargetProgress(custom_id.QttLoginDays, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if info := player.GetPlayerData().MainData; nil != info {
			return info.LoginedDays
		}
		return 0
	})
	//累计充值
	RegQuestTargetProgress(custom_id.QttTotalRecharge, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if info := player.GetBinaryData(); nil != info {
			return info.GetChargeInfo().GetTotalChargeDiamond()
		}
		return 0
	})

	// 首充或次日登录
	RegQuestTargetProgress(custom_id.QttFirstOrNextDayLogin, func(player iface.IPlayer, idx []uint32, args ...interface{}) uint32 {
		if info := player.GetPlayerData().MainData; nil != info {
			if info.LoginedDays > 1 {
				return 1
			}
		}
		if player.GetVipLevel() > 0 {
			return 1
		}

		return 0
	})

	// 登录X天或购买指定赞助礼包
	RegQuestTargetProgress(custom_id.QttFirstOrBuyXSponsorGift, func(player iface.IPlayer, idx []uint32, args ...interface{}) uint32 {
		if len(idx) != 2 {
			return 0
		}
		day := idx[0]
		giftId := idx[1]
		if info := player.GetPlayerData().MainData; nil != info {
			if info.LoginedDays >= day {
				return 1
			}
		}

		gifts := player.GetBinaryData().SponsorGifts
		if gifts != nil {
			_, ok := gifts[giftId]
			if ok {
				return 1
			}
		}

		return 0
	})

	RegQuestTargetProgress(custom_id.QttTotalLoginDays, func(player iface.IPlayer, idx []uint32, args ...interface{}) uint32 {
		return 1
	})
}
