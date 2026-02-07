package robotmgr

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/gameserver/dbworker"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
)

var RobotMgrInstance = NewRobotMgr()

type ActorRobot struct {
	ActorId       uint64 // 竞技场自动生成的字段
	RobotType     custom_id.ActorRobotType
	RobotConfigId uint64 // 配制机器人：配制ID; 真实玩家：玩家ID
}

type RobotMgr struct {
	ActorRobots         map[uint64]*ActorRobot
	Type2ActorRobotMap  map[custom_id.ActorRobotType][]*ActorRobot
	newAddRobotActorIds []uint64
}

func (m *RobotMgr) LoadRobots() error {
	actorRobots, err := dbworker.LoadActorRobots()
	if err != nil {
		logger.LogError(err.Error())
		return err
	}

	m.ActorRobots = map[uint64]*ActorRobot{}
	m.Type2ActorRobotMap = map[custom_id.ActorRobotType][]*ActorRobot{}

	var maxId uint64
	for _, actorRobotDO := range actorRobots {
		actorRobot := &ActorRobot{
			ActorId:       actorRobotDO.ActorId,
			RobotType:     custom_id.ActorRobotType(actorRobotDO.ConfigType),
			RobotConfigId: actorRobotDO.ConfigId,
		}
		m.ActorRobots[actorRobot.ActorId] = actorRobot
		list, _ := m.Type2ActorRobotMap[actorRobot.RobotType]
		m.Type2ActorRobotMap[actorRobot.RobotType] = append(list, actorRobot)
		if maxId < actorRobotDO.ActorId {
			maxId = actorRobotDO.ActorId
		}
	}

	if maxId > 0 {
		series.UpdateActorSeries(maxId)
	}

	if err = m.RefreshBattleArenaDefaultRobots(); err != nil {
		logger.LogError(err.Error())
		return err
	}

	return nil
}

func (m *RobotMgr) RefreshBattleArenaDefaultRobots() error {
	conf := jsondata.GetBattleArenaConf()

	robotId2robotConfMap := map[uint64]*jsondata.ActorRobot{}
	for _, robotConf := range conf.Robots {
		robotId2robotConfMap[robotConf.RobotId] = robotConf
	}

	var (
		newRobotConfs []*jsondata.ActorRobot
		delRobotIds   []uint64
	)
	origRobots, ok := m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt]
	if !ok {
		newRobotConfs = conf.Robots
	} else {
		origRobotId2robotMap := map[uint64]*ActorRobot{}
		for _, origRobot := range origRobots {
			if origRobot.RobotType != custom_id.ActorRobotTypeBattleArenaSysBuilt {
				continue
			}
			origRobotId2robotMap[origRobot.RobotConfigId] = origRobot
		}

		for _, robotConf := range conf.Robots {
			_, ok := origRobotId2robotMap[robotConf.RobotId]
			if ok {
				continue
			}
			newRobotConfs = append(newRobotConfs, robotConf)
		}

		for _, origRobot := range origRobots {
			_, ok := robotId2robotConfMap[origRobot.RobotConfigId]
			if ok {
				continue
			}
			delRobotIds = append(delRobotIds, origRobot.RobotConfigId)
		}
	}

	if len(delRobotIds) > 0 {
		if err := dbworker.DelActorRobots(delRobotIds, custom_id.ActorRobotTypeBattleArenaSysBuilt); err != nil {
			logger.LogError(err.Error())
			return err
		}

		delRobotIdSet := map[uint64]struct{}{}
		for _, delRobotId := range delRobotIds {
			delRobotIdSet[delRobotId] = struct{}{}
		}

		var remainRobots []*ActorRobot
		for _, robot := range m.ActorRobots {
			if robot.RobotType != custom_id.ActorRobotTypeBattleArenaSysBuilt {
				continue
			}
			if _, ok := delRobotIdSet[robot.RobotConfigId]; !ok {
				remainRobots = append(remainRobots, robot)
				continue
			}
		}

		m.ActorRobots = map[uint64]*ActorRobot{}
		if len(remainRobots) > 0 {
			for _, robot := range remainRobots {
				m.ActorRobots[robot.ActorId] = robot
			}
		}

		remainRobots = nil
		list, _ := m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt]
		for _, robot := range list {
			if robot.RobotType != custom_id.ActorRobotTypeBattleArenaSysBuilt {
				continue
			}
			if _, ok := delRobotIdSet[robot.RobotConfigId]; !ok {
				remainRobots = append(remainRobots, robot)
				continue
			}
			logger.LogWarn("robot(%d) del rank", robot.ActorId)
			manager.GRankMgrIns.UpdateRank(gshare.RankTypeBattleArena, robot.ActorId, 0)
		}

		if len(remainRobots) == 0 {
			m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt] = nil
		} else {
			copy(list, remainRobots)
			m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt] = list[:len(remainRobots)]
		}
	}

	logger.LogDebug("new robot config size:%d", len(newRobotConfs))
	if len(newRobotConfs) > 0 {
		var (
			robotDOs []*dbworker.ActorRobot
			robots   []*ActorRobot
		)
		pfId, sId := engine.GetPfId(), engine.GetServerId()
		for _, robotConf := range newRobotConfs {
			s, errno := series.GetActorIdSeries(engine.GetServerId())
			if 0 != errno {
				err := fmt.Errorf("alloc actor series error:%d", errno)
				logger.LogError(err.Error())
				return err
			}
			actorId, err := base.MakePlayerId(pfId, sId, s)
			if nil != err {
				logger.LogError(err.Error())
				return err
			}
			robots = append(robots, &ActorRobot{
				ActorId:       actorId,
				RobotType:     custom_id.ActorRobotTypeBattleArenaSysBuilt,
				RobotConfigId: robotConf.RobotId,
			})
			robotDOs = append(robotDOs, &dbworker.ActorRobot{
				PfId:       engine.GetPfId(),
				ServerId:   engine.GetServerId(),
				ConfigType: custom_id.ActorRobotTypeBattleArenaSysBuilt,
				ConfigId:   robotConf.RobotId,
				ActorId:    actorId,
			})
		}
		if err := dbworker.AddActorRobots(robotDOs); err != nil {
			logger.LogError(err.Error())
			return err
		}

		list, _ := m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt]
		m.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt] = append(list, robots...)
		for _, robot := range robots {
			m.ActorRobots[robot.ActorId] = robot
			rank := robotId2robotConfMap[robot.RobotConfigId].DefaultRank
			manager.GRankMgrIns.UpdateRank(gshare.RankTypeBattleArena, robot.ActorId, 10000-rank)
		}
		manager.GRankMgrIns.SaveRankByType(gshare.RankTypeBattleArena)
	}

	rankCnt := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena).GetRankCount()
	logger.LogDebug("rankCnt:%d", rankCnt)

	return nil
}

