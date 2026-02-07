package pyy

import (
	"fmt"
	"github.com/gzjjyz/logger"
	base2 "jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
)

type PlayerYYBase struct {
	Id       uint32 // 活动id
	OpenTime uint32 // 开启时间
	EndTime  uint32 // 结束时间
	ConfIdx  uint32 // 配置索引
	ConfName string // 配置名
	Class    uint32 // 模板类型
	player   iface.IPlayer

	pbYYStateInfo *pb3.YYStateInfo
	logger.ILogger
}

func (base *PlayerYYBase) GetPrefix() string {
	return fmt.Sprintf("confName:%s,confIdx:%d,class:%d", base.ConfName, base.ConfIdx, base.Class)
}

func (base *PlayerYYBase) GetClass() uint32 { return base.Class }

func (base *PlayerYYBase) Init(player iface.IPlayer, oTime, eTime uint32) {
	base.OpenTime = oTime
	base.EndTime = eTime

	// 发给前端的结构体
	base.pbYYStateInfo = &pb3.YYStateInfo{
		ActId:     base.Id,
		Status:    custom_id.YS_Start,
		ConfIdx:   base.ConfIdx,
		StartTime: base.OpenTime,
		EndTime:   base.EndTime,
	}

	if line := jsondata.GetPlayerYYConf(base.Id); nil != line {
		base.ConfName = line.ConfName
		base.Class = line.Class
	}

	base.player = player
	base.ILogger = base2.NewSysLogger(fmt.Sprintf("confName:%s,confIdx:%d,class:%d,player:%d", base.ConfName, base.ConfIdx, base.Class, player.GetId()))
}

func (base *PlayerYYBase) SetId(id uint32) {
	base.Id = id
}

func (base *PlayerYYBase) GetId() uint32 {
	return base.Id
}

func (base *PlayerYYBase) GetActName() string {
	if line := jsondata.GetPlayerYYConf(base.Id); nil != line {
		return line.Name
	}
	return ""
}

func (base *PlayerYYBase) GetOpenDay() uint32 {
	return time_util.GetZeroTime(time_util.NowSec())/86400 - time_util.GetZeroTime(base.OpenTime)/86400 + 1
}

func (base *PlayerYYBase) GetPlayer() iface.IPlayer { return base.player }

func (base *PlayerYYBase) SetPlayer(player iface.IPlayer) { base.player = player }

func (base *PlayerYYBase) GetYYData() *pb3.PlayerYYData {
	pb3binary := base.GetPlayer().GetBinaryData()
	return pb3binary.YyData
}

func (base *PlayerYYBase) SetConfIdx(idx uint32) {
	base.ConfIdx = idx
	base.pbYYStateInfo.ConfIdx = idx
	base.ILogger = base2.NewSysLogger(fmt.Sprintf("confName:%s,confIdx:%d,class:%d,player:%d", base.ConfName, base.ConfIdx, base.Class, base.player.GetId()))
}

func (base *PlayerYYBase) GetConfIdx() uint32 {
	return base.ConfIdx
}

func (base *PlayerYYBase) GetOpenTime() uint32 {
	return base.OpenTime
}

func (base *PlayerYYBase) GetEndTime() uint32 {
	return base.EndTime
}

func (base *PlayerYYBase) SetEndTime(eTime uint32) {
	base.EndTime = eTime
}

func (base *PlayerYYBase) IsOpen() bool {
	now := time_util.NowSec()
	return base.OpenTime <= now && base.EndTime > now
}

func (base *PlayerYYBase) OnInit()       {}
func (base *PlayerYYBase) OnOpen()       {}
func (base *PlayerYYBase) OnEnd()        {}
func (base *PlayerYYBase) Login()        {}
func (base *PlayerYYBase) OnAfterLogin() {}
func (base *PlayerYYBase) OnLogout()     {}
func (base *PlayerYYBase) BeforeNewDay() {}
func (base *PlayerYYBase) NewDay()       {}

func (base *PlayerYYBase) QuestEvent(qt, id, count uint32) {}

func (base *PlayerYYBase) GetYYStateInfo() *pb3.YYStateInfo { return base.pbYYStateInfo }

func (base *PlayerYYBase) BugFix()                                  {}
func (base *PlayerYYBase) PlayerUseDiamond(count int64)             {}
func (base *PlayerYYBase) PlayerCharge(*custom_id.ActorEventCharge) {}
func (base *PlayerYYBase) ResetData()                               {}
func (base *PlayerYYBase) MergeFix()                                {}
func (base *PlayerYYBase) CmdYYFix()                                {}
func (base *PlayerYYBase) OnLoginFight()                            {}
func (base *PlayerYYBase) SendProto3(sysId, cmdId uint16, msg pb3.Message) {
	base.player.SendProto3(sysId, cmdId, msg)
}

func (base *PlayerYYBase) SendYYStateInfo() {
	base.player.SendProto3(40, 250, &pb3.S2C_40_250{Info: base.GetYYStateInfo()})
}

func (base *PlayerYYBase) IsUseRealCharge() bool {
	if pyyConf := jsondata.GetPlayerYYConf(base.Id); nil != pyyConf {
		return pyyConf.SkipDiamondChargeTokens
	}

	return false
}

func (base *PlayerYYBase) GetDailyChargeMoney(timestamp uint32) uint32 {
	return base.GetPlayer().GetDailyChargeMoney(timestamp, base.IsUseRealCharge())
}

func (base *PlayerYYBase) GetDailyCharge() uint32 {
	return base.GetPlayer().GetDailyCharge(base.IsUseRealCharge())
}
