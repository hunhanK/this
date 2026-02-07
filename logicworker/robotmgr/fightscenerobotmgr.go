/**
 * @Author: zjj
 * @Date: 2024/2/23
 * @Desc: 战斗服场景机器人
**/

package robotmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/internal/timer"
	"strings"
	"time"
)

func InitFightSceneRobotMgr() {
	conf, ok := jsondata.GetRobotMgrConf()
	if !ok {
		return
	}
	for _, line := range conf.Refresh {
		limitCnt := line.TotalNum
		sceneId := line.SceneId
		timer.SetInterval(time.Duration(line.Interval)*time.Second, func() {
			if gshare.GetStaticVar().RobotRefreshStatus != "" && gshare.GetStaticVar().RobotRefreshStatus != "true" {
				return
			}
			hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)
			err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateSceneRobot, &pb3.CreateSceneRobotSt{
				SceneId:        uint32(sceneId),
				IsGm:           false,
				LimitCnt:       uint32(limitCnt),
				SmallCrossCamp: uint32(hostInfo.Camp),
			})
			if err != nil {
				logger.LogError("err:%v", err)
			}
		})
	}
}

func srvConnSuccessRefreshStatus() {
	globalVar := gshare.GetStaticVar()

	var status bool
	if len(globalVar.RobotRefreshStatus) > 0 {
		if strings.Compare(globalVar.RobotRefreshStatus, "true") == 0 {
			status = true
		} else {
			status = false
		}
	} else {
		conf, ok := jsondata.GetRobotMgrConf()
		if ok {
			status = conf.IsRefresh
		}
	}

	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFRobotRefreshStatus, &pb3.SyncRobotRefresh{Status: status})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		srvConnSuccessRefreshStatus()
	})
}
