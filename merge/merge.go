/*
合服流程:
1.停服
2.*备份主服数据库，以免出错!
3.将所有从服的数据库载入到主服所在的数据库, 在主服执行sql文件
4.将所有从服runtime下的globalVar_n和rankFile_n拷贝到主服的runtime下
4.更改后台服务器入口
5.起服, 在主服刷指令: merge 参数1:从服id1, 从服id2, 从服id3
*/

package merge

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"io"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/redisworker/redismid"
	path2 "path"
	"text/template"

	"net/http"
	"os"
)

const path = "runtime/globalVar_%d"

var (
	MasterInSlaves   = errors.New("运营后台配置的从服列表中包含了主服服务器id")
	SlaveRepeated    = errors.New("运营后台配置的从服列表中存在相同的服务器id")
	SlaveDifference  = errors.New("运营后台配置的从服列表与合服数据不一致")
	GlobalLoadFailed = errors.New("加载global var数据失败")
)

func exit(err error) {
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	os.Exit(1)
}

func PostMergeError(mergeError error) {
	pfId := gshare.GameConf.PfId
	pfName := "测试平台"
	content := fmt.Sprintf("合服失败告警：\n平台：%d(%s)\n区服：%v\n错误信息：%s", pfId, pfName, gshare.GameConf.SrvId, mergeError.Error())
	jsonStr := []byte(fmt.Sprintf(`{"msgtype":"text", "text": {"content": "%s", "mentioned_mobile_list":["15869875399", "17670650962","15521470679"]}`, content))

	url := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=7ab28eb5-b592-4478-b2d8-cb897dbb4b02"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.LogError("PostMergeError %s", err)
		exit(mergeError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.LogError("err:%v", err)
	} else {
		if _, err := io.ReadAll(resp.Body); nil != err {
			logger.LogError("err:%v", err)
		}
	}
	exit(mergeError)
}

func GlobalVar(slaves []uint32) bool {
	slaveMap := make(map[uint32]struct{})
	for _, id := range slaves {
		if _, exists := slaveMap[id]; exists {
			PostMergeError(SlaveRepeated)
			return false
		}
		slaveMap[id] = struct{}{}
	}

	mainSrvId := gshare.GameConf.SrvId

	if _, exists := slaveMap[mainSrvId]; exists {
		PostMergeError(MasterInSlaves)
		return false
	}

	// 加载globalVar
	var datas []mysql.GlobalVar
	if err := db.OrmEngine.SQL("call loadAllGlobalVar()").Find(&datas); nil != err {
		logger.LogError("load global error! %v", err)
		PostMergeError(GlobalLoadFailed)
		return false
	}

	// 从服列表
	slaveGlobal := make(map[uint32]*pb3.GlobalVar, 0)
	for _, data := range datas {
		if data.ServerId == mainSrvId {
			continue
		}
		global := &pb3.GlobalVar{}
		if err := pb3.Unmarshal(pb3.UnCompress(data.BinaryData), global); nil != err {
			logger.LogFatal("load global_%d error! %v", data.ServerId, err)
			continue
		}

		slaveGlobal[data.ServerId] = global
	}

	if len(slaveMap) != len(slaveGlobal) {
		PostMergeError(SlaveDifference)
		return false
	}

	for id := range slaveGlobal {
		if _, exists := slaveMap[id]; !exists {
			PostMergeError(SlaveDifference)
			return false
		}
	}

	// 清理部分数据
	cleanMasterGlobal()

	srvList := make([]uint32, 0, len(slaveGlobal))
	for srvId, v := range slaveGlobal {
		if _, err := db.OrmEngine.Exec("call delGlobalVar(?)", srvId); nil != err {
			logger.LogError("del global_%d error! %v", srvId, err)
			continue
		}

		// 合并部分数据
		ToMaster(v)
		srvList = append(srvList, srvId)
		gshare.SendRedisMsg(redismid.DelGameBasic, argsdef.GameBasicData{
			PfId:  engine.GetPfId(), // 合服的平台都是同一个
			SrvId: srvId,
		})
	}

	// 回存主服的globalVar到数据库
	gshare.SaveStaticVar()

	// 读取被删除玩家列表
	gshare.SendDBMsg(custom_id.GMsgLoadDeletaActorIds)

	OnMergeScc(srvList)

	return true
}

