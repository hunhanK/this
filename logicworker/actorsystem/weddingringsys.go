/**
 * @Author: lzp
 * @Date: 2023/12/5
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/net"
)

type WeddingRingSys struct {
	Base
}

func (sys *WeddingRingSys) OnReconnect() {
	sys.S2CRingInfo()
}

func (sys *WeddingRingSys) OnLogin() {
	sys.owner.SetExtraAttr(attrdef.MRingLv, int64(sys.GetWeddingRingLv()))
	sys.S2CRingInfo()
}

func (sys *WeddingRingSys) OnOpen() {
	sys.ResetSysAttr(attrdef.SaWeddingRing)
	sys.S2CRingInfo()
}

func (sys *WeddingRingSys) S2CRingInfo() {
	msg := &pb3.S2C_11_31{
		Data: &pb3.WeddingRingData{ExpLv: sys.GetData().ExpLv},
	}

	sys.SendProto3(11, 31, msg)
}

// 升级替换婚戒
func (sys *WeddingRingSys) c2sUpgrade(msg *base.Message) error {
	var req pb3.C2S_11_30
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("msg.UnPackPb3Msg err: %v", err)
	}

	conf := jsondata.GetWeddingRingConf()
	if conf == nil || conf.LevelUpItem == nil {
		return neterror.ParamsInvalidError("lvConf == nil")
	}

	data := sys.GetData()
	nextLvConf := jsondata.GetWeddingRingLvConf(data.ExpLv.Lv + 1)
	if nextLvConf == nil {
		return neterror.ParamsInvalidError("lvConf == nil")
	}

	if equipSys, ok := sys.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip := equipSys.GetEquipByPos(itemdef.EtWeddingRingPos)
		if equip == nil {
			return neterror.ParamsInvalidError("no weddingRing equip in pos=%d", itemdef.EtWeddingRingPos)
		}
	}

	lvConf := jsondata.GetWeddingRingLvConf(data.ExpLv.Lv)
	if lvConf != nil && !sys.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogWeddingRingLvUp}) {
		return neterror.ParamsInvalidError("weddingRing upgrade consume error")
	}

	// 升级
	data.ExpLv.Lv += 1

	// 替换装备
	newItemId := jsondata.GetWeddingRingLvConf(data.ExpLv.Lv).EquipId
	if equipSys, ok := sys.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem); ok {
		_, equip := equipSys.GetEquipByPos(itemdef.EtWeddingRingPos)
		if equip != nil && equip.ItemId != newItemId {
			equip.ItemId = newItemId
			mainData := sys.GetMainData()
			mainData.ItemPool.Equips = append(mainData.ItemPool.Equips, equip)

			sys.SendProto3(11, 1, &pb3.S2C_11_1{
				Ret:   custom_id.Success,
				Equip: equip,
			})
		}
	}

	sys.ResetSysAttr(attrdef.SaWeddingRing)
	sys.owner.SetExtraAttr(attrdef.MRingLv, int64(data.ExpLv.Lv))

	sys.owner.SendProto3(11, 30, &pb3.S2C_11_30{
		ExpLv: data.ExpLv,
	})
	return nil
}

func (sys *WeddingRingSys) GetData() *pb3.WeddingRingData {
	binData := sys.GetBinaryData()
	if binData.RingData == nil {
		binData.RingData = &pb3.WeddingRingData{
			ExpLv: &pb3.ExpLvSt{},
		}
	}

	ringData := binData.RingData
	if ringData.ExpLv == nil {
		ringData.ExpLv = &pb3.ExpLvSt{
			Lv: 0,
		}
	}

	return ringData
}

func (sys *WeddingRingSys) GetWeddingRingLv() uint32 {
	data := sys.GetData()
	return data.ExpLv.Lv
}

func (sys *WeddingRingSys) calcWeddingRingAttr(calc *attrcalc.FightAttrCalc) {
	conf := jsondata.GetWeddingRingLvConf(sys.GetData().ExpLv.Lv)
	if conf == nil {
		return
	}
	var attrL []*jsondata.Attr
	attrL = append(attrL, conf.CommonAttrs...)

	marData := sys.GetBinaryData().MarryData
	if marData != nil {
		if friendmgr.IsExistStatus(marData.CommonId, custom_id.FsMarry) {
			attrL = append(attrL, conf.ExtraAttrs...)
		}
	}
	engine.CheckAddAttrsToCalc(sys.owner, calc, attrL)
}

func calcWeddingRingAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiWeddingRing).(*WeddingRingSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcWeddingRingAttr(calc)
}

func onMarry(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiWeddingRing).(*WeddingRingSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.ResetSysAttr(attrdef.SaWeddingRing)
}

func onDivorce(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiWeddingRing).(*WeddingRingSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.ResetSysAttr(attrdef.SaWeddingRing)
}

func init() {
	RegisterSysClass(sysdef.SiWeddingRing, func() iface.ISystem {
		return &WeddingRingSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaWeddingRing, calcWeddingRingAttr)

	net.RegisterSysProto(11, 30, sysdef.SiWeddingRing, (*WeddingRingSys).c2sUpgrade)
	event.RegActorEvent(custom_id.AeMarrySuccess, onMarry)
	event.RegActorEvent(custom_id.AeDivorce, onDivorce)
}
