package actorsystem

import (
	"errors"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

/**
* @Author: YangQibin
* @Desc: 月卡周卡
* @Date: 2023/6/13
 */

type PrivilegeCardSys struct {
	Base
	data *pb3.PrivilegeCardData

	timers map[uint32]*time_util.Timer
}

func (sys *PrivilegeCardSys) OnInit() {
	if !sys.IsOpen() {
		return
	}

	sys.init()
}

func (sys *PrivilegeCardSys) init() bool {
	binaryData := sys.GetBinaryData()
	if binaryData.PrivilegeCardData == nil {
		binaryData.PrivilegeCardData = &pb3.PrivilegeCardData{}
	}

	sys.data = binaryData.PrivilegeCardData

	if sys.data.Cards == nil {
		sys.data.Cards = make(map[uint32]*pb3.PrivilegeCard)
	}

	sys.timers = make(map[uint32]*time_util.Timer)
	return true
}

func (sys *PrivilegeCardSys) OnOpen() {
	if !sys.init() {
		return
	}
}

func (sys *PrivilegeCardSys) OnLogin() {
	var needNotifyExpired []uint32
	for _, card := range sys.data.Cards {
		if sys.needSendExpiredP(card) {
			needNotifyExpired = append(needNotifyExpired, card.Type)
		}

		if sys.needSupplyRewardsP(card.Type) {
			sys.tryRewardOutDatedReward(card)
			continue
		}

		if sys.CardActivatedP(card.Type) {
			sys.setTimer(card.Type)
		}
	}

	sys.s2cInfo()

	for _, v := range needNotifyExpired {
		sys.sendExpireAndTryFlagSended(v)
	}
}

func (sys *PrivilegeCardSys) needSendExpiredP(card *pb3.PrivilegeCard) bool {
	if card == nil {
		return false
	}

	return !sys.CardActivatedP(card.Type) && !card.ExpireSended
}

func (sys *PrivilegeCardSys) OnLogout() {
	for _, t := range sys.timers {
		t.Stop()
	}
}

func (sys *PrivilegeCardSys) s2cInfo() {
	sys.SendProto3(37, 0, &pb3.S2C_37_0{
		Info: sys.data,
	})
}

func (sys *PrivilegeCardSys) OnReconnect() {
	sys.s2cInfo()
	for cardType, card := range sys.data.Cards {
		if sys.needSendExpiredP(card) {
			sys.sendExpireAndTryFlagSended(cardType)
		}
	}
}

func (sys *PrivilegeCardSys) c2sInfo(_ *base.Message) error {
	sys.s2cInfo()
	return nil
}

func (sys *PrivilegeCardSys) sendExpireAndTryFlagSended(cardType uint32) {
	if !sys.owner.GetLost() {
		sys.data.Cards[cardType].ExpireSended = true
	}

	sys.SendProto3(37, 3, &pb3.S2C_37_3{
		CardType: cardType,
	})
}

func (sys *PrivilegeCardSys) setTimer(cardType uint32) {
	if !sys.CardActivatedP(cardType) {
		return
	}

	card, ok := sys.data.Cards[cardType]
	if !ok {
		return
	}

	if sys.timers[cardType] != nil {
		sys.timers[cardType].Stop()
		return
	}

	cardConf := jsondata.GetPrivilegeCardConfByType(cardType)
	if cardConf == nil {
		return
	}
	now := time.Now()
	expireTime := time.Unix(int64(card.ExpireTime), 0)
	leftTIme := expireTime.Sub(now)
	// 月卡周卡过期
	sys.timers[cardConf.CardType] = sys.GetOwner().SetTimeout(leftTIme, func() {
		sys.onPrivilegeCardDisactivate(card)
	})
}

func (sys *PrivilegeCardSys) tryRewardOutDatedReward(card *pb3.PrivilegeCard) {
	conf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if conf == nil {
		return
	}

	rewards := sys.calcAllRewards(card.Type)
	if len(rewards) == 0 {
		return
	}

	cardNames := map[uint32]string{
		privilegedef.PrivilegeCardType_Week:  "周卡",
		privilegedef.PrivilegeCardType_Month: "月卡",
	}

	card.LastRewardTime = card.ExpireTime
	card.CanBuyRewardTimes = 0

	sys.GetOwner().SendMail(&mailargs.SendMailSt{
		ConfId: common.Mail_PrivilegeCardBack,
		Content: &mailargs.PrivilegeCardBackArgs{
			Name: cardNames[card.Type],
		},
		Rewards: rewards,
	})
}

func (sys *PrivilegeCardSys) onPrivilegeCardDisactivate(card *pb3.PrivilegeCard) {
	sys.tryRewardOutDatedReward(card)

	if sys.timers[card.Type] != nil {
		sys.timers[card.Type].Stop()
		delete(sys.timers, card.Type)
	}
	sys.owner.TriggerEvent(custom_id.AePrivilegeCardDisActivated, argsdef.AePvCardDisactivatedArg(card.Type))
	sys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	sys.s2cInfo()
	sys.sendExpireAndTryFlagSended(card.Type)
}

func (sys *PrivilegeCardSys) GetCardStartAndEndTime(cardType uint32) (uint32, uint32) {
	card, ok := sys.data.Cards[cardType]
	if !ok {
		return 0, 0
	}
	startTime := card.StartTime
	endTime := card.ExpireTime
	return startTime, endTime
}

func (sys *PrivilegeCardSys) needSupplyRewardsP(cardType uint32) bool {
	// 试用周卡不需要补发
	if cardType == privilegedef.PrivilegeCardType_TryWeek {
		return false
	}
	// 还有奖励没领，并且月卡已过期
	return len(sys.calcAllRewards(cardType)) > 0 && !sys.CardActivatedP(cardType)
}

func (sys *PrivilegeCardSys) CardActivatedP(cardType uint32) bool {
	card, ok := sys.data.Cards[cardType]
	if !ok {
		return false
	}

	now := time.Now()
	expireTime := time.Unix(int64(card.ExpireTime), 0)

	return !now.After(expireTime)
}

func (sys *PrivilegeCardSys) c2sBuyReward(msg *base.Message) error {
	var req pb3.C2S_37_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	if req.CardType == privilegedef.PrivilegeCardType_TryWeek {
		return neterror.ParamsInvalidError("try card can`t reward buyReward")
	}

	if !sys.CardActivatedP(uint32(req.CardType)) {
		return neterror.ParamsInvalidError("card is expired")
	}

	card, ok := sys.data.Cards[uint32(req.CardType)]
	if !ok {
		return neterror.ParamsInvalidError("card not exist")
	}

	if card.CanBuyRewardTimes <= 0 {
		return neterror.ParamsInvalidError("card can`t buyReward")
	}

	cardLvConf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if cardLvConf == nil {
		return neterror.InternalError("card conf not exist")
	}

	card.CanBuyRewardTimes--
	rewards := cardLvConf.BuyRewards
	state := engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPrivilegeCardBuyReward,
	})

	if len(rewards) > 0 {
		owner := sys.owner
		switch card.Type {
		case privilegedef.PrivilegeCardType_Week:
			engine.BroadcastTipMsgById(tipmsgid.ZhoukaPrivilege, owner.GetId(), owner.GetName())
		case privilegedef.PrivilegeCardType_Month:
			engine.BroadcastTipMsgById(tipmsgid.YuekaPrivilege, owner.GetId(), owner.GetName())
		}
	}

	if !state {
		return neterror.InternalError("give rewards failed")
	}

	sys.SendProto3(37, 1, &pb3.S2C_37_1{
		CardType: req.CardType,
	})
	return nil
}

