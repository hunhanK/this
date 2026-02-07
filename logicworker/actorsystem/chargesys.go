/**
 * @Author: DaiGuanYu
 * @Desc: 充值
 * @Date: 2021/8/31 14:56
 */

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

type ChargeSys struct {
	Base
}

func (sys *ChargeSys) IsOpen() bool {
	return true
}

func (sys *ChargeSys) OnLogin() {
	sys.CheckResetRecharge()
	if binary := sys.GetBinaryData(); nil != binary {
		sys.owner.SetExtraAttr(attrdef.ChargeTokens, attrdef.AttrValueAlias(binary.GetChargeTokens()))
		sys.owner.SetExtraAttr(attrdef.UseChargeItem, attrdef.AttrValueAlias(binary.GetUseChargeItem()))
		sys.owner.SetExtraAttr(attrdef.DitchTokens, attrdef.AttrValueAlias(sys.owner.GetDitchTokens()))
	}
	sys.SendProto3(36, 6, &pb3.S2C_36_6{
		Data: getLifetimeDirectPurchaseData(sys.GetOwner()),
	})
}

func (sys *ChargeSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetChargeInfo() {
		binary.ChargeInfo = &pb3.ChargeInfo{}
	}
	if nil == binary.ChargeInfo.DailyChargeMoneyMap {
		binary.ChargeInfo.DailyChargeMoneyMap = make(map[uint32]uint32)
	}
	if nil == binary.GetRealChargeInfo() {
		binary.RealChargeInfo = &pb3.RealChargeInfo{}
	}
	if nil == binary.RealChargeInfo.DailyChargeMoneyMap {
		binary.RealChargeInfo.DailyChargeMoneyMap = make(map[uint32]uint32)
	}
}

func (sys *ChargeSys) OnReconnect() {
	sys.SendProto3(36, 6, &pb3.S2C_36_6{
		Data: getLifetimeDirectPurchaseData(sys.GetOwner()),
	})
}

func (sys *ChargeSys) checkChargeToken(chargeId uint32) bool {
	if sys.owner.GetExtraAttr(attrdef.UseChargeItem) == 0 {
		return false
	}
	binary := sys.GetBinaryData()

	tokens := binary.GetChargeTokens()
	if tokens <= 0 {
		return false
	}

	conf := jsondata.GetChargeConf(chargeId)
	if nil == conf {
		sys.LogError("charge conf(%d) is nil", chargeId)
		return false
	}

	// 代金券不能购买仙票产品
	if conf.ChargeType == chargedef.DitchToken {
		sys.LogError("chargeType:%d error", conf.ChargeType)
		return false
	}
	if conf.NotUseVouchers {
		sys.LogError("chargeType:%d error", conf.ChargeType)
		return false
	}

	if tokens < conf.CashCent {
		return false
	}

	binary.ChargeTokens = tokens - conf.CashCent
	sys.owner.UpdateStatics(model.FieldChargeTokens_, binary.ChargeTokens)
	sys.owner.SetExtraAttr(attrdef.ChargeTokens, int64(binary.GetChargeTokens()))

	var params = &pb3.OnChargeParams{
		ChargeId:           chargeId,
		CashCent:           conf.CashCent,
		SkipLogFirstCharge: true,
	}
	sys.OnCharge(params, pb3.LogId_LogChargeByUseToken)
	return true
}

