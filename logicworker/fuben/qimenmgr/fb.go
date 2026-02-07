package qimenmgr

import (
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/monster_event"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"strings"
	"time"

	"github.com/gzjjyz/logger"
)

var bossTimer = make(map[uint32]*time_util.Timer)

func GetQiMenInfo() *pb3.QiMenFbInfo {
	sst := gshare.GetStaticVar()
	if nil == sst.QiMenInfo {
		sst.QiMenInfo = &pb3.QiMenFbInfo{}
	}
	if nil == sst.QiMenInfo.QiMenLoop {
		sst.QiMenInfo.QiMenLoop = make(map[uint32]uint32)
	}
	return sst.QiMenInfo
}

func GetOpenType(serverDay uint32) (loop, fbType, startTime, endTime uint32) {
	confList := jsondata.GetQiMenConf()
	if nil == confList {
		return
	}
	for _, conf := range confList {
		for idx, openDay := range conf.OpenDay {
			endDay := conf.LoopTime[idx] + openDay - 1
			if openDay <= serverDay && serverDay <= endDay {
				return uint32(idx + 1), conf.Id, openDay, endDay
			}
		}
	}

	//超过范围轮换
	var lastDay, roundTime uint32
	for _, conf := range confList {
		openDay := conf.OpenDay[len(conf.OpenDay)-1]
		endDay := conf.LoopTime[len(conf.OpenDay)-1] + openDay - 1
		lastDay = utils.MaxUInt32(lastDay, endDay)
		roundTime += conf.LoopTime[len(conf.LoopTime)-1]
	}

	if serverDay < lastDay {
		return
	}
	outDay := serverDay - lastDay

	curOutLoop := outDay / roundTime
	if outDay%roundTime > 0 {
		curOutLoop++
	}

	beforeRoundLoop := curOutLoop - 1
	var accDay uint32
	baseDay := lastDay + beforeRoundLoop*roundTime // 这里要算的前一轮的
	for _, conf := range confList {
		lastTime := conf.LoopTime[len(conf.LoopTime)-1]
		openDay, endDay := baseDay+accDay+1, baseDay+accDay+lastTime
		if openDay <= serverDay && serverDay <= endDay {
			curOutLoop = curOutLoop + uint32(len(conf.OpenDay))
			return curOutLoop, conf.Id, baseDay + accDay, baseDay + accDay + lastTime
		}
		accDay += lastTime
	}
	return
}

// 设置奇门副本类型
func onChangeFbType() bool {
	info := GetQiMenInfo()
	nxtLoop, nxtType, _, endDay := GetOpenType(gshare.GetOpenServerDay())
	if nxtType == info.QiMenType && nxtLoop == info.QiMenLoop[info.QiMenType] {
		return false
	}

	info.Lock = false
	info.QiMenType = nxtType
	endTime := time_util.GetDaysZeroTime(endDay - gshare.GetOpenServerDay() + 1)
	info.QiMenEndTimeStamp = endTime
	info.QiMenLoop[nxtType] = nxtLoop
	return true
}

func isQiMenOpen() bool {
	openTime := jsondata.GlobalUint("BFQM_starttime")
	openDay := gshare.GetOpenServerDay()
	if openDay < openTime {
		return false
	}
	return true
}

// 清空boss
func clearAllBoss() {
	msg := &pb3.QiMenClearAllBoss{}
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFQiMenClearAllBoss, msg)
	for _, v := range bossTimer {
		v.Stop()
	}
	bossTimer = make(map[uint32]*time_util.Timer)
}

func getBossList() map[uint32]uint32 {
	data := GetQiMenInfo()
	if nil == data.QiMenBossList {
		data.QiMenBossList = make(map[uint32]uint32)
	}
	return data.QiMenBossList
}

func getCurTypeLoop(layer uint32) uint32 {
	qmInfo := GetQiMenInfo()
	loop := qmInfo.QiMenLoop[qmInfo.QiMenType]
	typeConf := GetCurTypeConf()
	if nil == typeConf || nil == typeConf.LayerConf[layer] {
		return 0
	}
	length := uint32(len(typeConf.LayerConf[layer].Round))
	if loop >= length {
		loop = length
	}
	return loop
}

