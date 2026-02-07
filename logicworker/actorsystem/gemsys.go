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
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils"
)

type GemSys struct {
	Base
}

func (sys *GemSys) OnOpen() {
	sys.init()
	sys.s2cInfo()
}

func (sys *GemSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *GemSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *GemSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *GemSys) init() bool {
	if nil == sys.GetBinaryData().AllGemData {
		sys.GetBinaryData().AllGemData = make(map[uint32]*pb3.EquipPosGemData)
	}
	return true
}

func (sys *GemSys) c2sInfo(msg *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *GemSys) s2cInfo() {
	sys.SendProto3(2, 15, &pb3.S2C_2_15{
		AllGemData: sys.GetBinaryData().AllGemData,
		MasterLev:  sys.GetBinaryData().GemMasterLv,
	})
}

func (sys *GemSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_2_17
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	pos, slot := req.EquipPos, req.Slot

	if !sys.takeOff(pos, slot) {
		return neterror.ParamsInvalidError("can`t take off")
	}

	posData := sys.getPosData(pos)
	sys.onTakeOff(posData, req.Slot)
	return nil
}

func (sys *GemSys) onTakeOff(posData *pb3.EquipPosGemData, slot uint32) {
	sys.SendProto3(2, 19, &pb3.S2C_2_19{
		Data: posData,
	})
	sys.onGemChange()
}

func (sys *GemSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_2_16
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	pos, slot, hdl := req.EquipPos, req.Slot, req.Hdl

	slotNum := jsondata.GetGemSlotNum()
	if pos < itemdef.EtPosBegin || pos > itemdef.EtPosEnd {
		return neterror.ParamsInvalidError("pos invalid")
	}

	if slot <= 0 || slot > slotNum {
		return neterror.ParamsInvalidError("slot invalid")
	}

	equipsys, ok := sys.GetOwner().GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok {
		return neterror.InternalError("equipsys invalid")
	}

	_, equip := equipsys.GetEquipByPos(pos)

	if nil == equip {
		return neterror.ParamsInvalidError("equip not exist")
	}

	gem := sys.GetOwner().GetItemByHandle(hdl)

	if nil == gem {
		return neterror.ParamsInvalidError("gem not exist")
	}

	if !sys.equipSlotOpenP(equip.ItemId, slot) {
		return neterror.ParamsInvalidError("slot not open")
	}

	if !sys.gemCanInlayInEquip(equip.ItemId, gem.ItemId, slot) {
		return neterror.ParamsInvalidError("gem can`t inlay in equip")
	}

	if !sys.takeOn(gem.ItemId, pos, slot) {
		return neterror.InternalError("take on failed")
	}

	posData := sys.getPosData(pos)
	sys.onTakeOn(posData, req.Slot)
	return nil
}

func (sys *GemSys) onTakeOn(posData *pb3.EquipPosGemData, slot uint32) {
	sys.GetOwner().SendTipMsg(tipmsgid.TakeOnGemSuccess)
	sys.SendProto3(2, 16, &pb3.S2C_2_16{
		Data: posData,
		Slot: slot,
	})

	sys.SendProto3(2, 19, &pb3.S2C_2_19{
		Data: posData,
	})
	sys.onGemChange()
}

func (sys *GemSys) getPosData(pos uint32) *pb3.EquipPosGemData {
	if pos < itemdef.EtPosBegin || pos > itemdef.EtPosEnd {
		return nil
	}

	if data, ok := sys.GetBinaryData().AllGemData[pos]; ok {
		if data.Ids == nil {
			data.Ids = make(map[uint32]uint32)
		}

		return data
	}

	data := &pb3.EquipPosGemData{
		Pos: pos,
		Ids: make(map[uint32]uint32),
	}

	sys.GetBinaryData().AllGemData[pos] = data
	return data
}

