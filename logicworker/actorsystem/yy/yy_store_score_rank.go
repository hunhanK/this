/**
 * @Author: LvYuMeng
 * @Date: 2025/12/5
 * @Desc: 积分排行（积分比拼）
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type YYStoreScoreRank struct {
	YYBase
	loopId uint32
	rank   *base.Rank
}

func (yy *YYStoreScoreRank) OnInit() {
	if !yy.IsOpen() {
		return
	}
	conf := yy.GetConf()
	if nil == conf {
		return
	}

	openDay := yy.GetOpenDay()

	data := yy.getData()
	for _, v := range data.RankData {
		if v.IsSettlement {
			continue
		}
		chapterConf := conf.GetChapterConfigByOpenDay(openDay)
		if nil == chapterConf {
			continue
		}
		if chapterConf.EndDay >= openDay {
			continue
		}
		yy.settlement(chapterConf.LoopId)
	}

	if chapterConf := conf.GetChapterConfigByOpenDay(openDay); nil != chapterConf {
		yy.rank = base.NewRank(chapterConf.StatMaxLimit)
		yy.loopId = chapterConf.LoopId
		rankData := yy.getRankData(chapterConf.LoopId)
		for _, v := range rankData.Ranks {
			yy.rank.Update(v.Id, v.Score)
		}
	}
}

func (yy *YYStoreScoreRank) GetConf() *jsondata.YYStoreScoreRankConfig {
	return jsondata.GetYYStoreScoreRankConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYStoreScoreRank) OnOpen() {
	yy.Broadcast(143, 21, &pb3.S2C_143_21{
		ActiveId: yy.Id,
	})
}

func (yy *YYStoreScoreRank) OnEnd() {
	yy.settlement(yy.loopId)
}

func (yy *YYStoreScoreRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.StoreScoreRank {
		return
	}
	delete(globalVar.YyDatas.StoreScoreRank, yy.GetId())
}

func (yy *YYStoreScoreRank) PlayerLogin(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYStoreScoreRank) PlayerReconnect(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYStoreScoreRank) s2cPlayerInfo(player iface.IPlayer) {
	var (
		playerId = player.GetId()
		pData    = yy.getPlayerData(playerId)
		data     = yy.getData()
		client   = &pb3.YYStoreScoreClientData{
			Score: pData.Score,
		}
	)
	for _, v := range data.RankData {
		client.UseScore += v.RankScores[playerId]
	}
	player.SendProto3(143, 21, &pb3.S2C_143_21{
		ActiveId:   yy.Id,
		PlayerData: client,
	})
}

func (yy *YYStoreScoreRank) getData() *pb3.YYStoreScoreRank {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.StoreScoreRank == nil {
		globalVar.YyDatas.StoreScoreRank = make(map[uint32]*pb3.YYStoreScoreRank)
	}

	tmp, ok := globalVar.YyDatas.StoreScoreRank[yy.Id]
	if !ok {
		tmp = new(pb3.YYStoreScoreRank)
		globalVar.YyDatas.StoreScoreRank[yy.Id] = tmp
	}

	if nil == tmp.PlayerData {
		tmp.PlayerData = make(map[uint64]*pb3.YYStoreScorePlayerData)
	}

	if nil == tmp.RankData {
		tmp.RankData = make(map[uint32]*pb3.YYStoreScoreRankData)
	}

	return tmp
}

func (yy *YYStoreScoreRank) getPlayerData(playerId uint64) *pb3.YYStoreScorePlayerData {
	actData := yy.getData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.YYStoreScorePlayerData{}
	}
	return actData.PlayerData[playerId]
}

func (yy *YYStoreScoreRank) getRankData(loopId uint32) *pb3.YYStoreScoreRankData {
	actData := yy.getData()
	rankData, ok := actData.RankData[loopId]
	if !ok {
		rankData = &pb3.YYStoreScoreRankData{}
		actData.RankData[loopId] = rankData
	}

	if rankData.RankScores == nil {
		rankData.RankScores = make(map[uint64]int64)
	}

	return rankData
}

func (yy *YYStoreScoreRank) GetReachStandardScore(playerId uint64) int64 {
	var useScore int64
	data := yy.getData()
	for _, v := range data.RankData {
		useScore += v.RankScores[playerId]
	}
	return useScore
}

func (yy *YYStoreScoreRank) addScore(player iface.IPlayer, score int64) {
	pData := yy.getPlayerData(player.GetId())
	pData.Score += score
	yy.s2cPlayerInfo(player)
}

func (yy *YYStoreScoreRank) consumeItem(player iface.IPlayer, itemId uint32, count int64, isMoney bool) {
	conf := yy.GetConf()
	if nil == conf {
		return
	}
	var addScore uint32
	var exist bool
	if isMoney {
		addScore, exist = conf.ConsumeMoney[itemId]
	} else {
		addScore, exist = conf.ConsumeItems[itemId]
	}
	if !exist {
		return
	}

	score := int64(addScore) * count
	yy.addScore(player, score)
}

func (yy *YYStoreScoreRank) getRankList(loopId uint32) (ranks []*pb3.YYStoreScoreRankInfo) {
	loopConf := yy.GetConf().GetChapterConfigByLoopId(loopId)
	if nil == loopConf {
		return
	}

	if loopId == yy.loopId {
		yy.saveRank()
	}

	rankData := yy.getRankData(loopId)
	var checkMinEnterRankCand = func(rankIdx uint32, score int64) bool {
		cand := loopConf.MinEnterCand
		if cand == nil {
			return true
		}
		for _, enterCand := range cand {
			if enterCand.MinRank != 0 && enterCand.MinRank > rankIdx || enterCand.MaxRank < rankIdx {
				continue
			}
			if enterCand.MinScore != 0 && enterCand.MinScore > score {
				return false
			}
			return true
		}
		return false
	}

	rankIndex := uint32(0)
	for _, line := range rankData.Ranks {
		rankIndex++
		if rankIndex > loopConf.StatMaxLimit {
			break
		}

		for rankIndex <= loopConf.StatMaxLimit {
			if !checkMinEnterRankCand(rankIndex, line.Score) {
				rankIndex++
				continue
			}
			var rankInfo = &pb3.YYStoreScoreRankInfo{
				Rank:     rankIndex,
				Key:      line.Id,
				Value:    line.Score,
				PlayerId: line.Id,
			}
			if role, ok := manager.GetData(line.Id, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
				rankInfo.Head = role.Head
				rankInfo.HeadFrame = role.HeadFrame
				rankInfo.Job = role.Job
				rankInfo.Name = role.Name
			}
			ranks = append(ranks, rankInfo)
			break
		}
	}
	return
}

func (yy *YYStoreScoreRank) c2sRank(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_143_20
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	loopId := req.LoopId
	loopConf := yy.GetConf().GetChapterConfigByLoopId(loopId)
	if nil == loopConf {
		return neterror.ParamsInvalidError("loop conf is nil")
	}

	playerId := player.GetId()
	rankData := yy.getRankData(loopId)
	rsp := &pb3.S2C_143_20{
		ActiveId: yy.Id,
		LoopId:   loopId,
		MyRanks: &pb3.YYStoreScoreRankInfo{
			Key:       playerId,
			Value:     rankData.RankScores[playerId],
			PlayerId:  playerId,
			Head:      player.GetHead(),
			Name:      player.GetName(),
			Job:       player.GetJob(),
			HeadFrame: player.GetHeadFrame(),
		},
	}

	rankList := yy.getRankList(loopId)
	for _, line := range rankList {
		if line.Rank <= loopConf.ShowMaxLimit {
			rsp.Ranks = append(rsp.Ranks, line)
		}
		if line.Key == playerId {
			rsp.MyRanks = line
		}
	}

	player.SendProto3(143, 20, rsp)
	return nil
}

func (yy *YYStoreScoreRank) c2sUseScore(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_143_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	chapterConf := yy.GetConf().GetChapterConfigByOpenDay(yy.GetOpenDay())
	if nil == chapterConf {
		return neterror.ConfNotFoundError("chapter conf is nil")
	}

	if chapterConf.LoopId != yy.loopId {
		return neterror.InternalError("loopId not equal")
	}

	rankData := yy.getRankData(chapterConf.LoopId)
	if rankData.IsSettlement {
		return neterror.ParamsInvalidError("already calc")
	}

	useScore := req.Score
	playerId := player.GetId()
	pData := yy.getPlayerData(playerId)
	if pData.Score < useScore {
		return neterror.ParamsInvalidError("score not enough")
	}

	pData.Score -= useScore

	rankData.RankScores[playerId] += useScore
	yy.rank.Update(playerId, rankData.RankScores[playerId])

	yy.s2cPlayerInfo(player)
	return nil
}

func (yy *YYStoreScoreRank) ServerStopSaveData() {
	yy.saveRank()
}

func (yy *YYStoreScoreRank) saveRank() {
	if yy.loopId == 0 {
		return
	}
	rankData := yy.getRankData(yy.loopId)
	rankData.Ranks = yy.PackToProto()
}

func (yy *YYStoreScoreRank) PackToProto() []*pb3.OneRankItem {
	var bkItems []*pb3.OneRankItem
	yy.rank.ChunkAll(func(item *pb3.OneRankItem) bool {
		bkItems = append(bkItems, &pb3.OneRankItem{
			Id:    item.Id,
			Score: item.Score,
		})
		return false
	})
	return bkItems
}

func (yy *YYStoreScoreRank) NewDay() {
	loopConf := yy.GetConf().GetChapterConfigByLoopId(yy.loopId)
	if loopConf == nil {
		yy.LogError("conf not found")
		return
	}
	if loopConf.EndDay >= yy.GetOpenDay() {
		return
	}
	yy.settlement(yy.loopId)
	yy.changeLoop()
}

func (yy *YYStoreScoreRank) onHourArrive(hour int) {
	loopConf := yy.GetConf().GetChapterConfigByLoopId(yy.loopId)
	if loopConf == nil {
		yy.LogError("conf not found")
		return
	}
	if loopConf.EndDay != yy.GetOpenDay() {
		return
	}
	// 0 表示活动结束结算
	if loopConf.SettlementHour == 0 {
		return
	}
	if loopConf.SettlementHour != uint32(hour) {
		yy.LogError("%d != %d ", loopConf.SettlementHour, hour)
		return
	}
	yy.settlement(yy.loopId)
}

func (yy *YYStoreScoreRank) settlement(loopId uint32) {
	if yy.loopId == loopId {
		yy.saveRank()
	}
	loopConf := yy.GetConf().GetChapterConfigByLoopId(loopId)
	if nil == loopConf {
		yy.LogError("loop %d conf is nil", loopId)
		return
	}

	rankData := yy.getRankData(loopId)
	if rankData.IsSettlement {
		return
	}
	rankData.IsSettlement = true

	rankList := yy.getRankList(loopId)
	var checkAwardsRangeByRank = func(rank uint32, conf jsondata.CommonScoreRankAwardsVec) jsondata.StdRewardVec {
		for _, rankAward := range conf {
			if rankAward.Max != 0 && rank > rankAward.Max {
				continue
			}
			if rankAward.Min != 0 && rank < rankAward.Min {
				continue
			}

			if rankAward.MaxRank != 0 && rank > rankAward.MaxRank {
				continue
			}
			if rankAward.MinRank != 0 && rank < rankAward.MinRank {
				continue
			}

			return rankAward.Awards
		}
		return nil
	}

	for _, info := range rankList {
		rewardVec := checkAwardsRangeByRank(info.Rank, loopConf.RankAwards)
		if len(rewardVec) == 0 {
			logger.LogError("not found awards conf. idx:%d, info:%+v", info.Rank, info)
			continue
		}
		mailmgr.SendMailToActor(info.Key, &mailargs.SendMailSt{
			ConfId:  loopConf.MailId,
			Content: &mailargs.RankArgs{Rank: info.Rank},
			Rewards: rewardVec,
		})
	}
}

func (yy *YYStoreScoreRank) changeLoop() {
	yy.saveRank()
	chapterConf := yy.GetConf().GetChapterConfigByOpenDay(yy.GetOpenDay())
	if nil == chapterConf {
		return
	}

	if chapterConf.LoopId == yy.loopId {
		return
	}

	yy.rank = base.NewRank(chapterConf.StatMaxLimit)
	yy.loopId = chapterConf.LoopId
}

func onYYStoreScoreRankConsumeItem(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	itemId, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok {
		return
	}

	yymgr.EachAllYYObj(yydefine.YYStoreScoreRank, func(obj iface.IYunYing) {
		s, ok := obj.(*YYStoreScoreRank)
		if !ok || !s.IsOpen() {
			return
		}
		s.consumeItem(player, itemId, int64(count), false)
	})
}

func onYYStoreScoreRankConsumeMoney(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	mt, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok {
		return
	}
	yymgr.EachAllYYObj(yydefine.YYStoreScoreRank, func(obj iface.IYunYing) {
		s, ok := obj.(*YYStoreScoreRank)
		if !ok || !s.IsOpen() {
			return
		}
		s.consumeItem(player, mt, count, true)
	})
}

func onYYStoreScoreRankHourArriveHandler(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	yymgr.EachAllYYObj(yydefine.YYStoreScoreRank, func(obj iface.IYunYing) {
		s, ok := obj.(*YYStoreScoreRank)
		if !ok || !s.IsOpen() {
			return
		}
		s.onHourArrive(hour)
	})
}

func init() {
	yymgr.RegisterYYType(yydefine.YYStoreScoreRank, func() iface.IYunYing {
		return &YYStoreScoreRank{}
	})

	event.RegSysEvent(custom_id.SeHourArrive, onYYStoreScoreRankHourArriveHandler)

	event.RegActorEvent(custom_id.AeConsumeItem, onYYStoreScoreRankConsumeItem)
	event.RegActorEvent(custom_id.AeConsumeMoney, onYYStoreScoreRankConsumeMoney)

	net.RegisterGlobalYYSysProto(143, 20, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYStoreScoreRank).c2sRank
	})
	net.RegisterGlobalYYSysProto(143, 23, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYStoreScoreRank).c2sUseScore
	})

	gmevent.Register("YYStoreScoreRank.addScore", func(player iface.IPlayer, args ...string) bool {
		yymgr.EachAllYYObj(yydefine.YYStoreScoreRank, func(obj iface.IYunYing) {
			s, ok := obj.(*YYStoreScoreRank)
			if !ok || !s.IsOpen() {
				return
			}
			s.addScore(player, utils.AtoInt64(args[0]))
		})
		return true
	}, 1)
}
