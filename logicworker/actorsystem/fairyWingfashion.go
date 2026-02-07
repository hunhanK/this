package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/trialactivetype"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

/**
 * @Author: YangQibin
 * @Desc: 仙时装系统
 * @Date: 2023/3/20
 */

type FairyWingFashionSys struct {
	Base
	data *pb3.FairyWingFashionData
}

func (sys *FairyWingFashionSys) GetFashionQuality(fashionId uint32) uint32 {
	appearConf := jsondata.GetFairyWingFashionConf(fashionId)
	if nil == appearConf {
		return 0
	}
	return jsondata.GetItemQuality(appearConf.RelatedItem)
}

func (sys *FairyWingFashionSys) GetFashionBaseAttr(fashionId uint32) jsondata.AttrVec {
	lv, ok := sys.data.Fashions[fashionId]
	if !ok {
		return nil
	}
	conf := jsondata.GetFairyWingFashionLvConf(fashionId, lv)
	if conf == nil {
		return nil
	}
	return conf.Attrs
}

func (sys *FairyWingFashionSys) OnInit() {
}

func (sys *FairyWingFashionSys) newTrialActiveSt() (*TrialActiveHandler, error) {
	st := &TrialActiveHandler{}
	st.DoActive = func(params *jsondata.TrialActiveParams) error {
		if len(params.Params) < 1 {
			return neterror.ParamsInvalidError("params < 1")
		}

		appearId := params.Params[0]
		if sys.CheckFashionActive(appearId) {
			return neterror.ParamsInvalidError("is active")
		}

		conf := jsondata.GetFairyWingFashionLvConf(appearId, 0)
		if nil == conf {
			return neterror.ParamsInvalidError("fairy wing fashion Lv conf not found: %v", appearId)
		}

		sys.data.Fashions[appearId] = 0
		sys.onActivated(appearId)
		return nil
	}

	st.DoForget = func(params *jsondata.TrialActiveParams) error {
		if len(params.Params) < 1 {
			return neterror.ParamsInvalidError("params < 1")
		}

		appearId := params.Params[0]
		err := sys.delFashion(appearId)
		if err != nil {
			return err
		}
		return nil
	}
	return st, nil
}

func (sys *FairyWingFashionSys) OnOpen() {
	binaryData := sys.GetBinaryData()
	binaryData.FairyWingFashionData = &pb3.FairyWingFashionData{
		Fashions: make(map[uint32]uint32),
	}
	sys.data = binaryData.FairyWingFashionData
	sys.ResetSysAttr(attrdef.SaFairyWingFashion)
}

func (sys *FairyWingFashionSys) OnLogin() {
	binaryData := sys.GetBinaryData()
	sys.data = binaryData.FairyWingFashionData

	if nil == sys.data.Fashions {
		sys.data.Fashions = make(map[uint32]uint32)
	}

	sys.SendProto3(15, 20, &pb3.S2C_15_20{Data: sys.data})
}

func (sys *FairyWingFashionSys) OnReconnect() {
	sys.SendProto3(15, 20, &pb3.S2C_15_20{Data: sys.data})
}

func (sys *FairyWingFashionSys) onUpLv(appearId uint32, Lev uint32) {
	sys.owner.TriggerQuestEvent(custom_id.QttFairyWingFashion, 0, int64(Lev))
	sys.ResetSysAttr(attrdef.SaFairyWingFashion)
	sys.SendProto3(15, 21, &pb3.S2C_15_21{AppearId: appearId, Lv: Lev})

	conf := jsondata.GetFairyWingFashionLvConf(appearId, Lev)
	if conf != nil && conf.SkillId != 0 {
		sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true)
	}
}

func (sys *FairyWingFashionSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_15_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if sys.owner.IsInTrialActive(trialactivetype.ActiveTypeFashion, []uint32{req.AppearId}) {
		return neterror.ParamsInvalidError("is in trial")
	}

	curLv, ok := sys.data.Fashions[req.AppearId]
	if !ok {
		return neterror.ParamsInvalidError("fairy wing fashion not activated: %v", req.AppearId)
	}

	nextLv := curLv + 1

	conf := jsondata.GetFairyWingFashionLvConf(req.AppearId, nextLv)
	if nil == conf {
		return neterror.ParamsInvalidError("fairy wing fashion Lv conf not found: %v", req.AppearId)
	}

	if !sys.GetOwner().ConsumeByConf(conf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogFairyWingFashionUpLv}) {
		return neterror.ParamsInvalidError("fairy wing fashion Lv consume not enough: %v", req.AppearId)
	}

	sys.data.Fashions[req.AppearId] = nextLv
	sys.onUpLv(req.AppearId, nextLv)
	return nil
}

