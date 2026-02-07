package manager

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"time"

	"github.com/gzjjyz/logger"
)

var (
	_lastYearDay  int = time.Now().YearDay()
	_lastHourTime int = time.Now().Hour()
)

func CheckHourBegin() {
	h := time_util.Now().Hour()
	if _lastHourTime == h {
		return
	}
	_lastHourTime = h
	if 0 == _lastHourTime {
		_lastYearDay = time_util.Now().YearDay()
		newDayArrive()
	}
	event.TriggerSysEvent(custom_id.SeHourArrive, h)
	logger.LogInfo("server new hour：%d", h)
	AllOnlinePlayerDo(func(actor iface.IPlayer) {
		actor.NewHourArrive()
	})
}

// 跨天
func newDayArrive() {
	engine.Broadcast(chatdef.CIWorld, 0, 1, 12, &pb3.S2C_1_12{}, 0)

	event.TriggerSysEvent(custom_id.SeBeforeNewDayArrive)
	engine.ClearChargeMap()

	//周一
	if time_util.Now().Weekday() == time.Monday {
		event.TriggerSysEvent(custom_id.SeNewWeekArrive)
		logger.LogInfo("server new week!!!")
	}

	event.TriggerSysEvent(custom_id.SeNewDayArrive)

	AllOnlinePlayerDo(func(actor iface.IPlayer) {
		actor.CheckNewDay(true)
	})

	gshare.SendDBMsg(custom_id.GMsgClearChargeOrder)
}
