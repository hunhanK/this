package dbworker

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"strings"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func verifyAccount(args ...interface{}) {
	if !gcommon.CheckArgsCount("verifyAccount", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.VerifyAccount)
	if !ok {
		return
	}
	account, token := data.Account, data.Token

	retData := mysql.VerifyRet{
		GateId:   data.GateId,
		ConnId:   data.ConnId,
		Account:  data.Account,
		ServerId: data.ServerId,
	}

	rets, err := db.OrmEngine.QueryString("call checkUserValid(?)", account)
	if nil != err {
		retData.Errno = base.CheckTokenFailed
		verifyAccountRet(retData)
		logger.LogError("checkUserValid error!!! %v", err)
		return
	}
	if len(rets) <= 0 {
		retData.Errno = base.CheckTokenFailed
		verifyAccountRet(retData)
		logger.LogError("验证失败, 未找到对应的账号信息！account:%s", account)
		return
	}

	ret := rets[0]

	userId := utils.AtoUint32(ret["user_id"])
	if userId <= 0 {
		retData.Errno = base.CheckTokenFailed
		verifyAccountRet(retData)
		logger.LogError("验证失败，userId数据错误。 account:%s", account)
		return
	}

	pwd := ret["passwd"]

	uptime := utils.AtoUint32(ret["uptime"])
	pwtime := utils.AtoUint32(ret["pwtime"])
	isinvite := utils.AtoUint32(ret["isinvite"])

	if pwtime != 0 && uptime > pwtime {
		retData.Errno = base.UseOldToken
		verifyAccountRet(retData)
		logger.LogError("%s use old password, pwtime small than uptime", account)
		return
	}
	if !strings.EqualFold(pwd, token) {
		retData.Errno = base.TokenUnMatch
		verifyAccountRet(retData)
		logger.LogError("%s diff password, db=%s,got=%s", account, pwd, token)
		return
	}

	retData.UserId = userId
	retData.IsInvite = isinvite == 1
	retData.GmLevel = utils.AtoUint32(ret["gmlevel"])
	verifyAccountRet(retData)
	logger.LogInfo("%s password ok,accountID =%d", token, userId)
}

func verifyAccountRet(ret mysql.VerifyRet) {
	gshare.SendGameMsg(custom_id.GMsgVerifyAccountRet, ret)
}

func loadPlayerList(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadPlayerList", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.LoadPlayerList)
	if !ok {
		return
	}
	err := db.OrmEngine.SQL("call loadactorlistbyuserid(?,?)", data.UserId, data.ServerId).Find(&data.Actors)
	if nil != err {
		logger.LogError("加载玩家角色列表出错:%s", err)
		return
	}
	gshare.SendGameMsg(custom_id.GMsgLoadPlayerListRet, data)
}

func loadPlayerCount(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadPlayerCount", 2, len(args)) {
		return
	}
	userId, ok := args[0].(uint32)
	serverId, ok1 := args[1].(uint32)
	if !ok || !ok1 {
		return
	}
	flag := false
	var count uint32
	if rows, err := db.OrmEngine.QueryString("call getactorcount(?, ?)", userId, serverId); nil != err {
		logger.LogError("查询角色数量失败！！！err:%s", err)
	} else {
		flag = true
		for _, r := range rows {
			count += utils.AtoUint32(r["actor_count"])
		}
	}

	gshare.SendGameMsg(custom_id.GMsgLoadPlayerCountRet, userId, flag, count)
}

