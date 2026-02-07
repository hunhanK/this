/**
 * @Author: zjj
 * @Date: 2025/2/8
 * @Desc: 器魂修炼
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
)

var _qiHunSysRefAttrSys = map[uint32]uint32{
	sysdef.SiQiHunCultivationByRider:      attrdef.SaQiHunCultivationByRider,
	sysdef.SiQiHunCultivationByFairyWing:  attrdef.SaQiHunCultivationByFairyWing,
	sysdef.SiQiHunCultivationByWeapon:     attrdef.SaQiHunCultivationByWeapon,
	sysdef.SiQiHunCultivationBySpirit:     attrdef.SaQiHunCultivationBySpirit,
	sysdef.SiQiHunCultivationByRingsLaw:   attrdef.SaQiHunCultivationByRingsLaw,
	sysdef.SiQiHunCultivationByFootprints: attrdef.SaQiHunCultivationByFootprints,
	sysdef.SiQiHunBattleShield:            attrdef.SaQiHunCultivationByBattleShield,
}

type QiHunCultivationSys struct {
	Base
}

func (s *QiHunCultivationSys) s2cInfo() {
	s.SendProto3(8, 50, &pb3.S2C_8_50{
		Data: s.getData(),
	})
}

func (s *QiHunCultivationSys) getData() *pb3.QiHunCultivationData {
	m := s.GetBinaryData().QiHunCultivationMap
	if m == nil {
		s.GetBinaryData().QiHunCultivationMap = make(map[uint32]*pb3.QiHunCultivationData)
		m = s.GetBinaryData().QiHunCultivationMap
	}
	data := m[s.GetSysId()]
	if data == nil {
		m[s.GetSysId()] = &pb3.QiHunCultivationData{}
		data = m[s.GetSysId()]
	}
	if data.SysId == 0 {
		data.SysId = s.GetSysId()
	}
	if data.GodSkill == nil {
		data.GodSkill = make(map[uint32]uint32)
	}
	return data
}

func (s *QiHunCultivationSys) OnReconnect() {
	s.s2cInfo()
}

func (s *QiHunCultivationSys) OnLogin() {
	s.s2cInfo()
}

func (s *QiHunCultivationSys) OnOpen() {
	s.initBlessAndLv()
	s.initSkill()
	s.setCalcAttr()
	s.s2cInfo()
}

func (s *QiHunCultivationSys) setCalcAttr() {
	attrSysId, ok := _qiHunSysRefAttrSys[s.GetSysId()]
	if !ok {
		return
	}
	s.ResetSysAttr(attrSysId)
}

func (s *QiHunCultivationSys) initSkill() {
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	owner := s.GetOwner()
	data := s.getData()
	for _, skill := range conf.GodSkill {
		if !skill.IsDef {
			continue
		}
		if _, ok := data.GodSkill[skill.SkillId]; !ok {
			owner.LearnSkill(skill.SkillId, 1, true)
			data.GodSkill[skill.SkillId] = 1
		}
	}
}

func (s *QiHunCultivationSys) initBlessAndLv() {
	data := s.getData()
	if data.Level == 0 {
		data.Level = 1
	}
	data.Bless = 0
}

func (s *QiHunCultivationSys) getConf() (*jsondata.QiHunCultivationConfig, error) {
	conf := jsondata.GetQiHunCultivationConf(s.GetSysId())
	if conf == nil {
		return nil, neterror.ConfNotFoundError("%d not found conf", s.GetSysId())
	}
	return conf, nil
}

func (s *QiHunCultivationSys) addBless(addBless uint32, str string) {
	s.getData().Bless += addBless

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogQiHunCultivationAddBless, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetSysId()),
		StrArgs: fmt.Sprintf("%s_%d_%d", str, addBless, s.getData().Bless),
	})
	// 修正一下最大值
	conf, err := s.getConf()
	if err != nil {
		s.owner.LogError("err:%v", err)
		return
	}
	if conf.MaxBless > s.getData().Bless {
		return
	}
	s.getData().Bless = conf.MaxBless
}

func (s *QiHunCultivationSys) handleRecActiveFashionAwards(idx uint32) error {
	owner := s.owner
	sysId := s.GetSysId()
	conf, err := s.getConf()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	data := s.getData()
	if pie.Uint32s(data.ActiveIdxAwards).Contains(idx) {
		return neterror.ParamsInvalidError("[%d] already rec %d", sysId, idx)
	}

	var activeFashionAwards *jsondata.QiHunCultivationActiveFashionAwards
	for _, aConf := range conf.ActiveFashionAwards {
		if aConf.Idx != idx {
			continue
		}
		activeFashionAwards = aConf
		break
	}
	if activeFashionAwards == nil {
		return neterror.ConfNotFoundError("[%d] not found %d conf", sysId, idx)
	}

	obj := owner.GetSysObj(activeFashionAwards.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.ParamsInvalidError("[%d] %d not found %d sys", sysId, idx, activeFashionAwards.SysId)
	}

	sysCheck, ok := obj.(iface.IFashionActiveCheck)
	if !ok {
		return neterror.ParamsInvalidError("[%d] %d found %d sys not can check", sysId, idx, activeFashionAwards.SysId)
	}

	if !sysCheck.CheckFashionActive(activeFashionAwards.FashionId) {
		return neterror.ParamsInvalidError("[%d] %d found %d sys check %d failed", sysId, idx, activeFashionAwards.SysId, activeFashionAwards.FashionId)
	}

	data.ActiveIdxAwards = append(data.ActiveIdxAwards, idx)
	if len(activeFashionAwards.Awards) > 0 {
		engine.GiveRewards(owner, activeFashionAwards.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiHunCultivationActiveFashionAwards})
	}
	s.SendProto3(8, 51, &pb3.S2C_8_51{
		SysId: sysId,
		Idx:   idx,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogQiHunCultivationActiveFashionAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(sysId),
		StrArgs: fmt.Sprintf("%d_%d_%d", idx, activeFashionAwards.SysId, activeFashionAwards.FashionId),
	})
	return nil
}

func (s *QiHunCultivationSys) handleUpLevel() error {
	owner := s.owner
	sysId := s.GetSysId()
	conf, err := s.getConf()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}
	data := s.getData()
	nextLv := data.Level + 1
	var nextLvConf *jsondata.QiHunCultivationCultivation
	for _, cultivation := range conf.Cultivation {
		if cultivation.Lv != nextLv {
			continue
		}
		nextLvConf = cultivation
		break
	}
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("[%d] nextLv %d not found conf", sysId, nextLv)
	}

	if len(nextLvConf.Consume) == 0 || !owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogQiHunCultivationUpLevel}) {
		return neterror.ConsumeFailedError("[%d] nextLv %d consume failed", sysId, nextLv)
	}

	bless := data.Bless
	if bless >= conf.MaxBless || random.Hit(nextLvConf.Bless, conf.MaxBless) {
		data.Level += 1
		data.Bless = 0
		s.setCalcAttr()
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogQiHunCultivationUpLevel, &pb3.LogPlayerCounter{
			NumArgs: uint64(sysId),
			StrArgs: fmt.Sprintf("%d_%d", data.Level, bless),
		})
	} else {
		s.addBless(nextLvConf.FailedAddBless, "upLevelFailed")
	}
	s.SendProto3(8, 52, &pb3.S2C_8_52{
		SysId: sysId,
		Level: data.Level,
		Bless: data.Bless,
	})
	return nil
}

func (s *QiHunCultivationSys) handleUpgradeGodSkill(skillId uint32) error {
	owner := s.owner
	sysId := s.GetSysId()
	conf, err := s.getConf()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	var skillConf *jsondata.QiHunCultivationGodSkill
	for _, godSkill := range conf.GodSkill {
		if godSkill.SkillId != skillId {
			continue
		}
		skillConf = godSkill
		break
	}

	if skillConf == nil {
		return neterror.ParamsInvalidError("[%d] not found %d conf", sysId, skillId)
	}

	data := s.getData()
	level := data.GodSkill[skillId]
	var nextLv = level + 1
	var nextSkillLvConf *jsondata.QiHunCultivationSkillLv
	for _, skillLv := range skillConf.SkillLv {
		if skillLv.Level != nextLv {
			continue
		}
		nextSkillLvConf = skillLv
		break
	}

	if nextSkillLvConf == nil {
		return neterror.ParamsInvalidError("[%d] not found %d next Lv %d conf", sysId, skillId, nextLv)
	}

	if nextSkillLvConf.ItemId != 0 && nextSkillLvConf.Count != 0 {
		if !owner.ConsumeByConf([]*jsondata.Consume{{Id: nextSkillLvConf.ItemId, Count: nextSkillLvConf.Count}}, false, common.ConsumeParams{LogId: pb3.LogId_LogQiHunCultivationUpgradeGodSkill}) {
			return neterror.ConsumeFailedError("[%d] %d %d consume failed", sysId, nextSkillLvConf.ItemId, nextSkillLvConf.Count)
		}
	}

	data.GodSkill[skillId] = nextLv
	owner.LearnSkill(skillId, nextLv, true)
	s.SendProto3(8, 53, &pb3.S2C_8_53{
		SysId: sysId,
		Skill: &pb3.KeyValue{
			Key:   skillId,
			Value: nextLv,
		},
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogQiHunCultivationUpgradeGodSkill, &pb3.LogPlayerCounter{
		NumArgs: uint64(sysId),
		StrArgs: fmt.Sprintf("%d_%d", skillId, nextLv),
	})
	return nil
}

func (s *QiHunCultivationSys) handleResetGodSkill() error {
	owner := s.owner
	sysId := s.GetSysId()
	conf, err := s.getConf()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}
	data := s.getData()

	if len(data.GodSkill) == 0 {
		return neterror.ParamsInvalidError("[%d] not find learn skill", sysId)
	}

	var returnAwards jsondata.StdRewardVec
	var forgetIds pie.Uint32s
	for skillId, level := range data.GodSkill {
		for _, skill := range conf.GodSkill {
			if skill.IsDef {
				continue
			}
			if skill.SkillId != skillId {
				continue
			}
			forgetIds = append(forgetIds, skillId)
			for _, lv := range skill.SkillLv {
				if level < lv.Level {
					continue
				}
				if lv.Count == 0 {
					continue
				}
				returnAwards = append(returnAwards, &jsondata.StdReward{
					Id:    lv.ItemId,
					Count: int64(lv.Count),
				})
			}
			break
		}
		returnAwards = jsondata.MergeStdReward(returnAwards)
	}
	forgetIds = forgetIds.Unique()

	if len(forgetIds) == 0 {
		return neterror.ParamsInvalidError("not can forget skill")
	}

	if len(conf.ResetGodSkill) == 0 || !owner.ConsumeByConf(conf.ResetGodSkill, false, common.ConsumeParams{LogId: pb3.LogId_LogQiHunCultivationResetGodSkill}) {
		return neterror.ConsumeFailedError("[%d] consume failed", sysId)
	}

	for _, skillId := range forgetIds {
		owner.ForgetSkill(skillId, true, true, true)
		delete(data.GodSkill, skillId)
	}
	if len(returnAwards) != 0 {
		engine.GiveRewards(owner, returnAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiHunCultivationResetGodSkill})
	}
	s.SendProto3(8, 55, &pb3.S2C_8_55{SysId: sysId, GodSkill: data.GodSkill})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogQiHunCultivationResetGodSkill, &pb3.LogPlayerCounter{
		NumArgs: uint64(sysId),
	})
	return nil
}

func (s *QiHunCultivationSys) handleRecLvAwards(level uint32) error {
	owner := s.owner
	sysId := s.GetSysId()
	conf, err := s.getConf()
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}
	data := s.getData()

	// 默认不传领到当前等级
	if level == 0 {
		level = data.Level
	}

	lvAwardsHistory := pie.Uint32s(data.LevelAwards)
	if data.Level < level {
		return neterror.ParamsInvalidError("[%d] %d level not reach %d", sysId, data.Level, level)
	}

	var totalLvAwards jsondata.StdRewardVec
	for _, cultivation := range conf.Cultivation {
		if cultivation.Lv > level {
			continue
		}
		if lvAwardsHistory.Contains(cultivation.Lv) {
			continue
		}
		data.LevelAwards = append(data.LevelAwards, cultivation.Lv)
		totalLvAwards = append(totalLvAwards, cultivation.Awards...)
	}

	if len(totalLvAwards) > 0 {
		engine.GiveRewards(owner, totalLvAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiHunCultivationRecLvAwards})
		s.SendProto3(8, 56, &pb3.S2C_8_56{
			SysId:       sysId,
			LevelAwards: data.LevelAwards,
		})
		logworker.LogPlayerBehavior(owner, pb3.LogId_LogQiHunCultivationRecLvAwards, &pb3.LogPlayerCounter{
			NumArgs: uint64(sysId),
			StrArgs: fmt.Sprintf("%d", level),
		})
	}
	return nil
}

func (s *QiHunCultivationSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	conf, err := s.getConf()
	owner := s.owner
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}
	for _, cultivation := range conf.Cultivation {
		if data.Level != cultivation.Lv {
			continue
		}
		engine.CheckAddAttrsToCalc(owner, calc, cultivation.Attr)
		break
	}
}

func handleQiHunCultivationRecActiveFashionAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_51
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*QiHunCultivationSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}
	return sys.handleRecActiveFashionAwards(req.Idx)
}

func handleQiHunCultivationUpLevel(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_52
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*QiHunCultivationSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}

	return sys.handleUpLevel()
}
func handleQiHunCultivationLearnGodSkill(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_53
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*QiHunCultivationSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}
	return sys.handleUpgradeGodSkill(req.SkillId)
}

func handleQiHunCultivationResetGodSkill(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_55
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*QiHunCultivationSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}
	return sys.handleResetGodSkill()
}
func handleQiHunCultivationRecLvAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_56
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*QiHunCultivationSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}
	return sys.handleRecLvAwards(req.Level)
}
func calcQiHunCultivationProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc, attrSysId uint32) {
	var f = func(sysId uint32) {
		obj := player.GetSysObj(sysId)
		if obj == nil || !obj.IsOpen() {
			return
		}
		sys := obj.(*QiHunCultivationSys)
		if sys == nil {
			return
		}
		sys.calcAttr(calc)
	}
	for sysId, belongAttrSysId := range _qiHunSysRefAttrSys {
		if attrSysId == belongAttrSysId {
			f(sysId)
			break
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiQiHunCultivationByRider, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunCultivationByFairyWing, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunCultivationByWeapon, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunCultivationBySpirit, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunBattleShield, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunCultivationByFootprints, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	RegisterSysClass(sysdef.SiQiHunCultivationByRingsLaw, func() iface.ISystem {
		return &QiHunCultivationSys{}
	})
	net.RegisterProto(8, 51, handleQiHunCultivationRecActiveFashionAwards)
	net.RegisterProto(8, 52, handleQiHunCultivationUpLevel)
	net.RegisterProto(8, 53, handleQiHunCultivationLearnGodSkill)
	net.RegisterProto(8, 55, handleQiHunCultivationResetGodSkill)
	net.RegisterProto(8, 56, handleQiHunCultivationRecLvAwards)

	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByRider, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByRider)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByFairyWing, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByFairyWing)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByWeapon, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByWeapon)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationBySpirit, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationBySpirit)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByRingsLaw, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByRingsLaw)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByFootprints, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByFootprints)
	})
	engine.RegAttrCalcFn(attrdef.SaQiHunCultivationByBattleShield, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		calcQiHunCultivationProperty(player, calc, attrdef.SaQiHunCultivationByBattleShield)
	})
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeQiHunCultivationByRider, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaQiHunCultivationByRider)
	})
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeSaQiHunCultivationByFairyWing, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaQiHunCultivationByFairyWing)
	})
}
