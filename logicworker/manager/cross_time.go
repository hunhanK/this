/**
 * @Author: HeXinLi
 * @Desc:
 * @Date: 2022/4/8 16:44
 */

package manager

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"time"
)

func PackServerInfo() *pb3.S2C_2_6 {
	_, offset := time.Now().Zone()
	rsp := &pb3.S2C_2_6{
		OpenDay:              gshare.GetOpenServerDay(),
		MergeDay:             gshare.GetMergeSrvDay(),
		Time:                 time_util.Now().UnixMilli(),
		OpenTime:             gshare.GetOpenServerTime(),
		MergeTimes:           gshare.GetMergeTimes(),
		MergeTime:            gshare.GetStaticVar().GetMergeTimestamp(),
		CrossDay:             gshare.GetSmallCrossDay(),
		CrossAllocTimes:      gshare.GetCrossAllocTimes(),
		SmallCrossWorldLevel: gshare.GetSmallCrossWorldLevel(),
		TimeZoneOffset:       int32(offset),
		WorldLevel:           gshare.GetWorldLevel(),
		CrossMatchSrvNum:     gshare.GetMatchSrvNum(),
		PfId:                 engine.GetPfId(),
		SrvId:                engine.GetServerId(),
		MediumCrossDay:       gshare.GetMediumCrossDay(),
		OpenWeek:             gshare.GetOpenServerWeeks(),
		MergeData:            gshare.GetStaticVar().GetMergeData(),
	}
	return rsp
}

func onCrossSrvConnSucc(args ...interface{}) {
	engine.Broadcast(chatdef.CIWorld, 0, 2, 6, PackServerInfo(), 0)
}

func init() {
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, onCrossSrvConnSucc)
	event.RegSysEvent(custom_id.SeRefreshSmallCrossWorldLevel, func(args ...interface{}) {
		engine.Broadcast(chatdef.CIWorld, 0, 2, 6, PackServerInfo(), 0)
	})
}
