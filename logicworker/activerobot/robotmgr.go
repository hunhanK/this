/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2022/5/27 20:23
 */

package activerobot

import (
	"github.com/emirpasic/gods/trees/binaryheap"
	"github.com/emirpasic/gods/utils"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	series2 "jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"time"
)

var (
	robotMap         = make(map[uint64]*Robot) // 机器人列表 先不做移除
	onlineLiveHeap   *binaryheap.Heap
	_1mChecker       *time_util.TimeChecker
	_putRobotChecker *time_util.TimeChecker
)

func checkOpenMainCityRobotCand() bool {
	openCand := jsondata.GetMainCityRobotConfOpenCand()
	if openCand == nil {
		logger.LogTrace("开服第%d天没有配制投放主线机器人开启时间", gshare.GetOpenServerDay())
		return false
	}

	startAt := gshare.GetOpenServerTime() + openCand.OpenSecond
	endAt := gshare.GetOpenServerTime() + openCand.EndSecond
	nowSec := time_util.NowSec()

	// 没到开启时间
	if startAt > nowSec {
		return false
	}

	// 到了结束时间
	if endAt < nowSec {
		return false
	}

	// 等级榜入榜人数
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeLevel)
	if uint32(rank.GetRankCount()) < openCand.LevRankTotal {
		logger.LogTrace("等级榜不满足%d < %d", uint32(rank.GetRankCount()), openCand.LevRankTotal)
		return false
	}

	// 玩家总数
	totalActor := manager.GetAllOnlinePlayerCount() + manager.GetOfflineCount()
	if totalActor < openCand.RegActorTotal {
		logger.LogTrace("玩家总数不满足%d < %d", totalActor, openCand.RegActorTotal)
		return false
	}

	putCandMap := jsondata.GetMainCityRobotConfPutCand()
	cand := putCandMap[gshare.GetOpenServerDay()]
	if cand == nil {
		logger.LogTrace("开服第%d天没有配制投放主线机器人", gshare.GetOpenServerDay())
		return false
	}
	return true
}

// GetRobotById 获取机器人
func GetRobotById(robotId uint64) *Robot {
	return robotMap[robotId]
}

// 生成在线堆
func genOnlineLiveHeap() *binaryheap.Heap {
	return binaryheap.NewWith(func(a, b interface{}) int {
		aId := a.(uint64)
		bId := b.(uint64)
		return utils.UInt32Comparator(robotMap[aId].liveTime, robotMap[bId].liveTime)
	})
}

func Stop() {
	clearRobotTimeChecker()
	// 同步战斗服
	checkSync2Fight()
}

// 重置管理器
func resetMainCityRobotMgr() {
	clearRobotTimeChecker()
	// 更新投放配制
	putCandMap := jsondata.GetMainCityRobotConfPutCand()
	cand := putCandMap[gshare.GetOpenServerDay()]
	if cand == nil {
		return
	}

	onlineLiveHeap = genOnlineLiveHeap()
	_1mChecker = time_util.NewTimeChecker(time.Minute)
	_putRobotChecker = time_util.NewTimeChecker(time.Duration(cand.Interval) * time.Second)
}

// 登出所有在线机器人
func logoutAllOnlineRobots() {
	for _, robot := range robotMap {
		if !robot.IsOnline() {
			continue
		}
		robot.Logout()
	}
	if onlineLiveHeap == nil {
		return
	}
	onlineLiveHeap.Clear()
}

// 离线检查
func checkLiveTimeOut() {
	for i := 0; i < onlineLiveHeap.Size(); i++ {
		if onlineLiveHeap.Size() == 0 {
			break
		}
		val, ok := onlineLiveHeap.Peek()
		for !ok {
			onlineLiveHeap.Pop()
			val, ok = onlineLiveHeap.Peek()
		}
		robotId := val.(uint64)
		robot := robotMap[robotId]
		if time_util.NowSec() < robot.liveTime {
			break
		}
		robot.Logout()
		onlineLiveHeap.Pop()
	}
}

