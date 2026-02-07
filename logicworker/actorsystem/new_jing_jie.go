/**
 * @Author: yzh
 * @Date:
 * @Desc: 新境界
 * @Modify：
**/

package actorsystem

import (
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
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
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"strconv"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

var (
	NewJingJieQuestTargetMap map[uint32]map[uint32]struct{} // 境界任务事件对应的id
)

const (
	AdvancedByTuPo  = 1 // 突破
	AdvancedByDuJie = 2 // 渡劫
)

func NewNewJingJieSys() iface.ISystem {
	sys := &NewJingJieSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}

	return sys
}

func onAfterReloadNewJingJieConf(_ ...interface{}) {
	tmp := make(map[uint32]map[uint32]struct{})
	if nil == jsondata.NewJingJieConfMgr {
		return
	}
	for id, quest := range jsondata.NewJingJieConfMgr.Quests {
		for _, target := range quest.Targets {
			if _, ok := tmp[target.Type]; !ok {
				tmp[target.Type] = make(map[uint32]struct{})
			}
			tmp[target.Type][id] = struct{}{}
		}
	}
	NewJingJieQuestTargetMap = tmp
}

type NewJingJieSys struct {
	tupoFlag uint32

	*QuestTargetBase
}

func (sys *NewJingJieSys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if ids, ok := NewJingJieQuestTargetMap[qt]; ok {
		return ids
	}
	return nil
}

func (sys *NewJingJieSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := sys.GetData()
	for _, quest := range data.Quests {
		if quest.GetId() == id {
			return quest
		}
	}
	return nil
}

func (sys *NewJingJieSys) CheckResetQuest() {
	data := sys.GetData()

	if len(data.Quests) > 0 {
		return
	}

	// 读下一级的任务
	conf := sys.getLevelConf(data.Level + 1)
	if nil == conf {
		return
	}

	data.Quests = make([]*pb3.QuestData, 0, len(conf.QuestIds))
	for _, id := range conf.QuestIds {
		if questConf := jsondata.GetNewJingJieQuestConfById(id); nil != questConf {
			quest := &pb3.QuestData{
				Id: id,
			}
			data.Quests = append(data.Quests, quest)
		}
	}

	for _, quest := range data.Quests {
		sys.QuestTargetBase.OnAcceptQuest(quest)
	}

	sys.SendProto3(2, 23, &pb3.S2C_2_23{Quests: data.Quests})
}

func (sys *NewJingJieSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	if conf := jsondata.GetNewJingJieQuestConfById(id); nil != conf {
		return conf.Targets
	}
	return nil
}

func (sys *NewJingJieSys) onUpdateTargetData(questId uint32) {
	quest := sys.getUnFinishQuestData(questId)
	if nil == quest {
		return
	}

	//下发新增任务
	sys.SendProto3(2, 22, &pb3.S2C_2_22{Quest: quest})

}

func (sys *NewJingJieSys) GetData() *pb3.NewJingJieState {
	data := sys.GetBinaryData()
	if nil == data.NewJingJieState {
		data.NewJingJieState = &pb3.NewJingJieState{}
	}

	if data.NewJingJieState.LastXlTimeAt == 0 {
		data.NewJingJieState.LastXlTimeAt = time_util.NowSec()
	}
	if nil == data.NewJingJieState.SkillInfo {
		data.NewJingJieState.SkillInfo = make(map[uint32]uint32)
	}
	return data.NewJingJieState
}

func (sys *NewJingJieSys) OnOpen() {
	data := sys.GetData()
	data.LastXlTimeAt = time_util.NowSec()
	sys.GetOwner().UpdateStatics(model.FieldJingJieLv_, data.Level)
	sys.s2cInfo(nil)
}

func (sys *NewJingJieSys) OnReconnect() {
	sys.ResetSysAttr(attrdef.SaNewJingJie)
	sys.s2cInfo(nil)
}

func (sys *NewJingJieSys) OnLogin() {
	if !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	sys.owner.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Level))
	sys.supplementOfflineLingQi()
	sys.CheckResetQuest()
	sys.s2cInfo(nil)
}

func (sys *NewJingJieSys) supplementOfflineLingQi() {
	data := sys.GetData()

	if data.LastRevLingQiAt == 0 {
		return
	}

	conf := jsondata.NewJingJieConfMgr
	if nil == conf {
		sys.GetOwner().LogWarn("not found new j j conf")
		return
	}

	// 时间间隔
	sec := conf.AddLingQiSec
	if sec <= 0 {
		sec = 1
		sys.GetOwner().LogWarn("add ling qi sec has err ......")
	}

	offlinePass := time_util.NowSec() - data.LastRevLingQiAt
	maxValidPass := uint32(24 * 3600 * 2)
	if offlinePass > maxValidPass {
		offlinePass = maxValidPass
	}
	newAdd := sys.getNewAddLingQiPerTime() * uint64(offlinePass) / uint64(sec)
	sys.addLingQi(newAdd, pb3.LogId_LogNewJingJieGetLingQi)
}

