/**
 * @Author: ChenJunJi
 * @Desc: 好友数据
 * @Date: 2021/9/13 11:08
 */

package dbworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
)

// 加载好友数据
func loadFriendData(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok {
		return
	}
	ret, err := db.OrmEngine.QueryString("call loadFriends(?)", actorId)
	if nil != err {
		logger.LogError("%v", err)
		return
	}
	gshare.SendGameMsg(custom_id.GMsgLoadFriendData, actorId, ret)
}

var excludeFrType = []uint32{custom_id.FrEngagement, custom_id.FrMarry}

// 更新好友
func updateFriend(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateFriend", 4, len(args)) {
		return
	}
	actorId, ok0 := args[0].(uint64)
	fId, ok1 := args[1].(uint64)
	fType, ok2 := args[2].(uint32)
	opCode, ok3 := args[3].(uint32)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		return
	}
	if utils.SliceContainsUint32(excludeFrType, fType) {
		return
	}
	if _, err := db.OrmEngine.Exec("call updateFriends(?,?,?,?)", actorId, fId, opCode, fType); nil != err {
		logger.LogError("updateFriend error! %v", err)
		return
	}
}

// 加载好友数据
func getFriendStatus(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok {
		return
	}
	friendId, ok := args[1].(uint64)
	if !ok {
		return
	}
	ret, err := db.OrmEngine.QueryString("call getFriendStatus(?,?)", actorId, friendId)
	if nil != err {
		logger.LogError("%v", err)
		return
	}
	gshare.SendGameMsg(custom_id.GMsGetFriendStatusRet, actorId, friendId, ret)
}

// 保存亲密度
func saveFriendIntimacy(args ...interface{}) {
	if !gcommon.CheckArgsCount("updatefriendsintimacy", 1, len(args)) {
		return
	}
	common, ok := args[0].(*pb3.CommonSt)
	if !ok {
		logger.LogError("saveFriendIntimacy err")
		return
	}
	actorId := common.U64Param
	friendId := common.U64Param2
	intimacy := common.U32Param

	if _, err := db.OrmEngine.Exec("call updatefriendsintimacy(?,?,?)", actorId, friendId, intimacy); nil != err {
		logger.LogError("updatefriendsintimacy error! %v", err)
		return
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadFriendData, loadFriendData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateFriends, updateFriend)
		gshare.RegisterDBMsgHandler(custom_id.GMsGetFriendStatus, getFriendStatus)
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveFriendIntimacy, saveFriendIntimacy)
	})
}
