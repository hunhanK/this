/**
 * @Author: yzh
 * @Date:
 * @Desc: 唤灵
 * @Modify：
**/

package actorsystem

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math/rand"
	"time"

	"github.com/995933447/runtimeutil"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/gzjjyz/random"
)

var (
	hasInitDrawRewardWeight bool

	drawRaceRewardPoolW    drawRaceRewardPoolWeight
	drawSubjectRewardPoolW drawSubjectRewardPoolWeight
)

type drawRewardPoolWeight struct {
	rewardPackageCategoryWeight        *random.Pool
	mapCategoryToRewardWeightInPackage map[uint32]*random.Pool
}

func (w *drawRewardPoolWeight) randomOneCategory() uint32 {
	return w.rewardPackageCategoryWeight.RandomOne().(uint32)
}

func (w *drawRewardPoolWeight) randomOneReward(category uint32) *jsondata.DrawReward {
	return w.mapCategoryToRewardWeightInPackage[category].RandomOne().(*jsondata.DrawReward)
}

type drawRaceRewardPoolWeight struct {
	immortalRewardPackageCategoryWeight        *random.Pool
	mapCategoryToImmortalRewardWeightInPackage map[uint32]*random.Pool

	spiritRewardPackageCategoryWeight        *random.Pool
	mapCategoryToSpiritRewardWeightInPackage map[uint32]*random.Pool

	monsterRewardPackageCategoryWeight        *random.Pool
	mapCategoryToMonsterRewardWeightInPackage map[uint32]*random.Pool
}

func (w *drawRaceRewardPoolWeight) randomOneImmortalCategory() uint32 {
	return w.immortalRewardPackageCategoryWeight.RandomOne().(uint32)
}

func (w *drawRaceRewardPoolWeight) randomOneImmortalReward(category uint32) *jsondata.DrawReward {
	return w.mapCategoryToImmortalRewardWeightInPackage[category].RandomOne().(*jsondata.DrawReward)
}

func (w *drawRaceRewardPoolWeight) randomOneSpiritCategory() uint32 {
	return w.spiritRewardPackageCategoryWeight.RandomOne().(uint32)
}

func (w *drawRaceRewardPoolWeight) randomOneSpiritReward(category uint32) *jsondata.DrawReward {
	return w.mapCategoryToSpiritRewardWeightInPackage[category].RandomOne().(*jsondata.DrawReward)
}

func (w *drawRaceRewardPoolWeight) randomOneMonsterCategory() uint32 {
	return w.monsterRewardPackageCategoryWeight.RandomOne().(uint32)
}

func (w *drawRaceRewardPoolWeight) randomOneMonsterReward(category uint32) *jsondata.DrawReward {
	return w.mapCategoryToMonsterRewardWeightInPackage[category].RandomOne().(*jsondata.DrawReward)
}

type drawSubjectRewardPoolWeight struct {
	mapCycleToRewardPackageCategoryWeight        map[uint32]*random.Pool
	mapCycleToMapCategoryToRewardWeightInPackage map[uint32]map[uint32]*random.Pool
}

func (w *drawSubjectRewardPoolWeight) randomOneCategoryOnCycle(cycle uint32) uint32 {
	return w.mapCycleToRewardPackageCategoryWeight[cycle].RandomOne().(uint32)
}

func (w *drawSubjectRewardPoolWeight) randomOneRewardOnCycle(cycle, category uint32) *jsondata.DrawReward {
	return w.mapCycleToMapCategoryToRewardWeightInPackage[cycle][category].RandomOne().(*jsondata.DrawReward)
}

type DrawRewardSys struct {
	Base

	dramaRewardPoolWeight           *drawRewardPoolWeight
	mapCategoryToDramaRewardPackage map[uint32][]*jsondata.DrawReward

	curUsedSubRaceRewardPool pb3.DrawSubRaceRewardPool
	revRewardKinds           *bloom.BloomFilter
}

func newDrawRewardSys() *DrawRewardSys {
	return &DrawRewardSys{
		mapCategoryToDramaRewardPackage: map[uint32][]*jsondata.DrawReward{},
		dramaRewardPoolWeight:           &drawRewardPoolWeight{},
	}
}

func (s *DrawRewardSys) Init(sysId uint32, sysMgr iface.ISystemMgr, player iface.IPlayer) {
	s.Base.Init(sysId, sysMgr, player)
	if !hasInitDrawRewardWeight {
		initDrawRewardWeight()
		hasInitDrawRewardWeight = true
	}
	s.initRevRewardKinds()
	s.initDramaRewardPool()
	s.initDramaRewardPoolWeight()
	s.chooseSubRaceRewardPool()
}

