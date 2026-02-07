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
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type DailyMissionSys struct {
	Base

	dailyMissionStates map[uint32]*pb3.DailyMissionState
	missionScoreInfo   *pb3.MissionScoreSt
}

func (sys *DailyMissionSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	if !sys.init() {
		sys.GetOwner().LogError("init DailyMissionSys failed")
	}
}

func (sys *DailyMissionSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *DailyMissionSys) init() bool {
	sys.dailyMissionStates = sys.GetBinaryData().DailMissionStates
	sys.missionScoreInfo = sys.GetBinaryData().MissionScore

	if sys.dailyMissionStates == nil {
		sys.dailyMissionStates = make(map[uint32]*pb3.DailyMissionState)
		sys.GetBinaryData().DailMissionStates = sys.dailyMissionStates
	}

	if sys.missionScoreInfo == nil {
		sys.missionScoreInfo = &pb3.MissionScoreSt{}
		sys.GetBinaryData().MissionScore = sys.missionScoreInfo
	}

	for i := len(sys.missionScoreInfo.StageAwarded); jsondata.GetDailyPointConf(uint32(i)) != nil; i++ {
		sys.missionScoreInfo.StageAwarded = append(sys.missionScoreInfo.StageAwarded, false)
	}
	return true
}

func (sys *DailyMissionSys) OnOpen() {
	sys.init()
	sys.CompleteMission(custom_id.DailyMissionLogin, 0)
	sys.s2cInfo()
}

func (sys *DailyMissionSys) c2sInfo(_ *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *DailyMissionSys) OnLogin() {
	sys.s2cInfo()
	sys.CompleteMission(custom_id.DailyMissionLogin, 0)
}

func (sys *DailyMissionSys) s2cInfo() {
	sys.SendProto3(32, 0, &pb3.S2C_32_0{
		DailyMissionStates: sys.dailyMissionStates,
		MissionScoreInfo:   sys.missionScoreInfo,
	})
}

func (sys *DailyMissionSys) c2sReward(msg *base.Message) error {
	var req pb3.C2S_32_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if int(req.Stage) >= len(sys.missionScoreInfo.StageAwarded) {
		return neterror.ParamsInvalidError("stage %d not exist", req.Stage)
	}

	stageConf := jsondata.GetDailyPointConf(req.Stage)
	if stageConf == nil {
		return neterror.ParamsInvalidError("stage %d not exist", req.Stage)
	}

	if uint32(sys.missionScoreInfo.Point) < stageConf.ReqPoint {
		return neterror.ParamsInvalidError("point not enough")
	}

	if sys.missionScoreInfo.StageAwarded[req.Stage] {
		return neterror.ParamsInvalidError("stage %d already rewarded", req.Stage)
	}

	params := common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogDailyPointStageReward,
		NoTips: false,
	}

	sys.missionScoreInfo.StageAwarded[req.Stage] = true

	if !engine.GiveRewards(sys.GetOwner(), stageConf.Award, params) {
		return neterror.ParamsInvalidError("give reward failed")
	}

	sys.SendProto3(32, 2, &pb3.S2C_32_2{
		Stage: req.Stage,
	})

	return nil
}

func (sys *DailyMissionSys) onNewDay() {
	// 重置数据
	sys.GetBinaryData().DailMissionStates = make(map[uint32]*pb3.DailyMissionState)
	sys.GetBinaryData().MissionScore = &pb3.MissionScoreSt{}

	sys.dailyMissionStates = sys.GetBinaryData().DailMissionStates
	sys.missionScoreInfo = sys.GetBinaryData().MissionScore

	for i := uint32(0); jsondata.GetDailyPointConf(i) != nil; i++ {
		sys.missionScoreInfo.StageAwarded = append(sys.missionScoreInfo.StageAwarded, false)
	}

	sys.CompleteMission(custom_id.DailyMissionLogin, 0)

	sys.c2sInfo(nil)
}

