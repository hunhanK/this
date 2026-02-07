package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type GoldenPigTreasureNestSys struct {
	Base
}

func (s *GoldenPigTreasureNestSys) getData() *pb3.GoldenPigTreasureNest {
	data := s.GetBinaryData().GoldenPigTreasureNest
	if data == nil {
		s.GetBinaryData().GoldenPigTreasureNest = &pb3.GoldenPigTreasureNest{}
		data = s.GetBinaryData().GoldenPigTreasureNest
	}

	if data.Rec == nil {
		data.Rec = make(map[uint32]bool)
	}

	return data
}

func (s *GoldenPigTreasureNestSys) s2cInfo() {
	s.SendProto3(63, 8, &pb3.S2C_63_8{
		GoldenPigTreasureNest: s.getData(),
	})
}

func (s *GoldenPigTreasureNestSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GoldenPigTreasureNestSys) OnLogin() {
	s.s2cInfo()
}

func (s *GoldenPigTreasureNestSys) OnOpen() {
	data := s.getData()
	now := time_util.NowSec()
	data.StartTime = time_util.GetZeroTime(now)
	s.s2cInfo()
}

func (s *GoldenPigTreasureNestSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_63_9
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	loopConf, ok := jsondata.GetGoldenPigTreasureNestConf()
	if !ok {
		return neterror.ConfNotFoundError("GoldenPigTreasureNest conf is nil")
	}

	data := s.getData()

	pool := new(random.Pool)
	for _, v := range loopConf.Pool {
		st := v
		pool.AddItem(st, st.Weight)
	}

	if pool.Size() == 0 {
		return neterror.ParamsInvalidError("pool is empty")
	}

	var rands []*jsondata.GoldenPigNestPool
	for i := uint32(0); i < req.Times; i++ {
		rand := pool.RandomOne().(*jsondata.GoldenPigNestPool)
		rands = append(rands, rand)
	}

	consume := jsondata.GetGoldenPigNestConsume(req.Times)

	if len(consume) == 0 {
		return neterror.ParamsInvalidError("consume conf not found")
	}

	if !s.owner.ConsumeByConf(consume, req.AutoBuy, common.ConsumeParams{
		LogId: pb3.LogId_LogGoldenPigTreasureNestConsume,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.TotalTimes += req.Times
	rsp := &pb3.S2C_63_9{
		Times:      req.Times,
		TotalTimes: data.TotalTimes,
	}

	var allRewards jsondata.StdRewardVec
	awards := make(map[uint32]uint32)
	nowSec := time_util.NowSec()
	actorName := s.owner.GetName()
	actorId := s.owner.GetId()

	for _, randItem := range rands {
		record := &pb3.PigNestRecord{
			AwardId:   randItem.Id,
			ItemId:    randItem.Rewards[0].Id,
			Count:     uint32(randItem.Rewards[0].Count),
			TimeStamp: nowSec,
			ActorId:   actorId,
			ActorName: actorName,
		}

		if randItem.IsRare {
			globalRecord := s.getGlobalRecord()
			globalRecord.SuperRecords = append(globalRecord.SuperRecords, record)
			if uint32(len(globalRecord.SuperRecords)) > loopConf.RecordNum {
				globalRecord.SuperRecords = globalRecord.SuperRecords[1:]
			}
		}
		awards[randItem.Rewards[0].Id] += uint32(randItem.Rewards[0].Count)
		allRewards = append(allRewards, randItem.Rewards...)
	}

	engine.GiveRewards(s.owner, allRewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGoldenPigTreasureNestReward,
	})

	rsp.Awards = awards
	s.SendProto3(63, 9, rsp)

	return nil
}

// 检查是否需要重置累抽奖励
func (s *GoldenPigTreasureNestSys) checkResetAccReward() {
	conf, ok := jsondata.GetGoldenPigTreasureNestConf()
	if !ok {
		return
	}
	data := s.getData()
	now := time_util.NowSec()
	today := time_util.GetZeroTime(now)
	resetDays := uint32(conf.Reset) * 24 * 60 * 60
	if data.StartTime == 0 || today < resetDays+data.StartTime {
		return
	}
	var rewards jsondata.StdRewardVec
	for _, accConf := range conf.AccDrawReward {
		if accConf.DrawTimes > data.TotalTimes {
			continue
		}
		if data.Rec[accConf.DrawTimes] {
			continue
		}
		rewards = append(rewards, accConf.Rewards...)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.owner.GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}

	data.TotalTimes = 0
	data.Rec = make(map[uint32]bool)
	data.StartTime = today
	s.s2cInfo()
}

// 领取累抽奖励
func (s *GoldenPigTreasureNestSys) c2sGetAccReward(msg *base.Message) error {
	var req pb3.C2S_63_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetGoldenPigTreasureNestConf()
	if !ok {
		return neterror.ConfNotFoundError("GoldenPigTreasureNest conf is nil")
	}

	data := s.getData()

	var allRewards jsondata.StdRewardVec

	for _, rewardConf := range conf.AccDrawReward {
		if data.Rec[rewardConf.DrawTimes] {
			continue
		}
		if data.TotalTimes < rewardConf.DrawTimes {
			continue
		}
		allRewards = append(allRewards, rewardConf.Rewards...)
		data.Rec[rewardConf.DrawTimes] = true
	}

	engine.GiveRewards(s.owner, allRewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGoldenPigTreasureNestReward,
	})

	var recIds []uint32
	for k := range data.Rec {
		if data.Rec[k] {
			recIds = append(recIds, k)
		}
	}
	s.SendProto3(63, 11, &pb3.S2C_63_11{
		RecIds: recIds,
	})

	return nil
}

func (s *GoldenPigTreasureNestSys) getGlobalRecord() *pb3.GoldenPigTreasureNestList {
	globalVar := gshare.GetStaticVar()
	if globalVar.GoldenPigTreasureNestList == nil {
		globalVar.GoldenPigTreasureNestList = &pb3.GoldenPigTreasureNestList{}
	}
	return globalVar.GoldenPigTreasureNestList
}

// 获取抽奖记录
func (s *GoldenPigTreasureNestSys) c2sGetRecord(msg *base.Message) error {
	var req pb3.C2S_63_10
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	data := s.getGlobalRecord()
	rsp := &pb3.S2C_63_10{
		PigNestRecord: data.SuperRecords,
	}

	s.SendProto3(63, 10, rsp)

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiGoldenPigTreasureNest, func() iface.ISystem {
		return &GoldenPigTreasureNestSys{}
	})

	net.RegisterSysProtoV2(63, 9, sysdef.SiGoldenPigTreasureNest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GoldenPigTreasureNestSys).c2sDraw
	})
	net.RegisterSysProtoV2(63, 10, sysdef.SiGoldenPigTreasureNest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GoldenPigTreasureNestSys).c2sGetRecord
	})
	net.RegisterSysProtoV2(63, 11, sysdef.SiGoldenPigTreasureNest, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GoldenPigTreasureNestSys).c2sGetAccReward
	})
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		s, ok := player.GetSysObj(sysdef.SiGoldenPigTreasureNest).(*GoldenPigTreasureNestSys)
		if !ok || !s.IsOpen() {
			return
		}
		s.checkResetAccReward()
	})
}
