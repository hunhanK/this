package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/random"
)

const (
	waitBuy = 0
	hasBuy  = 1
)

type OriginTreasureSystem struct {
	Base
	timer *time_util.Timer
}

func (sys *OriginTreasureSystem) onNewDay() {
	shopList := sys.GetBinaryData().GetOtShop()
	for _, shop := range shopList {
		shop.RefreshTimes = 0
		shop.CosGetDay = 0
		shop.Lucky = 0
		sys.SendProto3(12, 5, &pb3.S2C_12_5{Shop: shop.Shop, RefreshTimes: shop.RefreshTimes})
		sys.SendProto3(12, 6, &pb3.S2C_12_6{Shop: shop.Shop, CosGetDay: shop.CosGetDay})
	}
}

func (sys *OriginTreasureSystem) OnAfterLogin() {
	confList := jsondata.GetOriginTreasureConf()
	if nil == confList {
		return
	}
	nowSec := time_util.NowSec()
	var refreshTime int64
	for _, conf := range confList {
		shop := sys.GetShop(conf.StoreType)
		if nil == shop {
			sys.reset(conf.StoreType, conf, true)
			continue
		}
		resetTime := shop.LastRefreshTimes + conf.RefreshTime
		if nowSec >= resetTime {
			diff := nowSec - resetTime
			lucky := diff/conf.RefreshTime + 1
			diff = diff % conf.RefreshTime
			shop.LastRefreshTimes = nowSec - diff
			shop.Lucky += lucky
			sys.reset(conf.StoreType, conf, false)
		} else { //没到刷新时间
		}
		nt := sys.getRefreshTime(conf)
		if nt == 0 {
			nt = 1
		}
		if refreshTime == 0 || nt < refreshTime {
			refreshTime = nt
		}
		sys.SendProto3(12, 2, &pb3.S2C_12_2{Shop: shop})
	}
	if refreshTime > 0 {
		if nil != sys.timer {
			sys.timer.Stop()
		}
		sys.timer = sys.GetOwner().SetTimeout(time.Duration(refreshTime)*time.Second, func() {
			sys.resetAll()
		})
	}
}

func (sys *OriginTreasureSystem) OnReconnect() {
	confList := jsondata.GetOriginTreasureConf()
	if nil == confList {
		return
	}
	for _, conf := range confList {
		if shop := sys.GetShop(conf.StoreType); nil != shop {
			sys.SendProto3(12, 2, &pb3.S2C_12_2{Shop: shop})
		}
	}
}

func (sys *OriginTreasureSystem) OnLogout() {
	if nil != sys.timer {
		sys.timer.Stop()
		sys.timer = nil
	}
}

func (sys *OriginTreasureSystem) GetShop(shop uint32) *pb3.OriginTreasuresShop {
	binary := sys.GetBinaryData()
	shopList := binary.GetOtShop()
	for _, v := range shopList {
		if v.Shop == shop {
			return v
		}
	}
	return nil
}

func (sys *OriginTreasureSystem) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetOtShop() {
		binary.OtShop = make(map[uint32]*pb3.OriginTreasuresShop)
	}
}

func (sys *OriginTreasureSystem) OnOpen() {
	confList := jsondata.GetOriginTreasureConf()
	if nil == confList {
		return
	}
	binary := sys.GetBinaryData()
	for _, conf := range confList {
		shop := &pb3.OriginTreasuresShop{
			Shop:             conf.StoreType,
			RefreshTimes:     0,
			Goods:            nil,
			CosGetDay:        0,
			LastRefreshTimes: 0,
		}
		binary.OtShop[conf.StoreType] = shop
	}
	sys.resetAll()
}

func (sys *OriginTreasureSystem) c2sInfo(msg *base.Message) error {
	var req pb3.C2S_12_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if shop := sys.GetShop(req.GetShop()); nil != shop {
		sys.SendProto3(12, 2, &pb3.S2C_12_2{Shop: shop})
	} else {
		return neterror.InternalError("shop(%d) not found", req.GetShop())
	}
	return nil
}

