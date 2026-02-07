/**
 * @Author: LvYuMeng
 * @Date: 2025/4/16
 * @Desc: 集卡
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type CollectCardSys struct {
	*QuestTargetBase
}

var (
	collectCardQuestTargetMap = map[uint32]map[uint32]struct{}{} // 境界任务事件对应的id
)

func newCollectCardSys() iface.ISystem {
	sys := &CollectCardSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *CollectCardSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	ids, ok := collectCardQuestTargetMap[qt]
	if !ok {
		return nil
	}

	data := s.getData()
	var mySet = make(map[uint32]struct{})
	for _, pri := range data.Privilege {
		for questId, quest := range pri.Quests {
			if _, exist := ids[questId]; exist {
				mySet[quest.Id] = struct{}{}
			}
		}
	}
	return mySet
}

func (s *CollectCardSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	pri, questId, ok := s.findPrivilegeByQuest(id)
	if !ok {
		return nil
	}

	questConf, ok := jsondata.GetCollectCardPrivilegeConf(pri.CardId, questId)
	if !ok {
		return nil
	}
	return questConf.Targets
}

func (s *CollectCardSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	pri, questId, ok := s.findPrivilegeByQuest(id)
	if !ok {
		return nil
	}

	if pie.Uint32s(pri.RevIds).Contains(questId) {
		return nil
	}

	return pri.Quests[questId]
}

func (s *CollectCardSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	pri, questId, ok := s.findPrivilegeByQuest(id)
	if !ok {
		return
	}
	//更新任务
	s.SendProto3(42, 15, &pb3.S2C_42_15{
		PrivilegeId: pri.Id,
		QuestId:     questId,
		Quest:       quest,
	})
}

func (s *CollectCardSys) findPrivilegeByQuest(id uint32) (*pb3.CardPrivilegeQuest, uint32, bool) {
	data := s.getData()
	for _, privilege := range data.Privilege {
		for questId, quest := range privilege.Quests {
			if quest.Id == id {
				return privilege, questId, true
			}
		}
	}
	return nil, 0, false
}

func (s *CollectCardSys) getData() *pb3.CardData {
	binary := s.GetBinaryData()
	cardData := binary.CardData
	if nil == cardData {
		cardData = &pb3.CardData{}
		binary.CardData = cardData
	}
	if nil == cardData.CardCount {
		cardData.CardCount = map[uint32]uint32{}
	}
	if nil == cardData.ActiveCard {
		cardData.ActiveCard = map[uint32]uint32{}
	}
	if nil == cardData.Privilege {
		cardData.Privilege = make(map[uint32]*pb3.CardPrivilegeQuest)
	}
	if nil == cardData.Collect {
		cardData.Collect = make(map[uint32]*pb3.CardCollect)
	}
	return binary.CardData
}

func (s *CollectCardSys) OnLogin() {
	s.checkOverPrivilege()
}

func (s *CollectCardSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *CollectCardSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CollectCardSys) s2cInfo() {
	s.SendProto3(42, 10, &pb3.S2C_42_10{Data: s.getData()})
}

func (s *CollectCardSys) checkOverPrivilege() {
	data := s.getData()
	var delIds []uint32
	for priId, pri := range data.Privilege {
		conf, ok := jsondata.GetCollectCardSt(pri.CardId)
		if !ok {
			continue
		}
		var completeCount uint32
		for _, questConf := range conf.PrivilegeConf {
			q, exist := pri.Quests[questConf.QuestId]
			if !exist {
				break
			}
			if !pie.Uint32s(pri.RevIds).Contains(q.Id) {
				break
			}
			completeCount++
		}
		if completeCount == uint32(len(conf.PrivilegeConf)) {
			delIds = append(delIds, priId)
		}
	}

	for _, delId := range delIds {
		delete(data.Privilege, delId)
		s.owner.LogInfo("过期卡牌特权删除:%d", delId)
	}

	if len(data.Privilege) == 0 { //手上没有任何特权直接清零
		data.Hdl = 0
		data.QuestHdl = 0
	}
}

func (s *CollectCardSys) c2sActiveCard(msg *base.Message) error {
	var req pb3.C2S_42_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	cardId := req.GetCardId()
	_, ok := jsondata.GetCollectCardSt(cardId)
	if !ok {
		return neterror.ConfNotFoundError("card conf %d not found", cardId)
	}

	data := s.getData()
	if _, ok := data.ActiveCard[cardId]; ok {
		return neterror.ParamsInvalidError("card %d already active", cardId)
	}

	if data.CardCount[cardId] == 0 {
		return neterror.ParamsInvalidError("card %d count not enough", cardId)
	}

	data.CardCount[cardId]--
	s.SendProto3(42, 16, &pb3.S2C_42_16{
		CardId: cardId,
		Count:  data.CardCount[cardId],
	})

	data.ActiveCard[cardId] = 1
	s.SendProto3(42, 11, &pb3.S2C_42_11{CardId: cardId})

	return nil
}

func (s *CollectCardSys) c2sGetCollectAward(msg *base.Message) error {
	var req pb3.C2S_42_12
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	groupId := req.CollectId
	groupConf, ok := jsondata.GetCollectAwards(groupId)
	if !ok {
		return neterror.ConfNotFoundError("collect award conf %d not found", groupId)
	}

	count := req.GetCount()
	data := s.getData()
	var activeCount uint32
	for _, v := range groupConf.Ids {
		if data.ActiveCard[v] > 0 {
			activeCount++
		}
	}

	collect, ok := data.Collect[groupId]
	if !ok {
		data.Collect[groupId] = &pb3.CardCollect{
			Id:     groupId,
			RevIds: nil,
		}
	}

	if pie.Uint32s(collect.RevIds).Contains(count) {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	progressConf, ok := groupConf.GetProgress(count)
	if !ok {
		return neterror.ConfNotFoundError("not found progress count conf %d", count)
	}

	if count > activeCount {
		s.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	collect.RevIds = append(collect.RevIds, count)

	// 发放奖励
	engine.GiveRewards(s.owner, progressConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogCollectCardAward,
	})

	s.SendProto3(42, 12, &pb3.S2C_42_12{
		CollectId: groupId,
		Count:     count,
	})

	engine.BroadcastTipMsgById(tipmsgid.CollectCardTip2, s.owner.GetId(), s.owner.GetName(), groupConf.TabName)

	return nil
}

func (s *CollectCardSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	for _, v := range data.Privilege {
		quest, ok := v.Quests[questId]
		if !ok {
			continue
		}
		if pie.Uint32s(v.RevIds).Contains(questId) {
			s.GmFinishQuest(quest)
			continue
		}
		s.OnAcceptQuestAndCheckUpdateTarget(quest)
	}
}

func (s *CollectCardSys) GMDelQuest(questId uint32) {
	data := s.getData()

	for _, v := range data.Privilege {
		v.RevIds = pie.Uint32s(v.RevIds).Filter(func(u uint32) bool {
			return u != questId
		})
		delete(v.Quests, questId)
	}
}

func (s *CollectCardSys) c2sGetPrivilegeAward(msg *base.Message) error {
	var req pb3.C2S_42_13
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	id := req.GetId()
	myId := req.GetQuestId()
	var questId uint32

	data := s.getData()
	privilege, ok := data.Privilege[id]
	if !ok {
		return neterror.ParamsInvalidError("privilege %d not found", id)
	}

	// 检查任务是否完成
	var quest *pb3.QuestData
	for idx, q := range privilege.Quests {
		if q.Id == myId {
			quest = q
			questId = idx
			break
		}
	}

	if quest == nil {
		return neterror.ParamsInvalidError("quest %d not found", myId)
	}

	if !s.CheckFinishQuest(quest) {
		return neterror.ParamsInvalidError("quest %d not finish", myId)
	}

	if pie.Uint32s(privilege.RevIds).Contains(myId) {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	// 获取奖励配置
	conf, ok := jsondata.GetCollectCardSt(privilege.CardId)
	if !ok {
		return neterror.ConfNotFoundError("card conf not found")
	}

	// 查找对应的特权配置
	var privilegeConf *jsondata.CollectCardPrivilegeConf
	for _, pc := range conf.PrivilegeConf {
		if pc.QuestId == questId {
			privilegeConf = pc
			break
		}
	}

	if privilegeConf == nil {
		return neterror.ConfNotFoundError("privilege conf not found")
	}

	privilege.RevIds = append(privilege.RevIds, myId)

	// 发放奖励
	engine.GiveRewards(s.owner, privilegeConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogCollectCardPrivilegeAward,
	})

	s.SendProto3(42, 13, &pb3.S2C_42_13{
		Id:      id,
		QuestId: myId,
	})

	return nil
}

func (s *CollectCardSys) allocPrivilegeId() uint32 {
	data := s.getData()
	data.Hdl++
	return data.Hdl
}

func (s *CollectCardSys) allocQuestId() uint32 {
	data := s.getData()
	data.QuestHdl++
	return data.QuestHdl
}

func (s *CollectCardSys) sendCard(targetId uint64, cardId uint32) bool {
	data := s.getData()

	if data.ActiveCard[cardId] == 0 {
		logger.LogError("card %d not active", cardId)
		return false
	}

	if data.CardCount[cardId] <= 0 {
		logger.LogError("card %d count not enough", cardId)
		return false
	}

	data.CardCount[cardId]--
	s.sendCardCount(cardId)

	engine.SendPlayerMessage(targetId, gshare.OfflineGetNewCollectCard, &pb3.CommonSt{U32Param: cardId})
	return true
}

func (s *CollectCardSys) addCard(cardId uint32) {
	data := s.getData()
	data.CardCount[cardId]++
	s.SendProto3(42, 16, &pb3.S2C_42_16{
		CardId: cardId,
		Count:  data.CardCount[cardId],
	})
}

func (s *CollectCardSys) sendCardCount(cardId uint32) {
	data := s.getData()
	s.SendProto3(42, 16, &pb3.S2C_42_16{
		CardId: cardId,
		Count:  data.CardCount[cardId],
	})
}

func (s *CollectCardSys) onCardRealGet(cardId uint32) (bool, error) {
	cardConf, ok := jsondata.GetCollectCardSt(cardId)
	if !ok {
		return false, neterror.ConfNotFoundError("not found card conf %d", cardId)
	}

	s.addCard(cardId)

	priId := s.allocPrivilegeId()
	pri := &pb3.CardPrivilegeQuest{
		Id:        priId,
		CardId:    cardId,
		Quests:    make(map[uint32]*pb3.QuestData, len(cardConf.PrivilegeConf)),
		TimeStamp: time_util.NowSec(),
	}

	data := s.getData()
	data.Privilege[priId] = pri

	for _, priConf := range cardConf.PrivilegeConf {
		quest := &pb3.QuestData{
			Id: s.allocQuestId(),
		}
		pri.Quests[priConf.QuestId] = quest
		s.OnAcceptQuestAndCheckUpdateTarget(quest)
	}

	data.Privilege[priId] = pri
	s.SendProto3(42, 14, &pb3.S2C_42_14{Privilege: pri})

	engine.BroadcastTipMsgById(tipmsgid.CollectCardTip1, s.owner.GetId(), s.owner.GetName(), cardConf.ItemId)

	return true, nil
}

func handleUseItemCollectCard(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiCollectCard).(*CollectCardSys)
	if !ok || !sys.IsOpen() {
		return false, false, 0
	}

	if len(conf.Param) < 1 {
		return false, false, 0
	}

	var useCount int64
	for i := int64(1); i <= param.Count; i++ {
		if succ, err := sys.onCardRealGet(conf.Param[0]); succ {
			useCount++
		} else {
			player.LogError("err:%v", err)
		}
	}
	return true, true, useCount
}

func onAfterReloadCollectCardConf(args ...interface{}) {
	conf := jsondata.GetCollectCardConf()
	if nil == conf {
		return
	}
	tmp := make(map[uint32]map[uint32]struct{})
	for _, card := range conf.Cards {
		for _, pri := range card.PrivilegeConf {
			for _, target := range pri.Targets {
				if _, ok := tmp[target.Type]; !ok {
					tmp[target.Type] = make(map[uint32]struct{})
				}
				tmp[target.Type][pri.QuestId] = struct{}{}
			}
		}
	}
	collectCardQuestTargetMap = tmp
}

func offlineGetNewCollectCard(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	cardId := st.U32Param
	if s, exist := player.GetSysObj(sysdef.SiCollectCard).(*CollectCardSys); exist && s.IsOpen() {
		s.addCard(cardId)
		return
	}

	return
}

func init() {
	RegisterSysClass(sysdef.SiCollectCard, func() iface.ISystem {
		return newCollectCardSys()
	})

	engine.RegisterMessage(gshare.OfflineGetNewCollectCard, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineGetNewCollectCard)

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadCollectCardConf)

	net.RegisterSysProtoV2(42, 11, sysdef.SiCollectCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CollectCardSys).c2sActiveCard
	})

	net.RegisterSysProtoV2(42, 12, sysdef.SiCollectCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CollectCardSys).c2sGetCollectAward
	})

	net.RegisterSysProtoV2(42, 13, sysdef.SiCollectCard, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CollectCardSys).c2sGetPrivilegeAward
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemCollectCard, handleUseItemCollectCard)

	gmevent.Register("collectCard.finishQuestAll", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiCollectCard).(*CollectCardSys)
		if !ok || !s.IsOpen() {
			return false
		}
		data := s.getData()
		for _, pri := range data.Privilege {
			for _, quest := range pri.Quests {
				s.GmFinishQuest(quest)
			}
		}
		return true
	}, 1)
}
