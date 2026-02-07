package entity

import (
	"errors"
	"fmt"
	"github.com/gzjjyz/srvlib/utils/pie"
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/cmd"
	"jjyz/base/common"
	"jjyz/base/compress"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/disableproto"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gateworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/mq"
	"jjyz/gameserver/net"
	"jjyz/gameserver/redisworker/redismid"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/alg/bitset"
	"github.com/gzjjyz/srvlib/utils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	FightTimeOut      = 10  // 战斗服心跳
	ReconnectWaitTime = 180 // 重连等待时间 3分钟
)

var (
	_ iface.IPlayer = (*Player)(nil)

	fightTypeError = errors.New("EnterFightSrv fight type error")
)

type Player struct {
	reconnectKey string // 重连key
	StateBitSet  *bitset.BitSet

	PlayerData *pb3.PlayerData

	MainData   *pb3.PlayerMainData
	BinaryData *pb3.PlayerBinaryData

	AttrSys  *actorsystem.AttrSys
	SysMgr   *actorsystem.Mgr
	Consumer *engine.Consumer

	FightHandle uint64 // 战斗服handle
	FightHeart  uint32 // 心跳

	_t1s      *time_util.TimeChecker // 1秒定时器
	_t5m      *time_util.TimeChecker // 5分钟定时器
	timerHeap *time_util.TimerHeap   // 玩家身上的计时器

	UserId      uint32 //账号id
	ActorId     uint64 //角色id
	GateId      uint32
	ConnId      uint32
	GmLevel     uint32
	RemoteAddr  string //连接ip
	AccountName string
	teamId      uint64

	proxy iface.IActorProxy

	SaveTimestamp   uint32
	EnterGameFinish bool
	ReloadCacheKick bool
	DestroyEntity   bool
	LoginTrace      bool

	FbId      uint32
	SceneId   uint32
	sceneName string

	lockFlag   uint32 // 操作锁
	lockTimers map[uint32]*time_util.Timer

	firstEnterFight bool // 第一次进入战斗服

	isLost   bool   // 掉线了
	lostTime uint32 // 掉线时间戳

	lastFightTime uint32 // 上一次战斗时间

	statics     map[string]interface{}
	saveStatics bool

	currentSceneId uint32
	currentLocalX  int32
	currentLocalY  int32

	gmAttr map[uint32]int64
}

func (player *Player) GetLogPrefix() string {
	return fmt.Sprintf("[playerId: %d]", player.ActorId)
}

func (player *Player) GetLogCallStackSkip() int {
	return 1
}

func (player *Player) GetLost() bool {
	return player.isLost
}

func (player *Player) SetLost(status bool) {
	if player.isLost == status {
		return
	}
	player.isLost = status
	if player.isLost {
		nowSec := time_util.NowSec()
		player.lostTime = nowSec

		player.TriggerEvent(custom_id.AeLost)
		player.Save(false)

		player.SetTimeout(time.Second*ReconnectWaitTime, func() {
			if player.isLost && nowSec == player.lostTime {
				player.ClosePlayer(cmd.DCRLost)
			}
		})

		player.SetExtraAttr(attrdef.IsLost, 1)
		player.CallActorFunc(actorfuncid.Lost, nil)
	} else {
		player.SetExtraAttr(attrdef.IsLost, 0)
	}
}

func (player *Player) IsOperateLock(bit uint32) bool {
	return utils.IsSetBit(player.lockFlag, bit)
}

func (player *Player) LockOperate(bit uint32, timeout uint32) {
	utils.SetBit(player.lockFlag, bit)
	// 超时处理
	player.lockTimers[bit] = player.SetTimeout(time.Duration(timeout)*time.Millisecond, func() {
		if nil == player || nil == player.lockTimers[bit] {
			return
		}
		player.UnlockOperate(bit)
	})
}

func (player *Player) UnlockOperate(bit uint32) {
	utils.ClearBit(player.lockFlag, bit)

	if nil != player.lockTimers[bit] {
		player.lockTimers[bit].Stop()
		player.lockTimers[bit] = nil
	}
}

func (player *Player) GetActorProxy() iface.IActorProxy {
	return player.proxy
}

func onUnlockOperate(player iface.IPlayer, buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}
	player.UnlockOperate(msg.U32Param)
}

func NewPlayer(initData *pb3.PlayerData) *Player {
	player := new(Player)
	player.statics = make(map[string]interface{})
	player.Consumer = engine.NewConsumer(player)
	player.StateBitSet = bitset.New(custom_id.ESEnd)

	player.AttrSys = actorsystem.NewAttrSys(player)
	player.PlayerData = initData

	player.MainData = initData.MainData
	player.BinaryData = initData.BinaryData
	if nil == player.BinaryData {
		player.BinaryData = &pb3.PlayerBinaryData{}
	}

	player.SysMgr = actorsystem.NewSystemMgr(player)

	player._t1s = time_util.NewTimeChecker(time.Second)
	player._t5m = time_util.NewTimeChecker(5 * time.Minute)
	player.timerHeap = time_util.NewTimerHeap()

	player.lockTimers = make(map[uint32]*time_util.Timer)

	mainData := initData.MainData
	if nil != mainData {
		player.SetExtraAttr(attrdef.Job, attrdef.AttrValueAlias(mainData.Job<<base.SexBit|mainData.Sex))
		player.SetExtraAttr(attrdef.SrvId, attrdef.AttrValueAlias(engine.GetServerId()))
		player.SetExtraAttr(attrdef.PfId, attrdef.AttrValueAlias(engine.GetPfId()))
		player.SetExtraAttr(attrdef.DitchId, attrdef.AttrValueAlias(mainData.DitchId))
		player.SetExtraAttr(attrdef.SubDitchId, attrdef.AttrValueAlias(mainData.SubDitchId))
	}

	return player
}

func (player *Player) GetMainData() *pb3.PlayerMainData {
	return player.MainData
}

func (player *Player) GetConsumer() iface.IConsumer {
	return player.Consumer
}

func (player *Player) GetAttrSys() iface.IAttrSys {
	return player.AttrSys
}

func (player *Player) SetFightAttr(t attrdef.AttrTypeAlias, v attrdef.AttrValueAlias) {
	if sys := player.AttrSys; nil != sys {
		sys.SetFightAttr(t, v)
	}
}

func (player *Player) SetExtraAttr(t attrdef.AttrTypeAlias, v attrdef.AttrValueAlias) {
	if sys := player.AttrSys; nil != sys {
		sys.SetExtraAttr(t, v)
	}
}

func (player *Player) GetExtraAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias {
	if sys := player.AttrSys; nil != sys {
		return sys.GetExtraAttr(attrType)
	}
	return 0
}

func (player *Player) GetExtraAttrU32(attrType attrdef.AttrTypeAlias) uint32 {
	if sys := player.AttrSys; nil != sys {
		return uint32(sys.GetExtraAttr(attrType))
	}
	return 0
}

func (player *Player) GetFightAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias {
	if sys := player.AttrSys; nil != sys {
		return sys.GetFightAttr(attrType)
	}
	return 0
}

func (player *Player) GetFbId() uint32 {
	return player.FbId
}

func (player *Player) GetSceneId() uint32 {
	return player.SceneId
}

func (player *Player) InitBootData(actorId uint64, gateId, connId, userId uint32, gmLevel uint32, remoteAddr string) {
	player.ActorId = actorId
	player.GateId = gateId
	player.ConnId = connId
	player.GmLevel = gmLevel
	player.UserId = userId
	player.RemoteAddr = remoteAddr
}

func (player *Player) InitCreateData() bool {
	if miscData := player.BinaryData; nil != miscData && nil != miscData.Pos {
		player.SetExtraAttr(attrdef.PosX, attrdef.AttrValueAlias(miscData.Pos.PosX))
		player.SetExtraAttr(attrdef.PosY, attrdef.AttrValueAlias(miscData.Pos.PosY))
	}

	player.initSettings()

	player.SysMgr.OnInit()

	player.OnFinishInit()

	return true
}

func (player *Player) initSettings() {
	if player.GetBinaryData().Settings == nil {
		player.GetBinaryData().Settings = &pb3.Settings{}
	}
}

// 初始化完成
func (player *Player) OnFinishInit() {
	appears := player.GetMainData().AppearInfo.Appear
	if nil == appears {
		player.GetMainData().AppearInfo.Appear = make(map[uint32]*pb3.SysAppearSt)
	}
	for pos, sysappear := range appears {
		if !player.checkTakeOnAppear(sysappear) {
			continue
		}

		appearId := int64(int64(sysappear.SysId)<<32 | int64(sysappear.AppearId))

		extraAttrId := appeardef.AppearPosMapToExtraAttr[pos]
		if extraAttrId == 0 {
			continue
		}
		player.SetExtraAttr(extraAttrId, appearId)
	}
}

func (player *Player) GetRemoteAddr() string {
	return player.RemoteAddr
}

func (player *Player) GetAccountName() string {
	return player.AccountName
}

func (player *Player) GetUserId() uint32 {
	return player.UserId
}

func (player *Player) GetGateInfo() (uint32, uint32) {
	return player.GateId, player.ConnId
}

func (player *Player) Destroy() {
	player.SysMgr.Destroy()
	player.AttrSys.Destroy()
	player.timerHeap = nil
}

func (player *Player) SetTimeout(duration time.Duration, cb func()) *time_util.Timer {
	return player.timerHeap.SetTimeout(duration, cb)
}

func (player *Player) SetInterval(duration time.Duration, cb func(), ops ...time_util.TimerOption) *time_util.Timer {
	return player.timerHeap.SetInterval(duration, cb, ops...)
}

func (player *Player) RunOne() {
	if sys := player.AttrSys; nil != sys {
		sys.LogicRun()
	}

	if player._t1s.CheckAndSet(true) { //1秒定时器
		player.CheckFightTimeout()
		if player.saveStatics {
			player.saveStatics = false
			gshare.SendPlayerStaticsMsg(player.ActorId, player.statics)
			player.statics = make(map[string]interface{})
		}
		if player.GetSysMgr() != nil {
			player.GetSysMgr().OneSecLoop()
		}
	}

	if player._t5m.CheckAndSet(true) { //5分钟定时器
		player.Save(false)
	}

	if player.timerHeap != nil {
		player.timerHeap.RunOne()
	}

	now := time_util.NowSec()
	if 0 < player.SaveTimestamp && player.SaveTimestamp <= now {
		if !player.ReloadCacheKick {
			player.Save(true)
		}
		if player.DestroyEntity {
			player.Destroy()
			manager.OnPlayerClosed(player)
		}
		player.SaveTimestamp = 0
	}
}

func (player *Player) GetId() uint64 {
	return player.ActorId
}

func (player *Player) GetName() string {
	return player.PlayerData.MainData.ActorName
}

func (player *Player) SetName(newName string) {
	player.PlayerData.MainData.ActorName = newName
}

func (player *Player) CallActorFunc(fnId uint16, msg pb3.Message) error {
	proxy := player.GetActorProxy()
	if nil == proxy {
		return fmt.Errorf("CallActorFunc %d proxy is nil", fnId)
	}

	if actorfuncid.OnlyLocalFightFuncId(fnId) && player.proxy.GetProxyType() != base.LocalFightServer {
		player.SendTipMsg(tipmsgid.TpShouldReturnLocal)
		return fmt.Errorf("CallActorFunc %d should return local", fnId)
	}

	if msg != nil {
		player.LogTrace("[RPC.CallActorFunc] req:%s{%+v}", msg.ProtoReflect().Descriptor().FullName(), msg)
	} else {
		player.LogTrace("[RPC.CallActorFunc] fnId:%d", fnId)
	}
	return proxy.CallActorFunc(fnId, msg)
}

// CallActorSysFn 玩家不需要在战斗服也能调用
func (player *Player) CallActorSysFn(typ base.ServerType, fnId uint16, msg pb3.Message) error {
	proxy := player.GetActorProxy()
	if nil == proxy {
		return fmt.Errorf("CallActorSmallCrossFunc %d proxy is nil", fnId)
	}

	if !engine.FightClientExistPredicate(typ) {
		return fmt.Errorf("not conn by small cross server")
	}

	if msg != nil {
		player.LogTrace("[RPC.CallActorFuncBySrvType] req:%s{%+v}", msg.ProtoReflect().Descriptor().FullName(), msg)
	} else {
		player.LogTrace("[RPC.CallActorFuncBySrvType] fnId:%d", fnId)
	}

	return proxy.CallActorSysFn(typ, fnId, msg)
}

func (player *Player) CallActorSmallCrossFunc(fnId uint16, msg pb3.Message) error {
	return player.CallActorSysFn(base.SmallCrossServer, fnId, msg)
}

func (player *Player) CallActorMediumCrossFunc(fnId uint16, msg pb3.Message) error {
	return player.CallActorSysFn(base.MediumCrossServer, fnId, msg)
}

func (player *Player) CheckNewDay(zeroReset bool) bool {
	data := player.GetPlayerData()
	if nil == data {
		return false
	}

	nowSec := time_util.NowSec()

	days := data.MainData.LoginedDays
	if 0 == days { //第一次登录
		data.MainData.LoginedDays = 1
		data.MainData.NewDayResetTime = nowSec //设置天重置时间
		return true
	}

	reset := data.MainData.NewDayResetTime
	if !zeroReset && reset > 0 && time_util.IsSameDay(nowSec, reset) {
		return false //当天
	}
	if zeroReset {
		todayZero := time_util.GetDaysZeroTime(0)
		starDayTime := utils.MaxUInt32(data.MainData.LoginTime, time_util.GetBeforeDaysZeroTime(1))
		if starDayTime > 0 && todayZero > starDayTime {
			diff := todayZero - starDayTime
			data.MainData.DayOnlineTime += diff
		}
	}
	player.TriggerEvent(custom_id.AeBeforeNewDay)

	data.MainData.DayOnlineTime = 0
	data.MainData.LoginedDays = days + 1

	data.MainData.NewDayResetTime = nowSec //设置天重置时间

	if nil != data.BinaryData.ChargeInfo {
		data.BinaryData.ChargeInfo.DailyChargeDiamond = 0
		data.BinaryData.ChargeInfo.DailyChargeMoney = 0
		data.BinaryData.RealChargeInfo.DailyChargeDiamond = 0
		data.BinaryData.RealChargeInfo.DailyChargeMoney = 0
	}

	player.NewDayArrive()

	if !time_util.IsSameWeek(nowSec, reset) {
		player.TriggerEvent(custom_id.AeBeforeNewWeek)
		player.NewWeekArrive()
	}

	player.TriggerQuestEvent(custom_id.QttLoginDays, uint32(0), 1)
	player.TriggerQuestEvent(custom_id.QttFirstOrNextDayLogin, uint32(0), 1)
	player.TriggerQuestEventRange(custom_id.QttFirstOrBuyXSponsorGift)

	if zeroReset {
		logworker.LogLogin(player, &pb3.LogLogin{
			Type:      common.LogLogout,
			Timestamp: nowSec - 1,
		})
		player.logLogin()
	}
	return true
}