func (sys *ChargeSys) checkDitchToken(chargeId uint32) bool {
	tokens := sys.GetOwner().GetDitchTokens()
	if tokens <= 0 {
		return false
	}
	conf := jsondata.GetChargeConf(chargeId)
	if conf == nil {
		sys.LogError("charge conf(%d) is nil", chargeId)
		return false
	}

	// 仙票不能购买仙票产品
	if conf.ChargeType == chargedef.DitchToken {
		sys.LogError("chargeType:%d error", conf.ChargeType)
		return false
	}
	if conf.NotUseDitchToken {
		sys.LogError("chargeType:%d error", conf.ChargeType)
		return false
	}

	configTokens := conf.CashCent / chargedef.ChargeTransfer
	if tokens < configTokens {
		sys.LogError("tokens:%d < configTokens:%d", tokens, configTokens)
		return false
	}

	player := sys.GetOwner()
	player.SubDitchTokens(configTokens)
	player.SetExtraAttr(attrdef.DitchTokens, attrdef.AttrValueAlias(player.GetDitchTokens()))
	player.UpdateStatics(model.FieldDitchTokens_, player.GetDitchTokens())
	player.UpdateStatics(model.FieldHistoryDitchTokens_, player.GetHistoryDitchTokens())

	var params = &pb3.OnChargeParams{
		ChargeId:           chargeId,
		CashCent:           conf.CashCent,
		SkipLogFirstCharge: true,
	}
	sys.OnCharge(params, pb3.LogId_LogChargeByUseDitchToken)
	logworker.LogTokenOrder(player, &pb3.LogTokenOrder{
		ProductId:    chargeId,
		Fee:          uint64(configTokens),
		RemainderFee: uint64(player.GetDitchTokens()),
	})

	ditchToken := player.GetBinaryData().DitchToken
	timeStr := fmt.Sprintf("%d", time_util.NowSec()-player.GetCreateTime())
	if !ditchToken.SkipFirst {
		logworker.LogPlayerBehavior(player, pb3.LogId_LogFirstUseTokenChargeDiffCreateTime, &pb3.LogPlayerCounter{
			NumArgs: uint64(chargeId),
			StrArgs: timeStr,
		})
		ditchToken.SkipFirst = true
	}
	return true
}

func (sys *ChargeSys) c2sCheckCharge(msg *base.Message) error {
	var req pb3.C2S_36_4
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return err
	}
	chargeId := req.GetChargeId()
	conf := jsondata.GetChargeConf(chargeId)
	if nil == conf {
		return neterror.ParamsInvalidError("charge conf(%d) is nil", chargeId)
	}
	canCharge := engine.GetChargeCheckResult(conf.ChargeType, sys.owner, conf)
	rsp := &pb3.S2C_36_4{ChargeId: chargeId, CanCharge: canCharge}

	if !canCharge {
		sys.SendProto3(36, 4, rsp)
		return nil
	}

	result := sys.checkChargeToken(chargeId)
	if !result {
		result = sys.checkDitchToken(chargeId)
	}
	rsp.UseToken = result

	sys.SendProto3(36, 4, rsp)
	return nil
}

func (sys *ChargeSys) logFirstCharge(params *pb3.OnChargeParams) {
	if params.SkipLogFirstCharge {
		return
	}

	owner := sys.GetOwner()
	chargeInfo := owner.GetBinaryData().GetChargeInfo()

	// 红包类型的充值都给过滤
	chargeConf := jsondata.GetChargeConf(params.ChargeId)
	if chargeConf != nil && chargedef.RedPack == chargeConf.ChargeType {
		return
	}

	if chargeInfo.FirstRealCashChargeAt != 0 {
		return
	}

	nowSec := time_util.NowSec()
	chargeInfo.FirstRealCashChargeAt = nowSec
	createTime := owner.GetCreateTime()
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogFirstChargeDiffCreateTime, &pb3.LogPlayerCounter{
		NumArgs: uint64(params.ChargeId),
		StrArgs: fmt.Sprintf("%d", nowSec-createTime),
	})
}

