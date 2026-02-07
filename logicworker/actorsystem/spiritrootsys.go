/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 灵根
**/

package actorsystem

import (
	"encoding/json"
	"fmt"
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
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type SpiritRootSys struct {
	Base
}

func (s *SpiritRootSys) GetData() *pb3.SpiritRoot {
	binary := s.GetBinaryData()
	if binary.SpiritRoot == nil {
		binary.SpiritRoot = new(pb3.SpiritRoot)
	}
	if binary.SpiritRoot.SrLvMap == nil {
		binary.SpiritRoot.SrLvMap = make(map[uint32]uint32)
	}
	if binary.SpiritRoot.SrUpgradeMap == nil {
		binary.SpiritRoot.SrUpgradeMap = make(map[uint32]*pb3.SpiritRootUpgrade)
	}
	return binary.SpiritRoot
}

// GetSpiritRootUpgrade 获取进阶数据
func (s *SpiritRootSys) GetSpiritRootUpgrade(spiritRootId uint32) *pb3.SpiritRootUpgrade {
	data := s.GetData()
	upgradeMap := data.SrUpgradeMap
	upgrade, ok := upgradeMap[spiritRootId]
	if !ok {
		upgradeMap[spiritRootId] = &pb3.SpiritRootUpgrade{}
		upgrade = upgradeMap[spiritRootId]
	}
	if upgrade.PosMap == nil {
		upgrade.PosMap = make(map[uint32]*pb3.SpiritRootUpgradePos)
	}
	return upgrade
}

func (s *SpiritRootSys) S2CInfo() {
	s.SendProto3(151, 1, &pb3.S2C_151_1{
		State: s.GetData(),
	})
}

func (s *SpiritRootSys) OnLogin() {
	s.S2CInfo()
}

func (s *SpiritRootSys) OnReconnect() {
	s.S2CInfo()
}

func (s *SpiritRootSys) OnOpen() {
	s.S2CInfo()
}

func (s *SpiritRootSys) checkActorLv() bool {
	conf, ok := jsondata.GetSpiritRootConf()
	if !ok {
		return false
	}
	if s.GetOwner().GetLevel() < conf.OpenLv {
		return false
	}
	return true
}

// 激活
func (s *SpiritRootSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_151_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	err := s.activeSpiritRoot(req.SpiritRootId)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *SpiritRootSys) activeSpiritRoot(spiritRootId uint32) error {
	rootConf, ok := jsondata.GetSpiritRootConf()
	if !ok {
		return neterror.ConfNotFoundError("conf not found")
	}
	conf, ok := jsondata.GetSpecifySpiritRootConf(spiritRootId)
	if !ok {
		return neterror.ConfNotFoundError("conf %d not found", spiritRootId)
	}

	if !s.checkActorLv() {
		return neterror.ConsumeFailedError("lv not reachable")
	}

	data := s.GetData()
	owner := s.GetOwner()
	_, ok = data.SrLvMap[spiritRootId]
	if ok {
		return neterror.ConsumeFailedError("already active %d", spiritRootId)
	}

	// 激活
	if len(conf.ActivationConsume) > 0 {
		if !owner.ConsumeByConf(conf.ActivationConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveSpiritRoot}) {
			return neterror.ConsumeFailedError("consume failed")
		}
	}

	// 成功 升级后默认 1 级
	data.SrLvMap[spiritRootId] = 1
	owner.TriggerQuestEvent(custom_id.QttAnySpiritRootMaxLv, 0, 1)
	owner.TriggerQuestEvent(custom_id.QttSpecificSpiritRootMaxLv, spiritRootId, 1)
	owner.TriggerQuestEventRange(custom_id.QttAnyNSpiritRootReachYLv)
	owner.TriggerQuestEventRange(custom_id.QttUnLockNSpiritRoot)
	s.ResetSysAttr(attrdef.SaSpiritRoot)

	// 去广播
	if rootConf.ActiveBroadcastId > 0 {
		engine.BroadcastTipMsgById(rootConf.ActiveBroadcastId, owner.GetName(), conf.Name)
	}

	s.SendProto3(151, 3, &pb3.S2C_151_3{
		SpiritRootId: spiritRootId,
	})

	bytes, _ := json.Marshal(map[string]interface{}{
		"FromLevel":    0,
		"ToLevel":      1,
		"SpiritRootId": spiritRootId,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSpiritRootToLv, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})
	return nil
}

