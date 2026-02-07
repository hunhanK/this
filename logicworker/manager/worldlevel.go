package manager

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"

	"github.com/gzjjyz/logger"
)

func syncWorldLevelToLocalFightSrv() {
	if !engine.FightClientExistPredicate(base.LocalFightServer) {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSyncWorldLevel, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		U32Param3: gshare.GetWorldLevel(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func syncWorldLevelToSmallCrossFightSrv() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		logger.LogWarn("syncWorldLevelToSmallCrossFightSrv small cross srv exist")
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncWorldLevel, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		U32Param3: gshare.GetWorldLevel(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
	logger.LogInfo("syncWorldLevelToSmallCrossFightSrv worldLevel %d", gshare.GetWorldLevel())
}

func syncTopFightToLocalFightSrv() {
	if !engine.FightClientExistPredicate(base.LocalFightServer) {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSyncTopFight, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		I64Param:  gshare.GetTopFight(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func syncTopFightToSmallCrossFightSrv() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		logger.LogWarn("syncWorldLevelToSmallCrossFightSrv small cross srv exist")
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncTopFight, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		I64Param:  gshare.GetTopFight(),
	})

	if err != nil {
		logger.LogError("err:%v", err)
	}
	logger.LogInfo("syncTopFightToSmallCrossSrv topFight %d", gshare.GetTopFight())
}

func onSmallCrossSyncWorldLevelToLogic(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("crossSyncWorldLevelToLogic err:%v", err)
		return
	}

	gshare.SetSmallCrossWorldLevel(req.U32Param)
	event.TriggerSysEvent(custom_id.SeRefreshSmallCrossWorldLevel)
}

func syncTopLevelToLocalFightSrv() {
	if !engine.FightClientExistPredicate(base.LocalFightServer) {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSyncTopLevel, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		U32Param3: gshare.GetTopLevel(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func syncTopLevelToSmallCrossFightSrv() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		logger.LogWarn("syncTopLevelToSmallCrossFightSrv small cross srv exist")
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncTopLevel, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		U32Param3: gshare.GetTopLevel(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
	logger.LogInfo("syncTopLevelToSmallCrossFightSrv worldLevel %d", gshare.GetWorldLevel())
}

func init() {
	engine.RegisterSysCall(sysfuncid.SmallCrossSyncWorldLevelToLogic, onSmallCrossSyncWorldLevelToLogic)

	event.RegSysEvent(custom_id.SeRefreshWorldLevel, func(args ...interface{}) {
		syncWorldLevelToLocalFightSrv()
		syncWorldLevelToSmallCrossFightSrv()
		engine.Broadcast(chatdef.CIWorld, 0, 2, 6, PackServerInfo(), 0)
	})

	event.RegSysEvent(custom_id.SeRefreshTopFight, func(args ...interface{}) {
		syncTopFightToLocalFightSrv()
		syncTopFightToSmallCrossFightSrv()
	})

	event.RegSysEvent(custom_id.SeRefreshTopLevel, func(args ...interface{}) {
		syncTopLevelToLocalFightSrv()
		syncTopLevelToSmallCrossFightSrv()
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		syncWorldLevelToSmallCrossFightSrv()
		syncTopFightToSmallCrossFightSrv()
		syncTopLevelToSmallCrossFightSrv()
	})

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		syncWorldLevelToLocalFightSrv()
		syncTopLevelToLocalFightSrv()
	})
}
