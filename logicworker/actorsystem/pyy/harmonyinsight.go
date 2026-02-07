/**
 * @Author: LvYuMeng
 * @Date: 2024/12/26
 * @Desc: 鸿蒙感悟
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
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type HarmonyInsightSys struct {
	PlayerYYBase
}

func (s *HarmonyInsightSys) ResetData() {
	state := s.GetYYData()
	if nil == state.HarmonyInsight {
		return
	}
	delete(state.HarmonyInsight, s.GetId())
}

func (s *HarmonyInsightSys) Login() {
	s.s2cInfo()
}

func (s *HarmonyInsightSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HarmonyInsightSys) OnOpen() {
	s.s2cInfo()
}

func (s *HarmonyInsightSys) getData() *pb3.PYY_HarmonyInsight {
	state := s.GetYYData()
	if nil == state.HarmonyInsight {
		state.HarmonyInsight = make(map[uint32]*pb3.PYY_HarmonyInsight)
	}
	if state.HarmonyInsight[s.Id] == nil {
		state.HarmonyInsight[s.Id] = &pb3.PYY_HarmonyInsight{}
	}
	return state.HarmonyInsight[s.Id]
}

func (s *HarmonyInsightSys) s2cInfo() {
	s.SendProto3(69, 242, &pb3.S2C_69_242{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *HarmonyInsightSys) c2sBuy(msg *base.Message) error {
	conf, ok := jsondata.GetYYHarmonyInsightConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("conf is nil")
	}

	level := s.GetPlayer().GetLevel()
	if gshare.GetWorldLevel() <= level {
		return neterror.ParamsInvalidError("world level limit")
	}

	data := s.getData()
	consumeConf, ok := conf.GetHarmonyInsightConsumeConf(data.DailyBuyTimes + 1)
	if !ok {
		return neterror.ConfNotFoundError("consume conf is nil")
	}

	expConf, ok := conf.GetHarmonyInsightExpConf(level)
	if !ok {
		return neterror.ConfNotFoundError("exp conf is nil")
	}

	if !s.GetPlayer().ConsumeByConf(consumeConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogHarmonyInsightExpBuy}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.GetPlayer().AddExp(int64(expConf.Exp), pb3.LogId_LogHarmonyInsightExpBuy, false)
	data.DailyBuyTimes++

	s.s2cInfo()

	return nil
}

func (s *HarmonyInsightSys) NewDay() {
	data := s.getData()
	data.DailyBuyTimes = 0
	s.s2cInfo()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYHarmonyInsight, func() iface.IPlayerYY {
		return &HarmonyInsightSys{}
	})

	net.RegisterYYSysProtoV2(69, 243, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*HarmonyInsightSys).c2sBuy
	})
}
