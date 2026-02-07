/**
 * @Author: zjj
 * @Date: 2024/11/6
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
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
	"strings"
	"time"
)

type RuneTurnTableSys struct {
	Base
}

func getRuneTurnTableLogList(isRate bool) []*pb3.RuneTurnTableLog {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}
	if isRate {
		return staticVar.SuperRuneTurnTableLogs
	}
	return staticVar.NormalRuneTurnTableLogs
}

func setRuneTurnTableLogList(isRate bool, list []*pb3.RuneTurnTableLog) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}
	if isRate {
		staticVar.SuperRuneTurnTableLogs = list
		return
	}
	staticVar.NormalRuneTurnTableLogs = list
}

func appendRuneTurnTableLog(log *pb3.RuneTurnTableLog) {
	var limit = uint32(20)
	commonConf := jsondata.GetRuneTurnTableConf()
	if commonConf != nil {
		limit = commonConf.GlobalRecordCount
	}

	list := getRuneTurnTableLogList(log.IsRate)
	list = append(list, log)
	if uint32(len(list)) <= limit {
		setRuneTurnTableLogList(log.IsRate, list)
		return
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].TimeStamp > list[j].TimeStamp
	})
	var ret = make([]*pb3.RuneTurnTableLog, limit)
	copy(ret, list)
	setRuneTurnTableLogList(log.IsRate, ret)
}

func (s *RuneTurnTableSys) s2cInfo() {
	s.SendProto3(47, 20, &pb3.S2C_47_20{
		Data: s.getData(),
	})
}

func (s *RuneTurnTableSys) getData() *pb3.RuneTurnTableData {
	data := s.GetBinaryData().RuneTurnTableData
	if data == nil {
		s.GetBinaryData().RuneTurnTableData = &pb3.RuneTurnTableData{}
		data = s.GetBinaryData().RuneTurnTableData
	}
	if data.ChargeIdxCount == nil {
		data.ChargeIdxCount = make(map[uint32]uint32)
	}
	return data
}

func (s *RuneTurnTableSys) OnReconnect() {
	s.s2cInfo()
}

func (s *RuneTurnTableSys) OnLogin() {
	s.s2cInfo()
}

func (s *RuneTurnTableSys) OnOpen() {
	s.s2cInfo()
}

func (s *RuneTurnTableSys) appendLog(logs ...*pb3.RuneTurnTableLog) {
	data := s.getData()
	// 暴力点 有性能问题再改 数量不会很多 最多十连
	for _, log := range logs {
		val := log
		appendRuneTurnTableLog(val)
		if log.IsRate {
			data.SuperLogs = append(data.SuperLogs, val)
		} else {
			data.NormalLogs = append(data.NormalLogs, val)
		}
	}

	var sub = func(list []*pb3.RuneTurnTableLog) []*pb3.RuneTurnTableLog {
		var limit = uint32(20)
		commonConf := jsondata.GetRuneTurnTableConf()
		if commonConf != nil {
			limit = commonConf.SelfRecordCount
		}
		if uint32(len(list)) <= limit {
			return list
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].TimeStamp > list[j].TimeStamp
		})
		var ret = make([]*pb3.RuneTurnTableLog, limit)
		copy(ret, list)
		return ret
	}

	data.SuperLogs = sub(data.SuperLogs)
	data.NormalLogs = sub(data.NormalLogs)
}

func (s *RuneTurnTableSys) c2sDrawAwards(msg *base.Message) error {
	var req pb3.C2S_47_21
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	owner := s.GetOwner()
	times := req.Times
	autoBuy := req.AutoBuy
	runeTurnTableConf := jsondata.GetRuneTurnTableConf()
	if runeTurnTableConf == nil {
		return neterror.ConfNotFoundError("not found conf")
	}
	drawPoolsConf := jsondata.GetRuneTurnTableDrawPoolsConf(owner.GetLevel())
	if drawPoolsConf == nil {
		return neterror.ConfNotFoundError("not found draw pool conf %d", owner.GetLevel())
	}

	var consume jsondata.ConsumeVec
	switch times {
	case 1:
		consume = runeTurnTableConf.OneDraw
	case 10:
		consume = runeTurnTableConf.TenDraw
	default:
		return neterror.ParamsInvalidError("not found %d times conf", times)
	}

	if len(consume) != 0 && !owner.ConsumeByConf(consume, autoBuy, common.ConsumeParams{LogId: pb3.LogId_LogRuneTurnTableDraw}) {
		return neterror.ConsumeFailedError("%d autoBuy:%v consume failed", times, autoBuy)
	}

	var totalAwards jsondata.StdRewardVec
	var logs []*pb3.RuneTurnTableLog
	randomPool := new(random.Pool)
	actorId := owner.GetId()
	name := owner.GetName()
	nowSec := time_util.NowSec()
	idx := drawPoolsConf.Idx
	var oneAwards = func(rewards jsondata.StdRewardVec) *jsondata.StdReward {
		randomPool.Clear()
		for _, stdReward := range rewards {
			val := stdReward
			randomPool.AddItem(val, val.Weight)
		}
		return randomPool.RandomOne().(*jsondata.StdReward)
	}
	var tipAwards jsondata.StdRewardVec
	for i := 0; i < int(times); i++ {
		var oneAward = oneAwards(drawPoolsConf.Rewards)
		totalAwards = append(totalAwards, oneAward)
		if oneAward.Extra > 0 {
			tipAwards = append(tipAwards, oneAward)
		}
		logs = append(logs, &pb3.RuneTurnTableLog{
			ActorId:     actorId,
			ActorName:   name,
			ItemId:      oneAward.Id,
			Count:       uint32(oneAward.Count),
			TimeStamp:   nowSec,
			DrawPoolIdx: uint32(idx),
			IsRate:      oneAward.Extra > 0,
		})
	}

	s.getData().DrawTimes += times
	var ret = &pb3.S2C_47_21{
		Times:       times,
		DrawPoolIdx: uint32(idx),
		Awards:      jsondata.StdRewardVecToPb3RewardVec(totalAwards),
		IsSkip:      req.IsSkip,
	}

	var doLogic = func() {
		s.appendLog(logs...)
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRuneTurnTableDraw})
		if len(tipAwards) > 0 {
			engine.BroadcastTipMsgById(tipmsgid.RuneTurntableTip, owner.GetId(), owner.GetName(), engine.StdRewardToBroadcast(owner, tipAwards))
		}
	}

	if req.IsSkip {
		doLogic()
	} else {
		owner.SetTimeout(time.Duration(runeTurnTableConf.Dur)*time.Second, doLogic)
	}

	s.SendProto3(47, 21, ret)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRuneTurnTableDraw, &pb3.LogPlayerCounter{
		NumArgs: uint64(times),
		StrArgs: fmt.Sprintf("%v", autoBuy),
	})
	return nil
}
func (s *RuneTurnTableSys) c2sGetLog(msg *base.Message) error {
	var req pb3.C2S_47_22
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	var superLogs, normalLogs []*pb3.RuneTurnTableLog
	if req.IsGlobal {
		superLogs = getRuneTurnTableLogList(true)
		normalLogs = getRuneTurnTableLogList(false)
	} else {
		data := s.getData()
		superLogs = data.SuperLogs
		normalLogs = data.NormalLogs
	}
	s.SendProto3(47, 22, &pb3.S2C_47_22{
		IsGlobal:   req.IsGlobal,
		NormalLogs: normalLogs,
		SuperLogs:  superLogs,
	})
	return nil
}
func (s *RuneTurnTableSys) c2sRecChargeAwards(msg *base.Message) error {
	var req pb3.C2S_47_23
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	owner := s.GetOwner()
	chargeInfo := owner.GetBinaryData().GetChargeInfo()
	cent := req.Cent
	if chargeInfo.DailyChargeMoney < cent {
		return neterror.ParamsInvalidError("%d not reach %d", chargeInfo.DailyChargeMoney, cent)
	}

	chargeLayerConf := jsondata.GetRuneTurnTableChargeLayerConf(cent)
	if chargeLayerConf == nil {
		return neterror.ConfNotFoundError("%d not found charge conf", cent)
	}

	data := s.getData()
	if pie.Uint32s(data.RecCentList).Contains(cent) {
		return neterror.ParamsInvalidError("%d already rec", cent)
	}

	data.RecCentList = append(data.RecCentList, cent)
	if len(chargeLayerConf.Rewards) != 0 && !engine.GiveRewards(owner, chargeLayerConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRuneTurnTableRecChargeAwards}) {
		return neterror.ConsumeFailedError("%d give rewards failed", cent)
	}

	s.SendProto3(47, 23, &pb3.S2C_47_23{
		Cent: cent,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRuneTurnTableRecChargeAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(cent),
	})
	return nil
}
func (s *RuneTurnTableSys) c2sRecReachAwards(msg *base.Message) error {
	var req pb3.C2S_47_24
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	owner := s.GetOwner()
	times := req.Times
	data := s.getData()
	if data.DrawTimes < times {
		return neterror.ParamsInvalidError("%d not reach %d", data.DrawTimes, times)
	}

	timesConf := jsondata.GetRuneTurnTableReachAwardsConf(times)
	if timesConf == nil {
		return neterror.ConfNotFoundError("%d not found times conf", times)
	}

	if pie.Uint32s(data.RecTimesList).Contains(times) {
		return neterror.ParamsInvalidError("%d already rec", times)
	}

	data.RecTimesList = append(data.RecTimesList, times)
	if len(timesConf.Rewards) != 0 && !engine.GiveRewards(owner, timesConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRuneTurnTableRecReachAwards}) {
		return neterror.ConsumeFailedError("%d give rewards failed", times)
	}

	s.SendProto3(47, 24, &pb3.S2C_47_24{
		Times: times,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRuneTurnTableRecReachAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(times),
	})
	return nil
}

func (s *RuneTurnTableSys) checkRecAwardSendMail() {
	owner := s.GetOwner()
	chargeInfo := owner.GetBinaryData().GetChargeInfo()
	chargeMoneyCent := chargeInfo.DailyChargeMoney
	data := s.getData()
	var totalAwards jsondata.StdRewardVec
	recList := pie.Uint32s(data.RecCentList)
	runeTurnTableConf := jsondata.GetRuneTurnTableConf()
	var logRecords []string
	jsondata.EachRuneTurnTableChargeLayerConf(func(c *jsondata.RuneTurnTableChargeLayerConf) {
		if chargeMoneyCent < c.Cent {
			return
		}
		if recList.Contains(c.Cent) {
			return
		}
		totalAwards = append(totalAwards, c.Rewards...)
		recList = recList.Append(c.Cent)
		logRecords = append(logRecords, fmt.Sprintf("%d", c.Cent))
	})
	data.RecCentList = recList
	if len(totalAwards) == 0 {
		return
	}
	mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
		ConfId:  runeTurnTableConf.ChargeFillMailId,
		Rewards: totalAwards,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRuneTurnTableSendMail, &pb3.LogPlayerCounter{
		StrArgs: strings.Join(logRecords, ","),
	})
}

func (s *RuneTurnTableSys) c2sRecFreeChargeAwards(msg *base.Message) error {
	var req pb3.C2S_47_25
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	owner := s.GetOwner()
	conf := jsondata.GetRuneTurnTableChargeAwardsConfByIdx(req.Idx)
	if conf == nil {
		return neterror.ParamsInvalidError("idx not found conf %d", req.Idx)
	}
	if conf.ChargeId != 0 {
		return neterror.ParamsInvalidError("idx not allow have charge conf %d", conf.ChargeId)
	}
	data := s.getData()
	if data.ChargeIdxCount[conf.Idx] >= conf.Count {
		return neterror.ParamsInvalidError("idx %d already rec", req.Idx)
	}
	data.ChargeIdxCount[conf.Idx] += 1
	engine.GiveRewards(owner, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRuneTurnTableChargeAwards})
	s.SendProto3(47, 25, &pb3.S2C_47_25{
		Idx:   conf.Idx,
		Count: data.ChargeIdxCount[conf.Idx],
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogRuneTurnTableChargeAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(conf.Idx),
		StrArgs: fmt.Sprintf("%d", data.ChargeIdxCount[conf.Idx]),
	})
	return nil
}

func checkRuneTurnTable(player iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	s := player.GetSysObj(sysdef.SiRuneTurnTable)
	if s == nil || !s.IsOpen() {
		return false
	}
	sys := s.(*RuneTurnTableSys)
	data := sys.getData()
	runeTurnTableChargeAwardsConf := jsondata.GetRuneTurnTableChargeAwardsConf(chargeConf.ChargeId)
	if runeTurnTableChargeAwardsConf == nil {
		return false
	}
	if runeTurnTableChargeAwardsConf.Count == 0 {
		return true
	}
	count := data.ChargeIdxCount[runeTurnTableChargeAwardsConf.Idx]
	return count < runeTurnTableChargeAwardsConf.Count
}

func chargeRuneTurnTable(player iface.IPlayer, chargeConf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !checkRuneTurnTable(player, chargeConf) {
		return false
	}
	s := player.GetSysObj(sysdef.SiRuneTurnTable)
	if s == nil || !s.IsOpen() {
		return false
	}
	sys := s.(*RuneTurnTableSys)
	runeTurnTableChargeAwardsConf := jsondata.GetRuneTurnTableChargeAwardsConf(chargeConf.ChargeId)
	if runeTurnTableChargeAwardsConf == nil {
		return false
	}
	data := sys.getData()
	data.ChargeIdxCount[runeTurnTableChargeAwardsConf.Idx] += 1
	engine.GiveRewards(player, runeTurnTableChargeAwardsConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRuneTurnTableChargeAwards})
	sys.SendProto3(47, 25, &pb3.S2C_47_25{
		Idx:   runeTurnTableChargeAwardsConf.Idx,
		Count: data.ChargeIdxCount[runeTurnTableChargeAwardsConf.Idx],
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogRuneTurnTableChargeAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(runeTurnTableChargeAwardsConf.Idx),
		StrArgs: fmt.Sprintf("%d", data.ChargeIdxCount[runeTurnTableChargeAwardsConf.Idx]),
	})
	return true
}

func init() {
	RegisterSysClass(sysdef.SiRuneTurnTable, func() iface.ISystem {
		return &RuneTurnTableSys{}
	})
	net.RegisterSysProtoV2(47, 21, sysdef.SiRuneTurnTable, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RuneTurnTableSys).c2sDrawAwards
	})
	net.RegisterSysProtoV2(47, 22, sysdef.SiRuneTurnTable, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RuneTurnTableSys).c2sGetLog
	})
	net.RegisterSysProtoV2(47, 23, sysdef.SiRuneTurnTable, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RuneTurnTableSys).c2sRecChargeAwards
	})
	net.RegisterSysProtoV2(47, 24, sysdef.SiRuneTurnTable, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RuneTurnTableSys).c2sRecReachAwards
	})
	net.RegisterSysProtoV2(47, 25, sysdef.SiRuneTurnTable, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*RuneTurnTableSys).c2sRecFreeChargeAwards
	})
	event.RegActorEvent(custom_id.AeBeforeNewDay, func(player iface.IPlayer, args ...interface{}) {
		s := player.GetSysObj(sysdef.SiRuneTurnTable)
		if s == nil || !s.IsOpen() {
			return
		}
		sys := s.(*RuneTurnTableSys)
		sys.checkRecAwardSendMail()
	})
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		s := player.GetSysObj(sysdef.SiRuneTurnTable)
		if s == nil || !s.IsOpen() {
			return
		}
		sys := s.(*RuneTurnTableSys)
		sys.getData().RecCentList = nil
		sys.getData().ChargeIdxCount = make(map[uint32]uint32)
		sys.s2cInfo()
	})
	engine.RegChargeEvent(chargedef.RuneTurnTable, checkRuneTurnTable, chargeRuneTurnTable)
}
