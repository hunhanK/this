/**
 * @Author: beiming
 * @Desc: 限时转盘
 * @Date: 2024/03/15
 */

package pyy

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"golang.org/x/exp/slices"
)

type TurntableDraw struct {
	*YYQuestTargetBase
}

func createYYTurntableDrawSys() iface.IPlayerYY {
	obj := &TurntableDraw{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetTargetConfFunc:        obj.getTargetConfFunc,
		GetQuestIdSetFunc:        obj.getQuestIdSetFunc,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

func (s *TurntableDraw) OnOpen() {
	s.getData()
	s.acceptQuests()
	s.s2cInfo()
	s.checkQuestFinish()
}

func (s *TurntableDraw) OnEnd() {
	s.reissueRewards()
}

func (s *TurntableDraw) Login() {
	s.reissueRewards()
	s.s2cInfo()
}

func (s *TurntableDraw) OnReconnect() {
	s.reissueRewards()
	s.s2cInfo()
}

func (s *TurntableDraw) getData() *pb3.PYY_TurntableDraw {
	data := s.GetYYData()

	if data.TurntableDraw == nil {
		data.TurntableDraw = make(map[uint32]*pb3.PYY_TurntableDraw)
	}

	if data.TurntableDraw[s.GetId()] == nil {
		data.TurntableDraw[s.GetId()] = &pb3.PYY_TurntableDraw{Round: 1}
	}

	return data.TurntableDraw[s.GetId()]
}

func (s *TurntableDraw) ResetData() {
	data := s.GetYYData()

	if data.TurntableDraw == nil {
		return
	}
	delete(data.TurntableDraw, s.Id)
}

// 补发奖励
func (s *TurntableDraw) reissueRewards() {
	data := s.getData()
	if data.NoRecvAwards != nil {
		rewards := jsondata.Pb3RewardVecToStdRewardVec(data.NoRecvAwards)
		if !engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTurntableDrawReward,
		}) {
			s.LogError("turntableDraw GiveRewards failed")
			return
		}
		data.NoRecvAwards = nil
	}
}

func (s *TurntableDraw) s2cInfo() {
	s.SendProto3(127, 50, &pb3.S2C_127_50{
		Id:   s.GetId(),
		Data: s.getData(),
	})
}

func (s *TurntableDraw) c2sTurn(msg *base.Message) error {
	var req pb3.C2S_127_51
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getData()

	if data.GetCount() <= 0 {
		return neterror.ParamsInvalidError("次数不足")
	}

	ac := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if ac == nil {
		return neterror.ConfNotFoundError("%s 限时转盘配置不存在", s.GetPrefix())
	}

	d := s.GetOpenDay()
	c := ac.GetYYTurntableDraw(data.GetRound(), d)
	if c == nil {
		return neterror.ConfNotFoundError("%s 限时转盘配置不存在round:%d, day:%d", s.GetPrefix(), data.GetRound(), d)
	}

	p := new(random.Pool)
	for k, item := range c.Pool {
		idx := uint32(k + 1) // idx 从1开始
		if slices.Contains(data.RecvIdx, idx) {
			continue
		}
		p.AddItem(idx, item.Weight)
	}

	if p.Size() == 0 {
		return neterror.ParamsInvalidError("已经抽完了")
	}

	idx := p.RandomOne().(uint32)
	if idx == 0 {
		return neterror.ParamsInvalidError("抽取失败")
	}

	item := c.Pool[idx-1]

	data.RecvIdx = append(data.RecvIdx, idx)
	data.Count--
	data.Idx = idx

	// 防止玩家掉线, 无法领取奖励
	// 将奖励缓存起来, 等待玩家下次登录时领取
	data.NoRecvAwards = base.StdRewardToProto(item.Rewards)

	dur := time.Second * time.Duration(int64(ac.TurnTime))
	s.GetPlayer().SetTimeout(dur, func() {
		if !engine.GiveRewards(s.GetPlayer(), item.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTurntableDrawReward,
		}) {
			s.LogError("turntableDraw GiveRewards failed")
			return
		}
		data.NoRecvAwards = nil
	})

	s.SendProto3(127, 51, &pb3.S2C_127_51{
		Id:      s.GetId(),
		Count:   data.Count,
		Idx:     data.Idx,
		Round:   data.Round,
		RecvIdx: data.RecvIdx,
	})

	nextRound := p.Size() == 1
	if nextRound {
		nc := ac.GetYYTurntableDraw(data.GetRound()+1, uint32(d))
		if nc == nil && c.IsLoop {
			nc = c
		}

		if nc != nil {
			data.Round = nc.Round
			data.RecvIdx = nil
			data.Idx = 0

			// 刷新任务
			s.resetNextRoundQuest()
			s.s2cInfo()
		}
	}

	return nil
}

