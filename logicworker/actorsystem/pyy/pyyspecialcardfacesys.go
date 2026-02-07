/**
 * @Author:
 * @Date:
 * @Desc: 特惠卡面
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/random"
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

type SpecialCardFaceSys struct {
	PlayerYYBase
}

func (s *SpecialCardFaceSys) s2cInfo() {
	s.SendProto3(8, 170, &pb3.S2C_8_170{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *SpecialCardFaceSys) getData() *pb3.PYYSpecialCardFaceData {
	state := s.GetYYData()
	if nil == state.SpecialCardFaceData {
		state.SpecialCardFaceData = make(map[uint32]*pb3.PYYSpecialCardFaceData)
	}
	if state.SpecialCardFaceData[s.Id] == nil {
		state.SpecialCardFaceData[s.Id] = &pb3.PYYSpecialCardFaceData{}
	}
	if state.SpecialCardFaceData[s.Id].ChargeNumMap == nil {
		state.SpecialCardFaceData[s.Id].ChargeNumMap = make(map[uint32]uint32)
	}
	return state.SpecialCardFaceData[s.Id]
}

func (s *SpecialCardFaceSys) ResetData() {
	state := s.GetYYData()
	if nil == state.SpecialCardFaceData {
		return
	}
	delete(state.SpecialCardFaceData, s.Id)
}

func (s *SpecialCardFaceSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SpecialCardFaceSys) Login() {
	s.s2cInfo()
}

func (s *SpecialCardFaceSys) OnOpen() {
	s.s2cInfo()
}

func (s *SpecialCardFaceSys) NewDay() {
	config := jsondata.GetSpecialCardFaceConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return
	}
	data := s.getData()
	for _, v := range config.Gift {
		if v.DailyReset {
			delete(data.ChargeNumMap, v.ChargeId)
		}
	}
	s.s2cInfo()
}

func checkSpecialCardFaceHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var ret bool
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYSpecialCardFace, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*SpecialCardFaceSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		data := sys.getData()
		config := jsondata.GetSpecialCardFaceConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		giftConf, ok := config.Gift[conf.ChargeId]
		if !ok {
			return
		}
		if data.ChargeNumMap[conf.ChargeId] >= giftConf.Count {
			return
		}
		ret = true
		return
	})
	return ret
}

func specialCardFaceChargeHandler(actor iface.IPlayer, chargeConf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	var ret bool
	if !checkSpecialCardFaceHandler(actor, chargeConf) {
		return false
	}
	pyymgr.EachPlayerAllYYObj(actor, yydefine.PYYSpecialCardFace, func(obj iface.IPlayerYY) {
		sys, ok := obj.(*SpecialCardFaceSys)
		if !ok {
			return
		}
		if ret {
			return
		}
		data := sys.getData()
		config := jsondata.GetSpecialCardFaceConfig(sys.ConfName, sys.ConfIdx)
		if config == nil {
			return
		}
		giftConf, ok := config.Gift[chargeConf.ChargeId]
		if !ok {
			return
		}
		if data.ChargeNumMap[chargeConf.ChargeId] >= giftConf.Count {
			return
		}
		var randomPool = new(random.Pool)
		copyStdRewardVec := jsondata.CopyStdRewardVec(giftConf.Rewards)
		var giveAwards jsondata.StdRewardVec
		for _, v := range copyStdRewardVec {
			if v.Weight != 0 {
				randomPool.AddItem(v, v.Weight)
				continue
			}
			giveAwards = append(giveAwards, v)
		}
		if randomPool.Size() != 0 {
			giveAwards = append(giveAwards, randomPool.RandomOne().(*jsondata.StdReward))
		}
		if len(giveAwards) != 0 {
			engine.GiveRewards(actor, giveAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSpecialCardFaceGift})
		}
		actor.SendShowRewardsPopByPYY(giveAwards, sys.GetId())
		data.ChargeNumMap[chargeConf.ChargeId]++
		sys.SendProto3(8, 171, &pb3.S2C_8_171{
			ActiveId: sys.Id,
			ChargeId: chargeConf.ChargeId,
			Count:    data.ChargeNumMap[chargeConf.ChargeId],
		})

		logworker.LogPlayerBehavior(actor, pb3.LogId_LogSpecialCardFaceGift, &pb3.LogPlayerCounter{
			NumArgs: uint64(sys.GetId()),
			StrArgs: fmt.Sprintf("%d", chargeConf.ChargeId),
		})
		ret = true
		return
	})
	return ret
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSpecialCardFace, func() iface.IPlayerYY {
		return &SpecialCardFaceSys{}
	})
	engine.RegChargeEvent(chargedef.SpecialCardFace, checkSpecialCardFaceHandler, specialCardFaceChargeHandler)
}
