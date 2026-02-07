package engine

type BaseData struct {
	Id          uint32 `xorm:"pk autoincr"`
	CreatedAt   uint32 `xorm:"created"`
	RoleId      uint64 //角色Id
	Name        string //名称
	UserId      uint64 //用户Id
	AccountName string //用户名称
	ServerId    uint32 //区服Id
}

type RoleData struct {
	Id       uint64 `xorm:"pk"`
	UpdateAt uint32 `xorm:"updated"`
	Role     []byte
}
