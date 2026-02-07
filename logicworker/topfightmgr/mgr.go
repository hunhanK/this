/**
* @Author: LvYuMeng
* @Date: 2023/11/20
* @Desc:
**/

package topfightmgr

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

var Status uint32 = 0
var ScheduleIdx uint32 = 1
var GroupSt = new(pb3.TopFightGroup)
var RacingMapDetails = make(map[uint32]*pb3.RacingMapDetail)
var PrepareInfo = make(map[uint64][]byte)
var AttenderMap = make(map[uint64]struct{})

func GetPlayerTopFightData(player iface.IPlayer) *pb3.TopFight {
	binary := player.GetBinaryData()
	if nil == binary.TopFight {
		binary.TopFight = &pb3.TopFight{}
	}
	return binary.TopFight
}

func onTopFightStatus(buf []byte) {
	msg := &pb3.SyncTopFightSchedule{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onTopFightStatus %v", err)
		return
	}

	Status = msg.Status
	ScheduleIdx = msg.ScheduleId
	GroupSt = msg.Group

	data := proto.Clone(msg).(*pb3.SyncTopFightSchedule)
	event.TriggerSysEvent(custom_id.SeTopFightStatus, data)
	logger.LogDebug("sync onTopFightStatus scheduleIdx(%d), status(%d)", ScheduleIdx, Status)
}

func sendLeftTime(player iface.IPlayer) {
	data := GetPlayerTopFightData(player)
	conf := jsondata.GetTopFightConf()
	player.SendProto3(51, 2, &pb3.S2C_51_2{
		LeftTimes:       conf.DailyTimes - data.GetTopFightMatchTimes(),
		Flag:            data.GetTopFightChipFlag(),
		MatchRewardsIds: data.GetMatchRewardsIds(),
	})
}

func onLogin(actor iface.IPlayer, args ...interface{}) {
	if Status != 0 {
		actor.SendProto3(51, 0, &pb3.S2C_51_0{
			Idx:    ScheduleIdx,
			Status: Status,
			Group:  GroupSt,
		})
	}

	sendLeftTime(actor)
	if buf, exist := PrepareInfo[actor.GetId()]; exist {
		timer.SetTimeout(time.Duration(1)*time.Second, func() {
			actor.SendProtoBuffer(51, 15, buf)
		})
	}
}
func onLogout(player iface.IPlayer, args ...interface{}) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CCancelTopFightMatch, &pb3.CommonSt{
		U64Param: player.GetId(),
	})
	if err != nil {
		logger.LogError("player(%d) cancel topFight match failed:%v", player.GetId(), err)
		return
	}
}

func onPlayerLogin(player iface.IPlayer, args ...interface{}) {
	syncCandidate(player)
}

func onPlayerLogout(player iface.IPlayer, args ...interface{}) {
	syncCandidate(player)
}

const candidateNum = 20

func onNewDay(player iface.IPlayer, args ...interface{}) {
	data := GetPlayerTopFightData(player)
	data.TopFightMatchTimes = 0
	data.MatchRewardsIds = nil
	sendLeftTime(player)
}

func onChangeName(actor iface.IPlayer, args ...interface{}) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CActorChangeInfo, nil)
	if err != nil {
		logger.LogError(err.Error())
		return
	}
}

func c2sReqMatch(player iface.IPlayer, msg *base.Message) error {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		return neterror.InternalError("SmallCrossServer fight client not exists")
	}
	conf := jsondata.GetTopFightConf()
	if nil == conf {
		return neterror.ConfNotFoundError("topfight conf not exist")
	}

	if player.GetCircle() < conf.Circle {
		player.SendTipMsg(tipmsgid.TpBoundary)
		return nil
	}

	info := GetPlayerTopFightData(player)

	if conf.DailyTimes <= info.GetTopFightMatchTimes() {
		player.SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}

	//押镖中，无法匹配
	//if player.GetExtraAttr(attrdef.DartCarType) > 0 {
	//	player.SendTipMsg(tipmsgid.Tpindartcar)
	//	return nil
	//}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CReqTopFightMatch, &pb3.CommonSt{
		U64Param:  player.GetId(),
		U64Param2: uint64(player.GetExtraAttr(attrdef.FightValue)),
	})
	if err != nil {
		return err
	}
	player.SetExtraAttr(attrdef.MatchingTopFight, 1)

	return nil
}

