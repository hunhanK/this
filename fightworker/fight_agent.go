package fightworker

import (
	"encoding/binary"
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id/crosscamp"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/network"
)

type CrossFightCli struct {
	*FightClient
	AllowEnterAt uint32
	CrossSrvNum  uint32
}

// TODO: 名称待定
type FightSt struct {
	client *FightClient
	agent  *FightAgentBase
}

func (f *FightSt) StartUp() {
	if f.client != nil {
		f.client.Startup()
	}
}

func (f *FightSt) Close() {
	if f.client != nil {
		f.client.Destroy()
	}
}

// TODO: 名称待定
type FightHostInfo struct {
	Host string
	Name string
	Camp crosscamp.CampType

	ZoneId  uint32
	CrossId uint32
}

var fightHosts = map[base.ServerType]FightHostInfo{} // key: 战斗服类型 val: 战斗服地址信息

var (
	fights = make(map[base.ServerType]*FightSt) // key: 战斗服类型 val: 战斗服客户端
)

func AddFightClient(ftype base.ServerType, hostInfo *FightHostInfo) {
	if hostInfo == nil {
		logger.LogStack("hostInfo is nil")
		return
	}

	_, ok := fightHosts[ftype]
	if ok {
		ClientDestroy(ftype)
	}

	fight := &FightSt{}
	fightHosts[ftype] = *hostInfo
	fights[ftype] = fight
	fight.client = NewFightClient(hostInfo.Host, GenNewFightAgentFunc(ftype))
	logger.LogInfo("添加[%s]战斗服", base.ServerTypeStrMap[ftype])
}

func GetFightClient(ftype base.ServerType) *FightSt {
	return fights[ftype]
}

func GetHostInfo(ftype base.ServerType) FightHostInfo {
	return fightHosts[ftype]
}

func DelHostInfo(ftype base.ServerType) {
	delete(fightHosts, ftype)
}

func GenNewFightAgentFunc(ftype base.ServerType) newAgentfunc {
	hostInfo, ok := fightHosts[ftype]
	if !ok {
		return func(conn *network.TCPConn) network.Agent { return nil }
	}

	return func(conn *network.TCPConn) network.Agent {
		fight := fights[ftype]
		fight.agent = NewFightAgentBase(conn, hostInfo.Name, hostInfo.Camp)
		return fight.agent
	}
}

func ClientDestroy(fType base.ServerType) {
	if fight, exist := fights[fType]; exist {
		if fight.client != nil {
			fight.client.Destroy()
			fight.client = nil
			logger.LogInfo("关闭[%s]战斗服连接", base.ServerTypeStrMap[fType])
		}
		delete(fights, fType)
	}
}

func sendToFight(ftype base.ServerType, cmdId uint16, msg pb3.Message) error {
	buf := [2]byte{}

	binary.LittleEndian.PutUint16(buf[:2], cmdId)

	data, err := pb3.Marshal(msg)
	if nil != err {
		return fmt.Errorf("send message to fight ftype: %d failed marshal error %s", ftype, err)
	}

	fight, ok := fights[ftype]
	if !ok {
		return fmt.Errorf("send message to fight ftype: %d failed not found fight client", ftype)
	}

	if fight.agent == nil {
		return fmt.Errorf("send message to fight ftype: %d failed not found fight agent", ftype)
	}

	return fight.agent.conn.WriteMsgWithTrace(buf[:], data)
}

func fightClientExistP(ftype base.ServerType) bool {
	_, ok := fights[ftype]
	return ok
}

func init() {
	engine.SendToFight = sendToFight
	engine.FightClientExistPredicate = fightClientExistP
	engine.GetMediumCrossCamp = getMediumCrossCamp
	engine.GetSmallCrossCamp = GetSmallCrossCamp
}
