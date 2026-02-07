/**
 * @Author: lzp
 * @Date: 2024/12/26
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
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

type PremiumGiftSys struct {
	PlayerYYBase
}

func (s *PremiumGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *PremiumGiftSys) Login() {
	s.s2cInfo()
}

func (s *PremiumGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *PremiumGiftSys) NewDay() {
	data := s.getData()
	data.IsRecDailyRewards = false
	s.s2cInfo()
}

func (s *PremiumGiftSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.PremiumGift == nil {
		return
	}
	delete(state.PremiumGift, s.Id)
}

func (s *PremiumGiftSys) s2cInfo() {
	s.SendProto3(127, 128, &pb3.S2C_127_128{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *PremiumGiftSys) getData() *pb3.PYY_PremiumGift {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.PremiumGift == nil {
		state.PremiumGift = make(map[uint32]*pb3.PYY_PremiumGift)
	}
	if state.PremiumGift[s.Id] == nil {
		state.PremiumGift[s.Id] = &pb3.PYY_PremiumGift{}
	}
	data := state.PremiumGift[s.Id]
	if data.BuyData == nil {
		data.BuyData = make(map[uint32]uint32)
	}
	return data
}

func (s *PremiumGiftSys) getCount() uint32 {
	data := s.getData()
	count := uint32(0)
	for _, v := range data.BuyData {
		count += v
	}
	return count
}

func (s *PremiumGiftSys) c2sRecDailyRewards(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_130
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetPYYPremiumGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("config not found")
	}

	data := s.getData()
	if data.IsRecDailyRewards {
		return neterror.ParamsInvalidError("has receive daily rewards")
	}

	data.IsRecDailyRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPremiumGiftDailyRewards})
	}
	s.SendProto3(127, 130, &pb3.S2C_127_130{ActId: s.GetId(), IsRec: true})
	return nil
}

func (s *PremiumGiftSys) c2sRecAccRewards(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_131
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetPYYPremiumGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("config not found")
	}

	data := s.getData()
	count := s.getCount()
	var canRecIds []uint32
	for _, aConf := range conf.AccRewards {
		if aConf.Count > count {
			continue
		}
		if utils.SliceContainsUint32(data.IdL, aConf.Count) {
			continue
		}
		canRecIds = append(canRecIds, aConf.Count)
	}

	if len(canRecIds) == 0 {
		return neterror.ParamsInvalidError("not rewards can rec")
	}

	var rewards jsondata.StdRewardVec
	for _, id := range canRecIds {
		aConf := jsondata.GetPYYPremiumGiftAccRewards(s.ConfName, s.ConfIdx, id)
		if aConf != nil {
			rewards = append(rewards, aConf.Rewards...)
		}
	}

	data.IdL = append(data.IdL, canRecIds...)
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPremiumGiftAccRewards})
	}
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPremiumGiftAccRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%v", canRecIds),
	})
	s.SendProto3(127, 131, &pb3.S2C_127_131{ActId: s.GetId(), IdL: canRecIds})
	return nil
}

func (s *PremiumGiftSys) chargeCheck(chargeId uint32) bool {
	gConf := jsondata.GetPYYPremiumGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	data := s.getData()
	count := data.BuyData[gConf.GiftId]
	if count >= gConf.CountLimit {
		return false
	}

	return true
}

func (s *PremiumGiftSys) chargeBack(chargeId uint32) bool {
	gConf := jsondata.GetPYYPremiumGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	// 设置奖励已发
	data := s.getData()
	data.BuyData[gConf.GiftId] += 1
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPremiumGiftPayRewards})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPremiumGiftPayRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", gConf.GiftId),
	})
	awards := engine.FilterRewardByPlayer(s.GetPlayer(), gConf.Rewards)
	s.SendProto3(127, 137, &pb3.S2C_127_137{Rewards: jsondata.StdRewardVecToPb3RewardVec(awards)})
	s.s2cInfo()
	return true
}

func PremiumGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYPremiumGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*PremiumGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func PremiumGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYPremiumGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*PremiumGiftSys); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYPremiumGift, func() iface.IPlayerYY {
		return &PremiumGiftSys{}
	})

	engine.RegChargeEvent(chargedef.PremiumGift, PremiumGiftCheck, PremiumGiftChargeBack)

	net.RegisterYYSysProtoV2(127, 130, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PremiumGiftSys).c2sRecDailyRewards
	})
	net.RegisterYYSysProtoV2(127, 131, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PremiumGiftSys).c2sRecAccRewards
	})
}
