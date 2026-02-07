/**
 * @Author: zjj
 * @Date: 2025/2/25
 * @Desc:
**/

package sdkworker

import (
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/base/custom_id/chatdef"
	"testing"
)

func TestWordMonitorBy360(t *testing.T) {
	monitor := wm.New360WanMonitor("lxzj", "lTe9KJbLlEzb5mlBwq639z8p5qKPAXAs", map[int]int{
		chatdef.CIWorld:     wm.ChatType360ByWorld,
		chatdef.CITeam:      wm.ChatType360ByTeam,
		chatdef.CIGuild:     wm.ChatType360ByGuild,
		chatdef.CICrossChat: wm.ChatType360ByWorld,
		chatdef.CINear:      wm.ChatType360ByNear,
		chatdef.CIBroadcast: wm.ChatType360ByBroadcast,
		chatdef.CIPrivate:   wm.ChatType360ByPrivate,
	})

	ret, err := monitor.CheckChat(&wm.CommonData{
		ActorId:                      1100434376711,
		ActorName:                    "你好",
		ActorIP:                      "14.145.59.58",
		PlatformUniquePlayerId:       "13000_3326974017",
		TargetActorId:                0,
		TargetActorName:              "",
		Content:                      "好好",
		PlatformUniqueTargetPlayerId: "",
		SrvId:                        55,
		ChatChannel:                  chatdef.CIWorld,
	})
	if err != nil {
		t.Logf("err:%v", err)
		return
	}
	t.Log(ret)
}
