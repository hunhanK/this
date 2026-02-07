package main

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gateworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/entity"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
)

func OnBootData(args ...interface{}) {
	logger.LogInfo("【登录流程】玩家数据返回")
	if !gcommon.CheckArgsCount("OnBootData", 3, len(args)) {
		logger.LogError("--------- args params != 3")
		return
	}

	actorId, ok := args[0].(uint64)
	if !ok {
		logger.LogError("收到不合理的启动数据返回")
		return
	}
	flag, ok := args[1].(bool)
	if !ok {
		logger.LogError("收到不合理的启动数据返回2")
		return
	}
	obj, ok := gateworker.BootMap[actorId]
	if ok {
		delete(gateworker.BootMap, actorId)
	}
	logger.LogInfo("【登录流程】玩家数据返回 actorId:%d flag:%v, obj:%v", actorId, flag, obj)
	if !flag { // 加载失败
		logger.LogError("func OnBootData 41 flag == %v", flag)
		return
	}

	actorData, ok := args[2].(*pb3.PlayerData)
	if !ok {
		logger.LogError("数据模块返回的玩家数据类型不对")
		return
	}

	if ok {
		logger.LogInfo("【登录流程】返回数据没问题，准备创建角色 actorId:%d", actorId)
		CreateActor(obj, actorData)
	}
}

func CreateActor(obj *gateworker.BootObj, actorData *pb3.PlayerData) {
	logger.LogInfo("【登录流程】CreateActor")
	if nil == obj {
		logger.LogDebug("----------------------obj is nil")
		return
	}
	tmp := gateworker.GetGateConn(obj.GateId)
	if nil == tmp {
		logger.LogError("Get gateworker id : %v conn nil!!!!!!!!!!!!", obj.GateId)
		return
	}
	gateUser := tmp.GetUser(obj.ConnId)
	if nil == gateUser || gateUser.ActorId != actorData.GetActorId() {
		// if nil == gateUser {
		logger.LogError("boot actorId:%d but lost GateUser", obj.ActorId)
		return
	}

	logger.LogInfo("【登录流程】创建actor id:%d, taAccountId:%s, taDistinctId:%s, registeTime:%d", actorData.ActorId, obj.TAAccountId, obj.TADistinctId, obj.RegisteTime)
	actor := entity.NewPlayer(actorData)
	actor.SetAccountName(obj.AccountName)

	actor.InitBootData(obj.ActorId, obj.GateId, obj.ConnId, obj.UserId, obj.GmLevel, obj.RemoteAddr)
	manager.OnPlayerLogin(actor)
	logger.LogInfo("【登录流程】将玩家放入entitymgr, 并开始InitCreateData()初始化数据")
	if !actor.InitCreateData() {
		//关闭角色
		logger.LogError("todo 关闭角色")
		return
	}

	logger.LogInfo("【登录流程】初始化数据完成")

	// 角色同名邮件
	if utils.IsSetBit(obj.Status, 3) {
		logger.LogDebug("角色同名，发放改名卡 %s", actor.GetName())
		obj.Status = utils.ClearBit(obj.Status, 3)
		gshare.SendDBMsg(custom_id.GMsgUpdateActorStatus, obj.ActorId, obj.Status)
		mailmgr.SendMailToActor(obj.ActorId, &mailargs.SendMailSt{
			ConfId: common.Mail_RoleChangeName,
			Rewards: jsondata.StdRewardVec{
				{Id: jsondata.GlobalUint("roleChangeNameId"), Count: 1, Bind: true},
			},
		})
	}

	// 公会同名邮件，会长才发
	if utils.IsSetBit(obj.Status, 4) {
		logger.LogDebug("行会同名，给会长发放改名卡 %s", actor.GetName())
		obj.Status = utils.ClearBit(obj.Status, 4)
		gshare.SendDBMsg(custom_id.GMsgUpdateActorStatus, obj.ActorId, obj.Status)
		mailmgr.SendMailToActor(obj.ActorId, &mailargs.SendMailSt{
			ConfId: common.Mail_GuildChangeName,
			Rewards: jsondata.StdRewardVec{
				{Id: jsondata.GlobalUint("guildChangeNameId"), Count: 1, Bind: true},
			},
		})
	}

	actor.EnterGame()
}

func OnCloseGateUser(user iface.IGateUser, reason uint16) {
	if nil == user {
		return
	}

	playerId := user.GetPlayerId()
	if playerId > 0 {
		if player := manager.GetPlayerPtrById(playerId); nil != player {
			player.SetLost(true)
			player.SendProto3(1, 8, &pb3.S2C_1_8{
				Reason: uint32(reason),
			})
			//player.ClosePlayer(reason)
		} else {
			delete(gateworker.BootMap, playerId)
		}
	} else {
		//在选角界面被顶了
		user.SendProto3(1, 8, &pb3.S2C_1_8{
			Reason: uint32(reason),
		})
	}
	user.Reset()
}

func init() {
	engine.CloseGateUser = OnCloseGateUser
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadActorDataRet, OnBootData)
	})
}
