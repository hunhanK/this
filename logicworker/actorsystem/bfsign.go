package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/benefitmgr"
	"jjyz/gameserver/net"
)

type BfSignSys struct {
	Base
}

func (sys *BfSignSys) OnInit() {
}

func (sys *BfSignSys) OnOpen() {
	data := sys.GetData()
	bsl := benefitmgr.GetBenefitSignLoop()
	if bsl.StartTime != data.SignStartTime {
		data.SignStartTime = bsl.StartTime
		loopId := benefitmgr.GetBenefitSignLoop().BenefitSignId
		data.SignLoopId = loopId
	}
	data.StartTime = time_util.NowSec()
	sys.S2CInfo()
}

func (sys *BfSignSys) GetData() *pb3.BenefitSignIn {
	binary := sys.GetBinaryData()
	if nil == binary.Benefit {
		binary.Benefit = &pb3.BenefitData{}
	}
	if nil == binary.Benefit.Sign {
		binary.Benefit.Sign = &pb3.BenefitSignIn{}
	}
	return binary.Benefit.Sign
}

func (sys *BfSignSys) S2CInfo() {
	sys.SendProto3(41, 11, &pb3.S2C_41_11{
		SignLoopId: benefitmgr.GetBenefitSignLoop().BenefitSignId,
		Benefit:    sys.GetData(),
	})
}

func (sys *BfSignSys) OnAfterLogin() {
	sys.S2CInfo()
}

func (sys *BfSignSys) OnReconnect() {
	sys.S2CInfo()
}

const (
	bfSignAwardType      = 1
	bfAccumulateSignType = 2
)

func (sys *BfSignSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_41_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	id := req.GetId()
	awardType := req.GetType()
	if awardType > bfAccumulateSignType {
		return neterror.ParamsInvalidError("the benefit award type(%d) is not exist", req.GetType())
	}
	var send bool
	var err error
	switch awardType {
	case bfSignAwardType:
		send, err = sys.signAward(id)
	case bfAccumulateSignType:
		send, err = sys.totalSignAward(id)
	}
	if send {
		sys.SendProto3(41, 3, &pb3.S2C_41_3{
			Type: awardType,
			Id:   id,
		})
		return nil
	}
	return err
}

func (sys *BfSignSys) GetStartDay() uint32 {
	data := sys.GetData()
	var startDay uint32
	st := time_util.GetZeroTime(data.StartTime)
	if st > data.SignStartTime {
		startDay = st/86400 - data.SignStartTime/86400 + 1
		return startDay
	}
	return 1
}

func (sys *BfSignSys) GetNowDay() uint32 {
	data := sys.GetData()
	var nowDay uint32
	st := time_util.GetDaysZeroTime(0)
	if st >= data.SignStartTime {
		nowDay = st/86400 - data.SignStartTime/86400 + 1
		return nowDay
	}
	return 0
}

func (sys *BfSignSys) getAwardDayAdd() uint32 {
	monStart := time_util.MonthStartTime(time_util.NowSec(), 0)
	nextMonStart := time_util.MonthStartTime(time_util.NowSec(), 1)
	day := nextMonStart/86400 - monStart/86400
	return 31 - day
}

