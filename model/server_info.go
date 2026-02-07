package model

type ServerInfo struct {
	ServerId         uint32 `xorm:"'server_id' notnull pk"`
	ForbidCreateFlag uint32 `xorm:"'forbid_create_flag' notnull pk"`
	OpenTime         uint32 `xorm:"'open_time' notnull"`
}

func (m ServerInfo) TableName() string {
	return "serverinfo"
}
