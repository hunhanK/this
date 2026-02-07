/**
 * @Author: lzp
 * @Date: 2025/6/27
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
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
)

const (
	MaxLayer = 5
)

const (
	SprintRankRecordGlobal = 1
	SprintRankRecordPerson = 2
)

type SprintRankTreasury struct {
	PlayerYYBase
}

func (s *SprintRankTreasury) OnReconnect() {
	s.s2cInfo()
}

func (s *SprintRankTreasury) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *SprintRankTreasury) Login() {
	s.s2cInfo()
}

func (s *SprintRankTreasury) ResetData() {
	if s.GetYYData().SprintRankTreasury == nil {
		return
	}
	delete(s.GetYYData().SprintRankTreasury, s.Id)
}

func (s *SprintRankTreasury) GetData() *pb3.PYY_SprintRankTreasury {
	if s.GetYYData().SprintRankTreasury == nil {
		s.GetYYData().SprintRankTreasury = make(map[uint32]*pb3.PYY_SprintRankTreasury)
	}

	data, ok := s.GetYYData().SprintRankTreasury[s.GetId()]
	if !ok {
		data = &pb3.PYY_SprintRankTreasury{}
		s.GetYYData().SprintRankTreasury[s.GetId()] = data
	}
	if data.GetGoods == nil {
		data.GetGoods = make(map[uint32]uint32)
	}
	//
	if data.Layer == 0 {
		data.Layer = 1
	}
	return data
}

func (s *SprintRankTreasury) c2sGet(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_127_172
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYSprintRankTreasuryConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("get conf failed actId %d", s.GetId())
	}

	if s.checkIndex(req.Index) {
		return neterror.ParamsInvalidError("index: %d has get", req.Index)
	}

	data := s.GetData()
	if data.GetGrand {
		return neterror.ParamsInvalidError("has get grand")
	}

	times := data.Times + 1
	var consumes jsondata.ConsumeVec
	for _, tConf := range conf.TimesConsume {
		if data.Layer != tConf.Layer {
			continue
		}
		if times >= tConf.MinTimes && times <= tConf.MaxTimes {
			consumes = tConf.Consumes
			break
		}
	}
	if len(consumes) <= 0 {
		return neterror.ConfNotFoundError("consumes empty")
	}

	lConf := jsondata.GetPYYSprintRankTreasuryLayerConf(s.ConfName, s.ConfIdx, data.Layer)
	if lConf == nil {
		return neterror.ConfNotFoundError("layer: %d config not found", data.Layer)
	}

	pool := new(random.Pool)
	for k, v := range lConf.LayerRewards {
		if data.GetGoods[k] > 0 {
			continue
		}
		pool.AddItem(v, v.Weight)
	}

	rConf := pool.RandomOne().(*jsondata.SprintRankLayer)
	if rConf == nil {
		return neterror.ParamsInvalidError("layer rewards empty")
	}

	if len(consumes) == 0 || !s.GetPlayer().ConsumeByConf(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogPYYSprintRankTreasuryConsume}) {
		return neterror.ParamsInvalidError("consumes not enough")
	}

	data.GetGoods[rConf.Id] = req.Index
	data.Times = times
	if rConf.ShowLevel == 1 {
		data.GetGrand = true
	}

	player := s.GetPlayer()
	if len(rConf.Rewards) > 0 {
		engine.GiveRewards(player, rConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYSprintRankTreasuryRewards})
		record := &pb3.ItemGetRecord{
			ActorId:   player.GetId(),
			ActorName: player.GetName(),
			ItemId:    rConf.Rewards[0].Id,
			Count:     uint32(rConf.Rewards[0].Count),
			TimeStamp: time_util.NowSec(),
		}

		recordNum := int(conf.RecordNum)

		s.record(&data.Records, record, recordNum)
		globalRecord := s.getGlobalRecord()
		if rConf.ShowLevel == 1 {
			s.record(&globalRecord.HighRecords, record, recordNum)
		} else {
			s.record(&globalRecord.NormalRecords, record, recordNum)
		}
	}

	if rConf.ShowLevel == 1 {
		if data.Layer >= 1 && data.Layer <= 3 {
			engine.BroadcastTipMsgById(conf.Notice1, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, rConf.Rewards))
		} else {
			engine.BroadcastTipMsgById(conf.Notice2, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, rConf.Rewards))
		}
	}

	s.s2cInfo()
	return nil
}
func (s *SprintRankTreasury) checkIndex(index uint32) bool {
	data := s.GetData()
	for _, v := range data.GetGoods {
		if v == index {
			return true
		}
	}
	return false
}

func (s *SprintRankTreasury) c2sRefresh(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}

	var req pb3.C2S_127_173
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	if !data.GetGrand {
		return neterror.ParamsInvalidError("grand rewards not get")
	}

	s.refresh()
	s.s2cInfo()
	return nil
}

func (s *SprintRankTreasury) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_127_174
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYSprintRankTreasuryConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf not exist")
	}

	rsp := &pb3.S2C_127_174{ActiveId: s.Id, Type: req.GetType()}
	switch req.Type {
	case SprintRankRecordGlobal:
		rsp.Records = s.getGlobalRecord()
	case SprintRankRecordPerson:
		rsp.Records = &pb3.SprintRankRecord{NormalRecords: s.GetData().Records}
	}

	s.SendProto3(127, 174, rsp)
	return nil
}

func (s *SprintRankTreasury) s2cInfo() {
	s.SendProto3(127, 172, &pb3.S2C_127_172{
		ActId: s.Id,
		Data:  s.GetData(),
	})
}

func (s *SprintRankTreasury) refresh() {
	conf := jsondata.GetPYYSprintRankTreasuryConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	data.GetGoods = make(map[uint32]uint32)
	data.Layer = data.Layer%MaxLayer + 1
	data.GetGrand = false
	data.Times = 0

	for _, conf := range conf.SprintRankTreasury {
		if conf.Layer != data.Layer {
			continue
		}
		data.GetGoods = make(map[uint32]uint32)
	}
	s.s2cInfo()
}

func (s *SprintRankTreasury) record(records *[]*pb3.ItemGetRecord, record *pb3.ItemGetRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SprintRankTreasury) clearRecord() {
	if record := s.getGlobalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.SprintRankRecords, s.GetId())
	}
}

func (s *SprintRankTreasury) getGlobalRecord() *pb3.SprintRankRecord {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if nil == globalVar.PyyDatas.SprintRankRecords {
		globalVar.PyyDatas.SprintRankRecords = make(map[uint32]*pb3.SprintRankRecord)
	}
	if nil == globalVar.PyyDatas.SprintRankRecords[s.Id] {
		globalVar.PyyDatas.SprintRankRecords[s.Id] = &pb3.SprintRankRecord{}
	}
	if globalVar.PyyDatas.SprintRankRecords[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SprintRankRecords[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SprintRankRecords[s.Id]
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSprintRankTreasury, func() iface.IPlayerYY {
		return &SprintRankTreasury{}
	})

	net.RegisterYYSysProtoV2(127, 172, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SprintRankTreasury).c2sGet
	})
	net.RegisterYYSysProtoV2(127, 173, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SprintRankTreasury).c2sRefresh
	})
	net.RegisterYYSysProtoV2(127, 174, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SprintRankTreasury).c2sRecord
	})
}
