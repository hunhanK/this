package iface

import (
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"time"

	"github.com/gzjjyz/logger"
)

type IPlayerEvent interface {
	TriggerEvent(id int, args ...interface{})
	TriggerQuestEvent(targetType, targetId uint32, count int64)
	TriggerQuestEventRangeTimes(targetType, targetId uint32, count int64)
	TriggerQuestEventRange(targetType uint32, args ...interface{})
	TriggerQuestEventByRange(qtt, tVal, preVal, qtype uint32)
}

type IPlayerAttr interface {
	GetAttrSys() IAttrSys
	SetExtraAttr(attrType attrdef.AttrTypeAlias, attrValue attrdef.AttrValueAlias)
	GetExtraAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias
	GetExtraAttrU32(attrType attrdef.AttrTypeAlias) uint32
	GetFightAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias
	SetFightAttr(attrType attrdef.AttrTypeAlias, attrValue attrdef.AttrValueAlias)
	SetGmAttr(attrType uint32, attrVal int64)
	GetGmAttr() map[uint32]int64
}

type IPlayerMoney interface {
	GetMoneyCount(mt uint32) int64
	AddMoney(mt uint32, count int64, btip bool, logId pb3.LogId) bool
	DeductMoney(mt uint32, count int64, params common.ConsumeParams) bool
	CopyMoneys() map[uint32]int64
	AddAmount(Diamond uint32, cash uint32, logId pb3.LogId, chargeId uint32)
}

type IPlayerData interface {
	GetMainData() *pb3.PlayerMainData
	GetBinaryData() *pb3.PlayerBinaryData
	GetPlayerData() *pb3.PlayerData
	Save(logout bool)
	SaveToObjVersion(version uint32)
	ToPlayerDataBase() *pb3.PlayerDataBase
	PackCreateData() *pb3.CreateActorData
}

type IAppearOwner interface {
	TakeOnAppear(pos uint32, appear *pb3.SysAppearSt, isTip bool)
	TakeOffAppear(pos uint32)
	TakeAppearChange(pos uint32, oldId uint64)
	TakeOffAppearById(pos uint32, appear *pb3.SysAppearSt)

	GetHeadFrame() uint32   // 头像框
	GetBubbleFrame() uint32 // 气泡框
	GetHead() uint32        // 头像
}

type IPlayerConsume interface {
	GetConsumer() IConsumer
	ConsumeRate(consumes jsondata.ConsumeVec, count int64, autoBuy bool, params common.ConsumeParams) bool
	ConsumeByConf(consumes jsondata.ConsumeVec, autoBuy bool, params common.ConsumeParams) bool
	CheckConsumeByConf(consumes jsondata.ConsumeVec, autoBuy bool, logId pb3.LogId) bool
	ConsumeByConfWithRet(consumes jsondata.ConsumeVec, autoBuy bool, params common.ConsumeParams) (bool, argsdef.RemoveMaps)
}

type ISysMgrOwner interface {
	GetSysMgr() ISystemMgr
	GetSysObj(sysId uint32) ISystem
	GetSysOpen(sysId uint32) bool
	IsOpenNotified(sysId uint32) bool
	SetSysStatus(sysId uint32, isOpen bool)
	GetSysStatus(sysId uint32) bool
	GetSysStatusData() map[uint32]uint32
}

type ISkillSysOwner interface {
	LearnSkill(id, level uint32, send bool) bool
	GetSkillLv(id uint32) uint32
	ForgetSkill(id uint32, recalcAttr, send, syncFight bool)
	GetSkillInfo(id uint32) *pb3.SkillInfo
}

type IPlayerBag interface {
	IPlayerBagAvailableCount
	IPlayerBagOptByHandle
	IPlayerGetBagSys

	// 通用方法

	Split(handle uint64, count int64, logId pb3.LogId) uint64
	SortBag()
	CanAddItem(param *itemdef.ItemParamSt, overlay bool) bool
	SendBagFullTip(itemId uint32)
	AddItem(st *itemdef.ItemParamSt) bool
	AddItemPtr(item *pb3.ItemSt, bTip bool, logId pb3.LogId) bool
	DeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId) bool
	DeleteItemById(itemId uint32, count int64, logId pb3.LogId) bool
	GetAddItemNeedGridCount(param *itemdef.ItemParamSt, overlay bool) int64
	CheckItemCond(conf *jsondata.ItemConf) bool
	OnDeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId)
}

type IPlayerGetBagSys interface {
	GetBagSysByBagType(bagType uint32) IBagSys
}

