/**
 * @Author: LvYuMeng
 * @Date: 2024/10/30
 * @Desc: 武魂神饰
**/

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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type BattleSoulGodEquipSys struct {
	Base
}

func (s *BattleSoulGodEquipSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *BattleSoulGodEquipSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BattleSoulGodEquipSys) s2cInfo() {
	s.SendProto3(11, 110, &pb3.S2C_11_110{
		Data: s.getData(),
	})
}

func (s *BattleSoulGodEquipSys) getData() *pb3.BattleSoulGodEquipData {
	binary := s.GetBinaryData()
	if nil == binary.BattleSoulGodEquipData {
		binary.BattleSoulGodEquipData = &pb3.BattleSoulGodEquipData{}
	}
	if nil == binary.BattleSoulGodEquipData.Info {
		binary.BattleSoulGodEquipData.Info = map[uint32]*pb3.BattleSoulGodEquipSt{}
	}
	return binary.BattleSoulGodEquipData
}

const (
	battleSoulGodEquipUpdateTypeStrongUp = 1
	battleSoulGodEquipUpdateTypeStageUp  = 2
	battleSoulGodEquipUpdateTypeInherit  = 3
	battleSoulGodEquipUpdateTypeBack     = 4
	battleSoulGodEquipUpdateTypeTakeOn   = 5
	battleSoulGodEquipUpdateTypeTakeOff  = 6
	battleSoulGodEquipUpdateTypeCompose  = 7
)

func (s *BattleSoulGodEquipSys) getGodEquipByPos(battleSoulId, pos uint32) *pb3.ItemSt {
	data := s.getData()
	origin, ok := data.Info[battleSoulId]
	if !ok {
		return nil
	}
	if nil == origin.Equip {
		return nil
	}
	equip, ok := origin.Equip[pos]
	if !ok {
		return nil
	}
	return equip
}

func (s *BattleSoulGodEquipSys) isValidBattleSoul(battleSoulId uint32) bool {
	bsSys, ok := s.owner.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok {
		return false
	}

	if !bsSys.isBattleSpiritAct(battleSoulId) {
		return false
	}

	return true
}

func (s *BattleSoulGodEquipSys) updateInfo(battleSoulId, pos, updateType uint32) {
	s.SendProto3(11, 111, &pb3.S2C_11_111{
		BattleSoulId: battleSoulId,
		Pos:          pos,
		Equip:        s.getGodEquipByPos(battleSoulId, pos),
		Type:         updateType,
	})
}

func (s *BattleSoulGodEquipSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_11_116
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	return s.takeOn(req.GetBattleSoulId(), req.GetPos(), req.GetHandle())
}

func (s *BattleSoulGodEquipSys) c2sStrongUp(msg *base.Message) error {
	var req pb3.C2S_11_112
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	battleSoulId, pos := req.GetBattleSoulId(), req.GetPos()

	equip := s.getGodEquipByPos(battleSoulId, pos)
	if nil == equip {
		return neterror.ParamsInvalidError("equip not take off")
	}

	if jsondata.GetItemQuality(equip.GetItemId()) < jsondata.GetBattleSoulGodEquipConf().StrongNeedQuality {
		return neterror.ParamsInvalidError("strong quality not meet")
	}

	nextLv := equip.Union1 + 1
	nextLvConf := jsondata.GetBattleSoulGodEquipStrongConf(pos, nextLv)
	if nil == nextLvConf {
		return neterror.ConfNotFoundError("strong lv conf %d is nil", nextLv)
	}

	if equip.Union2 < nextLvConf.StageLimit {
		return neterror.ParamsInvalidError("stage quality not meet")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBattleSoulGodEquipStrongUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	equip.Union1 = nextLv
	s.updateInfo(battleSoulId, pos, battleSoulGodEquipUpdateTypeStrongUp)
	s.ResetSysAttr(attrdef.SaBattleSoulGodEquip)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogBattleSoulGodEquipStrongUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"level":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *BattleSoulGodEquipSys) c2sStageUp(msg *base.Message) error {
	var req pb3.C2S_11_113
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	battleSoulId, pos := req.GetBattleSoulId(), req.GetPos()

	equip := s.getGodEquipByPos(battleSoulId, pos)
	if nil == equip {
		return neterror.ParamsInvalidError("equip not take off")
	}

	if jsondata.GetItemQuality(equip.GetItemId()) < jsondata.GetBattleSoulGodEquipConf().StageNeedQuality {
		return neterror.ParamsInvalidError("stage quality not meet")
	}

	nextStage := equip.Union2 + 1
	nextStageConf := jsondata.GetBattleSoulGodEquipStageConf(pos, nextStage)
	if nil == nextStageConf {
		return neterror.ConfNotFoundError("stage lv conf %d is nil", nextStage)
	}

	if equip.Union1 < nextStageConf.StrongLimit {
		return neterror.ParamsInvalidError("strong quality not meet")
	}

	if !s.owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBattleSoulGodEquipStageUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	equip.Union2 = nextStage
	s.updateInfo(battleSoulId, pos, battleSoulGodEquipUpdateTypeStageUp)
	s.ResetSysAttr(attrdef.SaBattleSoulGodEquip)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogBattleSoulGodEquipStageUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"stage":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *BattleSoulGodEquipSys) FindAnyOneGodEquipByItemIdInSlot(findItemId uint32) (equip *pb3.ItemSt, battleSoulId, pos uint32) {
	data := s.getData()
	for itemId, v := range data.Info {
		for slotId, equipSt := range v.Equip {
			if equipSt.GetItemId() != findItemId {
				continue
			}
			return equipSt, itemId, slotId
		}
	}

	return
}

func (s *BattleSoulGodEquipSys) GetGodEquipFromBagOrTake(hdl uint64) (equip *pb3.ItemSt, battleSoulId, pos uint32) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
	if !ok {
		return
	}

	equip = bagSys.FindItemByHandle(hdl)
	if nil != equip {
		return equip, 0, 0
	}

	data := s.getData()
	for itemId, v := range data.Info {
		for slotId, equipSt := range v.Equip {
			if equipSt.GetHandle() != hdl {
				continue
			}
			return equipSt, itemId, slotId
		}
	}

	return
}

