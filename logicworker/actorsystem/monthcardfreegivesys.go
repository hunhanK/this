/**
 * @Author: LvYuMeng
 * @Date: 2025/6/27
 * @Desc: 月卡白送
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type MonthCardFreeGiveSys struct {
	Base
}

func (s *MonthCardFreeGiveSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *MonthCardFreeGiveSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MonthCardFreeGiveSys) s2cInfo() {
	s.SendProto3(37, 5, &pb3.S2C_37_5{Data: s.getData()})
}

func (s *MonthCardFreeGiveSys) getData() *pb3.MonthCardFreeGive {
	binary := s.GetBinaryData()
	if nil == binary.MonthCardFreeGive {
		binary.MonthCardFreeGive = &pb3.MonthCardFreeGive{}
	}
	if nil == binary.MonthCardFreeGive.Rev {
		binary.MonthCardFreeGive.Rev = map[uint32]uint32{}
	}
	return binary.MonthCardFreeGive
}

const MonthCardFreeGiveTypeMonthCard = 1

func (s *MonthCardFreeGiveSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_37_6
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	id := req.Id

	data := s.getData()
	if data.Rev[id] > 0 {
		return neterror.ParamsInvalidError("has received")
	}

	conf, ok := jsondata.GetMonthCardFreeGiveConf(id)
	if !ok {
		return neterror.ConfNotFoundError("conf is nil")
	}

	if s.owner.GetLevel() < conf.Level {
		s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	if conf.Type == MonthCardFreeGiveTypeMonthCard {
		cardsys, exist := s.owner.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !exist || !cardsys.IsOpen() {
			return neterror.SysNotExistError("card sys not open")
		}

		if err = cardsys.monthCardChargeCheck(); nil != err {
			return err
		}
	}

	data.Rev[id]++
	s.SendProto3(37, 6, &pb3.S2C_37_6{
		Id:    id,
		Count: data.Rev[id],
	})

	err = s.sendRewards(conf)
	if nil != err {
		return nil
	}

	return nil
}

func (s *MonthCardFreeGiveSys) sendRewards(conf *jsondata.MonthCardFreeGiveConfig) error {
	if conf.Type == MonthCardFreeGiveTypeMonthCard {
		cardsys, exist := s.owner.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !exist || !cardsys.IsOpen() {
			return neterror.SysNotExistError("card sys not open")
		}
		if err := cardsys.monthCardCharge(false); err != nil {
			return err
		}
	}
	return neterror.InternalError("no reg")
}

func init() {
	RegisterSysClass(sysdef.SiMonthCardFreeGive, func() iface.ISystem {
		return &MonthCardFreeGiveSys{}
	})

	net.RegisterSysProtoV2(37, 6, sysdef.SiMonthCardFreeGive, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MonthCardFreeGiveSys).c2sRev
	})
}
