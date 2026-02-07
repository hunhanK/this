/**
 * @Author:
 * @Date:
 * @Desc: 飞剑玉符洗炼
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FlyingSwordEquipRefineSys struct {
	Base
}

func (s *FlyingSwordEquipRefineSys) s2cInfo() {
	s.SendProto3(8, 150, &pb3.S2C_8_150{
		Data: s.getData(),
	})
}

func (s *FlyingSwordEquipRefineSys) getData() *pb3.FlyingSwordEquipRefineData {
	data := s.GetBinaryData().FlyingSwordEquipRefineData
	if data == nil {
		s.GetBinaryData().FlyingSwordEquipRefineData = &pb3.FlyingSwordEquipRefineData{}
		data = s.GetBinaryData().FlyingSwordEquipRefineData
	}
	if data.RefineMap == nil {
		data.RefineMap = make(map[uint32]*pb3.FlyingSwordEquipRefineEntry)
	}
	return data
}

func (s *FlyingSwordEquipRefineSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FlyingSwordEquipRefineSys) OnLogin() {
	s.initPos()
	s.s2cInfo()
}

func (s *FlyingSwordEquipRefineSys) OnOpen() {
	s.initPos()
	s.s2cInfo()
	s.resetSysAttr()
}

func (s *FlyingSwordEquipRefineSys) resetSysAttr() {
	s.ResetSysAttr(attrdef.SaFlyingSwordEquipRefine)
}

func (s *FlyingSwordEquipRefineSys) initPos() {
	data := s.getData()
	jsondata.EachFlyingSwordEquipRefineConfig(func(config *jsondata.FlyingSwordEquipRefineConfig) {
		refineEntry, ok := data.RefineMap[config.Pos]
		if !ok {
			data.RefineMap[config.Pos] = &pb3.FlyingSwordEquipRefineEntry{}
			refineEntry = data.RefineMap[config.Pos]
			refineEntry.Pos = config.Pos
		}
		s.refreshNewAttr(refineEntry.Pos)
	})
}

func (s *FlyingSwordEquipRefineSys) refreshNewAttr(pos uint32) {
	data := s.getData()
	refineEntry, ok := data.RefineMap[pos]
	if !ok {
		return
	}
	lvConfig := jsondata.GetFlyingSwordEquipRefineLvConfig(pos, refineEntry.Lv)
	if lvConfig == nil {
		s.LogWarn("%d %d config not found", pos, 1)
		return
	}
	var attrStMap = make(map[uint32]*pb3.AttrSt)
	for _, attrSt := range refineEntry.Attrs {
		attrStMap[attrSt.Type] = attrSt
	}
	for _, attrPool := range lvConfig.AttrPool {
		_, ok := attrStMap[attrPool.Type]
		if ok {
			continue
		}
		attrStMap[attrPool.Type] = &pb3.AttrSt{
			Type:  attrPool.Type,
			Value: attrPool.Value,
		}
	}
	refineEntry.Attrs = nil
	for _, attrSt := range attrStMap {
		refineEntry.Attrs = append(refineEntry.Attrs, attrSt)
	}
}

func (s *FlyingSwordEquipRefineSys) commonCheckPos(pos uint32) error {
	owner := s.GetOwner()
	obj := owner.GetSysObj(sysdef.SiFlyingSwordEquip)
	if obj == nil || !obj.IsOpen() {
		return neterror.ParamsInvalidError("not found FlyingSwordEquip")
	}

	equip, ok := obj.(iface.IEquip)
	if !ok {
		return neterror.ParamsInvalidError("check equip failed")
	}

	if !equip.CheckEquipPosTakeOn(pos) {
		return neterror.ParamsInvalidError("not found pos %d", pos)
	}
	return nil
}

func (s *FlyingSwordEquipRefineSys) c2sRefine(msg *base.Message) error {
	var req pb3.C2S_8_151
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	if err := s.commonCheckPos(req.Pos); err != nil {
		return neterror.Wrap(err)
	}

	data := s.getData()
	refineEntry, ok := data.RefineMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("not open pos %d data", req.Pos)
	}

	config := jsondata.GetFlyingSwordEquipRefineLvConfig(refineEntry.Pos, refineEntry.Lv)
	if config == nil {
		return neterror.ConfNotFoundError("%d %d config not found", refineEntry.Pos, refineEntry.Lv)
	}

	var attrStMap = make(map[uint32]*pb3.AttrSt)
	for _, attrSt := range refineEntry.Attrs {
		attrStMap[attrSt.Type] = attrSt
	}

	var randomPool = new(random.Pool)
	for _, attr := range config.AttrPool {
		attrSt, ok := attrStMap[attr.Type]
		if !ok {
			return neterror.ParamsInvalidError("not found attr %d", attr.Type)
		}
		if attr.UpLimit <= attrSt.Value {
			continue
		}
		randomPool.AddItem(attr, attr.Weight)
	}
	if randomPool.Size() == 0 {
		return neterror.ParamsInvalidError("not can add value attr")
	}

	refineAttr := randomPool.RandomOne().(*jsondata.FlyingSwordEquipRefineAttr)
	if refineAttr == nil {
		return neterror.ParamsInvalidError("refine attr is nil")
	}

	attrSt, ok := attrStMap[refineAttr.Type]
	if !ok {
		return neterror.ParamsInvalidError("not found attr data %d", refineAttr.Type)
	}

	if len(refineAttr.AddRange) < 2 {
		return neterror.ParamsInvalidError("refine attr add range is %v", refineAttr.AddRange)
	}

	owner := s.GetOwner()
	if len(config.RefineConsume) == 0 || !owner.ConsumeByConf(config.RefineConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordEquipRefine}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	randomValue := random.IntervalUU(refineAttr.AddRange[0], refineAttr.AddRange[1])
	attrSt.Value += randomValue
	if attrSt.Value > refineAttr.UpLimit {
		attrSt.Value = refineAttr.UpLimit
	}

	s.SendProto3(8, 151, &pb3.S2C_8_151{
		Entry: refineEntry,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFlyingSwordEquipRefine, &pb3.LogPlayerCounter{
		NumArgs: uint64(refineEntry.Pos),
		StrArgs: fmt.Sprintf("%d_%d", attrSt.Value, refineAttr.UpLimit),
	})
	s.resetSysAttr()
	owner.TriggerQuestEvent(custom_id.QttFlyingSwordEquipRefine, 0, 1)
	return nil
}
func (s *FlyingSwordEquipRefineSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_8_152
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	if err := s.commonCheckPos(req.Pos); err != nil {
		return neterror.Wrap(err)
	}

	data := s.getData()
	refineEntry, ok := data.RefineMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("not open pos %d data", req.Pos)
	}

	var nextLv = refineEntry.Lv + 1
	nextLvConf := jsondata.GetFlyingSwordEquipRefineLvConfig(refineEntry.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("%d %d config not found", refineEntry.Pos, nextLv)
	}

	owner := s.GetOwner()
	if len(nextLvConf.UpConsume) == 0 || !owner.ConsumeByConf(nextLvConf.UpConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordEquipRefineUpLv}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	refineEntry.Lv += 1
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFlyingSwordEquipRefineUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(refineEntry.Pos),
		StrArgs: fmt.Sprintf("%d", refineEntry.Lv),
	})
	s.refreshNewAttr(refineEntry.Pos)
	s.resetSysAttr()
	s.SendProto3(8, 152, &pb3.S2C_8_152{
		Entry: refineEntry,
	})
	return nil
}

func (s *FlyingSwordEquipRefineSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	jsondata.EachFlyingSwordEquipRefineConfig(func(config *jsondata.FlyingSwordEquipRefineConfig) {
		if err := s.commonCheckPos(config.Pos); err != nil {
			return
		}
		entry := data.RefineMap[config.Pos]
		if entry == nil {
			return
		}
		for _, attr := range entry.Attrs {
			calc.AddValue(attr.Type, attrdef.AttrValueAlias(attr.Value))
		}
		lvConfig := jsondata.GetFlyingSwordEquipRefineLvConfig(config.Pos, entry.Lv)
		if lvConfig == nil {
			return
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, lvConfig.Attrs)
	})
}

func flyingSwordEquipRefineProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiFlyingSwordEquipRefine)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys := obj.(*FlyingSwordEquipRefineSys)
	if sys == nil {
		return
	}

	sys.calcAttr(calc)
}

func handleSeOptFlyingSwordEquip(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiFlyingSwordEquipRefine)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*FlyingSwordEquipRefineSys)
	if sys == nil {
		return
	}
	sys.resetSysAttr()
}

func init() {
	RegisterSysClass(sysdef.SiFlyingSwordEquipRefine, func() iface.ISystem {
		return &FlyingSwordEquipRefineSys{}
	})

	net.RegisterSysProtoV2(8, 151, sysdef.SiFlyingSwordEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipRefineSys).c2sRefine
	})
	net.RegisterSysProtoV2(8, 152, sysdef.SiFlyingSwordEquipRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyingSwordEquipRefineSys).c2sUpLv
	})
	engine.RegAttrCalcFn(attrdef.SaFlyingSwordEquipRefine, flyingSwordEquipRefineProperty)
	event.RegActorEvent(custom_id.SeOptFlyingSwordEquip, handleSeOptFlyingSwordEquip)
	gmevent.Register("FlyingSwordEquipRefineSys.fullAttr", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFlyingSwordEquipRefine)
		if obj == nil || !obj.IsOpen() {
			return false
		}

		pos := utils.AtoUint32(args[0])
		s := obj.(*FlyingSwordEquipRefineSys)
		if s == nil {
			return false
		}

		data := s.getData()
		refineEntry, ok := data.RefineMap[pos]
		if !ok {
			return false
		}

		config := jsondata.GetFlyingSwordEquipRefineLvConfig(refineEntry.Pos, refineEntry.Lv)
		if config == nil {
			return false
		}

		var attrStMap = make(map[uint32]*pb3.AttrSt)
		for _, attrSt := range refineEntry.Attrs {
			attrStMap[attrSt.Type] = attrSt
		}

		for _, attr := range config.AttrPool {
			attrSt, ok := attrStMap[attr.Type]
			if !ok {
				return false
			}
			if attr.UpLimit <= attrSt.Value {
				continue
			}
			attrSt.Value = attr.UpLimit
		}
		s.s2cInfo()
		return true
	}, 1)
}
