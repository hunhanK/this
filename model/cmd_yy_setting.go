/**
 * @Author: LvYuMeng
 * @Date: 2025/6/4
 * @Desc:
**/

package model

type YyCmdSetting struct {
	Id        uint64 `xorm:"pk 'id'"`
	YyId      uint32
	StartTime uint32
	EndTime   uint32
	Status    uint32
	ConfIdx   uint32
	Ext       string
	OpenTime  uint32
}