func (s *DrawRewardSys) initRevRewardKinds() {
	s.revRewardKinds = bloom.From(s.GetPlayerState().RecvRewardBloomSet, 7)
}

func (s *DrawRewardSys) initDramaRewardPool() {
	dramaPoolConf := jsondata.GetDrawRewardConf().DramaDrawRewardPool
	for category, rewardPackage := range dramaPoolConf.MapCategoryToRewardPackage {
		s.mapCategoryToDramaRewardPackage[category] = rewardPackage
	}

	for _, questId := range s.GetPlayerState().FinishAddDramaRewardQuestIds {
		s.addDrawDramaRewardByCond(jsondata.AddRewardPackageCondTypeFinishQuest, questId)
	}
}

func (s *DrawRewardSys) initDramaRewardPoolWeight() {
	dramaPoolConf := jsondata.GetDrawRewardConf().DramaDrawRewardPool
	s.dramaRewardPoolWeight.rewardPackageCategoryWeight = &random.Pool{}
	s.dramaRewardPoolWeight.mapCategoryToRewardWeightInPackage = map[uint32]*random.Pool{}
	for category, weight := range dramaPoolConf.MapCategoryToWeight {
		s.dramaRewardPoolWeight.rewardPackageCategoryWeight.AddItem(category, weight)

		weightCtl, ok := s.dramaRewardPoolWeight.mapCategoryToRewardWeightInPackage[category]
		if !ok {
			weightCtl = &random.Pool{}
			s.dramaRewardPoolWeight.mapCategoryToRewardWeightInPackage[category] = weightCtl
		}
		for _, reward := range s.mapCategoryToDramaRewardPackage[category] {
			weightCtl.AddItem(reward, reward.Weight)
		}
	}
}

func (s *DrawRewardSys) notifySubjectCycleRange() {
	cycles, _, err := s.getCurCycles()
	if err != nil {
		s.LogError(err.Error())
		return
	}

	subjectPoolConf := jsondata.GetDrawRewardConf().SubjectDrawRewardPool

	maxCycles := uint32(len(subjectPoolConf.CycleRewards))
	if maxCycles < cycles {
		return
	}

	todayInWeek := time.Now().Weekday()
	if todayInWeek == 0 {
		todayInWeek = 7
	}

	startedAt, endAt, err := s.getCurOrNextCycleRange()
	if err != nil {
		s.LogError(err.Error())
		return
	}

	if maxCycles == cycles {
		endTimeInWeek := time.Now().Add(time.Hour * time.Duration(7-todayInWeek) * 24)
		if endAt > uint32(endTimeInWeek.Unix()) {
			return
		}
	}

	s.SendProto3(31, 13, &pb3.S2C_31_13{
		StartedAt: startedAt,
		EndAt:     endAt,
	})
}

func (s *DrawRewardSys) chooseSubRaceRewardPool() {
	if !s.GetPlayerState().StartedRacePool {
		s.SendProto3(31, 12, &pb3.S2C_31_12{
			Race: uint32(pb3.DrawRewardPool_DrawRewardPoolNil),
		})
		return
	}

	playerState := s.GetPlayerState()
	lastSureSubPoolDayStartTime := runtimeutil.BeginTimeOfDate(time.Unix(int64(playerState.LastSureRacePoolAt), 0))
	passDays := runtimeutil.CountDaysNumberBetweenTwoTimes(runtimeutil.BeginTimeOfDate(time.Now()), lastSureSubPoolDayStartTime)
	playerState.LastRacePoolType += uint32(passDays)
	if playerState.LastRacePoolType > 3 {
		playerState.LastRacePoolType -= 3
	}
	s.curUsedSubRaceRewardPool = pb3.DrawSubRaceRewardPool(playerState.LastRacePoolType)
	playerState.LastSureRacePoolAt = uint32(time.Now().Unix())
	s.SendProto3(31, 12, &pb3.S2C_31_12{
		Race: playerState.LastRacePoolType,
	})
}

