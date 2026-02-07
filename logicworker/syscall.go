package logicworker

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"time"

	"github.com/gzjjyz/logger"
)

// 战斗服调用逻辑服函数回调
func onCallLogicSrvFunc(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	data, ok := args[0].([]byte)
	if !ok {
		return
	}

	var st pb3.CallGameSrvFunc
	if nil != pb3.Unmarshal(data, &st) {
		return
	}

	if cb := engine.GetSysCall(uint16(st.FnId)); nil != cb {
		cb(st.Buff)
	} else {
		logger.LogError("on call logic server func error. no callback func with id=%d", st.FnId)
	}
}

func handleSeCrossMatch(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	info, ok := args[0].(*argsdef.GameGetCross)
	if !ok {
		logger.LogError("跨服信息获取失败")
		return
	}

	gshare.SetSmallCrossTime(info.CrossTime)
	gshare.SetCrossAllocTimes(info.Times)

	logger.LogInfo("收到跨服分配信息,小跨服：%v", info)
	fightworker.AddFightClient(base.SmallCrossServer, &fightworker.FightHostInfo{
		Host:    fmt.Sprintf("%s:%d", info.Host, info.Port),
		Name:    base.ServerTypeStrMap[base.SmallCrossServer],
		Camp:    crosscamp.CampType(info.Camp),
		ZoneId:  info.ZoneId,
		CrossId: info.CrossId,
	})

	fight := fightworker.GetFightClient(base.SmallCrossServer)
	if fight != nil {
		fight.StartUp()
	}
	event.TriggerSysEvent(custom_id.SeCrossMatched, nil)
	logger.LogInfo("跨服匹配加载成功:%v", info)
}

func handleSeDelayCrossBreak(args ...interface{}) {
	gshare.SetSmallCrossTime(0)
	gshare.SetCrossAllocTimes(0)
	gshare.SetMatchSrvNum(0)
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.SetExtraAttr(attrdef.SmallCrossCamp, attrdef.AttrValueAlias(0))
		player.SetExtraAttr(attrdef.MediumCrossCamp, attrdef.AttrValueAlias(0))
	})
	// 更新场景机器人的跨服信息
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FUpdateSceneRobotSmallCross, &pb3.UpdateSceneRobotSt{
		SmallCrossCamp: 0,
	})
	if err != nil {
		logger.LogError("SeCrossMatched G2FUpdateSceneRobotSmallCross err:%v", err)
	}
	engine.Broadcast(chatdef.CIWorld, 0, 152, 25, &pb3.S2C_152_25{}, 0)
	fightworker.DelHostInfo(base.SmallCrossServer)
	event.TriggerSysEvent(custom_id.SeCleanCrossConnSuccess)
}

func handleSeMediumCrossBreak(args ...interface{}) {
	gshare.SetMediumCrossTime(0)
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.SetExtraAttr(attrdef.MediumCrossCamp, attrdef.AttrValueAlias(0))
		player.SendServerInfo()
	})

	fightworker.DelHostInfo(base.MediumCrossServer)
	event.TriggerSysEvent(custom_id.SeCleanMediumCrossConnSuccess)
}

func handleSeMediumCrossMatch(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	info, ok := args[0].(*argsdef.GameGetMediumCross)
	if !ok {
		logger.LogError("跨服信息获取失败")
		return
	}

	gshare.SetMediumCrossTime(info.CrossTime)

	logger.LogInfo("收到跨服分配信息,中跨服：%v", info)
	fightworker.AddFightClient(base.MediumCrossServer, &fightworker.FightHostInfo{
		Host:    fmt.Sprintf("%s:%d", info.Host, info.Port),
		Name:    base.ServerTypeStrMap[base.MediumCrossServer],
		ZoneId:  info.ZoneId,
		CrossId: info.CrossId,
	})

	fight := fightworker.GetFightClient(base.MediumCrossServer)
	if fight != nil {
		fight.StartUp()
	}
	event.TriggerSysEvent(custom_id.SeMediumCrossMatched, nil)
	logger.LogInfo("中跨服匹配加载成功:%v", info)
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgCallLogicSrvFun, onCallLogicSrvFunc)
	})

	engine.RegisterSysCall(sysfuncid.FGLogicSrvConnSucc, func(_ []byte) {
		logger.LogInfo("LocalFightSrv 连接成功")
		event.TriggerSysEvent(custom_id.SeFightSrvConnSucc)
	})

	engine.RegisterSysCall(sysfuncid.FGLogicSrvConnCrossSucc, func(buf []byte) {
		msg := pb3.SyncConnectCrossSuccess{}
		if err := pb3.Unmarshal(buf, &msg); err != nil {
			return
		}

		logger.LogInfo("收到跨服连接成功消息，跨服类型：%d", msg.CrossType)
		event.TriggerSysEvent(custom_id.SeCrossSrvConnSucc, &msg)
		gshare.SetSrvStatusFlag(gshare.SrvStatusByConnSmallCross)
		event.TriggerSysEvent(custom_id.SeRegGameSrv)
	})

	event.RegSysEvent(custom_id.SeCrossMatch, handleSeCrossMatch)

	event.RegSysEvent(custom_id.SeDelayCrossMatch, func(args ...interface{}) {
		timer.SetTimeout(3*time.Second, func() {
			event.TriggerSysEvent(custom_id.SeCrossMatch, args...)
		})
	})
	event.RegSysEvent(custom_id.SeDelayCrossBreak, func(args ...interface{}) {
		fightworker.ClientDestroy(base.SmallCrossServer)
		event.TriggerSysEvent(custom_id.SeCrossDisconnect, nil)
		gshare.ClearSrvStatusFlag(gshare.SrvStatusByConnSmallCross)
		event.TriggerSysEvent(custom_id.SeRegGameSrv)
	})
	event.RegSysEvent(custom_id.SeDelayCleanCrossData, handleSeDelayCrossBreak)

	event.RegSysEvent(custom_id.SeMediumCrossMatch, handleSeMediumCrossMatch)

	event.RegSysEvent(custom_id.SeDelayMediumCrossMatch, func(args ...interface{}) {
		timer.SetTimeout(3*time.Second, func() {
			event.TriggerSysEvent(custom_id.SeMediumCrossMatch, args...)
		})
	})

	event.RegSysEvent(custom_id.SeMediumCrossBreak, func(args ...interface{}) {
		fightworker.ClientDestroy(base.MediumCrossServer)
		event.TriggerSysEvent(custom_id.SeMediumCrossDisconnect, nil)
		gshare.ClearSrvStatusFlag(gshare.SrvStatusByConnMediumCross)
		event.TriggerSysEvent(custom_id.SeRegGameSrv)
	})
	event.RegSysEvent(custom_id.SeCleanMediumCrossData, handleSeMediumCrossBreak)
}
