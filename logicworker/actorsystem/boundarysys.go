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
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

/*
	desc:境界系统
	author: LvYuMeng
*/

var (
	BoundaryQuestTargetMap map[uint32]map[uint32]struct{} // 境界任务事件对应的id
)

type BoundarySystem struct {
	*QuestTargetBase
}

func newBoundarySys() iface.ISystem {
	sys := &BoundarySystem{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (sys *BoundarySystem) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.Boundary {
		binary.Boundary = &pb3.BoundaryInfo{}
	}
}

func (sys *BoundarySystem) OnOpen() {
	sys.CheckResetQuest()
}

func (sys *BoundarySystem) OnLogin() {
	binary := sys.GetBinaryData()
	if nil != binary.Boundary {
		sys.owner.SetExtraAttr(attrdef.BoundaryLevel, int64(sys.GetBinaryData().Boundary.Level))
	}
	sys.CheckResetQuest()
}

func (sys *BoundarySystem) OnAfterLogin() {
	binary := sys.GetBinaryData()
	sys.SendProto3(2, 20, &pb3.S2C_2_20{Boundary: binary.Boundary})
}

func (sys *BoundarySystem) OnReconnect() {
	binary := sys.GetBinaryData()
	sys.SendProto3(2, 20, &pb3.S2C_2_20{Boundary: binary.Boundary})
}

func (sys *BoundarySystem) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := BoundaryQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

func (sys *BoundarySystem) getUnFinishQuestData(id uint32) *pb3.QuestData {
	binary := sys.GetBinaryData()
	for _, quest := range binary.Boundary.Quests {
		if quest.GetId() == id {
			return quest
		}
	}
	return nil
}

func (sys *BoundarySystem) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	if conf := jsondata.GetBoundaryQuestConfById(id); nil != conf {
		return conf.Targets
	}
	return nil
}

func (sys *BoundarySystem) onUpdateTargetData(questId uint32) {
	quest := sys.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	//下发新增任务
	sys.SendProto3(2, 22, &pb3.S2C_2_22{Quest: quest})

}

func (sys *BoundarySystem) NotifyRemainPoint() {
	sys.SendProto3(2, 21, &pb3.S2C_2_21{Point: sys.GetBinaryData().Boundary.Point})
}

func (sys *BoundarySystem) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_2_24
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	boundary := sys.GetBinaryData().Boundary
	ntLv := boundary.GetLevel() + 1
	lvConf := jsondata.GetBoundaryLvConf(ntLv)
	if nil == lvConf {
		return neterror.ParamsInvalidError("boundary lv conf not found: %v", ntLv)
	}
	// 是否已完成所有任务
	if !sys.CheckFinishAllQuest() {
		return nil
	}
	boundary.Level = ntLv
	sys.owner.SetExtraAttr(attrdef.BoundaryLevel, int64(ntLv))
	sys.SendProto3(2, 24, &pb3.S2C_2_24{Level: ntLv})
	//sys.owner.TriggerEvent(custom_id.AeBoundaryChange, boundary.GetLevel())
	sys.owner.TriggerQuestEvent(custom_id.QttReachJingJieLayer, 0, int64(boundary.GetLevel()))
	if nil != jsondata.GetBoundaryLvConf(ntLv+1) {
		boundary.Quests = nil
		sys.CheckResetQuest()
	}
	sys.owner.SetRankValue(gshare.RankTypeBoundary, int64(boundary.Level))
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeBoundary, sys.owner.GetId(), int64(boundary.Level))
	sys.ResetSysAttr(attrdef.SaBoundary)
	oldPoint := boundary.Point
	boundary.Point += lvConf.Point
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldPoint": oldPoint,
		"newPoint": boundary.Point,
		"newLevel": boundary.Level,
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogBoundaryLevelUp, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	sys.NotifyRemainPoint()
	if lvConf.Broadcast > 0 {
		engine.BroadcastTipMsgById(tipmsgid.TpXianjieUpTip, sys.owner.GetId(), sys.owner.GetName(), lvConf.Level, sys.owner.GetFlyCamp())
	}
	return nil
}

func (sys *BoundarySystem) CheckResetQuest() {
	boundary := sys.GetBinaryData().Boundary
	if nil == boundary {
		return
	}

	if len(boundary.Quests) > 0 {
		return
	}

	level := boundary.GetLevel() + 1
	// 读下一级的任务
	conf := jsondata.GetBoundaryLvConf(level)
	if nil == conf {
		return
	}

	boundary.Quests = make([]*pb3.QuestData, 0, len(conf.Ids))
	for _, id := range conf.Ids {
		if questConf := jsondata.GetBoundaryQuestConfById(id); nil != questConf {
			quest := &pb3.QuestData{
				Id: id,
			}
			boundary.Quests = append(boundary.Quests, quest)
		}
	}

	for _, quest := range boundary.Quests {
		sys.QuestTargetBase.OnAcceptQuest(quest)
	}

	sys.SendProto3(2, 23, &pb3.S2C_2_23{Quests: boundary.Quests})
}

