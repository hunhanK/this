/**
 * @Author: zjj
 * @Date: 2024/4/19
 * @Desc:
**/

package system

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type FreeVipSys struct {
	System
}

func (sys *FreeVipSys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	owner.SetAttr(attrdef.VipLevel, attrdef.AttrValueAlias(data.Vip))
	owner.SetAttr(attrdef.FreeVipLv, attrdef.AttrValueAlias(data.FreeVipLv))
	owner.ResetSysAttr(attrdef.RobotAttrSysFreeVip)
	owner.SetChange(true)
}

func (sys *FreeVipSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *FreeVipSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotActorLevelWeight {
		conf := jsondata.GetMainCityRobotConfFreeVip()
		var val *jsondata.MainCityRobotActorLevelWeight
		for _, entry := range conf {
			if entry.ActorLevel <= lv {
				val = entry
			}
		}
		return val
	}

	oldLvConf := getConfByOwnerLv(data.OldLv)
	curLvConf := getConfByOwnerLv(data.Level)

	if curLvConf == nil {
		logger.LogTrace("%d 配制获取失败", data.Level)
		return
	}

	// 旧等级配制
	var needChange = oldLvConf == nil
	if !needChange && oldLvConf != nil && oldLvConf.ActorLevel < curLvConf.ActorLevel {
		needChange = true
	}
	if !needChange {
		return
	}

	data.FreeVipLv = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func (sys *FreeVipSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiFreeVip, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &FreeVipSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysFreeVip, handleRobotAttrSysFreeVip)
}

func handleRobotAttrSysFreeVip(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	freeVipLv := robot.GetAttr(attrdef.FreeVipLv)
	vipLvConf := jsondata.GetFreeVipLvConf(uint32(freeVipLv))
	if vipLvConf == nil {
		return
	}
	CheckAddAttrsToCalc(robot, calc, vipLvConf.Attrs)

	vipLv := robot.GetAttr(attrdef.VipLevel)
	if vipLv == 0 {
		return
	}
	confByLevel := jsondata.GetVipLevelConfByLevel(uint32(vipLv))
	if confByLevel == nil {
		return
	}
	CheckAddAttrsToCalc(robot, calc, confByLevel.Attrs)
}
