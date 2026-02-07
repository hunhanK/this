/**
 * @Author: lzp
 * @Date: 2024/8/2
 * @Desc: 砍价狂欢
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type PlayerYYBargainPrice struct {
	PlayerYYBase
}

func (s *PlayerYYBargainPrice) OnReconnect() {
	s.S2CInfo()
}

func (s *PlayerYYBargainPrice) Login() {
	s.S2CInfo()
}

func (s *PlayerYYBargainPrice) OnOpen() {
	s.S2CInfo()
}

func (s *PlayerYYBargainPrice) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.BargainPrice == nil {
		return
	}
	delete(state.BargainPrice, s.Id)
}

func (s *PlayerYYBargainPrice) S2CInfo() {
	data := s.GetData()
	s.SendProto3(127, 100, &pb3.S2C_127_100{
		ActId: s.GetId(),
		Data: &pb3.PYY_BargainPrice{
			KillReduce: data.KillReduce,
			IsBuy:      data.IsBuy,
			IsRec:      data.IsRec,
		},
	})
}

func (s *PlayerYYBargainPrice) GetData() *pb3.PYY_BargainPrice {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if state.BargainPrice == nil {
		state.BargainPrice = make(map[uint32]*pb3.PYY_BargainPrice)
	}
	if state.BargainPrice[s.Id] == nil {
		state.BargainPrice[s.Id] = &pb3.PYY_BargainPrice{}
	}
	return state.BargainPrice[s.Id]
}

func (s *PlayerYYBargainPrice) PlayerKillBoss(monId, fbId uint32) {
	conf := jsondata.GetYYBargainPriceConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	if utils.SliceContainsUint32(conf.NotCalc, fbId) {
		return
	}

	mConf := jsondata.GetMonsterConf(monId)
	if mConf.Type != custom_id.MtBoss {
		return
	}

	data := s.GetData()
	if data.IsBuy || data.IsRec {
		return
	}

	// 已经到最低价
	if conf.OriginalPrice-data.KillReduce <= conf.LowestPrice {
		return
	}

	if data.KillReduce+conf.KillReduce > conf.OriginalPrice {
		data.KillReduce = conf.OriginalPrice
	} else {
		data.KillReduce += conf.KillReduce
	}

	if conf.LowestPrice == conf.OriginalPrice-data.KillReduce && 0 == conf.LowestPrice {
		data.IsBuy = true
	}

	s.S2CInfo()
}

func (s *PlayerYYBargainPrice) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetYYBargainPriceConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}

	data := s.GetData()
	if conf.LowestPrice != conf.OriginalPrice-data.KillReduce {
		return false
	}

	if conf.ChargeId != chargeConf.ChargeId {
		return false
	}

	return true
}

func (s *PlayerYYBargainPrice) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	data := s.GetData()
	data.IsBuy = true
	s.S2CInfo()
	return true
}

func (s *PlayerYYBargainPrice) c2sBargainPriceGet(_ *base.Message) error {
	conf := jsondata.GetYYBargainPriceConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("%s config not found", s.GetPrefix())
	}

	data := s.GetData()
	if !data.IsBuy || data.IsRec {
		return neterror.ConfNotFoundError("rewards can not get")
	}

	data.IsRec = true
	if len(conf.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYBargainPriceGetRewards})
	}

	s.S2CInfo()
	return nil
}

func bargainPriceCheck(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}
	monsterId, ok := args[0].(uint32)
	if !ok {
		return
	}

	fbId, ok := args[3].(uint32)
	if !ok {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYBargainPrice)
	if nil == yyList {
		return
	}
	for _, v := range yyList {
		sys, ok := v.(*PlayerYYBargainPrice)
		if !ok || !sys.IsOpen() {
			continue
		}
		sys.PlayerKillBoss(monsterId, fbId)
	}
}

func bargainGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYBargainPrice)
	for _, obj := range yyList {
		if s, ok := obj.(*PlayerYYBargainPrice); ok && s.IsOpen() {
			return s.chargeCheck(conf)
		}
	}
	return false
}

func bargainGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYBargainPrice)
	for _, obj := range yyList {
		if s, ok := obj.(*PlayerYYBargainPrice); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYBargainPrice, func() iface.IPlayerYY {
		return &PlayerYYBargainPrice{}
	})

	event.RegActorEvent(custom_id.AeKillMon, bargainPriceCheck)
	net.RegisterYYSysProtoV2(127, 101, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*PlayerYYBargainPrice).c2sBargainPriceGet
	})

	engine.RegChargeEvent(chargedef.BargainGift, bargainGiftCheck, bargainGiftChargeBack)
}
