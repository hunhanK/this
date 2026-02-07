/**
 * @Author: zjj
 * @Date: 2024/5/9
 * @Desc:
**/

package manager

import (
	"fmt"
	log "github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/component"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/db/mysql"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

type CalcPowerRushRankSubTypeHandle func(player iface.IPlayer) (score int64)

var (
	// 计算局部战力方法
	calcPowerRushRankSubTypeHandleMgr = make(map[ranktype.PowerRushRankSubType]CalcPowerRushRankSubTypeHandle)

	// RushRankMgrIns 冲榜排行榜管理器
	RushRankMgrIns map[ranktype.PowerRushRankType]*component.ScoreRank
)

func CheckEndRushRankViewDay() bool {
	defaultDay := uint32(8)
	commonConf := jsondata.GetCommonConf("endRushRankViewDay")
	if commonConf != nil && commonConf.U32 != 0 {
		defaultDay = commonConf.U32
	}
	if defaultDay < gshare.GetOpenServerDay() {
		return false
	}
	return true
}

// LoadRushRankMgr 当场加载
func LoadRushRankMgr() error {
	RushRankMgrIns = make(map[ranktype.PowerRushRankType]*component.ScoreRank)
	rankConf := jsondata.GetAllRushRankConf()
	if rankConf == nil {
		log.LogStack("not found rush rank conf")
	}
	for _, conf := range rankConf {
		RushRankMgrIns[ranktype.PowerRushRankType(conf.Type)] = component.NewScoreRank(
			conf.CommonScoreRank,
			component.WithScoreRankOptionByNoReachMinScoreStopUpdate(),
			component.WithScoreRankOptionByNoReachHistoryScoreStopUpdate(),
			component.WithScoreRankOptionByPacketPlayerInfoToRankInfo(packetPlayerInfoToRankInfo),
		)
	}

	syncMsg := syncmsg.NewSyncMsg()
	gshare.SendDBMsg(custom_id.GMsgSyncLoadRushRankData, syncMsg)
	ret, err := syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}
	lists := ret.([]*mysql.RankList)
	for _, list := range lists {
		rank := RushRankMgrIns[ranktype.PowerRushRankType(list.RankType)]
		if rank == nil {
			log.LogTrace("not found rank %d", list.RankType)
			continue
		}
		rank.InitialUpdate(list.RankData)
	}
	return nil
}

func SaveRushRankMgr(sync bool) error {
	var list []*mysql.RankList
	for typ, ptr := range RushRankMgrIns {
		line := &mysql.RankList{RankType: uint32(typ)}
		line.RankData = ptr.PackToProto()
		list = append(list, line)
	}

	if !sync {
		gshare.SendDBMsg(custom_id.GMsgSaveRushRankData, list)
		return nil
	}

	syncMsg := syncmsg.NewSyncMsg(list)
	gshare.SendDBMsg(custom_id.GMsgSyncSaveRushRankData, syncMsg)
	_, err := syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}
	return nil
}

func RegCalcPowerRushRankSubTypeHandle(subType ranktype.PowerRushRankSubType, f CalcPowerRushRankSubTypeHandle) {
	_, ok := calcPowerRushRankSubTypeHandleMgr[subType]
	if ok {
		panic(fmt.Sprintf("already registered %d", subType))
	}
	calcPowerRushRankSubTypeHandleMgr[subType] = f
}

