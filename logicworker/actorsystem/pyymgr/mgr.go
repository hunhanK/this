package pyymgr

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

const (
	RangeMergeTimes      = 1 // 合服次数
	RangeMergeDays       = 2 // 合服天数
	RangeOpenDays        = 3 // 开服天数
	RangeOpenYearOfWeek  = 4 // 今年循环到的星期
	PlayerOfflineTime    = 5 // 玩家离线时长
	RangeOpenSpecDays    = 6 // 指定日期
	RangeOpenWeeks       = 7 // 开服周数
	RangeOpenSrvOfWeek   = 8 // 开服循环到的星期
	RangeSecondMergeDays = 9 // 二合持续天数
)

var (
	CreateMap = make(map[uint32]CreateFunc)
)

type (
	CreateFunc func() iface.IPlayerYY
	TimeParse  func(*jsondata.PlayerYYTimeConf) (uint32, uint32)

	YYMgr struct {
		actorsystem.Base

		ObjMap map[uint32]iface.IPlayerYY
		YYInfo *pb3.GlobalPlayerYY
	}
)

func GetPlayerYYStatus(playerId uint64) *pb3.GlobalPlayerYY {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.GlobalPlayerYY {
		globalVar.GlobalPlayerYY = make(map[uint64]*pb3.GlobalPlayerYY)
	}

	data, ok := globalVar.GlobalPlayerYY[playerId]
	if !ok || nil == data {
		data = &pb3.GlobalPlayerYY{}
		globalVar.GlobalPlayerYY[playerId] = data
	}

	if nil == data.Info {
		data.Info = make(map[uint32]*pb3.YYStatus)
	}

	return data
}

func (mgr *YYMgr) OnOpen() {
	mgr.checkAllClose(true)
	mgr.checkAllChainOpen(true)
	mgr.checkAllOpen(true)
	mgr.SendPlayerYYInfo()
}

func (mgr *YYMgr) OnInit() {
	mgr.ObjMap = make(map[uint32]iface.IPlayerYY)

	binaryData := mgr.GetOwner().GetBinaryData()
	if binaryData.YyData == nil {
		binaryData.YyData = &pb3.PlayerYYData{}
	}

	if mgr.YYInfo == nil {
		mgr.YYInfo = GetPlayerYYStatus(mgr.GetOwner().GetId())
	}

	delMap := make(map[uint32]struct{})
	// 加载活动数据
	for id, line := range mgr.YYInfo.Info {
		logger.LogInfo("[玩家活动] 加载, Id:%d, %v", id, line)
		if conf := jsondata.GetPlayerYYConf(id); nil == conf {
			delMap[id] = struct{}{}
			continue
		}

		yyLine := jsondata.GetPlayerYYConf(id)
		obj := mgr.createYYObj(yyLine.Class, id)
		if obj != nil {
			obj.Init(mgr.GetOwner(), line.OTime, line.ETime)

			if line.ConfIdx > 0 {
				obj.SetConfIdx(line.ConfIdx)
			}

			obj.OnInit()

		} else {
			logger.LogError("[玩家活动] 加载失败, id:%d", id)
		}
	}

	for id := range delMap {
		delete(mgr.YYInfo.Info, id)
		logger.LogInfo("玩家：%s 删除没有配置的运营活动：%d", mgr.GetOwner().GetName(), id)
	}
}

func (mgr *YYMgr) OnLogin() {
	mgr.checkAllClose(true)
	mgr.checkAllChainOpen(true)
	mgr.checkAllOpen(true)
	mgr.SendPlayerYYInfo()
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.BugFix()
			line.Login()
		})
	}
}

func (mgr *YYMgr) OnAfterLogin() {
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.OnAfterLogin()
		})
	}
}

func (mgr *YYMgr) OnReconnect() {
	mgr.SendPlayerYYInfo()
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.OnReconnect()
		})
	}
}

func (mgr *YYMgr) OnLoginFight() {
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.OnLoginFight()
		})
	}
}

func (mgr *YYMgr) OnLogout() {
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.OnLogout()
		})
	}
}

func (mgr *YYMgr) BeforeNewDay() {
	mgr.checkAllClose(false)
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.BeforeNewDay()
		})
	}
}

