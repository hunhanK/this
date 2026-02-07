/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type NewSpiritSys struct {
	Base
}

func (s *NewSpiritSys) s2cInfo() {
	s.SendProto3(9, 100, &pb3.S2C_9_100{
		Data: s.getData(),
	})
}

func (s *NewSpiritSys) getData() *pb3.NewSpiritData {
	data := s.GetBinaryData().NewSpiritData
	if data == nil {
		s.GetBinaryData().NewSpiritData = &pb3.NewSpiritData{}
		data = s.GetBinaryData().NewSpiritData
	}
	if data.PosSpirit == nil {
		data.PosSpirit = make(map[uint32]*pb3.ItemSt)
	}
	return data
}

func (s *NewSpiritSys) OnReconnect() {
	s.s2cInfo()
}

func (s *NewSpiritSys) OnLogin() {
	s.s2cInfo()
}

func (s *NewSpiritSys) OnOpen() {
	s.s2cInfo()
}

func (s *NewSpiritSys) LearnSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		s.owner.LearnSkill(itemConf.EquipSkill, itemConf.EquipSkillLv, true)
	}
}

func (s *NewSpiritSys) ForgetSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		s.owner.ForgetSkill(itemConf.EquipSkill, true, true, true)
	}
}

func (s *NewSpiritSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_9_101
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	pos := req.Pos
	handle := req.Handle
	config := jsondata.GetNewSpiritPosConfig(pos)
	if config == nil {
		return neterror.ParamsInvalidError("not found pos %d config", pos)
	}

	owner := s.GetOwner()
	if config.OpenLv != 0 && config.OpenLv > owner.GetLevel() {
		return neterror.ParamsInvalidError("%d need open lv %d", pos, config.OpenLv)
	}

	if config.OpenVipLv != 0 && config.OpenVipLv > owner.GetVipLevel() {
		return neterror.ParamsInvalidError("%d need open vip lv %d", pos, config.OpenVipLv)
	}

	newItemSt := owner.GetItemByHandle(handle)
	if newItemSt == nil {
		return neterror.ParamsInvalidError("not found %d item", handle)
	}

	itemConfig := jsondata.GetItemConfig(newItemSt.ItemId)
	if itemConfig == nil {
		return neterror.ParamsInvalidError("not found %d item config", newItemSt.ItemId)
	}

	spiritId := itemConfig.CommonField
	spiritConfig := jsondata.GetNewSpiritConfig(spiritId)
	if spiritConfig == nil {
		return neterror.ParamsInvalidError("not found %d spirit config", spiritId)
	}

	data := s.getData()
	for p, itemSt := range data.PosSpirit {
		if pos != p {
			continue
		}
		spiritId := itemSt.Union1
		sConfig := jsondata.GetNewSpiritConfig(spiritId)
		if sConfig == nil {
			return neterror.ParamsInvalidError("not found %d spirit config", spiritId)
		}
		if sConfig.Type == spiritConfig.Type {
			return neterror.ParamsInvalidError("pos %d already have %d spirit", pos, spiritId)
		}
	}

	newItemSt.Union1 = spiritId
	if !owner.RemoveItemByHandle(handle, pb3.LogId_LogNewSpiritTakeOn) {
		return neterror.ParamsInvalidError("%d item remove failed", handle)
	}

	oldItemSt := data.PosSpirit[pos]
	var isSamePos bool
	if oldItemSt != nil && oldItemSt.Handle == data.AppearHandle {
		s.appearTakeOff()
		isSamePos = true
	}
	if oldItemSt != nil {
		s.ForgetSkill(oldItemSt)
	}

	data.PosSpirit[pos] = newItemSt
	if isSamePos {
		data.AppearHandle = handle
		s.appearTakeOn()
	}
	s.LearnSkill(newItemSt)
	if oldItemSt != nil {
		owner.AddItemPtr(oldItemSt, true, pb3.LogId_LogNewSpiritTakeOn)
	}

	s.SendProto3(9, 101, &pb3.S2C_9_101{
		Pos:  pos,
		Item: newItemSt,
	})
	owner.TriggerQuestEventRange(custom_id.QttTakeOnSpecNewSpirit)
	s.ResetSysAttr(attrdef.SaNewSpirit)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNewSpiritTakeOn, &pb3.LogPlayerCounter{
		NumArgs: uint64(spiritId),
		StrArgs: fmt.Sprintf("%d", pos),
	})
	return nil
}

func (s *NewSpiritSys) s2cAppearHandle() {
	s.SendProto3(9, 102, &pb3.S2C_9_102{
		AppearHandle: s.getData().AppearHandle,
	})
}

