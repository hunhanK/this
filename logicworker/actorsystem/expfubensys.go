package actorsystem

import (
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/commontimesconter"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	pyy "jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/beasttidemgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/logger"
)

/**
* @Author: YangQibin
* @Desc: 经验副本
* @Date: 2023/3/21
 */

type ExpFbSys struct {
	Base
	data   *pb3.ExpFuBenData
	fbData *expFuBenInnerData

	counter *commontimesconter.CommonTimesCounter
}

type expFuBenInnerData struct {
	hdl          uint64
	sumExp       uint32
	enterLevel   uint32
	combineTimes uint32
}

func (sys *ExpFbSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *ExpFbSys) init() bool {
	if sys.GetBinaryData().Expfbdata == nil {
		data := &pb3.ExpFuBenData{}
		sys.GetBinaryData().Expfbdata = data
	}

	sys.data = sys.GetBinaryData().Expfbdata

	if nil == sys.fbData {
		sys.fbData = &expFuBenInnerData{}
	}

	if sys.data.TimesCounter == nil {
		sys.data.TimesCounter = commontimesconter.NewCommonTimesCounterData()
	}

	// 初始化计数器
	sys.counter = commontimesconter.NewCommonTimesCounter(
		sys.data.TimesCounter,
		commontimesconter.WithOnGetFreeTimes(func() uint32 {
			var totalTimes uint32
			fbConf := jsondata.GetExpFubenConf()
			if nil != fbConf {
				totalTimes += fbConf.FreeNum
			}
			firstTimes := sys.data.FirstUseTimes ^ 1
			return totalTimes + firstTimes
		}),
		commontimesconter.WithOnGetOtherAddFreeTimes(func() uint32 {
			return uint32(sys.GetOwner().GetFightAttr(attrdef.ExpFuBenTimesAdd))
		}),
		commontimesconter.WithOnGetDailyBuyTimesUpLimit(func() uint32 {
			unfreeNums, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumExpFubenBuyNums)
			return uint32(unfreeNums)
		}),
		commontimesconter.WithOnUpdateCanUseTimes(func(canUseTimes uint32) {
			firstTimes := sys.data.FirstUseTimes ^ 1
			if firstTimes != 0 && canUseTimes > firstTimes {
				canUseTimes -= firstTimes
			}
			sys.owner.SetExtraAttr(attrdef.ExpCanChangeTimes, attrdef.AttrValueAlias(canUseTimes))
			sys.owner.TriggerEvent(custom_id.AeExpFbUseTimeChange)
		},
		),
	)
	err := sys.counter.Init()
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return false
	}

	return true
}

func (sys *ExpFbSys) OnOpen() {
	sys.init()
	sys.s2cInfo()
}

func (sys *ExpFbSys) OnLogin() {
	sys.owner.SetExtraAttr(attrdef.ExpFbHisMaxExp, sys.data.HisMaxExp)
	sys.s2cInfo()
}

func (sys *ExpFbSys) OnAfterLogin() {
	sys.tryActivateTryWeekCardForException(false)
}

func (sys *ExpFbSys) s2cInfo() {
	sys.SendProto3(17, 60, &pb3.S2C_17_60{
		Data: sys.data,
	})
}

func (sys *ExpFbSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *ExpFbSys) c2sInfo(_ *base.Message) error {
	sys.SendProto3(17, 60, &pb3.S2C_17_60{
		Data: sys.data,
	})

	return nil
}

func (sys *ExpFbSys) c2sBuyChallengeTimes(msg *base.Message) error {
	var req pb3.C2S_17_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	fbConf := jsondata.GetExpFubenConf()
	if nil == fbConf {
		return neterror.ParamsInvalidError("fuben conf not exist")
	}

	if !sys.counter.CheckCanBuyDailyAddTimes(req.Nums) {
		return neterror.ParamsInvalidError("buyLimit")
	}

	consumes := jsondata.ConsumeVec{}
	dailyBuyTimes := sys.counter.GetDailyBuyTimes()
	for i := uint32(0); i < req.Nums; i++ {
		var consume = fbConf.UnFreeConsume[len(fbConf.UnFreeConsume)-1]
		if dailyBuyTimes < uint32(len(fbConf.UnFreeConsume)) {
			consume = fbConf.UnFreeConsume[dailyBuyTimes]
		}
		consumes = append(consumes, consume)
		dailyBuyTimes++
	}

	if !sys.GetOwner().ConsumeByConf(consumes, true, common.ConsumeParams{LogId: pb3.LogId_LogExpFubenBuyTimes}) {
		return neterror.ParamsInvalidError("consume not enough")
	}

	sys.counter.AddBuyDailyAddTimes(req.Nums)
	sys.SendProto3(17, 61, &pb3.S2C_17_61{
		Nums: sys.counter.GetDailyBuyTimes(),
	})

	return nil
}

