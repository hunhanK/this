/**
 * @Author: lzp
 * @Date: 2025/1/6
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type CircleSecretSys struct {
	Base
}

func (s *CircleSecretSys) OnLogin() {
	s.s2cInfo()
}

func (s *CircleSecretSys) OnOpen() {
	s.s2cInfo()
}

func (s *CircleSecretSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CircleSecretSys) data() *pb3.CircleSecretData {
	binary := s.GetBinaryData()
	if binary.CircleSecretData == nil {
		binary.CircleSecretData = &pb3.CircleSecretData{}
	}
	return binary.CircleSecretData
}

func (s *CircleSecretSys) s2cInfo() {
	s.SendProto3(2, 198, &pb3.S2C_2_198{Data: s.data()})
}

func (s *CircleSecretSys) onAddCharge(cashCent uint32) {
	data := s.data()
	data.CashCent += cashCent
	s.s2cInfo()
}

func (s *CircleSecretSys) c2sFetch(msg *base.Message) error {
	var req pb3.C2S_2_199
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.data()
	if !data.IsBuy {
		return neterror.ParamsInvalidError("not buy gift")
	}
	if req.Day > 0 && utils.SliceContainsUint32(data.Day, req.Day) {
		return neterror.ParamsInvalidError("day:%d rewards has fetched", req.Day)
	}

	day := gshare.GetOpenServerDay()
	if req.Day > 0 && req.Day > day {
		return neterror.ParamsInvalidError("day:%d not satisfy", req.Day)
	}

	var rewards jsondata.StdRewardVec
	var days []uint32
	if req.Day > 0 {
		dConf := jsondata.GetCircleSecretDayRewards(req.Day)
		if dConf != nil {
			rewards = dConf.Rewards
			days = append(days, req.Day)
		}
	} else {
		for i := uint32(1); i <= day; i++ {
			if utils.SliceContainsUint32(data.Day, i) {
				continue
			}
			dConf := jsondata.GetCircleSecretDayRewards(i)
			if dConf != nil {
				rewards = append(rewards, dConf.Rewards...)
				days = append(days, i)
			}
		}
	}

	owner := s.GetOwner()
	data.Day = append(data.Day, days...)
	if len(rewards) > 0 {
		engine.GiveRewards(owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogCircleSecretAwards})
	}

	for _, day := range days {
		dConf := jsondata.GetCircleSecretDayRewards(day)
		engine.BroadcastTipMsgById(tipmsgid.CircleSecretTip, owner.GetId(), owner.GetName(), day, engine.StdRewardToBroadcast(s.owner, dConf.Rewards))
	}
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogCircleSecretAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Day),
	})

	s.SendProto3(2, 199, &pb3.S2C_2_199{Day: req.Day})
	s.s2cInfo()
	return nil
}

func circleSecretGiftChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiCircleSecret).(*CircleSecretSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	conf := jsondata.CircleSecretConfMgr
	if conf == nil {
		return false
	}
	if conf.ChargeId != chargeConf.ChargeId {
		return false
	}

	data := sys.data()
	if data.IsBuy {
		return false
	}
	return true
}

func circleSecretGiftChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiCircleSecret).(*CircleSecretSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	data := sys.data()
	data.IsBuy = true
	sys.s2cInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiCircleSecret, func() iface.ISystem {
		return &CircleSecretSys{}
	})

	net.RegisterSysProtoV2(2, 199, sysdef.SiCircleSecret, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CircleSecretSys).c2sFetch
	})

	engine.RegChargeEvent(chargedef.CircleSecretGift, circleSecretGiftChargeCheck, circleSecretGiftChargeBack)

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiCircleSecret).(*CircleSecretSys)
		if !ok || !sys.IsOpen() {
			return
		}

		if len(args) < 1 {
			return
		}

		chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
		if !ok {
			return
		}

		chargeConf := jsondata.GetChargeConf(chargeEvent.ChargeId)
		if chargeConf == nil {
			return
		}
		if chargeConf.ChargeType == chargedef.Charge ||
			chargeConf.ChargeType == chargedef.PrivilegeWeekCard ||
			chargeConf.ChargeType == chargedef.PrivilegeMonthCard ||
			chargeConf.ChargeType == chargedef.PrivilegeWeekMonthQuickBuy {
			sys.onAddCharge(chargeEvent.CashCent)
		}
	})
}
