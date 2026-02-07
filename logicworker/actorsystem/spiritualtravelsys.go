/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 神识游历
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"strings"
	"time"

	"github.com/gzjjyz/srvlib/utils"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
)

type SpiritualTravelSys struct {
	Base
	fastTravelUpLimit  uint32 // 快速游历上限
	travelTimeUpLimit  uint32 // 游历时间上限(单位 秒)
	travelEventUpLimit uint32 // 游历事件上限
}

func (s *SpiritualTravelSys) OnLogin() {
	if !s.IsOpen() {
		s.GetOwner().LogWarn("OnLogin SpiritualTravelSys not opened")
		return
	}
	s.reLoginPubEvent()
	s.reCalcLimit()
	s.S2CInfo()
}

func (s *SpiritualTravelSys) OnReconnect() {
	if !s.IsOpen() {
		s.GetOwner().LogWarn("OnReconnect SpiritualTravelSys not opened")
		return
	}
	s.ResetSysAttr(attrdef.SaSpiritualTravel)
	s.reCalcLimit()
	s.S2CInfo()
}

func (s *SpiritualTravelSys) OnOpen() {
	s.initData()
	s.reCalcLimit()
	s.S2CInfo()
}

func (s *SpiritualTravelSys) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.initData()
}

func (s *SpiritualTravelSys) OnLogout() {
	if !s.IsOpen() {
		return
	}
}

func (s *SpiritualTravelSys) GetData() *pb3.SpiritualTravel {
	binary := s.GetBinaryData()
	if binary.SpiritualTravel == nil {
		binary.SpiritualTravel = new(pb3.SpiritualTravel)
	}
	if binary.SpiritualTravel.HistoryLvTravelTimeMap == nil {
		binary.SpiritualTravel.HistoryLvTravelTimeMap = make(map[uint32]uint32)
	}
	if binary.SpiritualTravel.HistoryLvWeightNumsMap == nil {
		binary.SpiritualTravel.HistoryLvWeightNumsMap = make(map[string]uint32)
	}
	if binary.SpiritualTravel.DailyLimit == nil {
		binary.SpiritualTravel.DailyLimit = make(map[uint32]uint32)
	}
	return binary.SpiritualTravel
}

func (s *SpiritualTravelSys) S2CInfo() {
	s.SendProto3(159, 0, &pb3.S2C_159_0{
		State: s.GetData(),
	})
}

func (s *SpiritualTravelSys) GetLv() uint32 {
	data := s.GetData()
	return data.Lv
}

// 初始化数据
func (s *SpiritualTravelSys) initData() {
	data := s.GetData()
	if data.LastRecAt == 0 {
		data.LastRecAt = time_util.NowSec()
	}
	s.reCalcLimit()
}

// 计算上限
func (s *SpiritualTravelSys) reCalcLimit() {
	conf, ok := jsondata.GetSpiritualTravelLevelConf(s.getLvWithDefault())
	if ok {
		s.fastTravelUpLimit = conf.FastTravelUpLimit
		s.travelTimeUpLimit = conf.TravelTimeUpLimit
		s.travelEventUpLimit = conf.TravelEventUpLimit
	}
}

