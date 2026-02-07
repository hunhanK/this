/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2022/5/30 11:49
 */

package engine

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/gameserver/iface"
)

type MainCityRobotCalcCBFn func(robot iface.IRobot, calc *attrcalc.FightAttrCalc)

var (
	_mainCityRobotAttrCallback = [attrdef.SysEnd]MainCityRobotCalcCBFn{}
)

func RegMainCityRobotCalcFn(sysId uint32, cb MainCityRobotCalcCBFn) {
	if sysId <= attrdef.SysBegin || sysId >= attrdef.SysEnd {
		logger.LogStack("reg attribution calc callback sys id error, [sysId:%d]!!!", sysId)
	}
	if nil == cb {
		logger.LogStack("reg attribution calc callback is nil![sysId:%d]!!!", sysId)
	}
	if nil != _mainCityRobotAttrCallback[sysId] {
		logger.LogStack("reg attribution calc repeat![sysId:%d]!!!", sysId)
	}
	_mainCityRobotAttrCallback[sysId] = cb
}

func GetMainCityRobotCalcFn(sysId uint32) MainCityRobotCalcCBFn {
	if sysId <= attrdef.SysBegin || sysId >= attrdef.SysEnd {
		logger.LogError("the attr sys id is out of range! [sysId:%d]", sysId)
		return nil
	}

	return _mainCityRobotAttrCallback[sysId]
}
