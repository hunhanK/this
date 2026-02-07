/**
 * @Author: LvYuMeng
 * @Date: 2025/08/25
 * @Desc: 领域
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	DomainOptToBat         = 1 // 空槽位上阵
	DomainOptOldPosReplace = 2 // 原槽位替换
	DomainOptPosReplace    = 3 // 槽位交换
)

type DomainSys struct {
	Base
}

func (s *DomainSys) getData() *pb3.DomainData {
	data := s.GetBinaryData()
	if data.DomainData == nil {
		data.DomainData = &pb3.DomainData{}
	}
	domainData := data.DomainData
	if domainData.DomainMap == nil {
		domainData.DomainMap = map[uint32]*pb3.OnceDomain{}
	}
	if domainData.SuitMap == nil {
		domainData.SuitMap = map[uint32]*pb3.OnceDomainSuit{}
	}
	if domainData.PosMap == nil {
		domainData.PosMap = map[uint32]uint32{}
	}
	return domainData
}

func (s *DomainSys) s2cInfo() {
	s.SendProto3(144, 30, &pb3.S2C_144_30{
		Data: s.getData(),
	})
}

func (s *DomainSys) initBattlePos() {
	data := s.getData()
	owner := s.GetOwner()
	jsondata.EachDomainBattlePosConf(func(config *jsondata.DomainBattlePosConfig) {
		if !config.Init {
			return
		}
		data.PosMap[config.Pos] = 0
		logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveDomainPos, &pb3.LogPlayerCounter{
			NumArgs: uint64(config.Pos),
		})
	})
}

func (s *DomainSys) OnOpen() {
	s.initBattlePos()
	s.ResetSysAttr(attrdef.SaDomain)
	s.s2cInfo()
}

func (s *DomainSys) OnLogin() {
	s.s2cInfo()
}

func (s *DomainSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DomainSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_144_31
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	itemId := req.ItemId
	domainConf := jsondata.GetDomainConf(itemId)
	if domainConf == nil {
		return neterror.ConfNotFoundError("%d not found domain conf", itemId)
	}

	data := s.getData()
	owner := s.GetOwner()
	_, ok := data.DomainMap[itemId]
	if ok {
		return neterror.ParamsInvalidError("%d already active", itemId)
	}

	if !owner.ConsumeByConf(jsondata.ConsumeVec{{
		Id:    itemId,
		Count: 1,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveDomain}) {
		return neterror.ConsumeFailedError("%d not enough", itemId)
	}

	data.DomainMap[itemId] = &pb3.OnceDomain{
		ItemId: itemId,
		Lv:     1,
	}
	s.ResetSysAttr(attrdef.SaDomain)
	s.SendProto3(144, 31, &pb3.S2C_144_31{
		Domain: data.DomainMap[itemId],
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveDomain, &pb3.LogPlayerCounter{
		NumArgs: uint64(itemId),
	})

	err := s.checkActiveSuit(itemId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	return nil
}

func (s *DomainSys) c2sUpLevel(msg *base.Message) error {
	var req pb3.C2S_144_33
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	itemId := req.ItemId
	domainConf := jsondata.GetDomainConf(itemId)
	if domainConf == nil {
		return neterror.ConfNotFoundError("%d not found domain conf", itemId)
	}

	data := s.getData()
	owner := s.GetOwner()
	domainData, ok := data.DomainMap[itemId]
	if !ok {
		return neterror.ParamsInvalidError("%d already active", itemId)
	}

	nextLv := domainData.Lv + 1
	nextLevelConf := domainConf.LevelConf[nextLv]
	if nextLevelConf == nil {
		return neterror.ConfNotFoundError("%d not found %d domain level conf", itemId, nextLv)
	}

	if len(nextLevelConf.Consume) != 0 && !owner.ConsumeByConf(nextLevelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLevelDomain}) {
		return neterror.ConsumeFailedError("%d %d not enough", itemId, nextLv)
	}

	domainData.Lv = nextLv
	s.ResetSysAttr(attrdef.SaDomain)
	s.SendProto3(144, 33, &pb3.S2C_144_33{
		Domain: domainData,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogUpLevelDomain, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextLv),
	})

	err := s.checkActiveSuit(itemId)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	return nil
}

func (s *DomainSys) c2sUnLockPos(msg *base.Message) error {
	var req pb3.C2S_144_35
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	pos := req.Pos
	posConf := jsondata.GetDomainBattlePosConf(pos)
	if posConf == nil {
		return neterror.ConfNotFoundError("%d not found domain pos conf", pos)
	}

	data := s.getData()
	owner := s.GetOwner()
	_, ok := data.PosMap[pos]
	if ok {
		return neterror.ParamsInvalidError("%d already active", pos)
	}

	if len(posConf.Consume) != 0 && !owner.ConsumeByConf(posConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogActiveDomainPos}) {
		return neterror.ConsumeFailedError("%d not enough", pos)
	}

	data.PosMap[pos] = 0
	s.SendProto3(144, 35, &pb3.S2C_144_35{
		Pos: pos,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveDomainPos, &pb3.LogPlayerCounter{
		NumArgs: uint64(pos),
	})

	return nil
}

func (s *DomainSys) c2sToBat(msg *base.Message) error {
	var req pb3.C2S_144_36
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	pos := req.Pos
	suitId := req.SuitId
	owner := s.GetOwner()

	suitConf := jsondata.GetDomainSuitConf(suitId)
	if suitConf == nil {
		return neterror.ConfNotFoundError("%d domain suit conf not found", suitId)
	}

	data := s.getData()
	_, ok := data.PosMap[pos]
	if !ok {
		return neterror.ParamsInvalidError("pos %d not can use", pos)
	}

	suitData, ok := data.SuitMap[suitId]
	if !ok {
		return neterror.ParamsInvalidError("suit %d not can use", suitId)
	}

	var err error
	opt := req.Opt
	switch opt {
	case DomainOptToBat:
		err = s.onDomainOptToBat(pos, suitId)
	case DomainOptOldPosReplace:
		err = s.onDomainOptOldPosReplace(pos, suitId)
	case DomainOptPosReplace:
		err = s.onDomainOptPosReplace(pos, suitId)
	default:
		err = neterror.ParamsInvalidError("not found opt %d", opt)
	}
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	jsondata.EachDomainSuitStarConfDo(suitId, suitData.Star, func(skillId, skillLv uint32) {
		owner.LearnSkill(skillId, skillLv, true)
	})

	// 通知战斗服最新的上阵信息
	err = owner.CallActorFunc(actorfuncid.G2FSyncDomainPos, &pb3.G2FSyncDomainPosSt{
		PosMap: data.PosMap,
	})
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	owner.SendProto3(144, 36, &pb3.S2C_144_36{
		PosMap: data.PosMap,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogToBatDomainSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(opt),
		StrArgs: fmt.Sprintf("%d_%d", pos, suitId),
	})
	return nil
}

func (s *DomainSys) onDomainOptToBat(pos uint32, suitId uint32) error {
	data := s.getData()

	oldSuitId := data.PosMap[pos]
	if oldSuitId != 0 {
		return neterror.ParamsInvalidError("pos:%d have suitId:%d", pos, oldSuitId)
	}

	data.PosMap[pos] = suitId
	data.SuitMap[suitId].Pos = pos
	return nil
}

func (s *DomainSys) onDomainOptOldPosReplace(pos uint32, suitId uint32) error {
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

func (s *DomainSys) onDomainOptPosReplace(newPos uint32, newSuitId uint32) error {
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

func (s *DomainSys) takeOffPos(pos uint32) error {
	data := s.getData()
	suitId := data.PosMap[pos]
	if suitId == 0 {
		return nil
	}

	suitData, ok := data.SuitMap[suitId]
	if !ok {
		return neterror.ParamsInvalidError("suitId:%d not found", suitId)
	}

	owner := s.GetOwner()

	jsondata.EachDomainSuitStarConfDo(suitId, suitData.Star, func(skillId, skillLv uint32) {
		owner.ForgetSkill(skillId, true, true, true)
	})

	// 清空槽位
	data.PosMap[pos] = 0
	data.SuitMap[suitId].Pos = 0
	return nil
}

func (s *DomainSys) checkActiveSuit(itemId uint32) error {
	domainConf := jsondata.GetDomainConf(itemId)
	if domainConf == nil {
		return neterror.ConfNotFoundError("%d not found domain conf to active suit", itemId)
	}

	suitId := domainConf.SuitId
	suitConf := jsondata.GetDomainSuitConf(suitId)
	if suitConf == nil {
		return neterror.ConfNotFoundError("%d not found domain suit conf to active suit %d", itemId, suitId)
	}

	data := s.getData()
	owner := s.GetOwner()

	if len(suitConf.DomainIds) == 0 {
		return neterror.ConfNotFoundError("%d not found domain ids ", suitId)
	}

	var suitStar uint32
	for i, domainId := range suitConf.DomainIds {
		domain, isActive := data.DomainMap[domainId]
		if !isActive {
			owner.LogInfo("%d suit %d domain not active", suitId, domainId)
			return nil
		}

		if i == 0 || suitStar > domain.Lv {
			suitStar = domain.Lv
		}
	}

	if suitStar == 0 {
		return nil
	}

	suit, ok := data.SuitMap[suitId]
	if !ok {
		suit = &pb3.OnceDomainSuit{
			SuitId:        suitId,
			ActiveSkillId: suitConf.ActiveSkillId,
		}
		data.SuitMap[suitId] = suit
	}

	if suit.Star >= suitStar {
		return nil
	}

	suit.Star = suitStar
	s.SendProto3(144, 32, &pb3.S2C_144_32{
		Suit: data.SuitMap[suitId],
	})

	if suit.Pos > 0 {
		jsondata.EachDomainSuitStarConfDo(suitId, suit.Star, func(skillId, skillLv uint32) {
			owner.LearnSkill(skillId, skillLv, true)
		})
	}

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActiveDomainSuit, &pb3.LogPlayerCounter{
		NumArgs: uint64(suitId),
	})
	return nil
}

func (s *DomainSys) SaveSkillData(furry uint32, pos uint32) {
	data := s.getData()
	data.Furry = furry
	data.ReadyPos = pos
}

func (s *DomainSys) PackFightSrvDomainInfo(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}
	createData.DomainInfo = &pb3.DomainInfo{}
	if !s.IsOpen() {
		return
	}
	data := s.getData()
	createData.DomainInfo.PosMap = data.PosMap
	createData.DomainInfo.ReadyPos = data.ReadyPos
	createData.DomainInfo.Furry = data.Furry
}

func calcDomainSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiDomain)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*DomainSys)
	data := sys.getData()
	for _, domain := range data.DomainMap {
		conf := jsondata.GetDomainConf(domain.ItemId)
		if conf == nil {
			continue
		}
		levelConf := conf.LevelConf[domain.Lv]
		if levelConf == nil {
			continue
		}
		if len(levelConf.Attrs) == 0 {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, levelConf.Attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiDomain, func() iface.ISystem {
		return &DomainSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaDomain, calcDomainSysAttr)
	net.RegisterSysProtoV2(144, 31, sysdef.SiDomain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSys).c2sActive
	})
	net.RegisterSysProtoV2(144, 33, sysdef.SiDomain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSys).c2sUpLevel
	})
	net.RegisterSysProtoV2(144, 35, sysdef.SiDomain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSys).c2sUnLockPos
	})
	net.RegisterSysProtoV2(144, 36, sysdef.SiDomain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DomainSys).c2sToBat
	})

	gmevent.Register("domainSys.unLockAllPos", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiDomain)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*DomainSys)
		jsondata.EachDomainBattlePosConf(func(config *jsondata.DomainBattlePosConfig) {
			sys.getData().PosMap[config.Pos] = 0
		})
		sys.s2cInfo()
		return true
	}, 1)
	gmevent.Register("domainSys.unAllDomain", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiDomain)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*DomainSys)
		jsondata.EachDomainConf(func(config *jsondata.DomainConfig) {
			sys.getData().DomainMap[config.ItemId] = &pb3.OnceDomain{
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
