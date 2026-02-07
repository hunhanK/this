/**
 * @Author: lzp
 * @Date: 2025/12/22
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type UniqueCustomSys struct {
	Base
}

func (s *UniqueCustomSys) OnReconnect() {
	s.s2cInfo()
}

func (s *UniqueCustomSys) OnOpen() {
	s.s2cInfo()
}

func (s *UniqueCustomSys) OnLogin() {
	s.updateLeftCount()
	s.updateSet()
	s.s2cInfo()
}

func (s *UniqueCustomSys) OnNewDay() {
	data := s.GetData()
	data.UsedCount = 0
	s.updateLeftCount()
	s.s2cInfo()
}

func (s *UniqueCustomSys) s2cInfo() {
	s.SendProto3(13, 25, &pb3.S2C_13_25{Data: s.GetData()})
}

func (s *UniqueCustomSys) s2cFashion(setId uint32, fashionId uint32) {
	setData := s.getSetData(setId)
	fData, ok := setData.Fashions[fashionId]
	if !ok {
		return
	}
	s.SendProto3(13, 23, &pb3.S2C_13_23{
		SetId: setId,
		Data:  fData,
	})
}

func (s *UniqueCustomSys) updateLeftCount() {
	commonConf := jsondata.UniqueCustomCommonConfMgr
	if commonConf == nil {
		return
	}

	data := s.GetData()
	if data.ActiveSetNum >= commonConf.ActiveSetNum {
		leftCount := commonConf.DayLimit - data.UsedCount
		s.owner.SetExtraAttr(attrdef.UniqueCustomLeftCount, attrdef.AttrValueAlias(leftCount))
	}
}

func (s *UniqueCustomSys) GetData() *pb3.UniqueCustom {
	data := s.GetBinaryData().UniqueCustomData
	if data == nil {
		data = &pb3.UniqueCustom{}
		s.GetBinaryData().UniqueCustomData = data
	}

	if data.Sets == nil {
		data.Sets = make(map[uint32]*pb3.UniqueCustomSet)
	}
	return data
}

func (s *UniqueCustomSys) getSetData(setId uint32) *pb3.UniqueCustomSet {
	data := s.GetData()
	setData, ok := data.Sets[setId]
	if !ok {
		setData = &pb3.UniqueCustomSet{
			SetId: setId,
		}
		data.Sets[setId] = setData
	}
	return setData
}

func (s *UniqueCustomSys) changeSet(setId uint32) {
	data := s.GetData()
	data.SetId = setId
	s.updateSet()
	s.SendProto3(13, 22, &pb3.S2C_13_22{SetId: setId})
}

func (s *UniqueCustomSys) updateSet() {
	data := s.GetData()
	s.owner.SetExtraAttr(attrdef.UniqueCustomSetId, attrdef.AttrValueAlias(data.SetId))
}

func (s *UniqueCustomSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.GetData()
	for setId, setData := range data.Sets {
		uConf := jsondata.UniqueCustomConfMgr[setId]
		if uConf == nil {
			continue
		}

		var attrs jsondata.AttrVec
		for fId, fashionData := range setData.Fashions {
			fConf, ok := uConf.Fashions[fId]
			if !ok {
				continue
			}

			// 基础属性
			for _, attrId := range fashionData.BaseAttrIds {
				attrConf, ok := fConf.BaseAttrs[attrId]
				if !ok {
					continue
				}
				valueConf, ok := attrConf.AttrValueConf[fashionData.Star]
				if !ok {
					continue
				}
				attrs = append(attrs, &jsondata.Attr{Type: attrConf.AttrId, Value: valueConf.Value})
			}
			// 特殊属性
			for _, attrId := range fashionData.SpecialAttrIds {
				attrConf, ok := fConf.SpecialAttrs[attrId]
				if !ok {
					continue
				}
				valueConf, ok := attrConf.AttrValueConf[fashionData.Star]
				if !ok {
					continue
				}
				attrs = append(attrs, &jsondata.Attr{Type: attrConf.AttrId, Value: valueConf.Value})
			}
			// 超级属性
			if starConf, ok := fConf.StarConf[fashionData.Star]; ok {
				attrs = append(attrs, starConf.SuperAttrs...)
			}
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, attrs)
	}
}

func (s *UniqueCustomSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_13_20
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	fConf := jsondata.GetUniqueCustomFashionConf(req.SetId, req.FId)
	if fConf == nil {
		return neterror.ConfNotFoundError("UniqueCustomFashionConf not found, id=%d, fId=%d", req.SetId, req.FId)
	}

	setData := s.getSetData(req.SetId)
	fData, ok := setData.Fashions[req.FId]
	if !ok {
		return neterror.ParamsInvalidError("fashionId %d not activate", req.FId)
	}

	starConf, ok := fConf.StarConf[fData.Star+1]
	if !ok {
		return neterror.ConfNotFoundError("UniqueCustomStarConf not found, id=%d, star=%d", req.FId, fData.Star+1)
	}

	if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUniqueCustomUpStarConsume}) {
		return neterror.ConsumeFailedError("not enough")
	}

	setData.Fashions[req.FId].Star += 1
	s.s2cFashion(req.SetId, req.FId)
	s.ResetSysAttr(attrdef.SaUniqueCustom)
	return nil
}

func (s *UniqueCustomSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_13_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if len(req.BaseAttrs) == 0 || len(req.SpecialAttrs) == 0 {
		return neterror.ParamsInvalidError("attrs is empty")
	}

	fConf := jsondata.GetUniqueCustomFashionConf(req.SetId, req.FId)
	if fConf == nil {
		return neterror.ConfNotFoundError("UniqueCustomFashionConf not found, id=%d, fId=%d", req.SetId, req.FId)
	}

	if !s.checkCustomAttrs(fConf.BaseAttrs, fConf.BaseAttrsNum, req.BaseAttrs) {
		return neterror.ParamsInvalidError("base attrs invalid")
	}
	if !s.checkCustomAttrs(fConf.SpecialAttrs, fConf.SpecialAttrsNum, req.SpecialAttrs) {
		return neterror.ParamsInvalidError("special attrs invalid")
	}

	starConf, ok := fConf.StarConf[1]
	if !ok {
		return neterror.ConfNotFoundError("UniqueCustomStarConf not found, id=%d, star=1", req.FId)
	}

	setData := s.getSetData(req.SetId)
	if _, ok := setData.Fashions[req.FId]; ok {
		return neterror.ParamsInvalidError("fashionId %d already activate", req.FId)
	}

	if !s.owner.ConsumeByConf(starConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUniqueCustomActiveConsume}) {
		return neterror.ConsumeFailedError("not enough")
	}

	if setData.Fashions == nil {
		setData.Fashions = make(map[uint32]*pb3.UniqueCustomFashion)
	}
	setData.Fashions[req.FId] = &pb3.UniqueCustomFashion{
		Id:             req.FId,
		Star:           1,
		BaseAttrIds:    req.BaseAttrs,
		SpecialAttrIds: req.SpecialAttrs,
	}

	s.checkLearnSkill(req.SetId)
	s.s2cFashion(req.SetId, req.FId)
	s.ResetSysAttr(attrdef.SaUniqueCustom)
	return nil

}

func (s *UniqueCustomSys) checkCustomAttrs(attrsConf map[uint32]*jsondata.UniqueCustomAttr, num uint32, attrs []uint32) bool {
	if len(attrs) != int(num) {
		return false
	}
	for _, id := range attrs {
		if _, ok := attrsConf[id]; !ok {
			return false
		}
	}
	return true
}

func (s *UniqueCustomSys) isCollectSet(setId uint32) bool {
	conf, ok := jsondata.UniqueCustomConfMgr[setId]
	if !ok {
		return false
	}

	setData := s.getSetData(setId)
	for fId := range conf.Fashions {
		if _, ok := setData.Fashions[fId]; !ok {
			return false
		}
	}
	return true
}

func (s *UniqueCustomSys) checkLearnSkill(setId uint32) {
	conf, ok := jsondata.UniqueCustomConfMgr[setId]
	if !ok {
		return
	}

	if s.isCollectSet(setId) {
		s.owner.LearnSkill(conf.SkillId, 1, true)
		s.updateActiveSetNum()
		s.updateLeftCount()
	}
}

func (s *UniqueCustomSys) updateActiveSetNum() {
	data := s.GetData()
	data.ActiveSetNum += 1
}

func (s *UniqueCustomSys) c2sDress(msg *base.Message) error {
	var req pb3.C2S_13_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	_, ok := jsondata.UniqueCustomConfMgr[req.SetId]
	if !ok {
		return neterror.ConfNotFoundError("UniqueCustomConf not found, id=%d", req.SetId)
	}

	if !s.isCollectSet(req.SetId) {
		return neterror.ParamsInvalidError("setId %d not collect", req.SetId)
	}

	s.changeSet(req.SetId)
	return nil
}

func uniqueCustomProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiUniqueCustom).(*UniqueCustomSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcAttr(calc)
}

func onAppearChange(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiUniqueCustom).(*UniqueCustomSys)
	if !ok || !sys.IsOpen() {
		return
	}

	appearPos := args[0].(uint32)
	if appearPos == appeardef.AppearPos_Cloth ||
		appearPos == appeardef.AppearPos_Weapon ||
		appearPos == appeardef.AppearPos_FootPrint ||
		appearPos == appeardef.AppearPos_Wing ||
		appearPos == appeardef.AppearPos_Rider ||
		appearPos == appeardef.AppearPos_Aura ||
		appearPos == appeardef.AppearPos_Bracelet ||
		appearPos == appeardef.AppearPos_RingsLaw {
		sys.changeSet(0)
	}
}

func init() {
	RegisterSysClass(sysdef.SiUniqueCustom, func() iface.ISystem {
		return &UniqueCustomSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaUniqueCustom, uniqueCustomProperty)
	event.RegActorEvent(custom_id.AeAppearChange, onAppearChange)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiUniqueCustom).(*UniqueCustomSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.OnNewDay()
	})

	engine.RegisterActorCallFunc(playerfuncid.F2GUniqueCustomUseCount, func(player iface.IPlayer, buf []byte) {
		sys, ok := player.GetSysObj(sysdef.SiUniqueCustom).(*UniqueCustomSys)
		if !ok || !sys.IsOpen() {
			return
		}
		data := sys.GetData()
		data.UsedCount += 1
		sys.updateLeftCount()
	})

	engine.RegisterSysCall(sysfuncid.F2GUniqueCustomStatus, func(buf []byte) {
		var msg pb3.CommonSt
		if err := pb3.Unmarshal(buf, &msg); err != nil {
			logger.LogError("unmarshal err: %v", err)
			return
		}

		isOpen := msg.BParam
		playerId := msg.U64Param
		openTimestamp := msg.U32Param
		manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
			player.SendProto3(13, 26, &pb3.S2C_13_26{
				IsOpen:        isOpen,
				OpenPlayerId:  playerId,
				OpenTimestamp: openTimestamp,
			})
		})
	})

	gmevent.Register("uniquecustom.reset", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiUniqueCustom).(*UniqueCustomSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		data := sys.GetData()
		data.UsedCount = 0
		sys.updateLeftCount()
		sys.s2cInfo()
		return true
	}, 1)

	net.RegisterSysProto(13, 20, sysdef.SiUniqueCustom, (*UniqueCustomSys).c2sUpStar)
	net.RegisterSysProto(13, 21, sysdef.SiUniqueCustom, (*UniqueCustomSys).c2sActive)
	net.RegisterSysProto(13, 22, sysdef.SiUniqueCustom, (*UniqueCustomSys).c2sDress)
}