func (sys *NewJingJieSys) getNewAddLingQiPerTime() uint64 {
	data := sys.GetData()
	lvConf := sys.getLevelConf(data.Level)
	var (
		multiple uint64
		openDay  = gshare.GetOpenServerDay()
	)
	for _, fenQi := range lvConf.FenQis {
		if len(fenQi.OpenSrvDays) > 1 {
			if openDay >= fenQi.OpenSrvDays[0] && openDay <= fenQi.OpenSrvDays[1] {
				multiple = fenQi.LingQiAddMutiple
				break
			}
			if openDay == fenQi.OpenSrvDays[0] {
				multiple = fenQi.LingQiAddMutiple
				break
			}
		}
	}
	addRate := uint64(sys.owner.GetFightAttr(attrdef.LingQiAddRate))
	lingQi := lvConf.AddLingQiBasic * (10000 + addRate) * uint64(utils.Max(int64(multiple), 10000)) / 100000000
	sys.GetOwner().LogTrace("actor[%d] add ling qi:%d, nowSec:%d", sys.GetOwner().GetId(), lingQi, time_util.NowSec())
	return lingQi
}

func (sys *NewJingJieSys) addLingQi(lingQi uint64, logId pb3.LogId) bool {
	data := sys.GetData()
	data.TodayGotLingQi += lingQi
	data.LastRevLingQiAt = time_util.NowSec()
	sys.owner.AddMoney(moneydef.LingQi, int64(lingQi), false, logId)
	return true
}

func (sys *NewJingJieSys) s2cInfo(_ *base.Message) {
	sys.SendProto3(153, 1, &pb3.S2C_153_1{Data: sys.GetData()})
}

func (sys *NewJingJieSys) LogPlayerBehavior(coreNumData uint64, argsMap map[string]interface{}, logId pb3.LogId) {
	bytes, err := json.Marshal(argsMap)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(sys.GetOwner(), logId, &pb3.LogPlayerCounter{
		NumArgs: coreNumData,
		StrArgs: string(bytes),
	})
}

func (sys *NewJingJieSys) c2sActive(_ *base.Message) {
	data := sys.GetData()
	sys.LogPlayerBehavior(0, map[string]interface{}{}, pb3.LogId_LogNewJingJieActive)
	data.Active = true
	data.Level = 1
	conf := jsondata.NewJingJieConfMgr
	sys.addLingQi(conf.InitLinQi, pb3.LogId_LogNewJingJieGetLingQi)
	sys.CheckResetQuest()
	sys.owner.TriggerQuestEventRange(custom_id.QttNewJingJieUpLv)
	sys.s2cInfo(nil)
	sys.learnNJJSkill()
}

func (sys *NewJingJieSys) learnNJJSkill() {
	mgr := jsondata.NewJingJieConfMgr
	if mgr == nil {
		return
	}
	data := sys.GetData()
	for _, conf := range mgr.SkillConf {
		if _, ok := data.SkillInfo[conf.SkillId]; ok {
			continue
		}
		if conf.MinLevel != data.Level {
			continue
		}
		sys.owner.LearnSkill(conf.SkillId, conf.Level, true)
		data.SkillInfo[conf.SkillId] = conf.Level
		sys.LogPlayerBehavior(uint64(conf.SkillId), map[string]interface{}{}, pb3.LogId_LogNewJingJieUpLevelSkill)
	}
}

func (sys *NewJingJieSys) canUpLevel(conf *jsondata.NewJingJieLevelConf) bool {
	if conf.LevelLimit > sys.owner.GetLevel() {
		return false // 等级限制
	}

	return true
}

func (sys *NewJingJieSys) getLevelConf(lv uint32) *jsondata.NewJingJieLevelConf {
	mgr := jsondata.NewJingJieConfMgr
	if nil == mgr {
		return nil
	}
	if lv >= uint32(len(mgr.LevelConf)) {
		return nil // 满级
	}
	return mgr.LevelConf[lv]
}

func (sys *NewJingJieSys) CheckFinishAllQuest() bool {
	data := sys.GetData()

	if len(data.Quests) <= 0 {
		return true
	}

	for _, quest := range data.Quests {
		if !sys.CheckFinishQuest(quest) {
			return false
		}
	}

	return true
}

