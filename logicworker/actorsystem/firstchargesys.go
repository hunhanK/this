/**
 * @Author: lzp
 * @Date: 2025/6/18
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
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

type HalfFirstChargeSys struct {
	Base
}

func (s *HalfFirstChargeSys) OnLogin() {
	s.s2cInfo()
}

func (s *HalfFirstChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HalfFirstChargeSys) OnOpen() {
	data := s.GetData()
	data.FuncTimestamp = time_util.NowSec()
	s.s2cInfo()
}

func (s *HalfFirstChargeSys) GetData() *pb3.HalfFirstCharge {
	binData := s.GetBinaryData()
	if binData.HalfFirstChargeData == nil {
		binData.HalfFirstChargeData = &pb3.HalfFirstCharge{}
	}
	return binData.HalfFirstChargeData
}

func (s *HalfFirstChargeSys) s2cInfo() {
	s.SendProto3(36, 5, &pb3.S2C_36_5{Data: s.GetData()})
}

func (s *HalfFirstChargeSys) checkExpired() bool {
	data := s.GetData()
	expiredSeconds := jsondata.GlobalUint("halfFirstChargeExpired")
	now := time_util.NowSec()

	if data.FuncTimestamp+expiredSeconds < now {
		return true
	}
	return false
}

// 检查是否全部购买
func (s *HalfFirstChargeSys) isPurchaseAll() bool {
	data := s.GetData()
	for _, gConf := range jsondata.FirstChargeConfMgr {
		if !utils.SliceContainsUint32(data.GiftIds, gConf.GiftId) &&
			!utils.SliceContainsUint32(data.GiftIds, gConf.HalfGiftId) {
			return false
		}
	}
	return true
}

func c2sReceiveFirstChargeAward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_36_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	sys, ok := player.GetSysObj(sysdef.SiHalfFirstCharge).(*HalfFirstChargeSys)
	if !ok || !sys.IsOpen() {
		return neterror.ParamsInvalidError("first charge sys not open")
	}

	binary := player.GetBinaryData()
	data := sys.GetData()

	// 是否购买礼包
	isBuy := func(conf *jsondata.FirstChargeConf) bool {
		return utils.SliceContainsUint32(data.GiftIds, conf.GiftId) ||
			utils.SliceContainsUint32(data.GiftIds, conf.HalfGiftId)
	}

	var rewards jsondata.StdRewardVec
	for id, gConf := range jsondata.FirstChargeConfMgr {
		if !isBuy(gConf) && !checkCanRev(player, id) {
			continue
		}
		fCharge := binary.FirstChargeInfo[id]
		revFlag := utils.High32(fCharge)
		timeStamp := utils.Low32(fCharge)
		for day, rConf := range gConf.DayAwards {
			idx := day - 1
			if utils.IsSetBit(revFlag, idx) {
				continue
			}
			// 检查是否到领取时间
			if sys.isPurchaseAll() {
				rewards = append(rewards, rConf.DayAwards...)
				revFlag = utils.SetBit(revFlag, idx)
			} else {
				revTime := time_util.GetZeroTime(timeStamp + (idx)*86400)
				nowSec := time_util.NowSec()
				if nowSec >= revTime {
					rewards = append(rewards, rConf.DayAwards...)
					revFlag = utils.SetBit(revFlag, idx)
				}
			}
		}
		binary.FirstChargeInfo[id] = utils.Make64(timeStamp, revFlag)
	}

	// 检查背包
	if !engine.CheckRewards(player, rewards) {
		player.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	// 发放奖励
	if !engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFirstCharge}) {
		return neterror.InternalError("give first charge award failed")
	}

	player.SendProto3(36, 8, &pb3.S2C_36_8{
		Data: binary.GetFirstChargeInfo(),
	})
	return nil
}

func s2cFirstChargeInit(actor iface.IPlayer) {
	binary := actor.GetBinaryData()
	if nil == binary.GetFirstChargeInfo() {
		binary.FirstChargeInfo = make(map[uint32]uint64)
	}
}

func s2cFirstChargeInfo(player iface.IPlayer) {
	binary := player.GetBinaryData()
	player.SendProto3(36, 1, &pb3.S2C_36_1{Award: binary.GetFirstChargeInfo()})
}

// 检查老号玩家是否可以领取
func checkCanRev(player iface.IPlayer, id uint32) bool {
	binary := player.GetBinaryData()
	fCharge := binary.FirstChargeInfo[id]
	timeStamp := utils.Low32(fCharge)
	return timeStamp > 0
}

func halfFirstChargeCheckHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiHalfFirstCharge).(*HalfFirstChargeSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	gConf := jsondata.GetFirstChargeConfByGiftId(conf.ChargeId)
	if gConf == nil {
		return false
	}

	// 旧号可以领取，也限制不可购买
	if checkCanRev(player, gConf.Grade) {
		return false
	}

	if gConf.HalfGiftId == conf.ChargeId && sys.checkExpired() {
		return false
	}

	data := sys.GetData()
	if utils.SliceContainsUint32(data.GiftIds, conf.ChargeId) {
		return false
	}

	return true
}

func halfFirstChargeBackHandler(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiHalfFirstCharge).(*HalfFirstChargeSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	gConf := jsondata.GetFirstChargeConfByGiftId(conf.ChargeId)
	if gConf == nil {
		return false
	}

	// 旧号可以领取，也限制不可购买
	if checkCanRev(player, gConf.Grade) {
		return false
	}

	// 购买半价礼包检查时间
	if gConf.HalfGiftId == conf.ChargeId && sys.checkExpired() {
		return false
	}

	// 已经购买过
	data := sys.GetData()
	if utils.SliceContainsUint32(data.GiftIds, conf.ChargeId) {
		return false
	}

	data.GiftIds = pie.Uint32s(data.GiftIds).Append(conf.ChargeId).Unique()
	sys.s2cInfo()

	// 半价礼包额外给vip经验
	if gConf.HalfGiftId == conf.ChargeId {
		player.AddVipExp(gConf.VipExp)
	}
	binary := player.GetBinaryData()
	nowSec := time_util.NowSec()
	binary.FirstChargeInfo[gConf.Grade] = utils.Make64(nowSec, 0)
	s2cFirstChargeInfo(player)

	return true
}

func init() {
	RegisterSysClass(sysdef.SiHalfFirstCharge, func() iface.ISystem {
		return &HalfFirstChargeSys{}
	})

	engine.RegChargeEvent(chargedef.HalfFirstChargeGift, halfFirstChargeCheckHandler, halfFirstChargeBackHandler)
	net.RegisterProto(36, 8, c2sReceiveFirstChargeAward)

	event.RegActorEvent(custom_id.AeLogin, func(player iface.IPlayer, args ...interface{}) {
		s2cFirstChargeInit(player)
	})
	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		s2cFirstChargeInfo(player)
	})
	event.RegActorEvent(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		s2cFirstChargeInfo(player)
	})
}