func (s *BattleSoulGodEquipSys) inherit(sendHdl, revHdl uint64) error {
	sendEquip, sendBattleSoulId, sendPos := s.GetGodEquipFromBagOrTake(sendHdl)
	if nil == sendEquip {
		return neterror.ParamsInvalidError("not found equip")
	}

	revEquip, revBattleSoulId, revPos := s.GetGodEquipFromBagOrTake(revHdl)
	if nil == revEquip {
		return neterror.ParamsInvalidError("not found equip")
	}

	if !s.isZeroStatus(revEquip) {
		return neterror.ParamsInvalidError("not zero status")
	}

	if jsondata.GetItemQuality(sendEquip.GetItemId()) < jsondata.GetBattleSoulGodEquipConf().SendNeedQuality {
		return neterror.ParamsInvalidError("sendEquip quality not meet")
	}

	if jsondata.GetItemQuality(revEquip.GetItemId()) < jsondata.GetBattleSoulGodEquipConf().RevNeedQuality {
		return neterror.ParamsInvalidError("revEquip quality not meet")
	}

	sendStrongLv, sendStage := sendEquip.GetUnion1(), sendEquip.GetUnion2()

	s.resetGodEquip(sendEquip)
	revEquip.Union1 = sendStrongLv
	revEquip.Union2 = sendStage

	s.onGodEquipDevelopChange(sendEquip, sendBattleSoulId, sendPos, battleSoulGodEquipUpdateTypeInherit)
	s.onGodEquipDevelopChange(revEquip, revBattleSoulId, revPos, battleSoulGodEquipUpdateTypeInherit)

	return nil
}

func (s *BattleSoulGodEquipSys) c2sInherit(msg *base.Message) error {
	var req pb3.C2S_11_114
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	sendHdl := req.GetSendHandle()
	revHdl := req.GetRevHandle()

	err = s.inherit(sendHdl, revHdl)
	if err != nil {
		return neterror.Wrap(err)
	}

	return nil
}

func (s *BattleSoulGodEquipSys) isZeroStatus(equip *pb3.ItemSt) bool {
	if equip.GetUnion1() > 0 {
		return false
	}
	if equip.GetUnion2() > 1 {
		return false
	}
	return true
}

func (s *BattleSoulGodEquipSys) packBackMaterial(equip *pb3.ItemSt) (jsondata.StdRewardVec, error) {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return nil, neterror.ConfNotFoundError("item conf %d is nil", equip.GetItemId())
	}

	var backRewards []jsondata.StdRewardVec

	strongConf := jsondata.GetBattleSoulGodEquipStrongConf(itemConf.SubType, equip.GetUnion1())
	if nil == strongConf {
		return nil, neterror.ConfNotFoundError("strongConf conf %d is nil", equip.GetItemId())
	}
	backRewards = append(backRewards, strongConf.Rewards)

	stageConf := jsondata.GetBattleSoulGodEquipStageConf(itemConf.SubType, equip.GetUnion2())
	if nil == stageConf {
		return nil, neterror.ConfNotFoundError("stageConf conf %d is nil", equip.GetItemId())
	}
	backRewards = append(backRewards, stageConf.Rewards)

	rewards := jsondata.MergeStdReward(backRewards...)
	return rewards, nil
}

