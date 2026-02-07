/**
 * @Author: beiming
 * @Desc: 仙宗-宗门大殿
 * @Date: 2023/11/23
 */
package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

func init() {
	RegisterSysClass(sysdef.SiSectHall, newSectHallSystem)

	engine.RegAttrCalcFn(attrdef.SaSectHall, calcSectHallSysAttr)
	event.RegActorEvent(custom_id.AeNewDay, sectHallOnNewDay)
	event.RegActorEvent(custom_id.AeNewHour, sectHallOnNewHour)

	net.RegisterSysProtoV2(167, 2, sysdef.SiSectHall, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHall).c2sDonate
	})
	net.RegisterSysProtoV2(167, 0, sysdef.SiSectHall, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHall).c2sGlobalData
	})

	gmevent.Register("sectHall.exp", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		exp := utils.AtoUint32(args[0])
		sys := player.GetSysObj(sysdef.SiSectHall)
		if sys == nil || !sys.IsOpen() {
			return false
		}
		s := sys.(*SectHall)
		globalData := s.getGlobalData()
		globalData.Exp += exp
		if s.canLevelUp() {
			s.levelUp()
		} else {
			s.s2cGlobalInfo()
		}
		return true
	}, 1)
}

type SectHall struct {
	Base
}

// c2sGlobalData 客户端主动获取全服数据
func (s *SectHall) c2sGlobalData(_ *base.Message) error {
	s.s2cGlobalInfo()

	return nil
}

// c2sDonate 宗门捐献
func (s *SectHall) c2sDonate(msg *base.Message) error {
	var req pb3.C2S_167_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("sectHall c2sDonate unpack msg, err: %w", err)
	}

	// 今日是否还有捐献次数
	err := s.checkDonateTimes(req.GetDonateId())
	if err != nil {
		return err
	}

	// 消耗
	donateCfg, ok := jsondata.GetSectHallDonateConf(req.GetDonateId())
	if !ok {
		return neterror.ConfNotFoundError("sectHall c2sDonate donateId[%d] not exist", req.GetDonateId())
	}

	success := s.GetOwner().ConsumeByConf(donateCfg.Consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogSectHallDonateConsume})
	if !success {
		return neterror.ConsumeFailedError("sectHall c2sDonate consume failed")
	}

	// 更新个人数据
	data := s.getPersonalData()
	if len(donateCfg.Rewards) > 0 { // 获得奖励
		if !engine.GiveRewards(s.GetOwner(), donateCfg.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectHallDonateReward}) {
			return neterror.InternalError("sectHall c2sDonate add reward failed")
		}
	}

	data.DailyDonate[req.GetDonateId()]++
	s.s2cInfo()

	// 全服数据
	// 未到达开服天数时，增加经验，达到开服天数后，增加等级
	// 经验值一直累加不清零
	// 升级后需要通知全服玩家
	// 满级就不需要加经验 但是还要正常发捐献奖励
	globalData := s.getGlobalData()
	_, ok = jsondata.GetSectHallConf(globalData.Level + 1) // 下一级配置
	if ok {
		globalData.Exp += donateCfg.Exp
		if s.canLevelUp() {
			s.levelUp()
		} else {
			s.s2cGlobalInfo()
		}
	}

	logParams := map[string]any{
		"donateId":     req.GetDonateId(),
		"global_exp":   globalData.Exp,
		"global_level": globalData.Level,
		"add_exp":      donateCfg.Exp,
	}
	bt, _ := json.Marshal(logParams)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogSectHallDonate, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.DonateId),
		StrArgs: string(bt),
	})

	return nil
}

func (s *SectHall) canLevelUp() bool {
	globalData := s.getGlobalData()
	cfg, ok := jsondata.GetSectHallConf(globalData.Level + 1) // 下一级配置
	if !ok {
		return false
	}
	return gshare.GetOpenServerDay() >= cfg.Day && globalData.Exp >= cfg.Exp
}

func (s *SectHall) checkDonateTimes(donateId uint32) error {
	donateCfg, ok := jsondata.GetSectHallDonateConf(donateId)
	if !ok {
		return neterror.ConfNotFoundError("sectHall c2sDonate donateId[%d] not exist", donateId)
	}

	data := s.getPersonalData()
	times, ok := data.DailyDonate[donateId]
	if ok && times >= donateCfg.DailyTimes {
		return neterror.ParamsInvalidError("sectHall c2sDonate donateId[%d] times[%d] over limit", donateId, times)
	}

	return nil
}

