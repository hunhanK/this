/**
 * @Author: lzp
 * @Date: 2024/9/9
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

const (
	LGroup = 1
	RGroup = 2
)

type TalentSys struct {
	Base
}

func (sys *TalentSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *TalentSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *TalentSys) OnOpen() {
	num := jsondata.GetCommonConf("initTalentPoint").U32
	sys.addSkillPoint(num)
	sys.s2cInfo()
}

func (sys *TalentSys) GetData() *pb3.TalentData {
	binary := sys.GetBinaryData()
	if binary.TalentData == nil {
		binary.TalentData = &pb3.TalentData{}
	}
	tData := binary.GetTalentData()
	if tData.Talent == nil {
		tData.Talent = make(map[uint32]uint32)
	}
	return tData
}

func (sys *TalentSys) s2cInfo() {
	data := sys.GetData()
	sys.owner.SendProto3(6, 70, &pb3.S2C_6_70{
		RetSkillPoint: data.SkillPoint - data.UsedSkillPoint,
		TalentData:    data.Talent,
	})
}

func (sys *TalentSys) addSkillPoint(addPoint uint32) {
	data := sys.GetData()
	data.SkillPoint += addPoint
}

func (sys *TalentSys) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_6_71
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	skillId := req.Id
	conf := jsondata.GetTalentConf(skillId)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found id = %d", skillId)
	}

	data := sys.GetData()
	if !sys.check(conf) {
		return neterror.ParamsInvalidError("cond limit id = %d", skillId)
	}

	lv := data.Talent[skillId]
	if lv >= uint32(len(conf.Level)) {
		return neterror.ParamsInvalidError("lv is max id = %d", skillId)
	}

	lConf := conf.Level[lv]
	if data.SkillPoint-data.UsedSkillPoint < lConf.NeedSkillPoint {
		return neterror.ParamsInvalidError("skill point is empty id = %d", skillId)
	}

	consume := lConf.Consume
	if len(consume) > 0 && !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogTalentLevelUp,
	}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	newLv := data.Talent[skillId] + 1
	if !sys.owner.LearnSkill(skillId, newLv, false) {
		return neterror.ParamsInvalidError("lean failed id = %d", skillId)
	}
	data.UsedSkillPoint += lConf.NeedSkillPoint
	data.Talent[skillId] = newLv

	sys.SendProto3(6, 71, &pb3.S2C_6_71{
		Id: skillId,
		Lv: newLv,
	})
	sys.s2cInfo()

	return nil
}

func (sys *TalentSys) check(conf *jsondata.TalentConf) bool {
	for _, preSkill := range conf.SkillLimit {
		var point uint32
		switch preSkill.Group {
		case LGroup:
			point = sys.getGroupPoint(conf.Type, LGroup)
		case RGroup:
			point = sys.getGroupPoint(conf.Type, RGroup)
		default:
			if preSkill.SkillId > 0 {
				point = sys.getSkillPoint(preSkill.SkillId)
			} else {
				point = sys.getTypePoint(conf.Type)
			}
		}
		if point < preSkill.SkillNum {
			return false
		}
	}
	return true
}

func (sys *TalentSys) getSkillPoint(id uint32) uint32 {
	data := sys.GetData()
	return data.Talent[id]
}

func (sys *TalentSys) getTypePoint(tp uint32) uint32 {
	data := sys.GetData()
	var point uint32
	for skillId, num := range data.Talent {
		conf := jsondata.GetTalentConf(skillId)
		if conf.Type != tp {
			continue
		}
		var lv uint32
		for lv = 1; lv <= num; lv++ {
			point += jsondata.GetTalentPointBySkillLv(skillId, lv)
		}
	}
	return point
}

func (sys *TalentSys) getGroupPoint(tp, group uint32) uint32 {
	data := sys.GetData()
	var point uint32
	for skillId, num := range data.Talent {
		conf := jsondata.GetTalentConf(skillId)
		if conf.Type == tp && conf.Group == group {
			point += num
		}
	}
	return point
}

func (sys *TalentSys) c2sReset(msg *base.Message) error {
	var req pb3.C2S_6_72
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	tp := req.Type
	conf := jsondata.GetTalentTypeConf(tp)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found type = %d", tp)
	}

	consume := conf.Consume
	if len(consume) > 0 && !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogTalentReset,
	}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data := sys.GetData()
	point := sys.getTypePoint(tp)
	sys.addSkillPoint(point)

	var skillIds []uint32
	for skillId := range data.Talent {
		sConf := jsondata.GetTalentConf(skillId)
		if sConf == nil {
			continue
		}
		if sConf.Type == tp {
			skillIds = append(skillIds, skillId)
		}
	}
	for _, skillId := range skillIds {
		data.Talent[skillId] = 0
		sys.owner.ForgetSkill(skillId, true, false, true)
	}

	sys.SendProto3(6, 72, &pb3.S2C_6_72{Success: true})
	sys.s2cInfo()
	return nil
}

func talentOnPlayerLvUp(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiTalent).(*TalentSys)
	if !ok || !sys.IsOpen() {
		return
	}
	if len(args) < 2 {
		return
	}
	oldLv, ok := args[0].(uint32)
	if !ok {
		return
	}
	newLv, ok := args[1].(uint32)
	if !ok {
		return
	}
	level := jsondata.GetSysOpenConf(sysdef.SiTalent).Level
	addPoint := newLv - utils.MaxUInt32(oldLv, level)
	sys.addSkillPoint(addPoint)
	sys.s2cInfo()
}

func talentOnFlyCampFinish(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiTalent).(*TalentSys)
	if !ok || !sys.IsOpen() {
		return
	}
	camp := player.GetFlyCamp()
	conf := jsondata.GetFlyCampConf(camp)
	if conf == nil {
		return
	}
	sys.addSkillPoint(conf.TalentPoint)
	sys.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiTalent, func() iface.ISystem {
		return &TalentSys{}
	})
	net.RegisterSysProto(6, 71, sysdef.SiTalent, (*TalentSys).c2sLevelUp)
	net.RegisterSysProto(6, 72, sysdef.SiTalent, (*TalentSys).c2sReset)

	event.RegActorEvent(custom_id.AeLevelUp, talentOnPlayerLvUp)
	event.RegActorEvent(custom_id.AeFlyCampFinishChallenge, talentOnFlyCampFinish)

	gmevent.Register("addSkillPoint", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sys, ok := player.GetSysObj(sysdef.SiTalent).(*TalentSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		addPoint := utils.AtoUint32(args[0])
		sys.addSkillPoint(addPoint)
		sys.s2cInfo()
		return true
	}, 1)
}
