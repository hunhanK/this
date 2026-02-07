/**
 * @Author: zjj
 * @Date: 2023/11/27
 * @Desc: 护送活动
**/

package dartcarmgr

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"

	"github.com/gzjjyz/logger"
)

var GDartCarMgrIns = newDartCarMgr()

func newDartCarMgr() *DartCarMgr {
	instance := &DartCarMgr{}
	instance.playerCarData = make(map[uint64]*pb3.DartCarData)
	return instance
}

type DartCarMgr struct {
	playerCarData map[uint64]*pb3.DartCarData
}

// 同步所有镖车数据
func (mgr *DartCarMgr) syncDartAllCarData(buf []byte) {
	msg := &pb3.SyncAllDartCar{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}
	// 区分是本服还是跨服传过来的数据
	srvType := msg.GetSrvType()
	now := time_util.NowSec()
	for actorId, data := range mgr.playerCarData {
		// 删除对应的数据
		if data.GetSrvType() == srvType || data.GetTime() < now {
			delete(mgr.playerCarData, actorId)
		}
	}

	fCarDateMap := msg.GetDataMap()
	for actorId, fdata := range fCarDateMap {
		mgr.addDartCar(actorId, fdata)
	}
}

// 新增镖车
func (mgr *DartCarMgr) addDartCar(actorId uint64, fData *pb3.InternalDartCarData) {
	// 这里有数据说明 跨服本服都有镖车 顶掉旧的镖车
	if playerData, ok := mgr.playerCarData[actorId]; ok {
		st := &pb3.CommonSt{}
		st.U64Param = actorId
		srvType := playerData.GetSrvType()
		err := engine.CallFightSrvFunc(base.ServerType(srvType), sysfuncid.G2FDelDartCar, st)
		if err != nil {
			logger.LogError("err:%v", err)
		}
		delete(mgr.playerCarData, actorId)
	}

	actor := manager.GetPlayerPtrById(actorId)
	if fData != nil {
		carData := &pb3.DartCarData{}
		carData.Handle = fData.GetCarHandle()
		carData.CarType = fData.GetCarType()
		carData.Time = fData.GetLiveTime()
		carData.IsDouble = fData.GetIsDouble()
		carData.SrvType = fData.GetSrvType()
		mgr.playerCarData[actorId] = carData

		if actor != nil {
			actor.SetExtraAttr(attrdef.DartCarType, attrdef.AttrValueAlias(fData.GetCarType()))
			mgr.SendDartCarData(actor)
		}
	} else {
		if actor != nil {
			actor.SetExtraAttr(attrdef.DartCarType, 0)
		}
	}
}

// 删除指定服的镖车
func (mgr *DartCarMgr) onDelCarBySrvType(srvType uint32) {
	for actorId, data := range mgr.playerCarData {
		if data.GetSrvType() != srvType {
			continue
		}
		delete(mgr.playerCarData, actorId)
		st := &pb3.CommonSt{}
		st.U64Param = actorId
		err := engine.CallFightSrvFunc(base.ServerType(srvType), sysfuncid.G2FDelDartCar, st)
		if err != nil {
			logger.LogError("err:%v", err)
		}
		actor := manager.GetPlayerPtrById(actorId)
		if actor != nil {
			actor.SetExtraAttr(attrdef.DartCarType, 0)
		}
	}
}

// 小跨服断开链接
func (mgr *DartCarMgr) onDartCarDisconnectCross(_ ...interface{}) {
	mgr.onDelCarBySrvType(uint32(base.SmallCrossServer))
}

// GetDartCarData 获取单个玩家镖车数据
func (mgr *DartCarMgr) GetDartCarData(actorId uint64) *pb3.DartCarData {
	if data, ok := mgr.playerCarData[actorId]; ok {
		return data
	}
	return nil
}

// SendDartCarData 发送镖车数据
func (mgr *DartCarMgr) SendDartCarData(actor iface.IPlayer) {
	if actor == nil {
		return
	}
	actorId := actor.GetId()
	data, ok := mgr.playerCarData[actorId]
	if !ok {
		return
	}

	actor.SendProto3(52, 2, &pb3.S2C_52_2{
		Data: data,
	})
}

// DelDartCar 删除镖车
func (mgr *DartCarMgr) DelDartCar(actorId uint64) {
	data := mgr.playerCarData[actorId]
	if data == nil {
		return
	}

	st := &pb3.CommonSt{}
	st.U64Param = actorId
	err := engine.CallFightSrvFunc(base.ServerType(data.GetSrvType()), sysfuncid.G2FDelDartCar, st)
	if err != nil {
		logger.LogError("err:%v", err)
	}

	actor := manager.GetPlayerPtrById(actorId)
	if actor != nil {
		actor.SetExtraAttr(attrdef.DartCarType, 0)
	}
	delete(mgr.playerCarData, actorId)
}

