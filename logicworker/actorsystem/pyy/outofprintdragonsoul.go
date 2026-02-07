/**
 * @Author: beiming
 * @Desc: 限时转盘
 * @Date: 2024/03/15
 */

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type OutOfPrintDragonSoulSys struct {
	PlayerYYBase
}

func (s *OutOfPrintDragonSoulSys) OnOpen() {
	data := s.getData()
	data.Round = 1
	s.s2cInfo()
}

func (s *OutOfPrintDragonSoulSys) OnEnd() {
	s.reissueRewards()
}

// 补发奖励
func (s *OutOfPrintDragonSoulSys) reissueRewards() {
	conf := jsondata.GetPYYOutOfPrintDragonSoulConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.getData()
	if data.TotalTimes <= data.UseTimes {
		return
	}
	remainTimes := data.TotalTimes - data.UseTimes
	mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
		ConfId:  conf.MailId,
		Rewards: jsondata.StdRewardMulti(conf.RemainRewards, int64(remainTimes)),
		Content: &mailargs.CommonMailArgs{Digit1: int64(remainTimes)},
	})
}

func (s *OutOfPrintDragonSoulSys) Login() {
	s.s2cInfo()
}

func (s *OutOfPrintDragonSoulSys) OnReconnect() {
	s.s2cInfo()
}

func (s *OutOfPrintDragonSoulSys) getData() *pb3.PYY_OutOfPrintDragonSoul {
	data := s.GetYYData()

	if data.OutOfPrintDragonSoul == nil {
		data.OutOfPrintDragonSoul = make(map[uint32]*pb3.PYY_OutOfPrintDragonSoul)
	}

	if data.OutOfPrintDragonSoul[s.GetId()] == nil {
		data.OutOfPrintDragonSoul[s.GetId()] = &pb3.PYY_OutOfPrintDragonSoul{}
	}

	return data.OutOfPrintDragonSoul[s.GetId()]
}

func (s *OutOfPrintDragonSoulSys) ResetData() {
	data := s.GetYYData()

	if data.OutOfPrintDragonSoul == nil {
		return
	}
	delete(data.OutOfPrintDragonSoul, s.GetId())
}

func (s *OutOfPrintDragonSoulSys) s2cInfo() {
	s.SendProto3(127, 148, &pb3.S2C_127_148{
		Id:   s.GetId(),
		Data: s.getData(),
	})
}

func (s *OutOfPrintDragonSoulSys) c2sTurn(msg *base.Message) error {
	var req pb3.C2S_127_149
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getData()
	if data.TotalTimes <= data.UseTimes {
		return neterror.ParamsInvalidError("times not enough")
	}

	conf := jsondata.GetPYYOutOfPrintDragonSoulConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("OutOfPrintDragonSoul conf is nil")
	}

	roundConf := conf.GetYYTurntableDraw(data.Round)
	if nil == roundConf {
		return neterror.ParamsInvalidError("round conf not found")
	}

	p := new(random.Pool)
	for k, item := range roundConf.Pool {
		idx := uint32(k + 1) // idx 从1开始
		if pie.Uint32s(data.RecvIdx).Contains(idx) {
			continue
		}
		p.AddItem(idx, item.Weight)
	}

	if p.Size() == 0 {
		return neterror.ParamsInvalidError("已经抽完了")
	}

	idx := p.RandomOne().(uint32)
	if idx == 0 {
		return neterror.ParamsInvalidError("抽取失败")
	}

	item := roundConf.Pool[idx-1]
	data.UseTimes++
	data.RecvIdx = append(data.RecvIdx, idx)
	data.Idx = idx

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogOutOfPrintDragonSouldConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("1"),
	})

	engine.GiveRewards(s.GetPlayer(), item.Rewards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogOutOfPrintDragonSouldAwards,
		NoTips: true,
	})

	if item.Broadcast {
		timer.SetTimeout(time.Duration(conf.TurnTime)*time.Second, func() {
			engine.BroadcastTipMsgById(conf.TipsId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), item.Rewards))
		})
	}

	s.SendProto3(127, 149, &pb3.S2C_127_149{
		Id:       s.Id,
		UseTimes: data.UseTimes,
		Idx:      idx,
		Round:    data.Round,
		RecvIdx:  data.RecvIdx,
	})

	nextRound := p.Size() == 1
	if nextRound {
		nextRoundConf := conf.GetYYTurntableDraw(data.GetRound() + 1)
		if nextRoundConf == nil && conf.IsLoop {
			nextRoundConf = roundConf
		}

		if nextRoundConf != nil {
			data.Round = nextRoundConf.Round
			data.RecvIdx = nil
			data.Idx = 0
			s.s2cInfo()
		}
	}

	return nil
}

func (s *OutOfPrintDragonSoulSys) complete(drawEvent *custom_id.ActDrawEvent) {
	conf := jsondata.GetPYYOutOfPrintDragonSoulConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	if conf.ActType != drawEvent.ActType {
		return
	}
	if conf.ActId > 0 && conf.ActId != drawEvent.ActId {
		return
	}
	data := s.getData()
	data.CompleteTimes += drawEvent.Times

	addTimes := data.CompleteTimes / conf.CompleteTimes
	if addTimes > 0 {
		data.CompleteTimes -= addTimes * conf.CompleteTimes
		data.TotalTimes += addTimes * conf.AddTurnTimes
	}

	s.SendProto3(127, 150, &pb3.S2C_127_150{
		Id:            s.Id,
		TotalTimes:    data.TotalTimes,
		CompleteTimes: data.CompleteTimes,
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYOutOfPrintDragonSoul, func() iface.IPlayerYY {
		return &OutOfPrintDragonSoulSys{}
	})

	net.RegisterYYSysProtoV2(127, 149, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*OutOfPrintDragonSoulSys).c2sTurn
	})

	event.RegActorEvent(custom_id.AeActDrawTimes, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}

		drawEvent, ok := args[0].(*custom_id.ActDrawEvent)
		if !ok {
			return
		}
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYOutOfPrintDragonSoul)
		for _, obj := range yyList {
			if s, ok := obj.(*OutOfPrintDragonSoulSys); ok && s.IsOpen() {
				s.complete(drawEvent)
			}
		}
	})

	gmevent.Register("oopds.addtimes", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		times := utils.AtoUint32(args[0])
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYOutOfPrintDragonSoul)
		for _, obj := range yyList {
			if s, ok := obj.(*OutOfPrintDragonSoulSys); ok && s.IsOpen() {
				s.getData().TotalTimes += times
				s.s2cInfo()
				return true
			}
		}
		return false
	}, 1)
}
