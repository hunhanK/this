/**
 * @Author: lzp
 * @Date: 2024/12/12
 * @Desc:
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	GiftTpe0 = 0 // 单个礼包
	GiftTpe1 = 1 // 一键礼包
)

type ThurGiftSys struct {
	PlayerYYBase
}

func (s *ThurGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ThurGiftSys) Login() {
	s.s2cInfo()
}

func (s *ThurGiftSys) OnOpen() {
	s.initThurGift()
	s.s2cInfo()
}

func (s *ThurGiftSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.ThurGift == nil {
		return
	}
	delete(state.ThurGift, s.Id)
}

func (s *ThurGiftSys) GetData() *pb3.PYY_ThurGift {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.ThurGift == nil {
		state.ThurGift = make(map[uint32]*pb3.PYY_ThurGift)
	}
	if state.ThurGift[s.Id] == nil {
		state.ThurGift[s.Id] = &pb3.PYY_ThurGift{}
	}
	data := state.ThurGift[s.Id]
	if data.BuyData == nil {
		data.BuyData = make(map[uint32]uint32)
	}
	return data
}

func (s *ThurGiftSys) s2cInfo() {
	s.SendProto3(127, 115, &pb3.S2C_127_115{
		ActId: s.GetId(),
		Data:  s.GetData(),
	})
}

func (s *ThurGiftSys) initThurGift() {
	data := s.GetData()
	data.GiftId = 0
	data.GiftIds = data.GiftIds[:0]
	data.BuyData = make(map[uint32]uint32)

	conf := jsondata.GetPYYThurGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	openDay := gshare.GetOpenServerDay()

	checkOpenDay := func(minDay, maxDay uint32) bool {
		if openDay >= minDay && openDay <= maxDay {
			return true
		}
		return false
	}

	for _, gConf := range conf.Gifts {
		isOpenDay := true
		if len(gConf.OpenDays) >= 2 {
			isOpenDay = checkOpenDay(gConf.OpenDays[0], gConf.OpenDays[1])
		}

		if isOpenDay {
			data.GiftIds = append(data.GiftIds, gConf.GiftId)
		}
	}
}

func (s *ThurGiftSys) c2sPurchase(msg *base.Message) error {
	if !s.IsOpen() {
		return neterror.ParamsInvalidError("not open activity")
	}

	var req pb3.C2S_127_116
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	gConf := jsondata.GetPYYThurGiftByGiftId(s.ConfName, s.ConfIdx, req.GiftId)
	if gConf == nil {
		return neterror.ParamsInvalidError("giftId: %d not found config", req.GiftId)
	}

	data := s.GetData()
	if !utils.SliceContainsUint32(data.GiftIds, req.GiftId) {
		return neterror.ParamsInvalidError("giftId: %d cannot buy", req.GiftId)
	}

	count, ok := data.BuyData[req.GiftId]
	if ok && count >= gConf.CountLimit {
		return neterror.ParamsInvalidError("giftId: %d buy limit", req.GiftId)
	}

	if gConf.Type == GiftTpe1 {
		if !s.checkCanOneKeyBuy() {
			return neterror.ParamsInvalidError("giftId: %d cannot oneKey buy", req.GiftId)
		}
	}

	data.GiftId = req.GiftId
	s.SendProto3(127, 116, &pb3.S2C_127_116{ActId: s.GetId(), GiftId: req.GiftId})
	return nil
}

// 检查是否可以一键购买
// 只要有一个付费礼包购买过，就不可以付费购买
func (s *ThurGiftSys) checkCanOneKeyBuy() bool {
	data := s.GetData()
	for giftId, count := range data.BuyData {
		conf := jsondata.GetPYYThurGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
		if conf.Type == GiftTpe0 && count > 0 {
			return false
		}
	}
	return true
}

func (s *ThurGiftSys) chargeCheck(chargeId uint32) bool {
	data := s.GetData()
	if data.GiftId == 0 {
		return false
	}

	gConf := jsondata.GetPYYThurGiftByGiftId(s.ConfName, s.ConfIdx, data.GiftId)
	if gConf == nil || gConf.ChargeId != chargeId {
		return false
	}

	count := data.BuyData[data.GiftId]
	if count >= gConf.CountLimit {
		return false
	}

	if gConf.Type == GiftTpe1 {
		return s.checkCanOneKeyBuy()
	}

	return true
}

func (s *ThurGiftSys) chargeBack() bool {
	data := s.GetData()
	giftId := data.GiftId
	gConf := jsondata.GetPYYThurGiftByGiftId(s.ConfName, s.ConfIdx, giftId)
	if gConf == nil {
		return false
	}

	var rewards jsondata.StdRewardVec
	if gConf.Type == GiftTpe1 {
		for _, gId := range data.GiftIds {
			tmpConf := jsondata.GetPYYThurGiftByGiftId(s.ConfName, s.ConfIdx, gId)
			if tmpConf.Type != GiftTpe0 {
				continue
			}
			count := tmpConf.CountLimit
			data.BuyData[gId] = count
			rewards = append(rewards, jsondata.StdRewardMulti(tmpConf.FixRewards, int64(count))...)
			rewards = append(rewards, jsondata.StdRewardMulti(tmpConf.ExtraRewards, int64(count))...)
		}
	} else {
		data.BuyData[giftId] += 1
		rewards = append(rewards, gConf.FixRewards...)
		if gConf.ExtraRate > 0 && gConf.ExtraRate >= random.IntervalUU(1, 10000) {
			rewards = append(rewards, gConf.ExtraRewards...)
		}
	}

	// 设置奖励已发
	data.GiftId = 0
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogThurGiftBuyPayReward})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogThurGiftBuyPayReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", gConf.Type, giftId),
	})

	s.s2cInfo()
	return true
}

func thurGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYThurGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*ThurGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func thurGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYThurGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*ThurGiftSys); ok && s.IsOpen() {
			if s.chargeBack() {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYThurGift, func() iface.IPlayerYY {
		return &ThurGiftSys{}
	})

	engine.RegChargeEvent(chargedef.ThurGift, thurGiftCheck, thurGiftChargeBack)

	net.RegisterYYSysProtoV2(127, 116, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ThurGiftSys).c2sPurchase
	})
}
