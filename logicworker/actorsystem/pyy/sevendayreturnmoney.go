/**
 * @Author: Zjj
 * @Date:
 * @Desc:七日返利
**/

package pyy

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SevenDayReturnMoney struct {
	PlayerYYBase
}

func (s *SevenDayReturnMoney) Login() {
	s.s2cInfo()
}

func (s *SevenDayReturnMoney) OnReconnect() {
	s.s2cInfo()
}

func (s *SevenDayReturnMoney) OnOpen() {
	s.s2cInfo()
}

func (s *SevenDayReturnMoney) ResetData() {
	state := s.GetYYData()
	if nil == state.SevenDayReturnMoney {
		return
	}
	delete(state.SevenDayReturnMoney, s.Id)
}

func (s *SevenDayReturnMoney) OnEnd() {
	conf := jsondata.GetPYYSevenDayReturnMoneyDayConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	settle, err := s.settle()
	if err != nil {
		s.GetPlayer().LogError("err:%v", err)
		return
	}
	if len(settle) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.Mail,
			Rewards: settle,
		})
	}
}

func (s *SevenDayReturnMoney) NewDay() {
	s.s2cInfo()
}

func (s *SevenDayReturnMoney) GetData() *pb3.PYYSevenDayReturnMoney {
	state := s.GetYYData()
	if nil == state.SevenDayReturnMoney {
		state.SevenDayReturnMoney = make(map[uint32]*pb3.PYYSevenDayReturnMoney)
	}
	if nil == state.SevenDayReturnMoney[s.Id] {
		state.SevenDayReturnMoney[s.Id] = &pb3.PYYSevenDayReturnMoney{}
	}
	dayReturnMoney := state.SevenDayReturnMoney[s.Id]
	if dayReturnMoney.DailyData == nil {
		dayReturnMoney.DailyData = make(map[uint32]*pb3.PYYSevenDayReturnMoneyDailyData)
	}
	return dayReturnMoney
}

func (s *SevenDayReturnMoney) s2cInfo() {
	s.GetPlayer().SendProto3(8, 80, &pb3.S2C_8_80{
		ActiveId: s.GetId(),
		Data:     s.GetData(),
		OpenDay:  s.GetOpenDay(),
	})
}

func (s *SevenDayReturnMoney) c2sRev(msg *base.Message) error {
	var req pb3.C2S_8_81
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	owner := s.GetPlayer()
	awards, err := s.settle()
	if err != nil {
		return neterror.Wrap(err)
	}
	if len(awards) > 0 {
		engine.GiveRewards(owner, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYSevenDayReturnMoneyDailyRecAwards})
		owner.SendShowRewardsPop(awards)
	}
	s.SendProto3(8, 81, &pb3.S2C_8_81{
		ActiveId: s.GetId(),
	})
	return nil
}

func (s *SevenDayReturnMoney) settle() (jsondata.StdRewardVec, error) {
	data := s.GetData()
	if data.IsRec {
		return nil, neterror.ParamsInvalidError("%s already rec", s.GetPrefix())
	}
	conf := jsondata.GetPYYSevenDayReturnMoneyDayConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil, neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}
	if conf.RecCond == nil {
		return nil, neterror.ConfNotFoundError("%s not found rec cond conf", s.GetPrefix())
	}
	openDay := s.GetOpenDay()
	if openDay != 0 && conf.RecCond.Day < openDay {
		return nil, neterror.ParamsInvalidError("%s not reach openDay", s.GetPrefix())
	}
	var totalReturnMt int64
	for _, dailyData := range data.DailyData {
		totalReturnMt += dailyData.DailyReturnMt
	}
	// 没达到最低消费
	totalChargeMoney := s.GetPlayer().GetBinaryData().GetChargeInfo().TotalChargeMoney
	if conf.RecCond.MinChargeAmount != 0 && int64(totalChargeMoney) < conf.RecCond.MinChargeAmount {
		return nil, neterror.ParamsInvalidError("%s not reach MinChargeAmount %d %d", s.GetPrefix(), totalChargeMoney, conf.RecCond.MinChargeAmount)
	}
	if totalReturnMt == 0 {
		return nil, neterror.ParamsInvalidError("%s not can rec %d", s.GetPrefix(), totalReturnMt)
	}
	data.IsRec = true

	idConfByType := jsondata.GetMoneyIdConfByType(conf.Mt)
	return jsondata.StdRewardVec{{
		Id:    idConfByType,
		Count: totalReturnMt,
	}}, nil
}

