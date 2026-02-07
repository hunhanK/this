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

type RiderSys struct {
	System
}

func (sys *RiderSys) reCalcAttrs() {
	sys.GetOwner().ResetSysAttr(attrdef.RobotAttrSysRider)
	sys.GetOwner().SetChange(true)
}

func (sys *RiderSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *RiderSys) doUpdate() {
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

	data.RiderLv = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func (sys *RiderSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiRider, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &RiderSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysRider, handleRobotAttrSysRider)

}

func handleRobotAttrSysRider(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	lv := robot.GetData().RiderLv
	conf := jsondata.GetRiderLvConf(lv)
	if conf == nil {
		logger.LogTrace("%d 的坐骑配制不存在", lv)
		return
	}
	CheckAddAttrsToCalc(robot, calc, conf.Attrs)
}
