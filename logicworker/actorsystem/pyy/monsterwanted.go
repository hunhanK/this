package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type MonsterWantedSys struct {
	PlayerYYBase
}

const (
	awardNormal = 0
	awardEx     = 1
)

func (s *MonsterWantedSys) GetData() *pb3.PYY_MonsterWanted {
	yyData := s.GetYYData()
	if nil == yyData.MonsterWanted {
		yyData.MonsterWanted = make(map[uint32]*pb3.PYY_MonsterWanted)
	}
	if nil == yyData.MonsterWanted[s.Id] {
		yyData.MonsterWanted[s.Id] = &pb3.PYY_MonsterWanted{}
	}
	if nil == yyData.MonsterWanted[s.Id].Quest {
		yyData.MonsterWanted[s.Id].Quest = make(map[uint32]uint32)
	}
	if nil == yyData.MonsterWanted[s.Id].Award {
		yyData.MonsterWanted[s.Id].Award = make(map[uint32]uint32)
	}
	return yyData.MonsterWanted[s.Id]
}
func (s *MonsterWantedSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.MonsterWanted {
		return
	}
	delete(yyData.MonsterWanted, s.Id)
}

func (s *MonsterWantedSys) S2CInfo() {
	s.SendProto3(134, 0, &pb3.S2C_134_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *MonsterWantedSys) OnOpen() {
	s.GetData()
	s.GetYYData().MonsterWanted[s.Id] = &pb3.PYY_MonsterWanted{
		Quest: make(map[uint32]uint32),
		Award: make(map[uint32]uint32),
	}
	s.S2CInfo()
}

func (s *MonsterWantedSys) OnEnd() {
	data := s.GetData()
	conf := jsondata.GetYYMonsterWantedConf(s.ConfName, s.ConfIdx)
	if nil == conf || nil == conf.Wanted {
		s.LogError("no monsterwanted conf(%d)", s.ConfIdx)
		return
	}
	var award []*jsondata.StdReward
	for _, v := range conf.Wanted.Quests {
		if data.Quest[v.Id] >= v.Count {
			if !utils.IsSetBit(data.Award[v.Id], awardNormal) {
				data.Award[v.Id] = utils.SetBit(data.Award[v.Id], awardNormal)
				award = jsondata.MergeStdReward(award, v.Awards)
			}
			if data.ActiveTime > 0 && !utils.IsSetBit(data.Award[v.Id], awardEx) {
				data.Award[v.Id] = utils.SetBit(data.Award[v.Id], awardEx)
				award = jsondata.MergeStdReward(award, v.BetterAwards)
			}
		}
	}

	if data.ActiveTime > 0 && !data.IsRevChargeAward {
		data.IsRevChargeAward = true
		if conf.Wanted.ChargeRewards != nil {
			award = jsondata.MergeStdReward(award, conf.Wanted.ChargeRewards)
		}
	}

	if len(award) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYMonsterWantedAward,
			Rewards: award,
		})
	}
}

func (s *MonsterWantedSys) Login() {
	s.S2CInfo()
}

func (s *MonsterWantedSys) OnReconnect() {
	s.S2CInfo()
}

func (s *MonsterWantedSys) addQuestProgress(monsterId, sceneId, count uint32) {
	data := s.GetData()
	conf := jsondata.GetYYMonsterWantedConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	monsterConf := jsondata.GetMonsterConf(monsterId)
	if nil == monsterConf {
		s.LogError("no monster conf(%d)", monsterId)
		return
	}
	if nil != conf.Wanted {
		for _, quest := range conf.Wanted.Quests {
			if quest.MonsterType == monsterConf.Type && quest.SceneId == sceneId {
				if data.Quest[quest.Id]+count > quest.Count {
					data.Quest[quest.Id] = quest.Count
				} else {
					data.Quest[quest.Id] += count
				}
				s.SendProto3(134, 1, &pb3.S2C_134_1{ActiveId: s.Id, QuestId: quest.Id, Count: data.Quest[quest.Id]})
			}
		}
	}
	return
}

