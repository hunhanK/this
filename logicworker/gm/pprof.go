package gm

import (
	"github.com/gzjjyz/logger"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"
)

var (
	pProfPort int //pProf监听端口
)

// 通过信号开启pProf
func sSetPProf() {
	if pProfPort == 0 { //默认不开启
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			logger.LogError("监听pProf失败:%s", err)
		} else {
			l, err := net.ListenTCP("tcp", addr)
			if err != nil {
				logger.LogError("监听pProf失败:%s", err)
				return
			}
			l.Close()
			pProfPort = l.Addr().(*net.TCPAddr).Port
			logger.LogInfo("命令行开启pProf:%v", pProfPort)
			go http.ListenAndServe(":"+strconv.Itoa(pProfPort), nil)
		}
	} else {
		logger.LogInfo("命令行开启pProf:%v", pProfPort)
	}
}

func init() {
	gmevent.Register("pprof", func(actor iface.IPlayer, args ...string) bool {
		sSetPProf()
		return true
	}, 1)
}