func (sys *BoundarySystem) c2sTalentUp(msg *base.Message) error {
	var req pb3.C2S_2_25
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	boundary := sys.GetBinaryData().Boundary
	point := boundary.GetPoint()
	if point < jsondata.BoundaryConfMgr.LimitPoint {
		return nil
	}

	talentId := req.GetId()
	conf := jsondata.GetBoundaryTalentConfById(talentId)
	if nil == conf {
		return neterror.ParamsInvalidError("boundary talent conf not found: %v", talentId)
	}

	var talent *pb3.KeyValue
	for _, line := range boundary.Talents {
		if line.GetKey() == talentId {
			talent = line
			break
		}
	}
	// 最大等级
	if nil != talent && talent.GetValue() >= jsondata.BoundaryConfMgr.MaxLv {
		return nil
	}

	if nil == talent {
		talent = &pb3.KeyValue{Key: talentId}
		boundary.Talents = append(boundary.Talents, talent)
	}
	oldPoint := boundary.Point
	boundary.Point -= jsondata.BoundaryConfMgr.LimitPoint
	talent.Value = talent.GetValue() + 1
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldPoint":  oldPoint,
		"newPoint":  boundary.Point,
		"newTalent": talent.GetValue(),
	})

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogBoundaryTalentUp, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	sys.SendProto3(2, 25, &pb3.S2C_2_25{
		Talent: talent,
	})
	sys.NotifyRemainPoint()
	sys.ResetSysAttr(attrdef.SaBoundaryTalent)
	return nil
}

func (sys *BoundarySystem) c2sTalentReset(msg *base.Message) {
	boundary := sys.GetBinaryData().Boundary
	if len(boundary.Talents) <= 0 {
		return
	}
	if nil == jsondata.BoundaryConfMgr {
		return
	}
	if !sys.owner.ConsumeByConf(jsondata.BoundaryConfMgr.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBoundaryTalentReset}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	var point uint32
	point = boundary.Point
	pointUnit := jsondata.BoundaryConfMgr.LimitPoint
	for _, talent := range boundary.Talents {
		point += talent.Value * pointUnit
	}

	boundary.Talents = nil
	oldPoint := boundary.Point
	boundary.Point = point

	logArg, _ := json.Marshal(map[string]interface{}{
		"oldPoint": oldPoint,
		"newPoint": boundary.Point,
		"src":      "talentReset",
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogBoundaryTalentReset, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	sys.SendProto3(2, 26, &pb3.S2C_2_26{})
	sys.NotifyRemainPoint()
	sys.ResetSysAttr(attrdef.SaBoundaryTalent)
}

func (sys *BoundarySystem) CheckFinishAllQuest() bool {
	binary := sys.GetBinaryData()

	if len(binary.Boundary.Quests) <= 0 {
		return false
	}

	for _, data := range binary.Boundary.Quests {
		if !sys.CheckFinishQuest(data) {
			return false
		}
	}
	return true
}

func onAfterReloadBoundaryConf(args ...interface{}) {
	tmp := make(map[uint32]map[uint32]struct{})
	if nil == jsondata.BoundaryConfMgr {
		return
	}
	for id, quest := range jsondata.BoundaryConfMgr.Quests {
		for _, target := range quest.Targets {
			if _, ok := tmp[target.Type]; !ok {
				tmp[target.Type] = make(map[uint32]struct{})
			}
			tmp[target.Type][id] = struct{}{}
		}
	}
	BoundaryQuestTargetMap = tmp
}

func calcBoundaryAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	if nil == binary || nil == binary.Boundary {
		return
	}
	conf := jsondata.GetBoundaryLvConf(binary.Boundary.GetLevel())
	if nil == conf {
		return
	}
	engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
}

func calcBoundaryTalentAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	if nil == binary || nil == binary.Boundary {
		return
	}
	for _, talent := range binary.Boundary.Talents {
		id, level := talent.GetKey(), talent.GetValue()
		conf := jsondata.GetBoundaryTalentConfById(id)
		if nil == conf {
			continue
		}
		engine.CheckAddAttrsToCalcTimes(player, calc, conf.Attrs, level)
	}
}

func init() {
	RegisterSysClass(sysdef.SiBoundary, newBoundarySys)

	engine.RegAttrCalcFn(attrdef.SaBoundary, calcBoundaryAttr)
	engine.RegAttrCalcFn(attrdef.SaBoundaryTalent, calcBoundaryTalentAttr)

	net.RegisterSysProto(2, 24, sysdef.SiBoundary, (*BoundarySystem).c2sLevelUp)
	net.RegisterSysProto(2, 25, sysdef.SiBoundary, (*BoundarySystem).c2sTalentUp)
	net.RegisterSysProto(2, 26, sysdef.SiBoundary, (*BoundarySystem).c2sTalentReset)
	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadBoundaryConf)

	gmevent.Register("boundary", func(actor iface.IPlayer, args ...string) bool {
		if len(args) <= 0 {
			return false
		}
		sys, ok := actor.GetSysObj(sysdef.SiBoundary).(*BoundarySystem)
		if !ok {
			return false
		}
		boundary := sys.GetBinaryData().Boundary
		if nil == boundary {
			return false
		}
		switch args[0] {
		case "task":
			taskId := utils.AtoUint32(args[1])
			for _, quest := range boundary.Quests {
				if quest.Id == taskId {
					sys.GmFinishQuest(quest)
				}
			}
		case "lv":
			lv := utils.AtoUint32(args[1])
			boundary.Level = lv
			sys.owner.SetExtraAttr(attrdef.BoundaryLevel, int64(lv))
			sys.SendProto3(2, 24, &pb3.S2C_2_24{Level: lv})
			//sys.owner.TriggerEvent(custom_id.AeBoundaryChange, boundary.GetLevel())
			sys.owner.TriggerQuestEvent(custom_id.QttReachJingJieLayer, 0, int64(boundary.GetLevel()))
			if nil != jsondata.GetBoundaryLvConf(lv+1) {
				boundary.Quests = nil
				sys.CheckResetQuest()
			}

			sys.ResetSysAttr(attrdef.SaBoundary)
		}
		return true
	}, 1)
}
