/**
 * @Author: lzp
 * @Date: 2025/12/15
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	LuxuryDayGiftType1 = 1 // 单个购买
	LuxuryDayGiftType2 = 2 // 一次购买
)

type LuxuryDayGiftSys struct {
	Base
}

func (s *LuxuryDayGiftSys) OnLogin() {
	s.s2cInfo()
}

func (s *LuxuryDayGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LuxuryDayGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *LuxuryDayGiftSys) OnNewDay() {
	data := s.GetData()
	data.IsRevDayRewards = false
	for id := range data.Gifts {
		gConf := jsondata.GetLuxuryDayGiftConf(id)
		if gConf != nil && gConf.Reset {
			data.Gifts[id] = 0
		}
	}
	s.s2cInfo()
}

func (s *LuxuryDayGiftSys) GetData() *pb3.LuxuryDayGiftData {
	data := s.GetBinaryData().LuxuryDayGiftData
	if data == nil {
		data = &pb3.LuxuryDayGiftData{}
		s.GetBinaryData().LuxuryDayGiftData = data
	}
	if data.Gifts == nil {
		data.Gifts = make(map[uint32]uint32)
	}
	return data
}

func (s *LuxuryDayGiftSys) s2cInfo() {
	s.SendProto3(83, 10, &pb3.S2C_83_10{
		Data: s.GetData(),
	})
}

func (s *LuxuryDayGiftSys) c2sFetchDayRewards(msg *base.Message) error {
	var req pb3.C2S_83_11
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.LuxuryDayGiftConfMgr
	if conf == nil {
		return neterror.ParamsInvalidError("config missing")
	}

	data := s.GetData()
	if data.IsRevDayRewards {
		return neterror.ParamsInvalidError("already received")
	}

	data.IsRevDayRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuxuryDayGiftDayReward})
	}
	s.s2cInfo()
	return nil
}

func luxuryDayGiftChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiLuxuryDayGift).(*LuxuryDayGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	gConf := jsondata.GetLuxuryDayGiftConfByChargeId(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	if gConf.GiftType == LuxuryDayGiftType1 {
		if data.Gifts[gConf.Id] >= gConf.Count {
			return false
		}
	}
	if gConf.GiftType == LuxuryDayGiftType2 {
		for _, count := range data.Gifts {
			if count > 0 {
				return false
			}
		}
	}
	return true
}

func luxuryDayGiftChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiLuxuryDayGift).(*LuxuryDayGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	conf := jsondata.LuxuryDayGiftConfMgr
	if conf == nil {
		return false
	}

	gConf := jsondata.GetLuxuryDayGiftConfByChargeId(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	var rewards jsondata.StdRewardVec

	if gConf.GiftType == LuxuryDayGiftType1 {
		data.Gifts[gConf.Id] += 1
		rewards = gConf.Rewards
	}
	if gConf.GiftType == LuxuryDayGiftType2 {
		for _, gConf := range conf.Gifts {
			data.Gifts[gConf.Id] += gConf.Count
			rewardsMulti := jsondata.StdRewardMulti(gConf.Rewards, int64(gConf.Count))
			rewards = append(rewards, rewardsMulti...)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuxuryDayGiftBuyPayReward})
	}

	logworker.LogPlayerBehavior(player, pb3.LogId_LogLuxuryDayGiftBuyPayReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(gConf.Id),
	})
	player.SendShowRewardsPop(rewards)
	engine.BroadcastTipMsgById(conf.BroadcastId, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, rewards))
	sys.s2cInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiLuxuryDayGift, func() iface.ISystem {
		return &LuxuryDayGiftSys{}
	})

	net.RegisterSysProtoV2(83, 11, sysdef.SiLuxuryDayGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuxuryDayGiftSys).c2sFetchDayRewards
	})

	engine.RegChargeEvent(chargedef.LuxuryDayGift, luxuryDayGiftChargeCheck, luxuryDayGiftChargeBack)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiLuxuryDayGift).(*LuxuryDayGiftSys); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})
}
