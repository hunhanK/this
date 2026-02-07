/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/commontimesconter"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type DemonKingSecKillSys struct {
	*QuestTargetBase
	counter *commontimesconter.CommonTimesCounter
}

func (s *DemonKingSecKillSys) OnInit() {
	data := s.getData()
	if data.Counter == nil {
		data.Counter = commontimesconter.NewCommonTimesCounterData()
	}
	s.counter = commontimesconter.NewCommonTimesCounter(
		s.getData().Counter,
		commontimesconter.WithOnGetFreeTimes(func() uint32 {
			config := jsondata.GetDemonKingSecKillConfig(s.getData().Level)
			if config == nil {
				return 0
			}
			return config.FreeTimes
		}),
		commontimesconter.WithOnUpdateCanUseTimes(func(canUseTimes uint32) {
			s.SendProto3(10, 43, &pb3.S2C_10_43{
				Counter: s.getData().Counter,
			})
			s.GetOwner().SetExtraAttr(attrdef.DemonKingSecKillTimes, attrdef.AttrValueAlias(canUseTimes))
		}),
	)
	err := s.counter.Init()
	if err != nil {
		s.LogError("init counter failed")
		return
	}
}

func (s *DemonKingSecKillSys) s2cInfo() {
	s.SendProto3(10, 40, &pb3.S2C_10_40{
		Data: s.getData(),
	})
}

func (s *DemonKingSecKillSys) getData() *pb3.DemonKingSecKillData {
	data := s.GetBinaryData().DemonKingSecKillData
	if data == nil {
		s.GetBinaryData().DemonKingSecKillData = &pb3.DemonKingSecKillData{}
		data = s.GetBinaryData().DemonKingSecKillData
	}
	return data
}

func (s *DemonKingSecKillSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DemonKingSecKillSys) OnLogin() {
	s.s2cInfo()
}

func (s *DemonKingSecKillSys) OnOpen() {
	s.initQuests()
	s.s2cInfo()
}