func (mgr *DartCarMgr) onSyncDartCarData(buf []byte) {
	msg := pb3.SyncDartCar{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	carData := msg.GetCarData()
	actorId := carData.GetActorId()
	if !gshare.IsActorInThisServer(actorId) {
		return
	}

	mgr.addDartCar(actorId, carData)
	actor := manager.GetPlayerPtrById(actorId)
	if actor == nil {
		return
	}
	mgr.SendDartCarData(actor)
}

func (mgr *DartCarMgr) onNoticeDartCarResult(buf []byte) {
	msg := pb3.NoticeDartCarResult{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	actorId := msg.GetActorId()
	if !gshare.IsActorInThisServer(actorId) {
		return
	}

	result := msg.GetResult()
	if result == custom_id.DCSucceed {
		result = mgr.onDartCarSucced(actorId, result)
	} else {
		mgr.onDartCarFail(actorId, result, msg.KillName)
	}
	actor := manager.GetPlayerPtrById(actorId)
	if actor != nil {
		actor.SendProto3(52, 4, &pb3.S2C_52_4{
			Result: result,
		})
		actor.SetExtraAttr(attrdef.DartCarType, 0)
		carData := mgr.GetDartCarData(actorId)
		if carData != nil {
			logworker.LogPlayerBehavior(actor, pb3.LogId_LoDartCarResult, &pb3.LogPlayerCounter{
				StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
					"result":  result,
					"carType": carData.CarType,
				}),
			})
		}
		actor.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionDartCar)
		actor.TriggerEvent(custom_id.AeFashionTalentEvent, &custom_id.FashionTalentEvent{
			Cond:  custom_id.FashionSetDartCarEvent,
			Count: 1,
		})
		actor.TriggerEvent(custom_id.AeFaBaoTalentEvent, &custom_id.FaBaoTalentEvent{
			Cond:  custom_id.FaBaoTalentCondDartCarTimes,
			Count: 1,
		})
		if result == custom_id.DCSucceed || result == custom_id.DCSucceedDouble {
			actor.TriggerQuestEvent(custom_id.QttDartCarSuccess, 0, 1)
			if carData != nil {
				actor.TriggerQuestEvent(custom_id.QttXDartCarTypeSuccess, carData.CarType, 1)
			}
		}
	}
	delete(mgr.playerCarData, actorId)
}

// 护送成功
func (mgr *DartCarMgr) onDartCarSucced(actorId uint64, result uint32) (ret uint32) {
	ret = result
	actor := manager.GetPlayerPtrById(actorId)
	if actor == nil {
		return
	}

	carData := mgr.GetDartCarData(actorId)
	if carData == nil {
		return
	}

	carType := carData.GetCarType()
	carInfoConfig, err := jsondata.GetDartCarInfoConfByCtAndSt(carType)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	conf, err := jsondata.GetDartCarAwardsConf(gshare.GetOpenServerDay(), carInfoConfig.AwardList)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	var awards = conf.Awards
	if carData.GetIsDouble() {
		awards = conf.ActAwards
		engine.BroadcastTipMsgById(tipmsgid.DartCarTips3, actor.GetName())
		ret = custom_id.DCSucceedDouble
	} else if custom_id.IsDartCarTypeDiamonds(carData.CarType) {
		engine.BroadcastTipMsgById(tipmsgid.DartCarTips2, actor.GetName())
	}

	engine.GiveRewards(actor, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDartCarGiveAwards})
	return
}

// 护送失败
func (mgr *DartCarMgr) onDartCarFail(actorId uint64, result uint32, killName string) {
	if !(result == custom_id.DCLoseDie || result == custom_id.DCLoseTimeOut) {
		return
	}

	carData := mgr.GetDartCarData(actorId)
	if carData == nil {
		return
	}

	carInfoConfig, err := jsondata.GetDartCarInfoConfByCtAndSt(carData.CarType)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	awardsConf, err := jsondata.GetDartCarAwardsConf(gshare.GetOpenServerDay(), carInfoConfig.AwardList)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	// 默认要给一个
	confId := common.Mail_DartCarFailMailId
	var awards = awardsConf.FailedAwards
	if carData.IsDouble {
		awards = awardsConf.ActFailedAwards
	}
	if result == custom_id.DCLoseDie {
		confId = common.Mail_DartCarFightMailId
		awards = awardsConf.LostAwards
		if carData.IsDouble {
			awards = awardsConf.ActLostAwards
		}
	}
	mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
		ConfId: uint16(confId),
		Content: &mailargs.CommonMailArgs{
			Str1: killName,
		},
		Rewards: awards,
	})
}

func init() {
	engine.RegisterSysCall(sysfuncid.F2GSyncAllDartCarData, GDartCarMgrIns.syncDartAllCarData)
	engine.RegisterSysCall(sysfuncid.F2GDartCarData, GDartCarMgrIns.onSyncDartCarData)
	engine.RegisterSysCall(sysfuncid.FGNoticeDartCarResult, GDartCarMgrIns.onNoticeDartCarResult)

	event.RegSysEvent(custom_id.SeCrossDisconnect, GDartCarMgrIns.onDartCarDisconnectCross)
}
