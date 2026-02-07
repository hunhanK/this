package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type RolePowerWelfareSys struct {
	Base
}

func (s *RolePowerWelfareSys) s2cInfo() {
	s.SendProto3(2, 247, &pb3.S2C_2_247{
		Data: s.getData(),
	})
}

func (s *RolePowerWelfareSys) getData() *pb3.RolePowerWelfareData {
	data := s.GetBinaryData().RolePowerWelfareData
	if data == nil {
		s.GetBinaryData().RolePowerWelfareData = &pb3.RolePowerWelfareData{}
		data = s.GetBinaryData().RolePowerWelfareData
	}

	if data.DayMissions == nil {
		data.DayMissions = make(map[uint32]*pb3.RolePowerWelfareMission)
	}
	return data
}

func (s *RolePowerWelfareSys) OnReconnect() {
	s.s2cInfo()
}

func (s *RolePowerWelfareSys) OnLogin() {
	s.s2cInfo()
}

func (s *RolePowerWelfareSys) OnOpen() {
	s.s2cInfo()
}

func (s *RolePowerWelfareSys) c2sRecAwards(msg *base.Message) error {
	var req pb3.C2S_2_248
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	config := jsondata.GetRolePowerWelfareConfig()
	if config == nil {
		return neterror.ConfNotFoundError("RolePowerWelfareConfig not found")
	}
	missionConf := config.Missions[req.Id]
	if missionConf == nil {
		return neterror.ParamsInvalidError("%d config not found", req.Id)
	}
	data := s.getData()
	mission, exist := data.DayMissions[missionConf.Day]
	if !exist {
		mission = &pb3.RolePowerWelfareMission{
			Day:        missionConf.Day,
			MissionRec: make(map[uint32]bool),
		}
		data.DayMissions[missionConf.Day] = mission
	}
	if mission.MissionRec == nil {
		mission.MissionRec = make(map[uint32]bool)
	}
	if mission.MissionRec[req.Id] {
		return neterror.ParamsInvalidError("%d already rec", req.Id)
	}
	if !CheckReach(s.owner, missionConf.MissionType, missionConf.TargetVal) {
		return neterror.ParamsInvalidError("reach not ok")
	}
	data.DayMissions[missionConf.Day].MissionRec[req.Id] = true
	s.SendProto3(2, 248, &pb3.S2C_2_248{
		Id: req.Id,
	})
	engine.GiveRewards(s.owner, missionConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogRolePowerWelfareSysRecAwards,
	})
	s.s2cInfo()
	s.owner.TriggerQuestEvent(custom_id.QttSevenDayReachStandardXTabYQuest, missionConf.Day, 1)
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiRolePowerWelfare, func() iface.ISystem {
		return &RolePowerWelfareSys{}
	})

	net.RegisterSysProtoV2(2, 248, sysdef.SiRolePowerWelfare, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RolePowerWelfareSys).c2sRecAwards
	})
}
