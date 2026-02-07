package engine

import (
	micro "github.com/gzjjyz/simple-micro"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/srvlib/utils"
)

var ClassSet = make(map[uint32]func() iface.ISystem)

// CheckAddAttrsToCalc 加属性
func CheckAddAttrsToCalc(player iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr) {
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

// AddAttrsToCalc 加属性跳过检查
func AddAttrsToCalc(player iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr) {
	if nil == calc {
		return
	}
	var attrTypes []uint32
	for _, line := range attrs {
		attrTypes = append(attrTypes, line.Type)
		calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
	}
}

// CheckAddAttrsToCalcTimes 加属性倍数
func CheckAddAttrsToCalcTimes(player iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr, times uint32) {
	if nil == calc {
		return
	}
	job := player.GetJob()
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value*times))
		}
	}
}

// CheckAddAttrsByQualityToCalc 加属性 (添加品质限制)
func CheckAddAttrsSelectQualityToCalc(player iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr, quality uint32) {
	if nil == calc {
		return
	}
	job := player.GetJob()
	for _, line := range attrs {
		if (line.Job <= 0 || job == line.Job) && quality >= line.EffectiveLimit {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}

// CheckAddAttrs 加属性
func CheckAddAttrs(actor iface.IPlayer, attrMap map[uint32]uint32, attrs []*jsondata.Attr) {
	if nil == attrMap {
		return
	}
	job := actor.GetJob()
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			attrMap[line.Type] += line.Value
		}
	}
}

// CheckAddAttrsByJob 加属性
func CheckAddAttrsByJob(job uint32, attrMap map[uint32]uint32, attrs []*jsondata.Attr) {
	if nil == attrMap {
		return
	}
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			attrMap[line.Type] += line.Value
		}
	}
}

// CheckAddAttrsTimes 加属性倍数
func CheckAddAttrsTimes(actor iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr, times uint32) {
	if nil == calc {
		return
	}
	job := actor.GetJob()
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value*times))
		}
	}
}

// CheckAddAttrsRate 属性加成
func CheckAddAttrsRate(actor iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr, addRate uint32) {
	if nil == calc {
		return
	}
	job := actor.GetJob()
	for _, line := range attrs {
		if line.Job <= 0 || job == line.Job {
			temp := utils.CalcMillionRate64(int64(line.Value), int64(addRate))
			calc.AddValue(line.Type, temp)
		}
	}
}

// CheckAddAttrsRateRoundingUp 属性加成向上取整
func CheckAddAttrsRateRoundingUp(actor iface.IPlayer, calc *attrcalc.FightAttrCalc, attrs []*jsondata.Attr, addRate uint32, excludeAttr ...uint32) {
	if nil == calc {
		return
	}
	if addRate == 0 {
		return
	}
	job := actor.GetJob()
	var calcMillionRate = func(base, rate int64) int64 {
		// 为了向上取整，先将rate乘以base并加上9999（确保在除以10000时能正确向上取整）
		// 然后除以10000得到的结果即为向上取整后的百万分率
		return (rate*base + 9999) / 10000
	}
	for _, line := range attrs {
		if pie.Uint32s(excludeAttr).Contains(line.Type) {
			continue
		}
		if line.Job <= 0 || job == line.Job {
			temp := calcMillionRate(int64(line.Value), int64(addRate))
			if temp == 0 {
				continue
			}
			calc.AddValue(line.Type, temp)
		}
	}
}

func GetServerId() uint32 {
	return gshare.GameConf.SrvId
}

func GetPfId() uint32 {
	return gshare.GameConf.PfId
}

func GetAppId() uint32 {
	if micro.MustMeta() == nil {
		return 0
	}
	return micro.MustMeta().AppId
}
