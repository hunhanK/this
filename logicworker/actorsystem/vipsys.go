package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

type VipSys struct {
	Base
}

func (sys *VipSys) OnInit() {
	binary := sys.GetBinaryData()
	if nil == binary.GetVipLimitBuyRecords() {
		binary.VipLimitBuyRecords = make(map[uint32]uint32)
	}
	sys.owner.SetExtraAttr(attrdef.VipLevel, attrdef.AttrValueAlias(sys.GetBinaryData().GetVip()))
	sys.owner.SetExtraAttr(attrdef.VipExp, attrdef.AttrValueAlias(sys.GetBinaryData().VipExp))
}

func (sys *VipSys) OnOpen() {
}

func (sys *VipSys) OnAfterLogin() {
	sys.SendProto3(38, 50, &pb3.S2C_38_50{Records: sys.GetBinaryData().GetVipLimitBuyRecords()})
	sys.sendVipGiftState()
	sys.sendExpProgress()
}

func (sys *VipSys) OnReconnect() {
	sys.SendProto3(38, 50, &pb3.S2C_38_50{Records: sys.GetBinaryData().GetVipLimitBuyRecords()})
	sys.sendVipGiftState()
	sys.sendExpProgress()
}

func (sys *VipSys) onNewDay() {
	state := sys.GiftState()
	state.LastDayVip = sys.owner.GetVipLevel()
	sys.sendVipGiftState()
}

func (sys *VipSys) sendVipGiftState() {
	state := sys.GiftState()
	var receivedDailyLvs []uint32
	for lv, dailyReceivedAt := range state.ReceivedDayGifts {
		if lv >= state.LastDayVip {
			if time.Unix(int64(dailyReceivedAt), 0).Format("20060102") == time.Now().Format("20060102") {
				receivedDailyLvs = append(receivedDailyLvs, lv)
			}
		} else {
			receivedDailyLvs = append(receivedDailyLvs, lv)
		}
	}
	sys.SendProto3(38, 53, &pb3.S2C_38_53{
		ReceivedLevelGifts: state.ReceivedLevelGifts,
		ReceivedDayGifts:   receivedDailyLvs,
	})
}

func (sys *VipSys) GiftState() *pb3.VipGifState {
	binary := sys.GetBinaryData()

	if nil == binary.VipGifState {
		binary.VipGifState = &pb3.VipGifState{
			ReceivedDayGifts: map[uint32]uint32{},
		}
	}

	if binary.VipGifState.ReceivedDayGifts == nil {
		binary.VipGifState.ReceivedDayGifts = map[uint32]uint32{}
	}

	return binary.VipGifState
}

func (sys *VipSys) c2sAcceptReward(msg *base.Message) {
	var req pb3.C2S_38_52
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return
	}

	if sys.acceptReward(req.Type, req.Lv) {
		sys.SendProto3(38, 52, &pb3.S2C_38_52{
			Type: req.Type,
			Lv:   req.Lv,
		})
	}
}

func (sys *VipSys) c2sBuyVipLimit(msg *base.Message) error {
	var req pb3.C2S_38_51
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	binary := sys.GetBinaryData()
	conf := jsondata.GetVipBuyConf(req.GetId())
	if nil == conf {
		return neterror.ParamsInvalidError("vip limitbuy conf(%d) is nil", req.GetId())
	}
	vip := sys.owner.GetVipLevel()
	if vip < conf.NeedVipLv {
		sys.owner.SendTipMsg(tipmsgid.TpVipLvNotEnough)
		return nil
	}
	record := binary.GetVipLimitBuyRecords()
	if record[req.GetId()] >= conf.Count {
		sys.owner.SendTipMsg(tipmsgid.TpBuyTimesLimit)
		return nil
	}

	if !engine.CheckRewards(sys.owner, conf.Award) {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}
	if !sys.owner.DeductMoney(conf.MoneyType, int64(conf.Money), common.ConsumeParams{
		LogId:   pb3.LogId_LogVipLimitBuy,
		SubType: conf.Type,
	}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	engine.GiveRewards(sys.owner, conf.Award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogVipReward})
	record[req.GetId()]++
	sys.SendProto3(38, 51, &pb3.S2C_38_51{
		Id:    req.GetId(),
		Count: record[req.GetId()],
	})

	if conf.NeedBroadcast {
		engine.BroadcastTipMsgById(tipmsgid.VipPrivilege, sys.owner.GetId(), sys.owner.GetName(), conf.NeedVipLv, engine.StdRewardToBroadcast(sys.owner, conf.Award))
	}

	return nil

}

