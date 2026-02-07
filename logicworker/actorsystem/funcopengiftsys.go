/**
 * @Author: lzp
 * @Date: 2025/2/7
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/srvlib/utils"
)

type FuncOpenGiftSys struct {
	Base
}

func (sys *FuncOpenGiftSys) OnOpen() {
	sys.s2cInfo()
}

func (sys *FuncOpenGiftSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *FuncOpenGiftSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *FuncOpenGiftSys) s2cInfo() {
	sys.SendProto3(2, 200, &pb3.S2C_2_200{
		GiftIds: sys.GetBinaryData().FuncOpenGifts,
	})
}

func funcOpenGiftCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sys, ok := player.GetSysObj(sysdef.SiFuncOpenGift).(*FuncOpenGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	gConf := jsondata.GetFuncOpenGiftConf(conf.ChargeId)
	if gConf == nil {
		return false
	}
	if gshare.GetOpenServerDay() < gConf.OpenSrvDay {
		return false
	}
	binData := sys.GetBinaryData()
	return !utils.SliceContainsUint32(binData.FuncOpenGifts, conf.ChargeId)
}

func funcOpenGiftChargeBack(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	sys, ok := player.GetSysObj(sysdef.SiFuncOpenGift).(*FuncOpenGiftSys)
	if !ok || !sys.IsOpen() {
		return false
	}
	gConf := jsondata.GetFuncOpenGiftConf(conf.ChargeId)
	if gConf == nil {
		return false
	}

	binData := sys.GetBinaryData()
	binData.FuncOpenGifts = append(binData.FuncOpenGifts, conf.ChargeId)

	if len(gConf.Rewards) > 0 {
		engine.GiveRewards(player, gConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFuncOpenGiftAward})
	}
	sys.s2cInfo()
	player.TriggerEvent(custom_id.AeBuyFuncOpenGift)
	return true
}

func init() {
	RegisterSysClass(sysdef.SiFuncOpenGift, func() iface.ISystem {
		return &FuncOpenGiftSys{}
	})

	engine.RegChargeEvent(chargedef.FuncOpenGift, funcOpenGiftCheck, funcOpenGiftChargeBack)
}