func (sys *OriginTreasureSystem) c2sRefresh(msg *base.Message) error {
	var req pb3.C2S_12_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	conf := jsondata.GetOriginTreasureShopConf(req.GetShop())
	if nil == conf {
		return neterror.ParamsInvalidError("shop conf (%d) not found", req.GetShop())
	}
	shop := sys.GetShop(req.GetShop())
	if nil == shop {
		return neterror.InternalError("shop(%d) not found", req.GetShop())
	}
	if shop.RefreshTimes < conf.Freenumber {
		shop.RefreshTimes++
	} else {
		times := shop.RefreshTimes - conf.Freenumber + 1
		var consume []*jsondata.Consume
		if len(conf.Moneynumber) <= int(times) {
			consume = conf.Moneynumber[len(conf.Moneynumber)-1].Contype
		} else {
			consume = conf.Moneynumber[times-1].Contype
		}
		if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogOriginTreasureRefresh}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		shop.RefreshTimes++
	}
	sys.reset(req.GetShop(), conf, false)
	sys.SendProto3(12, 5, &pb3.S2C_12_5{
		Shop:         req.GetShop(),
		RefreshTimes: shop.RefreshTimes,
	})
	return nil
}

func (sys *OriginTreasureSystem) c2sBuyGoods(msg *base.Message) error {
	var req pb3.S2C_12_4
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	conf := jsondata.GetOriginTreasureShopConf(req.GetShop())
	if nil == conf {
		return neterror.ParamsInvalidError("shop conf (%d) not found", req.GetShop())
	}
	shop := sys.GetShop(req.GetShop())
	if nil == shop {
		return neterror.InternalError("shop(%d) not found", req.GetShop())
	}
	if len(shop.Goods) <= int(req.GetIdx()) {
		return neterror.InternalError("shop(%d) idx(%d) exceed", req.GetShop(), req.GetIdx())
	}
	goods := shop.Goods[req.GetIdx()]
	if goods.Value == hasBuy {
		return neterror.InternalError("shop(%d) idx(%d) has buy", req.GetShop(), req.GetIdx())
	}
	goodsConf := conf.Goods[goods.Key]
	if nil == goodsConf {
		return neterror.ParamsInvalidError("goods conf (%d) not found", goods.Key)
	}

	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeItem, Id: conf.MoneyId, Count: goodsConf.Price},
	}

	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogOriginTreasureBuy}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	goods.Value = hasBuy
	engine.GiveRewards(sys.owner, conf.Goods[goods.Key].Item, common.EngineGiveRewardParam{LogId: pb3.LogId_LogOriginTreasureBuy})
	sys.owner.TriggerQuestEvent(custom_id.QttBuyOriginTreasure, 0, 1)
	sys.owner.TriggerQuestEvent(custom_id.QttBuyOriginFromTreasure, 0, 1)
	sys.owner.TriggerQuestEvent(custom_id.QttStoreBuyCnt, req.GetShop(), 1)
	sys.SendProto3(12, 4, &pb3.S2C_12_4{Shop: req.GetShop(), Idx: req.GetIdx()})
	for _, v := range goodsConf.Item {
		itemConf := jsondata.GetItemConfig(v.Id)
		if itemConf.Stage > jsondata.OriginCfg.ExchangeBroadcastLv {
			engine.BroadcastTipMsgById(tipmsgid.BYMB_broadcast1, sys.owner.GetName(), conf.Name, itemConf.Name)
		}
	}

	return nil
}

func (sys *OriginTreasureSystem) resetAll() {
	confList := jsondata.GetOriginTreasureConf()
	if nil == confList {
		return
	}

	for _, conf := range confList {
		sys.reset(conf.StoreType, conf, true)
	}

	var refreshTime int64
	for _, conf := range confList {
		nt := sys.getRefreshTime(conf)
		if refreshTime == 0 || nt < refreshTime {
			refreshTime = nt
		}
	}
	if refreshTime == 0 { //+1s延迟
		refreshTime = 1
	}
	if nil != sys.timer {
		sys.timer.Stop()
	}
	sys.timer = sys.GetOwner().SetTimeout(time.Duration(refreshTime)*time.Second, func() {
		sys.resetAll()
	})
}

func (sys *OriginTreasureSystem) getRefreshTime(conf *jsondata.OriginTreasureConf) int64 {
	shop := sys.GetShop(conf.StoreType)
	nt := int64(shop.LastRefreshTimes + conf.RefreshTime)
	nt -= time_util.Now().Unix()
	if nt <= 0 {
		nt = 0
	}
	return nt
}

