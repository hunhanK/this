package system

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/gameserver/iface"
)

const (
	SiSysIdStart    = 1
	SiProperty      = SiSysIdStart // 属性系统
	SiLevel         = 2            // 等级
	SiDestinedFaBao = 3            // 本命法宝
	SiJingJie       = 4            // 境界
	SiEquip         = 5            // 装备
	SiFairyWing     = 6            // 仙翼
	SiFashion       = 7            // 时装
	SiFourSymbols   = 8            // 四象
	SiFreeVip       = 9            // 踏仙途
	SiMageBody      = 10           // 炼体
	SiRider         = 11           // 坐骑
	SiGuild         = 12           // 仙盟
	SiSysIdMax
)

var (
	ClassSet = [SiSysIdMax + 1]func(sysId int, owner iface.IRobot) iface.IRobotSystem{}
)

type Mgr struct {
	owner iface.IRobot
	objs  [SiSysIdMax + 1]iface.IRobotSystem
}

func CreateSysMgr(owner iface.IRobot) *Mgr {
	mgr := &Mgr{
		owner: owner,
	}
	return mgr
}

// RegSysClass 注册系统类型
func RegSysClass(sysId uint32, create func(sysId int, owner iface.IRobot) iface.IRobotSystem) {
	if set := ClassSet[sysId]; nil != set {
		logger.LogError("机器人系统重复注册 id:%d", sysId)
		return
	}

	ClassSet[sysId] = create
}

func (mgr *Mgr) OnInit() {
	for sysId, create := range ClassSet {
		if nil == create {
			continue
		}
		obj := create(sysId, mgr.owner)
		mgr.objs[sysId] = obj
		obj.OnInit()
	}
}

func (mgr *Mgr) GetSysObj(sysId int) iface.IRobotSystem {
	if sysId < SiSysIdStart || sysId > SiSysIdMax {
		return nil
	}
	return mgr.objs[sysId]
}

func (mgr *Mgr) DoUpdate() {
	for _, obj := range mgr.objs {
		if nil != obj {
			if !obj.CheckUpdateTime(obj.GetUpdateInterval()) {
				continue
			}
			if !obj.CanUpdate() {
				continue
			}
			utils.ProtectRun(obj.DoUpdate)
		}
	}
}

func (mgr *Mgr) OnLoadFinish() {
	for _, obj := range mgr.objs {
		if nil != obj {
			utils.ProtectRun(obj.OnLoadFinish)
		}
	}
}

func (mgr *Mgr) OnLogin() {
	for _, obj := range mgr.objs {
		if nil != obj {
			utils.ProtectRun(obj.OnLogin)
		}
	}
}

func (mgr *Mgr) OnLogout() {
	for _, obj := range mgr.objs {
		if nil != obj {
			utils.ProtectRun(obj.OnLogout)
		}
	}
}

func (mgr *Mgr) OnReset() {
	for _, obj := range mgr.objs {
		if nil != obj {
			utils.ProtectRun(obj.OnLogin)
		}
	}
}
