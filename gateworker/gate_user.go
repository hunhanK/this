/**
 * @Author: ChenJunJi
 * @Desc: 网关用户
 * @Date: 2021/9/26 10:29
 */

package gateworker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/db/mysql"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	series2 "jjyz/gameserver/engine/series"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/invitecodemgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"unicode/utf8"

	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

// MaxActorCount 玩家最大角色数量
const MaxActorCount = 4
const CanRecoverySec = 259200

const (
	Verify          = 1<<8 | 1
	PlayerList      = 1<<8 | 2
	CreatePlayer    = 1<<8 | 3
	EnterGame       = 1<<8 | 4
	DeletePlayer    = 1<<8 | 6
	Reconnect       = 1<<8 | 10
	CheckInviteCode = 1<<8 | 14
)

const (
	sNone              = iota
	sCreatePlayerCheck // 创角检测
	sCreatePlayer      // 创角
)

type BootObj struct {
	GateId       uint32 //网关id
	ConnId       uint32 //网关连接id
	ActorId      uint64 //玩家id
	UserId       uint32 //账号id
	GmLevel      uint32 //gm等级
	Status       uint32 //actors表的status
	RemoteAddr   string //ip地址
	TAAccountId  string //ta账户id
	TADistinctId string //ta访客id
	RegisteTime  uint32 //注册时间
	AccountName  string //账号
}

// UserSt 网关用户
type UserSt struct {
	GateId uint32 // 网关id
	ConnId uint32 // 索引

	UserId      uint32 // 账号id
	AccountName string // 账号名
	ServerId    uint32 // 从哪个服务器接口登录的

	ActorId uint64 // 角色id

	RemoteAddr      string // 客户端ip
	GmLevel         uint32 // gm等级
	Closed          bool   // 是否已关闭
	IsInvite        bool
	Status          int
	CreatePlayerObj *pb3.C2S_1_3
}

type CreatePlayerCheckName struct {
	UserId  uint32
	ActorId uint64
}

type WordCheckCreatePlayerName struct {
	UserId     uint32
	InviteCode string
}

func (st *UserSt) GetPlayerId() uint64 {
	return st.ActorId
}

func (st *UserSt) Reset() {
	st.UserId = 0
	st.AccountName = ""
	st.ActorId = 0
	st.RemoteAddr = ""
	st.RemoteAddr = ""
	st.GmLevel = 0
	st.Closed = true
	st.IsInvite = false
	st.Status = sNone
	st.CreatePlayerObj = nil
}

// OnRecv 收到前端协议
func (st *UserSt) OnRecv(data []byte) {
	if len(data) < 2 {
		logger.LogError("gate user read buffer len less then 2!!!")
		return
	}
	cmdId := binary.LittleEndian.Uint16(data[:2])

	var protoIdH, protoIdL = cmdId >> 8, cmdId & 0xff
	// skip heart beat
	if protoIdH != 2 && protoIdL != 254 {
		logger.LogTrace("user(id:%d) actor(id:%d) gateway request proto:C2S_%d_%d", st.UserId, st.ActorId, protoIdH, protoIdL)
	}

	buff := data[2:]
	switch cmdId {
	case Verify:
		st.c2sVerify(buff)
	case PlayerList:
		st.s2cPlayerList(buff)
	case CreatePlayer:
		st.c2sCreatePlayer(buff)
	case EnterGame:
		st.c2sEnterGame(buff)
	case Reconnect:
		st.c2sReconnect(buff)
	case DeletePlayer:
		st.c2sDeletePlayer(buff)
	case CheckInviteCode:
		st.c2sCheckInviteCode(buff)
	default:
		if st.ActorId > 0 {
			if actor := manager.GetPlayerPtrById(st.ActorId); nil != actor {
				msg := base.NewMessage()
				msg.SetCmd(cmdId)
				msg.Package(buff)

				actor.DoNetMsg(protoIdH, protoIdL, msg)
			}
		}
	}
}

