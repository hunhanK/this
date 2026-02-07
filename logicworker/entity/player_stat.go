/**
 * @Author: zjj
 * @Date: 2024/12/21
 * @Desc:
**/

package entity

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

func OnSaveDailyKillData(player iface.IPlayer, buf []byte) {
	msg := &pb3.DailyStatData{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("【保存今日战斗服统计信息】出错 %v", err)
		return
	}
	data := player.GetBinaryData()
	data.DailyStatData = msg
}

func OnDailyStatDataAeNewDay(player iface.IPlayer, _ ...interface{}) {
	msg := &pb3.DailyStatData{
		DayZeroAt:  time_util.GetDaysZeroTime(0),
		MonKillMap: make(map[uint32]uint32),
	}
	data := player.GetBinaryData()
	data.DailyStatData = msg
	err := player.CallActorFunc(actorfuncid.G2FResetDailyKillData, msg)
	if err != nil {
		player.LogInfo("OnDailyStatDataAeNewDay failed")
		return
	}
}

func init() {
	engine.RegisterActorCallFunc(playerfuncid.SaveDailyKillData, OnSaveDailyKillData)
	event.RegActorEventL(custom_id.AeNewDay, OnDailyStatDataAeNewDay)
}
