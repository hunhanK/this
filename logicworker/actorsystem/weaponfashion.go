package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/fashion"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

/**
 * @Author: YangQibin
a* @Desc: 武器时装系统
 * @Date: 2024/3/30
*/

type WeaponFashionSys struct {
	Base
	data   *pb3.WeaponFashionData
	upstar fashion.UpStar
}

func (sys *WeaponFashionSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.WeaponFashionData == nil {
		binaryData.WeaponFashionData = &pb3.WeaponFashionData{}
	}

	sys.data = binaryData.WeaponFashionData

	if sys.data.Fashions == nil {
		sys.data.Fashions = make(map[uint32]*pb3.DressData)
	}
	sys.upstar = fashion.UpStar{
		Fashions:  sys.data.Fashions,
		LogId:     pb3.LogId_LogWeaponFashionUpStar,
		CheckJob:  false,
		AttrSysId: attrdef.SaWeaponFashion,
		GetLvConfHandler: func(fashionId, lv uint32) *jsondata.FashionStarConf {
			conf := jsondata.GetWeaponFashionStarConf(fashionId, lv)
			if conf != nil {
				return &conf.FashionStarConf
			}
			return nil
		},
		GetFashionConfHandler: func(fashionId uint32) *jsondata.FashionMeta {
			conf := jsondata.GetWeaponFashionConf(fashionId)
			if conf != nil {
				return &conf.FashionMeta
			}
			return nil
		},
		AfterUpstarCb:   sys.onUpstar,
		AfterActivateCb: sys.onActivate,
	}

	if err := sys.upstar.Init(); err != nil {
		sys.GetOwner().LogError("WeaponFashionSys upstar init failed, err: %v", err)
		return false
	}
	return true
}

func (sys *WeaponFashionSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.ResetSysAttr(attrdef.SaWeaponFashion)
	sys.s2cInfo()
}

func (sys *WeaponFashionSys) OnLogin() {
	if !sys.init() {
		return
	}
	sys.s2cInfo()
}

func (sys *WeaponFashionSys) s2cInfo() {
	sys.SendProto3(24, 20, &pb3.S2C_24_20{Data: sys.data})
}

func (sys *WeaponFashionSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *WeaponFashionSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_24_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg failed, err: %v", err)
	}

	err := sys.upstar.Upstar(sys.GetOwner(), req.AppearId, req.AutoBuy)
	if err != nil {
		return err
	}
	return nil
}

func (sys *WeaponFashionSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_24_24
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg failed, err: %v", err)
	}

	err := sys.upstar.Activate(sys.GetOwner(), req.AppearId, req.AutoBuy, false)
	if err != nil {
		return err
	}

	return nil
}

func (sys *WeaponFashionSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_24_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg failed, err: %v", err)
	}

	if req.AppearId == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Weapon)
		return nil
	}

	appearConf := jsondata.GetWeaponFashionConf(req.AppearId)
	if nil == appearConf {
		return neterror.InternalError("invalid appearId: %d", req.AppearId)
	}

	// 职业判断
	if appearConf.Job != 0 && sys.GetOwner().GetJob() != appearConf.Job {
		return neterror.ParamsInvalidError("job:%d failed: %d", appearConf.Job, req.AppearId)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Weapon, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_WeaponAppear,
		AppearId: req.AppearId,
	}, true)

	return nil
}

func (sys *WeaponFashionSys) onUpstar(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.GetOwner().LogError("WeaponFashionSys onUpstar failed, fashion not found, fashionId: %d", fashionId)
		return
	}

	weaponFashionConf := jsondata.GetWeaponFashionStarConf(fashionId, fashion.Star)
	if weaponFashionConf != nil && weaponFashionConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(weaponFashionConf.SkillId, weaponFashionConf.SkillLevel, true) {
			sys.GetOwner().LogError("learn skill failed for fashion %d star", fashionId, fashion.Star)
		}
	}

	sys.SendProto3(24, 21, &pb3.S2C_24_21{
		Data: fashion,
	})
}

func (sys *WeaponFashionSys) onActivate(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("onActivate failed, fashion not found, fashionId: %d", fashionId)
	}

	weaponFashionConf := jsondata.GetWeaponFashionStarConf(fashionId, fashion.Star)
	if weaponFashionConf != nil && weaponFashionConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(weaponFashionConf.SkillId, weaponFashionConf.SkillLevel, true) {
			sys.GetOwner().LogError("learn skill failed for fashion %d star", fashionId, fashion.Star)
		}
	}

	sys.SendProto3(24, 24, &pb3.S2C_24_24{
		Data: fashion,
	})
}

func (sys *WeaponFashionSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for _, foo := range sys.data.Fashions {
		conf := jsondata.GetWeaponFashionStarConf(foo.Id, foo.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}
}

func (sys *WeaponFashionSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Fashions[fashionId]
	return ok
}

func calcWeaponFashionSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiWeaponFashion).(*WeaponFashionSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func weaponOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaWeapon, attrdef.SaWeaponFashion, attrdef.SaGodWeapon}

	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}
	player.SetRankValue(gshare.RankTypeGodWeapon, sumPower)
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeGodWeapon, player.GetId(), sumPower)
}

func init() {
	RegisterSysClass(sysdef.SiWeaponFashion, func() iface.ISystem {
		return &WeaponFashionSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaWeaponFashion, calcWeaponFashionSysAttr)
	net.RegisterSysProto(24, 21, sysdef.SiWeaponFashion, (*WeaponFashionSys).c2sUpLv)
	net.RegisterSysProto(24, 22, sysdef.SiWeaponFashion, (*WeaponFashionSys).c2sDress)
	// net.RegisterSysProto(24, 23, gshare.SiWeaponFashion, (*WeaponFashionSys).c2sLearnSkill)
	net.RegisterSysProto(24, 24, sysdef.SiWeaponFashion, (*WeaponFashionSys).c2sActivate)
	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, weaponOnUpdateSysPowerMap)
}
