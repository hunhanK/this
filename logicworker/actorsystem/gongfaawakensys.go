/**
 * @Author: LvYuMeng
 * @Date: 2024/9/12
 * @Desc: 功法觉醒
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type GongFaAwakenSys struct {
	Base
}

func (s *GongFaAwakenSys) OnOpen() {
	s.ResetSysAttr(attrdef.SaGongFaAwaken)
}

func (s *GongFaAwakenSys) OnAfterLogin() {
}

func (s *GongFaAwakenSys) OnReconnect() {
}

func (s *GongFaAwakenSys) getExtraInfoById(id uint32) *pb3.GongFaExtra {
	gongFaSys, ok := s.owner.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys)
	if !ok || !gongFaSys.IsOpen() {
		return nil
	}
	data := gongFaSys.GetData()
	if data.MaxSkill[id] <= 0 {
		return nil
	}
	if nil == data.ExtraInfo[id] {
		data.ExtraInfo[id] = &pb3.GongFaExtra{}
	}
	return data.ExtraInfo[id]
}

func (s *GongFaAwakenSys) c2sFlyAwaken(msg *base.Message) error {
	var req pb3.C2S_6_59
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	gongFaSys, ok := s.owner.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys)
	if !ok || !gongFaSys.IsOpen() {
		return neterror.ParamsInvalidError("gongfa sys not open")
	}
	data := gongFaSys.GetData()
	skillId := req.GetId()
	conf := jsondata.GetGongFaConfByJob(s.owner.GetJob(), skillId)
	if nil == conf {
		return neterror.ConfNotFoundError("gongfa conf %d is nil", skillId)
	}
	extraData := s.getExtraInfoById(skillId)
	if nil == extraData {
		return neterror.ParamsInvalidError("gongfa skill %d not active", skillId)
	}
	flyCamp := s.owner.GetExtraAttrU32(attrdef.FlyCamp)
	if flyCamp == 0 {
		return neterror.ParamsInvalidError("flyCamp not choose")
	}
	nextLv := extraData.AwakenLv + 1
	flyAwakenConf := conf.GetFlyAwakenConf(s.owner.GetExtraAttrU32(attrdef.FlyCamp), nextLv)
	if nil == flyAwakenConf {
		return neterror.ParamsInvalidError("gongfa %d fly awaken conf %d is nil", skillId, nextLv)
	}
	if flyAwakenConf.GongFaLv > data.MaxSkill[skillId] {
		return neterror.ParamsInvalidError("gongfa fly awaken cond gongfa lv not enough")
	}
	if !s.owner.ConsumeByConf(flyAwakenConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGongFlyAwaken}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	extraData.AwakenLv = nextLv
	if flyAwakenConf.SkillId > 0 {
		if !s.owner.LearnSkill(flyAwakenConf.SkillId, flyAwakenConf.SkillLv, true) {
			s.LogError("skill %d learn failed", flyAwakenConf.SkillId)
		}
	}
	s.ResetSysAttr(attrdef.SaGongFaAwaken)
	s.SendProto3(6, 59, &pb3.S2C_6_59{
		Id:   skillId,
		Info: extraData,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogGongFlyAwaken, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"skillId":  skillId,
			"awakenLv": extraData.AwakenLv,
		}),
	})
	return nil
}

func (s *GongFaAwakenSys) onFlyCampChange() {
	gongFaSys, ok := s.owner.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys)
	if !ok || !gongFaSys.IsOpen() {
		return
	}
	data := gongFaSys.GetData()
	flyCamp := s.owner.GetExtraAttrU32(attrdef.FlyCamp)
	if flyCamp == 0 {
		return
	}
	for skillId, extraInfo := range data.ExtraInfo {
		if nil == extraInfo {
			continue
		}
		conf := jsondata.GetGongFaConfByJob(s.owner.GetJob(), skillId)
		if nil == conf {
			continue
		}
		for _, flyAwakenConf := range conf.FlyAwake {
			if flyAwakenConf.FlyCamp != flyCamp {
				s.owner.ForgetSkill(flyAwakenConf.SkillId, true, true, true)
			} else {
				s.owner.LearnSkill(flyAwakenConf.SkillId, flyAwakenConf.SkillLv, true)
			}
		}
	}

	s.ResetSysAttr(attrdef.SaGongFaAwaken)
}

func (s *GongFaAwakenSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	gongfaSys, ok := s.owner.GetSysObj(sysdef.SiGongFaSys).(*GongFaSys)
	if !ok || !gongfaSys.IsOpen() {
		return
	}
	data := gongfaSys.GetData()
	flyCamp := gongfaSys.owner.GetExtraAttrU32(attrdef.FlyCamp)
	if flyCamp == 0 {
		return
	}
	for skillId, extraInfo := range data.ExtraInfo {
		if nil == extraInfo {
			continue
		}
		conf := jsondata.GetGongFaConfByJob(s.owner.GetJob(), skillId)
		if nil == conf {
			continue
		}

		flyAwakenConf := conf.GetFlyAwakenConf(flyCamp, extraInfo.AwakenLv)
		if nil == flyAwakenConf {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, flyAwakenConf.Attrs)
	}
}

func onGongFaAwakenFlyCampChange(player iface.IPlayer, args ...interface{}) {
	awakenSys := player.GetSysObj(sysdef.SiGongFaAwaken).(*GongFaAwakenSys)
	if nil == awakenSys || !awakenSys.IsOpen() {
		return
	}
	awakenSys.onFlyCampChange()
}

func calcGongFaAwakenSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	awakenSys, ok := player.GetSysObj(sysdef.SiGongFaAwaken).(*GongFaAwakenSys)
	if !ok || !awakenSys.IsOpen() {
		return
	}
	awakenSys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiGongFaAwaken, func() iface.ISystem {
		return &GongFaAwakenSys{}
	})

	net.RegisterSysProtoV2(6, 59, sysdef.SiGongFaAwaken, func(s iface.ISystem) func(*base.Message) error {
		return s.(*GongFaAwakenSys).c2sFlyAwaken
	})

	event.RegActorEvent(custom_id.AeFlyCampChange, onGongFaAwakenFlyCampChange)

	engine.RegAttrCalcFn(attrdef.SaGongFaAwaken, calcGongFaAwakenSysAttr)

	gmevent.Register("setFlyCamp", func(player iface.IPlayer, args ...string) bool {
		camp := utils.AtoInt64(args[0])
		player.SetExtraAttr(attrdef.FlyCamp, camp)
		player.TriggerEvent(custom_id.AeFlyCampChange)
		return true
	}, 1)
}
