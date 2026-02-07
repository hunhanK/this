package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"math"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type FairyLandSys struct {
	Base
	data *pb3.FairyLandData
}

func (sys *FairyLandSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *FairyLandSys) init() bool {
	if sys.GetBinaryData().FairyLandData == nil {
		data := &pb3.FairyLandData{}
		sys.GetBinaryData().FairyLandData = data
	}

	sys.data = sys.GetBinaryData().GetFairyLandData()

	if sys.data.TreasureHunt == nil {
		sys.data.TreasureHunt = &pb3.FairyLandTreasureHuntData{}
	}

	if sys.data.TreasureHunt.StoredReward == nil {
		sys.data.TreasureHunt.StoredReward = make([]*pb3.StdAward, 0)
	}

	if sys.data.RewardInfo == nil {
		sys.data.RewardInfo = make(map[int32]bool)
	}
	return true
}

func (sys *FairyLandSys) OnOpen() {
	sys.init()
	now := uint32(time.Now().Unix())
	sys.data.TreasureHunt.StartTimestamp = now
	sys.data.TreasureHunt.LastStoreRewardTimeStamp = now
	sys.s2cInfo()
}

func (sys *FairyLandSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *FairyLandSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *FairyLandSys) s2cInfo() {
	res := &pb3.S2C_33_0{
		Info: &pb3.FairyLandInfo{
			Level:        sys.data.Level,
			TreasureHunt: sys.data.TreasureHunt,
			RewardInfo:   sys.data.RewardInfo,
		},
	}

	sys.SendProto3(33, 0, res)
}

func (sys *FairyLandSys) c2sInfo(*base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *FairyLandSys) c2sLevelPercent(_ *base.Message) error {
	rank := manager.GetFairyLandRankMgr()
	if rank == nil {
		sys.SendProto3(33, 5, &pb3.S2C_33_5{
			Percent: 0,
		})
		return nil
	}

	if !rank.IsInScoreMap(sys.GetOwner().GetId()) {
		sys.SendProto3(33, 5, &pb3.S2C_33_5{
			Percent: 0,
		})
		return nil
	}

	sumRankCount := rank.GetRankCount()
	no := rank.GetRankById(sys.GetOwner().GetId())

	percent := (1 - float64(no)/float64(sumRankCount)) * 100
	if no == 1 && sumRankCount == 1 {
		percent = 100
	}

	sys.LogDebug("c2sLevelPercent currentRank %d sumRankCount %d percent %f", no, sumRankCount, percent)

	sys.SendProto3(33, 5, &pb3.S2C_33_5{
		Percent: uint32(math.Ceil(float64(percent))),
	})

	return nil
}

func (sys *FairyLandSys) c2sAttackFairyLand(msg *base.Message) error {
	var req pb3.C2S_17_170
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("Enter FairyLand failed unpack msg failed %s", err)
	}

	curLevel := sys.data.Level
	if curLevel == 0 && req.Level != 1 {
		return neterror.ParamsInvalidError("Enter FairyLand failed level %d", req.Level)
	}

	fairySys, ok := sys.GetOwner().GetSysObj(sysdef.SiFairy).(*FairySystem)
	if !ok || !fairySys.IsOpen() {
		return neterror.ParamsInvalidError("fairy not open")
	}

	if fairySys.battlePos == 0 {
		return neterror.ParamsInvalidError("not fairy battle")
	}

	if fairySys.checkDieAll() {
		sys.GetOwner().SendTipMsg(tipmsgid.TpFairyIsDeath)
		return nil
	}

	fightValue := fairySys.getFightValue()
	lvConf := jsondata.GetFairyLandLevelConf(curLevel)
	if req.Level == int32(lvConf.SkipLayer) {
		if fightValue < lvConf.SkipFightVal {
			return neterror.ParamsInvalidError("Enter FairyLand failed level %d", req.Level)
		}
	} else {
		if req.Level > sys.data.Level+1 || req.Level <= sys.data.Level {
			return neterror.ParamsInvalidError("Enter FairyLand failed level %d", req.Level)
		}
	}

	err := sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.AttackFairyLand, &pb3.AttackFairyLand{
		Level:      req.Level,
		CurLevel:   sys.data.Level,
		FightValue: fightValue,
	})

	if err != nil {
		return neterror.ParamsInvalidError("Enter FairyLand failed level %d err: %s", req.Level, err)
	}

	return nil
}

