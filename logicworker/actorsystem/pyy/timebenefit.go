/**
 * @Author: lzp
 * @Date: 2024/4/25
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type TimeBenefitSys struct {
	PlayerYYBase
}

func (s *TimeBenefitSys) Login() {
	s.S2CInfo()
}

func (s *TimeBenefitSys) OnReconnect() {
	s.S2CInfo()
}

func (s *TimeBenefitSys) NewDay() {
	data := s.GetData()
	data.RevData = make(map[uint32]bool)
	s.S2CInfo()
}

func (s *TimeBenefitSys) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 61, &pb3.S2C_127_61{
		ActId: s.Id,
		Data:  data,
	})
}

func (s *TimeBenefitSys) GetData() *pb3.PYY_TimeBenefit {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.TimeBenefit == nil {
		state.TimeBenefit = make(map[uint32]*pb3.PYY_TimeBenefit)
	}
	if state.TimeBenefit[s.Id] == nil {
		state.TimeBenefit[s.Id] = &pb3.PYY_TimeBenefit{}
	}
	data := state.TimeBenefit[s.Id]
	if data.RevData == nil {
		data.RevData = make(map[uint32]bool)
	}
	return data
}

func (s *TimeBenefitSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.TimeBenefit == nil {
		return
	}
	delete(state.TimeBenefit, s.Id)
}

func (s *TimeBenefitSys) c2sFetchTimeReward(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}
	var req pb3.C2S_127_60
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := jsondata.GetTimeBenefit(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("timeBenefit conf is nil")
	}

	now := time_util.NowSec()
	tConf, _ := functional.Find(conf.TimeRewards, func(trConf *jsondata.TimeRewardConf) bool {
		if trConf.Id == req.Id {
			return true
		}
		return false
	})
	if tConf == nil {
		return neterror.ParamsInvalidError("timeBenefit conf=%d cannot found", req.Id)
	}
	startTime := time_util.ToTodayTime(tConf.StartTime)
	endTime := time_util.ToTodayTime(tConf.EndTime)
	if now < startTime || now > endTime {
		return neterror.ParamsInvalidError("timeBenefit cannot fetch rewards")
	}

	data := s.GetData()
	if data.RevData[req.Id] {
		return neterror.ParamsInvalidError("timeBenefit=%d is received", req.Id)
	}
	data.RevData[req.Id] = true
	if len(tConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), tConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTimeBenefitAward,
		})
	}

	s.SendProto3(127, 60, &pb3.S2C_127_60{
		ActId: s.Id,
		Id:    req.Id,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTimeBenefit, func() iface.IPlayerYY {
		return &TimeBenefitSys{}
	})
	net.RegisterYYSysProtoV2(127, 60, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TimeBenefitSys).c2sFetchTimeReward
	})
}
