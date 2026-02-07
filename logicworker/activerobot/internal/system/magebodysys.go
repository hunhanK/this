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

type MageBodySys struct {
	System
}

func (sys *MageBodySys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	sys.owner.SetAttr(attrdef.MageBodyLv, attrdef.AttrValueAlias(data.MageBodyLv))
	sys.owner.ResetSysAttr(attrdef.RobotAttrSysMageBody)
	sys.GetOwner().SetChange(true)
}

func (sys *MageBodySys) DoUpdate() {
	sys.doUpdate()
}

func (sys *MageBodySys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotActorLevelWeight {
		conf := jsondata.GetMainCityRobotConfMageBody()
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

	data.MageBodyLv = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func (sys *MageBodySys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiMageBody, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &MageBodySys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysMageBody, handleRobotAttrSysMageBody)
}

func handleRobotAttrSysMageBody(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	mageBodyLv := robot.GetData().MageBodyLv
	mageBodyConf := jsondata.GetMageBodyConfByLv(mageBodyLv)
	if mageBodyConf == nil {
		logger.LogTrace("%d 炼体配制不存在", mageBodyLv)
		return
	}
	if len(mageBodyConf.Star) == 0 {
		logger.LogTrace("%d 炼体星级配制不存在", mageBodyLv)
		return
	}
	CheckAddAttrsToCalc(robot, calc, mageBodyConf.Star[1].Attrs)
}
