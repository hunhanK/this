/**
 * @Author: LvYuMeng
 * @Date: 2023/11/9
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FaBaoGiftSys struct {
	Base
}

func (s *FaBaoGiftSys) OnOpen() {
	s.checkNewLoop()
	s.s2cInfo()
}

func (s *FaBaoGiftSys) OnLogin() {
	s.checkNewLoop()
}

func (s *FaBaoGiftSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *FaBaoGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FaBaoGiftSys) checkNewLoop() {
	data := s.GetData()
	loop := data.GetLoop()
	if loop > 0 {
		loopConf := jsondata.GetFaBaoGiftLoopConf(loop)
		nextLoopConf := jsondata.GetFaBaoGiftLoopConf(loop + 1)
		if nil == loopConf || nil == nextLoopConf || gshare.GetOpenServerDay() < nextLoopConf.OpenDay {
			return
		}
		for id := range loopConf.Gift {
			if data.Gift[id] <= 0 {
				return
			}
		}
	}

	data.Loop = loop + 1
	s.SendProto3(48, 2, &pb3.S2C_48_2{Loop: data.Loop})
}

func (s *FaBaoGiftSys) GetData() *pb3.FaBaoGift {
	binary := s.GetBinaryData()
	if nil == binary.FaBaoGift {
		binary.FaBaoGift = &pb3.FaBaoGift{}
	}
	if nil == binary.FaBaoGift.Gift {
		binary.FaBaoGift.Gift = map[uint32]uint32{}
	}
	return binary.FaBaoGift
}

func (s *FaBaoGiftSys) s2cInfo() {
	s.SendProto3(48, 0, &pb3.S2C_48_0{Gift: s.GetData()})
}

func (s *FaBaoGiftSys) onNewDay() {
	s.checkNewLoop()
}

const (
	faBaoGift_WeekDicount         = 1
	faBaoGift_MonDicount          = 2
	faBaoGift_FlyingFairyDicount  = 3
	faBaoGift_SponsorGiftDiscount = 4
)

func (s *FaBaoGiftSys) c2sBuyGift(msg *base.Message) error {
	var req pb3.C2S_48_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	data := s.GetData()
	loopConf := jsondata.GetFaBaoGiftLoopConf(data.GetLoop())
	if nil == loopConf || nil == loopConf.Gift || nil == loopConf.Gift[req.Id] {
		return neterror.ConfNotFoundError("no fabao gift conf(%d)", req.Id)
	}
	giftConf := loopConf.Gift[req.Id]
	if data.Gift[req.Id] > 0 {
		s.owner.SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}
	if s.owner.GetVipLevel() < giftConf.Vip {
		s.owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}
	if s.GetBinaryData().ChargeInfo.TotalChargeMoney < giftConf.SumCharge {
		s.owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}
	canDiscount := false
	switch giftConf.DiscountType {
	case faBaoGift_WeekDicount:
		if pvcardSys, ok := s.owner.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys); ok && pvcardSys.IsOpen() {
			if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Week) {
				canDiscount = true
			}
		}
	case faBaoGift_MonDicount:
		if pvcardSys, ok := s.owner.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys); ok && pvcardSys.IsOpen() {
			if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
				canDiscount = true
			}
		}
	case faBaoGift_FlyingFairyDicount:
		yyList := s.owner.GetPYYObjList(yydefine.YYSiFlyingFairyOrderFunc)
		for _, obj := range yyList {
			if obj.GetClass() == yydefine.YYSiFlyingFairyOrderFunc && obj.IsOpen() {
				yyData := s.GetBinaryData().YyData.FlyingFairyOrderFuncDataMap
				if nil != yyData && nil != yyData[obj.GetId()] && yyData[obj.GetId()].UnLockChargeId > 0 {
					canDiscount = true
					break
				}
			}
		}
	case faBaoGift_SponsorGiftDiscount:
		if sponsorSys, ok := s.owner.GetSysObj(sysdef.SiSponsorGift).(*SponsorGift); ok && sponsorSys.IsOpen() {
			if sponsorSys.IsBuyGift(giftConf.DiscountExt) {
				canDiscount = true
			}
		}
	}
	consumes := giftConf.Price
	if canDiscount {
		consumes = giftConf.Discount
	}
	if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyFaBaoGift}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	data.Gift[req.Id]++
	s.owner.TriggerQuestEvent(custom_id.QttFaBaoGiftBuy, 0, 1)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogBuyFaBaoGift, &pb3.LogPlayerCounter{NumArgs: uint64(req.Id)})
	s.GetOwner().TriggerEvent(custom_id.AeFaBaoGiftBuyGift)
	s.ResetSysAttr(attrdef.SaFaBaoGift)
	s.SendProto3(48, 1, &pb3.S2C_48_1{Id: req.Id})
	engine.BroadcastTipMsgById(tipmsgid.FaBaoGiftTip, s.owner.GetId(), s.owner.GetName())
	s.checkNewLoop()
	return nil
}

func calcFaBaoGiftAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiFaBaoGift).(*FaBaoGiftSys)
	if nil == sys || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	conf := jsondata.FaBaoGiftConfMgr
	for _, loopConf := range conf {
		for id, giftConf := range loopConf.Gift {
			if data.Gift[id] > 0 {
				engine.CheckAddAttrsToCalc(player, calc, giftConf.Attrs)
			}
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiFaBaoGift, func() iface.ISystem {
		return &FaBaoGiftSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFaBaoGift, calcFaBaoGiftAttr)

	net.RegisterSysProtoV2(48, 1, sysdef.SiFaBaoGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaBaoGiftSys).c2sBuyGift
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiFaBaoGift).(*FaBaoGiftSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

}
