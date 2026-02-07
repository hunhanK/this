package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type DiamondSupplySys struct {
	Base
}

func (s *DiamondSupplySys) getData() *pb3.DiamondSupplyData {
	data := s.GetBinaryData().DiamondSupplyData
	if data == nil {
		s.GetBinaryData().DiamondSupplyData = &pb3.DiamondSupplyData{}
		data = s.GetBinaryData().DiamondSupplyData
	}

	if data.LevelPack == nil {
		data.LevelPack = make(map[uint32]*pb3.DiamondSupplyPack)
	}
	for k := range data.LevelPack {
		if data.LevelPack[k] == nil {
			data.LevelPack[k] = s.getLevelPack(k)
		}

	}
	return data
}

func (s *DiamondSupplySys) getLevelPack(level uint32) *pb3.DiamondSupplyPack {
	data := s.getData()
	levelPack, exists := data.LevelPack[level]
	if !exists {
		levelPack = &pb3.DiamondSupplyPack{
			Level:   level,
			BuyPack: make(map[uint32]uint32),
			Time:    0,
		}
		data.LevelPack[level] = levelPack
	} else if levelPack.BuyPack == nil {
		levelPack.BuyPack = make(map[uint32]uint32)
	}
	return levelPack
}

func (s *DiamondSupplySys) s2cInfo() {
	s.SendProto3(36, 14, &pb3.S2C_36_14{
		DiamondSupplyData: s.getData(),
	})
}

func (s *DiamondSupplySys) OnReconnect() {
	s.s2cInfo()
}

func (s *DiamondSupplySys) OnLogin() {
	s.s2cInfo()
}

func (s *DiamondSupplySys) OnOpen() {
	s.s2cInfo()
}

func (s *DiamondSupplySys) onNewDay() {
	data := s.getData()
	data.LevelPack = make(map[uint32]*pb3.DiamondSupplyPack)
	s.s2cInfo()
}

func (s *DiamondSupplySys) checkPreLevel(preLevel uint32) bool {
	levelConf, ok := jsondata.GetDiamondSupplyConfByLevel(preLevel)
	if !ok {
		return false
	}

	data := s.getData()

	levelPack, exists := data.LevelPack[preLevel]
	if !exists {
		return false
	}

	for packId := range levelConf.LevelPack {
		if _, bought := levelPack.BuyPack[packId]; !bought {
			return false
		}
	}

	return true
}

func (s *DiamondSupplySys) c2sPackAppear(msg *base.Message) error {
	var req pb3.C2S_36_15
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	levelConf, ok := jsondata.GetDiamondSupplyConfByLevel(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("DiamondSupplyConf is nil")
	}
	if levelConf.PreLevel != 0 {
		if !s.checkPreLevel(levelConf.PreLevel) {
			return neterror.ParamsInvalidError("preLevel no buy")
		}
	}
	levelPack := s.getLevelPack(req.Level)
	if levelPack.Time == 0 {
		data.LevelPack[req.Level].Time = req.Time
	}

	s.SendProto3(36, 15, &pb3.S2C_36_15{})
	s.s2cInfo()
	return nil
}

func diamondSupplyChargeBack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !diamondSupplyChargeCheck(actor, conf) {
		return false
	}
	obj := actor.GetSysObj(sysdef.SiDiamondSupply)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(*DiamondSupplySys)
	if !ok {
		return false
	}
	packConf, level := jsondata.GetDiamondSupplyPackConfByChargeId(conf.ChargeId)
	if level == 0 {
		return false
	}
	data := s.getData()
	levelData := s.getLevelPack(level)
	if levelData != nil {
		data.LevelPack[level].BuyPack[packConf.PackId] = uint32(time_util.Now().Unix())
	}
	if s.checkPreLevel(level) { //如果当前档全部买完
		nextLevel := jsondata.GetDiamondSupplyNextLevel(level)
		if nextLevel != 0 {
			nextLevelData := s.getLevelPack(nextLevel)
			if nextLevelData != nil && nextLevelData.Time == 0 {
				data.LevelPack[nextLevel].Time = time_util.NowSec()
			}
			s.s2cInfo()
		}
	}

	s.SendProto3(36, 16, &pb3.S2C_36_16{
		Level:  level,
		PackId: packConf.PackId,
	})
	if len(packConf.Awards) > 0 {
		engine.GiveRewards(s.owner, packConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogDiamondSupplyAward,
		})
	}
	return true
}

func diamondSupplyChargeCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	obj := actor.GetSysObj(sysdef.SiDiamondSupply)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(*DiamondSupplySys)
	if !ok {
		return false
	}

	packConf, level := jsondata.GetDiamondSupplyPackConfByChargeId(conf.ChargeId)
	if level == 0 {
		return false
	}
	data := s.getLevelPack(level)
	levelConf, ok := jsondata.GetDiamondSupplyConfByLevel(level)
	if !ok {
		return false
	}
	if data.Time == 0 {
		return false
	}
	if data.Time+levelConf.LiveTime < uint32(time_util.Now().Unix()) {
		return false
	}
	if levelConf.PreLevel != 0 {
		if !s.checkPreLevel(levelConf.PreLevel) {
			return false
		}
	}
	levelPack := s.getLevelPack(level)
	if levelPack.BuyPack[packConf.PackId] != 0 {
		return false
	}

	return true
}

func init() {
	RegisterSysClass(sysdef.SiDiamondSupply, func() iface.ISystem {
		return &DiamondSupplySys{}
	})
	net.RegisterSysProtoV2(36, 15, sysdef.SiDiamondSupply, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DiamondSupplySys).c2sPackAppear
	})
	engine.RegChargeEvent(chargedef.DiamondSupply, diamondSupplyChargeCheck, diamondSupplyChargeBack)
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiDiamondSupply).(*DiamondSupplySys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})
	gmevent.Register("diamondSupply", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiDiamondSupply)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*DiamondSupplySys)
		if sys == nil {
			return false
		}
		level := utils.AtoUint32(args[0])
		data := sys.getData()
		levelPack := sys.getLevelPack(level)
		if levelPack.Time == 0 {
			data.LevelPack[level].Time = time_util.NowSec()
		}
		return true
	}, 1)
}
