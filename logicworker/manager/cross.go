package manager

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/redisworker/redismid"
)

func regGameSrv() {
	data := argsdef.GameBasicData{
		PfId:                      engine.GetPfId(),
		SrvId:                     engine.GetServerId(),
		OpenAt:                    gshare.GetOpenServerTime(),
		WorldLevel:                gshare.GetWorldLevel(),
		TopFight:                  gshare.GetTopFight(),
		TopLevel:                  gshare.GetTopLevel(),
		MergeTimes:                gshare.GetMergeTimes(),
		MergeTimestamp:            gshare.GetStaticVar().GetMergeTimestamp(),
		SrvStatusFlag:             gshare.GetSrvStatusFlag(),
		FirstMediumCrossTimestamp: gshare.GetFirstMediumCrossTimestamp(),
	}

	gshare.SendRedisMsg(redismid.SaveGameBasic, data)
}

func onSeCrossMatched(_ ...interface{}) {
	hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)
	crossCamp := uint32(hostInfo.Camp)
	AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.SetExtraAttr(attrdef.SmallCrossCamp, attrdef.AttrValueAlias(crossCamp))
		player.SetExtraAttr(attrdef.MediumCrossCamp, attrdef.AttrValueAlias(engine.GetMediumCrossCamp()))
	})

	// 更新场景机器人的跨服信息
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FUpdateSceneRobotSmallCross, &pb3.UpdateSceneRobotSt{
		SmallCrossCamp: crossCamp,
	})
	if err != nil {
		logger.LogError("SeCrossMatched G2FUpdateSceneRobotSmallCross err:%v", err)
	}
}

func onSeMediumCrossMatched(_ ...interface{}) {
	key := engine.GetMediumCrossCamp()
	AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.SetExtraAttr(attrdef.MediumCrossCamp, attrdef.AttrValueAlias(key))
		player.SendServerInfo()
	})
}

func init() {
	event.RegSysEvent(custom_id.SeHourArrive, func(args ...interface{}) {
		regGameSrv()
	})
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		regGameSrv()
	})
	event.RegSysEventL(custom_id.SeMerge, func(args ...interface{}) {
		gshare.ClearSrvStatusFlag(gshare.SrvStatusByCanMerge)
		regGameSrv()
	})
	event.RegSysEventL(custom_id.SeCmdGmBeforeMerge, func(args ...interface{}) {
		gshare.SetSrvStatusFlag(gshare.SrvStatusByCanMerge)
		regGameSrv()
	})
	event.RegSysEvent(custom_id.SeRegGameSrv, func(args ...interface{}) {
		regGameSrv()
	})
	event.RegSysEvent(custom_id.SeCrossMatched, onSeCrossMatched)
	event.RegSysEvent(custom_id.SeMediumCrossMatched, onSeMediumCrossMatched)
}
