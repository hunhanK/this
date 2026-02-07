package gshare

import (
	"fmt"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"
	"time"

	"github.com/gzjjyz/logger"
)

var (
	OpenTime             uint32 //开服时间（秒）
	SmallCrossTime       int64  //小跨服跨服时间（秒）
	CrossAllocTimes      uint32 //跨服分配次数
	smallCrossWorldLevel uint32 //小跨服世界等级
	MatchSrvNum          uint32 //当前跨服连接数目
	MediumCrossTime      uint32
)

func SetMediumCrossTime(crossTime uint32) {
	logger.LogInfo("SetMediumCrossTime:%d", crossTime)
	MediumCrossTime = crossTime

	tryUpdateFirstMediumCrossTimestamp(crossTime)
}

func tryUpdateFirstMediumCrossTimestamp(crossTime uint32) {
	if GetFirstMediumCrossTimestamp() == 0 && crossTime > 0 {
		logger.LogInfo("SetFirstMediumCrossTimestamp: %d", crossTime)
		GetStaticVar().FirstMediumCrossTimestamp = crossTime
		event.TriggerSysEvent(custom_id.SeRegGameSrv)
	}
}

func GetMediumCrossDay() uint32 {
	if MediumCrossTime == 0 {
		return 0
	}
	return time_util.TimestampSubDays(uint32(MediumCrossTime), time_util.NowSec()) + 1
}

func GetMediumCrossTime() uint32 {
	return MediumCrossTime
}

func SetSmallCrossWorldLevel(level uint32) {
	logger.LogInfo("SetSmallCrossWorldLevel:%d", level)
	smallCrossWorldLevel = level
}

func GetSmallCrossWorldLevel() uint32 {
	return smallCrossWorldLevel
}

func SetWorldLevel(level uint32) {
	logger.LogInfo("SetWorldLevel:%d", level)
	GetStaticVar().WorldLevel = int32(level)
}

func GetFirstMediumCrossTimestamp() uint32 {
	return GetStaticVar().FirstMediumCrossTimestamp
}

func SetTopFight(fightVal int64) {
	logger.LogInfo("SetTopFightVal: %d", fightVal)
	GetStaticVar().Topfight = fightVal
}

func SetTopLevel(level uint32) {
	logger.LogInfo("SetTopLevel:%d", level)
	GetStaticVar().TopLevel = level
}

func GetTopFight() int64 {
	return GetStaticVar().Topfight
}

// GetOpenServerTime 获取开服时间
func GetOpenServerTime() uint32 {
	return OpenTime
}

func GetWorldLevel() uint32 {
	return uint32(GetStaticVar().WorldLevel)
}

func GetTopLevel() uint32 {
	return uint32(GetStaticVar().TopLevel)
}

// GetOpenServerDayZeroTime 获取开服当天0点时间
func GetOpenServerDayZeroTime() uint32 {
	layout := "2006-01-02"
	t := time.Unix(int64(OpenTime), 0)

	zero, _ := time.ParseInLocation(layout, fmt.Sprintf("%04d-%02d-%02d", t.Year(), t.Month(), t.Day()), time.Local)
	return uint32(zero.Unix())
}

// GetOpenServerDay 获取开服天数
func GetOpenServerDay() uint32 {
	nowSec := uint32(time.Now().Unix())

	openZero := time_util.GetZeroTime(OpenTime)
	nowZero := time_util.GetZeroTime(nowSec)
	if openZero > nowZero {
		return 0
	}
	return nowZero/86400 - openZero/86400 + 1
}

// GetOpenServerWeeks 获取开服周数
func GetOpenServerWeeks() uint32 {
	if OpenTime == 0 {
		return 0
	}
	openTime := time.Unix(int64(OpenTime), 0)
	unix := openTime.Unix()
	nowSec := uint32(time.Now().Unix())
	weeks := time_util.GetDiffWeeks(uint32(unix), nowSec)
	return weeks + 1
}

// GetMergeSrvDay 获取合服天数
func GetMergeSrvDay() uint32 {
	timestamp := GetStaticVar().GetMergeTimestamp()
	if timestamp == 0 {
		return 0
	}

	return time_util.TimestampSubDays(timestamp, time_util.NowSec()) + 1
}

// GetMergeTimes 获取合服次数
func GetMergeTimes() uint32 {
	return GetStaticVar().GetMergeTimes()
}

// GetMergeSrvDayByTimes 获取第几次合服的天数
func GetMergeSrvDayByTimes(times uint32) uint32 {
	mergeData := GetStaticVar().GetMergeData()
	if mergeData == nil {
		return 0
	}
	timestamp, ok := mergeData[times]
	if !ok {
		return 0
	}
	return time_util.TimestampSubDays(timestamp, time_util.NowSec()) + 1
}

func GetSmallCrossDay() uint32 {
	if SmallCrossTime == 0 {
		return 0
	}
	return time_util.TimestampSubDays(uint32(SmallCrossTime), time_util.NowSec()) + 1
}

func SetSmallCrossTime(crossTime int64) {
	SmallCrossTime = crossTime
}

func GetCrossAllocTimes() uint32 {
	return CrossAllocTimes
}

func SetCrossAllocTimes(allocTimes uint32) {
	CrossAllocTimes = allocTimes
}

func GetMatchSrvNum() uint32 {
	return MatchSrvNum
}

func SetMatchSrvNum(num uint32) {
	MatchSrvNum = num
}

// LoadOpenSrvTime 加载开服天数
func LoadOpenSrvTime(sId uint32) error {
	info := make([]*model.ServerInfo, 0)

	err := db.OrmEngine.Where("`server_id` = ?", sId).Find(&info)
	if nil != err {
		return err
	}

	hasRecord := false
	if len(info) > 0 {
		OpenTime = info[0].OpenTime
		hasRecord = true
	}

	if OpenTime <= 0 {
		OpenTime = uint32(time.Now().Unix())
		logger.LogError("数据库未配置开服时间, 自动设置为当前时间 %d", OpenTime)
		if hasRecord {
			_, err = db.OrmEngine.Table(model.ServerInfo{}.TableName()).Where("`server_id` = ?", sId).Update(map[string]interface{}{
				"open_time": OpenTime,
			})

		} else {
			_, err = db.OrmEngine.InsertOne(model.ServerInfo{ServerId: sId, OpenTime: OpenTime})
		}
		if nil != err {
			return err
		}
	}

	base := uint32(time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local).Unix())
	if OpenTime < base {
		OpenTime = uint32(time.Date(2030, 1, 1, 0, 0, 0, 0, time.Local).Unix())
		logger.LogError("没配开服时间或者配置的开服天数小于2020年")
	}
	t := time.Unix(int64(OpenTime), 0)
	logger.LogInfo("开服时间：%d-%d-%d %d:%d:%d  %d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), OpenTime)
	logger.LogInfo("开服天数：%d", GetOpenServerDay())
	return nil
}

func GetGuildSecretLeaderId() uint64 {
	return GetStaticVar().GuildSecretLeaderId
}
