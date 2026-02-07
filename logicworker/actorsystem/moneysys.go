package actorsystem

import (
	"jjyz/base/argsdef"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/model"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type moneyAddHandler func(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool
type moneySubHandler func(sys *MoneySys, mt uint32, subCount int64, logId pb3.LogId) (bool, argsdef.RemoveMoneyMaps)

var moneyAddHandlers []moneyAddHandler
var moneySubHandlers []moneySubHandler

// add handlers ====================================

func unknownMoneyAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	return false
}

func moneyAddDefaultHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	sys.GetBinaryData().Money[uint32(mt)] += count

	player := sys.GetOwner()

	player.TriggerEvent(custom_id.AeMoneyChange, mt, count)
	sys.LogEconomy(uint32(mt), count, logId)
	if btip {
		player.SendTipMsg(tipmsgid.TpAddMoney, mt, count)
	}
	sys.owner.TriggerQuestEvent(custom_id.QttAddMoneyCount, uint32(mt), count)
	sys.SendProto3(2, 1, &pb3.S2C_2_1{Moneys: map[uint32]int64{uint32(mt): sys.GetMoney(uint32(mt))}})
	return true
}

func potentialExpAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	lvSys, ok := sys.GetOwner().GetSysObj(sysdef.SiPotential).(*PotentialSys)
	if !ok || !lvSys.IsOpen() {
		return false
	}
	lvSys.AddExp(count, logId)
	return true
}

func smithExpAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	ref, ok := gshare.SmithInstance.FindSmithRefByExpMoneyType(mt)
	if !ok {
		return false
	}

	lvSys, ok := sys.GetOwner().GetSysObj(ref.SmithSysId).(*SmithSys)
	if !ok || !lvSys.IsOpen() {
		return false
	}

	lvSys.AddExp(count, logId)
	return true
}

func ditchTokensAddHandler(sys *MoneySys, mt uint32, count int64, bTip bool, logId pb3.LogId) bool {
	if count < 0 {
		return false
	}
	player := sys.owner
	player.AddDitchTokens(uint32(count))
	player.SetExtraAttr(attrdef.DitchTokens, attrdef.AttrValueAlias(player.GetDitchTokens()))
	player.UpdateStatics(model.FieldDitchTokens_, player.GetDitchTokens())
	player.UpdateStatics(model.FieldHistoryDitchTokens_, player.GetHistoryDitchTokens())
	return true
}

var yyMoneyTypeMap = map[uint32]uint32{
	moneydef.SpringFestivalShopMoney: yydefine.PYYSpringFestivalShop,
	moneydef.MysteryStoreMoney:       yydefine.PYYMysteryStore,
}

func yyMoneyAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	yyId, ok := yyMoneyTypeMap[mt]
	if !ok {
		return false
	}

	yyObjs := sys.owner.GetPYYObjList(yyId)
	for _, obj := range yyObjs {
		if !obj.IsOpen() {
			continue
		}
		handler, ok := obj.(iface.IYYMoneyHandler)
		if !ok {
			continue
		}
		if !handler.CanAddYYMoney(mt) {
			continue
		}
		moneyAddDefaultHandler(sys, mt, count, btip, logId)
		return true
	}

	return false
}

func fairyDelegationPointAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	asset := sys.GetMoney(uint32(mt))
	if asset >= int64(jsondata.GetFairyLandCommonConf().MaxDelegationPoint) {
		return true
	}

	player := sys.GetOwner()

	sys.GetBinaryData().Money[moneydef.FairyDelegationPoint] += count

	player.TriggerEvent(custom_id.AeMoneyChange, mt, count)
	sys.LogEconomy(uint32(mt), count, logId)
	sys.SendProto3(2, 1, &pb3.S2C_2_1{Moneys: map[uint32]int64{uint32(mt): sys.GetMoney(uint32(mt))}})
	if btip {
		player.SendTipMsg(tipmsgid.TpAddMoney, mt, count)
	}
	return true
}

func expAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	lvSys, ok := sys.GetOwner().GetSysObj(sysdef.SiLevel).(*LevelSys)
	if !ok {
		return false
	}
	if nil == lvSys {
		return false
	}

	lvSys.AddExp(count, logId, false, true)
	return true
}

func achievePointAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	achieveSys, ok := sys.GetOwner().GetSysObj(sysdef.SiAchieve).(*AchieveSys)
	if !ok {
		return false
	}
	if nil == achieveSys {
		return false
	}
	//添加成就货币
	moneyAddDefaultHandler(sys, mt, count, btip, logId)
	//成就升级但不消耗货币
	achieveSys.AddAchievePoint(count, logId)
	return true
}

func guildMoneyAddHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	guildSys, ok := sys.GetOwner().GetSysObj(sysdef.SiGuild).(*GuildSys)
	if !ok || nil == guildSys {
		return false
	}
	guild := guildSys.GetGuild()
	if nil == guild {
		return false
	}
	success := guild.AddMoney(count)
	return success
}

func chaosEnergyAddDefaultHandler(sys *MoneySys, mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	transfigurationSys, ok := sys.GetOwner().GetSysObj(sysdef.SiTransfiguration).(*TransfigurationSys)
	if !ok || !transfigurationSys.IsOpen() {
		return false
	}
	conf := jsondata.GetTransfigurationConf()
	if nil == conf {
		return false
	}
	player := sys.GetOwner()
	currentCount := sys.GetMoney(mt)
	maxStorage := transfigurationSys.GetMaxStorageChaosEnergyLimit()
	if currentCount >= maxStorage {
		return false
	}
	addCount := count
	if count+currentCount >= maxStorage {
		addCount = maxStorage - currentCount
	}

	sys.GetBinaryData().Money[mt] += addCount

	player.TriggerEvent(custom_id.AeMoneyChange, mt, count)
	sys.LogEconomy(mt, count, logId)
	sys.SendProto3(2, 1, &pb3.S2C_2_1{Moneys: map[uint32]int64{mt: sys.GetMoney(mt)}})
	if btip {
		player.SendTipMsg(tipmsgid.TpAddMoney, mt, count)
	}

	return true
}

// sub handlers =============================================

func unknownMoneySubHandler(sys *MoneySys, mt uint32, subCount int64, logId pb3.LogId) (bool, argsdef.RemoveMoneyMaps) {
	return false, argsdef.RemoveMoneyMaps{}
}

func moneySubDefaultHandler(sys *MoneySys, mt uint32, subCount int64, logId pb3.LogId) (bool, argsdef.RemoveMoneyMaps) {
	money := sys.GetMoney(uint32(mt))
	if money < subCount {
		sys.LogError("moneySubDefaultHandler: money not enough, mt=%d, money=%d, subCount=%d", mt, money, subCount)
		return false, argsdef.RemoveMoneyMaps{}
	}

	sys.GetBinaryData().Money[uint32(mt)] -= subCount

	player := sys.GetOwner()

	if !sys.isSkipTriggerMoneySub(logId) {
		player.TriggerEvent(custom_id.AeMoneyChange, mt, -subCount)
		sys.GetOwner().TriggerQuestEvent(custom_id.QttConsumeMoney, mt, subCount)
		sys.GetOwner().TriggerQuestEvent(custom_id.QttActConsumeMoney, mt, subCount)
	}

	sys.LogEconomy(uint32(mt), -subCount, logId)
	sys.LogInfo("moneySubDefaultHandler, mt:%d, subCount:%d, logId:%d", mt, subCount, logId)
	sys.SendProto3(2, 1, &pb3.S2C_2_1{Moneys: map[uint32]int64{uint32(mt): sys.GetMoney(uint32(mt))}})
	return true, argsdef.RemoveMoneyMaps{MoneyMap: map[uint32]int64{mt: subCount}}
}

func bindDiamondsSubHander(sys *MoneySys, mt uint32, subCount int64, logId pb3.LogId) (bool, argsdef.RemoveMoneyMaps) {
	bind := sys.GetMoney(moneydef.BindDiamonds)
	unbind := sys.GetMoney(moneydef.Diamonds)

	ret := argsdef.RemoveMoneyMaps{}
	if bind+unbind < subCount {
		return false, ret
	}

	if bind >= subCount {
		return moneySubDefaultHandler(sys, moneydef.BindDiamonds, subCount, logId)
	}

	unBindSubCount := subCount - bind
	successed, _ := moneySubDefaultHandler(sys, moneydef.Diamonds, unBindSubCount, logId)
	if !successed {
		return false, ret
	}

	successed, _ = moneySubDefaultHandler(sys, moneydef.BindDiamonds, bind, logId)
	if !successed {
		return false, ret
	}

	ret.MoneyMap = map[uint32]int64{
		moneydef.BindDiamonds: bind,
		moneydef.Diamonds:     unBindSubCount,
	}

	return true, ret
}

func ditchTokensSubHandler(sys *MoneySys, mt uint32, subCount int64, logId pb3.LogId) (bool, argsdef.RemoveMoneyMaps) {
	ret := argsdef.RemoveMoneyMaps{}
	if subCount < 0 {
		return false, ret
	}
	player := sys.GetOwner()
	if player.GetDitchTokens() < uint32(subCount) {
		sys.LogError("ditchTokensSubHandler: money not enough, ditchTokens=%d, subCount=%d", player.GetDitchTokens(), subCount)
		return false, ret
	}

	player.SubDitchTokens(uint32(subCount))
	player.SetExtraAttr(attrdef.DitchTokens, attrdef.AttrValueAlias(player.GetDitchTokens()))
	player.UpdateStatics(model.FieldDitchTokens_, player.GetDitchTokens())
	player.UpdateStatics(model.FieldHistoryDitchTokens_, player.GetHistoryDitchTokens())

	return true, ret
}