func (s *TurntableDraw) NewDay() {
	data := s.getData()
	data.RecvIdx = nil
	data.Idx = 0
	data.Round = 1
	data.Quests = nil

	// 将剩余次数装换成奖励，通过邮件发给玩家
	if data.Count > 0 {
		s.dailyRewards()
	}

	s.acceptQuests()

	s.s2cInfo()

	s.checkQuestFinish()
}

func (s *TurntableDraw) dailyRewards() {
	data := s.getData()

	cfg := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if cfg == nil {
		return
	}
	rewards := jsondata.StdRewardMulti(cfg.RemainReward, int64(data.Count))

	var rewardsCount int64
	for _, reward := range rewards {
		rewardsCount += reward.Count
	}

	if len(rewards) > 0 {
		d1 := int64(data.Count)
		data.Count = 0

		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_TurntableDraw,
			Rewards: rewards,
			Content: &mailargs.CommonMailArgs{
				Digit1: d1,
				Digit2: rewardsCount,
			},
		})

	}
}

// acceptQuests 接受任务
func (s *TurntableDraw) acceptQuests() {
	data := s.getData()

	if len(data.Quests) > 0 {
		return
	}

	ac := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if ac == nil {
		s.GetPlayer().LogError("%s 限时转盘配置不存在", s.GetPrefix())
		return
	}

	d := s.GetOpenDay()
	c := ac.GetYYTurntableDraw(data.GetRound(), d)
	if c == nil {
		s.GetPlayer().LogError("%s 限时转盘配置不存在 round:%d, day:%d", s.GetPrefix(), data.GetRound(), d)
		return
	}

	data.Quests = make([]*pb3.QuestData, 0, len(c.Quests))

	for _, quest := range c.Quests {
		data.Quests = append(data.Quests, &pb3.QuestData{Id: quest.Id})
	}

	for _, quest := range data.Quests {
		s.OnAcceptQuest(quest)
	}
}

func (s *TurntableDraw) checkQuestFinish() {
	data := s.getData()
	for _, quest := range data.Quests {
		s.onUpdateTargetData(quest.Id)
	}
}

// resetQuest 重置下一轮任务
func (s *TurntableDraw) resetNextRoundQuest() {
	data := s.getData()

	ac := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if ac == nil {
		s.GetPlayer().LogError("%s 限时转盘配置不存在", s.GetPrefix())
		return
	}

	d := s.GetOpenDay()
	c := ac.GetYYTurntableDraw(data.GetRound(), uint32(d))
	if c == nil {
		s.GetPlayer().LogError("%s 限时转盘配置不存在 round:%d, day:%d", s.GetPrefix(), data.GetRound(), d)
		return
	}

	// 清空任务
	data.Quests = make([]*pb3.QuestData, 0, len(c.Quests))

	for _, quest := range c.Quests {
		qd := &pb3.QuestData{Id: quest.Id}
		s.OnAcceptQuest(qd)

		data.Quests = append(data.Quests, qd)

		cfg := s.getQuest(qd.Id)
		if cfg == nil {
			continue
		}

		if s.CheckFinishQuest(qd) {
			data.Count += cfg.DrawTimes
		}
	}
}

