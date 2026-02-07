/**
 * @Author: beiming
 * @Date: 2023/12/11
 * @Desc: 结婚仓库
**/
package actorsystem

import (
	"errors"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sync"

	"github.com/gzjjyz/srvlib/utils"
)

func init() {
	RegisterSysClass(sysdef.SiMarryDepot, func() iface.ISystem {
		return &MarryDepot{
			cm: newContainerManager(),
		}
	})
	net.RegisterSysProtoV2(53, 45, sysdef.SiMarryDepot, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryDepot).c2sDepot
	})
	net.RegisterSysProtoV2(53, 42, sysdef.SiMarryDepot, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryDepot).c2sDonate
	})
	net.RegisterSysProtoV2(53, 43, sysdef.SiMarryDepot, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarryDepot).c2sRemove
	})

	event.RegActorEvent(custom_id.AeBeforeDivorce, marryDepotOnDivorce)
	event.RegSysEvent(custom_id.SeGiveMarryReward, marryDepotOnGiveMarryReward)
}

type MarryDepot struct {
	Base
	cm    *containerManager
	depot *miscitem.Container
}

func (s *MarryDepot) OnOpen() {
	s.startUpDepot()
	s.s2cCanUseDepot()
}

func (s *MarryDepot) OnAfterLogin() {
	s.startUpDepot()
	s.s2cCanUseDepot()
}

func (s *MarryDepot) OnReconnect() {
	s.startUpDepot()
	s.s2cCanUseDepot()
}

func (s *MarryDepot) s2cCanUseDepot() {
	s.SendProto3(53, 46, &pb3.S2C_53_46{CanUseMarryDepot: s.canUseDepot()})
}

// c2sDepot 客户端主动获取结婚仓库数据
func (s *MarryDepot) c2sDepot(_ *base.Message) error {
	if !s.canUseDepot() { // 不能使用结婚仓库
		return neterror.ParamsInvalidError("can not use marry depot")
	}

	s.SendProto3(53, 45, &pb3.S2C_53_45{Depot: s.getDepot()})
	return nil
}

func (s *MarryDepot) getDepot() *pb3.MarryDepot {
	mds := gshare.GetStaticVar().MarryDepots
	key := friendmgr.MarryDepotGlobalVarKey(s.GetOwner().GetId(), s.marryId())
	return mds[key]
}

// c2sDonate 结婚仓库捐献物品
func (s *MarryDepot) c2sDonate(msg *base.Message) error {
	if !s.canUseDepot() { // 不能使用结婚仓库
		return neterror.ParamsInvalidError("can not use marry depot")
	}
	var req pb3.C2S_53_42
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("C2S_53_42 UnPackPb3Msg err: %w", err)
	}

	if int(s.depot.AvailableCount()) < len(req.GetItems()) {
		return neterror.ParamsInvalidError("depot not enough")
	}

	for _, reqItem := range req.GetItems() {
		err := s.donateItem(reqItem)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *MarryDepot) donateItem(reqItem *pb3.ItemHandleCount) error {
	item := s.GetOwner().GetItemByHandle(reqItem.Handle)
	if item == nil || item.GetBind() {
		return neterror.ParamsInvalidError("item not exist or bind")
	}

	itemId := item.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return neterror.ParamsInvalidError("item conf not exist")
	}

	if !base.CheckItemFlag(itemId, itemdef.CanDonateToMarryDepot) {
		return neterror.ParamsInvalidError("item can not donate")
	}

	stConf := itemConf.GetItemSettingConf()
	if stConf.MarryDepot == nil {
		return neterror.ParamsInvalidError("item can not donate")
	}

	owner := s.GetOwner()

	if itemdef.CannotOverlap(itemConf.Type) {
		if !owner.RemoveItemByHandle(item.GetHandle(), pb3.LogId_LogMarryDepotDonate) {
			return neterror.ParamsInvalidError("item can not donate, remove item fail")
		}

		if item.Ext == nil {
			item.Ext = &pb3.ItemExt{}
		}
		item.Ext.OwnerId = owner.GetId()

		if !s.depot.AddItemPtr(item, false, pb3.LogId_LogMarryDepotDonate) {
			return neterror.ParamsInvalidError("item can not donate, add item fail")
		}
	} else {
		// 对于可以叠加的物品
		//  结婚仓库中需要按照 handle 的归属进行叠加, 不同归属的物品不能叠加
		if !owner.DeleteItemPtr(item, int64(reqItem.GetCount()), pb3.LogId_LogMarryDepotDonate) {
			return neterror.ParamsInvalidError("item can not donate, remove item fail")
		}

		nItem := itemdef.ItemParamSt{
			ItemId:  itemId,
			Count:   int64(reqItem.GetCount()),
			LogId:   pb3.LogId_LogMarryDepotDonate,
			OwnerId: owner.GetId(),
		}
		if !s.depot.AddItem(&nItem) {
			return neterror.ParamsInvalidError("item can not donate, add item fail")
		}
	}

	if stConf.MarryDepot.DonateScore > 0 {
		score := stConf.MarryDepot.DonateScore * reqItem.GetCount()
		if !owner.AddMoney(moneydef.MarryDepotScore, int64(score), true, pb3.LogId_LogMarryDepotDonate) {
			return neterror.ParamsInvalidError("item can not donate, add money fail")
		}
	}

	s.AddRecord(&pb3.MarryDepotRecord{
		Type:      RecordDonate,
		Time:      time_util.NowSec(),
		Name:      owner.GetName(),
		ItemId:    itemId,
		ItemCount: reqItem.GetCount(),
	})

	return nil
}