func (sys *NewJingJieSys) c2sUpLevel(_ *base.Message) {
	var next uint32
	data := sys.GetData()
	oldLv := data.GetLevel()
	curLvConf := sys.getLevelConf(oldLv)
	next = data.GetLevel() + 1
	conf := sys.getLevelConf(next)
	if nil == conf {
		sys.GetOwner().LogWarn("conf is nil")
		return
	}

	if !sys.canUpLevel(conf) {
		sys.GetOwner().LogWarn("can't not up lv")
		return
	}

	nowSec := time_util.NowSec()

	if data.GetRecoverTime() > nowSec {
		sys.GetOwner().LogWarn("wait recover")
		return // 还在受损状态
	}

	if conf.AdvancedType != AdvancedByTuPo {
		sys.GetOwner().LogWarn("wrong up lv type, type:%d", conf.AdvancedType)
		return // 不能通过突破方式升级
	}

	// 开心魔系统才校验
	obj := sys.GetOwner().GetSysObj(sysdef.SiHeartEvil)
	if obj != nil && obj.IsOpen() {
		if curLvConf != nil && curLvConf.HeartEvilEvent != nil && !sys.GetData().IsPassHeartEvilEvent {
			sys.GetOwner().LogWarn("not pass heart evil event")
			return
		}
	}

	if !sys.CheckFinishAllQuest() {
		sys.GetOwner().LogWarn("sys.CheckFinishAllQuest() failed")
		return
	}

	if !sys.owner.CheckConsumeByConf(conf.Consume, false, 0) {
		sys.GetOwner().LogWarn("not enough consume")
		return
	}

	ret := sys.doUpLevelByTuPo(data.LoseTimes, conf)
	data.ApocalypseSuccRateUsedCnt = 0
	if !ret {
		sys.GetOwner().LogWarn("tu po failed")
		data.LoseTimes++
		data.RecoverTime = nowSec + conf.LostTime
		sys.GetOwner().TriggerQuestEvent(custom_id.QttNewJingJieUpLevelOrApocalypseFailed, 0, 1)
		sys.owner.TriggerQuestEvent(custom_id.QttNewJJUpLevelOrApocalypseFailedLv, data.Level, int64(data.Level))
		sys.LogPlayerBehavior(uint64(data.Level), map[string]interface{}{}, pb3.LogId_LogNewJingJieApocalypseFailed)
	} else {
		if !sys.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogNewJingJieTuPo}) {
			return
		}
		data.Level = next
		data.LoseTimes = 0
		data.ShieldItems = nil
		sys.owner.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Level))
		sys.owner.TriggerQuestEvent(custom_id.QttCircle, 0, int64(data.Level))
		sys.owner.TriggerEvent(custom_id.AeCircleChange, oldLv, data.Level)
		sys.ResetSysAttr(attrdef.SaNewJingJie)
		curLvConf := sys.getLevelConf(data.Level)
		preLvConf := sys.getLevelConf(data.Level - 1)
		if curLvConf.Stage != preLvConf.Stage {
			data.LastRevExtraGiftLingQiAt = 0
			data.EatLingQiDrugCnt = 0
		}
		sys.owner.SetRankValue(gshare.RankTypeBoundary, int64(data.Level))
		manager.GRankMgrIns.UpdateRank(gshare.RankTypeBoundary, sys.owner.GetId(), int64(data.Level))
		if curLvConf.WhetherBroadcast > 0 {
			sys.owner.SendTipMsg(tipmsgid.TpXianjieUpTip, sys.owner.GetId(), sys.owner.GetName(), curLvConf.Level, sys.owner.GetFlyCamp())
		}
		sys.GetData().IsPassHeartEvilEvent = false
		sys.GetOwner().UpdateStatics(model.FieldJingJieLv_, data.Level)
		sys.LogPlayerBehavior(uint64(data.Level), map[string]interface{}{}, pb3.LogId_LogNewJingJieUpLevel)
	}
	sys.owner.TriggerQuestEventRange(custom_id.QttNewJingJieUpLv)
	sys.SendProto3(153, 2, &pb3.S2C_153_2{
		Ret:         ret,
		Level:       data.Level,
		LoseTimes:   data.LoseTimes,
		RecoverTime: data.RecoverTime,
	})

	// 突破成功才刷下一批任务
	if ret && nil != sys.getLevelConf(data.Level+1) {
		data.Quests = nil
		sys.learnNJJSkill()
		sys.CheckResetQuest()
	}
}

func (sys *NewJingJieSys) calcUpLevelByTuPoSuccRate(loseTimes uint32, lvConf *jsondata.NewJingJieLevelConf) uint32 {
	var addSuccRate uint32
	data := sys.GetData()
	if lvConf.SuccRateItemId > 0 && data.ApocalypseSuccRateUsedCnt > 0 {
		itemUseConf := jsondata.GetUseItemConfById(lvConf.SuccRateItemId)
		addSuccRate = data.ApocalypseSuccRateUsedCnt * itemUseConf.Param[0]
	}

	privilegeAdd, _ := sys.owner.GetPrivilege(privilegedef.EnumJingJieBreakRatio)

	rate := lvConf.SuccessRate + lvConf.LoseRate*loseTimes + addSuccRate + uint32(privilegeAdd)

	if rate >= 10000 {
		return 10000
	}

	return rate
}

func (sys *NewJingJieSys) doUpLevelByTuPo(loseTimes uint32, lvConf *jsondata.NewJingJieLevelConf) bool {
	rate := sys.calcUpLevelByTuPoSuccRate(loseTimes, lvConf)

	if rate >= 10000 {
		return true
	}

	if !random.Hit(rate, 10000) {
		return false
	}

	return true
}

// 计算雷劫伤害
func (sys *NewJingJieSys) calcApocalypseDamage(conf *jsondata.NewJingJieLevelConf) (int64, error) {
	if nil == conf || jsondata.NewJingJieConfMgr == nil {
		return 0, neterror.ConfNotFoundError("not found conf")
	}
	confMgr := jsondata.NewJingJieConfMgr
	data := sys.GetData()
	rate := int64(0)
	if data.Zq > 0 {
		if data.Zq > uint64(conf.ApocalypseAttack) {
			data.Zq -= uint64(conf.ApocalypseAttack)
		} else {
			data.Zq = 0
		}
		rate = sys.owner.GetFightAttr(attrdef.DefZhenYuan)
	}
	for _, itemId := range data.ShieldItems {
		if itemConf := jsondata.GetUseItemConfById(itemId); nil != itemConf {
			if len(itemConf.Param) > 0 {
				rate += int64(itemConf.Param[0])
			}
		}
	}
	if rate > 10000 {
		rate = 10000
	}
	damage := int64(conf.ApocalypseHurt) * (10000 - rate) / 10000
	var factor int64
	if len(confMgr.ApocalypseFactor) > int(data.ApocalypseTimes) {
		factor = int64(confMgr.ApocalypseFactor[data.ApocalypseTimes])
	}
	if factor > 0 {
		damage *= factor
	}
	damage = damage / 10000
	data.ApocalypseTimes++
	if data.Hp > uint64(damage) {
		data.Hp -= uint64(damage)
	} else {
		damage = int64(data.Hp)
		data.Hp = 0
	}
	return damage, nil
}

