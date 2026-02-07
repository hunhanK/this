/**
 * @Author: lzp
 * @Date: 2024/1/15
 * @Desc: 欢乐年夜饭
**/

package pyy

import (
	"fmt"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type EvalGiftSys struct {
	PlayerYYBase
}

func (s *EvalGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EvalGiftSys) Login() {
	s.s2cInfo()
}

func (s *EvalGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *EvalGiftSys) NewDay() {
	data := s.getData()
	for _, gData := range data.GiftData {
		gData.IsRecDailyRewards = false
	}
	s.s2cInfo()
}

func (s *EvalGiftSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.EvalGift == nil {
		return
	}
	delete(state.EvalGift, s.Id)
}

func (s *EvalGiftSys) s2cInfo() {
	s.SendProto3(127, 134, &pb3.S2C_127_134{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *EvalGiftSys) getData() *pb3.PYY_EvalGift {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.EvalGift == nil {
		state.EvalGift = make(map[uint32]*pb3.PYY_EvalGift)
	}
	if state.EvalGift[s.Id] == nil {
		state.EvalGift[s.Id] = &pb3.PYY_EvalGift{}
	}
	data := state.EvalGift[s.Id]
	if data.GiftData == nil {
		data.GiftData = make(map[uint32]*pb3.EvalGift)
	}
	return data
}

func (s *EvalGiftSys) c2sPurchase(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}

	var req pb3.C2S_127_135
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetPYYEvalGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("config not found")
	}

	data := s.getData()

	// 检查是否可以一键购买
	checkCanOnceBuy := func() bool {
		canBuy := true
		for _, gData := range data.GiftData {
			if gData.IsBuy {
				canBuy = false
			}
		}
		return canBuy
	}

	if req.GiftId > 0 {
		gConf := jsondata.GetPYYEvalGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
		if gConf == nil {
			return neterror.ParamsInvalidError("giftId: %d not found config", req.GiftId)
		}
		gData, ok := data.GiftData[req.GiftId]
		if ok && gData.IsBuy {
			return neterror.ParamsInvalidError("giftId: %d has bought", req.GiftId)
		}
	} else {
		if !checkCanOnceBuy() {
			return neterror.ParamsInvalidError("cannot once buy")
		}
	}

	var consumes jsondata.ConsumeVec
	var rewards jsondata.StdRewardVec
	if req.GiftId > 0 {
		gConf := jsondata.GetPYYEvalGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
		if gConf == nil {
			return neterror.ConfNotFoundError("%s not found %d conf", s.GetPrefix(), req.GiftId)
		}
		consumes = gConf.Consumes
		rewards = gConf.Rewards
	} else {
		consumes = conf.OnceConsumes
		for _, gConf := range conf.Gifts {
			rewards = append(rewards, gConf.Rewards...)
		}
	}

	if !s.GetPlayer().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogEvalGiftBuyConsume}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	// 设置数据
	if req.GiftId > 0 {
		data.GiftData[req.GiftId] = &pb3.EvalGift{IsBuy: true}
	} else {
		for _, gConf := range conf.Gifts {
			data.GiftData[gConf.GiftId] = &pb3.EvalGift{IsBuy: true}
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogEvalGiftBuyAward})
	}

	if req.GiftId > 0 {
		logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogEvalGiftBuyAward, &pb3.LogPlayerCounter{
			NumArgs: uint64(s.GetId()),
			StrArgs: fmt.Sprintf("%d", req.GiftId),
		})
	} else {
		for _, gConf := range conf.Gifts {
			logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogEvalGiftBuyAward, &pb3.LogPlayerCounter{
				NumArgs: uint64(s.GetId()),
				StrArgs: fmt.Sprintf("%d", gConf.GiftId),
			})
		}
	}

	s.SendProto3(127, 135, &pb3.S2C_127_135{
		ActId:   s.GetId(),
		GiftId:  req.GiftId,
		Rewards: jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	s.s2cInfo()
	return nil
}

func (s *EvalGiftSys) c2sRecRewards(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_136
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	gConf := jsondata.GetPYYEvalGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil {
		return neterror.ParamsInvalidError("giftId: %d not found config", req.GiftId)
	}

	data := s.getData()
	gData, ok := data.GiftData[req.GiftId]
	if !ok || !gData.IsBuy {
		return neterror.ParamsInvalidError("giftId: %d not buy", req.GiftId)
	}

	day := s.GetOpenDay()
	var canRecDays []uint32
	for _, dConf := range gConf.DayRewards {
		if dConf.Day > day {
			continue
		}
		if utils.SliceContainsUint32(gData.IdL, dConf.Day) {
			continue
		}
		canRecDays = append(canRecDays, dConf.Day)
	}

	var rewards jsondata.StdRewardVec
	if !gData.IsRecDailyRewards {
		rewards = append(rewards, gConf.DailyRewards...)
	}

	for _, day := range canRecDays {
		dConf := jsondata.GetPYYEvalGiftDayRewards(s.ConfName, s.ConfIdx, req.GiftId, day)
		if dConf != nil {
			rewards = append(rewards, dConf.Rewards...)
		}
	}

	gData.IsRecDailyRewards = true
	gData.IdL = append(gData.IdL, canRecDays...)
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogEvalGiftRecAward})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogEvalGiftRecAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%v", canRecDays),
	})

	s.SendProto3(127, 136, &pb3.S2C_127_136{
		ActId:   s.GetId(),
		GiftId:  req.GiftId,
		Rewards: jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	s.s2cInfo()
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYEvalGift, func() iface.IPlayerYY {
		return &EvalGiftSys{}
	})

	net.RegisterYYSysProtoV2(127, 135, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*EvalGiftSys).c2sPurchase
	})
	net.RegisterYYSysProtoV2(127, 136, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*EvalGiftSys).c2sRecRewards
	})
}
