/**
 * @Author: LvYuMeng
 * @Date: 2024/4/19
 * @Desc:
**/

package pyy

import (
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

type YYGameNotice struct {
	PlayerYYBase
}

func (s *YYGameNotice) Login() {
	s.s2cInfo()
}

func (s *YYGameNotice) OnReconnect() {
	s.s2cInfo()
}

func (s *YYGameNotice) OnOpen() {
	s.s2cInfo()
}

func (s *YYGameNotice) ResetData() {
	state := s.GetYYData()
	if nil == state.GameNotice {
		return
	}
	delete(state.GameNotice, s.Id)
}

func (s *YYGameNotice) GetData() *pb3.PYY_GameNotice {
	state := s.GetYYData()
	if nil == state.GameNotice {
		state.GameNotice = make(map[uint32]*pb3.PYY_GameNotice)
	}
	if nil == state.GameNotice[s.Id] {
		state.GameNotice[s.Id] = &pb3.PYY_GameNotice{}
	}
	return state.GameNotice[s.Id]
}

func (s *YYGameNotice) s2cInfo() {
	s.GetPlayer().SendProto3(61, 0, &pb3.S2C_61_0{
		ActiveId: s.GetId(),
		Notice:   s.GetData(),
	})
}

func (s *YYGameNotice) c2sRev(msg *base.Message) error {
	var req pb3.C2S_61_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.GetData()
	if data.IsRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	conf := jsondata.GetYYGameNoticeConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("no game notice conf")
	}

	data.IsRev = true
	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogGameNoticeAward,
		})
	}

	s.s2cInfo()

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYGameNotice, func() iface.IPlayerYY {
		return &YYGameNotice{}
	})

	net.RegisterYYSysProtoV2(61, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*YYGameNotice).c2sRev
	})

}
