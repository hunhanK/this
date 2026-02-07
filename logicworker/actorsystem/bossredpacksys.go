/**
 * @Author: lzp
 * @Date: 2024/5/13
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"encoding/json"

	"github.com/gzjjyz/random"
)

const (
	RecordCountLimit = 30
)

type BossRedPackSys struct {
	Base
	isNewDay bool
}

func (sys *BossRedPackSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *BossRedPackSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *BossRedPackSys) OnAfterLogin() {
	if sys.isNewDay {
		sys.RefreshDropTimes()
		sys.isNewDay = false
	}
}

func (sys *BossRedPackSys) OnOpen() {
	sys.S2CInfo()
}

func (sys *BossRedPackSys) S2CInfo() {
	data := sys.GetData()
	records := sys.GetUseRecord()
	if len(records) > RecordCountLimit {
		records = records[len(records)-RecordCountLimit:]
	}
	sys.SendProto3(2, 170, &pb3.S2C_2_170{
		BossRedPackData: &pb3.BossRedPackData{
			DailyCharge:     data.DailyCharge,
			UsedTimes:       data.UsedTimes,
			UsedPaidTimes:   data.UsedPaidTimes,
			IsPrivilege:     data.IsPrivilege,
			TotalDiamond:    data.TotalDiamond,
			IsFetchDRewards: data.IsFetchDRewards,
			Record:          records,
		},
	})
}

func (sys *BossRedPackSys) GetData() *pb3.BossRedPackData {
	if sys.GetBinaryData().BossRedPackData == nil {
		sys.GetBinaryData().BossRedPackData = &pb3.BossRedPackData{}
	}
	return sys.GetBinaryData().BossRedPackData
}

func (sys *BossRedPackSys) OnAddDailyCharge(cashCent uint32) {
	data := sys.GetData()
	data.DailyCharge += cashCent
	sys.S2CInfo()
}

func (sys *BossRedPackSys) OnNewDay() {
	sys.TrySendMail()
	sys.Reset()
	sys.S2CInfo()

	// 处理player.proxy 为空的情况
	if sys.GetOwner().GetActorProxy() == nil {
		sys.isNewDay = true
		return
	}
	sys.RefreshDropTimes()
	sys.isNewDay = false
}

func (sys *BossRedPackSys) RefreshDropTimes() {
	binData := sys.GetBinaryData()
	dropData := binData.DropData
	if dropData != nil {
		dropData.RedPackDropTimes = make(map[uint32]uint32)
		sys.GetOwner().CallActorFunc(actorfuncid.G2FClearRedPackDropTimes, nil)
	}
}

func (sys *BossRedPackSys) TrySendMail() {
	data := sys.GetData()
	if !data.IsPrivilege {
		return
	}
	if data.IsFetchDRewards {
		return
	}
	data.IsFetchDRewards = true

	conf := jsondata.BossRedPackConfMgr
	if conf != nil && len(conf.DayRewards) > 0 {
		sys.owner.SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_BossRedPackPrivilegeDReward,
			Rewards: conf.DayRewards,
		})
	}
}

func (sys *BossRedPackSys) Reset() {
	data := sys.GetData()
	data.DailyCharge = 0
	data.UsedTimes = 0
	data.UsedPaidTimes = 0
	data.IsFetchDRewards = false
}

func (sys *BossRedPackSys) UseBossRedPack(conf *jsondata.BasicUseItemConf) bool {
	if len(conf.Param) < 2 {
		sys.owner.LogError("useitemconfig param error id: %v", conf.ItemId)
		return false
	}

	bConf := jsondata.BossRedPackConfMgr
	if bConf == nil {
		return false
	}

	data := sys.GetData()
	if !data.IsPrivilege && data.UsedTimes >= bConf.DayCount {
		return false
	}

	rateId := sys.GetUseRate()
	rConf := bConf.Rate[rateId]
	if rConf == nil {
		return false
	}

	data.UsedTimes += 1
	if rConf.CondType == jsondata.RateCondCharge {
		data.UsedPaidTimes += 1
	}

	pool := new(random.Pool)
	for i := range rConf.RateValues {
		pool.AddItem(rConf.RateValues[i], rConf.RateValues[i].Weight)
	}
	vConf := pool.RandomOne().(*jsondata.BossRPRateValueConf)

	itemId := conf.Param[0]
	mul := vConf.Mul
	count := conf.Param[1] * mul
	data.TotalDiamond += count

	var rewards jsondata.StdRewardVec
	rewards = append(rewards, &jsondata.StdReward{
		Id:    itemId,
		Count: int64(count),
	})
	if !engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogBossRedPackUseReward,
	}) {
		sys.owner.LogError("Boss红包打开获得奖励失败")
		return false
	}

	// 记录使用日志
	if mul >= 3 {
		rd := &pb3.ItemGetRecord{
			ActorId:   sys.owner.GetId(),
			ActorName: sys.owner.GetName(),
			ItemId:    itemId,
			Count:     count,
			Ext:       mul,
			TimeStamp: time_util.NowSec(),
		}
		sys.AddUseRecord(rd)
	}

	logArg, _ := json.Marshal(map[string]interface{}{
		"usedTimes":     data.UsedTimes,
		"usesPaidTimes": data.UsedPaidTimes,
	})
	logworker.LogPlayerBehavior(sys.GetOwner(), pb3.LogId_LogBossRedPackUseReward, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})

	engine.BroadcastTipMsgById(tipmsgid.BossHongbaoTip1, sys.GetOwner().GetId(), sys.GetOwner().GetName(), mul, count)
	sys.SendProto3(2, 172, &pb3.S2C_2_172{
		ItemId: itemId,
		Count:  count,
		Rate:   mul,
	})
	sys.S2CInfo()
	return true
}

func (sys *BossRedPackSys) GetUseRecord() []*pb3.ItemGetRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.BossRedPackUseRecords == nil {
		globalVar.BossRedPackUseRecords = make([]*pb3.ItemGetRecord, 0)
	}
	return globalVar.BossRedPackUseRecords
}

func (sys *BossRedPackSys) AddUseRecord(rd *pb3.ItemGetRecord) {
	globalVar := gshare.GetStaticVar()
	if globalVar.BossRedPackUseRecords == nil {
		globalVar.BossRedPackUseRecords = make([]*pb3.ItemGetRecord, 0)
	}
	globalVar.BossRedPackUseRecords = append(globalVar.BossRedPackUseRecords, rd)

	length := len(globalVar.BossRedPackUseRecords)
	if length > RecordCountLimit {
		globalVar.BossRedPackUseRecords = globalVar.BossRedPackUseRecords[length-RecordCountLimit:]
	}
}

func (sys *BossRedPackSys) GetUseRate() uint32 {
	data := sys.GetData()
	conf := jsondata.BossRedPackConfMgr

	rateId := uint32(0)
	if data.DailyCharge > 0 {
		// 优先付费
		for id, v := range conf.Rate {
			if v.CondType != jsondata.RateCondCharge {
				continue
			}
			switch v.CondStr {
			case "lt":
				if data.DailyCharge < v.CondValue {
					rateId = id
				}
			case "ge":
				if data.DailyCharge >= v.CondValue {
					rateId = id
				}
			}
		}

		if rateId > 0 {
			// 校验付费开启次数
			if data.UsedPaidTimes >= conf.RateCount {
				rateId = jsondata.GetRegularRateId()
				if data.IsPrivilege {
					rateId = jsondata.GetPrivilegeRateId()
				}
			}
		} else {
			rateId = jsondata.GetRegularRateId()
			if data.IsPrivilege {
				rateId = jsondata.GetPrivilegeRateId()
			}
		}
		return rateId
	}

	rateId = jsondata.GetRegularRateId()
	if data.IsPrivilege {
		rateId = jsondata.GetPrivilegeRateId()
	}

	return rateId
}

func (sys *BossRedPackSys) c2sFetchDailyRewards(msg *base.Message) error {
	var req pb3.C2S_2_171
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("unmarshal C2S_2_171 error")
	}

	conf := jsondata.BossRedPackConfMgr
	if conf == nil {
		return neterror.ConfNotFoundError("config not found")
	}

	data := sys.GetData()
	if !data.IsPrivilege {
		return neterror.ParamsInvalidError("boss red pack privilege not open")
	}
	if data.IsFetchDRewards {
		return neterror.ParamsInvalidError("boss red pack daily rewards is fetched")
	}

	data.IsFetchDRewards = true
	if !engine.GiveRewards(sys.GetOwner(), conf.DayRewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogBossRedPackPrivilegeDayReward,
	}) {
		return neterror.InternalError("give rewards failed")
	}

	sys.SendProto3(2, 171, &pb3.S2C_2_171{
		IsFetch: data.IsFetchDRewards,
	})

	return nil
}

func useBossRedPack(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	// 不支持批量开启
	if param.Count > 1 {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if sys.UseBossRedPack(conf) {
		return true, true, param.Count
	}

	return
}

func handleAddDailyCharge(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
	if !ok || !sys.IsOpen() {
		return
	}

	if len(args) < 2 {
		return
	}

	chargeId := args[0].(uint32)
	cashCent := args[1].(uint32)

	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf != nil && chargeConf.ChargeType == chargedef.RedPack {
		return
	}

	sys.OnAddDailyCharge(cashCent)
}

func onNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.OnNewDay()
}

func bossRedPackChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	data := sys.GetData()
	return !data.IsPrivilege
}

func bossRedPackChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
	if !ok || !sys.IsOpen() {
		return false
	}

	data := sys.GetData()
	if data.IsPrivilege {
		return false
	}

	bConf := jsondata.BossRedPackConfMgr
	if conf == nil {
		return false
	}

	data.IsPrivilege = true
	if !engine.GiveRewards(sys.GetOwner(), bConf.BuyRewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSponsorGiftReward,
	}) {
		player.LogError("red boss pack buy rewards give error, id: %v", conf.ChargeId)
		return false
	}

	engine.BroadcastTipMsgById(tipmsgid.DayuPrivilege, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, bConf.BuyRewards))

	sys.S2CInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiBossRedPack, func() iface.ISystem {
		return &BossRedPackSys{}
	})

	net.RegisterSysProto(2, 171, sysdef.SiBossRedPack, (*BossRedPackSys).c2sFetchDailyRewards)

	miscitem.RegCommonUseItemHandle(itemdef.UseBossRedPack, useBossRedPack)
	event.RegActorEvent(custom_id.AeAddDailyCharge, handleAddDailyCharge)
	event.RegActorEvent(custom_id.AeNewDay, onNewDay)

	engine.RegChargeEvent(chargedef.BossRedPack, bossRedPackChargeCheck, bossRedPackChargeBack)

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		sys, ok := player.GetSysObj(sysdef.SiBossRedPack).(*BossRedPackSys)
		if !ok || !sys.IsOpen() {
			return
		}

		data := sys.GetData()
		if !data.IsPrivilege {
			return
		}

		return int64(conf.BossRedPack), nil
	})
}