func (sys *NewJingJieSys) doApocalypse() {
	confMgr := jsondata.NewJingJieConfMgr
	if nil == confMgr {
		return
	}
	data := sys.GetData()
	next := data.GetLevel() + 1
	conf := sys.getLevelConf(next)
	if nil == conf {
		return
	}
	rate := int64(0)
	if data.Zq > 0 {
		if data.Zq > uint64(conf.ApocalypseAttack) {
			data.Zq -= uint64(conf.ApocalypseAttack)
		} else {
			data.Zq = 0
		}
		rate = sys.owner.GetFightAttr(attrdef.DefZhenYuan)
	}

	for _, itemId := range data.ShieldItems {
		if itemConf := jsondata.GetUseItemConfById(itemId); nil != itemConf {
			if len(itemConf.Param) > 0 {
				rate += int64(itemConf.Param[0])
			}
		}
	}
	if rate > 10000 {
		rate = 10000
	}

	damage := int64(conf.ApocalypseHurt) * (10000 - rate) / 10000

	var factor int64
	if len(confMgr.ApocalypseFactor) > int(data.ApocalypseTimes) {
		factor = int64(confMgr.ApocalypseFactor[data.ApocalypseTimes])
	}

	if factor > 0 {
		damage *= factor
	}
	damage = damage / 10000

	data.ApocalypseTimes++
	if data.Hp > uint64(damage) {
		data.Hp -= uint64(damage)
	} else {
		data.Hp = 0
	}

	sys.SendProto3(153, 4, &pb3.S2C_153_4{
		Hp:              data.Hp,
		Zq:              data.Zq,
		ApocalypseTimes: data.ApocalypseTimes,
	})

}

func (sys *NewJingJieSys) c2sApocalypse(msg *base.Message) {
	var st pb3.C2S_153_4
	if nil != msg.UnPackPb3Msg(&st) {
		return
	}

	data := sys.GetData()
	curLvConf := sys.getLevelConf(data.GetLevel())
	next := data.GetLevel() + 1

	conf := sys.getLevelConf(next)
	if nil == conf {
		return
	}

	if !sys.canUpLevel(conf) {
		return
	}

	if conf.AdvancedType != AdvancedByDuJie {
		return // 不能通过渡劫方式升级
	}

	// 开心魔系统才校验
	obj := sys.GetOwner().GetSysObj(sysdef.SiHeartEvil)
	if obj != nil && obj.IsOpen() {
		if curLvConf != nil && curLvConf.HeartEvilEvent != nil && !sys.GetData().IsPassHeartEvilEvent {
			sys.GetOwner().LogWarn("not pass heart evil event")
			return
		}
	}

	nowSec := time_util.NowSec()

	if data.GetRecoverTime() > nowSec {
		return // 还在受损状态
	}

	if data.ApocalypseTimes > conf.ApocalypseTimes {
		return
	}

	if st.Prepare && data.ApocalypseTimes != 0 {
		return
	}

	if !sys.owner.CheckConsumeByConf(conf.Consume, false, 0) {
		return
	}

	err := sys.StartApocalypseV2(conf)
	if err != nil {
		sys.owner.LogError("err:%v", err)
		return
	}
}

func (sys *NewJingJieSys) StartApocalypse(flag uint32, st *pb3.CommonSt) {
	if sys.tupoFlag != flag {
		return
	}
	data := sys.GetData()
	data.Hp = st.U64Param
	data.Zq = st.U64Param2
	data.MaxHp = st.U64Param
	data.MaxZq = st.U64Param2

	sys.SendProto3(153, 4, &pb3.S2C_153_4{
		Hp:              data.Hp,
		Zq:              data.Zq,
		ApocalypseTimes: data.ApocalypseTimes,
	})
}

