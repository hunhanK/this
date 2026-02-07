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
	HeavenGiftType1 = 1 // 单个购买
	HeavenGiftType2 = 2 // 一次购买
)

type HeavenGiftSys struct {
	Base
}

func (s *HeavenGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HeavenGiftSys) OnLogin() {
	s.s2cInfo()
}

func (s *HeavenGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *HeavenGiftSys) OnNewDay() {
	data := s.GetData()
	data.IsRevDayRewards = false
	for id := range data.Gifts {
		gConf := jsondata.GetHeavenGiftConf(id)
		if gConf != nil && gConf.Reset {
			data.Gifts[id] = 0
		}
	}
	s.s2cInfo()
}

func (s *HeavenGiftSys) GetData() *pb3.HeavenGiftData {
	data := s.GetBinaryData().HeavenGiftData
	if data == nil {
		data = &pb3.HeavenGiftData{}
		s.GetBinaryData().HeavenGiftData = data
	}
	if data.Gifts == nil {
		data.Gifts = make(map[uint32]uint32)
	}
	return data
}

func (s *HeavenGiftSys) s2cInfo() {
	s.SendProto3(83, 15, &pb3.S2C_83_15{
		Data: s.GetData(),
	})
}

func (s *HeavenGiftSys) c2sFetchDayRewards(msg *base.Message) error {
	var req pb3.C2S_83_16
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.HeavenGiftConfMgr
	if conf == nil {
		return neterror.ParamsInvalidError("config missing")
	}

	if !checkIsOpen(s.GetOwner()) {
		return neterror.ParamsInvalidError("system not open")
	}

	data := s.GetData()
	if data.IsRevDayRewards {
		return neterror.ParamsInvalidError("already received")
	}

	data.IsRevDayRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogHeavenGiftBuyPayReward})
	}
	s.s2cInfo()
	return nil
}

func checkIsOpen(player iface.IPlayer) bool {
	goldenSys, ok := player.GetSysObj(sysdef.SiGoldenPigTreasure).(*GoldenPigTreasureSys)
	if !ok || !goldenSys.IsOpen() {
		return false
	}
	if !goldenSys.CheckBoughtAllGifts() {
		return false
	}
	return true
}

func heavenGiftChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiHeavenGift).(*HeavenGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	if !checkIsOpen(player) {
		return false
	}

	gConf := jsondata.GetHeavenGiftConfByChargeId(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	if gConf.GiftType == HeavenGiftType1 {
		if data.Gifts[gConf.Id] >= gConf.Count {
			return false
		}
	}
	if gConf.GiftType == HeavenGiftType2 {
		for _, count := range data.Gifts {
			if count > 0 {
				return false
			}
		}
	}
	return true
}

func heavenGiftChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiHeavenGift).(*HeavenGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	conf := jsondata.HeavenGiftConfMgr
	if conf == nil {
		return false
	}

	gConf := jsondata.GetHeavenGiftConfByChargeId(chargeConf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	var rewards jsondata.StdRewardVec

	if gConf.GiftType == HeavenGiftType1 {
		data.Gifts[gConf.Id] += 1
		rewards = gConf.Rewards
	}
	if gConf.GiftType == HeavenGiftType2 {
		for _, gConf := range conf.Gifts {
			data.Gifts[gConf.Id] += gConf.Count
			rewardsMulti := jsondata.StdRewardMulti(gConf.Rewards, int64(gConf.Count))
			rewards = append(rewards, rewardsMulti...)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogHeavenGiftBuyPayReward})
	}

	logworker.LogPlayerBehavior(player, pb3.LogId_LogHeavenGiftBuyPayReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(gConf.Id),
	})
	player.SendShowRewardsPop(rewards)
	engine.BroadcastTipMsgById(conf.BroadcastId, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, rewards))
	sys.s2cInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiHeavenGift, func() iface.ISystem {
		return &HeavenGiftSys{}
	})

	net.RegisterSysProtoV2(83, 16, sysdef.SiHeavenGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenGiftSys).c2sFetchDayRewards
	})

	engine.RegChargeEvent(chargedef.HeavenGift, heavenGiftChargeCheck, heavenGiftChargeBack)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiHeavenGift).(*HeavenGiftSys); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})
}