// 发送消息去战斗服创建boss
func createBoss(st *pb3.CommonMonCreate) {
	fbId := GetFbId()
	engine.OnFightSrvCreateMonster(base.LocalFightServer, fbId, st.SceneId, st.MonId, st.X, st.Y)
	bossList := getBossList()
	bossList[st.MonId] = 0
	if t := bossTimer[st.MonId]; nil != t {
		t.Stop()
		delete(bossTimer, st.MonId)
	}
}

func tryReliveAllBoss(clear bool) {
	engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFQiMenChangeType, &pb3.CommonSt{U32Param: GetQiMenInfo().QiMenType})
	clearAllBoss()
	bossList := getBossList()
	typeConf := GetCurTypeConf()
	if nil == typeConf {
		return
	}
	if clear {
		GetQiMenInfo().QiMenBossList = make(map[uint32]uint32)
	}
	now := time_util.NowSec()
	for layer, layerConf := range typeConf.LayerConf {
		loop := getCurTypeLoop(layer)
		if loop > 0 {
			for bossId, line := range layerConf.Round[loop-1].Boss {
				sceneId := layerConf.SceneId
				st := &pb3.CommonMonCreate{MonId: bossId, SceneId: sceneId, X: line.X, Y: line.Y}
				if clear {
					createBoss(st)
				} else {
					reliveTime, exists := bossList[bossId]
					if exists && now <= reliveTime {
						createTombstone(&pb3.CreateQiMenTombstoneReq{
							RelieveTime: reliveTime,
							BornX:       line.X,
							BornY:       line.Y,
							BossId:      bossId,
							SceneId:     sceneId,
						})
						bossTimer[bossId] = timer.SetTimeout(time.Duration(reliveTime-now)*time.Second, func() {
							createBoss(st)
						})
					} else {
						createBoss(st)
					}
				}
			}
		}
	}
}

func trySetInitType() bool {
	if onChangeFbType() {
		tryReliveAllBoss(true)
		broadcastTypeChange()
		return true
	}
	return false
}

func broadcastTypeChange() {
	fbInfo := GetQiMenInfo()
	rsp := &pb3.S2C_17_51{}
	if !fbInfo.Lock {
		rsp = &pb3.S2C_17_51{
			FbType:       fbInfo.QiMenType,
			Loop:         fbInfo.QiMenLoop[fbInfo.QiMenType],
			EndTimeStamp: fbInfo.QiMenEndTimeStamp,
		}
	}
	var sendLevel uint32
	if sysConf := jsondata.GetSysOpenConf(sysdef.SiQiMen); nil != sysConf {
		sendLevel = sysConf.Level
	}
	engine.Broadcast(chatdef.CIWorld, 0, 17, 51, rsp, sendLevel)
}

func newTypeCheck() bool {
	if onChangeFbType() {
		tryReliveAllBoss(true)
		broadcastTypeChange()
		return true
	}
	return false
}

// 服务器切天
func onNewDay(args ...interface{}) {
	if !isQiMenOpen() {
		return
	}

	if trySetInitType() {
		return
	}

	newTypeCheck()
}

// 拿到当天的类型配置
func GetCurTypeConf() *jsondata.QiMenConf {
	info := GetQiMenInfo()
	if info.Lock {
		return nil
	}
	return jsondata.GetQiMenConfByType(GetQiMenInfo().QiMenType)
}

func GetFbId() uint32 {
	return jsondata.GlobalUint("qimen_fbid")
}

func createTombstone(req *pb3.CreateQiMenTombstoneReq) {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateQiMenTombStones, req)
	if err != nil {
		logger.LogError("CreateQiMenTombstone err:%v", err)
		return
	}
}

