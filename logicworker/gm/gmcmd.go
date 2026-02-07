/**
* @Author: ChenJunJi
* @Desc: 后台gm命令
* @Date: 2021/7/14 11:14
 */

package gm

import (
	"fmt"
	"io"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/cmd"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/crossservice"
	"jjyz/gameserver/engine/disableproto"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/pyy"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/cmdyysettingmgr"
	"jjyz/gameserver/logicworker/dartcarmgr"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gm/gmflag"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/invitecodemgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/merge"
	"jjyz/gameserver/model"
	"jjyz/gameserver/redisworker/redismid"
	"strconv"
	"strings"

	micro "github.com/gzjjyz/simple-micro"
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

var handleMap = map[string]func(args *model.GmCmd){
	"loadOpenTime":                        cmdLoadOpenTime,                            // 加载开服时间
	"setSmallCross":                       cmdSetSmallCross,                           // 设置小跨服
	"smallCrossDoMatch":                   cmdSmallCrossDoMatch,                       // 执行小跨服匹配
	"pprof":                               cmdSetPProf,                                // 开启pProf
	"rsf":                                 cmdReloadJson,                              // 热更配置
	"addNotice":                           cmdAddNotice,                               // 后台公告
	"removeNotice":                        cmdRemoveNotice,                            // 删除公告
	"servermail":                          cmdSrvMail,                                 // 加载全服邮件
	"getmail":                             cmdGetMail,                                 // 加载玩家邮件
	"delActorMail":                        cmdDelActorMail,                            // 删除玩家邮件
	"kick":                                cmdKick,                                    // 踢下线
	"jinyan":                              cmdJinYan,                                  // 禁言
	"loadcache":                           cmdLoadCache,                               // 回滚玩家数据
	"recovery":                            cmdRecoveryPlayer,                          // 恢复角色
	"kickAll":                             cmdKickAllPlayer,                           // 踢所有玩家下线
	"setDisableLoginFlag":                 cmdSetDisableLoginFlag,                     // 设置登录标记 param1 > 0 禁止 <= 0 可登录
	"forbidProto":                         cmdForbidProto,                             // 屏蔽协议 param1 protoIdH  param2 protoIdL
	"unFroBidProto":                       cmdUnFroBidProto,                           // 取消屏蔽协议 param1 protoIdH  param2 protoIdL
	"forbidCreatePlayer":                  cmdForbidCreatePlayer,                      // 禁止创建角色 param1 1 禁止 0 可登录
	"clearChatMsg":                        cmdClearChatMsg,                            // 清除玩家聊天记录
	"rsfsome":                             cmdReloadSomeJson,                          // 热更某些配置
	"finishMainQuest":                     cmdFinishMainQuest,                         // 完成主线任务
	"dirtyCache":                          cmdDirtyCache,                              // 把actorcache的dirty全部都设置为true
	"chargeevent":                         cmdChargeEvent,                             // 调用充值事件
	"merge":                               cmdMerge,                                   // 合服(合并数据库后操作)
	"setMergeTimes":                       cmdMergeTimes,                              // 设置合服次数
	"setMergeDay":                         cmdMergeDay,                                // 设置合服天数
	"delTitle":                            cmdDelTitle,                                // 删除称号
	"openAct":                             cmdOpenAct,                                 // 开启日常活动
	"openCrossAct":                        cmdOpenCrossAct,                            // 开启跨服日常活动
	"closeAct":                            cmdCloseAct,                                // 关闭日常活动
	"closeCrossAct":                       cmdCloseCrossAct,                           // 关闭跨服日常活动
	"showFightInfo":                       cmdFightInfo,                               // 战斗服调试信息
	"showCrossInfo":                       cmdCrossInfo,                               // 跨服调试信息
	"setMailId":                           cmdSetMailSerialId,                         // 设置邮件序列号
	"SetCheckSpeed":                       cmdCheckSpeed,                              // 是否检查加速
	"openYY":                              cmdOpenYY,                                  // 开启运营活动
	"endYY":                               cmdEndYY,                                   // 关闭运营活动
	"setYYTime":                           cmdSetYYTime,                               // 设置运营活动时间
	"skipFreeVip":                         skipFreeVip,                                // 将这个等级的所有任务设为完成
	"reloadItem":                          cmdReloadItem,                              // 重新加载物品使用函数
	"playerMergeEvent":                    cmdPlayerMergeEvent,                        // 玩家个人合服事件触发
	"delPlayerItemByHandle":               delPlayerItemByHandle,                      // 删除玩家道具通过handle
	"pyy.allOpen":                         cmdAllOpenPlayerYY,                         // 玩家活动开
	"pyy.allEnd":                          cmdAllClosePlayerYY,                        // 玩家活动关
	"gift":                                cmdGiftBroadcast,                           // 假礼包播报
	"stopTopFight":                        cmdStopTopFight,                            // 停止巅峰竞技
	"robotRefresh":                        cmdRobotRefresh,                            // 设置机器人刷新 param  1 开启 2 关闭
	"setLoggerLevel":                      cmdSetLoggerLevel,                          // 设置日志等级 p1：serverType p2：level
	"cleanSmallCross":                     cmdCleanSmallCross,                         // 清理跨服匹配数据
	"stopPlayer":                          cmdStopPlayer,                              // 暂停玩家
	"loadObsData":                         cmdLoadObsData,                             // 加载obs数据
	"forbidActor":                         cmdForbidActor,                             // 禁止actor(不会踢玩家下线)
	"changeGuildNotice":                   cmdChangeGuildNotice,                       // 修改仙盟公告
	"dismissGuild":                        cmdDismissGuild,                            // 解散仙盟
	"changeRoleName":                      cmdChangeRoleName,                          // 修改角色名字
	"resetFreeVipQuest":                   cmdResetFreeVipQuest,                       // 重置踏仙途任务 param1 playerId
	"notifyPlayerUpdateSrv":               cmdNotifyPlayerUpdateSrv,                   // 广播通知玩家更新服务器 param1 level
	"beforeMerge":                         cmdGmBeforeMerge,                           // 合服前操作
	"resetFirstRecharge":                  cmdResetFirstRecharge,                      // 重置首充双倍
	"cmdOptSysQuest":                      cmdOptSysQuest,                             // 操作任务
	"dissolveTeam":                        cmdDissolveTeam,                            // 解散队伍
	"genInviteCode":                       cmdGenInviteCode,                           // 生成邀请码
	"cmdOptPYYQuest":                      cmdOptPYYQuest,                             // 操作个人运营活动任务
	"cmdReloadChatRule":                   cmdReloadChatRule,                          // 重新加载聊天规则
	"cmdInstantlySaveDB":                  cmdInstantlySaveDB,                         // 玩家数据立即入库
	"cmdClearWordMonitorCache":            cmdClearWordMonitorCache,                   // 清理敏感词监控缓存
	"cmdGMDoMatchByZone":                  cmdGMDoMatchByZone,                         // 跨服匹配-通过战区
	"cmdGMUpdateCrossTimes":               cmdGMUpdateCrossTimes,                      // 更新跨服匹配次数
	"cmdAddMileCharge":                    cmdAddMileCharge,                           // GM操作法则累充额度
	"cmdClearChargeToken":                 cmdClearChargeToken,                        // 清除代币额度
	"cmdSetSrvStatusFlag":                 cmdSetSrvStatusFlag,                        // 设置区服状态合集
	"cmdResetBossRedPack":                 cmdResetBossRedPack,                        // 重置boss红包
	"cmdForceOpenNextLuckyTreasures":      cmdResetLuckyTreasures,                     // 强制开启下一期招财进宝
	"cmdForceResetBattleArena":            cmdForceResetBattleArena,                   // 强制重新初始化竞技场机器人
	"sectTaskRefresh":                     cmdSectTaskRefresh,                         // 宗门任务刷新
	"cmdSetMoney":                         cmdDeductMoney,                             // 扣除货币
	"cmdSetSevenDayReturnMoneyDailyData":  cmdSetSevenDayReturnMoneyDailyData,         // 离线GM操作个人运营活动-七日返利
	"setMediumCross":                      cmdSetMediumCross,                          // 设置中跨服链接
	"cleanMediumCross":                    cmdCleanMediumCross,                        // 清理中跨服
	"doMediumCrossMatch":                  cmdDoMediumCrossMatch,                      // 中跨服匹配（仅空闲）
	"cmdSetMediumCrossLink":               cmdSetMediumCrossLink,                      // 中跨服设置连接
	"cmdFixNewJingJieTuPo":                cmdFixNewJingJieTuPo,                       // 修复玩家新境界雷劫次数
	"cmdHiddenSuperVip":                   cmdHiddenSuperVip,                          // 隐藏玩家超级VIP面板
	"cmdOpenProtoStat":                    cmdOpenProtoStat,                           // 开启协议流量统计
	"cmdLogUltimateDamage":                cmdLogUltimateDamage,                       // 打印极品boss伤害
	"cmdAddMayDayDegree":                  cmdAddMayDayDegree,                         // 五一热度添加
	"cmdDartCar":                          cmdDartCar,                                 // 删除镖车
	"cmdClearForBidActorIds":              cmdClearForBidActorIds,                     // 清理禁止玩家登陆列表
	"cmdAllPlayerJinYan":                  cmdAllPlayerJinYan,                         // 清理所有玩家禁言
	"cmdLoadGuildRule":                    cmdLoadGuildRule,                           // 重新加载仙盟规则
	"cmdG2FShowCrossSrvInfo":              cmdG2FShowCrossSrvInfo,                     // 打印跨服逻辑服信息
	"cmdAddActSweepDay":                   cmdOfflineAddActSweepDay,                   // 增加活动任务日常扫荡时长
	"cmdSectHuntingSysResetChallengeBoss": cmdOfflineSectHuntingSysResetChallengeBoss, // 重置仙宗狩猎BOSS
	"cmdYYSetting":                        cmdYYSetting,                               // 后台预设运营活动
	"cmdOpenSpiritPaintingSeason":         cmdOpenSpiritPaintingSeason,                // 开启灵画赛季
	"pyy.oneOpen":                         cmdOneOpenPlayerYY,                         // 指定玩家活动开
	"pyy.oneEnd":                          cmdOneClosePlayerYY,                        // 指定玩家活动关
	"cmdAddSummerSurfDiamond":             cmdAddSummerSurfDiamond,                    // 增加夏日冲浪仙玉库数量
	"cmdAddBloodlineFBTimes":              cmdAddBloodlineFBTimes,                     // 增加血脉副本次数
	"cmdChangeGuildPrefixName":            cmdChangeGuildPrefixName,                   // 仙盟齐名修改前缀
	"cmdHeavenlyPalaceResetCall":          cmdHeavenlyPalaceResetCall,                 // 重置天宫秘境召唤boss
	"cmdSetFeatherStatus":                 cmdSetFeatherStatus,                        // 设置羽翼状态
	"cmdYYCrossRank":                      cmdYYCrossRank,                             // 设置跨服分数
	"cmdCleanYYCrossRank":                 cmdCleanYYCrossRank,                        // 设置跨服分数
}

func cmdCleanYYCrossRank(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)

	iyy := yymgr.GetYYByActId(yyId)
	if sys, ok := iyy.(*yy.CrossCommonRank); ok && sys.IsOpen() {
		sys.CleanRank()
	}
}

