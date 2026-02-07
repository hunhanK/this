/**
 * @Author: beiming
 * @Desc: 资源找回
 * @Date: 2023/12/13
 */

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/retrievalmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"sort"

	"github.com/gzjjyz/srvlib/utils"
)

func init() {
	RegisterSysClass(sysdef.SiRetrieval, newRetrievalSystem)
	event.RegActorEvent(custom_id.AeNewDay, retrievalOnNewDay)
	event.RegActorEvent(custom_id.AeCompleteRetrieval, retrievalOnComplete)

	net.RegisterSysProtoV2(170, 2, sysdef.SiRetrieval, func(s iface.ISystem) func(*base.Message) error {
		return s.(*Retrieval).c2sRetrievalOnce
	})
	net.RegisterSysProtoV2(170, 3, sysdef.SiRetrieval, func(s iface.ISystem) func(*base.Message) error {
		return s.(*Retrieval).c2sRetrievalAll
	})

	// 注册gm命令，给玩家添加资源找回次数
	// retry.count,129,2 新增给玩家增加找回次数的gm命令。129表示系统id(0表示给所有系统增加)，
	// 2表示增加次数(0将增加每日配置上限)。 直接 retry.count 就会给所有系统，增加配置上限次数
	gmevent.Register("retry.count", func(player iface.IPlayer, args ...string) bool {
		var sysId, count uint32
		if len(args) > 0 {
			sysId = utils.AtoUint32(args[0])
		}
		if len(args) > 1 {
			count = utils.AtoUint32(args[1])
		}

		s := player.GetSysObj(sysdef.SiRetrieval).(*Retrieval)

		return s.gmRetryCount(sysId, count)
	}, 1)
}

type Retrieval struct {
	Base
}

func (s *Retrieval) s2cInfo() {
	s.SendProto3(170, 1, &pb3.S2C_170_1{
		Records:    s.retrievalRecords(),
		LoginLevel: s.owner.GetLevel(),
	})
}

// c2sRetrievalOnce 资源找回一次
func (s *Retrieval) c2sRetrievalOnce(msg *base.Message) error {
	var req pb3.C2S_170_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("retrieval c2sRetrievalOnce unpack msg, err: %w", err)
	}

	err := s.retrieval(req.GetSysId(), req.GetConsumeId(), 1, false)
	if err != nil {
		return err
	}

	for _, r := range s.retrievalRecords() {
		if r.SysId == req.GetSysId() {
			s.SendProto3(170, 2, &pb3.S2C_170_2{Record: r})
			break
		}
	}

	return nil
}

func (s *Retrieval) hasRetrievalCount(sysId, count uint32) bool {
	records := s.retrievalRecords()
	for _, r := range records {
		if r.SysId == sysId && r.Count >= count {
			return true
		}
	}

	return false
}

func (s *Retrieval) retrieval(sysId, consumeId, count uint32, skipConsumeAndAwards bool) error {
	if !s.hasRetrievalCount(sysId, count) {
		return neterror.ParamsInvalidError("没有足够的找回次数")
	}

	cfg, ok := jsondata.GetRetrievalConf(sysId)
	if !ok {
		return neterror.ConfNotFoundError("retrieval sysId[%d] config not exist", sysId)
	}

	data := s.getData()
	owner := s.GetOwner()

	// 跳过消耗和奖励发放
	if !skipConsumeAndAwards {
		level := owner.GetLevel()
		// 根据登录时的等级来读取配置
		var lvConf *jsondata.RetrievalLvConf
		for _, v := range cfg.LvConf {
			if level >= v.MinLv && level <= v.MaxLv {
				lvConf = v
				break
			}
		}
		if lvConf == nil {
			return neterror.ConfNotFoundError("等级配置不存在")
		}

		consumes, rewards := s.consumeAndReward(lvConf, consumeId, count)
		if len(consumes) == 0 {
			return neterror.ConfNotFoundError("未找到消耗配置")
		}

		if cfg.DesignatedRewardId == 0 && len(rewards) == 0 {
			return neterror.ConfNotFoundError("未找到奖励配置")
		}

		if cfg.DesignatedRewardId > 0 {
			handle := retrievalmgr.Get(cfg.DesignatedRewardId)
			rewards = handle(owner, count, consumeId)
		}

		if len(rewards) == 0 {
			return neterror.ConfNotFoundError("未找到自定义奖励配置")
		}

		// 庆典节日特权活动
		if owner.GetExtraAttr(attrdef.CelebrationFreePrivilege) > 0 {
			consumes = jsondata.CalcConsumeDiscount(consumes, 5000)
		}

		// 消耗
		if !s.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogRetrievalConsume}) {
			return neterror.ConsumeFailedError("消耗道具失败")
		}

		// 发放奖励
		if !engine.GiveRewards(s.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRetrievalReward}) {
			return neterror.InternalError("发放奖励失败")
		}
	}

	sCount := int32(count)
	for _, r := range data.Records {
		if sCount <= 0 {
			break
		}

		// 依次记录剩余和领取次数
		// 优先找回更旧的可找回次数
		if r.SysId == sysId && r.RemainCount > 0 {
			if r.RemainCount >= sCount {
				r.RemainCount -= sCount
				r.ReceiveCount += sCount
				sCount = 0
			} else {
				r.ReceiveCount += r.RemainCount
				sCount -= r.RemainCount
				r.RemainCount = 0
			}
		}
	}

	return nil
}

