/**
 * @Author: yzh
 * @Date:
 * @Desc: 免费VIP(踏仙途)
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/srvlib/utils"
)

type FreeVipSys struct {
	*QuestTargetBase
}

func newFreeVipSys() iface.ISystem {
	sys := &FreeVipSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *FreeVipSys) OnLogin() {
	s.checkAndAddQuestList()
}

func (s *FreeVipSys) OnReconnect() {
	s.checkAndAddQuestList()
	s.ResetSysAttr(attrdef.SaFreeVip)
}

func (s *FreeVipSys) OnAfterLogin() {
	s.S2cFreeVipState()
	s.owner.SetExtraAttr(attrdef.FreeVipLv, attrdef.AttrValueAlias(s.getData().Level))
}

func (s *FreeVipSys) OnOpen() {
	s.checkAndAddQuestList()
	s.ResetSysAttr(attrdef.SaFreeVip)
	s.S2cFreeVipState()
	s.owner.SetExtraAttr(attrdef.FreeVipLv, attrdef.AttrValueAlias(s.getData().Level))
}

func (s *FreeVipSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), logId, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
		NumArgs: coreNumData,
	})
}

func (s *FreeVipSys) getData() *pb3.FreeVipState {
	state := s.GetBinaryData().FreeVipState
	if state == nil {
		state = &pb3.FreeVipState{}
		s.GetBinaryData().FreeVipState = state
	}
	return state
}

func (s *FreeVipSys) getQuestIdSet(questType uint32) map[uint32]struct{} {
	set := make(map[uint32]struct{})
	data := s.getData()
	conf := jsondata.GetFreeVipLvConf(data.Level + 1)
	if conf == nil {
		return set
	}

	finishQuestIdSet := map[uint32]struct{}{}
	for _, id := range data.FinishQuestIds {
		finishQuestIdSet[id] = struct{}{}
	}
	for _, quest := range conf.QuestList {
		if _, ok := finishQuestIdSet[quest.QuestId]; ok {
			continue
		}
		for _, target := range quest.Targets {
			if target.Type == questType {
				set[quest.QuestId] = struct{}{}
			}
		}
	}
	return set
}

func (s *FreeVipSys) getQuestData(questId uint32) *pb3.QuestData {
	data := s.getData()
	for _, quest := range data.Quests {
		if questId == quest.GetId() {
			return quest
		}
	}
	return nil
}

func (s *FreeVipSys) getQuestTargetConf(questId uint32) []*jsondata.QuestTargetConf {
	data := s.getData()
	conf := jsondata.GetFreeVipLvConf(data.GetLevel() + 1)
	if conf == nil {
		return nil
	}
	for _, v := range conf.QuestList {
		if v.QuestId == questId {
			return v.Targets
		}
	}
	return nil
}

func (s *FreeVipSys) onUpdateTargetData(questId uint32) {
	data := s.getData()
	var quest *pb3.QuestData
	for _, oneQuest := range data.Quests {
		if questId != oneQuest.GetId() {
			continue
		}

		quest = oneQuest
		break
	}

	msg := &pb3.S2C_7_22{}
	msg.Quest = quest
	s.SendProto3(7, 22, msg)

	if !s.CheckFinishQuest(quest) {
		return
	}

	for _, passQuestId := range data.PassQuestIds {
		if passQuestId == questId {
			return
		}
	}

	nextLv := data.GetLevel() + 1
	conf := jsondata.GetFreeVipLvConf(nextLv)
	if conf == nil {
		return
	}

	data.PassQuestIds = append(data.PassQuestIds, questId)

	s.owner.TriggerQuestEvent(custom_id.QttFreeVipFinishQuest, nextLv, 1)

	// 打点
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFreeVipQuestFin, &pb3.LogPlayerCounter{
		NumArgs: uint64(questId),
		StrArgs: utils.I32toa(nextLv),
	})
}

func (s *FreeVipSys) SkipCheckQuestProgress() {
	s.SkippedCheckProgress = true
}

func (s *FreeVipSys) ResumeCheckQuestProgress() {
	s.SkippedCheckProgress = false
}

func (s *FreeVipSys) c2sFreeVipState(_ *base.Message) {
	s.S2cFreeVipState()
}

func (s *FreeVipSys) S2cFreeVipState() {
	msg := &pb3.S2C_7_20{}
	msg.FreeVipState = s.getData()
	s.SendProto3(7, 20, msg)
}

func (s *FreeVipSys) checkAndAddQuestList() {
	data := s.getData()
	if len(data.Quests) > 0 {
		return
	}

	conf := jsondata.GetFreeVipLvConf(data.GetLevel() + 1)
	if conf == nil {
		return
	}
	questList := conf.QuestList
	if questList == nil { // 没有任务的时候不处理  等到获得道具的时候再检测
		return
	}
	data.Quests = make([]*pb3.QuestData, 0, len(questList))
	data.PassQuestIds = nil
	data.FinishQuestIds = nil
	for _, v := range questList {
		quest := &pb3.QuestData{}
		quest.Id = v.QuestId
		s.OnAcceptQuest(quest)
		data.Quests = append(data.Quests, quest)
		if s.CheckFinishQuest(quest) {
			data.PassQuestIds = append(data.PassQuestIds, quest.Id)
		}
	}
}

func (s *FreeVipSys) ResetQuest() {
	data := s.getData()
	data.Quests = nil
	data.FinishQuestIds = nil
	s.checkAndAddQuestList()
}

func (s *FreeVipSys) c2sRewardQuest(msg *base.Message) {
	var req pb3.C2S_7_23
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	data := s.getData()

	var quest *pb3.QuestData
	for _, oneQuest := range data.Quests {
		if oneQuest.Id == req.QuestId {
			quest = oneQuest
			break
		}
	}
	if quest == nil {
		return
	}

	for _, finishId := range data.FinishQuestIds {
		if finishId == req.QuestId {
			return
		}
	}

	if !s.CheckFinishQuest(quest) {
		return
	}

	// 下发奖励
	var questConf *jsondata.FreeVipQuest
	for _, conf := range jsondata.FreeVipConfMgr {
		for _, oneQuestConf := range conf.QuestList {
			if oneQuestConf.QuestId != req.QuestId {
				continue
			}
			questConf = oneQuestConf
			break
		}

		if questConf != nil {
			break
		}
	}

	if questConf == nil {
		return
	}

	s.LogPlayerBehavior(uint64(req.QuestId), map[string]interface{}{}, pb3.LogId_LogFreeVipRewardQuest)
	data.FinishQuestIds = append(data.FinishQuestIds, req.QuestId)

	engine.GiveRewards(s.owner, questConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFreeVipQuestAward})

	questAwardMsg := &pb3.S2C_7_23{}
	questAwardMsg.FinishQuestIds = data.FinishQuestIds
	s.owner.SendProto3(7, 23, questAwardMsg)

	// 如果任务全部完成就升级
	questNum := uint32(len(data.Quests))
	revRewardQuestNum := len(data.FinishQuestIds)
	if uint32(revRewardQuestNum) != questNum {
		return
	}
	s.S2cFreeVipState()
}

func (s *FreeVipSys) c2sGetLvAward(msg *base.Message) {
	var req pb3.C2S_7_24
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	lv := req.Level
	data := s.getData()
	if lv <= 0 || lv > data.Level+1 {
		return
	}

	if lv == data.Level+1 {
		questNum := uint32(len(data.Quests))
		passNum := len(data.PassQuestIds)
		if uint32(passNum) != questNum {
			return
		}
	}

	lvConf := jsondata.GetFreeVipLvConf(lv)
	if lvConf == nil {
		return
	}

	for _, recvLv := range data.RecvRewardLvs {
		if recvLv == lv {
			return
		}
	}

	// 如果任务全部完成 才可以
	questNum := uint32(len(data.Quests))
	revRewardQuestNum := len(data.FinishQuestIds)
	if uint32(revRewardQuestNum) != questNum {
		return
	}
	s.LogPlayerBehavior(uint64(req.Level), map[string]interface{}{}, pb3.LogId_LogFreeVipGetLvAward)

	data.RecvRewardLvs = append(data.RecvRewardLvs, lv)

	engine.GiveRewards(s.owner, lvConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFreeVipLvAward})

	recvRewardMsg := &pb3.S2C_7_24{}
	recvRewardMsg.RecvRewardLvs = data.RecvRewardLvs
	s.owner.SendProto3(7, 24, recvRewardMsg)

	// 添加新等级任务
	data.Level++
	data.PassQuestIds = nil
	data.FinishQuestIds = nil
	s.owner.TriggerEvent(custom_id.AeFreeUpLv, data.Level)
	s.owner.TriggerQuestEvent(custom_id.QttUpgradeFreeVipLevel, 0, int64(data.Level))
	s.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	s.owner.SetExtraAttr(attrdef.FreeVipLv, attrdef.AttrValueAlias(data.Level))
	// 重置任务
	s.ResetQuest()

	s.ResetSysAttr(attrdef.SaFreeVip)

	// 通知前端
	updateLvMsg := &pb3.S2C_7_21{}
	updateLvMsg.Level = data.Level
	updateLvMsg.FinishQuestIds = data.FinishQuestIds
	updateLvMsg.Quests = data.Quests
	s.owner.SendProto3(7, 21, updateLvMsg)
	//发送传闻公告
	engine.BroadcastTipMsgById(lvConf.Tips, s.owner.GetName(), lvConf.Name)
}

func (s *FreeVipSys) c2sBuyBag(msg *base.Message) {
	var req pb3.C2S_7_25
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	lv := req.GetLevel()
	data := s.getData()
	if lv <= 0 || lv > data.GetLevel() {
		return
	}

	conf := jsondata.GetFreeVipLvConf(lv)
	if conf == nil {
		return
	}

	for _, boughtLv := range data.BoughtBagLvs {
		if boughtLv == lv {
			return
		}
	}

	if !s.owner.ConsumeByConf(conf.BuyGift.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFreeVipBuyGift}) {
		return
	}

	s.LogPlayerBehavior(uint64(req.Level), map[string]interface{}{}, pb3.LogId_LogFreeVipBuyBag)
	data.BoughtBagLvs = append(data.BoughtBagLvs, lv)

	engine.GiveRewards(s.owner, conf.BuyGift.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFreeVipBuyGift})

	resp := &pb3.S2C_7_25{}
	resp.BoughtBagLvs = data.BoughtBagLvs
	s.owner.SendProto3(7, 25, resp)

	engine.BroadcastTipMsgById(tipmsgid.XianzhiPrivilege, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, conf.BuyGift.Awards))

}

func calcFreeVipSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys)
	if !sys.IsOpen() {
		return
	}

	data := sys.getData()
	if nil == data {
		return
	}
	if data.GetLevel() <= 0 { // 0级的时候读这个属性哇~ >0的时候就不用了哇~
		confZero := jsondata.GetFreeVipZeroConf()
		engine.AddAttrsToCalc(player, calc, confZero.Attrs)
	} else {
		conf := jsondata.GetFreeVipLvConf(data.GetLevel())
		if nil == conf {
			return
		}
		engine.AddAttrsToCalc(player, calc, conf.Attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiFreeVip, newFreeVipSys)
	engine.RegAttrCalcFn(attrdef.SaFreeVip, calcFreeVipSysAttr)

	net.RegisterSysProto(7, 20, sysdef.SiFreeVip, (*FreeVipSys).c2sFreeVipState)
	net.RegisterSysProto(7, 23, sysdef.SiFreeVip, (*FreeVipSys).c2sRewardQuest)
	net.RegisterSysProto(7, 24, sysdef.SiFreeVip, (*FreeVipSys).c2sGetLvAward)
	net.RegisterSysProto(7, 25, sysdef.SiFreeVip, (*FreeVipSys).c2sBuyBag)

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		freeVipSys, ok := player.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys)
		if !ok || !freeVipSys.IsOpen() {
			return
		}
		freeVipLv := freeVipSys.getData().Level
		if len(conf.FreeVip) == 0 {
			return
		}
		return int64(conf.FreeVip[freeVipLv]), nil
	})

	engine.RegQuestTargetProgress(custom_id.QttFreeVipFinishQuest, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) <= 0 {
			return 0
		}
		freeVipLv := ids[0]
		data := actor.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys).getData()
		curLevel := data.GetLevel()
		if freeVipLv <= curLevel {
			conf := jsondata.GetFreeVipLvConf(freeVipLv)
			if conf != nil {
				return uint32(len(conf.QuestList))
			}
		} else if freeVipLv == (curLevel + 1) {
			return uint32(len(data.GetFinishQuestIds()))
		}
		return 0
	})

	gmevent.Register("fv.finish", func(actor iface.IPlayer, args ...string) bool {
		if sys, ok := actor.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys); ok {
			data := sys.getData()
			if nil == data {
				return false
			}
			conf := jsondata.GetFreeVipLvConf(data.GetLevel() + 1)
			if nil == conf {
				return false
			}
			index := utils.AtoUint32(args[0])
			if len(data.Quests) < int(index) {
				return false
			}
			questData := data.Quests[index-1]
			questConf := conf.QuestList[index-1]
			questData.Progress = make([]uint32, 0)
			for _, v := range questConf.Targets {
				questData.Progress = append(questData.Progress, v.Count)
				data.PassQuestIds = pie.Uint32s(data.PassQuestIds).Append(questConf.QuestId).Unique()
			}
			msg := &pb3.S2C_7_22{}
			msg.Quest = questData
			sys.SendProto3(7, 22, msg)
			return true
		}
		return false
	}, 1)

	gmevent.Register("fv.uplv", func(actor iface.IPlayer, args ...string) bool {
		if sys, ok := actor.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys); ok {
			data := sys.getData()
			if nil == data {
				return false
			}
			lv := utils.AtoUint32(args[0])
			data.Level = lv
			sys.ResetQuest()
			sys.owner.TriggerEvent(custom_id.AeFreeUpLv, lv)
			sys.ResetSysAttr(attrdef.SaFreeVip)
			sys.owner.SetExtraAttr(attrdef.FreeVipLv, attrdef.AttrValueAlias(sys.getData().Level))
			sys.S2cFreeVipState()
			return true
		}
		return false
	}, 1)

	gmevent.Register("fv.c2sGetLvAward", func(actor iface.IPlayer, args ...string) bool {
		if sys, ok := actor.GetSysObj(sysdef.SiFreeVip).(*FreeVipSys); ok {
			msg := base.NewMessage()
			msg.SetCmd(7<<8 | 24)
			err := msg.PackPb3Msg(&pb3.C2S_7_24{
				Level: utils.AtoUint32(args[0]),
			})
			if err != nil {
				actor.LogError(err.Error())
				return false
			}
			sys.c2sGetLvAward(msg)
			return true
		}
		return false
	}, 1)
}
