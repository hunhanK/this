/**
 * @Author: lzp
 * @Date: 2025/4/14
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type MayDayGiftSys struct {
	PlayerYYBase
}

func (s *MayDayGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MayDayGiftSys) Login() {
	s.s2cInfo()
}

func (s *MayDayGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *MayDayGiftSys) ResetData() {
	state := s.GetYYData()
	if state.MayDayGiftData == nil {
		return
	}
	delete(state.MayDayGiftData, s.Id)
}

func (s *MayDayGiftSys) s2cInfo() {
	s.SendProto3(127, 151, &pb3.S2C_127_151{
		ActId: s.Id,
		Data:  s.getData(),
	})
}

func (s *MayDayGiftSys) getData() *pb3.PYYMayDayGiftData {
	state := s.GetYYData()
	if state.MayDayGiftData == nil {
		state.MayDayGiftData = make(map[uint32]*pb3.PYYMayDayGiftData)
	}
	if state.MayDayGiftData[s.Id] == nil {
		state.MayDayGiftData[s.Id] = &pb3.PYYMayDayGiftData{}
	}
	return state.MayDayGiftData[s.Id]
}

func (s *MayDayGiftSys) addRecharge(charge uint32) {
	data := s.getData()
	data.Recharge += charge
	s.s2cInfo()
}

func (s *MayDayGiftSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_127_152
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	gConf := jsondata.GetMayDayGiftConfigByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil {
		return neterror.ConfNotFoundError("giftId:%d config not found", req.GiftId)
	}

	data := s.getData()
	if data.Recharge < gConf.AccRecharge {
		return neterror.ParamsInvalidError("giftId:%d bought limit", req.GiftId)
	}
	if utils.SliceContainsUint32(data.GiftIds, req.GiftId) {
		return neterror.ParamsInvalidError("giftId:%d has bought", req.GiftId)
	}

	player := s.GetPlayer()
	if !player.ConsumeByConf(gConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYMayDayGiftBuy}) {
		return neterror.ConsumeFailedError("giftId:%d buy failed", req.GiftId)
	}

	data.GiftIds = append(data.GiftIds, req.GiftId)
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(player, gConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYMayDayGiftBuy,
		})
	}
	s.SendProto3(127, 152, &pb3.S2C_127_152{
		ActId:   s.Id,
		GiftId:  req.GiftId,
		Rewards: jsondata.StdRewardVecToPb3RewardVec(gConf.Rewards),
	})

	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYMayDayGiftBuy, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", req.GiftId),
	})
	return nil
}

func (s *MayDayGiftSys) c2sReceive(msg *base.Message) error {
	var req pb3.C2S_127_153
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	tConf := jsondata.GetMayDayTimeRewardsConfig(s.ConfName, s.ConfIdx, req.Idx)
	if tConf == nil {
		return neterror.ConfNotFoundError("idx:%d config not found", req.Idx)
	}

	data := s.getData()
	if len(data.ReceiveIds) >= len(data.GiftIds) {
		return neterror.ParamsInvalidError("cannot receive times limit")
	}
	if utils.SliceContainsUint32(data.ReceiveIds, req.Idx) {
		return neterror.ParamsInvalidError("idx:%d rewards has received", req.Idx)
	}

	player := s.GetPlayer()
	data.ReceiveIds = append(data.ReceiveIds, req.Idx)
	if len(tConf.Rewards) > 0 {
		engine.GiveRewards(player, tConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYMayDayGiftTimesRewards,
		})
	}
	s.SendProto3(127, 153, &pb3.S2C_127_153{
		ActId: s.Id,
		Idx:   req.Idx,
	})

	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYMayDayGiftTimesRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.Id),
		StrArgs: fmt.Sprintf("%d", req.Idx),
	})
	return nil
}

func (s *MayDayGiftSys) PlayerCharge(chargeEvent *custom_id.ActorEventCharge) {
	s.addRecharge(chargeEvent.CashCent)
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMayDayGift, func() iface.IPlayerYY {
		return &MayDayGiftSys{}
	})

	net.RegisterYYSysProtoV2(127, 152, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*MayDayGiftSys).c2sBuy
	})
	net.RegisterYYSysProtoV2(127, 153, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*MayDayGiftSys).c2sReceive
	})
}
