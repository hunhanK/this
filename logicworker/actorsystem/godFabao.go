package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/suitbase"
	"jjyz/gameserver/net"
)

/**
 * @Author: YangQibin
 * @Desc: 神兽
 * @Date: 2023/3/20
 */

type GodFabaoSys struct {
	Base
	data       *pb3.GodFabaoData
	fabaoChips map[uint32]*suitbase.ChipUpLvSuit
}

func (sys *GodFabaoSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *GodFabaoSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.ResetSysAttr(attrdef.SaGodFabao)
	sys.SendProto3(26, 0, &pb3.S2C_26_0{Data: sys.data})
}

func (sys *GodFabaoSys) OnLogin() {
	sys.SendProto3(26, 0, &pb3.S2C_26_0{Data: sys.data})
}

func (sys *GodFabaoSys) OnReconnect() {
	if !sys.init() {
		return
	}
	sys.SendProto3(26, 0, &pb3.S2C_26_0{Data: sys.data})
}

func (sys *GodFabaoSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.GodFabaoData == nil {
		binaryData.GodFabaoData = &pb3.GodFabaoData{
			Fabaos: make(map[uint32]*pb3.GodFabaoSt),
		}
	}

	sys.data = binaryData.GodFabaoData
	if sys.data.Fabaos == nil {
		sys.data.Fabaos = make(map[uint32]*pb3.GodFabaoSt)
	}

	sys.fabaoChips = make(map[uint32]*suitbase.ChipUpLvSuit)

	for _, id := range jsondata.GetGodFabaoIDs() {
		fabaoData := sys.data.Fabaos[id]

		if fabaoData == nil {
			fabaoData = &pb3.GodFabaoSt{
				Chips: make(map[uint32]*pb3.IdLvSt),
			}
			sys.data.Fabaos[id] = fabaoData
		}

		if fabaoData.Chips == nil {
			fabaoData.Chips = make(map[uint32]*pb3.IdLvSt)
		}

		suitIds := jsondata.GetGodFabaoSuitChipIds(id)

		if suitIds == nil {
			sys.LogError("GodFabaoSys init suitIds is nil, id: %v", id)
			return false
		}

		sys.fabaoChips[id] = &suitbase.ChipUpLvSuit{
			Chips:                  fabaoData.Chips,
			AttrSysId:              attrdef.SaGodFabao,
			SuitNum:                uint32(len(suitIds)),
			LogId:                  pb3.LogId_LogGodFabaoChipUpLv,
			GetChipIdBySlotHandler: sys.ChipIdBySlotHander(id),
			GetChipLvConfHandler:   sys.ChipLvConfHandler(id),
			AfterChipUpLvCb:        sys.AfterChipUpLvCb(id),
			AfterSuitActiveCb:      sys.AfterSuitActiveCb(id),
			AfterSuitUpLvCb:        sys.AfterSuitUpLvCb(id),
		}
	}

	for k, foo := range sys.fabaoChips {
		if err := foo.Init(); err != nil {
			sys.LogError("GodFabaoSys init culs.Init failed, id: %v", k)
			return false
		}
	}

	return true
}

func (sys *GodFabaoSys) AfterChipUpLvCb(suitId uint32) func(player iface.IPlayer, slot uint32) {
	return func(player iface.IPlayer, slot uint32) {
		chipConf := jsondata.GetGodFabaoChipConf(suitId, slot)
		if chipConf == nil {
			return
		}
		sys.SendProto3(26, 1, &pb3.S2C_26_1{
			FabaoId: suitId,
			Chip:    chipConf.ChipId,
			Lev:     sys.fabaoChips[suitId].Chips[slot].Lv,
		})
	}
}

func (sys *GodFabaoSys) AfterSuitActiveCb(suitId uint32) func() {
	return func() {
		engine.BroadcastTipMsgById(tipmsgid.TpGodFabaoSuited, sys.GetOwner().GetId(), sys.GetOwner().GetName(), suitId)
	}
}

func (sys *GodFabaoSys) AfterSuitUpLvCb(suitId uint32) func() {
	return func() {
	}
}

// 根据套装id 生成根据槽位获取芯片id的函数
func (sys *GodFabaoSys) ChipIdBySlotHander(suitId uint32) suitbase.ChipIdBySlotHandler {
	return func(slot uint32) uint32 {
		return jsondata.GetGodFabaoChipConf(suitId, slot).ChipId
	}
}

// 根据套装id 生成根据槽位获取芯片升级配置的函数
func (sys *GodFabaoSys) ChipLvConfHandler(id uint32) suitbase.ChipLvConfHandler {
	return func(slot uint32, lv uint32) *jsondata.ConsumeUpLvConf {
		return jsondata.GetGodFabaoChipLvConf(id, slot, lv)
	}
}