func (sys *NewJingJieSys) StartApocalypseV2(lvConf *jsondata.NewJingJieLevelConf) error {
	if lvConf == nil {
		return neterror.ConfNotFoundError("not found conf")
	}
	owner := sys.GetOwner()
	data := sys.GetData()
	data.Hp = uint64(owner.GetFightAttr(attrdef.MaxHp))
	data.Zq = uint64(owner.GetFightAttr(attrdef.MaxZhenYuan))
	data.MaxHp = data.Hp
	data.MaxZq = data.Zq
	defer func() {
		data.Hp = 0
		data.Zq = 0
		data.MaxHp = 0
		data.MaxZq = 0
		data.ApocalypseTimes = 0
	}()

	var resp = &pb3.S2C_153_11{
		Hp:              data.Hp,
		Zq:              data.Zq,
		ApocalypseTimes: 0,
		ApocalypseMap:   make(map[uint32]int64),
		Level:           data.Level,
	}
	for i := uint32(1); i <= lvConf.ApocalypseTimes; i++ {
		damage, err := sys.calcApocalypseDamage(lvConf)
		if err != nil {
			return neterror.Wrap(err)
		}
		resp.ApocalypseMap[i] = damage
		resp.ApocalypseTimes += 1
		if data.Hp == 0 {
			break
		}
	}

	resp.Success = data.Hp != 0
	if !resp.Success {
		data.RecoverTime = time_util.NowSec() + lvConf.LostTime
		resp.RecoverTime = data.RecoverTime
		data.LoseTimes += 1
		resp.LoseTimes = data.LoseTimes
		owner.SendProto3(153, 11, resp)
		sys.GetOwner().TriggerQuestEvent(custom_id.QttNewJingJieUpLevelOrApocalypseFailed, 0, 1)
		sys.owner.TriggerQuestEvent(custom_id.QttNewJJUpLevelOrApocalypseFailedLv, data.Level, int64(data.Level))
		return nil
	}

	// 开始消耗
	if !sys.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogNewJingJieTuPo}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	oldLv := data.Level
	data.Level = lvConf.Level
	resp.Level = data.Level
	owner.SendProto3(153, 11, resp)
	data.Quests = nil
	sys.CheckResetQuest()
	data.LoseTimes = 0
	data.ShieldItems = nil
	sys.owner.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Level))
	sys.owner.TriggerQuestEvent(custom_id.QttCircle, 0, int64(data.Level))
	sys.owner.TriggerEvent(custom_id.AeCircleChange, oldLv, data.Level)
	sys.ResetSysAttr(attrdef.SaNewJingJie)
	sys.owner.TriggerQuestEventRange(custom_id.QttNewJingJieUpLv)
	sys.owner.SetRankValue(gshare.RankTypeBoundary, int64(data.Level))
	manager.GRankMgrIns.UpdateRank(gshare.RankTypeBoundary, sys.owner.GetId(), int64(data.Level))
	curLvConf := sys.getLevelConf(data.Level)
	preLvConf := sys.getLevelConf(data.Level - 1)
	if curLvConf.Stage != preLvConf.Stage {
		data.LastRevExtraGiftLingQiAt = 0
		data.EatLingQiDrugCnt = 0
	}
	if curLvConf.WhetherBroadcast > 0 {
		sys.owner.SendTipMsg(tipmsgid.TpXianjieUpTip, sys.owner.GetId(), sys.owner.GetName(), curLvConf.Level, sys.owner.GetFlyCamp())
	}
	sys.learnNJJSkill()

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogNewJingJieTuPo, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.Level),
	})
	return nil
}

func (sys *NewJingJieSys) c2sGetExtraGiftLingQi(_ *base.Message) {
	data := sys.GetData()
	if time_util.IsSameDay(data.LastRevExtraGiftLingQiAt, time_util.NowSec()) {
		sys.GetOwner().LogWarn("LastRevExtraGiftLingQiAt is today")
		return
	}

	data.LastRevExtraGiftLingQiAt = time_util.NowSec()
	conf := sys.getLevelConf(data.Level)

	if len(conf.ExtraGiftValues) == 0 {
		sys.GetOwner().LogWarn("extra gift value is nil")
		return
	}

	engine.GiveRewards(sys.owner, conf.ExtraGiftValues, common.EngineGiveRewardParam{LogId: pb3.LogId_LogNewJingJieGetLingQi})

	sys.s2cInfo(nil)
	sys.SendProto3(153, 9, &pb3.S2C_153_9{})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogNewJingJieGetLingQi, &pb3.LogPlayerCounter{})
}

func (sys *NewJingJieSys) c2sUpLevelSkill(msg *base.Message) {
	var req pb3.C2S_153_7
	if nil != msg.UnPackPb3Msg(&req) {
		return
	}
	data := sys.GetData()

	_, ok := data.SkillInfo[req.SkillId]
	if ok {
		return
	}

	skillInfo := jsondata.GetNewJingJieSkillConf(req.SkillId)
	if skillInfo == nil {
		return
	}

	sys.LogPlayerBehavior(uint64(req.SkillId), map[string]interface{}{}, pb3.LogId_LogNewJingJieUpLevelSkill)
	data.SkillInfo[req.SkillId] = skillInfo.Level

	sys.owner.LearnSkill(req.SkillId, skillInfo.Level, true)

	sys.SendProto3(153, 7, &pb3.S2C_153_7{SkillId: req.SkillId, Level: skillInfo.Level})
}

func (sys *NewJingJieSys) UseShieldItem(itemId uint32) bool {
	data := sys.GetData()

	next := data.GetLevel() + 1
	conf := sys.getLevelConf(next)
	if conf.AdvancedType != AdvancedByDuJie {
		return false
	}

	for _, id := range data.ShieldItems {
		if id == itemId {
			return false // 已使用过
		}
	}

	data.ShieldItems = append(data.ShieldItems, itemId)
	sys.SendProto3(153, 5, &pb3.S2C_153_5{UseItemId: data.ShieldItems})

	return true
}