func (sys *FairyLandSys) c2sTreasureHuntReward(msg *base.Message) error {
	var req pb3.C2S_33_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("TreasureHunt failed unpack msg failed %s", err)
	}

	rewards := sys.calcTreasureHuntReward()

	maxHuntHour := jsondata.GetFairyLandMaxHountHour()
	if maxHuntHour == 0 {
		return neterror.InternalError("reward failed maxHuntHour is 0")
	}

	now := time.Now()
	sys.data.TreasureHunt.StartTimestamp = uint32(now.Unix())
	sys.data.TreasureHunt.LastStoreRewardTimeStamp = uint32(now.Unix())
	sys.data.TreasureHunt.StoredReward = make([]*pb3.StdAward, 0)

	if len(rewards) > 0 {
		engine.GiveRewards(sys.GetOwner(), jsondata.Pb3RewardVecToStdRewardVec(rewards), common.EngineGiveRewardParam{
			NoTips: false,
			LogId:  pb3.LogId_LogFairyLandTreasureHunt,
		})
	}

	sys.SendProto3(33, 1, &pb3.S2C_33_1{
		Data: sys.data.TreasureHunt,
	})
	return nil
}

func (sys *FairyLandSys) c2sTreasureHuntQuick(*base.Message) error {
	level := sys.data.Level

	buyTimes := sys.data.TreasureHunt.QuickTimes - jsondata.GetFairyLandCommonConf().QuickHuntFreeTimes
	if buyTimes < 0 {
		buyTimes = 0
	}

	freeTimes := sys.data.TreasureHunt.QuickTimes - buyTimes

	sumTimes := jsondata.GetFairyLandCommonConf().QuickHuntFreeTimes + jsondata.GetFairyLandMaxQuickTreasureHuntBuyNums()

	if sys.data.TreasureHunt.QuickTimes >= sumTimes {
		return neterror.ParamsInvalidError("TreasureHuntQuick failed freeTimes %d buyTimes %d", freeTimes, buyTimes)
	}

	if freeTimes >= jsondata.GetFairyLandCommonConf().QuickHuntFreeTimes {
		consume := jsondata.GetFairyLandTreasureHuntQuickConsume(buyTimes + 1)
		if consume != nil {
			state := sys.GetOwner().ConsumeByConf([]*jsondata.Consume{consume.Consume}, false, common.ConsumeParams{LogId: pb3.LogId_LogFairyLandQuickHunt})
			if !state {
				return neterror.ParamsInvalidError("TreasureHuntQuick failed consume")
			}
		}
	}

	lvConf := jsondata.GetFairyLandLevelConf(level)
	if lvConf == nil {
		return neterror.ParamsInvalidError("TreasureHuntQuick failed level %d", level)
	}

	sys.data.TreasureHunt.QuickTimes += 1

	reward := jsondata.StdRewardMulti(lvConf.TreasureHuntEffect, int64(jsondata.GetFairyLandCommonConf().QuickHuntSumHour)*60)

	state := engine.GiveRewards(sys.GetOwner(), reward, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogFairyLandQuickHunt,
		NoTips: true,
	})

	if !state {
		return neterror.InternalError("TreasureHuntQuick failed give reward")
	}

	sys.SendProto3(33, 2, &pb3.S2C_33_2{
		QuickTreasureHuntTimes: sys.data.TreasureHunt.QuickTimes,
		Rewards:                jsondata.StdRewardVecToPb3RewardVec(reward),
	})

	return nil
}

