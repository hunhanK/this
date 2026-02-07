package actorsystem

import (
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type CreateSysFuncHandle func() iface.ISystem

type Mgr struct {
	owner  iface.IPlayer
	objMap [sysdef.SYSDEF_MAX + 1]iface.ISystem
}

func NewSystemMgr(owner iface.IPlayer) *Mgr {
	mgr := &Mgr{owner: owner}
	return mgr
}

// 获取系统实例
func (mgr *Mgr) GetSysObj(sysId uint32) iface.ISystem {
	return mgr.objMap[sysId]
}

// 检查系统开启
func (mgr *Mgr) canOpenSys(id uint32, _ iface.ISystem) bool {
	// 默认开启
	if sysdef.IsSysDefOpen(id) {
		return true
	}

	// 背包默认开启 不然没开功能但是投放道具 会有问题
	if gshare.CheckSysBelongBagType(id) {
		return true
	}

	conf := jsondata.GetSysOpenConf(id)
	if nil == conf {
		return false
	}

	//开服天数
	if conf.OpenSrvDay > 0 && conf.OpenSrvDay > gshare.GetOpenServerDay() {
		return false
	}

	//合服次数
	if conf.MergeTimes > 0 && conf.MergeTimes > gshare.GetMergeTimes() {
		return false
	}

	//跨服次数
	if conf.CrossTimes > 0 {
		if conf.CrossTimes > gshare.GetCrossAllocTimes() {
			return false
		}

		if conf.CrossTimes == gshare.GetCrossAllocTimes() {
			//跨服天数
			if conf.CrossDay > 0 && conf.CrossDay > gshare.GetSmallCrossDay() {
				return false
			}
		}
	}

	//合服天数
	if conf.MergeDay > 0 && conf.MergeDay > gshare.GetMergeSrvDay() {
		return false
	}

	// 跨服匹配服务器数量
	if conf.CrossMatchSrvNum > 0 && conf.CrossMatchSrvNum > gshare.GetMatchSrvNum() {
		return false
	}

	// 进入中跨服进程
	if conf.EnterMediumCross && gshare.GetStaticVar().FirstMediumCrossTimestamp == 0 {
		return false
	}

	actor := mgr.owner
	level := actor.GetLevel()
	cLevel := actor.GetCircle()

	// 玩家等级
	if level < conf.Level {
		return false
	}

	// 新境界等级
	if cLevel < conf.Circle {
		return false
	}

	// 飞升上界
	if conf.FlyUpToWorld && !actor.IsFlyUpToWorld() {
		return false
	}

	//主线任务
	if conf.MainTaskId > 0 {
		questSys, ok := mgr.GetSysObj(sysdef.SiQuest).(*QuestSys)
		if !ok || !questSys.IsFinishMainQuest(conf.MainTaskId) {
			return false
		}
	}

	//功能试炼
	if conf.TrialId > 0 {
		trialSys, ok := mgr.GetSysObj(sysdef.SiFuncTrial).(*FuncTrialSys)
		if !ok || !trialSys.CheckFuncTrial(conf.TrialId) {
			return false
		}
	}

	//贵族等级
	if conf.VipLevel > 0 {
		vipSys, ok := mgr.GetSysObj(sysdef.SiVip).(*VipSys)
		if !ok || vipSys.GetBinaryData().GetVip() < conf.VipLevel {
			return false
		}
	}

	// 飞升转职需求
	if conf.IsFlyCamp && actor.GetFlyCamp() == 0 {
		return false
	}

	// 涅槃转职
	if conf.NirvanaLv != 0 && actor.GetNirvanaLevel() < conf.NirvanaLv {
		return false
	}

	// 功能礼包购买
	binData := actor.GetBinaryData()
	if conf.GiftId != 0 && !utils.SliceContainsUint32(binData.FuncOpenGifts, conf.GiftId) {
		return false
	}

	// 充值金额
	if conf.ChargeCent > 0 {
		chargeInfo := binData.GetRealChargeInfo()
		if chargeInfo == nil || chargeInfo.TotalChargeMoney < conf.ChargeCent {
			return false
		}
	}
	return true
}

// 判断系统开启
func (mgr *Mgr) check() {
	owner := mgr.owner
	bit := owner.GetExtraAttr(attrdef.TeamFbSysOpenFlag)
	for _, obj := range mgr.objMap {
		if nil == obj {
			continue
		}
		sysId := obj.GetSysId()
		flag := mgr.canOpenSys(sysId, obj)
		if obj.IsOpen() && !flag {
			obj.Close()
			obj.OnClose()
			owner.LogInfo("%s close sys:%d", owner.GetName(), sysId)
		}
		if !obj.IsOpen() && flag {
			obj.Open()
			obj.OnOpen()
			bit = gshare.SetTeamFbSysOpenFlag(bit, sysId)
			owner.TriggerEvent(custom_id.AeSysOpen, sysId)
			owner.LogInfo("%s open sys:%d", owner.GetName(), sysId)
		}
		if fOpenSys := mgr.GetSysObj(sysdef.SiFuncOpen); nil != fOpenSys && fOpenSys.IsOpen() {
			if !owner.IsOpenNotified(sysId) && flag {
				owner.TriggerEvent(custom_id.AeFuncOpenNotify, sysId)
			}
		}
	}
	owner.SetExtraAttr(attrdef.TeamFbSysOpenFlag, bit)
}

// 玩家登陆
func (mgr *Mgr) OnLogin() {
	owner := mgr.owner
	bit := owner.GetExtraAttr(attrdef.TeamFbSysOpenFlag)
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			bit = gshare.SetTeamFbSysOpenFlag(bit, obj.GetSysId())
			utils.ProtectRun(obj.DataFix)
			utils.ProtectRun(obj.OnLogin)
		}
	}
	// 登录时触发重算属性
	for sysId := attrdef.SysBegin; sysId < attrdef.SysEnd; sysId++ {
		if sysId == 0 {
			continue
		}
		cb := engine.GetAttrCalcFn(uint32(sysId))
		if nil == cb {
			continue
		}
		owner.GetAttrSys().ResetSysAttr(uint32(sysId))
	}
	owner.SetExtraAttr(attrdef.TeamFbSysOpenFlag, bit)
}

