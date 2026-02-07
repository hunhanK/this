/**
 * @Author: zjj
 * @Date: 2024/8/1
 * @Desc: 玩法得分目标比拼
**/

package ranktype

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id/yydefine"
)

type GameplayScoreGoalRankType = uint16

const (
	GameplayScoreGoalRankTypeByKillDragonBoss = 1 // 玩法得分目标比拼 - 屠龙榜
)

func GetGameplayScoreGoalRankYYRankKey(typ GameplayScoreGoalRankType) uint32 {
	return utils.Make32(uint16(yydefine.YYGameplayScoreGoalRank), typ)
}