func (s *DrawRewardSys) getCurOrNextCycleRange() (uint32, uint32, error) {
	subjectPoolConf := jsondata.GetDrawRewardConf().SubjectDrawRewardPool

	openSrvDayStartTime := runtimeutil.BeginTimeOfDate(time.Unix(int64(gshare.OpenTime), 0))
	firstOpenDayStartTime := time.Unix(int64(runtimeutil.AddDays(uint32(openSrvDayStartTime.Unix()), subjectPoolConf.FirstOpenDay)), 0)

	switch subjectPoolConf.CycleType {
	case jsondata.SubjectDrawRewardPoolOpenCycleWeek:
		// calculate that if today is in active time
		firstOpenDayInWeek := firstOpenDayStartTime.Weekday()
		if firstOpenDayInWeek == 0 {
			firstOpenDayInWeek = 7
		}

		openCycleDayInWeek := subjectPoolConf.OpenDayOnCycle
		if openCycleDayInWeek == 0 {
			openCycleDayInWeek = 7
		}

		endCycleDayInWeek := openCycleDayInWeek + subjectPoolConf.ActiveDays
		if endCycleDayInWeek > 7 {
			endCycleDayInWeek -= 7
		}

		firstCycleEndTime := firstOpenDayStartTime.Add(time.Duration(subjectPoolConf.ActiveDays) * time.Hour * 24)
		firstCycleEndInWeek := uint8(firstCycleEndTime.Weekday())
		if firstCycleEndInWeek == 0 {
			firstCycleEndInWeek = 7
		}

		if firstCycleEndInWeek > openCycleDayInWeek && firstCycleEndInWeek < endCycleDayInWeek {
			firstCycleEndInWeek = endCycleDayInWeek
		}

		var endDaysSinceFirstStartTime uint8
		if firstCycleEndInWeek > uint8(firstOpenDayInWeek) {
			endDaysSinceFirstStartTime = firstCycleEndInWeek - uint8(firstOpenDayInWeek)
		} else {
			endDaysSinceFirstStartTime = 7 - uint8(firstOpenDayInWeek) + endCycleDayInWeek
		}

		firstCycleEndTime = time.Unix(firstOpenDayStartTime.Unix()+24*3600*int64(endDaysSinceFirstStartTime), 0)

		now := time.Now().Unix()
		if now >= firstOpenDayStartTime.Unix() && now <= firstCycleEndTime.Unix() {
			return uint32(firstOpenDayStartTime.Unix()), uint32(firstCycleEndTime.Unix()), nil
		}

		todayInWeek := time.Now().Weekday()
		if todayInWeek == 0 {
			todayInWeek = 7
		}

		var startedAt, endAt uint32
		if uint8(todayInWeek) <= openCycleDayInWeek {
			startedAt = uint32(runtimeutil.BeginTimeOfDate(time.Now()).Add(time.Hour * time.Duration(24*(openCycleDayInWeek-uint8(todayInWeek)))).Unix())
		} else {
			startedAt = uint32(runtimeutil.BeginTimeOfDate(time.Now()).Add(-time.Hour * time.Duration(24*(uint8(todayInWeek)-openCycleDayInWeek))).Unix())
		}

		endAt = startedAt + 24*3600*uint32(subjectPoolConf.ActiveDays)

		if endAt < uint32(now) {
			startedAt += 24 * 3600 * 7
			endAt += 24 * 3600 * 7
		}

		return startedAt, endAt, nil
	}

	return 0, 0, errors.New("unknown subject cycle type")
}

