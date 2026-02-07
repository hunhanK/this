/**
 * @Author: zjj
 * @Date: 2024/9/11
 * @Desc: 法则
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	LawOptToBat         = 1 // 空槽位上阵
	LawOptOldPosReplace = 2 // 原槽位替换
	LawOptPosReplace    = 3 // 槽位交换
)

type LawSys struct {
	Base
}

func (s *LawSys) getData() *pb3.LawData {
	data := s.GetBinaryData()
	if data.LawData == nil {
		data.LawData = &pb3.LawData{}
	}
	lawData := data.LawData
	if lawData.LawMap == nil {
		lawData.LawMap = map[uint32]*pb3.OnceLaw{}
	}
	if lawData.SuitMap == nil {
		lawData.SuitMap = map[uint32]*pb3.OnceLawSuit{}
	}
	if lawData.PosMap == nil {
		lawData.PosMap = map[uint32]uint32{}
	}
	return lawData
}

func (s *LawSys) s2cInfo() {
	s.SendProto3(144, 20, &pb3.S2C_144_20{
		Data: s.getData(),
	})
}

func (s *LawSys) initBattlePos() {
	data := s.getData()
	owner := s.GetOwner()
	jsondata.EachLawBattlePosConf(func(config *jsondata.LawBattlePosConfig) {
		if !config.Init {
			return
		}
		data.PosMap[config.Pos] = 0
		logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveLawPos, &pb3.LogPlayerCounter{
			NumArgs: uint64(config.Pos),
		})
	})
}

func (s *LawSys) OnOpen() {
	s.initBattlePos()
	s.checkVipUnLockPos()
	s.ResetSysAttr(attrdef.SaLaw)
	s.s2cInfo()
}
func (s *LawSys) OnLogin() {
	s.learnAppearSuitPassivitySkill()
	s.s2cInfo()
}
func (s *LawSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LawSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_144_21
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	itemId := req.ItemId
	lawConf := jsondata.GetLawConf(itemId)
	if lawConf == nil {
		return neterror.ConfNotFoundError("%d not found law conf", itemId)
	}

	data := s.getData()
	owner := s.GetOwner()
	_, ok := data.LawMap[itemId]
	if ok {
		return neterror.ParamsInvalidError("%d already active", itemId)
	}

	if !owner.ConsumeByConf(jsondata.ConsumeVec{{
		Id:    itemId,
		Count: 1,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveLaw}) {
		return neterror.ConsumeFailedError("%d not enough", itemId)
	}

	data.LawMap[itemId] = &pb3.OnceLaw{
		ItemId: itemId,
		Lv:     1,
	}
	s.ResetSysAttr(attrdef.SaLaw)
	s.SendProto3(144, 21, &pb3.S2C_144_21{
		Law: data.LawMap[itemId],
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveLaw, &pb3.LogPlayerCounter{
		NumArgs: uint64(itemId),
	})

	err := s.checkActiveSuit(itemId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	return nil
}

func (s *LawSys) c2sUpLevel(msg *base.Message) error {
	var req pb3.C2S_144_23
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	itemId := req.ItemId
	lawConf := jsondata.GetLawConf(itemId)
	if lawConf == nil {
		return neterror.ConfNotFoundError("%d not found law conf", itemId)
	}

	data := s.getData()
	owner := s.GetOwner()
	lawData, ok := data.LawMap[itemId]
	if !ok {
		return neterror.ParamsInvalidError("%d already active", itemId)
	}

	nextLv := lawData.Lv + 1
	nextLevelConf := lawConf.LevelConf[nextLv]
	if nextLevelConf == nil {
		return neterror.ConfNotFoundError("%d not found %d law level conf", itemId, nextLv)
	}

	if len(nextLevelConf.Consume) != 0 && !owner.ConsumeByConf(nextLevelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLevelLaw}) {
		return neterror.ConsumeFailedError("%d %d not enough", itemId, nextLv)
	}

	lawData.Lv = nextLv
	s.ResetSysAttr(attrdef.SaLaw)
	s.SendProto3(144, 23, &pb3.S2C_144_23{
		Law: lawData,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogUpLevelLaw, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextLv),
	})

	return nil
}

func (s *LawSys) c2sAppear(msg *base.Message) error {
	var req pb3.C2S_144_24
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	skipAppear := req.SkipBroadcastAppear
	suitId := req.SuitId

	data := s.getData()
	appearSuitId := data.AppearSuitId

	// 卸下旧的幻化 幻化新的
	if suitId != 0 && appearSuitId != suitId {
		s.takeOffAppear(pb3.LogId_LogAppearLawSuitTakeOffOldSuit)
		s.takeOnAppear(suitId, skipAppear)
	}

	data.SkipBroadcastAppear = skipAppear
	s.learnAppearSuitPassivitySkill()
	s.SendProto3(144, 24, &pb3.S2C_144_24{
		SuitId:              req.SuitId,
		SkipBroadcastAppear: req.SkipBroadcastAppear,
	})
	return nil
}

func (s *LawSys) c2sUnLockPos(msg *base.Message) error {
	var req pb3.C2S_144_25
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	pos := req.Pos
	posConf := jsondata.GetLawBattlePosConf(pos)
	if posConf == nil {
		return neterror.ConfNotFoundError("%d not found law pos conf", pos)
	}

	data := s.getData()
	owner := s.GetOwner()
	_, ok := data.PosMap[pos]
	if ok {
		return neterror.ParamsInvalidError("%d already active", pos)
	}
	if posConf.Vip != 0 && owner.GetVipLevel() < posConf.Vip {
		return neterror.ParamsInvalidError("%d need vip lv %d", pos, posConf.Vip)
	}

	if len(posConf.Consume) != 0 && !owner.ConsumeByConf(posConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveLawPos}) {
		return neterror.ConsumeFailedError("%d not enough", pos)
	}

	data.PosMap[pos] = 0
	s.SendProto3(144, 25, &pb3.S2C_144_25{
		Pos: pos,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveLawPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
	})

	return nil
}

func (s *LawSys) checkVipUnLockPos() bool {
	handleLawSysAeVipLevelUp(s.GetOwner())
	return true
}

func (s *LawSys) c2sToBat(msg *base.Message) error {
	var req pb3.C2S_144_26
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	pos := req.Pos
	suitId := req.SuitId
	owner := s.GetOwner()

	suitConf := jsondata.GetLawSuitConf(suitId)
	if suitConf == nil {
		return neterror.ConfNotFoundError("%d law suit conf not found", suitId)
	}

	data := s.getData()
	_, ok := data.PosMap[pos]
	if !ok {
		return neterror.ParamsInvalidError("pos %d not can use", pos)
	}

	_, ok = data.SuitMap[suitId]
	if !ok {
		return neterror.ParamsInvalidError("suit %d not can use", suitId)
	}

	var err error
	opt := req.Opt
	switch opt {
	case LawOptToBat:
		err = s.onLawOptToBat(pos, suitId)
	case LawOptOldPosReplace:
		err = s.onLawOptOldPosReplace(pos, suitId)
	case LawOptPosReplace:
		err = s.onLawOptPosReplace(pos, suitId)
	default:
		err = neterror.ParamsInvalidError("not found opt %d", opt)
	}
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 学习新上阵的主动技能
	if suitConf.ActiveSkillId != 0 {
		owner.LearnSkill(suitConf.ActiveSkillId, 1, true)
	}

	// 通知战斗服最新的上阵信息
	err = owner.CallActorFunc(actorfuncid.G2FSyncLawPos, &pb3.G2FSyncLawPosSt{
		PosMap: data.PosMap,
	})
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	owner.SendProto3(144, 26, &pb3.S2C_144_26{
		PosMap: data.PosMap,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogToBatLawSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(opt),
		StrArgs: fmt.Sprintf("%d_%d", pos, suitId),
	})
	return nil
}

func (s *LawSys) onLawOptToBat(pos uint32, suitId uint32) error {
	data := s.getData()

	oldSuitId := data.PosMap[pos]
	if oldSuitId != 0 {
		return neterror.ParamsInvalidError("pos:%d have suitId:%d", pos, oldSuitId)
	}

	data.PosMap[pos] = suitId
	data.SuitMap[suitId].Pos = pos
	return nil
}

func (s *LawSys) onLawOptOldPosReplace(pos uint32, suitId uint32) error {
	data := s.getData()
	owner := s.GetOwner()

	oldSuitId := data.PosMap[pos]
	if oldSuitId == 0 {
		return neterror.ParamsInvalidError("pos:%d not suitId:%d", pos, oldSuitId)
	}

	err := s.takeOffPos(pos)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	data.PosMap[pos] = suitId
	data.SuitMap[suitId].Pos = pos
	return nil
}

func (s *LawSys) onLawOptPosReplace(newPos uint32, newSuitId uint32) error {
	data := s.getData()

	targetPosSuitId := data.PosMap[newPos]
	oldPos := data.SuitMap[newSuitId].Pos

	// 双重冗余判断
	if targetPosSuitId != 0 && targetPosSuitId == newSuitId {
		return neterror.ParamsInvalidError("newPos:%d,targetPosSuitId:%d,newSuitId:%d", newPos, targetPosSuitId, newSuitId)
	}

	if oldPos != 0 && oldPos == newPos {
		return neterror.ParamsInvalidError("newSuitId:%d,oldPos:%d,newPos:%d", newSuitId, oldPos, newPos)
	}

	// 直接交换
	data.PosMap[newPos] = newSuitId
	data.SuitMap[newSuitId].Pos = newPos

	// 旧槽位处理
	if oldPos != 0 {
		data.PosMap[oldPos] = targetPosSuitId
		if targetPosSuitId != 0 {
			data.SuitMap[targetPosSuitId].Pos = oldPos
		}
	}

	return nil
}

func (s *LawSys) takeOffPos(pos uint32) error {
	data := s.getData()
	suitId := data.PosMap[pos]
	if suitId == 0 {
		return nil
	}

	// 取消学习主动技能
	suitConf := jsondata.GetLawSuitConf(suitId)
	if suitConf != nil {
		owner := s.GetOwner()
		owner.ForgetSkill(suitConf.ActiveSkillId, true, true, true)
	}

	// 清空槽位
	data.PosMap[pos] = 0
	data.SuitMap[suitId].Pos = 0
	return nil
}

func (s *LawSys) takeOffAppear(logId pb3.LogId) bool {
	data := s.getData()
	owner := s.GetOwner()
	appearSuitId := data.AppearSuitId

	if appearSuitId == 0 {
		return false
	}

	owner.TakeOffAppear(appeardef.AppearPos_Law)

	logworker.LogPlayerBehavior(owner, logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(appearSuitId),
	})
	return true
}

func (s *LawSys) takeOnAppear(suitId uint32, skipAppear bool) {
	if !skipAppear {
		s.GetOwner().TakeOnAppear(appeardef.AppearPos_Law, &pb3.SysAppearSt{
			SysId:    appeardef.AppearSys_Law,
			AppearId: suitId,
		}, true)
	}

	s.changeActiveSkillId(suitId)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogAppearLawSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(suitId),
	})
}

func (s *LawSys) checkActiveSuit(itemId uint32) error {
	lawConf := jsondata.GetLawConf(itemId)
	if lawConf == nil {
		return neterror.ConfNotFoundError("%d not found law conf to active suit", itemId)
	}

	suitId := lawConf.SuitId
	suitConf := jsondata.GetLawSuitConf(suitId)
	if suitConf == nil {
		return neterror.ConfNotFoundError("%d not found law suit conf to active suit %d", itemId, suitId)
	}

	data := s.getData()
	owner := s.GetOwner()
	_, ok := data.SuitMap[suitId]
	if ok {
		owner.LogInfo("suit %d already active", suitId)
		return nil
	}

	if len(suitConf.LawIds) == 0 {
		return neterror.ConfNotFoundError("%d not found law ids ", suitId)
	}
	var canActive = true
	for _, lawId := range suitConf.LawIds {
		_, ok := data.LawMap[lawId]
		if ok {
			continue
		}
		owner.LogInfo("%d suit %d law not active", suitId, lawId)
		canActive = false
		break
	}
	if !canActive {
		return nil
	}

	data.SuitMap[suitId] = &pb3.OnceLawSuit{
		SuitId:           suitId,
		ActiveSkillId:    suitConf.ActiveSkillId,
		PassivitySkillId: suitConf.PassivitySkillId,
	}
	s.SendProto3(144, 22, &pb3.S2C_144_22{
		Suit: data.SuitMap[suitId],
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveLawSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(suitId),
	})
	return nil
}

func (s *LawSys) learnAppearSuitPassivitySkill() {
	data := s.getData()

	conf := jsondata.GetLawSuitConf(data.AppearSuitId)
	if conf == nil {
		return
	}

	if conf.PassivitySkillId == 0 {
		return
	}
	s.GetOwner().LearnSkill(conf.PassivitySkillId, 1, true)
}

func (s *LawSys) changeActiveSkillId(newSuitId uint32) {
	data := s.getData()
	owner := s.GetOwner()
	oldSuitId := data.AppearSuitId
	oldSuitConf := jsondata.GetLawSuitConf(oldSuitId)
	if oldSuitConf != nil {
		owner.ForgetSkill(oldSuitConf.PassivitySkillId, true, true, true)
	}

	newSuitConf := jsondata.GetLawSuitConf(newSuitId)
	if newSuitConf != nil {
		owner.LearnSkill(newSuitConf.PassivitySkillId, 1, true)
	}

	data.AppearSuitId = newSuitId
}

func (s *LawSys) SaveSkillData(furry uint32, pos uint32) {
	data := s.getData()
	data.Furry = furry
	data.ReadyPos = pos
}

func (s *LawSys) PackFightSrvLawInfo(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}
	createData.LawInfo = &pb3.LawInfo{}
	if !s.IsOpen() {
		return
	}
	data := s.getData()
	createData.LawInfo.PosMap = data.PosMap
	createData.LawInfo.ReadyPos = data.ReadyPos
	createData.LawInfo.Furry = data.Furry
}

func calcLawSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiLaw)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*LawSys)
	data := sys.getData()
	for _, law := range data.LawMap {
		conf := jsondata.GetLawConf(law.ItemId)
		if conf == nil {
			continue
		}
		levelConf := conf.LevelConf[law.Lv]
		if levelConf == nil {
			continue
		}
		if len(levelConf.Attrs) == 0 {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, levelConf.Attrs)
	}
}

func handleLawSysAeVipLevelUp(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiLaw)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*LawSys)
	data := sys.getData()
	jsondata.EachLawBattlePosConf(func(config *jsondata.LawBattlePosConfig) {
		if config.Vip == 0 || config.Vip > player.GetVipLevel() {
			return
		}
		if len(config.Consume) > 0 {
			return
		}
		_, ok := data.PosMap[config.Pos]
		if ok {
			return
		}
		data.PosMap[config.Pos] = 0
		player.SendProto3(144, 25, &pb3.S2C_144_25{
			Pos: config.Pos,
		})
		logworker.LogPlayerBehavior(player, pb3.LogId_LogActiveLawPos, &pb3.LogPlayerCounter{
			NumArgs: uint64(config.Pos),
		})
	})
}

func init() {
	RegisterSysClass(sysdef.SiLaw, func() iface.ISystem {
		return &LawSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaLaw, calcLawSysAttr)
	net.RegisterSysProtoV2(144, 21, sysdef.SiLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LawSys).c2sActive
	})
	net.RegisterSysProtoV2(144, 23, sysdef.SiLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LawSys).c2sUpLevel
	})
	net.RegisterSysProtoV2(144, 24, sysdef.SiLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LawSys).c2sAppear
	})
	net.RegisterSysProtoV2(144, 25, sysdef.SiLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LawSys).c2sUnLockPos
	})
	net.RegisterSysProtoV2(144, 26, sysdef.SiLaw, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LawSys).c2sToBat
	})
	event.RegActorEventL(custom_id.AeVipLevelUp, handleLawSysAeVipLevelUp)
	gmevent.Register("lawSys.unLockAllPos", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiLaw)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*LawSys)
		jsondata.EachLawBattlePosConf(func(config *jsondata.LawBattlePosConfig) {
			sys.getData().PosMap[config.Pos] = 0
		})
		sys.s2cInfo()
		return true
	}, 1)
	gmevent.Register("lawSys.unAllLaw", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiLaw)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*LawSys)
		jsondata.EachLawConf(func(config *jsondata.LawConfig) {
			sys.getData().LawMap[config.ItemId] = &pb3.OnceLaw{
				ItemId: config.ItemId,
				Lv:     1,
			}
			err := sys.checkActiveSuit(config.ItemId)
			if err != nil {
				player.LogWarn("err:%v", err)
			}
		})
		sys.s2cInfo()
		return true
	}, 1)
}
