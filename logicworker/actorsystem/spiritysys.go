package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type SpiritySys struct {
	Base
}

func (sys *SpiritySys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *SpiritySys) OnLogin() {
	data := sys.GetBinaryData().SpiritData
	tryFixSkill := func() {
		for skinId := range data.Skins {
			skinConf := jsondata.GetSpiritPetConf(skinId)
			if skinConf == nil {
				sys.LogError("skinConf for %v is nil", skinId)
				continue
			}
			if skinConf.SkillId == 0 {
				continue
			}
			skill := sys.GetOwner().GetSkillInfo(uint32(skinConf.SkillId))
			if skill == nil {
				sys.GetOwner().LearnSkill(uint32(skinConf.SkillId), 1, false)
			}
		}
	}
	tryFixSkill()
}

func (sys *SpiritySys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *SpiritySys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *SpiritySys) s2cInfo() {
	sys.SendProto3(16, 0, &pb3.S2C_16_0{
		Data: sys.GetBinaryData().SpiritData,
	})
}

func (sys *SpiritySys) init() {
	spiritData := sys.GetBinaryData().SpiritData

	if nil == spiritData {
		sys.GetBinaryData().SpiritData = &pb3.SpiritPetData{
			Skins:     make(map[uint32]uint32),
			SkinsInfo: make(map[uint32]*pb3.SpiritPetFashion),
		}
		return
	}

	if spiritData.Skins == nil {
		spiritData.Skins = make(map[uint32]uint32)
	}

	if spiritData.SkinsInfo == nil {
		spiritData.SkinsInfo = make(map[uint32]*pb3.SpiritPetFashion)
	}

	sys.fixData()
}

func (sys *SpiritySys) fixData() {
	data := sys.GetBinaryData().SpiritData
	for skin := range data.Skins {
		if nil == data.SkinsInfo[skin] {
			data.SkinsInfo[skin] = &pb3.SpiritPetFashion{}
		}
	}
}

func (sys *SpiritySys) OnOpen() {
	sys.init()

	sys.s2cInfo()
}

func (sys *SpiritySys) ActiveSkin(skinId uint32) error {
	data := sys.GetBinaryData().SpiritData

	_, ok := data.Skins[skinId]
	if ok {
		return neterror.ParamsInvalidError("skin already active %d", skinId)
	}

	data.Skins[skinId] = 1
	data.SkinsInfo[skinId] = &pb3.SpiritPetFashion{Star: 0}

	sys.SendProto3(16, 1, &pb3.S2C_16_1{
		SkinId: skinId,
		Level:  1,
	})

	sys.onSpiritActivated(skinId)
	return nil
}

func (sys *SpiritySys) DisactiveSkin(skinId uint32) error {
	data := sys.GetBinaryData().SpiritData

	_, ok := data.Skins[skinId]
	if !ok {
		return neterror.ParamsInvalidError("skin not active %d", skinId)
	}

	delete(data.Skins, skinId)
	delete(data.SkinsInfo, skinId)

	if data.CurId == skinId {
		sys.takeOff()
	}

	sys.onSpiritDisactived(skinId)
	sys.SendProto3(16, 4, &pb3.S2C_16_4{
		SkinId: skinId,
	})
	return nil
}

func (sys *SpiritySys) onSpiritDisactived(skinId uint32) {
	sys.ResetSysAttr(attrdef.SaSpiritPet)
	func() {
		skinConf := jsondata.GetSpiritPetConf(skinId)
		if skinConf == nil {
			sys.LogError("onSpiritDisactivated skinConf for %v is nil", skinId)
			return
		}
		if skinConf.SkillId == 0 {
			return
		}
		sys.GetOwner().ForgetSkill(uint32(skinConf.SkillId), false, true, true)
	}()
}

func (sys *SpiritySys) onSpiritActivated(skinId uint32) {
	sys.ResetSysAttr(attrdef.SaSpiritPet)
	sys.owner.TriggerQuestEventRange(custom_id.QttActiveSpiritSkin)
	func() {
		skinConf := jsondata.GetSpiritPetConf(skinId)
		if skinConf == nil {
			sys.LogError("onSpiritActivated skinConf for %v is nil", skinId)
			return
		}
		if skinConf.SkillId == 0 {
			return
		}
		success := sys.GetOwner().LearnSkill(uint32(skinConf.SkillId), 1, true)
		if !success {
			sys.LogError("onSpiritActivated learn skill failed %d", skinConf.SkillId)
		}
	}()

}

