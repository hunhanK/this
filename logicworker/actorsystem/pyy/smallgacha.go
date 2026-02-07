/**
 * @Author: LvYuMeng
 * @Date: 2025/11/27
 * @Desc: 小抽奖
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
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
	"jjyz/gameserver/net"
	"time"
)

type SmallGacha struct {
	PlayerYYBase
}

func (s *SmallGacha) OnReconnect() {
	s.s2cInfo()
}

func (s *SmallGacha) Login() {
	s.s2cInfo()
}

func (s *SmallGacha) OnOpen() {
	s.clearRecord(false)
	data := s.getData()
	data.LoopId = 1
	s.s2cInfo()
}

func (s *SmallGacha) OnEnd() {
	s.clearRecord(true)
	s.s2cInfo()
}

func (s *SmallGacha) ResetData() {
	state := s.GetYYData()
	if nil == state.SmallGacha {
		return
	}
	delete(state.SmallGacha, s.Id)
}

func (s *SmallGacha) s2cInfo() {
	data := s.getData()
	s.SendProto3(143, 5, &pb3.S2C_143_5{
		ActiveId: s.Id,
		Data: &pb3.SmallGachaClient{
			LoopId:     data.LoopId,
			GetIds:     data.GetIds,
			TotalTimes: data.TotalTimes,
		},
	})
}

func (s *SmallGacha) clearRecord(isEnd bool) {
	record := s.getGlobalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.SmallGachaRecordList, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.SmallGachaRecordList, s.Id)
	}
}

func (s *SmallGacha) getData() *pb3.PYY_SmallGacha {
	state := s.GetYYData()
	if state.SmallGacha == nil {
		state.SmallGacha = make(map[uint32]*pb3.PYY_SmallGacha)
	}
	if state.SmallGacha[s.Id] == nil {
		state.SmallGacha[s.Id] = &pb3.PYY_SmallGacha{}
	}
	if state.SmallGacha[s.Id].GetIds == nil {
		state.SmallGacha[s.Id].GetIds = map[uint32]uint32{}
	}
	return state.SmallGacha[s.Id]
}

func (s *SmallGacha) getGlobalRecord() *pb3.SmallGachaRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.SmallGachaRecordList == nil {
		globalVar.PyyDatas.SmallGachaRecordList = make(map[uint32]*pb3.SmallGachaRecordList)
	}
	if globalVar.PyyDatas.SmallGachaRecordList[s.Id] == nil {
		globalVar.PyyDatas.SmallGachaRecordList[s.Id] = &pb3.SmallGachaRecordList{}
	}
	if globalVar.PyyDatas.SmallGachaRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SmallGachaRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SmallGachaRecordList[s.Id]
}

func (s *SmallGacha) c2sGacha(msg *base.Message) error {
	var req pb3.C2S_143_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetSmallGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	if !pie.Uint32s(conf.Times).Contains(req.Times) {
		return neterror.ParamsInvalidError("invalid times")
	}

	data := s.getData()
	newSet := make(map[uint32]uint32, len(data.GetIds)+int(req.Times))
	for awardId, idx := range data.GetIds {
		if conf.Fixed && idx == req.Pos {
			return neterror.ParamsInvalidError("is open")
		}
		newSet[awardId] = idx
	}

	loopConf := conf.GetLoopConf(data.LoopId)
	if nil == loopConf {
		return neterror.ParamsInvalidError("loop %d not found", data.LoopId)
	}

	nowTimes := data.TotalTimes

	pool := new(random.Pool)
	randOne := func() *jsondata.SmallGachaPool {
		pool.Clear()
		for _, v := range loopConf.Pool {
			st := v
			if st.Times > nowTimes {
				continue
			}
			if _, exist := newSet[v.Id]; exist {
				continue
			}
			pool.AddItem(st, st.Weight)
		}
		if pool.Size() == 0 {
			return nil
		}
		rand := pool.RandomOne().(*jsondata.SmallGachaPool)
		return rand
	}

	var rands []*jsondata.SmallGachaPool
	right := data.TotalTimes + req.Times
	for left := data.TotalTimes + 1; left <= right; left++ {
		rand := randOne()
		if rand == nil {
			return neterror.ParamsInvalidError("pool item count not enough")
		}
		rands = append(rands, rand)
		newSet[rand.Id] = req.Pos
		nowTimes++
	}

	consume := loopConf.GetConsume(data.TotalTimes, req.Times)
	if len(consume) == 0 {
		return neterror.ParamsInvalidError("consume times conf lack")
	}

	if !s.GetPlayer().ConsumeByConf(consume, req.AutoBuy, common.ConsumeParams{
		LogId: pb3.LogId_LogSmallGachaConsume,
	}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.TotalTimes = nowTimes
	data.GetIds = newSet

	rsp := &pb3.S2C_143_6{
		ActiveId:   s.Id,
		Times:      req.Times,
		LoopId:     data.LoopId,
		TotalTimes: data.TotalTimes,
	}

	gData := s.getGlobalRecord()

	var (
		nowSec    = time_util.NowSec()
		actorName = s.GetPlayer().GetName()
		actorId   = s.GetPlayer().GetId()
	)

	var rewardsVec []jsondata.StdRewardVec
	for _, v := range rands {
		rewardsVec = append(rewardsVec, v.Rewards)
		rsp.Results = append(rsp.Results, &pb3.SmallGachaSt{
			AwardId: v.Id,
			Idx:     req.Pos,
		})
		record := &pb3.SmallGachaRecord{
			LoopId:    data.LoopId,
			AwardId:   v.Id,
			TimeStamp: nowSec,
			ActorId:   actorId,
			ActorName: actorName,
		}
		if len(v.Rewards) > 0 {
			record.ItemId = v.Rewards[0].Id
			record.Count = uint32(v.Rewards[0].Count)
		}
		if v.IsRare {
			s.record(&gData.SuperRecords, record, int(conf.RecordNum))
			s.record(&data.SuperRecords, record, int(conf.RecordNum))
		} else {
			s.record(&gData.Records, record, int(conf.RecordNum))
			s.record(&data.Records, record, int(conf.RecordNum))
		}
	}

	s.SendProto3(143, 6, rsp)

	jumpId := conf.TipsJump
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if !req.IsSkip {
		s.GetPlayer().SetTimeout(time.Millisecond*time.Duration(int64(conf.Dur)), func() {
			engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
				LogId:        pb3.LogId_LogSmallGachaAwards,
				NoTips:       true,
				BroadcastExt: []interface{}{s.Id, jumpId},
			})
		})
	} else {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId:        pb3.LogId_LogSmallGachaAwards,
			NoTips:       true,
			BroadcastExt: []interface{}{s.Id, jumpId},
		})
	}

	return nil
}

func (s *SmallGacha) record(records *[]*pb3.SmallGachaRecord, record *pb3.SmallGachaRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SmallGacha) c2sNext(msg *base.Message) error {
	var req pb3.C2S_143_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetSmallGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}
	data := s.getData()
	loopConf := conf.GetLoopConf(data.LoopId)
	if nil == loopConf {
		return neterror.ParamsInvalidError("loop %d not found", data.LoopId)
	}
	for _, v := range loopConf.Pool {
		if _, exist := data.GetIds[v.Id]; !exist {
			return neterror.ParamsInvalidError("pool not clear")
		}
	}
	nextPoolId := data.LoopId + 1
	nextPool := conf.GetLoopConf(nextPoolId)
	if nil == nextPool {
		return neterror.ParamsInvalidError("loop %d not found", nextPoolId)
	}
	nowSec := time_util.NowSec()
	unLockTime := s.GetOpenTime() + nextPool.UnlockTime
	if nowSec < unLockTime {
		return neterror.ParamsInvalidError("loop %d not at unlock time", nextPoolId)
	}
	data.LoopId = nextPoolId
	data.GetIds = nil
	data.TotalTimes = 0
	s.s2cInfo()
	return nil
}

const (
	SmallGachaRecordGType = 1
	SmallGachaRecordPType = 2
)

func (s *SmallGacha) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_143_8
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_143_8{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case SmallGachaRecordGType:
		gData := s.getGlobalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case SmallGachaRecordPType:
		data := s.getData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(143, 8, rsp)

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSmallGacha, func() iface.IPlayerYY {
		return &SmallGacha{}
	})

	net.RegisterYYSysProtoV2(143, 6, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SmallGacha).c2sGacha
	})
	net.RegisterYYSysProtoV2(143, 7, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SmallGacha).c2sNext
	})
	net.RegisterYYSysProtoV2(143, 8, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SmallGacha).c2sRecord
	})
}