func (sys *PrivilegeCardSys) todayRewardedP(card *pb3.PrivilegeCard) bool {
	if card == nil {
		return true
	}

	lastRewardTime := time.Unix(int64(card.LastRewardTime), 0)
	now := time.Now()

	return now.Year() == lastRewardTime.Year() &&
		now.Month() == lastRewardTime.Month() &&
		now.Day() == lastRewardTime.Day()
}

func (sys *PrivilegeCardSys) upgradedP(card *pb3.PrivilegeCard) bool {
	if card == nil {
		return false
	}

	return card.UpLevelStartTime > 0 && card.Level > 1
}

func (sys *PrivilegeCardSys) calcNatualDurDays(start uint32, end uint32) uint32 {
	return time_util.TimestampSubDays(start, end)
}

func (sys *PrivilegeCardSys) calcAllRewards(cardType uint32) jsondata.StdRewardVec {
	_, ok := sys.data.Cards[cardType]
	if !ok {
		return nil
	}

	dayRewards := sys.calcAllDailyRewards(cardType)
	buyRewards := sys.calcAllBuyRewards(cardType)

	if len(dayRewards) == 0 || len(buyRewards) == 0 {
		return nil
	}

	return jsondata.MergeStdReward(dayRewards, buyRewards)
}

func (sys *PrivilegeCardSys) calcAllBuyRewards(cardType uint32) jsondata.StdRewardVec {
	card, ok := sys.data.Cards[cardType]
	if !ok {
		return nil
	}

	if card.CanBuyRewardTimes == 0 {
		return nil
	}

	cardLvConf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if cardLvConf == nil {
		return nil
	}

	rewards := jsondata.StdRewardMulti(cardLvConf.BuyRewards, int64(card.CanBuyRewardTimes))

	return rewards
}

