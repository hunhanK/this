/**
 * @Author: zjj
 * @Date: 2024/6/20
 * @Desc: 全服运营活动-玩法得分目标比拼
**/

package manager

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/ranktype"
)

func UpdateGameplayScoreGoalRank(rankType ranktype.GameplayScoreGoalRankType, player iface.IPlayer, score int64, isAdd bool) {
	event.TriggerSysEvent(custom_id.SeGameplayScoreGoalRankUpdate, rankType, player.GetId(), score, isAdd)
}