func OnMergeScc(mergeSrvList []uint32) {
	sst := gshare.GetStaticVar()
	sst.MergeTimes++
	sst.MergeTimestamp = time_util.NowSec()
	if sst.MergeData == nil {
		sst.MergeData = make(map[uint32]uint32)
	}
	sst.MergeData[sst.MergeTimes] = sst.MergeTimestamp

	sst.SlaveSrvIds = pie.Uint32s(sst.SlaveSrvIds).Append(mergeSrvList...).Unique()
	// 合服时 不会有玩家在线 只处理与服务器相关的业务即可 如果合服还有玩家在家 那么DB会出问题
	event.TriggerSysEvent(custom_id.SeMerge, mergeSrvList)
	SyncMergeInfoToFight()
	SyncSlaveSrvIds2Cross()
	// 广播最新的服务器信息

	logger.LogInfo("此次合服从服列表:%v", mergeSrvList)
	logger.LogInfo("合服次数:%d", sst.MergeTimes)
	logger.LogInfo("合服天数:%d", gshare.GetMergeSrvDay())
}

func SyncMergeInfoToFight() {
	sst := gshare.GetStaticVar()
	msg := &pb3.G2FMerge{
		MergeTimes:      sst.MergeTimes,
		MergeTimestamp:  sst.MergeTimestamp,
		NMergeTimestamp: sst.MergeData,
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FMerge, msg)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func SyncSlaveSrvIds2Cross() {
	sst := gshare.GetStaticVar()
	msg := &pb3.CommonSt{
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
	}
	for _, slaveSrvId := range sst.SlaveSrvIds {
		msg.U64ListParam = append(msg.U64ListParam, uint64(slaveSrvId))
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncSlaveSrvIds2Cross, msg)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func IsSrvFileExist(srvId uint32) bool {
	file := utils.GetCurrentDir() + fmt.Sprintf(path, srvId)

	_, err := os.ReadFile(file)
	if nil != err && os.IsNotExist(err) {
		logger.LogFatal("err:%v", err)
		return false
	}
	return true
}

// 需要清理的全服数据
func cleanMasterGlobal() {
	masterGlobal := gshare.GetStaticVar()
	masterGlobal.BossLootRecord = []*pb3.CommonRecord{}                  // BOSS玩法拾取记录（清除）
	masterGlobal.BossRareLootRecord = []*pb3.CommonRecord{}              // BOSS玩法稀有拾取记录（清除）
	masterGlobal.GodAreaRace = &pb3.GodAreaRace{}                        // 神境争锋（清除）
	masterGlobal.MysteryStoreBuyRecords = []*pb3.MysteryStoreBuyRecord{} // 神秘商店购买记录（清除）
	masterGlobal.PreGuildWarData = &pb3.PreGuildWarData{}
	masterGlobal.FairyDynastyDomainCampRank = &pb3.FairyDynastyDomainCampRankStore{}                 // 仙朝征途-版图 宗门排行榜 （清除）
	masterGlobal.FairyDynastyDomainPlayerDates = make(map[uint64]*pb3.FairyDynastyDomainPlayerStore) // 仙朝征途-版图 玩家数据 （清除）
	masterGlobal.FaBaoDrawRecord = make(map[uint32]*pb3.FaBaoDrawRecord)                             // 法宝抽奖记录（清除）
	masterGlobal.ChangGoldRecord = make(map[uint32]*pb3.RecentChangGoldPlayer)                       // 装备换金最近获得者（清除）
	masterGlobal.LocalInfFairyPlaceInfo = &pb3.LocalInfFairyPlaceInfo{}                              // 本服无极仙宫信息（清除）
	masterGlobal.GuildSecretLeaderId = 0                                                             // 本服仙盟战第一仙盟盟主ID (清除)
	masterGlobal.HistoryActCrossZmWarRankInfos = []*pb3.ActCrossZmWarRankInfo{}                      // 跨服宗门战历史排行信息（清除）
	masterGlobal.WeddingBanquet = &pb3.WeddingBanquet{}                                              // 婚宴（清除）
	masterGlobal.WeddingBanquetReserveData = make(map[uint64]*pb3.WeddingBanquetReserve)             // 婚宴预约列表（清除）
	masterGlobal.DragonGrottoesFundRecords = make(map[uint32]*pb3.DragonGrottoesFundRecords)         // 龙窟基金购买记录 (清除)
	masterGlobal.WholeCityLoveRecord = make(map[uint32]*pb3.WholeCityLoveRecord)                     // 全城热恋记录（清除）
	masterGlobal.ConfessionForLoveRecord = make(map[uint32]*pb3.ConfessionForLoveRecord)             // 爱的表白记录 （清除）
	masterGlobal.TreasureDrawRecord = make(map[uint64]*pb3.TreasureDrawRecord)                       // 藏宝阁抽奖记录 （清除）
	masterGlobal.PyyDatas = &pb3.PYYDatas{}                                                          // 个人运营活动公共记录数据 (清除)
	masterGlobal.DiamondTreasureRecords = []*pb3.ItemGetRecord{}                                     // 寻玉宝库抽奖记录 （清除）
	masterGlobal.LuckyTurntableRecords = []*pb3.ItemGetRecord{}                                      // 福运转盘抽奖记录 （清除）
	masterGlobal.PyyActData = make(map[uint64]*pb3.PYYActData)                                       // 全服个人运营活动数据 （清除）
	masterGlobal.ZeroShopLogList = []*pb3.ZeroShopLog{}                                              // Deprecated 待弃用 零元购日志(清除)
	masterGlobal.TSStore = &pb3.TSStore{}                                                            // 天赏阁(清除)
}

func ToMaster(slaveGlobal *pb3.GlobalVar) {
	masterGlobal := gshare.GetStaticVar()
	// 全服每日充值钻石表  (合并)
	masterGlobal.DailyChargeDiamond = mergeU64I64Map(masterGlobal.DailyChargeDiamond, slaveGlobal.DailyChargeDiamond)
	// 全服每日消耗钻石表   (合并)
	masterGlobal.DailyUseDiamond = mergeU64I64Map(masterGlobal.DailyUseDiamond, slaveGlobal.DailyUseDiamond)
	// 全服每日充值表 (合并)
	masterGlobal.DailyChargeCash = mergeU64I64Map(masterGlobal.DailyChargeCash, slaveGlobal.DailyChargeCash)
	// 最高在线人数(合并)
	masterGlobal.HighestOnline += slaveGlobal.HighestOnline
	// 九州纪年(全服成就) map<目标ID, 达标的玩家ID组>  (合并)
	masterGlobal.GlobalHandBook = mergeGlobalHandBook(masterGlobal.GlobalHandBook, slaveGlobal.GlobalHandBook)
	// 排行榜玩家被点赞数  (合并)
	masterGlobal.RankLikeLs = mergeU64U32Map(masterGlobal.RankLikeLs, slaveGlobal.RankLikeLs)
	// 仙朝征途-仙位 宗门排行榜信息(合并重算)
	masterGlobal.FairyDynastyZongMenJobRank = append(masterGlobal.FairyDynastyZongMenJobRank, slaveGlobal.FairyDynastyZongMenJobRank...)
	// 仙灵境排行榜 (合并重算)
	masterGlobal.FairylandRank = append(masterGlobal.FairylandRank, slaveGlobal.FairylandRank...)
	// 最高战力 (合并)
	masterGlobal.Topfight = utils.MaxInt64(masterGlobal.Topfight, slaveGlobal.Topfight)
	// 玩家活动信息 (合并)
	masterGlobal.GlobalPlayerYY = mergeGlobalPlayerYY(masterGlobal.GlobalPlayerYY, slaveGlobal.GlobalPlayerYY)
	// 小鸡快跑（合并）
	masterGlobal.HuryChick = mergeHuryChick(masterGlobal.HuryChick, slaveGlobal.HuryChick)
	// 结婚仓库（合并）
	masterGlobal.MarryDepots = mergeMarryDepots(masterGlobal.MarryDepots, slaveGlobal.MarryDepots)
	// 玩家在线属性（合并）
	masterGlobal.OnlineAttr = mergeOnlineAttr(masterGlobal.OnlineAttr, slaveGlobal.OnlineAttr)
	// 禁止登录（合并）
	masterGlobal.ForBidActorIds = mergeU64BoolMap(masterGlobal.ForBidActorIds, slaveGlobal.ForBidActorIds)
	// boss红包使用记录 (合并)
	masterGlobal.BossRedPackUseRecords = append(masterGlobal.BossRedPackUseRecords, slaveGlobal.BossRedPackUseRecords...)
	// 最高等级(合并)
	masterGlobal.TopLevel = utils.MinUInt32(masterGlobal.TopLevel, slaveGlobal.TopLevel)
	// 飞升阵营人数(合并)
	masterGlobal.CampCount = mergeU32U32Map(masterGlobal.CampCount, slaveGlobal.CampCount)
	// 全服首通仙尊试炼记录 key by layer, val by actorId (合并)
	masterGlobal.FirstPassFairyMasterLayer = mergeU32U64Map(masterGlobal.FirstPassFairyMasterLayer, slaveGlobal.FirstPassFairyMasterLayer)
	// 零元购日志 key by subSysId(合并)
	masterGlobal.ZeroShopLogMap = mergeZeroShopLogMap(masterGlobal.ZeroShopLogMap, slaveGlobal.ZeroShopLogMap)
	// 仙侣数据（合并）
	masterGlobal.FriendShip = mergeFriendShip(masterGlobal.FriendShip, slaveGlobal.FriendShip)
	// 拍卖行（合并）
	masterGlobal.SrvAuction = mergeSrvAuction(masterGlobal.SrvAuction, slaveGlobal.SrvAuction)
	// 从服列表
	masterGlobal.SlaveSrvIds = append(masterGlobal.SlaveSrvIds, slaveGlobal.SlaveSrvIds...)
	// 欢乐比拼数据
	masterGlobal.YyDatas.PlayScoreRankMap = mergeYYPlayScoreRank(masterGlobal.YyDatas.PlayScoreRankMap, slaveGlobal.YyDatas.PlayScoreRankMap)
	// 玩法分数比拼
	masterGlobal.YyDatas.GameplayScoreGoalRankMap = mergeYYGameplayScoreGoalRank(masterGlobal.YyDatas.GameplayScoreGoalRankMap, slaveGlobal.YyDatas.GameplayScoreGoalRankMap)
	// 合并宗门赠礼
	masterGlobal.SectGiveGiftGlobalData = mergeSectGiveGiftGlobalData(masterGlobal.SectGiveGiftGlobalData, slaveGlobal.SectGiveGiftGlobalData)
	// 邀请码
	masterGlobal.InviteCodes = mergeStringBoolMap(masterGlobal.InviteCodes, slaveGlobal.InviteCodes)
	// 跨服大亨
	masterGlobal.YyDatas.SrvCrossRankData = mergeSrvCrossRankData(masterGlobal.YyDatas.SrvCrossRankData, slaveGlobal.YyDatas.SrvCrossRankData)
	// 跨服通用排行榜
	masterGlobal.YyDatas.CrossCommonRankMap = mergeSrvCrossCommonRankData(masterGlobal.YyDatas.CrossCommonRankMap, slaveGlobal.YyDatas.CrossCommonRankMap)
	// 五一祈福-热度
	masterGlobal.YyDatas.MayDayBlessDegree = mergeMayDayBlessDegree(masterGlobal.YyDatas.MayDayBlessDegree, slaveGlobal.YyDatas.MayDayBlessDegree)
	// 极品boss红包
	masterGlobal.UltimateBossRedPaperInfo = mergeUltimateBossRedPaperInfo(masterGlobal.UltimateBossRedPaperInfo, slaveGlobal.UltimateBossRedPaperInfo)
	// 开服预告数据（合并）
	masterGlobal.YyDatas.OpenPreviewInfo = mergeOpenPreviewInfo(masterGlobal.YyDatas.OpenPreviewInfo, slaveGlobal.YyDatas.OpenPreviewInfo)
	// 宗门狩猎 (合并)
	masterGlobal.SectHuntingGlobalData = mergeSectHuntingGlobalData(masterGlobal.SectHuntingGlobalData, slaveGlobal.SectHuntingGlobalData)
	// 仙域征伐2.0 (合并)
	masterGlobal.DivineRealmCrossData = mergeDivineRealmCrossData(masterGlobal.DivineRealmCrossData, slaveGlobal.DivineRealmCrossData)
	// 巅峰竞技 (合并)
	masterGlobal.TopFightSrvInfo = mergeTopFightSrvInfo(masterGlobal.TopFightSrvInfo, slaveGlobal.TopFightSrvInfo)
	// 重置首充次数(合并)
	masterGlobal.FirstChargeTimes = utils.MaxUInt32(masterGlobal.FirstChargeTimes, slaveGlobal.FirstChargeTimes)
}

// 巅峰竞技 (合并)
func mergeTopFightSrvInfo(globalData1 *pb3.TopFightSrvInfo, globalData2 *pb3.TopFightSrvInfo) *pb3.TopFightSrvInfo {
	if globalData2 == nil {
		return globalData1
	}

	if globalData1 == nil {
		return globalData2
	}

	if nil == globalData1.ScoreMap {
		globalData1.ScoreMap = map[uint64]uint32{}
	}

	for actorId, score := range globalData2.ScoreMap {
		globalData1.ScoreMap[actorId] = score
	}

	return globalData1
}

// 宗门狩猎 (合并)
func mergeSectHuntingGlobalData(globalData1 *pb3.SectHuntingGlobalData, globalData2 *pb3.SectHuntingGlobalData) *pb3.SectHuntingGlobalData {
	if globalData2 == nil {
		return globalData1
	}

	if globalData1 == nil {
		return globalData2
	}

	if globalData1.Boss != nil && globalData2.Boss != nil && globalData1.Boss.BossId < globalData2.Boss.BossId {
		return globalData2
	}
	return globalData1
}

// 仙域征伐2.0 (合并)
func mergeDivineRealmCrossData(globalData1 *pb3.DivineRealmCrossData, globalData2 *pb3.DivineRealmCrossData) *pb3.DivineRealmCrossData {
	if globalData2 == nil {
		return globalData1
	}

	if globalData1 == nil {
		return globalData2
	}

	globalData := &pb3.DivineRealmCrossData{
		PDatas:   map[uint64]*pb3.DivineRealmPlayer{},
		SyncTime: globalData1.SyncTime,
	}

	for i, v := range globalData1.PDatas {
		globalData.PDatas[i] = v
	}

	for i, v := range globalData2.PDatas {
		globalData.PDatas[i] = v
	}
	return globalData
}

// 合并宗门赠礼
func mergeSectGiveGiftGlobalData(globalData1 *pb3.SectGiveGiftGlobalData, globalData2 *pb3.SectGiveGiftGlobalData) *pb3.SectGiveGiftGlobalData {
	var globalData = &pb3.SectGiveGiftGlobalData{
		GiftTabData: make(map[uint32]*pb3.SectGiveGiftTabData),
		Chest:       &pb3.SectGiveGiftChest{},
	}

	if globalData2 == nil {
		return globalData1
	}

	if globalData1 == nil {
		return globalData
	}

	for k, data := range globalData1.GiftTabData {
		globalData.GiftTabData[k] = data
	}

	for k, data := range globalData2.GiftTabData {
		globalData.GiftTabData[k] = data
	}

	if globalData1.Chest.Lv > globalData2.Chest.Lv {
		globalData.Chest.Lv = globalData1.Chest.Lv
		globalData.Chest.Exp = globalData1.Chest.Exp
	} else {
		globalData.Chest.Lv = globalData2.Chest.Lv
		globalData.Chest.Exp = globalData2.Chest.Exp
	}

	return globalData
}

func mergeYYPlayScoreRank(rankMap map[uint32]*pb3.YYPlayScoreRank, rankMap2 map[uint32]*pb3.YYPlayScoreRank) map[uint32]*pb3.YYPlayScoreRank {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}
	for id, rank := range rankMap {
		scoreRank, ok := rankMap2[id]
		if !ok {
			continue
		}
		if scoreRank.RankType != rank.RankType {
			continue
		}
		rank.Items = append(rank.Items, scoreRank.Items...)
		for k, v := range scoreRank.JoinFlagMap {
			rank.JoinFlagMap[k] = v
		}
		for k, v := range scoreRank.ExtPlayerInfoMap {
			rank.ExtPlayerInfoMap[k] = v
		}
	}
	return rankMap
}

func mergeYYGameplayScoreGoalRank(rankMap map[uint32]*pb3.YYGameplayScoreGoalRank, rankMap2 map[uint32]*pb3.YYGameplayScoreGoalRank) map[uint32]*pb3.YYGameplayScoreGoalRank {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}
	for id, rank := range rankMap {
		scoreRank, ok := rankMap2[id]
		if !ok {
			continue
		}
		if scoreRank.RankType != rank.RankType {
			continue
		}
		rank.Items = append(rank.Items, scoreRank.Items...)
		for k, v := range scoreRank.JoinFlagMap {
			rank.JoinFlagMap[k] = v
		}
		for k, v := range scoreRank.ActorRecordMap {
			rank.ActorRecordMap[k] = v
		}
		for k, v := range scoreRank.DailyRewardFlagMap {
			rank.DailyRewardFlagMap[k] = v
		}
	}
	return rankMap
}

