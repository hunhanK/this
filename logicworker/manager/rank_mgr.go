package manager

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"time"

	"github.com/gzjjyz/srvlib/utils"

	"jjyz/base/functional"

	"github.com/gzjjyz/logger"
)

var GRankMgrIns = newRankMgr()

// RankMgr 排行榜管理器
type RankMgr struct {
	RankList    map[uint32]*base.Rank
	RankFirstLs map[uint32]uint64 // 排行榜类型 第一名的玩家ID
}

const NormalRankLimit uint32 = 30
const ValidTimeSec = uint32(60 * 60 * 24 * 365 * 5) // 5年时间

func newRankMgr() *RankMgr {
	instance := new(RankMgr)
	instance.RankList = make(map[uint32]*base.Rank)
	for i := gshare.RankTypePower; i < gshare.RankTypeMax; i++ {
		rank := new(base.Rank)
		limit := NormalRankLimit
		rank.Init(limit)
		if i == gshare.RankTypeFlyUpRoad { //登仙榜 需要多统计一些
			rank.Init(1000)
		}
		instance.RankList[uint32(i)] = rank
	}
	return instance
}

// 优化排行榜加载 改为当场加载
func (ins *RankMgr) LoadRank() error {
	loadRankList(nil)
	return nil
}

func loadRankList(args ...interface{}) {
	ret, err := db.OrmEngine.QueryString("call loadRankList(?)", 0)
	if nil != err {
		logger.LogError("loadRankList error. %v", err)
		return
	} else {
		list := make([]*mysql.RankList, 0, len(ret))
		for _, line := range ret {
			rankType := utils.AtoUint32(line["rank_type"])
			rankData := []byte(line["rank_data"])
			rankList := &pb3.OneRankList{}
			if err := pb3.Unmarshal(rankData, rankList); nil != err {
				logger.LogError("load rank data error! rankType=%d, err:%v", rankType, err)
				continue
			}

			list = append(list, &mysql.RankList{
				RankType: rankType,
				RankData: rankList.RankInfo,
			})
		}
		onLoadRankDataRet(list)
	}
	event.TriggerSysEvent(custom_id.SeSavedRankListLoaded)
}

func onLoadRankDataRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadRankDataRet", 1, len(args)) {
		return
	}
	list, ok := args[0].([]*mysql.RankList)
	if !ok {
		return
	}
	for _, line := range list {
		if rank := GRankMgrIns.GetRankByType(line.RankType); nil != rank {
			for _, line2 := range line.RankData {
				rank.Update(line2.Id, line2.Score)
			}
		}
	}

	// event.TriggerSysEvent(custom_id.SeRankLoaded, nil)
}

// SaveRank 保存排行榜数据  // 定时执行
func (ins *RankMgr) SaveRank() {
	list := make([]*mysql.RankList, 0, gshare.RankTypeMax)
	for t := gshare.RankTypePower; t < gshare.RankTypeMax; t++ {
		ptr := ins.GetRankByType(uint32(t))
		if nil == ptr {
			continue
		}
		limit := ptr.GetRankLimit()
		line := &mysql.RankList{RankType: uint32(t)}
		line.RankData = make([]*pb3.OneRankItem, 0, limit)
		for _, line2 := range ptr.GetList(1, int(limit)) {
			line.RankData = append(line.RankData, &pb3.OneRankItem{
				Id:    line2.GetId(),
				Score: line2.GetScore(),
			})
		}
		list = append(list, line)
	}
	saveRankList(list)
}

func (ins *RankMgr) SaveRankByType(t uint32) {
	var list []*mysql.RankList
	ptr := ins.GetRankByType(t)
	if nil == ptr {
		return
	}
	limit := ptr.GetRankLimit()
	line := &mysql.RankList{RankType: t}
	line.RankData = make([]*pb3.OneRankItem, 0, limit)
	for _, line2 := range ptr.GetList(1, int(limit)) {
		line.RankData = append(line.RankData, &pb3.OneRankItem{
			Id:    line2.GetId(),
			Score: line2.GetScore(),
		})
	}
	list = append(list, line)
	saveRankList(list)
}

func saveRankList(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveRankList", 1, len(args)) {
		return
	}
	list, ok := args[0].([]*mysql.RankList)
	if !ok {
		return
	}
	for _, line := range list {
		blob, err := pb3.Marshal(&pb3.OneRankList{RankInfo: line.RankData})
		if nil != err {
			logger.LogError("marshal rankdata error! rankType=%d err:%v", line.RankType, err)
			continue
		}
		if _, err := db.OrmEngine.Exec("call updateRank(?,?)", line.RankType, blob); nil != err {
			logger.LogError("updateRank error! rankType=%d, err:%v", line.RankType, err)
			continue
		}
	}
}

