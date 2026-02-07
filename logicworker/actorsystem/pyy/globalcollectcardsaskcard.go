/**
 * @Author: zjj
 * @Date: 2024/8/5
 * @Desc: 全民集卡-求助模块
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"sort"
)

func (s *GlobalCollectCardsSys) clearCardData() {
	staticVar := gshare.GetStaticVar()
	if staticVar.PyyActData == nil {
		return
	}
	delete(staticVar.PyyActData, s.GetPlayer().GetId())
}

func (s *GlobalCollectCardsSys) reissueLogWorker() {
	data := s.getCardData(0)
	for itemId, count := range data.OfflineItemCountMap {
		s.addCard(itemId, count, pb3.LogId_LogPYYGlobalCollectCardsAddByAskHelp)
	}
	data.OfflineItemCountMap = make(map[uint32]uint32)
}

// 获取卡包
func (s *GlobalCollectCardsSys) getCardData(actorId uint64) *pb3.PYYGlobalCollectCardData {
	if actorId == 0 {
		actorId = s.GetPlayer().GetId()
	}
	staticVar := gshare.GetStaticVar()
	bag := staticVar.PyyActData
	if bag == nil {
		staticVar.PyyActData = make(map[uint64]*pb3.PYYActData)
		bag = staticVar.PyyActData
	}
	actBag := bag[actorId]
	if actBag == nil {
		bag[actorId] = &pb3.PYYActData{}
		actBag = bag[actorId]
	}
	if actBag.CardData == nil {
		actBag.CardData = &pb3.PYYGlobalCollectCardData{}
	}
	cardData := actBag.CardData
	if cardData.CollectCardMap == nil {
		cardData.CollectCardMap = make(map[uint32]uint32)
	}
	if cardData.ActiveCollectCardMap == nil {
		cardData.ActiveCollectCardMap = make(map[uint32]uint32)
	}
	if cardData.AskRecordMap == nil {
		cardData.AskRecordMap = make(map[uint64]*pb3.AskRecord)
	}
	if cardData.OfflineItemCountMap == nil {
		cardData.OfflineItemCountMap = make(map[uint32]uint32)
	}
	return cardData
}

func (s *GlobalCollectCardsSys) addCard(itemId uint32, count uint32, logId pb3.LogId) {
	cardData := s.getCardData(0)
	haveCount := cardData.CollectCardMap[itemId]
	cardData.CollectCardMap[itemId] += count
	s.s2cCollectCardMap(itemId, cardData.CollectCardMap[itemId])
	logworker.LogPlayerBehavior(s.GetPlayer(), logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d_%d", itemId, haveCount, count, cardData.CollectCardMap[itemId]),
	})
}

func (s *GlobalCollectCardsSys) subCard(itemId uint32, count uint32) (realSubCount uint32) {
	cardData := s.getCardData(0)
	haveCount := cardData.CollectCardMap[itemId]
	if haveCount < count {
		cardData.CollectCardMap[itemId] = 0
		realSubCount = haveCount
	} else {
		cardData.CollectCardMap[itemId] -= count
		realSubCount = count
	}
	s.s2cCollectCardMap(itemId, cardData.CollectCardMap[itemId])
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYGlobalCollectCardsSub, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d_%d", itemId, haveCount, realSubCount, cardData.CollectCardMap[itemId]),
	})
	return
}

func (s *GlobalCollectCardsSys) handleUseItemByCollectCard(param *miscitem.UseItemParamSt, _ *jsondata.BasicUseItemConf) {
	cardConf, ok := jsondata.GetGlobalCollectCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	itemId := param.ItemId
	cConf, ok := cardConf[param.ItemId]
	if !ok {
		return
	}
	s.addCard(itemId, uint32(param.Count), pb3.LogId_LogPYYGlobalCollectCardsAdd)
	if cConf.Bro {
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf != nil {
			owner := s.GetPlayer()
			engine.BroadcastTipMsgById(tipmsgid.PYYGlobalCollectCardsActiveCardSuitBro, owner.GetId(), owner.GetName(), itemConf.Name)
		}
	}
}

func (s *GlobalCollectCardsSys) s2cCollectCardMap(itemId uint32, count uint32) {
	s.SendProto3(61, 54, &pb3.S2C_61_54{
		ActiveId: s.GetId(),
		ItemId:   itemId,
		Count:    count,
	})
}

func (s *GlobalCollectCardsSys) c2sAskHelp(msg *base.Message) error {
	var req pb3.C2S_61_67
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	itemId := req.ItemId

	owner := s.GetPlayer()
	data := s.getCardData(owner.GetId())

	confMgr, ok := jsondata.GetGlobalCollectCardsAskCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found ask conf", s.GetPrefix())
	}

	if len(confMgr.AskHelpTypes) == 0 || !pie.Uint32s(confMgr.AskHelpTypes).Contains(req.Channel) {
		return neterror.ParamsInvalidError("不支持该方式求助")
	}

	cardConf, ok := jsondata.GetGlobalCollectCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found card conf", s.GetPrefix())
	}

	if _, ok := cardConf[itemId]; !ok {
		return neterror.ConfNotFoundError("%s not found %d card conf", s.GetPrefix(), itemId)
	}

	// 构造求助记录
	hdl, err := series.AllocSeries()
	if err != nil {
		return neterror.Wrap(err)
	}

	var askRecord = &pb3.AskRecord{
		Hdl:          hdl,
		ItemId:       itemId,
		AskCreatedAt: time_util.NowSec(),
		AskPlayerId:  owner.GetId(),
		AskCount:     confMgr.BeGiftTimeLimit, // 每条求助，最大可被赠予次数
		AcceptCount:  0,
		Channel:      req.Channel,
	}
	data.AskRecordMap[hdl] = askRecord

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogAskHelpSendHelp, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", itemId),
	})

	nowSec := time_util.NowSec()
	lastAskHelpAt := data.LastAskHelpAt
	if lastAskHelpAt != 0 && lastAskHelpAt+confMgr.Cd > nowSec {
		return neterror.ParamsInvalidError("cd %d, lastAskHelpAt %d,nowSec", confMgr.Cd, lastAskHelpAt, nowSec)
	}
	data.LastAskHelpAt = nowSec

	// 推送消息
	owner.ChannelChat(&pb3.C2S_5_1{
		Channel:     req.Channel,
		Msg:         fmt.Sprintf(""),
		ItemIds:     []uint64{uint64(req.ItemId)},
		Params:      fmt.Sprintf("%d", hdl),
		ContentType: chatdef.ContentGlobalCollectCardAskHelp,
	}, true)

	s.SendProto3(61, 67, &pb3.S2C_61_67{
		ActiveId:  s.GetId(),
		AskRecord: askRecord,
	})

	err = s.checkAskRecordFull()
	if err != nil {
		return neterror.Wrap(err)
	}
	owner.TriggerQuestEvent(custom_id.QttGlobalCollectCardsAskHelpX, 0, 1)
	owner.TriggerQuestEvent(custom_id.QttAchievementsGlobalCollectCardsAskHelpX, 0, 1)
	return nil
}

func (s *GlobalCollectCardsSys) c2sToHelp(msg *base.Message) error {
	var req pb3.C2S_61_68
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	hdl := req.Hdl
	itemId := req.ItemId
	targetId := req.TargetId
	owner := s.GetPlayer()
	selfData := s.getCardData(owner.GetId())

	if hdl == 0 && itemId == 0 {
		return neterror.ParamsInvalidError("hdl and itemId is zero")
	}

	if targetId == owner.GetId() {
		owner.SendTipMsg(tipmsgid.AskHelpTips1)
		return neterror.ParamsInvalidError("不能自己赠予自己")
	}

	playerInfo, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		owner.SendTipMsg(tipmsgid.TpAddFriendNoOne)
		return neterror.ParamsInvalidError("找不到玩家信息")
	}

	conf, ok := jsondata.GetGlobalCollectCardsAskCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConsumeFailedError("%s not found conf", s.GetPrefix())
	}
	targetAskInfo := s.getCardData(targetId)
	if targetAskInfo == nil {
		owner.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return neterror.ParamsInvalidError("无效的求助")
	}

	// 通过求助记录赠予
	var giveItemId = itemId
	var targetRecord *pb3.AskRecord
	if hdl != 0 {
		// 对方的求助记录已过期
		targetRecord, ok = targetAskInfo.AskRecordMap[hdl]
		if !ok {
			owner.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
			return neterror.ParamsInvalidError("无效的求助")
		}
		// 对方的接受赠予已达上限
		if targetRecord.AcceptCount >= targetRecord.AskCount {
			owner.SendTipMsg(tipmsgid.AskHelpTips2)
			return neterror.ParamsInvalidError("接受赠予达上限")
		}
		giveItemId = targetRecord.ItemId
	} else { // 主动赠送
		var canHelp = true
		if !owner.IsExistFriend(req.TargetId) {
			owner.SendTipMsg(tipmsgid.NotFriend)
			canHelp = false
		}

		if !canHelp && playerInfo.GuildId != 0 && owner.GetGuildId() != playerInfo.GuildId {
			owner.SendTipMsg(tipmsgid.TpGuildPlayerIsntMember)
			canHelp = false
		}

		if !canHelp {
			return nil
		}
	}

	if giveItemId == 0 {
		return neterror.ParamsInvalidError("giveItemId is zero")
	}

	// 自己的赠予次数上限
	var totalGiftCount = selfData.TodayGiftCount
	if conf.TimesForGift != 0 && conf.TimesForGift <= totalGiftCount {
		owner.SendTipMsg(tipmsgid.AskHelpTips4)
		return neterror.ParamsInvalidError("赠予次数上限")
	}

	// 自己的道具
	count := selfData.CollectCardMap[giveItemId]
	if count < 2 {
		owner.SendTipMsg(tipmsgid.ToHelpCountNotEnough)
		return nil
	}

	itemConf := jsondata.GetItemConfig(giveItemId)
	if itemConf == nil {
		owner.SendTipMsg(tipmsgid.NotFoundItemConf)
		return neterror.ConfNotFoundError("%d item not found", giveItemId)
	}

	itemName := itemConf.Name

	// 新增求助记录
	nowSec := time_util.NowSec()
	var offer = hdl == 0
	if hdl == 0 {
		hdl, _ = series.AllocSeries()
	}
	targetLog := &pb3.AskLog{
		Hdl:        hdl,
		TargetId:   owner.GetId(),
		CreatedAt:  nowSec,
		ItemId:     giveItemId,
		IsAdk:      true,
		TargetName: owner.GetName(),
		ItemName:   itemName,
		Offer:      offer,
	}

	myLog := &pb3.AskLog{
		Hdl:        hdl,
		TargetId:   playerInfo.Id,
		CreatedAt:  nowSec,
		ItemId:     itemId,
		IsAdk:      false,
		TargetName: playerInfo.Name,
		ItemName:   itemName,
		Offer:      offer,
	}

	// 移除道具
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogPYYGlobalCollectCardsSubByAskHelp, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", giveItemId),
	})
	s.subCard(giveItemId, 1)

	targetPlayer := manager.GetPlayerPtrById(targetId)
	if targetPlayer != nil { // 在线 加入背包
		if sys, ok := getGlobalCollectCardsSys(targetPlayer); ok && sys != nil {
			sys.addCard(giveItemId, 1, pb3.LogId_LogPYYGlobalCollectCardsAddByAskHelp)
		}
	} else {
		targetAskInfo.OfflineItemCountMap[giveItemId] += 1
	}

	// 处理赠予次数
	selfData.TodayGiftCount += 1

	// 新增日志
	targetAskInfo.Logs = append(targetAskInfo.Logs, targetLog)
	if targetRecord != nil {
		targetRecord.Logs = append(targetRecord.Logs, targetLog)
		targetRecord.AcceptCount += 1
	}

	ret, err := s.checkAskHelpLogFull(targetAskInfo.Logs)
	if err != nil {
		s.LogError("err:%v", err)
	} else {
		targetAskInfo.Logs = ret
	}

	selfData.Logs = append(selfData.Logs, myLog)
	ret, err = s.checkAskHelpLogFull(selfData.Logs)
	if err != nil {
		s.LogError("err:%v", err)
	} else {
		selfData.Logs = ret
	}

	// 通知自己
	owner.SendProto3(61, 68, &pb3.S2C_61_68{
		ActiveId: s.GetId(),
		Log:      myLog,
	})

	// 通知对方
	if targetPlayer != nil {
		targetPlayer.SendProto3(61, 69, &pb3.S2C_61_69{
			ActiveId: s.GetId(),
			Log:      targetLog,
		})
	}

	// 广播这条记录已经有人赠予
	if targetRecord != nil {
		engine.Broadcast(chatdef.CIWorld, 0, 61, 70, &pb3.S2C_61_70{
			ActiveId:  s.GetId(),
			AskRecord: targetRecord,
		}, 0)
	}

	owner.TriggerQuestEvent(custom_id.QttGlobalCollectCardsToHelpX, 0, 1)
	owner.TriggerQuestEvent(custom_id.QttAchievementsGlobalCollectCardsToHelpX, 0, 1)
	return nil
}

func (s *GlobalCollectCardsSys) checkAskHelpLogFull(logs []*pb3.AskLog) ([]*pb3.AskLog, error) {
	confMgr, ok := jsondata.GetGlobalCollectCardsAskCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil, neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	if uint32(len(logs)) <= confMgr.RecordLimit {
		return logs, nil
	}

	// 从大到小排序
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt > logs[j].CreatedAt
	})
	var ret = make([]*pb3.AskLog, confMgr.RecordLimit)
	copy(ret, logs)
	return ret, nil
}

func (s *GlobalCollectCardsSys) checkAskRecordFull() error {
	owner := s.GetPlayer()
	data := s.getCardData(owner.GetId())
	confMgr, ok := jsondata.GetGlobalCollectCardsAskCardConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	askRecordMap := data.AskRecordMap
	if confMgr.AskHelpTimes == 0 {
		return nil
	}

	if uint32(len(askRecordMap)) < confMgr.AskHelpTimes {
		return nil
	}

	var timeSec = time_util.NowSec()
	var delHdl uint64
	for hdl, record := range askRecordMap {
		if record.AskCreatedAt > timeSec {
			continue
		}
		timeSec = record.AskCreatedAt
		delHdl = hdl
	}
	if delHdl != 0 {
		delete(askRecordMap, delHdl)
	}

	return nil
}

func (s *GlobalCollectCardsSys) c2sGetAskRecord(msg *base.Message) error {
	var req pb3.C2S_61_71
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}
	data := s.getCardData(req.TargetId)
	record := data.AskRecordMap[req.Hdl]
	if record == nil {
		s.GetPlayer().SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return nil
	}
	s.GetPlayer().SendProto3(61, 71, &pb3.S2C_61_71{
		AskRecord: record,
		ActiveId:  s.GetId(),
	})
	return nil
}

func handleUseItemByCollectCard(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := getGlobalCollectCardsSys(player)
	if ok {
		sys.handleUseItemByCollectCard(param, conf)
	}
	// 全民集卡系统不存在 也要使用成功 不用给补偿奖励
	return true, true, param.Count
}
