/**
 * @Author: lzp
 * @Date: 2025/7/10
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
	"time"
)

type MysteryStore struct {
	PlayerYYBase
	timer *time_util.Timer
}

func (s *MysteryStore) OnAfterLogin() {
	s.StartTimer()
	s.s2cInfo()
}

func (s *MysteryStore) OnLogout() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *MysteryStore) OnReconnect() {
	s.s2cInfo()
}

func (s *MysteryStore) OnOpen() {
	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	s.refreshGoods()
	data := s.GetData()
	data.RemainFreeFreshCount = conf.FreeCount
	s.s2cInfo()
}

func (s *MysteryStore) GetData() *pb3.PYYMysteryStoreData {
	state := s.GetYYData()
	if state.MysteryStore == nil {
		state.MysteryStore = make(map[uint32]*pb3.PYYMysteryStoreData)
	}
	if state.MysteryStore[s.Id] == nil {
		state.MysteryStore[s.Id] = &pb3.PYYMysteryStoreData{}
	}
	data := state.MysteryStore[s.Id]
	if data.BuyCount == nil {
		data.BuyCount = map[uint32]uint32{}
	}
	return data
}

func (s *MysteryStore) ResetData() {
	state := s.GetYYData()
	if state.MysteryStore == nil {
		return
	}
	delete(state.MysteryStore, s.Id)
}

func (s *MysteryStore) OnEnd() {
	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	if conf.EndRecycle {
		score := s.GetPlayer().GetMoneyCount(conf.MoneyType)
		if score > 0 {
			if !s.GetPlayer().DeductMoney(conf.MoneyType, int64(score), common.ConsumeParams{LogId: pb3.LogId_LogPYYMysteryStoreEndRecycle}) {
				s.LogError("回收失败")
				return
			}
		}
	}
}

func (s *MysteryStore) CanAddYYMoney(mt uint32) bool {
	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return false
	}
	return conf.MoneyType == mt
}

func (s *MysteryStore) StartTimer() {
	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil || conf.AutoInterval == 0 {
		return
	}

	data := s.GetData()
	nowSec := time_util.NowSec()
	if data.NextRefreshTime == 0 {
		nextTime := nowSec + conf.AutoInterval
		data.NextRefreshTime = nextTime
	}

	if data.NextRefreshTime < nowSec {
		if data.RemainFreeFreshCount < conf.FreeCount {
			data.RemainFreeFreshCount += 1
		}
		nextTime := nowSec + conf.AutoInterval
		data.NextRefreshTime = nextTime
		s.s2cInfo()

		if data.RemainFreeFreshCount >= conf.FreeCount {
			return
		}
	}

	dur := data.NextRefreshTime - nowSec
	s.timer = s.GetPlayer().SetTimeout(time.Duration(dur)*time.Second, func() {
		s.StartTimer()
	})
}

func (s *MysteryStore) s2cInfo() {
	s.SendProto3(127, 175, &pb3.S2C_127_175{ActId: s.Id, Data: s.GetData()})
}

func (s *MysteryStore) refreshGoods() {
	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	data.ItemInfo = make(map[uint32]*pb3.PYYMysteryStoreItemInfo)

	if !data.IsInit { //初次刷新
		for _, idx := range conf.InitItem {
			data.ItemInfo[idx] = &pb3.PYYMysteryStoreItemInfo{
				Idx:   idx,
				IsBuy: false,
			}
		}
		data.IsInit = true
		return
	}

	openSrvDay := gshare.GetOpenServerDay()
	var items []*jsondata.PYYMysteryShopItem
	for _, item := range conf.ItemList {
		// 次数达到购买限制
		buyCount := data.BuyCount[item.Index]
		if item.MaxBuyCount > 0 && item.MaxBuyCount <= buyCount {
			continue
		}

		if item.Rate > 0 && (openSrvDay >= item.OpenDay && openSrvDay <= item.EndDay) {
			items = append(items, item)
		}
	}

	pool := new(random.Pool)
	for _, v := range items {
		pool.AddItem(v, v.Rate)
	}

	rets := pool.RandomMany(conf.CommodityLimit)
	for _, ret := range rets {
		value := ret.(*jsondata.PYYMysteryShopItem)
		data.ItemInfo[value.Index] = &pb3.PYYMysteryStoreItemInfo{
			Idx:   value.Index,
			IsBuy: false,
		}
	}
}

func (s *MysteryStore) c2sRefresh(msg *base.Message) error {
	var req pb3.C2S_127_176
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	data := s.GetData()
	if data.RemainFreeFreshCount <= 0 {
		consume := jsondata.GetPYYMysteryShopRefreshConsumeByTimes(s.ConfName, s.ConfIdx, data.PaidFreshCount)
		if consume == nil {
			return neterror.ConfNotFoundError("paidFreshCount:%d config not found", data.PaidFreshCount)
		}
		if !s.GetPlayer().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYRefreshMysteryStore}) {
			return neterror.ConsumeFailedError("consume error")
		}
		data.PaidFreshCount++
	} else {
		data.RemainFreeFreshCount--
		s.StartTimer()
	}

	s.refreshGoods()

	s.SendProto3(127, 176, &pb3.S2C_127_176{ActId: s.GetId(), IsSuccess: true})
	s.s2cInfo()
	return nil
}

func (s *MysteryStore) c2sShop(msg *base.Message) error {
	var req pb3.C2S_127_177
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMysteryShopConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	data := s.GetData()
	itemInfo := data.ItemInfo[req.Index]
	if itemInfo == nil || itemInfo.IsBuy {
		return neterror.ParamsInvalidError("index:%d cannot buy", req.Index)
	}

	indexConf := jsondata.GetPYYMysteryShopItemConfByIdx(s.ConfName, s.ConfIdx, itemInfo.Idx)
	if nil == indexConf {
		return neterror.InternalError("shop index:%d conf is nil", req.Index)
	}

	itemConf := jsondata.GetItemConfig(indexConf.Id)
	if itemConf == nil {
		return neterror.InternalError("item conf is nil")
	}

	var rewards []*jsondata.StdReward
	rewards = append(rewards, &jsondata.StdReward{
		Id:    indexConf.Id,
		Count: int64(indexConf.Count),
		Bind:  indexConf.Bind,
	})

	if !s.GetPlayer().DeductMoney(indexConf.MoneyType, int64(indexConf.CurrentPrice), common.ConsumeParams{LogId: pb3.LogId_LogPYYShoppingAtMysteryStore}) {
		return neterror.ParamsInvalidError("deductMoney failed moneyType %d price %d", indexConf.MoneyType, indexConf.CurrentPrice)
	}

	itemInfo.IsBuy = true
	data.BuyCount[indexConf.Index] += 1

	if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYShoppingAtMysteryStore}) {
		return neterror.InternalError("give rewards failed")
	}

	s.GetPlayer().SendShowRewardsPopByPYY(rewards, s.GetId())
	s.SendProto3(127, 177, &pb3.S2C_127_177{ActId: s.GetId(), Index: req.GetIndex()})
	s.s2cInfo()
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMysteryStore, func() iface.IPlayerYY {
		return &MysteryStore{}
	})

	net.RegisterYYSysProtoV2(127, 176, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MysteryStore).c2sRefresh
	})
	net.RegisterYYSysProtoV2(127, 177, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MysteryStore).c2sShop
	})

}
