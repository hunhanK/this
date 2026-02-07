package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"

	"golang.org/x/exp/slices"
)

type PlayerYYOpenServerGift struct {
	PlayerYYBase
}

func (s *PlayerYYOpenServerGift) Login() {
	s.sendInfo()
}

func (s *PlayerYYOpenServerGift) OnOpen() {
	s.sendInfo()
}

func (s *PlayerYYOpenServerGift) OnReconnect() {
	s.sendInfo()
}

func (s *PlayerYYOpenServerGift) OnEnd() {

}

func (s *PlayerYYOpenServerGift) GetData() *pb3.PYY_OpenServerGiftSuit {
	if s.GetYYData().OpenServerGiftDataMap == nil {
		s.GetYYData().OpenServerGiftDataMap = make(map[uint32]*pb3.PYY_OpenServerGiftSuit)
	}

	data, ok := s.GetYYData().OpenServerGiftDataMap[s.GetId()]
	if !ok {
		data = &pb3.PYY_OpenServerGiftSuit{
			BoughtedGifts: make([]uint32, 0),
		}

		s.GetYYData().OpenServerGiftDataMap[s.GetId()] = data
	}

	if data.BoughtedGifts == nil {
		data.BoughtedGifts = make([]uint32, 0)
	}
	return data
}
func (s *PlayerYYOpenServerGift) ResetData() {
	if s.GetYYData().OpenServerGiftDataMap == nil {
		return
	}
	delete(s.GetYYData().OpenServerGiftDataMap, s.Id)
}

func (s *PlayerYYOpenServerGift) sendInfo() {
	s.SendProto3(136, 1, &pb3.S2C_136_1{
		ActiveId:      s.GetId(),
		BoughtedGifts: s.GetData(),
	})
}

func c2sBuyOpenServerGift(sys iface.IPlayerYY) func(msg *base.Message) error {
	return sys.(*PlayerYYOpenServerGift).c2sBuy
}

func (s *PlayerYYOpenServerGift) GetConf() *jsondata.PYYOpenServerGiftConf {
	return jsondata.GetPYYOpenServerGiftConf(s.ConfName, s.GetConfIdx())
}

func (s *PlayerYYOpenServerGift) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_136_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := s.GetConf()
	if conf == nil || len(conf.Gifts) == 0 {
		return neterror.InternalError("get conf failed for actId %d", s.GetId())
	}

	if req.GiftIdx > uint32(len(conf.Gifts)) {
		return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d is illegal", s.GetId(), s.GetConfIdx(), req.GiftIdx)
	}

	if s.GetData() == nil {
		return neterror.InternalError("get data failed for actId %d confIdx %d", s.GetId(), s.GetConfIdx())
	}

	if slices.Contains(s.GetData().BoughtedGifts, req.GiftIdx) {
		return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d already boughted", s.GetId(), s.GetConfIdx(), req.GiftIdx)
	}

	giftConf := conf.Gifts[req.GiftIdx]

	if s.player.GetMoneyCount(giftConf.MoneyType) < int64(giftConf.Money) {
		return neterror.ParamsInvalidError("money is not enough for actId %d confIdx %d", s.GetId(), s.GetConfIdx())
	}

	s.GetData().BoughtedGifts = append(s.GetData().BoughtedGifts, req.GiftIdx)

	if !s.GetPlayer().DeductMoney(giftConf.MoneyType, int64(giftConf.Money), common.ConsumeParams{
		LogId:   pb3.LogId_LogPYYOpenServerGiftBuy,
		SubType: s.GetId(),
	}) {
		return neterror.ParamsInvalidError("money not enough")
	}

	rewardSuccess := engine.GiveRewards(s.GetPlayer(), giftConf.Reward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPYYOpenServerGiftReward,
	})

	if !rewardSuccess {
		return neterror.InternalError("rewards failed for actId %d confIdx %d giftIndex %d", s.GetId(), s.GetConfIdx(), req.GiftIdx)
	}

	player := s.GetPlayer()
	engine.BroadcastTipMsgById(tipmsgid.OpenServerGiftTip, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, giftConf.Reward))

	s.SendProto3(136, 2, &pb3.S2C_136_2{
		ActiveId: s.GetId(),
		GiftIdx:  req.GiftIdx,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiOpenServerGift, func() iface.IPlayerYY {
		return &PlayerYYOpenServerGift{}
	})

	net.RegisterYYSysProtoV2(136, 2, c2sBuyOpenServerGift)
}
