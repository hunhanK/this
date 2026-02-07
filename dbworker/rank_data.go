/**
 * @Author: ChenJunJi
 * @Desc: 排行榜信息
 * @Date: 2021/9/23 14:10
 */

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

// 加载排行榜

func checkRankPlayer(args ...interface{}) {
	if !gcommon.CheckArgsCount("checkRankPlayer", 2, len(args)) {
		return
	}

	rt, ok := args[0].(int)
	if !ok {
		return
	}
	actorIds, ok := args[1].([]uint64)
	if !ok {
		return
	}

	retActorIds := make([]uint64, 0)
	for _, id := range actorIds {
		ret, err := db.OrmEngine.QueryString("call checkPlayerStatus(?)", id)
		if nil != err {
			logger.LogError("loadRankList error. %v", err)
			return
		} else {
			for _, line := range ret {
				status := utils.AtoUint32(line["status"])
				if status == 1 {
					retActorIds = append(retActorIds, id)
				}
			}
		}
	}

	if len(retActorIds) > 0 {
		gshare.SendGameMsg(custom_id.GMsgCheckRankRet, rt, retActorIds)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		//gshare.RegisterDBMsgHandler(custom_id.GMsgLoadRankData, loadRankList)
		//gshare.RegisterDBMsgHandler(custom_id.GMsgSaveRankData, saveRankList)
		gshare.RegisterDBMsgHandler(custom_id.GMsgCheckRank, checkRankPlayer)
	})
}