// 生产事件
func (s *SpiritualTravelSys) genEvent() *pb3.SpiritualTravelEvent {
	eventConf, ok := jsondata.GetSpiritualTravelEventConf()
	if !ok {
		s.GetOwner().LogTrace("not found time event conf")
		return nil
	}

	if len(eventConf.QuestPool) == 0 || len(eventConf.AwardsPool) == 0 {
		s.GetOwner().LogTrace("not found time event quest pool conf %d or awards pool conf %d", len(eventConf.QuestPool), len(eventConf.AwardsPool))
		return nil
	}

	data := s.GetData()
	if len(eventConf.QuestPool) <= len(data.EventQueue) {
		s.GetOwner().LogTrace("not found enough quest pool conf %d", len(eventConf.QuestPool))
		return nil
	}

	var dataQuestIdsSet = make(map[uint32]struct{})
	for _, travelEvent := range data.EventQueue {
		dataQuestIdsSet[travelEvent.QuestId] = struct{}{}
	}
	var questPool = new(random.Pool)
	for idx, quest := range eventConf.QuestPool {
		if _, ok := dataQuestIdsSet[quest.Id]; ok {
			continue
		}
		questPool.AddItem(eventConf.QuestPool[idx], uint32(idx+1))
	}

	var newEvent *pb3.SpiritualTravelEvent
	for i := 0; i < len(eventConf.QuestPool); i++ {
		val := questPool.RandomOne()
		if val == nil {
			s.GetOwner().LogTrace("not found quest pool , val is nil")
			continue
		}
		item := val.(*jsondata.SpiritualTravelEventQuestPool)
		if _, ok := dataQuestIdsSet[item.Id]; ok {
			continue
		}
		newEvent = &pb3.SpiritualTravelEvent{
			QuestId:   item.Id,
			ActorName: item.ActorName,
			Awards:    nil,
		}
		break
	}

	// 没事件
	if newEvent == nil {
		s.GetOwner().LogWarn("actor(%d) gen event not new event", s.GetOwner().GetId())
		return nil
	}

	var awardPoolConf *jsondata.SpiritualTravelEventAwardsPool
	var lv = s.getLvWithDefault()
	for _, pool := range eventConf.AwardsPool {
		if pool.MinLv > lv || pool.MaxLv < lv {
			continue
		}
		awardPoolConf = pool
		break
	}

	if awardPoolConf == nil || len(awardPoolConf.AwardRangeVec) == 0 {
		s.GetOwner().LogWarn("not found awards range vec pool , award is nil")
		return nil
	}

	var awardPool = new(random.Pool)
	s.GetOwner().LogInfo("actor(%d) NotOutRateTime is %d", s.GetOwner().GetId(), data.NotOutRateTime)
	if awardPoolConf.Time != 0 && data.NotOutRateTime+1 >= awardPoolConf.Time {
		// 保底出珍稀库
		for _, vec := range awardPoolConf.AwardRangeVec {
			if vec.IsRare > 0 {
				awardPool.AddItem(vec, vec.Weight)
			}
		}

		// 珍稀库没配
		if awardPool.Size() == 0 {
			for _, vec := range awardPoolConf.AwardRangeVec {
				awardPool.AddItem(vec, vec.Weight)
			}
		}
	} else {
		// 正常抽奖励
		for _, vec := range awardPoolConf.AwardRangeVec {
			awardPool.AddItem(vec, vec.Weight)
		}
	}

	awardsRangeVec := awardPool.RandomOne().(*jsondata.SpiritualTravelEventAwardRange)

	if len(awardsRangeVec.Awards) == 0 {
		s.GetOwner().LogWarn("awardsRangeVec is %v, award is nil", awardsRangeVec)
		return nil
	}

	if awardsRangeVec.IsRare > 0 {
		data.NotOutRateTime = 0
		newEvent.IsRate = true
	} else {
		data.NotOutRateTime = data.NotOutRateTime + 1
	}

	newEvent.Awards = jsondata.StdRewardVecToPb3RewardVec(jsondata.MergeStdReward(awardsRangeVec.Awards))
	return newEvent
}

// 推事件
func (s *SpiritualTravelSys) pubEvent() {
	if s.checkEventQueueFull() {
		s.GetOwner().LogDebug("queue is full")
		return
	}

	newEvent := s.genEvent()
	if newEvent == nil {
		s.GetOwner().LogWarn("newEvent is full")
		return
	}

	data := s.GetData()
	data.EventQueue = append(data.EventQueue, newEvent)
	data.LastPubEventTime = time_util.NowSec()
	s.SendProto3(159, 10, &pb3.S2C_159_10{
		Event: newEvent,
	})
}

// 检查事件队列满了没有
func (s *SpiritualTravelSys) checkEventQueueFull() bool {
	return uint32(len(s.GetData().EventQueue)) >= s.travelEventUpLimit
}

