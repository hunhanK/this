/**
 * @Author: LvYuMeng
 * @Date: 2024/10/11
 * @Desc:
**/

package auctionmgr

import (
	"github.com/gzjjyz/srvlib/utils"
	"golang.org/x/exp/maps"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/auction"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"time"
)

var (
	AuctionMgrInstance = NewAuctionMgr()
	bonusChecker       *time_util.TimeChecker
)

type AuctionMgr struct {
	detail map[uint64]*time_util.Timer
}

func NewAuctionMgr() *AuctionMgr {
	return &AuctionMgr{
		detail: map[uint64]*time_util.Timer{},
	}
}

func (mgr *AuctionMgr) getAuctionData() *pb3.SrvAuction {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.SrvAuction {
		globalVar.SrvAuction = &pb3.SrvAuction{}
	}
	if nil == globalVar.SrvAuction.Goods {
		globalVar.SrvAuction.Goods = make(map[uint64]*pb3.SrvAuctionGoods)
	}
	if nil == globalVar.SrvAuction.Bonus {
		globalVar.SrvAuction.Bonus = make(map[uint32]*pb3.SrvAuctionBonusCal)
	}
	return globalVar.SrvAuction
}

func onServerInit(_ ...interface{}) {
	conf := jsondata.GetAuctionConf()
	if nil == conf {
		return
	}
	bonusChecker = time_util.NewTimeChecker(time.Duration(conf.BonusCalTime) * time.Second)
	AuctionMgrInstance.initMgr()
}

func RunOne() {
	conf := jsondata.GetAuctionConf()
	if nil == conf {
		return
	}
	if nil == bonusChecker {
		bonusChecker = time_util.NewTimeChecker(time.Duration(conf.BonusCalTime) * time.Second)
	}
	if bonusChecker.CheckAndSet(false) {
		AuctionMgrInstance.checkSettlement()
	}
}

func (mgr *AuctionMgr) initMgr() {
	data := mgr.getAuctionData()
	keys := maps.Keys(data.Goods)
	for _, goodsId := range keys {
		mgr.checkGoodsTimer(goodsId, true)
	}
}

func (mgr *AuctionMgr) checkSettlement() {
	conf := jsondata.GetSectAuctionConf()
	if nil == conf {
		return
	}
	data := mgr.getAuctionData()
	bonusData := data.Bonus
	data.Bonus = nil

	for bossId, record := range bonusData {
		var bossName string
		if bossConf := jsondata.GetMonsterConf(bossId); nil != bossConf {
			bossName = bossConf.Name
		}
		for actorId, bonus := range record.Bonus {
			var rewards jsondata.StdRewardVec
			for mType, mCount := range bonus.Money {
				rewards = append(rewards, &jsondata.StdReward{
					Id:    jsondata.GetMoneyIdConfByType(mType),
					Count: mCount,
				})
			}
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  common.Mail_AuctionBonus,
				Rewards: rewards,
				Content: &mailargs.SectAuctionBonusArgs{
					Name: bossName,
				},
			})
		}
	}
}

func (mgr *AuctionMgr) generateGoodsId(pfId, srvId uint32) (uint64, error) {
	data := mgr.getAuctionData()
	series, err := base.MakeAuctionGoodsId(pfId, srvId, data.Series)
	if nil != err {
		return 0, err
	}
	data.Series++
	return series, nil
}

