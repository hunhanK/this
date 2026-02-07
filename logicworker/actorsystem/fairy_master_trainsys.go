package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type FairyMasterTrainSys struct {
	Base
	data *pb3.FairyMasterData
}

func (sys *FairyMasterTrainSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *FairyMasterTrainSys) init() bool {
	if sys.GetBinaryData().FairyMasterData == nil {
		data := &pb3.FairyMasterData{}
		sys.GetBinaryData().FairyMasterData = data
	}

	sys.data = sys.GetBinaryData().GetFairyMasterData()

	if sys.data.RewardedStage2Reward == nil {
		sys.data.RewardedStage2Reward = make(map[int32]bool)
	}
	return true
}

func (sys *FairyMasterTrainSys) OnOpen() {
	sys.init()
	sys.s2cInfo()
}

func (sys *FairyMasterTrainSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *FairyMasterTrainSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *FairyMasterTrainSys) s2cInfo() {
	res := &pb3.S2C_45_0{
		Info: sys.data,
	}

	sys.SendProto3(45, 0, res)
}

func (sys *FairyMasterTrainSys) c2sInfo(_ *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *FairyMasterTrainSys) c2sAttack(msg *base.Message) error {
	var req pb3.C2S_17_190
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("Enter FairyMasterTrain failed unpack msg failed %s", err)
	}

	curLevel := sys.data.Level
	if curLevel == 0 && req.Level != 1 {
		return neterror.ParamsInvalidError("Enter FairyMasterTrain failed level %d", req.Level)
	}

	if curLevel > 0 {
		fightValue := sys.owner.GetExtraAttr(attrdef.FightValue)
		curLvConf := jsondata.GetFairyMasterTrainLevelConf(curLevel)
		if req.Level == int32(curLvConf.SkipLayer) {
			if fightValue < curLvConf.SkipFightVal {
				return neterror.ParamsInvalidError("Enter FairyMasterTrain failed level %d", req.Level)
			}
		} else {
			if req.Level > sys.data.Level+1 || req.Level <= sys.data.Level {
				return neterror.ParamsInvalidError("Enter FairyLand failed level %d", req.Level)
			}
		}
	}

	levelConf := jsondata.GetFairyMasterTrainLevelConf(req.Level)
	if levelConf == nil {
		return neterror.ParamsInvalidError("Enter FairyMasterTrain failed conf is nil for level %d", req.Level)
	}

	if levelConf.RequiredJingjieLevel > 0 {
		if sys.owner.GetExtraAttr(attrdef.Circle) < int64(levelConf.RequiredJingjieLevel) {
			return neterror.ParamsInvalidError("Enter FairyMasterTrain failed jingjie level %d", levelConf.RequiredJingjieLevel)
		}
	}

	if levelConf.RequiredMissionId > 0 {
		questSys, ok := sys.GetOwner().GetSysObj(sysdef.SiQuest).(*QuestSys)
		if !ok {
			return neterror.InternalError("Enter FairyMasterTrain failed assert quest sys failed")
		}

		if !questSys.IsFinishMainQuest(uint32(levelConf.RequiredMissionId)) {
			return neterror.ParamsInvalidError("Enter FairyMasterTrain failed mission id %d", levelConf.RequiredMissionId)
		}
	}

	err := sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.AttackFairyMasterTrain, &pb3.AttackFairyMasterTrain{
		Level:    req.Level,
		CurLevel: sys.data.Level,
	})

	if err != nil {
		return neterror.ParamsInvalidError("Enter FairyMasterTrain failed level %d err: %s", req.Level, err)
	}

	return nil
}
func (sys *FairyMasterTrainSys) c2sRewardStage2Reward(msg *base.Message) error {
	var req pb3.C2S_45_5
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("c2sRewardStage2Reward failed unpack msg failed %s", err)
	}

	if sys.data.Level < req.Level {
		return neterror.ParamsInvalidError("c2sRewardStage2Reward failed haven`t pass level %d", req.Level)
	}

	levelConf := jsondata.GetFairyMasterTrainLevelConf(req.Level)
	if levelConf == nil {
		return neterror.ParamsInvalidError("c2sRewardStage2Reward failed conf is nil for level %d", req.Level)
	}

	if levelConf.StageReward2 == nil {
		return neterror.ParamsInvalidError("c2sRewardStage2Reward failed reward conf is nil for level %d", req.Level)
	}

	rewardState := sys.data.RewardedStage2Reward[req.Level]
	if rewardState {
		return neterror.ParamsInvalidError("c2sRewardStage2Reward failed already rewarded for level %d", req.Level)
	}

	sys.data.RewardedStage2Reward[req.Level] = true

	rewardState = engine.GiveRewards(sys.GetOwner(), levelConf.StageReward2, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFairyMasterTrainStage2Reward,
	})

	if !rewardState {
		return neterror.InternalError("c2sRewardStage2Reward failed give reward failed for level %d", req.Level)
	}

	sys.SendProto3(45, 5, &pb3.S2C_45_5{
		Level:    req.Level,
		Rewarded: rewardState,
	})

	return nil
}

