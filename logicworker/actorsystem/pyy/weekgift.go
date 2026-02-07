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
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type WeekGiftSys struct {
	PlayerYYBase
}

func (s *WeekGiftSys) GetData() *pb3.PYY_WeekGift {
	state := s.GetYYData()
	if nil == state.WeekGift {
		state.WeekGift = make(map[uint32]*pb3.PYY_WeekGift)
	}
	if state.WeekGift[s.Id] == nil {
		state.WeekGift[s.Id] = &pb3.PYY_WeekGift{}
	}
	return state.WeekGift[s.Id]
}

func (s *WeekGiftSys) Login() {
	s.s2cInfo()
}

func (s *WeekGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WeekGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *WeekGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.WeekGift {
		return
	}
	delete(state.WeekGift, s.Id)
}

func (s *WeekGiftSys) s2cInfo() {
	s.SendProto3(62, 0, &pb3.S2C_62_0{ActiveId: s.GetId(), WeekGift: s.GetData()})
}

func (s *WeekGiftSys) c2sRev(_ *base.Message) error {
	weekDay := uint32(time_util.Weekday())
	data := s.GetData()
	if utils.IsSetBit(data.WeekFlag, weekDay) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetYYWeekGiftAwardConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("week gift conf is nil")
	}

	var giftConf *jsondata.WeekGiftRewardConf
	for _, v := range conf.Gift {
		if v.Week == weekDay {
			giftConf = v
			break
		}
	}

	if nil == giftConf {
		return neterror.ConfNotFoundError("week gift conf is nil")
	}

	data.WeekFlag = utils.SetBit(data.WeekFlag, weekDay)

	if len(giftConf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), giftConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogWeekGiftAward})
	}

	s.s2cInfo()

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYWeekGift, func() iface.IPlayerYY {
		return &WeekGiftSys{}
	})

	net.RegisterYYSysProtoV2(62, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*WeekGiftSys).c2sRev
	})

}