func (sys *NewJingJieSys) handleNewJingJieXiuLianPush(args ...interface{}) {
	if !sys.IsOpen() {
		return
	}
	if len(args) < 1 {
		sys.GetOwner().LogWarn("handle new jing jie xiu lian push , args must be 1 , now sec %d", time_util.NowSec())
		return
	}
	var timeSec = utils.AtoUint32(fmt.Sprintf("%v", args[0]))

	conf := jsondata.NewJingJieConfMgr
	if nil == conf {
		sys.GetOwner().LogWarn("not found new j j conf")
		return
	}

	if sys.GetData().LastXlTimeAt > timeSec {
		sys.GetOwner().LogTrace("last xl time %d , now is %d", sys.GetData().LastXlTimeAt, timeSec)
		return
	}

	if sys.GetData().LastXlTimeAt+uint32(conf.AddLingQiSec) > timeSec {
		sys.GetOwner().LogTrace("next xl time %d , now is %d", sys.GetData().LastXlTimeAt+uint32(conf.AddLingQiSec), timeSec)
		return
	}

	if !sys.GetOwner().GetSysOpen(sysdef.SiSpiritRoot) {
		sys.GetOwner().LogTrace("SiSpiritRoot not open")
		return
	}

	rootSys, ok := sys.GetOwner().GetSysObj(sysdef.SiSpiritRoot).(*SpiritRootSys)
	if !ok || len(rootSys.GetData().SrLvMap) <= 0 {
		return
	}

	val := sys.getNewAddLingQiPerTime()
	sys.addLingQi(val, pb3.LogId_LogNewJingJieGetLingQi)

	sys.GetOwner().SendProto3(153, 10, &pb3.S2C_153_10{
		LingQi: val,
	})

	sys.GetData().LastXlTimeAt = timeSec
}

func (sys *NewJingJieSys) handlePassHeartEvilEvent(_ ...interface{}) {
	sys.GetData().IsPassHeartEvilEvent = true
	sys.SendProto3(153, 32, &pb3.S2C_153_32{
		IsPassHeartEvilEvent: true,
		JjLv:                 sys.GetData().Level,
	})
}

func handleUseItemNewJingJieRecover(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("xiu zhen recover useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return
	}

	logNewJingjieUseItem(player, param, conf, pb3.LogId_LogNewJingJieUseItemRecover)

	nowSec := time_util.NowSec()

	data := sys.GetData()
	if data.RecoverTime <= nowSec {
		return false, false, 0
	}

	deduct := uint32(param.Count) * conf.Param[0]
	if data.RecoverTime >= deduct {
		data.RecoverTime -= deduct
	} else {
		data.RecoverTime = 0
	}

	sys.SendProto3(153, 8, &pb3.S2C_153_8{RecoverTime: data.RecoverTime})

	return true, true, param.Count
}

func logNewJingjieUseItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf, logId pb3.LogId) {
	bytes, _ := json.Marshal(map[string]interface{}{
		"UseItemParamSt":   param,
		"BasicUseItemConf": conf,
	})
	logworker.LogPlayerBehavior(player, logId, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})
}

func useNewJingJieShieldItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return
	}

	logNewJingjieUseItem(player, param, conf, pb3.LogId_LogNewJingJieUseItemShield)

	if sys.UseShieldItem(param.ItemId) {
		return true, true, 1
	}
	return
}

func useNewJingJieAddSuccRateItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()

	// 不想在这个类型冗余一个减受损时间的,但是策划期望一个丹根据修炼不同状态来选择使用条件,没说服成功
	if data.RecoverTime > time_util.NowSec() {
		if len(conf.Param) != 2 {
			return false, false, 0
		}
		itemConf := conf.Copy()
		itemConf.Param = itemConf.Param[1:]
		return handleUseItemNewJingJieRecover(player, param, itemConf)
	}

	lvConf := sys.getLevelConf(data.Level + 1)
	if lvConf.SuccRateItemId != param.ItemId {
		sys.GetOwner().LogWarn("item id wrong, expect %d, given %d", lvConf.SuccRateItemId, param.ItemId)
		return
	}

	rate := sys.calcUpLevelByTuPoSuccRate(sys.GetData().LoseTimes, lvConf)
	if rate >= 10000 {
		sys.GetOwner().LogWarn("already over 100% success rate")
		return
	}

	logNewJingjieUseItem(player, param, conf, pb3.LogId_LogNewJingJieUseItemSuccRate)

	data.ApocalypseSuccRateUsedCnt += uint32(param.Count)

	success = true
	del = true
	cnt = param.Count

	sys.s2cInfo(nil)
	return
}

func useNewJingJieAddZhenQiItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return
	}

	data := sys.GetData()
	if !time_util.IsSameDay(data.LastEatLingQiDrugAt, time_util.NowSec()) {
		data.EatLingQiDrugCnt = 0
		data.EatLingQiDrugAddUpLimit = 0
	}

	itemConf := jsondata.GetItemConfig(param.ItemId)
	if nil == itemConf {
		return
	}

	lvConf := sys.getLevelConf(data.Level)
	// 丹药使用次数上限校验
	var lingQiDrugEatLimit uint32
	{
		total, _ := player.GetPrivilege(privilegedef.EnumDailyEatDrugCount)
		lingQiDrugEatLimit = lvConf.LingQiDrugEatLimit + uint32(total) + uint32(player.GetFightAttr(attrdef.DailyEatDrugCount))
		if lingQiDrugEatLimit > 0 && lingQiDrugEatLimit <= data.EatLingQiDrugCnt && itemConf.Quality < 6 {
			sys.GetOwner().LogError("EatLingQiDrugCnt over max")
			return
		}
	}

	// 加灵气上限校验
	var lingQiDrugAddUpLimit int64
	{
		rate, _ := player.GetPrivilege(privilegedef.EnumDailyEatDrugAddUpLimitRate)
		rate = player.GetFightAttr(attrdef.DailyEatDrugAddUpLimitRate) + rate
		upLimit := lvConf.LingQiDrugGiveLingQiLimit + player.GetFightAttr(attrdef.DailyEatDrugAddUpLimit)
		lingQiDrugAddUpLimit = (upLimit) * (10000 + rate) / 10000
		if lingQiDrugAddUpLimit > 0 && lingQiDrugAddUpLimit <= data.EatLingQiDrugAddUpLimit && itemConf.Quality < 6 {
			sys.GetOwner().LogError("EnumDailyEatDrugAddUpLimit over max")
			return
		}
	}

	giftConf := jsondata.GetRandGiftConf(param.ItemId)
	if nil == giftConf {
		return
	}

	logNewJingjieUseItem(player, param, conf, pb3.LogId_LogNewJingJieUseItemZhenQi)

	attr := sys.GetOwner().GetFightAttr(attrdef.LingQiElixir)
	var privilegeAdd int64
	if itemConf.Quality == itemdef.ItemQualityOrange {
		privilegeAdd, _ = sys.GetOwner().GetPrivilege(privilegedef.EnumOrAlchemyAddCircleRatio)
	}
	addRatio := attr + privilegeAdd
	lingQiBonus := 1 + float64(addRatio)/10000
	var vec jsondata.StdRewardVec
	var totalLingQi float64
	for _, giftRewards := range giftConf.Rewards {
		for _, randGiftReward := range giftRewards {
			addLingQi := float64(randGiftReward.StdReward.Count) * float64(param.Count)
			totalLingQi += addLingQi
			count := addLingQi * lingQiBonus
			vec = append(vec, &jsondata.StdReward{
				Id:    randGiftReward.StdReward.Id,
				Job:   randGiftReward.StdReward.Job,
				Count: int64(count),
				Bind:  randGiftReward.StdReward.Bind,
			})
		}
	}

	if len(vec) > 0 {
		success = true
		del = true
		cnt = param.Count
	}
	if success && itemConf.Quality < 6 {
		if lingQiDrugEatLimit > 0 {
			data.EatLingQiDrugCnt++
		}

		if lingQiDrugAddUpLimit > 0 {
			data.EatLingQiDrugAddUpLimit += int64(totalLingQi)
			if data.EatLingQiDrugAddUpLimit > lingQiDrugAddUpLimit {
				data.EatLingQiDrugAddUpLimit = lingQiDrugAddUpLimit
			}
		}

		data.LastEatLingQiDrugAt = time_util.NowSec()
	}

	if success {
		engine.GiveRewards(sys.GetOwner(), vec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLingQiDanUse})
	}
	player.TriggerQuestEvent(custom_id.QttUseNewJingJieAddZhenQiItem, 0, cnt)
	sys.s2cInfo(nil)
	return
}

func startNewJingJieApocalypse(player iface.IPlayer, buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok {
		return
	}
	sys.StartApocalypse(msg.U32Param, msg)
}

func calcNewJingJieAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok {
		return
	}
	level := sys.GetData().Level
	if conf := sys.getLevelConf(level); nil != conf {
		engine.CheckAddAttrsToCalc(player, calc, conf.Attrs)
		if len(conf.ExaAttrs) > 0 {
			engine.CheckAddAttrsToCalc(player, calc, conf.ExaAttrs)
		}
	}

	//境界等级特性属性
	mgr := jsondata.NewJingJieConfMgr
	if mgr == nil || len(mgr.LvFeatureConf) == 0 {
		return
	}
	for _, featureConf := range mgr.LvFeatureConf {
		if featureConf.Level > level {
			continue
		}
		engine.CheckAddAttrsToCalc(player, calc, featureConf.Attrs)
	}
}

// 修真等级达到x级
func QuestNewJingJieLv(actor iface.IPlayer, _ []uint32, _ ...interface{}) uint32 {
	if sys, ok := actor.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys); ok {
		data := sys.GetData()
		return data.Level
	}
	return 0
}

func handleNJJNewDay(player iface.IPlayer, args ...interface{}) {
	sys := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if sys != nil && !sys.IsOpen() {
		return
	}
	sys.GetData().TodayGotLingQi = 0
	sys.GetData().EatLingQiDrugCnt = 0
	sys.GetData().EatLingQiDrugAddUpLimit = 0
	sys.s2cInfo(nil)
}

func handleNJJNewJingJieXiuLianPush(player iface.IPlayer, args ...interface{}) {
	sys := player.GetSysObj(sysdef.SiNewJingJie)
	if sys != nil && !sys.IsOpen() {
		return
	}
	jieSys, ok := sys.(*NewJingJieSys)
	if !ok {
		return
	}
	jieSys.handleNewJingJieXiuLianPush(args...)
}

func handleNJJPassHeartEvilEvent(player iface.IPlayer, args ...interface{}) {
	sys := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if sys != nil && !sys.IsOpen() {
		return
	}
	sys.handlePassHeartEvilEvent(args...)
}

