/**
 * @Author: LvYuMeng
 * @Date: 2024/8/1
 * @Desc: 幸运金蛋
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"modernc.org/mathutil"
	"time"
)

type LuckyGoldEggSys struct {
	PlayerYYBase
	rewards map[uint32]struct{}
}

func (s *LuckyGoldEggSys) data() *pb3.PYY_LuckyGoldEgg {
	state := s.GetYYData()
	if state.LuckyGoldEgg == nil {
		state.LuckyGoldEgg = make(map[uint32]*pb3.PYY_LuckyGoldEgg)
	}
	if state.LuckyGoldEgg[s.Id] == nil {
		state.LuckyGoldEgg[s.Id] = &pb3.PYY_LuckyGoldEgg{}
	}
	return state.LuckyGoldEgg[s.Id]
}

func (s *LuckyGoldEggSys) Login() {
	s.s2cInfo()
}

func (s *LuckyGoldEggSys) OnReconnect() {
	s.s2cInfo()
}

func (s *LuckyGoldEggSys) s2cInfo() {
	s.SendProto3(69, 160, &pb3.S2C_69_160{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *LuckyGoldEggSys) OnOpen() {
	s.s2cInfo()
}

func (s *LuckyGoldEggSys) OnLogout() {
	if conf, ok := jsondata.GetYYLuckyGoldEggConf(s.ConfName, s.ConfIdx); ok {
		for awardId := range s.rewards {
			engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogLuckyGoldEggAward,
			})
		}
		s.rewards = make(map[uint32]struct{})
	}
}

func (s *LuckyGoldEggSys) ResetData() {
	state := s.GetYYData()
	if state.LuckyGoldEgg == nil {
		return
	}
	delete(state.LuckyGoldEgg, s.GetId())
}

func (s *LuckyGoldEggSys) updateScore(score uint32, isAdd bool) {
	data := s.data()
	if isAdd {
		data.Score += score
	} else {
		if data.Score < score {
			data.Score = 0
		} else {
			data.Score -= score
		}
	}
	s.SendProto3(69, 162, &pb3.S2C_69_162{
		ActiveId: s.GetId(),
		Score:    data.Score,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogLuckyGoldEggScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.Score),
	})
}

func (s *LuckyGoldEggSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_161
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYLuckyGoldEggConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("LuckyGoldEggSys conf is nil")
	}
	data := s.data()

	if pie.Uint32s(data.ClientParam).Contains(req.GetParam()) {
		return neterror.ConfNotFoundError("LuckyGoldEggSys client send repeated")
	}

	weightConf, ok := conf.GetTimeWeightConfByTimes(data.Times + 1)
	if !ok {
		return neterror.ConfNotFoundError("LuckyGoldEggSys weight conf is nil")
	}

	if weightConf.Price > data.Score {
		return neterror.ParamsInvalidError("score not enough")
	}

	data.Times++
	s.updateScore(weightConf.Price, false)

	var randomPool = new(random.Pool)
	for i := 0; i < len(weightConf.Rate); i += 2 {
		id := weightConf.Rate[i]
		if pie.Uint32s(data.RevIds).Contains(id) {
			continue
		}
		weight := weightConf.Rate[i+1]
		randomPool.AddItem(id, weight)
	}

	awardId := randomPool.RandomOne().(uint32)
	data.RevIds = append(data.RevIds, awardId)
	data.ClientParam = append(data.ClientParam, req.GetParam())

	var sendAwardsAfter = func(awardId uint32) {
		if _, noSend := s.rewards[awardId]; noSend {
			delete(s.rewards, awardId)
			engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogLuckyGoldEggAward,
			})
			if conf.TurntableAward[awardId].BroadcastId > 0 {
				engine.BroadcastTipMsgById(conf.TurntableAward[awardId].BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), conf.TurntableAward[awardId].Rewards))
			}
		}
	}

	if !req.IsSkip {
		s.rewards[awardId] = struct{}{}
		s.GetPlayer().SetTimeout(time.Second*time.Duration(int64(conf.Dur)), func() {
			sendAwardsAfter(awardId)
		})
	} else {
		engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogLuckyGoldEggAward,
		})
		if conf.TurntableAward[awardId].BroadcastId > 0 {
			engine.BroadcastTipMsgById(conf.TurntableAward[awardId].BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), conf.TurntableAward[awardId].Rewards))
		}
	}

	s.SendProto3(69, 161, &pb3.S2C_69_161{
		ActiveId: s.GetId(),
		Id:       awardId,
		Times:    data.Times,
		Param:    req.GetParam(),
	})
	return nil
}

func (s *LuckyGoldEggSys) checkAddScore(mt uint32, count int64) {
	conf, ok := jsondata.GetYYLuckyGoldEggConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if mt != conf.MoneyType && !(mt == moneydef.Diamonds && conf.MoneyType == moneydef.BindDiamonds) {
		return
	}

	data := s.data()
	data.ConsumeCount += uint32(count)

	gcd := mathutil.GCDUint32(conf.ScoreRate, 10000)
	unitX := 10000 / gcd
	unitY := conf.ScoreRate / gcd

	if data.ConsumeCount < unitX {
		return
	}

	transferCount := data.ConsumeCount / unitX
	data.ConsumeCount = data.ConsumeCount - transferCount*unitX
	transferScore := transferCount * unitY

	if transferScore == 0 {
		return
	}
	s.updateScore(transferScore, true)
	return
}

func forRangeLuckyGoldEggSys(player iface.IPlayer, f func(s *LuckyGoldEggSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYLuckyGoldEgg)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*LuckyGoldEggSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		f(sys)
	}

	return
}

func checkLuckyGoldEggScore(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	mt, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok {
		return
	}

	forRangeLuckyGoldEggSys(player, func(s *LuckyGoldEggSys) {
		s.checkAddScore(mt, count)
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYLuckyGoldEgg, func() iface.IPlayerYY {
		return &LuckyGoldEggSys{}
	})

	net.RegisterYYSysProtoV2(69, 161, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*LuckyGoldEggSys).c2sDraw
	})

	event.RegActorEvent(custom_id.AeConsumeMoney, checkLuckyGoldEggScore)
}
