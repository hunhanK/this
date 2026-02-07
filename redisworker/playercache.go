/**
 * @Author: zjj
 * @Date: 2025/1/8
 * @Desc:
**/

package redisworker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/pb3"
	"jjyz/base/rediskey"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/redisworker/redismid"
	"time"
)

func onSaveActorCache(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	actorId := args[0].(uint64)
	pb3DataBuf := args[1].([]byte)
	key := fmt.Sprintf(rediskey.GameActorCache, engine.GetPfId(), engine.GetServerId())
	err := client.HSet(context.Background(), key, actorId, pb3DataBuf).Err()
	if err != nil {
		logger.LogError("actor[%d] err:%v", actorId, err)
		return
	}
	client.Expire(context.Background(), key, time.Minute*10)
}

func onReportPlayerLogout(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	st, ok := args[0].(*pb3.ReportPlayerLogoutSt)
	if !ok {
		return
	}
	marshal, err := json.Marshal(st)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	client.RPush(context.Background(), "report_player_logout", string(marshal))
}

func init() {
	Register(redismid.SaveActorCache, onSaveActorCache)
	Register(redismid.ReportPlayerLogout, onReportPlayerLogout)
}
