package gm

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
)

//游戏公告

// 添加游戏公告
func addNotice(param string, tipMsgId, sTime, eTime uint32, interval uint32) {
	sVar := gshare.GetStaticVar()
	notice := &pb3.Notice{}
	notice.Param = param
	notice.TipMsgId = tipMsgId
	notice.StartTime = sTime
	notice.EndTime = eTime
	notice.Interval = interval
	sVar.Notices = append(sVar.Notices, notice)
}

// 删除公告
func removeNotice(msg string) {
	logger.LogInfo("remove notice! msg:%s", msg)
	if len(msg) <= 0 {
		return
	}
	sVar := gshare.GetStaticVar()
	if "$" == msg {
		sVar.Notices = make([]*pb3.Notice, 0)
		logger.LogInfo("remove all notice!!!")
		return
	}

	last := len(sVar.Notices) - 1
	if last < 0 {
		return
	}

	change := false
	for idx := last; idx >= 0; idx-- {
		notice := sVar.Notices[idx]
		if notice.GetParam() == msg {
			if idx != last {
				sVar.Notices[idx] = sVar.Notices[last]
			}
			last--
			change = true
		}
	}
	if change {
		sVar.Notices = sVar.Notices[:last+1]
	}
}

// CheckOneSec 每秒检测一次
func CheckOneSec() {
	sVar := gshare.GetStaticVar()
	size := len(sVar.Notices)
	if 0 == size {
		return
	}
	last := size - 1

	now := time_util.NowSec()
	for idx := last; idx >= 0; idx-- {
		notice := sVar.Notices[idx]
		if notice.GetStartTime() <= now && now < notice.GetEndTime() {
			notice.StartTime = now + notice.GetInterval()
			engine.BroadcastTipMsgById(notice.GetTipMsgId(), notice.GetParam())
		} else if now >= notice.GetEndTime() { //已过期
			if idx != last {
				sVar.Notices[idx] = sVar.Notices[last]
			}
			sVar.Notices[last] = nil
			last--
		}
	}
	if last < size-1 {
		sVar.Notices = sVar.Notices[:last+1]
	}
}
