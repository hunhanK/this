/**
 * @Author: zjj
 * @Date: 2024/12/5
 * @Desc: 活动任务日常扫荡
**/

package actorsystem

import (
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/actsweepmgr"
	"jjyz/base/argsdef"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
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
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type ActSweepSys struct {
	Base
}

func (s *ActSweepSys) s2cInfo() {
	s.SendProto3(32, 20, &pb3.S2C_32_20{
		Data: s.getData(),
	})
}

func (s *ActSweepSys) getData() *pb3.ActSweepData {
	data := s.GetBinaryData().ActSweepData
	if data == nil {
		s.GetBinaryData().ActSweepData = &pb3.ActSweepData{}
		data = s.GetBinaryData().ActSweepData
	}
	if data.ActMap == nil {
		data.ActMap = make(map[uint32]*pb3.ActSweepInfo)
	}
	return data
}

// 初始化
func (s *ActSweepSys) initAllSweep() {
	data := s.getData()
	data.ActMap = make(map[uint32]*pb3.ActSweepInfo)
	jsondata.EachActSweepConf(func(conf *jsondata.ActSweepConf) {
		_, ok := data.ActMap[conf.Id]
		if !ok {
			data.ActMap[conf.Id] = &pb3.ActSweepInfo{
				Id: conf.Id,
			}
		}
	})
}

func (s *ActSweepSys) calcCanUseTimes(id uint32) {
	data := s.getData()
	playerId := s.GetOwner().GetId()
	jsondata.EachActSweepConf(func(conf *jsondata.ActSweepConf) {
		if id != 0 && id != conf.Id {
			return
		}
		actSweepInfo, ok := data.ActMap[conf.Id]
		if !ok {
			return
		}
		iActSweep := actsweepmgr.Get(conf.Id)
		if iActSweep == nil {
			return
		}
		actSweepInfo.CanUseTimes = iActSweep.GetCanUseTimes(conf.Id, playerId)
		useTimes, ok := iActSweep.GetUseTimes(conf.Id, playerId)
		if ok {
			actSweepInfo.SweepTimes = useTimes
		}
	})
}

func (s *ActSweepSys) OneSecLoop() {
	expAt := s.getData().PrivilegeExpAt
	if expAt == 0 {
		return
	}
	nowSec := time_util.NowSec()
	if nowSec >= expAt {
		s.ResetSysAttr(attrdef.SaActSweep)
		s.getData().PrivilegeExpAt = 0
	}
}

func (s *ActSweepSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ActSweepSys) OnLogin() {
	s.calcCanUseTimes(0)
	s.s2cInfo()
}

func (s *ActSweepSys) OnOpen() {
	s.initAllSweep()
	s.calcCanUseTimes(0)
	s.s2cInfo()
}

func (s *ActSweepSys) onNewDay() {
	s.initAllSweep()
	s.calcCanUseTimes(0)
	s.s2cInfo()
}

func (s *ActSweepSys) checkPrivilege() bool {
	data := s.getData()
	privilegeExpAt := data.PrivilegeExpAt
	ok := privilegeExpAt != 0 && privilegeExpAt > time_util.NowSec()
	if ok {
		return true
	}
	return s.GetOwner().GetExtraAttr(attrdef.CelebrationFreePrivilege) > 0
}

func (s *ActSweepSys) c2sOne(msg *base.Message) error {
	var req pb3.C2S_32_22
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	if !s.checkPrivilege() {
		return neterror.ParamsInvalidError("not buy privilege or privilege expired")
	}

	id := req.Id
	times := req.Times
	data := s.getData()
	info, ok := data.ActMap[id]
	if !ok {
		return neterror.ParamsInvalidError("not found %d act sweep", id)
	}
	if times == 0 {
		return neterror.ParamsInvalidError("not input times")
	}

	if info.CanUseTimes == 0 || info.CanUseTimes <= info.SweepTimes || info.CanUseTimes < info.SweepTimes+times {
		return neterror.ParamsInvalidError("CanUseTimes %d SweepTimes %d times:%d", info.CanUseTimes, info.SweepTimes, times)
	}

	owner := s.GetOwner()
	level := owner.GetLevel()
	consume, awards, err := s.calcOneActSweepConsumeAndAwards(id, level)
	if err != nil {
		owner.LogError("err:%v", err)
		return err
	}

	consumeVec := jsondata.ConsumeMulti(consume, times)
	if len(consumeVec) == 0 || !owner.ConsumeByConf(consumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogActSweepOne}) {
		return neterror.ConsumeFailedError("%d %d not found consume", id, level)
	}

	var totalAwards jsondata.StdRewardVec
	for i := uint32(0); i < times; i++ {
		totalAwards = append(totalAwards, s.randomAwards(awards)...)
	}
	if len(totalAwards) > 0 {
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogActSweepOne})
	}

	useTimes := s.AddUseTimes(id, times)
	s.SendProto3(32, 22, &pb3.S2C_32_22{ActSweepInfo: info, GroupItems: useTimes})
	owner.SendShowRewardsPop(totalAwards)
	s.triggerEvent(id, times)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActSweepOne, &pb3.LogPlayerCounter{
		NumArgs: uint64(id),
		StrArgs: fmt.Sprintf("%d_%d", info.SweepTimes, level),
	})
	return nil
}

