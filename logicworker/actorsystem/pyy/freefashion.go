/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 免费时装
**/

package pyy

import (
	"fmt"
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
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type FreeFashionSys struct {
	*YYQuestTargetBase
}

func createYYFreeFashionSys() iface.IPlayerYY {
	obj := &FreeFashionSys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *FreeFashionSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}
	data := s.GetData()
	quests, ok := data.RoundQuestMap[data.CurRound]
	if !ok {
		return
	}
	if pie.Uint32s(quests.FinishQuestIds).Contains(quest.Id) {
		s.GetPlayer().LogWarn("quest finished")
		return
	}

	// 加入完成任务的队列
	if !s.CheckFinishQuest(quest) {
		//下发任务进度任务
		s.SendProto3(149, 2, &pb3.S2C_149_2{Quest: quest, ActiveId: s.Id})
		s.GetPlayer().LogWarn("quest not can finished")
		return
	}
	quests.FinishQuestIds = append(quests.FinishQuestIds, quest.Id)

	//下发任务进度任务
	s.SendProto3(149, 2, &pb3.S2C_149_2{Quest: quest, ActiveId: s.Id})
}

// 检查是否可以领奖
func (s *FreeFashionSys) checkCanRecAwards() bool {
	data := s.GetData()
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogWarn("not found conf ")
		return false
	}

	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return false
	}

	if data.IsRecAwards {
		s.GetPlayer().LogWarn("lock awards")
		return false
	}
	quests, ok := data.RoundQuestMap[data.CurRound]
	if !ok {
		return false
	}
	return len(quests.FinishQuestIds) == len(roundConf.QuestConf)
}

func (s *FreeFashionSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.GetData()

	quests, ok := data.RoundQuestMap[data.CurRound]
	if !ok {
		return nil
	}

	// 任务已经完成
	if pie.Uint32s(quests.FinishQuestIds).Contains(id) {
		return nil
	}

	for i := range quests.Quests {
		if quests.Quests[i].Id != id {
			continue
		}
		s.GetPlayer().LogInfo("free fashion quest is %v", quests.Quests[i])
		return quests.Quests[i]
	}

	return nil
}

func (s *FreeFashionSys) getTargetConfFunc(questId uint32) []*jsondata.QuestTargetConf {
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	data := s.GetData()
	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return nil
	}

	// 前置校验一下是不是空
	if roundConf.QuestConf == nil {
		return nil
	}

	questConf, ok := roundConf.QuestConf[questId]
	if !ok {
		return nil
	}

	return questConf.Targets
}

func (s *FreeFashionSys) getQuestIdSetFunc(qt uint32) map[uint32]struct{} {
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	data := s.GetData()
	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return nil
	}

	if roundConf.QuestConf == nil {
		return nil
	}

	set := make(map[uint32]struct{})
	for _, v := range roundConf.QuestConf {
		for _, target := range v.Targets {
			if target.Type != qt {
				continue
			}
			set[v.Id] = struct{}{}
		}
	}

	return set
}

func (s *FreeFashionSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.FreeFashionMap {
		return
	}
	delete(yyData.FreeFashionMap, s.Id)
}

func (s *FreeFashionSys) GetData() *pb3.YYFreeFashion {
	yyData := s.GetYYData()
	if nil == yyData.FreeFashionMap {
		yyData.FreeFashionMap = make(map[uint32]*pb3.YYFreeFashion)
	}
	if nil == yyData.FreeFashionMap[s.Id] {
		yyData.FreeFashionMap[s.Id] = &pb3.YYFreeFashion{}
	}
	if yyData.FreeFashionMap[s.Id].RoundQuestMap == nil {
		yyData.FreeFashionMap[s.Id].RoundQuestMap = make(map[uint32]*pb3.YYFreeFashionQuests)
	}
	if yyData.FreeFashionMap[s.Id].CurRound == 0 {
		yyData.FreeFashionMap[s.Id].CurRound = 1
	}
	return yyData.FreeFashionMap[s.Id]
}

func (s *FreeFashionSys) checkQuestProgress() {
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.GetData()
	quests, ok := data.RoundQuestMap[data.CurRound]
	if !ok {
		data.RoundQuestMap[data.CurRound] = &pb3.YYFreeFashionQuests{}
		quests = data.RoundQuestMap[data.CurRound]
	}

	// 已经接取任务
	if len(quests.Quests) != 0 {
		return
	}

	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return
	}

	for _, val := range roundConf.QuestConf {
		quests.Quests = append(quests.Quests, &pb3.QuestData{
			Id: val.Id,
		})
	}

	for _, val := range quests.Quests {
		s.OnAcceptQuestAndCheckUpdateTarget(val) // 检查任务进度
	}
}

func (s *FreeFashionSys) S2CInfo() {
	s.SendProto3(149, 1, &pb3.S2C_149_1{
		ActiveId: s.Id,
		State:    s.GetData(),
	})
}
func (s *FreeFashionSys) OnOpen() {
	s.checkQuestProgress()
	s.S2CInfo()
}

func (s *FreeFashionSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.checkQuestProgress()
	s.S2CInfo()
}

