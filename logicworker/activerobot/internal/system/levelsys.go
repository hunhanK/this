/**
 * @Author: zjj
 * @Date: 2024/4/19
 * @Desc:
**/

package system

import (
	"github.com/gzjjyz/random"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

type LevelSys struct {
	System
}

func (sys *LevelSys) reCalcAttrs() {
	owner := sys.GetOwner()
	data := owner.GetData()
	owner.SetAttr(attrdef.Level, attrdef.AttrValueAlias(data.Level))
	owner.ResetSysAttr(attrdef.RobotAttrSysLevel)
	owner.SetChange(true)
}

func (sys *LevelSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *LevelSys) doUpdate() {
	confLevel := jsondata.GetMainCityRobotConfLevel()
	data := sys.GetOwner().GetData()

	var newLv = confLevel.InitLev
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeLevel)
	count := rank.GetRankCount()
	list := rank.GetList(count, count)
	if len(list) != 0 {
		minLev := list[0].Score
		diff := random.Interval(int(confLevel.MinValue), int(confLevel.MaxValue))
		newLv = uint32(minLev) - uint32(diff)
	}

	if data.OldLv > newLv {
		return
	}

	data.OldLv = data.Level
	data.Level = newLv

	sys.reCalcAttrs()
}

func (sys *LevelSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func handleRobotAttrSysLevel(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	if jData := jsondata.GetVocationConf(robot.GetJob()); nil != jData {
		if nil != jData.InitAttr {
			for _, conf := range jData.InitAttr {
				calc.AddValue(conf.Type, attrdef.AttrValueAlias(conf.Value))
			}
		}
		level := robot.GetData().Level
		if lvConf := jData.LevelAttr[level]; nil != lvConf {
			CheckAddAttrsToCalc(robot, calc, lvConf)
		}
	}
}

func init() {
	RegSysClass(SiLevel, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &LevelSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysLevel, handleRobotAttrSysLevel)
}
