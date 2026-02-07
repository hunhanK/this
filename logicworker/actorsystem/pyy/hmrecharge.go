/**
 * @Author: lzp
 * @Date: 2025/6/16
 * @Desc: 鸿蒙累充
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type HMRecharge struct {
	PlayerYYBase
}

func (s *HMRecharge) Login() {
	s.s2cInfo()
}

func (s *HMRecharge) OnReconnect() {
	s.s2cInfo()
}

func (s *HMRecharge) OnOpen() {
	data := s.GetData()
	zeroTime := time_util.GetZeroTime(0)
	data.Recharge = s.GetDailyChargeMoney(zeroTime)
	s.s2cInfo()
}

func (s *HMRecharge) OnEnd() {
	data := s.GetData()
	conf := jsondata.GetPYYHMRechargeConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	var rewards jsondata.StdRewardVec
	for id, value := range conf.IntegralRewards {
		if utils.SliceContainsUint32(data.Ids, id) {
			continue
		}
		if data.Recharge < value.Integral {
			continue
		}
		rewards = append(rewards, value.Rewards...)
		data.Ids = append(data.Ids, id)
	}
	if len(rewards) > 0 {
		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_PYYHmChargeAwards,
			Rewards: rewards,
		})
	}
}

func (s *HMRecharge) ResetData() {
	state := s.GetYYData()
	if nil == state.HmRecharge {
		return
	}
	delete(state.HmRecharge, s.Id)
}

func (s *HMRecharge) GetData() *pb3.PYY_HMRecharge {
	state := s.GetYYData()
	if state.HmRecharge == nil {
		state.HmRecharge = make(map[uint32]*pb3.PYY_HMRecharge)
	}
	if state.HmRecharge[s.Id] == nil {
		state.HmRecharge[s.Id] = &pb3.PYY_HMRecharge{}
	}
	return state.HmRecharge[s.Id]
}

func (s *HMRecharge) s2cInfo() {
	s.SendProto3(127, 170, &pb3.S2C_127_170{ActId: s.Id, Data: s.GetData()})
}

func (s *HMRecharge) c2sFetch(msg *base.Message) error {
	var req pb3.C2S_127_171
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	conf := jsondata.GetPYYHMRechargeConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("config not found")
	}

	data := s.GetData()

	var rewards jsondata.StdRewardVec
	var idL []uint32
	for id, value := range conf.IntegralRewards {
		if utils.SliceContainsUint32(data.Ids, id) {
			continue
		}
		if data.Recharge < value.Integral {
			continue
		}
		idL = append(idL, id)
		rewards = append(rewards, value.Rewards...)
	}

	data.Ids = pie.Uint32s(data.Ids).Append(idL...).Unique()
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYHMRechargeAwards,
		})
	}

	player := s.GetPlayer()
	for _, id := range idL {
		if iConf, ok := conf.IntegralRewards[id]; ok {
			if iConf.Broadcast > 0 {
				engine.BroadcastTipMsgById(iConf.Broadcast, player.GetId(), player.GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), iConf.Rewards))
			}
		}
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYHMRechargeAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%v", idL),
	})
	s.s2cInfo()
	return nil
}

func (s *HMRecharge) addRecharge(charge uint32) {
	conf := jsondata.GetPYYHMRechargeConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	data := s.GetData()
	data.Recharge += charge * conf.IntegralRatio
	s.s2cInfo()
}

func (s *HMRecharge) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	s.addRecharge(chargeEvent.CashCent)
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYHMRecharge, func() iface.IPlayerYY {
		return &HMRecharge{}
	})

	net.RegisterYYSysProto(127, 171, (*HMRecharge).c2sFetch)
}
