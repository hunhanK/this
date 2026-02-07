/**
 * @Author: lzp
 * @Date: 2024/1/23
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type FunctionGiftSys struct {
	Base
}

func (s *FunctionGiftSys) OnLogin() {
	s.S2CInfo()

}

func (s *FunctionGiftSys) OnReconnect() {
	s.S2CInfo()
}

func (s *FunctionGiftSys) OnOpen() {
	s.S2CInfo()
}

func (s *FunctionGiftSys) GetData() map[uint32]uint32 {
	data := s.GetBinaryData().FuncGifts
	if data == nil {
		data = make(map[uint32]uint32)
		s.GetBinaryData().FuncGifts = data
	}
	return data
}

func (s *FunctionGiftSys) AddFuncGift(ref uint32) bool {
	data := s.GetData()
	if _, ok := data[ref]; ok {
		return false
	}
	data[ref] = 0
	return true
}

func (s *FunctionGiftSys) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 30, &pb3.S2C_127_30{
		Refs: data,
	})
}

func (s *FunctionGiftSys) IsBuyGift(ref uint32) bool {
	data := s.GetData()
	return data[ref] > 0
}

func (s *FunctionGiftSys) c2sPurchase(msg *base.Message) error {
	var req pb3.C2S_127_32
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetFuncGift(req.Ref)
	if conf == nil {
		return neterror.ConfNotFoundError("functionGift ref=%d config not found", req.Ref)
	}

	if s.owner.GetLevel() < conf.LvLimit {
		return neterror.ParamsInvalidError("funcGift lv limit ref=%d can not purchase", req.Ref)
	}

	if s.owner.GetCircle() < conf.CircleLimit {
		return neterror.ParamsInvalidError("funcGift circle limit ref=%d can not purchase", req.Ref)
	}

	data := s.GetData()
	count, ok := data[req.Ref]
	if ok && count >= conf.CountLimit {
		return neterror.ParamsInvalidError("functionGift ref=%d buy countLimit", req.Ref)
	}

	if len(conf.Consumes) > 0 {
		if !s.owner.ConsumeByConf(conf.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogFunctionGiftBuyConsume}) {
			return neterror.ParamsInvalidError("functionGift ref=%d consume error", req.Ref)
		}
	}
	data[req.Ref] += 1
	s.owner.TriggerQuestEvent(custom_id.QttAchievementsFunctionGiftPurchase, req.Ref, 1)

	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFunctionGiftBuyReward})
	}

	engine.BroadcastTipMsgById(tipmsgid.FuctionGiftTip, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, conf.Rewards))

	s.SendProto3(127, 32, &pb3.S2C_127_32{Ref: req.Ref})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiFunctionGift, func() iface.ISystem {
		return &FunctionGiftSys{}
	})

	net.RegisterSysProtoV2(127, 32, sysdef.SiFunctionGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FunctionGiftSys).c2sPurchase
	})

	gmevent.Register("functiongift.reset", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiFunctionGift)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys, ok := obj.(*FunctionGiftSys)
		if !ok {
			return false
		}
		data := sys.GetData()
		for k := range data {
			data[k] = 0
		}
		sys.S2CInfo()
		return true
	}, 1)
}
