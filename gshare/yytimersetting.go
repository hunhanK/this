/**
 * @Author: LvYuMeng
 * @Date: 2025/6/4
 * @Desc:
**/

package gshare

type YYTimerSettingCmdOpen struct {
	YYId      uint32
	StartTime uint32
	EndTime   uint32
	Ext       string
}

type YYTimerSettingCmdClose struct {
	Ext string
}

type YYTimerSettingCmdUpdate struct {
	Ext       string
	StartTime uint32
	EndTime   uint32
}

type YYTimerSettingCmdDelete struct {
	Ext string
}

const (
	YYTimerSettingStatusValid   = 1
	YYTimerSettingStatusInvalid = 2
)

const (
	YYTimerSettingOpAdd    = 1
	YYTimerSettingOpClose  = 2
	YYTimerSettingOpUpdate = 3
	YYTimerSettingOpDelete = 4
)
