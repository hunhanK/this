/**
 * @Author: yzh
 * @Desc: 幸运翻牌
 * @Modify： ChenJunJi
 * @Date:
 */

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type LuckyCardSys struct {
	PlayerYYBase
}

func (s *LuckyCardSys) OnOpen() {
	s.sendState()
}

func (s *LuckyCardSys) OnAfterLogin() {
	s.addMonExtraDrops()
	s.sendState()
}

func (s *LuckyCardSys) OnReconnect() {
	s.sendState()
}

func (s *LuckyCardSys) OnEnd() {
	s.delMonExtraDrops()
}

func (s *LuckyCardSys) OnLoginFight() {
	s.addMonExtraDrops()
}

func (s *LuckyCardSys) addMonExtraDrops() {
	conf := jsondata.GetYYLuckyCardConf(s.ConfName, s.ConfIdx)
	for _, mon := range conf.Monsters {
		s.GetPlayer().CallActorFunc(actorfuncid.AddMonExtraDrops, &pb3.AddActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
	}
}

func (s *LuckyCardSys) delMonExtraDrops() {
	conf := jsondata.GetYYLuckyCardConf(s.ConfName, s.ConfIdx)
	for _, mon := range conf.Monsters {
		s.GetPlayer().CallActorFunc(actorfuncid.DelMonExtraDrops, &pb3.DelActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
	}
}

func (s *LuckyCardSys) ResetData() {
	yyData := s.GetYYData()
	if yyData.Activity2RevCardStateMap == nil {
		return
	}
	delete(yyData.Activity2RevCardStateMap, s.Id)
}

func (s *LuckyCardSys) GetData() *pb3.YYLuckyCardRevState {
	yyData := s.GetYYData()
	if yyData.Activity2RevCardStateMap == nil {
		yyData.Activity2RevCardStateMap = make(map[uint32]*pb3.YYLuckyCardRevState)
	}
	if yyData.Activity2RevCardStateMap[s.Id] == nil {
		yyData.Activity2RevCardStateMap[s.Id] = &pb3.YYLuckyCardRevState{}
	}
	if yyData.Activity2RevCardStateMap[s.Id].RevPosCardMap == nil {
		yyData.Activity2RevCardStateMap[s.Id].RevPosCardMap = map[uint32]uint32{}
	}
	return yyData.Activity2RevCardStateMap[s.Id]
}

func (s *LuckyCardSys) sendState() {
	s.SendProto3(143, 2, &pb3.S2C_143_2{
		State:      s.GetData(),
		ActivityId: s.Id,
	})
}

func (s *LuckyCardSys) DrawCard(pos uint32) {
	conf := jsondata.GetYYLuckyCardConf(s.ConfName, s.ConfIdx)
	state := s.GetData()
	drawTimes := len(state.RevPosCardMap)

	cardPool := conf.CardPool
	if nil == cardPool {
		return
	}
	if drawTimes >= len(cardPool.Consume) {
		s.GetPlayer().LogWarn("draw time over max")
		return
	}

	if val, ok := state.RevPosCardMap[pos]; ok {
		s.GetPlayer().LogWarn("pos %d,val is %d already draw card", pos, val)
		return
	}

	consume := cardPool.Consume[drawTimes]
	if !s.GetPlayer().ConsumeByConf([]*jsondata.Consume{consume}, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeDrawLuckyCard}) {
		s.GetPlayer().LogWarn("not enough consume")
		return
	}

	existRewardIdxes := map[uint32]struct{}{}
	for _, idx := range state.RevPosCardMap {
		existRewardIdxes[idx] = struct{}{}
	}

	weightPool := new(random.Pool)
	for idx, reward := range cardPool.Rewards {
		if _, ok := existRewardIdxes[uint32(idx)]; ok {
			continue
		}
		weightPool.AddItem(idx, reward.Weight)
	}

	idx := weightPool.RandomOne().(int)
	rewardConf := cardPool.Rewards[idx]

	state.RevPosCardMap[pos] = uint32(idx)

	engine.GiveRewards(s.GetPlayer(), []*jsondata.StdReward{&rewardConf.StdReward}, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogAwardDrawLuckyCard,
	})

	if rewardConf.IsAdvance == 1 && conf.AdvanceCardBroadcastId > 0 {
		itemConf := jsondata.GetItemConfig(rewardConf.Id)
		if itemConf != nil {
			engine.BroadcastTipMsgById(conf.AdvanceCardBroadcastId, s.GetPlayer().GetName(), itemConf.Name)
		}
	}

	s.SendProto3(143, 1, &pb3.S2C_143_1{
		Pos:             pos,
		CardPoolItemIdx: uint32(idx),
		ActivityId:      s.Id,
	})
}

func (s *LuckyCardSys) c2sDrawCard(msg *base.Message) error {
	var req pb3.C2S_143_1
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}
	s.DrawCard(req.Pos)
	return nil
}

func c2sDrawLuckyCard(sys iface.IPlayerYY) func(msg *base.Message) error {
	return sys.(*LuckyCardSys).c2sDrawCard
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYSiLuckyCard, func() iface.IPlayerYY {
		return &LuckyCardSys{}
	})

	net.RegisterYYSysProtoV2(143, 1, c2sDrawLuckyCard)
}