func cmdHeavenlyPalaceResetCall(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	monId := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineReturnHeavenlyPalaceBossConsume, &pb3.CommonSt{
		U32Param: monId,
	})
}

func cmdOpenSpiritPaintingSeason(args *model.GmCmd) {
	idx := utils.AtoUint32(args.Param1)
	days := utils.AtoUint32(args.Param2)
	data := gshare.GetStaticVar()
	globalData := data.SpiritPaintingGlobalData
	globalData.CurIdx = idx
	zeroAt := time_util.GetDaysZeroTime(0)
	globalData.SeasonMap[idx] = &pb3.SpiritPaintingSeason{
		Idx:     idx,
		StartAt: zeroAt,
		EndAt:   zeroAt + 86400*days - 1,
	}
	engine.Broadcast(chatdef.CIWorld, 0, 8, 233, &pb3.S2C_8_233{
		GlobalData: globalData,
	}, 0)
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		engine.SendPlayerMessage(p.Id, gshare.OfflineOpenSpiritPaintingSeason, &pb3.CommonSt{
			U32Param: globalData.CurIdx,
			BParam:   true,
		})
		return true
	})
}

func cmdOfflineAddActSweepDay(args *model.GmCmd) {
	// 参数1： 玩家ID
	// 参数2： 增加天数
	actorId := utils.AtoUint64(args.Param1)
	day := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineAddActSweepDay, &pb3.CommonSt{
		U32Param: day,
	})
}

func cmdOfflineSectHuntingSysResetChallengeBoss(args *model.GmCmd) {
	// 参数1： 玩家ID
	// 参数2： 增加天数
	actorId := utils.AtoUint64(args.Param1)
	boss := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineSectHuntingSysResetChallengeBoss, &pb3.CommonSt{
		U32Param: boss,
	})
}

func cmdYYSetting(args *model.GmCmd) {
	//参数1:活动id
	//参数2:开启时间
	//参数3:结束时间
	//参数4:操作 (1开启,2关闭,3修改,4删除)
	//参数5:ext (后台穿透参数)
	var (
		yyId      = utils.AtoUint32(args.Param1)
		startTime = utils.AtoUint32(args.Param2)
		endTime   = utils.AtoUint32(args.Param3)
		op        = utils.AtoUint32(args.Param4)
		ext       = args.Param5
	)

	switch op {
	case gshare.YYTimerSettingOpAdd:
		cmdyysettingmgr.AddTimerSetting(&gshare.YYTimerSettingCmdOpen{
			YYId:      yyId,
			StartTime: startTime,
			EndTime:   endTime,
			Ext:       args.Param5,
		})
	case gshare.YYTimerSettingOpClose:
		cmdyysettingmgr.CloseTimerSetting(&gshare.YYTimerSettingCmdClose{
			Ext: ext,
		})
	case gshare.YYTimerSettingOpUpdate:
		cmdyysettingmgr.UpdateTimerSetting(&gshare.YYTimerSettingCmdUpdate{
			Ext:       ext,
			StartTime: startTime,
			EndTime:   endTime,
		})
	case gshare.YYTimerSettingOpDelete:
		cmdyysettingmgr.DeleteTimerSetting(&gshare.YYTimerSettingCmdDelete{
			Ext: ext,
		})
	}
}

