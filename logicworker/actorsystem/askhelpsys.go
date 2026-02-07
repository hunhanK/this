/**
 * @Author: zjj
 * @Date: 2024/5/13
 * @Desc: 求助系统
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type AskHelpSys struct {
	*QuestTargetBase
}

func createAskHelpSys() iface.ISystem {
	sys := &AskHelpSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *AskHelpSys) recAllQuest() {
	data := s.getData()
	owner := s.GetOwner()
	data.RecQuestIds = nil
	data.FinishQuestIds = nil
	data.QuestMap = make(map[uint32]*pb3.QuestData)
	mgr := jsondata.GetAskHelpQuestMgr()
	if mgr == nil {
		owner.LogError("not found ask help quest conf")
		return
	}
	for _, conf := range mgr {
		var quest = &pb3.QuestData{
			Id:       conf.Id,
			Progress: nil,
		}
		s.OnAcceptQuest(quest)
		data.QuestMap[conf.Id] = quest
	}
}

func (s *AskHelpSys) OnOpen() {
	s.recAllQuest()
	s.s2cInfo()
}
func (s *AskHelpSys) OnLogin() {
	data := s.getData()
	if data.OfflineCompleteAskHelpTimesMap == nil {
		return
	}
	for _, count := range data.OfflineCompleteAskHelpTimesMap {
		s.TriggerQuest(s.GetOwner(), 0, count)
	}
	data.OfflineCompleteAskHelpTimesMap = make(map[uint32]uint32)
}

func (s *AskHelpSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *AskHelpSys) OnReconnect() {
	s.s2cInfo()
}

func (s *AskHelpSys) getData() *pb3.AskHelpInfo {
	data := manager.GetAskHelpInfo(s.GetOwner().GetId())
	return data
}

func (s *AskHelpSys) s2cInfo() {
	s.SendProto3(67, 20, &pb3.S2C_67_20{
		Data: s.getData(),
	})
}

func (s *AskHelpSys) c2sAskHelp(msg *base.Message) error {
	var req pb3.C2S_67_21
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	groupId := req.GroupId
	itemId := req.ItemId

	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId=%d not found", itemId)
	}

	owner := s.GetOwner()
	data := s.getData()
	mgr, err := jsondata.GetAskHelpItmePoolConfMgr(groupId)
	if err != nil {
		return neterror.Wrap(err)
	}

	if mgr.Circle != 0 && mgr.Circle > owner.GetCircle() {
		return neterror.ParamsInvalidError("circle not enough")
	}

	confMgr, err := jsondata.GetAskHelpSystemConfMgr()
	if err != nil {
		return neterror.Wrap(err)
	}
	if len(confMgr.AskHelpTypes) == 0 || !pie.Uint32s(confMgr.AskHelpTypes).Contains(req.Channel) {
		return neterror.ParamsInvalidError("不支持该方式求助")
	}

	if req.TargetId > 0 && req.Channel == chatdef.CIPrivate {
		target := manager.GetPlayerPtrById(req.TargetId)
		if nil == target {
			s.owner.SendTipMsg(tipmsgid.TpTargetOffline)
			return nil
		}
	}

	// 构造求助记录
	hdl, err := series.AllocSeries()
	if err != nil {
		return neterror.Wrap(err)
	}

	var askRecord = &pb3.AskRecord{
		Hdl:          hdl,
		GroupId:      groupId,
		ItemId:       itemId,
		AskCreatedAt: time_util.NowSec(),
		AskPlayerId:  owner.GetId(),
		AskCount:     confMgr.BeGiftTimeLimit, // 每条求助，最大可被赠予次数
		AcceptCount:  0,
		Channel:      req.Channel,
	}
	data.AskRecordMap[hdl] = askRecord

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogAskHelpSendHelp, &pb3.LogPlayerCounter{
		NumArgs: uint64(itemId),
	})

	// 推送消息
	owner.ChannelChat(&pb3.C2S_5_1{
		Channel:     req.Channel,
		Msg:         "",
		ToId:        req.TargetId,
		ItemIds:     []uint64{uint64(req.ItemId)},
		Params:      fmt.Sprintf("%d", hdl),
		ContentType: chatdef.ContentAskHelp,
	}, !(req.Channel == chatdef.CIPrivate))

	s.SendProto3(67, 21, &pb3.S2C_67_21{
		AskRecord: askRecord,
	})

	err = s.checkAskRecordFull()
	if err != nil {
		return neterror.Wrap(err)
	}

	manager.SetAskHelpSaveFlag(owner.GetId())
	manager.SetAskHelpSaveFlag(req.TargetId)

	s.owner.TriggerQuestEvent(custom_id.QttAskHelp, itemConf.Type, 1)
	return nil
}

func (s *AskHelpSys) checkAskRecordFull() error {
	data := s.getData()
	confMgr, err := jsondata.GetAskHelpSystemConfMgr()
	if err != nil {
		return neterror.Wrap(err)
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

func checkAskHelpLogFull(logs []*pb3.AskLog) ([]*pb3.AskLog, error) {
	confMgr, err := jsondata.GetAskHelpSystemConfMgr()
	if err != nil {
		return nil, neterror.Wrap(err)
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

func (s *AskHelpSys) c2sToHelp(msg *base.Message) error {
	var req pb3.C2S_67_22
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	hdl := req.Hdl
	itemHdl := req.ItemHdl
	targetId := req.TargetId
	owner := s.GetOwner()
	selfData := s.getData()

	if targetId == owner.GetId() {
		owner.SendTipMsg(tipmsgid.AskHelpTips1)
		return neterror.ParamsInvalidError("不能自己赠予自己")
	}

	playerInfo, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		owner.SendTipMsg(tipmsgid.TpAddFriendNoOne)
		return neterror.ParamsInvalidError("找不到玩家信息")
	}

	conf, err := jsondata.GetAskHelpSystemConfMgr()
	if err != nil {
		return neterror.Wrap(err)
	}

	targetAskInfo := manager.GetAskHelpInfo(targetId)

	// 对方的求助记录已过期
	targetRecord, ok := targetAskInfo.AskRecordMap[hdl]
	if !ok {
		owner.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return neterror.ParamsInvalidError("无效的求助")
	}

	// 对方的获赠次数上限
	var totalAskCount uint32
	for _, count := range targetAskInfo.CompletedAskCountMap {
		totalAskCount += count
	}

	// 对方的获赠次数达上限
	if conf.TimesFromGift != 0 && conf.TimesFromGift <= totalAskCount {
		owner.SendTipMsg(tipmsgid.AskHelpRecordAccordFail)
		return neterror.ParamsInvalidError("获赠次数达上限")
	}

	// 对方的接受赠予已达上限
	if targetRecord.AcceptCount >= targetRecord.AskCount {
		owner.SendTipMsg(tipmsgid.AskHelpTips2)
		return neterror.ParamsInvalidError("接受赠予达上限")
	}

	// 对方的每条求助，每个玩家最大赠予次数
	giftCount := selfData.CompletedGiftCountMap[hdl]
	if giftCount != 0 && giftCount >= conf.GftTimeLimit {
		owner.SendTipMsg(tipmsgid.AskHelpTips3)
		return neterror.ParamsInvalidError("该条求助达赠予上限")
	}

	// 自己的赠予次数上限
	var totalGiftCount uint32
	for _, count := range selfData.CompletedGiftCountMap {
		totalGiftCount += count
	}
	if conf.TimesForGift != 0 && conf.TimesForGift <= totalGiftCount {
		owner.SendTipMsg(tipmsgid.AskHelpTips4)
		return neterror.ParamsInvalidError("赠予次数上限")
	}

	// 自己的道具
	itemSt := owner.GetItemByHandle(itemHdl)
	if itemSt == nil {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return neterror.ParamsInvalidError("赠予道具不存在")
	}
	if itemSt.Bind {
		owner.SendTipMsg(tipmsgid.TpUseItemFailed)
		return neterror.ParamsInvalidError("绑定道具无法操作")
	}

	itemId := itemSt.ItemId
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		owner.SendTipMsg(tipmsgid.NotFoundItemConf)
		return neterror.ConfNotFoundError("%d item not found", itemId)
	}

	itemName := itemConf.Name
	// 新增求助记录
	nowSec := time_util.NowSec()
	targetLog := &pb3.AskLog{
		Hdl:        hdl,
		ItemHdl:    itemHdl,
		TargetId:   owner.GetId(),
		CreatedAt:  nowSec,
		ItemId:     itemId,
		IsAdk:      true,
		TargetName: owner.GetName(),
		ItemName:   itemName,
	}

	// 新增赠予记录
	myLog := &pb3.AskLog{
		Hdl:        hdl,
		ItemHdl:    itemHdl,
		TargetId:   playerInfo.Id,
		CreatedAt:  nowSec,
		ItemId:     itemId,
		IsAdk:      false,
		TargetName: playerInfo.Name,
		ItemName:   itemName,
	}

	// 移除道具
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogAskHelpToHelpDelItem, &pb3.LogPlayerCounter{
		NumArgs: uint64(itemSt.ItemId),
		StrArgs: fmt.Sprintf("%d", itemSt.Handle),
	})
	owner.DelItemByHand(itemSt.Handle, pb3.LogId_LogAskHelpToHelp)
	targetRecord.AcceptCount += 1
	targetPlayer := manager.GetPlayerPtrById(targetId)
	if targetPlayer == nil { // 离线 直接发邮件
		mailmgr.SendMailToActor(targetId, &mailargs.SendMailSt{
			ConfId:    common.Mail_AskHelpGiftAwards,
			UserItems: []*pb3.ItemSt{itemSt},
			Content: &mailargs.CommonMailArgs{
				Str1: owner.GetName(),
				Str2: itemName,
			},
		})
		targetAskInfo.OfflineCompleteAskHelpTimesMap[itemId] += 1
	} else { // 在线 加入背包
		if !targetPlayer.AddItemPtr(itemSt, false, pb3.LogId_LogAskHelpToHelpAwardAddItem) {
			mailmgr.SendMailToActor(targetId, &mailargs.SendMailSt{
				ConfId:    common.Mail_AskHelpGiftAwards,
				UserItems: []*pb3.ItemSt{itemSt},
				Content: &mailargs.CommonMailArgs{
					Str1: owner.GetName(),
					Str2: itemName,
				},
			})
		}
		logworker.LogPlayerBehavior(targetPlayer, pb3.LogId_LogAskHelpToHelpAward, &pb3.LogPlayerCounter{
			NumArgs: uint64(itemSt.ItemId),
			StrArgs: fmt.Sprintf("%d", itemSt.Handle),
		})
		s.TriggerQuest(targetPlayer, itemId, 1)
	}

	// 处理求助、赠予次数
	targetAskInfo.CompletedAskCountMap[hdl]++
	selfData.CompletedGiftCountMap[hdl]++

	// 新增日志
	targetAskInfo.Logs = append(targetAskInfo.Logs, targetLog)
	targetRecord.Logs = append(targetRecord.Logs, targetLog)

	ret, err := checkAskHelpLogFull(targetAskInfo.Logs)
	if err != nil {
		s.LogError("err:%v", err)
	} else {
		targetAskInfo.Logs = ret
	}

	selfData.Logs = append(selfData.Logs, myLog)
	ret, err = checkAskHelpLogFull(selfData.Logs)
	if err != nil {
		s.LogError("err:%v", err)
	} else {
		selfData.Logs = ret
	}

	owner.SendProto3(67, 22, &pb3.S2C_67_22{
		Log: myLog,
	})
	owner.TriggerQuestEvent(custom_id.QttAchievementsAskHelpGiftByGroupId, targetRecord.GroupId, 1)
	owner.TriggerQuestEvent(custom_id.QttToHelp, itemConf.Type, 1)
	if targetPlayer != nil {
		targetPlayer.SendProto3(67, 23, &pb3.S2C_67_23{
			Log: targetLog,
		})
	}
	engine.Broadcast(chatdef.CIWorld, 0, 67, 25, &pb3.S2C_67_25{
		AskRecord: targetRecord,
	}, 0)

	manager.SetAskHelpSaveFlag(owner.GetId())
	manager.SetAskHelpSaveFlag(targetId)
	return nil
}

func (s *AskHelpSys) TriggerQuest(player iface.IPlayer, itemId uint32, count uint32) {
	player.TriggerQuestEvent(custom_id.QttAskHelpSuccess, 0, 1)
	player.TriggerQuestEvent(custom_id.QttAchievementsAskHelpSuccess, 0, 1)
}

func (s *AskHelpSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	owner := s.GetOwner()
	mgr := jsondata.GetAskHelpQuestMgr()
	if mgr == nil {
		owner.LogError("not found ask quest conf")
		return set
	}
	for _, conf := range mgr {
		var exist bool
		for _, target := range conf.Targets {
			if target.Type != qtt {
				continue
			}
			exist = true
			break
		}
		if exist {
			set[conf.Id] = struct{}{}
		}
	}
	return set
}

func (s *AskHelpSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()

	if pie.Uint32s(data.FinishQuestIds).Contains(id) {
		return nil
	}

	return data.QuestMap[id]
}

func (s *AskHelpSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetAskHelpQuest(id)
	owner := s.GetOwner()
	if conf == nil {
		owner.LogError("not found %d quest conf", id)
		return nil
	}
	return conf.Targets
}

func (s *AskHelpSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}
	data := s.getData()
	finishQuestIds := pie.Uint32s(data.FinishQuestIds)
	if finishQuestIds.Contains(quest.Id) {
		return
	}
	s.SendProto3(67, 26, &pb3.S2C_67_26{
		Quest: quest,
	})
	if !s.CheckFinishQuest(quest) {
		return
	}
	data.FinishQuestIds = finishQuestIds.Append(id).Unique()
}

func (s *AskHelpSys) c2sRecQuestAwards(msg *base.Message) error {
	var req pb3.C2S_67_27
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()
	owner := s.GetOwner()
	quest := jsondata.GetAskHelpQuest(req.QuestId)
	if quest == nil {
		return neterror.ConfNotFoundError("quest %d not found", req.QuestId)
	}
	questId := req.QuestId
	finishQuestIds := data.FinishQuestIds
	recQuestIds := data.RecQuestIds
	if !pie.Uint32s(finishQuestIds).Contains(questId) {
		return neterror.ParamsInvalidError("not found quest, id is %d", questId)
	}
	if pie.Uint32s(recQuestIds).Contains(questId) {
		return neterror.ParamsInvalidError("already rec quest awards, id is %d", questId)
	}
	recQuestIds = append(recQuestIds, questId)
	data.RecQuestIds = recQuestIds
	// 下发奖励
	if len(quest.Awards) > 0 {
		engine.GiveRewards(owner, quest.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogAskHelpQuestRecAwards,
		})
	}
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogAskHelpQuestRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.QuestId),
	})
	s.SendProto3(67, 27, &pb3.S2C_67_27{
		QuestId: questId,
	})
	return nil
}

func (s *AskHelpSys) c2sRead(msg *base.Message) error {
	var req pb3.C2S_67_28
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()
	var log *pb3.AskLog
	for _, l := range data.Logs {
		if l.Hdl != req.Hdl {
			continue
		}
		log = l
		break
	}
	if log == nil {
		return neterror.ParamsInvalidError("not found record %d", req.Hdl)
	}
	log.ReadAt = time_util.NowSec()
	s.SendProto3(67, 28, &pb3.S2C_67_28{
		Hdl:    req.Hdl,
		ReadAt: log.ReadAt,
	})

	manager.SetAskHelpSaveFlag(s.owner.GetId())
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiAskHelp, func() iface.ISystem {
		return createAskHelpSys()
	})
	event.RegActorEvent(custom_id.AeNewWeek, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiAskHelp).(*AskHelpSys); ok {
			sys.s2cInfo()
		}
	})
	net.RegisterSysProtoV2(67, 21, sysdef.SiAskHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskHelpSys).c2sAskHelp
	})
	net.RegisterSysProtoV2(67, 22, sysdef.SiAskHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskHelpSys).c2sToHelp
	})
	net.RegisterSysProtoV2(67, 27, sysdef.SiAskHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskHelpSys).c2sRecQuestAwards
	})
	net.RegisterSysProtoV2(67, 28, sysdef.SiAskHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskHelpSys).c2sRead
	})
}