func (s *ActSweepSys) AddUseTimes(id, times uint32) []*pb3.ActSweepInfo {
	var updateSweepTimes = func(entry *pb3.ActSweepInfo, playerId uint64, times uint32) {
		iActSweep := actsweepmgr.Get(id)
		if iActSweep == nil {
			return
		}
		useTimes, ok := iActSweep.GetUseTimes(id, playerId)
		if ok {
			entry.SweepTimes = useTimes
		} else {
			entry.SweepTimes += times
		}
	}

	// 有的业务需要同步去扣次数
	actorId := s.GetOwner().GetId()
	info := s.getData().ActMap[id]
	if actSweep := actsweepmgr.Get(id); actSweep != nil {
		actSweep.AddUseTimes(id, times, actorId)
	}
	updateSweepTimes(info, actorId, times)

	// 同步一下分组的最新数量
	var groupList []*pb3.ActSweepInfo
	sweepConf := jsondata.GetActSweepConf(id)
	if sweepConf.Group > 0 {
		confByGroups := jsondata.GetActSweepConfByGroup(sweepConf.Group)
		for _, groupItem := range confByGroups {
			info := s.getData().ActMap[groupItem.Id]
			groupList = append(groupList, info)
			if groupItem.Id == id {
				continue
			}
			updateSweepTimes(info, actorId, times)
		}
	}
	return groupList
}

func (s *ActSweepSys) calcOneActSweepConsumeAndAwards(id uint32, level uint32) (consume jsondata.ConsumeVec, awards jsondata.StdRewardVec, err error) {
	sweepConf := jsondata.GetActSweepConf(id)
	if sweepConf == nil {
		err = neterror.ConfNotFoundError("%d not found conf", id)
		return
	}

	var levelAwards *jsondata.ActSweepLevelAwards
	for _, lAward := range sweepConf.LevelAwards {
		if lAward.MinLv != 0 && lAward.MinLv > level {
			continue
		}
		if lAward.MaxLv != 0 && lAward.MaxLv < level {
			continue
		}
		levelAwards = lAward
		break
	}

	if levelAwards == nil {
		err = neterror.ConfNotFoundError("%d level awards not found", level)
		return
	}
	consume = levelAwards.Consume
	awards = levelAwards.Awards
	return
}

