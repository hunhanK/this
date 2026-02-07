/**
 * @Author: zjj
 * @Date: 2025/2/10
 * @Desc: 招财进宝
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type LuckyTreasuresSys struct {
	Base
}

func (s *LuckyTreasuresSys) s2cInfo() {
	treasures := manager.GetLuckyTreasures()
	data := treasures.GetData()
	s.SendProto3(8, 60, &pb3.S2C_8_60{
		Times:        data.Times,
		StartAt:      data.StartAt,
		EndAt:        data.EndAt,
		TimesConfIdx: data.TimesConfIdx,
		Data:         s.getData(),
	})
}

func (s *LuckyTreasuresSys) getData() *pb3.LuckyTreasuresData {
	treasures := manager.GetLuckyTreasures()
	return treasures.GetPlayerData(s.owner.GetId())
}

func (s *LuckyTreasuresSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LuckyTreasuresSys) OnLogin() {
	s.s2cInfo()
}

func (s *LuckyTreasuresSys) OnOpen() {
	s.s2cInfo()
}

func (s *LuckyTreasuresSys) OnNewDay() {
	s.s2cInfo()
}

func (s *LuckyTreasuresSys) c2sInput(msg *base.Message) error {
	var req pb3.C2S_8_61
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	err = manager.GetLuckyTreasures().Input(s.owner)
	if err != nil {
		s.owner.LogError("err:%v", err)
		return err
	}
	return nil
}
func (s *LuckyTreasuresSys) c2sRecDailyAwards(msg *base.Message) error {
	var req pb3.C2S_8_62
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	err = manager.GetLuckyTreasures().RecDailyAwards(s.owner)
	if err != nil {
		s.owner.LogError("err:%v", err)
		return err
	}
	return nil
}

func luckyTreasuresOnNewDay(forceOpenNext bool) {
	data := manager.GetLuckyTreasures().GetData()
	nowSec := time_util.NowSec()
	conf := jsondata.GetLuckyTreasuresConf()
	if conf == nil {
		logger.LogError("conf not found")
		return
	}

	var resetData = func(times, confIdx uint32) {
		data.Times = times
		data.StartAt = nowSec
		data.EndAt = time_util.GetDaysZeroTime(conf.DurationDays) - 1
		data.TimesConfIdx = confIdx
		data.PlayerData = make(map[uint64]*pb3.LuckyTreasuresData)
		data.IsSettle = false
	}

	// 强制开启下一期
	if forceOpenNext {
		lastConf := jsondata.GetLuckyTreasuresTimesConfOrLastConf(data.Times + 1)
		if lastConf == nil {
			logger.LogError("%d time conf not found", data.Times+1)
			return
		}
		resetData(data.Times+1, lastConf.Times)
		return
	}

	// 首次开启
	if data.Times == 0 {
		if conf.OpenSrvDay > gshare.GetOpenServerDay() {
			return
		}
		lastConf := jsondata.GetLuckyTreasuresTimesConfOrLastConf(1)
		if lastConf == nil {
			logger.LogError("%d time conf not found", 1)
			return
		}
		resetData(1, lastConf.Times)
		return
	}

	// 还没结束
	if nowSec < data.EndAt {
		return
	}

	// 已经结束 但是没到下一期开启
	nextStartAt := time_util.GetZeroTime(data.EndAt+1) + 86400*conf.IntervalDays
	if nowSec < nextStartAt {
		return
	}

	// 下一期可以开启
	lastConf := jsondata.GetLuckyTreasuresTimesConfOrLastConf(data.Times + 1)
	if lastConf == nil {
		logger.LogError("%d time conf not found", data.Times+1)
		return
	}

	// 结算
	luckyTreasuresSettle()

	// 初始化下一期数据
	resetData(data.Times+1, lastConf.Times)
}

func luckyTreasuresSettle() {
	data := manager.GetLuckyTreasures().GetData()
	if data.IsSettle {
		return
	}

	tConf := jsondata.GetLuckyTreasuresConf()
	if tConf == nil {
		logger.LogError("%d time conf not found", data.Times)
		return
	}

	conf, err := manager.GetLuckyTreasures().GetTimesConf()
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	data.IsSettle = true

	t := time.Unix(int64(data.EndAt), 0)
	format := t.AddDate(0, 0, 1).Format("20060102")
	lastCanRecDay := utils.AtoUint32(format)

	for playerId, treasuresData := range data.PlayerData {
		firstJoinDay := treasuresData.FirstJoinDay
		lastRecDay := treasuresData.LastRecDay

		// 没投入过
		if firstJoinDay == 0 {
			continue
		}

		if lastRecDay != 0 && lastRecDay == lastCanRecDay {
			continue
		}

		var totalDay = uint32(0)
		if lastRecDay < lastCanRecDay {
			lastRecDay := lastRecDay
			if lastRecDay == 0 {
				lastRecDay = firstJoinDay
			}
			if lastRecDay < lastCanRecDay {
				parse1, _ := time.Parse("20060102", fmt.Sprintf("%d", lastCanRecDay))
				parse2, _ := time.Parse("20060102", fmt.Sprintf("%d", lastRecDay))
				totalDay = uint32(parse1.Sub(parse2).Hours() / 24)
			}
		}

		// 兜底 最多那么多天
		if totalDay > tConf.DurationDays {
			logger.LogWarn("%d data has err,totalDay:%d", playerId, totalDay)
			totalDay = tConf.DurationDays
		}

		if totalDay == 0 {
			continue
		}

		awards := jsondata.StdRewardVec{{Id: conf.MoneyItemId, Count: int64(treasuresData.Money) + int64(totalDay)*int64(treasuresData.Money*conf.Ratio/10000)}}
		mailmgr.SendMailToActor(playerId, &mailargs.SendMailSt{
			ConfId:  common.Mail_LuckyTreasuresSettle,
			Rewards: awards,
		})
		treasuresData.LastRecDay = lastCanRecDay
	}
}

func CmdForceOpenNextLuckyTreasures() {
	luckyTreasuresSettle()
	luckyTreasuresOnNewDay(true)
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.GetSysObj(sysdef.SiLuckyTreasures).(*LuckyTreasuresSys).s2cInfo()
	})
}

func init() {
	RegisterSysClass(sysdef.SiLuckyTreasures, func() iface.ISystem {
		return &LuckyTreasuresSys{}
	})
	net.RegisterSysProtoV2(8, 61, sysdef.SiLuckyTreasures, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuckyTreasuresSys).c2sInput
	})
	net.RegisterSysProtoV2(8, 62, sysdef.SiLuckyTreasures, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuckyTreasuresSys).c2sRecDailyAwards
	})
	event.RegActorEventL(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		obj := player.GetSysObj(sysdef.SiLuckyTreasures)
		if obj == nil || !obj.IsOpen() {
			return
		}
		obj.(*LuckyTreasuresSys).OnNewDay()
	})
	event.RegSysEventH(custom_id.SeNewDayArrive, func(args ...interface{}) {
		luckyTreasuresOnNewDay(false)
	})
	event.RegSysEventH(custom_id.SeServerInit, func(args ...interface{}) {
		luckyTreasuresOnNewDay(false)
	})
	event.RegSysEvent(custom_id.SeCmdGmBeforeMerge, func(args ...interface{}) {
		data := manager.GetLuckyTreasures().GetData()
		if data.IsSettle {
			return
		}
		sec := time_util.GetDaysZeroTime(0) + 86400 - 1
		logger.LogInfo("需要提前结算,结算时间修改为今天:%d", sec)
		data.EndAt = sec
		// 结算
		luckyTreasuresSettle()
	})
	event.RegSysEvent(custom_id.SeMerge, func(args ...interface{}) {
		logger.LogInfo("合服强制开启下一期,先结算后开启")
		luckyTreasuresSettle()
		luckyTreasuresOnNewDay(true)
	})
	gmevent.Register("resetLuckyTreasuresGlobal", func(player iface.IPlayer, args ...string) bool {
		gshare.GetStaticVar().LuckyTreasuresData = &pb3.LuckyTreasuresGlobalData{}
		luckyTreasuresOnNewDay(false)
		return true
	}, 1)
}
