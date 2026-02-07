package actorsystem

import (
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math/rand"
)

type CsRedPacketSys struct {
	Base
}

func (s *CsRedPacketSys) GetData() *pb3.CsRedPacket {
	binary := s.GetBinaryData()
	if binary.CsRedPacket == nil {
		binary.CsRedPacket = new(pb3.CsRedPacket)
	}

	if binary.CsRedPacket.CurRecRedPacketIdx == 0 {
		binary.CsRedPacket.CurRecRedPacketIdx = 1
	}

	return binary.CsRedPacket
}

func (s *CsRedPacketSys) OnLogin() {
	s.S2CInfo()
}

func (s *CsRedPacketSys) OnReconnect() {
	s.S2CInfo()
}

func (s *CsRedPacketSys) S2CInfo() {
	s.SendProto3(147, 1, &pb3.S2C_147_1{
		State: s.GetData(),
	})
}

func (s *CsRedPacketSys) OnOpen() {
	s.S2CInfo()
}

// 开启红包
func (s *CsRedPacketSys) c2sOpen(_ *base.Message) (err error) {
	data := s.GetData()
	idx := data.CurRecRedPacketIdx

	if data.CurAmount == 0 { // 没有充值金额，表示还未开启过红包
		return s.open(idx)
	} else {
		s.SendProto3(147, 2, &pb3.S2C_147_2{
			Idx:    idx,
			Amount: data.CurAmount,
		})
	}

	return
}

func (s *CsRedPacketSys) open(idx uint32) error {
	conf, ok := jsondata.GetCsRedPacketConf(idx)
	if !ok {
		return fmt.Errorf("idx %d, err %w", idx, jsondata.ErrJsonDataNotFound)
	}

	data := s.GetData()

	// 随机金额
	amount := uint32(rand.Intn(int(conf.Max-conf.Min))) + conf.Min

	data.CurAmount = amount
	data.Exchange = false

	owner := s.GetOwner()
	engine.BroadcastTipMsgById(conf.TipsId, owner.GetName(), conf.Name, fmt.Sprintf("%.2f", float64(amount)/100))

	// 下发开启成功
	s.SendProto3(147, 2, &pb3.S2C_147_2{
		Idx:    idx,
		Amount: amount,
	})

	bytes, _ := json.Marshal(map[string]interface{}{
		"RecRedPacketIdx": idx,
		"Money":           fmt.Sprintf("%.2f", float64(amount)/100),
		"CsName":          conf.Name,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCsRedPacketOpen, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})

	return nil
}

// 请求兑换红包
func (s *CsRedPacketSys) c2sExchange(_ *base.Message) error {
	data := s.GetData()
	idx := data.CurRecRedPacketIdx

	if data.IsOver {
		return neterror.ParamsInvalidError("red packet is over")
	}

	cfg, ok := jsondata.GetCsRedPacketConf(idx)
	if !ok {
		return neterror.ConfNotFoundError("idx %d, err %w", idx, jsondata.ErrJsonDataNotFound)
	}

	owner := s.GetOwner()

	// 消耗背包中的红包
	cv := jsondata.ConsumeVec{{Id: cfg.ItemId, Count: 1}}
	if ok := owner.ConsumeByConf(cv, false, common.ConsumeParams{LogId: pb3.LogId_LogCsRedPacketExchange}); !ok {
		s.GetOwner().LogWarn("consume red packet failed id is %d, itemId is %d", idx, cfg.ItemId)
		return nil
	}

	// 下发结果
	entry := &pb3.CsRedPacketEntry{
		Idx:    data.CurRecRedPacketIdx,
		Amount: data.CurAmount,
	}

	data.ExchangeRedPackets = append(data.ExchangeRedPackets, entry)
	data.Exchange = true

	// 去充值
	chargeSys := owner.GetSysObj(sysdef.SiCharge).(*ChargeSys)
	var params = &pb3.OnChargeParams{
		ChargeId:           0,
		CashCent:           data.CurAmount,
		SkipLogFirstCharge: true,
	}
	chargeSys.OnCharge(params, pb3.LogId_LogCsRedPacket)

	if jsondata.CsRedPacketConfMgr != nil {
		data.IsOver = len(data.ExchangeRedPackets) == len(jsondata.CsRedPacketConfMgr)
	}

	s.SendProto3(147, 3, &pb3.S2C_147_3{
		Idx:    entry.Idx,
		Amount: entry.Amount,
	})

	// 还有下一个红包, idx递增，清空当前充值金额并通知客户端
	if !data.IsOver {
		data.CurRecRedPacketIdx++
		data.CurAmount = 0
		data.Exchange = false

		s.S2CInfo()
	}

	bytes, _ := json.Marshal(map[string]interface{}{
		"RecRedPacketIdx": entry.Idx,
		"Money":           entry.Amount,
		"IsOver":          data.IsOver,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCsRedPacketExchange, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiCsRedPacket, func() iface.ISystem {
		return &CsRedPacketSys{}
	})

	net.RegisterSysProtoV2(147, 1, sysdef.SiCsRedPacket, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CsRedPacketSys).c2sOpen
	})
	net.RegisterSysProtoV2(147, 2, sysdef.SiCsRedPacket, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CsRedPacketSys).c2sExchange
	})
}