// GetRankByType 根据类型获取排行指针
func (ins *RankMgr) GetRankByType(rType uint32) *base.Rank {
	if rType < gshare.RankTypePower || rType >= gshare.RankTypeMax {
		return nil
	}
	if rank, ok := ins.RankList[rType]; ok {
		return rank
	}

	return nil
}

func (ins *RankMgr) GetAverageScoreByInterval(rankType uint32, left, right int) int64 {
	rank := ins.GetRankByType(rankType)
	if nil == rank {
		return 0
	}

	rankList := rank.GetList(left, right)
	var average int64

	if len(rankList) > 0 {
		for _, v := range rankList {
			average += v.Score
		}
		average = average / int64(len(rankList))
	}

	return average
}

// UpdateRank 更新排行数据
func (ins *RankMgr) UpdateRank(rType uint32, id uint64, score int64) {
	// 检查类型是否合法
	rankConf := jsondata.GetRankConf(rType)
	if rankConf == nil {
		logger.LogWarn("rType:%d not found conf", rType)
		return
	}
	if score > 0 && int64(rankConf.MinPower) > score {
		return
	}
	rank := ins.GetRankByType(rType)
	if nil == rank {
		return
	}
	ret := rank.Update(id, score)
	if !ret {
		return
	}
	// logger.LogDebug("id:%d score:%d rank:%d", id, score, rank.GetRankById(id))
}

// 排行榜收到点赞
func (ins *RankMgr) SendRankLike(tarPlayerId uint64, rType uint32) bool {
	rank := ins.GetRankByType(rType)
	if nil == rank {
		return false
	}
	if !rank.IsInScoreMap(tarPlayerId) { // 在榜上才能接受点赞
		return false
	}

	//不显示的榜，直接跳过
	conf := jsondata.GetRankConf(rType)
	if conf == nil {
		return false
	}

	rankLikeMap := gshare.GetStaticVar().GetRankLikeLs()
	if nil == rankLikeMap {
		gshare.GetStaticVar().RankLikeLs = make(map[uint64]uint32, conf.ShowMaxLimit)
		rankLikeMap = gshare.GetStaticVar().RankLikeLs
	}
	currLike := rankLikeMap[tarPlayerId]
	currLike += 1
	rankLikeMap[tarPlayerId] = currLike
	return true
}

func LoadWorldLevel() bool {
	lastRefreshTime := time.Unix(int64(gshare.GetStaticVar().WorldLevelRefreshTime), 0)
	formatStr := "2022-01-02"

	if lastRefreshTime.Format(formatStr) == time.Now().Format(formatStr) {
		gshare.SetWorldLevel(gshare.GetWorldLevel())
		return true
	}

	defer func() {
		gshare.GetStaticVar().WorldLevelRefreshTime = int32(time.Now().Unix())
	}()

	foo := jsondata.GetCommonConf("worldLevelParam")
	if foo == nil {
		gshare.SetWorldLevel(0)
		return true
	}

	topLists := GRankMgrIns.GetRankByType(gshare.RankTypeLevel).GetList(1, int(foo.U32))

	add := func(val1 int64, val2 *pb3.OneRankItem) int64 {
		circle := val2.Score >> 32
		circle = circle << 32

		level := val2.Score ^ circle
		return val1 + level
	}

	sum := functional.Reduce(topLists, add, 0)

	meanLv := int64(0)
	if len(topLists) != 0 {
		meanLv = sum / int64(len(topLists))
	}

	gshare.SetWorldLevel(uint32(meanLv))

	return true
}

func LoadTopFight() bool {
	lastRefreshTime := time.Unix(int64(gshare.GetStaticVar().TopfightRefreshTime), 0)
	formatStr := "2022-01-02"

	if lastRefreshTime.Format(formatStr) == time.Now().Format(formatStr) {
		gshare.SetTopFight(gshare.GetTopFight())
		return true
	}

	foo := jsondata.GetCommonConf("worldLevelParam")
	if foo == nil {
		gshare.SetTopFight(0)
		return true
	}

	topLists := GRankMgrIns.GetRankByType(gshare.RankTypePower).GetList(1, int(foo.U32))

	validTopList := functional.Filter(topLists, func(val *pb3.OneRankItem) bool {
		return IsActorActive(val.Id)
	})

	add := func(val1 int64, val2 *pb3.OneRankItem) int64 {
		return val1 + val2.Score
	}

	sum := functional.Reduce(validTopList, add, 0)

	meanLv := int64(0)
	if len(validTopList) != 0 {
		meanLv = sum / int64(len(validTopList))
	}

	gshare.SetTopFight(meanLv)
	return true
}

