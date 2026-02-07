/**
 * @Author: lzp
 * @Date: 2025/5/12
 * @Desc:
**/

package pyy

import (
	"fmt"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
)

type SrvMergeGiftSys struct {
	PlayerYYBase
}

func (s *SrvMergeGiftSys) Login() {
	s.s2cInfo()
}

func (s *SrvMergeGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SrvMergeGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *SrvMergeGiftSys) NewDay() {
	data := s.GetData()
	data.Gifts = make(map[uint32]uint32)
	s.s2cInfo()
}

func (s *SrvMergeGiftSys) ResetData() {
	state := s.GetYYData()
	if state.SrvMergeGift == nil {
		return
	}
	delete(state.SrvMergeGift, s.Id)
}

func (s *SrvMergeGiftSys) GetData() *pb3.PYY_SrvMergeGift {
	state := s.GetYYData()
	if nil == state.SrvMergeGift {
		state.SrvMergeGift = make(map[uint32]*pb3.PYY_SrvMergeGift)
	}
	if state.SrvMergeGift[s.Id] == nil {
		state.SrvMergeGift[s.Id] = &pb3.PYY_SrvMergeGift{}
	}
	if nil == state.SrvMergeGift[s.Id].Gifts {
		state.SrvMergeGift[s.Id].Gifts = make(map[uint32]uint32)
	}
	return state.SrvMergeGift[s.Id]
}

func (s *SrvMergeGiftSys) s2cInfo() {
	s.SendProto3(127, 155, &pb3.S2C_127_155{
		ActId: s.Id,
		Data:  s.GetData(),
	})
}

func (s *SrvMergeGiftSys) chargeCheck(chargeId uint32) bool {
	gConf := jsondata.GetPYYSrvMergeGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	data := s.GetData()
	if data.Gifts[gConf.GiftId] >= gConf.CountLimit {
		return false
	}
	return true
}

func (s *SrvMergeGiftSys) chargeBack(chargeId uint32) bool {
	data := s.GetData()

	gConf := jsondata.GetPYYSrvMergeGiftByChargeId(s.ConfName, s.ConfIdx, chargeId)
	if gConf == nil {
		return false
	}

	data.Gifts[gConf.GiftId] += 1
	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSrvMergeGiftBuyRewards})
	}

	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogSrvMergeGiftBuyRewards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", gConf.GiftId),
	})

	s.GetPlayer().SendShowRewardsPopByPYY(gConf.Rewards, s.Id)
	s.s2cInfo()
	return true
}

func SrvMergeGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYSrvMergeGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*SrvMergeGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func SrvMergeGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYSrvMergeGift)
	for _, obj := range yyObjs {
		if s, ok := obj.(*SrvMergeGiftSys); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSrvMergeGift, func() iface.IPlayerYY {
		return &SrvMergeGiftSys{}
	})

	engine.RegChargeEvent(chargedef.SrvMergeGift, SrvMergeGiftCheck, SrvMergeGiftChargeBack)
}
