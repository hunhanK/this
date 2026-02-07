package sdkworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/safeworker"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"time"
)

type (
	sdkWorker struct{}
)

func NewSdkWorker() (worker *safeworker.Worker, err error) {
	st := sdkWorker{}

	router := safeworker.NewRouter(5 * time.Millisecond)
	worker, err = safeworker.NewWorker(
		safeworker.WithName("sdkWorker"),
		safeworker.WithLoopFunc(st.singleRun),
		safeworker.WithBeforeLoop(st.onInit),
		safeworker.WithChSize(10000),
		safeworker.WithRouter(router))

	if nil != err {
		logger.LogError("new logic worker error. %v", err)
	}

	initMonitor()

	gshare.SendSDkMsg = worker.SendMsg
	gshare.RegisterSDKMsgHandler = router.Register

	return
}

func (st *sdkWorker) onInit() {
	event.TriggerSysEvent(custom_id.SeSDKWorkerInitFinish)
}

func (st *sdkWorker) singleRun() {}