func (mgr *AuctionMgr) GenerateGoods(sellInfo *auction.SoldInfo) (*pb3.SrvAuctionGoods, error) {
	goodsConf, ok := jsondata.GetSrvAuctionGoodsConf(sellInfo.ItemId, sellInfo.SoldType)
	if !ok {
		return nil, neterror.ParamsInvalidError("goods conf %d is nil", sellInfo.ItemId)
	}
	if sellInfo.Count <= 0 {
		return nil, neterror.ParamsInvalidError("goods count is 0")
	}

	id, err := mgr.generateGoodsId(engine.GetPfId(), engine.GetServerId())
	if nil != err {
		return nil, neterror.InternalError("auction goods put err:%v, put data: %v", err, sellInfo)
	}

	switch sellInfo.SoldType {
	case auction.SoldTypeSys:
		return mgr.createSysGoods(id, sellInfo, goodsConf.(*jsondata.AuctionSysGoodsConf))
	case auction.SoldTypePersonal:
		return mgr.createPersonGoods(id, sellInfo, goodsConf.(*jsondata.AuctionPersonalGoodsConf))
	}

	return nil, neterror.ParamsInvalidError("unknown sole type")
}

func (mgr *AuctionMgr) createSysGoods(id uint64, sellInfo *auction.SoldInfo, goodsConf *jsondata.AuctionSysGoodsConf) (*pb3.SrvAuctionGoods, error) {
	// price * (常数比率 / 1000) * {rate + [(10000-sum(rate1,rate2...)/基准的人数]} / 10000
	// price * 常数 * 玩家比例常数
	calBonus := func() (bonus []*pb3.SrvAuctionBonusSt) {
		var totalRate, baseRate uint32
		for actorId, rate := range sellInfo.Bonus {
			totalRate += rate
			bonus = append(bonus, &pb3.SrvAuctionBonusSt{
				ActorId: actorId,
				AddRate: rate,
			})
		}
		if totalRate < 10000 {
			baseRate = 10000 - totalRate
		}
		defaultNum := utils.MaxUInt32(jsondata.GetAuctionConf().BaseNum, uint32(len(sellInfo.Bonus)))
		baseRate = baseRate / defaultNum
		for _, v := range bonus {
			v.AddRate += baseRate
		}
		return
	}
	goods := &pb3.SrvAuctionGoods{
		Id:        id,
		ItemId:    sellInfo.ItemId,
		Count:     sellInfo.Count,
		Status:    auction.AuctionGoodsStatusWaiting,
		BossId:    sellInfo.RelationBossId,
		Bonus:     calBonus(),
		SoldType:  sellInfo.SoldType,
		MoneyType: goodsConf.MoneyType,
	}
	return goods, nil
}

func (mgr *AuctionMgr) createPersonGoods(id uint64, sellInfo *auction.SoldInfo, goodsConf *jsondata.AuctionPersonalGoodsConf) (*pb3.SrvAuctionGoods, error) {
	goods := &pb3.SrvAuctionGoods{
		Id:           id,
		ItemId:       sellInfo.ItemId,
		Count:        sellInfo.Count,
		Status:       auction.AuctionGoodsStatusWaiting,
		BossId:       sellInfo.RelationBossId,
		SoldPlayerId: sellInfo.SoldActorId,
		SoldType:     sellInfo.SoldType,
		MoneyType:    goodsConf.MoneyType,
	}
	return goods, nil
}

func (mgr *AuctionMgr) PutIntoAuction(goods *pb3.SrvAuctionGoods, startTime uint32) error {
	if nil == goods {
		return neterror.InternalError("goods is nil")
	}

	conf := jsondata.GetAuctionConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	goodsConf, ok := jsondata.GetSrvAuctionGoodsConf(goods.GetItemId(), goods.GetSoldType())
	if !ok {
		return neterror.InternalError("goods item %d type %d conf is nil", goods.GetItemId(), goods.GetSoldType())
	}
	var continueTime uint32
	curTime := time_util.NowSec()

	defaultCd := conf.OpenCd

	switch goods.SoldType {
	case auction.SoldTypeSys:
		continueTime = goodsConf.(*jsondata.AuctionSysGoodsConf).CountDown
	case auction.SoldTypePersonal:
		canPartiBid := goodsConf.(*jsondata.AuctionPersonalGoodsConf).StartingPrice > 0
		continueTime = utils.Ternary(canPartiBid, conf.PersonalBidTime, conf.PersonalBuyItNowTime).(uint32)
		defaultCd = utils.Ternary(canPartiBid, defaultCd, uint32(0)).(uint32)
	default:
		return neterror.InternalError("invalid type")
	}

	if startTime == 0 {
		startTime = curTime + defaultCd
	}

	goods.StartTime = startTime
	goods.EndTime = startTime + continueTime

	data := mgr.getAuctionData()
	if _, hasGoods := data.Goods[goods.GetId()]; hasGoods {
		return neterror.InternalError("repeated add")
	}

	data.Goods[goods.GetId()] = goods
	var dur int64
	if startTime > curTime {
		dur = int64(startTime - curTime)
	}
	mgr.detail[goods.GetId()] = timer.SetTimeout(time.Duration(dur)*time.Second, func() {
		mgr.checkGoodsTimer(goods.GetId(), false)
	})
	mgr.broadcastGoods(goods.GetId())
	return nil
}