func (sys *GemSys) equipSlotOpenP(equipId uint32, slot uint32) bool {
	itemConf := jsondata.GetItemConfig(equipId)

	if nil == itemConf {
		return false
	}

	equipStage := itemConf.Stage

	vipLevel := sys.GetOwner().GetExtraAttrU32(attrdef.VipLevel)

	condConf := jsondata.GetGemSlotConf(slot)

	if condConf.Stage > 0 && equipStage < condConf.Stage {
		return false
	}

	if condConf.VipLevel > 0 && vipLevel < condConf.VipLevel {
		return false
	}

	return true
}

func (sys *GemSys) gemCanInlayInEquip(equipId uint32, gemId uint32, slot uint32) bool {
	// judge can be mounted
	gemItemConf := jsondata.GetItemConfig(gemId)
	if nil == gemItemConf {
		return false
	}

	if gemItemConf.Type != itemdef.ItemGem {
		return false
	}

	equipConf := jsondata.GetItemConfig(equipId)
	if nil == equipConf {
		return false
	}

	inlayConf := jsondata.GetGemInlayConf(equipConf.SubType)
	if nil == inlayConf {
		return false
	}

	slotConf := jsondata.GetGemSlotConf(slot)
	if nil == slotConf {
		return false
	}

	if slotConf.Special == 2 && !utils.SliceContainsUint32(inlayConf.SpecialSlotGemType, gemItemConf.SubType) {
		return false
	}

	if slotConf.Special == 1 && !utils.SliceContainsUint32(inlayConf.NormalSlotGemType, gemItemConf.SubType) {
		return false
	}
	return true
}

func (sys *GemSys) takeOff(pos, slot uint32) bool {
	posData := sys.getPosData(pos)
	if nil == posData {
		return false
	}

	slotNum := jsondata.GetGemSlotNum()
	if pos < itemdef.EtPosBegin || pos > itemdef.EtPosEnd {
		return false
	}

	if slot <= 0 || slot > slotNum {
		return false
	}

	id, ok := posData.Ids[slot]
	if !ok {
		return false
	}

	item := jsondata.StdReward{
		Id:    id,
		Count: 1,
	}

	param := common.EngineGiveRewardParam{LogId: pb3.LogId_LogGemTakeOff, NoTips: false}
	if !engine.GiveRewards(sys.GetOwner(), []*jsondata.StdReward{&item}, param) {
		return false
	}

	delete(posData.Ids, slot)
	return true
}

func (sys *GemSys) takeOn(gemId uint32, pos uint32, slot uint32) bool {
	posData := sys.getPosData(pos)
	if nil == posData {
		return false
	}

	slotNum := jsondata.GetGemSlotNum()
	if pos < itemdef.EtPosBegin || pos > itemdef.EtPosEnd {
		return false
	}

	if slot <= 0 || slot > slotNum {
		return false
	}

	if _, ok := posData.Ids[slot]; ok {
		sys.takeOff(pos, slot)
	}

	if !sys.GetOwner().DeleteItemById(gemId, 1, pb3.LogId_LogGemMount) {
		return false
	}

	posData.Ids[slot] = gemId
	return true
}

func (sys *GemSys) OnEquipTakeOff(equipPos uint32) {
	posData := sys.getPosData(equipPos)
	if nil == posData {
		return
	}

	var items []*jsondata.StdReward
	for _, id := range posData.Ids {
		items = append(items, &jsondata.StdReward{
			Id:    id,
			Count: 1,
		})
	}
	param := common.EngineGiveRewardParam{LogId: pb3.LogId_LogGemTakeOff, NoTips: false}
	if len(items) > 0 {
		if !engine.GiveRewards(sys.GetOwner(), items, param) {
			return
		}
	}

	delete(sys.GetBinaryData().AllGemData, equipPos)

	sys.SendProto3(2, 19, &pb3.S2C_2_19{Data: &pb3.EquipPosGemData{
		Pos: equipPos,
		Ids: make(map[uint32]uint32),
	}})

	sys.onGemChange()
}

