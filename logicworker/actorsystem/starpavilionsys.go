/**
 * @Author: lzp
 * @Date: 2025/12/16
 * @Desc:
**/

package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type StarPavilionSys struct {
	Base
}

func (s *StarPavilionSys) OnLogin() {
	s.s2cInfo()
}

func (s *StarPavilionSys) OnReconnect() {
	s.s2cInfo()
}

func (s *StarPavilionSys) OnOpen() {
	conf := jsondata.StarPavilionConfMgr
	if conf == nil {
		return
	}

	data := s.GetData()
	data.Round = 1
	data.RefreshTimestamp = time_util.GetDaysZeroTime(conf.RefreshDay)
	s.s2cInfo()
}

func (s *StarPavilionSys) OnNewDay() {
	conf := jsondata.StarPavilionConfMgr
	if conf == nil {
		return
	}

	data := s.GetData()
	data.IsFetchDayRewards = false
	if time_util.NowSec() >= s.GetData().RefreshTimestamp {
		data.Round += 1
		data.Round = utils.MinUInt32(data.Round, conf.MaxRound)
		data.RefreshTimestamp = time_util.GetDaysZeroTime(conf.RefreshDay)
		data.GoodsBuyCount = make(map[uint32]uint32)
	}

	s.s2cInfo()
}

func (s *StarPavilionSys) GetData() *pb3.StarPavilionData {
	data := s.GetBinaryData().StarPavilionData
	if data == nil {
		data = &pb3.StarPavilionData{}
		s.GetBinaryData().StarPavilionData = data
	}
	if data.GoodsBuyCount == nil {
		data.GoodsBuyCount = make(map[uint32]uint32)
	}
	return data
}

func (s *StarPavilionSys) s2cInfo() {
	data := s.GetData()
	s.SendProto3(83, 20, &pb3.S2C_83_20{
		Data: data,
	})
}

func (s *StarPavilionSys) c2sExchangeGood(msg *base.Message) error {
	var req pb3.C2S_83_21
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.StarPavilionConfMgr
	if conf == nil {
		return neterror.ConfNotFoundError("StarPavilionConfMgr is nil")
	}

	goodConf := jsondata.GetPavilionGoodById(req.Id)
	if goodConf == nil {
		return neterror.ParamsInvalidError("good conf not found")
	}

	data := s.GetData()
	if data.GoodsBuyCount[req.Id]+req.Count > goodConf.BuyCount {
		return neterror.ParamsInvalidError("good buy count limit")
	}

	consumes := jsondata.ConsumeMulti(goodConf.Consume, req.Count)
	if !s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogStarPavilionExchangeConsume}) {
		return neterror.ParamsInvalidError("not enough consume")
	}

	data.GoodsBuyCount[req.Id] += req.Count

	rewards := jsondata.StdRewardVec{
		{Id: goodConf.ItemId, Count: int64(goodConf.ItemCount), Bind: goodConf.Bind},
	}
	rewards = jsondata.StdRewardMulti(rewards, int64(req.Count))

	player := s.GetOwner()
	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogStarPavilionExchangeReward})
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"ItemId": goodConf.ItemId,
		"count":  req.Count,
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogStarPavilionExchange, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
	player.SendShowRewardsPop(rewards)
	engine.BroadcastTipMsgById(conf.BroadcastId, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, rewards))

	s.SendProto3(83, 21, &pb3.S2C_83_21{Id: req.Id, Count: req.Count})
	return nil
}

func (s *StarPavilionSys) c2sFetchDayRewards(msg *base.Message) error {
	var req pb3.C2S_83_22
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.StarPavilionConfMgr
	if conf == nil {
		return neterror.ConfNotFoundError("StarPavilionConfMgr is nil")
	}

	data := s.GetData()
	if data.IsFetchDayRewards {
		return neterror.ParamsInvalidError("day rewards already fetched")
	}
	data.IsFetchDayRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogStarPavilionDayReward})
	}

	s.s2cInfo()
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiStarPavilion, func() iface.ISystem {
		return &StarPavilionSys{}
	})

	net.RegisterSysProtoV2(83, 21, sysdef.SiStarPavilion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*StarPavilionSys).c2sExchangeGood
	})
	net.RegisterSysProtoV2(83, 22, sysdef.SiStarPavilion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*StarPavilionSys).c2sFetchDayRewards
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiStarPavilion).(*StarPavilionSys); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})
}