func (sys *ExpFbSys) c2sQuickAttackFuben(msg *base.Message) error {
	var req pb3.C2S_17_62
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := jsondata.GetExpFubenConf()
	if conf == nil {
		return neterror.ConfNotFoundError("配置没找到")
	}

	checker := new(manager.CondChecker)
	successed, err := checker.Check(sys.GetOwner(), conf.QuickCond.Expr, conf.QuickCond.Conf)
	if err != nil {
		return neterror.ParamsInvalidError("%s", err)
	}

	if !successed {
		return neterror.ParamsInvalidError("condition not meet")
	}

	if sys.counter.CheckTimeEnough(1) {
		return neterror.ParamsInvalidError("num limit")
	}

	if sys.data.HisMaxExp <= 0 {
		return neterror.ParamsInvalidError("not pass expfuben before")
	}

	if !sys.counter.DeductTimes(1) {
		return neterror.InternalError("times not enough")
	}

	awards := jsondata.CopyStdRewardVec(conf.Awards)

	if len(awards) < 1 {
		return neterror.ConfNotFoundError("配置没找到")
	}

	awards = awards[0:1]
	awards[0].Count = sys.data.HisMaxExp

	oldLv := sys.GetOwner().GetLevel()

	state := engine.GiveRewards(sys.GetOwner(), awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogExpFubenQuickRewards,
		NoTips: true,
	})

	if !state {
		return neterror.ParamsInvalidError("give rewards failed")
	}

	sys.SendProto3(17, 62, &pb3.S2C_17_62{})
	sys.SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: &pb3.FbSettlement{
			FbId:      conf.FbId,
			ShowAward: jsondata.StdRewardVecToPb3RewardVec(awards),
			Ret:       custom_id.FbSettleResultWin,
			ExData:    []uint32{oldLv, sys.data.HisMaxCount},
		},
	})
	sys.GetOwner().TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionExpFb)
	sys.GetOwner().TriggerEvent(custom_id.AePassFb, conf.FbId)
	sys.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiExpFuben, 1) // 触发资源找回事件
	return nil
}

func (sys *ExpFbSys) GetActorAttackTimes(teamId uint64) (uint32, bool) {
	actorId := sys.owner.GetId()
	settingData, ok := sys.getTeamSettingData(teamId)
	if !ok {
		return 0, false
	}

	var combineTimes uint32
	if tActor, ok := settingData.Data[actorId]; ok {
		combineTimes = tActor.GetCombineTimes()
	}

	attackTimes := utils.MaxUInt32(1, combineTimes)

	return attackTimes, true
}

func (sys *ExpFbSys) getTeamSettingData(teamId uint64) (*pb3.ExpFbTeamData, bool) {
	fbSet := teammgr.GetTeamFbSetting(teamId)
	if nil == fbSet {
		return nil, false
	}
	if nil == fbSet.ExpFbTeamData {
		fbSet.ExpFbTeamData = &pb3.ExpFbTeamData{}
	}
	if nil == fbSet.ExpFbTeamData.Data {
		fbSet.ExpFbTeamData.Data = make(map[uint64]*pb3.ExpFbTeamActorData)
	}
	return fbSet.ExpFbTeamData, true
}

func (sys *ExpFbSys) getTeamPlayerSetData(teamId, playerId uint64) *pb3.ExpFbTeamActorData {
	if setData, ok := sys.getTeamSettingData(teamId); ok {
		actorData, ok := setData.Data[playerId]
		if ok {
			return actorData
		}
	}
	return nil
}

