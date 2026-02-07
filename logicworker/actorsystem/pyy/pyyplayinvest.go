/**
 * @Author: lzp
 * @Date: 2025/9/9
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	PlayInvestGiftType1 = 1 // 货币礼包
	PlayInvestGiftType2 = 2 // 直购礼包
)

const (
	PlayInvestTaskCondDay = 1 // 活动天数
)

type PlayerYYPlayInvest struct {
	PlayerYYBase
}

func (s *PlayerYYPlayInvest) OnReconnect() {
	s.s2cInfo()
}

func (s *PlayerYYPlayInvest) Login() {
	s.s2cInfo()
}

func (s *PlayerYYPlayInvest) NewDay() {
	data := s.getData()
	for giftId := range data.Gifts {
		s.updateGiftTask(giftId)
	}
	s.s2cInfo()
}

func (s *PlayerYYPlayInvest) OnOpen() {
	s.s2cInfo()
}

func (s *PlayerYYPlayInvest) OnEnd() {
	data := s.getData()

	var rewards jsondata.StdRewardVec
	for giftId := range data.Gifts {
		rewards = append(rewards, s.getNotRevTaskRewards(giftId)...)
	}

	conf := jsondata.GetPYYPlayInvestConf(s.ConfName, s.ConfIdx)
	if conf != nil {
		if len(rewards) > 0 {
			s.GetPlayer().SendMail(&mailargs.SendMailSt{
				ConfId:  uint16(conf.MailId),
				Rewards: rewards,
			})
		}
	}
}

func (s *PlayerYYPlayInvest) ResetData() {
	state := s.GetYYData()
	if state.PlayInvest == nil {
		return
	}
	delete(state.PlayInvest, s.Id)
}

func (s *PlayerYYPlayInvest) getData() *pb3.PYY_PlayInvest {
	state := s.GetYYData()
	if state.PlayInvest == nil {
		state.PlayInvest = make(map[uint32]*pb3.PYY_PlayInvest)
	}
	if state.PlayInvest[s.Id] == nil {
		state.PlayInvest[s.Id] = &pb3.PYY_PlayInvest{}
	}
	if state.PlayInvest[s.Id].Gifts == nil {
		state.PlayInvest[s.Id].Gifts = make(map[uint32]*pb3.PYYPlayInvestGift)
	}
	return state.PlayInvest[s.Id]
}

func (s *PlayerYYPlayInvest) s2cInfo() {
	s.SendProto3(127, 185, &pb3.S2C_127_185{
		ActId: s.Id,
		Data:  s.getData(),
	})
}

func (s *PlayerYYPlayInvest) getNotRevTaskRewards(giftId uint32) jsondata.StdRewardVec {
	data := s.getData()
	gData, ok := data.Gifts[giftId]
	if !ok {
		return nil
	}

	gConf := jsondata.GetPYYPlayInvestGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return nil
	}

	var rewards jsondata.StdRewardVec
	for _, tConf := range gConf.TaskRewards {
		if utils.SliceContainsUint32(gData.TaskRewards, tConf.Id) {
			continue
		}
		if tConf.CondValue <= gData.TaskValue {
			rewards = append(rewards, tConf.Rewards...)
		}
	}
	return rewards
}

func (s *PlayerYYPlayInvest) checkCond(giftId, id uint32) bool {
	gConf := jsondata.GetPYYPlayInvestGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return false
	}

	tConf := jsondata.GetPYYPlayInvestGiftTaskConf(s.ConfName, s.ConfIdx, giftId, id)
	if tConf == nil {
		return false
	}

	data := s.getData()
	gData, ok := data.Gifts[giftId]
	if !ok {
		return false
	}

	return gData.TaskValue >= tConf.CondValue
}

func (s *PlayerYYPlayInvest) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_127_186
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	gConf := jsondata.GetPYYPlayInvestGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil || gConf.GiftType != PlayInvestGiftType1 {
		return neterror.ParamsInvalidError("giftId: %d type error", req.GiftId)
	}

	data := s.getData()
	gData, ok := data.Gifts[req.GiftId]
	if ok && gData.IsBuy {
		return neterror.ParamsInvalidError("giftId: %d has bought", req.GiftId)
	}

	data.Gifts[req.GiftId] = &pb3.PYYPlayInvestGift{
		GiftId: req.GiftId,
		IsBuy:  true,
	}

	engine.BroadcastTipMsgById(tipmsgid.PlayerYYPlayInvestTip, s.GetPlayer().GetId(), s.GetPlayer().GetName(), s.Id)
	s.updateGiftTask(req.GiftId)
	s.s2cInfo()
	return nil
}

func (s *PlayerYYPlayInvest) c2sRevBuyRewards(msg *base.Message) error {
	var req pb3.C2S_127_187
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	gConf := jsondata.GetPYYPlayInvestGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil {
		return neterror.ParamsInvalidError("giftId: %d config nil", req.GiftId)
	}

	data := s.getData()
	gData, ok := data.Gifts[req.GiftId]
	if !ok || !gData.IsBuy {
		return neterror.ParamsInvalidError("giftId: %d not buy", req.GiftId)
	}
	if gData.IsRev {
		return neterror.ParamsInvalidError("giftId: %d has fetched", req.GiftId)
	}

	gData.IsRev = true
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYPlayInvestGiftBuyRewards,
		})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYPlayInvestGiftBuyRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", gConf.GiftId),
	})

	s.GetPlayer().SendShowRewardsPopByPYY(gConf.Rewards, s.Id)
	s.s2cInfo()
	return nil
}

func (s *PlayerYYPlayInvest) c2sRevTaskRewards(msg *base.Message) error {
	var req pb3.C2S_127_188
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	giftId, id := req.GiftId, req.Id
	tConf := jsondata.GetPYYPlayInvestGiftTaskConf(s.ConfName, s.ConfIdx, giftId, id)
	if tConf == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	if !s.checkCond(giftId, id) {
		return neterror.ParamsInvalidError("rev task cond not meet")
	}

	data := s.getData()
	gData, ok := data.Gifts[giftId]
	if !ok {
		return neterror.ParamsInvalidError("giftId: %d not buy", giftId)
	}
	if utils.SliceContainsUint32(gData.TaskRewards, req.Id) {
		return neterror.ParamsInvalidError("giftId:%d, taskId:%d has fetched", giftId, id)
	}

	gData.TaskRewards = append(gData.TaskRewards, req.Id)
	if len(tConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), tConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYPlayInvestGiftTaskRewards,
		})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYPlayInvestGiftTaskRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d, %d", giftId, id),
	})
	s.GetPlayer().SendShowRewardsPopByPYY(tConf.Rewards, s.Id)
	s.s2cInfo()
	return nil
}

func (s *PlayerYYPlayInvest) updateGiftTask(giftId uint32) {
	gConf := jsondata.GetPYYPlayInvestGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return
	}

	data := s.getData()
	gData, ok := data.Gifts[giftId]
	if !ok {
		return
	}

	switch gConf.TaskCond {
	case PlayInvestTaskCondDay:
		gData.TaskValue = s.GetOpenDay()
	default:
	}
}

func (s *PlayerYYPlayInvest) chargeCheck(chargeId uint32) bool {
	gConf := jsondata.GetPYYPlayInvestGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil || gConf.GiftType != PlayInvestGiftType2 {
		return false
	}

	data := s.getData()
	gData, ok := data.Gifts[gConf.GiftId]
	if ok && gData.IsBuy {
		return false
	}
	return true
}

func (s *PlayerYYPlayInvest) chargeBack(chargeId uint32) bool {
	gConf := jsondata.GetPYYPlayInvestGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	data := s.getData()

	data.Gifts[gConf.GiftId] = &pb3.PYYPlayInvestGift{
		GiftId: gConf.GiftId,
		IsBuy:  true,
	}
	engine.BroadcastTipMsgById(tipmsgid.PlayerYYPlayInvestTip, s.GetPlayer().GetId(), s.GetPlayer().GetName(), s.Id)
	s.updateGiftTask(gConf.GiftId)
	s.s2cInfo()
	return true
}

func pyyPlayInvestGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYPlayInvest)
	for _, obj := range yyObjs {
		if s, ok := obj.(*PlayerYYPlayInvest); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func pyyPlayInvestGiftBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYPlayInvest)
	for _, obj := range yyObjs {
		if s, ok := obj.(*PlayerYYPlayInvest); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYPlayInvest, func() iface.IPlayerYY {
		return &PlayerYYPlayInvest{}
	})

	engine.RegChargeEvent(chargedef.PlayerYYPlayInVestGift, pyyPlayInvestGiftCheck, pyyPlayInvestGiftBack)

	net.RegisterYYSysProtoV2(127, 186, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlayInvest).c2sBuy
	})
	net.RegisterYYSysProtoV2(127, 187, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlayInvest).c2sRevBuyRewards
	})
	net.RegisterYYSysProtoV2(127, 188, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYPlayInvest).c2sRevTaskRewards
	})
}
