/**
 * @Author: lzp
 * @Date: 2025/5/20
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
	"math"
)

type MayDayTurnTable struct {
	PlayerYYBase
}

func (s *MayDayTurnTable) Login() {
	s.s2cInfo()
}

func (s *MayDayTurnTable) OnReconnect() {
	s.s2cInfo()
}

func (s *MayDayTurnTable) OnOpen() {
	s.s2cInfo()
}

func (s *MayDayTurnTable) OnEnd() {
	conf := jsondata.GetPYYMayDayTurnTableConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	var rewards jsondata.StdRewardVec
	for _, dConf := range conf.DayRewards {
		if utils.SliceContainsUint32(data.DayRevIds, dConf.Id) {
			continue
		}
		if data.Count < dConf.Times {
			continue
		}
		rewards = append(rewards, dConf.Rewards...)
	}

	if len(rewards) > 0 {
		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (s *MayDayTurnTable) GetData() *pb3.PYY_MayDayTurnTable {
	state := s.GetYYData()
	if state.MayDayTurn == nil {
		state.MayDayTurn = make(map[uint32]*pb3.PYY_MayDayTurnTable)
	}
	if state.MayDayTurn[s.Id] == nil {
		state.MayDayTurn[s.Id] = &pb3.PYY_MayDayTurnTable{}
	}
	return state.MayDayTurn[s.Id]
}

func (s *MayDayTurnTable) ResetData() {
	state := s.GetYYData()
	if state.MayDayTurn == nil {
		return
	}
	delete(state.MayDayTurn, s.Id)
}

func (s *MayDayTurnTable) NewDay() {
	data := s.GetData()
	data.AccRecharge = 0
	data.Count = 0
	data.Ids = data.Ids[:0]
	data.DayRevIds = data.DayRevIds[:0]
	s.s2cInfo()
}

func (s *MayDayTurnTable) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	chargeCent := chargeEvent.CashCent
	chargeId := chargeEvent.ChargeId
	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf != nil && chargeConf.ChargeType == chargedef.Charge {
		s.onAddCharge(chargeCent)
		s.s2cInfo()
	}
}

func (s *MayDayTurnTable) onAddCharge(chargeCent uint32) {
	data := s.GetData()
	data.AccRecharge += chargeCent
}

func (s *MayDayTurnTable) s2cInfo() {
	s.SendProto3(127, 156, &pb3.S2C_127_156{ActId: s.Id, Data: s.GetData()})
}

func (s *MayDayTurnTable) c2sTurn(msg *base.Message) error {
	var req pb3.C2S_127_157
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMayDayTurnTableConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	tConf, ok := conf.TurnTable[req.Id]
	if !ok {
		return neterror.ConfNotFoundError("conf not exit")
	}

	data := s.GetData()
	if data.AccRecharge < tConf.AccRecharge {
		return neterror.ParamsInvalidError("charge limit")
	}

	if utils.SliceContainsUint32(data.Ids, req.Id) {
		return neterror.ParamsInvalidError("turn limit")
	}

	// 消耗
	if !s.GetPlayer().DeductMoney(tConf.MoneyType, int64(tConf.MoneyCount), common.ConsumeParams{LogId: pb3.LogId_LogYYMayDayTurntableConsume}) {
		s.GetPlayer().LogWarn("money not enough")
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 设置状态
	data.Ids = append(data.Ids, req.Id)
	data.Count += 1

	// 发奖励
	pool := new(random.Pool)
	for i := range tConf.TurnTableWeight {
		pool.AddItem(tConf.TurnTableWeight[i], tConf.TurnTableWeight[i].Weight)
	}
	wConf := pool.RandomOne().(*jsondata.MayDayTurnTableWeight)
	moneyCount := math.Ceil(float64(tConf.MoneyCount) * float64(wConf.Ratio) / 100)

	s.GetPlayer().AddMoney(tConf.MoneyType, int64(moneyCount), true, pb3.LogId_LogYYMayDayTurntableAwards)

	s.SendProto3(127, 157, &pb3.S2C_127_157{ActId: s.Id, Id: req.Id, Idx: wConf.Idx, Count: data.Count})
	return nil
}

func (s *MayDayTurnTable) c2sFetchCountRewards(msg *base.Message) error {
	var req pb3.C2S_127_157
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYMayDayTurnTableConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf not exit")
	}

	dConf, ok := conf.DayRewards[req.Id]
	if !ok {
		return neterror.ConfNotFoundError("conf not exit")
	}

	data := s.GetData()
	if data.Count < dConf.Times {
		return neterror.ParamsInvalidError("count limit")
	}

	if utils.SliceContainsUint32(data.DayRevIds, req.Id) {
		return neterror.ParamsInvalidError("has received")
	}

	data.DayRevIds = append(data.DayRevIds, req.Id)
	engine.GiveRewards(s.GetPlayer(), dConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYMayDayTurntableDayAwards})

	s.SendProto3(127, 158, &pb3.S2C_127_158{ActId: s.Id, Id: req.Id})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMayDayTurnTable, func() iface.IPlayerYY {
		return &MayDayTurnTable{}
	})

	net.RegisterYYSysProtoV2(127, 157, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayTurnTable).c2sTurn
	})
	net.RegisterYYSysProtoV2(127, 158, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayTurnTable).c2sFetchCountRewards
	})
}
