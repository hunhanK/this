package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type GongFaSys struct {
	Base
}

func (sys *GongFaSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GongFa {
		binary.GongFa = &pb3.GongFaData{
			MaxSkill: make(map[uint32]uint32),
		}
	}
	if nil == binary.GongFa.MaxSkill {
		binary.GongFa.MaxSkill = make(map[uint32]uint32)
	}
}

func (sys *GongFaSys) GetData() *pb3.GongFaData {
	binary := sys.GetBinaryData()
	if nil == binary.GongFa {
		binary.GongFa = &pb3.GongFaData{
			MaxSkill: make(map[uint32]uint32),
		}
	}
	if nil == binary.GongFa.MaxSkill {
		binary.GongFa.MaxSkill = make(map[uint32]uint32)
	}
	if nil == binary.GongFa.ExtraInfo {
		binary.GongFa.ExtraInfo = make(map[uint32]*pb3.GongFaExtra)
	}
	return binary.GongFa
}

func (sys *GongFaSys) OnLogin() {
	sys.checkProgressOpen()
}

func (sys *GongFaSys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *GongFaSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *GongFaSys) s2cInfo() {
	data := sys.GetData()
	sys.SendProto3(6, 54, &pb3.S2C_6_54{Progress: data.GetProgress()})
	sys.SendProto3(6, 55, &pb3.S2C_6_55{
		MaxSkill:   data.GetMaxSkill(),
		SkillExtra: data.GetExtraInfo(),
	})
}

func (sys *GongFaSys) OnOpen() {
	sys.checkProgressOpen()
}

func (sys *GongFaSys) checkProgressOpen() bool {
	conf := jsondata.GetGongFaProgressConf()
	if nil == conf {
		return false
	}
	blv := sys.owner.GetExtraAttrU32(attrdef.Circle)
	openDay := gshare.GetOpenServerDay()
	send := false
	data := sys.GetData()
	for id, progress := range conf {
		if utils.SliceContainsUint32(data.Progress, id) {
			continue
		}
		if blv < progress.BoundaryLimit {
			continue
		}
		if openDay < progress.OpenDay {
			continue
		}
		//todo add 跨服条件
		data.Progress = append(data.Progress, id)
		send = true
	}
	if send {
		sys.SendProto3(6, 54, &pb3.S2C_6_54{Progress: sys.GetData().Progress})
	}
	return true
}

func (sys *GongFaSys) checkLearnSkillCond(conf *jsondata.GongFaJobConf) bool {
	blv := sys.owner.GetExtraAttrU32(attrdef.Circle)
	if blv < conf.Boundary {
		sys.owner.SendTipMsg(tipmsgid.TpBoundary)
		return false
	}
	//主线任务
	if conf.MainTaskId > 0 {
		questSys, ok := sys.owner.GetSysObj(sysdef.SiQuest).(*QuestSys)
		if !ok || !questSys.IsFinishMainQuest(conf.MainTaskId) {
			return false
		}
	}
	data := sys.GetData()
	for _, preSkill := range conf.SkillLimit {
		maxLv := data.MaxSkill[preSkill.SkillId]
		if maxLv < preSkill.SkillLv {
			sys.owner.SendTipMsg(tipmsgid.TpPreSkillNotUnLock)
			return false
		}
	}
	return true
}

func (sys *GongFaSys) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_6_53
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	skillId := req.GetId()
	conf := jsondata.GetGongFaConfByJob(sys.owner.GetJob(), skillId)
	if nil == conf {
		return neterror.ConfNotFoundError("gongfa conf(%d) is nil", skillId)
	}
	data := sys.GetData()
	if !utils.SliceContainsUint32(data.Progress, conf.Progress) {
		return neterror.ParamsInvalidError("server progress not open")
	}
	if !sys.checkLearnSkillCond(conf) {
		return nil
	}
	newLv := sys.owner.GetSkillLv(skillId) + 1
	if int(newLv) > len(conf.Level) {
		return neterror.ConfNotFoundError("gongfa skill(%d) is lv max", skillId)
	}
	if sys.owner.GetLevel() < conf.Level[newLv-1].LvLimit {
		sys.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}
	if !sys.owner.ConsumeByConf(conf.Level[newLv-1].Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGongFaLevelUp}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	if conf.ReplaceId > 0 {
		sys.owner.ForgetSkill(conf.ReplaceId, true, false, true)
	}
	success := sys.owner.LearnSkill(skillId, newLv, false)
	if !success {
		return neterror.ParamsInvalidError("skill:%d learn failed", skillId)
	}
	data.MaxSkill[skillId] = newLv
	sys.SendProto3(6, 53, &pb3.S2C_6_53{Id: skillId, Lv: newLv, ReplaceId: conf.ReplaceId})
	recordLv := sys.owner.GetTaskRecord(custom_id.QttGongFaLv, []uint32{conf.Group})
	sys.owner.TriggerQuestEvent(custom_id.QttGongFaLv, conf.Group, int64(utils.MaxUInt32(recordLv, newLv)))
	sys.owner.TriggerQuestEventRange(custom_id.QttActorGongFaTotalLv)
	return nil
}

func handleGongFaAfterUseItemQuickUpLvAndQuest(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys); ok && sys.IsOpen() {
		conf := jsondata.GetGongFaListConfByJob(sys.owner.GetJob())
		if nil == conf {
			return
		}
		data := sys.GetData()
		for _, skillConf := range conf {
			if skillConf.Group == 0 {
				continue
			}
			if !sys.checkLearnSkillCond(skillConf) {
				continue
			}
			skillId := skillConf.SkillId
			newLv := sys.owner.GetSkillLv(skillId) + 1
			if newLv != 1 {
				continue
			}
			if skillConf.ReplaceId > 0 {
				sys.owner.ForgetSkill(skillConf.ReplaceId, true, false, true)
			}
			success := sys.owner.LearnSkill(skillId, newLv, false)
			if !success {
				continue
			}
			data.MaxSkill[skillId] = newLv
			sys.SendProto3(6, 53, &pb3.S2C_6_53{Id: skillId, Lv: newLv, ReplaceId: skillConf.ReplaceId})
			recordLv := sys.owner.GetTaskRecord(custom_id.QttGongFaLv, []uint32{skillConf.Group})
			sys.owner.TriggerQuestEvent(custom_id.QttGongFaLv, skillConf.Group, int64(utils.MaxUInt32(recordLv, newLv)))
			sys.owner.TriggerQuestEventRange(custom_id.QttActorGongFaTotalLv)
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiGongFaSys, func() iface.ISystem {
		return &GongFaSys{}
	})
	net.RegisterSysProto(6, 53, sysdef.SiGongFaSys, (*GongFaSys).c2sLevelUp)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys); ok && sys.IsOpen() {
			sys.checkProgressOpen()
		}
	})
	event.RegActorEvent(custom_id.AeCircleChange, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys); ok && sys.IsOpen() {
			sys.checkProgressOpen()
		}
	})
	event.RegActorEvent(custom_id.AeAfterUseItemQuickUpLvAndQuest, handleGongFaAfterUseItemQuickUpLvAndQuest)
}
