package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type DirectRebateSys struct {
	Base
}

func (s *DirectRebateSys) getData() *pb3.DirectRebate {
	data := s.GetBinaryData().DirectRebateData
	if data == nil {
		s.GetBinaryData().DirectRebateData = &pb3.DirectRebate{}
		data = s.GetBinaryData().DirectRebateData
	}
	if data.DirectRebateRec == nil {
		data.DirectRebateRec = make(map[uint32]*pb3.DirectRebateRec)
	}
	return data
}

func (s *DirectRebateSys) s2cInfo() {
	s.SendProto3(127, 200, &pb3.S2C_127_200{
		DirectRebate: s.getData(),
	})
}

func (s *DirectRebateSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DirectRebateSys) OnLogin() {
	s.s2cInfo()
}

func (s *DirectRebateSys) OnOpen() {
	s.s2cInfo()
}

func (s *DirectRebateSys) c2sLevelIdRev(msg *base.Message) error {
	var req pb3.C2S_127_202
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	conf, ok := jsondata.GetDirectRebateConf(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("DirectRebateLevelConf is not found")
	}
	levelConf, ok := jsondata.GetDirectRebateLevelConfByLevel(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("DirectRebateLevelConf is not found")
	}
	drlevelConf, ok := levelConf[req.LevelId]
	if !ok {
		return neterror.ConfNotFoundError("DirectRebateLeveIdlConf is not found")
	}
	if drlevelConf.Days > gshare.GetOpenServerDay() {
		return neterror.ParamsInvalidError("open ser days not enough")
	}
	data := s.getData()
	if data.DirectRebateRec[req.Level] == nil {
		data.DirectRebateRec[req.Level] = &pb3.DirectRebateRec{
			Rec: make([]uint32, 0),
		}
	}
	for _, v := range data.DirectRebateRec[req.Level].Rec {
		if v == req.LevelId {
			return neterror.ParamsInvalidError("this reward is rec")
		}
	}
	data.DirectRebateRec[req.Level].Rec = append(data.DirectRebateRec[req.Level].Rec, req.LevelId)
	if len(drlevelConf.Awards) > 0 {
		engine.GiveRewards(s.owner, drlevelConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogDirectRebateRewards,
		})
		if conf.BroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.BroadcastId, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.GetOwner(), drlevelConf.Awards))
		}

	}
	s.SendProto3(127, 202, &pb3.S2C_127_202{
		LevelId: req.LevelId,
		Level:   req.Level,
	})

	s.s2cInfo()
	return nil
}

func (s *DirectRebateSys) c2sLevelRev(msg *base.Message) error {
	var req pb3.C2S_127_203
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	conf, ok := jsondata.GetDirectRebateConf(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("DirectRebateLevelConf is not found")
	}

	levelConf, ok := jsondata.GetDirectRebateLevelConfByLevel(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("DirectRebateLevelConf is not found")
	}
	data := s.getData()
	if data.DirectRebateRec[req.Level] == nil {
		data.DirectRebateRec[req.Level] = &pb3.DirectRebateRec{
			Rec: make([]uint32, 0),
		}
	}
	playerData := data.DirectRebateRec[req.Level]
	var totalAwards jsondata.StdRewardVec
	var receivedLevelIds []uint32
	openServerDay := gshare.GetOpenServerDay()
	for levelId, drLevelConf := range levelConf {
		if drLevelConf == nil {
			continue
		}

		if drLevelConf.Days > openServerDay {
			continue
		}

		isReceived := false
		for _, receivedId := range playerData.Rec {
			if receivedId == levelId {
				isReceived = true
				break
			}
		}
		if isReceived {
			continue
		}

		totalAwards = append(totalAwards, drLevelConf.Awards...)
		receivedLevelIds = append(receivedLevelIds, levelId)
	}
	data.DirectRebateRec[req.Level].Rec = append(data.DirectRebateRec[req.Level].Rec, receivedLevelIds...)
	if len(totalAwards) > 0 {
		engine.GiveRewards(s.owner, totalAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogDirectRebateRewards,
		})
		if conf.BroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.BroadcastId, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.GetOwner(), jsondata.MergeStdReward(engine.FilterRewardByPlayer(s.GetOwner(), totalAwards))))
		}

	}

	s.SendProto3(127, 203, &pb3.S2C_127_203{
		Level: req.Level,
	})
	s.s2cInfo()
	return nil
}

func directRebateChargeBack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !directRebateChargeCheck(actor, conf) {
		return false
	}
	obj := actor.GetSysObj(sysdef.SiDirectRebate)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys, ok := obj.(*DirectRebateSys)
	if !ok {
		return false
	}
	data := sys.getData()
	level, ok := jsondata.GetDirectRebateLevelByChargeId(conf.ChargeId)
	if !ok {
		return false
	}
	data.Buy = append(data.Buy, level)
	sys.SendProto3(127, 201, &pb3.S2C_127_201{
		Level: level,
	})
	logworker.LogPlayerBehavior(actor, pb3.LogId_LogDirectRebateCharge, &pb3.LogPlayerCounter{
		NumArgs: uint64(sys.sysId),
	})
	return true
}

func directRebateChargeCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	obj := actor.GetSysObj(sysdef.SiDirectRebate)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys, ok := obj.(*DirectRebateSys)
	if !ok {
		return false
	}
	data := sys.getData()
	level, ok := jsondata.GetDirectRebateLevelByChargeId(conf.ChargeId)
	if !ok {
		return false
	}
	levelConf, ok := jsondata.GetDirectRebateConf(level)
	if !ok {
		return false
	}
	preBought := false
	if levelConf.PreLevel == 0 {
		preBought = true
	} else {
		for _, v := range data.Buy {
			if v == level {
				return false
			}
			if v == levelConf.PreLevel {
				preBought = true
			}
		}
	}
	if !preBought {
		return false
	}
	return true

}

func init() {
	RegisterSysClass(sysdef.SiDirectRebate, func() iface.ISystem {
		return &DirectRebateSys{}
	})
	engine.RegChargeEvent(chargedef.DirectRebate, directRebateChargeCheck, directRebateChargeBack)
	net.RegisterSysProtoV2(127, 202, sysdef.SiDirectRebate, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DirectRebateSys).c2sLevelIdRev
	})
	net.RegisterSysProtoV2(127, 203, sysdef.SiDirectRebate, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DirectRebateSys).c2sLevelRev
	})
}
