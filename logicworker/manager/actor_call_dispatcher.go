package manager

import (
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"time"

	"github.com/gzjjyz/logger"
)

func onActorCallPlayerFunc(args ...interface{}) {
	if !gcommon.CheckArgsCount("onActorCallPlayerFunc", 1, len(args)) {
		return
	}
	data, ok := args[0].([]byte)
	if !ok {
		return
	}
	var st pb3.CallPlayerFunc

	if nil != pb3.Unmarshal(data, &st) {
		return
	}

	actorId := st.ActorId
	actor := GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}

	fn := engine.GetActorCallFunc(uint16(st.FnId))
	if nil == fn {
		logger.LogError("未注册的ActorCall方法, 方法id:%d", st.FnId)
		return
	}

	start := time.Now()
	fn(actor, st.Data)
	duration := time.Since(start)
	millisecond := duration.Milliseconds()
	if millisecond >= 2 {
		logger.LogWarn("onActorCallPlayerFunc time exceeds two milliseconds!! %v (%d)", duration, st.FnId)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgActorCallPlayerFunc, onActorCallPlayerFunc)
	})
}
