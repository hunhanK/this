/**
 * @Author: PengZiMing
 * @Desc: 处理从战斗服传回来需要保存的数据
 * @Date: 2021/10/11 11:50
 */

package entity

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/guildmgr"
)

func (player *Player) PackCreateData() *pb3.CreateActorData {
	create := &pb3.CreateActorData{
		Job:        player.GetJob(),
		Sex:        player.GetSex(),
		Name:       player.GetName(),
		Circle:     player.GetCircle(),
		Level:      player.GetLevel(),
		LastPkMode: player.GetBinaryData().PkMode,
	}

	if guild := guildmgr.GetGuildById(player.GetGuildId()); nil != guild {
		create.GuildName = guild.GetName()
		member := guild.GetMember(player.GetId())
		create.GuildPos = member.GetPosition()
		create.GuildId = player.GetGuildId()
	}

	if marData := player.GetBinaryData().MarryData; marData != nil {
		if friendmgr.IsExistStatus(marData.CommonId, custom_id.FsMarry) {
			create.MarryId = marData.MarryId
		}
	}

	if binary := player.GetBinaryData(); nil != binary {
		create.Hp = binary.Hp
		create.LastPkMode = binary.LastPkMode
		create.BuffList = binary.BuffList
		create.DropData = binary.DropData
		create.Zhenyuan = binary.Zhenyuan
		if binary.UnknownDarkTempleSec == nil {
			binary.UnknownDarkTempleSec = make(map[uint32]uint32)
		}
		create.UnknownDarkTempleSec = binary.UnknownDarkTempleSec
		if nil != binary.GetQiMen() {
			create.Energy = binary.GetQiMen().Energy
		}

		if mythicalBeastLand := binary.GetMythicalBeastLand(); nil != mythicalBeastLand {
			create.MythicalBeastLandInfo = &pb3.MythicalBeastLandInfo{
				Gather: mythicalBeastLand.Gather,
				Energy: mythicalBeastLand.Energy,
			}
		}

		statData := binary.DailyStatData
		// 首次创角处理
		if statData == nil {
			statData = &pb3.DailyStatData{}
			binary.DailyStatData = statData
		}
		if statData.DayZeroAt == 0 {
			statData.DayZeroAt = time_util.GetDaysZeroTime(0)
			statData.MonKillMap = make(map[uint32]uint32)
		}

		create.DemonKingSecKillInfo = &pb3.DemonKingSecKillInfo{}
		if binary.DemonKingSecKillData != nil {
			create.DemonKingSecKillInfo.IsOpen = binary.DemonKingSecKillData.IsOpen
		}

		create.DailyStatData = statData
	}

	create.ShowStr = player.PackageShowStr()

	if sys, ok := player.GetSysObj(sysdef.SiSkill).(iface.ISkillSys); ok {
		sys.PackFightSrvSkill(create)
	}

	if sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(iface.IImmortalSoulSys); ok {
		sys.PackFightSrvBattleSoul(create)
	}

	if sys, ok := player.GetSysObj(sysdef.SiLaw).(iface.ILawSys); ok {
		sys.PackFightSrvLawInfo(create)
	}

	if sys, ok := player.GetSysObj(sysdef.SiCustomTip).(iface.ICustomTipSys); ok {
		sys.PackFightCustomTip(create)
	}

	if sys := player.GetSysObj(sysdef.SiBattleShield); sys != nil && sys.IsOpen() {
		create.OpenBattleShieldSys = true
	}

	if sys, ok := player.GetSysObj(sysdef.SiDomain).(iface.IDomainSys); ok {
		sys.PackFightSrvDomainInfo(create)
	}

	if sys, ok := player.GetSysObj(sysdef.SiDoubleDropPrivilege).(iface.IPackCreateData); ok {
		sys.PackCreateData(create)
	}

	if sys := player.GetAttrSys(); nil != sys {
		sys.PackCreateData(create)
	}

	return create
}