func (s *DrawRewardSys) getCurCycles() (uint32, bool, error) {
	subjectPoolConf := jsondata.GetDrawRewardConf().SubjectDrawRewardPool

	openSrvDayStartTime := runtimeutil.BeginTimeOfDate(time.Unix(int64(gshare.OpenTime), 0))
	todayStartTime := runtimeutil.BeginTimeOfDate(time.Now())
	firstOpenDayStartTime := time.Unix(int64(runtimeutil.AddDays(uint32(openSrvDayStartTime.Unix()), subjectPoolConf.FirstOpenDay)), 0)
	passDays := runtimeutil.CountDaysNumberBetweenTwoTimes(todayStartTime, openSrvDayStartTime)

	switch subjectPoolConf.CycleType {
	case jsondata.SubjectDrawRewardPoolOpenCycleWeek:
		// calculate that if subject has been open
		if uint32(passDays) < subjectPoolConf.FirstOpenDay {
			return 0, false, nil
		}

		// calculate that if subject has been open and in active time
		if uint32(passDays)-subjectPoolConf.FirstOpenDay <= uint32(subjectPoolConf.ActiveDays) {
			return 1, true, nil
		}

		// calculate that if today is in active time
		firstOpenDayInWeek := firstOpenDayStartTime.Weekday()
		if firstOpenDayInWeek == 0 {
			firstOpenDayInWeek = 7
		}
		openCycleDayInWeek := subjectPoolConf.OpenDayOnCycle
		if openCycleDayInWeek == 0 {
			openCycleDayInWeek = 7
		}
		endCycleDayInWeek := openCycleDayInWeek + subjectPoolConf.ActiveDays
		if endCycleDayInWeek > 7 {
			endCycleDayInWeek -= 7
		}
		activeDaysInWeek := map[uint8]struct{}{}
		activeDayInWeek := openCycleDayInWeek
		for {
			activeDaysInWeek[activeDayInWeek] = struct{}{}

			activeDayInWeek++

			if openCycleDayInWeek <= endCycleDayInWeek {
				if activeDayInWeek > endCycleDayInWeek {
					break
				}

				continue
			}

			if activeDayInWeek > 7 {
				activeDayInWeek -= 7
			}
			if activeDayInWeek < openCycleDayInWeek {
				if activeDayInWeek > endCycleDayInWeek {
					break
				}
			}
		}
		todayInWeek := time.Now().Weekday()
		if todayInWeek == 0 {
			todayInWeek = 7
		}
		_, isTodayOpen := activeDaysInWeek[uint8(todayInWeek)]

		// count cycles
		cycleNum := uint32(1)
		if uint8(firstOpenDayInWeek) < openCycleDayInWeek {
			cycleNum++
		}
		secOpenWeekStartTime := time.Unix(int64(runtimeutil.AddDays(uint32(firstOpenDayStartTime.Unix()), uint32(7-firstOpenDayInWeek+1))), 0)
		passDaysFromSecWeekStartTime := runtimeutil.CountDaysNumberBetweenTwoTimes(runtimeutil.BeginTimeOfDate(time.Now()), secOpenWeekStartTime)
		cycleNum += uint32(passDaysFromSecWeekStartTime / 7)
		if passDaysFromSecWeekStartTime%7 > 0 && uint8(todayInWeek) >= subjectPoolConf.OpenDayOnCycle {
			cycleNum++
		}
		return cycleNum, isTodayOpen, nil
	}

	return 0, false, errors.New("unknown subject cycle type")
}

func (s *DrawRewardSys) drawFromSubjectRewardPool(drawTimes uint32) {
	cycles, isTodayOpen, err := s.getCurCycles()
	if err != nil {
		s.LogError(err.Error())
		return
	}

	if cycles == 0 {
		s.LogWarn("subject pool not open")
		return
	}

	if !isTodayOpen {
		s.LogWarn("subject pool not open")
		return
	}

	subjectPoolConf := jsondata.GetDrawRewardConf().SubjectDrawRewardPool

	if uint32(len(subjectPoolConf.CycleRewards)) < cycles {
		return
	}

	var consumes []*jsondata.Consume
	switch drawTimes {
	case 1:
		consumes = subjectPoolConf.Consume
	case 10:
		consumes = subjectPoolConf.Consume10
	default:
		return
	}

	if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogDrawDramaReward}) {
		return
	}

	var rewards []*jsondata.StdReward
	var isMustGet4StarReward bool
	if drawTimes == 10 {
		isMustGet4StarReward = true
	}
	var got4StarReward bool
	for i := uint32(0); i < drawTimes; i++ {
		category := drawSubjectRewardPoolW.randomOneCategoryOnCycle(cycles)
		reward := drawSubjectRewardPoolW.randomOneRewardOnCycle(cycles, category)
		if isMustGet4StarReward && !got4StarReward {
			fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
			if nil != fairyConf {
				if fairyConf.Star >= 4 {
					got4StarReward = true
				}
			}
		}
		if isMustGet4StarReward && i+1 >= drawTimes && !got4StarReward {
			rewardPool := subjectPoolConf.CycleRewards[cycles].Rewards
			rewardPoolSize := len(rewardPool)
			start := rand.Int() % rewardPoolSize
			for i := 0; i < rewardPoolSize; i++ {
				reward = rewardPool[start]
				fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
				if nil == fairyConf {
					continue
				}
				if fairyConf.Star >= 4 {
					break
				}
				start++
				if start >= rewardPoolSize {
					start = 0
				}
			}
		}
		rewards = append(rewards, &reward.StdReward)
	}

	if !engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawRaceReward}) {
		s.LogWarn("actor(id:%d) get reward failed", s.owner.GetId())
		return
	}

	s.LogPlayerBehavior(uint32(pb3.DrawRewardPool_DrawRewardPoolSubject), drawTimes)

	s.sendRewards2Cli(rewards, pb3.DrawRewardPool_DrawRewardPoolSubject)
	s.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionDrawFairy, int(drawTimes))
}