func (player *Player) NewHourArrive() {
	player.TriggerEvent(custom_id.AeNewHour)
}

func (player *Player) NewDayArrive() {
	player.SendServerInfo() // 先通知客户端最新的服务器信息
	player.TriggerEvent(custom_id.AeNewDay)
	player.LogDebug("%s NewDayArrive", player.GetName())
	player.SendActorData()
	player.SendLoginOrNewDayBaseInfo()
	player.TriggerQuestEventRange(custom_id.QttOpenSrvDay)
	player.TriggerQuestEventRange(custom_id.QttLoginAfterSeverOpenDay)
	player.TriggerQuestEvent(custom_id.QttTotalLoginDays, 0, 1)
	player.RefreshDropTimes()

	// 通知客户端跨天 放到最后
	player.SendProto3(2, 7, &pb3.S2C_2_7{})
}

func (player *Player) NewWeekArrive() {
	player.TriggerEvent(custom_id.AeNewWeek)
	player.RefreshWeeklyDropTimes()
	player.LogDebug("%s NewWeekArrive", player.GetName())
}

func (player *Player) ReturnToStaticScene() {

}

func (player *Player) LeaveGame() {
}

func (player *Player) SendReConnectKey() {
	player.reconnectKey = random.GenerateKey(10)
	player.SendProto3(1, 11, &pb3.S2C_1_11{Key: player.reconnectKey})
}

func (player *Player) Reconnect(key string, gateId, connId uint32, remoteAddr string) bool {
	if key != player.reconnectKey {
		return false
	}

	player.ReLogin(true, gateId, connId, remoteAddr)

	return true
}

func (player *Player) ReLogin(reconnect bool, gateId, connId uint32, remoteAddr string) {
	player.GateId = gateId
	player.ConnId = connId
	player.RemoteAddr = remoteAddr

	player.SetLost(false)

	player.SendReConnectKey()

	player.sendSettings()
	player.SendServerInfo()
	player.SendActorData()
	player.SendLoginOrNewDayBaseInfo()

	player.TriggerEvent(custom_id.AeReconnect)

	err := player.CallActorFunc(actorfuncid.ReLogin, nil)
	if err != nil {
		player.EnterLastFb()
		player.LogError("ReLogin actorfuncid.ReLogin err:%v", err)
		return
	}

	player.logReLogin()
}

func (player *Player) EnterGame() {
	player.LogInfo("【登录流程】EnterGame()")
	//先初始化，初始化完成后再响应前段请求
	player.EnterGameFinish = false
	player.LoginTrace = false
	player.firstEnterFight = true

	binary := player.GetBinaryData()
	isInit := false

	//第一次进入游戏
	if player.GetLevel() == 0 {
		isInit = true
		player.TriggerEvent(custom_id.AeFirstEnterGame)
	}

	// pk模式是0的时候设置为和平模式
	if binary.PkMode == 0 {
		binary.PkMode = custom_id.FpPeaceful
	}

	player.SetExtraAttr(attrdef.Hp, binary.Hp)
	player.SetExtraAttr(attrdef.ZhenYuan, binary.Zhenyuan)
	player.SetExtraAttr(attrdef.PkMode, attrdef.AttrValueAlias(binary.PkMode))
	if binary.BattleSoulData != nil {
		player.SetExtraAttr(attrdef.BattleSoulFurry, binary.BattleSoulData.Furry)
		player.SetExtraAttr(attrdef.BattleSoulReadyPos, int64(binary.BattleSoulData.ReadyPos))
	}

	if binary.NewJingJieState != nil {
		player.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(binary.NewJingJieState.Level))
	}

	if binary.FlyCampData != nil {
		flyCamp := binary.FlyCampData
		if flyCamp.Camp > 0 && flyCamp.IsFinish {
			player.SetExtraAttr(attrdef.FlyCamp, attrdef.AttrValueAlias(flyCamp.Camp))
		}
	}

	player.SendServerInfo()
	player.SendReConnectKey()

	if role := player.GetPlayerData(); nil != role {
		role.GetMainData().LoginTime = time_util.NowSec()
	}

	isNewDay := player.CheckNewDay(false)

	player.NewHourArrive()

	player.sendSettings()

	// 登录算属性在这里, 算完属性后要同步给战斗服
	player.TriggerEvent(custom_id.AeLogin)

	//第一次进入游戏
	if isInit {
		if sys := player.GetLevelSys(); nil != sys {
			sys.SetLevel(1, pb3.LogId_LogInitLevel)
		}
	}

	player.RunOne()
	player.SendActorData()

	player.LogInfo("【登录流程】EnterLastFb()")
	player.EnterLastFb()
	player.TriggerEvent(custom_id.AeAfterLogin)

	var crossCamp uint32
	hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)
	crossCamp = uint32(hostInfo.Camp)

	player.SetExtraAttr(attrdef.SmallCrossCamp, attrdef.AttrValueAlias(crossCamp))
	player.SetExtraAttr(attrdef.MediumCrossCamp, attrdef.AttrValueAlias(engine.GetMediumCrossCamp()))

	player.EnterGameFinish = true

	player.TriggerQuestEvent(custom_id.QttOnLogin, 0, 1)

	if isNewDay {
		if !isInit {
			player.TriggerQuestEvent(custom_id.QttFirstOrNextDayLogin, uint32(0), 1)
			player.TriggerQuestEventRange(custom_id.QttFirstOrBuyXSponsorGift)
		}
	}

	player.SendLoginOrNewDayBaseInfo()
	player.pubBaseInfoToCross()
	player.logLogin()
}

func (player *Player) IsEnterGameFinish() bool {
	return player.EnterGameFinish
}

func (player *Player) SendActorData() {
	mainData := player.GetMainData()
	data := player.GetBinaryData()
	var circle uint32
	if data != nil && data.NewJingJieState != nil {
		circle = data.NewJingJieState.Level
	}
	rsp := &pb3.S2C_2_0{
		Name:                 player.GetName(),
		TotalLoginDay:        mainData.LoginedDays,
		TodayTotalOnlineTime: mainData.DayOnlineTime,
		CreateTime:           player.MainData.CreateTime,
		DayOnlineTime:        player.GetDayOnlineTime(),
		ActorId:              player.ActorId,
		Circle:               circle,
		Level:                mainData.Level,
		HiddenSuperVip:       data.HiddenSuperVip,
	}

	if globalVar := gshare.GetStaticVar(); nil != globalVar {
		if diamond, ok := globalVar.DailyUseDiamond[player.GetId()]; ok {
			rsp.TodayUseDiamond = diamond
		}
	}

	player.SendProto3(2, 0, rsp)
}

func (player *Player) GetDayOnlineTime() uint32 {
	mainData := player.GetMainData()
	loginTime := mainData.LoginTime
	now := time_util.NowSec()
	if time_util.IsSameDay(now, loginTime) {
		diff := now - loginTime
		return mainData.DayOnlineTime + diff
	} else {
		zeroTime := time_util.GetZeroTime(now)
		diff := now - zeroTime
		return mainData.DayOnlineTime + diff
	}
}

func (player *Player) ClosePlayer(reason uint16) {
	if player.SaveTimestamp > 0 {
		return
	}

	player.SaveTimestamp = time_util.NowSec() + 3

	actorId := player.GetId()

	var loginTime uint32
	now := time_util.NowSec()
	if data := player.GetPlayerData(); nil != data {

		loginTime = data.MainData.LoginTime
		data.MainData.LastLogoutTime = now

		if time_util.IsSameDay(now, loginTime) {
			diff := now - loginTime
			data.MainData.DayOnlineTime = data.MainData.DayOnlineTime + diff
		} else {
			zeroTime := time_util.GetZeroTime(now)
			diff := now - zeroTime
			data.MainData.DayOnlineTime = data.MainData.DayOnlineTime + diff
		}
	}

	player.TriggerEvent(custom_id.AeLogout, actorId)

	player.SendCloseActor()

	player.SendProto3(1, 8, &pb3.S2C_1_8{Reason: uint32(reason)})

	player.LogDebug("[Name:%s] Logout, reason:%v", player.GetName(), reason)
	player.SetTimeout(1*time.Second, func() {
		// 这条如果跟1-8一起发过去, 到了网关那边没法确定时序, 延迟1秒
		gateworker.SendGateCloseUser(player.GateId, player.ConnId)
	})

	player.logLogout()
	gshare.SendRedisMsg(redismid.ReportPlayerLogout, &pb3.ReportPlayerLogoutSt{
		ActorId:    player.GetId(),
		UserId:     player.GetUserId(),
		LogoutAt:   now,
		LoginAt:    loginTime,
		PfId:       engine.GetPfId(),
		SrvId:      engine.GetServerId(),
		DitchId:    player.GetExtraAttrU32(attrdef.DitchId),
		SubDitchId: player.GetExtraAttrU32(attrdef.SubDitchId),
	})
}

func (player *Player) EnterLastFb() {
	binary := player.GetBinaryData()

	var (
		sceneId uint32
		x, y    int32
	)
	if pos := binary.Pos; nil != pos {
		sceneId, x, y = pos.GetSceneId(), pos.GetPosX(), pos.GetPosY()
	} else {
		sceneId = jsondata.GlobalUint("bornMapId")
		if conf := jsondata.GetSceneConf(sceneId); nil != conf {
			x, y = -1, -1
		}
	}

	req := &pb3.EnterFubenHdl{}
	req.FbHdl = 0
	req.SceneId = sceneId
	req.PosX = x
	req.PosY = y

	if conf := jsondata.GetPkConf(); nil != conf {
		if player.GetExtraAttrU32(attrdef.PKValue) >= conf.PrisonPKValue {
			req.SceneId = conf.PrisonSceneId
			req.PosX, req.PosY = -1, -1
		}
	}

	err := player.DoEnterFightSrv(base.LocalFightServer, fubendef.EnterFbHdl, req)
	if err != nil {
		player.LogError("EnterLastFb failed %s", err)
		return
	}

	now := time_util.NowSec()
	player.FightHeart = now + FightTimeOut // 10秒超时时间
}

func (player *Player) Save(logout bool) {
	player.SysMgr.Save()
	if logout {
		player.LeaveGame()
	}

	player.PlayerData.MainData.FightValue = player.GetExtraAttr(attrdef.FightValue)

	if data, err := pb3.Marshal(player.PlayerData); nil == err {
		gshare.SendDBMsg(custom_id.GMsgSaveActorDataToCache, player.RemoteAddr, data)
	} else {
		player.LogError("Player:Save error! %v", err)
	}

	player.pubBaseInfoToCross()
}

func (player *Player) SaveToObjVersion(version uint32) {
	if version == 0 {
		version = time_util.NowSec()
	}
	if data, err := pb3.Marshal(player.PlayerData); nil == err {
		gshare.SendDBMsg(custom_id.GMsgActorSaveToObjVersion, version, data)
	} else {
		player.LogError("Player:SaveToOBS error! %v", err)
	}
}

func (player *Player) LogTrace(format string, args ...interface{}) {
	logger.LogTraceWithRequester(player, format, args...)
}

func (player *Player) GetCallInfo() *logger.CallInfoSt {
	return nil
}

func (player *Player) LogDebug(format string, args ...interface{}) {
	logger.LogDebugWithRequester(player, format, args...)
}

func (player *Player) LogStack(format string, args ...interface{}) {
	logger.LogStackWithRequester(player, format, args...)
}

func (player *Player) LogInfo(format string, args ...interface{}) {
	logger.LogInfoWithRequester(player, format, args...)
}

func (player *Player) LogWarn(format string, args ...interface{}) {
	logger.LogWarnWithRequester(player, format, args...)
}

func (player *Player) LogError(format string, args ...interface{}) {
	logger.LogErrorWithRequester(player, format, args...)
}

func (player *Player) LogFatal(format string, args ...interface{}) {
	logger.LogFatalWithRequester(player, format, args...)
}

func (player *Player) LogWithCustomCallInfo(callInfo *logger.CallInfoSt, format string, args ...interface{}) {
	logger.LogErrorWithRequesterAndCustomCallInfo(player, callInfo, format, args...)
}

func SendDirectProtoToFight(serverType base.ServerType, actorId uint64, msg *base.Message) {
	if nil == msg {
		return
	}
	st := &pb3.ForwardClientMsgToFight{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: actorId,
	}
	st.Header = msg.Header
	st.Data = msg.Data

	err := engine.SendToFight(serverType, cmd.GFToFightClientMsg, st)
	if err != nil {
		logger.LogError("err:%v", err)
	}
}

type YYMessage interface {
	GetActiveId() uint32
}

