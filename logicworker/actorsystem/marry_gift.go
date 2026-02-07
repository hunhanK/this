/**
 * @Author: beiming
 * @Date: 2024/1/22
 * @Desc: 结婚共享礼包
**/
package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
	"golang.org/x/exp/slices"
)

func init() {
	RegisterSysClass(sysdef.SiMarryGift, func() iface.ISystem {
		return newMarryGiftSystem()
	})
	net.RegisterSysProtoV2(53, 60, sysdef.SiMarryGift, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryGift).c2sInfo
	})
	net.RegisterSysProtoV2(53, 61, sysdef.SiMarryGift, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryGift).c2sReceive
	})
	net.RegisterSysProtoV2(53, 62, sysdef.SiMarryGift, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryGift).c2sDailyCharge
	})
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		s, ok := actor.GetSysObj(sysdef.SiMarryGift).(*MarryGift)
		if !ok || !s.IsOpen() {
			return
		}
		s.onNewDay()
	})
}

type MarryGift struct {
	Base
}

func (s *MarryGift) OnOpen() {
	s.c2sInfo(nil)
}

func (s *MarryGift) OnAfterLogin() {
	lastLogout := s.GetMainData().GetLastLogoutTime()
	if !time_util.IsSameDay(lastLogout, time_util.NowSec()) { // 重置前一天礼包领取记录
		s.newDaySettlement()
	}
	s.c2sInfo(nil)
}

func (s *MarryGift) OnReconnect() {
	s.c2sInfo(nil)
}

func (s *MarryGift) c2sInfo(_ *base.Message) error {
	s.SendProto3(53, 60, &pb3.S2C_53_60{Gifts: s.getGifts()})
	return nil
}

// c2sReceive 领取结婚共享礼包
func (s *MarryGift) c2sReceive(msg *base.Message) error {
	var req pb3.C2S_53_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("C2S_53_61 UnPackPb3Msg err: %w", err)
	}

	cfg, ok := jsondata.GetMarryGiftConfigByID(req.Ref)
	if !ok || cfg == nil {
		return neterror.ConfNotFoundError("通过 ref 查找配置, ref: %d", req.Ref)
	}

	gifts := s.getGifts()
	if slices.Contains(gifts, req.Ref) {
		return neterror.ParamsInvalidError("重复领取礼包")
	}

	rechargeMoney := s.rechargeMoney(cfg.GiftType)
	if cfg.NeedCharge > rechargeMoney {
		return neterror.ParamsInvalidError("充值金额未满足")
	}

	if !engine.GiveRewards(s.GetOwner(), cfg.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMarryGiftReward}) {
		return neterror.ParamsInvalidError("结婚共享礼包奖励发放失败, id: %d", req.Ref)
	}

	s.GetBinaryData().MarryGifts = append(s.GetBinaryData().MarryGifts, req.Ref)

	return s.c2sInfo(nil)
}

func (s *MarryGift) rechargeMoney(giftType uint32) uint32 {
	selfChargeMoney := s.GetOwner().GetBinaryData().GetChargeInfo().GetDailyChargeMoney()

	partnerChargeMoney := s.partnerChargeMoney()

	if giftType == MarryGiftTypeSingle {
		return slices.Max([]uint32{selfChargeMoney, partnerChargeMoney})
	} else {
		return slices.Min([]uint32{selfChargeMoney, partnerChargeMoney})
	}
}

func (s *MarryGift) c2sDailyCharge(_ *base.Message) error {
	partnerChargeMoney := s.partnerChargeMoney()

	s.SendProto3(53, 62, &pb3.S2C_53_62{
		DailySelfCharge: s.GetOwner().GetBinaryData().GetChargeInfo().GetDailyChargeMoney(),
		DailyBothCharge: partnerChargeMoney,
	})
	return nil
}

func (s *MarryGift) partnerChargeMoney() uint32 {
	if player := manager.GetPlayerPtrById(s.marryId()); player != nil {
		return player.GetBinaryData().GetChargeInfo().GetDailyChargeMoney()
	}

	var partnerChargeMoney uint32
	if playerData, ok := manager.GetOfflineData(s.marryId(), gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		partnerChargeMoney = playerData.GetDailyChargeMoney()

		// 玩家离线超过一天，将充值金额重置为0
		if playerData.GetLastLogoutTime() > 0 && !time_util.IsSameDay(time_util.NowSec(), playerData.GetLastLogoutTime()) {
			partnerChargeMoney = 0
		}
	}
	return partnerChargeMoney
}

func (s *MarryGift) onNewDay() {
	s.newDaySettlement()
	s.c2sInfo(nil)
}

func (s *MarryGift) newDaySettlement() {
	gifts := s.GetBinaryData().MarryGifts

	var rewards []*jsondata.StdReward
	for _, cfg := range jsondata.GetMarryGiftConf() {
		if slices.Contains(gifts, cfg.Ref) {
			continue
		}

		rechargeMoney := s.rechargeMoney(cfg.GiftType)
		if cfg.NeedCharge > rechargeMoney {
			continue
		}

		rewards = jsondata.MergeStdReward(rewards, cfg.Rewards)

		gifts = append(gifts, cfg.Ref)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetOwner().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_MarryGift,
			Rewards: rewards,
		})
	}

	s.GetBinaryData().MarryGifts = make([]uint32, 0)
}

func (s *MarryGift) getGifts() []uint32 {
	if s.GetBinaryData().MarryGifts == nil {
		s.GetBinaryData().MarryGifts = make([]uint32, 0)
	}

	return s.GetBinaryData().MarryGifts
}

func (s *MarryGift) marryId() uint64 {
	marryData := s.GetBinaryData().MarryData
	if marryData == nil {
		return 0
	}

	if fd, ok := friendmgr.GetFriendCommonDataById(marryData.CommonId); ok {
		return utils.Ternary(fd.ActorId1 != s.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
	}

	return 0
}

func newMarryGiftSystem() iface.ISystem {
	return &MarryGift{}
}

const (
	MarryGiftTypeSingle = 1 // 共享礼包
	MarryGiftTypeBoth   = 2 // 双人礼包
)
