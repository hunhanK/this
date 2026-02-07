/**
 * @Author: zjj
 * @Date: 2024/8/2
 * @Desc: 开服庆典-消费豪礼
**/

package pyy

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type ConsumeLuxuryGiftsSys struct {
	*PlayerYYBase
}

func (s *ConsumeLuxuryGiftsSys) ResetData() {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.ConsumeLuxuryGiftsMap {
		return
	}
	delete(state.ConsumeLuxuryGiftsMap, s.Id)
}

func (s *ConsumeLuxuryGiftsSys) GetData() *pb3.PYYConsumeLuxuryGifts {
	state := s.GetPlayer().GetBinaryData().GetYyData()
	if nil == state.ConsumeLuxuryGiftsMap {
		state.ConsumeLuxuryGiftsMap = make(map[uint32]*pb3.PYYConsumeLuxuryGifts)
	}
	if state.ConsumeLuxuryGiftsMap[s.Id] == nil {
		state.ConsumeLuxuryGiftsMap[s.Id] = &pb3.PYYConsumeLuxuryGifts{}
	}
	if state.ConsumeLuxuryGiftsMap[s.Id].CountMap == nil {
		state.ConsumeLuxuryGiftsMap[s.Id].CountMap = make(map[uint32]int64)
	}
	return state.ConsumeLuxuryGiftsMap[s.Id]
}

func (s *ConsumeLuxuryGiftsSys) S2CInfo() {
	s.SendProto3(61, 40, &pb3.S2C_61_40{
		ActiveId: s.Id,
		Data:     s.GetData(),
	})
}

func (s *ConsumeLuxuryGiftsSys) OnOpen() {
	s.S2CInfo()
}

func (s *ConsumeLuxuryGiftsSys) Login() {
	s.checkNextRound()
	s.S2CInfo()
}
func (s *ConsumeLuxuryGiftsSys) OnReconnect() {
	s.S2CInfo()
}

func (s *ConsumeLuxuryGiftsSys) OnEnd() {
	conf, ok := jsondata.GetPYYConsumeLuxuryGiftsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.GetData()
	var totalCount int64
	for _, count := range data.CountMap {
		totalCount += count
	}

	var totalAwards jsondata.StdRewardVec
	flag := data.ReceiveFlag
	for _, reachAwards := range conf.ReachAwards {
		if utils.IsSetBit(flag, reachAwards.Idx) {
			continue
		}
		if totalCount < reachAwards.MinMoney {
			continue
		}
		flag = utils.SetBit(flag, reachAwards.Idx)
		totalAwards = append(totalAwards, reachAwards.Awards...)
	}

	data.ReceiveFlag = flag
	if len(totalAwards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_ConsumeLuxuryGiftsGiveMailAwards,
			Rewards: totalAwards,
		})
	}
}

func (s *ConsumeLuxuryGiftsSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_61_42
	if err := msg.UnpackagePbmsg(&req); err != nil {
		s.GetPlayer().LogError(err.Error())
		return err
	}

	conf, ok := jsondata.GetPYYConsumeLuxuryGiftsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	idx := req.Idx
	data := s.GetData()
	var totalCount int64
	for _, count := range data.CountMap {
		totalCount += count
	}

	var totalAwards jsondata.StdRewardVec
	if idx > 0 {
		reachAwards := conf.ReachAwards[idx]
		if reachAwards == nil {
			return neterror.ConfNotFoundError("%s %d not found conf", s.GetPrefix(), idx)
		}

		flag := data.ReceiveFlag
		if utils.IsSetBit(flag, idx) {
			return neterror.ParamsInvalidError("%s %d already rec", s.GetPrefix(), idx)
		}
		if totalCount < reachAwards.MinMoney {
			return neterror.ConfNotFoundError("%s %d not reach %d < %d", s.GetPrefix(), idx, totalCount, reachAwards.MinMoney)
		}
		data.ReceiveFlag = utils.SetBit(flag, idx)
		totalAwards = append(totalAwards, reachAwards.Awards...)
	} else {
		if !req.QuickRec {
			return neterror.ParamsInvalidError("not quick rec")
		}
		newFlag := data.ReceiveFlag
		for _, reachAwards := range conf.ReachAwards {
			if utils.IsSetBit(newFlag, reachAwards.Idx) {
				continue
			}
			if totalCount < reachAwards.MinMoney {
				continue
			}
			newFlag = utils.SetBit(newFlag, reachAwards.Idx)
			totalAwards = append(totalAwards, reachAwards.Awards...)
		}
		if newFlag == data.ReceiveFlag {
			return neterror.ParamsInvalidError("not can rec awards")
		}
		data.ReceiveFlag = newFlag
	}

	if len(totalAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), totalAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogConsumeLuxuryGiftsRecAwards,
		})
	}

	s.SendProto3(61, 42, &pb3.S2C_61_42{
		ActiveId:    s.Id,
		Idx:         idx,
		ReceiveFlag: data.ReceiveFlag,
	})
	s.checkNextRound()
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogConsumeLuxuryGiftsRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", idx, data.ReceiveFlag),
	})
	return nil
}