func (mgr *YYMgr) NewDay() {
	for _, line := range mgr.ObjMap {
		utils.ProtectRun(func() {
			if !line.IsOpen() {
				return
			}
			line.NewDay()
			mgr.GetOwner().TriggerQuestEvent(custom_id.QttPYYLoginDays, line.GetId(), 1)
		})
	}

	mgr.checkAllChainOpen(false)
	mgr.checkAllOpen(false)
	mgr.SendWaitOpenList()
}

func (mgr *YYMgr) SendPlayerYYInfo() {
	msg := &pb3.S2C_40_252{}

	for _, line := range mgr.ObjMap {
		msg.Infos = append(msg.Infos, line.GetYYStateInfo())
	}

	mgr.SendProto3(40, 252, msg)
	mgr.SendWaitOpenList()
}

func (mgr *YYMgr) createYYObj(class, id uint32) iface.IPlayerYY {
	fn, ok := CreateMap[class]
	if !ok {
		return nil
	}

	if obj, ok := mgr.ObjMap[id]; ok {
		logger.LogWarn("玩家活动已开启 id:%d, sTime:%s, eTime:%s",
			id, time_util.SecToTimeStr(obj.GetOpenTime()), time_util.SecToTimeStr(obj.GetEndTime()))
		return nil
	}

	obj := fn()
	obj.SetId(id)
	mgr.ObjMap[id] = obj

	return obj
}

func (mgr *YYMgr) OpenYY(id, sTime, eTime, confIdx, timeType uint32, isInit bool) {
	conf := jsondata.GetPlayerYYConf(id)
	if nil == conf {
		return
	}

	obj := mgr.createYYObj(conf.Class, id)
	if nil == obj {
		return
	}

	obj.Init(mgr.GetOwner(), sTime, eTime)

	if confIdx > 0 {
		obj.SetConfIdx(confIdx)
	}

	obj.OnInit()

	if !isInit {
		mgr.sendOneYYInfo(id)
	}

	obj.ResetData()
	obj.OnOpen()

	logworker.LogPlayerBehavior(mgr.GetOwner(), pb3.LogId_LogPYYOpen, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d,%d,%d,%d", sTime, eTime, confIdx, timeType),
	})

	switch timeType {
	case MergeSrvParser:
		obj.MergeFix()
	case CmdYYOpenParser:
		obj.CmdYYFix()
	}

	mgr.GetOwner().TriggerEvent(custom_id.AePyyOpen, id)
	mgr.GetOwner().TriggerQuestEvent(custom_id.QttPYYLoginDays, id, 1)
	mgr.YYInfo.Info[id] = &pb3.YYStatus{
		OTime:   sTime,
		ETime:   eTime,
		ConfIdx: obj.GetConfIdx(),
		Class:   conf.Class,
	}
}

func (mgr *YYMgr) checkAllChainOpen(isInit bool) {
	pyyPaths := jsondata.GetChainOpenPYYPath()
	if nil == pyyPaths || len(pyyPaths) == 0 {
		return
	}
	var canOpenPaths []pie.Uint32s
	for _, paths := range pyyPaths {
		var canOpen = true
		for _, id := range paths {
			obj, ok := mgr.ObjMap[id]
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
			line := jsondata.PlayerYYConfMgr[id]
			if line == nil {
				return
			}
			// 避免重复开
			obj, ok := mgr.ObjMap[id]
			if ok && nil != obj && obj.IsOpen() {
				return
			}
			mgr.checkOpen(id, line, isInit)
		})
	}
}

func (mgr *YYMgr) checkAllOpen(isInit bool) {
	if nil == jsondata.PlayerYYConfMgr {
		return
	}

	for id, line := range jsondata.PlayerYYConfMgr {
		// 需要链式开启的 跳过
		if jsondata.InChainOpenPYY(id) {
			continue
		}
		obj, ok := mgr.ObjMap[id]
		if ok && nil != obj && obj.IsOpen() {
			continue
		}
		mgr.checkOpen(id, line, isInit)
	}

	mgr.GetOwner().TriggerEvent(custom_id.AeCheckCmdYYOpen, isInit)
}

func (mgr *YYMgr) isOpenThisGroup(group uint32) bool {
	if group == 0 {
		return false
	}
	for id, _ := range mgr.ObjMap {
		if jsondata.GetPlayerYYGroup(id) == group {
			return true
		}
	}
	return false
}

