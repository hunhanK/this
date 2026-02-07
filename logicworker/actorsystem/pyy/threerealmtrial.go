/**
 * @Author: LvYuMeng
 * @Date: 2025/9/11
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type ThreeRealmTrial struct {
	*YYQuestTargetBase
}

const (
	ThreeRealmTrialTypeCharge  = 1
	ThreeRealmTrialTypeConsume = 2
	ThreeRealmTrialTypeQuest   = 3
)

func createThreeRealmTrial() iface.IPlayerYY {
	obj := &ThreeRealmTrial{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *ThreeRealmTrial) GMReAcceptQuest(questId uint32) {
	_, qData, ok := s.getQuest(questId)
	if !ok {
		return
	}
	if qData.BuyCount > 0 {
		s.GmFinishQuest(qData.Quest)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(qData.Quest)
}

func (s *ThreeRealmTrial) GMDelQuest(questId uint32) {
	tabId, _, ok := s.getQuestConf(questId)
	if !ok {
		return
	}
	tabData := s.getTabData(tabId)
	delete(tabData.Quests, questId)
}

func (s *ThreeRealmTrial) getConf() (*jsondata.ThreeRealmTrialConfig, bool) {
	return jsondata.GetThreeRealmsTrialConf(s.ConfName, s.ConfIdx)
}

func (s *ThreeRealmTrial) getQuestConf(questId uint32) (uint32, *jsondata.ThreeRealmTrialQuests, bool) {
	conf, ok := s.getConf()
	if !ok {
		return 0, nil, false
	}

	for _, tab := range conf.Tabs {
		if tab.Quests == nil {
			continue
		}
		if v, exist := tab.Quests[questId]; exist {
			return tab.Id, v, ok
		}
	}

	return 0, nil, false
}

func (s *ThreeRealmTrial) getQuestByChargeId(chargeId uint32) (uint32, *jsondata.ThreeRealmTrialQuests, bool) {
	conf, ok := s.getConf()
	if !ok {
		return 0, nil, false
	}

	for _, tab := range conf.Tabs {
		for _, v := range tab.Quests {
			if v.Type != ThreeRealmTrialTypeCharge {
				continue
			}
			if v.ChargeId != chargeId {
				continue
			}
			return tab.Id, v, true
		}
	}
	return 0, nil, false
}

func (s *ThreeRealmTrial) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	_, questConf, ok := s.getQuestConf(questId)
	if !ok {
		return nil
	}
	if questConf.Type != ThreeRealmTrialTypeQuest {
		return nil
	}
	return questConf.Targets
}

func (s *ThreeRealmTrial) getQuestIdSetFunc(qt uint32) map[uint32]struct{} {
	conf, ok := s.getConf()
	if !ok {
		return nil
	}

	set := make(map[uint32]struct{})
	for _, tab := range conf.Tabs {
		for _, quest := range tab.Quests {
			if quest.Type != ThreeRealmTrialTypeQuest {
				continue
			}
			for _, target := range quest.Targets {
				if target.Type == qt {
					set[quest.Id] = struct{}{}
				}
			}
		}
	}

	return set
}

func (s *ThreeRealmTrial) getQuest(id uint32) (uint32, *pb3.ThreeRealmTrialQuest, bool) {
	tabId, _, ok := s.getQuestConf(id)
	if !ok {
		return 0, nil, false
	}

	tabData := s.getTabData(tabId)

	quest, ok := tabData.Quests[id]
	if !ok {
		quest = &pb3.ThreeRealmTrialQuest{
			Id: id,
		}
		tabData.Quests[id] = quest
	}
	return tabId, quest, true
}

func (s *ThreeRealmTrial) getUnFinishQuestData(id uint32) *pb3.QuestData {
	_, questData, ok := s.getQuest(id)
	if !ok {
		return nil
	}
	if questData.BuyCount > 0 {
		return nil
	}
	return questData.Quest
}

func (s *ThreeRealmTrial) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	tabId, sysQuest, ok := s.getQuest(id)
	if !ok || sysQuest.Quest == nil {
		return
	}

	s.SendProto3(142, 11, &pb3.S2C_142_11{
		ActiveId: s.Id,
		TabId:    tabId,
		Quest:    sysQuest,
	})

	s.GetPlayer().TriggerQuestEventRange(custom_id.QttThreeRealmTrial)
}

func (s *ThreeRealmTrial) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.ThreeRealmTrial {
		return
	}
	delete(yyData.ThreeRealmTrial, s.Id)
}

func (s *ThreeRealmTrial) Login() {
	s.s2cInfo()
}

func (s *ThreeRealmTrial) OnReconnect() {
	s.s2cInfo()
}

func (s *ThreeRealmTrial) OnOpen() {
	s.acceptQuest()
	s.s2cInfo()
}

func (s *ThreeRealmTrial) OnEnd() {
	conf, ok := s.getConf()
	if !ok {
		return
	}

	data := s.getData()
	complete := true
	var rewardsVec []jsondata.StdRewardVec
	for _, tab := range conf.Tabs {
		for _, v := range tab.Quests {
			_, q, exist := s.getQuest(v.Id)
			if !exist {
				complete = false
			}
			if q.BuyCount > 0 {
				continue
			}
			if v.Type == ThreeRealmTrialTypeQuest && s.CheckFinishQuest(q.Quest) {
				rewardsVec = append(rewardsVec, v.Rewards)
				q.BuyCount++
				continue
			}
			complete = false
		}
	}

	if complete && !data.IsRev {
		rewardsVec = append(rewardsVec, conf.Rewards)
	}

	rewards := jsondata.AppendStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (s *ThreeRealmTrial) acceptQuest() {
	conf, ok := s.getConf()
	if !ok {
		return
	}
	for id, tab := range conf.Tabs {
		tabData := s.getTabData(id)
		for _, v := range tab.Quests {
			if v.Type != ThreeRealmTrialTypeQuest {
				continue
			}
			if q, exist := tabData.Quests[v.Id]; exist && q.Quest != nil {
				continue
			}
			quest := &pb3.ThreeRealmTrialQuest{
				Id: v.Id,
				Quest: &pb3.QuestData{
					Id:       v.Id,
					Progress: nil,
				},
			}
			tabData.Quests[v.Id] = quest
			s.OnAcceptQuestAndCheckUpdateTarget(quest.Quest)
		}
	}
}

func (s *ThreeRealmTrial) s2cInfo() {
	s.SendProto3(142, 10, &pb3.S2C_142_10{
		ActiveId: s.Id,
		Data:     s.getData(),
	})
}

func (s *ThreeRealmTrial) getTabData(id uint32) *pb3.ThreeRealmTrial {
	data := s.getData()
	tabData, ok := data.Data[id]
	if !ok {
		tabData = &pb3.ThreeRealmTrial{}
		data.Data[id] = tabData
	}
	if tabData.Quests == nil {
		tabData.Quests = make(map[uint32]*pb3.ThreeRealmTrialQuest)
	}
	return tabData
}

func (s *ThreeRealmTrial) getData() *pb3.PYYThreeRealmTrial {
	state := s.GetYYData()
	if nil == state.ThreeRealmTrial {
		state.ThreeRealmTrial = make(map[uint32]*pb3.PYYThreeRealmTrial)
	}
	if state.ThreeRealmTrial[s.Id] == nil {
		state.ThreeRealmTrial[s.Id] = &pb3.PYYThreeRealmTrial{}
	}
	if nil == state.ThreeRealmTrial[s.Id].Data {
		state.ThreeRealmTrial[s.Id].Data = map[uint32]*pb3.ThreeRealmTrial{}
	}
	return state.ThreeRealmTrial[s.Id]
}

func (s *ThreeRealmTrial) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	_, qConf, ok := s.getQuestByChargeId(chargeConf.ChargeId)
	if !ok {
		return false
	}
	_, questData, ok := s.getQuest(qConf.Id)
	if !ok {
		return false
	}
	if questData.BuyCount >= qConf.Count {
		return false
	}
	return true
}

func (s *ThreeRealmTrial) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	tabId, qConf, ok := s.getQuestByChargeId(chargeConf.ChargeId)
	if !ok {
		return false
	}
	_, questData, ok := s.getQuest(qConf.Id)
	if !ok {
		return false
	}
	if questData.BuyCount >= qConf.Count {
		return false
	}
	questData.BuyCount++
	s.SendProto3(142, 11, &pb3.S2C_142_11{
		ActiveId: s.Id,
		TabId:    tabId,
		Quest:    questData,
	})
	engine.GiveRewards(s.GetPlayer(), qConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogThreeRealmTrial})
	s.GetPlayer().TriggerQuestEventRange(custom_id.QttThreeRealmTrial)
	return true
}

func (s *ThreeRealmTrial) c2sRev(msg *base.Message) error {
	var req pb3.C2S_142_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	sendLastRewards := func() error {
		data := s.getData()
		if data.IsRev {
			s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		conf, ok := s.getConf()
		if !ok {
			return neterror.ConfNotFoundError("conf is nil")
		}
		for _, tab := range conf.Tabs {
			for _, v := range tab.Quests {
				_, q, exist := s.getQuest(v.Id)
				if !exist {
					return neterror.ParamsInvalidError("quest not exist")
				}
				if q.BuyCount > 0 {
					continue
				}
				if v.Type == ThreeRealmTrialTypeQuest && s.CheckFinishQuest(q.Quest) {
					continue
				}
				return neterror.ParamsInvalidError("not complete")
			}
		}
		data.IsRev = true
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogThreeRealmTrial})
		s.s2cInfo()
		s.GetPlayer().TriggerQuestEventRange(custom_id.QttThreeRealmTrial)
		return nil
	}

	sendQuestRewards := func(rTabId, id uint32) error {
		tabId, qConf, ok := s.getQuestConf(id)
		if !ok {
			return neterror.ParamsInvalidError("quest conf not exist")
		}
		if tabId != rTabId {
			return neterror.ParamsInvalidError("tabId miss")
		}
		_, qData, ok := s.getQuest(id)
		if !ok {
			return neterror.ParamsInvalidError("quest not exist")
		}
		if qData.BuyCount >= qConf.Count {
			s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		switch qConf.Type {
		case ThreeRealmTrialTypeConsume:
			if !s.GetPlayer().ConsumeByConf(qConf.Consume, false, common.ConsumeParams{
				LogId: pb3.LogId_LogThreeRealmTrial,
			}) {
				s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
				return nil
			}
		case ThreeRealmTrialTypeQuest:
			if !s.CheckFinishQuest(qData.Quest) {
				return neterror.ParamsInvalidError("not complete")
			}
		default:
			return neterror.ParamsInvalidError("quest type error")
		}
		qData.BuyCount++
		engine.GiveRewards(s.GetPlayer(), qConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogThreeRealmTrial})
		s.SendProto3(142, 11, &pb3.S2C_142_11{
			ActiveId: s.Id,
			TabId:    tabId,
			Quest:    qData,
		})
		s.GetPlayer().TriggerQuestEventRange(custom_id.QttThreeRealmTrial)
		return nil
	}

	if req.TabId == 0 {
		err = sendLastRewards()
	} else {
		err = sendQuestRewards(req.TabId, req.Id)
	}

	return err
}

func handleQttThreeRealmTrial(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) < 2 {
		return 0
	}
	pyyId := ids[0]
	tabId := ids[1]
	var count uint32
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYThreeRealmTrial, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*ThreeRealmTrial)
		if !ok {
			return
		}
		if sys.Id != pyyId {
			return
		}
		tabData := sys.getTabData(tabId)
		for _, v := range tabData.Quests {
			if v.BuyCount > 0 || sys.CheckFinishQuest(v.Quest) {
				count++
				continue
			}
		}
	})
	return count
}

func threeRealmTrialChargeCheck(actor iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYThreeRealmTrial, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*ThreeRealmTrial)
		if !ok {
			return
		}
		if ret {
			return
		}
		ret = sys.chargeCheck(chargeConf)
		return
	})
	return ret
}

func threeRealmTrialChargeBack(actor iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYThreeRealmTrial, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*ThreeRealmTrial)
		if !ok {
			return
		}
		if ret {
			return
		}
		ret = sys.chargeBack(chargeConf)
		return
	})
	return ret
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYThreeRealmTrial, createThreeRealmTrial)

	engine.RegChargeEvent(chargedef.ThreeRealmTrial, threeRealmTrialChargeCheck, threeRealmTrialChargeBack)

	engine.RegQuestTargetProgress(custom_id.QttThreeRealmTrial, handleQttThreeRealmTrial)

	net.RegisterYYSysProtoV2(142, 12, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ThreeRealmTrial).c2sRev
	})

	gmevent.Register("threeRealmTrial.finish", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYThreeRealmTrial, func(obj iface.IPlayerYY) {
			sys, ok := obj.(*ThreeRealmTrial)
			if !ok || !sys.IsOpen() {
				return
			}
			data := sys.getData()
			for _, i := range data.Data {
				for _, j := range i.Quests {
					if j.BuyCount > 0 || j.Quest == nil {
						continue
					}
					sys.GmFinishQuest(j.Quest)
				}
			}
			return
		})
		return true
	}, 1)
}
