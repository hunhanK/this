/**
 * @Author: LvYuMeng
 * @Date: 2024/3/25
 * @Desc:
**/

package lotterylibs

import (
	"encoding/csv"
	"fmt"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"os"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

// LotteryBase 奖库抽奖
type LotteryBase struct {
	Player        iface.IPlayer
	GetLuckTimes  func() uint16
	GetLuckyValEx func() *jsondata.LotteryLuckyValEx

	GetSingleDiamondPrice func() uint32

	RawData       func() *pb3.LotteryData
	AfterDraw     func(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec)
	AfterResetLib func(libId uint32, libConf *jsondata.LotteryLibConf)
}

type LibResult struct {
	LibId         uint32
	AwardPoolConf *jsondata.LotteryLibAwardPool
	OneAwards     jsondata.StdRewardVec
}

type LotteryResult struct {
	LibResult []*LibResult
	Awards    jsondata.StdRewardVec
}

const (
	LotteryLibLucky    = 1 //大奖库
	LotteryLibDiamond  = 2 //钻石消耗库
	LotteryLibCounts   = 3 //抽奖记数库
	LotteryLibTenTimes = 4 //十连大爆库
	LotteryLibNormal   = 5 //普通库

	CurrStage = 1
	CurrRound = 2
	CurrDay   = 3

	CalTypeRate  = 0
	CalTypeWight = 1
)

func (b *LotteryBase) InitData(rawData *pb3.LotteryData) {
	if nil == rawData.StageCount {
		rawData.StageCount = map[uint32]uint32{}
	}
	if nil == rawData.StageTimes {
		rawData.StageTimes = map[uint32]uint32{}
	}
	if nil == rawData.LibTimes {
		rawData.LibTimes = map[uint32]uint32{}
	}
	if nil == rawData.RoundCount {
		rawData.RoundCount = map[uint32]uint32{}
	}
	if nil == rawData.RoundTimes {
		rawData.RoundTimes = map[uint32]uint32{}
	}
	if nil == rawData.DayCount {
		rawData.DayCount = map[uint32]uint32{}
	}
	if nil == rawData.DayTimes {
		rawData.DayTimes = map[uint32]uint32{}
	}
	if nil == rawData.AwardCount {
		rawData.AwardCount = map[uint32]uint32{}
	}
	return
}

func (b *LotteryBase) data() *pb3.LotteryData {
	return b.RawData()
}

func (b *LotteryBase) OnLotteryNewDay() {
	bData := b.data()
	// 清空当天的寻宝次数和标记
	bData.DayDiamond = 0
	bData.DayCount = make(map[uint32]uint32)
	bData.DayTimes = make(map[uint32]uint32)
}

func (b *LotteryBase) calculateLuckyValue(count uint32) {
	bData := b.data()

	// 计算获得幸运值
	luckyTimes := b.GetLuckTimes()
	lotteryCount := bData.GetLotteryCount()

	// 高16位用来计算额外幸运值 低16位计算普通幸运值
	normalCount := utils.Low16(lotteryCount) + uint16(count)
	if luckyTimes > 0 {
		addValue := normalCount / luckyTimes * 1
		if addValue > 0 {
			bData.PlayerLucky += uint32(addValue)
			normalCount %= luckyTimes
		}
	}

	luckyValEx := b.GetLuckyValEx()
	// 计算额外幸运值
	extraCount := utils.High16(lotteryCount) + uint16(count)
	if luckyValEx != nil {
		luckyExTimes := luckyValEx.Times
		values := luckyValEx.Value
		addCount := extraCount / luckyExTimes
		if addCount > 0 {
			luckyValue := uint32(0)
			for i := uint16(0); i < addCount; i++ {
				randomLucky := random.Interval(0, len(values)-1)
				luckyValue += values[randomLucky]
			}
			bData.PlayerLucky += luckyValue
			extraCount %= luckyExTimes
		}
	}
	lotteryCount = utils.Make32(normalCount, extraCount)
	bData.LotteryCount = lotteryCount
}

func (b *LotteryBase) SetDiamondAll(diamond uint32) {
	if diamond == 0 {
		return
	}
	bData := b.data()
	bData.DayDiamond += diamond
	bData.RoundDiamond += diamond
	bData.StageDiamond += diamond
}

func (b *LotteryBase) GetDiamondAll(dType uint32) uint32 {
	bData := b.data()
	switch dType {
	case CurrStage:
		return bData.StageDiamond
	case CurrRound:
		return bData.RoundDiamond
	case CurrDay:
		return bData.DayDiamond
	}
	return 0
}

// 添加抽奖次数的地方统一，命中后，统一清0,当期/当轮/当天的清零分开
func (b *LotteryBase) SetLotteryCountAll(libId uint32, clear bool) {
	bData := b.data()
	if clear {
		bData.RoundCount[libId] = 0
		bData.DayCount[libId] = 0
		bData.StageCount[libId] = 0
	} else {
		bData.RoundCount[libId]++
		bData.DayCount[libId]++
		bData.StageCount[libId]++
	}
}

func (b *LotteryBase) GetLotteryCountAll(libId, cType uint32) uint32 {
	bData := b.data()
	switch cType {
	case CurrStage:
		return bData.StageCount[libId]
	case CurrRound:
		return bData.RoundCount[libId]
	case CurrDay:
		return bData.DayCount[libId]
	}
	return 0
}

// 如果配置中没用到次数，就不记录，减少要存的数据
func (b *LotteryBase) isCountConfig(line *jsondata.LotteryLibConf) bool {
	if line.StageCount > 0 || line.RoundCount > 0 || line.DayCount > 0 ||
		line.MaxStageCount > 0 || line.MaxRoundCount > 0 || line.MaxDayCount > 0 {
		return true
	}
	return false
}

// 添加可进次数的地方统一，清零的地方分开
func (b *LotteryBase) SetLibTimesCountAll(libId uint32) {
	bData := b.data()
	bData.RoundTimes[libId]++
	bData.DayTimes[libId]++
	bData.StageTimes[libId]++
}

func (b *LotteryBase) GetLibTimesCountAll(libId, tType uint32) uint32 {
	bData := b.data()
	switch tType {
	case CurrStage:
		return bData.StageTimes[libId]
	case CurrRound:
		return bData.RoundTimes[libId]
	case CurrDay:
		return bData.DayTimes[libId]
	}
	return 0
}

func (b *LotteryBase) GetGuaranteeCount(libId uint32) uint32 {
	bData := b.data()
	nowLibId := b.GetLibId(libId)
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		return 0
	}
	switch libConf.LibType {
	case LotteryLibLucky:
		return bData.GetPlayerLucky()
	case LotteryLibCounts:
		return bData.StageCount[nowLibId]
	}
	return 0
}

