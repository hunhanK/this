/**
 * @Author: LvYuMeng
 * @Date: 2024/12/11
 * @Desc: 剑宗主宰
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/swordguild"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"math"
)

type SwordGuildSys struct {
	Base
}

func (s *SwordGuildSys) OnAfterLogin() {
	s.callCross()
}

func (s *SwordGuildSys) OnReconnect() {
	s.callCross()
}

func (s *SwordGuildSys) OnOpen() {
	s.callCross()
}

func (s *SwordGuildSys) callCross() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncSwordGuildCrossActorInfoReq, &pb3.CommonSt{
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
		U64Param:  s.owner.GetId(),
	})
	if nil != err {
		s.LogError("err:%v", err)
	}
}

func (s *SwordGuildSys) joinSwordGuildShout(msg *base.Message) error {
	player := s.owner
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FJoinSwordGuildShout, &pb3.JoinSwordGuildShoutReq{
		FightValue: player.GetExtraAttr(attrdef.FightValue),
	})
	if err != nil {
		player.LogTrace("CallActorSmallCrossFunc %d failed", actorfuncid.G2FJoinSwordGuildShout)
		return err
	}
	return nil
}

func (s *SwordGuildSys) revPlayerAwards(req *pb3.SwordGuildPlayerAwardsReq) error {
	player := s.owner
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FSwordGuildPlayerAwards, req)
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) c2sEnterStronghold(msg *base.Message) error {
	var req pb3.C2S_54_115
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	player := s.owner

	err = player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterSwordGuildBattleground, &pb3.SwordGuildBattlegroundEnterReq{
		BattlegroundId: req.GetBattlegroundId(),
		SceneId:        req.GetSceneId(),
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *SwordGuildSys) c2sWorship(msg *base.Message) error {
	err := s.revPlayerAwards(&pb3.SwordGuildPlayerAwardsReq{
		Op: swordguild.PlayerAwardTypeWorship,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) c2sRevPersonalScoreAwards(msg *base.Message) error {
	err := s.revPlayerAwards(&pb3.SwordGuildPlayerAwardsReq{
		Op: swordguild.PlayerAwardTypePersonalScore,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) c2sRevSumChargeAwards(msg *base.Message) error {
	var req pb3.C2S_54_112
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	err = s.revPlayerAwards(&pb3.SwordGuildPlayerAwardsReq{
		Op: swordguild.PlayerAwardTypeSumCharge,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) c2sRevStrongholdAwards(msg *base.Message) error {
	var req pb3.C2S_54_113
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	err = s.revPlayerAwards(&pb3.SwordGuildPlayerAwardsReq{
		Op: swordguild.PlayerAwardTypeStronghold,
		Id: req.SceneId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) c2sRevEnergy(msg *base.Message) error {
	err := s.revPlayerAwards(&pb3.SwordGuildPlayerAwardsReq{
		Op: swordguild.PlayerAwardTypeEnergy,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *SwordGuildSys) onCharge(chargeCent uint32) {
	if !manager.InSwordGuildCompetitionDay() {
		return
	}
	err := s.owner.CallActorSmallCrossFunc(actorfuncid.G2FSyncChangeSwordGuild, &pb3.CommonSt{
		U32Param: chargeCent,
	})
	if err != nil {
		s.owner.LogError("G2FSyncChangeSwordGuild %d failed!", chargeCent)
		return
	}
}

func handleSwordGuildCharge(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiSwordGuild).(*SwordGuildSys)
	if !ok || !sys.IsOpen() {
		return
	}
	if len(args) < 1 {
		return
	}
	chargeEvent, ok := args[0].(*custom_id.ActorEventCharge)
	if !ok {
		return
	}
	sys.onCharge(chargeEvent.CashCent)
}

func useItemAddSwordGuildEnergy(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if !manager.InSwordGuildCompetitionDay() {
		return
	}

	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}

	if param.Count == 0 {
		return false, false, 0
	}

	addNum := conf.Param[0]
	cur := player.GetExtraAttrU32(attrdef.SwordGuildEnergy)
	limitNum := jsondata.GetSwordGuildCompetitionConf().MaxEnergy
	if cur+addNum > limitNum {
		player.LogError("useItem %d limit!", param.ItemId)
		return
	}

	residueNum := limitNum - cur

	useItemCount := int64(math.Floor(float64(residueNum) / float64(addNum)))
	if useItemCount > param.Count {
		useItemCount = param.Count
	}

	item := player.GetItemByHandle(param.Handle)
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FUseSwordGuildEnergyItem, &pb3.G2FUseSwordGuildEnergyItem{
		ItemId:    param.ItemId,
		Bind:      item.Bind,
		Count:     uint32(useItemCount),
		AddEnergy: uint32(useItemCount) * addNum,
	})
	if err != nil {
		player.LogError("G2FAddSwordGuildEnergy %d failed!", param.ItemId)
		return false, false, 0
	}
	return true, true, useItemCount
}

func init() {
	RegisterSysClass(sysdef.SiSwordGuild, func() iface.ISystem {
		return &SwordGuildSys{}
	})

	net.RegisterSysProtoV2(54, 41, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).joinSwordGuildShout
	})
	net.RegisterSysProtoV2(54, 115, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sEnterStronghold
	})
	net.RegisterSysProtoV2(54, 110, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sWorship
	})
	net.RegisterSysProtoV2(54, 111, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sRevPersonalScoreAwards
	})
	net.RegisterSysProtoV2(54, 112, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sRevSumChargeAwards
	})
	net.RegisterSysProtoV2(54, 113, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sRevStrongholdAwards
	})
	net.RegisterSysProtoV2(54, 114, sysdef.SiSwordGuild, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SwordGuildSys).c2sRevEnergy
	})

	event.RegActorEvent(custom_id.AeCharge, func(player iface.IPlayer, args ...interface{}) {
		handleSwordGuildCharge(player, args...)
	})

	gmevent.Register("swordguild.show", func(player iface.IPlayer, args ...string) bool {
		player.SendTipMsg(tipmsgid.TpStr, fmt.Sprintf("%t", manager.InSwordGuildCompetitionDay()))
		return true
	}, 1)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddSwordGuildEnergy, useItemAddSwordGuildEnergy)
}
