/**
 * @Author: lzp
 * @Date: 2024/1/22
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/srvlib/utils"

	"github.com/gzjjyz/random"
)

const (
	CoeRate = 10000
)

const (
	BuyType1 = 1 // 免费购买
	BuyType2 = 2 // 付费购买
	BuyType3 = 3 // 一次购买
)

type DailyDirGiftSys struct {
	Base
	ref uint32 // 购买礼包id
}

func (s *DailyDirGiftSys) OnLogin() {
	s.S2CInfo()
}

func (s *DailyDirGiftSys) OnReconnect() {
	s.S2CInfo()
}

func (s *DailyDirGiftSys) OnOpen() {
	s.RefreshDailyGift()
	s.S2CInfo()
}

func (s *DailyDirGiftSys) OnNewDay() {
	s.TrySendMail()
	s.RefreshDailyGift()
	s.tryCheckOutReachAwardsAndRefresh()
	s.S2CInfo()
}

func (s *DailyDirGiftSys) GetData() *pb3.DailyDirGiftData {
	data := s.GetBinaryData().DailyDirGiftData
	if data == nil {
		data = &pb3.DailyDirGiftData{
			Refs:       make([]uint32, 0),
			DayBuyRefs: make(map[uint32]uint32),
		}
		s.GetBinaryData().DailyDirGiftData = data
	}

	if data.DayBuyRefs == nil {
		data.DayBuyRefs = make(map[uint32]uint32)
	}
	if data.Refs == nil {
		data.Refs = make([]uint32, 0)
	}
	if data.Awards == nil {
		data.Awards = make([]*pb3.StdAward, 0)
	}
	if data.CurIdx == 0 {
		data.CurIdx = 1
	}
	return data
}

func (s *DailyDirGiftSys) TrySendMail() {
	data := s.GetData()
	if len(data.Awards) > 0 {
		s.owner.SendMail(&mailargs.SendMailSt{
			ConfId:  common.Mail_DailyDirGiftReward,
			Rewards: jsondata.Pb3RewardVecToStdRewardVec(data.Awards),
		})
	}
}

func (s *DailyDirGiftSys) CalcDayRewards() jsondata.StdRewardVec {
	data := s.GetData()
	rewards := make(jsondata.StdRewardVec, 0)
	for ref, count := range data.DayBuyRefs {
		conf := jsondata.GetDailyDirGift(ref)
		if conf == nil {
			continue
		}
		if len(conf.DayRewards) == 0 {
			continue
		}
		idx := 0
		for k, reward := range conf.DayRewards {
			if reward.OpenDay > gshare.GetOpenServerDay() {
				break
			}
			idx = k
		}
		tmpReward := conf.DayRewards[idx].Copy()
		tmpReward.Count *= int64(count)
		rewards = append(rewards, tmpReward)
	}
	return jsondata.MergeStdReward(rewards)
}

func (s *DailyDirGiftSys) RefreshDailyGift() {
	circle := s.owner.GetCircle()
	openDay := gshare.GetOpenServerDay()
	idL := jsondata.GetDailyDirGiftAllChargeIds()
	data := s.GetData()

	data.Refs = data.Refs[:0]
	data.DayBuyRefs = make(map[uint32]uint32)
	data.Awards = data.Awards[:0]

	for _, id := range idL {
		confL := jsondata.GetDailyDirGiftByChargeId(id)
		if len(confL) == 0 {
			continue
		}

		checkCircle := func(minCircle, maxCircle uint32) bool {
			if circle >= minCircle && circle <= maxCircle {
				return true
			}
			return false
		}
		checkOpenDay := func(minDay, maxDay uint32) bool {
			if openDay >= minDay && openDay <= maxDay {
				return true
			}
			return false
		}

		var conf *jsondata.DailyDirGiftConf
		for _, v := range confL {
			isCircle := true
			if len(v.Circle) >= 2 {
				isCircle = checkCircle(v.Circle[0], v.Circle[1])
			}

			isOpenDay := true
			if len(v.OpenDays) >= 2 {
				isOpenDay = checkOpenDay(v.OpenDays[0], v.OpenDays[1])
			}

			if isCircle && isOpenDay {
				conf = v
			}
		}
		if conf != nil {
			data.Refs = append(data.Refs, conf.Ref)
		}
	}
}

func (s *DailyDirGiftSys) S2CInfo() {
	data := s.GetData()
	msg := &pb3.S2C_127_20{
		Refs:              data.Refs,
		DayBuyRefs:        data.DayBuyRefs,
		Rewards:           data.Awards,
		ReceiveReachIds:   data.ReceiveReachIds,
		ReachChargeAmount: data.ReachChargeAmount,
		CurIdx:            data.CurIdx,
	}
	s.SendProto3(127, 20, msg)
}

func (s *DailyDirGiftSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_127_21
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	conf := jsondata.GetDailyDirGift(req.Ref)
	if conf == nil {
		return neterror.ConfNotFoundError("dailyDirectGift ref=%d config not found", req.Ref)
	}

	data := s.GetData()
	if !utils.SliceContainsUint32(data.Refs, req.Ref) {
		return neterror.ParamsInvalidError("dailyDirectGift ref=%d not exit", req.Ref)
	}

	// 次数检查
	count, ok := data.DayBuyRefs[req.Ref]
	if ok && count >= conf.CountLimit {
		return neterror.ParamsInvalidError("dailyDirectGift ref=%d buy countLimit", req.Ref)
	}

	// 一次性购买
	if conf.Type == BuyType3 {
		if !s.checkCanOneKeyBuy() {
			return neterror.ParamsInvalidError("dailyDirectGift ref=%d buy error", req.Ref)
		}
	}

	// 免费礼包直接发奖励
	if conf.Type == BuyType1 {
		data.DayBuyRefs[req.Ref] += 1

		var awards jsondata.StdRewardVec
		awards = append(awards, conf.FixRewards...)
		if conf.ExtraRate > 0 && conf.ExtraRate >= random.IntervalUU(1, CoeRate) {
			awards = append(awards, conf.ExtraRewards...)
		}
		if len(awards) > 0 {
			engine.GiveRewards(s.GetOwner(), awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyDirGiftBuyFreeReward})
		}
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogDailyDirGiftBuyFreeReward, &pb3.LogPlayerCounter{
			NumArgs: uint64(BuyType1),
			StrArgs: utils.I32toa(req.Ref),
		})
		s.S2CInfo()
		s.SendProto3(127, 23, &pb3.S2C_127_23{
			Rewards: jsondata.StdRewardVecToPb3RewardVec(awards),
		})
	} else {
		s.ref = req.Ref
		s.SendProto3(127, 21, &pb3.S2C_127_21{
			Ref: req.Ref,
		})
	}
	return nil
}

// 检查是否可以一键购买
// 只要有一个付费礼包购买过，就不可以付费购买
func (s *DailyDirGiftSys) checkCanOneKeyBuy() bool {
	data := s.GetData()
	for ref, count := range data.DayBuyRefs {
		conf := jsondata.GetDailyDirGift(ref)
		if conf.Type == BuyType2 && count > 0 {
			return false
		}
	}
	return true
}

func (s *DailyDirGiftSys) c2sReceiveReachAwards(msg *base.Message) error {
	var req pb3.C2S_127_24
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed")
	}

	data := s.GetData()
	mgr := jsondata.GetDailyDirGiftReachConfMgr()
	if mgr == nil {
		return neterror.ConfNotFoundError("not found GetDailyDirGiftReachConf")
	}

	if uint32(len(mgr)) < data.CurIdx {
		return neterror.ParamsInvalidError("idx is overflow %d", data.CurIdx)
	}
	reachConf := mgr[data.CurIdx-1]

	if req.Id > uint32(len(reachConf.DailyReach)) {
		return neterror.ParamsInvalidError("reach conf not enough")
	}

	id := req.Id
	if id == 0 {
		return neterror.ParamsInvalidError("id is zero")
	}

	if pie.Uint32s(data.ReceiveReachIds).Contains(id) {
		return neterror.ParamsInvalidError("already reach idx %d awards", id)
	}

	conf := reachConf.DailyReach[req.Id-1]
	if data.ReachChargeAmount < conf.Amount {
		return neterror.ParamsInvalidError("reach charge amount not enough receive")
	}

	data.ReceiveReachIds = append(data.ReceiveReachIds, req.Id)
	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetOwner(), conf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogReceiveDailyDirGiftReachReward,
		})
	}

	s.SendProto3(127, 24, &pb3.S2C_127_24{
		Id: req.Id,
	})
	return nil
}

func (s *DailyDirGiftSys) tryCheckOutReachAwardsAndRefresh() {
	data := s.GetData()
	mgr := jsondata.GetDailyDirGiftReachConfMgr()
	if data.CurIdx > uint32(len(mgr)) {
		return
	}
	if data.CurIdx == 0 {
		s.LogWarn("cur idx is zero")
		return
	}
	conf := mgr[data.CurIdx-1]

	var canReset = len(data.ReceiveReachIds) == len(conf.DailyReach)
	var sendEmailAwards jsondata.StdRewardVec
	if !canReset {
		for _, reach := range conf.DailyReach {
			if pie.Uint32s(data.ReceiveReachIds).Contains(reach.Id) {
				continue
			}
			if data.ReachChargeAmount > reach.Amount {
				sendEmailAwards = append(sendEmailAwards, reach.Rewards...)
				data.ReceiveReachIds = append(data.ReceiveReachIds, reach.Id)
			}
		}

		if len(sendEmailAwards) > 0 {
			s.owner.SendMail(&mailargs.SendMailSt{
				ConfId:  common.Mail_DailyDirGiftReachAwardsUnReceive,
				Rewards: sendEmailAwards,
			})
		}
		canReset = len(data.ReceiveReachIds) == len(conf.DailyReach)
	}

	if !canReset {
		return
	}

	if data.CurIdx+1 <= uint32(len(mgr)) {
		data.CurIdx = data.CurIdx + 1
	}
	data.ReceiveReachIds = nil
	data.ReachChargeAmount = 0
}

func dailyDirGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	obj := player.GetSysObj(sysdef.SiDailyDirectGift)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	sys, ok := obj.(*DailyDirGiftSys)
	if !ok {
		return false
	}
	if sys.ref == 0 {
		return false
	}
	gConf := jsondata.GetDailyDirGift(sys.ref)
	if gConf == nil {
		return false
	}

	// 检查购买次数
	data := sys.GetData()
	count := data.DayBuyRefs[sys.ref]
	if count >= gConf.CountLimit {
		return false
	}

	// 一次性购买检查
	if gConf.Type == BuyType3 {
		return sys.checkCanOneKeyBuy()
	}
	return true
}

func dailyDirGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	obj := player.GetSysObj(sysdef.SiDailyDirectGift)
	if obj == nil || !obj.IsOpen() {
		return false
	}
	s, ok := obj.(*DailyDirGiftSys)
	if !ok {
		return false
	}

	giftConf := jsondata.GetDailyDirGift(s.ref)
	if giftConf == nil {
		return false
	}

	data := s.GetData()
	var awards jsondata.StdRewardVec

	if giftConf.Type == BuyType3 { // 一次性购买所有付费礼包
		for _, ref := range data.Refs {
			tmpConf := jsondata.GetDailyDirGift(ref)
			if tmpConf.Type != BuyType2 {
				continue
			}
			count := tmpConf.CountLimit
			data.DayBuyRefs[ref] = count
			awards = append(awards, jsondata.StdRewardMulti(tmpConf.FixRewards, int64(count))...)
			// 2024.6.17 需求：一次性购买100%获取红丹
			awards = append(awards, jsondata.StdRewardMulti(tmpConf.ExtraRewards, int64(count))...)
		}
	} else { // 正常购买付费礼包
		data.DayBuyRefs[s.ref] += 1
		awards = append(awards, giftConf.FixRewards...)
		if giftConf.ExtraRate > 0 && giftConf.ExtraRate >= random.IntervalUU(1, CoeRate) {
			awards = append(awards, giftConf.ExtraRewards...)
		}
	}

	// 累加金额
	data.ReachChargeAmount += params.CashCent

	if len(awards) > 0 {
		engine.GiveRewards(s.GetOwner(), awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDailyDirGiftBuyPayReward})
	}

	// 设置每日可领取奖励
	rewards := s.CalcDayRewards()
	data.Awards = jsondata.StdRewardVecToPb3RewardVec(rewards)
	s.ref = 0

	s.S2CInfo()
	s.SendProto3(127, 23, &pb3.S2C_127_23{
		Rewards: jsondata.StdRewardVecToPb3RewardVec(awards),
	})

	s.SendProto3(127, 25, &pb3.S2C_127_25{
		ReachChargeAmount: data.ReachChargeAmount,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogDailyDirGiftBuyPayReward, &pb3.LogPlayerCounter{
		NumArgs: uint64(giftConf.Type),
		StrArgs: utils.I32toa(s.ref),
	})
	return true
}

func init() {
	RegisterSysClass(sysdef.SiDailyDirectGift, func() iface.ISystem {
		return &DailyDirGiftSys{}
	})

	engine.RegChargeEvent(chargedef.DailyDirGift, dailyDirGiftCheck, dailyDirGiftChargeBack)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		if s, ok := player.GetSysObj(sysdef.SiDailyDirectGift).(*DailyDirGiftSys); ok && s.IsOpen() {
			s.OnNewDay()
		}
	})

	net.RegisterSysProtoV2(127, 21, sysdef.SiDailyDirectGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DailyDirGiftSys).c2sBuy
	})

	net.RegisterSysProtoV2(127, 24, sysdef.SiDailyDirectGift, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DailyDirGiftSys).c2sReceiveReachAwards
	})

	gmevent.Register("dailydirgift.buy", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(127<<8 | 21)
		err := msg.PackPb3Msg(&pb3.C2S_127_21{
			Ref: utils.AtoUint32(args[0]),
		})
		if err != nil {
			player.LogError(err.Error())
		}
		player.DoNetMsg(127, 21, msg)
		return true
	}, 1)

	gmevent.Register("dailydirgift.zero", func(player iface.IPlayer, args ...string) bool {
		if s, ok := player.GetSysObj(sysdef.SiDailyDirectGift).(*DailyDirGiftSys); ok && s.IsOpen() {
			s.OnNewDay()
			return true
		}
		return false
	}, 1)
}
