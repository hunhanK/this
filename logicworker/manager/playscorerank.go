/**
 * @Author: zjj
 * @Date: 2024/6/20
 * @Desc: 全服运营活动-玩法分数比拼排行榜
**/

package manager

import (
	"fmt"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/ranktype"
)

type playScoreExtValueGetter func(player iface.IPlayer) *pb3.YYFightValueRushRankExt

var playScoreExtValueGetterMgr = make(map[uint32]playScoreExtValueGetter)

func PacketPlayScoreRankPlayerInfo(playerId uint64, rankInfo *pb3.RankInfo) {
	packetPlayerInfoToRankInfo(playerId, rankInfo)
}

func UpdatePlayScoreRank(rt ranktype.PlayScoreRankType, player iface.IPlayer, score int64, isAdd bool, exScore int64) {
	event.TriggerSysEvent(custom_id.SePlayScoreRankUpdate, rt, player.GetId(), score, isAdd, exScore)
	updatePlayScoreRankUpdateExtVal(rt, player)
}

func updatePlayScoreRankUpdateExtVal(rt ranktype.PlayScoreRankType, player iface.IPlayer) {
	getter, ok := playScoreExtValueGetterMgr[uint32(rt)]
	if !ok {
		return
	}
	commonSt := getter(player)
	if commonSt == nil {
		return
	}
	event.TriggerSysEvent(custom_id.SePlayScoreRankUpdateExtVal, rt, player.GetId(), commonSt)
}

func RegPlayScoreExtValueGetter(typ ranktype.PlayScoreRankType, f playScoreExtValueGetter) {
	_, ok := playScoreExtValueGetterMgr[uint32(typ)]
	if ok {
		panic(fmt.Sprintf("already registered %d", typ))
	}
	playScoreExtValueGetterMgr[uint32(typ)] = f
}

func init() {
	event.RegActorEvent(custom_id.AeConsumeMoney, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 2 {
			return
		}
		mt, ok := args[0].(uint32)
		if !ok {
			return
		}
		count, ok := args[1].(int64)
		if !ok {
			return
		}
		if mt != moneydef.Diamonds {
			return
		}
		UpdatePlayScoreRank(ranktype.PlayScoreRankTypeConsumeDiamonds, player, count, true, 0)
	})
}
