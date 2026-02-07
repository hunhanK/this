/**
 * @Author: LvYuMeng
 * @Date: 2025/6/10
 * @Desc:
**/

package redisworker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/argsdef"
	"jjyz/base/rediskey"
	"jjyz/gameserver/redisworker/redismid"
)

func onSaveCmdYYSetting(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	req, ok := args[0].(*argsdef.CmdYYSetting)
	if !ok {
		return
	}

	key := fmt.Sprintf(rediskey.GameCmdYYSetting)

	cfgJ, err := json.Marshal(req)
	if err != nil {
		logger.LogError("on save game basic error! %v", err.Error())
		return
	}

	err = client.RPush(context.Background(), key, cfgJ).Err()

	if nil != err {
		logger.LogError("on save game basic error! %v", err.Error())
		return
	}
}

func init() {
	Register(redismid.SaveCmdYYSetting, onSaveCmdYYSetting)
}
