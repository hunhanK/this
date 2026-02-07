/**
 * @Author: zjj
 * @Date: 2025/8/7
 * @Desc:
**/

package pyymgr

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/jsondata"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"strings"
	"time"
)

const (
	OpenSrvParser        = 1 // 开服天数
	MergeSrvParser       = 2 // 合服天数
	DateParser           = 3 // 普通日期
	WeekParser           = 4 // 星期
	SecondMergeSrvParser = 5 // 二合天数
	CmdYYOpenParser      = 6 // 后台开启
	XMergeSrvParser      = 7 // X合天数
)

var TimeHandler = map[uint32]TimeParse{
	OpenSrvParser:        handleOpenSrvParser,
	MergeSrvParser:       handleMergeSrvParser,
	DateParser:           handleDateParser,
	WeekParser:           handleWeekParser,
	SecondMergeSrvParser: handleSecondMergeSrvParser,
	XMergeSrvParser:      handleXMergeSrvParser,
}

func handleOpenSrvParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	day := gshare.GetOpenServerDay()
	return srvDayParser(day, conf)
}

func handleMergeSrvParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	day := gshare.GetMergeSrvDay()
	return srvDayParser(day, conf)
}

func handleDateParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	sTime := time_util.StrToTime(conf.StartTime + " 00:00:00")
	eTime := time_util.StrToTime(conf.EndTime + " 23:59:59")

	today := time_util.GetZeroTime(time_util.NowSec())
	if conf.FixedDay && today != sTime {
		return 0, 0
	}
	return sTime, eTime
}

func handleWeekParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	sWeekDay := utils.AtoUint32(conf.StartTime)

	weekday := uint32(time_util.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	var before uint32
	if weekday == sWeekDay {
		before = 0
	} else if weekday < sWeekDay {
		before = 7 + weekday - sWeekDay
	} else {
		before = weekday - sWeekDay
	}

	sTime := time_util.GetBeforeDaysZeroTime(before)

	today := time_util.GetZeroTime(time_util.NowSec())

	e := utils.AtoUint32(conf.EndTime)
	s := utils.AtoUint32(conf.StartTime)
	if e < s {
		return 0, 0
	}
	eTime := sTime + (e-s+1)*gshare.DAY_SECOND - 1
	if eTime < today {
		sTime += 7 * gshare.DAY_SECOND
		eTime += 7 * gshare.DAY_SECOND
	}

	if conf.FixedDay && today != sTime {
		return 0, 0
	}

	return sTime, eTime
}

func handleSecondMergeSrvParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	day := gshare.GetMergeSrvDayByTimes(2)
	return srvDayParser(day, conf)
}

func handleXMergeSrvParser(conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	mergeTimes := utils.AtoUint32(conf.StartTime)
	split := strings.Split(conf.EndTime, ",")
	if len(split) != 2 {
		return 0, 0
	}
	if mergeTimes == 0 {
		return 0, 0
	}
	sTime, eTime := utils.AtoUint32(split[0]), utils.AtoUint32(split[1])
	day := gshare.GetMergeSrvDayByTimes(mergeTimes)
	if day == 0 || sTime == 0 || eTime == 0 {
		return 0, 0
	}
	if sTime > eTime {
		return 0, 0
	}
	var newConf = &jsondata.PlayerYYTimeConf{
		TimeType:  MergeSrvParser,
		StartTime: split[0],
		EndTime:   split[1],
		ConfIdx:   conf.ConfIdx,
		Loop:      conf.Loop,
		FixedDay:  conf.FixedDay,
		Interval:  conf.Interval,
	}
	return srvDayParser(day, newConf)
}

func srvDayParser(day uint32, conf *jsondata.PlayerYYTimeConf) (uint32, uint32) {
	sTime, eTime := utils.AtoUint32(conf.StartTime), utils.AtoUint32(conf.EndTime)

	if conf.FixedDay && day != sTime {
		return 0, 0
	}

	duration := eTime - sTime + 1
	loopDuration := duration + conf.Interval

	var diff uint32
	if day >= sTime {
		diff = day - sTime
	}
	nLoop := (diff / loopDuration) + 1 // 现在是第几个循环
	if eTime+(nLoop-1)*loopDuration < day {
		nLoop++
	}
	if conf.Loop == -1 || int(nLoop) <= conf.Loop {
		// 无限循环，或者循环未结束
		thisLoopStartDay := sTime + (loopDuration * (nLoop - 1))

		sTime = time_util.GetBeforeDaysZeroTime(day - thisLoopStartDay)
		eTime = uint32(time.Unix(int64(sTime), 0).AddDate(0, 0, int(duration)).Unix())
		return sTime, eTime
	}
	return 0, 0
}