func mergeSrvCrossRankData(rankMap map[uint32]*pb3.SrvCrossRankData, rankMap2 map[uint32]*pb3.SrvCrossRankData) map[uint32]*pb3.SrvCrossRankData {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}
	for id, rank := range rankMap {
		merge, ok := rankMap2[id]
		if !ok {
			continue
		}
		if merge.IsCalc != rank.IsCalc {
			logger.LogError("活动id %d 结算信息不一致", id)
		}
		if nil == rank.PlayerData {
			rank.PlayerData = map[uint64]*pb3.SrvCrossRankPlayerData{}
		}
		for pid, pData := range merge.PlayerData {
			rank.PlayerData[pid] = pData
		}
	}
	return rankMap
}

func mergeSrvCrossCommonRankData(rankMap map[uint32]*pb3.YYCrossCommonRank, rankMap2 map[uint32]*pb3.YYCrossCommonRank) map[uint32]*pb3.YYCrossCommonRank {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}
	for id, rank := range rankMap {
		merge, ok := rankMap2[id]
		if !ok {
			continue
		}
		if merge.IsCalc != rank.IsCalc {
			logger.LogError("活动id %d 结算信息不一致", id)
		}
		if nil == rank.PlayerData {
			rank.PlayerData = map[uint64]*pb3.SrvCrossRankPlayerData{}
		}
		for pid, pData := range merge.PlayerData {
			rank.PlayerData[pid] = pData
		}
	}
	return rankMap
}

