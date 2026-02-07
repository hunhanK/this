/**
 * @Author: LvYuMeng
 * @Date: 2025/6/30
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type GMBenefitSys struct {
	*QuestTargetBase
}

var (
	gmBenefitQuestTargetMap = map[uint32]map[uint32]struct{}{}
)

func newGMBenefitSys() iface.ISystem {
	sys := &GMBenefitSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (s *GMBenefitSys) OnAfterLogin() {
	s.checkResetQuest()
	s.s2cInfo()
}

func (s *GMBenefitSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GMBenefitSys) s2cInfo() {
	s.SendProto3(41, 25, &pb3.S2C_41_25{Data: s.getData()})
}

func (s *GMBenefitSys) checkResetQuest() {
	conf := jsondata.GetGMBenefitConfig()
	if nil == conf {
		return
	}

	data := s.getData()
	srvDay := gshare.GetOpenServerDay()
	for _, v := range conf.Gift {
		if v.MinSrvDay > 0 && srvDay < v.MinSrvDay {
			continue
		}
		if v.MaxSrvDay > 0 && srvDay > v.MaxSrvDay {
			continue
		}

		quest, ok := data.Quests[v.Id]
		if !ok {
			quest = &pb3.GMBenefitQuest{
				Id: v.Id,
			}
			data.Quests[v.Id] = quest
		}

		if len(v.Targets) > 0 && nil == quest.Quest {
			quest.Quest = &pb3.QuestData{Id: v.Id}
			s.OnAcceptQuestAndCheckUpdateTarget(quest.Quest)
		}
	}
}

func (s *GMBenefitSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := gmBenefitQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

const (
	GMBenefitNormal = 1
	GMBenefitCharge = 2
)

func (s *GMBenefitSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	_, err := jsondata.GetGMBenefitQuestConf(id)
	if nil != err {
		return nil
	}
	data := s.getData()
	state, ok := data.Quests[id]
	if !ok {
		return nil
	}
	if state.RevFlag > 0 {
		return nil
	}

	return state.Quest
}

func (s *GMBenefitSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf, err := jsondata.GetGMBenefitQuestConf(id)
	if nil != err {
		return nil
	}
	return conf.Targets
}

func (s *GMBenefitSys) onUpdateTargetData(questId uint32) {
	quest := s.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	data := s.getData()
	s.SendProto3(41, 27, &pb3.S2C_41_27{
		Quest: data.Quests[questId],
	})
}

func (s *GMBenefitSys) OnOpen() {
	data := s.getData()
	if chargeInfo := s.owner.GetBinaryData().GetChargeInfo(); nil != chargeInfo {
		data.ChargeCent = chargeInfo.DailyChargeMoney
	}

	s.checkResetQuest()
	s.s2cInfo()
}

func (s *GMBenefitSys) beforeNewDay() {
	s.calcRewards(0, true)
}

func (s *GMBenefitSys) onNewDay() {
	s.reset()
	s.checkResetQuest()
	s.s2cInfo()
}

func (s *GMBenefitSys) onCharge() {
	data := s.getData()
	if chargeInfo := s.owner.GetBinaryData().GetChargeInfo(); nil != chargeInfo {
		data.ChargeCent = chargeInfo.DailyChargeMoney
	}
	s.s2cInfo()
}

func (s *GMBenefitSys) calcRewards(id uint32, isMail bool) {
	conf := jsondata.GetGMBenefitConfig()
	if nil == conf {
		return
	}
	data := s.getData()
	onlineTime := s.owner.GetDayOnlineTime()

	var rewardsVec []jsondata.StdRewardVec
	for _, v := range data.Quests {
		if id > 0 && v.Id != id {
			continue
		}
		gConf, err := jsondata.GetGMBenefitQuestConf(v.Id)
		if nil != err {
			logger.LogError("err:%v", err)
			continue
		}

		if nil != v.Quest && !s.CheckFinishQuest(v.Quest) {
			continue
		}

		if gConf.OnlineTime > 0 && onlineTime < gConf.OnlineTime {
			continue
		}

		if !utils.IsSetBit(v.RevFlag, GMBenefitNormal) {
			rewardsVec = append(rewardsVec, gConf.NormalRewards)
			v.RevFlag = utils.SetBit(v.RevFlag, GMBenefitNormal)
		}
		if !utils.IsSetBit(v.RevFlag, GMBenefitCharge) && data.ChargeCent >= gConf.ChargeCent {
			rewardsVec = append(rewardsVec, gConf.ChargeAwards)
			v.RevFlag = utils.SetBit(v.RevFlag, GMBenefitCharge)
		}
	}

	rewards := jsondata.AppendStdReward(rewardsVec...)
	if len(rewards) == 0 {
		return
	}

	if isMail {
		mailmgr.SendMailToActor(s.owner.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_GMBenefitAwards,
			Rewards: rewards,
		})
	} else {
		engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGMBenefitAwards})
	}
}

func (s *GMBenefitSys) reset() {
	s.GetBinaryData().GMBenefit = nil
	s.s2cInfo()
}

func (s *GMBenefitSys) getData() *pb3.GMBenefit {
	binary := s.owner.GetBinaryData()
	if nil == binary.GMBenefit {
		binary.GMBenefit = &pb3.GMBenefit{}
	}
	if nil == binary.GMBenefit.Quests {
		binary.GMBenefit.Quests = map[uint32]*pb3.GMBenefitQuest{}
	}
	return binary.GMBenefit
}

func (s *GMBenefitSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_41_26
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	s.calcRewards(req.Id, false)
	s.s2cInfo()

	return nil
}

func onAfterReloadGMBenefitConf(args ...interface{}) {
	conf := jsondata.GetGMBenefitConfig()
	if nil == conf {
		return
	}
	tmp := make(map[uint32]map[uint32]struct{})
	for id, quest := range conf.Gift {
		for _, target := range quest.Targets {
			if _, ok := tmp[target.Type]; !ok {
				tmp[target.Type] = make(map[uint32]struct{})
			}
			tmp[target.Type][id] = struct{}{}
		}
	}
	gmBenefitQuestTargetMap = tmp
}

func init() {
	RegisterSysClass(sysdef.SiGMBenefit, func() iface.ISystem {
		return newGMBenefitSys()
	})

	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadGMBenefitConf)

	event.RegActorEvent(custom_id.AeBeforeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiGMBenefit).(*GMBenefitSys); ok && sys.IsOpen() {
			sys.beforeNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiGMBenefit).(*GMBenefitSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetSysObj(sysdef.SiGMBenefit).(*GMBenefitSys); ok && sys.IsOpen() {
			sys.onCharge()
		}
	})

	net.RegisterSysProto(41, 26, sysdef.SiGMBenefit, (*GMBenefitSys).c2sRev)
}
