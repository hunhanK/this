/**
 * @Author: LvYuMeng
 * @Date: 2025/6/4
 * @Desc: 后台活动设置
**/

package cmdyysettingmgr

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/dbworker"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/model"
	"jjyz/gameserver/redisworker/redismid"
)

var (
	timeSettingMap = map[uint64]*pb3.YYTimerSetting{}
)

func Load() error {
	data, err := dbworker.LoadCmdYYSettingList()
	if nil != err {
		return err
	}
	timeSettingMap = data
	return nil
}

func AddTimerSetting(cmd *gshare.YYTimerSettingCmdOpen) {
	_, ok := jsondata.GetCmdYYConf(cmd.YYId)
	if !ok {
		logger.LogError("找不到配置:%d", cmd.YYId)
		return
	}

	nowSec := time_util.NowSec()
	if nowSec >= cmd.EndTime {
		logger.LogError("时间过期")
		return
	}

	for _, setting := range timeSettingMap {
		if setting.YyId != cmd.YYId {
			continue
		}
		if setting.Status != gshare.YYTimerSettingStatusValid { //只校验生效状态
			continue
		}
		if !(cmd.EndTime <= setting.StartTime || setting.EndTime <= cmd.StartTime) {
			logger.LogError("同步错误,运营活动:%d,时间重叠", cmd.YYId)
			return
		}
	}

	allocSeries, err := series.AllocSeries()
	if err != nil {
		logger.LogError("序列生成错误")
		return
	}

	SaveSetting(&model.YyCmdSetting{
		Id:        allocSeries,
		YyId:      cmd.YYId,
		StartTime: cmd.StartTime,
		EndTime:   cmd.EndTime,
		Status:    gshare.YYTimerSettingStatusValid,
		Ext:       cmd.Ext,
	}, false)

	logger.LogInfo("AddTimerSetting:%+v", cmd)
}

func GetSettingOpenTime(yyId uint32) uint32 {
	nowSec := time_util.NowSec()
	for _, v := range timeSettingMap {
		if v.YyId != yyId {
			continue
		}
		if v.Status != gshare.YYTimerSettingStatusValid {
			continue
		}
		if v.StartTime > nowSec || v.EndTime < nowSec {
			continue
		}
		return v.OpenTime
	}

	return 0
}

func SaveSetting(setting *model.YyCmdSetting, noCheck bool) {
	if nil == setting {
		return
	}
	gshare.SendDBMsg(custom_id.GMsgSaveCmdYYSetting, setting, noCheck)
}

func findSettingByExt(ext string) (*pb3.YYTimerSetting, error) {
	for _, setting := range timeSettingMap {
		if setting.Ext == ext {
			return setting, nil
		}
	}
	return nil, fmt.Errorf("不存在的穿透参数:%s", ext)
}

func UpdateTimerSetting(cmd *gshare.YYTimerSettingCmdUpdate) {
	setting, err := findSettingByExt(cmd.Ext)
	if nil != err {
		logger.LogError("UpdateTimerSetting err:%v", err)
		return
	}

	if cmd.StartTime == 0 || cmd.EndTime == 0 {
		logger.LogError("时间格式错误")
		return
	}

	nowSec := time_util.NowSec()
	if setting.Status == gshare.YYTimerSettingStatusValid && nowSec >= setting.StartTime && nowSec <= setting.EndTime {
		return
	}

	if nowSec >= cmd.EndTime {
		logger.LogError("时间过期")
		return
	}

	for _, line := range timeSettingMap {
		if setting.YyId != line.YyId || setting.Id == line.Id {
			continue
		}
		if setting.Status != gshare.YYTimerSettingStatusValid { //只校验生效状态
			continue
		}
		if !(cmd.EndTime <= setting.StartTime || setting.EndTime <= cmd.StartTime) {
			logger.LogError("同步错误,运营活动:%d,时间重叠", setting.YyId)
			return
		}
	}

	SaveSetting(&model.YyCmdSetting{
		Id:        setting.Id,
		YyId:      setting.YyId,
		StartTime: cmd.StartTime,
		EndTime:   cmd.EndTime,
		Status:    gshare.YYTimerSettingStatusValid,
		ConfIdx:   setting.ConfIdx,
		Ext:       setting.Ext,
		OpenTime:  setting.OpenTime,
	}, false)

	logger.LogInfo("UpdateTimerSetting:%+v", cmd)
}

func DeleteTimerSetting(cmd *gshare.YYTimerSettingCmdDelete) {
	setting, err := findSettingByExt(cmd.Ext)
	if nil != err {
		logger.LogError("DeleteTimerSetting err:%v", err)
		return
	}

	nowSec := time_util.NowSec()
	if setting.Status == gshare.YYTimerSettingStatusValid && setting.StartTime >= nowSec && setting.EndTime <= nowSec {
		logger.LogError("活动生效中:%d", setting.YyId)
		return
	}

	gshare.SendDBMsg(custom_id.GMsgDeleteCmdYYSetting, setting.Id)

	logger.LogInfo("DeleteTimerSetting:%+v", cmd)
}

func CloseTimerSetting(cmd *gshare.YYTimerSettingCmdClose) {
	setting, err := findSettingByExt(cmd.Ext)
	if nil != err {
		logger.LogError("CloseTimerSetting err:%v", err)
		return
	}

	if setting.Status == gshare.YYTimerSettingStatusInvalid {
		logger.LogError("序列活动重复关闭:%d", setting.Id)
		return
	}

	SaveSetting(&model.YyCmdSetting{
		Id:        setting.Id,
		YyId:      setting.YyId,
		StartTime: setting.StartTime,
		EndTime:   setting.EndTime,
		Status:    gshare.YYTimerSettingStatusInvalid,
		ConfIdx:   0,
		Ext:       setting.Ext,
		OpenTime:  0,
	}, false)

	logger.LogInfo("CloseTimerSetting:%+v", cmd)
}

func saveCmdYYSettingRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	data, ok := args[0].(*pb3.YYTimerSetting)
	if !ok {
		return
	}

	timeSettingMap[data.Id] = data

	switch data.Status {
	case gshare.YYTimerSettingStatusValid:
		checkCmdYYOpen(data.Id, false, nil)
	case gshare.YYTimerSettingStatusInvalid:
		closeCmdYY(data.Id)
	}
}

func closeCmdYY(id uint64) {
	setting, ok := timeSettingMap[id]
	if !ok {
		logger.LogError("运营活动序列:%d,关闭失败,找不到预设", id)
		return
	}

	if setting.Status != gshare.YYTimerSettingStatusInvalid {
		logger.LogError("运营活动序列:%d,关闭失败,正在生效中无法直接关闭", setting.Id)
		return
	}

	conf, ok := jsondata.GetCmdYYConf(setting.YyId)
	if !ok {
		logger.LogError("运营活动序列:%d,关闭失败,找不到配置", setting.Id)
		return
	}

	nowSec := time_util.NowSec()
	if nowSec < setting.StartTime || nowSec > setting.EndTime { //收到指令但不能把正在开的关了
		logger.LogWarn("运营活动序列:%d 开启时间不符,关闭失败,仅失效设置", setting.Id)
		return
	}

	if conf.IsGlobal {
		yymgr.GetYYMgr().CloseYY(setting.YyId, false)
	} else {
		pyymgr.CloseAllPlayerYY(setting.YyId)
	}
}

func checkCmdYYOpen(id uint64, isInit bool, player iface.IPlayer) {
	setting, ok := timeSettingMap[id]
	if !ok {
		return
	}

	if setting.Status != gshare.YYTimerSettingStatusValid {
		return
	}

	nowSec := time_util.NowSec()

	canOpen := setting.StartTime <= nowSec && setting.EndTime >= nowSec
	if !canOpen {
		return
	}

	conf, ok := jsondata.GetCmdYYConf(setting.YyId)
	if !ok {
		return
	}

	if setting.ConfIdx == 0 {
		tmplConf, exist := conf.GetCmdYYTemplate(gshare.GetOpenServerDay(), gshare.GetMergeSrvDay(), gshare.GetMergeTimes())
		if !exist {
			return
		}

		setting.ConfIdx = tmplConf.ConfIdx
		setting.OpenTime = time_util.NowSec()

		SaveSetting(&model.YyCmdSetting{
			Id:        setting.Id,
			YyId:      setting.YyId,
			StartTime: setting.StartTime,
			EndTime:   setting.EndTime,
			Status:    setting.Status,
			ConfIdx:   setting.ConfIdx,
			Ext:       setting.Ext,
			OpenTime:  setting.OpenTime,
		}, true)

		gshare.SendRedisMsg(redismid.SaveCmdYYSetting, &argsdef.CmdYYSetting{
			PfId:    engine.GetPfId(),
			SrvId:   engine.GetServerId(),
			Id:      setting.Id,
			ConfIdx: setting.ConfIdx,
		})
	}

	if conf.IsGlobal {
		yymgr.GetYYMgr().OpenYY(setting.YyId, setting.StartTime, setting.EndTime, setting.ConfIdx, isInit)
	} else {
		if nil == player {
			manager.AllOnlinePlayerDo(func(p iface.IPlayer) {
				p.OpenPyy(setting.YyId, setting.StartTime, setting.EndTime, setting.ConfIdx, pyymgr.CmdYYOpenParser, isInit)
			})
		} else {
			player.OpenPyy(setting.YyId, setting.StartTime, setting.EndTime, setting.ConfIdx, pyymgr.CmdYYOpenParser, isInit)
		}
	}
}

func checkAllPlayerCmdYYOpen(player iface.IPlayer, isInit bool) {
	for _, setting := range timeSettingMap {
		conf, ok := jsondata.GetCmdYYConf(setting.YyId)
		if !ok {
			continue
		}
		if conf.IsGlobal {
			continue
		}
		checkCmdYYOpen(setting.Id, isInit, player)
	}
}

func checkAllGlobalCmdYYOpen(isInit bool) {
	for _, setting := range timeSettingMap {
		conf, ok := jsondata.GetCmdYYConf(setting.YyId)
		if !ok {
			continue
		}
		if !conf.IsGlobal {
			continue
		}
		checkCmdYYOpen(setting.Id, isInit, nil)
	}
}

func deleteCmdYYSettingRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	id, ok := args[0].(uint64)
	if !ok {
		return
	}

	delete(timeSettingMap, id)
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgSaveCmdYYSettingRet, saveCmdYYSettingRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgDeleteCmdYYSettingRet, deleteCmdYYSettingRet)
	})

	event.RegSysEvent(custom_id.SeCheckCmdYYOpen, func(args ...interface{}) {
		if len(args) < 1 {
			return
		}
		isInit, _ := args[0].(bool)
		checkAllGlobalCmdYYOpen(isInit)
	})

	event.RegActorEvent(custom_id.AeCheckCmdYYOpen, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		isInit, _ := args[0].(bool)
		checkAllPlayerCmdYYOpen(player, isInit)
	})
}
