/**
 * @Author: zjj
 * @Date: 2023/12/11
 * @Desc:
**/

package iface

import (
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/pb3"
)

type IRobot interface {
	IRobotAppearOwner
	Logout()
	Login()
	GetLevel() uint32
	GetRobotId() uint64
	GetName() string
	SetGuildId(id uint64)
	GetGuildId() uint64
	GetData() *pb3.MainCityRobotData

	GetJob() uint32
	GetSex() uint32
	IsOnline() bool
	IsFlagBit(bit uint32) bool
	SetFlagBit(bit uint32)
	GetSysObj(sysId int) IRobotSystem
	ResetSysAttr(sysId uint32) // 重算系统属性
	SetAttr(attrType attrdef.AttrTypeAlias, attrValue attrdef.AttrValueAlias)
	GetAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias
	GetLastLogoutTime() uint32
	GetLoginTime() uint32
	SetChange(change bool)
}

type IRobotAppearOwner interface {
	GetHeadFrame() uint32   // 头像框
	GetBubbleFrame() uint32 // 气泡框
	GetHead() uint32        // 头像
}

type IRobotSystem interface {
	DoUpdate()
	CanUpdate() bool
	OnInit()
	OnLoadFinish()
	OnLogin()
	OnLogout()
	OnSave()
	OnReset()
	GetOwner() IRobot
	CheckUpdateTime(interval uint32) bool
	GetUpdateInterval() uint32
}
