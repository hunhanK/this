package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"strings"

	"github.com/gzjjyz/logger"

	"github.com/gzjjyz/srvlib/utils"
)

type FriendSys struct {
	Base
	friendMap    map[uint64]*pb3.FriendInfo // 好友数据
	recommendMap []*pb3.FriendSimplyInfo    // 缓存推荐列表
}

func (sys *FriendSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.FriendData {
		binary.FriendData = &pb3.FriendData{}
	}
	if nil == binary.FriendData.ApplyMap {
		binary.FriendData.ApplyMap = make(map[uint64]int64)
	}
}

func (sys *FriendSys) GetData() *pb3.FriendData {
	binary := sys.GetBinaryData()
	return binary.FriendData
}

func (sys *FriendSys) AddIntimacy(actorId uint64, add uint32) bool {
	if !sys.IsExistFriend(actorId, custom_id.FrFriend) {
		return false
	}
	friendmgr.GetIntimacyMgr().AddIntimacy(sys.owner.GetId(), actorId, add, true, true)
	return true
}

func (sys *FriendSys) GetIntimacyLv(actorId uint64) uint32 {
	if !sys.IsExistFriend(actorId, custom_id.FrFriend) {
		return 0
	}
	return sys.friendMap[actorId].IntimacyLv
}

func (sys *FriendSys) GetIntimacy(actorId uint64) uint32 {
	if !sys.IsExistFriend(actorId, custom_id.FrFriend) {
		return 0
	}
	return sys.friendMap[actorId].Intimacy
}

func (sys *FriendSys) GetAddLevel() uint32 {
	return jsondata.GlobalUint("contactsApplyFriendLevelLimit")
}

func (sys *FriendSys) OnLogin() {
	gshare.SendDBMsg(custom_id.GMsgLoadFriendData, sys.owner.GetId())
}

func (sys *FriendSys) OnLogout() {
	ownerId := sys.owner.GetId()
	nowSec := time_util.NowSec()
	for friendId := range sys.friendMap {
		friend := manager.GetPlayerPtrById(friendId)
		if nil == friend {
			continue
		}
		if fSys, ok := friend.GetSysObj(sysdef.SiFriend).(*FriendSys); ok { //告知在线的好友已离线
			fSys.UpdateFriendLogoutTime(ownerId, nowSec)
		}
	}
}

func (sys *FriendSys) OnOpen() {
	sys.s2cApplyList()
	sys.s2cKilledList()
	sys.ToFriendRecommend()
}

func (sys *FriendSys) ToFriendRecommend() {
	globalVar := gshare.GetStaticVar()
	minnLv := jsondata.GlobalUint("contactsRecommondLevelLimit")
	if minnLv > sys.owner.GetLevel() {
		return
	}
	if !utils.SliceContainsUint64(globalVar.FriendRecommed, sys.owner.GetId()) {
		globalVar.FriendRecommed = append(globalVar.FriendRecommed, sys.owner.GetId())
		if len(globalVar.FriendRecommed) > custom_id.FrRecommendActive {
			globalVar.FriendRecommed = globalVar.FriendRecommed[1:]
		}
	}
}

func (sys *FriendSys) OnReconnect() {
	sys.s2cAllFriendList()
	sys.s2cApplyList()
	sys.s2cKilledList()
}

func (sys *FriendSys) OnAfterLogin() {
	sys.s2cApplyList()
	sys.s2cKilledList()
	sys.ToFriendRecommend()
}

func toFriendSimplyInfo(player *pb3.SimplyPlayerData, ts uint64) *pb3.FriendSimplyInfo {
	if nil == player {
		return nil
	}
	return &pb3.FriendSimplyInfo{
		PlayerId:    player.GetId(),
		Name:        player.GetName(),
		Job:         player.GetJob(),
		Circle:      player.GetCircle(),
		Level:       player.GetLv(),
		Vip:         player.GetVipLv(),
		ApplyTime:   ts,
		HeadFrame:   player.HeadFrame,
		BubbleFrame: player.BubbleFrame,
		Head:        player.Head,
	}
}

func (sys *FriendSys) getRecommendLevel() (mi, ma uint32) {
	lv := sys.owner.GetLevel()
	interval := jsondata.GlobalUint("contactsRecommondFriendLevelPamram")
	if interval > sys.owner.GetLevel() {
		mi = 1
	} else {
		mi = lv - interval
	}
	ma = lv + interval
	return
}

func (sys *FriendSys) getRecommend() (frList []*pb3.FriendSimplyInfo) {
	var mark [3][]uint64
	useid := make(map[uint64]struct{})
	limit := int(jsondata.GlobalUint("contactsRecommondFriendNum"))
	minnLv, mxLv := sys.getRecommendLevel()
	limitLv := sys.GetAddLevel()
	var count int
	manager.AllOnlinePlayerDoCond(func(other iface.IPlayer) bool {
		if count >= limit {
			return false
		}
		if sys.owner.GetId() == other.GetId() || other.GetLevel() < limitLv || sys.IsExistFriend(other.GetId(), custom_id.FrFriend) {
			return true
		}
		if other.GetLevel() >= minnLv && other.GetLevel() <= mxLv {
			mark[0] = append(mark[0], other.GetId())
			count++
			return true
		}
		mark[1] = append(mark[1], other.GetId())
		return true
	})
	if len(mark[0])+len(mark[1]) < limit {
		globalVar := gshare.GetStaticVar()
		for _, otherId := range globalVar.FriendRecommed {
			if sys.owner.GetId() == otherId || sys.IsExistFriend(otherId, custom_id.FrFriend) {
				continue
			}
			if nil == getCommonData(otherId) {
				continue
			}
			mark[2] = append(mark[2], otherId)
		}
	}
	for _, fs := range mark {
		for _, otherId := range fs {
			if _, ok := useid[otherId]; !ok {
				frList = append(frList, toFriendSimplyInfo(getCommonData(otherId), 0))
			}
			if len(frList) >= limit {
				break
			}
			useid[otherId] = struct{}{}
		}
	}
	return frList
}

