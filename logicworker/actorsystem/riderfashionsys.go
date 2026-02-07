package actorsystem

import (
	"encoding/json"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/fashion"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

/**
 * @Author: YangQibin
* @Desc: 坐骑时装系统
* @Date: 2023/3/30
*/

type RiderFashionSys struct {
	Base
	data   *pb3.RiderFashionData
	upstar fashion.UpStar
}

func (sys *RiderFashionSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.RiderFashionData == nil {
		binaryData.RiderFashionData = &pb3.RiderFashionData{}
	}

	sys.data = binaryData.RiderFashionData

	if sys.data.Fashions == nil {
		sys.data.Fashions = make(map[uint32]*pb3.DressData)
	}
	sys.upstar = fashion.UpStar{
		Fashions:  sys.data.Fashions,
		LogId:     pb3.LogId_LogRiderFashionUpStar,
		CheckJob:  false,
		AttrSysId: attrdef.SaRiderFashion,
		GetLvConfHandler: func(fashionId, lv uint32) *jsondata.FashionStarConf {
			conf := jsondata.GetRiderFashionStarConf(fashionId, lv)
			if conf != nil {
				return &conf.FashionStarConf
			}
			return nil
		},
		GetFashionConfHandler: func(fashionId uint32) *jsondata.FashionMeta {
			conf := jsondata.GetRiderFashionConf(fashionId)
			if conf != nil {
				return &conf.FashionMeta
			}
			return nil
		},
		AfterUpstarCb:   sys.onUpstar,
		AfterActivateCb: sys.onActivated,
	}

	if err := sys.upstar.Init(); err != nil {
		sys.LogError("RiderFashionSys upstar init failed, err: %v", err)
		return false
	}
	return true
}

func (sys *RiderFashionSys) OnOpen() {
	if !sys.init() {
		return
	}
	sys.ResetSysAttr(attrdef.SaRiderFashion)
	sys.SendProto3(21, 20, &pb3.S2C_21_20{Data: sys.data})
}

func (sys *RiderFashionSys) OnReconnect() {
	sys.SendProto3(21, 20, &pb3.S2C_21_20{Data: sys.data})
}

func (sys *RiderFashionSys) OnLogin() {
	if !sys.init() {
		return
	}
	sys.SendProto3(21, 20, &pb3.S2C_21_20{Data: sys.data})
}

func (sys *RiderFashionSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_21_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	err := sys.upstar.Upstar(sys.GetOwner(), req.AppearId, req.AutoBuy)
	if err != nil {
		return err
	}
	return nil
}

func (sys *RiderFashionSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_21_24
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	err := sys.upstar.Activate(sys.GetOwner(), req.AppearId, req.AutoBuy, false)
	if err != nil {
		return err
	}

	return nil
}

// 助战-飞剑-幻形
func (sys *RiderFashionSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_21_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if req.AppearId == 0 {
		return nil
	}

	appearConf := jsondata.GetRiderFashionConf(req.AppearId)
	if nil == appearConf {
		return neterror.InternalError("invalid appearId: %d", req.AppearId)
	}

	_, ok := sys.data.Fashions[req.AppearId]
	if !ok {
		return neterror.InternalError("RiderFashionInternalSys not activate, fashionId: %d", req.AppearId)
	}

	sys.GetOwner().TakeOnAppear(appeardef.AppearPos_Rider, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_RiderAppear,
		AppearId: req.AppearId,
	}, true)

	return nil
}

func (sys *RiderFashionSys) onUpstar(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("RiderFashionSys onUpstar failed, fashionId: %d", fashionId)
		return
	}

	sys.GetOwner().TriggerQuestEvent(custom_id.QttUpgradeRiderFashionLevel, 0, int64(fashion.Star))

	riderFashionConf := jsondata.GetRiderFashionStarConf(fashionId, fashion.Star)
	if riderFashionConf != nil {
		sys.GetOwner().LearnSkill(riderFashionConf.SkillId, riderFashionConf.SkillLevel, true)
	}

	logArgs := map[string]any{
		"fashionId": fashionId,
		"star":      fashion.Star,
	}
	bt, _ := json.Marshal(logArgs)
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderUpStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(fashionId),
		StrArgs: string(bt),
	})

	sys.SendProto3(21, 21, &pb3.S2C_21_21{
		Data: fashion,
	})
}

func (sys *RiderFashionSys) onActivated(fashionId uint32) {
	fashion, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("RiderFashionSys onActivated failed, fashionId: %d", fashionId)
		return
	}

	sys.GetOwner().TriggerQuestEvent(custom_id.QttUpgradeRiderFashionLevel, 0, int64(sys.upstar.MinLv))

	riderFashionConf := jsondata.GetRiderFashionStarConf(fashionId, fashion.Star)
	if riderFashionConf != nil && riderFashionConf.SkillId != 0 {
		sys.GetOwner().LearnSkill(riderFashionConf.SkillId, riderFashionConf.SkillLevel, true)
	}

	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogRiderActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(fashionId),
	})

	sys.SendProto3(21, 24, &pb3.S2C_21_24{
		Data: fashion,
	})
}

func (sys *RiderFashionSys) GetFashionBaseAttr(fashionId uint32) jsondata.AttrVec {
	foo, ok := sys.data.Fashions[fashionId]
	if !ok {
		return nil
	}
	conf := jsondata.GetRiderFashionStarConf(foo.Id, foo.Star)
	if conf == nil {
		return nil
	}
	return conf.Attrs
}

func (sys *RiderFashionSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for _, foo := range sys.data.Fashions {
		conf := jsondata.GetRiderFashionStarConf(foo.Id, foo.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}
}

func (sys *RiderFashionSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Fashions[fashionId]
	return ok
}

func (sys *RiderFashionSys) GetFashionQuality(fashionId uint32) uint32 {
	appearConf := jsondata.GetRiderFashionConf(fashionId)
	if nil == appearConf {
		return 0
	}
	return jsondata.GetItemQuality(appearConf.RelatedItem)
}

func riderFashionProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiRiderFashion).(*RiderFashionSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

var _ iface.IMaxStarChecker = (*RiderFashionSys)(nil)

func (sys *RiderFashionSys) IsMaxStar(relatedItem uint32) bool {
	fashionId := jsondata.GetRiderFashionIdByRelateItem(relatedItem)
	if fashionId == 0 {
		sys.LogDebug("RiderFashion don't exist")
		return false
	}
	if sys.data.Fashions[fashionId] == nil {
		sys.LogError("RiderFashionSys data is nil")
		return false
	}
	currentStar := sys.data.Fashions[fashionId].Star
	nextStar := currentStar + 1
	nextStarConf := sys.upstar.GetLvConfHandler(fashionId, nextStar)
	return nextStarConf == nil
}

func init() {
	RegisterSysClass(sysdef.SiRiderFashion, func() iface.ISystem {
		return &RiderFashionSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaRiderFashion, riderFashionProperty)
	net.RegisterSysProto(21, 21, sysdef.SiRiderFashion, (*RiderFashionSys).c2sUpLv)
	net.RegisterSysProto(21, 22, sysdef.SiRiderFashion, (*RiderFashionSys).c2sDress)
	net.RegisterSysProto(21, 24, sysdef.SiRiderFashion, (*RiderFashionSys).c2sActivate)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeRiderFashion, func(player iface.IPlayer) (score int64) {
		return player.GetAttrSys().GetSysPower(attrdef.SaRiderFashion)
	})
}
