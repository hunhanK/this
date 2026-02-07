/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 天天返利
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type DailyChargeSys struct {
	*PlayerYYBase
}

func (s *DailyChargeSys) GetData() *pb3.YYDailyChargeData {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.DailyChargeDataMap {
		state.DailyChargeDataMap = make(map[uint32]*pb3.YYDailyChargeData)
	}
	if state.DailyChargeDataMap[s.Id] == nil {
		state.DailyChargeDataMap[s.Id] = &pb3.YYDailyChargeData{}
	}
	if state.DailyChargeDataMap[s.Id].CurYYDay == 0 {
		state.DailyChargeDataMap[s.Id].CurYYDay = s.GetOpenDay()
	}
	return state.DailyChargeDataMap[s.Id]
}

func (s *DailyChargeSys) S2CInfo() {
	s.SendProto3(148, 1, &pb3.S2C_148_1{
		ActiveId: s.Id,
		State:    s.GetData(),
	})
}

func (s *DailyChargeSys) OnOpen() {
	s.S2CInfo()
}

func (s *DailyChargeSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *DailyChargeSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *DailyChargeSys) onNewDay() {
	if !s.IsOpen() {
		s.resetNewDay()
		return
	}
	// 今天有没有领的奖励
	rewards := s.getUnRecAwards()
	if rewards != nil {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYDailyCharge,
			Rewards: rewards,
		})
	}
	s.resetNewDay()
	s.S2CInfo()
}

func (s *DailyChargeSys) resetNewDay() {
	data := s.GetData()
	data.RecDailyChargeLayers = nil
	data.RecDailyConsumeLayers = nil
	data.DailyChargeTotal = s.GetDailyCharge()
	data.DailyUseDiamondTotal = 0
	data.Circle = s.GetPlayer().GetCircle()
	data.CurYYDay = s.GetOpenDay()
}

func (s *DailyChargeSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.DailyChargeDataMap {
		return
	}
	delete(state.DailyChargeDataMap, s.Id)
}

func (s *DailyChargeSys) OnEnd() {
	rewards := s.getUnRecAwards()
	if rewards != nil {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYDailyCharge,
			Rewards: rewards,
		})
	}
}

func (s *DailyChargeSys) getFullCandAwards(candAwards []*jsondata.YYDailyChargeCandAwards) jsondata.StdRewardVec {
	var reAwards jsondata.StdRewardVec
	circle := s.GetData().Circle
	for _, candAward := range candAwards {
		if circle > candAward.Circle {
			continue
		}
		reAwards = candAward.Awards
		break
	}
	if len(reAwards) == 0 && len(candAwards) != 0 {
		s.GetPlayer().LogWarn("cand awards not found, circle is %d", circle)
		reAwards = candAwards[len(candAwards)-1].Awards
	}
	return reAwards
}
func (s *DailyChargeSys) c2sAward(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}
	var req pb3.C2S_148_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetYYDailyChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found daily charge conf", s.GetPrefix())
	}

	data := s.GetData()
	var chargeConf *jsondata.YYDailyChargeChargeConf
	for _, c := range conf.ChargeConf {
		if c.Day != data.CurYYDay {
			continue
		}
		chargeConf = c
		break
	}
	if chargeConf == nil {
		return neterror.ConfNotFoundError("%s not found daily charge conf, curYYDay:%d", s.GetPrefix(), data.CurYYDay)
	}

	var canRec *jsondata.YYDailyChargeLayerConf
	for _, layerConf := range chargeConf.LayerConf {
		layer := layerConf.Layer
		if req.Layer != layer {
			continue
		}

		if pie.Uint32s(data.RecDailyChargeLayers).Contains(layer) {
			s.GetPlayer().LogInfo("rec layer is %d", layer)
			continue
		}

		canRec = layerConf
		break
	}

	if canRec != nil {
		data.RecDailyChargeLayers = append(data.RecDailyChargeLayers, canRec.Layer)
		var giveAwards jsondata.StdRewardVec
		giveAwards = append(giveAwards, canRec.Awards...)
		giveAwards = append(giveAwards, s.getFullCandAwards(canRec.CandAwards)...)
		if len(giveAwards) != 0 {
			engine.GiveRewards(s.GetPlayer(), giveAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyCharge})
		}
		player := s.GetPlayer()
		engine.BroadcastTipMsgById(tipmsgid.DailyChargeTip, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(player, giveAwards))
	}

	s.SendProto3(148, 3, &pb3.S2C_148_3{
		ActiveId: s.Id,
		Layer:    req.Layer,
	})

	return nil
}