func (s *FreeFashionSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.checkQuestProgress()
	s.changeToNextRound()
	s.S2CInfo()
}

func (s *FreeFashionSys) OnEnd() {
	// 活动结束
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogWarn("not found conf ")
		return
	}
	if !s.checkCanRecAwards() {
		return
	}

	data := s.GetData()
	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return
	}

	if len(roundConf.Awards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYFreeFashion,
			Rewards: roundConf.Awards,
		})
	}
}

func (s *FreeFashionSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_149_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	if !s.IsOpen() {
		return neterror.SysNotExistError("%s not found free fashion sys", s.GetPrefix())
	}

	iPlayer := s.GetPlayer()

	if !s.checkCanRecAwards() {
		s.GetPlayer().LogWarn("not rec awards ")
		iPlayer.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	// 活动结束
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s yy free fashion conf", s.GetPrefix())
	}

	data := s.GetData()
	data.IsRecAwards = true
	roundConf, ok := conf.RoundConf[data.CurRound]
	if !ok {
		return neterror.ConfNotFoundError("%s yy free fashion conf, curRound is %d", s.GetPrefix(), data.CurRound)
	}

	if roundConf.Awards != nil {
		engine.GiveRewards(iPlayer, roundConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFreeFashion})
		// 去广播
		for _, val := range roundConf.Awards {
			itemConf := jsondata.GetItemConfig(val.Id)
			engine.BroadcastTipMsgById(conf.TipsId, iPlayer.GetName(), itemConf.Name)
		}
	}
	s.SendProto3(149, 3, &pb3.S2C_149_3{
		ActiveId: s.Id,
	})

	s.checkNextRound(data.CurRound + 1)
	s.checkClose()
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogFreeFashionRecAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", req.Base.ActiveId),
	})
	return nil
}

func (s *FreeFashionSys) checkClose() {
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.GetData()
	if !data.IsRecAwards {
		return
	}
	if data.CurRound != uint32(len(conf.RoundConf)) {
		return
	}
}

// 切换轮次
func (s *FreeFashionSys) checkNextRound(round uint32) {
	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.GetData()
	if round > uint32(len(conf.RoundConf)) {
		return
	}

	data.CurRound = round
	data.IsRecAwards = false
	s.checkQuestProgress()
	s.S2CInfo()
}

// NewDay 跨天
func (s *FreeFashionSys) NewDay() {
	s.changeToNextRound()
}

func (s *FreeFashionSys) changeToNextRound() {
	openDay := s.GetOpenDay()
	data := s.GetData()
	player := s.GetPlayer()

	// 20240902 ps: 活动第几天 活动就第几套 如果这里有问题 要找策划
	curRound := data.CurRound
	if curRound >= openDay {
		return
	}

	conf, ok := jsondata.GetYYFreeFashionConf(s.ConfName, s.ConfIdx)
	if !ok {
		player.LogWarn("%s not found free fashion %d", s.GetPrefix(), curRound)
		return
	}

	var sendAwardsByRound = func(curRound uint32) {
		roundConf, ok := conf.RoundConf[curRound]
		if !ok {
			return
		}

		if len(roundConf.Awards) > 0 {
			mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
				ConfId:  common.Mail_YYFreeFashion,
				Rewards: roundConf.Awards,
			})
		}
	}

	// 手动切到下一轮
	// 先检查这一轮是否能领奖
	if s.checkCanRecAwards() {
		sendAwardsByRound(curRound)
	}

	// 直接切到第几天的轮次
	s.checkNextRound(openDay)
}

func rangeFreeFashionSys(player iface.IPlayer, doLogic func(sys *FreeFashionSys)) {
	if doLogic == nil {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYFreeFashion)
	if len(yyList) == 0 {
		player.LogWarn("not found yy obj , id is %d", yydefine.YYFreeFashion)
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*FreeFashionSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		doLogic(sys)
	}

	return
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYFreeFashion, createYYFreeFashionSys)

	net.RegisterYYSysProtoV2(149, 1, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*FreeFashionSys).c2sAward
	})

	initFreeFashionGm()
}

func initFreeFashionGm() {
	// 免费时装领取奖励
	gmevent.Register("ffash.c2sAward", func(actor iface.IPlayer, args ...string) bool {
		rangeFreeFashionSys(actor, func(sys *FreeFashionSys) {
			msg := base.NewMessage()
			msg.SetCmd(149<<8 | 1)
			err := msg.PackPb3Msg(&pb3.C2S_149_1{
				Base: &pb3.YYBase{ActiveId: sys.Id},
			})
			if err != nil {
				actor.LogError(err.Error())
			}
			sys.GetPlayer().DoNetMsg(149, 1, msg)
		})
		return true
	}, 1)
	// 免费时装完成任务
	gmevent.Register("ffash.finish", func(actor iface.IPlayer, args ...string) bool {
		rangeFreeFashionSys(actor, func(sys *FreeFashionSys) {
			data := sys.GetData()
			quests := data.RoundQuestMap[data.CurRound]
			for _, quest := range quests.Quests {
				sys.GmFinishQuest(quest)
			}
		})

		return true
	}, 1)
}
