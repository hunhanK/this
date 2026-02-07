/**
 * @Author: LvYuMeng
 * @Date: 2024/6/20
 * @Desc:
**/

package pyy

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math"
)

type PlaneContinueChargeSys struct {
	PlayerYYBase
}

const (
	PlaneContinueChargeAwardNotActive = 0 //未激活
	PlaneContinueChargeAwardWaitRev   = 1 //待领取
	PlaneContinueChargeAwardRev       = 2 //已领取

	PlaneContinueChargeAwardTypeDaily = 1
	PlaneContinueChargeAwardTypeSum   = 2
)

func (s *PlaneContinueChargeSys) GetData() *pb3.PYY_PlaneContinueCharge {
	state := s.GetYYData()
	if nil == state.PlaneContinueCharge {
		state.PlaneContinueCharge = make(map[uint32]*pb3.PYY_PlaneContinueCharge)
	}
	if nil == state.PlaneContinueCharge[s.Id] {
		state.PlaneContinueCharge[s.Id] = &pb3.PYY_PlaneContinueCharge{}
	}
	if nil == state.PlaneContinueCharge[s.Id].ChargeAmountMap {
		state.PlaneContinueCharge[s.Id].ChargeAmountMap = make(map[uint32]uint32)
	}
	if nil == state.PlaneContinueCharge[s.Id].DailyChargeMap {
		state.PlaneContinueCharge[s.Id].DailyChargeMap = make(map[uint32]uint32)
	}
	if nil == state.PlaneContinueCharge[s.Id].SumChargeMap {
		state.PlaneContinueCharge[s.Id].SumChargeMap = make(map[uint32]uint32)
	}

	return state.PlaneContinueCharge[s.Id]
}

func (s *PlaneContinueChargeSys) OnEnd() {
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	var rewardsVec []jsondata.StdRewardVec
	for id, state := range data.DailyChargeMap {
		if state != PlaneContinueChargeAwardWaitRev {
			continue
		}
		if dailyConf, ok := conf.DailyCharge[id]; ok {
			rewardsVec = append(rewardsVec, dailyConf.Rewards)
		}
	}
	for id, state := range data.SumChargeMap {
		if state != PlaneContinueChargeAwardWaitRev {
			continue
		}
		if sumConf, ok := conf.SumAward[id]; ok {
			rewardsVec = append(rewardsVec, sumConf.Rewards)
		}
	}
	rewards := jsondata.AppendStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *PlaneContinueChargeSys) OnOpen() {
	s.s2cInfo()
}

func (s *PlaneContinueChargeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.PlaneContinueCharge {
		return
	}
	delete(state.PlaneContinueCharge, s.GetId())
}

func (s *PlaneContinueChargeSys) Login() {
	s.s2cInfo()
}

func (s *PlaneContinueChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlaneContinueChargeSys) MergeFix() {
	openZeroTime := time_util.GetZeroTime(s.OpenTime)
	openDayCent := s.GetDailyChargeMoney(openZeroTime)
	s.onPlayerCharge(openDayCent, 1)
}

func (s *PlaneContinueChargeSys) s2cInfo() {
	s.SendProto3(69, 11, &pb3.S2C_69_11{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *PlaneContinueChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}

	chargeCent := chargeEvent.CashCent
	day := s.GetOpenDay()
	s.onPlayerCharge(chargeCent, day)
}

func (s *PlaneContinueChargeSys) onPlayerCharge(chargeCent, day uint32) {
	data := s.GetData()

	var addDollar uint32
	if conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx); nil != conf {
		todayStandard := conf.GetDailyChargeMostAmount(day)
		if data.ChargeAmountMap[day] >= todayStandard {
			addDollar = chargeCent
		} else if data.ChargeAmountMap[day]+chargeCent > todayStandard {
			addDollar = data.ChargeAmountMap[day] + chargeCent - todayStandard
		}
	}

	data.ChargeAmountMap[day] += chargeCent
	s.SendProto3(69, 16, &pb3.S2C_69_16{
		ActiveId:     s.GetId(),
		Day:          day,
		ChargeAmount: data.ChargeAmountMap[day],
	})

	s.checkActiveDailyAward()

	s.addSilverDollar(addDollar)

	s.checkActiveSumAward()
}

func (s *PlaneContinueChargeSys) addSilverDollar(chargeCent uint32) {
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	openDay := s.GetOpenDay()
	if !s.isSilverDollarOpenDay(s.GetOpenDay()) {
		return
	}
	data := s.GetData()
	for _, v := range conf.DailyCharge {
		if v.Day != openDay {
			continue
		}
		if !s.isActive(data.DailyChargeMap[v.ID]) {
			return
		}
	}
	data.SilverDollar += conf.SilverDollarRate * chargeCent
	s.onSilverDollarChange()
}

func (s *PlaneContinueChargeSys) onSilverDollarChange() {
	dollar := s.GetData().GetSilverDollar()
	s.SendProto3(69, 12, &pb3.S2C_69_12{
		ActiveId:     s.GetId(),
		SilverDollar: dollar,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlaneContinueChargesilverDollar, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", dollar),
	})
}

func (s *PlaneContinueChargeSys) isActive(state uint32) bool {
	return state == PlaneContinueChargeAwardWaitRev || state == PlaneContinueChargeAwardRev
}