func (s *BattleSoulGodEquipSys) onGodEquipDevelopChange(equip *pb3.ItemSt, battleSoulId, pos, updateType uint32) {
	if battleSoulId > 0 {
		s.updateInfo(battleSoulId, pos, updateType)
		s.ResetSysAttr(attrdef.SaBattleSoulGodEquip)
	} else {
		bagSys := s.owner.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
		bagSys.OnItemChange(equip, 0, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogBattleSoulGodEquipBack,
		})
	}
}

func (s *BattleSoulGodEquipSys) c2sBack(msg *base.Message) error {
	var req pb3.C2S_11_115
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}
	hdl := req.GetHandle()

	equip, battleSoulId, pos := s.GetGodEquipFromBagOrTake(hdl)
	if nil == equip {
		return neterror.ParamsInvalidError("not found equip")
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return neterror.ConfNotFoundError("item conf %d is nil", equip.GetItemId())
	}

	if s.isZeroStatus(equip) {
		return neterror.ParamsInvalidError("cant back isZeroStatus")
	}

	rewards, err := s.packBackMaterial(equip)
	if nil != err {
		return neterror.Wrap(err)
	}

	s.resetGodEquip(equip)

	s.onGodEquipDevelopChange(equip, battleSoulId, pos, battleSoulGodEquipUpdateTypeBack)

	engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogBattleSoulGodEquipBack,
	})

	return nil
}

func (s *BattleSoulGodEquipSys) resetGodEquip(equip *pb3.ItemSt) {
	equip.Union1 = 0
	equip.Union2 = 1
}

func (s *BattleSoulGodEquipSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_11_117
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	return s.takeOff(req.GetBattleSoulId(), req.GetPos())
}

func (s *BattleSoulGodEquipSys) checkTakeOnSlotHandle(equip *pb3.ItemSt, battleSoulId, pos uint32) (bool, error) {
	if !s.isValidBattleSoul(battleSoulId) {
		return false, neterror.ParamsInvalidError("not valid battleSoulId")
	}

	equipItemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == equipItemConf {
		return false, neterror.ConfNotFoundError("item conf %d not find", equip.GetItemId())
	}

	conf := jsondata.GetBattleSoulGodEquipConf()
	if nil == conf {
		return false, neterror.ConfNotFoundError("conf is nil")
	}
	_, ok := conf.Slot[pos]
	if !ok {
		return false, neterror.ConfNotFoundError("slot conf is nil")
	}

	if equipItemConf.SubType != pos {
		return false, neterror.ParamsInvalidError("pos not meet")
	}

	_, ok = s.owner.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
	if !ok {
		return false, neterror.SysNotExistError("bag sys get err")
	}
	if !s.owner.CheckItemCond(equipItemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}

	if !itemdef.IsBattleSoulGodEquipItem(equipItemConf.Type) {
		return false, neterror.ParamsInvalidError("not BattleSoulGodEquip type")
	}

	bsConf := jsondata.GetBattleSpiritsConf(battleSoulId)
	if nil == bsConf {
		return false, neterror.ConfNotFoundError("BattleSpirits conf %d not find", battleSoulId)
	}

	if equipItemConf.Quality > bsConf.GodEquipQuality {
		return false, neterror.ParamsInvalidError("quality limit")
	}

	return true, nil
}

func (s *BattleSoulGodEquipSys) takeOn(battleSoulId, pos uint32, hdl uint64) error {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	equip := bagSys.FindItemByHandle(hdl)
	if nil == equip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	ok, err := s.checkTakeOnSlotHandle(equip, battleSoulId, pos)
	if !ok {
		return neterror.Wrap(err)
	}

	oldEquip := s.getGodEquipByPos(battleSoulId, pos)
	if nil != oldEquip {
		if err := s.takeOff(battleSoulId, pos); err != nil {
			return neterror.Wrap(err)
		}
	}

	originSt, err := s.GetBattleSoulGodEquipSt(battleSoulId)
	if nil != err {
		return neterror.Wrap(err)
	}

	if removeSuccess := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogBattleSoulGodEquipTakeOn); !removeSuccess {
		return neterror.InternalError("remove soul halo hdl:%d item:%d failed", equip.GetHandle(), equip.GetItemId())
	}

	originSt.Equip[pos] = equip
	s.updateInfo(battleSoulId, pos, battleSoulGodEquipUpdateTypeTakeOn)
	s.afterTakeOn(battleSoulId, pos)

	if s.isZeroStatus(equip) && !s.isZeroStatus(oldEquip) {
		err = s.inherit(oldEquip.GetHandle(), equip.GetHandle())
		if err != nil {
			return neterror.Wrap(err)
		}
		s.owner.SendTipMsg(tipmsgid.TpStrongLvInheritSuccess)
	}

	return nil
}

