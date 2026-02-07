package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/excel"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
	"path"
	"strings"

	"github.com/gzjjyz/srvlib/alg/bitset"
	"github.com/gzjjyz/srvlib/utils"
)

type AttrSys struct {
	owner iface.IPlayer

	extraAttr *attrcalc.ExtraAttrCalc // 非战斗属性

	fightAttr *attrcalc.FightAttrCalc // 战斗属性（都是战斗服同步回来的）

	sysAttr        [attrdef.SysEnd]*attrcalc.FightAttrCalc // 各系统属性
	sysAddRateAttr [attrdef.SysEnd]*attrcalc.FightAttrCalc // 各系统加成属性

	mask             *bitset.BitSet
	extraUpdateMask  *bitset.BitSet
	bSysAttrChange   bool
	bExtraAttrChange bool

	sysPowerMap map[uint32]int64 // 各系统战力保存用于重连的时候发送
}

func NewAttrSys(owner iface.IPlayer) *AttrSys {
	sys := &AttrSys{}
	sys.owner = owner
	sys.init()
	return sys
}

func (sys *AttrSys) init() {
	sys.mask = bitset.New(attrdef.SysEnd)
	sys.extraUpdateMask = bitset.New(attrdef.MaxExtraCount)
	sys.extraAttr = &attrcalc.ExtraAttrCalc{}
	sys.fightAttr = &attrcalc.FightAttrCalc{}
}

func (sys *AttrSys) Destroy() {
	sys.trace()
}

// ReSendPowerMap 重新发送战力map
func (sys *AttrSys) ReSendPowerMap() {
	resp := pb3.NewS2C_2_28()
	defer pb3.RealeaseS2C_2_28(resp)
	resp.SysPowerMap = sys.sysPowerMap
	resp.DailyInitPowerInfo = sys.GetDailyInitPowerInfo()
	sys.owner.LogDebug("战力重连重发: %v", sys.sysPowerMap)
	sys.owner.SendProto3(2, 28, resp)
}

func (sys *AttrSys) TriggerUpdateSysPowerMap() {
	sys.owner.TriggerEvent(custom_id.AeUpdateSysPowerMap, sys.sysPowerMap)
}

func (sys *AttrSys) GetSysPower(attrSysId uint32) int64 {
	return sys.sysPowerMap[attrSysId]
}

func (sys *AttrSys) GetAttrCalcByAttrId(t attrdef.AttrTypeAlias) attrcalc.FightAttrCalc {
	if nil == sys.sysAttr[t] {
		return attrcalc.FightAttrCalc{}
	}
	return *sys.sysAttr[t]
}

func (sys *AttrSys) SetExtraAttr(t attrdef.AttrTypeAlias, value attrdef.AttrValueAlias) {
	calc := sys.extraAttr
	if calc.GetValue(t) == value {
		return
	}
	calc.SetValue(t, value)
	sys.extraUpdateMask.Set(t - attrdef.ExtraAttrBegin)
	sys.bExtraAttrChange = true
}

func (sys *AttrSys) SetFightAttr(t attrdef.AttrTypeAlias, value attrdef.AttrValueAlias) {
	calc := sys.fightAttr
	if calc.GetValue(t) == value {
		return
	}
	calc.SetValue(t, value)
	sys.mask.Set(t - attrdef.FightPropBegin)
	sys.bSysAttrChange = true
}

func (sys *AttrSys) GetExtraAttr(t attrdef.AttrTypeAlias) attrdef.AttrValueAlias {
	return sys.extraAttr.GetValue(t)
}

func (sys *AttrSys) GetFightAttr(t attrdef.AttrTypeAlias) attrdef.AttrValueAlias {
	return sys.fightAttr.GetValue(t)
}

func (sys *AttrSys) PackCreateData(create *pb3.CreateActorData) {
	create.Attrs = make(map[uint32]*pb3.SysAttr)
	create.AddRateAttr = &pb3.SysAttr{}
	for sysId, calc := range sys.sysAttr {
		if nil == calc {
			continue
		}
		st := &pb3.SysAttr{Attrs: make(map[uint32]int64)}

		calc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
			st.Attrs[t] = v
		})

		create.Attrs[uint32(sysId)] = st
	}

	// 计算加成的
	st := &pb3.SysAttr{Attrs: make(map[uint32]int64)}
	for _, calc := range sys.sysAddRateAttr {
		if nil == calc {
			continue
		}
		calc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
			st.Attrs[t] += v
		})
	}

	create.AddRateAttr = st

	create.ExtraAttrs = make(map[uint32]int64)
	sys.extraAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		create.ExtraAttrs[t] = v
	})
}

