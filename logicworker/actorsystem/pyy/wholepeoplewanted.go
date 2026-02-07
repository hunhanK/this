/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 全民通缉
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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

type WholePeopleWanted struct {
	PlayerYYBase
}

func (s *WholePeopleWanted) OnOpen() {
	s.S2CInfo()
}

func (s *WholePeopleWanted) OnReconnect() {
	s.S2CInfo()
}

func (s *WholePeopleWanted) Login() {
	s.S2CInfo()
}

func (s *WholePeopleWanted) S2CInfo() {
	s.SendProto3(142, 1, &pb3.S2C_142_1{
		ActiveId: s.Id,
		State:    s.State(),
	})
}

func (s *WholePeopleWanted) ResetData() {
	yyData := s.GetYYData()
	if yyData.WholePeopleWanted == nil {
		return
	}
	delete(yyData.WholePeopleWanted, s.Id)
}

func (s *WholePeopleWanted) State() *pb3.YYWholePeopleWanted {
	yyData := s.GetYYData()
	if yyData.WholePeopleWanted == nil {
		yyData.WholePeopleWanted = make(map[uint32]*pb3.YYWholePeopleWanted)
	}
	if yyData.WholePeopleWanted[s.Id] == nil {
		yyData.WholePeopleWanted[s.Id] = &pb3.YYWholePeopleWanted{}
	}
	return yyData.WholePeopleWanted[s.Id]
}

// 收到领取奖励
func (s *WholePeopleWanted) c2sReceiveReward(msg *base.Message) error {
	var req pb3.C2S_142_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	playerProcess := s.State()
	conf, ok := jsondata.GetYYWholePeopleWantedConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s yy whole people want conf not found", s.GetPrefix())
	}

	var layerConfList = conf.NormalLayers
	if req.IsExcellent {
		layerConfList = conf.ExcellentLayers
	}

	// 校验 层次
	if len(playerProcess.StdRewardIds) >= len(conf.NormalLayers)+len(conf.ExcellentLayers) {
		s.GetPlayer().LogWarn("%s complete start svr wanted, not allow already receive , pid is %d", s.GetPrefix(), s.GetPlayer().GetId())
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	var nextLayer *jsondata.YYWholePeopleWantedConfLayer
	for i := range layerConfList {
		if req.Layer == layerConfList[i].Layers {
			nextLayer = layerConfList[i]
			break
		}
	}
	if nextLayer == nil {
		return neterror.ConfNotFoundError("%s yy whole people want next layer conf not found, layer: %d", s.GetPrefix(), req.Layer)
	}

	if !s.checkReceivedCond(nextLayer, playerProcess) {
		s.GetPlayer().LogWarn("the eligibility is not met")
		s.GetPlayer().SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	// 回写领取层次
	if req.IsExcellent {
		playerProcess.CurExcellentLayer = nextLayer.Layers
	} else {
		playerProcess.CurNormalLayer = nextLayer.Layers
	}

	// boss击杀数
	playerProcess.StdRewardIds = pie.Uint32s(playerProcess.StdRewardIds).Append(nextLayer.TargetKillCount).Unique()

	// 下发奖励
	engine.GiveRewards(s.GetPlayer(), nextLayer.Awards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogYYWholePeopleWanted,
	})

	// 领取成功
	s.SendProto3(142, 2, &pb3.S2C_142_2{
		ActiveId:    s.Id,
		Layer:       req.Layer,
		IsExcellent: req.IsExcellent,
	})

	s.S2CInfo()

	return nil
}

func (s *WholePeopleWanted) checkReceivedCond(layer *jsondata.YYWholePeopleWantedConfLayer, playerProcess *pb3.YYWholePeopleWanted) bool {
	// 校验领取奖励
	if pie.Uint32s(playerProcess.StdRewardIds).Contains(layer.TargetKillCount) {
		s.GetPlayer().LogWarn("have already claimed the award , pid is %d , target kill count is %d", s.GetPlayer().GetId(), layer.TargetKillCount)
		return false
	}

	// 校验击杀条件
	if layer.TargetKillCount > uint32(len(playerProcess.KillBossIds)) {
		s.GetPlayer().LogWarn("targetKillCount is %d, player kill count is %d", layer.TargetKillCount, len(playerProcess.KillBossIds))
		return false
	}
	return true
}

func (s *WholePeopleWanted) addPlayerAeKillMonProcess(monsterId uint32) {
	playerProcess := s.State()
	conf, ok := jsondata.GetYYWholePeopleWantedConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	if !pie.Uint32s(conf.Bosses).Contains(monsterId) {
		return
	}

	if pie.Uint32s(playerProcess.KillBossIds).Contains(monsterId) {
		return
	}

	// 击杀队列
	playerProcess.KillBossIds = pie.Uint32s(playerProcess.KillBossIds).Append(monsterId).Unique()

	s.S2CInfo()
}

func (s *WholePeopleWanted) OnEnd() {
	conf, ok := jsondata.GetYYWholePeopleWantedConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.GetPlayer().LogWarn("get yy whole people wanted failed")
		return
	}

	// 玩家进度
	playerProcess := s.State()

	var rewards []*jsondata.StdReward
	for _, layer := range conf.NormalLayers {
		if !s.checkReceivedCond(layer, playerProcess) {
			s.GetPlayer().LogWarn("the eligibility is not met")
			continue
		}
		playerProcess.StdRewardIds = append(playerProcess.StdRewardIds, layer.TargetKillCount)
		rewards = append(rewards, layer.Awards...)
	}

	for _, layer := range conf.ExcellentLayers {
		if !s.checkReceivedCond(layer, playerProcess) {
			s.GetPlayer().LogWarn("the eligibility is not met")
			continue
		}
		playerProcess.StdRewardIds = append(playerProcess.StdRewardIds, layer.TargetKillCount)
		rewards = append(rewards, layer.Awards...)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YYWholepeoplewanted,
			Rewards: rewards,
		})
	}
}

func c2sStartSvrWantedReceiveReward(sys iface.IPlayerYY) func(*base.Message) error {
	return sys.(*WholePeopleWanted).c2sReceiveReward
}

// 怪物被击杀 通知全民通缉
func onAeKillMonCallWholePeopleWanted(player iface.IPlayer, args ...interface{}) {
	if len(args) < 1 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}

	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYWholePeopleWanted)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*WholePeopleWanted)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.addPlayerAeKillMonProcess(monsterId)
		break
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYWholePeopleWanted, func() iface.IPlayerYY {
		return &WholePeopleWanted{}
	})
	net.RegisterYYSysProtoV2(142, 1, c2sStartSvrWantedReceiveReward)
	event.RegActorEvent(custom_id.AeKillMon, onAeKillMonCallWholePeopleWanted)
}