func (sys *VipSys) acceptReward(rewardType uint32, lv uint32) bool {
	switch rewardType {
	case uint32(pb3.VipRewardType_VipRewardTypeLevel):
		return sys.levelReward(lv)
	case uint32(pb3.VipRewardType_VipRewardTypeDaily):
		return sys.dailyReward(lv)
	default:
		return false
	}
}

func (sys *VipSys) levelReward(level uint32) bool {
	state := sys.GiftState()
	if nil == state {
		return false
	}

	received := state.ReceivedLevelGifts
	receivedSet := map[uint32]struct{}{}
	for _, lv := range received {
		receivedSet[lv] = struct{}{}
	}

	if _, ok := receivedSet[level]; ok {
		return false
	}

	conf := jsondata.GetVipLevelConfByLevel(level)
	if nil == conf {
		return false
	}

	if conf.Level > sys.owner.GetVipLevel() {
		return false
	}

	if !engine.CheckRewards(sys.owner, conf.LevelGift) {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return false
	}

	if !engine.GiveRewards(sys.owner, conf.LevelGift, common.EngineGiveRewardParam{LogId: pb3.LogId_LogVipReward}) {
		return false
	}

	state.ReceivedLevelGifts = append(state.ReceivedLevelGifts, level)

	return true
}

func (sys *VipSys) dailyReward(level uint32) bool {
	state := sys.GiftState()
	if nil == state {
		return false
	}

	received := state.ReceivedDayGifts
	lastDayVip := state.LastDayVip

	if level >= lastDayVip { //0点的vip等级以及今日最高达到今日可领
		if lastReceivedAt, ok := received[level]; ok {
			if time.Unix(int64(lastReceivedAt), 0).Format("20060102") == time.Now().Format("20060102") {
				return false
			}
		}
	} else { //达到过但没有领
		if _, ok := received[level]; ok {
			return false
		}
	}

	conf := jsondata.GetVipLevelConfByLevel(level)
	if nil == conf {
		return false
	}

	if conf.Level > sys.owner.GetVipLevel() {
		return false
	}

	if !engine.CheckRewards(sys.owner, conf.DayGift) {
		sys.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return false
	}

	if !engine.GiveRewards(sys.owner, conf.DayGift, common.EngineGiveRewardParam{LogId: pb3.LogId_LogVipReward}) {
		return false
	}

	state.ReceivedDayGifts[level] = uint32(time.Now().Unix())

	return true
}

func (sys *VipSys) sendExpProgress() {
	expProgress := uint32(sys.owner.GetExtraAttr(attrdef.VipExp))
	sys.SendProto3(38, 54, &pb3.S2C_38_54{
		ExpProgress: expProgress,
	})
}

func (sys *VipSys) AddExp(exp uint32) {
	if exp <= 0 {
		return
	}

	defer func() {
		sys.sendExpProgress()
	}()

	totalExp := sys.owner.GetExtraAttr(attrdef.VipExp) + int64(exp)
	sys.owner.SetExtraAttr(attrdef.VipExp, totalExp)
	sys.GetBinaryData().VipExp = uint32(totalExp)

	newLv := sys.calcVipLevel(uint32(totalExp))
	if newLv == sys.owner.GetVipLevel() {
		return
	}

	sys.UpLv(newLv)
}

