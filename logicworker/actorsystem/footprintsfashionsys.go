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
	"jjyz/base/common"
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

type FootprintsFashionSys struct {
	Base
}

func (s *FootprintsFashionSys) s2cInfo() {
	s.SendProto3(9, 140, &pb3.S2C_9_140{
		Data: s.getData(),
	})
}

func (s *FootprintsFashionSys) getData() *pb3.FootprintsFashionData {
	data := s.GetBinaryData().FootprintsFashionData
	if data == nil {
		s.GetBinaryData().FootprintsFashionData = &pb3.FootprintsFashionData{}
		data = s.GetBinaryData().FootprintsFashionData
	}
	if data.FashionData == nil {
		data.FashionData = make(map[uint32]*pb3.FootprintsFashion)
	}
	return data
}

func (s *FootprintsFashionSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FootprintsFashionSys) OnLogin() {
	s.s2cInfo()
}

func (s *FootprintsFashionSys) OnOpen() {
	s.s2cInfo()
}

func (s *FootprintsFashionSys) getFashionData(id uint32) *pb3.FootprintsFashion {
	data := s.getData()
	fashion := data.FashionData[id]
	if fashion == nil {
		return nil
	}
	if fashion.ExpLv == nil {
		fashion.ExpLv = &pb3.ExpLvSt{}
	}
	if fashion.ExpLv.Lv == 0 {
		fashion.ExpLv.Lv = 1
	}
	return fashion
}

func (s *FootprintsFashionSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_9_141
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	itemMap := req.ItemMap
	id := req.Id
	if itemMap == nil || len(itemMap) == 0 {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	fashionData := s.getFashionData(id)
	if fashionData == nil {
		return neterror.ParamsInvalidError("%d fashionData not active", id)
	}

	expUpLv := fashionData.ExpLv
	config := jsondata.GetFootprintsFashionConfig(id)
	lvConf := jsondata.GetFootprintsFashionLvConfig(id, expUpLv.Lv+1)
	if lvConf == nil || config == nil {
		return neterror.ConfNotFoundError("%d not found lv %d config", id, expUpLv.Lv+1)
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
		if !owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogFootprintsFashionUpLv) {
			owner.LogError("item del failed %+v", item)
			continue
		}
		expToAdd += uint64(itemConf.CommonField * entry.Value)
	}

	expUpLv.Exp += expToAdd
	oldLv := expUpLv.Lv
	for lvConf != nil && expUpLv.Exp >= uint64(lvConf.ReqExp) {
		expUpLv.Exp -= uint64(lvConf.ReqExp)
		expUpLv.Lv += 1
		lvConf = jsondata.GetFootprintsFashionLvConfig(id, expUpLv.Lv+1)
	}

	s.SendProto3(9, 141, &pb3.S2C_9_141{
		Id:    id,
		ExpLv: expUpLv,
	})
	s.ResetSysAttr(attrdef.SaFootprintsFashion)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFootprintsFashionUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d_%d_%d", oldLv, expUpLv.Lv, expToAdd),
	})
	return nil
}

func (s *FootprintsFashionSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_9_143
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.getFashionData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("%d not found fashion data", req.Id)
	}
	config := jsondata.GetFootprintsFashionConfig(req.Id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", req.Id)
	}
	owner := s.GetOwner()
	if req.Dress {
		owner.TakeOnAppear(appeardef.AppearPos_Footprints, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_FootprintsFashion,
			AppearId: config.Id,
		}, true)
	} else {
		owner.TakeOffAppear(appeardef.AppearPos_Footprints)
	}
	s.SendProto3(9, 143, &pb3.S2C_9_143{
		Id:    req.Id,
		Dress: req.Dress,
	})
	return nil
}

