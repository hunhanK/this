/**
 * @Author: lzp
 * @Date: 2024/5/11
 * @Desc:
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/trialactivetype"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FaGulSys struct {
	Base
}

func (sys *FaGulSys) newTrialActiveSt() (*TrialActiveHandler, error) {
	st := &TrialActiveHandler{}

	st.DoActive = func(params *jsondata.TrialActiveParams) error {
		if len(params.Params) < 2 {
			return neterror.ParamsInvalidError("params < 2")
		}
		gulId, ringId := params.Params[0], params.Params[1]
		if sys.isRingActive(gulId, ringId) {
			return neterror.ParamsInvalidError("is active")
		}
		err := sys.ringUp(gulId, ringId, 1)
		if err != nil {
			return err
		}
		return nil
	}

	st.DoForget = func(params *jsondata.TrialActiveParams) error {
		if len(params.Params) < 2 {
			return neterror.ParamsInvalidError("params < 2")
		}
		gulId, ringId := params.Params[0], params.Params[1]
		err := sys.delRing(gulId, ringId)
		if err != nil {
			return err
		}
		return nil
	}
	return st, nil
}

func (sys *FaGulSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *FaGulSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *FaGulSys) OnOpen() {
	sys.ResetSysAttr(attrdef.SaFaGul)
	sys.S2CInfo()
}

func (sys *FaGulSys) S2CInfo() {
	sys.SendProto3(158, 20, &pb3.S2C_158_20{
		FaGulData: sys.GetData(),
	})
}

func (sys *FaGulSys) GetData() map[uint32]*pb3.FaGul {
	if sys.GetBinaryData().FaGulData == nil {
		sys.GetBinaryData().FaGulData = map[uint32]*pb3.FaGul{}
	}
	return sys.GetBinaryData().FaGulData
}

func (sys *FaGulSys) GetGulRingMinLv(gulId uint32) uint32 {
	gData := sys.GetData()[gulId]
	if gData == nil || gData.FaRing == nil {
		return 0
	}
	minLv := uint32(0)
	for id, lv := range gData.FaRing {
		if sys.owner.IsInTrialActive(trialactivetype.ActiveTypeFaGul, []uint32{gulId, id}) {
			return 0
		}
		if minLv == 0 || lv < minLv {
			minLv = lv
		}
	}
	return minLv
}

func (sys *FaGulSys) getGulData(gulId uint32) *pb3.FaGul {
	data := sys.GetData()
	gData, ok := data[gulId]
	if !ok {
		gData = &pb3.FaGul{GulId: gulId}
		data[gulId] = gData
	}

	if nil == gData.FaRing {
		gData.FaRing = map[uint32]uint32{}
	}

	return gData
}

func (sys *FaGulSys) isRingActive(gulId, ringId uint32) bool {
	data := sys.GetData()
	gData, ok := data[gulId]
	if !ok {
		return false
	}
	if nil == gData.FaRing {
		return false
	}
	return gData.FaRing[ringId] > 0
}

func (sys *FaGulSys) payRingConsume(gulId, ringId, newLv uint32) (bool, error) {
	rConf := jsondata.GetFaGulRingLvConf(gulId, ringId, newLv)
	if rConf == nil {
		return false, neterror.ConfNotFoundError("(%d, %d) fagul ring config not found", gulId, ringId)
	}

	if !sys.owner.ConsumeByConf(rConf.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogFaRingUpgrade}) {
		return false, neterror.ConsumeFailedError("consume failed: %v", rConf.Consumes)
	}

	return true, nil
}

func (sys *FaGulSys) ringUp(gulId, ringId, newLv uint32) error {
	rConf := jsondata.GetFaGulRingLvConf(gulId, ringId, newLv)
	if rConf == nil {
		return neterror.ConfNotFoundError("(%d, %d) fagul ring config not found", gulId, ringId)
	}

	gData := sys.getGulData(gulId)

	gData.FaRing[ringId] = newLv

	if rConf.SkillId > 0 {
		if !sys.GetOwner().LearnSkill(rConf.SkillId, rConf.SkillLv, true) {
			sys.LogError("LearnSkill failed, skillId: %d", rConf.SkillId)
		}
	}

	sys.afterChange(gulId, ringId, gData.FaRing[ringId])

	return nil
}

func (sys *FaGulSys) delRing(gulId, ringId uint32) error {
	gData := sys.getGulData(gulId)

	if _, ok := gData.FaRing[ringId]; !ok {
		return nil
	}

	rConf := jsondata.GetFaGulRingLvConf(gulId, ringId, gData.FaRing[ringId])
	if rConf == nil {
		return neterror.ConfNotFoundError("(%d, %d) fagul ring config not found", gulId, ringId)
	}

	delete(gData.FaRing, ringId)

	if rConf.SkillId > 0 {
		sys.GetOwner().ForgetSkill(rConf.SkillId, true, true, true)
	}

	sys.afterChange(gulId, ringId, gData.FaRing[ringId])

	return nil
}

func (sys *FaGulSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_158_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	gulId := req.GulId
	ringId := req.RingId

	data := sys.GetData()
	if data[gulId] == nil {
		data[gulId] = &pb3.FaGul{GulId: gulId}
	}
	gData := data[gulId]

	if req.RingId > 0 { // 法戒升级
		isTrial := sys.owner.IsInTrialActive(trialactivetype.ActiveTypeFaGul, []uint32{gulId, ringId})
		isActive := sys.isRingActive(gulId, ringId)
		needActive := isTrial || !isActive

		nextLv := gData.FaRing[ringId] + 1
		if needActive {
			nextLv = 1
		}

		if payOk, err := sys.payRingConsume(gulId, ringId, nextLv); !payOk {
			return err
		}

		sys.owner.StopTrialActive(trialactivetype.ActiveTypeFaGul, []uint32{gulId, ringId})

		err := sys.ringUp(gulId, ringId, nextLv)
		if err != nil {
			return err
		}

	} else { // 戒灵升级
		var newLv uint32
		gConf := jsondata.FaGulConfMgr[gulId]
		if gConf == nil || gConf.FaGulLv[gData.GulLv+1] == nil {
			return neterror.ConfNotFoundError("(%d) fagul config not found", gulId)
		}

		minLv := sys.GetGulRingMinLv(gulId)
		if gData.GulLv >= minLv {
			return neterror.ParamsInvalidError("upgrade limit: (%d >= %d)", gData.GulLv, minLv)
		}

		gData.GulLv += 1
		newLv = gData.GulLv

		gLvConf := gConf.FaGulLv[gData.GulLv]
		if gLvConf != nil && gLvConf.SkillId > 0 {
			if !sys.GetOwner().LearnSkill(gLvConf.SkillId, gLvConf.SkillLv, true) {
				sys.LogError("LearnSkill failed, skillId: %d", gLvConf.SkillId)
			}
		}
		sys.afterChange(gulId, ringId, newLv)
	}

	return nil
}

func (sys *FaGulSys) afterChange(gulId, ringId, newLv uint32) {
	sys.SendProto3(158, 21, &pb3.S2C_158_21{
		GulId:  gulId,
		RingId: ringId,
		NewLv:  newLv,
	})
	sys.ResetSysAttr(attrdef.SaFaGul)

	strByte, err := json.Marshal(map[string]interface{}{
		"GulId":  gulId,
		"RingId": ringId,
		"NewLv":  newLv,
	})
	if err != nil {
		sys.LogError("err: %v", err)
	}
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogFaRingUpgrade, &pb3.LogPlayerCounter{
		StrArgs: string(strByte),
	})
}

func (sys *FaGulSys) calcFaGulAttr(calc *attrcalc.FightAttrCalc) {
	data := sys.GetData()

	var attrs jsondata.AttrVec
	for gulId, gData := range data {
		gulLv := gData.GulLv
		gConf := jsondata.FaGulConfMgr[gulId]
		if gConf == nil {
			continue
		}
		gLvConf := gConf.FaGulLv[gulLv]
		if gLvConf == nil {
			continue
		}
		if len(gLvConf.Attrs) > 0 {
			attrs = append(attrs, gLvConf.Attrs...)
		}

		// 法戒
		for ringId, ringLv := range gData.FaRing {
			rConf := gConf.FaRing[ringId]
			if rConf == nil {
				continue
			}
			rLvConf := rConf.FaRingLv[ringLv]
			if rLvConf == nil {
				continue
			}
			if len(rLvConf.Attrs) > 0 {
				attrs = append(attrs, rLvConf.Attrs...)
			}
		}
	}
	engine.CheckAddAttrsToCalc(sys.GetOwner(), calc, attrs)
}

func calcFaGulAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFaGul).(*FaGulSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcFaGulAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiFaGul, func() iface.ISystem {
		return &FaGulSys{}
	})

	net.RegisterSysProto(158, 21, sysdef.SiFaGul, (*FaGulSys).c2sUpLv)
	engine.RegAttrCalcFn(attrdef.SaFaGul, calcFaGulAttr)
}
