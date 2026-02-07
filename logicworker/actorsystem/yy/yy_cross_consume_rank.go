/**
 * @Author: LvYuMeng
 * @Date: 2025/4/21
 * @Desc: 跨服大亨(跨服 消费排行)
**/

package yy

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type YYCrossConsumeRank struct {
	YYBase
}

func (s *YYCrossConsumeRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.SrvCrossRankData {
		return
	}
	delete(globalVar.YyDatas.SrvCrossRankData, s.GetId())
}

func (yy *YYCrossConsumeRank) getPlayerData(playerId uint64) *pb3.SrvCrossRankPlayerData {
	actData := yy.getThisData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.SrvCrossRankPlayerData{
			ActorId: playerId,
		}
	}
	return actData.PlayerData[playerId]
}

func (yy *YYCrossConsumeRank) getThisData() *pb3.SrvCrossRankData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.SrvCrossRankData == nil {
		globalVar.YyDatas.SrvCrossRankData = make(map[uint32]*pb3.SrvCrossRankData)
	}

	tmp := globalVar.YyDatas.SrvCrossRankData[yy.Id]
	if nil == tmp {
		tmp = new(pb3.SrvCrossRankData)
	}
	if nil == tmp.PlayerData {
		tmp.PlayerData = make(map[uint64]*pb3.SrvCrossRankPlayerData)
	}
	globalVar.YyDatas.SrvCrossRankData[yy.Id] = tmp

	return tmp
}

func (s *YYCrossConsumeRank) OnOpen() {
	s.syncActInfo()
}

func (s *YYCrossConsumeRank) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.syncActInfo()
	s.syncAllScore(0)
}

func (s *YYCrossConsumeRank) PlayerLogin(player iface.IPlayer) {
}

func (s *YYCrossConsumeRank) PlayerReconnect(player iface.IPlayer) {
}

func (s *YYCrossConsumeRank) OnEnd() {
	s.onSettle()
}

func (s *YYCrossConsumeRank) syncActInfo() {
	yyCrossRankActInfoSync(&pb3.G2CSyncYYCrossRankOpen{
		YyId:      s.Id,
		StartTime: s.OpenTime,
		EndTime:   s.EndTime,
		ConfIdX:   s.ConfIdx,
		ConfName:  s.ConfName,
		RankType:  custom_id.YYCrossRankTypeConsume,
	})
}

func (s *YYCrossConsumeRank) syncAllScore(actorId uint64) {
	var syncList []*pb3.SyncYYCrossRankValueSt
	if actorId == 0 {
		data := s.getThisData()
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
		RankType: custom_id.YYCrossRankTypeConsume,
		Value:    syncList,
	})
}

func (s *YYCrossConsumeRank) handleAeConsumeMoney(player iface.IPlayer, mt uint32, count int64) {
	data := s.getThisData()
	if data.IsCalc {
		return
	}

	if mt != moneydef.Diamonds {
		return
	}

	if count == 0 {
		return
	}

	playerData := s.getPlayerData(player.GetId())
	playerData.Score += count
	playerData.TimeStamp = time_util.NowSec()

	s.syncAllScore(player.GetId())

	logworker.LogPlayerBehavior(player, pb3.LogId_LogYYConsumeRankCount, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", playerData.Score),
	})
}

func (s *YYCrossConsumeRank) c2sRank(player iface.IPlayer, msg *base.Message) error {
	playerData := s.getPlayerData(player.GetId())

	yyCrossRankInfoReq(&pb3.G2CReqYYCrossRankInfo{
		YyId:     s.Id,
		ActorId:  player.GetId(),
		MyScore:  playerData.Score,
		RankType: custom_id.YYCrossRankTypeConsume,
	})

	return nil
}

func (s *YYCrossConsumeRank) onSettle() {
	data := s.getThisData()
	if data.IsCalc {
		return
	}
	data.IsCalc = true

	yyCrossRankAwardsCalc(&pb3.G2CCalcRankAwards{
		RankType: custom_id.YYCrossRankTypeConsume,
		YyId:     s.Id,
	})
}

func (s *YYCrossConsumeRank) onHourArrive(hour int) {
	conf, ok := jsondata.GetYYCrossRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	if !time_util.IsSameDay(s.EndTime-1, time_util.NowSec()) {
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

func rangeYYCrossRank(doLogic func(yy iface.IYunYing)) {
	allYY := yymgr.GetAllYY(yydefine.YYCrossConsumeRank)
	for _, iYunYing := range allYY {
		if !iYunYing.IsOpen() {
			continue
		}
		doLogic(iYunYing)
	}
}

func yyCrossRankOnSeHourArrive(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	allYY := yymgr.GetAllYY(yydefine.YYCrossConsumeRank)
	for _, iYunYing := range allYY {
		iYunYing.(*YYCrossConsumeRank).onHourArrive(hour)
	}
}

func syncYYCrossConsumeInfo(args ...interface{}) {
	allYY := yymgr.GetAllYY(yydefine.YYCrossConsumeRank)
	for _, iYunYing := range allYY {
		iYunYing.(*YYCrossConsumeRank).syncActInfo()
		iYunYing.(*YYCrossConsumeRank).syncAllScore(0)
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYCrossConsumeRank, func() iface.IYunYing {
		return &YYCrossConsumeRank{}
	})

	net.RegisterGlobalYYSysProto(75, 61, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYCrossConsumeRank).c2sRank
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, syncYYCrossConsumeInfo)

	event.RegSysEvent(custom_id.SeHourArrive, yyCrossRankOnSeHourArrive)

	event.RegActorEvent(custom_id.AeConsumeMoney, func(player iface.IPlayer, args ...interface{}) {
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
		if mt != moneydef.Diamonds {
			return
		}
		rangeYYCrossRank(func(yy iface.IYunYing) {
			s := yy.(*YYCrossConsumeRank)
			s.handleAeConsumeMoney(player, mt, count)
		})
	})
}
