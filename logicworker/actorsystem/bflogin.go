package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type BfLoginSys struct {
	Base
}

func (sys *BfLoginSys) OnLogin() {
	sys.checkSubSysOpen()
}

func (sys *BfLoginSys) OnOpen() {
	sys.checkSubSysOpen()
	sys.S2CInfo()
}

func (sys *BfLoginSys) onSubSysOpen(subSysId uint32, entry *pb3.BenefitLoginEntry) {
	entry.TotalLoginDay = 1
	entry.IsLoginToday = true
}

func (sys *BfLoginSys) GetData() *pb3.BenefitLogin {
	binary := sys.GetBinaryData()
	if nil == binary.Benefit {
		binary.Benefit = &pb3.BenefitData{}
	}
	if nil == binary.Benefit.Login {
		binary.Benefit.Login = &pb3.BenefitLogin{}
	}
	if binary.Benefit.Login.SubSysEntry == nil {
		binary.Benefit.Login.SubSysEntry = make(map[uint32]*pb3.BenefitLoginEntry)
	}
	return binary.Benefit.Login
}

func (sys *BfLoginSys) S2CInfo() {
	sys.SendProto3(41, 10, &pb3.S2C_41_10{Login: sys.GetData()})
}

func (sys *BfLoginSys) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *BfLoginSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *BfLoginSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_41_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}

	subSysId, day := req.GetSubSysId(), req.GetId()
	conf, ok := jsondata.GetBenefitLoginConf(subSysId, day)
	if !ok {
		return neterror.ConfNotFoundError("benefit login conf is nil")
	}

	data := sys.GetData()
	if !pie.Uint32s(data.OpenSubSysIds).Contains(subSysId) {
		return neterror.SysNotExistError("sys %d not open", subSysId)
	}

	entry, ok := data.SubSysEntry[subSysId]
	if !ok {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	loginDay := entry.GetTotalLoginDay()

	if entry.TotalLoginDay < loginDay {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if utils.SliceContainsUint32(entry.LoginAward, day) {
		sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	if !engine.CheckRewards(sys.owner, conf.Award) {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}

	entry.LoginAward = append(entry.LoginAward, day)
	engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitLogin})

	sys.SendProto3(41, 1, &pb3.S2C_41_1{Id: day, SubSysId: subSysId})
	return nil
}

func (sys *BfLoginSys) allOpenSubSysDo(fn func(subSysId uint32, entry *pb3.BenefitLoginEntry)) {
	data := sys.GetData()
	for _, subSysId := range data.OpenSubSysIds {
		fn(subSysId, data.SubSysEntry[subSysId])
	}
}

func (sys *BfLoginSys) checkSubSysOpen() []uint32 {
	mgr := jsondata.BenefitConfMgr
	if mgr == nil {
		return nil
	}

	data := sys.GetData()
	sysMgr := sys.GetOwner().GetSysMgr().(*Mgr)

	openSys := map[uint32]struct{}{}
	for _, v := range data.OpenSubSysIds {
		openSys[v] = struct{}{}
	}

	var newOpenIds []uint32
	for subSysId := range mgr.NewLogin {
		if _, ok := openSys[subSysId]; ok {
			continue
		}
		open := sysMgr.canOpenSys(subSysId, nil)
		if !open {
			continue
		}
		data.OpenSubSysIds = append(data.OpenSubSysIds, subSysId)
		if _, ok := data.SubSysEntry[subSysId]; !ok {
			data.SubSysEntry[subSysId] = &pb3.BenefitLoginEntry{}
		}
		newOpenIds = append(newOpenIds, subSysId)
		sys.onSubSysOpen(subSysId, data.SubSysEntry[subSysId])
	}
	return newOpenIds
}

func onBenefitsLoginNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBenefitLogin).(*BfLoginSys)
	if !ok || !sys.IsOpen() {
		return
	}
	newOpenIds := sys.checkSubSysOpen()
	sys.allOpenSubSysDo(func(subSysId uint32, entry *pb3.BenefitLoginEntry) {
		if pie.Uint32s(newOpenIds).Contains(subSysId) {
			return
		}
		mxDay := jsondata.GetBenefitLoginMxDay(subSysId)
		if mxDay > entry.TotalLoginDay {
			entry.TotalLoginDay++
		}
		entry.IsLoginToday = true
	})
	sys.S2CInfo()
}

func init() {
	RegisterSysClass(sysdef.SiBenefitLogin, func() iface.ISystem {
		return &BfLoginSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, onBenefitsLoginNewDay)
	net.RegisterSysProto(41, 1, sysdef.SiBenefitLogin, (*BfLoginSys).c2sAward)
}
