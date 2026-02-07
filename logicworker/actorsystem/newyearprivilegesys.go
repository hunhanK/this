/**
 * @Author: LvYuMeng
 * @Date: 2025/12/25
 * @Desc: 周年特权
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
	"time"
)

type NewYearPrivilegeSys struct {
	Base
}

func (s *NewYearPrivilegeSys) getData() *pb3.NewYearPrivilege {
	binary := s.GetBinaryData()
	if binary.NewYearPrivilege == nil {
		binary.NewYearPrivilege = &pb3.NewYearPrivilege{}
	}
	return binary.NewYearPrivilege
}

func (s *NewYearPrivilegeSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_37_41
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}
	data := s.getData()

	if data.ActiveTime > 0 {
		return neterror.ParamsInvalidError("has active")
	}

	conf := jsondata.GetNewYearPrivilegeConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	if !s.owner.ConsumeByConf(conf.ActiveItem, false, common.ConsumeParams{LogId: pb3.LogId_LogNewYearPrivilegeActiveConsume}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data.ActiveTime = time_util.NowSec()
	s.SendProto3(37, 41, &pb3.S2C_37_41{ActiveTime: data.ActiveTime})
	s.owner.SetExtraAttr(attrdef.NewYearPrivilege, 1)
	s.ResetSysAttr(attrdef.SaNewYearPrivilege)
	return nil
}

func (s *NewYearPrivilegeSys) c2sChallenge(msg *base.Message) error {
	var req pb3.C2S_37_42
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}
	data := s.getData()
	if data.ActiveTime == 0 {
		return neterror.ParamsInvalidError("not active")
	}
	if data.CompleteFbTimes > 0 {
		return neterror.ParamsInvalidError("has pass")
	}
	err := s.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterNewYearPrivilege, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *NewYearPrivilegeSys) c2sMonthAward(msg *base.Message) error {
	var req pb3.C2S_37_44
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}
	data := s.getData()
	if data.ActiveTime == 0 {
		return neterror.ParamsInvalidError("not active")
	}
	conf := jsondata.GetNewYearPrivilegeConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}
	if int(conf.MonthAwardDay) != time_util.Now().Day() {
		return neterror.ParamsInvalidError("not MonthAwardDay")
	}
	nowSec := time_util.NowSec()
	if time_util.IsSameDay(data.MonthRecTimestamp, nowSec) {
		return neterror.ParamsInvalidError("already rec MonthAward")
	}
	if conf.MonthAwardTimes <= data.MonthRecTimes {
		return neterror.ParamsInvalidError("times limit")
	}
	data.MonthRecTimestamp = nowSec
	data.MonthRecTimes++
	engine.GiveRewards(s.owner, conf.MonthAward, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogNewYearPrivilegeMonthAward,
	})
	s.SendProto3(37, 44, &pb3.S2C_37_44{
		MonthRecTimestamp: data.MonthRecTimestamp,
		MonthRecTimes:     data.MonthRecTimes,
	})
	return nil
}

func (s *NewYearPrivilegeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *NewYearPrivilegeSys) OnAfterLogin() {
	data := s.getData()
	if data.ActiveTime > 0 {
		s.owner.SetExtraAttr(attrdef.NewYearPrivilege, 1)
	}
	s.s2cInfo()
}

func (s *NewYearPrivilegeSys) OnOpen() {
	s.s2cInfo()
}

func (s *NewYearPrivilegeSys) s2cInfo() {
	s.SendProto3(37, 40, &pb3.S2C_37_40{Data: s.getData()})
}

func (s *NewYearPrivilegeSys) onNewDay() {
	data := s.getData()
	if data.ActiveTime == 0 {
		return
	}
	conf := jsondata.GetNewYearPrivilegeConf()
	if nil == conf {
		return
	}
	checkMonthAwards := func() {
		now := time_util.Now()
		today := time.Unix(int64(now.Unix()), 0)

		// 如果是28号，不自动发放（玩家手动领取）
		if int(conf.MonthAwardDay) == today.Day() {
			return
		}

		// 如果没有激活时间或已无领取次数，则返回
		if data.ActiveTime == 0 || conf.MonthAwardTimes <= data.MonthRecTimes {
			return
		}

		// 获取激活时间
		activeTime := time.Unix(int64(data.ActiveTime), 0)

		// 获取玩家上次领取时间对应的日期
		var lastRecDate time.Time
		if data.MonthRecTimestamp > 0 {
			lastRecDate = time.Unix(int64(data.MonthRecTimestamp), 0)
		} else {
			// 如果从未领取，从激活时间的前一个月28号开始检查
			// 这样可以包括激活当月的28号（如果激活在28号之前）
			lastRecDate = time.Date(activeTime.Year(), activeTime.Month()-1, int(conf.MonthAwardDay), 0, 0, 0, 0, today.Location())
		}

		// 从上次领取日期的下一个月开始检查
		currentCheckMonth := lastRecDate.Month() + 1
		currentCheckYear := lastRecDate.Year()
		if currentCheckMonth > 12 {
			currentCheckMonth = 1
			currentCheckYear++
		}

		// 循环检查每个月的28号
		for {
			// 构建当前检查的28号
			checkDate := time.Date(currentCheckYear, currentCheckMonth, int(conf.MonthAwardDay), 0, 0, 0, 0, today.Location())

			// 如果检查日期在今天或之后，停止
			if !checkDate.Before(today) {
				break
			}

			// 如果检查日期在激活时间之前，跳过
			if checkDate.Before(activeTime) {
				// 移动到下一个月
				currentCheckMonth++
				if currentCheckMonth > 12 {
					currentCheckMonth = 1
					currentCheckYear++
				}
				continue
			}

			// 检查是否有领取次数
			if conf.MonthAwardTimes > data.MonthRecTimes {
				data.MonthRecTimes++
				data.MonthRecTimestamp = uint32(checkDate.Unix())

				mailmgr.SendMailToActor(s.owner.GetId(), &mailargs.SendMailSt{
					ConfId:  common.Mail_NewYearPrivilegeMonthAward,
					Rewards: conf.MonthAward,
				})
			} else {
				break
			}

			// 移动到下一个月
			currentCheckMonth++
			if currentCheckMonth > 12 {
				currentCheckMonth = 1
				currentCheckYear++
			}
		}
	}
	checkMonthAwards()

	data.CompleteFbTimes = 0
	s.s2cInfo()
}

func (s *NewYearPrivilegeSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	if data.ActiveTime == 0 {
		return
	}
	conf := jsondata.GetNewYearPrivilegeConf()
	if nil == conf {
		return
	}
	engine.CheckAddAttrsToCalc(s.owner, calc, conf.Attrs)
}

func (s *NewYearPrivilegeSys) checkout(settle *pb3.FbSettlement) {
	conf := jsondata.GetNewYearPrivilegeFbConf()
	if conf == nil {
		return
	}

	data := s.getData()

	if data.CompleteFbTimes > 0 {
		return
	}

	data.CompleteFbTimes++
	engine.GiveRewards(s.owner, conf.ChallengeAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogNewYearPrivilegeFbAward,
	})
	res := &pb3.S2C_17_254{
		Settle: settle,
	}
	res.Settle.ShowAward = jsondata.StdRewardVecToPb3RewardVec(conf.ChallengeAwards)

	s.SendProto3(17, 254, res)

	s.SendProto3(37, 43, &pb3.S2C_37_43{CompleteFbTimes: data.CompleteFbTimes})
}

func init() {
	RegisterSysClass(sysdef.SiNewYearPrivilege, func() iface.ISystem {
		return &NewYearPrivilegeSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.CheckOutNewYearPrivilegeFuBen, func(player iface.IPlayer, buf []byte) {
		if sys, ok := player.GetSysObj(sysdef.SiNewYearPrivilege).(*NewYearPrivilegeSys); ok && sys.IsOpen() {
			var req pb3.FbSettlement
			if err := pb3.Unmarshal(buf, &req); err != nil {
				player.LogError("unmarshal failed %s", err)
				return
			}
			sys.checkout(&req)
		}
	})

	engine.RegAttrCalcFn(attrdef.SaNewYearPrivilege, func(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
		if sys, ok := player.GetSysObj(sysdef.SiNewYearPrivilege).(*NewYearPrivilegeSys); ok && sys.IsOpen() {
			sys.calcAttr(calc)
		}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiNewYearPrivilege).(*NewYearPrivilegeSys); ok && sys.IsOpen() {
			sys.onNewDay()
		}
	})

	net.RegisterSysProtoV2(37, 41, sysdef.SiNewYearPrivilege, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewYearPrivilegeSys).c2sActive
	})
	net.RegisterSysProtoV2(37, 42, sysdef.SiNewYearPrivilege, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewYearPrivilegeSys).c2sChallenge
	})
	net.RegisterSysProtoV2(37, 44, sysdef.SiNewYearPrivilege, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*NewYearPrivilegeSys).c2sMonthAward
	})

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		s, ok := player.GetSysObj(sysdef.SiNewYearPrivilege).(*NewYearPrivilegeSys)
		if !ok || !s.IsOpen() {
			return
		}

		data := s.getData()
		if data.ActiveTime > 0 {
			return conf.NewYearPrivilege, nil
		}

		return
	})

}