// todo 最好要拦截ready后的状态
func (sys *ExpFbSys) c2sCombineTimes(msg *base.Message) error {
	var req pb3.C2S_17_75
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}
	conf := jsondata.GetExpFubenConf()
	if conf == nil {
		return neterror.ParamsInvalidError("conf not exist")
	}

	if sys.owner.GetFbId() == conf.FbId {
		return neterror.ParamsInvalidError("in fb")
	}

	teamId := sys.owner.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("not in team")
	}

	if sys.owner.GetLevel() < conf.CombineLevel {
		sys.owner.SendTipMsg(tipmsgid.TpLevelNotReachTarget, conf.CombineLevel)
		return nil
	}

	state, err := teammgr.GetTeamState(teamId)
	if nil != err {
		return err
	}

	if state != teammgr.TeamStateWaiting && state != teammgr.TeamStateConsultEnterFb {
		return neterror.ParamsInvalidError("cant change combine when entering")
	}

	if req.CombineTimes > 1 {
		// 小于2次无法合并
		leftTimes := sys.counter.GetLeftTimes()
		if leftTimes < 2 || leftTimes < req.CombineTimes {
			return neterror.ParamsInvalidError("left times limit")
		}

		consumes := jsondata.ConsumeMulti(conf.CombineConsumes, req.CombineTimes-1)
		if !sys.GetOwner().CheckConsumeByConf(consumes, false, 0) {
			return neterror.ParamsInvalidError("consume not enough")
		}
	}

	err = sys.setCombineTimes(teamId, req.CombineTimes)
	if nil != err {
		return err
	}
	return nil
}

func (sys *ExpFbSys) setCombineTimes(teamId uint64, times uint32) error {
	settingData, ok := sys.getTeamSettingData(teamId)
	if !ok {
		return neterror.ParamsInvalidError("setting data get err")
	}

	tActorMap := settingData.Data
	tActor := tActorMap[sys.owner.GetId()]
	if tActor == nil {
		tActor = &pb3.ExpFbTeamActorData{}
	}

	tActor.CombineTimes = times
	tActorMap[sys.owner.GetId()] = tActor
	sys.owner.SendProto3(17, 75, &pb3.S2C_17_75{CombineTimes: tActor.CombineTimes})
	return nil
}
func (sys *ExpFbSys) c2sRecordAutoInspire(msg *base.Message) error {
	var req pb3.C2S_17_76
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	buyInspireConf := jsondata.GetExpFubenConf().Inspire[req.Id]
	if buyInspireConf == nil {
		return neterror.ParamsInvalidError("buyInspireConf not exists")
	}

	if req.GetIsAuto() {
		if !pie.Uint32s(sys.data.InspireAutoIds).Contains(req.GetId()) {
			sys.data.InspireAutoIds = append(sys.data.InspireAutoIds, req.GetId())
		}
	} else {
		sys.data.InspireAutoIds = pie.Uint32s(sys.data.InspireAutoIds).Filter(func(u uint32) bool {
			return req.GetId() != u
		})
	}
	sys.SendProto3(17, 76, &pb3.S2C_17_76{
		Id:     req.GetId(),
		IsAuto: req.GetIsAuto(),
	})
	return nil
}

func (sys *ExpFbSys) onNewDay() {
	// 先加上 下个版本移除
	sys.data.ChallengedTimes = 0
	sys.data.BuyedTimes = 0
	sys.data.ItemAddTimes = 0
	sys.counter.NewDay()
	sys.s2cInfo()
}

