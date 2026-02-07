package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type GodWeaponComeSys struct {
	*YYQuestTargetBase
}

func createYYGodWeaponComeSys() iface.IPlayerYY {
	obj := &GodWeaponComeSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *GodWeaponComeSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	//下发新增任务
	s.SendProto3(132, 1, &pb3.S2C_132_1{Quest: quest, ActiveId: s.Id})
}

func (s *GodWeaponComeSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.GetData()
	for _, quest := range data.Quests {
		if quest.GetId() == id {
			return quest
		}
	}
	return nil
}

func (s *GodWeaponComeSys) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetYYGodWeaponComeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil
	}
	for _, v := range conf.Quests {
		if v.Id == questId {
			return v.Targets
		}
	}
	return nil
}

func (s *GodWeaponComeSys) getQuestIdSetFunc(qt uint32) map[uint32]struct{} {
	conf := jsondata.GetYYGodWeaponComeConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	set := make(map[uint32]struct{})
	for _, v := range conf.Quests {
		for _, target := range v.Targets {
			if target.Type == qt {
				set[v.Id] = struct{}{}
			}
		}
	}
	return set
}

func (s *GodWeaponComeSys) GetData() *pb3.PYY_GodWeaponCome {
	yyData := s.GetYYData()
	if nil == yyData.GodWeaponCome {
		yyData.GodWeaponCome = make(map[uint32]*pb3.PYY_GodWeaponCome)
	}
	if nil == yyData.GodWeaponCome[s.Id] {
		yyData.GodWeaponCome[s.Id] = &pb3.PYY_GodWeaponCome{}
	}
	return yyData.GodWeaponCome[s.Id]
}

func (s *GodWeaponComeSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.GodWeaponCome {
		return
	}
	delete(yyData.GodWeaponCome, s.Id)
}

func (s *GodWeaponComeSys) S2CInfo() {
	s.SendProto3(132, 0, &pb3.S2C_132_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *GodWeaponComeSys) OnOpen() {
	s.GetData()
	s.GetYYData().GodWeaponCome[s.Id] = &pb3.PYY_GodWeaponCome{} //清除上一轮的数据
	s.CheckResetQuest()
	s.S2CInfo()
}

func (s *GodWeaponComeSys) OnEnd() {
	data := s.GetData()
	if !data.IsRev && s.CheckFinishAllQuest() {
		conf := jsondata.GetYYGodWeaponComeConf(s.ConfName, s.ConfIdx)
		if nil == conf {
			s.LogError("no godweaponcome conf(%d)", s.ConfIdx)
			return
		}
		data.IsRev = true
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_GodWeaponComeEndAward,
			Rewards: conf.Award,
		})
	}
}

func (s *GodWeaponComeSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.CheckResetQuest()
	s.S2CInfo()
}

func (s *GodWeaponComeSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *GodWeaponComeSys) CheckResetQuest() {
	data := s.GetData()
	if len(data.Quests) > 0 {
		return
	}

	conf := jsondata.GetYYGodWeaponComeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data.Quests = make([]*pb3.QuestData, 0, len(conf.Quests))

	for _, quest := range conf.Quests {
		data.Quests = append(data.Quests, &pb3.QuestData{Id: quest.Id})
	}
	for _, quest := range data.Quests {
		s.YYQuestTargetBase.OnAcceptQuest(quest)
	}
}

func (s *GodWeaponComeSys) CheckFinishAllQuest() bool {
	data := s.GetData()

	if len(data.Quests) <= 0 {
		return false
	}

	for _, quest := range data.Quests {
		if !s.CheckFinishQuest(quest) {
			return false
		}
	}
	return true
}

func (s *GodWeaponComeSys) c2sAward(msg *base.Message) error {
	data := s.GetData()
	// 是否已完成所有任务
	if !s.CheckFinishAllQuest() {
		return nil
	}
	conf := jsondata.GetYYGodWeaponComeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("godweaponcome conf(%d) is nil", s.ConfIdx)
	}
	if data.IsRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	data.IsRev = true
	if len(conf.Award) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Award, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogYYGodWeaponComeAward,
		})
	}
	s.SendProto3(132, 3, &pb3.S2C_132_3{ActiveId: s.Id})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiGodWeaponCome, createYYGodWeaponComeSys)

	net.RegisterYYSysProto(132, 3, (*GodWeaponComeSys).c2sAward)
}