func (sys *FairyLandSys) c2sLevelReward(msg *base.Message) error {
	var req pb3.C2S_33_4
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("LevelReward failed unpack msg failed %s", err)
	}

	lvConf := jsondata.GetFairyLandLevelConf(req.Level)
	if lvConf == nil {
		return neterror.ParamsInvalidError("LevelReward failed level %d", req.Level)
	}

	if sys.data.Level < req.Level {
		return neterror.ParamsInvalidError("LevelReward failed level not meet %d", req.Level)
	}

	rewarded := sys.data.RewardInfo[req.Level]
	if rewarded {
		return neterror.ParamsInvalidError("LevelReward failed already rewarded")
	}

	// normalReward := lvConf.NormalReward
	specialReward := lvConf.SpecialReward

	if specialReward == nil {
		return neterror.ParamsInvalidError("no special rewards")
	}

	// rewards := jsondata.MergeStdReward(normalReward, specialReward)

	sys.data.RewardInfo[req.Level] = true

	status := engine.GiveRewards(sys.GetOwner(), specialReward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyLandSpecialLevelReward,
	})

	if !status {
		return neterror.InternalError("LevelReward failed give reward")
	}

	sys.SendProto3(33, 4, &pb3.S2C_33_4{
		Level: req.Level,
	})
	return nil
}

func (sys *FairyLandSys) calcTreasureHuntReward() []*pb3.StdAward {
	startTime := time.Unix(int64(sys.data.TreasureHunt.StartTimestamp), 0)
	lastStoreTime := time.Unix(int64(sys.data.TreasureHunt.LastStoreRewardTimeStamp), 0)

	begin := startTime

	if lastStoreTime.After(startTime) {
		begin = lastStoreTime
	}
	maxHuntHour := jsondata.GetFairyLandMaxHountHour()
	ceaseHuntTime := startTime.Add(time.Hour * time.Duration(maxHuntHour))

	end := time.Now()
	if ceaseHuntTime.Before(end) {
		end = ceaseHuntTime
	}

	minutes := int(math.Floor(end.Sub(begin).Minutes()))
	lvConf := jsondata.GetFairyLandLevelConf(sys.data.Level)
	if lvConf == nil {
		return sys.data.TreasureHunt.StoredReward
	}

	effctientConf := lvConf.TreasureHuntEffect

	effectientReward := jsondata.StdRewardMulti(effctientConf, int64(minutes))

	sys.LogInfo("calcTreasureHuntReward minutes %d effectientReward %v", minutes, effectientReward)
	sys.LogInfo("calcTrsureHuntReward begin %v end %v", begin, end)
	sys.LogInfo("calcTreasureHuntReward stored reward %v", sys.data.TreasureHunt.StoredReward)

	pbRewards := jsondata.Pb3RewardMergeStdReward(sys.data.TreasureHunt.StoredReward, effectientReward)

	return pbRewards
}

func (sys *FairyLandSys) getLevel() int32 {
	if !sys.IsOpen() {
		return 0
	}
	return sys.data.Level
}

func (sys *FairyLandSys) onNewDay() {
	sys.data.TreasureHunt.QuickTimes = 0
	sys.s2cInfo()
}

func (sys *FairyLandSys) tryStoreTreasureHuntReward() {
	oldLvConf := jsondata.GetFairyLandLevelConf(sys.data.Level)
	startTime := time.Unix(int64(sys.data.TreasureHunt.StartTimestamp), 0)
	lastStoreTime := time.Unix(int64(sys.data.TreasureHunt.LastStoreRewardTimeStamp), 0)
	maxHuntHour := jsondata.GetFairyLandMaxHountHour()
	ceaseHuntTime := startTime.Add(time.Hour * time.Duration(maxHuntHour))
	if lastStoreTime.Before(ceaseHuntTime) {
		begin := lastStoreTime
		end := time.Now()
		// 防止溢出
		if ceaseHuntTime.Before(end) {
			end = ceaseHuntTime
		}
		minutes := int64(math.Ceil(end.Sub(begin).Minutes()))

		if minutes > 0 {
			if oldLvConf != nil {
				effectientConf := oldLvConf.TreasureHuntEffect
				effectient := jsondata.StdRewardMulti(effectientConf, int64(minutes))
				sys.data.TreasureHunt.StoredReward =
					jsondata.Pb3RewardMergeStdReward(sys.data.TreasureHunt.StoredReward, effectient)
				sys.LogInfo("level %d minutes %d stored reward %v",
					oldLvConf.LevelId, minutes,
					sys.data.TreasureHunt.StoredReward)
				sys.LogInfo("Begin %v End %v", begin, end)

			}
			sys.data.TreasureHunt.LastStoreRewardTimeStamp = uint32(end.Unix())
		}
	}
}

