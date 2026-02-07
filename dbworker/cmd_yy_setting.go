/**
 * @Author: LvYuMeng
 * @Date: 2025/6/4
 * @Desc:
**/

package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"

	"github.com/gzjjyz/logger"
	"xorm.io/xorm"
)

func saveCmdYYSetting(args ...interface{}) {
	if len(args) < 2 {
		logger.LogError("saveCmdYYSetting args length %d", len(args))
		return
	}

	data, ok := args[0].(*model.YyCmdSetting)
	if !ok {
		logger.LogError("yySetting args 1 type error")
		return
	}

	noCheck, ok := args[1].(bool)
	if !ok {
		logger.LogError("yySetting args 2 type error")
		return
	}

	// 检查区间冲突
	var existList []model.YyCmdSetting
	err := db.OrmEngine.Where("yy_id = ? AND id != ? AND status != ?", data.YyId, data.Id, gshare.YYTimerSettingStatusInvalid).Find(&existList)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	for _, v := range existList {
		// 区间重叠判断：!(end <= other.start || start >= other.end)
		if !(data.EndTime <= v.StartTime || data.StartTime >= v.EndTime) {
			logger.LogError("YYCmdSetting 时间区间冲突: yyid=%d, id=%d, start=%d, end=%d 与已存在id=%d, start=%d, end=%d", data.YyId, data.Id, data.StartTime, data.EndTime, v.Id, v.StartTime, v.EndTime)
			return
		}
	}

	count, err := db.OrmEngine.Where("id = ?", data.Id).Count(&model.YyCmdSetting{})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	if count == 0 {
		_, err = db.OrmEngine.Insert(data)
		if err != nil {
			logger.LogError("err:%v", err)
			return
		}
	} else {
		_, err = db.OrmEngine.Where("id = ?", data.Id).Update(data)
		if err != nil && err != xorm.ErrNoColumnsTobeUpdated {
			logger.LogError("err:%v", err)
			return
		}
	}

	if !noCheck {
		gshare.SendGameMsg(custom_id.GMsgSaveCmdYYSettingRet, &pb3.YYTimerSetting{
			Id:        data.Id,
			YyId:      data.YyId,
			StartTime: data.StartTime,
			EndTime:   data.EndTime,
			Status:    data.Status,
			ConfIdx:   data.ConfIdx,
			Ext:       data.Ext,
			OpenTime:  data.OpenTime,
		})
	}
}

func deleteCmdYYSetting(args ...interface{}) {
	if len(args) < 1 {
		logger.LogError("deleteCmdYYSetting args length %d", len(args))
		return
	}

	id, ok := args[0].(uint64)
	if !ok {
		logger.LogError("yySetting args 1 type error")
		return
	}

	_, err := db.OrmEngine.Where("id = ?", id).Delete(&model.YyCmdSetting{})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	gshare.SendGameMsg(custom_id.GMsgDeleteCmdYYSettingRet, id)
}

func LoadCmdYYSettingList() (map[uint64]*pb3.YYTimerSetting, error) {
	var list []*model.YyCmdSetting
	err := db.OrmEngine.Where("id > ? AND status != ?", 0, gshare.YYTimerSettingStatusInvalid).Find(&list)
	if err != nil {
		logger.LogError("err:%v", err)
		return nil, err
	}

	var mgr = make(map[uint64]*pb3.YYTimerSetting, len(list))
	for _, line := range list {
		mgr[line.Id] = &pb3.YYTimerSetting{
			Id:        line.Id,
			YyId:      line.YyId,
			StartTime: line.StartTime,
			EndTime:   line.EndTime,
			Status:    line.Status,
			ConfIdx:   line.ConfIdx,
			Ext:       line.Ext,
			OpenTime:  line.OpenTime,
		}
	}
	return mgr, nil
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveCmdYYSetting, saveCmdYYSetting)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeleteCmdYYSetting, deleteCmdYYSetting)
	})
}