func (s *ActSweepSys) c2sAll(msg *base.Message) error {
	var req pb3.C2S_32_23
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	if !s.checkPrivilege() {
		return neterror.ParamsInvalidError("not buy privilege or privilege expired")
	}

	if len(req.ActMap) == 0 {
		return neterror.ParamsInvalidError("not input act and times")
	}

	data := s.getData()
	var canSweepMap = make(map[uint32]uint32)
	for id, count := range req.ActMap {
		if count == 0 {
			continue
		}
		info, ok := data.ActMap[id]
		if !ok {
			return neterror.ParamsInvalidError("not found %d info", id)
		}
		if info.CanUseTimes == 0 || info.SweepTimes >= info.CanUseTimes || info.SweepTimes+count > info.CanUseTimes {
			return neterror.ParamsInvalidError("%d sweep time enough", id)
		}
		canSweepMap[info.Id] = count
	}

	if len(canSweepMap) == 0 {
		return neterror.ParamsInvalidError("can not sweep list")
	}

	owner := s.GetOwner()
	level := owner.GetLevel()
	var totalConsumeList jsondata.ConsumeVec
	var totalAwards jsondata.StdRewardVec

	// 活动的消耗和奖励
	var calcActSweepCA = func(id, count, level uint32) error {
		consume, awards, err := s.calcOneActSweepConsumeAndAwards(id, level)
		if err != nil {
			return neterror.Wrap(err)
		}

		if len(consume) == 0 {
			return neterror.ConfNotFoundError("%d %d not found consume", id, level)
		}

		totalConsumeList = append(totalConsumeList, jsondata.ConsumeMulti(consume, count)...)
		for i := uint32(0); i < count; i++ {
			randomAwards := s.randomAwards(awards)
			if len(randomAwards) == 0 {
				return neterror.ConfNotFoundError("%d %d not found awards", id, level)
			}
			totalAwards = append(totalAwards, randomAwards...)
		}

		totalAwards = jsondata.MergeStdReward(totalAwards)
		totalConsumeList = jsondata.MergeConsumeVec(totalConsumeList)
		return nil
	}

	var groupKV = make(map[uint32]pie.Uint32s)

	// 处理没分组的
	for id, count := range canSweepMap {
		sweepConf := jsondata.GetActSweepConf(id)
		if sweepConf == nil {
			return neterror.ConfNotFoundError("%d not found conf", id)
		}

		if sweepConf.Group > 0 {
			groupKV[sweepConf.Group] = append(groupKV[sweepConf.Group], id)
			continue
		}

		err := calcActSweepCA(id, count, level)
		if err != nil {
			return neterror.Wrap(err)
		}
	}

	// 处理分组的
	for _, ids := range groupKV {
		id := ids.Max()
		count := canSweepMap[id]
		err := calcActSweepCA(id, count, level)
		if err != nil {
			return neterror.Wrap(err)
		}
	}

	// 去消耗
	if !owner.ConsumeByConf(totalConsumeList, false, common.ConsumeParams{LogId: pb3.LogId_LogActSweepAll}) {
		return neterror.ConsumeFailedError("all sweep %d not found awards", level)
	}

	var resp = &pb3.S2C_32_23{
		ActMap: make(map[uint32]*pb3.ActSweepInfo),
	}

	// 先处理分组的
	for _, ids := range groupKV {
		id := ids.Max()
		count := canSweepMap[id]
		useTimes := s.AddUseTimes(id, count)
		s.triggerEvent(id, count)
		for _, sweepInfo := range useTimes {
			resp.ActMap[sweepInfo.Id] = sweepInfo
			delete(canSweepMap, sweepInfo.Id)
		}
	}

	// 再处理没分组的
	for id, count := range canSweepMap {
		s.AddUseTimes(id, count)
		s.triggerEvent(id, count)
		resp.ActMap[id] = data.ActMap[id]
	}

	engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogActSweepAll})
	bytes, _ := json.Marshal(resp.ActMap)
	s.SendProto3(32, 23, resp)
	owner.SendShowRewardsPop(totalAwards)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActSweepAll, &pb3.LogPlayerCounter{
		NumArgs: uint64(level),
		StrArgs: string(bytes),
	})
	return nil
}

func (s *ActSweepSys) triggerEvent(id uint32, times uint32) {
	owner := s.GetOwner()
	sweepConf := jsondata.GetActSweepConf(id)
	if sweepConf == nil {
		return
	}

	// 触发减少资源找回
	if sweepConf.CompleteRetrievalSysId > 0 {
		event.TriggerEvent(owner, custom_id.AeCompleteRetrieval, int(sweepConf.CompleteRetrievalSysId), int(times))
	}

	// 触发日常任务
	if sweepConf.DailyMissionId > 0 {
		owner.TriggerEvent(custom_id.AeDailyMissionComplete, uint32(sweepConf.DailyMissionId), int(times))
	}

	// 增加任务进度
	for _, quest := range sweepConf.TriggerQuestList {
		owner.TriggerQuestEvent(quest.Type, quest.Target, int64(times))
	}

	// 法宝天赋
	if sweepConf.FaBaoTalent > 0 {
		s.owner.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
			Cond:  sweepConf.FaBaoTalent,
			Count: times,
		})
	}

	// 时装套装
	if sweepConf.FashionSet > 0 {
		s.owner.TriggerEvent(custom_id.AeFashionTalentEvent, &custom_id.FashionTalentEvent{
			Cond:  sweepConf.FashionSet,
			Count: times,
		})
	}

	// 完成任务
	if qSys := owner.GetSysObj(sysdef.SiQuest); qSys != nil && qSys.IsOpen() {
		questSys := qSys.(*QuestSys)
		for _, questId := range sweepConf.FinishQuestIds {
			questSys.Finish(argsdef.NewFinishQuestParams(questId, false, false, true))
		}
	}
}