func (sys *GemSys) onGemChange() {
	sys.ResetSysAttr(attrdef.SaGem)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttMountGemCount, 0, int64(len(sys.GetBinaryData().AllGemData)))
	sys.GetOwner().TriggerQuestEvent(custom_id.QttMountTotalGemLevel, 0, int64(sys.calcGemTotalLv()))
}

func (sys *GemSys) OnReplaceEquip(nEquip *pb3.ItemSt, oEquip *pb3.ItemSt, pos uint32) {

	posData := sys.getPosData(pos)
	if nil == posData {
		return
	}

	newEquipConf := jsondata.GetItemConfig(nEquip.ItemId)
	oldEquipConf := jsondata.GetItemConfig(oEquip.ItemId)
	if newEquipConf.Stage >= oldEquipConf.Stage {
		sys.SendProto3(2, 19, &pb3.S2C_2_19{Data: posData})
		sys.ResetSysAttr(sysdef.SiGem)
		return
	}

	for slot, id := range posData.Ids {
		if !sys.equipSlotOpenP(nEquip.ItemId, slot) {
			item := jsondata.StdReward{
				Id:    id,
				Count: 1,
			}

			param := common.EngineGiveRewardParam{LogId: pb3.LogId_LogGemTakeOff, NoTips: false}
			if !engine.GiveRewards(sys.GetOwner(), []*jsondata.StdReward{&item}, param) {
				return
			}

			delete(posData.Ids, slot)
			continue
		}
		if !sys.gemCanInlayInEquip(nEquip.ItemId, id, slot) {
			item := jsondata.StdReward{
				Id:    id,
				Count: 1,
			}

			param := common.EngineGiveRewardParam{LogId: pb3.LogId_LogGemMount, NoTips: false}
			if !engine.GiveRewards(sys.GetOwner(), []*jsondata.StdReward{&item}, param) {
				return
			}
			delete(posData.Ids, id)
			continue
		}
	}

	sys.SendProto3(2, 19, &pb3.S2C_2_19{Data: posData})
	sys.ResetSysAttr(sysdef.SiGem)
}

func (sys *GemSys) c2sUpMasterLv(msg *base.Message) error {
	curGemMasterLv := sys.GetBinaryData().GemMasterLv
	nextGemMasterLv := curGemMasterLv + 1
	totalGemLv := sys.calcGemTotalLv()

	maxMasterLv := jsondata.GetGemAssembleConfMaxStage()

	if maxMasterLv <= curGemMasterLv {
		return neterror.ParamsInvalidError("Max Gem level is %d", curGemMasterLv)
	}

	nextAssembleConf := jsondata.GetGemAssembleConfByStage(curGemMasterLv + 1)

	if totalGemLv < nextAssembleConf.Level {
		return neterror.ParamsInvalidError("can`t activate totalGemLv not meat next required %d", nextAssembleConf.Level)
	}

	sys.GetBinaryData().GemMasterLv = nextGemMasterLv

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogGemMasterUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(sys.GetBinaryData().GemMasterLv),
	})

	sys.SendProto3(2, 18, &pb3.S2C_2_18{Stage: nextGemMasterLv})
	sys.ResetSysAttr(attrdef.SaGem)
	return nil
}

func (sys *GemSys) GetTotalGemLevel() uint32 {
	if !sys.IsOpen() {
		return 0
	}

	return sys.calcGemTotalLv()
}

func (sys *GemSys) calcGemTotalLv() uint32 {
	totalGemLv := 0
	allGemData := sys.GetBinaryData().AllGemData
	for _, gemData := range allGemData {
		for _, itemId := range gemData.Ids {
			item := jsondata.GetItemConfig(itemId)
			if nil == item {
				continue
			}

			totalGemLv += int(item.Stage)
		}
	}

	return uint32(totalGemLv)
}

func calcGemSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	allGemData := player.GetBinaryData().AllGemData
	for _, gemdata := range allGemData {
		var attrs jsondata.AttrVec
		for _, itemId := range gemdata.Ids {
			item := jsondata.GetItemConfig(itemId)
			if nil == item {
				continue
			}
			attrs = append(attrs, item.StaticAttrs...)
		}
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}

	GemMasterLv := player.GetBinaryData().GemMasterLv

	assembleConf := jsondata.GetGemAssembleConfByStage(GemMasterLv)

	if nil == assembleConf {
		return
	}
	engine.CheckAddAttrsToCalc(player, calc, assembleConf.Attrs)
}

func calcGemSysAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	allGemData := player.GetBinaryData().AllGemData
	otherSysAddRate := totalSysCalc.GetValue(attrdef.GemAddRate)
	if otherSysAddRate == 0 {
		return
	}
	for _, gemdata := range allGemData {
		var attrs jsondata.AttrVec
		for _, itemId := range gemdata.Ids {
			item := jsondata.GetItemConfig(itemId)
			if nil == item {
				continue
			}
			for _, val := range item.StaticAttrs {
				if val.Type == attrdef.GemAddRate {
					continue
				}
				attrs = append(attrs, &jsondata.Attr{
					Type:           val.Type,
					Value:          utils.CalcMillionRate(val.Value, uint32(otherSysAddRate)),
					Job:            val.Job,
					EffectiveLimit: val.EffectiveLimit,
				})
			}
		}
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func handlePowerRushRankSubTypeEquipGem(player iface.IPlayer) (score int64) {
	return player.GetAttrSys().GetSysPower(attrdef.SaGem)
}

func GemOnEquipTakeOff(player iface.IPlayer, args ...interface{}) {
	allGemData := player.GetBinaryData().AllGemData
	if nil == allGemData {
		return
	}

	if len(args) < 2 {
		return
	}

	pos, ok := args[1].(uint32)
	if !ok {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiGem).(*GemSys)
	if !ok {
		return
	}
	sys.OnEquipTakeOff(pos)
}

func GemOnEquipTakeReplace(player iface.IPlayer, args ...interface{}) {
	allGemData := player.GetBinaryData().AllGemData
	if nil == allGemData {
		return
	}

	if len(args) < 3 {
		return
	}

	newequip, ok := args[0].(*pb3.ItemSt)
	if !ok {
		return
	}

	oldEquip, ok := args[1].(*pb3.ItemSt)
	if !ok {
		return
	}
	pos, ok := args[2].(uint32)
	if !ok {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiGem).(*GemSys)
	if !ok {
		return
	}

	sys.OnReplaceEquip(newequip, oldEquip, pos)
}

func init() {
	RegisterSysClass(sysdef.SiGem, func() iface.ISystem {
		return &GemSys{}
	})

	engine.RegQuestTargetProgress(custom_id.QttMountTotalGemLevel, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		sys, ok := actor.GetSysObj(sysdef.SiGem).(*GemSys)
		if !ok || !sys.IsOpen() {
			return 0
		}
		totalGemLv := sys.calcGemTotalLv()
		return totalGemLv
	})

	engine.RegAttrCalcFn(attrdef.SaGem, calcGemSysAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaGem, calcGemSysAttrAddRate)
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeEquipGem, handlePowerRushRankSubTypeEquipGem)
	event.RegActorEvent(custom_id.AeTakeOffEquip, GemOnEquipTakeOff)
	event.RegActorEvent(custom_id.AeTakeReplaceEquip, GemOnEquipTakeReplace)

	net.RegisterSysProto(2, 15, sysdef.SiGem, (*GemSys).c2sInfo)
	net.RegisterSysProto(2, 16, sysdef.SiGem, (*GemSys).c2sTakeOn)
	net.RegisterSysProto(2, 17, sysdef.SiGem, (*GemSys).c2sTakeOff)
	net.RegisterSysProto(2, 18, sysdef.SiGem, (*GemSys).c2sUpMasterLv)
}
