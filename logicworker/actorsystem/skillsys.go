package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type SkillSys struct {
	Base
	skills map[uint32]*pb3.SkillInfo
}

func (sys *SkillSys) IsOpen() bool {
	return true
}

func (sys *SkillSys) init() bool {
	mainData := sys.GetMainData()
	if mainData == nil {
		return false
	}

	if nil == mainData.Skills {
		mainData.Skills = make(map[uint32]*pb3.SkillInfo)
	}
	sys.skills = mainData.Skills
	return true
}

func (sys *SkillSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *SkillSys) OnLogin() {
}

func (sys *SkillSys) OnLoginFight() {
	isNotice := false
	if len(sys.skills) <= 0 {
		isNotice = true
	}

	sys.AutoLearnVocationSkill(isNotice)

	sys.S2C_List()
}

func (sys *SkillSys) OnAfterLogin() {
}

func (sys *SkillSys) OnReconnect() {
	sys.S2C_List()
}

func (sys *SkillSys) PackFightSrvSkill(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}

	createData.Skills = make(map[uint32]uint32)
	createData.SkillsCd = make(map[uint32]int64)
	for skillId, line := range sys.skills {
		if conf := jsondata.GetSkillConfig(skillId); nil != conf {
			createData.Skills[line.GetId()] = line.GetLevel()
			createData.SkillsCd[line.GetId()] = line.GetCd()
		}
	}
}

// GetSkill 获取技能信息
func (sys *SkillSys) GetSkill(skillId uint32) *pb3.SkillInfo {
	return sys.skills[skillId]
}

// GetSkillLevel 获取技能等级
func (sys *SkillSys) GetSkillLevel(skillId uint32) uint32 {
	if skill := sys.GetSkill(skillId); nil != skill {
		return skill.GetLevel()
	}
	return 0
}

func (sys *SkillSys) s2cUpdateSkill(skill *pb3.SkillInfo, resetCd bool) {
	sys.SendProto3(6, 2, &pb3.S2C_6_2{Skill: skill, Reset_: resetCd})
}

func (sys *SkillSys) addSkill(conf *jsondata.SkillConf, id, level uint32, login bool) bool {
	if nil == conf {
		return false
	}
	lvConf := conf.LevelConf[level]
	if nil == lvConf {
		return false
	}

	//var oldLv uint32 = 0
	if skill := sys.GetSkill(id); nil != skill {
		if skill.GetLevel() < level {
			skill.Level = level
		} else if !login {
			return false
		}
	} else {
		skill = &pb3.SkillInfo{Id: id, Level: level}
		sys.skills[id] = skill
	}
	return true
}

func (sys *SkillSys) checkSkillCd(conf *jsondata.SkillConf, id, level uint32, isUpdate bool) (int64, bool) {
	if isUpdate {
		return 0, false
	}

	return 0, true
}

func (sys *SkillSys) isSkillUpdate(id, level uint32) bool {
	if skill := sys.GetSkill(id); nil != skill && skill.GetLevel() != level {
		return true
	}
	return false
}

// LearnSkill 学习技能
func (sys *SkillSys) LearnSkill(id, level uint32, send bool) bool {
	conf := jsondata.GetSkillConfig(id)
	if nil == conf {
		return false
	}

	isUpdate := sys.isSkillUpdate(id, level)

	if !sys.addSkill(conf, id, level, false) {
		return false
	}

	cd, resetCd := sys.checkSkillCd(conf, id, level, isUpdate)

	sys.onLearnSkill(conf, id, level, cd, resetCd)

	if send {
		if skill, ok := sys.skills[id]; ok {
			sys.s2cUpdateSkill(skill, resetCd)
		}
	}
	sys.owner.TriggerQuestEvent(custom_id.QttActiveSkill, id, 1)
	sys.ResetSysAttr(attrdef.SaSkill)
	return true
}

func (sys *SkillSys) onLearnSkill(conf *jsondata.SkillConf, id, level uint32, cd int64, resetCd bool) {
	if nil == conf {
		return
	}
	actor := sys.owner

	sys.owner.CallActorFunc(actorfuncid.LearnSkill, &pb3.G2FLearnSkillSt{
		SkillId:    id,
		SkillLv:    level,
		Cd:         cd,
		NedResetCd: resetCd,
	})

	// 触发事件
	switch conf.DeriveFrom {
	case custom_id.SkillDeriveFromVocation:
		actor.TriggerEvent(custom_id.AeLearnVocationSkill, id, level)
		sys.onVocationSkillLvChange()
	case custom_id.SkillDeriveFromSys:
		actor.TriggerEvent(custom_id.AeLearnSysSkill, id, level)
	}
}

