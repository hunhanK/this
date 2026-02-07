/**
 * @Author: LvYuMeng
 * @Date: 2024/10/31
 * @Desc: 超值礼包
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils/pie"
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
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/supervaluegiftmgr"
	"jjyz/gameserver/net"
)

type SuperValueGift struct {
	Base
}

func (s *SuperValueGift) getData() *pb3.SuperValueGiftData {
	binary := s.GetBinaryData()
	if binary.SuperValueGiftData == nil {
		binary.SuperValueGiftData = &pb3.SuperValueGiftData{}
	}
	if binary.SuperValueGiftData.Group == nil {
		binary.SuperValueGiftData.Group = make(map[uint32]*pb3.SuperValueGiftGroup)
	}
	return binary.SuperValueGiftData
}

func (s *SuperValueGift) OnLogin() {
	s.clearGlobalTimeOut()
}

func (s *SuperValueGift) OnReconnect() {
	s.s2cGlobalInfo()
	s.s2cInfo()
}

func (s *SuperValueGift) OnAfterLogin() {
	s.s2cGlobalInfo()
	s.clearGlobalTimeOut()
	s.s2cInfo()
}

func (s *SuperValueGift) s2cInfo() {
	s.SendProto3(69, 237, &pb3.S2C_69_237{
		Data: s.getData(),
	})
}

func (s *SuperValueGift) clearGlobalTimeOut() {
	conf, ok := jsondata.GetSuperValueGiftConf()
	if !ok {
		return
	}
	globalData := supervaluegiftmgr.GetData()
	data := s.getData()
	var delIds []uint32
	for groupId, groupConf := range conf.Groups {
		if groupConf.RefreshType != custom_id.SuperValueGiftRefreshTypeGlobal {
			continue
		}
		if _, ok := data.Group[groupId]; !ok {
			continue
		}
		gData, ok := globalData[groupId]
		if !ok {
			delIds = append(delIds, groupId)
			continue
		}

		groupData := s.getGroupData(groupId)
		if groupData.ResetTime > 0 && groupData.ResetTime != gData.NextRefreshTime {
			delIds = append(delIds, groupId)
			continue
		}
	}

	for _, delId := range delIds {
		delete(data.Group, delId)
	}
}

func (s *SuperValueGift) s2cGlobalInfo() {
	s.SendProto3(69, 239, &pb3.S2C_69_239{
		Data: supervaluegiftmgr.GetData(),
	})
}

func (s *SuperValueGift) OnOpen() {
	s.initData()
	s.s2cGlobalInfo()
	s.s2cInfo()
}

func (s *SuperValueGift) initData() {
	data := s.getData()
	data.SysOpenTime = time_util.NowSec()
}

const (
	superValueGiftBuyTypeDaily   = 1
	superValueGiftBuyTypeWeek    = 2
	superValueGiftBuyTypePersist = 3
)

var superValueGiftBuyTypeList = []uint32{
	superValueGiftBuyTypeDaily,
	superValueGiftBuyTypeWeek,
	superValueGiftBuyTypePersist,
}

func (s *SuperValueGift) isGiftBuyTimesEnough(buyType, times uint32, giftConf *jsondata.SuperValueGiftConf) bool {
	switch buyType {
	case superValueGiftBuyTypeDaily:
		return giftConf.DailyLimit == 0 || times < giftConf.DailyLimit
	case superValueGiftBuyTypeWeek:
		return giftConf.WeeklyLimit == 0 || times < giftConf.WeeklyLimit
	case superValueGiftBuyTypePersist:
		return giftConf.PersistLimit == 0 || times < giftConf.PersistLimit
	}
	return false
}

func (s *SuperValueGift) canBuy(groupConf *jsondata.SuperValueGroupConf, giftConf *jsondata.SuperValueGiftConf) (bool, error) {
	groupId, giftId := groupConf.GroupId, giftConf.GiftID
	giftData := s.getGiftData(groupId, giftId)

	if groupConf.RefreshType == custom_id.SuperValueGiftRefreshTypeGlobal {
		gData := supervaluegiftmgr.GetData()
		gGroup, ok := gData[groupId]
		if !ok {
			return false, neterror.InternalError("global group %d gift not gen", groupId)
		}
		if !pie.Uint32s(gGroup.GiftId).Contains(giftId) {
			return false, neterror.ParamsInvalidError("group %d giftId %d not sold in global", groupId, giftId)
		}
		s.getGroupData(groupId).ResetTime = gGroup.NextRefreshTime
	}

	for _, buyType := range superValueGiftBuyTypeList {
		if !s.isGiftBuyTimesEnough(buyType, giftData.BuyTimes[buyType], giftConf) {
			s.owner.SendTipMsg(tipmsgid.TpBuyTimesLimit)
			return false, nil
		}
	}

	return true, nil
}

func (s *SuperValueGift) buy(groupConf *jsondata.SuperValueGroupConf, giftConf *jsondata.SuperValueGiftConf) error {
	groupId, giftId := groupConf.GroupId, giftConf.GiftID

	giftData := s.getGiftData(groupId, giftId)

	if !s.owner.ConsumeByConf(giftConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogSuperValueGiftConsume,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	for _, buyType := range superValueGiftBuyTypeList {
		giftData.BuyTimes[buyType]++
	}

	engine.GiveRewards(s.owner, giftConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSuperValueGiftAwards})

	s.SendProto3(69, 238, &pb3.S2C_69_238{
		GroupId: groupId,
		GiftId:  giftId,
		Count:   giftData.BuyTimes,
	})

	if giftConf.Broadcast {
		engine.BroadcastTipMsgById(tipmsgid.SuperValueGiftTip, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, giftConf.Rewards))
	}

	return nil
}

func (s *SuperValueGift) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_69_238
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetSuperValueGiftConf()
	if !ok {
		return neterror.ConfNotFoundError("conf is nil")
	}
	groupConf, ok := conf.Groups[req.GetGroupId()]
	if !ok {
		return neterror.ConfNotFoundError("group conf %d is nil", req.GetGroupId())
	}
	giftConf, ok := groupConf.Gifts[req.GetGiftId()]
	if !ok {
		return neterror.ConfNotFoundError("gift conf %d is nil", req.GetGiftId())
	}

	s.clearGlobalTimeOut()

	if can, err := s.canBuy(groupConf, giftConf); !can {
		return err
	}

	err := s.buy(groupConf, giftConf)
	if err != nil {
		return err
	}

	return nil
}

func (s *SuperValueGift) getGroupData(groupId uint32) *pb3.SuperValueGiftGroup {
	data := s.getData()
	if _, ok := data.Group[groupId]; !ok {
		data.Group[groupId] = &pb3.SuperValueGiftGroup{}
	}
	if nil == data.Group[groupId].GiftSt {
		data.Group[groupId].GiftSt = make(map[uint32]*pb3.SuperValueGiftSt)
	}
	return data.Group[groupId]
}

func (s *SuperValueGift) getGiftData(groupId, giftId uint32) *pb3.SuperValueGiftSt {
	groupData := s.getGroupData(groupId)
	if _, ok := groupData.GiftSt[giftId]; !ok {
		groupData.GiftSt[giftId] = &pb3.SuperValueGiftSt{}
	}
	if nil == groupData.GiftSt[giftId].BuyTimes {
		groupData.GiftSt[giftId].BuyTimes = make(map[uint32]uint32)
	}
	return groupData.GiftSt[giftId]
}

func (s *SuperValueGift) delBuyType(buyType uint32) {
	conf, ok := jsondata.GetSuperValueGiftConf()
	if !ok {
		return
	}
	data := s.getData()
	for groupId := range conf.Groups {
		if _, ok := data.Group[groupId]; !ok {
			continue
		}
		groupData := s.getGroupData(groupId)
		for _, giftSt := range groupData.GiftSt {
			delete(giftSt.BuyTimes, buyType)
		}
	}
	s.s2cInfo()
}

func (s *SuperValueGift) onNewDay() {
	s.delBuyType(superValueGiftBuyTypeDaily)
}

func (s *SuperValueGift) onNewWeek() {
	s.delBuyType(superValueGiftBuyTypeWeek)
}

func onSuperValueGiftNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSuperValueGift).(*SuperValueGift)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.onNewDay()
}

func onSuperValueGiftNewWeek(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSuperValueGift).(*SuperValueGift)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.onNewWeek()
}

func onSuperValueGiftRefresh(args ...interface{}) {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		if sys, ok := player.GetSysObj(sysdef.SiSuperValueGift).(*SuperValueGift); ok {
			sys.clearGlobalTimeOut()
			sys.s2cInfo()
		}
	})
}

func init() {
	RegisterSysClass(sysdef.SiSuperValueGift, func() iface.ISystem {
		return &SuperValueGift{}
	})

	event.RegActorEvent(custom_id.AeNewDay, onSuperValueGiftNewDay)
	event.RegActorEvent(custom_id.AeNewWeek, onSuperValueGiftNewWeek)

	event.RegSysEvent(custom_id.SeSuperValueGiftRefresh, onSuperValueGiftRefresh)

	net.RegisterSysProtoV2(69, 238, sysdef.SiSuperValueGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SuperValueGift).c2sBuy
	})
}
