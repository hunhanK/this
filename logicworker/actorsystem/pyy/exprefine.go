package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type ExpRefineSys struct {
	PlayerYYBase
}

func (s *ExpRefineSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.ExpRefine {
		return
	}
	delete(yyData.ExpRefine, s.Id)
}

func (s *ExpRefineSys) GetData() *pb3.PYY_ExpRefine {
	yyData := s.GetYYData()
	if nil == yyData.ExpRefine {
		yyData.ExpRefine = make(map[uint32]*pb3.PYY_ExpRefine)
	}
	if nil == yyData.ExpRefine[s.Id] {
		yyData.ExpRefine[s.Id] = &pb3.PYY_ExpRefine{}
	}
	return yyData.ExpRefine[s.Id]
}

func c2sRefineExp(sys iface.IPlayerYY) func(msg *base.Message) error {
	return func(msg *base.Message) error {
		s, ok := sys.(*ExpRefineSys)
		if !ok {
			return neterror.InternalError("exprefine sys is nil")
		}
		data := s.GetData()
		conf := jsondata.GetYYExpRefineConf(s.ConfName, s.ConfIdx)
		if nil == conf {
			return neterror.ConfNotFoundError("no exprefine conf")
		}
		if data.DailyTimes >= conf.RefineTimes {
			s.player.SendTipMsg(tipmsgid.TpBuyTimesLimit)
			return nil
		}
		idx := int(data.DailyTimes)
		if idx >= len(conf.GiveConf) {
			idx = len(conf.GiveConf) - 1
		}
		giveConf := conf.GiveConf[idx]
		if !s.player.ConsumeByConf(giveConf.Consume, false, common.ConsumeParams{
			LogId:   pb3.LogId_LogExpRefine,
			SubType: s.GetId(),
		}) {
			s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}

		addExp := giveConf.ExpAdd
		if addExp <= 0 {
			s.LogError("exprefine addexp:0 for idx(%d)", idx)
			return nil
		}
		data.DailyTimes++
		if !s.player.AddMoney(moneydef.Exp, addExp, false, pb3.LogId_LogExpRefine) {
			return neterror.InternalError("has unknown err in exprefine")
		}
		s.S2CInfo()
		if conf.Broadcast > 0 {
			engine.BroadcastTipMsgById(conf.Broadcast, s.player.GetName(), addExp)
		}
		s.SendProto3(138, 1, &pb3.S2C_138_1{
			AddExp: addExp,
		})
		return nil
	}
}

func (s *ExpRefineSys) S2CInfo() {
	s.SendProto3(138, 0, &pb3.S2C_138_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *ExpRefineSys) Login() {
	s.S2CInfo()
}

func (s *ExpRefineSys) OnReconnect() {
	s.S2CInfo()
}

func (s *ExpRefineSys) OnOpen() {
	s.GetData()
	s.GetYYData().ExpRefine[s.Id] = &pb3.PYY_ExpRefine{}
	s.S2CInfo()
}

func onNewDayExpRefine(player iface.IPlayer, args ...interface{}) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYExpRefine)
	if nil == yyList {
		return
	}
	for _, v := range yyList {
		sys, ok := v.(*ExpRefineSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		data := sys.GetData()
		data.DailyTimes = 0
		sys.S2CInfo()
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYExpRefine, func() iface.IPlayerYY {
		return &ExpRefineSys{}
	})

	net.RegisterYYSysProtoV2(138, 1, c2sRefineExp)
	event.RegActorEvent(custom_id.AeNewDay, onNewDayExpRefine)

}
