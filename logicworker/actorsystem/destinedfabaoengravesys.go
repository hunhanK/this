/**
 * @Author: zjj
 * @Date: 2024/7/17
 * @Desc: 本命法宝铭刻系统
**/

package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	DestinedFaBaoEngraveAdd    uint8 = 1
	DestinedFaBaoEngraveReduce uint8 = 2
	DestinedFaBaoEngraveKeep   uint8 = 3
)

type EngraveSys struct {
	Base
}

func (s *EngraveSys) getData() *pb3.EngraveData {
	data := s.GetOwner().GetBinaryData()
	val := data.EngraveData
	if val == nil {
		data.EngraveData = &pb3.EngraveData{}
		val = data.EngraveData
	}
	return val
}

func (s *EngraveSys) s2cInfo() {
	s.SendProto3(163, 30, &pb3.S2C_163_30{
		Data: s.getData(),
	})
}

func (s *EngraveSys) OnLogin() {
	s.s2cInfo()
}

func (s *EngraveSys) checkAttrCanUse(player iface.IPlayer, attr *jsondata.Attr) bool {
	if attr == nil {
		return false
	}
	if player == nil {
		return false
	}
	if attr.Job != 0 && attr.Job != player.GetJob() {
		return false
	}
	return true
}

func (s *EngraveSys) initAttrs() {
	data := s.getData()
	if data.Level != 0 {
		return
	}
	conf := jsondata.GetDestinedFaBaoEngraveConf()
	if conf == nil {
		return
	}
	owner := s.GetOwner()
	for _, attr := range conf.InitAttrs {
		if !s.checkAttrCanUse(owner, attr.Attr) {
			continue
		}
		data.AttrList = append(data.AttrList, &pb3.AttrSt{
			Type:  attr.Type,
			Value: attr.Value,
		})
	}
	data.Level = 1
}
func (s *EngraveSys) OnOpen() {
	s.initAttrs()
	s.ResetSysAttr(attrdef.AtDestinedFaBaoEngrave)
	s.s2cInfo()
}

func (s *EngraveSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EngraveSys) getEngraveConf(level uint32) *jsondata.DestinedFaBaoEngraveInfo {
	conf := jsondata.GetDestinedFaBaoEngraveConf()
	if conf == nil || conf.EngraveMgr == nil {
		return nil
	}
	info, ok := conf.EngraveMgr[level]
	if !ok {
		return nil
	}
	return info
}

// return count: 达到上限的数量; fullAll: 当前所有属性是否达到上限;
func (s *EngraveSys) GetReachMaxAttrNumAndMgr() (count uint32, maxAttrMgr map[uint32]*jsondata.DestinedFaBaoEngraveAttr) {
	data := s.getData()
	conf := s.getEngraveConf(data.Level)
	if conf == nil {
		return
	}

	owner := s.GetOwner()
	maxAttrMgr = make(map[uint32]*jsondata.DestinedFaBaoEngraveAttr)
	for _, attr := range conf.EngraveMaxAttrs {
		if !s.checkAttrCanUse(owner, attr.Attr) {
			continue
		}
		// 防止通过职业区分 对某条属性有更高的上限
		maxAttrMgr[attr.Type] = attr
	}

	for _, attr := range data.AttrList {
		val1 := attr.Value
		if val1 == 0 {
			continue
		}
		val2 := maxAttrMgr[attr.Type]
		if val2 == nil {
			continue
		}
		if val2.Value > val1 {
			continue
		}
		count++
	}
	return
}

func (s *EngraveSys) c2sRefreshAttrs(_ *base.Message) error {
	count, maxAttrMgr := s.GetReachMaxAttrNumAndMgr()
	if count == uint32(len(maxAttrMgr)) {
		return neterror.InternalError("all attr reach max value")
	}
	data := s.getData()
	owner := s.GetOwner()

	curLvConf := s.getEngraveConf(data.Level)
	if curLvConf == nil {
		return neterror.ConfNotFoundError("%d engrave conf not found", data.Level)
	}

	if len(curLvConf.RefreshConsume) != 0 && !owner.ConsumeByConf(curLvConf.RefreshConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoEngraveRefreshAttrs}) {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	pool := new(random.Pool)
	// 内部函数处理一下
	var doAttrStChange = func(st *pb3.AttrSt) {
		stVal := st.Value
		attr := maxAttrMgr[st.Type]
		if attr == nil || attr.Value == stVal { // 满了就不用动了
			return
		}
		value := attr.Value
		if len(attr.OptChanges) == 0 {
			return
		}

		pool.Clear()
		for _, change := range attr.OptChanges {
			pool.AddItem(change, change.Weight)
		}
		one := pool.RandomOne().(*jsondata.DestinedFaBaoEngraveAttrChange)

		switch one.Opt {
		case DestinedFaBaoEngraveAdd:
			add := one.Val
			if value > 0 && stVal+add > value {
				stVal = value
			} else {
				stVal += add
			}
		case DestinedFaBaoEngraveReduce:
			reduce := one.Val
			if reduce > stVal {
				stVal = 0
			} else {
				stVal -= reduce
			}
		case DestinedFaBaoEngraveKeep:
			//nothing
		}
		st.Value = stVal
	}

	// 变动
	for _, st := range data.AttrList {
		doAttrStChange(st)
	}

	s.SendProto3(163, 31, &pb3.S2C_163_31{
		AttrList: data.AttrList,
	})

	s.ResetSysAttr(attrdef.AtDestinedFaBaoEngrave)

	logArg, _ := json.Marshal(data)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDestinedFaBaoEngraveRefreshAttrs, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
	return nil
}

func (s *EngraveSys) c2sBreakLv(_ *base.Message) error {
	count, _ := s.GetReachMaxAttrNumAndMgr()
	data := s.getData()
	owner := s.GetOwner()
	engraveConf := s.getEngraveConf(data.Level + 1)
	if engraveConf == nil {
		return neterror.ConfNotFoundError("%d not found conf", data.Level+1)
	}
	if engraveConf.BreakCond > count {
		return neterror.InternalError("break lv need %d attrs, cur:%d", engraveConf.BreakCond, count)
	}
	if len(engraveConf.BreakConsume) != 0 && !owner.ConsumeByConf(engraveConf.BreakConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoEngraveBreakLv}) {
		owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	data.Level += 1
	s.SendProto3(163, 32, &pb3.S2C_163_32{
		Level: data.Level,
	})

	s.ResetSysAttr(attrdef.AtDestinedFaBaoEngrave)

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDestinedFaBaoEngraveBreakLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.Level),
	})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiDestinedFaBaoEngrave, func() iface.ISystem {
		return &EngraveSys{}
	})
	net.RegisterSysProtoV2(163, 31, sysdef.SiDestinedFaBaoEngrave, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EngraveSys).c2sRefreshAttrs
	})
	net.RegisterSysProtoV2(163, 32, sysdef.SiDestinedFaBaoEngrave, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*EngraveSys).c2sBreakLv
	})
	engine.RegAttrCalcFn(attrdef.AtDestinedFaBaoEngrave, calcDestinedFaBaoEngraveAttr)
}

func calcDestinedFaBaoEngraveAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDestinedFaBaoEngrave).(*EngraveSys)
	if !ok || nil == sys {
		return
	}
	data := sys.getData()
	for _, st := range data.AttrList {
		if st.Value == 0 {
			continue
		}
		calc.AddValue(st.Type, attrdef.AttrValueAlias(st.Value))
	}
}
