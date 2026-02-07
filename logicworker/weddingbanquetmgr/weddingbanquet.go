/**
 * @Author: LvYuMeng
 * @Date: 2023/12/11
 * @Desc:
**/

package weddingbanquetmgr

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func GetWBanquetReserve() map[uint64]*pb3.WeddingBanquetReserve {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.WeddingBanquetReserveData {
		globalVar.WeddingBanquetReserveData = make(map[uint64]*pb3.WeddingBanquetReserve)
	}

	return globalVar.WeddingBanquetReserveData
}

func GetWeddingBanquetKey(id, startTime uint32) uint64 {
	return utils.Make64(id, startTime)
}

func GetWeddingBanquetId(key uint64) uint32 {
	return utils.Low32(key)
}

func GetWeddingBanquetStartTime(key uint64) uint32 {
	return utils.High32(key)
}

func GetWeddingBanquetReserveInfo(id, startTime uint32) *pb3.WeddingBanquetReserve {
	return GetWeddingBanquetReserveInfoByKey(GetWeddingBanquetKey(id, startTime))
}

func GetWeddingBanquetReserveInfoByKey(key uint64) *pb3.WeddingBanquetReserve {
	data := GetWBanquetReserve()
	v, ok := data[key]
	if !ok {
		return nil
	}
	if GetWeddingBanquetEndTime(key) < time_util.NowSec() {
		return nil
	}
	if nil == v.InviteIds {
		v.InviteIds = make(map[uint64]bool)
	}
	if nil == v.SelfBuyIds {
		v.SelfBuyIds = make(map[uint64]bool)
	}
	if nil == v.ReqEnter {
		v.ReqEnter = make(map[uint64]bool)
	}
	return v
}

func HasWeddingBanquetReserve(commonId uint64) bool {
	data := GetWBanquetReserve()
	nowSec := time_util.NowSec()
	for key, v := range data {
		if v.CommonId == commonId {
			startTime, endTime := GetWeddingBanquetStartTime(key), GetWeddingBanquetEndTime(key)
			if nowSec < startTime {
				return true
			}
			if nowSec >= startTime && nowSec < endTime {
				return true
			}
		}
	}
	return false
}

func CanReservedWeddingBanquet(actorId, reserveKey uint64) bool {
	data := GetWBanquetReserve()
	nowSec := time_util.NowSec()
	for key, v := range data {
		if nowSec > GetWeddingBanquetEndTime(key) {
			continue
		}
		if reserveKey == key {
			return false
		}
		if v.ActorId1 == actorId || v.ActorId2 == actorId {
			return false
		}
	}
	return true
}

func CheckWeddingBanquetLicense(key uint64, actorId uint64, license int) bool {
	reserveInfo := GetWeddingBanquetReserveInfoByKey(key)
	if nil == reserveInfo {
		return false
	}
	switch license {
	case custom_id.WbLicenseCanEnter:
		return reserveInfo.SelfBuyIds[actorId] || reserveInfo.InviteIds[actorId] ||
			reserveInfo.ActorId1 == actorId || reserveInfo.ActorId2 == actorId
	case custom_id.WbLicenseInvitationCanGetTime:
		nowSec := time_util.NowSec()
		endTime := GetWeddingBanquetEndTime(reserveInfo.GetId())
		conf := jsondata.GetWeddingBanquetConf()
		if endTime < nowSec || (endTime-nowSec) < conf.EnterDeadline {
			return false
		}
	}
	return false
}

func ReservedWeddingBanquet(key uint64, grade uint32, a1, a2, commonId uint64) bool {
	data := GetWBanquetReserve()
	data[key] = &pb3.WeddingBanquetReserve{
		Id:       key,
		Grade:    grade,
		CommonId: commonId,
		ActorId1: a1,
		ActorId2: a2,
	}
	GiveWeddingBanquetReserveReward(data[key], key)
	timer.SetTimeout(time.Duration(GetWeddingBanquetStartTime(key)-time_util.NowSec())*time.Second, func() {
		OpenWeddingBanquet(key)
	})
	return false
}

