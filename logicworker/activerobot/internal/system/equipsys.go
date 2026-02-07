/**
 * @Author: zjj
 * @Date: 2024/4/19
 * @Desc:
**/

package system

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/iface"
)

type EquipSys struct {
	System
}

func (sys *EquipSys) reCalcAttrs() {
	owner := sys.GetOwner()
	owner.ResetSysAttr(attrdef.RobotAttrSysEquip)
	owner.SetChange(true)
}

func (sys *EquipSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *EquipSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotEquip {
		conf := jsondata.GetMainCityRobotConfEquip()
		var val *jsondata.MainCityRobotEquip
		for _, entry := range conf {
			if entry.JingJieLevel < lv {
				val = entry
			}
		}
		return val
	}

	curLvConf := getConfByOwnerLv(data.Circle)

	if curLvConf == nil {
		logger.LogTrace("%d 配制获取失败", data.Level)
		return
	}

	data.Equips = nil

	// 循环后续优化
	for _, pos := range curLvConf.EquipPos {
		allocSeries, _ := series.AllocSeries()
		for _, item := range pos.EquipPosItem {
			if item.Job > 0 && item.Job != owner.GetJob() {
				continue
			}

			if item.Sex > 0 && item.Sex != owner.GetSex() {
				continue
			}

			data.Equips = append(data.Equips, &pb3.ItemSt{
				Handle: allocSeries,
				ItemId: item.ItemId,
				Count:  1,
				Pos:    pos.Pos,
			})
		}
	}

	sys.reCalcAttrs()
}

func (sys *EquipSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiEquip, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &EquipSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})

	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysEquip, handleRobotAttrSysEquip)

	engine.RegisterRobotViewFunc(common.ViewPlayerEquip, func(et iface.IRobot, rsp *pb3.DetailedRoleInfo) {
		data := et.GetData()
		rsp.EquipDeatil = &pb3.EquipDetail{}
		rsp.EquipDeatil.Equip = append(rsp.EquipDeatil.Equip, data.Equips...)
	})
}

func handleRobotAttrSysEquip(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	for _, equip := range data.Equips {
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if itemConf == nil {
			continue
		}
		CheckAddAttrsToCalc(robot, calc, itemConf.SuperAttrs)
		CheckAddAttrsToCalc(robot, calc, itemConf.StaticAttrs)
		CheckAddAttrsToCalc(robot, calc, itemConf.StaticBeautyAttrs)
		CheckAddAttrsToCalc(robot, calc, itemConf.PremiumAttrs)
	}
}
