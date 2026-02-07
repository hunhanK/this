/**
 * @Author: HeXinLi
 * @Desc: 日清全服充值统计表
 * @Date: 2021/11/8 11:23
 */

package engine

import (
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

func onPlayerCharge(player iface.IPlayer, diamond int64, cashCent uint32) {
	playerId := player.GetId()
	globalVar := gshare.GetStaticVar()

	if nil == globalVar.DailyChargeCash {
		globalVar.DailyChargeCash = make(map[uint64]int64)
	}
	globalVar.DailyChargeCash[playerId] += int64(cashCent)

	// 钻石
	if nil == globalVar.DailyChargeDiamond {
		globalVar.DailyChargeDiamond = make(map[uint64]int64)
	}
	if _, ok := globalVar.DailyChargeDiamond[playerId]; !ok {
		globalVar.DailyChargeDiamond[playerId] = 0
	}
	globalVar.DailyChargeDiamond[playerId] += diamond
}

func onPlayerUseDiamond(player iface.IPlayer, diamond int64) {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.DailyUseDiamond {
		globalVar.DailyUseDiamond = make(map[uint64]int64)
	}

	playerId := player.GetId()

	// 消耗钻石和绑钻
	if _, ok := globalVar.DailyUseDiamond[playerId]; !ok {
		globalVar.DailyUseDiamond[playerId] = 0
	}
	globalVar.DailyUseDiamond[playerId] += diamond

	player.SendProto3(2, 3, &pb3.S2C_2_3{
		TodayUseDiamond: globalVar.DailyUseDiamond[playerId],
	})
}

// ClearChargeMap 切天的其他函数跑完，再跑这个
func ClearChargeMap() {
	globalVar := gshare.GetStaticVar()
	globalVar.DailyChargeCash = nil
	globalVar.DailyChargeDiamond = nil
	globalVar.DailyUseDiamond = nil
}

func init() {
	// 玩家充值，把金额记下来
	event.RegActorEventH(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
		if !ok {
			return
		}
		onPlayerCharge(player, int64(chargeEvent.Diamond), chargeEvent.CashCent)
	})

	// 玩家使用钻石
	event.RegActorEventH(custom_id.AeUseDiamond, func(player iface.IPlayer, args ...interface{}) {
		diamond := args[0].(int64)
		onPlayerUseDiamond(player, diamond)
	})

	event.RegActorEventH(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		player.SendProto3(2, 3, &pb3.S2C_2_3{
			TodayUseDiamond: 0,
		})
	})
}