func c2sCancelMatch(player iface.IPlayer, msg *base.Message) error {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CCancelTopFightMatch, &pb3.CommonSt{
		U64Param: player.GetId(),
	})
	if err != nil {
		return err
	}
	player.SetExtraAttr(attrdef.MatchingTopFight, 0)
	return nil
}

// 领取筹码
func c2sFetchChip(player iface.IPlayer, msg *base.Message) error {
	req := &pb3.C2S_51_12{}
	if err := msg.UnpackagePbmsg(req); err != nil {
		return err
	}
	conf := jsondata.GetTopFightConf()
	pType := req.GetType()
	chipConf := conf.ChipConf

	if nil == chipConf[pType] {
		return neterror.ConfNotFoundError("topFight chip conf(%d) nil", pType)
	}
	data := GetPlayerTopFightData(player)
	flag := data.GetTopFightChipFlag()
	if utils.IsSetBit(flag, pType) {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	data.TopFightChipFlag = utils.SetBit(flag, pType)
	sendLeftTime(player)

	player.AddMoney(moneydef.Chip, int64(chipConf[pType].Chip), true, pb3.LogId_LogFetchTopFightChip)
	return nil
}

// 请求竞猜
func c2sReqGuess(player iface.IPlayer, msg *base.Message) error {
	req := &pb3.C2S_51_13{}
	if err := msg.UnpackagePbmsg(req); err != nil {
		return err
	}

	useChip := req.GetMoney()
	guessLimit := jsondata.GetTopFightConf().GuessLimit
	if useChip > guessLimit {
		return neterror.ParamsInvalidError("topFight guess limit")
	}

	if req.GetUniqueKey() == 0 {
		return neterror.ParamsInvalidError("topFight guess no choice")
	}

	if !player.DeductMoney(moneydef.Chip, int64(useChip), common.ConsumeParams{LogId: pb3.LogId_LogTopFightGuess}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	player.SendTipMsg(tipmsgid.TptopFightTips5)

	return engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CReqGuess, &pb3.CommonSt{
		U64Param:  player.GetId(),
		U32Param:  req.GetIdx(),
		U32Param2: req.GetUniqueKey(),
		U32Param3: useChip,
		BParam:    req.GetIsRed(),
		U64Param2: uint64(engine.GetPfId())<<32 | uint64(engine.GetServerId()),
	})
}

const (
	TopFightRaceTypeScore = 1 //积分淘汰赛
)

func isTopFightAttender(player iface.IPlayer) bool {
	schedule := jsondata.GetTopFightScheduleConf(ScheduleIdx)
	if schedule.Type <= 0 {
		return false
	}
	if schedule.Type == TopFightRaceTypeScore {
		return true
	}
	//晋级赛
	details := RacingMapDetails[schedule.Type]
	for _, v := range details.Attenders {
		if v.ActorId == player.GetId() {
			return true
		}
	}

	return false
}

func c2sReqEnterPrepareScene(player iface.IPlayer, msg *base.Message) error {
	if !isTopFightAttender(player) {
		player.SendTipMsg(tipmsgid.TpNoCompetitionsOpen)
		return nil
	}

	err := player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterTopFightPrepareScene, nil)
	if err != nil {
		logger.LogError("进入巅峰竞技准备场景出错:%v", err)
		return err
	}
	return nil
}

func c2sMatchRewards(player iface.IPlayer, msg *base.Message) error {
	req := &pb3.C2S_51_23{}
	if err := msg.UnpackagePbmsg(req); err != nil {
		return err
	}

	conf := jsondata.GetTopFightConf()
	if nil == conf {
		return neterror.ConfNotFoundError("topfight conf is nil")
	}

	var ids []uint32
	var rewardsVec []jsondata.StdRewardVec
	data := GetPlayerTopFightData(player)
	for _, v := range conf.MatchRewards {
		if data.TopFightMatchTimes < v.Times {
			continue
		}
		if pie.Uint32s(data.MatchRewardsIds).Contains(v.Times) {
			continue
		}
		ids = append(ids, v.Times)
		rewardsVec = append(rewardsVec, v.Awards)
	}

	if len(ids) <= 0 {
		return neterror.ParamsInvalidError("no can rev")
	}

	data.MatchRewardsIds = append(data.MatchRewardsIds, ids...)
	rewards := jsondata.MergeStdReward(rewardsVec...)
	engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogTopFightMatchAwards,
	})

	player.SendProto3(51, 23, &pb3.S2C_51_23{Ids: ids})
	return nil
}