func (b *LotteryBase) ResetLibAll(libId uint32) bool {
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		logger.LogError("lib conf %d is nil", libId)
		return false
	}
	for _, rId := range libConf.ReplaceId {
		b.ResetLib(rId)
	}
	b.ResetLib(libId)
	return true
}

func (b *LotteryBase) CanLibUse(libId uint32, checkReplace bool) bool {
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		logger.LogError("lib conf %d is nil", libId)
		return false
	}

	stageTime := b.GetLibTimesCountAll(libId, CurrStage)
	if libConf.MaxStageTimes == 0 || libConf.MaxStageTimes > stageTime {
		return true
	}

	if !checkReplace {
		return false
	}

	for _, rId := range libConf.ReplaceId {
		rConf := b.Player.GetDrawLibConf(rId)
		if nil == rConf {
			continue
		}
		rStageTime := b.GetLibTimesCountAll(rId, CurrStage)
		if rConf.MaxStageTimes > 0 && rConf.MaxStageTimes <= rStageTime {
			continue
		}
		return true
	}

	return false
}

func (b *LotteryBase) GetLibId(libId uint32) uint32 {
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		logger.LogError("lib conf %d is nil", libId)
		return 0
	}
	if len(libConf.ReplaceId) <= 0 {
		return libId
	}
	stageTime := b.GetLibTimesCountAll(libId, CurrStage)
	if libConf.MaxStageTimes > 0 && libConf.MaxStageTimes <= stageTime { // 当期可进次数已满
		for _, rId := range libConf.ReplaceId {
			rConf := b.Player.GetDrawLibConf(rId)
			if nil == rConf {
				continue
			}
			rStageTime := b.GetLibTimesCountAll(rId, CurrStage)
			if rConf.MaxStageTimes > 0 && rConf.MaxStageTimes <= rStageTime {
				continue
			}
			return rId
		}
	}
	return libId
}