func cmdG2FShowCrossSrvInfo(args *model.GmCmd) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FShowCrossSrvInfo, &pb3.CommonSt{})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func cmdLoadGuildRule(args *model.GmCmd) {
	logger.LogInfo("重新加载仙盟规则")
	gshare.SendRedisMsg(redismid.ReloadGuildRule)
}

func cmdAllPlayerJinYan(args *model.GmCmd) {
	nowSec := time_util.NowSec()
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		engine.SendPlayerMessage(p.Id, gshare.OfflineJinYan, &pb3.CommonSt{U32Param: nowSec})
		return true
	})
}

func cmdClearForBidActorIds(args *model.GmCmd) {
	global := gshare.GetStaticVar()
	if global.ForBidActorIds == nil {
		return
	}
	global.ForBidActorIds = make(map[uint64]bool)
}

func cmdDartCar(args *model.GmCmd) {
	if dartcarmgr.GDartCarMgrIns != nil {
		actorId := utils.AtoUint64(args.Param1)
		dartcarmgr.GDartCarMgrIns.DelDartCar(actorId)
	}
}

func cmdOpenProtoStat(args *model.GmCmd) {
	var openFlag uint32
	if len(args.Param1) > 0 {
		openFlag = utils.AtoUint32(args.Param1)
	}
	gshare.GetStaticVar().OpenProtoStat = openFlag > 0
}

func cmdHiddenSuperVip(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	open := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineHiddenSuperVip, &pb3.CommonSt{
		U32Param: open,
	})
}

func cmdFixNewJingJieTuPo(args *model.GmCmd) {
	// 参数1： 玩家ID
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineFixNewJingJieTuPo, &pb3.CommonSt{})
}
func cmdSetSevenDayReturnMoneyDailyData(args *model.GmCmd) {
	// 参数1： 玩家ID
	// 参数2： 运营活动ID
	// 参数3： 第几天
	// 参数4： 消费仙玉数量
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineGMSevenDayReturnMoneyDailyData, &pb3.CommonSt{
		U32Param:  utils.AtoUint32(args.Param2),
		U32Param2: utils.AtoUint32(args.Param3),
		I64Param:  utils.AtoInt64(args.Param4),
	})
}

func cmdResetLuckyTreasures(_ *model.GmCmd) {
	actorsystem.CmdForceOpenNextLuckyTreasures()
}

func cmdDeductMoney(args *model.GmCmd) {
	// 参数1： 玩家ID
	// 参数2： 货币类型
	// 参数3： 货币数量
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineCmdDeductMoney, &pb3.CommonSt{
		U32Param:  utils.AtoUint32(args.Param2),
		U32Param2: utils.AtoUint32(args.Param3),
		U32Param3: utils.AtoUint32(args.Param4),
	})
}

func cmdForceResetBattleArena(_ *model.GmCmd) {
	robotmgr.RobotMgrInstance.GMForceResetBattleArena()
}

func cmdSetSrvStatusFlag(args *model.GmCmd) {
	gshare.GetStaticVar().SrvStatusFlag = utils.AtoUint64(args.Param1)
	event.TriggerSysEvent(custom_id.SeRegGameSrv)
}

func cmdLoadObsData(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	version := args.Param2
	if actorId <= 0 || version == "" {
		return
	}

	meta := micro.MustMeta()
	if meta == nil {
		return
	}

	obsClient, err := obs.New(meta.HuaWeiObs.Ak, meta.HuaWeiObs.Sk, meta.HuaWeiObs.Endpoint, obs.WithPathStyle(meta.HuaWeiObs.PathStyle))
	if err != nil {
		return
	}
	defer obsClient.Close()

	bucketName := micro.MustMeta().BucketName
	input := &obs.GetObjectInput{}
	input.Bucket = bucketName
	input.Key = fmt.Sprintf("%v/%v/%v", uint32((actorId>>24)>>16), actorId, version)

	output, err := obsClient.GetObject(input)
	if err == nil {
		defer output.Body.Close()
		data, err := io.ReadAll(output.Body)
		if err != nil {
			fmt.Println(err)
			return
		}
		if actor := manager.GetPlayerPtrById(actorId); nil != actor {
			actor.ClosePlayer(uint16(cmd.DCRKick))
		}
		gshare.SendDBMsg(custom_id.GMsgSaveActorDataToCache, "", pb3.UnCompress(data))
	}

}

func cmdCleanSmallCross(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMCleanSmallCross, argsdef.CleanSmallCrossReq{
		CommonCrossSt: argsdef.CommonCrossSt{
			ZoneId:  utils.AtoUint32(args.Param1),
			CrossId: utils.AtoUint32(args.Param2),
		},
	})
	if err != nil {
		logger.LogError("set small cross error %v", err)
		return
	}
	gshare.GetStaticVar().IsActivityCross = 0
}

func cmdCleanMediumCross(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMCleanMediumCross, argsdef.CleanMediumCrossReq{
		CommonCrossSt: argsdef.CommonCrossSt{
			ZoneId:  utils.AtoUint32(args.Param1),
			CrossId: utils.AtoUint32(args.Param2),
		},
	})
	if err != nil {
		logger.LogError("set medium cross error %v", err)
		return
	}
}

func cmdDoMediumCrossMatch(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMDoMediumCrossMatchByZone, argsdef.GMDoMediumMatchByZone{
		ZoneId: utils.AtoUint32(args.Param1),
	})
	if err != nil {
		logger.LogError("set medium cross error %v", err)
		return
	}
}

func cmdSetMediumCrossLink(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMSetMediumCrossLink, argsdef.SetMediumCrossLink{
		MediumCross: argsdef.CommonMediumCrossSt{
			ZoneId:  utils.AtoUint32(args.Param1),
			CrossId: utils.AtoUint32(args.Param2),
		},
		ZoneId:   utils.AtoUint32(args.Param3),
		CrossIds: []uint32{utils.AtoUint32(args.Param4)},
	})
	if err != nil {
		logger.LogError("set medium cross error %v", err)
		return
	}
}

func cmdSetMediumCross(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMSetMediumCross, argsdef.SetMediumCross{
		CommonGameSt: argsdef.CommonGameSt{
			PfId:  engine.GetPfId(),
			SrvId: engine.GetServerId(),
		},
		CommonMediumCrossSt: argsdef.CommonMediumCrossSt{
			ZoneId:  utils.AtoUint32(args.Param1),
			CrossId: utils.AtoUint32(args.Param2),
		},
	})
	if err != nil {
		logger.LogError("set medium cross error %v", err)
	}
}

