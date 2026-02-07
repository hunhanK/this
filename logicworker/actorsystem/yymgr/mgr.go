package yymgr

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"sync"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

const (
	RangeMergeTimes      = 1 // 合服次数
	RangeMergeDays       = 2 // 合服天数
	RangeOpenDays        = 3 // 开服天数
	RangeOpenYearOfWeek  = 4 // 当前年循环到的周
	RangeOpenSpecDays    = 6 // 指定日期
	RangeOpenWeeks       = 7 // 开服周数
	RangeOpenSrvOfWeek   = 8 // 开服循环到的星期
	RangeSecondMergeDays = 9 // 二合持续天数
)

var (
	once      sync.Once
	singleton *YYMgr
	CreateMap = make(map[uint32]CreateFunc)
)

type (
	CreateFunc func() iface.IYunYing
	TimeParse  func(*jsondata.YunYingTimeConf) (uint32, uint32)

	YYMgr struct {
		objMap map[uint32]iface.IYunYing
		YYInfo *pb3.GlobalYY
	}
)

func GetYYMgr() *YYMgr {
	once.Do(func() {
		singleton = &YYMgr{}
		singleton.objMap = make(map[uint32]iface.IYunYing)
	})
	return singleton
}

// RegisterYYType 注册运营活动大类
func RegisterYYType(yyType uint32, fn CreateFunc) {
	CreateMap[yyType] = fn
}

func GetYYByActId(actId uint32) iface.IYunYing {
	mgr := GetYYMgr()
	if mgr == nil || mgr.objMap == nil {
		return nil
	}
	iYunYing := mgr.objMap[actId]
	if iYunYing == nil {
		return nil
	}
	return iYunYing
}

func GetAllYY(class uint32) []iface.IYunYing {
	list := make([]iface.IYunYing, 0, 5)
	for _, v := range GetYYMgr().objMap {
		if v.GetClass() == class {
			list = append(list, v)
		}
	}
	return list
}

func (mgr *YYMgr) GetYYInfo() map[uint32]*pb3.YYStatus {
	staticVar := gshare.GetStaticVar()
	if staticVar.GlobalYY == nil {
		staticVar.GlobalYY = &pb3.GlobalYY{}
	}
	mgr.YYInfo = staticVar.GlobalYY
	if mgr.YYInfo.Info == nil {
		mgr.YYInfo.Info = make(map[uint32]*pb3.YYStatus)
	}
	return mgr.YYInfo.Info
}

func (mgr *YYMgr) OnInit() {
	mgr.objMap = make(map[uint32]iface.IYunYing)

	delMap := make(map[uint32]struct{})
	// 加载活动数据
	for id, line := range mgr.GetYYInfo() {
		logger.LogInfo("[全服活动] 加载, Id:%d, %v", id, line)
		if conf := jsondata.GetYunYingConf(id); nil == conf {
			delMap[id] = struct{}{}
			continue
		}

		yyLine := jsondata.GetYunYingConf(id)
		obj := mgr.createYYObj(yyLine.Class, id)
		if obj != nil {
			obj.Init(line.OTime, line.ETime)

			if line.ConfIdx > 0 {
				obj.SetConfIdx(line.ConfIdx)
			}

			obj.OnInit()

		} else {
			logger.LogError("[玩家活动] 加载失败, id:%d", id)
		}
	}

	yyInfo := mgr.GetYYInfo()
	for id := range delMap {
		delete(yyInfo, id)
		logger.LogInfo("全服活动：删除没有配置的运营活动：%d", id)
	}
}

func (mgr *YYMgr) OnAfterLogin(player iface.IPlayer) {
	mgr.checkAllClose(true)
	mgr.checkAllOpen(true)
	mgr.checkAllChainOpen(true)
	mgr.SendYunYingInfo(player)
	for _, line := range mgr.objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.PlayerLogin(player)
		})
	}
}

func (mgr *YYMgr) OnReconnect(player iface.IPlayer) {
	mgr.SendYunYingInfo(player)
	for _, line := range mgr.objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.PlayerReconnect(player)
		})
	}
}