// 补推事件
func (s *SpiritualTravelSys) reLoginPubEvent() {
	loginAt := s.GetMainData().LoginTime
	var logoutAt uint32
	if s.GetOwner() != nil && s.GetOwner().GetMainData() != nil {
		logoutAt = s.GetOwner().GetMainData().LastLogoutTime
	}

	if logoutAt == 0 {
		s.GetOwner().LogWarn("first login not logout")
		return
	}

	if logoutAt >= loginAt {
		s.GetOwner().LogWarn("logout at %d , login at %d", logoutAt, loginAt)
		return
	}

	if s.checkEventQueueFull() {
		s.GetOwner().LogWarn("queue is full")
		return
	}

	var diff = loginAt - logoutAt
	eventConf, ok := jsondata.GetSpiritualTravelEventConf()
	if !ok {
		s.GetOwner().LogWarn("event conf not found")
		return
	}

	if eventConf.PubRate == 0 {
		s.GetOwner().LogWarn("event conf PubRate is zero")
		return
	}

	count := diff / eventConf.PubRate
	for i := uint32(0); i < count; i++ {
		newEvent := s.genEvent()
		// 二次校验
		if s.checkEventQueueFull() {
			s.GetOwner().LogWarn("queue is full")
			break
		}
		if newEvent == nil {
			s.GetOwner().LogWarn("newEvent is full")
			break
		}
		s.GetData().EventQueue = append(s.GetData().EventQueue, newEvent)
	}
}

// 领取游历奖励
func (s *SpiritualTravelSys) c2sRecTravelAwards(msg *base.Message) error {
	var req pb3.C2S_159_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	data := s.GetData()

	// 神识配制
	travelConf, ok := jsondata.GetSpiritualTravelConf()
	if !ok {
		return neterror.ConfNotFoundError("not found travelConf")
	}

	// 时间单位
	if travelConf.TimeInterval == 0 {
		return neterror.ConfNotFoundError("not found travelConf TimeInterval")
	}

	var nowSec = time_util.NowSec()
	var totalTime = uint32(0)
	var toFrontTotalTime = uint32(0)
	if nowSec > data.LastRecAt {
		totalTime = nowSec - data.LastRecAt
		toFrontTotalTime = nowSec - data.LastRecAt
	}

	// 校验下领取的时间间隔
	if totalTime < travelConf.GetRewardTime {
		return neterror.ParamsInvalidError("total time %d < get reward time %d", totalTime, travelConf.GetRewardTime)
	}

	if totalTime > s.travelTimeUpLimit {
		totalTime = s.travelTimeUpLimit
		toFrontTotalTime = s.travelTimeUpLimit
	}

	owner := s.GetOwner()
	level := s.GetOwner().GetMainData().GetLevel()
	exp := s.GetOwner().GetMainData().GetExp()

	// 奖励
	var allRewards jsondata.StdRewardVec

	// 修炼期间升级记录的奖励
	reward, err := s.calcReward(data.Lv, totalTime)
	if err != nil {
		return neterror.Wrap(err)
	}
	allRewards = append(allRewards, reward...)
	allRewards = append(allRewards, s.calcOtherReward(totalTime)...)

	if len(allRewards) > 0 {
		engine.GiveRewards(owner, allRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardSpiritualTravel})
	}

	data.LastRecAt = nowSec
	s.SendProto3(159, 2, &pb3.S2C_159_2{
		TotalTime:      toFrontTotalTime,
		Award:          jsondata.StdRewardVecToPb3RewardVec(jsondata.MergeStdReward(allRewards)),
		LastRecAt:      nowSec,
		BeforeActorLv:  level,
		BeforeActorExp: exp,
	})
	owner.TriggerQuestEvent(custom_id.QttSpiritualTravelRecAwards, 0, 1)

	return nil
}