func LoadTopLevel() bool {
	score := GRankMgrIns.GetRankByType(gshare.RankTypeLevel).Score(1)

	circle := score >> 32
	circle = circle << 32
	topLevel := score ^ circle

	gshare.SetTopLevel(uint32(topLevel))

	return true
}

func refreshWorldLevel() (ret bool) {
	ret = LoadWorldLevel()
	if !ret {
		return
	}
	event.TriggerSysEvent(custom_id.SeRefreshWorldLevel)
	return
}

func refreshTopFight() (ret bool) {
	ret = LoadTopFight()
	if !ret {
		return
	}
	event.TriggerSysEvent(custom_id.SeRefreshTopFight)
	return
}

func refreshTopLevel() (ret bool) {
	ret = LoadTopLevel()
	if !ret {
		return
	}
	event.TriggerSysEvent(custom_id.SeRefreshTopLevel)
	return
}

// 仙盟战力变化
func onGuildLevelPowerChange(args ...interface{}) {
	if !gcommon.CheckArgsCount("onGuildPowerChange", 1, len(args)) {
		return
	}
	commonSt, ok := args[0].(*pb3.CommonSt)
	if !ok {
		return
	}
	guildId := commonSt.U64Param
	lv := commonSt.U32Param
	power := commonSt.U64Param2

	limit := uint64(1<<56 - 1)
	if power > limit {
		power = limit
		logger.LogError("rank guild power exceed")
	}

	score := int64(lv)<<56 | int64(power)
	GRankMgrIns.UpdateRank(gshare.RankTypeGuild, guildId, score)
}

// 星河图层数变化
// Deprecated
func onStarRiverLayerChange(player iface.IPlayer, _ ...interface{}) {
	binary := player.GetBinaryData()
	if sr := binary.GetStartRiver(); nil != sr {
		player.SetRankValue(gshare.RankTypeStarRiver, int64(sr.Layer))
		GRankMgrIns.UpdateRank(gshare.RankTypeStarRiver, player.GetId(), int64(sr.Layer))
	}
}

// 玩家等级变化
func actorLevelChange(player iface.IPlayer, _ ...interface{}) {
	level := player.GetLevel()
	player.SetRankValue(gshare.RankTypeLevel, int64(level))
	GRankMgrIns.UpdateRank(gshare.RankTypeLevel, player.GetId(), int64(level))
	TriggerCalcPowerRushRankByType(player, ranktype.PowerRushRankTypeLevel)
}

func actorFairyMasterTrainLevelChange(player iface.IPlayer, args ...interface{}) {
	binary := player.GetBinaryData()
	if binary.FairyMasterData == nil {
		return
	}

	player.SetRankValue(gshare.RankTypeFairyMasterTrain, int64(binary.FairyMasterData.Level))
	GRankMgrIns.UpdateRank(gshare.RankTypeFairyMasterTrain, player.GetId(), int64(binary.FairyMasterData.Level))
}

func actorBeastRampantDiffChange(player iface.IPlayer, _ ...interface{}) {
	binary := player.GetBinaryData()
	player.SetRankValue(gshare.RankTypeBeastRampant, int64(binary.BeastRampantDiff))
	GRankMgrIns.UpdateRank(gshare.RankTypeBeastRampant, player.GetId(), int64(binary.BeastRampantDiff))
}

func actorFlyCampFinish(player iface.IPlayer, _ ...interface{}) {
	binary := player.GetBinaryData()
	if binary.FlyCampData == nil {
		return
	}
	finishTime := getBaseTimeAt() - binary.FlyCampData.FinishTime
	player.SetRankValue(gshare.RankTypeFlyCamp, int64(finishTime))
	GRankMgrIns.UpdateRank(gshare.RankTypeFlyCamp, player.GetId(), int64(finishTime))
}

func actorAncientTowerLayer(player iface.IPlayer, _ ...interface{}) {
	ancientData := player.GetBinaryData().AncientTowerData
	player.SetRankValue(gshare.RankAncientTower, int64(ancientData.Layer))
	GRankMgrIns.UpdateRank(gshare.RankAncientTower, player.GetId(), int64(ancientData.Layer))
}

