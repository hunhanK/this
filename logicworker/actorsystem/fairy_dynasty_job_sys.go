package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	fairydynastyjobrank "jjyz/gameserver/logicworker/fairyDynastyJobRank"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

// TODO 跨服合服清空
type FairyDynastyJobSys struct {
	Base
	data *pb3.FairyDynastyJobData
}

func (sys *FairyDynastyJobSys) init() {
	if nil == sys.GetBinaryData().FairyDynastyJobData {
		sys.GetBinaryData().FairyDynastyJobData = &pb3.FairyDynastyJobData{}
	}

	sys.data = sys.GetBinaryData().FairyDynastyJobData
}

func (sys *FairyDynastyJobSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *FairyDynastyJobSys) OnOpen() {
	sys.init()
	sys.sendInfo()
}

func (sys *FairyDynastyJobSys) OnReconnect() {
	sys.sendInfo()
	sys.checkLastRewardAt()
}

func (sys *FairyDynastyJobSys) OnLogin() {
	sys.sendInfo()
	sys.checkLastRewardAt()
}

func (sys *FairyDynastyJobSys) checkLastRewardAt() {
	if sys.data.LastRewardAt == 0 {
		return
	}

	// 如果不是同一天
	if time_util.IsSameDay(sys.data.LastRewardAt, time_util.NowSec()) {
		return
	}

	// 但是已经领取过, 表示离线了没走到跨天的处理
	if sys.data.Rewarded {
		sys.data.Rewarded = false
	}
}

func (sys *FairyDynastyJobSys) AddPoint(point int64) {
	sys.data.Point += point
	fairydynastyjobrank.UpdateZongMenRank(sys.owner.GetId(), sys.data.Point)

	conf := jsondata.GetFairyDynastyJobCommonConf()
	if conf == nil {
		return
	}

	if sys.data.Point < int64(conf.CrossRankLimit) {
		return
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer,
		sysfuncid.G2FFairyDynastyJobAddRenownRankPointReq,
		&pb3.G2FFairyDynastyJobAddRenownRankPointReq{
			PlayerId: sys.owner.GetId(),
			Score:    sys.data.Point,
		})
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
	}

	sys.SendProto3(43, 204, &pb3.S2C_43_204{
		Point: sys.data.Point,
	})
}

func (sys *FairyDynastyJobSys) sendInfo() {
	sys.SendProto3(43, 200, &pb3.S2C_43_200{
		Data: sys.data,
	})
}

func (sys *FairyDynastyJobSys) sendZongMenRankInfo() {
	ranks := fairydynastyjobrank.PackZongMenRank()

	if ranks == nil {
		return
	}

	sys.SendProto3(43, 201, &pb3.S2C_43_201{
		Rank: ranks,
	})
}

func (sys *FairyDynastyJobSys) c2sZongMemRankInfo(_ *base.Message) error {
	sys.sendZongMenRankInfo()
	return nil
}

func (sys *FairyDynastyJobSys) c2sReward(_ *base.Message) error {
	if sys.data.Rewarded {
		return neterror.ParamsInvalidError("已经领取过奖励")
	}

	conf := jsondata.GetFairyDynastyJobConfigByPoint(sys.data.Point)

	if conf == nil {
		return neterror.ParamsInvalidError("没有达到最低奖励要求")
	}

	sys.data.Rewarded = true
	sys.data.LastRewardAt = time_util.NowSec()

	state := engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyDynastyJobReward,
	})

	if !state {
		return neterror.InternalError("give reward failed")
	}

	sys.SendProto3(43, 203, &pb3.S2C_43_203{
		Rewarded: sys.data.Rewarded,
	})
	return nil
}

func gmAddFairyDynastyJobPoint(player iface.IPlayer, args ...string) bool {
	if player == nil {
		return false
	}

	sys := player.GetSysObj(sysdef.SiFairyDynastyJobSys).(*FairyDynastyJobSys)
	if sys == nil || !sys.IsOpen() {
		return false
	}

	sys.AddPoint(100)
	sys.SendProto3(43, 204, &pb3.S2C_43_204{
		Point: sys.data.Point,
	})
	return true
}

func init() {
	RegisterSysClass(sysdef.SiFairyDynastyJobSys, func() iface.ISystem {
		return &FairyDynastyJobSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys := player.GetSysObj(sysdef.SiFairyDynastyJobSys).(*FairyDynastyJobSys)
		if sys == nil || !sys.IsOpen() {
			return
		}
		sys.data.Rewarded = false

		sys.sendInfo()
	})

	net.RegisterSysProtoV2(43, 203, sysdef.SiFairyDynastyJobSys,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(msg *base.Message) error {
				return sys.(*FairyDynastyJobSys).c2sReward(nil)
			}
		})

	net.RegisterSysProtoV2(43, 201, sysdef.SiFairyDynastyJobSys,
		func(sys iface.ISystem) func(*base.Message) error {
			return func(msg *base.Message) error {
				return sys.(*FairyDynastyJobSys).c2sZongMemRankInfo(nil)
			}
		})

	gmevent.Register("add_fairy_dynasty_job_point", gmAddFairyDynastyJobPoint, 1)
}
