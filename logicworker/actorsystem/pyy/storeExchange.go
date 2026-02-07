/**
 * @Author: lzp
 * @Date: 2024/6/27
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type StoreExChange struct {
	PlayerYYBase
}

func (s *StoreExChange) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *StoreExChange) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *StoreExChange) OnOpen() {
	s.S2CInfo()
}

func (s *StoreExChange) S2CInfo() {
	s.SendProto3(127, 70, &pb3.S2C_127_70{
		ActId: s.GetId(),
		Data:  s.GetData(),
	})
}

func (s *StoreExChange) GetData() *pb3.PYY_StoreExchange {
	if s.GetYYData().StoreExchange == nil {
		s.GetYYData().StoreExchange = make(map[uint32]*pb3.PYY_StoreExchange)
	}

	data, ok := s.GetYYData().StoreExchange[s.GetId()]
	if !ok {
		data = &pb3.PYY_StoreExchange{}
		s.GetYYData().StoreExchange[s.GetId()] = data
	}
	if data.ExChangeData == nil {
		data.ExChangeData = make(map[uint32]uint32)
	}
	return data
}

func (s *StoreExChange) ResetData() {
	if s.GetYYData().StoreExchange == nil {
		return
	}
	delete(s.GetYYData().StoreExchange, s.Id)
}

func (s *StoreExChange) c2sExchange(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_127_71
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	count := utils.MaxUInt32(req.Count, 1)

	conf := jsondata.GetPYYStoreExchangeConf(s.ConfName, s.GetConfIdx(), req.Idx)
	if conf == nil {
		return neterror.ConfNotFoundError("get conf failed actId %d", s.GetId())
	}

	data := s.GetData()
	if data.ExChangeData[req.Idx]+count > conf.ExchangeLimit {
		return neterror.ConfNotFoundError("exchange limit actId:%d, goodId:%d", s.GetId(), req.Idx)
	}

	if !s.GetPlayer().ConsumeRate(conf.Consumes, int64(count), false, common.ConsumeParams{LogId: pb3.LogId_LogPYYStoreExchangeConsume}) {
		return neterror.ParamsInvalidError("consumes not enough actId:%d, goodId:%d", s.GetId(), req.Idx)
	}

	rewards := jsondata.StdRewardMulti(conf.Rewards, int64(count))
	data.ExChangeData[req.Idx] += count
	if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPYYStoreExchangeAward,
	}) {
		return neterror.InternalError("rewards failed actId:%d, goodId:%d", s.GetId(), req.Idx)
	}

	s.GetPlayer().SendShowRewardsPopByYY(rewards, s.Id)

	s.GetPlayer().SendProto3(127, 71, &pb3.S2C_127_71{
		ActId: s.GetId(),
		Idx:   req.Idx,
		Num:   data.ExChangeData[req.Idx],
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYStoreExchange, func() iface.IPlayerYY {
		return &StoreExChange{}
	})

	net.RegisterYYSysProtoV2(127, 71, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*StoreExChange).c2sExchange
	})
}
