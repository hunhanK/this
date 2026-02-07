/**
 * @Author: LvYuMeng
 * @Date: 2024/4/17
 * @Desc:
**/

package system

import (
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/manager"
)

// GuildSys 机器人行会系统
type GuildSys struct {
	System
}

func (sys *GuildSys) OnInit() {
	data := sys.owner.GetData()
	if nil == data {
		return
	}
	sys.GetOwner().SetAttr(attrdef.GuildId, int64(data.GetGuildId()))
}

func (sys *GuildSys) OnLoadFinish() {
	if guild := sys.GetGuild(); nil != guild {
		guild.OnRobotLoadFinish(sys.owner)
	}
}

func (sys *GuildSys) OnReset() {
	if pGuild := sys.GetGuild(); nil != pGuild {
		robotId := sys.owner.GetRobotId()
		member := pGuild.GetMember(robotId)
		if nil == member {
			return
		}
		pGuild.RemoveMember(robotId)
	}
}

func (sys *GuildSys) GetGuild() *guildmgr.Guild {
	guildId := sys.owner.GetGuildId()
	if guildId <= 0 {
		return nil
	}
	return guildmgr.GetGuildById(guildId)
}

func (sys *GuildSys) OnLogin() {
	if guild := sys.GetGuild(); nil != guild {
		guild.OnRobotLogin(sys.owner)
	}
}

func (sys *GuildSys) OnLogout() {
	if guild := sys.GetGuild(); nil != guild {
		guild.OnRobotLogout(sys.owner)
	}
}

func (sys *GuildSys) DoUpdate() {
	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return
	}
	level := sys.owner.GetLevel()
	if level <= conf.ApplyLevel {
		return
	}

	robotId := sys.owner.GetRobotId()

	inGuild := false

	guildId := sys.owner.GetGuildId()
	if guild := guildmgr.GetGuildById(guildId); nil != guild {
		if member := guild.GetMember(robotId); nil == member {
			sys.owner.SetGuildId(0)
		} else {
			inGuild = true
		}
	}
	if !inGuild {
		for tid := range guildmgr.GuildMap {
			if guild := guildmgr.GuildMap[tid]; nil != guild {
				if member := guild.GetMember(robotId); member != nil {
					if !inGuild {
						inGuild = true
						sys.owner.SetGuildId(tid) //离线上线加入行会
					} else {
						guild.RemoveMember(robotId) //已经有加入,则后面的行会要移除
					}
				}
			}
		}
		if !inGuild && guildId > 0 { //这种情况可能行会已被解散
			sys.owner.SetGuildId(0)
		}
	}

	if !inGuild {
		sys.checkCreateGuild()
		sys.checkJoinGuild()
	} else {
		guild := sys.GetGuild()
		guild.OnMemberDataBaseChange(manager.GetSimplyData(robotId))
	}
}

func (sys *GuildSys) checkJoinGuild() {
	if sys.owner.GetGuildId() > 0 {
		return // 已在行会中
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeGuild)
	rankConf := jsondata.GetRankConf(gshare.RankTypeGuild)
	if nil == rankConf {
		return
	}

	rankLine := rank.GetList(1, int(rankConf.ShowMaxLimit))
	for i := len(rankLine) - 1; i >= 0; i-- {
		r := rankLine[i]
		guild := guildmgr.GetGuildById(r.GetId())
		if nil == guild {
			continue
		}
		if guild.RobotApplyJoin(sys.owner) {
			return
		}
	}
}

func (sys *GuildSys) checkCreateGuild() {
	if sys.owner.GetGuildId() > 0 {
		return // 已在行会中
	}

	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return
	}

	gNum := uint32(len(guildmgr.GuildMap))
	if gNum >= conf.CreateGuildLimit {
		return
	}

	if gNum > 0 {
		for _, guild := range guildmgr.GuildMap {
			if !guild.IsFull() {
				return
			}
		}
	}

	gConf := jsondata.GetGuildConf()
	if nil == gConf || nil == gConf.Create {
		return
	}

	if uint32(sys.owner.GetAttr(attrdef.VipLevel)) < gConf.Create.CreateNeedVipLv {
		return
	}

	guildmgr.CreateRobotGuild(sys.owner)
}

func (sys *GuildSys) GetUpdateInterval() uint32 {
	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return 0
	}

	return conf.ApplyCd * 60
}

func init() {
	RegSysClass(SiGuild, func(sysId int, owner iface.IRobot) iface.IRobotSystem {
		sys := &GuildSys{}
		sys.sysId = sysId
		sys.owner = owner
		return sys
	})
}