// 升级
func (s *SpiritRootSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_151_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	conf, ok := jsondata.GetSpecifySpiritRootConf(req.SpiritRootId)
	if !ok {
		s.GetOwner().LogWarn("not found conf")
		return nil
	}

	if !s.checkActorLv() {
		s.GetOwner().LogWarn("lv not reachable")
		return nil
	}

	data := s.GetData()
	curLv, ok := data.SrLvMap[req.SpiritRootId]
	if !ok {
		s.GetOwner().LogWarn("already active , id is %d", req.SpiritRootId)
		return nil
	}

	// 满级
	lastLvConf := conf.LevelConf[len(conf.LevelConf)-1]
	if curLv >= lastLvConf.Level {
		s.GetOwner().LogWarn("cur lv is max , level is %d , conf lv is %d", curLv, len(conf.LevelConf))
		return nil
	}

	var levelConf *jsondata.SpiritRootLevelConf
	for i := range conf.LevelConf {
		if conf.LevelConf[i].Level != curLv {
			continue
		}
		levelConf = conf.LevelConf[i]
		break
	}

	if levelConf == nil {
		s.GetOwner().LogWarn("not found level conf")
		return neterror.ConfNotFoundError("not found level conf")
	}

	// 升级
	owner := s.GetOwner()
	if len(levelConf.Consume) > 0 {
		if !owner.ConsumeByConf(levelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvSpiritRoot}) {
			s.GetOwner().LogWarn("consume failed")
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return nil
		}
	}

	var oldLv = curLv
	var newLv = curLv + 1
	data.SrLvMap[req.SpiritRootId] = newLv

	s.ResetSysAttr(attrdef.SaSpiritRoot)

	owner.TriggerQuestEvent(custom_id.QttAnySpiritRootMaxLv, 0, int64(newLv))
	owner.TriggerQuestEvent(custom_id.QttSpecificSpiritRootMaxLv, req.SpiritRootId, int64(newLv))
	owner.TriggerQuestEventRange(custom_id.QttAnyNSpiritRootReachYLv)
	s.SendProto3(151, 4, &pb3.S2C_151_4{
		SpiritRootId: req.SpiritRootId,
		CurLv:        newLv,
	})

	bytes, _ := json.Marshal(map[string]interface{}{
		"FromLevel":    oldLv,
		"ToLevel":      newLv,
		"SpiritRootId": req.SpiritRootId,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSpiritRootToLv, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})

	return nil
}

// 共鸣
func (s *SpiritRootSys) c2sUpResonance(msg *base.Message) error {
	var req pb3.C2S_151_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	conf, ok := jsondata.GetSpecifySpiritRootResonanceConf(req.ResonanceId)
	if !ok {
		s.GetOwner().LogWarn("not found conf")
		return nil
	}

	if !s.checkActorLv() {
		s.GetOwner().LogWarn("lv not reachable")
		return nil
	}

	data := s.GetData()

	if pie.Uint32s(data.ReachResonanceIds).Contains(conf.Id) {
		s.GetOwner().LogWarn("already reachable")
		return nil
	}

	var count uint32
	for _, rootUpgrade := range data.SrUpgradeMap {
		if rootUpgrade.PosMap == nil {
			continue
		}
		for _, posData := range rootUpgrade.PosMap {
			// 灵根吼位
			if posData.Type != 2 {
				continue
			}
			if posData.Level < conf.Lv {
				continue
			}
			count++
		}
	}

	if count < conf.SpiritRootNum {
		s.GetOwner().LogWarn("not reachable , conf num is %v, cur num %v", conf.SpiritRootNum, count)
		return nil
	}

	data.ReachResonanceIds = append(data.ReachResonanceIds, req.ResonanceId)

	// 去广播
	rootConf, ok := jsondata.GetSpiritRootConf()
	if ok {
		engine.BroadcastTipMsgById(rootConf.ResonanceBroadcastId, s.GetOwner().GetName(), conf.Name)
	}

	s.ResetSysAttr(attrdef.SaSpiritRoot)

	s.SendProto3(151, 5, &pb3.S2C_151_5{
		ReachResonanceIds: data.ReachResonanceIds,
	})

	bytes, _ := json.Marshal(map[string]interface{}{
		"ResonanceId":       req.ResonanceId,
		"ReachResonanceIds": data.ReachResonanceIds,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogSpiritRootReachResonance, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})

	return nil
}

