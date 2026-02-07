/**
 * @Author: zjj
 * @Date: 2024/12/12
 * @Desc: 连充豪礼-带累积充值天数奖励
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
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

type ContinuousChargeGiftV2Sys struct {
	PlayerYYBase
}

func (s *ContinuousChargeGiftV2Sys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.CountiousChargeGiftV2 {
		return
	}
	delete(yyData.CountiousChargeGiftV2, s.Id)
}

func (s *ContinuousChargeGiftV2Sys) GetData() *pb3.PYY_ContinuousChargeGiftV2 {
	yyData := s.GetYYData()
	if nil == yyData.CountiousChargeGiftV2 {
		yyData.CountiousChargeGiftV2 = make(map[uint32]*pb3.PYY_ContinuousChargeGiftV2)
	}
	if nil == yyData.CountiousChargeGiftV2[s.Id] {
		yyData.CountiousChargeGiftV2[s.Id] = &pb3.PYY_ContinuousChargeGiftV2{}
	}
	return yyData.CountiousChargeGiftV2[s.Id]
}

func (s *ContinuousChargeGiftV2Sys) S2CInfo() {
	s.SendProto3(139, 10, &pb3.S2C_139_10{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *ContinuousChargeGiftV2Sys) openActCheckDailyCharge() {
	data := s.GetData()
	if time_util.GetDaysZeroTime(0) == time_util.GetZeroTime(data.ChargeTime) {
		return
	}

	// 校验一下充值金额 满足才加1天
	chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogInfo("no found conf")
		return
	}

	if chargeConf.ReachAmount != 0 && s.GetDailyCharge() < chargeConf.ReachAmount {
		return
	}

	data.Days++
	data.ChargeTime = time_util.NowSec()
}

func (s *ContinuousChargeGiftV2Sys) Login() {
	s.S2CInfo()
}

func (s *ContinuousChargeGiftV2Sys) OnReconnect() {
	s.S2CInfo()
}

func (s *ContinuousChargeGiftV2Sys) OnOpen() {
	s.GetData()
	s.GetYYData().CountiousChargeGiftV2[s.Id] = &pb3.PYY_ContinuousChargeGiftV2{}
	s.openActCheckDailyCharge()
	s.S2CInfo()
}

func (s *ContinuousChargeGiftV2Sys) OnEnd() {
	data := s.GetData()
	chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogError("%s no found conf", s.GetPrefix())
		return
	}

	// 总充值天数
	days := data.Days

	var award []*jsondata.StdReward

	// 每日奖励
	for _, v := range chargeConf.Template {
		if v.Day <= days && !utils.SliceContainsUint32(data.Award, v.Day) {
			award = jsondata.MergeStdReward(award, v.Rewards)
			data.Award = append(data.Award, v.Day)
		}
	}

	// 累充奖励
	for _, v := range chargeConf.Cumulative {
		if v.Day <= days && !utils.SliceContainsUint32(data.CumulativeDaysAward, v.Day) {
			award = jsondata.MergeStdReward(award, v.Rewards)
			data.CumulativeDaysAward = append(data.CumulativeDaysAward, v.Day)
		}
	}

	mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
		ConfId:  chargeConf.EndMailId,
		Rewards: award,
	})
}

func c2sContinuousChargeGiftV2Awards(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*ContinuousChargeGiftV2Sys)
		if !ok {
			return neterror.InternalError("ContinuousChargeGiftV2Sys sys is nil")
		}
		var req pb3.C2S_139_11
		if err := msg.UnPackPb3Msg(&req); err != nil {
			return err
		}

		day := req.GetDay()
		chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
		if nil == chargeConf {
			return neterror.ConfNotFoundError("no found conf")
		}

		var templateConf *jsondata.ContinuousChargeGiftV2TemplateConf
		for _, conf := range chargeConf.Template {
			if conf.Day == day {
				templateConf = conf
				break
			}
		}

		if nil == templateConf {
			return neterror.ConfNotFoundError("no found conf(%d)", day)
		}

		if chargeConf.ReachAmount != 0 && s.GetDailyCharge() < chargeConf.ReachAmount {
			return neterror.ConfNotFoundError("charge not reach amount(%d)", day)
		}

		data := s.GetData()
		if utils.SliceContainsUint32(data.Award, day) {
			s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}

		data.Award = append(data.Award, day)
		if !engine.GiveRewards(s.player, templateConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogContinuousChargeGiftV2Awards}) {
			return neterror.InternalError("has unknown err in continuousChargeGift receive award")
		}
		s.SendProto3(139, 11, &pb3.S2C_139_11{
			ActiveId: s.GetId(),
			Day:      day,
		})
		logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogContinuousChargeGiftV2Awards, &pb3.LogPlayerCounter{
			NumArgs: uint64(s.GetId()),
			StrArgs: fmt.Sprintf("%d", day),
		})
		return nil
	}
}

func c2sContinuousChargeGiftV2Cumulative(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*ContinuousChargeGiftV2Sys)
		if !ok {
			return neterror.InternalError("ContinuousChargeGiftV2Sys sys is nil")
		}
		var req pb3.C2S_139_13
		if err := msg.UnPackPb3Msg(&req); err != nil {
			return err
		}

		day := req.GetDay()
		chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
		if nil == chargeConf {
			return neterror.ConfNotFoundError("no continuousChargeGift conf")
		}
		var cumulativeConf *jsondata.ContinuousChargeGiftV2CumulativeConf
		for _, conf := range chargeConf.Cumulative {
			if conf.Day == day {
				cumulativeConf = conf
				break
			}
		}

		if nil == cumulativeConf {
			return neterror.ConfNotFoundError("no continuousChargeGift conf(%d)", day)
		}

		data := s.GetData()
		if cumulativeConf.Day > data.Days {
			return neterror.ParamsInvalidError("continuousChargeGift charge day not reach")
		}

		if utils.SliceContainsUint32(data.CumulativeDaysAward, day) {
			s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}

		data.CumulativeDaysAward = append(data.CumulativeDaysAward, day)
		if !engine.GiveRewards(s.player, cumulativeConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogContinuousChargeGiftV2Cumulative}) {
			return neterror.InternalError("has unknown err in continuousChargeGift receive award")
		}
		s.SendProto3(139, 13, &pb3.S2C_139_13{
			ActiveId: s.GetId(),
			Day:      day,
		})
		logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogContinuousChargeGiftV2Cumulative, &pb3.LogPlayerCounter{
			NumArgs: uint64(s.GetId()),
			StrArgs: fmt.Sprintf("%d", day),
		})
		return nil
	}
}

func (s *ContinuousChargeGiftV2Sys) PlayerCharge(*custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}
	data := s.GetData()
	if time_util.GetDaysZeroTime(0) == time_util.GetZeroTime(data.ChargeTime) {
		return
	}

	// 校验一下充值金额 满足才加1天
	chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogInfo("no found conf")
		return
	}

	if chargeConf.ReachAmount != 0 && s.GetDailyCharge() < chargeConf.ReachAmount {
		return
	}

	data.Days++
	data.ChargeTime = time_util.NowSec()
	s.SendProto3(139, 12, &pb3.S2C_139_12{Days: data.Days, ActiveId: s.GetId()})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogContinuousChargeGiftV2Charge, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.Days),
	})
}

func (s *ContinuousChargeGiftV2Sys) NewDay() {
	player := s.GetPlayer()
	chargeConf := jsondata.GetYYContinuousChargeGiftV2Conf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		player.LogInfo("%s new day not found conf", s.GetPrefix())
		return
	}

	data := s.GetData()
	days := data.Days
	if pie.Uint32s(data.Award).Contains(days) {
		return
	}

	var templateConf *jsondata.ContinuousChargeGiftV2TemplateConf
	for _, conf := range chargeConf.Template {
		if conf.Day == days {
			templateConf = conf
			break
		}
	}

	if nil == templateConf {
		player.LogInfo("%s %d day not found conf", s.GetPrefix(), days)
		return
	}

	data.Award = append(data.Award, days)
	mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
		ConfId:  chargeConf.DayEndMailId,
		Rewards: templateConf.Rewards,
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYContinuousChargeGiftV2, func() iface.IPlayerYY {
		return &ContinuousChargeGiftV2Sys{}
	})
	net.RegisterYYSysProtoV2(139, 11, c2sContinuousChargeGiftV2Awards)
	net.RegisterYYSysProtoV2(139, 13, c2sContinuousChargeGiftV2Cumulative)
}