func HandleInviteReq(player iface.IPlayer, req *pb3.C2S_53_14) error {
	reserveInfo := GetWeddingBanquetReserveInfo(req.Id, req.StartTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) is nil", req.Id, req.StartTime)
	}

	if reserveInfo.ActorId1 != player.GetId() && reserveInfo.ActorId2 != player.GetId() {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) no authority to handle invite", req.Id, req.StartTime)
	}

	if req.ActorId == 0 {
		for actorId := range reserveInfo.ReqEnter {
			handleInvite(player, reserveInfo, actorId, req.Agree)
		}
	} else {
		handleInvite(player, reserveInfo, req.ActorId, req.Agree)
	}
	SendBanquetInfoByReserveToReservePlayer(reserveInfo, utils.SetBit(0, custom_id.WeddingBanquetReqEnter))
	SendBanquetInfoByReserveToReservePlayer(reserveInfo, utils.SetBit(0, custom_id.WeddingBanquetInvite))
	return nil
}

func checkInviteStatus(player iface.IPlayer, reserveInfo *pb3.WeddingBanquetReserve, actorId uint64) bool {
	var (
		conf       = jsondata.GetWeddingBanquetConf()
		useTimes   = uint32(len(reserveInfo.InviteIds))
		totalTimes = conf.InviteTimes + reserveInfo.BuyInviteTimes
	)
	if reserveInfo.SelfBuyIds[actorId] || reserveInfo.InviteIds[actorId] {
		delete(reserveInfo.ReqEnter, actorId)
		player.SendTipMsg(tipmsgid.WeddingBanquetInvited)
		return false
	}
	if totalTimes <= useTimes {
		player.SendTipMsg(tipmsgid.TimesNotEnough)
		return false
	}
	return true
}

func handleInvite(player iface.IPlayer, reserveInfo *pb3.WeddingBanquetReserve, actorId uint64, agree bool) {
	if agree {
		if reserveInfo.ActorId1 == actorId || reserveInfo.ActorId2 == actorId {
			return
		}
		if !reserveInfo.ReqEnter[actorId] {
			return
		}
		if !checkInviteStatus(player, reserveInfo, actorId) {
			return
		}
		OnInvite(reserveInfo, actorId)
	}

	delete(reserveInfo.ReqEnter, actorId)
}

func OnInvite(reserveInfo *pb3.WeddingBanquetReserve, actorId uint64) {
	reserveInfo.InviteIds[actorId] = true
	if target := manager.GetPlayerPtrById(actorId); nil != target {
		target.SendProto3(53, 16, &pb3.S2C_53_16{
			Id:        GetWeddingBanquetId(reserveInfo.Id),
			StartTime: GetWeddingBanquetStartTime(reserveInfo.Id),
		})
	}
}

func SendInvite(player iface.IPlayer, id, startTime uint32, targetId uint64) error {
	reserveInfo := GetWeddingBanquetReserveInfo(id, startTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) is nil")
	}
	if reserveInfo.ActorId1 != player.GetId() && reserveInfo.ActorId2 != player.GetId() {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) no authority to handle invite", id, startTime)
	}
	if !checkInviteStatus(player, reserveInfo, targetId) {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) invite status false", id, startTime)
	}
	OnInvite(reserveInfo, targetId)
	SendBanquetInfoByReserveToReservePlayer(reserveInfo, utils.SetBit(0, custom_id.WeddingBanquetReqEnter))
	SendBanquetInfoByReserveToReservePlayer(reserveInfo, utils.SetBit(0, custom_id.WeddingBanquetInvite))
	return nil
}

func SendBanquetInfoByReserveToReservePlayer(st *pb3.WeddingBanquetReserve, reqType uint32) {
	SendBanquetInfoByReserve(manager.GetPlayerPtrById(st.ActorId1), st, reqType)
	SendBanquetInfoByReserve(manager.GetPlayerPtrById(st.ActorId2), st, reqType)
}