func (s *DrawRewardSys) switchCurUsedSubRaceRewardPool(newRaceType pb3.DrawSubRaceRewardPool) {
	if s.GetPlayerState().StartedRacePool {
		raceRewardConf := jsondata.GetDrawRewardConf().RaceDrawRewardPool
		if !s.owner.ConsumeByConf(raceRewardConf.ManualSwitchPackagesConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogSwitchRaceReward}) {
			return
		}
	}

	s.curUsedSubRaceRewardPool = newRaceType
	playerState := s.GetPlayerState()
	playerState.LastRacePoolType = uint32(newRaceType)
	playerState.LastSureRacePoolAt = uint32(time.Now().Unix())
	playerState.StartedRacePool = true

	s.SendProto3(31, 12, &pb3.S2C_31_12{
		Race: uint32(newRaceType),
	})
}

func (s *DrawRewardSys) drawFromRaceRewardPool(drawTimes uint32) {
	if !s.GetPlayerState().StartedRacePool {
		return
	}

	racePoolConf := jsondata.GetDrawRewardConf().RaceDrawRewardPool

	var consumes []*jsondata.Consume
	switch drawTimes {
	case 1:
		consumes = racePoolConf.Consume
	case 10:
		consumes = racePoolConf.Consume10
	default:
		return
	}

	if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogDrawDramaReward}) {
		return
	}

	var (
		curSubRaceRewardPoolCategoryWeight           *random.Pool
		curMapCategoryToSubRaceRewardWeightInPackage map[uint32]*random.Pool
		rewardPool                                   []*jsondata.DrawReward
	)
	switch s.curUsedSubRaceRewardPool {
	case pb3.DrawSubRaceRewardPool_DrawSubRaceRewardPoolImmortal:
		curSubRaceRewardPoolCategoryWeight = drawRaceRewardPoolW.immortalRewardPackageCategoryWeight
		curMapCategoryToSubRaceRewardWeightInPackage = drawRaceRewardPoolW.mapCategoryToImmortalRewardWeightInPackage
		rewardPool = racePoolConf.ImmortalRewards
	case pb3.DrawSubRaceRewardPool_DrawSubRaceRewardPoolSpirit:
		curSubRaceRewardPoolCategoryWeight = drawRaceRewardPoolW.spiritRewardPackageCategoryWeight
		curMapCategoryToSubRaceRewardWeightInPackage = drawRaceRewardPoolW.mapCategoryToSpiritRewardWeightInPackage
		rewardPool = racePoolConf.SpiritRewards
	default:
		curSubRaceRewardPoolCategoryWeight = drawRaceRewardPoolW.monsterRewardPackageCategoryWeight
		curMapCategoryToSubRaceRewardWeightInPackage = drawRaceRewardPoolW.mapCategoryToMonsterRewardWeightInPackage
		rewardPool = racePoolConf.MonsterRewards
	}

	var isMustGet4StarReward bool
	if drawTimes == 10 {
		isMustGet4StarReward = true
	}
	var rewards []*jsondata.StdReward
	var got4StarReward bool
	for i := uint32(0); i < drawTimes; i++ {
		category := curSubRaceRewardPoolCategoryWeight.RandomOne().(uint32)
		reward := curMapCategoryToSubRaceRewardWeightInPackage[category].RandomOne().(*jsondata.DrawReward)
		if isMustGet4StarReward && !got4StarReward {
			fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
			if nil != fairyConf {
				if fairyConf.Star >= 4 {
					got4StarReward = true
				}
			}
		}
		if isMustGet4StarReward && i+1 >= drawTimes && !got4StarReward {
			rewardPoolSize := len(rewardPool)
			start := rand.Int() % rewardPoolSize
			for i := 0; i < rewardPoolSize; i++ {
				reward = rewardPool[start]
				fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
				if nil == fairyConf {
					continue
				}
				if fairyConf.Star >= 4 {
					break
				}
				start++
				if start >= rewardPoolSize {
					start = 0
				}
			}
		}
		rewards = append(rewards, &reward.StdReward)
	}

	if !engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawRaceReward}) {
		s.LogWarn("actor(id:%d) get reward failed", s.owner.GetId())
		return
	}

	s.LogPlayerBehavior(uint32(pb3.DrawRewardPool_DrawRewardPoolRace), drawTimes)

	s.sendRewards2Cli(rewards, pb3.DrawRewardPool_DrawRewardPoolRace)
	s.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionDrawFairy, int(drawTimes))
}

