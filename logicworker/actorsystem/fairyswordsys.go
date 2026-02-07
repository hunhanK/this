/**
 * @Author: lzp
 * @Date: 2024/11/25
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"
)

type FairySwordSys struct {
	Base
}

const (
	FairySwordForgeId1 = 1
	FairySwordForgeId2 = 2
	FairySwordForgeId3 = 3
)

func (s *FairySwordSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FairySwordSys) OnLogin() {
	s.s2cInfo()
}

func (s *FairySwordSys) OnInit() {
	binary := s.GetBinaryData()
	if binary.FairySwordData == nil {
		binary.FairySwordData = &pb3.FairySwordData{}
	}
	data := binary.FairySwordData
	if data.PosEquips == nil {
		data.PosEquips = make(map[uint32]*pb3.FairySwordPosData)
	}
	if data.CastMap == nil {
		data.CastMap = make(map[uint32]uint32)
	}
}

func (s *FairySwordSys) s2cInfo() {
	s.SendProto3(73, 0, &pb3.S2C_73_0{Data: s.getData()})
}

func (s *FairySwordSys) getData() *pb3.FairySwordData {
	binary := s.GetBinaryData()
	return binary.FairySwordData
}

func (s *FairySwordSys) getPosData(pos uint32) *pb3.FairySwordPosData {
	data := s.getData()
	posData := data.PosEquips[pos]
	if posData == nil {
		posData = &pb3.FairySwordPosData{}
		data.PosEquips[pos] = posData
	}
	return posData
}

func (s *FairySwordSys) s2cPosData(pos uint32) {
	s.SendProto3(73, 7, &pb3.S2C_73_7{PosData: s.getPosData(pos)})
}

func (s *FairySwordSys) c2sLvUp(msg *base.Message) error {
	var req pb3.C2S_73_1
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	posData := s.getPosData(req.Pos)
	if posData.Equip == nil {
		return neterror.ParamsInvalidError("pos:%d not equip fairy sword", req.Pos)
	}

	nextLv := posData.Lv + 1
	nextLvConf := jsondata.GetFairySwordLvConf(req.Pos, nextLv)
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("lv: %d is nil", nextLv)
	}

	if posData.Stage < nextLvConf.StageLimit {
		return neterror.ConfNotFoundError("stage not satisfy")
	}

	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySwordLvUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Lv = nextLv
	s.afterLvUp()
	s.s2cPosData(req.Pos)
	s.owner.TriggerQuestEvent(custom_id.QttFairySwordOptLvUp, 0, 1)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySwordLvUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"pos":   req.Pos,
			"level": nextLv,
		}),
	})
	s.SendProto3(73, 1, &pb3.S2C_73_1{Pos: req.Pos})
	return nil
}

func (s *FairySwordSys) c2sStageUp(msg *base.Message) error {
	var req pb3.C2S_73_2
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	posData := s.getPosData(req.Pos)
	if posData.Equip == nil {
		return neterror.ParamsInvalidError("pos:%d not equip fairy sword", req.Pos)
	}

	nextStage := posData.Stage + 1
	nextStageConf := jsondata.GetFairySwordStageConf(req.Pos, nextStage)
	if nextStageConf == nil {
		return neterror.ConfNotFoundError("stage: %d is nil", nextStage)
	}

	if posData.Lv < nextStageConf.LvLimit {
		return neterror.ConfNotFoundError("lv not satisfy")
	}

	if !s.owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySwordStageUp}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Stage = nextStage
	s.afterStageUp()
	s.s2cPosData(req.Pos)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogFairySwordStageUp, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"pos":   req.Pos,
			"level": nextStage,
		}),
	})
	s.SendProto3(73, 2, &pb3.S2C_73_2{Pos: req.Pos})
	return nil
}

func (s *FairySwordSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_73_3
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOn(req.Pos, req.Hdl); err != nil {
		return err
	}
	s.takeOn(req.Pos, req.Hdl)
	return nil
}

func (s *FairySwordSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_73_4
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	if err := s.checkTakeOff(req.Pos); err != nil {
		return err
	}
	s.takeOff(req.Pos)
	return nil
}

func (s *FairySwordSys) c2sCompose(msg *base.Message) error {
	var req pb3.C2S_73_5
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	pos := req.Pos
	conf := jsondata.GetFairySwordConf(pos)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found pos:%d", pos)
	}

	posData := s.getPosData(pos)
	if posData.Equip == nil {
		return neterror.ParamsInvalidError("pos:%d not equip fairy sword", pos)
	}

	equip := posData.Equip
	composeConf, ok := conf.ComposeConf[equip.ItemId]
	if !ok {
		return neterror.ConfNotFoundError("compose conf is nil, itemId:%d", equip.ItemId)
	}

	// 消耗
	if !s.owner.ConsumeByConf(composeConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogFairySwordCompose,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 获得新装备
	itemId := composeConf.NewItemId
	itemConf := jsondata.GetItemConfig(itemId)
	if itemConf == nil {
		return nil
	}

	engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    itemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairySwordCompose})

	bagSys, _ := s.owner.GetSysObj(sysdef.SiFairySwordBag).(*FairySwordBagSys)
	itemList := bagSys.GetItemListByItemId(itemId, 1)
	itemHdl := itemList[random.Interval(0, len(itemList)-1)]
	s.takeOn(pos, itemHdl)

	// 删除返回的旧装备
	consumes := jsondata.ConsumeVec{
		{Id: equip.ItemId, Count: 1},
	}
	s.owner.ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySwordCompose})
	s.owner.TriggerQuestEvent(custom_id.QttFairySwordOptCompose, 0, 1)
	s.owner.TriggerQuestEvent(custom_id.QttFairySwordComposeSword, itemConf.Quality, 1)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogFairySwordCompose, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"newItem":    composeConf.NewItemId,
			"beforeItem": composeConf.TakeItemId,
			"pos":        pos,
		}),
	})

	s.SendProto3(73, 5, &pb3.S2C_73_5{Pos: req.Pos})
	return nil
}

func (s *FairySwordSys) c2sForge(msg *base.Message) error {
	var req pb3.C2S_73_6
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	posData := s.getPosData(req.Pos)
	if posData.Equip == nil {
		return neterror.ParamsInvalidError("pos:%d not equip fairy sword", req.Pos)
	}

	itemConf := jsondata.GetItemConfig(posData.Equip.ItemId)
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemId:%d config not found", posData.Equip.ItemId)
	}

	nextId := posData.Equip.Union1 + 1
	conf := jsondata.GetFairySwordForgeConf(nextId)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", nextId)
	}

	if itemConf.Stage < conf.StageLimit {
		return neterror.ParamsInvalidError("id: %d stage not satisfy", nextId)
	}
	if itemConf.Quality < conf.QualityLimit {
		return neterror.ParamsInvalidError("id: %d quality not satisfy", nextId)
	}

	condConf := jsondata.GetFairySwordForgeCondConf(nextId, req.Pos, itemConf.Stage)
	if condConf == nil {
		return neterror.ConfNotFoundError("id:%d, pos:%d stage:%d config not found", nextId, req.Pos, itemConf.Stage)
	}

	if !s.owner.ConsumeByConf(condConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySwordForge}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	posData.Equip.Union1 = nextId
	s.afterForge()
	s.s2cPosData(req.Pos)
	s.SendProto3(73, 6, &pb3.S2C_73_6{Pos: req.Pos})
	return nil
}

func (s *FairySwordSys) c2sCast(msg *base.Message) error {
	var req pb3.C2S_73_8
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	nextStage := data.CastMap[req.Id] + 1
	castConf := jsondata.GetFairySwordCastConf(req.Id, nextStage)
	if castConf == nil {
		return neterror.ConfNotFoundError("id:%d stage:%d, config not found", req.Id, nextStage)
	}

	if !s.owner.ConsumeByConf(castConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogFairySwordCast}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.CastMap[req.Id] = nextStage
	s.afterCast()
	s.ResetSysAttr(attrdef.SaFairySwordCast)
	s.SendProto3(73, 8, &pb3.S2C_73_8{Id: req.Id, Stage: nextStage})
	return nil
}

func (s *FairySwordSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_73_9
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	data := s.getData()
	nextLv := data.SuitLv + 1
	suitConf := jsondata.GetFairySwordCastSuitConf(nextLv)
	if suitConf == nil {
		return neterror.ConfNotFoundError("nextLv:%d, config not found", nextLv)
	}

	count := s.getCastNumByStage(suitConf.StageLimit)
	if count < suitConf.NumLimit {
		return neterror.ParamsInvalidError("cond not satisfy")
	}

	data.SuitLv = nextLv
	s.owner.TriggerQuestEvent(custom_id.QttFairySwordSuitLv, 0, int64(nextLv))
	s.ResetSysAttr(attrdef.SaFairySwordCast)
	s.SendProto3(73, 9, &pb3.S2C_73_9{Lv: nextLv})
	return nil
}

func (s *FairySwordSys) getCastNumByStage(stage uint32) uint32 {
	data := s.getData()
	var count uint32
	for _, castStage := range data.CastMap {
		if castStage >= stage {
			count++
		}
	}
	return count
}

func (s *FairySwordSys) getFairySwordItem(hdl uint64) *pb3.ItemSt {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiFairySwordBag).(*FairySwordBagSys)
	if !ok {
		return nil
	}
	return bagSys.FindItemByHandle(hdl)
}

func (s *FairySwordSys) checkTakeOn(pos uint32, hdl uint64) error {
	equip := s.getFairySwordItem(hdl)
	if equip == nil {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	conf := jsondata.GetFairySwordConf(pos)
	itemConf := jsondata.GetItemConfig(equip.ItemId)
	if conf.Type != itemConf.Type || conf.SubType != itemConf.SubType {
		return neterror.ParamsInvalidError("item not fairy sword")
	}

	return nil
}

func (s *FairySwordSys) checkTakeOff(pos uint32) error {
	posData := s.getPosData(pos)
	if posData.Equip == nil {
		return neterror.ParamsInvalidError("pos:%d not equip fairy sword", pos)
	}
	return nil
}

func (s *FairySwordSys) takeOn(pos uint32, hdl uint64) {
	posData := s.getPosData(pos)
	oldEquip := posData.Equip
	if oldEquip != nil {
		s.takeOff(pos)
	}

	equip := s.getFairySwordItem(hdl)

	// 删除装备
	if !s.owner.RemoveFairySwordItemByHandle(hdl, pb3.LogId_LogFairySwordTakeOn) {
		return
	}

	// 穿戴装备
	posData.Equip = equip
	posData.Pos = pos

	if oldEquip != nil && equip != nil {
		s.inherit(pos, oldEquip, equip)
	}

	s.afterTakeOn()
	s.s2cPosData(pos)
}

func (s *FairySwordSys) takeOff(pos uint32) {
	posData := s.getPosData(pos)
	if posData.Equip == nil {
		return
	}

	oldEquip := posData.Equip
	posData.Equip = nil
	if !engine.GiveRewards(s.owner, jsondata.StdRewardVec{
		&jsondata.StdReward{
			Id:    oldEquip.ItemId,
			Count: 1,
		},
	}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairySwordTakeOff}) {
		return
	}

	s.afterTakeOff()
	s.s2cPosData(pos)
}

func (s *FairySwordSys) afterTakeOn() {
	s.ResetSysAttr(attrdef.SaFairySword)
	s.ResetSysAttr(attrdef.SaFairySwordForge)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordTakeOn)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordTakeOnByType)
}

func (s *FairySwordSys) afterTakeOff() {
	s.ResetSysAttr(attrdef.SaFairySword)
	s.ResetSysAttr(attrdef.SaFairySwordForge)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordTakeOn)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordTakeOnByType)
}

func (s *FairySwordSys) afterForge() {
	s.ResetSysAttr(attrdef.SaFairySwordForge)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordForge)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordSuitForge)
}

func (s *FairySwordSys) afterLvUp() {
	s.ResetSysAttr(attrdef.SaFairySword)
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordLvUp)
}

func (s *FairySwordSys) afterStageUp() {
	s.ResetSysAttr(attrdef.SaFairySword)
	s.owner.TriggerQuestEvent(custom_id.QttFairySwordOptStageUp, 0, 1)
}

func (s *FairySwordSys) afterCast() {
	s.owner.TriggerQuestEventRange(custom_id.QttFairySwordCast)
}

func (s *FairySwordSys) calcFairySwordAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	var attrs jsondata.AttrVec
	for _, posData := range data.PosEquips {
		if posData.Equip == nil {
			continue
		}
		// 装备本身属性
		itemConf := jsondata.GetItemConfig(posData.Equip.ItemId)
		if itemConf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.PremiumAttrs)
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.SuperAttrs)

		// 等级属性
		lvConf := jsondata.GetFairySwordLvConf(posData.Pos, posData.Lv)
		if lvConf != nil {
			attrs = append(attrs, lvConf.Attrs...)
		}

		// 阶级属性
		stageConf := jsondata.GetFairySwordStageConf(posData.Pos, posData.Stage)
		if stageConf != nil {
			attrs = append(attrs, stageConf.Attrs...)
		}
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, attrs)
}

func (s *FairySwordSys) calcFairySwordAttrRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	data := s.getData()

	addRate := totalSysCalc.GetValue(attrdef.FairySwordBaseAttrRate)
	var attrs jsondata.AttrVec
	for _, posData := range data.PosEquips {
		if posData.Equip == nil {
			continue
		}

		itemConf := jsondata.GetItemConfig(posData.Equip.ItemId)
		if itemConf == nil {
			continue
		}
		attrs = append(attrs, itemConf.StaticAttrs...)
	}
	if addRate > 0 && len(attrs) > 0 {
		engine.CheckAddAttrsRateRoundingUp(s.GetOwner(), calc, attrs, uint32(addRate))
	}
}

func (s *FairySwordSys) calcFairySwordForgeAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()

	// 筛选出有打造词条的装备,按照大->小排序
	var itemList []*pb3.ItemSt
	for _, posData := range data.PosEquips {
		equip := posData.Equip
		if equip == nil || equip.Union1 == 0 {
			continue
		}
		itemList = append(itemList, equip)
	}
	sort.Slice(itemList, func(i, j int) bool {
		return itemList[i].Union1 > itemList[j].Union1
	})

	forgeMap := make(map[uint32][]*pb3.ItemSt)
	forgeMap[FairySwordForgeId1] = make([]*pb3.ItemSt, 0)
	forgeMap[FairySwordForgeId2] = make([]*pb3.ItemSt, 0)
	forgeMap[FairySwordForgeId3] = make([]*pb3.ItemSt, 0)
	for _, posData := range data.PosEquips {
		equip := posData.Equip
		if equip == nil {
			continue
		}
		switch equip.Union1 {
		case FairySwordForgeId1:
			forgeMap[FairySwordForgeId1] = append(forgeMap[FairySwordForgeId1], equip)
		case FairySwordForgeId2:
			forgeMap[FairySwordForgeId1] = append(forgeMap[FairySwordForgeId1], equip)
			forgeMap[FairySwordForgeId2] = append(forgeMap[FairySwordForgeId2], equip)
		case FairySwordForgeId3:
			forgeMap[FairySwordForgeId1] = append(forgeMap[FairySwordForgeId1], equip)
			forgeMap[FairySwordForgeId2] = append(forgeMap[FairySwordForgeId2], equip)
			forgeMap[FairySwordForgeId3] = append(forgeMap[FairySwordForgeId3], equip)
		}
	}
	for _, forgeL := range forgeMap {
		sortItemStByStage(forgeL)
	}

	length := len(itemList)
	for i := 1; i <= length; i++ {
		id := itemList[i-1].Union1
		forgeL := forgeMap[id]
		item := forgeL[i-1]
		itemConf := jsondata.GetItemConfig(item.ItemId)
		stage := itemConf.Stage

		suitConf := jsondata.GetFairySwordForgeSuitConf(id, uint32(i), stage)
		if suitConf == nil {
			continue
		}

		// 套装效果
		engine.CheckAddAttrsToCalc(s.owner, calc, suitConf.Attrs)
		if suitConf.SkillId > 0 {
			s.owner.LearnSkill(suitConf.SkillId, suitConf.SkillLv, true)
		}
	}
}

func (s *FairySwordSys) calcFairySwordCastAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	var attrs jsondata.AttrVec
	for id, stage := range data.CastMap {
		castConf := jsondata.GetFairySwordCastConf(id, stage)
		if castConf == nil {
			continue
		}
		attrs = append(attrs, castConf.Attrs...)
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, attrs)

	// 套装属性
	lv := data.SuitLv
	suitConf := jsondata.GetFairySwordCastSuitConf(lv)
	if suitConf != nil {
		engine.CheckAddAttrsToCalc(s.owner, calc, suitConf.Attrs)
		if suitConf.SkillId > 0 {
			s.owner.LearnSkill(suitConf.SkillId, suitConf.SkillLv, true)
		}
	}
}

// 继承
func (s *FairySwordSys) inherit(pos uint32, oldEquip *pb3.ItemSt, equip *pb3.ItemSt) {
	oldItemConf := jsondata.GetItemConfig(oldEquip.ItemId)
	itemConf := jsondata.GetItemConfig(equip.ItemId)

	oldUnion := oldEquip.Union1
	if oldUnion <= 0 {
		return
	}

	// 获取返还材料
	getReturnItems := func(union uint32) jsondata.StdRewardVec {
		var rewards jsondata.StdRewardVec
		for i := uint32(1); i <= union; i++ {
			condConf := jsondata.GetFairySwordForgeCondConf(i, pos, oldItemConf.Stage)
			if condConf == nil {
				continue
			}
			for _, consume := range condConf.Consume {
				rewards = append(rewards, &jsondata.StdReward{
					Id:    consume.Id,
					Count: int64(consume.Count),
					Bind:  false,
				})
			}
		}
		return rewards
	}

	var items jsondata.StdRewardVec
	if oldItemConf.Stage == itemConf.Stage {
		// 同阶级替换锻造id
		if oldUnion > 0 {
			// 满足条件直接继承
			conf := jsondata.GetFairySwordForgeConf(oldUnion)
			if itemConf.Stage >= conf.StageLimit && itemConf.Quality >= conf.QualityLimit {
				equip.Union1 = oldEquip.Union1
				return
			}

			// 返还材料
			items = getReturnItems(oldUnion)
		}
	} else {
		// 非同阶返回材料
		items = getReturnItems(oldUnion)
	}

	if len(items) > 0 && !engine.GiveRewards(s.owner, items, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairySwordTakeOnOff}) {
		return
	}
}

func calcFairySwordAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySwordAttr(calc)
}

func calcFairySwordAttrRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySwordAttrRate(totalSysCalc, calc)
}

func calcFairySwordForgeAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySwordForgeAttr(calc)
}

func calcFairySwordCastAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !s.IsOpen() {
		return
	}
	s.calcFairySwordCastAttr(calc)
}

func sortItemStByStage(itemList []*pb3.ItemSt) {
	sort.Slice(itemList, func(i, j int) bool {
		conf1 := jsondata.GetItemConfig(itemList[i].ItemId)
		conf2 := jsondata.GetItemConfig(itemList[j].ItemId)
		return conf1.Stage > conf2.Stage
	})
}

// 任务统计
func fairySwordTakeOnCount(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	var count uint32
	for _, posData := range data.PosEquips {
		if posData.Equip != nil {
			count++
		}
	}
	return count
}

// 剑装穿戴X件类型的装备
func fairySwordTakeOnCountByType(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) == 0 {
		return 0
	}
	typ := ids[0]
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	var count uint32
	for _, posData := range data.PosEquips {
		if posData.Equip != nil {
			itemConf := jsondata.GetItemConfig(posData.Equip.ItemId)
			if itemConf != nil && itemConf.Type == typ {
				count++
			}
		}
	}
	return count
}

func fairySwordLvCount(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	lv := ids[0]
	var count uint32
	for _, posData := range data.PosEquips {
		if posData.Lv >= lv {
			count++
		}
	}
	return count
}

func fairySwordComposeCount(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	stage := ids[0]
	var count uint32
	for _, posData := range data.PosEquips {
		equip := posData.Equip
		if equip == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if itemConf == nil {
			continue
		}
		if itemConf.Stage >= stage {
			count++
		}
	}
	return count
}

func fairySwordForgeCount(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	stage := ids[0]
	var count uint32
	for _, posData := range data.PosEquips {
		equip := posData.Equip
		if equip == nil {
			continue
		}
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if itemConf == nil {
			continue
		}
		if itemConf.Stage >= stage {
			count++
		}
	}
	return count
}

func fairySwordForgeSuitCount(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	lv := ids[0]
	var count uint32
	for _, posData := range data.PosEquips {
		equip := posData.Equip
		if equip == nil {
			continue
		}
		if equip.Union1 >= lv {
			count++
		}
	}
	return count
}

func fairySwordCastCount(actor iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}
	sys, ok := actor.GetSysObj(sysdef.SiFairySword).(*FairySwordSys)
	if !ok || !sys.IsOpen() {
		return 0
	}
	data := sys.getData()
	stage := ids[0]
	var count uint32
	for _, castStage := range data.CastMap {
		if castStage >= stage {
			count++
		}
	}
	return count
}

func init() {
	RegisterSysClass(sysdef.SiFairySword, func() iface.ISystem {
		return &FairySwordSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaFairySword, calcFairySwordAttr)
	engine.RegAttrCalcFn(attrdef.SaFairySwordForge, calcFairySwordForgeAttr)
	engine.RegAttrCalcFn(attrdef.SaFairySwordCast, calcFairySwordCastAttr)

	engine.RegAttrAddRateCalcFn(attrdef.SaFairySword, calcFairySwordAttrRate)

	engine.RegQuestTargetProgress(custom_id.QttFairySwordTakeOn, fairySwordTakeOnCount)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordTakeOnByType, fairySwordTakeOnCountByType)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordLvUp, fairySwordLvCount)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordCompose, fairySwordComposeCount)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordForge, fairySwordForgeCount)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordSuitForge, fairySwordForgeSuitCount)
	engine.RegQuestTargetProgress(custom_id.QttFairySwordCast, fairySwordCastCount)

	net.RegisterSysProtoV2(73, 1, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sLvUp
	})
	net.RegisterSysProtoV2(73, 2, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sStageUp
	})

	net.RegisterSysProtoV2(73, 3, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(73, 4, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sTakeOff
	})

	net.RegisterSysProtoV2(73, 5, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sCompose
	})

	net.RegisterSysProtoV2(73, 6, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sForge
	})

	net.RegisterSysProtoV2(73, 8, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sCast
	})

	net.RegisterSysProtoV2(73, 9, sysdef.SiFairySword, func(s iface.ISystem) func(*base.Message) error {
		return s.(*FairySwordSys).c2sActive
	})
}
