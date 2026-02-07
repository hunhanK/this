/**
 * @Author: LvYuMeng
 * @Date: 2024/6/20
 * @Desc:
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type PlaneSumChargeSys struct {
	PlayerYYBase
}

func (s *PlaneSumChargeSys) GetData() *pb3.PYY_PlaneSumCharge {
	state := s.GetYYData()
	if nil == state.PlaneSumCharge {
		state.PlaneSumCharge = make(map[uint32]*pb3.PYY_PlaneSumCharge)
	}
	if nil == state.PlaneSumCharge[s.Id] {
		state.PlaneSumCharge[s.Id] = &pb3.PYY_PlaneSumCharge{}
	}
	return state.PlaneSumCharge[s.Id]
}

func (s *PlaneSumChargeSys) OnEnd() {
	conf := jsondata.GetYYPlaneSumChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.GetData()
	var rewardsVec []jsondata.StdRewardVec
	for _, v := range conf.Charges {
		if data.GetCent() < v.Cent {
			continue
		}
		if pie.Uint32s(data.Rev).Contains(v.ID) {
			continue
		}
		data.Rev = append(data.Rev, v.ID)
		rewardsVec = append(rewardsVec, v.Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardsVec...)

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *PlaneSumChargeSys) OnOpen() {
	s.s2cInfo()
}

func (s *PlaneSumChargeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.PlaneSumCharge {
		return
	}
	delete(state.PlaneSumCharge, s.GetId())
}

func (s *PlaneSumChargeSys) Login() {
	s.s2cInfo()
}

func (s *PlaneSumChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlaneSumChargeSys) s2cInfo() {
	s.SendProto3(69, 5, &pb3.S2C_69_5{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *PlaneSumChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}
	chargeCent := chargeEvent.CashCent

	data := s.GetData()
	data.Cent += chargeCent

	s.s2cInfo()
}

func (s *PlaneSumChargeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_69_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYPlaneSumChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PlaneSumCharge conf nil")
	}

	smcConf := conf.GetSumChargeConfById(req.GetId())
	if nil == smcConf {
		return neterror.ConfNotFoundError("PlaneSumCharge conf %d nil", req.GetId())
	}

	data := s.GetData()
	if pie.Uint32s(data.Rev).Contains(smcConf.ID) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	if data.GetCent() < smcConf.Cent {
		return neterror.ParamsInvalidError("PlaneSumCharge charge cent not enough")
	}

	data.Rev = append(data.Rev, smcConf.ID)
	engine.GiveRewards(s.GetPlayer(), smcConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlaneSumChargeAward})
	s.SendProto3(69, 6, &pb3.S2C_69_6{
		ActiveId: s.GetId(),
		Id:       smcConf.ID,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneSumCharge, func() iface.IPlayerYY {
		return &PlaneSumChargeSys{}
	})

	net.RegisterYYSysProtoV2(69, 6, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PlaneSumChargeSys).c2sRev
	})
}
