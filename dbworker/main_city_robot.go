/**
 * @Author: zjj
 * @Date: 2024/4/17
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
)

type mainCityRobotData struct {
	Id            uint64 `xorm:"id"`
	RobotSeriesId uint64 `xorm:"robotSeriesId"`
	Data          []byte `xorm:"data"`
}

func (d mainCityRobotData) TableName() string {
	return "main_city_robot_data"
}

func loadMainCityRobotData(_ ...interface{}) {
	var list []*mainCityRobotData
	err := db.OrmEngine.Where("robotSeriesId > ?", 0).Find(&list)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	var ret []*pb3.MainCityRobotData
	for _, v := range list {
		var data pb3.MainCityRobotData
		err := pb3.Unmarshal(v.Data, &data)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		ret = append(ret, &data)
	}
	gshare.SendGameMsg(custom_id.GMsgLoadMainCityRobotDataRet, ret)
}

func saveMainCityRobotData(args ...interface{}) {
	if len(args) != 1 {
		logger.LogError("saveMainCityRobotData args length %d", len(args))
		return
	}

	mainCityRobotDataList, ok := args[0].([]*pb3.MainCityRobotData)
	if !ok {
		logger.LogError("saveMainCityRobotData args type error")
		return
	}

	for _, v := range mainCityRobotDataList {
		data, err := pb3.Marshal(v)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		count, err := db.OrmEngine.Where("robotSeriesId = ?", v.Id).Count(&mainCityRobotData{})
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
		if count == 0 {
			m := &mainCityRobotData{
				RobotSeriesId: v.Id,
				Data:          data,
			}
			_, err = db.OrmEngine.Insert(m)
			if err != nil {
				logger.LogError("err:%v", err)
			}
			continue
		}
		_, err = db.OrmEngine.Where("robotSeriesId = ?", v.Id).Update(&mainCityRobotData{
			Data: data,
		})
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadMainCityRobotData, loadMainCityRobotData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveMainCityRobotData, saveMainCityRobotData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgSyncSaveMainCityRobotData, syncmsg.NewSyncMsgHandle(func(args ...interface{}) (interface{}, error) {
			saveMainCityRobotData(args...)
			return nil, nil
		}))
	})
}