func (sys *VipSys) UpLv(newLv uint32) {
	sys.LogInfo("%s,%d vip等级变化:vipLevel:->%d", sys.owner.GetName(), sys.owner.GetId(), newLv)
	sys.GetOwner().TriggerQuestEvent(custom_id.QttFirstOrNextDayLogin, 0, 1)

	newLvConf := jsondata.GetVipLevelConfByLevel(newLv)
	if newLvConf == nil {
		sys.LogWarn("not found %d lv conf", newLv)
		return
	}
	oldLv := sys.owner.GetVipLevel()
	olvLvConf := jsondata.GetVipLevelConfByLevel(oldLv)
	if olvLvConf == nil {
		sys.LogWarn("not found %d lv conf", oldLv)
		return
	}
	if oldLv == 0 {
		sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem).OnEnlargeSize(newLvConf.ExpandBagGridNum)
	} else {
		if newLvConf.ExpandBagGridNum > olvLvConf.ExpandBagGridNum {
			sys.owner.GetSysObj(sysdef.SiBag).(*BagSystem).OnEnlargeSize(newLvConf.ExpandBagGridNum - olvLvConf.ExpandBagGridNum)
		}
	}

	sys.owner.GetBinaryData().Vip = newLv
	sys.owner.SetExtraAttr(attrdef.VipLevel, int64(newLv))
	sys.owner.TriggerEvent(custom_id.AeVipLevelUp, oldLv, newLv)
	sys.owner.TriggerEvent(custom_id.AePrivilegeRelatedSysChanged)
	sys.ResetSysAttr(attrdef.SaVip)
}

func (sys *VipSys) calcVipLevel(exp uint32) uint32 {
	conf := jsondata.GetVipConf()
	if nil == conf {
		return 0
	}

	var vipLevel uint32
	for _, cfg := range conf {
		if exp >= cfg.NeedScore {
			vipLevel = cfg.Level
			continue
		}
		break
	}
	return vipLevel
}

func calcVipProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys := player.GetSysObj(sysdef.SiVip).(*VipSys)
	conf := jsondata.GetVipLevelConfByLevel(sys.owner.GetVipLevel())
	if nil == conf {
		return
	}

	engine.AddAttrsToCalc(player, calc, conf.Attrs)
}

func init() {
	RegisterSysClass(sysdef.SiVip, func() iface.ISystem {
		return &VipSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		sys, ok := actor.GetSysObj(sysdef.SiVip).(*VipSys)
		if !ok {
			return
		}

		if !sys.IsOpen() {
			return
		}

		sys.onNewDay()
	})

	engine.RegAttrCalcFn(attrdef.SaVip, calcVipProperty)

	net.RegisterSysProto(38, 51, sysdef.SiVip, (*VipSys).c2sBuyVipLimit)
	net.RegisterSysProto(38, 52, sysdef.SiVip, (*VipSys).c2sAcceptReward)

	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		vipLv := player.GetVipLevel()

		if len(conf.Vip) == 0 {
			return 0, nil
		}

		vipLv = utils.MinUInt32(vipLv, uint32(len(conf.Vip)-1))

		return int64(conf.Vip[vipLv]), nil
	})

	manager.RegisterSettingChangeTriggerFunc(custom_id.Setting_HideVipLevel, func(player iface.IPlayer, old, new bool) {
		if new {
			player.SetExtraAttr(attrdef.HideVipLevel, attrdef.AttrValueAlias(1))
		} else {
			player.SetExtraAttr(attrdef.HideVipLevel, attrdef.AttrValueAlias(0))
		}
	})
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddVipExp, handleUseItemAddVipExp)
}

func handleUseItemAddVipExp(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) == 0 {
		return
	}
	if conf.Param[0] == 0 {
		return
	}
	addVipExp := conf.Param[0]
	totalVipExp := uint32(param.Count) * addVipExp
	if totalVipExp == 0 {
		return
	}
	player.AddVipExp(totalVipExp)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogUseItemAddVipExp, &pb3.LogPlayerCounter{
		NumArgs: uint64(param.ItemId),
		StrArgs: fmt.Sprintf("%d", totalVipExp),
	})
	return true, true, param.Count
}