type MoneySys struct {
	Base
}

func (sys *MoneySys) IsOpen() bool {
	return true
}

func (sys *MoneySys) OnInit() {
	binary := sys.GetBinaryData()

	moneys := binary.Money
	if nil == moneys {
		moneys = make(map[uint32]int64)
		binary.Money = moneys
	}
}

func (sys *MoneySys) OnAfterLogin() {
	sys.s2cInfo()
}

func (sys *MoneySys) s2cInfo() {
	role := sys.GetBinaryData()
	sys.SendProto3(2, 1, &pb3.S2C_2_1{Moneys: role.Money})
}

func (sys *MoneySys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *MoneySys) OnOpen() {
	sys.s2cInfo()
}

// 不会自动处理货币转换关系
func (sys *MoneySys) GetMoney(mt uint32) int64 {
	if !moneydef.IsMoneyType(mt) {
		sys.LogError("GetMoney mt is not money type, mt: %d", mt)
		return 0
	}

	if mt == moneydef.DitchTokens {
		player := sys.GetOwner()
		return int64(player.GetDitchTokens())
	}

	moneys := sys.GetBinaryData().Money

	if num, ok := moneys[mt]; ok {
		return num
	}

	return 0
}

func (sys *MoneySys) CopyMoneys() map[uint32]int64 {
	moneys := make(map[uint32]int64)
	for k, v := range sys.GetBinaryData().Money {
		moneys[k] = v
	}
	return moneys
}

// 会自动处理货币转换关系
func (sys *MoneySys) GetMoneyCount(mt uint32) int64 {
	switch mt {
	case moneydef.BindDiamonds:
		return sys.GetMoney(moneydef.BindDiamonds) + sys.GetMoney(moneydef.Diamonds)
	default:
		return sys.GetMoney(mt)
	}
}

func (sys *MoneySys) LogEconomy(mt uint32, count int64, logId pb3.LogId) {
	if mt == moneydef.Exp {
		return
	}
	if mt == moneydef.LingQi && count > 0 {
		return // 获得灵气不大点
	}
	st := &pb3.LogEconomy{
		MoneyType: mt,
		LogId:     uint32(logId),
	}
	st.Amount = count
	st.Left = sys.GetMoney(mt)

	logworker.LogEconomy(sys.GetOwner(), st)
}

// 获得资源
func (sys *MoneySys) AddMoney(mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	if count == 0 {
		return true
	}

	if count < 0 {
		return false
	}

	if !moneydef.IsMoneyType(mt) {
		sys.LogError("mt %d is not monety type", mt)
		return false
	}

	hdl := moneyAddHandlers[mt]

	success := hdl(sys, mt, count, btip, logId)

	return success
}

func (sys *MoneySys) SetMoney(mt uint32, count int64, btip bool, logId pb3.LogId) bool {
	changed := count - sys.GetBinaryData().Money[uint32(mt)]

	if changed >= 0 {
		sys.AddMoney(mt, count, btip, logId)
	}

	if changed < 0 {
		sys.DeductMoney(mt, -changed, common.ConsumeParams{LogId: logId})
	}

	return true
}

// 扣资源
func (sys *MoneySys) DeductMoney(mt uint32, count int64, params common.ConsumeParams) bool {
	if count == 0 {
		return true
	}

	if count < 0 {
		return false
	}

	if !moneydef.IsMoneyType(mt) {
		return false
	}

	hdl := moneySubHandlers[mt]

	success, removeMoney := hdl(sys, mt, count, params.LogId)
	if success {
		if sys.isSkipTriggerMoneySub(params.LogId) {
			return success
		}
		if mt == moneydef.Diamonds || mt == moneydef.BindDiamonds {
			sys.owner.TriggerQuestEvent(custom_id.QttBuyItemUseDiamond, 0, count)
			sys.owner.TriggerEvent(custom_id.AeUseDiamond, count)
		}
		for rMt, rCount := range removeMoney.MoneyMap {
			sys.owner.TriggerEvent(custom_id.AeConsumeMoney, rMt, rCount, params)
		}
	}

	return success
}

