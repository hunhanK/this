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

type DestineFaBaoSys struct {
	System
}

func (sys *DestineFaBaoSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *DestineFaBaoSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func (sys *DestineFaBaoSys) reCalcAttrs() {
	owner := sys.GetOwner()
	mgr, _ := jsondata.GetDefaultDestinedFaBaoConf()
	if mgr == nil {
		logger.LogError("无法获取本命法宝配制")
		return
	}
	appearId := int64(appeardef.AppearSys_DestinedFabao)<<32 | int64(mgr.Id)
	owner.SetAttr(attrdef.NewFaBaoBattle, appearId)
	owner.ResetSysAttr(attrdef.RobotAttrSysDestinedFaBao)
	owner.SetChange(true)
}

func (sys *DestineFaBaoSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()
	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotActorLevelWeight {
		conf := jsondata.GetMainCityRobotConfDestinedFaBao()
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

	mgr, _ := jsondata.GetDefaultDestinedFaBaoConf()
	if mgr == nil {
		logger.LogError("无法获取本命法宝配制")
		return
	}

	data.DestinedFaBaoLv = uint32(random.Interval(int(curLvConf.MinValue), int(curLvConf.MaxValue)))
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiDestinedFaBao, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &DestineFaBaoSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysDestinedFaBao, handleRobotAttrSysDestinedFaBao)
}

func handleRobotAttrSysDestinedFaBao(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	lv := data.DestinedFaBaoLv
	if lv == 0 {
		return
	}

	conf, err := jsondata.GetDestinedFaBaoLevelConf(lv)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	CheckAddAttrsToCalc(robot, calc, conf.DestinedAttrVes)
	CheckAddAttrsToCalc(robot, calc, conf.AttrVes)
}
