/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type FootprintsSys struct {
	Base
}

func (s *FootprintsSys) s2cInfo() {
	s.SendProto3(9, 130, &pb3.S2C_9_130{
		Data: s.getData(),
	})
}

func (s *FootprintsSys) getData() *pb3.FootprintsData {
	data := s.GetBinaryData().FootprintsData
	if data == nil {
		s.GetBinaryData().FootprintsData = &pb3.FootprintsData{}
		data = s.GetBinaryData().FootprintsData
	}
	if data.Medicine == nil {
		data.Medicine = make(map[uint32]*pb3.UseCounter)
	}
	if data.ExpLv == nil {
		data.ExpLv = &pb3.ExpLvSt{}
	}
	if data.ExpLv.Lv == 0 {
		data.ExpLv.Lv = 1
	}
	return data
}

func (s *FootprintsSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FootprintsSys) OnLogin() {
	s.s2cInfo()
}

func (s *FootprintsSys) OnOpen() {
	s.s2cInfo()
	config := jsondata.GetFootprintsCommonConfig()
	if config != nil {
		s.GetOwner().TakeOnAppear(appeardef.AppearPos_Footprints, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_Footprints,
			AppearId: config.Appear.Id,
		}, true)
	}
}

func (s *FootprintsSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_9_131
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	itemMap := req.ItemMap
	if itemMap == nil || len(itemMap) == 0 {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	data := s.getData()
	config := jsondata.GetFootprintsCommonConfig()
	if config == nil {
		return neterror.ConfNotFoundError("not found config")
	}

	levelUpItem := pie.Uint32s(config.LevelUpItem)
	owner := s.GetOwner()
	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !levelUpItem.Contains(item.ItemId) {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count %d < count %d", item.Count, entry.Value)
		}
	}

	expToAdd := uint64(0)
	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogFootprintsUpLv) {
			owner.LogError("item del failed %+v", item)
			continue
		}
		expToAdd += uint64(itemConf.CommonField * entry.Value)
	}

	data.ExpLv.Exp += expToAdd
	oldLv := data.ExpLv.Lv
	lvConf := jsondata.GetFootprintsLvConfig(data.ExpLv.Lv + 1)
	for lvConf != nil && data.ExpLv.Exp >= uint64(lvConf.ReqExp) {
		data.ExpLv.Exp -= uint64(lvConf.ReqExp)
		data.ExpLv.Lv += 1
		lvConf = jsondata.GetFootprintsLvConfig(data.ExpLv.Lv + 1)
	}

	s.SendProto3(9, 131, &pb3.S2C_9_131{
		ExpLv: data.ExpLv,
	})
	s.ResetSysAttr(attrdef.SaFootprints)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFootprintsUpLv, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%d_%d_%d", oldLv, data.ExpLv.Lv, expToAdd),
	})

	return nil
}

func (s *FootprintsSys) c2sUseMedicine(msg *base.Message) error {
	var req pb3.C2S_9_132
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	itemMap := req.ItemMap
	if itemMap == nil || len(itemMap) == 0 {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	config := jsondata.GetFootprintsCommonConfig()
	if config == nil {
		return neterror.ConfNotFoundError("not found config")
	}

	if config.Medicine == nil {
		return neterror.ParamsInvalidError("not found medicine")
	}

	owner := s.GetOwner()
	data := s.getData()
	var itemCountMap = make(map[uint32]uint32)
	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if _, ok := config.Medicine[item.ItemId]; !ok {
			return neterror.ParamsInvalidError("item not in levelUpItem %d", item.ItemId)
		}
		if uint32(item.Count) < entry.Value {
			return neterror.ParamsInvalidError("item.Count %d < count %d", item.Count, entry.Value)
		}
		itemCountMap[item.ItemId] += entry.Value
	}

	for itemId, count := range itemCountMap {
		medicineConf := config.Medicine[itemId]
		var limitConf *jsondata.MedicineUseLimit
		for _, mul := range medicineConf.UseLimit {
			if owner.GetLevel() <= mul.LevelLimit {
				limitConf = mul
				break
			}
		}

		if limitConf == nil && len(medicineConf.UseLimit) > 0 {
			limitConf = medicineConf.UseLimit[len(medicineConf.UseLimit)-1]
		}

		medicine, ok := data.Medicine[itemId]
		if !ok {
			medicine = &pb3.UseCounter{}
			medicine.Id = itemId
			data.Medicine[itemId] = medicine
		}

		if medicine.Count+count > limitConf.Limit {
			return neterror.ParamsInvalidError("useMedicine failed, medicine.Count >= limitConf.Limit, medicine.Count: %d, limitConf.Limit: %d", medicine.Count, limitConf.Limit)
		}
	}

	for _, entry := range itemMap {
		item := owner.GetItemByHandle(entry.Key)
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogFootprintsUseMedicine) {
			owner.LogError("item del failed %+v", item)
			continue
		}
		data.Medicine[item.ItemId].Count += entry.Value
	}
	s.SendProto3(9, 132, &pb3.S2C_9_132{
		Medicines: data.Medicine,
	})
	s.ResetSysAttr(attrdef.SaFootprints)
	return nil
}

