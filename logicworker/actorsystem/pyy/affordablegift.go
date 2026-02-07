/**
 * @Author: LvYuMeng
 * @Date: 2024/1/29
 * @Desc:
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type AffordableGiftSys struct {
	PlayerYYBase
}

func (s *AffordableGiftSys) GetData() *pb3.PYY_AffordableGift {
	state := s.GetYYData()
	if nil == state.AffordableGift {
		state.AffordableGift = make(map[uint32]*pb3.PYY_AffordableGift)
	}
	if state.AffordableGift[s.Id] == nil {
		state.AffordableGift[s.Id] = &pb3.PYY_AffordableGift{}
	}
	if nil == state.AffordableGift[s.Id].Gift {
		state.AffordableGift[s.Id].Gift = make(map[uint32]uint32)
	}
	return state.AffordableGift[s.Id]
}

func (s *AffordableGiftSys) GlobalRecord() *pb3.AffordableGiftBuyRecord {
	globalVar := gshare.GetStaticVar()
	if globalVar.PyyDatas == nil {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.AffordableGiftBuyRecords == nil {
		globalVar.PyyDatas.AffordableGiftBuyRecords = make(map[uint32]*pb3.AffordableGiftBuyRecord)
	}
	if globalVar.PyyDatas.AffordableGiftBuyRecords[s.Id] == nil {
		globalVar.PyyDatas.AffordableGiftBuyRecords[s.Id] = &pb3.AffordableGiftBuyRecord{}
	}
	return globalVar.PyyDatas.AffordableGiftBuyRecords[s.Id]
}

func (s *AffordableGiftSys) GetGuildBuyCount(guildId uint64) *pb3.AffordableGiftBuyCountRecord {
	globalRecord := s.GlobalRecord()
	if globalRecord.GuildBuy == nil {
		globalRecord.GuildBuy = make(map[uint64]*pb3.AffordableGiftBuyCountRecord)
	}
	guildBuy, ok := globalRecord.GuildBuy[guildId]
	if !ok {
		globalRecord.GuildBuy[guildId] = &pb3.AffordableGiftBuyCountRecord{}
		guildBuy = globalRecord.GuildBuy[guildId]
	}

	if guildBuy.BuyCount == nil {
		guildBuy.BuyCount = make(map[uint32]uint32)
	}

	return guildBuy
}

func (s *AffordableGiftSys) Login() {
	s.checkBindAct()
	s.s2cInfo()
}

func (s *AffordableGiftSys) OnReconnect() {
	s.s2cInfo()
}

func (s *AffordableGiftSys) OnOpen() {
	s.checkBindAct()
	s.s2cInfo()
}

func (s *AffordableGiftSys) ResetData() {
	state := s.GetYYData()
	if nil == state.AffordableGift {
		return
	}
	delete(state.AffordableGift, s.Id)

	globalRecord := s.GlobalRecord()
	if globalRecord == nil {
		return
	}
	globalVar := gshare.GetStaticVar()
	delete(globalVar.PyyDatas.AffordableGiftBuyRecords, s.Id)
}

func (s *AffordableGiftSys) OnEnd() {
	conf := jsondata.GetYYAffordableGiftBindActConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	obj := pyymgr.GetPlayerYYObj(s.player, conf.ActId)
	if nil == obj || !obj.IsOpen() {
		return
	}
	actObj, ok := obj.(*DragonDrawSys)
	if !ok {
		s.LogError("act(%d) not dragon draw", conf.ActId)
		return
	}
	actObj.SetHighPoolId(0, true)
}

func (s *AffordableGiftSys) s2cInfo() {
	buyCountRecord := s.GetGuildBuyCount(0)

	s.SendProto3(56, 10, &pb3.S2C_56_10{
		ActiveId: s.Id,
		Gifts:    s.GetData().Gift,
		BuyCount: buyCountRecord.BuyCount,
	})
}

func (s *AffordableGiftSys) checkBindAct() {
	conf := jsondata.GetYYAffordableGiftBindActConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	obj := pyymgr.GetPlayerYYObj(s.player, conf.ActId)
	if nil == obj || !obj.IsOpen() {
		return
	}
	actObj, ok := obj.(*DragonDrawSys)
	if !ok {
		s.LogError("act(%d) not dragon draw", conf.ActId)
		return
	}
	actObj.SendYYStateInfo()
	actObj.SetHighPoolId(conf.PoolId, true)
}

func (s *AffordableGiftSys) c2sRevGift(msg *base.Message) error {
	var req pb3.C2S_56_11
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	gConf := jsondata.GetYYAffordableGiftConf(s.ConfName, s.ConfIdx)
	if nil == gConf {
		return neterror.ConfNotFoundError("affordable gift conf not exist")
	}

	conf := gConf[req.Id]
	if nil == conf {
		return neterror.ConfNotFoundError("affordable gift conf not exist")
	}

	if conf.ChargeID != 0 {
		return neterror.ConfNotFoundError("need affordable gift charge")
	}

	data := s.GetData()
	if data.Gift[conf.ID] >= conf.Count {
		return neterror.ParamsInvalidError("affordable gift rev limit")
	}
	data.Gift[conf.ID]++

	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAffordableGift})
	}

	s.SendProto3(56, 11, &pb3.S2C_56_11{
		ActiveId: s.Id,
		Id:       conf.ID,
		Count:    data.Gift[conf.ID],
	})
	return nil
}

func (s *AffordableGiftSys) chargeCheck(chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetYYAffordableGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return false
	}

	var giftConf *jsondata.AffordableGiftConf
	for _, v := range conf {
		if v.ChargeID != chargeConf.ChargeId {
			if v.BatchCharge == nil || len(v.BatchCharge) == 0 {
				continue
			}
			if _, ok := v.BatchCharge[chargeConf.ChargeId]; !ok {
				continue
			}
		}
		giftConf = v
		break
	}

	if giftConf == nil {
		return false
	}

	// 父礼包购买上限
	curCount := s.GetData().Gift[giftConf.ChargeID]
	if curCount >= giftConf.Count {
		return false
	}

	// 如果充的是子礼包
	if giftConf.ChargeID != chargeConf.ChargeId {
		if giftConf.BatchCharge == nil || len(giftConf.BatchCharge) == 0 {
			return false
		}
		chargeConf, ok := giftConf.BatchCharge[chargeConf.ChargeId]
		if !ok {
			return false
		}
		if chargeConf.Count+curCount > giftConf.Count {
			return false
		}
	}

	return true
}

func (s *AffordableGiftSys) chargeBack(chargeConf *jsondata.ChargeConf) bool {
	if !s.chargeCheck(chargeConf) {
		return false
	}
	gConf := jsondata.GetYYAffordableGiftConf(s.ConfName, s.ConfIdx)
	if nil == gConf {
		s.LogError("affordable gift conf is nil")
		return false
	}

	var giftConf *jsondata.AffordableGiftConf
	for _, v := range gConf {
		if v.ChargeID != chargeConf.ChargeId {
			if v.BatchCharge == nil || len(v.BatchCharge) == 0 {
				continue
			}
			if _, ok := v.BatchCharge[chargeConf.ChargeId]; !ok {
				continue
			}
		}
		giftConf = v
		break
	}

	if nil == giftConf {
		s.LogError("affordable gift charge reward conf(%d) is nil", chargeConf.ChargeId)
		return false
	}

	data := s.GetData()
	if data.Gift[giftConf.ID] >= giftConf.Count {
		s.LogError("buy repeated")
		return false
	}

	var count = uint32(1)
	// 校验是不是买的批量礼包
	if giftConf.ChargeID != chargeConf.ChargeId {
		if giftConf.BatchCharge == nil || len(giftConf.BatchCharge) == 0 {
			s.LogError("player(%d) buy affordable gift(%d) not found batch charge!", s.GetPlayer().GetId(), giftConf.ID)
			return false
		}
		chargeConf, ok := giftConf.BatchCharge[chargeConf.ChargeId]
		if !ok {
			s.LogError("player(%d) buy affordable gift(%d) not found batch charge!", s.GetPlayer().GetId(), giftConf.ID)
			return false
		}
		count = chargeConf.Count
	}

	data.Gift[giftConf.ID] += count

	player := s.GetPlayer()
	if len(giftConf.Rewards) > 0 {
		var rewards = jsondata.StdRewardMulti(giftConf.Rewards, int64(count))
		rewards = jsondata.MergeStdReward(rewards)
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogAffordableGift})
		player.SendShowRewardsPop(rewards)
	}

	// 记录全服购买
	buyCountRecord := s.GetGuildBuyCount(0)
	buyCountRecord.BuyCount[giftConf.ID] += 1

	// 记录仙盟购买
	guildId := player.GetGuildId()
	if guildId > 0 {
		buyCountRecord := s.GetGuildBuyCount(guildId)
		buyCountRecord.BuyCount[giftConf.ID] += 1
	}

	engine.BroadcastTipMsgById(giftConf.BroadcastId, player.GetId(), player.GetName(), giftConf.Name, engine.StdRewardToBroadcast(player, giftConf.Rewards))

	s.SendProto3(56, 11, &pb3.S2C_56_11{
		ActiveId: s.Id,
		Id:       giftConf.ID,
		Count:    data.Gift[giftConf.ID],
	})
	s.s2cInfo()
	return true
}

func (s *AffordableGiftSys) NewDay() {
	if !s.IsOpen() {
		s.GetPlayer().LogWarn("not open")
		return
	}
	conf := jsondata.GetYYAffordableGiftConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	data := s.GetData()
	for _, v := range conf {
		if v.Reset {
			delete(data.Gift, s.Id)
		}
	}
	s.s2cInfo()
}

func checkBindActOpen(actor iface.IPlayer, args ...interface{}) {
	id, ok := args[0].(uint32)
	if !ok {
		return
	}
	conf := jsondata.GetPlayerYYConf(id)
	if nil == conf || conf.Class != yydefine.YYDragonDraw {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(actor, yydefine.YYAffordableGift)
	for _, obj := range yyList {
		sys, ok := obj.(*AffordableGiftSys)
		if !ok || !sys.IsOpen() {
			continue
		}
		sys.checkBindAct()
	}
}

func affordableGiftChargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYAffordableGift)
	for _, obj := range yyList {
		if s, ok := obj.(*AffordableGiftSys); ok && s.IsOpen() {
			if s.chargeCheck(conf) {
				return true
			}
		}
	}
	return false
}

func affordableGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYAffordableGift)
	for _, obj := range yyList {
		if s, ok := obj.(*AffordableGiftSys); ok && s.IsOpen() {
			if s.chargeBack(conf) {
				return true
			}
		}
	}
	return false
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYAffordableGift, func() iface.IPlayerYY {
		return &AffordableGiftSys{}
	})
	event.RegActorEvent(custom_id.AePyyOpen, checkBindActOpen)

	net.RegisterYYSysProtoV2(56, 11, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*AffordableGiftSys).c2sRevGift
	})

	engine.RegChargeEvent(chargedef.AffordableGift, affordableGiftChargeCheck, affordableGiftChargeBack)

}