func (mgr *Mgr) OnAfterLogin() {
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			utils.ProtectRun(obj.OnAfterLogin)
		}
	}
}

func (mgr *Mgr) OnLogout() {
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			utils.ProtectRun(obj.OnLogout)
		}
	}
}

// OnReconnect 玩家重连
func (mgr *Mgr) OnReconnect() {
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			utils.ProtectRun(obj.OnReconnect)
		}
	}
}

func (mgr *Mgr) OnLoginFight() {
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			utils.ProtectRun(obj.OnLoginFight)
		}
	}
}

func (mgr *Mgr) Destroy() {
	for _, obj := range mgr.objMap {
		if nil != obj {
			utils.ProtectRun(obj.OnDestroy)
		}
	}
}

func (mgr *Mgr) Save() {
	for _, obj := range mgr.objMap {
		if nil != obj {
			utils.ProtectRun(obj.OnSave)
		}
	}
}

func (mgr *Mgr) OnInit() {
	if nil == mgr.owner.GetBinaryData().SysOpenStatus {
		mgr.owner.GetBinaryData().SysOpenStatus = make(map[uint32]uint32)
	}
	for sysId, create := range engine.ClassSet {
		utils.ProtectRun(func() {
			obj := create()
			obj.Init(sysId, mgr, mgr.owner)
			obj.OnInit()
			mgr.objMap[sysId] = obj
		})
	}
}

func (mgr *Mgr) OneSecLoop() {
	for _, obj := range mgr.objMap {
		if nil != obj && obj.IsOpen() {
			utils.ProtectRun(obj.OneSecLoop)
		}
	}
}

func onEventCheckSystemMgr(actor iface.IPlayer, args ...interface{}) {
	if mgr, ok := actor.GetSysMgr().(*Mgr); ok {
		mgr.check()
	}
}