func (s *FootprintsSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_9_133
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	config := jsondata.GetFootprintsCommonConfig()
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", req.Id)
	}
	owner := s.GetOwner()
	if req.Dress {
		owner.TakeOnAppear(appeardef.AppearPos_Footprints, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_Footprints,
			AppearId: config.Appear.Id,
		}, true)
	} else {
		owner.TakeOffAppear(appeardef.AppearPos_Footprints)
	}
	s.SendProto3(9, 133, &pb3.S2C_9_133{
		Id:    req.Id,
		Dress: req.Dress,
	})
	return nil
}

func (s *FootprintsSys) CheckFashionActive(_ uint32) bool {
	return true
}

func handleFootprints(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFootprints).(*FootprintsSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getData()

	var expLvAdd = func(lv uint32) {
		config := jsondata.GetFootprintsLvConfig(lv)
		if config == nil {
			return
		}
		engine.CheckAddAttrsToCalc(player, calc, config.Attrs)
	}

	var medicineAdd = func(data *pb3.FootprintsData) {
		if data == nil {
			return
		}

		config := jsondata.GetFootprintsCommonConfig()
		if config == nil || config.Medicine == nil {
			return
		}

		for _, medicine := range data.Medicine {
			medicineConf := config.Medicine[medicine.Id]
			if medicineConf == nil {
				continue
			}
			if medicine.Count == 0 {
				continue
			}
			// 计算丹药的固定数值加成
			engine.CheckAddAttrsTimes(player, calc, medicineConf.Attrs, medicine.Count)
			if len(medicineConf.RateAttrs) > 0 {
				engine.CheckAddAttrsTimes(player, calc, medicineConf.RateAttrs, medicine.Count)
			}
		}
	}

	medicineAdd(data)
	if data.ExpLv != nil {
		expLvAdd(data.ExpLv.Lv)
	}
}

func handleFootprintsAddRate(player iface.IPlayer, totalSysCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFootprints).(*FootprintsSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getData()
	addRate := uint32(totalSysCalc.GetValue(attrdef.FootprintsBaseAttrRate))
	if data.ExpLv != nil && addRate != 0 {
		config := jsondata.GetFootprintsLvConfig(data.ExpLv.Lv)
		if config == nil {
			return
		}
		engine.CheckAddAttrsRateRoundingUp(player, calc, config.Attrs, addRate)
	}
}

func init() {
	RegisterSysClass(sysdef.SiFootprints, func() iface.ISystem {
		return &FootprintsSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFootprints, handleFootprints)
	engine.RegAttrAddRateCalcFn(attrdef.SaFootprints, handleFootprintsAddRate)
	net.RegisterSysProtoV2(9, 131, sysdef.SiFootprints, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsSys).c2sUpLv
	})
	net.RegisterSysProtoV2(9, 132, sysdef.SiFootprints, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsSys).c2sUseMedicine
	})
	net.RegisterSysProtoV2(9, 133, sysdef.SiFootprints, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsSys).c2sAppear
	})
}
