/**
 * @Author: yzh
 * @Date:
 * @Desc: 本源
 * @Modify： zjj
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type OriginSys struct {
	Base
}

func (s *OriginSys) GetPlayerState() *pb3.OriginSysState {
	state := s.owner.GetBinaryData().OriginSysState
	if state == nil {
		state = &pb3.OriginSysState{
			PosToEquipMap: map[uint32]*pb3.ItemSt{},
		}
		s.owner.GetBinaryData().OriginSysState = state
	}
	if state.PosToEquipMap == nil {
		state.PosToEquipMap = map[uint32]*pb3.ItemSt{}
	}
	return state
}

func (s *OriginSys) takeOn(handle uint64) {
	item := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem).FindItemByHandle(handle)

	var isValidItem bool
	for _, conf := range jsondata.OriginCfg.Suits {
		for _, itemId := range conf.EquipIds {
			if item.ItemId != itemId {
				continue
			}

			isValidItem = true
			break
		}
		if isValidItem {
			break
		}
	}

	if !isValidItem {
		return
	}

	playerState := s.GetPlayerState()
	s.LogPlayerBehavior(uint64(item.ItemId), map[string]interface{}{
		"Item":     item,
		"CurLevel": playerState.CurLevel,
	}, pb3.LogId_LogOriginTakeOn)

	bagSys := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	var needReCalcOriginLevel bool
	itemConf := jsondata.GetItemConfig(item.ItemId)
	pos, _ := itemdef.GetPosByType(itemConf.SubType)
	item.Pos = pos
	oldItem, ok := playerState.PosToEquipMap[pos]
	if ok {
		// 有旧装备 需要检查一下能否卸下
		if s.GetOwner().GetBagAvailableCount() == 0 {
			s.GetOwner().SendTipMsg(tipmsgid.TpBagIsFull)
			return
		}
		oldItemConf := jsondata.GetItemConfig(oldItem.ItemId)
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if oldItemConf.Stage != itemConf.Stage {
			needReCalcOriginLevel = true
		}

		oldItem.Pos = 0
		bagSys.AddItemPtr(oldItem, true, pb3.LogId_LogTakeOffEquip)
		s.SendProto3(34, 12, &pb3.S2C_34_12{
			Item: oldItem,
		})
	} else {
		needReCalcOriginLevel = true
	}

	playerState.PosToEquipMap[pos] = item
	bagSys.RemoveItemByHandle(handle, pb3.LogId_LogTakeOnOriginEquip)

	if needReCalcOriginLevel {
		s.calcOriginLevel()
	}

	s.ResetSysAttr(attrdef.SaOrigin)
	s.owner.TriggerQuestEvent(custom_id.QttWoreOriginEquip, 0, int64(len(playerState.PosToEquipMap)))
	s.owner.TriggerQuestEventRange(custom_id.QttTakenOriginEquipStageNum)

	s.SendProto3(34, 11, &pb3.S2C_34_11{
		Item: item,
	})

	s.s2cState()
}

func (s *OriginSys) takeOff(itemId uint32, isSend bool) {
	if s.GetOwner().GetBagAvailableCount() == 0 {
		s.GetOwner().SendTipMsg(tipmsgid.TpBagIsFull)
		return
	}
	playerState := s.GetPlayerState()
	var item *pb3.ItemSt
	for _, oneItem := range playerState.PosToEquipMap {
		if oneItem.ItemId != itemId {
			continue
		}
		item = oneItem
	}

	if item == nil {
		return
	}
	s.LogPlayerBehavior(uint64(item.ItemId), map[string]interface{}{
		"item":     item,
		"curLevel": playerState.CurLevel,
	}, pb3.LogId_LogOriginTakeOff)

	delete(playerState.PosToEquipMap, item.Pos)
	item.Pos = 0
	s.owner.GetSysObj(sysdef.SiBag).(*BagSystem).AddItemPtr(item, true, pb3.LogId_LogTakeOffEquip)

	origLv := playerState.CurLevel
	levelConf, ok := jsondata.GetOriginConf(origLv)
	if ok {
		s.owner.ForgetSkill(levelConf.SkillId, true, true, true)
	}

	s.calcOriginLevel()

	s.ResetSysAttr(attrdef.SaOrigin)

	s.SendProto3(34, 12, &pb3.S2C_34_12{
		Item: item,
	})

	s.owner.TriggerQuestEvent(custom_id.QttWoreOriginEquip, 0, int64(len(playerState.PosToEquipMap)))
	s.owner.TriggerQuestEventRange(custom_id.QttTakenOriginEquipStageNum)

	if isSend {
		s.s2cState()
	}
}

func (s *OriginSys) calcOriginLevel() {
	state := s.GetPlayerState()

	if uint32(len(state.PosToEquipMap)) < jsondata.OriginCfg.EquipNum {
		state.CurLevel = 0
		return
	}

	var minStage uint32
	for _, item := range state.PosToEquipMap {
		itemConf := jsondata.GetItemConfig(item.ItemId)
		if minStage == 0 {
			minStage = itemConf.Stage
			continue
		}

		if minStage < itemConf.Stage {
			continue
		}

		minStage = itemConf.Stage
	}

	levelConf, ok := jsondata.GetOriginConf(minStage)
	if !ok {
		return
	}

	oldStage := state.CurLevel
	state.CurLevel = minStage

	if oldStage != state.CurLevel {
		gifts := map[uint32]uint32{}
		for giftId, gift := range s.owner.GetBinaryData().SponsorGifts {
			if gift.State != SponsorGiftStateCanBuy {
				gifts[giftId] = gift.State
			}
		}
		s.LogPlayerBehavior(uint64(state.CurLevel), map[string]interface{}{
			"sponsorGifts": gifts,
		}, pb3.LogId_LogOriginLevelChange)
	}

	var sendTip bool
	if minStage > state.HistoryMaxLevel {
		state.HistoryMaxLevel = minStage
		sendTip = true
	}

	s.owner.SetExtraAttr(attrdef.OriginLevel, attrdef.AttrValueAlias(state.CurLevel))

	skillLv := s.owner.GetSkillLv(levelConf.SkillId)
	if skillLv != levelConf.SkillLv {
		if skillLv > levelConf.Level {
			s.owner.ForgetSkill(levelConf.SkillId, true, false, true)
		}
		s.owner.LearnSkill(levelConf.SkillId, levelConf.SkillLv, true)
		s.owner.TriggerQuestEvent(custom_id.QttUpgradeOriginSkillLevel, levelConf.SkillId, int64(levelConf.SkillLv))
		if sendTip && levelConf.SkillBroadcastId > 0 {
			engine.BroadcastTipMsgById(levelConf.SkillBroadcastId, state.CurLevel, s.owner.GetName())
		}
		if state.FirstUpSkillLvAt == 0 {
			state.FirstUpSkillLvAt = time_util.NowSec()
		}
		s.owner.SendProto3(34, 13, &pb3.S2C_34_13{
			OrigLv:  state.CurLevel,
			SkillId: levelConf.SkillId,
			SkillLv: levelConf.SkillLv,
		})
		return
	}
}

func (s *OriginSys) s2cState() {
	s.SendProto3(34, 10, &pb3.S2C_34_10{
		State: s.GetPlayerState(),
	})
}

func (s *OriginSys) c2sState(_ *base.Message) {
	s.s2cState()
}

func (s *OriginSys) c2sTakeOn(msg *base.Message) {
	var req pb3.C2S_34_10
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("un marshal take on origin equip err:%v", err)
		return
	}
	s.takeOn(req.Handle)
}

func (s *OriginSys) c2sTakeOff(msg *base.Message) {
	var req pb3.C2S_34_11
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("un marshal take on origin equip err:%v", err)
		return
	}
	s.takeOff(req.ItemId, true)
}

func (s *OriginSys) OnLogin() {
	s.owner.SetExtraAttr(attrdef.OriginLevel, attrdef.AttrValueAlias(s.GetPlayerState().CurLevel))
	s.s2cState()
}

func (s *OriginSys) OnReconnect() {
	s.owner.SetExtraAttr(attrdef.OriginLevel, attrdef.AttrValueAlias(s.GetPlayerState().CurLevel))
	s.s2cState()
}

func (s *OriginSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: coreNumData,
		StrArgs: string(bytes),
	})
}

func calcOriginSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	state := player.GetSysObj(sysdef.SiOrigin).(*OriginSys).GetPlayerState()

	for _, origEquip := range state.PosToEquipMap {
		equipConf := jsondata.GetItemConfig(origEquip.ItemId)
		//基础属性
		engine.CheckAddAttrsToCalc(player, calc, equipConf.StaticAttrs)
		//极品属性
		engine.CheckAddAttrsToCalc(player, calc, equipConf.PremiumAttrs)
		//品质属性
		engine.CheckAddAttrsSelectQualityToCalc(player, calc, equipConf.SuperAttrs, equipConf.Quality)
	}

	curLevelConf, ok := jsondata.GetOriginConf(state.CurLevel)
	if !ok {
		return
	}

	engine.AddAttrsToCalc(player, calc, curLevelConf.Attrs)
}

func init() {
	RegisterSysClass(sysdef.SiOrigin, func() iface.ISystem {
		return &OriginSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaOrigin, calcOriginSysAttr)

	engine.RegQuestTargetProgress(custom_id.QttUpgradeOriginSkillLevel, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		obj := actor.GetSysObj(sysdef.SiOrigin)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys, ok := obj.(*OriginSys)
		if !ok {
			return 0
		}
		// 本源就一套技能 直接返回等级吧
		return sys.GetPlayerState().CurLevel
	})

	engine.RegQuestTargetProgress(custom_id.QttWoreOriginEquip, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		obj := actor.GetSysObj(sysdef.SiOrigin)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys, ok := obj.(*OriginSys)
		if !ok {
			return 0
		}
		return uint32(len(sys.GetPlayerState().PosToEquipMap))
	})

	engine.RegQuestTargetProgress(custom_id.QttTakenOriginEquipStageNum, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) == 0 {
			return 0
		}

		obj := actor.GetSysObj(sysdef.SiOrigin)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys, ok := obj.(*OriginSys)
		if !ok {
			return 0
		}

		var num uint32
		for _, v := range sys.GetPlayerState().PosToEquipMap {
			itemConf := jsondata.GetItemConfig(v.ItemId)
			if itemConf == nil {
				continue
			}

			if itemConf.Stage >= ids[0] {
				num++
			}
		}

		return num
	})

	net.RegisterSysProto(34, 10, sysdef.SiOrigin, (*OriginSys).c2sTakeOn)
	net.RegisterSysProto(34, 11, sysdef.SiOrigin, (*OriginSys).c2sTakeOff)
}