func (s *SevenDayReturnMoney) checkAddScore(openDay, mt uint32, count int64, _ common.ConsumeParams) {
	conf := jsondata.GetPYYSevenDayReturnMoneyDayConf(s.ConfName, s.ConfIdx)
	owner := s.GetPlayer()
	if conf == nil {
		owner.LogWarn("%s not found conf", s.GetPrefix())
		return
	}
	if conf.Mt != mt {
		return
	}
	data := s.GetData()
	dailyData, ok := data.DailyData[openDay]
	if !ok {
		data.DailyData[openDay] = &pb3.PYYSevenDayReturnMoneyDailyData{}
		dailyData = data.DailyData[openDay]
		dailyData.OpenDay = openDay
	}
	var dailyConf *jsondata.PYYSevenDayReturnMoneyDayConf
	for _, dayConf := range conf.DayConf {
		if dayConf.Day != openDay {
			continue
		}
		dailyConf = dayConf
		break
	}
	if dailyConf == nil {
		owner.LogWarn("%s not found %d day conf", s.GetPrefix(), openDay)
		return
	}
	dailyConsumeMt := dailyData.DailyConsumeMt
	dailyConsumeMt += count
	var dailyReturnMt int64
	for _, phase := range dailyConf.Phase {
		if len(phase.ConsumeRange) != 2 {
			continue
		}
		minMt := phase.ConsumeRange[0]
		maxMt := phase.ConsumeRange[1]
		switch {
		case maxMt != 0:
			if dailyConsumeMt >= maxMt {
				dailyReturnMt += phase.Rate * (maxMt - minMt)
			} else if dailyConsumeMt >= minMt && dailyConsumeMt <= maxMt {
				dailyReturnMt += phase.Rate * (dailyConsumeMt - minMt)
			}
		case maxMt == 0:
			if dailyConsumeMt >= minMt {
				dailyReturnMt += phase.Rate * (dailyConsumeMt - minMt)
			}
		}
	}
	dailyData.DailyConsumeMt = dailyConsumeMt
	dailyData.DailyReturnMt = dailyReturnMt / 10000
	s.SendProto3(8, 82, &pb3.S2C_8_82{
		ActiveId:  s.GetId(),
		DailyData: dailyData,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYSevenDayReturnMoneyDailyAddConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d", openDay, dailyData.DailyConsumeMt, dailyData.DailyReturnMt),
	})
}

func forRangeSevenDayReturnMoney(player iface.IPlayer, f func(s *SevenDayReturnMoney)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYSevenDayReturnMoney)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*SevenDayReturnMoney)
		if !ok || !sys.IsOpen() {
			continue
		}
		f(sys)
	}

	return
}

func checkSevenDayReturnMoneyAddConsume(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	mt, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok {
		return
	}
	params, ok := args[2].(common.ConsumeParams)
	if !ok {
		return
	}

	forRangeSevenDayReturnMoney(player, func(s *SevenDayReturnMoney) {
		s.checkAddScore(s.GetOpenDay(), mt, count, params)
	})
}

func offlineGMSevenDayReturnMoneyDailyData(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		player.LogError("offlineGMSevenDayReturnMoneyDailyData convert CommonSt failed")
		return
	}
	forRangeSevenDayReturnMoney(player, func(s *SevenDayReturnMoney) {
		if s.Id != st.U32Param {
			return
		}
		day := st.U32Param2
		dailyConsumeMt := st.I64Param
		s.GetData().DailyData[day] = &pb3.PYYSevenDayReturnMoneyDailyData{
			OpenDay: day,
		}
		s.checkAddScore(day, moneydef.Diamonds, dailyConsumeMt, common.ConsumeParams{})
		s.s2cInfo()
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSevenDayReturnMoney, func() iface.IPlayerYY {
		return &SevenDayReturnMoney{}
	})

	net.RegisterYYSysProtoV2(8, 81, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SevenDayReturnMoney).c2sRev
	})

	event.RegActorEvent(custom_id.AeConsumeMoney, checkSevenDayReturnMoneyAddConsume)
	engine.RegisterMessage(gshare.OfflineGMSevenDayReturnMoneyDailyData, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineGMSevenDayReturnMoneyDailyData)
}