func (sys *SpiritySys) activate(skinId uint32) error {
	conf := jsondata.GetSpiritPetConf(skinId)
	if nil == conf {
		return neterror.ParamsInvalidError("skin not exist %d", skinId)
	}

	if len(conf.Lvconf) == 0 {
		return neterror.ParamsInvalidError("skin lv not exist %d", skinId)
	}

	consume := conf.Lvconf[0].Consume
	if !sys.GetOwner().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritActivate}) {
		return neterror.ParamsInvalidError("not enough %v", consume)
	}
	err := sys.GetOwner().InitiativeEndExperience(pb3.ExperienceType_ExperienceTypeSpirity, &pb3.CommonSt{
		U32Param: skinId,
	})
	if err != nil {
		sys.LogError("err:%v", err)
		return err
	}

	err = sys.ActiveSkin(skinId)
	if err != nil {
		return err
	}
	return nil
}

func (sys *SpiritySys) takeOff() error {
	data := sys.GetBinaryData().SpiritData

	curSkin := data.CurId
	data.CurId = 0

	if curSkin == 0 {
		return neterror.ParamsInvalidError("curSkin is zero")
	}

	sys.GetOwner().SetExtraAttr(attrdef.SpiritSkin, int64(0))
	sys.GetOwner().CallActorFunc(actorfuncid.TakeOffSpiritPet, nil)

	sys.SendProto3(16, 3, &pb3.S2C_16_3{
		SkinId: curSkin,
	})

	return nil
}

func (sys *SpiritySys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_16_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	if err := sys.takeOn(req.SkinId); err != nil {
		return err
	}
	return nil
}

func (sys *SpiritySys) takeOn(skinId uint32) error {
	data := sys.GetBinaryData().SpiritData

	_, ok := data.Skins[skinId]
	if !ok {
		return neterror.ParamsInvalidError("skin not active %d", skinId)
	}

	data.CurId = skinId

	sys.GetOwner().CallActorFunc(actorfuncid.TakeOnSpiritPet, &pb3.G2FTakeOnSpirit{
		SpiritId: data.CurId,
	})

	sys.SendProto3(16, 2, &pb3.S2C_16_2{
		SkinId: skinId,
	})

	return nil
}

func (sys *SpiritySys) CalcProperty(calc *attrcalc.FightAttrCalc) {
	if nil == sys.GetBinaryData().SpiritData {
		return
	}

	data := sys.GetBinaryData().SpiritData
	for skin, lev := range data.Skins {
		if lvConf := jsondata.GetSpiritPetLvConf(skin, lev); nil != lvConf {
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.Attrs)
		}

		if starConf := jsondata.GetSpiritPetStarConf(skin, data.SkinsInfo[skin].Star); nil != starConf {
			engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, starConf.Attrs)
		}
	}
}

func (sys *SpiritySys) c2sStarUp(msg *base.Message) error {
	var req pb3.C2S_16_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	skinId := req.SkinId
	conf := jsondata.GetSpiritPetConf(skinId)
	if nil == conf {
		return neterror.ParamsInvalidError("skin not exist %d", skinId)
	}

	data := sys.GetBinaryData().SpiritData

	_, ok := data.Skins[skinId]
	if !ok {
		if nil == conf.StarConf[0] {
			return neterror.ConfNotFoundError("spirit pet star conf is nil")
		}
		consume := conf.StarConf[0].Consumes
		if !sys.GetOwner().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritActivate}) {
			sys.GetOwner().SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		if err := sys.GetOwner().InitiativeEndExperience(pb3.ExperienceType_ExperienceTypeSpirity, &pb3.CommonSt{
			U32Param: skinId,
		}); nil != err {
			return err
		}
		return sys.ActiveSkin(skinId)
	}

	if obj := sys.GetOwner().GetSysObj(sysdef.SiFirstExperience); nil != obj && obj.IsOpen() {
		firstExperienceSys, succ := obj.(*FirstExperienceSys)
		if !succ {
			return neterror.ParamsInvalidError("first experience sys get err")
		}
		inExperience, _ := firstExperienceSys.IsInExperience(pb3.ExperienceType_ExperienceTypeSpirity, skinId)
		if inExperience {
			return neterror.ParamsInvalidError("skin %d is in experience", skinId)
		}
	}

	skinInfo := data.SkinsInfo[skinId]
	nextStar := skinInfo.Star + 1
	if int(nextStar) >= len(conf.StarConf) {
		return neterror.ParamsInvalidError("skin %d star not exist %d", skinId, nextStar)
	}

	consume := conf.StarConf[nextStar].Consumes
	if !sys.GetOwner().ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritStarUp}) {
		sys.GetOwner().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	skinInfo.Star = nextStar

	sys.ResetSysAttr(attrdef.SaSpiritPet)

	sys.SendProto3(16, 5, &pb3.S2C_16_5{
		SkinId: skinId,
		Star:   skinInfo.Star,
	})
	return nil
}

