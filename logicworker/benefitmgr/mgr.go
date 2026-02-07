package benefitmgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

func GetBenefitSignLoop() *pb3.BenefitSign {
	if nil == gshare.GetStaticVar().BenefitSign {
		gshare.GetStaticVar().BenefitSign = &pb3.BenefitSign{}
	}
	return gshare.GetStaticVar().BenefitSign
}

// 服务器切天
func benefitSignCheck(args ...interface{}) {
	openServerTime := gshare.GetOpenServerTime()
	serverMonStartTime := time_util.MonthStartTime(openServerTime, 0)
	bsl := GetBenefitSignLoop()
	signLoop := bsl.BenefitSignId
	monStartTime := time_util.MonthStartTime(time_util.NowSec(), 0)
	if monStartTime == serverMonStartTime { //本月服务器开服
		if conf := jsondata.GetBenefitSignConfByOpenDay(1); nil != conf {
			signLoop = conf.Id
			bsl.BenefitSignId = signLoop
			bsl.StartTime = monStartTime
			return
		}
	}
	openZero := time_util.GetZeroTime(openServerTime)
	if openZero > monStartTime {
		return
	}
	days := monStartTime/86400 - openZero/86400 + 1
	if signLoop == 0 {
		if conf := jsondata.GetBenefitSignConfByOpenDay(days); nil != conf {
			signLoop = conf.Id
			bsl.BenefitSignId = signLoop
			bsl.StartTime = monStartTime
			return
		}
	}

	if bsl.BenefitSignId > 0 { //判断是不是换月了
		if bsl.StartTime != monStartTime {
			if conf := jsondata.GetBenefitSignConfByLastConf(bsl.BenefitSignId, days); nil != conf {
				bsl.BenefitSignId = conf.Id
				bsl.StartTime = monStartTime
				return
			}
		} else {
			nowConf := jsondata.GetBenefitSignConfById(bsl.BenefitSignId)
			if nil == nowConf { //配置删除后替补重新读第一轮的
				if conf := jsondata.GetBenefitSignConfByOpenDay(days); nil != conf {
					bsl.BenefitSignId = conf.Id
					bsl.StartTime = monStartTime
					return
				}
			}
		}
	}
	return
}

func init() {
	// 服务器切天
	event.RegSysEvent(custom_id.SeServerInit, benefitSignCheck)
	event.RegSysEvent(custom_id.SeNewDayArrive, benefitSignCheck)
	event.RegSysEvent(custom_id.SeReloadJson, benefitSignCheck)
}
