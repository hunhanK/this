/**
 * @Author: lzp
 * @Date: 2024/4/29
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type TreasureCourtSys struct {
	PlayerYYBase
}

func (s *TreasureCourtSys) OnReconnect() {
	s.S2CInfo()
}

func (s *TreasureCourtSys) Login() {
	s.S2CInfo()
}

func (s *TreasureCourtSys) OnOpen() {
	s.S2CInfo()
}

func (s *TreasureCourtSys) ResetData() {
	state := s.GetYYData()
	if state.TreasureCourt == nil {
		return
	}
	delete(state.TreasureCourt, s.Id)
}

func (s *TreasureCourtSys) S2CInfo() {
	data := s.GetData()
	s.SendProto3(63, 6, &pb3.S2C_63_6{
		ActiveId:       s.Id,
		Exchange:       data.Exchange,
		ExchangeRemind: data.ExchangeRemind,
	})
}

func (s *TreasureCourtSys) GetData() *pb3.PYY_TreasureCourt {
	state := s.GetYYData()
	if state.TreasureCourt == nil {
		state.TreasureCourt = make(map[uint32]*pb3.PYY_TreasureCourt)
	}
	if state.TreasureCourt[s.Id] == nil {
		state.TreasureCourt[s.Id] = &pb3.PYY_TreasureCourt{}
	}
	sData := state.TreasureCourt[s.Id]
	if sData.ExchangeRemind == nil {
		sData.ExchangeRemind = make(map[uint32]bool)
	}
	if sData.Exchange == nil {
		sData.Exchange = make(map[uint32]uint32)
	}
	return sData
}

func (s *TreasureCourtSys) c2sExchange(msg *base.Message) error {
	var req pb3.C2S_63_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYTreasureCourtExConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("treasurecourt conf is nil")
	}

	eConf := conf[req.Id]
	data := s.GetData()

	if eConf.ExchangeTimes > 0 && (data.Exchange[req.GetId()]+req.Count) > eConf.ExchangeTimes {
		s.GetPlayer().SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	if !s.player.ConsumeRate(eConf.Consume, int64(req.Count), false, common.ConsumeParams{LogId: pb3.LogId_LogTreasurePavExchange}) {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Exchange[req.Id] += req.Count

	rewards := jsondata.StdRewardMultiRate(eConf.Rewards, float64(req.Count))
	if len(rewards) > 0 {
		engine.GiveRewards(s.player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTreasurePavExchange})
		if eConf.Broadcast > 0 {
			itemId := rewards[0].Id
			itemConf := jsondata.GetItemConfig(itemId)
			s.player.SendTipMsg(tipmsgid.TreasureCourtExchangeTip, s.player.GetId(), s.player.GetName(), itemConf.Name)
		}
	}

	s.player.SendProto3(63, 4, &pb3.S2C_63_4{
		ActiveId: s.Id,
		Id:       req.Id,
		Times:    data.Exchange[req.Id],
	})

	return nil
}

func (s *TreasureCourtSys) c2sExchangeRemind(msg *base.Message) error {
	var req pb3.C2S_63_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYTreasureCourtExConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return ErrorConfNotFound
	}

	data := s.GetData()
	if req.Need {
		data.ExchangeRemind[req.Id] = true
	} else {
		delete(data.ExchangeRemind, req.Id)
	}
	s.SendProto3(63, 5, &pb3.S2C_63_5{
		ActiveId: s.Id,
		Id:       req.Id,
		Need:     data.ExchangeRemind[req.Id],
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTreasureCourt, func() iface.IPlayerYY {
		return &TreasureCourtSys{}
	})

	net.RegisterYYSysProtoV2(63, 4, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TreasureCourtSys).c2sExchange
	})

	net.RegisterYYSysProtoV2(63, 5, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TreasureCourtSys).c2sExchangeRemind
	})
}
