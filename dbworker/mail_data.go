/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:59
 */

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func updateMailStatus(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateMailStatus", 3, len(args)) {
		return
	}
	actorId, ok1 := args[0].(uint64)
	mailId, ok2 := args[1].(int64)
	status, ok3 := args[2].(uint32)
	if !ok1 || !ok2 || !ok3 {
		logger.LogError("updateMailStatus Args Error!!!, ok1=%v, ok2=%v, ok3=%v", ok1, ok2, ok3)
		return
	}
	_, err := db.OrmEngine.Exec(SQLUpdateMailStatus, actorId, mailId, status)
	if err != nil {
		logger.LogError("[actor: %d]updateMailStatus Error! mailId:%d, status:%d, error:%v", actorId, mailId, status, err)
	}
}

// 加载玩家邮件
func loadActorMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorMail", 2, len(args)) {
		return
	}

	actorId, ok1 := args[0].(uint64)
	mailId, ok2 := args[1].(uint64)
	if !ok1 || !ok2 {
		logger.LogError("remote call loadActorMail args error, ok1=%v, ok2=%v", ok1, ok2)
		return
	}

	var list []*gshare.ActorMailStEx
	if err := db.OrmEngine.SQL(SQLLoadActorMail, actorId, mailId).Find(&list); nil != err {
		logger.LogError("%v", err)
		ret := make([]*gshare.ActorMailSt, 0)
		gshare.SendGameMsg(custom_id.GMsgLoadActorMail, actorId, false, mailId, ret)
		return
	}

	ret := make([]*gshare.ActorMailSt, 0, len(list))
	for _, mailItem := range list {
		st := &gshare.ActorMailSt{
			ActorId:  mailItem.ActorId,
			MailId:   mailItem.MailId,
			ConfId:   mailItem.ConfId,
			MailType: mailItem.Type,
			Status:   mailItem.Status,
			SendTick: mailItem.SendTick,
			SendName: mailItem.SendName,
			Title:    mailItem.Title,
			Content:  mailItem.Content,
			AwardStr: mailItem.AwardStr,
			UserItem: mailItem.UserItem,
		}

		ret = append(ret, st)
	}
	gshare.SendGameMsg(custom_id.GMsgLoadActorMail, actorId, true, mailId, ret)
}

// 加载玩家服务器邮件
func loadActorServerMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorServerMail", 1, len(args)) {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok {
		logger.LogError("remote call loadActorMail args error!!!")
		return
	}

	index, success := uint64(0), true
	ret, err := db.OrmEngine.QueryString(SQLLoadActorServerMail, actorId)
	if nil != err {
		logger.LogError("LoadActorServerMail Error! actorId:%d, error:%v", actorId, err)
		success = false
	} else {
		if len(ret) > 0 {
			index = utils.AtoUint64(ret[0]["mail_id"])
		}
	}
	gshare.SendGameMsg(custom_id.GMsgLoadActorServerMail, actorId, success, index)
}

// 新增玩家邮件
func addActorMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("addActorMail", 2, len(args)) {
		return
	}

	actorId, ok1 := args[0].(uint64)
	data, ok2 := args[1].(*gshare.ActorMailSt)
	if !ok1 || !ok2 {
		logger.LogError("addActorMail Args Error!!!, ok1=%v, ok2=%v", ok1, ok2)
		return
	}
	var err error
	var ret []map[string]string
	if data.MailType == gshare.MailTypeMsg || data.MailType == gshare.MailTypeFile {
		ret, err = db.OrmEngine.QueryString(SQLAddActorMail, data.MailId, actorId, data.ConfId, data.MailType, gshare.MailUnRead,
			time_util.NowSec(), "", data.Title, data.Content, data.AwardStr)
		if err != nil {
			logger.LogError("[actor: %d] AddActorMail Error! error:%v", actorId, err)
		}
	} else if data.MailType == gshare.MailTypeUserItem {
		ret, err = db.OrmEngine.QueryString(SQLAddUserItemMail, data.MailId, actorId, data.ConfId, data.MailType, 0, time_util.NowSec(), "",
			data.Title, data.Content, data.UserItem)
		if err != nil {
			logger.LogError("[actor: %d] AddActorMail Error! error:%s", actorId, err)
		}
	} else {
		logger.LogError("[actor: %d] AddActorMail Error!! mail type error. mailType:%d", actorId, data.MailType)
		return
	}

	if nil != err {
		return
	}
	var delId uint64
	if len(ret) > 0 {
		delId = utils.AtoUint64(ret[0]["minmailid"])
	}
	gshare.SendGameMsg(custom_id.GMsgAddMailRet, actorId, uint64(data.MailId), delId)
}

// 更新玩家服务器邮件
func updateActorServerMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateActorServerMail", 2, len(args)) {
		return
	}
	actorId, ok1 := args[0].(uint64)
	index, ok2 := args[1].(uint64)
	if !ok1 || !ok2 {
		logger.LogError("updateActorServerMail Args Error!!!, ok1=%v, ok2=%v", ok1, ok2)
		return
	}
	_, err := db.OrmEngine.Exec(SQLUpdateActorServerMail, actorId, index)
	if err != nil {
		logger.LogError("UpdateActorServerMail Error! actorId:%d, index:%d, error:%v", actorId, index, err)
	}
}

func deleteActorMail(args ...interface{}) {
	if !gcommon.CheckArgsCount("deleteActorMail", 2, len(args)) {
		return
	}

	actorId, ok1 := args[0].(uint64)
	delVec, ok2 := args[1].([]uint64)
	if !ok1 || !ok2 {
		logger.LogError("deleteActorMail Args Error!!!, ok1=%v, ok2=%v", ok1, ok2)
		return
	}

	for _, mailId := range delVec {
		_, err := db.OrmEngine.Exec(SQLDeleteActorMail, actorId, mailId)
		if err != nil {
			logger.LogError("DeleteActorMail Error! actorId:%d, mailId:%d, error:%v", actorId, mailId, err)
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadActorMail, loadActorMail)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadActorServerMail, loadActorServerMail)
		gshare.RegisterDBMsgHandler(custom_id.GMsgAddMail, addActorMail)
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateActorServerMail, updateActorServerMail)
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateMailStatus, updateMailStatus)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeleteActorMail, deleteActorMail)
	})
}
