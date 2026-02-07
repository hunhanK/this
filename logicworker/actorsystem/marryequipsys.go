/**
 * @Author: lzp
 * @Date: 2023/12/11
 * @Desc: 结婚装备
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type MarryEquipSys struct {
	Base
	*miscitem.EquipContainer
}

func (sys *MarryEquipSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *MarryEquipSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *MarryEquipSys) ResetProp() {
	sys.ResetSysAttr(attrdef.SaMarryEquip)
	sys.ResetSysAttr(attrdef.SaMarryMateEquip)
}

func (sys *MarryEquipSys) OnInit() {
	mData := sys.GetMainData()
	itemPool := mData.ItemPool

	if itemPool == nil {
		itemPool = &pb3.ItemPool{}
		mData.ItemPool = itemPool
	}

	if itemPool.MarryEquips == nil {
		itemPool.MarryEquips = make([]*pb3.ItemSt, 0)
	}

	container := miscitem.NewEquipContainer(&mData.ItemPool.MarryEquips)

	container.TakeOnLogId = pb3.LogId_LogMarryEquipTakeOn
	container.TakeOffLogId = pb3.LogId_LogMarryEquipTakeOff
	container.AddItem = sys.owner.AddItemPtr
	container.DelItem = sys.owner.RemoveItemByHandle
	container.GetItem = sys.owner.GetItemByHandle
	container.GetBagAvailable = sys.owner.GetBagAvailableCount
	container.CheckTakeOnPosHandle = sys.CheckTakeOnPosHandle
	container.ResetProp = sys.ResetProp

	container.AfterTakeOn = sys.AfterTakeOn
	container.AfterTakeOff = sys.AfterTakeOff

	sys.EquipContainer = container

}

func (sys *MarryEquipSys) OnNewDay() {

}

// S2C
func (sys *MarryEquipSys) S2CInfo() {
	msg := &pb3.S2C_11_36{}
	if mData := sys.GetMainData(); mData != nil {
		msg.Equips = mData.ItemPool.MarryEquips
	}

	marData := sys.GetBinaryData().MarryData
	if marData != nil {
		if friendmgr.IsExistStatus(marData.CommonId, custom_id.FsMarry) {
			if equipSt, ok := manager.GetData(marData.MarryId, gshare.ActorDataEquip).(*pb3.OfflineEquipData); ok {
				msg.MEquips = equipSt.MarryEquips
			}
		}
	}

	sys.SendProto3(11, 36, msg)
}

// C2S
func (sys *MarryEquipSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_11_35
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if _, oldEquip := sys.GetEquipByPos(req.GetPos()); oldEquip != nil {
		if err := sys.Replace(req.GetHandle(), req.GetPos()); err != nil {
			return err
		}
		return nil
	}

	if err, _ := sys.TakeOn(req.GetHandle(), req.GetPos()); err != nil {
		return err
	}

	return nil
}

// CallBack
func (sys *MarryEquipSys) AfterTakeOn(equip *pb3.ItemSt) {
	sys.SendProto3(11, 35, &pb3.S2C_11_35{MEquip: equip})

	// 同步更新伴侣装备
	marData := sys.GetBinaryData().MarryData
	if marData == nil {
		return
	}
	if !friendmgr.IsExistStatus(marData.CommonId, custom_id.FsMarry) {
		return
	}
	marPlayer := manager.GetPlayerPtrById(marData.MarryId)
	if marPlayer == nil {
		return
	}

	if marSys, ok := marPlayer.GetSysObj(sysdef.SiMarryEquip).(*MarryEquipSys); ok && marSys.IsOpen() {
		marSys.ResetProp()
		marSys.S2CInfo()
	}
}

func (sys *MarryEquipSys) AfterTakeOff(equip *pb3.ItemSt, pos uint32) {
}

func (sys *MarryEquipSys) CheckTakeOnPosHandle(st *pb3.ItemSt, pos uint32) bool {
	itemId := st.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return false
	}
	if !itemdef.IsMarryEquip(itemConf.Type, itemConf.SubType) {
		return false
	}
	if itemConf.SubType != pos {
		return false
	}
	conf := jsondata.GetMarryEquipConfByStage(itemConf.Stage)
	// 检查婚戒等级
	rSys, ok := sys.owner.GetSysObj(sysdef.SiWeddingRing).(*WeddingRingSys)
	if !ok {
		return false
	}
	if rSys.GetWeddingRingLv() < conf.RingLv {
		return false
	}
	return true
}

// 本身结婚装备属性重算
func (sys *MarryEquipSys) calcMarryEquipAttr(calc *attrcalc.FightAttrCalc) {
	mData := sys.owner.GetMainData()
	stageCountMap := make(map[uint32]uint32)

	// 基础属性
	var attrs1 jsondata.AttrVec
	for _, mItem := range mData.ItemPool.MarryEquips {
		conf := jsondata.GetItemConfig(mItem.GetItemId())
		if conf == nil {
			continue
		}
		stageCountMap[conf.Stage] = 0
		attrs1 = append(attrs1, conf.StaticAttrs...)
	}
	engine.CheckAddAttrsToCalc(sys.owner, calc, attrs1)

	// 套装属性
	var attrs2 jsondata.AttrVec
	for _, mItem := range mData.ItemPool.MarryEquips {
		conf := jsondata.GetItemConfig(mItem.GetItemId())
		if conf == nil {
			continue
		}
		for stage := range stageCountMap {
			if stage <= conf.Stage {
				stageCountMap[stage]++
			}
		}
	}
	suitList := jsondata.GetMarryEquipSuitConfL()
	for _, suitConf := range suitList {
		if suitConf.MarryEquipSuitLvMap == nil {
			continue
		}
		var maxStage, min uint32
		for stage, num := range stageCountMap {
			if num >= suitConf.SuitNum && stage > maxStage {
				maxStage = stage
			}
		}
		if maxStage > 0 {
			for _, lvConf := range suitConf.MarryEquipSuitLvMap {
				if lvConf.Level <= maxStage && (min == 0 || min < lvConf.Level) {
					min = lvConf.Level
				}
			}
			// 套装属性
			suitLvConf := suitConf.MarryEquipSuitLvMap[min]
			attrs2 = append(attrs2, suitLvConf.Attrs...)
		}
	}
	engine.CheckAddAttrsToCalc(sys.owner, calc, attrs2)
}

// 伴侣结婚装备属性重算
func (sys *MarryEquipSys) calcMarryMateEquipAttr(calc *attrcalc.FightAttrCalc) {
	marData := sys.GetBinaryData().MarryData
	if marData != nil {
		if friendmgr.IsExistStatus(marData.CommonId, custom_id.FsMarry) {
			if equipSt, ok := manager.GetData(marData.MarryId, gshare.ActorDataEquip).(*pb3.OfflineEquipData); ok {
				conLv := sys.GetBinaryData().ConfessionLv
				conLvConf := jsondata.GetConfessionConf(conLv)
				if conLvConf == nil {
					return
				}

				var attrs jsondata.AttrVec
				for _, mItem := range equipSt.MarryEquips {
					conf := jsondata.GetItemConfig(mItem.GetItemId())
					if conf == nil {
						continue
					}

					attrs2 := conf.StaticAttrs.Copy()
					for i := range attrs2 {
						if attrs2[i].Job == 0 || attrs2[i].Job == sys.owner.GetJob() {
							attrValue := utils.CalcMillionRate(attrs2[i].Value, conLvConf.Ratio)
							attrs2[i].Value = attrValue
							attrs = append(attrs, attrs2[i])
						}
					}
				}
				engine.CheckAddAttrsToCalc(sys.owner, calc, attrs)
			}
		}
	}
}

func calcMarryEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiMarryEquip).(*MarryEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcMarryEquipAttr(calc)
}

func calcMarryMateEquipAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiMarryEquip).(*MarryEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcMarryMateEquipAttr(calc)
}

func onMarryEquipResetAttr(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiMarryEquip).(*MarryEquipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.ResetProp()
}

func init() {
	RegisterSysClass(sysdef.SiMarryEquip, func() iface.ISystem {
		return &MarryEquipSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaMarryEquip, calcMarryEquipAttr)
	engine.RegAttrCalcFn(attrdef.SaMarryMateEquip, calcMarryMateEquipAttr)

	net.RegisterSysProto(11, 35, sysdef.SiMarryEquip, (*MarryEquipSys).c2sTakeOn)

	event.RegActorEvent(custom_id.AeMarrySuccess, onMarryEquipResetAttr)
	event.RegActorEvent(custom_id.AeDivorce, onMarryEquipResetAttr)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		mEquipSys, ok := player.GetSysObj(sysdef.SiMarryEquip).(*MarryEquipSys)
		if !ok || !mEquipSys.IsOpen() {
			return
		}
		mEquipSys.OnNewDay()
	})
}