func (st *UserSt) SendProto3(protoH, protoL uint16, msg pb3.Message) {
	gate := GetGateConn(st.GateId)
	if nil == gate {
		return
	}

	buff, err := pb3.Marshal(msg)
	if nil != err {
		logger.LogError("发送网关消息(%d,%d)失败. error:%v", protoH, protoL, err)
		return
	}

	clientData, err := pb3.Marshal(&pb3.ClientData{
		ConnId:  st.ConnId,
		ProtoId: uint32(protoH<<8 | protoL),
		Data:    buff,
	})
	if nil != err {
		logger.LogError("send proto marshal error! err:%v", err)
		return
	}

	if err := gate.SendMessage(base.GW_DATA, clientData); err != nil {
		logger.LogError("发送网关消息(%d,%d)失败. error:%v", protoH, protoL, err)
		return
	}

	if !(protoH == 2 && protoL == 254) {
		logger.LogTrace("[RPC.SendProto3] userId:%d req:%s{%+v}", st.UserId, msg.ProtoReflect().Descriptor().FullName(), msg)
	}
	logworker.LogNetProtoStat(uint32(protoH<<8|protoL), uint32(len(buff)), engine.GetPfId(), engine.GetServerId())
}

func (st *UserSt) SendProtoBuffer(protoH, protoL uint16, buffer []byte) {
	gate := GetGateConn(st.GateId)
	if nil == gate {
		return
	}

	clientData, err := pb3.Marshal(&pb3.ClientData{
		ConnId:  st.ConnId,
		ProtoId: uint32(protoH<<8 | protoL),
		Data:    buffer,
	})
	if nil != err {
		logger.LogError("send proto marshal error! err:%v", err)
		return
	}

	if logger.GetLevel() <= logger.TraceLevel {
		msgName := fmt.Sprintf("pb3.S2C_%d_%d", protoH, protoL)
		protoType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
		if err != nil {
			logger.LogError("(%d %d)SendProtoBuffer Failed err %s", protoH, protoL, err)
			return
		}
		msg := protoType.New().Interface()
		err = proto.Unmarshal(buffer, msg)
		if err != nil {
			logger.LogError("SendProtoBuffer failed err %s", err)
		}
		logger.LogTrace("player %d send msg %s {%+v}", st.GetPlayerId(), msg.ProtoReflect().Descriptor().FullName(), msg)
	}

	err = gate.SendMessage(base.GW_DATA, clientData)
	if err != nil {
		logger.LogError("(%d %d)SendProtoBuffer failed err %s", protoH, protoL, err)
	}
	logworker.LogNetProtoStat(uint32(protoH<<8|protoL), uint32(len(buffer)), engine.GetPfId(), engine.GetServerId())
}

// 删除角色，记录删除时间
// 若删除后3天内登录过该角色，移除删除时间
// 若3天内一直没登录过，不出现在选角列表
func (st *UserSt) c2sDeletePlayer(buff []byte) {
	var req pb3.C2S_1_6
	if nil != pb3.Unmarshal(buff, &req) {
		return
	}
	playerId := req.GetRoleId()
	if 0 == playerId {
		return
	}

	gshare.SendDBMsg(custom_id.GMsgDeletePlayer, playerId, st.UserId)
}

func (st *UserSt) c2sCheckInviteCode(buff []byte) {
	var req pb3.C2S_1_14
	if nil != pb3.Unmarshal(buff, &req) {
		return
	}

	rsp := &pb3.S2C_1_14{Code: req.GetCode()}

	if code := invitecodemgr.CheckCode(req.GetCode()); 0 != code {
		rsp.ErrCode = code
	}

	st.SendProto3(1, 14, rsp)
}

func (st *UserSt) onDeletePlayer(playerId uint64) {
	st.SendProto3(1, 6, &pb3.S2C_1_6{
		RoleId:          playerId,
		Ret:             0,
		CanRecoveryTime: time_util.NowSec() + CanRecoverySec,
	})
}