func cmdGMDoMatchByZone(args *model.GmCmd) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMDoMatchByZone, argsdef.GMDoMatchByZone{
		ZoneId:      utils.AtoUint32(args.Param1),
		TimesByRule: utils.AtoUint32(args.Param2),
	})
	if err != nil {
		logger.LogError("GMDoMatchByZone error %v", err)
		return
	}
}
func cmdGMUpdateCrossTimes(args *model.GmCmd) {
	crossIdsStr := strings.Split(args.Param2, ",")
	var crossIds []uint32
	for _, idStr := range crossIdsStr {
		crossId := utils.AtoUint32(idStr)
		crossIds = append(crossIds, crossId)
	}
	if len(crossIds) == 0 {
		logger.LogWarn("crossIds not found")
		return
	}
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMUpdateCrossTimes, argsdef.UpdateCrossTimesReq{
		ZoneId:   utils.AtoUint32(args.Param1),
		CrossIds: crossIds,
		Times:    utils.AtoUint32(args.Param3),
	})
	if err != nil {
		logger.LogError("GMDoMatchByZone error %v", err)
		return
	}
}

func cmdAddMileCharge(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)
	actorId := utils.AtoUint64(args.Param2)
	score := utils.AtoUint32(args.Param3)
	replace := utils.AtoUint32(args.Param4)
	engine.SendPlayerMessage(actorId, gshare.OfflineGMAddMileCharge, &pb3.CommonSt{
		U32Param:  yyId,
		BParam:    replace == 1,
		U32Param2: score,
	})
}

func cmdClearChargeToken(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineGMClearChargeToken, &pb3.CommonSt{})
}

func cmdOpenYY(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)
	durationDay := utils.AtoUint32(args.Param2)
	confIdx := utils.AtoUint32(args.Param3)
	sTime := time_util.NowSec()
	eTime := time_util.GetDaysZeroTime(durationDay)

	if nil != jsondata.GetYunYingConf(yyId) {
		yymgr.GmOpenYY(yyId, sTime, eTime, confIdx, true)
	} else if nil != jsondata.GetPlayerYYConf(yyId) {
		pyymgr.GmAllPlayerOpen(args.Param1, args.Param2, args.Param3)
	}
}

func cmdEndYY(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)

	if nil != jsondata.GetYunYingConf(yyId) {
		yymgr.GmEndYY(args.Param1)
	} else if nil != jsondata.GetPlayerYYConf(yyId) {
		pyymgr.GmAllPlayerEnd(args.Param1)
	}
}

func cmdSetYYTime(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)
	startDay := utils.AtoUint32(args.Param2)
	yymgr.SetYYTimeGm(yyId, startDay)
}

func cmdSetMailSerialId(args *model.GmCmd) {
	serial := utils.AtoUint32(args.Param1)
	sst := gshare.GetStaticVar()
	sst.MailSeries = serial
}

// 通知加载开服时间
func cmdLoadOpenTime(args *model.GmCmd) {
	if err := gshare.LoadOpenSrvTime(engine.GetServerId()); nil == err {
		logger.LogInfo("后台通知加载开服时间完成")
	} else {
		logger.LogError("后台通知加载开服时间出错. %v", err)
	}
}

func cmdSetSmallCross(args *model.GmCmd) {
	zoneId := utils.AtoUint32(args.Param1)
	crossId := utils.AtoUint32(args.Param2)
	camp := utils.AtoInt32(args.Param3)
	times := utils.AtoUint32(args.Param4)
	setSmallCross(zoneId, crossId, camp, times)
}

func cmdSmallCrossDoMatch(args *model.GmCmd) {
	smallCrossDoMatch()
}

// 开启pprof
func cmdSetPProf(args *model.GmCmd) {
	sSetPProf()
}

// 热更新配置
func cmdReloadJson(args *model.GmCmd) {
	gcommon.LoadJsonData(true)
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFReloadJson, &pb3.GFReloadJson{})
}

// 热更新某些配置
func cmdReloadSomeJson(args *model.GmCmd) {
	gcommon.LoadSomeJsonData(args.Param1)
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFReloadSomeJson, &pb3.GFReloadJson{
		Ids: args.Param1,
	})
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.GFReloadSomeJson, &pb3.GFReloadJson{
		Ids: args.Param1,
	})
}

// 添加公告
func cmdAddNotice(args *model.GmCmd) {
	sTime := utils.AtoUint32(args.Param3)
	eTime := utils.AtoUint32(args.Param4)
	interval := utils.AtoUint32(args.Param5)
	id := utils.AtoUint32(args.Param2)
	addNotice(args.Param1, id, sTime, eTime, interval)
}

// 删除公告
func cmdRemoveNotice(args *model.GmCmd) {
	removeNotice(args.Param1)
}

// 加载全服邮件
func cmdSrvMail(args *model.GmCmd) {
	mailmgr.LoadSrvMail()
}

// 加载玩家邮件
func cmdGetMail(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiMail).(iface.IMailSys)
	if !ok {
		return
	}
	sys.SendDbLoadMail(0)
}

// 强制删除玩家邮件
func cmdDelActorMail(args *model.GmCmd) {
	actorId, idStr := utils.AtoUint64(args.Param1), args.Param2

	ids := strings.Split(idStr, ",")
	mailIds := make([]uint64, 0, len(ids))
	for _, v := range ids {
		mailIds = append(mailIds, utils.AToU64(v))
	}

	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}

	sys, ok := actor.GetSysObj(sysdef.SiMail).(*actorsystem.MailSys)
	if !ok {
		return
	}

	sys.DeleteMailByIds(mailIds, true)
}

// 踢下线
func cmdKick(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	reason := utils.AtoUint32(args.Param2)
	if reason > 0 { // 平台封禁类型的默认都传1, 其他的都是0
		reason = cmd.DCRBan
	} else {
		reason = cmd.DCRKick
	}
	if actor := manager.GetPlayerPtrById(actorId); nil != actor {
		actor.ClosePlayer(uint16(reason))
	}
	if reason == cmd.DCRBan {
		event.TriggerSysEvent(custom_id.SeClearActorChat, &pb3.CommonSt{
			U64Param: actorId,
		})
		engine.Broadcast(chatdef.CIWorld, 0, 5, 12, &pb3.S2C_5_12{
			ActorId: actorId,
		}, 0)
	}
}

func cmdStopPlayer(args *model.GmCmd) {
	// 踢玩家下线
	playerId := utils.AtoUint64(args.Param1)
	if player := manager.GetPlayerPtrById(playerId); nil != player {
		player.ClosePlayer(uint16(cmd.DCRKick)) // 这里不会存入actorCaches
		player.Save(false)
	} else {

	}

	// 禁止登录
	gmCmd := &model.GmCmd{
		Param1: strconv.Itoa(int(playerId)),
		Param2: "1",
	}
	cmdForbidActor(gmCmd)

	// 立即存库
	gshare.SendDBMsg(custom_id.GMsgInstantlySaveDB, playerId)
}

// 玩家数据立即存库
func cmdInstantlySaveDB(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	data := manager.GetOfflineData(playerId, gshare.ActorDataBase)
	if data == nil {
		logger.LogError("not found %d player offline data", playerId)
		return
	}
	player := manager.GetPlayerPtrById(playerId)
	if player != nil {
		player.Save(false)
	}
	// 立即存库
	gshare.SendDBMsg(custom_id.GMsgInstantlySaveDB, playerId)
}