func (sys *FriendSys) getRecommendByName(name string) (frList []*pb3.FriendSimplyInfo) {
	lvLimit := sys.GetAddLevel()
	mark := make(map[uint64]struct{})
	manager.AllOnlinePlayerDoCond(func(other iface.IPlayer) bool {
		if len(mark) >= custom_id.FrSearchNameLimit {
			return false
		}
		if sys.owner.GetId() == other.GetId() || other.GetLevel() < lvLimit {
			return true
		}
		if strings.Contains(other.GetName(), name) {
			mark[other.GetId()] = struct{}{}
		}
		return true
	})
	if len(mark) < custom_id.FrSearchNameLimit {
		fn := func(other *pb3.PlayerDataBase) bool {
			if len(mark) >= custom_id.FrSearchNameLimit {
				return false
			}
			if sys.owner.GetId() == other.GetId() || other.GetLv() < lvLimit {
				return true
			}
			if _, ok := mark[other.GetId()]; ok {
				return true
			}
			if strings.Contains(other.GetName(), name) {
				mark[other.GetId()] = struct{}{}
			}
			return true
		}
		manager.AllOfflineDataBaseDo(fn)
	}
	for otherId := range mark {
		frList = append(frList, toFriendSimplyInfo(getCommonData(otherId), 0))
	}
	return frList
}

func (sys *FriendSys) c2sGetRecommend(msg *base.Message) error {
	var req pb3.C2S_14_3
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	var frList []*pb3.FriendSimplyInfo
	switch req.SearchType {
	case custom_id.FrSearchRecommend:
		if len(sys.recommendMap) > 0 {
			frList = sys.recommendMap
			break
		}
		fallthrough
	case custom_id.FrSearchReFresh:
		frList = sys.getRecommend()
	case custom_id.FrSearchName:
		frList = sys.getRecommendByName(req.GetName())
	}

	sys.recommendMap = frList
	sys.owner.SendProto3(14, 3, &pb3.S2C_14_3{
		SearchType:    req.SearchType,
		RecommendList: frList,
	})
	return nil
}

func (sys *FriendSys) c2sClearRecord(msg *base.Message) error {
	var req pb3.C2S_14_10
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	binary := sys.owner.GetBinaryData()
	if req.GetId() == 0 {
		binary.FriendData.Records = make([]*pb3.BeKillData, 0)
		binary.FriendData.KillMxId = 0
		sys.s2cKilledList()
	} else {
		idx := 0
		lenRecord := len(binary.FriendData.Records)
		for idx = 0; idx < lenRecord; idx++ {
			if binary.FriendData.Records[idx].Id == req.GetId() {
				break
			}
		}
		binary.FriendData.Records = append(binary.FriendData.Records[:idx], binary.FriendData.Records[idx+1:]...)
		sys.owner.SendProto3(14, 10, &pb3.S2C_14_10{
			Id:     req.Id,
			Param1: req.Param1,
		})
	}
	return nil
}

func (sys *FriendSys) c2sProcessApplyBatch(msg *base.Message) error {
	var req pb3.C2S_14_7
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	for applyId := range sys.GetBinaryData().FriendData.ApplyMap {
		sys.processApply(req.GetHandle(), applyId, false)
	}
	sys.s2cApplyList()
	return nil
}

func (sys *FriendSys) processApply(handle uint32, applyId uint64, isSingle bool) bool {
	isAccept := handle == custom_id.FrApplyAccept
	//拒绝
	if !isAccept {
		return sys.refuseApply(handle, applyId)
	}

	if sys.illegalApply(applyId) {
		delete(sys.GetBinaryData().FriendData.ApplyMap, applyId)
		return false
	}

	if isSingle && sys.IsExistFriend(applyId, custom_id.FrBlack) { //单次确认拉出
		sys.DelFriend(applyId, custom_id.FrBlack)
	}

	sys.agreeApply(applyId)
	return true
}

func (sys *FriendSys) c2sProcessApply(msg *base.Message) error {
	var req pb3.C2S_14_6
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	success := sys.processApply(req.GetHandle(), req.GetId(), true)
	if success {
		sys.owner.SendProto3(14, 6, &pb3.S2C_14_6{
			Id:     req.Id,
			Handle: req.Handle,
			Param1: req.Param1,
			Param2: req.Param2,
		})
	}
	return nil
}

func (sys *FriendSys) c2sProcessRelationBatch(msg *base.Message) error {
	var req pb3.C2S_14_2
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if nil == req.Ids {
		return nil
	}
	rsp := &pb3.S2C_14_2{}
	for _, targetId := range req.Ids {
		success := sys.SendFriendApply(targetId)
		if success {
			rsp.Ids = append(rsp.Ids, targetId)
		}
	}
	sys.owner.SendProto3(14, 2, rsp)
	return nil
}

func (sys *FriendSys) c2sProcessRelation(msg *base.Message) error {
	var req pb3.C2S_14_1
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	var success bool
	switch req.FEvent {
	case custom_id.FrEventAddFriend:
		success = sys.SendFriendApply(req.PlayerId)
	case custom_id.FrEventDelFriend:
		success = sys.DelFriend(req.PlayerId, custom_id.FrFriend)
	case custom_id.FrEventAddBlack:
		success = sys.AddBlack(req.PlayerId)
	case custom_id.FrEventDelBlack:
		success = sys.DelFriend(req.PlayerId, custom_id.FrBlack)
	case custom_id.FrEventAddEnemy:
		success = sys.AddEnemy(req.PlayerId)
	case custom_id.FrEventDelEnemy:
		success = sys.DelFriend(req.PlayerId, custom_id.FrEnemy)
	}
	if success { //成功发送申请
		sys.owner.SendProto3(14, 1, &pb3.S2C_14_1{
			PlayerId: req.PlayerId,
			FEvent:   req.FEvent,
		})
	}
	return nil
}

