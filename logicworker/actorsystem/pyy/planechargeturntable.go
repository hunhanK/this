/**
 * @Author: LvYuMeng
 * @Date: 2024/7/17
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type PlaneChargeTurntableSys struct {
	PlayerYYBase
	rewards map[uint32]jsondata.StdRewardVec
}

func (s *PlaneChargeTurntableSys) OnInit() {
	s.rewards = make(map[uint32]jsondata.StdRewardVec)
}

func (s *PlaneChargeTurntableSys) OnLogout() {
	for _, awards := range s.rewards {
		engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPlaneChargeTurntableAward,
		})
	}
	s.rewards = make(map[uint32]jsondata.StdRewardVec)
}

func (s *PlaneChargeTurntableSys) data() *pb3.PYY_PlaneChargeTurntable {
	state := s.GetYYData()
	if state.PlaneChargeTurntable == nil {
		state.PlaneChargeTurntable = make(map[uint32]*pb3.PYY_PlaneChargeTurntable)
	}
	if state.PlaneChargeTurntable[s.Id] == nil {
		state.PlaneChargeTurntable[s.Id] = &pb3.PYY_PlaneChargeTurntable{}
	}
	return state.PlaneChargeTurntable[s.Id]
}

func (s *PlaneChargeTurntableSys) globalRecord() *pb3.PlaneChargeTurntableRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.PlaneChargeTurntableRecord == nil {
		globalVar.PyyDatas.PlaneChargeTurntableRecord = make(map[uint32]*pb3.PlaneChargeTurntableRecordList)
	}
	if globalVar.PyyDatas.PlaneChargeTurntableRecord[s.Id] == nil || globalVar.PyyDatas.PlaneChargeTurntableRecord[s.Id].StartTime != s.GetOpenTime() {
		globalVar.PyyDatas.PlaneChargeTurntableRecord[s.Id] = &pb3.PlaneChargeTurntableRecordList{StartTime: s.GetOpenTime()}
	}
	return globalVar.PyyDatas.PlaneChargeTurntableRecord[s.Id]
}

func (s *PlaneChargeTurntableSys) Login() {
	s.s2cInfo()
}

func (s *PlaneChargeTurntableSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlaneChargeTurntableSys) OnOpen() {
	s.s2cInfo()
}

func (s *PlaneChargeTurntableSys) ResetData() {
	state := s.GetYYData()
	if state.PlaneChargeTurntable == nil {
		return
	}
	delete(state.PlaneChargeTurntable, s.GetId())
}

func (s *PlaneChargeTurntableSys) s2cInfo() {
	s.SendProto3(69, 100, &pb3.S2C_69_100{
		ActiveId: s.GetId(),
		Info:     s.data(),
	})
}

func (s *PlaneChargeTurntableSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	chargeCent := chargeEvent.CashCent
	chargeId := chargeEvent.ChargeId
	data := s.data()

	conf, ok := jsondata.GetYYPlaneChargeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	chargeConf := jsondata.GetChargeConf(chargeId)
	if nil == chargeConf {
		return
	}
	var canRec bool
	for _, v := range conf.ExtraScore {
		if v.ChargeType != chargeConf.ChargeType {
			continue
		}
		if len(v.ChargeIds) > 0 && !pie.Uint32s(v.ChargeIds).Contains(chargeId) {
			continue
		}
		canRec = true
	}
	if !canRec {
		return
	}
	data.ChargeCent += chargeCent
	score := data.ChargeCent / conf.BaseChargeCent
	data.ChargeCent = data.ChargeCent % conf.BaseChargeCent
	s.addScore(score)
}

func (s *PlaneChargeTurntableSys) addScore(score uint32) {
	data := s.data()
	data.Score += score

	s.SendProto3(69, 102, &pb3.S2C_69_102{
		ActiveId: s.GetId(),
		Score:    data.Score,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlaneChargeTurntableScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.Score),
	})
}

const (
	PlaneChargeTurntableRecordGType = 1
	PlaneChargeTurntableRecordPType = 2
)

func (s *PlaneChargeTurntableSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_103
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_103{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case PlaneChargeTurntableRecordGType:
		rsp.Records = s.globalRecord().Records
	case PlaneChargeTurntableRecordPType:
		rsp.Records = s.data().Record
	}

	s.SendProto3(69, 103, rsp)

	return nil
}

func (s *PlaneChargeTurntableSys) c2sDailyAward(_ *base.Message) error {
	data := s.data()
	nowSec := time_util.NowSec()
	if data.LastRecDailyAwardsAt != 0 && time_util.IsSameDay(data.LastRecDailyAwardsAt, time_util.NowSec()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	conf, ok := jsondata.GetYYPlaneChargeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data.LastRecDailyAwardsAt = nowSec

	engine.GiveRewards(s.GetPlayer(), conf.DailyAward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPlaneChargeTurntableDailyAward,
	})

	s.SendProto3(69, 105, &pb3.S2C_69_105{
		ActiveId:             s.GetId(),
		LastRecDailyAwardsAt: nowSec,
	})
	return nil
}

func (s *PlaneChargeTurntableSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_101
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYPlaneChargeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("PlaneChargeTurntableSys conf is nil")
	}
	data := s.data()

	weightConf, ok := conf.GetTimeWeightConfByTimes(data.Times + 1)
	if !ok {
		return neterror.ConfNotFoundError("PlaneChargeTurntableSys weight conf is nil")
	}

	if weightConf.Price > data.Score {
		return neterror.ParamsInvalidError("score not enough")
	}

	enough, _ := s.GetPlayer().ConsumeByConfWithRet(weightConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogPlaneChargeTurntableConsume,
	})
	if !enough {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.Times++
	var randomPool = new(random.Pool)
	for _, v := range weightConf.MultipleWeight {
		randomPool.AddItem(v, v.Weight)
	}

	one := randomPool.RandomOne().(*jsondata.PlaneChargeTurntableMultipleWeight)

	rewards := jsondata.StdRewardMultiRate(weightConf.BaseRewards, float64(one.Multiple)/10000)

	var sendAwardsAfter = func(times uint32) {
		awards := s.rewards[times]
		delete(s.rewards, times)
		if len(awards) > 0 {
			engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogPlaneChargeTurntableAward,
			})
		}
	}

	if !req.IsSkip {
		curTime := data.Times
		s.rewards[curTime] = rewards
		s.GetPlayer().SetTimeout(time.Second*time.Duration(int64(conf.Dur)), func() {
			sendAwardsAfter(curTime)
		})
	} else {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPlaneChargeTurntableAward,
		})
	}

	var item []*pb3.KeyValue
	for _, v := range rewards {
		item = append(item, &pb3.KeyValue{
			Key:   v.Id,
			Value: uint32(v.Count),
		})
	}

	record := &pb3.PlaneChargeTurntableRecord{
		ActorId:   s.GetPlayer().GetId(),
		CreatedAt: time_util.NowSec(),
		Name:      s.GetPlayer().GetName(),
		Multiple:  one.Multiple,
		Item:      item,
	}

	s.appendRecord(record)

	s.SendProto3(69, 101, &pb3.S2C_69_101{
		ActiveId:         s.GetId(),
		Curtimes:         data.Times,
		MultipleWeightId: one.Id,
		Item:             item,
	})

	return nil
}

func (s *PlaneChargeTurntableSys) appendRecord(record *pb3.PlaneChargeTurntableRecord) {
	conf, ok := jsondata.GetYYPlaneChargeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var (
		data         = s.data()
		globalRecord = s.globalRecord()
	)

	data.Record = append(data.Record, record)
	globalRecord.Records = append(globalRecord.Records, record)

	if uint32(len(globalRecord.Records)) > conf.GlobalRecordCount {
		globalRecord.Records = globalRecord.Records[1:]
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneChargeTurntable, func() iface.IPlayerYY {
		return &PlaneChargeTurntableSys{}
	})

	net.RegisterYYSysProtoV2(69, 101, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlaneChargeTurntableSys).c2sDraw
	})

	net.RegisterYYSysProtoV2(69, 103, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlaneChargeTurntableSys).c2sRecord
	})

	net.RegisterYYSysProtoV2(69, 105, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlaneChargeTurntableSys).c2sDailyAward
	})

}