func (s *DrawRewardSys) drawFromDramaRewardPool(drawTimes uint32) {
	dramaPoolConf := jsondata.GetDrawRewardConf().DramaDrawRewardPool

	var consumes []*jsondata.Consume
	switch drawTimes {
	case 1:
		consumes = dramaPoolConf.Consume
	case 10:
		consumes = dramaPoolConf.Consume10
	default:
		return
	}

	if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogDrawDramaReward}) {
		return
	}
	var rewards []*jsondata.StdReward
	if !s.GetPlayerState().HasDramaDraw && drawTimes == 1 {
		rewards = append(rewards, &jsondata.StdReward{Id: dramaPoolConf.FirstGetFairy, Count: 1})
		if !engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawDramaRewardFirst}) {
			s.LogWarn("actor(id:%d) get reward failed", s.owner.GetId())
			return
		}
	} else {
		var isMustGet4StarReward bool
		if drawTimes == 10 {
			isMustGet4StarReward = true
		}
		var got4StarReward bool
		for i := uint32(0); i < drawTimes; i++ {
			category := s.dramaRewardPoolWeight.randomOneCategory()
			reward := s.dramaRewardPoolWeight.randomOneReward(category)
			if isMustGet4StarReward && !got4StarReward {
				fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
				if nil != fairyConf {
					if fairyConf.Star >= 4 {
						got4StarReward = true
					}
				}
			}
			if isMustGet4StarReward && i+1 >= drawTimes && !got4StarReward {
				rewardPoolSize := len(dramaPoolConf.Rewards)
				start := rand.Int() % rewardPoolSize
				for i := 0; i < rewardPoolSize; i++ {
					reward = dramaPoolConf.Rewards[start]
					fairyConf := jsondata.GetFairyConf(reward.StdReward.Id)
					if nil == fairyConf {
						continue
					}
					if fairyConf.Star >= 4 {
						break
					}
					start++
					if start >= rewardPoolSize {
						start = 0
					}
				}
			}
			rewards = append(rewards, &reward.StdReward)
		}
		if !engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawDramaReward}) {
			s.LogWarn("actor(id:%d) get reward failed", s.owner.GetId())
			return
		}
	}

	if !s.GetPlayerState().HasDramaDraw && drawTimes == 1 {
		s.GetPlayerState().HasDramaDraw = true
	}

	s.LogPlayerBehavior(uint32(pb3.DrawRewardPool_DrawRewardPoolDrama), drawTimes)

	s.sendRewards2Cli(rewards, pb3.DrawRewardPool_DrawRewardPoolDrama)
	s.owner.TriggerQuestEvent(custom_id.QttDrawRewardTimes, 0, int64(drawTimes))
	s.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionDrawFairy, int(drawTimes))
}

func (s *DrawRewardSys) onFirstRecvKindReward(reward *jsondata.StdReward) {
	itemIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(itemIdBytes, reward.Id)
	s.revRewardKinds.Add(itemIdBytes)
}

func (s *DrawRewardSys) sendRewards2Cli(rewards []*jsondata.StdReward, pool pb3.DrawRewardPool) {
	var itemPbs []*pb3.ItemSt
	firstRecvItemIds := []uint32{}
	for _, reward := range rewards {
		itemIdBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(itemIdBytes, reward.Id)
		// maybe result is not exact, but rate is very low, acceptable
		if !s.revRewardKinds.Test(itemIdBytes) {
			firstRecvItemIds = append(firstRecvItemIds, reward.Id)
			s.onFirstRecvKindReward(reward)
		}

		itemPb := jsondata.StdRewardToPb3ShowFairyItem(reward)
		itemPbs = append(itemPbs, itemPb)
	}

	if len(firstRecvItemIds) > 0 {
		s.GetPlayerState().RecvRewardBloomSet = s.revRewardKinds.BitSet().Bytes()
	}

	s.owner.SendProto3(31, 11, &pb3.S2C_31_11{
		RewardItems:      itemPbs,
		Pool:             uint32(pool),
		FirstRecvItemIds: firstRecvItemIds,
	})
}

