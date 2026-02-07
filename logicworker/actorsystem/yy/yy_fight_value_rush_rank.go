/**
 * @Author: zjj
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-玩法分数比拼
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
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

type PlayScoreRank struct {
	YYBase

	ScoreRank *component.ScoreRank
}

func (s *PlayScoreRank) getData() *pb3.YYPlayScoreRank {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.PlayScoreRankMap == nil {
		datas.PlayScoreRankMap = make(map[uint32]*pb3.YYPlayScoreRank)
	}
	if datas.PlayScoreRankMap[s.Id] == nil {
		datas.PlayScoreRankMap[s.Id] = &pb3.YYPlayScoreRank{}
	}
	if datas.PlayScoreRankMap[s.Id].ExtPlayerInfoMap == nil {
		datas.PlayScoreRankMap[s.Id].ExtPlayerInfoMap = make(map[uint64]*pb3.YYFightValueRushRankExt)
	}
	if datas.PlayScoreRankMap[s.Id].JoinFlagMap == nil {
		datas.PlayScoreRankMap[s.Id].JoinFlagMap = make(map[uint64]bool)
	}
	if datas.PlayScoreRankMap[s.Id].JoinFlagMap == nil {
		datas.PlayScoreRankMap[s.Id].JoinFlagMap = make(map[uint64]bool)
	}
	if datas.PlayScoreRankMap[s.Id].ExScore == nil {
		datas.PlayScoreRankMap[s.Id].ExScore = make(map[uint64]int64)
	}
	if datas.PlayScoreRankMap[s.Id].DailyRev == nil {
		datas.PlayScoreRankMap[s.Id].DailyRev = make(map[uint64]uint32)
	}
	return datas.PlayScoreRankMap[s.Id]
}

func (s *PlayScoreRank) OnInit() {
	s.YYBase.OnInit()
	data := s.getData()
	conf, ok := jsondata.GetYYPlayScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("conf not found")
		return
	}
	data.RankType = conf.RankType
	s.ScoreRank = component.NewScoreRank(
		conf.CommonScoreRank,
		component.WithScoreRankOptionByPacketPlayerInfoToRankInfo(manager.ExportPacketPlayerInfoToRankInfo),
		component.WithScoreRankOptionByGetExScore(s.GetPlayerExScore),
	)
	s.ScoreRank.InitialUpdate(data.Items)
}

func (s *PlayScoreRank) GetPlayerExScore(playerId uint64) int64 {
	data := s.getData()
	return data.ExScore[playerId]
}

func (s *PlayScoreRank) PlayerLogin(player iface.IPlayer) {
	s.s2cDailyAwards(player)
	err := s.s2cRankData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *PlayScoreRank) PlayerReconnect(player iface.IPlayer) {
	s.s2cDailyAwards(player)
	err := s.s2cRankData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *PlayScoreRank) s2cDailyAwards(player iface.IPlayer) {
	player.SendProto3(69, 53, &pb3.S2C_69_53{
		ActiveId:        s.Id,
		LastReceiveTime: s.getData().DailyRev[player.GetId()],
	})
}

func (s *PlayScoreRank) NewDay() {
	data := s.getData()
	data.DailyRev = nil
	s.Broadcast(69, 53, &pb3.S2C_69_53{ActiveId: s.Id})
}

func (s *PlayScoreRank) packetPlayerInfo(playerId uint64, rankInfo *pb3.RankInfo) *pb3.YYFightValueRushRankInfo {
	data := s.getData()
	manager.PacketPlayScoreRankPlayerInfo(playerId, rankInfo)
	newRankInfo := rankInfoToYYFightValueRushRankInfo(rankInfo)
	commonSt := data.ExtPlayerInfoMap[playerId]
	if commonSt == nil {
		commonSt = &pb3.YYFightValueRushRankExt{}
	}
	newRankInfo.Ext = commonSt
	return newRankInfo
}

func (s *PlayScoreRank) BroadcastRank() {
	var rsp = &pb3.S2C_69_50{
		ActiveId: s.Id,
		RankType: s.getData().RankType,
	}
	var rankMgr = make(map[uint32]*pb3.YYFightValueRushRankInfo)
	rankData, err := s.ScoreRank.GetRankData(false)
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
	for rankIndex, rankInfo := range rankData {
		rankMgr[rankIndex] = s.packetPlayerInfo(rankInfo.PlayerId, rankInfo)
	}
	rsp.RankMgr = rankMgr
	s.Broadcast(69, 50, rsp)
}

func rankInfoToYYFightValueRushRankInfo(rankInfo *pb3.RankInfo) *pb3.YYFightValueRushRankInfo {
	var newRankInfo = &pb3.YYFightValueRushRankInfo{
		Rank:      rankInfo.Rank,
		Key:       rankInfo.Key,
		Value:     rankInfo.Value,
		PlayerId:  rankInfo.PlayerId,
		Head:      rankInfo.Head,
		VipLv:     rankInfo.VipLv,
		Name:      rankInfo.Name,
		Job:       rankInfo.Job,
		Like:      rankInfo.Like,
		GuildName: rankInfo.GuildName,
		Appear:    rankInfo.Appear,
		HeadFrame: rankInfo.HeadFrame,
		Circle:    rankInfo.Circle,
		Ext:       nil,
	}
	return newRankInfo
}

func (s *PlayScoreRank) s2cRankData(player iface.IPlayer, _ *base.Message) error {
	data := s.getData()
	yyId := s.GetId()

	var rankMgr = make(map[uint32]*pb3.YYFightValueRushRankInfo)
	rankData, err := s.ScoreRank.GetRankData(true)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	limit := s.ScoreRank.GetLimit(false)
	for rankIndex, rankInfo := range rankData {
		if rankIndex > limit {
			continue
		}
		rankMgr[rankIndex] = s.packetPlayerInfo(rankInfo.PlayerId, rankInfo)
	}

	myRankInfo := s.ScoreRank.GetMyRankInfo(rankData, player.GetId())
	newRankInfo := s.packetPlayerInfo(player.GetId(), myRankInfo)
	if newRankInfo.Value == 0 {
		s.resetJoinFlag(player, yyId)
		newRankInfo.Value = player.GetYYRankValue(yyId)
	}

	var rsp = &pb3.S2C_69_51{
		ActiveId:   s.Id,
		RankType:   data.RankType,
		RankMgr:    rankMgr,
		MyRankInfo: newRankInfo,
		InRank:     newRankInfo.Rank != 0,
	}

	player.SendProto3(69, 51, rsp)
	return nil
}

func (s *PlayScoreRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	datas := globalVar.YyDatas
	if datas.PlayScoreRankMap == nil {
		return
	}
	delete(datas.PlayScoreRankMap, s.Id)
}

func (s *PlayScoreRank) OnOpen() {
	s.OnInit()
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.GetAttrSys().TriggerUpdateSysPowerMap()
	})
	s.BroadcastRank()
}

func (s *PlayScoreRank) OnEnd() {
	s.settlement()
}

func (s *PlayScoreRank) ServerStopSaveData() {
	s.getData().Items = s.ScoreRank.PackToProto()
}

func (s *PlayScoreRank) resetJoinFlag(player iface.IPlayer, k uint32) {
	data := s.getData()
	playerId := player.GetId()
	if flag := data.JoinFlagMap[playerId]; !flag {
		player.SetYYRankValue(k, 0)
		data.JoinFlagMap[playerId] = true
	}
}

func (s *PlayScoreRank) updateRank(rankType ranktype.PlayScoreRankType, playerId uint64, score int64, isAdd bool, exScore int64) {
	data := s.getData()
	if data.RankType != uint32(rankType) {
		return
	}
	if data.IsSettlement {
		return
	}

	player := manager.GetPlayerPtrById(playerId)
	s.resetJoinFlag(player, uint32(rankType))
	var newScore = score
	if isAdd {
		oldScore := player.GetYYRankValue(uint32(rankType))
		newScore = oldScore + score
	}

	player.SetYYRankValue(uint32(rankType), newScore)
	data.ExScore[playerId] = exScore
	if s.ScoreRank.Update(playerId, newScore) {
		s.BroadcastRank()
	}
}

func (s *PlayScoreRank) onHourArrive(hour int) {
	endTime := s.GetEndTime()
	nowSec := time_util.NowSec()
	if !time_util.IsSameDay(endTime-1, nowSec) {
		return
	}
	conf, ok := jsondata.GetYYPlayScoreRankConf(s.ConfName, s.ConfIdx)
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
	s.settlement()
}

func (s *PlayScoreRank) settlement() {
	conf, ok := jsondata.GetYYPlayScoreRankConf(s.ConfName, s.ConfIdx)
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
	var getContentAndMailId = func(rankType ranktype.PlayScoreRankType, rankIdx uint32) (uint16, proto.Message) {
		mailId := uint16(conf.MailId)
		args := &mailargs.RankArgs{
			Rank: rankIdx,
		}

		// 强制先写死 避免策划配错难排查
		switch rankType {
		case ranktype.PlayScoreRankTypeFairy:
			mailId = common.Mail_YYPlayScoreRankByFariy
		case ranktype.PlayScoreRankTypeFlyUpLoadItem:
			mailId = common.Mail_YYPlayScoreRankByFlyUpRoad
		case ranktype.PlayScoreRankTypeNewFaBao:
			mailId = common.Mail_YYPlayScoreRankByNewFaBao
		case ranktype.PlayScoreRankTypeSoulHalo:
			mailId = common.Mail_YYPlayScoreRankBySoulHalo
		case ranktype.PlayScoreRankTypeConsumeDiamonds:
			mailId = common.Mail_YYPlayScoreRankByConsumeDiamonds
		case ranktype.PlayScoreRankTypeGlobalCollectCards:
			mailId = common.Mail_YYPlayScoreRankByGlobalCollectCards
		default:
			s.LogWarn("rank type not confect %d", rankType)
		}
		return mailId, args
	}

	s.ScoreRank.Settlement(func(rank uint32, rankInfo *pb3.RankInfo, awards jsondata.StdRewardVec) {
		mailId, content := getContentAndMailId(ranktype.PlayScoreRankType(data.RankType), rank)
		mailmgr.SendMailToActor(rankInfo.PlayerId, &mailargs.SendMailSt{
			ConfId:  mailId,
			Rewards: awards,
			Content: content,
		})
	})
}

func (s *PlayScoreRank) c2sDailyAward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_53
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPlayScoreRankConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("conf not found")
	}

	if len(conf.DailyAwards) == 0 {
		return neterror.ConfNotFoundError("no awards")
	}

	playerId := player.GetId()
	data := s.getData()
	_, exist := data.DailyRev[playerId]
	if exist {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	data.DailyRev[playerId] = time_util.NowSec()
	player.SendProto3(69, 53, &pb3.S2C_69_53{
		ActiveId:        s.Id,
		LastReceiveTime: data.DailyRev[playerId],
	})

	engine.GiveRewards(player, conf.DailyAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlayScoreRankDailyAwards,
	})

	return nil
}

func playScoreRankUpdateHandler(args ...interface{}) {
	if len(args) < 5 {
		return
	}
	rankType := args[0].(ranktype.PlayScoreRankType)
	playerId := args[1].(uint64)
	score := args[2].(int64)
	isAdd := args[3].(bool)
	exScore := args[4].(int64)
	allYY := yymgr.GetAllYY(yydefine.YYPlayScoreRank)
	for _, iYunYing := range allYY {
		iYunYing.(*PlayScoreRank).updateRank(rankType, playerId, score, isAdd, exScore)
	}
}

func playScoreRankUpdateExtValHandler(args ...interface{}) {
	if len(args) != 3 {
		return
	}
	rankType := args[0].(ranktype.PlayScoreRankType)
	playerId := args[1].(uint64)
	extVal := args[2].(*pb3.YYFightValueRushRankExt)
	allYY := yymgr.GetAllYY(yydefine.YYPlayScoreRank)
	for _, iYunYing := range allYY {
		data := iYunYing.(*PlayScoreRank).getData()
		if data.RankType != uint32(rankType) {
			continue
		}
		data.ExtPlayerInfoMap[playerId] = extVal
	}
}

func playScoreRankHourArriveHandler(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	allYY := yymgr.GetAllYY(yydefine.YYPlayScoreRank)
	for _, iYunYing := range allYY {
		if !iYunYing.IsOpen() {
			continue
		}
		iYunYing.(*PlayScoreRank).onHourArrive(hour)
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYPlayScoreRank, func() iface.IYunYing {
		return &PlayScoreRank{}
	})
	event.RegSysEvent(custom_id.SePlayScoreRankUpdate, playScoreRankUpdateHandler)
	event.RegSysEvent(custom_id.SePlayScoreRankUpdateExtVal, playScoreRankUpdateExtValHandler)
	event.RegSysEvent(custom_id.SeHourArrive, playScoreRankHourArriveHandler)
	net.RegisterGlobalYYSysProto(69, 51, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*PlayScoreRank).s2cRankData
	})
	net.RegisterGlobalYYSysProto(69, 53, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*PlayScoreRank).c2sDailyAward
	})

	gmevent.Register("addPowerByPlayerScoreRank", func(player iface.IPlayer, args ...string) bool {
		rankType := utils.AtoUint32(args[0])
		score := utils.AToU64(args[1])
		manager.UpdatePlayScoreRank(ranktype.PlayScoreRankType(rankType), player, int64(score), false, 0)
		return true
	}, 1)
}
