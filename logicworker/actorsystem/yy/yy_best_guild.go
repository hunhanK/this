/**
 * @Author: LvYuMeng
 * @Date: 2025/7/18
 * @Desc: 抽奖排行
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
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
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type YYBestGuild struct {
	YYBase
	rank *base.Rank
}

func (yy *YYBestGuild) OnInit() {
	if !yy.IsOpen() {
		return
	}

	conf := yy.GetConf()
	if nil == conf {
		return
	}

	yy.rank = base.NewRank(conf.GetBestGuildCount())

	data := yy.getData()
	for _, v := range data.Ranks {
		yy.rank.Update(v.Id, v.Score)
	}
}

func (yy *YYBestGuild) OnOpen() {
	yy.Broadcast(75, 110, &pb3.S2C_75_110{
		ActiveId: yy.Id,
		Data:     &pb3.BestGuildPlayerData{},
	})
}

func (yy *YYBestGuild) targetCalc() {
	conf := yy.GetConf()
	if nil == conf {
		return
	}

	data := yy.getData()

	for guildId, gData := range data.GuildScores {
		g := guildmgr.GetGuildById(guildId)
		if nil == g {
			continue
		}

		var revIds []uint32
		var ids []int
		for i, v := range conf.SumAwards {
			if gData.Kills[v.Id] < v.Count {
				continue
			}
			revIds = append(revIds, v.Id)
			ids = append(ids, i)
		}

		for playerId := range g.Members {
			if engine.IsRobot(playerId) {
				continue
			}

			pData := yy.getPlayerData(playerId)

			var rewardVec []jsondata.StdRewardVec

			for i, revId := range revIds {
				if pie.Uint32s(pData.RevIds).Contains(revId) {
					continue
				}
				pData.RevIds = append(pData.RevIds, revId)
				rewardVec = append(rewardVec, conf.SumAwards[ids[i]].Rewards)
			}

			rewards := jsondata.AppendStdReward(rewardVec...)
			if len(rewards) > 0 {
				mailmgr.SendMailToActor(playerId, &mailargs.SendMailSt{
					ConfId:  conf.PersonMailId,
					Rewards: rewards,
				})
			}
		}
	}
}

func (yy *YYBestGuild) rankCalc() {
	conf := yy.GetConf()
	if nil == conf {
		return
	}

	data := yy.getData()
	if data.IsCalc {
		return
	}

	data.IsCalc = true

	var rankIndex uint32
	for _, v := range yy.rank.GetList(1, yy.rank.GetRankCount()) {
		g := guildmgr.GetGuildById(v.Id)
		if nil == g {
			continue
		}

		rankIndex++

		obj := &pb3.BestGuildRankInfo{
			GuildId: v.Id,
			Name:    g.GetName(),
			Rank:    rankIndex,
			Score:   v.Score,
		}

		yy.GetRealRank(conf, obj)

		rankIndex = obj.Rank

		if rankConf := conf.GetBestGuildConf(rankIndex); nil != rankConf {
			mailSt := &mailargs.SendMailSt{
				ConfId: conf.RankMailId,
				Content: &mailargs.YYBestGuildRank{
					GuildName: obj.Name,
					KillNum:   uint32(obj.Score),
					Rank:      rankIndex,
				},
				Rewards: rankConf.Rewards,
			}
			for playerId := range g.Members {
				if engine.IsRobot(playerId) {
					continue
				}
				mailmgr.SendMailToActor(playerId, mailSt)
			}
		}
	}
}

func (yy *YYBestGuild) OnEnd() {
	yy.rankCalc()
	yy.targetCalc()
}

func (yy *YYBestGuild) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.BestGuild {
		return
	}
	delete(globalVar.YyDatas.BestGuild, yy.GetId())
}

func (yy *YYBestGuild) PlayerLogin(player iface.IPlayer) {
	yy.sendPlayerInfo(player)
	yy.sendGData(player)
}

func (yy *YYBestGuild) PlayerReconnect(player iface.IPlayer) {
	yy.sendPlayerInfo(player)
	yy.sendGData(player)
}

func (yy *YYBestGuild) getData() *pb3.YYBestGuild {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.BestGuild == nil {
		globalVar.YyDatas.BestGuild = make(map[uint32]*pb3.YYBestGuild)
	}

	tmp, ok := globalVar.YyDatas.BestGuild[yy.Id]
	if !ok {
		tmp = new(pb3.YYBestGuild)
		globalVar.YyDatas.BestGuild[yy.Id] = tmp
	}

	if nil == tmp.PlayerData {
		tmp.PlayerData = make(map[uint64]*pb3.BestGuildPlayerData)
	}

	if nil == tmp.GuildScores {
		tmp.GuildScores = make(map[uint64]*pb3.BestGuildData)
	}

	return tmp
}

func (yy *YYBestGuild) getPlayerData(playerId uint64) *pb3.BestGuildPlayerData {
	actData := yy.getData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.BestGuildPlayerData{}
	}
	return actData.PlayerData[playerId]
}

func (yy *YYBestGuild) getGData(guildId uint64) (*pb3.BestGuildData, bool) {
	if guildId == 0 {
		return nil, false
	}

	actData := yy.getData()
	if _, ok := actData.GuildScores[guildId]; !ok {
		actData.GuildScores[guildId] = &pb3.BestGuildData{}
	}
	if nil == actData.GuildScores[guildId].Kills {
		actData.GuildScores[guildId].Kills = make(map[uint32]uint32)
	}
	return actData.GuildScores[guildId], true
}

func (yy *YYBestGuild) getGuildScore(guildId uint64) int64 {
	gData, ok := yy.getGData(guildId)
	if !ok {
		return 0
	}
	return gData.Score
}

func (yy *YYBestGuild) getGuildTargetCount(guildId uint64, id uint32) uint32 {
	gData, ok := yy.getGData(guildId)
	if !ok {
		return 0
	}
	return gData.Kills[id]
}

func (yy *YYBestGuild) sendPlayerInfo(player iface.IPlayer) {
	player.SendProto3(75, 110, &pb3.S2C_75_110{
		ActiveId:   yy.Id,
		Data:       yy.getPlayerData(player.GetId()),
		GuildScore: yy.getGuildScore(player.GetGuildId()),
	})
}

func (yy *YYBestGuild) GetConf() *jsondata.YYBestGuildConfig {
	return jsondata.GetYYBestGuildConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYBestGuild) GetRealRank(conf *jsondata.YYBestGuildConfig, rank *pb3.BestGuildRankInfo) {
	for _, v := range conf.Ranks {
		if v.Max < rank.Rank || v.Min > rank.Rank {
			continue
		}
		if v.MinScore > rank.Score {
			rank.Rank = v.Max + 1
			continue
		}
		break
	}
}

func (yy *YYBestGuild) c2sRank(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_111
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := yy.GetConf()
	if nil == conf {
		return neterror.ParamsInvalidError("YYBestGuild rank conf is nil")
	}

	rsp := &pb3.S2C_75_111{
		ActiveId: yy.Id,
	}

	var rankIndex uint32
	for _, v := range yy.rank.GetList(1, int(conf.ShowRank)) {
		g := guildmgr.GetGuildById(v.Id)
		if nil == g {
			continue
		}

		rankIndex++

		obj := &pb3.BestGuildRankInfo{
			GuildId: v.Id,
			Name:    g.GetName(),
			Rank:    rankIndex,
			Score:   v.Score,
		}

		yy.GetRealRank(conf, obj)

		rankIndex = obj.Rank
		rsp.RankList = append(rsp.RankList, obj)
	}

	rsp.MyGuildScore = yy.getGuildScore(player.GetGuildId())

	player.SendProto3(75, 111, rsp)
	return nil
}

func (yy *YYBestGuild) c2sSumAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_112
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	sumAwardConf := yy.GetConf().GetSumAwardConf(req.Id)
	if nil == sumAwardConf {
		return neterror.ParamsInvalidError("sumAwardConf is nil")
	}

	score := yy.getGuildTargetCount(player.GetGuildId(), sumAwardConf.Id)
	pData := yy.getPlayerData(player.GetId())

	if score < sumAwardConf.Count {
		player.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if pie.Uint32s(pData.RevIds).Contains(req.Id) {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	pData.RevIds = append(pData.RevIds, req.Id)
	engine.GiveRewards(player, sumAwardConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogYYBestGuildSumAwards,
	})

	player.SendProto3(75, 112, &pb3.S2C_75_112{
		ActiveId: yy.Id,
		Id:       req.Id,
	})

	return nil
}

func (yy *YYBestGuild) c2sGData(player iface.IPlayer, msg *base.Message) error {
	yy.sendGData(player)
	return nil
}

func (yy *YYBestGuild) sendGData(player iface.IPlayer) {
	data := yy.getData()
	player.SendProto3(75, 113, &pb3.S2C_75_113{
		ActiveId: yy.Id,
		GData:    data.GuildScores[player.GetGuildId()],
	})
}

func (yy *YYBestGuild) clearRank(guildId uint64) {
	data := yy.getData()
	if data.IsCalc {
		return
	}

	delete(data.GuildScores, guildId)
	yy.updateRank(guildId, 0)
}

func (yy *YYBestGuild) updateRank(guildId uint64, score int64) {
	if yy.rank == nil {
		return
	}

	yy.rank.Update(guildId, score)
}

func (yy *YYBestGuild) handleKillMon(player iface.IPlayer, monId, sceneId, lv, count uint32) {
	guildId := player.GetGuildId()
	if guildId == 0 {
		return
	}

	g := guildmgr.GetGuildById(guildId)
	if nil == g {
		return
	}

	mConf := jsondata.GetMonsterConf(monId)
	if nil == mConf {
		return
	}

	conf := yy.GetConf()
	if nil == conf {
		return
	}

	if mConf.Type != custom_id.MtBoss {
		return
	}

	if !pie.Uint32s(conf.BossType).Contains(mConf.SubType) {
		return
	}

	if mConf.SubType == custom_id.MstWorldBoss {
		lConf := jsondata.GetWorldBossLayerConfigBySceneId(sceneId)
		if lConf == nil || !lConf.NeedDeductTimes {
			return
		}
	}

	gData, ok := yy.getGData(guildId)
	if !ok {
		return
	}

	data := yy.getData()

	if !data.IsCalc {
		gData.Score += int64(count)
		yy.updateRank(guildId, gData.Score)
	}

	var bro bool
	for _, v := range conf.SumAwards {
		if v.MonsterLv > lv {
			continue
		}
		oldCount := gData.Kills[v.Id]
		newCount := gData.Kills[v.Id] + count
		gData.Kills[v.Id] = newCount
		if !bro {
			bro = oldCount < v.Count && newCount >= v.Count
		}
	}

	if bro {
		g.BroadcastProto(75, 113, &pb3.S2C_75_113{
			ActiveId: yy.Id,
			GData:    gData,
		})
	}
}

func (yy *YYBestGuild) NewDay() {}

func (yy *YYBestGuild) PackToProto() []*pb3.OneRankItem {
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

func (yy *YYBestGuild) onHourArrive(hour int) {
	endTime := yy.GetEndTime()
	nowSec := time_util.NowSec()
	if !time_util.IsSameDay(endTime-1, nowSec) {
		return
	}
	conf := yy.GetConf()
	if nil == conf {
		yy.LogError("conf not found")
		return
	}

	// 0 表示活动结束结算
	if conf.SettlementHour == 0 {
		return
	}
	if conf.SettlementHour != uint32(hour) {
		yy.LogError("%d != %d ", conf.SettlementHour, hour)
		return
	}
	yy.rankCalc()
}

func (yy *YYBestGuild) ServerStopSaveData() {
	yy.getData().Ranks = yy.PackToProto()
}

func handleYYBestGuildAeKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 5 {
		return
	}
	monId, ok := args[0].(uint32)
	if !ok {
		return
	}
	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}
	count, ok := args[2].(uint32)
	if !ok {
		return
	}
	lv, ok := args[4].(uint32)
	if !ok {
		return
	}

	allYYBestGuildDo(func(yy *YYBestGuild) {
		yy.handleKillMon(player, monId, sceneId, lv, count)
	})
}

func handleYYBestGuildJoinGuild(player iface.IPlayer, args ...interface{}) {
	allYYBestGuildDo(func(yy *YYBestGuild) {
		yy.sendGData(player)
	})
}

func handleYYBestGuildLeaveGuild(player iface.IPlayer, args ...interface{}) {
	allYYBestGuildDo(func(yy *YYBestGuild) {
		yy.sendGData(player)
	})
}

func handleYYBestGuildGuildDestroy(args ...interface{}) {
	gId, ok := args[0].(uint64)
	if !ok {
		return
	}
	allYYBestGuildDo(func(yy *YYBestGuild) {
		yy.clearRank(gId)
	})
}

func handleYYBestGuildHourArriveHandler(args ...interface{}) {
	hour, ok := args[0].(int)
	if !ok {
		logger.LogStack("hour convert failed")
		return
	}
	allYYBestGuildDo(func(yy *YYBestGuild) {
		yy.onHourArrive(hour)
	})
}

func allYYBestGuildDo(fn func(yy *YYBestGuild)) {
	allYY := yymgr.GetAllYY(yydefine.YYBestGuild)
	for _, iYunYing := range allYY {
		if !iYunYing.IsOpen() {
			continue
		}
		if obj, ok := iYunYing.(*YYBestGuild); ok {
			fn(obj)
		}
	}

}
func init() {
	yymgr.RegisterYYType(yydefine.YYBestGuild, func() iface.IYunYing {
		return &YYBestGuild{}
	})

	event.RegActorEvent(custom_id.AeKillMon, handleYYBestGuildAeKillMon)
	event.RegActorEvent(custom_id.AeJoinGuild, handleYYBestGuildJoinGuild)
	event.RegActorEvent(custom_id.AeLeaveGuild, handleYYBestGuildLeaveGuild)

	event.RegSysEvent(custom_id.SeHourArrive, handleYYBestGuildHourArriveHandler)
	event.RegSysEvent(custom_id.SeGuildDestroy, handleYYBestGuildGuildDestroy)

	net.RegisterGlobalYYSysProto(75, 111, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYBestGuild).c2sRank
	})

	net.RegisterGlobalYYSysProto(75, 112, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYBestGuild).c2sSumAwards
	})

	net.RegisterGlobalYYSysProto(75, 113, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYBestGuild).c2sGData
	})
}