func (st *UserSt) c2sEnterGame(buff []byte) {
	logger.LogInfo("【登录流程】玩家请求进入游戏")
	var req pb3.C2S_1_4
	if err := pb3.Unmarshal(buff, &req); err != nil {
		logger.LogError("%v", err)
		return
	}
	actorId := req.GetRoleId()
	logger.LogInfo("【登录流程】actorId:%d", actorId)
	if 0 == actorId {
		logger.LogError("func c2sEnterGame actorId == 0 !!!!!!!!!!!!!")
		return
	}

	forbidIds := gshare.GetStaticVar().ForBidActorIds
	if forbidIds != nil {
		if _, ok := forbidIds[actorId]; ok {
			logger.LogError("func c2sEnterGame actorId=%d forbid", actorId)
			return
		}
	}

	actor := manager.GetPlayerPtrById(actorId)
	if nil != actor {
		if actor.GetUserId() != st.UserId {
			logger.LogError("userId:%d进入游戏未找到对应角色%d", st.UserId, st.ActorId)
			return
		}
		logger.LogInfo("【登录流程】角色不为空, 直接进入游戏")
		st.ActorId = actorId
		actor.ReLogin(false, st.GateId, st.ConnId, st.RemoteAddr)
		return
	}

	args := mysql.ClientEnterGame{
		UserId:       st.UserId,
		ActorId:      actorId,
		Ip:           utils.Ip2int(st.RemoteAddr),
		TaAccountId:  req.GetTaAccountId(),
		TaDistinctId: req.GetTaDistinctId(),
		RegisteTime:  req.GetRegisteTime(),
	}
	gshare.SendDBMsg(custom_id.GMsgClientEnterGame, args)
}

func (st *UserSt) onClientEnterGameRet(data mysql.ClientEnterGameRet) {
	st.ActorId = data.ActorId

	logger.LogInfo("【登录流程】角色为空, 启动数据加载 Boot start")
	st.Boot(data.TaAccountId, data.TaDistinctId, data.RegisteTime, data.Status)

	st.SendProto3(1, 5, &pb3.S2C_1_5{ErrCode: 0})
	logger.LogDebug("enterGame actorId:%d userId:%d", data.ActorId, st.UserId)
}

func (st *UserSt) Boot(taAccountId, taDistinctId string, registeTime, status uint32) {
	actorId := st.ActorId
	if obj, ok := BootMap[actorId]; !ok {
		BootMap[st.ActorId] = &BootObj{
			GateId:       st.GateId,
			ConnId:       st.ConnId,
			ActorId:      st.ActorId,
			UserId:       st.UserId,
			GmLevel:      st.GmLevel,
			RemoteAddr:   st.RemoteAddr,
			AccountName:  st.AccountName,
			Status:       status,
			TAAccountId:  taAccountId,
			TADistinctId: taDistinctId,
			RegisteTime:  registeTime,
		}

		logger.LogInfo("【登录流程】发送到DB线程加载玩家数据")
		gshare.SendDBMsg(custom_id.GMsgLoadActorData, st.ActorId)
	} else {
		//obj.GateId, obj.ConnId 需要把旧的socket踢掉
		if obj.GateId != st.GateId && obj.ConnId != st.ConnId {
			SendGateCloseUser(obj.GateId, obj.ConnId)
			obj.GateId = st.GateId
			obj.ConnId = st.ConnId
		}

		logger.LogError("bootMap[%v] == nil", actorId)
	}
}

func (st *UserSt) c2sVerify(buff []byte) {
	var req pb3.C2S_1_1
	if nil != pb3.Unmarshal(buff, &req) {
		return
	}

	if engine.GetDisableLogin() {
		return
	}

	gshare.SendDBMsg(custom_id.GMsgVerifyAccount, mysql.VerifyAccount{
		GateId:   st.GateId,
		ConnId:   st.ConnId,
		Account:  req.GetAccount(),
		Token:    req.GetToken(),
		ServerId: req.GetServerId(),
	})
}

func (st *UserSt) onVerifyRet(ret mysql.VerifyRet) {
	rsp := &pb3.S2C_1_1{ErrCode: 0, TimeStamp: time_util.Now().UnixMilli()}

	rsp.ErrCode = uint32(ret.Errno)
	if ret.Errno > 0 {
		st.SendProto3(1, 1, rsp)
		logger.LogDebug(" send 1-1 ")
		return
	}

	st.AccountName = ret.Account
	st.UserId = ret.UserId
	st.ServerId = ret.ServerId
	st.IsInvite = ret.IsInvite
	st.GmLevel = ret.GmLevel

	exist := GetGateUserByUserId(st.UserId)
	if nil != exist {
		if exist.GateId == st.GateId && exist.ConnId == st.ConnId {
			return
		}
		engine.CloseGateUser(exist, cmd.DCRReplace)
	}

	AddGateUser(st)
	st.SendProto3(1, 1, rsp)
	logger.LogDebug(" send 1-1 ")
}

