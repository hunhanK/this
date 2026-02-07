/**
 * @Author: lzp
 * @Date: 2024/5/23
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/retrievalmgr"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	pyy "jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/beasttidemgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

const (
	OneStar = iota + 1
	TwoStar
	ThreeStar
)

type EquipFbSys struct {
	Base
	data *pb3.EquipFbPlayerData
}

func (sys *EquipFbSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *EquipFbSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *EquipFbSys) OnOpen() {
	sys.init()
	sys.S2CInfo()
}

func (sys *EquipFbSys) OnLogin() {
	sys.updateLeftTimes()
	sys.S2CInfo()
}

func (sys *EquipFbSys) S2CInfo() {
	sys.SendProto3(17, 30, &pb3.S2C_17_30{
		Data: sys.data,
	})
}

func (sys *EquipFbSys) init() {
	data := sys.GetBinaryData().EquipFbData
	if data == nil {
		data = &pb3.EquipFbPlayerData{}
		sys.GetBinaryData().EquipFbData = data
	}

	sys.data = data
	sys.updateLeftTimes()
}

func (sys *EquipFbSys) c2sBuyRewardTimes(msg *base.Message) error {
	var req pb3.C2S_17_35
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return neterror.InternalError("cont not exit")
	}

	canBuyTimes, err := sys.GetOwner().GetPrivilege(privilegedef.EnumEquipFbBuyTimes)
	if err != nil {
		return neterror.InternalError("get privilege failed %s", err)
	}

	if sys.data.BuyTimes+req.Times > uint32(canBuyTimes) {
		return neterror.ParamsInvalidError("buy times limit")
	}

	var consumes jsondata.ConsumeVec
	for buyTimes := sys.data.BuyTimes; buyTimes < sys.data.BuyTimes+req.Times; buyTimes++ {
		var consume *jsondata.Consume
		if buyTimes >= uint32(len(conf.BuyConsumes)) {
			consume = conf.BuyConsumes[len(conf.BuyConsumes)-1]
		} else {
			consume = conf.BuyConsumes[buyTimes]
		}
		consumes = append(consumes, consume)
	}

	if !sys.GetOwner().ConsumeByConf(consumes, true, common.ConsumeParams{LogId: pb3.LogId_LogEquipFbBuyTimes}) {
		return neterror.ParamsInvalidError("consume not enough")
	}

	sys.data.BuyTimes += req.Times

	sys.updateLeftTimes()
	sys.SendProto3(17, 35, &pb3.S2C_17_35{BuyTimes: sys.data.BuyTimes})

	return nil
}

func (sys *EquipFbSys) c2sSelDiff(msg *base.Message) error {
	var req pb3.C2S_17_32
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetEquipFbConf(req.Diff)
	if conf == nil {
		return neterror.ParamsInvalidError("conf not exist")
	}

	player := sys.GetOwner()
	if player.GetCircle() < conf.BoundaryLv {
		return neterror.ParamsInvalidError("boundary level limit")
	}

	if player.GetLevel() < conf.Lv {
		return neterror.ParamsInvalidError("level limit")
	}

	if player.GetTeamId() == 0 {
		return neterror.ParamsInvalidError("team not exit")
	}

	aData := sys.getTeamActorData()
	aData.Diff = req.Diff

	sys.SendProto3(17, 32, &pb3.S2C_17_32{Diff: req.Diff})
	return nil
}

func (sys *EquipFbSys) c2sCombineTimes(msg *base.Message) error {
	var req pb3.C2S_17_33
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return neterror.ParamsInvalidError("conf not exist")
	}

	player := sys.GetOwner()
	if player.GetTeamId() == 0 {
		return neterror.ParamsInvalidError("team not exit")
	}

	if player.GetLevel() < conf.CombineLv {
		return neterror.ParamsInvalidError("level not satisfy")
	}

	if req.CombineTimes > 1 {
		// 小于2次无法合并
		if sys.getLeftTimes() < 2 || sys.getLeftTimes() < req.CombineTimes {
			return neterror.ParamsInvalidError("left times limit")
		}

		consumes := jsondata.ConsumeMulti(conf.CombineConsumes, req.CombineTimes-1)
		if !sys.GetOwner().CheckConsumeByConf(consumes, false, 0) {
			return neterror.ParamsInvalidError("consume not enough")
		}
	}

	aData := sys.getTeamActorData()
	aData.CombineTimes = req.CombineTimes

	sys.SendProto3(17, 33, &pb3.S2C_17_33{CombineTimes: req.CombineTimes})

	return nil
}

func (sys *EquipFbSys) OnNewDay() {
	sys.data.UsedTimes = 0
	sys.data.BuyTimes = 0
	sys.S2CInfo()
	sys.updateLeftTimes()
}

func (sys *EquipFbSys) getTeamActorData() *pb3.EquipFbTeamActorData {
	player := sys.GetOwner()
	teamId := player.GetTeamId()

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet.EquipFbTData == nil {
		fbSet.EquipFbTData = &pb3.EquipFbTeamData{}
	}
	eData := fbSet.EquipFbTData
	if eData.EFbActorData == nil {
		eData.EFbActorData = make(map[uint64]*pb3.EquipFbTeamActorData)
	}

	playerId := player.GetId()
	aData, ok := eData.EFbActorData[playerId]
	if !ok {
		aData = &pb3.EquipFbTeamActorData{ActorId: playerId}
		eData.EFbActorData[playerId] = aData
	}
	return aData
}

func (sys *EquipFbSys) getLeftTimes() uint32 {
	if sys.data == nil {
		return 0
	}

	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return 0
	}

	data := sys.data

	addTimes := sys.GetOwner().GetFightAttr(attrdef.EquipFuBenTimesAdd)

	leftTimes := conf.DayTimes + uint32(addTimes) + data.BuyTimes - data.UsedTimes
	return uint32(utils.MaxInt32(0, int32(leftTimes)))
}

func (sys *EquipFbSys) updateLeftTimes() {
	leftTimes := sys.getLeftTimes()
	sys.owner.SetExtraAttr(attrdef.EquipFbTimes, int64(leftTimes))
}

func (sys *EquipFbSys) OnSettlement(msg *pb3.F2GEquipFbSettlement) {
	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return
	}

	teamId := msg.TeamId
	_, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	aData := sys.getTeamActorData()
	combineTimes := aData.CombineTimes
	diff := aData.Diff

	sys.GetOwner().TriggerQuestEvent(custom_id.QttJoinToEquipFb, 0, int64(combineTimes))

	// 没有奖励次数,使用协助奖励
	if sys.getLeftTimes() <= 0 {
		if msg.IsSuccess {
			if s, ok := sys.owner.GetSysObj(sysdef.SiAssistance).(*AssistanceSys); ok && s.IsOpen() {
				if !s.CompileTeam() {
					sys.SendProto3(17, 34, &pb3.S2C_17_34{IsSuccess: true})
				}
			}
		} else {
			sys.SendProto3(17, 34, &pb3.S2C_17_34{IsSuccess: false})
		}
		return
	}

	// 失败结算
	if !msg.IsSuccess {
		sys.SendProto3(17, 34, &pb3.S2C_17_34{IsSuccess: false})
		sys.S2CInfo()
		return
	}

	// 成功结算扣除次数
	var passTimes = uint32(1)
	if combineTimes > 1 {
		// 消耗合成道具
		consumes := jsondata.ConsumeMulti(conf.CombineConsumes, combineTimes-1)
		if !sys.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipFbCombineConsume}) {
			sys.GetOwner().LogWarn("consume not enough")
			return
		}
		passTimes = combineTimes
	}

	sys.data.UsedTimes += passTimes
	sys.updateLeftTimes()
	sys.GetOwner().TriggerEvent(custom_id.AePassFb, conf.FbId, passTimes)
	sys.GetOwner().TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionEquipFuBenConsumeTimes, int(passTimes))
	sys.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiEquipFuBen, int(passTimes))

	var rewards jsondata.StdRewardVec
	var sumExp int64
	if combineTimes > 1 {
		for i := 0; i < int(combineTimes); i++ {
			addRewards, addExp := sys.GetSettleRewards(diff, msg.Star)
			rewards = append(rewards, addRewards...)
			sumExp += addExp
		}
	} else {
		rewards, sumExp = sys.GetSettleRewards(diff, msg.Star)
	}

	// 发奖励
	rewards = engine.FilterRewardByPlayer(sys.owner, rewards)
	if len(rewards) > 0 {
		engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogEquipFbAwards})
	}
	// 加经验
	if sumExp > 0 {
		sys.owner.AddExp(sumExp, pb3.LogId_LogEquipFbExp, false, 0)
	}

	sys.SendProto3(17, 34, &pb3.S2C_17_34{
		IsSuccess: true,
		Exp:       uint64(sumExp),
		Star:      msg.Star,
		Awards:    jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	sys.S2CInfo()
}

func (sys *EquipFbSys) GetSettleRewards(diff, star uint32) (rewards jsondata.StdRewardVec, exp int64) {
	conf := jsondata.GetEquipFbConf(diff)
	if conf == nil {
		return
	}

	comConf := jsondata.GetEquipFbCommonConf()
	if comConf == nil {
		return
	}

	levelSys, ok := sys.owner.GetSysObj(sysdef.SiLevel).(*LevelSys)
	if !ok {
		sys.owner.LogError("get level sys failed")
		return
	}

	getAwards := func(aGConfL []*jsondata.AwardGroup) (rewards jsondata.StdRewardVec) {
		for _, aGConf := range aGConfL {
			pool := new(random.Pool)
			for _, aConf := range aGConf.Awards {
				if aConf.Job > 0 && aConf.Job != sys.owner.GetJob() {
					continue
				}
				pool.AddItem(aConf, aConf.Weight)
			}
			for i := 1; i <= int(aGConf.Count); i++ {
				reward, ok := pool.RandomOne().(*jsondata.StdReward)
				if !ok {
					continue
				}
				rewards = append(rewards, reward)
			}
		}
		return
	}

	var rate uint32
	switch star {
	case OneStar:
		rate = comConf.OneExpRate
		rewards = getAwards(conf.AwardGroup1)
	case TwoStar:
		rate = comConf.TwoExpRate
		rewards = getAwards(conf.AwardGroup2)
	case ThreeStar:
		rate = comConf.ThreeExpRate
		rewards = getAwards(conf.AwardGroup3)
	}

	// 经验加成
	rate2, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumExpFubenYield)
	rate3 := beasttidemgr.GetBeastTideAddition(pyy.BTEffectTypeDoubleExperience)
	allRate := rate + uint32(rate2) + uint32(rate3)

	// 加经验
	sumExp, _ := levelSys.CalcFinalExp(int64(conf.Exp), true, allRate)
	exp = sumExp
	return
}

// 结算
func onSettlement(player iface.IPlayer, buf []byte) {
	var st pb3.F2GEquipFbSettlement
	if err := pb3.Unmarshal(buf, &st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys obj failed: %d", sysdef.SiEquipFuBen)
		return
	}

	sys.OnSettlement(&st)
}

// 队伍成员发生变化
func onTeamMemberChange(buf []byte) {
	var req pb3.SyncTeamInfo
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("unmarshal failed err: %s", err)
		return
	}

	syncTeamLeftTimesToFightSrv(req.Actors)
}

func syncTeamLeftTimesToFightSrv(actors []uint64) {
	for _, actorId := range actors {
		player := manager.GetPlayerPtrById(actorId)
		if player == nil {
			continue
		}
		sys, ok := player.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
		if !ok || !sys.IsOpen() {
			continue
		}

		sys.updateLeftTimes()
	}
}

func onRetrievalRewards(player iface.IPlayer, count, consumeId uint32) jsondata.StdRewardVec {
	sys, ok := player.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
	if !ok || !sys.IsOpen() {
		return nil
	}

	conf := jsondata.GetEquipFbCommonConf()
	if conf == nil {
		return nil
	}

	diff := getMaxDiff(player)
	star := uint32(1)
	if consumeId == 2 {
		star = 2
	}

	var rewards jsondata.StdRewardVec
	var sumExp int64

	for i := 0; i < int(count); i++ {
		addRewards, addExp := sys.GetSettleRewards(diff, star)
		rewards = append(rewards, addRewards...)
		sumExp += addExp
	}

	rewards = append(rewards, &jsondata.StdReward{
		Id:    conf.ExpItemId,
		Count: sumExp,
	})
	return rewards
}

func getMaxDiff(player iface.IPlayer) (diff uint32) {
	Lv := player.GetLevel()
	var maxDiff uint32 = 0
	for _, conf := range jsondata.EquipFbConfMgr {
		if Lv >= conf.Lv && conf.Diff > maxDiff {
			maxDiff = conf.Diff
		}
	}
	diff = maxDiff
	return
}

func init() {
	RegisterSysClass(sysdef.SiEquipFuBen, func() iface.ISystem {
		return &EquipFbSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.EquipFbSettlement, onSettlement)

	engine.RegisterSysCall(sysfuncid.F2GTeamMemberChangeToEquipFb, onTeamMemberChange)

	net.RegisterSysProto(17, 32, sysdef.SiEquipFuBen, (*EquipFbSys).c2sSelDiff)
	net.RegisterSysProto(17, 33, sysdef.SiEquipFuBen, (*EquipFbSys).c2sCombineTimes)
	net.RegisterSysProto(17, 35, sysdef.SiEquipFuBen, (*EquipFbSys).c2sBuyRewardTimes)

	retrievalmgr.Reg(retrievalmgr.RetrievalByEqualFuBen, onRetrievalRewards)

	event.RegActorEvent(custom_id.AeRegFightAttrChange, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		attrType, ok := args[0].(uint32)
		if !ok {
			return
		}
		if attrType != attrdef.EquipFuBenTimesAdd {
			return
		}
		sys, ok := actor.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.updateLeftTimes()
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.OnNewDay()
	})

	gmevent.Register("resetDailyTimes", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiEquipFuBen).(*EquipFbSys)
		if !ok || !sys.IsOpen() {
			return false
		}

		sys.data.UsedTimes = 0
		sys.S2CInfo()
		sys.updateLeftTimes()
		return true
	}, 1)
}
