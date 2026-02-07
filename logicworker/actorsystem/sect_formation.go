/**
 * @Author: beiming
 * @Desc: 仙宗-宗门大阵
 * @Date: 2023/12/4
 */
package actorsystem

import (
	"encoding/json"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

func init() {
	RegisterSysClass(sysdef.SiSectFormation, newSectFormationSystem)

	engine.RegAttrCalcFn(attrdef.SaSectFormation, calcSectFormationSysAttr)

	net.RegisterSysProtoV2(167, 4, sysdef.SiSectFormation, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectFormation).c2sEyeLevelUp
	})
	net.RegisterSysProtoV2(167, 5, sysdef.SiSectFormation, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectFormation).c2sHeartLevelUp
	})
}

type SectFormation struct {
	Base
}

func (s *SectFormation) c2sEyeLevelUp(msg *base.Message) error {
	var req pb3.C2S_167_4
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("sectFormation c2sEyeLevelUp unpack msg, err: %w", err)
	}

	data := s.getData()

	var slot *pb3.SectFormationSlot
	for _, v := range data.GetSlots() {
		if v.Idx == req.GetIdx() {
			slot = v
			break
		}
	}
	if slot == nil {
		return neterror.ParamsInvalidError("sectFormation c2sEyeLevelUp invalid idx: %d", req.GetIdx())
	}

	// 获取下一级的配置
	var cfg *jsondata.SectFormationEye
	for _, v := range jsondata.GetSectFormationEyeConfMap() {
		if v.Level == slot.Level+1 && v.Slot == req.GetIdx() {
			cfg = v
			break
		}
	}
	if cfg == nil {
		return neterror.ConfNotFoundError("sectFormation c2sEyeLevelUp cfg is nil, idx: %d, level: %d", req.GetIdx(), slot.Level+1)
	}

	// 检查阵心等级
	if data.Level < cfg.RequireHeartLevel {
		return neterror.ParamsInvalidError("sectFormation c2sEyeLevelUp level invalid, idx: %d, level: %d", req.GetIdx(), data.Level)
	}

	// 消耗
	if ok := s.GetOwner().ConsumeByConf(cfg.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogSectFormationEyeLevelUpConsume}); !ok {
		s.GetOwner().LogWarn("consume sectFormation levelUp eye failed, idx: %d, level: %d", req.GetIdx(), slot.Level+1)
		return nil
	}

	slot.Level++

	// 更新属性
	s.ResetSysAttr(attrdef.SaSectFormation)

	logParams := map[string]any{
		"idx":   req.GetIdx(),
		"level": slot.Level,
	}
	bt, _ := json.Marshal(logParams)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogSectFormationEyeLevelUp, &pb3.LogPlayerCounter{
		NumArgs: uint64(slot.Level),
		StrArgs: string(bt),
	})

	s.GetOwner().SendProto3(167, 4, &pb3.S2C_167_4{
		Slot: slot,
	})

	return nil
}

func (s *SectFormation) c2sHeartLevelUp(_ *base.Message) error {
	data := s.getData()

	// 获取下一级的配置
	cfg, ok := jsondata.GetSectFormationHeartConf(data.Level + 1)
	if !ok {
		return neterror.ConfNotFoundError("sectFormation c2sHeartLevelUp cfg is nil, level: %d", data.Level+1)
	}

	// 消耗
	if ok := s.GetOwner().ConsumeByConf(cfg.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogSectFormationHeartLevelUpConsume}); !ok {
		s.GetOwner().LogWarn("consume sectFormation levelUp heart failed, level: %d", data.Level+1)
		return nil
	}

	data.Level++

	// 更新属性
	s.ResetSysAttr(attrdef.SaSectFormation)

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogSectFormationHeartLevelUp, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.Level),
	})

	s.GetOwner().SendProto3(167, 5, &pb3.S2C_167_5{
		Level: data.Level,
	})

	return nil
}

func (s *SectFormation) s2cInfo() {
	s.SendProto3(167, 3, &pb3.S2C_167_3{
		State: s.getData(),
	})
}

func newSectFormationSystem() iface.ISystem {
	return &SectFormation{}
}

func (s *SectFormation) getData() *pb3.SectFormation {
	if s.GetBinaryData().SectFormation == nil {
		s.GetBinaryData().SectFormation = &pb3.SectFormation{
			Level: 0,
		}
	}

	// 取配表中最大的阵眼数量
	var sectFormationEyes int
	for _, v := range jsondata.GetSectFormationEyeConfMap() {
		if sectFormationEyes < int(v.Slot) {
			sectFormationEyes = int(v.Slot)
		}
	}

	// 阵眼数量不够, 说明已经有新的阵眼配置了
	// 给新的阵眼配置默认等级
	l := len(s.GetBinaryData().SectFormation.Slots)
	if l < sectFormationEyes {
		for i := 0; i < sectFormationEyes-l; i++ {
			s.GetBinaryData().SectFormation.Slots = append(s.GetBinaryData().SectFormation.Slots, &pb3.SectFormationSlot{Idx: uint32(l + i + 1), Level: 0})
		}
	}

	return s.GetBinaryData().SectFormation
}

func (s *SectFormation) OnOpen() {
	// 初始化数据
	s.getData()

	// 活动开启, 就会增加属性
	s.ResetSysAttr(attrdef.SaSectFormation)

	// 发送个人数据
	s.s2cInfo()
}

func (s *SectFormation) OnLogin() {
	s.s2cInfo()
}

func (s *SectFormation) OnReconnect() {
	s.ResetSysAttr(attrdef.SaSectFormation)
	s.s2cInfo()
}

// calcSectFormationSysAttr 计算宗门大阵系统属性
func calcSectFormationSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	data := player.GetSysObj(sysdef.SiSectFormation).(*SectFormation).getData()

	var attrs []*jsondata.Attr
	// 阵眼属性
	for _, s := range data.GetSlots() {
		for _, c := range jsondata.GetSectFormationEyeConfMap() {
			if s.Level == c.Level && s.Idx == c.Slot {
				attrs = append(attrs, c.Attrs...)
				break
			}
		}
	}

	// 阵心属性
	cfg, ok := jsondata.GetSectFormationHeartConf(data.Level)
	if ok {
		attrs = append(attrs, cfg.Attrs...)
	}

	// 共鸣属性
	level := resonanceLevel(data)
	var resonanceCfg *jsondata.SectFormationResonance
	for _, c := range jsondata.GetSectFormationResonanceConfMap() {
		// 取最大的共鸣等级
		if level >= c.NeedEyeLevel &&
			(resonanceCfg == nil || c.Level > resonanceCfg.Level) {
			resonanceCfg = c
		}
	}
	if resonanceCfg != nil {
		attrs = append(attrs, resonanceCfg.Attrs...)
	}

	engine.CheckAddAttrsToCalc(player, calc, attrs)
}

func resonanceLevel(data *pb3.SectFormation) uint32 {
	var level uint32
	// 取最低等级的阵眼
	for _, v := range data.Slots {
		if level == 0 || v.Level < level {
			level = v.Level
		}
	}

	return level
}