// 计算奖励
func (s *SpiritualTravelSys) calcReward(lv uint32, totalTime uint32) (jsondata.StdRewardVec, error) {
	// 神识配制
	travelConf, ok := jsondata.GetSpiritualTravelConf()
	if !ok {
		err := neterror.ConfNotFoundError("not found travelConf")
		s.GetOwner().LogWarn("err:%v", err)
		return nil, err
	}

	// 时间单位
	if travelConf.TimeInterval == 0 {
		err := neterror.ConfNotFoundError("not found travelConf TimeInterval")
		s.GetOwner().LogWarn("err:%v", err)
		return nil, err
	}

	// 等级配制
	levelConf, ok := jsondata.GetSpiritualTravelLevelConf(lv)
	if !ok {
		err := neterror.ConfNotFoundError("not found travelConf TimeInterval")
		s.GetOwner().LogWarn("err:%v", err)
		return nil, err
	}

	// 加成效率
	addRateAttr := s.owner.GetFightAttr(attrdef.SpiritualTravelSpeed)

	// 特权加成
	privilegeAddition, _ := s.GetOwner().GetPrivilege(privilegedef.EnumSpiritualTravel)
	addRateAttr += privilegeAddition

	var rewards jsondata.StdRewardVec
	// 固定奖励
	for _, awards := range levelConf.Awards {
		reward := awards.StdReward.Copy()
		reward.Count = reward.Count * int64(totalTime) * (10000 + addRateAttr) / int64(travelConf.TimeInterval) / 10000
		rewards = append(rewards, reward)
	}

	// 合并一下奖励
	rewards = jsondata.MergeStdReward(rewards)

	return rewards, nil
}