func (sys *PrivilegeCardSys) calcAllDailyRewards(cardType uint32) jsondata.StdRewardVec {
	card, ok := sys.data.Cards[cardType]
	if !ok {
		return nil
	}

	// 上次距离当前时间经过多少天
	durDays := sys.calcNatualDurDays(card.LastRewardTime, uint32(time.Now().Unix()))
	if durDays == 0 {
		return nil
	}

	// 防止超过最大天数
	durDays = utils.MinUInt32(durDays, sys.calcNatualDurDays(card.LastRewardTime, card.ExpireTime))

	cardLvConf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if cardLvConf == nil {
		return nil
	}

	if durDays == 0 {
		return nil
	}

	rewards := cardLvConf.DailyRewards
	return jsondata.StdRewardMulti(rewards, int64(durDays))
}

func (sys *PrivilegeCardSys) c2sDailyReward(msg *base.Message) error {
	var req pb3.C2S_37_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal failed %s", err)
	}

	if req.CardType == privilegedef.PrivilegeCardType_TryWeek {
		return neterror.ParamsInvalidError("try card can`t daily reward")
	}

	if !sys.CardActivatedP(uint32(req.CardType)) {
		return neterror.ParamsInvalidError("card is expired")
	}

	card, ok := sys.data.Cards[uint32(req.CardType)]
	if !ok {
		return neterror.ParamsInvalidError("card not exist")
	}

	if sys.todayRewardedP(card) {
		return neterror.ParamsInvalidError("card already rewarded today")
	}

	cardLvConf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if cardLvConf == nil {
		return neterror.InternalError("card conf not exist")
	}

	var rewards jsondata.StdRewardVec
	for !sys.todayRewardedP(card) {
		rewards = jsondata.MergeStdReward(rewards, cardLvConf.DailyRewards)
		card.LastRewardTime = uint32(time.Unix(int64(card.LastRewardTime), 0).AddDate(0, 0, 1).Unix())
	}

	state := engine.GiveRewards(sys.GetOwner(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPrivilegeCardDailyReward,
	})
	if !state {
		return neterror.InternalError("give rewards failed")
	}

	sys.SendProto3(37, 2, &pb3.S2C_37_2{
		LastRewardTime: card.LastRewardTime,
	})
	return nil
}

func (sys *PrivilegeCardSys) weekCardChargeCheck() error {
	conf := jsondata.GetPrivilegeCardConfByType(privilegedef.PrivilegeCardType_Week)
	if conf == nil {
		return errors.New("no config for weekcard")
	}

	if len(conf.LevelConf) == 0 {
		return errors.New("no level config for weekcard")
	}

	card, ok := sys.data.Cards[privilegedef.PrivilegeCardType_Week]
	if !ok {
		return nil
	}

	if conf.RenewTimes > 0 && sys.CardActivatedP(privilegedef.PrivilegeCardType_Week) && card.RenewTimes >= conf.RenewTimes {
		return errors.New("reach max renew times")
	}

	return nil
}

