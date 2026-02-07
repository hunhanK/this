/**
 * @Author: zjj
 * @Date: 2024/11/29
 * @Desc:
**/

package commontimesconter

import (
	"fmt"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"sort"
)

func NewCommonTimesCounterData() *pb3.CommonTimesCounter {
	return &pb3.CommonTimesCounter{
		DailyUseTimes:     0,
		BuyTimes:          &pb3.CommonTimes{},
		ItemAddTimes:      &pb3.CommonTimes{},
		RetrievalTimes:    &pb3.CommonTimes{},
		DailyUseFreeTimes: 0,
		DailyItemAddTimes: &pb3.CommonTimes{},
	}
}

type CommonTimesCounter struct {
	owner                         iface.IPlayer
	Counter                       *pb3.CommonTimesCounter
	onGetFreeTimes                GetTimesFunc             // 获取免费次数
	onGetOtherAddFreeTimes        GetTimesFunc             // 其他系统加成免费次数
	onGetDailyBuyTimesUpLimit     GetTimesFunc             // 今日购买上限
	onGetDailyItemAddTimesUpLimit GetTimesFunc             // 今日道具添加上限
	onUpdateCanUseTimes           UpdateTimesAttrFunc      // 更新可用总次数
	onUpdateRetrievalTimes        UpdateRetrievalTimesFunc // 更新找回次数
	onGetLogoutDays               GetTimesFunc             // 获取离线天数
	onGetRetrievalDays            GetTimesFunc             // 获取保留天数
}

type GetTimesFunc func() uint32
type DeductTimesFunc func(times uint32)
type UpdateTimesAttrFunc func(canUseTimes uint32)
type UpdateRetrievalTimesFunc func(retrievalTimes uint32)

type Option func(c *CommonTimesCounter)

func WithOnGetFreeTimes(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetFreeTimes = f
	}
}

func WithOnGetOtherAddFreeTimes(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetOtherAddFreeTimes = f
	}
}

func WithOnGetDailyBuyTimesUpLimit(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetDailyBuyTimesUpLimit = f
	}
}

func WithOnGetDailyItemAddTimesUpLimit(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetDailyItemAddTimesUpLimit = f
	}
}

func WithOnUpdateCanUseTimes(f UpdateTimesAttrFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onUpdateCanUseTimes = f
	}
}

func WithOnUpdateRetrievalTimes(f UpdateRetrievalTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onUpdateRetrievalTimes = f
	}
}

func WithOnGetLogoutDays(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetLogoutDays = f
	}
}
func WithOnGetRetrievalDays(f GetTimesFunc) Option {
	return func(c *CommonTimesCounter) {
		c.onGetRetrievalDays = f
	}
}

