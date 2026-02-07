package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/trialactivetype"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/fashion"
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/net"
)

type FashionSys struct {
	Base
	data   *pb3.FashionData
	upstar fashion.UpStar
}

func (sys *FashionSys) IsOpen() bool {
	return true
}

func (sys *FashionSys) OnInit() {
}

func (sys *FashionSys) newTrialActiveSt() (*TrialActiveHandler, error) {
	st := &TrialActiveHandler{}

	st.DoActive = func(params *jsondata.TrialActiveParams) error {
		if len(params.Params) < 1 {
			return neterror.ParamsInvalidError("params < 1")
		}
		appearId := params.Params[0]
		if sys.CheckFashionActive(appearId) {
			return neterror.ParamsInvalidError("is active")
		}
		conf := jsondata.GetFashionConf(appearId)
		if conf == nil {
			return neterror.ParamsInvalidError("fashion conf of %d not found", appearId)
		}

		if conf.IsDefault {
			return neterror.ParamsInvalidError("fashion %d is default can`t be upstar", appearId)
		}
		err := sys.upstar.Activate(sys.GetOwner(), appearId, false, true)
		if err != nil {
			return err
		}
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

func (sys *FashionSys) OnReconnect() {
	sys.SendProto3(13, 0, &pb3.S2C_13_0{Data: sys.data})
}

func (sys *FashionSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.Fashions == nil {
		binaryData.Fashions = &pb3.FashionData{}
	}

	sys.data = binaryData.Fashions

	if sys.data.Fashions == nil {
		sys.data.Fashions = make(map[uint32]*pb3.DressData)
	}

	sys.upstar = fashion.UpStar{
		Fashions:  sys.data.Fashions,
		LogId:     pb3.LogId_LogFashionUpStar,
		CheckJob:  false,
		AttrSysId: attrdef.SaFashion,
		GetLvConfHandler: func(fashionId, lv uint32) *jsondata.FashionStarConf {
			conf := jsondata.GetFashionStartConf(fashionId, lv)
			if conf != nil {
				return &conf.FashionStarConf
			}
			return nil
		},

		GetFashionConfHandler: func(fashionId uint32) *jsondata.FashionMeta {
			conf := jsondata.GetFashionConf(fashionId)
			if conf != nil {
				return &conf.FashionMeta
			}
			return nil
		},

		AfterUpstarCb:   sys.onUpstar,
		AfterActivateCb: sys.onActivated,
	}

	if err := sys.upstar.Init(); err != nil {
		sys.LogError("FashionSys init upstar err:%v", err)
		return false
	}

	return true
}

func (sys *FashionSys) OnOpen() {
	if !sys.init() {
		return
	}

	sys.ResetSysAttr(attrdef.SaFashion)

	sys.SendProto3(13, 0, &pb3.S2C_13_0{
		Data: sys.data,
	})
}

func (sys *FashionSys) OnLogin() {
	if !sys.init() {
		return
	}

	sys.SendProto3(13, 0, &pb3.S2C_13_0{Data: sys.data})
}

func (sys *FashionSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_13_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	conf := jsondata.GetFashionConf(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("fashion conf of %d not found", req.Id)
	}

	if conf.IsDefault {
		return neterror.ParamsInvalidError("fashion %d is default can`t be upstar", req.Id)
	}

	if sys.owner.IsInTrialActive(trialactivetype.ActiveTypeFashion, []uint32{req.Id}) {
		return neterror.ParamsInvalidError("is in trial")
	}

	err := sys.upstar.Upstar(sys.GetOwner(), req.Id, false)
	if nil != err {
		return err
	}

	return nil
}

func (sys *FashionSys) delFashion(fashionId uint32) error {
	fashionData, ok := sys.data.Fashions[fashionId]
	if !ok {
		return nil
	}

	delete(sys.data.Fashions, fashionId)

	for skillId := range fashionData.SkillMap {
		sys.owner.ForgetSkill(skillId, true, true, true)
	}

	if fashionConf := jsondata.GetFashionConf(fashionId); nil != fashionConf {
		sys.owner.TakeOffAppearById(fashionConf.Pos, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_Fashion,
			AppearId: fashionId,
		})
	}

	sys.SendProto3(13, 8, &pb3.S2C_13_8{
		AppearId: fashionId,
	})

	sys.ResetSysAttr(attrdef.SaFashion)

	return nil
}

func (sys *FashionSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_13_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetFashionConf(req.AppearId)
	if conf == nil {
		return neterror.ParamsInvalidError("fashion conf of %d not found", req.AppearId)
	}

	if conf.IsDefault {
		return neterror.ParamsInvalidError("fashion %d is default can`t be upstar", req.AppearId)
	}

	if can, err := sys.upstar.CanActive(sys.owner, req.AppearId, true); !can {
		return err
	}

	if sys.CheckFashionActive(req.AppearId) && sys.owner.IsInTrialActive(trialactivetype.ActiveTypeFashion, []uint32{req.AppearId}) { //试用激活中
		sys.owner.StopTrialActive(trialactivetype.ActiveTypeFashion, []uint32{req.AppearId})
	}

	err := sys.upstar.Activate(sys.GetOwner(), req.AppearId, false, false)
	if err != nil {
		return err
	}

	if req.TakeOn {
		return sys.Dress(req.AppearId, true)
	}

	return nil
}

func (sys *FashionSys) Dress(id uint32, isTip bool) error {
	player := sys.GetOwner()

	fashionConf := jsondata.GetFashionConf(id)
	if nil == fashionConf {
		return neterror.ParamsInvalidError("fashionconf of %d not found", id)
	}

	if _, ok := appeardef.AppearFashionPosSet[fashionConf.Pos]; !ok {
		return neterror.ParamsInvalidError("fashionconf of %d pos invalid pos: %d", id, fashionConf.Pos)
	}

	if !fashionConf.IsDefault && nil == sys.data.Fashions[id] {
		return neterror.ParamsInvalidError("fashion %d not activate or default", id)
	}

	player.TakeOnAppear(fashionConf.Pos, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_Fashion,
		AppearId: id,
	}, isTip)

	return nil
}

