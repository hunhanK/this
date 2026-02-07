/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	FaShenBattleMaster uint32 = 1
)

type FaShenBattleSys struct {
	Base
}

func (s *FaShenBattleSys) s2cInfo() {
	s.SendProto3(9, 20, &pb3.S2C_9_20{
		Data: s.getData(),
	})
}

func (s *FaShenBattleSys) getData() *pb3.FaShenBattleData {
	data := s.GetBinaryData().FaShenBattleData
	if data == nil {
		s.GetBinaryData().FaShenBattleData = &pb3.FaShenBattleData{}
		data = s.GetBinaryData().FaShenBattleData
	}
	if data.FsBattleMap == nil {
		data.FsBattleMap = make(map[uint32]uint32)
	}
	return data
}

func (s *FaShenBattleSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FaShenBattleSys) OnLogin() {
	s.checkUnlock()
	s.s2cInfo()
}

func (s *FaShenBattleSys) OnOpen() {
	s.checkUnlock()
	s.s2cInfo()
}

func (s *FaShenBattleSys) checkUnlock() {
	data := s.getData()
	fsBattleUnlockFlag := data.FsBattleUnlockFlag
	jsondata.EachFaShenBattleConfig(func(config *jsondata.FaShenBattleConfig) {
		if utils.IsSetBit(fsBattleUnlockFlag, config.Id) {
			return
		}
		if len(config.Consume) != 0 || len(config.Cond) != 0 {
			return
		}
		fsBattleUnlockFlag = utils.SetBit(fsBattleUnlockFlag, config.Id)
	})
	data.FsBattleUnlockFlag = fsBattleUnlockFlag
}

func (s *FaShenBattleSys) checkEventUnlock() bool {
	data := s.getData()
	fsBattleUnlockFlag := data.FsBattleUnlockFlag
	owner := s.GetOwner()
	jsondata.EachFaShenBattleConfig(func(config *jsondata.FaShenBattleConfig) {
		if utils.IsSetBit(fsBattleUnlockFlag, config.Id) {
			return
		}
		if len(config.Consume) == 0 && len(config.Cond) != 0 {
			for _, cond := range config.Cond {
				if !CheckReach(owner, cond.Type, cond.Value) {
					return
				}
			}
			fsBattleUnlockFlag = utils.SetBit(fsBattleUnlockFlag, config.Id)
		}
	})
	if data.FsBattleUnlockFlag != fsBattleUnlockFlag {
		data.FsBattleUnlockFlag = fsBattleUnlockFlag
		return true
	}
	return false
}

func (s *FaShenBattleSys) getFaShen(id uint32) *pb3.FaShen {
	owner := s.GetOwner()
	obj := owner.GetSysObj(sysdef.SiFaShen)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys := obj.(*FaShenSys)
	faShen := sys.getFaShen(id)
	return faShen
}

func (s *FaShenBattleSys) c2sToBattle(msg *base.Message) error {
	var req pb3.C2S_9_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	pos := req.Pos
	id := req.Id
	if !utils.IsSetBit(data.FsBattleUnlockFlag, pos) {
		return neterror.ParamsInvalidError("%d not active", pos)
	}

	owner := s.GetOwner()
	bloodSys, ok := owner.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !s.IsOpen() {
		return neterror.ParamsInvalidError("not found blood sys")
	}

	fsConfig := jsondata.GetFaShenConfig(id)
	if fsConfig == nil {
		return neterror.ConfNotFoundError("%d not found fa shen config", id)
	}

	faShen := s.getFaShen(id)
	if faShen == nil {
		return neterror.ParamsInvalidError("%d fa shen not active", id)
	}

	shenStarConf := fsConfig.StarConf[faShen.Star]
	if shenStarConf == nil {
		return neterror.ParamsInvalidError("%d not found star conf", id)
	}

	config := jsondata.GetFaShenBattleConfig(pos)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found", pos)
	}

	if config.Type == FaShenBattleMaster {
		if !bloodSys.CheckCanLearnSkill(fsConfig.SqType) {
			return neterror.ParamsInvalidError("blood sys not can learn skill %d", fsConfig.SqType)
		}
	}

	for otherPos, oldOtherPosId := range data.FsBattleMap {
		if oldOtherPosId != id {
			continue
		}
		return neterror.ParamsInvalidError("%d in otherPos %d", oldOtherPosId, otherPos)
	}

	oldId := data.FsBattleMap[pos]
	if oldId != 0 {
		err := s.downBattle(pos, oldId)
		if err != nil {
			s.LogError("err:%v", err)
		}
	}

	data.FsBattleMap[pos] = id
	s.SendProto3(9, 21, &pb3.S2C_9_21{
		Pos: pos,
		Id:  id,
	})

	s.ResetSysAttr(attrdef.SaFaShenBattle)
	if config.Type == FaShenBattleMaster {
		owner.TakeOnAppear(appeardef.AppearPos_FaShenBattle, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_FaShenBattle,
			AppearId: id,
		}, true)
		if shenStarConf.SkillId != 0 {
			owner.LearnSkill(shenStarConf.SkillId, shenStarConf.SkillLv, true)
		}
		s.faShenBattleMasterSkillLearn(fsConfig.SqType)
	}
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFaShenBattleToBattle, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d_%d", id, oldId),
	})
	return nil
}