func TriggerCalcPowerRushRankByType(actor iface.IPlayer, rushRankType ranktype.PowerRushRankType) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}

	// 未设置冲榜类型
	if !utils.IsSetBit(staticVar.CurrentRushRankTypeBit, uint32(rushRankType)) {
		return
	}

	// 已结算
	if utils.IsSetBit(staticVar.RushRankTypeRecAwardsBit, uint32(rushRankType)) {
		return
	}

	types := ranktype.GetPowerRushRankSubTypes(rushRankType)
	if len(types) == 0 {
		log.LogTrace("当前冲榜类型:%d, 找不到子类型统计", rushRankType)
		return
	}

	rank, ok := RushRankMgrIns[rushRankType]
	if !ok {
		return
	}

	var totalPower int64
	for _, subType := range types {
		utils.ProtectRun(func() {
			handle := calcPowerRushRankSubTypeHandleMgr[subType]
			if handle == nil {
				return
			}
			totalPower += handle(actor)
		})
	}

	actorId := actor.GetId()
	if !rank.Update(actorId, totalPower) {
		return
	}

	rankData, err := rank.GetRankData(false)
	if err != nil {
		log.LogError("err:%v", err)
		return
	}

	myRankInfo := rank.GetMyRankInfo(rankData, actorId)
	recordLastRushRankOneItem(myRankInfo, actor, rushRankType)

	engine.Broadcast(chatdef.CIWorld, 0, 65, 0, &pb3.S2C_65_0{
		RushRankType: uint32(rushRankType),
		RankMgr:      rankData,
	}, 0)

	resp := pb3.NewS2C_65_3()
	defer pb3.RealeaseS2C_65_3(resp)
	resp.RushRankType = uint32(rushRankType)
	resp.MyRankInfo = rank.GetMyRankInfo(rankData, actorId)
	actor.SendProto3(65, 3, resp)
}

func recordLastRushRankOneItem(myRankInfo *pb3.RankInfo, player iface.IPlayer, rushRankType ranktype.PowerRushRankType) {
	mgr := player.GetBinaryData().LastRushRankItem
	if mgr == nil {
		player.GetBinaryData().LastRushRankItem = make(map[uint32]*pb3.RankInfo)
		mgr = player.GetBinaryData().LastRushRankItem
	}

	rank := RushRankMgrIns[rushRankType]
	if rank != nil {
		mgr[uint32(rushRankType)] = myRankInfo
	}
}

func c2SGetRushRankData(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_65_1
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return nil
	}

	var rsp = &pb3.S2C_65_1{
		RushRankType: req.RushRankType,
	}

	rank := RushRankMgrIns[ranktype.PowerRushRankType(req.RushRankType)]
	if rank == nil {
		return neterror.ParamsInvalidError("not found %d rank", req.RushRankType)
	}

	rsp.RankMgr, _ = rank.GetRankData(false)
	rsp.MyRankInfo = rank.GetMyRankInfo(rsp.RankMgr, player.GetId())
	player.SendProto3(65, 1, rsp)
	return nil
}

func handlePlayerLogin(player iface.IPlayer) {
	if !CheckEndRushRankViewDay() {
		return
	}
	rankConf := jsondata.GetAllRushRankConf()
	if rankConf == nil {
		return
	}

	mgr := player.GetBinaryData().LastRushRankItem
	if mgr == nil {
		player.GetBinaryData().LastRushRankItem = make(map[uint32]*pb3.RankInfo)
		mgr = player.GetBinaryData().LastRushRankItem
	}

	var rsp = &pb3.S2C_65_2{
		LastRushRankItem: mgr,
	}

	var rankMgr = make(map[uint32]*pb3.RushRankInfo)
	for _, conf := range rankConf {
		v, ok := RushRankMgrIns[ranktype.PowerRushRankType(conf.Type)]
		if !ok {
			continue
		}
		data, _ := v.GetRankData(false)
		rankMgr[conf.Type] = &pb3.RushRankInfo{RankMgr: data}
	}

	rsp.RankMgr = rankMgr
	player.SendProto3(65, 2, rsp)

	s2cRushRankDailyReward(player)
}

// 跨天
func handleRushRankNewDayArrive(_ ...interface{}) {
	serverDay := gshare.GetOpenServerDay()
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}
	bit := staticVar.CurrentRushRankTypeBit
	for _, conf := range jsondata.GetAllRushRankConf() {
		if conf.OpenDay > serverDay && utils.IsSetBit(bit, conf.Type) {
			bit = utils.ClearBit(bit, conf.Type)
			continue
		}
		if conf.EndDay < serverDay && utils.IsSetBit(bit, conf.Type) {
			bit = utils.ClearBit(bit, conf.Type)
			continue
		}
		bit = utils.SetBit(bit, conf.Type)
	}
	staticVar.CurrentRushRankTypeBit = bit
}

