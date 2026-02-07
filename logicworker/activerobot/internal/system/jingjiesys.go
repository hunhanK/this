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

type JingJieSys struct {
	System
}

func (sys *JingJieSys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	sys.owner.SetAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Circle))
	sys.owner.ResetSysAttr(attrdef.RobotAttrSysJingJie)
	sys.GetOwner().SetChange(true)
}

func (sys *JingJieSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *JingJieSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotActorLevelWeight {
		conf := jsondata.GetMainCityRobotConfJingJie()
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
	if data.Circle == 0 {
		needChange = true
	}
	if !needChange && oldLvConf != nil && oldLvConf.ActorLevel < curLvConf.ActorLevel {
		needChange = true
	}
	if !needChange {
		return
	}

	data.Circle = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func (sys *JingJieSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiJingJie, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &JingJieSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysJingJie, handleRobotAttrSysJingJie)
}

func handleRobotAttrSysJingJie(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	lv := robot.GetData().Circle
	if jsondata.NewJingJieConfMgr == nil {
		return
	}
	if len(jsondata.NewJingJieConfMgr.LevelConf) == 0 {
		return
	}
	levelConf := jsondata.NewJingJieConfMgr.LevelConf[lv]
	CheckAddAttrsToCalc(robot, calc, levelConf.Attrs)
	CheckAddAttrsToCalc(robot, calc, levelConf.ExaAttrs)
}
