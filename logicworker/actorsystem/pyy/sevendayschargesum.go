package pyy

import (
	"encoding/json"
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
	"jjyz/gameserver/logicworker/cmdyysettingmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type SevenDaysChargeSumSys struct {
	PlayerYYBase
}

func (s *SevenDaysChargeSumSys) GetData() *pb3.PYY_SevenDaysChargeSum {
	yyData := s.GetYYData()
	if nil == yyData.SevenDaysChargeSum {
		yyData.SevenDaysChargeSum = make(map[uint32]*pb3.PYY_SevenDaysChargeSum)
	}
	if nil == yyData.SevenDaysChargeSum[s.Id] {
		yyData.SevenDaysChargeSum[s.Id] = &pb3.PYY_SevenDaysChargeSum{}
	}
	return yyData.SevenDaysChargeSum[s.Id]
}

func (s *SevenDaysChargeSumSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.SevenDaysChargeSum {
		return
	}
	delete(yyData.SevenDaysChargeSum, s.Id)
}

func (s *SevenDaysChargeSumSys) S2CInfo() {
	s.SendProto3(137, 0, &pb3.S2C_137_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *SevenDaysChargeSumSys) Login() {
	s.S2CInfo()
}

func (s *SevenDaysChargeSumSys) OnReconnect() {
	s.S2CInfo()
}

func (s *SevenDaysChargeSumSys) OnOpen() {
	s.S2CInfo()
}

func (s *SevenDaysChargeSumSys) NewDay() {
	data := s.GetData()
	data.IsRevDayRewards = false
	s.S2CInfo()
}

func (s *SevenDaysChargeSumSys) MergeFix() {
	openZeroTime := time_util.GetZeroTime(s.OpenTime)
	openDayCent := s.GetDailyChargeMoney(openZeroTime)

	if openDayCent > 0 {
		data := s.GetData()
		data.ChargeSum = openDayCent
		s.S2CInfo()
	}
}

func (s *SevenDaysChargeSumSys) CmdYYFix() {
	openTime := cmdyysettingmgr.GetSettingOpenTime(s.Id)
	if openTime == 0 {
		return
	}

	openZeroTime := time_util.GetZeroTime(openTime)
	openDayCent := s.GetDailyChargeMoney(openZeroTime)

	if openDayCent > 0 {
		data := s.GetData()
		data.ChargeSum = openDayCent
		s.S2CInfo()
	}
}

func (s *SevenDaysChargeSumSys) OnEnd() {
	data := s.GetData()
	chargeConf := jsondata.GetYYSevenDaysChargeSumConf(s.ConfName, s.ConfIdx)
	if nil == chargeConf {
		s.LogError("no SevenDaysChargeSumConf conf(%d)", s.ConfIdx)
		return
	}
	var award []*jsondata.StdReward
	for _, v := range chargeConf.Template {
		if v.ChargeSum <= data.ChargeSum && !utils.SliceContainsUint32(data.Award, v.Id) {
			award = jsondata.MergeStdReward(award, v.Rewards)
			data.Award = append(data.Award, v.Id)
		}
	}
	if len(award) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  chargeConf.MailId,
			Rewards: award,
		})
	}
}

func c2sSevenDaysChargeSumAward(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*SevenDaysChargeSumSys)
		if !ok {
			return neterror.InternalError("sevendayschargesum sys is nil")
		}
		var req pb3.C2S_137_2
		if err := msg.UnPackPb3Msg(&req); err != nil {
			return err
		}
		id := req.GetId()
		chargeConf := jsondata.GetYYSevenDaysChargeSumConf(s.ConfName, s.ConfIdx)
		if nil == chargeConf {
			return neterror.ConfNotFoundError("no sevendayschargesum conf")
		}

		var totalAwards jsondata.StdRewardVec
		data := s.GetData()
		player := s.GetPlayer()
		if id > 0 {
			var templateConf *jsondata.SevenDaysChargeSumTemplateConf
			for _, conf := range chargeConf.Template {
				if conf.Id == id {
					templateConf = conf
					break
				}
			}
			if nil == templateConf {
				return neterror.ConfNotFoundError("no sevendaychargesum conf(%d)", id)
			}
			if templateConf.ChargeSum > data.ChargeSum {
				return neterror.ParamsInvalidError("charge sum not reach")
			}
			if utils.SliceContainsUint32(data.Award, id) {
				s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
				return nil
			}
			data.Award = append(data.Award, id)
			totalAwards = append(totalAwards, templateConf.Rewards...)
			if chargeConf.BroadcastId > 0 {
				engine.BroadcastTipMsgById(chargeConf.BroadcastId, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, templateConf.Rewards), s.Id, chargeConf.TipsJump)
			}
		} else {
			if !req.QuickRec {
				return neterror.ParamsInvalidError("not found id")
			}
			for _, templateConf := range chargeConf.Template {
				if templateConf.ChargeSum > data.ChargeSum {
					continue
				}
				if utils.SliceContainsUint32(data.Award, templateConf.Id) {
					continue
				}
				data.Award = append(data.Award, templateConf.Id)
				totalAwards = append(totalAwards, templateConf.Rewards...)
				if chargeConf.BroadcastId > 0 {
					engine.BroadcastTipMsgById(chargeConf.BroadcastId, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, templateConf.Rewards), s.Id, chargeConf.TipsJump)
				}
			}
		}
		if len(totalAwards) == 0 {
			s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}

		if !engine.GiveRewards(s.player, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSevenDaysChargeSumReward}) {
			return neterror.InternalError("has unknown err in sevendayssumcharge receive award")
		}
		s.SendProto3(137, 2, &pb3.S2C_137_2{
			ActiveId: s.GetId(),
			Id:       id,
			Award:    data.Award,
		})
		return nil
	}
}

func c2sSevenDaysFreeAward(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*SevenDaysChargeSumSys)
		if !ok {
			return neterror.InternalError("sevendayschargesum sys is nil")
		}
		chargeConf := jsondata.GetYYSevenDaysChargeSumConf(s.ConfName, s.ConfIdx)
		if nil == chargeConf {
			return neterror.ConfNotFoundError("no sevendayschargesum conf")
		}
		data := s.GetData()
		if data.IsRevDayRewards {
			s.player.SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		data.IsRevDayRewards = true
		if !engine.GiveRewards(s.player, chargeConf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSevenDaysChargeFreeReward}) {
			return neterror.InternalError("has unknown err in sevendayssumcharge receive day reward")
		}
		s.S2CInfo()
		return nil
	}
}

func (s *SevenDaysChargeSumSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}
	cashCent := chargeEvent.CashCent
	data := s.GetData()
	data.ChargeSum += cashCent
	s.S2CInfo()

	logArg, _ := json.Marshal(map[string]interface{}{
		"yyId":     s.Id,
		"charge":   data.ChargeSum,
		"cashCent": cashCent,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSevenDaysCharge, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArg),
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSevenDaysChargeSum, func() iface.IPlayerYY {
		return &SevenDaysChargeSumSys{}
	})

	net.RegisterYYSysProtoV2(137, 2, c2sSevenDaysChargeSumAward)
	net.RegisterYYSysProtoV2(137, 3, c2sSevenDaysFreeAward)
}