// 投放机器人
func putRobot() {
	putCandMap := jsondata.GetMainCityRobotConfPutCand()
	cand := putCandMap[gshare.GetOpenServerDay()]
	if cand == nil {
		logger.LogTrace("开服第%d天没有配制投放主线机器人", gshare.GetOpenServerDay())
		return
	}

	if cand.OnlineTotal <= uint32(onlineLiveHeap.Size()) {
		logger.LogTrace("机器人在线人数达到上限%d", onlineLiveHeap.Size())
		return
	}

	normalTotal := cand.OnlineTotal * cand.Normal / 10000
	vipTotal := cand.OnlineTotal * cand.Vip / 10000

	var curNormalTotal, curVipTotal uint32
	iterator := onlineLiveHeap.Iterator()
	iterator.NextTo(func(index int, value interface{}) bool {
		robotId := value.(uint64)
		robot := robotMap[robotId]
		if robot.data.Vip > 0 {
			curVipTotal++
		} else {
			curNormalTotal++
		}
		return false
	})

	var vipList []uint64
	var normalList []uint64
	for _, robot := range robotMap {
		if robot.IsOnline() {
			continue
		}
		if robot.data.Vip > 0 {
			vipList = append(vipList, robot.robotId)
		} else {
			normalList = append(normalList, robot.robotId)
		}
	}

	// 随机一下出生什么类型的机器人
	var randomList []int8
	if normalTotal > curNormalTotal && len(normalList) > 0 {
		randomList = append(randomList, custom_id.NormalMainCityRobot)
	}
	if vipTotal > curVipTotal && len(vipList) > 0 {
		randomList = append(randomList, custom_id.VipMainCityRobot)
	}
	if len(randomList) == 0 {
		logger.LogTrace("暂无投放机器人的资源")
		return
	}
	val := randomList[random.Interval(0, len(randomList)-1)]
	var robotId uint64
	switch val {
	case custom_id.NormalMainCityRobot:
		robotId = normalList[random.Interval(0, len(normalList)-1)]
	case custom_id.VipMainCityRobot:
		robotId = vipList[random.Interval(0, len(vipList)-1)]
	}

	// 出生
	if robot := GetRobotById(robotId); nil != robot {
		robot.Login()
		robot.attrSys.DoUpdate()
		onlineLiveHeap.Push(robot.robotId)
	}
}

// 1. 清理定时器
// 2. 登出所有在线机器人
func clearRobotTimeChecker() {
	_1mChecker = nil
	_putRobotChecker = nil
	logoutAllOnlineRobots()
}

// 加载机器人返回
func onLoadMainCityRobotDataRet(args ...interface{}) {
	if len(args) != 1 {
		logger.LogError("onLoadMainCityRobotDataRet args len error")
		return
	}
	modelMainCityRobotDataList := args[0].([]*pb3.MainCityRobotData)

	// 组装数据
	var makeMainCityRobotData = func(conf *jsondata.MainCityRobotRobotPool) *pb3.MainCityRobotData {
		series, _ := series2.GetActorIdSeries(engine.GetServerId())
		allocSeries, _ := base.MakeMainCityRobotId(engine.GetPfId(), engine.GetServerId(), series)
		return &pb3.MainCityRobotData{
			Id:      allocSeries,
			ConfId:  uint64(conf.Id),
			Name:    conf.Name,
			Vip:     conf.Vip,
			Job:     conf.Job,
			Sex:     conf.Sex,
			TitleId: conf.TitleId,
		}
	}

	// 首次加载 需要创建所有机器人
	var insert = len(modelMainCityRobotDataList) == 0
	if insert {
		robotPool := jsondata.GetMainCityRobotConfNormalRobotPool()
		for _, v := range robotPool {
			data := makeMainCityRobotData(v)
			modelMainCityRobotDataList = append(modelMainCityRobotDataList, data)
		}
		robotPool = jsondata.GetMainCityRobotConfVipRobotPool()
		for _, v := range robotPool {
			data := makeMainCityRobotData(v)
			modelMainCityRobotDataList = append(modelMainCityRobotDataList, data)
		}
	}

	for _, dbRecord := range modelMainCityRobotDataList {
		newRobot := NewRobot(dbRecord.Id, dbRecord)
		newRobot.sysMgr.OnInit()
		newRobot.sysMgr.OnLoadFinish()
		robotMap[dbRecord.Id] = newRobot
	}

	modelMainCityRobotDataList = nil
}

// SaveMainCityRobotData 保存机器人数据
func SaveMainCityRobotData(sync bool) {
	var modelMainCityRobotDataList []*pb3.MainCityRobotData

	for _, robot := range robotMap {
		if !robot.Change() {
			continue
		}
		robot.SetChange(false)
		robotData := robot.data.Copy()
		modelMainCityRobotDataList = append(modelMainCityRobotDataList, robotData)
	}

	if !sync {
		gshare.SendDBMsg(custom_id.GMsgSaveMainCityRobotData, modelMainCityRobotDataList)
		return
	}

	syncMsg := syncmsg.NewSyncMsg(modelMainCityRobotDataList)
	gshare.SendDBMsg(custom_id.GMsgSyncSaveMainCityRobotData, syncMsg)
	_, err := syncMsg.Ret()
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	return
}

