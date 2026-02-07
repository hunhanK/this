package iface

import "jjyz/base/pb3"

type IAttrSys interface {
	ResetSysAttr(sysId uint32)
	PackPropertyData(data *pb3.OfflineProperty)
	PackFightPropertyData(data *pb3.DetailedRoleInfo)
	PackCreateData(data *pb3.CreateActorData)
	ReSendPowerMap()
	TriggerUpdateSysPowerMap()
	GetSysPower(attrSysId uint32) int64

	GetDailyInitPowerInfo() *pb3.DailyInitPowerInfo
}