func (mgr *YYMgr) BeforeNewDay() {
	mgr.checkAllClose(false)
	for _, line := range mgr.objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.BeforeNewDay()
		})
	}
}

func (mgr *YYMgr) NewDay() {
	for _, line := range mgr.objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.NewDay()
		})
	}
	mgr.checkAllOpen(false)
	mgr.checkAllChainOpen(false)
	mgr.SendWaitOpenList(nil)
}

func (mgr *YYMgr) SendYunYingInfo(player iface.IPlayer) {
	msg := &pb3.S2C_40_2{}

	for _, line := range mgr.objMap {
		msg.Infos = append(msg.Infos, line.GetYYStateInfo())
	}

	player.SendProto3(40, 2, msg)
	mgr.SendWaitOpenList(player)
}

func (mgr *YYMgr) createYYObj(class, id uint32) iface.IYunYing {
	fn, ok := CreateMap[class]
	if !ok {
		return nil
	}

	if obj, ok := mgr.objMap[id]; ok {
		logger.LogWarn("全服活动已开启 id:%d, sTime:%s, eTime:%s",
			id, time_util.SecToTimeStr(obj.GetOpenTime()), time_util.SecToTimeStr(obj.GetEndTime()))
		return nil
	}

	obj := fn()
	obj.SetId(id)
	mgr.objMap[id] = obj

	return obj
}

func (mgr *YYMgr) OpenYY(id, sTime, eTime, confIdx uint32, isInit bool) {
	conf := jsondata.GetYunYingConf(id)
	if nil == conf {
		return
	}

	obj := mgr.createYYObj(conf.Class, id)
	if nil == obj {
		return
	}

	obj.Init(sTime, eTime)

	if confIdx > 0 {
		obj.SetConfIdx(confIdx)
	}

	obj.OnInit()

	if !isInit {
		obj.BroadcastYYStateInfo()
	}

	obj.ResetData()
	obj.OnOpen()

	event.TriggerSysEvent(custom_id.SeGlobalYyOpen, id)

	yyInfo := mgr.GetYYInfo()
	status := &pb3.YYStatus{
		OTime:   sTime,
		ETime:   eTime,
		ConfIdx: obj.GetConfIdx(),
		Class:   conf.Class,
	}

	yyInfo[id] = status

	if mgr.isCrossYYClass(status.Class) {
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncCrossYYInfo, &pb3.G2CSyncCrossYYInfo{
			Status: &pb3.CrossYYStatus{
				Id:      id,
				OTime:   status.OTime,
				ETime:   status.ETime,
				ConfIdx: status.ConfIdx,
			},
			Op: custom_id.CrossYYOpSync,
		})
		if err != nil {
			logger.LogError("err:%v", err)
		}
	}
}

var crossYYList = map[uint32]struct{}{
	yydefine.YYSummerSurfDiamond: {},
}

func (mgr *YYMgr) isCrossYYClass(id uint32) bool {
	_, ok := crossYYList[id]
	return ok
}

func (mgr *YYMgr) checkAllChainOpen(isInit bool) {
	yyPaths := jsondata.GetChainOpenYYPath()
	if nil == yyPaths || len(yyPaths) == 0 {
		return
	}
	var canOpenPaths []pie.Uint32s
	for _, paths := range yyPaths {
		var canOpen = true
		for _, id := range paths {
			obj, ok := mgr.objMap[id]
			if ok && nil != obj && obj.IsOpen() {
				canOpen = false
				break
			}
		}
		if !canOpen {
			continue
		}
		canOpenPaths = append(canOpenPaths, paths)
	}
	for _, paths := range canOpenPaths {
		paths.Each(func(id uint32) {
			line := jsondata.YunYingConfMgr[id]
			if line == nil {
				return
			}
			// 避免重复开
			obj, ok := mgr.objMap[id]
			if ok && nil != obj && obj.IsOpen() {
				return
			}
			mgr.checkOpen(id, line, isInit)
		})
	}
}

