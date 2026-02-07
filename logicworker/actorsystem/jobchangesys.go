/**
 * @Author: zjj
 * @Date: 2025/1/21
 * @Desc: 职业转职
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type JobChangeSys struct {
	Base
}

const (
	SingleBuyAwards = 1
	AllBuyAwards    = 2
)

func (s *JobChangeSys) s2cInfo() {
	s.SendProto3(8, 40, &pb3.S2C_8_40{
		Data: s.getData(),
	})
}

func (s *JobChangeSys) getData() *pb3.JobChangeData {
	data := s.GetBinaryData().JobChangeData
	if data == nil {
		s.GetBinaryData().JobChangeData = &pb3.JobChangeData{}
		data = s.GetBinaryData().JobChangeData
	}
	if data.JobUnlockAt == nil {
		data.JobUnlockAt = make(map[uint32]uint32)
	}
	if data.BuyAwards == nil {
		data.BuyAwards = make(map[uint32]bool)
	}
	return data
}

func (s *JobChangeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *JobChangeSys) OnLogin() {
	data := s.getData()
	job := 0
	for _, v := range data.JobUnlockAt {
		if v != 0 {
			job++
		}
	}
	conf := jsondata.GetJobChangeConf()
	if conf == nil {
		return
	}
	if job > 1 && job < len(conf.ChargeConf) {
		data.IsSingleBuy = true
	}
	s.s2cInfo()
}

func (s *JobChangeSys) OnOpen() {
	data := s.getData()
	data.JobUnlockAt[s.owner.GetJob()] = time_util.NowSec()
	s.s2cInfo()
}

func (s *JobChangeSys) c2sJobChange(msg *base.Message) error {
	var req pb3.C2S_8_41
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	reqJob := req.Job
	if reqJob == s.GetOwner().GetJob() {
		return neterror.ParamsInvalidError("job is same")
	}

	data := s.getData()
	conf := jsondata.GetJobChangeConf()
	if conf == nil {
		return neterror.ConfNotFoundError("job change conf not found")
	}

	unlockChargeAt := data.JobUnlockAt[reqJob]
	if unlockChargeAt == 0 {
		return neterror.ParamsInvalidError("un lock job change charge ")
	}

	sec := time_util.NowSec()
	if sec < data.LastJobChangeAt+conf.Cd {
		return neterror.ParamsInvalidError("in job change cd")
	}

	// 进行转职
	owner := s.GetOwner()
	if err := jobchange.JobChange(owner, reqJob); err != nil {
		return err
	}

	// 转职前先立即存个档 - 覆盖也没事 下面会存当前帧的最新数据
	owner.SaveToObjVersion(time_util.NowSec() - 1)

	data.LastJobChangeAt = sec
	data.BeforeJob = owner.GetJob()
	owner.SetJob(reqJob)
	newSex := custom_id.GetSexByJob(reqJob)
	owner.SetSex(newSex)
	owner.TriggerEvent(custom_id.AeJobChange)
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogJobChange, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.BeforeJob),
		StrArgs: fmt.Sprintf("%d", reqJob),
	})
	owner.Save(false)
	owner.ClosePlayer(cmd.DCJobChange)
	gshare.SendDBMsg(custom_id.GMsgInstantlySaveDB, owner.GetId())
	return nil
}

func (s *JobChangeSys) c2sRecDayGift(msg *base.Message) error {
	var req pb3.C2S_8_44
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	if data.DayGift {
		return neterror.ParamsInvalidError("today gift is rec")
	}
	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return neterror.ConfNotFoundError("JobChangeConf is nil")
	}
	if len(jobChangeConf.DayGift) > 0 {
		engine.GiveRewards(s.owner, jobChangeConf.DayGift, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogJobChangeDayGift,
		})
	}
	data.DayGift = true
	s.s2cInfo()
	return nil
}

func (s *JobChangeSys) c2sRecBuyAwards(msg *base.Message) error {
	var req pb3.C2S_8_45
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	data := s.getData()
	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return neterror.ConfNotFoundError("JobChangeConf is nil")
	}

	var awards jsondata.StdRewardVec
	var log pb3.LogId
	switch req.Type {
	case SingleBuyAwards:
		isJob := false
		for _, jobConf := range jobChangeConf.ChargeConf {
			if req.Job == jobConf.Job {
				isJob = true
			}
		}
		if !isJob {
			return neterror.ParamsInvalidError("job is wrong")
		}
		awards = jobChangeConf.Rewards
		log = pb3.LogId_LogJobChangeAwards
		if !data.IsSingleBuy {
			return neterror.ParamsInvalidError("no buy single jobChange")
		}
		data.BuyAwards[req.Job] = true
	case AllBuyAwards:
		awards = jobChangeConf.AllRewards
		log = pb3.LogId_LogJobChangeAllBuyAwards
		if data.IsSingleBuy {
			return neterror.ParamsInvalidError("buy single jobChange")
		}

		for _, v := range data.JobUnlockAt {
			if v == 0 {
				return neterror.ParamsInvalidError("no allBuy")
			}
		}
		for _, jobConf := range jobChangeConf.ChargeConf {
			data.BuyAwards[jobConf.Job] = true
		}

	}
	if len(awards) > 0 {
		engine.GiveRewards(s.owner, awards, common.EngineGiveRewardParam{
			LogId: log,
		})
	}
	s.SendProto3(8, 45, &pb3.S2C_8_45{})
	s.s2cInfo()
	return nil
}

func jobChangeChargeHandler(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	obj := actor.GetSysObj(sysdef.SiJobChange)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys := obj.(*JobChangeSys)
	if sys == nil {
		return false
	}

	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return false
	}

	var cConf *jsondata.JobChangeChargeConf
	for _, chargeConf := range jobChangeConf.ChargeConf {
		if chargeConf.ChargeId != conf.ChargeId {
			continue
		}
		cConf = chargeConf
		break
	}

	if cConf == nil {
		sys.LogWarn("not found conf")
		return false
	}

	data := sys.getData()
	at := data.JobUnlockAt[cConf.Job]
	if at != 0 {
		return false
	}
	data.JobUnlockAt[cConf.Job] = time_util.NowSec()
	data.IsSingleBuy = true
	actor.SendProto3(8, 42, &pb3.S2C_8_42{
		UnlockChargeAt: data.JobUnlockAt[cConf.Job],
		Job:            cConf.Job,
	})
	return true
}

func jobChangeChargeCheckFunHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	obj := actor.GetSysObj(sysdef.SiJobChange)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys := obj.(*JobChangeSys)
	if sys == nil {
		return false
	}

	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return false
	}

	var cConf *jsondata.JobChangeChargeConf
	for _, chargeConf := range jobChangeConf.ChargeConf {
		if chargeConf.ChargeId != conf.ChargeId {
			continue
		}
		cConf = chargeConf
		break
	}

	if cConf == nil {
		sys.LogWarn("not found conf")
		return false
	}

	data := sys.getData()
	at := data.JobUnlockAt[cConf.Job]
	return at == 0
}

func jobChangeAllBuyChargeHandler(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !jobChangeAllBuyChargeCheckFunHandler(actor, conf) {
		return false
	}
	obj := actor.GetSysObj(sysdef.SiJobChange)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys := obj.(*JobChangeSys)
	if sys == nil {
		return false
	}

	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return false
	}
	if jobChangeConf.AllBuyId != conf.ChargeId {
		return false
	}
	data := sys.getData()
	selfJob := actor.GetJob()
	if data.IsSingleBuy {
		return false
	}
	for _, chargeConf := range jobChangeConf.ChargeConf {
		if chargeConf.Job == selfJob {
			continue
		}
		data.JobUnlockAt[chargeConf.Job] = time_util.NowSec()
	}

	actor.SendProto3(8, 43, &pb3.S2C_8_43{
		JobsUnlockChargeAt: data.JobUnlockAt,
	})
	return true
}

func jobChangeAllBuyChargeCheckFunHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	obj := actor.GetSysObj(sysdef.SiJobChange)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys := obj.(*JobChangeSys)
	if sys == nil {
		return false
	}

	jobChangeConf := jsondata.GetJobChangeConf()
	if jobChangeConf == nil {
		return false
	}
	if jobChangeConf.AllBuyId != conf.ChargeId {
		return false
	}
	data := sys.getData()
	if data.IsSingleBuy {
		return false
	}
	return true
}

func init() {
	RegisterSysClass(sysdef.SiJobChange, func() iface.ISystem {
		return &JobChangeSys{}
	})
	engine.RegChargeEvent(chargedef.JobChange, jobChangeChargeCheckFunHandler, jobChangeChargeHandler)
	engine.RegChargeEvent(chargedef.JobChangeAllBuy, jobChangeAllBuyChargeCheckFunHandler, jobChangeAllBuyChargeHandler)

	net.RegisterSysProtoV2(8, 41, sysdef.SiJobChange, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*JobChangeSys).c2sJobChange
	})
	net.RegisterSysProtoV2(8, 44, sysdef.SiJobChange, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*JobChangeSys).c2sRecDayGift
	})
	net.RegisterSysProtoV2(8, 45, sysdef.SiJobChange, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*JobChangeSys).c2sRecBuyAwards
	})

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		s, ok := player.GetSysObj(sysdef.SiJobChange).(*JobChangeSys)
		if !ok || !s.IsOpen() {
			return
		}
		data := s.getData()
		data.DayGift = false
		s.s2cInfo()
	})
	gmevent.Register("jobChange.DoChange", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiJobChange)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*JobChangeSys)
		if sys == nil {
			return false
		}
		data := sys.getData()
		data.LastJobChangeAt = 0
		msg := base.PackMsg(0, 8, 41, 0, &pb3.C2S_8_41{
			Job: utils.AtoUint32(args[0]),
		})
		err := sys.c2sJobChange(msg)
		if err != nil {
			player.LogError("err:%v", err)
			return false
		}
		return true
	}, 1)
}