func (b *LotteryBase) DoDraw(count, useDiamondCount uint32, libraryIds []uint32) *LotteryResult {
	bData := b.data()
	bData.Times += count

	singlePrice := b.GetSingleDiamondPrice()

	var libResult []*LibResult
	var rewardVec []jsondata.StdRewardVec
	for i := uint32(0); i < count; i++ {
		// 记录消耗的钻石,先消耗物品，后算钻石
		if count-1-i < useDiamondCount {
			b.SetDiamondAll(singlePrice)
		}

		b.calculateLuckyValue(1)

		for _, srcId := range libraryIds {
			libId := b.GetLibId(srcId)

			var randomAward *jsondata.LotteryLibAwardPool
			libConf := b.Player.GetDrawLibConf(libId)
			if nil == libConf {
				logger.LogError("【奖库】没有找到寻宝库配置 库id:%d", libId)
				continue
			}

			if b.isCountConfig(libConf) {
				b.SetLotteryCountAll(libId, false) // 抽奖次数 +1
			}

			// 计算终身进入次数
			maxTimes := libConf.LifeLongTimes
			if maxTimes > 0 && bData.LibTimes[libId] >= maxTimes {
				continue
			}

			// 库是否命中
			libRes := b.checkMatchLib(libId, libConf)
			if !libRes {
				continue
			}

			// 奖励是否命中
			randomAward = b.getAwardForPool(libConf.AwardPool, libConf.CalcType)
			if randomAward == nil {
				logger.LogDebug("【奖库】没有随机出来 库id :%d", libId)
				continue
			}

			var reward jsondata.StdRewardVec
			if fn, ok := GenLotteryAwards(randomAward.RegAwardType); ok {
				reward = fn(libId, libConf, randomAward)
			} else {
				reward = randomAward.Awards
			}

			reward = engine.FilterRewardByPlayer(b.Player, reward)

			if len(reward) == 0 {
				logger.LogError("奖励池【%d】不存在奖励", randomAward.Id)
				continue
			}

			// 拿到奖励后数据处理
			b.SetLibTimesCountAll(libId) // 该库的命中次数 +1

			b.afterHitLib(libId, libConf)

			if maxTimes > 0 {
				bData.LibTimes[libId]++
			}

			if randomAward.Count > 0 {
				bData.AwardCount[randomAward.Id]++
			}

			rewardVec = append(rewardVec, reward)

			libResult = append(libResult, &LibResult{
				LibId:         libId,
				AwardPoolConf: randomAward,
				OneAwards:     reward,
			})

			if nil != b.AfterDraw {
				b.AfterDraw(libId, libConf, randomAward, reward)
			}

			b.checkReplaceLoop(srcId, libId)

			break
		}
	}

	rewards := jsondata.MergeStdReward(rewardVec...)
	// 返回结果
	result := &LotteryResult{
		LibResult: libResult,
		Awards:    rewards,
	}

	return result
}

func (b *LotteryBase) ReduceExplodeTimeCount() {
	bData := b.data()
	if bData.ExplodeTimes <= 0 {
		return
	}
	bData.ExplodeTimes = bData.ExplodeTimes - 1
}

func (b *LotteryBase) ClearRoundDataAll() {
	bData := b.data()
	bData.RoundDiamond = 0
	bData.RoundCount = make(map[uint32]uint32)
	bData.RoundTimes = make(map[uint32]uint32)
}

func (b *LotteryBase) afterHitLib(libId uint32, libConf *jsondata.LotteryLibConf) {
	bData := b.data()
	switch libConf.LibType {
	case LotteryLibLucky:
		// 清空幸运值，十连大爆-1，当轮相关的数据
		curLucky := bData.PlayerLucky
		if curLucky >= libConf.MaxLucky {
			curLucky = curLucky - libConf.MaxLucky
			bData.PlayerLucky = curLucky
		} else {
			bData.PlayerLucky = 0
		}
		b.ReduceExplodeTimeCount()
		b.ClearRoundDataAll()
	case LotteryLibDiamond:
		// 十连大爆-1
		b.ReduceExplodeTimeCount()
	case LotteryLibCounts:
		// 十连大爆-1
		b.ReduceExplodeTimeCount()
		b.SetLotteryCountAll(libId, true) // 该库抽奖次数清零
	case LotteryLibTenTimes:
		// 十连大爆-1
		b.ReduceExplodeTimeCount()
		b.SetLotteryCountAll(libId, true) // 该库抽奖次数清零
	case LotteryLibNormal:
		// 十连大爆-1
		b.ReduceExplodeTimeCount()
	}

	if libConf.LibType != LotteryLibLucky && libConf.HitLuckyClear > 0 { //非幸运库按指定值扣取幸运值
		curLucky := bData.PlayerLucky
		if curLucky >= libConf.HitLuckyClear {
			curLucky = curLucky - libConf.HitLuckyClear
			bData.PlayerLucky = curLucky
		} else {
			bData.PlayerLucky = 0
		}
	}
}

const (
	LotteryResetTypeEnd = 1
	LotteryResetTypeAll = 2
)

