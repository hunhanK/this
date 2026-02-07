/**
 * @Author: zjj
 * @Date: 2024/7/16
 * @Desc: 聚灵池
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/privilegedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

const (
	BuyQiLingPrivilegeByMoney  = 1
	BuyQiLingPrivilegeByCharge = 2
)

type QiLingSys struct {
	Base
	timerOut *time_util.Timer
}

func (q *QiLingSys) getData() *pb3.QiLingData {
	data := q.GetOwner().GetBinaryData()
	qiLingData := data.QiLingData
	if qiLingData == nil {
		data.QiLingData = &pb3.QiLingData{}
		qiLingData = data.QiLingData
	}
	return qiLingData
}

func (q *QiLingSys) s2cInfo() {
	q.SendProto3(163, 20, &pb3.S2C_163_20{
		Data: q.getData(),
	})
}

func (q *QiLingSys) setTimeAfter() {
	q.stopTimeAfter()
	owner := q.GetOwner()
	generateInfo := q.getGenerateInfo()
	if generateInfo == nil {
		return
	}

	q.timerOut = owner.SetTimeout(time.Duration(generateInfo.Interval)*time.Second, func() {
		q.addQiLing(generateInfo.SecondAddQiLing*generateInfo.Interval/10000, pb3.LogId_LogQiLingOnLineGiveQiLing)
		q.setTimeAfter()
	})
}
func (q *QiLingSys) stopTimeAfter() {
	if q.timerOut != nil {
		q.timerOut.Stop()
		q.timerOut = nil
	}
}

func (q *QiLingSys) getGenerateInfo() *jsondata.QiLingGenerateConf {
	conf := jsondata.GetQiLingConf()
	if conf == nil {
		return nil
	}
	data := q.getData()
	if data.BuyPrivilege {
		return conf.PrivilegeGenerate
	}
	return conf.NormalGenerate
}

func (q *QiLingSys) checkFull() bool {
	owner := q.GetOwner()
	moneyCount := owner.GetMoneyCount(moneydef.QiLing)
	info := q.getGenerateInfo()
	if info == nil {
		return false
	}

	// 没达到上限
	if info.UpLimit > moneyCount {
		return false
	}

	// 还有存储空间
	if info.StoreUpLimit != 0 && info.StoreUpLimit > q.getData().Store {
		return false
	}

	return true
}

func (q *QiLingSys) reCalcOfflineAwards() {
	if q.checkFull() {
		return
	}

	conf := jsondata.GetQiLingConf()
	if conf == nil {
		return
	}

	generateInfo := q.getGenerateInfo()
	if generateInfo == nil {
		return
	}
	interval := generateInfo.Interval
	if interval == 0 {
		return
	}

	logoutTime := q.getData().LogoutTime
	if logoutTime == 0 {
		return
	}

	owner := q.GetOwner()
	loginTime := owner.GetMainData().GetLoginTime()
	if logoutTime > loginTime {
		return
	}

	diff := loginTime - logoutTime
	if diff == 0 {
		return
	}

	multiple := int64(diff) / interval
	addQiLing := generateInfo.SecondAddQiLing * interval * multiple / 10000
	q.addQiLing(addQiLing, pb3.LogId_LogQiLingOffLineGiveQiLing)
}

func (q *QiLingSys) addQiLing(addQiLing int64, logId pb3.LogId) {
	if q.checkFull() {
		return
	}

	if addQiLing == 0 {
		return
	}

	owner := q.GetOwner()
	data := q.getData()
	generateInfo := q.getGenerateInfo()
	if generateInfo == nil {
		return
	}

	moneyCount := owner.GetMoneyCount(moneydef.QiLing)
	var overCount int64
	if addQiLing+moneyCount > generateInfo.UpLimit {
		overCount = (addQiLing + moneyCount) - generateInfo.UpLimit
		addQiLing = addQiLing - overCount
	}

	if generateInfo.StoreUpLimit != 0 {
		if generateInfo.StoreUpLimit < data.Store+overCount {
			data.Store = generateInfo.StoreUpLimit
		} else {
			data.Store += overCount
		}
		q.s2cInfo()
	}

	owner.AddMoney(moneydef.QiLing, addQiLing, false, logId)
}

func (q *QiLingSys) OnLogin() {
	q.reCalcOfflineAwards()
	q.setTimeAfter()
	q.s2cInfo()
}

func (q *QiLingSys) OnOpen() {
	q.getData().LogoutTime = time_util.NowSec()
	q.setTimeAfter()
	q.s2cInfo()
}

func (q *QiLingSys) OnLogout() {
	q.getData().LogoutTime = time_util.NowSec()
	q.stopTimeAfter()
}

func (q *QiLingSys) OnReconnect() {
	q.setTimeAfter()
	q.s2cInfo()
}

func (q *QiLingSys) buyPrivilege(_ *base.Message) error {
	conf := jsondata.GetQiLingConf()
	if conf == nil {
		return neterror.ConfNotFoundError("not found lq ling conf")
	}
	if conf.BuyType != BuyQiLingPrivilegeByMoney {
		return neterror.ParamsInvalidError("not supported buy by money")
	}
	owner := q.GetOwner()
	if len(conf.Consume) != 0 && !owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogQiLingByPrivilegeByMoney}) {
		return neterror.ConsumeFailedError("buy by money consume failed")
	}
	if !qiLingChargeHandler(owner, pb3.LogId_LogQiLingByPrivilegeByMoney) {
		return neterror.InternalError("buy by money failed")
	}
	return nil
}

func checkQiLingChargeHandler(actor iface.IPlayer, chargeConf *jsondata.ChargeConf) bool {
	conf := jsondata.GetQiLingConf()
	if conf == nil {
		return false
	}
	if conf.BuyType != BuyQiLingPrivilegeByCharge {
		return false
	}
	if conf.ChargeId != chargeConf.ChargeId {
		return false
	}
	total, err := actor.GetPrivilege(privilegedef.EnumQiLing)
	if err != nil {
		actor.LogError("err:%v", err)
		return false
	}
	if total != 0 {
		return false
	}
	return true
}

func qiLingChargeHandler(actor iface.IPlayer, logId pb3.LogId) bool {
	qiLingSys, ok := actor.GetSysObj(sysdef.SiQiLing).(*QiLingSys)
	if !ok || nil == qiLingSys {
		return false
	}

	data := qiLingSys.getData()
	if data.BuyPrivilege {
		return false
	}

	data.BuyPrivilege = true
	logworker.LogPlayerBehavior(actor, logId, &pb3.LogPlayerCounter{})
	actor.AddMoney(moneydef.QiLing, data.Store, false, logId) // 充值直接发放给玩家
	data.Store = 0
	conf := jsondata.GetQiLingConf()
	if conf != nil && conf.GiveLingQi != 0 {
		actor.AddMoney(moneydef.QiLing, conf.GiveLingQi, true, logId) // 发放给玩家奖励器灵
	}
	actor.SendProto3(163, 21, &pb3.S2C_163_21{})
	qiLingSys.s2cInfo()
	qiLingSys.ResetSysAttr(attrdef.AtQiLingPrivilege)
	return true
}

func handleAddDailyMissionPointByQiLing(player iface.IPlayer, args ...interface{}) {
	if len(args) == 0 {
		return
	}
	addPoint := args[0].(uint32)
	qiLingSys, ok := player.GetSysObj(sysdef.SiQiLing).(*QiLingSys)
	if !ok || nil == qiLingSys {
		return
	}
	info := qiLingSys.getGenerateInfo()
	if info.DailyMissionScoreAddQiLing == 0 {
		return
	}
	addQiLing := int64(addPoint) * info.DailyMissionScoreAddQiLing
	qiLingSys.addQiLing(addQiLing, pb3.LogId_LogQiLingDailyMissionAwardsQiLing)
}
func init() {
	RegisterSysClass(sysdef.SiQiLing, func() iface.ISystem {
		return &QiLingSys{}
	})
	net.RegisterSysProtoV2(163, 21, sysdef.SiQiLing, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*QiLingSys).buyPrivilege
	})
	engine.RegChargeEvent(chargedef.QiLingCharge, checkQiLingChargeHandler, func(actor iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
		return qiLingChargeHandler(actor, pb3.LogId_LogQiLingByPrivilegeByCharge)
	})
	RegisterPrivilegeCalculater(func(player iface.IPlayer, conf *jsondata.PrivilegeConf) (total int64, err error) {
		sys, ok := player.GetSysObj(sysdef.SiQiLing).(*QiLingSys)
		if !ok || !sys.IsOpen() {
			return
		}

		if sys.getData().BuyPrivilege {
			total = int64(conf.QiLing)
		}
		return
	})
	event.RegActorEvent(custom_id.AeAddDailyMissionPoint, handleAddDailyMissionPointByQiLing)
	engine.RegAttrCalcFn(attrdef.AtQiLingPrivilege, calcQiLingSysAttr)
}

func calcQiLingSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	qiLingSys, ok := player.GetSysObj(sysdef.SiQiLing).(*QiLingSys)
	if !ok || nil == qiLingSys {
		return
	}
	if !qiLingSys.getData().BuyPrivilege {
		return
	}
	conf := jsondata.GetQiLingConf()
	if conf == nil {
		return
	}
	engine.CheckAddAttrsToCalc(player, calc, conf.PrivilegeAttrs)
}