// c2sRemove 结婚仓库兑换物品
func (s *MarryDepot) c2sRemove(msg *base.Message) error {
	if !s.canUseDepot() { // 不能使用结婚仓库
		return neterror.ParamsInvalidError("can not use marry depot")
	}

	var req pb3.C2S_53_43
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("C2S_53_43 UnPackPb3Msg err: %w", err)
	}

	if req.GetItem() == nil {
		return neterror.ParamsInvalidError("item not exist")
	}

	item := s.depot.FindItemByHandle(req.GetItem().Handle)
	if item == nil {
		return neterror.ParamsInvalidError("item not exist")
	}

	itemId := item.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return neterror.ParamsInvalidError("item conf not exist")
	}

	stConf := itemConf.GetItemSettingConf()
	if stConf.MarryDepot == nil {
		return neterror.ParamsInvalidError("item can not remove")
	}

	owner := s.GetOwner()
	if stConf.MarryDepot.RemoveScore > 0 {
		score := stConf.MarryDepot.RemoveScore * req.GetItem().Count
		if !owner.DeductMoney(moneydef.MarryDepotScore, int64(score), common.ConsumeParams{LogId: pb3.LogId_LogMarryDepotRemove}) {
			return neterror.ParamsInvalidError("item can not donate, deduct money fail")
		}
	}

	if itemdef.CannotOverlap(itemConf.Type) {
		if !s.depot.RemoveItemByHandle(item.GetHandle(), pb3.LogId_LogMarryDepotRemove) {
			return neterror.ParamsInvalidError("remove item fail")
		}

		if item.Ext != nil {
			item.Ext.OwnerId = 0
		}

		if !owner.AddItemPtr(item, false, pb3.LogId_LogMarryDepotRemove) {
			return neterror.ParamsInvalidError("add item fail")
		}
	} else {
		if !s.depot.DeleteItemPtr(item, int64(req.GetItem().GetCount()), pb3.LogId_LogMarryDepotRemove) {
			return neterror.ParamsInvalidError("remove item fail")
		}

		nItem := itemdef.ItemParamSt{
			ItemId: itemId,
			Count:  int64(req.GetItem().GetCount()),
			LogId:  pb3.LogId_LogMarryDepotRemove,
		}
		if !owner.AddItem(&nItem) {
			return neterror.ParamsInvalidError("add item fail")
		}
	}

	s.AddRecord(&pb3.MarryDepotRecord{
		Type:      RecordRemove,
		Time:      time_util.NowSec(),
		Name:      owner.GetName(),
		ItemId:    itemId,
		ItemCount: req.GetItem().Count,
	})

	return nil
}