func (st *UserSt) s2cPlayerList(buff []byte) {
	if st.UserId <= 0 {
		return
	}
	gshare.SendDBMsg(custom_id.GMsgLoadPlayerList, mysql.LoadPlayerList{UserId: st.UserId, ServerId: st.ServerId})
}

func (st *UserSt) onLoadPlayerList(ret mysql.LoadPlayerList) {
	rsp := &pb3.S2C_1_2{}
	rsp.LastId = 0
	rsp.NeedInvite = engine.GetForbidCreatePlayer() && len(ret.Actors) == 0

	for _, actor := range ret.Actors {
		data := &pb3.RoleListData{}
		data.RoleId = actor.ActorId
		data.Job = actor.Job<<base.SexBit | actor.Sex
		data.Level = actor.Level
		data.Circle = actor.Circle
		data.Name = actor.ActorName
		data.BanTime = actor.BanTime
		data.AppearInfo = new(pb3.AppearInfo)

		if err := pb3.Unmarshal(actor.AppearInfo, data.AppearInfo); err != nil {
			logger.LogError("unmarshal appear info error:%v", err)
		} else if data.AppearInfo.Appear != nil {
			headData, ok := data.AppearInfo.Appear[appeardef.AppearPos_Head]
			if ok {
				data.Head = headData.AppearId
			}

			headFrameData, ok := data.AppearInfo.Appear[appeardef.AppearPos_HeadFrame]
			if ok {
				data.HeadFrame = headFrameData.AppearId
			}
		}

		if actor.RecoveryTime != 0 {
			data.CanRecoveryTime = actor.RecoveryTime + CanRecoverySec
		}
		rsp.List = append(rsp.List, data)
		if actor.Status&(1<<2) > 0 {
			rsp.LastId = actor.ActorId
		}
	}
	st.SendProto3(1, 2, rsp)
	logger.LogDebug("player list:%v", rsp)
}

func (st *UserSt) onCreatePlayerCheck(success bool, count uint32) {
	st.Status = sNone

	if !success {
		return
	}
	if count >= MaxActorCount {
		logger.LogWarn("%s 创角失败，已达到最大角色数量：%d", st.AccountName, count)
		return
	}
	logger.LogInfo("创建角色请求:%d [%d,%d]", st.UserId, st.GateId, st.ConnId)

	req := st.CreatePlayerObj
	if nil == req {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.DbCreateActorFailed})
		return
	}

	inviteCode := req.GetInviteCode()
	// 禁止创角
	if engine.GetForbidCreatePlayer() && (count <= 0 || st.IsInvite) {
		isInvite := false
		if inviteCode != "" {
			if code := invitecodemgr.CheckCode(inviteCode); 0 != code {
				st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: code})
				logger.LogWarn("邀请码错误 accountName:%s 邀请码:%s code:%d", st.AccountName, inviteCode, code)
				return
			}

			isInvite = true
		}
		if !isInvite {
			st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.ForbidCreatePlayer})
			logger.LogWarn("禁止未拥有角色玩家创建 accountName : %s", st.AccountName)
			return
		}
	}

	if code := adjustName(req); 0 != code {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: code})
		return
	}

	st.Status = sCreatePlayer

	var ditchId uint32
	if obj := st.CreatePlayerObj; nil != obj {
		ditchId = obj.GetDitchId()
	}

	engine.SendWordMonitor(wordmonitor.Name, wordmonitor.CreatePlayerName, req.GetName(),
		wordmonitoroption.WithRawData(&WordCheckCreatePlayerName{
			UserId:     st.UserId,
			InviteCode: inviteCode,
		}),
		wordmonitoroption.WithDitchId(ditchId),
		wordmonitoroption.WithCommonData(&wordmonitor2.CommonData{
			PlatformUniquePlayerId: st.AccountName,
			ActorIP:                st.RemoteAddr,
			SrvId:                  engine.GetServerId(),
			ActorName:              req.GetName(),
		}),
	)
}

