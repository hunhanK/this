/**
 * @Author: zjj
 * @Date: 2024/5/9
 * @Desc:
**/

package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"xorm.io/xorm"
)

type AskHelp struct {
	PlayerId uint64 `xorm:"player_id"`
	Data     []byte `xorm:"data"`
}

func (d AskHelp) TableName() string {
	return "ask_help"
}

func loadAskHelpData(_ ...interface{}) (interface{}, error) {
	var list []*AskHelp
	err := db.OrmEngine.Where("player_id > ?", 0).Find(&list)
	if err != nil {
		logger.LogError("err:%v", err)
		return nil, err
	}

	var mgr = make(map[uint64]*pb3.AskHelpInfo, len(list))
	for _, rank := range list {
		info := &pb3.AskHelpInfo{}
		err := pb3.Unmarshal(rank.Data, info)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		mgr[rank.PlayerId] = info
	}
	return mgr, nil
}

func saveAskHelpData(args ...interface{}) {
	if len(args) != 1 {
		logger.LogError("saveAskHelpData args length %d", len(args))
		return
	}

	list, ok := args[0].(map[uint64]*pb3.AskHelpInfo)
	if !ok {
		logger.LogError("saveAskHelpData args type error")
		return
	}

	for playerId, v := range list {
		if playerId == 0 {
			continue
		}
		blob, err := pb3.Marshal(v)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}

		count, err := db.OrmEngine.Where("player_id = ?", playerId).Count(&AskHelp{})
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}

		if count == 0 {
			_, err = db.OrmEngine.Insert(&AskHelp{
				PlayerId: playerId,
				Data:     blob,
			})
			if err != nil {
				logger.LogError("err:%v", err)
			}
			continue
		}

		_, err = db.OrmEngine.Where("player_id = ?", playerId).Update(&AskHelp{
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
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncSaveAskHelpData, syncmsg.NewSyncMsgHandle(func(args ...interface{}) (interface{}, error) {
			saveAskHelpData(args...)
			return nil, nil
		}))
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncLoadAskHelpData, syncmsg.NewSyncMsgHandle(loadAskHelpData))
		gshare.RegisterDBMsgHandler(custom_id.GMsgAskHelpData, saveAskHelpData)
	})
}
