/**
 * @Author: LvYuMeng
 * @Date: 2024/8/19
 * @Desc: 疯狂购物车
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"golang.org/x/exp/maps"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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

type CrazyShoppingCart struct {
	PlayerYYBase
}

func (s *CrazyShoppingCart) ResetData() {
	state := s.GetYYData()
	if nil == state.CrazyShoppingCart {
		return
	}
	delete(state.CrazyShoppingCart, s.Id)
}

func (s *CrazyShoppingCart) data() *pb3.PYY_CrazyShoppingCart {
	state := s.GetYYData()
	if nil == state.CrazyShoppingCart {
		state.CrazyShoppingCart = make(map[uint32]*pb3.PYY_CrazyShoppingCart)
	}
	if nil == state.CrazyShoppingCart[s.Id] {
		state.CrazyShoppingCart[s.Id] = &pb3.PYY_CrazyShoppingCart{}
	}
	return state.CrazyShoppingCart[s.Id]
}

func (s *CrazyShoppingCart) Login() {
	s.s2cInfo()
}

func (s *CrazyShoppingCart) OnReconnect() {
	s.s2cInfo()
}

func (s *CrazyShoppingCart) OnOpen() {
	s.reset()
	s.s2cInfo()
}

func (s *CrazyShoppingCart) OnEnd() {
	s.reset()
}

func (s *CrazyShoppingCart) reset() {
	state := s.GetYYData()
	delete(state.CrazyShoppingCart, s.Id)
}

func (s *CrazyShoppingCart) s2cInfo() {
	s.SendProto3(69, 180, &pb3.S2C_69_180{
		ActiveId: s.Id,
		Data:     s.data(),
	})
}

func (s *CrazyShoppingCart) c2sAdd(msg *base.Message) error {
	var req pb3.C2S_69_181
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	cartType := req.GetType()

	shoppingCart := s.getShoppingCart(cartType)
	if nil == shoppingCart {
		return neterror.ParamsInvalidError("no cartType %d", cartType)
	}

	if req.GetCount() == 0 {
		delete(shoppingCart.ShoppingCart, req.GetId())
		shoppingCart.SortIds = pie.Uint32s(shoppingCart.SortIds).Filter(func(u uint32) bool {
			return u != req.GetId()
		})
		s.SendProto3(69, 181, &pb3.S2C_69_181{
			ActiveId: s.GetId(),
			Type:     cartType,
			Id:       req.GetId(),
			Count:    shoppingCart.ShoppingCart[req.GetId()],
		})
		return nil
	}

	conf, ok := jsondata.GetYYCrazyShoppingCartConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("CrazyShoppingCart conf nil")
	}

	goodsConf := conf.GetGoodsByType(cartType, req.GetId())
	if nil == goodsConf {
		return neterror.ConfNotFoundError("CrazyShoppingCart goods conf %d nil", req.GetId())
	}

	if goodsConf.Count < req.GetCount()+shoppingCart.BuyCount[req.GetId()] {
		return neterror.ParamsInvalidError("buy count limit")
	}

	var totalCount uint32
	for id, count := range shoppingCart.ShoppingCart {
		if id == req.GetId() {
			totalCount += req.GetCount()
		} else {
			totalCount += count
		}
	}

	if totalCount > conf.GetCap(cartType) {
		return neterror.ParamsInvalidError("shopping cart count limit")
	}

	shoppingCart.ShoppingCart[req.GetId()] = req.GetCount()

	if !pie.Uint32s(shoppingCart.SortIds).Contains(req.GetId()) {
		shoppingCart.SortIds = append(shoppingCart.SortIds, req.GetId())
	}

	s.SendProto3(69, 181, &pb3.S2C_69_181{
		ActiveId: s.GetId(),
		Type:     cartType,
		Id:       req.GetId(),
		Count:    shoppingCart.ShoppingCart[req.GetId()],
	})

	return nil
}

func (s *CrazyShoppingCart) checkBuy(cartType uint32, buyInfo map[uint32]uint32) (bool, error) {
	shoppingCart := s.getShoppingCart(cartType)
	if nil == shoppingCart {
		return false, neterror.ParamsInvalidError("no cartType %d", cartType)
	}

	conf, ok := jsondata.GetYYCrazyShoppingCartConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false, neterror.ConfNotFoundError("CrazyShoppingCart conf nil")
	}

	if len(shoppingCart.ShoppingCart) == 0 {
		return false, nil
	}

	var delIds []uint32
	for id, count := range shoppingCart.ShoppingCart {
		goodsConf := conf.GetGoodsByType(cartType, id)
		if nil == goodsConf {
			delIds = append(delIds, id)
			continue
		}
		if goodsConf.Count < shoppingCart.BuyCount[id]+shoppingCart.ShoppingCart[id] {
			delIds = append(delIds, id)
		}
		if count == 0 {
			delIds = append(delIds, id)
		}
	}

	for _, v := range delIds {
		delete(shoppingCart.ShoppingCart, v)
	}
	shoppingCart.SortIds = pie.Uint32s(shoppingCart.SortIds).FilterNot(func(u uint32) bool {
		return pie.Uint32s(delIds).Contains(u)
	})

	if len(delIds) > 0 {
		return false, neterror.InternalError("data is old, conf is change")
	}

	if !maps.Equal(shoppingCart.ShoppingCart, buyInfo) {
		return false, neterror.ParamsInvalidError("cart info not equal server data")
	}

	return true, nil
}

func (s *CrazyShoppingCart) buy(cartType uint32, buyInfo map[uint32]uint32) (bool, error) {
	ok, err := s.checkBuy(cartType, buyInfo)
	if !ok {
		return false, err
	}

	shoppingCart := s.getShoppingCart(cartType)
	if nil == shoppingCart {
		return false, neterror.ParamsInvalidError("no cartType %d", cartType)
	}

	conf, _ := jsondata.GetYYCrazyShoppingCartConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return false, neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	var originalPrice, totalCount uint32
	var rewards jsondata.StdRewardVec
	for id, count := range shoppingCart.ShoppingCart {
		goodsConf := conf.GetGoodsByType(cartType, id)
		originalPrice += goodsConf.Price * count
		rewards = append(rewards, jsondata.StdRewardMulti(goodsConf.Rewards, int64(count))...)
		totalCount += count
	}

	trulyPrice := conf.GetTrulyPrice(cartType, originalPrice, totalCount)
	if trulyPrice == 0 {
		return false, neterror.InternalError("price calc err")
	}

	moneyType := conf.GetMoneyType(cartType)
	if moneyType == 0 {
		return false, neterror.ConfNotFoundError("money conf is nil")
	}

	if !s.GetPlayer().DeductMoney(moneyType, int64(trulyPrice), common.ConsumeParams{
		LogId: pb3.LogId_LogCrazyShoppingCartConsume,
	}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return false, nil
	}

	for id, count := range shoppingCart.ShoppingCart {
		shoppingCart.BuyCount[id] += count
	}
	shoppingCart.ShoppingCart = nil
	shoppingCart.SortIds = nil

	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogCrazyShoppingCartRewards})

	s.SendProto3(69, 182, &pb3.S2C_69_182{
		ActiveId:     s.GetId(),
		Type:         cartType,
		ShoppingCart: shoppingCart,
	})

	if trulyPrice < originalPrice {
		engine.BroadcastTipMsgById(tipmsgid.PYYcrazyShoppingTips, s.GetPlayer().GetId(), s.GetPlayer().GetName(), originalPrice-trulyPrice)
	}

	return true, nil
}

func (s *CrazyShoppingCart) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_69_182
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if ok, err := s.buy(req.GetType(), req.GetShoppingCartInfo()); !ok {
		s.s2cInfo()
		return err
	}

	return nil
}

func (s *CrazyShoppingCart) getShoppingCart(cartType uint32) *pb3.CrazyShoppingCart {
	if !s.isValidType(cartType) {
		return nil
	}
	data := s.data()
	if nil == data.ShoppingCart {
		data.ShoppingCart = make(map[uint32]*pb3.CrazyShoppingCart)
	}
	if nil == data.ShoppingCart[cartType] {
		data.ShoppingCart[cartType] = &pb3.CrazyShoppingCart{}
	}
	if nil == data.ShoppingCart[cartType].ShoppingCart {
		data.ShoppingCart[cartType].ShoppingCart = make(map[uint32]uint32)
	}
	if nil == data.ShoppingCart[cartType].BuyCount {
		data.ShoppingCart[cartType].BuyCount = make(map[uint32]uint32)
	}
	return data.ShoppingCart[cartType]
}

func (s *CrazyShoppingCart) isValidType(cartType uint32) bool {
	return cartType == custom_id.CrazyShoppingCartDiscount || cartType == custom_id.CrazyShoppingCartRebate
}

func (s *CrazyShoppingCart) NewDay() {
	if !s.IsOpen() {
		return
	}
	conf, ok := jsondata.GetYYCrazyShoppingCartConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	if disCountData := s.getShoppingCart(custom_id.CrazyShoppingCartDiscount); nil != disCountData {
		for id, v := range conf.DiscountGoods {
			if !v.DailyReset {
				continue
			}
			delete(disCountData.BuyCount, id)
		}
	}
	if rebateData := s.getShoppingCart(custom_id.CrazyShoppingCartRebate); nil != rebateData {
		for id, v := range conf.RebateGoods {
			if !v.DailyReset {
				continue
			}
			delete(rebateData.BuyCount, id)
		}
	}
	s.s2cInfo()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYCrazyShoppingCart, func() iface.IPlayerYY {
		return &CrazyShoppingCart{}
	})

	net.RegisterYYSysProtoV2(69, 181, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CrazyShoppingCart).c2sAdd
	})

	net.RegisterYYSysProtoV2(69, 182, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CrazyShoppingCart).c2sBuy
	})

}
