/**
 * @Author: LvYuMeng
 * @Date: 2024/9/11
 * @Desc: 装备觉醒
**/

package actorsystem

import (
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

type EquipAwakenSys struct {
	Base
}

func (s *EquipAwakenSys) OnLogin() {
	s.s2cInfo()
}

func (s *EquipAwakenSys) OnOpen() {
	s.ResetSysAttr(attrdef.SaEquipAwaken)
}

func (s *EquipAwakenSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EquipAwakenSys) s2cInfo() {
	s.SendProto3(11, 80, &pb3.S2C_11_80{Data: s.data()})
}

func (s *EquipAwakenSys) data() *pb3.EquipAwaken {
	binary := s.GetBinaryData()
	if nil == binary.EquipAwaken {
		binary.EquipAwaken = &pb3.EquipAwaken{}
	}
	if nil == binary.EquipAwaken.Lv {
		binary.EquipAwaken.Lv = make(map[uint32]uint32)
	}
	return binary.EquipAwaken
}

const (
	EquipAwakenBaseAddRate    = 1 // 基础
	EquipAwakenBaseStrongRate = 2 // 强化
	EquipAwakenBaseStarRate   = 3 // 星级（极品属性）
)

func (s *EquipAwakenSys) GetAwakenRate(subType, rateType uint32) uint32 {
	data := s.data()
	conf := jsondata.GetEquipAwakenConf(subType, data.Lv[subType])
	if nil == conf {
		return 0
	}
	switch rateType {
	case EquipAwakenBaseAddRate:
		return conf.BaseRate
	case EquipAwakenBaseStrongRate:
		return conf.StrongRate
	case EquipAwakenBaseStarRate:
		return conf.StarRate
	}
	return 0
}

func (s *EquipAwakenSys) c2sAwaken(msg *base.Message) error {
	var req pb3.C2S_11_81
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	subType := req.GetSubType()
	if equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		if _, equip := equipSys.GetEquipByPos(subType); nil == equip {
			return neterror.ParamsInvalidError("no equip in pos(%d)", subType)
		}
	}

	data := s.data()
	nextLv := data.Lv[subType] + 1
	conf := jsondata.GetEquipAwakenConf(subType, nextLv)
	if nil == conf {
		return neterror.ConfNotFoundError("equip awaken conf is nil")
	}

	if !s.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipAwaken}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Lv[subType] = nextLv
	s.ResetSysAttr(attrdef.SaEquipAwaken)
	s.SendProto3(11, 81, &pb3.S2C_11_81{
		SubType: subType,
		Lv:      nextLv,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipAwaken, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"subType": subType,
			"lv":      data.Lv[subType],
		}),
	})
	return nil
}

func (s *EquipAwakenSys) calcEquipAwakenEquipAttr(calc *attrcalc.FightAttrCalc) {
	for _, equip := range s.owner.GetMainData().ItemPool.Equips {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}
		if baseRate := s.GetAwakenRate(conf.SubType, EquipAwakenBaseAddRate); baseRate > 0 {
			engine.CheckAddAttrsRate(s.owner, calc, conf.StaticAttrs, baseRate)
		}
		if starRate := s.GetAwakenRate(conf.SubType, EquipAwakenBaseStarRate); starRate > 0 {
			engine.CheckAddAttrsRate(s.owner, calc, conf.PremiumAttrs, starRate)
		}
	}
}

func (s *EquipAwakenSys) calcEquipAwakenStrongSysAttr(calc *attrcalc.FightAttrCalc) {
	equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok {
		return
	}
	binary := s.owner.GetBinaryData()
	for subType, lv := range binary.Intensify {
		if _, equip := equipSys.GetEquipByPos(subType); nil == equip {
			continue
		}
		conf := jsondata.GetStrongLvConf(subType, lv)
		if nil == conf {
			continue
		}
		if baseRate := s.GetAwakenRate(subType, EquipAwakenBaseStrongRate); baseRate > 0 {
			engine.CheckAddAttrsRate(s.owner, calc, conf.Attrs, baseRate)
		}
	}
}

func calcEquipAwakenSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	awakenSys, ok := player.GetSysObj(sysdef.SiEquipAwaken).(*EquipAwakenSys)
	if !ok || !awakenSys.IsOpen() {
		return
	}
	awakenSys.calcEquipAwakenEquipAttr(calc)
	awakenSys.calcEquipAwakenStrongSysAttr(calc)
}

func reCalcEquipAwakenSysAttr(player iface.IPlayer, args ...interface{}) {
	awakenSys, ok := player.GetSysObj(sysdef.SiEquipAwaken).(*EquipAwakenSys)
	if !ok || !awakenSys.IsOpen() {
		return
	}
	awakenSys.ResetSysAttr(attrdef.SaEquipAwaken)
}

func init() {
	RegisterSysClass(sysdef.SiEquipAwaken, func() iface.ISystem {
		return &EquipAwakenSys{}
	})

	net.RegisterSysProtoV2(11, 81, sysdef.SiEquipAwaken, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EquipAwakenSys).c2sAwaken
	})

	event.RegActorEvent(custom_id.AeTakeOnEquip, reCalcEquipAwakenSysAttr)
	event.RegActorEvent(custom_id.AeTakeOffEquip, reCalcEquipAwakenSysAttr)
	event.RegActorEvent(custom_id.AeTakeReplaceEquip, reCalcEquipAwakenSysAttr)
	event.RegActorEvent(custom_id.AeEquipStrong, reCalcEquipAwakenSysAttr)

	engine.RegAttrCalcFn(attrdef.SaEquipAwaken, calcEquipAwakenSysAttr)

}