func (m *RobotMgr) AddRealActorMirrorRobot(mirrorActorId uint64) (uint64, error) {
	s, errno := series.GetActorIdSeries(engine.GetServerId())
	if 0 != errno {
		err := fmt.Errorf("alloc actor series error:%d", errno)
		logger.LogError(err.Error())
		return 0, err
	}

	robotActorId, err := base.MakePlayerId(engine.GetPfId(), engine.GetServerId(), s)
	if nil != err {
		logger.LogError(err.Error())
		return 0, err
	}
	m.ActorRobots[robotActorId] = &ActorRobot{
		ActorId:       robotActorId,
		RobotType:     custom_id.ActorRobotTypeRealActorMirror,
		RobotConfigId: mirrorActorId,
	}
	m.newAddRobotActorIds = append(m.newAddRobotActorIds, robotActorId)

	return robotActorId, nil
}

func (m *RobotMgr) SyncNewAddRobot2DB() {
	var actorRobotDOs []*dbworker.ActorRobot
	for _, actorId := range m.newAddRobotActorIds {
		actorRobot, ok := m.ActorRobots[actorId]
		if !ok {
			continue
		}
		actorRobotDOs = append(actorRobotDOs, &dbworker.ActorRobot{
			PfId:       engine.GetPfId(),
			ServerId:   engine.GetServerId(),
			ActorId:    actorId,
			ConfigId:   actorRobot.RobotConfigId,
			ConfigType: uint32(actorRobot.RobotType),
		})
	}
	if len(actorRobotDOs) > 0 {
		gshare.SendDBMsg(custom_id.GMsgAddActorRobots, actorRobotDOs)
	}
}

func (m *RobotMgr) GMForceResetBattleArena() {
	conf := jsondata.GetBattleArenaConf()
	var delRobotIds []uint64
	for _, robotConf := range conf.Robots {
		delRobotIds = append(delRobotIds, robotConf.RobotId)
	}
	if err := dbworker.DelActorRobots(delRobotIds, custom_id.ActorRobotTypeBattleArenaSysBuilt); err != nil {
		logger.LogError(err.Error())
		return
	}
	manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena).Clear()
	err := m.LoadRobots()
	if err != nil {
		logger.LogError(err.Error())
		return
	}
	BattleArenaRobotMgrInstance.refreshArenaDefaultRobots()
}

func NewRobotMgr() *RobotMgr {
	return &RobotMgr{
		Type2ActorRobotMap: map[custom_id.ActorRobotType][]*ActorRobot{},
	}
}
