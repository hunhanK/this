/**
 * @Author: zjj
 * @Date: 2025/2/25
 * @Desc:
**/

package sdkworker

import (
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/gameserver/engine"
)

var (
	defaultMonitor wm.Monitor
)

func initMonitor() {
	defaultMonitor = wm.NewYDunMonitor("b408bb52311552e8c875e0f71de232bf", "49836b42a9a6ab0b1380b6e3d0016018")
	defaultMonitor.SetNameBusinessId("77c60a4339d5681e5ccf5a8583969202")
	defaultMonitor.SetChatBusinessId("3c90baad431212396179b7a191a7d8bb")
}

func GetMonitor(ditchId uint32) wm.Monitor {
	if engine.Is360Wan(ditchId) {
		info := engine.Get360WanInfo(ditchId)
		return wm.New360WanMonitor(info.Gkey, info.LoginKey, _360ChatTypeRef)
	}
	return defaultMonitor
}
