/**
 * @Author: LvYuMeng
 * @Date: 2024/4/17
 * @Desc:
**/

package guildmgr

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

func CreateRobotMember(robot iface.IRobot) *pb3.GuildMemberInfo {
	if nil == robot {
		return nil
	}

	member := &pb3.GuildMemberInfo{JoinTime: time_util.NowSec()}
	member.PlayerInfo = manager.GetRobotSimplyData(robot.GetRobotId())

	return member
}

func GuildAddRobotMember(guild *Guild, robot iface.IRobot, pos uint32) {
	if nil == guild || nil == robot {
		return
	}

	member := CreateRobotMember(robot)
	if nil == member {
		return
	}

	member.IsOnline = robot.IsFlagBit(custom_id.AfOnline)

	guild.addMember(member, pos)
}

func (guild *Guild) RobotApplyJoin(robot iface.IRobot) bool {
	if nil == robot {
		return false
	}
	if guild.IsFull() {
		return false
	}

	if robot.GetGuildId() > 0 {
		return false
	}

	ok, canAutoEnter := guild.checkApplyCondRobot(robot)
	if !ok {
		return false
	}

	if canAutoEnter {
		GuildAddRobotMember(guild, robot, custom_id.GuildPos_Common)
	} else {
		guild.addRobotToApply(robot)
	}
	return true
}

func (guild *Guild) checkActiveCond() bool {
	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return false
	}

	var applyCount uint32
	applyList := guild.GetBinary().ApplyIds
	for actor := range applyList {
		if engine.IsRobot(actor) {
			applyCount++
			if applyCount >= conf.MaxApplyCount {
				return false
			}
			continue
		}
	}

	if guild.IsRobotFull() {
		return false
	}

	mxLv := uint32(len(conf.TotalCountCond))
	index := utils.Ternary(guild.GetLevel() >= mxLv, mxLv-1, guild.GetLevel()).(uint32)

	//行会人数小于60人时，机器人可以申请进行会
	if guild.GetMemberCount() < conf.TotalCountCond[index] {
		return true
	}

	return false
}

// 条件通过,可否自动加入
func (guild *Guild) checkApplyCondRobot(robot iface.IRobot) (bool, bool) {
	if nil == robot {
		return false, false
	}

	level := robot.GetLevel()
	fightValue := robot.GetAttr(attrdef.FightValue)
	if !CheckApplyLv(level) {
		return false, false
	}

	if !guild.checkActiveCond() {
		return false, false
	}

	mode := guild.GetBasicInfo().GetMode()
	switch mode {
	case custom_id.GuildApplyMode_Verify:
		return true, false
	case custom_id.GuildApplyMode_Ban:
		return false, false
	case custom_id.GuildApplyMode_Cond:
		minCombat := guild.BasicInfo.GetApplyPower()
		minLevel := guild.BasicInfo.GetApplyLevel()
		if level < minLevel || fightValue < minCombat {
			return false, false
		}
		return true, true
	}

	return false, false
}

func (guild *Guild) addRobotToApply(robot iface.IRobot) {
	if nil == robot {
		return
	}

	actorId := robot.GetRobotId()

	member := guild.getApplyInfo(actorId)
	if nil != member {
		return
	}

	if guild.IsApply(actorId) {
		return
	}

	binary := guild.GetBinary()
	nowSec := time_util.NowSec()
	binary.ApplyIds[actorId] = nowSec

	member = &pb3.GuildApplyInfo{
		PlayerInfo: manager.GetRobotSimplyData(actorId),
		ApplyTime:  nowSec,
	}

	// 通知加入列表
	guild.BroadcastProto(29, 3, &pb3.S2C_29_3{
		Member: member,
	})

	SetSaveFlag(guild.GetId())
}

func (guild *Guild) OnRobotLoadFinish(robot iface.IRobot) {
	if nil == robot {
		return
	}

	robotId := robot.GetRobotId()
	member := guild.GetMember(robotId)
	if nil == member {
		return
	}

	rData := robot.GetData()
	role := &pb3.SimplyPlayerData{
		Id:             robotId,
		Name:           rData.GetName(),
		Circle:         rData.GetCircle(),
		Lv:             rData.GetLevel(),
		VipLv:          rData.GetVip(),
		Job:            rData.GetJob(),
		Sex:            rData.GetSex(),
		GuildId:        rData.GetGuildId(),
		LastLogoutTime: rData.GetLogoutTime(),
		LoginTime:      rData.GetLoginTime(),
		Power:          uint64(rData.GetFightValue()),
	}
	guild.OnMemberDataBaseChange(role)
}

func (guild *Guild) OnRobotLogin(robot iface.IRobot) {
	if nil == robot {
		return
	}

	robotId := robot.GetRobotId()
	member := guild.GetMember(robotId)
	if nil == member {
		return
	}

	robot.SetAttr(attrdef.GuildPos, int64(member.GetPosition()))
	member.IsOnline = true
	guild.OnMemberDataBaseChange(manager.GetSimplyData(robotId))
}

func (guild *Guild) OnRobotLogout(robot iface.IRobot) {
	if nil == robot {
		return
	}

	robotId := robot.GetRobotId()
	member := guild.GetMember(robotId)
	if nil == member {
		return
	}
	member.IsOnline = false
	guild.OnMemberDataBaseChange(manager.GetSimplyData(robotId))
}

func CreateRobotGuild(robot iface.IRobot) bool {
	if nil == robot {
		return false
	}

	nameMap := jsondata.GuildRobotNameMap
	if nil == nameMap {
		return false
	}

	var name string
	for tmpName := range nameMap {
		if !CheckGuildName(tmpName) {
			continue
		}
		name = tmpName
	}
	if name == "" {
		return false
	}

	member := CreateRobotMember(robot)
	if nil == member {
		return false
	}

	member.IsOnline = robot.IsFlagBit(custom_id.AfOnline)
	member.JoinTime = time_util.NowSec()

	gConf := jsondata.GetGuildConf()
	if nil == gConf {
		return false
	}

	bannerPicture := gConf.GuildBanner[random.Interval(0, utils.MaxInt(len(gConf.GuildBanner)-1, 0))]
	bannerColour := gConf.GuildBannerColour[random.Interval(0, utils.MaxInt(len(gConf.GuildBannerColour)-1, 0))]

	banner := &pb3.GuildBanner{
		BannerPicture: bannerPicture.Type,
		BannerColor:   bannerColour.Type,
		BannerChar:    string([]rune(name)[0]),
	}

	guild := NewGuild(name, banner)

	guildId := guild.BasicInfo.GetId()

	allGuildName[name] = struct{}{}

	GuildMap[guildId] = guild

	guild.BasicInfo.LeaderId = robot.GetRobotId()

	guild.addMember(member, custom_id.GuildPos_Common)
	member.Position = custom_id.GuildPos_Leader
	robot.SetAttr(attrdef.GuildPos, int64(member.Position))

	guild.SetGuildLeader(member)

	toUpGuildRank(guild.GetId(), guild.GetLevel(), guild.GetPower())
	guild.Save()
	return true
}
