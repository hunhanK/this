package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

// 回血回调
func resumeHp(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	//maxHp := player.GetFightAttr(attrdef.MaxHp)

	if len(conf.Param) < 1 {
		player.LogError("itemId %d useItem param error", conf.ItemId)
		return false, false, 0
	}

	if param.Count > 1 {
		player.LogError("itemId %d useItem param error", conf.ItemId)
		return false, false, 0
	}

	buffId := conf.Param[0]
	player.AddBuff(buffId)

	return true, true, param.Count
}

func s2cEquipGiftInfo(actor iface.IPlayer) {
	binary := actor.GetBinaryData()
	actor.SendProto3(12, 11, &pb3.S2C_12_11{Gifts: binary.GetEquipGifts()})
}

func c2sEquipGiftBuy(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_12_12
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	binary := player.GetBinaryData()
	if utils.SliceContainsUint32(binary.EquipGifts, req.Id) {
		return neterror.ParamsInvalidError("has buy %d", req.Id)
	}

	conf := jsondata.GetEquipGiftConf(req.GetId())
	if nil == conf {
		return neterror.ConfNotFoundError("equip gift(%d) not exist", req.Id)
	}

	if !player.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipGift}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	binary.EquipGifts = append(binary.EquipGifts, req.Id)
	engine.GiveRewards(player, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogEquipGift})

	player.SendProto3(12, 12, &pb3.S2C_12_12{Id: req.Id})

	return nil
}

func c2sRevDownloadAward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_12_25
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	binary := player.GetBinaryData()
	if binary.DownloadAwardsRev {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	vec := jsondata.GlobalU32Vec("downloadAwards")
	if len(vec) < 2 {
		return neterror.ConfNotFoundError("awards is nil")
	}

	var rewards jsondata.StdRewardVec
	for i := 0; i+1 < len(vec); i += 2 {
		rewards = append(rewards, &jsondata.StdReward{
			Id:    vec[i],
			Count: int64(vec[i+1]),
		})
	}

	binary.DownloadAwardsRev = true
	s2cDownloadAwardsRev(player)

	mailmgr.SendMailToActor(player.GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_ResDownloadAwards,
		Rewards: rewards,
	})

	return nil
}

func c2sRevDownloadMircoAward(player iface.IPlayer, msg *base.Message) error {
	if !engine.Is360Wan(player.GetExtraAttrU32(attrdef.DitchId)) {
		player.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	var req pb3.C2S_12_26
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	binary := player.GetBinaryData()
	if binary.DownloadMicroAwardsRev {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	onlineTime := jsondata.GlobalUint("downMicroEnd")
	if onlineTime != 0 && onlineTime > player.GetDayOnlineTime() {
		player.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	vec := jsondata.GlobalU32Vec("downMicroEnd")
	if len(vec) < 2 {
		return neterror.ConfNotFoundError("awards is nil")
	}

	var rewards jsondata.StdRewardVec
	for i := 0; i+1 < len(vec); i += 2 {
		rewards = append(rewards, &jsondata.StdReward{
			Id:    vec[i],
			Count: int64(vec[i+1]),
		})
	}

	binary.DownloadMicroAwardsRev = true
	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDownloadMicroAwards})
		player.SendShowRewardsPop(rewards)
	}
	s2cDownloadMicroAwardsRev(player)

	return nil
}

func s2cDownloadAwardsRev(player iface.IPlayer) {
	binary := player.GetBinaryData()
	player.SendProto3(12, 25, &pb3.S2C_12_25{IsRev: binary.DownloadAwardsRev})
}

func s2cDownloadMicroAwardsRev(player iface.IPlayer) {
	binary := player.GetBinaryData()
	player.SendProto3(12, 26, &pb3.S2C_12_26{IsRev: binary.DownloadMicroAwardsRev})
}

func s2cLogicToCross(player iface.IPlayer) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CPlayerLogicWithoutEntity, &pb3.G2CPlayerLogicWithoutEntity{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: player.GetId(),
	})
}

func s2cLogicToReconnect(player iface.IPlayer) {
	engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CPlayerReconnectWithoutEntity, &pb3.G2CPlayerReconnectWithoutEntity{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: player.GetId(),
	})
}

func calcGmAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	gmAttr := player.GetGmAttr()
	if gmAttr == nil {
		return
	}

	for k, v := range gmAttr {
		engine.CheckAddAttrsToCalc(player, calc, jsondata.AttrVec{
			{
				Type:  k,
				Value: uint32(v),
			},
		})
	}
}

func init() {
	engine.RegAttrCalcFn(attrdef.SaGmAttr, calcGmAttr)

	net.RegisterProto(12, 12, c2sEquipGiftBuy)

	net.RegisterProto(12, 25, c2sRevDownloadAward)
	net.RegisterProto(12, 26, c2sRevDownloadMircoAward)

	event.RegActorEvent(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		s2cEquipGiftInfo(player)
		s2cDownloadAwardsRev(player)
		s2cDownloadMicroAwardsRev(player)
		s2cLogicToCross(player)
	})
	event.RegActorEvent(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		s2cEquipGiftInfo(player)
		s2cDownloadAwardsRev(player)
		s2cDownloadMicroAwardsRev(player)
		s2cLogicToReconnect(player)
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemResumeHp, resumeHp)

	event.RegActorEvent(custom_id.AeLogin, func(player iface.IPlayer, args ...interface{}) {
		player.TriggerQuestEventRange(custom_id.QttLoginAfterSeverOpenDay)
	})

	engine.RegQuestTargetProgress(custom_id.QttLoginAfterSeverOpenDay, func(player iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) < 1 {
			return 0
		}
		openDay := gshare.GetOpenServerDay()
		if openDay >= ids[0] {
			return 1
		}
		return 0
	})

	gmevent.Register("gmAttr", func(player iface.IPlayer, args ...string) bool {
		attrType := utils.AtoUint32(args[0])
		attrVal := utils.AtoUint32(args[1])
		player.SetGmAttr(attrType, int64(attrVal))
		return true
	}, 1)
}
