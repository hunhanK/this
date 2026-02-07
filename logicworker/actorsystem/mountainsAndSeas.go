package actorsystem

import (
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type MountainsAndSeasSys struct {
	Base
}

func (s *MountainsAndSeasSys) OnInit() {
	binary := s.GetBinaryData()
	if nil == binary.GetMountainsAndSeas() {
		binary.MountainsAndSeas = &pb3.MountainsAndSeas{}
	}
	if nil == binary.MountainsAndSeas.GetData() {
		binary.MountainsAndSeas.Data = make(map[uint32]uint32)
	}
}

func (s *MountainsAndSeasSys) GetData() *pb3.MountainsAndSeas {
	binary := s.GetBinaryData()
	if nil == binary.GetMountainsAndSeas() {
		binary.MountainsAndSeas = &pb3.MountainsAndSeas{}
	}
	mas := binary.MountainsAndSeas
	if nil == mas.Data {
		mas.Data = make(map[uint32]uint32)
	}
	if nil == mas.Level {
		mas.Level = make(map[uint32]uint32)
	}
	if nil == mas.Bonds {
		mas.Bonds = make(map[uint32]bool)
	}
	return binary.MountainsAndSeas
}

func (s *MountainsAndSeasSys) OnOpen() {
	s.S2CInfo()
}

func (s *MountainsAndSeasSys) S2CInfo() {
	s.SendProto3(42, 0, &pb3.S2C_42_0{Data: s.GetData()})
}

func (s *MountainsAndSeasSys) OnLogin() {
}

func (s *MountainsAndSeasSys) OnAfterLogin() {
	s.S2CInfo()
}

func (s *MountainsAndSeasSys) OnReconnect() {
	s.S2CInfo()
}

func (s *MountainsAndSeasSys) c2sStarUp(msg *base.Message) error {
	var req pb3.C2S_42_1
	if err := pb3.Unmarshal(msg.Data, &req); err != nil {
		return err
	}

	id := req.GetId()
	conf := jsondata.GetMountainsAndSeasConf(id)
	if nil == conf {
		return neterror.ConfNotFoundError("no conf(%d) mountainsandsea", id)
	}
	data := s.GetData()

	star, isNotActive := data.Data[id]
	if !isNotActive {
		if !s.owner.ConsumeByConf([]*jsondata.Consume{{Id: conf.ActiveCos, Count: conf.ActiveCount}}, false, common.ConsumeParams{LogId: pb3.LogId_LogMountainsAndSeasActive}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		data.Data[id], data.Level[id] = 0, 1
		s.SendProto3(42, 2, &pb3.S2C_42_2{Id: id, Lv: data.Level[id]})
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogMountainsAndSeasActive, &pb3.LogPlayerCounter{NumArgs: uint64(id)})
		s.GetOwner().TriggerQuestEventRange(custom_id.QttMountainsAndSeasActive)
	} else {
		starConf := conf.GetMountainsAndSeasConfByStar(star + 1)
		if nil == starConf {
			return neterror.ParamsInvalidError("mountainsandseas is max star")
		}
		if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogMountainsAndSeasStarUp}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		data.Data[id] = star + 1
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogMountainsAndSeasStarUp, &pb3.LogPlayerCounter{
			StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
				"id":   id,
				"star": data.Data[id],
			}),
		})
	}

	if starConf := conf.GetMountainsAndSeasConfByStar(data.Data[id]); nil != starConf && starConf.SkillId > 0 {
		s.owner.LearnSkill(starConf.SkillId, starConf.SkillLv, true)
	}

	s.ResetSysAttr(attrdef.SaMountainsAndSeas)
	s.SendProto3(42, 1, &pb3.S2C_42_1{
		Id:   id,
		Star: data.Data[id],
	})
	return nil
}

