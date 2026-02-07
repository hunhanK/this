/**
 * @Author: yzh
 * @Date:
 * @Desc: 经络
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type MeridianSys struct {
	Base
}

func (s *MeridianSys) OnOpen() {
	s.ResetSysAttr(attrdef.SaMeridian)
}

func (s *MeridianSys) OnLogin() {
	s.owner.SendProto3(129, 1, &pb3.S2C_129_1{
		State: s.State(),
	})
}

func (s *MeridianSys) OnReconnect() {
	s.owner.SendProto3(129, 1, &pb3.S2C_129_1{
		State: s.State(),
	})
}

func (s *MeridianSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(s.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: coreNumData,
		StrArgs: string(bytes),
	})
}

func (s *MeridianSys) State() *pb3.MeridianState {
	state := s.owner.GetBinaryData().MeridianState
	if state == nil {
		state = &pb3.MeridianState{
			KindLevelMap: map[uint32]uint32{},
		}
		s.owner.GetBinaryData().MeridianState = state
	}
	if state.KindLevelMap == nil {
		state.KindLevelMap = map[uint32]uint32{}
	}
	return state
}

func (s *MeridianSys) c2sUpLevel(msg *base.Message) {
	var req pb3.C2S_129_1
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return
	}
	owner := s.GetOwner()
	s.LogPlayerBehavior(uint64(req.Kind), map[string]interface{}{}, pb3.LogId_LogMeridianUpLevel)

	kindConf := jsondata.GetMeridianKindConf(req.Kind)
	if kindConf == nil {
		owner.LogWarn("not such kind:%d", req.Kind)
		return
	}

	jingjieLV := s.owner.GetExtraAttrU32(attrdef.Circle)

	if kindConf.OpenJingJieLevel > jingjieLV {
		owner.LogWarn("jingjie level not reach")
		return
	}

	state := s.State()
	curLevel := state.KindLevelMap[req.Kind]
	nextLevel := curLevel + 1
	maxLv := len(kindConf.KindCfg) - 1

	if maxLv < int(nextLevel) {
		owner.LogWarn("overflow kind max level")
		return
	}

	nextLevelConf := kindConf.KindCfg[nextLevel]
	owner.LogTrace("cur level:%d next level:%d", curLevel, nextLevel)

	if !owner.ConsumeByConf(nextLevelConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUpLvMeridian}) {
		owner.LogWarn("consume not enough")
		return
	}

	owner.LogInfo("try to rate success,nextLevelConf.Rate:%d", nextLevelConf.Rate)
	privilegeRateAdd, _ := s.owner.GetPrivilege(privilegedef.EnumMeridianBreakRateAdd)
	if !random.Hit(nextLevelConf.Rate+uint32(privilegeRateAdd), 100) {
		owner.LogWarn("up lv failed")
		if nextLevelConf.FailedBroadcastId > 0 {
			owner.SendTipMsg(nextLevelConf.FailedBroadcastId)
			return
		}
	}

	s.LogPlayerBehavior(uint64(req.Kind), map[string]interface{}{
		"formLv": curLevel,
		"nextLv": nextLevel,
	}, pb3.LogId_LogMeridianUpLevelExp)

	owner.LogInfo("set kind next lv:%d", nextLevel)
	state.KindLevelMap[req.Kind] = nextLevel

	owner.TriggerQuestEvent(custom_id.QttUpMeridianLv, req.Kind, int64(nextLevel))
	owner.TriggerQuestEvent(custom_id.QttUpMeridianStage, req.Kind, int64(nextLevelConf.Layer))
	var totalLv uint32
	for _, kindLv := range state.KindLevelMap {
		totalLv += kindLv
	}
	owner.TriggerQuestEvent(custom_id.QttUpMeridianTotalLv, 0, int64(totalLv))

	owner.LogInfo("reset meridian attr")
	s.ResetSysAttr(attrdef.SaMeridian)

	owner.SendProto3(129, 2, &pb3.S2C_129_2{
		Kind: req.Kind,
		Lv:   nextLevel,
	})
	owner.SendProto3(129, 1, &pb3.S2C_129_1{
		State: s.State(),
	})
	owner.UpdateStatics(model.FieldMeridiansTotalLv_, totalLv)
}

func calcMeridianSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiMeridian).(*MeridianSys)
	if !sys.IsOpen() {
		return
	}

	state := sys.State()

	for kind, level := range state.KindLevelMap {
		kindConf := jsondata.GetMeridianKindConf(kind)
		if kindConf == nil {
			continue
		}

		if int(level) >= len(kindConf.KindCfg) {
			return
		}

		engine.AddAttrsToCalc(player, calc, kindConf.KindCfg[level].Attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiMeridian, func() iface.ISystem {
		return &MeridianSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaMeridian, calcMeridianSysAttr)

	net.RegisterSysProto(129, 1, sysdef.SiMeridian, (*MeridianSys).c2sUpLevel)
}