func (s *DailyChargeSys) c2sConsumeAward(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}
	var req pb3.C2S_148_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetYYDailyChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found daily charge conf", s.GetPrefix())
	}

	data := s.GetData()
	var chargeConf *jsondata.YYDailyChargeChargeConf
	for _, c := range conf.ChargeConf {
		if c.Day != data.CurYYDay {
			continue
		}
		chargeConf = c
		break
	}
	if chargeConf == nil {
		return neterror.ConfNotFoundError("%s not found daily charge conf,curYYDay:%d", s.GetPrefix(), data.CurYYDay)
	}

	var canRec *jsondata.YYDailyChargeLayerConf
	for _, consumeLayerConf := range chargeConf.LayerConf {
		layer := consumeLayerConf.Layer
		if req.Layer != layer {
			continue
		}

		if pie.Uint32s(data.RecDailyConsumeLayers).Contains(layer) {
			s.GetPlayer().LogInfo("rec layer is %d", layer)
			continue
		}

		canRec = consumeLayerConf
		break
	}

	if canRec == nil {
		return neterror.ConfNotFoundError("not found daily charge conf")
	}

	if canRec.Diamond > data.DailyUseDiamondTotal {
		return neterror.ParamsInvalidError("not enough diamond")
	}

	data.RecDailyConsumeLayers = append(data.RecDailyConsumeLayers, canRec.Layer)
	var giveAwards jsondata.StdRewardVec
	giveAwards = append(giveAwards, canRec.ConsumeAwards...)
	giveAwards = append(giveAwards, s.getFullCandAwards(canRec.ConsumeCandAwards)...)
	if len(giveAwards) != 0 {
		engine.GiveRewards(s.GetPlayer(), giveAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyCharge})
	}

	s.SendProto3(148, 4, &pb3.S2C_148_4{
		ActiveId: s.Id,
		Layer:    req.Layer,
	})

	return nil
}

// 获取还没领取的奖励
func (s *DailyChargeSys) getUnRecAwards() jsondata.StdRewardVec {
	conf, ok := jsondata.GetYYDailyChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogError("%s not found DailyCharge conf", s.GetPrefix())
		return nil
	}

	data := s.GetData()
	var chargeConf *jsondata.YYDailyChargeChargeConf
	for _, c := range conf.ChargeConf {
		if c.Day != data.CurYYDay {
			continue
		}
		chargeConf = c
		break
	}

	if chargeConf == nil {
		s.GetPlayer().LogWarn("not found charge , day is %d", data.CurYYDay)
		return nil
	}

	var unRecList jsondata.StdRewardVec
	for _, layerConf := range chargeConf.LayerConf {
		if data.DailyChargeTotal < layerConf.Amount {
			continue
		}
		if pie.Uint32s(data.RecDailyChargeLayers).Contains(layerConf.Layer) {
			continue
		}
		unRecList = append(unRecList, layerConf.Awards...)
		unRecList = append(unRecList, s.getFullCandAwards(layerConf.CandAwards)...)
	}

	for _, layerConf := range chargeConf.LayerConf {
		if data.DailyUseDiamondTotal < layerConf.Diamond {
			continue
		}
		if pie.Uint32s(data.RecDailyConsumeLayers).Contains(layerConf.Layer) {
			continue
		}
		unRecList = append(unRecList, layerConf.ConsumeAwards...)
		unRecList = append(unRecList, s.getFullCandAwards(layerConf.ConsumeCandAwards)...)
	}
	return unRecList
}

func (s *DailyChargeSys) PlayerCharge(*custom_id.ActorEventCharge) {
	data := s.GetData()
	data.DailyChargeTotal = s.GetDailyCharge()
	s.S2CInfo()
}

// PlayerUseDiamond 玩家消耗钻石
func (s *DailyChargeSys) playerUseDiamond(count int64) {
	data := s.GetData()
	data.DailyUseDiamondTotal += uint32(count)
	s.S2CInfo()
}

func rangeDailyChargeSys(player iface.IPlayer, doLogic func(yy iface.IPlayerYY)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYDailyCharge)
	if len(yyList) == 0 {
		player.LogWarn("not found yy obj, id is %d", yydefine.YYDailyCharge)
		return
	}
	for i := range yyList {
		v := yyList[i]
		doLogic(v)
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDailyCharge, func() iface.IPlayerYY {
		return &DailyChargeSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	// 注册跨天
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		rangeDailyChargeSys(player, func(yy iface.IPlayerYY) {
			sys, ok := yy.(*DailyChargeSys)
			if !ok || !sys.IsOpen() {
				return
			}
			sys.onNewDay()
		})
	})

	event.RegActorEvent(custom_id.AeConsumeMoney, func(player iface.IPlayer, args ...interface{}) {
		rangeDailyChargeSys(player, func(yy iface.IPlayerYY) {
			sys, ok := yy.(*DailyChargeSys)
			if !ok || !sys.IsOpen() {
				return
			}
			if len(args) < 2 {
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
			if mt != moneydef.Diamonds {
				return
			}
			sys.playerUseDiamond(count)
		})
	})

	net.RegisterYYSysProtoV2(148, 1, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*DailyChargeSys).c2sAward
	})

	net.RegisterYYSysProtoV2(148, 2, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*DailyChargeSys).c2sConsumeAward
	})
}
