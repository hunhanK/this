/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
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

type PinkMonthCardSys struct {
	PlayerYYBase
}

func (s *PinkMonthCardSys) s2cInfo() {
	s.SendProto3(9, 40, &pb3.S2C_9_40{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *PinkMonthCardSys) getData() *pb3.PYYPinkMonthCardData {
	state := s.GetYYData()
	if nil == state.PinkMonthCardData {
		state.PinkMonthCardData = make(map[uint32]*pb3.PYYPinkMonthCardData)
	}
	if state.PinkMonthCardData[s.Id] == nil {
		state.PinkMonthCardData[s.Id] = &pb3.PYYPinkMonthCardData{}
	}
	return state.PinkMonthCardData[s.Id]
}

func (s *PinkMonthCardSys) ResetData() {
	state := s.GetYYData()
	if nil == state.PinkMonthCardData {
		return
	}
	delete(state.PinkMonthCardData, s.Id)
}

func (s *PinkMonthCardSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PinkMonthCardSys) Login() {
	s.s2cInfo()
}

func (s *PinkMonthCardSys) OnOpen() {
	s.s2cInfo()
}

func (s *PinkMonthCardSys) OnEnd() {
	data := s.getData()
	if data.ChargeAt <= 0 {
		return
	}
	dailyAwardRecFlag := data.DailyAwardRecFlag
	config := jsondata.GetPyyPinkMonthCardConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return
	}
	openDay := s.GetOpenDay()
	var totalAwards jsondata.StdRewardVec
	for _, dayConf := range config.Days {
		if dayConf.Day > openDay {
			continue
		}
		if utils.IsSetBit64(dailyAwardRecFlag, dayConf.Day) {
			continue
		}
		dailyAwardRecFlag = utils.SetBit64(dailyAwardRecFlag, dayConf.Day)
		totalAwards = append(totalAwards, dayConf.Rewards...)
	}
	if dailyAwardRecFlag == data.DailyAwardRecFlag {
		return
	}
	data.DailyAwardRecFlag = dailyAwardRecFlag
	if len(totalAwards) > 0 {
		player := s.GetPlayer()
		totalAwards = jsondata.MergeStdReward(totalAwards)
		mailmgr.SendMailToActor(player.GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(config.MailId),
			Rewards: totalAwards,
		})
	}
}

func (s *PinkMonthCardSys) c2sQuickRecAwards(msg *base.Message) error {
	var req pb3.C2S_9_42
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	if data.ChargeAt <= 0 {
		return neterror.ParamsInvalidError("not charge open height awards")
	}
	dailyAwardRecFlag := data.DailyAwardRecFlag
	config := jsondata.GetPyyPinkMonthCardConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ParamsInvalidError("%d not pink month card config", s.Id)
	}
	openDay := s.GetOpenDay()
	var totalAwards jsondata.StdRewardVec
	for _, dayConf := range config.Days {
		if dayConf.Day > openDay {
			continue
		}
		if utils.IsSetBit64(dailyAwardRecFlag, dayConf.Day) {
			continue
		}
		dailyAwardRecFlag = utils.SetBit64(dailyAwardRecFlag, dayConf.Day)
		totalAwards = append(totalAwards, dayConf.Rewards...)
	}
	if dailyAwardRecFlag == data.DailyAwardRecFlag {
		return neterror.ParamsInvalidError("%d no pink month card quick rec awards", s.Id)
	}
	data.DailyAwardRecFlag = dailyAwardRecFlag
	player := s.GetPlayer()
	if len(totalAwards) > 0 {
		totalAwards = jsondata.MergeStdReward(totalAwards)
		engine.GiveRewards(player, totalAwards, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogPinkMonthCardDailyAwards,
			NoTips: true,
		})
		player.SendShowRewardsPopByPYY(totalAwards, s.Id)
	}
	s.SendProto3(9, 42, &pb3.S2C_9_42{
		ActiveId:          s.GetId(),
		DailyAwardRecFlag: dailyAwardRecFlag,
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogPinkMonthCardDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", dailyAwardRecFlag),
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYPinkMonthCard, func() iface.IPlayerYY {
		return &PinkMonthCardSys{}
	})
	net.RegisterYYSysProtoV2(9, 42, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PinkMonthCardSys).c2sQuickRecAwards
	})
	engine.RegChargeEvent(chargedef.PYYPinkMonthCard, pyyPinkMonthCardChargeCheck, pyyPinkMonthCardChargeBack)
}

func pyyPinkMonthCardChargeCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var result bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYPinkMonthCard, func(obj iface.IPlayerYY) {
		if result {
			return
		}
		sys, ok := obj.(*PinkMonthCardSys)
		if !ok {
			return
		}
		config := jsondata.GetPyyPinkMonthCardConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		if config.ChargeId != conf.ChargeId {
			return
		}
		pinkMonthCardData := sys.getData()
		if pinkMonthCardData.ChargeAt > 0 {
			return
		}
		result = true
	})
	return result

}

func pyyPinkMonthCardChargeBack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !pyyPinkMonthCardChargeCheck(actor, conf) {
		return false
	}
	var result bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYPinkMonthCard, func(obj iface.IPlayerYY) {
		if result {
			return
		}
		sys, ok := obj.(*PinkMonthCardSys)
		if !ok {
			return
		}
		config := jsondata.GetPyyPinkMonthCardConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		if config.ChargeId != conf.ChargeId {
			return
		}
		pinkMonthCardData := sys.getData()
		pinkMonthCardData.ChargeAt = time_util.NowSec()
		if len(config.Rewards) > 0 {
			engine.GiveRewards(actor, config.Rewards, common.EngineGiveRewardParam{
				LogId:  pb3.LogId_LogPinkMonthCardCharge,
				NoTips: true,
			})
			actor.SendShowRewardsPopByPYY(config.Rewards, sys.Id)
		}
		sys.SendProto3(9, 41, &pb3.S2C_9_41{
			ActiveId: sys.GetId(),
			ChargeAt: pinkMonthCardData.ChargeAt,
		})

		if config.BroadcastId > 0 {
			engine.BroadcastTipMsgById(config.BroadcastId, actor.GetId(), actor.GetName(), config.Name)
		}

		logworker.LogPlayerBehavior(actor, pb3.LogId_LogPinkMonthCardCharge, &pb3.LogPlayerCounter{
			NumArgs: uint64(sys.Id),
		})
		result = true
	})
	return result
}
