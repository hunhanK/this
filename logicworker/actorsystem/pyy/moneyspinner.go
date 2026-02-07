/**
 * @Author: LvYuMeng
 * @Date: 2024/8/1
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

type MoneySpinnerSys struct {
	PlayerYYBase
	rewards map[uint32]struct{}
}

func (s *MoneySpinnerSys) OnInit() {
	s.rewards = make(map[uint32]struct{})
}

func (s *MoneySpinnerSys) data() *pb3.PYY_MoneySpinner {
	state := s.GetYYData()
	if state.MoneySpinner == nil {
		state.MoneySpinner = make(map[uint32]*pb3.PYY_MoneySpinner)
	}
	if state.MoneySpinner[s.Id] == nil {
		state.MoneySpinner[s.Id] = &pb3.PYY_MoneySpinner{}
	}
	if nil == state.MoneySpinner[s.Id].AwardTimes {
		state.MoneySpinner[s.Id].AwardTimes = make(map[uint32]uint32)
	}
	return state.MoneySpinner[s.Id]
}

func (s *MoneySpinnerSys) globalRecord() *pb3.MoneySpinnerRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.MoneySpinnerRecord == nil {
		globalVar.PyyDatas.MoneySpinnerRecord = make(map[uint32]*pb3.MoneySpinnerRecordList)
	}
	if globalVar.PyyDatas.MoneySpinnerRecord[s.Id] == nil || globalVar.PyyDatas.MoneySpinnerRecord[s.Id].StartTime != s.GetOpenTime() {
		globalVar.PyyDatas.MoneySpinnerRecord[s.Id] = &pb3.MoneySpinnerRecordList{StartTime: s.GetOpenTime()}
	}
	return globalVar.PyyDatas.MoneySpinnerRecord[s.Id]
}

func (s *MoneySpinnerSys) Login() {
	s.s2cInfo()
}

func (s *MoneySpinnerSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MoneySpinnerSys) s2cInfo() {
	s.SendProto3(69, 150, &pb3.S2C_69_150{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *MoneySpinnerSys) OnOpen() {
	s.s2cInfo()
}

func (s *MoneySpinnerSys) OnEnd() {
	s.clearRecord()
}

func (s *MoneySpinnerSys) OnLogout() {
	if conf, ok := jsondata.GetYYMoneySpinnerConf(s.ConfName, s.ConfIdx); ok {
		for awardId := range s.rewards {
			engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogMoneySpinnerAward,
			})
		}
		s.rewards = make(map[uint32]struct{})
	}
}

func (s *MoneySpinnerSys) ResetData() {
	state := s.GetYYData()
	if state.MoneySpinner == nil {
		return
	}
	delete(state.MoneySpinner, s.GetId())
}

func (s *MoneySpinnerSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() == s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.MoneySpinnerRecord, s.GetId())
	}
}

func (s *MoneySpinnerSys) addScore(score uint32) {
	data := s.data()
	data.Score += score
	s.SendProto3(69, 152, &pb3.S2C_69_152{
		ActiveId: s.GetId(),
		Score:    data.Score,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogMoneySpinnerScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.Score),
	})
}

func (s *MoneySpinnerSys) appendRecord(record *pb3.MoneySpinnerRecord) {
	conf, ok := jsondata.GetYYMoneySpinnerConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var (
		data         = s.data()
		globalRecord = s.globalRecord()
	)

	data.Records = append(data.Records, record)
	globalRecord.Records = append(globalRecord.Records, record)

	if uint32(len(globalRecord.Records)) > conf.GlobalRecordCount {
		globalRecord.Records = globalRecord.Records[1:]
	}
}

func (s *MoneySpinnerSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	chargeCent := chargeEvent.CashCent
	chargeId := chargeEvent.ChargeId

	conf, ok := jsondata.GetYYMoneySpinnerConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var chargeType uint32
	if chargeConf := jsondata.GetChargeConf(chargeId); nil != chargeConf {
		chargeType = chargeConf.ChargeType
	}

	var score, refId uint32
	for _, v := range conf.ExtraScore {
		if v.Multiple == 0 || v.ChargeType == 0 {
			continue
		}
		if v.ChargeType != chargeType {
			continue
		}
		if len(v.ChargeIds) > 0 && !pie.Uint32s(v.ChargeIds).Contains(chargeId) {
			continue
		}
		refId = v.Id
		break
	}

	score = chargeCent / 100 * conf.ScoreRate
	if refId > 0 {
		data := s.data()
		if conf.ExtraScore[refId].AwardTimes > data.AwardTimes[refId] {
			data.AwardTimes[refId]++
			score *= conf.ExtraScore[refId].Multiple
			s.SendProto3(69, 155, &pb3.S2C_69_155{
				ActiveId: s.GetId(),
				Id:       refId,
				Times:    data.AwardTimes[refId],
			})
		}
	}

	s.addScore(score)
}

func (s *MoneySpinnerSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_151
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYMoneySpinnerConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("MoneySpinnerSys conf is nil")
	}
	data := s.data()

	weightConf, ok := conf.GetTimeWeightConfByTimes(data.Times + 1)
	if !ok {
		return neterror.ConfNotFoundError("MoneySpinnerSys weight conf is nil")
	}

	if weightConf.Price > data.Score {
		return neterror.ParamsInvalidError("score not enough")
	}

	data.Times++

	var randomPool = new(random.Pool)
	for i := 0; i < len(weightConf.Rate); i += 2 {
		id := weightConf.Rate[i]
		if pie.Uint32s(data.RevIds).Contains(id) {
			continue
		}
		weight := weightConf.Rate[i+1]
		randomPool.AddItem(id, weight)
	}

	awardId := randomPool.RandomOne().(uint32)
	data.RevIds = append(data.RevIds, awardId)

	var sendAwardsAfter = func(awardId uint32) {
		if _, noSend := s.rewards[awardId]; noSend {
			delete(s.rewards, awardId)
			engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogMoneySpinnerAward,
			})
		}
	}

	if !req.IsSkip {
		s.rewards[awardId] = struct{}{}
		s.GetPlayer().SetTimeout(time.Second*time.Duration(int64(conf.Dur)), func() {
			sendAwardsAfter(awardId)
		})
	} else {
		engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogMoneySpinnerAward,
		})
	}

	record := &pb3.MoneySpinnerRecord{
		ActorId:   s.GetPlayer().GetId(),
		Name:      s.GetPlayer().GetName(),
		TimeStamp: time_util.NowSec(),
		AwardId:   awardId,
	}
	for _, v := range conf.TurntableAward[awardId].Rewards {
		record.Reward = append(record.Reward, &pb3.KeyValue{
			Key:   v.Id,
			Value: uint32(v.Count),
		})
	}
	s.appendRecord(record)

	s.SendProto3(69, 151, &pb3.S2C_69_151{
		ActiveId: s.GetId(),
		Id:       awardId,
		Times:    data.Times,
	})
	return nil
}

const (
	MoneySpinnerRecordGType = 1
	MoneySpinnerRecordPType = 2
)

func (s *MoneySpinnerSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_153
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_153{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case MoneySpinnerRecordGType:
		rsp.Record = s.globalRecord().Records
	case MoneySpinnerRecordPType:
		rsp.Record = s.data().Records
	}

	s.SendProto3(69, 153, rsp)

	return nil
}

func (s *MoneySpinnerSys) c2sDailyAward(_ *base.Message) error {
	data := s.data()
	nowSec := time_util.NowSec()
	if data.LastRecDailyAwardsAt != 0 && time_util.IsSameDay(data.LastRecDailyAwardsAt, time_util.NowSec()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	conf, ok := jsondata.GetYYMoneySpinnerConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data.LastRecDailyAwardsAt = nowSec

	engine.GiveRewards(s.GetPlayer(), conf.DailyAward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogMoneySpinnerDailyAward,
	})

	s.SendProto3(69, 154, &pb3.S2C_69_154{
		ActiveId:             s.GetId(),
		LastRecDailyAwardsAt: nowSec,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYMoneySpinner, func() iface.IPlayerYY {
		return &MoneySpinnerSys{}
	})

	net.RegisterYYSysProtoV2(69, 151, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*MoneySpinnerSys).c2sDraw
	})

	net.RegisterYYSysProtoV2(69, 153, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*MoneySpinnerSys).c2sRecord
	})

	net.RegisterYYSysProtoV2(69, 154, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*MoneySpinnerSys).c2sDailyAward
	})
}
