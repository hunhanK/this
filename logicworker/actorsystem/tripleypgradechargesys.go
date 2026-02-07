/**
 * @Author: LvYuMeng
 * @Date: 2025/12/15
 * @Desc: 三倍直购
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

type TripleUpgradeChargeSys struct {
	Base
}

func (s *TripleUpgradeChargeSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *TripleUpgradeChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *TripleUpgradeChargeSys) s2cInfo() {
	s.SendProto3(131, 5, &pb3.S2C_131_5{Data: s.getData()})
}

func (s *TripleUpgradeChargeSys) getData() *pb3.TripleUpgradeCharge {
	binary := s.GetBinaryData()
	if binary.TripleUpgradeCharge == nil {
		binary.TripleUpgradeCharge = &pb3.TripleUpgradeCharge{}
	}
	if binary.TripleUpgradeCharge.BuyCount == nil {
		binary.TripleUpgradeCharge.BuyCount = make(map[uint32]uint32)
	}
	return binary.TripleUpgradeCharge
}

func (s *TripleUpgradeChargeSys) chargeCheck(chargeId uint32) bool {
	conf := jsondata.GetTripleUpgradeChargeByChargeId(chargeId)
	if nil == conf {
		return false
	}
	data := s.getData()
	if data.BuyCount[conf.Id] >= conf.Count {
		return false
	}
	openSrvDay := gshare.GetOpenServerDay()
	if len(conf.OpenSrvDay) >= 2 {
		if conf.OpenSrvDay[0] > 0 && conf.OpenSrvDay[0] > openSrvDay {
			return false
		}
		if conf.OpenSrvDay[1] > 0 && conf.OpenSrvDay[1] < openSrvDay {
			return false
		}
	}
	return true
}

func (s *TripleUpgradeChargeSys) chargeBack(chargeId uint32) bool {
	if !s.chargeCheck(chargeId) {
		return false
	}
	conf := jsondata.GetTripleUpgradeChargeByChargeId(chargeId)
	if nil == conf {
		return false
	}
	data := s.getData()
	data.BuyCount[conf.Id]++
	s.SendProto3(131, 6, &pb3.S2C_131_6{
		Id:    conf.Id,
		Count: data.BuyCount[conf.Id],
	})
	engine.GiveRewards(s.owner, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogTripleUpgradeChargeAwards})
	if conf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(conf.BroadcastId, s.owner.GetId(), s.owner.GetName(), conf.Name, engine.StdRewardToBroadcast(s.owner, conf.Rewards), conf.TipsDesc, conf.TipsJumpId)
	}
	return true
}

func (s *TripleUpgradeChargeSys) onNewDay() {
	data := s.getData()
	data.BuyCount = nil
	s.s2cInfo()
}

func tripleUpgradeChargeChargeCheck(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiTripleUpgradeCharge).(*TripleUpgradeChargeSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeCheck(chargeConf.ChargeId)
}

func tripleUpgradeChargeChargeBack(player iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiTripleUpgradeCharge).(*TripleUpgradeChargeSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	return sys.chargeBack(chargeConf.ChargeId)
}

func init() {
	RegisterSysClass(sysdef.SiTripleUpgradeCharge, func() iface.ISystem {
		return &TripleUpgradeChargeSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiTripleUpgradeCharge).(*TripleUpgradeChargeSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	engine.RegChargeEvent(chargedef.TripleUpgradeCharge, tripleUpgradeChargeChargeCheck, tripleUpgradeChargeChargeBack)
}
