/**
 * @Author: LvYuMeng
 * @Date: 2025/12/05
 * @Desc: 赞助特权
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type SponsorPrivilege struct {
	Base
}

func (s *SponsorPrivilege) OnReconnect() {
	s.s2cInfo()
}

func (s *SponsorPrivilege) OnLogin() {
	s.s2cInfo()
}

func (s *SponsorPrivilege) OnOpen() {
	s.s2cInfo()
}

func (s *SponsorPrivilege) s2cInfo() {
	data := s.getData()
	s.SendProto3(37, 25, &pb3.S2C_37_25{
		SponsorId:        data.SponsorId,
		LastReceiveTimes: data.LastReceiveTimes,
	})
}

func (s *SponsorPrivilege) getData() *pb3.SponsorPrivilege {
	binary := s.GetBinaryData()
	if nil == binary.SponsorPrivilege {
		binary.SponsorPrivilege = &pb3.SponsorPrivilege{}
	}
	return binary.SponsorPrivilege
}

func (s *SponsorPrivilege) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetSponsorPrivilegeByChargeId(chargeConf.ChargeId)
	if conf == nil {
		return false
	}

	data := s.getData()
	nextId := data.SponsorId + 1
	if nextId != conf.Id {
		return false
	}
	return true
}

func (s *SponsorPrivilege) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	if !s.chargeCheck(chargeConf) {
		return false
	}

	conf := jsondata.GetSponsorPrivilegeByChargeId(chargeConf.ChargeId)
	if conf == nil {
		return false
	}

	data := s.getData()
	data.SponsorId = conf.Id

	engine.GiveRewards(s.owner, conf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSponsorPrivilege,
	})
	s.s2cInfo()

	if conf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(conf.BroadcastId, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, conf.Rewards))
	}

	s.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)

	return true
}

func (s *SponsorPrivilege) c2sDailyAwards(msg *base.Message) error {
	var req pb3.C2S_37_26
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getData()
	if data.SponsorId == 0 {
		return neterror.ParamsInvalidError("not open privilege")
	}

	if data.LastReceiveTimes > 0 {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetSponsorPrivilegeBySponsorId(data.SponsorId)
	if nil == conf {
		return neterror.ConfNotFoundError("conf %d is nil", data.SponsorId)
	}

	data.LastReceiveTimes = time_util.NowSec()

	engine.GiveRewards(s.owner, conf.DailyAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSponsorPrivilegeDailyAward,
	})

	s.SendProto3(37, 26, &pb3.S2C_37_26{LastReceiveTimes: data.LastReceiveTimes})

	return nil
}

func (s *SponsorPrivilege) onNewDay() {
	data := s.getData()
	data.LastReceiveTimes = 0
	s.s2cInfo()
}

func sponsorPrivilegeChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	if sys, ok := player.GetSysObj(sysdef.SiSponsorPrivilege).(*SponsorPrivilege); ok && sys.IsOpen() {
		return sys.chargeCheck(conf)
	}
	return false
}

func sponsorPrivilegeChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if sys, ok := player.GetSysObj(sysdef.SiSponsorPrivilege).(*SponsorPrivilege); ok && sys.IsOpen() {
		return sys.chargeBack(conf)
	}
	return false
}

func init() {
	RegisterSysClass(sysdef.SiSponsorPrivilege, func() iface.ISystem {
		return &SponsorPrivilege{}
	})

	net.RegisterSysProtoV2(37, 26, sysdef.SiSponsorPrivilege, func(sys iface.ISystem) func(msg *base.Message) error {
		return sys.(*SponsorPrivilege).c2sDailyAwards
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiSponsorPrivilege).(*SponsorPrivilege); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		s, ok := player.GetSysObj(sysdef.SiSponsorPrivilege).(*SponsorPrivilege)
		if !ok || !s.IsOpen() {
			return
		}

		if len(conf.SponsorPrivilege) == 0 {
			return
		}

		data := s.getData()
		for k, v := range conf.SponsorPrivilege {
			if v == 0 {
				continue
			}

			id := uint32(k + 1)

			if data.SponsorId < id {
				continue
			}

			total += int64(v)
		}

		return
	})

	engine.RegChargeEvent(chargedef.SponsorPrivilege, sponsorPrivilegeChargeCheck, sponsorPrivilegeChargeBack)

}
