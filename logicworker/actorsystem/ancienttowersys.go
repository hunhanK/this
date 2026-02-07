/**
 * @Author: lzp
 * @Date: 2024/11/7
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/commontimesconter"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type AncientTowerSys struct {
	Base
	data    *pb3.AncientTowerData
	counter *commontimesconter.CommonTimesCounter
}

func (sys *AncientTowerSys) OnInit() {
	if !sys.IsOpen() {
		return
	}
	sys.init()
}

func (sys *AncientTowerSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *AncientTowerSys) OnOpen() {
	sys.init()
	sys.updateLayer()
	sys.S2CInfo()
}

func (sys *AncientTowerSys) OnLogin() {
	sys.updateLayer()
	sys.S2CInfo()
}

func (sys *AncientTowerSys) OnNewDay() {
	sys.data.UsedTimes = 0
	sys.counter.NewDay()
	sys.updateLayer()
	sys.S2CInfo()
}

func (sys *AncientTowerSys) S2CInfo() {
	sys.SendProto3(17, 110, &pb3.S2C_17_110{
		Data: sys.data,
	})
}

func (sys *AncientTowerSys) init() {
	data := sys.GetBinaryData().AncientTowerData
	if data == nil {
		data = &pb3.AncientTowerData{Layer: 0}
		sys.GetBinaryData().AncientTowerData = data
	}

	if data.TimesCounter == nil {
		data.TimesCounter = commontimesconter.NewCommonTimesCounterData()
	}

	sys.data = data
	sys.owner.TriggerEvent(custom_id.AeAncientTowerLayer)

	// 初始化计数器
	sys.counter = commontimesconter.NewCommonTimesCounter(
		sys.data.TimesCounter,
		commontimesconter.WithOnGetFreeTimes(func() uint32 {
			return jsondata.GetAncientTowerCommonConf().DayTimes
		}),
		commontimesconter.WithOnGetOtherAddFreeTimes(func() uint32 {
			return 0
		}),
		commontimesconter.WithOnGetDailyBuyTimesUpLimit(func() uint32 {
			canBuyTimes, _ := sys.GetOwner().GetPrivilege(privilegedef.EnumAncientTowerBuyTimes)
			return uint32(canBuyTimes)
		}),
		commontimesconter.WithOnUpdateCanUseTimes(func(canUseTimes uint32) {
			sys.owner.SetExtraAttr(attrdef.AncientTowerTimes, attrdef.AttrValueAlias(canUseTimes))
		}),
	)
	err := sys.counter.Init()
	if err != nil {
		sys.GetOwner().LogError("err: %v", err)
	}
}

func (sys *AncientTowerSys) updateLayer() {
	layer := sys.data.Layer
	sys.owner.SetExtraAttr(attrdef.AncientTowerLayer, int64(layer))
}

func (sys *AncientTowerSys) onAncientTowerLayerChange() {
	sys.updateLayer()
	sys.owner.TriggerEvent(custom_id.AeAncientTowerLayer)
}

func (sys *AncientTowerSys) c2sCombineTimes(msg *base.Message) error {
	var req pb3.C2S_17_111
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return neterror.ParamsInvalidError("conf not exist")
	}

	teamId := sys.owner.GetTeamId()
	if teamId == 0 {
		return neterror.ParamsInvalidError("team not create")
	}

	aData := sys.getTeamPlayerData()
	if aData == nil {
		return neterror.ParamsInvalidError("team actor data nil")
	}

	if req.CombineTimes > 1 {
		// 小于2次无法合并
		leftTimes := sys.counter.GetLeftTimes()
		if leftTimes < 2 || leftTimes < req.CombineTimes {
			return neterror.ParamsInvalidError("left times limit")
		}

		consumes := jsondata.ConsumeMulti(conf.CombineConsumes, req.CombineTimes-1)
		if !sys.GetOwner().CheckConsumeByConf(consumes, false, 0) {
			return neterror.ParamsInvalidError("consume not enough")
		}
		aData.CombineTimes = req.CombineTimes
	}

	sys.SendProto3(17, 111, &pb3.S2C_17_111{CombineTimes: req.CombineTimes})

	return nil
}

func (sys *AncientTowerSys) c2sBuyRewardTimes(msg *base.Message) error {
	var req pb3.C2S_17_112
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return neterror.InternalError("cont not exit")
	}

	if !sys.counter.CheckCanBuyDailyAddTimes(req.Times) {
		return neterror.ParamsInvalidError("buyLimit")
	}

	var consumes jsondata.ConsumeVec
	dailyBuyTimes := sys.counter.GetDailyBuyTimes()
	for i := uint32(0); i < req.Times; i++ {
		var consume *jsondata.Consume
		consume = conf.BuyConsumes[len(conf.BuyConsumes)-1]
		if dailyBuyTimes < uint32(len(conf.BuyConsumes)) {
			consume = conf.BuyConsumes[dailyBuyTimes]
		}
		consumes = append(consumes, consume)
		dailyBuyTimes++
	}

	if !sys.GetOwner().ConsumeByConf(consumes, true, common.ConsumeParams{LogId: pb3.LogId_LogAncientTowerBuyTimes}) {
		return neterror.ParamsInvalidError("consume not enough")
	}

	sys.counter.AddBuyDailyAddTimes(req.Times)
	sys.SendProto3(17, 112, &pb3.S2C_17_112{BuyTimes: sys.counter.GetDailyBuyTimes()})

	return nil
}

func (sys *AncientTowerSys) c2sFetchLayerRewards(msg *base.Message) error {
	var req pb3.C2S_17_114
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	data := sys.data
	if req.Layer > 0 {
		rewards := jsondata.GetAncientTowerPassRewards(req.Layer)
		if rewards == nil {
			return neterror.ConfNotFoundError("cont not exit, layer: %d", req.Layer)
		}
		if req.Layer > sys.data.Layer {
			return neterror.ParamsInvalidError("layer not finish, layer: %d", req.Layer)
		}
		if utils.SliceContainsUint32(data.Layers, req.Layer) {
			return neterror.ParamsInvalidError("layer rewards has get layer:%d", req.Layer)
		}
	}
	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("not found common conf")
	}
	var rewards jsondata.StdRewardVec
	var layers []uint32
	if req.Layer > 0 {
		data.Layers = append(data.Layers, req.Layer)
		rewards = jsondata.GetAncientTowerPassRewards(req.Layer)
		layers = append(layers, req.Layer)
	} else {
		for _, rConf := range conf.PassRewards {
			if utils.SliceContainsUint32(data.Layers, rConf.Layer) {
				continue
			}
			if data.Layer < rConf.Layer {
				continue
			}
			data.Layers = append(data.Layers, rConf.Layer)
			rewards = append(rewards, rConf.Rewards...)
			layers = append(layers, rConf.Layer)
		}
	}

	if len(rewards) > 0 {
		engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAncientTowerLayerAwards})
	}
	sys.SendProto3(17, 114, &pb3.S2C_17_114{Layers: layers})
	return nil
}

func (sys *AncientTowerSys) UpdateLayer(layer uint32) {
	maxLayer := jsondata.GetAncientTowerMaxLayer()
	layer = utils.MinUInt32(maxLayer, layer)
	sys.data.Layer = layer
	sys.onAncientTowerLayerChange()
	sys.S2CInfo()
}

func (sys *AncientTowerSys) OnSettlement(state, layer, combineTimes uint32) {
	conf := jsondata.GetAncientTowerCommonConf()
	if conf == nil {
		return
	}

	settlePb := &pb3.FbSettlement{
		FbId:   conf.FbId,
		Ret:    state,
		ExData: []uint32{layer},
	}

	// 失败结算
	if state == custom_id.FbSettleResultLose {
		sys.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settlePb})
		return
	}

	// 没有奖励次数
	if sys.counter.GetLeftTimes() == 0 {
		if state == custom_id.FbSettleResultWin {
			if s, ok := sys.owner.GetSysObj(sysdef.SiAssistance).(*AssistanceSys); ok && s.IsOpen() {
				if !s.CompileTeam() {
					sys.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settlePb})
				}
			}
		} else {
			sys.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settlePb})
		}
		return
	}

	curLayer := sys.data.Layer
	if curLayer < layer {
		curLayer = layer
	}

	// 初始按照1层奖励
	curLayer = utils.MaxUInt32(curLayer, 1)
	lConf := jsondata.GetAncientTowerConf(curLayer)
	if lConf == nil {
		return
	}

	var passTimes = uint32(1)
	if combineTimes > 1 {
		consumes := jsondata.ConsumeMulti(conf.CombineConsumes, combineTimes-1)
		if !sys.GetOwner().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogAncientTowerCombineConsume}) {
			sys.GetOwner().LogWarn("consume not enough")
			return
		}
		passTimes = combineTimes
	}

	if !sys.counter.DeductTimes(passTimes) {
		sys.owner.LogError("times not enough")
		return
	}

	// 更新最新层数
	if sys.data.Layer < layer {
		sys.data.Layer = layer
		sys.onAncientTowerLayerChange()
	}

	rewards := lConf.NormalRewards
	if passTimes > 1 {
		rewards = jsondata.StdRewardMulti(lConf.NormalRewards, int64(passTimes))
	}
	if len(rewards) > 0 {
		engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAncientTowerAwards})
	}

	sys.GetOwner().TriggerEvent(custom_id.AePassFb, conf.FbId, passTimes)
	settlePb.ShowAward = jsondata.StdRewardVecToPb3RewardVec(rewards)
	sys.SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: settlePb,
	})
	sys.S2CInfo()
}

func (sys *AncientTowerSys) getTeamPlayerData() *pb3.AncientTowerActorData {
	player := sys.GetOwner()
	teamId := player.GetTeamId()

	fbSet := teammgr.GetTeamFbSetting(teamId)
	if fbSet == nil {
		return nil
	}

	aTowerSet := fbSet.AncientTowerTData
	if aTowerSet == nil || aTowerSet.ActorsData == nil {
		return nil
	}

	return aTowerSet.ActorsData[player.GetId()]
}

// 结算
func onAncientTowerSettlement(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if err := pb3.Unmarshal(buf, &st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiAncientTower).(*AncientTowerSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys obj failed: %d", sysdef.SiAncientTower)
		return
	}

	state := st.U32Param
	layer := st.U32Param2

	aData := sys.getTeamPlayerData()

	sys.OnSettlement(state, layer, aData.CombineTimes)
}

func onAncientTowerLayer(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if err := pb3.Unmarshal(buf, &st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiAncientTower).(*AncientTowerSys)
	if !ok || !sys.IsOpen() {
		player.LogError("sys obj failed: %d", sysdef.SiAncientTower)
		return
	}

	layer := st.U32Param
	sys.UpdateLayer(layer)
}

func init() {
	RegisterSysClass(sysdef.SiAncientTower, func() iface.ISystem {
		return &AncientTowerSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.AncientTowerSettlement, onAncientTowerSettlement)
	engine.RegisterActorCallFunc(playerfuncid.AncientTowerLayer, onAncientTowerLayer)

	net.RegisterSysProto(17, 111, sysdef.SiAncientTower, (*AncientTowerSys).c2sCombineTimes)
	net.RegisterSysProto(17, 112, sysdef.SiAncientTower, (*AncientTowerSys).c2sBuyRewardTimes)
	net.RegisterSysProto(17, 114, sysdef.SiAncientTower, (*AncientTowerSys).c2sFetchLayerRewards)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiAncientTower).(*AncientTowerSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.OnNewDay()
	})

	gmevent.Register("setAncientTower", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		layer := utils.AtoUint32(args[0])
		sys, ok := player.GetSysObj(sysdef.SiAncientTower).(*AncientTowerSys)
		if ok && sys.IsOpen() {
			maxLayer := jsondata.GetAncientTowerMaxLayer()
			sys.data.Layer = utils.MinUInt32(layer, maxLayer)
			sys.onAncientTowerLayerChange()
			sys.S2CInfo()
		}
		return true
	}, 1)
}