func (s *FootprintsFashionSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_9_144
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	id := req.Id
	fashionData := s.getFashionData(id)
	if fashionData == nil {
		return neterror.ParamsInvalidError("%d fashionData not active", id)
	}

	nextStage := fashionData.Stage + 1
	config := jsondata.GetFootprintsFashionStageConfig(id, nextStage)
	if config == nil {
		return neterror.ParamsInvalidError("%d not found next stage %d config", id, nextStage)
	}

	owner := s.GetOwner()
	if len(config.Consume) == 0 || !owner.ConsumeByConf(config.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFootprintsFashionUpStage}) {
		return neterror.ConsumeFailedError("useMedicine failed, consume failed")
	}
	fashionData.Stage = nextStage
	s.SendProto3(9, 144, &pb3.S2C_9_144{
		Id:    id,
		Stage: fashionData.Stage,
	})
	if config.SkillId != 0 && config.SkillLv != 0 {
		owner.LearnSkill(config.SkillId, config.SkillLv, true)
	}
	s.ResetSysAttr(attrdef.SaFootprintsFashion)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFootprintsFashionUpStage, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d", nextStage),
	})
	return nil
}

func (s *FootprintsFashionSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_9_145
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	id := req.Id
	config := jsondata.GetFootprintsFashionConfig(id)
	if config == nil {
		return neterror.ConfNotFoundError("%d not found config", id)
	}
	data := s.getData()
	fashion := s.getFashionData(id)
	if fashion != nil {
		return neterror.ParamsInvalidError("%d fashion already active", id)
	}
	if len(config.ActiveConsume) == 0 || !s.GetOwner().ConsumeByConf(config.ActiveConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogFootprintsFashionActive}) {
		return neterror.ConfNotFoundError("active %d consume failed", id)
	}
	data.FashionData[id] = &pb3.FootprintsFashion{Id: id}
	fashion = s.getFashionData(id)
	s.SendProto3(9, 145, &pb3.S2C_9_145{
		Fashion: fashion,
	})
	stageConfig := jsondata.GetFootprintsFashionStageConfig(fashion.Id, fashion.Stage)
	if stageConfig != nil && stageConfig.SkillId != 0 && stageConfig.SkillLv != 0 {
		s.GetOwner().LearnSkill(stageConfig.SkillId, stageConfig.SkillLv, true)
	}
	s.ResetSysAttr(attrdef.SaFootprintsFashion)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFootprintsFashionActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	return nil
}

func (s *FootprintsFashionSys) CheckFashionActive(fashionId uint32) bool {
	data := s.getData()
	_, ok := data.FashionData[fashionId]
	if !ok {
		return false
	}
	return true
}

func handleFootprintsFashion(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFootprintsFashion).(*FootprintsFashionSys)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getData()

	var expLvAdd = func(id uint32, lv uint32) {
		config := jsondata.GetFootprintsFashionLvConfig(id, lv)
		if config == nil {
			return
		}
		engine.CheckAddAttrsToCalc(player, calc, config.Attrs)
	}

	var stageAdd = func(id uint32, stage uint32) {
		config := jsondata.GetFootprintsFashionStageConfig(id, stage)
		if config == nil {
			return
		}
		engine.CheckAddAttrsToCalc(player, calc, config.Attrs)
	}

	for _, fashion := range data.FashionData {
		stageAdd(fashion.Id, fashion.Stage)
		if fashion.ExpLv != nil {
			expLvAdd(fashion.Id, fashion.ExpLv.Lv)
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiFootprintsFashion, func() iface.ISystem {
		return &FootprintsFashionSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaFootprintsFashion, handleFootprintsFashion)
	net.RegisterSysProtoV2(9, 141, sysdef.SiFootprintsFashion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsFashionSys).c2sUpLv
	})
	net.RegisterSysProtoV2(9, 143, sysdef.SiFootprintsFashion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsFashionSys).c2sAppear
	})
	net.RegisterSysProtoV2(9, 144, sysdef.SiFootprintsFashion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsFashionSys).c2sUpStage
	})
	net.RegisterSysProtoV2(9, 145, sysdef.SiFootprintsFashion, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FootprintsFashionSys).c2sActive
	})
}