func (sys *ChargeSys) OnCharge(params *pb3.OnChargeParams, logId pb3.LogId) {
	chargeId := params.ChargeId
	cashCent := params.CashCent
	owner := sys.GetOwner()
	conf := jsondata.GetChargeConf(chargeId)
	sys.logFirstCharge(params)
	if nil == conf {
		// 没有配置直接当充值
		owner.AddAmount(cashCent, cashCent, logId, chargeId)
		engine.TriggerChargeCallBackEvent(chargedef.Charge, owner, nil, &pb3.ChargeCallBackParams{
			CashCent: cashCent,
			Diamond:  0,
			LogId:    uint32(logId),
		})
	} else {
		var diamond uint32
		var cash uint32
		diamond = conf.Diamond
		cash = conf.CashCent
		if params.BatchCharge {
			cash = cash * params.BatchCount
			diamond = diamond * params.BatchCount
		}
		if conf.IsAddAmount {
			owner.AddAmount(diamond, cash, logId, chargeId)
		}

		if logId == pb3.LogId_LogCharge {
			binary := sys.GetBinaryData()
			binary.ChargeNum++
		}

		// 如果充值类型分发失败，直接当充值
		chargeType := conf.ChargeType
		if chargeType != chargedef.AddAmountCharge && !engine.TriggerChargeCallBackEvent(chargeType, owner, conf, &pb3.ChargeCallBackParams{
			CashCent:    cash,
			Diamond:     diamond,
			BatchCharge: params.BatchCharge,
			BatchCount:  params.BatchCount,
			LogId:       uint32(logId),
		}) {
			chargeType = chargedef.Charge
			engine.TriggerChargeCallBackEvent(chargedef.Charge, owner, nil, &pb3.ChargeCallBackParams{
				CashCent: cash,
				Diamond:  diamond,
				LogId:    uint32(logId),
			})
		}
	}
	sys.SendProto3(36, 3, &pb3.S2C_36_3{ChargeId: chargeId})
	owner.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionCharge)
	owner.TriggerEvent(custom_id.AeAddDailyCharge, chargeId, cashCent)
	manager.TriggerCalcPowerRushRankByType(owner, ranktype.PowerRushRankTypeCharge)
	handleChargeReturn(owner, chargeId)
}

func (sys *ChargeSys) CheckResetRecharge() {
	staticVar := gshare.GetStaticVar()
	serverFirstChargeTimes := staticVar.GetFirstChargeTimes()
	owner := sys.GetOwner()
	binaryData := owner.GetBinaryData()
	if binaryData != nil && binaryData.FirstChargeTimes != serverFirstChargeTimes {
		oldFlag := binaryData.ChargeFlag
		binaryData.ChargeFlag = 0
		binaryData.ChargeExtraRewardsFlag = 0
		binaryData.FirstChargeTimes = serverFirstChargeTimes
		logworker.LogPlayerBehavior(sys.owner, pb3.LogId_LogPlayerResetFirstChargeTimes, &pb3.LogPlayerCounter{
			NumArgs: uint64(serverFirstChargeTimes),
			StrArgs: fmt.Sprintf("%d", oldFlag),
		})
	}
}

func (sys *ChargeSys) onNewDay() {
	binary := sys.GetBinaryData()
	var delIds []uint32
	expireTime := time_util.GetBeforeDaysZeroTime(30)

	for timeStamp := range binary.ChargeInfo.DailyChargeMoneyMap {
		if timeStamp < expireTime {
			delIds = append(delIds, timeStamp)
		}
	}

	for _, id := range delIds {
		delete(binary.ChargeInfo.DailyChargeMoneyMap, id)
	}
}

func handleChargeReturn(actor iface.IPlayer, chargeId uint32) {
	returnConf := jsondata.GetChargeReturnConf(chargeId)
	if returnConf == nil {
		return
	}

	if len(returnConf.Rewards) == 0 {
		return
	}

	if !actor.ConsumeByConf(jsondata.ConsumeVec{{
		Id:    returnConf.ItemId,
		Count: 1,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogChargeReturnAwards}) {
		actor.LogWarn("chargeId %d return failed", chargeId)
		return
	}

	itemConf := jsondata.GetItemConfig(returnConf.ItemId)
	mailmgr.SendMailToActor(actor.GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_ChargeReturnAwards,
		Rewards: returnConf.Rewards,
		Content: &mailargs.CommonMailArgs{
			Str1: itemConf.Name,
		},
	})
}