// 禁言
func cmdJinYan(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	duration := utils.AtoUint32(args.Param2)
	var time uint32
	if duration == 0 {
		time = 0
	} else {
		time = time_util.NowSec() + duration*60
	}
	engine.SendPlayerMessage(actorId, gshare.OfflineJinYan, &pb3.CommonSt{U32Param: time})
	gshare.SendPlayerStaticsMsg(actorId, map[string]interface{}{"jy_timestamp": time})
}

func offlineJinYan(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	time := st.U32Param
	player.GetBinaryData().JinYanTime = time
}

func offlineChangeRoleName(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	newName := st.StrParam
	if !engine.CheckNameRepeat(newName) {
		logger.LogError("role name:%s is exit", newName)
		return
	}
	actorsystem.DoChangeName(player, newName)
}

func offlineHiddenSuperVip(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	player.GetBinaryData().HiddenSuperVip = st.U32Param > 0
	player.SendProto3(2, 201, &pb3.S2C_2_201{
		HiddenSuperVip: player.GetBinaryData().HiddenSuperVip,
	})
}

// 回滚玩家数据
func cmdLoadCache(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	fileName := args.Param2

	gshare.SendDBMsg(custom_id.GMsgLoadActorCache, actorId, fileName)
}

func setSmallCross(zoneId, crossId uint32, camp int32, times uint32) {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMSetCross, argsdef.SetSmallCross{
		CommonGameSt: argsdef.CommonGameSt{
			PfId:  engine.GetPfId(),
			SrvId: engine.GetServerId(),
		},
		CommonCrossSt: argsdef.CommonCrossSt{
			ZoneId:  zoneId,
			CrossId: crossId,
		},
		Camp:  camp,
		Times: times,
	})
	if err != nil {
		logger.LogError("set small cross error %v", err)
	}
}

func smallCrossDoMatch() {
	err := crossservice.PubJsonNats(pb3.CrossServiceNatsOpCode_GMDoMatch, nil)
	if err != nil {
		logger.LogError("small cross do match error %v", err)
	}
}

// 恢复角色
func cmdRecoveryPlayer(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	userId := utils.AtoUint32(args.Param2)
	recoveryPlayer(playerId, userId)
}

func cmdAllOpenPlayerYY(args *model.GmCmd) {
	pyymgr.GmAllPlayerOpen(args.Param1, args.Param2, args.Param3)
}
func cmdAllClosePlayerYY(args *model.GmCmd) {
	pyymgr.GmAllPlayerEnd(args.Param1)
}
func cmdOneOpenPlayerYY(args *model.GmCmd) {
	pyymgr.GmPlayerOpen(args.Param1, args.Param2, args.Param3, args.Param4)
}
func cmdOneClosePlayerYY(args *model.GmCmd) {
	pyymgr.GmPlayerEnd(args.Param1, args.Param2)
}

func recoveryPlayer(playerId uint64, userId uint32) {
	gshare.SendDBMsg(custom_id.GMsgRecoveryPlayer, playerId, userId)
}

func cmdKickAllPlayer(args *model.GmCmd) {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.ClosePlayer(cmd.DCRMaintenance)
	})
}

func cmdSetDisableLoginFlag(args *model.GmCmd) {
	num := utils.AtoInt(args.Param1)
	if num > 0 {
		engine.SetDisableLogin(true)
	} else {
		engine.SetDisableLogin(false)
	}
}

// 设置屏蔽协议
func cmdForbidProto(args *model.GmCmd) {
	protoIdH := utils.Atoi(args.Param1)
	protoIdL := utils.Atoi(args.Param2)

	disableproto.SetDisableProto(uint32(protoIdH<<8 | protoIdL))
}

// 把actorcache的dirty全部都设置为true
func cmdDirtyCache(args *model.GmCmd) {
	gshare.SendDBMsg(custom_id.GMsgDirtyActorCache)
}

// 充值事件
func cmdChargeEvent(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	diamond := utils.AtoInt64(args.Param2)
	chargeId := utils.AtoInt64(args.Param3)
	player := manager.GetPlayerPtrById(playerId)
	if nil == player {
		return
	}
	player.TriggerEvent(custom_id.AeCharge, &custom_id.ActorEventCharge{
		Diamond:  uint32(diamond),
		CashCent: uint32(diamond),
		LogId:    pb3.LogId_LogChargeEvent,
		ChargeId: uint32(chargeId),
	})
}

func cmdResetFreeVipQuest(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	player := manager.GetPlayerPtrById(playerId)
	if nil == player {
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiFreeVip).(*actorsystem.FreeVipSys)
	if !ok {
		return
	}
	sys.ResetQuest()
	sys.S2cFreeVipState()
}

func cmdGmBeforeMerge(args *model.GmCmd) {
	// 踢玩家下线
	cmdNotifyPlayerUpdateSrv(&model.GmCmd{Param1: fmt.Sprintf("%d", 1)})
	event.TriggerSysEvent(custom_id.SeCmdGmBeforeMerge)
}

func cmdMerge(args *model.GmCmd) {
	srvList := utils.GetUint32SliceFromString(args.Param1)
	merge.GlobalVar(srvList)
}

func cmdMergeDay(args *model.GmCmd) {
	setMergeDay(utils.AtoUint32(args.Param1))
}

func cmdMergeTimes(args *model.GmCmd) {
	setMergeTimes(utils.AtoUint32(args.Param1))
}

// 合服后被删除的玩家列表返回
func onDeleteActorIdsRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onDeleteActorIdsRet", 1, len(args)) {
		return
	}

	actorIds := args[0].([]uint64)
	if len(actorIds) <= 0 {
		return
	}

	idMap := make(map[uint64]struct{})
	for _, id := range actorIds {
		idMap[id] = struct{}{}
	}
	manager.GRankMgrIns.DeleteFromRank(idMap)
	friendmgr.DeleteDelPlayerData(idMap)
	pyymgr.DelDataByPlayerIds(idMap)
}

// 设置合服次数，天数重置为1
func setMergeTimes(times uint32) {
	sst := gshare.GetStaticVar()
	sst.MergeTimes = times
	nowSec := time_util.NowSec()

	if times == 0 {
		sst.MergeTimestamp = 0
	} else {
		sst.MergeTimestamp = nowSec
	}

	if sst.MergeData == nil {
		sst.MergeData = make(map[uint32]uint32)
	}
	if times > 0 {
		sst.MergeData[times] = nowSec
	}

	event.TriggerSysEvent(custom_id.SeMerge)
	logger.LogDebug("设置合服次数:%d", times)
}

// 设置合服天数
func setMergeDay(day uint32) {
	var beginTime uint32 = 0
	if day > 0 {
		beginTime = time_util.GetZeroTime(time_util.NowSec()) // 今天0点
		beginTime -= uint32(day-1) * gshare.DAY_SECOND
	}

	sst := gshare.GetStaticVar()
	sst.MergeTimestamp = beginTime
	if sst.MergeData == nil {
		sst.MergeData = make(map[uint32]uint32)
	}
	sst.MergeData[sst.MergeTimes] = beginTime
	event.TriggerSysEvent(custom_id.SeMerge)
}

