package engine

import (
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type CalcCBFn func(iface.IPlayer, *attrcalc.FightAttrCalc)

var (
	_callback = [attrdef.SysEnd]CalcCBFn{}
)

func RegAttrCalcFn(sysId uint32, cb CalcCBFn) {
	if sysId <= attrdef.SysBegin || sysId >= attrdef.SysEnd {
		logger.LogFatal("reg attribution calc callback sys id error, [sysId:%d]!!!", sysId)
	}
	if nil == cb {
		logger.LogFatal("reg attribution calc callback is nil![sysId:%d]!!!", sysId)
	}
	if nil != _callback[sysId] {
		logger.LogFatal("reg attribution calc repeat![sysId:%d]!!!", sysId)
	}
	_callback[sysId] = cb
}

func GetAttrCalcFn(sysId uint32) CalcCBFn {
	if sysId <= attrdef.SysBegin || sysId >= attrdef.SysEnd {
		logger.LogError("the attr sys id is out of range! [sysId:%d]", sysId)
		return nil
	}

	return _callback[sysId]
}
