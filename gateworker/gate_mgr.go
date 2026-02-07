/**
 * @Author: ChenJunJi
 * @Desc: 网关管理器
 * @Date: 2021/8/28 10:15
 */

package gateworker

import (
	"jjyz/base"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logworker"
	"sync/atomic"

	"github.com/gzjjyz/logger"
)

var (
	bStop atomic.Bool
)

func Startup() {
	conf := gshare.GameConf
	GAcceptor.Startup(conf.GateHost)
}

func OnSrvStop() {
	GAcceptor.Destroy()
}

func OnSrvBeforeStop() {
	bStop.Store(true)
}

func SendGateCloseUser(gateIdx, connId uint32) {
	if conn := GetGateConn(gateIdx); nil != conn {
		conn.SendGateCloseUser(connId)
	}
}

func broadcast(channelId uint32, param int64, sysId, cmdId uint16, msg pb3.Message, level uint32) {
	buf, err := pb3.Marshal(msg)
	if nil != err {
		logger.LogError("broadcast error!! pb3:%v", err)
		return
	}
	broadcastBuf(channelId, param, sysId, cmdId, buf, level)
}

func broadcastBuf(channelId uint32, param int64, sysId, cmdId uint16, buf []byte, level uint32) {
	st := &pb3.G2GwBroadcast{
		Cmd:     base.CCBroadCast,
		Channel: channelId,
		Param64: param,
		Param32: int32(level),
		ProtoId: int32(sysId<<8 | cmdId),
		Buf:     buf,
	}
	logger.LogTrace("broadcast channel:%d msg:%+v", channelId, st)
	buf1, err := pb3.Marshal(st)
	if nil != err {
		logger.LogError("%d_%d broadcast error!! pb3:%v", sysId, cmdId, err)
		return
	}

	for _, gate := range GateAgentArray {
		if nil != gate {
			err := gate.SendMessage(base.GW_CHANNEL, buf1)
			if nil != err {
				logger.LogError("%d_%d broadcast error!! err:%v", sysId, cmdId, err)
			}
			logworker.LogNetProtoStat(uint32(sysId<<8|cmdId), uint32(len(buf1)), engine.GetPfId(), engine.GetServerId())
		}
	}
}

// 玩家等级变化 通知网关
func playerLevelChange(connId int64, level uint32) {
	st := &pb3.G2GwBroadcast{
		Cmd:     base.CCLevel,
		Param64: connId,
		Param32: int32(level),
	}
	buf, err := pb3.Marshal(st)
	if nil != err {
		logger.LogError("connId:%d,level:%d broadcast error!! pb3:%v", connId, level, err)
		return
	}

	for _, gate := range GateAgentArray {
		if nil != gate {
			err := gate.SendMessage(base.GW_CHANNEL, buf)
			if err != nil {
				logger.LogError("connId:%d,level:%d broadcast error!! error:%v", connId, level, err)
			}
		}
	}
}

func init() {
	engine.Broadcast = broadcast
	engine.BroadcastBuf = broadcastBuf

	engine.PlayerLevelChange = playerLevelChange
}
