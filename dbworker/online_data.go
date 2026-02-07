/**
 * @Author: LvYuMeng
 * @Date: 2025/4/17
 * @Desc:
**/

package dbworker

import (
	"jjyz/base/compress"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"

	"github.com/gzjjyz/logger"
	"xorm.io/xorm"
)

type OnlineData struct {
	PlayerId uint64 `xorm:"player_id"`
	Data     []byte `xorm:"data"`
}

func (d OnlineData) TableName() string {
	return "online_data"
}

func loadPlayerOnlineData(_ ...interface{}) (interface{}, error) {
	var list []*OnlineData
	err := db.OrmEngine.Where("player_id > ?", 0).Find(&list)
	if err != nil {
		logger.LogError("err:%v", err)
		return nil, err
	}

	var mgr = make(map[uint64]*pb3.PlayerOnlineData, len(list))
	for _, m := range list {
		if m.PlayerId == 0 {
			continue
		}
		info := &pb3.PlayerOnlineData{}
		if len(m.Data) > 0 {
			err := pb3.Unmarshal(compress.UncompressPb(m.Data), info)
			if err != nil {
				logger.LogFatal("unmarshal cross var error %v", err)
				continue
			}
		}

		mgr[m.PlayerId] = info
	}
	return mgr, nil
}

func savePlayerOnlineData(args ...interface{}) {
	if len(args) != 1 {
		logger.LogError("savePlayerOnlineData args length %d", len(args))
		return
	}

	list, ok := args[0].(map[uint64]*pb3.PlayerOnlineData)
	if !ok {
		logger.LogError("savePlayerOnlineData args type error")
		return
	}

	for playerId, v := range list {
		blob, err := compress.PB3ToByte(v)
		if nil != err {
			logger.LogError("player %d save err:%v data:{%+v}", playerId, err, v)
			continue
		}

		count, err := db.OrmEngine.Where("player_id = ?", playerId).Count(&OnlineData{})
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}

		if count == 0 {
			_, err = db.OrmEngine.Insert(&OnlineData{
				PlayerId: playerId,
				Data:     blob,
			})
			if err != nil {
				logger.LogError("err:%v", err)
			}
			continue
		}

		_, err = db.OrmEngine.Where("player_id = ?", playerId).Update(&OnlineData{
			Data: blob,
		})
		if err != nil && err != xorm.ErrNoColumnsTobeUpdated {
			logger.LogError("err:%v", err)
			continue
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncSavePlayerOnlineData, syncmsg.NewSyncMsgHandle(func(args ...interface{}) (interface{}, error) {
			savePlayerOnlineData(args...)
			return nil, nil
		}))
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncLoadPlayerOnlineData, syncmsg.NewSyncMsgHandle(loadPlayerOnlineData))
		gshare.RegisterDBMsgHandler(custom_id.GMsgPlayerOnlineData, savePlayerOnlineData)
	})
}
