/**
 * @Author: LvYuMeng
 * @Date: 2024/3/19
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type MythicalBeastLandSys struct {
	Base
}

func (s *MythicalBeastLandSys) OnLogin() {
	s.s2cInfo()
	s.fightInfo()
}

func (s *MythicalBeastLandSys) OnReconnect() {
	s.s2cInfo()
	s.fightInfo()
}

func (s *MythicalBeastLandSys) OnOpen() {
	s.data().Energy = jsondata.GetMythicalBeastLandConf().InitEnergy
	s.s2cInfo()
	s.fightInfo()
}

func (s *MythicalBeastLandSys) s2cInfo() {
	s.SendProto3(17, 80, &pb3.S2C_17_80{Data: s.data()})
}

func (s *MythicalBeastLandSys) fightInfo() {
	req := &pb3.MythicalBeastLandBossReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: s.owner.GetId(),
	}
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FReqMythicalBeastLandBoss, req)
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FReqMythicalBeastLandBoss, req)
}

func (s *MythicalBeastLandSys) data() *pb3.MythicalBeastLand {
	binary := s.owner.GetBinaryData()
	if nil == binary.MythicalBeastLand {
		binary.MythicalBeastLand = &pb3.MythicalBeastLand{}
	}
	if nil == binary.MythicalBeastLand.Gather {
		binary.MythicalBeastLand.Gather = make(map[uint32]uint32)
	}
	if nil == binary.MythicalBeastLand.BossRemind {
		binary.MythicalBeastLand.BossRemind = make(map[uint32]bool)
	}

	return binary.MythicalBeastLand
}

func (s *MythicalBeastLandSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_17_81
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	return s.enter(req.ScenId, req.IsLocal)
}

func (s *MythicalBeastLandSys) enter(sceneId uint32, isLocal bool) error {
	conf := jsondata.GetMythicalBeastLandLayerConf(sceneId, isLocal)
	if nil == conf {
		return neterror.ConfNotFoundError("MythicalBeastLandLayerConf scene(%d) is nil", sceneId)
	}

	if s.owner.GetCircle() < conf.Circle {
		s.owner.SendTipMsg(tipmsgid.CircleNotEnough)
		return nil
	}

	if !isLocal && conf.CrossTimes > 0 && conf.CrossTimes > gshare.GetCrossAllocTimes() {
		return neterror.ParamsInvalidError("cross times not meet")
	}

	enterFight := base.LocalFightServer
	if !isLocal {
		enterFight = base.SmallCrossServer
	}
	return s.owner.EnterFightSrv(enterFight, fubendef.EnterMythicalBeastLand, &pb3.CommonSt{
		U32Param: sceneId,
	})
}

func (s *MythicalBeastLandSys) c2sBuyEnergy(msg *base.Message) error {
	var req pb3.C2S_17_85
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	data := s.data()
	buyLimit, err := s.owner.GetPrivilege(privilegedef.MythicalBeastLandEnergyBuyTimes)
	if nil != err {
		return err
	}

	if int64(data.EnergyBuyTimes) >= buyLimit {
		s.owner.SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	conf := jsondata.GetMythicalBeastLandConf()
	if nil == conf {
		return neterror.ConfNotFoundError("MythicalBeastLandConf is nil")
	}

	sysConf := jsondata.GetMythicalBeastLandConf()

	//不可以超过上限
	if data.Energy >= sysConf.EnergyLimit {
		return neterror.ParamsInvalidError("MythicalBeastLandConfbuyLimit")
	}

	if !s.owner.ConsumeByConf(conf.EnergyPrice, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyMythicalBeastLandEnergy}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.EnergyBuyTimes++
	s.addEnergy(1, true)

	s.SendProto3(17, 85, &pb3.S2C_17_85{BuyTimes: data.EnergyBuyTimes})

	return nil
}

func (s *MythicalBeastLandSys) c2sRemind(msg *base.Message) error {
	var req pb3.C2S_17_90
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if nil == jsondata.GetMythicalBeastLandLayerConf(req.SceneId, req.IsLocal) {
		return neterror.ParamsInvalidError("MythicalBeastLandConf scene not exist")
	}

	monsterId := req.Id
	exist := false
	if conf := jsondata.GetMonsterLocationConf(req.SceneId); nil != conf {
		for _, v := range conf {
			if v.MonsterId == monsterId {
				exist = true
				break
			}
		}
	}
	if !exist {
		return neterror.ParamsInvalidError("MythicalBeastLandConf not exist boss(%d)", monsterId)
	}
	data := s.data()
	if req.IsRemind {
		data.BossRemind[monsterId] = true
	} else {
		delete(data.BossRemind, monsterId)
	}
	s.SendProto3(17, 90, &pb3.S2C_17_90{
		Id:       monsterId,
		IsRemind: req.IsRemind,
		SceneId:  req.SceneId,
		IsLocal:  req.IsLocal,
	})
	return nil
}

func (s *MythicalBeastLandSys) addEnergy(energy uint32, isSend bool) {
	data := s.data()
	data.Energy += energy
	s.onEnergyChange(isSend)

	msg := &pb3.CommonSt{U32Param: energy}

	proxy := s.owner.GetActorProxy()
	if nil == proxy {
		return
	}

	err := s.owner.CallActorFunc(actorfuncid.AddMythicalBeastLandEnergy, msg)
	if err != nil {
		s.LogError("MythicalBeastLandSys add energy to fight err: %v", err)
		return
	}
}

func (s *MythicalBeastLandSys) decEnergy(energy uint32) {
	data := s.data()
	if data.Energy >= energy {
		data.Energy -= energy
	} else {
		data.Energy = 0
		s.LogError("player(%d) MythicalBeastLand energy dec exceed energy(%d)", s.owner.GetId(), energy)
	}

	s.onEnergyChange(true)
}

func (s *MythicalBeastLandSys) onEnergyChange(isSend bool) {
	energy := s.data().Energy
	if isSend {
		s.SendProto3(17, 84, &pb3.S2C_17_84{Energy: energy})
	}
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogBuyMythicalEnergyChange, &pb3.LogPlayerCounter{
		NumArgs: uint64(energy),
	})
}

func decMythicalBeastLandEnergy(actor iface.IPlayer, buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		actor.LogError("MythicalBeastLandEnergy sync energy err %v", err)
		return
	}

	if sys, ok := actor.GetSysObj(sysdef.SiMythicalBeastLand).(*MythicalBeastLandSys); ok {
		sys.decEnergy(msg.U32Param)
	}
}

func syncMythicalBeastLandGatherInfo(player iface.IPlayer, buf []byte) {
	msg := &pb3.SyncMythicalBeastLandGather{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		player.LogError("MythicalBeastLandEnergy sync gather info err %v", err)
		return
	}

	if sys, ok := player.GetSysObj(sysdef.SiMythicalBeastLand).(*MythicalBeastLandSys); ok {
		data := sys.data()
		data.Gather = msg.GatherTimes
		player.SendProto3(17, 86, &pb3.S2C_17_86{Gathers: data.Gather})
	}
}

func useItemRelieveMonster(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	if param.Count > 1 {
		player.LogError("useItem:%d, count exceed 1!", param.ItemId)
		return false, false, 0
	}

	// 通用怪物复活卡不能在仙界魔王使用
	if conf.Param[0] == 0 {
		spiritFbConfig := jsondata.GetBossSpiritFbConfig()
		if spiritFbConfig != nil && player.GetFbId() == spiritFbConfig.FbId {
			return false, false, 0
		}
	}

	item := player.GetItemByHandle(param.Handle)
	msg := &pb3.G2FReqItemUse{
		ItemId:   param.ItemId,
		Bind:     item.Bind,
		Params:   conf.Param,
		ItemType: itemdef.UseItemRelieveMonster,
	}
	err := player.CallActorFunc(actorfuncid.G2FReqItemUse, msg)
	if err != nil {
		player.LogError("useItem:%d, actorfuncid.G2FReqItemUse failed!", param.ItemId)
		return false, false, 0
	}
	return true, true, 1
}

func useItemRecoverMythicalBeastLandEnergy(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	s, ok := player.GetSysObj(sysdef.SiMythicalBeastLand).(*MythicalBeastLandSys)
	if !ok || !s.IsOpen() {
		return false, false, 0
	}

	addValue := uint32(param.Count) * conf.Param[0]
	sysConf := jsondata.GetMythicalBeastLandConf()
	data := s.data()

	//不可以超过上限
	if addValue+data.Energy > sysConf.EnergyLimit {
		return false, false, 0
	}

	s.addEnergy(addValue, true)
	return true, true, param.Count
}

func onNewDayMythicalBeastLand(player iface.IPlayer, args ...interface{}) {
	s, ok := player.GetSysObj(sysdef.SiMythicalBeastLand).(*MythicalBeastLandSys)
	if !ok || !s.IsOpen() {
		return
	}
	conf := jsondata.GetMythicalBeastLandConf()
	data := s.data()
	addEnergy := conf.RecoverEnergy
	if conf.RecoverEnergy+data.Energy >= conf.EnergyLimit {
		addEnergy = conf.EnergyLimit - data.Energy
	}
	if addEnergy > 0 {
		s.addEnergy(addEnergy, false)
	}
	data.EnergyBuyTimes = 0
	data.Gather = make(map[uint32]uint32)
	s.s2cInfo()

	proxy := s.owner.GetActorProxy()
	if nil == proxy {
		return
	}
	err := s.owner.CallActorFunc(actorfuncid.G2FClearMythicalBeastLandGatherInfo, nil)
	if nil != err {
		s.LogError("MythicalBeastLandSys player(%d) reset gather times err:%v", player.GetId(), err)
	}
}

func init() {
	RegisterSysClass(sysdef.SiMythicalBeastLand, func() iface.ISystem {
		return &MythicalBeastLandSys{}
	})

	gmevent.Register("enterLocalLand", func(actor iface.IPlayer, args ...string) bool {
		sceneId := utils.AtoUint32(args[0])
		actor.EnterFightSrv(base.LocalFightServer, fubendef.EnterMythicalBeastLand, &pb3.CommonSt{
			U32Param: sceneId,
		})
		return true
	}, 1)

	net.RegisterSysProtoV2(17, 81, sysdef.SiMythicalBeastLand, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MythicalBeastLandSys).c2sEnter
	})
	net.RegisterSysProtoV2(17, 85, sysdef.SiMythicalBeastLand, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MythicalBeastLandSys).c2sBuyEnergy
	})
	net.RegisterSysProtoV2(17, 90, sysdef.SiMythicalBeastLand, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MythicalBeastLandSys).c2sRemind
	})

	event.RegActorEvent(custom_id.AeNewDay, onNewDayMythicalBeastLand)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemRelieveMonster, useItemRelieveMonster)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemRecoverMythicalBeastLandEnergy, useItemRecoverMythicalBeastLandEnergy)

	engine.RegisterActorCallFunc(playerfuncid.SyncMythicalBeastLandGatherTimes, syncMythicalBeastLandGatherInfo)
	engine.RegisterActorCallFunc(playerfuncid.DecMythicalBeastLandEnergy, decMythicalBeastLandEnergy)

}