func mergeMayDayBlessDegree(rankMap map[uint32]*pb3.YYMayDayBlessDegree, rankMap2 map[uint32]*pb3.YYMayDayBlessDegree) map[uint32]*pb3.YYMayDayBlessDegree {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}
	for id, rank := range rankMap {
		merge, ok := rankMap2[id]
		if !ok {
			continue
		}
		rank.Degree += merge.Degree
		if nil == rank.PlayerDatas {
			rank.PlayerDatas = map[uint64]*pb3.MayDayBlessDegree{}
		}
		for pid, pData := range merge.PlayerDatas {
			rank.PlayerDatas[pid] = pData
		}
	}
	return rankMap
}

func mergeUltimateBossRedPaperInfo(rankMap map[uint64]*pb3.UltimateBossRedPaperInfo, rankMap2 map[uint64]*pb3.UltimateBossRedPaperInfo) map[uint64]*pb3.UltimateBossRedPaperInfo {
	if rankMap == nil || rankMap2 == nil {
		return rankMap
	}

	newMap := make(map[uint64]*pb3.UltimateBossRedPaperInfo, len(rankMap)+len(rankMap2))

	for id, rank := range rankMap {
		newMap[id] = rank
	}

	for id, rank := range rankMap2 {
		newMap[id] = rank
	}

	return newMap
}

