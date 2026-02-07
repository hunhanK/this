/**
 * @Author: DaiGuanYu
 * @Desc: 充值回调
 * @Date: 2021/8/31 15:06
 */

package engine

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

type ChargeCheckFun func(actor iface.IPlayer, conf *jsondata.ChargeConf) bool
type ChargeFun func(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool

var (
	ChargeCheckFunc = make(map[uint32]ChargeCheckFun)
	ChargeCallBack  = make(map[uint32]ChargeFun)
)

// RegSysEvent 注册服务器事件回调函数
func RegChargeEvent(chargeType uint32, checkFn ChargeCheckFun, callbackFn ChargeFun) {
	if nil != checkFn {
		regChargeCheckFunEvent(chargeType, checkFn)
	}

	if nil != callbackFn {
		regChargeCallBackEvent(chargeType, callbackFn)
	}
}

// RegSysEvent 注册服务器事件回调函数
func regChargeCallBackEvent(id uint32, fn ChargeFun) {
	_, ok := ChargeCallBack[id]
	if ok {
		logger.LogError("充值类型重复注册 id:%d", id)
		return
	}
	ChargeCallBack[id] = fn
}

func TriggerChargeCallBackEvent(id uint32, actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	if fnSet, ok := ChargeCallBack[id]; ok {
		return fnSet(actor, conf, params)
	}
	logger.LogError("TriggerChargeCallBackEvent not found type%v", id)
	return false
}

// RegSysEvent 注册档位购买权限
func regChargeCheckFunEvent(id uint32, fn ChargeCheckFun) {
	_, ok := ChargeCheckFunc[id]
	if ok {
		logger.LogError("充值类型重复注册 id:%d", id)
		return
	}
	ChargeCheckFunc[id] = fn
}

func GetChargeCheckResult(id uint32, actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	if fnSet, ok := ChargeCheckFunc[id]; ok { //未注册默认通过
		return fnSet(actor, conf)
	}
	return true
}