func (s *Retrieval) getRetrievalConsume(sysId, consumeId, count uint32) ([]*jsondata.Consume, []*jsondata.StdReward, error) {
	if !s.hasRetrievalCount(sysId, count) {
		return nil, nil, neterror.ParamsInvalidError("没有足够的找回次数")
	}

	cfg, ok := jsondata.GetRetrievalConf(sysId)
	if !ok {
		return nil, nil, neterror.ConfNotFoundError("retrieval sysId[%d] config not exist", sysId)
	}

	owner := s.GetOwner()
	level := owner.GetLevel()
	// 根据登录时的等级来读取配置
	var lvConf *jsondata.RetrievalLvConf
	for _, v := range cfg.LvConf {
		if level >= v.MinLv && level <= v.MaxLv {
			lvConf = v
			break
		}
	}
	if lvConf == nil {
		return nil, nil, neterror.ConfNotFoundError("等级配置不存在")
	}

	consumes, rewards := s.consumeAndReward(lvConf, consumeId, count)
	if len(consumes) == 0 {
		return nil, nil, neterror.ConfNotFoundError("未找到消耗配置")
	}

	if cfg.DesignatedRewardId == 0 && len(rewards) == 0 {
		return nil, nil, neterror.ConfNotFoundError("未找到奖励配置")
	}

	if cfg.DesignatedRewardId > 0 {
		handle := retrievalmgr.Get(cfg.DesignatedRewardId)
		rewards = handle(owner, count, consumeId)
	}

	if len(rewards) == 0 {
		return nil, nil, neterror.ConfNotFoundError("未找到自定义奖励配置")
	}

	return consumes, rewards, nil
}

// consumeAndReward 找回所需消耗和获得奖励
func (s *Retrieval) consumeAndReward(lvConf *jsondata.RetrievalLvConf, consumeId, retrievalCount uint32) ([]*jsondata.Consume, []*jsondata.StdReward) {
	var consumes []*jsondata.Consume
	var rewards []*jsondata.StdReward

	for _, v := range lvConf.Consume1 {
		if v.Id == consumeId {
			consumes = jsondata.CopyConsumeVec(lvConf.Consume1)
			rewards = jsondata.CopyStdRewardVec(lvConf.Reward1)
		}
	}
	for _, v := range lvConf.Consume2 {
		if v.Id == consumeId {
			consumes = jsondata.CopyConsumeVec(lvConf.Consume2)
			rewards = jsondata.CopyStdRewardVec(lvConf.Reward2)
		}
	}

	for _, consume := range consumes {
		consume.Count *= retrievalCount
	}

	for _, reward := range rewards {
		reward.Count *= int64(retrievalCount)
	}

	return consumes, rewards
}

