package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/fuben/qimenmgr"
	"jjyz/gameserver/net"
)

type PrivilegeBossSys struct {
	Base
}

func (sys *PrivilegeBossSys) getData() map[uint32]uint32 {
	if nil == sys.GetBinaryData().PrivilegeBoss {
		sys.GetBinaryData().PrivilegeBoss = make(map[uint32]uint32)
	}
	return sys.GetBinaryData().PrivilegeBoss
}

func (sys *PrivilegeBossSys) s2cInfo() {
	sys.SendProto3(17, 201, &pb3.S2C_17_201{
		PrivilegeBoss: sys.getData(),
		BossIds:       sys.GetBinaryData().PrivilegeBossKillIds,
	})
}

func (sys *PrivilegeBossSys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *PrivilegeBossSys) OnReconnect() {
	sys.s2cInfo()
}

const (
	priboss_vip   = 1
	priboss_week  = 2
	priboss_month = 3
)

func (sys *PrivilegeBossSys) checkEnterCond(conf *jsondata.QiMenPrivilegeConf) bool {
	if len(conf.PriCond) == 0 {
		return true
	}
	switch conf.PriCond[0] {
	case priboss_vip:
		return len(conf.PriCond) >= 2 && sys.GetBinaryData().Vip >= conf.PriCond[1]
	case priboss_week:
		if pvcardSys, ok := sys.GetOwner().GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys); ok && pvcardSys.IsOpen() {
			if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Week) {
				return true
			}
		}
	case priboss_month:
		if pvcardSys, ok := sys.GetOwner().GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys); ok && pvcardSys.IsOpen() {
			if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
				return true
			}
		}
	}
	return false
}

func (sys *PrivilegeBossSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_17_200
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	sceneId := req.GetSceneId()
	typeConf := qimenmgr.GetCurTypeConf()
	if nil == typeConf {
		return neterror.ConfNotFoundError("no qimen open conf now")
	}

	conf := typeConf.PrivilegeConf[sceneId]
	if nil == conf {
		return neterror.ConfNotFoundError("no privilege boss conf(%d)", sceneId)
	}
	if !sys.checkEnterCond(conf) {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}
	data := sys.getData()
	times := data[sceneId]
	if times >= conf.ChangeTimes {
		sys.owner.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}

	isPass := pie.Uint32s(sys.GetBinaryData().PrivilegeBossKillIds).Contains(conf.BossId)
	if !isPass {
		err = sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterPrivilegeBoss, &pb3.CommonSt{
			U32Param:  qimenmgr.GetQiMenInfo().QiMenType,
			U32Param2: sceneId,
		})
		if nil != err {
			return err
		}
	} else {
		err = sys.owner.CallActorFunc(actorfuncid.G2FPrivilegeBossQuickAttack, &pb3.CommonSt{
			U32Param:  conf.Id,
			U32Param2: conf.BossId,
		})
		if nil != err {
			return err
		}
		data[sceneId]++
		sys.owner.FinishBossQuickAttack(conf.BossId, sceneId, jsondata.GlobalUint("tequanbossfbid"))
		sys.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionKillQimenMonster, 1)
		sys.s2cInfo()
	}

	return nil
}

func c2sEnterFb(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		return sys.(*PrivilegeBossSys).c2sEnterFb(msg)
	}
}

func (sys *PrivilegeBossSys) onNewDay() {
	sys.GetBinaryData().PrivilegeBoss = make(map[uint32]uint32)
	sys.s2cInfo()
}

func onPassPrivilegeBossFb(actor iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if err := pb3.Unmarshal(buf, &st); nil != err {
		actor.LogError("onActorCallAddPlayer error:%v", err)
		return
	}

	sceneId, monId := st.U32Param, st.U32Param2
	if sys, ok := actor.GetSysObj(sysdef.SiPrivilegeBoss).(*PrivilegeBossSys); ok {
		data := sys.getData()
		data[sceneId]++
		sys.GetBinaryData().PrivilegeBossKillIds = append(sys.GetBinaryData().PrivilegeBossKillIds, monId)
		sys.s2cInfo()
	}
}

func init() {
	RegisterSysClass(sysdef.SiPrivilegeBoss, func() iface.ISystem {
		return &PrivilegeBossSys{}
	})

	net.RegisterSysProtoV2(17, 200, sysdef.SiPrivilegeBoss, c2sEnterFb)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiPrivilegeBoss).(*PrivilegeBossSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	engine.RegisterActorCallFunc(playerfuncid.PassPrivilegeBossFb, onPassPrivilegeBossFb)
}