func (sys *PrivilegeCardSys) weekCardCharge(isPay bool) error {
	if err := sys.weekCardChargeCheck(); err != nil {
		return err
	}

	// 如果之前有体验卡，先把体验卡停掉
	if sys.CardActivatedP(privilegedef.PrivilegeCardType_TryWeek) {
		sys.timers[privilegedef.PrivilegeCardType_TryWeek].Stop()
		tryWeekCard := sys.data.Cards[privilegedef.PrivilegeCardType_TryWeek]
		tryWeekCard.ExpireTime = uint32(time.Now().Unix())
		tryWeekCard.ExpireSended = true
	}

	card, ok := sys.data.Cards[privilegedef.PrivilegeCardType_Week]
	if !ok {
		card = &pb3.PrivilegeCard{
			Type: privilegedef.PrivilegeCardType_Week,
		}
		sys.data.Cards[privilegedef.PrivilegeCardType_Week] = card
	}

	if !sys.CardActivatedP(privilegedef.PrivilegeCardType_Week) { // 非续费
		card.StartTime = uint32(time.Now().Unix())
		card.ExpireTime = uint32(time.Unix(int64(card.StartTime), 0).AddDate(0, 0, 7).Unix())
		card.LastRewardTime = card.StartTime
		card.CanBuyRewardTimes = 0
		card.RenewTimes = 0
	} else { // 续费
		card.RenewTimes++
		card.ExpireTime = uint32(time.Unix(int64(card.ExpireTime), 0).AddDate(0, 0, 7).Unix())
	}

	card.CanBuyRewardTimes++
	card.ExpireSended = false
	card.Level = 1
	if isPay {
		card.IsPayActive = true
	}

	if sys.timers[card.Type] != nil {
		sys.timers[card.Type].Stop()
	}
	sys.setTimer(card.Type)

	sys.GetOwner().SetTimeout(1*time.Millisecond, func() {
		sys.owner.TriggerEvent(custom_id.AePrivilegeCardActivated, argsdef.AePvCardActivatedArg(card.Type))
		sys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	})
	return nil
}

func (sys *PrivilegeCardSys) monthCardChargeCheck() error {
	conf := jsondata.GetPrivilegeCardConfByType(privilegedef.PrivilegeCardType_Month)
	if conf == nil {
		return errors.New("no config for monthCard")
	}

	if len(conf.LevelConf) == 0 {
		return errors.New("no level config for monthCard")
	}

	card, ok := sys.data.Cards[privilegedef.PrivilegeCardType_Month]
	if !ok {
		return nil
	}

	// 配制了上限才校验
	if conf.RenewTimes > 0 && card.RenewTimes >= conf.RenewTimes && sys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
		return errors.New("reach max renew times cur %d limit")
	}

	return nil
}

func (sys *PrivilegeCardSys) monthCardCharge(isPay bool) error {
	if err := sys.monthCardChargeCheck(); err != nil {
		return err
	}

	card, ok := sys.data.Cards[privilegedef.PrivilegeCardType_Month]
	if !ok {
		card = &pb3.PrivilegeCard{
			Type: privilegedef.PrivilegeCardType_Month,
		}
		sys.data.Cards[privilegedef.PrivilegeCardType_Month] = card
	}

	if !sys.CardActivatedP(privilegedef.PrivilegeCardType_Month) { // 非续费
		card.StartTime = uint32(time.Now().Unix())
		card.ExpireTime = uint32(time.Unix(int64(card.StartTime), 0).AddDate(0, 0, 30).Unix())
		card.LastRewardTime = card.StartTime
		card.CanBuyRewardTimes = 0
		card.RenewTimes = 0
	} else { // 续费
		card.RenewTimes++
		card.ExpireTime = uint32(time.Unix(int64(card.ExpireTime), 0).AddDate(0, 0, 30).Unix())
	}
	card.CanBuyRewardTimes++
	if card.Level == 0 {
		card.Level = 1
	}
	if isPay {
		card.IsPayActive = true
	}

	if sys.timers[card.Type] != nil {
		sys.timers[card.Type].Stop()
	}
	sys.setTimer(card.Type)

	// 这里延迟一帧，方便前端处理弹窗顺序
	sys.GetOwner().SetTimeout(1*time.Millisecond, func() {
		sys.owner.TriggerEvent(custom_id.AePrivilegeCardActivated, argsdef.AePvCardActivatedArg(card.Type))
		sys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	})

	return nil
}