// 灵根进阶
func (s *SpiritRootSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_151_10
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	spiritRootId := req.SpiritRootId
	pos := req.Pos

	conf, ok := jsondata.GetSpecifySpiritRootConf(spiritRootId)
	if !ok {
		s.GetOwner().LogWarn("not found %d conf", spiritRootId)
		return nil
	}

	upgradeData := s.GetSpiritRootUpgrade(spiritRootId)
	upgradePosConf := conf.UpgradeConf[pos]
	if upgradePosConf == nil {
		return neterror.ConfNotFoundError("%d %d not found upgrade conf", spiritRootId, pos)
	}

	// 初始化进阶信息
	upgradePos, ok := upgradeData.PosMap[pos]
	if !ok {
		upgradeData.PosMap[pos] = &pb3.SpiritRootUpgradePos{
			Pos:  pos,
			Type: upgradePosConf.Type,
		}
		upgradePos = upgradeData.PosMap[pos]
	}
	if upgradePos.SkillMap == nil {
		upgradePos.SkillMap = make(map[uint32]uint32)
	}

	var curLv = upgradePos.Level
	var nextLv = curLv + 1
	upgradeNextLvConf := upgradePosConf.UpgradeLevel[nextLv]
	if upgradeNextLvConf == nil {
		return neterror.ConfNotFoundError("%d %d not found next lv %d upgrade conf", spiritRootId, pos, nextLv)
	}

	// 技能校验
	skillIds := upgradeNextLvConf.SkillIds
	skillLvs := upgradeNextLvConf.SkillLvs
	if len(skillIds) != len(skillLvs) {
		return neterror.ConfNotFoundError("%d %d not found next lv %d upgrade skill conf have err", spiritRootId, pos, nextLv)
	}

	if !s.checkUpgradeNextLvCond(spiritRootId, upgradeNextLvConf) {
		return neterror.ConfNotFoundError("%d %d not found next lv %d upgrade conf not reach cond", spiritRootId, pos, nextLv)
	}

	if len(upgradeNextLvConf.Consume) != 0 && !s.GetOwner().ConsumeByConf(upgradeNextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritRootUpgrade}) {
		return neterror.ConfNotFoundError("%d %d not found next lv %d upgrade conf consume failed", spiritRootId, pos, nextLv)
	}

	upgradePos.Level = nextLv
	owner := s.GetOwner()
	for idx, id := range skillIds {
		lv := skillLvs[idx]
		upgradePos.SkillMap[id] = lv
		owner.LearnSkill(id, lv, true)
	}

	s.ResetSysAttr(attrdef.SaSpiritRoot)

	owner.SendProto3(151, 10, &pb3.S2C_151_10{
		SpiritRootId: spiritRootId,
		Pos:          pos,
		Lv:           nextLv,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSpiritRootUpgrade, &pb3.LogPlayerCounter{
		NumArgs: uint64(spiritRootId),
		StrArgs: fmt.Sprintf("%d_%d_%d", pos, curLv, nextLv),
	})
	return nil
}

func (s *SpiritRootSys) checkUpgradeNextLvCond(spiritRootId uint32, upgradeNextLvConf *jsondata.SpiritRootUpgradeLevel) bool {
	if len(upgradeNextLvConf.Consume) != 0 && !s.GetOwner().CheckConsumeByConf(upgradeNextLvConf.Consume, false, 0) {
		return false
	}

	if len(upgradeNextLvConf.BreakCond) == 0 {
		return true
	}

	conds := upgradeNextLvConf.BreakCond
	if len(conds) != 3 {
		return false
	}

	number := conds[0]
	typ := conds[1]
	lv := conds[2]
	rootUpgradeData := s.GetSpiritRootUpgrade(spiritRootId)

	var count uint32
	for _, pos := range rootUpgradeData.PosMap {
		if typ != pos.Type {
			continue
		}
		if lv > pos.Level {
			continue
		}
		count++
	}

	return number <= count
}