func (s *SpiritualTravelSys) calcOtherReward(totalTime uint32) jsondata.StdRewardVec {
	data := s.GetData()
	owner := s.GetOwner()
	// 加成效率
	addRateAttr := owner.GetFightAttr(attrdef.SpiritualTravelSpeed)
	// 特权加成
	privilegeAddition, _ := s.GetOwner().GetPrivilege(privilegedef.EnumSpiritualTravel)
	addRateAttr += privilegeAddition

	// 神识配制
	travelConf, ok := jsondata.GetSpiritualTravelConf()
	if !ok {
		return nil
	}

	// 时间单位
	if travelConf.TimeInterval == 0 {
		return nil
	}

	// 计算额外奖励
	var calcExtAwards = func(extAwards jsondata.StdSpiritualTravelAwardsVec) jsondata.StdRewardVec {
		if len(extAwards) == 0 {
			return nil
		}

		// 奖励
		var num = totalTime / travelConf.TimeInterval
		if num == 0 {
			num = 1
		}
		historyLvWeightNumsMap := data.HistoryLvWeightNumsMap

		// 额外奖励
		itemMap := make(map[uint32]jsondata.StdReward)

		// 计算随机奖励
		for _, ext := range extAwards {
			if len(ext.Weights) == 0 {
				continue
			}

			// 保底奖励
			var firstTimeWeight *jsondata.SpiritualTravelAwardWeight
			pool := new(random.Pool)
			for _, weight := range ext.Weights {
				pool.AddItem(weight, weight.Weight)
				if firstTimeWeight == nil && weight.Time > 0 {
					firstTimeWeight = weight
				}
			}

			// 出奖
			for i := uint32(0); i < num; i++ {
				var mk = fmt.Sprintf("%d", ext.Id)
				extWeightReward := ext.StdReward.Copy()

				// 保底
				if firstTimeWeight != nil && historyLvWeightNumsMap[mk]+1 >= firstTimeWeight.Time {
					extWeightReward.Count = int64(firstTimeWeight.Num) * (10000 + addRateAttr) / 10000

					reward, ok := itemMap[extWeightReward.Id]
					if !ok {
						reward = *extWeightReward
						continue
					}
					reward.Count += extWeightReward.Count
					itemMap[extWeightReward.Id] = reward

					historyLvWeightNumsMap[mk] = 0
					continue
				}

				// 抽奖
				awardWeight := pool.RandomOne().(*jsondata.SpiritualTravelAwardWeight)
				if awardWeight.Num == 0 {
					if firstTimeWeight != nil {
						historyLvWeightNumsMap[mk]++
					}
					continue
				}

				// 保底次数
				if firstTimeWeight != nil {
					historyLvWeightNumsMap[mk] = utils.Ternary(awardWeight.Time > 0, uint32(0), historyLvWeightNumsMap[mk]+1).(uint32)
				}
				extWeightReward.Count = int64(awardWeight.Num) * (10000 + addRateAttr) / 10000

				reward, ok := itemMap[extWeightReward.Id]
				if !ok {
					itemMap[extWeightReward.Id] = *extWeightReward
					continue
				}
				reward.Count += extWeightReward.Count
				itemMap[extWeightReward.Id] = reward
			}

		}

		var rewards jsondata.StdRewardVec
		if len(itemMap) > 0 {
			// 额外奖励
			for _, v := range itemMap {
				rewards = append(rewards, &v)
			}
			// 最后合并一下奖励
			rewards = jsondata.MergeStdReward(rewards)
		}

		// 过滤一下上限
		var finRewards jsondata.StdRewardVec
		for _, v := range rewards {
			if v.Count == 0 {
				continue
			}

			limitConf := jsondata.GetSpiritualTravelItemLimitConf(v.Id)
			if limitConf == nil {
				finRewards = append(finRewards, v)
				continue
			}

			if limitConf.DailyLimit == 0 {
				finRewards = append(finRewards, v)
				continue
			}

			daily := data.DailyLimit[v.Id]
			if limitConf.DailyLimit <= daily {
				continue
			}

			if limitConf.DailyLimit > daily+uint32(v.Count) {
				data.DailyLimit[v.Id] = daily + uint32(v.Count)
				finRewards = append(finRewards, v)
				continue
			}

			v.Count = int64(limitConf.DailyLimit - daily)
			data.DailyLimit[v.Id] = limitConf.DailyLimit
			finRewards = append(finRewards, v)
		}
		finRewards = jsondata.MergeStdReward(finRewards)
		return finRewards
	}

	// 涅槃等级奖励
	var calcNirvanaLvAwards = func() jsondata.StdRewardVec {
		level := owner.GetNirvanaLevel()
		subLevel := owner.GetNirvanaSubLevel()

		// 是否满足条件
		if sys := owner.GetSysObj(sysdef.SiNirvana).(*NirvanaSys); sys != nil {
			if !sys.checkNextCond(level+1, subLevel+1) {
				return nil
			}
		}

		nirvanaLvConf := jsondata.GetSpiritualTravelNirvanaLvConf(level, subLevel)
		if nirvanaLvConf == nil {
			return nil
		}

		var rewards jsondata.StdRewardVec
		var extAwards []*jsondata.SpiritualTravelAwards
		// 固定奖励
		for _, awards := range nirvanaLvConf.Awards {
			reward := awards.StdReward.Copy()
			if reward.Count != 0 {
				reward.Count = reward.Count * int64(totalTime) * (10000 + addRateAttr) / int64(travelConf.TimeInterval) / 10000
				rewards = append(rewards, reward)
			}
			if len(awards.Weights) > 0 {
				extAwards = append(extAwards, awards)
			}
		}
		if len(extAwards) > 0 {
			rewards = append(rewards, calcExtAwards(extAwards)...)
		}
		return rewards
	}

	// 境界等级奖励
	var calcJingJieAwards = func() jsondata.StdRewardVec {
		lv := owner.GetCircle()
		jingJieConf := jsondata.GetSpiritualTravelJingJieConf(lv)
		if jingJieConf == nil {
			return nil
		}

		var rewards jsondata.StdRewardVec
		var extAwards []*jsondata.SpiritualTravelAwards
		// 固定奖励
		for _, awards := range jingJieConf.Awards {
			reward := awards.StdReward.Copy()
			if reward.Count != 0 {
				reward.Count = reward.Count * int64(totalTime) * (10000 + addRateAttr) / int64(travelConf.TimeInterval) / 10000
				rewards = append(rewards, reward)
			}
			if len(awards.Weights) > 0 {
				extAwards = append(extAwards, awards)
			}
		}
		if len(extAwards) > 0 {
			rewards = append(rewards, calcExtAwards(extAwards)...)
		}
		return rewards
	}

	// 角色等级奖励
	var calcActorLvAwards = func() jsondata.StdRewardVec {
		lv := owner.GetLevel()
		lvConf := jsondata.GetSpiritualTravelActorLvConf(lv)
		if lvConf == nil {
			return nil
		}

		var rewards jsondata.StdRewardVec
		var extAwards []*jsondata.SpiritualTravelAwards
		// 固定奖励
		for _, awards := range lvConf.Awards {
			reward := awards.StdReward.Copy()
			if reward.Count != 0 {
				reward.Count = reward.Count * int64(totalTime) * (10000 + addRateAttr) / int64(travelConf.TimeInterval) / 10000
				rewards = append(rewards, reward)
			}
			if len(awards.Weights) > 0 {
				extAwards = append(extAwards, awards)
			}
		}
		if len(extAwards) > 0 {
			rewards = append(rewards, calcExtAwards(extAwards)...)
		}
		return rewards
	}

	var rewards jsondata.StdRewardVec

	// 涅槃等级奖励
	rewards = append(rewards, calcNirvanaLvAwards()...)

	// 境界等级奖励
	rewards = append(rewards, calcJingJieAwards()...)

	// 角色等级奖励
	rewards = append(rewards, calcActorLvAwards()...)

	// 合并一下奖励
	rewards = jsondata.MergeStdReward(rewards)

	return rewards
}