func (sys *BfSignSys) signAward(signDay uint32) (bool, error) {
	loopId := benefitmgr.GetBenefitSignLoop().BenefitSignId
	loopConf := jsondata.GetBenefitSignConfById(loopId)
	if nil == loopConf {
		return false, neterror.ConfNotFoundError("benefit sign openDay(%d) is nil", gshare.GetOpenServerDay())
	}
	awardNedAdd := sys.getAwardDayAdd()
	conf := jsondata.GetBenefitSignDayConf(signDay+awardNedAdd, loopConf)
	if nil == conf {
		return false, neterror.ConfNotFoundError("benefit sign openDay(%d) conf(%d) is nil", gshare.GetOpenServerDay(), signDay)
	}
	data := sys.GetData()
	startDay := sys.GetStartDay()
	end := sys.GetNowDay()
	if end == 0 {
		return false, neterror.ParamsInvalidError("sign get err,startime:%d", data.SignStartTime)
	}
	var day, resignDay uint32
	for i := startDay; i <= end; i++ {
		if !utils.IsSetBit(data.MonthSignDay, i-1) {
			day = i
			break
		}
	}

	for i := uint32(1); i <= end; i++ {
		if !utils.IsSetBit(data.MonthSignDay, i-1) {
			if !data.IsSignToday && i == day {
				continue
			}
			resignDay = i
			break
		}
	}

	if signDay > end || signDay < 1 {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return false, nil
	}

	if utils.IsSetBit(data.MonthSignDay, signDay-1) {
		sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return false, nil
	}

	if (data.IsSignToday) || (!data.IsSignToday && day != signDay) { //补签
		if signDay != resignDay {
			return false, neterror.ParamsInvalidError("cant resign beyond first resign day")
		}
		if data.MonthResignTimes >= loopConf.ResignTimes {
			sys.owner.SendTipMsg(tipmsgid.TpReSignTimesNotEnough)
			return false, nil
		}
		cosConf := jsondata.GetBenefitReSignConf(data.MonthResignTimes+1, loopConf)
		if nil == cosConf {
			return false, neterror.ConfNotFoundError("benefit resign conf(%d) is nil", data.MonthResignTimes)
		}
		if !sys.owner.ConsumeByConf(cosConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBenefitSign}) {
			sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return false, nil
		}
		data.MonthResignTimes++
	} else if !data.IsSignToday && day == signDay {
		data.IsSignToday = true
	} else {
		return false, neterror.ParamsInvalidError("sign rev param req day(%d) error", signDay)
	}
	data.MonthSignDay = utils.SetBit(data.MonthSignDay, signDay-1)
	var addRate uint32
	vip := sys.owner.GetVipLevel()
	idx := vip
	if int(idx) >= len(conf.VipMulit) {
		idx = uint32(len(conf.VipMulit) - 1)
	}
	addRate = conf.VipMulit[idx]
	if pvcardSys, ok := sys.GetOwner().GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys); ok && pvcardSys.IsOpen() {
		if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Week) {
			addRate = utils.MaxUInt32(addRate, conf.WeekMulit)
		}
		if pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
			addRate = utils.MaxUInt32(addRate, conf.MonMulit)
		}
	}
	var reward []*jsondata.StdReward
	for _, v := range conf.Award {
		reward = append(reward, &jsondata.StdReward{
			Id:    v.Id,
			Count: v.Count * int64(1+addRate),
			Bind:  v.Bind,
			Job:   v.Job,
		})
	}
	sys.SendProto3(41, 2, &pb3.S2C_41_2{MonthSignDay: data.MonthSignDay})
	engine.GiveRewards(sys.owner, reward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitSign})
	return true, nil
}

func (sys *BfSignSys) totalSignAward(days uint32) (bool, error) {
	loopId := benefitmgr.GetBenefitSignLoop().BenefitSignId
	loopConf := jsondata.GetBenefitSignConfById(loopId)
	if nil == loopConf {
		return false, neterror.ConfNotFoundError("benefit total sign openDay(%d) is nil", gshare.GetOpenServerDay())
	}
	conf := jsondata.GetBenefitTotalSignConf(days, loopConf)
	if nil == conf {
		return false, neterror.ConfNotFoundError("benefit total sign openDay(%d) conf(%d) is nil", gshare.GetOpenServerDay(), days)
	}
	data := sys.GetData()
	var signDays uint32
	for i := uint32(1); i <= 31; i++ {
		if utils.IsSetBit(data.MonthSignDay, i-1) {
			signDays++
		}
	}
	if signDays < days {
		sys.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return false, nil
	}
	if utils.SliceContainsUint32(data.SignAward, days) {
		sys.owner.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return false, nil
	}
	data.SignAward = append(data.SignAward, days)
	engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBenefitTotalSign})
	return true, nil
}

func (sys *BfSignSys) sendLastMonForgetAward(player iface.IPlayer) {
	data := sys.GetData()
	loopId := data.SignLoopId
	loopConf := jsondata.GetBenefitSignConfById(loopId)
	if nil == loopConf {
		sys.LogError("benefit total sign openDay(%d) is nil", gshare.GetOpenServerDay())
		return
	}
	var signDays uint32
	for i := uint32(1); i <= 31; i++ {
		if utils.IsSetBit(data.MonthSignDay, i-1) {
			signDays++
		}
	}
	var award []*jsondata.StdReward
	for _, line := range loopConf.TotalSign {
		if line.Day <= signDays && !utils.SliceContainsUint32(data.SignAward, line.Day) {
			data.SignAward = append(data.SignAward, line.Day)
			award = jsondata.MergeStdReward(award, line.Award)
		}
	}
	if len(award) > 0 {
		player.SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_YYBenefitSignAward,
			Rewards: award,
		})
	}
}

func onBenefitsSignNewDay(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiBenefitSign).(*BfSignSys)
	if !ok || !sys.IsOpen() {
		return
	}
	data := sys.GetData()
	bsl := benefitmgr.GetBenefitSignLoop()
	if bsl.StartTime != data.SignStartTime {
		sys.sendLastMonForgetAward(player)
		data.SignStartTime = bsl.StartTime
		loopId := benefitmgr.GetBenefitSignLoop().BenefitSignId
		data.SignLoopId = loopId
		data.MonthResignTimes = 0
		data.MonthSignDay = 0
		data.SignAward = nil
		data.StartTime = bsl.StartTime
	}
	data.IsSignToday = false

	sys.S2CInfo()
}

func init() {
	RegisterSysClass(sysdef.SiBenefitSign, func() iface.ISystem {
		return &BfSignSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, onBenefitsSignNewDay)
	net.RegisterSysProto(41, 3, sysdef.SiBenefitSign, (*BfSignSys).c2sAward)
}