func (sys *SkillSys) GetVocationSkillCountByLevel(level uint32) uint32 {
	job := sys.owner.GetJob()
	var count uint32
	for id, data := range sys.skills {
		curLevel := data.GetLevel()
		if curLevel < level {
			continue
		}
		if conf := jsondata.GetSkillConfig(id); nil != conf && uint32(conf.Job) == job {
			count++
		}
	}

	return count
}

// ForgetSkill 遗忘技能
func (sys *SkillSys) ForgetSkill(id uint32, recalcAttr bool, send bool, syncFight bool) {
	if _, ok := sys.skills[id]; !ok {
		return
	}

	delete(sys.skills, id)
	if syncFight {
		sys.onForgetSkill(id)
	}

	if recalcAttr {
		sys.ResetSysAttr(attrdef.SaSkill)
	}

	if send {
		sys.SendProto3(6, 3, &pb3.S2C_6_3{Id: id})
	}
}

func (sys *SkillSys) onForgetSkill(id uint32) {
	conf := jsondata.GetSkillConfig(id)
	if nil == conf {
		return
	}

	sys.owner.CallActorFunc(actorfuncid.ForgetSkill, &pb3.G2FForgetSkillSt{SkillId: id})
}

func (sys *SkillSys) OnPlayerLevelChange() {
	sys.AutoLearnVocationSkill(true)
}

func (sys *SkillSys) AutoLearnVocationSkill(notice bool) {
	actor := sys.owner

	actorLevel := actor.GetLevel()

	job := actor.GetJob()
	tmp := make(map[uint32]uint32)
	for _, line := range jsondata.SkillConfMgr {
		// 来源判断
		if line.DeriveFrom != custom_id.SkillDeriveFromVocation {
			continue
		}

		// 职业判断
		if line.Job > 0 && line.Job != uint8(job) {
			continue
		}
		//已经学过
		if _, ok := sys.skills[line.Id]; ok {
			continue
		}

		//等级不足
		if line.ActiveLevel <= 0 || actorLevel < line.ActiveLevel {
			continue
		}

		tmp[line.Id] = 1
	}
	for id, level := range tmp {
		sys.LearnSkill(id, level, notice)
	}
}

func (sys *SkillSys) S2C_List() {
	sys.SendProto3(6, 1, &pb3.S2C_6_1{Datas: sys.skills})
}

// 一键升级技能
func (sys *SkillSys) c2sUpgradeSkillsLevels(msg *base.Message) error {
	var req pb3.C2S_6_4
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	consume := jsondata.ConsumeVec{}

	// build consume and check level
	for _, kv := range req.Skills {
		skillId := kv.Key
		targetLevel := kv.Value

		skill := sys.GetSkill(skillId)

		if nil == skill {
			return neterror.ParamsInvalidError("invalid skill %d", skillId)
		}

		if targetLevel <= skill.Level {
			return neterror.ParamsInvalidError("skill %d don`t need to upgrade", skillId)
		}

		conf := jsondata.GetSkillConfig(skillId)

		if nil == conf {
			return neterror.InternalError("skill conf is nil")
		}

		if !conf.CanUpdateDirectly {
			return neterror.ParamsInvalidError("not allow skill level up directly")
		}

		for i := skill.Level + 1; i <= targetLevel; i++ {
			lvConf := conf.LevelConf[i]
			consume = append(consume, lvConf.Consume...)
		}
	}

	// consume
	if !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpgradeSkill}) {
		return neterror.InternalError("consume failed")
	}

	// upgrade
	res := pb3.S2C_6_4{}
	for _, kv := range req.Skills {
		skillId := kv.Key
		targetLevel := kv.Value

		skill := sys.GetSkill(skillId)

		conf := jsondata.GetSkillConfig(skillId)
		if !sys.addSkill(conf, skillId, targetLevel, true) {
			return neterror.InternalError("add level failed")
		}

		sys.onLearnSkill(conf, skillId, targetLevel, 0, false)

		sys.ResetSysAttr(attrdef.SaSkill)

		res.Skills = append(res.Skills, skill)
	}
	sys.SendProto3(6, 4, &res)
	return nil
}