func (b *LotteryBase) checkReplaceLoop(libId, childId uint32) (reset bool) {
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		return
	}

	if libConf.LoopType == 0 {
		return
	}

	mainLast := len(libConf.ReplaceId) == 0 && libId == childId
	subLast := len(libConf.ReplaceId) > 0 && libConf.ReplaceId[len(libConf.ReplaceId)-1] == childId

	if !mainLast && !subLast {
		return
	}

	if b.CanLibUse(libId, true) {
		return
	}

	switch libConf.LoopType {
	case LotteryResetTypeEnd:
		b.ResetLib(childId)
	case LotteryResetTypeAll:
		b.ResetLibAll(libId)
	default:
		return false
	}

	reset = true

	return
}

func (b *LotteryBase) ResetLib(libId uint32) {
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		logger.LogError("【奖库】没有找到寻宝库配置 库id:%d", libId)
		return
	}

	bData := b.data()

	delete(bData.StageCount, libId)
	delete(bData.StageTimes, libId)
	delete(bData.LibTimes, libId)
	delete(bData.RoundCount, libId)
	delete(bData.RoundTimes, libId)
	delete(bData.DayCount, libId)
	delete(bData.DayTimes, libId)

	for _, v := range libConf.AwardPool {
		delete(bData.AwardCount, v.Id)
	}

	if libConf.LibType == LotteryLibLucky {
		bData.PlayerLucky = 0
	}

	if nil != b.AfterResetLib {
		b.AfterResetLib(libId, libConf)
	}

	logworker.LogPlayerBehavior(b.Player, pb3.LogId_LogLotteryReset, &pb3.LogPlayerCounter{
		NumArgs: uint64(libId),
	})
	return
}

// GetNotGetAwards 获取未获得的限定奖励
func (b *LotteryBase) GetNotGetAwards(libId uint32) (result *LotteryResult) {
	result = &LotteryResult{}
	libConf := b.Player.GetDrawLibConf(libId)
	if nil == libConf {
		return
	}

	if !b.CanLibUse(libId, false) {
		return
	}

	var rewardVec []jsondata.StdRewardVec
	bData := b.data()

	for _, v := range libConf.AwardPool {
		awardConf := v
		if awardConf.Count == 0 {
			continue
		}
		if bData.AwardCount[awardConf.Id] >= awardConf.Count {
			continue
		}
		count := awardConf.Count - bData.AwardCount[awardConf.Id]
		for i := uint32(1); i <= count; i++ {
			result.LibResult = append(result.LibResult, &LibResult{
				LibId:         libId,
				AwardPoolConf: awardConf,
				OneAwards:     awardConf.Awards,
			})
			rewardVec = append(rewardVec, awardConf.Awards)
		}
	}

	result.Awards = jsondata.AppendStdReward(rewardVec...)

	return result
}

func (b *LotteryBase) getAwardForPool(awardPool []*jsondata.LotteryLibAwardPool, calcType uint32) *jsondata.LotteryLibAwardPool {
	// 不判断概率必得东西
	bData := b.data()
	var randomAward *jsondata.LotteryLibAwardPool
	if calcType == CalTypeWight {
		// 权重获得东西
		randHelper := random.Pool{}
		for _, v := range awardPool {
			if v.Count > 0 && bData.AwardCount[v.Id] >= v.Count {
				continue
			}
			randHelper.AddItem(v, v.Weight)
		}
		if randHelper.Size() == 0 {
			return nil
		}
		randomAward = randHelper.RandomOne().(*jsondata.LotteryLibAwardPool)
	} else {
		// 概率获得东西
		for _, v := range awardPool {
			if v.Count > 0 && bData.AwardCount[v.Id] >= v.Count {
				continue
			}
			if v.Rate == 0 {
				logger.LogError("award pool id %d rate is zero", v.Id)
				continue
			}
			if random.Hit(1, v.Rate) && len(v.Awards) >= 1 {
				randomAward = v
				break
			}
		}
	}
	return randomAward
}