func (s *BattleSoulGodEquipSys) afterTakeOn(battleSoulId, pos uint32) {
	s.ResetSysAttr(attrdef.SaBattleSoulGodEquip)
}

func (s *BattleSoulGodEquipSys) afterTakeOff(battleSoulId, pos uint32, oldEquip *pb3.ItemSt) {
	s.ResetSysAttr(attrdef.SaBattleSoulGodEquip)
}

func (s *BattleSoulGodEquipSys) GetBattleSoulGodEquipSt(battleSoulId uint32) (*pb3.BattleSoulGodEquipSt, error) {
	if !s.isValidBattleSoul(battleSoulId) {
		return nil, neterror.ParamsInvalidError("not valid battleSoulId")
	}
	data := s.getData()
	if nil == data.Info[battleSoulId] {
		data.Info[battleSoulId] = &pb3.BattleSoulGodEquipSt{}
	}
	if nil == data.Info[battleSoulId].Equip {
		data.Info[battleSoulId].Equip = make(map[uint32]*pb3.ItemSt)
	}
	return data.Info[battleSoulId], nil
}

func (s *BattleSoulGodEquipSys) takeOff(battleSoulId, pos uint32) error {
	originSt, err := s.GetBattleSoulGodEquipSt(battleSoulId)
	if nil != err {
		return neterror.Wrap(err)
	}

	oldEquip := s.getGodEquipByPos(battleSoulId, pos)
	if nil == oldEquip {
		return neterror.ParamsInvalidError("pos not has equip")
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBattleSoulGodEquipBag).(*BattleSoulGodEquipBagSys)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	delete(originSt.Equip, pos)
	s.updateInfo(battleSoulId, pos, battleSoulGodEquipUpdateTypeTakeOff)

	if success := bagSys.AddItemPtr(oldEquip, true, pb3.LogId_LogBattleSoulGodEquipTakeOff); !success {
		return neterror.InternalError("add item bag failed")
	}

	s.afterTakeOff(battleSoulId, pos, oldEquip)

	return nil
}

func (s *BattleSoulGodEquipSys) calcAttrs(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	for battleSoulId, v := range data.Info {
		statisticsMap := make(map[uint64]uint32)
		for pos, equip := range v.Equip {
			itemConf := jsondata.GetItemConfig(equip.GetItemId())
			if nil == itemConf {
				continue
			}
			//基础属性/极品属性
			engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)
			engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.PremiumAttrs)

			//基础属性/极品属性 加成属性
			if stageConf := jsondata.GetBattleSoulGodEquipStageConf(pos, equip.GetUnion2()); nil != stageConf {
				if stageConf.Rate > 0 {
					engine.CheckAddAttrsRateRoundingUp(s.owner, calc, itemConf.StaticAttrs, stageConf.Rate)
				}
				if stageConf.PremiumRate > 0 {
					engine.CheckAddAttrsRateRoundingUp(s.owner, calc, itemConf.PremiumAttrs, stageConf.PremiumRate)
				}
			}

			if strongConf := jsondata.GetBattleSoulGodEquipStrongConf(pos, equip.GetUnion1()); nil != strongConf {
				engine.CheckAddAttrsToCalc(s.owner, calc, strongConf.Attrs)
			}

			for star := uint32(1); star <= itemConf.Star; star++ {
				for quality := uint32(1); quality <= itemConf.Quality; quality++ {
					statisticsMap[utils.Make64(star, quality)]++
				}
			}
		}
		//套装属性
		suitConf := jsondata.GetBattleSoulGodEquipSuitConf(battleSoulId)
		if nil == suitConf {
			continue
		}
		for _, suit := range suitConf.Suit {
			if statisticsMap[utils.Make64(suit.Star, suit.Quality)] < suit.Num {
				continue
			}
			engine.CheckAddAttrsToCalc(s.owner, calc, suit.Attrs)
		}
	}
}

func calcBattleSoulGodEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiBattleSoulGodEquip).(*BattleSoulGodEquipSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrs(calc)
}

func init() {
	RegisterSysClass(sysdef.SiBattleSoulGodEquip, func() iface.ISystem {
		return &BattleSoulGodEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaBattleSoulGodEquip, calcBattleSoulGodEquipAttr)

	net.RegisterSysProtoV2(11, 112, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sStrongUp
	})
	net.RegisterSysProtoV2(11, 113, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sStageUp
	})
	net.RegisterSysProtoV2(11, 114, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sInherit
	})
	net.RegisterSysProtoV2(11, 115, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sBack
	})
	net.RegisterSysProtoV2(11, 116, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(11, 117, sysdef.SiBattleSoulGodEquip, func(s iface.ISystem) func(*base.Message) error {
		return s.(*BattleSoulGodEquipSys).c2sTakeOff
	})
}
