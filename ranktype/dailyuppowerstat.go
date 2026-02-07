/**
 * @Author: zjj
 * @Date: 2025/11/28
 * @Desc:
**/

package ranktype

import (
	"jjyz/gameserver/iface"
	"log"
	"sync"
)

type DailyUpPowerStatType = uint32
type DailyUpPowerStatFunc func(player iface.IPlayer) int64

var dupsMu sync.RWMutex

const (
	DailyUpPowerStatTypeTotalPower DailyUpPowerStatType = 1 // 总战力
)

var dailyUpPowerStatFuncMap = map[DailyUpPowerStatType]DailyUpPowerStatFunc{}

func RegDailyUpPowerStatFunc(t DailyUpPowerStatType, f DailyUpPowerStatFunc) {
	dupsMu.Lock()
	defer dupsMu.Unlock()
	_, ok := dailyUpPowerStatFuncMap[t]
	if ok {
		log.Fatalf("already registered daily up power stat func")
	}
	dailyUpPowerStatFuncMap[t] = f
}

func GetDailyUpPowerStatFunc(t DailyUpPowerStatType) DailyUpPowerStatFunc {
	f, ok := dailyUpPowerStatFuncMap[t]
	if !ok {
		return nil
	}
	return f
}