func SendBanquetInfoByReserve(player iface.IPlayer, st *pb3.WeddingBanquetReserve, reqType uint32) {
	if nil == player {
		return
	}
	id, startTime, endTime := GetWeddingBanquetId(st.Id), GetWeddingBanquetStartTime(st.Id), GetWeddingBanquetEndTime(st.Id)
	if endTime < time_util.NowSec() { //已过期
		return
	}
	if st.ActorId1 != player.GetId() && st.ActorId2 != player.GetId() {
		return
	}
	if utils.IsSetBit(reqType, custom_id.WeddingBanquetSponsor) {
		player.SendProto3(53, 23, &pb3.S2C_53_23{Id: id, Grade: st.Grade, StartTime: startTime, EndTime: endTime})
	}
	if utils.IsSetBit(reqType, custom_id.WeddingBanquetBuyTimes) {
		player.SendProto3(53, 22, &pb3.S2C_53_22{Id: id, StartTime: startTime, BuyTimes: st.BuyInviteTimes})
	}
	if utils.IsSetBit(reqType, custom_id.WeddingBanquetReqEnter) {
		rsp := &pb3.S2C_53_13{Id: id, StartTime: startTime, Type: custom_id.WeddingBanquetReqEnter}
		for actorId := range st.ReqEnter {
			rsp.Actor = append(rsp.Actor, manager.GetSimplyData(actorId))
		}
		player.SendProto3(53, 13, rsp)
	}
	if utils.IsSetBit(reqType, custom_id.WeddingBanquetInvite) {
		rsp := &pb3.S2C_53_13{Id: id, StartTime: startTime, Type: custom_id.WeddingBanquetInvite}
		for actorId := range st.InviteIds {
			rsp.Actor = append(rsp.Actor, manager.GetSimplyData(actorId))
		}
		player.SendProto3(53, 13, rsp)
	}
}

func SendWeddingBanquetInfo(player iface.IPlayer, reqType uint32) bool {
	data := GetWBanquetReserve()
	for _, st := range data {
		SendBanquetInfoByReserve(player, st, reqType)
	}
	return false
}

func SendWeddingBeInvited(player iface.IPlayer) bool {
	data := GetWBanquetReserve()
	rsp := &pb3.S2C_53_17{}
	for key, st := range data {
		if st.InviteIds[player.GetId()] {
			rsp.InviteList = append(rsp.InviteList, key)
		}
	}
	if len(rsp.InviteList) > 0 {
		player.SendProto3(53, 17, rsp)
	}
	return false
}

func SendWeddingReserveList(player iface.IPlayer) bool {
	data := GetWBanquetReserve()
	rsp := &pb3.S2C_53_19{}
	nowSec := time_util.NowSec()
	for key := range data {
		endTime := GetWeddingBanquetEndTime(key)
		if nowSec <= endTime {
			rsp.Id = append(rsp.Id, key)
		}
	}
	player.SendProto3(53, 19, rsp)
	return false
}

func GetWBanquetInfo() *pb3.WeddingBanquet {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.WeddingBanquet {
		globalVar.WeddingBanquet = &pb3.WeddingBanquet{}
	}
	if nil == globalVar.WeddingBanquet.ScoreMap {
		globalVar.WeddingBanquet.ScoreMap = make(map[uint64]uint32)
	}

	return globalVar.WeddingBanquet
}

func saveWeddingBanquetData(buf []byte) {
	data := &pb3.WeddingBanquet{}
	if err := pb3.Unmarshal(buf, data); err != nil {
		return
	}

	logicData := GetWBanquetInfo()
	if logicData.StartTime > data.StartTime {
		logger.LogError("banquet new data has create !! sync data is: %v,logic data is:%v", data, logicData)
		return
	}

	globalVar := gshare.GetStaticVar()
	globalVar.WeddingBanquet = data
}

func BroadProto3ToReservePlayer(key uint64, protoH, protoL uint16, msg pb3.Message) {
	data := GetWeddingBanquetReserveInfoByKey(key)
	if actor1 := manager.GetPlayerPtrById(data.ActorId1); nil != actor1 {
		actor1.SendProto3(protoH, protoL, msg)
	}
	if actor2 := manager.GetPlayerPtrById(data.ActorId2); nil != actor2 {
		actor2.SendProto3(protoH, protoL, msg)
	}
	return
}

