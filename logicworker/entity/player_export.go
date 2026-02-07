package entity

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/mailmgr"
)

func (player *Player) GetCircleLevel() uint32 {
	return uint32(player.GetExtraAttr(attrdef.Circle))
}

// GetServerId 获取原始服务器id
func (player *Player) GetServerId() uint32 {
	_, srvId := base.GetPfIdAndSrvIdByPlayerId(player.GetId())
	return srvId
}

// GetPfId 获取原始服务器平台id
func (player *Player) GetPfId() uint32 {
	pfId, _ := base.GetPfIdAndSrvIdByPlayerId(player.GetId())
	return pfId
}

func (player *Player) TriggerMoneyChange(mt uint32, changeNum int64) {
	player.TriggerEvent(custom_id.AeMoneyChange, mt, changeNum)
}

func (player *Player) SendMail(mail *mailargs.SendMailSt) {
	mailmgr.SendMailToActor(player.PlayerData.GetActorId(), mail)
}

func (player *Player) CheckItemCond(conf *jsondata.ItemConf) bool {
	if conf.OpenDay > 0 && conf.OpenDay > gshare.GetOpenServerDay() {
		return false
	}
	//职业
	if conf.Job > 0 && player.GetJob() != uint32(conf.Job) {
		return false
	}
	//性别
	if conf.Sex > 0 && player.GetSex() != uint32(conf.Sex) {
		return false
	}
	//转生
	if conf.Circle > 0 && conf.Circle > player.GetCircle() {
		return false
	}
	//涅槃转生
	if conf.NirvanaLevel > 0 && conf.NirvanaLevel > player.GetNirvanaLevel() {
		return false
	}

	if player.GetLevel() < conf.Level {
		return false
	}

	return true
}

func (player *Player) IsInTrialActive(activeType uint32, args []uint32) bool {
	sys := player.GetTrialActiveSys()
	if nil == sys || !sys.IsOpen() {
		return false
	}
	return sys.IsInTrialActive(activeType, args)
}

func (player *Player) StopTrialActive(activeType uint32, args []uint32) {
	sys := player.GetTrialActiveSys()
	if nil == sys || !sys.IsOpen() {
		return
	}
	sys.StopTrialActive(activeType, args)
}

func (player *Player) GetDrawLibConf(lib uint32) *jsondata.LotteryLibConf {
	conf := jsondata.GetLotteryLibConf(lib)
	if nil == conf {
		return nil
	}

	var checkPyyClass = []uint32{yydefine.YYDrawRateModify, yydefine.PYYBigGacha}
	var checkYYClass = []uint32{yydefine.YYSummerSurfDiamond}

	checkPyy := func() *jsondata.LotteryLibConf {
		for _, yyId := range checkPyyClass {
			//是否需要替换
			yyList := pyymgr.GetPlayerAllYYObj(player, yyId)
			if len(yyList) == 0 {
				continue
			}

			for i := range yyList {
				v := yyList[i]
				if !v.IsOpen() {
					continue
				}
				sys, ok := v.(iface.IYYLotteryChangeLibConf)
				if !ok {
					continue
				}
				if changeConf := sys.GetChangeLibConf(lib); nil != changeConf {
					return changeConf
				}
			}
		}
		return nil
	}

	checkYY := func() *jsondata.LotteryLibConf {
		for _, yyId := range checkYYClass {
			//是否需要替换
			yyList := yymgr.GetAllYY(yyId)
			if len(yyList) == 0 {
				continue
			}

			for i := range yyList {
				v := yyList[i]
				if !v.IsOpen() {
					continue
				}
				sys, ok := v.(iface.IYYLotteryChangeLibConf)
				if !ok {
					continue
				}
				if changeConf := sys.GetChangeLibConf(lib); nil != changeConf {
					return changeConf
				}
			}
		}
		return nil
	}

	replaceConf := checkPyy()
	if nil != replaceConf {
		return replaceConf
	}

	replaceConf = checkYY()
	if nil != replaceConf {
		return replaceConf
	}

	return conf
}

func (player *Player) SetGmAttr(attrType uint32, attrVal int64) {
	if player.gmAttr == nil {
		player.gmAttr = make(map[uint32]int64)
	}
	if attrType >= attrdef.FightPropBegin && attrType <= attrdef.FightPropEnd {
		player.gmAttr[attrType] = attrVal
	}
	if attrSys := player.GetAttrSys(); nil != attrSys {
		attrSys.ResetSysAttr(attrdef.SaGmAttr)
	}
}

func (player *Player) GetGmAttr() map[uint32]int64 {
	return player.gmAttr
}

func (player *Player) GetDailyChargeMoney(timestamp uint32, isReal bool) uint32 {
	if isReal {
		chargeInfo := player.GetBinaryData().GetRealChargeInfo()
		if nil == chargeInfo {
			return 0
		}
		return chargeInfo.DailyChargeMoneyMap[timestamp]
	} else {
		chargeInfo := player.GetBinaryData().GetChargeInfo()
		if nil == chargeInfo {
			return 0
		}
		return chargeInfo.DailyChargeMoneyMap[timestamp]
	}
}

func (player *Player) GetDailyCharge(isReal bool) uint32 {
	if isReal {
		chargeInfo := player.GetBinaryData().GetRealChargeInfo()
		if nil == chargeInfo {
			return 0
		}
		return chargeInfo.DailyChargeMoney
	} else {
		chargeInfo := player.GetBinaryData().GetChargeInfo()
		if nil == chargeInfo {
			return 0
		}
		return chargeInfo.DailyChargeMoney
	}
}