func mergeSrvAuction(global *pb3.SrvAuction, slave *pb3.SrvAuction) *pb3.SrvAuction {
	newPb := &pb3.SrvAuction{
		Series:      utils.MaxUInt32(global.Series, slave.Series),
		Goods:       make(map[uint64]*pb3.SrvAuctionGoods),
		Bonus:       make(map[uint32]*pb3.SrvAuctionBonusCal),
		LastCalTime: global.GetLastCalTime(),
	}

	for k, v := range global.Goods {
		newPb.Goods[k] = v
	}
	for k, v := range slave.Goods {
		newPb.Goods[k] = v
	}

	for k, v := range global.Bonus {
		newPb.Bonus[k] = v
	}
	for k, v := range slave.Bonus {
		newPb.Bonus[k] = v
	}

	return newPb
}

func mergeFriendShip(global *pb3.FriendShip, slave *pb3.FriendShip) *pb3.FriendShip {
	newPb := &pb3.FriendShip{
		Series:           utils.MaxUInt32(global.Series, slave.Series),
		FriendCommonData: make(map[uint64]*pb3.FriendCommonData),
		MarryApply:       make(map[uint64]*pb3.MarryApply),
	}

	for k, v := range global.FriendCommonData {
		newPb.FriendCommonData[k] = v
	}
	for k, v := range slave.FriendCommonData {
		newPb.FriendCommonData[k] = v
	}

	for k, v := range global.MarryApply {
		newPb.MarryApply[k] = v
	}
	for k, v := range slave.MarryApply {
		newPb.MarryApply[k] = v
	}

	return newPb
}