func calcBanquetBallGift(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onSyncPrepareInfo %v", err)
		return
	}
	conf := jsondata.GetWeddingBanquetConf()
	reward := conf.BallGift
	actorId := msg.U64Param
	if actor := manager.GetPlayerPtrById(actorId); nil != actor { //在线进背包不在线直接发
		engine.GiveRewards(actor, reward, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogWeddingBanquetBallGift,
		})
	} else {
		mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
			ConfId:  common.Mail_WeddingBanquetBallGift,
			Rewards: reward,
		})
	}
}

func GiveWeddingBanquetReserveReward(reserveInfo *pb3.WeddingBanquetReserve, banquetKey uint64) bool {
	conf := jsondata.GetWeddingBanquetConf()
	if nil == conf {
		return false
	}

	timeStr := time_util.TimeToStr(GetWeddingBanquetStartTime(banquetKey))

	mailmgr.SendMailToActor(reserveInfo.ActorId1, &mailargs.SendMailSt{
		ConfId:  common.Mail_WeddingBanquetReserve,
		Rewards: conf.ReserveAward,
		Content: &mailargs.BanquetTimeStr{
			TimeStr: timeStr,
		},
	})

	mailmgr.SendMailToActor(reserveInfo.ActorId2, &mailargs.SendMailSt{
		ConfId:  common.Mail_WeddingBanquetReserve,
		Rewards: conf.ReserveAward,
		Content: &mailargs.BanquetTimeStr{
			TimeStr: timeStr,
		},
	})
	return true
}

func BuyInvitation(player iface.IPlayer, id, startTime uint32) error {
	reserveInfo := GetWeddingBanquetReserveInfo(id, startTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet  not exist")
	}
	if reserveInfo.ActorId1 == player.GetId() || reserveInfo.ActorId2 == player.GetId() {
		return neterror.ParamsInvalidError("cant buy self banquet")
	}
	conf := jsondata.GetWeddingBanquetConf()
	if player.GetLevel() < conf.Level {
		player.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}
	if reserveInfo.SelfBuyIds[player.GetId()] || reserveInfo.InviteIds[player.GetId()] {
		player.SendTipMsg(tipmsgid.WeddingBanquetInvited)
		return nil
	}

	if CheckWeddingBanquetLicense(reserveInfo.Id, player.GetId(), custom_id.WbLicenseInvitationCanGetTime) {
		return neterror.ParamsInvalidError("banquet license invitation getTime end")
	}

	if !player.ConsumeByConf(conf.SelfBuy, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyWeddingBanquetInvitation}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	reserveInfo.SelfBuyIds[player.GetId()] = true
	return SendReserveInfo(player, id, startTime)
}

func SendReserveInfo(player iface.IPlayer, id, startTime uint32) error {
	reserveInfo := GetWeddingBanquetReserveInfo(id, startTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) is nil", id, startTime)
	}
	player.SendProto3(53, 18, &pb3.S2C_53_18{
		Id:        id,
		StartTime: startTime,
		Actor1:    manager.GetSimplyData(reserveInfo.ActorId1),
		Actor2:    manager.GetSimplyData(reserveInfo.ActorId2),
		Grade:     reserveInfo.Grade,
		CanEnter:  CheckWeddingBanquetLicense(reserveInfo.Id, player.GetId(), custom_id.WbLicenseCanEnter),
	})
	return nil
}