func (s *PlaneContinueChargeSys) checkActiveDailyAward() {
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.GetData()
	for _, v := range conf.DailyCharge {
		if data.ChargeAmountMap[v.Day] < v.Cent {
			continue
		}
		if s.isActive(data.DailyChargeMap[v.ID]) {
			continue
		}
		data.DailyChargeMap[v.ID] = PlaneContinueChargeAwardWaitRev
		s.SendProto3(69, 14, &pb3.S2C_69_14{
			ActiveId: s.GetId(),
			Id:       v.ID,
			State:    data.DailyChargeMap[v.ID],
			Type:     PlaneContinueChargeAwardTypeDaily,
		})
	}
}

func (s *PlaneContinueChargeSys) checkActiveSumAward() {
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.GetData()

	for _, v := range conf.SumAward {
		if s.isActive(data.SumChargeMap[v.ID]) {
			continue
		}
		var sumDay uint32
		for _, amount := range data.ChargeAmountMap {
			if amount < v.Cent {
				continue
			}
			sumDay++
		}
		if sumDay < v.Days {
			continue
		}
		data.SumChargeMap[v.ID] = PlaneContinueChargeAwardWaitRev
		s.SendProto3(69, 14, &pb3.S2C_69_14{
			ActiveId: s.GetId(),
			Id:       v.ID,
			State:    data.SumChargeMap[v.ID],
			Type:     PlaneContinueChargeAwardTypeSum,
		})
	}
}

func (s *PlaneContinueChargeSys) c2sDailyRev(msg *base.Message) error {
	var req pb3.C2S_69_13
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PlaneChargeToday conf nil")
	}

	id := req.GetId()
	if nil == conf.DailyCharge[id] {
		return neterror.ConfNotFoundError("PlaneChargeToday daily award conf %d nil", id)
	}

	data := s.GetData()
	if data.DailyChargeMap[id] != PlaneContinueChargeAwardWaitRev {
		return neterror.ParamsInvalidError("PlaneChargeToday daily award not active %d", id)
	}

	data.DailyChargeMap[id] = PlaneContinueChargeAwardRev
	engine.GiveRewards(s.GetPlayer(), conf.DailyCharge[id].Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlaneContinueChargeDailyAward})

	s.SendProto3(69, 13, &pb3.S2C_69_13{
		ActiveId: s.GetId(),
		Id:       id,
	})
	return nil
}

func (s *PlaneContinueChargeSys) isSilverDollarOpenDay(day uint32) bool {
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	return day >= conf.SilverDollarOpen
}

func (s *PlaneContinueChargeSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_69_14
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PlaneChargeToday conf nil")
	}

	id := req.GetId()
	dayConf := conf.DailyCharge[id]
	if nil == dayConf {
		return neterror.ConfNotFoundError("PlaneChargeToday daily award conf %d nil", id)
	}

	if !s.isSilverDollarOpenDay(s.GetOpenDay()) {
		return neterror.ParamsInvalidError("PlaneChargeToday dollar cant active end day")
	}

	if s.GetOpenDay() <= dayConf.Day {
		return neterror.ParamsInvalidError("PlaneChargeToday dollar cant active the day after today")
	}

	data := s.GetData()
	if s.isActive(data.DailyChargeMap[id]) {
		return neterror.ParamsInvalidError("PlaneChargeToday daily award %d has active", id)
	}

	baseCharge := data.ChargeAmountMap[dayConf.Day]
	if dayConf.Cent > baseCharge { //银元买额度
		var supple uint32
		if dayConf.Cent > baseCharge {
			supple = dayConf.Cent - baseCharge
		}
		supple = uint32(math.Ceil(float64(supple) * float64(dayConf.Tokens) / float64(dayConf.Cent)))
		if supple > data.SilverDollar {
			return neterror.ParamsInvalidError("PlaneChargeToday SilverDollar not enough")
		}
		data.SilverDollar = data.SilverDollar - supple
		s.onSilverDollarChange()
		data.ChargeAmountMap[dayConf.Day] = dayConf.Cent
		s.SendProto3(69, 16, &pb3.S2C_69_16{
			ActiveId:     s.GetId(),
			Day:          dayConf.Day,
			ChargeAmount: data.ChargeAmountMap[dayConf.Day],
		})
	}

	s.checkActiveDailyAward()
	s.checkActiveSumAward()

	return nil
}

func (s *PlaneContinueChargeSys) c2sSumAwardRev(msg *base.Message) error {
	var req pb3.C2S_69_15
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYPlaneContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PlaneChargeToday conf nil")
	}

	id := req.GetId()
	if nil == conf.SumAward[id] {
		return neterror.ConfNotFoundError("PlaneChargeToday daily award conf %d nil", id)
	}

	data := s.GetData()
	if data.SumChargeMap[id] != PlaneContinueChargeAwardWaitRev {
		return neterror.ParamsInvalidError("PlaneChargeToday daily award not active %d", id)
	}

	data.SumChargeMap[id] = PlaneContinueChargeAwardRev
	engine.GiveRewards(s.GetPlayer(), conf.SumAward[id].Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlaneContinueChargeSumAward})

	s.SendProto3(69, 15, &pb3.S2C_69_15{
		ActiveId: s.GetId(),
		Id:       id,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneContinueCharge, func() iface.IPlayerYY {
		return &PlaneContinueChargeSys{}
	})

	net.RegisterYYSysProtoV2(69, 13, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PlaneContinueChargeSys).c2sDailyRev
	})
	net.RegisterYYSysProtoV2(69, 14, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PlaneContinueChargeSys).c2sActive
	})
	net.RegisterYYSysProtoV2(69, 15, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PlaneContinueChargeSys).c2sSumAwardRev
	})
}
