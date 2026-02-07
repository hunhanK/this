/**
 * @Author:
 * @Date:
 * @Desc: 庆典狂嗨
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type CelebratorySys struct {
	*YYQuestTargetBase
}

func createCelebratorySys() iface.IPlayerYY {
	obj := &CelebratorySys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *CelebratorySys) s2cInfo() {
	s.SendProto3(8, 160, &pb3.S2C_8_160{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *CelebratorySys) getData() *pb3.PYYCelebratoryData {
	state := s.GetYYData()
	if nil == state.CelebratoryData {
		state.CelebratoryData = make(map[uint32]*pb3.PYYCelebratoryData)
	}
	if state.CelebratoryData[s.Id] == nil {
		state.CelebratoryData[s.Id] = &pb3.PYYCelebratoryData{}
	}
	data := state.CelebratoryData[s.Id]
	if data.QuestMap == nil {
		data.QuestMap = make(map[uint32]*pb3.PYYCelebratoryQuest)
	}
	return data
}

func (s *CelebratorySys) ResetData() {
	state := s.GetYYData()
	if nil == state.CelebratoryData {
		return
	}
	delete(state.CelebratoryData, s.Id)
}

func (s *CelebratorySys) OnReconnect() {
	s.s2cInfo()
}

func (s *CelebratorySys) Login() {
	s.s2cInfo()
}

func (s *CelebratorySys) OnOpen() {
	s.acceptAllQuest()
	s.s2cInfo()
}

func (s *CelebratorySys) NewDay() {
	data := s.getData()
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	for _, quest := range conf.CelebratoryQuest {
		questData, ok := data.QuestMap[quest.Id]
		if ok {
			continue
		}
		if quest.DailyReset {
			questData.Times = 0
		}
	}
	s.s2cInfo()
}

func (s *CelebratorySys) getConf() (*jsondata.PYYCelebratoryConfig, error) {
	config := jsondata.GetCelebratoryConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return nil, neterror.ConfNotFoundError("%s %d not found conf", s.ConfName, s.ConfIdx)
	}
	return config, nil
}

func (s *CelebratorySys) acceptAllQuest() {
	data := s.getData()
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	for _, quest := range conf.CelebratoryQuest {
		if _, ok := data.QuestMap[quest.Id]; ok {
			continue
		}
		data.QuestMap[quest.Id] = &pb3.PYYCelebratoryQuest{
			Data: &pb3.QuestData{
				Id:       quest.Id,
				Progress: nil,
			},
		}
		s.OnAcceptQuestAndCheckUpdateTarget(data.QuestMap[quest.Id].Data)
	}
}

func (s *CelebratorySys) c2sRecAwards(msg *base.Message) error {
	var req pb3.C2S_8_163
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	data := s.getData()
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	var processAwardConf *jsondata.CelebratoryProcessAwards
	for _, processAwards := range conf.ProcessAwards {
		if processAwards.Process != req.Process {
			continue
		}
		processAwardConf = processAwards
		break
	}
	if processAwardConf == nil {
		return neterror.ConfNotFoundError("%d not found processAwardConf", req.Process)
	}
	if utils.IsSetBit64(data.ReceiveFlag, processAwardConf.Idx) {
		return neterror.ParamsInvalidError("%d already rec", processAwardConf.Idx)
	}
	if data.Process < processAwardConf.Process {
		return neterror.ParamsInvalidError("%d %d not reach", data.Process, req.Process)
	}
	data.ReceiveFlag = utils.SetBit64(data.ReceiveFlag, processAwardConf.Idx)
	if len(processAwardConf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), processAwardConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYCelebratoryRecAwards})
	}
	s.SendProto3(8, 163, &pb3.S2C_8_163{
		ActiveId:    s.Id,
		ReceiveFlag: data.ReceiveFlag,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYCelebratoryRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", processAwardConf.Idx),
	})
	return nil
}

func (s *CelebratorySys) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return nil
	}
	for _, quest := range conf.CelebratoryQuest {
		if quest.Id == id {
			return quest.Targets
		}
	}
	return nil
}

func (s *CelebratorySys) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return nil
	}
	for _, quest := range conf.CelebratoryQuest {
		for _, target := range quest.Targets {
			if target.Type == qtt {
				set[quest.Id] = struct{}{}
				break
			}
		}
	}
	return set
}

func (s *CelebratorySys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	questData := data.QuestMap[id]
	if questData == nil {
		return nil
	}
	if s.CheckFinishQuest(questData.Data) {
		return nil
	}
	return questData.Data
}

func (s *CelebratorySys) onUpdateTargetData(id uint32) {
	conf, err := s.getConf()
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	var questConf *jsondata.CelebratoryQuest
	for _, quest := range conf.CelebratoryQuest {
		if quest.Id != id {
			continue
		}
		questConf = quest
		break
	}
	if questConf == nil {
		return
	}
	data := s.getData()
	questData := data.QuestMap[id]
	if s.CheckFinishQuest(questData.Data) && questData.Times < questConf.Count {
		data.Process += questConf.Process
		questData.Times += 1
		questData.Data.Progress = nil
		s.SendProto3(8, 162, &pb3.S2C_8_162{
			ActiveId: s.Id,
			Process:  data.Process,
		})
		logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYCelebratoryCompleteQuest, &pb3.LogPlayerCounter{
			NumArgs: uint64(s.GetId()),
			StrArgs: fmt.Sprintf("%d_%d_%d", id, questData.Times, questConf.Process),
		})
	}
	s.SendProto3(8, 161, &pb3.S2C_8_161{
		ActiveId: s.Id,
		Data:     questData,
	})
}

func (s *CelebratorySys) GMReAcceptQuest(questId uint32) {
	data := s.getData()
	questData := data.QuestMap[questId]
	if questData == nil {
		return
	}
	s.OnAcceptQuestAndCheckUpdateTarget(questData.Data)
}

func handleCelebratoryAeActDrawTimes(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	drawEvent, ok := args[0].(*custom_id.ActDrawEvent)
	if !ok {
		return
	}

	player.TriggerQuestEvent(custom_id.QttMayDayBlessXTimes, drawEvent.ActId, int64(drawEvent.Times))
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYCelebratory, createCelebratorySys)
	net.RegisterYYSysProtoV2(8, 163, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CelebratorySys).c2sRecAwards
	})
	event.RegActorEvent(custom_id.AeActDrawTimes, handleCelebratoryAeActDrawTimes)
	gmevent.Register("CelebratorySys.addPoint", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYCelebratory, func(obj iface.IPlayerYY) {
			obj.(*CelebratorySys).getData().Process += utils.AtoUint32(args[0])
			obj.(*CelebratorySys).s2cInfo()
		})
		return true
	}, 1)
	gmevent.Register("CelebratorySys.gmAllQuest", func(player iface.IPlayer, args ...string) bool {
		pyymgr.EachPlayerAllYYObj(player, yydefine.PYYCelebratory, func(obj iface.IPlayerYY) {
			for _, quest := range obj.(*CelebratorySys).getData().QuestMap {
				obj.(*CelebratorySys).GmFinishQuest(quest.Data)
			}
			obj.(*CelebratorySys).s2cInfo()
		})
		return true
	}, 1)
}