func (s *SpiritualTravelSys) getLvWithDefault() uint32 {
	data := s.GetData()
	lv := data.Lv
	if lv == 0 {
		lv = 1
	}
	return lv
}

// 快速游历
func (s *SpiritualTravelSys) c2sRecFastTravelAwards(msg *base.Message) error {
	var req pb3.C2S_159_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	data := s.GetData()

	// 今日达上限
	if data.DailyFastTravelNum >= s.fastTravelUpLimit {
		s.GetOwner().LogWarn("not found travelConf")
		return nil
	}

	// 快速游历配制
	fastTravelConf, ok := jsondata.GetSpiritualFastTravelConf(data.DailyFastTravelNum + 1)
	if !ok {
		err := neterror.ConfNotFoundError("not found fastTravelConf")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	if fastTravelConf.Time == 0 {
		err := neterror.ConfNotFoundError("not found fastTravelConf time")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}
	owner := s.GetOwner()
	level := owner.GetMainData().GetLevel()
	exp := owner.GetMainData().GetExp()

	if len(fastTravelConf.Consume) > 0 {
		if !owner.ConsumeByConf(fastTravelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeFastSpiritualTravel}) {
			s.GetOwner().LogWarn("consume failed")
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return nil
		}
	}

	s.GetOwner().TriggerQuestEvent(custom_id.QttFastTravelTimes, 0, 1)

	var rewards, err = s.calcReward(s.getLvWithDefault(), fastTravelConf.Time)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	rewards = append(rewards, s.calcOtherReward(fastTravelConf.Time)...)

	if len(rewards) > 0 {
		engine.GiveRewards(owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardFastSpiritualTravel})
	}

	data.DailyFastTravelNum += 1
	s.SendProto3(159, 3, &pb3.S2C_159_3{
		TotalTime:          fastTravelConf.Time,
		Award:              jsondata.StdRewardVecToPb3RewardVec(jsondata.MergeStdReward(rewards)),
		DailyFastTravelNum: data.DailyFastTravelNum,
		BeforeActorLv:      level,
		BeforeActorExp:     exp,
	})

	return nil
}

