/**
 * @Author: LvYuMeng
 * @Date: 2024/4/25
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type DailyLoginGiftSys struct {
	PlayerYYBase
}

func (s *DailyLoginGiftSys) GetData() *pb3.PYY_DailyLoginGift {
	state := s.GetYYData()
	if nil == state.DailyLoginGift {
		state.DailyLoginGift = make(map[uint32]*pb3.PYY_DailyLoginGift)
	}
	if state.DailyLoginGift[s.Id] == nil {
		state.DailyLoginGift[s.Id] = &pb3.PYY_DailyLoginGift{}
	}
	state.DailyLoginGift[s.Id].OpenDay = s.GetOpenDay()
	return state.DailyLoginGift[s.Id]
}

func (s *DailyLoginGiftSys) Login() {
	s.s2cInfo()
}

func (s *DailyLoginGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DailyLoginGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *DailyLoginGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.DailyLoginGift {
		return
	}
	delete(state.DailyLoginGift, s.Id)
}

func (s *DailyLoginGiftSys) s2cInfo() {
	s.SendProto3(62, 10, &pb3.S2C_62_10{ActiveId: s.GetId(), DailyLoginGift: s.GetData()})
}

func (s *DailyLoginGiftSys) c2sRev(_ *base.Message) error {
	day := s.GetOpenDay()
	data := s.GetData()
	if utils.IsSetBit(data.DayFlag, day) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetPYYDailyLoginConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("daily login gift conf is nil")
	}

	var giftConf *jsondata.DailyLoginGiftRewardConf
	for _, v := range conf.Gift {
		if v.Day == day {
			giftConf = v
			break
		}
	}

	if nil == giftConf {
		return neterror.ConfNotFoundError("daily login gift conf is nil")
	}

	data.DayFlag = utils.SetBit(data.DayFlag, day)

	if len(giftConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), giftConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyLoginGiftAward})
	}

	s.s2cInfo()

	return nil
}

func (s *DailyLoginGiftSys) NewDay() {
	if !s.IsOpen() {
		return
	}
	s.s2cInfo()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDailyLoginGift, func() iface.IPlayerYY {
		return &DailyLoginGiftSys{}
	})

	net.RegisterYYSysProtoV2(62, 10, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DailyLoginGiftSys).c2sRev
	})

}
