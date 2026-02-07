/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 心魔塔
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

// 心魔事件

type HeartEvilSys struct {
	Base
}

func (s *HeartEvilSys) OnOpen() {
	s.s2cInfo()
}

func (s *HeartEvilSys) OnLogin() {
	s.s2cInfo()
}

func (s *HeartEvilSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HeartEvilSys) getData() *pb3.HeartEvilState {
	if s.GetBinaryData().HeartEvilState == nil {
		s.GetBinaryData().HeartEvilState = &pb3.HeartEvilState{}
	}
	return s.GetBinaryData().HeartEvilState
}

func (s *HeartEvilSys) s2cInfo() {
	s.SendProto3(153, 30, &pb3.S2C_153_30{
		State: s.getData(),
	})
}

func (s *HeartEvilSys) c2sAttack(msg *base.Message) error {
	var req pb3.C2S_17_211
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	if req.Level == 0 {
		return neterror.ParamsInvalidError("req level is zero")
	}

	data := s.getData()
	conf, ok := jsondata.GetHeartEvilConf(req.Level)
	if !ok {
		return neterror.ConfNotFoundError("cur lv %d not found heard evil boss conf", req.Level)
	}

	if data.Level >= conf.Level {
		s.GetOwner().LogWarn("this copy has been cleared , lv is %d,cur lv is %d", conf.Level, data.Level)
		s.GetOwner().SendTipMsg(tipmsgid.TpFbEnterCondNotEnough)
		return nil
	}

	err := s.GetOwner().EnterFightSrv(base.LocalFightServer, fubendef.EnterHeartEvil, &pb3.AttackHeartEvil{
		Level: conf.Level,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	s.GetOwner().TriggerQuestEvent(custom_id.QttAttackHeartEvil, 0, 1)

	return nil
}

func (s *HeartEvilSys) handleCheckOutHeartEvilFb(buf []byte) {
	var req pb3.FbSettlement
	if err := pb3.Unmarshal(buf, &req); err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}
	if len(req.ExData) == 0 {
		s.GetOwner().LogError("not found ext data. req is %v", &req)
		return
	}

	exData := req.ExData
	level := exData[0]
	evilConf, ok := jsondata.GetHeartEvilConf(level)
	if !ok {
		s.GetOwner().LogError("not found evil conf , level is %d", level)
		return
	}
	owner := s.GetOwner()

	// 未胜利
	if req.Ret != custom_id.FbSettleResultWin {
		owner.SendProto3(17, 254, &pb3.S2C_17_254{
			Settle: &req,
		})
		return
	}
	// 下发奖励
	data := s.getData()
	if data.Level == evilConf.Level {
		s.GetOwner().LogError("data.Level %d , evilConf.Level %d", data.Level, evilConf.Level)
		return
	}

	data.Level = evilConf.Level
	mgr, ok := jsondata.GetHeartEvilCommonConfMgr()
	if ok && !engine.CheckBagSpaceByRewards(owner, evilConf.Rewards) {
		engine.SendRewardsByEmail(owner, uint16(mgr.MailId), nil, evilConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogHeartEvilAttackSuccessAwards,
		})
		owner.SendTipMsg(tipmsgid.BagIsFullAwardSendByMail)
	} else {
		status := engine.GiveRewards(owner, evilConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogHeartEvilAttackSuccessAwards,
		})

		if !status {
			s.GetOwner().LogError("handleCheckOutHeartEvilFb failed give reward failed")
			return
		}
	}
	req.ShowAward = jsondata.StdRewardVecToPb3RewardVec(evilConf.Rewards)
	// 任务
	owner.TriggerQuestEvent(custom_id.QttUpgradeHeartEvilLevel, data.Level, 1)

	owner.TriggerEvent(custom_id.AePassHeartEvilEvent, int64(data.Level))

	owner.SendProto3(17, 254, &pb3.S2C_17_254{
		Settle: &req,
	})
}

func init() {
	RegisterSysClass(sysdef.SiHeartEvil, func() iface.ISystem {
		return &HeartEvilSys{}
	})

	engine.RegQuestTargetProgress(custom_id.QttUpgradeHeartEvilLevel, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(args) < 1 {
			return 0
		}
		obj := actor.GetSysObj(sysdef.SiHeartEvil)
		if obj == nil {
			return 0
		}
		if !obj.IsOpen() {
			return 0
		}
		sys, ok := obj.(*HeartEvilSys)
		if !ok {
			return 0
		}
		return sys.getData().Level
	})

	engine.RegisterActorCallFunc(playerfuncid.CheckOutHeartEvilFb, func(player iface.IPlayer, buf []byte) {
		obj := player.GetSysObj(sysdef.SiHeartEvil)
		if obj == nil {
			return
		}
		if !obj.IsOpen() {
			return
		}
		sys, ok := obj.(*HeartEvilSys)
		if !ok {
			return
		}
		sys.handleCheckOutHeartEvilFb(buf)
	})

	net.RegisterSysProtoV2(17, 211, sysdef.SiHeartEvil, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeartEvilSys).c2sAttack
	})
}