func (sys *MoneySys) isSkipTriggerMoneySub(logId pb3.LogId) bool {
	var logIds = map[uint32]struct{}{
		uint32(pb3.LogId_LogChargeLotteryReBateRecDraw):  {}, // 招财喵喵投入
		uint32(pb3.LogId_LogPlaneChargeTurntableConsume): {}, // 福运连连投入
		uint32(pb3.LogId_LogSectAuctionConsume):          {}, // 仙宗拍卖消耗
		uint32(pb3.LogId_LogAuctionBidConsume):           {}, // 拍卖行-竞拍消耗
		uint32(pb3.LogId_LogLuckyTreasuresInput):         {}, // 招财进宝-投入
		uint32(pb3.LogId_LogMarryApply):                  {}, // 结婚申请消耗
	}
	if _, ok := logIds[uint32(logId)]; !ok {
		return false
	}

	if setIds := jsondata.GlobalU32Vec("ban_consume_behavior"); nil != setIds {
		if pie.Uint32s(setIds).Contains(uint32(logId)) {
			return false
		}
	}
	return true
}

func init() {
	RegisterSysClass(sysdef.SiMoney, func() iface.ISystem {
		return &MoneySys{}
	})

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		// Register add handlers
		for i := moneydef.MoneyUnknown; i <= moneydef.MoneyEnd; i++ {
			moneyAddHandlers = append(moneyAddHandlers, moneyAddDefaultHandler)
		}
		moneyAddHandlers[moneydef.MoneyUnknown] = unknownMoneyAddHandler
		moneyAddHandlers[moneydef.YuanBao] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.BindDiamonds] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.Diamonds] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.FairyDelegationPoint] = fairyDelegationPointAddHandler
		moneyAddHandlers[moneydef.Exp] = expAddHandler
		moneyAddHandlers[moneydef.AchievePoint] = achievePointAddHandler
		moneyAddHandlers[moneydef.GuildMoney] = guildMoneyAddHandler
		moneyAddHandlers[moneydef.TSPoint] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.ChaosEnergy] = chaosEnergyAddDefaultHandler
		moneyAddHandlers[moneydef.FlyCoin] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.PotentialExp] = potentialExpAddHandler
		moneyAddHandlers[moneydef.DitchTokens] = ditchTokensAddHandler
		moneyAddHandlers[moneydef.DomainEyeMoney] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.StarPavilionMoney1] = moneyAddDefaultHandler
		moneyAddHandlers[moneydef.StarPavilionMoney2] = moneyAddDefaultHandler

		gshare.SmithInstance.EachSmithRefDo(func(ref *gshare.SmithRef) {
			moneyAddHandlers[ref.SmithExpMoneyType] = smithExpAddHandler
		})

		for mt := range yyMoneyTypeMap {
			moneyAddHandlers[mt] = yyMoneyAddHandler
		}

		// Register sub handlers
		for i := moneydef.MoneyUnknown; i <= moneydef.MoneyEnd; i++ {
			moneySubHandlers = append(moneySubHandlers, moneySubDefaultHandler)
		}
		moneySubHandlers[moneydef.MoneyUnknown] = unknownMoneySubHandler
		moneySubHandlers[moneydef.YuanBao] = moneySubDefaultHandler
		moneySubHandlers[moneydef.BindDiamonds] = bindDiamondsSubHander
		moneySubHandlers[moneydef.GuildMoney] = unknownMoneySubHandler
		moneySubHandlers[moneydef.FairyStone] = moneySubDefaultHandler
		moneySubHandlers[moneydef.TSPoint] = moneySubDefaultHandler
		moneySubHandlers[moneydef.FlyCoin] = moneySubDefaultHandler
		moneySubHandlers[moneydef.DitchTokens] = ditchTokensSubHandler
		moneySubHandlers[moneydef.DomainEyeMoney] = moneySubDefaultHandler
		moneySubHandlers[moneydef.StarPavilionMoney1] = moneySubDefaultHandler
		moneySubHandlers[moneydef.StarPavilionMoney2] = moneySubDefaultHandler
	})

	engine.RegisterMessage(gshare.OfflineCmdDeductMoney, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineCmdDeductMoney)
}

func offlineCmdDeductMoney(player iface.IPlayer, msg pb3.Message) {
	sys, ok := player.GetSysObj(sysdef.SiMoney).(*MoneySys)
	if !ok || !sys.IsOpen() {
		return
	}
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		player.LogError("offlineCmdDeductMoney convert CommonSt failed")
		return
	}
	mt := st.U32Param
	count := st.U32Param2
	notReachDeduct := st.U32Param3
	money := sys.GetMoney(mt)

	if money >= int64(count) {
		sys.DeductMoney(mt, int64(count), common.ConsumeParams{LogId: pb3.LogId_LogGm})
		return
	}

	if notReachDeduct > 0 {
		sys.DeductMoney(mt, money, common.ConsumeParams{LogId: pb3.LogId_LogGm})
		return
	}
}
