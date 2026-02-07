/**
 * @Author: LvYuMeng
 * @Date: 2025/12/24
 * @Desc: 战纹洗炼
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type WarPaintRefineSys struct {
	Base
}

func (s *WarPaintRefineSys) GetSysData() *pb3.WarPaintRefineSysData {
	binary := s.GetBinaryData()

	if nil == binary.WarPaintRefineData {
		binary.WarPaintRefineData = &pb3.WarPaintRefineData{}
	}

	if nil == binary.WarPaintRefineData.SysData {
		binary.WarPaintRefineData.SysData = make(map[uint32]*pb3.WarPaintRefineSysData)
	}

	sysId := s.GetSysId()
	sysData, ok := binary.WarPaintRefineData.SysData[sysId]
	if !ok {
		sysData = &pb3.WarPaintRefineSysData{}
		binary.WarPaintRefineData.SysData[sysId] = sysData
	}

	if nil == sysData.RefineMap {
		sysData.RefineMap = make(map[uint32]*pb3.WarPaintEquipRefineEntry)
	}

	return sysData
}

func (s *WarPaintRefineSys) s2cInfo() {
	s.SendProto3(15, 150, &pb3.S2C_15_150{
		SysId: s.GetSysId(),
		Data:  s.GetSysData(),
	})
}

func (s *WarPaintRefineSys) getRef() (*gshare.WarPaintRef, bool) {
	ref, ok := gshare.WarPaintInstance.FindWarPaintRefByWarPaintRefineSysId(s.GetSysId())
	return ref, ok
}

func (s *WarPaintRefineSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WarPaintRefineSys) OnLogin() {
	s.initPos()
	s.s2cInfo()
}

func (s *WarPaintRefineSys) OnOpen() {
	s.initPos()
	s.s2cInfo()
	s.resetSysAttr()
}

func (s *WarPaintRefineSys) resetSysAttr() {
	if ref, ok := s.getRef(); ok {
		s.ResetSysAttr(ref.CalAttrWarPaintRefineDef)
	}
}

func (s *WarPaintRefineSys) initPos() {
	data := s.GetSysData()
	jsondata.EachWarPaintEquipRefineConfig(s.GetSysId(), func(config *jsondata.WarPaintEquipRefineConfig) {
		refineEntry, ok := data.RefineMap[config.Pos]
		if !ok {
			data.RefineMap[config.Pos] = &pb3.WarPaintEquipRefineEntry{}
			refineEntry = data.RefineMap[config.Pos]
			refineEntry.Pos = config.Pos
		}
		s.refreshNewAttr(refineEntry.Pos)
	})
}

func (s *WarPaintRefineSys) refreshNewAttr(pos uint32) {
	data := s.GetSysData()
	refineEntry, ok := data.RefineMap[pos]
	if !ok {
		return
	}
	lvConfig := jsondata.GetWarPaintEquipRefineLvConfig(s.GetSysId(), pos, refineEntry.Lv)
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

func (s *WarPaintRefineSys) commonCheckPos(pos uint32) error {
	ref, ok := s.getRef()
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	owner := s.GetOwner()
	obj := owner.GetSysObj(ref.WarPaintSysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.ParamsInvalidError("not found WarPaintSys")
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

func (s *WarPaintRefineSys) c2sRefine(req *pb3.C2S_15_151) error {
	if err := s.commonCheckPos(req.Pos); err != nil {
		return neterror.Wrap(err)
	}

	ref, ok := s.getRef()
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	data := s.GetSysData()
	refineEntry, ok := data.RefineMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("not open pos %d data", req.Pos)
	}

	config := jsondata.GetWarPaintEquipRefineLvConfig(s.GetSysId(), refineEntry.Pos, refineEntry.Lv)
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

	refineAttr := randomPool.RandomOne().(*jsondata.WarPaintEquipRefineAttr)
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
	if len(config.RefineConsume) == 0 || !owner.ConsumeByConf(config.RefineConsume, false, common.ConsumeParams{LogId: ref.LogWarPaintEquipRefine}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	randomValue := random.IntervalUU(refineAttr.AddRange[0], refineAttr.AddRange[1])
	attrSt.Value += randomValue
	if attrSt.Value > refineAttr.UpLimit {
		attrSt.Value = refineAttr.UpLimit
	}

	s.SendProto3(15, 151, &pb3.S2C_15_151{
		SysId: s.GetSysId(),
		Entry: refineEntry,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), ref.LogWarPaintEquipRefine, &pb3.LogPlayerCounter{
		NumArgs: uint64(refineEntry.Pos),
		StrArgs: fmt.Sprintf("%d_%d", attrSt.Value, refineAttr.UpLimit),
	})
	s.resetSysAttr()
	return nil
}

func (s *WarPaintRefineSys) c2sUpLv(req *pb3.C2S_15_152) error {
	if err := s.commonCheckPos(req.Pos); err != nil {
		return neterror.Wrap(err)
	}
	ref, ok := s.getRef()
	if !ok {
		return neterror.ParamsInvalidError("not found %d ref sys", s.GetSysId())
	}
	data := s.GetSysData()
	refineEntry, ok := data.RefineMap[req.Pos]
	if !ok {
		return neterror.ParamsInvalidError("not open pos %d data", req.Pos)
	}

	var nextLv = refineEntry.Lv + 1
	nextLvConf := jsondata.GetWarPaintEquipRefineLvConfig(s.GetSysId(), refineEntry.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("%d %d config not found", refineEntry.Pos, nextLv)
	}

	owner := s.GetOwner()
	if len(nextLvConf.UpConsume) == 0 || !owner.ConsumeByConf(nextLvConf.UpConsume, false, common.ConsumeParams{LogId: ref.LogWarPaintEquipRefineUpLv}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	refineEntry.Lv += 1
	logworker.LogPlayerBehavior(s.GetOwner(), ref.LogWarPaintEquipRefineUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(refineEntry.Pos),
		StrArgs: fmt.Sprintf("%d", refineEntry.Lv),
	})
	s.refreshNewAttr(refineEntry.Pos)
	s.resetSysAttr()
	s.SendProto3(15, 152, &pb3.S2C_15_152{
		SysId: s.GetSysId(),
		Entry: refineEntry,
	})
	return nil
}

func (s *WarPaintRefineSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetSysData()
	jsondata.EachWarPaintEquipRefineConfig(s.GetSysId(), func(config *jsondata.WarPaintEquipRefineConfig) {
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
		lvConfig := jsondata.GetWarPaintEquipRefineLvConfig(s.GetSysId(), config.Pos, entry.Lv)
		if lvConfig == nil {
			return
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, lvConfig.Attrs)
	})
}

func GetWarPaintRefineSys(player iface.IPlayer, sysId uint32) (*WarPaintRefineSys, error) {
	sys := player.GetSysObj(sysId).(*WarPaintRefineSys)
	if sys == nil || !sys.IsOpen() {
		return nil, neterror.SysNotExistError("WarPaintRefineSys %d err is nil", sysId)
	}
	return sys, nil
}

func warPaintRefineSysDo(player iface.IPlayer, sysId uint32, fn func(sys *WarPaintRefineSys)) {
	if sys, err := GetWarPaintRefineSys(player, sysId); err == nil && sys.IsOpen() {
		fn(sys)
	}
	return
}

func regWarPaintRefineSys() {
	gshare.WarPaintInstance.EachWarPaintRefDo(func(ref *gshare.WarPaintRef) {
		RegisterSysClass(ref.WarPaintRefineSysId, func() iface.ISystem {
			return &WarPaintRefineSys{}
		})

		engine.RegAttrCalcFn(ref.CalAttrWarPaintRefineDef, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
			warPaintRefineSysDo(player, ref.WarPaintRefineSysId, func(sys *WarPaintRefineSys) {
				sys.calcAttr(calc)
			})
		})

		event.RegActorEvent(ref.SeOptWarPaintEquip, func(player iface.IPlayer, args ...interface{}) {
			warPaintRefineSysDo(player, ref.WarPaintRefineSysId, func(sys *WarPaintRefineSys) {
				sys.resetSysAttr()
			})
		})
	})

	net.RegisterProto(15, 151, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_151
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintRefineSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sRefine(&req)
	})

	net.RegisterProto(15, 152, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_15_152
		err := msg.UnPackPb3Msg(&req)
		if err != nil {
			return err
		}
		sys, err := GetWarPaintRefineSys(player, req.GetSysId())
		if err != nil {
			return err
		}
		return sys.c2sUpLv(&req)
	})
}

func init() {
	regWarPaintRefineSys()
}