func sendRanking(player iface.IPlayer, rType uint32) {
	if nil == player {
		return
	}
	conf := jsondata.GetRankConf(rType)
	if nil == conf {
		return
	}
	rank := GRankMgrIns.GetRankByType(rType)
	if nil == rank {
		return
	}

	rsp := &pb3.S2C_2_64{}
	rsp.RankType = rType

	rankLine := rank.GetList(1, int(conf.ShowMaxLimit))

	rsp.RankData = make([]*pb3.RankInfo, 0, conf.ShowMaxLimit)
	rankLikeMap := gshare.GetStaticVar().GetRankLikeLs()
	if nil == rankLikeMap {
		gshare.GetStaticVar().RankLikeLs = make(map[uint64]uint32, conf.ShowMaxLimit)
	}
	var cur uint32 = 1
	var isOnRank bool
	for _, line := range rankLine { // 获取最新的点赞数量
		keyId := line.GetId()
		likeNum := rankLikeMap[keyId]
		info := &pb3.RankInfo{}
		gLeaderId := keyId
		if rType == gshare.RankTypeGuild {
			if gsys, ok := player.GetSysObj(sysdef.SiGuild).(iface.IGuildSys); ok {
				guild := gsys.GetGuildBasicById(keyId)
				info.GuildName = guild.GetName()
				gLeaderId = guild.LeaderId
			}
		}

		val := line.GetScore()
		if rType == gshare.RankTypeFlyUpRoad {
			val = int64(gshare.GetFlyUpRoadBastTimeAt()) - val
		} else if rType == gshare.RankTypeFlyCamp {
			val = int64(getBaseTimeAt()) - val
		}
		info.Value = val
		info.Rank = cur
		info.Key = line.GetId()
		info.Like = likeNum
		info.ExtVal = DecodeExVal(rType, val)

		SetPlayerRankInfo(gLeaderId, info, rType)

		if player.GetId() == keyId { // 我在榜上
			rsp.MyRankData = info
			isOnRank = true
		}

		rsp.RankData = append(rsp.RankData, info)

		cur++
	}
	if !isOnRank { // 不在榜上
		playerId := player.GetId()
		selfInfo := &pb3.RankInfo{}
		selfInfo.Value = player.GetRankValue(rType)
		selfInfo.ExtVal = DecodeExVal(rType, selfInfo.Value)
		SetPlayerRankInfo(playerId, selfInfo, rType)
		rsp.MyRankData = selfInfo
	}
	// 自己剩余点赞数
	rsp.MyLikeNum = gshare.RankLikeDailyNumMax - player.GetBinaryData().GetPlayerRankDailyLikeNum()

	// 排行榜总人数
	rsp.Total = uint32(rank.GetRankCount())
	player.SendProto3(2, 64, rsp)
}

// 部分排行榜解码
func DecodeExVal(rType uint32, val int64) []int64 {
	var exVal []int64
	switch rType {
	case gshare.RankTypeGuild:
		lv := val >> 56
		power := val & (1<<56 - 1)
		exVal = append(exVal, lv, power)
	}
	return exVal
}

// 请求排行榜信息
func c2sReqRanking(actor iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_64
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return nil
	}
	sendRanking(actor, req.GetRankType())
	return nil
}

// 点赞
func c2sSendLike(actor iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_66
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return nil
	}
	if nil == actor {
		return nil
	}

	rType := req.RankType
	tarId := req.TargetId
	// 不能给自己点赞
	if rType != gshare.RankTypeGuild && actor.GetId() == tarId {
		actor.SendTipMsg(tipmsgid.TpRoleRankCanNotLikeSelf)
		return nil
	}
	selfNum := actor.GetBinaryData().PlayerRankDailyLikeNum
	if selfNum >= gshare.RankLikeDailyNumMax {
		// 点赞达到每日上限
		actor.SendTipMsg(tipmsgid.TpRoleRankLikeNumIsZero)
		return nil
	}

	isSuccess := GRankMgrIns.SendRankLike(tarId, rType)
	if isSuccess {
		selfNum += 1
		actor.GetBinaryData().PlayerRankDailyLikeNum = selfNum
		rsp := &pb3.S2C_2_66{
			MyLikeNum: gshare.RankLikeDailyNumMax - selfNum,
			RankType:  rType,
			TargetId:  tarId,
		}
		actor.SendProto3(2, 66, rsp)
	} else {
		// 错误码
		actor.SendTipMsg(tipmsgid.TpRoleRankLikeMiss)
	}
	return nil
}

