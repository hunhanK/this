package pyymgr

import (
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func GmAllPlayerOpen(args ...string) bool {
	if len(args) < 3 {
		return false
	}
	yyId := utils.AtoUint32(args[0])
	durationDay := utils.AtoUint32(args[1])
	sTime := time_util.NowSec()
	eTime := time_util.GetDaysZeroTime(durationDay)

	if eTime <= sTime {
		logger.LogWarn("gmAllOpen error id:%d, sTime:%s, eTime:%s", yyId, time_util.TimeToStr(sTime), time_util.TimeToStr(eTime))
		return false
	}

	confIdx := uint32(1)
	if len(args) == 3 {
		confIdx = utils.AtoUint32(args[2])
	}

	globalVar := gshare.GetStaticVar()
	for id, line := range globalVar.GlobalPlayerYY {
		isAdd := true
		player := manager.GetPlayerPtrById(id)
		if nil != player {
			sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
			if ok {
				sys.OpenYY(yyId, sTime, eTime, confIdx, 0, false)
				isAdd = false
			}
		}

		if isAdd {
			if nil == line.Info {
				line.Info = make(map[uint32]*pb3.YYStatus)
			}
			_, ok := line.Info[yyId]
			if !ok {
				line.Info[yyId] = &pb3.YYStatus{
					OTime:   sTime,
					ETime:   eTime,
					ConfIdx: confIdx,
				}
			}
		}
	}

	logger.LogInfo("gmAllOpen open id:%d, sTime:%s, eTime:%s", yyId, time_util.TimeToStr(sTime), time_util.TimeToStr(eTime))
	return true
}

func GmAllPlayerEnd(args ...string) bool {
	if len(args) < 1 {
		return false
	}
	yyId := utils.AtoUint32(args[0])

	CloseAllPlayerYY(yyId)

	logger.LogInfo("GMAllPlayerEnd id:%d", yyId)

	return true
}

func CloseAllPlayerYY(yyId uint32) {
	globalVar := gshare.GetStaticVar()

	nowSec := time_util.NowSec()
	for id, line := range globalVar.GlobalPlayerYY {
		data, ok := line.Info[yyId]
		if ok && nil != data {
			data.ETime = nowSec
		}

		player := manager.GetPlayerPtrById(id)
		if nil != player {
			sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
			if ok {
				sys.CloseYY(yyId, false)
			}
		}
	}

	logger.LogInfo("CloseAllPlayerYY id:%d", yyId)
}

func GmPlayerOpen(args ...string) bool {
	if len(args) < 3 {
		return false
	}
	playerId := utils.AtoUint64(args[0])

	yyId := utils.AtoUint32(args[1])
	durationDay := utils.AtoUint32(args[2])
	sTime := time_util.NowSec()
	eTime := time_util.GetDaysZeroTime(durationDay)

	if eTime <= sTime {
		logger.LogWarn("gmAllOpen error id:%d, sTime:%s, eTime:%s", yyId, time_util.TimeToStr(sTime), time_util.TimeToStr(eTime))
		return false
	}

	confIdx := uint32(1)
	if len(args) == 4 {
		index := utils.AtoUint32(args[3])
		if index > 0 {
			confIdx = index
		}
	}

	globalVar := gshare.GetStaticVar()

	for id, line := range globalVar.GlobalPlayerYY {
		if id != playerId {
			continue
		}
		isAdd := true
		player := manager.GetPlayerPtrById(id)
		if nil != player {
			sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
			if ok {
				sys.OpenYY(yyId, sTime, eTime, confIdx, 0, false)
				isAdd = false
			}
		}

		if isAdd {
			if nil == line.Info {
				line.Info = make(map[uint32]*pb3.YYStatus)
			}
			_, ok := line.Info[yyId]
			if !ok {
				line.Info[yyId] = &pb3.YYStatus{
					OTime:   sTime,
					ETime:   eTime,
					ConfIdx: confIdx,
				}
			}
		}
		break
	}

	logger.LogInfo("gmAllOpen open id:%d, confIdx:%d sTime:%s, eTime:%s", yyId, confIdx, time_util.TimeToStr(sTime), time_util.TimeToStr(eTime))
	return true
}

func GmPlayerEnd(args ...string) bool {
	if len(args) < 2 {
		return false
	}
	playerId := utils.AtoUint64(args[0])
	yyId := utils.AtoUint32(args[1])

	globalVar := gshare.GetStaticVar()

	nowSec := time_util.NowSec()
	for id, line := range globalVar.GlobalPlayerYY {
		if id != playerId {
			continue
		}
		data, ok := line.Info[yyId]
		if ok && nil != data {
			data.ETime = nowSec
		}

		player := manager.GetPlayerPtrById(id)
		if nil != player {
			sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
			if ok {
				sys.CloseYY(yyId, false)
			}
		}
		break
	}

	logger.LogInfo("GMAllPlayerEnd id:%d", yyId)

	return true
}

func GmPYYSetEndTime(playerId uint64, yyId uint32, timeStr string) bool {
	globalVar := gshare.GetStaticVar()
	for id, line := range globalVar.GlobalPlayerYY {
		if playerId != 0 && id != playerId {
			continue
		}

		if timeStr == "" {
			continue
		}

		data := line.Info[yyId]
		if data == nil {
			continue
		}

		endTime := time_util.StrToTime(timeStr + " 23:59:59")
		if endTime <= data.ETime {
			logger.LogDebug("个人运营活动 设置时间少于当前结束时间 %v %v %v ", id, yyId, endTime)
			continue
		}
		data.ETime = endTime
		logger.LogDebug("个人运营活动 设置时间 %v %v %v", id, yyId, endTime)

		if player := manager.GetPlayerPtrById(id); player != nil {
			if sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr); ok {
				if obj := sys.GetObjById(yyId); obj != nil {
					obj.SetEndTime(endTime)
					sys.sendOneYYInfo(yyId)
				}
			}
		}
		logger.LogInfo("GmSetPYYEndTime %v id:%v endTime:%v", id, yyId, endTime)
	}
	return true
}

func GmPYYAddDay(playerId uint64, yyId uint32, day uint32) bool {
	globalVar := gshare.GetStaticVar()
	for id, line := range globalVar.GlobalPlayerYY {
		if playerId != 0 && id != playerId {
			continue
		}

		data := line.Info[yyId]
		if nil == data {
			continue
		}

		endTime := data.ETime + day*gshare.DAY_SECOND
		data.ETime = endTime
		logger.LogDebug("个人运营活动 设置时间 %v %v %v", id, yyId, endTime)

		if player := manager.GetPlayerPtrById(id); player != nil {
			if sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr); ok {
				if obj := sys.GetObjById(yyId); obj != nil {
					obj.SetEndTime(endTime)
					sys.sendOneYYInfo(yyId)
				}
			}
		}

		logger.LogInfo("GmPYYAddDay %v id:%d endTime:%d", id, yyId, endTime)
	}
	return true
}

func init() {
	gmevent.Register("pyy.open", func(actor iface.IPlayer, args ...string) bool {
		allArgs := append([]string{utils.Itoa(actor.GetId())}, args...)
		GmPlayerOpen(allArgs...)
		return true
	}, 1)

	gmevent.Register("pyy.end", func(actor iface.IPlayer, args ...string) bool {
		GmPlayerEnd(utils.Itoa(actor.GetId()), args[0])
		return true
	}, 1)
}