func (player *Player) DoNetMsg(protoIdH, protoIdL uint16, message *base.Message) {
	// 屏蔽协议号
	protoId := protoIdH<<8 | protoIdL
	if disableproto.IsDisableProto(uint32(protoId)) {
		return
	}

	//direct
	if serverType, exist := net.DirectTbl[protoId]; exist {
		SendDirectProtoToFight(serverType, player.GetId(), message)
		return
	}

	if _, exist := net.FightProtoList[protoId]; exist {
		// 协议转发到场景服务器
		// 玩家id + 协议
		if nil != player.proxy {
			player.proxy.OnRawProto(message)
		}
		return
	}

	p := net.GetRegister(protoIdH, protoIdL)
	if p == nil {
		player.LogError("proto not register: (%d, %d)", protoIdH, protoIdL)
		return
	}

	var doProtoTypeUnknown = func() {
		err := p.Func(player, message)
		if err != nil {
			player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
			protoErr, ok := err.(*neterror.NetError)
			if !ok {
				return
			}
			player.LogWithCustomCallInfo(protoErr.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
		}
	}

	// 个人系统协议
	var doProtoTypePSys = func() {
		obj := player.GetSysObj(uint32(p.SysId))
		if obj == nil {
			err := neterror.InternalError("no sysObj %d on player", p.SysId)
			player.SendProto3(0, 0, err.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			return
		}

		if nil != obj && !obj.IsOpen() { //系统未开启
			err := neterror.SysNotExistError("系统:%d 未开启", p.SysId)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, err.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			return
		}

		if p.Caller == nil && p.SysHdlFunc == nil {
			err := neterror.InternalError("(%d, %d) not register handle func", protoIdH, protoIdL)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, err.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			return
		}

		if p.SysHdlFunc != nil {
			err := (p.SysHdlFunc(obj))(message)
			if nil != err {
				player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
				protoErr, ok := err.(*neterror.NetError)
				if !ok {
					return
				}
				err := neterror.InternalError("%s", err)
				player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
				player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			}
			return
		}

		_, err := net.Call(p.Caller, obj, message)
		if nil != err {
			player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
			protoErr, ok := err.(*neterror.NetError)
			if !ok {
				return
			}
			player.LogWithCustomCallInfo(protoErr.GetCallInfo(), "%v", err)
			player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
		}
	}

	// 个人运营活动协议
	var doProtoTypePyy = func() {
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return
		}

		var msg pb3.YYBaseDriveTmpl
		if err := message.UnPackPb3Msg(&msg); err != nil {
			err := neterror.ParamsInvalidError("UnPackPb3Msg err:%s", err)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			return
		}

		obj := sys.GetObjById(msg.Base.ActiveId)
		if obj == nil {
			player.LogWarn("sys of activeId:%d not exist", msg.Base.ActiveId)
			return
		}

		if nil != obj && !obj.IsOpen() { //系统未开启
			err := neterror.SysNotExistError("系统:%d 未开启", p.SysId)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, err.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			return
		}

		if p.YYSysHdlFunc == nil && p.Caller == nil {
			player.LogError("(%d, %d) not register handle func", protoIdH, protoIdL)
			return
		}

		if p.YYSysHdlFunc != nil {
			err := (p.YYSysHdlFunc(obj))(message)
			if nil != err {
				player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
				protoErr, ok := err.(*neterror.NetError)
				if !ok {
					return
				}
				player.LogWithCustomCallInfo(protoErr.GetCallInfo(), "%s", err)
				player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			}
			return
		}

		_, err := net.Call(p.Caller, obj, message)
		if nil != err {
			player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
			protoErr, ok := err.(*neterror.NetError)
			if !ok {
				return
			}
			player.LogWithCustomCallInfo(protoErr.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))

		}
	}

	// 全服运营活动协议
	var doProtoTypeGlobalYy = func() {
		var msg pb3.YYBaseDriveTmpl
		if err := message.UnPackPb3Msg(&msg); err != nil {
			err := neterror.ParamsInvalidError("UnPackPb3Msg err:%s", err)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			return
		}

		obj := yymgr.GetYYByActId(msg.Base.ActiveId)
		if obj == nil {
			player.LogWarn("sys of activeId:%d not exist", msg.Base.ActiveId)
			return
		}

		if nil != obj && !obj.IsOpen() { //系统未开启
			err := neterror.SysNotExistError("活动:%d 未开启", msg.Base.ActiveId)
			player.LogWithCustomCallInfo(err.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, err.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
			return
		}

		if p.GlobalYYSysHdlFunc == nil {
			player.LogError("(%d, %d) not register handle func", protoIdH, protoIdL)
			return
		}

		err := (p.GlobalYYSysHdlFunc(obj))(player, message)
		if nil != err {
			player.LogError("(%d, %d) %s", protoIdH, protoIdL, err)
			protoErr, ok := err.(*neterror.NetError)
			if !ok {
				return
			}
			player.LogWithCustomCallInfo(protoErr.GetCallInfo(), "%s", err)
			player.SendProto3(0, 0, protoErr.ToProto(uint32(protoIdH), uint32(protoIdL), gshare.GameConf.SendErrorMsg))
		}
	}

	start := time.Now()
	switch p.Type {
	case net.ProtoType_Unknown:
		doProtoTypeUnknown()
	case net.ProtoType_PSys: //个人系统协议
		doProtoTypePSys()
	case net.ProtoType_Pyy: //个人运营活动协议
		doProtoTypePyy()
	case net.ProtoType_GlobalYy: //全服运营活动协议
		doProtoTypeGlobalYy()
	}
	// 协议处理超过两毫秒加个打印
	duration := time.Since(start)
	millisecond := duration.Milliseconds()
	if millisecond >= 2 {
		player.LogWarn("The processing time exceeds two milliseconds!! %v (%d, %d)", duration, protoIdH, protoIdL)
	}

}

func (player *Player) SendProto3(sysId, cmdId uint16, message pb3.Message) {
	if s := gateworker.GetGateConn(player.GateId); nil != s {
		if user := s.GetUser(player.ConnId); nil != user {
			user.SendProto3(sysId, cmdId, message)
		}
	}
}

func (player *Player) SendProtoBuffer(sysId, cmdId uint16, buffer []byte) {
	if s := gateworker.GetGateConn(player.GateId); nil != s {
		if user := s.GetUser(player.ConnId); nil != user {
			user.SendProtoBuffer(sysId, cmdId, buffer)
		}
	}
}

func (player *Player) GetPlayerData() *pb3.PlayerData {
	return player.PlayerData
}

func (player *Player) GetBinaryData() *pb3.PlayerBinaryData {
	return player.BinaryData
}

func (player *Player) GetSysMgr() iface.ISystemMgr {
	return player.SysMgr
}

func (player *Player) GetSysObj(sysId uint32) iface.ISystem {
	return player.SysMgr.GetSysObj(sysId)
}

func (player *Player) GetPYYObjList(yyDefine uint32) []iface.IPlayerYY {
	return pyymgr.GetPlayerAllYYObj(player, yyDefine)
}

func (player *Player) GetPYYObj(actId uint32) iface.IPlayerYY {
	return pyymgr.GetPlayerYYObj(player, actId)
}

func (player *Player) GetYYObj(actId uint32) iface.IYunYing {
	return yymgr.GetYYByActId(actId)
}

func (player *Player) SetSysStatus(sysId uint32, isOpen bool) {
	idxInt := sysId / 32
	idxByte := sysId % 32

	binary := player.GetBinaryData()
	if isOpen {
		binary.SysOpenStatus[idxInt] = utils.SetBit(binary.SysOpenStatus[idxInt], idxByte)
	} else {
		binary.SysOpenStatus[idxInt] = utils.ClearBit(binary.SysOpenStatus[idxInt], idxByte)
	}
}

func (player *Player) GetSysStatus(sysId uint32) bool {

	idxInt := sysId / 32
	idxByte := sysId % 32

	flag := player.GetBinaryData().SysOpenStatus[idxInt]

	return utils.IsSetBit(flag, idxByte)
}

func (player *Player) GetSysStatusData() map[uint32]uint32 {
	return player.GetBinaryData().SysOpenStatus
}

func (player *Player) GetSysOpen(sysId uint32) bool {
	if obj := player.GetSysObj(sysId); nil != obj && !obj.IsOpen() {
		return false
	}
	return true
}

func (player *Player) TriggerEvent(id int, args ...interface{}) {
	event.TriggerEvent(player, id, args...)
}

func (player *Player) TriggerQuestEvent(targetType, targetId uint32, count int64) {
	player.TriggerEvent(custom_id.AeQuestEvent, targetType, targetId, count)
}

func (player *Player) TriggerQuestEventRangeTimes(targetType, targetId uint32, count int64) {
	for i := count; i >= 1; i-- {
		player.TriggerEvent(custom_id.AeQuestEvent, targetType, targetId, int64(1))
	}
}

func (player *Player) TriggerQuestEventRange(targetType uint32, args ...interface{}) {
	var newArgs []interface{}
	newArgs = append(newArgs, targetType)
	newArgs = append(newArgs, args...)
	player.TriggerEvent(custom_id.AeQuestEventRange, newArgs...)
}

func (player *Player) TriggerQuestEventByRange(qtt, tVal, preVal, qtype uint32) {
	player.TriggerEvent(custom_id.AeQuestEventByRange, qtt, tVal, preVal, qtype)
}

func (player *Player) GetLevel() uint32 {
	level := player.GetExtraAttr(attrdef.Level)
	if level > 0 {
		return uint32(level)
	} else {
		return player.PlayerData.MainData.Level
	}
}

func (player *Player) GetVipLevel() uint32 {
	level := player.GetExtraAttr(attrdef.VipLevel)
	if level > 0 {
		return uint32(level)
	} else {
		return player.PlayerData.BinaryData.Vip
	}
}

func (player *Player) GetSpiritLv() uint32 {
	val := player.GetExtraAttr(attrdef.BossSpirits)
	if val > 0 {
		return custom_id.GetBossSpiritLv(uint64(val))
	} else {
		if player.BinaryData.BossSpiritData != nil {
			return player.BinaryData.BossSpiritData.Lv
		}
		return 0
	}
}

func (player *Player) GetExp() int64 {
	exp := player.GetExtraAttr(attrdef.Exp)
	if exp > 0 {
		return int64(exp)
	} else {
		return player.PlayerData.MainData.Exp
	}
}

func (player *Player) GetCreateTime() uint32 {
	return player.PlayerData.MainData.CreateTime
}

// 获取创角天数
func (player *Player) GetCreateDay() uint32 {
	createTime := player.PlayerData.MainData.CreateTime
	return time_util.TimestampSubDays(createTime, time_util.NowSec()) + 1
}

// 获取离线天数
func (player *Player) GetLogoutDay() uint32 {
	lastLogoutTime := player.PlayerData.MainData.LastLogoutTime
	loginTime := player.PlayerData.MainData.LoginTime
	if lastLogoutTime == 0 || lastLogoutTime > loginTime {
		return 0
	}
	return time_util.TimestampSubDays(lastLogoutTime, loginTime)
}

func (player *Player) GetLoginTime() uint32 {
	return player.PlayerData.MainData.LoginTime
}

func (player *Player) GetJob() uint32 {
	return player.PlayerData.MainData.Job
}
func (player *Player) SetJob(job uint32) {
	player.PlayerData.MainData.Job = job
}

func (player *Player) GetSex() uint32 {
	return player.PlayerData.MainData.Sex
}

func (player *Player) SetSex(sex uint32) {
	player.PlayerData.MainData.Sex = sex
}

func (player *Player) GetCircle() uint32 {
	return uint32(player.GetExtraAttr(attrdef.Circle))
}

func (player *Player) IsFlyUpToWorld() bool {
	rank := uint32(player.GetExtraAttr(attrdef.FlyUpWorldQuestRank))
	if rank > 0 {
		return rank > 0
	} else {
		if player.PlayerData.BinaryData.FlyUpRoadData == nil {
			return false
		}
		return player.PlayerData.BinaryData.FlyUpRoadData.Rank > 0
	}
}

func (player *Player) GetFlyCamp() uint32 {
	return player.GetExtraAttrU32(attrdef.FlyCamp)
}

func (player *Player) LearnSkill(id, level uint32, send bool) bool {
	if sys := player.GetSkillSys(); nil != sys {
		return sys.LearnSkill(id, level, send)
	}
	return false
}

func (player *Player) GetSkillLv(id uint32) uint32 {
	if sys := player.GetSkillSys(); nil != sys {
		return sys.GetSkillLevel(id)
	}
	return 0
}

func (player *Player) ForgetSkill(id uint32, recalcAttr, send, syncFight bool) {
	if sys := player.GetSkillSys(); nil != sys {
		sys.ForgetSkill(id, recalcAttr, send, syncFight)
	}
}

func (player *Player) GetSkillInfo(skillId uint32) *pb3.SkillInfo {
	if sys := player.GetSkillSys(); nil != sys {
		return sys.GetSkill(skillId)
	}
	return nil
}

func (player *Player) AddExp(exp int64, logId pb3.LogId, withWorldAddRate bool, addRate ...uint32) (finalExpAdded int64) {
	sys := player.GetLevelSys()
	if nil == sys {
		return 0
	}

	return sys.AddExp(exp, logId, withWorldAddRate, true, addRate...)
}

func (player *Player) AddExpV2(exp int64, logId pb3.LogId, addRateByTip uint32) (finalExpAdded int64) {
	sys := player.GetLevelSys()
	if nil == sys {
		return 0
	}

	return sys.AddExpV2(exp, logId, addRateByTip)
}

func (player *Player) GetMoneyCount(mt uint32) int64 {
	return player.GetMoneySys().GetMoneyCount(mt)
}

func (player *Player) AddMoney(mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	return player.GetMoneySys().AddMoney(mt, count, btip, logId)
}

func (player *Player) DeductMoney(mt uint32, count int64, params common.ConsumeParams) bool {
	return player.GetMoneySys().DeductMoney(mt, count, params)
}

func (player *Player) GetBagAvailableCount() uint32 {
	return player.GetBagSys().AvailableCount()
}

func (player *Player) GetFairyBagAvailableCount() uint32 {
	return player.GetFairyBagSys().AvailableCount()
}

func (player *Player) GetFairyEquipBagAvailableCount() uint32 {
	return player.GetFairyEquipBagSys().AvailableCount()
}

func (player *Player) GetBattleSoulGodEquipAvailableCount() uint32 {
	return player.GetBattleSoulGodEquipBagSys().AvailableCount()
}

func (player *Player) GetMementoBagAvailableCount() uint32 {
	return player.GetMementoBagSys().AvailableCount()
}

func (player *Player) GetFairySwordBagAvailableCount() uint32 {
	return player.GetFairySwordBagSys().AvailableCount()
}

func (player *Player) GetFairySpiritBagAvailableCount() uint32 {
	return player.GetFairySpiritEquBagSys().AvailableCount()
}

func (player *Player) GetHolyBagAvailableCount() uint32 {
	return player.GetHolyEquBagSys().AvailableCount()
}

func (player *Player) GetBloodBagAvailableCount() uint32 {
	return player.GetBloodBagSys().AvailableCount()
}

func (player *Player) GetBloodEquBagAvailableCount() uint32 {
	return player.GetBloodEquBagSys().AvailableCount()
}

func (player *Player) GetSmithBagAvailableCount() uint32 {
	return player.GetSmithBagSys().AvailableCount()
}

func (player *Player) GetFeatherEquBagAvailableCount() uint32 {
	return player.GetBloodEquBagSys().AvailableCount()
}

func (player *Player) GetBagAvailableCountByBagType(bagType uint32) uint32 {
	bagSys := player.GetBagSysByBagType(bagType)
	if nil == bagSys {
		return 0
	}
	return bagSys.AvailableCount()
}

func (player *Player) GetGodBeastBagAvailableCount() uint32 {
	return player.GetGodBeastBagSys().AvailableCount()
}

func (player *Player) GetFlyingSwordEquipBagAvailableCount() uint32 {
	return player.GetFlyingSwordBagSys().AvailableCount()
}

func (player *Player) GetSourceSoulBagAvailableCount() uint32 {
	return player.GetSourceSoulBagSys().AvailableCount()
}

func (player *Player) GetDitchTokens() uint32 {
	binary := player.GetBinaryData()
	if binary.DitchToken == nil {
		return 0
	}
	return binary.DitchToken.Tokens
}

func (player *Player) GetHistoryDitchTokens() uint32 {
	binary := player.GetBinaryData()
	if binary.DitchToken == nil {
		return 0
	}
	return binary.DitchToken.AccTokens
}

func (player *Player) AddDitchTokens(tokens uint32) {
	binary := player.GetBinaryData()
	if binary.DitchToken == nil {
		binary.DitchToken = &pb3.DitchToken{}
	}
	binary.DitchToken.Tokens += tokens
	binary.DitchToken.AccTokens += tokens
}

func (player *Player) SubDitchTokens(tokens uint32) {
	if player.GetDitchTokens() < tokens {
		return
	}
	binary := player.GetBinaryData()
	binary.DitchToken.Tokens -= tokens
}

func (player *Player) Split(handle uint64, count int64, logId pb3.LogId) uint64 {
	return player.GetBagSys().Split(handle, count, logId)
}

func (player *Player) SortBag() {
	player.GetBagSys().Sort()
}

func (player *Player) AddItem(st *itemdef.ItemParamSt) bool {
	sysObj := player.GetBagByItemType(st.ItemId)
	if sysObj != nil {
		return sysObj.AddItem(st)
	}
	return false
}

func (player *Player) OnRecvMessage(msgType int, msg pb3.Message) {
	if sys, ok := player.GetSysObj(sysdef.SiMessage).(*actorsystem.MessageSys); ok {
		sys.OnRecvMessage(msgType, msg)
	}
}

func (player *Player) CopyMoneys() map[uint32]int64 {
	return player.GetMoneySys().CopyMoneys()
}

func (player *Player) SendFbResult(settle *pb3.FbSettlement) {
	rsp := &pb3.S2C_17_254{
		Settle: settle,
	}
	player.SendProto3(17, 254, rsp)
}

func (player *Player) SendTipMsg(tipMsgId uint32, params ...interface{}) {
	player.SendProto3(5, 0, common.PackMsg(tipMsgId, params...))
}

func (player *Player) CanAddItem(param *itemdef.ItemParamSt, overlay bool) bool {
	sysObj := player.GetBagByItemType(param.ItemId)
	if sysObj != nil {
		return sysObj.CanAddItem(param, overlay)
	}
	return false
}

func (player *Player) GetAddItemNeedGridCount(param *itemdef.ItemParamSt, overlay bool) int64 {
	sysObj := player.GetBagByItemType(param.ItemId)
	if sysObj != nil {
		return sysObj.GetAddItemNeedGridCount(param, overlay)
	}
	return 0
}

func (player *Player) AddAmount(Diamond, cashCent uint32, logId pb3.LogId, chargeId uint32) {
	if data := player.GetBinaryData(); nil != data {
		chargeInfo := data.GetChargeInfo()
		total := chargeInfo.GetTotalChargeDiamond() + Diamond
		chargeInfo.TotalChargeDiamond = total
		chargeInfo.DailyChargeDiamond += Diamond

		chargeInfo.TotalChargeMoney = chargeInfo.TotalChargeMoney + cashCent
		chargeInfo.DailyChargeMoney += cashCent
		chargeInfo.DailyChargeMoneyMap[time_util.GetZeroTime(time_util.NowSec())] += cashCent

		if custom_id.IsRealChargeLog(logId) {
			realChargeInfo := data.GetRealChargeInfo()
			realChargeInfo.TotalChargeDiamond += Diamond
			realChargeInfo.DailyChargeDiamond += Diamond

			realChargeInfo.TotalChargeMoney += cashCent
			realChargeInfo.DailyChargeMoney += cashCent
			realChargeInfo.DailyChargeMoneyMap[time_util.GetZeroTime(time_util.NowSec())] += cashCent
		}

		player.TriggerEvent(custom_id.AeCharge, &custom_id.ActorEventCharge{
			Diamond:  Diamond,
			CashCent: cashCent,
			LogId:    logId,
			ChargeId: chargeId,
		})
		player.TriggerQuestEvent(custom_id.QttTotalRecharge, 0, int64(total))
		player.TriggerQuestEvent(custom_id.QttTodayChargeXAmount, 0, int64(chargeInfo.DailyChargeMoney))

		player.TriggerQuestEvent(custom_id.QttCharge, 0, int64(Diamond))
		player.TriggerQuestEvent(custom_id.QttChargeNoRecord, 0, int64(Diamond))
		player.addVipExp(Diamond)
		player.SendChargeInfo()

		if logId != pb3.LogId_LogReceiveFreePriRedPacket {
			player.UpdateStatics("charge_diamond", total)
			player.UpdateStatics("last_charge_time", time_util.NowSec())
		}
	}
}

func (player *Player) AddVipExp(exp uint32) {
	sys, ok := player.GetSysObj(sysdef.SiVip).(*actorsystem.VipSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.AddExp(exp)
}

func (player *Player) addVipExp(exp uint32) {
	player.GetSysObj(sysdef.SiVip).(*actorsystem.VipSys).AddExp(exp)
}

// 充值
func (player *Player) Charge(Diamond int64, logId pb3.LogId) {
	player.AddMoney(moneydef.Diamonds, Diamond, true, logId)
	player.SendChargeInfo()
}

func (player *Player) SendLoginOrNewDayBaseInfo() {
	player.SendChargeInfo()
}

func (player *Player) SendChargeInfo() {
	binary := player.GetBinaryData()
	chargeInfo := binary.GetChargeInfo()
	player.SendProto3(36, 0, &pb3.S2C_36_0{
		DailyCharge:            chargeInfo.GetDailyChargeDiamond(),
		TotalCharge:            chargeInfo.GetTotalChargeDiamond(),
		TotalChargeMoney:       chargeInfo.GetTotalChargeMoney(),
		DailyChargeMoney:       chargeInfo.GetDailyChargeMoney(),
		ChargeFlag:             binary.ChargeFlag,
		ChargeExtraRewardsFlag: binary.ChargeExtraRewardsFlag,
	})

	realChargeInfo := binary.GetRealChargeInfo()
	player.SendProto3(36, 10, &pb3.S2C_36_10{
		DailyCharge:      realChargeInfo.GetDailyChargeDiamond(),
		TotalCharge:      realChargeInfo.GetTotalChargeDiamond(),
		TotalChargeMoney: realChargeInfo.GetTotalChargeMoney(),
		DailyChargeMoney: realChargeInfo.GetDailyChargeMoney(),
	})
}

func (player *Player) SendServerInfo() {
	rsp := manager.PackServerInfo()
	player.SendProto3(2, 6, rsp)
}

func (player *Player) SendYYInfo(msg *pb3.S2C_127_255) {
	obj, ok := player.GetSysObj(sysdef.SiPlayerYY).(iface.IPlayerYYMgr)
	if !ok {
		player.SendProto3(127, 255, msg)
		return
	}

	obj.SendPlayerYYInfo(msg)
}

func (player *Player) GetGmLevel() uint32 {
	return player.GmLevel
}

// EnterFightSrv 进入战斗服
func (player *Player) EnterFightSrv(fightType base.ServerType, todoId uint32, todoMsg pb3.Message, args ...interface{}) error {
	// 检查背包格子
	if !player.CheckBagCount(todoId) {
		return fmt.Errorf("enter fight err: player(%d) bagCount Limit", player.GetId())
	}

	if player.InDartCar() && todoId > 0 && todoId != fubendef.EnterCrossDartCar {
		player.SendTipMsg(tipmsgid.Tpindartcar)
		return fmt.Errorf("enter fight err: player(%d) in dartCar", player.GetId())
	}

	if !player.CheckEnterFb(todoId) {
		if fbConf := jsondata.GetFbConf(player.GetFbId()); fbConf != nil {
			player.SendTipMsg(tipmsgid.EnterFbLimit, fbConf.FbName)
			return fmt.Errorf("enter fight err: player(%d) in otherFb", player.GetId())
		}
	}

	if pkConf := jsondata.GetPkConf(); nil != pkConf {
		if player.GetFbId() == 0 && player.GetSceneId() == pkConf.PrisonSceneId {
			return fmt.Errorf("enter fight err: player(%d) in prision", player.GetId())
		}
	}

	// 检查消耗
	if len(args) > 0 {
		consumesSt, ok := args[0].(*argsdef.ConsumesSt)
		if !ok {
			return fmt.Errorf("player(%d) enter fight consume err", player.GetId())
		}
		if len(consumesSt.Consumes) > 0 && !player.ConsumeByConf(consumesSt.Consumes, false, common.ConsumeParams{LogId: consumesSt.LogId}) {
			player.SendTipMsg(tipmsgid.TpItemNotEnough)
			return fmt.Errorf("enter fight err: player(%d) item not enough", player.GetId())
		}
	}
	return player.DoEnterFightSrv(fightType, todoId, todoMsg)
}

func (player *Player) DoEnterFightSrv(fightType base.ServerType, todoId uint32, todoMsg pb3.Message) error {
	if !fightType.IsFightSrv() {
		return fmt.Errorf("fight srv type(%d) is invalid!!!", fightType)
	}

	buf, err := pb3.Marshal(todoMsg)
	if nil != err {
		player.LogError("enter fight srv marsh error! %v", err)
		return fmt.Errorf("enter fight srv marshal error! %s", err)
	}

	if nil == player.proxy {
		player.proxy = NewActorProxy(player, base.LocalFightServer)
		return player.proxy.InitEnter(player, todoId, buf)
	}

	if player.proxy.GetProxyType() == fightType {
		return player.proxy.ToDo(todoId, todoMsg)
	}

	// 在其他服务器
	return player.CallActorFunc(actorfuncid.ChangeFightSrv, &pb3.ChangeFightSrvReq{
		FightSrvType: uint32(fightType),
		TodoId:       todoId,
		Buf:          buf,
	})
}

func (player *Player) CheckBagCount(todoId uint32) bool {
	var hdlId uint32
	if todoId == fubendef.EnterActivity {
		hdlId = fubendef.EnterActivity
	} else {
		hdlId = todoId
	}

	fbConf := jsondata.GetFbConfByHdlId(hdlId)
	if fbConf == nil {
		return false
	}

	for _, bagId := range fbConf.CheckBags {
		bagConf := jsondata.GetBagConf(int(bagId))
		if bagConf == nil {
			logger.LogError("bagId: %d not found config", bagId)
			return false
		}

		count := bagConf.CountLimit
		if bagId == bagdef.BagType && player.GetBagAvailableCount() < count {
			player.SendProto3(17, 250, &pb3.S2C_17_250{
				HdlId:       hdlId,
				Result:      false,
				BagType:     bagId,
				RemainLimit: count,
			})
			return false
		}

		if bagId == bagdef.BagGodGodBeastType && player.GetGodBeastBagAvailableCount() < count {
			player.SendProto3(17, 250, &pb3.S2C_17_250{
				HdlId:       hdlId,
				Result:      false,
				BagType:     bagId,
				RemainLimit: count,
			})
			return false
		}

		if bagId == bagdef.BagFairySpirit && player.GetFairySpiritBagAvailableCount() < count {
			player.SendProto3(17, 250, &pb3.S2C_17_250{
				HdlId:       hdlId,
				Result:      false,
				BagType:     bagId,
				RemainLimit: count,
			})
			return false
		}
	}

	player.SendProto3(17, 250, &pb3.S2C_17_250{
		HdlId:  hdlId,
		Result: true,
	})
	return true
}

func (player *Player) CheckEnterFb(todoId uint32) bool {
	fbId := player.GetFbId()
	fbConf := jsondata.GetFbConf(fbId)
	if fbConf != nil {
		// CanEnterHdlIds 为空的话，表示不能进入其他副本
		if len(fbConf.CanEnterHdlIds) == 0 {
			return false
		}
		// CanEnterHdlIds包含0，表示全部副本都可以切换
		if utils.SliceContainsUint32(fbConf.CanEnterHdlIds, 0) {
			return true
		}
		return utils.SliceContainsUint32(fbConf.CanEnterHdlIds, todoId)
	}
	return false
}

func (player *Player) IsInPrison() bool {
	conf := jsondata.GetPkConf()
	if nil == conf {
		return false
	}
	sceneId := player.GetSceneId()
	return sceneId == conf.PrisonSceneId
}

func (player *Player) ChangeCastDragonEquip() {
	player.TriggerEvent(custom_id.AeChangeCastDragonEquip)
}

func (player *Player) FullHp() {
	player.CallActorFunc(actorfuncid.LevelUpFullHp, nil)
}

// EnterMainScene 进入主城
func (player *Player) EnterMainScene() {
	player.CallActorFunc(actorfuncid.BackToMain, nil)
}

// IsExistFriend 是否是好友
func (player *Player) IsExistFriend(targetId uint64) bool {
	friendSys := player.GetSysObj(sysdef.SiFriend).(*actorsystem.FriendSys)
	if !friendSys.IsExistFriend(targetId, custom_id.FrFriend) {
		return false
	}
	return true
}

// 收到战斗服同步坐标信息
func (player *Player) onActorSyncLocation(sceneId uint32, x, y int32) {
	binary := player.GetBinaryData()
	if nil == binary.Pos {
		binary.Pos = &pb3.ActorPosition{}
	}
	pos := binary.Pos
	pos.SceneId = sceneId
	pos.PosX = x
	pos.PosY = y
}

func (player *Player) onActorSyncCurrentPos(sceneId uint32, x, y int32) {
	player.currentSceneId = sceneId
	player.currentLocalX = x
	player.currentLocalY = y
}

// 收到战斗服同步坐标信息
func (player *Player) onActorSyncFbId(fbId, sceneId uint32) {
	player.FbId = fbId
	player.SceneId = sceneId
}

// 收到战斗服返回可使用的传送石
func (player *Player) onActorUseItems(itemType, itemId uint32, canUse bool) {
	if canUse {
		switch itemType {
		case itemdef.BackToMain:
			player.CallActorFunc(actorfuncid.ReqUseItem, &pb3.CheckItemUse{ItemId: itemId, ItemType: itemdef.BackToMain})
		case itemdef.RandTransfer:
			player.CallActorFunc(actorfuncid.ReqUseItem, &pb3.CheckItemUse{ItemId: itemId, ItemType: itemdef.RandTransfer})
		}
	} else {
		switch itemType {
		case itemdef.BackToMain:
			player.SendTipMsg(tipmsgid.BackToMainFailed)
		case itemdef.RandTransfer:
			player.SendTipMsg(tipmsgid.RandTransferFailed)
		}
	}
}

// SendCloseActor 关闭actor
func (player *Player) SendCloseActor() {
	proxy := player.proxy
	if nil != proxy {
		proxy.Exit()
	}
	player.DestroyEntity = true
}

func (player *Player) CheckFightTimeout() {
	if player.FightHeart == 0 {
		return
	}
	if time_util.NowSec() <= player.FightHeart {
		return
	}
	player.proxy = nil
	// 超时 无论在跨服超时还是本服超时，统一请求进入本服战斗服
	player.EnterLastFb()
}

func (player *Player) UpdateStatics(key string, value interface{}) {
	player.statics[key] = value
	player.saveStatics = true
}

// TaskRecord 任务记录
func (player *Player) TaskRecord(tp, id uint32, count int64, isAdd bool) {
	taskRecordMap := player.BinaryData.GetTaskCompleteRecord()

	if nil == taskRecordMap {
		taskRecordMap = make(map[uint32]uint32)
		player.BinaryData.TaskCompleteRecord = taskRecordMap
	}

	key := utils.Make32(uint16(tp), uint16(id))
	tmpCount := taskRecordMap[key]

	if isAdd {
		taskRecordMap[key] = tmpCount + uint32(count)
	} else {
		taskRecordMap[key] = uint32(count)
	}
}

func (player *Player) GetTaskRecord(tp uint32, ids []uint32) (ret uint32) {
	taskRecordMap := player.BinaryData.GetTaskCompleteRecord()

	if len(ids) > 0 {
		for _, id := range ids {
			key := utils.Make32(uint16(tp), uint16(id))
			ret += taskRecordMap[key]
		}
	} else {
		key := utils.Make32(uint16(tp), uint16(0))
		ret = taskRecordMap[key]
	}

	return ret
}

func (player *Player) InLocalFightSrv() bool {
	return player.GetExtraAttr(attrdef.FightSrvType) == attrdef.AttrValueAlias(base.LocalFightServer)
}

func (player *Player) SetAccountName(name string) {
	player.AccountName = name
}

func (player *Player) AddBuff(BuffId uint32) {
	buffConf := jsondata.GetBuffConfig(BuffId)
	if nil == buffConf {
		return
	}

	msg := &pb3.AddBuffSt{
		BuffId: BuffId,
	}

	player.CallActorFunc(actorfuncid.GFAddBuff, msg)
}

func (player *Player) AddBuffByTime(buffId uint32, dur int64) {
	buffConf := jsondata.GetBuffConfig(buffId)
	if nil == buffConf {
		return
	}

	msg := &pb3.AddBuffSt{
		BuffId: buffId,
		Dur:    dur,
	}

	player.CallActorFunc(actorfuncid.GFAddBuff, msg)
}

func (player *Player) DelBuff(BuffId uint32) {
	player.CallActorFunc(actorfuncid.GFDelBuff, &pb3.AddBuffSt{
		BuffId: BuffId,
	})
}

func (player *Player) GetLoginTrace() bool {
	return player.LoginTrace
}

func (player *Player) SetLoginTrace(flag bool) {
	player.LoginTrace = flag
}

func (player *Player) logLogin() {
	logworker.LogLogin(player, &pb3.LogLogin{
		Type: common.LogLogin,
	})
}

func (player *Player) logLogout() {
	logworker.LogLogin(player, &pb3.LogLogin{
		Type: common.LogLogout,
	})
}

func (player *Player) logReLogin() {
	logworker.LogLogin(player, &pb3.LogLogin{
		Type: common.LogReLogin,
	})
}

func (player *Player) DelItemByHand(hand uint64, logId pb3.LogId) {
	pItem := player.GetItemByHandle(hand)
	if nil == pItem {
		return
	}

	player.DeleteItemPtr(pItem, pItem.GetCount(), logId)
}

func (player *Player) SetGuildId(guildId uint64) {
	player.SetExtraAttr(attrdef.GuildId, int64(guildId))

	binary := player.GetBinaryData()
	oldGuildId := binary.GuildData.GetGuildId()
	binary.GuildData.GuildId = guildId
	if 0 == guildId {
		player.TriggerEvent(custom_id.AeLeaveGuild, oldGuildId)
	} else {
		player.TriggerEvent(custom_id.AeJoinGuild)
	}
}

func (player *Player) GetGuildId() uint64 {
	guildData := player.GetBinaryData().GetGuildData()
	if nil != guildData {
		return guildData.GetGuildId()
	}
	return 0
}

func (player *Player) IsGuildLeader() bool {
	guildId := player.GetGuildId()
	if guildId == 0 {
		return false
	}

	guild := guildmgr.GetGuildById(guildId)
	if guild == nil {
		return false
	}

	return guild.GetLeaderId() == player.GetId()
}

func (player *Player) SetQuitGuildCd(exitTime uint32) {
	conf := jsondata.GetGuildConf()
	if nil == conf || nil == conf.Create {
		return
	}
	guildData := player.GetBinaryData().GuildData
	cd := jsondata.GlobalUint("guildExitCd") + exitTime
	if cd < guildData.CoolTime {
		return
	}
	guildData.CoolTime = cd
	player.SetExtraAttr(attrdef.GuildCoolTime, int64(guildData.CoolTime))
	if guildData.GuildId == 0 {
		player.SendProto3(29, 1, &pb3.S2C_29_1{CoolTime: guildData.GetCoolTime(), GuildRule: guildmgr.GetPfRule()})
	}
}

func (player *Player) GetGuildName() string {
	if sys, ok := player.GetSysObj(sysdef.SiGuild).(*actorsystem.GuildSys); ok {
		if guild := sys.GetGuild(); nil != guild {
			return guild.GetName()
		}
	}
	return ""
}

// ToPlayerDataBase 玩家离线数据
func (player *Player) ToPlayerDataBase() *pb3.PlayerDataBase {
	skills := map[uint32]uint32{}
	for skillId, skill := range player.MainData.Skills {
		skills[skillId] = skill.Level
	}
	appear := make(map[uint32]*pb3.SysAppearSt, len(player.MainData.AppearInfo.Appear))
	for k, v := range player.MainData.AppearInfo.Appear {
		appear[k] = &pb3.SysAppearSt{
			SysId:    v.SysId,
			AppearId: v.AppearId,
		}
	}
	binary := player.GetBinaryData()
	st := &pb3.PlayerDataBase{
		Id:               player.GetId(),
		Name:             player.GetName(),
		Circle:           player.GetCircle(),
		Lv:               player.GetLevel(),
		VipLv:            binary.GetVip(),
		Job:              player.GetJob(),
		Sex:              player.GetSex(),
		LastLogoutTime:   player.GetMainData().GetLastLogoutTime(),
		LoginTime:        player.GetMainData().GetLoginTime(),
		Power:            uint64(player.GetExtraAttr(attrdef.FightValue)),
		GuildId:          player.GetGuildId(),
		GuildName:        player.GetGuildName(),
		PowerCompare:     player.getPowerCompare(),
		Skills:           skills,
		Head:             player.GetHead(),
		HeadFrame:        player.GetHeadFrame(),
		BubbleFrame:      player.GetBubbleFrame(),
		AppearInfo:       appear,
		SmallCrossCamp:   int32(player.GetSmallCrossCamp()),
		MiddleCrossCamp:  player.GetExtraAttr(attrdef.MediumCrossCamp),
		CharacterTags:    binary.CharacterTags,
		ConfessionLv:     binary.ConfessionLv,
		DailyChargeMoney: binary.GetChargeInfo().GetDailyChargeMoney(),
		FreeVipLv:        player.GetExtraAttrU32(attrdef.FreeVipLv),
		WeddingRing:      player.GetExtraAttrU32(attrdef.MRingLv),
		FlyCamp:          player.GetFlyCamp(),
		SysOpenStatus:    player.GetSysStatusData(),
	}

	if nil != binary.GetMageBody() {
		st.MageLvStar = utils.Make64(binary.GetMageBody().Star, binary.GetMageBody().Lv)
	}

	player.packMarryBaseData(st)
	player.packDragonEquips(st)
	player.packBattleFaBao(st)
	player.packFaGulData(st)
	player.packDragonBall(st)
	player.packBattleFairy(st)
	player.packSoulHalo(st)
	player.packFairyColdWeapon(st)
	player.packKillDragonEq(st)

	player.LogTrace("玩家战力评分: %v", player.getPowerCompare())
	return st
}

func (player *Player) packMarryBaseData(st *pb3.PlayerDataBase) {
	data := player.GetBinaryData().MarryData
	if nil == data {
		return
	}
	if friendmgr.IsExistStatus(data.CommonId, custom_id.FsEngagement) {
		st.MarryCommonId = data.CommonId
	}
	if friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry) { //有婚姻关系
		st.MarryId = data.MarryId
		st.MarryName = data.MarryName
	}
}

func (player *Player) packDragonEquips(st *pb3.PlayerDataBase) {
	data := player.GetBinaryData().DragonData
	if data == nil || data.DragonEqData == nil {
		return
	}
	st.DragonEquips = make(map[uint32]uint32)
	for k, v := range data.DragonEqData.Equips {
		st.DragonEquips[k] = v
	}
}

func (player *Player) packBattleFaBao(st *pb3.PlayerDataBase) {
	state := player.GetBinaryData().FaBaoState
	if state == nil || state.BattleSlots == nil {
		return
	}

	st.BattleFaBao = make(map[uint32]*pb3.NewFaBao)
	for id, faBao := range state.BattleSlots {
		st.BattleFaBao[id] = &pb3.NewFaBao{
			Id:      faBao.Id,
			Quality: faBao.Quality,
			Star:    faBao.Star,
			Lv:      faBao.Lv,
			Exp:     faBao.Exp,
		}
	}
}

func (player *Player) packFaGulData(st *pb3.PlayerDataBase) {
	data := player.GetBinaryData().FaGulData
	if data == nil {
		return
	}

	st.FaGulData = make(map[uint32]*pb3.FaGul)
	for k, faGul := range data {
		fData := &pb3.FaGul{
			FaRing: make(map[uint32]uint32),
		}
		fData.GulId = faGul.GulId
		fData.GulLv = faGul.GulLv
		for k, v := range faGul.FaRing {
			fData.FaRing[k] = v
		}
		st.FaGulData[k] = fData
	}
}

func (player *Player) packDragonBall(st *pb3.PlayerDataBase) {
	data := player.GetBinaryData().DragonBallData
	if data == nil {
		return
	}

	st.DragonBall = make(map[uint32]uint32)
	for k, v := range data {
		st.DragonBall[k] = v
	}
}

func (player *Player) packBattleFairy(st *pb3.PlayerDataBase) {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*actorsystem.FairySystem)
	if !ok || !sys.IsOpen() {
		return
	}

	st.BattleFairy = make(map[uint32]*pb3.ItemSt)
	for pos := range sys.GetBattleFairy() {
		itemSt := sys.GetFairyByBattlePos(pos)
		if itemSt == nil {
			continue
		}
		st.BattleFairy[pos] = itemSt
	}
}

