/**
 * @Author: HeXinLi
 * @Desc: 升级系统基类
 * @Date: 2021/9/8 15:24
 */

package uplevelbase

import (
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type IUplevelAbstract interface {
	OnLevelSysUp()
	AfterDoUplevel()
}

type UplevelBase struct {
	Exp          uint32    // 经验
	Level        uint32    // 等级
	ConsumeLogId pb3.LogId // 消耗logid
	abstract     IUplevelAbstract
	conf         *jsondata.LvConfVec
}

func (b *UplevelBase) SetAbstract(abstract IUplevelAbstract) {
	b.abstract = abstract
}

func (b *UplevelBase) SetLevelConf(conf *jsondata.LvConfVec) {
	b.conf = conf
}

func (b *UplevelBase) GetCurLvConf() *jsondata.LvConfBase {
	if lvConf, ok := (*b.conf)[b.Level]; ok {
		return lvConf
	}
	return nil
}

func (b *UplevelBase) GetNextLvConf() *jsondata.LvConfBase {
	if lvConf, ok := (*b.conf)[b.Level+1]; ok {
		return lvConf
	}
	return nil
}

func (b *UplevelBase) UplevelCheck(actor iface.IPlayer, autoBuy bool, lvConf *jsondata.LvConfBase) bool {
	// 等级上限检测
	if nil == b.GetNextLvConf() {
		return false
	}

	// 消耗检测
	if !actor.ConsumeByConf(lvConf.Consume, autoBuy, common.ConsumeParams{LogId: b.ConsumeLogId}) {
		return false
	}

	return true
}

func (b *UplevelBase) DoUplevel(actor iface.IPlayer, autoBuy bool) bool {
	lvConf := b.GetCurLvConf()
	if nil == lvConf {
		return false
	}

	if !b.UplevelCheck(actor, autoBuy, lvConf) {
		return false
	}

	b.Exp += lvConf.AddExp
	for {
		if lvConf.ExpLimit <= b.Exp {
			b.Exp -= lvConf.ExpLimit
			b.Level = b.Level + 1

			b.abstract.OnLevelSysUp()

			lvConf = b.GetCurLvConf()
			if nil == b.GetNextLvConf() {
				break
			}
		} else {
			break
		}
	}

	b.abstract.AfterDoUplevel()
	return true
}

func (b *UplevelBase) GetLevelAttrs(job uint32, attrMap map[uint32]uint32) {
	lvConf := b.GetCurLvConf()
	if nil == lvConf {
		return
	}

	engine.CheckAddAttrsByJob(job, attrMap, lvConf.Attrs)
}

func (b *UplevelBase) OnLevelSysUp()   {}
func (b *UplevelBase) AfterDoUplevel() {}
