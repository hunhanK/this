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

type RingsLawSys struct {
	Base
}

func (s *RingsLawSys) s2cInfo() {
	s.SendProto3(9, 110, &pb3.S2C_9_110{
		Data: s.getData(),
	})
}

func (s *RingsLawSys) getData() *pb3.RingsLawData {
	data := s.GetBinaryData().RingsLawData
	if data == nil {
		s.GetBinaryData().RingsLawData = &pb3.RingsLawData{}
		data = s.GetBinaryData().RingsLawData
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

func (s *RingsLawSys) OnReconnect() {
	s.s2cInfo()
}

func (s *RingsLawSys) OnLogin() {
	s.s2cInfo()
}

func (s *RingsLawSys) OnOpen() {
	s.s2cInfo()
	config := jsondata.GetRingsLawCommonConfig()
	if config != nil {
		s.GetOwner().TakeOnAppear(appeardef.AppearPos_RingsLaw, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_RingsLaw,
			AppearId: config.Appear.Id,
		}, true)
	}
}

func (s *RingsLawSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_9_111
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	itemMap := req.ItemMap
	if itemMap == nil || len(itemMap) == 0 {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	data := s.getData()
	config := jsondata.GetRingsLawCommonConfig()
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
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogRingsLawUpLv) {
			owner.LogError("item del failed %+v", item)
			continue
		}
		expToAdd += uint64(itemConf.CommonField * entry.Value)
	}

	data.ExpLv.Exp += expToAdd
	oldLv := data.ExpLv.Lv
	lvConf := jsondata.GetRingsLawLvConfig(data.ExpLv.Lv + 1)
	for lvConf != nil && data.ExpLv.Exp >= uint64(lvConf.ReqExp) {
		data.ExpLv.Exp -= uint64(lvConf.ReqExp)
		data.ExpLv.Lv += 1
		lvConf = jsondata.GetRingsLawLvConfig(data.ExpLv.Lv + 1)
	}

	s.SendProto3(9, 111, &pb3.S2C_9_111{
		ExpLv: data.ExpLv,
	})
	s.ResetSysAttr(attrdef.SaRingsLaw)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRingsLawUpLv, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%d_%d_%d", oldLv, data.ExpLv.Lv, expToAdd),
	})
	return nil
}
func (s *RingsLawSys) c2sUseMedicine(msg *base.Message) error {
	var req pb3.C2S_9_112
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	itemMap := req.ItemMap
	if itemMap == nil || len(itemMap) == 0 {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	config := jsondata.GetRingsLawCommonConfig()
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
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogRingsLawUseMedicine) {
			owner.LogError("item del failed %+v", item)
			continue
		}
		data.Medicine[item.ItemId].Count += entry.Value
	}
	s.SendProto3(9, 112, &pb3.S2C_9_112{
		Medicines: data.Medicine,
	})
	s.ResetSysAttr(attrdef.SaRingsLaw)
	return nil
}
func (s *RingsLawSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_9_113
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	config := jsondata.GetRingsLawCommonConfig()
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", req.Id)
	}
	owner := s.GetOwner()
	if req.Dress {
		owner.TakeOnAppear(appeardef.AppearPos_RingsLaw, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_RingsLaw,
			AppearId: config.Appear.Id,
		}, true)
	} else {
		owner.TakeOffAppear(appeardef.AppearPos_RingsLaw)
	}
	s.SendProto3(9, 113, &pb3.S2C_9_113{
		Id:    req.Id,
		Dress: req.Dress,
	})
	return nil
}
func (s *RingsLawSys) CheckFashionActive(_ uint32) bool {
	return true
}

func ringsLawProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiRingsLaw).(*RingsLawSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getData()

	var expLvAdd = func(lv uint32) {
		config := jsondata.GetRingsLawLvConfig(lv)
		if config == nil {
			return
		}
		engine.CheckAddAttrsToCalc(player, calc, config.Attrs)
	}

	var medicineAdd = func(data *pb3.RingsLawData) {
		if data == nil {
			return
		}

		config := jsondata.GetRingsLawCommonConfig()
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

func ringsLawPropertyAddRate(player iface.IPlayer, totalSysCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiRingsLaw).(*RingsLawSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getData()
	addRate := uint32(totalSysCalc.GetValue(attrdef.RingsLawBaseAttrRate))
	if data.ExpLv != nil && addRate != 0 {
		config := jsondata.GetRingsLawLvConfig(data.ExpLv.Lv)
		if config == nil {
			return
		}
		engine.CheckAddAttrsRateRoundingUp(player, calc, config.Attrs, addRate)
	}
}

func init() {
	RegisterSysClass(sysdef.SiRingsLaw, func() iface.ISystem {
		return &RingsLawSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaRingsLaw, ringsLawProperty)
	engine.RegAttrAddRateCalcFn(attrdef.SaRingsLaw, ringsLawPropertyAddRate)

	net.RegisterSysProtoV2(9, 111, sysdef.SiRingsLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RingsLawSys).c2sUpLv
	})
	net.RegisterSysProtoV2(9, 112, sysdef.SiRingsLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RingsLawSys).c2sUseMedicine
	})
	net.RegisterSysProtoV2(9, 113, sysdef.SiRingsLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RingsLawSys).c2sAppear
	})
}