func (s *DrawRewardSys) GetPlayerState() *pb3.DrawRewardPoolState {
	bin := s.owner.GetBinaryData()
	if bin.DrawRewardPoolState == nil {
		bin.DrawRewardPoolState = &pb3.DrawRewardPoolState{
			RecvRewardBloomSet: make([]uint64, 1000),
			LastSureRacePoolAt: uint32(time.Now().Unix()),
			StartedRacePool:    false,
		}
	}
	return bin.DrawRewardPoolState
}

func (s *DrawRewardSys) addDrawDramaRewardByCond(condType jsondata.AddRewardPackageCondType, condId uint32) bool {
	dramaPoolConf := jsondata.GetDrawRewardConf().DramaDrawRewardPool
	var success bool
	for _, cond := range dramaPoolConf.AddRewardPackageConds {
		if cond.CondType != condType {
			continue
		}

		if cond.CondId != condId {
			continue
		}

		playerState := s.GetPlayerState()
		switch condType {
		case jsondata.AddRewardPackageCondTypeFinishQuest:
			for _, questId := range playerState.FinishAddDramaRewardQuestIds {
				if questId != condId {
					continue
				}

				return false
			}

			playerState.FinishAddDramaRewardQuestIds = append(playerState.FinishAddDramaRewardQuestIds, condId)
		}

		success = true

		newAddCategoryToRewardPackageMap := cond.MapCategoryToRewardPackage
		for category, rewardPackage := range newAddCategoryToRewardPackageMap {
			theRewardPackage := s.mapCategoryToDramaRewardPackage[category]
			s.mapCategoryToDramaRewardPackage[category] = append(theRewardPackage, rewardPackage...)
		}

		break
	}

	return success
}

func (s *DrawRewardSys) c2sDrawReward(msg *base.Message) {
	var req pb3.C2S_31_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	switch req.Pool {
	case uint32(pb3.DrawRewardPool_DrawRewardPoolDrama):
		s.drawFromDramaRewardPool(req.DrawTimes)
	case uint32(pb3.DrawRewardPool_DrawRewardPoolRace):
		s.drawFromRaceRewardPool(req.DrawTimes)
	case uint32(pb3.DrawRewardPool_DrawRewardPoolSubject):
		s.drawFromSubjectRewardPool(req.DrawTimes)
	}
}

func (s *DrawRewardSys) c2sSwitchSubRaceRewardPool(msg *base.Message) {
	var req pb3.C2S_31_12
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}

	if req.Race > 3 {
		return
	}

	s.switchCurUsedSubRaceRewardPool(pb3.DrawSubRaceRewardPool(req.Race))
}

