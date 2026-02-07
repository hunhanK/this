/**
 * @Author: LvYuMeng
 * @Date: 2024/6/5
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

type PreferentialGiftSys struct {
	PlayerYYBase
}

func (s *PreferentialGiftSys) GetData() *pb3.PYY_PreferentialGift {
	state := s.GetYYData()
	if nil == state.PreferentialGift {
		state.PreferentialGift = make(map[uint32]*pb3.PYY_PreferentialGift)
	}
	if state.PreferentialGift[s.Id] == nil {
		state.PreferentialGift[s.Id] = &pb3.PYY_PreferentialGift{}
	}
	if nil == state.PreferentialGift[s.Id].Gift {
		state.PreferentialGift[s.Id].Gift = make(map[uint32]uint32)
	}
	return state.PreferentialGift[s.Id]
}

func (s *PreferentialGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *PreferentialGiftSys) OnEnd() {
	conf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	data := s.GetData()
	var totalCount uint32
	for _, count := range data.Gift {
		totalCount += count
	}

	var rewards jsondata.StdRewardVec
	for _, bConf := range conf.BuyCountRewards {
		if utils.IsSetBit64(data.ReceiveFlag, bConf.Idx) {
			continue
		}
		if totalCount < bConf.Count {
			continue
		}
		rewards = append(rewards, bConf.Rewards...)
		data.ReceiveFlag = utils.SetBit64(data.ReceiveFlag, bConf.Idx)
	}
	if len(rewards) > 0 {
		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId: common.Mail_PreferentialGiftBuyCountRewards,
			Content: &mailargs.ReachStandardArgs{
				Name: s.GetActName(),
			},
			Rewards: rewards,
		})
	}
}

func (s *PreferentialGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.PreferentialGift {
		return
	}
	delete(state.PreferentialGift, s.GetId())
}

func (s *PreferentialGiftSys) Login() {
	s.s2cInfo()
}

func (s *PreferentialGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PreferentialGiftSys) s2cInfo() {
	s.SendProto3(47, 15, &pb3.S2C_47_15{
		ActiveId: s.GetId(),
		Gift:     s.GetData(),
	})
}

const (
	PreferentialGiftTypeFree           = 1 // 免费礼包
	PreferentialGiftTypeConsumeBuy     = 2 // 货币直购
	PreferentialGiftTypeCharge         = 3 // 充值直购
	PreferentialGiftTypeQuickCharge    = 4 // 一键充值直购
	PreferentialGiftTypeDaily          = 5 // 每日礼包
	PreferentialGiftTypeQuickChargeAll = 6 // 一键充值直购所有礼包
)

func (s *PreferentialGiftSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_47_16
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PreferentialGiftSys conf nil")
	}

	id := req.GetId()
	giftConf, ok := conf.Gifts[id]
	if !ok {
		return neterror.ConfNotFoundError("PreferentialGiftSys gift conf(%d) nil", id)
	}
	data := s.GetData()
	if data.Gift[id] >= giftConf.Count {
		return neterror.ParamsInvalidError("PreferentialGiftSys gift conf(%d) buy limit", id)
	}

	switch giftConf.Type {
	case PreferentialGiftTypeFree:
	case PreferentialGiftTypeConsumeBuy:
		if !s.GetPlayer().ConsumeByConf(giftConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPreferentialGiftBuy}) {
			s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	case PreferentialGiftTypeDaily:
	default:
		return neterror.ConfNotFoundError("PreferentialGiftSys no define type(%d)", giftConf.Type)
	}

	data.Gift[id]++
	engine.GiveRewards(s.GetPlayer(), giftConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPreferentialGiftBuy})
	if giftConf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(giftConf.BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), giftConf.Rewards))
	}

	s.SendProto3(47, 16, &pb3.S2C_47_16{
		ActiveId: s.GetId(),
		Id:       id,
		Times:    data.Gift[id],
	})
	return nil
}

func (s *PreferentialGiftSys) c2sRevCountAwards(msg *base.Message) error {
	var req pb3.C2S_47_18
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	conf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("PreferentialGiftSys conf nil")
	}

	bConf := jsondata.GetYYPreferentialBuyCountConf(s.ConfName, s.ConfIdx, req.GetIdx())
	if bConf == nil {
		return neterror.ConfNotFoundError("PreferentialGiftSys buy count conf(%d) nil", req.GetIdx())
	}

	data := s.GetData()
	if utils.IsSetBit64(data.ReceiveFlag, bConf.Idx) {
		return neterror.ParamsInvalidError("PreferentialGiftSys buy count conf(%d) received", req.GetIdx())
	}

	var totalCount uint32
	for _, count := range data.Gift {
		totalCount += count
	}
	if totalCount < bConf.Count {
		return neterror.ParamsInvalidError("PreferentialGiftSys buy count conf(%d) buy count not enough", req.GetIdx())
	}

	data.ReceiveFlag = utils.SetBit64(data.ReceiveFlag, bConf.Idx)
	if len(bConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYPreferentialBuyCountAwards})
	}
	s.SendProto3(47, 18, &pb3.S2C_47_18{
		ActiveId:    s.Id,
		ReceiveFlag: data.ReceiveFlag,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYPreferentialBuyCountAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", bConf.Idx),
	})
	return nil
}

func (s *PreferentialGiftSys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	var giftConf *jsondata.PreferentialGiftConf
	for _, v := range conf.Gifts {
		if v.ChargeId != chargeConf.ChargeId {
			if v.BatchCharge == nil || len(v.BatchCharge) == 0 {
				continue
			}
			if _, ok := v.BatchCharge[chargeConf.ChargeId]; !ok {
				continue
			}
		}
		giftConf = v
		break
	}
	if giftConf == nil {
		return false
	}

	// 父礼包购买上限
	curCount := s.GetData().Gift[giftConf.Id]
	if curCount >= giftConf.Count {
		return false
	}

	// 校验是不是买的批量礼包
	if giftConf.ChargeId != chargeConf.ChargeId {
		if giftConf.BatchCharge == nil || len(giftConf.BatchCharge) == 0 {
			return false
		}
		chargeConf, ok := giftConf.BatchCharge[chargeConf.ChargeId]
		if !ok {
			return false
		}
		if chargeConf.Count+curCount > giftConf.Count {
			return false
		}
	}

	// 一键购买直购 需要校验其他直购礼包买了没
	var ret = true
	if giftConf.Type == PreferentialGiftTypeQuickCharge {
		for giftId, v := range conf.Gifts {
			if v.Type != PreferentialGiftTypeCharge {
				continue
			}
			if s.GetData().Gift[giftId] == 0 {
				continue
			}
			ret = false
			break
		}
	}
	if !ret {
		return false
	}

	// 一键充值直购所有礼包 需要校验其他直购礼包买了没
	if giftConf.Type == PreferentialGiftTypeQuickChargeAll {
		for giftId, v := range conf.Gifts {
			if v.Type != PreferentialGiftTypeCharge {
				continue
			}
			if s.GetData().Gift[giftId] == 0 {
				continue
			}
			ret = false
			break
		}
	}
	return ret
}

func (s *PreferentialGiftSys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	if !s.chargeCheck(chargeConf) {
		return false
	}

	gConf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if nil == gConf {
		return false
	}

	var giftConf *jsondata.PreferentialGiftConf
	for _, v := range gConf.Gifts {
		if v.ChargeId != chargeConf.ChargeId {
			if v.BatchCharge == nil || len(v.BatchCharge) == 0 {
				continue
			}
			if _, ok := v.BatchCharge[chargeConf.ChargeId]; !ok {
				continue
			}
		}
		giftConf = v
		break
	}

	if nil == giftConf {
		s.LogWarn("PreferentialGiftSys charge conf(%d) is nil", chargeConf.ChargeId)
		return false
	}

	data := s.GetData()
	if data.Gift[giftConf.Id] >= giftConf.Count {
		s.LogError("player(%d) buy preferential gift(%d) repeated!", s.GetPlayer().GetId(), giftConf.Id)
		return false
	}

	var count = uint32(1)
	// 校验是不是买的批量礼包
	if giftConf.ChargeId != chargeConf.ChargeId {
		if giftConf.BatchCharge == nil || len(giftConf.BatchCharge) == 0 {
			s.LogError("player(%d) buy preferential gift(%d) not found batch charge!", s.GetPlayer().GetId(), giftConf.Id)
			return false
		}
		chargeConf, ok := giftConf.BatchCharge[chargeConf.ChargeId]
		if !ok {
			s.LogError("player(%d) buy preferential gift(%d) not found batch charge!", s.GetPlayer().GetId(), giftConf.Id)
			return false
		}
		count = chargeConf.Count
	}

	data.Gift[giftConf.Id] += count
	var buyIds = make(map[uint32]uint32)
	buyIds[giftConf.Id] = data.Gift[giftConf.Id]

	// 组装奖励
	var rewards = jsondata.StdRewardMulti(giftConf.Rewards, int64(count))

	// 一键直购
	if giftConf.Type == PreferentialGiftTypeQuickCharge {
		for _, v := range gConf.Gifts {
			if v.Type != PreferentialGiftTypeCharge {
				continue
			}
			data.Gift[v.Id] += 1
			buyIds[v.Id] = data.Gift[v.Id]
			rewards = append(rewards, v.Rewards...)
		}
	}

	// 一键充值直购所有礼包
	if giftConf.Type == PreferentialGiftTypeQuickChargeAll {
		for _, v := range gConf.Gifts {
			if v.Type != PreferentialGiftTypeCharge {
				continue
			}
			data.Gift[v.Id] += v.Count
			buyIds[v.Id] = data.Gift[v.Id]
			rewardMulti := jsondata.StdRewardMulti(v.Rewards, int64(v.Count))
			rewards = append(rewards, rewardMulti...)
		}
		rewards = jsondata.MergeStdReward(rewards)
	}

	if len(rewards) > 0 {
		rewards = jsondata.MergeStdReward(rewards)
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPreferentialGiftBuy})
		s.GetPlayer().SendShowRewardsPop(rewards)
	}

	if giftConf.BroadcastId > 0 {
		engine.BroadcastTipMsgById(giftConf.BroadcastId, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), giftConf.Rewards))
	}

	if len(buyIds) == 1 {
		s.SendProto3(47, 16, &pb3.S2C_47_16{
			ActiveId: s.GetId(),
			Id:       giftConf.Id,
			Times:    data.Gift[giftConf.Id],
		})
		return true
	}

	s.SendProto3(47, 17, &pb3.S2C_47_17{
		ActiveId:    s.GetId(),
		BuyTimesMap: buyIds,
	})

	return true
}

func (s *PreferentialGiftSys) NewDay() {
	conf := jsondata.GetYYPreferentialGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	for _, v := range conf.Gifts {
		if v.Reset {
			delete(data.Gift, v.Id)
		}
	}
	if conf.IsReset {
		data.ReceiveFlag = 0
	}
	s.s2cInfo()
}

func preferentialGiftChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYPreferentialGift)
	for _, obj := range yyList {
		if s, ok := obj.(*PreferentialGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf) {
				return true
			}
		}
	}
	return false
}

func preferentialGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYPreferentialGift)
	for _, obj := range yyList {
		if s, ok := obj.(*PreferentialGiftSys); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPreferentialGift, func() iface.IPlayerYY {
		return &PreferentialGiftSys{}
	})

	net.RegisterYYSysProtoV2(47, 16, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PreferentialGiftSys).c2sRev
	})

	net.RegisterYYSysProtoV2(47, 18, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*PreferentialGiftSys).c2sRevCountAwards
	})
	engine.RegChargeEvent(chargedef.PreferentialGift, preferentialGiftChargeCheck, preferentialGiftChargeBack)

}