func handleRushRankLogout(player iface.IPlayer, _ ...interface{}) {
	// 超过了查看天数就不用记录了
	if !CheckEndRushRankViewDay() {
		return
	}
	rankConf := jsondata.GetAllRushRankConf()
	if rankConf == nil {
		log.LogStack("not found rush rank conf")
	}
	for _, conf := range rankConf {
		v, ok := RushRankMgrIns[ranktype.PowerRushRankType(conf.Type)]
		if !ok {
			continue
		}
		data, _ := v.GetRankData(false)
		info := v.GetMyRankInfo(data, player.GetId())
		recordLastRushRankOneItem(info, player, ranktype.PowerRushRankType(conf.Type))
	}
}
func getData(player iface.IPlayer) *pb3.RushRankDailyRewardData {
	data := player.GetBinaryData()
	if data.RushRankDailyRewardData == nil {
		data.RushRankDailyRewardData = &pb3.RushRankDailyRewardData{}
	}
	if data.RushRankDailyRewardData.DailyReward == nil {
		data.RushRankDailyRewardData.DailyReward = make(map[uint32]uint32)
	}
	return data.RushRankDailyRewardData
}
func s2cRushRankDailyReward(player iface.IPlayer, args ...interface{}) {
	data := getData(player)
	var revTypes []uint32
	for k, v := range data.DailyReward {
		if v > 0 {
			revTypes = append(revTypes, k)
		}
	}
	player.SendProto3(65, 5, &pb3.S2C_65_5{
		RevrushRankType: revTypes,
	})
}

func handleNewDay(player iface.IPlayer, args ...interface{}) {
	data := getData(player)
	for k := range data.DailyReward {
		if data.DailyReward[k] > 0 {
			data.DailyReward[k] = 0
		}
	}
}

func c2sReward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_65_4
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return nil
	}
	if !CheckEndRushRankViewDay() {
		return neterror.ParamsInvalidError("rank end")
	}
	rankType := req.RushRankType
	sData := getData(player)
	if sData.DailyReward[rankType] != 0 {
		return neterror.ParamsInvalidError("today reward rev")
	}
	rankConf := jsondata.GetAllRushRankConf()
	if rankConf == nil {
		return neterror.ParamsInvalidError("conf is nil")
	}
	_, ok := rankConf[rankType]
	if !ok {
		return neterror.ParamsInvalidError("%d this rank not exist", req.RushRankType)
	}
	rewards := rankConf[rankType].DayRewards
	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogRushRankDailyAwards,
		})
	}
	sData.DailyReward[rankType] = uint32(time_util.Now().Unix())
	player.SendProto3(65, 4, &pb3.S2C_65_4{
		RushRankType: req.RushRankType,
	})
	return nil
}
func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		err := LoadRushRankMgr()
		if err != nil {
			log.LogError("Load RushRankMgr err:%v", err)
			return
		}
		handleRushRankNewDayArrive()
		event.TriggerSysEvent(custom_id.SeAfterRushRankInit)
	})
	event.RegSysEvent(custom_id.SeNewDayArrive, handleRushRankNewDayArrive)
	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		handlePlayerLogin(player)
	})
	event.RegActorEvent(custom_id.AeLogout, handleRushRankLogout)
	event.RegActorEvent(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		handlePlayerLogin(player)
	})
	net.RegisterProto(65, 1, c2SGetRushRankData)
	net.RegisterProto(65, 4, c2sReward)
	event.RegActorEvent(custom_id.AeAfterUpdateSysPowerMap, handleRushRankOnUpdateSysPowerMap)
	event.RegActorEvent(custom_id.AeFightValueChange, handleRushRankOnFightValueChange)
	event.RegActorEvent(custom_id.AeNewDay, handleNewDay)
}

// 战力更新时直接触发计算
func handleRushRankOnUpdateSysPowerMap(player iface.IPlayer, _ ...interface{}) {
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeEquip)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFourSymbolsDragon)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFourSymbolsTiger)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFourSymbolsRoseFinch)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFourSymbolsTortoise)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFaBao)
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeFairy)
}

func handleRushRankOnFightValueChange(player iface.IPlayer, _ ...interface{}) {
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeDailyUpPower)
}

func ExportPacketPlayerInfoToRankInfo(playerId uint64, info *pb3.RankInfo) {
	packetPlayerInfoToRankInfo(playerId, info)
}
