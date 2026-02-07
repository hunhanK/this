/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:43
 */

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"
	"jjyz/gameserver/mq"
	"time"

	"github.com/gzjjyz/logger"
)

var (
	forceLoadGMTime time.Time
)

// 定时检测gm表
func loadGmCmd(force bool) error {
	now := time.Now()
	if !force && now.Before(forceLoadGMTime) {
		return nil
	}

	interval := 5 * time.Minute

	var cmdVec []*model.GmCmd
	err := db.OrmEngine.Where("deltime = ?", 0).Find(&cmdVec)
	if nil != err {
		logger.LogError("loadGmCmd error!!! err:%v", err)
		forceLoadGMTime = now.Add(interval)
		return err
	}

	next := int32(0)
	nowSec := int32(time.Now().Unix())
	m := &model.GmCmd{DelTime: nowSec}
	for _, line := range cmdVec {
		if nowSec < line.ExecTime {
			if diff := line.ExecTime - nowSec; next == 0 || next > diff {
				next = diff
			}
			continue
		}
		if _, err := db.OrmEngine.ID(line.Id).Update(m); nil != err {
			logger.LogError("DelGmCmd Error!!! error:%s", err)
			continue
		}
		gshare.SendGameMsg(custom_id.GMsgLoadGmCmdRet, line)
	}

	if nextInterval := time.Duration(next) * time.Second; nextInterval > 0 && nextInterval < interval {
		forceLoadGMTime = now.Add(nextInterval)
	} else {
		forceLoadGMTime = now.Add(interval)
	}

	return nil
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadGMData, func(param ...interface{}) {
			if err := loadGmCmd(true); nil != err {
				logger.LogError("load gm error %v", err)
			}
		})
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		mq.RegisterMQHandler(pb3.GameServerNatsOpCode_NotifyGm, func(data []byte) error {
			gshare.SendDBMsg(custom_id.GMsgLoadGMData)
			return nil
		})

		gshare.SendDBMsg(custom_id.GMsgLoadGMData)
	})
}
