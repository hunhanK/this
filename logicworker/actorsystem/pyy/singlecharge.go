/**
 * @Author: LvYuMeng
 * @Date: 2024/7/17
 * @Desc: 单笔充值
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type SingleChargeSys struct {
	PlayerYYBase
}

func (s *SingleChargeSys) data() *pb3.PYY_SingleCharge {
	state := s.GetYYData()
	if state.SingleCharge == nil {
		state.SingleCharge = make(map[uint32]*pb3.PYY_SingleCharge)
	}
	if state.SingleCharge[s.Id] == nil {
		state.SingleCharge[s.Id] = &pb3.PYY_SingleCharge{}
	}
	if state.SingleCharge[s.Id].RevStatus == nil {
		state.SingleCharge[s.Id].RevStatus = make(map[uint32]*pb3.SingleChargeInfo)
	}
	return state.SingleCharge[s.Id]
}

func (s *SingleChargeSys) Login() {
	s.s2cInfo()
}

func (s *SingleChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SingleChargeSys) OnOpen() {
	s.s2cInfo()
}

func (s *SingleChargeSys) OnEnd() {
	conf, ok := jsondata.GetYYSingleChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var rewards jsondata.StdRewardVec
	data := s.data()
	for id, v := range data.RevStatus {
		awardConf, ok := conf.SingleChargeAward[id]
		if !ok {
			continue
		}
		totalRevTime := utils.MinUInt32(v.ChargeTimes, awardConf.Count)
		if v.RevTimes >= totalRevTime {
			continue
		}
		leftTimes := totalRevTime - v.RevTimes
		rewards = append(rewards, jsondata.StdRewardMulti(awardConf.Rewards, int64(leftTimes))...)
		data.RevStatus[id].RevTimes = awardConf.Count
	}
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (s *SingleChargeSys) ResetData() {
	state := s.GetYYData()
	if state.SingleCharge == nil {
		return
	}
	delete(state.SingleCharge, s.GetId())
}

func (s *SingleChargeSys) s2cInfo() {
	s.SendProto3(69, 110, &pb3.S2C_69_110{
		ActiveId: s.GetId(),
		Info:     s.data(),
	})
}

func (s *SingleChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	chargeId := chargeEvent.ChargeId
	conf, ok := jsondata.GetYYSingleChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.data()
	for id, v := range conf.SingleChargeAward {
		if v.ChargeId != chargeId {
			continue
		}
		if nil == data.RevStatus[id] {
			data.RevStatus[id] = &pb3.SingleChargeInfo{}
		}
		if data.RevStatus[id].RevTimes >= v.Count {
			continue
		}
		data.RevStatus[id].ChargeTimes++
		s.SendProto3(69, 111, &pb3.S2C_69_111{
			ActiveId: s.GetId(),
			Id:       id,
			Status:   data.RevStatus[id],
		})
	}
}

func (s *SingleChargeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_69_111
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	id := req.GetId()
	data := s.data()
	conf, ok := jsondata.GetYYSingleChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SingleChargeSys conf is nil")
	}

	if nil == conf.SingleChargeAward[id] {
		return neterror.ConfNotFoundError("SingleChargeSys award conf %d is nil", id)
	}

	info, ok := data.RevStatus[id]
	if !ok {
		return neterror.ParamsInvalidError("award conf %d not can receive status", id)
	}

	if info.RevTimes >= conf.SingleChargeAward[id].Count {
		return neterror.ParamsInvalidError("times limit %d", id)
	}

	if info.ChargeTimes <= info.RevTimes {
		return neterror.ParamsInvalidError("times limit %d", id)
	}

	info.RevTimes++
	engine.GiveRewards(s.GetPlayer(), conf.SingleChargeAward[id].Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSingleChargeAward,
	})
	s.SendProto3(69, 111, &pb3.S2C_69_111{
		ActiveId: s.GetId(),
		Id:       id,
		Status:   data.RevStatus[id],
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSingleCharge, func() iface.IPlayerYY {
		return &SingleChargeSys{}
	})

	net.RegisterYYSysProtoV2(69, 111, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*SingleChargeSys).c2sRev
	})
}
