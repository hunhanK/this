/**
 * @Author:
 * @Date:
 * @Desc:
**/

package pyy

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type WeekConsumeGiftSys struct {
	PlayerYYBase
}

func (s *WeekConsumeGiftSys) s2cInfo() {
	s.SendProto3(8, 110, &pb3.S2C_8_110{
		ActiveId: s.Id,
		Data:     s.getData(),
	})
}

func (s *WeekConsumeGiftSys) getData() *pb3.PYYWeekConsumeGiftData {
	state := s.GetYYData()
	if nil == state.WeekConsumeGiftData {
		state.WeekConsumeGiftData = make(map[uint32]*pb3.PYYWeekConsumeGiftData)
	}
	if state.WeekConsumeGiftData[s.Id] == nil {
		state.WeekConsumeGiftData[s.Id] = &pb3.PYYWeekConsumeGiftData{}
	}
	if state.WeekConsumeGiftData[s.Id].IdxBuyTimes == nil {
		state.WeekConsumeGiftData[s.Id].IdxBuyTimes = make(map[uint32]uint32)
	}
	return state.WeekConsumeGiftData[s.Id]
}

func (s *WeekConsumeGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.WeekConsumeGiftData {
		return
	}
	delete(state.WeekConsumeGiftData, s.Id)
}

func (s *WeekConsumeGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *WeekConsumeGiftSys) Login() {
	s.s2cInfo()
}

func (s *WeekConsumeGiftSys) OnOpen() {
	s.s2cInfo()
}

func (s *WeekConsumeGiftSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_8_111
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getData()
	times := data.IdxBuyTimes[req.Idx]
	config := jsondata.GetWeekConsumeGiftConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}
	var layerConf *jsondata.WeekConsumeGiftLayer
	for _, layer := range config.Layer {
		if layer.Idx != req.Idx {
			continue
		}
		layerConf = layer
		break
	}
	if layerConf == nil {
		return neterror.ConfNotFoundError("%s not found %d conf", s.GetPrefix(), req.Idx)
	}
	if times >= layerConf.Times {
		return neterror.ParamsInvalidError("%s %d already buy %d", s.GetPrefix(), req.Idx, times)
	}
	player := s.GetPlayer()
	if len(layerConf.Consume) == 0 || !player.ConsumeByConf(layerConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPYYWeekConsumeGiftBuy}) {
		return neterror.ConsumeFailedError("%s %d buy failed", s.GetPrefix(), req.Idx)
	}
	data.IdxBuyTimes[req.Idx]++
	s.SendProto3(8, 111, &pb3.S2C_8_111{
		ActiveId: s.Id,
		Idx:      req.Idx,
		Times:    data.IdxBuyTimes[req.Idx],
	})
	if len(layerConf.Rewards) > 0 {
		engine.GiveRewards(player, layerConf.Rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogPYYWeekConsumeGiftBuy,
		})
		player.SendShowRewardsPop(layerConf.Rewards)
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogPYYWeekConsumeGiftBuy, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", req.Idx, data.IdxBuyTimes[req.Idx]),
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYWeekConsumeGift, func() iface.IPlayerYY {
		return &WeekConsumeGiftSys{}
	})
	net.RegisterYYSysProtoV2(8, 111, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*WeekConsumeGiftSys).c2sBuy
	})
}
