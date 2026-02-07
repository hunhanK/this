/**
 * @Author: zjj
 * @Date: 2025年2月12日
 * @Desc: 龙装-屠龙套装
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

var KillDragonEquipSuitAttrAddRateMap = map[uint32]uint32{
	DEquSpirit:   attrdef.KillDragonEquipSuit1,
	DEquHorn:     attrdef.KillDragonEquipSuit2,
	DEquWings:    attrdef.KillDragonEquipSuit3,
	DEquTail:     attrdef.KillDragonEquipSuit4,
	DEquPei:      attrdef.KillDragonEquipSuit5,
	DEquBracelet: attrdef.KillDragonEquipSuit6,
	DEquRing:     attrdef.KillDragonEquipSuit7,
	DEquChain:    attrdef.KillDragonEquipSuit8,
	DEquCrown:    attrdef.KillDragonEquipSuit9,
	DEquWrist:    attrdef.KillDragonEquipSuit10,
	DEquLegs:     attrdef.KillDragonEquipSuit11,
	DEquBoots:    attrdef.KillDragonEquipSuit12,
}

type KillDragonEquipSuitSys struct {
	Base
}

func (s *KillDragonEquipSuitSys) s2cInfo() {
	s.SendProto3(50, 20, &pb3.S2C_50_20{
		Data: s.getData(),
	})
}

func (s *KillDragonEquipSuitSys) getData() *pb3.KillDragonEquipSuitData {
	data := s.GetBinaryData().KillDragonEquipSuitData
	if data == nil {
		s.GetBinaryData().KillDragonEquipSuitData = &pb3.KillDragonEquipSuitData{}
		data = s.GetBinaryData().KillDragonEquipSuitData
	}
	if data.EquipCastMap == nil {
		data.EquipCastMap = make(map[uint32]uint32)
	}
	if data.SuitMap == nil {
		data.SuitMap = make(map[uint32]uint32)
	}
	return data
}

func (s *KillDragonEquipSuitSys) GetData() *pb3.KillDragonEquipSuitData {
	return s.getData()
}

func (s *KillDragonEquipSuitSys) OnReconnect() {
	s.s2cInfo()
}

func (s *KillDragonEquipSuitSys) OnLogin() {
	s.s2cInfo()
}

func (s *KillDragonEquipSuitSys) OnOpen() {
	s.s2cInfo()
}

func (s *KillDragonEquipSuitSys) GetPosConf(pos uint32) (*jsondata.KillDragonEquipCastConfig, error) {
	conf := jsondata.GetKillDragonEquipCastConf(pos)
	if conf == nil {
		return nil, neterror.ConfNotFoundError("%d not found KillDragonEquipCastConf", pos)
	}
	return conf, nil
}

func (s *KillDragonEquipSuitSys) GetSuitConf(suitId uint32) (*jsondata.KillDragonEquipSuitConfig, error) {
	conf := jsondata.GetKillDragonEquipSuitConf(suitId)
	if conf == nil {
		return nil, neterror.ConfNotFoundError("%d not found KillDragonEquipSuitConf", suitId)
	}
	return conf, nil
}

func (s *KillDragonEquipSuitSys) learnSkill(ids, lvs []uint32) {
	if len(ids) != len(lvs) {
		return
	}
	owner := s.GetOwner()
	for i, id := range ids {
		owner.LearnSkill(id, lvs[i], true)
	}
}
func (s *KillDragonEquipSuitSys) forgetSkill(ids []uint32) {
	owner := s.GetOwner()
	for _, id := range ids {
		owner.ForgetSkill(id, true, true, true)
	}
}

func (s *KillDragonEquipSuitSys) c2sCastSingleEquip(msg *base.Message) error {
	var req pb3.C2S_50_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	pos := req.Pos
	data := s.getData()
	owner := s.GetOwner()
	if !s.checkDragonEquipTakeOn(pos) {
		return neterror.ParamsInvalidError("not take on %d dragon equip", pos)
	}

	conf, err := s.GetPosConf(pos)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	stage := data.EquipCastMap[pos]
	nextStage := stage + 1
	var nextStageConf *jsondata.KillDragonEquipCastStageConf
	for _, castStageConf := range conf.CastStageConf {
		if castStageConf.Stage != nextStage {
			continue
		}
		nextStageConf = castStageConf
		break
	}
	if nextStageConf == nil {
		return neterror.ParamsInvalidError("not found next stage %d conf", nextStage)
	}

	if !owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogKillDragonEquipSuitCast}) {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.EquipCastMap[pos] = nextStage
	s.learnSkill(nextStageConf.SkillId, nextStageConf.SkillLevel)
	s.SendProto3(50, 21, &pb3.S2C_50_21{
		Pos:   pos,
		Stage: nextStage,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogKillDragonEquipSuitCast, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", nextStage),
	})
	s.checkSuitActive(conf.SuitId)
	s.ResetSysAttr(attrdef.SaKillDragonEquipSuit)
	return nil
}

// 检查龙装是否穿戴
func (s *KillDragonEquipSuitSys) checkDragonEquipTakeOn(pos uint32) bool {
	if s := s.GetOwner().GetSysObj(sysdef.SiDragon); s != nil && s.IsOpen() {
		eqData := s.(*DragonSys).GetDragonEqData()
		itemId := eqData.Equips[pos]
		itemConf := jsondata.GetItemConfig(itemId)
		if itemConf != nil {
			return true
		}
	}
	return false
}

func (s *KillDragonEquipSuitSys) onDragonEquipTakeOff(pos uint32) {
	data := s.getData()
	owner := s.GetOwner()
	conf, err := s.GetPosConf(pos)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}
	suitConf, err := s.GetSuitConf(conf.SuitId)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	// 锻造装备
	stage := data.EquipCastMap[pos]
	var returnAwards jsondata.StdRewardVec
	for _, stageConf := range conf.CastStageConf {
		if stage < stageConf.Stage {
			continue
		}
		s.forgetSkill(stageConf.SkillId)
		returnAwards = append(returnAwards, stageConf.Awards...)
	}
	if len(returnAwards) != 0 {
		returnAwards = jsondata.MergeStdReward(returnAwards)
		engine.GiveRewards(owner, returnAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogKillDragonEquipSuitEquipTakeOff})
	}

	// 套装
	oldLevel := data.SuitMap[conf.SuitId]
	for _, suit := range suitConf.Suit {
		if suit.Level > oldLevel {
			continue
		}
		s.forgetSkill(suit.SkillId)
	}
	data.SuitMap[conf.SuitId] = 0
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogKillDragonEquipSuitLevelChange, &pb3.LogPlayerCounter{
		NumArgs: uint64(conf.SuitId),
		StrArgs: fmt.Sprintf("%d_0", oldLevel),
	})
	s.ResetSysAttr(attrdef.SaKillDragonEquipSuit)
}

func (s *KillDragonEquipSuitSys) checkSuitActive(suitId uint32) {
	if suitId == 0 {
		return
	}

	data := s.getData()
	owner := s.GetOwner()
	conf, err := s.GetSuitConf(suitId)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	var minLevel = uint32(999)
	for _, subPos := range conf.SubList {
		level := data.EquipCastMap[subPos]
		if level >= minLevel {
			continue
		}
		minLevel = level
	}
	if minLevel == 0 {
		return
	}

	var canActiveSuitConf *jsondata.KillDragonEquipSuit
	for _, suit := range conf.Suit {
		if minLevel < suit.StageLimit {
			continue
		}
		canActiveSuitConf = suit
	}

	if canActiveSuitConf == nil {
		return
	}

	oldLevel := data.SuitMap[suitId]
	if oldLevel == canActiveSuitConf.Level {
		return
	}
	data.SuitMap[suitId] = canActiveSuitConf.Level
	s.learnSkill(canActiveSuitConf.SkillId, canActiveSuitConf.SkillLevel)
	s.SendProto3(50, 22, &pb3.S2C_50_22{
		SuitId: suitId,
		Level:  canActiveSuitConf.Level,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogKillDragonEquipSuitLevelChange, &pb3.LogPlayerCounter{
		NumArgs: uint64(suitId),
		StrArgs: fmt.Sprintf("%d_%d", oldLevel, canActiveSuitConf.Level),
	})
}

func (s *KillDragonEquipSuitSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for pos, stage := range data.EquipCastMap {
		conf, err := s.GetPosConf(pos)
		if err != nil {
			owner.LogError("err:%v", err)
			continue
		}
		for _, stageConf := range conf.CastStageConf {
			if stage != stageConf.Stage {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, stageConf.Attrs)
			break
		}
	}

	for suitId, level := range data.SuitMap {
		conf, err := s.GetSuitConf(suitId)
		if err != nil {
			owner.LogError("err:%v", err)
			continue
		}
		for _, suit := range conf.Suit {
			if level != suit.Level {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, suit.Attrs)
			break
		}
	}
}

func calcKillDragonEquipSuitProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if s := player.GetSysObj(sysdef.SiKillDragonEquipSuit); s != nil && s.IsOpen() {
		s.(*KillDragonEquipSuitSys).calcAttr(calc)
	}
}

func init() {
	RegisterSysClass(sysdef.SiKillDragonEquipSuit, func() iface.ISystem {
		return &KillDragonEquipSuitSys{}
	})
	net.RegisterSysProtoV2(50, 21, sysdef.SiKillDragonEquipSuit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*KillDragonEquipSuitSys).c2sCastSingleEquip
	})
	engine.RegAttrCalcFn(attrdef.SaKillDragonEquipSuit, calcKillDragonEquipSuitProperty)
}