func (s *ActSweepSys) c2sRecDailyAwards(_ *base.Message) error {
	if !s.checkPrivilege() {
		return neterror.ParamsInvalidError("not buy privilege or privilege expired")
	}

	data := s.getData()
	nowSec := time_util.NowSec()
	if nowSec != 0 && time_util.IsSameDay(data.LastRecDailyAwardsAt, nowSec) {
		return neterror.ParamsInvalidError("today already rec daily awards")
	}

	commonConf := jsondata.GetActSweepCommonConf()
	if commonConf == nil {
		return neterror.ConfNotFoundError("not found common conf")
	}

	if len(commonConf.DailyAwards) == 0 {
		return neterror.ConfNotFoundError("not found daily awards")
	}

	data.LastRecDailyAwardsAt = nowSec
	owner := s.GetOwner()
	engine.GiveRewards(owner, commonConf.DailyAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogActSweepRecDailyAwards})

	s.SendProto3(32, 24, &pb3.S2C_32_24{
		LastRecDailyAwardsAt: data.LastRecDailyAwardsAt,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActSweepRecDailyAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.LastRecDailyAwardsAt),
	})
	return nil
}

func (s *ActSweepSys) onCharge(chargeId uint32) {
	owner := s.GetOwner()
	commonConf := jsondata.GetActSweepCommonConf()
	if commonConf == nil {
		owner.LogWarn("not found common conf")
		return
	}
	if commonConf.ChargeId != chargeId {
		owner.LogWarn("chargeId not equal %d %d", chargeId, commonConf.ChargeId)
		return
	}
	data := s.getData()
	privilegeExpAt := data.PrivilegeExpAt
	nowSec := time_util.NowSec()
	var newPrivilegeExpAt uint32
	if nowSec > privilegeExpAt {
		newPrivilegeExpAt = nowSec + commonConf.Days*86400
	} else {
		newPrivilegeExpAt += privilegeExpAt + commonConf.Days*86400
	}
	data.PrivilegeExpAt = newPrivilegeExpAt
	s.ResetSysAttr(attrdef.SaActSweep)
	s.owner.TriggerEvent(custom_id.AeActiveActSweep)
	s.SendProto3(32, 21, &pb3.S2C_32_21{
		PrivilegeExpAt:       newPrivilegeExpAt,
		PrivilegeStartAt:     data.PrivilegeStartAt,
		LastRecDailyAwardsAt: data.LastRecDailyAwardsAt,
	})
	engine.BroadcastTipMsgById(tipmsgid.ActSweepCommonTip, owner.GetId(), owner.GetName())
}

func (s *ActSweepSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	if !s.checkPrivilege() {
		return
	}
	commonConf := jsondata.GetActSweepCommonConf()
	if commonConf == nil {
		return
	}
	if len(commonConf.Attrs) == 0 {
		return
	}
	engine.CheckAddAttrsToCalc(s.GetOwner(), calc, commonConf.Attrs)
}

func (s *ActSweepSys) randomAwards(awards jsondata.StdRewardVec) jsondata.StdRewardVec {
	var retAwards jsondata.StdRewardVec
	if len(awards) == 0 {
		return nil
	}
	stdRewardVec := engine.FilterRewardByPlayer(s.GetOwner(), awards)
	for _, v := range stdRewardVec {
		if !random.Hit(v.Weight, 10000) {
			continue
		}
		retAwards = append(retAwards, v)
	}
	return retAwards
}

// MergeTimesChallengeBoss 合并次数挑战BOSS
func (s *ActSweepSys) MergeTimesChallengeBoss(fightType base.ServerType, fbTodoId uint32, mergeTimes uint32) error {
	if mergeTimes == 0 || mergeTimes == 1 {
		return nil
	}
	if !s.checkPrivilege() {
		return neterror.ParamsInvalidError("not buy privilege or privilege expired")
	}
	owner := s.GetOwner()
	conf := jsondata.GetActSweepCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("act sweep not found conf")
	}
	err := owner.CallActorSysFn(fightType, actorfuncid.G2FMergeTimesChallengeBossReq, &pb3.G2FMergeTimesChallengeBossReq{
		FbToDoId:   fbTodoId,
		MergeTimes: mergeTimes - 1,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

// GiveMergeAwards 下发合并奖励
func (s *ActSweepSys) GiveMergeAwards(req *pb3.F2GActSweepGiveMergeAwardsReq) {
	if req == nil {
		return
	}
	owner := s.GetOwner()
	rewardVec := jsondata.Pb3RewardVecToStdRewardVec(req.Awards)
	if len(rewardVec) == 0 {
		return
	}
	rewardVec = jsondata.MergeStdReward(rewardVec)
	engine.GiveRewards(owner, rewardVec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogActSweepMergeBossTimesAwards})
	owner.SendShowRewardsPop(rewardVec)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogActSweepMergeBossTimesAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.FbToDoId),
		StrArgs: fmt.Sprintf("%d", req.BossId),
	})
}

