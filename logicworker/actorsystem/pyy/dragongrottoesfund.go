/**
 * @Author: LvYuMeng
 * @Date: 2024/3/5
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type DragonGrottoesFundSys struct {
	PlayerYYBase
}

func (s *DragonGrottoesFundSys) GetData() *pb3.PYY_DragonGrottoesFund {
	state := s.GetYYData()
	if nil == state.DragonGrottoesFund {
		state.DragonGrottoesFund = make(map[uint32]*pb3.PYY_DragonGrottoesFund)
	}
	if state.DragonGrottoesFund[s.Id] == nil {
		state.DragonGrottoesFund[s.Id] = &pb3.PYY_DragonGrottoesFund{}
	}
	if nil == state.DragonGrottoesFund[s.Id].Rev {
		state.DragonGrottoesFund[s.Id].Rev = make(map[uint32]bool)
	}
	return state.DragonGrottoesFund[s.Id]
}

func (s *DragonGrottoesFundSys) Login() {
	s.s2cInfo()
}

func (s *DragonGrottoesFundSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DragonGrottoesFundSys) OnOpen() {
	s.s2cInfo()
}

func (s *DragonGrottoesFundSys) ResetData() {
	state := s.GetYYData()
	if nil == state.DragonGrottoesFund {
		return
	}
	delete(state.DragonGrottoesFund, s.Id)
}

func (s *DragonGrottoesFundSys) s2cInfo() {
	s.SendProto3(57, 0, &pb3.S2C_57_0{
		ActiveId: s.Id,
		Info:     s.GetData(),
	})
}

func (s *DragonGrottoesFundSys) OnEnd() {
	conf := jsondata.GetYYDragonGrottoesFundConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	if data.BuyTime <= 0 {
		return
	}

	var rewards jsondata.StdRewardVec
	for _, v := range conf.Days {
		if !data.Rev[v.Day] {
			rewards = append(rewards, v.Rewards...)
			data.Rev[v.Day] = true
		}
	}

	if len(rewards) > 0 {
		s.GetPlayer().SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_DragonGrottoesFund,
			Rewards: rewards,
		})
	}
}

func (s *DragonGrottoesFundSys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetYYDragonGrottoesFundConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	if conf.ChargeID != chargeConf.ChargeId {
		return false
	}

	data := s.GetData()
	if data.BuyTime > 0 {
		return false
	}

	nowSec := time_util.GetZeroTime(time_util.NowSec())
	startTime := time_util.GetZeroTime(s.GetOpenTime())
	days := nowSec/86400 - startTime/86400 + 1

	if days > conf.BuyDay {
		return false
	}
	return true
}

func (s *DragonGrottoesFundSys) getRecord() *pb3.DragonGrottoesFundRecords {
	staticVar := gshare.GetStaticVar()
	if nil == staticVar.DragonGrottoesFundRecords {
		staticVar.DragonGrottoesFundRecords = make(map[uint32]*pb3.DragonGrottoesFundRecords)
	}
	if nil == staticVar.DragonGrottoesFundRecords[s.Id] {
		staticVar.DragonGrottoesFundRecords[s.Id] = &pb3.DragonGrottoesFundRecords{}
	}
	recordTime := staticVar.DragonGrottoesFundRecords[s.Id].OpenTime
	if recordTime == 0 || recordTime < s.OpenTime {
		staticVar.DragonGrottoesFundRecords[s.Id] = &pb3.DragonGrottoesFundRecords{OpenTime: s.OpenTime}
	}
	return staticVar.DragonGrottoesFundRecords[s.Id]
}

func (s *DragonGrottoesFundSys) record() {
	data := s.getRecord()
	data.Records = append(data.Records, s.GetPlayer().GetName())
	if len(data.Records) > dragonGrottoesFundRecordLimit {
		data.Records = data.Records[1:]
	}
	s.SendProto3(57, 3, &pb3.S2C_57_3{
		ActiveId: s.Id,
		Records:  data.Records,
	})
}

func (s *DragonGrottoesFundSys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetYYDragonGrottoesFundConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}
	if conf.ChargeID != chargeConf.ChargeId {
		return false
	}

	data := s.GetData()
	if data.BuyTime > 0 {
		s.LogError("player %d buy funds repeated", s.GetPlayer().GetId())
		return false
	}

	data.BuyTime = time_util.NowSec()

	s.SendProto3(57, 1, &pb3.S2C_57_1{
		ActiveId: s.Id,
		BuyTime:  data.BuyTime,
	})
	s.record()
	return true
}

func (s *DragonGrottoesFundSys) c2sRevReward(msg *base.Message) error {
	var req pb3.C2S_57_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYDragonGrottoesFundConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("dragon grottoes funds conf(%d) is nil", s.ConfIdx)
	}

	day := req.Id
	if nil == conf.Days[day-1] {
		return neterror.ConfNotFoundError("dragon grottoes funds day(%d)  reward is nil", day)
	}

	data := s.GetData()
	if data.BuyTime <= 0 {
		return neterror.ParamsInvalidError("dragon grottoes funds not buy confIdx(%d)", s.ConfIdx)
	}

	buyDays := (time_util.GetZeroTime(time_util.NowSec())-time_util.GetZeroTime(data.BuyTime))/86400 + 1
	if day > buyDays {
		return neterror.ParamsInvalidError("dragon grottoes funds not rev day")
	}

	if data.Rev[day] {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	data.Rev[req.Id] = true

	engine.GiveRewards(s.GetPlayer(), conf.Days[day-1].Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogDragonGrottoesFund,
	})

	s.SendProto3(57, 2, &pb3.S2C_57_2{
		ActiveId: s.Id,
		Id:       day,
	})

	return nil
}

const dragonGrottoesFundRecordLimit = 100

func (s *DragonGrottoesFundSys) c2sRecord(msg *base.Message) error {
	s.SendProto3(57, 3, &pb3.S2C_57_3{
		ActiveId: s.Id,
		Records:  s.getRecord().Records,
	})
	return nil
}

func dragonGrottoesFundChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYDragonGrottoesFund)
	for _, obj := range yyList {
		if s, ok := obj.(*DragonGrottoesFundSys); ok && s.IsOpen() {
			if s.chargeCheck(conf) {
				return true
			}
		}
	}
	return false
}

func dragonGrottoesFundChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYDragonGrottoesFund)
	for _, obj := range yyList {
		if s, ok := obj.(*DragonGrottoesFundSys); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDragonGrottoesFund, func() iface.IPlayerYY {
		return &DragonGrottoesFundSys{}
	})

	net.RegisterYYSysProtoV2(57, 2, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonGrottoesFundSys).c2sRevReward
	})
	net.RegisterYYSysProtoV2(57, 3, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DragonGrottoesFundSys).c2sRecord
	})

	engine.RegChargeEvent(chargedef.DragonGrottoesFund, dragonGrottoesFundChargeCheck, dragonGrottoesFundChargeBack)

}
