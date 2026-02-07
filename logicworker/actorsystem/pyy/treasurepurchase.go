/**
 * @Author: zjj
 * @Date: 2024/8/2
 * @Desc: 武魂庆典-秘宝直购
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type TreasurePurchaseSys struct {
	*PlayerYYBase
}

func (s *TreasurePurchaseSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TreasurePurchaseMap {
		return
	}
	delete(state.TreasurePurchaseMap, s.Id)
}

func (s *TreasurePurchaseSys) GetData() *pb3.PYYTreasurePurchaseData {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TreasurePurchaseMap {
		state.TreasurePurchaseMap = make(map[uint32]*pb3.PYYTreasurePurchaseData)
	}
	if state.TreasurePurchaseMap[s.Id] == nil {
		state.TreasurePurchaseMap[s.Id] = &pb3.PYYTreasurePurchaseData{}
	}
	if state.TreasurePurchaseMap[s.Id].Day == 0 {
		state.TreasurePurchaseMap[s.Id].Day = s.GetOpenDay()
	}
	if state.TreasurePurchaseMap[s.Id].DailyDataMap == nil {
		state.TreasurePurchaseMap[s.Id].DailyDataMap = make(map[uint32]*pb3.PYYTreasurePurchaseDailyData)
	}
	return state.TreasurePurchaseMap[s.Id]
}

func (s *TreasurePurchaseSys) S2CInfo() {
	s.SendProto3(61, 30, &pb3.S2C_61_30{
		ActiveId: s.Id,
		Data:     s.GetData(),
	})
}

func (s *TreasurePurchaseSys) OnOpen() {
	s.S2CInfo()
}

func (s *TreasurePurchaseSys) Login() {
	s.S2CInfo()
}

func (s *TreasurePurchaseSys) OnReconnect() {
	s.S2CInfo()
}

func (s *TreasurePurchaseSys) onNewDay() {
	err := s.recAwards(func(awards jsondata.StdRewardVec) {
		if len(awards) == 0 {
			return
		}
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_TreasurePurchaseGiveMailAwards,
			Rewards: awards,
		})
	})
	if err != nil {
		s.GetPlayer().LogError("onNewDay err:%v", err)
	}
	s.GetData().Day = s.GetOpenDay()
	s.S2CInfo()
}

func (s *TreasurePurchaseSys) handleCharge(conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) {
	purchaseConf, ok := jsondata.GetPYYTreasurePurchaseConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.GetData()
	day := data.Day
	dailyData, ok := data.DailyDataMap[day]
	if !ok {
		data.DailyDataMap[day] = &pb3.PYYTreasurePurchaseDailyData{}
		dailyData = data.DailyDataMap[day]
	}

	dailyConf, ok := purchaseConf.DailyConf[day]
	if !ok {
		return
	}

	goods, ok := dailyConf.Goods[conf.ChargeId]
	if !ok {
		return
	}

	if pie.Uint32s(dailyData.ChargeIds).Contains(conf.ChargeId) {
		return
	}

	dailyData.ChargeIds = append(dailyData.ChargeIds, conf.ChargeId)
	if len(goods.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), goods.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTreasurePurchaseGiveAwards,
		})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogTreasurePurchaseGiveAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", conf.ChargeId),
	})

	s.SendProto3(61, 32, &pb3.S2C_61_32{
		ActiveId: s.Id,
		ChargeId: conf.ChargeId,
		Day:      day,
	})
	return
}

func (s *TreasurePurchaseSys) handleCanCharge(conf *jsondata.ChargeConf) bool {
	purchaseConf, ok := jsondata.GetPYYTreasurePurchaseConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false
	}

	data := s.GetData()
	day := data.Day
	dailyData, ok := data.DailyDataMap[day]
	if !ok {
		data.DailyDataMap[day] = &pb3.PYYTreasurePurchaseDailyData{}
		dailyData = data.DailyDataMap[day]
	}

	dailyConf, ok := purchaseConf.DailyConf[day]
	if !ok {
		return false
	}

	_, ok = dailyConf.Goods[conf.ChargeId]
	if !ok {
		return false
	}

	if pie.Uint32s(dailyData.ChargeIds).Contains(conf.ChargeId) {
		return false
	}

	return true
}

func (s *TreasurePurchaseSys) recAwards(giveAwardsFunc func(awards jsondata.StdRewardVec)) error {
	conf, ok := jsondata.GetPYYTreasurePurchaseConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data := s.GetData()
	day := data.Day
	dailyConf, ok := conf.DailyConf[day]
	if !ok {
		return neterror.ConfNotFoundError("%s %d not found conf", s.GetPrefix(), day)
	}

	dailyData, ok := data.DailyDataMap[day]
	if !ok {
		data.DailyDataMap[day] = &pb3.PYYTreasurePurchaseDailyData{}
		dailyData = data.DailyDataMap[day]
	}

	if dailyData.RecLastAwards {
		return neterror.ParamsInvalidError("%s %d already rec", s.GetPrefix(), day)
	}

	if len(dailyData.ChargeIds) != len(dailyConf.Goods) {
		return neterror.ParamsInvalidError("%s %d cond not reach", s.GetPrefix(), day)
	}

	dailyData.RecLastAwards = true

	if giveAwardsFunc == nil {
		return neterror.ParamsInvalidError("not found give awards function")
	}
	giveAwardsFunc(dailyConf.BuyEndAwards)
	return nil
}

func (s *TreasurePurchaseSys) OnEnd() {
	s.onNewDay()
}

func (s *TreasurePurchaseSys) c2sAward(_ *base.Message) error {
	return s.recAwards(func(awards jsondata.StdRewardVec) {
		day := s.GetData().Day
		if len(awards) > 0 {
			engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogTreasurePurchaseGiveLastAwards,
			})
		}
		logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogTreasurePurchaseGiveLastAwards, &pb3.LogPlayerCounter{
			NumArgs: uint64(s.GetId()),
			StrArgs: fmt.Sprintf("%d", day),
		})

		s.SendProto3(61, 33, &pb3.S2C_61_33{
			ActiveId: s.Id,
			Day:      day,
		})
	})
}

func (s *TreasurePurchaseSys) c2sDailyAward(_ *base.Message) error {
	conf, ok := jsondata.GetPYYTreasurePurchaseConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data := s.GetData()
	day := data.Day
	dailyConf, ok := conf.DailyConf[day]
	if !ok {
		return neterror.ConfNotFoundError("%s %d not found conf", s.GetPrefix(), day)
	}

	dailyData, ok := data.DailyDataMap[day]
	if !ok {
		data.DailyDataMap[day] = &pb3.PYYTreasurePurchaseDailyData{}
		dailyData = data.DailyDataMap[day]
	}

	if dailyData.Receive {
		return neterror.ParamsInvalidError("%s %d already rec", s.GetPrefix(), day)
	}

	dailyData.Receive = true
	if len(dailyConf.DailyFreeAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), dailyConf.DailyFreeAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTreasurePurchaseGiveDailyAwards,
		})
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogTreasurePurchaseGiveDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", day),
	})

	s.SendProto3(61, 34, &pb3.S2C_61_34{
		ActiveId: s.Id,
		Day:      day,
	})
	return nil
}

func rangeTreasurePurchaseSys(player iface.IPlayer, doLogic func(yy iface.IPlayerYY)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PyyTreasurePurchase)
	if len(yyList) == 0 {
		player.LogWarn("not found yy obj ,class:%d", yydefine.PyyTreasurePurchase)
		return
	}
	for i := range yyList {
		v := yyList[i]
		doLogic(v)
	}
}

func treasurePurchaseChargeHandler(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	rangeTreasurePurchaseSys(actor, func(yy iface.IPlayerYY) {
		sys, ok := yy.(*TreasurePurchaseSys)
		if !ok {
			return
		}
		sys.handleCharge(conf, params)
	})
	return true
}
func treasurePurchaseChargeCheckFunHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var ret bool
	rangeTreasurePurchaseSys(actor, func(yy iface.IPlayerYY) {
		if ret {
			return
		}
		sys, ok := yy.(*TreasurePurchaseSys)
		if !ok {
			return
		}
		ret = sys.handleCanCharge(conf)
	})
	return true
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PyyTreasurePurchase, func() iface.IPlayerYY {
		return &TreasurePurchaseSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	// 注册跨天
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		rangeTreasurePurchaseSys(player, func(yy iface.IPlayerYY) {
			sys, ok := yy.(*TreasurePurchaseSys)
			if !ok || !sys.IsOpen() {
				return
			}
			sys.onNewDay()
		})
	})
	engine.RegChargeEvent(chargedef.TreasurePurchase, treasurePurchaseChargeCheckFunHandler, treasurePurchaseChargeHandler)
	net.RegisterYYSysProtoV2(61, 33, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*TreasurePurchaseSys).c2sAward
	})
	net.RegisterYYSysProtoV2(61, 34, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*TreasurePurchaseSys).c2sDailyAward
	})
}