func (sys *FairyLandSys) updateRank(score int64) {
	limit := jsondata.GetFairyLandCommonConf().RankCond

	if int64(limit) > score {
		return
	}

	manager.GetFairyLandRankMgr().Update(sys.GetOwner().GetId(), score)
}

func checkOutFairyLandFb(player iface.IPlayer, buf []byte) {
	sys, ok := player.GetSysObj(sysdef.SiFairyLand).(*FairyLandSys)
	if !ok || !sys.IsOpen() {
		player.LogError("checkOutFairyLandFb failed assert sys failed")
		return
	}

	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("checkOutFairyLandFb failed unpack msg failed %s", err)
		return
	}
	if len(req.ExData) == 0 {
		player.LogError("checkOutFairyLandFb failed exdata is empty")
		return
	}

	resp := &pb3.S2C_17_254{
		Settle: &req,
	}

	if req.Ret == custom_id.FbSettleResultWin {
		sys.tryStoreTreasureHuntReward()

		level := int32(req.ExData[0])
		preLevel := sys.data.Level
		sys.data.Level = level

		player.SendProto3(33, 3, &pb3.S2C_33_3{Level: uint32(sys.data.Level)})
		sys.updateRank(int64(level))
		sys.owner.TriggerQuestEvent(custom_id.QttUpgradeFairyLandLevel, 0, int64(sys.data.Level))

		var rewards jsondata.StdRewardVec
		for i := preLevel + 1; i <= level; i++ {
			lvConf := jsondata.GetFairyLandLevelConf(i)
			if lvConf == nil {
				continue
			}
			rewards = append(rewards, lvConf.NormalReward...)
		}

		if len(rewards) > 0 {
			if !engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogFairyLandLevelReward,
			}) {
				player.LogError("checkOutFairyLandFb failed give reward failed")
				return
			}

			resp.Settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(rewards)
		}
	}

	player.SendProto3(17, 254, resp)
}

func gmAttackFairyLand(player iface.IPlayer, _ ...string) bool {
	fairyLandSys, ok := player.GetSysObj(sysdef.SiFairyLand).(*FairyLandSys)
	if !ok || !fairyLandSys.IsOpen() {
		return ok
	}

	req := &pb3.C2S_17_170{
		Level: fairyLandSys.data.Level + 1,
	}

	msg := base.NewMessage()
	err := msg.PackPb3Msg(req)
	if err != nil {
		return false
	}

	if err := fairyLandSys.c2sAttackFairyLand(msg); err != nil {
		player.LogError("gmAttackFairyLand failed %s", err)
		return false
	}
	return true
}

func gmfairylandSetFb(player iface.IPlayer, args ...string) bool {
	fairyLandSys, ok := player.GetSysObj(sysdef.SiFairyLand).(*FairyLandSys)
	if !ok || !fairyLandSys.IsOpen() {
		return ok
	}

	if len(args) < 1 {
		return false
	}

	lv := utils.AtoUint32(args[0])

	fairyLandSys.data.Level = int32(lv)

	fairyLandSys.c2sInfo(nil)
	return true
}

func init() {
	RegisterSysClass(sysdef.SiFairyLand, func() iface.ISystem {
		return &FairyLandSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.CheckOutFairyLandFuben, checkOutFairyLandFb)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiFairyLand).(*FairyLandSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	gmevent.Register("attackFairyLandFb", gmAttackFairyLand, 1)
	gmevent.Register("fairylandSetLevel", gmfairylandSetFb, 1)

	net.RegisterSysProto(33, 0, sysdef.SiFairyLand, (*FairyLandSys).c2sInfo)
	net.RegisterSysProto(33, 1, sysdef.SiFairyLand, (*FairyLandSys).c2sTreasureHuntReward)
	net.RegisterSysProto(33, 2, sysdef.SiFairyLand, (*FairyLandSys).c2sTreasureHuntQuick)
	net.RegisterSysProto(33, 4, sysdef.SiFairyLand, (*FairyLandSys).c2sLevelReward)
	net.RegisterSysProto(33, 5, sysdef.SiFairyLand, (*FairyLandSys).c2sLevelPercent)

	net.RegisterSysProto(17, 170, sysdef.SiFairyLand, (*FairyLandSys).c2sAttackFairyLand)
}
