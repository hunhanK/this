/**
 * @Author: zjj
 * @Date: 2023/11/14
 * @Desc: 本命法宝
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type DestinedFaBaoSys struct {
	Base
}

func (s *DestinedFaBaoSys) OnOpen() {
	s.resetSysAttr()
	s.s2cInfo()
}

func (s *DestinedFaBaoSys) OnLogin() {
	s.s2cInfo()
}

func (s *DestinedFaBaoSys) OnAfterLogin() {
	s.callToBattle()
}

func (s *DestinedFaBaoSys) OnReconnect() {
	s.callToBattle()
	s.s2cInfo()
}

func (s *DestinedFaBaoSys) getData() *pb3.DestinedFaBao {
	state := s.GetBinaryData().DestinedFaBaoState
	if state == nil {
		s.GetBinaryData().DestinedFaBaoState = &pb3.DestinedFaBao{}
		state = s.GetBinaryData().DestinedFaBaoState
	}
	if state.SkillMap == nil {
		state.SkillMap = make(map[uint32]uint32)
	}
	if state.FashionMgr == nil {
		state.FashionMgr = make(map[uint32]*pb3.DestinedFaBaoFashion)
	}
	if state.ItemId != 0 && state.AppearId == 0 {
		state.AppearId = 1
	}
	return state
}

func (s *DestinedFaBaoSys) s2cInfo() {
	s.SendProto3(163, 0, &pb3.S2C_163_0{
		State: s.getData(),
	})
}

func (s *DestinedFaBaoSys) resetSysAttr() {
	// 重算属性
	s.ResetSysAttr(attrdef.SaDestinedFaBao)
}

func (s *DestinedFaBaoSys) c2sUpLevel(_ *base.Message) error {
	data := s.getData()
	owner := s.GetOwner()
	qualityConf, err := jsondata.GetDestinedFaBaoQualityConf(data.Quality)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	if data.Lv >= qualityConf.MaxLv {
		err := neterror.ParamsInvalidError("the level has reached its maximum , cur quality is %d,cur lv is %d", data.Quality, data.Lv)
		owner.LogWarn("err:%v", err)
		return err
	}

	nextLevelConf, err := jsondata.GetDestinedFaBaoLevelConf(data.Lv + 1)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	if len(nextLevelConf.Consume) > 0 && !owner.ConsumeByConf(nextLevelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoUpLevel}) {
		err := neterror.ConsumeFailedError("consume item up level failed ,cur lv is %d", data.Lv)
		owner.LogWarn("err:%v", err)
		return err
	}

	nextLv := data.Lv + 1
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogDestinedFaBaoUpLevelOpt, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextLv),
	})

	data.Lv = nextLv
	owner.TriggerQuestEvent(custom_id.QttDestinedFaBaoUpLv, 0, 1)
	owner.TriggerQuestEvent(custom_id.QttDestinedFaBaoToLv, 0, int64(data.Lv))
	s.learnSkill()
	s.resetSysAttr()
	s.SendProto3(163, 1, &pb3.S2C_163_1{
		AfterLv: data.Lv,
	})
	owner.UpdateStatics(model.FieldDestinedFaBaoLv_, data.Lv)
	return nil
}

func (s *DestinedFaBaoSys) learnSkill() {
	data := s.getData()
	owner := s.GetOwner()

	conf, err := jsondata.GetDestinedFaBaoConf(data.AppearId)
	if err != nil {
		s.LogError("err:%v", err)
		return
	}

	curLevelConf, err := jsondata.GetDestinedFaBaoLevelConf(data.Lv)
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}

	// 拿到旧的技能ID
	var oldSkillId uint32
	if len(data.SkillMap) > 0 {
		for id := range data.SkillMap {
			oldSkillId = id
			break
		}
	}

	var curSkillId = conf.DefaultSkillId
	var curSkillLv = curLevelConf.SkillLevel

	// 处理下法宝礼包里的技能 需要替换默认的技能
	fbpSkillMap, ok := jsondata.GetDestinedFaBaoSkillPacketConfMgr(data.AppearId)
	// 要遗忘技能
	var needForgetSkillIds pie.Uint32s
	if ok {
		obj := s.GetOwner().GetSysObj(sysdef.SiFaBaoGift)
		var buyId uint64
		if obj != nil && obj.IsOpen() {
			giftSys := obj.(*FaBaoGiftSys)
			data := giftSys.GetData()
			for id := range data.Gift {
				buyId |= 1 << id
			}
		}

		// 理论上礼包全买位移后的值
		maxDestinedFaBaoSkillPacketBitNum := jsondata.GetMaxDestinedFaBaoSkillPacketBitNum()
		buyId &= maxDestinedFaBaoSkillPacketBitNum
		if buyId > 0 {
			val, ok := fbpSkillMap[buyId]
			if ok {
				curSkillId = val
			}
			for _, skillId := range fbpSkillMap {
				if skillId == curSkillId {
					continue
				}
				_, ok := data.SkillMap[skillId]
				if ok {
					needForgetSkillIds = append(needForgetSkillIds, skillId)
				}
			}
		}
	}

	if curSkillId == 0 {
		return
	}

	// 去重
	needForgetSkillIds = needForgetSkillIds.Unique()

	// 学习技能
	data.SkillMap[curSkillId] = curSkillLv
	err = s.GetOwner().CallActorFunc(actorfuncid.SyncDestinedFaBaoLearnSkill, &pb3.LearnSkillSt{
		SkillId: curSkillId,
		SkillLv: curSkillLv,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}

	// 同步一波 cd
	if oldSkillId > 0 && oldSkillId != curSkillId {
		needForgetSkillIds = append(needForgetSkillIds, oldSkillId)
		err = s.GetOwner().CallActorFunc(actorfuncid.SyncDestinedFaBaoOldSkillCd, &pb3.CommonSt{
			U32Param:  oldSkillId,
			U32Param2: curSkillId,
		})
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return
		}
	}

	// 遗忘技能
	err = s.GetOwner().CallActorFunc(actorfuncid.SyncDestinedFaBaoForgetSkill, &pb3.ForgetSkillSt{
		SkillIds: needForgetSkillIds,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
	for _, id := range needForgetSkillIds {
		delete(data.SkillMap, id)
	}

}

func (s *DestinedFaBaoSys) c2sUpQuality(_ *base.Message) error {
	data := s.getData()
	owner := s.GetOwner()
	qualityConf, err := jsondata.GetDestinedFaBaoQualityConf(data.Quality + 1)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	if len(qualityConf.Consume) > 0 {
		if !owner.ConsumeByConf(qualityConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoUpQuality}) {
			err := neterror.ConsumeFailedError("consume item up level failed ,cur lv is %d", data.Lv)
			owner.LogWarn("err:%v", err)
			return err
		}
	}

	nextQuality := data.Quality + 1
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDestinedFaBaoUpQualityOpt, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextQuality),
	})

	data.Quality = nextQuality
	s.resetSysAttr()
	s.SendProto3(163, 2, &pb3.S2C_163_2{
		AfterQuality: data.Quality,
	})

	return nil
}

func (s *DestinedFaBaoSys) c2sResonance(_ *base.Message) error {
	owner := s.GetOwner()
	data := s.getData()
	resonanceConf, err := jsondata.GetDestinedFaBaoResonanceConf(data.ResonanceLv + 1)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	var totalLv = data.Lv
	obj := owner.GetSysObj(sysdef.SiNewFabao)
	if obj != nil && obj.IsOpen() {
		if fbSys, ok := obj.(*FaBaoSys); ok {
			totalLv += fbSys.GetTotalLv()
		}
	}

	if resonanceConf.Lv > totalLv {
		err := neterror.ParamsInvalidError("there are not enough levels, totalLv is %d, destined fb lv is %d,need lv is %d", totalLv, data.Lv, resonanceConf.Lv)
		owner.LogWarn("err:%v", err)
		return err
	}

	nextResonanceLv := data.ResonanceLv + 1
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogDestinedFaBaoUpResonanceOpt, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextResonanceLv),
	})

	data.ResonanceLv = nextResonanceLv
	s.resetSysAttr()
	s.SendProto3(163, 3, &pb3.S2C_163_3{
		AfterResonanceLv: data.ResonanceLv,
	})

	return nil
}

func (s *DestinedFaBaoSys) callToBattle() {
	data := s.getData()
	owner := s.GetOwner()
	if data.ItemId == 0 {
		return
	}

	err := owner.CallActorFunc(actorfuncid.DestinedFaBaoToBattle, &pb3.BattleSt{
		ItemId:   data.ItemId,
		SkillMap: data.SkillMap,
	})
	if err != nil {
		owner.LogError("err:%v", err)
		return
	}
}

func (s *DestinedFaBaoSys) c2sChangeAppear(msg *base.Message) error {
	var req pb3.C2S_163_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	if s.getData().ItemId == 0 {
		return nil
	}

	return s.changeAppear(req.Id)
}
func (s *DestinedFaBaoSys) changeAppear(id uint32) error {
	data := s.getData()
	if data.ItemId == 0 {
		return nil
	}

	conf, err := jsondata.GetDestinedFaBaoConf(id)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	_, ok := s.getData().FashionMgr[id]
	if conf.Type == jsondata.DestinedFaBaoTypeByFashion && !ok {
		return neterror.ParamsInvalidError("un active fashion :%d", id)
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogDestinedFaBaoChangeAppear, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})

	s.GetOwner().TakeOnAppear(appeardef.AppearPos_DestinedFaBao, &pb3.SysAppearSt{
		SysId:    appeardef.AppearSys_DestinedFabao,
		AppearId: id,
	}, true)
	data.AppearId = id
	s.learnSkill()
	s.SendProto3(163, 4, &pb3.S2C_163_4{
		Id:       id,
		SkillMap: data.SkillMap,
	})
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeNewFaBao, s.GetOwner(), s.GetOwner().GetYYRankValue(uint32(ranktype.PlayScoreRankTypeNewFaBao)), false, 0)
	return nil
}

func (s *DestinedFaBaoSys) c2sUpStarFashion(msg *base.Message) error {
	var req pb3.C2S_163_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id := req.Id
	fashionConf, err := jsondata.GetDestinedFaBaoFashionConf(id)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	fashion, ok := s.getData().FashionMgr[id]
	if !ok {
		return neterror.ParamsInvalidError("un active fashion :%d", id)
	}

	oldStar := fashion.Star
	nextStar := oldStar + 1

	conf, ok := fashionConf.FashionStarConf[nextStar]
	if !ok {
		return neterror.ConfNotFoundError("next star %d conf not found", nextStar)
	}

	if len(conf.Consume) > 0 && !s.GetOwner().ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoUpStarFashionConsume}) {
		return neterror.ConfNotFoundError("next star %d consume failed", nextStar)
	}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogDestinedFaBaoUpStarFashion, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	fashion.Star = nextStar
	s.resetSysAttr()
	s.SendProto3(163, 5, &pb3.S2C_163_5{
		Id:   id,
		Star: nextStar,
	})
	return nil
}

func (s *DestinedFaBaoSys) c2sActiveFashion(msg *base.Message) error {
	var req pb3.C2S_163_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unpack msg failed: %v", err)
	}

	id := req.Id
	fashionConf, err := jsondata.GetDestinedFaBaoFashionConf(id)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	_, ok := s.getData().FashionMgr[id]
	if ok {
		return neterror.ParamsInvalidError("already active fashion :%d", id)
	}

	if !s.GetOwner().ConsumeByConf(jsondata.ConsumeVec{
		{
			Id:         fashionConf.ActivateItemId,
			Count:      1,
			CanAutoBuy: false,
		},
	}, false, common.ConsumeParams{LogId: pb3.LogId_LogDestinedFaBaoActiveFashionConsume}) {
		return neterror.ConfNotFoundError("active %d consume failed", id)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogDestinedFaBaoActiveFashion, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
	})
	s.getData().FashionMgr[id] = &pb3.DestinedFaBaoFashion{
		Star: 0,
	}
	s.owner.TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
		SetId:     fashionConf.SetId,
		FType:     fashionConf.FType,
		FashionId: id,
	})
	s.resetSysAttr()
	s.SendProto3(163, 6, &pb3.S2C_163_6{
		Id: id,
	})
	return nil
}

func handleActiveDestinedFaBao(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys := player.GetSysObj(sysdef.SiDestinedFaBao)

	if sys == nil {
		return
	}

	if !sys.IsOpen() {
		return
	}

	destinedFaBaoSys, ok := sys.(*DestinedFaBaoSys)
	if !ok {
		return
	}

	data := destinedFaBaoSys.getData()
	if data.ItemId == param.ItemId {
		player.LogWarn("already active destined fa bao , itemId is %d", data.ItemId)
		return
	}
	destinedFaBaoConf, err := jsondata.GetDestinedFaBaoQualityConf(1)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	// 激活道具
	data.ItemId = param.ItemId

	// 激活品质
	data.Quality = destinedFaBaoConf.Quality
	data.Lv = 1

	// 重算属性
	destinedFaBaoSys.resetSysAttr()

	// 出战
	destinedFaBaoSys.callToBattle()

	// 学习技能
	destinedFaBaoSys.learnSkill()

	player.TriggerQuestEvent(custom_id.QttDestinedFaBaoToLv, 0, int64(data.Lv))

	player.SendProto3(163, 10, &pb3.S2C_163_10{
		State: destinedFaBaoSys.getData(),
	})

	destinedFaBaoSys.changeAppear(1)
	return true, true, 1
}

func calcSaDestinedFaBaoSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiDestinedFaBao)

	if sys == nil {
		return
	}

	if !sys.IsOpen() {
		return
	}

	destinedFaBaoSys, ok := sys.(*DestinedFaBaoSys)
	if !ok {
		return
	}

	data := destinedFaBaoSys.getData()

	if data.ItemId == 0 {
		return
	}

	var addAttrFunc = func(attrs jsondata.AttrVec) {
		if len(attrs) > 0 {
			engine.CheckAddAttrsToCalc(player, calc, attrs)
		}
	}

	levelConf, err := jsondata.GetDestinedFaBaoLevelConf(data.Lv)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	qualityConf, err := jsondata.GetDestinedFaBaoQualityConf(data.Quality)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	addAttrFunc(levelConf.AttrVes)
	addAttrFunc(levelConf.DestinedAttrVes)
	addAttrFunc(qualityConf.AttrVes)
	addAttrFunc(qualityConf.DestinedAttrVes)

	resonanceConf, err := jsondata.GetDestinedFaBaoResonanceConf(data.ResonanceLv)
	if err == nil {
		addAttrFunc(resonanceConf.AttrVes)
		addAttrFunc(resonanceConf.DestinedAttrVes)
	}

	for id, fashion := range data.FashionMgr {
		conf, err := jsondata.GetDestinedFaBaoFashionConf(id)
		if err != nil {
			player.LogError("err:%v", err)
			continue
		}
		starConf, ok := conf.FashionStarConf[fashion.Star]
		if !ok {
			continue
		}
		addAttrFunc(starConf.AttrVes)
	}
}

func calcSaDestinedFaBaoSysAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiDestinedFaBao)
	if sys == nil || !sys.IsOpen() {
		return
	}
	destinedFaBaoSys, ok := sys.(*DestinedFaBaoSys)
	if !ok {
		return
	}
	data := destinedFaBaoSys.getData()
	if data.ItemId == 0 {
		return
	}
	levelConf, err := jsondata.GetDestinedFaBaoLevelConf(data.Lv)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	addRate := totalSysCalc.GetValue(attrdef.DestinedFaBaoBaseAttrRate)
	if addRate > 0 && levelConf != nil {
		engine.CheckAddAttrsRateRoundingUp(player, calc, levelConf.AttrVes, uint32(addRate))
	}
}

func handleDestinedFaBaoAfterUseItemQuickUpLvAndQuest(player iface.IPlayer, args ...interface{}) {
	bagSys, ok := player.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return
	}
	ret := bagSys.GetAllItemHandleByItemId(11490000)
	for _, hdl := range ret {
		bagSys.UseItem(hdl, 1, []uint32{})
	}
}

func init() {
	RegisterSysClass(sysdef.SiDestinedFaBao, func() iface.ISystem {
		s := &DestinedFaBaoSys{}
		return s
	})

	net.RegisterSysProtoV2(163, 1, sysdef.SiDestinedFaBao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DestinedFaBaoSys).c2sUpLevel
	})
	net.RegisterSysProtoV2(163, 2, sysdef.SiDestinedFaBao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DestinedFaBaoSys).c2sUpQuality
	})
	net.RegisterSysProtoV2(163, 3, sysdef.SiDestinedFaBao, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DestinedFaBaoSys).c2sResonance
	})

	net.RegisterSysProto(163, 4, sysdef.SiDestinedFaBao, (*DestinedFaBaoSys).c2sChangeAppear)
	net.RegisterSysProto(163, 5, sysdef.SiDestinedFaBao, (*DestinedFaBaoSys).c2sUpStarFashion)
	net.RegisterSysProto(163, 6, sysdef.SiDestinedFaBao, (*DestinedFaBaoSys).c2sActiveFashion)

	engine.RegQuestTargetProgress(custom_id.QttDestinedFaBaoToLv, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		sys := actor.GetSysObj(sysdef.SiDestinedFaBao)

		if sys == nil {
			return 0
		}

		if !sys.IsOpen() {
			return 0
		}

		destinedFaBaoSys, ok := sys.(*DestinedFaBaoSys)
		if !ok {
			return 0
		}

		data := destinedFaBaoSys.getData()
		return data.Lv
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemActiveDestinedFaBao, handleActiveDestinedFaBao)

	engine.RegAttrCalcFn(attrdef.SaDestinedFaBao, calcSaDestinedFaBaoSysAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaDestinedFaBao, calcSaDestinedFaBaoSysAttrAddRate)

	event.RegActorEvent(custom_id.AeUpdateSysPowerMap, destinedFaBaoOnUpdateSysPowerMap)
	manager.RegPlayScoreExtValueGetter(ranktype.PlayScoreRankTypeNewFaBao, handlePlayScoreRankTypeNewFaBao)

	event.RegActorEvent(custom_id.AeFaBaoGiftBuyGift, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiDestinedFaBao).(*DestinedFaBaoSys)
		if !ok || !sys.IsOpen() {
			return
		}
		if sys.getData().ItemId == 0 {
			return
		}
		sys.learnSkill()
		sys.s2cInfo()
	})

	event.RegActorEvent(custom_id.AeLoginFight, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiDestinedFaBao).(*DestinedFaBaoSys)
		if !ok || !sys.IsOpen() {
			return
		}
		if sys.getData().ItemId > 0 {
			sys.callToBattle()
		}
	})

	engine.RegisterActorCallFunc(playerfuncid.SyncDestinedFaBaoToBattle, func(player iface.IPlayer, _ []byte) {
		sys, ok := player.GetSysObj(sysdef.SiDestinedFaBao).(*DestinedFaBaoSys)
		if !ok || !sys.IsOpen() {
			return
		}
		if sys.getData().ItemId > 0 {
			sys.callToBattle()
		}
	})
	event.RegActorEvent(custom_id.AeAfterUseItemQuickUpLvAndQuest, handleDestinedFaBaoAfterUseItemQuickUpLvAndQuest)
}

func destinedFaBaoOnUpdateSysPowerMap(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		player.LogError("len of args is nil")
		return
	}

	powerMap := args[0].(map[uint32]int64)
	collectIds := []uint32{attrdef.SaNewFaBao, attrdef.AtDestinedFaBaoEngrave, attrdef.SaDestinedFaBao, attrdef.AtQiLingPrivilege, attrdef.SaFaBaoGift}
	sumPower := int64(0)
	for _, id := range collectIds {
		sumPower += powerMap[id]
	}
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeNewFaBao, player, sumPower, false, 0)

}

func handlePlayScoreRankTypeNewFaBao(player iface.IPlayer) *pb3.YYFightValueRushRankExt {
	var ext = &pb3.YYFightValueRushRankExt{}
	ext.FaBao = &pb3.YYFightValueRushRankExtFaBao{}
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if ok && sys.IsOpen() {
		for _, newFaBao := range sys.state().BattleSlots {
			ext.FaBao.NewFaBaoMgr = append(ext.FaBao.NewFaBaoMgr, &pb3.NewFaBao{
				Id:      newFaBao.Id,
				Quality: newFaBao.Quality,
				Star:    newFaBao.Star,
				Lv:      newFaBao.Lv,
			})
		}
	}
	sys1, ok := player.GetSysObj(sysdef.SiDestinedFaBao).(*DestinedFaBaoSys)
	if ok && sys1.IsOpen() {
		ext.FaBao.DestinedFaBaoId = sys1.getData().AppearId
	}
	return ext
}
