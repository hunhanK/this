package actorsystem

import (
	"errors"
	"fmt"
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
	"math"
	"time"

	"github.com/gzjjyz/random"
)

type FairyDelegationSys struct {
	Base
	data *pb3.FairyDelegationData
}

func (sys *FairyDelegationSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *FairyDelegationSys) init() bool {
	if sys.GetBinaryData().FairydelegationData == nil {
		data := &pb3.FairyDelegationData{}
		sys.GetBinaryData().FairydelegationData = data
	}

	sys.data = sys.GetBinaryData().FairydelegationData

	if sys.data.Missions == nil {
		sys.data.Missions = make(map[string]*pb3.FairyDelegationMissionSt, 0)
	}

	return true
}

func (sys *FairyDelegationSys) OnOpen() {
	sys.init()

	sys.initMission()

	sys.s2cInfo()
}

func (sys *FairyDelegationSys) initMission() {
	missionRefreshNum := jsondata.GetFairyDelegationCommonConf().DelegrationRefreshNum

	newMissions, err := sys.randomMissions(missionRefreshNum, true)

	if err != nil {
		sys.LogError("randomMissions err %s", err)
		return
	}
	for key, nmiss := range newMissions {
		sys.data.Missions[key] = nmiss
	}
}

func (sys *FairyDelegationSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *FairyDelegationSys) s2cInfo() {
	sys.SendProto3(35, 0, &pb3.S2C_35_0{
		Missions: sys.data.Missions,
	})
}

func (sys *FairyDelegationSys) c2sInfo(msg *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *FairyDelegationSys) IsFairyBeOccupied(fairyHdl uint64) bool {
	for _, fdms := range sys.data.Missions {
		if fdms.StartTime == 0 {
			continue
		}

		for _, fooHdl := range fdms.FairyIds {
			if fooHdl == fairyHdl {
				return true
			}
		}
	}

	return false
}

func (sys *FairyDelegationSys) c2sRefreshMission(msg *base.Message) error {
	var req pb3.C2S_35_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg %s", err)
	}

	curUnDelegatedMissionNum := uint32(0)

	for _, fdms := range sys.data.Missions {
		if fdms.StartTime == 0 {
			curUnDelegatedMissionNum++
		}
	}

	isFree := curUnDelegatedMissionNum <= 1

	if !isFree {
		refreshConsume := jsondata.GetFairyDelegationCommonConf().RefreshConsume
		consumed := sys.GetOwner().ConsumeByConf(refreshConsume, req.Autobuy, common.ConsumeParams{LogId: pb3.LogId_LogFairyDelegationRefreshMission})
		if !consumed {
			return neterror.ParamsInvalidError("消耗失败")
		}
	}

	missionRefreshNum := jsondata.GetFairyDelegationCommonConf().DelegrationRefreshNum

	newMissions, err := sys.randomMissions(missionRefreshNum, isFree)
	if newMissions == nil || err != nil {
		return neterror.InternalError("no mission randomed %s", err)
	}

	for key, miss := range sys.data.Missions {
		if miss.StartTime == 0 {
			delete(sys.data.Missions, key)
		}
	}

	for key, nmiss := range newMissions {
		sys.data.Missions[key] = nmiss
	}

	sys.SendProto3(35, 1, &pb3.S2C_35_1{
		Missions: sys.data.Missions,
	})

	return nil
}

func (sys *FairyDelegationSys) c2sStartMission(msg *base.Message) error {
	var req pb3.C2S_35_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg %s", err)
	}

	mission := sys.data.Missions[req.Id]
	if mission == nil {
		return neterror.ParamsInvalidError("任务不存在 req.Id %s", req.Id)
	}

	if mission.StartTime != 0 {
		return neterror.ParamsInvalidError("任务已经开始")
	}

	qualityConf := jsondata.GetFairyDelegationQualityConfById(mission.QualityId)
	if qualityConf == nil {
		return neterror.InternalError("任务品质配置不存在 %d", mission.QualityId)
	}

	meet, err := sys.checkMissionStartCond(req.FairyIds, mission)
	if err != nil || !meet {
		return neterror.ParamsInvalidError("条件不满足 %s", err)
	}

	commonConf := jsondata.GetFairyDelegationCommonConf()
	if commonConf == nil {
		return neterror.InternalError("通用配置不存在")
	}

	if !sys.GetOwner().ConsumeByConf(commonConf.DelegateConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyDelegationStart}) {
		return neterror.ParamsInvalidError("消耗失败")
	}

	mission.StartTime = uint32(time.Now().Unix())
	mission.FairyIds = req.FairyIds

	sys.SendProto3(35, 4, &pb3.S2C_35_4{
		Mission: mission,
	})
	return nil
}

