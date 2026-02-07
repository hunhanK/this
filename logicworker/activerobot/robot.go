package activerobot

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/activerobot/internal/system"
	"jjyz/gameserver/logicworker/guildmgr"
)

type Robot struct {
	robotId uint64 // 机器人id
	change  bool   // 是否改变

	liveTime uint32 // 存活结束时间

	data *pb3.MainCityRobotData

	attrSys *system.AttrSys
	sysMgr  *system.Mgr //系统管理器
}

func (r *Robot) Change() bool {
	return r.change
}

func (r *Robot) SetChange(change bool) {
	r.change = change
}

func (r *Robot) Logout() {
	r.ClearFlagBit(custom_id.AfOnline)
	r.SetFlagBit(custom_id.AfNeedSync)
	r.data.LogoutTime = time_util.NowSec()
	r.sysMgr.OnLogout()
	r.SetChange(true)
}

func (r *Robot) Login() {
	// 设置存活时间
	putCandMap := jsondata.GetMainCityRobotConfPutCand()
	cand := putCandMap[gshare.GetOpenServerDay()]

	// 加入上线标识
	r.SetFlagBit(custom_id.AfOnline)
	r.SetFlagBit(custom_id.AfNeedSync)
	r.liveTime = time_util.NowSec() + cand.LiveSecond
	r.sysMgr.OnLogin()
	r.data.LoginTime = time_util.NowSec()
	r.SetChange(true)
}

func (r *Robot) GetRobotId() uint64 {
	if r.data == nil {
		return 0
	}
	return r.data.Id
}

func (r *Robot) GetLevel() uint32 {
	if r.data == nil {
		return 0
	}
	return r.data.Level
}

func (r *Robot) GetName() string {
	if r.data == nil {
		return ""
	}
	return r.data.Name
}

func (r *Robot) SetGuildId(id uint64) {
	if r.data == nil {
		return
	}
	r.data.GuildId = id
}

func (r *Robot) GetGuildId() uint64 {
	if r.data == nil {
		return 0
	}
	return r.data.GuildId
}

func (r *Robot) GetData() *pb3.MainCityRobotData {
	return r.data
}

func (r *Robot) GetJob() uint32 {
	if r.data == nil {
		return 0
	}
	return r.data.Job
}

func (r *Robot) GetSex() uint32 {
	if r.data == nil {
		return 0
	}
	return uint32(r.GetAttr(attrdef.Job)) & (1 << base.SexBit)
}

func (r *Robot) IsOnline() bool {
	return r.IsFlagBit(custom_id.AfOnline)
}

func (r *Robot) IsFlagBit(bit uint32) bool {
	return utils.IsSetBit64(r.GetFlag(), bit)
}

func (r *Robot) SetFlagBit(bit uint32) {
	r.SetFlag(utils.SetBit64(r.GetFlag(), bit))
}

func (r *Robot) GetSysObj(sysId int) iface.IRobotSystem {
	if r.sysMgr == nil {
		return nil
	}
	obj := r.sysMgr.GetSysObj(sysId)
	if obj == nil {
		return nil
	}
	return obj
}

func (r *Robot) ResetSysAttr(sysId uint32) {
	if r.attrSys == nil {
		return
	}
	r.attrSys.ResetSysAttr(sysId)
}

func (r *Robot) SetAttr(attrType attrdef.AttrTypeAlias, attrValue attrdef.AttrValueAlias) {
	if r.attrSys == nil {
		return
	}
	r.attrSys.SetAttr(attrType, attrValue)
}

func (r *Robot) GetAttr(attrType attrdef.AttrTypeAlias) attrdef.AttrValueAlias {
	if r.attrSys == nil {
		return 0
	}
	return r.attrSys.GetAttr(attrType)
}