func onMonsterWantedKillMon(player iface.IPlayer, args ...interface{}) {
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
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiMonsterWanted)
	if nil == yyList {
		return
	}
	for _, v := range yyList {
		sys, ok := v.(*MonsterWantedSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		sys.addQuestProgress(monsterId, sceneId, count)
	}
}

func (s *MonsterWantedSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_134_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	id := req.GetId()
	data := s.GetData()
	conf := jsondata.GetYYMonsterWantedConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("monster wanted conf(%d) is nil", s.ConfIdx)
	}
	var quest *jsondata.MonsterWantedQuestConf
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
	bits := data.Award[id]
	if !req.IsExAward {
		if utils.IsSetBit(bits, awardNormal) {
			s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		data.Award[id] = utils.SetBit(bits, awardNormal)
		engine.GiveRewards(s.GetPlayer(), quest.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMonsterWanted})
	} else {
		if utils.IsSetBit(bits, awardEx) {
			s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
			return nil
		}
		if data.ActiveTime <= 0 {
			s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
			return nil
		}
		data.Award[id] = utils.SetBit(bits, awardEx)
		engine.GiveRewards(s.GetPlayer(), quest.BetterAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMonsterWanted})
	}

	s.GetPlayer().TriggerQuestEvent(custom_id.QttAchievementsCompleteMonsterWanted, 0, int64(len(data.Award)))

	s.SendProto3(134, 2, &pb3.S2C_134_2{
		ActiveId:  s.Id,
		Id:        id,
		IsExAward: req.GetIsExAward(),
	})
	return nil
}

func (s *MonsterWantedSys) c2sChargeAward(msg *base.Message) error {
	var req pb3.C2S_134_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	if data.IsRevChargeAward {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetYYMonsterWantedConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("monster wanted conf(%d) is nil", s.ConfIdx)
	}

	if data.ActiveTime <= 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	data.IsRevChargeAward = true

	if len(conf.Wanted.ChargeRewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Wanted.ChargeRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMonsterWantedCharge})
	}

	s.SendProto3(134, 4, &pb3.S2C_134_4{
		ActiveId: s.Id,
		IsRev:    data.IsRevChargeAward,
	})
	return nil
}

func monsterWantedChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiMonsterWanted)
	if nil == yyList {
		return false
	}
	for _, v := range yyList {
		sys, ok := v.(*MonsterWantedSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		yyConf := jsondata.GetYYMonsterWantedConf(sys.ConfName, sys.ConfIdx)
		if nil == yyConf || nil == yyConf.Wanted {
			continue
		}
		if yyConf.Wanted.ChargeID == conf.ChargeId {
			if sys.GetData().ActiveTime > 0 { //已开通
				return false
			}
			return true
		}
	}
	return false
}

func monsterWantedChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSiMonsterWanted)
	if nil == yyList {
		return false
	}
	for _, v := range yyList {
		sys, ok := v.(*MonsterWantedSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		yyConf := jsondata.GetYYMonsterWantedConf(sys.ConfName, sys.ConfIdx)
		if nil == yyConf || nil == yyConf.Wanted {
			continue
		}
		if yyConf.Wanted.ChargeID == conf.ChargeId {
			sys.GetData().ActiveTime = time_util.NowSec()
			sys.SendProto3(134, 3, &pb3.S2C_134_3{
				ActiveId:   sys.Id,
				ActiveTime: sys.GetData().ActiveTime,
			})
			logworker.LogPlayerBehavior(sys.GetPlayer(), pb3.LogId_LogMonsterWantedCharge, &pb3.LogPlayerCounter{
				NumArgs: uint64(sys.GetId()),
			})
			engine.BroadcastTipMsgById(tipmsgid.Guaiwuxuanshang, player.GetName())
			return true
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiMonsterWanted, func() iface.IPlayerYY {
		return &MonsterWantedSys{}
	})
	event.RegActorEvent(custom_id.AeKillMon, onMonsterWantedKillMon)

	engine.RegChargeEvent(chargedef.SpMonsterWantedExAward, monsterWantedChargeCheck, monsterWantedChargeBack)

	net.RegisterYYSysProto(134, 2, (*MonsterWantedSys).c2sAward)
	net.RegisterYYSysProto(134, 4, (*MonsterWantedSys).c2sChargeAward)

}
