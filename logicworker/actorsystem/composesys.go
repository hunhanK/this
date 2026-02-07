package actorsystem

/*
	desc:合成系统
	author: twl
	time:	2023/03/27
*/

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/composedef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

// ComposeSys 合成系统
type ComposeSys struct {
	Base
}

func getComposeRate(conf *jsondata.ComposeConf, itemNum int) uint32 {
	successRateMap := conf.SuccessRate
	if nil == successRateMap { // 没有配置的必定成功
		return 10000
	}
	rateVal, isExist := successRateMap[uint32(itemNum)]
	if !isExist { // 配置了找不到
		return 0
	}
	return rateVal.Id
}

// 通知
func (sys *ComposeSys) packSend(isSuccess bool) {
	sys.SendProto3(2, 30, &pb3.S2C_2_30{IsSuccess: isSuccess})
}

func (sys *ComposeSys) OnReconnect() {}

// 检查穿戴
func (sys *ComposeSys) checkOn(itemType uint32, Pos uint32, itemId uint32) bool {
	actor := sys.owner
	switch itemType {
	case itemdef.ItemTypeFairyWingPrincipaleEquip:
		obj := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
		return obj.principalSuit.EquipOnP(itemId)
	case itemdef.ItemTypeFairyWingDeputyEquip:
		obj := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
		return obj.deputySuit.EquipOnP(itemId)
	case itemdef.ItemRiderPrincipaleEquip:
		obj := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
		return obj.principalSuit.EquipOnP(itemId)
	case itemdef.ItemRiderDeputyEquip:
		obj := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
		return obj.deputySuit.EquipOnP(itemId)
	case itemdef.ItemFsDragonPrincipalEquip, itemdef.ItemFsTigerPrincipalEquip, itemdef.ItemFsRosefinchPrincipalEquip, itemdef.ItemFsTortoisePrincipalEquip:
		obj := actor.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
		fsType := GetFourSymbolsTypeByEqType(itemType)
		principalSuit, ok := obj.principalSuit[fsType]
		if ok {
			return principalSuit.EquipOnP(itemId)
		}
		return false
	case itemdef.ItemFsDragonDeputyEquip, itemdef.ItemFsTigerDeputyEquipEquip, itemdef.ItemFsRosefinchDeputyEquip, itemdef.ItemFsTortoiseDeputyEquip:
		obj := actor.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
		fsType := GetFourSymbolsTypeByEqType(itemType)
		deputySuit, ok := obj.deputySuit[fsType]
		if ok {
			return deputySuit.EquipOnP(itemId)
		}
		return false
	case itemdef.ItemTypeDragonEquip:
		obj := actor.GetSysObj(sysdef.SiDragon).(*DragonSys)
		return obj.EquipOnP(itemId)
	case itemdef.ItemsEdict:
		obj := actor.GetSysObj(sysdef.SiMageBody).(*MageBodySystem)
		return obj.EquipOnP(itemId)
	case itemdef.ItemTypeDomainSoul:
		obj := actor.GetSysObj(sysdef.SiDomainSoulEquip).(*DomainSoulEquipSys)
		return obj.EquipOnP(itemId)
	case itemdef.ItemTypeSoulHaloSkeleton:
		obj := actor.GetSysObj(sysdef.SiSoulHaloSkeleton).(*SoulHaloSkeletonSys)
		return obj.EquipOnP(itemId)
	}
	return false
}

// 更新"身上"的装备
func (sys *ComposeSys) updateBodyItem(itemType uint32, conf *jsondata.ItemConf, logId uint32) error {
	actor := sys.owner
	switch itemType {
	case itemdef.ItemTypeFairyWingPrincipaleEquip:
		fairyWing := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
		return fairyWing.principalSuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemTypeFairyWingDeputyEquip:
		fairyWing := actor.GetSysObj(sysdef.SiFairyWing).(*FairyWingSys)
		return fairyWing.deputySuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemRiderPrincipaleEquip:
		obj := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
		return obj.principalSuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemRiderDeputyEquip:
		obj := actor.GetSysObj(sysdef.SiRider).(*RiderSys)
		return obj.deputySuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemFsDragonPrincipalEquip, itemdef.ItemFsTigerPrincipalEquip, itemdef.ItemFsRosefinchPrincipalEquip, itemdef.ItemFsTortoisePrincipalEquip:
		obj := actor.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
		fsType := GetFourSymbolsTypeByEqType(itemType)
		principalSuit, ok := obj.principalSuit[fsType]
		if ok {
			return principalSuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
		}
		return nil
	case itemdef.ItemFsDragonDeputyEquip, itemdef.ItemFsTigerDeputyEquipEquip, itemdef.ItemFsRosefinchDeputyEquip, itemdef.ItemFsTortoiseDeputyEquip:
		obj := actor.GetSysObj(sysdef.SiFourSymbols).(*FourSymbolsSys)
		// 照着上面的写
		fsType := GetFourSymbolsTypeByEqType(itemType)
		deputySuit, ok := obj.deputySuit[fsType]
		if ok {
			return deputySuit.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
		}
		return nil
	case itemdef.ItemTypeDragonEquip:
		obj := actor.GetSysObj(sysdef.SiDragon).(*DragonSys)
		return obj.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemsEdict:
		obj := actor.GetSysObj(sysdef.SiMageBody).(*MageBodySystem)
		return obj.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	case itemdef.ItemTypeDomainSoul:
		obj := actor.GetSysObj(sysdef.SiDomainSoulEquip).(*DomainSoulEquipSys)
		return obj.TakeOnWithItemConfAndWithoutRewardToBag(actor, conf, logId)
	}
	return nil
}

