package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
)

type ActorRobot struct {
	PfId       uint32
	ServerId   uint32
	ActorId    uint64
	ConfigType uint32
	ConfigId   uint64
}

func LoadActorRobots() ([]*ActorRobot, error) {
	var (
		list   []*ActorRobot
		offset int
	)
	for {
		var batch []*ActorRobot
		err := db.OrmEngine.
			Where("pf_id = ? AND server_id = ?", engine.GetPfId(), engine.GetServerId()).
			Limit(1000, offset).
			Find(&batch)
		if err != nil {
			logger.LogError(err.Error())
			return nil, err
		}

		offset += len(batch)

		for _, item := range batch {
			list = append(list, item)
		}

		if len(batch) < 1000 {
			break
		}
	}

	return list, nil
}

func AddActorRobots(robots []*ActorRobot) error {
	_, err := db.OrmEngine.Insert(robots)
	if err != nil {
		logger.LogError(err.Error())
		return err
	}
	return nil
}

func DelActorRobots(robotIds []uint64, typ custom_id.ActorRobotType) error {
	_, err := db.OrmEngine.In("config_id", robotIds).Where("config_type=?", typ).Delete(&ActorRobot{})
	if err != nil {
		logger.LogError(err.Error())
		return err
	}
	return nil
}

func onAddActorRobots(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveRankList", 1, len(args)) {
		return
	}
	list, ok := args[0].([]*ActorRobot)
	if !ok {
		return
	}

	if err := AddActorRobots(list); err != nil {
		logger.LogError(err.Error())
		return
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgAddActorRobots, onAddActorRobots)
	})
}
