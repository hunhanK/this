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

type RegressGiftSys struct {
	PlayerYYBase
}

func (s *RegressGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *RegressGiftSys) Login() {
	s.s2cInfo()
}

func (s *RegressGiftSys) OnOpen() {
	s.refreshRegressGift()
	s.s2cInfo()
}

func (s *RegressGiftSys) NewDay() {
	data := s.getData()
	data.IsRecDailyRewards = false
	s.refreshRegressGift()
	s.s2cInfo()
}

func (s *RegressGiftSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.ThurGift == nil {
		return
	}
	delete(state.RegressGift, s.Id)
}

func (s *RegressGiftSys) s2cInfo() {
	s.SendProto3(127, 125, &pb3.S2C_127_125{
		ActId: s.GetId(),
		Data:  s.getData(),
	})
}

func (s *RegressGiftSys) refreshRegressGift() {
	data := s.getData()

	conf := jsondata.GetPYYRegressGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	day := s.GetOpenDay()
	lv := s.GetPlayer().GetLevel()

	checkLvRange := func(minLv, maxLv uint32) bool {
		if lv >= minLv && lv <= maxLv {
			return true
		}
		return false
	}
	for _, gConf := range conf.Gifts {
		isLv := true
		if len(gConf.LvRange) >= 2 {
			isLv = checkLvRange(gConf.LvRange[0], gConf.LvRange[1])
		}

		if day == gConf.Day && isLv {
			data.GiftIds = append(data.GiftIds, gConf.GiftId)
		}
	}
}

func (s *RegressGiftSys) getData() *pb3.PYY_RegressGift {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.RegressGift == nil {
		state.RegressGift = make(map[uint32]*pb3.PYY_RegressGift)
	}
	if state.RegressGift[s.Id] == nil {
		state.RegressGift[s.Id] = &pb3.PYY_RegressGift{}
	}
	data := state.RegressGift[s.Id]
	if data.BuyData == nil {
		data.BuyData = make(map[uint32]uint32)
	}
	return data
}

func (s *RegressGiftSys) c2sPurchase(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}

	var req pb3.C2S_127_126
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	gConf := jsondata.GetPYYRegressGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil {
		return neterror.ParamsInvalidError("giftId: %d not found config", req.GiftId)
	}

	data := s.getData()
	if !utils.SliceContainsUint32(data.GiftIds, req.GiftId) {
		return neterror.ParamsInvalidError("giftId: %d cannot buy", req.GiftId)
	}

	count, ok := data.BuyData[req.GiftId]
	if ok && count >= gConf.CountLimit {
		return neterror.ParamsInvalidError("giftId: %d buy limit", req.GiftId)
	}

	data.GiftId = req.GiftId
	s.SendProto3(127, 126, &pb3.S2C_127_126{ActId: s.GetId(), GiftId: req.GiftId})
	return nil
}

func (s *RegressGiftSys) c2sRecDailyRewards(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}
	var req pb3.C2S_127_127
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetPYYRegressGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("config not found")
	}

	data := s.getData()
	if data.IsRecDailyRewards {
		return neterror.ParamsInvalidError("has receive daily rewards")
	}

	data.IsRecDailyRewards = true
	if len(conf.DayRewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.DayRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRegressGiftDailyRewards})
	}
	s.SendProto3(127, 127, &pb3.S2C_127_127{ActId: s.GetId(), IsRec: true})
	return nil
}

func (s *RegressGiftSys) chargeCheck(chargeId uint32) bool {
	data := s.getData()
	if data.GiftId == 0 {
		return false
	}

	gConf := jsondata.GetPYYRegressGiftByGiftId(s.ConfName, s.ConfIdx, data.GiftId)
	if gConf == nil || gConf.ChargeId != chargeId {
		return false
	}

	count := data.BuyData[data.GiftId]
	if count >= gConf.CountLimit {
		return false
	}

	return true
}

func (s *RegressGiftSys) chargeBack() bool {
	data := s.getData()
	giftId := data.GiftId
	gConf := jsondata.GetPYYRegressGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return false
	}

	// 设置奖励已发
	data.GiftId = 0
	data.BuyData[giftId] += 1
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogRegressGiftPayRewards})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogRegressGiftPayRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", gConf.GiftId),
	})
	s.s2cInfo()
	return true
}

func regressGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYRegressGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*RegressGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func regressGiftChargeBack(player iface.IPlayer, _ *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYRegressGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*RegressGiftSys); ok && s.IsOpen() {
			if s.chargeBack() {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYRegressGift, func() iface.IPlayerYY {
		return &RegressGiftSys{}
	})

	engine.RegChargeEvent(chargedef.RegressGift, regressGiftCheck, regressGiftChargeBack)

	net.RegisterYYSysProtoV2(127, 126, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*RegressGiftSys).c2sPurchase
	})
	net.RegisterYYSysProtoV2(127, 127, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*RegressGiftSys).c2sRecDailyRewards
	})
}