func (sys *AttrSys) ResetSysAttr(sysId uint32) {
	if sysId >= attrdef.SysEnd {
		return
	}

	cb := engine.GetAttrCalcFn(sysId)
	if nil == cb {
		sys.owner.LogStack("un register attr(id:%d) calc callback!", sysId)
		return
	}

	calc := sys.sysAttr[sysId]
	if nil == calc {
		calc = &attrcalc.FightAttrCalc{}
		sys.sysAttr[sysId] = calc
	}
	calc.Reset()

	utils.ProtectRun(func() {
		cb(sys.owner, calc)
	})

	sys.mask.Set(sysId)
	sys.bSysAttrChange = true
}

func (sys *AttrSys) LogicRun() {
	sys.checkSync()
	if sys.bExtraAttrChange {
		sys.bExtraAttrChange = false
		calc := sys.extraAttr
		msg := pb3.SyncExtraAttr{Attrs: make(map[uint32]attrdef.AttrValueAlias)}
		sys.extraUpdateMask.Range(func(idx uint32) {
			sys.extraUpdateMask.Unset(idx)
			attrType := idx + attrdef.ExtraAttrBegin
			msg.Attrs[attrType] = calc.GetValue(attrType)
		})
		sys.owner.CallActorFunc(actorfuncid.UpdateExtraAttr, &msg)
	}
}

func (sys *AttrSys) calcTotalSysAddRate() {
	totalSysCalc := attrcalc.GetSingleCalc()
	for _, line := range sys.sysAttr {
		// 每个分支系统的属性汇总
		if nil == line {
			continue
		}
		totalSysCalc.AddCalc(line)
	}
	owner := sys.owner
	engine.EachAttrCalcFn(func(sysId uint32, cb engine.AddRateCalcCBFn) {
		if sysId >= attrdef.SysEnd {
			owner.LogError("calcTotalSysAddRate sysId:%d to long", sysId)
			return
		}
		calc := sys.sysAddRateAttr[sysId]
		if nil == calc {
			calc = &attrcalc.FightAttrCalc{}
			sys.sysAddRateAttr[sysId] = calc
		}
		calc.Reset()
		utils.ProtectRun(func() {
			cb(owner, totalSysCalc, calc)
		})
	})
}

func (sys *AttrSys) checkSync() {
	if !sys.bSysAttrChange {
		return
	}

	// 计算各种系统属性加成
	sys.calcTotalSysAddRate()

	msg := pb3.SyncSysAttr{
		SysAttrs: make(map[uint32]*pb3.SysAttr),
	}

	sys.bSysAttrChange = false
	sys.mask.Range(func(sysId uint32) {
		sys.mask.Unset(sysId)
		calc := sys.sysAttr[sysId]
		if nil == calc {
			return
		}

		st := &pb3.SysAttr{Attrs: make(map[uint32]int64)}

		calc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
			st.Attrs[t] = v
		})
		msg.SysAttrs[sysId] = st
	})

	// 遍历所有属性
	var influencePowerAttrSetMap = make(map[int]int64)
	for _, line := range sys.sysAttr {
		if nil == line {
			continue
		}
		attrdef.RangeInfluencePowerAttr(func(t int) {
			if v := line.GetValue(uint32(t)); v != 0 {
				influencePowerAttrSetMap[t] += v
			}
		})
	}

	// 计算加成的
	st := &pb3.SysAttr{Attrs: make(map[uint32]int64)}
	for _, calc := range sys.sysAddRateAttr {
		if nil == calc {
			continue
		}
		calc.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
			st.Attrs[t] += v
			if attrdef.IsInfluencePowerAttr(int(t)) {
				influencePowerAttrSetMap[int(t)] += v
			}
		})
	}
	msg.AddRateAttrs = st

	job := sys.owner.GetJob()
	var sysPowerMap = make(map[uint32]int64, attrdef.SysEnd-1)
	var singleCalc = new(attrcalc.FightAttrCalc)
	for id, line := range sys.sysAttr {
		// 每个分支系统的属性计算出来 发送给前端
		if nil == line {
			continue
		}
		singleCalc.Reset()
		singleCalc.AddCalc(line)
		addRateCalc := sys.sysAddRateAttr[uint32(id)]
		if addRateCalc != nil {
			singleCalc.AddCalc(addRateCalc)
		}
		for t, v := range influencePowerAttrSetMap {
			singleCalc.SetValue(uint32(t), v)
		}
		sysPowerMap[uint32(id)] = attrcalc.CalcByJobConvertAttackAndDefendGetFightValue(singleCalc, int8(job))
	}
	singleCalc.Reset()
	sys.owner.LogDebug("战力重算: %v", sysPowerMap)
	sys.owner.TriggerEvent(custom_id.AeUpdateSysPowerMap, sysPowerMap)
	sys.owner.CallActorFunc(actorfuncid.UpdateSysAttr, &msg)
	sys.CheckInitDailyInitPowerMap(sys.sysPowerMap)
	sys.sysPowerMap = sysPowerMap
	sys.RecordLastSysPowerMap(sysPowerMap)
	sys.owner.TriggerEvent(custom_id.AeAfterUpdateSysPowerMap, sysPowerMap)
	sys.ReSendPowerMap()
}

