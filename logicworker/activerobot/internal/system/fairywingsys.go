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
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type FairyWingSys struct {
	System
}

func (sys *FairyWingSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *FairyWingSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func (sys *FairyWingSys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	stage := jsondata.GetFairyWingStageConfByLv(data.GodWingLv)
	appearId := int64(appeardef.AppearSys_FairyWing)<<32 | int64(stage.Stage)
	owner.SetAttr(attrdef.AppearWing, appearId)
	owner.ResetSysAttr(attrdef.RobotAttrSysWing)
	owner.SetChange(true)
}

func (sys *FairyWingSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotActorLevelWeight {
		conf := jsondata.GetMainCityRobotConfFairyWing()
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

	data.GodWingLv = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiFairyWing, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &FairyWingSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysWing, handleRobotAttrSysWing)

}

func handleRobotAttrSysWing(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	lv := data.GodWingLv

	lvConf := jsondata.GetFairyWingLvConf(lv)
	if lvConf != nil {
		CheckAddAttrsToCalc(robot, calc, lvConf.Attrs)
	}
}
