package yy

import (
	"fmt"
	"github.com/gzjjyz/logger"
	base2 "jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

var _ iface.IYunYing = (*YYBase)(nil)

type YYBase struct {
	Id       uint32 // 活动id
	OpenTime uint32 // 开启时间
	EndTime  uint32 // 结束时间
	ConfIdx  uint32 // 配置索引
	ConfName string // 配置名
	Class    uint32 // 模板类型

	pbYYStateInfo *pb3.YYStateInfo
	logger.ILogger
}

func (base *YYBase) GetPrefix() string {
	return fmt.Sprintf("confName:%s,confIdx:%d,class:%d", base.ConfName, base.ConfIdx, base.Class)
}

func (base *YYBase) PlayerLogin(_ iface.IPlayer) {}

func (base *YYBase) PlayerReconnect(_ iface.IPlayer) {}

func (base *YYBase) GetClass() uint32 { return base.Class }

func (base *YYBase) Init(oTime, eTime uint32) {
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

	if line := jsondata.GetYunYingConf(base.Id); nil != line {
		base.ConfName = line.ConfName
		base.Class = line.Class
	}
	base.ILogger = base2.NewSysLogger(fmt.Sprintf("confName:%s,confIdx:%d,class:%d", base.ConfName, base.ConfIdx, base.Class))
}

func (base *YYBase) SetId(id uint32) {
	base.Id = id
}

func (base *YYBase) GetId() uint32 {
	return base.Id
}

func (base *YYBase) GetOpenDay() uint32 {
	return time_util.GetZeroTime(time_util.NowSec())/86400 - time_util.GetZeroTime(base.OpenTime)/86400 + 1
}

func (base *YYBase) SetConfIdx(idx uint32) {
	base.ConfIdx = idx
	base.pbYYStateInfo.ConfIdx = idx
	base.ILogger = base2.NewSysLogger(fmt.Sprintf("confName:%s,confIdx:%d,class:%d", base.ConfName, base.ConfIdx, base.Class))
}

func (base *YYBase) GetConfIdx() uint32 {
	return base.ConfIdx
}

func (base *YYBase) GetOpenTime() uint32 {
	return base.OpenTime
}

func (base *YYBase) GetEndTime() uint32 {
	return base.EndTime
}

func (base *YYBase) SetEndTime(eTime uint32) {
	base.EndTime = eTime
}

func (base *YYBase) IsOpen() bool {
	now := time_util.NowSec()
	return base.OpenTime <= now && base.EndTime > now
}

func (base *YYBase) OnInit()       {}
func (base *YYBase) OnOpen()       {}
func (base *YYBase) OnEnd()        {}
func (base *YYBase) BeforeNewDay() {}
func (base *YYBase) NewDay()       {}

func (base *YYBase) QuestEvent(qt, id, count uint32) {}

func (base *YYBase) GetYYStateInfo() *pb3.YYStateInfo { return base.pbYYStateInfo }

func (base *YYBase) BugFix()                                  {}
func (base *YYBase) PlayerUseDiamond(count int64)             {}
func (base *YYBase) PlayerCharge(*custom_id.ActorEventCharge) {}
func (base *YYBase) Broadcast(sysId, cmdId uint16, msg pb3.Message) {
	engine.Broadcast(chatdef.CIWorld, 0, sysId, cmdId, msg, 0)
}
func (base *YYBase) BroadcastYYStateInfo() {
	engine.Broadcast(chatdef.CIWorld, 0, 40, 0, &pb3.S2C_40_0{Info: base.GetYYStateInfo()}, 0)
}
func (base *YYBase) ServerStopSaveData() {}
func (base *YYBase) ResetData()          {}