func (sys *FriendSys) c2sFriendList(msg *base.Message) error {
	var req pb3.C2S_14_0
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	sys.s2cFriendList(req.FType)
	return nil
}

func (sys *FriendSys) IsLimit(fType uint32) bool {
	var count uint32
	for _, line := range sys.friendMap {
		if utils.IsSetBit(line.GetFType(), fType) {
			count++
		}
	}
	var limit uint32
	switch fType {
	case custom_id.FrFriend:
		limit = jsondata.GlobalUint("contactsMaxFriendNum")
	case custom_id.FrEnemy:
		limit = jsondata.GlobalUint("contactsMaxBlackListNum")
	case custom_id.FrBlack:
		limit = jsondata.GlobalUint("contactsMaxenemyNum")
	}
	return count >= limit
}

func (sys *FriendSys) AddEnemy(targetId uint64) bool {
	if targetId == sys.owner.GetId() {
		return false
	}
	targetInfo := getCommonData(targetId)
	if nil == targetInfo {
		sys.owner.SendTipMsg(sys.getNoOneTipsId(custom_id.FrEnemy))
		return false
	}
	if sys.IsLimit(custom_id.FrEnemy) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend9)
		return false
	}
	sys.addToFriendList(targetInfo, custom_id.FrEnemy)
	return true
}

func (sys *FriendSys) AddBlack(targetId uint64) bool {
	if targetId == sys.owner.GetId() {
		return false
	}
	targetInfo := getCommonData(targetId)
	if nil == targetInfo {
		sys.owner.SendTipMsg(sys.getNoOneTipsId(custom_id.FrBlack))
		return false
	}
	if sys.IsLimit(custom_id.FrBlack) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend9)
		return false
	}
	if sys.IsExistFriend(targetId, custom_id.FrFriend) { //是好友要删除
		sys.DelFriend(targetId, custom_id.FrFriend)
	}
	sys.addToFriendList(targetInfo, custom_id.FrBlack)
	return true
}

func (sys *FriendSys) SendFriendApply(targetId uint64) bool {
	if targetId == sys.owner.GetId() {
		return false
	}
	targetInfo := getCommonData(targetId)
	if nil == targetInfo {
		sys.owner.SendTipMsg(sys.getNoOneTipsId(custom_id.FrFriend))
		return false
	}

	if data := manager.GetData(targetId, gshare.ActorDataBase); data != nil {
		dataBase := data.(*pb3.PlayerDataBase)
		if dataBase == nil {
			return false
		}
		if dataBase.Setting != nil && dataBase.Setting.Bits&1<<custom_id.Setting_AutoRefuseFriendQequest != 0 {
			sys.owner.SendTipMsg(tipmsgid.TPFriendRefuseApply)
			return false
		}
	}

	lvLimit := sys.GetAddLevel()
	//有一方等级不足加好友
	if sys.owner.GetLevel() < lvLimit || targetInfo.Lv < lvLimit {
		sys.owner.SendTipMsg(tipmsgid.TPFriend1, lvLimit)
		return false
	}
	//已是好友
	if sys.IsExistFriend(targetId, custom_id.FrFriend) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend8)
		return false
	}
	//好友列表已满
	if sys.IsLimit(custom_id.FrFriend) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend4)
		return false
	}
	//在黑名单要捞出来
	if sys.IsExistFriend(targetId, custom_id.FrBlack) {
		sys.DelFriend(targetId, custom_id.FrBlack)
	}

	target := manager.GetPlayerPtrById(targetId)
	if nil == target { //不在线,默认申请成功,对方上线后处理
		// 发离线消息处理
		engine.SendPlayerMessage(targetId, gshare.OfflineFriendApply, &pb3.OfflineFriend{
			Id:         sys.owner.GetId(),
			HandleTime: time_util.Now().Unix(),
		})
		sys.owner.SendTipMsg(tipmsgid.TpFriendSend)
		return true
	}

	targetFrSys, ok := target.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if !ok {
		return false
	}
	//对方列表已满
	if targetFrSys.IsLimit(custom_id.FrFriend) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend5)
		return false
	}
	//在对方黑名单
	if targetFrSys.IsExistFriend(sys.owner.GetId(), custom_id.FrBlack) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend3)
		return false
	}

	targetFrData := targetFrSys.GetData()
	if isRefuseActive(targetFrData.ApplyMap[sys.owner.GetId()]) {
		return false
	}

	targetFrSys.addToApplyList(sys.owner)

	sys.owner.SendTipMsg(tipmsgid.TpFriendSend)

	return true
}

func (sys *FriendSys) getNoOneTipsId(relation uint32) uint32 {
	if sys.owner != nil && sys.owner.GetActorProxy() != nil && sys.owner.GetActorProxy().GetProxyType().IsCrossSrv() {
		return tipmsgid.TpShouldReturnLocal
	}

	switch relation {
	case custom_id.FrFriend:
		return tipmsgid.FriendBanOtherCamp
	case custom_id.FrBlack:
		return tipmsgid.FriendBlackBanOtherCamp
	case custom_id.FrEnemy:
		return tipmsgid.FriendEnemyBanOtherCamp
	}

	return tipmsgid.TpAddFriendNoOne
}

var addFriendBoth = []uint32{custom_id.FrFriend, custom_id.FrEngagement, custom_id.FrMarry}