// 检查合成条件
func (sys *ComposeSys) checkComposeNeed(conf *jsondata.ComposeConf) bool {
	condVec := conf.ComposeCondition
	if nil == condVec {
		return true
	}
	actor := sys.owner
	for _, condition := range condVec {
		needCount := condition.Count
		var currCount uint32
		switch condition.Type {
		case composedef.ComposeNeedXiuZhenLv:
			currCount = actor.GetCircle()
		case composedef.ComposeNeedFairyWingLv:
			currCount = GetFairyWingLv(actor, condition.Ids, nil)
		case composedef.ComposeNeedRiderLv:
			currCount = QuestRiderLv(actor, condition.Ids, nil)
		case composedef.ComposeNeedFourSymbolsLv:
			currCount = GetFourSymbolsLvByType(actor, condition.Ids, nil)
		case composedef.ComposeNeedEquipLv:
			currCount, _, _, _ = GetEquipLQSSByPos(actor, condition.Ids, nil)
		case composedef.ComposeNeedEquipQuality:
			_, currCount, _, _ = GetEquipLQSSByPos(actor, condition.Ids, nil)
		case composedef.ComposeNeedEquipStar:
			_, _, currCount, _ = GetEquipLQSSByPos(actor, condition.Ids, nil)
		case composedef.ComposeNeedEquipState:
			_, _, _, currCount = GetEquipLQSSByPos(actor, condition.Ids, nil)
		case composedef.ComposeNeedActorLv:
			currCount = actor.GetLevel()
		case composedef.ComposeNeedNirvanaLv:
			currCount = actor.GetNirvanaLevel()
		}
		if needCount > currCount {
			actor.SendTipMsg(tipmsgid.TpComposeCondNotEnough)
			return false
		}
	}
	return true
}

// compose 请求合成
func (sys *ComposeSys) compose(confId uint32, consumeHdl uint64, itemHandle []uint64, exItemHandle []uint64, autoBuy bool, composeCount uint32, isBody bool) {
	actor := sys.owner
	conf := jsondata.GetComposeConf(confId)
	if nil == conf {
		actor.SendTipMsg(tipmsgid.TpComposeConfNil)
		return
	}
	bagSys := actor.GetSysObj(sysdef.SiBag).(*BagSystem)
	if bagSys == nil {
		actor.SendTipMsg(tipmsgid.TpComposeBagSysNil)
		return
	}
	// 是否满足合成的前置条件要求
	if !sys.checkComposeNeed(conf) {
		return
	}

	// 因为不知道外面是怎么传进来的 所以在这里做一层过滤
	itemHandle = pie.Uint64s(itemHandle).Unique()
	exItemHandle = pie.Uint64s(exItemHandle).Unique()

	// 取消耗 判断是否可以合成( 成功率> 0
	successRate := getComposeRate(conf, len(itemHandle)+len(exItemHandle))
	if successRate <= 0 {
		actor.SendTipMsg(tipmsgid.TpComposeRateIsZero)
		return
	}
	// 判断是否是装备, 是装备: 先生成合成后的装备, 然后替换身上的装备 再移除身上的装备 全程不通过背包
	// 打log
	switch conf.ComposeType {
	case composedef.ComposeTypeEquip:
		sys.composeEquip(successRate, conf, itemHandle, exItemHandle)
	case composedef.ComposeTypeQuality:
		sys.composeEquipQuality(successRate, conf)
	case composedef.ComposeTypeItem:
		sys.composeItem(conf, autoBuy, composeCount)
	case composedef.ComposeTypeBattleHelpItem:
		sys.composeBattleHelpItem(conf)
	case composedef.ComposeTypeDragonEquip:
		sys.composeDragonEquItem(conf)
	case composedef.ComposeTypeGodBeastEquip:
		sys.composeGodBeastItem(conf, itemHandle)
	case composedef.ComposeTypeEdictEquip:
		sys.composeEdictEquip(conf)
	case composedef.ComposeTypeBattleSoulGodEquip:
		sys.composeBattleSoulGodEquipMultiple(conf, composeCount)
	case composedef.ComposeTypeDragonVeinEquip:
		sys.composeDragonVeinEquip(conf, itemHandle)
	case composedef.ComposeTypeFairySword:
		sys.composeFairySwordEquip(conf)
	case composedef.ComposeTypeFairySpirit:
		sys.composeFairySpiritEqu(conf, itemHandle)
	case composedef.ComposeTypeFlyingSwordEquip:
		sys.composeFlyingSwordEquipMultiple(conf, composeCount)
	case composedef.ComposeTypeSourceSoulEquip:
		sys.composeSourceSoulEquip(successRate, conf, itemHandle, exItemHandle)
	case composedef.ComposeTypeBlood:
		sys.composeBlood(conf, consumeHdl, itemHandle, exItemHandle)
	case composedef.ComposeTypeFeatherGen:
		sys.composeFeatherGen(conf, autoBuy, composeCount)
	case composedef.ComposeTypeFeatherEqu:
		sys.composeFeatherEqu(conf, composeCount)
	case composedef.ComposeTypeDomainSoul:
		sys.composeDomainSoul(conf)
	case composedef.ComposeTypeDomainEyeRune:
		sys.composeDomainEyeRune(conf, consumeHdl)
	case composedef.ComposeTypeSaNewSpirit:
		sys.composeTypeSaNewSpirit(successRate, conf, itemHandle, exItemHandle)
	case composedef.ComposeTypeSoulHaloSkeleton:
		sys.composeTypeSoulHaloSkeleton(conf, itemHandle, isBody)
	}
}