// 游历事件
func (s *SpiritualTravelSys) c2sRecTravelEventAwards(msg *base.Message) error {
	var req pb3.C2S_159_4
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	eventConf, ok := jsondata.GetSpiritualTravelEventConf()
	if !ok {
		return neterror.ConfNotFoundError("not found eventConf")
	}

	var newQueue []*pb3.SpiritualTravelEvent
	var event *pb3.SpiritualTravelEvent

	// 重置队列
	data := s.GetData()
	for i := range data.EventQueue {
		if data.EventQueue[i].QuestId != req.QuestId {
			newQueue = append(newQueue, data.EventQueue[i])
			continue
		}
		event = data.EventQueue[i]
	}

	if event == nil {
		s.GetOwner().LogWarn("not found event quest id is %d", req.QuestId)
		data.EventQueue = newQueue
		return nil
	}

	stdRewardVec := jsondata.Pb3RewardVecToStdRewardVec(event.Awards)
	if event.IsRate {
		s.broadcastEventAwardsTips(eventConf.TipsId, event.ActorName, stdRewardVec)
	}
	s.GetOwner().TriggerQuestEvent(custom_id.QttSpiritualTravelRandomTimes, 0, 1)
	engine.GiveRewards(s.GetOwner(), stdRewardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAwardEventSpiritualTravel})

	s.GetData().EventQueue = newQueue

	s.SendProto3(159, 4, &pb3.S2C_159_4{
		QuestId: event.QuestId,
		Award:   event.Awards,
	})
	return nil
}

// 广播事件稀有奖励
func (s *SpiritualTravelSys) broadcastEventAwardsTips(tipsId uint32, actorName string, awards jsondata.StdRewardVec) {
	if len(awards) == 0 {
		return
	}

	var ns []string
	for _, reward := range awards {
		itemConfig := jsondata.GetItemConfig(reward.Id)
		if itemConfig == nil {
			s.GetOwner().LogWarn("broadcastEventAwardsTips item not found , reward id is %v", reward)
			continue
		}
		ns = append(ns, itemConfig.Name)
	}
	unique := pie.Strings(ns).Unique()
	if len(unique) > 0 {
		engine.BroadcastTipMsgById(tipsId, s.GetOwner().GetName(), actorName, strings.Join(unique, ","))
	}
}

const (
	MergeTypeByCross  = 1
	MergeTypeByNormal = 3
)

func (s *SpiritualTravelSys) c2sSpiritualWindowRank(msg *base.Message) error {
	var req pb3.C2S_159_5
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	var rsp = &pb3.S2C_159_5{
		ChartsRank: &pb3.SpiritualWindowRank{},
		GuildRank: &pb3.SpiritualWindowRank{
			RankType: gshare.RankTypeGuild,
		},
		MergeType:            MergeTypeByNormal,
		SpiritualTravelState: s.GetData(),
	}

	conf, ok := jsondata.GetSpiritualWindowConf()
	if !ok {
		err := neterror.ConfNotFoundError("conf not found")
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}

	openServerDay := gshare.GetOpenServerDay()
	var chart *jsondata.SpiritualWindowChart
	for _, c := range conf.Charts {
		if !(c.MinDay <= openServerDay && openServerDay <= c.MaxDay) {
			continue
		}
		chart = c
		break
	}

	if chart != nil {
		rankLine := manager.GRankMgrIns.GetRankByType(chart.Type)
		rsp.ChartsRank.RankType = chart.Type
		first := rankLine.GetList(1, 1)
		if len(first) == 1 {
			topPlayer := first[0]
			info := &pb3.RankInfo{}
			val := topPlayer.GetScore()
			info.Value = val
			info.Rank = 1
			info.Key = topPlayer.GetId()
			info.ExtVal = manager.DecodeExVal(chart.Type, topPlayer.GetScore())
			manager.SetPlayerRankInfo(topPlayer.GetId(), info, chart.Type)
			rsp.ChartsRank.Rank = append(rsp.ChartsRank.Rank, info)
		}
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeGuild)
	rankLine := rank.GetList(1, 3)
	for rank, item := range rankLine {
		info := &pb3.RankInfo{}
		if guild := guildmgr.GetGuildById(item.GetId()); nil != guild {
			info.GuildName = guild.GetName()
			info.Value = item.GetScore()
			info.Rank = uint32(rank)
			info.Key = item.GetId()
			info.ExtVal = manager.DecodeExVal(gshare.RankTypeGuild, item.GetScore())
			rsp.GuildRank.Rank = append(rsp.GuildRank.Rank, info)
		}
	}

	s.SendProto3(159, 5, rsp)
	return nil
}