func (s *MountainsAndSeasSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_42_2
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return err
	}

	id := req.GetId()
	conf := jsondata.GetMountainsAndSeasConf(id)
	if nil == conf {
		return neterror.ConfNotFoundError("no conf(%d) mountainsandsea", id)
	}

	data := s.GetData()

	if _, isNotActive := data.Data[id]; !isNotActive {
		return neterror.ParamsInvalidError("mountainsandsea(%d) not active", id)
	}

	lvConf := conf.GetMountainsAndSeasConfByLv(data.Level[id] + 1)
	if nil == lvConf {
		return neterror.ParamsInvalidError("mountainsandseas is max lv")
	}

	if !s.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogMountainsAndSeasLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Level[id]++

	s.ResetSysAttr(attrdef.SaMountainsAndSeas)
	s.SendProto3(42, 2, &pb3.S2C_42_2{
		Id: id,
		Lv: data.Level[id],
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogMountainsAndSeasLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"id": id,
			"lv": data.Level[id],
		}),
	})
	return nil
}

func (s *MountainsAndSeasSys) c2sBondsActive(msg *base.Message) error {
	var req pb3.C2S_42_3
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return err
	}

	id := req.GetId()
	conf := jsondata.GetMountainsAndSeasBondsConf(id)
	if nil == conf {
		return neterror.ConfNotFoundError("no conf(%d) mountainsandsea bonds", id)
	}

	data := s.GetData()
	for _, idx := range conf.Ids {
		if _, ok := data.Data[idx]; !ok {
			return neterror.ConfNotFoundError("mountainsandsea bonds(%d) not complete", id)
		}
	}
	data.Bonds[id] = true

	s.ResetSysAttr(attrdef.SaMountainsAndSeasBonds)
	s.SendProto3(42, 3, &pb3.S2C_42_3{Id: id})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogMountainsAndSeasBondsActive, &pb3.LogPlayerCounter{NumArgs: uint64(id)})

	return nil
}

func calcSaMountainsAndSeasAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiMountainsAndSeas).(*MountainsAndSeasSys)
	if nil == sys || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	for id, star := range data.Data {
		conf := jsondata.GetMountainsAndSeasConf(id)
		if nil == conf {
			continue
		}
		//星级属性
		starConf := conf.GetMountainsAndSeasConfByStar(star)
		if nil == starConf {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, starConf.Attr)

		//等级属性
		lvConf := conf.GetMountainsAndSeasConfByLv(data.Level[id])
		if nil == lvConf {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, lvConf.Attr)
	}
}

func calcSaMountainsAndBondsAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiMountainsAndSeas).(*MountainsAndSeasSys)
	if nil == sys || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	for id := range data.Bonds {
		conf := jsondata.GetMountainsAndSeasBondsConf(id)
		if nil == conf {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, conf.Attr)
	}
}

// onMountainsAndSeasActive 累计激活多少个山海经
func onMountainsAndSeasActive(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiMountainsAndSeas).(*MountainsAndSeasSys)
	if !ok || !s.IsOpen() {
		return 0
	}

	data := s.GetData()

	return uint32(len(data.Data))
}

func init() {
	RegisterSysClass(sysdef.SiMountainsAndSeas, func() iface.ISystem {
		return &MountainsAndSeasSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaMountainsAndSeas, calcSaMountainsAndSeasAttr)
	engine.RegAttrCalcFn(attrdef.SaMountainsAndSeasBonds, calcSaMountainsAndBondsAttr)

	engine.RegQuestTargetProgress(custom_id.QttMountainsAndSeasActive, onMountainsAndSeasActive)

	net.RegisterSysProto(42, 1, sysdef.SiMountainsAndSeas, (*MountainsAndSeasSys).c2sStarUp)
	net.RegisterSysProto(42, 2, sysdef.SiMountainsAndSeas, (*MountainsAndSeasSys).c2sLvUp)
	net.RegisterSysProto(42, 3, sysdef.SiMountainsAndSeas, (*MountainsAndSeasSys).c2sBondsActive)
}
