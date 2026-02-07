package gateworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
)

const MaxGateAgentCount = 2

var (
	gGatePool = func() *GatePool {
		pool := &GatePool{p: make(chan *GateAgent, MaxGateAgentCount)}
		for idx := uint32(0); idx < MaxGateAgentCount; idx++ {
			st := &GateAgent{GateIdx: idx}
			for connId := uint32(0); connId < base.MaxGateUserCount; connId++ {
				st.UserList[connId] = &UserSt{Closed: true, GateId: idx, ConnId: connId}
			}
			pool.p <- st
		}
		return pool
	}()
)

type GatePool struct {
	p chan *GateAgent
}

func (st *GatePool) Get() (agent *GateAgent) {
	select {
	case agent = <-st.p:
	default:
		logger.LogError("gate agent pool get nil")
		return nil
	}
	return
}

func (st *GatePool) Put(agent *GateAgent) {
	select {
	case st.p <- agent:
	default:
		logger.LogError("gate agent pool is full!!!")
	}
}