func (st *UserSt) onNameMonitorRet(ret wordmonitor2.Ret, data *WordCheckCreatePlayerName) {
	if ret != wordmonitor2.Success {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.NameContainFilterChar})
		st.Status = sNone
		return
	}

	req := st.CreatePlayerObj
	if nil == req {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.DbCreateActorFailed})
		st.Status = sNone
		return
	}

	series, errno := series2.GetActorIdSeries(st.ServerId)
	if 0 != errno {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: errno})
		st.Status = sNone
		return
	}

	if !engine.CheckNameRepeat(req.GetName()) {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.NameIsExist})
		st.Status = sNone
		return
	}

	if !engine.CheckNameSpecialCharacter(req.GetName()) {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: custom_id.NameContainFilterChar})
		st.Status = sNone
		return
	}

	if engine.GetForbidCreatePlayer() && data.InviteCode != "" {
		if code := invitecodemgr.UseCode(data.InviteCode); 0 != code {
			st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: code})
			logger.LogWarn("邀请码错误 accountName:%s 邀请码:%s code:%d", st.AccountName, data.InviteCode, code)
			return
		}
	}

	engine.AddPendingName(req.GetName())

	st.Status = sCreatePlayer
	createPlayerSt := mysql.CreatePlayer{
		UserId:      st.UserId,
		AccountName: st.AccountName,
		Series:      series,
		JobSex:      req.GetJob(),
		DitchId:     req.GetDitchId(),
		SubDitch:    req.GetSubDitchId(),
		Name:        req.GetName(),
		ServerId:    st.ServerId,
	}
	gshare.SendDBMsg(custom_id.GMsgCreatePlayer, createPlayerSt, st.RemoteAddr)
}

func (st *UserSt) onCreatePlayerRet(data mysql.CreatePlayerRet) {
	st.Status = sNone
	engine.DelPendingName(data.Name)
	if data.Errno > 0 {
		st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: uint32(data.Errno)})
		return
	}

	engine.AddPlayerName(data.Name)
	st.SendProto3(1, 3, &pb3.S2C_1_3{ErrorCode: uint32(0), RoleId: data.ActorId})
}

func (st *UserSt) c2sCreatePlayer(buff []byte) {
	if st.UserId <= 0 {
		return
	}

	if st.ActorId > 0 {
		return //已完成创角流程
	}

	var req pb3.C2S_1_3
	if nil != pb3.Unmarshal(buff, &req) {
		return
	}

	job := req.GetJob()
	if !checkJobInvalid(job) {
		return
	}

	// 正在创角中
	if st.Status == sCreatePlayerCheck || st.Status == sCreatePlayer {
		return
	}

	st.Status = sCreatePlayerCheck
	st.CreatePlayerObj = &req
	gshare.SendDBMsg(custom_id.GMsgLoadPlayerCount, st.UserId, st.ServerId)
}

// 客户端重连
func (st *UserSt) c2sReconnect(buff []byte) {
	var req pb3.C2S_1_10
	if nil != pb3.Unmarshal(buff, &req) {
		return
	}
	if req.ActorId == 0 {
		return
	}
	if st.ActorId > 0 && st.ActorId != req.ActorId {
		return
	}

	rsp := pb3.S2C_1_10{}

	st.ActorId = req.ActorId

	success := false
	actor := manager.GetPlayerPtrById(st.ActorId)
	if nil != actor {
		logger.LogInfo("【登录流程】角色重连, key=%s", req.Key)
		success = actor.Reconnect(req.Key, st.GateId, st.ConnId, st.RemoteAddr)
		if success {
			st.UserId = actor.GetUserId()
			st.AccountName = actor.GetAccountName()
			logger.LogInfo("player reconnect succeed, actorId:%d, userId:%d, account:%s",
				st.ActorId, st.UserId, st.AccountName)
			rsp.Result = 0
		} else {
			rsp.Result = 2
		}
	} else {
		rsp.Result = 1
	}

	st.SendProto3(1, 10, &rsp)
	if !success {
		SendGateCloseUser(st.GateId, st.ConnId)
	}
}

