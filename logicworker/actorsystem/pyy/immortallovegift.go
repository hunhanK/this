/**
 * @Author: LvYuMeng
 * @Date: 2024/4/27
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/net"
)

type ImmortalLoveGiftSys struct {
	PlayerYYBase
}

const (
	ImmortalLoveGift_Charge  = 1
	ImmortalLoveGift_Consume = 2
)

func (s *ImmortalLoveGiftSys) GetData() *pb3.PYY_ImmortalLoveGift {
	state := s.GetYYData()
	if nil == state.ImmortalLoveGift {
		state.ImmortalLoveGift = make(map[uint32]*pb3.PYY_ImmortalLoveGift)
	}
	if state.ImmortalLoveGift[s.Id] == nil {
		state.ImmortalLoveGift[s.Id] = &pb3.PYY_ImmortalLoveGift{}
	}
	if state.ImmortalLoveGift[s.Id].Progress == nil {
		state.ImmortalLoveGift[s.Id].Progress = make(map[uint32]uint32)
	}
	return state.ImmortalLoveGift[s.Id]
}

func (s *ImmortalLoveGiftSys) Login() {
	s.s2cInfo()
}

func (s *ImmortalLoveGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ImmortalLoveGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *ImmortalLoveGiftSys) OnEnd() {
	conf := jsondata.GetYYImmortalLoveGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("immortal love gift conf is nil")
		return
	}

	data := s.GetData()
	var rewards jsondata.StdRewardVec
	var revTimes uint32

	if conf.ChargeTarget > 0 {
		revTimes += data.Progress[ImmortalLoveGift_Charge] / conf.ChargeTarget
	}
	if conf.ConsumeTarget > 0 {
		revTimes += data.Progress[ImmortalLoveGift_Consume] / conf.ConsumeTarget
	}

	data.Progress = make(map[uint32]uint32)
	if revTimes <= 0 {
		return
	}

	rewards = jsondata.StdRewardMulti(conf.Rewards, int64(revTimes))

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_ImmortalLoveGiftAward,
			Rewards: rewards,
		})
	}
}

func (s *ImmortalLoveGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.ImmortalLoveGift {
		return
	}
	delete(state.ImmortalLoveGift, s.Id)
}

func (s *ImmortalLoveGiftSys) s2cInfo() {
	s.SendProto3(53, 70, &pb3.S2C_53_70{ActiveId: s.GetId(), Gift: s.GetData()})
}

func (s *ImmortalLoveGiftSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_53_71
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYImmortalLoveGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("immortal love gift conf is nil")
	}

	data := s.GetData()
	revType := req.Type

	var revTimes, target uint32

	switch revType {
	case ImmortalLoveGift_Charge:
		target = conf.ChargeTarget
	case ImmortalLoveGift_Consume:
		target = conf.ConsumeTarget
	default:
		return neterror.ConfNotFoundError("not immortal love gift valid type(%d)", revType)
	}

	if target > 0 {
		revTimes = data.Progress[revType] / target
	}

	if revTimes <= 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	data.Progress[revType] = data.Progress[revType] - revTimes*target

	rewards := jsondata.StdRewardMulti(conf.Rewards, int64(revTimes))
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogImmortalLoveGiftAward})
	}

	s.SendProto3(53, 71, &pb3.S2C_53_71{
		ActiveId: s.Id,
		Type:     revType,
		Awards:   jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	s.s2cInfo()
	return nil
}

func (s *ImmortalLoveGiftSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	chargeCent := chargeEvent.CashCent

	data := s.GetData()
	data.Progress[ImmortalLoveGift_Charge] += chargeCent

	s.s2cInfo()
}

func (s *ImmortalLoveGiftSys) MoneyConsume(mt uint32, count int64) {
	conf := jsondata.GetYYImmortalLoveGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	if !utils.SliceContainsUint32(conf.ConsumeType, mt) {
		return
	}

	data := s.GetData()
	data.Progress[ImmortalLoveGift_Consume] += uint32(count)

	s.s2cInfo()
}

func onImmortalLoveGiftMoneyChange(player iface.IPlayer, args ...interface{}) {
	mt, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok || count >= 0 {
		return
	}

	count = -count

	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYImmortalLoveGift)
	for _, obj := range yyList {
		s, sok := obj.(*ImmortalLoveGiftSys)
		if !sok || !s.IsOpen() {
			continue
		}
		s.MoneyConsume(mt, count)
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYImmortalLoveGift, func() iface.IPlayerYY {
		return &ImmortalLoveGiftSys{}
	})

	net.RegisterYYSysProtoV2(53, 71, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ImmortalLoveGiftSys).c2sRev
	})

	event.RegActorEvent(custom_id.AeMoneyChange, onImmortalLoveGiftMoneyChange)
}