func (s *SectHall) s2cInfo() {
	s.SendProto3(167, 1, &pb3.S2C_167_1{
		State: s.getPersonalData(),
	})
}

func (s *SectHall) s2cGlobalInfo() {
	s.SendProto3(167, 0, &pb3.S2C_167_0{
		State: s.getGlobalData(),
	})
}

func newSectHallSystem() iface.ISystem {
	return &SectHall{}
}

func (s *SectHall) getPersonalData() *pb3.SectHall {
	if s.GetBinaryData().SectHall == nil {
		s.GetBinaryData().SectHall = new(pb3.SectHall)
	}

	if s.GetBinaryData().SectHall.DailyDonate == nil {
		s.GetBinaryData().SectHall.DailyDonate = make(map[uint32]uint32)
	}

	return s.GetBinaryData().SectHall
}

func (s *SectHall) getGlobalData() *pb3.GlobalSectHall {
	if gshare.GetStaticVar().SectHall == nil {
		gshare.GetStaticVar().SectHall = &pb3.GlobalSectHall{
			Level: 1, // 从1级开始
		}
	}

	return gshare.GetStaticVar().SectHall
}

func (s *SectHall) OnOpen() {
	// 初始化数据
	s.getPersonalData()
	s.getGlobalData()

	// 重置属性
	s.ResetSysAttr(attrdef.SaSectHall)

	// 发送个人数据
	s.s2cInfo()
	s.s2cGlobalInfo()
}

func (s *SectHall) OnLogin() {
	s.s2cInfo()
	s.s2cGlobalInfo()
}

func (s *SectHall) OnReconnect() {
	s.ResetSysAttr(attrdef.SaSectHall)
	s.s2cInfo()
	s.s2cGlobalInfo()
}

func (s *SectHall) levelUp() {
	globalData := s.getGlobalData()

	var level uint32
	for _, cfg := range jsondata.GetSectHallConfigMap() {
		if gshare.GetOpenServerDay() >= cfg.Day && globalData.Exp >= cfg.Exp && cfg.Id > level {
			level = cfg.Id
		}
	}

	globalData.Level = level
	// 更新所有在线玩家的属性
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		if st := player.GetAttrSys(); nil != st {
			st.ResetSysAttr(attrdef.SaSectHall)
		}
	})

	globalData.Power = s.allPlayerPower()

	engine.Broadcast(chatdef.CIWorld, 0, 167, 0, &pb3.S2C_167_0{State: s.getGlobalData()}, 0)
}

// allPlayerPower 计算全服玩家战力
// 仅计算战力排行榜战力
func (s *SectHall) allPlayerPower() uint64 {
	var power uint64

	manager.GRankMgrIns.GetRankByType(gshare.RankTypePower).ChunkAll(func(item *pb3.OneRankItem) bool {
		power += uint64(item.Score)
		return false
	})

	return power
}

// sectHallOnNewDay 宗门大殿跨天
// 1.重置贡献记录
// 2. 到达开服天数时并且经验值足够升级时，升级
func sectHallOnNewDay(actor iface.IPlayer, args ...interface{}) {
	if s, ok := actor.GetSysObj(sysdef.SiSectHall).(*SectHall); ok && s.IsOpen() {
		data := s.getPersonalData()
		data.DailyDonate = make(map[uint32]uint32)
		s.s2cInfo()

		if s.canLevelUp() {
			s.levelUp()
		}
	}
}

// sectHallOnNewHour 宗门大殿每小时更新宗门大殿战力
func sectHallOnNewHour(actor iface.IPlayer, args ...interface{}) {
	if s, ok := actor.GetSysObj(sysdef.SiSectHall).(*SectHall); ok && s.IsOpen() {
		globalData := s.getGlobalData()
		globalData.Power = s.allPlayerPower()
	}
}

// calcSectHallSysAttr 计算宗门大殿系统属性
func calcSectHallSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	globalData := player.GetSysObj(sysdef.SiSectHall).(*SectHall).getGlobalData()

	cfg, ok := jsondata.GetSectHallConf(globalData.Level)
	if !ok {
		return
	}

	engine.CheckAddAttrsToCalc(player, calc, cfg.Attrs)
}
