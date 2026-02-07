/**
 * @Author: LvYuMeng
 * @Date: 2024/10/12
 * @Desc: 拍卖行
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/auction"
	"jjyz/base/custom_id/itemdef"
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
	"jjyz/gameserver/logicworker/auctionmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
)

type AuctionSys struct {
	Base
}

func (s *AuctionSys) OnAfterLogin() {
	s.clearInvalidParticipateIds()
	s.clearInvalidSoldIds()
	s.s2cGoodsInfo()
	s.s2cInfo()
}

func (s *AuctionSys) OnReconnect() {
	s.s2cGoodsInfo()
	s.s2cInfo()
}

func (s *AuctionSys) OnOpen() {
	s.defaultFollow()
	s.s2cInfo()
}

func (s *AuctionSys) defaultFollow() {
	data := s.getData()
	for _, v := range jsondata.AuctionDefaultFollowIds {
		conf := jsondata.GetItemConfig(v)
		if nil == conf {
			continue
		}
		if conf.Job > 0 && uint32(conf.Job) != s.owner.GetJob() {
			continue
		}
		if conf.Sex > 0 && uint32(conf.Sex) != s.owner.GetSex() {
			continue
		}
		data.FollowItemIds = append(data.FollowItemIds, v)
	}
	s.checkEquipFollow()
}

func (s *AuctionSys) checkEquipFollow() {
	conf := jsondata.GetAuctionConf()
	if nil == conf {
		return
	}

	data := s.getData()
	var newFollow []uint32

	nLv, lv, ok := jsondata.FindAuctionEquipFollowIds(s.owner.GetNirvanaLevel(), s.owner.GetLevel())
	if !ok {
		return
	}

	if data.FollowEquipNirvanaLevel == nLv && data.FollowEquipLevel == lv {
		return
	}

	data.FollowEquipNirvanaLevel = nLv
	data.FollowEquipLevel = lv

	needDel := func(itemId uint32) bool {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			return true
		}
		if itemdef.IsEquip(itemConf.Type, itemConf.SubType) {
			return true
		}
		return false
	}

	for _, itemId := range data.FollowItemIds {
		if !needDel(itemId) {
			newFollow = append(newFollow, itemId)
		}
	}

	needFollow := func(itemId uint32) bool {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			return false
		}
		if itemConf.Sex > 0 && uint32(itemConf.Sex) != s.owner.GetSex() {
			return false
		}
		if itemConf.Job > 0 && uint32(itemConf.Job) != s.owner.GetJob() {
			return false
		}
		return true
	}

	for _, itemId := range jsondata.AuctionDefaultEquipFollowIds[nLv][lv] {
		if needFollow(itemId) {
			newFollow = append(newFollow, itemId)
		}
	}

	data.FollowItemIds = newFollow
}

func (s *AuctionSys) clearInvalidParticipateIds() {
	data := s.getData()
	var newPartiIds []uint64
	for _, goodsId := range data.ParticipateIds {
		if auctionmgr.AuctionMgrInstance.IsGoodsExist(goodsId) {
			newPartiIds = append(newPartiIds, goodsId)
		}
	}
	data.ParticipateIds = newPartiIds
}

func (s *AuctionSys) clearInvalidSoldIds() {
	data := s.getData()
	var newSoldIds []uint64
	for _, goodsId := range data.SoldGoodsIds {
		if auctionmgr.AuctionMgrInstance.IsGoodsExist(goodsId) {
			newSoldIds = append(newSoldIds, goodsId)
		}
	}
	data.SoldGoodsIds = newSoldIds
}

func (s *AuctionSys) s2cInfo() {
	s.SendProto3(70, 50, &pb3.S2C_70_50{Data: s.getData()})
}

func (s *AuctionSys) s2cGoodsInfo() {
	auctionmgr.AuctionMgrInstance.SendGoodsInfo(s.owner)
}

func (s *AuctionSys) getData() *pb3.PlayerAuctionData {
	binary := s.owner.GetBinaryData()
	if nil == binary.PlayerAuctionData {
		binary.PlayerAuctionData = &pb3.PlayerAuctionData{}
	}
	return binary.PlayerAuctionData
}

func (s *AuctionSys) c2sSold(msg *base.Message) error {
	var req pb3.C2S_70_51
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	conf := jsondata.GetAuctionConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	data := s.getData()
	if conf.PersonalSoldCount <= uint32(len(data.SoldGoodsIds)) {
		return neterror.ParamsInvalidError("sold count limit")
	}

	soldCount := req.GetCount()

	item := s.owner.GetItemByHandle(req.GetItemHandle())
	if nil == item {
		return neterror.ParamsInvalidError("item %d is nil", req.GetItemHandle())
	}
	if item.GetBind() {
		return neterror.ParamsInvalidError("is bind")
	}
	if item.GetCount() == 0 || item.GetCount() < int64(soldCount) {
		return neterror.ParamsInvalidError("count is invalid")
	}

	goods, err := auctionmgr.AuctionMgrInstance.GenerateGoods(&auction.SoldInfo{
		ItemId:      item.GetItemId(),
		Count:       soldCount,
		SoldActorId: s.owner.GetId(),
		SoldType:    auction.SoldTypePersonal,
	})
	if nil != err {
		return neterror.Wrap(err)
	}

	if !s.owner.DeleteItemPtr(item, int64(soldCount), pb3.LogId_LogAuctionSoldItem) {
		return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
	}

	err = auctionmgr.AuctionMgrInstance.PutIntoAuction(goods, 0)
	if err != nil {
		return neterror.Wrap(err)
	}

	data.SoldGoodsIds = append(data.SoldGoodsIds, goods.GetId())

	s.SendProto3(70, 51, &pb3.S2C_70_51{
		GoodsId: goods.GetId(),
	})

	itemConf := jsondata.GetItemConfig(item.GetItemId())
	if itemConf.Type > 0 {
		s.owner.TriggerQuestEvent(custom_id.QttAuctionPersonalTotalSold, itemConf.Type, int64(req.GetCount()))
		s.owner.TriggerQuestEvent(custom_id.QttAuctionPersonalSold, itemConf.Type, int64(req.GetCount()))
	}

	if itemConf.Sex > 0 && s.owner.GetSex() != uint32(itemConf.Sex) {
		s.owner.TriggerQuestEvent(custom_id.QttAuctionPersonalTotalSoldByOtherSex, itemConf.Id, int64(req.GetCount()))
	}

	return nil
}

func (s *AuctionSys) c2sBid(msg *base.Message) error {
	var req pb3.C2S_70_54
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	_, err = auctionmgr.AuctionMgrInstance.BidGoods(s.owner, &auction.BidInfo{
		GoodsId: req.GetId(),
		BuyWay:  req.GetBuyWay(),
		SeeBid:  req.GetCurrentBid(),
	})
	if nil != err {
		return neterror.Wrap(err)
	}

	data := s.getData()
	data.ParticipateIds = append(data.ParticipateIds, req.GetId())
	s.SendProto3(70, 56, &pb3.S2C_70_56{Id: req.GetId()})
	return nil
}

func (s *AuctionSys) c2sHistoryRecord(msg *base.Message) error {
	var req pb3.C2S_70_55
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()
	s.SendProto3(70, 55, &pb3.S2C_70_55{Records: data.GetRecords()})
	return nil
}

func (s *AuctionSys) c2sFollowItem(msg *base.Message) error {
	var req pb3.C2S_70_57
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	data := s.getData()
	if req.GetIsFollow() {
		if _, ok := jsondata.GetSrvAuctionGoodsConf(req.GetItemId(), auction.SoldTypePersonal); !ok {
			return neterror.ConfNotFoundError("conf is nil")
		}
		if !pie.Uint32s(data.FollowItemIds).Contains(req.GetItemId()) {
			data.FollowItemIds = append(data.FollowItemIds, req.GetItemId())
		}
	} else {
		data.FollowItemIds = pie.Uint32s(data.FollowItemIds).Filter(func(u uint32) bool {
			return req.GetItemId() != u
		})
	}

	s.SendProto3(70, 57, &pb3.S2C_70_57{ItemId: req.GetItemId(), IsFollow: req.GetIsFollow()})

	return nil
}

func (s *AuctionSys) c2sAdvertise(msg *base.Message) error {
	var req pb3.C2S_70_58
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	data := s.getData()
	currentTime := time_util.NowSec()
	if data.AdviseSendTime > currentTime {
		s.owner.SendTipMsg(tipmsgid.AuctionCoolDown, data.AdviseSendTime-currentTime)
		return nil
	}

	if !auctionmgr.AuctionMgrInstance.AdvertiseGoods(s.owner, req.GetGoodsId()) {
		return neterror.ParamsInvalidError("advertise failed")
	}

	data.AdviseSendTime = currentTime + jsondata.GetAuctionConf().AdvertiseCd

	s.SendProto3(70, 58, &pb3.S2C_70_58{GoodsId: req.GetGoodsId()})
	return nil
}

func (s *AuctionSys) c2sGoodsTakeOff(msg *base.Message) error {
	var req pb3.C2S_70_59
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	auctionmgr.AuctionMgrInstance.TakeOffGoods(s.owner, req.GetGoodsId())

	return nil
}

func (s *AuctionSys) onSoldEnd(goodsId uint64) {
	data := s.getData()
	data.SoldGoodsIds = pie.Uint64s(data.SoldGoodsIds).Filter(func(u uint64) bool {
		return u != goodsId
	})
	s.SendProto3(70, 59, &pb3.S2C_70_59{GoodsId: goodsId})
}

func (s *AuctionSys) addRecord(record *pb3.SrvAuctionRecord) {
	data := s.getData()
	data.Records = append(data.Records, record)
	if len(data.Records) > int(jsondata.GetAuctionConf().RecordsCount) {
		data.Records = data.Records[1:]
	}
}

func auctionPersonalSoldEnd(player iface.IPlayer, args ...interface{}) {
	sys := player.GetSysObj(sysdef.SiAuction).(*AuctionSys)
	if sys != nil && !sys.IsOpen() {
		return
	}
	if len(args) < 1 {
		return
	}
	goodsId, ok := args[0].(uint64)
	if !ok {
		return
	}
	sys.onSoldEnd(goodsId)
}

func checkAuctionFollow(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiAuction)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*AuctionSys)
	if !ok {
		return
	}
	sys.checkEquipFollow()
	sys.s2cInfo()
}

func offlineAuctionRecord(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.SrvAuctionRecord)
	if !ok {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiAuction).(*AuctionSys)
	if !ok {
		return
	}
	sys.addRecord(st)
}

func init() {
	RegisterSysClass(sysdef.SiAuction, func() iface.ISystem {
		return &AuctionSys{}
	})

	event.RegActorEvent(custom_id.AeAuctionPersonalSoldEnd, auctionPersonalSoldEnd)
	event.RegActorEvent(custom_id.AeNirvanaLvChange, checkAuctionFollow)
	event.RegActorEvent(custom_id.AeLevelUp, checkAuctionFollow)

	engine.RegisterMessage(gshare.OfflineAddAuctionRecord, func() pb3.Message {
		return &pb3.SrvAuctionRecord{}
	}, offlineAuctionRecord)

	net.RegisterSysProtoV2(70, 51, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sSold
	})
	net.RegisterSysProtoV2(70, 54, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sBid
	})
	net.RegisterSysProtoV2(70, 55, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sHistoryRecord
	})
	net.RegisterSysProtoV2(70, 57, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sFollowItem
	})
	net.RegisterSysProtoV2(70, 58, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sAdvertise
	})
	net.RegisterSysProtoV2(70, 59, sysdef.SiAuction, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AuctionSys).c2sGoodsTakeOff
	})

	gmevent.Register("auction.take", func(player iface.IPlayer, args ...string) bool {
		for _, v := range jsondata.GetAuctionConf().SysGoods {
			if goods, err := auctionmgr.AuctionMgrInstance.GenerateGoods(&auction.SoldInfo{
				ItemId:         v.ItemId,
				Count:          random.IntervalUU(1, 100),
				Bonus:          map[uint64]uint32{player.GetId(): 0},
				RelationBossId: 22000004,
				SoldActorId:    0,
				SoldType:       auction.SoldTypeSys,
			}); err != nil {
				return false
			} else {
				err = auctionmgr.AuctionMgrInstance.PutIntoAuction(goods, time_util.NowSec())
				if err != nil {
					return false
				}
			}
			break
		}

		return true
	}, 1)
	gmevent.Register("auction.sold", func(player iface.IPlayer, args ...string) bool {
		for _, v := range jsondata.GetAuctionConf().PersonalGoods {
			if goods, err := auctionmgr.AuctionMgrInstance.GenerateGoods(&auction.SoldInfo{
				ItemId:      v.GetItemId(),
				Count:       random.IntervalUU(1, 100),
				SoldActorId: player.GetId(),
				SoldType:    auction.SoldTypePersonal,
			}); err != nil {
				return false
			} else {
				err = auctionmgr.AuctionMgrInstance.PutIntoAuction(goods, time_util.NowSec())
				if err != nil {
					return false
				}
				data := player.GetBinaryData().GetPlayerAuctionData()
				data.SoldGoodsIds = append(data.SoldGoodsIds, goods.GetId())
			}
			break
		}

		return true
	}, 1)
	gmevent.Register("auction.buyAll", func(player iface.IPlayer, args ...string) bool {
		auctionmgr.AuctionMgrInstance.AllGoodsDo(func(goods *pb3.SrvAuctionGoods) {
			auctionmgr.AuctionMgrInstance.BidGoods(player, &auction.BidInfo{
				GoodsId: goods.GetId(),
				BuyWay:  auction.AuctionGoodsBuyWayBuyItNowPrice,
				SeeBid:  goods.GetCurrentBid(),
			})
		})

		return true
	}, 1)
	gmevent.Register("auction.takeoff", func(player iface.IPlayer, args ...string) bool {
		auctionmgr.AuctionMgrInstance.AllGoodsDo(func(goods *pb3.SrvAuctionGoods) {
			auctionmgr.AuctionMgrInstance.TakeOffGoods(player, goods.GetId())
		})
		return true
	}, 1)
	gmevent.Register("auction.adv", func(player iface.IPlayer, args ...string) bool {
		auctionmgr.AuctionMgrInstance.AllGoodsDo(func(goods *pb3.SrvAuctionGoods) {
			auctionmgr.AuctionMgrInstance.AdvertiseGoods(player, goods.GetId())
		})
		return true
	}, 1)
}
