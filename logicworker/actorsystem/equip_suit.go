/**
 * @Author: yzh
 * @Date:
 * @Desc: 装备套装/装备收集
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type EquipSuiteSys struct {
	Base
}

func (e *EquipSuiteSys) OnLogin() {
	e.S2CDescState()
}

func (e *EquipSuiteSys) OnReconnect() {
	e.S2CDescState()
}

func (e *EquipSuiteSys) S2CDescState() {
	if jsondata.EquipSuitConfMgr == nil {
		return
	}

	msg := &pb3.S2C_11_11{}
	for _, conf := range jsondata.EquipSuitConfMgr {
		confPb := &pb3.EquipSuit{
			Id: conf.Id,
		}

		for _, suit := range conf.Suits {
			suitPb := &pb3.SubEquipSuit{
				Id: suit.Id,
			}

			confPb.SubSuits = append(confPb.SubSuits, suitPb)

			binary := e.owner.GetBinaryData()

			maxEquipNum := uint32(len(suit.Pos))
			if binary.ActiveEquipSuits == nil {
				binary.ActiveEquipSuits = map[uint32]*pb3.ActiveEquipSuitActive{}
			} else if _, ok := binary.ActiveEquipSuits[suit.Id]; ok {
				suitPb.ActiveEquipNum = maxEquipNum
				continue
			}

			if binary.ProcessingEquipSuits == nil {
				binary.ProcessingEquipSuits = map[uint32]*pb3.EquipSuitActiveProcedure{}
				continue
			}

			processing, ok := binary.ProcessingEquipSuits[suit.Id]
			if !ok {
				continue
			}

			suitPb.ActiveEquipNum = processing.ActiveEquipNum
			// fix dirty data
			if suitPb.ActiveEquipNum >= maxEquipNum {
				suitPb.ActiveEquipNum = maxEquipNum
				binary.ActiveEquipSuits[suit.Id] = &pb3.ActiveEquipSuitActive{
					ConfId: conf.Id,
					SuitId: suit.Id,
				}
				delete(binary.ProcessingEquipSuits, suit.Id)
			}
		}

		msg.Suits = append(msg.Suits, confPb)
	}

	e.SendProto3(11, 11, msg)
}

func (e *EquipSuiteSys) c2sActive(msg *base.Message) {
	var req pb3.C2S_11_12
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return
	}

	binary := e.owner.GetBinaryData()

	// already active
	if _, ok := binary.ActiveEquipSuits[req.SubEquipSuitId]; ok {
		return
	}

	conf := jsondata.GetEquipSuitConf(req.EquipSuitId)
	if conf == nil {
		delete(binary.ProcessingEquipSuits, req.SubEquipSuitId)
		delete(binary.ActiveEquipSuits, req.SubEquipSuitId)
		return
	}

	var suit *jsondata.EquipSuit
	for _, suit = range conf.Suits {
		if suit.Id != req.SubEquipSuitId {
			continue
		}
		break
	}
	if suit == nil {
		delete(binary.ProcessingEquipSuits, req.SubEquipSuitId)
		delete(binary.ActiveEquipSuits, req.SubEquipSuitId)
		return
	}

	posSet := map[uint32]struct{}{}
	for _, pos := range suit.Pos {
		posSet[pos] = struct{}{}
	}

	var woreSuitEquipNum uint32
	for _, equip := range e.owner.GetMainData().ItemPool.Equips {
		if _, ok := posSet[equip.Pos]; !ok {
			continue
		}

		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if itemConf == nil {
			continue
		}

		if itemConf.Quality < suit.EquipQualityLimit {
			continue
		}

		if itemConf.Stage < suit.EquipStageLimit {
			continue
		}

		if itemConf.Star < suit.EquipStarLimit {
			continue
		}

		woreSuitEquipNum++
	}

	if woreSuitEquipNum == 0 {
		return
	}

	e.LogDebug("wore:%d", woreSuitEquipNum)

	var activeEquipNum uint32
	processingEquipSuit, ok := binary.ProcessingEquipSuits[req.SubEquipSuitId]
	if ok {
		activeEquipNum = processingEquipSuit.ActiveEquipNum
	}

	if woreSuitEquipNum <= activeEquipNum {
		return
	}

	var newActiveEquipNum uint32
	for _, rewardConf := range suit.DegreeSuitAttrRewards {
		e.LogDebug("need:%d to get reward attrs", rewardConf.EquipNum)

		if rewardConf.EquipNum > woreSuitEquipNum || rewardConf.EquipNum <= activeEquipNum {
			continue
		}

		if newActiveEquipNum == 0 {
			newActiveEquipNum = rewardConf.EquipNum
			continue
		}

		if rewardConf.EquipNum < newActiveEquipNum {
			newActiveEquipNum = rewardConf.EquipNum
		}
	}

	if newActiveEquipNum >= uint32(len(suit.Pos)) {
		if binary.ActiveEquipSuits == nil {
			binary.ActiveEquipSuits = map[uint32]*pb3.ActiveEquipSuitActive{}
		}
		binary.ActiveEquipSuits[req.SubEquipSuitId] = &pb3.ActiveEquipSuitActive{
			SuitId: req.SubEquipSuitId,
			ConfId: req.EquipSuitId,
		}
		delete(binary.ProcessingEquipSuits, req.SubEquipSuitId)
		if suit.SkillId > 0 {
			e.owner.LearnSkill(suit.SkillId, 1, true)
			engine.BroadcastTipMsgById(tipmsgid.EquipSuitStrongTip, e.owner.GetId(), e.owner.GetName(), uint32(len(suit.Pos)), suit.SClassName, suit.SkillName)
		}
	} else {
		if binary.ProcessingEquipSuits == nil {
			binary.ProcessingEquipSuits = map[uint32]*pb3.EquipSuitActiveProcedure{}
		}
		binary.ProcessingEquipSuits[req.SubEquipSuitId] = &pb3.EquipSuitActiveProcedure{
			SuitId:         req.SubEquipSuitId,
			ConfId:         req.EquipSuitId,
			ActiveEquipNum: newActiveEquipNum,
		}
	}
	e.ResetSysAttr(attrdef.SaEquipSuit)

	logArg, _ := json.Marshal(map[string]interface{}{
		"suitId":         req.SubEquipSuitId,
		"confId":         req.EquipSuitId,
		"activeEquipNum": newActiveEquipNum,
	})
	logworker.LogPlayerBehavior(e.owner, pb3.LogId_LogEquipSuitActive, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	e.owner.TriggerQuestEvent(custom_id.QttEquipSuiteActiveSubEquipSuit, req.SubEquipSuitId, int64(newActiveEquipNum))

	e.SendProto3(11, 12, &pb3.S2C_11_12{
		State: &pb3.SubEquipSuit{
			Id:             req.SubEquipSuitId,
			ActiveEquipNum: newActiveEquipNum,
		},
		SuitId: req.EquipSuitId,
	})
}

func calcEquipSuiteSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if jsondata.EquipSuitConfMgr == nil {
		return
	}

	binary := player.GetBinaryData()

	suitIdToConfMap := map[uint32]*jsondata.EquipSuitConf{}
	suitIdToProcessingNum := map[uint32]uint32{}
	for _, activeSuit := range binary.ActiveEquipSuits {
		conf := jsondata.GetEquipSuitConf(activeSuit.ConfId)
		if conf == nil {
			delete(binary.ProcessingEquipSuits, activeSuit.SuitId)
			delete(binary.ActiveEquipSuits, activeSuit.SuitId)
			return
		}
		suitIdToConfMap[activeSuit.SuitId] = conf
	}

	for _, processingSuit := range binary.ProcessingEquipSuits {
		conf := jsondata.GetEquipSuitConf(processingSuit.ConfId)
		if conf == nil {
			delete(binary.ProcessingEquipSuits, processingSuit.SuitId)
			delete(binary.ActiveEquipSuits, processingSuit.SuitId)
			return
		}
		suitIdToProcessingNum[processingSuit.SuitId] = processingSuit.ActiveEquipNum
		suitIdToConfMap[processingSuit.SuitId] = conf
	}

	for suitId := range suitIdToConfMap {
		suit := jsondata.GetEquipSuitByIdFromConf(suitId)

		if suit == nil {
			delete(binary.ProcessingEquipSuits, suitId)
			delete(binary.ActiveEquipSuits, suitId)
			return
		}

		var attrs []*jsondata.Attr
		equipNum := suitIdToProcessingNum[suitId]
		for _, rewardConf := range suit.DegreeSuitAttrRewards {
			if equipNum == 0 || equipNum >= rewardConf.EquipNum {
				attrs = append(attrs, rewardConf.Attrs...)
			}
		}
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func (e *EquipSuiteSys) c2sDescState(msg *base.Message) {
	var req pb3.C2S_11_11
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return
	}
	e.S2CDescState()
}

var _ iface.ISystem = (*EquipSuiteSys)(nil)

func init() {
	engine.RegAttrCalcFn(attrdef.SaEquipSuit, calcEquipSuiteSysAttr)

	RegisterSysClass(sysdef.SiEquipSuit, func() iface.ISystem {
		return &EquipSuiteSys{}
	})

	net.RegisterSysProto(11, 11, sysdef.SiEquipSuit, (*EquipSuiteSys).c2sDescState)
	net.RegisterSysProto(11, 12, sysdef.SiEquipSuit, (*EquipSuiteSys).c2sActive)
}
