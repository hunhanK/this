/**
 * @Author: LvYuMeng
 * @Date: 2024/6/20
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type PlaneChargeTodaySys struct {
	PlayerYYBase
}

func (s *PlaneChargeTodaySys) GetData() *pb3.PYY_PlaneChargeToday {
	state := s.GetYYData()
	if nil == state.PlaneChargeToday {
		state.PlaneChargeToday = make(map[uint32]*pb3.PYY_PlaneChargeToday)
	}
	if nil == state.PlaneChargeToday[s.Id] {
		state.PlaneChargeToday[s.Id] = &pb3.PYY_PlaneChargeToday{}
	}
	return state.PlaneChargeToday[s.Id]
}

func (s *PlaneChargeTodaySys) OnEnd() {
	conf := jsondata.GetYYPlaneChargeTodayConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	if data.GetIsRev() || data.GetCent() < conf.Cent {
		return
	}
	mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
		ConfId:  conf.MailId,
		Rewards: conf.Rewards,
	})
}

func (s *PlaneChargeTodaySys) OnOpen() {
	s.s2cInfo()
}

func (s *PlaneChargeTodaySys) ResetData() {
	state := s.GetYYData()
	if nil == state.PlaneChargeToday {
		return
	}
	delete(state.PlaneChargeToday, s.GetId())
}

func (s *PlaneChargeTodaySys) Login() {
	s.s2cInfo()
}

func (s *PlaneChargeTodaySys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlaneChargeTodaySys) s2cInfo() {
	s.SendProto3(69, 0, &pb3.S2C_69_0{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *PlaneChargeTodaySys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}

	chargeCent := chargeEvent.CashCent

	data := s.GetData()
	data.Cent += chargeCent

	s.s2cInfo()
}

func (s *PlaneChargeTodaySys) c2sRev(msg *base.Message) error {
	conf := jsondata.GetYYPlaneChargeTodayConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PlaneChargeToday conf nil")
	}

	data := s.GetData()
	if data.GetIsRev() {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	if data.GetCent() < conf.Cent {
		return neterror.ParamsInvalidError("PlaneChargeToday charge cent not enough")
	}

	data.IsRev = true
	engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlaneChargeTodayAward})
	s.s2cInfo()

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneChargeToday, func() iface.IPlayerYY {
		return &PlaneChargeTodaySys{}
	})

	net.RegisterYYSysProtoV2(69, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PlaneChargeTodaySys).c2sRev
	})
}