func onMatchSucc(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onMatchSucc %v", err)
		return
	}

	actorId := msg.U64Param
	player := manager.GetPlayerPtrById(actorId)
	if nil == player {
		return
	}
	data := GetPlayerTopFightData(player)
	data.TopFightMatchTimes++
	sendLeftTime(player)
	player.SetExtraAttr(attrdef.MatchingTopFight, 0)
	player.TriggerQuestEvent(custom_id.QttJoinTopFightTimes, 0, 1)
}

func onEnterPromotion(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onEnterPromotion %v", err)
		return
	}

	actorId, fbHdl, sceneId, isRed := msg.U64Param, msg.U64Param2, msg.U32Param, msg.BParam
	player := manager.GetPlayerPtrById(actorId)
	if nil == player {
		return
	}

	sceneConf := jsondata.GetTopFightConf().SceneConf
	x, y := sceneConf.RedX, sceneConf.RedY
	if !isRed {
		x, y = sceneConf.BlueX, sceneConf.BlueY
	}
	todo := &pb3.CommonSt{
		U64Param:  fbHdl,
		U32Param:  sceneId,
		U32Param2: x,
		U32Param3: y,
	}
	err := player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterFbHdl, todo)
	if err != nil {
		logger.LogError("player(%d) enter promotion race scene failed", player.GetId())
		return
	}
}

func onCancelTopFightMatch(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onCancelTopFightMatch %v", err)
		return
	}

	player := manager.GetPlayerPtrById(msg.U64Param)
	if nil == player {
		return
	}
	player.SetExtraAttr(attrdef.MatchingTopFight, 0)
}

// 请求淘汰赛排行榜
func c2sReqScoreRank(player iface.IPlayer, msg *base.Message) error {
	data := GetPlayerTopFightData(player)
	total := data.GetTopFightTotalTimes()
	win := data.GetTopFightWinTimes()
	var winRate float64
	if total > 0 {
		winRate = float64(win) / float64(total) * 100
	}

	err := player.CallActorSmallCrossFunc(actorfuncid.G2CTopFightScoreRank, &pb3.CommonSt{
		U32Param: uint32(winRate),
	})

	if nil != err {
		return err
	}

	return nil
}

// 角色可能不在线
// 玩家直接退出游戏，触发跨服的结算，再返回来可能拿不到这边的玩家实体
// 淘汰赛结果
func onTopFightScoreResult(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onTopFightScoreResult %v", err)
		return
	}

	engine.SendPlayerMessage(msg.U64Param, gshare.OfflineTopFightScoreReulst, msg)
}

// 把镜像机器人拉入副本
func onTopFightScoreRobotEnterReq(buf []byte) {
	msg := &pb3.RobotEnterTopFightReq{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onTopFightScoreRobotEnterReq %v", err)
		return
	}

	robotActorId, err := robotmgr.GetMirrorRobotActorId()
	if nil != err {
		logger.LogError("GetMirrorRobotActorId %v", err)
		return
	}

	createData := robotmgr.CopyRealActorMirrorRobotData(msg.GetCloneActorId(), &custom_id.MirrorRobotParam{
		RobotType:     custom_id.ActorRobotTypeTopFight,
		RobotConfigId: msg.GetCloneActorId(),
	})

	if nil == createData {
		logger.LogError("createData copy fail:%d", msg.GetCloneActorId())
		return
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CTopFightScoreRobotEnterRet, &pb3.RobotEnterTopFightRet{
		CloneActorId:    msg.GetCloneActorId(),
		CloneCreateData: createData,
		RobotActorId:    robotActorId,
		PfId:            engine.GetPfId(),
		SrvId:           engine.GetServerId(),
		ActorRivalId:    msg.GetActorRivalId(),
	})
	if err != nil {
		logger.LogError("send G2CTopFightScoreRobotEnterRet fail:%v", err)
		return
	}

	return
}

