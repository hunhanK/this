/**
 * @Author: lzp
 * @Date: 2024/7/16
 * @Desc:
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
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

const (
	PrivilegeGiftState0 = 0
	PrivilegeGiftState1 = 1 // 已购买可领取
	PrivilegeGiftState2 = 2 // 已领取
)

const (
	PrivilegeRewardsType0 = 0 // 特权
	PrivilegeRewardsType1 = 1 // 邮件
	PrivilegeRewardsType2 = 2 // 普通
)

const (
	PrivilegeGiftType1 = 1 // 直购
	PrivilegeGiftType2 = 2 // 一键
)

type PrivilegeMustBuySys struct {
	Base
}

func (sys *PrivilegeMustBuySys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *PrivilegeMustBuySys) OnLogin() {
	sys.s2cInfo()
}

func (sys *PrivilegeMustBuySys) OnOpen() {
	sys.s2cInfo()
}

func (sys *PrivilegeMustBuySys) OnNewDay() {
	data := sys.GetData()
	for k, v := range data {
		conf := jsondata.GetPrivilegeGiftConf(k)
		if conf == nil {
			continue
		}
		// 奖励是邮件类型,每日发邮件(购买后)
		if v.State > 0 && conf.RewardsType == PrivilegeRewardsType1 {
			sys.sendMail(conf)
		}
	}
}

func (sys *PrivilegeMustBuySys) GetData() map[uint32]*pb3.PrivilegeGift {
	if sys.GetBinaryData().PrivilegeGifts == nil {
		sys.GetBinaryData().PrivilegeGifts = make(map[uint32]*pb3.PrivilegeGift)
	}
	return sys.GetBinaryData().PrivilegeGifts
}

func (sys *PrivilegeMustBuySys) s2cInfo() {
	sys.SendProto3(2, 190, &pb3.S2C_2_190{
		Data: sys.GetData(),
	})
}

func (sys *PrivilegeMustBuySys) c2sReceive(msg *base.Message) error {
	var req pb3.C2S_2_191
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPrivilegeGiftConf(req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("conf id=%d not found", req.Id)
	}

	if len(conf.BuyRewards) == 0 {
		return neterror.ParamsInvalidError("id=%d buyRewards nil", req.Id)
	}

	data := sys.GetData()
	pData, ok := data[req.Id]
	if !ok || pData.State != PrivilegeGiftState1 {
		return neterror.ParamsInvalidError("id=%d receive limit", req.Id)
	}

	// 已经领取
	if pData != nil && pData.State == PrivilegeGiftState2 {
		return neterror.ParamsInvalidError("id=%d is received", req.Id)
	}

	if pData == nil {
		data[req.Id] = &pb3.PrivilegeGift{
			Id:        conf.Id,
			ChargeId:  conf.ChargeId,
			State:     PrivilegeGiftState2,
			Timestamp: time_util.NowSec(),
		}
	} else {
		// 设置状态
		pData.State = PrivilegeGiftState2
		pData.Timestamp = time_util.NowSec()
	}

	// 发送奖励
	engine.GiveRewards(sys.owner, conf.BuyRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPrivilegeGiftBuyAwards})

	// 广播播报
	var tipId uint32
	switch conf.RewardsType {
	case PrivilegeRewardsType0:
		tipId = tipmsgid.XiuxianPrivilege
	case PrivilegeRewardsType1:
		tipId = tipmsgid.ShoulingPrivilege
	case PrivilegeRewardsType2:
		tipId = tipmsgid.GexingPrivilege
	}
	if tipId > 0 {
		engine.BroadcastTipMsgById(tipId, sys.owner.GetId(), sys.owner.GetName(), engine.StdRewardToBroadcast(sys.owner, conf.BuyRewards))
	}

	sys.SendProto3(2, 192, &pb3.S2C_2_192{
		Data: data,
	})
	return nil
}

func (sys *PrivilegeMustBuySys) sendMail(conf *jsondata.PrivilegeMustBuyConf) {
	sys.owner.SendMail(&mailargs.SendMailSt{
		ConfId:  common.Mail_PrivilegeGiftBuyAwards,
		Rewards: conf.Rewards,
	})
}

func (sys *PrivilegeMustBuySys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	data := sys.GetData()

	chargeId := chargeConf.ChargeId
	conf := jsondata.GetPrivilegeGiftConfByChargeId(chargeId)
	if conf == nil {
		return false
	}

	// 一键购买检查
	if conf.Type == PrivilegeGiftType2 {
		for _, v := range data {
			if v.State != PrivilegeGiftState0 {
				return false
			}
		}
		return true
	}

	// 单个购买
	for _, v := range data {
		if v.ChargeId != chargeConf.ChargeId {
			continue
		}
		if v.State != PrivilegeGiftState0 {
			return false
		}
	}
	return true
}

func (sys *PrivilegeMustBuySys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	chargeId := chargeConf.ChargeId
	conf := jsondata.GetPrivilegeGiftConfByChargeId(chargeId)
	if conf == nil {
		return false
	}

	data := sys.GetData()
	if pData, ok := data[conf.Id]; ok {
		if pData.State != PrivilegeGiftState0 {
			return false
		}
	}

	if conf.Type == PrivilegeGiftType2 {
		confL := jsondata.GetPrivilegeGifts()
		if confL == nil {
			return false
		}
		for _, tmpConf := range confL {
			data[tmpConf.Id] = &pb3.PrivilegeGift{
				Id:       tmpConf.Id,
				ChargeId: tmpConf.ChargeId,
				State:    PrivilegeGiftState1,
			}
		}
	} else {
		data[conf.Id] = &pb3.PrivilegeGift{
			Id:       conf.Id,
			ChargeId: conf.ChargeId,
			State:    PrivilegeGiftState1,
		}
	}

	sys.SendProto3(2, 192, &pb3.S2C_2_192{
		Data: data,
	})
	return true
}

func privilegeGiftChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	if sys, ok := player.GetSysObj(sysdef.SiPrivilegeMustBuy).(*PrivilegeMustBuySys); ok && sys.IsOpen() {
		return sys.chargeCheck(conf)
	}
	return false
}

func privilegeGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if sys, ok := player.GetSysObj(sysdef.SiPrivilegeMustBuy).(*PrivilegeMustBuySys); ok && sys.IsOpen() {
		return sys.chargeBack(conf)
	}
	return false
}

func init() {
	RegisterSysClass(sysdef.SiPrivilegeMustBuy, func() iface.ISystem {
		return &PrivilegeMustBuySys{}
	})

	// 特权注册
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		sys, ok := player.GetSysObj(sysdef.SiPrivilegeMustBuy).(*PrivilegeMustBuySys)
		if !ok || !sys.IsOpen() {
			return
		}

		if len(conf.PrivilegeBuy) == 0 {
			return
		}

		data := sys.GetData()
		for k, v := range conf.PrivilegeBuy {
			id := k + 1
			pData, ok := data[uint32(id)]
			if !ok {
				continue
			}
			if pData.State == PrivilegeGiftState2 && v > 0 {
				total += int64(v)
			}
		}

		return
	})

	engine.RegChargeEvent(chargedef.PrivilegeGift, privilegeGiftChargeCheck, privilegeGiftChargeBack)
	net.RegisterSysProtoV2(2, 191, sysdef.SiPrivilegeMustBuy, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*PrivilegeMustBuySys).c2sReceive
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiPrivilegeMustBuy).(*PrivilegeMustBuySys); ok && sys.IsOpen() {
			sys.OnNewDay()
		}
	})
}
