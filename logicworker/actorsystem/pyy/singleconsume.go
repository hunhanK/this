/**
 * @Author: lzp
 * @Date: 2025/12/8
 * @Desc:
**/

package pyy

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
)

type SingleConsumeSys struct {
	PlayerYYBase
}

func (s *SingleConsumeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SingleConsumeSys) Login() {
	s.s2cInfo()
}

func (s *SingleConsumeSys) OnOpen() {
	s.s2cInfo()
}

func (s *SingleConsumeSys) NewDay() {
	data := s.GetData()
	data.IsConsume = false
	s.s2cInfo()
}

func (s *SingleConsumeSys) GetData() *pb3.PYYSingleConsume {
	yyData := s.GetYYData()
	if nil == yyData.SingleConsumeData {
		yyData.SingleConsumeData = make(map[uint32]*pb3.PYYSingleConsume)
	}
	if nil == yyData.SingleConsumeData[s.Id] {
		yyData.SingleConsumeData[s.Id] = &pb3.PYYSingleConsume{}
	}
	return yyData.SingleConsumeData[s.Id]
}

func (s *SingleConsumeSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.SingleConsumeData {
		return
	}
	delete(yyData.SingleConsumeData, s.Id)
}

func (s *SingleConsumeSys) s2cInfo() {
	s.SendProto3(127, 205, &pb3.S2C_127_205{
		ActId: s.Id,
		Data:  s.GetData(),
	})
}

func (s *SingleConsumeSys) checkCanGetReward(chargeId uint32) {
	data := s.GetData()
	if data.IsConsume {
		return
	}
	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf.ChargeType != chargedef.Charge {
		return
	}

	conf := jsondata.GetYYSingleConsumeConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	var rewards jsondata.StdRewardVec
	if conf.MoneyItem1 > 0 && conf.MoneyRatio1 > 0 {
		rewards = append(rewards, &jsondata.StdReward{
			Id:    conf.MoneyItem1,
			Count: int64(chargeConf.Diamond * conf.MoneyRatio1 / 10000),
		})
	}

	player := s.GetPlayer()
	mailmgr.SendMailToActor(player.GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_SingleConsumeAwards,
		Rewards: rewards,
		Content: &mailargs.CommonMailArgs{
			Digit1: int64(chargeConf.CashCent / chargedef.ChargeTransfer),
		},
	})
	data.IsConsume = true
	s.s2cInfo()
}

func (s *SingleConsumeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	s.checkCanGetReward(chargeEvent.ChargeId)
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSingleConsume, func() iface.IPlayerYY {
		return &SingleConsumeSys{}
	})
}
