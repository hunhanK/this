package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
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

// 灵气兑换系统

type LingQiExchangeSys struct {
	Base
}

func (s *LingQiExchangeSys) OnOpen() {
	s.s2cInfo()
}

func (s *LingQiExchangeSys) OnLogin() {
	s.s2cInfo()
}

func (s *LingQiExchangeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LingQiExchangeSys) OnNewDay() {
	data := s.getData()
	data.DailyTimes = 0
	data.DailyBuyTimes = 0
	s.s2cInfo()
}

func (s *LingQiExchangeSys) getData() *pb3.LingQiExchangeData {
	if s.GetBinaryData().LingQiExchangeData == nil {
		s.GetBinaryData().LingQiExchangeData = &pb3.LingQiExchangeData{}
	}
	return s.GetBinaryData().LingQiExchangeData
}

func (s *LingQiExchangeSys) s2cInfo() {
	s.SendProto3(2, 180, &pb3.S2C_2_180{
		Data: s.getData(),
	})
}

func (s *LingQiExchangeSys) c2sBuyTimes(_ *base.Message) error {
	data := s.getData()
	owner := s.GetOwner()
	commonConf := jsondata.GetLingQiExchangeCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("ling qi exchange common conf not found")
	}

	// 特权
	privilege, err := owner.GetPrivilege(privilegedef.EnumLingQiExchangeBuyTimes)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 今日购买次数
	if data.DailyBuyTimes >= uint32(privilege) {
		return neterror.ParamsInvalidError("daily buy times %d > privilege %d", data.DailyBuyTimes, privilege)
	}

	// 消耗
	times := data.DailyBuyTimes + 1
	confVec := commonConf.BuyTimesConsumeMgr[times]
	if len(confVec.Consume) > 0 && !owner.ConsumeByConf(confVec.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogLingQiExchangeBuyTimesConsume}) {
		return neterror.ConsumeFailedError("buy ling qi exchange buy time failed")
	}

	// 打点
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLingQiExchangeBuyTimes, &pb3.LogPlayerCounter{
		NumArgs: uint64(times),
	})
	data.DailyBuyTimes = times
	owner.SendProto3(2, 181, &pb3.S2C_2_181{DailyBuyTimes: times})

	return nil
}

func (s *LingQiExchangeSys) c2sExchange(msg *base.Message) error {
	owner := s.GetOwner()
	var req pb3.C2S_2_182
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	// 境界校验
	circle := owner.GetCircle()
	exchangeConf := jsondata.GetLingQiExchangeConf(circle)
	if exchangeConf == nil {
		return neterror.ConfNotFoundError("ling qi %d exchange conf not found", circle)
	}

	commonConf := jsondata.GetLingQiExchangeCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("ling qi exchange common conf not found")
	}

	// 等级校验
	levelSys := owner.GetSysObj(sysdef.SiLevel).(*LevelSys)
	if !levelSys.IsReachMaxLevel() {
		return neterror.InternalError("not reach max level")
	}

	exp := levelSys.GetExp()
	next := levelSys.GetLevel() + 1
	needExp, ok := jsondata.GetLevelConfig(owner.GetLevel() + 1)
	if !ok {
		return neterror.ConfNotFoundError("level %d conf not found", next)
	}
	if exp < needExp {
		return neterror.ConfNotFoundError("cur exp %d , next level exp %d, can't exchange ling qi", exp, needExp)
	}

	// 溢出经验校验
	overflowExp := exp - needExp
	var deductionExp uint32
	for _, consume := range exchangeConf.Consume {
		if consume.Id != moneydef.Exp {
			continue
		}
		if consume.Count < uint32(overflowExp) {
			overflowExp -= int64(consume.Count)
			deductionExp += consume.Count
			continue
		}
		return neterror.ParamsInvalidError("overflowExp %d exp %d needExp %d. not enough consume", overflowExp, exp, needExp)
	}

	// 次数校验
	data := s.getData()
	nextTimes := data.DailyTimes + 1
	if nextTimes > data.DailyBuyTimes+commonConf.DailyTimes {
		return neterror.ParamsInvalidError("daily times %d + 1 > daily buy times %d + conf daily times %d", data.DailyTimes, data.DailyBuyTimes, commonConf.DailyTimes)
	}

	// 消耗
	if len(exchangeConf.Consume) == 0 {
		return neterror.ConsumeFailedError("ling qi exchange consume failed")
	}

	levelSys.SetExp(exp - int64(deductionExp))
	engine.GiveRewards(owner, exchangeConf.Awards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogLingQiExchangeAwards,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogLingQiExchange, &pb3.LogPlayerCounter{
		NumArgs: uint64(nextTimes),
	})
	data.DailyTimes = nextTimes
	s.SendProto3(2, 182, &pb3.S2C_2_182{DailyTimes: nextTimes})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiLingQiExchange, func() iface.ISystem {
		return &LingQiExchangeSys{}
	})
	net.RegisterSysProtoV2(2, 181, sysdef.SiLingQiExchange, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LingQiExchangeSys).c2sBuyTimes
	})
	net.RegisterSysProtoV2(2, 182, sysdef.SiLingQiExchange, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LingQiExchangeSys).c2sExchange
	})
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		obj := player.GetSysObj(sysdef.SiLingQiExchange)
		if obj != nil && !obj.IsOpen() {
			return
		}
		sys := obj.(*LingQiExchangeSys)
		if sys == nil {
			return
		}
		sys.OnNewDay()
	})
	gmevent.Register("LingQiExchangeSys.OnNewDay", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiLingQiExchange).(*LingQiExchangeSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		sys.OnNewDay()
		return true
	}, 1)
}
