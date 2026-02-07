package model

type OfflineData struct {
	ActorId uint64 `xorm:"pk 'actor_id'"`
	SysId   uint32 `xorm:"pk 'sys_id'"`
	Data    []byte `xorm:"data"`
}

func (m OfflineData) TableName() string {
	return "offlinedata"
}