func (s *MarryDepot) startUpDepot() {
	if !s.canUseDepot() {
		if err := s.clearMoney(s.GetOwner()); err != nil {
			s.LogWarn("marryDepotStartUp clearMoney err: %v", err)
		}
		return
	}

	depot := s.getDepot()

	key := friendmgr.MarryDepotGlobalVarKey(s.GetOwner().GetId(), s.marryId())
	container, exist := s.cm.GetContainer(key)
	if !exist {
		c := miscitem.NewContainer(&depot.Items)
		s.cm.AddContainer(key, c)

		container = c
	}

	container.DefaultSizeHandle = s.DefaultSize
	container.OnAddNewItem = s.OnAddNewItem
	container.OnItemChange = s.OnItemChange
	container.OnRemoveItem = s.OnRemoveItem
	container.NeedSameOwner(true)

	s.depot = container
}

func (s *MarryDepot) canUseDepot() bool {
	if !s.IsOpen() {
		return false
	}

	if s.marryId() == 0 { // 没有结婚对象
		return false
	}

	mds := gshare.GetStaticVar().MarryDepots
	if mds == nil {
		return false
	}

	return s.getDepot() != nil
}

func (s *MarryDepot) DefaultSize() uint32 {
	return jsondata.GetMarryConf().DepotLimit
}

func (s *MarryDepot) OnAddNewItem(item *pb3.ItemSt, bTip bool, logId pb3.LogId) {
	s.SendBothProto3(53, 40, &pb3.S2C_53_40{Items: []*pb3.ItemSt{item}})
	s.changeStat(item, int32(item.Count), false, logId)
}

func (s *MarryDepot) OnItemChange(item *pb3.ItemSt, add int64, param common.EngineGiveRewardParam) {
	s.SendBothProto3(53, 40, &pb3.S2C_53_40{Items: []*pb3.ItemSt{item}})
	s.changeStat(item, int32(add), false, param.LogId)
}

func (s *MarryDepot) OnRemoveItem(item *pb3.ItemSt, logId pb3.LogId) {
	s.SendBothProto3(53, 41, &pb3.S2C_53_41{Handle: item.GetHandle()})
	s.changeStat(item, int32(item.Count), true, logId)
}

func (s *MarryDepot) changeStat(item *pb3.ItemSt, add int32, remove bool, logId pb3.LogId) {
	logworker.LogItem(s.owner, &pb3.LogItem{
		ItemId: item.GetItemId(),
		Value:  int64(add),
		LogId:  uint32(logId),
	})
}

// SendBothProto3 同时发送给自己和结婚对象
func (s *MarryDepot) SendBothProto3(sysId, cmdId uint16, message pb3.Message) {
	s.GetOwner().SendProto3(sysId, cmdId, message)

	marryId := s.marryId()
	partner := manager.GetPlayerPtrById(marryId)
	if partner != nil {
		partner.SendProto3(sysId, cmdId, message)
	}
}

const (
	RecordDonate = 1
	RecordRemove = 2
)

func (s *MarryDepot) AddRecord(record *pb3.MarryDepotRecord) {
	const maxLen = 80

	depot := s.getDepot()

	m := len(depot.Records)
	n := m + 1

	if depot.Records == nil {
		depot.Records = make([]*pb3.MarryDepotRecord, maxLen)
	}

	if n > cap(depot.Records) {
		nr := make([]*pb3.MarryDepotRecord, (n+1)*2)
		copy(nr, depot.Records)
		depot.Records = nr
	}

	depot.Records = depot.Records[0:n]
	depot.Records[m] = record

	if len(depot.Records) > maxLen {
		depot.Records = depot.Records[1:]
	}

	s.SendBothProto3(53, 44, &pb3.S2C_53_44{Record: record})
}

func (s *MarryDepot) marryData() *pb3.MarryData {
	return s.GetBinaryData().MarryData
}

