/**
 * @Author: zjj
 * @Date: 2025/4/16
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math"
)

type SuperContinueChargeSys struct {
	PlayerYYBase
}

const (
	SuperContinueChargeAwardNotActive = 0 //未激活
	SuperContinueChargeAwardWaitRev   = 1 //待领取
	SuperContinueChargeAwardRev       = 2 //已领取

	SuperContinueChargeAwardTypeDaily = 1 // 每日奖励
	SuperContinueChargeAwardTypeSum   = 2 // 累积奖励
	SuperContinueChargeAwardTypeLast  = 3 // 最终奖励
)

func (s *SuperContinueChargeSys) GetData() *pb3.PYY_SuperContinueCharge {
	state := s.GetYYData()
	if nil == state.SuperContinueCharge {
		state.SuperContinueCharge = make(map[uint32]*pb3.PYY_SuperContinueCharge)
	}
	if nil == state.SuperContinueCharge[s.Id] {
		state.SuperContinueCharge[s.Id] = &pb3.PYY_SuperContinueCharge{}
	}
	if nil == state.SuperContinueCharge[s.Id].ChargeAmountMap {
		state.SuperContinueCharge[s.Id].ChargeAmountMap = make(map[uint32]uint32)
	}
	if nil == state.SuperContinueCharge[s.Id].DailyChargeMap {
		state.SuperContinueCharge[s.Id].DailyChargeMap = make(map[uint32]*pb3.PYY_SuperContinueDailyCharge)
	}
	if nil == state.SuperContinueCharge[s.Id].SumChargeMap {
		state.SuperContinueCharge[s.Id].SumChargeMap = make(map[uint32]uint32)
	}
	if nil == state.SuperContinueCharge[s.Id].SuperTargetMap {
		state.SuperContinueCharge[s.Id].SuperTargetMap = make(map[uint32]uint32)
	}

	return state.SuperContinueCharge[s.Id]
}

func (s *SuperContinueChargeSys) GetDailyChargeMap(openDay uint32) *pb3.PYY_SuperContinueDailyCharge {
	if openDay == 0 {
		return nil
	}
	data := s.GetData()
	dailyCharge, ok := data.DailyChargeMap[openDay]
	if !ok {
		data.DailyChargeMap[openDay] = &pb3.PYY_SuperContinueDailyCharge{}
		dailyCharge = data.DailyChargeMap[openDay]
	}
	if dailyCharge.DailyChargeMap == nil {
		dailyCharge.DailyChargeMap = make(map[uint32]uint32)
	}
	return dailyCharge
}

func (s *SuperContinueChargeSys) OnEnd() {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	var rewardsVec []jsondata.StdRewardVec
	for _, dailyData := range data.DailyChargeMap {
		if dailyData == nil || dailyData.DailyChargeMap == nil {
			continue
		}
		for _id, _state := range dailyData.DailyChargeMap {
			if _state != SuperContinueChargeAwardWaitRev {
				continue
			}
			if dailyConf, ok := conf.DailyCharge[_id]; ok {
				rewardsVec = append(rewardsVec, dailyConf.Rewards)
			}
			dailyData.DailyChargeMap[_id] = SuperContinueChargeAwardRev
		}
	}
	for id, state := range data.SumChargeMap {
		if state != SuperContinueChargeAwardWaitRev {
			continue
		}
		if sumConf, ok := conf.SumAward[id]; ok {
			rewardsVec = append(rewardsVec, sumConf.Rewards)
		}
		data.SumChargeMap[id] = SuperContinueChargeAwardRev
	}
	s.checkSuperTargetAward()
	for id, state := range data.SuperTargetMap {
		if state != SuperContinueChargeAwardWaitRev {
			continue
		}
		if sumConf, ok := conf.SuperTargetAward[id]; ok {
			rewardsVec = append(rewardsVec, sumConf.Rewards)
		}
		data.SuperTargetMap[id] = SuperContinueChargeAwardRev
	}
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *SuperContinueChargeSys) openActCheckDailyCharge() {
	s.handleCharge(s.GetDailyCharge())
}

func (s *SuperContinueChargeSys) OnOpen() {
	s.s2cInfo()
	s.openActCheckDailyCharge()
}

func (s *SuperContinueChargeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.SuperContinueCharge {
		return
	}
	delete(state.SuperContinueCharge, s.GetId())
}

func (s *SuperContinueChargeSys) Login() {
	s.s2cInfo()
}

func (s *SuperContinueChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SuperContinueChargeSys) s2cInfo() {
	s.SendProto3(8, 180, &pb3.S2C_8_180{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *SuperContinueChargeSys) handleCharge(chargeCent uint32) {
	if chargeCent == 0 {
		return
	}

	day := s.GetOpenDay()
	data := s.GetData()

	var addDollar uint32
	if conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx); nil != conf {
		todayStandard := conf.GetDailyChargeMostAmount(day)
		if data.ChargeAmountMap[day] >= todayStandard {
			addDollar = chargeCent
		} else if data.ChargeAmountMap[day]+chargeCent > todayStandard {
			addDollar = data.ChargeAmountMap[day] + chargeCent - todayStandard
		}
	}

	data.ChargeAmountMap[day] += chargeCent
	s.SendProto3(8, 186, &pb3.S2C_8_186{
		ActiveId:     s.GetId(),
		Day:          day,
		ChargeAmount: data.ChargeAmountMap[day],
	})

	s.checkActiveDailyAward(day)

	s.addSilverDollar(addDollar)

	s.checkActiveSumAward()
}

func (s *SuperContinueChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}
	chargeCent := chargeEvent.CashCent
	s.handleCharge(chargeCent)
}

func (s *SuperContinueChargeSys) addSilverDollar(chargeCent uint32) {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
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
		state := s.GetDailyChargeMap(openDay)
		if !s.isActive(state.DailyChargeMap[v.ID]) {
			return
		}
	}
	data.SilverDollar += conf.SilverDollarRate * chargeCent
	s.onSilverDollarChange()
}

func (s *SuperContinueChargeSys) onSilverDollarChange() {
	dollar := s.GetData().GetSilverDollar()
	s.SendProto3(8, 182, &pb3.S2C_8_182{
		ActiveId:     s.GetId(),
		SilverDollar: dollar,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSuperContinueChargesilverDollar, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", dollar),
	})
}

func (s *SuperContinueChargeSys) isActive(state uint32) bool {
	return state == SuperContinueChargeAwardWaitRev || state == SuperContinueChargeAwardRev
}

func (s *SuperContinueChargeSys) NewDay() {
	s.s2cInfo()
}

func (s *SuperContinueChargeSys) checkActiveDailyAward(openDay uint32) {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	if openDay == 0 {
		openDay = s.GetOpenDay()
	}

	data := s.GetData()
	state := s.GetDailyChargeMap(openDay)
	for _, v := range conf.DailyCharge {
		if v.Day != openDay {
			continue
		}
		if data.ChargeAmountMap[v.Day] < v.Cent {
			continue
		}
		if s.isActive(state.DailyChargeMap[v.ID]) {
			continue
		}
		state.DailyChargeMap[v.ID] = SuperContinueChargeAwardWaitRev
		s.SendProto3(8, 184, &pb3.S2C_8_184{
			ActiveId: s.GetId(),
			Id:       v.ID,
			State:    state.DailyChargeMap[v.ID],
			Type:     SuperContinueChargeAwardTypeDaily,
			OpenDay:  openDay,
		})
	}
}

func (s *SuperContinueChargeSys) checkActiveSumAward() {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
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
		data.SumChargeMap[v.ID] = SuperContinueChargeAwardWaitRev
		s.SendProto3(8, 184, &pb3.S2C_8_184{
			ActiveId: s.GetId(),
			Id:       v.ID,
			State:    data.SumChargeMap[v.ID],
			Type:     SuperContinueChargeAwardTypeSum,
		})
	}
}

func (s *SuperContinueChargeSys) checkSuperTargetAward() {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	for _, v := range conf.SuperTargetAward {
		if s.isActive(data.SuperTargetMap[v.ID]) {
			continue
		}
		var canAdd = true
		for _, id := range v.GradeAwardIds {
			state := data.SumChargeMap[id]
			if state != SuperContinueChargeAwardRev {
				canAdd = false
				break
			}
		}
		if !canAdd {
			continue
		}
		data.SuperTargetMap[v.ID] = SuperContinueChargeAwardWaitRev
		s.SendProto3(8, 184, &pb3.S2C_8_184{
			ActiveId: s.GetId(),
			Id:       v.ID,
			State:    data.SuperTargetMap[v.ID],
			Type:     SuperContinueChargeAwardTypeLast,
		})
	}
}

func (s *SuperContinueChargeSys) c2sDailyRev(msg *base.Message) error {
	var req pb3.C2S_8_183
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("ChargeToday conf nil")
	}

	id := req.GetId()
	if nil == conf.DailyCharge[id] {
		return neterror.ConfNotFoundError("ChargeToday daily award conf %d nil", id)
	}

	state := s.GetDailyChargeMap(req.OpenDay)
	if state == nil {
		return neterror.ParamsInvalidError("%d not data", req.OpenDay)
	}
	if state.DailyChargeMap[id] != SuperContinueChargeAwardWaitRev {
		return neterror.ParamsInvalidError("ChargeToday daily award not active %d", id)
	}

	state.DailyChargeMap[id] = SuperContinueChargeAwardRev
	mergeStdReward := jsondata.MergeStdReward(conf.DailyCharge[id].Rewards)
	engine.GiveRewards(s.GetPlayer(), mergeStdReward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSuperContinueChargeDailyAward})
	s.player.SendShowRewardsPop(mergeStdReward)

	s.SendProto3(8, 183, &pb3.S2C_8_183{
		ActiveId: s.GetId(),
		Id:       id,
		OpenDay:  req.OpenDay,
	})
	return nil
}

func (s *SuperContinueChargeSys) isSilverDollarOpenDay(day uint32) bool {
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	return day >= conf.SilverDollarOpen
}

func (s *SuperContinueChargeSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_8_184
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("ChargeToday conf nil")
	}

	id := req.GetId()
	dayConf := conf.DailyCharge[id]
	if nil == dayConf {
		return neterror.ConfNotFoundError("ChargeToday daily award conf %d nil", id)
	}

	if !s.isSilverDollarOpenDay(s.GetOpenDay()) {
		return neterror.ParamsInvalidError("ChargeToday dollar cant active end day")
	}

	if s.GetOpenDay() <= dayConf.Day {
		return neterror.ParamsInvalidError("ChargeToday dollar cant active the day after today")
	}

	data := s.GetData()
	state := s.GetDailyChargeMap(req.OpenDay)
	if state == nil {
		return neterror.ParamsInvalidError("%d not data", req.OpenDay)
	}
	if s.isActive(state.DailyChargeMap[id]) {
		return neterror.ParamsInvalidError("ChargeToday daily award %d has active", id)
	}

	baseCharge := data.ChargeAmountMap[dayConf.Day]
	if dayConf.Cent > baseCharge { //银元买额度
		var supple uint32
		if dayConf.Cent > baseCharge {
			supple = dayConf.Cent - baseCharge
		}
		supple = uint32(math.Ceil(float64(supple) * float64(dayConf.Tokens) / float64(dayConf.Cent)))
		if supple > data.SilverDollar {
			return neterror.ParamsInvalidError("ChargeToday SilverDollar not enough")
		}
		data.SilverDollar = data.SilverDollar - supple
		s.onSilverDollarChange()
		data.ChargeAmountMap[dayConf.Day] = dayConf.Cent
		s.SendProto3(8, 186, &pb3.S2C_8_186{
			ActiveId:     s.GetId(),
			Day:          dayConf.Day,
			ChargeAmount: data.ChargeAmountMap[dayConf.Day],
		})
	}

	s.checkActiveDailyAward(req.OpenDay)
	s.checkActiveSumAward()

	return nil
}

func (s *SuperContinueChargeSys) c2sSumAwardRev(msg *base.Message) error {
	var req pb3.C2S_8_185
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("ChargeToday conf nil")
	}

	id := req.GetId()
	if nil == conf.SumAward[id] {
		return neterror.ConfNotFoundError("ChargeToday daily award conf %d nil", id)
	}

	data := s.GetData()
	if data.SumChargeMap[id] != SuperContinueChargeAwardWaitRev {
		return neterror.ParamsInvalidError("ChargeToday daily award not active %d", id)
	}

	data.SumChargeMap[id] = SuperContinueChargeAwardRev
	mergeStdReward := jsondata.MergeStdReward(conf.SumAward[id].Rewards)
	engine.GiveRewards(s.GetPlayer(), mergeStdReward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSuperContinueChargeSumAward})
	s.player.SendShowRewardsPop(mergeStdReward)

	s.SendProto3(8, 185, &pb3.S2C_8_185{
		ActiveId: s.GetId(),
		Id:       id,
	})
	s.checkSuperTargetAward()
	return nil
}

func (s *SuperContinueChargeSys) c2sSuperTargetAwardRev(msg *base.Message) error {
	var req pb3.C2S_8_187
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYSuperContinueChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("ChargeToday conf nil")
	}

	id := req.GetId()
	if nil == conf.SuperTargetAward[id] {
		return neterror.ConfNotFoundError("ChargeToday daily award conf %d nil", id)
	}

	data := s.GetData()
	if data.SuperTargetMap[id] != SuperContinueChargeAwardWaitRev {
		return neterror.ParamsInvalidError("ChargeToday daily award not active %d", id)
	}

	data.SuperTargetMap[id] = SuperContinueChargeAwardRev
	mergeStdReward := jsondata.MergeStdReward(conf.SuperTargetAward[id].Rewards)
	engine.GiveRewards(s.GetPlayer(), mergeStdReward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSuperContinueChargeSumAward})
	s.player.SendShowRewardsPop(mergeStdReward)

	s.SendProto3(8, 187, &pb3.S2C_8_187{
		ActiveId: s.GetId(),
		Id:       id,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSuperContinueCharge, func() iface.IPlayerYY {
		return &SuperContinueChargeSys{}
	})

	net.RegisterYYSysProtoV2(8, 183, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SuperContinueChargeSys).c2sDailyRev
	})
	net.RegisterYYSysProtoV2(8, 184, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SuperContinueChargeSys).c2sActive
	})
	net.RegisterYYSysProtoV2(8, 185, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SuperContinueChargeSys).c2sSumAwardRev
	})
	net.RegisterYYSysProtoV2(8, 187, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SuperContinueChargeSys).c2sSuperTargetAwardRev
	})
}