func (sys *FairyDelegationSys) c2sRewardMission(msg *base.Message) error {
	var req pb3.C2S_35_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("UnPackPb3Msg %s", err)
	}

	mission := sys.data.Missions[req.Id]
	if mission == nil {
		return neterror.ParamsInvalidError("任务不存在 req.Id %s", req.Id)
	}

	qualityConf := jsondata.GetFairyDelegationQualityConfById(mission.QualityId)
	if qualityConf == nil {
		return neterror.InternalError("任务品质配置不存在 %d", mission.QualityId)
	}

	missionEndTime := time.Unix(int64(mission.StartTime), 0).Add(time.Minute * time.Duration(qualityConf.Duration))

	leftMinutes := int32(math.Ceil(time.Until(missionEndTime).Minutes()))
	if leftMinutes > 0 {
		consume := jsondata.ConsumeMulti(jsondata.GetFairyDelegationCommonConf().MinuteEx, uint32(leftMinutes))

		if !sys.GetOwner().ConsumeByConf(consume, true, common.ConsumeParams{LogId: pb3.LogId_LogFairyDelegationQuickMissionReward}) {
			return neterror.ParamsInvalidError("消耗失败")
		}
	}

	rewardGroup := qualityConf.RandRewards
	if len(rewardGroup)-1 < int(mission.RewardIdx) {
		return neterror.InternalError("奖励不存在 %d", mission.RewardIdx)
	}
	state := engine.GiveRewards(sys.GetOwner(), qualityConf.RandRewards[mission.RewardIdx].Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyDelegationMissionReward,
	})

	if !state {
		return neterror.ParamsInvalidError("发奖失败")
	}

	delete(sys.data.Missions, req.Id)

	sys.owner.TriggerQuestEvent(custom_id.QttCompleteFairyDelegation, 0, 1)
	sys.SendProto3(35, 2, &pb3.S2C_35_2{
		Id: req.Id,
	})
	return nil
}

func (sys *FairyDelegationSys) checkMissionStartCond(fairyHdls []uint64, mission *pb3.FairyDelegationMissionSt) (success bool, failReason error) {
	if uint32(len(fairyHdls)) != mission.NeedFairy {
		return false, errors.New("精灵数量不满足要求")
	}

	qualityConf := jsondata.GetFairyDelegationQualityConfById(mission.QualityId)
	if qualityConf == nil {
		return false, errors.New("任务品质配置不存在")
	}

	fairysys, ok := sys.GetOwner().GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !fairysys.IsOpen() {
		return false, errors.New("精灵系统未开启")
	}

	var fairyItems []*pb3.ItemSt
	for _, hdl := range fairyHdls {
		fairyItem, err := fairysys.GetFairy(hdl)
		if err != nil {
			return false, err
		}
		fairyItems = append(fairyItems, fairyItem)
	}

	isStarMeet := false
	for _, fi := range fairyItems {
		if fi.Union2 >= qualityConf.StarLimit {
			isStarMeet = true
		}
	}

	if !isStarMeet {
		return false, errors.New("精灵星级不符合")
	}

	return isStarMeet, nil
}

func (sys *FairyDelegationSys) randomMissions(counts uint32, isFree bool) (map[string]*pb3.FairyDelegationMissionSt, error) {
	qualityConfs := jsondata.GetFairyDelegationQualityConfs()

	pool := random.Pool{}

	for k, fdqc := range qualityConfs {
		if isFree {
			pool.AddItem(k, fdqc.FreeWeight)
			continue
		}

		pool.AddItem(k, fdqc.PayWeigh)
	}
	randedKeys := pool.RandomMany(counts)

	randedQualityConfs := make(map[string]*jsondata.FairyDelegationQualityConf, 0)
	now := time.Now().UnixMilli()

	for i, v := range randedKeys {
		foo := v.(uint32)
		missionKey := fmt.Sprint(now + int64(i))
		randedQualityConfs[missionKey] = qualityConfs[foo]
	}

	missions := map[string]*pb3.FairyDelegationMissionSt{}

	for key, fdqc := range randedQualityConfs {
		if len(fdqc.Descs) == 0 {
			continue
		}

		descRandIdx := random.IntervalU(0, len(fdqc.Descs)-1)
		desc := fdqc.Descs[descRandIdx]

		if len(fdqc.FairyCount) < 2 {
			continue
		}
		count := random.IntervalUU(fdqc.FairyCount[0], fdqc.FairyCount[1])

		missions[key] = &pb3.FairyDelegationMissionSt{
			DescId:    desc.Id,
			QualityId: fdqc.QualityId,
			NeedFairy: count,
			Id:        key,
			RewardIdx: sys.randRewardIdx(fdqc),
		}
	}
	return missions, nil
}

func (sys *FairyDelegationSys) randRewardIdx(qualityConf *jsondata.FairyDelegationQualityConf) uint32 {
	pool := random.Pool{}

	for k, fdqc := range qualityConf.RandRewards {
		pool.AddItem(k, fdqc.Weight)
	}
	randedKeyItface := pool.RandomOne()

	randIdx, ok := randedKeyItface.(int)
	if ok {
		return uint32(randIdx)
	}

	return 0
}

func init() {
	RegisterSysClass(sysdef.SiFairyDelegation, func() iface.ISystem {
		return &FairyDelegationSys{}
	})

	net.RegisterSysProto(35, 0, sysdef.SiFairyDelegation, (*FairyDelegationSys).c2sInfo)
	net.RegisterSysProto(35, 1, sysdef.SiFairyDelegation, (*FairyDelegationSys).c2sRefreshMission)
	net.RegisterSysProto(35, 2, sysdef.SiFairyDelegation, (*FairyDelegationSys).c2sRewardMission)
	net.RegisterSysProto(35, 4, sysdef.SiFairyDelegation, (*FairyDelegationSys).c2sStartMission)
}