func onMonDie(layer, sceneId, monId uint32, killerId uint64, args []uint32) {
	typeConf := GetCurTypeConf()
	if nil == typeConf {
		return
	}
	layerConf := typeConf.GetLayerConf(sceneId)
	loop := getCurTypeLoop(layerConf.Layer)
	if loop > 0 {
		if line, ok := layerConf.Round[loop-1].Boss[monId]; ok {
			bossList := getBossList()
			interval := jsondata.GetSpBossSceneRefreshTime(sceneId, line.Interval, gshare.GetOpenServerDay()) //刷新间隔
			now := time_util.NowSec()
			reliveTimeStamp := now + interval

			bossList[monId] = reliveTimeStamp
			err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.GFCallQiMenBossDie, &pb3.QiMenBossBroadcast{
				SceneId: sceneId,
				Status: &pb3.BossStatus{
					Layer:           layerConf.Layer,
					BossId:          monId,
					ReliveTimeStamp: reliveTimeStamp,
					BronX:           line.X,
					BronY:           line.Y,
				},
			})
			if err != nil {
				logger.LogError("err:%v", err)
			}
			createTombstone(&pb3.CreateQiMenTombstoneReq{
				RelieveTime: reliveTimeStamp,
				BornX:       line.X,
				BornY:       line.Y,
				BossId:      monId,
				SceneId:     sceneId,
			})
			st := &pb3.CommonMonCreate{MonId: monId, SceneId: sceneId, X: line.X, Y: line.Y}
			bossTimer[monId] = timer.SetTimeout(time.Duration(reliveTimeStamp-now)*time.Second, func() {
				createBoss(st)
			})
		}
	}
}

// 下发boss列表
func SendQiMenAllBossList(actor iface.IPlayer, nedlayer uint32) {
	msg := &pb3.S2C_17_53{}

	bossList := getBossList()
	conf := GetCurTypeConf()
	if nil == conf {
		logger.LogDebug("【八方奇门】配置为空")
		return
	}
	for layer, layerConf := range conf.LayerConf {
		if nedlayer > 0 && nedlayer != layer {
			continue
		}
		loop := getCurTypeLoop(layer)
		if loop > 0 {
			for bossId := range layerConf.Round[loop-1].Boss {
				tmp := &pb3.BossStatus{
					Layer:           layerConf.Layer,
					BossId:          bossId,
					ReliveTimeStamp: bossList[bossId],
				}
				msg.BossList = append(msg.BossList, tmp)
			}
		}
	}

	actor.SendProto3(17, 53, msg)
}

func afterConnectFightSrv(args ...interface{}) {
	if !isQiMenOpen() {
		return
	}
	isInit := trySetInitType()
	if isInit {
		return
	}
	if !newTypeCheck() {
		tryReliveAllBoss(false)
	}
}

func resetQiMen(player iface.IPlayer, args ...string) bool {
	st := gshare.GetStaticVar()
	st.QiMenInfo = nil
	trySetInitType()
	return true
}

func init() {
	event.RegSysEvent(custom_id.SeReloadJson, func(args ...interface{}) {
		fbId := GetFbId()
		if fbId != 0 {
			monster_event.RegisterMonDieEvent(fbId, onMonDie)
		}
	})

	gmevent.Register("resetQimen", resetQiMen, 1)
	gmevent.Register("logQimenType", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}

		days := utils.AtoInt(args[0])
		var strBuilder strings.Builder

		for serverDay := 1; serverDay <= days; serverDay++ {
			curLoop, fbType, _, _ := GetOpenType(uint32(serverDay))
			strBuilder.WriteString(fmt.Sprintf("开服天数 %d 开放类型 %d 轮数 %d\n", serverDay, fbType, curLoop))
		}
		logger.LogDebug("%s", strBuilder.String())
		return true
	}, 1)

	// 服务器切天
	event.RegSysEvent(custom_id.SeNewDayArrive, onNewDay)

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, afterConnectFightSrv)

	engine.RegisterSysCall(sysfuncid.F2GRelieveQiMenBoss, func(buf []byte) {
		var req pb3.CommonMonCreate
		if err := pb3.Unmarshal(buf, &req); err != nil {
			logger.LogError("F2GRelieveQiMenBoss Unmarshal error:%v", err)
		}
		createBoss(&req)
	})
}
