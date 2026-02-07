/**
 * @Author: LvYuMeng
 * @Date: 2024/1/2
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type PkSys struct {
	Base
	pkValueTimer *time_util.Timer
}

func (sys *PkSys) IsOpen() bool {
	return true
}

func (sys *PkSys) OnLogin() {
	sys.owner.SetExtraAttr(attrdef.PKValue, int64(sys.GetBinaryData().PKValue))
	sys.checkSetPkValueTimer()
}

func (sys *PkSys) OnReconnect() {
}

func (sys *PkSys) AddPkValue(value uint32) {
	pkValue := sys.GetBinaryData().PKValue + value
	sys.GetBinaryData().PKValue = pkValue
	sys.owner.SetExtraAttr(attrdef.PKValue, int64(pkValue))
	sys.checkSetPkValueTimer()
}

func (sys *PkSys) SubPkValue(value uint32) {
	pkValue := sys.GetBinaryData().PKValue
	if pkValue <= value {
		pkValue = 0
	} else {
		pkValue -= value
	}

	sys.GetBinaryData().PKValue = pkValue
	sys.owner.SetExtraAttr(attrdef.PKValue, int64(pkValue))
}

func (sys *PkSys) checkPkValueTimerExit() {
	if cur := sys.owner.GetExtraAttr(attrdef.PKValue); cur <= 0 {
		if nil != sys.pkValueTimer {
			sys.pkValueTimer.Stop()
			sys.pkValueTimer = nil
		}
	}
}

func (sys *PkSys) checkSetPkValueTimer() {
	sys.checkPkValueTimerExit()
	if nil != sys.pkValueTimer {
		return
	}
	sys.pkValueTimer = sys.owner.SetInterval(time.Minute, func() {
		if conf := jsondata.GetPkConf(); nil != conf {
			sys.SubPkValue(conf.SubPkValuePerMin)
		}
		sys.checkPkValueTimerExit()
	})
}

func useItemSubPKValuePotion(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiPK).(*PkSys)
	if !ok {
		return false, false, 0
	}

	value := player.GetExtraAttrU32(attrdef.PKValue)
	if value <= 0 {
		return
	}

	subValue := conf.Param[0]
	if subValue <= 0 {
		player.LogError("useItem:%d err: pk sub is zero!", param.ItemId)
		return false, false, 0
	}

	cnt = utils.MinInt64(param.Count, int64((value+subValue-1)/subValue))

	subValue = subValue * uint32(cnt)
	if subValue > value {
		subValue = value
	}
	sys.SubPkValue(subValue)
	return true, true, cnt
}

func opPKValue(player iface.IPlayer, buf []byte) {
	if len(buf) == 0 {
		return
	}
	var st pb3.OpPkValue
	if err := pb3.Unmarshal(buf, &st); nil != err {
		player.LogError("unmarshal OpPkValue err:%v", err)
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiPK).(*PkSys)
	if !ok {
		return
	}
	if st.IsAdd {
		sys.AddPkValue(st.Val)
	} else {
		sys.SubPkValue(st.Val)
	}

}

func gmPkAdd(player iface.IPlayer, args ...string) bool {
	val := utils.AtoUint32(args[0])
	sys, ok := player.GetSysObj(sysdef.SiPK).(*PkSys)
	if !ok {
		return false
	}
	sys.GetBinaryData().PKValue = val
	sys.owner.SetExtraAttr(attrdef.PKValue, int64(val))
	sys.checkSetPkValueTimer()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiPK, func() iface.ISystem {
		return &PkSys{}
	})

	gmevent.Register("addpk", gmPkAdd, 1)

	engine.RegisterActorCallFunc(playerfuncid.OpPkValue, opPKValue)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemSubPKValuePotion, useItemSubPKValuePotion)
}