func syncCandidate(player iface.IPlayer) {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		return
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeLevel)
	playerRank := rank.GetRankById(player.GetId())
	if playerRank > candidateNum {
		return
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CTopFightSyncScoreCandidate, &pb3.TopFightCandidateListRet{
		Result: []*pb3.OneRankItem{
			{
				Id:    player.GetId(),
				Score: int64(player.GetLevel()),
			},
		},
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func onTopFightSyncScoreCandidateReq(buf []byte) {
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeLevel)
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CTopFightSyncScoreCandidate, &pb3.TopFightCandidateListRet{
		Result: rank.GetList(1, candidateNum),
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func checkSrvInfoClear() {
	if GroupSt.GroupId == 0 || nil == GroupSt.PrepareList {
		return
	}

	globalVar := gshare.GetStaticVar()
	oldInfo := globalVar.TopFightSrvInfo
	if nil == oldInfo {
		return
	}

	if oldInfo.GroupId != oldInfo.GroupId || oldInfo.StartTime != GroupSt.PrepareList[0].StartTime {
		globalVar.TopFightSrvInfo = nil
	}

	schedule := jsondata.GetTopFightScheduleConf(ScheduleIdx)
	if schedule.Type > TopFightRaceTypeScore {
		globalVar.TopFightSrvInfo = nil
	}

	return
}

func onSyncTopFightScore(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onTopFightScoreRobotEnterReq %v", err)
		return
	}

	if GroupSt.GroupId == 0 || nil == GroupSt.PrepareList {
		return
	}

	actorId, score := msg.U64Param, msg.U32Param

	globalVar := gshare.GetStaticVar()
	if nil == globalVar.TopFightSrvInfo {
		globalVar.TopFightSrvInfo = &pb3.TopFightSrvInfo{
			GroupId:   GroupSt.GroupId,
			StartTime: GroupSt.PrepareList[0].StartTime,
		}
	}

	srvInfo := globalVar.TopFightSrvInfo

	if nil == srvInfo.ScoreMap {
		srvInfo.ScoreMap = map[uint64]uint32{}
	}

	srvInfo.ScoreMap[actorId] = score
}

func onC2GTopFightScoreReq(buf []byte) {
	msg := &pb3.C2GTopFightScoreReq{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onTopFightScoreRobotEnterReq %v", err)
		return
	}

	srvInfo := gshare.GetStaticVar().TopFightSrvInfo
	if nil == srvInfo {
		return
	}

	if srvInfo.GroupId == 0 {
		return
	}

	if srvInfo.GroupId != msg.GroupId || srvInfo.StartTime != msg.StartTime {
		gshare.GetStaticVar().TopFightSrvInfo = nil
		logger.LogInfo("新赛程组%d 服务器上一组%d 不匹配合并条件", msg.GroupId, srvInfo.GroupId)
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CTopFightScoreRet, &pb3.C2GTopFightScoreRet{
		SrvInfo: gshare.GetStaticVar().TopFightSrvInfo,
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
	})

	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// 淘汰赛结果
func offlineTopFightScoreResult(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	isWin := st.BParam
	data := GetPlayerTopFightData(player)
	data.TopFightTotalTimes++
	if isWin {
		data.TopFightWinTimes++
	}
}

// 重置巅峰竞技个人数据
func onResetTopFightPersonal(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onResetTopFightPersonal %v", err)
		return
	}

	if msg.U64Param > 0 {
		engine.SendPlayerMessage(msg.U64Param, gshare.OfflineResetTopFight, msg)
	}
}

// 重置巅峰竞技数据
func OnResetTopFight(buf []byte) {
	Status = 0
	ScheduleIdx = 1
	RacingMapDetails = make(map[uint32]*pb3.RacingMapDetail)
	PrepareInfo = make(map[uint64][]byte)
	AttenderMap = make(map[uint64]struct{})
	logger.LogDebug("sync OnResetTopFight")
}

func offlineResetTopFight(player iface.IPlayer, msg pb3.Message) {
	data := GetPlayerTopFightData(player)
	data.TopFightTotalTimes = 0
	data.TopFightWinTimes = 0
	data.TopFightChipFlag = 0
	data.TopFightMatchTimes = 0
	data.MatchRewardsIds = nil
	sendLeftTime(player)
}

// 如果轮空准备阶段不在线，玩家在10分钟内上线，要发提示告诉他轮空
func offlineDirectWinPrepare(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	if time_util.NowSec() < st.U32Param {
		player.SendTipMsg(tipmsgid.TpTopFightDirectWin)
	}
}

func onSyncPrepareInfo(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onSyncPrepareInfo %v", err)
		return
	}

	actorId, buffer := msg.U64Param, msg.Buf
	PrepareInfo[actorId] = buffer
	if actor := manager.GetPlayerPtrById(actorId); nil != actor {
		actor.SendProtoBuffer(51, 15, buffer)
	}
}

func onClearPrepareInfo(buf []byte) {
	PrepareInfo = make(map[uint64][]byte)
}

func onClearAttenderMap(buf []byte) {
	AttenderMap = make(map[uint64]struct{})
}

func onSyncRacingMap(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onSyncRacingMap %v", err)
		return
	}

	detail := &pb3.RacingMapDetail{}
	if err := pb3.Unmarshal(msg.Buf, detail); nil != err {
		return
	}

	RacingMapDetails[detail.GetPType()] = detail
}