func (r *Robot) SyncFightData() *pb3.RobotSync2Fight {
	data := r.GetData()
	if nil == data {
		return nil
	}
	hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)

	ret := &pb3.RobotSync2Fight{
		Id:             r.robotId,
		Name:           data.GetName(),
		GuildName:      r.GetGuildName(),
		GuildId:        data.GetGuildId(),
		SmallCrossCamp: uint32(hostInfo.Camp),
	}

	r.attrSys.PackSyncData(ret)
	return ret
}

func (r *Robot) GetFlag() uint64 {
	if nil != r.data {
		return r.data.GetFlag()
	}
	return 0
}

func (r *Robot) SetFlag(flag uint64) {
	if nil != r.data {
		r.data.Flag = flag
	}
}

func (r *Robot) ClearFlagBit(bit uint32) {
	r.SetFlag(utils.ClearBit64(r.GetFlag(), bit))
}

func (r *Robot) GetGuildName() string {
	guildId := r.GetGuildId()
	if guildId == 0 {
		return ""
	}
	if guild := guildmgr.GetGuildById(guildId); nil != guild {
		return guild.GetName()
	}
	return ""
}

func (r *Robot) GetLastLogoutTime() uint32 {
	return r.data.LogoutTime
}

func (r *Robot) GetLoginTime() uint32 {
	return r.data.LoginTime
}

func (r *Robot) GetBubbleFrame() uint32 {
	bubbleFrameRaw := r.GetAttr(attrdef.AppearBubbleFrame)
	foo := bubbleFrameRaw >> 32
	bubbleFrame := (foo << 32) ^ bubbleFrameRaw
	return uint32(bubbleFrame)
}

func (r *Robot) GetHeadFrame() uint32 {
	headFrameRaw := r.GetAttr(attrdef.AppearHeadFrame)
	foo := headFrameRaw >> 32
	headframe := (foo << 32) ^ headFrameRaw
	return uint32(headframe)
}

func (r *Robot) GetHead() uint32 {
	headRaw := r.GetAttr(attrdef.AppearHead)
	foo := headRaw >> 32
	head := (foo << 32) ^ headRaw
	return uint32(head)
}

func (r *Robot) getPowerCompare() map[uint32]int64 {
	powerC := make(map[uint32]int64)
	powerCopareSlice := jsondata.GetPowerCompareConf()
	for _, conf := range powerCopareSlice {
		var power int64
		for _, attrDefId := range conf.AttrGroup {
			calc := r.attrSys.GetAttrCalcByAttrId(attrDefId)
			fightVal := calc.GetFightValue(int8(r.GetJob()))
			power += fightVal
		}
		powerC[conf.Id] += power
	}
	return powerC
}

func checkSync2Fight() {
	for robotId, robot := range robotMap {
		if !robot.IsFlagBit(custom_id.AfNeedSync) {
			continue
		}
		var err error
		if robot.IsFlagBit(custom_id.AfOnline) && !robot.IsFlagBit(custom_id.AfKick) {
			if data := robot.SyncFightData(); nil != data {
				// 这里不单只是创建机器人, 同步机器人数据也是在这里
				err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateActiveRobot, data)
			}
		} else {
			err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FActiveRobotOffline, &pb3.CommonSt{U64Param: robotId})
		}
		if err != nil {
			logger.LogError("err:%v", err)
		}
		robot.ClearFlagBit(custom_id.AfNeedSync)
	}
}

func NewRobot(robotId uint64, data *pb3.MainCityRobotData) *Robot {
	if nil == data {
		return nil
	}
	robot := &Robot{robotId: robotId, data: data}
	robot.attrSys = system.CreateAttrSys(robot)
	robot.SetAttr(attrdef.Job, attrdef.AttrValueAlias(data.Job<<base.SexBit|data.Sex))
	hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)
	robot.SetAttr(attrdef.SmallCrossCamp, attrdef.AttrValueAlias(hostInfo.Camp))
	robot.sysMgr = system.CreateSysMgr(robot)
	robot.attrSys.ResetAttr() // 初始化完重算一次属性
	return robot
}