func (s *MarryDepot) marryId() uint64 {
	data := s.marryData()
	if data == nil {
		return 0
	}

	if fd, ok := friendmgr.GetFriendCommonDataById(data.CommonId); ok {
		return utils.Ternary(fd.ActorId1 != s.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
	}

	return 0
}

func marryDepotOnDivorce(actor iface.IPlayer, args ...interface{}) {
	s := actor.GetSysObj(sysdef.SiMarryDepot).(*MarryDepot)

	itemMap := s.divisionItems()

	for actorId, items := range itemMap {
		if len(items) == 0 {
			continue
		}

		rewards := make([]*jsondata.StdReward, 0, len(items))
		for _, item := range items {
			rewards = append(rewards, &jsondata.StdReward{
				Id:    item.GetItemId(),
				Count: item.GetCount(),
				Bind:  item.GetBind(),
			})
		}

		mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
			ConfId:  common.Mail_MarryDepotReturn,
			Rewards: rewards,
		})
	}

	if err := s.clearDepot(); err != nil {
		s.LogWarn("marryDepotOnDivorce clearDepot err: %v", err)
	}

	s.SendBothProto3(53, 46, &pb3.S2C_53_46{CanUseMarryDepot: false})
}

// divisionItems 按归属分仓库中的物品
func (s *MarryDepot) divisionItems() map[uint64][]*pb3.ItemSt {
	items := make(map[uint64][]*pb3.ItemSt)
	if nil != s.depot {
		for _, item := range s.depot.GetAllItemMap() {
			items[item.Ext.OwnerId] = append(items[item.Ext.OwnerId], item)
		}
	}
	return items
}

func (s *MarryDepot) clearDepot() error {
	// // 移除仓库中所有的道具
	for handle := range s.depot.GetAllItemMap() {
		s.depot.RemoveItemByHandle(handle, pb3.LogId_LogMarryDepotRemove)
	}

	s.depot.Clear()

	owner := s.GetOwner()
	if err := s.clearMoney(owner); err != nil {
		return err
	}

	marryId := s.marryId()
	partner := manager.GetPlayerPtrById(marryId)
	if partner != nil {
		if err := s.clearMoney(partner); err != nil {
			return err
		}
	} else {
		// 结婚对象不在线, 在下次上线时清理
	}

	key := friendmgr.MarryDepotGlobalVarKey(owner.GetId(), marryId)
	delete(gshare.GetStaticVar().MarryDepots, key)
	delete(s.cm.containers, key)

	return nil
}

func (s *MarryDepot) clearMoney(owner iface.IPlayer) error {
	score := owner.GetMoneyCount(moneydef.MarryDepotScore)
	if score > 0 {
		if !owner.DeductMoney(moneydef.MarryDepotScore, int64(score), common.ConsumeParams{LogId: pb3.LogId_LogMarryDepotRemove}) {
			return errors.New("clear money fail")
		}
	}

	return nil
}

func marryDepotOnGiveMarryReward(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	commonSt, ok := args[0].(*pb3.CommonSt)
	if !ok {
		return
	}

	var (
		actorId1 = commonSt.GetU64Param()
		actorId2 = commonSt.GetU64Param2()
		confId   = commonSt.GetU32Param()
	)

	cfg, ok := jsondata.GetMarryConf().Grade[confId]
	if !(ok && cfg.OpenDepot) {
		return
	}

	if gshare.GetStaticVar().MarryDepots == nil {
		gshare.GetStaticVar().MarryDepots = make(map[string]*pb3.MarryDepot)
	}
	mds := gshare.GetStaticVar().MarryDepots
	key := friendmgr.MarryDepotGlobalVarKey(actorId1, actorId2)

	if _, ok := mds[key]; !ok {
		mds[key] = &pb3.MarryDepot{}
	}

	marryDepotStartUpAndSend(actorId1)
	marryDepotStartUpAndSend(actorId2)
}

func marryDepotStartUpAndSend(actorId uint64) {
	actor := manager.GetPlayerPtrById(actorId)
	if actor != nil {
		s := actor.GetSysObj(sysdef.SiMarryDepot).(*MarryDepot)
		s.startUpDepot()
		s.s2cCanUseDepot()
	}
}

type containerManager struct {
	containers map[string]*miscitem.Container
}

var onceContainerManager sync.Once
var globalContainerManager *containerManager

func newContainerManager() *containerManager {
	onceContainerManager.Do(func() {
		globalContainerManager = &containerManager{
			containers: make(map[string]*miscitem.Container),
		}
	})

	return globalContainerManager
}

func (cm *containerManager) GetContainer(key string) (*miscitem.Container, bool) {
	c, exist := cm.containers[key]
	return c, exist
}

func (cm *containerManager) AddContainer(key string, container *miscitem.Container) {
	cm.containers[key] = container
}