func (sys *SpiritySys) CheckSpiritActive(skinId uint32) bool {
	data := sys.GetBinaryData().SpiritData
	_, ok := data.Skins[skinId]
	return ok
}

func (sys *SpiritySys) CheckFashionActive(fashionId uint32) bool {
	return sys.CheckSpiritActive(fashionId)
}

func calcSpiritAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.CalcProperty(calc)
}

func actSpirit(player iface.IPlayer, args ...string) bool {
	spiritsys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok {
		return false
	}

	if len(args) < 1 {
		return false
	}

	skinId := utils.AtoUint32(args[0])

	err := spiritsys.ActiveSkin(skinId)
	if err != nil {
		player.LogError("actSpirit failed %s", err)
		return false
	}

	return true
}

func handleUseItemSpirit(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		return
	}
	skinId := conf.Param[0]
	obj := player.GetSysObj(sysdef.SiSpiritPet)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritySys)
	if !ok {
		return
	}
	err := sys.activate(skinId)
	if err != nil {
		sys.LogError("err:%v", err)
		return
	}
	return true, true, 1
}

func handleQttActiveSpiritSkin(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	sys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok || !sys.IsOpen() {
		return 0
	}

	skinId := ids[0]
	if !sys.CheckSpiritActive(skinId) {
		return 0
	}

	if feSys := sys.GetOwner().GetSysObj(sysdef.SiFirstExperience).(*FirstExperienceSys); nil != feSys && feSys.IsOpen() {
		inExperience, _ := feSys.IsInExperience(pb3.ExperienceType_ExperienceTypeSpirity, skinId)
		if inExperience {
			return 0
		}
	}

	return 1
}

var _ iface.IMaxStarChecker = (*SpiritySys)(nil)

func (sys *SpiritySys) IsMaxStar(relatedItem uint32) bool {
	data := sys.GetBinaryData().SpiritData
	if data == nil {
		return false
	}
	id := jsondata.GetSpiritPetIdByRelateItem(relatedItem)
	if id == 0 {
		sys.LogError("Spirit don't exist")
		return false
	}
	_, isActive := data.Skins[id]
	if !isActive {
		return false
	}

	skinConf := jsondata.GetSpiritPetConf(id)
	if skinConf == nil {
		sys.LogError("Spirit config not found")
		return false
	}

	skinInfo := data.SkinsInfo[id]
	if skinInfo == nil {
		sys.LogError("Spirit skin info not found")
		return false
	}
	currentStar := skinInfo.Star
	nextStar := currentStar + 1

	// 下一星级 >= 配置中的星级数量 说明无配置
	// 长度即为最大可升星数
	if int(nextStar) >= len(skinConf.StarConf) {
		return true
	}

	return false
}

func init() {
	RegisterSysClass(sysdef.SiSpiritPet, func() iface.ISystem {
		return &SpiritySys{}
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemSpirit, handleUseItemSpirit)

	engine.RegAttrCalcFn(attrdef.SaSpiritPet, calcSpiritAttr)

	net.RegisterSysProto(16, 2, sysdef.SiSpiritPet, (*SpiritySys).c2sTakeOn)
	net.RegisterSysProto(16, 5, sysdef.SiSpiritPet, (*SpiritySys).c2sStarUp)

	engine.RegQuestTargetProgress(custom_id.QttActiveSpiritSkin, handleQttActiveSpiritSkin)

	event.RegActorEvent(custom_id.AeLoginFight, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
		if !ok || !sys.IsOpen() {
			return
		}
		data := sys.GetBinaryData().SpiritData

		tryTakeOnSpirit := func() {
			if data.CurId == 0 {
				return
			}

			err := sys.takeOn(data.CurId)
			if err != nil {
				sys.LogError("%s", err)
			}
		}
		tryTakeOnSpirit()
	})

	gmevent.Register("actspirit", actSpirit, 1)
}