func (sys *SkillSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_6_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	id := req.GetId()
	conf := jsondata.GetSkillConfig(id)
	if nil == conf {
		return neterror.ConfNotFoundError("skill conf not found for skill id %d", id)
	}

	var lvBeforeUp uint32 = 0
	info, ok := sys.skills[id]

	if !ok {
		return neterror.ParamsInvalidError("not learn skill %d", id)
	}

	if !conf.CanUpdateDirectly {
		return neterror.ParamsInvalidError("not allow skill level up directly")
	}

	lvBeforeUp = info.GetLevel()
	lvConf := conf.LevelConf[lvBeforeUp+1]
	if nil == lvConf {
		return neterror.ConfNotFoundError("lvConf not found for level %d", lvBeforeUp+1)
	}

	owner := sys.GetOwner()
	for _, v := range lvConf.UpLvCond {
		if !CheckReach(owner, v.Type, v.Val) {
			return neterror.ParamsInvalidError("active skill cond not reach")
		}
	}

	if len(lvConf.Consume) == 0 {
		sys.LogWarn("skill %d lv %d up consume is nil", id, lvBeforeUp+1)
	}

	if !owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpgradeSkill}) {
		return neterror.ParamsInvalidError("consume failed")
	}
	newLv := lvBeforeUp + 1

	if !sys.LearnSkill(id, newLv, true) {
		return neterror.InternalError("learn skill failed")
	}

	return nil
}

func (sys *SkillSys) calcVocationSkillTotalLv() (totalLv uint32) {
	for key, line := range sys.skills {
		if conf := jsondata.GetSkillConfig(key); nil != conf {
			if conf.IsBasic {
				totalLv += line.GetLevel()
			}
		}
	}
	return totalLv
}

func (sys *SkillSys) calcActorGongFaTotalLv() (totalLv uint32) {
	for key, line := range sys.skills {
		if conf := jsondata.GetGongFaConfByJob(sys.GetOwner().GetJob(), key); nil != conf {
			totalLv += line.GetLevel()
		}
	}
	return totalLv
}

// 当来源为职业技能的等级变化时 触发该函数
func (sys *SkillSys) onVocationSkillLvChange() {
	totalLv := sys.calcVocationSkillTotalLv()
	sys.owner.TriggerQuestEvent(custom_id.QttBasicsSkillTotalLv, 0, int64(totalLv))
}

func (sys *SkillSys) calcSysAttr(calc *attrcalc.FightAttrCalc) {
	for id, skill := range sys.skills {
		conf := jsondata.GetSkillConfig(id)
		if nil == conf {
			continue
		}

		level := skill.GetLevel()

		if lvConf, ok := conf.LevelConf[level]; ok {
			if len(lvConf.FixedAttrs) > 0 {
				engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, lvConf.FixedAttrs)
			}
		}
	}
}

// 一键升级技能
func (sys *SkillSys) c2sActiveSkillByReachCond(msg *base.Message) error {
	var req pb3.C2S_6_150
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	skillId := req.SkillId
	conf := jsondata.GetCondSkillActiveConfById(req.SysId, req.SkillId)
	if nil == conf {
		return neterror.ConfNotFoundError("active skill conf is nil")
	}

	if conf.ServerSysId > 0 {
		sys := sys.owner.GetSysObj(conf.ServerSysId)
		if nil == sys || !sys.IsOpen() {
			return neterror.ParamsInvalidError("sys %d not open", conf.ServerSysId)
		}
	}

	for _, v := range conf.Cond {
		if !CheckReach(sys.owner, v.Type, v.Val) {
			return neterror.ParamsInvalidError("active skill cond not reach")
		}
	}

	if skillInfo := sys.owner.GetSkillInfo(skillId); nil != skillInfo {
		return neterror.InternalError("skill learned")
	}

	if !sys.owner.LearnSkill(skillId, 1, true) {
		return neterror.InternalError("active skill failed, skillId: %d", skillId)
	}

	return nil
}
func skillSysOnPlayerLvChange(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys); ok {
		sys.OnPlayerLevelChange()
	}
}

// 技能系统属性
func skillSysAttrCalcFn(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcSysAttr(calc)
}

// 获取指定技能激活
func getActiveSkillById(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return 0
	}
	if _, isActive := sys.skills[ids[0]]; isActive {
		return 1
	}
	return 0
}

// 获取技能数量任务进度
func getSkillCountByLevel(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return 0
	}
	return sys.GetVocationSkillCountByLevel(ids[0])
}