func (s *FaShenBattleSys) faShenBattleMasterSkillLearn(sqType uint32) {
	owner := s.GetOwner()
	data := s.getData()
	var canOpt bool
	for pos, id := range data.FsBattleMap {
		faShenBattleConfig := jsondata.GetFaShenBattleConfig(pos)
		if faShenBattleConfig == nil {
			continue
		}
		if faShenBattleConfig.Type != FaShenBattleMaster {
			continue
		}
		faShenConfig := jsondata.GetFaShenConfig(id)
		if faShenConfig == nil {
			continue
		}
		canOpt = faShenConfig.SqType == sqType
		break
	}
	if !canOpt {
		return
	}
	bloodSys, ok := owner.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !s.IsOpen() {
		return
	}
	bloodSys.LearnSkill(sqType)

	bloodEquSys, ok := owner.GetSysObj(sysdef.SiBloodEqu).(*BloodEquSys)
	if !ok || !s.IsOpen() {
		return
	}
	bloodEquSys.LearnSkill(sqType)
}

func (s *FaShenBattleSys) faShenBattleMasterSkillForget(sqType uint32) {
	owner := s.GetOwner()
	data := s.getData()
	var canOpt bool
	for pos, id := range data.FsBattleMap {
		faShenBattleConfig := jsondata.GetFaShenBattleConfig(pos)
		if faShenBattleConfig == nil {
			continue
		}
		if faShenBattleConfig.Type != FaShenBattleMaster {
			continue
		}
		faShenConfig := jsondata.GetFaShenConfig(id)
		if faShenConfig == nil {
			continue
		}
		canOpt = faShenConfig.SqType == sqType
		break
	}
	if !canOpt {
		return
	}
	bloodSys, ok := owner.GetSysObj(sysdef.SiBlood).(*BloodSys)
	if !ok || !s.IsOpen() {
		return
	}
	bloodSys.ForgetSkill(sqType)

	bloodEquSys, ok := owner.GetSysObj(sysdef.SiBloodEqu).(*BloodEquSys)
	if !ok || !s.IsOpen() {
		return
	}
	bloodEquSys.ForgetSkill(sqType)
}

func (s *FaShenBattleSys) c2sDownBattle(msg *base.Message) error {
	var req pb3.C2S_9_22
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.getData()
	pos := req.Pos
	if !utils.IsSetBit(data.FsBattleUnlockFlag, pos) {
		return neterror.ParamsInvalidError("%d not active", pos)
	}
	oldId := data.FsBattleMap[pos]
	if oldId == 0 {
		return neterror.ParamsInvalidError("%d not need down", pos)
	}
	err = s.downBattle(pos, oldId)
	if err != nil {
		return neterror.Wrap(err)
	}
	s.SendProto3(9, 22, &pb3.S2C_9_22{
		Pos: pos,
	})

	s.ResetSysAttr(attrdef.SaFaShenBattle)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFaShenBattleDownBattle, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", oldId),
	})
	return nil
}

func (s *FaShenBattleSys) c2sUnlock(msg *base.Message) error {
	var req pb3.C2S_9_23
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	pos := req.Pos
	fsBattleUnlockFlag := data.FsBattleUnlockFlag
	if utils.IsSetBit(fsBattleUnlockFlag, pos) {
		return neterror.ParamsInvalidError("%d already active", pos)
	}
	config := jsondata.GetFaShenBattleConfig(pos)
	if config == nil {
		return neterror.ConfNotFoundError("%d config not found", pos)
	}
	owner := s.GetOwner()
	for _, cond := range config.Cond {
		if !CheckReach(owner, cond.Type, cond.Value) {
			return neterror.ParamsInvalidError("reach not ok")
		}
	}
	if len(config.Consume) != 0 && !owner.ConsumeByConf(config.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFaShenBattleUnlock}) {
		return neterror.ConsumeFailedError("%d consume failed", pos)
	}
	fsBattleUnlockFlag = utils.SetBit(fsBattleUnlockFlag, pos)
	data.FsBattleUnlockFlag = fsBattleUnlockFlag
	s.SendProto3(9, 23, &pb3.S2C_9_23{
		FsBattleUnlockFlag: fsBattleUnlockFlag,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFaShenBattleUnlock, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", fsBattleUnlockFlag),
	})
	return nil
}

