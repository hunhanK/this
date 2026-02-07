/**
 * @Author: zjj
 * @Date: 2024/12/4
 * @Desc: 活动扫荡
**/

package activity

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/actsweepmgr"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/activitydef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

var singleActSweep = &ActSweep{}

type ActSweep struct {
	dailyActOpenInfo *pb3.DailyActOpenInfo
}

func (s *ActSweep) calcEndAct(endList []*pb3.ActStatusInfo) {
	if len(endList) == 0 {
		return
	}

	var actOpenTimes = make(map[uint32]uint32)
	for _, end := range endList {
		actOpenTimes[end.ActId] += 1
	}

	var calcActToAllPlayer []uint32
	for actId, times := range actOpenTimes {
		oldTimes, ok := s.dailyActOpenInfo.ActOpenTimes[actId]
		if ok && oldTimes == times {
			continue
		}

		// 重启后才结束的活动
		calcActToAllPlayer = append(calcActToAllPlayer, actId)
		s.dailyActOpenInfo.ActOpenTimes[actId] = times
	}

	for _, actId := range calcActToAllPlayer {
		s.calcSingleAct(actId)
	}
}

func (s *ActSweep) calcSingleAct(actId uint32) {
	// 检查新结束的活动是否能够找回
	logger.LogInfo("calc act to all player actId: %d", actId)
	sweepConf := jsondata.GetActSweepConfByActivity(actId)
	if sweepConf == nil {
		logger.LogError("not found sweep conf actId: %d", actId)
		return
	}

	activityConf := jsondata.GetActivityConf(actId)
	if activityConf == nil {
		logger.LogError("not found activity conf actId: %d", actId)
		return
	}

	times := s.dailyActOpenInfo.ActOpenTimes[actId]
	if sweepConf.OpenCond != nil && sweepConf.OpenCond.ActEndTimes > times {
		return
	}

	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, sweepConf.Id)
}

func (s *ActSweep) playerJoinAct(playerId uint64, actId uint32, actStartAt uint32, replace bool) {
	val, ok := s.dailyActOpenInfo.ActJoinPlayerMap[actId]
	if !ok {
		s.dailyActOpenInfo.ActJoinPlayerMap[actId] = &pb3.DailyActSweepJoinPlayer{}
		val = s.dailyActOpenInfo.ActJoinPlayerMap[actId]
	}

	if val.ActorJoinTimes == nil {
		val.ActorJoinTimes = make(map[uint64]*pb3.DailyActSweepJoinByActStartAt)
	}

	joinByActStartAt, ok := val.ActorJoinTimes[playerId]
	if !ok {
		val.ActorJoinTimes[playerId] = &pb3.DailyActSweepJoinByActStartAt{
			ActStartAtTimes: make(map[uint32]uint32),
		}
		joinByActStartAt = val.ActorJoinTimes[playerId]
	}

	if replace {
		joinByActStartAt.ActStartAtTimes[actStartAt] = 1
	} else {
		joinByActStartAt.ActStartAtTimes[actStartAt] += 1
	}
}

// 优先级比战斗服连接事件高
func actSweepHandleSeServerInit(_ ...interface{}) {
	staticVar := gshare.GetStaticVar()
	if staticVar.DailyActOpenInfo == nil {
		staticVar.DailyActOpenInfo = &pb3.DailyActOpenInfo{}
	}

	dailyActOpenInfo := staticVar.DailyActOpenInfo
	if dailyActOpenInfo.ActOpenTimes == nil {
		dailyActOpenInfo.ActOpenTimes = make(map[uint32]uint32)
	}

	if dailyActOpenInfo.TodayZeroAt == 0 {
		dailyActOpenInfo.TodayZeroAt = time_util.GetDaysZeroTime(0)
	}

	if dailyActOpenInfo.ActJoinPlayerMap == nil {
		dailyActOpenInfo.ActJoinPlayerMap = make(map[uint32]*pb3.DailyActSweepJoinPlayer)
	}

	todayZeroAt := time_util.GetDaysZeroTime(0)

	// 不是同一天
	if !time_util.IsSameDay(dailyActOpenInfo.TodayZeroAt, todayZeroAt) {
		dailyActOpenInfo.TodayZeroAt = todayZeroAt
		dailyActOpenInfo.ActOpenTimes = make(map[uint32]uint32)
		dailyActOpenInfo.ActJoinPlayerMap = make(map[uint32]*pb3.DailyActSweepJoinPlayer)
	}

	// 今日的活动关服前已开启列表
	singleActSweep.dailyActOpenInfo = dailyActOpenInfo
}

