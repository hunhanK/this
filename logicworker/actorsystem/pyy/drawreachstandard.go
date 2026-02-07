package pyy

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

// 和达标活动有点冗余

// PlayerYYActivityReachStandard 抽奖达标
type PlayerYYActivityReachStandard struct {
	PlayerYYBase
}

func (s *PlayerYYActivityReachStandard) Login() {
	s.sendInfo()
}

func (s *PlayerYYActivityReachStandard) GetData() *pb3.PYYActivityReachStandard {
	if s.GetYYData().ActivityReachStandard == nil {
		s.GetYYData().ActivityReachStandard = make(map[uint32]*pb3.PYYActivityReachStandard)
	}

	data, ok := s.GetYYData().ActivityReachStandard[s.GetId()]
	if !ok {
		data = &pb3.PYYActivityReachStandard{}
		s.GetYYData().ActivityReachStandard[s.GetId()] = data
	}

	if data.Rev == nil {
		data.Rev = make(map[uint32]bool)
	}

	if data.MissionTimes == nil {
		data.MissionTimes = make(map[uint32]uint32)
	}

	return data
}

func (s *PlayerYYActivityReachStandard) OnOpen() {
	s.sendInfo()
}

func (s *PlayerYYActivityReachStandard) ResetData() {
	if s.GetYYData().ActivityReachStandard != nil {
		delete(s.GetYYData().ActivityReachStandard, s.GetId())
	}
}

func (s *PlayerYYActivityReachStandard) OnEnd() {
	confs := s.GetMissionConfs()
	data := s.GetData()
	var rewardVecs []jsondata.StdRewardVec
	for k, conf := range confs {

		if ok := s.GetData().Rev[k]; ok {
			continue
		}

		if len(conf.TargetVal) != 2 {
			continue
		}
		pyyId := conf.TargetVal[0]
		times := conf.TargetVal[1]
		if data.MissionTimes[uint32(pyyId)] < uint32(times) {
			continue
		}

		rewardVecs = append(rewardVecs, conf.Rewards)

		s.GetData().Rev[k] = true
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
		ConfId: common.Mail_ActivityReachStandard,
		Content: &mailargs.ReachStandardArgs{
			Name: name,
		},
		Rewards: rewards,
	})
}

func (s *PlayerYYActivityReachStandard) sendInfo() {
	s.SendProto3(27, 110, &pb3.S2C_27_110{
		Data:     s.GetData(),
		ActiveId: s.GetId(),
	})
}

func (s *PlayerYYActivityReachStandard) OnReconnect() {
	s.sendInfo()
}

func (s *PlayerYYActivityReachStandard) GetMissionConf(missionId uint32) *jsondata.PYYActivityReachStandardMission {
	return jsondata.GetPYYActivityReachStandardConf(s.ConfName, s.GetConfIdx(), missionId)
}

func (s *PlayerYYActivityReachStandard) GetMissionConfs() map[uint32]*jsondata.PYYActivityReachStandardMission {
	return jsondata.GetPYYActivityReachStandardConfs(s.ConfName, s.GetConfIdx())
}

func (s *PlayerYYActivityReachStandard) c2sReward(msg *base.Message) error {
	var req pb3.C2S_27_111
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := s.GetMissionConf(req.MissionId)
	if conf == nil {
		return neterror.ConfNotFoundError("confIdx %d missionId %d", s.GetConfIdx(), req.MissionId)
	}

	if len(conf.TargetVal) != 2 {
		return neterror.ConfNotFoundError("confIdx %d missionId %d, target val %v", s.GetConfIdx(), req.MissionId, conf.TargetVal)
	}

	pyyActivityId := conf.TargetVal[0]
	totalTimes := conf.TargetVal[1]
	times := s.GetData().MissionTimes[uint32(pyyActivityId)]
	if uint32(totalTimes) > times {
		return neterror.ParamsInvalidError("confIdx %d missionId %d, target val %v", s.GetConfIdx(), req.MissionId, times)
	}

	if ok := s.GetData().Rev[req.MissionId]; ok {
		return neterror.ParamsInvalidError("already reawarded for ConfIdx %d id %d", s.GetConfIdx(), req.MissionId)
	}

	s.GetData().Rev[req.MissionId] = true

	engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlayerYYActivityReachStandardReward,
	})

	logArgs := map[string]any{
		"configName": s.ConfName,
		"activityId": s.GetId(),
		"confIdx":    s.GetConfIdx(),
		"missionId":  req.MissionId,
	}

	logArgByte, _ := json.Marshal(logArgs)
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlayerYYActivityReachStandardReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArgByte),
	})

	s.SendProto3(27, 111, &pb3.S2C_27_111{
		MissionId: req.MissionId,
		State:     true,
		ActiveId:  s.GetId(),
	})

	return nil
}