func (s *TurntableDraw) getQuest(id uint32) *jsondata.TurntableDrawQuest {
	conf := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil
	}

	d := s.GetOpenDay()

	var td *jsondata.TurntableDraw
	for _, record := range conf.TurntableDraw {
		if record.Day == uint32(d) {
			td = record
			break
		}
	}

	if td == nil {
		return nil
	}

	for _, v := range td.Quests {
		if v.Id == id {
			return v
		}
	}

	return nil
}

func (s *TurntableDraw) getTargetConfFunc(id uint32) []*jsondata.QuestTargetConf {
	q := s.getQuest(id)
	if q == nil {
		return nil
	}

	return q.Targets
}

func (s *TurntableDraw) getQuestIdSetFunc(qtt uint32) map[uint32]struct{} {
	conf := jsondata.GetYYTurntableDrawConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	d := s.GetOpenDay()

	var td *jsondata.TurntableDraw
	for _, record := range conf.TurntableDraw {
		if record.Day == uint32(d) {
			td = record
			break
		}
	}

	set := make(map[uint32]struct{})
	if td == nil {
		return set
	}

	for _, v := range td.Quests {
		for _, target := range v.Targets {
			if target.Type == qtt {
				set[v.Id] = struct{}{}
			}
		}
	}

	return set
}

func (s *TurntableDraw) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()

	for i := range data.Quests {
		if data.Quests[i].Id == id && !s.CheckFinishQuest(data.Quests[i]) {
			return data.Quests[i]
		}
	}

	return nil
}

func (s *TurntableDraw) onUpdateTargetData(id uint32) {
	data := s.getData()
	var quest *pb3.QuestData
	for i := range data.Quests {
		if data.Quests[i].Id == id {
			quest = data.Quests[i]
		}
	}

	if quest == nil {
		return
	}

	cfg := s.getQuest(id)
	if cfg == nil {
		return
	}

	if s.CheckFinishQuest(quest) {
		s.onQuestFinish(quest)
		data.Count += cfg.DrawTimes
	}

	s.SendProto3(127, 52, &pb3.S2C_127_52{Id: s.GetId(), Quest: quest, Count: data.Count})
}

func (s *TurntableDraw) onQuestFinish(quest *pb3.QuestData) {
	logArgs := map[string]any{
		"configName": s.ConfName,
		"activityId": s.GetId(),
		"confIdx":    s.GetConfIdx(),
		"questId":    quest.GetId(),
	}
	logArgByte, _ := json.Marshal(logArgs)
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogTurntableDrawQuestFinish, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: string(logArgByte),
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTurntableDraw, createYYTurntableDrawSys)

	net.RegisterYYSysProtoV2(127, 51, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*TurntableDraw).c2sTurn
	})

	// 增加抽奖次数, ttd.count,yyId,count
	gmevent.Register("ttd.count", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		count := utils.AtoInt(args[1])
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		obj := sys.GetObjById(yyId)
		if nil == obj || obj.GetClass() != yydefine.YYTurntableDraw {
			return false
		}

		s := obj.(*TurntableDraw)

		c := s.getData().Count
		nc := int(c) + count
		if nc < 0 {
			nc = 0
		}
		s.getData().Count = uint32(nc)

		s.s2cInfo()

		return true
	}, 1)

	// 完成当前轮次任务 ttd.finish,yyId
	gmevent.Register("ttd.finish", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		obj := sys.GetObjById(yyId)
		if nil == obj || obj.GetClass() != yydefine.YYTurntableDraw {
			return false
		}

		s := obj.(*TurntableDraw)

		data := s.getData()
		for i, quest := range data.Quests {
			if !s.CheckFinishQuest(data.Quests[i]) {
				s.GmFinishQuest(quest)
			}
		}

		return true
	}, 1)
}
