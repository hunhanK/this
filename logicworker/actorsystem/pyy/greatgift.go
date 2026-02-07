/**
 * @Author: lzp
 * @Date: 2024/1/30
 * @Desc: 必买专场
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type GreatGiftSys struct {
	*PlayerYYBase
}

func (s *GreatGiftSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *GreatGiftSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *GreatGiftSys) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 40, &pb3.S2C_127_40{
		ActivityId: s.Id,
		Refs:       data.Gifts,
	})
}

func (s *GreatGiftSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.GreatGift == nil {
		return
	}
	delete(state.GreatGift, s.Id)
}

func (s *GreatGiftSys) GetData() *pb3.PYY_GreatGift {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.GreatGift == nil {
		state.GreatGift = make(map[uint32]*pb3.PYY_GreatGift)
	}
	if state.GreatGift[s.Id] == nil {
		state.GreatGift[s.Id] = &pb3.PYY_GreatGift{}
	}
	data := state.GreatGift[s.Id]
	if data.Gifts == nil {
		data.Gifts = make(map[uint32]uint32)
	}
	return data
}

func (s *GreatGiftSys) c2sPurchase(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_127_41
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := jsondata.GetGreatGift(req.Ref)
	if conf == nil {
		return neterror.ConfNotFoundError("great gift=%d config not found", req.Ref)
	}

	if conf.OpenSrvDay > 0 && gshare.GetOpenServerDay() < conf.OpenSrvDay {
		return neterror.ParamsInvalidError("great gift=%d srvOpenDay limit", req.Ref)
	}

	data := s.GetData()
	if data.Gifts[req.Ref] >= conf.CountLimit {
		return neterror.ConfNotFoundError("great gift=%d count limit", req.Ref)
	}

	s.SendProto3(127, 41, &pb3.S2C_127_41{Ref: req.Ref, ActivityId: s.Id})

	return nil
}

func getGreatGiftSys(player iface.IPlayer) (*GreatGiftSys, bool) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYGreatGift)
	if len(yyList) == 0 {
		return nil, false
	}

	for i := range yyList {
		sys, ok := yyList[i].(*GreatGiftSys)
		if !ok || !sys.IsOpen() {
			return nil, false
		}
		return sys, true
	}

	return nil, false
}

func greatGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sys, ok := getGreatGiftSys(player)
	if !ok {
		return false
	}
	gConf := jsondata.GetGreatGiftByChargeId(conf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	if data.Gifts[gConf.Ref] >= gConf.CountLimit {
		return false
	}

	return true
}

func greatGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := getGreatGiftSys(player)
	if !ok {
		return false
	}

	gConf := jsondata.GetGreatGiftByChargeId(conf.ChargeId)
	if gConf == nil {
		return false
	}

	data := sys.GetData()
	data.Gifts[gConf.Ref] += 1

	if !utils.SliceContainsUint32(player.GetBinaryData().GreatGiftBuyStatus, gConf.Ref) {
		player.GetBinaryData().GreatGiftBuyStatus = append(player.GetBinaryData().GreatGiftBuyStatus, gConf.Ref)
		onSendGreatGiftInfo(player)
	}

	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(sys.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGreatGiftBuyReward})
	}

	sys.S2CInfo()
	sys.SendProto3(127, 42, &pb3.S2C_127_42{
		Rewards: jsondata.StdRewardVecToPb3RewardVec(gConf.Rewards),
	})
	player.SendTipMsg(tipmsgid.TpGreatGiftCharge, player.GetId(), player.GetName())
	return true
}

func onSendGreatGiftInfo(player iface.IPlayer, args ...interface{}) {
	player.SendProto3(127, 43, &pb3.S2C_127_43{Gifts: player.GetBinaryData().GreatGiftBuyStatus})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYGreatGift, func() iface.IPlayerYY {
		return &GreatGiftSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	event.RegActorEvent(custom_id.AeAfterLogin, onSendGreatGiftInfo)
	event.RegActorEvent(custom_id.AeReconnect, onSendGreatGiftInfo)

	engine.RegChargeEvent(chargedef.GreatGift, greatGiftCheck, greatGiftChargeBack)

	net.RegisterYYSysProtoV2(127, 41, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*GreatGiftSys).c2sPurchase
	})
}
