package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/net"
)

type DomainSoulEquipSys struct {
	Base
}

func (s *DomainSoulEquipSys) GetSysData() *pb3.DomainSoulData {
	binary := s.GetBinaryData()

	if nil == binary.DomainSoulData {
		binary.DomainSoulData = &pb3.DomainSoulData{}
	}

	if nil == binary.DomainSoulData.DomainSoulEqData {
		binary.DomainSoulData.DomainSoulEqData = &pb3.DomainSoulEqData{}
	}
	if nil == binary.DomainSoulData.EqStageData {
		binary.DomainSoulData.EqStageData = &pb3.DomainSoulEqStageData{}
	}

	domainSoulEq := binary.DomainSoulData.DomainSoulEqData
	if domainSoulEq.Equips == nil {
		domainSoulEq.Equips = make(map[uint32]uint32)
	}

	eqStageData := binary.DomainSoulData.EqStageData
	if eqStageData.EquipStage == nil {
		eqStageData.EquipStage = make(map[uint32]uint32)
	}

	return binary.DomainSoulData
}

func (s *DomainSoulEquipSys) OnLogin() {

}

func (s *DomainSoulEquipSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *DomainSoulEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DomainSoulEquipSys) s2cInfo() {
	s.SendProto3(144, 48, &pb3.S2C_144_48{
		DomainSoulData: s.GetSysData(),
	})
}

func (s *DomainSoulEquipSys) GetSlotByItem(itemId uint32) uint32 {
	data := s.GetSysData()

	eqData := data.DomainSoulEqData
	if eqData == nil || len(eqData.Equips) == 0 {
		return 0
	}

	for pos, v := range eqData.Equips {
		if v == itemId {
			return pos
		}
	}

	return 0
}

func (s *DomainSoulEquipSys) GetDomainSoulEqData() *pb3.DomainSoulEqData {
	return s.GetSysData().GetDomainSoulEqData()
}

func (s *DomainSoulEquipSys) EquipOnP(itemId uint32) bool {
	return s.GetSlotByItem(itemId) > 0
}

func (s *DomainSoulEquipSys) TakeOnWithItemConfAndWithoutRewardToBag(player iface.IPlayer, itemConf *jsondata.ItemConf, logId uint32) error {
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemConf is nil")
	}

	slot := itemConf.SubType

	eqData := s.GetSysData().DomainSoulEqData

	eqData.Equips[slot] = itemConf.Id

	s.SendProto3(144, 49, &pb3.S2C_144_49{
		Pos:    slot,
		ItemId: itemConf.Id,
	})
	s.checkSuit(slot)
	s.ResetSysAttr(attrdef.SaDomainSoulAttrs)

	return nil
}

func (s *DomainSoulEquipSys) GetBagSys() (*DomainSoulBagSys, error) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiDomainSoulBag).(*DomainSoulBagSys)
	if !ok {
		return nil, neterror.SysNotExistError("SmithBagSys get err")
	}

	return bagSys, nil
}

func (s *DomainSoulEquipSys) getDomainSoulEqBySlot(slot uint32) uint32 {
	data := s.GetDomainSoulEqData()
	v, ok := data.Equips[slot]
	if ok {
		return v
	}
	return 0
}

func (s *DomainSoulEquipSys) takeOn(slot uint32, hdl uint64) error {
	bagSys, err := s.GetBagSys()
	if nil != err {
		s.LogError("err:%v", err)
		return err
	}

	newEquip := bagSys.FindItemByHandle(hdl)

	newEquipId := newEquip.ItemId
	if nil == newEquip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	err = s.checkTakeOnSlotHandle(newEquip, slot) //检查装备是否符合穿戴条件
	if err != nil {
		return neterror.Wrap(err)
	}

	eqData := s.GetDomainSoulEqData()
	if eqData == nil {
		return neterror.ParamsInvalidError("DomainSoulEquipData is nil")
	}

	oldEq := s.getDomainSoulEqBySlot(slot)
	if oldEq != 0 {
		if err := s.takeOff(slot); err != nil {
			return err
		}
	}

	if removeSucc := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogDomainSoulEquipTakeOn); !removeSucc {
		return neterror.InternalError("remove SmithEquip hdl:%d item:%d failed", newEquip.GetHandle(), newEquip.GetItemId())
	}
	eqData.Equips[slot] = newEquipId
	s.SendProto3(144, 49, &pb3.S2C_144_49{
		Pos:    slot,
		ItemId: newEquipId,
	})
	return nil
}
func (s *DomainSoulEquipSys) checkTakeOnSlotHandle(equip *pb3.ItemSt, slot uint32) error {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return neterror.ConfNotFoundError("item itemConf(%d) nil", equip.GetItemId())
	}

	if !itemdef.IsDomainSoulItem(itemConf.Type) {
		return neterror.SysNotExistError("not DomainSoulEquip item")
	}

	if itemConf.SubType != slot {
		return neterror.ParamsInvalidError("smith equip take pos is not equal")
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return neterror.ParamsInvalidError("the wearing conditions are not met")
	}

	if jsondata.GetDomainSoulConfBySlot(slot) == nil {
		return neterror.ParamsInvalidError("DomainSoul(%d) conf is nil", slot)
	}

	return nil
}

func (s *DomainSoulEquipSys) c2sDSTakeOn(msg *base.Message) error {
	var req pb3.C2S_144_49
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	err = s.takeOn(req.GetPos(), req.GetHandle())
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.checkSuit(req.GetPos())
	s.ResetSysAttr(attrdef.SaDomainSoulAttrs)
	return nil
}

