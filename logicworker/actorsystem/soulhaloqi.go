/**
 * @Author: LvYuMeng
 * @Date: 2024/5/13
 * @Desc: 魂环
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/net"
)

func (s *SoulHaloSys) initExpUpLv() {
	data := s.GetData()
	s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)

	for id := range data.SoulHaloQiInfo {
		s.initExpUpLvById(id)
	}
}
func (s *SoulHaloSys) initExpUpLvById(id uint32) {
	fData := s.GetSoulHaloQiData(id)
	if fData == nil {
		return
	}

	if s.expUpLvs == nil {
		s.expUpLvs = make(map[uint32]*uplevelbase.ExpUpLv)
	}

	s.expUpLvs[id] = &uplevelbase.ExpUpLv{
		ExpLv:            fData.ExpLv,
		AttrSysId:        attrdef.SaSoulHalo,
		BehavAddExpLogId: pb3.LogId_LogSoulHaloQiUpLv,
		AfterAddExpCb:    s.afterAddExp,
		AfterUpLvCb:      s.afterLvUp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			if conf := jsondata.GetSoulHaloQiLvConf(id, lv); conf != nil {
				return conf.ExpLvConf
			}
			return nil
		},
	}
}

func (s *SoulHaloSys) afterAddExp() {
}

func (s *SoulHaloSys) afterLvUp(_ uint32) {
}

func (s *SoulHaloSys) GetSoulHaloQiData(id uint32) *pb3.SoulHaloQi {
	return s.GetData().SoulHaloQiInfo[id]
}

func (s *SoulHaloSys) initSoulHaloQi() {
	data := s.GetData()
	jsondata.EachSoulHaloQiConfig(func(config *jsondata.SoulHaloQiConfig) {
		soulHaloQi := data.SoulHaloQiInfo[config.HunQiId]
		if soulHaloQi == nil {
			soulHaloQi = &pb3.SoulHaloQi{
				Id:    config.HunQiId,
				Stage: 1,
				ExpLv: &pb3.ExpLvSt{Lv: 1, Exp: 0},
			}
			data.SoulHaloQiInfo[config.HunQiId] = soulHaloQi
		}
	})
}

func (s *SoulHaloSys) calcAttrSoulHaloQi(calc *attrcalc.FightAttrCalc) {
	var attrs jsondata.AttrVec
	for id, soulHaloQi := range s.GetData().SoulHaloQiInfo {
		lv := soulHaloQi.ExpLv.Lv
		lvConf := jsondata.GetSoulHaloQiLvConf(id, lv)
		if lvConf != nil {
			attrs = append(attrs, lvConf.Attrs...)
		}

		stage := soulHaloQi.Stage
		stageConf := jsondata.GetSoulHaloQiStageConf(id, stage)
		if stageConf != nil {
			attrs = append(attrs, stageConf.Attrs...)
		}
	}
	engine.AddAttrsToCalc(s.GetOwner(), calc, attrs)
}

func (s *SoulHaloSys) calcAttrSoulHaloQiAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	for id, soulHaloQi := range s.GetData().SoulHaloQiInfo {
		stage := soulHaloQi.Stage
		stageConf := jsondata.GetSoulHaloQiStageConf(id, stage)
		if stageConf == nil {
			continue
		}
		if stageConf.AddRate == 0 {
			continue
		}
		lv := soulHaloQi.ExpLv.Lv
		lvConf := jsondata.GetSoulHaloQiLvConf(id, lv)
		if lvConf != nil {
			engine.CheckAddAttrsRateRoundingUp(s.GetOwner(), calc, lvConf.Attrs, stageConf.AddRate)
		}
	}
}

func (s *SoulHaloSys) c2sSoulHaloQiUpLv(msg *base.Message) error {
	var req pb3.C2S_67_9
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id, itemMap := req.GetId(), req.GetItemMap()
	if itemMap == nil {
		return neterror.ParamsInvalidError("req.ItemMap == nil")
	}

	data := s.GetSoulHaloQiData(id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", id)
	}

	conf := jsondata.GetSoulHaloQiConfig(id)
	if conf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		if item == nil {
			return neterror.ParamsInvalidError("item == nil")
		}
		if !utils.SliceContainsUint32(conf.LevelUpItem, item.ItemId) {
			return neterror.ParamsInvalidError("item:%d not in LevelUpItem", item.ItemId)
		}
		if item.Count < int64(entry.Value) {
			return neterror.ParamsInvalidError("item.Count < count")
		}
	}

	addExp := uint64(0)
	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		addExp += uint64(itemConf.CommonField * entry.Value)
	}

	nextLv := data.ExpLv.Lv + 1
	lvConf := jsondata.GetSoulHaloQiLvConf(id, nextLv)
	if lvConf != nil && data.ExpLv.Exp+addExp >= lvConf.RequiredExp {
		// 检查阶级是否满足
		if data.Stage < lvConf.StageLimit {
			return neterror.ParamsInvalidError("id:%d stage limit", req.Id)
		}
	}

	for _, entry := range itemMap {
		item := s.owner.GetItemByHandle(entry.Key)
		s.owner.DeleteItemPtr(item, int64(entry.Value), pb3.LogId_LogSoulHaloQiUpLv)
	}

	err := s.expUpLvs[id].AddExp(s.GetOwner(), addExp)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.SendProto3(67, 9, &pb3.S2C_67_9{Id: id, ExpLv: s.GetSoulHaloQiData(id).ExpLv})
	return nil
}

func (s *SoulHaloSys) c2sSoulHaloQiUpStage(msg *base.Message) error {
	var req pb3.C2S_67_10
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	data := s.GetSoulHaloQiData(req.Id)
	if data == nil {
		return neterror.ParamsInvalidError("id:%d lock", req.Id)
	}

	nextStage := data.Stage + 1
	sConf := jsondata.GetSoulHaloQiStageConf(req.Id, nextStage)
	if sConf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	if data.ExpLv.Lv < sConf.LvLimit {
		return neterror.ParamsInvalidError("id:%d lv limit", req.Id)
	}

	if !s.owner.ConsumeByConf(sConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloQiUpStage}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data.Stage = nextStage
	s.ResetSysAttr(attrdef.SaSoulHalo)
	s.SendProto3(67, 10, &pb3.S2C_67_10{Id: req.Id, Stage: nextStage})
	return nil
}

func init() {
	net.RegisterSysProtoV2(67, 9, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sSoulHaloQiUpLv
	})
	net.RegisterSysProtoV2(67, 10, sysdef.SiSoulHalo, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSys).c2sSoulHaloQiUpStage
	})
}
