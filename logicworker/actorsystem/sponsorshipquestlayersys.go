/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SponsorshipQuestLayerSys struct {
	*QuestTargetBase
}

func (s *SponsorshipQuestLayerSys) s2cInfo() {
	s.SendProto3(10, 60, &pb3.S2C_10_60{
		Data: s.getData(),
	})
}

func (s *SponsorshipQuestLayerSys) getData() *pb3.SponsorshipQuestLayerData {
	data := s.GetBinaryData().SponsorshipQuestLayerData
	if data == nil {
		s.GetBinaryData().SponsorshipQuestLayerData = &pb3.SponsorshipQuestLayerData{}
		data = s.GetBinaryData().SponsorshipQuestLayerData
	}
	return data
}

func (s *SponsorshipQuestLayerSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SponsorshipQuestLayerSys) OnLogin() {
	s.s2cInfo()
}

func (s *SponsorshipQuestLayerSys) OnOpen() {
	s.initQuests()
	s.s2cInfo()
}

func (s *SponsorshipQuestLayerSys) c2sQuestAwards(msg *base.Message) error {
	var req pb3.C2S_10_61
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.getData()
	config := jsondata.GetSponsorshipQuestLayerConfig(data.Layer)
	if config == nil {
		return neterror.ConfNotFoundError("sponsorshipQuestLayerConfig not found")
	}

	var qConf *jsondata.SponsorshipQuestLayerQuest
	for _, questConf := range config.Quest {
		if questConf.QuestId == req.QuestId {
			qConf = questConf
			break
		}
	}
	if qConf == nil {
		return neterror.ParamsInvalidError("quest %d not found", req.QuestId)
	}
	if pie.Uint32s(data.RecQuestIds).Contains(req.QuestId) {
		return neterror.ParamsInvalidError("already rec %d quest awards", req.QuestId)
	}
	var qData *pb3.QuestData
	for _, quest := range data.Quests {
		if quest.Id == req.QuestId {
			qData = quest
			break
		}
	}
	if qData == nil {
		return neterror.ParamsInvalidError("quest data %d not found", req.QuestId)
	}
	if !s.CheckFinishQuest(qData) {
		return neterror.ParamsInvalidError("quest %d not finish", req.QuestId)
	}
	data.RecQuestIds = append(data.RecQuestIds, qData.Id)
	owner := s.GetOwner()
	engine.GiveRewards(owner, qConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSponsorshipQuestLayerQuestAwards})
	owner.SendProto3(10, 61, &pb3.S2C_10_61{
		QuestId: qData.Id,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSponsorshipQuestLayerQuestAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.Layer),
		StrArgs: fmt.Sprintf("%d", qData.Id),
	})
	return nil
}

func (s *SponsorshipQuestLayerSys) c2sLayerAwards(_ *base.Message) error {
	data := s.getData()
	layer := data.Layer
	config := jsondata.GetSponsorshipQuestLayerConfig(layer)
	if config == nil {
		return neterror.ConfNotFoundError("sponsorshipQuestLayerConfig not found")
	}
	if len(data.RecQuestIds) != len(data.Quests) {
		return neterror.ParamsInvalidError("quest not finish")
	}
	owner := s.GetOwner()
	engine.GiveRewards(owner, config.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSponsorshipQuestLayerAwards})

	nextLayer := layer + 1
	data.Layer = nextLayer
	nextConfig := jsondata.GetSponsorshipQuestLayerConfig(nextLayer)
	if nextConfig != nil {
		data.Quests = nil
		data.RecQuestIds = nil
		for _, questConf := range nextConfig.Quest {
			q := &pb3.QuestData{
				Id: questConf.QuestId,
			}
			data.Quests = append(data.Quests, q)
			s.OnAcceptQuest(q)
		}
	}
	s.SendProto3(10, 62, &pb3.S2C_10_62{
		Data: s.getData(),
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSponsorshipQuestLayerAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(layer),
		StrArgs: fmt.Sprintf("%d", data.Layer),
	})
	return nil
}

func (s *SponsorshipQuestLayerSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	data := s.getData()
	config := jsondata.GetSponsorshipQuestLayerConfig(data.Layer)
	if config == nil {
		return nil
	}
	var set = make(map[uint32]struct{})
	for _, conf := range config.Quest {
		var exist bool
		for _, target := range conf.Targets {
			if target.Type != qtt {
				continue
			}
			exist = true
			break
		}
		if exist {
			set[conf.QuestId] = struct{}{}
		}
	}
	return set
}

func (s *SponsorshipQuestLayerSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	for _, quest := range data.Quests {
		if quest.Id != id {
			continue
		}
		if s.CheckFinishQuest(quest) {
			return nil
		}
		return quest
	}
	return nil
}

func (s *SponsorshipQuestLayerSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	data := s.getData()
	config := jsondata.GetSponsorshipQuestLayerConfig(data.Layer)
	if config == nil {
		return nil
	}
	for _, conf := range config.Quest {
		if conf.QuestId != id {
			continue
		}
		return conf.Targets
	}
	return nil
}

func (s *SponsorshipQuestLayerSys) onUpdateTargetData(id uint32) {
	data := s.getData()
	for _, quest := range data.Quests {
		if quest.Id != id {
			continue
		}
		s.SendProto3(10, 63, &pb3.S2C_10_63{
			Quest: quest,
		})
		return
	}
}

func (s *SponsorshipQuestLayerSys) initQuests() {
	data := s.getData()
	if data.Layer == 0 {
		data.Layer = 1
	}
	config := jsondata.GetSponsorshipQuestLayerConfig(data.Layer)
	if config == nil {
		return
	}
	data.Quests = nil
	data.RecQuestIds = nil
	for _, questConf := range config.Quest {
		q := &pb3.QuestData{
			Id: questConf.QuestId,
		}
		s.OnAcceptQuest(q)
		data.Quests = append(data.Quests, q)
	}
}

func (s *SponsorshipQuestLayerSys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	for _, quest := range data.Quests {
		if quest.Id != questId {
			continue
		}
		if pie.Uint32s(data.RecQuestIds).Contains(questId) {
			s.GmFinishQuest(quest)
			return
		}
		s.OnAcceptQuestAndCheckUpdateTarget(quest)
		return
	}
}

func (s *SponsorshipQuestLayerSys) GMDelQuest(questId uint32) {
	data := s.getData()
	var newQuests []*pb3.QuestData
	for _, quest := range data.Quests {
		if quest.Id == questId {
			continue
		}
		newQuests = append(newQuests, quest)
	}
	data.Quests = newQuests
	data.RecQuestIds = pie.Uint32s(data.RecQuestIds).Filter(func(u uint32) bool {
		return u != questId
	})
}

func getSponsorshipQuestLayerSys(player iface.IPlayer) *SponsorshipQuestLayerSys {
	obj := player.GetSysObj(sysdef.SiSponsorshipQuestLayer)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*SponsorshipQuestLayerSys)
	if !ok {
		return nil
	}
	return sys
}

func NewSponsorshipQuestLayerSys() iface.ISystem {
	sys := &SponsorshipQuestLayerSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func init() {
	RegisterSysClass(sysdef.SiSponsorshipQuestLayer, NewSponsorshipQuestLayerSys)
	net.RegisterSysProtoV2(10, 61, sysdef.SiSponsorshipQuestLayer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SponsorshipQuestLayerSys).c2sQuestAwards
	})
	net.RegisterSysProtoV2(10, 62, sysdef.SiSponsorshipQuestLayer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SponsorshipQuestLayerSys).c2sLayerAwards
	})
	gmevent.Register("SponsorshipQuestLayerSys.complete", func(player iface.IPlayer, args ...string) bool {
		sys := getSponsorshipQuestLayerSys(player)
		if sys == nil {
			return false
		}
		data := sys.getData()
		for _, quest := range data.Quests {
			sys.GmFinishQuest(quest)
		}
		sys.s2cInfo()
		return true
	}, 1)
}
