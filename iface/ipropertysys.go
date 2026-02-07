package iface

type IPropertySys interface {
	ResetSysAttr(sysId uint32)
	TraceProperty(force bool)
}
