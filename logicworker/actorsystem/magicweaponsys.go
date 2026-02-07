/**
 * @Author: LvYuMeng
 * @Date: 2025/2/7
 * @Desc:仙器目标
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type MagicWeaponSys struct {
	*QuestTargetBase
}

func newMagicWeaponSys() iface.ISystem {
	sys := &MagicWeaponSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getTargetConfFunc,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func (s *MagicWeaponSys) getData() *pb3.MagicWeapon {
	binary := s.GetBinaryData()
	if nil == binary.MagicWeapon {
		binary.MagicWeapon = &pb3.MagicWeapon{}
	}
	if nil == binary.MagicWeapon.SegQuests {
		binary.MagicWeapon.SegQuests = make(map[uint32]*pb3.MagicWeaponQuest)
	}
	return binary.MagicWeapon
}

func (s *MagicWeaponSys) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	conf := jsondata.GetMagicWeaponConf()
	for _, v := range conf.Quests {
		if v.Id == questId {
			return v.Targets
		}
	}
	return nil
}

func (s *MagicWeaponSys) getQuestIdSetFunc(qt uint32) map[uint32]struct{} {
	conf := jsondata.GetMagicWeaponConf()
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

func (s *MagicWeaponSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	segQuest, ok := data.SegQuests[id]
	if !ok {
		return nil
	}
	if segQuest.IsRev {
		return nil
	}
	return segQuest.Quest
}

func (s *MagicWeaponSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	s.SendProto3(132, 11, &pb3.S2C_132_11{
		QuestData: s.getData().SegQuests[id].Quest,
	})
}

func (s *MagicWeaponSys) OnOpen() {
	s.acceptQuests()
	s.s2cInfo()
}

func (s *MagicWeaponSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MagicWeaponSys) OnAfterLogin() {
	s.acceptQuests()
	s.s2cInfo()
}

func (s *MagicWeaponSys) s2cInfo() {
	s.SendProto3(132, 10, &pb3.S2C_132_10{
		Info: s.getData(),
	})
}

func (s *MagicWeaponSys) acceptQuests() {
	conf := jsondata.GetMagicWeaponConf()
	data := s.getData()
	for _, questConf := range conf.Quests {
		if _, ok := data.SegQuests[questConf.Id]; ok {
			continue
		}

		data.SegQuests[questConf.Id] = &pb3.MagicWeaponQuest{
			Quest: &pb3.QuestData{
				Id: questConf.Id,
			},
		}
	}

	for _, segQuest := range data.SegQuests {
		s.OnAcceptQuest(segQuest.Quest)
	}
}

func (s *MagicWeaponSys) c2sSmallAward(msg *base.Message) error {
	var req pb3.C2S_132_12
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	id := req.QuestId
	data := s.getData()

	segQuest, ok := data.SegQuests[id]
	if !ok {
		return neterror.ParamsInvalidError("no segQuest %d", id)
	}

	if segQuest.IsRev {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	if !s.CheckFinishQuest(segQuest.Quest) {
		return neterror.ParamsInvalidError("task not finish")
	}

	questConf := jsondata.GetMagicWeaponQuestConf(id)
	if nil == questConf {
		return neterror.ConfNotFoundError("segQuest conf %d is nil", id)
	}

	segQuest.IsRev = true

	engine.GiveRewards(s.owner, questConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogMagicWeaponSegQuestAwards,
	})

	s.SendProto3(132, 12, &pb3.S2C_132_12{QuestId: id})

	return nil
}

func (s *MagicWeaponSys) c2sBigAward(_ *base.Message) error {
	data := s.getData()
	if data.IsLastRev {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetMagicWeaponConf()
	for _, v := range conf.Quests {
		segQuest, ok := data.SegQuests[v.Id]
		if !ok || (!s.CheckFinishQuest(segQuest.Quest) && !segQuest.IsRev) {
			return neterror.ParamsInvalidError("not finish")
		}
	}

	data.IsLastRev = true

	engine.GiveRewards(s.owner, conf.LastReward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogMagicWeaponAwards,
	})

	s.owner.SendShowRewardsPop(conf.LastReward)

	if conf.TipMsgId > 0 {
		engine.BroadcastTipMsgById(conf.TipMsgId, s.owner.GetId(), s.owner.GetName(), engine.StdRewardToBroadcast(s.owner, conf.LastReward))
	}

	s.SendProto3(132, 13, &pb3.S2C_132_13{})
	return nil
}

func (s *MagicWeaponSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	questData, ok := data.SegQuests[questId]
	if !ok {
		return
	}
	if questData.IsRev {
		s.GmFinishQuest(questData.Quest)
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(questData.Quest)
}

func (s *MagicWeaponSys) GMDelQuest(questId uint32) {
	data := s.getData()
	delete(data.SegQuests, questId)
}

func init() {
	RegisterSysClass(sysdef.SiMagicWeapon, func() iface.ISystem {
		return newMagicWeaponSys()
	})

	net.RegisterSysProtoV2(132, 12, sysdef.SiMagicWeapon, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MagicWeaponSys).c2sSmallAward
	})
	net.RegisterSysProtoV2(132, 13, sysdef.SiMagicWeapon, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MagicWeaponSys).c2sBigAward
	})
}