func (mgr *YYMgr) checkOpen(id uint32, conf *jsondata.PlayerYYConf, isInit bool) {
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
				mgr.OpenYY(id, start, end, timeConf.ConfIdx, timeConf.TimeType, false)
				return
			}
		}
	}
}

func (mgr *YYMgr) checkAllClose(isInit bool) {
	now := time_util.NowSec()
	for id, obj := range mgr.ObjMap {
		if obj.GetEndTime() <= now {
			utils.ProtectRun(func() {
				mgr.CloseYY(id, isInit)
			})
		}
	}
}

func (mgr *YYMgr) SendWaitOpenList() {
	if nil == jsondata.PlayerYYConfMgr {
		return
	}
	waitOpen := make(map[uint32]*pb3.YYStateInfo)
	for id, line := range jsondata.PlayerYYConfMgr {
		obj, ok := mgr.ObjMap[id]
		if ok && nil != obj && obj.IsOpen() {
			continue
		}
		if info := mgr.calcWaitOpen(id, line); nil != info {
			waitOpen[id] = info
		}
	}
	mgr.SendProto3(40, 240, &pb3.S2C_40_240{WaitOpenInfo: waitOpen})
}

func (mgr *YYMgr) calcWaitOpen(id uint32, conf *jsondata.PlayerYYConf) *pb3.YYStateInfo {
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
	obj, ok := mgr.ObjMap[id]
	if ok {
		delete(mgr.ObjMap, id)
		obj.OnEnd()
		obj.ResetData()
		if !isInit {
			mgr.SendProto3(40, 251, &pb3.S2C_40_251{Id: id})
		}

		delete(mgr.YYInfo.Info, id)
		logworker.LogPlayerBehavior(mgr.GetOwner(), pb3.LogId_LogPYYEnd, &pb3.LogPlayerCounter{
			NumArgs: uint64(id),
		})
		logger.LogInfo("玩家:%d,结算活动 id:%d, isInit:%t, start:%d-end:%d", mgr.GetOwner().GetId(), id, isInit, obj.GetOpenTime(), obj.GetEndTime())
	}
}

func (mgr *YYMgr) inRange(conf *jsondata.PlayerYYRangeConf) bool {
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
		end = end.Add((86400 - 1) * time.Second)

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
		case PlayerOfflineTime:
			lastLogoutTime := mgr.GetOwner().GetMainData().GetLastLogoutTime()
			nowSec := time_util.NowSec()
			if lastLogoutTime == 0 || lastLogoutTime > nowSec {
				return false
			}
			offlineHour := (nowSec - lastLogoutTime) / 3600
			if offlineHour < line.Param1 || (line.Param2 > 0 && offlineHour > line.Param2) {
				flag = false
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

func (mgr *YYMgr) sendOneYYInfo(id uint32) {
	obj, _ := mgr.ObjMap[id]
	if obj != nil {
		msg := &pb3.S2C_40_250{Info: obj.GetYYStateInfo()}
		mgr.SendProto3(40, 250, msg)
	}
}

func (mgr *YYMgr) GetAllObj(class uint32) []iface.IPlayerYY {
	list := make([]iface.IPlayerYY, 0, 5)
	for _, v := range mgr.ObjMap {
		if v.GetClass() == class {
			list = append(list, v)
		}
	}
	return list
}

func (mgr *YYMgr) GetObjById(id uint32) iface.IPlayerYY {
	for _, v := range mgr.ObjMap {
		if v.GetId() == id {
			return v
		}
	}
	return nil
}

func RegPlayerYY(class uint32, fn func() iface.IPlayerYY) {
	CreateMap[class] = fn
}

// initPlayerYYData 初始化一下全局的玩家活动状态列表
func initPlayerYYData(args ...interface{}) {
	idMap, ok := args[0].(map[uint64]struct{})
	if !ok {
		return
	}

	for id, _ := range idMap {
		if !manager.IsActorActive(id) {
			continue
		}

		GetPlayerYYStatus(id)
	}
}

// 处理合服需要关闭的活动
func handleMerge(_ ...interface{}) {
	globalVar := gshare.GetStaticVar()
	nowSec := time_util.NowSec()

	for _, line := range globalVar.GlobalPlayerYY {
		for yyId, yyData := range line.Info {
			conf := jsondata.GetPlayerYYConf(yyId)
			if !conf.MergeClose {
				continue
			}
			if nowSec >= yyData.OTime && nowSec < yyData.ETime {
				yyData.ETime = nowSec
			}
		}
	}
}

func playerCharge(player iface.IPlayer, args ...interface{}) {
	mgr, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return
	}

	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok {
		return
	}

	for _, line := range mgr.ObjMap {
		if line == nil || !line.IsOpen() {
			continue
		}
		if !custom_id.IsRealChargeLog(chargeEvent.LogId) && line.IsUseRealCharge() {
			continue
		}
		utils.ProtectRun(func() {
			line.PlayerCharge(chargeEvent)
		})
	}
}

func playerUseDiamond(player iface.IPlayer, args ...interface{}) {
	count, ok := args[0].(int64)
	if !ok {
		return
	}

	mgr, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return
	}

	for _, line := range mgr.ObjMap {
		if nil != line && line.IsOpen() {
			utils.ProtectRun(func() {
				line.PlayerUseDiamond(count)
			})
		}
	}
}

