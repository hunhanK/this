/**
 * @Author: LvYuMeng
 * @Date: 2024/7/29
 * @Desc: 极品白送
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type OneGiveTwoSys struct {
	PlayerYYBase
}

func (s *OneGiveTwoSys) ResetData() {
	state := s.GetYYData()
	if nil == state.OneGiveTwo {
		return
	}
	delete(state.OneGiveTwo, s.Id)
}

func (s *OneGiveTwoSys) data() *pb3.PYY_OneGiveTwo {
	state := s.GetYYData()
	if nil == state.OneGiveTwo {
		state.OneGiveTwo = make(map[uint32]*pb3.PYY_OneGiveTwo)
	}
	if nil == state.OneGiveTwo[s.Id] {
		state.OneGiveTwo[s.Id] = &pb3.PYY_OneGiveTwo{}
	}
	if nil == state.OneGiveTwo[s.Id].Status {
		state.OneGiveTwo[s.Id].Status = make(map[uint32]*pb3.OneGiveTwo)
	}
	if nil == state.OneGiveTwo[s.Id].ChargeInfo {
		state.OneGiveTwo[s.Id].ChargeInfo = make(map[uint32]uint32)
	}
	return state.OneGiveTwo[s.Id]
}

func (s *OneGiveTwoSys) Login() {
	s.refreshChargeToday(false)
	s.s2cInfo()
}

func (s *OneGiveTwoSys) OnReconnect() {
	s.s2cInfo()
}

func (s *OneGiveTwoSys) OnEnd() {
	conf, ok := jsondata.GetPYYOneGiveTwoConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.data()
	var rewardsVec []jsondata.StdRewardVec
	for _, line := range conf.ChargeConf {
		if r := s.calculateDayRewards(line, data, 0); nil != r {
			rewardsVec = append(rewardsVec, r)
			s.record(data, s.Id, 0)
		}
		for _, line2 := range line.DayConf {
			if r := s.calculateDayRewards(line, data, line2.Day); nil != r {
				rewardsVec = append(rewardsVec, r)
				s.record(data, s.Id, line2.Day)
			}
		}
	}
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (s *OneGiveTwoSys) s2cInfo() {
	s.SendProto3(75, 30, &pb3.S2C_75_30{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *OneGiveTwoSys) OnOpen() {
	s.refreshChargeToday(false)
	s.s2cInfo()
}

func (s *OneGiveTwoSys) refreshChargeToday(bro bool) {
	data := s.data()
	zeroTime := time_util.GetDaysZeroTime(0)
	data.ChargeInfo[zeroTime] = s.GetDailyChargeMoney(zeroTime)
	if bro {
		s.SendProto3(75, 33, &pb3.S2C_75_33{
			ActiveId:   s.Id,
			TimeStamp:  zeroTime,
			ChargeCent: data.ChargeInfo[zeroTime],
		})
	}
}

func (s *OneGiveTwoSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}

	s.refreshChargeToday(true)
}

func (s *OneGiveTwoSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_75_31
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := s.getChargeConf(req.GetId())
	if !ok {
		return neterror.ConfNotFoundError("conf %d is nil", req.GetId())
	}

	data := s.data()

	rewards := s.calculateDayRewards(conf, data, req.Day)
	if rewards == nil {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	s.record(data, req.Id, req.Day)
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogOneGiveTwoAwards})

	s.SendProto3(75, 31, &pb3.S2C_75_31{
		ActiveId: s.Id,
		Id:       req.Id,
		Day:      req.Day,
	})
	return nil
}

func (s *OneGiveTwoSys) record(data *pb3.PYY_OneGiveTwo, id, day uint32) {
	status, ok := data.Status[id]
	if !ok {
		status = &pb3.OneGiveTwo{}
		data.Status[id] = status
	}

	status.DayRev = append(status.DayRev, day)
}

// actDay2Timestamp 第一天是0
func (s *OneGiveTwoSys) actDay2Timestamp(day uint32) uint32 {
	return time_util.GetZeroTime(s.GetOpenTime()) + (86400 * day)
}

func (s *OneGiveTwoSys) getChargeConf(id uint32) (*jsondata.OneGiveTwoChargeConf, bool) {
	conf, ok := jsondata.GetPYYOneGiveTwoConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil, false
	}
	chargeConf, ok := conf.ChargeConf[id]
	return chargeConf, ok
}

func (s *OneGiveTwoSys) countChargeDays(chargeInfo map[uint32]uint32, chargeCent uint32) uint32 {
	var chargeDay uint32
	for _, cent := range chargeInfo {
		if cent >= chargeCent {
			chargeDay++
		}
	}
	return chargeDay
}

func (s *OneGiveTwoSys) calculateDayRewards(conf *jsondata.OneGiveTwoChargeConf, data *pb3.PYY_OneGiveTwo, day uint32) jsondata.StdRewardVec {
	if st, ok := data.Status[conf.Id]; ok && pie.Uint32s(st.DayRev).Contains(day) {
		return nil
	}

	if day == 0 {
		chargeDay := s.countChargeDays(data.ChargeInfo, conf.ChargeCent)
		if chargeDay < conf.TargetDays {
			return nil
		}
		return conf.TargetRewards
	}

	cent := data.ChargeInfo[s.actDay2Timestamp(day-1)]
	if cent < conf.ChargeCent {
		return nil
	}

	if dayConf, ok := conf.DayConf[day]; ok {
		return dayConf.Rewards
	}
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYOneGiveTwo, func() iface.IPlayerYY {
		return &OneGiveTwoSys{}
	})

	net.RegisterYYSysProtoV2(75, 31, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*OneGiveTwoSys).c2sRev
	})
}
