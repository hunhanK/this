/**
 * @Author: lzp
 * @Date: 2025/12/4
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
)

type LifeLongLuxuryGiftSys struct {
	Base
}

func (s *LifeLongLuxuryGiftSys) OnLogin() {
	s.s2cInfo()
}

func (s *LifeLongLuxuryGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LifeLongLuxuryGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *LifeLongLuxuryGiftSys) s2cInfo() {
	s.SendProto3(2, 245, &pb3.S2C_2_245{Gifts: s.GetBinaryData().LifeLongLuxuryGifts})
}

func lifeLongLuxuryGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	s, ok := player.GetSysObj(sysdef.SiLifeLongLuxuryGift).(*LifeLongLuxuryGiftSys)
	if !ok || !s.IsOpen() {
		return false
	}

	gConf := jsondata.GetLifeLongLuxuryGiftConfByChargeId(conf.ChargeId)
	if gConf == nil {
		return false
	}
	if gConf.OpenSrvDay > gshare.GetOpenServerDay() {
		return false
	}

	gifts := s.GetBinaryData().LifeLongLuxuryGifts
	if utils.SliceContainsUint32(gifts, gConf.GiftId) {
		return false
	}
	return true
}

func lifeLongLuxuryGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	s, ok := player.GetSysObj(sysdef.SiLifeLongLuxuryGift).(*LifeLongLuxuryGiftSys)
	if !ok || !s.IsOpen() {
		return false
	}

	gConf := jsondata.GetLifeLongLuxuryGiftConfByChargeId(conf.ChargeId)
	if gConf == nil {
		return false
	}

	s.GetBinaryData().LifeLongLuxuryGifts = append(s.GetBinaryData().LifeLongLuxuryGifts, gConf.GiftId)
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLifeLongLuxuryGiftReward})
	}

	owner := s.GetOwner()
	owner.SendShowRewardsPop(gConf.Rewards)
	engine.BroadcastTipMsgById(tipmsgid.LifeLongLuxuryRewards, owner.GetId(), owner.GetName(), gConf.GiftName, engine.StdRewardToBroadcast(owner, gConf.Rewards))

	s.s2cInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiLifeLongLuxuryGift, func() iface.ISystem {
		return &LifeLongLuxuryGiftSys{}
	})

	engine.RegChargeEvent(chargedef.LifeLongLuxuryGift, lifeLongLuxuryGiftCheck, lifeLongLuxuryGiftChargeBack)
}
