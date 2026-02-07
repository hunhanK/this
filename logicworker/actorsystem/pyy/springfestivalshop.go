/**
 * @Author: LvYuMeng
 * @Date: 2025/1/15
 * @Desc: 春节商店
**/

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
)

type SpringFestivalShopSys struct {
	PlayerYYBase
}

func (s *SpringFestivalShopSys) Login() {
	s.s2cInfo()
}

func (s *SpringFestivalShopSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SpringFestivalShopSys) OnOpen() {
	s.s2cInfo()
}

func (s *SpringFestivalShopSys) OnEnd() {
	s.clearYYMoney()
}

func (s *SpringFestivalShopSys) getData() *pb3.PYY_SpringFestivalShop {
	state := s.GetYYData()
	if nil == state.SpringFestivalShop {
		state.SpringFestivalShop = make(map[uint32]*pb3.PYY_SpringFestivalShop)
	}
	if state.SpringFestivalShop[s.Id] == nil {
		state.SpringFestivalShop[s.Id] = &pb3.PYY_SpringFestivalShop{}
	}
	if nil == state.SpringFestivalShop[s.Id].BuyData {
		state.SpringFestivalShop[s.Id].BuyData = make(map[uint32]uint32)
	}
	return state.SpringFestivalShop[s.Id]
}

func (s *SpringFestivalShopSys) s2cInfo() {
	s.SendProto3(75, 20, &pb3.S2C_75_20{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *SpringFestivalShopSys) ResetData() {
	state := s.GetYYData()
	if nil == state.SpringFestivalShop {
		return
	}
	delete(state.SpringFestivalShop, s.GetId())
}

const (
	SFShopBuyLimitPersist = 1
	SFShopBuyLimitDaily   = 2
)

func (s *SpringFestivalShopSys) NewDay() {
	conf := jsondata.GetPYYSpringFestivalShopConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.getData()
	for _, v := range conf.Goods {
		if v.LimitType == SFShopBuyLimitDaily {
			delete(data.BuyData, v.Id)
		}
	}

	s.s2cInfo()
}

func (s *SpringFestivalShopSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_75_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYSpringFestivalShopConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	goodsConf, ok := conf.Goods[req.Id]
	if !ok {
		return neterror.ConfNotFoundError("goods conf %d is nil", req.Id)
	}

	data := s.getData()
	if data.BuyData[req.Id]+req.Count > goodsConf.BuyLimit {
		return neterror.ParamsInvalidError("buy limit")
	}

	if !s.GetPlayer().DeductMoney(conf.MoneyType, int64(goodsConf.Price*req.Count), common.ConsumeParams{
		LogId: pb3.LogId_LogSpringFestivalShopBuy,
	}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.BuyData[req.Id] += req.Count

	s.SendProto3(75, 21, &pb3.S2C_75_21{
		ActiveId: s.Id,
		Id:       req.Id,
		Count:    data.BuyData[req.Id],
	})

	engine.GiveRewards(s.GetPlayer(), jsondata.StdRewardMulti(goodsConf.Rewards, int64(req.Count)), common.EngineGiveRewardParam{LogId: pb3.LogId_LogSpringFestivalShopBuy})

	return nil
}

func (s *SpringFestivalShopSys) clearYYMoney() {
	conf := jsondata.GetPYYSpringFestivalShopConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	count := s.GetPlayer().GetMoneyCount(conf.MoneyType)
	s.GetPlayer().DeductMoney(conf.MoneyType, count, common.ConsumeParams{
		LogId: pb3.LogId_LogSpringFestivalShopMoneyClear,
	})
}

func (s *SpringFestivalShopSys) CanAddYYMoney(mt uint32) bool {
	conf := jsondata.GetPYYSpringFestivalShopConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}

	return conf.MoneyType == mt
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSpringFestivalShop, func() iface.IPlayerYY {
		return &SpringFestivalShopSys{}
	})

	net.RegisterYYSysProtoV2(75, 21, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SpringFestivalShopSys).c2sBuy
	})
}
