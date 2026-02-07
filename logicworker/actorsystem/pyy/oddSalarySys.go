package pyy

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type OddSalarySys struct {
	PlayerYYBase
}

func (s *OddSalarySys) ResetData() {
	yyData := s.GetYYData()
	if yyData.OddSalaryData == nil {
		return
	}
	delete(yyData.OddSalaryData, s.Id)
}

func (s *OddSalarySys) OnReconnect() {
	s.s2cInfo()
}

func (s *OddSalarySys) Login() {
	s.s2cInfo()
}

func (s *OddSalarySys) s2cInfo() {
	data := s.GetData()
	s.SendProto3(142, 3, &pb3.S2C_142_3{
		Data:     data,
		ActiveId: s.Id,
	})
}

func (s *OddSalarySys) OnOpen() {
	s.s2cInfo()
}

func (s *OddSalarySys) OnEnd() {
	s.sendReward()
}

func (s *OddSalarySys) NewDay() {
	s.sendReward()
}

func (s *OddSalarySys) sendReward() {
	data := s.GetData()
	s.autoFlip()
	//发邮件
	moneyToSend := data.Money
	data.Money = 0
	s.s2cInfo()
	if moneyToSend <= 0 {
		return
	}

	conf := jsondata.GetOddSalaryConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	var award jsondata.StdRewardVec
	reward := &jsondata.StdReward{
		Id:    jsondata.GetMoneyIdConfByType(conf.MoneyType),
		Count: int64(moneyToSend),
	}
	award = append(award, reward)
	s.GetPlayer().SendMail(&mailargs.SendMailSt{
		ConfId:  uint16(conf.Mail),
		Rewards: award,
	})

}

func (s *OddSalarySys) GetData() *pb3.PYYOddSalaryData {
	data := s.GetYYData()
	if nil == data.OddSalaryData {
		data.OddSalaryData = make(map[uint32]*pb3.PYYOddSalaryData)
	}
	if data.OddSalaryData[s.Id] == nil {
		data.OddSalaryData[s.Id] = &pb3.PYYOddSalaryData{
			Money: 0,
		}
	}
	return data.OddSalaryData[s.Id]
}

func (s *OddSalarySys) randomMultiple(flipConf *jsondata.FlipConf) uint32 {
	pool := new(random.Pool)
	weightLen := len(flipConf.Weight)

	for i := 0; i < weightLen; i += 2 {
		multiple := flipConf.Weight[i]
		weight := flipConf.Weight[i+1]

		if weight <= 0 {
			continue
		}
		pool.AddItem(multiple, weight)
	}
	if pool.Size() == 0 {
		s.LogError("pool size is zero ")
		return 0
	}
	return pool.RandomOne().(uint32)
}

func (s *OddSalarySys) autoFlip() {
	conf := jsondata.GetOddSalaryConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		s.LogError("OddSalaryConf is nil")
		return
	}

	data := s.GetData()
	beforeDayZero := time_util.GetBeforeDaysZeroTime(1)
	todayCharge := s.GetDailyChargeMoney(beforeDayZero)
	for i := 0; i < len(conf.Flip); i++ {
		flipConf := conf.GetOddSalaryWeightNext(data.Money)
		if flipConf == nil {
			break
		}
		if flipConf.Multiple != 1 && (flipConf.Multiple%10) > 0 {
			s.LogError("multiple invalid")
			break
		}
		if flipConf.Charge > todayCharge {
			break
		}

		num := s.randomMultiple(flipConf)
		digit := uint32(flipConf.Multiple)
		money := uint64(num) * uint64(digit)
		data.Money += money
	}
}

func (s *OddSalarySys) c2sFlip(msg *base.Message) error {
	var req pb3.C2S_142_4
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.GetData()
	todayCharge := s.GetDailyCharge()
	conf := jsondata.GetOddSalaryConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("conf is nil")
	}
	oddSalaryConf := conf.GetOddSalaryWeightNext(data.Money)
	if oddSalaryConf == nil {
		return neterror.ConfNotFoundError("OddSalaryConfig is nil")
	}

	if oddSalaryConf.Multiple != 1 && (oddSalaryConf.Multiple%10) > 0 {
		return neterror.InternalError("multiple invalid")
	}

	if oddSalaryConf.Multiple != uint64(req.Digit) || !jsondata.IsOneFollowedByZeros(oddSalaryConf.Multiple) {
		return neterror.ParamsInvalidError("There are cards ahead that haven't been flipped")
	}

	if oddSalaryConf.Charge > todayCharge { //金额不够
		return neterror.ParamsInvalidError("Charge not enough")
	}

	num := s.randomMultiple(oddSalaryConf)
	money := num * req.Digit
	data.Money += uint64(money)
	logworker.LogPlayerBehavior(s.player, pb3.LogId_LogOddSalaryFlip, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.Id),
	})
	s.SendProto3(142, 4, &pb3.S2C_142_4{
		Money:    money,
		Digit:    req.Digit,
		ActiveId: s.Id,
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYOddSalary, func() iface.IPlayerYY {
		return &OddSalarySys{}
	})
	net.RegisterYYSysProtoV2(142, 4, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*OddSalarySys).c2sFlip
	})
}
