/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type LifetimeBuyoutEventGiftSys struct {
	Base
}

func (s *LifetimeBuyoutEventGiftSys) s2cInfo() {
	s.SendProto3(10, 30, &pb3.S2C_10_30{
		Data: s.getData(),
	})
}

func (s *LifetimeBuyoutEventGiftSys) getData() *pb3.LifetimeBuyoutEventGiftData {
	data := s.GetBinaryData().LifetimeBuyoutEventGiftData
	if data == nil {
		s.GetBinaryData().LifetimeBuyoutEventGiftData = &pb3.LifetimeBuyoutEventGiftData{}
		data = s.GetBinaryData().LifetimeBuyoutEventGiftData
	}
	if data.GiftMap == nil {
		data.GiftMap = make(map[uint32]*pb3.LifetimeBuyoutEventGiftEntry)
	}
	return data
}

func (s *LifetimeBuyoutEventGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LifetimeBuyoutEventGiftSys) OnLogin() {
	s.s2cInfo()
}

func (s *LifetimeBuyoutEventGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *LifetimeBuyoutEventGiftSys) c2sRecFreeAwards(msg *base.Message) error {
	var req pb3.C2S_10_31
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	dailyAwardRecAt := data.DailyAwardRecAt
	nowSec := time_util.NowSec()
	if dailyAwardRecAt != 0 && time_util.IsSameDay(dailyAwardRecAt, nowSec) {
		return neterror.ParamsInvalidError("already rec free awards")
	}
	commonGiftConf := jsondata.GetLifetimeBuyoutCommonGiftConf()
	if commonGiftConf == nil {
		return neterror.ParamsInvalidError("no common gift conf")
	}
	data.DailyAwardRecAt = nowSec
	owner := s.GetOwner()
	engine.GiveRewards(owner, commonGiftConf.FreeAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogLifetimeBuyoutCommonGiftRecFreeAwards,
	})
	s.SendProto3(10, 31, &pb3.S2C_10_31{
		DailyAwardRecAt: data.DailyAwardRecAt,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLifetimeBuyoutCommonGiftRecFreeAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.DailyAwardRecAt),
	})
	return nil
}
func (s *LifetimeBuyoutEventGiftSys) c2sRecDailyAwards(msg *base.Message) error {
	var req pb3.C2S_10_32
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	packId := req.PackId
	config := jsondata.GetLifetimeBuyoutEventGiftConfig(packId)
	if config == nil {
		return neterror.ParamsInvalidError("no config %d", packId)
	}

	openServerDay := gshare.GetOpenServerDay()
	if openServerDay < config.OpenSrvDay {
		return neterror.ParamsInvalidError("not open gift %d %d", openServerDay, config.OpenSrvDay)
	}

	data := s.getData()
	entry := data.GiftMap[packId]
	if entry == nil {
		return neterror.ParamsInvalidError("%d not buy gift", packId)
	}
	if entry.BuyLimit < config.BuyLimit {
		return neterror.ParamsInvalidError("buy not limit %d %d", entry.BuyLimit, config.BuyLimit)
	}

	nowSec := time_util.NowSec()
	dailyAwardRecAt := entry.DailyAwardRecAt
	if dailyAwardRecAt != 0 && time_util.IsSameDay(dailyAwardRecAt, nowSec) {
		return neterror.ParamsInvalidError("already rec free awards")
	}

	entry.DailyAwardRecAt = nowSec
	owner := s.GetOwner()
	engine.GiveRewards(owner, config.DailyAwards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogLifetimeBuyoutCommonGiftRecDailyAwards,
		NoTips: true,
	})
	owner.SendShowRewardsPop(config.DailyAwards)
	s.SendProto3(10, 32, &pb3.S2C_10_32{
		PackId:          packId,
		DailyAwardRecAt: nowSec,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLifetimeBuyoutCommonGiftRecDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(packId),
		StrArgs: fmt.Sprintf("%d", nowSec),
	})
	return nil
}

func getLifetimeBuyoutEventGiftSys(player iface.IPlayer) *LifetimeBuyoutEventGiftSys {
	obj := player.GetSysObj(sysdef.SiLifetimeBuyoutEventGift)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*LifetimeBuyoutEventGiftSys)
	if !ok {
		return nil
	}
	return sys
}