func (sys *ExpFbSys) checkout(calSt *pb3.ExpFbSettlement) {
	// 没有奖励次数
	player := sys.GetOwner()
	settle := calSt.Settle
	actorData := sys.getTeamPlayerSetData(player.GetTeamId(), player.GetId())
	if actorData != nil && !actorData.CanGetExp {
		if s, ok := sys.owner.GetSysObj(sysdef.SiAssistance).(*AssistanceSys); ok && s.IsOpen() {
			if !s.CompileTeam() {
				sys.SendProto3(17, 254, &pb3.S2C_17_254{
					Settle: &pb3.FbSettlement{
						FbId:     settle.FbId,
						Ret:      settle.Ret,
						PassTime: settle.PassTime,
					}})
			}
		}
		return
	}
	actorData.CanGetExp = false

	useTimes, _ := sys.GetActorAttackTimes(sys.owner.GetTeamId())
	calSt.CombineTimes = 1
	if useTimes > 1 && sys.counter.CheckTimeEnough(useTimes-1) {
		settingCombineTimes := useTimes - 1
		consumes := jsondata.ConsumeMulti(jsondata.GetExpFubenConf().CombineConsumes, settingCombineTimes)
		if sys.owner.ConsumeByConf(consumes, false, common.ConsumeParams{
			LogId: pb3.LogId_LogExpFbCombineConsume,
		}) {
			calSt.CombineTimes = useTimes
			sys.ReduceTimes(settingCombineTimes)
		}
	}

	// 结算后取消合并打钩选项
	err := sys.setCombineTimes(sys.owner.GetTeamId(), 0)
	if err != nil {
		sys.LogError("err:%v")
	}

	passTimes := calSt.CombineTimes

	sys.GetOwner().TriggerEvent(custom_id.AePassFb, jsondata.GetExpFubenConf().FbId, passTimes)

	if settle.ShowAward == nil {
		sys.owner.LogError("checkout failed, no award")
		return
	}

	sumExp := settle.ShowAward[0].Count

	guaranteedBonusExp := jsondata.GetGuaranteedBonusExp(sys.fbData.enterLevel)
	expToSupply := utils.MaxInt64(0, int64(guaranteedBonusExp)-sumExp)
	// 没达到保底，补足
	if expToSupply > 0 {
		sys.owner.AddExp(expToSupply, pb3.LogId_LogExpFubenSupplyExp, false)
	}

	sumExp = sumExp + expToSupply
	var combineExp int64
	if calSt.CombineTimes > 1 {
		combineExp = int64(calSt.CombineTimes-1) * sumExp
	}

	if combineExp > 0 {
		sys.owner.AddExp(combineExp, pb3.LogId_LogCombineExpFuben, false)
	}

	settle.ShowAward[0].Count = sumExp + combineExp

	sys.SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: settle,
	})

	if sys.data.HisMaxExp <= 0 {
		sys.tryActivateTryWeekCardForException(true)
	}

	if sumExp > sys.data.HisMaxExp {
		sys.data.HisMaxExp = int64(sumExp)
		sys.owner.SetExtraAttr(attrdef.ExpFbHisMaxExp, sys.data.HisMaxExp)
		if len(settle.ExData) >= 2 {
			sys.data.HisMaxCount = settle.ExData[0]
		}
		sys.SendProto3(17, 69, &pb3.S2C_17_69{
			HisMaxExp: uint32(sys.data.HisMaxExp),
		})
	}
}

func (sys *ExpFbSys) tryActivateTryWeekCardForException(whenCheckout bool) {
	pvCardSys, ok := sys.owner.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !pvCardSys.IsOpen() {
		return
	}

	if _, ok := pvCardSys.data.Cards[privilegedef.PrivilegeCardType_TryWeek]; ok {
		return
	}

	cardConf := jsondata.GetPrivilegeCardConfByType(privilegedef.PrivilegeCardType_TryWeek)
	if cardConf == nil {
		sys.GetOwner().LogError("get conf for PrivilegeCardType_TryWeek failed")
		return
	}

	freeWeekCardParams := jsondata.GetCommonConf("freeWeeklyCardTask").U32Vec
	if len(freeWeekCardParams) < 4 {
		sys.GetOwner().LogError("get conf for freeWeeklyCardTask failed")
		return
	}

	if !whenCheckout {
		questSys, ok := sys.owner.GetSysObj(sysdef.SiQuest).(*QuestSys)
		if !ok || !questSys.IsOpen() {
			return
		}

		questConf := jsondata.GetQuestConf(freeWeekCardParams[1])
		if questConf == nil {
			sys.LogError("get conf for freeWeeklyCardTask failed or quest %d not found", freeWeekCardParams[1])
			return
		}
		if questConf.Type != custom_id.QtMain {
			sys.LogError("get conf for freeWeeklyCardTask failed or quest %d is not main type", questConf.Id)
			return
		}

		if !questSys.IsFinishMainQuest(freeWeekCardParams[1]) {
			return
		}
	}

	state := sys.GetOwner().ConsumeByConf(
		jsondata.ConsumeVec{&jsondata.Consume{Id: freeWeekCardParams[3], Count: 1, Type: custom_id.ConsumeTypeItem}},
		false,
		common.ConsumeParams{LogId: pb3.LogId_LogUseItem},
	)

	if !state {
		sys.GetOwner().LogError("consume failed but try week card still activated for exception")
	}

	cardData := &pb3.PrivilegeCard{
		Type: privilegedef.PrivilegeCardType_TryWeek,
	}

	pvCardSys.data.Cards[privilegedef.PrivilegeCardType_TryWeek] = cardData
	cardData.Level = 1
	cardData.StartTime = uint32(time.Now().Unix())
	cardData.ExpireTime = uint32(time.Now().Unix()) + cardConf.DurTime

	pvCardSys.setTimer(privilegedef.PrivilegeCardType_TryWeek)

	sys.GetOwner().SetTimeout(1*time.Millisecond, func() {
		sys.GetOwner().TriggerEvent(custom_id.AePrivilegeCardActivated, argsdef.AePvCardActivatedArg(privilegedef.PrivilegeCardType_TryWeek))
		sys.GetOwner().TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	})
	pvCardSys.s2cInfo()
}

