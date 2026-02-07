/**
 * @Author: beiming
 * @Desc: 赞助豪礼
 * @Date: 2023/12/20
 */
package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SponsorGift struct {
	Base
}

func init() {
	RegisterSysClass(sysdef.SiSponsorGift, newSponsorSystem)
	engine.RegAttrCalcFn(attrdef.SaSponsorGift, calcSponsorGiftSysAttr)

	event.RegActorEvent(custom_id.AeCharge, checkActiveSponsor)

	net.RegisterSysProtoV2(127, 11, sysdef.SiSponsorGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SponsorGift).c2sReceive
	})

	// 注册特权计算器
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		s, ok := player.GetSysObj(sysdef.SiSponsorGift).(*SponsorGift)
		if !ok || !s.IsOpen() {
			return
		}

		if len(conf.Sponsor) == 0 {
			return
		}

		gifts := s.getData()
		for k, v := range conf.Sponsor {
			id := k + 1 // 对应赞助豪礼配表 ref

			gift, ok := gifts[uint32(id)]
			if !ok {
				continue
			}

			if v > 0 && gift.State == SponsorGiftStateReceived {
				total += int64(v)
			}
		}

		return
	})

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		s, ok := player.GetSysObj(sysdef.SiSponsorGift).(*SponsorGift)
		if !ok || !s.IsOpen() {
			return
		}

		if len(conf.GodBeastUnlockBatPosX) < 2 {
			return
		}
		gifts := s.getData()

		id := conf.GodBeastUnlockBatPosX[0]
		gift, ok := gifts[id]
		if !ok {
			return
		}
		if gift.State != SponsorGiftStateReceived {
			return
		}
		var flag uint64
		for i := 1; i < len(conf.GodBeastUnlockBatPosX); i++ {
			pos := conf.GodBeastUnlockBatPosX[i]
			flag = utils.SetBit64(flag, pos)
		}
		total = int64(flag)
		return
	})
}

func (s *SponsorGift) s2cInfo() {
	data := s.getData()

	gifts := make([]*pb3.SponsorGift, 0, len(data))
	for _, v := range data {
		gifts = append(gifts, v)
	}

	s.SendProto3(127, 10, &pb3.S2C_127_10{
		Gifts: gifts,
	})
}

func (s *SponsorGift) getData() map[uint32]*pb3.SponsorGift {
	if s.GetBinaryData().SponsorGifts == nil {
		s.GetBinaryData().SponsorGifts = make(map[uint32]*pb3.SponsorGift, len(jsondata.GetSponsorGiftConfMap()))
	}
	return s.GetBinaryData().SponsorGifts
}

func (s *SponsorGift) IsBuyGift(cfgId uint32) bool {
	data := s.getData()
	gift, ok := data[cfgId]
	if !ok {
		return false
	}
	if gift.State != SponsorGiftStateCanReceive && gift.State != SponsorGiftStateReceived {
		return false
	}
	return true
}

// c2sReceive 领取赞助豪礼
func (s *SponsorGift) c2sReceive(msg *base.Message) error {
	var req pb3.C2S_127_11
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("sponsorGift c2sReceive unpack msg, err: %w", err)
	}

	gifts := s.getData()
	var gift *pb3.SponsorGift
	for _, g := range gifts {
		if g.ChargeId == req.ChargeId {
			gift = g
			break
		}
	}

	if gift == nil || gift.State != SponsorGiftStateCanReceive {
		return neterror.ParamsInvalidError("赞助豪礼不可领取, id: %d", req.ChargeId)
	}

	// 领取奖励
	cfg := jsondata.GetSponsorGiftConf(req.ChargeId)
	if cfg == nil {
		return neterror.ConfNotFoundError("赞助豪礼配置不存在, id: %d", req.ChargeId)
	}
	if !engine.GiveRewards(s.GetOwner(), cfg.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSponsorGiftReward}) {
		return neterror.ParamsInvalidError("赞助豪礼奖励发放失败, id: %d", req.ChargeId)
	}

	// 更新属性
	s.ResetSysAttr(attrdef.SaSponsorGift)

	// 更新状态
	gift.State = SponsorGiftStateReceived
	s.GetOwner().TriggerEvent(custom_id.AeReceiveSponsorGift, cfg.Id, cfg.ChargeId)
	s.GetOwner().TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	s.SendProto3(127, 11, &pb3.S2C_127_11{Gift: gift})

	return nil
}

func (s *SponsorGift) OnOpen() {
	s.s2cInfo()
}

func (s *SponsorGift) OnLogin() {
	s.s2cInfo()
}

func (s *SponsorGift) OnReconnect() {
	s.s2cInfo()
}

func calcSponsorGiftSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	gifts := player.GetBinaryData().GetSponsorGifts()

	var attrs []*jsondata.Attr
	for _, gift := range gifts {
		if gift.State != SponsorGiftStateReceived {
			cfg := jsondata.GetSponsorGiftConf(gift.ChargeId)

			attrs = append(attrs, cfg.Attrs...)
		}
	}

	engine.CheckAddAttrsToCalc(player, calc, attrs)
}

func checkActiveSponsor(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		actor.LogError("player %d charge params get err,args %v", actor.GetId(), args)
		return
	}
	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok || chargeEvent.ChargeId == 0 {
		return
	}
	gifts := actor.GetBinaryData().GetSponsorGifts()
	var gift *pb3.SponsorGift
	for _, v := range gifts {
		if v.ChargeId == chargeEvent.ChargeId {
			gift = v
			break
		}
	}

	if gift == nil {
		var err error
		gift, err = addGift(actor, chargeEvent.ChargeId)
		if err != nil {
			return
		}
	}

	if gift.State != SponsorGiftStateCanBuy {
		return
	}

	gift.State = SponsorGiftStateCanReceive

	actor.SendProto3(127, 11, &pb3.S2C_127_11{Gift: gift})
	actor.TriggerQuestEventRange(custom_id.QttFirstOrBuyXSponsorGift)
	logworker.LogPlayerBehavior(actor, pb3.LogId_LogSponsorGiftReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(gift.ChargeId),
	})

	gConf := jsondata.GetSponsorGiftConf(gift.ChargeId)
	rewardStr := engine.StdRewardToBroadcast(actor, gConf.Rewards)
	engine.BroadcastTipMsgById(tipmsgid.SponsorGiftTip, actor.GetId(), actor.GetName(), rewardStr)
}
func addGift(actor iface.IPlayer, chargeId uint32) (*pb3.SponsorGift, error) {
	cfg := jsondata.GetSponsorGiftConf(chargeId)
	if cfg == nil {
		return nil, neterror.ConfNotFoundError("赞助豪礼配置不存在, id: %d", chargeId)
	}

	gift := &pb3.SponsorGift{
		Ref:      cfg.Id,
		ChargeId: cfg.ChargeId,
		State:    SponsorGiftStateCanBuy,
	}

	if actor.GetBinaryData().SponsorGifts == nil {
		actor.GetBinaryData().SponsorGifts = make(map[uint32]*pb3.SponsorGift)
	}
	actor.GetBinaryData().SponsorGifts[cfg.Id] = gift

	return gift, nil
}

func newSponsorSystem() iface.ISystem {
	return &SponsorGift{}
}

const (
	SponsorGiftStateCanBuy     = 0
	SponsorGiftStateCanReceive = 1
	SponsorGiftStateReceived   = 2
)