// 取消屏蔽协议
func cmdUnFroBidProto(args *model.GmCmd) {
	protoIdH := utils.Atoi(args.Param1)
	protoIdL := utils.Atoi(args.Param2)

	disableproto.DelDisableProto(uint32(protoIdH<<8 | protoIdL))
}

func cmdForbidCreatePlayer(args *model.GmCmd) {
	param1 := utils.Atoi(args.Param1)

	var flag bool
	if param1 > 0 {
		flag = true
	}

	engine.SetForbidCreatePlayer(flag)
}

func cmdGiftBroadcast(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	itemId := utils.AtoUint32(args.Param2)
	if actor := manager.GetPlayerPtrById(actorId); nil != actor {
		engine.CrossBroadcastTipMsgById(tipmsgid.GiftTips, actor.GetId(), actor.GetName(), itemId)
	}
}

func cmdClearChatMsg(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	actorsystem.ClearWorldChat(playerId)
}

// 完成玩家主线任务
func cmdFinishMainQuest(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}
	sys, ok := actor.GetSysObj(sysdef.SiQuest).(*actorsystem.QuestSys)
	if !ok {
		return
	}
	if quest := sys.GetCurMainTask(); nil != quest {
		sys.Finish(argsdef.NewFinishQuestParams(quest.GetId(), true, false, true))
	}
}

func cmdDelTitle(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	titleId := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineDisActiveTitle, &pb3.OfflineTitle{
		TitleId: titleId,
	})
}

func cmdOpenAct(args *model.GmCmd) {

	actId := utils.AtoUint32(args.Param1)
	lastTime := utils.AtoUint32(args.Param2)
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FGmOpenAct, &pb3.CommonSt{
		U32Param:  actId,
		U32Param2: lastTime,
	})
}

func cmdOpenCrossAct(args *model.GmCmd) {
	actId := utils.AtoUint32(args.Param1)
	lastTime := utils.AtoUint32(args.Param2)
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FGmOpenAct, &pb3.CommonSt{
		U32Param:  actId,
		U32Param2: lastTime,
	})
}

func cmdCloseAct(args *model.GmCmd) {
	actId := utils.AtoUint32(args.Param1)
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FGmCloseAct, &pb3.CommonSt{
		U32Param: actId,
	})
}

func cmdCloseCrossAct(args *model.GmCmd) {
	actId := utils.AtoUint32(args.Param1)
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FGmCloseAct, &pb3.CommonSt{
		U32Param: actId,
	})
}

func cmdFightInfo(args *model.GmCmd) {
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FShowDebugInfo, &pb3.CommonSt{U32Param: utils.AtoUint32(args.Param1)})
}

func cmdCrossInfo(args *model.GmCmd) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FShowDebugInfo, &pb3.CommonSt{U32Param: utils.AtoUint32(args.Param1)})
}

func cmdCheckSpeed(args *model.GmCmd) {
	param1 := utils.Atoi(args.Param1)
	var isClose bool
	if param1 <= 0 {
		isClose = true
	}

	// 是否关闭检测
	gmflag.GetGmCmdData().CloseCheckSpeed = isClose
}

func loadGmCmd(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadGmCmd", 1, len(args)) {
		return
	}
	st, ok := args[0].(*model.GmCmd)
	if !ok {
		logger.LogError("loadGmCmd args Error!!!")
		return
	}
	logger.LogDebug("loadGmCmd %s, %s %s %s %s %s", st.Cmd, st.Param1, st.Param2, st.Param3, st.Param4, st.Param5)
	if cb, ok := handleMap[st.Cmd]; ok {
		cb(st)
	} else {
		logger.LogError("cannot handle command[%s]", st.Cmd)
	}
}

func cmdReloadItem(args *model.GmCmd) {
	miscitem.ReloadItemFunc()
}

func cmdPlayerMergeEvent(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflinePlayerMergeEvent, nil)
}

func delPlayerItemByHandle(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	itemHandle := utils.AtoUint64(args.Param2)
	itemId := utils.AtoUint32(args.Param3)

	player := manager.GetPlayerPtrById(playerId)
	if nil != player {
		if itemHandle > 0 {
			player.DelItemByHand(itemHandle, pb3.LogId_LogGm)
		}
		if itemId > 0 {
			player.DeleteItemById(itemId, player.GetItemCount(itemId, -1), pb3.LogId_LogGm)
		}
	} else {
		engine.SendPlayerMessage(playerId, gshare.OfflineDeleteItemByHand, &pb3.CommonSt{
			U64Param: itemHandle,
			U32Param: itemId,
		})
	}
}

func cmdStopTopFight(args *model.GmCmd) {
	flag := utils.AtoUint32(args.Param1)
	if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CStopTopFight, &pb3.CommonSt{
		BParam: flag == 1,
	}); nil != err {
		logger.LogError("cmd stopTopFight err:%v", err)
	}
}

func skipFreeVip(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineSkipFreeVip, &pb3.CommonSt{})
}

func cmdRobotRefresh(args *model.GmCmd) {
	global := gshare.GetStaticVar()

	var status bool
	if utils.AtoInt(args.Param1) > 0 {
		global.RobotRefreshStatus = "true"
		status = true
	} else {
		global.RobotRefreshStatus = "false"
		status = false
	}

	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFRobotRefreshStatus, &pb3.SyncRobotRefresh{Status: status})
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

func cmdForbidActor(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	flag := utils.AtoInt(args.Param2)
	if actorId <= 0 {
		return
	}
	global := gshare.GetStaticVar()
	if global.ForBidActorIds == nil {
		global.ForBidActorIds = make(map[uint64]bool)
	}

	if flag == 1 {
		global.ForBidActorIds[actorId] = true
		return
	}

	delete(global.ForBidActorIds, actorId)
}

func cmdSetLoggerLevel(args *model.GmCmd) {
	serverType := utils.AtoUint32(args.Param1)
	level := utils.Atoi(args.Param2)
	if serverType == uint32(base.GameServer) {
		logger.SetLevel(level)
	} else if serverType == uint32(base.LocalFightServer) {
		engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FGmSetLoggerLevel, &pb3.CommonSt{
			U32Param: uint32(level),
		})
	} else {
		logger.LogError("set logger level serveType= %d, level=%d", serverType, level)
	}
}

func cmdChangeGuildNotice(args *model.GmCmd) {
	guildId := utils.AtoUint64(args.Param1)
	notice := args.Param2
	flag := utils.AtoUint32(args.Param3)

	guild := guildmgr.GetGuildById(guildId)
	if nil == guild {
		return
	}
	guild.BasicInfo.Notice = notice
	if flag > 0 {
		guild.Binary.NoticeFlag = true
	} else {
		guild.Binary.NoticeFlag = false
	}
	guild.BroadcastProto(29, 116, &pb3.S2C_29_116{Notice: notice})
}

func cmdDismissGuild(args *model.GmCmd) {
	guildId := utils.AtoUint64(args.Param1)
	guild := guildmgr.GetGuildById(guildId)
	if nil == guild {
		return
	}

	for actorId := range guild.Members {
		guild.RemoveMember(actorId)
	}
	guildmgr.DelGuild(guildId)
}

