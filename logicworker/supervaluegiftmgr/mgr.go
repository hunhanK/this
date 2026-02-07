/**
 * @Author: LvYuMeng
 * @Date: 2024/11/6
 * @Desc: 超值礼包
**/

package supervaluegiftmgr

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/internal/timer"
	"time"
)

var groupTimer = make(map[uint32]*time_util.Timer)

func GetData() map[uint32]*pb3.GlobalSuperValueGiftGroup {
	if nil == gshare.GetStaticVar().SuperValueGift {
		gshare.GetStaticVar().SuperValueGift = make(map[uint32]*pb3.GlobalSuperValueGiftGroup)
	}
	return gshare.GetStaticVar().SuperValueGift
}

func checkGiftInit(args ...interface{}) {
	conf, ok := jsondata.GetSuperValueGiftConf()
	if !ok {
		return
	}
	for groupId := range conf.Groups {
		resetGroup(groupId)
	}
}

func resetGroup(groupId uint32) {
	groupConf, ok := jsondata.GetSuperValueGiftGroupConf(groupId)
	if !ok {
		return
	}
	if groupConf.RefreshType != custom_id.SuperValueGiftRefreshTypeGlobal {
		return
	}
	if groupConf.RefreshInterval == 0 {
		logger.LogError("conf %d interval cant be zero!", groupId)
		return
	}
	groupMap := GetData()
	if _, ok := groupMap[groupId]; !ok {
		groupMap[groupId] = &pb3.GlobalSuperValueGiftGroup{}
	}

	curTime := time_util.NowSec()
	group := groupMap[groupId]
	group.GroupId = groupId

	needReset := group.NextRefreshTime <= curTime
	if needReset {
		if group.NextRefreshTime == 0 {
			group.NextRefreshTime = curTime + groupConf.RefreshInterval
		} else {
			loop := utils.MaxInt64(int64(curTime-group.NextRefreshTime), 0)/int64(groupConf.RefreshInterval) + 1
			group.NextRefreshTime = group.NextRefreshTime + uint32(loop)*groupConf.RefreshInterval
		}
		group.GiftId = groupConf.RandGiftList()
	}

	engine.Broadcast(chatdef.CIWorld, 0, 69, 240, &pb3.S2C_69_240{
		Group: group,
	}, 0)

	event.TriggerSysEvent(custom_id.SeSuperValueGiftRefresh)

	if tm, ok := groupTimer[groupId]; ok {
		tm.Stop()
	}

	diff := utils.MaxInt64(0, int64(group.NextRefreshTime-curTime))

	groupTimer[groupId] = timer.SetTimeout(time.Duration(diff)*time.Second, func() {
		resetGroup(groupId)
	})
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, checkGiftInit)
}