func getBasicSkillTotalLv(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return 0
	}

	return sys.calcVocationSkillTotalLv()
}

func getActorGongFaTotalLv(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return 0
	}

	return sys.calcActorGongFaTotalLv()
}

func onSetSkill(player iface.IPlayer, buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	if msg.BParam {
		player.LearnSkill(msg.U32Param, msg.U32Param2, false)
	} else {
		player.ForgetSkill(msg.U32Param, true, true, true)
	}
}

func onSyncSkillCd(player iface.IPlayer, buf []byte) {
	var req pb3.F2GSyncSkillCd
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return
	}

	for id, cd := range req.Info {
		if v, ok := sys.skills[id]; ok && nil != v {
			v.Cd = cd
		}
	}

	cur := time.Now().UnixMilli()
	for _, line := range sys.skills {
		if line.GetCd() > 0 && line.GetCd() < cur {
			line.Cd = 0
		}
	}
}

func handleJobChangeBasicSkill(player iface.IPlayer, job uint32) bool {
	sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys)
	if !ok {
		return false
	}
	gongFaSys, ok := player.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys)
	if !ok || !gongFaSys.IsOpen() {
		return false
	}
	data := gongFaSys.GetData()
	var list []*pb3.KeyValue
	for _, skillInfo := range sys.skills {
		newSkillId := jsondata.GetJobChangeSkillConfByIdAndJob(skillInfo.Id, job)
		if newSkillId == 0 {
			continue
		}

		// 玩家设置那里需要重新设置技能键位
		settings := player.GetBinaryData().Settings
		for idx, skillId := range settings.U64S {
			if skillId != uint64(skillInfo.Id) {
				continue
			}
			settings.U64S[idx] = uint64(newSkillId)
			break
		}

		list = append(list, &pb3.KeyValue{
			Key:   newSkillId,
			Value: skillInfo.Level,
		})

		// 功法+功法觉醒
		delete(data.MaxSkill, skillInfo.Id)
		extra := data.ExtraInfo[skillInfo.Id]
		delete(data.ExtraInfo, skillInfo.Id)
		data.ExtraInfo[newSkillId] = extra

		sys.ForgetSkill(skillInfo.Id, false, false, false)
	}
	for _, val := range list {
		data.MaxSkill[val.Key] = val.Value
		sys.LearnSkill(val.Key, val.Value, false)
	}
	return true
}

func init() {
	RegisterSysClass(sysdef.SiSkill, func() iface.ISystem {
		return &SkillSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSkill, skillSysAttrCalcFn)
	engine.RegisterActorCallFunc(playerfuncid.SetSKill, onSetSkill)

	event.RegActorEvent(custom_id.AeLevelUp, skillSysOnPlayerLvChange)

	net.RegisterSysProto(6, 1, sysdef.SiSkill, (*SkillSys).c2sUpgrade)
	net.RegisterSysProto(6, 4, sysdef.SiSkill, (*SkillSys).c2sUpgradeSkillsLevels)
	net.RegisterSysProto(6, 150, sysdef.SiSkill, (*SkillSys).c2sActiveSkillByReachCond)

	engine.RegQuestTargetProgress(custom_id.QttSkillCount, getSkillCountByLevel)
	engine.RegQuestTargetProgress(custom_id.QttBasicsSkillTotalLv, getBasicSkillTotalLv)
	engine.RegQuestTargetProgress(custom_id.QttActorGongFaTotalLv, getActorGongFaTotalLv)
	engine.RegQuestTargetProgress(custom_id.QttActiveSkill, getActiveSkillById)

	engine.RegisterActorCallFunc(playerfuncid.SyncSkillCd, onSyncSkillCd)

	gmevent.Register("addSkill", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		id := utils.AtoUint32(args[0])
		level := uint32(1)
		if len(args) >= 2 {
			level = utils.AtoUint32(args[1])
		}
		if sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys); ok {
			sys.LearnSkill(id, level, true)
		}
		return true
	}, 1)

	gmevent.Register("delSkill", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		id := utils.AtoUint32(args[0])

		if sys, ok := player.GetSysObj(sysdef.SiSkill).(*SkillSys); ok {
			sys.ForgetSkill(id, true, true, true)
		}
		return true
	}, 1)

	jobchange.RegJobChangeFunc(jobchange.BasicSkill, &jobchange.Fn{Fn: handleJobChangeBasicSkill})
}