func (sys *FriendSys) DelFriend(targetId uint64, fType uint32) bool {
	friend, ok := sys.friendMap[targetId]
	if !ok {
		return false
	}
	if !sys.IsExistFriend(targetId, fType) {
		return false
	}
	if fType == custom_id.FrFriend { //有婚姻关系无法删除好友
		if sys.IsExistFriend(targetId, custom_id.FrEngagement) {
			return false
		}
		if sys.IsExistFriend(targetId, custom_id.FrMarry) {
			return false
		}
	}
	oldType := sys.friendMap[targetId].FType
	newType := utils.ClearBit(oldType, fType)
	friend.FType = newType
	if newType <= 0 { //无任何关系
		delete(sys.friendMap, targetId)
	}
	ownerId := sys.owner.GetId()
	sys.SaveFriend(targetId, fType, custom_id.FrDBDelete)
	if utils.SliceContainsUint32(addFriendBoth, fType) { //从对方好友列表接触好友关系
		target := manager.GetPlayerPtrById(targetId)
		if nil != target { //对方在线
			if targetFrSys, ok := target.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
				if !targetFrSys.IsExistFriend(ownerId, fType) {
					return false
				}
				oldType = targetFrSys.friendMap[ownerId].FType
				newType = utils.ClearBit(oldType, fType)
				targetFrSys.friendMap[ownerId].FType = newType
				delObj := targetFrSys.friendMap[ownerId]
				if newType <= 0 {
					delete(targetFrSys.friendMap, ownerId)
				}
				targetFrSys.SendFriendInfo(delObj)
			}
		}
		if fType == custom_id.FrFriend {
			friendmgr.GetIntimacyMgr().DelIntimacy(ownerId, targetId)
			onDelFriendSuccess(ownerId, targetId)
		}
		gshare.SendDBMsg(custom_id.GMsgUpdateFriends, targetId, sys.owner.GetId(), fType, custom_id.FrDBDelete)
	}
	sys.SendFriendInfo(friend)
	return true
}

func isRefuseActive(refuseTime int64) bool {
	if refuseTime >= 0 {
		return false
	}
	refuseTime = -refuseTime
	now := time_util.Now().Unix()
	diff := now - refuseTime
	cd := int64(jsondata.GlobalUint("contactsRejectedAddAgainCd"))
	return diff < cd*24*60*60
}

func (sys *FriendSys) addToApplyList(applier iface.IPlayer) {
	binary := sys.GetBinaryData()
	if isRefuseActive(binary.FriendData.ApplyMap[applier.GetId()]) {
		return
	}

	now := time_util.Now().Unix()
	binary.FriendData.ApplyMap[applier.GetId()] = now
	newApply := &pb3.FriendSimplyInfo{
		PlayerId:  applier.GetId(),
		Name:      applier.GetName(),
		Job:       applier.GetJob(),
		Vip:       applier.GetVipLevel(),
		Circle:    applier.GetCircle(),
		Level:     applier.GetLevel(),
		ApplyTime: uint64(now),
	}
	sys.SendProto3(14, 5, &pb3.S2C_14_5{Apply: newApply})
}

func (sys *FriendSys) addToFriendList(friend *pb3.SimplyPlayerData, fType uint32) {
	if nil == friend {
		return
	}
	actorId := friend.GetId()
	if data, ok := sys.friendMap[friend.GetId()]; ok {
		data.FType = utils.SetBit(data.FType, fType)
	} else {
		newFr := createFriendInfo(actorId, utils.SetBit(0, fType))
		sys.friendMap[friend.GetId()] = newFr
	}
	sys.SaveFriend(friend.GetId(), fType, custom_id.FrDBAdd)
	sys.SendFriendInfo(sys.friendMap[friend.GetId()])
}

func createFriendInfo(actorId uint64, fType uint32) *pb3.FriendInfo {
	return &pb3.FriendInfo{
		FType:   fType,
		ActorId: actorId,
	}
}

func (sys *FriendSys) IsInAnyStatus(bits uint32) bool {
	for _, v := range sys.friendMap {
		if (bits & v.FType) > 0 {
			return true
		}
	}
	return false
}

func (sys *FriendSys) IsExistFriend(actorId uint64, fType uint32) bool {
	data, ok := sys.friendMap[actorId]
	if !ok {
		return false
	}
	return utils.IsSetBit(data.GetFType(), fType)
}

func (sys *FriendSys) IsExistApply(actorId uint64) bool {
	binary := sys.GetBinaryData()
	_, ok := binary.FriendData.ApplyMap[actorId]
	return ok
}

// 数据库加载好友返回
func onLoadFriendData(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadFriendData", 2, len(args)) {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok {
		return
	}
	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}

	ret, ok := args[1].([]map[string]string)
	if !ok {
		return
	}

	if sys, ok := actor.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
		sys.OnLoadFriend(ret)
	}
}

func (sys *FriendSys) s2cAllFriendList() {
	for fType := custom_id.FrFriend; fType <= custom_id.FrMarry; fType++ {
		sys.s2cFriendList(uint32(fType))
	}
}

func (sys *FriendSys) s2cFriendList(fType uint32) {
	resp := &pb3.S2C_14_0{
		FType: fType,
	}
	for _, friendInfo := range sys.friendMap {
		if !utils.IsSetBit(friendInfo.FType, fType) {
			continue
		}
		resp.Friends = append(resp.Friends, sys.PackFriendInfoToClient(friendInfo))
	}
	sys.SendProto3(14, 0, resp)
}

func (sys *FriendSys) s2cApplyList() {
	applyList := make(map[uint64]*pb3.FriendSimplyInfo)
	for tarId, ts := range sys.owner.GetBinaryData().FriendData.ApplyMap {
		if ts < 0 {
			continue
		}
		fsi := toFriendSimplyInfo(getCommonData(tarId), uint64(ts))
		applyList[tarId] = fsi
	}
	sys.SendProto3(14, 4, &pb3.S2C_14_4{ApplyList: functional.MapToSlice(applyList)})
}