func (mgr *AuctionMgr) tryNextStatus(goods *pb3.SrvAuctionGoods) bool {
	curTime := time_util.NowSec()
	if goods.Status == auction.AuctionGoodsStatusWaiting && goods.StartTime <= curTime {
		mgr.onStart(goods)
		return true
	}
	if goods.Status == auction.AuctionGoodsStatusStarting && goods.EndTime <= curTime {
		mgr.onTimeOut(goods)
		return true
	}
	if goods.Status == auction.AuctionGoodsStatusEnding {
		mgr.onPullOff(goods)
		return true
	}
	return false
}

func (mgr *AuctionMgr) onStart(goods *pb3.SrvAuctionGoods) {
	goods.Status = auction.AuctionGoodsStatusStarting
}

func (mgr *AuctionMgr) onTimeOut(goods *pb3.SrvAuctionGoods) {
	goods.Status = auction.AuctionGoodsStatusEnding
	//判断是否需要确定竞价得标者
	if goods.SuccessfulBidder > 0 { //已确定得标者
		return
	}
	if goods.TheLastBidder == 0 { //无人竞价
		return
	}
	//竞价得标
	goods.BuyWay = auction.AuctionGoodsBuyWayBid
	goods.HammerPrice = goods.GetCurrentBid()
	goods.SuccessfulBidder = goods.GetTheLastBidder()
}

func (mgr *AuctionMgr) AdvertiseGoods(player iface.IPlayer, goodsId uint64) bool {
	goods, ok := mgr.getGoodsDataById(goodsId)
	if !ok {
		return false
	}
	if goods.GetSoldPlayerId() != player.GetId() {
		return false
	}
	engine.BroadcastTipMsgById(tipmsgid.AuctionTipsShow, engine.StdRewardToBroadcast(player, jsondata.StdRewardVec{
		{
			Id:    goods.GetItemId(),
			Count: int64(goods.GetCount()),
		},
	}))
	return true
}

func (mgr *AuctionMgr) TakeOffGoods(player iface.IPlayer, goodsId uint64) bool {
	goods, ok := mgr.getGoodsDataById(goodsId)
	if !ok {
		return false
	}

	if goods.GetSoldPlayerId() != player.GetId() {
		return false
	}

	if checkTimer, ok := mgr.detail[goodsId]; ok {
		checkTimer.Stop()
		checkTimer = nil
	}

	goods.Status = auction.AuctionGoodsStatusEnding //广播结束
	mgr.failedBidBack(goods)
	mgr.broadcastGoods(goodsId)

	delete(mgr.getAuctionData().Goods, goodsId)
	delete(mgr.detail, goods.GetId())

	if goods.IsCalc {
		return false
	}
	goods.IsCalc = true

	mailmgr.SendMailToActor(goods.GetSoldPlayerId(), &mailargs.SendMailSt{
		ConfId: common.Mail_AuctionSoldTakeOff,
		Rewards: jsondata.StdRewardVec{
			&jsondata.StdReward{
				Id:    goods.GetItemId(),
				Count: int64(goods.GetCount()),
			},
		},
	})

	player.TriggerEvent(custom_id.AeAuctionPersonalSoldEnd, goods.GetId())

	return true
}

