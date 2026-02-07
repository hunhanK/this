/**
 * @Author: lzp
 * @Date: 2025/3/26
 * @Desc: 源神系统
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type SourceGodSys struct {
	Base
}

func (s *SourceGodSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SourceGodSys) OnLogin() {
	s.s2cInfo()
}

func (s *SourceGodSys) OnOpen() {
	s.s2cInfo()
}

func (s *SourceGodSys) GetData() map[uint32]*pb3.SourceGod {
	binary := s.GetBinaryData()
	if binary.SourceGodData == nil {
		binary.SourceGodData = make(map[uint32]*pb3.SourceGod)
	}
	return binary.SourceGodData
}

func (s *SourceGodSys) s2cInfo() {
	s.SendProto3(27, 87, &pb3.S2C_27_87{Data: s.GetData()})
}

func (s *SourceGodSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_27_85
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetSourceGodStarConf(req.Id, 0)
	if conf == nil {
		return neterror.ConfNotFoundError("sourceGod=%d not found", req.Id)
	}

	data := s.GetData()
	_, ok := data[req.Id]
	if ok {
		return neterror.ParamsInvalidError("sourceGod=%d has activated", req.Id)
	}

	if !s.GetOwner().ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSourceGodUpStar}) {
		return neterror.ParamsInvalidError("sourceGod active consume not enough: %v", req.Id)
	}

	data[req.Id] = &pb3.SourceGod{Id: req.Id, Star: 0}
	s.onStarChange(req.Id)
	s.SendProto3(27, 85, &pb3.S2C_27_85{Id: req.Id})
	return nil
}

func (s *SourceGodSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_27_86
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	iData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("sourceGod not activated: %d", req.Id)
	}

	nextStar := iData.Star + 1
	conf := jsondata.GetSourceGodStarConf(req.Id, nextStar)
	if conf == nil {
		return neterror.ConfNotFoundError("sourceGod star conf not found: %d, %d", req.Id, nextStar)
	}

	if !s.GetOwner().ConsumeByConf(conf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogSourceGodUpStar}) {
		return neterror.ParamsInvalidError("sourceGod star consume not enough: %v", req.Id)
	}

	iData.Star = nextStar
	s.onStarChange(req.Id)
	s.SendProto3(27, 86, &pb3.S2C_27_86{Id: req.Id, Star: nextStar})
	return nil
}

func (s *SourceGodSys) c2sActivePassiveSkill(msg *base.Message) error {
	var req pb3.C2S_27_90
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	iData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("sourceGod not activated: %d", req.Id)
	}

	sConf, _ := jsondata.GetSourceGodPassiveSkillConf(req.Id, req.SkillId)
	if sConf == nil {
		return neterror.ConfNotFoundError("id:%d, skillId:%d config not found", req.Id, req.SkillId)
	}
	if iData.Star < sConf.Star {
		return neterror.ParamsInvalidError("id:%d, skillId:%d star limit", req.Id, req.SkillId)
	}

	iData.PassiveSkillIds = append(iData.PassiveSkillIds, req.SkillId)

	s.SendProto3(27, 90, &pb3.S2C_27_90{Id: req.Id, SkillId: req.SkillId})
	return nil
}

func (s *SourceGodSys) onStarChange(id uint32) {
	data := s.GetData()
	iData, ok := data[id]
	if !ok {
		return
	}
	conf := jsondata.GetSourceGodStarConf(id, iData.Star)
	if conf == nil {
		return
	}
	s.ResetSysAttr(attrdef.SaSourceGod)
}

func (s *SourceGodSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for _, v := range data {
		conf := jsondata.GetSourceGodStarConf(v.Id, v.Star)
		if conf == nil {
			continue
		}
		for _, passiveSkill := range conf.PassiveSkill {
			if passiveSkill.Target == 1 {
				s.owner.LearnSkill(passiveSkill.SkillId, passiveSkill.SkillLv, true)
			}
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, conf.Attrs)
	}
}

func (s *SourceGodSys) GetSourceGodSkills(id uint32) map[uint32]*pb3.SkillInfo {
	data := s.GetData()
	sData, ok := data[id]
	if !ok {
		return nil
	}

	starConf := jsondata.GetSourceGodStarConf(sData.Id, sData.Star)
	if starConf == nil {
		return nil
	}

	skillInfo := make(map[uint32]*pb3.SkillInfo)
	if starConf.Attack > 0 {
		skillInfo[starConf.Attack] = &pb3.SkillInfo{
			Id:    starConf.Attack,
			Level: 1,
		}
	}
	if starConf.ActiveSkillId > 0 {
		skillInfo[starConf.ActiveSkillId] = &pb3.SkillInfo{
			Id:    starConf.ActiveSkillId,
			Level: starConf.ActiveSkillLv,
		}
	}

	for _, skillId := range sData.PassiveSkillIds {
		_, pConf := jsondata.GetSourceGodPassiveSkillConf(id, skillId)
		if pConf == nil {
			continue
		}
		if pConf.Target == 0 {
			skillInfo[pConf.SkillId] = &pb3.SkillInfo{
				Id:    pConf.SkillId,
				Level: pConf.SkillLv,
			}
		}
	}
	return skillInfo
}

func sourceGodProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiSourceGod).(*SourceGodSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiSourceGod, func() iface.ISystem {
		return &SourceGodSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSourceGod, sourceGodProperty)

	net.RegisterSysProto(27, 85, sysdef.SiSourceGod, (*SourceGodSys).c2sActivate)
	net.RegisterSysProto(27, 86, sysdef.SiSourceGod, (*SourceGodSys).c2sUpStar)
	net.RegisterSysProto(27, 90, sysdef.SiSourceGod, (*SourceGodSys).c2sActivePassiveSkill)
}