func cmdChangeRoleName(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	newName := args.Param2
	if actorId == 0 || newName == "" {
		return
	}
	engine.SendPlayerMessage(actorId, gshare.OfflineChangeRoleName, &pb3.CommonSt{
		StrParam: newName,
	})
}

func cmdNotifyPlayerUpdateSrv(args *model.GmCmd) {
	var isEmergency uint32
	if len(args.Param1) != 0 {
		isEmergency = utils.AtoUint32(args.Param1)
	}
	engine.Broadcast(chatdef.CIWorld, 0, 1, 13, &pb3.S2C_1_13{IsEmergency: isEmergency}, 0)
}

func cmdResetFirstRecharge(args *model.GmCmd) {
	gshare.SendGameMsg(custom_id.GMsgResetFirstRecharge)
}

func cmdOptSysQuest(args *model.GmCmd) {
	sysId := utils.AtoUint32(args.Param1)
	opt := utils.AtoUint32(args.Param2)
	idsString := args.Param3

	var sendMessageToActor = func(actorId uint64) {
		var commonSt = &pb3.CommonSt{}
		commonSt.U32Param = sysId
		commonSt.U32Param2 = opt
		commonSt.StrParam = idsString
		engine.SendPlayerMessage(actorId, gshare.OfflineGMReAcceptQuest, commonSt)
	}
	if args.Param4 != "" {
		actorIdsStr := strings.Split(args.Param4, ",")
		for _, actorIdStr := range actorIdsStr {
			actorId := utils.AtoUint64(actorIdStr)
			sendMessageToActor(actorId)
		}
		return
	}
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		sendMessageToActor(p.Id)
		return true
	})
}

func cmdOptPYYQuest(args *model.GmCmd) {
	pyyClassId := utils.AtoUint32(args.Param1)
	opt := utils.AtoUint32(args.Param2)
	idsString := args.Param3

	var sendMessageToActor = func(actorId uint64) {
		var commonSt = &pb3.CommonSt{}
		commonSt.U32Param = pyyClassId
		commonSt.U32Param2 = opt
		commonSt.StrParam = idsString
		engine.SendPlayerMessage(actorId, gshare.OfflineGMReAcceptPYYQuest, commonSt)
	}
	if args.Param4 != "" {
		actorIdsStr := strings.Split(args.Param4, ",")
		for _, actorIdStr := range actorIdsStr {
			actorId := utils.AtoUint64(actorIdStr)
			sendMessageToActor(actorId)
		}
		return
	}
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		sendMessageToActor(p.Id)
		return true
	})
}

func cmdDissolveTeam(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	teamId := player.GetTeamId()
	err := teammgr.DissolveTeam(teamId)
	if err != nil {
		logger.LogError("err: %v", err)
	}
}

func cmdGenInviteCode(args *model.GmCmd) {
	codeNum := utils.AtoUint32(args.Param1)
	invitecodemgr.GenerateCodeGroup(codeNum)
}

func cmdReloadChatRule(_ *model.GmCmd) {
	manager.LoadChatRule()
}

func cmdClearWordMonitorCache(_ *model.GmCmd) {
	gshare.SendSDkMsg(custom_id.GMsgWordMonitor, &wordmonitor.Word{Type: wordmonitor.ClearCache})
}

func cmdResetBossRedPack(args *model.GmCmd) {
	playerId := utils.AtoUint64(args.Param1)
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	if sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*actorsystem.BossRedPackSys); ok {
		sys.RefreshDropTimes()
	}
}

func cmdSectTaskRefresh(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	engine.SendPlayerMessage(actorId, gshare.OfflineGMSectTaskRefresh, &pb3.CommonSt{
		U64Param: actorId,
	})
}

func cmdLogUltimateDamage(args *model.GmCmd) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CLogUltimateDamage, &pb3.CommonSt{
		I64Param: utils.AtoInt64(args.Param1),
	})
}

func cmdAddMayDayDegree(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)
	score := utils.AtoUint32(args.Param2)
	iYY := yymgr.GetYYByActId(yyId)
	if sys, ok := iYY.(*yy.YYMayDayDegreeMgr); ok && sys.IsOpen() {
		sys.AddDegree(score)
	}
}

func cmdAddSummerSurfDiamond(args *model.GmCmd) {
	yyId := utils.AtoUint32(args.Param1)
	score := utils.AtoUint32(args.Param2)
	iYY := yymgr.GetYYByActId(yyId)
	if sys, ok := iYY.(*yy.YYSummerSurfDiamond); ok && sys.IsOpen() {
		sys.PutBountyPool(int64(score))
	}
}

func cmdChangeGuildPrefixName(args *model.GmCmd) {
	guildId := utils.AtoUint64(args.Param1)
	name := args.Param2

	allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
	for _, iYY := range allYY {
		if iYY.IsOpen() {
			if sys, ok := iYY.(*yy.YYGuildChangeName); ok {
				data := sys.GetData()
				gData, ok := data.GuildData[guildId]
				if !ok {
					gData := &pb3.YYGuildPrefixNameData{
						GuildId: guildId,
					}
					data.GuildData[guildId] = gData
				}
				gData.PrefixName = name
				manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
					player.SendProto3(127, 180, &pb3.S2C_127_180{
						ActiveId:   sys.Id,
						GuildData:  data.GuildData[player.GetGuildId()],
						PlayerData: data.PlayerData[player.GetId()],
					})
				})
			}
		}
	}
}

func cmdAddBloodlineFBTimes(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	recoverTimes := utils.AtoUint32(args.Param2)
	engine.SendPlayerMessage(actorId, gshare.OfflineAddBloodlineFBTimes, &pb3.CommonSt{
		U32Param: recoverTimes,
		U64Param: actorId,
	})
}

func cmdSetFeatherStatus(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	featherId := utils.AtoUint32(args.Param2)

	engine.SendPlayerMessage(actorId, gshare.OfflineSetFeatherStatus, &pb3.CommonSt{
		U32Param: featherId,
		U64Param: actorId,
	})
}

