/**
 * @Author: LvYuMeng
 * @Date: 2025/1/14
 * @Desc: 春节签到
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type SpringFestivalSignSys struct {
	PlayerYYBase
}

func (s *SpringFestivalSignSys) OnEnd() {
	s.sendDaySignMail()
	s.sendTotalSignMail()
}

func (s *SpringFestivalSignSys) sendTotalSignMail() {
	conf := jsondata.GetPYYSpringFestivalSignConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	data := s.getData()
	totalSignDay := s.getTotalSignTimes()

	var rewardsVec []jsondata.StdRewardVec
	for _, v := range conf.TotalSignConf {
		if totalSignDay < v.Times {
			continue
		}
		if pie.Uint32s(data.TotalSignRevIds).Contains(v.Times) {
			continue
		}
		data.TotalSignRevIds = append(data.TotalSignRevIds, v.Times)
		rewardsVec = append(rewardsVec, v.Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  conf.TotalSignMailId,
			Rewards: rewards,
		})
	}
}

func (s *SpringFestivalSignSys) sendDaySignMail() {
	conf := jsondata.GetPYYSpringFestivalSignConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	hasPrivilege, _ := s.GetPlayer().GetPrivilege(privilegedef.PrivilegeType(conf.PrivilegeType))

	var rewardsVec []jsondata.StdRewardVec
	for _, v := range conf.DayConf {
		dayInfo, ok := s.getSignDayInfo(v.Day)
		if !ok || !dayInfo.CanRev {
			continue
		}
		if hasPrivilege > 0 && !utils.IsSetBit(dayInfo.RevFlag, SpringFestivalSignRevPrivilege) {
			dayInfo.RevFlag = utils.SetBit(dayInfo.RevFlag, SpringFestivalSignRevPrivilege)
			rewardsVec = append(rewardsVec, v.PrivilegeRewards)
		}
		if !utils.IsSetBit(dayInfo.RevFlag, SpringFestivalSignRevNormal) {
			dayInfo.RevFlag = utils.SetBit(dayInfo.RevFlag, SpringFestivalSignRevNormal)
			rewardsVec = append(rewardsVec, v.SignRewards)
		}
	}

	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  conf.DaySignMailId,
			Rewards: rewards,
		})
	}
}

func (s *SpringFestivalSignSys) Login() {
	s.signDay(s.GetOpenDay(), false)
	s.s2cInfo()
}

func (s *SpringFestivalSignSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SpringFestivalSignSys) s2cInfo() {
	s.SendProto3(75, 0, &pb3.S2C_75_0{
		ActiveId: s.GetId(),
		Data:     s.getData(),
	})
}

func (s *SpringFestivalSignSys) getData() *pb3.PYY_SpringFestivalSign {
	state := s.GetYYData()
	if nil == state.SpringFestivalSign {
		state.SpringFestivalSign = make(map[uint32]*pb3.PYY_SpringFestivalSign)
	}
	if state.SpringFestivalSign[s.Id] == nil {
		state.SpringFestivalSign[s.Id] = &pb3.PYY_SpringFestivalSign{}
	}
	if nil == state.SpringFestivalSign[s.Id].SignInfo {
		state.SpringFestivalSign[s.Id].SignInfo = make(map[uint32]*pb3.SpringFestivalSign)
	}
	return state.SpringFestivalSign[s.Id]
}

func (s *SpringFestivalSignSys) ResetData() {
	state := s.GetYYData()
	if nil == state.SpringFestivalSign {
		return
	}
	delete(state.SpringFestivalSign, s.GetId())
}

func (s *SpringFestivalSignSys) OnOpen() {
	s.signDay(s.GetOpenDay(), false)
	s.s2cInfo()
}

func (s *SpringFestivalSignSys) NewDay() {
	s.signDay(s.GetOpenDay(), true)
}

func (s *SpringFestivalSignSys) getSignDayInfo(day uint32) (*pb3.SpringFestivalSign, bool) {
	data := s.getData()

	signInfo, ok := data.SignInfo[day]

	return signInfo, ok
}

func (s *SpringFestivalSignSys) signDay(day uint32, bro bool) {
	if day > s.GetOpenDay() || day == 0 {
		return
	}

	data := s.getData()

	if _, ok := data.SignInfo[day]; !ok {
		data.SignInfo[day] = &pb3.SpringFestivalSign{}
	}

	dayInfo := data.SignInfo[day]

	if dayInfo.CanRev {
		return
	}

	dayInfo.CanRev = true

	if bro {
		s.sendDayInfo(day)
	}
}

func (s *SpringFestivalSignSys) sendDayInfo(day uint32) {
	s.SendProto3(75, 1, &pb3.S2C_75_1{
		ActiveId: s.GetId(),
		Day:      day,
		Sign:     s.getData().SignInfo[day],
	})
}

const (
	SpringFestivalSignRevNormal    uint32 = 0
	SpringFestivalSignRevPrivilege uint32 = 1
)

func (s *SpringFestivalSignSys) c2sSignRev(msg *base.Message) error {
	var req pb3.C2S_75_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	bit := utils.Ternary(req.IsPrivilege, SpringFestivalSignRevPrivilege, SpringFestivalSignRevNormal).(uint32)

	s.revSignRewards(req.Day, bit)

	s.sendDayInfo(req.Day)
	return nil
}

func (s *SpringFestivalSignSys) revSignRewards(day, bit uint32) bool {
	conf := jsondata.GetPYYSpringFestivalSignConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}

	dayConf, ok := conf.GetDayConf(day)
	if !ok {
		return false
	}

	if s.GetOpenDay() < day {
		return false
	}

	dayInfo, ok := s.getSignDayInfo(day)
	if !ok {
		return false
	}

	if !dayInfo.CanRev {
		return false
	}
	if utils.IsSetBit(dayInfo.RevFlag, bit) {
		return false
	}

	var rewards jsondata.StdRewardVec
	switch bit {
	case SpringFestivalSignRevPrivilege:
		hasPrivilege, _ := s.GetPlayer().GetPrivilege(privilegedef.PrivilegeType(conf.PrivilegeType))
		if hasPrivilege <= 0 {
			return false
		}
		rewards = dayConf.PrivilegeRewards
	case SpringFestivalSignRevNormal:
		rewards = dayConf.SignRewards
	default:
		return false
	}

	dayInfo.RevFlag = utils.SetBit(dayInfo.RevFlag, bit)

	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYSpringFestivalTotalSignAwards})
	return true
}

func (s *SpringFestivalSignSys) c2sReSign(msg *base.Message) error {
	var req pb3.C2S_75_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetPYYSpringFestivalSignConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("PYYSpringFestivalSignConf conf is nil")
	}

	_, ok := conf.GetDayConf(req.Day)
	if !ok {
		return neterror.ConfNotFoundError("day %d conf is nil", req.Day)
	}

	if s.GetOpenDay() < req.Day {
		return neterror.ParamsInvalidError("day %d not open", req.Day)
	}

	dayInfo, ok := s.getSignDayInfo(req.Day)
	if ok && dayInfo.CanRev {
		return neterror.ParamsInvalidError("is sign")
	}

	if !s.GetPlayer().ConsumeByConf(conf.Consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogPYYSpringFestivalReSignConsume,
	}) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.signDay(req.Day, false)

	s.revSignRewards(req.Day, SpringFestivalSignRevNormal)
	s.revSignRewards(req.Day, SpringFestivalSignRevPrivilege)

	s.sendDayInfo(req.Day)
	return nil
}

func (s *SpringFestivalSignSys) getTotalSignTimes() uint32 {
	data := s.getData()
	var totalSignDay uint32

	for _, v := range data.SignInfo {
		if v.CanRev && v.RevFlag > 0 {
			totalSignDay++
		}
	}

	return totalSignDay
}

func (s *SpringFestivalSignSys) c2sTotalSignRev(msg *base.Message) error {
	var req pb3.C2S_75_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	data := s.getData()
	if pie.Uint32s(data.TotalSignRevIds).Contains(req.Day) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	totalSignDay := s.getTotalSignTimes()

	tsConf, ok := jsondata.GetPYYSpringFestivalSignConf(s.ConfName, s.ConfIdx).GetTotalSignConf(req.Day)
	if !ok {
		return neterror.ConfNotFoundError("total sign conf %d is nil", req.Day)
	}

	if tsConf.Times > totalSignDay {
		return neterror.ParamsInvalidError("total sign day %d not enough", tsConf.Times)
	}

	data.TotalSignRevIds = append(data.TotalSignRevIds, req.Day)

	s.SendProto3(75, 3, &pb3.S2C_75_3{
		ActiveId: s.GetId(),
		Day:      req.GetDay(),
	})
	engine.GiveRewards(s.GetPlayer(), tsConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYSpringFestivalTotalSignAwards})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSpringFestivalSign, func() iface.IPlayerYY {
		return &SpringFestivalSignSys{}
	})

	net.RegisterYYSysProtoV2(75, 1, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SpringFestivalSignSys).c2sSignRev
	})
	net.RegisterYYSysProtoV2(75, 2, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SpringFestivalSignSys).c2sReSign
	})
	net.RegisterYYSysProtoV2(75, 3, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SpringFestivalSignSys).c2sTotalSignRev
	})
}