func (mgr *YYMgr) checkAllOpen(isInit bool) {
	if nil == jsondata.YunYingConfMgr {
		return
	}

	for id, line := range jsondata.YunYingConfMgr {
		// 需要链式开启的 跳过
		if jsondata.InChainOpenYY(id) {
			continue
		}
		obj, ok := mgr.objMap[id]
		if ok && nil != obj && obj.IsOpen() {
			continue
		}
		mgr.checkOpen(id, line, isInit)
	}

	event.TriggerSysEvent(custom_id.SeCheckCmdYYOpen, isInit)
}

func (mgr *YYMgr) isOpenThisGroup(group uint32) bool {
	if group == 0 {
		return false
	}
	for id, _ := range mgr.objMap {
		if jsondata.GetYunYingGroup(id) == group {
			return true
		}
	}
	return false
}

func (mgr *YYMgr) checkOpen(id uint32, conf *jsondata.YunYingConf, isInit bool) {
	if mgr.isOpenThisGroup(conf.Group) {
		return
	}
	nowSec := time_util.NowSec()
	pfId := engine.GetPfId()
	for _, line := range conf.Range {
		if len(line.PfIds) > 0 && !utils.SliceContainsUint32(line.PfIds, pfId) {
			continue
		}

		if !mgr.inRange(line) {
			continue
		}

		for _, timeConf := range line.TimeConf {
			parser, ok := TimeHandler[timeConf.TimeType]
			if !ok {
				continue
			}
			start, end := parser(timeConf)
			if nowSec >= start && nowSec < end {
				mgr.OpenYY(id, start, end, timeConf.ConfIdx, false)
				return
			}
		}
	}
}

func (mgr *YYMgr) checkAllClose(isInit bool) {
	now := time_util.NowSec()
	for id, obj := range mgr.objMap {
		if obj.GetEndTime() <= now {
			utils.ProtectRun(func() {
				mgr.CloseYY(id, isInit)
			})
		}
	}
}

func (mgr *YYMgr) SendWaitOpenList(player iface.IPlayer) {
	if nil == jsondata.YunYingConfMgr {
		return
	}
	waitOpen := make(map[uint32]*pb3.YYStateInfo)
	for id, line := range jsondata.YunYingConfMgr {
		obj, ok := mgr.objMap[id]
		if ok && nil != obj && obj.IsOpen() {
			continue
		}
		if info := mgr.calcWaitOpen(id, line); nil != info {
			waitOpen[id] = info
		}
	}
	if player != nil {
		player.SendProto3(40, 3, &pb3.S2C_40_3{WaitOpenInfo: waitOpen})
	} else {
		engine.Broadcast(chatdef.CIWorld, 0, 40, 3, &pb3.S2C_40_3{WaitOpenInfo: waitOpen}, 0)
	}
}

func (mgr *YYMgr) calcWaitOpen(id uint32, conf *jsondata.YunYingConf) *pb3.YYStateInfo {
	if mgr.isOpenThisGroup(conf.Group) {
		return nil
	}
	nowSec := time_util.NowSec()
	pfId := engine.GetPfId()
	for _, line := range conf.Range {
		if len(line.PfIds) > 0 && !utils.SliceContainsUint32(line.PfIds, pfId) {
			continue
		}

		if !mgr.inRange(line) {
			continue
		}

		for _, timeConf := range line.TimeConf {
			parser, ok := TimeHandler[timeConf.TimeType]
			if !ok {
				continue
			}
			start, end := parser(timeConf)
			if nowSec < start {
				return &pb3.YYStateInfo{
					ActId:     id,
					StartTime: start,
					EndTime:   end,
					ConfIdx:   timeConf.ConfIdx,
				}
			}
		}
	}
	return nil
}

func (mgr *YYMgr) CloseYY(id uint32, isInit bool) {
	obj, ok := mgr.objMap[id]
	yyInfo := mgr.GetYYInfo()
	if ok {
		delete(mgr.objMap, id)
		obj.OnEnd()
		obj.ResetData()
		if !isInit {
			obj.Broadcast(40, 1, &pb3.S2C_40_1{Id: id})
		}

		delete(yyInfo, id)
		logger.LogInfo("结算活动 id:%d, isInit:%t, start:%d-end:%d", id, isInit, obj.GetOpenTime(), obj.GetEndTime())
	}
}

