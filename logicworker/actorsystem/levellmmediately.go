/**
 * @Author: LvYuMeng
 * @Date: 2025/9/5
 * @Desc: 百级直升
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type LevelImmediatelySys struct {
	Base
}

func (s *LevelImmediatelySys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *LevelImmediatelySys) OnReconnect() {
	s.s2cInfo()
}

func (s *LevelImmediatelySys) s2cInfo() {
	s.SendProto3(37, 10, &pb3.S2C_37_10{Data: s.getData()})
}

func (s *LevelImmediatelySys) getData() *pb3.LevelImmediatelyData {
	binary := s.GetBinaryData()
	if nil == binary.LevelImmediatelyData {
		binary.LevelImmediatelyData = &pb3.LevelImmediatelyData{}
	}
	if nil == binary.LevelImmediatelyData.Gift {
		binary.LevelImmediatelyData.Gift = make(map[uint32]uint32)
	}
	return binary.LevelImmediatelyData
}

func (s *LevelImmediatelySys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf, ok := jsondata.GetLevelImmediatelyConfByChargeId(chargeConf.ChargeId)
	if !ok {
		return false
	}

	if s.owner.GetLevel() > conf.MaxLevel {
		return false
	}

	data := s.getData()
	if data.Gift[conf.Id] > 0 {
		return false
	}

	return true
}

func (s *LevelImmediatelySys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	conf, ok := jsondata.GetLevelImmediatelyConfByChargeId(chargeConf.ChargeId)
	if !ok {
		return false
	}

	data := s.getData()
	if data.Gift[conf.Id] > 0 {
		return false
	}

	data.Gift[conf.Id]++
	s.SendProto3(37, 11, &pb3.S2C_37_11{
		Id:    conf.Id,
		Count: data.Gift[conf.Id],
	})

	engine.GiveRewards(s.owner, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLevelImmediatelyCharge})
	return true
}

func levelImmediatelyChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiLevelImmediately).(*LevelImmediatelySys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeCheck(chargeConf)
}

func levelImmediatelyChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiLevelImmediately).(*LevelImmediatelySys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeBack(chargeConf)
}

func init() {
	RegisterSysClass(sysdef.SiLevelImmediately, func() iface.ISystem {
		return &LevelImmediatelySys{}
	})

	engine.RegChargeEvent(chargedef.LevelImmediatelyData, levelImmediatelyChargeCheck, levelImmediatelyChargeBack)
}