// 跨天
func onSpiritualTravelNewDay(player iface.IPlayer, _ ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSpiritualTravel).(*SpiritualTravelSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.GetData().DailyFastTravelNum = 0
	sys.GetData().DailyLimit = make(map[uint32]uint32)
	sys.S2CInfo()
}

// 重新计算属性
func calcSpiritualTravelSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiSpiritualTravel).(*SpiritualTravelSys)
	if !s.IsOpen() {
		return
	}
	var attrs jsondata.AttrVec

	conf, ok := jsondata.GetSpiritualTravelLevelConf(s.GetLv())
	if !ok {
		s.GetOwner().LogWarn("not found lv conf lv is %d", s.GetLv())
		return
	}

	if len(conf.Attrs) > 0 {
		attrs = append(attrs, conf.Attrs...)
	}

	// 加属性
	if len(attrs) > 0 {
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func handleSpiritualTravelEventPush(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiSpiritualTravel)
	if obj == nil {
		return
	}
	if !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritualTravelSys)
	if !ok {
		return
	}
	sys.pubEvent()
}

func onFairyMasterTrainLevelChange(player iface.IPlayer, args ...interface{}) {
	obj := player.GetSysObj(sysdef.SiSpiritualTravel)
	if obj == nil {
		return
	}
	if !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritualTravelSys)
	if !ok {
		return
	}
	sys.reCalcLimit()
	sys.GetData().Lv = uint32(player.GetBinaryData().FairyMasterData.Level)
}

func init() {
	RegisterSysClass(sysdef.SiSpiritualTravel, func() iface.ISystem {
		return &SpiritualTravelSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaSpiritualTravel, calcSpiritualTravelSysAttr)
	engine.RegQuestTargetProgress(custom_id.QttSpiritualTravelLv, QuestSpiritualTravelLv)

	event.RegActorEvent(custom_id.AeNewDay, onSpiritualTravelNewDay)
	event.RegActorEvent(custom_id.AeSpiritualTravelEventPush, handleSpiritualTravelEventPush)
	event.RegActorEvent(custom_id.AeFairyMasterTrainLevelChanged, onFairyMasterTrainLevelChange)

	//net.RegisterSysProtoV2(159, 1, gshare.SiSpiritualTravel, func(sys iface.ISystem) func(*base.Message) error {
	//	return sys.(*SpiritualTravelSys).c2sUpLv
	//})
	net.RegisterSysProtoV2(159, 2, sysdef.SiSpiritualTravel, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritualTravelSys).c2sRecTravelAwards
	})
	net.RegisterSysProtoV2(159, 3, sysdef.SiSpiritualTravel, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritualTravelSys).c2sRecFastTravelAwards
	})
	net.RegisterSysProtoV2(159, 4, sysdef.SiSpiritualTravel, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritualTravelSys).c2sRecTravelEventAwards
	})
	net.RegisterSysProtoV2(159, 5, sysdef.SiSpiritualTravel, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritualTravelSys).c2sSpiritualWindowRank
	})
	gmevent.Register("SpiritualTravelSys.setLastRecAt", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiSpiritualTravel).(*SpiritualTravelSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		sys.GetData().LastRecAt = uint32(time.Now().AddDate(0, 0, -1).Unix())
		sys.S2CInfo()
		return true
	}, 1)
}

func QuestSpiritualTravelLv(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	if sys, ok := actor.GetSysObj(sysdef.SiSpiritualTravel).(*SpiritualTravelSys); ok {
		return sys.GetLv()
	}
	return 0
}