// c2sRetrievalAll 一键资源找回
func (s *Retrieval) c2sRetrievalAll(msg *base.Message) error {
	var req pb3.C2S_170_3
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("retrieval c2sRetrievalAll unpack msg, err: %w", err)
	}
	owner := s.owner

	var totalConsumeVec jsondata.ConsumeVec
	var totalAwards jsondata.StdRewardVec
	for _, r := range s.retrievalRecords() {
		consume, rewards, err := s.getRetrievalConsume(r.GetSysId(), req.GetConsumeId(), r.Count)
		if err != nil {
			owner.LogError("%d retrieval failed, err:%v", r.GetSysId(), err)
			continue
		}
		totalConsumeVec = append(totalConsumeVec, consume...)
		totalAwards = append(totalAwards, rewards...)
	}

	totalConsumeVec = jsondata.MergeConsumeVec(totalConsumeVec)
	totalAwards = jsondata.MergeStdReward(totalAwards)

	// 庆典节日特权活动
	if owner.GetExtraAttr(attrdef.CelebrationFreePrivilege) > 0 {
		totalConsumeVec = jsondata.CalcConsumeDiscount(totalConsumeVec, 5000)
	}

	// 消耗
	if len(totalConsumeVec) == 0 || !s.GetOwner().ConsumeByConf(totalConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogRetrievalConsume}) {
		return neterror.ConsumeFailedError("消耗道具失败")
	}

	// 扣次数
	for _, r := range s.retrievalRecords() {
		if err := s.retrieval(r.GetSysId(), req.GetConsumeId(), r.Count, true); err != nil {
			owner.LogError("%d retrieval failed, err:%v", r.GetSysId(), err)
			continue
		}
	}

	// 发放奖励
	if !engine.GiveRewards(s.GetOwner(), totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRetrievalReward}) {
		return neterror.InternalError("发放奖励失败")
	}

	s.s2cInfo()
	return nil
}

func (s *Retrieval) OnOpen() {
	s.resetData()
	s.s2cInfo()
}

func (s *Retrieval) OnAfterLogin() {
	s.triggerLoginEvent(s.GetOwner())
	s.resetData()
	s.s2cInfo()
}

func (s *Retrieval) OnReconnect() {
	s.resetData()
	s.s2cInfo()
}

func newRetrievalSystem() iface.ISystem {
	return &Retrieval{}
}

func (s *Retrieval) getData() *pb3.Retrieval {
	if s.GetBinaryData().Retrieval == nil {
		s.GetBinaryData().Retrieval = &pb3.Retrieval{}
	}
	if s.GetBinaryData().Retrieval.OpenTime == 0 {
		s.GetBinaryData().Retrieval.OpenTime = time_util.GetZeroTime(time_util.NowSec())
	}
	return s.GetBinaryData().Retrieval
}

// resetData 重置数据
func (s *Retrieval) resetData() {
	s.removeTimeoutRecord()

	for _, cfg := range jsondata.GetRetrievalConfMap() {
		s.fillRecords(cfg)
	}
}

func (s *Retrieval) canRecord(sysId uint32, beforeDay uint32) bool {
	cfg, ok := jsondata.GetRetrievalConf(sysId)
	if !ok {
		return false
	}

	if cfg.EndRecordDay > 0 && cfg.EndRecordDay >= gshare.GetOpenServerDay() {
		return false
	}

	if cfg.OpenRecordDay > 0 && cfg.OpenRecordDay > gshare.GetOpenServerDay() {
		return false
	}

	// x 天前的开服天数
	if gshare.GetOpenServerDay() < beforeDay {
		return false
	}
	beforeOpenDay := gshare.GetOpenServerDay() - beforeDay
	if cfg.OpenRecordDay > 0 && cfg.OpenRecordDay > beforeOpenDay {
		return false
	}

	// 玩家等级
	if cfg.OpenLevel > 0 && cfg.OpenLevel > s.GetOwner().GetLevel() {
		return false
	}

	return true
}

// canOpen 是否可以开启
func (s *Retrieval) canRetrieval(sysId uint32) bool {
	cfg, ok := jsondata.GetRetrievalConf(sysId)
	if !ok {
		return false
	}

	// 关闭天数
	if cfg.EndDay > 0 && cfg.EndDay <= gshare.GetOpenServerDay() {
		return false
	}

	//开服天数
	if cfg.OpenDay > 0 && cfg.OpenDay > gshare.GetOpenServerDay() {
		return false
	}

	//合服次数
	if cfg.CombineTimes > 0 && cfg.CombineTimes > gshare.GetMergeTimes() {
		return false
	}

	//合服天数
	if cfg.CombineDay > 0 && cfg.CombineDay > gshare.GetMergeSrvDay() {
		return false
	}

	// 玩家等级
	if cfg.OpenLevel > 0 && cfg.OpenLevel > s.GetOwner().GetLevel() {
		return false
	}

	return true
}

