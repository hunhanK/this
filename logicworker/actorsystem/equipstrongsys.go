package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

/*
	desc:锻造-强化系统
	author: LvYuMeng
*/

type EquipStrongSystem struct {
	Base
}

func (sys *EquipStrongSystem) OnInit() {
	binary := sys.GetBinaryData()

	if nil == binary.Intensify {
		binary.Intensify = make(map[uint32]uint32)
	}

	if nil == binary.IntensifySuitState {
		binary.IntensifySuitState = make(map[uint32]uint32)
	}

}

func (sys *EquipStrongSystem) OnOpen() {
	sys.S2CInfo()
}

func (sys *EquipStrongSystem) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *EquipStrongSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *EquipStrongSystem) S2CInfo() {
	if binary := sys.GetBinaryData(); nil != binary {
		sys.SendProto3(11, 3, &pb3.S2C_11_3{
			Level: binary.Intensify,
			Suit:  binary.IntensifySuitState,
		})
	}
}

func (sys *EquipStrongSystem) c2sStrong(msg *base.Message) error {
	var req pb3.C2S_11_4
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	if err := sys.AddEquipLevel(req.GetPos(), req.GetExt()); nil != err {
		return err
	}
	return nil
}

// 强化
func (sys *EquipStrongSystem) AddEquipLevel(pos uint32, ext string) error {
	if pos < itemdef.EtBegin || pos > itemdef.EtEnd {
		return neterror.ParamsInvalidError("not equip pos(%d)", pos)
	}
	binary := sys.GetBinaryData()

	ntConf := jsondata.GetStrongLvConf(pos, binary.Intensify[pos]+1)
	if nil == ntConf {
		return neterror.ParamsInvalidError("equip strong conf(%d) is nil", binary.Intensify[pos]+1)
	}
	var equip *pb3.ItemSt
	if equipSys, ok := sys.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip = equipSys.GetEquipByPos(pos)
	}
	if nil == equip {
		return neterror.ParamsInvalidError("no equip in pos(%d)", pos)
	}
	equipConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == equipConf {
		return neterror.ParamsInvalidError("equip item conf(%d) is nil", equip.GetItemId())
	}

	if equipConf.Stage < ntConf.ClassLimit { //阶级条件不满足
		return neterror.ParamsInvalidError("equip item stage(%d) is not satisfy", equip.GetItemId())
	}

	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeItem, Id: ntConf.Itemid, Count: uint32(ntConf.Count)},
		{Type: custom_id.ConsumeTypeMoney, Id: ntConf.MoneyType, Count: uint32(ntConf.MoneyCount)},
	}

	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogStrongConsume}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	preLv := binary.Intensify[pos]
	binary.Intensify[pos]++

	var allStrongLevel, baseLevel, jewelryLevel uint32
	for _, suit := range jsondata.StrongLvConfMgr {
		lv := binary.Intensify[suit.Type]
		switch suit.SuitType {
		case itemdef.EtEquipSuitTypeBase:
			baseLevel += lv
		case itemdef.EtEquipSuitTypeJewelry:
			jewelryLevel += lv
		}
		allStrongLevel += lv
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"pos":             pos,
		"oldLv":           preLv,
		"newLv":           binary.Intensify[pos],
		"baseAllLevel":    baseLevel,
		"jewelryAllLevel": jewelryLevel,
	})

	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogStrongConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(allStrongLevel),
		StrArgs: string(logArg),
	})
	sys.owner.TriggerEvent(custom_id.AeEquipStrong, equip, preLv)

	sys.ResetSysAttr(attrdef.SaEquipStrong)

	if ntConf.Broadcast {
		engine.BroadcastTipMsgById(tipmsgid.TpStrongUpgrade, sys.owner.GetId(), sys.owner.GetName(), itemdef.EquipPosNameVec[pos], ntConf.Level)
	}
	sys.SendProto3(11, 4, &pb3.S2C_11_4{
		Ret: true,
		Level: &pb3.KeyValue{
			Key:   pos,
			Value: binary.Intensify[pos],
		},
		Ext: ext,
	})
	sys.owner.TriggerQuestEventRange(custom_id.QttAllStrongLv)
	sys.owner.TriggerQuestEvent(custom_id.QttStrongTimes, 0, 1)
	sys.owner.UpdateStatics(model.FieldAllEquipStrongLv_, baseLevel)
	sys.owner.UpdateStatics(model.FieldAllAccessoryStrongLv_, jewelryLevel)
	return nil
}

