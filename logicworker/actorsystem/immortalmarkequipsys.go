/**
 * @Author: LvYuMeng
 * @Date: 2024/11/10
 * @Desc: 仙印装备
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type ImmortalMarkEquipSys struct {
	Base
}

func (s *ImmortalMarkEquipSys) OnLogin() {
}

func (s *ImmortalMarkEquipSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *ImmortalMarkEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ImmortalMarkEquipSys) s2cInfo() {
	s.SendProto3(11, 140, &pb3.S2C_11_140{Data: s.getData()})
}

func (s *ImmortalMarkEquipSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_11_141
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	success, err := s.takeOn(req.GetEquipPos(), req.GetMarkPos(), req.GetHdl())
	if !success {
		return err
	}

	return nil
}

func (s *ImmortalMarkEquipSys) takeOff(equipPos, markPos uint32, clearSuit bool) (bool, error) {
	oldEquip := s.getMarkBySlot(equipPos, markPos)
	if nil == oldEquip {
		return false, neterror.ParamsInvalidError("no equip")
	}

	if s.owner.GetBagAvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return false, nil
	}

	data := s.getData()
	delete(data.SlotInfo[equipPos].MarkInfo, markPos)

	if !engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    oldEquip.ItemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogImmortalMarkEquipTakeOff}) {
		return false, neterror.ParamsInvalidError("item lose")
	}

	s.updateInSlot(equipPos, markPos, ImmortalMarkEquipUpdateSlotTakeOff)

	if clearSuit {
		s.clearSuit(equipPos)
	}

	s.afterTakeOff(equipPos, markPos, oldEquip)

	return true, nil
}

func (s *ImmortalMarkEquipSys) takeOn(equipPos, markPos uint32, hdl uint64) (bool, error) {
	markEquip := s.owner.GetItemByHandle(hdl)
	if nil == markEquip || markEquip.GetCount() <= 0 {
		return false, neterror.ParamsInvalidError("markEquip not found")
	}
	success, err := s.checkCanTakOn(equipPos, markPos, markEquip)
	if !success {
		return false, err
	}

	oldSuitQuality := s.getSuitQuality(equipPos)

	oldEquip := s.getMarkBySlot(equipPos, markPos)
	if nil != oldEquip {
		if success, err = s.takeOff(equipPos, markPos, false); !success {
			return false, err
		}
	}

	if removeSucc := s.owner.DeleteItemPtr(markEquip, 1, pb3.LogId_LogImmortalMarkEquipTakeOn); !removeSucc {
		return false, neterror.InternalError("remove soul halo hdl:%d item:%d failed", markEquip.GetHandle(), markEquip.GetItemId())
	}

	s.takeToSlot(equipPos, markPos, &pb3.ItemSt{
		ItemId: markEquip.GetItemId(),
		Count:  1,
	})
	s.updateInSlot(equipPos, markPos, ImmortalMarkEquipUpdateSlotTakeOn)

	newSuitQuality := s.getSuitQuality(equipPos)

	if newSuitQuality != oldSuitQuality || newSuitQuality == 0 {
		s.clearSuit(equipPos)
	}

	s.afterTakeOn(equipPos, markPos)

	return true, nil
}

func (s *ImmortalMarkEquipSys) afterTakeOn(equipPos, markPos uint32) {
	s.ResetSysAttr(attrdef.SaImmortalMarkEquip)
}

func (s *ImmortalMarkEquipSys) afterTakeOff(equipPos, markPos uint32, oldEquip *pb3.ItemSt) {
	s.ResetSysAttr(attrdef.SaImmortalMarkEquip)
}

func (s *ImmortalMarkEquipSys) clearSuit(equipPos uint32) {
	data := s.getData()
	delete(data.EquipSuitActive, equipPos)
	s.SendProto3(11, 143, &pb3.S2C_11_143{
		EquipPos: equipPos,
		Stage:    data.EquipSuitActive[equipPos],
	})
}

const (
	ImmortalMarkEquipUpdateSlotTakeOn  = 1
	ImmortalMarkEquipUpdateSlotTakeOff = 2
	ImmortalMarkEquipUpdateSlotCompose = 3
)

func (s *ImmortalMarkEquipSys) updateInSlot(equipPos, markPos, event uint32) {
	s.SendProto3(11, 141, &pb3.S2C_11_141{
		EquipPos:   equipPos,
		MarkPos:    markPos,
		Equip:      s.getMarkBySlot(equipPos, markPos),
		UpdateType: event,
	})
}

func (s *ImmortalMarkEquipSys) getMarkBySlot(equipPos, markPos uint32) *pb3.ItemSt {
	data := s.getData()
	if _, ok := data.SlotInfo[equipPos]; !ok {
		return nil
	}
	if nil == data.SlotInfo[equipPos].MarkInfo {
		return nil
	}
	if _, ok := data.SlotInfo[equipPos].MarkInfo[markPos]; !ok {
		return nil
	}

	return data.SlotInfo[equipPos].MarkInfo[markPos]
}

func (s *ImmortalMarkEquipSys) takeToSlot(equipPos, markPos uint32, equip *pb3.ItemSt) {
	data := s.getData()
	if _, ok := data.SlotInfo[equipPos]; !ok {
		data.SlotInfo[equipPos] = &pb3.ImmortalMarkEquipSlot{}
	}
	if nil == data.SlotInfo[equipPos].MarkInfo {
		data.SlotInfo[equipPos].MarkInfo = make(map[uint32]*pb3.ItemSt)
	}

	data.SlotInfo[equipPos].MarkInfo[markPos] = equip
}

func (s *ImmortalMarkEquipSys) getData() *pb3.ImmortalMarkEquipData {
	binary := s.GetBinaryData()
	if nil == binary.ImmortalMarkEquip {
		binary.ImmortalMarkEquip = &pb3.ImmortalMarkEquipData{}
	}
	if nil == binary.ImmortalMarkEquip.SlotInfo {
		binary.ImmortalMarkEquip.SlotInfo = make(map[uint32]*pb3.ImmortalMarkEquipSlot)
	}
	if nil == binary.ImmortalMarkEquip.EquipSuitActive {
		binary.ImmortalMarkEquip.EquipSuitActive = make(map[uint32]uint32)
	}
	return binary.ImmortalMarkEquip
}

func (s *ImmortalMarkEquipSys) checkCanTakOn(equipPos, markPos uint32, markEquip *pb3.ItemSt) (bool, error) {
	conf := jsondata.GetImmortalMarkEquipConf()
	if nil == conf {
		return false, neterror.ConfNotFoundError("conf is nil")
	}
	equipSlotConf, ok := conf.EquipSlot[equipPos]
	if !ok {
		return false, neterror.ConfNotFoundError("equipSlotConf %d is nil", equipPos)
	}
	markSlotConf, ok := conf.MarkSlot[markPos]
	if !ok {
		return false, neterror.ConfNotFoundError("markSlotConf %d is nil", markPos)
	}

	equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok {
		return false, neterror.InternalError("equipsys invalid")
	}

	_, equip := equipSys.GetEquipByPos(equipPos)

	if nil == equip {
		return false, neterror.ParamsInvalidError("equip not exist")
	}

	equipItemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == equipItemConf {
		return false, neterror.ConfNotFoundError("equip conf is nil")
	}

	markItemConf := jsondata.GetItemConfig(markEquip.GetItemId())
	if nil == markItemConf {
		return false, neterror.ConfNotFoundError("markItemConf conf is nil")
	}

	//非仙印道具
	if !itemdef.IsImmortalMarkEquip(markItemConf.Type) {
		return false, neterror.ParamsInvalidError("item type err")
	}
	//非孔位可穿仙印类型
	if !pie.Uint32s(equipSlotConf.SlotMarkType).Contains(markItemConf.SubType) {
		return false, neterror.ParamsInvalidError("item sub type err")
	}
	//孔位仙印需要装备阶级品质判断
	if equipItemConf.Stage < markSlotConf.Stage {
		return false, neterror.ParamsInvalidError("stage is not enough")
	}
	if equipItemConf.Quality < markSlotConf.Quality {
		return false, neterror.ParamsInvalidError("quality is not enough")
	}
	return true, nil
}

func (s *ImmortalMarkEquipSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_11_142
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	success, err := s.takeOff(req.GetEquipPos(), req.GetMarkPos(), true)
	if !success {
		return err
	}

	return nil
}

func (s *ImmortalMarkEquipSys) c2sSuitActive(msg *base.Message) error {
	var req pb3.C2S_11_143
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	equipPos := req.GetEquipPos()
	equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok {
		return neterror.InternalError("equipsys invalid")
	}

	_, equip := equipSys.GetEquipByPos(equipPos)

	if nil == equip {
		return neterror.ParamsInvalidError("equip not exist")
	}

	equipItemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == equipItemConf {
		return neterror.ConfNotFoundError("item conf is nil")
	}

	stage := equipItemConf.Stage
	stageSuitConf, ok := jsondata.GetImmortalMarkSuitConf(equipPos, stage)
	if !ok {
		return neterror.ConfNotFoundError("suit conf is nil")
	}

	data := s.getData()
	if _, ok := data.SlotInfo[equipPos]; !ok {
		return neterror.ParamsInvalidError("equip mark is nil")
	}

	count := uint32(len(data.SlotInfo[equipPos].MarkInfo))
	if count < stageSuitConf.ActiveNum {
		return neterror.ParamsInvalidError("active num is not enough")
	}

	if data.EquipSuitActive[equipPos] == stage {
		return neterror.ParamsInvalidError("already active")
	}

	data.EquipSuitActive[equipPos] = stage

	s.SendProto3(11, 143, &pb3.S2C_11_143{
		EquipPos: equipPos,
		Stage:    stage,
	})

	s.ResetSysAttr(attrdef.SaImmortalMarkEquip)

	return nil
}

func (s *ImmortalMarkEquipSys) c2sCompose(msg *base.Message) error {
	var req pb3.C2S_11_144
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	equipPos, markPos := req.GetEquipPos(), req.GetMarkPos()
	conf := jsondata.GetImmortalMarkEquipConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	equip := s.getMarkBySlot(equipPos, markPos)
	if nil == equip {
		return neterror.ParamsInvalidError("equip not eist")
	}

	composeConf, ok := conf.Compose[equip.GetItemId()]
	if !ok {
		return neterror.ConfNotFoundError("compose conf is nil")
	}
	needScore := composeConf.Score
	myScore := s.getScoreByItemId(equip.GetItemId())

	var swallowItemIds []uint32
	swallowItemIds = append(swallowItemIds, equip.GetItemId())
	swallowItemIds = append(swallowItemIds, pie.Uint32s(composeConf.SwallowItemIds).Filter(func(u uint32) bool {
		return u != equip.GetItemId()
	})...)

	var consume jsondata.ConsumeVec
	for _, v := range swallowItemIds {
		if needScore <= myScore {
			break
		}
		addScore := s.getScoreByItemId(v)
		needCount := int64(math.Ceil(float64(needScore-myScore) / float64(addScore)))
		hasCount := s.owner.GetItemCount(v, -1)
		cosCount := needCount
		if hasCount < needCount {
			cosCount = hasCount
		}
		consume = append(consume, &jsondata.Consume{
			Type:  custom_id.ConsumeTypeItem,
			Id:    v,
			Count: uint32(cosCount),
		})
		myScore += addScore * uint32(cosCount)
	}

	if myScore < needScore {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	consume = jsondata.MergeConsumeVec(consume, composeConf.Consume)

	if !s.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogImmortalMarkEquipCompose}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	equip.ItemId = composeConf.NewItemId

	s.updateInSlot(equipPos, markPos, ImmortalMarkEquipUpdateSlotCompose)

	overFlowScore := myScore - needScore

	var backCount uint32
	unitScore := s.getScoreByItemId(composeConf.BackItemId)
	if unitScore > 0 {
		backCount = overFlowScore / unitScore
	}

	if backCount > 0 {
		engine.GiveRewards(s.owner, jsondata.StdRewardVec{
			&jsondata.StdReward{
				Id:    composeConf.BackItemId,
				Count: int64(backCount),
			},
		}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogImmortalMarkEquipCompose})
	}

	s.clearSuit(equipPos)

	s.ResetSysAttr(attrdef.SaImmortalMarkEquip)

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogImmortalMarkEquipCompose, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"newItem":    composeConf.NewItemId,
			"beforeItem": composeConf.TakeItemId,
			"equipPos":   equipPos,
			"markPos":    markPos,
		}),
	})

	return nil
}

func (s *ImmortalMarkEquipSys) getSuitQuality(pos uint32) uint32 {
	data := s.getData()

	if data.EquipSuitActive[pos] == 0 {
		return 0
	}
	slotInfo, ok := data.SlotInfo[pos]
	if !ok {
		return 0
	}

	var minQuality uint32
	for _, v := range slotInfo.MarkInfo {
		quality := jsondata.GetItemQuality(v.GetItemId())
		if minQuality == 0 || minQuality > quality {
			minQuality = quality
		}
	}

	return minQuality
}

func (s *ImmortalMarkEquipSys) getScoreByItemId(itemId uint32) uint32 {
	if itemConf := jsondata.GetItemConfig(itemId); nil != itemConf {
		return itemConf.CommonField
	}
	return 0
}

func (s *ImmortalMarkEquipSys) calcAttrs(calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetImmortalMarkEquipConf()
	if nil == conf {
		return
	}

	data := s.getData()

	var minQuality = make(map[uint32]uint32, len(data.SlotInfo))
	for equipPos, equipSlot := range data.SlotInfo {
		for _, markEquip := range equipSlot.MarkInfo {
			itemConf := jsondata.GetItemConfig(markEquip.GetItemId())
			if nil == itemConf {
				continue
			}
			if minQuality[equipPos] == 0 || minQuality[equipPos] > itemConf.Quality {
				minQuality[equipPos] = itemConf.Quality
			}
			engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)
		}
	}

	for equipPos, suitStage := range data.EquipSuitActive {
		suitConf, ok := jsondata.GetImmortalMarkSuitConf(equipPos, suitStage)
		if !ok {
			continue
		}
		suitAttrs, ok := suitConf.SuitAttrs[minQuality[equipPos]]
		if !ok {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, suitAttrs.Attrs)
	}
}

func (s *ImmortalMarkEquipSys) calcAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for _, equipSlot := range data.SlotInfo {
		for _, markEquip := range equipSlot.MarkInfo {
			itemConf := jsondata.GetItemConfig(markEquip.GetItemId())
			if itemConf == nil {
				continue
			}
			addRate := totalSysCalc.GetValue(attrdef.ImmortalMarkBaseAttrAdd)
			if addRate == 0 {
				continue
			}
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, itemConf.StaticAttrs, uint32(addRate))
		}
	}
}

func (s *ImmortalMarkEquipSys) onEquipChange(equipPos uint32, skip bool) {
	var backItems jsondata.StdRewardVec
	data := s.getData()
	equipSlot, ok := data.SlotInfo[equipPos]
	if !ok {
		return
	}
	var delIds []uint32
	if !skip {
		for markPos, markEquip := range equipSlot.MarkInfo {
			success, _ := s.checkCanTakOn(equipPos, markPos, markEquip)
			if success {
				continue
			}
			delIds = append(delIds, markPos)
		}
	}

	isOff := false
	for markPos, markEquip := range equipSlot.MarkInfo {
		if skip || pie.Uint32s(delIds).Contains(markPos) {
			backItems = append(backItems, &jsondata.StdReward{
				Id:    markEquip.GetItemId(),
				Count: 1,
			})
			delete(equipSlot.MarkInfo, markPos)
			isOff = true
		}
	}

	if len(backItems) > 0 {
		engine.GiveRewards(s.owner, backItems, common.EngineGiveRewardParam{LogId: pb3.LogId_LogImmortalMarkEquipTakeOff})
	}

	if isOff {
		s.clearSuit(equipPos)
	}

	s.s2cInfo()
	s.ResetSysAttr(attrdef.SaImmortalMarkEquip)
}

func ImmortalMarkEquipOnEquipTakeOff(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}

	equipPos, ok := args[1].(uint32)
	if !ok {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiImmortalMarkEquip).(*ImmortalMarkEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.onEquipChange(equipPos, true)
}

func ImmortalMarkEquipOnEquipReplace(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}

	equipPos, ok := args[2].(uint32)
	if !ok {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiImmortalMarkEquip).(*ImmortalMarkEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.onEquipChange(equipPos, false)
}

func calcImmortalMarkEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalMarkEquip).(*ImmortalMarkEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttrs(calc)
}

func calcImmortalMarkEquipAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalMarkEquip).(*ImmortalMarkEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttrAddRate(totalSysCalc, calc)
}

func init() {
	RegisterSysClass(sysdef.SiImmortalMarkEquip, func() iface.ISystem {
		return &ImmortalMarkEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaImmortalMarkEquip, calcImmortalMarkEquipAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaImmortalMarkEquip, calcImmortalMarkEquipAttrAddRate)

	event.RegActorEvent(custom_id.AeTakeOffEquip, ImmortalMarkEquipOnEquipTakeOff)
	event.RegActorEvent(custom_id.AeTakeReplaceEquip, ImmortalMarkEquipOnEquipReplace)

	net.RegisterSysProtoV2(11, 141, sysdef.SiImmortalMarkEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalMarkEquipSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(11, 142, sysdef.SiImmortalMarkEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalMarkEquipSys).c2sTakeOff
	})
	net.RegisterSysProtoV2(11, 143, sysdef.SiImmortalMarkEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalMarkEquipSys).c2sSuitActive
	})
	net.RegisterSysProtoV2(11, 144, sysdef.SiImmortalMarkEquip, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalMarkEquipSys).c2sCompose
	})
}
