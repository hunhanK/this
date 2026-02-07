/**
 * @Author:
 * @Date:
 * @Desc:
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

	"github.com/gzjjyz/srvlib/utils/pie"
)

type FairyColdWeaponSys struct {
	Base
}

func (s *FairyColdWeaponSys) s2cInfo() {
	s.SendProto3(27, 60, &pb3.S2C_27_60{
		Data: s.getData(),
	})
}

func (s *FairyColdWeaponSys) getData() *pb3.FairyColdWeaponData {
	data := s.GetBinaryData().FairyColdWeaponData
	if data == nil {
		s.GetBinaryData().FairyColdWeaponData = &pb3.FairyColdWeaponData{}
		data = s.GetBinaryData().FairyColdWeaponData
	}
	if data.WeaponMap == nil {
		data.WeaponMap = make(map[uint32]*pb3.FairyColdWeapon)
	}
	if data.SuitMap == nil {
		data.SuitMap = make(map[uint32]*pb3.FairyColdWeaponSuit)
	}
	return data
}

func (s *FairyColdWeaponSys) GetData() *pb3.FairyColdWeaponData {
	return s.getData()
}

func (s *FairyColdWeaponSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FairyColdWeaponSys) OnLogin() {
	s.s2cInfo()
}

func (s *FairyColdWeaponSys) OnOpen() {
	s.s2cInfo()
}

func (s *FairyColdWeaponSys) getWeaponConf(id uint32) (*jsondata.FairyColdWeaponConfig, error) {
	config := jsondata.GetFairyColdWeaponConfig(id)
	if config == nil {
		return nil, neterror.ConfNotFoundError("%d not found weapon conf", id)
	}
	return config, nil
}

func (s *FairyColdWeaponSys) getSuitConf(id uint32) (*jsondata.FairyColdWeaponSuitConfig, error) {
	config := jsondata.GetFairyColdWeaponSuitConfig(id)
	if config == nil {
		return nil, neterror.ConfNotFoundError("%d not found suit conf", id)
	}
	return config, nil
}

func (s *FairyColdWeaponSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_27_61
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	conf, err := s.getWeaponConf(req.WeaponId)
	if err != nil {
		return neterror.Wrap(err)
	}

	weaponId := req.WeaponId
	data := s.getData()
	weapon := data.WeaponMap[weaponId]
	if weapon == nil {
		data.WeaponMap[weaponId] = &pb3.FairyColdWeapon{
			Id: weaponId,
		}
		weapon = data.WeaponMap[weaponId]
	}

	nextLv := weapon.Lv + 1
	var nextLvConf *jsondata.FairyColdWeaponLevelConf
	for _, levelConf := range conf.LevelConf {
		if levelConf.Lv != nextLv {
			continue
		}
		nextLvConf = levelConf
		break
	}

	if nextLvConf == nil {
		return neterror.ConfNotFoundError("weapon %d lv %d not found", weaponId, nextLv)
	}

	if len(nextLvConf.Consume) == 0 || !s.GetOwner().ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyColdWeaponUpLevel}) {
		s.GetOwner().SendTipMsg(tipmsgid.TpItemNotEnough)
		return neterror.ConsumeFailedError("weapon %d not enough consume", weaponId)
	}

	weapon.Lv = nextLv
	s.SendProto3(27, 61, &pb3.S2C_27_61{
		Weapon: weapon,
	})
	s.ResetSysAttr(attrdef.SaFairyColdWeapon)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFairyColdWeaponUpLevel, &pb3.LogPlayerCounter{
		NumArgs: uint64(weaponId),
		StrArgs: fmt.Sprintf("%d", nextLv),
	})
	s.checkActiveSuit(conf.SuitId)
	return nil
}

func (s *FairyColdWeaponSys) checkActiveSuit(suitId uint32) {
	owner := s.GetOwner()
	data := s.getData()
	suitConf, err := s.getSuitConf(suitId)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}
	weaponSuit := data.SuitMap[suitConf.SuitId]
	if weaponSuit == nil {
		data.SuitMap[suitConf.SuitId] = &pb3.FairyColdWeaponSuit{
			SuitId: suitConf.SuitId,
		}
		weaponSuit = data.SuitMap[suitConf.SuitId]
	}

	// 检测这个系列是否达标
	var checkReachSuit = func(suitActiveLvConf *jsondata.FairyColdWeaponSuitActiveLvConf) bool {
		// 达标数量
		var reachNum uint32
		for _, weaponId := range suitConf.WeaponIds {
			if reachNum >= suitActiveLvConf.MinNum {
				break
			}
			weapon := data.WeaponMap[weaponId]
			if weapon == nil {
				return false
			}
			if weapon.Lv < suitActiveLvConf.MinLv {
				continue
			}
			reachNum += 1
		}
		return reachNum >= suitActiveLvConf.MinNum
	}

	var reachNum2Idx = make(map[uint32]uint32)
	for _, suitActiveLvConf := range suitConf.ActiveLvConf {
		if !checkReachSuit(suitActiveLvConf) {
			continue
		}
		reachNum2Idx[suitActiveLvConf.MinNum] = suitActiveLvConf.Idx
	}

	for _, idx := range weaponSuit.IdxList {
		for _, suitActiveLvConf := range suitConf.ActiveLvConf {
			if idx != suitActiveLvConf.Idx {
				continue
			}
			if suitActiveLvConf.SkillId != 0 {
				owner.ForgetSkill(suitActiveLvConf.SkillId, true, true, true)
			}
		}
	}
	weaponSuit.IdxList = nil
	for _, idx := range reachNum2Idx {
		for _, suitActiveLvConf := range suitConf.ActiveLvConf {
			if idx != suitActiveLvConf.Idx {
				continue
			}
			if suitActiveLvConf.SkillId != 0 && suitActiveLvConf.SkillLv != 0 {
				owner.LearnSkill(suitActiveLvConf.SkillId, suitActiveLvConf.SkillLv, true)
			}
			weaponSuit.IdxList = append(weaponSuit.IdxList, idx)
		}
	}

	owner.SendProto3(27, 62, &pb3.S2C_27_62{
		Suit: weaponSuit,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFairyColdWeaponUpgradeSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(suitConf.SuitId),
		StrArgs: fmt.Sprintf("%v", weaponSuit.IdxList),
	})
	s.ResetSysAttr(attrdef.SaFairyColdWeapon)
}

func (s *FairyColdWeaponSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for _, weapon := range data.WeaponMap {
		conf, err := s.getWeaponConf(weapon.Id)
		if err != nil {
			continue
		}
		for _, levelConf := range conf.LevelConf {
			if levelConf.Lv != weapon.Lv {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, levelConf.Attrs)
			break
		}
	}

	for _, suit := range data.SuitMap {
		suitConf, err := s.getSuitConf(suit.SuitId)
		if err != nil {
			continue
		}
		for _, suitActiveLvConf := range suitConf.ActiveLvConf {
			if !pie.Uint32s(suit.IdxList).Contains(suitActiveLvConf.Idx) {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, suitActiveLvConf.Attrs)
		}
	}
}

func (s *FairyColdWeaponSys) calcFairyAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for _, weapon := range data.WeaponMap {
		conf, err := s.getWeaponConf(weapon.Id)
		if err != nil {
			continue
		}
		for _, levelConf := range conf.LevelConf {
			if levelConf.Lv != weapon.Lv {
				continue
			}
			for _, attr := range levelConf.FairyAttrs {
				calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
			}
			break
		}
	}

	for _, suit := range data.SuitMap {
		suitConf, err := s.getSuitConf(suit.SuitId)
		if err != nil {
			continue
		}
		for _, suitActiveLvConf := range suitConf.ActiveLvConf {
			if !pie.Uint32s(suit.IdxList).Contains(suitActiveLvConf.Idx) {
				continue
			}
			for _, attr := range suitActiveLvConf.FairyAttrs {
				calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
			}
		}
	}
}

func (s *FairyColdWeaponSys) calcAddRateAttr(totalSysCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for _, weapon := range data.WeaponMap {
		conf, err := s.getWeaponConf(weapon.Id)
		if err != nil {
			continue
		}
		addRate := totalSysCalc.GetValue(conf.AddRateAttrId)
		if addRate == 0 {
			continue
		}
		for _, levelConf := range conf.LevelConf {
			if levelConf.Lv != weapon.Lv {
				continue
			}
			engine.CheckAddAttrsRateRoundingUp(owner, calc, levelConf.Attrs, uint32(addRate))
			break
		}
	}
}

func calcFairyColdWeaponProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFairyColdWeapon)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FairyColdWeaponSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func calcFairyColdWeaponAddRate(player iface.IPlayer, totalSysCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFairyColdWeapon)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FairyColdWeaponSys)
	if !ok {
		return
	}
	sys.calcAddRateAttr(totalSysCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiFairyColdWeapon, func() iface.ISystem {
		return &FairyColdWeaponSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFairyColdWeapon, calcFairyColdWeaponProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaFairyColdWeapon, calcFairyColdWeaponAddRate)
	net.RegisterSysProtoV2(27, 61, sysdef.SiFairyColdWeapon, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FairyColdWeaponSys).c2sUpLv
	})
}
