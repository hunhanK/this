package actorsystem

import (
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
	"jjyz/gameserver/net"
)

type LimitExtractSys struct {
	Base
}

func (s *LimitExtractSys) getData() *pb3.LimitExtract {
	data := s.GetBinaryData().LimitExtract
	if data == nil {
		s.GetBinaryData().LimitExtract = &pb3.LimitExtract{}
		data = s.GetBinaryData().LimitExtract
	}
	if data.Extracted == nil {
		data.Extracted = make(map[uint32]uint32)
	}
	if data.EveryLevelRec == nil {
		data.EveryLevelRec = make(map[uint32]uint32)
	}
	return data
}

func (s *LimitExtractSys) s2cInfo() {
	s.SendProto3(36, 12, &pb3.S2C_36_12{
		LimitExtractData: s.getData(),
	})
}

func (s *LimitExtractSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LimitExtractSys) OnLogin() {
	s.s2cInfo()
}

func (s *LimitExtractSys) OnOpen() {
	s.s2cInfo()
}

func (s *LimitExtractSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_36_13
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	conf, ok := jsondata.GetLimitExtractConf()
	if !ok {
		return neterror.ConfNotFoundError("limitExtract conf not found")
	}

	var totalAwards jsondata.StdRewardVec
	for _, levelConf := range conf {
		var day uint32
		for weekDay := uint32(1); weekDay <= uint32(7); weekDay++ {
			cashCent := data.Extracted[weekDay]
			if cashCent < levelConf.ChargeCent {
				continue
			}
			day++
		}

		for _, dayConf := range levelConf.LimitExtractLevel {
			if utils.IsSetBit(data.EveryLevelRec[levelConf.Level], dayConf.Days-1) {
				continue
			}
			if dayConf.Days > day {
				continue
			}
			totalAwards = append(totalAwards, dayConf.Awards...)
			data.EveryLevelRec[levelConf.Level] = utils.SetBit(data.EveryLevelRec[levelConf.Level], dayConf.Days-1)
		}
	}

	if len(totalAwards) == 0 {
		return nil
	}

	engine.GiveRewards(s.owner, totalAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogLimitExtractAwards,
	})

	s.SendProto3(36, 13, &pb3.S2C_36_13{})
	s.s2cInfo()
	return nil
}

func (s *LimitExtractSys) onNewWeek() {
	data := s.getData()
	data.Extracted = make(map[uint32]uint32)
	data.EveryLevelRec = make(map[uint32]uint32)
	s.s2cInfo()
}

func handleAddLimitExtract(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	charge, ok1 := args[0].(*custom_id.ActorEventCharge)
	if !ok1 {
		return
	}
	if charge.LogId != pb3.LogId_LogChargeByUseDiamonTokens {
		return
	}
	s, ok := player.GetSysObj(sysdef.SiLimitExtract).(*LimitExtractSys)
	if !ok || !s.IsOpen() {
		return
	}
	if charge.CashCent > 0 {
		data := s.getData()
		day := uint32(time_util.Now().Weekday())
		if day == 0 {
			day = 7
		}
		data.Extracted[day] += charge.CashCent
	}
	s.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiLimitExtract, func() iface.ISystem {
		return &LimitExtractSys{}
	})
	event.RegActorEvent(custom_id.AeCharge, handleAddLimitExtract)
	net.RegisterSysProtoV2(36, 13, sysdef.SiLimitExtract, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LimitExtractSys).c2sRev
	})
	event.RegActorEvent(custom_id.AeNewWeek, func(actor iface.IPlayer, args ...interface{}) {
		if s, ok := actor.GetSysObj(sysdef.SiLimitExtract).(*LimitExtractSys); ok {
			s.onNewWeek()
		}
	})
}
