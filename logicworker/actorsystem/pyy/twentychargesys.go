/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 每日充值
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type TwentyChargeSys struct {
	*PlayerYYBase
}

func (s *TwentyChargeSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TwentyChargeMap {
		return
	}
	delete(state.TwentyChargeMap, s.Id)
}

func (s *TwentyChargeSys) GetData() *pb3.TwentyChargeState {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TwentyChargeMap {
		state.TwentyChargeMap = make(map[uint32]*pb3.TwentyChargeState)
	}
	if state.TwentyChargeMap[s.Id] == nil {
		state.TwentyChargeMap[s.Id] = &pb3.TwentyChargeState{}
	}
	return state.TwentyChargeMap[s.Id]
}

func (s *TwentyChargeSys) S2CInfo() {
	s.SendProto3(162, 0, &pb3.S2C_162_0{
		ActiveId: s.Id,
		State:    s.GetData(),
	})
}

func (s *TwentyChargeSys) OnOpen() {
	s.S2CInfo()
}

func (s *TwentyChargeSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *TwentyChargeSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *TwentyChargeSys) OnEnd() {
	conf, ok := jsondata.GetYYTwentyChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogWarn("%s not found twenty charge conf", s.GetPrefix())
		return
	}
	todayCharge := s.getTodayCharge()
	if todayCharge < conf.Amount {
		s.GetPlayer().LogWarn("%s charge amount not enough , today charge %d, conf amount %d ", s.GetPrefix(), todayCharge, conf.Amount)
		return
	}

	if s.GetData().RecAwards {
		return
	}

	s.GetData().RecAwards = true
	if len(conf.Awards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: conf.Awards,
		})
	}
}

func (s *TwentyChargeSys) getTodayCharge() uint32 {
	player := s.GetPlayer()
	if player == nil {
		return 0
	}
	binaryData := player.GetBinaryData()
	if binaryData == nil {
		return 0
	}
	chargeInfo := binaryData.ChargeInfo
	if chargeInfo == nil {
		return 0
	}
	return chargeInfo.DailyChargeDiamond
}

func (s *TwentyChargeSys) c2sAward(_ *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	if s.GetData().RecAwards {
		return nil
	}

	conf, ok := jsondata.GetYYTwentyChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found twenty charge conf", s.GetPrefix())
	}

	todayCharge := s.getTodayCharge()
	if todayCharge < conf.Amount {
		return neterror.ParamsInvalidError("%s charge amount not enough , today charge %d, conf amount %d", s.GetPrefix(), todayCharge, conf.Amount)
	}

	data := s.GetData()
	data.RecAwards = true
	if len(conf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTwentyChargeRecAwards})
	}
	s.SendProto3(162, 1, &pb3.S2C_162_1{
		ActiveId: s.Id,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTwentyCharge, func() iface.IPlayerYY {
		return &TwentyChargeSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	net.RegisterYYSysProtoV2(162, 1, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*TwentyChargeSys).c2sAward
	})
}
