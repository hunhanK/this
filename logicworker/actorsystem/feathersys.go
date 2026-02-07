/**
 * @Author: lzp
 * @Date: 2025/7/21
 * @Desc:
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
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const DefaultFeatherId = 1

const (
	FeatherEquUpdateTypeLvUp    = 1
	FeatherEquUpdateTypeStageUp = 2
	FeatherEquUpdateInherit     = 3
	FeatherEquUpdateTypeBack    = 4
	FeatherEquUpdateTypeTakeOn  = 5
	FeatherEquUpdateTypeTakeOff = 6
	FeatherEquUpdateTypeCompose = 7
)

type FeatherSys struct {
	Base
	expUpLvs map[uint32]*uplevelbase.ExpUpLv
}

func (s *FeatherSys) OnReconnect() {
	s.updateFeatherAppear()
	s.s2cInfo()
}

func (s *FeatherSys) OnLogin() {
	s.initExpUpLv()
	s.updateFeatherAppear()
	s.s2cInfo()
}

func (s *FeatherSys) OnOpen() {
	s.unlockFeather(DefaultFeatherId)

	data := s.getFeatherData(DefaultFeatherId)
	data.IsValid = true

	s.s2cInfo()
}

func (s *FeatherSys) GetData() map[uint32]*pb3.FeatherData {
	binData := s.GetBinaryData()
	if binData.FeatherData == nil {
		binData.FeatherData = map[uint32]*pb3.FeatherData{}
	}
	return binData.FeatherData
}

func (s *FeatherSys) getFeatherData(id uint32) *pb3.FeatherData {
	data := s.GetData()
	fData, ok := data[id]
	if !ok {
		return nil
	}
	if fData.Equips == nil {
		fData.Equips = make(map[uint32]*pb3.ItemSt)
	}
	if fData.Gens == nil {
		fData.Gens = make(map[uint32]*pb3.ItemSt)
	}
	if fData.NewGens == nil {
		fData.NewGens = make(map[uint32]uint32)
	}
	return fData
}

func (s *FeatherSys) getFeatherEquByPos(id, pos uint32) *pb3.ItemSt {
	data := s.getFeatherData(id)
	if data == nil {
		return nil
	}

	return data.Equips[pos]
}

func (s *FeatherSys) getFeatherGenByPos(id, pos uint32) uint32 {
	data := s.getFeatherData(id)
	if data == nil {
		return 0
	}
	return data.NewGens[pos]
}

func (s *FeatherSys) getFeatherEquStar(id uint32) uint32 {
	data := s.getFeatherData(id)
	if data == nil {
		return 0
	}

	if data.Equips == nil {
		return 0
	}

	var star uint32
	for _, equip := range data.Equips {
		itemConf := jsondata.GetItemConfig(equip.GetItemId())
		if itemConf == nil {
			continue
		}
		star += itemConf.Star
	}
	return star
}

func (s *FeatherSys) updateFeatherAppear() {
	appearId := uint32(0)
	for id, fData := range s.GetData() {
		if fData.IsAppear {
			appearId = id
			break
		}
	}
	s.owner.SetExtraAttr(attrdef.FeatherAppearId, attrdef.AttrValueAlias(appearId))
}

func (s *FeatherSys) unlockFeather(id uint32) {
	data := s.GetData()
	_, ok := data[id]
	if !ok {
		data[id] = &pb3.FeatherData{Id: id, Stage: 1}
	}

	fData := data[id]
	if fData.ExpLv == nil {
		fData.ExpLv = &pb3.ExpLvSt{Lv: 1}
	}
	if fData.Equips == nil {
		fData.Equips = make(map[uint32]*pb3.ItemSt)
	}
	if fData.Gens == nil {
		fData.Gens = make(map[uint32]*pb3.ItemSt)
	}
	if fData.NewGens == nil {
		fData.NewGens = make(map[uint32]uint32)
	}
	s.initExpUpLvById(id)
}

func (s *FeatherSys) unlockGen(id, pos uint32) {
	data := s.getFeatherData(id)
	if data == nil {
		return
	}

	_, ok := data.NewGens[pos]
	if !ok {
		data.NewGens[pos] = 0
	}
}

func (s *FeatherSys) initExpUpLv() {
	data := s.GetData()
	s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)

	for id := range data {
		s.initExpUpLvById(id)
	}
}

func (s *FeatherSys) initExpUpLvById(id uint32) {
	fData := s.getFeatherData(id)
	if fData == nil {
		return
	}

	if s.expUpLvs == nil {
		s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)
	}

	s.expUpLvs[id] = &uplevelbase.ExpUpLv{
		ExpLv:            fData.ExpLv,
		AttrSysId:        attrdef.SaFeather,
		BehavAddExpLogId: pb3.LogId_LogFeatherUpLv,
		AfterAddExpCb:    s.afterAddExp,
		AfterUpLvCb:      s.afterLvUp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetFeatherLvConf(id, lv); conf != nil {
				return conf.ExpLvConf
			}
			return nil
		},
	}
}

func (s *FeatherSys) afterAddExp() {
}

func (s *FeatherSys) afterLvUp(_ uint32) {
}

func (s *FeatherSys) s2cInfo() {
	s.SendProto3(81, 1, &pb3.S2C_81_1{Data: s.GetData()})
}

func (s *FeatherSys) s2cPos(id, pos, uType uint32) {
	s.SendProto3(81, 20, &pb3.S2C_81_20{
		Id:    id,
		Pos:   pos,
		Equip: s.getFeatherEquByPos(id, pos),
		Type:  uType,
	})
}

func (s *FeatherSys) s2cGen(id, pos uint32) {
	s.SendProto3(81, 27, &pb3.S2C_81_27{
		Id:     id,
		Pos:    pos,
		ItemId: s.getFeatherGenByPos(id, pos),
	})
}

func (s *FeatherSys) packBackMaterial(equip *pb3.ItemSt) jsondata.StdRewardVec {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return nil
	}

	var backRewards []jsondata.StdRewardVec
	strongConf := jsondata.GetFeatherEquLvConf(itemConf.SubType, equip.GetUnion1())
	if strongConf != nil {
		backRewards = append(backRewards, strongConf.Rewards)
	}

	stageConf := jsondata.GetFeatherEquStageConf(itemConf.SubType, equip.GetUnion2())
	if stageConf != nil {
		backRewards = append(backRewards, stageConf.Rewards)
	}

	rewards := jsondata.MergeStdReward(backRewards...)
	return rewards
}

func (s *FeatherSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	attrs1 := s.calcFeatherAttr()
	attrs2 := s.calcFeatherEquAttr()
	attrs3 := s.calcFeatherGenAttr()

	var attrs jsondata.AttrVec
	attrs = append(attrs, attrs1...)
	attrs = append(attrs, attrs2...)
	attrs = append(attrs, attrs3...)

	engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
}

func (s *FeatherSys) calcFeatherAttr() jsondata.AttrVec {
	data := s.GetData()
	var sumAttrs jsondata.AttrVec

	for id := range data {
		if !s.isFeatherUnLock(id) {
			continue
		}
		attrs := s.getFeatherAttrsById(id)
		if attrs == nil {
			continue
		}
		sumAttrs = append(sumAttrs, attrs...)
	}

	return sumAttrs
}

func (s *FeatherSys) calcFeatherEquAttr() jsondata.AttrVec {
	data := s.GetData()
	var sumAttrs jsondata.AttrVec

	for id := range data {
		if !s.isFeatherUnLock(id) {
			continue
		}
		attrs := s.getFeatherEquAttrsById(id)
		if attrs == nil {
			continue
		}
		sumAttrs = append(sumAttrs, attrs...)

	}
	return sumAttrs
}

func (s *FeatherSys) calcFeatherGenAttr() jsondata.AttrVec {
	data := s.GetData()
	var sumAttrs jsondata.AttrVec

	for id := range data {
		if !s.isFeatherUnLock(id) {
			continue
		}
		attrs := s.getFeatherGenAttrsById(id)
		if attrs == nil {
			continue
		}
		sumAttrs = append(sumAttrs, attrs...)
	}
	return sumAttrs
}

func (s *FeatherSys) calcAttrAddRate(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	featherCalc := new(attrcalc.FightAttrCalc)
	for id := range data {
		featherAttrs := s.getFeatherAttrsById(id)
		equAttrs := s.getFeatherEquAttrsById(id)
		genAttrs := s.getFeatherGenAttrsById(id)
		engine.CheckAddAttrsToCalc(s.owner, featherCalc, featherAttrs)
		engine.CheckAddAttrsToCalc(s.owner, featherCalc, equAttrs)
		engine.CheckAddAttrsToCalc(s.owner, featherCalc, genAttrs)

		// 计算好所有加成比
		s.calcFeatherCoreGenAttrAddRate(featherCalc)

		s.calcFeatherGenAttrAddRate(id, featherCalc, calc)
		s.calcFeatherAttrAddRate(id, featherCalc, calc)
		s.calcFeatherEquAttrAddRate(id, featherCalc, calc)

		featherCalc.Reset()
	}
}

// 核心血玉的全加成
func (s *FeatherSys) calcFeatherCoreGenAttrAddRate(featherCalc *attrcalc.FightAttrCalc) {
	addRate := featherCalc.GetValue(attrdef.FeatherGenAttrRate)
	var attrs jsondata.AttrVec
	if addRate > 0 {
		value1 := featherCalc.GetValue(attrdef.FeatherAttrRate)
		if value1 > 0 {
			attrs = append(attrs, &jsondata.Attr{Type: attrdef.FeatherAttrRate, Value: uint32(value1)})
		}
		value2 := featherCalc.GetValue(attrdef.FeatherEquBaseAttrRate)
		if value2 > 0 {
			attrs = append(attrs, &jsondata.Attr{Type: attrdef.FeatherEquBaseAttrRate, Value: uint32(value2)})
		}
		value3 := featherCalc.GetValue(attrdef.FeatherEquStrengthAttrRate)
		if value3 > 0 {
			attrs = append(attrs, &jsondata.Attr{Type: attrdef.FeatherEquStrengthAttrRate, Value: uint32(value3)})
		}
		value4 := featherCalc.GetValue(attrdef.FeatherEquPremiumAttrRate)
		if value4 > 0 {
			attrs = append(attrs, &jsondata.Attr{Type: attrdef.FeatherEquPremiumAttrRate, Value: uint32(value4)})
		}
		if len(attrs) > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, featherCalc, attrs, uint32(addRate))
		}
	}
}

func (s *FeatherSys) calcFeatherGenAttrAddRate(id uint32, featherCalc, calc *attrcalc.FightAttrCalc) {
	fData := s.getFeatherData(id)
	if fData != nil {
		baseAddRate := featherCalc.GetValue(attrdef.FeatherGenBaseAttrRate)

		var baseAttrs jsondata.AttrVec
		for _, genItemId := range fData.NewGens {
			if genItemId == 0 {
				continue
			}
			itemConf := jsondata.GetItemConfig(genItemId)
			if itemConf != nil {
				baseAttrs = append(baseAttrs, itemConf.StaticAttrs...)
			}
		}
		if baseAddRate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, baseAttrs, uint32(baseAddRate))
		}
	}
}

func (s *FeatherSys) calcFeatherAttrAddRate(id uint32, featherCalc, calc *attrcalc.FightAttrCalc) {
	fData := s.getFeatherData(id)
	if fData != nil {
		var attrs jsondata.AttrVec

		lv := fData.ExpLv.Lv
		lvConf := jsondata.GetFeatherLvConf(id, lv)
		if lvConf != nil {
			attrs = append(attrs, lvConf.Attrs...)
		}

		stage := fData.Stage
		stageConf := jsondata.GetFeatherStageConf(id, stage)
		if stageConf != nil {
			attrs = append(attrs, stageConf.Attrs...)
		}

		addRate := featherCalc.GetValue(attrdef.FeatherAttrRate)
		if addRate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, attrs, uint32(addRate))
		}
	}
}

func (s *FeatherSys) calcFeatherEquAttrAddRate(id uint32, featherCalc, calc *attrcalc.FightAttrCalc) {
	fData := s.getFeatherData(id)
	if fData != nil {
		baseAddRate := featherCalc.GetValue(attrdef.FeatherEquBaseAttrRate)
		strengthAddRate := featherCalc.GetValue(attrdef.FeatherEquStrengthAttrRate)
		premiumAddRate := featherCalc.GetValue(attrdef.FeatherEquPremiumAttrRate)

		var baseAttrs, strengthAttrs, premiumAttrs jsondata.AttrVec
		for pos, equip := range fData.Equips {
			if equip == nil {
				continue
			}
			itemConf := jsondata.GetItemConfig(equip.ItemId)
			if itemConf != nil {
				baseAttrs = append(baseAttrs, itemConf.StaticAttrs...)
				premiumAttrs = append(premiumAttrs, itemConf.PremiumAttrs...)
			}

			lv := equip.Union1
			lvConf := jsondata.GetFeatherEquLvConf(pos, lv)
			if lvConf != nil {
				strengthAttrs = append(strengthAttrs, lvConf.Attrs...)
			}
		}

		if baseAddRate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, baseAttrs, uint32(baseAddRate))
		}
		if strengthAddRate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, strengthAttrs, uint32(strengthAddRate))
		}
		if premiumAddRate > 0 {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, premiumAttrs, uint32(premiumAddRate))
		}
	}
}

func (s *FeatherSys) getFeatherAttrsById(id uint32) jsondata.AttrVec {
	data := s.getFeatherData(id)
	if data == nil {
		return nil
	}

	var attrs jsondata.AttrVec

	lv := data.ExpLv.Lv
	lvConf := jsondata.GetFeatherLvConf(id, lv)
	if lvConf != nil {
		attrs = append(attrs, lvConf.Attrs...)
	}

	stage := data.Stage
	stageConf := jsondata.GetFeatherStageConf(id, stage)
	if stageConf != nil {
		attrs = append(attrs, stageConf.Attrs...)
	}

	if data.IsAwaken {
		star := data.Star
		starConf := jsondata.GetFeatherStarConf(id, star)
		if starConf != nil {
			attrs = append(attrs, starConf.Attrs...)
		}
	}
	return attrs
}

func (s *FeatherSys) getFeatherEquAttrsById(id uint32) jsondata.AttrVec {
	fData := s.getFeatherData(id)
	if fData == nil {
		return nil
	}

	var attrs jsondata.AttrVec
	for pos, equip := range fData.Equips {
		if equip == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if itemConf == nil {
			continue
		}
		attrs = append(attrs, itemConf.StaticAttrs...)
		attrs = append(attrs, itemConf.PremiumAttrs...)
		attrs = append(attrs, itemConf.SuperAttrs...)

		lv := equip.Union1
		lvConf := jsondata.GetFeatherEquLvConf(pos, lv)
		if lvConf != nil {
			attrs = append(attrs, lvConf.Attrs...)
		}

		stage := equip.Union2
		stageConf := jsondata.GetFeatherEquStageConf(pos, stage)
		if stageConf != nil {
			if stageConf.Rate > 0 {
				for _, line := range itemConf.StaticAttrs {
					value := utils.CalcMillionRate64(int64(line.Value), int64(stageConf.Rate))
					attrs = append(attrs, &jsondata.Attr{
						Type:  line.Type,
						Job:   line.Job,
						Value: uint32(value),
					})
				}
			}
			if stageConf.PremiumRate > 0 {
				for _, line := range itemConf.PremiumAttrs {
					value := utils.CalcMillionRate64(int64(line.Value), int64(stageConf.PremiumRate))
					attrs = append(attrs, &jsondata.Attr{
						Type:  line.Type,
						Job:   line.Job,
						Value: uint32(value),
					})
				}
			}
		}
	}
	return attrs
}

func (s *FeatherSys) getFeatherGenAttrsById(id uint32) jsondata.AttrVec {
	fData := s.getFeatherData(id)
	if fData == nil {
		return nil
	}

	var attrs jsondata.AttrVec
	for pos, genItemId := range fData.NewGens {
		if genItemId == 0 {
			continue
		}
		itemConf := jsondata.GetItemConfig(genItemId)
		if itemConf == nil {
			continue
		}
		attrs = append(attrs, itemConf.StaticAttrs...)
		attrs = append(attrs, itemConf.PremiumAttrs...)
		attrs = append(attrs, itemConf.SuperAttrs...)

		gConf := jsondata.GetFeatherGenPosConf(pos)
		if gConf != nil {
			if v, ok := gConf.ExtraAttrs[itemConf.Level]; ok {
				attrs = append(attrs, v.Attrs...)
			}
		}
	}
	return attrs
}

func (s *FeatherSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_81_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id, itemMap := req.GetId(), req.GetItemMap()
	if itemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	data := s.getFeatherData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	conf := jsondata.GetFeatherConf(id)
	if conf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	if !s.isFeatherUnLock(id) {
		return neterror.ParamsInvalidError("id:%d feather lock", id)
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !utils.SliceContainsUint32(conf.LevelUpItem, item.ItemId) {
			return neterror.ParamsInvalidError("item:%d not in LevelUpItem", item.ItemId)
		}
		if item.Count < int64(entry.Value) {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	addExp := uint64(0)
	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		addExp += uint64(itemConf.CommonField * entry.Value)
	}

	nextLv := data.ExpLv.Lv + 1
	lvConf := jsondata.GetFeatherLvConf(id, nextLv)
	if lvConf != nil && data.ExpLv.Exp+addExp >= lvConf.RequiredExp {
		// 检查阶级是否满足
		if data.Stage < lvConf.StageLimit {
			return neterror.ParamsInvalidError("id:%d stage limit", req.Id)
		}
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		s.owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogFeatherUpLv)
	}

	err := s.expUpLvs[id].AddExp(s.GetOwner(), addExp)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaFeather)
	s.SendProto3(81, 2, &pb3.S2C_81_2{Id: id, ExpLv: s.getFeatherData(id).ExpLv})
	return nil
}

func (s *FeatherSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_81_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	data := s.getFeatherData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", req.Id)
	}

	if !s.isFeatherUnLock(req.Id) {
		return neterror.ParamsInvalidError("id:%d feather lock", req.Id)
	}

	nextStage := data.Stage + 1
	sConf := jsondata.GetFeatherStageConf(req.Id, nextStage)
	if sConf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	if data.ExpLv.Lv < sConf.LvLimit {
		return neterror.ParamsInvalidError("id:%d lv limit", req.Id)
	}

	if !s.owner.ConsumeByConf(sConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherUpStage}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data.Stage = nextStage
	s.ResetSysAttr(attrdef.SaFeather)
	s.SendProto3(81, 3, &pb3.S2C_81_3{Id: req.Id, Stage: nextStage})
	return nil
}

func (s *FeatherSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_81_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	data := s.getFeatherData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("feather id:%d lock", req.Id)
	}

	if !s.isFeatherUnLock(req.Id) {
		return neterror.ParamsInvalidError("id:%d feather lock", req.Id)
	}

	if !data.IsAwaken {
		return neterror.ParamsInvalidError("feather id:%d not awaken", req.Id)
	}

	nextStar := data.Star + 1
	sConf := jsondata.GetFeatherStarConf(req.Id, nextStar)
	if sConf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	boolConsume1 := s.owner.CheckConsumeByConf(sConf.Consume1, false, pb3.LogId_LogFeatherUpStar)
	boolConsume2 := s.owner.CheckConsumeByConf(sConf.Consume2, false, pb3.LogId_LogFeatherUpStar)
	if !boolConsume1 && !boolConsume2 {
		return neterror.ParamsInvalidError("consume error")
	}

	// 优先消耗consume1
	if boolConsume1 {
		s.owner.ConsumeByConf(sConf.Consume1, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherUpStar})
	} else {
		s.owner.ConsumeByConf(sConf.Consume2, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherUpStar})
	}

	data.Star = nextStar
	s.ResetSysAttr(attrdef.SaFeather)
	s.SendProto3(81, 4, &pb3.S2C_81_4{Id: req.Id, Star: nextStar})
	return nil
}

func (s *FeatherSys) c2sAwaken(msg *base.Message) error {
	var req pb3.C2S_81_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	conf := jsondata.GetFeatherConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("feather id:%d config not found", req.Id)
	}

	data := s.getFeatherData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("feather id:%d lock", req.Id)
	}

	if conf.AwakenNeedStage > 0 {
		if conf.AwakenNeedStage > data.Stage {
			return neterror.ParamsInvalidError("feather id:%d stage limit", req.Id)
		}
	} else {
		starConf := jsondata.GetFeatherStarConf(req.Id, 0)
		if starConf == nil {
			return neterror.ConfNotFoundError("feather id:%d awake consume not found", req.Id)
		}

		boolConsume1 := s.owner.CheckConsumeByConf(starConf.Consume1, false, pb3.LogId_LogFeatherAwake)
		boolConsume2 := s.owner.CheckConsumeByConf(starConf.Consume2, false, pb3.LogId_LogFeatherAwake)
		if !boolConsume1 && !boolConsume2 {
			return neterror.ParamsInvalidError("consume error")
		}

		// 优先消耗consume1
		if boolConsume1 {
			s.owner.ConsumeByConf(starConf.Consume1, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherUpStar})
		} else {
			s.owner.ConsumeByConf(starConf.Consume2, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherUpStar})
		}
	}

	data.IsAwaken = true
	s.ResetSysAttr(attrdef.SaFeather)
	s.SendProto3(81, 5, &pb3.S2C_81_5{Id: req.Id, IsAwaken: true})
	return nil
}

func (s *FeatherSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_81_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	conf := jsondata.GetFeatherConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("feather id:%d config not found", req.Id)
	}

	data := s.getFeatherData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("feather id:%d lock", req.Id)
	}

	if !data.IsAwaken {
		return neterror.ParamsInvalidError("feather id:%d not awaken", req.Id)
	}

	for id, fData := range s.GetData() {
		if id == req.Id {
			fData.IsAppear = true
		} else {
			fData.IsAppear = false
		}
	}

	s.owner.SetExtraAttr(attrdef.FeatherAppearId, attrdef.AttrValueAlias(req.Id))
	return nil
}

func (s *FeatherSys) c2sUnDress(msg *base.Message) error {
	var req pb3.C2S_81_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	conf := jsondata.GetFeatherConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("feather id:%d config not found", req.Id)
	}

	data := s.getFeatherData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("feather id:%d lock", req.Id)
	}

	if !data.IsAppear {
		return neterror.ParamsInvalidError("feather id:%d not dress", req.Id)
	}

	data.IsAppear = false
	s.owner.SetExtraAttr(attrdef.FeatherAppearId, 0)
	return nil
}

func (s *FeatherSys) c2sEquUpLv(msg *base.Message) error {
	var req pb3.C2S_81_15
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id, pos := req.GetId(), req.GetPos()

	equip := s.getFeatherEquByPos(id, pos)
	if equip == nil {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip", id, pos)
	}

	conf := jsondata.GetFeatherEquConf()
	if conf == nil {
		return neterror.ConfNotFoundError("id:%d config not found", id)
	}

	if jsondata.GetItemQuality(equip.GetItemId()) < conf.StrongNeedQuality {
		return neterror.ParamsInvalidError("strong quality not meet")
	}

	nextLv := equip.Union1 + 1
	nextLvConf := jsondata.GetFeatherEquLvConf(pos, nextLv)
	if nil == nextLvConf {
		return neterror.ConfNotFoundError("strong lv conf %d is nil", nextLv)
	}

	if equip.Union2 < nextLvConf.StageLimit {
		return neterror.ParamsInvalidError("stage quality not meet")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherEquLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	equip.Union1 = nextLv
	s.ResetSysAttr(attrdef.SaFeather)

	s.s2cPos(id, pos, FeatherEquUpdateTypeLvUp)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFeatherEquLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"level":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *FeatherSys) c2sEquUpStage(msg *base.Message) error {
	var req pb3.C2S_81_16
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id, pos := req.GetId(), req.GetPos()

	equip := s.getFeatherEquByPos(id, pos)
	if equip == nil {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip", id, pos)
	}

	conf := jsondata.GetFeatherEquConf()
	if conf == nil {
		return neterror.ConfNotFoundError("id:%d config not found", id)
	}

	if jsondata.GetItemQuality(equip.GetItemId()) < conf.StageNeedQuality {
		return neterror.ParamsInvalidError("stage quality not meet")
	}

	nextStage := equip.Union2 + 1
	nextStageConf := jsondata.GetFeatherEquStageConf(pos, nextStage)
	if nil == nextStageConf {
		return neterror.ConfNotFoundError("stage lv conf %d is nil", nextStage)
	}

	if equip.Union1 < nextStageConf.LvLimit {
		return neterror.ParamsInvalidError("lv quality not meet")
	}

	if !s.owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFeatherEquLvStageUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	equip.Union2 = nextStage
	s.s2cPos(id, pos, FeatherEquUpdateTypeStageUp)
	s.ResetSysAttr(attrdef.SaFeather)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFeatherEquLvStageUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": equip.GetItemId(),
			"handle": equip.GetHandle(),
			"stage":  equip.GetUnion1(),
		}),
	})
	return nil
}

func (s *FeatherSys) c2sEquInherit(msg *base.Message) error {
	var req pb3.C2S_81_17
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	sendHdl, revHdl := req.GetSendHandle(), req.GetRevHandle()
	if err := s.checkInherit(sendHdl, revHdl); err != nil {
		return err
	}

	s.inherit(sendHdl, revHdl)
	return nil
}

func (s *FeatherSys) c2sEquTakeOn(msg *base.Message) error {
	var req pb3.C2S_81_18
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if err := s.checkEquTakeOn(req.Id, req.Pos, req.Handle); err != nil {
		return err
	}
	s.takeOnEqu(req.Id, req.Pos, req.Handle)
	return nil
}

func (s *FeatherSys) c2sEquTakeOff(msg *base.Message) error {
	var req pb3.C2S_81_19
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if err := s.checkEquTakeOff(req.Id, req.Pos); err != nil {
		return err
	}
	s.takeOffEqu(req.Id, req.Pos)
	return nil
}

func (s *FeatherSys) c2sBack(msg *base.Message) error {
	var req pb3.C2S_81_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	hdl := req.GetHandle()
	equip := s.getFeatherEquFromBagOrTake(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("equip not found")
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId:%d not found", equip.GetItemId())
	}

	if s.isZeroStatus(equip) {
		return neterror.ParamsInvalidError("cannot back")
	}

	rewards := s.getBackRewards(equip)

	s.resetFeatherEqu(equip)
	s.onFeatherEquChange(equip, FeatherEquUpdateTypeBack)
	if len(rewards) > 0 {
		engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogFeatherEquBack,
		})
	}
	return nil
}

func (s *FeatherSys) c2sGenTakeOn(msg *base.Message) error {
	var req pb3.C2S_81_25
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if err := s.checkGenTakeOn(req.Id, req.Pos, req.ItemId); err != nil {
		return err
	}
	s.takeOnGen(req.Id, req.Pos, req.ItemId)
	return nil
}

func (s *FeatherSys) c2sGenTakeOff(msg *base.Message) error {
	var req pb3.C2S_81_26
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if err := s.checkGenTakeOff(req.Id, req.Pos); err != nil {
		return err
	}
	s.takeOffGen(req.Id, req.Pos)
	return nil
}

func (s *FeatherSys) isFeatherUnLock(id uint32) bool {
	data := s.getFeatherData(id)
	return data.IsValid
}

func (s *FeatherSys) checkLockFeather() {
	keys := jsondata.GetFeatherSortIdList()

	lockIdx := 0
	for idx := range keys {
		if idx == 0 {
			continue
		}
		id := keys[idx]
		preId := id - 1
		conf := jsondata.GetFeatherConf(id)
		if s.getFeatherEquStar(preId) < conf.UnlockStar {
			lockIdx = idx
			break
		}
	}

	if lockIdx == 0 {
		return
	}

	// lockIdx以及后面的羽翼都失效
	for idx := lockIdx; idx < len(keys); idx++ {
		id := keys[idx]
		data := s.getFeatherData(id)
		if data != nil {
			data.IsValid = false
		}
	}
}

func (s *FeatherSys) checkUnlockFeather() {
	keys := jsondata.GetFeatherSortIdList()

	for idx := range keys {
		id := keys[idx]
		fData := s.getFeatherData(id)
		if fData == nil || !fData.IsValid {
			break
		}

		nextId := id + 1
		conf := jsondata.GetFeatherConf(nextId)
		if conf == nil {
			break
		}
		if s.getFeatherEquStar(id) >= conf.UnlockStar {
			nextFData := s.getFeatherData(nextId)
			if nextFData == nil {
				s.unlockFeather(nextId)
			}
			nextFData = s.getFeatherData(nextId)
			nextFData.IsValid = true
		}
	}
}

func (s *FeatherSys) checkUnlockGen(id uint32) {
	conf := jsondata.GetFeatherEquConf()
	if conf == nil {
		return
	}

	data := s.getFeatherData(id)
	if data == nil {
		return
	}

	star := s.getFeatherEquStar(id)
	for pos, posConf := range conf.SlotGen {
		if _, ok := data.NewGens[pos]; ok {
			continue
		}
		if star >= posConf.StarLimit {
			s.unlockGen(id, pos)
		}
	}
}

func (s *FeatherSys) checkEquTakeOn(id, pos uint32, hdl uint64) error {
	conf := jsondata.GetFeatherPosConf(pos)
	if conf == nil {
		return neterror.ConfNotFoundError("equ pos:%d config not found", pos)
	}

	equip := s.getFeatherEqu(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("equip not exist")
	}

	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId:%d config not found", equip.GetItemId())
	}

	if itemConf.SubType != pos {
		return neterror.ParamsInvalidError("id:%d, pos:%d itemId:%d equip limit", id, pos, equip.GetItemId())
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return neterror.ParamsInvalidError("id:%d, pos:%d equip limit", itemConf.Id, itemConf.SubType)
	}

	if itemConf.Type != itemdef.ItemTypeFeatherEqu {
		return neterror.ParamsInvalidError("itemId:%d is not feather equipment", equip.GetItemId())
	}

	data := s.getFeatherData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	return nil
}

func (s *FeatherSys) takeOnEqu(id, pos uint32, hdl uint64) {
	oldEquip := s.getFeatherEquByPos(id, pos)
	if oldEquip != nil {
		s.takeOffEqu(id, pos)
	}

	equip := s.getFeatherEqu(hdl)
	s.deleteFeatherEqu(hdl, pb3.LogId_LogFeatherEquTakeOn)

	data := s.getFeatherData(id)
	data.Equips[pos] = equip
	equip.Pos = pos
	equip.Ext.OwnerId = uint64(id)

	s.afterTakeOnEqu(id)
	s.s2cPos(id, pos, FeatherEquUpdateTypeTakeOn)
	s.s2cInfo()

	if oldEquip != nil {
		if err := s.checkInherit(oldEquip.Handle, equip.Handle); err == nil {
			s.inherit(oldEquip.Handle, equip.Handle)
		}
	}
}

func (s *FeatherSys) afterTakeOnEqu(id uint32) {
	s.checkUnlockFeather()
	s.checkUnlockGen(id)
	s.ResetSysAttr(attrdef.SaFeather)
}

func (s *FeatherSys) checkEquTakeOff(id, pos uint32) error {
	conf := jsondata.GetFeatherPosConf(pos)
	if conf == nil {
		return neterror.ConfNotFoundError("equ pos:%d config not found", pos)
	}

	data := s.getFeatherData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	equip := s.getFeatherEquByPos(id, pos)
	if equip == nil {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip featherEqu", id, pos)
	}

	if !s.checkFeatherEquBag() {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return neterror.ParamsInvalidError("feather equ bag is full")
	}

	return nil
}

func (s *FeatherSys) takeOffEqu(id, pos uint32) {
	equip := s.getFeatherEquByPos(id, pos)
	if equip == nil {
		return
	}

	data := s.getFeatherData(id)
	data.Equips[pos] = nil

	equip.Pos = 0
	equip.Ext.OwnerId = 0

	s.addFeatherEqu(equip)
	s.afterTakeOff()
	s.s2cPos(id, pos, FeatherEquUpdateTypeTakeOff)
}

func (s *FeatherSys) afterTakeOff() {
	s.checkLockFeather()
	s.ResetSysAttr(attrdef.SaFeather)
}

func (s *FeatherSys) checkGenTakeOn(id, pos uint32, itemId uint32) error {
	conf := jsondata.GetFeatherGenPosConf(pos)
	if conf == nil {
		return neterror.ConfNotFoundError("gen pos:%d config not found", pos)
	}

	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return neterror.ConfNotFoundError("itemId:%d config not found", itemId)
	}

	if itemConf.Type != itemdef.ItemTypeFeatherGen {
		return neterror.ParamsInvalidError("itemId:%d is not feather gen", itemId)
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return neterror.ParamsInvalidError("id:%d, pos:%d equip limit", itemId, itemConf.SubType)
	}

	data := s.getFeatherData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	if _, ok := data.NewGens[pos]; !ok {
		return neterror.ParamsInvalidError("pos:%d lock", pos)
	}
	return nil
}

func (s *FeatherSys) takeOnGen(id, pos uint32, itemId uint32) {
	oldGenItemId := s.getFeatherGenByPos(id, pos)
	if oldGenItemId > 0 {
		s.takeOffGen(id, pos)
	}

	if !s.GetOwner().DeleteItemById(itemId, 1, pb3.LogId_LogFeatherGenTakeOn) {
		return
	}

	data := s.getFeatherData(id)
	data.NewGens[pos] = itemId

	s.s2cGen(id, pos)
	s.afterTakeOnGen()
}

func (s *FeatherSys) afterTakeOnGen() {
	s.ResetSysAttr(attrdef.SaFeather)
}

func (s *FeatherSys) checkGenTakeOff(id, pos uint32) error {
	conf := jsondata.GetFeatherGenPosConf(pos)
	if conf == nil {
		return neterror.ConfNotFoundError("gen pos:%d config not found", pos)
	}

	data := s.getFeatherData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	genItemId := s.getFeatherGenByPos(id, pos)
	if genItemId == 0 {
		return neterror.ParamsInvalidError("id:%d, pos:%d not equip featherGen", id, pos)
	}

	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.SysNotExistError("bag sys get err")
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	return nil
}

func (s *FeatherSys) takeOffGen(id, pos uint32) {
	genItemId := s.getFeatherGenByPos(id, pos)
	if genItemId == 0 {
		return
	}

	if !engine.GiveRewards(s.GetOwner(), []*jsondata.StdReward{{
		Id:    genItemId,
		Count: 1,
	}}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFeatherGenTakeOff, NoTips: false}) {
		return
	}

	data := s.getFeatherData(id)
	data.NewGens[pos] = 0

	s.afterTakeOffGen()
	s.s2cGen(id, pos)
}

func (s *FeatherSys) afterTakeOffGen() {
	s.ResetSysAttr(attrdef.SaFeather)
}

func (s *FeatherSys) checkInherit(sendHdl, revHdl uint64) error {
	conf := jsondata.GetFeatherEquConf()
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	sendEquip := s.getFeatherEquFromBagOrTake(sendHdl)
	if nil == sendEquip {
		return neterror.ParamsInvalidError("not found equip")
	}

	revEquip := s.getFeatherEquFromBagOrTake(revHdl)
	if nil == revEquip {
		return neterror.ParamsInvalidError("not found equip")
	}

	if s.isZeroStatus(sendEquip) || !s.isZeroStatus(revEquip) {
		return neterror.ParamsInvalidError("equip cannot inherit")
	}

	if jsondata.GetItemQuality(sendEquip.GetItemId()) < conf.SendNeedQuality {
		return neterror.ParamsInvalidError("sendEquip quality not meet")
	}

	if jsondata.GetItemQuality(revEquip.GetItemId()) < conf.RevNeedQuality {
		return neterror.ParamsInvalidError("revEquip quality not meet")
	}

	return nil
}

func (s *FeatherSys) inherit(sendHdl, revHdl uint64) {
	sendEquip := s.getFeatherEquFromBagOrTake(sendHdl)
	revEquip := s.getFeatherEquFromBagOrTake(revHdl)

	sendStrongLv, sendStage := sendEquip.GetUnion1(), sendEquip.GetUnion2()

	s.resetFeatherEqu(sendEquip)
	revEquip.Union1 = sendStrongLv
	revEquip.Union2 = sendStage

	s.onFeatherEquChange(sendEquip, FeatherEquUpdateInherit)
	s.onFeatherEquChange(revEquip, FeatherEquUpdateInherit)
	s.ResetSysAttr(attrdef.SaFeather)
}

func (s *FeatherSys) onFeatherEquChange(equip *pb3.ItemSt, uType uint32) {
	if equip.Ext.OwnerId > 0 {
		s.s2cPos(uint32(equip.Ext.OwnerId), equip.Pos, uType)
	} else {
		logId := pb3.LogId_LogFeatherEquLvInherit
		if uType == FeatherEquUpdateTypeBack {
			logId = pb3.LogId_LogFeatherEquBack
		}

		bagSys := s.owner.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
		bagSys.OnItemChange(equip, 0, common.EngineGiveRewardParam{
			LogId: logId,
		})
	}
}

func (s *FeatherSys) getBackRewards(equip *pb3.ItemSt) jsondata.StdRewardVec {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return nil
	}

	var backRewards []jsondata.StdRewardVec

	strongConf := jsondata.GetFeatherEquLvConf(itemConf.SubType, equip.GetUnion1())
	if nil == strongConf {
		return nil
	}
	backRewards = append(backRewards, strongConf.Rewards)

	stageConf := jsondata.GetFeatherEquStageConf(itemConf.SubType, equip.GetUnion2())
	if nil == stageConf {
		return nil
	}
	backRewards = append(backRewards, stageConf.Rewards)

	rewards := jsondata.MergeStdReward(backRewards...)
	return rewards
}

func (s *FeatherSys) isZeroStatus(equip *pb3.ItemSt) bool {
	if equip.GetUnion1() > 0 {
		return false
	}
	if equip.GetUnion2() > 1 {
		return false
	}
	return true
}

func (s *FeatherSys) resetFeatherEqu(equip *pb3.ItemSt) {
	equip.Union1 = 0
	equip.Union2 = 1
}

func (s *FeatherSys) getFeatherEqu(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *FeatherSys) getFeatherEquFromBagOrTake(hdl uint64) *pb3.ItemSt {
	equip := s.getFeatherEqu(hdl)
	if equip != nil {
		return equip
	}

	data := s.GetData()
	for _, fData := range data {
		for _, equipSt := range fData.Equips {
			if equipSt.GetHandle() == hdl {
				return equipSt
			}
		}
	}
	return nil
}

func (s *FeatherSys) addFeatherEqu(equip *pb3.ItemSt) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
	if !ok {
		return
	}
	bagSys.AddItemPtr(equip, true, pb3.LogId_LogFeatherEquTakeOff)
}

func (s *FeatherSys) deleteFeatherEqu(hdl uint64, logId pb3.LogId) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
	if !ok {
		return
	}
	bagSys.RemoveItemByHandle(hdl, logId)
}

func (s *FeatherSys) checkFeatherEquBag() bool {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFeatherBag).(*FeatherBagSys)
	if !ok {
		return false
	}
	return bagSys.AvailableCount() > 0
}

func calcFeatherAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFeather).(*FeatherSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttr(calc)
}

func calcFeatherAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFeather).(*FeatherSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcAttrAddRate(calc)
}

func offlineSetFeatherStatus(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	featherId := st.U32Param
	if s, ok := player.GetSysObj(sysdef.SiFeather).(*FeatherSys); ok && s.IsOpen() {
		fData := s.getFeatherData(featherId)
		if fData == nil {
			return
		}
		fData.IsValid = true
		s.s2cInfo()
	}
}

func init() {
	RegisterSysClass(sysdef.SiFeather, func() iface.ISystem {
		return &FeatherSys{}
	})

	net.RegisterSysProto(81, 2, sysdef.SiFeather, (*FeatherSys).c2sUpLv)
	net.RegisterSysProto(81, 3, sysdef.SiFeather, (*FeatherSys).c2sUpStage)
	net.RegisterSysProto(81, 4, sysdef.SiFeather, (*FeatherSys).c2sUpStar)
	net.RegisterSysProto(81, 5, sysdef.SiFeather, (*FeatherSys).c2sAwaken)
	net.RegisterSysProto(81, 6, sysdef.SiFeather, (*FeatherSys).c2sDress)
	net.RegisterSysProto(81, 7, sysdef.SiFeather, (*FeatherSys).c2sUnDress)

	net.RegisterSysProto(81, 15, sysdef.SiFeather, (*FeatherSys).c2sEquUpLv)
	net.RegisterSysProto(81, 16, sysdef.SiFeather, (*FeatherSys).c2sEquUpStage)
	net.RegisterSysProto(81, 17, sysdef.SiFeather, (*FeatherSys).c2sEquInherit)
	net.RegisterSysProto(81, 18, sysdef.SiFeather, (*FeatherSys).c2sEquTakeOn)
	net.RegisterSysProto(81, 19, sysdef.SiFeather, (*FeatherSys).c2sEquTakeOff)
	net.RegisterSysProto(81, 21, sysdef.SiFeather, (*FeatherSys).c2sBack)

	net.RegisterSysProto(81, 25, sysdef.SiFeather, (*FeatherSys).c2sGenTakeOn)
	net.RegisterSysProto(81, 26, sysdef.SiFeather, (*FeatherSys).c2sGenTakeOff)

	engine.RegAttrCalcFn(attrdef.SaFeather, calcFeatherAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaFeather, calcFeatherAttrAddRate)

	engine.RegisterMessage(gshare.OfflineSetFeatherStatus, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineSetFeatherStatus)
}