func (sys *FriendSys) s2cKilledList() {
	binary := sys.GetBinaryData()
	records := make([]*pb3.BeKilledRecord, 0, len(binary.FriendData.Records))
	for _, rc := range binary.FriendData.Records {
		killer := getCommonData(rc.GetPlayerId())
		if nil == killer {
			continue
		}
		records = append(records, &pb3.BeKilledRecord{
			Id:         rc.GetId(),
			PlayerId:   rc.GetPlayerId(),
			Name:       killer.GetName(),
			Job:        killer.GetJob(),
			Circle:     killer.GetCircle(),
			Level:      killer.GetLv(),
			Vip:        killer.GetVipLv(),
			SenceId:    rc.SenceId,
			CreateTime: rc.CreateTime,
		})
	}
	sys.SendProto3(14, 8, &pb3.S2C_14_8{Records: records})
}

func getCommonData(id uint64) *pb3.SimplyPlayerData {
	data := manager.GetSimplyData(id)
	if data == nil {
		return nil
	}
	return data
}

// 加载好友数据
func (sys *FriendSys) OnLoadFriend(ret []map[string]string) {
	sys.friendMap = make(map[uint64]*pb3.FriendInfo, len(ret))
	ownerId := sys.owner.GetId()
	for _, line := range ret {
		var (
			fId      = utils.AtoUint64(line["friend_id"])
			ftype    = utils.AtoUint32(line["f_type"])
			intimacy = utils.AtoUint32(line["intimacy"])
		)

		fi := createFriendInfo(fId, ftype)

		if utils.IsSetBit(ftype, custom_id.FrFriend) {
			if ok, _, _ := friendmgr.GetIntimacyMgr().IsIntimacyLoad(ownerId, fId); !ok {
				friendmgr.GetIntimacyMgr().AddIntimacy(ownerId, fId, intimacy, false, false)
			}

			if ok, val, lv := friendmgr.GetIntimacyMgr().IsIntimacyLoad(ownerId, fId); ok {
				fi.Intimacy = val
				fi.IntimacyLv = lv
			}
		}

		sys.friendMap[fId] = fi

		//好友在线
		friend := manager.GetPlayerPtrById(fId)
		if nil != friend {
			if fSys, ok := friend.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
				fSys.UpdateFriendLogoutTime(ownerId, 0)
				if fSys.IsExistFriend(ownerId, custom_id.FrFriend) {
					friend.SendTipMsg(tipmsgid.TPFriend7, sys.owner.GetName())
				}
			}
		}
	}

	sys.fillMarryInfo()
	sys.s2cAllFriendList()
}

func (sys *FriendSys) fillMarryInfo() {
	marryData := sys.GetBinaryData().MarryData
	if fd, ok := friendmgr.GetFriendCommonDataById(marryData.CommonId); ok {
		marryId := utils.Ternary(fd.ActorId1 != sys.owner.GetId(), fd.ActorId1, fd.ActorId2).(uint64)
		if friendmgr.IsExistStatus(marryData.CommonId, custom_id.FsEngagement) {
			sys.friendMap[marryId].FType = utils.SetBit(sys.friendMap[marryId].FType, custom_id.FrEngagement)
		}
		if friendmgr.IsExistStatus(marryData.CommonId, custom_id.FsMarry) {
			sys.friendMap[marryId].FType = utils.SetBit(sys.friendMap[marryId].FType, custom_id.FrFriend)
		}
	}
}

func (sys *FriendSys) GetFriend(id uint64) *pb3.FriendInfo {
	return sys.friendMap[id]
}

func onFriendSysNewDay(player iface.IPlayer, args ...interface{}) {
	binary := player.GetBinaryData()
	if nil == binary {
		return
	}
	if nil == binary.FriendData {
		return
	}
	rcDayLimit := int64(jsondata.GlobalUint("contactsKillRecordSaveDays"))
	minn := 0
	if nil != binary.FriendData.Records {
		for i, record := range binary.FriendData.Records {
			crTime := int64(record.GetCreateTime())
			diff := time_util.Now().Unix() - crTime
			if diff >= rcDayLimit*24*60*60 {
				minn = i + 1
			}
		}
		if minn > 0 {
			binary.FriendData.Records = binary.FriendData.Records[minn:]
			if len(binary.FriendData.Records) == 0 {
				binary.FriendData.KillMxId = 1
			}
		}
	}
	if friendSys := player.GetSysObj(sysdef.SiFriend).(*FriendSys); nil != friendSys && friendSys.IsOpen() {
		friendSys.s2cKilledList()
	}
	binary.FriendData.DailyAdd = 0
	player.SetExtraAttr(attrdef.FriendDailyAdd, int64(binary.FriendData.DailyAdd))
}

func (sys *FriendSys) PackFriendInfoToClient(fr *pb3.FriendInfo) *pb3.FriendInfoToClient {
	playerDataBase := getCommonData(fr.ActorId)
	return &pb3.FriendInfoToClient{
		PlayerInfo: playerDataBase,
		FType:      fr.FType,
		Intimacy:   fr.Intimacy,
		IntimacyLv: fr.IntimacyLv,
	}
}

func (sys *FriendSys) SendFriendInfo(fr *pb3.FriendInfo) {
	toClient := sys.PackFriendInfoToClient(fr)
	sys.SendProto3(14, 11, &pb3.S2C_14_11{Friend: toClient})
}

func (sys *FriendSys) UpdateFriendLogoutTime(friendId uint64, logoutTime uint32) {
	if fr := sys.GetFriend(friendId); nil != fr {
		sys.SendFriendInfo(fr)
	}
}

