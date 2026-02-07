package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type VipBossSys struct {
	Base
	*miscitem.BossTipContainer
}

func (sys *VipBossSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.RemindVipBossId {
		binary.RemindVipBossId = make([]uint32, 0)
	}
	sys.BossTipContainer = miscitem.NewBossTipContainer(&binary.RemindVipBossId)
}

func (sys *VipBossSys) OnAfterLogin() {
	sys.SendRareInfo()
	sys.SendRemind()
}

func (sys *VipBossSys) OnReconnect() {
	sys.SendRareInfo()
	sys.SendRemind()
}

func (sys *VipBossSys) OnOpen() {
	sys.SendRareInfo()
	sys.SendRemind()
}

func (sys *VipBossSys) SendRareInfo() {
	if localReq, err := base.MakeMessage(17, 163, &pb3.C2S_17_163{}); nil == err {
		sys.owner.DoNetMsg(17, 163, localReq)
	} else {
		sys.LogError("本服vipboss信息发送出错:%v", err)
	}
	if crossReq, err := base.MakeMessage(17, 166, &pb3.C2S_17_166{}); nil == err {
		sys.owner.DoNetMsg(17, 166, crossReq)
	} else {
		sys.LogError("跨服vipboss信息发送出错:%v", err)
	}
}

func (sys *VipBossSys) SendRemind() {
	binary := sys.GetBinaryData()
	sys.owner.SendProto3(17, 162, &pb3.S2C_17_162{BossIds: binary.GetRemindVipBossId()})
}

func (sys *VipBossSys) c2sRemindRareBoss(msg *base.Message) error {
	req := &pb3.C2S_17_150{}
	if err := msg.UnPackPb3Msg(req); nil != err {
		return err
	}
	if !jsondata.IsRareVipBoss(req.GetBossId()) {
		return neterror.ParamsInvalidError("boss(%d) not rare vip boss type", req.GetBossId())
	}
	sys.ChangeTip(req.GetBossId(), req.GetNeed())
	sys.SendRemind()
	return nil
}

// 请求进入副本
func (sys *VipBossSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_17_160
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	conf := jsondata.GetVipBossConfByScene(req.GetSceneId())
	if nil == conf {
		return neterror.ParamsInvalidError("vip boss conf(%d) is nil", req.GetSceneId())
	}
	if conf.Level > 0 && conf.Level > sys.owner.GetLevel() {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	if conf.Circle > 0 && conf.Circle > sys.owner.GetCircle() {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	if conf.Vip > 0 && conf.Vip > sys.owner.GetVipLevel() {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	if conf.IsCross > 0 {
		sys.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterCrossVipBoss, &pb3.CommonSt{U32Param: req.GetSceneId()})
	} else {
		sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterVipBoss, &pb3.CommonSt{U32Param: req.GetSceneId()})
	}
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiVipBoss, func() iface.ISystem {
		return &VipBossSys{}
	})

	net.RegisterSysProto(17, 160, sysdef.SiVipBoss, (*VipBossSys).c2sEnterFb)
	net.RegisterSysProto(17, 162, sysdef.SiVipBoss, (*VipBossSys).c2sRemindRareBoss)
}
