package dbworker

import (
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/model"
	"jjyz/gameserver/mq"
	"time"

	"github.com/gzjjyz/logger"
)

var (
	forceLoadChargeTime time.Time
)

// 定时检测充值表
func loadCharge(force bool) error {
	now := time.Now()
	if !force && now.Before(forceLoadChargeTime) {
		return nil
	}
	forceLoadChargeTime = now.Add(5 * time.Minute)

	var orders []*model.Charge
	err := db.OrmEngine.Table(&model.Charge{}).Where("check_time=0").Find(&orders)
	if nil != err {
		logger.LogError("加载充值订单列表出错:%s", err)
		return err
	}

	m := model.Charge{CheckTime: int32(time.Now().Unix())}
	for idx := range orders {
		order := orders[idx]
		if _, err = db.OrmEngine.ID(order.Id).Update(m); nil == err {
			gshare.SendGameMsg(custom_id.GMsgNewChargeOrder, order)
			logger.LogInfo("load charge order. %d %d", order.ActorId, order.ChargeId)
		} else {
			logger.LogError("update order check time error! %v", err)
		}
	}

	return nil
}

func clearChargeOrders(param ...interface{}) {
	var orders []*model.Charge
	err := db.OrmEngine.Table(&model.Charge{}).Where("check_time !=0").Find(&orders)
	if nil != err {
		logger.LogError("clear charge order load error:%v", err)
		return
	}

	var ids []uint64
	for _, order := range orders {
		ids = append(ids, order.Id)
	}

	if _, err := db.OrmEngine.In("id", ids).Delete(&model.Charge{}); nil != err {
		logger.LogError("clear charge orders error %v", err)
	} else {
		for _, order := range orders {
			logger.LogInfo("clear order:%v", order)
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgClearChargeOrder, clearChargeOrders)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadChargeData, func(param ...interface{}) {
			if err := loadCharge(true); nil != err {
				logger.LogError("load charge error %v", err)
			}
		})
	})
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		mq.RegisterMQHandler(pb3.GameServerNatsOpCode_NotifyCharge, func(data []byte) error {
			gshare.SendDBMsg(custom_id.GMsgLoadChargeData)
			return nil
		})

		gshare.SendDBMsg(custom_id.GMsgLoadChargeData)
	})

}
