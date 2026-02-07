/**
 * @Author: LvYuMeng
 * @Date: 2024/7/29
 * @Desc: 定制累充
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type YYCustomizeSumChargeSys struct {
	PlayerYYBase
}

func (s *YYCustomizeSumChargeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.CustomizeSumCharge {
		return
	}
	delete(state.CustomizeSumCharge, s.Id)
}

func (s *YYCustomizeSumChargeSys) data() *pb3.PYY_CustomizeSumCharge {
	state := s.GetYYData()
	if nil == state.CustomizeSumCharge {
		state.CustomizeSumCharge = make(map[uint32]*pb3.PYY_CustomizeSumCharge)
	}
	if nil == state.CustomizeSumCharge[s.Id] {
		state.CustomizeSumCharge[s.Id] = &pb3.PYY_CustomizeSumCharge{}
	}
	if nil == state.CustomizeSumCharge[s.Id].Gift {
		state.CustomizeSumCharge[s.Id].Gift = make(map[uint32]*pb3.CustomizeSumChargeGift)
	}
	return state.CustomizeSumCharge[s.Id]
}

func (s *YYCustomizeSumChargeSys) getGiftDataById(id uint32) *pb3.CustomizeSumChargeGift {
	data := s.data()
	if _, ok := data.Gift[id]; !ok {
		data.Gift[id] = &pb3.CustomizeSumChargeGift{}
	}
	if nil == data.Gift[id].RefId {
		data.Gift[id].RefId = make(map[uint32]uint32)
	}
	return data.Gift[id]
}

func (s *YYCustomizeSumChargeSys) Login() {
	s.s2cInfo()
}

func (s *YYCustomizeSumChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *YYCustomizeSumChargeSys) OnEnd() {
	s.sendReward()
}

func (s *YYCustomizeSumChargeSys) sendReward() {
	conf := jsondata.GetYYCustomizeSumChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.data()
	var rewardsVec []jsondata.StdRewardVec
	for _, v := range conf.Gifts {
		if data.ChargeCent < v.ChargeCent {
			continue
		}
		g := s.getGiftDataById(v.Id)
		if g.IsRev {
			continue
		}
		if v.ChooseCount == 0 {
			rewardsVec = append(rewardsVec, v.FixedAwards)
			continue
		} else {
			if v.ChooseCount != uint32(len(g.RefId)) {
				continue
			}
			rewardsVec = append(rewardsVec, v.FixedAwards)
			for _, refId := range g.RefId {
				refConf, ok := v.Choose[refId]
				if !ok {
					continue
				}
				rewardsVec = append(rewardsVec, refConf.Rewards)
			}
		}
	}
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (s *YYCustomizeSumChargeSys) reset() {
	delete(s.GetYYData().CustomizeSumCharge, s.GetId())
}

func (s *YYCustomizeSumChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}

	chargeCent := chargeEvent.CashCent

	data := s.data()
	data.ChargeCent += chargeCent

	s.SendProto3(69, 143, &pb3.S2C_69_143{
		ActiveId:   s.GetId(),
		ChargeCent: data.GetChargeCent(),
	})
}

func (s *YYCustomizeSumChargeSys) s2cInfo() {
	s.SendProto3(69, 140, &pb3.S2C_69_140{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *YYCustomizeSumChargeSys) OnOpen() {
	s.reset()
	s.s2cInfo()
}

func (s *YYCustomizeSumChargeSys) c2sChoose(msg *base.Message) error {
	var req pb3.C2S_69_141
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYCustomizeSumChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("YYCustomizeSumChargeSys conf nil")
	}

	giftId := req.GetGiftId()
	refId := req.GetRefId()

	giftConf, ok := conf.Gifts[giftId]
	if !ok {
		return neterror.ConfNotFoundError("gift conf %d is nil", giftId)
	}

	refConf, ok := giftConf.Choose[refId]
	if !ok {
		return neterror.ConfNotFoundError("gift refId conf %d is nil", refId)
	}

	g := s.getGiftDataById(giftId)

	if g.IsRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	g.RefId[refConf.Type] = refId
	s.SendProto3(69, 141, &pb3.S2C_69_141{
		ActiveId: s.GetId(),
		GiftId:   giftId,
		RefId:    refId,
		Type:     refConf.Type,
	})

	return nil
}

func (s *YYCustomizeSumChargeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_69_142
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYCustomizeSumChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("YYCustomizeSumChargeSys conf nil")
	}

	giftId := req.GetGiftId()
	giftConf, ok := conf.Gifts[giftId]
	if !ok {
		return neterror.ConfNotFoundError("gift conf %d is nil", giftId)
	}

	data := s.data()
	g := s.getGiftDataById(giftId)

	if giftConf.ChooseCount != uint32(len(g.RefId)) {
		return neterror.ConfNotFoundError("gift choose conf %d is nil", g.RefId)
	}

	if g.IsRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	if data.ChargeCent < giftConf.ChargeCent {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	g.IsRev = true

	var rewardVec []jsondata.StdRewardVec
	rewardVec = append(rewardVec, giftConf.FixedAwards)

	for _, refId := range g.RefId {
		refConf, ok := giftConf.Choose[refId]
		if !ok {
			continue
		}
		rewardVec = append(rewardVec, refConf.Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardVec...)
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogCustomizeSumChargeAward,
	})

	s.SendProto3(69, 142, &pb3.S2C_69_142{
		ActiveId: s.GetId(),
		GiftId:   giftId,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYCustomizeSumCharge, func() iface.IPlayerYY {
		return &YYCustomizeSumChargeSys{}
	})

	net.RegisterYYSysProtoV2(69, 141, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*YYCustomizeSumChargeSys).c2sChoose
	})
	net.RegisterYYSysProtoV2(69, 142, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*YYCustomizeSumChargeSys).c2sRev
	})
}
