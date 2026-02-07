/**
 * @Author: lzp
 * @Date: 2025/8/7
 * @Desc:
**/

package actorsystem

import (
	log "github.com/gzjjyz/logger"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type MeChaSys struct {
	Base
}

func (s *MeChaSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MeChaSys) OnLogin() {
	s.s2cInfo()
}

func (s *MeChaSys) OnOpen() {
	s.s2cInfo()
}

func (s *MeChaSys) GetData() map[uint32]*pb3.MeCha {
	binary := s.GetBinaryData()
	if binary.MeChaData == nil {
		binary.MeChaData = make(map[uint32]*pb3.MeCha)
	}
	return binary.MeChaData
}

func (s *MeChaSys) s2cInfo() {
	s.SendProto3(11, 145, &pb3.S2C_11_145{Data: s.GetData()})
}

func (s *MeChaSys) c2sActivate(msg *base.Message) error {
	var req pb3.C2S_11_146
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetMeChaStarConf(req.Id, 0)
	if conf == nil {
		return neterror.ConfNotFoundError("meCha id: %d config not found", req.Id)
	}

	data := s.GetData()
	if _, ok := data[req.Id]; ok {
		return neterror.ParamsInvalidError("meCha id:%d has activated", req.Id)
	}

	if !s.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogMeChaActivate}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data[req.Id] = &pb3.MeCha{Id: req.Id}
	s.onStarChange(req.Id)
	s.SendProto3(11, 146, &pb3.S2C_11_146{Id: req.Id})
	return nil
}

func (s *MeChaSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_11_147
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	cData, ok := data[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("meCha id:%d not activated", req.Id)
	}

	nextStar := cData.Star + 1
	conf := jsondata.GetMeChaStarConf(req.Id, nextStar)
	if conf == nil {
		return neterror.ConfNotFoundError("meCha id:%d star:%d config not found", req.Id, nextStar)
	}

	if !s.owner.ConsumeByConf(conf.Consume, req.AutoBuy, common.ConsumeParams{
		LogId: pb3.LogId_LogMeChaUpStar,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}

	cData.Star = nextStar
	s.onStarChange(req.Id)
	s.SendProto3(11, 147, &pb3.S2C_11_147{
		Id:   req.Id,
		Star: nextStar,
	})
	return nil
}

func (s *MeChaSys) onStarChange(id uint32) {
	s.ResetSysAttr(attrdef.SaMeCha)

	data := s.GetData()
	cData, ok := data[id]
	if !ok {
		return
	}

	err := s.owner.CallActorFunc(actorfuncid.G2FBattleMeChaToStar, &pb3.CommonSt{U32Param: id, U32Param2: cData.Star})
	if err != nil {
		log.LogError("err: %v", err)
	}

	immortalSys, ok := s.owner.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !immortalSys.IsOpen() {
		return
	}
	if immortalSys.IsBattleMeChaOnBattle(id) {
		starConf := jsondata.GetMeChaStarConf(id, cData.Star)
		if starConf == nil || len(starConf.SkillId) < 2 {
			return
		}
		s.owner.LearnSkill(starConf.SkillId[0], starConf.SkillId[1], true)
	}
}

func (s *MeChaSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for _, v := range data {
		conf := jsondata.GetMeChaStarConf(v.Id, v.Star)
		if conf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, conf.Attrs)
	}
}

func (s *MeChaSys) learnMeChaSkill(id uint32) {
	data := s.GetData()
	cData, ok := data[id]
	if !ok {
		return
	}
	starConf := jsondata.GetMeChaStarConf(id, cData.Star)
	if starConf == nil || len(starConf.SkillId) < 2 {
		return
	}
	s.owner.LearnSkill(starConf.SkillId[0], starConf.SkillId[1], true)
}

func (s *MeChaSys) forgetMeChaSkill(id uint32) {
	data := s.GetData()
	cData, ok := data[id]
	if !ok {
		return
	}
	starConf := jsondata.GetMeChaStarConf(id, cData.Star)
	if starConf == nil || len(starConf.SkillId) < 2 {
		return
	}
	s.owner.ForgetSkill(starConf.SkillId[0], true, true, true)
}

func meChaProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiMeCha).(*MeChaSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func onBattleSoulSlotMeCha(player iface.IPlayer, buf []byte) {
	msg := &pb3.SyncBattleSoulSlotMeCha{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiMeCha).(*MeChaSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if msg.NewTakeOffId > 0 {
		sys.forgetMeChaSkill(msg.NewTakeOffId)
	}
	if msg.NewTakeId > 0 {
		sys.learnMeChaSkill(msg.NewTakeId)
	}

	sys.owner.TriggerEvent(custom_id.AeMeChaBattle, msg)
}

func init() {
	RegisterSysClass(sysdef.SiMeCha, func() iface.ISystem {
		return &MeChaSys{}
	})

	net.RegisterSysProto(11, 146, sysdef.SiMeCha, (*MeChaSys).c2sActivate)
	net.RegisterSysProto(11, 147, sysdef.SiMeCha, (*MeChaSys).c2sUpStar)

	engine.RegAttrCalcFn(attrdef.SaMeCha, meChaProperty)

	engine.RegisterActorCallFunc(playerfuncid.SyncBattleSoulSlotMeCha, onBattleSoulSlotMeCha)
}
