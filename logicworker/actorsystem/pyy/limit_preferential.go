/**
 * @Author: yzh
 * @Date:
 * @Desc: 限时特惠(达标礼包)
 * @Modify：
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
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
)

type LimitPreferentialSys struct {
	PlayerYYBase
}

func (s *LimitPreferentialSys) Login() {
	s.sendBoughtState()
}

func (s *LimitPreferentialSys) OnReconnect() {
	s.sendBoughtState()
}

func (s *LimitPreferentialSys) ResetData() {
	yyData := s.GetYYData()
	if yyData.ConfIdx2BoughtSuitsMap == nil {
		return
	}
	delete(yyData.ConfIdx2BoughtSuitsMap, s.Id)
}

func (s *LimitPreferentialSys) OnOpen() {
	s.sendBoughtState()
}

func (s *LimitPreferentialSys) OnEnd() {
	conf := jsondata.GetYYLimitReferentialConf(s.ConfName, s.ConfIdx)

	if conf == nil {
		s.GetPlayer().LogWarn("%s not found conf", s.GetPrefix())
		return
	}

	state := s.GetData()
	var rewards []*jsondata.StdReward
	for i, suitConf := range conf.Suits {
		boughtSuit, ok := state.SuitMap[uint32(i)]
		if !ok {
			s.GetPlayer().LogWarn("never buy suit")
			continue
		}

		if boughtSuit.ReceivedRewards {
			s.GetPlayer().LogWarn("already received rewards")
			continue
		}

		if len(boughtSuit.BoughtIdxes) != len(suitConf.LimitBuy) {
			s.GetPlayer().LogWarn("buy not enough")
			continue
		}

		boughtSuit.SuitIdx = uint32(i)
		boughtSuit.ReceivedRewards = true
		rewards = append(rewards, suitConf.Reward...)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.player.GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *LimitPreferentialSys) GetData() *pb3.YYLimitPreferentialLimitBuySuits {
	yyData := s.GetYYData()
	if yyData.ConfIdx2BoughtSuitsMap == nil {
		yyData.ConfIdx2BoughtSuitsMap = make(map[uint32]*pb3.YYLimitPreferentialLimitBuySuits)
	}
	if yyData.ConfIdx2BoughtSuitsMap[s.Id] == nil {
		yyData.ConfIdx2BoughtSuitsMap[s.Id] = &pb3.YYLimitPreferentialLimitBuySuits{}
	}
	if yyData.ConfIdx2BoughtSuitsMap[s.Id].SuitMap == nil {
		yyData.ConfIdx2BoughtSuitsMap[s.Id].SuitMap = make(map[uint32]*pb3.YYLimitPreferentialLimitBuySuit)
	}
	return yyData.ConfIdx2BoughtSuitsMap[s.Id]
}

func (s *LimitPreferentialSys) sendBoughtState() {
	s.SendProto3(131, 1, &pb3.S2C_131_1{
		ActiveId:    s.Id,
		BoughtSuits: s.GetData(),
	})
}

func (s *LimitPreferentialSys) c2sLimitBuy(msg *base.Message) error {
	var req pb3.C2S_131_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	state := s.GetData()
	boughtSuit, ok := state.SuitMap[req.SuitIdx]
	if ok {
		for _, boughtIdx := range boughtSuit.BoughtIdxes {
			if boughtIdx != req.BuyIdx {
				continue
			}

			s.GetPlayer().LogWarn("already bought suit:%d buyIdx:%d", req.SuitIdx, req.BuyIdx)
			return nil
		}
	} else {
		boughtSuit = &pb3.YYLimitPreferentialLimitBuySuit{}
		state.SuitMap[req.SuitIdx] = boughtSuit
	}

	conf := jsondata.GetYYLimitReferentialConf(s.ConfName, s.ConfIdx)
	suitConf := conf.Suits[req.SuitIdx]
	limitBuyConf := suitConf.LimitBuy[req.BuyIdx]

	// 消耗
	if !s.GetPlayer().DeductMoney(limitBuyConf.MoneyType, int64(limitBuyConf.Money), common.ConsumeParams{LogId: pb3.LogId_LogYYLimitPreferentialBuy}) {
		s.GetPlayer().LogWarn("money not enough")
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	// 设置状态
	boughtSuit.BoughtIdxes = append(boughtSuit.BoughtIdxes, req.BuyIdx)
	boughtSuit.SuitIdx = req.SuitIdx

	// 发奖励
	engine.GiveRewards(s.GetPlayer(), limitBuyConf.Reward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYLimitPreferentialReward})

	s.SendProto3(131, 2, &pb3.S2C_131_2{
		ActiveId: s.Id,
		SuitIdx:  req.SuitIdx,
		BuyIdx:   req.BuyIdx,
	})

	s.GetPlayer().TriggerQuestEvent(custom_id.QttBySpecIdLimitPreferentialGift, suitConf.LimitBuyId, 1)
	s.GetPlayer().TriggerQuestEvent(custom_id.QttByAnyLimitPreferentialGift, 0, 1)

	return nil
}

func c2sGetLimitPreferentialRewards(sys iface.IPlayerYY) func(msg *base.Message) error {
	return sys.(*LimitPreferentialSys).c2sGetRewards
}

func c2sLimitBuyPreferential(sys iface.IPlayerYY) func(msg *base.Message) error {
	return sys.(*LimitPreferentialSys).c2sLimitBuy
}

func (s *LimitPreferentialSys) c2sGetRewards(msg *base.Message) error {
	var req pb3.C2S_131_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf := jsondata.GetYYLimitReferentialConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}
	suitConf := conf.Suits[req.SuitIdx]

	state := s.GetData()
	boughtSuit, ok := state.SuitMap[req.SuitIdx]
	if !ok {
		s.GetPlayer().LogWarn("never buy suit")
		return nil
	}

	if boughtSuit.ReceivedRewards {
		s.GetPlayer().LogWarn("already received rewards")
		return nil
	}

	if len(boughtSuit.BoughtIdxes) != len(suitConf.LimitBuy) {
		s.GetPlayer().LogWarn("buy not enough")
		return nil
	}

	engine.GiveRewards(s.GetPlayer(), suitConf.Reward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogVipReward})

	boughtSuit.SuitIdx = req.SuitIdx
	boughtSuit.ReceivedRewards = true

	player := s.GetPlayer()
	if actConf := jsondata.GetPlayerYYConf(s.GetId()); nil != actConf {
		engine.BroadcastTipMsgById(tipmsgid.RankDiscountTip, player.GetId(), player.GetName(), actConf.Name, engine.StdRewardToBroadcast(player, suitConf.Reward))
	}

	s.SendProto3(131, 3, &pb3.S2C_131_3{
		ActiveId: s.Id,
		SuitIdx:  req.SuitIdx,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiLimitReferential, func() iface.IPlayerYY {
		return &LimitPreferentialSys{}
	})

	net.RegisterYYSysProtoV2(131, 1, c2sLimitBuyPreferential)
	net.RegisterYYSysProtoV2(131, 2, c2sGetLimitPreferentialRewards)
}
