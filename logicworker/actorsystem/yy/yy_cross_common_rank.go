/**
 * @Author: zjj
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-玩法分数比拼
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type CrossCommonRank struct {
	YYBase
}

func (s *CrossCommonRank) getData() *pb3.YYCrossCommonRank {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.CrossCommonRankMap == nil {
		datas.CrossCommonRankMap = make(map[uint32]*pb3.YYCrossCommonRank)
	}
	if datas.CrossCommonRankMap[s.Id] == nil {
		datas.CrossCommonRankMap[s.Id] = &pb3.YYCrossCommonRank{}
	}
	if nil == datas.CrossCommonRankMap[s.Id].PlayerData {
		datas.CrossCommonRankMap[s.Id].PlayerData = make(map[uint64]*pb3.SrvCrossRankPlayerData)
	}
	return datas.CrossCommonRankMap[s.Id]
}

func (s *CrossCommonRank) getPlayerData(playerId uint64) *pb3.SrvCrossRankPlayerData {
	actData := s.getData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.SrvCrossRankPlayerData{
			ActorId: playerId,
		}
	}
	return actData.PlayerData[playerId]
}

func (s *CrossCommonRank) PlayerLogin(player iface.IPlayer) {}

func (s *CrossCommonRank) PlayerReconnect(player iface.IPlayer) {}

func (s *CrossCommonRank) OnInit() {
	if !s.IsOpen() {
		return
	}
	data := s.getData()
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	data.RankType = conf.RankType
	s.syncActInfo()
	s.syncAllScore(0)
}

func (s *CrossCommonRank) syncAllScore(actorId uint64) {
	var syncList []*pb3.SyncYYCrossRankValueSt
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if actorId == 0 {
		data := s.getData()
		for actorId, v := range data.PlayerData {
			syncList = append(syncList, &pb3.SyncYYCrossRankValueSt{
				ActorId:   actorId,
				MyScore:   v.Score,
				TimeStamp: v.TimeStamp,
			})
		}
	} else {
		playerData := s.getPlayerData(actorId)
		syncList = append(syncList, &pb3.SyncYYCrossRankValueSt{
			ActorId:   actorId,
			MyScore:   playerData.Score,
			TimeStamp: playerData.TimeStamp,
		})
	}
	yyCrossRankDataSync(&pb3.G2CSyncYYCrossRankValue{
		YyId:     s.Id,
		RankType: conf.CrossRankType,
		Value:    syncList,
	})
}

func (s *CrossCommonRank) c2sRank(player iface.IPlayer, msg *base.Message) error {
	playerData := s.getPlayerData(player.GetId())
	var req pb3.C2S_69_52
	if err := pb3.Unmarshal(msg.Data, &req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("config not found")
	}
	yyCrossRankInfoReq(&pb3.G2CReqYYCrossRankInfo{
		YyId:     s.Id,
		ActorId:  player.GetId(),
		MyScore:  playerData.Score,
		RankType: conf.CrossRankType,
	})
	return nil
}

func (s *CrossCommonRank) syncActInfo() {
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	yyCrossRankActInfoSync(&pb3.G2CSyncYYCrossRankOpen{
		YyId:      s.Id,
		StartTime: s.OpenTime,
		EndTime:   s.EndTime,
		ConfIdX:   s.ConfIdx,
		ConfName:  s.ConfName,
		RankType:  conf.CrossRankType,
	})
}

func (s *CrossCommonRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.CrossCommonRankMap == nil {
		return
	}
	delete(datas.CrossCommonRankMap, s.Id)
}

func (s *CrossCommonRank) OnOpen() {
	s.OnInit()
	s.syncActInfo()
}

func (s *CrossCommonRank) OnEnd() {
	s.onSettle()
}

func (s *CrossCommonRank) onSettle() {
	data := s.getData()
	if data.IsCalc {
		return
	}
	data.IsCalc = true
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	yyCrossRankAwardsCalc(&pb3.G2CCalcRankAwards{
		RankType: conf.CrossRankType,
		YyId:     s.Id,
	})
}

func (s *CrossCommonRank) updateRank(params *pb3.YYCrossRankParams) {
	data := s.getData()
	if data.RankType != params.RankType {
		return
	}
	if data.IsCalc {
		return
	}

	playerData := s.getPlayerData(params.GetActorId())
	playerData.Score += params.Score
	playerData.TimeStamp = time_util.NowSec()
	s.syncAllScore(params.GetActorId())
	logworker.LogPlayerBehavior(manager.GetPlayerPtrById(params.GetActorId()), pb3.LogId_LogPYYCrossCommonRankUpdate, &pb3.LogPlayerCounter{
		NumArgs: uint64(params.Score),
	})
}

func (s *CrossCommonRank) onHourArrive(hour int) {
	endTime := s.GetEndTime()
	nowSec := time_util.NowSec()
	if !time_util.IsSameDay(endTime-1, nowSec) {
		return
	}
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	// 0 表示活动结束结算
	if conf.SettlementHour == 0 {
		return
	}
	if conf.SettlementHour != uint32(hour) {
		s.LogError("%d != %d ", conf.SettlementHour, hour)
		return
	}
	s.onSettle()
}

func (s *CrossCommonRank) SetScore(actorId uint64, score int64) {
	data := s.getData()
	if data.IsCalc {
		return
	}

	playerData := s.getPlayerData(actorId)
	playerData.Score = score
	playerData.TimeStamp = time_util.NowSec()
	s.syncAllScore(actorId)
}

func (s *CrossCommonRank) CleanRank() {
	data := s.getData()
	conf, ok := jsondata.GetYYCrossScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var syncList []*pb3.SyncYYCrossRankValueSt
	for actorId, v := range data.PlayerData {
		syncList = append(syncList, &pb3.SyncYYCrossRankValueSt{
			ActorId:   actorId,
			MyScore:   0,
			TimeStamp: v.TimeStamp,
		})
	}
	data.PlayerData = make(map[uint64]*pb3.SrvCrossRankPlayerData)
	yyCrossRankDataSync(&pb3.G2CSyncYYCrossRankValue{
		YyId:     s.Id,
		RankType: conf.CrossRankType,
		Value:    syncList,
	})
}

func syncYYCrossCommonInfo(args ...interface{}) {
	allYY := yymgr.GetAllYY(yydefine.YYCrossCommonRank)
	for _, iYunYing := range allYY {
		iYunYing.(*CrossCommonRank).syncActInfo()
		iYunYing.(*CrossCommonRank).syncAllScore(0)
	}
}

func CrossCommonRankUpdateHandler(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	params := args[0].(*pb3.YYCrossRankParams)
	allYY := yymgr.GetAllYY(yydefine.YYCrossCommonRank)
	for _, iYunYing := range allYY {
		iYunYing.(*CrossCommonRank).updateRank(params)
	}
}

func CrossCommonRankHourArriveHandler(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	allYY := yymgr.GetAllYY(yydefine.YYCrossCommonRank)
	for _, iYunYing := range allYY {
		if !iYunYing.IsOpen() {
			continue
		}
		iYunYing.(*CrossCommonRank).onHourArrive(hour)
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYCrossCommonRank, func() iface.IYunYing {
		return &CrossCommonRank{}
	})
	event.RegSysEvent(custom_id.SeCrossCommonRankUpdate, CrossCommonRankUpdateHandler)
	event.RegSysEvent(custom_id.SeHourArrive, CrossCommonRankHourArriveHandler)
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, syncYYCrossCommonInfo)
	net.RegisterGlobalYYSysProto(69, 52, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*CrossCommonRank).c2sRank
	})
}