func (sys *FriendSys) addKilledRecord(killId uint64, sceneId uint32) {
	killer := getCommonData(killId)
	if nil == killer {
		return
	}
	binary := sys.GetBinaryData()
	binary.FriendData.KillMxId++
	newRecord := &pb3.BeKillData{
		Id:         binary.FriendData.KillMxId,
		PlayerId:   killer.GetId(),
		SenceId:    sceneId,
		CreateTime: uint64(time_util.Now().Unix()),
	}
	clientRecord := &pb3.BeKilledRecord{
		Id:         binary.FriendData.KillMxId,
		PlayerId:   killer.GetId(),
		Name:       killer.GetName(),
		Job:        killer.GetJob(),
		Circle:     killer.GetCircle(),
		Level:      killer.GetLv(),
		Vip:        killer.GetVipLv(),
		SenceId:    sceneId,
		CreateTime: uint64(time_util.Now().Unix()),
	}
	binary.FriendData.Records = append(binary.FriendData.Records, newRecord)
	rcNumLimit := jsondata.GlobalUint("contactsKillRecordNum")
	if len(binary.FriendData.Records) > int(rcNumLimit) {
		binary.FriendData.Records = binary.FriendData.Records[1:]
	}
	sys.SendProto3(14, 9, &pb3.S2C_14_9{Records: clientRecord})
}

func (sys *FriendSys) c2sApplyList(msg *base.Message) error {
	sys.s2cApplyList()
	return nil
}

func (sys *FriendSys) SaveFriend(fId uint64, fType uint32, op uint32) {
	gshare.SendDBMsg(custom_id.GMsgUpdateFriends, sys.owner.GetId(), fId, fType, op)
}

func checkTodayFriendAdd(actorId uint64) bool {
	todayAdd := manager.GetOnlineAttr(actorId, custom_id.OnlineAttrFriendDayAdd)
	dailyAddLimit := jsondata.GlobalUint("contactsDailyAddFriendNum")
	return todayAdd < dailyAddLimit
}

func checkTotalFriendAdd(actorId uint64) bool {
	totalAdd := manager.GetOnlineAttr(actorId, custom_id.OnlineAttrFriendTotal)
	totalLimit := jsondata.GlobalUint("contactsMaxFriendNum")
	return totalAdd < totalLimit
}

func (sys *FriendSys) refuseApply(handle uint32, applyId uint64) bool {
	if !sys.IsExistApply(applyId) {
		return false
	}
	if handle == custom_id.FrApplyRefuseAndLimit { //拒绝cd日
		sys.GetBinaryData().FriendData.ApplyMap[applyId] = -1 * time_util.Now().Unix()
	}
	if sys.GetBinaryData().FriendData.ApplyMap[applyId] >= 0 {
		delete(sys.GetBinaryData().FriendData.ApplyMap, applyId)
	}
	if applier := manager.GetPlayerPtrById(applyId); nil != applier { //在线通知对方拒绝
		applier.SendTipMsg(tipmsgid.TPFriendRefuseApply)
	}
	return true
}

func (sys *FriendSys) illegalApply(applyId uint64) bool {
	if applyId == sys.owner.GetId() { //添加人不能是自己
		return true
	}
	if sys.IsExistFriend(applyId, custom_id.FrFriend) {
		return true
	}
	if !gshare.IsActorInThisServer(applyId) {
		return true
	}
	limitLv := sys.GetAddLevel()
	if limitLv > sys.owner.GetLevel() || limitLv > uint32(manager.GetExtraAttr(applyId, attrdef.Level)) {
		return true
	}
	return false
}

func (sys *FriendSys) addOnline(applier iface.IPlayer) bool {
	delete(sys.GetBinaryData().FriendData.ApplyMap, applier.GetId())
	applySys, ok := applier.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if !ok {
		return true
	}
	// 我已被对方拉入黑名单
	if applySys.IsExistFriend(sys.owner.GetId(), custom_id.FrBlack) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend3)
		return true
	}
	if applySys.IsLimit(custom_id.FrFriend) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend5) // 对方的好友列表已满
		return true
	}
	applySys.addToFriendList(getCommonData(sys.owner.GetId()), custom_id.FrFriend)
	sys.addToFriendList(manager.GetSimplyData(applier.GetId()), custom_id.FrFriend)
	sys.owner.SendTipMsg(tipmsgid.Tpmyfriend, applier.GetName())
	onAddFriendSuccess(sys.owner, applier.GetId(), applier.GetName())
	return true
}

func onAddFriendSuccess(actor iface.IPlayer, friendId uint64, friendName string) {
	manager.SetOnlineAttr(actor.GetId(), custom_id.OnlineAttrFriendDayAdd, 1, true)
	manager.SetOnlineAttr(actor.GetId(), custom_id.OnlineAttrFriendTotal, 1, true)
	actor.TriggerEvent(custom_id.AeAddFriends, friendName)
	engine.SendPlayerMessage(friendId, gshare.OfflineAcceptFriend, &pb3.OfflineAcceptFriend{
		Name:       actor.GetName(),
		AcceptTime: time_util.NowSec(),
	})
}

func onDelFriendSuccess(actorId1, actorId uint64) {
	val1 := manager.GetOnlineAttr(actorId1, custom_id.OnlineAttrFriendTotal)
	manager.SetOnlineAttr(actorId1, custom_id.OnlineAttrFriendTotal, uint32(utils.MaxInt(int(val1)-1, 0)), false)
	val2 := manager.GetOnlineAttr(actorId, custom_id.OnlineAttrFriendTotal)
	manager.SetOnlineAttr(actorId, custom_id.OnlineAttrFriendTotal, uint32(utils.MaxInt(int(val2)-1, 0)), false)
}