func (mgr *YYMgr) inRange(conf *jsondata.YunYingRangeConf) bool {
	flag := true

	var checkSpecDay = func(param1, param2 uint32) bool {
		if param1 == 0 || param2 == 0 {
			flag = false
		}

		if param2 < param1 {
			return false
		}

		start, err := time.Parse("20060102", fmt.Sprintf("%d", param1))
		if err != nil {
			logger.LogError("%d %d failed", param1, param2)
			return false
		}

		end, err := time.Parse("20060102", fmt.Sprintf("%d", param2))
		if err != nil {
			logger.LogError("%d %d failed", param1, param2)
			return false
		}

		now := time.Now()
		if !now.After(start) {
			return false
		}
		if !now.Before(end) {
			return false
		}
		return true
	}

	for _, line := range conf.Cond {
		switch line.CondType {
		case RangeMergeTimes:
			times := gshare.GetMergeTimes()
			if times < line.Param1 || times > line.Param2 {
				flag = false
			}
		case RangeMergeDays:
			days := gshare.GetMergeSrvDay()
			if days < line.Param1 || days > line.Param2 {
				flag = false
			}
		case RangeOpenDays:
			days := gshare.GetOpenServerDay()
			if days < line.Param1 || days > line.Param2 {
				flag = false
			}
		case RangeOpenYearOfWeek:
			// param1: 循环周期
			// param2: 第几周
			_, week := time.Now().ISOWeek()
			if line.Param1 == 0 || line.Param2 == 0 {
				flag = false
			}
			if flag {
				w := uint32(week) % line.Param1
				if w == 0 {
					w = line.Param1
				}
				if w != line.Param2 {
					flag = false
				}
			}
		case RangeOpenSpecDays:
			if !checkSpecDay(line.Param1, line.Param2) {
				flag = false
			}
		case RangeOpenWeeks:
			weeks := gshare.GetOpenServerWeeks()
			if weeks < line.Param1 || weeks > line.Param2 {
				flag = false
			}
		case RangeOpenSrvOfWeek:
			// param1: 循环周期
			// param2: 第几周
			week := gshare.GetOpenServerWeeks()
			if line.Param1 == 0 || line.Param2 == 0 {
				flag = false
			}
			if flag {
				w := week % line.Param1
				if w == 0 {
					w = line.Param1
				}
				if w != line.Param2 {
					flag = false
				}
			}
		case RangeSecondMergeDays:
			days := gshare.GetMergeSrvDayByTimes(2)
			if days < line.Param1 || days > line.Param2 {
				flag = false
			}
		default:
			return false
		}
		if !flag {
			return false
		}
	}

	return flag
}

func (mgr *YYMgr) beforeMerge() {
	for id, obj := range mgr.objMap {
		if !obj.IsOpen() {
			continue
		}
		yunYingConf := jsondata.GetYunYingConf(id)
		if yunYingConf == nil || yunYingConf.MergeUnClose {
			continue
		}
		utils.ProtectRun(func() {
			mgr.CloseYY(id, false)
		})
	}
}

func (mgr *YYMgr) syncCrossYYInfo() {
	for id, obj := range mgr.objMap {
		if obj != nil {
			if mgr.isCrossYYClass(obj.GetClass()) {
				err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncCrossYYInfo, &pb3.G2CSyncCrossYYInfo{
					Status: &pb3.CrossYYStatus{
						Id:      id,
						OTime:   obj.GetOpenTime(),
						ETime:   obj.GetEndTime(),
						ConfIdx: obj.GetConfIdx(),
					},
					Op: custom_id.CrossYYOpSync,
				})
				if err != nil {
					logger.LogError("err:%v", err)
				}
			}
		}
	}
}