func BuyInviteTimes(player iface.IPlayer, id, startTime uint32) error {
	reserveInfo := GetWeddingBanquetReserveInfo(id, startTime)
	if nil == reserveInfo {
		return neterror.ParamsInvalidError("wedding banquet  not exist")
	}
	if reserveInfo.ActorId1 != player.GetId() && reserveInfo.ActorId2 != player.GetId() {
		return neterror.ParamsInvalidError("wedding banquet id(%d),startTime(%d) no authority to handle invite", id, startTime)
	}
	conf := jsondata.GetWeddingBanquetConf()
	if conf.BuyInviteLimit <= reserveInfo.BuyInviteTimes {
		return neterror.ParamsInvalidError("wedding banquet BuyInviteLimit")
	}

	if !player.ConsumeByConf(conf.BuyInviteConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogBuyWeddingBanquetInviteTimes}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	reserveInfo.BuyInviteTimes++
	BroadProto3ToReservePlayer(reserveInfo.Id, 53, 22, &pb3.S2C_53_22{
		Id:        id,
		StartTime: startTime,
		BuyTimes:  reserveInfo.BuyInviteTimes,
	})
	return nil
}

func onWeddingBanquetNewDay(args ...interface{}) {
	data := GetWBanquetReserve()
	nowSec := time_util.NowSec()
	var delIds []uint64
	for key := range data { //清理过期预约信息
		endTime := GetWeddingBanquetEndTime(key)
		if endTime < nowSec {
			delIds = append(delIds, key)
		}
	}
	for _, v := range delIds {
		delete(data, v)
	}
}

func GetWeddingBanquetEndTime(key uint64) uint32 {
	conf := jsondata.GetWeddingBanquetConf()
	var (
		id        = GetWeddingBanquetId(key)
		startTime = GetWeddingBanquetStartTime(key)
	)
	if nil == conf.Reserve[id] {
		return 0
	}
	interval := time_util.ToTodayTime(conf.Reserve[id].End) - time_util.ToTodayTime(conf.Reserve[id].Start)

	endTime := startTime + interval
	return endTime
}

func checkWeddingBanquetOpen(args ...interface{}) {
	data := GetWBanquetReserve()
	nowSec := time_util.NowSec()
	for key := range data {
		rKey := key
		startTime, endTime := GetWeddingBanquetStartTime(rKey), GetWeddingBanquetEndTime(rKey)
		if nowSec < startTime {
			timer.SetTimeout(time.Duration(startTime-nowSec)*time.Second, func() {
				OpenWeddingBanquet(rKey)
			})
		}
		if nowSec >= startTime && nowSec < endTime {
			OpenWeddingBanquet(rKey)
		}
	}
}

func OpenWeddingBanquet(key uint64) {
	var (
		reserveInfo = GetWeddingBanquetReserveInfoByKey(key)
		globalVar   = gshare.GetStaticVar()
		id          = GetWeddingBanquetId(key)
		startTime   = GetWeddingBanquetStartTime(key)
		banquet     = GetWBanquetInfo()
	)
	if nil == reserveInfo {
		return
	}

	if banquet.StartTime > startTime {
		logger.LogError("banquet(startTime = %d) cant open because has new banquet(startTime = %d) running", startTime, banquet.StartTime)
		return
	}

	if banquet.StartTime < startTime {
		globalVar.WeddingBanquet = nil
	}

	banquet = GetWBanquetInfo()
	banquet.Id = id
	banquet.StartTime = startTime
	banquet.ActorId1 = reserveInfo.ActorId1
	banquet.ActorId2 = reserveInfo.ActorId2
	banquet.Grade = reserveInfo.Grade
	a1Info := manager.GetSimplyData(banquet.ActorId1)
	a2Info := manager.GetSimplyData(banquet.ActorId2)
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSyncWeddingBanquet, &pb3.SyncWeddingBanquet{
		Banquet: banquet,
		CoupleInfo: map[uint64]*pb3.WeddingBanquetCoupleInfo{
			banquet.ActorId1: {Name: a1Info.GetName(), ActorId: a1Info.GetId(), Head: a1Info.GetHead(), Job: a1Info.GetJob()},
			banquet.ActorId2: {Name: a2Info.GetName(), ActorId: a2Info.GetId(), Head: a2Info.GetHead(), Job: a2Info.GetJob()},
		},
	})
	if err != nil {
		logger.LogError("open banquet err,startTime(%d) actorId1:%d,actorId2:%d", startTime, banquet.ActorId1, banquet.ActorId2)
		return
	}
}

