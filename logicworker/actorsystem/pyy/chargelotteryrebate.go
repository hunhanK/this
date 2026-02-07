/**
 * @Author: zjj
 * @Date: 2024/6/20
 * @Desc: 位面庆典-招财喵喵
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type ChargeLotteryReBateSys struct {
	PlayerYYBase
}

const (
	ChargeLotteryReBateGType = 1
	ChargeLotteryReBatePType = 2
)

func (s *ChargeLotteryReBateSys) ResetData() {
	state := s.GetYYData()
	if state.PYYChargeLotteryReBate == nil {
		return
	}
	delete(state.PYYChargeLotteryReBate, s.Id)
}

func (s *ChargeLotteryReBateSys) resetGlobalRecord() {
	recordInfo := s.getGlobalRecord()

	// 清理时间是这个活动内的 不用二次清理
	if recordInfo.ClearAt != 0 && recordInfo.ClearAt >= s.GetOpenTime() && recordInfo.ClearAt <= s.GetEndTime() {
		return
	}

	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}

	if globalVar.PyyDatas.ChargeLotteryReBateRecordMgr == nil {
		globalVar.PyyDatas.ChargeLotteryReBateRecordMgr = make(map[uint32]*pb3.PYYChargeLotteryReBateRecordList)
	}

	globalVar.PyyDatas.ChargeLotteryReBateRecordMgr[s.Id] = &pb3.PYYChargeLotteryReBateRecordList{
		ClearAt: time_util.NowSec(),
	}
}

func (s *ChargeLotteryReBateSys) getData() *pb3.PYYChargeLotteryReBate {
	state := s.GetYYData()
	if state.PYYChargeLotteryReBate == nil {
		state.PYYChargeLotteryReBate = make(map[uint32]*pb3.PYYChargeLotteryReBate)
	}
	if state.PYYChargeLotteryReBate[s.Id] == nil {
		state.PYYChargeLotteryReBate[s.Id] = &pb3.PYYChargeLotteryReBate{}
	}
	if state.PYYChargeLotteryReBate[s.Id].DrawTimesMgr == nil {
		state.PYYChargeLotteryReBate[s.Id].DrawTimesMgr = make(map[uint32]*pb3.PYYChargeLotteryReBateTimesInfo)
	}
	return state.PYYChargeLotteryReBate[s.Id]
}

func (s *ChargeLotteryReBateSys) getGlobalRecord() *pb3.PYYChargeLotteryReBateRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.ChargeLotteryReBateRecordMgr == nil {
		globalVar.PyyDatas.ChargeLotteryReBateRecordMgr = make(map[uint32]*pb3.PYYChargeLotteryReBateRecordList)
	}
	if globalVar.PyyDatas.ChargeLotteryReBateRecordMgr[s.Id] == nil {
		globalVar.PyyDatas.ChargeLotteryReBateRecordMgr[s.Id] = &pb3.PYYChargeLotteryReBateRecordList{}
	}
	return globalVar.PyyDatas.ChargeLotteryReBateRecordMgr[s.Id]
}

func (s *ChargeLotteryReBateSys) appendRecord(record *pb3.PYYChargeLotteryReBateRecord) {
	conf, ok := jsondata.GetPYYChargeLotteryReBateConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.getData()
	globalRecord := s.getGlobalRecord()
	data.Records = append(data.Records, record)
	globalRecord.Records = append(globalRecord.Records, record)
	if uint32(len(globalRecord.Records)) > conf.GlobalRecordCount {
		globalRecord.Records = globalRecord.Records[1:]
	}
}

func (s *ChargeLotteryReBateSys) S2CInfo() {
	s.SendProto3(69, 30, &pb3.S2C_69_30{
		ActiveId: s.Id,
		Info:     s.getData(),
	})
}

func (s *ChargeLotteryReBateSys) Login() {
	s.S2CInfo()
}

func (s *ChargeLotteryReBateSys) OnReconnect() {
	s.S2CInfo()
}

func (s *ChargeLotteryReBateSys) OnOpen() {
	s.resetGlobalRecord()
	s.S2CInfo()
}

func (s *ChargeLotteryReBateSys) NewDay() {
	s.S2CInfo()
}

func (s *ChargeLotteryReBateSys) OnEnd() {
	data := s.getData()
	data.Records = nil
}

func (s *ChargeLotteryReBateSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_31
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getData()
	info, ok := data.DrawTimesMgr[req.ChargeId]
	if !ok {
		return neterror.ParamsInvalidError("%d not found", req.ChargeId)
	}

	if info.CanUse <= info.Used {
		return neterror.ParamsInvalidError("CanUse %d Used %d", info.CanUse, info.Used)
	}

	conf, ok := jsondata.GetPYYChargeLotteryReBateConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	layer, ok := conf.ChargeConf[req.ChargeId]
	if !ok {
		return neterror.ConfNotFoundError("%d not found layer conf", req.ChargeId)
	}

	if len(layer.Consume) == 0 {
		return neterror.ConsumeFailedError("%d not found layer consume conf", req.ChargeId)
	}

	var nextTimes = info.Used + 1
	multiple, ok := layer.TimesMultiple[nextTimes]
	if !ok {
		return neterror.ConfNotFoundError(" nextTimes %d not found multiple conf", nextTimes)
	}

	if len(multiple.MultipleWeight) == 0 {
		return neterror.ConfNotFoundError(" nextTimes %d not found multiple weight conf", nextTimes)
	}

	// 招财喵喵投入仙玉，需要不计入仙玉消费,避免影响其他累计仙玉消耗活动
	if !s.GetPlayer().ConsumeByConf(layer.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogChargeLotteryReBateRecDraw}) {
		return neterror.ConsumeFailedError("%d layer consume failed", req.ChargeId)
	}

	var randomPool = new(random.Pool)
	for _, weight := range multiple.MultipleWeight {
		randomPool.AddItem(weight, weight.Weight)
	}

	owner := s.GetPlayer()
	one := randomPool.RandomOne().(*jsondata.PYYChargeLotteryReBateMultipleWeight)

	count := layer.Consume[0].Count * one.Multiple / 10000
	var item = &pb3.KeyValue{
		Key:   conf.ItemId,
		Value: count,
	}

	record := &pb3.PYYChargeLotteryReBateRecord{
		ActorId:   owner.GetId(),
		Item:      item,
		CreatedAt: time_util.NowSec(),
		Name:      owner.GetName(),
		Multiple:  one.Multiple,
	}

	var sendAwards = func() {
		engine.GiveRewards(s.GetPlayer(), jsondata.StdRewardVec{{
			Id:    conf.ItemId,
			Count: int64(count),
		}}, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogChargeLotteryReBateGiveAwards,
		})
	}

	s.appendRecord(record)
	info.Used += 1
	if !req.IsSkip {
		dur := time.Second * time.Duration(int64(conf.Dur))
		owner.SetTimeout(dur, func() {
			sendAwards()
		})
	} else {
		sendAwards()
	}

	s.SendProto3(69, 31, &pb3.S2C_69_31{
		ActiveId:         s.Id,
		ChargeId:         req.ChargeId,
		Times:            nextTimes,
		MultipleWeightId: one.Id,
		Item:             item,
		TimesInfo:        info,
		IsSkip:           req.IsSkip,
	})

	logworker.LogPlayerBehavior(owner, pb3.LogId_LogChargeLotteryReBateRecDraw, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d_%d_%d_%d", req.ChargeId, info.CanUse, info.Used, nextTimes, one.Id),
	})
	return nil
}

func (s *ChargeLotteryReBateSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_32
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_32{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case ChargeLotteryReBateGType:
		rsp.Records = s.getGlobalRecord().Records
	case ChargeLotteryReBatePType:
		rsp.Records = s.getData().Records
	}

	s.SendProto3(69, 32, rsp)

	return nil
}

func (s *ChargeLotteryReBateSys) handleAddDailyCharge(args ...interface{}) {
	if len(args) < 2 {
		return
	}

	chargeId := args[0].(uint32)
	owner := s.GetPlayer()
	conf, ok := jsondata.GetPYYChargeLotteryReBateConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogError("%s not found YYChargeLotteryReBateConf", s.GetPrefix())
		return
	}

	layer, ok := conf.ChargeConf[chargeId]
	if !ok {
		owner.LogTrace("%d not found", chargeId)
		return
	}

	data := s.getData()
	info, ok := data.DrawTimesMgr[chargeId]
	if !ok {
		data.DrawTimesMgr[chargeId] = &pb3.PYYChargeLotteryReBateTimesInfo{}
		info = data.DrawTimesMgr[chargeId]
	}

	if info.CanUse >= layer.Times {
		owner.LogTrace("canUse %d, conf %d", info.CanUse, layer.Times)
		return
	}

	info.CanUse += 1
	owner.SendProto3(69, 33, &pb3.S2C_69_33{
		ActiveId:  s.Id,
		ChargeId:  chargeId,
		TimesInfo: info,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogChargeLotteryReBateRecTimeAdd, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", chargeId, info.CanUse),
	})
}

func (s *ChargeLotteryReBateSys) c2sRecDailyAwards(_ *base.Message) error {
	data := s.getData()
	nowSec := time_util.NowSec()
	if data.LastRecDailyAwardsAt != 0 && time_util.IsSameDay(data.LastRecDailyAwardsAt, time_util.NowSec()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	conf, ok := jsondata.GetPYYChargeLotteryReBateConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}
	if len(conf.DailyAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.DailyAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogChargeLotteryReBateRecDailyAwards,
		})
	}
	data.LastRecDailyAwardsAt = nowSec
	s.SendProto3(69, 34, &pb3.S2C_69_34{
		ActiveId:             s.Id,
		LastRecDailyAwardsAt: nowSec,
	})
	return nil
}

func eachAllChargeLotteryReBateSys(player iface.IPlayer, f func(sys *ChargeLotteryReBateSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYChargeLotteryReBate)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*ChargeLotteryReBateSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		f(sys)
	}

	return
}

func handleChargeLotteryReBateAddDailyCharge(player iface.IPlayer, args ...interface{}) {
	eachAllChargeLotteryReBateSys(player, func(sys *ChargeLotteryReBateSys) {
		sys.handleAddDailyCharge(args...)
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYChargeLotteryReBate, func() iface.IPlayerYY {
		return &ChargeLotteryReBateSys{}
	})

	net.RegisterYYSysProtoV2(69, 31, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ChargeLotteryReBateSys).c2sDraw
	})

	net.RegisterYYSysProtoV2(69, 32, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ChargeLotteryReBateSys).c2sRecord
	})
	net.RegisterYYSysProtoV2(69, 34, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ChargeLotteryReBateSys).c2sRecDailyAwards
	})

	event.RegActorEvent(custom_id.AeAddDailyCharge, handleChargeLotteryReBateAddDailyCharge)
}
