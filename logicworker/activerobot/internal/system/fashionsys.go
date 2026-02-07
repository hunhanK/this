/**
 * @Author: zjj
 * @Date: 2024/4/19
 * @Desc:
**/

package system

import (
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type FashionSys struct {
	System
}

func (sys *FashionSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func (sys *FashionSys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	stage := jsondata.GetFairyWingStageConfByLv(data.GodWingLv)
	appearId := int64(appeardef.AppearSys_FairyWing)<<32 | int64(stage.Stage)
	owner.SetAttr(attrdef.AppearWing, appearId)
	owner.ResetSysAttr(attrdef.RobotAttrSysWing)
	owner.SetChange(true)
}

func (sys *FashionSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()
	owner.SetAttr(attrdef.TitleId, attrdef.AttrValueAlias(data.TitleId))
	vipLev := data.Vip
	if vipLev == 0 {
		return
	}

	confFashion := jsondata.GetMainCityRobotConfFashion()
	var conf *jsondata.MainCityRobotJobFashion
	for _, fashion := range confFashion {
		if fashion.OpenVipLevel > vipLev {
			continue
		}
		conf = fashion.JobFashion[owner.GetJob()]
	}
	if conf == nil {
		return
	}
	if conf.SpiritId > 0 {
		data.SpiritId = conf.SpiritId
		owner.SetAttr(attrdef.SpiritSkin, attrdef.AttrValueAlias(conf.SpiritId))
	}
	if conf.WeaponId > 0 {
		data.WeaponId = conf.WeaponId
		appearId := int64(appeardef.AppearSys_Fashion)<<32 | int64(conf.WeaponId)
		owner.SetAttr(attrdef.AppearWeapon, appearId)
	}
	if conf.ClothesId > 0 {
		data.ClothesId = conf.ClothesId
		appearId := int64(appeardef.AppearSys_Fashion)<<32 | int64(conf.ClothesId)
		owner.SetAttr(attrdef.AppearCloth, appearId)
	}
}

func init() {
	RegSysClass(SiFashion, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &FashionSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysFashion, handleRobotAttrSysFashion)
}

func handleRobotAttrSysFashion(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()

	if data.TitleId > 0 {
		conf := jsondata.GetTitleConfig(data.TitleId)
		if conf != nil {
			CheckAddAttrsToCalc(robot, calc, conf.Attr)
		}
	}

	if data.SpiritId > 0 {
		conf := jsondata.GetSpiritPetConf(data.SpiritId)
		if conf != nil && len(conf.Lvconf) != 0 {
			CheckAddAttrsToCalc(robot, calc, conf.Lvconf[0].Attrs)
		}
	}

	if data.WeaponId > 0 {
		conf := jsondata.GetFashionStartConf(data.WeaponId, 0)
		if conf != nil {
			CheckAddAttrsToCalc(robot, calc, conf.Attrs)
		}
	}

	if data.ClothesId > 0 {
		conf := jsondata.GetFashionStartConf(data.ClothesId, 0)
		if conf != nil {
			CheckAddAttrsToCalc(robot, calc, conf.Attrs)
		}
	}
}