func gmOpenBanquet(actor iface.IPlayer, args ...string) bool {
	targetId := utils.AtoUint64(args[0])
	grade := utils.AtoUint32(args[1])
	target := manager.GetSimplyData(targetId)
	mp := map[uint64]*pb3.WeddingBanquetCoupleInfo{
		actor.GetId():  {Name: actor.GetName(), ActorId: actor.GetId(), Head: actor.GetHead()},
		target.GetId(): {Name: target.GetName(), ActorId: target.GetId(), Head: target.GetHead()},
	}
	key := GetWeddingBanquetKey(1, time_util.NowSec())
	ReservedWeddingBanquet(key, grade, actor.GetId(), targetId, 0)
	gshare.GetStaticVar().WeddingBanquet = &pb3.WeddingBanquet{
		Id:        1,
		StartTime: time_util.NowSec(),
		ActorId1:  actor.GetId(),
		ActorId2:  targetId,
		ScoreMap:  make(map[uint64]uint32),
		Grade:     grade,
	}
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FSyncWeddingBanquet, &pb3.SyncWeddingBanquet{
		Banquet:    gshare.GetStaticVar().WeddingBanquet,
		CoupleInfo: mp,
	})
	return true
}

func gmClearBanquet(actor iface.IPlayer, args ...string) bool {
	gshare.GetStaticVar().WeddingBanquetReserveData = nil
	return true
}

func checkWeddingBanquetRunning(args ...interface{}) {
	data := GetWBanquetInfo()
	key := GetWeddingBanquetKey(data.Id, data.StartTime)
	reserveInfo := GetWeddingBanquetReserveInfoByKey(key)
	if nil == reserveInfo {
		return
	}
	startTime, endTime := GetWeddingBanquetStartTime(key), GetWeddingBanquetEndTime(key)
	nowSec := time_util.NowSec()
	if nowSec >= startTime && nowSec < endTime {
		OpenWeddingBanquet(key)
	}
}

func onBeforeMerge(args ...interface{}) {
	data := GetWBanquetReserve()
	nowSec := time_util.NowSec()
	var delIds []uint64
	var backInfo []*pb3.WeddingBanquetReserve
	for k := range data {
		rKey := k
		startTime := GetWeddingBanquetStartTime(rKey)
		if nowSec < startTime { //未开启的婚宴
			delIds = append(delIds, k)
		}
	}
	for _, delId := range delIds {
		backInfo = append(backInfo, data[delId])
		delete(data, delId)
	}

	backAwards := jsondata.ConsumeChangeToRewards(jsondata.GetWeddingBanquetConf().SelfBuy)
	for _, reserve := range backInfo {
		commonData, ok := friendmgr.GetFriendCommonDataById(reserve.CommonId)
		if !ok {
			continue
		}
		if nil == commonData.MarryInfo.WeddingBanquetTimes {
			commonData.MarryInfo.WeddingBanquetTimes = make(map[uint32]uint32)
		}
		commonData.MarryInfo.WeddingBanquetTimes[reserve.Grade]++

		for actorId := range reserve.SelfBuyIds {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  common.Mail_BackWeddingBanquetBuyInv,
				Rewards: backAwards,
			})
		}
	}
}

func init() {
	gmevent.Register("openbanquet", gmOpenBanquet, 1)
	gmevent.Register("clearbanquet", gmClearBanquet, 1)

	event.RegSysEvent(custom_id.SeNewDayArrive, onWeddingBanquetNewDay)
	event.RegSysEvent(custom_id.SeServerInit, checkWeddingBanquetOpen)
	event.RegSysEvent(custom_id.SeFightSrvConnSucc, checkWeddingBanquetRunning)
	event.RegSysEvent(custom_id.SeCmdGmBeforeMerge, onBeforeMerge)

	engine.RegisterSysCall(sysfuncid.F2GSaveWeddingBanquetData, saveWeddingBanquetData)
	engine.RegisterSysCall(sysfuncid.F2GWeddingBanquetBallGift, calcBanquetBallGift)

}