func (mgr *AuctionMgr) sendBidAwards(goods *pb3.SrvAuctionGoods) {
	if goods.GetSuccessfulBidder() == 0 {
		return
	}
	mailmgr.SendMailToActor(goods.GetSuccessfulBidder(), &mailargs.SendMailSt{
		ConfId: common.Mail_AuctionBidSuccess,
		Rewards: jsondata.StdRewardVec{
			&jsondata.StdReward{
				Id:    goods.GetItemId(),
				Count: int64(goods.GetCount()),
			},
		},
	})
	engine.SendPlayerMessage(goods.GetSuccessfulBidder(), gshare.OfflineAddAuctionRecord, &pb3.SrvAuctionRecord{
		ItemId:         goods.GetItemId(),
		Count:          goods.GetCount(),
		MoneyType:      goods.GetMoneyType(),
		Price:          goods.GetHammerPrice(),
		BuyWay:         goods.GetBuyWay(),
		TimeStamp:      time_util.NowSec(),
		SoldPlayerId:   goods.GetSoldPlayerId(),
		SoldCommission: jsondata.GetAuctionConf().PersonalCommission,
		RecordType:     auction.AuctionGoodsRecordTypeBuy,
	})
}

func (mgr *AuctionMgr) calcSysGoods(goods *pb3.SrvAuctionGoods, goodsConf *jsondata.AuctionSysGoodsConf) {
	price := goods.GetHammerPrice()
	if price == 0 {
		price = goodsConf.PassBonusPrice * goods.GetCount()
	}
	for _, v := range goods.Bonus {
		mgr.addBonus(goods.GetBossId(), v.GetActorId(), goodsConf.BonusMoneyType, jsondata.GetAuctionConf().GetBonus(int64(price), v.GetAddRate()))
	}
}

func (mgr *AuctionMgr) calcPersonalGoods(goods *pb3.SrvAuctionGoods) {
	if goods.GetSuccessfulBidder() == 0 {
		mailmgr.SendMailToActor(goods.GetSoldPlayerId(), &mailargs.SendMailSt{
			ConfId: common.Mail_AuctionSoldBack,
			Rewards: jsondata.StdRewardVec{
				&jsondata.StdReward{
					Id:    goods.GetItemId(),
					Count: int64(goods.GetCount()),
				},
			},
		})
	} else {
		price := int64(goods.GetHammerPrice()) * utils.MaxInt64(10000-int64(jsondata.GetAuctionConf().PersonalCommission), 0) / 10000
		mailmgr.SendMailToActor(goods.GetSoldPlayerId(), &mailargs.SendMailSt{
			ConfId: common.Mail_AuctionSoldAwards,
			Rewards: jsondata.StdRewardVec{
				&jsondata.StdReward{
					Id:    jsondata.GetMoneyIdConfByType(goods.GetMoneyType()),
					Count: price,
				},
			},
			Content: &mailargs.AuctionSoldSuccess{
				MoneyCount: goods.GetHammerPrice(),
				MoneyName:  jsondata.GetItemName(jsondata.GetMoneyIdConfByType(goods.GetMoneyType())),
				Tax:        float64(jsondata.GetAuctionConf().PersonalCommission) / 100,
				Profit:     uint32(price),
				ItemName:   jsondata.GetItemName(goods.GetItemId()),
				ItemCount:  goods.GetCount(),
			},
		})
	}
	if soldPlayer := manager.GetPlayerPtrById(goods.GetSoldPlayerId()); nil != soldPlayer {
		soldPlayer.TriggerEvent(custom_id.AeAuctionPersonalSoldEnd, goods.GetId())
	}
	engine.SendPlayerMessage(goods.GetSoldPlayerId(), gshare.OfflineAddAuctionRecord, &pb3.SrvAuctionRecord{
		ItemId:         goods.GetItemId(),
		Count:          goods.GetCount(),
		MoneyType:      goods.GetMoneyType(),
		Price:          goods.GetHammerPrice(),
		BuyWay:         goods.GetBuyWay(),
		TimeStamp:      time_util.NowSec(),
		SoldPlayerId:   goods.GetSoldPlayerId(),
		SoldCommission: jsondata.GetAuctionConf().PersonalCommission,
		RecordType:     auction.AuctionGoodsRecordTypeSold,
	})
}