// 库命中判断
func (b *LotteryBase) checkMatchLib(libId uint32, line *jsondata.LotteryLibConf) bool {
	bData := b.data()
	switch line.LibType {
	case LotteryLibLucky:
		curLucky := bData.PlayerLucky
		if curLucky >= line.MaxLucky {
			return true
		} else if curLucky >= line.MinLucky {
			if random.Hit(1, line.Rate) {
				return true
			}
		}
	case LotteryLibDiamond:
		roundDiamond := b.GetDiamondAll(CurrRound)
		if line.RoundDiamond > roundDiamond { // 当轮钻石消耗不够
			return false
		}
		dayDiamond := b.GetDiamondAll(CurrDay)
		if line.DayDiamond > dayDiamond { // 当天钻石消耗不够
			return false
		}
		stageDiamond := b.GetDiamondAll(CurrStage)
		if line.StageDiamond > stageDiamond { // 当期钻石消耗不够
			return false
		}
		roundTime := b.GetLibTimesCountAll(libId, CurrRound)
		if line.MaxRoundTimes <= roundTime { // 当轮可进次数已满
			return false
		}
		dayTime := b.GetLibTimesCountAll(libId, CurrDay)
		if line.MaxDayTimes <= dayTime { // 当天可进次数已满
			return false
		}
		stageTime := b.GetLibTimesCountAll(libId, CurrStage)
		if line.MaxStageTimes <= stageTime { // 当期可进次数已满
			return false
		}
		if random.Hit(1, line.Rate) {
			return true
		}
	case LotteryLibCounts:
		roundCount := b.GetLotteryCountAll(libId, CurrRound)
		rcFlag := roundCount >= line.RoundCount // 当轮次数达标

		dayCount := b.GetLotteryCountAll(libId, CurrDay)
		dcFlag := dayCount >= line.DayCount // 当天次数达标

		stageCount := b.GetLotteryCountAll(libId, CurrStage)
		scFlag := stageCount >= line.StageCount // 当期次数达标

		roundTime := b.GetLibTimesCountAll(libId, CurrRound)
		rtFlag := roundTime < line.MaxRoundTimes || line.MaxRoundTimes == 0 // 当轮可进次数未满

		dayTime := b.GetLibTimesCountAll(libId, CurrDay)
		dtFlag := dayTime < line.MaxDayTimes || line.MaxDayTimes == 0 // 当轮可进次数未满

		stageTime := b.GetLibTimesCountAll(libId, CurrStage)
		stFlag := stageTime < line.MaxStageTimes || line.MaxStageTimes == 0 // 当轮可进次数未满

		if rcFlag && dcFlag && scFlag && rtFlag && dtFlag && stFlag {
			if line.MaxRoundCount > 0 && roundCount >= line.MaxRoundCount {
				return true
			}
			if line.MaxDayCount > 0 && dayCount >= line.MaxDayCount {
				return true
			}
			if line.MaxStageCount > 0 && stageCount >= line.MaxStageCount {
				return true
			}
			if random.Hit(1, line.Rate) {
				return true
			}
		}
	case LotteryLibTenTimes:
		epTimes := bData.ExplodeTimes
		if epTimes > 0 { // 十连幸运大爆次数足够
			return true
		} else {
			roundCount := b.GetLotteryCountAll(libId, CurrRound)
			if line.RoundCount > roundCount { // 当轮次数不够
				return false
			}
			if random.Hit(1, line.Rate) {
				bData.ExplodeTimes = 10 // 获得大爆 +10
				return true
			}
		}
	case LotteryLibNormal:
		if random.Hit(1, line.Rate) {
			return true
		}
	}
	return false
}

func (b *LotteryBase) GmDraw(count, useDiamondCount uint32, libraryIds []uint32) {
	result := b.DoDraw(count, useDiamondCount, libraryIds)
	f, err := os.Create(utils.GetCurrentDir() + fmt.Sprintf("lottry-%d.csv", time_util.NowSec()))
	defer f.Close()
	if err != nil {
		return
	}
	writer := csv.NewWriter(f)
	var header = []string{"libId", "awardId", "itemId", "name"}
	err = writer.Write(header)
	if err != nil {
		return
	}
	for _, v := range result.LibResult {
		err := writer.Write([]string{utils.I32toa(v.LibId), utils.I32toa(v.AwardPoolConf.Id), utils.I32toa(v.OneAwards[0].Id), jsondata.GetItemName(v.OneAwards[0].Id)})
		if err != nil {
			return
		}
	}
	writer.Flush()
	return
}

const (
	LotteryAwards_SummerSurfDiamond = 1
)

type LotteryAwardsFn func(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool) jsondata.StdRewardVec

var lotteryAwards = map[uint32]LotteryAwardsFn{}

func RegLotteryAwards(awardsType uint32, fn LotteryAwardsFn) {
	if _, ok := lotteryAwards[awardsType]; ok {
		logger.LogError("重复注册")
		return
	}

	lotteryAwards[awardsType] = fn
}

func GenLotteryAwards(awardsType uint32) (LotteryAwardsFn, bool) {
	fn, ok := lotteryAwards[awardsType]
	if !ok {
		return nil, false
	}

	return fn, true
}
