/**
 * @Author: yzh
 * @Date:
 * @Desc: 灵符
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

type CimeliaSys struct {
	Base
}

const (
	CimeliaFirst    = 1 // 天谴灵符
	CimeliaSecond   = 2 // 诛邪灵符
	CimeliaThird    = 3 // 封灵灵符
	CimeliaFourthly = 4 // 往生灵符
)

func (sys *CimeliaSys) OnAfterLogin() {
	sys.sendState()
}

func (sys *CimeliaSys) GetCimeliaData(slType uint32) (*pb3.KeyValue, bool) {
	if conf := jsondata.GetShenLuConf(slType); nil == conf {
		return nil, false
	}

	for _, line := range sys.owner.GetBinaryData().Cimelia {
		if line.GetKey() == slType {
			return line, true
		}
	}

	return nil, false
}

// 升级
func (sys *CimeliaSys) Upgrade(slType uint32) bool {
	conf := jsondata.GetShenLuConf(slType)
	if nil == conf {
		return false
	}

	data, existData := sys.GetCimeliaData(slType)
	var preLv, nextLv uint32
	if existData {
		preLv = data.GetValue()
		nextLv = preLv + 1
		if nextLv > uint32(len(conf.LvConf)) {
			return false
		}
	} else {
		preLv = 0
		nextLv = 1
		data = new(pb3.KeyValue)
		data.Key = slType
		data.Value = nextLv
	}

	lvConf := conf.LvConf[nextLv]
	if nil == lvConf {
		return false
	}

	if !sys.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeUpCimeliaLv}) {
		return false
	}

	if !existData {
		sys.owner.GetBinaryData().Cimelia = append(sys.owner.GetBinaryData().Cimelia, data)
	} else {
		data.Value = nextLv
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"oldLv": preLv,
		"newLv": nextLv,
	})
	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogCimeliaUpgrade, &pb3.LogPlayerCounter{
		NumArgs: uint64(slType),
		StrArgs: string(logArg),
	})
	sys.owner.TriggerQuestEvent(custom_id.QttCiMeLiaLevel, slType, int64(nextLv))

	sys.ResetSysAttr(attrdef.SaCimeliaProperty)

	if lvConf.Bro > 0 {
		engine.BroadcastTipMsgById(lvConf.Bro, sys.owner.GetName(), conf.Name, lvConf.Stage)
	}

	return true
}

// 升级
func (sys *CimeliaSys) c2sUpgrade(msg *base.Message) {
	var req pb3.C2S_144_1
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	if !sys.Upgrade(req.Type) {
		return
	}

	data, _ := sys.GetCimeliaData(req.Type)
	sys.SendProto3(144, 1, &pb3.S2C_144_1{
		Data: data,
	})
}

// 下发信息
func (sys *CimeliaSys) sendState() {
	sys.SendProto3(144, 2, &pb3.S2C_144_2{
		State: sys.state(),
	})
}

func (sys *CimeliaSys) OnReconnect() {
	sys.sendState()
}

func (sys *CimeliaSys) state() []*pb3.KeyValue {
	return sys.owner.GetBinaryData().Cimelia
}

// 重算属性
func calcCimeliaProperty(actor iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	for _, line := range actor.GetBinaryData().Cimelia {
		slType, lv := line.GetKey(), line.GetValue()
		conf := jsondata.GetShenLuConf(slType)
		if nil == conf {
			continue
		}

		if lv > uint32(len(conf.LvConf)) {
			continue
		}

		lvConf := conf.LvConf[lv]
		engine.CheckAddAttrsToCalc(actor, calc, lvConf.Attrs)
	}
}
