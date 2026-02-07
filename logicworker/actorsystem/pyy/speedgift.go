/**
 * @Author: lzp
 * @Date: 2024/8/1
 * @Desc: 加速礼包
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
)

type PlayerYYSpeedGift struct {
	PlayerYYBase
}

const (
	BuyType1 = 1 // 单独购买
	BuyType2 = 2 // 一次购买
)

func (s *PlayerYYSpeedGift) OnReconnect() {
	s.S2CInfo()
}

func (s *PlayerYYSpeedGift) Login() {
	s.S2CInfo()
}

func (s *PlayerYYSpeedGift) OnOpen() {
	s.S2CInfo()
}

func (s *PlayerYYSpeedGift) GetData() *pb3.PYY_SpeedGift {
	state := s.GetYYData()
	if state.SpeedGift == nil {
		state.SpeedGift = make(map[uint32]*pb3.PYY_SpeedGift)
	}
	if state.SpeedGift[s.Id] == nil {
		state.SpeedGift[s.Id] = &pb3.PYY_SpeedGift{}
	}
	return state.SpeedGift[s.Id]
}

func (s *PlayerYYSpeedGift) ResetData() {
	state := s.GetYYData()
	if state.SpeedGift == nil {
		return
	}
	delete(state.SpeedGift, s.Id)
}

func (s *PlayerYYSpeedGift) S2CInfo() {
	s.SendProto3(127, 90, &pb3.S2C_127_90{
		ActId: s.GetId(),
		Ids:   s.GetData().Ids,
	})
}

func (s *PlayerYYSpeedGift) NewDay() {
	if !s.IsOpen() {
		return
	}
	conf := jsondata.GetPYYSpeedGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	data := s.GetData()
	data.Ids = data.Ids[:0]
	s.S2CInfo()
}

func (s *PlayerYYSpeedGift) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetPYYSpeedGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	for _, v := range conf.Gifts {
		if v.ChargeId == chargeConf.ChargeId {
			data := s.GetData()
			if v.Type == BuyType2 && len(data.Ids) == 0 {
				return true
			} else {
				if !utils.SliceContainsUint32(data.Ids, v.Id) {
					return true
				}
			}
		}
	}
	return false
}

func (s *PlayerYYSpeedGift) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetPYYSpeedGiftConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		s.LogError("speed gift conf is nil")
		return false
	}

	var gConf *jsondata.SpeedGiftConf
	for _, v := range conf.Gifts {
		if v.ChargeId == chargeConf.ChargeId {
			gConf = v
			break
		}
	}

	if gConf == nil {
		s.LogError("speed gift(chargeId=%d) conf is nil", chargeConf.ChargeId)
		return false
	}

	data := s.GetData()

	var awards jsondata.StdRewardVec
	if gConf.Type == BuyType2 {
		for id, v := range conf.Gifts {
			if v.Type == BuyType2 {
				continue
			}
			data.Ids = append(data.Ids, id)
			awards = append(awards, v.Rewards...)
		}
	} else {
		data.Ids = append(data.Ids, gConf.Id)
		awards = append(awards, gConf.Rewards...)
	}

	if len(awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYSpeedGiftBuyRewards})
	}

	s.S2CInfo()
	return true
}

func speedGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSpeedGift)
	for _, obj := range yyList {
		if s, ok := obj.(*PlayerYYSpeedGift); ok && s.IsOpen() {
			return s.chargeCheck(conf)
		}
	}
	return false
}

func speedGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYSpeedGift)
	for _, obj := range yyList {
		if s, ok := obj.(*PlayerYYSpeedGift); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSpeedGift, func() iface.IPlayerYY {
		return &PlayerYYSpeedGift{}
	})

	engine.RegChargeEvent(chargedef.SpeedGift, speedGiftCheck, speedGiftChargeBack)
}
