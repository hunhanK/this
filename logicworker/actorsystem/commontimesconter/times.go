/**
 * @Author: zjj
 * @Date: 2024/11/29
 * @Desc:
**/

package commontimesconter

import "jjyz/base/pb3"

func (c *CommonTimesCounter) deductTimesLogic(subTimes uint32, getTimesFunc GetTimesFunc, deductF DeductTimesFunc) (leftSubTimes uint32) {
	if getTimesFunc == nil || deductF == nil {
		leftSubTimes = subTimes
		return
	}

	leftTimes := getTimesFunc()
	if leftTimes == 0 {
		leftSubTimes = subTimes
		return
	}

	// 计算要从这里扣几次
	deductTimes := subTimes
	if subTimes > leftTimes {
		leftSubTimes = subTimes - leftTimes
		deductTimes = leftTimes
	}

	// 扣次数
	deductF(deductTimes)

	return
}

func (c *CommonTimesCounter) getCommonLeftTimes(timesSt *pb3.CommonTimes) (leftTimes uint32) {
	if timesSt == nil {
		return
	}
	if timesSt.MaxTimes == 0 {
		return
	}
	if timesSt.UseTimes >= timesSt.MaxTimes {
		return
	}
	leftTimes = uint32(timesSt.MaxTimes - timesSt.UseTimes)
	return
}

func (c *CommonTimesCounter) addCommonUseTimes(times uint64, timesSt *pb3.CommonTimes) {
	if timesSt == nil {
		return
	}
	if timesSt.MaxTimes == 0 {
		return
	}
	timesSt.UseTimes += times
	return
}

// DeductTimes 扣除次数
func (c *CommonTimesCounter) DeductTimes(subTimes uint32) (ok bool) {
	if !c.CheckTimeEnough(subTimes) {
		return
	}
	if subTimes == 0 {
		return
	}
	defer func() {
		c.updateCanUseTimes()
	}()
	ok = true

	// 每日道具增加次数
	resultNeedSubTimes := c.deductTimesLogic(subTimes, func() uint32 {
		return c.getCommonLeftTimes(c.Counter.DailyItemAddTimes)
	}, func(times uint32) {
		c.addCommonUseTimes(uint64(times), c.Counter.DailyItemAddTimes)
		c.AddDailyUseTimes(times)
	})
	if resultNeedSubTimes == 0 {
		return
	}

	// 免费次数+其他加成免费次数
	resultNeedSubTimes = c.deductTimesLogic(resultNeedSubTimes, func() uint32 {
		var totalFreeTimes uint32
		if c.onGetFreeTimes != nil {
			totalFreeTimes += c.onGetFreeTimes()
		}
		if c.onGetOtherAddFreeTimes != nil {
			totalFreeTimes += c.onGetOtherAddFreeTimes()
		}
		dailyUseFreeTimes := c.GetDailyUseFreeTimes()
		if dailyUseFreeTimes >= totalFreeTimes {
			return 0
		}
		return totalFreeTimes - dailyUseFreeTimes
	}, func(times uint32) {
		c.AddDailyUseFreeTimes(times)
		c.AddDailyUseTimes(times)
	})
	if resultNeedSubTimes == 0 {
		return
	}

	// 购买次数
	resultNeedSubTimes = c.deductTimesLogic(resultNeedSubTimes, func() uint32 {
		return c.getCommonLeftTimes(c.Counter.BuyTimes)
	}, func(times uint32) {
		c.addCommonUseTimes(uint64(times), c.Counter.BuyTimes)
		c.AddDailyUseTimes(times)
	})
	if resultNeedSubTimes == 0 {
		return
	}

	// 道具增加次数
	resultNeedSubTimes = c.deductTimesLogic(resultNeedSubTimes, func() uint32 {
		return c.getCommonLeftTimes(c.Counter.ItemAddTimes)
	}, func(times uint32) {
		c.addCommonUseTimes(uint64(times), c.Counter.ItemAddTimes)
		c.AddDailyUseTimes(times)
	})
	ok = resultNeedSubTimes == 0
	return
}
