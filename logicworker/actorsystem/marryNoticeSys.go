/**
 * @Author: LvYuMeng
 * @Date: 2024/4/25
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type MarryNoticeSys struct {
	Base
}

func (s *MarryNoticeSys) GetData() map[uint32]bool {
	if s.GetBinaryData().MarryNotice == nil {
		s.GetBinaryData().MarryNotice = make(map[uint32]bool)
	}
	return s.GetBinaryData().MarryNotice
}

func (s *MarryNoticeSys) OnOpen() {
	s.S2CInfo()
}

func (s *MarryNoticeSys) OnAfterLogin() {
	s.S2CInfo()
}

func (s *MarryNoticeSys) OnReconnect() {
	s.S2CInfo()
}

func (s *MarryNoticeSys) S2CInfo() {
	s.SendProto3(53, 65, &pb3.S2C_53_65{Rev: s.GetData()})
}

func (s *MarryNoticeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_53_66
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	level := req.Level

	conf := jsondata.GetMarryNoticeConfByLevel(level)
	if nil == conf {
		return neterror.ConfNotFoundError("marry notice conf(%d) is nil", level)
	}

	if s.owner.GetLevel() < level {
		s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	data := s.GetData()
	if data[level] {
		s.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	data[level] = true

	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.owner, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMarryNotice})
	}

	s.SendProto3(53, 66, &pb3.S2C_53_66{Level: level})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiMarryNotice, func() iface.ISystem {
		return &MarryNoticeSys{}
	})

	net.RegisterSysProtoV2(53, 66, sysdef.SiMarryNotice, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MarryNoticeSys).c2sRev
	})
}