func srvDayParser(day uint32, conf *jsondata.YunYingTimeConf) (uint32, uint32) {
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
	if conf.Loop == -1 || int(nLoop) <= conf.Loop {
		// 无限循环，或者循环未结束
		thisLoopStartDay := sTime + (loopDuration * (nLoop - 1))

		sTime = time_util.GetBeforeDaysZeroTime(day - thisLoopStartDay)
		eTime = uint32(time.Unix(int64(sTime), 0).AddDate(0, 0, int(duration)).Unix())
		return sTime, eTime
	}
	return 0, 0
}

func playerCharge(player iface.IPlayer, args ...interface{}) {
	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok {
		return
	}
	for _, line := range GetYYMgr().objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.PlayerCharge(chargeEvent)
		})
	}
}

func playerUseDiamond(player iface.IPlayer, args ...interface{}) {
	count, ok := args[0].(int64)
	if !ok {
		return
	}

	for _, line := range GetYYMgr().objMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.PlayerUseDiamond(count)
		})
	}
}

func questEvent(player iface.IPlayer, args ...interface{}) {
	qt, ok1 := args[0].(uint32)
	id, ok2 := args[1].(uint32)
	count, ok3 := args[2].(uint32)
	if !ok1 || !ok2 || !ok3 {
		return
	}

	for _, obj := range GetYYMgr().objMap {
		if obj != nil && obj.IsOpen() {
			obj.QuestEvent(qt, id, count)
		}
	}
}

func GmOpenYY(id, sTime, eTime, confIdx uint32, isGm bool) {
	GetYYMgr().OpenYY(id, sTime, eTime, confIdx, false)
}

func GmEndYY(args ...string) {
	yyId := utils.AtoUint32(args[0])
	GetYYMgr().CloseYY(yyId, false)
}

func GmChangeState(id, state uint32, isGm bool) {
}

func SetYYTimeGm(id, startDay uint32) {

}

func Save() {
	gshare.GetStaticVar().GlobalYY = GetYYMgr().YYInfo
	for _, obj := range GetYYMgr().objMap {
		utils.ProtectRun(func() {
			obj.ServerStopSaveData()
		})
	}
}

func On30Min() {
}

// EachAllYYObj 遍历指定模板活动
func EachAllYYObj(class uint32, doLogic func(obj iface.IYunYing)) {
	yyList := GetAllYY(class)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		if v == nil || !v.IsOpen() {
			continue
		}
		doLogic(v)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeBeforeNewDayArrive, func(args ...interface{}) {
		GetYYMgr().BeforeNewDay()
	})

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		GetYYMgr().NewDay()
	})

	event.RegSysEvent(custom_id.SeMerge, func(args ...interface{}) {
		GetYYMgr().beforeMerge()
		GetYYMgr().checkAllOpen(true)
		GetYYMgr().checkAllChainOpen(true)
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		GetYYMgr().OnInit()
		GetYYMgr().checkAllClose(true)
		GetYYMgr().checkAllOpen(true)
		GetYYMgr().checkAllChainOpen(true)
	})

	event.RegActorEvent(custom_id.AeGlobalYYQuest, func(player iface.IPlayer, args ...interface{}) {
		yyType, ok1 := args[0].(uint32)
		fn, ok2 := args[1].(func(sys iface.IQuestTargetSys))
		if !ok1 || !ok2 {
			return
		}
		for _, v := range GetYYMgr().objMap {
			if v.GetClass() == yyType && v.IsOpen() {
				if sys, ok := v.(iface.IQuestTargetSys); ok {
					utils.ProtectRun(func() {
						fn(sys)
					})
				}
			}
		}
	})

	event.RegActorEvent(custom_id.AeUseDiamond, func(player iface.IPlayer, args ...interface{}) {
		playerUseDiamond(player, args...)
	})
	event.RegActorEvent(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		GetYYMgr().OnReconnect(player)
	})
	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		GetYYMgr().OnAfterLogin(player)
	})

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		playerCharge(player, args...)
	})

	event.RegActorEvent(custom_id.AeQuestEvent, questEvent)
	event.RegSysEvent(custom_id.SeCmdGmBeforeMerge, func(args ...interface{}) {
		GetYYMgr().beforeMerge()
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		GetYYMgr().syncCrossYYInfo()
	})
}