func cmdYYCrossRank(args *model.GmCmd) {
	actorId := utils.AtoUint64(args.Param1)
	yyId := utils.AtoUint32(args.Param2)
	score := utils.AtoUint32(args.Param3)

	iyy := yymgr.GetYYByActId(yyId)
	if sys, ok := iyy.(*yy.CrossCommonRank); ok && sys.IsOpen() {
		sys.SetScore(actorId, int64(score))
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadGmCmdRet, loadGmCmd)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadDeletaActorIds, onDeleteActorIdsRet)
	})

	engine.RegisterMessage(gshare.OfflineJinYan, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineJinYan)

	engine.RegisterMessage(gshare.OfflineChangeRoleName, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineChangeRoleName)

	engine.RegisterMessage(gshare.OfflineHiddenSuperVip, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineHiddenSuperVip)

	gmevent.Register("setSmallCross", func(player iface.IPlayer, args ...string) bool {
		var times uint32
		if len(args) >= 4 {
			times = utils.AtoUint32(args[3])
		}
		if times == 0 {
			times = 1
		}
		setSmallCross(utils.AtoUint32(args[0]), utils.AtoUint32(args[1]), utils.AtoInt32(args[2]), times)
		return true
	}, 1)

	gmevent.Register("setMediumCross", func(player iface.IPlayer, args ...string) bool {
		cmdSetMediumCross(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)

	gmevent.Register("doMediumCrossMatch", func(player iface.IPlayer, args ...string) bool {
		cmdDoMediumCrossMatch(&model.GmCmd{
			Param1: args[0],
		})
		return true
	}, 1)

	gmevent.Register("smallCrossDoMatch", func(player iface.IPlayer, args ...string) bool {
		smallCrossDoMatch()
		return true
	}, 1)

	gmevent.Register("setmergetimes", func(actor iface.IPlayer, args ...string) bool {
		setMergeTimes(utils.AtoUint32(args[0]))
		return true
	}, 1)

	gmevent.Register("setMerge", func(actor iface.IPlayer, args ...string) bool {
		var srvList []uint32
		for _, arg := range args {
			srvList = append(srvList, utils.AtoUint32(arg))
		}
		if len(args) == 0 {
			return false
		}
		merge.GlobalVar(srvList)
		return true
	}, 1)

	gmevent.Register("setmergeday", func(actor iface.IPlayer, args ...string) bool {
		setMergeDay(utils.AtoUint32(args[0]))
		return true
	}, 1)

	gmevent.Register("kickAll", func(player iface.IPlayer, args ...string) bool {
		cmdKickAllPlayer(nil)
		return true
	}, 1)

	gmevent.Register("disableLogin", func(player iface.IPlayer, args ...string) bool {
		cmdSetDisableLoginFlag(&model.GmCmd{Param1: args[0]})
		return true
	}, 1)

	gmevent.Register("forbidProto", func(player iface.IPlayer, args ...string) bool {
		cmdForbidProto(&model.GmCmd{Param1: args[0], Param2: args[1]})
		return true
	}, 1)

	gmevent.Register("unFroBidProto", func(player iface.IPlayer, args ...string) bool {
		cmdUnFroBidProto(&model.GmCmd{Param1: args[0], Param2: args[1]})
		return true
	}, 1)

	gmevent.Register("forbidCreatePlayer", func(player iface.IPlayer, args ...string) bool {
		cmdForbidCreatePlayer(&model.GmCmd{Param1: args[0]})
		return true
	}, 1)

	gmevent.Register("mailId", func(player iface.IPlayer, args ...string) bool {
		gshare.GetStaticVar().MailSeries = utils.AtoUint32(args[0])
		return true
	}, 1)

	gmevent.Register("peopleWantedOnEnd", func(player iface.IPlayer, args ...string) bool {
		mgr := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		peopleWanted := mgr.GetObjById(760001).(*pyy.WholePeopleWanted)
		peopleWanted.OnEnd()
		return true
	}, 1)

	gmevent.Register("BenefitPrayC2sAward", func(player iface.IPlayer, args ...string) bool {
		id := utils.AtoUint32(args[0])
		msg := base.NewMessage()
		msg.SetCmd(41<<8 | 6)
		err := msg.PackPb3Msg(&pb3.C2S_41_6{
			Id: id,
		})
		if err != nil {
			logger.LogError(err.Error())
		}
		manager.GetPlayerPtrById(player.GetId()).DoNetMsg(41, 6, msg)
		return true
	}, 1)

	gmevent.Register("robotRefresh.gm", func(player iface.IPlayer, args ...string) bool {
		var param1 = ""
		if len(args) > 0 {
			param1 = "1"
		}
		cmdRobotRefresh(&model.GmCmd{
			Param1: param1,
		})
		return true
	}, 1)

	gmevent.Register("setGLogLv", func(player iface.IPlayer, args ...string) bool {
		level := 0
		if len(args) > 0 {
			level = utils.Atoi(args[0])
		}
		logger.SetLevel(level)
		return true
	}, 1)

	gmevent.Register("forbidActor", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		actorId := utils.AtoUint64(args[0])
		cmdForbidActor(&model.GmCmd{
			Param1: strconv.Itoa(int(actorId)),
		})
		return true
	}, 1)

	gmevent.Register("stopPlayer", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		actorId := utils.AtoUint64(args[0])
		cmdStopPlayer(&model.GmCmd{
			Param1: strconv.Itoa(int(actorId)),
		})
		return true
	}, 1)
	gmevent.Register("loadObs", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		cmdLoadObsData(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)
	gmevent.Register("showFightInfo", func(player iface.IPlayer, args ...string) bool {
		cmdFightInfo(&model.GmCmd{
			Param1: args[0],
		})
		return true
	}, 1)
	gmevent.Register("cleanSmallCross", func(player iface.IPlayer, args ...string) bool {
		cmdCleanSmallCross(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)
	gmevent.Register("cleanMediumCross", func(player iface.IPlayer, args ...string) bool {
		cmdCleanMediumCross(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)
	gmevent.Register("cmdGMDoMatchByZone", func(player iface.IPlayer, args ...string) bool {
		if len(args) != 2 {
			return false
		}
		cmdGMDoMatchByZone(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)
	gmevent.Register("cmdSetMediumCrossLink", func(player iface.IPlayer, args ...string) bool {
		if len(args) != 4 {
			return false
		}
		cmdSetMediumCrossLink(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
			Param3: args[2],
			Param4: args[3],
		})
		return true
	}, 1)
	gmevent.Register("notifyPlayerUpdateSrv", func(player iface.IPlayer, args ...string) bool {
		cmdNotifyPlayerUpdateSrv(&model.GmCmd{
			Param1: args[0],
		})
		return true
	}, 1)
	gmevent.Register("beforeMerge", func(player iface.IPlayer, args ...string) bool {
		cmdGmBeforeMerge(&model.GmCmd{})
		return true
	}, 1)
	gmevent.Register("genMergeSql", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 3 {
			return false
		}
		var pfStr = args[0]
		var master = utils.Atoi(args[1])
		var slaveList []int
		for i := 2; i < len(args); i++ {
			slave := utils.Atoi(args[i])
			slaveList = append(slaveList, slave)
		}
		merge.GenSqlTemp(pfStr, master, slaveList)
		return true
	}, 1)

	gmevent.Register("cross.act.start", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		cmdOpenCrossAct(&model.GmCmd{
			Param1: args[0],
			Param2: args[1],
		})
		return true
	}, 1)

	gmevent.Register("cross.act.close", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		cmdCloseCrossAct(&model.GmCmd{
			Param1: args[0],
		})
		return true
	}, 1)

	gmevent.Register("cmdLogUltimateDamage", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		cmdLogUltimateDamage(&model.GmCmd{Param1: args[0]})
		return true
	}, 1)

	gmevent.Register("cmdYYSetting", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 5 {
			return false
		}
		var (
			yyId      = args[0]
			startTime = args[1]
			endTime   = args[2]
			op        = args[3]
			ext       = args[4]
		)

		cmdYYSetting(&model.GmCmd{
			Param1: yyId,
			Param2: startTime,
			Param3: endTime,
			Param4: op,
			Param5: ext,
		})
		return true
	}, 1)
	gmevent.Register("SpiritPaintingSys.setOpen", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		cmdOpenSpiritPaintingSeason(&model.GmCmd{Param1: args[0], Param2: args[1]})
		return true
	}, 1)
}
