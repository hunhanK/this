/**
 * @Author: LvYuMeng
 * @Date: 2025/1/8
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type BossSweepSys struct {
	Base
}

func (s *BossSweepSys) s2cInfo() {
	s.SendProto3(161, 12, &pb3.S2C_161_12{Data: s.getData()})
}

func (s *BossSweepSys) getRecord(recordType, id uint32) (*pb3.BossSweepKillRecord, bool) {
	recordSt, ok := s.getRecordSt(recordType)
	if !ok {
		return nil, false
	}
	killData, ok := recordSt.Records[id]

	return killData, ok
}

func (s *BossSweepSys) getRecordSt(recordType uint32) (*pb3.BossSweepDailyRecord, bool) {
	if recordType > BossSweepTypeBossMax {
		return nil, false
	}
	data := s.getData()
	record, ok := data[recordType]
	if !ok {
		record = &pb3.BossSweepDailyRecord{}
		data[recordType] = record
	}
	if nil == record.Records {
		record.Records = map[uint32]*pb3.BossSweepKillRecord{}
	}
	return record, true
}

func (s *BossSweepSys) record(recordType uint32, monId, sceneId, fbId uint32) {
	recordSt, ok := s.getRecordSt(recordType)
	if !ok {
		return
	}

	Id := uint32(len(recordSt.Records)) + 1
	newRecord := &pb3.BossSweepKillRecord{
		Id:        Id,
		SceneId:   sceneId,
		FbId:      fbId,
		MonId:     monId,
		Timestamp: time_util.NowSec(),
	}
	recordSt.Records[Id] = newRecord

	canSweepNow := false
	if ins, ok := newBossSweepIns(recordType, s.owner); ok {
		canSweepNow = ins.BossSweepChecker(monId)
	}

	s.SendProto3(161, 13, &pb3.S2C_161_13{
		Type:        recordType,
		Record:      newRecord,
		CanSweepNow: canSweepNow,
	})
}

func (s *BossSweepSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *BossSweepSys) OnReconnect() {
	s.s2cInfo()
}

func (s *BossSweepSys) getData() map[uint32]*pb3.BossSweepDailyRecord {
	binary := s.owner.GetBinaryData()
	if nil == binary.BossSweepData {
		binary.BossSweepData = map[uint32]*pb3.BossSweepDailyRecord{}
	}
	return binary.BossSweepData
}

func (s *BossSweepSys) onNewDay() {
	s.owner.GetBinaryData().BossSweepData = nil
	s.s2cInfo()
}

const (
	BossSweepTypeQiMenBoss          = 1 // 奇门Boss
	BossSweepTypeMergeChallengeBoss = 2 // 合服Boss
	BossSweepTypeBossMax            = iota
)

func newBossSweepIns(recordType uint32, player iface.IPlayer) (iface.IBossSweep, bool) {
	switch recordType {
	case BossSweepTypeQiMenBoss:
		return newBossSweepQiMenIns(player)
	case BossSweepTypeMergeChallengeBoss:
		return newBossSweepMergeChallengeIns(player)
	}
	return nil, false
}

func newBossSweepQiMenIns(player iface.IPlayer) (iface.IBossSweep, bool) {
	sys, ok := player.GetSysObj(sysdef.SiQiMen).(*QiMenSys)
	if !ok || !sys.IsOpen() {
		return nil, false
	}
	return sys, true
}

func newBossSweepMergeChallengeIns(player iface.IPlayer) (iface.IBossSweep, bool) {
	sys, ok := player.GetSysObj(sysdef.SiKillToken).(*KillTokenSys)
	if !ok || !sys.IsOpen() {
		return nil, false
	}
	return sys, true
}

func (s *BossSweepSys) c2sQuickAttack(msg *base.Message) error {
	var req pb3.C2S_161_11
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	killData, ok := s.getRecord(req.Type, req.Id)
	if !ok || killData.IsFinish {
		return neterror.ParamsInvalidError("cannot quick attack")
	}

	ins, ok := newBossSweepIns(req.Type, s.owner)
	if !ok {
		return neterror.ParamsInvalidError("checker failed")
	}

	if !ins.BossSweepChecker(killData.MonId) {
		return neterror.ParamsInvalidError("checker failed")
	}

	err = s.owner.CallActorFunc(actorfuncid.G2FBossSweepQuickReward, &pb3.BossSweepQuickRewardReq{
		Id:         req.Id,
		SceneId:    killData.SceneId,
		MonId:      killData.MonId,
		RecordType: req.Type,
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *BossSweepSys) onBossSweepQuickReward(req *pb3.BossSweepQuickRewardRet) {
	killData, ok := s.getRecord(req.RecordType, req.Id)
	if !ok || killData.IsFinish {
		return
	}

	rewards := jsondata.Pb3RewardVecToStdRewardVec(req.Rewards)

	ins, ok := newBossSweepIns(req.RecordType, s.owner)
	if !ok {
		return
	}

	if !ins.BossSweepSettle(req.MonId, req.SceneId, rewards) {
		return
	}

	killData.IsFinish = true

	s.owner.FinishBossQuickAttack(killData.MonId, killData.SceneId, killData.FbId)

	s.SendProto3(161, 11, &pb3.S2C_161_11{
		Type: req.RecordType,
		Id:   req.Id,
	})
}

func (s *BossSweepSys) onVestKillMon(monsterId, sceneId, fbId uint32) {
	mConf := jsondata.GetMonsterConf(monsterId)
	if nil == mConf {
		return
	}
	if mConf.SweepType == 0 {
		return
	}

	s.record(mConf.SweepType, monsterId, sceneId, fbId)
}

func onBossSweepQuickReward(player iface.IPlayer, buf []byte) {
	var req pb3.BossSweepQuickRewardRet
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("onWorldBossQuickReward Unmarshal failed err: %s", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiBossSweep).(*BossSweepSys)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.onBossSweepQuickReward(&req)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogQiMenBossSweepAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.RecordType),
		StrArgs: fmt.Sprintf("%d_%d", req.SceneId, req.MonId),
	})
}

func init() {
	RegisterSysClass(sysdef.SiBossSweep, func() iface.ISystem {
		return &BossSweepSys{}
	})

	net.RegisterSysProtoV2(161, 11, sysdef.SiBossSweep, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BossSweepSys).c2sQuickAttack
	})

	engine.RegisterActorCallFunc(playerfuncid.BossSweepQuickReward, onBossSweepQuickReward)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiBossSweep).(*BossSweepSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	event.RegActorEvent(custom_id.AeVestKillMon, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiBossSweep).(*BossSweepSys)
		if !ok || !sys.IsOpen() {
			return
		}
		if len(args) < 4 {
			return
		}
		monsterId, ok := args[0].(uint32)
		if !ok {
			return
		}

		sceneId, ok := args[1].(uint32)
		if !ok {
			return
		}

		fbId, ok := args[3].(uint32)
		if !ok {
			return
		}

		sys.onVestKillMon(monsterId, sceneId, fbId)
	})
}