func (s *PlayerYYActivityReachStandard) c2sBatReward(msg *base.Message) error {
	var req pb3.C2S_27_112
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	var totalAwards jsondata.StdRewardVec
	var canRecMissionIds []uint32
	for _, missionId := range req.MissionIdList {
		conf := s.GetMissionConf(missionId)
		if conf == nil {
			continue
		}
		if len(conf.TargetVal) != 2 {
			continue
		}
		pyyActivityId := conf.TargetVal[0]
		totalTimes := conf.TargetVal[1]
		times := s.GetData().MissionTimes[uint32(pyyActivityId)]
		if uint32(totalTimes) > times {
			continue
		}
		if ok := s.GetData().Rev[missionId]; ok {
			continue
		}
		s.GetData().Rev[missionId] = true
		canRecMissionIds = append(canRecMissionIds, missionId)
		totalAwards = append(totalAwards, conf.Rewards...)
		totalAwards = jsondata.MergeStdReward(totalAwards)
	}
	if len(totalAwards) == 0 {
		return neterror.ParamsInvalidError("not can rec awards")
	}
	engine.GiveRewards(s.GetPlayer(), totalAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlayerYYActivityReachStandardReward,
	})
	logArgs := map[string]any{
		"configName": s.ConfName,
		"activityId": s.GetId(),
		"confIdx":    s.GetConfIdx(),
		"missionIds": canRecMissionIds,
	}

	logArgByte, _ := json.Marshal(logArgs)
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlayerYYActivityReachStandardReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArgByte),
	})

	s.SendProto3(27, 112, &pb3.S2C_27_112{
		MissionIdList: canRecMissionIds,
		State:         true,
		ActiveId:      s.GetId(),
	})

	return nil
}

func (s *PlayerYYActivityReachStandard) handleAeActivityReachStandardTimes(event *custom_id.ActDrawEvent) {
	data := s.GetData()
	pyyActivityId := event.ActId
	addTimes := event.Times
	if event.ReachScore > 0 {
		addTimes = event.ReachScore
	}

	var skip = make(map[uint32]struct{})
	for _, mission := range s.GetMissionConfs() {
		if len(mission.TargetVal) != 2 {
			continue
		}
		if mission.TargetVal[0] != int64(pyyActivityId) {
			continue
		}
		if _, ok := skip[pyyActivityId]; ok {
			continue
		}
		data.MissionTimes[pyyActivityId] += addTimes
		skip[pyyActivityId] = struct{}{}
	}
	s.sendInfo()
}

func handleAeActivityReachStandardTimes(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	drawEvent, ok := args[0].(*custom_id.ActDrawEvent)
	if !ok {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYLimitGoal)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*PlayerYYActivityReachStandard)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.handleAeActivityReachStandardTimes(drawEvent)
		continue
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYLimitGoal, func() iface.IPlayerYY {
		return &PlayerYYActivityReachStandard{}
	})

	net.RegisterYYSysProto(27, 111, (*PlayerYYActivityReachStandard).c2sReward)
	net.RegisterYYSysProto(27, 112, (*PlayerYYActivityReachStandard).c2sBatReward)

	event.RegActorEvent(custom_id.AeActDrawTimes, handleAeActivityReachStandardTimes)
}
