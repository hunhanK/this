/**
 * @Author: lzp
 * @Date: 2024/10/28
 * @Desc: 充值返利
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
)

type RechargeRebateSys struct {
	*PlayerYYBase
}

func (s *RechargeRebateSys) Login() {
	s.S2CInfo()
}

func (s *RechargeRebateSys) OnReconnect() {
	s.S2CInfo()
}

func (s *RechargeRebateSys) S2CInfo() {
	s.SendProto3(69, 225, &pb3.S2C_69_225{ActiveId: s.GetId(), Data: s.GetData()})
}

func (s *RechargeRebateSys) GetData() *pb3.PYY_RechargeRebate {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.RechargeRebate == nil {
		state.RechargeRebate = make(map[uint32]*pb3.PYY_RechargeRebate)
	}
	if state.RechargeRebate[s.Id] == nil {
		state.RechargeRebate[s.Id] = &pb3.PYY_RechargeRebate{}
	}
	return state.RechargeRebate[s.Id]
}

func (s *RechargeRebateSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.RechargeRebate == nil {
		return
	}
	delete(state.RechargeRebate, s.Id)
}

func (s *RechargeRebateSys) chargeCheck(chargeId uint32) bool {
	gConf := jsondata.GetYYRechargeRebateGiftConf(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		s.LogError("rechargeRebate conf is nil")
		return false
	}

	data := s.GetData()
	if utils.SliceContainsUint32(data.IdL, gConf.Id) {
		s.LogError("rechargeRebate gift has charged id: %d", gConf.Id)
		return false
	}
	return true
}

func (s *RechargeRebateSys) chargeBack(chargeId uint32) bool {
	gConf := jsondata.GetYYRechargeRebateGiftConf(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		s.LogError("rechargeRebate conf is nil")
		return false
	}

	data := s.GetData()
	data.IdL = pie.Uint32s(data.IdL).Append(gConf.Id).Unique()

	player := s.GetPlayer()
	chargeConf := jsondata.GetChargeConf(chargeId)
	if chargeConf != nil {
		player.AddMoney(moneydef.Diamonds, int64(chargeConf.Diamond), true, pb3.LogId_LogCharge)
	}
	mailmgr.SendMailToActor(player.GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_RechargeRebateAwards,
		Rewards: jsondata.MergeStdReward(gConf.Rewards),
	})

	s.S2CInfo()
	return true
}

func rechargeRebateBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYRechargeRebate)
	for _, obj := range yyList {
		if s, ok := obj.(*RechargeRebateSys); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func rechargeRebateCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYRechargeRebate)
	for _, obj := range yyList {
		if s, ok := obj.(*RechargeRebateSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYRechargeRebate, func() iface.IPlayerYY {
		return &RechargeRebateSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})
	engine.RegChargeEvent(chargedef.ReChargeRebate, rechargeRebateCheck, rechargeRebateBack)
}