var listenFightAttr = []uint32{
	attrdef.ExpFuBenTimesAdd,
	attrdef.WorldBossTimesAdd,
	attrdef.EquipFuBenTimesAdd,
	attrdef.BeastRampantSettleFuBenTimesAdd,
}

func onSyncFightAttrs(player iface.IPlayer, buf []byte) {
	var st pb3.SyncFightAndAttr
	if nil != pb3.Unmarshal(buf, &st) {
		return
	}
	if sys, ok := player.GetAttrSys().(*AttrSys); ok && nil != sys {
		var listenFightAttrVal []int64
		for _, v := range listenFightAttr {
			listenFightAttrVal = append(listenFightAttrVal, sys.fightAttr.GetValue(v))
		}

		sys.fightAttr.Reset()
		for t, v := range st.Attrs {
			sys.fightAttr.SetValue(t, v)
		}

		for i, attrType := range listenFightAttr {
			if sys.fightAttr.GetValue(attrType) != listenFightAttrVal[i] {
				player.TriggerEvent(custom_id.AeRegFightAttrChange, attrType)
			}
		}
	}

	old := player.GetExtraAttr(attrdef.FightValue)
	if old != st.FightValue {
		player.SetExtraAttr(attrdef.FightValue, st.FightValue)
		player.LogInfo("玩家（%s）当前战力变化 %d -> %d", player.GetName(), old, st.FightValue)
		player.SetRankValue(gshare.RankTypePower, st.FightValue)
		player.TriggerEvent(custom_id.AeFightValueChange, st.FightValue, old)
		player.TriggerQuestEvent(custom_id.QttFightValue, 0, st.FightValue)
	}
}

func (sys *AttrSys) PackFightPropertyData(info *pb3.DetailedRoleInfo) {
	fightAttr := make(map[uint32]int64)
	sys.fightAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		if t == 0 {
			return
		}
		fightAttr[t] = v
	})
	info.FightProp = fightAttr
}

func (sys *AttrSys) PackPropertyData(data *pb3.OfflineProperty) {
	data.FightAttr = make(map[uint32]int64)
	data.ExtraAttr = make(map[uint32]int64)
	sys.fightAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		if t == 0 {
			return
		}
		data.FightAttr[t] = v
	})
	data.ExtraAttr = make(map[uint32]int64)
	sys.extraAttr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
		data.ExtraAttr[t] = v
	})
}

func (sys *AttrSys) ReConnectSendSysAttr() {
	mgr, ok := jsondata.GetSysAttrPushConfMgr()
	if !ok {
		return
	}
	var sysAttrsMap = make(map[uint32]*pb3.PushSysAttr)
	for attrSysId, attr := range sys.sysAttr {
		if attr == nil {
			continue
		}
		attr.DoRange(func(t uint32, v attrdef.AttrValueAlias) {
			sysAttrPushConf, ok := mgr[fmt.Sprintf("%d", t)]
			if !ok {
				return
			}
			for _, pushAttr := range sysAttrPushConf.PushAttrs {
				if pushAttr.AttrSysId != uint32(attrSysId) {
					continue
				}
				sysAttr, ok := sysAttrsMap[t]
				if !ok {
					sysAttr = &pb3.PushSysAttr{
						SysAttrMap: make(map[uint32]int64),
					}
					sysAttrsMap[t] = sysAttr
				}
				sysAttr.SysAttrMap[pushAttr.SysId] += v
			}
		})
	}
	if len(sysAttrsMap) > 0 {
		sys.owner.SendProto3(2, 90, &pb3.S2C_2_90{
			SysAttrsMap: sysAttrsMap,
		})
	}
}

func (sys *AttrSys) trace() {
	builder := &strings.Builder{}
	builder.WriteString(fmt.Sprintf("-----------%s attr trace start----------------\n", sys.owner.GetName()))
	for id, line := range sys.sysAttr {
		if nil != line {
			if desc := line.Trace(); len(desc) > 0 {
				builder.WriteString(fmt.Sprintf("sysid=%d, desc=%s\n", id, desc))
			}
		}
	}
	builder.WriteString("-----------attr trace end----------------")
	sys.owner.LogInfo(builder.String())
}