func (player *Player) packSoulHalo(st *pb3.PlayerDataBase) {
	sys, ok := player.GetSysObj(sysdef.SiSoulHalo).(*actorsystem.SoulHaloSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	st.SoulHalo = make(map[uint32]uint32)
	for slot, slotInfo := range data.SoltInfo {
		st.SoulHalo[slot] = slotInfo.SoulHalo.GetItemId()
	}
}

func (player *Player) packFairyColdWeapon(st *pb3.PlayerDataBase) {
	sys, ok := player.GetSysObj(sysdef.SiFairyColdWeapon).(*actorsystem.FairyColdWeaponSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	st.FairyCWData = data.WeaponMap
}

func (player *Player) packKillDragonEq(st *pb3.PlayerDataBase) {
	sys, ok := player.GetSysObj(sysdef.SiKillDragonEquipSuit).(*actorsystem.KillDragonEquipSuitSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	st.KillDragonEqs = data.EquipCastMap
}

func (player *Player) getPowerCompare() map[uint32]int64 {
	powerC := make(map[uint32]int64)
	attrSys := player.AttrSys
	powerCompareSlice := jsondata.GetPowerCompareConf()
	for _, conf := range powerCompareSlice {
		var power int64
		for _, attrDefId := range conf.AttrGroup {
			power += attrSys.GetSysPower(attrDefId)
			continue
		}
		powerC[conf.Id] += power
	}
	return powerC
}

func (player *Player) GetPrivilege(pType privilegedef.PrivilegeType) (total int64, err error) {
	obj := player.GetSysObj(sysdef.SiPrivilegeSys)
	if obj == nil {
		return 0, fmt.Errorf("PrivilegeSys is nil")
	}
	sys, ok := obj.(*actorsystem.PrivilegeSys)
	if !ok || !sys.IsOpen() {
		return 0, nil
	}

	return sys.GetPrivilege(pType)
}

func (player *Player) HasPrivilege(pType privilegedef.PrivilegeType) bool {
	sys := player.GetSysObj(sysdef.SiPrivilegeSys).(*actorsystem.PrivilegeSys)

	if sys == nil || !sys.IsOpen() {
		return false
	}

	value, err := sys.GetPrivilege(pType)
	if err == nil && value > 0 {
		return true
	}
	return false
}

func (player *Player) sendSettings() {
	binaryData := player.GetBinaryData()
	if nil == binaryData {
		return
	}

	settings := &pb3.S2C_2_5{
		Settings: player.BinaryData.Settings,
	}

	player.SendProto3(2, 5, settings)
}

func (player *Player) GetBubbleFrame() uint32 {
	bubbleFrameRaw := player.GetExtraAttr(attrdef.AppearBubbleFrame)
	foo := bubbleFrameRaw >> 32
	bubbleFrame := (foo << 32) ^ bubbleFrameRaw
	return uint32(bubbleFrame)
}

func (player *Player) GetHeadFrame() uint32 {
	headFrameRaw := player.GetExtraAttr(attrdef.AppearHeadFrame)
	foo := headFrameRaw >> 32
	headframe := (foo << 32) ^ headFrameRaw
	return uint32(headframe)
}

func (player *Player) GetHead() uint32 {
	headRaw := player.GetExtraAttr(attrdef.AppearHead)
	foo := headRaw >> 32
	head := (foo << 32) ^ headRaw
	return uint32(head)
}

func (player *Player) GetSmallCrossCamp() crosscamp.CampType {
	return crosscamp.CampType(player.GetExtraAttr(attrdef.SmallCrossCamp))
}

func (player *Player) GetMediumCrossCamp() uint64 {
	return uint64(player.GetExtraAttr(attrdef.MediumCrossCamp))
}

func (player *Player) GetTeamId() uint64 {
	return player.teamId
}

func (player *Player) SetTeamId(teamId uint64) {
	player.teamId = teamId
}

func (player *Player) HasState(bit uint32) bool {
	return player.StateBitSet.Get(bit)
}

func (player *Player) SetLastFightTime(timeStamp uint32) {
	player.lastFightTime = timeStamp
}

func (player *Player) GetFightStatus() bool {
	return player.HasState(custom_id.EsInFight)
}

func (player *Player) InDartCar() bool {
	return player.GetExtraAttrU32(attrdef.DartCarType) > 0
}

func (player *Player) DoFirstExperience(typ pb3.ExperienceType, unExperienceFunc func() error) error {
	obj := player.GetSysObj(sysdef.SiFirstExperience)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("not open SiFirstExperience")
	}

	sys, ok := obj.(*actorsystem.FirstExperienceSys)
	if !ok {
		return neterror.SysNotExistError("convert SiFirstExperience failed")
	}

	experience, err := sys.CheckExperience(typ)
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}

	if experience {
		return unExperienceFunc()
	}

	sys.StartFirstExperience(uint32(typ))
	return nil
}

func (player *Player) InitiativeEndExperience(typ pb3.ExperienceType, commonSt *pb3.CommonSt) error {
	obj := player.GetSysObj(sysdef.SiFirstExperience)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("not open SiFirstExperience")
	}

	sys, ok := obj.(*actorsystem.FirstExperienceSys)
	if !ok {
		return neterror.SysNotExistError("convert SiFirstExperience failed")
	}

	sys.EndFirstExperience(uint32(typ), commonSt)
	return nil
}