// 重新计算属性
func calcSpiritRootSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiSpiritRoot).(*SpiritRootSys)
	if !s.IsOpen() {
		return
	}

	data := s.GetData()
	var attrs jsondata.AttrVec

	// 计算等级带来的提升
	for id, lv := range data.SrLvMap {
		spiritRootConf, ok := jsondata.GetSpecifySpiritRootConf(id)
		if !ok {
			s.GetOwner().LogWarn("calcSpiritRootSysAttr not found spiritRootConf , id is %d", id)
			continue
		}
		var levelConf *jsondata.SpiritRootLevelConf
		for i := range spiritRootConf.LevelConf {
			if spiritRootConf.LevelConf[i].Level != lv {
				continue
			}
			levelConf = spiritRootConf.LevelConf[i]
			break
		}
		if levelConf != nil {
			attrs = append(attrs, levelConf.Attrs...)
		}
	}

	// 计算共鸣带来的提升
	for i := range data.ReachResonanceIds {
		reachResonanceId := data.ReachResonanceIds[i]
		resonanceConf, ok := jsondata.GetSpecifySpiritRootResonanceConf(reachResonanceId)
		if ok {
			attrs = append(attrs, resonanceConf.Attrs...)
		}
	}

	// 进阶属性
	for id, rootUpgrade := range data.SrUpgradeMap {
		spiritRootConf, ok := jsondata.GetSpecifySpiritRootConf(id)
		if !ok {
			s.GetOwner().LogWarn("calcSpiritRootSysAttr not found spiritRootConf , id is %d", id)
			continue
		}

		if rootUpgrade.PosMap == nil {
			continue
		}

		for _, posData := range rootUpgrade.PosMap {
			upgradeConf, ok := spiritRootConf.UpgradeConf[posData.Pos]
			if !ok {
				continue
			}
			levelConf, ok := upgradeConf.UpgradeLevel[posData.Level]
			if !ok {
				continue
			}
			attrs = append(attrs, levelConf.Attrs...)
		}
	}

	// 加属性
	if len(attrs) > 0 {
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func handleSpiritRootAfterUseItemQuickUpLvAndQuest(player iface.IPlayer, args ...interface{}) {
	s := player.GetSysObj(sysdef.SiSpiritRoot).(*SpiritRootSys)
	if !s.IsOpen() {
		s.GetOwner().LogWarn("sys not open")
		return
	}
	rootConf, ok := jsondata.GetSpiritRootConf()
	if !ok {
		s.GetOwner().LogWarn("not found rootConf")
		return
	}
	for _, root := range rootConf.SpiritRoots {
		s.activeSpiritRoot(root.Id)
	}
}

func init() {
	RegisterSysClass(sysdef.SiSpiritRoot, func() iface.ISystem {
		return &SpiritRootSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSpiritRoot, calcSpiritRootSysAttr)

	net.RegisterSysProtoV2(151, 1, sysdef.SiSpiritRoot, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritRootSys).c2sActive
	})
	net.RegisterSysProtoV2(151, 2, sysdef.SiSpiritRoot, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritRootSys).c2sUpLv
	})
	net.RegisterSysProtoV2(151, 3, sysdef.SiSpiritRoot, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritRootSys).c2sUpResonance
	})
	net.RegisterSysProtoV2(151, 10, sysdef.SiSpiritRoot, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritRootSys).c2sUpgrade
	})

	engine.RegQuestTargetProgress(custom_id.QttAnyNSpiritRootReachYLv, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) == 0 {
			actor.LogWarn("not found cand")
			return 0
		}
		candLv := ids[0]
		s := actor.GetSysObj(sysdef.SiSpiritRoot).(*SpiritRootSys)
		if !s.IsOpen() {
			s.GetOwner().LogWarn("sys not open")
			return 0
		}
		lvMap := s.GetData().SrLvMap
		if lvMap == nil {
			s.GetOwner().LogWarn("not lv map")
			return 0
		}
		var count uint32
		for u := range lvMap {
			if lvMap[u] < candLv {
				continue
			}
			count++
		}
		return count
	})

	engine.RegQuestTargetProgress(custom_id.QttUnLockNSpiritRoot, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		s := actor.GetSysObj(sysdef.SiSpiritRoot).(*SpiritRootSys)
		if !s.IsOpen() {
			s.GetOwner().LogWarn("sys not open")
			return 0
		}
		lvMap := s.GetData().SrLvMap
		if lvMap == nil {
			s.GetOwner().LogWarn("not lv map")
			return 0
		}
		return uint32(len(lvMap))
	})
	event.RegActorEvent(custom_id.AeAfterUseItemQuickUpLvAndQuest, handleSpiritRootAfterUseItemQuickUpLvAndQuest)
}