func (mgr *AuctionMgr) onPullOff(goods *pb3.SrvAuctionGoods) {
	data := mgr.getAuctionData()

	if checkTimer, ok := mgr.detail[goods.GetId()]; ok {
		checkTimer.Stop()
		checkTimer = nil
	}

	delete(data.Goods, goods.GetId())
	delete(mgr.detail, goods.GetId())

	if goods.IsCalc {
		return
	}
	goods.IsCalc = true

	goodsConf, ok := jsondata.GetSrvAuctionGoodsConf(goods.GetItemId(), goods.GetSoldType())
	if !ok {
		return
	}

	if goods.GetSuccessfulBidder() > 0 {
		mgr.sendBidAwards(goods)
	}

	if goods.GetSoldType() == auction.SoldTypeSys {
		mgr.calcSysGoods(goods, goodsConf.(*jsondata.AuctionSysGoodsConf))
	} else if goods.GetSoldType() == auction.SoldTypePersonal {
		mgr.calcPersonalGoods(goods)
	}
}

func (mgr *AuctionMgr) addBonus(bossId uint32, actorId uint64, moneyType uint32, count int64) {
	data := mgr.getAuctionData()
	if nil == data.Bonus[bossId] {
		data.Bonus[bossId] = &pb3.SrvAuctionBonusCal{}
	}
	if nil == data.Bonus[bossId].Bonus {
		data.Bonus[bossId].Bonus = make(map[uint64]*pb3.SrvAuctionPersonlBonusCal)
	}
	if nil == data.Bonus[bossId].Bonus[actorId] {
		data.Bonus[bossId].Bonus[actorId] = &pb3.SrvAuctionPersonlBonusCal{}
	}
	if nil == data.Bonus[bossId].Bonus[actorId].Money {
		data.Bonus[bossId].Bonus[actorId].Money = make(map[uint32]int64)
	}
	data.Bonus[bossId].Bonus[actorId].Money[moneyType] += count
}

func (mgr *AuctionMgr) checkGoodsTimer(id uint64, isInit bool) {
	goods, ok := mgr.getGoodsDataById(id)
	if !ok {
		return
	}
	change := mgr.tryNextStatus(goods)
	if change && !isInit {
		mgr.broadcastGoods(id)
	}

	curTime := time_util.NowSec()
	nowStatus := goods.GetStatus()

	var nextCheckTime int64
	switch nowStatus {
	case auction.AuctionGoodsStatusWaiting:
		nextCheckTime = int64(goods.StartTime) - int64(curTime)
	case auction.AuctionGoodsStatusStarting:
		nextCheckTime = int64(goods.EndTime) - int64(curTime)
	case auction.AuctionGoodsStatusEnding:
		mgr.checkGoodsTimer(goods.GetId(), isInit)
		return
	}

	if nextCheckTime <= 0 {
		nextCheckTime = 0
	}

	if checkTimer, ok := mgr.detail[id]; ok {
		checkTimer.Stop()
		checkTimer = nil
	}

	if nowStatus != auction.AuctionGoodsStatusEnding {
		mgr.detail[id] = timer.SetTimeout(time.Duration(nextCheckTime)*time.Second, func() {
			mgr.checkGoodsTimer(id, false)
		})
	}
}

