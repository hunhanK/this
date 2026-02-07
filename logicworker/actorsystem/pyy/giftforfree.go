/**
 * @Author: LvYuMeng
 * @Date: 2024/7/29
 * @Desc: 极品白送
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type GiftForFreeSys struct {
	PlayerYYBase
}

func (s *GiftForFreeSys) ResetData() {
	state := s.GetYYData()
	if nil == state.GiftForFree {
		return
	}
	delete(state.GiftForFree, s.Id)
}

func (s *GiftForFreeSys) data() *pb3.PYY_GiftForFree {
	state := s.GetYYData()
	if nil == state.GiftForFree {
		state.GiftForFree = make(map[uint32]*pb3.PYY_GiftForFree)
	}
	if nil == state.GiftForFree[s.Id] {
		state.GiftForFree[s.Id] = &pb3.PYY_GiftForFree{}
	}
	if nil == state.GiftForFree[s.Id].Gift {
		state.GiftForFree[s.Id].Gift = make(map[uint32]*pb3.GiftForFree)
	}
	return state.GiftForFree[s.Id]
}

func (s *GiftForFreeSys) getGiftDataById(id uint32) (*pb3.GiftForFree, error) {
	if id == 0 {
		return nil, neterror.ParamsInvalidError("id is invalid")
	}
	data := s.data()
	if _, ok := data.Gift[id]; !ok {
		data.Gift[id] = &pb3.GiftForFree{}
	}
	return data.Gift[id], nil
}

func (s *GiftForFreeSys) Login() {
	s.s2cInfo()
}

func (s *GiftForFreeSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GiftForFreeSys) OnEnd() {
	s.sendReward()
}

func (s *GiftForFreeSys) canReceive(g *pb3.GiftForFree) bool {
	if nil == g {
		return false
	}
	return g.BuyTimes > g.ReviceTimes
}

func (s *GiftForFreeSys) sendReward() {
	conf, ok := jsondata.GetPYYGiftForFreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.data()
	var rewardsVec jsondata.StdRewardVec
	for id, g := range data.Gift {
		gConf, ok := s.getGiftConf(id)
		if !ok {
			continue
		}
		if g.BuyTimes <= g.ReviceTimes {
			continue
		}
		remains := g.BuyTimes - g.ReviceTimes
		rewards, err := gConf.PacketReward(g)
		if nil != err {
			s.LogError("rewards not found err:%v", err)
			continue
		}
		if remains > 1 {
			rewards = jsondata.StdRewardMulti(rewards, int64(remains))
		}
		rewardsVec = append(rewardsVec, rewards...)
		g.ReviceTimes = g.BuyTimes
	}
	if len(rewardsVec) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewardsVec,
		})
	}
}

func (s *GiftForFreeSys) s2cInfo() {
	s.SendProto3(75, 25, &pb3.S2C_75_25{
		ActiveId: s.GetId(),
		Data:     s.data(),
	})
}

func (s *GiftForFreeSys) OnOpen() {
	s.s2cInfo()
}

func (s *GiftForFreeSys) c2sChoose(msg *base.Message) error {
	var req pb3.C2S_75_27
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	giftId := req.GetGiftId()

	giftConf, ok := s.getGiftConf(giftId)
	if !ok {
		return neterror.ConfNotFoundError("gift conf %d is nil", giftId)
	}

	refIds := pie.Uint32s(req.RefIds).Unique()
	if len(refIds) > 0 && len(refIds) != int(giftConf.ChooseCount) {
		return neterror.ConfNotFoundError("ids count not enough")
	}

	for _, refId := range refIds {
		if _, ok := giftConf.Choose[refId]; !ok {
			return neterror.ConfNotFoundError("gift refId conf %d is nil", refId)
		}
	}

	g, err := s.getGiftDataById(giftId)
	if nil != err {
		return err
	}

	if s.canReceive(g) {
		return neterror.ParamsInvalidError("need receive before")
	}

	if g.BuyTimes >= giftConf.BuyCount {
		return neterror.ParamsInvalidError("buy limit")
	}

	g.RefIds = refIds
	s.SendProto3(75, 27, &pb3.S2C_75_27{
		ActiveId: s.GetId(),
		GiftId:   giftId,
		RefIds:   g.RefIds,
	})

	return nil
}

func (s *GiftForFreeSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_75_28
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	giftId := req.GetGiftId()
	giftConf, ok := s.getGiftConf(giftId)
	if !ok {
		return neterror.ConfNotFoundError("gift conf %d is nil", giftId)
	}

	g, err := s.getGiftDataById(giftId)
	if nil != err {
		return err
	}

	if !s.canReceive(g) {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	rewards, err := giftConf.PacketReward(g)
	if nil != err {
		return err
	}

	g.ReviceTimes++

	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogGiftForFreeAward,
	})

	s.SendProto3(75, 28, &pb3.S2C_75_28{
		ActiveId:     s.GetId(),
		GiftId:       giftId,
		ReceiveTimes: g.ReviceTimes,
	})

	return nil
}

func (s *GiftForFreeSys) getGiftConf(chargeId uint32) (*jsondata.GiftForFreeGiftConf, bool) {
	conf, ok := jsondata.GetPYYGiftForFreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil, false
	}
	giftConf, ok := conf.Gifts[chargeId]
	return giftConf, ok
}

func (s *GiftForFreeSys) chargeCheck(chargeId uint32) bool {
	giftConf, ok := s.getGiftConf(chargeId)
	if !ok {
		return false
	}

	g, err := s.getGiftDataById(chargeId)
	if nil != err {
		return false
	}

	if g.BuyTimes >= giftConf.BuyCount {
		return false
	}

	if s.canReceive(g) {
		return false
	}
	return true
}

func (s *GiftForFreeSys) chargeBack(chargeId uint32) bool {
	giftConf, ok := s.getGiftConf(chargeId)
	if !ok {
		return false
	}

	g, err := s.getGiftDataById(giftConf.ChargeId)
	if nil != err {
		return false
	}

	if giftConf.BuyCount <= g.BuyTimes {
		return false
	}

	g.BuyTimes++

	s.SendProto3(75, 26, &pb3.S2C_75_26{
		ActiveId: s.GetId(),
		GiftId:   chargeId,
		BuyTimes: g.BuyTimes,
	})
	return false
}

const (
	DayLimitGiftType = 2
)

func (s *GiftForFreeSys) NewDay() {
	conf, ok := jsondata.GetPYYGiftForFreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.data()
	// 先补发奖励
	var rewardsVec jsondata.StdRewardVec
	for giftId, g := range data.Gift {
		giftConf, ok := s.getGiftConf(giftId)
		if !ok {
			continue
		}

		if giftConf.BuyType != DayLimitGiftType {
			continue
		}

		if g.BuyTimes <= g.ReviceTimes {
			continue
		}

		remains := g.BuyTimes - g.ReviceTimes
		rewards, err := giftConf.PacketReward(g)
		if nil != err {
			s.LogError("rewards not found err:%v", err)
			continue
		}
		if remains > 1 {
			rewards = jsondata.StdRewardMulti(rewards, int64(remains))
		}
		rewardsVec = append(rewardsVec, rewards...)
		g.BuyTimes = 0
		g.ReviceTimes = 0
	}

	if len(rewardsVec) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewardsVec,
		})
	}
	s.s2cInfo()
}

func giftForFreeChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYGiftForFree)
	for _, obj := range yyObjs {
		if s, ok := obj.(*GiftForFreeSys); ok && s.IsOpen() {
			if s.chargeCheck(conf.ChargeId) {
				return true
			}
		}
	}
	return true
}

func giftForFreeChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	yyObjs := player.GetPYYObjList(yydefine.PYYGiftForFree)
	for _, obj := range yyObjs {
		if s, ok := obj.(*GiftForFreeSys); ok && s.IsOpen() {
			if s.chargeBack(conf.ChargeId) {
				return true
			}
		}
	}
	return true
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYGiftForFree, func() iface.IPlayerYY {
		return &GiftForFreeSys{}
	})

	net.RegisterYYSysProtoV2(75, 27, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*GiftForFreeSys).c2sChoose
	})
	net.RegisterYYSysProtoV2(75, 28, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*GiftForFreeSys).c2sRev
	})

	engine.RegChargeEvent(chargedef.GiftForFree, giftForFreeChargeCheck, giftForFreeChargeBack)
}
