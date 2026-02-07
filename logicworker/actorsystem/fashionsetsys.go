/**
 * @Author: lzp
 * @Date: 2024/4/16
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type FashionSetSys struct {
	Base
}

func (s *FashionSetSys) OnInit() {
	if s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *FashionSetSys) OnLogin() {
	s.S2CInfo()
}

func (s *FashionSetSys) OnReconnect() {
	s.S2CInfo()
	s.ResetSysAttr(attrdef.SaFashionSet)
	s.ResetSysAttr(attrdef.SaFashionSetTalent)
}

func (s *FashionSetSys) OnOpen() {
	s.S2CInfo()
}

func (s *FashionSetSys) GetData() map[uint32]*pb3.FashionSetData {
	if s.GetBinaryData().FashionSetData == nil {
		s.GetBinaryData().FashionSetData = make(map[uint32]*pb3.FashionSetData)
	}
	return s.GetBinaryData().FashionSetData
}

func (s *FashionSetSys) S2CInfo() {
	dataMap := s.GetData()
	s.SendProto3(13, 6, &pb3.S2C_13_6{FSetDataL: functional.MapToSlice(dataMap)})
}

func (s *FashionSetSys) PushFashionSetData(setId uint32) {
	dataMap := s.GetData()
	if _, ok := dataMap[setId]; !ok {
		return
	}
	s.SendProto3(13, 7, &pb3.S2C_13_7{FSetData: dataMap[setId]})
}

func (s *FashionSetSys) c2sActiveFashionSet(msg *base.Message) error {
	var req pb3.C2S_13_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	setId := req.SetId
	suitNum := req.SuitNum
	dataMap := s.GetData()
	_, ok := dataMap[setId]

	if !ok {
		s.GetOwner().LogError("fashion set not active")
		return neterror.ParamsInvalidError("fashion set not active")
	}

	setData := s.getSetData(setId)
	isActive := setData.Sets[suitNum]
	if isActive {
		s.GetOwner().LogError("fashion set(suitNum=%d) is activated", suitNum)
		return neterror.ParamsInvalidError("fashion set is activated")
	}

	setConf := jsondata.GetFashionSetSetConf(setId, suitNum)
	if setConf == nil {
		return neterror.ConfNotFoundError("fashionsetconfig[setId=%d, suitNum=%d] not found", setId, suitNum)
	}

	if len(setData.AppearIds) < int(suitNum) {
		s.GetOwner().LogError("fashionset active limit(setNum=%d, reqSetNum=%d)", len(setData.AppearIds), suitNum)
		return neterror.ParamsInvalidError("fashionset active limit")
	}

	setData.Sets[suitNum] = true
	if setConf.SkillId > 0 {
		s.GetOwner().LearnSkill(setConf.SkillId, setConf.SkillLv, true)
	}

	s.ResetSysAttr(attrdef.SaFashionSet)
	s.PushFashionSetData(setId)
	return nil
}

func (s *FashionSetSys) getSetData(setId uint32) *pb3.FashionSetData {
	dataMap := s.GetData()
	data, ok := dataMap[setId]
	if !ok {
		dataMap[setId] = &pb3.FashionSetData{}
		data = dataMap[setId]
		data.Id = setId
	}
	if data.FashionData == nil {
		data.FashionData = make(map[int64]*pb3.FashionSetFashionData)
	}
	if data.Sets == nil {
		data.Sets = make(map[uint32]bool)
	}
	return data
}

func (s *FashionSetSys) initFashionSet(setId, fType, fashionId uint32) {
	conf := jsondata.GetFashionSetConf()
	if _, ok := conf[setId]; !ok {
		return
	}

	data := s.getSetData(setId)
	appearId := getAppearId(fType, fashionId)

	if !pie.Int64s(data.AppearIds).Contains(appearId) {
		data.AppearIds = pie.Int64s(data.AppearIds).Append(appearId).Unique()
	}

	if _, ok := data.FashionData[appearId]; !ok {
		data.FashionData[appearId] = &pb3.FashionSetFashionData{
			AppearId: appearId,
			Talents:  make([]*pb3.FashionTalent, 0),
		}
	}

	fData := data.FashionData[appearId]
	if len(fData.Talents) == 0 {
		fConf := jsondata.GetFashionSetFashionConf(setId, fType, fashionId)
		if fConf != nil {
			talents :=
				functional.Map(fConf.Talents, func(tConf *jsondata.FashionSetTalentConf) *pb3.FashionTalent {
					return &pb3.FashionTalent{
						Cond:  tConf.Cond,
						Lv:    0,
						Count: 0,
					}
				})
			fData.Talents = append(fData.Talents, talents...)
		}
	}

	s.PushFashionSetData(setId)
}

func (s *FashionSetSys) updateFashionSet(setId, fType, fashionId uint32) {
	dataMap := s.GetData()
	data, ok := dataMap[setId]
	if !ok {
		return
	}

	appearId := getAppearId(fType, fashionId)
	idx := pie.Int64s(data.AppearIds).FindFirstUsing(func(value int64) bool {
		return value == appearId
	})

	data.AppearIds = append(data.AppearIds[:idx], data.AppearIds[idx+1:]...)

	// 删除已激活的套装
	for suitNum, _ := range data.Sets {
		if int(suitNum) > len(data.AppearIds) {
			data.Sets[suitNum] = false
		}
	}

	// 重算套装属性(时装天赋属性保留)
	s.ResetSysAttr(attrdef.SaFashionSet)
}

func (s *FashionSetSys) calcFashionSetAttr(calc *attrcalc.FightAttrCalc) {
	fashionSetConf := jsondata.GetFashionSetConf()
	if fashionSetConf == nil {
		return
	}

	dataMap := s.GetData()
	for setId, data := range dataMap {
		var attrs jsondata.AttrVec
		for suitNum, isActive := range data.Sets {
			if !isActive {
				continue
			}
			setConf := jsondata.GetFashionSetSetConf(setId, suitNum)
			if setConf == nil {
				continue
			}
			attrs = append(attrs, setConf.Attrs...)
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, attrs)
	}
}

func (s *FashionSetSys) calcFashionSetTalentAttr(calc *attrcalc.FightAttrCalc) {
	dataMap := s.GetData()
	var calcAttr = func(setId uint32, appearId int64, fData *pb3.FashionSetFashionData) jsondata.AttrVec {
		fType, fId := getFashionTypeAndId(appearId)
		var attrs jsondata.AttrVec
		for _, tData := range fData.Talents {
			tConf := jsondata.GetFashionSetTalentConf(setId, fType, fId, tData.Cond)
			if tConf == nil {
				continue
			}
			tLvConf, ok := tConf.TalentLv[tData.Lv]
			if !ok {
				continue
			}
			attrs = append(attrs, tLvConf.Attrs...)
		}
		return attrs
	}

	for setId, data := range dataMap {
		var attrs jsondata.AttrVec
		for appearId, fData := range data.FashionData {
			calcAttrs := calcAttr(setId, appearId, fData)
			attrs = append(attrs, calcAttrs...)
		}
		engine.CheckAddAttrsToCalc(s.GetOwner(), calc, attrs)
	}
}

func (s *FashionSetSys) autoUpdateTalentLv(setId uint32, appearId int64) {
	dataMap := s.GetData()
	data, ok := dataMap[setId]
	if !ok {
		return
	}
	fData, ok := data.FashionData[appearId]
	if !ok {
		return
	}

	fType, fId := getFashionTypeAndId(appearId)

	var doLevelUp = func(tData *pb3.FashionTalent, tConf *jsondata.FashionSetTalentConf) (isLvUp bool) {
		oldLv := tData.Lv
		for i := oldLv; i < uint32(len(tConf.TalentLv)); i++ {
			newLv := i + 1
			tLvConf, ok := tConf.TalentLv[newLv]
			if !ok {
				break
			}
			if tData.Count < tLvConf.Count {
				break
			}
			tData.Lv = newLv
			isLvUp = true
		}
		return
	}

	for _, tData := range fData.Talents {
		tConf := jsondata.GetFashionSetTalentConf(setId, fType, fId, tData.Cond)
		if tConf == nil {
			continue
		}
		if doLevelUp(tData, tConf) {
			s.ResetSysAttr(attrdef.SaFashionSetTalent)
		}
	}
}

func (s *FashionSetSys) handleFashionTalent(param *custom_id.FashionTalentEvent) {
	dataMap := s.GetData()
	cond := param.Cond

	var doCheckUpdateTalent = func(fData *pb3.FashionSetFashionData,
		fConf *jsondata.FashionSetFashionConf) (isUpdate bool) {
		for _, talent := range fData.Talents {
			if talent.Cond != cond {
				continue
			}

			// 找出对应的配置
			tConf, _ := functional.Find(fConf.Talents, func(tConf *jsondata.FashionSetTalentConf) bool {
				return tConf.Cond == talent.Cond
			})

			if tConf == nil {
				continue
			}

			// 比对配置的条件值
			isAdd := false
			if len(tConf.Params) == 0 {
				isAdd = true
			} else if len(tConf.Params) == 1 {
				if param.Param0 >= tConf.Params[0] {
					isAdd = true
				}
			} else if len(tConf.Params) == 2 {
				if param.Param0 >= tConf.Params[0] &&
					param.Param1 >= tConf.Params[1] {
					isAdd = true
				}
			}

			if isAdd {
				talent.Count += param.Count
				isUpdate = true
			}
		}
		return
	}
	for setId, data := range dataMap {
		isPush := false
		for appearId, fashion := range data.FashionData {
			fType, fId := getFashionTypeAndId(appearId)
			fConf := jsondata.GetFashionSetFashionConf(setId, fType, fId)
			if doCheckUpdateTalent(fashion, fConf) {
				s.autoUpdateTalentLv(setId, appearId)
				isPush = true
			}
		}
		if isPush {
			s.PushFashionSetData(setId)
		}
	}
}

// 处理时装激活
func handleActiveFashion(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 1 {
		return
	}

	eData, ok := args[0].(*custom_id.FashionSetEvent)
	if !ok {
		return
	}
	sys.initFashionSet(eData.SetId, eData.FType, eData.FashionId)
}

// 处理时装过期
func handleFashionExpired(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 1 {
		return
	}

	eData, ok := args[0].(*custom_id.FashionSetEvent)
	if !ok {
		return
	}
	sys.updateFashionSet(eData.SetId, eData.FType, eData.FashionId)
}

// 处理时装天赋
func handleFashionTalent(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 1 {
		return
	}

	eData, ok := args[0].(*custom_id.FashionTalentEvent)
	if !ok {
		return
	}
	sys.handleFashionTalent(eData)
}

func handleKillMon(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 4 {
		return
	}

	monId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	conf := jsondata.GetMonsterConf(monId)
	if conf == nil {
		return
	}

	switch conf.SubType {
	case custom_id.MstGodWarMonster1, custom_id.MstGodWarMonster2,
		custom_id.MstGodWarMonster3, custom_id.MstGodWarBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:  custom_id.FashionSetGodAreaEvent,
			Count: count,
		})
	case custom_id.MstSelfBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:   custom_id.FashionSetSelfBossEvent,
			Count:  count,
			Param0: conf.Quality,
		})
	case custom_id.MstSuitBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:   custom_id.FashionSetSuitBossEvent,
			Count:  count,
			Param0: conf.Quality,
		})
	case custom_id.MstWorldBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:   custom_id.FashionSetWorldBossEvent,
			Count:  count,
			Param0: conf.Quality,
		})
	case custom_id.MstQiMenBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:   custom_id.FashionSetQiMenBossEvent,
			Count:  count,
			Param0: conf.Quality,
		})
	case custom_id.MstGodBeastBoss:
		sys.handleFashionTalent(&custom_id.FashionTalentEvent{
			Cond:   custom_id.FashionSetGodBeastEvent,
			Count:  count,
			Param0: conf.Quality,
		})
	}
}

func handlePlayerLevelUp(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 2 {
		return
	}
	oldLv, ok := args[0].(uint32)
	if !ok {
		return
	}
	newLv, ok := args[1].(uint32)
	if !ok {
		return
	}

	sys.handleFashionTalent(&custom_id.FashionTalentEvent{
		Cond:  custom_id.FashionSetPlayerLvEvent,
		Count: newLv - oldLv,
	})
}

func handleCircleChange(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 2 {
		return
	}
	oldLv, ok := args[0].(uint32)
	if !ok {
		return
	}
	newLv, ok := args[1].(uint32)
	if !ok {
		return
	}

	sys.handleFashionTalent(&custom_id.FashionTalentEvent{
		Cond:  custom_id.FashionSetJingJieEvent,
		Count: newLv - oldLv,
	})
}

func calcFashionSetAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcFashionSetAttr(calc)
}

func calcFashionSetTalentAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFashionSet).(*FashionSetSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcFashionSetTalentAttr(calc)
}

func getAppearId(fType, appearId uint32) int64 {
	return int64(fType)<<32 | int64(appearId)
}

func getFashionTypeAndId(appearId int64) (uint32, uint32) {
	fType := uint32(appearId >> 32)
	fId := uint32(appearId & 0xFFFFFFFF)
	return fType, fId
}

func init() {
	RegisterSysClass(sysdef.SiFashionSet, func() iface.ISystem {
		return &FashionSetSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFashionSet, calcFashionSetAttr)
	engine.RegAttrCalcFn(attrdef.SaFashionSetTalent, calcFashionSetTalentAttr)

	event.RegActorEvent(custom_id.AeFashionTalentEvent, handleFashionTalent)

	event.RegActorEvent(custom_id.AeActiveFashion, handleActiveFashion)
	event.RegActorEvent(custom_id.AeKillMon, handleKillMon)
	event.RegActorEvent(custom_id.AeLevelUp, handlePlayerLevelUp)
	event.RegActorEvent(custom_id.AeCircleChange, handleCircleChange)

	net.RegisterSysProtoV2(13, 5, sysdef.SiFashionSet, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FashionSetSys).c2sActiveFashionSet
	})
}