func OnCharge(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if nil == actor {
		return false
	}

	if params == nil {
		return false
	}

	cashCent := params.CashCent

	if conf == nil {
		// 传过来的配置为nil
		actor.Charge(int64(params.Diamond), pb3.LogId_LogCharge)
	} else {
		if cashCent == conf.CashCent {
			// 如果有配置, 并且发过来的cashNum跟配置的cash相符
			binary := actor.GetBinaryData()
			flag := binary.ChargeFlag
			mTimes := gshare.GetStaticVar().GetMergeTimes()
			if nil != conf.GiveIndex && nil != conf.GiveIndex[mTimes] {
				idx := conf.GiveIndex[mTimes].Index
				if !utils.IsSetBit(flag, idx-1) { // 合服重置flag
					binary.ChargeFlag = utils.SetBit(flag, idx-1)
					actor.AddMoney(conf.GiveIndex[mTimes].MoneyType, int64(conf.GiveIndex[mTimes].Diamond), true, pb3.LogId_LogCharge)
					if len(conf.GiveIndex[mTimes].Reward) > 0 {
						engine.GiveRewards(actor, conf.GiveIndex[mTimes].Reward, common.EngineGiveRewardParam{LogId: pb3.LogId_LogCharge})
					}
				}
			}
			if len(conf.ExtraRewards) > 0 && custom_id.IsRealChargeLog(pb3.LogId(params.LogId)) {
				if !utils.IsSetBit(binary.ChargeExtraRewardsFlag, conf.ExtraIndex-1) {
					binary.ChargeExtraRewardsFlag = utils.SetBit(binary.ChargeExtraRewardsFlag, conf.ExtraIndex-1)
					engine.GiveRewards(actor, conf.ExtraRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogCharge})
				}
			}

			if !conf.NoAddDiamond {
				actor.Charge(int64(conf.Diamond), pb3.LogId_LogCharge)
			}
		} else {
			// 否则, diamond按照cash*100给他
			actor.Charge(int64(conf.Diamond), pb3.LogId_LogCharge)
		}
	}
	return true
}

func RedPack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if nil == actor {
		return false
	}
	if params == nil {
		return false
	}

	var count = uint32(1)
	if params.BatchCharge && params.BatchCount > 0 {
		count = params.BatchCount
	}

	diaNum := conf.Diamond
	actor.AddMoney(moneydef.Diamonds, int64(diaNum*count), true, pb3.LogId_LogRedPack)
	return true
}

func RandRedPack(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if nil == actor {
		return false
	}

	if params == nil {
		return false
	}

	diamond := params.Diamond
	if diamond > 0 {
		actor.AddMoney(moneydef.BindDiamonds, int64(diamond), true, pb3.LogId_LogRedPack)
	}
	return true
}

func DitchToken(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if nil == actor {
		return false
	}
	if params == nil {
		return false
	}

	actor.AddDitchTokens(conf.Tokens)
	actor.SetExtraAttr(attrdef.DitchTokens, attrdef.AttrValueAlias(actor.GetDitchTokens()))
	actor.UpdateStatics(model.FieldDitchTokens_, actor.GetDitchTokens())
	actor.UpdateStatics(model.FieldHistoryDitchTokens_, actor.GetHistoryDitchTokens())
	return true
}

func offlineCharge(actor iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.OfflineCommonSt)
	if !ok {
		return
	}

	sys, ok := actor.GetSysObj(sysdef.SiCharge).(*ChargeSys)
	if !ok {
		return
	}

	var params = &pb3.OnChargeParams{
		ChargeId: uint32(st.Param1),
		CashCent: st.U32Param,
		OrderId:  st.StrParam1,
	}
	sys.OnCharge(params, pb3.LogId_LogCharge)
}

