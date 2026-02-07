package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type BfOnlineSys struct {
	Base
}

func (sys *BfOnlineSys) OnInit() {
}

func (sys *BfOnlineSys) GetData() *pb3.BenefitData {
	binary := sys.GetBinaryData()
	if nil == binary.Benefit {
		binary.Benefit = &pb3.BenefitData{}
	}
	return binary.Benefit
}

func (sys *BfOnlineSys) S2CInfo() {
	sys.SendProto3(41, 13, &pb3.S2C_41_13{OnlineAward: sys.GetData().OnlineAward})
}

func (sys *BfOnlineSys) OnOpen() {
	sys.S2CInfo()
}

func (sys *BfOnlineSys) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *BfOnlineSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *BfOnlineSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_41_5
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}
	onlineTime := sys.owner.GetDayOnlineTime()
	data := sys.GetData()
	award := make(jsondata.StdRewardVec, 0)
	conf := jsondata.GetBenefitConf()
	if nil == conf {
		return neterror.ConfNotFoundError("benefit online conf is nil")
	}
	for _, v := range conf.Online {
		if onlineTime >= v.OnlineTime && !utils.SliceContainsUint32(data.OnlineAward, v.OnlineTime) {
			award = jsondata.MergeStdReward(award, v.Award)
			data.OnlineAward = append(data.OnlineAward, v.OnlineTime)
		}
	}
	if len(award) > 0 {
		engine.GiveRewards(sys.owner, award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitOnline})
	} else {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}
	sys.S2CInfo()
	return nil
}

func onBenefitsOnlineNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBenefitOnline).(*BfOnlineSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	if conf := jsondata.GetBenefitConf(); nil != conf {
		onlineTime := sys.GetData().DailyOnlineTime
		award := make(jsondata.StdRewardVec, 0)
		for _, v := range conf.Online {
			if onlineTime >= v.OnlineTime && !utils.SliceContainsUint32(data.OnlineAward, v.OnlineTime) {
				award = jsondata.MergeStdReward(award, v.Award)
				data.OnlineAward = append(data.OnlineAward, v.OnlineTime)
			}
		}
		if len(award) > 0 {
			player.SendMail(&mailargs.SendMailSt{
				ConfId:  common.Mail_BenefitOnlineAward,
				Rewards: award,
			})
		}
	}

	data.OnlineAward = nil
	data.DailyOnlineTime = 0
	sys.S2CInfo()
}

func init() {
	RegisterSysClass(sysdef.SiBenefitOnline, func() iface.ISystem {
		return &BfOnlineSys{}
	})
	event.RegActorEvent(custom_id.AeBeforeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiBenefitOnline).(*BfOnlineSys)
		if !ok || !sys.IsOpen() {
			return
		}
		data := sys.GetData()
		data.DailyOnlineTime = player.GetMainData().DayOnlineTime
	})
	event.RegActorEvent(custom_id.AeNewDay, onBenefitsOnlineNewDay)
	net.RegisterSysProto(41, 5, sysdef.SiBenefitOnline, (*BfOnlineSys).c2sAward)
}