func (sys *OriginTreasureSystem) reset(shopType uint32, conf *jsondata.OriginTreasureConf, isAuto bool) {
	binary := sys.GetBinaryData()
	shop := sys.GetShop(shopType)
	nowSec := time_util.NowSec()
	if nil == shop {
		binary.OtShop[conf.StoreType] = &pb3.OriginTreasuresShop{
			Shop:             conf.StoreType,
			LastRefreshTimes: 0,
		}
		shop = sys.GetShop(shopType)
	}
	if isAuto {
		nt := sys.getRefreshTime(conf)
		if nt > 0 { //未到自动刷新的时间
			return
		}
	}
	luckypool := new(random.Pool)
	pool := new(random.Pool)
	day := gshare.GetOpenServerDay()
	shop.Lucky++ //幸运值
	var luckyItem, commonItem uint32
	for goodsId, line := range conf.Goods {
		if line.StartDay > 0 && day < line.StartDay {
			continue
		}
		lucky := shop.Lucky
		if stepConf := jsondata.GetOriginTreasureGoodsStepConf(goodsId); nil != stepConf {
			times := shop.Lucky
			isFind := false
			for _, step := range stepConf {
				if step == times {
					luckypool.AddItem(goodsId, line.Weight)
					luckyItem++
					isFind = true
					break
				}
			}
			if !isFind && int(times) >= len(line.Refreshmust) {
				lucky -= stepConf[times-1]
				if lucky%line.Refreshmust[len(line.Refreshmust)-1] == 0 {
					luckypool.AddItem(goodsId, line.Weight)
					luckyItem++
				}

			}
		}
		pool.AddItem(goodsId, line.Weight)
	}
	var goods []*pb3.KeyValue
	commonItem = conf.TreasureCount
	if luckyItem > 0 {
		if luckyItem >= conf.TreasureCount {
			luckyItem = conf.TreasureCount
			commonItem = 0
		} else {
			commonItem = conf.TreasureCount - luckyItem
		}
		for i := uint32(0); i <= luckyItem; i++ {
			line := luckypool.RandomOne()
			if goodsId, ok := line.(uint32); ok {
				goods = append(goods, &pb3.KeyValue{Key: goodsId, Value: waitBuy})
			}
		}
	}
	if commonItem > 0 {
		for i := uint32(0); i < commonItem; i++ {
			line := pool.RandomOne()
			if goodsId, ok := line.(uint32); ok {
				goods = append(goods, &pb3.KeyValue{Key: goodsId, Value: waitBuy})
			}
		}
	}

	shop.Goods = goods
	if isAuto {
		shop.LastRefreshTimes = nowSec
		sys.SendProto3(12, 2, &pb3.S2C_12_2{Shop: shop})
	} else {
		sys.SendProto3(12, 3, &pb3.S2C_12_3{Shop: shop.Shop, Goods: shop.Goods})
	}
}
func onKillMonster(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	sys, ok := actor.GetSysObj(sysdef.SiOriginTreasure).(*OriginTreasureSystem)
	if !ok || !sys.IsOpen() {
		return
	}

	confList := jsondata.GetOriginTreasureConf()
	if nil == confList {
		return
	}

	for _, conf := range confList {
		originEx, _ := actor.GetPrivilege(privilegedef.EnumOriginGet)
		maxGetDay := uint32(originEx) + conf.MaxGetDay
		shop := sys.GetShop(conf.StoreType)
		if nil == shop || maxGetDay <= shop.CosGetDay {
			continue
		}
		if drop, ok := conf.Getway[monId]; ok {
			count := drop.ItemCount
			if shop.CosGetDay+count > maxGetDay {
				count = maxGetDay - shop.CosGetDay
			}
			st := &itemdef.ItemParamSt{ItemId: drop.ItemId, Count: int64(count), Bind: true, LogId: pb3.LogId_LogOriginTreasureBoss}
			if sys.owner.AddItem(st) {
				shop.CosGetDay += count
			}
			sys.SendProto3(12, 6, &pb3.S2C_12_6{Shop: shop.Shop, CosGetDay: shop.CosGetDay})
			break
		}
	}

}

func init() {
	RegisterSysClass(sysdef.SiOriginTreasure, func() iface.ISystem {
		return &OriginTreasureSystem{}
	})
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiOriginTreasure).(*OriginTreasureSystem); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeKillMon, onKillMonster)

	net.RegisterSysProto(12, 2, sysdef.SiOriginTreasure, (*OriginTreasureSystem).c2sInfo)
	net.RegisterSysProto(12, 3, sysdef.SiOriginTreasure, (*OriginTreasureSystem).c2sRefresh)
	net.RegisterSysProto(12, 4, sysdef.SiOriginTreasure, (*OriginTreasureSystem).c2sBuyGoods)
}
