/**
 * @Author: lzp
 * @Date: 2025/3/7
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type BossSpiritStoreSys struct {
	Base
}

func (s *BossSpiritStoreSys) OnOpen() {
	s.refresh()
	s.s2cInfo()
}

func (s *BossSpiritStoreSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BossSpiritStoreSys) OnLogin() {
	s.s2cInfo()
}

func (s *BossSpiritStoreSys) onNewDay() {
	data := s.getData()
	data.FreeFresh = 0
	data.PayFresh = 0
	data.ItemCount = make(map[uint32]uint32)
	s.refresh()
	s.s2cInfo()
}

func (s *BossSpiritStoreSys) getData() *pb3.BossSpiritStoreData {
	binData := s.GetBinaryData()
	if binData.BsStoreData == nil {
		binData.BsStoreData = &pb3.BossSpiritStoreData{}
	}
	sData := binData.BsStoreData
	if sData.ItemInfo == nil {
		sData.ItemInfo = make(map[uint32]bool)
	}
	if sData.ItemCount == nil {
		sData.ItemCount = make(map[uint32]uint32)
	}
	return sData
}

func (s *BossSpiritStoreSys) s2cInfo() {
	s.SendProto3(30, 35, &pb3.S2C_30_35{Data: s.getData()})
}

func (s *BossSpiritStoreSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_36
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetBossSpiritShopItemConfById(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("id:%d shop good config not exit", req.Id)
	}

	data := s.getData()
	isBuy := data.ItemInfo[req.Id]
	if isBuy {
		return neterror.ParamsInvalidError("id:%d shop good has buy", req.Id)
	}

	if utils.SliceContainsUint32(data.BuyItems, req.Id) {
		return neterror.ParamsInvalidError("id:%d shop good only buy once", req.Id)
	}

	buyCount := data.ItemCount[req.Id]
	if buyCount >= conf.BuyCount {
		return neterror.ParamsInvalidError("id:%d, shop good buy count limit", req.Id)
	}

	if !s.owner.CheckBossSpirit(conf.ItemId) {
		return neterror.ParamsInvalidError("id:%d shop good has", req.Id)
	}

	if !s.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBossSpiritShopBuy}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.ItemInfo[req.Id] = true
	data.ItemCount[req.Id] += 1
	if conf.CanBuyOnce {
		data.BuyItems = append(data.BuyItems, req.Id)
	}
	var rewards []*jsondata.StdReward
	rewards = append(rewards, &jsondata.StdReward{
		Id:    conf.ItemId,
		Count: int64(conf.Count),
		Bind:  conf.Bind,
	})

	if !engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBossSpiritShopBuy}) {
		return neterror.InternalError("give rewards failed")
	}
	s.GetOwner().TriggerQuestEvent(custom_id.QttBossSpiritStoreBuy, 0, 1)
	s.SendProto3(30, 36, &pb3.S2C_30_36{
		Id:    req.Id,
		Count: data.ItemCount[req.Id],
	})
	return nil
}

func (s *BossSpiritStoreSys) c2sRefresh(msg *base.Message) error {
	var req pb3.C2S_30_37
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetBossSpiritShopConf()
	if conf == nil {
		return neterror.ParamsInvalidError("config no t exits")
	}

	data := s.getData()
	if req.IsFree && data.FreeFresh >= conf.FreeCount {
		return neterror.ParamsInvalidError("free count limit")
	}

	if !req.IsFree {
		if data.PayFresh >= conf.PayCount {
			return neterror.ParamsInvalidError("pay count limit")
		}
		consumes := jsondata.GetBossSpiritShopRefreshConsume(data.PayFresh + 1)
		if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogBossSpiritShopRefresh}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	}

	s.refresh()
	if req.IsFree {
		data.FreeFresh += 1
	} else {
		data.PayFresh += 1
	}

	s.s2cInfo()
	return nil
}

func (s *BossSpiritStoreSys) refresh() {
	conf := jsondata.GetBossSpiritShopConf()
	if conf == nil {
		return
	}

	bsLv := s.owner.GetSpiritLv()
	data := s.getData()
	data.ItemInfo = make(map[uint32]bool)

	pool := new(random.Pool)
	for _, v := range conf.ItemList {
		if bsLv < v.BsLv {
			continue
		}
		if data.ItemCount[v.Id] >= v.BuyCount {
			continue
		}
		pool.AddItem(v, v.Rate)
	}

	rets := pool.RandomMany(conf.NumLimit)

	for _, ret := range rets {
		value := ret.(*jsondata.BsShopItemConf)
		data.ItemInfo[value.Id] = false
	}
}

func init() {
	RegisterSysClass(sysdef.SiBossSpiritStore, func() iface.ISystem {
		return &BossSpiritStoreSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiBossSpiritStore).(*BossSpiritStoreSys); ok && s.IsOpen() {
			s.onNewDay()
		}
	})

	net.RegisterSysProto(30, 36, sysdef.SiBossSpiritStore, (*BossSpiritStoreSys).c2sBuy)
	net.RegisterSysProto(30, 37, sysdef.SiBossSpiritStore, (*BossSpiritStoreSys).c2sRefresh)
}