// RegisterSysClass 注册系统类型
func RegisterSysClass(sysId uint32, create CreateSysFuncHandle) {
	if _, repeat := engine.ClassSet[sysId]; repeat {
		logger.LogError("系统重复注册 id:%d", sysId)
		return
	}

	engine.ClassSet[sysId] = create
}

func init() {
	event.RegActorEvent(custom_id.AeLogin, func(actor iface.IPlayer, args ...interface{}) {
		if mgr := actor.GetSysMgr(); nil != mgr {
			mgr.OnLogin()
		}
	})

	event.RegActorEvent(custom_id.AeReconnect, func(actor iface.IPlayer, args ...interface{}) {
		if mgr := actor.GetSysMgr(); nil != mgr {
			mgr.OnReconnect()

		}
	})

	event.RegActorEvent(custom_id.AeAfterLogin, func(actor iface.IPlayer, args ...interface{}) {
		if mgr := actor.GetSysMgr(); nil != mgr {
			mgr.OnAfterLogin()
		}
	})
	event.RegActorEvent(custom_id.AeLogout, func(actor iface.IPlayer, args ...interface{}) {
		if mgr := actor.GetSysMgr(); nil != mgr {
			mgr.OnLogout()
		}
	})

	event.RegActorEventH(custom_id.AeNewDay, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeAfterMerge, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeSetMergeDay, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeLogin, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeLevelUp, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeCircleChange, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeVipLevelUp, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeFinishMainQuest, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeFuncOpenSysActive, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeActiveTrailFunc, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeNirvanaLvChange, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeFlyCampChange, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeFlyUpWorldFinish, onEventCheckSystemMgr)
	event.RegActorEventH(custom_id.AeBuyFuncOpenGift, onEventCheckSystemMgr)

	event.RegActorEvent(custom_id.AeLoginFight, func(actor iface.IPlayer, args ...interface{}) {
		if mgr := actor.GetSysMgr(); nil != mgr {
			mgr.OnLoginFight()
		}
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
			onEventCheckSystemMgr(player)
		})
	})

	gmevent.Register("sysopenall", func(player iface.IPlayer, args ...string) bool {
		if mgr, ok := player.GetSysMgr().(*Mgr); ok {
			for _, obj := range mgr.objMap {
				if nil == obj {
					continue
				}
				sysId := obj.GetSysId()
				conf := jsondata.GetSysOpenConf(sysId)
				if conf == nil {
					continue
				}
				if player.GetLevel() < conf.Level {
					player.SetExtraAttr(attrdef.Level, attrdef.AttrValueAlias(conf.Level))
				}
				if player.GetCircle() < conf.Circle {
					player.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(conf.Circle))
				}
				if conf.MainTaskId > 0 {
					questSys, _ := mgr.GetSysObj(sysdef.SiQuest).(*QuestSys)
					// 接任务
					questLs := questSys.GetBinaryData().AllQuest
					isAcc := true
					for _, v := range questLs {
						if v.Id == conf.MainTaskId {
							isAcc = false
						}
					}
					if isAcc {
						quest := &pb3.QuestData{Id: conf.MainTaskId}
						questSys.GetBinaryData().AllQuest = append(questSys.GetBinaryData().AllQuest, quest)
						questSys.QuestTargetBase.OnAcceptQuest(quest)
					}

					// 完成任务

					questSys.Finish(argsdef.NewFinishQuestParams(conf.MainTaskId, true, false, true))
				}
				if !obj.IsOpen() {
					obj.Open()
					obj.OnOpen()
				}
				if fOpenSys := mgr.GetSysObj(sysdef.SiFuncOpen); nil != fOpenSys && fOpenSys.IsOpen() {
					if !mgr.owner.IsOpenNotified(sysId) {
						mgr.owner.TriggerEvent(custom_id.AeFuncOpenNotify, sysId)
					}
				}
			}
			return true
		}
		return false
	}, 1)
}
