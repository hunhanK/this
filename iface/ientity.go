package iface

type IEntity interface {
	IEntityBase
}

type IEntityBase interface {
	GetId() uint64
	GetName() string
	GetLevel() uint32
	RunOne()
	Destroy()
}