func (sys *AttrSys) GetDailyInitPowerInfo() *pb3.DailyInitPowerInfo {
	if sys.owner.GetBinaryData().DailyInitPowerInfo == nil {
		sys.owner.GetBinaryData().DailyInitPowerInfo = &pb3.DailyInitPowerInfo{}
	}
	if sys.owner.GetBinaryData().DailyInitPowerInfo.PowerMap == nil {
		sys.owner.GetBinaryData().DailyInitPowerInfo.PowerMap = make(map[uint32]int64)
	}
	if sys.owner.GetBinaryData().DailyInitPowerInfo.LastPowerMap == nil {
		sys.owner.GetBinaryData().DailyInitPowerInfo.LastPowerMap = make(map[uint32]int64)
	}
	return sys.owner.GetBinaryData().DailyInitPowerInfo
}

func (sys *AttrSys) CheckInitDailyInitPowerMap(lastSysPowerMap map[uint32]int64) {
	dailyInitPowerInfo := sys.GetDailyInitPowerInfo()
	nowSec := time_util.NowSec()
	// 首次初始化 那么今天的所有战力都从0开始累计
	if dailyInitPowerInfo.TodayAt == 0 {
		dailyInitPowerInfo.TodayAt = nowSec
		return
	}
	if time_util.IsSameDay(nowSec, dailyInitPowerInfo.TodayAt) {
		return
	}
	dailyInitPowerInfo.TodayAt = nowSec
	if lastSysPowerMap == nil {
		lastSysPowerMap = dailyInitPowerInfo.LastPowerMap
	}
	// 做个兜底
	if lastSysPowerMap == nil {
		return
	}
	for k, v := range lastSysPowerMap {
		dailyInitPowerInfo.PowerMap[k] = v
	}
}

func (sys *AttrSys) RecordLastSysPowerMap(lastSysPowerMap map[uint32]int64) {
	dailyInitPowerInfo := sys.GetDailyInitPowerInfo()
	dailyInitPowerInfo.LastPowerMap = make(map[uint32]int64)
	for k, v := range lastSysPowerMap {
		dailyInitPowerInfo.LastPowerMap[k] = v
	}
}

func handlePowerRushRankSubTypeDailyUpPower(player iface.IPlayer) (score int64) {
	attrSys := player.GetAttrSys()
	dailyInitPowerInfo := attrSys.GetDailyInitPowerInfo()
	var dailyTotalPower int64
	for _, power := range dailyInitPowerInfo.PowerMap {
		dailyTotalPower += power
	}
	totalPower := player.GetExtraAttr(attrdef.FightValue)
	if totalPower <= dailyTotalPower {
		return 0
	}
	return totalPower - dailyTotalPower
}

func handleDailyUpPowerStatTypeTotalPower(player iface.IPlayer) int64 {
	return player.GetExtraAttr(attrdef.FightValue)
}

func init() {
	engine.RegisterActorCallFunc(playerfuncid.SyncFightAttrs, onSyncFightAttrs)
	event.RegActorEvent(custom_id.AeReconnect, func(actor iface.IPlayer, args ...interface{}) {
		if sys := actor.GetAttrSys(); nil != sys {
			sys.ReSendPowerMap()
			attrSys, ok := sys.(*AttrSys)
			if ok {
				attrSys.ReConnectSendSysAttr()
			}
		}
	})

	event.RegActorEvent(custom_id.AeLoginFight, func(player iface.IPlayer, args ...interface{}) {
		if sys, ok := player.GetAttrSys().(*AttrSys); ok {
			sys.trace()
		}
	})

	net.RegisterProto(2, 28, func(player iface.IPlayer, msg *base.Message) error {
		if sys := player.GetAttrSys(); nil != sys {
			sys.ReSendPowerMap()
		}
		return nil
	})
	gmevent.Register("writerPowerMapToTxt", func(player iface.IPlayer, args ...string) bool {
		if sys := player.GetAttrSys(); nil != sys {
			attrSys, ok := sys.(*AttrSys)
			if !ok {
				return false
			}
			// 创建一个新的Excel文件用于保存结果
			exporter := excel.NewExporter(path.Join(utils.GetCurrentDir(), player.GetName()+".xlsx"))
			sheet := exporter.NewSheet("系统属性战力")
			sheet.WriterTitle([]interface{}{"系统ID", "名称", "战力"}...)
			for id := attrdef.SaLevel; id < attrdef.SysEnd; id++ {
				value := attrSys.sysPowerMap[uint32(id)]
				sheet.WriterData([]interface{}{id, attrdef.SysIdDescMap[id], value}...)
			}
			err := exporter.Export()
			if err != nil {
				player.LogError("err:%v", err)
				return false
			}
		}
		return true
	}, 1)

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeDailyUpPower, handlePowerRushRankSubTypeDailyUpPower)
	ranktype.RegDailyUpPowerStatFunc(ranktype.DailyUpPowerStatTypeTotalPower, handleDailyUpPowerStatTypeTotalPower)
}
