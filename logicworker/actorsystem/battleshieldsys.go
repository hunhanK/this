/**
 * @Author: zjj
 * @Date: 2024/11/22
 * @Desc: 战盾系统
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type BattleShieldSys struct {
	Base

	expUpLv      uplevelbase.ExpUpLv
	stageExpUpLv uplevelbase.ExpUpLv
}

func (s *BattleShieldSys) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.init()
}

func (s *BattleShieldSys) init() bool {
	data := s.getData()
	if data.ExpLv == nil {
		data.ExpLv = &pb3.ExpLvSt{}
	}
	if data.StageLv == nil {
		data.StageLv = &pb3.ExpLvSt{}
	}
	s.expUpLv = uplevelbase.ExpUpLv{
		ExpLv:            data.ExpLv,
		AttrSysId:        attrdef.SaBattleShield,
		BehavAddExpLogId: pb3.LogId_LogBattleShieldAddLevelExp,
		AfterUpLvCb:      s.AfterUpLevel,
		AfterAddExpCb:    s.AfterAddExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetBattleShieldLvConf(lv); conf != nil {
				return &conf.ExpLvConf
			}
			return nil
		},
	}
	s.stageExpUpLv = uplevelbase.ExpUpLv{
		ExpLv:            data.StageLv,
		AttrSysId:        attrdef.SaBattleShield,
		BehavAddExpLogId: pb3.LogId_LogBattleShieldAddStageExp,
		AfterUpLvCb:      s.AfterUpStageLevel,
		AfterAddExpCb:    s.AfterAddStageExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetBattleShieldStageLvConf(lv); conf != nil {
				return &conf.ExpLvConf
			}
			return nil
		},
	}
	if err := s.expUpLv.Init(s.GetOwner()); err != nil {
		s.LogError("BattleShieldSys OnOpen expUpLv.Init err: %v", err)
		return false
	}
	if err := s.stageExpUpLv.Init(s.GetOwner()); err != nil {
		s.LogError("BattleShieldSys OnOpen expUpLv.Init err: %v", err)
		return false
	}
	return true
}

func (s *BattleShieldSys) s2cInfo() {
	s.SendProto3(22, 10, &pb3.S2C_22_10{
		Data: s.getData(),
	})
}

func (s *BattleShieldSys) getData() *pb3.BattleShieldData {
	data := s.GetBinaryData().BattleShieldData
	if data == nil {
		s.GetBinaryData().BattleShieldData = &pb3.BattleShieldData{}
		data = s.GetBinaryData().BattleShieldData
	}
	if data.Medicine == nil {
		data.Medicine = make(map[uint32]*pb3.UseCounter)
	}
	return data
}

func (s *BattleShieldSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BattleShieldSys) OnLogin() {
	s.s2cInfo()
}

func (s *BattleShieldSys) OnOpen() {
	if !s.init() {
		return
	}
	s.getData().ExpLv.Lv = 1
	s.getData().StageLv.Lv = 1
	s.ResetSysAttr(attrdef.SaBattleShield)
	s.s2cInfo()
	appearId := jsondata.GetBattleShieldCommonConf().Appear.Id
	owner := s.GetOwner()
	owner.TakeOnAppear(appeardef.AppearPos_BattleShield, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_BattleShield,
		AppearId: appearId,
	}, true)
	if err := owner.CallActorFunc(actorfuncid.G2FOpenBattleShieldSys, &pb3.CommonSt{}); err != nil {
		owner.LogError("open sys call %d failed", actorfuncid.G2FOpenBattleShieldSys)
	}
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogBattleShieldAppear, &pb3.LogPlayerCounter{
		NumArgs: uint64(appearId),
	})
}

func (s *BattleShieldSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_22_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	appearId := jsondata.GetBattleShieldCommonConf().Appear.Id
	owner := s.GetOwner()
	s.GetOwner().TakeOnAppear(appeardef.AppearPos_BattleShield, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_BattleShield,
		AppearId: appearId,
	}, true)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogBattleShieldAppear, &pb3.LogPlayerCounter{
		NumArgs: uint64(appearId),
	})
	return nil
}

func (s *BattleShieldSys) c2sUpLevel(msg *base.Message) error {
	var req pb3.C2S_22_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	if req.ItemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	itemMap := req.ItemMap
	owner := s.GetOwner()
	expUpLv := s.expUpLv
	lvConf := expUpLv.GetLvConfHandler(expUpLv.ExpLv.Lv + 1)
	if lvConf == nil {
		return neterror.ParamsInvalidError("lvConf == nil")
	}

	expToAdd := uint64(0)

	levelUpItem := pie.Uint32s(jsondata.GetBattleShieldCommonConf().LevelUpItem)
	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !levelUpItem.Contains(item.ItemId) {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count %d < count %d", item.Count, entry.Value)
		}
	}

	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		expToAdd += uint64(itemConf.CommonField * entry.Value)
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogBattleShieldAddLevelExp) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	return expUpLv.AddExp(owner, expToAdd)
}

func (s *BattleShieldSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_22_13
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	if req.ItemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	itemMap := req.ItemMap
	owner := s.GetOwner()
	stageExpUpLv := s.stageExpUpLv
	lvConf := stageExpUpLv.GetLvConfHandler(stageExpUpLv.ExpLv.Lv + 1)
	if lvConf == nil {
		return neterror.ParamsInvalidError("lvConf == nil")
	}

	expToAdd := uint64(0)

	stageUpItem := pie.Uint32s(jsondata.GetBattleShieldCommonConf().StageUpItem)
	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !stageUpItem.Contains(item.ItemId) {
			return neterror.ParamsInvalidError("item not in stageUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count %d < count %d", item.Count, entry.Value)
		}
	}

	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		expToAdd += uint64(itemConf.CommonField * entry.Value)
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogBattleShieldAddStageExp) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	return stageExpUpLv.AddExp(owner, expToAdd)
}

func (s *BattleShieldSys) AfterUpLevel(lv uint32) {}

func (s *BattleShieldSys) AfterAddExp() {
	s.SendProto3(22, 12, &pb3.S2C_22_12{ExpLv: s.getData().ExpLv})
}

func (s *BattleShieldSys) AfterUpStageLevel(lv uint32) {}

func (s *BattleShieldSys) AfterAddStageExp() {
	s.SendProto3(22, 13, &pb3.S2C_22_13{StageLv: s.getData().StageLv})
}

func (s *BattleShieldSys) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (bool, bool, int64) {
	medicineConf := jsondata.GetBattleShieldCommonConf().Medicine[conf.ItemId]
	if medicineConf == nil {
		return false, false, 0
	}
	data := s.getData()
	owner := s.GetOwner()
	medicine, ok := data.Medicine[conf.ItemId]
	if !ok {
		medicine = &pb3.UseCounter{
			Id: conf.ItemId,
		}
		data.Medicine[conf.ItemId] = medicine
	}

	var limitConf *jsondata.MedicineUseLimit

	for _, mul := range medicineConf.UseLimit {
		if owner.GetLevel() <= mul.LevelLimit {
			limitConf = mul
			break
		}
	}

	if limitConf == nil && len(medicineConf.UseLimit) > 0 {
		limitConf = medicineConf.UseLimit[len(medicineConf.UseLimit)-1]
	}

	if medicine.Count+uint32(param.Count) > limitConf.Limit {
		owner.LogError("useMedicine failed, medicine.Count >= limitConf.Limit, medicine.Count: %d, limitConf.Limit: %d", medicine.Count, limitConf.Limit)
		return false, false, 0
	}

	medicine.Count += uint32(param.Count)

	s.ResetSysAttr(attrdef.SaBattleShield)
	s.SendProto3(22, 14, &pb3.S2C_22_14{
		Medicines: data.Medicine,
	})
	return true, true, param.Count
}

func (s *BattleShieldSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()

	// 丹药对基础数值的加成比例
	for id, medicine := range data.Medicine {
		medicineConf := jsondata.GetBattleShieldCommonConf().Medicine[id]
		if medicineConf == nil {
			continue
		}
		// 基本属性百分比加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.RateAttrs, medicine.Count)
		// 计算丹药的固定数值加成
		engine.CheckAddAttrsTimes(owner, calc, medicineConf.Attrs, medicine.Count)
	}
	lv := data.ExpLv.Lv
	stageLv := data.StageLv.Lv
	lvConf := jsondata.GetBattleShieldLvConf(lv)
	stageLvConf := jsondata.GetBattleShieldStageLvConf(stageLv)
	if lvConf != nil {
		engine.CheckAddAttrsToCalc(owner, calc, lvConf.Attrs)
	}
	if stageLvConf != nil {
		engine.CheckAddAttrsToCalc(owner, calc, stageLvConf.Attrs)
	}
}

func (s *BattleShieldSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()
	medicineAddRate := uint32(totalSysCalc.GetValue(attrdef.BattleShieldBaseAttrAddRate))
	lv := data.ExpLv.Lv
	lvConf := jsondata.GetBattleShieldLvConf(lv)
	if lvConf != nil && medicineAddRate != 0 {
		engine.CheckAddAttrsRateRoundingUp(owner, calc, lvConf.Attrs, medicineAddRate)
	}
}

func battleShieldUseMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
	if !ok || !s.IsOpen() {
		return false, false, 0
	}
	return s.useMedicine(param, conf)
}

func handleBattleShieldAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttr(calc)
}

func handleBattleShieldAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrAddRate(totalSysCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiBattleShield, func() iface.ISystem {
		return &BattleShieldSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaBattleShield, handleBattleShieldAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaBattleShield, handleBattleShieldAttrAddRate)
	net.RegisterSysProtoV2(22, 11, sysdef.SiBattleShield, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldSys).c2sAppear
	})
	net.RegisterSysProtoV2(22, 12, sysdef.SiBattleShield, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldSys).c2sUpLevel
	})
	net.RegisterSysProtoV2(22, 13, sysdef.SiBattleShield, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BattleShieldSys).c2sUpStage
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemBattleShieldMedicine, battleShieldUseMedicine)
	initBattleShieldGm()
}

func initBattleShieldGm() {
	gmevent.Register("BattleShieldSys.addLvExp", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
		if !ok || !s.IsOpen() {
			return false
		}
		err := s.expUpLv.AddExp(player, utils.AtoUint64(args[0]))
		if err != nil {
			player.LogError("err:%v", err)
			return false
		}
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("BattleShieldSys.changeLv", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
		if !ok || !s.IsOpen() {
			return false
		}
		s.getData().ExpLv.Lv = utils.AtoUint32(args[0])
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("BattleShieldSys.addStageLvExp", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
		if !ok || !s.IsOpen() {
			return false
		}
		err := s.stageExpUpLv.AddExp(player, utils.AtoUint64(args[0]))
		if err != nil {
			player.LogError("err:%v", err)
			return false
		}
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("BattleShieldSys.changeStageLv", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiBattleShield).(*BattleShieldSys)
		if !ok || !s.IsOpen() {
			return false
		}
		s.getData().ExpLv.Lv = utils.AtoUint32(args[0])
		s.s2cInfo()
		return false
	}, 1)
}