func (mgr *AuctionMgr) broadcastGoods(id uint64) {
	goods, ok := mgr.getGoodsDataById(id)
	if !ok {
		return
	}
	engine.Broadcast(chatdef.CIWorld, 0, 70, 53, &pb3.S2C_70_53{Goods: goods}, 0)
}

func (mgr *AuctionMgr) SendGoodsInfo(player iface.IPlayer) {
	data := mgr.getAuctionData()
	result := make([]*pb3.SrvAuctionGoods, 0, len(data.GetGoods()))
	for _, v := range data.Goods {
		if v.Status == auction.AuctionGoodsStatusEnding {
			continue
		}
		result = append(result, v)
	}
	player.SendProto3(70, 52, &pb3.S2C_70_52{Goods: result})
}

func (mgr *AuctionMgr) getGoodsDataById(id uint64) (*pb3.SrvAuctionGoods, bool) {
	goods, ok := mgr.getAuctionData().Goods[id]
	return goods, ok
}

func (mgr *AuctionMgr) IsGoodsExist(id uint64) bool {
	goods, ok := mgr.getGoodsDataById(id)
	if !ok {
		return false
	}
	if goods.Status == auction.AuctionGoodsStatusEnding {
		return false
	}
	return true
}

func (mgr *AuctionMgr) canActorBuy(player iface.IPlayer, goods *pb3.SrvAuctionGoods, goodsConf jsondata.IAuctionGoodsBaseConf, info *auction.BidInfo) (bool, error) {
	goodsId := goods.GetId()
	curTime := time_util.NowSec()
	if goods.GetStatus() != auction.AuctionGoodsStatusStarting {
		return false, neterror.ParamsInvalidError("no goods not start", goodsId)
	}
	if goods.GetStartTime() > curTime || goods.GetEndTime() < curTime {
		return false, neterror.InternalError("goods %d time not open", goodsId)
	}
	if goods.GetSuccessfulBidder() > 0 {
		return false, neterror.ParamsInvalidError("goods %d is bidden", goodsId)
	}
	if goods.GetIsCalc() {
		return false, neterror.InternalError("goods %d is off", goodsId)
	}

	if info.BuyWay == auction.AuctionGoodsBuyWayBid {
		if goods.GetCurrentBid() > 0 && goods.GetCurrentBid() != info.SeeBid {
			return false, neterror.ParamsInvalidError("goods %d bid price is change", goodsId)
		}
		if goods.GetTheLastBidder() == player.GetId() {
			return false, neterror.ParamsInvalidError("goods %d the last bidder is self", goodsId)
		}
		if goodsConf.GetStartingPrice() == 0 {
			return false, neterror.ParamsInvalidError("goods not allow bid", goodsId)
		}
	}

	return true, nil
}

func (mgr *AuctionMgr) BidGoods(player iface.IPlayer, info *auction.BidInfo) (bool, error) {
	goods, ok := mgr.getGoodsDataById(info.GoodsId)
	if !ok {
		return false, neterror.ParamsInvalidError("goods is nil")
	}
	goodsConf, ok := jsondata.GetSrvAuctionGoodsConf(goods.GetItemId(), goods.GetSoldType())
	if !ok {
		return false, neterror.InternalError("goods item %d type %d conf is nil", goods.GetItemId(), goods.GetSoldType())
	}
	ok, err := mgr.canActorBuy(player, goods, goodsConf, info)
	if !ok {
		return false, err
	}

	var success bool
	switch info.BuyWay {
	case auction.AuctionGoodsBuyWayBid:
		success, err = mgr.handleBid(player, goods, goodsConf, info)
	case auction.AuctionGoodsBuyWayBuyItNowPrice:
		success, err = mgr.handleBuyItNow(player, goods, goodsConf)
	default:
		return false, neterror.ParamsInvalidError("invalid buy way")
	}

	if !success {
		return false, err
	}

	mgr.broadcastGoods(info.GoodsId)

	if goods.Status == auction.AuctionGoodsStatusEnding {
		mgr.checkGoodsTimer(info.GoodsId, false)
	}
	return true, nil
}