func onSyncAttenderMap(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onSyncRacingMap %v", err)
		return
	}

	AttenderMap[msg.U64Param] = struct{}{}
}

func IsInTopFightAttenderMap(actorId uint64) bool {
	if _, exist := AttenderMap[actorId]; exist {
		return true
	}
	return false
}

func onDirectWinPrepare(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onDirectWinPrepare %v", err)
		return
	}

	msg.U32Param = time_util.NowSec() + 600 // 有效时限
	engine.SendPlayerMessage(msg.U64Param, gshare.OfflineDirectWinPrepare, msg)
}

func onDirectWin(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onDirectWin %v", err)
		return
	}
	//轮空不发结算面板了
	actorId := msg.U64Param
	conf := jsondata.GetTopFightConf()
	gshare.AddMoneyOffline(actorId, moneydef.Honor, conf.WinHonor*2, true, uint32(pb3.LogId_LogTopFightPromotionRace))
}

// 请求对战表
func c2sReqRacingMap(player iface.IPlayer, msg *base.Message) error {
	req := &pb3.C2S_51_3{}
	if err := msg.UnpackagePbmsg(req); err != nil {
		return err
	}

	pType := req.GetPType()

	sendMsg := &pb3.S2C_51_3{
		PType:     pType,
		RacingMap: make([]*pb3.RacingMapDetail, 0),
	}

	if pType == 0 {
		for _, v := range RacingMapDetails {
			sendMsg.RacingMap = append(sendMsg.RacingMap, v)
		}
		player.SendProto3(51, 3, sendMsg)
	} else {
		if v, exist := RacingMapDetails[pType]; exist {
			sendMsg.RacingMap = append(sendMsg.RacingMap, v)
		}
		player.SendProto3(51, 3, sendMsg)
	}
	return nil
}

func gmAddTopFightScore(actor iface.IPlayer, args ...string) bool {
	score := utils.AtoUint32(args[0])
	st := &pb3.CommonSt{
		U64Param: actor.GetId(),
		U32Param: score,
	}
	if len(args) >= 2 {
		st.U64Param = utils.AtoUint64(args[1])
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSetTopScore, st)
	if err != nil {
		logger.LogError("use gm topscore fail:%v", err)
		return false
	}
	return true
}

func gmTopFightStatusChange(actor iface.IPlayer, args ...string) bool {
	idx := utils.AtoUint32(args[0])
	status := utils.AtoUint32(args[1])
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSetTopFightStatus, &pb3.CommonSt{
		U32Param:  status,
		U32Param2: idx,
	})
	if err != nil {
		logger.LogError("use gm topstatus fail:%v", err)
		return false
	}
	return true
}

func gmResetTopFightPromote(actor iface.IPlayer, args ...string) bool {
	if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CResetTopFightPromote, nil); nil != err {
		logger.LogError("resetPromote err:%v", err)
		return false
	}
	return true
}

func loadTopFightRobot(actor iface.IPlayer, args ...string) bool {
	id, hp, fight := utils.AToU64(args[0]), utils.AToU64(args[1]), utils.AToU64(args[2])
	if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CLoadTestTopFightRobot, &pb3.CommonSt{U64Param: id, U64Param2: hp, U64Param3: fight,
		U32Param: engine.GetServerId(), U32Param2: engine.GetPfId(), U32Param3: uint32(actor.GetSmallCrossCamp())}); nil != err {
		logger.LogError("loadTopFightRobot err:%v", err)
		return false
	}
	return true
}

