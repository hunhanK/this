package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

type Base struct {
	owner  iface.IPlayer
	sysMgr iface.ISystemMgr
	sysId  uint32
	logger.ILogger
}

func (sys *Base) GetSysId() uint32 {
	return sys.sysId
}

func (sys *Base) Init(sysId uint32, sysMgr iface.ISystemMgr, player iface.IPlayer) {
	sys.sysId = sysId
	sys.sysMgr = sysMgr
	sys.owner = player
	sys.ILogger = base.NewSysLogger(fmt.Sprintf("sys:%d,playerId:%d", sysId, player.GetId()))
}

func (sys *Base) GetMainData() *pb3.PlayerMainData {
	return sys.owner.GetMainData()
}

func (sys *Base) GetBinaryData() *pb3.PlayerBinaryData {
	return sys.owner.GetBinaryData()
}

func (sys *Base) GetPlayerData() *pb3.PlayerData {
	return sys.owner.GetPlayerData()
}

func (sys *Base) ResetSysAttr(id uint32) {
	if st := sys.owner.GetAttrSys(); nil != st {
		st.ResetSysAttr(id)
	}
}

func (sys *Base) SendProto3(sysId, cmdId uint16, msg pb3.Message) {
	sys.owner.SendProto3(sysId, cmdId, msg)
}

func (sys *Base) Open() {
	sys.owner.SetSysStatus(sys.sysId, true)
}

func (sys *Base) Close() {
	sys.owner.SetSysStatus(sys.sysId, false)
}

func (sys *Base) IsOpen() bool {
	return sys.owner.GetSysStatus(sys.sysId)
}

func (sys *Base) GetOwner() iface.IPlayer {
	return sys.owner
}

func (sys *Base) OnSave()       {}
func (sys *Base) OnOpen()       {}
func (sys *Base) OnClose()      {}
func (sys *Base) OnInit()       {}
func (sys *Base) OnLogin()      {}
func (sys *Base) OnLogout()     {}
func (sys *Base) OnAfterLogin() {}
func (sys *Base) OnDestroy()    {}
func (sys *Base) OnLoginFight() {}
func (sys *Base) OnAfterMerge() {}
func (sys *Base) OnNewDay()     {}
func (sys *Base) OneSecLoop()   {}
func (sys *Base) DataFix()      {}