func handleSaActSweep(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		s.calcAttr(calc)
		return
	}
}

func chargeActSweepBack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if s, ok := actor.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		s.onCharge(conf.ChargeId)
		return true
	}
	return false
}

func chargeActSweepCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	return true
}

func handleSiActSweepAeNewDay(player iface.IPlayer, _ ...interface{}) {
	if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		s.onNewDay()
	}
}

func handleSiActSweepSeActSweepNewActivityEnd(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	id := args[0].(uint32)
	sweepConf := jsondata.GetActSweepConf(id)
	if sweepConf == nil {
		return
	}
	var playerId uint64
	if len(args) >= 2 {
		playerId = args[1].(uint64)
	}
	var playerFunc = func(player iface.IPlayer) {
		if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
			s.calcCanUseTimes(sweepConf.Id)
			s.s2cInfo()
		}
	}
	if playerId != 0 {
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			logger.LogWarn("not found player:%d", playerId)
			return
		}
		playerFunc(player)
		return
	}
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		playerFunc(player)
	})
}

func handleActSweepPrivilegeCalculater(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
	if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		if s.checkPrivilege() {
			total = conf.ActSweep
		}
		return
	}
	return
}

func handleF2GActSweepGiveMergeAwardsReq(player iface.IPlayer, buf []byte) {
	var req pb3.F2GActSweepGiveMergeAwardsReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		return
	}
	if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		s.GiveMergeAwards(&req)
	}
}

func init() {
	engine.RegAttrCalcFn(attrdef.SaActSweep, handleSaActSweep)
	engine.RegChargeEvent(chargedef.ActSweep, chargeActSweepCheck, chargeActSweepBack)
	net.RegisterSysProtoV2(32, 22, sysdef.SiActSweep, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ActSweepSys).c2sOne
	})
	net.RegisterSysProtoV2(32, 23, sysdef.SiActSweep, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ActSweepSys).c2sAll
	})
	net.RegisterSysProtoV2(32, 24, sysdef.SiActSweep, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ActSweepSys).c2sRecDailyAwards
	})
	event.RegActorEventL(custom_id.AeNewDay, handleSiActSweepAeNewDay)
	event.RegSysEvent(custom_id.SeActSweepNewActSweepAdd, handleSiActSweepSeActSweepNewActivityEnd)
	RegisterSysClass(sysdef.SiActSweep, func() iface.ISystem {
		return &ActSweepSys{}
	})
	RegisterPrivilegeCalculater(handleActSweepPrivilegeCalculater)
	engine.RegisterActorCallFunc(playerfuncid.F2GActSweepGiveMergeAwardsReq, handleF2GActSweepGiveMergeAwardsReq)
	gmevent.Register("ActSweepSys.resetAll", func(player iface.IPlayer, args ...string) bool {
		if s, ok := player.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
			s.initAllSweep()
			s.calcCanUseTimes(0)
			s.s2cInfo()
		}
		return true
	}, 1)
	engine.RegisterMessage(gshare.OfflineAddActSweepDay, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineAddActSweepDay)
}

func offlineAddActSweepDay(actor iface.IPlayer, msg pb3.Message) {
	commonSt := msg.(*pb3.CommonSt)
	if commonSt == nil {
		return
	}
	if s, ok := actor.GetSysObj(sysdef.SiActSweep).(*ActSweepSys); ok && s.IsOpen() {
		data := s.getData()
		privilegeExpAt := data.PrivilegeExpAt
		nowSec := time_util.NowSec()
		var newPrivilegeExpAt uint32
		if nowSec > privilegeExpAt {
			newPrivilegeExpAt = nowSec + commonSt.U32Param*86400
		} else {
			newPrivilegeExpAt += privilegeExpAt + commonSt.U32Param*86400
		}
		data.PrivilegeExpAt = newPrivilegeExpAt
		s.s2cInfo()
		return
	}
}
