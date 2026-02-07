/**
 * @Author: ChenJunJi
 * @Desc: 接收网关连接
 * @Date: 2021/9/24 15:55
 */

package gateworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/network"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"math"
)

const MaxGateCount = 2

var (
	GAcceptor      = NewAcceptor()
	GateAgentArray [MaxGateCount]*GateAgent
)

type Acceptor struct {
	tcpServer *network.TCPServer
}

func NewAcceptor() *Acceptor {
	acceptor := &Acceptor{}
	tcpServer := &network.TCPServer{}
	tcpServer.MaxConnNum = 4096
	tcpServer.PendingWriteNum = 10240
	tcpServer.LenMsgLen = 4
	tcpServer.MaxMsgLen = math.MaxUint16 // unity的协议包头长度定义为2个字节
	tcpServer.LittleEndian = true

	tcpServer.NewAgent = func(conn *network.TCPConn) network.Agent {
		logger.LogInfo("accept gate connect：%s", conn.RemoteAddr().String())
		if agent := gGatePool.Get(); nil != agent {
			agent.conn = conn
			return agent
		}
		return nil
	}
	acceptor.tcpServer = tcpServer
	return acceptor
}

func (acceptor *Acceptor) Startup(host string) {
	acceptor.tcpServer.Addr = host
	acceptor.tcpServer.Start()
}

func (acceptor *Acceptor) Destroy() {
	acceptor.tcpServer.Close()
}

func onNewGateConnect(args ...interface{}) {
	if len(args) <= 0 {
		return
	}
	agent, ok := args[0].(*GateAgent)
	if !ok {
		return
	}
	GateAgentArray[agent.GateIdx] = agent
}

// 网关断线
func onGateDisConnect(args ...interface{}) {
	if len(args) <= 0 {
		return
	}

	agent, ok := args[0].(*GateAgent)
	if !ok {
		return
	}

	GateAgentArray[agent.GateIdx] = nil

	agent.CloseAllUser()
	agent.bReg = false
	agent.conn = nil
	gGatePool.Put(agent)
}

func GetGateConn(gateIdx uint32) *GateAgent {
	for _, line := range GateAgentArray {
		if nil != line && line.GateIdx == gateIdx {
			return line
		}
	}
	return nil
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgNewGateConnect, onNewGateConnect)
		gshare.RegisterGameMsgHandler(custom_id.GMsgGateDisconnect, onGateDisConnect)
	})
}
