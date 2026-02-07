/**
 * @Author: ChenJunJi
 * @Desc: 机器人属性系统
 * @Date: 2022/5/27 20:05
 */

package system

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/alg/bitset"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type (
	FightPropSt  [attrdef.FightPropEnd + 1]uint32
	NormalPropSt [attrdef.ExtraAttrEnd - attrdef.ExtraAttrBegin + 1]uint32

	AttrSys struct {
		robot     iface.IRobot
		sysAttr   [attrdef.RobotAttrSysEnd]*attrcalc.FightAttrCalc // 系统属性
		fightAttr *attrcalc.FightAttrCalc                          // 战斗属性
		extraAttr *attrcalc.ExtraAttrCalc                          // 特殊属性

		ToFightProp *bitset.BitSet

		bNeedReset bool
	}
)

func CreateAttrSys(owner iface.IRobot) *AttrSys {
	return &AttrSys{
		robot:      owner,
		sysAttr:    [attrdef.RobotAttrSysEnd]*attrcalc.FightAttrCalc{},
		fightAttr:  &attrcalc.FightAttrCalc{},
		extraAttr:  &attrcalc.ExtraAttrCalc{},
		bNeedReset: false,
	}
}

func (sys *AttrSys) PackSyncData(data *pb3.RobotSync2Fight) {
	if nil == data {
		return
	}
	if nil == data.Propertys {
		data.Propertys = make(map[uint32]int64)
	}
	sys.fightAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		data.Propertys[t] = v
	})
	sys.extraAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		data.Propertys[t] = v
	})
}

func (sys *AttrSys) PackFightAttr(fightAttr map[uint32]int64) {
	if nil == fightAttr {
		fightAttr = make(map[uint32]int64)
	}
	sys.fightAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		fightAttr[t] = v
	})
}

func (sys *AttrSys) ResetAttr() {
	if !sys.bNeedReset {
		return
	}
	sys.bNeedReset = false

	sys.fightAttr.Reset()
	for _, calc := range sys.sysAttr {
		if nil != calc {
			sys.fightAttr.AddCalc(calc)
		}
	}
	sys.robot.SetFlagBit(custom_id.AfNeedSync)
}

func (sys *AttrSys) SetAttr(attrType uint32, attrValue attrdef.AttrValueAlias) {
	if attrType >= attrdef.FightPropBegin && attrType <= attrdef.FightPropEnd {
		sys.fightAttr.SetValue(attrType, attrValue)
	} else if attrType >= attrdef.ExtraAttrBegin && attrType <= attrdef.ExtraAttrEnd {
		sys.extraAttr.SetValue(attrType, attrValue)
	}
}

func (sys *AttrSys) GetAttr(attrType uint32) attrdef.AttrValueAlias {
	if attrType >= attrdef.FightPropBegin && attrType <= attrdef.FightPropEnd {
		return sys.fightAttr.GetValue(attrType)
	} else if attrType >= attrdef.ExtraAttrBegin && attrType <= attrdef.ExtraAttrEnd {
		return sys.extraAttr.GetValue(attrType)
	}
	return 0
}

func (sys *AttrSys) ResetSysAttr(sysId uint32) {
	fn := engine.GetMainCityRobotCalcFn(sysId)
	if nil == fn {
		logger.LogStack("未注册系统%d的属性计算回调函数", sysId)
		return
	}

	utils.ProtectRun(func() {
		calc := sys.sysAttr[sysId]
		if nil == calc {
			sys.sysAttr[sysId] = &attrcalc.FightAttrCalc{}
			calc = sys.sysAttr[sysId]
		}
		calc.Reset()
		fn(sys.robot, calc)
	})
	sys.bNeedReset = true
}

func (sys *AttrSys) DoUpdate() {
	sys.ResetAttr()
}

func (sys *AttrSys) Trace() {
	sys.fightAttr.Trace()
	sys.extraAttr.Trace()
}

func (sys *AttrSys) GetAttrCalcByAttrId(t attrdef.AttrTypeAlias) attrcalc.FightAttrCalc {
	attrId := attrdef.PlayerAttrSysToRobotAttrSys[int(t)]
	if nil == sys.sysAttr[attrId] {
		return attrcalc.FightAttrCalc{}
	}
	return *sys.sysAttr[attrId]
}

// CheckAddAttrsToCalc 加属性
func CheckAddAttrsToCalc(player iface.IRobot, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr) {
	if nil == calc {
		return
	}
	job := player.GetJob()
	var attrTypes []uint32
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			attrTypes = append(attrTypes, line.Type)
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}
