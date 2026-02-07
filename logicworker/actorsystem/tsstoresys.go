/**
 * @Author: lzp
 * @Date: 2024/7/15
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type TSStoreSystem struct {
	Base
}

func (sys *TSStoreSystem) OnReconnect() {
	sys.s2cInfo()
}

func (sys *TSStoreSystem) OnLogin() {
	sys.s2cInfo()
}

func (sys *TSStoreSystem) OnOpen() {
	sys.s2cInfo()
}

func (sys *TSStoreSystem) GetData() *pb3.TSStore {
	store := gshare.GetStaticVar().TSStore
	if store == nil {
		store = &pb3.TSStore{}
		gshare.GetStaticVar().TSStore = store
	}
	if store.Goods == nil {
		store.Goods = make(map[uint32]*pb3.TSStoreGoodData)
	}
	return store
}

func (sys *TSStoreSystem) OnNewDay() {
	sys.s2cInfo()
}

func (sys *TSStoreSystem) s2cInfo() {
	data := sys.GetData()
	playerId := sys.owner.GetId()

	goodsInfo := make(map[uint32]uint32)
	buyInfo := make(map[uint32]uint32)

	for k, v := range data.Goods {
		goodsInfo[k] = v.BuyCount
		if buyCount, ok := v.BuyData[playerId]; ok {
			buyInfo[k] = buyCount
		}
	}

	sys.SendProto3(30, 20, &pb3.S2C_30_20{
		GoodsInfo: goodsInfo,
		BuyInfo:   buyInfo,
	})
}

func (sys *TSStoreSystem) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_21
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return err
	}

	gConf := jsondata.GetTSStoreGoodConf(req.Id)
	if gConf == nil {
		return neterror.ParamsInvalidError("goodId=%d config not find", req.Id)
	}

	if len(gConf.OpenDays) < 2 {
		return neterror.ConfNotFoundError("goodId=%d config err", req.Id)
	}

	openDay := gshare.GetOpenServerDay()
	min, max := gConf.OpenDays[0], gConf.OpenDays[1]
	if gConf.MergeTimes > 0 && gConf.MergeTimes == gshare.GetMergeTimes() {
		openDay = gshare.GetMergeSrvDay()
	}
	if openDay < min || openDay > max {
		return neterror.ParamsInvalidError("goodId=%d not sell", req.Id)
	}

	data := sys.GetData()
	gData, ok := data.Goods[req.Id]
	if !ok {
		gData = &pb3.TSStoreGoodData{
			GoodId:   req.Id,
			BuyCount: 0,
			BuyData:  make(map[uint64]uint32),
		}
		data.Goods[req.Id] = gData
	}

	// 全服限购
	if gConf.CountLimit > 0 && gData.BuyCount+req.Count > gConf.CountLimit {
		return neterror.ParamsInvalidError("goodId=%d buy limit", req.Id)
	}

	// 个人限购
	playerId := sys.owner.GetId()
	if gData.BuyData != nil {
		buyCount, ok := gData.BuyData[playerId]
		if ok {
			if buyCount+req.Count > gConf.PerLimit {
				return neterror.ParamsInvalidError("goodId=%d buy perLimit", req.Id)
			}
		}
	}

	consumes := jsondata.ConsumeMulti(gConf.Consume, req.Count)
	if !sys.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogTSStoreBuyConsume}) {
		sys.GetOwner().SendTipMsg(tipmsgid.TpUseItemFailed)
		return neterror.ParamsInvalidError("ConsumeByConf failed")
	}

	gData.BuyCount += req.Count
	if gData.BuyData == nil {
		gData.BuyData = make(map[uint64]uint32)
	}
	gData.BuyData[playerId] += req.Count

	rewards := []*jsondata.StdReward{{Id: gConf.ItemId, Count: int64(gConf.ItemCount * req.Count), Bind: gConf.Bind}}
	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTSStoreBuyAwards})

	if gConf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(gConf.BroadcastId, sys.owner.GetId(), sys.owner.GetName(), engine.StdRewardToBroadcast(sys.owner, rewards))
	}

	sys.SendProto3(30, 21, &pb3.S2C_30_21{
		Id:    req.Id,
		Count: req.Count,
	})

	if gConf.CountLimit > 0 {
		engine.Broadcast(chatdef.CIWorld, 0, 30, 22, &pb3.S2C_30_22{
			Id:    req.Id,
			Count: gData.BuyCount,
		}, 0)
	}

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiTianShangStore, func() iface.ISystem {
		return &TSStoreSystem{}
	})

	net.RegisterSysProtoV2(30, 21, sysdef.SiTianShangStore, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TSStoreSystem).c2sBuy
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiTianShangStore).(*TSStoreSystem); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})
}
