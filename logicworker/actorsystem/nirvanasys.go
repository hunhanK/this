/**
 * @Author: zjj
 * @Date: 2024/9/23
 * @Desc: 涅槃
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
)

type NirvanaSys struct {
	*QuestTargetBase
}

func createNirvanaSys() iface.ISystem {
	sys := &NirvanaSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func (s *NirvanaSys) OnInit() {
	s.setMaxRoleLv()
}

func (s *NirvanaSys) setMaxRoleLv() { //初始化角色最大等级
	data := s.getData()
	owner := s.GetOwner()
	nirvanaConf := jsondata.GetNirvanaLvConf(data.Lv)
	if nirvanaConf == nil {
		owner.LogError("not found %d nirvana lv conf", data.Lv)
		return
	}
	obj := owner.GetSysObj(sysdef.SiLevel)
	if obj == nil || !obj.IsOpen() {
		return
	}
	obj.(*LevelSys).SetMaxLv(nirvanaConf.MaxRoleLv)
}

func (s *NirvanaSys) setExtAttr() {
	owner := s.GetOwner()
	data := s.getData()
	make64 := utils.Make64(data.SubLv, data.Lv)
	owner.SetExtraAttr(attrdef.NirvanaLvAndSubLv, attrdef.AttrValueAlias(make64))
}

func (s *NirvanaSys) getQuestIdSet(qtt uint32) map[uint32]struct{} { //任务id合集获取
	var set = make(map[uint32]struct{})
	owner := s.GetOwner()
	mgr := jsondata.GetNirvanaQuestMgr()
	if mgr == nil {
		owner.LogError("not found quest conf")
		return set
	}
	for _, conf := range mgr { //遍历配置表
		var exist bool
		for _, target := range conf.Targets { //找出每个任务中目标
			if target.Type != qtt { //匹配目标
				continue
			}
			exist = true
			break
		}
		if exist {
			set[conf.Id] = struct{}{} //把这个任务加入任务合集
		}
	}
	return set
}

func (s *NirvanaSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()

	fData, ok := data.QuestMap[id]
	if !ok {
		return nil
	}

	if s.CheckFinishQuest(fData.Quest) {
		return nil
	}

	return fData.Quest
}

func (s *NirvanaSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetNirvanaQuest(id)
	owner := s.GetOwner()
	if conf == nil {
		owner.LogError("%d quest not found", id)
		return nil
	}
	return conf.Targets //任务目标列表：完成任务需达成的条件
}

func (s *NirvanaSys) onUpdateTargetData(id uint32) { //更新任务数据
	data := s.getData()
	fData, ok := data.QuestMap[id] //获取该任务的数据
	if !ok {
		return
	}
	if !fData.IsFinished && s.CheckFinishQuest(fData.Quest) { //任务还在未完成状态，但实际上完成了（刚完成）
		fData.IsFinished = true
		nirvanaQuest := jsondata.GetNirvanaQuest(id)              //获取任务的配置数据
		if nirvanaQuest != nil && len(nirvanaQuest.Awards) == 0 { //若任务配置中没有奖励则直接将任务标记为 “已领取”
			fData.IsReceived = true
		}
		s.checkAcceptNewQuest(id)         //检查并解锁新任务
		s.ResetSysAttr(attrdef.SaNirvana) //刷新相关属性
	}
	s.SendProto3(153, 51, &pb3.S2C_153_51{ //同步到客户端
		Quest: fData,
	})
}

func (s *NirvanaSys) s2cInfo() {
	s.SendProto3(153, 40, &pb3.S2C_153_40{
		Data: s.getData(),
	})
}

func (s *NirvanaSys) s2cQuestMap() {
	data := s.getData()
	s.SendProto3(153, 52, &pb3.S2C_153_52{
		QuestMap: data.QuestMap,
	})
}

func (s *NirvanaSys) getData() *pb3.NirvanaData {
	data := s.GetBinaryData().NirvanaData
	if data == nil {
		s.GetBinaryData().NirvanaData = &pb3.NirvanaData{}
		data = s.GetBinaryData().NirvanaData
	}
	if data.QuestMap == nil {
		data.QuestMap = make(map[uint32]*pb3.QuestFData)
	}
	return data
}

func (s *NirvanaSys) OnReconnect() {
	s.setExtAttr()
	s.s2cInfo()
}

func (s *NirvanaSys) OnLogin() {
	s.setExtAttr()
	s.setMaxRoleLv()
	s.checkAcceptNewQuest(0)
}

func (s *NirvanaSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *NirvanaSys) OnOpen() {
	s.setExtAttr()
	s.setMaxRoleLv()
	s.checkAcceptNewQuest(0)
	s.ResetSysAttr(attrdef.SaNirvana)
	s.s2cInfo()
}

func (s *NirvanaSys) learnSkill() {
	data := s.getData()
	lvConf := jsondata.GetNirvanaLvConf(data.Lv)
	if lvConf == nil {
		return
	}
	owner := s.GetOwner()
	for _, id := range lvConf.SkillIds {
		skillLv := owner.GetSkillLv(id)
		if skillLv != 0 {
			continue
		}
		owner.LearnSkill(id, 1, true)
	}
}

func (s *NirvanaSys) checkCond(cond *jsondata.NirvanaLvCond) bool {
	owner := s.GetOwner()
	if cond.OpenSrvDay != 0 && gshare.GetOpenServerDay() < cond.OpenSrvDay {
		return false
	}
	if cond.TaskId != 0 && owner.GetBinaryData().FinMainQuestId < cond.TaskId {
		return false
	}
	if cond.Level != 0 && owner.GetLevel() < cond.Level {
		return false
	}
	return true
}

// 检查和突破等级
func (s *NirvanaSys) checkAndBreakLv() {
	data := s.getData()
	owner := s.GetOwner()
	nextLv := data.Lv + 1
	subLv := data.SubLv //子任务

	nextLvConf := jsondata.GetNirvanaLvConf(nextLv)
	if nextLvConf == nil {
		owner.LogWarn("%d not found conf", nextLv)
		return
	}

	// 检查任务是否都完成
	var completeQuest = true
	for _, subLvConf := range nextLvConf.SubLvConf { //下一个等级的子任务配置
		if subLv >= subLvConf.Level {
			continue
		}
		ids := jsondata.GetNirvanaQuestIdsByLvSubLv(nextLv, subLvConf.Level) //获取子等级的任务id
		for _, id := range ids {                                             //检查每个子等级下的所有任务是否都已完成（IsFinished）并领取奖励（IsReceived）
			quest, ok := data.QuestMap[id]
			if ok {
				if !quest.IsFinished || !quest.IsReceived {
					completeQuest = false
					break
				}
				continue
			}
			completeQuest = false
			break
		}
		if !completeQuest {
			break
		}
	}
	if !completeQuest {
		return
	}

	cond := nextLvConf.Cond //获取下一级的特殊条件配置
	// 突破条件
	if cond != nil && !s.checkCond(cond) { //通过 checkCond 方法验证玩家是否满足这些条件
		return
	}

	data.Lv = nextLv
	data.SubLv = 0

	// 变化要立即写入 否则校验系统开启时拿到的是旧的
	s.setExtAttr()
	s.setMaxRoleLv()
	s.learnSkill()
	// 先清理任务
	s.rmBeforeLvQuest()
	s.checkAcceptNewQuest(0)
	s.ResetSysAttr(attrdef.SaNirvana)
	if len(nextLvConf.Awards) != 0 {
		engine.GiveRewards(owner, nextLvConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNirvanaBreakLv})
	}
	s.SendProto3(153, 44, &pb3.S2C_153_44{
		Lv:    data.Lv,
		SubLv: data.SubLv,
	})

	engine.BroadcastTipMsgById(tipmsgid.NirvanaLvUpTip, owner.GetId(), owner.GetName(), nextLvConf.Name)
	owner.TriggerEvent(custom_id.AeNirvanaLvChange, data.Lv)
	owner.TriggerQuestEvent(custom_id.QttNirvanaXLevel, 0, int64(data.Lv))
	owner.TriggerQuestEventRange(custom_id.QttNirvana)
	s.GetOwner().UpdateStatics(model.FieldNirvanaLv_, data.Lv)
	s.GetOwner().UpdateStatics(model.FieldNirvanaSubLv_, data.SubLv)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogNirvanaBreakLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextLv),
	})
}

// 检查和突破子阶段
func (s *NirvanaSys) checkAndBreakSubLv() {
	data := s.getData()
	nextLv := data.Lv + 1
	nextSubLv := data.SubLv + 1

	// 检查任务是否都完成
	var completeQuest = true
	ids := jsondata.GetNirvanaQuestIdsByLvSubLv(nextLv, nextSubLv)
	for _, id := range ids {
		quest, ok := data.QuestMap[id]
		if ok {
			if !quest.IsFinished || !quest.IsReceived {
				completeQuest = false
				break
			}
			continue
		}
		completeQuest = false
		break
	}

	if !completeQuest {
		return
	}

	if !s.checkNextCond(nextLv, nextSubLv) { //调用 checkNextCond 验证突破的额外条件（如开服天数、主线任务进度、玩家等级）
		return
	}

	data.SubLv = nextSubLv
	s.setExtAttr()
	s.checkAcceptNewQuest(0)
	s.SendProto3(153, 45, &pb3.S2C_153_45{
		Lv:    data.Lv,
		SubLv: data.SubLv,
	})
	s.owner.TriggerQuestEventRange(custom_id.QttNirvana)
	s.GetOwner().UpdateStatics(model.FieldNirvanaLv_, data.Lv)
	s.GetOwner().UpdateStatics(model.FieldNirvanaSubLv_, data.SubLv)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogNirvanaBreakSubLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.Lv),
		StrArgs: fmt.Sprintf("%d_%d", nextLv, nextSubLv),
	})
}

func (s *NirvanaSys) rmBeforeLvQuest() { //用于清理玩家已过时的涅槃任务数据（当前主等级之前的任务）
	data := s.getData()
	lv := data.Lv
	var ids []uint32
	for id := range data.QuestMap {
		questConf := jsondata.GetNirvanaQuest(id)
		if questConf == nil {
			continue
		}
		if questConf.NirvanaLv > lv {
			continue
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		delete(data.QuestMap, id)
	}
}

// 检查下一级的条件，用于验证子等级突破的额外限制条件
func (s *NirvanaSys) checkNextCond(nextLv, nextSubLv uint32) bool {
	nextLvConf := jsondata.GetNirvanaLvConf(nextLv)
	if nextLvConf == nil {
		return false
	}

	nextSubLvConf := nextLvConf.SubLvConf[nextSubLv]
	if nextSubLvConf == nil {
		return false
	}

	cond := nextSubLvConf.Cond
	if cond != nil && !s.checkCond(cond) {
		return false
	}
	return true
}

// 检查是否可以接新任务
func (s *NirvanaSys) checkAcceptNewQuest(parentQuestId uint32) {
	owner := s.GetOwner()                          //玩家实体
	data := s.getData()                            //涅槃数据
	var acceptNewQuest bool                        // 是否接取到新任务的标记
	var newIds []uint32                            // 新接取任务的ID列表
	var acceptNewQuestByIds = func(ids []uint32) { //批量处理任务接取
		for _, id := range ids { //遍历待接取的任务列表
			quest := jsondata.GetNirvanaQuest(id) //获取任务配置
			if quest == nil {
				owner.LogWarn("not found %d quest conf", id)
				continue
			}
			// 校验父任务条件：若任务有父任务，需与触发的父任务ID一致
			if quest.ParentQuestId != 0 && quest.ParentQuestId != parentQuestId {
				continue
			}
			// 校验是否已接取：避免重复接取同一任务
			_, ok := data.QuestMap[id]
			if ok {
				continue
			}
			// 接取任务：初始化任务数据并添加到玩家任务列表
			fData := &pb3.QuestFData{
				Quest: &pb3.QuestData{
					Id: id,
				},
			}
			data.QuestMap[id] = fData

			newIds = append(newIds, id)
			acceptNewQuest = true

			s.OnAcceptQuest(fData.Quest)         // 触发任务接取回调（如初始化进度）
			if s.CheckFinishQuest(fData.Quest) { //检查任务是否接取时已完成（如玩家已满足目标,直接标记
				fData.IsFinished = true
			}
		}
	}

	ids := jsondata.GetNirvanaQuestIdsByParentId(parentQuestId) // 获取与父任务ID关联的任务列表（任务链接取）
	acceptNewQuestByIds(ids)                                    // 调用工具函数接取这些任务
	// 计算下一级涅槃等级和子等级（当前进度的下一个阶段）
	nextLv := data.Lv + 1
	nextSubLv := data.SubLv + 1
	// 检查下一级是否满足接取条件（如开服天数、主线进度等）
	if !s.checkNextCond(nextLv, nextSubLv) {
		return
	}
	// 获取下一级等级/子等级对应的任务列表
	ids = jsondata.GetNirvanaQuestIdsByLvSubLv(nextLv, nextSubLv)
	acceptNewQuestByIds(ids) //// 调用工具函数接取这些任务
	// 若有新任务接取清理历史任务（删除低于当前等级的任务） 同步更新后的任务列表到客户端
	if acceptNewQuest {
		s.rmBeforeLvQuest()
		s.s2cQuestMap()
	}
	if len(newIds) != 0 {
		logworker.LogPlayerBehavior(owner, pb3.LogId_LogNirvanaAcceptNewQuest, &pb3.LogPlayerCounter{
			NumArgs: uint64(parentQuestId),
			StrArgs: pie.Uint32s(newIds).JSONString(),
		})
	}
}

func (s *NirvanaSys) c2sConsumeToQuest(msg *base.Message) error { //任务加速完成
	var req pb3.C2S_153_41
	err := msg.UnpackagePbmsg(&req) //解析id，验证数据
	if err != nil {
		return err
	}

	questId := req.QuestId
	quest := jsondata.GetNirvanaQuest(questId)
	if quest == nil {
		return neterror.ConfNotFoundError("%d not found quest conf", questId)
	}

	data := s.getData()
	fData, ok := data.QuestMap[questId] //检查玩家是否已接取该任务
	if !ok {
		return neterror.ConfNotFoundError("%d not accept quest data", questId)
	}
	//确保任务未完成（避免重复消耗）
	if fData.IsFinished {
		return neterror.ParamsInvalidError("%d quest is finish", questId)
	}
	owner := s.GetOwner()
	//按配置消耗资源
	if len(quest.Consume) != 0 && !owner.ConsumeByConf(quest.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogNirvanaConsumeCompleteQuest,
	}) {
		return neterror.ConsumeFailedError("%d consume failed", questId)
	}
	//触发任务返回事件
	owner.TriggerQuestEvent(custom_id.QttNirvanaCompleteConsume, questId, 1)
	//同步客户端
	s.SendProto3(153, 41, &pb3.S2C_153_41{
		QuestId: questId,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNirvanaConsumeCompleteQuest, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.QuestId),
	})
	return nil
}

func (s *NirvanaSys) c2sRecQuestAwards(msg *base.Message) error {
	var req pb3.C2S_153_42
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	return s.recQuestAwards(req.QuestId)
}

func (s *NirvanaSys) recQuestAwards(questId uint32) error {
	quest := jsondata.GetNirvanaQuest(questId)
	if quest == nil {
		return neterror.ConfNotFoundError("%d not found quest conf", questId)
	}

	data := s.getData()
	fData, ok := data.QuestMap[questId] //玩家已接取任务且任务已完成
	if !ok {
		return neterror.ConfNotFoundError("%d not accept quest data", questId)
	}

	if !fData.IsFinished {
		return neterror.ParamsInvalidError("%d quest is finish", questId)
	}
	//奖励未被领取
	if fData.IsReceived {
		return neterror.ParamsInvalidError("%d quest is received", questId)
	}

	owner := s.GetOwner()
	if len(quest.Awards) != 0 { //发奖励
		engine.GiveRewards(owner, quest.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNirvanaCompleteQuestAwards})
	}

	fData.IsReceived = true //标记奖励领取
	s.SendProto3(153, 42, &pb3.S2C_153_42{
		QuestId: questId,
	})
	s.checkAcceptNewQuest(questId) //检查新任务

	if len(quest.Attrs) > 0 { //若任务包含属性加成（quest.Attrs），重置系统属性（如战力）
		s.ResetSysAttr(attrdef.SaNirvana)
	}
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNirvanaCompleteQuestAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(questId),
	})
	return nil
}

// 副本挑战
func (s *NirvanaSys) c2sAttackFb(_ *base.Message) error {
	data := s.getData()
	nextLv := data.Lv + 1
	lvConf := jsondata.GetNirvanaLvConf(nextLv)
	if lvConf == nil {
		return neterror.ConfNotFoundError("not found %d lv conf", nextLv)
	}

	if lvConf.BreakFbLayer <= data.PassFbLayer {
		return neterror.ParamsInvalidError("pass fb layer %d %d", nextLv, data.PassFbLayer)
	}

	owner := s.GetOwner()
	err := owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterNirvanaFb, &pb3.CommonSt{U32Param: lvConf.BreakFbLayer})
	if err != nil {
		owner.LogWarn("err:%v", err)
		return err
	}
	return nil
}

// 副本结算
func (s *NirvanaSys) handleSettlement(settle *pb3.FbSettlement) {
	if settle.Ret == custom_id.FbSettleResultWin {
		data := s.getData()
		owner := s.GetOwner()

		oldFbLayer := data.PassFbLayer
		data.PassFbLayer = settle.ExData[0]

		conf := jsondata.GetNirvanaFbConf()
		layerConf := conf.LayerMap[data.PassFbLayer]
		if len(layerConf.Rewards) != 0 && !engine.GiveRewards(owner, layerConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNirvanaAttackFbSuccess}) {
			owner.LogError("handleSettlement failed give reward failed %d", data.PassFbLayer)
		}

		settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(layerConf.Rewards)
		owner.TriggerQuestEvent(custom_id.QttNirvanaXPassFb, data.PassFbLayer, 1)
		logworker.LogPlayerBehavior(owner, pb3.LogId_LogNirvanaAttackFbSuccess, &pb3.LogPlayerCounter{
			NumArgs: uint64(oldFbLayer),
			StrArgs: fmt.Sprintf("%d", data.PassFbLayer),
		})
	}
	s.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settle})
	s.s2cInfo()
}

func (s *NirvanaSys) c2sBreakSubLv(_ *base.Message) error {
	s.checkAndBreakSubLv()
	return nil
}

func (s *NirvanaSys) c2sBreakLv(_ *base.Message) error {
	s.checkAndBreakLv()
	return nil
}

func (s *NirvanaSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	fData, ok := data.QuestMap[questId]
	if !ok {
		s.owner.LogError("not found %d quest", questId)
		return
	}
	if fData.IsFinished || fData.IsReceived {
		s.GmFinishQuest(fData.Quest)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(fData.Quest)
}

func (s *NirvanaSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.QuestMap, questId)

}

// 属性计算
func calcNirvanaProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiNirvana)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*NirvanaSys)
	if !ok {
		return
	}

	data := s.getData()
	lv := data.Lv
	lvConf := jsondata.GetNirvanaLvConf(lv)
	if lvConf != nil && len(lvConf.Attrs) != 0 {
		engine.CheckAddAttrsToCalc(player, calc, lvConf.Attrs)
	}

	mgr := jsondata.GetNirvanaQuestMgr()
	for _, conf := range mgr {
		if conf.NirvanaLv > lv {
			continue
		}
		if len(conf.Attrs) == 0 {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
	}

	for questId, fData := range data.QuestMap {
		if !fData.IsReceived {
			continue
		}
		quest := jsondata.GetNirvanaQuest(questId)
		if quest == nil {
			continue
		}
		if len(quest.Attrs) == 0 {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, quest.Attrs)
	}
}

// 副本结算
func handleNirvanaFbSettlement(player iface.IPlayer, buf []byte) {
	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("err:%v", err)
		return
	}
	obj := player.GetSysObj(sysdef.SiNirvana)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*NirvanaSys)
	if !ok {
		return
	}

	sys.handleSettlement(&req)
}

func nirvanaOnFlyCampFinish(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiNirvana)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*NirvanaSys)
	if !ok {
		return
	}
	sys.setMaxRoleLv()
}

func handleNirvanaAcceptNewQuest(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiNirvana)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*NirvanaSys)
	if !ok {
		return
	}
	sys.checkAcceptNewQuest(0)
}

func (s *NirvanaSys) GMFinishAll(questId uint32) error {
	owner := s.GetOwner()
	data := s.getData()
	var totalConsume jsondata.ConsumeVec
	var totalAwards jsondata.StdRewardVec
	var unRecQuestIds []uint32
	var unQuestIds []uint32
	nextLv := data.Lv + 1
	nextSubLv := data.SubLv + 1
	questIds := jsondata.GetNirvanaQuestIdsByLvSubLv(nextLv, nextSubLv)
	for _, tId := range questIds {
		toque := jsondata.GetNirvanaQuest(tId)
		if toque == nil || toque.XYConsume == nil || len(toque.XYConsume) == 0 {
			return neterror.ConfNotFoundError("not found %d quest conf", tId)
		}
		fData := data.QuestMap[tId]
		// 完成且领奖
		if fData != nil && fData.IsFinished && fData.IsReceived {
			continue
		}
		// 完成没领奖
		if fData != nil && fData.IsFinished && !fData.IsReceived {
			unRecQuestIds = append(unRecQuestIds, tId)
			continue
		}
		// 没完成 需要花钱完成
		totalConsume = append(totalConsume, toque.XYConsume...)
		totalAwards = append(totalAwards, toque.Awards...)
		unQuestIds = append(unQuestIds, tId)
	}

	// 先扣资源
	if len(totalConsume) == 0 || !owner.ConsumeByConf(totalConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogNirvanaConsumeCompleteQuest}) {
		return neterror.ConsumeFailedError("%d %d consume failed", nextLv, nextSubLv)
	}

	// 批量完成
	for _, id := range unQuestIds {
		fData := &pb3.QuestFData{
			Quest: &pb3.QuestData{
				Id: id,
			},
		}
		data.QuestMap[id] = fData
		s.GmFinishQuest(fData.Quest)
		fData.IsFinished = true
		fData.IsReceived = true
	}

	// 把没领奖的领奖
	for _, unRecQuestId := range unRecQuestIds {
		data.QuestMap[unRecQuestId].IsReceived = true
	}

	// 发奖励
	if len(totalAwards) != 0 {
		totalAwards = jsondata.MergeStdReward(totalAwards)
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNirvanaCompleteQuestAwards})
		owner.SendShowRewardsPop(totalAwards)
	}

	// 通知客户端
	s.SendProto3(153, 54, &pb3.S2C_153_54{
		QuestMap: data.QuestMap,
	})

	// 重算属性
	s.ResetSysAttr(attrdef.SaNirvana)
	s.checkAndBreakSubLv()

	return nil
}
func (s *NirvanaSys) c2sXYConsumeToQuest(msg *base.Message) error { //任务加速完成
	owner := s.GetOwner()
	data := s.getData()
	var totalConsume jsondata.ConsumeVec
	var totalAwards jsondata.StdRewardVec
	var unRecQuestIds []uint32
	var unQuestIds []uint32
	nextLv := data.Lv + 1
	nextSubLv := data.SubLv + 1
	questIds := jsondata.GetNirvanaQuestIdsByLvSubLv(nextLv, nextSubLv)
	for _, tId := range questIds {
		toque := jsondata.GetNirvanaQuest(tId)
		if toque == nil || toque.XYConsume == nil || len(toque.XYConsume) == 0 {
			return neterror.ConfNotFoundError("not found %d quest conf", tId)
		}
		fData := data.QuestMap[tId]
		// 完成且领奖
		if fData != nil && fData.IsFinished && fData.IsReceived {
			continue
		}
		// 完成没领奖
		if fData != nil && fData.IsFinished && !fData.IsReceived {
			unRecQuestIds = append(unRecQuestIds, tId)
			continue
		}
		// 没完成 需要花钱完成
		totalConsume = append(totalConsume, toque.XYConsume...)
		totalAwards = append(totalAwards, toque.Awards...)
		unQuestIds = append(unQuestIds, tId)
	}

	// 先扣资源
	if len(totalConsume) == 0 || !owner.ConsumeByConf(totalConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogNirvanaConsumeCompleteQuest}) {
		return neterror.ConsumeFailedError("%d %d consume failed", nextLv, nextSubLv)
	}

	// 批量完成
	for _, id := range unQuestIds {
		fData := &pb3.QuestFData{
			Quest: &pb3.QuestData{
				Id: id,
			},
		}
		data.QuestMap[id] = fData
		s.GmFinishQuest(fData.Quest)
		fData.IsFinished = true
		fData.IsReceived = true
	}

	// 把没领奖的领奖
	for _, unRecQuestId := range unRecQuestIds {
		data.QuestMap[unRecQuestId].IsReceived = true
	}

	// 发奖励
	if len(totalAwards) != 0 {
		totalAwards = jsondata.MergeStdReward(totalAwards)
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNirvanaCompleteQuestAwards})
		owner.SendShowRewardsPop(totalAwards)
	}

	// 通知客户端
	s.SendProto3(153, 54, &pb3.S2C_153_54{
		QuestMap: data.QuestMap,
	})

	// 重算属性
	s.ResetSysAttr(attrdef.SaNirvana)
	s.checkAndBreakSubLv()
	// 记录行为日志
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNirvanaConsumeCompleteQuest, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%d_%d_%v", nextLv, nextSubLv, unQuestIds),
	})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiNirvana, createNirvanaSys)
	net.RegisterSysProtoV2(153, 41, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sConsumeToQuest
	})
	net.RegisterSysProtoV2(153, 42, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sRecQuestAwards
	})
	net.RegisterSysProtoV2(153, 43, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sAttackFb
	})
	net.RegisterSysProtoV2(153, 44, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sBreakLv
	})
	net.RegisterSysProtoV2(153, 45, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sBreakSubLv
	})
	net.RegisterSysProtoV2(153, 54, sysdef.SiNirvana, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NirvanaSys).c2sXYConsumeToQuest
	})

	engine.RegAttrCalcFn(attrdef.SaNirvana, calcNirvanaProperty)
	engine.RegisterActorCallFunc(playerfuncid.NirvanaFbSettlement, handleNirvanaFbSettlement)
	engine.RegQuestTargetProgress(custom_id.QttNirvana, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) != 2 {
			return 0
		}
		sys, ok := actor.GetSysObj(sysdef.SiNirvana).(*NirvanaSys)
		if !ok || !sys.IsOpen() {
			return 0
		}

		data := sys.getData()
		// 跨大阶段
		if data.Lv > ids[0] {
			return 1
		}
		if data.Lv == ids[0] && data.SubLv >= ids[1] {
			return 1
		}
		return 0
	})
	event.RegActorEvent(custom_id.AeFlyCampFinishChallenge, nirvanaOnFlyCampFinish)
	event.RegActorEvent(custom_id.AeNewDay, handleNirvanaAcceptNewQuest)
	event.RegActorEvent(custom_id.AeLevelUp, handleNirvanaAcceptNewQuest)
	initNirvanaGm()
}

func initNirvanaGm() {
	gmevent.Register("SiNirvana.finishall", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiNirvana)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*NirvanaSys)
		if !ok {
			return false
		}

		s.GMFinishAll(utils.AtoUint32(args[0]))
		return true
	}, 1)
	gmevent.Register("SiNirvana.SetLvAndSubLv", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiNirvana)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*NirvanaSys)
		if !ok {
			return false
		}
		data := s.getData()
		data.Lv = utils.AtoUint32(args[0])
		data.SubLv = utils.AtoUint32(args[1])
		s.setExtAttr()
		player.TriggerEvent(custom_id.AeNirvanaLvChange, data.Lv)
		player.TriggerQuestEvent(custom_id.QttNirvanaXLevel, 0, int64(data.Lv))
		s.checkAcceptNewQuest(0)
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("SiNirvana.finishCurQuests", func(player iface.IPlayer, args ...string) bool { //强制完成当前所有任务
		obj := player.GetSysObj(sysdef.SiNirvana)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*NirvanaSys)
		if !ok {
			return false
		}
		data := s.getData()
		for _, fData := range data.QuestMap {
			if fData.IsFinished {
				continue
			}
			s.GmFinishQuest(fData.Quest)
		}
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("SiNirvana.recAllQuestAwards", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiNirvana)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*NirvanaSys)
		if !ok {
			return false
		}
		data := s.getData()
		for _, fData := range data.QuestMap {
			if fData.IsReceived {
				continue
			}
			msg := base.NewMessage()
			msg.SetCmd(153<<8 | 42)
			err := msg.PackPb3Msg(&pb3.C2S_153_42{
				QuestId: fData.Quest.Id,
			})
			if err != nil {
				player.LogError(err.Error())
				continue
			}
			player.DoNetMsg(153, 42, msg)
		}
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("SiNirvana.setPassBreakFbLayer", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiNirvana)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*NirvanaSys)
		if !ok {
			return false
		}
		data := s.getData()
		data.PassFbLayer = utils.AtoUint32(args[0])
		player.TriggerQuestEvent(custom_id.QttNirvanaXPassFb, 0, int64(data.PassFbLayer))
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("SiNirvana.attackFb", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(153<<8 | 43)
		err := msg.PackPb3Msg(&pb3.C2S_153_43{})
		if err != nil {
			player.LogError(err.Error())
			return false
		}
		player.DoNetMsg(153, 43, msg)
		return true
	}, 1)
}
