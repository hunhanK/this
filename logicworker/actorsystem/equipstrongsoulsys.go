/**
 * @Author: zjj
 * @Date: 2024/11/7
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
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
)

type EquipStrongSoulSys struct {
	Base
}

func (s *EquipStrongSoulSys) s2cInfo() {
	s.SendProto3(11, 130, &pb3.S2C_11_130{
		Data: s.getData(),
	})
}

func (s *EquipStrongSoulSys) getData() *pb3.EquipStrongSoulData {
	data := s.GetBinaryData().EquipStrongSoulData
	if data == nil {
		s.GetBinaryData().EquipStrongSoulData = &pb3.EquipStrongSoulData{}
		data = s.GetBinaryData().EquipStrongSoulData
	}
	if data.PosMap == nil {
		data.PosMap = make(map[uint32]*pb3.EquipStrongSoulSinglePos)
	}
	if data.ResonanceMap == nil {
		data.ResonanceMap = make(map[uint32]uint32)
	}
	return data
}

func (s *EquipStrongSoulSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EquipStrongSoulSys) OnLogin() {
	s.s2cInfo()
}

func (s *EquipStrongSoulSys) OnOpen() {
	owner := s.GetOwner()
	jsondata.EachEquipStrongSoulConf(func(conf *jsondata.EquipStrongSoulConf) {
		if conf.OpenCond == nil {
			_, err := s.unLock(conf.Pos)
			if err != nil {
				owner.LogError("err:%v", err)
				return
			}
		}
	})
	s.ResetSysAttr(attrdef.SaEquipStrongSoul)
	s.s2cInfo()
}

func (s *EquipStrongSoulSys) checkOpenCand(pos uint32, openCond *jsondata.EquipStrongSoulOpenCond) bool {
	if openCond == nil {
		return true
	}
	owner := s.GetOwner()

	// 大荒古塔层数
	if openCond.PassLayer != 0 && owner.GetBinaryData().AncientTowerData != nil && owner.GetBinaryData().AncientTowerData.Layer < openCond.PassLayer {
		return false
	}

	// 铸魂等级
	data := s.getData()
	if openCond.Lv != 0 {
		singlePos, ok := data.PosMap[pos]
		if !ok {
			return false
		}
		if singlePos.Lv < openCond.Lv {
			return false
		}
	}

	return true
}

func (s *EquipStrongSoulSys) unLock(pos uint32) (*pb3.EquipStrongSoulSinglePos, error) {
	strongSoulConf := jsondata.GetEquipStrongSoulConf(pos)
	if strongSoulConf == nil {
		return nil, neterror.ConfNotFoundError("%d conf not found", pos)
	}
	data := s.getData()

	if _, ok := data.PosMap[pos]; ok {
		return nil, neterror.ParamsInvalidError("already unlock %d pos", pos)
	}

	if !s.checkOpenCand(pos, strongSoulConf.OpenCond) {
		return nil, neterror.ParamsInvalidError("not reach open cond %d pos", pos)
	}

	ret := &pb3.EquipStrongSoulSinglePos{
		Pos:   pos,
		Lv:    0,
		Stage: 0,
	}
	data.PosMap[pos] = ret
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipStrongSoulUnLock, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
	})
	return ret, nil
}

func (s *EquipStrongSoulSys) c2sUnLock(msg *base.Message) error {
	var req pb3.C2S_11_131
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	ret, err := s.unLock(req.Pos)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.SendProto3(11, 131, &pb3.S2C_11_131{
		PosData: ret,
	})
	return nil
}

func (s *EquipStrongSoulSys) checkEquipTakeOn(pos uint32) bool {
	var equip *pb3.ItemSt
	if equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip = equipSys.GetEquipByPos(pos)
	}
	if nil == equip {
		s.owner.LogTrace("no equip in pos(%d)", pos)
		return false
	}
	return true
}

func (s *EquipStrongSoulSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_11_132
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	pos := req.Pos
	data := s.getData()
	singlePos, ok := data.PosMap[pos]
	if !ok {
		return neterror.ParamsInvalidError("pos %d is lock", pos)
	}

	stageConf := jsondata.GetEquipStrongSoulStage(pos, singlePos.Stage)
	if stageConf == nil {
		return neterror.ConfNotFoundError("%d %d state conf not found ", pos, singlePos.Stage)
	}

	if singlePos.Lv >= stageConf.MaxLv {
		return neterror.ParamsInvalidError("pos %d lv %d max lv %d", pos, singlePos.Lv, stageConf.MaxLv)
	}

	nextLv := singlePos.Lv + 1
	nextLvConf := jsondata.GetEquipStrongSoulLevel(pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("%d %d lv conf not found", pos, nextLv)
	}

	owner := s.GetOwner()
	if !s.checkEquipTakeOn(pos) {
		return neterror.ParamsInvalidError("pos %d not has equip", pos)
	}

	if len(nextLvConf.Consume) != 0 && !owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipStrongSoulUpLv}) {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	singlePos.Lv = nextLv
	s.SendProto3(11, 132, &pb3.S2C_11_132{
		Lv:  nextLv,
		Pos: pos,
	})

	s.ResetSysAttr(attrdef.SaEquipStrongSoul)

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogEquipStrongSoulUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", nextLv),
	})
	return nil
}
func (s *EquipStrongSoulSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_11_133
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	pos := req.Pos
	data := s.getData()
	singlePos, ok := data.PosMap[pos]
	if !ok {
		return neterror.ParamsInvalidError("pos %d is lock", pos)
	}

	nextStage := singlePos.Stage + 1
	nextStageConf := jsondata.GetEquipStrongSoulStage(pos, nextStage)
	if nextStageConf == nil {
		return neterror.ConfNotFoundError("%d %d state conf not found ", pos, nextStage)
	}

	owner := s.GetOwner()
	if !s.checkEquipTakeOn(pos) {
		return neterror.ParamsInvalidError("pos %d not has equip", pos)
	}

	if !s.checkOpenCand(pos, nextStageConf.OpenCond) {
		return neterror.ParamsInvalidError("not reach open cond %d %d pos", pos, nextStage)
	}

	if len(nextStageConf.Consume) != 0 && !owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipStrongSoulUpStage}) {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	singlePos.Stage = nextStage
	s.SendProto3(11, 133, &pb3.S2C_11_133{
		Stage: nextStage,
		Pos:   pos,
	})

	s.ResetSysAttr(attrdef.SaEquipStrongSoul)

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogEquipStrongSoulUpStage, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
		StrArgs: fmt.Sprintf("%d", nextStage),
	})
	return nil
}
func (s *EquipStrongSoulSys) c2sActiveResonance(msg *base.Message) error {
	var req pb3.C2S_11_134
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	t := req.Type
	stage := req.Stage
	data := s.getData()

	resonanceConf := jsondata.GetEquipStrongSoulResonanceConf(t)
	if resonanceConf == nil {
		return neterror.ConfNotFoundError("%d resonance conf not found", t)
	}

	minStage := data.ResonanceMap[t]
	if minStage > stage {
		return neterror.ParamsInvalidError("min stage %d > %d", minStage, stage)
	}

	// 得到最小等级
	var calcMinStage uint32 = 999
	for _, pos := range data.PosMap {
		if !pie.Uint32s(resonanceConf.PosList).Contains(pos.Pos) {
			continue
		}
		if pos.Stage < calcMinStage {
			calcMinStage = pos.Stage
		}
	}
	if calcMinStage == 999 || calcMinStage < stage {
		return neterror.ParamsInvalidError("min stage %d != %d", stage, calcMinStage)
	}
	data.ResonanceMap[t] = stage
	s.SendProto3(11, 134, &pb3.S2C_11_134{
		Type:  t,
		Stage: stage,
	})
	s.ResetSysAttr(attrdef.SaEquipStrongSoul)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipStrongSoulActiveResonance, &pb3.LogPlayerCounter{
		NumArgs: uint64(t),
		StrArgs: fmt.Sprintf("%d", calcMinStage),
	})
	return nil
}

func (s *EquipStrongSoulSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	var calcSinglePos = func(posData *pb3.EquipStrongSoulSinglePos) {
		strongSoulConf := jsondata.GetEquipStrongSoulConf(posData.Pos)
		if strongSoulConf == nil {
			return
		}
		var maxAttrVec jsondata.AttrVec
		var rate uint32
		for _, attr := range strongSoulConf.MaxStageAttrShow {
			if !s.checkOpenCand(posData.Pos, attr.Cond) {
				continue
			}
			if attr.AttrsRate > 0 {
				rate += attr.AttrsRate
				continue
			}
			maxAttrVec = append(maxAttrVec, attr.Attrs...)
		}

		strongSoulLevel := jsondata.GetEquipStrongSoulLevel(posData.Pos, posData.Lv)
		if strongSoulLevel != nil && len(strongSoulLevel.Attrs) > 0 {
			engine.CheckAddAttrsToCalc(s.owner, calc, strongSoulLevel.Attrs)
			if rate > 0 {
				engine.CheckAddAttrsRateRoundingUp(s.owner, calc, strongSoulLevel.Attrs, rate)
			}
		}

		strongSoulStage := jsondata.GetEquipStrongSoulStage(posData.Pos, posData.Stage)
		if strongSoulStage != nil && strongSoulStage.Rate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, maxAttrVec, strongSoulStage.Rate)
		}
	}
	data := s.getData()
	for _, posData := range data.PosMap {
		if !s.checkEquipTakeOn(posData.Pos) {
			continue
		}
		calcSinglePos(posData)
	}
	for t, minStage := range data.ResonanceMap {
		resonanceConf := jsondata.GetEquipStrongSoulResonanceConf(t)
		if resonanceConf == nil {
			continue
		}
		stageConf, ok := resonanceConf.StageMap[minStage]
		if !ok {
			continue
		}
		engine.AddAttrsToCalc(s.GetOwner(), calc, stageConf.Attrs)
	}
}

func calcSaEquipStrongSoulAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiEquipStrongSoul)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*EquipStrongSoulSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiEquipStrongSoul, func() iface.ISystem {
		return &EquipStrongSoulSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaEquipStrongSoul, calcSaEquipStrongSoulAttr)
	net.RegisterSysProtoV2(11, 131, sysdef.SiEquipStrongSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipStrongSoulSys).c2sUnLock
	})
	net.RegisterSysProtoV2(11, 132, sysdef.SiEquipStrongSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipStrongSoulSys).c2sUpLv
	})
	net.RegisterSysProtoV2(11, 133, sysdef.SiEquipStrongSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipStrongSoulSys).c2sUpStage
	})
	net.RegisterSysProtoV2(11, 134, sysdef.SiEquipStrongSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipStrongSoulSys).c2sActiveResonance
	})
	event.RegActorEvent(custom_id.AeAncientTowerLayer, handleEquipStrongSoulAncientTowerLayerChange)
}

func handleEquipStrongSoulAncientTowerLayerChange(player iface.IPlayer, args ...interface{}) {
	attrSys := player.GetAttrSys()
	if attrSys != nil {
		attrSys.ResetSysAttr(attrdef.SaEquipStrongSoul)
	}
}