func (sys *FashionSys) UnDress(pos uint32) {
	sys.GetOwner().TakeOffAppear(pos)
	sys.SendProto3(13, 4, &pb3.S2C_13_4{
		Pos: pos,
	})
}

func (sys *FashionSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_13_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	player := sys.GetOwner()

	if nil == player {
		return neterror.ParamsInvalidError("player is nil")
	}
	return sys.Dress(req.AppearId, true)
}

func (sys *FashionSys) c2sUnDress(msg *base.Message) error {
	var req pb3.C2S_13_4
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	sys.UnDress(req.Pos)
	return nil
}

func (sys *FashionSys) learnSkill(fashionId uint32) {
	fashionData, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("fashion not found fashionId %d", fashionId)
		return
	}

	conf := jsondata.GetFashionConf(fashionId)
	if conf == nil {
		sys.LogError("fashion conf of %d not found", fashionId)
		return
	}
	starConf := conf.StarConf[fashionData.Star]
	if starConf == nil {
		sys.LogError("fashion conf of %d %d not found", fashionId, fashionData.Star)
		return
	}

	if fashionData.SkillMap == nil {
		fashionData.SkillMap = make(map[uint32]uint32)
	}

	if starConf.SkillId != 0 && starConf.SkillLevel != 0 {
		sys.owner.LearnSkill(starConf.SkillId, starConf.SkillLevel, true)
		fashionData.SkillMap[starConf.SkillId] = starConf.SkillLevel
	}
}

func (sys *FashionSys) onActivated(fashionId uint32) {
	f, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("fashion not found fashionId %d", fashionId)
		return
	}

	sys.learnSkill(fashionId)

	sys.SendProto3(13, 3, &pb3.S2C_13_3{
		Data: f,
	})
}