func (sys *ExpFbSys) c2sUseThounderSkill(msg *base.Message) error {
	var req pb3.C2S_6_60
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	pv, err := sys.GetOwner().GetPrivilege(privilegedef.EnumExpFubenThounder)
	if err != nil {
		return neterror.InternalError("get privilege failed %s", err)
	}

	var useSkillReq *pb3.ExpFubenUseSkill = &pb3.ExpFubenUseSkill{}

	useSkillReq.Inifite = pv > 0

	useSkillReq.Skill = &pb3.UseSkillSt{
		Id:     req.Skill.Id,
		Handle: req.Skill.Handle,
		Posx:   req.Skill.Posx,
		Posy:   req.Skill.Posy,
		Dir:    req.Skill.Dir,
	}

	err = sys.GetOwner().CallActorFunc(actorfuncid.ExpFubenUseSkill, useSkillReq)
	if err != nil {
		return neterror.InternalError("call actor func failed %s", err)
	}

	return nil
}

func gmExpFbRefresh(player iface.IPlayer, args ...string) bool {
	expFuBen, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !expFuBen.IsOpen() {
		return false
	}

	expFuBen.onNewDay()
	return true
}

func expFubenCheckout(player iface.IPlayer, buf []byte) {
	expfubenSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !expfubenSys.IsOpen() {
		player.LogError("expFubenCheckout sys == nil or not open")
		return
	}

	var req pb3.ExpFbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("expFubenCheckout Unmarshal failed %s", err)
		return
	}

	expfubenSys.checkout(&req)
}

func (sys *ExpFbSys) onEnterFb(req *pb3.CommonSt) {
	enterLevel, fbHdl := req.GetU32Param(), req.GetU64Param()
	if fbHdl == 0 {
		return
	}
	useTimes, _ := sys.GetActorAttackTimes(sys.owner.GetTeamId())
	sys.fbData = &expFuBenInnerData{
		hdl:          fbHdl,
		enterLevel:   enterLevel,
		combineTimes: useTimes,
	}
}

func expFubSys_onEnterFb(player iface.IPlayer, buf []byte) {
	expfubenSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !expfubenSys.IsOpen() {
		player.LogError("expFubenCheckout sys == nil or not open")
		return
	}

	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("expFubenCheckout Unmarshal failed %s", err)
		return
	}

	expfubenSys.onEnterFb(&req)

}

