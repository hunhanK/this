/**
 * @Author:
 * @Date:
 * @Desc: SiDemonTreasure 魔王秘宝
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	DemonKingShopTypeByType1 = 1 // 魔王商店
	DemonKingShopTypeByType2 = 2 // 秘宝商店
)

type DemonKingShopSys struct {
	Base
}

func (s *DemonKingShopSys) s2cInfo() {
	s.SendProto3(10, 50, &pb3.S2C_10_50{
		Data: s.getData(),
	})
}

func (s *DemonKingShopSys) getData() *pb3.DemonKingShopData {
	data := s.GetBinaryData().DemonKingShopData
	if data == nil {
		s.GetBinaryData().DemonKingShopData = &pb3.DemonKingShopData{}
		data = s.GetBinaryData().DemonKingShopData
	}
	if data.ShopData == nil {
		data.ShopData = make(map[uint32]*pb3.DemonKingShopEntry)
	}
	return data
}

func (s *DemonKingShopSys) GetShopData(tye uint32) (*pb3.DemonKingShopEntry, error) {
	switch tye {
	case DemonKingShopTypeByType1, DemonKingShopTypeByType2:
	default:
		return nil, neterror.ParamsInvalidError("%d not found", tye)
	}
	data := s.getData()
	entry := data.ShopData[tye]
	if entry == nil {
		entry = &pb3.DemonKingShopEntry{}
		data.ShopData[tye] = entry
	}
	if entry.FixItemList == nil {
		entry.FixItemList = &pb3.DemonKingShopItemList{}
	}
	if entry.RandomItemList == nil {
		entry.RandomItemList = &pb3.DemonKingShopItemList{}
	}
	return entry, nil
}

func (s *DemonKingShopSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DemonKingShopSys) OnLogin() {
	s.s2cInfo()
}

func (s *DemonKingShopSys) OnOpen() {
	_ = s.refreshShop(DemonKingShopTypeByType1)
	_ = s.refreshShop(DemonKingShopTypeByType2)
	s.initDailyCycle()
	s.resetFreeFreshCount()
	s.s2cInfo()
}

func (s *DemonKingShopSys) initDailyCycle() {
	data := s.getData()
	data.DailyCycleStartAt = time_util.NowSec()
}

func (s *DemonKingShopSys) refreshShop(tye uint32) error {
	config := jsondata.GetDemonKingShopConfig(tye)
	if config == nil {
		return neterror.ConfNotFoundError("%d cconfig not found", tye)
	}
	data, err := s.GetShopData(tye)
	if err != nil {
		return neterror.Wrap(err)
	}

	// 固定商品
	openServerDay := gshare.GetOpenServerDay()
	data.FixItemList.Items = nil
	for _, itemList := range config.FixItemList {
		if len(itemList.OpenSrvDayRange) == 2 {
			minOpenSrvDay := itemList.OpenSrvDayRange[0]
			maxOpenSrvDay := itemList.OpenSrvDayRange[1]
			if openServerDay < minOpenSrvDay || openServerDay > maxOpenSrvDay {
				continue
			}
		}
		data.FixItemList.Items = append(data.FixItemList.Items, &pb3.DemonKingShopItem{
			Idx: itemList.Idx,
		})
	}

	// 随机商品
	data.RandomItemList.Items = nil
	var randomList []*jsondata.DemonKingShopItem
	for _, item := range config.ItemList {
		if len(item.OpenSrvDayRange) == 2 {
			minOpenSrvDay := item.OpenSrvDayRange[0]
			maxOpenSrvDay := item.OpenSrvDayRange[1]
			if openServerDay < minOpenSrvDay || openServerDay > maxOpenSrvDay {
				continue
			}
		}
		randomList = append(randomList, item)
	}
	var randomSize uint32 = 5
	if config.ItemListCount > 0 {
		randomSize = config.ItemListCount
	}
	var skipSet = make(map[uint32]struct{})
	var randomPool = new(random.Pool)
	for i := uint32(0); i < randomSize; i++ {
		for _, item := range randomList {
			if _, ok := skipSet[item.Idx]; ok {
				continue
			}
			randomPool.AddItem(item, item.Rate)
		}
		if randomPool.Size() == 0 {
			break
		}
		shopItem := randomPool.RandomOne().(*jsondata.DemonKingShopItem)
		data.RandomItemList.Items = append(data.RandomItemList.Items, &pb3.DemonKingShopItem{
			Idx: shopItem.Idx,
		})
		randomPool.Clear()
		skipSet[shopItem.Idx] = struct{}{}
	}
	return nil
}

func (s *DemonKingShopSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_10_51
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	t := req.Type
	config := jsondata.GetDemonKingShopConfig(t)
	if config == nil {
		return neterror.ConfNotFoundError("%d cconfig not found", t)
	}
	data, err := s.GetShopData(t)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	var itemList = data.RandomItemList
	var itemListConf = config.ItemList
	if req.IsFix {
		itemList = data.FixItemList
		itemListConf = config.FixItemList
	}

	var sItem *pb3.DemonKingShopItem
	for _, shopItem := range itemList.Items {
		if shopItem.Idx != req.Idx {
			continue
		}
		sItem = shopItem
		break
	}
	if sItem == nil {
		return neterror.ParamsInvalidError("idx %d not found item", req.Idx)
	}
	var sItemConf *jsondata.DemonKingShopItem
	for _, shopItem := range itemListConf {
		if shopItem.Idx != req.Idx {
			continue
		}
		sItemConf = shopItem
		break
	}
	if sItemConf == nil {
		return neterror.ParamsInvalidError("idx %d not found config", req.Idx)
	}

	if req.Count == 0 {
		return neterror.ParamsInvalidError("count not zero")
	}

	if sItem.Count >= sItemConf.Count || sItem.Count+req.Count > sItemConf.Count {
		return neterror.ParamsInvalidError("idx %d count %d %d", sItem.Idx, sItem.Count, sItemConf.Count)
	}

	owner := s.GetOwner()
	consumeVec := jsondata.ConsumeMulti(sItemConf.Consume, req.Count)
	awardVec := jsondata.StdRewardMulti(sItemConf.Awards, int64(req.Count))
	if consumeVec == nil || !owner.ConsumeByConf(consumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogDemonKingShopBuy}) {
		return neterror.ConsumeFailedError("consume not enough")
	}
	sItem.Count += req.Count
	engine.GiveRewards(owner, awardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDemonKingShopBuy})
	s.SendProto3(10, 51, &pb3.S2C_10_51{
		Idx:   sItem.Idx,
		Type:  t,
		IsFix: req.IsFix,
		Item:  sItem,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDemonKingShopBuy, &pb3.LogPlayerCounter{
		NumArgs: uint64(t),
		StrArgs: fmt.Sprintf("%v_%d_%d", req.IsFix, sItem.Idx, sItem.Count),
	})
	return nil
}

func (s *DemonKingShopSys) c2sDailyAwards(_ *base.Message) error {
	data := s.getData()
	config := jsondata.GetDemonKingShopDailyConfig()
	if config == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	cycleStartAt := data.DailyCycleStartAt
	dailyAwardsRecFlag := data.DailyAwardsRecFlag
	nowSec := time_util.NowSec()
	days := time_util.TimestampSubDays(cycleStartAt, nowSec) + 1
	var canRecFlag uint32
	for i := uint32(1); i <= days; i++ {
		if utils.IsSetBit(dailyAwardsRecFlag, i) {
			continue
		}
		canRecFlag = utils.SetBit(canRecFlag, i)
	}
	if canRecFlag == 0 {
		return neterror.ParamsInvalidError("no daily awards can rec")
	}
	var dailyAwardList jsondata.StdRewardVec
	for _, dailyAward := range config.DailyAwards {
		if dailyAward.Day > days {
			break
		}
		if !utils.IsSetBit(canRecFlag, dailyAward.Day) {
			continue
		}
		dailyAwardList = append(dailyAwardList, dailyAward.Awards...)
		dailyAwardsRecFlag = utils.SetBit(dailyAwardsRecFlag, dailyAward.Day)
	}
	if dailyAwardsRecFlag == data.DailyAwardsRecFlag {
		return neterror.ParamsInvalidError("no daily awards can rec")
	}
	data.DailyAwardsRecFlag = dailyAwardsRecFlag
	owner := s.GetOwner()
	dailyAwardList = jsondata.MergeStdReward(dailyAwardList)
	engine.GiveRewards(owner, dailyAwardList, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDemonKingShopDailyAwards,
	})
	s.SendProto3(10, 52, &pb3.S2C_10_52{
		DailyAwardsRecFlag: dailyAwardsRecFlag,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDemonKingShopDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(dailyAwardsRecFlag),
	})
	return nil
}

func (s *DemonKingShopSys) c2sDailyProcessAwards(_ *base.Message) error {
	data := s.getData()
	config := jsondata.GetDemonKingShopDailyConfig()
	if config == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	cycleStartAt := data.DailyCycleStartAt
	nowSec := time_util.NowSec()
	days := time_util.TimestampSubDays(cycleStartAt, nowSec) + 1
	var totalAwards jsondata.StdRewardVec
	for _, cycleProcessAward := range config.CycleProcessAwards {
		if cycleProcessAward.Process > days {
			continue
		}
		if pie.Uint32s(data.DailyRecProcess).Contains(cycleProcessAward.Process) {
			continue
		}
		data.DailyRecProcess = append(data.DailyRecProcess, cycleProcessAward.Process)
		totalAwards = append(totalAwards, cycleProcessAward.Awards...)
	}
	if len(totalAwards) == 0 {
		return neterror.ParamsInvalidError("no daily process awards can rec")
	}
	owner := s.GetOwner()
	totalAwards = jsondata.MergeStdReward(totalAwards)
	engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDemonKingShopDailyProcessAwards,
	})
	s.SendProto3(10, 53, &pb3.S2C_10_53{
		DailyRecProcess: data.DailyRecProcess,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDemonKingShopDailyProcessAwards, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%v", data.DailyRecProcess),
	})
	return nil
}

func (s *DemonKingShopSys) c2sRefresh(msg *base.Message) error {
	var req pb3.C2S_10_54
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	t := req.Type
	config := jsondata.GetDemonKingShopConfig(t)
	if config == nil {
		return neterror.ConfNotFoundError("%d cconfig not found", t)
	}

	data, err := s.GetShopData(t)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	var freeFresh bool
	if data.FreeFreshCount > 0 {
		freeFresh = true
		data.FreeFreshCount -= 1
	}

	owner := s.GetOwner()
	if !freeFresh {
		if config.CommodityLimit != 0 && config.CommodityLimit <= data.PaidFreshCount {
			return neterror.ParamsInvalidError("refresh count limit %d %d", config.CommodityLimit, data.PaidFreshCount)
		}
		var rConsume *jsondata.DemonKingShopRefreshConsume
		for _, refreshConsume := range config.RefreshConsume {
			if data.PaidFreshCount > refreshConsume.Times {
				continue
			}
			rConsume = refreshConsume
			break
		}
		if rConsume == nil || !owner.ConsumeByConf(rConsume.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDemonKingShopRefresh}) {
			return neterror.ConsumeFailedError("consume not enough")
		}
		data.PaidFreshCount += 1
	}
	err = s.refreshShop(t)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	s.SendProto3(10, 54, &pb3.S2C_10_54{
		Type:  t,
		Entry: data,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDemonKingShopRefresh, &pb3.LogPlayerCounter{
		NumArgs: uint64(t),
		StrArgs: fmt.Sprintf("%v_%d_%d", freeFresh, data.FreeFreshCount, data.PaidFreshCount),
	})
	return nil
}

func (s *DemonKingShopSys) resetFreeFreshCount() {
	config1 := jsondata.GetDemonKingShopConfig(DemonKingShopTypeByType1)
	data1, _ := s.GetShopData(DemonKingShopTypeByType1)
	if data1 != nil && config1 != nil {
		data1.FreeFreshCount = config1.FreeCount
	}
	config2 := jsondata.GetDemonKingShopConfig(DemonKingShopTypeByType2)
	data2, _ := s.GetShopData(DemonKingShopTypeByType2)
	if data2 != nil && config2 != nil {
		data2.FreeFreshCount = config2.FreeCount
	}
}

func (s *DemonKingShopSys) onNewDay() {
	config := jsondata.GetDemonKingShopDailyConfig()
	if config == nil {
		return
	}
	data := s.getData()
	cycleStartAt := data.DailyCycleStartAt
	nowSec := time_util.NowSec()
	days := time_util.TimestampSubDays(cycleStartAt, nowSec) + 1
	if days > config.Cycle {
		data.DailyCycleStartAt = nowSec
		data.DailyAwardsRecFlag = 0
		data.DailyRecProcess = nil
	}
	_ = s.refreshShop(DemonKingShopTypeByType1)
	_ = s.refreshShop(DemonKingShopTypeByType2)
	s.resetFreeFreshCount()
	s.s2cInfo()
}

func getDemonKingShopSys(player iface.IPlayer) *DemonKingShopSys {
	obj := player.GetSysObj(sysdef.SiDemonTreasure)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*DemonKingShopSys)
	if !ok {
		return nil
	}
	return sys
}

func init() {
	RegisterSysClass(sysdef.SiDemonTreasure, func() iface.ISystem {
		return &DemonKingShopSys{}
	})
	net.RegisterSysProtoV2(10, 51, sysdef.SiDemonTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DemonKingShopSys).c2sBuy
	})
	net.RegisterSysProtoV2(10, 52, sysdef.SiDemonTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DemonKingShopSys).c2sDailyAwards
	})
	net.RegisterSysProtoV2(10, 53, sysdef.SiDemonTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DemonKingShopSys).c2sDailyProcessAwards
	})
	net.RegisterSysProtoV2(10, 54, sysdef.SiDemonTreasure, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DemonKingShopSys).c2sRefresh
	})
	event.RegActorEventL(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys := getDemonKingShopSys(player)
		if sys == nil {
			return
		}
		sys.onNewDay()
	})
}
