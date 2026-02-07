/**
 * @Author: LvYuMeng
 * @Date: 2023/12/4
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
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
	"jjyz/gameserver/logicworker/inffairyplacemgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/weddingbanquetmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type MarrySys struct {
	Base
	applications map[uint64]uint32
}

func (s *MarrySys) OnInit() {
	s.applications = make(map[uint64]uint32)
	s.fixMarryData()
}

func (s *MarrySys) OnLogin() {
	data := s.GetData()
	s.owner.SetExtraAttr(attrdef.MarryId, int64(data.MarryId))
}

func (s *MarrySys) fixMarryData() {
	data := s.GetData()
	realMarry := friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry)
	if data.MarryId > 0 && !realMarry { //被离婚
		data.MarryId = 0
		data.EndTime = make(map[uint32]uint32)
	}
	if data.MarryId == 0 && realMarry {
		fd, _ := friendmgr.GetFriendCommonDataById(data.CommonId)
		data.MarryId = utils.Ternary(fd.ActorId1 != s.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
	}
}

func (s *MarrySys) s2cInfo() {
	data := s.GetData()

	rsp := &pb3.S2C_53_0{
		Id:            data.CommonId,
		EndTime:       data.EndTime,
		GradeDailyBuy: data.GradeDailyBuy,
	}

	fd, ok := friendmgr.GetFriendCommonDataById(data.CommonId)
	if ok {
		rsp.MarryId = utils.Ternary(fd.ActorId1 != s.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
		if utils.IsSetBit(fd.Status, custom_id.FsEngagement) {
			rsp.Type = ReqEngagement
		}
		if utils.IsSetBit(fd.Status, custom_id.FsMarry) {
			rsp.Type = ReqMarry
			rsp.MarryTime = fd.MarryInfo.MarryTime
		}
	}

	if actor, ok := manager.GetData(rsp.MarryId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		rsp.MarryName = actor.GetName()
		rsp.MarryJob = actor.GetJob()
		rsp.MarrySex = actor.GetSex()
		rsp.AppearInfo = actor.GetAppearInfo()
	}
	s.owner.SendProto3(53, 0, rsp)
	if rsp.Type == ReqMarry {
		s.SendProto3(53, 20, &pb3.S2C_53_20{Times: fd.MarryInfo.WeddingBanquetTimes})
	}
}

func (s *MarrySys) OnAfterLogin() {
	s.s2cInfo()
	s.sendEngagementInfo(s.owner)
	friendmgr.GetMarryApplyMgr().SendMarryInfo(s.owner)
	s.owner.SyncShowStr(custom_id.ShowStrMarryName)
}

func (s *MarrySys) OnReconnect() {
	s.s2cInfo()
	s.sendEngagementInfo(s.owner)
	friendmgr.GetMarryApplyMgr().SendMarryInfo(s.owner)
}

func (s *MarrySys) OnLogout() {
	s.RefuseAllEngagement()
}

const (
	MarryCd_Engagement = 1
	MarryCd_Marry      = 2
	MarryCd_CashGift   = 3

	ReqBreakEngagement = 0
	ReqEngagement      = 1
	ReqMarry           = 2
	ReqDivorce         = 3
)

func (s *MarrySys) GetData() *pb3.MarryData {
	binary := s.GetBinaryData()
	if nil == binary.MarryData {
		binary.MarryData = &pb3.MarryData{}
	}
	data := binary.MarryData
	if nil == data.EndTime {
		data.EndTime = make(map[uint32]uint32)
	}
	if nil == data.GradeDailyBuy {
		data.GradeDailyBuy = make(map[uint32]uint32)
	}
	if nil == data.RecGradeDailyAwards {
		data.RecGradeDailyAwards = make(map[uint32]uint32)
	}
	return data
}

func (s *MarrySys) canBuyMarryGift(target iface.IPlayer) (bool, error) {
	actor := s.owner
	conf := jsondata.GetMarryConf()
	if nil == conf {
		return false, neterror.ConfNotFoundError("no marry conf")
	}

	if actor.GetLevel() < conf.Level || target.GetLevel() < conf.Level {
		return false, neterror.ConfNotFoundError("level not meet")
	}

	fSys, ok := actor.GetSysObj(sysdef.SiFriend).(*FriendSys)

	if !ok || !fSys.IsOpen() {
		return false, neterror.InternalError("friend sys is nil")
	}

	data := s.GetData()
	isMarry := friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry)

	if isMarry { //结婚直接买
		return true, nil
	}

	//没结婚判断能否结婚
	if !friendmgr.IsExistStatus(data.CommonId, custom_id.FsEngagement) {
		return false, neterror.ParamsInvalidError("not Engagement")
	}
	//亲密度足够
	if fSys.GetIntimacy(target.GetId()) < conf.Intimacy {
		return false, neterror.ParamsInvalidError("intimacy not enough")
	}

	marryId := friendmgr.GetEngagementId(data.CommonId, s.owner.GetId())
	if marryId != target.GetId() {
		return false, neterror.ParamsInvalidError("target not engagement object")
	}

	return true, nil
}

func (s *MarrySys) c2sInfo(msg *base.Message) error {
	s.s2cInfo()
	return nil
}

func (s *MarrySys) c2sMarry(msg *base.Message) error {
	var req pb3.C2S_53_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	tarId := req.ActorId

	target := manager.GetPlayerPtrById(tarId)
	if nil == target {
		s.owner.SendTipMsg(tipmsgid.TpTargetOffline)
		return nil
	}

	if can, err := s.canBuyMarryGift(target); !can {
		return err
	}

	data := s.GetData()

	if data.EndTime[MarryCd_Marry] >= time_util.NowSec() {
		return neterror.ParamsInvalidError("marry apply cd")
	}

	conf := jsondata.GetMarryConf()

	gradeConf := conf.Grade[req.ConfId]
	if nil == gradeConf {
		return neterror.ConfNotFoundError("no marry grade conf(%d)", req.ConfId)
	}

	if data.GradeDailyBuy[gradeConf.Type] >= gradeConf.DailyTimes {
		s.owner.SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	isMarry := friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry)
	if (!isMarry || req.IsAA) && friendmgr.GetMarryApplyMgr().HasMarryInvite(tarId) { //请求结婚/aa要确认请求
		return neterror.ParamsInvalidError("has a marry invite")
	}

	isSameSex := s.owner.GetSex() == target.GetSex()
	if gradeConf.IsSameSex != isSameSex {
		return neterror.ParamsInvalidError("not meet marry grade sex %d", req.ConfId)
	}

	consume := gradeConf.Consume
	if req.IsAA {
		consume = gradeConf.AAConsume
	}

	var (
		back []*pb3.StdAward
	)

	succ, remove := s.owner.ConsumeByConfWithRet(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogMarryApply})
	if !succ {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	if isMarry && !req.IsAA { //全款礼金直接发放奖励
		data.GradeDailyBuy[gradeConf.Type]++
		friendmgr.GetMarryApplyMgr().GiveMarryReward(data.CommonId, gradeConf)
		s.SendProto3(53, 1, &pb3.S2C_53_1{ActorId: tarId, ConfId: req.ConfId})
		s.SendProto3(53, 10, &pb3.S2C_53_10{
			Type:  gradeConf.Type,
			Count: data.GradeDailyBuy[gradeConf.Type],
		})
		return nil
	}

	for mt, count := range remove.MoneyMap {
		back = append(back, &pb3.StdAward{
			Id:    jsondata.GetMoneyIdConfByType(mt),
			Count: count,
		})
	}

	endTime := conf.WaitTime + time_util.NowSec()
	newApply := &pb3.MarryApply{
		ReqId:   s.owner.GetId(),
		ConfId:  req.ConfId,
		IsAA:    req.IsAA,
		EndTime: endTime,
		Back:    back,
	}

	friendmgr.GetMarryApplyMgr().SendMarryInvite(newApply, tarId)
	target.SendProto3(53, 3, &pb3.S2C_53_3{
		Apply:  newApply,
		Player: manager.GetSimplyData(tarId),
	})

	s.updateCd(MarryCd_Marry, time_util.NowSec()+conf.WaitTime)
	s.SendProto3(53, 1, &pb3.S2C_53_1{ActorId: tarId, ConfId: req.ConfId})

	return nil
}

func (s *MarrySys) rspMarry(msg *base.Message) error {
	var req pb3.C2S_53_4
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	switch req.Type {
	case ReqEngagement:
		err = s.handleEngagement(req.ActorId, req.Agree)
	case ReqMarry:
		err = s.handleMarry(req.ActorId, req.Agree)
	}
	return err
}

func notifyMarryStatus(myId, commonId uint64, status uint32) {
	player := manager.GetPlayerPtrById(myId)
	if nil == player {
		return
	}
	marrySys, ok := player.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok {
		player.LogError("player(%d) marrySys get nil,status is %d", myId, status)
	}
	switch status {
	case ReqMarry: //结婚成功
		marrySys.onMarry()
	case ReqDivorce:
		marrySys.onDivorce()
	case ReqBreakEngagement:
		marrySys.onBreakEngagement()
	case ReqEngagement:
		marrySys.onEngagement(commonId)
	}
	player.SendProto3(53, 7, &pb3.S2C_53_7{ActorId: player.GetId(), Type: status})
	marrySys.s2cInfo()
	player.TriggerEvent(custom_id.AeMarryStatusChange)
}

func (s *MarrySys) onEngagement(commonId uint64) {
	data := s.GetData()
	data.CommonId = commonId
	s.RefuseAllEngagement()
}

func (s *MarrySys) handleEngagement(targetId uint64, agree bool) error {
	target := manager.GetPlayerPtrById(targetId)
	if nil == target {
		s.owner.SendTipMsg(tipmsgid.TpTargetOffline)
		agree = false
	}

	if !agree { //直接返还
		s.RefuseEngagement(targetId)
		return nil
	}

	targetMarrySys, ok := target.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !targetMarrySys.IsOpen() {
		return neterror.InternalError("sys not open")
	}

	//检查婚姻状态
	if ok, err := s.checkEngagementStatus(targetId); !ok {
		return err
	}
	if ok, err := targetMarrySys.checkEngagementStatus(s.owner.GetId()); !ok {
		return err
	}

	if fid := s.AcceptEngagement(targetId); fid > 0 {
		notifyMarryStatus(s.owner.GetId(), fid, ReqEngagement)
		notifyMarryStatus(targetId, fid, ReqEngagement)
		saveFriendRelation(s.owner.GetId(), targetId, custom_id.FrEngagement)
	}
	target.TriggerQuestEvent(custom_id.QttMarrySysByEngagement, 0, 1)
	s.owner.TriggerQuestEvent(custom_id.QttMarrySysByEngagement, 0, 1)
	return nil
}

func (s *MarrySys) reset() {
	data := s.GetData()
	data.MarryId = 0
	data.CommonId = 0
	data.EndTime = make(map[uint32]uint32)
	data.MarryTime = 0
	data.MarryName = ""
	s.owner.SyncShowStr(custom_id.ShowStrMarryName)
}

func (s *MarrySys) onMarry() {
	data := s.GetData()
	var targetId uint64
	fd, ok := friendmgr.GetFriendCommonDataById(data.CommonId)
	if !ok || nil == fd.MarryInfo {
		s.LogError("p1(%d) and p2(%d) common data(%d) is nil", s.owner.GetId(), targetId, data.CommonId)
	}
	targetId = utils.Ternary(fd.ActorId1 != s.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
	data.MarryId = targetId
	data.MarryTime = fd.MarryInfo.MarryTime
	if baseData, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		data.MarryName = baseData.GetName()
	}
	s.owner.SyncShowStr(custom_id.ShowStrMarryName)
	s.owner.TriggerEvent(custom_id.AeMarrySuccess, targetId)
	inffairyplacemgr.GetLocalInfFairyPlaceMgr().OnMarry(s.owner.GetId(), targetId)
	s.owner.SetExtraAttr(attrdef.MarryId, attrdef.AttrValueAlias(targetId))
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FOnMarry, &pb3.CommonSt{
		U64Param:  s.owner.GetId(),
		U64Param2: targetId,
	})
	if err != nil {
		s.LogError("G2FOnMarry err:%v", err)
	}
}

func (s *MarrySys) onDivorce() {
	s.reset()
	s.owner.TriggerEvent(custom_id.AeDivorce)
	inffairyplacemgr.GetLocalInfFairyPlaceMgr().OnDivorce(s.owner.GetId())
	s.owner.SetExtraAttr(attrdef.MarryId, 0)
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FOnDivorce, &pb3.CommonSt{
		U64Param: s.owner.GetId(),
	})
	if err != nil {
		s.LogError("G2FOnDivorce err:%v", err)
	}
}

func (s *MarrySys) onBreakEngagement() {
	s.reset()
}

func (s *MarrySys) handleMarry(targetId uint64, agree bool) error {
	if !agree {
		friendmgr.GetMarryApplyMgr().BackCashGift(s.owner.GetId())
		engine.SendPlayerMessage(targetId, gshare.OfflineMarryCdClear, &pb3.CommonSt{
			U32Param: MarryCd_Marry,
		})
		return nil
	}

	data := s.GetData()

	if !friendmgr.IsExistStatus(data.CommonId, custom_id.FsEngagement) {
		return neterror.ParamsInvalidError("not engagement")
	}

	isForMarry := !friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry)

	gradeId, ok := friendmgr.GetMarryApplyMgr().AcceptMarry(s.owner, targetId, data.CommonId)
	if !ok {
		return nil
	}

	if isForMarry {
		saveFriendRelation(s.owner.GetId(), targetId, custom_id.FrMarry)
		notifyMarryStatus(s.owner.GetId(), data.CommonId, ReqMarry)
		notifyMarryStatus(targetId, data.CommonId, ReqMarry)
	}

	engine.SendPlayerMessage(targetId, gshare.OfflineRspMarryReq, &pb3.CommonSt{
		U32Param:  gradeId,
		U32Param2: time_util.NowSec(),
	})

	engine.SendPlayerMessage(targetId, gshare.OfflineMarryCdClear, &pb3.CommonSt{
		U32Param: MarryCd_Marry,
	})

	return nil
}

func (s *MarrySys) divorce() error {
	fSys, ok := s.owner.GetSysObj(sysdef.SiFriend).(*FriendSys)

	if !ok || !fSys.IsOpen() {
		return neterror.InternalError("friend sys is nil")
	}

	data := s.GetData()

	if weddingbanquetmgr.HasWeddingBanquetReserve(data.CommonId) {
		return neterror.ParamsInvalidError("Has Wedding Banquet Reserve")
	}

	if friendmgr.GetMarryApplyMgr().HasMarryInvite(data.MarryId) || friendmgr.GetMarryApplyMgr().HasMarryInvite(s.owner.GetId()) {
		return neterror.ParamsInvalidError("has marry invite")
	}

	s.owner.TriggerEvent(custom_id.AeBeforeDivorce)

	divorceId := data.MarryId

	friendmgr.GetMarryApplyMgr().BackCashGift(s.owner.GetId())
	friendmgr.GetMarryApplyMgr().BackCashGift(divorceId)

	friendmgr.OnDivorce(data.CommonId)

	fSys.DelFriend(divorceId, custom_id.FrMarry)
	fSys.DelFriend(divorceId, custom_id.FrEngagement)

	notifyMarryStatus(s.owner.GetId(), 0, ReqDivorce)
	notifyMarryStatus(divorceId, 0, ReqDivorce)

	mailmgr.SendMailToActor(divorceId, &mailargs.SendMailSt{
		ConfId:  common.Mail_MarryBreak,
		Content: &mailargs.PlayerNameArgs{Name: s.owner.GetName()},
	})
	return nil
}

func (s *MarrySys) cancelEngagement() error {
	fSys, ok := s.owner.GetSysObj(sysdef.SiFriend).(*FriendSys)

	if !ok || !fSys.IsOpen() {
		return neterror.InternalError("friend sys is nil")
	}

	data := s.GetData()
	targetId := friendmgr.GetEngagementId(data.CommonId, s.owner.GetId())

	if friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry) {
		return neterror.ParamsInvalidError("need divorce")
	}

	if friendmgr.GetMarryApplyMgr().HasMarryInvite(targetId) || friendmgr.GetMarryApplyMgr().HasMarryInvite(s.owner.GetId()) {
		return neterror.ParamsInvalidError("has marry invite")
	}

	friendmgr.OnCancel(data.CommonId)

	fSys.DelFriend(targetId, custom_id.FrEngagement)

	notifyMarryStatus(s.owner.GetId(), 0, ReqBreakEngagement)
	notifyMarryStatus(targetId, 0, ReqBreakEngagement)
	return nil
}

func (s *MarrySys) c2sDivorce(msg *base.Message) error {
	var req pb3.C2S_53_7
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	switch req.Type {
	case ReqEngagement:
		return s.cancelEngagement()
	case ReqMarry:
		return s.divorce()
	}

	return nil
}

func (s *MarrySys) checkEngagementStatus(otherId uint64) (bool, error) {
	fSys, ok := s.owner.GetSysObj(sysdef.SiFriend).(*FriendSys)

	if !ok || !fSys.IsOpen() {
		return false, neterror.InternalError("friend sys is nil")
	}

	if !fSys.IsExistFriend(otherId, custom_id.FrFriend) {
		return false, neterror.InternalError("not friend")
	}

	commonId := s.GetData().CommonId

	if friendmgr.IsExistStatus(commonId, custom_id.FsEngagement) {
		return false, neterror.InternalError("repeated engagement")
	}
	if friendmgr.IsExistStatus(commonId, custom_id.FsEngagement) {
		return false, neterror.InternalError("repeated marry")
	}

	return true, nil
}

func (s *MarrySys) updateCd(cdType, cd uint32) {
	data := s.GetData()
	data.EndTime[cdType] = cd
	s.owner.SendProto3(53, 2, &pb3.S2C_53_2{Type: cdType, EndTime: s.GetData().EndTime[cdType]})
}

func (s *MarrySys) c2sEngagement(msg *base.Message) error {
	var req pb3.C2S_53_8
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	tarId := req.ActorId

	target := manager.GetPlayerPtrById(tarId)
	if nil == target {
		s.owner.SendTipMsg(tipmsgid.TpTargetOffline)
		return nil
	}

	targetMarrySys, ok := target.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !targetMarrySys.IsOpen() {
		return neterror.InternalError("sys not open")
	}

	//检查婚姻状态
	if ok, err := s.checkEngagementStatus(tarId); !ok {
		return err
	}
	if ok, err := targetMarrySys.checkEngagementStatus(s.owner.GetId()); !ok {
		return err
	}

	data := s.GetData()
	if data.EndTime[MarryCd_Engagement] >= time_util.NowSec() {
		s.owner.SendTipMsg(tipmsgid.Incd)
		return nil
	}

	if !targetMarrySys.CanAcceptEngagement(s.owner.GetId()) {
		return neterror.ParamsInvalidError("in target apply limit")
	}

	conf := jsondata.GetMarryConf()
	nowSec := time_util.NowSec()
	endTime := nowSec + conf.WaitTime
	targetMarrySys.SendEngagementApply(s.owner.GetId(), endTime)

	target.SendProto3(53, 9, &pb3.S2C_53_9{Apply: []*pb3.EngagementApply{{
		Player:  manager.GetSimplyData(s.owner.GetId()),
		EndTime: endTime,
	}}})

	s.updateCd(MarryCd_Engagement, nowSec+conf.Cd)

	return nil
}

// 结缘
func (s *MarrySys) sendEngagementInfo(actor iface.IPlayer) {
	nowSec := time_util.NowSec()
	rsp := &pb3.S2C_53_9{}
	for actorId, endTime := range s.applications {
		if endTime < nowSec {
			s.RefuseEngagement(actorId)
			continue
		}
		rsp.Apply = append(rsp.Apply, &pb3.EngagementApply{
			Player:  manager.GetSimplyData(actorId),
			EndTime: endTime,
		})
	}
	if len(rsp.Apply) > 0 {
		actor.SendProto3(53, 9, rsp)
	}
}

func (s *MarrySys) CanAcceptEngagement(reqId uint64) bool {
	var ed uint32
	for actorId, sendTime := range s.applications {
		ed = utils.MaxUInt32(ed, sendTime)
		if actorId == reqId { //已经在申请中
			return false
		}
	}

	conf := jsondata.GetMarryConf()

	cd := time_util.NowSec() - (ed - conf.WaitTime)
	if cd < conf.Cd { //cd冷却时间内有收到申请不允许通过
		return false
	}
	return true
}

func (s *MarrySys) SendEngagementApply(reqId uint64, endTime uint32) {
	s.applications[reqId] = endTime
	s.owner.SetTimeout(time.Duration(endTime-time_util.NowSec())*time.Second, func() {
		deadline, ok := s.applications[reqId]
		if !ok || deadline > time_util.NowSec() {
			return
		}
		s.RefuseEngagement(reqId)
	})
}

func (s *MarrySys) AcceptEngagement(reqId uint64) uint64 {
	if _, ok := s.applications[reqId]; !ok {
		return 0
	}

	fData := friendmgr.NewFriendship(s.owner.GetId(), reqId)

	fData.Status = utils.SetBit(fData.Status, custom_id.FsEngagement)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEngagement, &pb3.LogPlayerCounter{
		NumArgs: reqId,
	})

	delete(s.applications, reqId)

	return fData.Id
}

func (s *MarrySys) RefuseAllEngagement() {
	for actorId := range s.applications {
		s.sendRefuseEngagementMail(actorId)
	}
	s.applications = make(map[uint64]uint32)
}

func (s *MarrySys) RefuseEngagement(reqId uint64) {
	s.sendRefuseEngagementMail(reqId)
	delete(s.applications, reqId)
}

func (s *MarrySys) sendRefuseEngagementMail(actorId uint64) {
	mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
		ConfId: common.Mail_EngagementRefuse,
		Content: &mailargs.PlayerNameArgs{
			Name: s.owner.GetName(),
		},
	})
	if target := manager.GetPlayerPtrById(actorId); nil != target {
		target.SendTipMsg(tipmsgid.MarryRefuseReq, s.owner.GetName())
	}
}

func (s *MarrySys) onNewDay() {
	data := s.GetData()
	data.GradeDailyBuy = nil
	data.RecGradeDailyAwards = nil
	s.s2cInfo()
}

func onMarryChangeName(actor iface.IPlayer, args ...interface{}) {
	marrySys, ok := actor.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !marrySys.IsOpen() {
		return
	}
	data := marrySys.GetData()
	if data.MarryId == 0 {
		return
	}
	if obj := manager.GetPlayerPtrById(data.MarryId); nil != obj {
		obj.SyncShowStr(custom_id.ShowStrMarryName)
		if mData := obj.GetBinaryData().MarryData; nil != mData {
			mData.MarryName = actor.GetName()
		}
	}
}

func onMarryNewDay(actor iface.IPlayer, args ...interface{}) {
	marrySys, ok := actor.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !marrySys.IsOpen() {
		return
	}
	marrySys.onNewDay()
}

func offlineMarryCdClear(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	marrySys, ok := player.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !marrySys.IsOpen() {
		return
	}
	marrySys.updateCd(st.U32Param, 0)
}

func offlineRspMarryReq(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	marrySys, ok := player.GetSysObj(sysdef.SiMarry).(*MarrySys)
	if !ok || !marrySys.IsOpen() {
		return
	}

	if !time_util.IsSameDay(st.U32Param2, time_util.NowSec()) {
		return
	}

	conf := jsondata.GetMarryConf()
	if nil == conf {
		return
	}

	gradeConf, ok := conf.Grade[st.U32Param]
	if !ok {
		return
	}
	data := marrySys.GetData()
	data.GradeDailyBuy[gradeConf.Type]++
	player.SendProto3(53, 10, &pb3.S2C_53_10{
		Type:  gradeConf.Type,
		Count: data.GradeDailyBuy[gradeConf.Type],
	})
}

func init() {
	RegisterSysClass(sysdef.SiMarry, func() iface.ISystem {
		return &MarrySys{}
	})

	net.RegisterSysProtoV2(53, 0, sysdef.SiMarry, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarrySys).c2sInfo
	})
	net.RegisterSysProtoV2(53, 1, sysdef.SiMarry, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarrySys).c2sMarry
	})
	net.RegisterSysProtoV2(53, 4, sysdef.SiMarry, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarrySys).rspMarry
	})
	net.RegisterSysProtoV2(53, 7, sysdef.SiMarry, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarrySys).c2sDivorce
	})
	net.RegisterSysProtoV2(53, 8, sysdef.SiMarry, func(s iface.ISystem) func(*base.Message) error {
		return s.(*MarrySys).c2sEngagement
	})

	engine.RegisterMessage(gshare.OfflineMarryCdClear, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineMarryCdClear)
	engine.RegisterMessage(gshare.OfflineRspMarryReq, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineRspMarryReq)

	event.RegActorEvent(custom_id.AeChangeName, onMarryChangeName)
	event.RegActorEvent(custom_id.AeNewDay, onMarryNewDay)
}