func questEvent(player iface.IPlayer, args ...interface{}) {
	qt, ok1 := args[0].(uint32)
	id, ok2 := args[1].(uint32)
	count, ok3 := args[2].(uint32)
	if !ok1 || !ok2 || !ok3 {
		return
	}

	mgr, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return
	}

	for _, obj := range mgr.ObjMap {
		if obj != nil && obj.IsOpen() {
			obj.QuestEvent(qt, id, count)
		}
	}
}

func GetPlayerYYObj(player iface.IPlayer, id uint32) iface.IPlayerYY {
	sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return nil
	}

	return sys.GetObjById(id)
}

func GetPlayerAllYYObj(player iface.IPlayer, class uint32) []iface.IPlayerYY {
	sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return nil
	}

	return sys.GetAllObj(class)
}

// EachPlayerAllYYObj 遍历指定模板活动
func EachPlayerAllYYObj(player iface.IPlayer, class uint32, doLogic func(obj iface.IPlayerYY)) {
	yyList := GetPlayerAllYYObj(player, class)
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

func DelDataByPlayerIds(ids map[uint64]struct{}) {
	globalVar := gshare.GetStaticVar()
	for id := range ids {
		delete(globalVar.GlobalPlayerYY, id)
	}
}

func gmEndAll(player iface.IPlayer, args ...string) bool {
	mgr, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
	if !ok {
		return false
	}

	for id := range mgr.ObjMap {
		mgr.CloseYY(id, false)
	}

	return true
}

func init() {
	actorsystem.RegisterSysClass(sysdef.SiPlayerYY, func() iface.ISystem {
		return &YYMgr{}
	})

	event.RegActorEvent(custom_id.AeBeforeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
		if ok {
			sys.BeforeNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
		if ok {
			sys.NewDay()
		}
	})

	event.RegActorEvent(custom_id.AeUseDiamond, func(player iface.IPlayer, args ...interface{}) {
		playerUseDiamond(player, args...)
	})

	event.RegActorEvent(custom_id.AeYYQuest, func(player iface.IPlayer, args ...interface{}) {
		yyType, ok1 := args[0].(uint32)
		fn, ok2 := args[1].(func(sys iface.IQuestTargetSys))
		if !ok1 || !ok2 {
			return
		}
		mgr, ok := player.GetSysObj(sysdef.SiPlayerYY).(*YYMgr)
		if !ok {
			return
		}
		for _, v := range mgr.ObjMap {
			if v.GetClass() == yyType && v.IsOpen() {
				if sys, ok := v.(iface.IQuestTargetSys); ok {
					utils.ProtectRun(func() {
						fn(sys)
					})
				}
			}
		}
	})

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		playerCharge(player, args...)
	})

	event.RegActorEventL(custom_id.AeQuestEvent, questEvent)
	event.RegSysEventL(custom_id.SeOfflineDataLoadSucc, initPlayerYYData)
	event.RegSysEvent(custom_id.SeMerge, handleMerge)

	gmevent.Register("pyy.endAll", gmEndAll, 1)
}
