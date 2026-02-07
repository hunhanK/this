/**
 * @Author: zjj
 * @Date: 2025/5/12
 * @Desc:
**/

package redisworker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/redisworker/redismid"
)

const (
	GuildRuleConfKey = "guild_rule_config_%d"
)

type GuildRuleCache struct {
	PfId       uint32 `json:"pf_id"`
	InitNum    uint32 `json:"init_num"`
	AddNum     uint32 `json:"add_num"`
	UpdateTime uint32 `json:"update_time"`
}

func ReloadGuildRule(_ ...interface{}) {
	bytes, err := client.HGet(context.Background(), fmt.Sprintf(GuildRuleConfKey, engine.GetAppId()), fmt.Sprintf("%d", engine.GetPfId())).Bytes()
	if err != nil {
		logger.LogError("加载后台仙盟规则失败 err:%v", err)
		return
	}
	var st GuildRuleCache
	err = json.Unmarshal(bytes, &st)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	if st.InitNum == 0 {
		return
	}
	logger.LogInfo("后台仙盟规则: %s", string(bytes))
	gshare.SendGameMsg(custom_id.GMsgReloadGuildRuleRet, &pb3.BackStageGuildRule{
		InitNum: st.InitNum,
		AddNum:  st.AddNum,
	})
}
func init() {
	Register(redismid.ReloadGuildRule, ReloadGuildRule)
}