func createPlayer(args ...interface{}) {
	if !gcommon.CheckArgsCount("createPlayer", 1, len(args)) {
		return
	}
	var ip string
	if len(args) >= 2 {
		ip = args[1].(string)
	}
	data, ok := args[0].(mysql.CreatePlayer)
	if !ok {
		return
	}
	job := data.JobSex >> base.SexBit
	sex := data.JobSex & base.SexMask
	actorId, err := base.MakePlayerId(engine.GetPfId(), data.ServerId, data.Series)

	var errno int
	if nil != err {
		errno = custom_id.DbCreateActorFailed
		gshare.SendGameMsg(custom_id.GMsgCreatePlayerRet, mysql.CreatePlayerRet{
			UserId:  data.UserId,
			Errno:   errno,
			ActorId: actorId,
			Name:    data.Name,
		})
		return
	}

	ditchId, subDitchId := data.DitchId, data.SubDitch

	rows, err := db.OrmEngine.QueryString("call clientcreatenewactor(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		data.UserId, 0, actorId, data.Name, data.AccountName, sex, job, data.ServerId, ditchId, subDitchId)

	if nil != err {
		errno = custom_id.DbCreateActorFailed
		logger.LogError("%s", err)
	}
	createPlayerRet := mysql.CreatePlayerRet{
		UserId:  data.UserId,
		Errno:   errno,
		ActorId: actorId,
		Name:    data.Name,

		DitchId:     ditchId,
		SubDitch:    subDitchId,
		AccountName: data.AccountName,
	}

	gshare.SendGameMsg(custom_id.GMsgCreatePlayerRet, createPlayerRet)
	info := engine.Get360WanInfo(ditchId)
	if info != nil {
		gshare.SendSDkMsg(custom_id.GMsgSdkReport, uint32(pb3.SdkReportEventType_SdkReportEventTypeRoleRegister), createPlayerRet, &argsdef.RepostTo360WanCreateRole{
			RepostTo360WanBase: argsdef.RepostTo360WanBase{
				Gid:        info.Gid,
				Sid:        fmt.Sprintf("S%d", engine.GetServerId()),
				OldSid:     fmt.Sprintf("S%d", data.ServerId),
				User:       engine.Get360WanUserId(data.AccountName),
				RoleId:     fmt.Sprintf("%d", actorId),
				Dept:       info.Dept,
				Time:       int64(time_util.NowSec()),
				Gname:      info.Gkey,
				DitchId:    ditchId,
				SubDitchId: subDitchId,
			},
			RoleName: data.Name,
			Prof:     fmt.Sprintf("%d", job),
			Ip:       ip,
		})
	}

	if nil == err && data.IsInvite {
		_, err := db.OrmEngine.Table("account").Where("user_id = ?", data.UserId).
			Update(map[string]interface{}{"isinvite": 1})
		if nil != err {
			logger.LogError("%s", err)
		}
	}

	logger.LogDebug("createPlayer:%v", rows)
}

func clientEnterGame(args ...interface{}) {
	if !gcommon.CheckArgsCount("clientEnterGame", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.ClientEnterGame)
	if !ok {
		return
	}
	var status uint32 = 0
	if rows, err := db.OrmEngine.QueryString("call cliententergame(?, ?, ?)", data.ActorId, data.UserId, data.Ip); nil != err {
		logger.LogError("%s", err)
		return
	} else {
		if len(rows) <= 0 || 1 != utils.Atoi(rows[0]["ret"]) {
			logger.LogError("userId:%d进入游戏未找到对应角色%d或者角色已封禁", data.UserId, data.ActorId)
			return
		}
		status = utils.AtoUint32(rows[0]["status"])
	}

	gshare.SendGameMsg(custom_id.GMsgClientEnterGameRet, mysql.ClientEnterGameRet{
		UserId:       data.UserId,
		ActorId:      data.ActorId,
		TaAccountId:  data.TaAccountId,
		TaDistinctId: data.TaDistinctId,
		RegisteTime:  data.RegisteTime,
		Status:       status,
	})
	logger.LogInfo("【登录流程】角色数据库状态无异常 status:%d", status)
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgVerifyAccount, verifyAccount)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadPlayerList, loadPlayerList)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadPlayerCount, loadPlayerCount)
		gshare.RegisterDBMsgHandler(custom_id.GMsgCreatePlayer, createPlayer)
		gshare.RegisterDBMsgHandler(custom_id.GMsgClientEnterGame, clientEnterGame)
	})
}