func checkJobInvalid(in uint32) bool {
	sex := in & base.SexMask
	job := in >> base.SexBit
	if sex != gshare.Male && sex != gshare.Female { //性别有误
		logger.LogWarn("性别有误:%d", job)
		return false
	}

	if job < custom_id.JobIdMin || job > custom_id.JobIdMax { //职业
		logger.LogWarn("职业有误:%d", job)
		return false
	}
	return true
}

func adjustName(req *pb3.C2S_1_3) uint32 {
	//name := utils.RemoveSpace(&req.Name)
	name := req.Name
	if len(name) == 0 {
		return custom_id.NameIsEmpty //角色名不能为空
	}
	req.Name = name
	//名字最大不能超过7个字符
	nameLen := utf8.RuneCountInString(req.GetName())
	nameLenLimit := jsondata.GetNameLenLimit()
	if nameLen > int(nameLenLimit) {
		return custom_id.NameLenLimit
	}

	if !engine.CheckNameRepeat(req.GetName()) {
		return custom_id.NameIsExist
	}
	if !engine.CheckNameSpecialCharacter(req.GetName()) {
		return custom_id.NameContainFilterChar
	}
	return 0
}

func onVerifyRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onVerifyRet", 1, len(args)) {
		return
	}
	ret, ok := args[0].(mysql.VerifyRet)
	if !ok {
		return
	}
	gateId, connId := ret.GateId, ret.ConnId
	gate := GetGateConn(gateId)
	if nil == gate {
		return
	}
	user := gate.GetUser(connId)
	if nil == user {
		return
	}
	user.onVerifyRet(ret)
}

func onLoadPlayerListRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadPlayerListRet", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.LoadPlayerList)
	if !ok {
		return
	}
	if user := GetGateUserByUserId(data.UserId); nil != user {
		user.onLoadPlayerList(data)
	}
}

func onDeletePlayerRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onDeletePlayerRet", 2, len(args)) {
		return
	}
	playerId, ok1 := args[0].(uint64)
	userId, ok2 := args[1].(uint32)
	if !ok1 || !ok2 {
		return
	}
	if user := GetGateUserByUserId(userId); nil != user {
		user.onDeletePlayer(playerId)
	}
}

func onLoadPlayerCountRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onLoadPlayerCountRet", 3, len(args)) {
		return
	}
	userId, ok := args[0].(uint32)
	if !ok {
		return
	}

	success, ok1 := args[1].(bool)
	count, ok2 := args[2].(uint32)

	if !ok1 || !ok2 {
		return
	}

	if user := GetGateUserByUserId(userId); nil != user {
		user.onCreatePlayerCheck(success, count)
	}
}

func onCreatePlayerRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onCreatePlayerRet", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.CreatePlayerRet)
	if !ok {
		return
	}
	if user := GetGateUserByUserId(data.UserId); nil != user {
		user.onCreatePlayerRet(data)
	}
}

func onClientEnterGameRet(args ...interface{}) {
	if !gcommon.CheckArgsCount("onClientEnterGameRet", 1, len(args)) {
		return
	}
	data, ok := args[0].(mysql.ClientEnterGameRet)
	if !ok {
		return
	}
	if user := GetGateUserByUserId(data.UserId); nil != user {
		user.onClientEnterGameRet(data)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgVerifyAccountRet, onVerifyRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadPlayerListRet, onLoadPlayerListRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgDeletePlayerRet, onDeletePlayerRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgLoadPlayerCountRet, onLoadPlayerCountRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgCreatePlayerRet, onCreatePlayerRet)
		gshare.RegisterGameMsgHandler(custom_id.GMsgClientEnterGameRet, onClientEnterGameRet)
		engine.RegWordMonitorOpCodeHandler(wordmonitor.CreatePlayerName, func(word *wordmonitor.Word) error {
			data, ok := word.Data.(*WordCheckCreatePlayerName)
			if !ok {
				return errors.New("data not userId")
			}
			if user := GetGateUserByUserId(data.UserId); nil != user {
				user.onNameMonitorRet(word.Ret, data)
			}
			return nil
		})
	})
}