func (sys *EquipStrongSystem) c2sStrongMasterActivate(msg *base.Message) error {
	var req pb3.C2S_11_5
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	if err := sys.ActivateEquipMaster(req.GetSuitType()); nil != err {
		return err
	}
	return nil
}

// 强化大师升阶
func (sys *EquipStrongSystem) ActivateEquipMaster(suitType uint32) error {
	binary := sys.GetBinaryData()
	ntConf := jsondata.GetStrongSuitNextConf(suitType, binary.IntensifySuitState[suitType])
	if nil == ntConf {
		return neterror.ParamsInvalidError("equip strong master conf nil: suit(%d) lv(%d) ", suitType, binary.IntensifySuitState[suitType])
	}
	preLv := binary.IntensifySuitState[suitType]
	var suitLevel uint32
	for pos, lv := range binary.Intensify {
		conf := jsondata.GetStrongConfConf(pos)
		if nil == conf {
			continue
		}
		if conf.SuitType == suitType {
			suitLevel += lv
		}
	}
	if suitLevel < ntConf.Level {
		return nil
	}
	binary.IntensifySuitState[suitType] = ntConf.Level

	sys.ResetSysAttr(attrdef.SaEquipStrong)

	sys.SendProto3(11, 5, &pb3.S2C_11_5{
		Ret: true,
		State: &pb3.KeyValue{
			Key:   suitType,
			Value: binary.IntensifySuitState[suitType],
		},
	})
	logArg, _ := json.Marshal(map[string]interface{}{
		"oldLv":    preLv,
		"newLv":    binary.IntensifySuitState[suitType],
		"suitType": suitType,
	})
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogEquipMasterActive, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
	return nil
}

func (sys *EquipStrongSystem) GetEquipStrongAllLv() uint32 {
	var sumStrongLv uint32
	for _, lv := range sys.GetBinaryData().Intensify {
		sumStrongLv += lv
	}
	return sumStrongLv
}

func calcEquipStrongSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	if nil == binary {
		return
	}

	//强化属性
	for pos, lv := range binary.Intensify {
		conf := jsondata.GetStrongLvConf(pos, lv)
		if nil == conf {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
	}
	//套装属性
	for suitType, lv := range binary.IntensifySuitState {
		conf := jsondata.GetStrongSuitConf(suitType, lv)
		if nil == conf {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
	}
}

func calcEquipStrongSysAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	if nil == binary {
		return
	}

	addRate := totalSysCalc.GetValue(attrdef.EquipStrongAddRate)
	//强化属性
	for pos, lv := range binary.Intensify {
		conf := jsondata.GetStrongLvConf(pos, lv)
		if addRate > 0 && nil != conf {
			engine.CheckAddAttrsRateRoundingUp(player, calc, conf.Attrs, uint32(addRate))
		}
	}
}

func handlePowerRushRankSubTypeEquipStrong(player iface.IPlayer) (score int64) {
	return player.GetAttrSys().GetSysPower(attrdef.SaEquipStrong)
}

func init() {
	RegisterSysClass(sysdef.SiStrong, func() iface.ISystem {
		return &EquipStrongSystem{}
	})
	gmevent.Register("strong", func(player iface.IPlayer, args ...string) bool {
		pos := utils.AtoUint32(args[0])
		lv := utils.AtoUint32(args[1])
		player.GetBinaryData().Intensify[pos] = lv
		sys := player.GetSysObj(sysdef.SiStrong).(*EquipStrongSystem)
		sys.ResetSysAttr(attrdef.SaEquipStrong)
		sys.SendProto3(11, 4, &pb3.S2C_11_4{
			Ret: true,
			Level: &pb3.KeyValue{
				Key:   pos,
				Value: lv,
			},
		})
		return true
	}, 1)
	engine.RegAttrCalcFn(attrdef.SaEquipStrong, calcEquipStrongSysAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaEquipStrong, calcEquipStrongSysAttrAddRate)
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeEquipStrong, handlePowerRushRankSubTypeEquipStrong)

	net.RegisterSysProto(11, 4, sysdef.SiStrong, (*EquipStrongSystem).c2sStrong)
	net.RegisterSysProto(11, 5, sysdef.SiStrong, (*EquipStrongSystem).c2sStrongMasterActivate)
	engine.RegQuestTargetProgress(custom_id.QttAllStrongLv, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if sys, ok := actor.GetSysObj(sysdef.SiStrong).(*EquipStrongSystem); ok {
			return sys.GetEquipStrongAllLv()
		}
		return 0
	})
}
