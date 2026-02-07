/**
 * @Author: lzp
 * @Date: 2024/6/7
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

const (
	DragonBallMeta  = 1 // 金
	DragonBallWood  = 2 // 木
	DragonBallWater = 3 // 水
	DragonBallFire  = 4 // 火
	DragonBallEarth = 5 // 土
	DragonBallHoly  = 6 // 圣
)

type DragonBallSys struct {
	Base
}

func (sys *DragonBallSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *DragonBallSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *DragonBallSys) OnOpen() {
	sys.ResetSysAttr(attrdef.SaDragonBall)
	sys.S2CInfo()
}

func (sys *DragonBallSys) S2CInfo() {
	sys.SendProto3(2, 160, &pb3.S2C_2_160{
		DragonBallData: sys.GetData(),
	})
}

func (sys *DragonBallSys) GetData() map[uint32]uint32 {
	if sys.GetBinaryData().DragonBallData == nil {
		sys.GetBinaryData().DragonBallData = make(map[uint32]uint32)
	}
	return sys.GetBinaryData().DragonBallData
}

func (sys *DragonBallSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_2_161
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if _, ok := jsondata.DragonBallConfMgr[req.Id]; !ok {
		return neterror.ParamsInvalidError("dragonBall id %d not exist", req.Id)
	}

	data := sys.GetData()
	lv := data[req.Id]
	lvConf := jsondata.GetDragonBallLvConf(req.Id, lv+1)
	if lvConf == nil {
		return neterror.ParamsInvalidError("dragonBall id %d lv %d not exist", req.Id, lv+1)
	}

	canUpgrade := true
	for idx, lvLimit := range lvConf.LvLimits {
		lv := data[uint32(idx+1)]
		if lv < lvLimit {
			canUpgrade = false
			break
		}
	}
	if !canUpgrade {
		return neterror.ParamsInvalidError("dragonBall id %d lv %d limit", req.Id, lv+1)
	}

	// 检查消耗
	consumes := lvConf.Consume1
	if req.CIdx == 2 {
		consumes = lvConf.Consume2
	}
	if !sys.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogDragonBallUpLv}) {
		return neterror.ConsumeFailedError("consume failed: %v", consumes)
	}

	data[req.Id] += 1
	newLv := data[req.Id]

	if lvConf.SkillId > 0 {
		if !sys.GetOwner().LearnSkill(lvConf.SkillId, lvConf.SkillLv, true) {
			sys.GetOwner().LogError("LearnSkill failed, skillId: %d", lvConf.SkillId)
		}
	}

	sys.SendProto3(2, 161, &pb3.S2C_2_161{
		Id: req.Id,
		Lv: newLv,
	})

	sys.ResetSysAttr(attrdef.SaDragonBall)
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogDragonBallUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Id),
		StrArgs: utils.I32toa(newLv),
	})
	return nil
}

func (sys *DragonBallSys) calcDragonBallAttr(calc *attrcalc.FightAttrCalc) {
	data := sys.GetData()

	var attrs jsondata.AttrVec
	for id, lv := range data {
		lvConf := jsondata.GetDragonBallLvConf(id, lv)
		if lvConf == nil {
			continue
		}
		attrs = append(attrs, lvConf.Attrs...)
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, attrs)
}

func calcDragonBallAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiDragonBall).(*DragonBallSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcDragonBallAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiDragonBall, func() iface.ISystem {
		return &DragonBallSys{}
	})

	net.RegisterSysProto(2, 161, sysdef.SiDragonBall, (*DragonBallSys).c2sUpLv)
	engine.RegAttrCalcFn(attrdef.SaDragonBall, calcDragonBallAttr)
}
