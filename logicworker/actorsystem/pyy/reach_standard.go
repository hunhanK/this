package pyy

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/reachconddef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type PlayerYYReachStandard struct {
	PlayerYYBase
}

func (s *PlayerYYReachStandard) Login() {
	s.sendInfo()
}

func (s *PlayerYYReachStandard) OnOpen() {
	s.sendInfo()
}

func (s *PlayerYYReachStandard) OnEnd() {
	if !jsondata.NeedPYYReachStandardSendMail(s.ConfName, s.ConfIdx) {
		return
	}

	confs := s.GetMissionConfs()

	rewardVecs := []jsondata.StdRewardVec{}
	for k, conf := range confs {

		if ok := s.GetData().Status[k]; ok {
			continue
		}

		if !actorsystem.CheckReach(s.GetPlayer(), conf.Type, conf.TargetVal) {
			continue
		}

		rewardVecs = append(rewardVecs, conf.Rewards)

		s.GetData().Status[k] = true
	}

	rewards := jsondata.MergeStdReward(rewardVecs...)

	if len(rewards) == 0 {
		return
	}

	var name string
	if actconf := jsondata.GetPlayerYYConf(s.GetId()); actconf != nil {
		name = actconf.Name
	}

	s.GetPlayer().SendMail(&mailargs.SendMailSt{
		ConfId: common.Mail_ReachStandard,
		Content: &mailargs.ReachStandardArgs{
			Name: name,
		},
		Rewards: rewards,
	})
}

func (s *PlayerYYReachStandard) sendInfo() {
	s.SendProto3(140, 0, &pb3.S2C_140_0{
		Info:     s.GetData(),
		ActiveId: s.GetId(),
	})
}

func (s *PlayerYYReachStandard) GetData() *pb3.PYY_ReachStandardData {
	if s.GetYYData().ReachStandardData == nil {
		s.GetYYData().ReachStandardData = make(map[uint32]*pb3.PYY_ReachStandardData)
	}

	data, ok := s.GetYYData().ReachStandardData[s.GetId()]
	if !ok {
		data = &pb3.PYY_ReachStandardData{
			Status: make(map[uint32]bool),
		}

		s.GetYYData().ReachStandardData[s.GetId()] = data
	}

	if data.Status == nil {
		data.Status = make(map[uint32]bool)
	}

	if data.Progress == nil {
		data.Progress = make(map[uint32]uint32)
	}
	return data
}

func (s *PlayerYYReachStandard) ResetData() {
	if s.GetYYData().ReachStandardData == nil {
		return
	}
	delete(s.GetYYData().ReachStandardData, s.Id)
}

func (s *PlayerYYReachStandard) OnReconnect() {
	s.sendInfo()
}

func (s *PlayerYYReachStandard) GetMissionConf(missionId uint32) *jsondata.PYYReachStandardMission {
	return jsondata.GetPYYReachStandardConf(s.ConfName, s.GetConfIdx(), missionId)
}

func (s *PlayerYYReachStandard) GetMissionConfs() map[uint32]*jsondata.PYYReachStandardMission {
	return jsondata.GetPYYReachStandardConfs(s.ConfName, s.GetConfIdx())
}

func (s *PlayerYYReachStandard) c2sReward(msg *base.Message) error {
	var req pb3.C2S_140_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := s.GetMissionConf(req.MissionId)
	if conf == nil {
		return neterror.ConfNotFoundError("confIdx %d missionId %d", s.GetConfIdx(), req.MissionId)
	}

	data := s.GetData()
	if conf.Type == reachconddef.ReachStandard_CountProgress { //这里特殊处理先 （不通用）
		if len(conf.TargetVal) < 2 || data.Progress[req.MissionId] < uint32(conf.TargetVal[1]) {
			return neterror.ParamsInvalidError("progress is not enough confIdx %d missionId %d", s.GetConfIdx(), req.MissionId)
		}
	} else {
		if !actorsystem.CheckReach(s.GetPlayer(), conf.Type, conf.TargetVal) {
			return neterror.ParamsInvalidError("check val is false confIdx %d missionId %d", s.GetConfIdx(), req.MissionId)
		}
	}

	if ok := s.GetData().Status[req.MissionId]; ok {
		return neterror.ParamsInvalidError("already reawarded for ConfIdx %d id %d", s.GetConfIdx(), req.MissionId)
	}

	data.Status[req.MissionId] = true

	engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlayerYYReachStandardReward,
	})

	logArgs := map[string]any{
		"configName": s.ConfName,
		"activityId": s.GetId(),
		"confIdx":    s.GetConfIdx(),
		"missionId":  req.MissionId,
	}

	logArgByte, _ := json.Marshal(logArgs)
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlayerYYReachStandardReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArgByte),
	})

	s.SendProto3(140, 1, &pb3.S2C_140_1{
		MissionId: req.MissionId,
		State:     true,
		ActiveId:  s.GetId(),
	})

	return nil
}

func (s *PlayerYYReachStandard) handleEvent(event *custom_id.ActReachStandardEvent) {
	data := s.GetData()
	confs := s.GetMissionConfs()

	if pie.Uint32s(data.Ext).Contains(event.Val) {
		return
	}

	var add bool
	for _, conf := range confs {
		if conf.Type != event.ReachType {
			continue
		}
		if len(conf.TargetVal) < 2 {
			continue
		}

		key := conf.TargetVal[0]
		if key != int64(event.Key) {
			continue
		}

		data.Progress[conf.Id]++
		add = true
	}

	if add {
		data.Ext = append(data.Ext, event.Val)
		s.sendInfo()
	}
}

func handleReachStandardQuest(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	reachEvent, ok := args[0].(*custom_id.ActReachStandardEvent)
	if !ok {
		return
	}

	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiReachStandard)
	for _, obj := range yyList {
		if s, ok := obj.(*PlayerYYReachStandard); ok && s.IsOpen() {
			s.handleEvent(reachEvent)
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiReachStandard, func() iface.IPlayerYY {
		return &PlayerYYReachStandard{}
	})

	event.RegActorEvent(custom_id.AeReachStandardQuest, handleReachStandardQuest)

	net.RegisterYYSysProto(140, 1, (*PlayerYYReachStandard).c2sReward)
}