func expFubenCalcOrCalcAddExp(player iface.IPlayer, buf []byte) {
	expfubenSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !expfubenSys.IsOpen() {
		player.LogError("expFubenCalcOrCalcAddExp sys == nil or not open")
		return
	}

	var req pb3.ExpFubenCalcOrCalcAddExp
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("expFubenCalcOrCalcAddExp unmarshal failed %s", err)
		return
	}

	actorData := expfubenSys.getTeamPlayerSetData(player.GetTeamId(), player.GetId())
	if actorData != nil && actorData.CanGetExp {
		// 经验副本特权额外加成
		rate, _ := player.GetPrivilege(privilegedef.EnumExpFubenYield)
		rate2 := beasttidemgr.GetBeastTideAddition(pyy.BTEffectTypeDoubleExperience)

		// 队友的经验加成(怪物本身的基础经验) * (1 + 加成/10000)
		allRate := req.AddRate + uint32(rate) + uint32(rate2)

		levelSys, ok := player.GetSysObj(sysdef.SiLevel).(*LevelSys)
		if !ok {
			player.LogError("get level sys failed")
			return
		}

		fbData := expfubenSys.fbData
		if fbData.hdl != req.GetFbHdl() {
			return
		}

		attenuationConf := jsondata.GetAttenuationRateConf(fbData.enterLevel)
		if nil == attenuationConf {
			return
		}

		baseExp := int64(req.GetBaseExp())
		baseExp = baseExp * int64(attenuationConf.GetAttenuationRate(fbData.sumExp)) / 100 //衰减率

		finalExp, totalAddRate := levelSys.CalcFinalExp(int64(baseExp), true, allRate)
		if finalExp+int64(fbData.sumExp) >= int64(attenuationConf.ExpGetLimit) {
			finalExp = utils.MaxInt64(int64(attenuationConf.ExpGetLimit)-int64(fbData.sumExp), 0)
		}

		fbData.sumExp += uint32(finalExp)
		player.AddExpV2(finalExp, pb3.LogId_LogKillExpFuBenMonsterExp, totalAddRate)

		player.CallActorFunc(actorfuncid.ExpFubenCalcOrCalcAddExpCb, &pb3.ExpFubenCalcOrCalcAddExpCb{
			FinalAddedExp: uint32(finalExp),
		})
	}
}

func addExpFubenSize(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	addTimes := conf.Param[0]

	expfubenSys, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !expfubenSys.IsOpen() {
		return false, false, 0
	}

	sumAddTimes := 0
	for i := 0; i < int(param.Count); i++ {
		sumAddTimes += int(addTimes)
	}

	expfubenSys.counter.AddItemDailyAddTimes(uint32(sumAddTimes))
	player.SendProto3(17, 68, &pb3.S2C_17_68{
		ItemAddTimes: expfubenSys.counter.GetDailyItemAddTimes(),
	})

	return true, true, param.Count
}

func useExpFubenMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sysObj, ok := player.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
	if !ok || !sysObj.IsOpen() {
		return
	}

	medicineId := conf.Param[0]

	medicineConf := jsondata.GetExpFubenConf().Medicine[medicineId]
	if medicineConf == nil {
		return false, false, 0
	}

	nowSec := time_util.NowSec()
	data := sysObj.data
	preMedicineId := data.UseMedicineId
	if preMedicineId != 0 && data.UseMedicineEndTime >= nowSec {
		preMedicineConf := jsondata.GetExpFubenConf().Medicine[preMedicineId]
		if preMedicineConf != nil && preMedicineConf.AddDis > medicineConf.AddDis {
			return false, false, 0
		}
	}

	buffConf := jsondata.GetBuffConfig(medicineConf.BuffId)
	if nil == buffConf {
		return false, false, 0
	}

	durTime := buffConf.Duration * param.Count
	addTimeSec := uint32(durTime / 1000)
	if data.UseMedicineId == medicineId && data.UseMedicineEndTime >= nowSec { //在生效期间直接叠加
		data.UseMedicineEndTime += addTimeSec
	} else {
		data.UseMedicineId = medicineId
		data.UseMedicineEndTime = addTimeSec + nowSec
	}

	player.AddBuffByTime(medicineConf.BuffId, durTime)
	player.SendProto3(17, 64, &pb3.S2C_17_64{
		Id:      data.UseMedicineId,
		EndTime: data.UseMedicineEndTime,
	})
	return true, true, param.Count
}

func (sys *ExpFbSys) TeamCheckEnterConsume(teamId uint64) bool {
	useTimes, ok := sys.GetActorAttackTimes(teamId)
	if !ok {
		return false
	}

	if useTimes > 1 {
		consumes := jsondata.ConsumeMulti(jsondata.GetExpFubenConf().CombineConsumes, useTimes-1)
		if !sys.owner.CheckConsumeByConf(consumes, false, 0) {
			sys.owner.SendTipMsg(tipmsgid.TeamMemberNotEnoughCombineConsumes, sys.owner.GetId(), sys.owner.GetName())
			return false
		}
	}

	return sys.counter.GetLeftTimes() > 0
}