func (sys *PrivilegeCardSys) monthCardUplvChargeCheck() error {
	if !sys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
		return errors.New("monthCard not activated")
	}

	card := sys.data.Cards[privilegedef.PrivilegeCardType_Month]
	if sys.upgradedP(card) {
		return errors.New("monthCard already upgraded")
	}

	return nil
}

func (sys *PrivilegeCardSys) monthCardUpLvCharge() error {
	if err := sys.monthCardUplvChargeCheck(); err != nil {
		return err
	}

	card := sys.data.Cards[privilegedef.PrivilegeCardType_Month]
	card.UpLevelStartTime = uint32(time.Now().Unix())
	card.Level++

	cardLvConf := jsondata.GetPrivilegeCardLevelConf(card.Type, card.Level)
	if cardLvConf != nil && cardLvConf.MonthUpLvRewards != nil {
		engine.GiveRewards(sys.GetOwner(), cardLvConf.MonthUpLvRewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogUpgratedMonthCardBuyReward})
	} else {
		sys.LogError("monthCardUpLvCharge: cardLvConf is nil")
	}

	sys.owner.TriggerEvent(custom_id.AePrivilegeCardEnhanced, argsdef.AePvCardEnhancedArg(card.Type))
	sys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	return nil
}

func check_privilegeCardWeekChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	privilegeCardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !privilegeCardSys.IsOpen() {
		player.LogError("privilegeCardWeekChargeCheckHandler: privilegeCardSys not open")
		return false
	}

	err := privilegeCardSys.weekCardChargeCheck()
	if err != nil {
		player.LogError("privilegeCardWeekChargeCheckHandler: %s", err.Error())
		return false
	}

	return true
}

func privilegeCardWeekChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !check_privilegeCardWeekChargeHandler(player, conf) {
		return false
	}

	privilegeCardSys := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)

	err := privilegeCardSys.weekCardCharge(true)
	if err != nil {
		player.LogError("privilegeCardWeekChargeHandler: %s", err.Error())
		return false
	}

	return true
}

func check_privilegeCardMonthChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	privilegeCardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !privilegeCardSys.IsOpen() {
		player.LogError("privilegeCardMonthChargeCheckHandler: privilegeCardSys not open")
		return false
	}

	err := privilegeCardSys.monthCardChargeCheck()
	if err != nil {
		player.LogError("privilegeCardMonthChargeCheckHandler: %s", err.Error())
		return false
	}

	return true
}

func privilegeCardMonthChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !check_privilegeCardMonthChargeHandler(player, conf) {
		return false
	}

	privilegeCardSys := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)

	err := privilegeCardSys.monthCardCharge(true)
	if err != nil {
		player.LogError("privilegeCardMonthChargeHandler: %s", err.Error())
		return false
	}

	return true
}

func check_privilegeCardMonthCardUpLvChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	privilegeCardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !privilegeCardSys.IsOpen() {
		player.LogError("privilegeCardMonthCardUpLvChargeCheckHandler: privilegeCardSys not open")
		return false
	}

	err := privilegeCardSys.monthCardUplvChargeCheck()
	if err != nil {
		player.LogError("privilegeCardMonthCardUpLvChargeCheckHandler: %s", err.Error())
		return false
	}

	return true
}

func privilegeCardMonthCardUpLvChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !check_privilegeCardMonthCardUpLvChargeHandler(player, conf) {
		return false
	}

	privilegeCardSys := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)

	err := privilegeCardSys.monthCardUpLvCharge()
	if err != nil {
		player.LogError("privilegeCardMonthCardUpLvChargeHandler: %s", err.Error())
		return false
	}

	return true
}

func useItem_ActivateTryWeekCard(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	cardsys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !cardsys.IsOpen() {
		player.LogError("useItem:%d, cardsys not open!", param.ItemId)
		return false, false, 0
	}

	cardConf := jsondata.GetPrivilegeCardConfByType(privilegedef.PrivilegeCardType_TryWeek)
	if cardConf == nil {
		player.LogError("get conf for PrivilegeCardType_TryWeek failed")
		return false, false, 0
	}

	cardData := cardsys.data.Cards[privilegedef.PrivilegeCardType_TryWeek]
	if cardData != nil {
		player.LogError("useItem:%d, cardData already exist!", param.ItemId)
		return false, false, 0
	}

	cardData = &pb3.PrivilegeCard{
		Type: privilegedef.PrivilegeCardType_TryWeek,
	}
	cardsys.data.Cards[privilegedef.PrivilegeCardType_TryWeek] = cardData
	cardData.Level = 1
	cardData.StartTime = uint32(time.Now().Unix())
	cardData.ExpireTime = uint32(time.Now().Unix()) + cardConf.DurTime

	// 次日领 避免为 0 导致计算奖励 panic
	if cardData.LastRewardTime == 0 {
		cardData.LastRewardTime = cardData.StartTime
	}

	cardsys.setTimer(privilegedef.PrivilegeCardType_TryWeek)

	cardsys.GetOwner().SetTimeout(1*time.Millisecond, func() {
		cardsys.owner.TriggerEvent(custom_id.AePrivilegeCardActivated, argsdef.AePvCardActivatedArg(cardData.Type))
		cardsys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	})
	return true, true, param.Count
}

func useItem_ActivatePrivilegeCard(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	cardsys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !cardsys.IsOpen() {
		player.LogError("useItem:%d, cardsys not open!", param.ItemId)
		return false, false, 0
	}

	if len(conf.Param) < 1 {
		return false, false, 0
	}

	privilege := conf.Param[0]

	fn := func(privilegeType uint32) error {
		switch privilege {
		case privilegedef.PrivilegeCardType_Week:
			if err := cardsys.weekCardChargeCheck(); nil != err {
				return err
			}
			if err := cardsys.weekCardCharge(true); err != nil {
				return err
			}
		case privilegedef.PrivilegeCardType_Month:
			if err := cardsys.monthCardChargeCheck(); nil != err {
				return err
			}
			if err := cardsys.monthCardCharge(true); err != nil {
				return err
			}
		default:
			return neterror.ParamsInvalidError("not define type")
		}
		return nil
	}

	var useCount int64
	for i := 1; i <= int(param.Count); i++ {
		if err := fn(privilege); nil != err {
			break
		}
		useCount++
	}

	if useCount == 0 {
		return false, false, 0
	}

	cardsys.s2cInfo()

	return true, true, useCount
}

func check_privilegeWeekMonthQuickBuyChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	privilegeCardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
	if !ok || !privilegeCardSys.IsOpen() {
		player.LogError("privilegeCardWeekChargeCheckHandler: privilegeCardSys not open")
		return false
	}

	wCard, weekBuy := privilegeCardSys.data.Cards[privilegedef.PrivilegeCardType_Week]
	mCard, monthBuy := privilegeCardSys.data.Cards[privilegedef.PrivilegeCardType_Month]

	if (weekBuy && wCard.IsPayActive) || (monthBuy && mCard.IsPayActive) {
		player.LogError("privilegeCardWeekChargeCheckHandler: already owner one")
		return false
	}

	err := privilegeCardSys.weekCardChargeCheck()
	if err != nil {
		player.LogError("privilegeCardWeekChargeCheckHandler: %s", err.Error())
		return false
	}

	err = privilegeCardSys.monthCardChargeCheck()
	if err != nil {
		player.LogError("privilegeCardMonthChargeCheckHandler: %s", err.Error())
		return false
	}

	return true
}
func privilegeWeekMonthQuickBuyChargeHandler(player iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !check_privilegeWeekMonthQuickBuyChargeHandler(player, conf) {
		return false
	}

	privilegeCardSys := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)

	err := privilegeCardSys.weekCardCharge(true)
	if err != nil {
		player.LogError("privilegeCardWeekChargeHandler: %s", err.Error())
		return false
	}

	err = privilegeCardSys.monthCardCharge(true)
	if err != nil {
		player.LogError("privilegeCardMonthChargeHandler: %s", err.Error())
		return false
	}

	return true
}