func actSweepHandleF2GSyncFightSrvActiveEndList(buf []byte) {
	var req pb3.SyncFightSrvActiveEndList
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("pb3.Unmarshal err:%v", err)
		return
	}
	logger.LogInfo("SyncFightSrvActiveEndList cross:%v", req.IsCross)
	singleActSweep.calcEndAct(req.EndList)
}

func actSweepHandleF2GActSweepPlayerJoinAct(buf []byte) {
	var req pb3.CallActSweepPlayerJoinAct
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("pb3.Unmarshal err:%v", err)
		return
	}
	logger.LogInfo("actSweepHandleF2GActSweepPlayerJoinAct cross:%v", req.IsCross)
	singleActSweep.playerJoinAct(req.PlayerId, req.ActId, req.ActStartAt, req.Replace)
}

var singleSweepController = &SweepController{}

type SweepController struct {
	actsweepmgr.Base
}

func (receiver *SweepController) GetCanUseTimes(id uint32, playerId uint64) (canUseTimes uint32) {
	sweepConf := jsondata.GetActSweepConf(id)
	if sweepConf == nil {
		logger.LogWarn("not found conf %d", id)
		return
	}

	actId := sweepConf.ActId
	activityConf := jsondata.GetActivityConf(actId)
	if activityConf == nil {
		logger.LogError("not found activity conf id:%d actId: %d", id, actId)
		return
	}

	// 今天是否开过活动
	dailyActOpenInfo := singleActSweep.dailyActOpenInfo
	openTimes, ok := dailyActOpenInfo.ActOpenTimes[actId]
	if !ok {
		return
	}

	if sweepConf.OpenCond != nil && sweepConf.OpenCond.ActEndTimes > openTimes {
		return
	}

	if sweepConf.OpenTimesToSweepTimes {
		canUseTimes = openTimes
	} else {
		canUseTimes = sweepConf.SweepTimes
	}

	// 否则就拿活动统计的
	actJoinPlayer := dailyActOpenInfo.ActJoinPlayerMap[actId]
	if actJoinPlayer != nil {
		actorJoinTimes := actJoinPlayer.ActorJoinTimes[playerId]
		if actorJoinTimes != nil {
			useTimes := uint32(len(actorJoinTimes.ActStartAtTimes))
			if useTimes >= canUseTimes {
				canUseTimes = 0
			} else {
				canUseTimes -= useTimes
			}
		}
	}

	return
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		actSweepHandleSeServerInit()
		jsondata.EachActSweepConfByActivity(func(conf *jsondata.ActSweepConf) {
			registerActStatusEvent(conf.ActId, func(actId uint32, status *pb3.ActStatusInfo) {
				switch status.Status {
				case activitydef.ActEnd:
					singleActSweep.dailyActOpenInfo.ActOpenTimes[conf.ActId] += 1
					singleActSweep.calcSingleAct(conf.ActId)
				case activitydef.ActStart:
					_, ok := singleActSweep.dailyActOpenInfo.ActJoinPlayerMap[conf.ActId]
					if ok {
						return
					}
					singleActSweep.dailyActOpenInfo.ActJoinPlayerMap[conf.ActId] = &pb3.DailyActSweepJoinPlayer{
						ActorJoinTimes: make(map[uint64]*pb3.DailyActSweepJoinByActStartAt),
					}
				}
			})
			if conf.ActId > 0 {
				actsweepmgr.Reg(conf.Id, singleSweepController)
			}
		})
	})
	event.RegSysEvent(custom_id.SeNewDayArrive, actSweepHandleSeServerInit)
	engine.RegisterSysCall(sysfuncid.F2GSyncFightSrvActiveEndList, actSweepHandleF2GSyncFightSrvActiveEndList)
	engine.RegisterSysCall(sysfuncid.F2GCallActSweepPlayerJoinAct, actSweepHandleF2GActSweepPlayerJoinAct)
}