func (sys *ExpFbSys) ReduceTimes(useTimes uint32) bool {
	if useTimes == 0 {
		sys.owner.LogError("useTimes is zero")
		return false
	}

	if !sys.counter.CheckTimeEnough(useTimes) {
		return false
	}

	var dTimes = uint32(0)
	if sys.data.FirstUseTimes == 0 {
		sys.data.FirstUseTimes++
		dTimes = useTimes - 1
	} else {
		dTimes = useTimes
	}

	if dTimes > 0 && !sys.counter.DeductTimes(dTimes) {
		sys.owner.LogWarn("times not enough")
		return false
	}

	logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogExpFbTimesReduce, &pb3.LogPlayerCounter{
		NumArgs: uint64(useTimes),
	})

	sys.owner.SendProto3(17, 67, &pb3.S2C_17_67{
		ChallengedTimes: sys.counter.GetDailyUseTimes(),
	})
	sys.owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionExpFb, int(useTimes))
	// 触发资源找回事件
	sys.owner.TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiExpFuben, int(useTimes))
	return true
}

func (sys *ExpFbSys) TeamDoEnterConsume() bool {
	useTimes := uint32(1)
	if !sys.counter.CheckTimeEnough(useTimes) {
		return false
	}

	sys.ReduceTimes(useTimes)
	return true
}

func init() {
	RegisterSysClass(sysdef.SiExpFuben, func() iface.ISystem {
		return &ExpFbSys{}
	})

	net.RegisterSysProto(17, 60, sysdef.SiExpFuben, (*ExpFbSys).c2sInfo)
	net.RegisterSysProto(17, 61, sysdef.SiExpFuben, (*ExpFbSys).c2sBuyChallengeTimes)
	net.RegisterSysProto(17, 62, sysdef.SiExpFuben, (*ExpFbSys).c2sQuickAttackFuben)
	net.RegisterSysProto(17, 75, sysdef.SiExpFuben, (*ExpFbSys).c2sCombineTimes)
	net.RegisterSysProto(17, 76, sysdef.SiExpFuben, (*ExpFbSys).c2sRecordAutoInspire)

	net.RegisterSysProto(6, 60, sysdef.SiExpFuben, (*ExpFbSys).c2sUseThounderSkill)

	engine.RegisterActorCallFunc(playerfuncid.ExpFbCheckout, expFubenCheckout)

	engine.RegisterActorCallFunc(playerfuncid.ExpFubenCalcOrCalcAddExp, expFubenCalcOrCalcAddExp)

	engine.RegisterActorCallFunc(playerfuncid.F2GExpFubenEnter, expFubSys_onEnterFb)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		expFuben, ok := actor.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
		if !ok || !expFuben.IsOpen() {
			return
		}
		expFuben.onNewDay()
	})

	event.RegActorEvent(custom_id.AeRegFightAttrChange, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		attrType, ok := args[0].(uint32)
		if !ok {
			return
		}
		if attrType != attrdef.ExpFuBenTimesAdd {
			return
		}
		expFuben, ok := actor.GetSysObj(sysdef.SiExpFuben).(*ExpFbSys)
		if !ok || !expFuben.IsOpen() {
			return
		}
		expFuben.counter.ReCalcTimes()
	})

	event.RegSysEvent(custom_id.SeReloadJson, func(args ...interface{}) {
		freeWeekCardParams := jsondata.GetCommonConf("freeWeeklyCardTask").U32Vec
		if len(freeWeekCardParams) < 4 {
			logger.LogError("get conf for freeWeeklyCardTask failed")
			return
		}
		questConf := jsondata.GetQuestConf(freeWeekCardParams[1])
		if questConf == nil {
			logger.LogError("get conf for freeWeeklyCardTask failed or quest %d not found", freeWeekCardParams[1])
			return
		}
		if questConf.Type != custom_id.QtMain {
			logger.LogError("get conf for freeWeeklyCardTask failed or quest %d is not main type", questConf.Id)
			return
		}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddExpFubenTimes, addExpFubenSize)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemUseExpFubenMedicine, useExpFubenMedicine)

	gmevent.Register("expFbRefresh", gmExpFbRefresh, 1)
}
