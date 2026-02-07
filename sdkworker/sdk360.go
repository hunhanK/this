/**
 * @Author: zjj
 * @Date: 2025/2/25
 * @Desc:
**/

package sdkworker

import (
	"github.com/gzjjyz/logger"
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
)

// 聊天频道映射
var _360ChatTypeRef = map[int]int{
	chatdef.CIWorld:     wm.ChatType360ByWorld,
	chatdef.CITeam:      wm.ChatType360ByTeam,
	chatdef.CIGuild:     wm.ChatType360ByGuild,
	chatdef.CICrossChat: wm.ChatType360ByWorld,
	chatdef.CINear:      wm.ChatType360ByNear,
	chatdef.CIBroadcast: wm.ChatType360ByBroadcast,
	chatdef.CIPrivate:   wm.ChatType360ByPrivate,
}

func on360WanSdkReport(args ...interface{}) {
	if len(args) < 3 {
		logger.LogError("sdk report failed. args = %v", args)
		return
	}
	reportEventType := args[0].(uint32)
	// 判断是什么类型的上报
	switch reportEventType {
	case uint32(pb3.SdkReportEventType_SdkReportEventTypeRoleRegister):
		data := args[2].(*argsdef.RepostTo360WanCreateRole)
		if data == nil || !engine.Is360Wan(data.DitchId) {
			return
		}
		data.Type = "create_role"
		if data.Channel == "" {
			data.Channel = "0"
		}
		if data.Poster == "" {
			data.Channel = "0"
		}
		if data.Site == "" {
			data.Site = "0"
		}
		resp, err := NewRequest().SetFormData(data.ToMap()).Post("http://dd.mgame.360.cn/t/gameinfo/log")
		if err != nil {
			logger.LogError("err:%v", err)
			return
		}
		logger.LogInfo("resp:%s", string(resp.Body()))
	case uint32(pb3.SdkReportEventType_SdkReportEventTypeRoleLogin):
		data := args[2].(*argsdef.RepostTo360WanLogin)
		if data == nil || !engine.Is360Wan(data.DitchId) {
			return
		}
		data.Type = "login"
		resp, err := NewRequest().SetFormData(data.ToMap()).Post("http://dd.mgame.360.cn/t/gameinfo/log")
		if err != nil {
			logger.LogError("err:%v", err)
			return
		}
		logger.LogInfo("resp:%s", string(resp.Body()))
	}
}
