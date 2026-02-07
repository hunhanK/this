/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
)

type DirectPurchaseRebateSys struct {
	PlayerYYBase
}

func (s *DirectPurchaseRebateSys) s2cInfo() {
	s.SendProto3(8, 100, &pb3.S2C_8_100{
		ActiveId: s.Id,
		Data:     s.getData(),
	})
}

func (s *DirectPurchaseRebateSys) getData() *pb3.PYYDirectPurchaseRebateData {
	state := s.GetYYData()
	if nil == state.DirectPurchaseRebateData {
		state.DirectPurchaseRebateData = make(map[uint32]*pb3.PYYDirectPurchaseRebateData)
	}
	if state.DirectPurchaseRebateData[s.Id] == nil {
		state.DirectPurchaseRebateData[s.Id] = &pb3.PYYDirectPurchaseRebateData{}
	}
	return state.DirectPurchaseRebateData[s.Id]
}

func (s *DirectPurchaseRebateSys) ResetData() {
	state := s.GetYYData()
	if nil == state.DirectPurchaseRebateData {
		return
	}
	delete(state.DirectPurchaseRebateData, s.Id)
}

func (s *DirectPurchaseRebateSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DirectPurchaseRebateSys) Login() {
	s.s2cInfo()
}

func (s *DirectPurchaseRebateSys) OnOpen() {
	s.s2cInfo()
}

func checkDirectPurchaseRebateHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYDirectPurchaseRebate, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*DirectPurchaseRebateSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		config := jsondata.GetPyyDirectPurchaseRebateConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		giftConf, ok := config.Gifts[conf.ChargeId]
		if !ok {
			return
		}
		if giftConf.Vip != 0 && giftConf.Vip > actor.GetVipLevel() {
			return
		}
		data := sys.getData()
		if pie.Uint32s(data.ChargeIds).Contains(conf.ChargeId) {
			return
		}
		ret = true
		return
	})
	return ret
}

func directPurchaseRebateChargeHandler(actor iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	var ret bool
	if !checkDirectPurchaseRebateHandler(actor, chargeConf) {
		return false
	}
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYDirectPurchaseRebate, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*DirectPurchaseRebateSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		config := jsondata.GetPyyDirectPurchaseRebateConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		giftConf, ok := config.Gifts[chargeConf.ChargeId]
		if !ok {
			return
		}
		if giftConf.Vip != 0 && giftConf.Vip > actor.GetVipLevel() {
			return
		}
		data := sys.getData()
		if pie.Uint32s(data.ChargeIds).Contains(chargeConf.ChargeId) {
			return
		}
		data.ChargeIds = append(data.ChargeIds, chargeConf.ChargeId)
		if len(giftConf.Rewards) > 0 {
			engine.GiveRewards(actor, giftConf.Rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogPYYDirectPurchaseRebateBuy,
			})
			actor.SendShowRewardsPop(giftConf.Rewards)
		}
		if config.TipMsgId > 0 {
			engine.BroadcastTipMsgById(config.TipMsgId, actor.GetId(), actor.GetName(), chargeConf.ChargeName, engine.StdRewardToBroadcast(actor, giftConf.Rewards))
		}
		actor.SendProto3(8, 101, &pb3.S2C_8_101{
			ActiveId: sys.Id,
			ChargeId: giftConf.ChargeId,
		})

		logworker.LogPlayerBehavior(actor, pb3.LogId_LogPYYDirectPurchaseRebateBuy, &pb3.LogPlayerCounter{
			NumArgs: uint64(sys.GetId()),
			StrArgs: fmt.Sprintf("%d", giftConf.ChargeId),
		})
		ret = true
		return
	})
	return ret
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYDirectPurchaseRebate, func() iface.IPlayerYY {
		return &DirectPurchaseRebateSys{}
	})
	engine.RegChargeEvent(chargedef.PYYDirectPurchaseRebate, checkDirectPurchaseRebateHandler, directPurchaseRebateChargeHandler)
}
