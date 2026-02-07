package spiritualtraveleventmgr

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

func RunOne() {
	conf, ok := jsondata.GetSpiritualTravelEventConf()
	if !ok {
		logger.LogError("not found spiritual travel event conf")
		return
	}
	if conf.PubRate == 0 {
		logger.LogError("not found pubRate %d", conf.PubRate)
		return
	}

	nowSec := time_util.NowSec()
	if gshare.GetOpenServerTime() == 0 {
		logger.LogError("not found open server time")
		return
	}

	nowTimeAt := nowSec - gshare.GetOpenServerTime()
	if nowTimeAt == 0 {
		return
	}

	v1 := nowTimeAt % conf.PubRate
	if v1 != 0 {
		return
	}

	// 突然想到的奇怪判断
	v2 := nowTimeAt / conf.PubRate
	if v2*conf.PubRate != nowTimeAt {
		return
	}

	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		logger.LogInfo("actor[%d] TriggerEvent, curTime is %d", player.GetId(), nowSec)
		player.TriggerEvent(custom_id.AeSpiritualTravelEventPush)
	})

	logger.LogInfo("cur time is %d,next pub event time is %d", nowSec, nowSec+conf.PubRate)
}
