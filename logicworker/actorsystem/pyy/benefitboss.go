package pyy

import (
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

/**
 * @Author: lvyumeng
 * @Desc: 福利boss
 * @Date:
 */

type BenefitBossSys struct {
	PlayerYYBase
}

func (s *BenefitBossSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.BenefitBoss {
		return
	}
	delete(yyData.BenefitBoss, s.Id)
}

func (s *BenefitBossSys) GetData() *pb3.PYY_BenefitBoss {
	yyData := s.GetYYData()
	if nil == yyData.BenefitBoss {
		yyData.BenefitBoss = make(map[uint32]*pb3.PYY_BenefitBoss)
	}
	if nil == yyData.BenefitBoss[s.Id] {
		yyData.BenefitBoss[s.Id] = &pb3.PYY_BenefitBoss{}
	}
	if nil == yyData.BenefitBoss[s.Id].Quest {
		yyData.BenefitBoss[s.Id].Quest = make(map[uint32]uint32)
	}
	if nil == yyData.BenefitBoss[s.Id].Award {
		yyData.BenefitBoss[s.Id].Award = make(map[uint32]bool)
	}
	return yyData.BenefitBoss[s.Id]
}

func (s *BenefitBossSys) S2CInfo() {
	s.SendProto3(135, 0, &pb3.S2C_135_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *BenefitBossSys) OnOpen() {
	s.GetData()
	s.GetYYData().BenefitBoss[s.Id] = &pb3.PYY_BenefitBoss{
		Quest: make(map[uint32]uint32),
		Award: make(map[uint32]bool),
	}
	s.S2CInfo()
}

func (s *BenefitBossSys) OnEnd() {
	data := s.GetData()
	conf := jsondata.GetYYBenefitBossConf(s.ConfName, s.ConfIdx)
	if nil == conf || nil == conf.Wanted {
		s.LogError("%s no monster wanted conf", s.GetPrefix())
		return
	}
	var award []*jsondata.StdReward
	for _, v := range conf.Wanted.Quests {
		if data.Quest[v.Id] >= v.Count {
			if !data.Award[v.Id] {
				data.Award[v.Id] = true
				award = jsondata.MergeStdReward(award, v.Awards)
			}
		}
	}
	if len(award) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYBenefitBossAward,
			Rewards: award,
		})
	}
}

func (s *BenefitBossSys) Login() {
	s.S2CInfo()
}

func (s *BenefitBossSys) OnReconnect() {
	s.S2CInfo()
}

func (s *BenefitBossSys) addQuestProgress(monsterId, sceneId, count uint32) {
	data := s.GetData()
	conf := jsondata.GetYYBenefitBossConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	if nil != conf.Wanted {
		for _, quest := range conf.Wanted.Quests {
			if quest.MonsterId == monsterId {
				data.Quest[quest.Id] += count
				s.SendProto3(135, 1, &pb3.S2C_135_1{ActiveId: s.Id, QuestId: quest.Id, Count: data.Quest[quest.Id]})
			}
		}
	}
	return
}

func onBenefitBossKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}

	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}
	count, ok := args[2].(uint32)
	if !ok {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiBenefitBoss)
	if nil == yyList {
		return
	}
	for _, v := range yyList {
		sys, ok := v.(*BenefitBossSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		sys.addQuestProgress(monsterId, sceneId, count)
	}
}

func (s *BenefitBossSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_134_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	id := req.GetId()
	data := s.GetData()
	conf := jsondata.GetYYBenefitBossConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("benefitboss conf(%d) is nil", s.ConfIdx)
	}
	var quest *jsondata.BenefitBossQuestConf
	for _, v := range conf.Wanted.Quests {
		if v.Id == id {
			quest = v
			break
		}
	}
	if data.Quest[id] < quest.Count {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}
	if data.Award[id] {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	data.Award[id] = true

	engine.GiveRewards(s.GetPlayer(), quest.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitBoss})

	s.SendProto3(135, 2, &pb3.S2C_135_2{
		ActiveId: s.Id,
		Id:       id,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiBenefitBoss, func() iface.IPlayerYY {
		return &BenefitBossSys{}
	})
	event.RegActorEvent(custom_id.AeKillMon, onBenefitBossKillMon)

	net.RegisterYYSysProto(135, 2, (*BenefitBossSys).c2sAward)

}