func offlineGMClearChargeToken(player iface.IPlayer, msg pb3.Message) {
	binary := player.GetBinaryData()
	oldToken := binary.GetChargeTokens()
	binary.ChargeTokens = 0
	player.UpdateStatics(model.FieldChargeTokens_, binary.ChargeTokens)
	player.SetExtraAttr(attrdef.ChargeTokens, 0)
	player.LogInfo("clear charge token %d -> 0", oldToken)
}
func dealPlayerChargeEvent(player iface.IPlayer, msg pb3.Message) {
	player.TriggerEvent(custom_id.AeAfterMerge)
}

func UseItemAddChargeTokens(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	binary := player.GetBinaryData()
	binary.UseChargeItem = chargedef.UseChargeTokenOk
	binary.ChargeTokens += uint32(param.Count) * conf.Param[0]
	binary.HistoryChargeTokens += uint32(param.Count) * conf.Param[0]
	player.SetExtraAttr(attrdef.ChargeTokens, int64(binary.GetChargeTokens()))
	player.SetExtraAttr(attrdef.UseChargeItem, chargedef.UseChargeTokenOk)
	player.UpdateStatics(model.FieldUseChargeItem_, binary.UseChargeItem)
	player.UpdateStatics(model.FieldChargeTokens_, binary.ChargeTokens)
	player.UpdateStatics(model.FieldHistoryChargeTokens_, binary.HistoryChargeTokens)
	return true, true, param.Count
}

func useItemCharge(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	player.LogDebug("%s 使用充值道具 itemId:%d", player.GetName(), conf.ItemId)
	if len(conf.Param) < 3 {
		return false, false, 0
	}
	success, del = false, false

	sys, ok := player.GetSysObj(sysdef.SiCharge).(*ChargeSys)
	if !ok {
		return
	}
	chargeId := conf.Param[0]
	line := jsondata.GetChargeConf(chargeId)
	if nil == line {
		return
	}

	var params = &pb3.OnChargeParams{
		ChargeId:           chargeId,
		CashCent:           line.CashCent * uint32(param.Count),
		BatchCharge:        true,
		BatchCount:         uint32(param.Count),
		SkipLogFirstCharge: true,
	}
	sys.OnCharge(params, pb3.LogId_LogChargeItem)

	if conf.Param[1] > 0 {
		binary := player.GetBinaryData()
		binary.UseChargeItem = chargedef.UseChargeTokenOk
		player.SetExtraAttr(attrdef.UseChargeItem, chargedef.UseChargeTokenOk)
		player.UpdateStatics("use_charge_item", 1)
	}

	if conf.Param[2] > 0 {
		engine.BroadcastTipMsgById(conf.Param[2], player.GetId(), player.GetName())
	}

	return true, true, param.Count
}

func OfflineResetFirstRecharge(player iface.IPlayer, msg pb3.Message) {
	if sys, ok := player.GetSysObj(sysdef.SiCharge).(*ChargeSys); ok {
		sys.CheckResetRecharge()
		player.SendChargeInfo()
	}
}

func getLifetimeDirectPurchaseData(actor iface.IPlayer) *pb3.LifetimeDirectPurchaseData {
	lifetimeDirectPurchaseData := actor.GetBinaryData().LifetimeDirectPurchaseData
	if lifetimeDirectPurchaseData == nil {
		actor.GetBinaryData().LifetimeDirectPurchaseData = &pb3.LifetimeDirectPurchaseData{}
		lifetimeDirectPurchaseData = actor.GetBinaryData().LifetimeDirectPurchaseData
	}
	if lifetimeDirectPurchaseData.ChargeCount == nil {
		lifetimeDirectPurchaseData.ChargeCount = make(map[uint32]uint32)
	}
	return lifetimeDirectPurchaseData
}

