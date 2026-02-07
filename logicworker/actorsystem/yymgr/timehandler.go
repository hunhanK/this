/**
 * @Author: zjj
 * @Date: 2025/8/7
 * @Desc:
**/

package yymgr

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/jsondata"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"strings"
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

func handleOpenSrvParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
	day := gshare.GetOpenServerDay()
	return srvDayParser(day, conf)
}

func handleMergeSrvParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
	day := gshare.GetMergeSrvDay()
	return srvDayParser(day, conf)
}

func handleDateParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
	sTime := time_util.StrToTime(conf.StartTime + " 00:00:00")
	eTime := time_util.StrToTime(conf.EndTime + " 23:59:59")

	today := time_util.GetZeroTime(time_util.NowSec())
	if conf.FixedDay && today != sTime {
		return 0, 0
	}
	return sTime, eTime
}

func handleWeekParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
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
	if conf.FixedDay && today != sTime {
		return 0, 0
	}
	e := utils.AtoUint32(conf.EndTime)
	s := utils.AtoUint32(conf.StartTime)
	if e < s {
		return 0, 0
	}
	eTime := sTime + (e-s+1)*gshare.DAY_SECOND - 1

	return sTime, eTime
}

func handleSecondMergeSrvParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
	day := gshare.GetMergeSrvDayByTimes(2)
	return srvDayParser(day, conf)
}

func handleXMergeSrvParser(conf *jsondata.YunYingTimeConf) (uint32, uint32) {
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
	var newConf = &jsondata.YunYingTimeConf{
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
