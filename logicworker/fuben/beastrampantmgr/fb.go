/**
 * @Author: lzp
 * @Date: 2024/11/20
 * @Desc:
**/

package beastrampantmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
)

func onF2GBeastRampantClear(_ []byte) {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FBeastRampantClear, nil)
	if err != nil {
		logger.LogError("onF2GBeastRampantClear err: %v", err)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FBeastRampantStart, nil)
		if err != nil {
			logger.LogError("onConnectFightSrv err: %v", err)
		}
	})

	engine.RegisterSysCall(sysfuncid.C2GBeastRampantClear, onF2GBeastRampantClear)
}
