/**
 * @Author: LvYuMeng
 * @Date: 2025/02/24
 * @Desc: 限时激活
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
)

type TrialActiveSys struct {
	Base
	timers map[uint32]*time_util.Timer
}

func (s *TrialActiveSys) OnInit() {
	s.timers = make(map[uint32]*time_util.Timer)
}

func (s *TrialActiveSys) OnLogout() {
	for _, tm := range s.timers {
		tm.Stop()
	}
}

func (s *TrialActiveSys) OnLogin() {
	data := s.GetData()
	for _, line := range data.Data {
		id := line.Id
		s.checkTimeOut(id)
	}
}

func (s *TrialActiveSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *TrialActiveSys) OnReconnect() {
	s.s2cInfo()
}

func (s *TrialActiveSys) s2cInfo() {
	s.SendProto3(12, 20, &pb3.S2C_12_20{Data: s.GetData()})
}

func (s *TrialActiveSys) GetData() *pb3.TrialActiveInfo {
	binary := s.GetBinaryData()
	if nil == binary.TrialActiveInfo {
		binary.TrialActiveInfo = &pb3.TrialActiveInfo{}
	}
	if nil == binary.TrialActiveInfo.Data {
		binary.TrialActiveInfo.Data = map[uint32]*pb3.TrialActive{}
	}
	return binary.TrialActiveInfo
}

func (s *TrialActiveSys) stopTimer(id uint32) {
	if tm, ok := s.timers[id]; ok {
		tm.Stop()
	}
}

func (s *TrialActiveSys) checkTimeOut(id uint32) {
	s.stopTimer(id)

	nowSec := time_util.NowSec()
	data := s.GetData()
	st, ok := data.Data[id]
	if !ok {
		return
	}

	if st.TimeOut < nowSec {
		delete(data.Data, id)
		s.disActive(id)
		s.SendProto3(12, 22, &pb3.S2C_12_22{EffectId: id})

		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogTrialActiveEffect, &pb3.LogPlayerCounter{
			NumArgs: uint64(id),
		})
		return
	}

	s.timers[id] = timer.SetTimeout(time.Duration(data.Data[id].TimeOut-nowSec)*time.Second, func() {
		s.checkTimeOut(id)
	})
}

func (s *TrialActiveSys) disActive(id uint32) {
	conf, ok := jsondata.GetTrialActiveConfById(id)
	if !ok {
		s.LogError("conf is nil")
		return
	}

	handler, err := getTrialActiveHandler(s.owner, conf.ActiveType)
	if nil != err {
		s.LogError("err:%v", err)
		return
	}

	paramsConf, ok := conf.GetTrialActiveParamsConf(s.owner.GetJob())
	if !ok {
		s.LogError("paramsConf is nil")
		return
	}

	err = handler.DoForget(paramsConf)
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *TrialActiveSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_12_21
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	useId := req.UseId
	conf, ok := jsondata.GetTrialActiveUseConfById(useId)
	if !ok {
		return neterror.ConfNotFoundError("conf %d is nil", useId)
	}

	if !s.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogTrialActiveConsume}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	err := s.doActive(conf.EffectId, conf.TimeLimit)
	if nil != err {
		return err
	}

	return nil
}

func (s *TrialActiveSys) doActive(effectId, addTime uint32) error {
	conf, ok := jsondata.GetTrialActiveConfById(effectId)
	if !ok {
		return neterror.ConfNotFoundError("conf %d is nil", effectId)
	}
	data := s.GetData()

	nowSec := time_util.NowSec()
	st, inTrial := data.Data[effectId]
	if inTrial {
		if st.TimeOut < nowSec {
			st.TimeOut = nowSec + addTime
		} else {
			st.TimeOut += addTime
		}
	} else {
		handler, err := getTrialActiveHandler(s.owner, conf.ActiveType)
		if nil != err {
			return err
		}
		paramsConf, ok := conf.GetTrialActiveParamsConf(s.owner.GetJob())
		if !ok {
			return neterror.ConfNotFoundError("paramsConf is nil")
		}
		err = handler.DoActive(paramsConf)
		if nil != err {
			return err
		}
		st = &pb3.TrialActive{
			Id:         effectId,
			TimeOut:    nowSec + addTime,
			ActiveType: conf.ActiveType,
		}
		data.Data[effectId] = st
	}

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogTrialActiveEffect, &pb3.LogPlayerCounter{
		NumArgs: uint64(effectId),
		StrArgs: fmt.Sprintf("%d", data.Data[effectId].TimeOut),
	})
	s.stopTimer(effectId)

	var dur uint32
	if data.Data[effectId].TimeOut > nowSec {
		dur = data.Data[effectId].TimeOut - nowSec
	}

	s.timers[effectId] = timer.SetTimeout(time.Duration(dur)*time.Second, func() {
		s.checkTimeOut(effectId)
	})

	s.SendProto3(12, 21, &pb3.S2C_12_21{St: st})

	return nil
}

func (s *TrialActiveSys) IsInTrialActive(activeType uint32, args []uint32) bool {
	data := s.GetData()
	for id, line := range data.Data {
		if line.ActiveType != activeType {
			continue
		}
		conf, ok := jsondata.GetTrialActiveConfById(id)
		if !ok {
			continue
		}
		paramsConf, ok := conf.GetTrialActiveParamsConf(s.owner.GetJob())
		if !ok {
			continue
		}
		if !pie.Uint32s(paramsConf.Params).Equals(args) {
			continue
		}
		return true
	}

	return false
}

func (s *TrialActiveSys) StopTrialActive(activeType uint32, args []uint32) {
	data := s.GetData()
	var delIds []uint32

	for id, line := range data.Data {
		if line.ActiveType != activeType {
			continue
		}
		conf, ok := jsondata.GetTrialActiveConfById(id)
		if !ok {
			continue
		}
		paramsConf, ok := conf.GetTrialActiveParamsConf(s.owner.GetJob())
		if !ok {
			continue
		}
		if !pie.Uint32s(paramsConf.Params).Equals(args) {
			continue
		}
		delIds = append(delIds, id)
	}

	for _, id := range delIds {
		delete(data.Data, id)
		s.stopTimer(id)
		s.disActive(id)
		s.SendProto3(12, 22, &pb3.S2C_12_22{EffectId: id})
	}
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogTrialActiveEffect, &pb3.LogPlayerCounter{
		StrArgs: fmt.Sprintf("%v", delIds),
	})
}

func init() {
	RegisterSysClass(sysdef.SiTrialActive, func() iface.ISystem {
		return &TrialActiveSys{}
	})

	net.RegisterSysProtoV2(12, 21, sysdef.SiTrialActive, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TrialActiveSys).c2sActive
	})

	gmevent.Register("trialActive", func(player iface.IPlayer, args ...string) bool {
		id := utils.AtoUint32(args[0])
		sec := utils.AtoUint32(args[1])
		sys := player.GetSysObj(sysdef.SiTrialActive).(*TrialActiveSys)

		err := sys.doActive(id, sec)

		return err == nil
	}, 1)
}