func (s *ConsumeLuxuryGiftsSys) handleAeConsumeMoney(mt uint32, count int64) {
	conf, ok := jsondata.GetPYYConsumeLuxuryGiftsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var exist bool
	for _, mtConf := range conf.Mts {
		if mt != mtConf {
			continue
		}
		exist = true
		break
	}
	if !exist {
		return
	}
	data := s.GetData()
	data.CountMap[mt] += count
	s.SendProto3(61, 41, &pb3.S2C_61_41{
		ActiveId: s.Id,
		CountMap: data.CountMap,
	})
}

func (s *ConsumeLuxuryGiftsSys) checkRound() {
	conf, ok := jsondata.GetPYYConsumeLuxuryGiftsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if s.GetData().CurRound == 0 && conf.MaxRound != 0 {
		s.GetData().CurRound = 1
	}
}

func (s *ConsumeLuxuryGiftsSys) checkNextRound() {
	conf, ok := jsondata.GetPYYConsumeLuxuryGiftsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if conf.MaxRound == 0 {
		return
	}
	data := s.GetData()
	if data.CurRound+1 > conf.MaxRound {
		return
	}
	var maxMoney int64
	for _, reachAwards := range conf.ReachAwards {
		if utils.IsSetBit(data.ReceiveFlag, reachAwards.Idx) {
			if maxMoney < reachAwards.MinMoney {
				maxMoney = reachAwards.MinMoney
			}
			continue
		}
		return
	}
	data.CurRound++
	data.ReceiveFlag = 0
	for _, mt := range conf.Mts {
		count := data.CountMap[mt]
		if count > maxMoney {
			data.CountMap[mt] = count - maxMoney
		} else {
			data.CountMap[mt] = 0
		}
	}
	s.SendProto3(61, 43, &pb3.S2C_61_43{
		ActiveId: s.GetId(),
		Data:     data,
	})
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogConsumeLuxuryGiftsNextRound, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d", data.CurRound),
	})
}

func rangeConsumeLuxuryGiftsSys(player iface.IPlayer, doLogic func(yy iface.IPlayerYY)) {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.PyyConsumeLuxuryGifts)
	if len(yyList) == 0 {
		player.LogWarn("not found yy obj, id is %d", yydefine.PyyConsumeLuxuryGifts)
		return
	}
	for i := range yyList {
		v := yyList[i]
		doLogic(v)
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PyyConsumeLuxuryGifts, func() iface.IPlayerYY {
		return &ConsumeLuxuryGiftsSys{
			PlayerYYBase: &PlayerYYBase{},
		}
	})
	event.RegActorEvent(custom_id.AeConsumeMoney, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 2 {
			return
		}
		mt, ok := args[0].(uint32)
		if !ok {
			return
		}
		count, ok := args[1].(int64)
		if !ok {
			return
		}
		if mt != moneydef.Diamonds {
			return
		}
		rangeConsumeLuxuryGiftsSys(player, func(yy iface.IPlayerYY) {
			s := yy.(*ConsumeLuxuryGiftsSys)
			s.handleAeConsumeMoney(mt, count)
		})
	})
	net.RegisterYYSysProtoV2(61, 42, func(sys iface.IPlayerYY) func(*base.Message) error {
		return sys.(*ConsumeLuxuryGiftsSys).c2sAward
	})
}
