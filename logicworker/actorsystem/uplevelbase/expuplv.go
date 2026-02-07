package uplevelbase

import (
	"encoding/json"
	"errors"
	"fmt"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
)

// 升级组件 通过添加经验值来升级
// 升级后会要求 属性系统重新计算属性
type ExpUpLv struct {
	ExpLv *pb3.ExpLvSt

	RefId            uint32
	BehavAddExpLogId pb3.LogId

	AttrSysId        uint32
	AfterUpLvCb      func(oldLv uint32)
	AfterAddExpCb    func()
	GetLvConfHandler func(lv uint32) *jsondata.ExpLvConf
}

func (u *ExpUpLv) Init(player iface.IPlayer) error {
	if u.ExpLv == nil {
		return errors.New("ExpLv is nil")
	}

	if u.AfterUpLvCb == nil {
		return errors.New("AfterUpLvCb is nil")
	}

	if u.GetLvConfHandler == nil {
		return errors.New("GetLvConfCb is nil")
	}

	if u.AttrSysId <= 0 {
		return fmt.Errorf("AttrSysId incorrect %d", u.AttrSysId)
	}

	if u.AfterAddExpCb == nil {
		return errors.New("AfterAddExpCb is nil")
	}

	if u.BehavAddExpLogId <= pb3.LogId_Unknown {
		return fmt.Errorf("BehavAddExpLogId incorrect %d", u.BehavAddExpLogId)
	}

	return u.reCalculateLv(player)
}

func (u *ExpUpLv) reCalculateLv(player iface.IPlayer) error {
	oldLv := u.ExpLv.Lv
	lvConf := u.GetLvConfHandler(u.ExpLv.Lv + 1)
	for lvConf != nil && u.ExpLv.Exp >= lvConf.RequiredExp {
		u.ExpLv.Exp -= lvConf.RequiredExp
		u.ExpLv.Lv += 1
		lvConf = u.GetLvConfHandler(u.ExpLv.Lv + 1)
	}
	if oldLv < u.ExpLv.Lv {
		u.afterLvUp(player, oldLv)
	}
	return nil
}

func (u *ExpUpLv) AddExp(player iface.IPlayer, exp uint64) error {
	lvConf := u.GetLvConfHandler(u.ExpLv.Lv + 1)
	if lvConf == nil {
		return errors.New("lv is max")
	}

	u.ExpLv.Exp += exp
	oldLv := u.ExpLv.Lv

	for lvConf != nil && u.ExpLv.Exp >= lvConf.RequiredExp {
		u.ExpLv.Exp -= lvConf.RequiredExp
		u.ExpLv.Lv += 1
		lvConf = u.GetLvConfHandler(u.ExpLv.Lv + 1)
	}
	if oldLv < u.ExpLv.Lv {
		u.afterLvUp(player, oldLv)
	}

	u.afterAddExp(player, oldLv)
	return nil
}

func (u *ExpUpLv) afterAddExp(player iface.IPlayer, oldLv uint32) {
	l := &pb3.LogExpLv{
		FromLevel: oldLv,
		ToLevel:   u.ExpLv.Lv,
		Exp:       int64(u.ExpLv.Exp),
		RefId:     u.RefId,
	}
	bt, _ := json.Marshal(l)

	logworker.LogPlayerBehavior(player, u.BehavAddExpLogId, &pb3.LogPlayerCounter{
		NumArgs: uint64(u.ExpLv.Exp),
		StrArgs: string(bt),
	})

	u.AfterAddExpCb()
}

func (u *ExpUpLv) afterLvUp(player iface.IPlayer, oldLv uint32) {
	player.GetAttrSys().ResetSysAttr(u.AttrSysId)
	u.AfterUpLvCb(oldLv)
	return
}
