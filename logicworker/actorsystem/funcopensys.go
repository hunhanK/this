package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type FuncOpenSystem struct {
	Base
}

func (sys *FuncOpenSystem) OnInit() {
	binary := sys.owner.GetBinaryData()
	if nil == binary.FuncOpenInfo {
		binary.FuncOpenInfo = &pb3.FuncOpenInfo{
			FuncOpenStatus: make(map[uint32]uint32),
		}
	}
	if nil == binary.FuncOpenInfo.FuncOpenStatus {
		binary.FuncOpenInfo.FuncOpenStatus = make(map[uint32]uint32)
	}
}

func (sys *FuncOpenSystem) GetData() *pb3.FuncOpenInfo {
	binary := sys.GetBinaryData()
	foi := binary.GetFuncOpenInfo()
	return foi
}

func (sys *FuncOpenSystem) OnAfterLogin() {
	sys.s2cFuncOpenInfo()
}

func (sys *FuncOpenSystem) OnOpen() {
	sys.owner.TriggerEvent(custom_id.AeFuncOpenSysActive)
}

func (sys *FuncOpenSystem) OnReconnect() {
	sys.s2cFuncOpenInfo()
}

func (sys *FuncOpenSystem) s2cFuncOpenInfo() {
	binary := sys.GetBinaryData()
	sys.owner.SendProto3(12, 0, &pb3.S2C_12_0{Ids: binary.FuncOpenInfo.FuncOpenAwardList})
}

func (sys *FuncOpenSystem) c2sReceiveFuncOpenAward(msg *base.Message) error {
	var req pb3.C2S_12_0
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	binary := sys.owner.GetBinaryData()
	if nil == binary.FuncOpenInfo {
		return nil
	}
	if utils.SliceContainsUint32(binary.FuncOpenInfo.FuncOpenAwardList, req.GetId()) {
		return nil
	}
	if !sys.owner.GetSysOpen(sysdef.SiFuncOpen) {
		sys.owner.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}

	conf := jsondata.GetFuncOpenConfById(req.GetId())
	if nil == conf || nil == conf.Rewards {
		return nil
	}

	if !sys.checkFuncOpenCond(conf) {
		sys.owner.SendTipMsg(tipmsgid.TpSySNotOpen)
		return nil
	}
	rewards := make([]*jsondata.StdReward, 0, len(conf.Rewards))
	for _, awardConf := range conf.Rewards {
		rewards = append(rewards, &jsondata.StdReward{
			Id:    awardConf.Id,
			Count: awardConf.Count,
			Bind:  awardConf.Bind,
		})
	}
	binary.FuncOpenInfo.FuncOpenAwardList = append(binary.FuncOpenInfo.FuncOpenAwardList, req.GetId())

	engine.GiveRewards(sys.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFuncOpenReward})

	sys.s2cFuncOpenInfo()
	return nil
}

// 功能预告弹框通知
func notifySysOpen(player iface.IPlayer, args ...interface{}) {
	if args == nil {
		return
	}
	sysId, ok := args[0].(uint32)
	if !ok || sysId == 0 {
		return
	}
	fSys, ok := player.GetSysObj(sysdef.SiFuncOpen).(*FuncOpenSystem)
	if !ok || !fSys.IsOpen() {
		return
	}
	if !player.GetSysOpen(sysId) {
		return
	}
	if player.IsOpenNotified(sysId) {
		return
	}
	conf := jsondata.GetFuncOpenBroadcastBySysId(sysId)
	if nil != conf {
		fSys.SetSysStatus(sysId)
		player.SendProto3(12, 1, &pb3.S2C_12_1{Ids: conf.Id})
	}
}

func (sys *FuncOpenSystem) SetSysStatus(sysId uint32) {
	if sysId == 0 {
		return
	}
	idxInt := sysId / 32
	idxByte := sysId % 32

	foi := sys.GetData()
	foi.FuncOpenStatus[idxInt] = utils.SetBit(foi.FuncOpenStatus[idxInt], idxByte)
}

func (sys *FuncOpenSystem) checkFuncOpenCond(conf *jsondata.FuncOpen) bool {
	if nil == conf {
		return false
	}
	if sys.owner.GetSysOpen(conf.SysId) && sys.owner.IsOpenNotified(conf.SysId) {
		return true
	}
	return false
}

func onOpenSysEmailSend(player iface.IPlayer, args ...interface{}) {
	sysId, ok := args[0].(uint32)
	if !ok {
		return
	}
	conf := jsondata.GetSysOpenMailConf()
	if nil == conf {
		return
	}
	mailConf, ok := conf[sysId]
	if !ok {
		return
	}
	player.SendMail(&mailargs.SendMailSt{
		ConfId:  uint16(mailConf.MailId),
		Rewards: engine.FilterRewardByPlayer(player, mailConf.Rewards),
	})
}

func init() {
	RegisterSysClass(sysdef.SiFuncOpen, func() iface.ISystem {
		return &FuncOpenSystem{}
	})
	net.RegisterSysProto(12, 0, sysdef.SiFuncOpen, (*FuncOpenSystem).c2sReceiveFuncOpenAward)

	event.RegActorEvent(custom_id.AeFuncOpenNotify, notifySysOpen)
	event.RegActorEvent(custom_id.AeSysOpen, onOpenSysEmailSend)
}
