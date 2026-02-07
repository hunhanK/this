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
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"xorm.io/xorm"
)

type RushRank struct {
	RankType uint8  `xorm:"rank_type"`
	Data     []byte `xorm:"data"`
}

func (d RushRank) TableName() string {
	return "rush_rank"
}

func loadRushRankData(_ ...interface{}) (interface{}, error) {
	var list []*RushRank
	err := db.OrmEngine.Where("rank_type > ?", 0).Find(&list)
	if err != nil {
		logger.LogError("err:%v", err)
		return nil, err
	}
	var retList []*mysql.RankList
	for _, rank := range list {
		rankList := &pb3.OneRankList{}
		err := pb3.Unmarshal(rank.Data, rankList)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		retList = append(retList, &mysql.RankList{
			RankType: uint32(rank.RankType),
			RankData: rankList.RankInfo,
		})
	}
	return retList, nil
}

func saveRushRankData(args ...interface{}) {
	if len(args) != 1 {
		logger.LogError("saveRushRankData args length %d", len(args))
		return
	}

	list, ok := args[0].([]*mysql.RankList)
	if !ok {
		logger.LogError("saveRushRankData args type error")
		return
	}

	for _, v := range list {
		blob, err := pb3.Marshal(&pb3.OneRankList{RankInfo: v.RankData})
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		_, err = db.OrmEngine.Where("rank_type = ?", v.RankType).Delete(&RushRank{})
		if err != nil {
			logger.LogError("err:%v", err)
		}
		_, err = db.OrmEngine.Insert(&RushRank{
			RankType: uint8(v.RankType),
			Data:     blob,
		})
		if err != nil && err != xorm.ErrNoColumnsTobeUpdated {
			logger.LogError("err:%v", err)
			continue
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncSaveRushRankData, syncmsg.NewSyncMsgHandle(func(args ...interface{}) (interface{}, error) {
			saveRushRankData(args...)
			return nil, nil
		}))
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveRushRankData, saveRushRankData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncLoadRushRankData, syncmsg.NewSyncMsgHandle(loadRushRankData))
	})
}
