/**
 * @Author: ChenJunJi
 * @Desc: 玩家消息
 * @Date: 2021/9/18 10:17
 */

package dbworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
)

// 新增玩家消息
func addActorMsg(param ...interface{}) {
	if !gcommon.CheckArgsCount("addActorMsg", 3, len(param)) {
		return
	}
	playerId, ok := param[0].(uint64)
	if !ok {
		return
	}
	msgType, ok := param[1].(int)
	if !ok {
		return
	}
	data, ok := param[2].([]byte)
	if !ok {
		return
	}
	ret, err := db.OrmEngine.QueryString("call addActorMsg(?,?,?)", playerId, msgType, data)
	if nil != err {
		logger.LogError("add actor message!!! actorId=%d msgType=%d, err:%v", playerId, msgType, err)
		return
	}
	if len(ret) <= 0 {
		return
	}

	addId := utils.AtoInt64(ret[0]["msgId"])
	gshare.SendGameMsg(custom_id.GMsgAddActorMsg, playerId, addId)
}

// 加载玩家消息
func loadActorMsg(param ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorMsg", 2, len(param)) {
		return
	}
	playerId, ok := param[0].(uint64)
	if !ok {
		return
	}
	msgId, ok := param[1].(int64)
	if !ok {
		return
	}
	ret, err := db.OrmEngine.QueryString("call loadActorMsgList(?,?)", playerId, msgId)
	if nil != err {
		logger.LogError("load actor message error!!! actorId=%d msgId=%d, err:%v", playerId, msgId, err)
		return
	}
	if len(ret) <= 0 {
		return
	}
	rets := make([]*gshare.MsgSt, 0, len(ret))
	for _, line := range ret {
		rets = append(rets, &gshare.MsgSt{
			MsgId:   utils.AtoInt64(line["msg_id"]),
			MsgType: utils.AtoInt(line["msg_type"]),
			Msg:     []byte(line["msg"]),
		})
	}
	gshare.SendGameMsg(custom_id.GMsgLoadActorMsg, playerId, rets)
}

func delActorMsg(param ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorMsg", 2, len(param)) {
		return
	}
	playerId, ok := param[0].(uint64)
	if !ok {
		return
	}
	msgId, ok := param[1].(int64)
	if !ok {
		return
	}
	_, err := db.OrmEngine.Exec("call deleteActorMsg(?,?)", playerId, msgId)
	if nil != err {
		logger.LogError("delete actor message error!!! actorId=%d msgId=%d, err:%v", playerId, msgId, err)
		return
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgAddActorMsg, addActorMsg)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadActorMsg, loadActorMsg)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeleteActorMsg, delActorMsg)
	})
}