func (player *Player) ResetSpecCycleBuy(typ uint32, subType uint32) {
	obj := player.GetSysObj(sysdef.SiStore)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*actorsystem.StoreSys)
	if !ok {
		return
	}
	err := sys.ResetSpecCycleBuy(typ, subType)
	if err != nil {
		player.LogError("err:%d", err)
	}
}

func (player *Player) ChannelChat(req *pb3.C2S_5_1, checkCd bool) {
	sys, ok := player.GetSysObj(sysdef.SiChat).(*actorsystem.ChatSys)
	if !ok {
		return
	}
	if checkCd {
		target := manager.GetPlayerPtrById(req.ToId)
		if !sys.Check(req.Channel, target) {
			return
		}
	}
	sys.ChannelChat(req)
}

func (player *Player) SetRankValue(rankType uint32, value int64) {
	binary := player.GetBinaryData()
	if binary.RankValues == nil {
		binary.RankValues = make(map[uint32]int64)
	}
	binary.RankValues[rankType] = value
}

func (player *Player) GetRankValue(rankType uint32) int64 {
	binary := player.GetBinaryData()
	if rankType == gshare.RankTypeGuild {
		guild := guildmgr.GetGuildById(player.GetGuildId())
		if guild == nil {
			return 0
		}
		lv := guild.GetLevel()
		power := guild.GetPower()
		score := int64(lv)<<56 | int64(power)
		return score
	}
	value := binary.RankValues[rankType]
	return value
}

