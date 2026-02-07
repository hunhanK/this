package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/ranktype"
	"time"
)

// 冲榜活动跨小时发送邮件
func handleRushRankHourArrive(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	openServerDay := gshare.GetOpenServerDay()
	for _, conf := range jsondata.GetAllRushRankConf() {
		if conf.EndDay != openServerDay {
			continue
		}
		switch hour {
		case int(conf.BeforeCheckoutNotifyHour):
			argStr, _ := mailargs.MarshalMailArg(&mailargs.CommonMailArgs{Str1: conf.Name})
			mailmgr.AddSrvMailStr(common.Mail_RushRankNotify, argStr, nil)
		case int(conf.CheckOutHour):
			doSendCheckOutAwards(conf)
		}
	}
}

// 检查奖励补发
func handleAfterRushRankInit(_ ...interface{}) {
	openServerDay := gshare.GetOpenServerDay()
	hour := uint32(time.Now().Hour())
	for _, conf := range jsondata.GetAllRushRankConf() {
		if conf.EndDay == openServerDay {
			// 已经过去
			if hour >= conf.CheckOutHour {
				doSendCheckOutAwards(conf)
			}
			continue
		}

		if conf.OpenDay > openServerDay || conf.EndDay > openServerDay {
			continue
		}

		doSendCheckOutAwards(conf)
	}
}

func doSendCheckOutAwards(conf *jsondata.OneRushRankConf) {
	bit := gshare.GetStaticVar().RushRankTypeRecAwardsBit
	// 已经发过奖励
	if utils.IsSetBit(bit, conf.Type) {
		return
	}
	data, ok := manager.RushRankMgrIns[ranktype.PowerRushRankType(conf.Type)]
	if !ok {
		logger.LogError("not found %d rush rank", conf.Type)
		return
	}
	data.Settlement(func(rank uint32, rankInfo *pb3.RankInfo, awards jsondata.StdRewardVec) {
		mailmgr.SendMailToActor(rankInfo.PlayerId, &mailargs.SendMailSt{
			ConfId:  common.Mail_RushRankGiveAwards,
			Rewards: jsondata.StdRewardFilterJob(rankInfo.Job, awards),
			Content: &mailargs.CommonMailArgs{
				Str1:   conf.Name,
				Digit1: int64(rank),
			},
		})
	})
	gshare.GetStaticVar().RushRankTypeRecAwardsBit = utils.SetBit(bit, conf.Type)
}
