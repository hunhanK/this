/**
 * @Author: LvYuMeng
 * @Date: 2024/7/16
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
	"jjyz/base/custom_id/moneydef"
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
)

type PlaneConsumeTurntableSys struct {
	PlayerYYBase
}

func (s *PlaneConsumeTurntableSys) data() *pb3.PYY_PlaneConsumeTurntable {
	state := s.GetYYData()
	if state.PlaneConsumeTurntable == nil {
		state.PlaneConsumeTurntable = make(map[uint32]*pb3.PYY_PlaneConsumeTurntable)
	}
	if state.PlaneConsumeTurntable[s.Id] == nil {
		state.PlaneConsumeTurntable[s.Id] = &pb3.PYY_PlaneConsumeTurntable{}
	}
	if state.PlaneConsumeTurntable[s.Id].AwardTimes == nil {
		state.PlaneConsumeTurntable[s.Id].AwardTimes = make(map[uint32]uint32)
	}
	return state.PlaneConsumeTurntable[s.Id]
}

func (s *PlaneConsumeTurntableSys) globalRecord() *pb3.PlaneConsumeTurntableRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.PlaneConsumeTurntableRecord == nil {
		globalVar.PyyDatas.PlaneConsumeTurntableRecord = make(map[uint32]*pb3.PlaneConsumeTurntableRecordList)
	}
	if globalVar.PyyDatas.PlaneConsumeTurntableRecord[s.Id] == nil || globalVar.PyyDatas.PlaneConsumeTurntableRecord[s.Id].StartTime != s.GetOpenTime() {
		globalVar.PyyDatas.PlaneConsumeTurntableRecord[s.Id] = &pb3.PlaneConsumeTurntableRecordList{StartTime: s.GetOpenTime()}
	}
	return globalVar.PyyDatas.PlaneConsumeTurntableRecord[s.Id]
}

func (s *PlaneConsumeTurntableSys) Login() {
	s.s2cInfo()
}

func (s *PlaneConsumeTurntableSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PlaneConsumeTurntableSys) s2cInfo() {
	s.SendProto3(69, 85, &pb3.S2C_69_85{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *PlaneConsumeTurntableSys) OnOpen() {
	s.s2cInfo()
}

func (s *PlaneConsumeTurntableSys) ResetData() {
	state := s.GetYYData()
	if state.PlaneConsumeTurntable == nil {
		return
	}
	delete(state.PlaneConsumeTurntable, s.GetId())
}

func (s *PlaneConsumeTurntableSys) NewDay() {
	conf, ok := jsondata.GetYYPlaneConsumeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.data()
	for _, v := range conf.ExtraScore {
		if v.Reset {
			delete(data.AwardTimes, v.Id)
		}
	}
	s.s2cInfo()
}

func (s *PlaneConsumeTurntableSys) checkAddScore(mt uint32, count int64, params common.ConsumeParams) {
	conf, ok := jsondata.GetYYPlaneConsumeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if pie.Uint32s(conf.BanBehavior).Contains(uint32(params.LogId)) {
		return
	}
	if mt != conf.MoneyType && !(mt == moneydef.Diamonds && conf.MoneyType == moneydef.BindDiamonds) {
		return
	}
	data := s.data()
	var score, refId uint32
	for _, v := range conf.ExtraScore {
		if v.Multiple == 0 {
			continue
		}
		if v.BehaviorId != uint32(params.LogId) {
			continue
		}
		if !engine.CheckConsumeBehavior(params, v.BehaviorId, v.SubType, v.Params) {
			continue
		}
		refId = v.Id
		break
	}

	score = uint32(count) * conf.ExchangeRate

	if refId > 0 {
		if conf.ExtraScore[refId].AwardTimes > data.AwardTimes[refId] { //检查双倍次数
			score *= conf.ExtraScore[refId].Multiple
			data.AwardTimes[refId]++
			s.SendProto3(69, 90, &pb3.S2C_69_90{
				ActiveId: s.GetId(),
				Id:       refId,
				Times:    data.AwardTimes[refId],
			})
		}
	}

	if score == 0 {
		return
	}
	s.updateScore(score, refId, true)
	return
}

func (s *PlaneConsumeTurntableSys) updateScore(score uint32, ext uint32, isAdd bool) {
	data := s.data()

	if isAdd {
		data.Score += score
	} else {
		if data.Score < score {
			data.Score = 0
		} else {
			data.Score -= score
		}
	}

	s.SendProto3(69, 88, &pb3.S2C_69_88{
		ActiveId: s.GetId(),
		Score:    data.Score,
	})

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPlaneConsumeTurntableScore, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", data.Score, ext),
	})
}

func (s *PlaneConsumeTurntableSys) appendRecord(record *pb3.PlaneConsumeTurntableRecord) {
	conf, ok := jsondata.GetYYPlaneConsumeTurntableConf(s.ConfName, s.ConfIdx)
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

func (s *PlaneConsumeTurntableSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_86
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYPlaneConsumeTurntableConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("PlaneConsumeTurntableSys conf is nil")
	}
	data := s.data()

	weightConf, ok := conf.GetTimeWeightConfByTimes(data.Times + 1)
	if !ok {
		return neterror.ConfNotFoundError("PlaneConsumeTurntableSys weight conf is nil")
	}

	if weightConf.Price > data.Score {
		return neterror.ParamsInvalidError("score not enough")
	}

	data.Times++

	var randomPool = new(random.Pool)
	for i := 0; i < len(weightConf.Rate); i += 2 {
		id := weightConf.Rate[i]
		weight := weightConf.Rate[i+1]
		randomPool.AddItem(id, weight)
	}
	awardId := randomPool.RandomOne().(uint32)

	engine.GiveRewards(s.GetPlayer(), conf.TurntableAward[awardId].Rewards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogPlaneConsumeTurntableAward,
		NoTips: true,
	})

	record := &pb3.PlaneConsumeTurntableRecord{
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

	s.SendProto3(69, 87, &pb3.S2C_69_87{
		ActiveId: s.GetId(),
		Id:       awardId,
		Times:    data.Times,
	})
	return nil
}

const (
	PlaneConsumeTurntableRecordGType = 1
	PlaneConsumeTurntableRecordPType = 2
)

func (s *PlaneConsumeTurntableSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_89
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_89{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case PlaneConsumeTurntableRecordGType:
		rsp.Record = s.globalRecord().Records
	case PlaneConsumeTurntableRecordPType:
		rsp.Record = s.data().Record
	}

	s.SendProto3(69, 89, rsp)

	return nil
}

func forRangePlaneConsumeTurntableSys(player iface.IPlayer, f func(s *PlaneConsumeTurntableSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYPlaneConsumeTurntable)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*PlaneConsumeTurntableSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		f(sys)
	}

	return
}

func checkPlaneConsumeTurntableExtraScore(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	mt, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[1].(int64)
	if !ok {
		return
	}
	params, ok := args[2].(common.ConsumeParams)
	if !ok {
		return
	}

	forRangePlaneConsumeTurntableSys(player, func(s *PlaneConsumeTurntableSys) {
		s.checkAddScore(mt, count, params)
	})
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPlaneConsumeTurntable, func() iface.IPlayerYY {
		return &PlaneConsumeTurntableSys{}
	})

	net.RegisterYYSysProtoV2(69, 86, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlaneConsumeTurntableSys).c2sDraw
	})

	net.RegisterYYSysProtoV2(69, 89, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlaneConsumeTurntableSys).c2sRecord
	})

	event.RegActorEvent(custom_id.AeConsumeMoney, checkPlaneConsumeTurntableExtraScore)
}