func (player *Player) SetYYRankValue(rankType uint32, value int64) {
	binary := player.GetBinaryData()
	if binary.YyRankValues == nil {
		binary.YyRankValues = make(map[uint32]int64)
	}
	binary.YyRankValues[rankType] = value
}

func (player *Player) GetYYRankValue(rankType uint32) int64 {
	binary := player.GetBinaryData()
	value := binary.YyRankValues[rankType]
	return value
}

func (player *Player) GetNirvanaLevel() uint32 {
	val := player.GetExtraAttr(attrdef.NirvanaLvAndSubLv)
	if val == 0 {
		return 0
	}
	level := utils.High32(uint64(val))
	if level > 0 {
		return level
	}
	if player.PlayerData.BinaryData == nil || player.PlayerData.BinaryData.NirvanaData == nil {
		return 0
	}
	return player.PlayerData.BinaryData.NirvanaData.Lv
}

func (player *Player) GetNirvanaSubLevel() uint32 {
	val := player.GetExtraAttr(attrdef.NirvanaLvAndSubLv)
	if val == 0 {
		return 0
	}
	level := utils.Low32(uint64(val))
	if level > 0 {
		return level
	}
	if player.PlayerData.BinaryData == nil || player.PlayerData.BinaryData.NirvanaData == nil {
		return 0
	}
	return player.PlayerData.BinaryData.NirvanaData.SubLv
}

func (player *Player) GetCurrentPos() (int32, int32) {
	return player.currentLocalX, player.currentLocalY
}

func (player *Player) GetKillMonsterRecordData(subType uint32) *pb3.KillMonsterRecordEntry {
	data := player.GetBinaryData()
	recordData := data.KillMonsterRecordData
	if recordData == nil {
		data.KillMonsterRecordData = &pb3.KillMonsterRecordData{}
		recordData = data.KillMonsterRecordData
	}

	if recordData.RecordMap == nil {
		recordData.RecordMap = make(map[uint32]*pb3.KillMonsterRecordEntry)
	}
	if recordData.RecordMap[subType] == nil {
		recordData.RecordMap[subType] = &pb3.KillMonsterRecordEntry{}
	}
	return recordData.RecordMap[subType]
}

func (player *Player) RefreshDropTimes() {
	binData := player.GetBinaryData()
	dropData := binData.DropData
	if dropData != nil {
		dropData.DailyDropTimes = make(map[uint32]uint32)
		player.CallActorFunc(actorfuncid.G2FClearDropDailyTimes, nil)
	}
}

func (player *Player) RefreshWeeklyDropTimes() {
	binData := player.GetBinaryData()
	dropData := binData.DropData
	if dropData != nil {
		dropData.WeeklyDropTimes = make(map[uint32]uint32)
		player.CallActorFunc(actorfuncid.G2FClearDropWeeklyTimes, nil)
	}
}

// FinishBossQuickAttack 扫荡boss 触发击杀事件
func (player *Player) FinishBossQuickAttack(monId, sceneId, fbId uint32) {
	player.TriggerEvent(custom_id.AeKillMon, monId, sceneId, uint32(1), fbId)
}

func (player *Player) pubBaseInfoToCross() {
	actorId := player.GetId()
	baseData := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	data, err := pb3.Marshal(baseData)
	if nil == err {
		err = mq.PubCrossFightPlayerDataMqHandle(engine.GetPfId(), engine.GetServerId(), actorId, data)
	}
	if nil != err {
		player.LogError("Player:Save publish error! %v", err)
	}
}

func (player *Player) sendShowRewardsPop(rewards jsondata.StdRewardVec, msg *pb3.S2C_4_25) {
	if msg == nil {
		msg = &pb3.S2C_4_25{}
	}
	showRewards := jsondata.FilterRewardByOption(rewards,
		jsondata.WithFilterRewardOptionByJob(player.GetJob()),
		jsondata.WithFilterRewardOptionBySex(player.GetSex()),
		jsondata.WithFilterRewardOptionByOpenDayRange(gshare.GetOpenServerDay()),
	)
	msg.ShowAward = jsondata.StdRewardVecToPb3RewardVec(showRewards)
	player.SendProto3(4, 25, msg)
}
func (player *Player) SendShowRewardsPop(rewards jsondata.StdRewardVec) {
	player.sendShowRewardsPop(rewards, nil)
}

func (player *Player) SendShowRewardsPopByPYY(rewards jsondata.StdRewardVec, id uint32) {
	player.sendShowRewardsPop(rewards, &pb3.S2C_4_25{PyyId: id})
}

func (player *Player) SendShowRewardsPopBySys(rewards jsondata.StdRewardVec, id uint32) {
	player.sendShowRewardsPop(rewards, &pb3.S2C_4_25{SysId: id})
}

func (player *Player) SendShowRewardsPopByYY(rewards jsondata.StdRewardVec, id uint32) {
	player.sendShowRewardsPop(rewards, &pb3.S2C_4_25{YyId: id})
}

func (player *Player) GetRankDragonEqData() *pb3.RankDragonEqData {
	sys, ok := player.GetSysObj(sysdef.SiDragon).(*actorsystem.DragonSys)
	if !ok || !sys.IsOpen() {
		return nil
	}
	eqData := sys.GetDragonEqData()
	return &pb3.RankDragonEqData{
		Equips: eqData.Equips,
	}
}

func (player *Player) GetRankFairyData() *pb3.RankFairyData {
	sys, ok := player.GetSysObj(sysdef.SiFairy).(*actorsystem.FairySystem)
	if !ok || !sys.IsOpen() {
		return nil
	}

	posMap := make(map[uint32]*pb3.RankFairy)
	for pos := range sys.GetBattleFairy() {
		if !itemdef.IsFairyMainPos(pos) {
			continue
		}
		itemSt := sys.GetFairyByBattlePos(pos)
		if itemSt == nil {
			continue
		}
		posMap[pos] = &pb3.RankFairy{
			ItemId: itemSt.ItemId,
			Lv:     itemSt.Union1,
			Star:   itemSt.Union2,
		}
	}

	return &pb3.RankFairyData{BattleFairy: posMap}
}

func (player *Player) GetRankSoulHaloData() *pb3.RankSoulHaloData {
	sys, ok := player.GetSysObj(sysdef.SiSoulHalo).(*actorsystem.SoulHaloSys)
	if !ok || !sys.IsOpen() {
		return nil
	}

	data := sys.GetData()
	slotMap := make(map[uint32]uint32)
	for slot, slotInfo := range data.SoltInfo {
		slotMap[slot] = slotInfo.SoulHalo.GetItemId()
	}

	return &pb3.RankSoulHaloData{SlotInfo: slotMap}
}

func (player *Player) GetRankFairyCWData() *pb3.RankFairyColdWeaponData {
	sys, ok := player.GetSysObj(sysdef.SiFairyColdWeapon).(*actorsystem.FairyColdWeaponSys)
	if !ok || !sys.IsOpen() {
		return nil
	}

	data := sys.GetData()
	return &pb3.RankFairyColdWeaponData{WeaponMap: data.WeaponMap}
}

func (player *Player) GetRankKDragonEqData() *pb3.RankKillDragonEqData {
	sys, ok := player.GetSysObj(sysdef.SiKillDragonEquipSuit).(*actorsystem.KillDragonEquipSuitSys)
	if !ok || !sys.IsOpen() {
		return nil
	}

	data := sys.GetData()
	return &pb3.RankKillDragonEqData{EquipCastMap: data.EquipCastMap}
}

func (player *Player) BuildChatBaseData(target iface.IPlayer) *wm.CommonData {
	data := &wm.CommonData{
		ActorId:                player.GetId(),
		ActorName:              player.GetName(),
		ActorIP:                player.GetRemoteAddr(),
		PlatformUniquePlayerId: player.GetAccountName(),
		SrvId:                  engine.GetServerId(),
	}
	if target != nil {
		data.TargetActorId = target.GetId()
		data.TargetActorName = target.GetName()
		data.PlatformUniqueTargetPlayerId = target.GetAccountName()
	}
	return data
}

func (player *Player) CheckBossSpirit(itemId uint32) bool {
	config := jsondata.GetItemConfig(itemId)
	if config == nil || !itemdef.IsItemTypeBossSpiritShard(config.Type) {
		return true
	}
	obj := player.GetSysObj(sysdef.SiBossSpirit)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys := obj.(*actorsystem.BossSpiritSys)
	if sys == nil {
		return false
	}
	return sys.CheckLootBossSpirit(itemId)
}

func (player *Player) BroadcastCustomTipMsgById(tipMsgId uint32, params ...interface{}) {
	rsp := common.PackCustomTip(tipMsgId, params...)
	sys, ok := player.GetSysObj(sysdef.SiCustomTip).(*actorsystem.CustomTipSys)
	if ok && sys.IsOpen() {
		customTip := sys.GetCustomTip(tipMsgId)
		if customTip != nil {
			rsp.CustomTipId = customTip.Id
			rsp.CustomContent = customTip.Content
		}
	}
	engine.Broadcast(chatdef.CIWorld, 0, 2, 208, rsp, 0)
}

