/**
 * @Author: lzp
 * @Date: 2023/11/9
 * @Desc: 龙装
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

const (
	DragonAmType1 = 1 // 龙武
	DragonAmType2 = 2 // 龙甲
)

const (
	DeBegin      = 1
	DEquSpirit   = 1  //龙灵
	DEquHorn     = 2  //龙角
	DEquWings    = 3  //龙翼
	DEquTail     = 4  //龙尾
	DEquPei      = 5  //龙佩
	DEquBracelet = 6  //龙镯
	DEquRing     = 7  //龙戒
	DEquChain    = 8  //龙链
	DEquCrown    = 9  //龙冠
	DEquWrist    = 10 //龙腕
	DEquLegs     = 11 //龙腿
	DEquBoots    = 12 //龙靴
	DEEnd
)

type DragonSys struct {
	Base
	expUpLvs map[uint32]*uplevelbase.ExpUpLv
}

func (sys *DragonSys) OnLogin() {
	data := sys.GetData()
	sys.InitExpUpLv(data)
	sys.S2CInfo()
}

func (sys *DragonSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *DragonSys) OnOpen() {
	data := sys.GetData()
	data.SoulLv = 0
	data.DragonAmData.ExpLvs[DragonAmType1] = &pb3.ExpLvSt{Lv: 1}
	data.DragonAmData.ExpLvs[DragonAmType2] = &pb3.ExpLvSt{Lv: 1}
	sys.InitExpUpLv(data)
	sys.ResetAllSysAttr()
	sys.S2CInfo()
}

func (sys *DragonSys) S2CInfo() {
	msg := &pb3.S2C_50_1{}
	data := sys.GetData()
	if data == nil {
		return
	}
	msg.Data = data
	sys.SendProto3(50, 1, msg)
}

func (sys *DragonSys) ResetAllSysAttr() {
	sys.ResetSysAttr(attrdef.SaDragonSoul)
	sys.ResetSysAttr(attrdef.SaDragonEqu)
	sys.ResetSysAttr(attrdef.SaDragonAm1)
	sys.ResetSysAttr(attrdef.SaDragonAm2)
}

// Get
func (sys *DragonSys) GetData() *pb3.DragonData {
	if sys.GetBinaryData().DragonData == nil {
		sys.GetBinaryData().DragonData = &pb3.DragonData{}
	}

	dragonData := sys.GetBinaryData().DragonData

	if dragonData.DragonEqData == nil {
		dragonData.DragonEqData = &pb3.DragonEqData{}
	}

	eqData := dragonData.GetDragonEqData()
	if eqData.Equips == nil {
		eqData.Equips = make(map[uint32]uint32)
	}

	if eqData.Fashions == nil {
		eqData.Fashions = make(map[uint32]*pb3.DragonEqFashion)
	}

	if dragonData.DragonAmData == nil {
		dragonData.DragonAmData = &pb3.DragonAmData{}
	}

	amData := dragonData.DragonAmData
	if amData.ExpLvs == nil {
		amData.ExpLvs = make(map[uint32]*pb3.ExpLvSt)
	}

	if amData.ExpLvs[DragonAmType1] == nil {
		amData.ExpLvs[DragonAmType1] = &pb3.ExpLvSt{}
	}

	if amData.ExpLvs[DragonAmType2] == nil {
		amData.ExpLvs[DragonAmType2] = &pb3.ExpLvSt{}
	}

	if amData.Stars == nil {
		amData.Stars = make(map[uint32]uint32)
	}

	if amData.Medicines == nil {
		amData.Medicines = make(map[uint32]*pb3.UseCounter)
	}

	return dragonData
}

func (sys *DragonSys) GetDragonAmData() *pb3.DragonAmData {
	return sys.GetData().DragonAmData
}

func (sys *DragonSys) GetDragonEqData() *pb3.DragonEqData {
	return sys.GetData().DragonEqData
}

func (sys *DragonSys) GetDragonSoulLv() uint32 {
	return sys.GetData().SoulLv
}

func (sys *DragonSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_50_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	oldLv := sys.GetDragonSoulLv()
	newLv := oldLv + 1

	conf := jsondata.GetDragonSoulConf(newLv)
	if conf == nil {
		return neterror.ParamsInvalidError("lv not found: %v", newLv)
	}
	if !sys.owner.ConsumeByConf(conf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogDragonSoulUpLv}) {
		return neterror.ConsumeFailedError("consume failed: %v", conf.Consume)
	}
	sys.GetData().SoulLv = newLv
	sys.onUpLv(newLv)

	sys.SendProto3(50, 2, &pb3.S2C_50_2{Lv: newLv})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogDragonSoulUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(newLv),
	})
	return nil
}

// 前端直接请求的等级
func (sys *DragonSys) c2sChangeSoulAppear(msg *base.Message) error {
	var req pb3.C2S_50_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}
	// 脱下
	if req.Lv == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_DragonSoul)
		return nil
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_DragonSoul, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_DragonSoul,
		AppearId: req.Lv,
	}, true)

	return nil
}

func (sys *DragonSys) c2sChangeEquAppear(msg *base.Message) error {
	var req pb3.C2S_50_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	// 检查slot的合理性
	equConf := jsondata.GetDragonEquConfBySlot(req.Slot)
	if equConf == nil {
		return neterror.ParamsInvalidError("dragon equipment not found: %v", req.Slot)
	}

	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return neterror.InternalError("dragon equipment data not init")
	}

	// 幻化检查
	if req.FashionId > 0 {
		fashion := eqData.Fashions[req.Slot]
		if fashion == nil || !utils.SliceContainsUint32(fashion.FIds, req.FashionId) {
			return neterror.ParamsInvalidError("not have fashion id = %v", req.FashionId)
		}
	}

	// 获取龙装部位的appear_pos
	posId := getDragonEquAppearPos(equConf.ReqType)
	if posId <= 0 {
		return neterror.ParamsInvalidError("pos = %v not have appear", req.Slot)
	}

	// 脱下
	if req.FashionId == 0 {
		sys.GetOwner().TakeOffAppear(posId)
		return nil
	}

	// 幻化
	sys.GetOwner().TakeOnAppear(posId, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_DragonEqu,
		AppearId: req.FashionId,
	}, true)

	return nil
}

func (sys *DragonSys) c2sTakeEquipOn(msg *base.Message) error {
	var req pb3.C2S_50_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	item := sys.GetOwner().GetItemByHandle(req.EquipHandle)
	if item == nil {
		return neterror.ParamsInvalidError("dragonEquip not found: %d", req.EquipHandle)
	}

	itemConf := jsondata.GetItemConfig(item.ItemId)
	if itemConf == nil || itemConf.Type != itemdef.ItemTypeDragonEquip {
		return neterror.ParamsInvalidError("equip is not dragonEquip: %d", req.EquipHandle)
	}

	// 检查穿戴位置
	conf := jsondata.GetDragonEquConfBySlot(req.Slot)
	if conf == nil || conf.ReqType != itemConf.SubType {
		return neterror.ParamsInvalidError("dragonEquip equip slot EquipHandle : %d, slot : %d", req.EquipHandle, req.Slot)
	}

	player := sys.GetOwner()
	_, suitMap1 := jsondata.GetDragonEquSuitConf(sys.GetDragonEqData().Equips)

	if err := sys.takeOnEquip(player, req.EquipHandle, req.Slot); err != nil {
		return err
	}

	// 激活时装
	sys.activateEquFashion(itemConf)

	// 重算属性
	player.GetAttrSys().ResetSysAttr(attrdef.SaDragonEqu)

	pbMsg := &pb3.S2C_50_4{
		Slot:   req.Slot,
		ItemId: itemConf.Id,
	}

	fashionMap := sys.GetDragonEqData().Fashions
	if fashionMap != nil && fashionMap[itemConf.SubType] != nil {
		pbMsg.FIds = fashionMap[itemConf.SubType].FIds
	}

	sys.SendProto3(50, 4, pbMsg)

	// 统计套装激活
	_, suitMap2 := jsondata.GetDragonEquSuitConf(sys.GetDragonEqData().Equips)
	for k, v := range suitMap2 {
		if suitMap1[k] != v {
			logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogDragonActiveSuit, &pb3.LogPlayerCounter{
				NumArgs: uint64(v),
				StrArgs: utils.I32toa(k),
			})
		}
	}
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByDragon)

	return nil
}

func (sys *DragonSys) c2sTakeEquipOff(msg *base.Message) error {
	var req pb3.C2S_50_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return neterror.InternalError("dragon equip data not init")
	}

	_, ok := eqData.Equips[req.Slot]
	if !ok {
		return neterror.InternalError("not dragon in equip slot: %d", req.Slot)
	}

	player := sys.GetOwner()
	if err := sys.takeOffEquip(player, req.Slot); err != nil {
		return err
	}

	// 重算属性
	player.GetAttrSys().ResetSysAttr(attrdef.SaDragonEqu)

	// 推送
	sys.SendProto3(50, 5, &pb3.S2C_50_5{Slot: req.Slot})

	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByDragon)

	if s := sys.GetOwner().GetSysObj(sysdef.SiKillDragonEquipSuit); s != nil && s.IsOpen() {
		s.(*KillDragonEquipSuitSys).onDragonEquipTakeOff(req.Slot)
	}
	return nil
}

func (sys *DragonSys) c2sAddExp(msg *base.Message) error {
	var req pb3.C2S_50_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	if req.ItemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	lvUpItem := jsondata.GetDragonAmConf(req.AmType).LevelUpItem

	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}

		if !utils.SliceContainsUint32(lvUpItem, item.ItemId) {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	addExp := uint64(0)
	for _, entry := range req.ItemMap {
		item := sys.owner.GetItemByHandle(entry.Key)

		itemConf := jsondata.GetItemConfig(item.ItemId)

		addExp += uint64(itemConf.CommonField * entry.Value)

		if !sys.GetOwner().DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogWeaponUpLv) {
			return neterror.ParamsInvalidError("sys.GetOwner().RemoveItemByHandle err")
		}
	}

	err := sys.expUpLvs[req.AmType].AddExp(sys.GetOwner(), addExp)
	if err != nil {
		return err
	}

	return nil
}

func (sys *DragonSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_50_8
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	conf := jsondata.GetDragonAmConf(req.AmType)
	if conf == nil || len(conf.DragonStar) <= 0 {
		return neterror.ParamsInvalidError("id = %d config not found", req.AmType)
	}

	if sys.GetDragonAmData() == nil {
		return neterror.InternalError("dragon amData not init")
	}

	if !sys.Upgrade(req.AmType) {
		return neterror.ParamsInvalidError("id = %d upgrade err", req.AmType)
	}

	amData := sys.GetDragonAmData()
	pbMsg := &pb3.S2C_50_8{
		AmType: req.AmType,
		Stage:  amData.Stars[req.AmType],
	}
	sys.SendProto3(50, 8, pbMsg)

	return nil
}

func (sys *DragonSys) c2sChangeAmAppear(msg *base.Message) error {
	var req pb3.C2S_50_10
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	amConf := jsondata.GetDragonAmConf(req.AmType)
	if amConf == nil || len(amConf.DragonStar) <= 0 {
		return neterror.ParamsInvalidError("id=%d config not found", req.AmType)
	}

	amData := sys.GetDragonAmData()
	if amData == nil {
		return neterror.InternalError("dragon amData not init")
	}

	starConf, ok := amConf.DragonStar[req.Star]
	if !ok || starConf.SkinId == 0 {
		return neterror.ParamsInvalidError("id=%d star=%d skin not found", req.AmType, req.Star)
	}

	if req.Star > amData.Stars[req.AmType] {
		return neterror.ParamsInvalidError("id=%d star=%d not enough", req.AmType, req.Star)
	}

	posId := uint32(0)
	switch req.AmType {
	case DragonAmType1:
		posId = appeardef.AppearPos_Weapon
	case DragonAmType2:
		posId = appeardef.AppearPos_Cloth
	}

	if req.Opt == 0 {
		sys.GetOwner().TakeOffAppear(posId)
		return nil
	}

	sys.GetOwner().TakeOnAppear(posId, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_DragonAm,
		AppearId: req.Star,
	}, true)

	return nil
}

func (sys *DragonSys) c2sActiveSkill(msg *base.Message) error {
	var req pb3.C2S_50_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}
	data := sys.GetData()
	if utils.SliceContainsUint32(data.SuitIds, req.Id) {
		return neterror.ParamsInvalidError("suitId: %d skill has active", req.Id)
	}

	conf := jsondata.GetDragonModelConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("suitId: %d config not found", req.Id)
	}

	if conf.SkillId > 0 {
		if !sys.GetOwner().LearnSkill(conf.SkillId, 1, true) {
			sys.LogError("LearnSkill failed, skillId: %d", conf.SkillId)
		}
	}

	data.SuitIds = append(data.SuitIds, req.Id)
	sys.SendProto3(50, 11, &pb3.S2C_50_11{Id: req.Id})
	return nil
}

func (sys *DragonSys) Upgrade(amType uint32) bool {
	conf := jsondata.GetDragonAmConf(amType)
	if conf == nil {
		return false
	}
	amData := sys.GetDragonAmData()

	if amData.Stars == nil {
		amData.Stars = make(map[uint32]uint32)
	}

	newStar := amData.Stars[amType] + 1
	if newStar >= uint32(len(conf.DragonStar)) {
		return false
	}

	if conf.DragonStar[newStar] == nil {
		return false
	}

	var logId pb3.LogId
	switch amType {
	case DragonAmType1:
		logId = pb3.LogId_LogDragonAm1Upgrade
	case DragonAmType2:
		logId = pb3.LogId_LogDragonAm2Upgrade
	}

	if !sys.owner.ConsumeByConf(conf.DragonStar[newStar].Consume, false, common.ConsumeParams{LogId: logId}) {
		return false
	}

	amData.Stars[amType] = newStar

	switch amType {
	case DragonAmType1:
		sys.ResetSysAttr(attrdef.SaDragonAm1)
	case DragonAmType2:
		sys.ResetSysAttr(attrdef.SaDragonAm2)
	}

	logworker.LogPlayerBehavior(sys.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(newStar),
	})

	return true
}

func (sys *DragonSys) EquipOnP(itemId uint32) bool {
	if sys.GetData() == nil {
		return false
	}

	eqData := sys.GetDragonEqData()
	if eqData == nil || len(eqData.Equips) == 0 {
		return false
	}

	for _, v := range eqData.Equips {
		if v == itemId {
			return true
		}
	}

	return false
}

func (sys *DragonSys) TakeOnWithItemConfAndWithoutRewardToBag(player iface.IPlayer, itemConf *jsondata.ItemConf, logId uint32) error {
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemConf is nil")
	}

	// 找出装备穿戴slot
	confMap := jsondata.GetDragonEquConf()
	if confMap == nil {
		return neterror.ConfNotFoundError("conf map not found")
	}
	slot := uint32(0)
	for _, conf := range confMap {
		if conf.ReqType == itemConf.SubType {
			slot = conf.Slot
			break
		}
	}

	if slot == 0 {
		return neterror.ParamsInvalidError("item subtype err: %v", itemConf.SubType)
	}

	if sys.GetData() == nil {
		return neterror.InternalError("dragonSys data not init")
	}

	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return neterror.InternalError("dragonSys data not init")
	}

	if eqData.Equips == nil {
		eqData.Equips = make(map[uint32]uint32)
	}

	eqData.Equips[slot] = itemConf.Id

	// 激活时装
	sys.activateEquFashion(itemConf)

	// 重算属性
	player.GetAttrSys().ResetSysAttr(attrdef.SaDragonEqu)

	pbMsg := &pb3.S2C_50_4{
		Slot:   slot,
		ItemId: itemConf.Id,
	}

	fashionMap := sys.GetDragonEqData().Fashions
	if fashionMap != nil && fashionMap[itemConf.SubType] != nil {
		pbMsg.FIds = fashionMap[itemConf.SubType].FIds
	}

	sys.SendProto3(50, 4, pbMsg)

	sys.GetOwner().TriggerQuestEventRange(custom_id.QttTakeOnXTypeYStageZEquipByDragon)

	return nil
}

func (sys *DragonSys) InitExpUpLv(data *pb3.DragonData) {
	sys.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)

	sys.expUpLvs[DragonAmType1] = &uplevelbase.ExpUpLv{
		ExpLv:            data.DragonAmData.ExpLvs[DragonAmType1],
		AttrSysId:        attrdef.SaDragonAm1,
		BehavAddExpLogId: pb3.LogId_LogDragonAm1AddExp,
		AfterUpLvCb:      sys.AfterUpLevelAm1,
		AfterAddExpCb:    sys.AfterAddExpAm1,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetDragonAmLvConf(DragonAmType1, lv); conf != nil {
				return conf
			}
			return nil
		},
	}
	sys.expUpLvs[DragonAmType2] = &uplevelbase.ExpUpLv{
		ExpLv:            data.DragonAmData.ExpLvs[DragonAmType2],
		AttrSysId:        attrdef.SaDragonAm2,
		BehavAddExpLogId: pb3.LogId_LogDragonAm2AddExp,
		AfterUpLvCb:      sys.AfterUpLevelAm2,
		AfterAddExpCb:    sys.AfterAddExpAm2,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetDragonAmLvConf(DragonAmType2, lv); conf != nil {
				return conf
			}
			return nil
		},
	}

	for _, v := range sys.expUpLvs {
		if err := v.Init(sys.GetOwner()); err != nil {
			sys.LogError("DragonSys expUpLvs.Init err: %v", err)
			return
		}
	}
}

func (sys *DragonSys) onUpLv(lv uint32) {
	sys.owner.SetExtraAttr(attrdef.DragonSoulLv, int64(lv))
	sys.ResetSysAttr(attrdef.SaDragonSoul)

	// 学习技能
	lvConf := jsondata.GetDragonSoulConf(lv)
	if lvConf == nil {
		return
	}
	if lvConf.SkillID != 0 {
		if !sys.GetOwner().LearnSkill(lvConf.SkillID, 1, true) {
			sys.LogError("LearnSkill failed, skillId: %d", lvConf.SkillID)
			return
		}
	}
}

func (sys *DragonSys) AfterUpLevelAm1(oldLv uint32) {
}

func (sys *DragonSys) AfterAddExpAm1() {
	sys.SendProto3(50, 7, &pb3.S2C_50_7{
		AmType: DragonAmType1,
		ExpLv:  sys.GetDragonAmData().ExpLvs[DragonAmType1],
	})
}

func (sys *DragonSys) AfterUpLevelAm2(oldLv uint32) {
}

func (sys *DragonSys) AfterAddExpAm2() {
	sys.SendProto3(50, 7, &pb3.S2C_50_7{
		AmType: DragonAmType2,
		ExpLv:  sys.GetDragonAmData().ExpLvs[DragonAmType2],
	})
}

// 属性重算
func (sys *DragonSys) calcDragSoulAttr(calc *attrcalc.FightAttrCalc) {
	lv := sys.GetDragonSoulLv()
	lvConf := jsondata.GetDragonSoulConf(lv)
	if lvConf == nil {
		return
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)
}

func (sys *DragonSys) calcDragonEquAttr(calc *attrcalc.FightAttrCalc) {
	// 装备本身属性
	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return
	}
	var staticAttrs, premiumAttrs []*jsondata.Attr
	for _, itemId := range eqData.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}
		staticAttrs = append(staticAttrs, itemConf.StaticAttrs...)
		premiumAttrs = append(premiumAttrs, itemConf.PremiumAttrs...)
	}
	// 基础属性
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, staticAttrs)
	// 极品属性
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, premiumAttrs)

	// 套装属性/技能
	suitConfL, _ := jsondata.GetDragonEquSuitConf(eqData.Equips)
	if suitConfL == nil {
		return
	}
	var attrs []*jsondata.Attr
	for _, v := range suitConfL {
		attrs = append(attrs, v.Attrs...)
		if v.SkillID != 0 {
			if !sys.GetOwner().LearnSkill(v.SkillID, v.SkillLv, true) {
				continue
			}
		}
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, attrs)
}

func (sys *DragonSys) calcDragonEquAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	// 装备本身属性
	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return
	}
	addRate := totalSysCalc.GetValue(attrdef.DragonEquBaseAttrRate)
	for pos, itemId := range eqData.Equips {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}
		addRateAttrId := KillDragonEquipSuitAttrAddRateMap[pos]
		if addRateAttrId == 0 {
			continue
		}
		addRateByKillDragon := totalSysCalc.GetValue(addRateAttrId)
		rate := uint32(addRate + addRateByKillDragon)
		if rate == 0 {
			continue
		}
		engine.CheckAddAttrsRateRoundingUp(sys.GetOwner(), calc, itemConf.StaticAttrs, rate)
	}
}

func (sys *DragonSys) calcDragonAmAttr(amType uint32, calc *attrcalc.FightAttrCalc) {
	amData := sys.GetDragonAmData()
	if amData == nil {
		return
	}

	// 等级
	if amData.ExpLvs == nil || amData.ExpLvs[amType] == nil {
		return
	}
	lv := amData.ExpLvs[amType].Lv
	lvConf := jsondata.GetDragonAmLvConf(amType, lv)
	if lvConf == nil {
		return
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)

	// 星级
	if amData.Stars == nil {
		return
	}
	star := amData.Stars[amType]
	starConf := jsondata.GetDragonAmStarConf(amType, star)
	if starConf == nil {
		return
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, starConf.Attrs)
	if starConf.SkillId != 0 {
		if !sys.GetOwner().LearnSkill(starConf.SkillId, 1, true) {
			sys.LogError("LearnSkill failed, skillId: %d", starConf.SkillId)
		}
	}

	// 丹药
	var medAttrs jsondata.AttrVec
	for id, medicine := range sys.GetDragonAmData().Medicines {
		mConfMap := jsondata.GetDragonAmMedicineConf(amType)
		if mConfMap == nil {
			continue
		}
		medicineConf := mConfMap[id]
		if medicineConf == nil {
			continue
		}

		itemConf := jsondata.GetItemConfig(id)
		if itemConf == nil {
			continue
		}

		tmpAttrs := itemConf.StaticAttrs.Copy()
		for _, attr := range tmpAttrs {
			attr.Value *= medicine.Count
			medAttrs = append(medAttrs, attr)
		}
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, medAttrs)

	// 计算龙兵/龙武属性加成(只计算等级的)
	var medicineAddRate uint32
	for _, v := range medAttrs {
		if v.Type == attrdef.DragonAm1AddRate ||
			v.Type == attrdef.DragonAm2AddRate {
			medicineAddRate += v.Value
		}
	}
	engine.CheckAddAttrsRate(sys.GetOwner(), calc, lvConf.Attrs, medicineAddRate)
}

// 穿戴装备
func (sys *DragonSys) takeOnEquip(player iface.IPlayer, itemHdl uint64, slot uint32) error {
	eqData := sys.GetDragonEqData()
	if eqData == nil {
		return neterror.ParamsInvalidError("dragonEquData is nil")
	}

	_, ok := eqData.Equips[slot]
	if ok {
		return sys.replaceEquip(player, itemHdl, slot)
	}

	item := player.GetItemByHandle(itemHdl)
	itemId := item.ItemId
	if !player.DeleteItemPtr(item, 1, pb3.LogId_LogDragonEquTakeOn) {
		return neterror.InternalError("item not enough: %d", item.ItemId)
	}

	eqData.Equips[slot] = itemId

	return nil
}

// 卸下装备
func (sys *DragonSys) takeOffEquip(player iface.IPlayer, slot uint32) error {
	eqData := sys.GetDragonEqData()
	itemId := eqData.Equips[slot]
	itemConf := jsondata.GetItemConfig(itemId)

	if itemConf == nil {
		return neterror.InternalError("itemId=%d not found ", itemId)
	}

	itemReward := &jsondata.StdReward{
		Id:    itemId,
		Count: 1,
	}
	param := common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogDragonEquTakeOff,
		NoTips: true,
	}

	if !engine.GiveRewards(player, []*jsondata.StdReward{itemReward}, param) {
		return neterror.InternalError("give reward failed")
	}

	delete(eqData.Equips, slot)

	return nil
}

// 替换装备
func (sys *DragonSys) replaceEquip(player iface.IPlayer, itemHdl uint64, slot uint32) error {
	item := player.GetItemByHandle(itemHdl)
	itemConf := jsondata.GetItemConfig(item.ItemId)
	if itemConf == nil {
		return neterror.InternalError("itemId=%d not found ", item.ItemId)
	}
	eqData := sys.GetDragonEqData()

	oldItemId := eqData.Equips[slot]
	eqData.Equips[slot] = itemConf.Id
	if !player.DeleteItemPtr(item, 1, pb3.LogId_LogDragonEquTakeOn) {
		return neterror.InternalError("item not enough")
	}
	oldItemConf := jsondata.GetItemConfig(oldItemId)
	if oldItemConf == nil {
		return neterror.InternalError("itemId=%d config not found", oldItemId)
	}

	reward := &jsondata.StdReward{
		Id:    oldItemId,
		Count: 1,
	}

	logParam := common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogDragonEquTakeOn,
		NoTips: true,
	}

	if !engine.GiveRewards(player, []*jsondata.StdReward{reward}, logParam) {
		return neterror.InternalError("give reward failed")
	}

	return nil
}

func (sys *DragonSys) activateEquFashion(itemConf *jsondata.ItemConf) {
	if itemConf == nil {
		return
	}

	eqData := sys.GetDragonEqData()

	_, ok := eqData.Fashions[itemConf.SubType]
	if !ok {
		eqData.Fashions[itemConf.SubType] = &pb3.DragonEqFashion{
			FIds: make([]uint32, 0),
		}
	}
	fData := eqData.Fashions[itemConf.SubType]

	if itemConf.CommonField == 0 {
		return
	}
	fData.FIds = append(fData.FIds, itemConf.CommonField)
}

func (sys *DragonSys) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	useConf := jsondata.GetUseItemConfById(conf.ItemId)
	if useConf == nil || len(useConf.Param) <= 0 {
		return false, false, 0
	}

	amType := useConf.Param[0]
	mConf := jsondata.GetDragonAmConf(amType).Medicine[conf.ItemId]
	if mConf == nil {
		return false, false, 0
	}

	amData := sys.GetDragonAmData()
	if amData == nil {
		return false, false, 0
	}

	medicine, ok := amData.Medicines[conf.ItemId]
	if !ok {
		medicine = &pb3.UseCounter{
			Id: conf.ItemId,
		}
		amData.Medicines[conf.ItemId] = medicine
	}

	var limitConf *jsondata.DragonMedUseLimit
	for _, v := range mConf.UseLimit {
		if amData.Stars[amType] <= v.StarLimit {
			limitConf = v
			break
		}
	}

	if limitConf == nil && len(mConf.UseLimit) > 0 {
		limitConf = mConf.UseLimit[len(mConf.UseLimit)-1]
	}

	if limitConf == nil {
		return false, false, 0
	}

	if medicine.Count+uint32(param.Count) > limitConf.Limit {
		return false, false, 0
	}

	medicine.Count += uint32(param.Count)

	// 重算属性
	switch amType {
	case DragonAmType1:
		sys.ResetSysAttr(attrdef.SaDragonAm1)
	case DragonAmType2:
		sys.ResetSysAttr(attrdef.SaDragonAm2)
	}

	// 推送
	sys.SendProto3(50, 9, &pb3.S2C_50_9{
		Medicines: amData.Medicines,
	})

	return true, true, param.Count
}

func calcDragonSoulAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.calcDragSoulAttr(calc)
}

func calcDragonEquAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDragonEquAttr(calc)
}
func calcDragonEquAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDragonEquAttrAddRate(totalSysCalc, calc)
}

func calcDragonAm1Attr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDragonAmAttr(DragonAmType1, calc)
}

func calcDragonAm2Attr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDragonAmAttr(DragonAmType2, calc)
}

func useMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	dragonSys, ok := player.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok || !dragonSys.IsOpen() {
		return false, false, 0
	}
	return dragonSys.useMedicine(param, conf)
}

func getDragonEquAppearPos(equPos uint32) uint32 {
	switch equPos {
	case DEquSpirit:
		return appeardef.AppearPos_DragonSpirit
	case DEquHorn:
		return appeardef.AppearPos_Horn
	case DEquWings: // 翅膀是互斥的 直接用之前的pos
		return appeardef.AppearPos_Wing
	case DEquTail:
		return appeardef.AppearPos_Tail
	default:
		return 0
	}
}

func init() {
	RegisterSysClass(sysdef.SiDragon, func() iface.ISystem {
		return &DragonSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddDragonMedicine, useMedicine)

	net.RegisterSysProto(50, 2, sysdef.SiDragon, (*DragonSys).c2sUpLv)
	net.RegisterSysProto(50, 3, sysdef.SiDragon, (*DragonSys).c2sChangeSoulAppear)
	net.RegisterSysProto(50, 4, sysdef.SiDragon, (*DragonSys).c2sTakeEquipOn)
	net.RegisterSysProto(50, 5, sysdef.SiDragon, (*DragonSys).c2sTakeEquipOff)
	net.RegisterSysProto(50, 6, sysdef.SiDragon, (*DragonSys).c2sChangeEquAppear)
	net.RegisterSysProto(50, 7, sysdef.SiDragon, (*DragonSys).c2sAddExp)
	net.RegisterSysProto(50, 8, sysdef.SiDragon, (*DragonSys).c2sUpgrade)
	net.RegisterSysProto(50, 10, sysdef.SiDragon, (*DragonSys).c2sChangeAmAppear)
	net.RegisterSysProto(50, 11, sysdef.SiDragon, (*DragonSys).c2sActiveSkill)

	engine.RegAttrCalcFn(attrdef.SaDragonSoul, calcDragonSoulAttr)
	engine.RegAttrCalcFn(attrdef.SaDragonEqu, calcDragonEquAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaDragonEqu, calcDragonEquAttrAddRate)
	engine.RegAttrCalcFn(attrdef.SaDragonAm1, calcDragonAm1Attr)
	engine.RegAttrCalcFn(attrdef.SaDragonAm2, calcDragonAm2Attr)
	engine.RegQuestTargetProgress(custom_id.QttTakeOnXTypeYStageZEquipByDragon, handleQttTakeOnXTypeYStageZEquipByDragon)

	initDragonSysGm()
}

func handleQttTakeOnXTypeYStageZEquipByDragon(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) != 1 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiDragon).(*DragonSys)
	if !ok {
		return 0
	}

	nedStage := ids[0]
	var count uint32
	data := sys.GetData()
	if data == nil {
		return 0
	}

	eqData := data.GetDragonEqData()
	if eqData == nil {
		return 0
	}

	if eqData.Equips == nil {
		return 0
	}

	for _, itemId := range eqData.Equips {
		if jsondata.GetItemConfig(itemId).Stage >= nedStage {
			count++
		}
	}
	return count
}

func initDragonSysGm() {
	gmevent.Register("dragon.c2sUpLv", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 2)
		err := msg.PackPb3Msg(&pb3.C2S_50_2{})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 2, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sChangeSoulAppear", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 3)
		err := msg.PackPb3Msg(&pb3.C2S_50_3{Lv: utils.AtoUint32(args[0])})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 3, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sTakeEquipOn", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 4)
		err := msg.PackPb3Msg(&pb3.C2S_50_4{
			Slot:        utils.AtoUint32(args[0]),
			EquipHandle: 0,
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 4, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sTakeEquipOff", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 5)
		err := msg.PackPb3Msg(&pb3.C2S_50_5{
			Slot: utils.AtoUint32(args[0]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 5, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sChangeEquAppear", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 6)
		err := msg.PackPb3Msg(&pb3.C2S_50_6{
			Slot:      utils.AtoUint32(args[0]),
			FashionId: utils.AtoUint32(args[1]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 6, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sAddExp", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 7)
		err := msg.PackPb3Msg(&pb3.C2S_50_7{
			AmType: utils.AtoUint32(args[0]),
			ItemMap: []*pb3.Key64Value{
				{Key: 0, Value: 0},
			},
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 7, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sUpgrade", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 8)
		err := msg.PackPb3Msg(&pb3.C2S_50_8{
			AmType: utils.AtoUint32(args[0]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 8, msg)
		return true
	}, 1)
	gmevent.Register("dragon.c2sChangeAmAppear", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(50<<8 | 10)
		err := msg.PackPb3Msg(&pb3.C2S_50_10{
			AmType: utils.AtoUint32(args[0]),
			Star:   utils.AtoUint32(args[1]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(50, 10, msg)
		return true
	}, 1)
}
