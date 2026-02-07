/**
 * @Author: lzp
 * @Date: 2025/5/22
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

const (
	MayDayShopGiftType1 = 1 // 直购
	MayDayShopGiftType2 = 2 // 特惠
	MayDayShopGiftType3 = 3 // 活动
)

type MayDayShopping struct {
	PlayerYYBase
}

func (s *MayDayShopping) OnReconnect() {
	s.s2cInfo()
}

func (s *MayDayShopping) Login() {
	s.s2cInfo()
}

func (s *MayDayShopping) OnOpen() {
	s.s2cInfo()
}

func (s *MayDayShopping) NewDay() {
	conf := jsondata.GetPYYMayDayShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	for id := range data.Gifts {
		gConf, ok := conf.Gifts[id]
		if !ok || !gConf.DayReset {
			continue
		}
		delete(data.Gifts, id)
	}
	s.s2cInfo()
}

func (s *MayDayShopping) GetData() *pb3.PYY_MayDayShop {
	state := s.GetYYData()
	if state.MayDayShop == nil {
		state.MayDayShop = make(map[uint32]*pb3.PYY_MayDayShop)
	}
	if state.MayDayShop[s.Id] == nil {
		state.MayDayShop[s.Id] = &pb3.PYY_MayDayShop{}
	}
	data := state.MayDayShop[s.Id]
	if data.Gifts == nil {
		data.Gifts = make(map[uint32]uint32)
	}
	return state.MayDayShop[s.Id]
}

func (s *MayDayShopping) ResetData() {
	yyData := s.GetYYData()
	if yyData.MayDayShop == nil {
		return
	}
	delete(yyData.MayDayShop, s.Id)
}

func (s *MayDayShopping) s2cInfo() {
	s.SendProto3(127, 162, &pb3.S2C_127_162{ActId: s.GetId(), Data: s.GetData()})
}

func (s *MayDayShopping) chargeCheck(chargeId uint32) bool {
	gConf := jsondata.GetPYYMayDayShopGiftConf(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	data := s.GetData()
	if data.Gifts[gConf.Id] >= gConf.Count {
		return false
	}

	return true
}

func (s *MayDayShopping) chargeBack(chargeId uint32) bool {
	gConf := jsondata.GetPYYMayDayShopGiftConf(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	data := s.GetData()
	data.Gifts[gConf.Id] += 1

	// 发奖励
	engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYMayDayShopGiftAwards})
	s.GetPlayer().SendShowRewardsPop(gConf.Rewards)
	s.SendProto3(127, 163, &pb3.S2C_127_163{ActId: s.Id, Id: gConf.Id, Count: data.Gifts[gConf.Id]})
	return true
}

func (s *MayDayShopping) c2sPurchase(msg *base.Message) error {
	var req pb3.C2S_127_163
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMayDayShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	gConf, ok := conf.Gifts[req.Id]
	if !ok {
		return neterror.ConfNotFoundError("giftId=%d conf not found", req.Id)
	}

	data := s.GetData()
	if data.Gifts[req.Id] >= gConf.Count {
		return neterror.ParamsInvalidError("giftId=%d count limit", req.Id)
	}

	if gConf.Type == MayDayShopGiftType1 {
		return neterror.ParamsInvalidError("giftId=%d type error", req.Id)
	}

	if gConf.Type == MayDayShopGiftType2 {
		if s.GetOpenDay() != gConf.OpenDay {
			return neterror.ParamsInvalidError("giftId=%d openDay limit", req.Id)
		}
	}

	if !s.GetPlayer().ConsumeByConf(gConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYMayDayShopGiftConsume}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Gifts[req.Id]++
	engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYMayDayShopGiftAwards})
	s.GetPlayer().SendShowRewardsPopByPYY(gConf.Rewards, s.Id)
	s.SendProto3(127, 163, &pb3.S2C_127_163{ActId: s.Id, Id: req.Id, Count: data.Gifts[req.Id]})
	return nil
}

func mayDayShopCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYMayDayShopping)
	for _, obj := range yyList {
		if s, ok := obj.(*MayDayShopping); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func mayDayShopBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYMayDayShopping)
	for _, obj := range yyList {
		if s, ok := obj.(*MayDayShopping); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMayDayShopping, func() iface.IPlayerYY {
		return &MayDayShopping{}
	})

	engine.RegChargeEvent(chargedef.MayDayShopGift, mayDayShopCheck, mayDayShopBack)

	net.RegisterYYSysProtoV2(127, 163, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayShopping).c2sPurchase
	})
}