func (s *Retrieval) removeTimeoutRecord() {
	data := s.getData()

	var records []*pb3.RetrievalInternalRecord
	for _, record := range data.Records {
		cfg, ok := jsondata.GetRetrievalConf(record.SysId)
		if !ok {
			continue
		}

		if cfg.MaxSaveDay > 0 {
			t := time_util.GetBeforeDaysZeroTime(cfg.MaxSaveDay)
			if record.Time >= t {
				records = append(records, record)
			}
		} else {
			records = append(records, record)
		}
	}

	data.Records = records
}

// retrievalOnNewDay 每日重置
func retrievalOnNewDay(actor iface.IPlayer, args ...interface{}) {
	s := actor.GetSysObj(sysdef.SiRetrieval).(*Retrieval)
	s.triggerLoginEvent(actor)
	s.resetData()
	s.s2cInfo()
}

// retrievalOnComplete 触发资源找回事件
func retrievalOnComplete(actor iface.IPlayer, args ...interface{}) {
	obj := actor.GetSysObj(sysdef.SiRetrieval)
	if obj == nil || !obj.IsOpen() {
		return
	}

	s := actor.GetSysObj(sysdef.SiRetrieval).(*Retrieval)

	// args[0] = sysId
	// args[1] = count
	if len(args) != 2 {
		return
	}

	sysId, count := args[0].(int), args[1].(int)
	today := time_util.GetZeroTime(time_util.NowSec())

	cfg, ok := jsondata.GetRetrievalConf(uint32(sysId))
	if !ok {
		return
	}

	s.fillRecordsByTime(cfg, today)

	data := s.getData()
	for _, record := range data.Records {
		if record.SysId != uint32(sysId) {
			continue
		}
		if record.Time != today {
			continue
		}
		if record.AttendCount >= int32(cfg.DailyMaxCount) {
			return
		}
		record.AttendCount += int32(count)
		break
	}
}

// fillRecordsByTime 填充指定时间记录
func (s *Retrieval) fillRecordsByTime(cfg *jsondata.RetrievalConf, time uint32) {
	data := s.getData()

	var hasRecord bool
	for _, v := range data.Records {
		if v.SysId == cfg.SysId && v.Time == time {
			hasRecord = true
			break
		}
	}

	if !hasRecord {
		data.Records = append(data.Records, &pb3.RetrievalInternalRecord{
			SysId:       cfg.SysId,
			Time:        time,
			RemainCount: int32(cfg.DailyMaxCount),
		})
	}
}

// fillRecords 按照最大保存天数(包含当天), 生成每日记录
func (s *Retrieval) fillRecords(cfg *jsondata.RetrievalConf) {
	data := s.getData()
	for i := uint32(0); i <= cfg.MaxSaveDay; i++ {
		t := time_util.GetBeforeDaysZeroTime(i)
		// 只有开启活动后才会填充记录
		if (data.OpenTime > 0 && t >= data.OpenTime) && s.canRecord(cfg.SysId, i) {
			s.fillRecordsByTime(cfg, t)
		}
	}

	sort.Slice(data.Records, func(i, j int) bool {
		return data.Records[i].Time < data.Records[j].Time
	})
}

// retrievalRecords 获得资源找回可领取记录
func (s *Retrieval) retrievalRecords() []*pb3.RetrievalRecord {
	cfgMap := jsondata.GetRetrievalConfMap()

	records := make([]*pb3.RetrievalRecord, 0, len(cfgMap))
	for sysId, cfg := range cfgMap {
		if !s.canRetrieval(sysId) {
			continue
		}
		counter, ok := retrievalCounter[sysId]
		if ok {
			records = append(records, counter(s.owner, cfg))
		} else {
			records = append(records, s.retrievalCommonCounter(cfg))
		}
	}

	return records
}

var retrievalCounter = map[uint32]func(actor iface.IPlayer, cfg *jsondata.RetrievalConf) *pb3.RetrievalRecord{
	sysdef.SiWildBoss: retrievalOffLineCounter,
	sysdef.SiSelfBoss: retrievalOffLineCounter,
}

// retrievalCommonCounter 公共资源找回次数计算
// 处理参与1次减少1次可资源找回情况
func (s *Retrieval) retrievalCommonCounter(cfg *jsondata.RetrievalConf) *pb3.RetrievalRecord {
	data := s.getData()

	record := &pb3.RetrievalRecord{
		SysId: cfg.SysId,
	}
	for _, r := range data.Records {
		// 今日记录不可找回
		if time_util.IsSameDay(r.Time, time_util.NowSec()) {
			continue
		}
		if r.SysId == cfg.SysId {
			// 剩余次数 = 每日最大次数 - 参与次数 - 领取次数
			r.RemainCount = int32(cfg.DailyMaxCount) - r.AttendCount - r.ReceiveCount
			r.RemainCount = utils.Ternary(r.RemainCount < 0, int32(0), r.RemainCount).(int32)
			record.Count += uint32(r.RemainCount)
		}
	}

	return record
}

