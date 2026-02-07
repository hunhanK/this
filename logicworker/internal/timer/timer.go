package timer

import (
	"time"

	"jjyz/base/time_util"
)

// note: 非协程安全, 请只在logicworker主协程中调用
var timeHeap *time_util.TimerHeap

func init() {
	timeHeap = time_util.NewTimerHeap()
}

// SetTimeout 延时回调
func SetTimeout(duration time.Duration, cb func()) *time_util.Timer {
	return timeHeap.SetTimeout(duration, cb)
}

// SetInterval 循环回调
func SetInterval(duration time.Duration, cb func()) *time_util.Timer {
	return timeHeap.SetInterval(duration, cb)
}

func RunOne() {
	timeHeap.RunOne()
}