func (sys *FairyMasterTrainSys) onNewDay() {
	sys.s2cInfo()
}

func (sys *FairyMasterTrainSys) checkSrvFirstPass(layer int32, actorId uint64, name string) {
	mgr := gshare.GetFirstPassFairyMasterLayerMgr()
	_, ok := mgr[uint32(layer)]
	if ok {
		return
	}
	mgr[uint32(layer)] = actorId
	srvPassAwards := jsondata.GetFairyMasterTrainLevelConf(layer)
	if srvPassAwards != nil && len(srvPassAwards.SrvPassReward) != 0 {
		argStr, _ := mailargs.MarshalMailArg(&mailargs.CommonMailArgs{Str1: name, Digit1: int64(layer)})
		mailmgr.AddSrvMailStr(common.Mail_SrvPassAwards, argStr, srvPassAwards.SrvPassReward)
	}
}

func checkOutFairyMasterTrainFb(player iface.IPlayer, buf []byte) {
	sys, ok := player.GetSysObj(sysdef.SiFairyMasterTrain).(*FairyMasterTrainSys)
	if !ok || !sys.IsOpen() {
		player.LogError("checkOutFairyMasterTrainFb failed assert sys failed")
		return
	}

	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		player.LogError("checkoutFairyMasterTrainFb failed unpack msg failed %s", err)
		return
	}
	if len(req.ExData) == 0 {
		player.LogError("checkoutFairyMasterTrainFb failed exdata is empty")
		return
	}

	resp := &pb3.S2C_17_254{
		Settle: &req,
	}

	if req.Ret == custom_id.FbSettleResultWin {
		level := int32(req.ExData[0])
		preLevel := sys.data.Level
		sys.data.Level = level

		sys.owner.TriggerEvent(custom_id.AeFairyMasterTrainLevelChanged, level)
		player.SendProto3(45, 3, &pb3.S2C_45_3{Level: uint32(sys.data.Level)})

		player.TriggerQuestEvent(custom_id.QttUpgradeFairyMasterLevel, 0, int64(sys.data.Level))
		logworker.LogPlayerBehavior(player, pb3.LogId_LogPassFairyMasterTrain, &pb3.LogPlayerCounter{
			NumArgs: uint64(level),
			StrArgs: fmt.Sprintf("{\"actorLv\": %d,\"circle\": %d}", player.GetLevel(), player.GetCircle()),
		})

		var rewards jsondata.StdRewardVec
		for i := preLevel + 1; i <= level; i++ {
			lvConf := jsondata.GetFairyMasterTrainLevelConf(i)
			if lvConf == nil {
				continue
			}
			rewards = append(rewards, lvConf.NormalReward...)
			rewards = append(rewards, lvConf.StageReward...)
		}

		rewards = jsondata.MergeStdReward(rewards)
		if !engine.CheckBagSpaceByRewards(player, rewards) {
			engine.SendRewardsByEmail(player, uint16(jsondata.GetFairyMasterTrainCommonConf().Mail), nil, rewards, common.EngineGiveRewardParam{
				LogId: pb3.LogId_LogFairyMasterTrainFbLevelReward,
			})
			player.SendTipMsg(tipmsgid.BagIsFullAwardSendByMail)
		} else {
			if len(rewards) > 0 {
				if !engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{
					LogId: pb3.LogId_LogFairyMasterTrainFbLevelReward,
				}) {
					player.LogError("checkoutFairyMasterTrainFb failed give reward failed")
					return
				}
			}
		}

		resp.Settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(rewards)
		sys.GetOwner().UpdateStatics(model.FieldFairyMasterTrainLv_, level)
		sys.checkSrvFirstPass(level, sys.owner.GetId(), sys.owner.GetName())
	}

	player.SendProto3(17, 254, resp)
}

func gmFairyMasterTrainSetLevel(player iface.IPlayer, args ...string) bool {
	sys, ok := player.GetSysObj(sysdef.SiFairyMasterTrain).(*FairyMasterTrainSys)

	if !ok || !sys.IsOpen() {
		return ok
	}

	if len(args) < 1 {
		return false
	}

	lv := utils.AtoUint32(args[0])

	sys.data.Level = int32(lv)

	sys.s2cInfo()
	return true
}

func init() {
	RegisterSysClass(sysdef.SiFairyMasterTrain, func() iface.ISystem {
		return &FairyMasterTrainSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.CheckOutFairyMasterTrainFb, checkOutFairyMasterTrainFb)

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiFairyMasterTrain).(*FairyMasterTrainSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	gmevent.Register("fairyMasterTrainSetLevel", gmFairyMasterTrainSetLevel, 1)

	net.RegisterSysProto(45, 0, sysdef.SiFairyMasterTrain, (*FairyMasterTrainSys).c2sInfo)
	net.RegisterSysProtoV2(17, 190, sysdef.SiFairyMasterTrain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FairyMasterTrainSys).c2sAttack
	})

	net.RegisterSysProtoV2(45, 5, sysdef.SiFairyMasterTrain, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FairyMasterTrainSys).c2sRewardStage2Reward
	})
}
