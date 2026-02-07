package fightworker

import (
	"encoding/binary"
	"jjyz/base"
	"jjyz/base/cmd"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"math"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/network"
)

type (
	FightClient struct {
		tcpClient *network.TCPClient
	}

	FightAgentBase struct {
		conn      *network.TCPConn
		agentName string
		camp      crosscamp.CampType
	}
)

type newAgentfunc func(conn *network.TCPConn) network.Agent

func NewFightAgentBase(conn *network.TCPConn, agentName string, camp crosscamp.CampType) *FightAgentBase {
	agent := new(FightAgentBase)
	agent.conn = conn
	agent.agentName = agentName
	agent.camp = camp
	agent.OnInit()
	return agent
}

func NewFightClient(host string, fn newAgentfunc) *FightClient {
	c := new(network.TCPClient)
	c.Addr = host
	c.ConnNum = 1
	c.ConnectInterval = 3 * time.Second
	c.PendingWriteNum = 4096
	c.LenMsgLen = 4
	c.MaxMsgLen = math.MaxUint32
	c.LittleEndian = true
	c.AutoReconnect = true
	c.NewAgent = fn
	return &FightClient{tcpClient: c}
}

func (c *FightClient) Startup() {
	if nil == c.tcpClient {
		logger.LogError("fight tcp client is nil!!")
		return
	}
	c.tcpClient.Start()
}

func (c *FightClient) Destroy() {
	if nil != c.tcpClient {
		c.tcpClient.Close()
	}
}

func (a *FightAgentBase) OnInit() {
	logger.LogInfo("[%s] OnInit", a.agentName)
	a.reg()
}

func (a *FightAgentBase) Run() {
	if a.agentName == localFightName {
		engine.NotifyLocalFightConn()
	}
	for {
		data, err := a.conn.ReadMsgWithTrace()
		if err != nil {
			logger.LogError("[%s] read message: %v", a.agentName, err)
			break
		}
		if len(data) < 2 {
			logger.LogError("[%s] read message less then 2 length", a.agentName)
			continue
		}
		cmdId := binary.LittleEndian.Uint16(data[:2])

		data = data[2:]
		switch cmdId {
		case cmd.GFToFightClientMsg:
			gshare.SendGameMsg(custom_id.GMsgFightSrvClientMsg, data)
		case cmd.GFCallPlayerFunc:
			gshare.SendGameMsg(custom_id.GMsgActorCallPlayerFunc, data)
		case cmd.FGCallLogicSrvFunc:
			gshare.SendGameMsg(custom_id.GMsgCallLogicSrvFun, data)
		}
	}
}

func (a *FightAgentBase) OnClose() {
	logger.LogInfo("[%s] is close!!", a.agentName)
}

func (a *FightAgentBase) reg() {
	logger.LogInfo("[%s] send reg message start!", a.agentName)
	//注册服务器N
	req := &pb3.RegGameSrv{}
	req.GameId = base.GID
	req.ServerType = uint32(base.GameServer)
	req.PfId = gshare.GameConf.PfId
	req.ServerId = gshare.GameConf.SrvId
	req.OpenTime = gshare.GetOpenServerTime()
	req.MergeTimestamp = gshare.GetStaticVar().GetMergeTimestamp()
	req.MergeTimes = gshare.GetMergeTimes()
	req.Camp = uint32(a.camp)
	req.NMergeTimestamp = gshare.GetStaticVar().GetMergeData()

	if data, err := pb3.Marshal(req); nil == err {
		buf := [2]byte{}
		binary.LittleEndian.PutUint16(buf[:2], cmd.G2FRegGameSrv)
		a.conn.WriteMsgWithTrace(buf[:], data)
		logger.LogInfo("[%s] send reg message finish!", a.agentName)
	} else {
		logger.LogError("[%s] send reg message error! %v", a.agentName, err)
	}
}