func RunOne() {
	if !checkOpenMainCityRobotCand() {
		return
	}

	// 机器人存活检查
	checkLiveTimeOut()

	// 机器人投放检查
	if _putRobotChecker.CheckAndSet(false) {
		putRobot()
	}

	// 保存入库
	if _1mChecker.CheckAndSet(false) {
		// 在线才更新机器人数据
		for _, robot := range robotMap {
			if !robot.IsOnline() {
				continue
			}
			robot.sysMgr.DoUpdate()
			robot.attrSys.DoUpdate()
		}
		SaveMainCityRobotData(false)
	}

	// 同步战斗服
	checkSync2Fight()
}

func onNewDay(_ ...interface{}) {
	resetMainCityRobotMgr()
}

func onServerInit(_ ...interface{}) {
	resetMainCityRobotMgr()
	gshare.RegisterGameMsgHandler(custom_id.GMsgLoadMainCityRobotDataRet, onLoadMainCityRobotDataRet)
	gshare.SendDBMsg(custom_id.GMsgLoadMainCityRobotData)
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, onServerInit)
	event.RegSysEvent(custom_id.SeNewDayArrive, onNewDay)

	engine.GetRobotById = func(id uint64) iface.IRobot {
		if robot := GetRobotById(id); robot != nil {
			return robot
		}
		return nil
	}

	engine.RegisterRobotViewFunc(common.ViewPlayerBasic, func(et iface.IRobot, rsp *pb3.DetailedRoleInfo) {
		robot, ok := et.(*Robot)
		if !ok {
			return
		}
		rsp.Basic.Id = robot.GetRobotId()
		rsp.Basic.Name = robot.GetName()
		rsp.Basic.GuildName = robot.GetGuildName()
		rsp.Basic.Lv = uint32(robot.GetAttr(attrdef.Level))
		rsp.Basic.Circle = uint32(robot.GetAttr(attrdef.Circle))
		rsp.Basic.VipLv = uint32(robot.GetAttr(attrdef.VipLevel))
		rsp.Basic.Job = uint32(robot.GetAttr(attrdef.Job)) >> base.SexBit
		rsp.Basic.Sex = uint32(robot.GetAttr(attrdef.Job)) & (1 << base.SexBit)
		rsp.Basic.GuildId = robot.GetGuildId()
		rsp.Basic.LastLogoutTime = robot.GetLastLogoutTime()
		rsp.Basic.BubbleFrame = robot.GetBubbleFrame()
		rsp.Basic.HeadFrame = robot.GetHeadFrame()
		rsp.Basic.LoginTime = robot.GetLoginTime()
		rsp.Basic.Head = robot.GetHead()
		rsp.Basic.Power = uint64(robot.GetAttr(attrdef.FightValue))
		rsp.Basic.PowerCompare = robot.getPowerCompare()

		// 打包属性
		rsp.Basic.AppearInfo = map[uint32]*pb3.SysAppearSt{
			appeardef.AppearPos_Cloth: {
				SysId:    appeardef.AppearSys_Fashion,
				AppearId: robot.GetData().ClothesId,
			},
			appeardef.AppearPos_Weapon: {
				SysId:    appeardef.AppearSys_Weapon,
				AppearId: robot.GetData().WeaponId,
			},
		}
		rsp.FightProp = make(map[uint32]int64)
		robot.attrSys.PackFightAttr(rsp.FightProp)
	})

	engine.RegisterSysCall(sysfuncid.C2GSyncMainCityRobotFightValue, func(buf []byte) {
		var req pb3.SyncMainCityRobotFightAndAttr
		if err := pb3.Unmarshal(buf, &req); err != nil {
			logger.LogError("err:%v", err)
			return
		}
		robot, ok := robotMap[req.RobotId]
		if !ok {
			return
		}
		robot.GetData().FightValue = req.Data.FightValue
		robot.SetAttr(attrdef.FightValue, req.Data.FightValue)
	})

	gmevent.Register("loginAllMainCityRobot", func(player iface.IPlayer, args ...string) bool {
		for _, robot := range robotMap {
			if robot.IsOnline() {
				continue
			}
			robot.Login()
			robot.attrSys.DoUpdate()
			if onlineLiveHeap == nil {
				onlineLiveHeap = genOnlineLiveHeap()
			}
			onlineLiveHeap.Push(robot.robotId)
		}
		// 同步战斗服
		checkSync2Fight()
		return true
	}, 1)

	gmevent.Register("logoutAllMainCityRobot", func(player iface.IPlayer, args ...string) bool {
		for _, robot := range robotMap {
			if !robot.IsOnline() {
				continue
			}
			robot.Logout()
		}
		// 同步战斗服
		checkSync2Fight()
		onlineLiveHeap.Clear()
		return true
	}, 1)
}
