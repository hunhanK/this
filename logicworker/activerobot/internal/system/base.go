package system

import (
	"jjyz/base/jsondata"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
)

type System struct {
	owner          iface.IRobot
	sysId          int
	lastUpdateTime uint32
}

func (sys *System) GetUpdateInterval() uint32 {
	putCandMap := jsondata.GetMainCityRobotConfPutCand()
	cand := putCandMap[gshare.GetOpenServerDay()]
	if nil == cand {
		return 0
	}
	return cand.GrowUpInterval
}

func (sys *System) CheckUpdateTime(interval uint32) bool {
	if interval == 0 {
		return false
	}
	now := time_util.NowSec()

	// 还没到时间
	if sys.lastUpdateTime > 0 && sys.lastUpdateTime+interval > now {
		return false
	}
	sys.lastUpdateTime = now
	return true
}

func (sys *System) DoUpdate()       {}
func (sys *System) CanUpdate() bool { return true }
func (sys *System) OnInit()         {}
func (sys *System) OnLoadFinish()   {}
func (sys *System) OnLogin()        {}
func (sys *System) OnLogout()       {}
func (sys *System) OnSave()         {}
func (sys *System) OnReset()        {}
func (sys *System) GetOwner() iface.IRobot {
	if sys.owner == nil {
		return nil
	}
	return sys.owner
}
