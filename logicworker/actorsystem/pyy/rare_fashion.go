/**
 * @Author: lzp
 * @Date: 2023/11/14
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"

	"golang.org/x/exp/slices"
)

type PlayerYYRareFashion struct {
	PlayerYYBase
}

func (s *PlayerYYRareFashion) Login() {
	if !s.IsOpen() {
		return
	}
	s.sendInfo()
}

func (s *PlayerYYRareFashion) OnOpen() {
	s.sendInfo()
}

func (s *PlayerYYRareFashion) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.sendInfo()
}

func (s *PlayerYYRareFashion) GetData() *pb3.PYY_RareFashionData {
	if s.GetYYData().RareFashionMap == nil {
		s.GetYYData().RareFashionMap = make(map[uint32]*pb3.PYY_RareFashionData)
	}

	data, ok := s.GetYYData().RareFashionMap[s.GetId()]
	if !ok {
		data = &pb3.PYY_RareFashionData{
			BoughtFashions: make([]uint32, 0),
		}

		s.GetYYData().RareFashionMap[s.GetId()] = data
	}

	if data.BoughtFashions == nil {
		data.BoughtFashions = make([]uint32, 0)
	}
	return data
}
func (s *PlayerYYRareFashion) ResetData() {
	if s.GetYYData().RareFashionMap == nil {
		return
	}
	delete(s.GetYYData().RareFashionMap, s.Id)
}

func (s *PlayerYYRareFashion) c2sBuyRareFashion(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity %v", s.GetId())
	}

	conf := jsondata.GetPYYRareFashionConf(s.ConfName, s.ConfIdx)
	if conf == nil || len(conf.Gifts) == 0 {
		return neterror.InternalError("get conf failed for actId %d", s.GetId())
	}

	var req pb3.C2S_164_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	giftConf := jsondata.GetPYYRareFashionGiftConf(req.FashionIdx, conf)

	if giftConf == nil {
		return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d is illegal", s.GetId(), s.GetConfIdx(), req.FashionIdx)
	}

	if giftConf.SponsorId > 0 {
		gifts := s.player.GetBinaryData().GetSponsorGifts()
		if gifts == nil {
			return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d buy limit", s.GetId(), s.GetConfIdx(), req.FashionIdx)
		}
		gift, ok := gifts[giftConf.SponsorId]
		if !ok || gift.State == actorsystem.SponsorGiftStateCanBuy {
			return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d buy limit", s.GetId(), s.GetConfIdx(), req.FashionIdx)
		}
	}

	if s.GetData() == nil {
		return neterror.InternalError("get data failed for actId %d confIdx %d", s.GetId(), s.GetConfIdx())
	}

	if slices.Contains(s.GetData().BoughtFashions, req.FashionIdx) {
		return neterror.ParamsInvalidError("actId %d confIdx %d giftIdx %d already boughted", s.GetId(), s.GetConfIdx(), req.FashionIdx)
	}

	if s.player.GetMoneyCount(giftConf.MoneyType) < int64(giftConf.Money) {
		return neterror.ParamsInvalidError("money is not enough for actId %d confIdx %d", s.GetId(), s.GetConfIdx())
	}

	s.GetData().BoughtFashions = append(s.GetData().BoughtFashions, req.FashionIdx)

	if !s.GetPlayer().DeductMoney(giftConf.MoneyType, int64(giftConf.Money), common.ConsumeParams{
		LogId:   pb3.LogId_LogPYYRareFashionGiftBuy,
		SubType: s.GetId(),
	}) {
		return neterror.ParamsInvalidError("money not enough")
	}

	engine.GiveRewards(s.GetPlayer(), giftConf.Reward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPYYRareFashionGiftReward,
	})

	s.SendProto3(164, 2, &pb3.S2C_164_2{
		ActiveId:   s.GetId(),
		FashionIdx: req.FashionIdx,
	})

	engine.BroadcastTipMsgById(giftConf.BroadcastId, s.player.GetName(), giftConf.Name)

	return nil
}

func (s *PlayerYYRareFashion) sendInfo() {
	s.SendProto3(164, 1, &pb3.S2C_164_1{
		ActiveId:       s.GetId(),
		BoughtFashions: s.GetData(),
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYRareFashion, func() iface.IPlayerYY {
		return &PlayerYYRareFashion{}
	})

	net.RegisterYYSysProtoV2(164, 2, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYRareFashion).c2sBuyRareFashion
	})
}