func (sys *DailyMissionSys) CompleteMission(missionId uint32, times int) {
	missionConf := jsondata.GetDailyMissionConf(missionId)
	if missionConf == nil {
		sys.LogDebug("mission %d not exist", missionId)
		return
	}

	mission, ok := sys.dailyMissionStates[missionId]
	if !ok {
		mission = &pb3.DailyMissionState{
			MissionId: missionId,
			CmpTimes:  0,
		}
		sys.dailyMissionStates[missionId] = mission
	}

	if mission.CmpTimes >= uint32(missionConf.CmpTimes) {
		return
	}

	var addPoint uint32
	var totalAwards jsondata.StdRewardVec
	if times > 0 {
		times = int(utils.MinUInt32(uint32(times), uint32(missionConf.CmpTimes)-mission.CmpTimes))
		mission.CmpTimes += uint32(times)
		addPoint = missionConf.Point * uint32(times)
		stdRewardVec := engine.FilterRewardByPlayer(sys.GetOwner(), missionConf.Award)
		if len(stdRewardVec) > 0 {
			totalAwards = append(totalAwards, jsondata.StdRewardMulti(stdRewardVec, int64(times))...)
		}
		sys.missionScoreInfo.Point += addPoint
	} else {
		mission.CmpTimes++
		addPoint = missionConf.Point
		sys.missionScoreInfo.Point += missionConf.Point
		totalAwards = append(totalAwards, engine.FilterRewardByPlayer(sys.GetOwner(), missionConf.Award)...)
	}
	if len(totalAwards) > 0 {
		engine.GiveRewards(sys.GetOwner(), totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyPointOneItemReward})
	}
	sys.GetOwner().TriggerEvent(custom_id.AeAddDailyMissionPoint, addPoint)
	sys.GetOwner().TriggerEvent(custom_id.AeAddFlyingFairyOrderFuncExp, sysdef.SiDaily, uint32(addPoint))
	sys.GetOwner().TriggerEvent(custom_id.AeAddActivityHandBookExp, sysdef.SiDaily, uint32(addPoint))
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttReachDailyMissionScore)

	sys.SendProto3(32, 1, &pb3.S2C_32_1{
		DailyMissionStates: &pb3.DailyMissionState{
			MissionId: missionId,
			CmpTimes:  mission.CmpTimes,
		},
		MissionScore: sys.missionScoreInfo.Point,
	})
}

func gmAddDailyPoint(player iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}

	dailyMissionSys, ok := player.GetSysObj(sysdef.SiDaily).(*DailyMissionSys)
	if !ok || !dailyMissionSys.IsOpen() {
		player.LogError("gmAddDailyPoint get dailyMissionSys failed or sys not open")
		return false
	}

	dailyMissionSys.missionScoreInfo.Point += uint32(utils.AtoInt64(args[0]))

	dailyMissionSys.SendProto3(32, 0, &pb3.S2C_32_0{
		DailyMissionStates: dailyMissionSys.dailyMissionStates,
		MissionScoreInfo:   dailyMissionSys.missionScoreInfo,
	})
	return true
}

func gmDailyMissionCmp(player iface.IPlayer, args ...string) bool {
	if len(args) < 1 {
		return false
	}

	missionId := utils.AtoUint32(args[0])

	player.TriggerEvent(custom_id.AeDailyMissionComplete, missionId)
	return true
}

func init() {
	RegisterSysClass(sysdef.SiDaily, func() iface.ISystem {
		return &DailyMissionSys{}
	})

	event.RegActorEvent(custom_id.AeDailyMissionComplete, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		missionId, ok := args[0].(uint32)
		if !ok {
			actor.LogError("DailyMissionSys: missionId not uint32")
			return
		}
		sys, ok := actor.GetSysObj(sysdef.SiDaily).(*DailyMissionSys)
		if !ok || !sys.IsOpen() {
			return
		}

		times := 0
		if len(args) > 1 {
			times, ok = args[1].(int)
			if !ok {
				actor.LogError("DailyMissionSys: times not int")
			}
		}

		sys.CompleteMission(missionId, times)
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiDaily).(*DailyMissionSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	net.RegisterSysProto(32, 0, sysdef.SiDaily, (*DailyMissionSys).c2sInfo)
	net.RegisterSysProto(32, 2, sysdef.SiDaily, (*DailyMissionSys).c2sReward)

	engine.RegQuestTargetProgress(custom_id.QttReachDailyMissionScore, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		obj := actor.GetSysObj(sysdef.SiDaily)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys := obj.(*DailyMissionSys)
		if sys == nil {
			return 0
		}
		return sys.missionScoreInfo.Point
	})

	gmevent.Register("dailypointadd", gmAddDailyPoint, 1)
	gmevent.Register("dailymissioncmp", gmDailyMissionCmp, 1)
}
