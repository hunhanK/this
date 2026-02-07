/**
 * @Author: zjj
 * @Date: 2024/12/27
 * @Desc: 活跃战令
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

const (
	ActivityHandBookTypeByCharge = 1
	ActivityHandBookTypeByMoney  = 2
)

type ActivityHandBookSys struct {
	PlayerYYBase
}

func (s *ActivityHandBookSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.ActivityHandBook {
		return
	}
	delete(yyData.ActivityHandBook, s.Id)
}

func (s *ActivityHandBookSys) GetData() *pb3.PYYActivityHandBookData {
	yyData := s.GetYYData()
	if nil == yyData.ActivityHandBook {
		yyData.ActivityHandBook = make(map[uint32]*pb3.PYYActivityHandBookData)
	}
	if nil == yyData.ActivityHandBook[s.Id] {
		yyData.ActivityHandBook[s.Id] = &pb3.PYYActivityHandBookData{}
	}
	orderFunc := yyData.ActivityHandBook[s.Id]
	return orderFunc
}

func (s *ActivityHandBookSys) S2CInfo() {
	s.SendProto3(145, 10, &pb3.S2C_145_10{
		ActiveId: s.Id,
		Data:     s.GetData(),
	})
}

func (s *ActivityHandBookSys) triggerLoginAddExp() {
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if conf != nil && conf.OtherProvideProcessSysId == sysdef.SiRoleInfo {
		s.GetPlayer().TriggerEvent(custom_id.AeAddActivityHandBookExp, sysdef.SiRoleInfo, uint32(1), s.GetId())
	}
}

func (s *ActivityHandBookSys) OnOpen() {
	s.S2CInfo()
	s.triggerLoginAddExp()
}

func (s *ActivityHandBookSys) Login() {
	s.S2CInfo()
}

func (s *ActivityHandBookSys) OnReconnect() {
	s.S2CInfo()
}

func (s *ActivityHandBookSys) NewDay() {
	s.triggerLoginAddExp()
}

func (s *ActivityHandBookSys) OnEnd() {
	// 活动结束
	data := s.GetData()
	process := data.Process
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}

	// 检查下还有哪些奖励没领
	var rewards jsondata.StdRewardVec
	for _, processAward := range conf.ProcessAwards {
		awardsProcess := processAward.Process
		if awardsProcess > process {
			continue
		}

		if !pie.Int64s(data.NormalProcessList).Contains(awardsProcess) {
			rewards = append(rewards, processAward.Awards...)
			data.NormalProcessList = append(data.NormalProcessList, awardsProcess)
		}

		if data.UnlockHighAwardsTimeAt == 0 {
			continue
		}

		if !pie.Int64s(data.HighProcessList).Contains(awardsProcess) {
			rewards = append(rewards, processAward.HAwards...)
			data.HighProcessList = append(data.HighProcessList, awardsProcess)
		}
	}

	if len(rewards) > 0 {
		rewards = jsondata.MergeStdReward(rewards)
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_YyFlyingFairyOrderFunc,
			Rewards: rewards,
		})
	}
}

func (s *ActivityHandBookSys) c2sUnlockHighAwards(msg *base.Message) error {
	var req pb3.C2S_145_12
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}
	data := s.GetData()
	if data.UnlockHighAwardsTimeAt > 0 {
		return neterror.ParamsInvalidError("already unlock high awards")
	}

	bookConf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if bookConf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	if bookConf.UnLockType != ActivityHandBookTypeByMoney {
		return neterror.ParamsInvalidError("unlock high awards type %d error", bookConf.UnLockType)
	}

	owner := s.GetPlayer()
	if len(bookConf.UnlockHighAwards) == 0 || !owner.ConsumeByConf(bookConf.UnlockHighAwards, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYActivityHandBookDataUnlock}) {
		return neterror.ConfNotFoundError("%s consume failed", s.GetPrefix())
	}

	err := s.unlockHighAwards()
	if err != nil {
		return err
	}
	return nil
}

func (s *ActivityHandBookSys) unlockHighAwards() error {
	data := s.GetData()
	if data.UnlockHighAwardsTimeAt > 0 {
		return neterror.ParamsInvalidError("already unlock high awards")
	}
	data.UnlockHighAwardsTimeAt = time_util.NowSec()
	s.SendProto3(145, 12, &pb3.S2C_145_12{
		ActiveId:               s.Id,
		UnlockHighAwardsTimeAt: data.UnlockHighAwardsTimeAt,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYActivityHandBookDataUnlock, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.UnlockHighAwardsTimeAt),
	})
	return nil
}

func (s *ActivityHandBookSys) c2sRecReward(msg *base.Message) error {
	var req pb3.C2S_145_13
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError("err:%v", err.Error())
		return err
	}

	iPlayer := s.GetPlayer()
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	awards, err := s.recAwards(req.IsH, req.Process)
	if err != nil {
		iPlayer.LogError("err:%v", err)
		return err
	}

	// 下发奖励
	engine.GiveRewards(iPlayer, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYActivityHandBookDataRecAwards})
	s.SendProto3(145, 13, &pb3.S2C_145_13{
		ActiveId: s.Id,
		IsH:      req.IsH,
		Process:  req.Process,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYActivityHandBookDataRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%v", req.Process, req.IsH),
	})

	return nil
}

func (s *ActivityHandBookSys) recAwards(isH bool, reqProcess int64) (jsondata.StdRewardVec, error) {
	data := s.GetData()
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		err := neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
		return nil, err
	}
	if data.Process < reqProcess {
		return nil, neterror.ParamsInvalidError("%s reqProcess not reach %d %d", s.GetPrefix(), data.Process, reqProcess)
	}

	// 高级奖励校验
	if isH && data.UnlockHighAwardsTimeAt == 0 {
		return nil, neterror.ParamsInvalidError("%s not open high awards", s.GetPrefix())
	}

	// 历史领取
	var historyRecLvs = data.NormalProcessList
	if isH {
		historyRecLvs = data.HighProcessList
	}
	if pie.Int64s(historyRecLvs).Contains(reqProcess) {
		return nil, neterror.ParamsInvalidError("%s already rec %v %d", s.GetPrefix(), isH, reqProcess)
	}

	var expAwardConf *jsondata.PYYActivityHandBookProcessAward
	for _, expAwards := range conf.ProcessAwards {
		if expAwards.Process != int64(reqProcess) {
			continue
		}
		expAwardConf = expAwards
		break
	}
	if expAwardConf == nil {
		return nil, neterror.ConfNotFoundError("%s not found exp awards %d", s.GetPrefix(), reqProcess)
	}

	// 领取该等级的奖励
	var awards = expAwardConf.Awards
	if isH {
		awards = expAwardConf.HAwards
	}

	// 记录领取等级
	if isH {
		data.HighProcessList = append(data.HighProcessList, reqProcess)
	} else {
		data.NormalProcessList = append(data.NormalProcessList, reqProcess)
	}
	return awards, nil
}

func (s *ActivityHandBookSys) c2sQuickRecReward(_ *base.Message) error {
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	data := s.GetData()
	process := data.Process
	var totalAwards jsondata.StdRewardVec
	for _, awardConf := range conf.ProcessAwards {
		if process < awardConf.Process {
			continue
		}
		awards, err := s.recAwards(false, awardConf.Process)
		if err == nil && len(awards) != 0 {
			totalAwards = append(totalAwards, awards...)
		}
		if data.UnlockHighAwardsTimeAt == 0 {
			continue
		}
		awards, err = s.recAwards(true, awardConf.Process)
		if err == nil && len(awards) != 0 {
			totalAwards = append(totalAwards, awards...)
		}
	}

	player := s.GetPlayer()
	if len(totalAwards) == 0 {
		s.LogWarn("not can rec awards")
		return nil
	}

	engine.GiveRewards(player, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYActivityHandBookDataQuickAwards})
	// 直接发最新的数据过去
	s.S2CInfo()
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYActivityHandBookDataQuickAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", process),
	})
	return nil
}

func (s *ActivityHandBookSys) addExp(sysId uint64, process int64) {
	s.GetData().Process += process
	s.SendProto3(145, 11, &pb3.S2C_145_11{
		ActiveId: s.Id,
		Process:  s.GetData().Process,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogPYYActivityHandBookDataAddExp, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", sysId, process),
	})
}

func (s *ActivityHandBookSys) c2sLevelBuy(msg *base.Message) error {
	var req pb3.C2S_145_15
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.LogError("err:%v", err.Error())
		return err
	}
	if req.Count == 0 {
		return neterror.ParamsInvalidError("not can set count zero")
	}
	conf := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
	data := s.GetData()
	count := req.Count
	var record string
	player := s.GetPlayer()
	switch conf.BuyType {
	case 1: // 直接购买档位
		process := data.Process
		var curIdx int
		var find bool
		for idx, processAward := range conf.ProcessAwards {
			if processAward.Process > process {
				continue
			}
			curIdx = idx
			find = true
		}
		newIdx := curIdx + int(count)
		if !find && newIdx > 0 {
			newIdx -= 1
		}
		if newIdx >= len(conf.ProcessAwards) {
			return neterror.ParamsInvalidError("exp:%d count:%d length:%d", data.Process, count, len(conf.ProcessAwards))
		}
		multi := jsondata.ConsumeMulti(conf.PurchaseLevel, count)
		if len(multi) == 0 {
			return neterror.ConsumeFailedError("%s not found conf", s.GetPrefix())
		}
		if !player.ConsumeByConf(multi, false, common.ConsumeParams{LogId: pb3.LogId_LogYYActivityHandBook}) {
			return neterror.ConsumeFailedError("%s consume failed", s.GetPrefix())
		}
		processAward := conf.ProcessAwards[newIdx]
		record = fmt.Sprintf("%d_%d", data.Process, processAward.Process-process)
		s.addExp(0, int64(processAward.Process-process))
	default: // 购买进度
		if uint32(data.Process)+count > uint32(len(conf.ProcessAwards)) {
			return neterror.ParamsInvalidError("exp:%d count:%d length:%d", data.Process, count, len(conf.ProcessAwards))
		}
		multi := jsondata.ConsumeMulti(conf.PurchaseLevel, count)
		if len(multi) == 0 {
			return neterror.ConsumeFailedError("%s not found conf", s.GetPrefix())
		}

		if !player.ConsumeByConf(multi, false, common.ConsumeParams{LogId: pb3.LogId_LogYYActivityHandBook}) {
			return neterror.ConsumeFailedError("%s consume failed", s.GetPrefix())
		}
		record = fmt.Sprintf("%d_%d", data.Process, count)
		s.addExp(0, int64(count))
	}
	s.SendProto3(145, 15, &pb3.S2C_145_15{
		ActiveId: s.Id,
		Process:  uint32(data.Process),
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogYYActivityHandBook, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%s", data.Process, record),
	})
	return nil
}

func rangeActivityHandBookSys(player iface.IPlayer, doLogic func(sys *ActivityHandBookSys)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYActivityHandbook)
	if len(yyList) == 0 {
		return
	}
	for i := range yyList {
		v := yyList[i]
		sys, ok := v.(*ActivityHandBookSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		doLogic(sys)
	}
}

func handleAeAddActivityHandBookExp(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	sysId := args[0].(int)
	exp := args[1].(uint32)
	var yyId uint32
	if len(args) >= 3 {
		yyId = args[2].(uint32)
	}
	rangeActivityHandBookSys(player, func(s *ActivityHandBookSys) {
		// 登陆 只能自己给自己加经验
		if yyId != 0 && yyId != s.Id {
			return
		}
		config := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
		if config == nil {
			return
		}
		if config.OtherProvideProcessSysId != 0 && config.OtherProvideProcessSysId == sysId {
			s.addExp(uint64(sysId), int64(exp))
		}
	})
}

func activityHandBookChargeHandler(actor iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	var ret bool
	rangeActivityHandBookSys(actor, func(s *ActivityHandBookSys) {
		if ret {
			return
		}
		config := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
		if config.UnLockType == ActivityHandBookTypeByCharge && conf.ChargeId == config.ChargeId {
			err := s.unlockHighAwards()
			if err != nil {
				return
			}
			ret = true
		}
	})
	return ret
}

func activityHandBookChargeCheckFunHandler(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	var ret bool
	rangeActivityHandBookSys(actor, func(s *ActivityHandBookSys) {
		if ret {
			return
		}
		config := jsondata.GetPYYActivityHandBookConf(s.ConfName, s.ConfIdx)
		if config.UnLockType == ActivityHandBookTypeByCharge && conf.ChargeId == config.ChargeId {
			data := s.GetData()
			if data.UnlockHighAwardsTimeAt > 0 {
				return
			}
			ret = true
		}
	})
	return ret
}

func init() {
	engine.RegChargeEvent(chargedef.ActivityHandBook, activityHandBookChargeCheckFunHandler, activityHandBookChargeHandler)
	event.RegActorEvent(custom_id.AeAddActivityHandBookExp, handleAeAddActivityHandBookExp)
	pyymgr.RegPlayerYY(yydefine.PYYActivityHandbook, func() iface.IPlayerYY {
		return &ActivityHandBookSys{}
	})
	net.RegisterYYSysProtoV2(145, 12, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ActivityHandBookSys).c2sUnlockHighAwards
	})
	net.RegisterYYSysProtoV2(145, 13, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ActivityHandBookSys).c2sRecReward
	})
	net.RegisterYYSysProtoV2(145, 14, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ActivityHandBookSys).c2sQuickRecReward
	})
	net.RegisterYYSysProtoV2(145, 15, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ActivityHandBookSys).c2sLevelBuy
	})
	gmevent.Register("ActivityHandBookSys.addExp", func(player iface.IPlayer, args ...string) bool {
		rangeActivityHandBookSys(player, func(sys *ActivityHandBookSys) {
			sys.addExp(0, utils.AtoInt64(args[0]))
		})
		return true
	}, 1)
}
