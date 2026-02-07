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
)

const (
	RefreshType0 = 0 // 永久
	RefreshType1 = 1 // 每日
	RefreshType2 = 2 // 每周
)

type QSStoreSystem struct {
	Base
}

func (sys *QSStoreSystem) OnReconnect() {
	sys.s2cInfo()
}

func (sys *QSStoreSystem) OnLogin() {
	sys.s2cInfo()
}

func (sys *QSStoreSystem) OnOpen() {
	data := sys.GetData()
	data.RefreshTime = time_util.NowSec()
	sys.s2cInfo()
}

func (sys *QSStoreSystem) OnNewDay() {
	sys.dayRefresh()

	now := time_util.NowSec()
	data := sys.GetData()
	if !time_util.IsSameWeek(now, data.RefreshTime) {
		sys.weekRefresh()
		data.RefreshTime = now
	}

	sys.s2cInfo()
}

func (sys *QSStoreSystem) GetData() *pb3.QSStore {
	store := sys.GetBinaryData().QSStore
	if store == nil {
		store = &pb3.QSStore{}
		sys.GetBinaryData().QSStore = store
	}
	if store.BuyData == nil {
		store.BuyData = make(map[uint32]uint32)
	}
	return store
}

func (sys *QSStoreSystem) s2cInfo() {
	sys.SendProto3(30, 30, &pb3.S2C_30_30{
		Data: sys.GetData(),
	})
}

func (sys *QSStoreSystem) dayRefresh() {
	data := sys.GetData()
	for k := range data.BuyData {
		conf := jsondata.GetQSStoreGoodConf(k)
		if conf != nil && conf.RefreshType == RefreshType1 {
			data.BuyData[k] = 0
		}
	}
}

func (sys *QSStoreSystem) weekRefresh() {
	data := sys.GetData()
	for k := range data.BuyData {
		conf := jsondata.GetQSStoreGoodConf(k)
		if conf != nil && conf.RefreshType == RefreshType2 {
			data.BuyData[k] = 0
		}
	}
}

func (sys *QSStoreSystem) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_30_31
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return err
	}

	conf := jsondata.GetQSStoreGoodConf(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("goodId=%d config not find", req.Id)
	}

	data := sys.GetData()
	if data.BuyData[req.Id]+req.Count > conf.CountLimit {
		return neterror.ParamsInvalidError("goodId=%d buy count limit", req.Id)
	}

	openDay := gshare.GetOpenServerDay()
	if len(conf.OpenDay) >= 2 {
		if openDay < conf.OpenDay[0] || openDay > conf.OpenDay[1] {
			return neterror.ParamsInvalidError("goodId=%d buy openDay limit", req.Id)
		}
	}

	consumes := jsondata.ConsumeMulti(conf.Consume, req.Count)
	if !sys.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogQSStoreBuyConsume}) {
		sys.GetOwner().SendTipMsg(tipmsgid.TpUseItemFailed)
		return neterror.ParamsInvalidError("ConsumeByConf failed")
	}

	data.BuyData[req.Id] += req.Count
	engine.GiveRewards(sys.owner, []*jsondata.StdReward{{Id: conf.ItemId, Count: int64(conf.ItemCount * req.Count), Bind: conf.Bind}},
		common.EngineGiveRewardParam{LogId: pb3.LogId_LogQSStoreBuyAwards})

	sys.SendProto3(30, 31, &pb3.S2C_30_31{
		Id:    req.Id,
		Count: req.Count,
	})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiQiShangStore, func() iface.ISystem {
		return &QSStoreSystem{}
	})

	net.RegisterSysProtoV2(30, 31, sysdef.SiQiShangStore, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*QSStoreSystem).c2sBuy
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiQiShangStore).(*QSStoreSystem); ok && sys.IsOpen() {
			sys.OnNewDay()
		}
	})
}
