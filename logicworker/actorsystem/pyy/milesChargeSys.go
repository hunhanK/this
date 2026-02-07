/**
 * @Author: LvYuMeng
 * @Date: 2024/10/28
 * @Desc: 法则累充（里程充值）
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type MileChargeSys struct {
	PlayerYYBase
}

func (s *MileChargeSys) MergeFix() {
	openZeroTime := time_util.GetZeroTime(s.OpenTime)
	openDayCent := s.GetDailyChargeMoney(openZeroTime)

	if openDayCent > 0 {
		data := s.GetData()
		data.Cent = openDayCent
		s.s2cInfo()
	}
}

func (s *MileChargeSys) s2cInfo() {
	s.SendProto3(69, 210, &pb3.S2C_69_210{
		ActiveId: s.GetId(),
		Data:     s.GetData(),
	})
}

func (s *MileChargeSys) GetData() *pb3.PYY_MilesCharge {
	state := s.GetYYData()
	if nil == state.MilesCharge {
		state.MilesCharge = make(map[uint32]*pb3.PYY_MilesCharge)
	}
	if nil == state.MilesCharge[s.Id] {
		state.MilesCharge[s.Id] = &pb3.PYY_MilesCharge{}
	}
	return state.MilesCharge[s.Id]
}

func (s *MileChargeSys) packCanRevRewards(id uint32) ([]uint32, jsondata.StdRewardVec) {
	conf := jsondata.GetYYMilesChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil, nil
	}

	data := s.GetData()
	var ids []uint32
	var rewardsVec []jsondata.StdRewardVec
	for _, v := range conf.Charges {
		if id > 0 && v.Id != id {
			continue
		}
		if data.GetCent() < v.Cent {
			continue
		}
		if pie.Uint32s(data.RevIds).Contains(v.Id) {
			continue
		}
		ids = append(ids, v.Id)
		rewardsVec = append(rewardsVec, v.Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardsVec...)
	return ids, rewards
}

func (s *MileChargeSys) OnEnd() {
	conf := jsondata.GetYYMilesChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.GetData()
	ids, rewards := s.packCanRevRewards(0)
	data.RevIds = append(data.RevIds, ids...)

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *MileChargeSys) OnOpen() {
	s.s2cInfo()
}

func (s *MileChargeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.MilesCharge {
		return
	}
	delete(state.MilesCharge, s.GetId())
}

func (s *MileChargeSys) Login() {
	s.s2cInfo()
}

func (s *MileChargeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MileChargeSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	if !s.IsOpen() {
		return
	}

	chargeId := chargeEvent.ChargeId

	chargeConf := jsondata.GetChargeConf(chargeId)
	if nil == chargeConf {
		return
	}

	chargeCent := chargeEvent.CashCent

	data := s.GetData()
	data.Cent += chargeCent

	s.s2cInfo()
}

func (s *MileChargeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_69_211
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYMilesChargeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	data := s.GetData()
	ids, rewards := s.packCanRevRewards(req.GetId())

	if len(ids) == 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	data.RevIds = append(data.RevIds, ids...)
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPYYMilesChargeAwards,
	})
	engine.BroadcastTipMsgById(tipmsgid.MilesChargeRev, s.GetPlayer().GetId(), s.GetPlayer().GetName(), s.GetId(), engine.StdRewardToBroadcast(s.GetPlayer(), rewards))

	s.s2cInfo()

	return nil
}

func offlineGMAddMileCharge(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		player.LogError("offlineGMAddMileCharge convert CommonSt failed")
		return
	}
	var (
		yyId    = st.U32Param
		score   = st.U32Param2
		replace = st.BParam
	)

	obj := pyymgr.GetPlayerYYObj(player, yyId)
	if nil == obj || !obj.IsOpen() {
		return
	}

	mileChargeSys, ok := obj.(*MileChargeSys)
	if !ok {
		return
	}

	mileChargeSys.updateScore(score, replace)
}

func (s *MileChargeSys) updateScore(cent uint32, isReplace bool) {
	data := s.GetData()
	if !isReplace {
		data.Cent += cent
	} else {
		data.Cent = cent
	}

	s.s2cInfo()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMileChargeSys, func() iface.IPlayerYY {
		return &MileChargeSys{}
	})

	net.RegisterYYSysProtoV2(69, 211, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MileChargeSys).c2sRev
	})

	engine.RegisterMessage(gshare.OfflineGMAddMileCharge, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineGMAddMileCharge)

	gmevent.Register("addMileCharge", func(player iface.IPlayer, args ...string) bool {
		addScore := utils.AtoUint32(args[0])
		replace := utils.AtoUint32(args[1])
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYMileChargeSys)
		for _, obj := range yyList {
			if s, ok := obj.(*MileChargeSys); ok && s.IsOpen() {
				s.updateScore(addScore, replace == 1)
				return true
			}
		}
		return false
	}, 1)
}
