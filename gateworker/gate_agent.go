package gateworker

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/network"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
)

type (
	GateAgent struct {
		conn    *network.TCPConn
		bReg    bool
		GateIdx uint32

		UserList [base.MaxGateUserCount]*UserSt
	}
)

func (agent *GateAgent) Close() {
	agent.conn.Close()
}

func (agent *GateAgent) Run() {
	engine.NotifyGateConn()
	for {
		data, err := agent.conn.ReadMsg()
		if nil != err {
			logger.LogError("gate agent read message error! %v", err)
			break
		}

		if !agent.bReg {
			if !agent.onReg(data) {
				logger.LogError("gate agent register failed!")
				break
			}
			continue
		}
		if !bStop.Load() {
			gshare.SendGameMsg(custom_id.GMsgRecvGateMsg, agent.GateIdx, data)
		}
	}
}

func (agent *GateAgent) OnClose() {
	gshare.SendGameMsg(custom_id.GMsgGateDisconnect, agent)
	logger.LogInfo("gate agent disconnect id=%d", agent.GateIdx)
	if !agent.bReg {
		logger.LogInfo("==========no register gate agent disconnect!!=============")
	}
}

func (agent *GateAgent) onReg(data []byte) bool {
	logger.LogInfo("buff:%v", data)
	var gate pb3.RegGateWay
	err := pb3.Unmarshal(data, &gate)
	if nil != err {
		logger.LogError("gate agent register unmarshal error! %v", err)
		return false
	}
	if gate.GameId != base.GID || base.ServerType(gate.ServerType) != base.GateServer {
		return false
	}
	agent.bReg = true

	gshare.SendGameMsg(custom_id.GMsgNewGateConnect, agent)

	return true
}

func (agent *GateAgent) SendMessage(cmdId uint8, buff []byte) (err error) {
	err = agent.conn.WriteMsg([]byte{cmdId}, buff)
	if nil != err {
		logger.LogError("gate agent send message error. %v", err)
		return err
	}
	return err
}

func (agent *GateAgent) GetUser(connId uint32) *UserSt {
	if connId >= base.MaxGateUserCount {
		return nil
	}
	user := agent.UserList[connId]
	if user.Closed {
		return nil
	}
	return user
}

func (agent *GateAgent) SendGateCloseUser(connId uint32) {
	clientData, err := pb3.Marshal(&pb3.ClientData{
		ConnId: connId,
	})
	if nil != err {
		logger.LogError("conn:%d err:%v", connId, err)
		return
	}
	err = agent.SendMessage(base.GW_CLOSE, clientData)
	if nil != err {
		logger.LogError("conn:%d err:%v", connId, err)
		return
	}
}

func (agent *GateAgent) OpenNewUser(connId uint32, ipAddr string) {
	if connId >= base.MaxGateUserCount {
		return
	}

	user := agent.UserList[connId]
	user.AccountName = ""
	user.UserId = 0
	user.RemoteAddr = ipAddr
	user.Closed = false
	user.GmLevel = 0
	logger.LogDebug("OpenNewUser:%v", user)
}

func (agent *GateAgent) CloseUser(connId uint32) {
	if connId >= base.MaxGateUserCount {
		return
	}
	user := agent.UserList[connId]
	if user.Closed {
		return
	}
	if user.UserId > 0 {
		DelGateUserByUserId(user.UserId)
	}
	if user.ActorId != 0 {
		engine.CloseGateUser(user, cmd.DCRLost)
	} else {
		user.Reset()
	}
	logger.LogDebug("CloseUser:%v", user)
}

func (agent *GateAgent) CloseAllUser() {
	for _, user := range agent.UserList {
		agent.CloseUser(user.ConnId)
	}
}

func (agent *GateAgent) OnUserMsg(connId uint32, data []byte) {
	if connId >= base.MaxGateUserCount {
		return
	}
	user := agent.UserList[connId]
	if user.Closed {
		return
	}
	user.OnRecv(data)
}
