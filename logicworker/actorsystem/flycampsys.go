/**
 * @Author: lzp
 * @Date: 2024/9/12
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
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
	"jjyz/gameserver/logicworker/flycampmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"math"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type FlyCampSys struct {
	Base
}

func (sys *FlyCampSys) OnReconnect() {
	sys.syncFlyCamp()
	sys.s2cInfo()
}

func (sys *FlyCampSys) OnLogin() {
	sys.syncFlyCamp()
	sys.s2cInfo()
}

func (sys *FlyCampSys) OnOpen() {
	sys.s2cInfo()
}

func (sys *FlyCampSys) GetData() *pb3.FlyCampData {
	binary := sys.GetBinaryData()
	if binary.FlyCampData == nil {
		binary.FlyCampData = &pb3.FlyCampData{}
	}
	data := binary.FlyCampData
	if data.StageData == nil {
		data.StageData = make(map[uint32]*pb3.FlyCampStage)
	}
	return data
}

func (sys *FlyCampSys) c2sChangeCamp(msg *base.Message) error {
	var req pb3.C2S_166_21
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}

	commConf := jsondata.GetFlyCampCommonConf()
	if commConf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	data := sys.GetData()
	if data.Camp > 0 && !sys.checkFinish() {
		return neterror.ParamsInvalidError("fly camp cannot change")
	}

	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CFlyCampChangeReq, &pb3.G2CFlyCampChangeReq{
			ActorId: sys.owner.GetId(),
			PfId:    engine.GetPfId(),
			SrvId:   engine.GetServerId(),
			Camp:    req.Camp,
		}); err != nil {
			return neterror.ParamsInvalidError("err: %v", err)
		}
		return nil
	} else {
		campCount := flycampmgr.GetCampCount()
		if !sys.checkCanChangeFlyCamp(req.Camp, campCount) {
			return neterror.ParamsInvalidError("fly camp is count limit camp = %d", req.Camp)
		}

		// 未完成转职,选择职业
		if !sys.checkFinish() {
			sys.initFlyCamp(req.Camp)
			flycampmgr.AddCampCount(req.Camp)
			sys.SendProto3(166, 21, &pb3.S2C_166_21{Camp: req.Camp})
			return nil
		}

		// 已完成转职,更改职业
		consume := commConf.ChangeCampConsume
		if len(consume) > 0 && !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{
			LogId: pb3.LogId_LogFlyCampChangeConsume,
		}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}

		oldCamp := data.Camp
		data.Camp = req.Camp
		sys.onFlyCampChangeFinish()
		flycampmgr.OnChangeCamp(oldCamp, data.Camp)

		sys.SendProto3(166, 21, &pb3.S2C_166_21{Camp: req.Camp})
		return nil
	}
}

func (sys *FlyCampSys) c2sCompleteStage(msg *base.Message) error {
	var req pb3.C2S_166_22
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}

	data := sys.GetData()
	conf := jsondata.GetFlyCampConf(data.Camp)
	if conf == nil {
		return neterror.ConfNotFoundError("fly camp config not found id: %d", data.Camp)
	}
	if data.Camp == 0 {
		return neterror.ParamsInvalidError("not select camp")
	}

	stage := data.CurStage
	sData := data.StageData[stage]
	if sData == nil {
		sData = &pb3.FlyCampStage{Stage: stage}
		data.StageData[stage] = sData
	}

	//消耗
	sConf := jsondata.GetFlyCampStageConf(data.Camp, stage)
	if sConf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	if stage == conf.StageCond && sData.Num >= uint32(len(sConf.StageSlot)) {
		return neterror.ParamsInvalidError("fly camp stage max, stage: %d, num: %d", stage, sData.Num)
	}

	slotConf := sConf.StageSlot[sData.Num]
	if !sys.GetOwner().ConsumeByConf(slotConf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogFlyCampCompleteStageConsume,
	}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	sData.Num += 1
	// 检查是否到下阶段
	if sData.Num >= sConf.FinishCond {
		sData.IsFinish = true
		nextStage := stage + 1
		if nextStage <= conf.StageCond {
			data.CurStage = nextStage
			sys.owner.SetExtraAttr(attrdef.FlyCampStage, attrdef.AttrValueAlias(nextStage))
		}
	}

	sys.ResetSysAttr(attrdef.SaFlyCamp)
	sys.SendProto3(166, 22, &pb3.S2C_166_22{Stage: sData})
	sys.s2cInfo()
	return nil
}

func (sys *FlyCampSys) c2sCampCountReq(msg *base.Message) error {
	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CFlyCampCountReq, &pb3.G2CFlyCampCountReq{
			ActorId: sys.owner.GetId(),
			PfId:    engine.GetPfId(),
			SrvId:   engine.GetServerId(),
		}); err != nil {
			return neterror.ParamsInvalidError("err: %v", err)
		}
		return nil
	} else {
		campCount := flycampmgr.GetCampCount()
		sys.SendProto3(166, 25, &pb3.S2C_166_25{CampCount: campCount})
	}

	return nil
}

func (sys *FlyCampSys) c2sFlyCampFinish(msg *base.Message) error {
	data := sys.GetData()
	if !data.KillBoss {
		return neterror.ParamsInvalidError("not kill boss")
	}

	conf := jsondata.GetFlyCampConf(data.Camp)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	data.FinishTime = time_util.NowSec()
	data.IsFinish = true

	sys.onFlyCampChangeFinish()
	sys.ResetSysAttr(attrdef.SaFlyCamp)
	sys.owner.LearnSkill(conf.SkillId, 1, true)
	sys.owner.TriggerEvent(custom_id.AeFlyCampFinishChallenge)
	sys.GetOwner().TriggerQuestEventRange(custom_id.QttFinishFlyCamp)
	sys.sendRankRewards()

	sys.s2cInfo()
	return nil
}

func (sys *FlyCampSys) c2sAttack(msg *base.Message) error {
	var req pb3.C2S_166_23
	if err := msg.UnPackPb3Msg(&req); err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}

	data := sys.GetData()
	conf := jsondata.GetFlyCampConf(data.Camp)
	if conf == nil {
		return neterror.ConfNotFoundError("camp conf not found camp=%d", data.Camp)
	}

	if data.FinishTime > 0 {
		return neterror.ParamsInvalidError("has completed")
	}

	canAttack := true
	if len(data.StageData) == 0 {
		canAttack = false
	} else {
		for _, stage := range data.StageData {
			if !stage.IsFinish {
				canAttack = false
			}
		}
	}
	if !canAttack {
		return neterror.ParamsInvalidError("cannot attack")
	}

	err := sys.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterFlyCampBoss, &pb3.AttackFlyCampBoss{
		Camp: data.Camp,
	})
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}
	return nil
}

func (sys *FlyCampSys) initFlyCamp(camp uint32) {
	data := sys.GetData()
	data.Camp = camp
	data.CurStage = 1
	sys.owner.SetExtraAttr(attrdef.FlyCampStage, attrdef.AttrValueAlias(data.CurStage))
}

func (sys *FlyCampSys) handleSettlement(settle *pb3.FbSettlement) {
	if settle.Ret == custom_id.FbSettleResultWin {
		data := sys.GetData()
		data.KillBoss = true

		conf := jsondata.GetFlyCampConf(data.Camp)
		if !engine.GiveRewards(sys.GetOwner(),
			conf.Rewards,
			common.EngineGiveRewardParam{LogId: pb3.LogId_LogFlyCampFinishRewards}) {
			sys.owner.LogError("flyCamp handleSettlement failed give reward failed")
		}
		settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(conf.Rewards)
	}
	sys.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settle})
	sys.s2cInfo()
}

func (sys *FlyCampSys) s2cInfo() {
	data := sys.GetData()
	pbMsg := &pb3.S2C_166_20{Data: data}
	sys.SendProto3(166, 20, pbMsg)
}

func (sys *FlyCampSys) sendRankRewards() {
	commConf := jsondata.GetFlyCampCommonConf()
	if commConf == nil {
		return
	}

	data := sys.GetData()
	if data.IsGetRankRewards {
		return
	}

	data.IsGetRankRewards = true

	owner := sys.GetOwner()
	rankById := manager.GRankMgrIns.GetRankByType(gshare.RankTypeFlyCamp).GetRankById(owner.GetId())

	// 奖励通过邮件下发
	var awards jsondata.StdRewardVec
	for _, conf := range commConf.RankAwards {
		if conf.Min <= rankById && rankById <= conf.Max {
			awards = conf.Awards
			break
		}
	}
	if len(awards) > 0 {
		mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
			ConfId: commConf.RankMailId,
			Content: &mailargs.CommonMailArgs{
				Str1:   owner.GetName(),
				Digit1: int64(rankById),
			},
			Rewards: awards,
		})
	}
}

func (sys *FlyCampSys) syncFlyCamp() {
	sys.owner.SetExtraAttr(attrdef.FlyCampStage, attrdef.AttrValueAlias(sys.getCurStage()))
}

func (sys *FlyCampSys) onFlyCampChangeFinish() {
	camp := sys.GetData().Camp
	sys.owner.SetExtraAttr(attrdef.FlyCamp, attrdef.AttrValueAlias(camp))
	sys.owner.SetExtraAttr(attrdef.FlyCampStage, attrdef.AttrValueAlias(0))
	sys.owner.TriggerEvent(custom_id.AeFlyCampChange)
}

func (sys *FlyCampSys) checkFinish() bool {
	data := sys.GetData()
	return data.IsFinish
}

func (sys *FlyCampSys) getCamp() uint32 {
	data := sys.GetData()
	return data.Camp
}

// 获取当前处于飞升哪个阶段
// 返回0：还没选职业 || 完成转职
func (sys *FlyCampSys) getCurStage() uint32 {
	// 击杀boss 完成转职
	data := sys.GetData()
	if data.IsFinish {
		return 0
	}

	return data.CurStage
}

func (sys *FlyCampSys) checkCanChangeFlyCamp(camp uint32, campData map[uint32]uint32) bool {
	count1 := float64(campData[custom_id.FlyCampImmortals])
	count2 := float64(campData[custom_id.FlyCampCultivation])

	commConf := jsondata.GetFlyCampCommonConf()

	campCount := campData[camp]
	if campCount < commConf.CampLimit {
		return true
	}

	if uint32(count1) >= commConf.CampLimit && uint32(count2) >= commConf.CampLimit {
		if camp == custom_id.FlyCampImmortals {
			count1 += 1
		} else if camp == custom_id.FlyCampCultivation {
			count2 += 1
		}

		rate := math.Abs(count1-count2) / utils.MaxFloat64(count1, count2)
		return uint32(rate*10000) <= commConf.CampDiff
	}

	return false
}

func (sys *FlyCampSys) handleCrossFlyCampChange(camp uint32, campData map[uint32]uint32) {
	commConf := jsondata.GetFlyCampCommonConf()
	if commConf == nil {
		return
	}

	if !sys.checkCanChangeFlyCamp(camp, campData) {
		sys.LogError("fly camp is count limit camp = %d", camp)
		return
	}

	// 未完成转职,选择职业
	data := sys.GetData()
	if !sys.checkFinish() {
		sys.initFlyCamp(camp)
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncPlayerFlyCamp, &pb3.CommonSt{
			U32Param: camp,
		})
		if err != nil {
			sys.LogError("err: %v", err)
		}
		sys.SendProto3(166, 21, &pb3.S2C_166_21{Camp: camp})
		return
	}

	// 已完成转职,更改职业
	consume := commConf.ChangeCampConsume
	if len(consume) > 0 && !sys.owner.ConsumeByConf(consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogFlyCampChangeConsume,
	}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	oldCamp := data.Camp
	data.Camp = camp
	sys.onFlyCampChangeFinish()
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CChangePlayerFlyCamp, &pb3.CommonSt{
		U32Param:  oldCamp,
		U32Param2: data.Camp,
	})
	if err != nil {
		sys.LogError("err: %v", err)
	}
	sys.SendProto3(166, 21, &pb3.S2C_166_21{Camp: camp})
}

func (sys *FlyCampSys) getStageAttrs(camp, stage uint32, count int) jsondata.AttrVec {
	var attrs jsondata.AttrVec
	sConf := jsondata.GetFlyCampStageConf(camp, stage)
	if count <= 0 {
		return nil
	}
	if sConf == nil || len(sConf.StageSlot) < count {
		return attrs
	}
	return sConf.StageSlot[count-1].Attrs
}

func onFlyCampChangeRet(buf []byte) {
	msg := &pb3.C2GFlyCampChangeRet{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	player := manager.GetPlayerPtrById(msg.GetActorId())
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiFlyCamp)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*FlyCampSys)
	if !ok {
		return
	}
	sys.handleCrossFlyCampChange(msg.Camp, msg.CampData)
}

func onFlyCampCountRet(buf []byte) {
	msg := &pb3.C2GFlyCampCountRet{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	player := manager.GetPlayerPtrById(msg.GetActorId())
	if player == nil {
		return
	}

	player.SendProto3(166, 25, &pb3.S2C_166_25{CampCount: msg.CampCount})
}

func calcFlyCampAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiFlyCamp).(*FlyCampSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	var attrs jsondata.AttrVec
	for _, stageData := range data.StageData {
		tmpAttrs := sys.getStageAttrs(data.Camp, stageData.Stage, int(stageData.Num))
		attrs = append(attrs, tmpAttrs...)
	}
	// 击杀boss, 属性加成
	if data.KillBoss {
		conf := jsondata.GetFlyCampConf(data.Camp)
		if conf != nil {
			attrs = append(attrs, conf.BossAttrs...)
		}
	}
	engine.CheckAddAttrsToCalc(player, calc, attrs)
}

func synFlyCampCount() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		logger.LogWarn("synFlyCampCount small cross srv not exist")
		return
	}
	campCount := flycampmgr.GetCampCount()
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncSrvFlyCamp, &pb3.G2CSrvFlyCampCount{
		CampCount: campCount,
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
	logger.LogInfo("synFlyCampCount Immortals: %d, Cultivation: %d", campCount[custom_id.FlyCampImmortals], campCount[custom_id.FlyCampCultivation])
}

func init() {
	RegisterSysClass(sysdef.SiFlyCamp, func() iface.ISystem {
		return &FlyCampSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.FlyCampFbSettlement, func(player iface.IPlayer, buf []byte) {
		var req pb3.FbSettlement
		if err := pb3.Unmarshal(buf, &req); err != nil {
			player.LogError("err:%v", err)
			return
		}
		obj := player.GetSysObj(sysdef.SiFlyCamp)
		if obj == nil || !obj.IsOpen() {
			return
		}
		sys, ok := obj.(*FlyCampSys)
		if !ok {
			return
		}

		sys.handleSettlement(&req)
	})

	engine.RegisterSysCall(sysfuncid.C2GFlyCampChangeRet, onFlyCampChangeRet)
	engine.RegisterSysCall(sysfuncid.C2GFlyCampCountRet, onFlyCampCountRet)
	engine.RegAttrCalcFn(attrdef.SaFlyCamp, calcFlyCampAttr)

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		synFlyCampCount()
	})

	net.RegisterSysProtoV2(166, 21, sysdef.SiFlyCamp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyCampSys).c2sChangeCamp
	})

	net.RegisterSysProtoV2(166, 22, sysdef.SiFlyCamp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyCampSys).c2sCompleteStage
	})

	net.RegisterSysProtoV2(166, 23, sysdef.SiFlyCamp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyCampSys).c2sAttack
	})

	net.RegisterSysProtoV2(166, 25, sysdef.SiFlyCamp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyCampSys).c2sCampCountReq
	})

	net.RegisterSysProtoV2(166, 26, sysdef.SiFlyCamp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FlyCampSys).c2sFlyCampFinish
	})

	engine.RegQuestTargetProgress(custom_id.QttFinishFlyCamp, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if player.GetFlyCamp() > 0 {
			return 1
		}
		return 0
	})

	gmevent.Register("resetFlyCampCount", func(actor iface.IPlayer, args ...string) bool {
		if engine.FightClientExistPredicate(base.SmallCrossServer) {
			if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CGMResetFlyCampCount, nil); err != nil {
				logger.LogError("err: %v", err)
			}
		}
		data := flycampmgr.GetCampCount()
		data[custom_id.FlyCampImmortals] = 0
		data[custom_id.FlyCampCultivation] = 0
		return true
	}, 1)
	gmevent.Register("setFlyCampV2", func(actor iface.IPlayer, args ...string) bool {
		camp := utils.AtoUint32(args[0])
		sys, ok := actor.GetSysObj(sysdef.SiFlyCamp).(*FlyCampSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		data := sys.GetData()
		oldCamp := data.Camp
		data.Camp = camp
		data.IsFinish = true
		sys.onFlyCampChangeFinish()
		if engine.FightClientExistPredicate(base.SmallCrossServer) {
			if oldCamp == 0 {
				if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncPlayerFlyCamp, &pb3.CommonSt{
					U32Param: camp,
				}); err != nil {
					return false
				}
				return true
			}
			if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CChangePlayerFlyCamp, &pb3.CommonSt{
				U32Param:  oldCamp,
				U32Param2: camp,
			}); err != nil {
				return false
			}
			sys.s2cInfo()
			return true
		} else {
			if oldCamp == 0 {
				flycampmgr.AddCampCount(camp)
			} else {
				flycampmgr.OnChangeCamp(oldCamp, data.Camp)
			}
			sys.s2cInfo()
			return true
		}
	}, 1)
}
