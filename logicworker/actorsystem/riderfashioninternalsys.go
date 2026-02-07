package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/fashion"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

/**
* @Author: YangQibin
* @Desc: 坐骑内部时装系统
* @Date: 2023/11/6
 */

type RiderFashionInternalSys struct {
	Base
	data   *pb3.RiderFashionData
	upstar fashion.UpStar
}

func (sys *RiderFashionInternalSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.RiderInternalFashionData == nil {
		binaryData.RiderInternalFashionData = &pb3.RiderFashionData{}
	}

	sys.data = binaryData.RiderInternalFashionData

	if sys.data.Fashions == nil {
		sys.data.Fashions = make(map[uint32]*pb3.DressData)
	}
	sys.upstar = fashion.UpStar{
		Fashions:  sys.data.Fashions,
		LogId:     pb3.LogId_LogRiderInternalFashionUpStar,
		CheckJob:  false,
		AttrSysId: attrdef.SaRiderInternalFashion,
		GetLvConfHandler: func(fashionId, lv uint32) *jsondata.FashionStarConf {
			conf := jsondata.GetRiderInternalFashionStarConf(fashionId, lv)
			if conf != nil {
				return &conf.FashionStarConf
			}
			return nil
		},
		GetFashionConfHandler: func(fashionId uint32) *jsondata.FashionMeta {
			conf := jsondata.GetRiderInternalFashionConf(fashionId)
			if conf != nil {
				return &conf.FashionMeta
			}
			return nil
		},
		AfterUpstarCb:   sys.onUpstar,
		AfterActivateCb: sys.onActivated,
	}

	if err := sys.upstar.Init(); err != nil {
		sys.LogError("RiderFashionInternalSys upstar init failed, err: %v", err)
		return false
	}
	return true
}

func (sys *RiderFashionInternalSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.ResetSysAttr(attrdef.SaRiderInternalFashion)
	sys.SendProto3(21, 40, &pb3.S2C_21_40{Data: sys.data})
}

func (sys *RiderFashionInternalSys) OnReconnect() {
	sys.SendProto3(21, 40, &pb3.S2C_21_40{Data: sys.data})
}

func (sys *RiderFashionInternalSys) OnLogin() {
	if !sys.init() {
		return
	}
	sys.SendProto3(21, 40, &pb3.S2C_21_40{Data: sys.data})
}

func (sys *RiderFashionInternalSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_21_41
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	err := sys.upstar.Upstar(sys.GetOwner(), req.AppearId, req.AutoBuy)
	if err != nil {
		return err
	}
	return nil
}

func (sys *RiderFashionInternalSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_21_44
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	err := sys.upstar.Activate(sys.GetOwner(), req.AppearId, req.AutoBuy, false)
	if err != nil {
		return err
	}

	sys.GetOwner().TriggerQuestEvent(custom_id.QttUnLockRiderFashion, req.AppearId, 1)

	return nil
}

// 助战-飞剑-坐骑外观
func (sys *RiderFashionInternalSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_21_42
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if req.AppearId == 0 {
		return nil
	}

	appearConf := jsondata.GetRiderInternalFashionConf(req.AppearId)
	if nil == appearConf {
		return neterror.InternalError("invalid appearId: %d", req.AppearId)
	}

	_, ok := sys.data.Fashions[req.AppearId]
	if !ok {
		return neterror.InternalError("RiderFashionInternalSys not activate, fashionId: %d", req.AppearId)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Rider, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_RiderInternalAppear,
		AppearId: req.AppearId,
	}, true)

	return nil
}

func (sys *RiderFashionInternalSys) onUpstar(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("RiderFashionInternalSys onUpstar failed, fashionId: %d", fashionId)
		return
	}

	fashionConf := jsondata.GetRiderInternalFashionStarConf(fashionId, fashion.Star)
	if fashionConf != nil {
		sys.GetOwner().LearnSkill(fashionConf.SkillId, fashionConf.SkillLevel, true)
	}

	sys.SendProto3(21, 41, &pb3.S2C_21_41{
		Data: fashion,
	})
}

func (sys *RiderFashionInternalSys) onActivated(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("RiderFashionInternalSys onActivated failed, fashionId: %d", fashionId)
		return
	}

	riderFashionConf := jsondata.GetRiderInternalFashionStarConf(fashionId, fashion.Star)
	if riderFashionConf != nil && riderFashionConf.SkillId != 0 {
		sys.GetOwner().LearnSkill(riderFashionConf.SkillId, riderFashionConf.SkillLevel, true)
	}

	sys.SendProto3(21, 44, &pb3.S2C_21_44{
		Data: fashion,
	})
}

func (sys *RiderFashionInternalSys) GetFashionBaseAttr(fashionId uint32) jsondata.AttrVec {
	foo, ok := sys.data.Fashions[fashionId]
	if !ok {
		return nil
	}
	conf := jsondata.GetRiderInternalFashionStarConf(foo.Id, foo.Star)
	if conf == nil {
		return nil
	}
	return conf.Attrs
}

func (sys *RiderFashionInternalSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for _, foo := range sys.data.Fashions {
		conf := jsondata.GetRiderInternalFashionStarConf(foo.Id, foo.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}
}

func (sys *RiderFashionInternalSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Fashions[fashionId]
	return ok
}

func (sys *RiderFashionInternalSys) GetFashionQuality(fashionId uint32) uint32 {
	appearConf := jsondata.GetRiderInternalFashionConf(fashionId)
	if nil == appearConf {
		return 0
	}
	return jsondata.GetItemQuality(appearConf.RelatedItem)
}

func riderFashionInternalProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiRiderInternalFashion).(*RiderFashionInternalSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func riderOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}
	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaRider, attrdef.SaRiderFashion, attrdef.SaGodRider, attrdef.SaRiderInternalFashion}

	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}
	player.SetRankValue(gshare.RankTypeMount, sumPower)
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeMount, player.GetId(), sumPower)
}

var _ iface.IMaxStarChecker = (*RiderFashionInternalSys)(nil)

func (sys *RiderFashionInternalSys) IsMaxStar(relatedItem uint32) bool {
	fashionId := jsondata.GetRiderInternalFashionIdByRelateItem(relatedItem)
	if fashionId == 0 {
		sys.LogError("RiderFashionInternal don't exist")
		return false
	}
	fashionData, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogDebug("RiderFashionInternal %d is not activated", fashionId)
		return false
	}

	nextStar := fashionData.Star + 1
	nextStarConf := sys.upstar.GetLvConfHandler(fashionId, nextStar)

	return nextStarConf == nil
}

func init() {
	RegisterSysClass(sysdef.SiRiderInternalFashion, func() iface.ISystem {
		return &RiderFashionInternalSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaRiderInternalFashion, riderFashionInternalProperty)
	net.RegisterSysProto(21, 41, sysdef.SiRiderInternalFashion, (*RiderFashionInternalSys).c2sUpLv)
	net.RegisterSysProto(21, 42, sysdef.SiRiderInternalFashion, (*RiderFashionInternalSys).c2sDress)
	net.RegisterSysProto(21, 44, sysdef.SiRiderInternalFashion, (*RiderFashionInternalSys).c2sActivate)
	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, riderOnUpdateSysPowerMap)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeRiderInternalFashion, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaRiderInternalFashion)
	})
}