func (player *Player) MergeTimesChallengeBoss(fightType base.ServerType, fbTodoId uint32, mergeTimes uint32) error {
	if s, ok := player.GetSysObj(sysdef.SiActSweep).(*actorsystem.ActSweepSys); ok && s.IsOpen() {
		err := s.MergeTimesChallengeBoss(fightType, fbTodoId, mergeTimes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (player *Player) OpenPyy(id, sTime, eTime, confIdx, timeType uint32, isInit bool) {
	sys, exist := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
	if !exist {
		return
	}
	sys.OpenYY(id, sTime, eTime, confIdx, timeType, isInit)
}

func (player *Player) CheckMarryRewardTimes(gradeConf *jsondata.MarryGradeConf) bool {
	if gradeConf == nil {
		return false
	}
	marrySys, ok := player.GetSysObj(sysdef.SiMarry).(*actorsystem.MarrySys)
	if !ok || !marrySys.IsOpen() {
		return false
	}
	data := marrySys.GetData()
	if data.RecGradeDailyAwards[gradeConf.Type] >= gradeConf.DailyTimes {
		return false
	}
	return true
}

func (player *Player) AddMarryRewardTimes(gradeConf *jsondata.MarryGradeConf) bool {
	if gradeConf == nil {
		return false
	}
	marrySys, ok := player.GetSysObj(sysdef.SiMarry).(*actorsystem.MarrySys)
	if !ok || !marrySys.IsOpen() {
		return false
	}
	data := marrySys.GetData()
	data.RecGradeDailyAwards[gradeConf.Type] += 1
	logworker.LogPlayerBehavior(player, pb3.LogId_LogRecGradeDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(gradeConf.Type),
		StrArgs: fmt.Sprintf("%d", data.RecGradeDailyAwards[gradeConf.Type]),
	})
	return true
}

func onChangeFightSrv(et iface.IPlayer, buf []byte) {
	player, ok := et.(*Player)
	if !ok {
		return
	}

	msg := &pb3.ChangeFightSrvReq{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	if err := player.proxy.SwitchFtype(base.ServerType(msg.FightSrvType)); nil != err {
		player.LogError("onChangeFightSrv failed! %s", err.Error())
		return
	}

	err := player.proxy.InitEnter(player, msg.TodoId, msg.Buf)
	if err != nil {
		player.LogError("onChangeFightSrv err: %v", err)
	}
}

func onLogFightSrvActorBehavior(et iface.IPlayer, buf []byte) {
	msg := &pb3.LogPlayerCounter{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		et.LogError("onLogFightSrvActorBehavior Unmarshal err:%v", err)
		return
	}
	logworker.LogPlayerBehavior(et, pb3.LogId(msg.LogId), msg)
}

func OnFightSrvClientMsg(args ...interface{}) {
	if !gcommon.CheckArgsCount("OnFightSrvClientMsg", 1, len(args)) {
		return
	}

	data, ok := args[0].([]byte)
	if !ok {
		return
	}
	var st pb3.ForwardClientMsgToGame
	if nil != pb3.Unmarshal(data, &st) {
		return
	}
	actorId := st.ActorId
	if engine.IsRobot(actorId) {
		return
	}
	player, ok := manager.GetPlayerPtrById(actorId).(*Player)
	if !ok || nil == player {
		logger.LogWarn("%d not found player", actorId)
		return
	}

	gate := gateworker.GetGateConn(player.GateId)
	if nil == gate {
		logger.LogWarn("%d not found conn", player.GateId)
		return
	}

	clientData, err := pb3.Marshal(&pb3.ClientData{
		ConnId:  player.ConnId,
		ProtoId: st.ProtoId,
		Data:    st.Data,
	})
	if nil != err {
		player.LogError("send proto marshal error! err:%v", err)
		return
	}

	if logger.GetLevel() <= logger.TraceLevel {
		protoH, protoL := st.ProtoId>>8, st.ProtoId&0xFF
		msgName := fmt.Sprintf("pb3.S2C_%d_%d", protoH, protoL)
		protoType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
		if err != nil {
			logger.LogError("SendProtoBuffer Failed err %s", err)
			return
		}
		msg := protoType.New().Interface()
		err = proto.Unmarshal(st.Data, msg)
		if err != nil {
			logger.LogError("SendProtoBuffer failed err %s", err)
		}
		logger.LogTrace("player %d send msg %s {%+v}", st.ActorId, msg.ProtoReflect().Descriptor().FullName(), msg)
	}

	err = gate.SendMessage(base.GW_DATA, clientData)
	if err != nil {
		player.LogError("protoId:%d SendProtoBuffer failed err %s", st.ProtoId, err)
	}
	logworker.LogNetProtoStat(st.ProtoId, uint32(len(st.Data)), engine.GetPfId(), engine.GetServerId())
}

func onReloadActorFinish(args ...interface{}) {
	if !gcommon.CheckArgsCount("onReloadActorFinish", 2, len(args)) {
		return
	}

	actorId, ok := args[0].(uint64)
	if !ok {
		logger.LogError("保存玩家数据失败, 参数1 actorId 不是uint64")
		return
	}

	player, ok := manager.GetPlayerPtrById(actorId).(*Player)
	if !ok {
		return
	}

	data, ok := args[1].([]byte)
	if !ok {
		player.LogError("保存玩家数据失败, 参数2 不是[]byte")
		return
	}

	playerData := &pb3.PlayerData{}
	if err := pb3.Unmarshal(compress.UncompressPb(data), playerData); nil != err {
		player.LogError("reload actor cache error! %v", err)
		return
	}

	player.PlayerData = playerData

	player.ReloadCacheKick = true
	player.ClosePlayer(cmd.DCRKick)
}

func OnNewChargeOrderMsg(args ...interface{}) {
	if !gcommon.CheckArgsCount("OnNewChargeOrderMsg", 1, len(args)) {
		return
	}
	order, ok := args[0].(*model.Charge)
	if !ok {
		return
	}
	actorId := order.ActorId
	var cashCent = uint32(order.CashNum)
	chargeId := order.ChargeId
	payNo := order.PayNo
	cpNo := order.CpNo

	logger.LogDebug("charge order [actorId:%d, chargeId:%d, payNo:%s, cpNo:%s]", actorId, chargeId, payNo, cpNo)

	// 发离线消息处理
	engine.SendPlayerMessage(actorId, gshare.OfflineChargeOrder, &pb3.OfflineCommonSt{
		Param1:    uint64(chargeId),
		U32Param:  cashCent,
		StrParam1: cpNo,
	})
}

func onEnterFightSucc(et iface.IPlayer, buf []byte) {
	var st pb3.SyncFightBegin
	if err := pb3.Unmarshal(buf, &st); nil != err {
		return
	}
	player, ok := et.(*Player)
	if !ok {
		return
	}
	proxy := player.proxy
	if nil == proxy {
		return
	}

	srvType := base.ServerType(st.SrvType)
	if proxy.GetProxyType() != srvType {
		engine.CallFightSrvFunc(srvType, sysfuncid.GFExitFight, &pb3.SyncExitFight{
			ActorId: player.GetId(),
			Handle:  st.Handle,
		})
		return
	}

	player.SetExtraAttr(attrdef.FightSrvType, attrdef.AttrValueAlias(st.SrvType))

	player.FightHandle = st.Handle
	player.FightHeart = time_util.NowSec() + FightTimeOut
	player.TriggerEvent(custom_id.AeLoginFight)
	player.firstEnterFight = false
}

func SaveFData(player *Player, msg *pb3.SaveFActorMiscData) {
	if binary := player.GetBinaryData(); nil != binary {
		binary.Hp = msg.Hp
		binary.LastPkMode = msg.LastPkMode
		binary.PkMode = msg.PkMode
		binary.Zhenyuan = msg.NeiGong
		if msg.UnknownDarkTempleSec != nil {
			if binary.UnknownDarkTempleSec == nil {
				binary.UnknownDarkTempleSec = make(map[uint32]uint32)
			}
			for k, v := range msg.UnknownDarkTempleSec {
				binary.UnknownDarkTempleSec[k] = v
			}
		}
	}

	if sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*actorsystem.ImmortalSoulSystem); ok && sys.IsOpen() {
		sys.SaveSkillData(msg.BattleSoulFurry, msg.BattleSoulReadyPos)
	}

	if sys, ok := player.GetSysObj(sysdef.SiLaw).(*actorsystem.LawSys); ok && sys.IsOpen() {
		sys.SaveSkillData(msg.LawFurry, msg.LawReadyPos)
	}

	if sys, ok := player.GetSysObj(sysdef.SiDomain).(*actorsystem.DomainSys); ok && sys.IsOpen() {
		sys.SaveSkillData(msg.DomainFurry, msg.DomainReadyPos)
	}

	if sys, ok := player.GetSysObj(sysdef.SiDomain).(*actorsystem.DoubleDropPrivilegeSys); ok && sys.IsOpen() {
		sys.SaveTimes(msg.DoubleDropPrivilegeTimes)
	}
}

func onSaveDataFinish(actor iface.IPlayer, buf []byte) {
	msg := &pb3.SaveFActorMiscData{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	player, ok := actor.(*Player)
	if !ok {
		return
	}
	SaveFData(player, msg)
	player.SaveTimestamp = time_util.NowSec() + 1
}

func onSaveDataFiveMin(actor iface.IPlayer, buf []byte) {
	msg := &pb3.SaveFActorMiscData{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	player, ok := actor.(*Player)
	if !ok {
		return
	}
	SaveFData(player, msg)
}

func onChangeFightSrvScene(player iface.IPlayer, buf []byte) {
	msg := &pb3.EnterFubenHdl{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}
	defaultServer := base.LocalFightServer
	if msg.IsCross {
		defaultServer = base.SmallCrossServer
	}
	err := player.DoEnterFightSrv(defaultServer, fubendef.EnterFbHdl, msg)
	if err != nil {
		player.LogError("EnterLastFb failed %s", err)
		return
	}
}

func onReturnToLocalFight(player iface.IPlayer, buf []byte) {
	sceneId := jsondata.GlobalUint("mainSceneId")
	req := &pb3.EnterFubenHdl{
		SceneId: sceneId,
		PosX:    -1,
		PosY:    -1,
	}
	err := player.DoEnterFightSrv(base.LocalFightServer, fubendef.EnterFbHdl, req)
	if err != nil {
		player.LogError("EnterLastFb failed %s", err)
		return
	}
}

// 和enterLastFb类似
func onReturnToLastScene(player iface.IPlayer, buf []byte) {
	binary := player.GetBinaryData()
	var (
		sceneId uint32
		x, y    int32
	)
	if pos := binary.Pos; nil != pos {
		sceneId, x, y = pos.GetSceneId(), pos.GetPosX(), pos.GetPosY()
	} else {
		sceneId := jsondata.GlobalUint("mainSceneId")
		if conf := jsondata.GetSceneConf(sceneId); nil != conf {
			x, y = -1, -1
		}
	}

	err := enterDefaultFbScene(player, &pb3.EnterFubenHdl{
		SceneId: sceneId,
		PosX:    x,
		PosY:    y,
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// 进入场景
func enterDefaultFbScene(player iface.IPlayer, req *pb3.EnterFubenHdl) error {
	sceneId := req.SceneId
	var x, y = req.PosX, req.PosY
	var srvType = base.LocalFightServer

	var inFbZero = true
	fbConf := jsondata.GetFbConf(0)
	if fbConf != nil && !pie.Uint32s(fbConf.SceneIds).Contains(sceneId) {
		player.LogError("sceneId:%d not in fb:0", sceneId)
		srvType = base.LocalFightServer
		sceneId = jsondata.GlobalUint("mainSceneId")
		inFbZero = false
	}

	// 在副本0中
	if inFbZero {
		conf := jsondata.GetSceneConf(sceneId)
		if conf != nil && conf.CrossType != 0 {
			srvType = base.ServerType(conf.CrossType)
		}
		// 配0拿当前所属场景的战斗服类型
		if conf != nil && conf.CrossType == 0 {
			srvType = player.GetActorProxy().GetProxyType()
		}
		if !engine.FightClientExistPredicate(srvType) {
			srvType = base.LocalFightServer
			sceneId = jsondata.GlobalUint("mainSceneId")
			x, y = -1, -1
		}
	}

	req.FbHdl = 0
	req.SceneId = sceneId
	req.PosX = x
	req.PosY = y
	req.IsCross = srvType != base.LocalFightServer

	// 进监狱
	if conf := jsondata.GetPkConf(); nil != conf && player.GetExtraAttrU32(attrdef.PKValue) >= conf.PrisonPKValue {
		srvType = base.LocalFightServer
		req.SceneId = conf.PrisonSceneId
		req.PosX, req.PosY = -1, -1
	}

	err := player.DoEnterFightSrv(srvType, fubendef.EnterFbHdl, req)
	if err != nil {
		player.LogError("EnterLastFb failed %s", err)
		return err
	}
	return nil
}

func onSyncSceneName(player iface.IPlayer, buf []byte) {
	msg := &pb3.SyncSceneName{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	if et, ok := player.(*Player); ok {
		et.sceneName = msg.Name
	}
}

func onFightSrvGiveAwards(buf []byte) {
	msg := &pb3.FGiveAwards{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	if !gshare.IsActorInThisServer(msg.ActorId) {
		return
	}

	awards := jsondata.Pb3RewardVecToStdRewardVec(msg.Awards)

	actor := manager.GetPlayerPtrById(msg.ActorId)

	if nil == actor {
		logger.LogStack("actor not found")
		return
	}

	engine.GiveRewards(actor, awards, common.EngineGiveRewardParam{LogId: pb3.LogId(msg.LogId), NoTips: msg.NoTips})

	if msg.SendPop {
		rewards := engine.FilterRewardByPlayer(actor, awards)
		actor.SendShowRewardsPop(rewards)
	}
}

func onFightSrvSendMail(buf []byte) {
	msg := &pb3.FSendMail{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	if !gshare.IsActorInThisServer(msg.ActorId) {
		return
	}

	awards := jsondata.Pb3RewardVecToStdRewardVec(msg.Awards)
	mailmgr.SendMailToActor(msg.ActorId, &mailargs.SendMailSt{
		ConfId:  uint16(msg.ConfId),
		Content: msg.ArgString,
		Rewards: awards,
	})
}

func onOfflineAddMoney(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onOfflineAddMoney %v", err)
		return
	}

	actorId, mType, count, btip, logId := msg.U64Param, msg.U32Param, msg.U32Param2, msg.BParam, msg.U32Param3
	gshare.AddMoneyOffline(actorId, mType, count, btip, logId)
}

func onSendProtoBuffer(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onSendProtoBuffer %v", err)
		return
	}

	actorId, protoH, protoL, buffer := msg.U64Param, msg.U32Param, msg.U32Param2, msg.Buf
	broadcast := msg.BParam
	if broadcast {
		engine.BroadcastBuf(chatdef.CIWorld, 0, uint16(protoH), uint16(protoL), buffer, 0)
	} else {
		if actor := manager.GetPlayerPtrById(actorId); nil != actor {
			actor.SendProtoBuffer(uint16(protoH), uint16(protoL), buffer)
		}
	}
}

func onF2GActorFightValueChange(buf []byte) {
	msg := &pb3.LogFightValueChange{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("onF2GActorFightValueChange %v", err)
		return
	}

	player := manager.GetPlayerPtrById(msg.ActorId)
	if player != nil {
		logworker.LogFightChange(player, msg)
	}
}

// 前端心跳
func c2sHeart(actor iface.IPlayer, msg *base.Message) error {
	now := time_util.Now().UnixMilli()

	resp := pb3.NewS2C_2_254()
	defer pb3.RealeaseS2C_2_254(resp)
	resp.TimeStamp = now
	actor.SendProto3(2, 254, resp)
	return nil
}

func offlineLearnSkill(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	skillId := st.U32Param
	lv := st.U32Param2

	player.LearnSkill(skillId, lv, false)
}

func deleteItemByHand(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	itemHandle := st.U64Param
	itemId := st.U32Param
	if itemHandle > 0 {
		player.DelItemByHand(itemHandle, pb3.LogId_LogGm)
	}
	if itemId > 0 {
		player.DeleteItemById(itemId, player.GetItemCount(itemId, -1), pb3.LogId_LogGm)
	}
}

func offlineAddMoney(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	moneyType, value, btips, logId := st.U32Param, st.U32Param2, st.BParam, st.U32Param3
	player.AddMoney(moneyType, int64(value), btips, pb3.LogId(logId))
}

func addMoneyOffline(actorId uint64, moneyType, value uint32, bTip bool, logId uint32) {
	msg := &pb3.CommonSt{
		U32Param:  moneyType,
		U32Param2: value,
		U32Param3: logId,
		BParam:    bTip,
	}
	engine.SendPlayerMessage(actorId, gshare.OfflineAddMoney, msg)
}

func onActor2PlayerHeart(iPlayer iface.IPlayer, buf []byte) {
	var req pb3.ActorHeart
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTriggerPlayerEvent error:%v", err)
		return
	}

	player, ok := iPlayer.(*Player)
	if !ok {
		return
	}

	if player.FightHandle == req.Handle {
		player.FightHeart = time_util.NowSec() + FightTimeOut
	} else {
		// 通知战斗服没有该actor了
		engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFNoThatActor, &pb3.SyncExitFight{ActorId: player.ActorId, Handle: req.Handle})
	}
}

func onActor2PlayerSyncFbId(iPlayer iface.IPlayer, buf []byte) {
	var req pb3.SyncFbId
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTriggerPlayerEvent error:%v", err)
		return
	}

	player, ok := iPlayer.(*Player)
	if !ok {
		return
	}
	player.onActorSyncFbId(req.FbId, req.SceneId)
}

func onActor2PlayerSyncLocation(iPlayer iface.IPlayer, buf []byte) {
	var req pb3.SyncLocation
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTriggerPlayerEvent error:%v", err)
		return
	}

	player, ok := iPlayer.(*Player)
	if !ok {
		return
	}

	if req.IsLocal {
		player.onActorSyncLocation(req.SceneId, int32(req.PosX), int32(req.PosY))
	}

	player.onActorSyncCurrentPos(req.SceneId, int32(req.PosX), int32(req.PosY))
}

func onActor2PlayerResCheckUseItem(iPlayer iface.IPlayer, buf []byte) {
	var req pb3.RetCheckItemUse
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("onTriggerPlayerEvent error:%v", err)
		return
	}

	player, ok := iPlayer.(*Player)
	if !ok {
		return
	}
	player.onActorUseItems(req.ItemType, req.ItemId, req.CanUse)
}

func onActorF2GChannelChat(player iface.IPlayer, buf []byte) {
	var req pb3.C2S_5_1
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("onActorF2GChannelChat error:%v", err)
		return
	}

	player.ChannelChat(&req, false)
}

const (
	ClientSetU32Size = 200
	ClientSetU64Size = 100
)

// 客户端系统设置
func c2sSet(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_4

	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	playerData := player.GetBinaryData()
	if nil == playerData {
		return neterror.InternalError("playerBinaryData is nil")
	}

	initSetData(player)

	oldBits := player.GetBinaryData().Settings.U64S[0]
	var newBits *uint64

	result := pb3.S2C_2_4{}
	for _, v := range req.U32S {
		if v.Key >= uint32(len(player.GetBinaryData().Settings.U32S)-1) {
			logger.LogWarn("c2sSet u32s key:%d is out of range", v.Key)
			continue
		}

		playerData.Settings.U32S[v.Key] = v.Value
		result.U32S = append(result.U32S, v)
	}

	for _, v := range req.U64S {
		if v.Key >= uint32(len(player.GetBinaryData().Settings.U32S)-1) {
			logger.LogWarn("c2sSet u64s key:%d is out of range", v.Key)
			continue
		}

		// 第一个值是位标记
		if v.Key == 0 {
			newBits = &v.Value
		}

		playerData.Settings.U64S[v.Key] = v.Value
		result.U64S = append(result.U64S, v)
	}

	if newBits != nil {
		manager.TriggerSettingChange(player, oldBits, *newBits)
	}

	player.SendProto3(2, 4, &result)

	return nil
}

func initSetData(player iface.IPlayer) {
	if player.GetBinaryData().Settings == nil {
		player.GetBinaryData().Settings = &pb3.Settings{
			U32S: make([]uint32, ClientSetU32Size),
			U64S: make([]uint64, ClientSetU64Size),
		}
	}

	if player.GetBinaryData().Settings.U32S == nil || len(player.GetBinaryData().Settings.U32S) == 0 {
		player.GetBinaryData().Settings.U32S = make([]uint32, ClientSetU32Size)
	}

	if player.GetBinaryData().Settings.U64S == nil || len(player.GetBinaryData().Settings.U64S) == 0 {
		player.GetBinaryData().Settings.U64S = make([]uint64, ClientSetU64Size)
	}
}

func c2sLeaveGame(player iface.IPlayer, msg *base.Message) error {
	player.ClosePlayer(cmd.DCPlayerKick)
	return nil
}

func c2sReportSdkEvent(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_110
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}
	info := engine.Get360WanInfo(player.GetExtraAttrU32(attrdef.DitchId))
	if info != nil {
		val := &argsdef.RepostTo360WanLogin{
			RepostTo360WanBase: argsdef.RepostTo360WanBase{
				Gid:        info.Gid,
				Sid:        fmt.Sprintf("S%d", engine.GetServerId()),
				OldSid:     fmt.Sprintf("S%d", player.GetServerId()),
				User:       engine.Get360WanUserId(player.GetAccountName()),
				RoleId:     fmt.Sprintf("%d", player.GetId()),
				Dept:       info.Dept,
				Time:       int64(time_util.NowSec()),
				Gname:      info.Gkey,
				DitchId:    player.GetExtraAttrU32(attrdef.DitchId),
				SubDitchId: player.GetExtraAttrU32(attrdef.SubDitchId),
			},
			Lv:    player.GetLevel(),
			Ip:    player.GetRemoteAddr(),
			MapId: "0",
		}
		gshare.SendSDkMsg(custom_id.GMsgSdkReport, req.ReportEventType, &pb3.SimpleReportPlayer{
			Account:    player.GetAccountName(),
			DitchId:    player.GetExtraAttrU32(attrdef.DitchId),
			SubDitchId: player.GetExtraAttrU32(attrdef.SubDitchId),
			LoginTime:  player.GetLoginTime(),
			Id:         player.GetId(),
		}, val)
	} else {
		gshare.SendSDkMsg(custom_id.GMsgSdkReport, req.ReportEventType, &pb3.SimpleReportPlayer{
			Account:    player.GetAccountName(),
			DitchId:    player.GetExtraAttrU32(attrdef.DitchId),
			SubDitchId: player.GetExtraAttrU32(attrdef.SubDitchId),
			LoginTime:  player.GetLoginTime(),
			Id:         player.GetId(),
		})
	}
	return nil
}

func c2sLogEnterSceneTime(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_2_120
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}
	if player.GetLevel() >= 150 {
		return nil
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogPlayerEnterSceneTime, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.SceneId),
		StrArgs: utils.I32toa(req.Dur),
	})
	return nil
}

