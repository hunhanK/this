/**
 * @Author: lzp
 * @Date: 2025/4/16
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
	"jjyz/gameserver/net"
)

type CollectionSys struct {
	Base
}

func (s *CollectionSys) OnReconnect() {
	s.s2cInfo()
}

func (s *CollectionSys) OnLogin() {
	s.s2cInfo()
}

func (s *CollectionSys) OnOpen() {
	s.s2cInfo()
}

func (s *CollectionSys) GetData() map[uint32]*pb3.Collection {
	data := s.GetBinaryData().CollectionData
	if data == nil {
		data = make(map[uint32]*pb3.Collection)
	}
	return data
}

func (s *CollectionSys) getSeriesData(typ, series uint32) *pb3.CollectionSeries {
	m := s.GetBinaryData().CollectionData
	if m == nil {
		s.GetBinaryData().CollectionData = make(map[uint32]*pb3.Collection)
		m = s.GetBinaryData().CollectionData
	}

	data := m[typ]
	if data == nil {
		m[typ] = &pb3.Collection{}
		data = m[typ]
	}

	sMap := data.SeriesData
	if sMap == nil {
		data.SeriesData = make(map[uint32]*pb3.CollectionSeries)
		sMap = data.SeriesData
	}

	sData := sMap[series]
	if sData == nil {
		sMap[series] = &pb3.CollectionSeries{}
		sData = sMap[series]
	}
	return sData
}

func (s *CollectionSys) s2cInfo() {
	s.SendProto3(2, 223, &pb3.S2C_2_223{Data: s.GetData()})
}

func (s *CollectionSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for typ, tData := range data {
		for series := range tData.SeriesData {
			s.calcCollectionAttr(typ, series, calc)
		}
	}
}

func (s *CollectionSys) calcCollectionAttr(typ, series uint32, calc *attrcalc.FightAttrCalc) {
	data := s.getSeriesData(typ, series)
	lConf := jsondata.GetCollectionSeriesLvConf(typ, series, data.Lv)
	if lConf != nil {
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, lConf.Attrs)
	}

	sLvConf := jsondata.GetCollectionSeriesSuitConf(typ, series, data.SuitLv)
	if sLvConf != nil {
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, sLvConf.Attrs)
	}

	for id, stage := range data.StageData {
		sConf := jsondata.GetCollectionSeriesGoodStageConf(typ, series, id, stage)
		if sConf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, sConf.Attrs)
	}
}

func (s *CollectionSys) c2sStageUp(msg *base.Message) error {
	var req pb3.C2S_2_220
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	typ, series, id := req.Type, req.Series, req.Id
	gConf := jsondata.GetCollectionSeriesGoodConf(typ, series, id)
	if gConf == nil {
		return neterror.ConfNotFoundError("type:%d, series:%d, id:%d config not found", typ, series, id)
	}

	data := s.getSeriesData(typ, series)
	if data.StageData == nil {
		data.StageData = make(map[uint32]uint32)
	}
	stage := data.StageData[id]
	nextStage := stage + 1

	sConf := jsondata.GetCollectionSeriesGoodStageConf(typ, series, id, nextStage)
	if sConf == nil {
		return neterror.ConfNotFoundError("type:%d, series:%d, id:%d, stage:%d config not found", typ, series, id, nextStage)
	}

	if len(sConf.Consume) <= 0 {
		return neterror.ConfNotFoundError("consume empty")
	}

	var consumes jsondata.ConsumeVec
	consume := sConf.Consume[0]
	count := s.GetOwner().GetItemCount(consume.Id, -1)
	var consumeCount = consume.Count
	if uint32(count) < consume.Count {
		consumeCount = uint32(count)
	}
	consumes = append(consumes, &jsondata.Consume{
		Id:    consume.Id,
		Count: consumeCount,
	})

	diff := int64(consume.Count) - count
	if diff > 0 {
		ratio := jsondata.GetCommonConf("collectionRatio").U32
		consumes = append(consumes, &jsondata.Consume{
			Id:    gConf.ItemId,
			Count: uint32(diff) * ratio,
		})
	}

	if len(consumes) > 0 && !s.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogCollectionStageUp}) {
		return neterror.ParamsInvalidError("consume error")
	}

	data.StageData[id] = nextStage
	s.SendProto3(2, 220, &pb3.S2C_2_220{
		Type:   typ,
		Series: series,
		Id:     id,
		Stage:  nextStage,
	})
	s.ResetSysAttr(attrdef.SaCollection)
	return nil
}

func (s *CollectionSys) c2sStrengthen(msg *base.Message) error {
	var req pb3.C2S_2_221
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getSeriesData(req.Type, req.Series)
	nextLv := data.Lv + 1
	lConf := jsondata.GetCollectionSeriesLvConf(req.Type, req.Series, nextLv)
	if lConf == nil {
		return neterror.ConfNotFoundError("type:%d, series:%d, lv:%d config not found", req.Type, req.Series, nextLv)
	}

	if !s.GetOwner().ConsumeByConf(lConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCollectionLevelUp}) {
		return neterror.ParamsInvalidError("consume error")
	}
	data.Lv = nextLv
	s.SendProto3(2, 221, &pb3.S2C_2_221{
		Type:   req.Type,
		Series: req.Series,
		Lv:     nextLv,
	})
	s.ResetSysAttr(attrdef.SaCollection)
	return nil
}

func (s *CollectionSys) c2sSuitStrengthen(msg *base.Message) error {
	var req pb3.C2S_2_222
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getSeriesData(req.Type, req.Series)
	nextSuitLv := data.SuitLv + 1
	sLvConf := jsondata.GetCollectionSeriesSuitConf(req.Type, req.Series, nextSuitLv)
	if sLvConf == nil {
		return neterror.ConfNotFoundError("type:%d, series:%d, suitLv:%d config not found", req.Type, req.Series, nextSuitLv)
	}

	var count uint32
	for _, stage := range data.StageData {
		if stage >= sLvConf.StageLimit {
			count += 1
		}
	}
	if count < sLvConf.Count {
		return neterror.ParamsInvalidError("type:%d, series:%d, suitLv:%d limit", req.Type, req.Series, nextSuitLv)
	}

	data.SuitLv = nextSuitLv
	s.SendProto3(2, 222, &pb3.S2C_2_222{
		Type:   req.Type,
		Series: req.Series,
		SuitLv: nextSuitLv,
	})
	s.ResetSysAttr(attrdef.SaCollection)
	return nil
}

func calcCollectionAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiCollection).(*CollectionSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.calcAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiCollection, func() iface.ISystem {
		return &CollectionSys{}
	})
	net.RegisterSysProto(2, 220, sysdef.SiCollection, (*CollectionSys).c2sStageUp)
	net.RegisterSysProto(2, 221, sysdef.SiCollection, (*CollectionSys).c2sStrengthen)
	net.RegisterSysProto(2, 222, sysdef.SiCollection, (*CollectionSys).c2sSuitStrengthen)

	engine.RegAttrCalcFn(attrdef.SaCollection, calcCollectionAttr)
}