func initDrawRewardWeight() {
	conf := jsondata.GetDrawRewardConf()

	racePoolConf := conf.RaceDrawRewardPool

	drawRaceRewardPoolW.immortalRewardPackageCategoryWeight = &random.Pool{}
	drawRaceRewardPoolW.mapCategoryToImmortalRewardWeightInPackage = map[uint32]*random.Pool{}
	for category, weight := range racePoolConf.MapImmortalCategoryToWeight {
		drawRaceRewardPoolW.immortalRewardPackageCategoryWeight.AddItem(category, weight)

		weightCtl, ok := drawRaceRewardPoolW.mapCategoryToImmortalRewardWeightInPackage[category]
		if !ok {
			weightCtl = &random.Pool{}
			drawRaceRewardPoolW.mapCategoryToImmortalRewardWeightInPackage[category] = weightCtl
		}
		for _, reward := range racePoolConf.MapCategoryToRewardPackageInImmortal[category] {
			weightCtl.AddItem(reward, reward.Weight)
		}
	}

	drawRaceRewardPoolW.spiritRewardPackageCategoryWeight = &random.Pool{}
	drawRaceRewardPoolW.mapCategoryToSpiritRewardWeightInPackage = map[uint32]*random.Pool{}
	for category, weight := range racePoolConf.MapSpiritCategoryToWeight {
		drawRaceRewardPoolW.spiritRewardPackageCategoryWeight.AddItem(category, weight)

		weightCtl, ok := drawRaceRewardPoolW.mapCategoryToSpiritRewardWeightInPackage[category]
		if !ok {
			weightCtl = &random.Pool{}
			drawRaceRewardPoolW.mapCategoryToSpiritRewardWeightInPackage[category] = weightCtl
		}
		for _, reward := range racePoolConf.MapCategoryToRewardPackageInSpirit[category] {
			weightCtl.AddItem(reward, reward.Weight)
		}
	}

	drawRaceRewardPoolW.monsterRewardPackageCategoryWeight = &random.Pool{}
	drawRaceRewardPoolW.mapCategoryToMonsterRewardWeightInPackage = map[uint32]*random.Pool{}
	for category, weight := range racePoolConf.MapMonsterCategoryToWeight {
		drawRaceRewardPoolW.monsterRewardPackageCategoryWeight.AddItem(category, weight)

		weightCtl, ok := drawRaceRewardPoolW.mapCategoryToMonsterRewardWeightInPackage[category]
		if !ok {
			weightCtl = &random.Pool{}
			drawRaceRewardPoolW.mapCategoryToMonsterRewardWeightInPackage[category] = weightCtl
		}
		for _, reward := range racePoolConf.MapCategoryToRewardPackageInMonster[category] {
			weightCtl.AddItem(reward, reward.Weight)
		}
	}

	subjectPoolConf := conf.SubjectDrawRewardPool

	drawSubjectRewardPoolW.mapCycleToRewardPackageCategoryWeight = map[uint32]*random.Pool{}
	drawSubjectRewardPoolW.mapCycleToMapCategoryToRewardWeightInPackage = map[uint32]map[uint32]*random.Pool{}
	for _, cycleReward := range subjectPoolConf.CycleRewards {
		for category, weight := range cycleReward.MapCategoryToWeight {
			weightCtl, ok := drawSubjectRewardPoolW.mapCycleToRewardPackageCategoryWeight[cycleReward.CycleNum]
			if !ok {
				weightCtl = &random.Pool{}
				drawSubjectRewardPoolW.mapCycleToRewardPackageCategoryWeight[cycleReward.CycleNum] = weightCtl
			}
			weightCtl.AddItem(category, weight)

			mapCategoryToRewardWeightInPackage, ok := drawSubjectRewardPoolW.mapCycleToMapCategoryToRewardWeightInPackage[cycleReward.CycleNum]
			if !ok {
				mapCategoryToRewardWeightInPackage = map[uint32]*random.Pool{}
				drawSubjectRewardPoolW.mapCycleToMapCategoryToRewardWeightInPackage[cycleReward.CycleNum] = mapCategoryToRewardWeightInPackage
			}

			rewardWeightCtl, ok := mapCategoryToRewardWeightInPackage[category]
			if !ok {
				rewardWeightCtl = &random.Pool{}
				mapCategoryToRewardWeightInPackage[category] = rewardWeightCtl
			}

			for _, reward := range cycleReward.MapCategoryToRewardPackage[category] {
				rewardWeightCtl.AddItem(reward, reward.Weight)
			}
		}
	}
}

func (s *DrawRewardSys) LogPlayerBehavior(poolType, drawTimes uint32) {
	bytes, err := json.Marshal(map[string]interface{}{
		"pool":      poolType,
		"drawTimes": drawTimes,
	})
	if err != nil {
		s.LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogPoolDrawReward, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})
}

func (s *DrawRewardSys) OnLogin() {
	s.chooseSubRaceRewardPool()
	s.notifySubjectCycleRange()
}

func (s *DrawRewardSys) OnReconnect() {
	s.chooseSubRaceRewardPool()
	s.notifySubjectCycleRange()
}

func init() {
	RegisterSysClass(sysdef.SiDrawReward, func() iface.ISystem {
		return newDrawRewardSys()
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiDrawReward).(*DrawRewardSys); ok && sys.IsOpen() {
			sys.chooseSubRaceRewardPool()
		}
	})

	event.RegSysEvent(custom_id.SeReloadJson, func(args ...interface{}) {
		initDrawRewardWeight()
		manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
			sys := player.GetSysObj(sysdef.SiDrawReward).(*DrawRewardSys)
			sys.initDramaRewardPool()
			sys.initDramaRewardPoolWeight()
			sys.chooseSubRaceRewardPool()
		})
	})

	event.RegActorEvent(custom_id.AeFinishMainQuest, func(player iface.IPlayer, args ...interface{}) {
		sys := player.GetSysObj(sysdef.SiDrawReward).(*DrawRewardSys)
		sys.addDrawDramaRewardByCond(jsondata.AddRewardPackageCondTypeFinishQuest, args[1].(uint32))
		sys.initDramaRewardPoolWeight()
	})

	// C2S_31_11
	net.RegisterSysProto(31, 11, sysdef.SiDrawReward, (*DrawRewardSys).c2sDrawReward)
	// C2S_31_12
	net.RegisterSysProto(31, 12, sysdef.SiDrawReward, (*DrawRewardSys).c2sSwitchSubRaceRewardPool)
}
