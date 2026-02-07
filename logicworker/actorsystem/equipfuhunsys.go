/**
 * @Author: lzp
 * @Date: 2024/7/17
 * @Desc:
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

// 附魂条件类型
const (
	FuHunCondEquipQuality = 1 //附魂 - 穿戴的X装备品质达N
	FuHunCondEquipStar    = 2 //附魂 - 穿戴的X装备星级达N
	FuHunCondEquipStage   = 3 //附魂 - 穿戴的X装备阶级达N
)

type FuHunSys struct {
	Base
}

func (sys *FuHunSys) OnInit() {
	binary := sys.GetBinaryData()
	if binary.FuFun == nil {
		binary.FuFun = make(map[uint32]uint32)
	}
}

func (sys *FuHunSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *FuHunSys) OnOpen() {
	sys.ResetSysAttr(attrdef.SaEquipFuHun)
	sys.s2cInfo()
}

func (sys *FuHunSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *FuHunSys) s2cInfo() {
	sys.SendProto3(11, 60, &pb3.S2C_11_60{
		FuHun: sys.GetBinaryData().FuFun,
	})
}

func (sys *FuHunSys) c2sFuHun(msg *base.Message) error {
	var req pb3.C2S_11_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	pos := req.Pos
	if pos < itemdef.EtBegin || pos > itemdef.EtEnd {
		return neterror.ParamsInvalidError("not equip pos(%d)", pos)
	}

	binary := sys.GetBinaryData()
	lvConf := jsondata.GetFuHunLvConf(pos, binary.FuFun[pos]+1)
	if lvConf == nil {
		return neterror.ParamsInvalidError("equip fuHun conf(%d) is nil", binary.FuFun[pos]+1)
	}

	var equip *pb3.ItemSt
	if equipSys, ok := sys.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip = equipSys.GetEquipByPos(pos)
	}
	if equip == nil {
		return neterror.ParamsInvalidError("no equip in pos(%d)", pos)
	}

	equipConf := jsondata.GetItemConfig(equip.GetItemId())
	if equipConf == nil {
		return neterror.ParamsInvalidError("equip item conf(%d) is nil", equip.GetItemId())
	}

	if !sys.checkFuHunCond(equipConf, lvConf) {
		return neterror.ParamsInvalidError("equip fuHun cond(%d) limit", equip.GetItemId())
	}

	if !sys.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipFuHunConsume}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	preLv := binary.FuFun[pos]
	binary.FuFun[pos] += 1

	logArg, _ := json.Marshal(map[string]interface{}{
		"pos":   pos,
		"oldLv": preLv,
		"newLv": binary.FuFun[pos],
	})

	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogEquipFuHunConsume, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
	sys.ResetSysAttr(attrdef.SaEquipFuHun)

	sys.SendProto3(11, 61, &pb3.S2C_11_61{
		Ret:  true,
		Data: &pb3.KeyValue{Key: pos, Value: binary.FuFun[pos]},
	})
	return nil
}

func (sys *FuHunSys) checkFuHunCond(eConf *jsondata.ItemConf, conf *jsondata.FuHunLv) bool {
	condVec := conf.Cond
	if condVec == nil {
		return true
	}

	for _, cond := range condVec {
		switch cond.Type {
		case FuHunCondEquipQuality:
			return eConf.Quality >= cond.Count
		case FuHunCondEquipStar:
			return eConf.Star >= cond.Count
		case FuHunCondEquipStage:
			return eConf.Stage >= cond.Count
		}
	}
	return true
}

func calcEquipFuHunSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	binary := player.GetBinaryData()
	if binary == nil {
		return
	}

	for pos, lv := range binary.FuFun {
		conf := jsondata.GetFuHunLvConf(pos, lv)
		if conf == nil {
			continue
		}
		var attrs jsondata.AttrVec
		attrs = append(attrs, conf.Attrs1...)
		attrs = append(attrs, conf.Attrs2...)
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiFuHun, func() iface.ISystem {
		return &FuHunSys{}
	})

	net.RegisterSysProtoV2(11, 61, sysdef.SiFuHun, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FuHunSys).c2sFuHun
	})
	engine.RegAttrCalcFn(attrdef.SaEquipFuHun, calcEquipFuHunSysAttr)
}
