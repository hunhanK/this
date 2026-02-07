/**
 * @Author: lzp
 * @Date: 2024/7/8
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type MissionScoreSys struct {
	Base
}

func (sys *MissionScoreSys) OnLogin() {
	sys.checkRefresh()
	sys.s2cInfo()
}

func (sys *MissionScoreSys) OnReconnect() {
	sys.checkRefresh()
	sys.s2cInfo()
}

func (sys *MissionScoreSys) OnOpen() {
	data := sys.GetData()
	data.StartTime = time_util.NowSec()
	sys.s2cInfo()
}

func (sys *MissionScoreSys) OnNewDay() {
	data := sys.GetData()
	data.StartTime = time_util.NowSec()
	data.Ids = data.Ids[:0]
	sys.s2cInfo()
}

func (sys *MissionScoreSys) s2cInfo() {
	sys.SendProto3(32, 10, &pb3.S2C_32_10{
		Data: sys.GetData(),
	})
}

func (sys *MissionScoreSys) checkRefresh() {
	now := time_util.NowSec()
	data := sys.GetData()
	if !time_util.IsSameDay(data.StartTime, now) {
		data.StartTime = now
		data.Ids = data.Ids[:0]
	}
}

func (sys *MissionScoreSys) GetData() *pb3.MissionScoreReach {
	if sys.GetBinaryData().MScoreReach == nil {
		sys.GetBinaryData().MScoreReach = &pb3.MissionScoreReach{}
	}

	return sys.GetBinaryData().MScoreReach
}

func (sys *MissionScoreSys) c2sFetchRewards(msg *base.Message) error {
	var req pb3.C2S_32_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.MissionScoreConfMgr
	if conf == nil || conf.Missions == nil || conf.Missions[req.Idx] == nil {
		return neterror.ParamsInvalidError("idx=%d not found", req.Idx)
	}

	mConf := conf.Missions[req.Idx]
	mScoreSt := sys.GetBinaryData().MissionScore
	if mScoreSt == nil || mScoreSt.Point < mConf.TarVal {
		return neterror.ParamsInvalidError("score limit")
	}

	data := sys.GetData()
	if utils.SliceContainsUint32(data.Ids, req.Idx) {
		return neterror.ParamsInvalidError("rewards is get")
	}

	now := time_util.NowSec()
	if data.StartTime+conf.Dur < now {
		return neterror.ParamsInvalidError("fetch limit")
	}

	// 开服两天后无法领取
	if gshare.GetOpenServerDay() > conf.CloseTime {
		return neterror.ParamsInvalidError("fetch limit")
	}

	data.Ids = append(data.Ids, req.Idx)
	if len(mConf.Rewards) > 0 {
		engine.GiveRewards(sys.GetOwner(), mConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogMissionScoreReward,
		})
	}

	sys.SendProto3(32, 11, &pb3.S2C_32_11{
		Ids: data.Ids,
	})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiMissionScore, func() iface.ISystem {
		return &MissionScoreSys{}
	})

	net.RegisterSysProto(32, 11, sysdef.SiMissionScore, (*MissionScoreSys).c2sFetchRewards)
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiMissionScore).(*MissionScoreSys); ok && sys.IsOpen() {
			sys.OnNewDay()
		}
	})
}
