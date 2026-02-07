/**
 * @Author: lzp
 * @Date: 2025/1/15
 * @Desc:
**/

package pyy

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
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

type SFCollectSys struct {
	PlayerYYBase
}

func (s *SFCollectSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SFCollectSys) Login() {
	s.s2cInfo()
}

func (s *SFCollectSys) OnOpen() {
	s.addMonExtraDrops()
	s.s2cInfo()
}

func (s *SFCollectSys) OnEnd() {
	s.delMonExtraDrops()
}

func (s *SFCollectSys) OnAfterLogin() {
	if !s.IsOpen() {
		return
	}
	s.addMonExtraDrops()
}

func (s *SFCollectSys) NewDay() {
	data := s.getData()
	for itemId := range data.ExChangeData {
		eConf := jsondata.GetPYYSFCollectExchangeConf(s.ConfName, s.ConfIdx, itemId)
		if eConf != nil && eConf.DailyReset == 1 {
			data.ExChangeData[itemId] = 0
		}
	}
	s.s2cInfo()
}

func (s *SFCollectSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.SFCollect == nil {
		return
	}
	delete(state.SFCollect, s.Id)
}

func (s *SFCollectSys) s2cInfo() {
	s.SendProto3(127, 132, &pb3.S2C_127_132{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *SFCollectSys) getData() *pb3.PYY_SpringFestivalCollect {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.SFCollect == nil {
		state.SFCollect = make(map[uint32]*pb3.PYY_SpringFestivalCollect)
	}
	if state.SFCollect[s.Id] == nil {
		state.SFCollect[s.Id] = &pb3.PYY_SpringFestivalCollect{}
	}
	data := state.SFCollect[s.Id]
	if data.ExChangeData == nil {
		data.ExChangeData = make(map[uint32]uint32)
	}
	return data
}

func (s *SFCollectSys) OnLoginFight() {
	s.addMonExtraDrops()
}

func (s *SFCollectSys) addMonExtraDrops() {
	conf := jsondata.GetPYYSFCollectConf(s.ConfName, s.ConfIdx)
	for _, mon := range conf.Monsters {
		err := s.GetPlayer().CallActorFunc(actorfuncid.AddMonExtraDrops, &pb3.AddActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
		if err != nil {
			s.GetPlayer().LogWarn("err:%v", err)
		}
	}
}

func (s *SFCollectSys) delMonExtraDrops() {
	conf := jsondata.GetPYYSFCollectConf(s.ConfName, s.ConfIdx)
	for _, mon := range conf.Monsters {
		err := s.GetPlayer().CallActorFunc(actorfuncid.DelMonExtraDrops, &pb3.DelActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
		if err != nil {
			s.GetPlayer().LogWarn("err:%v", err)
		}
	}
}

func (s *SFCollectSys) c2sExchange(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_133
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	eConf := jsondata.GetPYYSFCollectExchangeConf(s.ConfName, s.ConfIdx, req.Id)
	if eConf == nil {
		return neterror.ParamsInvalidError("exchange id:%d not exit", req.Id)
	}

	data := s.getData()
	count := data.ExChangeData[req.Id]
	if count > eConf.Count {
		return neterror.ParamsInvalidError("exchange id:%d count limit", req.Id)
	}

	if !s.GetPlayer().ConsumeByConf(eConf.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogSpringFestivalCollectConsume}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	data.ExChangeData[req.Id] += 1
	if len(eConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), eConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSpringFestivalCollectAward})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSpringFestivalCollectAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", eConf.Id),
	})

	s.SendProto3(127, 133, &pb3.S2C_127_133{ActId: s.GetId(), Id: req.Id})
	s.s2cInfo()
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSpringFestivalCollect, func() iface.IPlayerYY {
		return &SFCollectSys{}
	})

	net.RegisterYYSysProtoV2(127, 133, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*SFCollectSys).c2sExchange
	})
}
