/**
 * @Author: zjj
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-玩法得分目标比拼
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/component"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"time"
)

type GameplayScoreGoalRank struct {
	YYBase

	ScoreRank *component.ScoreRank
}

func (s *GameplayScoreGoalRank) getData() *pb3.YYGameplayScoreGoalRank {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.GameplayScoreGoalRankMap == nil {
		datas.GameplayScoreGoalRankMap = make(map[uint32]*pb3.YYGameplayScoreGoalRank)
	}
	if datas.GameplayScoreGoalRankMap[s.Id] == nil {
		datas.GameplayScoreGoalRankMap[s.Id] = &pb3.YYGameplayScoreGoalRank{}
	}
	if datas.GameplayScoreGoalRankMap[s.Id].ActorRecordMap == nil {
		datas.GameplayScoreGoalRankMap[s.Id].ActorRecordMap = make(map[uint64]*pb3.YYGameplayScoreGoalRankDailyReachGoalFlag)
	}
	if datas.GameplayScoreGoalRankMap[s.Id].DailyRewardFlagMap == nil {
		datas.GameplayScoreGoalRankMap[s.Id].DailyRewardFlagMap = make(map[uint32]bool)
	}
	if datas.GameplayScoreGoalRankMap[s.Id].JoinFlagMap == nil {
		datas.GameplayScoreGoalRankMap[s.Id].JoinFlagMap = make(map[uint64]bool)
	}
	if datas.GameplayScoreGoalRankMap[s.Id].Day == 0 {
		datas.GameplayScoreGoalRankMap[s.Id].Day = s.GetOpenDay()
	}
	return datas.GameplayScoreGoalRankMap[s.Id]
}

func (s *GameplayScoreGoalRank) getPlayerDailyReachGoalFlagData(actorId uint64) *pb3.YYGameplayScoreGoalRankDailyReachGoalFlag {
	data := s.getData()
	flagData, ok := data.ActorRecordMap[actorId]
	if !ok {
		data.ActorRecordMap[actorId] = &pb3.YYGameplayScoreGoalRankDailyReachGoalFlag{}
		flagData = data.ActorRecordMap[actorId]
	}
	if flagData.DailyReachGoalFlagMap == nil {
		flagData.DailyReachGoalFlagMap = make(map[uint32]uint32)
	}
	if flagData.DailyScoreMap == nil {
		flagData.DailyScoreMap = make(map[uint32]int64)
	}
	return flagData
}

func (s *GameplayScoreGoalRank) OnInit() {
	s.YYBase.OnInit()
	data := s.getData()
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	data.RankType = conf.RankType
	s.ScoreRank = component.NewScoreRank(
		conf.CommonScoreRank,
		component.WithScoreRankOptionByPacketPlayerInfoToRankInfo(manager.ExportPacketPlayerInfoToRankInfo),
		component.WithScoreRankOptionByNoReachMinScoreStopUpdate(),
	)
	s.ScoreRank.InitialUpdate(data.Items)
}

func (s *GameplayScoreGoalRank) resetCurrentRankJoinFlag(player iface.IPlayer) {
	key := ranktype.GetGameplayScoreGoalRankYYRankKey(ranktype.GameplayScoreGoalRankType(s.getData().RankType))
	s.resetJoinFlag(player, key)
}

func (s *GameplayScoreGoalRank) PlayerLogin(player iface.IPlayer) {
	s.resetCurrentRankJoinFlag(player)
	s.sendTodayScore(player)
	err := s.c2sRankData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *GameplayScoreGoalRank) PlayerReconnect(player iface.IPlayer) {
	s.sendTodayScore(player)
	err := s.c2sRankData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *GameplayScoreGoalRank) packetPlayerInfo(playerId uint64, rankInfo *pb3.RankInfo) {
	manager.PacketPlayScoreRankPlayerInfo(playerId, rankInfo)
}

func (s *GameplayScoreGoalRank) BroadcastRank() {
	var rsp = &pb3.S2C_61_20{
		ActiveId: s.Id,
		RankType: s.getData().RankType,
	}
	data, err := s.ScoreRank.GetRankData(false)
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	rsp.RankMgr = data
	s.Broadcast(61, 20, rsp)
}

func (s *GameplayScoreGoalRank) c2sRankData(player iface.IPlayer, _ *base.Message) error {
	data := s.getData()

	rankData, err := s.ScoreRank.GetRankData(false)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	info := s.ScoreRank.GetMyRankInfo(rankData, player.GetId())
	s.packetPlayerInfo(player.GetId(), info)
	if info.Value == 0 {
		key := ranktype.GetGameplayScoreGoalRankYYRankKey(ranktype.GameplayScoreGoalRankType(data.RankType))
		s.resetJoinFlag(player, key)
		info.Value = player.GetYYRankValue(key)
	}
	flagData := s.getPlayerDailyReachGoalFlagData(player.GetId())
	var rsp = &pb3.S2C_61_21{
		ActiveId:              s.Id,
		RankType:              data.RankType,
		RankMgr:               rankData,
		MyRankInfo:            info,
		DailyReachGoalFlagMap: flagData.DailyReachGoalFlagMap,
	}

	player.SendProto3(61, 21, rsp)
	return nil
}

func (s *GameplayScoreGoalRank) c2sRecReachAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_61_22
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	confIdx := s.ConfIdx
	confName := s.ConfName
	idx := req.Idx
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(confName, confIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s %d not found conf", confName, confIdx)
	}

	if conf.DailyReach == nil || len(conf.DailyReach) == 0 {
		return neterror.ConfNotFoundError("%s %d not found daily reach conf", confName, confIdx)
	}

	openDay := s.getData().Day
	reachGoalFlagData := s.getPlayerDailyReachGoalFlagData(player.GetId())

	flag := reachGoalFlagData.DailyReachGoalFlagMap[openDay]
	if utils.IsSetBit(flag, idx) {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	reachConf, ok := conf.DailyReach[openDay]
	if !ok {
		return neterror.ConfNotFoundError("%s %d not found daily reach day %d conf", confName, confIdx, openDay)
	}

	dailyScore := reachGoalFlagData.DailyScoreMap[openDay]
	reachAwardsConf := reachConf.ReachAwards[idx]
	if reachAwardsConf == nil {
		return neterror.ConfNotFoundError("%s %d not found daily reach idx %d conf", confName, confIdx, idx)
	}
	minPower := reachAwardsConf.MinPower
	if minPower > dailyScore {
		return neterror.ParamsInvalidError("%s %d %d %d > %d", confName, confIdx, idx, minPower, dailyScore)
	}

	flag = utils.SetBit(flag, idx)
	reachGoalFlagData.DailyReachGoalFlagMap[openDay] = flag

	if len(reachAwardsConf.Awards) > 0 {
		engine.GiveRewards(player, reachAwardsConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogYYGameplayScoreGoalRankRecDailyAwards,
		})
	}

	player.SendProto3(61, 22, &pb3.S2C_61_22{
		ActiveId: s.GetId(),
		Idx:      idx,
		Day:      openDay,
		Flag:     flag,
	})
	return nil
}

func (s *GameplayScoreGoalRank) sendTodayScore(player iface.IPlayer) {
	player.SendProto3(61, 23, &pb3.S2C_61_23{
		ActiveId: s.GetId(),
		RankType: s.getData().RankType,
		Day:      s.getData().Day,
		Score:    s.getPlayerDailyReachGoalFlagData(player.GetId()).DailyScoreMap[s.getData().Day],
	})
}

func (s *GameplayScoreGoalRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.GameplayScoreGoalRankMap == nil {
		datas.GameplayScoreGoalRankMap = make(map[uint32]*pb3.YYGameplayScoreGoalRank)
	}
	datas.GameplayScoreGoalRankMap[s.Id] = &pb3.YYGameplayScoreGoalRank{}
}

func (s *GameplayScoreGoalRank) OnOpen() {
	s.OnInit()
	s.BroadcastRank()
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.sendTodayScore(player)
	})
}

// 补发今天未领取的奖励
func (s *GameplayScoreGoalRank) reissueTodayAwards() {
	day := s.getData().Day
	data := s.getData()
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if data.DailyRewardFlagMap[day] {
		return
	}
	data.DailyRewardFlagMap[day] = true
	dailyReachConf := conf.DailyReach[day]

	for actorId := range data.ActorRecordMap {
		flagData := s.getPlayerDailyReachGoalFlagData(actorId)
		flag := flagData.DailyReachGoalFlagMap[day]
		score, _ := s.ScoreRank.Rank.GetScoreById(int64(actorId))
		var awards jsondata.StdRewardVec

		for _, reachAwards := range dailyReachConf.ReachAwards {
			if utils.IsSetBit(flag, reachAwards.Idx) {
				continue
			}
			if reachAwards.MinPower > score {
				continue
			}
			awards = append(awards, reachAwards.Awards...)
			flag = utils.SetBit(flag, reachAwards.Idx)
		}

		flagData.DailyReachGoalFlagMap[day] = flag
		if len(awards) > 0 {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  uint16(conf.ReissueMailId),
				Rewards: awards,
			})
		}
	}
}

func (s *GameplayScoreGoalRank) OnEnd() {
	s.reissueTodayAwards()
	s.settlement()
}

func (s *GameplayScoreGoalRank) ServerStopSaveData() {
	s.getData().Items = s.ScoreRank.PackToProto()
}

func (s *GameplayScoreGoalRank) resetJoinFlag(player iface.IPlayer, k uint32) {
	data := s.getData()
	playerId := player.GetId()
	if flag := data.JoinFlagMap[playerId]; !flag {
		player.SetYYRankValue(k, 0)
		data.JoinFlagMap[playerId] = true
	}
}

func (s *GameplayScoreGoalRank) updateRank(rankType ranktype.GameplayScoreGoalRankType, playerId uint64, score int64, isAdd bool) {
	data := s.getData()

	if data.RankType != uint32(rankType) {
		return
	}

	if data.IsSettlement {
		return
	}

	key := ranktype.GetGameplayScoreGoalRankYYRankKey(rankType)
	player := manager.GetPlayerPtrById(playerId)
	s.resetJoinFlag(player, key)
	value := player.GetYYRankValue(key)
	if value >= score {
		return
	}

	var newScore = score
	if isAdd {
		oldScore := player.GetYYRankValue(uint32(rankType))
		newScore = oldScore + score
	}

	player.SetYYRankValue(key, newScore)
	if s.ScoreRank.Update(playerId, newScore) {
		day := s.getData().Day
		flagData := s.getPlayerDailyReachGoalFlagData(playerId)
		flagData.DailyScoreMap[day] = newScore
		s.sendTodayScore(player)
		s.BroadcastRank()
	}
}

func (s *GameplayScoreGoalRank) onHourArrive(hour int) {
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	// 不是每日刷新 那就是需要活动快结束前x小时进行结算
	if !conf.DailyRefresh {
		endTime := s.GetEndTime()
		nowSec := time_util.NowSec()
		if !time_util.IsSameDay(endTime-1, nowSec) {
			return
		}
	}
	// 0 表示活动结束结算
	if conf.SettlementHour == 0 {
		return
	}
	if conf.SettlementHour != uint32(hour) {
		s.LogError("%d != %d ", conf.SettlementHour, hour)
		return
	}

	// 延时几分钟后结算
	timer.SetTimeout(time.Duration(conf.SettlementMinutes)*time.Minute, func() {
		s.settlement()
	})
}

func (s *GameplayScoreGoalRank) settlement() {
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}

	data := s.getData()
	if data.IsSettlement {
		return
	}
	data.IsSettlement = true

	// 获取对应榜的邮件参数及结构
	var getContentAndMailId = func(_ uint32, rankIdx uint32) (uint16, proto.Message) {
		mailId := uint16(conf.MailId)
		args := &mailargs.RankArgs{
			Rank: rankIdx,
		}
		return mailId, args
	}
	s.ScoreRank.Settlement(func(rank uint32, rankInfo *pb3.RankInfo, awards jsondata.StdRewardVec) {
		mailId, content := getContentAndMailId(data.RankType, rank)
		mailmgr.SendMailToActor(rankInfo.PlayerId, &mailargs.SendMailSt{
			ConfId:  mailId,
			Rewards: awards,
			Content: content,
		})
	})
}

func (s *GameplayScoreGoalRank) NewDay() {
	s.reissueTodayAwards()
	s.getData().Day = s.GetOpenDay()
	conf, ok := jsondata.GetYYGameplayScoreGoalRankConf(s.ConfName, s.ConfIdx)
	if ok && conf.DailyRefresh {
		s.ResetData()
		s.OnInit()
	}
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.resetCurrentRankJoinFlag(player)
		s.sendTodayScore(player)
	})
	s.BroadcastRank()
}

func gameplayScoreGoalRankUpdateHandler(args ...interface{}) {
	if len(args) < 4 {
		return
	}
	rankType := args[0].(ranktype.GameplayScoreGoalRankType)
	playerId := args[1].(uint64)
	score := args[2].(int64)
	isAdd := args[3].(bool)
	allYY := yymgr.GetAllYY(yydefine.YYGameplayScoreGoalRank)
	for _, iYunYing := range allYY {
		iYunYing.(*GameplayScoreGoalRank).updateRank(rankType, playerId, score, isAdd)
	}
}

func gameplayScoreGoalRankOnSeHourArrive(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	allYY := yymgr.GetAllYY(yydefine.YYGameplayScoreGoalRank)
	for _, iYunYing := range allYY {
		iYunYing.(*GameplayScoreGoalRank).onHourArrive(hour)
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYGameplayScoreGoalRank, func() iface.IYunYing {
		return &GameplayScoreGoalRank{}
	})
	event.RegSysEvent(custom_id.SeHourArrive, gameplayScoreGoalRankOnSeHourArrive)
	event.RegSysEvent(custom_id.SeGameplayScoreGoalRankUpdate, gameplayScoreGoalRankUpdateHandler)
	net.RegisterGlobalYYSysProto(61, 21, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*GameplayScoreGoalRank).c2sRankData
	})
	net.RegisterGlobalYYSysProto(61, 22, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*GameplayScoreGoalRank).c2sRecReachAwards
	})

	gmevent.Register("addGameplayScoreGoalRankUpdateHandler", func(player iface.IPlayer, args ...string) bool {
		rankType := utils.AtoUint32(args[0])
		score := utils.AToU64(args[1])
		manager.UpdateGameplayScoreGoalRank(uint16(rankType), player, int64(score), false)
		return true
	}, 1)
}