func (sys *FairyWingFashionSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_15_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetFairyWingFashionLvConf(req.AppearId, 0)
	if nil == conf {
		return neterror.ParamsInvalidError("fairy wing fashion Lv conf not found: %v", req.AppearId)
	}

	isTrial := sys.owner.IsInTrialActive(trialactivetype.ActiveTypeWingFashion, []uint32{req.AppearId})
	isActive := sys.CheckFashionActive(req.AppearId)
	needActive := isTrial || !isActive

	if !needActive {
		return neterror.ParamsInvalidError("fairy wing fashion has activated: %v", req.AppearId)
	}

	if !sys.GetOwner().ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyWingFashionUpLv}) {
		return neterror.ParamsInvalidError("fairy wing fashion Lv consume not enough: %v", req.AppearId)
	}

	sys.owner.StopTrialActive(trialactivetype.ActiveTypeWingFashion, []uint32{req.AppearId})

	sys.data.Fashions[req.AppearId] = 0
	sys.onActivated(req.AppearId)
	return nil
}

func (sys *FairyWingFashionSys) delFashion(fashionId uint32) error {
	_, ok := sys.data.Fashions[fashionId]
	if !ok {
		return nil
	}

	delete(sys.data.Fashions, fashionId)

	conf := jsondata.GetFairyWingFashionLvConf(fashionId, 0)
	if conf != nil && conf.SkillId != 0 {
		sys.GetOwner().ForgetSkill(conf.SkillId, true, true, true)
	}

	sys.owner.TakeOffAppearById(appeardef.AppearPos_Wing, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_FairyWingAppear,
		AppearId: fashionId,
	})

	sys.SendProto3(15, 26, &pb3.S2C_15_26{AppearId: fashionId})

	sys.ResetSysAttr(attrdef.SaFairyWingFashion)

	return nil
}

func (sys *FairyWingFashionSys) onActivated(appearId uint32) {
	conf := jsondata.GetFairyWingFashionLvConf(appearId, 0)
	if conf != nil && conf.SkillId != 0 {
		sys.GetOwner().LearnSkill(conf.SkillId, conf.SkillLevel, true)
	}

	sys.SendProto3(15, 23, &pb3.S2C_15_23{AppearId: appearId, Lv: 0})
	sys.ResetSysAttr(attrdef.SaFairyWingFashion)

	if fConf := jsondata.GetFairyWingFashionConf(appearId); fConf != nil {
		sys.GetOwner().TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
			SetId:     fConf.SetId,
			FType:     fConf.FType,
			FashionId: appearId,
		})
		sys.GetOwner().TriggerEvent(custom_id.AeRareTitleActiveFashion, &custom_id.FashionSetEvent{
			FType:     fConf.FType,
			FashionId: appearId,
		})
	}
}

// 仙翼-幻形-幻化
func (sys *FairyWingFashionSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_15_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	if req.AppearId == 0 {
		sys.GetOwner().TakeOffAppear(appeardef.AppearPos_Wing)
		return nil
	}

	appearConf := jsondata.GetFairyWingFashionConf(req.AppearId)
	if nil == appearConf {
		return neterror.ParamsInvalidError("fairy wing fashion conf not found: %v", req.AppearId)
	}

	if _, ok := sys.data.Fashions[req.AppearId]; !ok {
		return neterror.ParamsInvalidError("fairy wing fashion not activated: %v", req.AppearId)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Wing, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_FairyWingAppear,
		AppearId: req.AppearId,
	}, true)
	return nil
}

func (sys *FairyWingFashionSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Fashions[fashionId]
	return ok
}

func (sys *FairyWingFashionSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for itemId, level := range sys.data.Fashions {
		itemConf := jsondata.GetFairyWingFashionLvConf(itemId, level)
		if nil == itemConf {
			continue
		}

		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, itemConf.Attrs)
	}
}

func fairyWingFashionProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFairyWingFashion).(*FairyWingFashionSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func wingsOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaFairyWing, attrdef.SaFairyWingFashion, attrdef.SaGodWing}

	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}
	player.SetRankValue(gshare.RankTypeWing, sumPower)
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeWing, player.GetId(), sumPower)
}

var _ iface.IMaxStarChecker = (*FairyWingFashionSys)(nil)

func (sys *FairyWingFashionSys) IsMaxStar(relateItem uint32) bool {
	fashionId := jsondata.GetFairyWingFashionIdByRelateItem(relateItem)
	if fashionId == 0 {
		sys.LogError("FairyWingFashion don't exist")
		return false
	}
	currentStar, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogDebug("FairyWingFashion %d is not activated", fashionId)
		return false
	}
	nextStar := currentStar + 1
	nextStarConf := jsondata.GetFairyWingFashionLvConf(fashionId, nextStar)
	return nextStarConf == nil
}

func init() {
	RegisterSysClass(sysdef.SiFairyWingFashion, func() iface.ISystem {
		return &FairyWingFashionSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairyWingFashion, fairyWingFashionProperty)
	net.RegisterSysProto(15, 21, sysdef.SiFairyWingFashion, (*FairyWingFashionSys).c2sUpLv)
	net.RegisterSysProto(15, 22, sysdef.SiFairyWingFashion, (*FairyWingFashionSys).c2sDress)
	net.RegisterSysProto(15, 23, sysdef.SiFairyWingFashion, (*FairyWingFashionSys).c2sActivate)

	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, wingsOnUpdateSysPowerMap)
	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeSaFairyWingFashion, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaFairyWingFashion)
	})
}
