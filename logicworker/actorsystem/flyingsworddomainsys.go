/**
 * @Author: LvYuMeng
 * @Date: 2025/3/25
 * @Desc: 飞剑领域
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type FlyingSwordDomainSys struct {
	Base
}

func (s *FlyingSwordDomainSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FlyingSwordDomainSys) OnLogin() {
	s.checkAndUnlockScabbards()
}

func (s *FlyingSwordDomainSys) OnAfterLogin() {
	s.checkAndUnlockScabbards()
	s.s2cInfo()
	if s.getData().TransformId != 0 {
		s.owner.SetExtraAttr(attrdef.FlyingSwordDomainEffect, int64(s.getData().TransformId))
	}
}

func (s *FlyingSwordDomainSys) OnOpen() {
	data := s.getData()
	data.CoreLevel = 0
	data.CoreTier = 1
	s.checkAndUnlockScabbards()
	s.s2cInfo()

	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)
}

// 获取数据
func (s *FlyingSwordDomainSys) getData() *pb3.FlyingSwordDomain {
	data := s.GetBinaryData().FlyingSwordDomain

	if data == nil {
		data = &pb3.FlyingSwordDomain{}
		s.GetBinaryData().FlyingSwordDomain = data
	}

	if data.Scabbards == nil {
		data.Scabbards = make(map[uint32]*pb3.Scabbard)
	}

	if data.Transform == nil {
		data.Transform = make(map[uint32]uint32)
	}

	return data
}

// 下发数据
func (s *FlyingSwordDomainSys) s2cInfo() {
	s.SendProto3(21, 49, &pb3.S2C_21_49{Data: s.getData()})
}

// 剑核升级(包含升阶)
func (s *FlyingSwordDomainSys) c2sUpgradeCore(msg *base.Message) error {
	var req pb3.C2S_21_50
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	nextLevel := data.CoreLevel + 1

	// 获取下一级配置
	nextLvConf := jsondata.GetFSDomainCoreLvConf(nextLevel)
	if nextLvConf == nil {
		return neterror.ParamsInvalidError("next level config not found")
	}

	// 判断是否需要升阶
	needTierUp := data.CoreTier < nextLvConf.Tier
	if needTierUp {
		// 获取升阶配置
		nextTierConf := jsondata.GetFSDomainCoreTierConf(data.CoreTier + 1)
		if nextTierConf == nil {
			return neterror.ParamsInvalidError("next tier config not found")
		}

		// 检查消耗
		if !s.owner.ConsumeByConf(nextTierConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordCoreTierUp}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}

		// 升阶
		data.CoreTier++

		// 检查并解锁新剑匣
		s.checkAndUnlockScabbards()
	} else {
		// 检查消耗
		if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordCoreUpgrade}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}

		// 升级
		data.CoreLevel = nextLevel
	}

	// 发送升级/升阶成功协议
	s.SendProto3(21, 50, &pb3.S2C_21_50{
		Level: data.CoreLevel,
		Tier:  data.CoreTier,
	})
	// 重新计算属性
	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)

	return nil
}

// 检查并解锁新剑匣
func (s *FlyingSwordDomainSys) checkAndUnlockScabbards() {
	data := s.getData()
	coreTier := data.CoreTier
	jsondata.RangeScabbardConf(func(scabbardConf *jsondata.FSDomainScabbardConf) {
		if scabbardConf.UnlockTier > coreTier {
			return
		}
		if _, ok := data.Scabbards[scabbardConf.ScabbardId]; ok {
			return
		}
		data.Scabbards[scabbardConf.ScabbardId] = &pb3.Scabbard{}
	})
}

// 剑匣升级
func (s *FlyingSwordDomainSys) c2sUpgradeScabbard(msg *base.Message) error {
	var req pb3.C2S_21_52
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	scabbardId := req.ScabbardId

	// 检查剑匣是否已解锁
	scabbard, ok := data.Scabbards[scabbardId]
	if !ok {
		return neterror.ParamsInvalidError("scabbard not unlocked")
	}

	// 获取剑匣配置
	scabbardConf := jsondata.GetFSDomainScabbardConf(scabbardId)
	if scabbardConf == nil {
		return neterror.ParamsInvalidError("scabbard config not found")
	}

	// 获取下一级配置
	nextLevel := scabbard.ScabbardLevel + 1
	nextLvConf := jsondata.GetFSDomainScabbardLvConf(scabbardId, nextLevel)
	if nextLvConf == nil {
		return neterror.ParamsInvalidError("next level config not found")
	}

	// 检查消耗
	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordScabbardUpgrade}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 升级
	scabbard.ScabbardLevel = nextLevel

	// 发送升级成功协议
	s.SendProto3(21, 52, &pb3.S2C_21_52{
		ScabbardId: scabbardId,
		Level:      nextLevel,
	})
	// 重新计算属性
	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)

	return nil
}

var (
	_ iface.IFashionChecker = (*RiderFashionInternalSys)(nil)
	_ iface.IFashionChecker = (*RiderFashionSys)(nil)
)

func (s *FlyingSwordDomainSys) getFashionChecker(swordSysId uint32) (iface.IFashionChecker, bool) {
	var fashionSys iface.ISystem
	switch swordSysId {
	case sysdef.SiRiderFashion:
		fashionSys = s.owner.GetSysObj(sysdef.SiRiderFashion)
	case sysdef.SiRiderInternalFashion:
		fashionSys = s.owner.GetSysObj(sysdef.SiRiderInternalFashion)
	default:
		return nil, false
	}

	checker, ok := fashionSys.(iface.IFashionChecker)
	return checker, ok
}

// 飞剑放置
func (s *FlyingSwordDomainSys) c2sPlaceSword(msg *base.Message) error {
	var req pb3.C2S_21_53
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	scabbardId := req.ScabbardId
	swordSysId := req.SwordSysId
	swordId := req.SwordId

	// 检查剑匣是否已解锁
	scabbard, ok := data.Scabbards[scabbardId]
	if !ok {
		return neterror.ParamsInvalidError("scabbard not unlocked")
	}

	// 获取剑匣配置
	scabbardConf := jsondata.GetFSDomainScabbardConf(scabbardId)
	if scabbardConf == nil {
		return neterror.ParamsInvalidError("scabbard config not found")
	}

	// 检查该飞剑是否已放置在其他剑匣中
	for id, scab := range data.Scabbards {
		if id != scabbardId && (scab.SwordId == swordId && scab.SwordSysId == swordSysId) {
			return neterror.ParamsInvalidError("sword already placed in another scabbard")
		}
	}

	// 根据系统ID获取对应时装系统并验证
	checker, ok := s.getFashionChecker(swordSysId)
	if !ok {
		return neterror.ParamsInvalidError("invalid sword system")
	}

	// 检查飞剑是否已激活
	if !checker.CheckFashionActive(swordId) {
		return neterror.ParamsInvalidError("sword fashion not activated")
	}

	// 检查飞剑是否达到品质
	if checker.GetFashionQuality(swordId) < scabbardConf.SwordQuality {
		return neterror.ParamsInvalidError("sword quality not reach")
	}

	// 放置飞剑
	scabbard.SwordSysId = swordSysId
	scabbard.SwordId = swordId

	// 检查技能学习
	swordCount := uint32(0)
	for _, scab := range data.Scabbards {
		if scab.SwordId > 0 {
			swordCount++
		}
	}

	for _, skillConf := range jsondata.FlyingSwordDomainConfigMgr.SkillConf {
		if swordCount >= skillConf.SwordNum {
			s.owner.LearnSkill(skillConf.SkillId, 1, true)
		}
	}

	// 发送放置成功协议
	s.SendProto3(21, 53, &pb3.S2C_21_53{
		ScabbardId: scabbardId,
		SwordSysId: swordSysId,
		SwordId:    swordId,
	})
	// 重新计算属性
	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)

	return nil
}

// 化形激活
func (s *FlyingSwordDomainSys) c2sActivateTransform(msg *base.Message) error {
	var req pb3.C2S_21_54
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	transformId := req.TransformId

	// 检查是否已激活
	if _, ok := data.Transform[transformId]; ok {
		return neterror.ParamsInvalidError("transform already activated")
	}

	// 检查消耗
	starConf := jsondata.GetFSDomainTransformStarConf(transformId, 0) // 0星级配置
	if starConf == nil {
		return neterror.ParamsInvalidError("transform active config not found")
	}
	if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordTransformActivate}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 激活化形,初始0星
	data.Transform[transformId] = 0

	// 发送激活成功协议
	s.SendProto3(21, 54, &pb3.S2C_21_54{
		TransformId: transformId,
	})
	// 重新计算属性
	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)

	return nil
}

// 化形升星
func (s *FlyingSwordDomainSys) c2sUpgradeTransform(msg *base.Message) error {
	var req pb3.C2S_21_55
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	transformId := req.TransformId

	// 检查是否已激活
	star, ok := data.Transform[transformId]
	if !ok {
		return neterror.ParamsInvalidError("transform not activated")
	}

	// 获取化形配置
	transformConf := jsondata.GetFSDomainTransformConf(transformId)
	if transformConf == nil {
		return neterror.ParamsInvalidError("transform config not found")
	}

	// 获取下一星级配置
	nextStar := star + 1
	starConf := jsondata.GetFSDomainTransformStarConf(transformId, nextStar) // 0星级配置
	if !ok {
		return neterror.ParamsInvalidError("star config not found")
	}

	// 检查消耗
	if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFlyingSwordTransformUpgrade}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 升星
	data.Transform[transformId] = nextStar

	// 发送升星成功协议
	s.SendProto3(21, 55, &pb3.S2C_21_55{
		TransformId: transformId,
		Star:        nextStar,
	})
	// 重新计算属性
	s.ResetSysAttr(attrdef.SaFlyingSwordDomain)

	return nil
}

// 化形幻化
func (s *FlyingSwordDomainSys) c2sTransformIllusion(msg *base.Message) error {
	var req pb3.C2S_21_56
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	transformId := req.TransformId

	if transformId > 0 {
		// 检查是否已激活
		if _, ok := data.Transform[transformId]; !ok {
			return neterror.ParamsInvalidError("transform not activated")
		}

		transformConf := jsondata.GetFSDomainTransformConf(transformId)
		if transformConf == nil {
			return neterror.ParamsInvalidError("transform config not found")
		}
	}

	// 更新当前幻化ID
	data.TransformId = transformId

	// 发送幻化成功协议
	s.SendProto3(21, 56, &pb3.S2C_21_56{
		TransformId: transformId,
	})

	s.owner.SetExtraAttr(attrdef.FlyingSwordDomainEffect, int64(transformId))
	return nil
}

// 属性计算
func (s *FlyingSwordDomainSys) calcFlyingSwordDomainAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()

	// 1. 计算剑核属性
	// 1.1 等级属性
	if coreLvConf := jsondata.GetFSDomainCoreLvConf(data.CoreLevel); coreLvConf != nil {
		engine.CheckAddAttrsToCalc(s.owner, calc, coreLvConf.Attrs)
	}
	// 1.2 阶级属性
	if coreTierConf := jsondata.GetFSDomainCoreTierConf(data.CoreTier); coreTierConf != nil {
		engine.CheckAddAttrsToCalc(s.owner, calc, coreTierConf.Attrs)
	}

	// 2. 计算剑匣属性
	for scabbardId, scabbard := range data.Scabbards {
		// 2.1 等级属性
		if lvConf := jsondata.GetFSDomainScabbardLvConf(scabbardId, scabbard.ScabbardLevel); lvConf != nil {
			engine.CheckAddAttrsToCalc(s.owner, calc, lvConf.Attrs)
		}
		// 2.2 生效飞剑固定属性
		if scabbard.SwordId > 0 {
			scabbardConf := jsondata.GetFSDomainScabbardConf(scabbardId)
			if nil == scabbardConf {
				continue
			}
			engine.CheckAddAttrsToCalc(s.owner, calc, scabbardConf.SwordBaseAttrs)
		}
	}

	// 3. 计算化形属性
	for transformId, star := range data.Transform {
		starConf := jsondata.GetFSDomainTransformStarConf(transformId, star)
		if nil == starConf {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, starConf.Attrs)
	}
}

// 计算飞剑加成属性
func (s *FlyingSwordDomainSys) calcFlyingSwordDomainRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	fsDomainScabbardAddRate := totalSysCalc.GetValue(attrdef.FlyingSwordDomainScabbardAddRate)
	scabbardBaseAddRateAttrs := jsondata.GetFSDomainScabbardBaseAddRateAttrs()
	// 遍历所有剑匣
	for scabbardId, scabbard := range data.Scabbards {
		// 获取剑匣配置
		scabbardConf := jsondata.GetFSDomainScabbardConf(scabbardId)
		if scabbardConf == nil {
			continue
		}

		//剑匣加成
		if lvConf := jsondata.GetFSDomainScabbardLvConf(scabbardId, scabbard.ScabbardLevel); lvConf != nil {
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, lvConf.Attrs, uint32(fsDomainScabbardAddRate),
				scabbardBaseAddRateAttrs...)

			if baseRate := totalSysCalc.GetValue(scabbardConf.ScabbardBaseAddRateAttr); baseRate > 0 {
				engine.CheckAddAttrsRateRoundingUp(s.owner, calc, lvConf.Attrs, uint32(baseRate),
					scabbardBaseAddRateAttrs...)
			}
		}

		// 如果剑匣中有飞剑
		if scabbard.SwordId > 0 {
			// 获取时装系统检查器
			checker, ok := s.getFashionChecker(scabbard.SwordSysId)
			if !ok {
				continue
			}

			if attrVec := checker.GetFashionBaseAttr(scabbard.SwordId); nil != attrVec {
				engine.CheckAddAttrsRateRoundingUp(s.owner, calc, attrVec, scabbardConf.SwordBaseAddRate)
			}
		}
	}
}

func calcFlyingSwordDomain(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if s, ok := player.GetSysObj(sysdef.SiFlyingSwordDomain).(*FlyingSwordDomainSys); ok && s.IsOpen() {
		s.calcFlyingSwordDomainAttr(calc)
	}
}

func calcFlyingSwordDomainRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	if s, ok := player.GetSysObj(sysdef.SiFlyingSwordDomain).(*FlyingSwordDomainSys); ok && s.IsOpen() {
		s.calcFlyingSwordDomainRate(totalSysCalc, calc)
	}
}

func init() {
	RegisterSysClass(sysdef.SiFlyingSwordDomain, func() iface.ISystem {
		return &FlyingSwordDomainSys{}
	})

	//注册属性计算
	engine.RegAttrCalcFn(attrdef.SaFlyingSwordDomain, calcFlyingSwordDomain)
	engine.RegAttrAddRateCalcFn(attrdef.SaFlyingSwordDomain, calcFlyingSwordDomainRate)

	// 注册协议
	net.RegisterSysProto(21, 50, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sUpgradeCore)
	net.RegisterSysProto(21, 52, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sUpgradeScabbard)
	net.RegisterSysProto(21, 53, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sPlaceSword)
	net.RegisterSysProto(21, 54, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sActivateTransform)
	net.RegisterSysProto(21, 55, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sUpgradeTransform)
	net.RegisterSysProto(21, 56, sysdef.SiFlyingSwordDomain, (*FlyingSwordDomainSys).c2sTransformIllusion)
}
