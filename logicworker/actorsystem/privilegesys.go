package actorsystem

import (
	"fmt"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

/**
* @Author: YangQibin
* @Desc: 特权系统
* @Date: 2023/6/13
 */

type (
	PrivilegeSys struct {
		Base
	}
	pvCalculater func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error)
)

var (
	pvCalculaters []pvCalculater = make([]pvCalculater, 0)
)

func RegisterPrivilegeCalculater(calc pvCalculater) {
	pvCalculaters = append(pvCalculaters, calc)
}

func (sys *PrivilegeSys) OnReconnect() {}

func (sys *PrivilegeSys) GetPrivilege(pType privilegedef.PrivilegeType) (total int64, err error) {
	conf := jsondata.GetPrivilegeConf(int32(pType))
	if conf == nil {
		err = fmt.Errorf("PrivilegeConf for type %d is nil", pType)
		return
	}

	for _, calc := range pvCalculaters {
		var foo int64
		foo, err = calc(sys.GetOwner(), conf)
		if err != nil {
			return
		}

		total += foo
	}

	return
}

func (sys *PrivilegeSys) OnLoginFight() {
	sys.syncPrivilegesToFight()
}

func (sys *PrivilegeSys) collectPrivileges() (privielges map[int32]int64) {
	privielges = make(map[int32]int64)
	for i := privilegedef.EnumPrivilegeStart; i <= privilegedef.EnumPrivilegeTypeMax; i++ {
		privielges[int32(i)], _ = sys.GetPrivilege(privilegedef.PrivilegeType(i))
	}

	return
}

func (sys *PrivilegeSys) syncPrivilegesToFight() {
	privileges := sys.collectPrivileges()
	sys.GetOwner().CallActorFunc(actorfuncid.SyncPrivileges, &pb3.SyncPrivilegesReq{
		Privileges: privileges,
	})
}

func init() {
	RegisterSysClass(sysdef.SiPrivilegeSys, func() iface.ISystem {
		return &PrivilegeSys{}
	})

	event.RegActorEvent(custom_id.AePrivilegeRelatedSysChanged, func(actor iface.IPlayer, args ...interface{}) {
		sys := actor.GetSysObj(sysdef.SiPrivilegeSys).(*PrivilegeSys)
		sys.syncPrivilegesToFight()
	})
}