// c2sCompose 请求合成
func (sys *ComposeSys) c2sCompose(msg *base.Message) {
	var req pb3.C2S_2_30
	err := pb3.Unmarshal(msg.Data, &req)
	if err != nil {
		return
	}
	sys.compose(req.GetConfId(), req.ConsumeHdl, req.GetItemHandle(), req.GetExItemHandle(), req.GetAutoBuy(), req.GetComposeCount(), req.GetIsBody())
}

// 通过handle消耗
func (sys *ComposeSys) consumeWithHandler(fixConsume jsondata.ConsumeVec, resHandles []uint64, logId pb3.LogId, statics *pb3.LogCompose) bool {
	consumer := sys.owner.GetConsumer()
	normalizedConsum, valid := consumer.NormalizeConsumeConf(fixConsume, false)
	if !valid {
		return false
	}

	handlerItemMap := make(map[uint64]uint32)

	// 先把handle道具加到标准消耗的参数里面
	for _, handle := range resHandles {
		system := sys.GetOwner().GetBelongBagSysByHandle(handle)
		if system == nil {
			return false
		}
		bagSys, ok := system.(iface.IBagSys)
		if !ok {
			return false
		}
		item := bagSys.FindItemByHandle(handle)
		if item == nil {
			return false
		}

		// 堆叠不对
		if item.GetCount() > 1 {
			return false
		}

		normalizedConsum.ItemMap[item.ItemId] += item.GetCount()
		handlerItemMap[item.Handle] = item.GetItemId()
	}

	removeMaps, _, valid := consumer.CalcRemoveAndBuyItem(normalizedConsum, common.ConsumeParams{LogId: logId})
	if !valid {
		return valid
	}

	if nil == statics.Items {
		statics.Items = make(map[uint32]uint32)
	}
	for itemId, count := range removeMaps.ItemMap {
		statics.Items[itemId] += uint32(count)
	}

	if nil == statics.Moneys {
		statics.Moneys = make(map[uint32]uint32)
	}
	for moneyType, count := range removeMaps.MoneyMap {
		statics.Moneys[moneyType] += uint32(count)
	}

	removeMapsUsedToTriggerEvent := removeMaps.Copy()

	for handle, itemId := range handlerItemMap {
		system := sys.GetOwner().GetBelongBagSysByHandle(handle)
		if system == nil {
			return false
		}
		bagSys, ok := system.(iface.IBagSys)
		if !ok {
			return false
		}
		bagSys.RemoveItemByHandle(handle, logId)
		removeMaps.ItemMap[itemId] -= 1
	}

	// 扣除资源
	consumer.RemoveResource(removeMaps, common.ConsumeParams{LogId: logId})

	consumer.TriggerConsumeEvent(removeMapsUsedToTriggerEvent, normalizedConsum.AutoBuyItemMap, nil)

	return true
}

// 发送tip
func (sys *ComposeSys) sendTips(success bool, conf *jsondata.ComposeConf) {
	if !success {
		return
	}
	if !conf.SendTips {
		return
	}
	itemConf := jsondata.GetItemConfig(conf.ItemId)
	if itemConf == nil {
		return
	}
	owner := sys.GetOwner()
	switch itemConf.Type {
	case composedef.ComposeTypeEquip:
		owner.SendTipMsg(tipmsgid.TpComposeSendTipByEquip, owner.GetId(), owner.GetName(), itemConf.Star, itemConf.Id)
	}
}

func init() {
	RegisterSysClass(sysdef.SiCompose, func() iface.ISystem {
		return &ComposeSys{}
	})
	net.RegisterSysProto(2, 30, sysdef.SiCompose, (*ComposeSys).c2sCompose)
}