func (sys *FriendSys) agreeApply(applyId uint64) bool {
	if !sys.checkStatus(applyId) {
		return false
	}

	if applier := manager.GetPlayerPtrById(applyId); nil != applier {
		return sys.addOnline(applier)
	}
	gshare.SendDBMsg(custom_id.GMsGetFriendStatus, sys.owner.GetId(), applyId)
	return true
}

func (sys *FriendSys) checkStatus(applyId uint64) bool {
	if !sys.IsExistApply(applyId) {
		return false
	}
	myId := sys.owner.GetId()
	if !checkTodayFriendAdd(myId) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend2)
		return false
	}
	if !checkTotalFriendAdd(myId) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend4)
		return false
	}
	if !checkTodayFriendAdd(applyId) {
		sys.owner.SendTipMsg(tipmsgid.TpOtherAddFriendLimitToday)
		return false
	}
	if !checkTotalFriendAdd(applyId) {
		sys.owner.SendTipMsg(tipmsgid.TPFriend5)
		return false
	}
	return true
}

func onGetFriendStatus(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadFriendData", 3, len(args)) {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok {
		return
	}
	applyId, ok := args[1].(uint64)
	if !ok {
		return
	}
	actor := manager.GetPlayerPtrById(actorId)
	if nil == actor {
		return
	}

	ret, ok := args[2].([]map[string]string)
	if !ok || len(ret) <= 0 {
		return
	}
	blackCount := utils.AtoUint32(ret[0]["isblack"])
	if sys, ok := actor.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
		if !sys.checkStatus(applyId) {
			return
		}
		if applier := manager.GetPlayerPtrById(applyId); nil != applier {
			sys.addOnline(applier)
			return
		}
		delete(sys.GetBinaryData().FriendData.ApplyMap, applyId)
		if blackCount > 0 {
			sys.owner.SendTipMsg(tipmsgid.TPFriend3)
			return
		}
		applyInfo := getCommonData(applyId)
		sys.addToFriendList(getCommonData(applyId), custom_id.FrFriend)
		sys.owner.SendTipMsg(tipmsgid.Tpmyfriend, applyInfo.GetName())
		onAddFriendSuccess(sys.owner, applyId, applyInfo.GetName())
		sys.s2cApplyList()
		gshare.SendDBMsg(custom_id.GMsgUpdateFriends, applyId, sys.owner.GetId(), uint32(custom_id.FrFriend), custom_id.FrDBAdd)
	}
}

func onFriendInfoChange(player iface.IPlayer, args ...interface{}) {
	if sys, ok := player.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
		sys.OnFriendInfoChange()
	}
}

func onFriendInfoLvChange(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	if sys, ok := player.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
		if oldLv, ok := args[0].(uint32); ok {
			minnLv := jsondata.GlobalUint("contactsRecommondLevelLimit")
			if oldLv < minnLv && player.GetLevel() >= minnLv {
				sys.ToFriendRecommend()
			}
		}
	}
}

func (sys *FriendSys) OnFriendInfoChange() {
	actorId := sys.owner.GetId()
	for friendId := range sys.friendMap {
		friend := manager.GetPlayerPtrById(friendId)
		if nil == friend {
			continue
		}
		fSys, ok := friend.GetSysObj(sysdef.SiFriend).(*FriendSys)
		if !ok {
			continue
		}
		if data := fSys.GetFriend(actorId); nil != data {
			fSys.SendFriendInfo(data)
		}
	}
}

func onKillByOther(player iface.IPlayer, args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	killerId, ok := args[0].(uint64)
	if !ok {
		return
	}
	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}
	if nil == player {
		return
	}
	if sys, ok := player.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
		sys.addKilledRecord(killerId, sceneId)
	}
}

func offlineFriendApply(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineFriend)
	if !ok {
		return
	}
	frSys, ok := player.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if !ok || !frSys.IsOpen() {
		return
	}

	if frSys.IsExistFriend(st.Id, custom_id.FrBlack) {
		return
	}

	binary := player.GetBinaryData()
	if isRefuseActive(binary.FriendData.ApplyMap[st.Id]) { //还在拒绝中
		return
	}
	binary.FriendData.ApplyMap[st.Id] = st.HandleTime
	applier := getCommonData(st.Id)
	newApply := &pb3.FriendSimplyInfo{
		PlayerId:  applier.GetId(),
		Name:      applier.GetName(),
		Job:       applier.GetJob(),
		Circle:    applier.GetCircle(),
		Level:     applier.GetLv(),
		ApplyTime: uint64(st.HandleTime),
	}
	frSys.SendProto3(14, 5, &pb3.S2C_14_5{Apply: newApply})
}

func offlineAcceptFriend(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineAcceptFriend)
	if !ok {
		return
	}
	player.TriggerEvent(custom_id.AeAddFriends, st.Name)
}

func saveFriendRelation(friendId1, friendId2 uint64, fType uint32) {
	if f1 := manager.GetPlayerPtrById(friendId1); nil != f1 {
		fsys := f1.GetSysObj(sysdef.SiFriend).(*FriendSys)
		fsys.addToFriendList(getCommonData(friendId2), fType)
	} else {
		gshare.SendDBMsg(custom_id.GMsgUpdateFriends, friendId1, friendId2, fType, custom_id.FrDBAdd)
	}
	if f2 := manager.GetPlayerPtrById(friendId2); nil != f2 {
		fsys := f2.GetSysObj(sysdef.SiFriend).(*FriendSys)
		fsys.addToFriendList(getCommonData(friendId1), fType)
	} else {
		gshare.SendDBMsg(custom_id.GMsgUpdateFriends, friendId2, friendId1, fType, custom_id.FrDBAdd)
	}
}