func (s *DemonKingSecKillSys) c2sOpen(msg *base.Message) error {
	var req pb3.C2S_10_41
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	config := jsondata.GetDemonKingSecKillCommonConfig()
	if config == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	owner := s.GetOwner()
	data := s.getData()
	data.IsOpen = !data.IsOpen
	if data.IsOpen {
		owner.LearnSkill(config.SkillId, config.SkillLv, false)
	} else {
		owner.ForgetSkill(config.SkillId, true, false, true)
	}
	s.SendProto3(10, 41, &pb3.S2C_10_41{
		IsOpen: data.IsOpen,
	})
	err = owner.CallActorFunc(actorfuncid.G2FOpenDemonKingSecKill, &pb3.CommonSt{
		BParam: data.IsOpen,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *DemonKingSecKillSys) initQuests() {
	data := s.getData()
	nextConfig := jsondata.GetDemonKingSecKillConfig(data.Level)
	if nextConfig == nil {
		return
	}
	data.Quests = nil
	for _, quest := range nextConfig.Quest {
		q := &pb3.QuestData{
			Id: quest.QuestId,
		}
		s.OnAcceptQuest(q)
		data.Quests = append(data.Quests, q)
	}
}

func (s *DemonKingSecKillSys) checkCompleteQuest() {
	data := s.getData()
	var completedCount int
	for _, quest := range data.Quests {
		if !s.CheckFinishQuest(quest) {
			continue
		}
		completedCount += 1
	}
	config := jsondata.GetDemonKingSecKillConfig(data.Level)
	if config == nil {
		return
	}
	if completedCount != len(config.Quest) {
		return
	}
	nextConfig := jsondata.GetDemonKingSecKillConfig(data.Level + 1)
	if nextConfig == nil {
		return
	}
	data.Level += 1
	data.Quests = nil
	for _, quest := range nextConfig.Quest {
		q := &pb3.QuestData{
			Id: quest.QuestId,
		}
		s.OnAcceptQuest(q)
		data.Quests = append(data.Quests, q)
	}
	s.SendProto3(10, 42, &pb3.S2C_10_42{
		Level:  data.Level,
		Quests: data.Quests,
	})
	s.counter.ReCalcTimes()
	return
}

func (s *DemonKingSecKillSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	data := s.getData()
	level := data.Level
	config := jsondata.GetDemonKingSecKillConfig(level)
	var set = make(map[uint32]struct{})
	if config == nil {
		return set
	}
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

func (s *DemonKingSecKillSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
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

func (s *DemonKingSecKillSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	data := s.getData()
	level := data.Level
	config := jsondata.GetDemonKingSecKillConfig(level)
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

func (s *DemonKingSecKillSys) onUpdateTargetData(id uint32) {
	data := s.getData()
	for _, quest := range data.Quests {
		if quest.Id != id {
			continue
		}
		s.SendProto3(10, 44, &pb3.S2C_10_44{
			Quest: quest,
		})
		s.checkCompleteQuest()
		return
	}
}

func NewDemonKingSecKillSys() iface.ISystem {
	sys := &DemonKingSecKillSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func handleF2GSubDemonKingSecKillTimes(player iface.IPlayer, buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		return
	}
	sys := getDemonKingSecKillSys(player)
	if sys == nil {
		return
	}
	sys.counter.DeductTimes(1)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogDemonKingSecKillDeductTimes, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.U32Param),
	})
}

func handleUseDemonKingSecKillDailyTimes(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys := getDemonKingSecKillSys(player)
	if sys == nil {
		return
	}
	sys.counter.AddDailyItemAddTimes(uint32(param.Count))
	return true, true, param.Count
}

func handleUseDemonKingSecKillTimes(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys := getDemonKingSecKillSys(player)
	if sys == nil {
		return
	}
	sys.counter.AddItemDailyAddTimes(uint32(param.Count))
	return true, true, param.Count
}

func getDemonKingSecKillSys(player iface.IPlayer) *DemonKingSecKillSys {
	obj := player.GetSysObj(sysdef.SiDemonKingSecKill)
	if obj == nil || !obj.IsOpen() {
		return nil
	}
	sys, ok := obj.(*DemonKingSecKillSys)
	if !ok {
		return nil
	}
	return sys
}

func handleDemonKingSecKillSysNewDay(player iface.IPlayer, args ...interface{}) {
	sys := getDemonKingSecKillSys(player)
	if sys == nil {
		return
	}
	sys.counter.NewDay()
	sys.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiDemonKingSecKill, NewDemonKingSecKillSys)
	net.RegisterSysProtoV2(10, 41, sysdef.SiDemonKingSecKill, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DemonKingSecKillSys).c2sOpen
	})
	event.RegActorEventL(custom_id.AeNewDay, handleDemonKingSecKillSysNewDay)
	engine.RegisterActorCallFunc(playerfuncid.F2GSubDemonKingSecKillTimes, handleF2GSubDemonKingSecKillTimes)

	miscitem.RegCommonUseItemHandle(itemdef.UseDemonKingSecKillDailyTimes, handleUseDemonKingSecKillDailyTimes)
	miscitem.RegCommonUseItemHandle(itemdef.UseDemonKingSecKillTimes, handleUseDemonKingSecKillTimes)
	gmevent.Register("DemonKingSecKillSys.quickFull", func(player iface.IPlayer, args ...string) bool {
		sys := getDemonKingSecKillSys(player)
		if sys == nil {
			return false
		}
		data := sys.getData()
		config := jsondata.GetDemonKingSecKillConfig(data.Level)
		level := data.Level
		for config != nil {
			for _, quest := range data.Quests {
				sys.GmFinishQuest(quest)
			}
			if data.Level == level {
				break
			}
			config = jsondata.GetDemonKingSecKillConfig(data.Level)
			level = data.Level
		}
		sys.s2cInfo()
		return true
	}, 1)
}