func c2sEnterCross(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_128_13
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	err = player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterFbHdl, &pb3.EnterFubenHdl{
		SceneId: jsondata.GlobalUint("crossMainSceneId"),
	})
	if err != nil {
		logger.LogError("enter small cross err: %v", err)
		player.SendProto3(128, 13, &pb3.S2C_128_13{Ret: false})
		return err
	}
	player.SendProto3(128, 13, &pb3.S2C_128_13{Ret: true})
	return nil
}
func c2sEnterDefaultFbScene(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_128_50
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.ParamsInvalidError("unmarshal msg failed %s", err)
	}

	return enterDefaultFbScene(player, &pb3.EnterFubenHdl{
		SceneId: req.SceneId,
		PosX:    req.X,
		PosY:    req.Y,
		Param:   req.Param,
	})
}
func handlePlayerSeServerInit(args ...interface{}) {
	gshare.RegisterGameMsgHandler(custom_id.GMsgFightSrvClientMsg, OnFightSrvClientMsg)
	gshare.RegisterGameMsgHandler(custom_id.GMsgReloadActorFinish, onReloadActorFinish)
	gshare.RegisterGameMsgHandler(custom_id.GMsgNewChargeOrder, OnNewChargeOrderMsg)
}
func handlePlayerAeLogin(actor iface.IPlayer, args ...interface{}) {
	gshare.SendDBMsg(custom_id.GMsgUpdateActorLogin, actor.GetId())
}
func handlePlayerAeChangeSex(actor iface.IPlayer, args ...interface{}) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CActorChangeInfo, nil)
}
func handlePlayerAeLogout(actor iface.IPlayer, args ...interface{}) {
	gshare.SendDBMsg(custom_id.GMsgUpdateActorLogout, actor.GetId())
}
func handlePlayerQttFightValue(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	return actor.GetExtraAttrU32(attrdef.FightValue)
}
func handlePlayerQttOpenSrvDay(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	return gshare.GetOpenServerDay()
}
func handleQttMonsterQualityCount(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	var count uint32
	if len(ids) == 0 {
		return count
	}
	subType := ids[0]
	quality := ids[1]
	p := actor.(*Player)
	data := p.GetKillMonsterRecordData(subType)
	if data.QualityCountMap == nil {
		return 0
	}
	for q, c := range data.QualityCountMap {
		if quality > q {
			continue
		}
		count += c
	}
	return count
}
func handlePlayerAeKillOtherActor(actor iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	_, ok := args[0].(uint64)
	if !ok {
		return
	}
	sceneId, ok := args[1].(uint32)
	if !ok {
		return
	}
	camp, ok := args[2].(uint32)
	if !ok {
		return
	}
	actor.TriggerQuestEvent(custom_id.QttKillPlayer, 0, 1)
	actorCamp1 := actor.GetSmallCrossCamp()
	actorCamp2 := crosscamp.CampType(camp)
	if actorCamp1 != 0 && actorCamp2 != 0 && actorCamp1 != actorCamp2 {
		actor.TriggerQuestEvent(custom_id.QttKillCampActorTimes, 0, 1)
		actor.TriggerQuestEvent(custom_id.QttKillCampActorTimesByScene, sceneId, 1)
	}
}

func handlePlayerAeKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}

	count, ok := args[2].(uint32)
	if !ok {
		return
	}

	mConf := jsondata.GetMonsterConf(monsterId)
	if mConf == nil {
		return
	}

	p := player.(*Player)
	recordData := p.GetKillMonsterRecordData(mConf.SubType)
	if recordData.QualityCountMap == nil {
		recordData.QualityCountMap = make(map[uint32]uint32)
	}
	recordData.QualityCountMap[mConf.Quality] += count
	player.TriggerQuestEventRange(custom_id.QttMonsterQualityCount)
}

func init() {
	net.RegisterProto(2, 254, c2sHeart)
	net.RegisterProto(2, 4, c2sSet)
	net.RegisterProto(2, 80, c2sLeaveGame)
	net.RegisterProto(2, 110, c2sReportSdkEvent)
	net.RegisterProto(2, 120, c2sLogEnterSceneTime)
	net.RegisterProto(128, 13, c2sEnterCross)
	net.RegisterProto(128, 50, c2sEnterDefaultFbScene)
	event.RegSysEvent(custom_id.SeServerInit, handlePlayerSeServerInit)
	engine.RegisterActorCallFunc(playerfuncid.EnterFightSucc, onEnterFightSucc)
	engine.RegisterActorCallFunc(playerfuncid.SaveDataFinish, onSaveDataFinish)
	engine.RegisterActorCallFunc(playerfuncid.SaveDataFiveMin, onSaveDataFiveMin)
	engine.RegisterActorCallFunc(playerfuncid.ReturnToLocalFight, onReturnToLocalFight)
	engine.RegisterActorCallFunc(playerfuncid.ReturnToLastScene, onReturnToLastScene)
	engine.RegisterActorCallFunc(playerfuncid.ChangeFightSrvScene, onChangeFightSrvScene)
	engine.RegisterActorCallFunc(playerfuncid.SyncSceneName, onSyncSceneName)
	engine.RegisterActorCallFunc(playerfuncid.UnlockOperate, onUnlockOperate)
	engine.RegisterActorCallFunc(playerfuncid.ChangeFightSrv, onChangeFightSrv)
	engine.RegisterActorCallFunc(playerfuncid.LogFightSrvActorBehavior, onLogFightSrvActorBehavior)
	//remotecall.RegisterGameCall(todoid.FightSrvClientMsg, OnFightSrvClientMsg)
	//remotecall.RegisterGameCall(todoid.ToPlayerData, OnFightToPlayerData)
	engine.RegisterSysCall(sysfuncid.FGGiveAwards, onFightSrvGiveAwards)
	engine.RegisterSysCall(sysfuncid.FGSendMail, onFightSrvSendMail)
	engine.RegisterSysCall(sysfuncid.F2GAddMoney, onOfflineAddMoney)
	engine.RegisterSysCall(sysfuncid.C2GSendProtoBuffer, onSendProtoBuffer)
	engine.RegisterSysCall(sysfuncid.F2GActorFightValueChange, onF2GActorFightValueChange)
	event.RegActorEvent(custom_id.AeLogin, handlePlayerAeLogin)
	event.RegActorEvent(custom_id.AeChangeSex, handlePlayerAeChangeSex)
	event.RegActorEvent(custom_id.AeLogout, handlePlayerAeLogout)
	event.RegActorEvent(custom_id.AeKillMon, handlePlayerAeKillMon)
	engine.RegQuestTargetProgress(custom_id.QttFightValue, handlePlayerQttFightValue)
	engine.RegQuestTargetProgress(custom_id.QttOpenSrvDay, handlePlayerQttOpenSrvDay)
	engine.RegQuestTargetProgress(custom_id.QttMonsterQualityCount, handleQttMonsterQualityCount)
	event.RegActorEvent(custom_id.AeKillOtherActor, handlePlayerAeKillOtherActor)
	engine.RegisterMessage(gshare.OfflineLearnSkill, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineLearnSkill)
	engine.RegisterMessage(gshare.OfflineDeleteItemByHand, func() pb3.Message {
		return &pb3.CommonSt{}
	}, deleteItemByHand)

	engine.RegisterMessage(gshare.OfflineAddMoney, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineAddMoney)
	engine.RegisterActorCallFunc(playerfuncid.Actor2PlayerHeart, onActor2PlayerHeart)
	engine.RegisterActorCallFunc(playerfuncid.Actor2PlayerSyncFbId, onActor2PlayerSyncFbId)
	engine.RegisterActorCallFunc(playerfuncid.Actor2PlayerSyncLocation, onActor2PlayerSyncLocation)
	engine.RegisterActorCallFunc(playerfuncid.ResCheckUseItem, onActor2PlayerResCheckUseItem)
	engine.RegisterActorCallFunc(playerfuncid.F2GChannelChat, onActorF2GChannelChat)
	gshare.AddMoneyOffline = addMoneyOffline

	initPlayerGm()
}

func initPlayerGm() {
	gmevent.Register("cross", func(player iface.IPlayer, args ...string) bool {
		req := &pb3.EnterFubenHdl{}
		req.FbHdl = 0
		req.SceneId = jsondata.GlobalUint("crossMainSceneId")
		req.PosX = 0
		req.PosY = 0
		player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterFbHdl, req)
		return true
	}, 1)
	gmevent.Register("playernewday", func(player iface.IPlayer, args ...string) bool {
		player.TriggerEvent(custom_id.AeNewDay)
		return true
	}, 1)
	gmevent.Register("enterDefaultFbScene", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		enterDefaultFbScene(player, &pb3.EnterFubenHdl{
			SceneId: utils.AtoUint32(args[0]),
		})
		return true
	}, 1)
}
