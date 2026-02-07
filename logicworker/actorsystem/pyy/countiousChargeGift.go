package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type ContinuousChargeGiftSys struct {
	PlayerYYBase
}

func (s *ContinuousChargeGiftSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.CountiousChargeGift {
		return
	}
	delete(yyData.CountiousChargeGift, s.Id)
}
func (s *ContinuousChargeGiftSys) GetData() *pb3.PYY_ContinuousChargeGift {
	yyData := s.GetYYData()
	if nil == yyData.CountiousChargeGift {
		yyData.CountiousChargeGift = make(map[uint32]*pb3.PYY_ContinuousChargeGift)
	}
	if nil == yyData.CountiousChargeGift[s.Id] {
		yyData.CountiousChargeGift[s.Id] = &pb3.PYY_ContinuousChargeGift{}
	}
	return yyData.CountiousChargeGift[s.Id]
}

func (s *ContinuousChargeGiftSys) S2CInfo() {
	s.SendProto3(139, 0, &pb3.S2C_139_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *ContinuousChargeGiftSys) Login() {
	s.S2CInfo()
}

func (s *ContinuousChargeGiftSys) OnReconnect() {
	s.S2CInfo()
}

func (s *ContinuousChargeGiftSys) OnOpen() {
	s.GetData()
	s.GetYYData().CountiousChargeGift[s.Id] = &pb3.PYY_ContinuousChargeGift{}
	s.S2CInfo()
}

func (s *ContinuousChargeGiftSys) OnEnd() {
	data := s.GetData()
	chargeConf := jsondata.GetYYContinuousChargeGiftConf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogError("no ContinuousChargeGift conf(%d)", s.ConfIdx)
		return
	}
	var award []*jsondata.StdReward
	for _, v := range chargeConf.Template {
		if v.Day <= data.Days && !utils.SliceContainsUint32(data.Award, v.Day) {
			award = jsondata.MergeStdReward(award, v.Rewards)
			data.Award = append(data.Award, v.Day)
		}
	}
	if len(award) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYContinuousChargeGift,
			Rewards: award,
		})
	}
}

func c2sContinuousChargeGift(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*ContinuousChargeGiftSys)
		if !ok {
			return neterror.InternalError("continuousChargeGiftSys sys is nil")
		}
		var req pb3.C2S_139_1
		if err := msg.UnPackPb3Msg(&req); err != nil {
			return err
		}
		day := req.GetDay()
		chargeConf := jsondata.GetYYContinuousChargeGiftConf(s.ConfName, s.ConfIdx)
		if nil == chargeConf {
			return neterror.ConfNotFoundError("no continuousChargeGift conf")
		}
		var templateConf *jsondata.ContinuousChargeGiftTemplateConf
		for _, conf := range chargeConf.Template {
			if conf.Day == day {
				templateConf = conf
				break
			}
		}

		if nil == templateConf {
			return neterror.ConfNotFoundError("no continuousChargeGift conf(%d)", day)
		}
		data := s.GetData()
		if templateConf.Day > data.Days {
			return neterror.ParamsInvalidError("continuousChargeGift charge day not reach")
		}
		if utils.SliceContainsUint32(data.Award, day) {
			s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		data.Award = append(data.Award, day)
		if !engine.GiveRewards(s.player, templateConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogContinuousChargeGift}) {
			return neterror.InternalError("has unknown err in continuousChargeGift receive award")
		}
		s.SendProto3(139, 1, &pb3.S2C_139_1{
			ActiveId: s.GetId(),
			Day:      day,
		})
		return nil
	}
}

func (s *ContinuousChargeGiftSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}
	data := s.GetData()
	if time_util.GetDaysZeroTime(0) == time_util.GetZeroTime(data.ChargeTime) {
		return
	}

	// 校验一下充值金额 满足才加1天
	chargeConf := jsondata.GetYYContinuousChargeGiftConf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogInfo("no continuousChargeGift conf")
		return
	}

	if chargeConf.ReachAmount != 0 && s.GetDailyCharge() < chargeConf.ReachAmount {
		return
	}

	data.Days++
	data.ChargeTime = time_util.NowSec()
	s.SendProto3(139, 2, &pb3.S2C_139_2{Days: data.Days, ActiveId: s.GetId()})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogContiousChargeCharge, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.Days),
	})
	s.GetPlayer().TriggerQuestEventRange(custom_id.QttContinuousChargeGiftDays)
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYContinuousChargeGift, func() iface.IPlayerYY {
		return &ContinuousChargeGiftSys{}
	})

	net.RegisterYYSysProtoV2(139, 1, c2sContinuousChargeGift)
	engine.RegQuestTargetProgress(custom_id.QttContinuousChargeGiftDays, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		iPlayerYYS := pyymgr.GetPlayerAllYYObj(actor, yydefine.YYContinuousChargeGift)
		var days uint32
		for _, yy := range iPlayerYYS {
			if yy == nil || !yy.IsOpen() {
				continue
			}
			data := yy.(*ContinuousChargeGiftSys).GetData()
			if data.Days > days {
				days = data.Days
			}
		}
		return days
	})
}
