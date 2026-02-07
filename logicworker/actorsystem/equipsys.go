package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
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
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils"

	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

/*
	desc:装备系统
	author: LvYuMeng
	maintainer:ChenJunJi
*/

type EquipSystem struct {
	Base
}

func (sys *EquipSystem) OnInit() {
	mainData := sys.GetMainData()
	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.Equips {
		itemPool.Equips = make([]*pb3.ItemSt, 0)
	}
}

func (sys *EquipSystem) IsOpen() bool {
	return true
}

func (sys *EquipSystem) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *EquipSystem) OnReconnect() {
	sys.S2CInfo()
}

func (sys *EquipSystem) c2sInfo(msg *base.Message) {
	sys.S2CInfo()
}

func (sys *EquipSystem) S2CInfo() {
	if mData := sys.GetMainData(); nil != mData {
		sys.SendProto3(11, 0, &pb3.S2C_11_0{
			Equips: mData.ItemPool.Equips,
		})
	}
}

func (sys *EquipSystem) CheckTakeOnPosHandle(equip *pb3.ItemSt, pos uint32) (bool, error) {
	_, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, neterror.SysNotExistError("bag sys get err")
	}
	conf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == conf {
		return false, neterror.ConfNotFoundError("item conf(%d) nil", equip.GetItemId())
	}
	if !itemdef.IsEquip(conf.Type, conf.SubType) {
		sys.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}
	if itemdef.EquipPosTypeMap[pos] != conf.SubType {
		sys.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}
	if !sys.owner.CheckItemCond(conf) {
		sys.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return false, nil
	}
	return true, nil
}

func (sys *EquipSystem) TakeOn(handle uint64, pos uint32) error {
	bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}
	equip := bagSys.FindItemByHandle(handle)
	if nil == equip {
		return neterror.ParamsInvalidError("equip hdl:%d is nil", handle)
	}
	canTake, err := sys.CheckTakeOnPosHandle(equip, pos)
	if err != nil || !canTake {
		return err
	}
	_, oldEquip := sys.GetEquipByPos(pos)
	// 走替换逻辑
	if nil != oldEquip {
		err := sys.Replace(handle, pos)
		return err
	}
	bagSys.RemoveItemByHandle(handle, pb3.LogId_LogTakeOnEquip)
	equip.Pos = pos
	mainData := sys.GetMainData()
	mainData.ItemPool.Equips = append(mainData.ItemPool.Equips, equip)

	sys.SendProto3(11, 1, &pb3.S2C_11_1{Ret: custom_id.Success, Equip: equip})
	sys.AfterTakeOn(equip)
	return nil
}

func (sys *EquipSystem) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_11_1
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	err = sys.TakeOn(req.Handle, req.Pos)
	return err
}

func (sys *EquipSystem) Replace(nhdl uint64, pos uint32) error {
	bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}
	equip := bagSys.FindItemByHandle(nhdl)
	if nil == equip {
		return neterror.ParamsInvalidError("equip %d is nil", nhdl)
	}
	canTake, err := sys.CheckTakeOnPosHandle(equip, pos)
	if nil != err || !canTake {
		return err
	}
	idx, oldEquip := sys.GetEquipByPos(pos)
	if nil == oldEquip {
		return neterror.InternalError("there is no equip in pos(%d)", pos)
	}
	if sys.DelEquip(uint32(idx), oldEquip) > 0 {
		return neterror.InternalError("equip(%d) that taken on remove from pos(%d) fail", oldEquip.GetHandle(), pos)
	}
	bagSys.RemoveItemByHandle(nhdl, pb3.LogId_LogTakeOnEquip)

	sys.SendProto3(11, 2, &pb3.S2C_11_2{Ret: custom_id.Success, Pos: pos})

	mainData := sys.GetMainData()
	equip.Pos = pos
	mainData.ItemPool.Equips = append(mainData.ItemPool.Equips, equip)
	bagSys.AddItemPtr(oldEquip, true, pb3.LogId_LogTakeOffEquip)
	sys.SendProto3(11, 1, &pb3.S2C_11_1{Ret: custom_id.Success, Equip: equip})
	sys.AfterTakeReplace(equip, oldEquip, pos)
	return nil
}

