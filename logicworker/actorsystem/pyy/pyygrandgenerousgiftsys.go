/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
)

type GrandGenerousGiftSys struct {
	PlayerYYBase
}

func (s *GrandGenerousGiftSys) s2cInfo() {
	s.SendProto3(10, 10, &pb3.S2C_10_10{
		Data:     s.getData(),
		ActiveId: s.GetId(),
	})
}

func (s *GrandGenerousGiftSys) getData() *pb3.PYYGrandGenerousGiftData {
	state := s.GetYYData()
	if nil == state.GrandGenerousGiftData {
		state.GrandGenerousGiftData = make(map[uint32]*pb3.PYYGrandGenerousGiftData)
	}
	if state.GrandGenerousGiftData[s.Id] == nil {
		state.GrandGenerousGiftData[s.Id] = &pb3.PYYGrandGenerousGiftData{}
	}
	return state.GrandGenerousGiftData[s.Id]
}

func (s *GrandGenerousGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.GrandGenerousGiftData {
		return
	}
	delete(state.GrandGenerousGiftData, s.Id)
}

func (s *GrandGenerousGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GrandGenerousGiftSys) Login() {
	s.s2cInfo()
}

func (s *GrandGenerousGiftSys) OnOpen() {
	s.checkNewGift()
	s.getData().ActDay = s.GetOpenDay()
	s.s2cInfo()
}

func (s *GrandGenerousGiftSys) BeforeNewDay() {
	s.getData().TodayGiftId = 0
	s.checkNewGift()
}

func (s *GrandGenerousGiftSys) NewDay() {
	s.getData().ActDay = s.GetOpenDay()
	s.s2cInfo()
}

func (s *GrandGenerousGiftSys) checkNewGift() {
	data := s.getData()
	actDay := data.ActDay
	openDay := s.GetOpenDay()
	if actDay == 0 {
		if openDay == 1 {
			return
		}
		// 直接触发礼包
		generousGiftConfig := jsondata.GetGrandGenerousGiftConfig(s.ConfName, s.ConfIdx)
		if generousGiftConfig == nil {
			return
		}
		dailyGift := generousGiftConfig.GetGiftConfigByActDay(openDay)
		if dailyGift == nil {
			return
		}
		data.TodayGiftId = dailyGift.Id
		return
	}
	// 同一天 或者 出问题了
	if actDay >= openDay {
		return
	}

	// 隔天了才上线
	if actDay+1 < openDay {
		// 直接触发礼包
		generousGiftConfig := jsondata.GetGrandGenerousGiftConfig(s.ConfName, s.ConfIdx)
		if generousGiftConfig == nil {
			return
		}
		dailyGift := generousGiftConfig.GetGiftConfigByActDay(openDay)
		if dailyGift == nil {
			return
		}
		data.TodayGiftId = dailyGift.Id
		return
	}

	lastDailyChargeMoney := s.GetDailyCharge()
	generousGiftConfig := jsondata.GetGrandGenerousGiftConfig(s.ConfName, s.ConfIdx)
	if generousGiftConfig == nil {
		return
	}
	dailyGift := generousGiftConfig.GetGiftConfigByActDay(openDay)
	if dailyGift == nil {
		return
	}
	if dailyGift.Cond != nil && dailyGift.Cond.LastDayChargeMoney != 0 && dailyGift.Cond.LastDayChargeMoney < lastDailyChargeMoney {
		return
	}
	data.TodayGiftId = dailyGift.Id
}

func (s *GrandGenerousGiftSys) checkCanCharge(chargeId uint32) bool {
	data := s.getData()
	generousGiftConfig := jsondata.GetGrandGenerousGiftConfig(s.ConfName, s.ConfIdx)
	if generousGiftConfig == nil {
		return false
	}
	dailyGift := generousGiftConfig.GetGiftConfigByChargeId(chargeId)
	if dailyGift == nil {
		return false
	}
	if data.TodayGiftId != dailyGift.Id {
		return false
	}
	if pie.Uint32s(data.BuyIds).Contains(dailyGift.Id) {
		return false
	}
	return true
}

func checkPYYGrandGenerousGift(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(player, yydefine.PYYGrandGenerousGift, func(obj iface.IPlayerYY) {
		if ret {
			return
		}
		sys := obj.(*GrandGenerousGiftSys)
		if sys == nil {
			return
		}
		ret = sys.checkCanCharge(chargeConf.ChargeId)
	})
	return ret
}

func chargePYYGrandGenerousGift(player iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(player, yydefine.PYYGrandGenerousGift, func(obj iface.IPlayerYY) {
		if ret {
			return
		}
		sys := obj.(*GrandGenerousGiftSys)
		if sys == nil {
			return
		}
		if !sys.checkCanCharge(chargeConf.ChargeId) {
			return
		}

		data := sys.getData()
		generousGiftConfig := jsondata.GetGrandGenerousGiftConfig(sys.ConfName, sys.ConfIdx)
		if generousGiftConfig == nil {
			return
		}
		dailyGift := generousGiftConfig.GetGiftConfigByChargeId(chargeConf.ChargeId)
		if dailyGift == nil {
			return
		}
		engine.GiveRewards(player, dailyGift.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogGrandGenerousGiftAward,
		})
		data.BuyIds = append(data.BuyIds, dailyGift.Id)
		sys.SendProto3(10, 11, &pb3.S2C_10_11{
			ActiveId: sys.GetId(),
			BuyIds:   data.BuyIds,
		})
		logworker.LogPlayerBehavior(player, pb3.LogId_LogGrandGenerousGiftAward, &pb3.LogPlayerCounter{
			NumArgs: uint64(sys.GetId()),
			StrArgs: fmt.Sprintf("%v", dailyGift.Id),
		})
		ret = true
	})
	return ret
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYGrandGenerousGift, func() iface.IPlayerYY {
		return &GrandGenerousGiftSys{}
	})
	engine.RegChargeEvent(chargedef.PYYGrandGenerousGift, checkPYYGrandGenerousGift, chargePYYGrandGenerousGift)
}