func (s *FaShenBattleSys) downBattle(pos, id uint32) error {
	data := s.getData()
	config := jsondata.GetFaShenBattleConfig(pos)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found", pos)
	}
	if config.Type == FaShenBattleMaster {
		fsConfig := jsondata.GetFaShenConfig(id)
		if fsConfig == nil {
			return neterror.ConfNotFoundError("%d not found fa shen config", id)
		}
		faShen := s.getFaShen(id)
		if faShen == nil {
			return neterror.ParamsInvalidError("%d fa shen not active", id)
		}
		shenStarConf := fsConfig.StarConf[faShen.Star]
		if shenStarConf == nil {
			return neterror.ParamsInvalidError("%d not found star conf", id)
		}
		owner := s.GetOwner()
		owner.TakeOffAppear(appeardef.AppearPos_FaShenBattle)
		if shenStarConf.SkillId != 0 {
			s.owner.ForgetSkill(shenStarConf.SkillId, true, true, true)
		}
		s.faShenBattleMasterSkillForget(fsConfig.SqType)
	}
	delete(data.FsBattleMap, pos)
	return nil
}

func (s *FaShenBattleSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()
	for pos, fsId := range data.FsBattleMap {
		faShenBattleConfig := jsondata.GetFaShenBattleConfig(pos)
		if faShenBattleConfig == nil {
			return
		}
		if faShenBattleConfig.AddRate == 0 {
			return
		}
		config := jsondata.GetFaShenConfig(fsId)
		if config == nil || config.StarConf == nil {
			return
		}
		faShen := s.getFaShen(fsId)
		if faShen == nil {
			continue
		}
		faShenStarConf := config.StarConf[faShen.Star]
		if faShenStarConf == nil {
			continue
		}
		engine.CheckAddAttrsRateRoundingUp(owner, calc, faShenStarConf.Attrs, faShenBattleConfig.AddRate)
	}
	jsondata.EachFaShenBattleSeriesConfig(func(config *jsondata.FaShenBattleSeriesConfig) {
		if !utils.IsSetBit(data.FsBattleSeriesFlag, config.Id) {
			return
		}
		engine.CheckAddAttrsToCalc(owner, calc, config.Attrs)
	})
}

func (s *FaShenBattleSys) c2sActiveSeries(msg *base.Message) error {
	var req pb3.C2S_9_24
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	faShenBattleSeriesId := req.Id
	seriesConfig := jsondata.GetFaShenBattleSeriesConfig(faShenBattleSeriesId)
	if seriesConfig == nil {
		return neterror.ConfNotFoundError("%d not found conf", faShenBattleSeriesId)
	}

	fsBattleSeriesFlag := data.FsBattleSeriesFlag
	if utils.IsSetBit(fsBattleSeriesFlag, faShenBattleSeriesId) {
		return neterror.ParamsInvalidError("%d already active", faShenBattleSeriesId)
	}

	for _, id := range seriesConfig.Ids {
		shen := s.getFaShen(id)
		if shen != nil {
			continue
		}
		return neterror.ParamsInvalidError("%d not reach active cond %d", faShenBattleSeriesId, id)
	}

	data.FsBattleSeriesFlag = utils.SetBit(fsBattleSeriesFlag, faShenBattleSeriesId)
	owner := s.GetOwner()
	for _, skillId := range seriesConfig.SkillIds {
		owner.LearnSkill(skillId, 1, true)
	}

	s.ResetSysAttr(attrdef.SaFaShenBattle)
	s.SendProto3(9, 24, &pb3.S2C_9_24{
		FsBattleSeriesFlag: data.FsBattleSeriesFlag,
	})
	return nil
}

func calcFaShenBattleAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFaShenBattle)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FaShenBattleSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func handleFaShenBattleAeLevelUp(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiFaShenBattle)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FaShenBattleSys)
	if !ok {
		return
	}
	if sys.checkEventUnlock() {
		sys.s2cInfo()
	}
}

func handleFaShenBattleOtherUpdate(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	sqType, ok := args[0].(uint32)
	if !ok {
		return
	}
	obj := player.GetSysObj(sysdef.SiFaShenBattle)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FaShenBattleSys)
	if !ok {
		return
	}
	sys.faShenBattleMasterSkillLearn(sqType)
}

func init() {
	RegisterSysClass(sysdef.SiFaShenBattle, func() iface.ISystem {
		return &FaShenBattleSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFaShenBattle, calcFaShenBattleAttr)
	event.RegActorEvent(custom_id.AeLevelUp, handleFaShenBattleAeLevelUp)
	event.RegActorEvent(custom_id.AeBloodTakeOn, handleFaShenBattleOtherUpdate)
	event.RegActorEvent(custom_id.AeBloodEquTakeOn, handleFaShenBattleOtherUpdate)
	net.RegisterSysProtoV2(9, 21, sysdef.SiFaShenBattle, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenBattleSys).c2sToBattle
	})
	net.RegisterSysProtoV2(9, 22, sysdef.SiFaShenBattle, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenBattleSys).c2sDownBattle
	})
	net.RegisterSysProtoV2(9, 23, sysdef.SiFaShenBattle, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenBattleSys).c2sUnlock
	})
	net.RegisterSysProtoV2(9, 24, sysdef.SiFaShenBattle, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FaShenBattleSys).c2sActiveSeries
	})
}
