/**
 * @Author: LvYuMeng
 * @Date: 2025/7/18
 * @Desc: 抽奖排行
**/

package yy

import (
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type YYGachaRank struct {
	YYBase
	rank *base.Rank
}

func (yy *YYGachaRank) OnInit() {
	if !yy.IsOpen() {
		return
	}

	conf := yy.GetConf()
	if nil == conf {
		return
	}

	yy.rank = base.NewRank(conf.GetGachaRankCount())

	data := yy.getData()
	for _, v := range data.Ranks {
		yy.rank.Update(v.Id, v.Score)
	}
}

func (yy *YYGachaRank) OnOpen() {
	yy.Broadcast(75, 100, &pb3.S2C_75_100{
		ActiveId: yy.Id,
		Data:     &pb3.GachaRankPlayerData{},
	})
}

func (yy *YYGachaRank) OnEnd() {
	conf := yy.GetConf()
	if nil == conf {
		return
	}

	data := yy.getData()
	for actorId, pData := range data.PlayerData {
		var rewardVec []jsondata.StdRewardVec
		for _, v := range conf.SumAwards {
			if uint32(pData.Score) < v.Count {
				continue
			}
			if pie.Uint32s(pData.RevIds).Contains(v.Id) {
				continue
			}
			pData.RevIds = append(pData.RevIds, v.Id)
			rewardVec = append(rewardVec, v.Rewards)
		}

		rewards := jsondata.AppendStdReward(rewardVec...)
		if len(rewards) > 0 {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  conf.PersonMailId,
				Rewards: rewards,
			})
		}
	}

	var rankIndex uint32
	for _, v := range yy.rank.GetList(1, yy.rank.GetRankCount()) {
		rankIndex++

		obj := &pb3.GachaRankInfo{
			ActorId: v.Id,
			Rank:    rankIndex,
			Score:   v.Score,
		}

		yy.GetRealRank(conf, obj)

		rankIndex = obj.Rank

		if rankConf := conf.GetGachaRankConf(rankIndex); nil != rankConf {
			mailmgr.SendMailToActor(v.Id, &mailargs.SendMailSt{
				ConfId:  conf.RankMailId,
				Content: &mailargs.RankArgs{Rank: rankIndex},
				Rewards: rankConf.Rewards,
			})
			if obj.Rank == 1 && conf.FirstBroadcastId > 0 {
				if baseData, ok := manager.GetData(obj.ActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
					engine.BroadcastTipMsgById(conf.FirstBroadcastId, obj.ActorId, baseData.Name, engine.StdRewardToBroadcastV2(obj.ActorId, rankConf.Rewards))
				}
			}
		}
	}
}

func (yy *YYGachaRank) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.GachaRankData {
		return
	}
	delete(globalVar.YyDatas.GachaRankData, yy.GetId())
}

func (yy *YYGachaRank) PlayerLogin(player iface.IPlayer) {
	yy.sendPlayerInfo(player)
}

func (yy *YYGachaRank) PlayerReconnect(player iface.IPlayer) {
	yy.sendPlayerInfo(player)
}

func (yy *YYGachaRank) getData() *pb3.YYGachaRankData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.GachaRankData == nil {
		globalVar.YyDatas.GachaRankData = make(map[uint32]*pb3.YYGachaRankData)
	}

	tmp, ok := globalVar.YyDatas.GachaRankData[yy.Id]
	if !ok {
		tmp = new(pb3.YYGachaRankData)
		globalVar.YyDatas.GachaRankData[yy.Id] = tmp
	}

	if nil == tmp.PlayerData {
		tmp.PlayerData = make(map[uint64]*pb3.GachaRankPlayerData)
	}

	return tmp
}

func (yy *YYGachaRank) getPlayerData(playerId uint64) *pb3.GachaRankPlayerData {
	actData := yy.getData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.GachaRankPlayerData{}
	}
	return actData.PlayerData[playerId]
}

func (yy *YYGachaRank) sendPlayerInfo(player iface.IPlayer) {
	player.SendProto3(75, 100, &pb3.S2C_75_100{
		ActiveId: yy.Id,
		Data:     yy.getPlayerData(player.GetId()),
	})
}