type IPlayerBagAvailableCount interface {
	GetBagAvailableCount() uint32
	GetFairyBagAvailableCount() uint32
	GetGodBeastBagAvailableCount() uint32
	GetFairyEquipBagAvailableCount() uint32
	GetBattleSoulGodEquipAvailableCount() uint32
	GetMementoBagAvailableCount() uint32
	GetFairySwordBagAvailableCount() uint32
	GetFairySpiritBagAvailableCount() uint32
	GetHolyBagAvailableCount() uint32
	GetFlyingSwordEquipBagAvailableCount() uint32
	GetSourceSoulBagAvailableCount() uint32
	GetBloodBagAvailableCount() uint32
	GetBloodEquBagAvailableCount() uint32
	GetSmithBagAvailableCount() uint32
	GetFeatherEquBagAvailableCount() uint32
	GetBagAvailableCountByBagType(bagType uint32) uint32
}

type IPlayerBagOptByHandle interface {
	GetBelongBagSysByHandle(hdl uint64) ISystem

	GetItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetFairyByHandle(hdl uint64) *pb3.ItemSt
	RemoveFairyByHandle(hdl uint64, logId pb3.LogId) bool

	GetGodBeastItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveGodBeastItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetFairyEquipItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveFairyEquipItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetFairySwordItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveFairySwordItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetFairySpiritItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveFairySpiritItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetHolyEquipItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveHolyItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetFlyingSwordEquipItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveFlyingSwordEquipItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetSourceSoulEquipItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveSourceSoulEquipItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetBloodItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveBloodItemByHandle(hdl uint64, logId pb3.LogId) bool
	RemoveBloodEquItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetDomainEyeRuneItemByHandle(hdl uint64) *pb3.ItemSt
	RemoveDomainEyeRuneItemByHandle(hdl uint64, logId pb3.LogId) bool

	GetItemByHandleWithBagType(bagType uint32, hdl uint64) *pb3.ItemSt
	RemoveItemByHandleWithBagType(bagType uint32, hdl uint64, logId pb3.LogId) bool
}

type IGuildMember interface {
	GetGuildId() uint64
	GetGuildName() string
	SetGuildId(guildId uint64)
	SetQuitGuildCd(exitTime uint32)
	IsGuildLeader() bool
}

type IPlayerNet interface {
	DoNetMsg(protoIdH, protoIdL uint16, message *base.Message)
	OnRecvMessage(msgType int, msg pb3.Message)
	SendProto3(sysId, cmdId uint16, message pb3.Message)
	SendProtoBuffer(sysId, cmdId uint16, buffer []byte)
}

type IShowStr interface {
	PackageShowStr() map[uint32]string
	SyncShowStr(attr uint32)
}

type ITrialActiveSys interface {
	IsInTrialActive(activeType uint32, args []uint32) bool
	StopTrialActive(activeType uint32, args []uint32)
}