func matchTopFightRobot(actor iface.IPlayer, args ...string) bool {
	id := utils.AToU64(args[0])
	if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CMatchTestTopFightRobot, &pb3.CommonSt{U64Param: id}); nil != err {
		logger.LogError("matchTopFightRobot err:%v", err)
		return false
	}
	return true
}

func init() {
	engine.RegisterMessage(gshare.OfflineResetTopFight, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineResetTopFight)
	engine.RegisterMessage(gshare.OfflineTopFightScoreReulst, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineTopFightScoreResult)
	engine.RegisterMessage(gshare.OfflineDirectWinPrepare, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineDirectWinPrepare)

	engine.RegisterSysCall(sysfuncid.C2GTopFightStatus, onTopFightStatus)

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		checkSrvInfoClear()
	})
	// 重置
	engine.RegisterSysCall(sysfuncid.C2GResetTopFightPersonal, onResetTopFightPersonal)
	engine.RegisterSysCall(sysfuncid.C2GResetTopFight, OnResetTopFight)
	engine.RegisterSysCall(sysfuncid.C2GClearPrepareInfo, onClearPrepareInfo)
	engine.RegisterSysCall(sysfuncid.C2GClearAttenderMap, onClearAttenderMap)

	// 积分淘汰赛
	engine.RegisterSysCall(sysfuncid.C2GCancelTopFightMatch, onCancelTopFightMatch)
	engine.RegisterSysCall(sysfuncid.C2GMatchSucc, onMatchSucc)
	engine.RegisterSysCall(sysfuncid.C2GTopFightScoreResult, onTopFightScoreResult)
	engine.RegisterSysCall(sysfuncid.C2GTopFightScoreRobotEnterReq, onTopFightScoreRobotEnterReq)
	engine.RegisterSysCall(sysfuncid.C2GTopFightSyncScoreCandidateReq, onTopFightSyncScoreCandidateReq)
	engine.RegisterSysCall(sysfuncid.C2GSyncTopFightScore, onSyncTopFightScore)
	engine.RegisterSysCall(sysfuncid.C2GTopFightScoreReq, onC2GTopFightScoreReq)

	// 晋级赛
	engine.RegisterSysCall(sysfuncid.C2GEnterPromotion, onEnterPromotion)
	engine.RegisterSysCall(sysfuncid.C2GDirectWinPrepare, onDirectWinPrepare)
	engine.RegisterSysCall(sysfuncid.C2GDirectWin, onDirectWin)
	// 同步信息
	engine.RegisterSysCall(sysfuncid.C2GSyncPrepareInfo, onSyncPrepareInfo)
	engine.RegisterSysCall(sysfuncid.C2GSyncAttenderMap, onSyncAttenderMap)
	engine.RegisterSysCall(sysfuncid.C2GSyncRacingMap, onSyncRacingMap)
	//
	event.RegActorEvent(custom_id.AeAfterLogin, onLogin)
	event.RegActorEvent(custom_id.AeReconnect, onLogin)
	event.RegActorEvent(custom_id.AeLogout, onLogout)
	event.RegActorEvent(custom_id.AeNewDay, onNewDay)
	event.RegActorEventL(custom_id.AeChangeName, onChangeName)
	event.RegActorEvent(custom_id.AeLogin, onPlayerLogin)
	event.RegActorEvent(custom_id.AeLogout, onPlayerLogout)
	//
	net.RegisterProto(51, 1, c2sReqScoreRank)
	net.RegisterProto(51, 3, c2sReqRacingMap)
	net.RegisterProto(51, 10, c2sReqMatch)
	net.RegisterProto(51, 11, c2sCancelMatch)
	net.RegisterProto(51, 12, c2sFetchChip)
	net.RegisterProto(51, 13, c2sReqGuess)
	net.RegisterProto(51, 16, c2sReqEnterPrepareScene)
	net.RegisterProto(51, 23, c2sMatchRewards)

	gmevent.Register("topscore", gmAddTopFightScore, 1)
	gmevent.Register("topstatus", gmTopFightStatusChange, 1)
	gmevent.Register("resetPromote", gmResetTopFightPromote, 1)
	gmevent.Register("loadTopFightRobot", loadTopFightRobot, 1)
	gmevent.Register("matchTopFightRobot", matchTopFightRobot, 1)
}