func retrievalOffLineCounter(actor iface.IPlayer, cfg *jsondata.RetrievalConf) *pb3.RetrievalRecord {
	s := actor.GetSysObj(sysdef.SiRetrieval).(*Retrieval)
	data := s.getData()

	// 可找回天数, 从玩家开启活动时间开始算起
	var saveDay uint32
	for i := cfg.MaxSaveDay; i > 0; i-- {
		if data.OpenTime > 0 && time_util.IsSameDay(data.OpenTime, time_util.GetBeforeDaysZeroTime(i)) && s.canRecord(cfg.SysId, i) {
			saveDay = i
			break
		}
	}

	record := &pb3.RetrievalRecord{SysId: cfg.SysId}
	if saveDay == 0 {
		return record
	}

	// 总次数 = 每日最大次数 * 可找回天数
	// 这里为什么用总数减去已领取次数, 而不是剩余次数相加
	// 因为 data.Records 中不保证每日记录都存在,
	// 玩家可能某天没有参与, 也没有领取, 这样就会没有记录，导致剩余次数不准确
	// 而领取次数和参与次数都是准确的
	count := int32(cfg.DailyMaxCount) * int32(saveDay)
	for _, r := range data.Records {
		// 今日记录不参与计算
		if time_util.IsSameDay(r.Time, time_util.NowSec()) {
			continue
		}
		if r.SysId != cfg.SysId {
			continue
		}

		if r.AttendCount > 0 {
			// 单日有参与，则当日次数都不可找回
			count -= int32(cfg.DailyMaxCount)
			// 将剩余可找回次数置为0
			r.RemainCount = 0
		} else {
			// 单日未参与，则当日次数可找回, 减去已领取次数
			count -= r.ReceiveCount
		}
	}

	count = utils.Ternary(count < 0, int32(0), count).(int32)
	record.Count = uint32(count)

	return record
}

func (s *Retrieval) gmRetryCount(sysId, count uint32) bool {
	data := s.getData()
	if sysId > 0 {
		cfg, ok := jsondata.GetRetrievalConf(sysId)
		if !ok {
			return false
		}

		if count == 0 {
			count = cfg.DailyMaxCount
		}

		s.gmFillRecords(cfg)

		for _, r := range data.Records {
			if r.SysId == sysId {
				r.AttendCount = 0
				r.ReceiveCount = 0
				r.RemainCount += int32(count)
				r.RemainCount = utils.Ternary(r.RemainCount > int32(cfg.DailyMaxCount), cfg.DailyMaxCount, r.RemainCount).(int32)
			}
		}
	} else {
		for _, cfg := range jsondata.GetRetrievalConfMap() {
			s.fillRecords(cfg)
		}

		for _, r := range data.Records {
			cfg, ok := jsondata.GetRetrievalConf(r.SysId)
			if !ok {
				continue
			}

			if count == 0 {
				count = cfg.DailyMaxCount
			}

			r.AttendCount = 0
			r.ReceiveCount = 0
			r.RemainCount += int32(count)
			r.RemainCount = utils.Ternary(r.RemainCount > int32(cfg.DailyMaxCount), int32(cfg.DailyMaxCount), r.RemainCount).(int32)
		}
	}

	s.s2cInfo()
	return true
}

func (s *Retrieval) gmFillRecords(cfg *jsondata.RetrievalConf) {
	data := s.getData()
	for i := uint32(0); i <= cfg.MaxSaveDay; i++ {
		t := time_util.GetBeforeDaysZeroTime(i)
		s.fillRecordsByTime(cfg, t) // gm命令不判断是否开启
	}

	sort.Slice(data.Records, func(i, j int) bool {
		return data.Records[i].Time < data.Records[j].Time
	})
}

// 只需登录就算参与的系统, 每日触发完成事件
func (s *Retrieval) triggerLoginEvent(actor iface.IPlayer) {
	event.TriggerEvent(actor, custom_id.AeCompleteRetrieval, sysdef.SiWildBoss, 1)
	event.TriggerEvent(actor, custom_id.AeCompleteRetrieval, sysdef.SiSelfBoss, 1)
}
