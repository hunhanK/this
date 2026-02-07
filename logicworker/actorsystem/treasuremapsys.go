/**
 * @Author: LvYuMeng
 * @Date: 2024/5/9
 * @Desc: 藏宝图
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/actsweepmgr"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

type TreasureMapEventFunc func(player iface.IPlayer, conf *jsondata.TreasureMapConf, awardLibsId, posIdx uint32) error

var treasureMapEvent map[uint32]TreasureMapEventFunc = make(map[uint32]TreasureMapEventFunc)

const (
	treasureMapEvent_TreasureChest     = 1
	treasureMapEvent_StrangeEncounters = 2
	treasureMapEvent_MirrorFb          = 3
	treasureMapEvent_TreasureCave      = 4
)

func registerTreasureMapEvent(eventType uint32, fn TreasureMapEventFunc) {
	_, ok := treasureMapEvent[eventType]
	if ok {
		logger.LogWarn("repeated register treasureMapEvent %d", eventType)
	}

	treasureMapEvent[eventType] = fn
}

type TreasureMapSys struct {
	Base
	StrangeEncountersAwards jsondata.StdRewardVec
}

func (s *TreasureMapSys) OnOpen() {
	s.s2cInfo()
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByTreasureMap, s.GetOwner().GetId())
}

func (s *TreasureMapSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *TreasureMapSys) OnReconnect() {
	s.s2cInfo()
	s.s2cRunningEvent()
}
func (s *TreasureMapSys) s2cRunningEvent() {
	if len(s.StrangeEncountersAwards) > 0 {
		s.SendProto3(66, 2, &pb3.S2C_66_2{Awards: jsondata.StdRewardVecToPb3RewardVec(s.StrangeEncountersAwards)})
	}
}

func (s *TreasureMapSys) OnLogout() {
	if nil != s.StrangeEncountersAwards {
		engine.GiveRewards(s.owner, s.StrangeEncountersAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTreasureMapAward,
		})
		s.StrangeEncountersAwards = nil
	}
}

func (s *TreasureMapSys) s2cInfo() {
	data := s.data()
	s.SendProto3(66, 0, &pb3.S2C_66_0{FreeCount: data.FreeCount, EnterCaveTimes: data.TreasureCaveCount})
}

func (s *TreasureMapSys) data() *pb3.TreasureMap {
	binary := s.GetBinaryData()
	if nil == binary.TreasureMap {
		binary.TreasureMap = &pb3.TreasureMap{}
	}
	return binary.TreasureMap
}

func (s *TreasureMapSys) IsInPos(idx uint32) bool {
	conf := jsondata.GetTreasureMapConf()
	if nil == conf {
		return false
	}

	if int(idx) >= len(conf.Pos) {
		return false
	}

	sceneId, treasureX, treasureY := conf.Pos[idx].SceneId, conf.Pos[idx].X, conf.Pos[idx].Y

	if s.owner.GetFbId() != 0 || s.owner.GetSceneId() != sceneId {
		return false
	}

	playerX, playerY := s.owner.GetCurrentPos()

	return base.InRange(uint32(playerX), uint32(playerY), treasureX, treasureY, conf.ExploreDistance)
}

func (s *TreasureMapSys) c2sUseMap(msg *base.Message) error {
	var req pb3.C2S_66_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetTreasureMapConf()
	if nil == conf {
		return neterror.ConfNotFoundError("treasure conf is nil")
	}

	//确认在坐标附近
	if !s.IsInPos(req.PosIdx) {
		return neterror.ParamsInvalidError("not found treasure map pos(%d)", req.PosIdx)
	}

	mapId := req.MapId

	mapConf := conf.TreasureMap[mapId]
	if nil == mapConf {
		return neterror.ParamsInvalidError("not found treasure map map(%d)", mapId)
	}

	data := s.data()

	if mapId == conf.FreeMapId && data.FreeCount >= conf.FreeCount {
		return neterror.ParamsInvalidError("not found treasure free times not enough")
	}

	var eventLibsId, awardLibsId uint32

	isCaveLimit := data.TreasureCaveCount >= conf.TreasureCave.CaveEnterTimes
	//随机事件
	pool := new(random.Pool)
	for i := 0; i < len(mapConf.EventParams); i += 2 {
		if isCaveLimit {
			eConf := conf.TreasureMapEvents[mapConf.EventParams[i]]
			if nil == eConf {
				return neterror.InternalError("not found treasure map event(%d)", eventLibsId)
			}
			if eConf.Type == treasureMapEvent_TreasureCave {
				continue
			}
		}
		pool.AddItem(mapConf.EventParams[i], mapConf.EventParams[i+1])
	}
	eventLibsId = pool.RandomOne().(uint32)

	eventConf := conf.TreasureMapEvents[eventLibsId]
	if nil == eventConf {
		return neterror.InternalError("not found treasure map event(%d)", eventLibsId)
	}

	//随机奖励
	awardParams := mapConf.AwardParams
	if len(eventConf.SpLibs) > 0 {
		awardParams = eventConf.SpLibs
	}
	pool.Clear()
	for i := 0; i < len(awardParams); i += 2 {
		pool.AddItem(awardParams[i], awardParams[i+1])
	}
	awardLibsId = pool.RandomOne().(uint32)

	fn := treasureMapEvent[eventConf.Type]
	if nil == fn {
		return neterror.InternalError("not register treasure map event(%d)", eventLibsId)
	}

	if mapId == conf.FreeMapId {
		if data.FreeCount >= conf.FreeCount {
			return neterror.ParamsInvalidError("not found treasure free times not enough")
		}
		data.FreeCount++
		s.s2cInfo()
		s.owner.TriggerQuestEvent(custom_id.QttTreasureFreeMap, 0, 1)
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogTreasureMapConsume, &pb3.LogPlayerCounter{NumArgs: uint64(data.FreeCount)})
		event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByTreasureMap, s.GetOwner().GetId())
	} else {
		if !s.owner.ConsumeByConf(mapConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogTreasureMapConsume}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	}

	s.owner.TriggerQuestEvent(custom_id.QttTreasureMap, 0, 1)

	if eventConf.Type == treasureMapEvent_TreasureCave {
		data.TreasureCaveCount++
		s.s2cInfo()
	}

	err := fn(s.owner, conf, awardLibsId, req.PosIdx)
	if nil != err {
		return err
	}

	s.SendProto3(66, 1, &pb3.S2C_66_1{EventType: eventConf.Type})
	return nil
}

func (s *TreasureMapSys) c2sRevStrangeEncounters(msg *base.Message) error {
	var req pb3.C2S_66_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if nil == s.StrangeEncountersAwards {
		return neterror.ParamsInvalidError("treasure strange encounters is not set")
	}

	if req.IsShow {
		s.SendProto3(66, 2, &pb3.S2C_66_2{Awards: jsondata.StdRewardVecToPb3RewardVec(s.StrangeEncountersAwards)})
		return nil
	}

	if len(s.StrangeEncountersAwards) > 0 {
		engine.GiveRewards(s.owner, s.StrangeEncountersAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTreasureMapAward,
		})
	}
	s.StrangeEncountersAwards = nil
	return nil
}

func onTreasureMapEventTreasureChest(player iface.IPlayer, conf *jsondata.TreasureMapConf, awardLibsId, posIdx uint32) error {
	rewards := jsondata.GetTreasureMapAwards(awardLibsId)
	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTreasureMapAward,
		})
	}
	return nil
}

func onTreasureMapEvenStrangeEncounters(player iface.IPlayer, conf *jsondata.TreasureMapConf, awardLibsId, posIdx uint32) error {
	s, ok := player.GetSysObj(sysdef.SiTreasureMap).(*TreasureMapSys)
	if !ok || !s.IsOpen() {
		return neterror.ConfNotFoundError("treasure map sys get err")
	}
	s.StrangeEncountersAwards = jsondata.GetTreasureMapAwards(awardLibsId)
	return nil
}

func onTreasureMapEventMirrorFb(player iface.IPlayer, conf *jsondata.TreasureMapConf, awardLibsId, posIdx uint32) error {
	rewards := jsondata.GetTreasureMapAwards(awardLibsId)
	posConf := conf.Pos[posIdx]

	var playerX, playerY int32
	pos := player.GetBinaryData().GetPos()
	if pos != nil {
		playerX, playerY = pos.PosX, pos.PosY
	}

	reqEnter := &pb3.EnterMirrorFb{
		MirrorId: posConf.MirrorId,
		X:        playerX,
		Y:        playerY,
		PassAward: &pb3.MirrorPassAward{
			Awards:  jsondata.StdRewardVecToPb3RewardVec(rewards),
			RevFlag: false,
			LogId:   uint32(pb3.LogId_LogTreasureMapAward),
		},
	}

	err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterMirror, reqEnter)
	if err != nil {
		return err
	}

	return nil
}

func onTreasureMapEventTreasureCave(player iface.IPlayer, conf *jsondata.TreasureMapConf, awardLibsId, posIdx uint32) error {
	defaultServer := utils.Ternary(player.InLocalFightSrv(), base.LocalFightServer, base.SmallCrossServer).(base.ServerType)
	err := player.EnterFightSrv(defaultServer, fubendef.EnterTreasureCave, &pb3.CommonSt{
		U32Param:  awardLibsId,
		U32Param2: posIdx,
	})
	if err != nil {
		return err
	}
	return nil
}

func onTreasureMapNewDay(player iface.IPlayer, args ...interface{}) {
	s, ok := player.GetSysObj(sysdef.SiTreasureMap).(*TreasureMapSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.data()
	data.TreasureCaveCount = 0
	data.FreeCount = 0
	s.s2cInfo()
}

var singleTreasureController = &TreasureController{}

type TreasureController struct {
	actsweepmgr.Base
}

func (receiver *TreasureController) GetUseTimes(id uint32, playerId uint64) (useTimes uint32, ret bool) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiTreasureMap)
	if obj == nil || !obj.IsOpen() {
		return
	}

	s, ok := obj.(*TreasureMapSys)
	if !ok {
		return
	}

	return s.data().FreeCount, true
}

func (receiver *TreasureController) GetCanUseTimes(_ uint32, playerId uint64) (canUseTimes uint32) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiTreasureMap)
	if obj == nil || !obj.IsOpen() {
		return
	}

	_, ok := obj.(*TreasureMapSys)
	if !ok {
		return
	}

	conf := jsondata.GetTreasureMapConf()
	if nil == conf {
		return
	}

	return conf.FreeCount
}

func (receiver *TreasureController) AddUseTimes(_ uint32, times uint32, playerId uint64) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiTreasureMap)
	if obj == nil || !obj.IsOpen() {
		return
	}

	s, ok := obj.(*TreasureMapSys)
	if !ok {
		return
	}

	data := s.data()
	data.FreeCount += times

	s.owner.TriggerQuestEvent(custom_id.QttTreasureFreeMap, 0, int64(times))
	s.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiTreasureMap, func() iface.ISystem {
		return &TreasureMapSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, onTreasureMapNewDay)

	net.RegisterSysProtoV2(66, 1, sysdef.SiTreasureMap, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TreasureMapSys).c2sUseMap
	})
	net.RegisterSysProtoV2(66, 2, sysdef.SiTreasureMap, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TreasureMapSys).c2sRevStrangeEncounters
	})

	registerTreasureMapEvent(treasureMapEvent_TreasureChest, onTreasureMapEventTreasureChest)
	registerTreasureMapEvent(treasureMapEvent_StrangeEncounters, onTreasureMapEvenStrangeEncounters)
	registerTreasureMapEvent(treasureMapEvent_MirrorFb, onTreasureMapEventMirrorFb)
	registerTreasureMapEvent(treasureMapEvent_TreasureCave, onTreasureMapEventTreasureCave)

	actsweepmgr.Reg(actsweepmgr.ActSweepByTreasureMap, singleTreasureController)

	gmevent.Register("treasureMap", func(player iface.IPlayer, args ...string) bool {
		et := utils.AtoUint32(args[0])
		awardId := utils.AtoUint32(args[1])
		posIdx := utils.AtoUint32(args[2])
		fn := treasureMapEvent[et]
		if nil == fn {
			return false
		}
		err := fn(player, jsondata.GetTreasureMapConf(), awardId, posIdx)
		if nil != err {
			return false
		}
		player.SendProto3(66, 1, &pb3.S2C_66_1{EventType: et})
		return true
	}, 1)
}