func (sys *EquipSystem) GetEquipByPos(pos uint32) (int, *pb3.ItemSt) {
	equips := sys.GetMainData().ItemPool.Equips

	if nil == equips {
		return 0, nil
	}

	for idx, equip := range equips {
		if equip.Pos == pos {
			return idx, equip
		}
	}
	return 0, nil
}

// GetEquipLQSSByPos 获取装备的等级 品质 星级 阶级
func GetEquipLQSSByPos(actor iface.IPlayer, ids []uint32, _ ...interface{}) (uint32, uint32, uint32, uint32) {
	if len(ids) <= 0 {
		return 0, 0, 0, 0
	}
	pos := ids[0]
	sys := actor.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	_, equip := sys.GetEquipByPos(pos)
	if nil == equip {
		return 0, 0, 0, 0
	}
	confId := equip.ItemId
	conf := jsondata.GetItemConfig(confId)
	return conf.Level, conf.Quality, conf.Star, conf.Stage
}

func (sys *EquipSystem) DelEquip(idx uint32, equip *pb3.ItemSt) (result int) {
	equips := sys.GetMainData().ItemPool.Equips

	if nil == equips {
		return 1
	}

	size := len(equips)
	if size <= 0 {
		return 2
	}

	last := size - 1
	if idx != uint32(last) {
		equips[idx] = equips[last]
	}

	sys.GetMainData().ItemPool.Equips = equips[:last]
	equip.Pos = 0
	return 0
}

func (sys *EquipSystem) AfterTakeReplace(equip *pb3.ItemSt, oldEquip *pb3.ItemSt, pos uint32) {
	sys.ResetSysAttr(attrdef.SaEquip)
	sys.ForgetSkill(oldEquip)
	sys.LearnSkill(equip)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipStage)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipDefCond)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnJewelryEquipStage)
	sys.owner.TriggerEvent(custom_id.AeTakeReplaceEquip, equip, oldEquip, pos)
}

func (sys *EquipSystem) AfterTakeOn(equip *pb3.ItemSt) {
	sys.ResetSysAttr(attrdef.SaEquip)
	sys.LearnSkill(equip)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipStage)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnJewelryEquipStage)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipDefCond)
	sys.owner.TriggerQuestEvent(custom_id.QttTakeOnEquipPos, equip.Pos, 1)
	sys.owner.TriggerEvent(custom_id.AeTakeOnEquip, equip)
}

func (sys *EquipSystem) AfterTakeOff(equip *pb3.ItemSt, pos uint32) {
	sys.ResetSysAttr(attrdef.SaEquip)
	sys.ForgetSkill(equip)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipStage)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnEquipDefCond)
	sys.owner.TriggerQuestEventRange(custom_id.QttTakeOnJewelryEquipStage)
	sys.owner.TriggerEvent(custom_id.AeTakeOffEquip, equip, pos)
}

func (sys *EquipSystem) LearnSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		sys.owner.LearnSkill(itemConf.EquipSkill, itemConf.EquipSkillLv, true)
	}
}

func (sys *EquipSystem) ForgetSkill(equip *pb3.ItemSt) {
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if itemConf == nil {
		return
	}
	if itemConf.EquipSkill > 0 {
		sys.owner.ForgetSkill(itemConf.EquipSkill, true, true, true)
	}
}

func (sys *EquipSystem) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_11_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if _, err := sys.TakeOff(req.Pos, false); nil != err {
		return err
	}
	return nil
}

func (sys *EquipSystem) TakeOff(pos uint32, onReplace bool) (bool, error) {
	bagSys, ok := sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return false, neterror.SysNotExistError("bag sys get err")
	}

	if pos == itemdef.EtWeddingRingPos {
		return false, neterror.SysNotExistError("weddingRing cannot take off")
	}

	if bagSys.AvailableCount() <= 0 {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return false, nil
	}

	idx, equip := sys.GetEquipByPos(pos)
	if nil == equip {
		return false, neterror.InternalError("there is no equip in pos(%d)", pos)
	}

	if sys.DelEquip(uint32(idx), equip) > 0 {
		return false, neterror.InternalError("equip(%d) that taken on remove from pos(%d) fail", equip.GetHandle(), pos)
	}

	bagSys.AddItemPtr(equip, true, pb3.LogId_LogTakeOffEquip)

	sys.AfterTakeOff(equip, pos)

	sys.SendProto3(11, 2, &pb3.S2C_11_2{Ret: 0, Pos: pos})

	return true, nil
}

func calcEquipSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	mData := player.GetMainData()
	for _, equip := range mData.ItemPool.Equips {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}
		//基础属性
		engine.CheckAddAttrsToCalc(player, calc, conf.StaticAttrs)
		//极品属性
		engine.CheckAddAttrsToCalc(player, calc, conf.PremiumAttrs)
		//品质属性
		engine.CheckAddAttrsSelectQualityToCalc(player, calc, conf.SuperAttrs, conf.Quality)
	}
}

func calcEquipSysAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	mData := player.GetMainData()

	otherSysAddRate := totalSysCalc.GetValue(attrdef.StaticAttrsRate)
	weaponAddRate := totalSysCalc.GetValue(attrdef.EquipWeaponAddRate)
	clothAddRate := totalSysCalc.GetValue(attrdef.EquipClothAddRate)

	for _, equip := range mData.ItemPool.Equips {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}

		//基础属性
		if otherSysAddRate != 0 {
			engine.CheckAddAttrsRateRoundingUp(player, calc, conf.StaticAttrs, uint32(otherSysAddRate))
		}

		var wAttrs, cAttrs jsondata.AttrVec
		//武器基础属性加成
		if equip.Pos == itemdef.EtWeapon && weaponAddRate != 0 {
			for _, attr := range conf.StaticAttrs {
				if attr.Type != attrdef.Attack {
					continue
				}
				value := utils.CalcMillionRate(attr.Value, uint32(weaponAddRate))
				wAttrs = append(wAttrs, &jsondata.Attr{Type: attr.Type, Value: value, Job: attr.Job})
			}
		}

		//衣服基础属性加成
		if equip.Pos == itemdef.EtClothes && clothAddRate != 0 {
			for _, attr := range conf.StaticAttrs {
				if attr.Type != attrdef.Defend {
					continue
				}
				value := utils.CalcMillionRate(attr.Value, uint32(clothAddRate))
				cAttrs = append(cAttrs, &jsondata.Attr{Type: attr.Type, Value: value, Job: attr.Job})
			}
		}
		if len(wAttrs) > 0 {
			engine.CheckAddAttrsToCalc(player, calc, wAttrs)
		}
		if len(cAttrs) > 0 {
			engine.CheckAddAttrsToCalc(player, calc, cAttrs)
		}
	}
}

func handlePowerRushRankSubTypeEquip(player iface.IPlayer) int64 {
	return player.GetAttrSys().GetSysPower(attrdef.SaEquip)
}

func equipOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	sysPowerMap, ok := args[0].(map[uint32]int64)
	if !ok {
		return
	}
	equipValue := sysPowerMap[attrdef.SaEquip]
	equipValue += sysPowerMap[attrdef.SaEquipStrong]
	equipValue += sysPowerMap[attrdef.SaGem]
	player.SetRankValue(gshare.RankTypeEquip, equipValue)
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeEquip, player.GetId(), equipValue)
}

func handleQttTakeOnEquipStage(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	var count uint32
	mData := player.GetMainData()
	for _, equip := range mData.ItemPool.Equips {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}

		var add bool
		switch len(ids) {
		case 1:
			add = conf.Stage >= ids[0]
		case 2:
			add = conf.Stage >= ids[0] && conf.Quality >= ids[1]
		case 3:
			add = conf.Stage >= ids[0] && conf.Quality >= ids[1] && conf.Star >= ids[2]
		}

		if add {
			count++
		}
	}
	return count
}