func (s *NewSpiritSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_9_102
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	// 取消幻化
	handle := req.AppearHandle
	if handle == 0 {
		s.appearTakeOff()
		s.s2cAppearHandle()
		return nil
	}
	data := s.getData()
	if data.AppearHandle == handle {
		return neterror.ParamsInvalidError("already appear %d", handle)
	}
	var itemSt *pb3.ItemSt
	for _, val := range data.PosSpirit {
		if val.Handle != handle {
			continue
		}
		itemSt = val
	}
	if itemSt == nil || itemSt.Union1 == 0 {
		return neterror.ParamsInvalidError("not found %d item", handle)
	}
	if data.AppearHandle != 0 {
		s.appearTakeOff()
	}
	data.AppearHandle = handle
	s.appearTakeOn()
	s.s2cAppearHandle()
	return nil
}

func (s *NewSpiritSys) appearTakeOn() {
	data := s.getData()
	if data.AppearHandle == 0 {
		return
	}
	var itemSt *pb3.ItemSt
	for _, val := range data.PosSpirit {
		if val.Handle != data.AppearHandle {
			continue
		}
		itemSt = val
	}
	if itemSt == nil || itemSt.Union1 == 0 {
		data.AppearHandle = 0
		s.s2cInfo()
		return
	}
	err := s.GetOwner().CallActorFunc(actorfuncid.G2FTakeOnNewSpirit, &pb3.G2FTakeOnSpirit{
		SpiritId: itemSt.Union1,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
	s.SendProto3(9, 102, &pb3.S2C_9_102{
		AppearHandle: data.AppearHandle,
	})
	return
}

func (s *NewSpiritSys) appearTakeOff() {
	data := s.getData()
	if data.AppearHandle == 0 {
		return
	}
	var itemSt *pb3.ItemSt
	for _, val := range data.PosSpirit {
		if val.Handle != data.AppearHandle {
			continue
		}
		itemSt = val
	}
	if itemSt == nil || itemSt.Union1 == 0 {
		data.AppearHandle = 0
		s.s2cInfo()
		return
	}
	data.AppearHandle = 0
	err := s.GetOwner().CallActorFunc(actorfuncid.G2FTakeOffNewSpirit, &pb3.G2FTakeOnSpirit{
		SpiritId: itemSt.Union1,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
}

func (s *NewSpiritSys) AfterTakeReplace(_ uint32, eq *pb3.ItemSt, oldEq *pb3.ItemSt) {
	data := s.getData()
	if data.AppearHandle == oldEq.Handle {
		s.appearTakeOff()
		s.ForgetSkill(oldEq)
	}
	data.AppearHandle = eq.Handle
	s.appearTakeOn()
	s.ForgetSkill(eq)
	s.ResetSysAttr(attrdef.SaNewSpirit)
}

func (s *NewSpiritSys) takeOff(pos uint32, eq *pb3.ItemSt) {
	data := s.getData()
	if data.AppearHandle == eq.Handle {
		s.appearTakeOff()
	}
	s.ForgetSkill(eq)
	delete(data.PosSpirit, pos)
}

func handleNewSpiritLoginFight(player iface.IPlayer, _ ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiNewSpirit).(*NewSpiritSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.appearTakeOn()
}

func init() {
	RegisterSysClass(sysdef.SiNewSpirit, func() iface.ISystem {
		return &NewSpiritSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaNewSpirit, calcSaNewSpirit)
	net.RegisterSysProtoV2(9, 101, sysdef.SiNewSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewSpiritSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(9, 102, sysdef.SiNewSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewSpiritSys).c2sAppear
	})
	event.RegActorEventL(custom_id.AeLoginFight, handleNewSpiritLoginFight)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnSpecNewSpirit, handleQttTakeOnSpecNewSpirit)
}

func handleQttTakeOnSpecNewSpirit(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	sys, ok := actor.GetSysObj(sysdef.SiNewSpirit).(*NewSpiritSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	if len(ids) == 0 {
		return uint32(len(data.PosSpirit))
	}
	var count uint32
	var posSpiritSet = make(map[uint32]struct{})
	for _, itemSt := range data.PosSpirit {
		posSpiritSet[itemSt.Union1] = struct{}{}
	}
	for _, spiritId := range ids {
		if _, ok := posSpiritSet[spiritId]; ok {
			count++
		}
	}
	return count
}

func calcSaNewSpirit(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiNewSpirit).(*NewSpiritSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := sys.getData()
	for _, itemSt := range data.PosSpirit {
		conf := jsondata.GetItemConfig(itemSt.ItemId)
		if nil == conf {
			continue
		}
		//基础属性
		engine.CheckAddAttrsToCalc(player, calc, conf.StaticAttrs)
		//品质属性
		engine.CheckAddAttrsSelectQualityToCalc(player, calc, conf.SuperAttrs, conf.Quality)
		//极品属性
		engine.CheckAddAttrsSelectQualityToCalc(player, calc, conf.PremiumAttrs, conf.Quality)

	}
}