func NewCommonTimesCounter(counter *pb3.CommonTimesCounter, opts ...Option) *CommonTimesCounter {
	c := &CommonTimesCounter{
		Counter: counter,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *CommonTimesCounter) Init() error {
	if c.Counter == nil {
		return fmt.Errorf("not counter data")
	}
	if c.Counter.BuyTimes == nil {
		return fmt.Errorf("not buy times data")
	}
	if c.Counter.ItemAddTimes == nil {
		return fmt.Errorf("not item add times data")
	}
	if c.Counter.DailyItemAddTimes == nil {
		return fmt.Errorf("not daily item add times data")
	}
	c.updateCanUseTimes()
	return nil
}

func (c *CommonTimesCounter) NewDay() {
	// 先更新找回次数
	c.Counter.RetrievalTimes.UseTimes = 0
	c.updateRetrievalDailyTimes()

	c.Counter.DailyUseTimes = 0
	c.Counter.DailyUseFreeTimes = 0
	c.Counter.BuyTimes.DailyAddTimes = 0
	c.Counter.ItemAddTimes.DailyAddTimes = 0
	c.Counter.DailyItemAddTimes.Reset()
	c.updateCanUseTimes()
}

// GetLeftTimes 获取剩余次数
func (c *CommonTimesCounter) GetLeftTimes() (leftTimes uint32) {
	var totalFreeTimes uint32
	if c.onGetFreeTimes != nil {
		totalFreeTimes += c.onGetFreeTimes()
	}

	if c.onGetOtherAddFreeTimes != nil {
		totalFreeTimes += c.onGetOtherAddFreeTimes()
	}

	useFreeTimes := c.GetDailyUseFreeTimes()
	if totalFreeTimes > useFreeTimes {
		leftTimes += totalFreeTimes - useFreeTimes
	}

	leftTimes += c.getCommonLeftTimes(c.Counter.BuyTimes)
	leftTimes += c.getCommonLeftTimes(c.Counter.ItemAddTimes)
	leftTimes += c.getCommonLeftTimes(c.Counter.DailyItemAddTimes)
	return
}

func (c *CommonTimesCounter) GetDailyUseTimes() uint32 {
	if c.Counter == nil {
		return 0
	}
	return c.Counter.DailyUseTimes
}
func (c *CommonTimesCounter) GetDailyUseFreeTimes() uint32 {
	if c.Counter == nil {
		return 0
	}
	return c.Counter.DailyUseFreeTimes
}

func (c *CommonTimesCounter) GetDailyBuyTimes() uint32 {
	if c.Counter == nil {
		return 0
	}
	return c.Counter.BuyTimes.DailyAddTimes
}

func (c *CommonTimesCounter) GetDailyItemAddTimes() uint32 {
	if c.Counter == nil {
		return 0
	}
	return c.Counter.ItemAddTimes.DailyAddTimes
}

func (c *CommonTimesCounter) CheckTimeEnough(needSubTimes uint32) bool {
	leftTimes := c.GetLeftTimes()
	if leftTimes < needSubTimes {
		return false
	}
	return true
}

func (c *CommonTimesCounter) AddDailyUseTimes(times uint32) {
	c.Counter.DailyUseTimes += times
}

func (c *CommonTimesCounter) AddDailyUseFreeTimes(times uint32) {
	c.Counter.DailyUseFreeTimes += times
}

func (c *CommonTimesCounter) CheckCanBuyDailyAddTimes(times uint32) bool {
	if c.onGetDailyBuyTimesUpLimit == nil {
		return true
	}
	var upLimit = c.onGetDailyBuyTimesUpLimit()
	if upLimit == 0 {
		return false
	}
	if c.Counter.BuyTimes.DailyAddTimes > upLimit || c.Counter.BuyTimes.DailyAddTimes+times > upLimit {
		return false
	}
	return true
}

func (c *CommonTimesCounter) AddBuyDailyAddTimes(times uint32) bool {
	if !c.CheckCanBuyDailyAddTimes(times) {
		return false
	}
	c.Counter.BuyTimes.DailyAddTimes += times
	c.Counter.BuyTimes.MaxTimes += uint64(times)
	c.updateCanUseTimes()
	return true
}

func (c *CommonTimesCounter) CheckCanAddItemDailyAddTimes(times uint32) bool {
	if c.onGetDailyItemAddTimesUpLimit == nil {
		return true
	}
	upLimit := c.onGetDailyItemAddTimesUpLimit()
	if upLimit != 0 && (c.Counter.ItemAddTimes.DailyAddTimes > upLimit || c.Counter.ItemAddTimes.DailyAddTimes+times > upLimit) {
		return false
	}
	return true
}

func (c *CommonTimesCounter) AddItemDailyAddTimes(times uint32) bool {
	if !c.CheckCanAddItemDailyAddTimes(times) {
		return false
	}
	c.Counter.ItemAddTimes.DailyAddTimes += times
	c.Counter.ItemAddTimes.MaxTimes += uint64(times)
	c.updateCanUseTimes()
	return true
}

func (c *CommonTimesCounter) updateCanUseTimes() {
	if c.onUpdateCanUseTimes == nil {
		return
	}
	leftTimes := c.GetLeftTimes()
	if leftTimes == 0 {
		c.onUpdateCanUseTimes(0)
		return
	}
	c.onUpdateCanUseTimes(leftTimes)
}

func (c *CommonTimesCounter) AddRetrievalDailyAddTimes(times uint32) bool {
	c.Counter.RetrievalTimes.DailyAddTimes += times
	c.updateRetrievalTimes()
	return true
}

func (c *CommonTimesCounter) AddRetrievalUsedTimes(times uint32) bool {
	c.Counter.RetrievalTimes.UseTimes += uint64(times)
	c.updateRetrievalTimes()
	c.updateDailyRetrievalTimes(times)
	return true
}

func (c *CommonTimesCounter) updateRetrievalDailyTimes() bool {
	if c.Counter.DailyRetrievalTimes == nil {
		c.Counter.DailyRetrievalTimes = make(map[uint32]uint32)
	}

	var leftFreeTimes, freeTimes uint32
	if c.onGetFreeTimes != nil {
		leftFreeTimes = c.onGetFreeTimes()
		freeTimes = c.onGetFreeTimes()
	}

	useFreeTimes := c.GetDailyUseFreeTimes()
	if leftFreeTimes >= useFreeTimes {
		leftFreeTimes = leftFreeTimes - useFreeTimes
	}

	lastDaysZeroTime := time_util.GetBeforeDaysZeroTime(1)
	c.Counter.DailyRetrievalTimes[lastDaysZeroTime] = leftFreeTimes

	// x天前的零点
	var retrievalDays uint32
	if c.onGetRetrievalDays != nil {
		retrievalDays = c.onGetRetrievalDays()
	}

	var leftTimes = uint32(0)
	var newMap = make(map[uint32]uint32)
	for lastDay := uint32(1); lastDay <= retrievalDays; lastDay++ {
		dayZeroTime := time_util.GetBeforeDaysZeroTime(lastDay)
		times, ok := c.Counter.DailyRetrievalTimes[dayZeroTime]
		if !ok {
			times = freeTimes
		}
		newMap[dayZeroTime] = times
		leftTimes += times
	}
	c.Counter.DailyRetrievalTimes = newMap
	c.Counter.RetrievalTimes.DailyAddTimes = leftTimes
	c.updateRetrievalTimes()
	return true
}

func (c *CommonTimesCounter) updateRetrievalTimes() {
	if c.onUpdateRetrievalTimes == nil {
		return
	}
	times := c.Counter.RetrievalTimes.DailyAddTimes
	usedTimes := uint32(c.Counter.RetrievalTimes.UseTimes)
	leftTimes := times - usedTimes
	if leftTimes > 0 {
		c.onUpdateRetrievalTimes(leftTimes)
	} else {
		c.onUpdateRetrievalTimes(0)
	}
}

func (c *CommonTimesCounter) updateDailyRetrievalTimes(times uint32) {
	dailyRetrievalTimes := c.Counter.DailyRetrievalTimes
	if dailyRetrievalTimes == nil {
		return
	}
	keys := make([]uint32, 0, len(dailyRetrievalTimes))
	for k := range dailyRetrievalTimes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, ts := range keys {
		if times == 0 {
			break
		}
		if dailyRetrievalTimes[ts] == 0 {
			continue
		}
		if dailyRetrievalTimes[ts] >= times {
			dailyRetrievalTimes[ts] -= times
			times = 0
		} else {
			times -= dailyRetrievalTimes[ts]
			dailyRetrievalTimes[ts] = 0
		}
	}
}

func (c *CommonTimesCounter) GetRetrievalTimes() uint32 {
	if c.Counter == nil || c.Counter.RetrievalTimes == nil {
		return 0
	}

	dailyTimes := c.Counter.RetrievalTimes.DailyAddTimes
	usedTimes := uint32(c.Counter.RetrievalTimes.UseTimes)

	if dailyTimes > usedTimes {
		return dailyTimes - usedTimes
	} else {
		return 0
	}
}

func (c *CommonTimesCounter) ReCalcTimes() {
	c.updateCanUseTimes()
}

func (c *CommonTimesCounter) AddDailyItemAddTimes(times uint32) bool {
	c.Counter.DailyItemAddTimes.DailyAddTimes += times
	c.Counter.DailyItemAddTimes.MaxTimes += uint64(times)
	c.updateCanUseTimes()
	return true
}