func (sys *GodFabaoSys) c2sUpChipLv(msg *base.Message) error {
	var req pb3.C2S_26_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	chipConf := jsondata.GetGodFabaoChipConf(req.FabaoId, req.Slot)
	if nil == chipConf {
		return neterror.ParamsInvalidError("chipConf is nil")
	}

	chipItem := jsondata.GetItemConfig(chipConf.ChipId)
	if nil == chipItem {
		return neterror.ParamsInvalidError("chipItem is nil")
	}

	if chipItem.Type != itemdef.ItemTypeGodFabaoChip {
		return neterror.ParamsInvalidError("chipItem type err")
	}

	_, ok := sys.data.Fabaos[chipItem.CommonField]
	if !ok {
		return neterror.ParamsInvalidError("fabaoData is nil")
	}

	chipUpLvER, ok := sys.fabaoChips[chipItem.CommonField]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if err := chipUpLvER.ChipUpLv(sys.GetOwner(), req.Slot, true); err != nil {
		return neterror.ParamsInvalidError("chipUpLvER.ChipUpLv err: %v", err)
	}
	return nil
}

func (sys *GodFabaoSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_26_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	if req.FabaoId == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Fabao)
		return nil
	}

	fabao := sys.data.Fabaos[req.FabaoId]
	if nil == fabao {
		return neterror.ParamsInvalidError("fabao is nil")
	}

	chipUpLvER, ok := sys.fabaoChips[req.FabaoId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	if !chipUpLvER.SuitActivated() {
		return neterror.ParamsInvalidError("fabao suit not activated")
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Fabao, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_GodFabao,
		AppearId: req.FabaoId,
	}, true)

	return nil
}

func (sys *GodFabaoSys) c2sUpSkillLevel(msg *base.Message) error {
	var req pb3.C2S_26_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg err: %v", err)
	}

	fabao := sys.data.Fabaos[req.FabaoId]
	if nil == fabao {
		return neterror.ParamsInvalidError("fabao is nil")
	}

	chipUpLvER, ok := sys.fabaoChips[req.FabaoId]
	if !ok {
		return neterror.InternalError("chipUpLvER is nil")
	}

	fabaoLvConf := jsondata.GetGodFabaoLvConf(req.FabaoId, chipUpLvER.GetSuitLv())
	if fabaoLvConf == nil {
		return neterror.ParamsInvalidError("fabaoLvConf is nil")
	}

	if fabaoLvConf.SkillId == 0 {
		return neterror.ParamsInvalidError("fabaoLvConf.SkillId != 0")
	}

	skill := sys.GetOwner().GetSkillInfo(uint32(fabaoLvConf.SkillId))
	if skill != nil && fabaoLvConf.SkillLv <= skill.Level {
		return neterror.ParamsInvalidError("skill level is max")
	}

	if !sys.GetOwner().LearnSkill(uint32(fabaoLvConf.SkillId), uint32(fabaoLvConf.SkillLv), true) {
		return neterror.ParamsInvalidError("LearnSkill err")
	}
	return nil
}

func (sys *GodFabaoSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for fabaoId, fabao := range sys.data.Fabaos {
		chipUpLvER, ok := sys.fabaoChips[fabaoId]
		if !ok {
			sys.LogError("get chipUplvER failed fabaoId %d", fabaoId)
			continue
		}

		if chipUpLvER.SuitActivated() {
			fabaoLvConf := jsondata.GetGodFabaoLvConf(fabaoId, chipUpLvER.GetSuitLv())
			if nil != fabaoLvConf {
				engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, fabaoLvConf.Attrs)
			}
		}

		for slot, chip := range fabao.Chips {
			chipLvConf := jsondata.GetGodFabaoChipLvConf(fabaoId, slot, chip.Lv)
			if nil == chipLvConf {
				continue
			}
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, chipLvConf.Attrs)
		}
	}
}

func godFabaoAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	godFabaoSys, ok := player.GetSysObj(sysdef.SiGodFabao).(*GodFabaoSys)
	if !ok || !godFabaoSys.IsOpen() {
		return
	}
	godFabaoSys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiGodFabao, func() iface.ISystem {
		return &GodFabaoSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaGodFabao, godFabaoAttrCalcFn)

	net.RegisterSysProto(26, 2, sysdef.SiGodFabao, (*GodFabaoSys).c2sUpChipLv)
	net.RegisterSysProto(26, 3, sysdef.SiGodFabao, (*GodFabaoSys).c2sDress)
	net.RegisterSysProto(26, 4, sysdef.SiGodFabao, (*GodFabaoSys).c2sUpSkillLevel)
}
