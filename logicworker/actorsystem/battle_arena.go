/**
 * @Author: yzh
 * @Date:
 * @Desc: 太虚论剑(竞技场)
 * @Modify：
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math/rand"
	"time"

	"github.com/gzjjyz/logger"
)

type BattleArenaSys struct {
	Base
	curOpponents     []*pb3.PlayerDataBase
	rankBeforeSettle uint32
	setInterval      bool
}

func (s *BattleArenaSys) OnOpen() {
	s.setRefreshOpponentInterval()
	s.RandomOpponents()
	s.sendState()
}

func (s *BattleArenaSys) setRefreshOpponentInterval() {
	if !s.setInterval {
		conf := jsondata.GetBattleArenaConf()
		if conf == nil {
			s.LogWarn("not found conf")
			return
		}
		if conf.RefreshOpponentIntervalSec > 0 {
			s.owner.SetInterval(time.Duration(conf.RefreshOpponentIntervalSec)*time.Second, func() {
				s.RandomOpponents()
				s.sendState()
			}, time_util.WithOutChaseFrame())
		}
		s.setInterval = true
	}
}

func (s *BattleArenaSys) OnLogin() {
	if !s.IsOpen() {
		return
	}
	s.setRefreshOpponentInterval()
	if s.curOpponents == nil {
		s.RandomOpponents()
	}

	s.sendState()
}

func (s *BattleArenaSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.setRefreshOpponentInterval()
	if s.curOpponents == nil {
		s.RandomOpponents()
	}

	s.sendState()
}

const showBattleTopRank = 3

func (s *BattleArenaSys) sendState() {
	rankList := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena)
	var myRank uint32
	mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(s.owner.GetId())
	if ok {
		myRank = rankList.GetRankById(mirrorRobot.ActorId)
	}
	rsp := &pb3.S2C_133_4{
		RankNo:           myRank,
		BattleArenaState: s.state(),
	}

	if rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena); nil != rank {
		rankLine := rank.GetList(1, showBattleTopRank)
		rsp.RankData = make([]*pb3.RankInfo, 0, showBattleTopRank)
		for idx, line := range rankLine { // 获取最新的点赞数量
			info := &pb3.RankInfo{}
			manager.SetPlayerRankInfo(line.GetId(), info, gshare.RankTypeBattleArena)
			info.Rank = uint32(idx + 1)
			info.Key = line.GetId()
			rsp.RankData = append(rsp.RankData, info)
		}
	}
	s.SendProto3(133, 4, rsp)

	var ranks []uint32
	for _, opponent := range s.curOpponents {
		var opponentId = opponent.Id
		mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(opponentId)
		if ok {
			opponentId = mirrorRobot.ActorId
		}
		rankById := rankList.GetRankById(opponentId)
		ranks = append(ranks, rankById)
	}
	s.SendProto3(133, 1, &pb3.S2C_133_1{
		Players:     s.curOpponents,
		PlayerRanks: ranks,
	})
}

func (s *BattleArenaSys) state() *pb3.BattleArenaState {
	state := s.GetBinaryData().BattleArenaState
	var (
		highestRank          uint32
		challengedActorIds   []uint64
		recvFirstAwardsRanks []uint32
	)
	if state != nil && !time_util.IsSameDay(state.LastOpenSysAt, time_util.NowSec()) {
		highestRank = state.HighestRank
		challengedActorIds = state.ChallengedActorIds
		recvFirstAwardsRanks = state.RecvFirstAwardsRanks
		state = nil
	}
	if state == nil {
		state = &pb3.BattleArenaState{
			LastOpenSysAt:        time_util.NowSec(),
			HighestRank:          highestRank,
			ChallengedActorIds:   challengedActorIds,
			RecvFirstAwardsRanks: recvFirstAwardsRanks,
		}
		s.GetBinaryData().BattleArenaState = state
	}
	return state
}

// RandomOpponents 随机对手
func (s *BattleArenaSys) RandomOpponents() {
	state := s.state()

	challengedActorIdSet := make(map[uint64]struct{})
	for _, actorId := range state.ChallengedActorIds {
		challengedActorIdSet[actorId] = struct{}{}
	}

	rankList := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena)

	var rank uint32
	mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(s.owner.GetId())
	if ok {
		rank = rankList.GetRankById(mirrorRobot.ActorId)
	}

	conf := jsondata.GetBattleArenaConf()
	if conf == nil || len(conf.RandomOpponentConfs) == 0 {
		s.LogWarn("not found conf")
		return
	}

	randomOpponentConf := s.getMatchedOpponentConf(rank)
	var frontRanks, backRanks []*pb3.OneRankItem

	if randomOpponentConf != nil {
		if randomOpponentConf.OpponentRandomRankUp > 0 && rank > 0 {
			start := int(rank - randomOpponentConf.OpponentRandomRankUp)
			start = utils.MaxInt(1, start)
			frontRanks = rankList.GetList(start, int(rank-1))
		}
		backRanks = rankList.GetList(int(rank+1), int(rank+randomOpponentConf.OpponentRandomRankDown))
	} else {
		randomOpponentConf = conf.RandomOpponentConfs[len(conf.RandomOpponentConfs)-1]
		frontRanks = rankList.GetList(int(randomOpponentConf.StartRank-randomOpponentConf.OpponentRandomRankUp), int(randomOpponentConf.StartRank-1))
		backRanks = rankList.GetList(int(randomOpponentConf.StartRank+1), int(randomOpponentConf.StartRank+randomOpponentConf.OpponentRandomRankDown))
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(frontRanks), func(i, j int) {
		frontRanks[i], frontRanks[j] = frontRanks[j], frontRanks[i]
	})
	r.Shuffle(len(backRanks), func(i, j int) {
		backRanks[i], backRanks[j] = backRanks[j], backRanks[i]
	})

	opponentActorIdSet := map[uint64]struct{}{}
	var opponentActorIds []uint64

	frontRankSize := s.calcFrontOpponentCount(len(backRanks))
	opponentActorIds = append(opponentActorIds, s.selectOpponents(frontRanks, challengedActorIdSet, opponentActorIdSet, frontRankSize)...)
	opponentActorIds = append(opponentActorIds, s.selectOpponents(backRanks, challengedActorIdSet, opponentActorIdSet, 10-len(opponentActorIds))...)

	if len(opponentActorIds) > 0 {
		s.curOpponents = nil
		for _, actorId := range opponentActorIds {
			data, ok := robotmgr.BattleArenaRobotMgrInstance.GetRobotData(actorId)
			if !ok {
				s.GetOwner().LogWarn("robot(%d) data not found", actorId)
				continue
			}
			s.curOpponents = append(s.curOpponents, data.BaseData)
		}
	}
}

func (s *BattleArenaSys) getMatchedOpponentConf(rank uint32) *jsondata.RandomOpponentConf {
	conf := jsondata.GetBattleArenaConf()
	for _, conf := range conf.RandomOpponentConfs {
		if rank >= conf.StartRank && rank <= conf.EndRank {
			return conf
		}
	}
	return nil
}

func (s *BattleArenaSys) calcFrontOpponentCount(backRankSize int) int {
	switch {
	case backRankSize == 0:
		return 10
	case backRankSize < 5:
		return 10 - backRankSize
	default:
		return 5
	}
}

func (s *BattleArenaSys) selectOpponents(ranks []*pb3.OneRankItem, challengedSet, selectedSet map[uint64]struct{}, count int) []uint64 {
	var result []uint64
	// 优先选出还未挑战过的
	for _, rankInfo := range ranks {
		id := rankInfo.Id
		if _, challenged := challengedSet[id]; challenged {
			continue
		}
		if _, selected := selectedSet[id]; selected {
			continue
		}
		result = append(result, id)
		selectedSet[id] = struct{}{}
		if len(result) >= count {
			return result
		}
	}

	for _, rankInfo := range ranks {
		id := rankInfo.Id
		if _, selected := selectedSet[id]; selected {
			continue
		}
		result = append(result, id)
		selectedSet[id] = struct{}{}
		if len(result) >= count {
			break
		}
	}
	return result
}

// 挑战
func (s *BattleArenaSys) challenge(opponentActorId uint64, isBuyNewChallengeTicket bool) {
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		s.LogWarn("not found conf")
		return
	}

	var legalOpponent bool
	for _, opponent := range s.curOpponents {
		if opponent.GetId() != opponentActorId {
			continue
		}
		legalOpponent = true
		break
	}

	if !legalOpponent {
		s.LogWarn("not legal opponent")
		return
	}

	if s.GetOwner().InDartCar() {
		s.GetOwner().SendTipMsg(tipmsgid.Tpindartcar)
		return
	}

	state := s.state()
	if state.ChallengeCnt >= conf.DailyChallengeCnt {
		if !isBuyNewChallengeTicket {
			s.LogWarn("actor's challenge over max times")
			return
		}
		if state.BuyChallengeCnt >= uint32(len(conf.ExtChallenge)) {
			s.LogWarn("actor's challenge over max times , not cann't buy cnt")
			return
		}
		if !s.owner.ConsumeByConf(conf.ExtChallenge[state.BuyChallengeCnt].Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeChallengeBattleArena}) {
			s.LogWarn("actor's challenge over max times, consume fail")
			return
		}
		state.BuyChallengeCnt++
	} else {
		state.ChallengeCnt++
	}

	opponentCrateData, ok := robotmgr.BattleArenaRobotMgrInstance.GetRobotCreateData(opponentActorId)
	if !ok {
		s.GetOwner().LogWarn("not found opponent(actor id:%d) create data", opponentActorId)
		return
	}

	// 战力
	opponentFightVal, ok := robotmgr.BattleArenaRobotMgrInstance.GetRobotFightVal(opponentActorId)
	if !ok {
		err := neterror.InternalError("opponentActorId %d opponentFightVal not found", opponentActorId)
		s.GetOwner().LogWarn("err:%v", err)
		return
	}

	// 处理秒杀
	fightValue := uint64(s.owner.GetExtraAttr(attrdef.FightValue))
	if state.HighestRank > 0 && fightValue > opponentFightVal && fightValue-opponentFightVal >= conf.EasyKillFightValGap {
		var found bool
		for _, actorId := range state.SecKilledActorIds {
			if actorId != opponentActorId {
				continue
			}
			found = true
			break
		}
		if !found {
			state.SecKilledActorIds = append(state.SecKilledActorIds, opponentActorId)
		}
	}

	var found bool
	for _, actorId := range state.ChallengedActorIds {
		if actorId != opponentActorId {
			continue
		}
		found = true
	}
	if !found {
		state.ChallengedActorIds = append(state.ChallengedActorIds, opponentActorId)
	}

	var triggerQuest = func() {
		s.owner.TriggerQuestEvent(custom_id.QttChallengeBattleArenaCnt, 0, 1)
		s.owner.TriggerQuestEvent(custom_id.QttChallengeBattleArenaTimes, 0, 1)

		s.owner.TriggerQuestEvent(custom_id.QttTodayChallengeBattleArenaCnt, 0, int64(state.ChallengeCnt))
		s.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiBattleArena, 1)
		s.owner.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
			Cond:  custom_id.FaBaoTalentCondBattleArena,
			Count: 1,
		})
		s.owner.TriggerEvent(custom_id.AeFashionTalentEvent, &custom_id.FashionTalentEvent{
			Cond:  custom_id.FashionSetBattleAreaEvent,
			Count: 1,
		})
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogArenaChallenge, &pb3.LogPlayerCounter{
		NumArgs: opponentActorId,
	})

	for _, actorId := range state.SecKilledActorIds {
		if actorId != opponentActorId {
			continue
		}
		s.GetOwner().LogWarn("sec killed %d", actorId)
		s.SendProto3(133, 3, &pb3.S2C_133_3{})
		s.settleChallenge(opponentActorId, true, 1, false)
		triggerQuest()
		return
	}

	var myRank uint32
	mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(s.owner.GetId())
	if ok {
		myRank = manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena).GetRankById(mirrorRobot.ActorId)
		s.rankBeforeSettle = myRank
	}

	err := s.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterBattleArena, &pb3.EnterBattleArenaReq{
		PfId:               engine.GetPfId(),
		SrvId:              engine.GetServerId(),
		OpponentId:         opponentActorId,
		OpponentCreateData: opponentCrateData,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
	triggerQuest()
}

// 设置挑战
func (s *BattleArenaSys) settleChallenge(opponentActorId uint64, isWin bool, challengeCostSec uint32, secKilled bool) {
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		s.GetOwner().LogWarn("not found battle arena conf")
		return
	}
	s.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionBattleArena)

	settle := &pb3.FbSettlement{
		FbId:     conf.FbId,
		PassTime: challengeCostSec,
		Ret:      custom_id.FbSettleResultLose,
		IsQuick:  !secKilled,
	}

	var err error
	if isWin {
		err = s.onChallengeSuccess(opponentActorId, settle)
	} else {
		err = s.onChallengeFailed(settle)
	}
	if err != nil {
		s.LogError("err:%v", err)
		return
	}

	// 推送结果
	s.GetOwner().SendProto3(17, 254, &pb3.S2C_17_254{Settle: settle})

	// 排名变动
	s.afterSettleChallenge(opponentActorId)

	// 刷新排行榜对手
	if isWin {
		s.RandomOpponents()
	}

	// 推送下竞技场状态
	s.sendState()
}

// 挑战成功
func (s *BattleArenaSys) onChallengeSuccess(opponentActorId uint64, settle *pb3.FbSettlement) (err error) {
	owner := s.GetOwner()
	actorId := owner.GetId()
	state := s.state()
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		return neterror.ConfNotFoundError("not found battle arena conf")
	}

	// 排行榜
	rankList := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena)

	// 对手排名
	opponentRank := rankList.GetRankById(opponentActorId)

	// 个人排名
	var myRank uint32
	mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(actorId)
	switch {
	case ok: // 能找到对应挑战者信息(这里重新刷新下排行榜镜像玩家数据)
		robotmgr.BattleArenaRobotMgrInstance.RefreshRealActorMirrorRobot(mirrorRobot.RobotConfigId, mirrorRobot.ActorId)
		myRank = rankList.GetRankById(mirrorRobot.ActorId)
	default: // 打赢, 但是找不到 需要加入玩家的镜像
		if err = robotmgr.BattleArenaRobotMgrInstance.AddRealActorMirrorRobot(actorId); err != nil {
			s.GetOwner().LogError(err.Error())
			return
		}
		mirrorRobot, ok = robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(actorId)
		if !ok {
			err = neterror.InternalError("mirrorRobot %d is nil", actorId)
			s.GetOwner().LogWarn("err:%v", err)
			return
		}
	}

	// 提交事件
	s.owner.TriggerQuestEvent(custom_id.QttWinBattleArenaCnt, 0, 1)
	s.owner.TriggerEvent(custom_id.AeBattleAreaWin, opponentActorId)

	// 结算
	settle.Ret = custom_id.FbSettleResultWin
	stdRewardVec := engine.FilterRewardByPlayer(owner, conf.ChallengeAwards)
	settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(stdRewardVec)

	// 奖励
	engine.GiveRewards(s.owner, stdRewardVec, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogChallengeBattleArea,
	})

	// 排名变动
	if opponentRank == 0 {
		s.LogInfo("onChallengeSuccess target %d rank is zero", opponentActorId)
		return nil
	}

	if myRank != 0 && myRank < opponentRank {
		return
	}

	// 得到新排名
	newMyRank, newOpponentRank := opponentRank, myRank

	// 历史最高排名
	if state.HighestRank == 0 || state.HighestRank > newMyRank {
		var origHighestRank = state.HighestRank
		state.HighestRank = newMyRank
		for _, rankAwardConf := range conf.RankAwards {
			var found, alreadyRevAwards bool
			switch len(rankAwardConf.Ranks) {
			case 1:
				found = rankAwardConf.Ranks[0] == state.HighestRank
			case 2:
				found = rankAwardConf.Ranks[0] <= state.HighestRank && rankAwardConf.Ranks[1] >= state.HighestRank
				alreadyRevAwards = rankAwardConf.Ranks[0] <= origHighestRank && rankAwardConf.Ranks[1] >= origHighestRank
			}

			if !found {
				continue
			}

			if alreadyRevAwards {
				break
			}

			rewardVec := rankAwardConf.Awards
			if len(rewardVec) > 0 {
				rewardVec = engine.FilterRewardByPlayer(s.owner, rewardVec)
				mailmgr.SendMailToActor(s.owner.GetId(), &mailargs.SendMailSt{
					ConfId:  rankAwardConf.FirstAwardMailId,
					Rewards: rewardVec,
					Content: &mailargs.RankArgs{Rank: state.HighestRank},
				})
			}
			break
		}
	}

	// 更新排行榜
	opponentRankScore := rankList.Score(newOpponentRank)
	myRankScore := rankList.Score(newMyRank)
	owner.LogDebug("new myRank: %d, myRankScore: %d", newMyRank, myRankScore)
	owner.LogDebug("new opponentRank: %d, opponentRankScore: %d", newOpponentRank, opponentRankScore)

	rankList.Update(opponentActorId, opponentRankScore)
	owner.LogDebug("update opponentActorId: %d, opponentRankScore: %d", opponentActorId, opponentRankScore)
	rankList.Update(mirrorRobot.ActorId, myRankScore)
	owner.LogDebug("update myActorId: %d, myRankScore: %d", mirrorRobot.ActorId, myRankScore)

	return
}

// 挑战失败 失败没有排名变动
func (s *BattleArenaSys) onChallengeFailed(settle *pb3.FbSettlement) (err error) {
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		return neterror.ConfNotFoundError("not found battle arena conf")
	}
	stdRewardVec := engine.FilterRewardByPlayer(s.owner, conf.ChallengeFailAwards)
	settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(stdRewardVec)
	// 奖励
	engine.GiveRewards(s.owner, stdRewardVec, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogChallengeBattleArea,
	})
	return
}

func (s *BattleArenaSys) afterSettleChallenge(opponentActorId uint64) {
	owner := s.GetOwner()
	actorId := owner.GetId()
	rankList := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena)
	mirrorRobot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(actorId)
	if !ok {
		// 初始化真实玩家
		if err := robotmgr.BattleArenaRobotMgrInstance.AddRealActorMirrorRobot(actorId); err != nil {
			s.GetOwner().LogError(err.Error())
			return
		}
		mirrorRobot, ok = robotmgr.BattleArenaRobotMgrInstance.GetRealActorMirrorRobotByConfigId(actorId)
		if !ok {
			err := neterror.InternalError("mirrorRobot %d is nil", actorId)
			s.GetOwner().LogWarn("err:%v", err)
			return
		}
	}

	myRank := rankList.GetRankById(mirrorRobot.ActorId)
	// 打之前是有排名的 打之后没排名 那么就是出bug了
	if myRank == 0 && s.rankBeforeSettle != 0 {
		s.LogError("rankBeforeSettle: %d, myRank: %d", s.rankBeforeSettle, myRank)
		return
	}

	// 排名一样 表示打输了 排名没变化
	if myRank == s.rankBeforeSettle {
		return
	}
	opponentRank := rankList.GetRankById(opponentActorId)

	// 表示挑战者的排名还在自己的前面 有问题
	if opponentRank < myRank {
		s.LogError("opponentRank: %d, myRank: %d", opponentRank, myRank)
		return
	}

	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		s.LogWarn("not found battle arena conf")
		return
	}

	var raiseRanks uint32
	if opponentRank == 0 {
		raiseRanks = myRank
	} else {
		raiseRanks = opponentRank - myRank
	}

	// 刷新对手
	s.RandomOpponents()

	// 邮件告知对手排名变动
	var sendEmailToOpponent = func(opponentActorId uint64, newOpponentRank uint32, owner iface.IPlayer) {
		opponent, ok := robotmgr.BattleArenaRobotMgrInstance.GetRobotData(opponentActorId)
		if !ok {
			return
		}

		// 竞技场 不是真人镜像 不用告知对手排名变动
		if opponent.CreateData.RobotType != custom_id.ActorRobotTypeRealActorMirror {
			return
		}

		opponentSource := manager.GetPlayerPtrById(opponent.CreateData.RobotConfigId)
		if opponentSource != nil {
			opponentSource.GetSysObj(sysdef.SiBattleArena).(*BattleArenaSys).RandomOpponents()
		}

		// 默认跌出排行榜
		var (
			mailId        = conf.RankDownOutMailId
			robotConfigId = opponent.CreateData.RobotConfigId
		)

		// 推送邮件
		if newOpponentRank > 0 {
			s.GetOwner().LogInfo(" newOpponentRank: %d,Rank Down", newOpponentRank)
			mailId = conf.RankDownMailId
		}

		s.GetOwner().LogInfo("send mail to robot config id: %d", robotConfigId)
		mailmgr.SendMailToActor(robotConfigId, &mailargs.SendMailSt{
			ConfId: mailId,
			Content: &mailargs.RankDownArgs{
				ReplacerName: owner.GetName(),
				NewRank:      newOpponentRank,
			},
		})
	}
	sendEmailToOpponent(opponentActorId, opponentRank, s.GetOwner())

	s.owner.SendProto3(133, 5, &pb3.S2C_133_5{
		RaiseRanks:   raiseRanks,
		CurRank:      myRank,
		OpponentRank: opponentRank,
	})
}

// MopUp 扫荡
func (s *BattleArenaSys) MopUp() {
	// 开服第一天不允许扫荡
	if gshare.GetOpenServerDay() <= 1 {
		return
	}

	state := s.state()
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		s.LogWarn("not found battle arena conf")
		return
	}
	// Not enough challenges are available
	if conf.DailyChallengeCnt <= state.ChallengeCnt {
		s.GetOwner().LogWarn("daily challenge cnt is %d , challenge cnt is %d , not allow mop up", conf.DailyChallengeCnt, state.ChallengeCnt)
		return
	}

	ableChallengeCnt := conf.DailyChallengeCnt - state.ChallengeCnt
	state.ChallengeCnt = conf.DailyChallengeCnt

	settle := &pb3.FbSettlement{
		FbId:    conf.FbId,
		Ret:     2,
		IsQuick: true,
	}

	stdRewardVec := engine.FilterRewardByPlayer(s.owner, conf.ChallengeAwards)
	awards := jsondata.StdRewardMulti(stdRewardVec, int64(ableChallengeCnt))
	if len(awards) > 0 {
		engine.GiveRewards(s.owner, awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogChallengeBattleArea,
		})
	}
	settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(awards)
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogArenaModUp, &pb3.LogPlayerCounter{})

	// 扫荡也算挑战
	s.owner.TriggerQuestEvent(custom_id.QttChallengeBattleArenaCnt, 0, int64(ableChallengeCnt))
	s.owner.TriggerQuestEventRangeTimes(custom_id.QttChallengeBattleArenaTimes, 0, int64(ableChallengeCnt))
	s.owner.TriggerQuestEvent(custom_id.QttTodayChallengeBattleArenaCnt, 0, int64(state.ChallengeCnt))
	s.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiBattleArena, int(ableChallengeCnt))
	for i := uint32(0); i < ableChallengeCnt; i++ {
		s.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionBattleArena)
	}
	s.owner.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
		Cond:  custom_id.FaBaoTalentCondBattleArena,
		Count: ableChallengeCnt,
	})
	s.owner.TriggerEvent(custom_id.AeFashionTalentEvent, &custom_id.FashionTalentEvent{
		Cond:  custom_id.FashionSetBattleAreaEvent,
		Count: ableChallengeCnt,
	})

	s.owner.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settle})

	// Repost the latest challenge status
	s.sendState()
}

func c2sRefreshOpponents(sys iface.ISystem) func(*base.Message) error {
	return func(_ *base.Message) error {
		sysObj := sys.(*BattleArenaSys)
		state := sysObj.state()
		conf := jsondata.GetBattleArenaConf()
		if conf == nil {
			return neterror.ConfNotFoundError("not found battle arena conf")
		}
		state.RefreshChallengerCnt++
		sysObj.RandomOpponents()
		sysObj.sendState()
		return nil
	}
}

func c2sMopUpBattleArena(sys iface.ISystem) func(*base.Message) error {
	return func(_ *base.Message) error {
		sys.(*BattleArenaSys).MopUp()
		return nil
	}
}

func c2sChallengeBattleArea(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		sysObj := sys.(*BattleArenaSys)
		var req pb3.C2S_133_2
		if err := msg.UnpackagePbmsg(&req); err != nil {
			return neterror.Wrap(err)
		}

		sysObj.challenge(req.OpponentActorId, req.IsBuy)
		return nil
	}
}

func c2sSendBattleArenaOpponent(sys iface.ISystem) func(*base.Message) error {
	return func(_ *base.Message) error {
		sys.(*BattleArenaSys).sendState()
		return nil
	}
}

func GmChallengeBattleArea(actor iface.IPlayer, _ ...string) bool {
	sys := actor.GetSysObj(sysdef.SiBattleArena).(*BattleArenaSys)

	sys.challenge(sys.curOpponents[0].GetId(), false)
	return true
}

func SettleBattleArena(player iface.IPlayer, buf []byte) {
	var req pb3.SettleBattleAreaReq
	if err := pb3.Unmarshal(buf, &req); err != nil {
		return
	}
	player.GetSysObj(sysdef.SiBattleArena).(*BattleArenaSys).settleChallenge(req.Opponent, req.IsWin, req.ChallengeCostSec, true)
}

func dailySettleBattleArenaRankAwards() {
	conf := jsondata.GetBattleArenaConf()
	if conf == nil {
		logger.LogWarn("not found battle arena conf")
		return
	}
	rankList := manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena)
	rankCnt := uint32(rankList.GetRankCount())
	// s.GetOwner().LogDebug("rankList size:%d", rankCnt)
	for rank := uint32(1); rank < rankCnt; rank++ {
		robotActorId := rankList.GetIdByRank(uint32(rank))
		if robotActorId == 0 {
			continue
		}

		robot, ok := robotmgr.BattleArenaRobotMgrInstance.GetRobotCreateData(robotActorId)
		if !ok {
			logger.LogWarn("GetRobotCreateData failed , robotActorId is %d", robotActorId)
			continue
		}

		if robot.RobotType != custom_id.ActorRobotTypeRealActorMirror {
			// logger.LogInfo("robot[%d] is robot , type is %v , val is %d", robotActorId, robot, robot.RobotType)
			continue
		}

		realActorId := robot.RobotConfigId
		logger.LogInfo("realActor[%d] robotActorId is %d ,val is %v", robotActorId, robot.RobotConfigId, robot)

		if !manager.IsActorActive(realActorId) {
			logger.LogWarn("realActor[%d] is unActive", realActorId)
			continue
		}

		for idx, rankAwardConf := range conf.RankAwards {
			var found bool
			switch len(rankAwardConf.Ranks) {
			case 1:
				found = rankAwardConf.Ranks[0] == rank
			case 2:
				found = rankAwardConf.Ranks[0] <= rank && rankAwardConf.Ranks[1] >= rank
			}

			if !found {
				logger.LogWarn("not found rank awards,idx is %d, rankAwardConf is %v", idx, rankAwardConf)
				continue
			}
			dailyAwards := rankAwardConf.DailyAwards
			if len(dailyAwards) > 0 {
				playerDataBase, ok := manager.GetData(realActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
				if ok {
					dailyAwards = jsondata.FilterRewardByOption(dailyAwards,
						jsondata.WithFilterRewardOptionByJob(playerDataBase.Job),
						jsondata.WithFilterRewardOptionBySex(playerDataBase.Sex),
						jsondata.WithFilterRewardOptionByLvRange(playerDataBase.Lv),
					)
				}
				mailmgr.SendMailToActor(realActorId, &mailargs.SendMailSt{
					ConfId:  rankAwardConf.DailyAwardMailId,
					Rewards: dailyAwards,
					Content: &mailargs.RankArgs{Rank: rank},
				})
				break
			}
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiBattleArena, func() iface.ISystem {
		return &BattleArenaSys{}
	})

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		dailySettleBattleArenaRankAwards()
	})

	event.RegActorEvent(custom_id.AeNewHour, func(player iface.IPlayer, args ...interface{}) {
		sys := player.GetSysObj(sysdef.SiBattleArena)
		if sys != nil && sys.IsOpen() {
			arenaSys, ok := sys.(*BattleArenaSys)
			if !ok {
				return
			}
			arenaSys.state().SecKilledActorIds = nil
			arenaSys.RandomOpponents()
			arenaSys.sendState()
		}
	})

	engine.RegisterActorCallFunc(playerfuncid.SettleBattleAreaChallenge, SettleBattleArena)
	net.RegisterSysProtoV2(133, 1, sysdef.SiBattleArena, c2sSendBattleArenaOpponent)
	net.RegisterSysProtoV2(133, 2, sysdef.SiBattleArena, c2sChallengeBattleArea)
	net.RegisterSysProtoV2(133, 3, sysdef.SiBattleArena, c2sRefreshOpponents)
	net.RegisterSysProtoV2(133, 6, sysdef.SiBattleArena, c2sMopUpBattleArena)
	//net.RegisterSysProtoV2(133, 4, gshare.SiBattleArena, c2sRecvBattleArenaRankFirstAwards)
	//net.RegisterSysProtoV2(133, 5, gshare.SiBattleArena, c2sRecvBattleArenaRankDailyAwards)

	gmevent.Register("enter_battle_arena", GmChallengeBattleArea, 1)
	gmevent.Register("refresh_battle_arena_times", func(player iface.IPlayer, args ...string) bool {
		sysObj := player.GetSysObj(sysdef.SiBattleArena).(*BattleArenaSys)
		sysObj.state().LastOpenSysAt = 0
		return true
	}, 1)
}
