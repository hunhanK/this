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
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
)

type FourSymbolsSys struct {
	System
}

func (sys *FourSymbolsSys) reCalcAttrs() {
	owner := sys.GetOwner()
	owner.ResetSysAttr(attrdef.RobotAttrSysFourSymbols)
	owner.SetChange(true)
}

func (sys *FourSymbolsSys) init() {
	data := sys.GetOwner().GetData()
	if data.DragonHolySoulData == nil {
		data.DragonHolySoulData = &pb3.FourSymbolsInfo{}
	}
	if data.TigerHolySoulData == nil {
		data.TigerHolySoulData = &pb3.FourSymbolsInfo{}
	}
	if data.RoseFinchHolySoulData == nil {
		data.RoseFinchHolySoulData = &pb3.FourSymbolsInfo{}
	}
	if data.TortoiseHolySoulData == nil {
		data.TortoiseHolySoulData = &pb3.FourSymbolsInfo{}
	}
}

func (sys *FourSymbolsSys) DoUpdate() {
	sys.doUpdate()
}

func (sys *FourSymbolsSys) doUpdate() {
	owner := sys.GetOwner()
	data := owner.GetData()

	var getConfByOwnerLv = func(lv uint32) *jsondata.MainCityRobotFourSymbol {
		conf := jsondata.GetMainCityRobotConfFourSymbols()
		var val *jsondata.MainCityRobotFourSymbol
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

	for _, fourSymbolsPo := range curLvConf.FourSymbolsPos {
		if sys.checkCircleAndLevel(fourSymbolsPo.Pos) {
			continue
		}
		var fourSymbolsInfo *pb3.FourSymbolsInfo
		switch fourSymbolsPo.Pos {
		case custom_id.FourSymbolsDragon:
			fourSymbolsInfo = data.DragonHolySoulData
		case custom_id.FourSymbolsTiger:
			fourSymbolsInfo = data.TigerHolySoulData
		case custom_id.FourSymbolsRosefinch:
			fourSymbolsInfo = data.RoseFinchHolySoulData
		case custom_id.FourSymbolsTortoise:
			fourSymbolsInfo = data.TortoiseHolySoulData
		}
		if fourSymbolsInfo == nil {
			continue
		}
		fourSymbolsInfo.Level = uint32(random.Interval(int(fourSymbolsPo.MinValue), int(fourSymbolsPo.MaxValue)))
	}

	sys.reCalcAttrs()
}

func (sys *FourSymbolsSys) OnLogin() {
	sys.doUpdate()
	sys.reCalcAttrs()
}

func init() {
	RegSysClass(SiFourSymbols, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		return &FourSymbolsSys{
			System{
				sysId: sysId,
				owner: owner,
			},
		}
	})
	engine.RegMainCityRobotCalcFn(attrdef.RobotAttrSysFourSymbols, handleRobotAttrSysFourSymbols)
}

func handleRobotAttrSysFourSymbols(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	handleRobotAttrSysDragonHolySoul(robot, calc)
	handleRobotAttrSysTigerHolySoul(robot, calc)
	handleRobotAttrSysRoseFinchHolySoul(robot, calc)
	handleRobotAttrSysTortoiseHolySoul(robot, calc)
}
func handleRobotAttrSysDragonHolySoul(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	fsConf := jsondata.GetFourSymbolsConf(custom_id.FourSymbolsDragon)
	if nil == fsConf || data.DragonHolySoulData == nil || data.DragonHolySoulData.Level <= 0 || data.DragonHolySoulData.Level > uint32(len(fsConf.LevelCof)) {
		return
	}
	lvConf := fsConf.LevelCof[data.DragonHolySoulData.Level]
	CheckAddAttrsToCalc(robot, calc, lvConf.Attrs)
}
func handleRobotAttrSysTigerHolySoul(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	fsConf := jsondata.GetFourSymbolsConf(custom_id.FourSymbolsTiger)
	if nil == fsConf || data.TigerHolySoulData == nil || data.TigerHolySoulData.Level <= 0 || data.TigerHolySoulData.Level > uint32(len(fsConf.LevelCof)) {
		return
	}
	lvConf := fsConf.LevelCof[data.DragonHolySoulData.Level]
	CheckAddAttrsToCalc(robot, calc, lvConf.Attrs)
}
func handleRobotAttrSysRoseFinchHolySoul(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	fsConf := jsondata.GetFourSymbolsConf(custom_id.FourSymbolsRosefinch)
	if nil == fsConf || data.RoseFinchHolySoulData == nil || data.RoseFinchHolySoulData.Level <= 0 || data.DragonHolySoulData.Level > uint32(len(fsConf.LevelCof)) {
		return
	}
	lvConf := fsConf.LevelCof[data.DragonHolySoulData.Level]
	if lvConf == nil {
		return
	}
	CheckAddAttrsToCalc(robot, calc, lvConf.Attrs)
}
func handleRobotAttrSysTortoiseHolySoul(robot iface.IRobot, calc *attrcalc.FightAttrCalc) {
	data := robot.GetData()
	fsConf := jsondata.GetFourSymbolsConf(custom_id.FourSymbolsTortoise)
	if nil == fsConf || data.TortoiseHolySoulData == nil || data.TortoiseHolySoulData.Level <= 0 || data.TortoiseHolySoulData.Level > uint32(len(fsConf.LevelCof)) {
		return
	}
	lvConf := fsConf.LevelCof[data.DragonHolySoulData.Level]
	CheckAddAttrsToCalc(robot, calc, lvConf.Attrs)
}

func (sys *FourSymbolsSys) checkCircleAndLevel(typ uint32) bool {
	owner := sys.GetOwner()
	fourSymbolConf := jsondata.GetFourSymbolsConf(typ)
	if fourSymbolConf == nil {
		return false
	}

	// 检查子系统开启是否正常
	if !sys.canOpenSys(fourSymbolConf.SubSysId) {
		return false
	}

	// 检查开启条件
	boundaryLv := owner.GetAttr(attrdef.Circle)
	if fourSymbolConf.Condition > uint32(boundaryLv) {
		return false
	}
	level := owner.GetLevel()
	if fourSymbolConf.ActorLevel > level {
		return false
	}
	return true
}

func (sys *FourSymbolsSys) canOpenSys(id uint32) bool {
	conf := jsondata.GetSysOpenConf(id)
	if nil == conf {
		return true //没配的默认开启
	}

	//开服天数
	if conf.OpenSrvDay > 0 && conf.OpenSrvDay > gshare.GetOpenServerDay() {
		return false
	}

	//合服次数
	if conf.MergeTimes > 0 && conf.MergeTimes > gshare.GetMergeTimes() {
		return false
	}

	//跨服次数
	if conf.CrossTimes > 0 {
		if conf.CrossTimes > gshare.GetCrossAllocTimes() {
			return false
		}

		if conf.CrossTimes == gshare.GetCrossAllocTimes() {
			//跨服天数
			if conf.CrossDay > 0 && conf.CrossDay > gshare.GetSmallCrossDay() {
				return false
			}
		}
	}

	//合服天数
	if conf.MergeDay > 0 && conf.MergeDay > gshare.GetMergeSrvDay() {
		return false
	}

	return true
}