func mergeZeroShopLogMap(global map[uint32]*pb3.GlobalZeroShopLogData, slave map[uint32]*pb3.GlobalZeroShopLogData) map[uint32]*pb3.GlobalZeroShopLogData {
	newMap := make(map[uint32]*pb3.GlobalZeroShopLogData)
	for k, v := range global {
		newMap[k] = v
	}
	for k, v := range slave {
		val, ok := newMap[k]
		if !ok {
			newMap[k] = &pb3.GlobalZeroShopLogData{}
			val = newMap[k]
		}
		val.ZeroShopLogList = append(val.ZeroShopLogList, v.ZeroShopLogList...)
	}
	return newMap
}

func mergeOnlineAttr(global map[uint64]*pb3.SpAttr, slave map[uint64]*pb3.SpAttr) map[uint64]*pb3.SpAttr {
	newMap := make(map[uint64]*pb3.SpAttr)
	for k, v := range global {
		newMap[k] = v
	}
	for k, v := range slave {
		newMap[k] = v
	}
	return newMap
}

func mergeMarryDepots(global map[string]*pb3.MarryDepot, slave map[string]*pb3.MarryDepot) map[string]*pb3.MarryDepot {
	newMap := make(map[string]*pb3.MarryDepot)
	for k, v := range global {
		newMap[k] = v
	}
	for k, v := range slave {
		newMap[k] = v
	}
	return newMap
}

