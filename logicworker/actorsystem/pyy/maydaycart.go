/**
 * @Author: lzp
 * @Date: 2025/5/21
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
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
	"math"
	"time"
)

const (
	HLib = 1 // 极品库
	MLib = 2 // 高级库
	LLib = 3 // 普通库
)

type MayDayCart struct {
	PlayerYYBase
	timer *time_util.Timer
}

func (s *MayDayCart) OnLogout() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *MayDayCart) OnAfterLogin() {
	data := s.GetData()
	if data.RefreshTimestamp > 0 {
		s.StartTimer()
	}
	s.s2cInfo()
}

func (s *MayDayCart) OnReconnect() {
	s.s2cInfo()
}

func (s *MayDayCart) OnOpen() {
	s.StartTimer()
	s.s2cInfo()
}

func (s *MayDayCart) GetData() *pb3.PYY_MayDayCart {
	state := s.GetYYData()
	if state.MayDayCart == nil {
		state.MayDayCart = make(map[uint32]*pb3.PYY_MayDayCart)
	}
	if state.MayDayCart[s.Id] == nil {
		state.MayDayCart[s.Id] = &pb3.PYY_MayDayCart{}
	}
	return state.MayDayCart[s.Id]
}

func (s *MayDayCart) ResetData() {
	state := s.GetYYData()
	if state.MayDayCart == nil {
		return
	}
	delete(state.MayDayCart, s.Id)
}

func (s *MayDayCart) NewDay() {
	data := s.GetData()
	data.GoodIds = data.GoodIds[:0]
	data.BuyGoodIds = data.BuyGoodIds[:0]
	data.RefreshCount = 0
	data.DiscountTimes = 0
}

func (s *MayDayCart) StartTimer() {
	nowSec := time_util.NowSec()
	conf := jsondata.GetPYYMayDayCartConf(s.ConfName, s.ConfIdx)
	if conf == nil || conf.RefreshSecond == 0 {
		return
	}

	data := s.GetData()
	if data.RefreshTimestamp < nowSec {
		s.refreshGoods()
		nextTime := nowSec + conf.RefreshSecond
		data.RefreshTimestamp = nextTime
		s.s2cInfo()
	}

	dur := data.RefreshTimestamp - nowSec
	s.timer = s.GetPlayer().SetTimeout(time.Duration(dur)*time.Second, func() {
		s.StartTimer()
	})
}

func (s *MayDayCart) s2cInfo() {
	s.SendProto3(127, 159, &pb3.S2C_127_159{ActId: s.Id, Data: s.GetData()})
}

func (s *MayDayCart) refreshGoods() {
	conf := jsondata.GetPYYMayDayCartConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	data.GoodIds = data.GoodIds[:0]
	data.BuyGoodIds = data.BuyGoodIds[:0]

	openSrvDay := gshare.GetOpenServerDay()

	var hPool, mPool, lPool random.Pool
	for i := range conf.GoodsShop {
		gConf := conf.GoodsShop[i]
		if openSrvDay < gConf.MinOpenDay || openSrvDay > gConf.MaxOpenDay {
			continue
		}

		switch gConf.Lib {
		case HLib:
			hPool.AddItem(gConf, gConf.Weight)
		case MLib:
			mPool.AddItem(gConf, gConf.Weight)
		case LLib:
			lPool.AddItem(gConf, gConf.Weight)
		}
	}

	// 极品库
	if len(conf.BestLibCounts) >= 4 {
		min, minWeight := conf.BestLibCounts[0], conf.BestLibCounts[1]
		max, maxWeight := conf.BestLibCounts[2], conf.BestLibCounts[3]

		var count uint32
		weight := random.IntervalUU(1, minWeight+maxWeight)
		if weight <= minWeight {
			count = min
		} else {
			count = max
		}

		rets := hPool.RandomMany(count)
		for _, ret := range rets {
			value := ret.(*jsondata.MayDayCartGood)
			data.GoodIds = append(data.GoodIds, value.Id)
		}
	}

	// 高级库
	if len(conf.HighLibCounts) >= 4 {
		min, minWeight := conf.HighLibCounts[0], conf.HighLibCounts[1]
		max, maxWeight := conf.HighLibCounts[2], conf.HighLibCounts[3]

		var count uint32
		weight := random.IntervalUU(1, minWeight+maxWeight)
		if weight <= minWeight {
			count = min
		} else {
			count = max
		}

		rets := mPool.RandomMany(count)
		for _, ret := range rets {
			value := ret.(*jsondata.MayDayCartGood)
			data.GoodIds = append(data.GoodIds, value.Id)
		}
	}

	// 普通库
	if conf.GoodsCount > uint32(len(data.GoodIds)) {
		count := conf.GoodsCount - uint32(len(data.GoodIds))
		rets := lPool.RandomMany(count)
		for _, ret := range rets {
			value := ret.(*jsondata.MayDayCartGood)
			data.GoodIds = append(data.GoodIds, value.Id)
		}
	}
}

func (s *MayDayCart) c2sShop(msg *base.Message) error {
	var req pb3.C2S_127_160
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMayDayCartConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	count := len(req.Ids)
	if len(req.Ids) <= 0 || uint32(count) > conf.OnceBuyCount {
		return neterror.ParamsInvalidError("cart empty or cart goods overload")
	}

	data := s.GetData()

	if count > int(conf.DiscountCount-data.DiscountTimes) {
		return neterror.ParamsInvalidError("discount times limit")
	}

	var moneyType, moneyCount uint32
	var rewards jsondata.StdRewardVec
	for _, id := range req.Ids {
		gConf := jsondata.GetPYYMayDayCartGoodConf(s.ConfName, s.ConfIdx, id)
		if gConf == nil {
			return neterror.ConfNotFoundError("id=%d good conf not exit", id)
		}
		if utils.SliceContainsUint32(data.BuyGoodIds, id) {
			return neterror.ParamsInvalidError("id=%d good has bought", id)
		}
		moneyType = gConf.MoneyType
		moneyCount += gConf.MoneyCount
		rewards = append(rewards, &jsondata.StdReward{
			Id:    gConf.ItemId,
			Count: int64(gConf.ItemCount),
		})
	}

	discount := conf.Discounts[utils.MinInt(len(conf.Discounts)-1, count-1)]
	money := math.Ceil(float64(moneyCount) * (float64(discount) / float64(10000)))
	// 消耗
	if !s.GetPlayer().DeductMoney(moneyType, int64(money), common.ConsumeParams{LogId: pb3.LogId_LogYYMayDayCartBuyConsume}) {
		s.GetPlayer().LogWarn("money not enough")
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 设置状态
	data.BuyGoodIds = append(data.BuyGoodIds, req.Ids...)
	data.DiscountTimes += uint32(count)

	// 发奖励
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYMayDayCartBuyAwards})

	s.GetPlayer().SendShowRewardsPopByPYY(rewards, s.Id)
	s.SendProto3(127, 160, &pb3.S2C_127_160{ActId: s.Id, Ids: req.Ids})

	return nil
}

func (s *MayDayCart) c2sRefresh(msg *base.Message) error {
	var req pb3.C2S_127_161
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMayDayCartConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	data := s.GetData()
	if data.RefreshCount >= conf.FreeCount {
		times := data.RefreshCount - conf.FreeCount + 1
		rConf := jsondata.GetPYYMayDayCartRefreshConsumeConf(s.ConfName, s.ConfIdx, times)
		if rConf == nil {
			return neterror.ConsumeFailedError("refresh consume conf is nil")
		}
		if !s.GetPlayer().ConsumeByConf(rConf.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogYYMayDayCartRefreshConsume}) {
			return neterror.ConsumeFailedError("消耗失败")
		}
	}

	data.RefreshCount++
	s.refreshGoods()
	s.s2cInfo()

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMayDayCart, func() iface.IPlayerYY {
		return &MayDayCart{}
	})

	net.RegisterYYSysProtoV2(127, 160, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayCart).c2sShop
	})
	net.RegisterYYSysProtoV2(127, 161, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayCart).c2sRefresh
	})
}