func addIntimacy(actor iface.IPlayer, args ...string) bool {
	length := len(args)
	if length < 2 {
		return false
	}
	id := utils.AtoUint64(args[0])
	exp := utils.AtoUint32(args[1])
	friendSys := actor.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if nil == friendSys || !friendSys.IsOpen() {
		return false
	}
	friendSys.AddIntimacy(id, exp)
	return true
}

func checkIntimacyBuffChange(player iface.IPlayer, args ...interface{}) {
	friendId := args[0].(uint64)
	lv, oldLv := args[1].(uint32), args[3].(uint32)
	if lv == oldLv {
		return
	}
	friend := manager.GetPlayerPtrById(friendId)
	if nil == friend {
		return
	}
	if !teammgr.IsSameTeam(player, friend) {
		return
	}
	calcTeamIntimacy(player.GetTeamId())
}

func calcTeamIntimacy(teamId uint64) {
	teamPb, err := teammgr.GetTeamPb(teamId)
	if nil != err {
		return
	}

	lvMap := map[uint64]uint32{}

	var playerList []iface.IPlayer
	for _, v := range teamPb.GetMembers() {
		actorId := v.GetPlayerInfo().GetId()
		actor := manager.GetPlayerPtrById(actorId)
		if nil == actor {
			continue
		}
		playerList = append(playerList, actor)
		err := actor.CallActorFunc(actorfuncid.G2FClearIntimacyBuff, nil)
		if err != nil {
			logger.LogError("err:%v", err)
			continue
		}
	}

	for _, actor := range playerList {
		if nil == actor {
			continue
		}
		friendSys := actor.GetSysObj(sysdef.SiFriend).(*FriendSys)
		if nil == friendSys || !friendSys.IsOpen() {
			continue
		}
		for _, friend := range playerList {
			if friend.GetId() == actor.GetId() {
				continue
			}
			lv := friendSys.GetIntimacyLv(friend.GetId())
			lvMap[actor.GetId()] = utils.MaxUInt32(lvMap[actor.GetId()], lv)
			lvMap[friend.GetId()] = utils.MaxUInt32(lvMap[friend.GetId()], lv)
		}
	}

	for _, actor := range playerList {
		buffId, ok := jsondata.GetIntimacyBuff(lvMap[actor.GetId()])
		if !ok {
			continue
		}
		actor.AddBuff(buffId)
	}
}

func onIntimacyChange(player iface.IPlayer, args ...interface{}) {
	friendId := args[0].(uint64)
	lv := args[1].(uint32)
	val := args[2].(uint32)
	fSys := player.GetSysObj(sysdef.SiFriend).(*FriendSys)
	if nil == fSys || !fSys.IsExistFriend(friendId, custom_id.FrFriend) {
		return
	}
	friendObj := fSys.friendMap[friendId]
	friendObj.Intimacy = val
	friendObj.IntimacyLv = lv
	fSys.SendFriendInfo(friendObj)
}

func onFriendAdd(player iface.IPlayer, args ...interface{}) {
	player.TriggerQuestEvent(custom_id.QttAddFriendTimes, 0, 1)
}

func init() {
	RegisterSysClass(sysdef.SiFriend, func() iface.ISystem {
		return &FriendSys{}
	})

	gmevent.Register("addIntimacy", addIntimacy, 1)

	net.RegisterSysProto(14, 0, sysdef.SiFriend, (*FriendSys).c2sFriendList)
	net.RegisterSysProto(14, 1, sysdef.SiFriend, (*FriendSys).c2sProcessRelation)
	net.RegisterSysProto(14, 2, sysdef.SiFriend, (*FriendSys).c2sProcessRelationBatch)
	net.RegisterSysProto(14, 3, sysdef.SiFriend, (*FriendSys).c2sGetRecommend)
	net.RegisterSysProto(14, 6, sysdef.SiFriend, (*FriendSys).c2sProcessApply)
	net.RegisterSysProto(14, 7, sysdef.SiFriend, (*FriendSys).c2sProcessApplyBatch)
	net.RegisterSysProto(14, 10, sysdef.SiFriend, (*FriendSys).c2sClearRecord)
	net.RegisterSysProto(14, 4, sysdef.SiFriend, (*FriendSys).c2sApplyList)

	event.RegActorEvent(custom_id.AeKillByActor, onKillByOther)
	event.RegActorEvent(custom_id.AeNewDay, onFriendSysNewDay)

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadFriendData, onLoadFriendData)
		gshare.RegisterGameMsgHandler(custom_id.GMsGetFriendStatusRet, onGetFriendStatus)
	})

	event.RegSysEvent(custom_id.SeTeamMemberChange, func(args ...interface{}) {
		if len(args) < 1 {
			return
		}
		teamId := args[0].(uint64)
		calcTeamIntimacy(teamId)
	})

	engine.RegisterMessage(gshare.OfflineFriendApply, func() pb3.Message {
		return &pb3.OfflineFriend{}
	}, offlineFriendApply)

	engine.RegisterMessage(gshare.OfflineAcceptFriend, func() pb3.Message {
		return &pb3.OfflineFriend{}
	}, offlineAcceptFriend)

	event.RegActorEvent(custom_id.AeAddFriends, onFriendAdd)
	event.RegActorEvent(custom_id.AeVipLevelUp, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeLevelUp, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeLevelUp, onFriendInfoLvChange)
	event.RegActorEvent(custom_id.AeCircleChange, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeChangeName, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeJoinGuild, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeLeaveGuild, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeHeadChange, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeHeadFrameChange, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeMarryStatusChange, onFriendInfoChange)
	event.RegActorEvent(custom_id.AeIntimacyChange, checkIntimacyBuffChange)
	event.RegActorEvent(custom_id.AeIntimacyChange, onIntimacyChange)
}