func init() {
	RegisterSysClass(sysdef.SiPrivilegeCard, func() iface.ISystem {
		return &PrivilegeCardSys{}
	})

	gmevent.Register("privilegeCard.ExpireTry", func(player iface.IPlayer, _ ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !sys.IsOpen() {
			return false
		}

		sys.sendExpireAndTryFlagSended(privilegedef.PrivilegeCardType_TryWeek)
		return true
	}, 1)

	engine.RegChargeEvent(chargedef.PrivilegeWeekCard, check_privilegeCardWeekChargeHandler, privilegeCardWeekChargeHandler)
	engine.RegChargeEvent(chargedef.PrivilegeMonthCard, check_privilegeCardMonthChargeHandler, privilegeCardMonthChargeHandler)
	engine.RegChargeEvent(chargedef.PrivilegeMonthCardEnhance, check_privilegeCardMonthCardUpLvChargeHandler, privilegeCardMonthCardUpLvChargeHandler)
	engine.RegChargeEvent(chargedef.PrivilegeWeekMonthQuickBuy, check_privilegeWeekMonthQuickBuyChargeHandler, privilegeWeekMonthQuickBuyChargeHandler)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		privilegeCardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !privilegeCardSys.IsOpen() {
			return
		}

		privilegeCardSys.s2cInfo()
	})

	net.RegisterSysProto(37, 0, sysdef.SiPrivilegeCard, (*PrivilegeCardSys).c2sInfo)
	net.RegisterSysProto(37, 1, sysdef.SiPrivilegeCard, (*PrivilegeCardSys).c2sBuyReward)
	net.RegisterSysProto(37, 2, sysdef.SiPrivilegeCard, (*PrivilegeCardSys).c2sDailyReward)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemActivateTryWeekCard, useItem_ActivateTryWeekCard)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemActivatePrivilegeCard, useItem_ActivatePrivilegeCard)

	// 試用周卡
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		pvcardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !pvcardSys.IsOpen() {
			return
		}

		if !pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_TryWeek) {
			return
		}

		return int64(conf.FreeWeekly), nil
	})

	// 周卡
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		pvcardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !pvcardSys.IsOpen() {
			return
		}

		if !pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Week) {
			return
		}

		return int64(conf.Weekly), nil
	})

	// 月卡
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		pvcardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !pvcardSys.IsOpen() {
			return
		}

		if !pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
			return
		}

		return int64(conf.Month), nil
	})

	// 升级周卡
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		pvcardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !pvcardSys.IsOpen() {
			return
		}

		if !pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Week) {
			return
		}

		if !pvcardSys.upgradedP(pvcardSys.data.Cards[privilegedef.PrivilegeCardType_Week]) {
			return
		}

		return int64(conf.StrengthenWeekly), nil
	})

	// 升级月卡
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		pvcardSys, ok := player.GetSysObj(sysdef.SiPrivilegeCard).(*PrivilegeCardSys)
		if !ok || !pvcardSys.IsOpen() {
			return
		}

		if !pvcardSys.CardActivatedP(privilegedef.PrivilegeCardType_Month) {
			return
		}

		if !pvcardSys.upgradedP(pvcardSys.data.Cards[privilegedef.PrivilegeCardType_Month]) {
			return
		}

		return int64(conf.StrengthenMonth), nil
	})
}
