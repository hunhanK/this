/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:36
 */

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/time_util"
	"jjyz/gameserver/dbworker/cache"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/safeworker"
)

func NewDBWorker() (worker *safeworker.Worker, err error) {
	st := dbWorker{}

	router := safeworker.NewRouter(5 * time.Millisecond)
	worker, err = safeworker.NewWorker(
		safeworker.WithName("dbWorker"),
		safeworker.WithLoopFunc(st.singleRun),
		safeworker.WithBeforeLoop(st.onInit),
		safeworker.WithAfterLoop(st.afterLoop),
		safeworker.WithChSize(10000),
		safeworker.WithRouter(router),
		safeworker.WithSkipAlarm(),
	)

	if nil != err {
		logger.LogError("new logic worker error. %v", err)
	}

	gshare.SendDBMsg = worker.SendMsg
	gshare.RegisterDBMsgHandler = router.Register

	return
}

type dbWorker struct {
	_checker *time_util.TimeChecker
}

func (st *dbWorker) onInit() {
	st._checker = time_util.NewTimeChecker(time.Second)
	event.TriggerSysEvent(custom_id.SeDBWorkerInitFinish)
}

func (st *dbWorker) singleRun() {
	if st._checker.CheckAndSet(true) {
		if err := loadCharge(false); nil != err {
			logger.LogError("load charge error %v", err)
		}

		if err := loadGmCmd(false); nil != err {
			logger.LogError("load gm error %v", err)
		}
	}

	SaveActorCachesToMysql()
}

func (st *dbWorker) afterLoop() {
	saveCacheImme()
}

func SaveActorCachesToMysql() {
	for _, v := range cache.GetActorCaches() {
		if v.Dirty {
			SaveActorData(v.RemoteAddr, v.Pb3Data)
			v.Dirty = false
		}
	}
}

func saveCacheImme(_ ...interface{}) {
	for _, v := range cache.GetActorCaches() {
		if v.Dirty {
			SaveActorData(v.RemoteAddr, v.Pb3Data)
			v.Dirty = false
		}
	}
}
