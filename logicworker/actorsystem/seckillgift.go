/**
 * @Author: zjj
 * @Date: 2025/7/10
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"
)

type SecKillGiftSys struct {
	Base
}

func (s *SecKillGiftSys) s2cInfo() {
	s.SendProto3(9, 50, &pb3.S2C_9_50{
		Data: s.getData(),
	})
}

func (s *SecKillGiftSys) getData() *pb3.SecKillGiftData {
	m := s.GetBinaryData().SecKillGiftData
	if m == nil {
		s.GetBinaryData().SecKillGiftData = make(map[uint32]*pb3.SecKillGiftData)
		m = s.GetBinaryData().SecKillGiftData
	}
	data := m[s.GetSysId()]
	if data == nil {
		m[s.GetSysId()] = &pb3.SecKillGiftData{}
		data = m[s.GetSysId()]
	}
	if data.SysId == 0 {
		data.SysId = s.GetSysId()
	}
	if data.GiftMustHit == nil {
		data.GiftMustHit = make(map[uint32]*pb3.SecKillGiftMustHit)
	}
	return data
}

func (s *SecKillGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SecKillGiftSys) OnLogin() {
	s.s2cInfo()
}

func (s *SecKillGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *SecKillGiftSys) handleSecKillGiftRecDailyAwards() error {
	data := s.getData()
	sId := data.SysId
	secKillGiftConfig := jsondata.GetSecKillGiftConfig(sId)
	if secKillGiftConfig == nil {
		return neterror.ConfNotFoundError("%d not found conf", sId)
	}

	var freeGiftConf *jsondata.SecKillGiftConf
	for _, giftConf := range secKillGiftConfig.GiftConf {
		if giftConf.Type != jsondata.SecKillGiftByFree {
			continue
		}
		freeGiftConf = giftConf
		break
	}
	if freeGiftConf == nil {
		return neterror.ConfNotFoundError("%d not found free gift conf", sId)
	}

	if utils.IsSetBit(data.DailyAwardRecFlag, freeGiftConf.Idx) {
		return neterror.ParamsInvalidError("%d free gift already rec", sId)
	}

	openDay := gshare.GetOpenServerDay()
	dayAwardsConf := findDayAwardsConf(freeGiftConf.DayAwards, openDay)
	if dayAwardsConf == nil {
		return neterror.ConfNotFoundError("%d not found day %d awards conf", sId, openDay)
	}
	data.DailyAwardRecFlag = utils.SetBit(data.DailyAwardRecFlag, freeGiftConf.Idx)
	owner := s.GetOwner()
	if len(dayAwardsConf.Awards) > 0 {
		engine.GiveRewards(owner, dayAwardsConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSecKillGiftFree,
		})
	}
	s.SendProto3(9, 52, &pb3.S2C_9_52{
		SysId:             sId,
		DailyAwardRecFlag: data.DailyAwardRecFlag,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSecKillGiftFree, &pb3.LogPlayerCounter{
		NumArgs: uint64(sId),
		StrArgs: fmt.Sprintf("%d_%d_%d", openDay, freeGiftConf.Idx, data.DailyAwardRecFlag),
	})
	return nil
}

func (s *SecKillGiftSys) handleSecKillGiftGetLogs(global bool) error {
	sId := s.GetSysId()
	var rest = &pb3.S2C_9_53{
		SysId:  sId,
		Global: global,
	}
	if global {
		rest.Logs = getSecKillGiftLogList(sId)
	} else {
		rest.Logs = s.getData().Logs
	}
	s.SendProto3(9, 53, rest)
	return nil
}

func (s *SecKillGiftSys) appendLog(logs ...*pb3.SecKillGiftLog) {
	sId := s.GetSysId()
	config := jsondata.GetSecKillGiftConfig(sId)
	if config == nil {
		return
	}
	limit := config.RecordLimit
	if limit == 0 {
		limit = 20
	}
	data := s.getData()
	logList := getSecKillGiftLogList(sId)
	data.Logs = append(data.Logs, logs...)
	logList = append(logList, logs...)
	sort.Slice(data.Logs, func(i, j int) bool {
		return data.Logs[i].CreatedAt > data.Logs[j].CreatedAt
	})
	sort.Slice(logList, func(i, j int) bool {
		return logList[i].CreatedAt > logList[j].CreatedAt
	})
	setSecKillGiftLogList(sId, logList)
	if uint32(len(data.Logs)) > limit {
		ret1 := make([]*pb3.SecKillGiftLog, limit)
		copy(ret1, data.Logs)
		data.Logs = ret1
	}
	if uint32(len(logList)) > limit {
		ret2 := make([]*pb3.SecKillGiftLog, limit)
		copy(ret2, logList)
		setSecKillGiftLogList(sId, ret2)
	}
}

func (s *SecKillGiftSys) checkTenDayGiftAwards() {
	data := s.getData()
	sId := data.SysId
	secKillGiftConfig := jsondata.GetSecKillGiftConfig(sId)
	if secKillGiftConfig == nil {
		return
	}

	if data.TenDayGiftBuyAt == 0 {
		data.TenDayGiftRecDay = 0
		return
	}

	if data.TenDayGiftRecDay >= 10 {
		data.TenDayGiftBuyAt = 0
		data.TenDayGiftRecDay = 0
		return
	}

	openSrvDay := gshare.GetOpenServerDay()
	canRecDay := 10 - data.TenDayGiftRecDay
	todayZeroTime := time_util.GetBeforeDaysZeroTime(0)
	lastTenDayGiftRecAt := data.TenDayGiftBuyAt + (data.TenDayGiftRecDay-1)*86400
	diffDays := time_util.GetDiffDays(int64(lastTenDayGiftRecAt), int64(todayZeroTime))
	if diffDays > canRecDay {
		diffDays = canRecDay
	}
	owner := s.GetOwner()
	actorId := owner.GetId()
	lastOpenSrvDay := openSrvDay - diffDays

	var giftConf *jsondata.SecKillGiftConf
	for _, gConf := range secKillGiftConfig.GiftConf {
		if gConf.Type == jsondata.SecKillGiftByTenDay {
			data.DailyAwardRecFlag = utils.SetBit(data.DailyAwardRecFlag, gConf.Idx)
			giftConf = gConf
		}
	}
	if giftConf == nil {
		return
	}
	startRecSrvDay := lastOpenSrvDay + 1
	for i := startRecSrvDay; i <= openSrvDay; i++ {
		data.TenDayGiftRecDay += 1
		totalAwards := handleTenDayGift(data, secKillGiftConfig, i)
		dayAwardsConf := findDayAwardsConf(secKillGiftConfig.DayAwards, i)
		if dayAwardsConf != nil {
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		args := &mailargs.CommonMailArgs{
			Digit1: int64(data.TenDayGiftRecDay),
			Str1:   giftConf.Name,
		}
		mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
			ConfId:  uint16(secKillGiftConfig.TenDaysMailId),
			Rewards: totalAwards,
			Content: args,
		})
	}
	data.AllBuyAwardsRec = true
}

func getSecKillGiftLogList(sid uint32) []*pb3.SecKillGiftLog {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}
	if staticVar.SecKillGiftLogMap == nil {
		staticVar.SecKillGiftLogMap = make(map[uint32]*pb3.SecKillGiftLogList)
	}
	if staticVar.SecKillGiftLogMap[sid] == nil {
		staticVar.SecKillGiftLogMap[sid] = &pb3.SecKillGiftLogList{}
	}
	logList := staticVar.SecKillGiftLogMap[sid]
	return logList.Logs
}

func setSecKillGiftLogList(sid uint32, logList []*pb3.SecKillGiftLog) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}
	if staticVar.SecKillGiftLogMap == nil {
		staticVar.SecKillGiftLogMap = make(map[uint32]*pb3.SecKillGiftLogList)
	}
	if staticVar.SecKillGiftLogMap[sid] == nil {
		staticVar.SecKillGiftLogMap[sid] = &pb3.SecKillGiftLogList{}
	}
	staticVar.SecKillGiftLogMap[sid].Logs = logList
	return
}

func handleSecKillGiftRecDailyAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_9_52
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*SecKillGiftSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}
	return sys.handleSecKillGiftRecDailyAwards()
}

func handleSecKillGiftGetLogs(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_9_53
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	obj := player.GetSysObj(req.SysId)
	if obj == nil || !obj.IsOpen() {
		return neterror.SysNotExistError("%d not open", req.SysId)
	}
	sys := obj.(*SecKillGiftSys)
	if sys == nil {
		return neterror.SysNotExistError("%d not convert success", req.SysId)
	}

	return sys.handleSecKillGiftGetLogs(req.Global)
}

func secKillGiftChargeCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sysId := jsondata.GetSecKillGiftSysIdByChargeId(conf.ChargeId)
	if sysId == 0 {
		return false
	}
	obj := actor.GetSysObj(sysId)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s := obj.(*SecKillGiftSys)
	if s == nil {
		return false
	}
	data := s.getData()
	sId := data.SysId

	secKillGiftConfig := jsondata.GetSecKillGiftConfig(sId)
	if secKillGiftConfig == nil {
		return false
	}

	chargeId := conf.ChargeId
	var giftConf *jsondata.SecKillGiftConf
	for _, gConf := range secKillGiftConfig.GiftConf {
		if gConf.ChargeId != chargeId {
			continue
		}
		giftConf = gConf
		break
	}
	if giftConf == nil {
		return false
	}

	var freeGiftConf *jsondata.SecKillGiftConf
	for _, giftConf := range secKillGiftConfig.GiftConf {
		if giftConf.Type != jsondata.SecKillGiftByFree {
			continue
		}
		freeGiftConf = giftConf
		break
	}

	dailyAwardRecFlag := data.DailyAwardRecFlag
	switch {
	case giftConf.Type == jsondata.SecKillGiftBySingle:
		// 重复购买
		if dailyAwardRecFlag != 0 && utils.IsSetBit(dailyAwardRecFlag, giftConf.Idx) {
			return false
		}
	case giftConf.Type == jsondata.SecKillGiftByQuick:
		if freeGiftConf != nil {
			// 除了免费礼包外，其他礼包有买的了
			if dailyAwardRecFlag != 0 && utils.ClearBit(dailyAwardRecFlag, freeGiftConf.Idx) > 0 {
				return false
			}
		}
	case giftConf.Type == jsondata.SecKillGiftByTenDay:
		if freeGiftConf != nil {
			// 除了免费礼包外，其他礼包有买的了
			if dailyAwardRecFlag != 0 && utils.ClearBit(dailyAwardRecFlag, freeGiftConf.Idx) > 0 {
				return false
			}
		}
	}

	return true
}

func secKillGiftChargeBack(actor iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !secKillGiftChargeCheck(actor, conf) {
		return false
	}
	sysId := jsondata.GetSecKillGiftSysIdByChargeId(conf.ChargeId)
	if sysId == 0 {
		return false
	}
	obj := actor.GetSysObj(sysId)
	if obj == nil || !obj.IsOpen() {
		return false
	}

	s := obj.(*SecKillGiftSys)
	if s == nil {
		return false
	}

	data := s.getData()
	sId := data.SysId

	secKillGiftConfig := jsondata.GetSecKillGiftConfig(sId)
	if secKillGiftConfig == nil {
		return false
	}

	giftConf := findGiftConfByChargeId(secKillGiftConfig.GiftConf, conf.ChargeId)
	if giftConf == nil {
		return false
	}

	totalAwards, tipMsgId, strParams := processGiftType(actor, s, data, giftConf, secKillGiftConfig)
	if len(totalAwards) > 0 {
		engine.GiveRewards(actor, totalAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogSecKillGiftCharge,
		})
		actor.SendShowRewardsPopBySys(totalAwards, sId)
	}

	broadcastTipMessage(tipMsgId, strParams)
	s.appendLog(&pb3.SecKillGiftLog{
		TigMsgId:  uint32(tipMsgId),
		Params:    strParams,
		CreatedAt: time_util.NowSec(),
	})

	s.SendProto3(9, 51, &pb3.S2C_9_51{
		SysId:             sId,
		DailyAwardRecFlag: data.DailyAwardRecFlag,
		AllBuyAwardsRec:   data.AllBuyAwardsRec,
		TenDayGiftBuyAt:   data.TenDayGiftBuyAt,
	})

	logworker.LogPlayerBehavior(actor, pb3.LogId_LogSecKillGiftCharge, &pb3.LogPlayerCounter{
		NumArgs: uint64(sId),
		StrArgs: fmt.Sprintf("%d_%d_%d_%v", gshare.GetOpenServerDay(), giftConf.Idx, data.DailyAwardRecFlag, data.AllBuyAwardsRec),
	})

	return true
}

func findGiftConfByChargeId(giftConfs []*jsondata.SecKillGiftConf, chargeId uint32) *jsondata.SecKillGiftConf {
	for _, gConf := range giftConfs {
		if gConf.ChargeId == chargeId {
			return gConf
		}
	}
	return nil
}

func processGiftType(actor iface.IPlayer, s *SecKillGiftSys, data *pb3.SecKillGiftData, giftConf *jsondata.SecKillGiftConf, secKillGiftConfig *jsondata.SecKillGiftConfig) (jsondata.StdRewardVec, int, []string) {
	var totalAwards jsondata.StdRewardVec
	var tipMsgId int
	var strParams []string
	openDay := gshare.GetOpenServerDay()

	strParams = append(strParams, fmt.Sprintf("%d", actor.GetId()), actor.GetName())

	switch giftConf.Type {
	case jsondata.SecKillGiftBySingle:
		totalAwards = handleSingleGift(data, giftConf, openDay)
		tipMsgId = tipmsgid.SecKillGiftTip1
		strParams = append(strParams, giftConf.Name, engine.StdRewardToBroadcast(s.GetOwner(), totalAwards))
	case jsondata.SecKillGiftByQuick:
		totalAwards = handleQuickGift(data, secKillGiftConfig, openDay)
		strParams = append(strParams, secKillGiftConfig.TabName, engine.StdRewardToBroadcast(s.GetOwner(), totalAwards))
		tipMsgId = tipmsgid.SecKillGiftTip2
	case jsondata.SecKillGiftByTenDay:
		totalAwards = handleTenDayGift(data, secKillGiftConfig, openDay)
		data.TenDayGiftBuyAt = time_util.GetBeforeDaysZeroTime(0)
		data.TenDayGiftRecDay = 1
		tipMsgId = tipmsgid.SecKillGiftTip3
		strParams = append(strParams, secKillGiftConfig.TabName)
	}

	data.DailyAwardRecFlag = utils.SetBit(data.DailyAwardRecFlag, giftConf.Idx)

	// 全购奖励
	secKillSingleGiftFlag := jsondata.GetBuyAllSecKillSingleGiftFlag(data.SysId)
	if !data.AllBuyAwardsRec && data.DailyAwardRecFlag&secKillSingleGiftFlag == secKillSingleGiftFlag {
		data.AllBuyAwardsRec = true
		dayAwardsConf := findDayAwardsConf(secKillGiftConfig.DayAwards, openDay)
		if dayAwardsConf != nil {
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		totalAwards = jsondata.MergeStdReward(totalAwards)
	}
	return totalAwards, tipMsgId, strParams
}

func handleSecKillGiftMustHit(data *pb3.SecKillGiftData, giftConf *jsondata.SecKillGiftConf) bool {
	mustHit := data.GiftMustHit[giftConf.Idx]
	if mustHit == nil {
		mustHit = &pb3.SecKillGiftMustHit{}
		data.GiftMustHit[giftConf.Idx] = mustHit
	}
	mustHit.Times += 1
	endMustHit := giftConf.GetBackEndMustHit(mustHit.Idx)
	var canMustHit bool
	if endMustHit != nil {
		if mustHit.Times >= endMustHit.Times {
			mustHit.Idx += 1
			canMustHit = true
		}
	}
	return canMustHit
}

func handleSingleGift(data *pb3.SecKillGiftData, giftConf *jsondata.SecKillGiftConf, openDay uint32) jsondata.StdRewardVec {
	var totalAwards jsondata.StdRewardVec
	dayAwardsConf := findDayAwardsConf(giftConf.DayAwards, openDay)
	if dayAwardsConf != nil {
		totalAwards = append(totalAwards, dayAwardsConf.Awards...)
	}

	var canMustHit = handleSecKillGiftMustHit(data, giftConf)
	var isHit = giftConf.ExtraRate > 0 && giftConf.ExtraRate >= random.IntervalUU(1, CoeRate)
	if canMustHit || isHit {
		dayAwardsConf := findDayAwardsConf(giftConf.ExtraRewards, openDay)
		totalAwards = append(totalAwards, dayAwardsConf.Awards...)
	}
	return totalAwards
}

func handleQuickGift(data *pb3.SecKillGiftData, secKillGiftConfig *jsondata.SecKillGiftConfig, openDay uint32) jsondata.StdRewardVec {
	var totalAwards jsondata.StdRewardVec

	for _, gConf := range secKillGiftConfig.GiftConf {
		if gConf.Type != jsondata.SecKillGiftBySingle {
			continue
		}
		dayAwardsConf := findDayAwardsConf(gConf.DayAwards, openDay)
		if dayAwardsConf != nil {
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		canHit := handleSecKillGiftMustHit(data, gConf)
		var isHit = gConf.ExtraRate > 0 && gConf.ExtraRate >= random.IntervalUU(1, CoeRate)
		if canHit || isHit {
			dayAwardsConf := findDayAwardsConf(gConf.ExtraRewards, openDay)
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		data.DailyAwardRecFlag = utils.SetBit(data.DailyAwardRecFlag, gConf.Idx)
	}

	return totalAwards
}

func handleTenDayGift(data *pb3.SecKillGiftData, secKillGiftConfig *jsondata.SecKillGiftConfig, openDay uint32) jsondata.StdRewardVec {
	var totalAwards jsondata.StdRewardVec
	for _, gConf := range secKillGiftConfig.GiftConf {
		if gConf.Type != jsondata.SecKillGiftBySingle {
			continue
		}
		dayAwardsConf := findDayAwardsConf(gConf.DayAwards, openDay)
		if dayAwardsConf != nil {
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		canHit := handleSecKillGiftMustHit(data, gConf)
		var isHit = gConf.ExtraRate > 0 && gConf.ExtraRate >= random.IntervalUU(1, CoeRate)
		if canHit || isHit {
			dayAwardsConf := findDayAwardsConf(gConf.ExtraRewards, openDay)
			totalAwards = append(totalAwards, dayAwardsConf.Awards...)
		}
		data.DailyAwardRecFlag = utils.SetBit(data.DailyAwardRecFlag, gConf.Idx)
	}
	return totalAwards
}

func findDayAwardsConf(dayAwards []*jsondata.SecKillGiftDayAwards, openDay uint32) *jsondata.SecKillGiftDayAwards {
	var awards *jsondata.SecKillGiftDayAwards
	for _, dayAward := range dayAwards {
		if dayAward.MinOpenDay <= openDay {
			awards = dayAward
		}
	}
	return awards
}

func broadcastTipMessage(tipMsgId int, strParams []string) {
	var iArr []interface{}
	for _, param := range strParams {
		iArr = append(iArr, param)
	}
	engine.BroadcastTipMsgById(uint32(tipMsgId), iArr...)
}

func handleSecKillGiftAeNewDay(player iface.IPlayer, _ ...interface{}) {
	var sysIdList = []uint32{sysdef.SiSecKillGiftEight, sysdef.SiSecKillGiftThirty, sysdef.SiSecKillGiftSixtyEight}
	for _, sysId := range sysIdList {
		obj := player.GetSysObj(sysId)
		if obj == nil || !obj.IsOpen() {
			continue
		}
		s := obj.(*SecKillGiftSys)
		if s == nil {
			continue
		}
		data := s.getData()
		data.DailyAwardRecFlag = 0
		data.AllBuyAwardsRec = false
		s.checkTenDayGiftAwards()
		s.s2cInfo()
	}
}

func init() {
	RegisterSysClass(sysdef.SiSecKillGiftEight, func() iface.ISystem {
		return &SecKillGiftSys{}
	})
	RegisterSysClass(sysdef.SiSecKillGiftThirty, func() iface.ISystem {
		return &SecKillGiftSys{}
	})
	RegisterSysClass(sysdef.SiSecKillGiftSixtyEight, func() iface.ISystem {
		return &SecKillGiftSys{}
	})
	engine.RegChargeEvent(chargedef.SecKillGift, secKillGiftChargeCheck, secKillGiftChargeBack)
	net.RegisterProto(9, 52, handleSecKillGiftRecDailyAwards)
	net.RegisterProto(9, 53, handleSecKillGiftGetLogs)
	// 注册跨天
	event.RegActorEvent(custom_id.AeNewDay, handleSecKillGiftAeNewDay)
}