func (sys *FashionSys) onUpstar(fashionId uint32) {
	f, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogError("fashion not found fashionId %d", fashionId)
		return
	}
	sys.learnSkill(fashionId)
	sys.SendProto3(13, 1, &pb3.S2C_13_1{
		Data: f,
	})
}

func (sys *FashionSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	for _, foo := range sys.data.Fashions {
		conf := jsondata.GetFashionStartConf(foo.Id, foo.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, conf.Attrs)
	}
}

func (sys *FashionSys) CheckFashionActive(fashionId uint32) bool {
	_, ok := sys.data.Fashions[fashionId]
	return ok
}

func fashionUpStarProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFashion).(*FashionSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func handleJobChangeFashion(player iface.IPlayer, job uint32) bool {
	sys, ok := player.GetSysObj(sysdef.SiFashion).(*FashionSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	var kv = make(map[uint32]uint32)
	for oldFashionId, line := range sys.data.Fashions {
		newFashionId := jsondata.GetJobChangeFashionConfByIdAndJob(line.Id, job)
		if newFashionId == 0 {
			sys.LogWarn("Id:%d fashionId=0!!!", player.GetId())
			continue
		}
		kv[oldFashionId] = newFashionId
	}

	for oldFashionId, newFashionId := range kv {
		data := sys.data.Fashions[oldFashionId]
		delete(sys.data.Fashions, oldFashionId)
		data.Id = newFashionId
		sys.data.Fashions[newFashionId] = data
	}

	appearInfo := player.GetMainData().AppearInfo
	for _, appearSt := range appearInfo.Appear {
		newAppearId, ok := kv[appearSt.AppearId]
		if !ok {
			if appearSt.SysId != appeardef.AppearSys_Fashion {
				continue
			}
			fashionConf := jsondata.GetFashionConf(appearSt.AppearId)
			if fashionConf == nil || !fashionConf.IsDefault {
				continue
			}
			newAppearId = jsondata.GetJobChangeFashionConfByIdAndJob(appearSt.AppearId, job)
			if newAppearId == 0 {
				continue
			}
		}
		appearSt.AppearId = newAppearId
		err := sys.Dress(newAppearId, false)
		if err != nil {
			player.LogWarn("err:%v", err)
		}
	}

	return true
}

var _ iface.IMaxStarChecker = (*FashionSys)(nil)

func (sys *FashionSys) IsMaxStar(relatedItem uint32) bool {
	job := sys.owner.GetJob()
	fashionId := jsondata.GetFashionByRelatedItem(relatedItem, job)
	if fashionId == 0 {
		sys.LogError("Fashion data don't exist")
		return false
	}
	fashionData, ok := sys.data.Fashions[fashionId]
	if !ok {
		sys.LogDebug("Fashion %d is not activated", fashionId)
		return false
	}
	conf := jsondata.GetFashionConf(fashionId)
	if conf != nil && conf.IsDefault {
		sys.LogDebug("Fashion %d is default, cannot upstar", fashionId)
		return false
	}

	nextStar := fashionData.Star + 1
	nextStarConf := sys.upstar.GetLvConfHandler(fashionId, nextStar)
	return nextStarConf == nil
}

func init() {
	RegisterSysClass(sysdef.SiFashion, func() iface.ISystem {
		return &FashionSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFashion, fashionUpStarProperty)

	net.RegisterSysProto(13, 1, sysdef.SiFashion, (*FashionSys).c2sUpStar)
	net.RegisterSysProto(13, 2, sysdef.SiFashion, (*FashionSys).c2sDress)
	net.RegisterSysProto(13, 3, sysdef.SiFashion, (*FashionSys).c2sActivate)
	net.RegisterSysProto(13, 4, sysdef.SiFashion, (*FashionSys).c2sUnDress)
	jobchange.RegJobChangeFunc(jobchange.Fashion, &jobchange.Fn{Fn: handleJobChangeFashion})
}
