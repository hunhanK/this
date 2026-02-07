/**
 * @Author: yzh
 * @Date:
 * @Desc: 跨服预告
 * @Modify：
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type CrossForecastSys struct {
	Base
}

func (s *CrossForecastSys) OnReconnect() {
	s.s2cInfo()
}
func (s *CrossForecastSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *CrossForecastSys) s2cInfo() {
	s.SendProto3(152, 20, &pb3.S2C_152_20{
		LastRevCrossForecastRewardAt: s.GetBinaryData().LastRevCrossForecastRewardAt,
	})
}

func (s *CrossForecastSys) award() {
	owner := s.GetOwner()
	conf := jsondata.MustGetCrossForecastConf()
	if len(conf) == 0 {
		owner.LogWarn("not found conf")
		return
	}

	forecastConf := conf[0]
	if s.GetBinaryData().LastRevCrossForecastRewardAt > 0 {
		s.LogWarn("already rev rewards")
		return
	}

	now := time_util.NowSec()
	s.GetBinaryData().LastRevCrossForecastRewardAt = now
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogCrossForecastAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(now),
	})

	if !engine.GiveRewards(s.owner, forecastConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardCrossForecast}) {
		s.LogWarn("actor(id:%d) get cross forecast reward failed", s.owner.GetId())
		return
	}

	s.SendProto3(152, 21, &pb3.S2C_152_21{
		LastRevCrossForecastRewardAt: now,
	})
}

var _ iface.ISystem = (*CrossForecastSys)(nil)

func c2sAwardCrossForecast(sys iface.ISystem) func(*base.Message) error {
	return func(_ *base.Message) error {
		sys.(*CrossForecastSys).award()
		return nil
	}
}

func init() {
	RegisterSysClass(sysdef.SiCrossForecast, func() iface.ISystem {
		return &CrossForecastSys{}
	})

	net.RegisterSysProtoV2(152, 21, sysdef.SiCrossForecast, c2sAwardCrossForecast)
}