func onPlayerRecoverySucc(args ...interface{}) {
	/*
		if !remotecall.CheckArgsCount("onPlayerRecoverySucc", 1, len(args)) {
			return
		}
		playerId, ok := args[0].(uint64)
		if !ok {
			return
		}

		for t := gshare.RankTypePower; t <= gshare.RankTypeMax; t++ {
		}
	*/
}

func onCheckRankPlayerRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onCheckRankPlayerRet", 2, len(args)) {
		return
	}
	rt, ok := args[0].(int)
	if !ok {
		return
	}
	actorIds, ok := args[1].([]uint64)
	if !ok {
		return
	}
	rank := GRankMgrIns.GetRankByType(uint32(rt))
	for _, id := range actorIds {
		rank.Update(id, 0) // 设置为0  让它删除
	}
}

// 玩家战力变化
func onPlayerFightChange(player iface.IPlayer, args ...interface{}) {
	fight, ok := args[0].(int64)
	if !ok {
		logger.LogError("onPlayerFightChange not int64")
		return
	}
	GRankMgrIns.UpdateRank(gshare.RankTypePower, player.GetId(), fight)
}

// 套装等级变化
func onPlayerStrongSuitChange(player iface.IPlayer, args ...interface{}) {
}

// 玩家上线 判断上次下线时间是否跨天  跨天重置每日点赞次数
func onPlayerLogin(player iface.IPlayer, args ...interface{}) {
	lastLoginOut := player.GetMainData().LastLogoutTime
	Now := time_util.NowSec()
	if !time_util.IsSameDay(lastLoginOut, Now) { // 跨天了 重置
		player.GetBinaryData().PlayerRankDailyLikeNum = 0
	}
}

// 获取基础时间
func getBaseTimeAt() uint32 {
	return gshare.GetOpenServerTime() + ValidTimeSec
}

// 清除排行榜
func GmClearRank(actor iface.IPlayer, args ...string) bool {
	GRankMgrIns = newRankMgr()
	return true
}

func init() {
	event.RegActorEvent(custom_id.AeLevelDown, actorLevelChange)
	event.RegActorEvent(custom_id.AeLevelUp, actorLevelChange)
	event.RegActorEvent(custom_id.AeCircleChange, actorLevelChange)
	event.RegActorEvent(custom_id.AeFightValueChange, onPlayerFightChange)
	event.RegActorEvent(custom_id.AeEquipStrongSuit, onPlayerStrongSuitChange)
	event.RegActorEvent(custom_id.AeLogin, onPlayerLogin)
	event.RegActorEvent(custom_id.AeStarRiverLayerChange, onStarRiverLayerChange)
	event.RegActorEvent(custom_id.AeFairyMasterTrainLevelChanged, actorFairyMasterTrainLevelChange)
	event.RegActorEvent(custom_id.AeBeastRampantDiffChange, actorBeastRampantDiffChange)
	event.RegActorEvent(custom_id.AeFlyCampChange, actorFlyCampFinish)
	event.RegActorEvent(custom_id.AeAncientTowerLayer, actorAncientTowerLayer)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		player.GetBinaryData().PlayerRankDailyLikeNum = 0
	})
	event.RegSysEvent(custom_id.GuildLevelPowerChange, onGuildLevelPowerChange)
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgCheckRankRet, onCheckRankPlayerRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgRecoveryPlayer, onPlayerRecoverySucc)
	})

	event.RegSysEvent(custom_id.SeSavedRankListLoaded, func(args ...interface{}) {
		LoadWorldLevel()
		LoadTopFight()
		LoadTopLevel()
	})

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		refreshWorldLevel()
		refreshTopFight()
		refreshTopLevel()
	})

	event.RegSysEvent(custom_id.SeHourArrive, func(args ...interface{}) {
		refreshTopLevel()
	})

	gmevent.Register("clear_rank", GmClearRank, 1)
	gmevent.Register("refresh_worldLv", func(player iface.IPlayer, args ...string) bool {
		gshare.GetStaticVar().WorldLevelRefreshTime = 0
		refreshWorldLevel()
		return true
	}, 1)

	net.RegisterProto(2, 64, c2sReqRanking)
	net.RegisterProto(2, 66, c2sSendLike)

}

func (ins *RankMgr) DeleteFromRank(ids map[uint64]struct{}) {
	for i := gshare.RankTypePower; i < gshare.RankTypeMax; i++ {
		if ptr := ins.GetRankByType(uint32(i)); nil != ptr {
			for id, _ := range ids {
				if ptr.IsInScoreMap(id) {
					ptr.Update(id, 0)
				}
			}
		}
	}
}
