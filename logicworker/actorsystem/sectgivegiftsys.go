/**
 * @Author: zjj
 * @Date: 2024/12/24
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/page"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/sectgivegiftmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SectGiveGiftSys struct {
	Base
}

const (
	SectGiveGiftTypeMonsterId   = 1 // 击杀怪物
	SectGiveGiftTypeAnyAmount   = 2 // 充值任意金额
	SectGiveGiftTypeTotalAmount = 3 // 今日累充
	SectGiveGiftTypeBuySpecPack = 4 // 购买指定礼包
	SectGiveGiftTypeChargeId    = 5 // 充值指定充值ID
)

func (s *SectGiveGiftSys) s2cInfo() {
	s.SendProto3(43, 10, &pb3.S2C_43_10{
		Data: s.getData().BaseData,
	})
}

func (s *SectGiveGiftSys) s2cChest() {
	s.SendProto3(43, 16, &pb3.S2C_43_16{
		Data: sectgivegiftmgr.GetSectGiveGiftChest(),
	})
}

func (s *SectGiveGiftSys) getData() *pb3.SectGiveGiftData {
	data := s.GetBinaryData().SectGiveGiftData
	if data == nil {
		s.GetBinaryData().SectGiveGiftData = &pb3.SectGiveGiftData{}
		data = s.GetBinaryData().SectGiveGiftData
	}
	if data.BaseData == nil {
		data.BaseData = &pb3.SectGiveGiftBaseData{}
	}
	if data.BaseData.TabDailyCount == nil {
		data.BaseData.TabDailyCount = make(map[uint32]uint32)
	}
	return data
}

func (s *SectGiveGiftSys) checkExpiredHdl() {
	data := s.getData()
	set := sectgivegiftmgr.GetSectGiveGiftHdlSet()
	data.RecHdlList = pie.Uint64s(data.RecHdlList).Filter(func(u uint64) bool {
		if _, ok := set[u]; ok {
			return true
		}
		return false
	})
}

func (s *SectGiveGiftSys) OnReconnect() {
	s.checkExpiredHdl()
	s.s2cInfo()
	s.s2cChest()
}

func (s *SectGiveGiftSys) OnLogin() {
	s.checkExpiredHdl()
	s.s2cInfo()
	s.s2cChest()
}

func (s *SectGiveGiftSys) OnOpen() {
	s.s2cInfo()
	s.s2cChest()
}

func (s *SectGiveGiftSys) c2sRecSelfGift(msg *base.Message) error {
	var req pb3.C2S_43_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	tab := req.Tab
	data := s.getData()

	commonConf := jsondata.GetSectGiveGiftCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("sect give gift common conf not found")
	}

	if tab == 0 || uint32(len(commonConf.DailyLimits)) < tab {
		return neterror.ConfNotFoundError("sect give gift common conf tab:%d is error", tab)
	}

	recHdlList := pie.Uint64s(req.HdlList)
	recCount := uint32(recHdlList.Len())
	dailyCount := data.BaseData.TabDailyCount[tab]
	totalCount := dailyCount + recCount
	if totalCount > commonConf.DailyLimits[tab-1] {
		return neterror.ConfNotFoundError("daily limit %d %d %d", dailyCount, recCount, commonConf.DailyLimits[tab-1])
	}

	tabGiftData := sectgivegiftmgr.GetTabGiftData(tab)
	var set = make(map[uint64]struct{})
	for _, hdl := range recHdlList {
		set[hdl] = struct{}{}
	}

	for _, hdl := range data.RecHdlList {
		if _, ok := set[hdl]; !ok {
			continue
		}
		return neterror.ParamsInvalidError("%d %d already rec", tab, hdl)
	}

	owner := s.GetOwner()
	size := uint32(len(set))
	list, total := page.FindPaginateWithCond(tabGiftData.ItemList, size, 0, func(item *pb3.SectGiveGiftItem) bool {
		if _, ok := set[item.Hdl]; ok {
			return true
		}
		return false
	})
	if total != size {
		return neterror.ParamsInvalidError("not found %d rec list", tab)
	}

	var totalAwards jsondata.StdRewardVec
	for _, item := range list {
		giftConf := jsondata.GetSectGiveGiftConf(item.GiftId)
		if giftConf == nil {
			return neterror.ConfNotFoundError("sect give gift conf not found %d", item.GiftId)
		}
		totalAwards = append(totalAwards, giftConf.Awards...)
	}

	data.RecHdlList = append(data.RecHdlList, recHdlList...)
	data.BaseData.TabDailyCount[tab] = totalCount
	engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectGiveGiftRecSelfAwards})
	s.SendProto3(43, 11, &pb3.S2C_43_11{
		Tab:     tab,
		Count:   totalCount,
		HdlList: recHdlList,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectGiveGiftRecSelfAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(tab),
		StrArgs: recHdlList.JSONString(),
	})
	return nil
}

func (s *SectGiveGiftSys) c2sExchangeChest(msg *base.Message) error {
	var req pb3.C2S_43_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	count := req.Count
	chest := sectgivegiftmgr.GetSectGiveGiftChest()
	chestConf := jsondata.GetSectGiveGiftChestConf(chest.Lv)
	if chestConf == nil {
		return neterror.ParamsInvalidError("chest conf not found %d", chest.Lv)
	}

	consumeVec := jsondata.ConsumeMulti(chestConf.Consume, count)
	owner := s.GetOwner()

	// 消耗失败
	if len(consumeVec) == 0 || !owner.ConsumeByConf(consumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogSectGiveGiftRecChestAwards}) {
		return neterror.ConsumeFailedError("consume failed %d %d", chest.Lv, count)
	}

	rewardVec := jsondata.StdRewardMulti(chestConf.Awards, int64(count))
	engine.GiveRewards(owner, rewardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectGiveGiftRecChestAwards})

	s.SendProto3(43, 12, &pb3.S2C_43_12{
		Count: count,
		Lv:    chest.Lv,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectGiveGiftRecChestAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(chest.Lv),
		StrArgs: fmt.Sprintf("%d", count),
	})
	return nil
}

func (s *SectGiveGiftSys) c2sGetList(msg *base.Message) error {
	var req pb3.C2S_43_13
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	commonConf := jsondata.GetSectGiveGiftCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("sect give gift common conf not found")
	}

	offset := req.Offset
	limit := req.Limit

	// 不能超过配制的单页显示数量
	if limit == 0 || limit > commonConf.PageLimit {
		limit = commonConf.PageLimit
	}

	tab := req.Tab
	giftData := sectgivegiftmgr.GetTabGiftData(tab)
	recHdlList := s.getData().RecHdlList
	var recHdlSet = make(map[uint64]struct{})
	for _, v := range recHdlList {
		recHdlSet[v] = struct{}{}
	}

	var resp pb3.S2C_43_13
	resp.List, resp.Total = page.FindPaginateWithCond(giftData.ItemList, limit, offset, func(item *pb3.SectGiveGiftItem) bool {
		if _, ok := recHdlSet[item.Hdl]; ok {
			return false
		}
		return true
	})
	resp.Limit = limit
	resp.Offset = offset
	resp.Tab = tab
	s.SendProto3(43, 13, &resp)
	return nil
}

func (s *SectGiveGiftSys) AddSectGiveGift(typ uint32, params []uint32) {
	owner := s.GetOwner()
	idListByType := jsondata.GetSectGiveGiftIdListByType(typ)
	if len(idListByType) == 0 {
		owner.LogWarn("type:%d not found conf", typ)
		return
	}

	var addGift = func(giftConf *jsondata.SectGiveGiftConf) {
		hdl, _ := series.AllocSeries()
		nowSec := time_util.NowSec()
		var item = &pb3.SectGiveGiftItem{
			Hdl:       hdl,
			ActorId:   owner.GetId(),
			GiftId:    giftConf.Id,
			ExpiredAt: giftConf.LiveTime + nowSec,
			ActorName: owner.GetName(),
		}

		sectgivegiftmgr.AddSectGiveGift(giftConf.Tab, item)
		sectgivegiftmgr.AddChestExp(giftConf.ChestExp)

		engine.Broadcast(chatdef.CIWorld, 0, 43, 14, &pb3.S2C_43_14{
			Item: item,
		}, 0)

		engine.Broadcast(chatdef.CIWorld, 0, 43, 16, &pb3.S2C_43_16{
			Data: sectgivegiftmgr.GetSectGiveGiftChest(),
		}, 0)

		if giftConf.TipsId > 0 {
			engine.BroadcastTipMsgById(giftConf.TipsId, owner.GetId(), owner.GetName())
		}

		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectGiveGiftAddGift, &pb3.LogPlayerCounter{
			NumArgs: uint64(giftConf.Id),
			StrArgs: fmt.Sprintf("%d_%v", item.ExpiredAt, params),
		})
	}

	switch typ {
	case SectGiveGiftTypeMonsterId: // 击杀怪物
		if len(params) < 1 {
			owner.LogWarn("type:%d params:%v err", typ, params)
			return
		}
		// 如果遍历次数过多 影响单次消息的处理 用内存换时间 把需要枚举的怪物ID 提前缓存
		monsterId := params[0]
		for _, id := range idListByType {
			giveGiftConf := jsondata.GetSectGiveGiftConf(id)
			if giveGiftConf == nil {
				owner.LogWarn("type:%d not found %d conf", typ, id)
				return
			}
			if !pie.Uint32s(giveGiftConf.Params).Contains(monsterId) {
				continue
			}
			addGift(giveGiftConf)
			break
		}
	case SectGiveGiftTypeAnyAmount: // 充值任意金额
		id := idListByType[0]
		giveGiftConf := jsondata.GetSectGiveGiftConf(id)
		if giveGiftConf == nil {
			owner.LogWarn("type:%d not found %d conf", typ, id)
			return
		}
		addGift(giveGiftConf)
	case SectGiveGiftTypeTotalAmount: // 今日累充
		if len(params) < 2 {
			owner.LogWarn("type:%d params:%v err", typ, params)
			return
		}

		lastChargeAmount, cashCent := params[0], params[1]
		curChargeAmount := lastChargeAmount + cashCent
		for _, id := range idListByType {
			giveGiftConf := jsondata.GetSectGiveGiftConf(id)
			if giveGiftConf == nil {
				owner.LogWarn("type:%d not found %d conf", typ, id)
				return
			}

			// 过滤已经领过的充值奖励
			if lastChargeAmount != 0 && lastChargeAmount >= giveGiftConf.Params[0] {
				continue
			}

			// 此次可以领的充值奖励
			if curChargeAmount >= giveGiftConf.Params[0] {
				addGift(giveGiftConf)
			}
		}
	case SectGiveGiftTypeBuySpecPack: // 购买指定礼包
	case SectGiveGiftTypeChargeId: // 充值指定充值ID
		if len(params) < 1 {
			owner.LogWarn("type:%d params:%v err", typ, params)
			return
		}
		chargeId := params[0]
		for _, id := range idListByType {
			giveGiftConf := jsondata.GetSectGiveGiftConf(id)
			if giveGiftConf == nil {
				owner.LogWarn("type:%d not found %d conf", typ, id)
				return
			}
			if !pie.Uint32s(giveGiftConf.Params).Contains(chargeId) {
				continue
			}
			addGift(giveGiftConf)
			break
		}
	}
}

func getSectGiveGiftSys(player iface.IPlayer) *SectGiveGiftSys {
	obj := player.GetSysObj(sysdef.SiSectGiveGift)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys := obj.(*SectGiveGiftSys)
	return sys
}

func handleAeSectGiveGiftExpired(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	set := args[0].(map[uint64]struct{})
	if len(set) == 0 {
		return
	}

	sys := getSectGiveGiftSys(player)
	if sys == nil {
		return
	}
	data := sys.getData()
	data.RecHdlList = pie.Uint64s(data.RecHdlList).Filter(func(u uint64) bool {
		if _, ok := set[u]; ok {
			return false
		}
		return true
	})
	var delHdlList []uint64
	for hdl := range set {
		delHdlList = append(delHdlList, hdl)
	}
	player.SendProto3(43, 15, &pb3.S2C_43_15{
		HdlList: delHdlList,
	})
}

func handleSectGiveGiftAeKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}

	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}
	sys := getSectGiveGiftSys(player)
	if sys == nil {
		return
	}
	for i := uint32(0); i < count; i++ {
		sys.AddSectGiveGift(SectGiveGiftTypeMonsterId, []uint32{monId})
	}
}

func handleSectGiveGiftAeNewDay(player iface.IPlayer, args ...interface{}) {
	sys := getSectGiveGiftSys(player)
	if sys == nil {
		return
	}
	sys.getData().BaseData.TabDailyCount = make(map[uint32]uint32)
	sys.getData().DailyLastChargeAmount = 0
}

func handleSectGiveGiftAeCharge(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok {
		return
	}
	sys := getSectGiveGiftSys(player)
	if sys == nil {
		return
	}
	lastChargeAmount := sys.getData().DailyLastChargeAmount
	if lastChargeAmount == 0 {
		sys.AddSectGiveGift(SectGiveGiftTypeAnyAmount, []uint32{chargeEvent.CashCent})
	}
	sys.getData().DailyLastChargeAmount = lastChargeAmount + chargeEvent.CashCent
	sys.AddSectGiveGift(SectGiveGiftTypeTotalAmount, []uint32{lastChargeAmount, chargeEvent.CashCent})
	sys.AddSectGiveGift(SectGiveGiftTypeChargeId, []uint32{chargeEvent.ChargeId})
}

func init() {
	RegisterSysClass(sysdef.SiSectGiveGift, func() iface.ISystem {
		return &SectGiveGiftSys{}
	})
	net.RegisterSysProtoV2(43, 11, sysdef.SiSectGiveGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectGiveGiftSys).c2sRecSelfGift
	})
	net.RegisterSysProtoV2(43, 12, sysdef.SiSectGiveGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectGiveGiftSys).c2sExchangeChest
	})
	net.RegisterSysProtoV2(43, 13, sysdef.SiSectGiveGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectGiveGiftSys).c2sGetList
	})
	event.RegActorEvent(custom_id.AeSectGiveGiftExpired, handleAeSectGiveGiftExpired)
	event.RegActorEvent(custom_id.AeKillMon, handleSectGiveGiftAeKillMon)
	event.RegActorEvent(custom_id.AeNewDay, handleSectGiveGiftAeNewDay)
	event.RegActorEvent(custom_id.AeCharge, handleSectGiveGiftAeCharge)

	gmevent.Register("SectGiveGiftSys.AddSectGiveGift", func(player iface.IPlayer, args ...string) bool {
		sys := getSectGiveGiftSys(player)
		if sys == nil {
			return false
		}
		if len(args) < 3 {
			return false
		}
		typ := utils.AtoUint32(args[0])
		num := utils.Atoi(args[1])
		var params []uint32
		for _, arg := range args[2:] {
			params = append(params, utils.AtoUint32(arg))
		}
		for i := 0; i < num; i++ {
			sys.AddSectGiveGift(typ, params)
		}
		return true
	}, 1)
	gmevent.Register("SectGiveGiftSys.AddExp", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sectgivegiftmgr.AddChestExp(utils.AtoInt64(args[0]))
		engine.Broadcast(chatdef.CIWorld, 0, 43, 16, &pb3.S2C_43_16{
			Data: sectgivegiftmgr.GetSectGiveGiftChest(),
		}, 0)
		return true
	}, 1)
}
