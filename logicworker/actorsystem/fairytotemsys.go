/**
 * @Author: LvYuMeng
 * @Date: 2025/2/21
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type FairyTotemSys struct {
	Base
}

func (s *FairyTotemSys) getData() *pb3.FairyTotem {
	binary := s.GetBinaryData()
	if nil == binary.FairyTotem {
		binary.FairyTotem = &pb3.FairyTotem{}
	}
	if nil == binary.FairyTotem.Stars {
		binary.FairyTotem.Stars = make(map[uint32]uint32)
	}
	return binary.FairyTotem
}

func (s *FairyTotemSys) OnOpen() {
	s.s2cInfo()
}

func (s *FairyTotemSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FairyTotemSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *FairyTotemSys) s2cInfo() {
	s.SendProto3(27, 55, &pb3.S2C_27_55{
		Data: s.getData(),
	})
}

func (s *FairyTotemSys) c2sStarUp(msg *base.Message) error {
	var req pb3.C2S_27_56
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	id := req.GetId()
	data := s.getData()

	var nextStar uint32
	if nowStar, isActive := data.Stars[id]; isActive {
		nextStar = nowStar + 1
	}

	starConf, ok := jsondata.GetFairyTotemStarConf(id, nextStar)
	if !ok {
		return neterror.ConfNotFoundError("conf %d star %d is nil", id, nextStar)
	}

	if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyTotemStarUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Stars[id] = nextStar
	s.SendProto3(27, 56, &pb3.S2C_27_56{
		Id:   id,
		Star: data.Stars[id],
	})
	s.owner.TriggerEvent(custom_id.AeReCalcFairyMainPower)
	s.ResetSysAttr(attrdef.SaFairyTotem)
	return nil
}

func (s *FairyTotemSys) calcFairyAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for id, star := range data.Stars {
		starConf, ok := jsondata.GetFairyTotemStarConf(id, star)
		if !ok {
			continue
		}
		for _, line := range starConf.FairyAttrs {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}

func (s *FairyTotemSys) calcPlayerAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for id, star := range data.Stars {
		starConf, ok := jsondata.GetFairyTotemStarConf(id, star)
		if !ok {
			continue
		}
		for _, line := range starConf.PlayerAttr {
			calc.AddValue(line.Type, attrdef.AttrValueAlias(line.Value))
		}
	}
}

func calcFairyTotemProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if s := player.GetSysObj(sysdef.SiFairyTotem); s != nil && s.IsOpen() {
		fairyTotemSys, ok := s.(*FairyTotemSys)
		if !ok {
			return
		}
		fairyTotemSys.calcPlayerAttrs(calc)
	}
}

func init() {
	RegisterSysClass(sysdef.SiFairyTotem, func() iface.ISystem {
		return &FairyTotemSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairyTotem, calcFairyTotemProperty)

	net.RegisterSysProtoV2(27, 56, sysdef.SiFairyTotem, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FairyTotemSys).c2sStarUp
	})

	gmevent.Register("fairyTotem", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFairyTotem)
		if nil == obj {
			return false
		}

		id, star := utils.AtoUint32(args[0]), utils.AtoUint32(args[1])
		totemSys, ok := obj.(*FairyTotemSys)
		if !ok || !totemSys.IsOpen() {
			return false
		}
		data := totemSys.getData()
		data.Stars[id] = star
		totemSys.s2cInfo()
		player.TriggerEvent(custom_id.AeReCalcFairyMainPower)

		return true
	}, 1)
}
