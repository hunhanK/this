package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type PlayerYYPlaneTreasureBox struct {
	PlayerYYBase
}

func (s *PlayerYYPlaneTreasureBox) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *PlayerYYPlaneTreasureBox) OnOpen() {
	s.S2CInfo()
}

func (s *PlayerYYPlaneTreasureBox) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *PlayerYYPlaneTreasureBox) GetData() *pb3.PYY_PlaneTreasureBox {
	if s.GetYYData().PlaneTreasureBox == nil {
		s.GetYYData().PlaneTreasureBox = make(map[uint32]*pb3.PYY_PlaneTreasureBox)
	}

	data, ok := s.GetYYData().PlaneTreasureBox[s.GetId()]
	if !ok {
		data = &pb3.PYY_PlaneTreasureBox{}
		s.GetYYData().PlaneTreasureBox[s.GetId()] = data
	}
	if data.ExChangeData == nil {
		data.ExChangeData = make(map[uint32]uint32)
	}

	return data
}
func (s *PlayerYYPlaneTreasureBox) ResetData() {
	if s.GetYYData().PlaneTreasureBox == nil {
		return
	}
	delete(s.GetYYData().PlaneTreasureBox, s.Id)
}

func (s *PlayerYYPlaneTreasureBox) S2CInfo() {
	s.SendProto3(69, 40, &pb3.S2C_69_40{
		ActiveId: s.GetId(),
		Info:     s.GetData(),
	})
}

func (s *PlayerYYPlaneTreasureBox) GetConfByGoodId(goodId uint32) *jsondata.PYYPlaneTreasureBoxGood {
	return jsondata.GetPYYPlaneTreasureBoxConf(s.ConfName, s.GetConfIdx(), goodId)
}

func (s *PlayerYYPlaneTreasureBox) c2sExchange(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_69_41
	if err := msg.UnPackPb3Msg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	goodId := req.GoodId
	exNum := req.Num

	conf := s.GetConfByGoodId(goodId)
	if conf == nil {
		return neterror.ConfNotFoundError("get conf failed actId %d", s.GetId())
	}

	data := s.GetData()
	if data.ExChangeData[goodId]+exNum > conf.ExchangeLimit {
		return neterror.ConfNotFoundError("exchange limit actId:%d, goodId:%d", s.GetId(), goodId)
	}

	var consumes jsondata.ConsumeVec
	config := jsondata.GetItemConfig(conf.ItemId)
	if itemdef.IsMoney(config.Type, config.SubType) {
		consumes = append(consumes, &jsondata.Consume{Id: config.SubType, Count: conf.ItemNum * exNum, Type: custom_id.ConsumeTypeMoney})
	} else {
		consumes = append(consumes, &jsondata.Consume{Id: conf.ItemId, Count: conf.ItemNum * exNum})
	}
	if !s.GetPlayer().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogPlaneTreasureBoxExchangeConsume}) {
		return neterror.ParamsInvalidError("consumes not enough actId:%d, goodId:%d", s.GetId(), goodId)
	}

	data.ExChangeData[goodId] += exNum
	rewards := jsondata.StdRewardMulti(conf.Rewards, int64(exNum))
	if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlaneTreasureBoxExchangeAward,
	}) {
		return neterror.InternalError("rewards failed actId:%d, goodId:%d", s.GetId(), goodId)
	}

	s.SendProto3(69, 41, &pb3.S2C_69_41{
		ActiveId: s.GetId(),
		GoodId:   goodId,
		Num:      data.ExChangeData[goodId],
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneTreasureBox, func() iface.IPlayerYY {
		return &PlayerYYPlaneTreasureBox{}
	})

	net.RegisterYYSysProtoV2(69, 41, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlaneTreasureBox).c2sExchange
	})
}
