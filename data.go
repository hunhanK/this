package main

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/db/migrate"
	"jjyz/base/pb3"
	"jjyz/gameserver/dbworker/autodb"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/cmdyysettingmgr"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/redisworker/redismid"
)

func LoadData() error {
	doMigrate()

	// 初始化所有数据
	gcommon.LoadJsonData(false)

	var err error

	conf := gshare.GameConf

	if err = gshare.LoadOpenSrvTime(conf.SrvId); nil != err {
		logger.LogError("load open server time error. %s", err.Error())
		return err
	}

	if err = loadGlobalVar(conf.SrvId); nil != err {
		return err
	}

	series.SetServerId(conf.SrvId)
	if err = series.LoadSeries(); nil != err {
		return err
	}
	if err = series.LoadMailSeries(); nil != err {
		return err
	}

	if err = engine.LoadName(); nil != err {
		return err
	}

	if err = manager.LoadOfflineData(); nil != err {
		return err
	}

	if err = mailmgr.Init(); nil != err {
		return err
	}

	if err = manager.GRankMgrIns.LoadRank(); nil != err {
		return err
	}

	if err = robotmgr.RobotMgrInstance.LoadRobots(); err != nil {
		return err
	}
	robotmgr.BattleArenaRobotMgrInstance.Init()
	robotmgr.InitFightSceneRobotMgr()

	if err = guildmgr.Load(conf.SrvId); nil != err {
		return err
	}

	if err = cmdyysettingmgr.Load(); nil != err {
		return err
	}

	return nil
}

func doMigrate() {
	migrate.Parse(autodb.Tables)
	migrate.BuildProcedures(autodb.Procedures)

	migrate.BuildTables()

	migrate.Exec("call initdb", true)
}

func loadCrossInfo() {
	logger.LogInfo("开始加载跨服信息")
	gshare.SendRedisMsg(redismid.LoadSmallCross)
}

func loadMediumCross() {
	logger.LogInfo("开始加载中跨服信息")
	gshare.SendRedisMsg(redismid.LoadMediumCross)
}

func loadLoadGuildRule() {
	logger.LogInfo("开始加载仙盟规则")
	gshare.SendRedisMsg(redismid.ReloadGuildRule)
}

func onLoadSmallCrossRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	data, ok := args[0].(*argsdef.GameGetCross)
	if !ok {
		return
	}

	logger.LogInfo("成功加载跨服信息")

	logger.LogInfo("============小跨服：%v", data)

	if len(data.Host) > 0 {
		fightworker.AddFightClient(base.SmallCrossServer, &fightworker.FightHostInfo{
			Host: fmt.Sprintf("%s:%d", data.Host, data.Port),
			Name: base.ServerTypeStrMap[base.SmallCrossServer],
			Camp: crosscamp.CampType(data.Camp),

			ZoneId:  data.ZoneId,
			CrossId: data.CrossId,
		})
	}

	gshare.SetCrossAllocTimes(data.Times)
	gshare.SetSmallCrossTime(data.CrossTime)

	if fight := fightworker.GetFightClient(base.SmallCrossServer); nil != fight {
		fight.StartUp()
	}
}

func onEnterSmallCrossRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	info, ok := args[0].(*argsdef.GameGetCross)
	if !ok {
		return
	}

	gshare.SetCrossAllocTimes(info.Times)
	gshare.SetSmallCrossTime(info.CrossTime)

	fight := fightworker.GetFightClient(base.SmallCrossServer)
	if fight == nil {
		event.TriggerSysEvent(custom_id.SeCrossMatch, info)
		return
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.FCDisconnectCross, &pb3.CrossDisconnect{
		PfId:   gshare.GameConf.PfId,
		SrvId:  gshare.GameConf.SrvId,
		Params: nil,
	})

	event.TriggerSysEvent(custom_id.SeDelayCrossMatch, info)
	if err != nil {
		logger.LogError("跨服断开请求失败:%v", err)
	}

	return
}

func onLoadMediumCrossRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	data, ok := args[0].(*argsdef.GameGetMediumCross)
	if !ok {
		return
	}

	logger.LogInfo("成功加载跨服信息")

	logger.LogInfo("============中跨服：%v", data)

	if len(data.Host) > 0 {
		fightworker.AddFightClient(base.MediumCrossServer, &fightworker.FightHostInfo{
			Host: fmt.Sprintf("%s:%d", data.Host, data.Port),
			Name: base.ServerTypeStrMap[base.MediumCrossServer],

			ZoneId:  data.ZoneId,
			CrossId: data.CrossId,
		})
	}

	gshare.SetMediumCrossTime(data.CrossTime)

	if fight := fightworker.GetFightClient(base.MediumCrossServer); nil != fight {
		fight.StartUp()
	}
}

func onEnterMediumCrossRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	info, ok := args[0].(*argsdef.GameGetMediumCross)
	if !ok {
		return
	}

	gshare.SetMediumCrossTime(info.CrossTime)

	fight := fightworker.GetFightClient(base.MediumCrossServer)
	if fight == nil {
		event.TriggerSysEvent(custom_id.SeMediumCrossMatch, info)
		return
	}

	err := engine.CallFightSrvFunc(base.MediumCrossServer, sysfuncid.FCDisconnectCross, &pb3.CrossDisconnect{
		PfId:   gshare.GameConf.PfId,
		SrvId:  gshare.GameConf.SrvId,
		Params: nil,
	})

	event.TriggerSysEvent(custom_id.SeDelayMediumCrossMatch, info)
	if err != nil {
		logger.LogError("跨服断开请求失败:%v", err)
	}

	return
}

func handleF2GCloseSmallCrossConn(_ []byte) {
	logger.LogInfo("收到小跨服断开消息")
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.FCDisconnectCross, &pb3.CrossDisconnect{
		PfId:   gshare.GameConf.PfId,
		SrvId:  gshare.GameConf.SrvId,
		Params: nil,
	})
	if err != nil {
		logger.LogError("跨服断开请求失败:%v", err)
	}
	event.TriggerSysEvent(custom_id.SeDelayCrossBreak)
	event.TriggerSysEvent(custom_id.SeDelayCleanCrossData)
}

func handleF2GCleanMediumCrossConn(_ []byte) {
	logger.LogInfo("收到中跨服断开消息")
	err := engine.CallFightSrvFunc(base.MediumCrossServer, sysfuncid.FCDisconnectCross, &pb3.CrossDisconnect{
		PfId:   gshare.GameConf.PfId,
		SrvId:  gshare.GameConf.SrvId,
		Params: nil,
	})
	if err != nil {
		logger.LogError("跨服断开请求失败:%v", err)
	}
	event.TriggerSysEvent(custom_id.SeMediumCrossBreak)
	event.TriggerSysEvent(custom_id.SeCleanMediumCrossData)
}

func handleF2GSyncMatchSmallSrvNum(buf []byte) {
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	gshare.SetMatchSrvNum(req.U32Param)
	engine.Broadcast(chatdef.CIWorld, 0, 152, 25, &pb3.S2C_152_25{
		MatchSrvNum: gshare.GetMatchSrvNum(),
	}, 0)
}

func init() {
	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		gshare.SendGameMsg(custom_id.GMsgSaveSrvData)
		gshare.SendDBMsg(custom_id.GMsgSaveCacheImme)
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadSmallCrossRet, onLoadSmallCrossRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadMediumCrossRet, onLoadMediumCrossRet)
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgEnterSmallCrossRet, onEnterSmallCrossRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgEnterMediumCrossRet, onEnterMediumCrossRet)
	})

	engine.RegisterSysCall(sysfuncid.F2GCleanSmallCrossConn, handleF2GCloseSmallCrossConn)
	engine.RegisterSysCall(sysfuncid.F2GCleanMediumCrossConn, handleF2GCleanMediumCrossConn)

	engine.RegisterSysCall(sysfuncid.F2GSyncMatchSmallSrvNum, handleF2GSyncMatchSmallSrvNum)
}