func handleTakeOnEquipDefCond(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 4 {
		return 0
	}
	sys, ok := player.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok || !sys.IsOpen() {
		return 0
	}

	var (
		pos     = ids[0]
		stage   = ids[1]
		quality = ids[2]
		star    = ids[3]
	)

	_, equip := sys.GetEquipByPos(pos)
	if nil == equip {
		return 0
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return 0
	}
	if itemConf.Stage > 0 && itemConf.Stage < stage {
		return 0
	}
	if itemConf.Quality > 0 && itemConf.Quality < quality {
		return 0
	}
	if itemConf.Star > 0 && itemConf.Star < star {
		return 0
	}

	return 1
}

func handleQttTakeOnJewelryEquipStage(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	var count uint32
	mData := player.GetMainData()
	for _, equip := range mData.ItemPool.Equips {
		conf := jsondata.GetItemConfig(equip.GetItemId())
		if nil == conf {
			continue
		}
		if equip.Pos < itemdef.EtNecklace {
			continue
		}
		if equip.Pos > itemdef.EtBracelet {
			continue
		}
		var add bool
		switch len(ids) {
		case 1:
			add = conf.Stage >= ids[0]
		case 2:
			add = conf.Stage >= ids[0] && conf.Quality >= ids[1]
		case 3:
			add = conf.Stage >= ids[0] && conf.Quality >= ids[1] && conf.Star >= ids[2]
		}

		if add {
			count++
		}
	}
	return count
}

func handleQttTakeOnEquipPos(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	pos := ids[0]
	if sys, ok := actor.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip := sys.GetEquipByPos(pos)
		if nil != equip {
			return 1
		}
	}
	return 0
}

func handleCompositeEquipMultiCond(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 3 || len(args) < 1 {
		return 0
	}
	itemId, ok := args[0].(uint32)
	if !ok {
		return 0
	}
	stage, quality, star := ids[0], ids[1], ids[2]
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return 0
	}
	if itemConf.Stage < stage {
		return 0
	}
	if itemConf.Quality < quality {
		return 0
	}
	if itemConf.Star < star {
		return 0
	}
	return 1
}

func handleJobChangeBaseEquip(player iface.IPlayer, job uint32) bool {
	mData := player.GetMainData()
	if nil == mData {
		return false
	}
	pool := mData.ItemPool
	if pool == nil {
		return false
	}
	sys, ok := player.GetSysObj(sysdef.SiEquip).(*EquipSystem)
	if !ok {
		return true
	}
	if len(pool.Equips) != 0 {
		for _, line := range pool.Equips {
			itemId := jsondata.GetJobChangeItemConfByIdAndJob(line.ItemId, job)
			if itemId == 0 {
				player.LogWarn("Id:%d itemId=0!!!", player.GetId())
				continue
			}
			sys.LearnSkill(line)
			line.ItemId = itemId
			sys.ForgetSkill(line)
		}
	}
	return true
}

func init() {
	RegisterSysClass(sysdef.SiEquip, func() iface.ISystem {
		return &EquipSystem{}
	})
	engine.RegAttrCalcFn(attrdef.SaEquip, calcEquipSysAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaEquip, calcEquipSysAttrAddRate)
	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, equipOnUpdateSysPowerMap)

	engine.RegQuestTargetProgress(custom_id.QttTakeOnEquipStage, handleQttTakeOnEquipStage)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnJewelryEquipStage, handleQttTakeOnJewelryEquipStage)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnEquipPos, handleQttTakeOnEquipPos)
	engine.RegQuestTargetProgress(custom_id.QttCompositeEquipMultiCond, handleCompositeEquipMultiCond)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnEquipDefCond, handleTakeOnEquipDefCond)

	net.RegisterSysProto(11, 0, sysdef.SiEquip, (*EquipSystem).c2sInfo)
	net.RegisterSysProto(11, 1, sysdef.SiEquip, (*EquipSystem).c2sTakeOn)
	net.RegisterSysProto(11, 2, sysdef.SiEquip, (*EquipSystem).c2sTakeOff)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeEquip, handlePowerRushRankSubTypeEquip)
	jobchange.RegJobChangeFunc(jobchange.BaseEquip, &jobchange.Fn{Fn: handleJobChangeBaseEquip})
}
