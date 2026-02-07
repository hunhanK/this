package engine

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/gameserver/iface"
)

type AddRateCalcCBFn func(player iface.IPlayer, totalSysCalc, singleSysCalc *attrcalc.FightAttrCalc)

var (
	_addRateCalcCBFn = map[uint32]AddRateCalcCBFn{}
)

func RegAttrAddRateCalcFn(sysId uint32, cb AddRateCalcCBFn) {
	if sysId <= attrdef.SysBegin || sysId >= attrdef.SysEnd {
		logger.LogFatal("reg attribution add rate calc callback sys id error, [sysId:%d]!!!", sysId)
	}
	if nil == cb {
		logger.LogFatal("reg attribution add rate calc callback is nil![sysId:%d]!!!", sysId)
	}
	if nil != _addRateCalcCBFn[sysId] {
		logger.LogFatal("reg attribution add rate calc repeat![sysId:%d]!!!", sysId)
	}
	_addRateCalcCBFn[sysId] = cb
}

func EachAttrCalcFn(f func(sysId uint32, cb AddRateCalcCBFn)) {
	if f == nil {
		return
	}
	for sysId, fn := range _addRateCalcCBFn {
		utils.ProtectRun(func() {
			f(sysId, fn)
		})
	}
}