func checkLifetimeDirectPurchase(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	data := getLifetimeDirectPurchaseData(actor)
	purchaseConf := jsondata.GetLifetimeDirectPurchaseConf(conf.ChargeId)
	return purchaseConf != nil && data.ChargeCount[conf.ChargeId] < purchaseConf.Count
}

func chargeLifetimeDirectPurchase(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if !checkLifetimeDirectPurchase(actor, conf) {
		return false
	}
	purchaseConf := jsondata.GetLifetimeDirectPurchaseConf(conf.ChargeId)
	data := getLifetimeDirectPurchaseData(actor)
	data.ChargeCount[conf.ChargeId] += 1
	engine.GiveRewards(actor, purchaseConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLifetimeDirectPurchaseAward})
	actor.SendShowRewardsPop(purchaseConf.Rewards)
	logworker.LogPlayerBehavior(actor, pb3.LogId_LogLifetimeDirectPurchaseAward, &pb3.LogPlayerCounter{
		NumArgs: uint64(conf.ChargeId),
		StrArgs: fmt.Sprintf("%d", data.ChargeCount[conf.ChargeId]),
	})
	DitchToken(actor, conf, params)
	actor.SendProto3(36, 6, &pb3.S2C_36_6{Data: data})
	return true
}

func init() {
	RegisterSysClass(sysdef.SiCharge, func() iface.ISystem {
		return &ChargeSys{}
	})

	engine.RegisterMessage(gshare.OfflinePlayerMergeEvent, func() pb3.Message {
		return nil
	}, dealPlayerChargeEvent)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemCharge, useItemCharge)

	engine.RegChargeEvent(chargedef.Charge, nil, OnCharge)
	engine.RegChargeEvent(chargedef.RedPack, nil, RedPack)
	engine.RegChargeEvent(chargedef.RandRedPack, nil, RandRedPack)
	engine.RegChargeEvent(chargedef.DitchToken, nil, DitchToken)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiCharge).(*ChargeSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	net.RegisterSysProto(36, 4, sysdef.SiCharge, (*ChargeSys).c2sCheckCharge)

	engine.RegisterMessage(gshare.OfflineChargeOrder, func() pb3.Message {
		return &pb3.OfflineCommonSt{}
	}, offlineCharge)

	engine.RegisterMessage(gshare.OfflineGMClearChargeToken, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineGMClearChargeToken)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddChargeTokens, UseItemAddChargeTokens)

	engine.RegQuestTargetProgress(custom_id.QttTodayChargeXAmount, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		binaryData := player.GetBinaryData()
		if binaryData == nil {
			return 0
		}
		chargeInfo := binaryData.GetChargeInfo()
		if chargeInfo == nil {
			return 0
		}
		return chargeInfo.DailyChargeDiamond
	})

	manager.RegCalcPowerRushRankSubTypeHandle(ranktype.PowerRushRankSubTypeCharge, func(player iface.IPlayer) (score int64) {
		binaryData := player.GetBinaryData()
		if binaryData == nil {
			return 0
		}
		chargeInfo := binaryData.GetChargeInfo()
		if chargeInfo == nil {
			return 0
		}
		return int64(chargeInfo.DailyChargeDiamond)
	})
	engine.RegisterMessage(gshare.OfflineResetFirstRecharge, func() pb3.Message {
		return &pb3.CommonSt{}
	}, OfflineResetFirstRecharge)

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgResetFirstRecharge, func(param ...interface{}) {
			staticVar := gshare.GetStaticVar()
			staticVar.FirstChargeTimes = staticVar.GetFirstChargeTimes() + 1
			manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
				engine.SendPlayerMessage(p.Id, gshare.OfflineResetFirstRecharge, &pb3.CommonSt{})
				return true
			})
		})
	})

	engine.RegChargeEvent(chargedef.LifetimeDirectPurchase, checkLifetimeDirectPurchase, chargeLifetimeDirectPurchase)
}
