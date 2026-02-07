/**
 * @Author: LvYuMeng
 * @Date: 2025/4/17
 * @Desc:
**/

package manager

import (
	"encoding/json"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"

	log "github.com/gzjjyz/logger"
)

var playerOnlineData = map[uint64]*pb3.PlayerOnlineData{}
var playerOnlineSaveFlag = map[uint64]struct{}{}

func LoadPlayerOnlineMgr() error {
	playerOnlineData = make(map[uint64]*pb3.PlayerOnlineData)

	syncMsg := syncmsg.NewSyncMsg()
	gshare.SendDBMsg(custom_id.GMsgSyncLoadPlayerOnlineData, syncMsg)
	ret, err := syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}

	lists := ret.(map[uint64]*pb3.PlayerOnlineData)
	for k, v := range lists {
		playerOnlineData[k] = v
	}
	return nil
}

func SavePlayerOnlineMgr(sync bool) error {
	m := make(map[uint64]*pb3.PlayerOnlineData)
	for actorId := range playerOnlineSaveFlag {
		m[actorId] = playerOnlineData[actorId]
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	copyData := make(map[uint64]*pb3.PlayerOnlineData)
	err = json.Unmarshal(bytes, &copyData)
	if err != nil {
		return err
	}

	playerOnlineSaveFlag = map[uint64]struct{}{}

	if !sync {
		gshare.SendDBMsg(custom_id.GMsgPlayerOnlineData, copyData)
		return nil
	}

	syncMsg := syncmsg.NewSyncMsg(copyData)
	gshare.SendDBMsg(custom_id.GMsgSyncSavePlayerOnlineData, syncMsg)
	_, err = syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}
	return nil
}

func GetPlayerOnlineData(actorId uint64) (*pb3.PlayerOnlineData, bool) {
	info, ok := playerOnlineData[actorId]
	if !ok {
		if isActorInThisServer(actorId) {
			playerOnlineData[actorId] = newPlayerOnlineData()
			return playerOnlineData[actorId], true
		}

		log.LogStack("cant init online data %d", actorId)
		return nil, false
	}

	return info, true
}

func SetOlineDataSaveFlag(actorId uint64) {
	playerOnlineSaveFlag[actorId] = struct{}{}
}

func newPlayerOnlineData() *pb3.PlayerOnlineData {
	val := &pb3.PlayerOnlineData{}
	return val
}

func AllOnlineDataDo(fn func(actorId uint64, data *pb3.PlayerOnlineData)) {
	for actorId, data := range playerOnlineData {
		fn(actorId, data)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		err := LoadPlayerOnlineMgr()
		if err != nil {
			log.LogError("LoadPlayerOnlineMgr err:%v", err)
			return
		}
	})
}
