/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 今日累充
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type PYYTodayChargeSys struct {
	*PlayerYYBase
}

func (s *PYYTodayChargeSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TodayChargeDataMap {
		return
	}
	delete(state.TodayChargeDataMap, s.Id)
}

func (s *PYYTodayChargeSys) GetData() *pb3.YYTodayChargeData {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.TodayChargeDataMap {
		state.TodayChargeDataMap = make(map[uint32]*pb3.YYTodayChargeData)
	}
	if state.TodayChargeDataMap[s.Id] == nil {
		state.TodayChargeDataMap[s.Id] = &pb3.YYTodayChargeData{}
	}
	if state.TodayChargeDataMap[s.Id].CurYYDay == 0 {
		state.TodayChargeDataMap[s.Id].CurYYDay = s.GetOpenDay()
	}
	return state.TodayChargeDataMap[s.Id]
}

func (s *PYYTodayChargeSys) S2CInfo() {
	s.SendProto3(148, 20, &pb3.S2C_148_20{
		ActiveId: s.Id,
		State:    s.GetData(),
	})
}

func (s *PYYTodayChargeSys) OnOpen() {
	s.S2CInfo()
}

func (s *PYYTodayChargeSys) Login() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *PYYTodayChargeSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *PYYTodayChargeSys) MergeFix() {
	openZeroTime := time_util.GetZeroTime(s.OpenTime)
	openDayCent := s.GetDailyChargeMoney(openZeroTime)

	if openDayCent > 0 {
		data := s.GetData()
		data.DailyChargeTotal = openDayCent
		s.S2CInfo()
	}
}

func (s *PYYTodayChargeSys) onNewDay() {
	if !s.IsOpen() {
		s.reset()
		return
	}
	// 今天有没有领的奖励
	rewards := s.getUnRecAwards()
	if rewards != nil {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYTodayCharge,
			Rewards: rewards,
		})
	}
	s.reset()
	s.S2CInfo()
}

func (s *PYYTodayChargeSys) reset() {
	data := s.GetData()
	data.RecDailyChargeLayers = nil
	data.DailyChargeTotal = s.GetDailyCharge()
	data.Circle = s.GetPlayer().GetCircle()
	data.CurYYDay = s.GetOpenDay()
}

func (s *PYYTodayChargeSys) OnEnd() {
	rewards := s.getUnRecAwards()
	if rewards != nil {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYTodayCharge,
			Rewards: rewards,
		})
	}
}

func (s *PYYTodayChargeSys) getFullCandAwards(candAwards []*jsondata.YYTodayChargeCandAwards) jsondata.StdRewardVec {
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

func (s *PYYTodayChargeSys) c2sAward(msg *base.Message) error {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open activity")
		return nil
	}
	var req pb3.C2S_148_21
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetYYTodayChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		err := neterror.ConfNotFoundError("%s not found daily charge conf", s.GetPrefix())
		s.GetPlayer().LogWarn("err:%v", err)
		return err
	}

	data := s.GetData()
	var chargeConf *jsondata.YYTodayChargeChargeConf
	for _, c := range conf.ChargeConf {
		if c.Day != data.CurYYDay {
			continue
		}
		chargeConf = c
		break
	}

	if chargeConf == nil {
		err := neterror.ConfNotFoundError("%s not found daily charge conf,curYYDay is %d", s.GetPrefix(), data.CurYYDay)
		s.GetPlayer().LogWarn("err:%v", err)
		return err
	}

	var canRec *jsondata.YYTodayChargeLayerConf
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
	}

	s.SendProto3(148, 21, &pb3.S2C_148_21{
		ActiveId: s.Id,
		Layer:    req.Layer,
	})

	return nil
}

// 获取还没领取的奖励
func (s *PYYTodayChargeSys) getUnRecAwards() jsondata.StdRewardVec {
	conf, ok := jsondata.GetYYTodayChargeConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogError("%s not found DailyCharge conf", s.GetPrefix())
		return nil
	}

	data := s.GetData()
	var chargeConf *jsondata.YYTodayChargeChargeConf
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
	return unRecList
}

func (s *PYYTodayChargeSys) PlayerCharge(*custom_id.ActorEventCharge) {
	data := s.GetData()
	data.DailyChargeTotal = s.GetDailyCharge()
	s.S2CInfo()
}

func rangePYYTodayChargeSys(player iface.IPlayer, doLogic func(sys *PYYTodayChargeSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYTodayCharge)
	if len(yyList) == 0 {
		player.LogWarn("not found yy obj ,class: %d", yydefine.YYTodayCharge)
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*PYYTodayChargeSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		doLogic(sys)
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYTodayCharge, func() iface.IPlayerYY {
		return &PYYTodayChargeSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})

	// 注册跨天
	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		rangePYYTodayChargeSys(player, func(sys *PYYTodayChargeSys) {
			sys.onNewDay()
		})
	})

	net.RegisterYYSysProtoV2(148, 21, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PYYTodayChargeSys).c2sAward
	})
}