func (mgr *AuctionMgr) failedBidBack(goods *pb3.SrvAuctionGoods) {
	if goods.GetTheLastBidder() <= 0 {
		return
	}
	actorId := goods.GetTheLastBidder()
	goods.TheLastBidder = 0

	var backReward jsondata.StdRewardVec
	for _, v := range goods.BidMoney {
		backReward = append(backReward, &jsondata.StdReward{
			Id:    jsondata.GetMoneyIdConfByType(v.Key),
			Count: int64(v.Value),
		})
	}

	goods.BidMoney = nil

	mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
		ConfId:  common.Mail_AuctionFailBack,
		Rewards: backReward,
	})
}

func (mgr *AuctionMgr) payConsume(player iface.IPlayer, moneyType uint32, price uint32) (bool, []*pb3.KeyVal64) {
	consumeMoney := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeMoney,
			Id:    moneyType,
			Count: price,
		},
	}

	success, remove := player.ConsumeByConfWithRet(consumeMoney, false, common.ConsumeParams{
		LogId: pb3.LogId_LogAuctionBidConsume,
	})

	if !success {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false, nil
	}

	var bidMoney []*pb3.KeyVal64
	for k, v := range remove.MoneyMap {
		if v == 0 {
			continue
		}
		bidMoney = append(bidMoney, &pb3.KeyVal64{
			Key:   k,
			Value: uint64(v),
		})
	}

	return true, bidMoney
}

func (mgr *AuctionMgr) handleBid(player iface.IPlayer, goods *pb3.SrvAuctionGoods, goodsConf jsondata.IAuctionGoodsBaseConf, info *auction.BidInfo) (bool, error) {
	price := goodsConf.GetStartingPrice() * goods.GetCount()
	if goods.GetCurrentBid() > 0 {
		price = goods.GetCurrentBid() + goodsConf.GetPlaceBidRange()*goods.GetCount()
	}

	success, bidMoney := mgr.payConsume(player, goodsConf.GetMoneyType(), price)

	if !success {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false, nil
	}

	mgr.failedBidBack(goods)

	goods.TheLastBidder = player.GetId()
	goods.CurrentBid = price
	goods.BidMoney = bidMoney

	if goods.GetCurrentBid() >= goodsConf.GetBuyItNowPrice()*goods.GetCount() {
		goods.Status = auction.AuctionGoodsStatusEnding
		goods.SuccessfulBidder = player.GetId()
		goods.BuyWay = auction.AuctionGoodsBuyWayBid
		goods.HammerPrice = goods.GetCurrentBid()
		return true, nil
	}

	curTime, resetCountdown := time_util.NowSec(), jsondata.GetAuctionConf().ResetCountdown
	if goods.GetEndTime()-curTime < resetCountdown {
		goods.EndTime = curTime + resetCountdown
	}
	return true, nil
}

func (mgr *AuctionMgr) handleBuyItNow(player iface.IPlayer, goods *pb3.SrvAuctionGoods, goodsConf jsondata.IAuctionGoodsBaseConf) (bool, error) {
	price := goodsConf.GetBuyItNowPrice() * goods.GetCount()
	success, bidMoney := mgr.payConsume(player, goodsConf.GetMoneyType(), price)

	if !success {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return false, nil
	}
	goods.Status = auction.AuctionGoodsStatusEnding
	goods.SuccessfulBidder = player.GetId()
	goods.BuyWay = auction.AuctionGoodsBuyWayBuyItNowPrice
	goods.HammerPrice = price
	mgr.failedBidBack(goods)

	goods.BidMoney = bidMoney
	return true, nil
}

func (mgr *AuctionMgr) AllGoodsDo(fn func(goods *pb3.SrvAuctionGoods)) {
	data := mgr.getAuctionData()
	for _, goods := range data.Goods {
		fn(goods)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, onServerInit)
}
