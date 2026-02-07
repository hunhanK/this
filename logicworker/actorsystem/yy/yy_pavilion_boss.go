/**
 * @Author: lzp
 * @Date: 2024/9/24
 * @Desc:
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysfuncid"
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
	"time"
)

type YYPavilionBoss struct {
	YYBase
	BossRank  map[uint32]*base.Rank // boss排行榜 k:bossId v:rank
	BossTimer map[uint32]*time_util.Timer
}

const (
	PoolGroup1 = 1 // 品质高
	PoolGroup2 = 2
	PoolGroup3 = 3
)

const (
	ValidTimeSec = uint32(60 * 60 * 24 * 365 * 5) // 5年时间
	DaySec       = uint32(60 * 60 * 24)
)

func (s *YYPavilionBoss) OnInit() {
	conf := jsondata.GetYYPavBossConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	if !s.IsOpen() {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FPavBossActOpen, &pb3.CommonSt{
		U32Param: s.ConfIdx,
		StrParam: s.ConfName,
	})
	if err != nil {
		s.LogError("err: %v", err)
	}

	s.initPavBoss(conf)
	s.initRank(conf)
	s.initTimer(conf)
}

func (s *YYPavilionBoss) OnEnd() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FPavBossActClose, nil)
	if err != nil {
		s.LogError("err: %v", err)
	}
	data := s.getData()
	data.BossInfo = nil
	data.ActorInfo = nil
}

func (s *YYPavilionBoss) ServerStopSaveData() {
	data := s.getData()
	for bId, bRank := range s.BossRank {
		bData := data.BossInfo[bId]
		if bData == nil {
			continue
		}
		var bkItems []*pb3.OneRankItem
		bRank.ChunkAll(func(item *pb3.OneRankItem) bool {
			bkItems = append(bkItems, &pb3.OneRankItem{
				Id:    item.Id,
				Score: item.Score,
			})
			return false
		})
		bData.Items = bkItems
	}
}

func (s *YYPavilionBoss) PlayerLogin(player iface.IPlayer) {
	s.s2cInfo(player)
}

func (s *YYPavilionBoss) PlayerReconnect(player iface.IPlayer) {
	s.s2cInfo(player)
}

func (s *YYPavilionBoss) s2cInfo(player iface.IPlayer) {
	pbMsg := &pb3.S2C_69_200{}
	pbMsg.ActId = s.Id

	data := s.getData()
	pbMsg.PavBoss = data.BossInfo

	playerId := player.GetId()
	actorInfo := data.ActorInfo[playerId]
	if actorInfo != nil {
		pbMsg.KillInfo = actorInfo.KillInfo
		pbMsg.DrawInfo = actorInfo.DrawInfo
	}

	player.SendProto3(69, 200, pbMsg)
}

func (s *YYPavilionBoss) getData() *pb3.YYPavilionBoss {
	gVar := gshare.GetStaticVar()
	if gVar.YyDatas == nil {
		gVar.YyDatas = &pb3.YYDatas{}
	}
	if gVar.YyDatas.PavBossData == nil {
		gVar.YyDatas.PavBossData = make(map[uint32]*pb3.YYPavilionBoss)
	}
	if gVar.YyDatas.PavBossData[s.Id] == nil {
		gVar.YyDatas.PavBossData[s.Id] = &pb3.YYPavilionBoss{}
	}
	return gVar.YyDatas.PavBossData[s.Id]
}

func (s *YYPavilionBoss) initPavBoss(conf *jsondata.YYPavBossConf) {
	// 初始化每个boss 奖励池
	genItemPool := func(bConf *jsondata.PavBoss) map[uint32]*pb3.PavBossItem {
		iPool := make(map[uint32]*pb3.PavBossItem)
		for _, pConf := range bConf.RewardsPool {
			for _, rConf := range pConf.Items {
				iPool[rConf.Id] = &pb3.PavBossItem{
					Id:      rConf.Id,
					ItemId:  rConf.ItemId,
					Count:   rConf.Count,
					Weight:  rConf.Weight,
					GroupId: pConf.Group,
				}
			}
		}
		return iPool
	}
	genGroupPool := func(bConf *jsondata.PavBoss) map[uint32]*pb3.PavBossGroup {
		gPool := make(map[uint32]*pb3.PavBossGroup)
		for _, pConf := range bConf.RewardsPool {
			gPool[pConf.Group] = &pb3.PavBossGroup{
				Group:    pConf.Group,
				Weight:   pConf.Weight,
				MaxCount: pConf.MaxCount,
			}
		}
		return gPool
	}

	data := s.getData()
	if data.BossInfo == nil {
		data.BossInfo = make(map[uint32]*pb3.PavBossInfo)
		for _, bConf := range conf.PavBoss {
			data.BossInfo[bConf.MonsterId] = &pb3.PavBossInfo{}
			data.BossInfo[bConf.MonsterId].MonId = bConf.MonsterId
			data.BossInfo[bConf.MonsterId].GroupPool = genGroupPool(bConf)
			data.BossInfo[bConf.MonsterId].ItemPool = genItemPool(bConf)
		}
	}
}

func (s *YYPavilionBoss) initRank(conf *jsondata.YYPavBossConf) {
	s.BossRank = make(map[uint32]*base.Rank)
	for _, bConf := range conf.PavBoss {
		s.BossRank[bConf.MonsterId] = new(base.Rank)
		s.BossRank[bConf.MonsterId].Init(conf.RankCount)
	}
	data := s.getData()
	for bId, bData := range data.BossInfo {
		bRank := s.BossRank[bId]
		if bRank == nil {
			continue
		}
		for _, item := range bData.Items {
			bRank.Update(item.Id, item.Score)
		}
	}
}

func (s *YYPavilionBoss) initTimer(conf *jsondata.YYPavBossConf) {
	// 先暂停所有定时器
	if s.BossTimer != nil {
		for _, t := range s.BossTimer {
			t.Stop()
		}
	}
	s.BossTimer = make(map[uint32]*time_util.Timer)

	// 提醒当天要开启的boss
	for _, bConf := range conf.PavBoss {
		timeAt := s.transfer2TodayTimeAt(bConf.DayLimit, bConf.TimeLimit)
		now := time_util.NowSec()
		if !time_util.IsSameDay(now, timeAt) {
			continue
		}
		if timeAt <= time_util.NowSec() {
			continue
		}

		monId := bConf.MonsterId
		s.BossTimer[monId] = timer.SetTimeout(time.Duration(timeAt-now)*time.Second, func() {
			s.reminder(monId)
		})
	}
}

func (s *YYPavilionBoss) c2sChallenge(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_201
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	bConf := jsondata.GetYYPavBossBossConf(s.ConfName, s.ConfIdx, req.MonId)
	if bConf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	if !s.checkCanChallenge(req.MonId) {
		return neterror.ParamsInvalidError("time limit bossId=%d", req.MonId)
	}

	data := s.getData()
	bossInfo := data.BossInfo[req.MonId]
	if bossInfo == nil {
		return neterror.ParamsInvalidError("not found monId=%d", req.MonId)
	}

	actorInfo := data.ActorInfo[player.GetId()]
	if actorInfo != nil && actorInfo.KillInfo[req.MonId] {
		return neterror.ParamsInvalidError("boss has killed bossId=%d", req.MonId)
	}

	if bossInfo.DrawCount >= uint32(len(bConf.KillCount)) {
		return neterror.ParamsInvalidError("cannot challenge bossId=%d", req.MonId)
	}

	err := player.EnterFightSrv(base.LocalFightServer, fubendef.EnterPavBoss, &pb3.ChallengePavBoss{
		ConfName: s.ConfName,
		ConfIdx:  s.ConfIdx,
		MonId:    req.MonId,
		Times:    bossInfo.DrawCount,
	})
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	player.TriggerQuestEvent(custom_id.QttPavBossChallengeTimes, 0, 1)
	return nil
}

func (s *YYPavilionBoss) c2sDraw(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_202
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	monId := req.MonId
	bConf := jsondata.GetYYPavBossBossConf(s.ConfName, s.ConfIdx, monId)
	if bConf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data := s.getData()
	bossInfo := data.BossInfo[monId]
	if bossInfo == nil {
		return neterror.ParamsInvalidError("not found monId=%d", monId)
	}
	if bossInfo.DrawCount >= uint32(len(bConf.KillCount)) {
		return neterror.ParamsInvalidError("monId=%d draw pool is empty", monId)
	}

	actorInfo := data.ActorInfo[player.GetId()]
	if actorInfo == nil || !actorInfo.KillInfo[monId] {
		return neterror.ParamsInvalidError("cannot draw boss not kill bossId=%d", monId)
	}

	if actorInfo.DrawInfo == nil {
		actorInfo.DrawInfo = make(map[uint32]bool)
	}

	if actorInfo.DrawInfo[monId] {
		return neterror.ParamsInvalidError("has draw bossId=%d", monId)
	}

	actorInfo.DrawInfo[monId] = true
	bossInfo.DrawCount += 1

	rewards := s.draw(monId)
	if len(rewards) > 0 && !engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPavBossDrawRewards,
	}) {
		return neterror.InternalError("pavilion boss draw error")
	}

	s.addDrawRecord(player, rewards, monId)
	player.SendProto3(69, 202, &pb3.S2C_69_202{
		ActId:  s.Id,
		Awards: jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	player.SendProto3(69, 204, &pb3.S2C_69_204{
		ActId:    s.Id,
		DrawInfo: actorInfo.DrawInfo,
	})
	engine.Broadcast(chatdef.CIWorld, 0, 69, 204, &pb3.S2C_69_204{
		PavBoss: data.BossInfo,
	}, 0)

	return nil
}

func (s *YYPavilionBoss) c2sGetRank(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_203
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rank := s.BossRank[req.MonId]
	if rank == nil {
		return neterror.ParamsInvalidError("pavilion boss rank not exit bossId=%d", req.MonId)
	}

	pbMsg := &pb3.S2C_69_203{}
	pbMsg.ActId = s.Id
	pbMsg.MonId = req.MonId
	pbMsg.Rank = make([]*pb3.RankInfo, 0)

	rList := rank.GetList(1, rank.GetRankCount())
	rankIdx := uint32(0)
	for _, line := range rList {
		rankIdx++
		info := &pb3.RankInfo{
			Rank:  rankIdx,
			Key:   line.Id,
			Value: int64(s.getBaseTimeAt()) - line.Score,
		}
		manager.SetPlayerRankInfo(line.GetId(), info, 0)
		pbMsg.Rank = append(pbMsg.Rank, info)
	}

	player.SendProto3(69, 203, pbMsg)
	return nil
}

func (s *YYPavilionBoss) addDrawRecord(player iface.IPlayer, rewards jsondata.StdRewardVec, monId uint32) {
	items := make(map[uint32]uint32)
	for _, reward := range rewards {
		items[reward.Id] = uint32(reward.Count)
	}
	rd := &pb3.ItemsGetRecord{
		ActorId:   player.GetId(),
		ActorName: player.GetName(),
		Items:     items,
		TimeStamp: time_util.NowSec(),
	}
	data := s.getData()
	bData := data.BossInfo[monId]
	if bData == nil {
		return
	}
	if bData.DrawRecords == nil {
		bData.DrawRecords = make([]*pb3.ItemsGetRecord, 0)
	}
	bData.DrawRecords = append(bData.DrawRecords, rd)
	maxCount := 50
	conf := jsondata.GetYYPavBossConf(s.ConfName, s.ConfIdx)
	if conf != nil {
		maxCount = int(conf.RecordCount)
	}
	if len(bData.DrawRecords) > maxCount {
		bData.DrawRecords = bData.DrawRecords[1:]
	}
}

func (s *YYPavilionBoss) addKillInfo(actorId uint64, monId uint32) {
	data := s.getData()
	if data.ActorInfo == nil {
		data.ActorInfo = make(map[uint64]*pb3.PavBossActorInfo)
	}

	actorInfo := data.ActorInfo[actorId]
	if actorInfo == nil {
		data.ActorInfo[actorId] = &pb3.PavBossActorInfo{}
		actorInfo = data.ActorInfo[actorId]
	}

	if actorInfo.KillInfo == nil {
		actorInfo.KillInfo = make(map[uint32]bool)
	}
	actorInfo.KillInfo[monId] = true
}

func (s *YYPavilionBoss) draw(monId uint32) jsondata.StdRewardVec {
	data := s.getData()
	bossInfo := data.BossInfo[monId]
	times := bossInfo.DrawCount

	bConf := jsondata.GetYYPavBossBossConf(s.ConfName, s.ConfIdx, monId)
	if bConf == nil {
		return nil
	}

	kConf := bConf.KillCount[times]
	if kConf == nil {
		return nil
	}

	var curHit uint32
	var rewards []*jsondata.StdReward
	for i := 1; i <= int(kConf.DrawCount); i++ {
		gPool := bossInfo.GroupPool
		iPool := bossInfo.ItemPool
		gItem := s.selectGroupPool(kConf.HitCount, curHit, gPool)
		if gItem == nil {
			s.LogError("confHitCount:%d, curHitCount:%d, pool data: %+v", kConf.HitCount, curHit, gPool)
			continue
		}
		if gItem.Group == PoolGroup1 {
			curHit++
		}
		gPool[gItem.Group].CurCount += 1
		iItem := s.selectItemPool(gItem.Group, iPool)
		if iItem == nil {
			s.LogError("group: %d, iPool: %+v", gItem.Group, iPool)
			continue
		}
		iPool[iItem.Id].CurCount += 1
		rewards = append(rewards, &jsondata.StdReward{Id: iItem.ItemId, Count: 1})
	}
	return jsondata.MergeStdReward(rewards)
}

func (s *YYPavilionBoss) selectItemPool(group uint32, iPool map[uint32]*pb3.PavBossItem) *pb3.PavBossItem {
	pool := new(random.Pool)
	for _, item := range iPool {
		if item.GroupId != group {
			continue
		}
		if item.CurCount >= item.Count {
			continue
		}
		pool.AddItem(item, item.Weight)
	}
	if ret, ok := pool.RandomOne().(*pb3.PavBossItem); ok {
		return ret
	}
	return nil
}

func (s *YYPavilionBoss) selectGroupPool(hitCount, curHit uint32, gPool map[uint32]*pb3.PavBossGroup) *pb3.PavBossGroup {
	pool := new(random.Pool)
	for _, item := range gPool {
		// 玩家红库只能命中hitCount次数
		if item.Group == PoolGroup1 && curHit >= hitCount {
			continue
		}
		// 全服玩家每个库的命中次数
		if item.CurCount >= item.MaxCount {
			continue
		}
		pool.AddItem(item, item.Weight)
	}
	if ret, ok := pool.RandomOne().(*pb3.PavBossGroup); ok {
		return ret
	}
	return nil
}

// 更新排行榜
func (s *YYPavilionBoss) updateRank(playerId uint64, bossId uint32, score int64) {
	rank := s.BossRank[bossId]
	if rank == nil {
		return
	}
	rank.Update(playerId, score)
}

func (s *YYPavilionBoss) trySendMail(monId uint32, playerId uint64) {
	bConf := jsondata.GetYYPavBossBossConf(s.ConfName, s.ConfIdx, monId)
	if bConf == nil {
		return
	}
	monConf := jsondata.GetMonsterConf(monId)
	if monConf == nil {
		return
	}

	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	data := s.getData()
	bData := data.BossInfo[monId]
	if bData == nil || bData.IsSendSrvRewards {
		return
	}

	bData.IsSendSrvRewards = true
	argStr, _ := mailargs.MarshalMailArg(&mailargs.CommonMailArgs{Str1: player.GetName(), Str2: monConf.Name})
	mailmgr.AddSrvMailStr(bConf.FistKillMailId, argStr, bConf.FirstKillRewards)
}

func (s *YYPavilionBoss) getBaseTimeAt() uint32 {
	return gshare.GetOpenServerTime() + ValidTimeSec
}

func (s *YYPavilionBoss) checkCanChallenge(monId uint32) bool {
	bConf := jsondata.GetYYPavBossBossConf(s.ConfName, s.ConfIdx, monId)
	if bConf == nil {
		return false
	}

	timeAt := s.transfer2TodayTimeAt(bConf.DayLimit, bConf.TimeLimit)
	if timeAt == 0 || time_util.NowSec() < timeAt {
		return false
	}
	return true
}

// 提醒boss开启
func (s *YYPavilionBoss) reminder(monId uint32) {
	engine.Broadcast(chatdef.CIWorld, 0, 69, 206, &pb3.S2C_69_206{
		MonId: monId,
	}, 0)
	if t := s.BossTimer[monId]; t != nil {
		t.Stop()
		delete(s.BossTimer, monId)
	}
}

func (s *YYPavilionBoss) transfer2TodayTimeAt(timeDay uint32, timeHour string) uint32 {
	t, err := time.Parse("15:04:05", timeHour)
	if err != nil {
		return 0
	}

	dayTimeAt := s.GetOpenTime() + utils.MaxUInt32(0, timeDay-1)*DaySec
	dayTime := time.Unix(int64(dayTimeAt), 0)

	t = time.Date(dayTime.Year(), dayTime.Month(), dayTime.Day(), t.Hour(), t.Minute(), t.Second(), 0, dayTime.Location())
	return uint32(t.Unix())
}

func getYYPavilionBoss() *YYPavilionBoss {
	allYY := yymgr.GetAllYY(yydefine.YYPavilionBoss)
	for _, v := range allYY {
		if sys, ok := v.(*YYPavilionBoss); ok && sys.IsOpen() {
			return sys
		}
	}
	return nil
}

func handlePavBossSettlement(buf []byte) {
	var req pb3.PavBossSettlement
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	playerId := req.ActorId
	monId := req.MonId

	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	s := getYYPavilionBoss()
	if s == nil {
		return
	}

	s.addKillInfo(playerId, monId)
	score := s.getBaseTimeAt() - req.CompleteTimeAt
	s.updateRank(playerId, monId, int64(score))
	s.trySendMail(monId, playerId)

	data := s.getData()
	aData := data.ActorInfo[playerId]
	player.SendProto3(69, 204, &pb3.S2C_69_204{KillInfo: aData.KillInfo})
}

// 玩家击杀boss,但是没抽奖,在玩家退出副本时候,击杀重置false
func handlePavBossActorExit(buf []byte) {
	var req pb3.PavBossActorExit
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	playerId := req.ActorId
	monId := req.MonId

	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	s := getYYPavilionBoss()
	if s == nil {
		return
	}

	data := s.getData()
	if data.ActorInfo == nil {
		return
	}

	aData := data.ActorInfo[playerId]
	if aData == nil {
		return
	}

	killData := aData.KillInfo
	DrawData := aData.DrawInfo

	if killData != nil && killData[monId] {
		if DrawData == nil || !DrawData[monId] {
			killData[monId] = false
		}
	}

	player.SendProto3(69, 204, &pb3.S2C_69_204{
		KillInfo: killData,
	})
}

func handleNewDay(_ ...interface{}) {
	s := getYYPavilionBoss()
	if s == nil {
		return
	}

	conf := jsondata.GetYYPavBossConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	s.initTimer(conf)
}

func init() {
	yymgr.RegisterYYType(yydefine.YYPavilionBoss, func() iface.IYunYing {
		return &YYPavilionBoss{}
	})
	net.RegisterGlobalYYSysProto(69, 201, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPavilionBoss).c2sChallenge
	})

	net.RegisterGlobalYYSysProto(69, 202, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPavilionBoss).c2sDraw
	})

	net.RegisterGlobalYYSysProto(69, 203, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPavilionBoss).c2sGetRank
	})
	engine.RegisterSysCall(sysfuncid.F2GPavBossSettlement, handlePavBossSettlement)
	engine.RegisterSysCall(sysfuncid.F2GPavBossActorExit, handlePavBossActorExit)

	event.RegSysEvent(custom_id.SeNewDayArrive, handleNewDay)

	gmevent.Register("pavBoss.challenge", func(player iface.IPlayer, args ...string) bool {
		sys := getYYPavilionBoss()
		if sys == nil {
			return false
		}
		if len(args) < 1 {
			return false
		}

		monId := utils.AtoUint32(args[0])

		msg := base.NewMessage()
		msg.SetCmd(69<<8 | 201)
		err := msg.PackPb3Msg(&pb3.C2S_69_201{
			Base: &pb3.YYBase{
				ActiveId: sys.Id,
			},
			MonId: monId,
		})
		if err != nil {
			return false
		}

		err = sys.c2sChallenge(player, msg)
		if err != nil {
			logger.LogError("err: %v", err)
			return false
		}

		return true
	}, 1)

	gmevent.Register("pavBoss.draw", func(player iface.IPlayer, args ...string) bool {
		sys := getYYPavilionBoss()
		if sys == nil {
			return false
		}
		if len(args) < 1 {
			return false
		}

		monId := utils.AtoUint32(args[0])

		msg := base.NewMessage()
		msg.SetCmd(69<<8 | 202)
		err := msg.PackPb3Msg(&pb3.C2S_69_202{
			Base: &pb3.YYBase{
				ActiveId: sys.Id,
			},
			MonId: monId,
		})
		if err != nil {
			return false
		}

		err = sys.c2sDraw(player, msg)
		if err != nil {
			logger.LogError("err: %v", err)
			return false
		}

		return true
	}, 1)
}
