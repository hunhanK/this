/**
 * @Author: lzp
 * @Date: 2025/4/1
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type PlayInvestSys struct {
	Base
}

const (
	PlayInvestChargeTypeDirect = 1
	PlayInvestChargeTypeQuick  = 2
)

func (s *PlayInvestSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlayInvestSys) OnLogin() {
	s.s2cInfo()
}

func (s *PlayInvestSys) OnOpen() {
	s.s2cInfo()
}

func (s *PlayInvestSys) GetData() *pb3.PlayInvestData {
	binary := s.GetBinaryData()
	if binary.InvestData == nil {
		binary.InvestData = &pb3.PlayInvestData{}
	}
	investData := binary.InvestData
	if investData.Gifts == nil {
		investData.Gifts = make(map[uint32]*pb3.PlayInvest)
	}
	return binary.InvestData
}

func (s *PlayInvestSys) onNewDay() {
	data := s.GetData()
	data.IsFetchDRewards = false
	s.s2cInfo()
}

func (s *PlayInvestSys) s2cInfo() {
	s.SendProto3(2, 210, &pb3.S2C_2_210{Data: s.GetData()})
}

func (s *PlayInvestSys) s2cPlayInvest(giftId uint32) {
	data := s.GetData()
	s.SendProto3(2, 214, &pb3.S2C_2_214{PlayInvest: data.Gifts[giftId]})
}

func (s *PlayInvestSys) c2sFetchDailyRewards(msg *base.Message) error {
	var req pb3.C2S_2_211
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	data := s.GetData()
	if data.IsFetchDRewards {
		return neterror.ParamsInvalidError("daily awards has fetched")
	}

	data.IsFetchDRewards = true
	dRewards := jsondata.GetPlayInvestDailyRewards()
	if len(dRewards) > 0 {
		engine.GiveRewards(s.GetOwner(), dRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlayInvestDailyRewards})
	}

	s.SendProto3(2, 211, &pb3.S2C_2_211{IsFetch: true})
	return nil
}

func (s *PlayInvestSys) c2sFetchDayRewards(msg *base.Message) error {
	var req pb3.C2S_2_212
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	gConf := jsondata.GetPlayInvestGiftConf(req.GiftId)
	if gConf == nil {
		return neterror.ConfNotFoundError("playInvest giftId=%d not found", req.GiftId)
	}

	data := s.GetData()
	gData, ok := data.Gifts[req.GiftId]
	if !ok || !gData.IsBuy {
		return neterror.ParamsInvalidError("playInvest giftId=%d not buy", req.GiftId)
	}

	if utils.SliceContainsUint32(gData.DayRewards, req.Day) {
		return neterror.ParamsInvalidError("playInvest giftId=%d day=%d rewards has fetched", req.GiftId, req.Day)
	}

	durDay := time_util.TimestampSubDays(gData.Timestamp, time_util.NowSec()) + 1
	if req.Day > durDay {
		return neterror.ParamsInvalidError("playInvest giftId=%d day=%d rewards day limit", req.GiftId, req.Day)
	}

	dConf := jsondata.GetPlayInvestDayRewards(req.GiftId, req.Day)
	if dConf == nil {
		return neterror.ConfNotFoundError("playInvest giftId=%d day=%d rewards not found", req.GiftId, req.Day)
	}

	gData.DayRewards = append(gData.DayRewards, req.Day)
	if len(dConf.Rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), dConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlayInvestDayRewards})
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogPlayInvestDayRewards, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"giftId": req.GiftId,
			"day":    req.Day,
		}),
	})
	s.SendProto3(2, 212, &pb3.S2C_2_212{
		GiftId: req.GiftId,
		Day:    req.Day,
		Awards: jsondata.StdRewardVecToPb3RewardVec(dConf.Rewards),
	})
	s.s2cPlayInvest(req.GiftId)
	return nil
}

func (s *PlayInvestSys) c2sFetchDayRewardsQuick(msg *base.Message) error {
	var req pb3.C2S_2_214
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	var rewards jsondata.StdRewardVec
	data := s.GetData()
	for giftId, gData := range data.Gifts {
		gConf := jsondata.GetPlayInvestGiftConf(giftId)
		if gConf == nil {
			continue
		}
		if gConf.Type != PlayInvestChargeTypeDirect {
			continue
		}
		if !gData.IsBuy {
			continue
		}
		durDay := time_util.TimestampSubDays(gData.Timestamp, time_util.NowSec()) + 1
		for _, dayConf := range gConf.DayRewards {
			if utils.SliceContainsUint32(gData.DayRewards, dayConf.Day) {
				continue
			}
			if dayConf.Day > durDay {
				continue
			}
			rewards = append(rewards, dayConf.Rewards...)
			gData.DayRewards = append(gData.DayRewards, dayConf.Day)
		}
	}
	if len(rewards) == 0 {
		return neterror.ParamsInvalidError("no rewards")
	}

	engine.GiveRewards(s.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlayInvestDayRewardsQuick})

	s.s2cInfo()
	return nil
}

func (s *PlayInvestSys) chargeCheck(chargeId uint32) bool {
	conf := jsondata.GetPlayInvestGiftConfByChargeId(chargeId)
	if conf == nil {
		return false
	}
	data := s.GetData()
	checkBuy := func(giftId uint32) bool {
		gConf := jsondata.GetPlayInvestGiftConf(chargeId)
		if gConf == nil {
			return false
		}
		gData, ok := data.Gifts[giftId]
		if ok && gData.IsBuy {
			return true
		}
		return false
	}

	if checkBuy(conf.GiftId) {
		return false
	}

	switch conf.Type {
	case PlayInvestChargeTypeDirect:
		return true
	case PlayInvestChargeTypeQuick:
		for _, giftId := range conf.PackIds {
			if checkBuy(giftId) {
				return false
			}
		}
		return true
	}
	return false
}

func (s *PlayInvestSys) chargeBack(chargeId uint32) bool {
	if !s.chargeCheck(chargeId) {
		return false
	}

	data := s.GetData()
	nowSec := time_util.NowSec()

	setBuy := func(giftId uint32) {
		data.Gifts[giftId] = &pb3.PlayInvest{
			GiftId:    giftId,
			IsBuy:     true,
			Timestamp: nowSec,
		}
		s.s2cPlayInvest(giftId)
	}

	conf := jsondata.GetPlayInvestGiftConfByChargeId(chargeId)
	if conf == nil {
		return false
	}

	var rewards jsondata.StdRewardVec

	setBuy(conf.GiftId)

	switch conf.Type {
	case PlayInvestChargeTypeDirect:
		rewards = append(rewards, conf.Rewards...)
	case PlayInvestChargeTypeQuick:
		for _, giftId := range conf.PackIds {
			setBuy(giftId)
			gConf := jsondata.GetPlayInvestGiftConfByChargeId(chargeId)
			if gConf == nil {
				continue
			}
			rewards = append(rewards, gConf.Rewards...)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPlayInvestBuyRewards})
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogPlayInvestBuyRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(conf.GiftId),
	})
	return true
}

func PlayInvestGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiPlayInvest).(*PlayInvestSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeCheck(conf.ChargeId)
}

func PlayInvestGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiPlayInvest).(*PlayInvestSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeBack(conf.ChargeId)
}

func init() {
	RegisterSysClass(sysdef.SiPlayInvest, func() iface.ISystem {
		return &PlayInvestSys{}
	})

	net.RegisterSysProto(2, 211, sysdef.SiPlayInvest, (*PlayInvestSys).c2sFetchDailyRewards)
	net.RegisterSysProto(2, 212, sysdef.SiPlayInvest, (*PlayInvestSys).c2sFetchDayRewards)
	net.RegisterSysProto(2, 214, sysdef.SiPlayInvest, (*PlayInvestSys).c2sFetchDayRewardsQuick)

	engine.RegChargeEvent(chargedef.PlayInvestGift, PlayInvestGiftCheck, PlayInvestGiftChargeBack)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiPlayInvest).(*PlayInvestSys); ok && s.IsOpen() {
			s.onNewDay()
		}
	})
}