type IPlayer interface {
	IEntity
	logger.ILogger
	IPlayerEvent
	IPlayerAttr
	IPlayerMoney
	IPlayerData
	IAppearOwner
	ISysMgrOwner
	ISkillSysOwner
	IPlayerNet
	IGuildMember
	IPlayerBag
	IPlayerConsume
	IShowStr
	ITrialActiveSys
	logger.IRequester

	SetLost(status bool)
	GetLost() bool

	EnterFightSrv(fightType base.ServerType, todoId uint32, todoMsg pb3.Message, args ...interface{}) error
	DoEnterFightSrv(fightType base.ServerType, todoId uint32, todoMsg pb3.Message) error

	GetActorProxy() IActorProxy

	CallActorFunc(fnId uint16, msg pb3.Message) error
	CallActorSysFn(typ base.ServerType, fnId uint16, msg pb3.Message) error
	CallActorSmallCrossFunc(fnId uint16, msg pb3.Message) error
	CallActorMediumCrossFunc(fnId uint16, msg pb3.Message) error

	UpdateStatics(key string, value interface{})

	GetFbId() uint32
	GetSceneId() uint32

	Reconnect(key string, gateId, connId uint32, remoteAddr string) bool
	ReLogin(reconnect bool, gateId, connId uint32, remoteAddr string)
	EnterGame()
	IsEnterGameFinish() bool
	ClosePlayer(reason uint16)
	GetServerId() uint32
	GetPfId() uint32
	GetRemoteAddr() string
	GetAccountName() string
	GetUserId() uint32
	GetGateInfo() (gateId, connId uint32)
	CheckNewDay(zeroReset bool) bool
	NewHourArrive()
	NewWeekArrive()
	NewDayArrive()

	GetLevel() uint32
	GetVipLevel() uint32
	GetSpiritLv() uint32
	GetCreateTime() uint32
	GetCreateDay() uint32
	GetLoginTime() uint32
	GetLogoutDay() uint32
	GetJob() uint32
	SetJob(job uint32)
	GetSex() uint32
	SetSex(sex uint32)
	GetCircle() uint32
	IsFlyUpToWorld() bool
	GetFlyCamp() uint32

	GetTeamId() uint64
	SetTeamId(teamId uint64)

	AddExp(exp int64, logId pb3.LogId, withWorldAddRate bool, addRates ...uint32) (finalExpAdded int64)
	AddExpV2(exp int64, logId pb3.LogId, addRateByTip uint32) (finalExpAdded int64)
	GetExp() int64

	SendActorData()
	SendTipMsg(tipMsgId uint32, params ...interface{})
	SendFbResult(settle *pb3.FbSettlement)

	SendMail(mail *mailargs.SendMailSt)

	ReturnToStaticScene()

	Charge(Diamond int64, logId pb3.LogId)
	SendServerInfo()
	SendYYInfo(msg *pb3.S2C_127_255)

	SetName(name string)

	SetTimeout(duration time.Duration, cb func()) *time_util.Timer
	SetInterval(duration time.Duration, cb func(), ops ...time_util.TimerOption) *time_util.Timer

	TaskRecord(tp, id uint32, count int64, isAdd bool)
	GetTaskRecord(tp uint32, ids []uint32) uint32

	GetItemCount(itemId uint32, bind int8) int64

	InLocalFightSrv() bool

	GetGmLevel() uint32

	AddBuff(BuffId uint32)
	DelBuff(BuffId uint32)

	AddBuffByTime(buffId uint32, dur int64)

	GetLoginTrace() bool
	SetLoginTrace(bool)
	EnterMainScene()
	SendChargeInfo()

	IsOperateLock(bit uint32) bool
	LockOperate(bit, timeout uint32)
	UnlockOperate(bit uint32)
	FullHp()
	DelItemByHand(hand uint64, logId pb3.LogId)

	GetPrivilege(pType privilegedef.PrivilegeType) (total int64, err error)
	HasPrivilege(pType privilegedef.PrivilegeType) bool

	GetDayOnlineTime() uint32
	GetSmallCrossCamp() crosscamp.CampType
	GetMediumCrossCamp() uint64

	HasState(bit uint32) bool

	SetLastFightTime(timeStamp uint32)
	GetFightStatus() bool

	GetPYYObjList(yyDefine uint32) []IPlayerYY
	InDartCar() bool

	DoFirstExperience(typ pb3.ExperienceType, unExperienceFunc func() error) error // 首次体验该类型功能, 如果不是首次则执行 unExperienceFunc
	InitiativeEndExperience(typ pb3.ExperienceType, commonSt *pb3.CommonSt) error  // 主动结束体验

	CheckItemBind(itemId uint32, isBind bool) bool

	ResetSpecCycleBuy(typ uint32, subType uint32)
	ChannelChat(req *pb3.C2S_5_1, checkCd bool)
	SetRankValue(rankType uint32, value int64)
	GetRankValue(rankType uint32) int64
	SetYYRankValue(rankType uint32, value int64)
	GetYYRankValue(rankType uint32) int64

	CheckEnterFb(todo uint32) bool
	IsInPrison() bool
	ChangeCastDragonEquip()
	IsExistFriend(targetId uint64) bool

	GetPYYObj(actId uint32) IPlayerYY
	GetYYObj(actId uint32) IYunYing

	GetNirvanaLevel() uint32
	GetNirvanaSubLevel() uint32

	GetCurrentPos() (int32, int32)

	FinishBossQuickAttack(monId, sceneId, fbId uint32)

	GetRankDragonEqData() *pb3.RankDragonEqData
	GetRankFairyData() *pb3.RankFairyData
	GetRankSoulHaloData() *pb3.RankSoulHaloData
	GetRankFairyCWData() *pb3.RankFairyColdWeaponData
	GetRankKDragonEqData() *pb3.RankKillDragonEqData
	BuildChatBaseData(target IPlayer) *wm.CommonData
	CheckBossSpirit(itemId uint32) bool

	CheckBagCount(todoId uint32) bool

	BroadcastCustomTipMsgById(tipMsgId uint32, params ...interface{})

	SendShowRewardsPop(rewards jsondata.StdRewardVec)
	SendShowRewardsPopByPYY(rewards jsondata.StdRewardVec, id uint32)
	SendShowRewardsPopBySys(rewards jsondata.StdRewardVec, id uint32)
	SendShowRewardsPopByYY(rewards jsondata.StdRewardVec, id uint32)

	GetDitchTokens() uint32
	GetHistoryDitchTokens() uint32
	AddDitchTokens(tokens uint32)
	SubDitchTokens(tokens uint32)

	MergeTimesChallengeBoss(fightType base.ServerType, fbTodoId uint32, mergeTimes uint32) error

	GetDrawLibConf(lib uint32) *jsondata.LotteryLibConf

	OpenPyy(id, sTime, eTime, confIdx, timeType uint32, isInit bool)

	AddVipExp(exp uint32)
	CheckMarryRewardTimes(gradeConf *jsondata.MarryGradeConf) bool
	AddMarryRewardTimes(gradeConf *jsondata.MarryGradeConf) bool

	GetDailyChargeMoney(timestamp uint32, isReal bool) uint32
	GetDailyCharge(isReal bool) uint32
}