func checkLifetimeBuyoutEventGift(actor iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	s := getLifetimeBuyoutEventGiftSys(actor)
	if s == nil {
		return false
	}

	// 是否是普通购买
	var isQuickBuy bool
	config := jsondata.GetLifetimeBuyoutEventGiftConfigByChargeId(chargeConf.ChargeId)
	if config == nil {
		config = jsondata.GetLifetimeBuyoutEventGiftConfigByQuickChargeId(chargeConf.ChargeId)
		if config != nil {
			isQuickBuy = true
		}
	}
	if config == nil {
		return false
	}

	data := s.getData()
	giftEntry := data.GiftMap[config.PackId]
	if giftEntry == nil {
		giftEntry = &pb3.LifetimeBuyoutEventGiftEntry{
			PackId: config.PackId,
		}
		data.GiftMap[config.PackId] = giftEntry
	}

	// 一键购买不能买过礼包
	if isQuickBuy {
		if giftEntry.DailyLimit != 0 {
			return false
		}
		return true
	}

	// 不能超过今日上限
	if giftEntry.DailyLimit >= config.DailyLimit {
		return false
	}

	// 不能超过总上限
	if giftEntry.BuyLimit >= config.BuyLimit {
		return false
	}
	return true
}

func chargeLifetimeBuyoutEventGift(actor iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !checkLifetimeBuyoutEventGift(actor, chargeConf) {
		return false
	}

	s := getLifetimeBuyoutEventGiftSys(actor)
	if s == nil {
		return false
	}

	// 是否是普通购买
	var isQuickBuy bool
	config := jsondata.GetLifetimeBuyoutEventGiftConfigByChargeId(chargeConf.ChargeId)
	if config == nil {
		config = jsondata.GetLifetimeBuyoutEventGiftConfigByQuickChargeId(chargeConf.ChargeId)
		if config != nil {
			isQuickBuy = true
		}
	}
	if config == nil {
		return false
	}

	data := s.getData()
	giftEntry := data.GiftMap[config.PackId]

	if isQuickBuy {
		rewardMulti := jsondata.StdRewardMulti(config.Awards, int64(config.BuyLimit))
		giftEntry.BuyLimit = config.BuyLimit
		giftEntry.DailyAwardRecAt = time_util.NowSec()
		engine.GiveRewards(actor, rewardMulti, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogLifetimeBuyoutGiftChargeGift,
			NoTips: true,
		})
		actor.SendShowRewardsPop(config.Awards)
		s.SendProto3(10, 33, &pb3.S2C_10_33{
			Data: giftEntry,
		})
		logworker.LogPlayerBehavior(actor, pb3.LogId_LogLifetimeBuyoutGiftChargeGift, &pb3.LogPlayerCounter{
			NumArgs: uint64(config.PackId),
			StrArgs: fmt.Sprintf("%d_%v", chargeConf.ChargeId, isQuickBuy),
		})
		engine.BroadcastTipMsgById(tipmsgid.LifetimeBuyoutEventGiftTip2, actor.GetId(), actor.GetName(), config.Name, config.GetWay)
		return true
	}

	giftEntry.BuyLimit += 1
	giftEntry.DailyLimit += 1
	if giftEntry.BuyLimit >= config.BuyLimit {
		giftEntry.DailyAwardRecAt = time_util.NowSec()
	}

	engine.GiveRewards(actor, config.Awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogLifetimeBuyoutGiftChargeGift,
		NoTips: true,
	})
	actor.SendShowRewardsPop(config.Awards)

	s.SendProto3(10, 33, &pb3.S2C_10_33{
		Data: giftEntry,
	})
	logworker.LogPlayerBehavior(actor, pb3.LogId_LogLifetimeBuyoutGiftChargeGift, &pb3.LogPlayerCounter{
		NumArgs: uint64(config.PackId),
		StrArgs: fmt.Sprintf("%d_%v", chargeConf.ChargeId, isQuickBuy),
	})
	engine.BroadcastTipMsgById(tipmsgid.LifetimeBuyoutEventGiftTip1, actor.GetId(), actor.GetName(), config.Name, engine.StdRewardToBroadcast(actor, config.Awards), config.GetWay)
	return true
}

func handleLifetimeBuyoutEventGiftNewDay(player iface.IPlayer, args ...interface{}) {
	sys := getLifetimeBuyoutEventGiftSys(player)
	if sys == nil {
		return
	}
	data := sys.getData()
	for _, entry := range data.GiftMap {
		entry.DailyLimit = 0
	}
	sys.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiLifetimeBuyoutEventGift, func() iface.ISystem {
		return &LifetimeBuyoutEventGiftSys{}
	})
	net.RegisterSysProtoV2(10, 31, sysdef.SiLifetimeBuyoutEventGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LifetimeBuyoutEventGiftSys).c2sRecFreeAwards
	})
	net.RegisterSysProtoV2(10, 32, sysdef.SiLifetimeBuyoutEventGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LifetimeBuyoutEventGiftSys).c2sRecDailyAwards
	})
	engine.RegChargeEvent(chargedef.LifetimeBuyoutEventGift, checkLifetimeBuyoutEventGift, chargeLifetimeBuyoutEventGift)
	event.RegActorEvent(custom_id.AeNewDay, handleLifetimeBuyoutEventGiftNewDay)
}