func mergeHuryChick(global map[uint32]*pb3.HuryChickInfoStore, slave map[uint32]*pb3.HuryChickInfoStore) map[uint32]*pb3.HuryChickInfoStore {
	newMap := make(map[uint32]*pb3.HuryChickInfoStore)
	for k, v := range global {
		newMap[k] = v
	}
	for k, v := range slave {
		val, ok := newMap[k]
		if !ok {
			newMap[k] = &pb3.HuryChickInfoStore{}
			val = newMap[k]
		}
		val.ChickId = v.ChickId
		val.HisWinTimes += v.HisWinTimes
	}
	return newMap
}

func mergeGlobalHandBook(global map[uint32]*pb3.HandBookPlayerIds, slave map[uint32]*pb3.HandBookPlayerIds) map[uint32]*pb3.HandBookPlayerIds {
	newMap := make(map[uint32]*pb3.HandBookPlayerIds)
	for k, v := range global {
		newMap[k] = v
	}
	for k, v := range slave {
		val, ok := newMap[k]
		if !ok {
			newMap[k] = &pb3.HandBookPlayerIds{}
			val = newMap[k]
		}
		val.PlayerIds = append(val.PlayerIds, v.PlayerIds...)
	}
	return newMap
}

func mergeU64I64Map(map1, map2 map[uint64]int64) map[uint64]int64 {
	newMap := make(map[uint64]int64)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}
	return newMap
}
func mergeU64U64Map(map1, map2 map[uint64]uint64) map[uint64]uint64 {
	newMap := make(map[uint64]uint64)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

func mergeU64U32Map(map1, map2 map[uint64]uint32) map[uint64]uint32 {
	newMap := make(map[uint64]uint32)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}
func mergeU32U32Map(map1, map2 map[uint32]uint32) map[uint32]uint32 {
	newMap := make(map[uint32]uint32)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}
func mergeU32U64Map(map1, map2 map[uint32]uint64) map[uint32]uint64 {
	newMap := make(map[uint32]uint64)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

func mergeU64F64Map(map1, map2 map[uint64]float64) map[uint64]float64 {
	newMap := make(map[uint64]float64)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

func mergeStringBoolMap(map1, map2 map[string]bool) map[string]bool {
	newMap := make(map[string]bool)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

func mergeU64BoolMap(map1, map2 map[uint64]bool) map[uint64]bool {
	newMap := make(map[uint64]bool)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

func max(x, y uint32) uint32 {
	if x > y {
		return x
	}
	return y
}

func mergeGlobalPlayerYY(map1, map2 map[uint64]*pb3.GlobalPlayerYY) map[uint64]*pb3.GlobalPlayerYY {
	newMap := make(map[uint64]*pb3.GlobalPlayerYY)
	for k, v := range map1 {
		newMap[k] = v
	}
	for k, v := range map2 {
		newMap[k] = v
	}

	return newMap
}

// mergeOpenPreviewInfo 合并开服预告数据
func mergeOpenPreviewInfo(master, slave map[uint32]*pb3.YYOpenPreviewInfo) map[uint32]*pb3.YYOpenPreviewInfo {
	newMap := make(map[uint32]*pb3.YYOpenPreviewInfo)

	for k, v := range master {
		newMap[k] = v
	}

	for id, slaveInfo := range slave {
		masterInfo, ok := newMap[id]
		if !ok {
			newMap[id] = slaveInfo
			continue
		}

		// 合并玩家数据
		if masterInfo.PlayerData == nil {
			masterInfo.PlayerData = make(map[uint64]*pb3.YYOpenPreviewPlayerData)
		}
		for playerId, playerData := range slaveInfo.PlayerData {
			masterInfo.PlayerData[playerId] = playerData
		}
	}

	return master
}

type Instance struct {
	Pf     string `json:"pf"`
	Master int    `json:"Master"`
	Slave  []int  `json:"Slave"`
}

//go:embed ynjg_dev.sql.tpl
var tplFs embed.FS

func Concat(a, b interface{}) string {
	return fmt.Sprintf("%v%v", a, b)
}

func ParseTmpl(tmplName string) (*template.Template, error) {
	buf, err := tplFs.ReadFile(tmplName)
	if err != nil {
		logger.LogError("err:%v", err)
		return nil, err
	}
	t, err := template.New(tmplName).Funcs(template.FuncMap{"concat": Concat}).Parse(string(buf))
	return t, err
}

func GenSqlTemp(pfStr string, master int, slaveList []int) {
	tmpl, err := ParseTmpl("ynjg_dev.sql.tpl")
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	var merge = &Instance{
		Pf:     pfStr,
		Master: master,
		Slave:  slaveList,
	}
	fileName := fmt.Sprintf("ynjg.sql")
	out, err := os.Create(path2.Join(utils.GetCurrentDir(), fileName))
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
	defer out.Close()
	err = tmpl.Execute(out, merge)
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
}

func init() {
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		SyncSlaveSrvIds2Cross()
	})
}