func handleOfflineFixNewJingJieTuPo(actor iface.IPlayer, msg pb3.Message) {
	sys := actor.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if sys != nil && !sys.IsOpen() {
		return
	}
	sys.GetData().ApocalypseTimes = 0
	sys.s2cInfo(nil)
}

func init() {
	RegisterSysClass(sysdef.SiNewJingJie, func() iface.ISystem {
		return NewNewJingJieSys()
	})
	event.RegSysEvent(custom_id.SeReloadJson, onAfterReloadNewJingJieConf)
	engine.RegQuestTargetProgress(custom_id.QttCircle, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		return actor.GetCircle()
	})
	event.RegActorEvent(custom_id.AeNewDay, handleNJJNewDay)
	event.RegActorEvent(custom_id.AeNewJingJieXiuLianPush, handleNJJNewJingJieXiuLianPush)
	event.RegActorEvent(custom_id.AePassHeartEvilEvent, handleNJJPassHeartEvilEvent)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemNewJingJieRecover, handleUseItemNewJingJieRecover)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemNewJingJieShield, useNewJingJieShieldItem)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemZhenQi, useNewJingJieAddZhenQiItem)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemNewJingJieSuccRateItem, useNewJingJieAddSuccRateItem)
	net.RegisterSysProto(153, 1, sysdef.SiNewJingJie, (*NewJingJieSys).s2cInfo)
	net.RegisterSysProto(153, 2, sysdef.SiNewJingJie, (*NewJingJieSys).c2sUpLevel)
	net.RegisterSysProto(153, 4, sysdef.SiNewJingJie, (*NewJingJieSys).c2sApocalypse)
	net.RegisterSysProto(153, 6, sysdef.SiNewJingJie, (*NewJingJieSys).c2sGetExtraGiftLingQi)
	net.RegisterSysProto(153, 7, sysdef.SiNewJingJie, (*NewJingJieSys).c2sUpLevelSkill)
	net.RegisterSysProto(153, 8, sysdef.SiNewJingJie, (*NewJingJieSys).c2sActive)

	engine.RegisterActorCallFunc(playerfuncid.Apocalypse, startNewJingJieApocalypse)
	engine.RegAttrCalcFn(attrdef.SaNewJingJie, calcNewJingJieAttr)
	engine.RegQuestTargetProgress(custom_id.QttNewJingJieUpLv, QuestNewJingJieLv)
	engine.RegisterMessage(gshare.OfflineFixNewJingJieTuPo, func() pb3.Message {
		return &pb3.CommonSt{}
	}, handleOfflineFixNewJingJieTuPo)
	initNewJingJieGm()
}

func initNewJingJieGm() {
	gmevent.Register("newjj.active", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		sys.c2sActive(nil)
		return true
	}, 1)

	gmevent.Register("newjj.revExtraZq", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		sys.c2sGetExtraGiftLingQi(nil)
		return true
	}, 1)

	gmevent.Register("newjj.upLv", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		sys.c2sUpLevel(nil)
		return true
	}, 1)

	gmevent.Register("newjj.apocalypse", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		msg := base.NewMessage()
		msg.PackPb3Msg(&pb3.C2S_153_4{
			Prepare: true,
		})
		sys.c2sApocalypse(nil)
		return true
	}, 1)

	gmevent.Register("newjj.addLingQi", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}

		lingQi, err := strconv.Atoi(args[0])
		if err != nil {
			sys.GetOwner().LogError(err.Error())
			return false
		}
		sys.addLingQi(uint64(lingQi), pb3.LogId_LogNewJingJieGetLingQi)
		return true
	}, 1)

	gmevent.Register("newxzLevel", func(player iface.IPlayer, args ...string) bool {
		var level uint32
		if len(args) > 0 {
			level = utils.AtoUint32(args[0])
		}

		if level <= 0 {
			level = 1
		}

		sys, ok := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		data := sys.GetData()
		oldLv := data.GetLevel()
		data.Level = oldLv + level

		sys.owner.SetExtraAttr(attrdef.Circle, attrdef.AttrValueAlias(data.Level))
		sys.owner.TriggerQuestEvent(custom_id.QttCircle, 0, int64(data.Level))
		sys.owner.TriggerQuestEventRange(custom_id.QttNewJingJieUpLv)

		sys.owner.TriggerEvent(custom_id.AeCircleChange, oldLv, data.Level)
		sys.ResetSysAttr(attrdef.SaNewJingJie)

		sys.SendProto3(153, 2, &pb3.S2C_153_2{
			Ret:         true,
			Level:       data.Level,
			LoseTimes:   data.LoseTimes,
			RecoverTime: data.RecoverTime,
		})
		return true
	}, 1)

	gmevent.Register("newjj.task", func(actor iface.IPlayer, args ...string) bool {
		if len(args) <= 0 {
			return false
		}
		sys, ok := actor.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
		if !ok {
			return false
		}
		boundary := sys.GetBinaryData().NewJingJieState
		if nil == boundary {
			return false
		}
		taskId := utils.AtoUint32(args[0])
		for _, quest := range boundary.Quests {
			if quest.Id == taskId {
				sys.GmFinishQuest(quest)
			}
		}
		sys.handlePassHeartEvilEvent()
		return true
	}, 1)
}
