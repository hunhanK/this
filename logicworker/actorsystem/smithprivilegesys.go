/**
 * @Author: zjj
 * @Date: 2025年7月28日
 * @Desc: 奇匠特权
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
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
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type SmithPrivilegeSys struct {
	Base
}

func (s *SmithPrivilegeSys) s2cInfo() {
	s.SendProto3(9, 70, &pb3.S2C_9_70{
		Data: s.getData(),
	})
}

func (s *SmithPrivilegeSys) getData() *pb3.SmithPrivilegeData {
	data := s.GetBinaryData().SmithPrivilegeData
	if data == nil {
		s.GetBinaryData().SmithPrivilegeData = &pb3.SmithPrivilegeData{}
		data = s.GetBinaryData().SmithPrivilegeData
	}
	return data
}

func (s *SmithPrivilegeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SmithPrivilegeSys) OnLogin() {
	s.s2cInfo()
}

func (s *SmithPrivilegeSys) OnOpen() {
	s.s2cInfo()
}

func (s *SmithPrivilegeSys) checkPrivilege() bool {
	data := s.getData()
	privilegeExpAt := data.PrivilegeExpAt
	ok := privilegeExpAt != 0 && privilegeExpAt > time_util.NowSec()
	if ok {
		return true
	}
	return false
}

func (s *SmithPrivilegeSys) c2sRecDailyAwards(_ *base.Message) error {
	if !s.checkPrivilege() {
		return neterror.ParamsInvalidError("not buy privilege or privilege expired")
	}
	data := s.getData()
	nowSec := time_util.NowSec()
	if nowSec != 0 && time_util.IsSameDay(data.LastRecDailyAwardsAt, nowSec) && data.LastRecDailyAwardsAt > nowSec {
		return neterror.ParamsInvalidError("today already rec daily awards")
	}
	commonConf := jsondata.GetMagicalCraftsmanPrivilegeConfig()
	if commonConf == nil {
		return neterror.ConfNotFoundError("not found common conf")
	}
	if len(commonConf.DailyAwards) == 0 {
		return neterror.ConfNotFoundError("not found daily awards")
	}
	data.LastRecDailyAwardsAt = nowSec
	owner := s.GetOwner()
	if len(commonConf.DailyAwards) > 0 {
		engine.GiveRewards(owner, commonConf.DailyAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSmithPrivilegeDailyAwards, NoTips: true})
		owner.SendShowRewardsPop(commonConf.DailyAwards)
	}
	s.SendProto3(9, 71, &pb3.S2C_9_71{
		LastRecDailyAwardsAt: data.LastRecDailyAwardsAt,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSmithPrivilegeDailyAwards, &pb3.LogPlayerCounter{NumArgs: uint64(data.LastRecDailyAwardsAt)})
	return nil
}

func (s *SmithPrivilegeSys) onCharge(chargeId uint32) {
	owner := s.GetOwner()
	commonConf := jsondata.GetMagicalCraftsmanPrivilegeConfig()
	if commonConf == nil {
		owner.LogWarn("not found common conf")
		return
	}

	data := s.getData()
	privilegeExpAt := data.PrivilegeExpAt
	nowSec := time_util.NowSec()
	var newPrivilegeExpAt uint32
	if nowSec > privilegeExpAt {
		data.PrivilegeStartAt = nowSec
		newPrivilegeExpAt = nowSec + commonConf.Days*86400
	} else {
		newPrivilegeExpAt += privilegeExpAt + commonConf.Days*86400
	}
	data.PrivilegeExpAt = newPrivilegeExpAt
	s.SendProto3(9, 72, &pb3.S2C_9_72{
		PrivilegeExpAt:   newPrivilegeExpAt,
		PrivilegeStartAt: data.PrivilegeStartAt,
	})
	if len(commonConf.DailyAwards) > 0 {
		engine.GiveRewards(owner, commonConf.ChargeAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSmithPrivilege, NoTips: true})
		owner.SendShowRewardsPop(commonConf.ChargeAwards)
	}
	engine.BroadcastTipMsgById(tipmsgid.SmithPrivilegeTip1, owner.GetId(), owner.GetName())
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogSmithPrivilege, &pb3.LogPlayerCounter{NumArgs: uint64(chargeId), StrArgs: fmt.Sprintf("%d_%d", data.PrivilegeStartAt, data.PrivilegeExpAt)})

}

func (s *SmithPrivilegeSys) onNewDay() {
	commonConf := jsondata.GetMagicalCraftsmanPrivilegeConfig()
	if commonConf == nil || len(commonConf.DailyAwards) == 0 {
		return
	}

	data := s.getData()
	nowSec := time_util.NowSec()

	privilegeStartAt := data.PrivilegeStartAt
	privilegeExpAt := data.PrivilegeExpAt
	lastRecDailyAwardsAt := data.LastRecDailyAwardsAt

	// 无效特权时间
	if privilegeExpAt == 0 || privilegeStartAt == 0 || privilegeExpAt < privilegeStartAt {
		return
	}

	// 特权未开始
	if nowSec < privilegeStartAt {
		return
	}

	// 补发起点修正逻辑
	if lastRecDailyAwardsAt == 0 {
		// 从特权前一天开始判断
		lastRecDailyAwardsAt = privilegeStartAt - 86400
	} else if lastRecDailyAwardsAt < privilegeStartAt {
		// 如果上次领取是旧特权，新的特权不能重复发
		if !time_util.IsSameDay(lastRecDailyAwardsAt, privilegeStartAt) {
			lastRecDailyAwardsAt = privilegeStartAt - 86400
		}
	}

	// 起始补发日：上次领取的第二天零点
	startDay := time_util.GetZeroTime(lastRecDailyAwardsAt + 86400)

	// 补发截止日：昨天零点
	endDay := time_util.GetBeforeDaysZeroTime(1) // ✅ 修复点：必须是昨天，不是今天！

	// 昨日领取的补发日不能重复
	if time_util.IsSameDay(lastRecDailyAwardsAt, endDay) {
		return
	}

	// 补发范围限制在特权有效期内
	privilegeStartDay := time_util.GetZeroTime(privilegeStartAt)
	privilegeExpDay := time_util.GetZeroTime(privilegeExpAt)

	if startDay < privilegeStartDay {
		startDay = privilegeStartDay
	}
	if endDay > privilegeExpDay {
		endDay = privilegeExpDay
	}

	// 计算应补发天数
	diffDays := time_util.GetDiffDays(int64(startDay), int64(endDay)) + 1
	if diffDays <= 0 {
		return
	}

	owner := s.GetOwner()

	// 奖励补发
	copyConsumeVec := jsondata.CopyStdRewardVec(commonConf.DailyAwards)
	totalReward := jsondata.StdRewardMulti(copyConsumeVec, int64(diffDays))

	mailmgr.SendMailToActor(owner.GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_SmithPrivilege,
		Rewards: totalReward,
	})

	// ✅ 设置最后领取时间为补发的最后一天的 23:59:59
	lastAwardDayEnd := endDay + 86399
	if lastAwardDayEnd > privilegeExpAt {
		lastAwardDayEnd = privilegeExpAt
	}
	data.LastRecDailyAwardsAt = lastAwardDayEnd

	owner.LogInfo("特权卡补发完成: 补发天数=%d, 最后领奖时间=%d (%s)", diffDays, lastAwardDayEnd, time_util.TimeToStr(uint32(lastAwardDayEnd)))
}

func (s *SmithPrivilegeSys) GetRecycleExtCount(itemId uint32) uint32 {
	if !s.checkPrivilege() {
		return 0
	}
	commonConf := jsondata.GetMagicalCraftsmanPrivilegeConfig()
	if commonConf == nil || commonConf.RecycleExtCount == nil {
		return 0
	}
	recycleExtCount := commonConf.RecycleExtCount[itemId]
	if recycleExtCount == nil {
		return 0
	}
	return recycleExtCount.Count
}

func chargeSmithPrivilegeCheck(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	return true
}

func chargeSmithPrivilegeBack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if s, ok := actor.GetSysObj(sysdef.SiSmithPrivilege).(*SmithPrivilegeSys); ok && s.IsOpen() {
		s.onCharge(conf.ChargeId)
		return true
	}
	return false
}

func handleSmithPrivilegeAeNewDay(player iface.IPlayer, _ ...interface{}) {
	if s, ok := player.GetSysObj(sysdef.SiSmithPrivilege).(*SmithPrivilegeSys); ok && s.IsOpen() {
		s.onNewDay()
	}
}

func init() {
	RegisterSysClass(sysdef.SiSmithPrivilege, func() iface.ISystem {
		return &SmithPrivilegeSys{}
	})
	engine.RegChargeEvent(chargedef.SmithPrivilege, chargeSmithPrivilegeCheck, chargeSmithPrivilegeBack)
	event.RegActorEventL(custom_id.AeNewDay, handleSmithPrivilegeAeNewDay)
	net.RegisterSysProtoV2(9, 71, sysdef.SiSmithPrivilege, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SmithPrivilegeSys).c2sRecDailyAwards
	})
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		if s, ok := player.GetSysObj(sysdef.SiSmithPrivilege).(*SmithPrivilegeSys); ok && s.IsOpen() {
			if s.checkPrivilege() {
				return conf.Smith, nil
			}
		}
		return
	})
}