func (yy *YYGachaRank) GetConf() *jsondata.GachaRankConfig {
	return jsondata.GetYYGachaRankConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYGachaRank) handleGachaEvent(player iface.IPlayer, event *custom_id.ActDrawEvent) {
	conf := yy.GetConf()
	if nil == conf {
		return
	}

	if conf.ActType > 0 && conf.ActType != event.ActType {
		return
	}

	if conf.ActId > 0 && conf.ActId != event.ActId {
		return
	}

	pData := yy.getPlayerData(player.GetId())
	pData.Score += int64(event.Times)

	yy.rank.Update(player.GetId(), pData.Score)

	player.SendProto3(75, 103, &pb3.S2C_75_103{
		ActiveId: yy.Id,
		Score:    uint32(pData.Score),
	})
}

func (yy *YYGachaRank) GetRealRank(conf *jsondata.GachaRankConfig, rank *pb3.GachaRankInfo) {
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

func (yy *YYGachaRank) c2sRank(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_101
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := yy.GetConf()
	if nil == conf {
		return neterror.ParamsInvalidError("gacha rank conf is nil")
	}

	rsp := &pb3.S2C_75_101{
		ActiveId: yy.Id,
	}

	myId := player.GetId()

	limit := int(conf.ShowRank)
	var rankIndex uint32
	for i, v := range yy.rank.GetList(1, int(conf.GetGachaRankCount())) {
		role, ok := manager.GetData(v.Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			continue
		}

		rankIndex++

		obj := &pb3.GachaRankInfo{
			ActorId: v.Id,
			Name:    role.Name,
			Rank:    rankIndex,
			Score:   v.Score,
		}

		yy.GetRealRank(conf, obj)

		rankIndex = obj.Rank

		if i < limit {
			rsp.RankList = append(rsp.RankList, obj)
		}

		if obj.ActorId == myId {
			rsp.MyRank = obj.Rank
		}
	}

	pData := yy.getPlayerData(player.GetId())
	rsp.MyScore = pData.Score

	player.SendProto3(75, 101, rsp)
	return nil
}

func (yy *YYGachaRank) c2sSumAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_102
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	sumAwardConf := yy.GetConf().GetGachaSumAwardConf(req.Id)
	if nil == sumAwardConf {
		return neterror.ParamsInvalidError("sumAwardConf is nil")
	}

	pData := yy.getPlayerData(player.GetId())

	if pData.Score < int64(sumAwardConf.Count) {
		player.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if pie.Uint32s(pData.RevIds).Contains(req.Id) {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	pData.RevIds = append(pData.RevIds, req.Id)
	engine.GiveRewards(player, sumAwardConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGachaRankSumAwards,
	})

	player.SendProto3(75, 102, &pb3.S2C_75_102{
		ActiveId: yy.Id,
		Id:       req.Id,
	})

	return nil
}

func (yy *YYGachaRank) NewDay() {
	data := yy.getData()
	for _, v := range data.PlayerData {
		v.DailyRev = false
	}
	yy.Broadcast(75, 104, &pb3.S2C_75_104{ActiveId: yy.Id})
}

func (yy *YYGachaRank) c2sDailyAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_104
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := yy.GetConf()
	if nil == conf {
		return neterror.ParamsInvalidError("gacha rank conf is nil")
	}

	pData := yy.getPlayerData(player.GetId())

	if pData.DailyRev {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	pData.DailyRev = true
	engine.GiveRewards(player, conf.DailyAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGachaRankDailyAwards,
	})

	player.SendProto3(75, 104, &pb3.S2C_75_104{
		ActiveId: yy.Id,
		DailyRev: pData.DailyRev,
	})

	return nil
}

func (yy *YYGachaRank) PackToProto() []*pb3.OneRankItem {
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

func (yy *YYGachaRank) ServerStopSaveData() {
	yy.getData().Ranks = yy.PackToProto()
}

func handleGachaEvent(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}

	gachaEvent, ok := args[0].(*custom_id.ActDrawEvent)
	if !ok {
		return
	}

	yyList := yymgr.GetAllYY(yydefine.YYGachaRank)
	for _, obj := range yyList {
		if s, ok := obj.(*YYGachaRank); ok && s.IsOpen() {
			s.handleGachaEvent(player, gachaEvent)
		}
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYGachaRank, func() iface.IYunYing {
		return &YYGachaRank{}
	})

	net.RegisterGlobalYYSysProto(75, 101, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGachaRank).c2sRank
	})

	net.RegisterGlobalYYSysProto(75, 102, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGachaRank).c2sSumAwards
	})

	net.RegisterGlobalYYSysProto(75, 104, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGachaRank).c2sDailyAwards
	})

	event.RegActorEvent(custom_id.AeActDrawTimes, handleGachaEvent)
}
