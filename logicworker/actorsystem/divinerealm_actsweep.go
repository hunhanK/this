/**
 * @Author: LvYuMeng
 * @Date: 2025/5/16
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/actsweepmgr"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/manager"
)

var singleDivineRealmController = &DivineRealmController{}

type DivineRealmController struct {
	actsweepmgr.Base
}

var divineRealmEndTimes uint32

func (receiver *DivineRealmController) GetCanUseTimes(id uint32, playerId uint64) (useTimes uint32) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiDivineRealmConquer)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*DivineRealmSys)
	if !ok || !sys.IsOpen() {
		return
	}

	return divineRealmEndTimes
}

func (receiver *DivineRealmController) GetUseTimes(id uint32, playerId uint64) (useTimes uint32, ret bool) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	obj := player.GetSysObj(sysdef.SiDivineRealmConquer)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*DivineRealmSys)
	if !ok {
		return
	}

	partiTimes := sys.getDailyPartiTimes()

	canSweepTimes := divineRealmEndTimes
	partiTimes = utils.MinUInt32(canSweepTimes, partiTimes)
	return partiTimes, true
}

func (receiver *DivineRealmController) AddUseTimes(_ uint32, times uint32, playerId uint64) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiDivineRealmConquer)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*DivineRealmSys)
	if !ok {
		return
	}
	var addTimes uint32
	data := sys.getData()
	for i := uint32(1); i <= divineRealmEndTimes; i++ {
		if utils.IsSetBit(data.PartiBits, i) {
			continue
		}
		if addTimes >= times {
			break
		}
		data.PartiBits = utils.SetBit(data.PartiBits, i)
		player.TriggerQuestEvent(custom_id.QttDivineRealmParticipate, 0, 1)
		addTimes++
	}
}

func handleC2GDivineRealmEndTimesSync(buf []byte) {
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("C2GDivineRealmEndTimesSync err:%v", err)
		return
	}
	divineRealmEndTimes = req.U32Param
	conf := jsondata.GetActSweepConfByDivineRealm()
	if conf == nil {
		return
	}
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByDivineRealm)
}

func init() {
	actsweepmgr.Reg(actsweepmgr.ActSweepByDivineRealm, singleDivineRealmController)

	engine.RegisterSysCall(sysfuncid.C2GDivineRealmEndTimesSync, handleC2GDivineRealmEndTimesSync)

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		divineRealmEndTimes = 0
	})

}
