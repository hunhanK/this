package logicworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/activerobot"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/auctionmgr"
	"jjyz/gameserver/logicworker/beasttidemgr"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gm"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/newjingjiemgr"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/logicworker/sectgivegiftmgr"
	"jjyz/gameserver/logicworker/spiritualtraveleventmgr"
	"jjyz/gameserver/logworker"
	"sync/atomic"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/safeworker"
)

var (
	isStop atomic.Bool
)

type logicWorker struct {
	_1sChecker  *time_util.TimeChecker
	_5mTicker   *time_util.TimeChecker
	_30mChecker *time_util.TimeChecker
}

func NewLogicWorker() (worker *safeworker.Worker, err error) {
	st := &logicWorker{}

	router := safeworker.NewRouter(5 * time.Millisecond)
	worker, err = safeworker.NewWorker(
		safeworker.WithName("logicWorker"),
		safeworker.WithLoopFunc(st.singleRun),
		safeworker.WithBeforeLoop(st.onInit),
		safeworker.WithAfterLoop(st.afterLoop),
		safeworker.WithChSize(10000),
		safeworker.WithRouter(router),
	)

	if nil != err {
		logger.LogError("new logic worker error. %v", err)
	}

	gshare.SendGameMsg = worker.SendMsg
	gshare.RegisterGameMsgHandler = router.Register

	return
}

func (st *logicWorker) onInit() {
	st._1sChecker = time_util.NewTimeChecker(time.Second)
	st._5mTicker = time_util.NewTimeChecker(5 * time.Minute)
	st._30mChecker = time_util.NewTimeChecker(30 * time.Minute)
	manager.LoadWorldLevel()
	event.TriggerSysEvent(custom_id.SeServerInit)
}

func (st *logicWorker) singleRun() {
	if isStop.Load() {
		return
	}

	start := time.Now()
	manager.CheckHourBegin()
	timer.RunOne()
	manager.RunOne()

	if st._1sChecker.CheckAndSet(true) {
		series.UpdateTime(uint32(start.Unix()))
		gm.CheckOneSec()
		spiritualtraveleventmgr.RunOne()
		newjingjiemgr.RunOne()
		manager.LogDitchOnlinePlayer(logworker.LogOnline)
		activerobot.RunOne()
		auctionmgr.RunOne()
		sectgivegiftmgr.RunOne()
		manager.Checker1sRunOne()
	}
	if st._5mTicker.CheckAndSet(true) {
		manager.CheckSaveOfflineData()
		guildmgr.Run5Minutes()
		if err := manager.SaveRushRankMgr(false); nil != err {
			logger.LogError("save rush rank mgr error! %v", err)
		}
		if err := manager.SaveAskHelpMgr(false); nil != err {
			logger.LogError("save ask help mgr error! %v", err)
		}
		if err := manager.SavePlayerOnlineMgr(false); nil != err {
			logger.LogError("save ask help mgr error! %v", err)
		}
		beasttidemgr.AutoDonateBeastTide()
		beasttidemgr.AutoDonateDemonSubduing()
	}
	if st._30mChecker.CheckAndSet(true) {
		gshare.SaveStaticVar()
		guildmgr.Save()
		friendmgr.GetIntimacyMgr().Save()
		yymgr.On30Min()

		if err := series.SaveSeries(); nil != err {
			logger.LogError("save actor series error! %v", err)
		}
	}

	if since := time.Since(start).Milliseconds(); since > 1000 {
		logger.LogDebug("logic run timeout %d(million)", since)
	}
}

func (st *logicWorker) afterLoop() {
	logger.LogInfo("-------OnSrvStop------------")
	start := time_util.Now()

	manager.CloseAllPlayer(-1)

	manager.CloseAllPlayer(-1)

	activerobot.Stop()

	waitForSavePlayer()

	saveSrvData(true)

	// 关服的时候标记一下小跨服断开连接
	gshare.ClearSrvStatusFlag(gshare.SrvStatusByConnSmallCross)
	event.TriggerSysEvent(custom_id.SeRegGameSrv)

	logger.LogWarn("保存数据完成,总耗时:%+v", time.Since(start))
}

func waitForSavePlayer() {
	logger.LogInfo("wait for save player......")
	cur := time_util.NowSec()

	for {
		if count := manager.GetAllOnlinePlayerCount(); count <= 0 {
			break
		}
		manager.RunOne()

		now := time_util.NowSec()
		if now > cur {
			logger.LogInfo("wait for save player......")
			cur = now
			manager.CloseAllPlayer(-1)
		}
		time.Sleep(time.Millisecond * 50)
	}
}

func saveSrvData(sync bool) {
	activerobot.SaveMainCityRobotData(sync)
	yymgr.Save()
	gshare.SaveStaticVar()
	manager.CheckSaveOfflineData()
	guildmgr.Save()
	friendmgr.GetIntimacyMgr().Save()
	manager.GRankMgrIns.SaveRank()
	err := manager.SaveRushRankMgr(sync)
	if err != nil {
		logger.LogError("save rush rank err:%v", err)
	}
	err = manager.SaveAskHelpMgr(sync)
	if err != nil {
		logger.LogError("save ask help err:%v", err)
	}
	err = manager.SavePlayerOnlineMgr(sync)
	if err != nil {
		logger.LogError("save ask help err:%v", err)
	}
	if err := series.SaveSeries(); nil != err {
		logger.LogError("save actor series error! %v", err)
	}
	robotmgr.RobotMgrInstance.SyncNewAddRobot2DB()
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgSaveSrvData, func(_ ...interface{}) {
			saveSrvData(false)
		})
	})
}
