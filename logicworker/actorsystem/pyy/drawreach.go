/**
 * @Author: LvYuMeng
 * @Date: 2024/4/3
 * @Desc:
**/

package pyy

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type DrawReachSys struct {
	PlayerYYBase
}

func (s *DrawReachSys) ResetData() {
	state := s.GetYYData()
	if nil == state.DrawReach {
		return
	}
	delete(state.DrawReach, s.Id)
}

func (s *DrawReachSys) GetData() *pb3.PYY_DrawReach {
	state := s.GetYYData()
	if nil == state.DrawReach {
		state.DrawReach = make(map[uint32]*pb3.PYY_DrawReach)
	}
	if state.DrawReach[s.Id] == nil {
		state.DrawReach[s.Id] = &pb3.PYY_DrawReach{}
	}
	if state.DrawReach[s.Id].Rev == nil {
		state.DrawReach[s.Id].Rev = make(map[uint32]bool)
	}
	return state.DrawReach[s.Id]
}

func (s *DrawReachSys) Login() {
	s.s2cInfo()
}

func (s *DrawReachSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DrawReachSys) OnOpen() {
	s.s2cInfo()
}

func (s *DrawReachSys) OnEnd() {
	conf := jsondata.GetYYDrawReachConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("draw reach conf is nil")
		return
	}

	data := s.GetData()
	var rewards jsondata.StdRewardVec

	for _, v := range conf.Reach {
		if data.Rev[v.Id] || data.Score < v.Score {
			continue
		}
		data.Rev[v.Id] = true
		rewards = jsondata.MergeStdReward(rewards, v.Rewards)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_DrawReach,
			Rewards: rewards,
		})
	}
}

func (s *DrawReachSys) s2cInfo() {
	s.SendProto3(59, 0, &pb3.S2C_59_0{ActiveId: s.GetId(), Info: s.GetData()})
}

func (s *DrawReachSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_59_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	id := req.Id
	data := s.GetData()

	if data.Rev[id] {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetYYDrawReachConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("draw reach conf is nil")
	}

	targetConf := conf.GetYYDrawTargetConf(id)
	if nil == targetConf {
		return neterror.ConfNotFoundError("draw reach target(%d) is nil", id)
	}

	if targetConf.Score > data.Score {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	data.Rev[id] = true
	engine.GiveRewards(s.GetPlayer(), targetConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDrawReachScore,
	})

	s.SendProto3(59, 1, &pb3.S2C_59_1{
		ActiveId: s.GetId(),
		Id:       id,
	})
	return nil
}

func (s *DrawReachSys) addScore(scoreType, score uint32) {
	conf := jsondata.GetYYDrawReachConf(s.ConfName, s.ConfIdx)
	if conf.ScoreType != scoreType {
		return
	}
	data := s.GetData()
	data.Score += score
	s.SendProto3(59, 2, &pb3.S2C_59_2{
		ActiveId: s.GetId(),
		Score:    data.GetScore(),
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogDrawReachScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.GetScore()),
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDrawReach, func() iface.IPlayerYY {
		return &DrawReachSys{}
	})

	net.RegisterYYSysProtoV2(59, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DrawReachSys).c2sRev
	})

	event.RegActorEvent(custom_id.AeGetDrawReachScore, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 2 {
			player.LogError("draw reach score add args not enough")
			return
		}
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYDrawReach)
		if nil == yyList {
			return
		}
		scType, sc := args[0].(uint32), args[1].(uint32)
		for _, v := range yyList {
			if s, ok := v.(*DrawReachSys); ok && s.IsOpen() {
				s.addScore(scType, sc)
			}
		}
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.DrawStandardsTips, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		return []interface{}{actorName, id}, true //玩家名，道具
	})
}