func (s *DomainSoulEquipSys) checkSuit(pos uint32) {
	// 获取域灵装备数据（包含已穿戴装备和阶段数据）
	domainData := s.GetSysData()
	eqData := domainData.DomainSoulEqData

	wornEquips := eqData.Equips // 已穿戴装备：map[槽位]装备ID
	if len(wornEquips) == 0 {
		return
	}

	stageData := domainData.EqStageData
	// 根据槽位获取唯一所属套装配置
	suitConf := jsondata.GetDomainSoulSuitBySlot(pos)
	if suitConf == nil {
		s.LogError("checkSuit: no suit config found for slot %d", pos)
		return
	}

	suitId := suitConf.Id
	oldStage := stageData.EquipStage[suitId]

	var minStage uint32 = 0
	for i, slot := range suitConf.SubList {
		itemId, exists := wornEquips[slot]
		if !exists {
			minStage = 0
			break
		}
		stage := jsondata.GetItemStage(itemId)
		// 计算当前套装中装备的最低阶段
		if i == 0 || stage < minStage {
			minStage = stage
		}
	}

	if oldStage == minStage {
		return
	}

	if oldSuitConf := jsondata.GetDomainSoulSuitStageConf(suitId, oldStage); nil != oldSuitConf {
		s.GetOwner().ForgetSkill(oldSuitConf.SkillId, true, true, true)
	}

	stageData.EquipStage[suitId] = minStage

	if newSuitConf := jsondata.GetDomainSoulSuitStageConf(suitId, minStage); nil != newSuitConf {
		s.GetOwner().LearnSkill(newSuitConf.SkillId, newSuitConf.SkillLevel, true)
	}
	s.s2cInfo()
}

func (s *DomainSoulEquipSys) c2sDSTakeOff(msg *base.Message) error {
	var req pb3.C2S_144_50
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	err = s.takeOff(req.GetPos())
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.checkSuit(req.GetPos())
	s.ResetSysAttr(attrdef.SaDomainSoulAttrs)
	return nil
}

func (s *DomainSoulEquipSys) takeOff(pos uint32) error {
	eqData := s.GetDomainSoulEqData()
	oldItemId := eqData.Equips[pos]
	if oldItemId == 0 {
		return neterror.ParamsInvalidError("DomainSoulEquip is nil")
	}

	bagSys, err := s.GetBagSys()
	if nil != err {
		return neterror.Wrap(err)
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return neterror.ParamsInvalidError("bag availablecount <=0 ")
	}
	delete(eqData.Equips, pos)
	engine.GiveRewards(s.GetOwner(), []*jsondata.StdReward{{Id: oldItemId, Count: 1}}, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDomainSoulEquipTakeOff,
	})
	s.SendProto3(144, 50, &pb3.S2C_144_50{Pos: pos})
	return nil
}

func (s *DomainSoulEquipSys) calcDomainSoulEquAttr(calc *attrcalc.FightAttrCalc) {
	domainData := s.GetSysData()
	for _, itemId := range domainData.DomainSoulEqData.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, itemConf.StaticAttrs)
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, itemConf.PremiumAttrs)
	}

	for suitId, stage := range domainData.EqStageData.EquipStage {
		if stage == 0 { // 只有激活的套装才计算属性
			continue
		}
		suitStageConf := jsondata.GetDomainSoulSuitStageConf(suitId, stage)
		if nil == suitStageConf {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, suitStageConf.Attrs)
	}
}

func (s *DomainSoulEquipSys) calcDomainSoulEquAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	hpRate := totalSysCalc.GetValue(attrdef.DomainSoulHpRate)
	attackRate := totalSysCalc.GetValue(attrdef.DomainSoulAttackRate)
	defRate := totalSysCalc.GetValue(attrdef.DomainSoulDefRate)
	armorBreakRate := totalSysCalc.GetValue(attrdef.DomainSoulArmorBreakRate)

	eqData := s.GetSysData().DomainSoulEqData
	for _, itemId := range eqData.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}
		for _, attr := range itemConf.StaticAttrs {
			var addVal int64
			switch attr.Type {
			case attrdef.Hp:
				addVal = utils.CalcMillionRate64(int64(attr.Value), hpRate)
			case attrdef.Attack:
				addVal = utils.CalcMillionRate64(int64(attr.Value), attackRate)
			case attrdef.Defend:
				addVal = utils.CalcMillionRate64(int64(attr.Value), defRate)
			case attrdef.ArmorBreak:
				addVal = utils.CalcMillionRate64(int64(attr.Value), armorBreakRate)
			}
			if addVal > 0 {
				engine.CheckAddAttrsToCalc(s.owner, calc, []*jsondata.Attr{
					{Type: attr.Type, Value: uint32(addVal), Job: attr.Job},
				})
			}
		}
	}
}

func calcDomainSoulEquAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDomainSoulEquip).(*DomainSoulEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDomainSoulEquAttr(calc)
}

func calcDomainSoulEquAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDomainSoulEquip).(*DomainSoulEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDomainSoulEquAttrAddRate(totalSysCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiDomainSoulEquip, func() iface.ISystem {
		return &DomainSoulEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaDomainSoulAttrs, calcDomainSoulEquAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaDomainSoulAttrs, calcDomainSoulEquAttrAddRate)

	net.RegisterSysProtoV2(144, 49, sysdef.SiDomainSoulEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSoulEquipSys).c2sDSTakeOn
	})
	net.RegisterSysProtoV2(144, 50, sysdef.SiDomainSoulEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSoulEquipSys).c2sDSTakeOff
	})
}
