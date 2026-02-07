package dbworker

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
)

func saveGuildData(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveGuildData", 1, len(args)) {
		return
	}
	var guildInfo pb3.GuildInfo
	if err := pb3.Unmarshal(args[0].([]byte), &guildInfo); err != nil {
		return
	}
	basicInfo := guildInfo.GetBasicInfo()
	guildId := basicInfo.GetId()
	saveGuild(&guildInfo)
	saveGuildMemberData(guildId, guildInfo.Members)
	saveNewGuildItem(guildId, guildInfo.DepotItems)
}

func saveGuildMemberData(guildId uint64, memberList map[uint64]*pb3.GuildMemberInfo) {
	srvId := GetSrvIdFromGuildId(guildId)
	for playerId, line := range memberList {
		if _, err := db.OrmEngine.Exec("call saveGuildMemberData(?,?,?,?,?,?)", guildId, srvId, playerId, line.GetPosition(), line.GetDonate(), line.GetJoinTime()); nil != err {
			logger.LogError("REPLACE INTO guildmgr error!!! server_id:%d guild_id:%d actor_id:%d %v", srvId, guildId, playerId, err)
		}
	}
}

func saveGuild(guildInfo *pb3.GuildInfo) {
	basicInfo := guildInfo.GetBasicInfo()
	guildId := basicInfo.GetId()
	srvId := GetSrvIdFromGuildId(guildId)
	if _, err := db.OrmEngine.Exec("call saveGuildData(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", guildId, srvId, basicInfo.GetName(), basicInfo.GetLevel(), basicInfo.GetLeaderId(), basicInfo.GetLeaderName(),
		basicInfo.GetLeaderBoundaryLv(), basicInfo.GetApplyLevel(), basicInfo.GetApplyPower(), basicInfo.GetMode(), basicInfo.GetNotice(), basicInfo.GetMoney(), pb3.CompressByte(guildInfo.Binary), basicInfo.CreateTime); nil != err {
		logger.LogError("save guild error!!! guildId:%d %v", guildId, err)
	}
}

func LoadGuildDataFromMysql() ([]*pb3.GuildInfo, error) {
	var guilds []mysql.Guilds
	if err := db.OrmEngine.SQL("call loadGuildData()").Find(&guilds); nil != err {
		logger.LogError("%s", err)
		return nil, err
	}

	if len(guilds) <= 0 {
		return nil, nil
	}

	guildList := make([]*pb3.GuildInfo, 0)
	for _, guild := range guilds {
		tmpGuild := new(pb3.GuildInfo)
		tmpGuildBasic := &pb3.GuildBasicInfo{
			Id:               guild.GuildId,
			Name:             guild.Name,
			Level:            guild.Level,
			LeaderId:         guild.LeaderId,
			LeaderName:       guild.LeaderName,
			LeaderBoundaryLv: guild.LeaderBoundaryLv,
			ApplyLevel:       guild.ApplyLevel,
			ApplyPower:       guild.ApplyPower,
			Notice:           guild.Notice,
			Mode:             guild.ApprovalMode,
			Money:            guild.Money,
			CreateTime:       guild.CreatTime,
		}
		tmpGuild.BasicInfo = tmpGuildBasic

		binary := new(pb3.GuildBinaryInfo)
		if err := pb3.Unmarshal(pb3.UnCompress(guild.BinaryData), binary); nil == err {
			tmpGuild.Binary = binary
			tmpGuildBasic.Banner = binary.Banner
		}

		if guildMember, err := loadGuildMemberDataFromMysql(guild.GuildId); nil != err {
			logger.LogError("loadGuildMemberDataFromMysql failed %v", err)
		} else {
			tmpGuild.Members = guildMember
		}

		if itemVec, err := loadNewGuildItems(guild.GuildId); nil != err {
			logger.LogError("loadGuildItems failed %v", err)
		} else {
			tmpGuild.DepotItems = itemVec
		}

		guildList = append(guildList, tmpGuild)
	}
	return guildList, nil
}

func GetSrvIdFromGuildId(guildId uint64) uint32 {
	return uint32((guildId >> 20) & 0xFFFF)
}

func loadGuildMemberDataFromMysql(guildId uint64) (map[uint64]*pb3.GuildMemberInfo, error) {
	var guildMember []mysql.GuildMember
	srvId := GetSrvIdFromGuildId(guildId)
	if err := db.OrmEngine.SQL("call loadGuildMemberData(?, ?)", srvId, guildId).Find(&guildMember); nil != err {
		logger.LogError("%s", err)
		return nil, err
	}

	if len(guildMember) <= 0 {
		return nil, nil
	}

	guildMemberList := make(map[uint64]*pb3.GuildMemberInfo, 0)

	for _, member := range guildMember {
		tmpMember := &pb3.GuildMemberInfo{
			JoinTime: member.JoinTime,
			GuildId:  member.GuildId,
			Position: member.Position,
			Donate:   member.Donate,
		}

		guildMemberList[member.ActorId] = tmpMember
	}

	return guildMemberList, nil
}

func deleteGuild(args ...interface{}) {
	if !gcommon.CheckArgsCount("deleteGuild", 1, len(args)) {
		return
	}

	guildId := args[0].(uint64)
	srvId := GetSrvIdFromGuildId(guildId)
	if _, err := db.OrmEngine.Exec("call deleteGuild(?)", guildId); nil != err {
		logger.LogError("Delete Guild error!!! server_id:%d guild_id:%d err:%v", srvId, guildId, err)
	}
}

func deleteGuildMember(args ...interface{}) {
	if !gcommon.CheckArgsCount("deleteGuildMember", 2, len(args)) {
		return
	}

	guildId := args[0].(uint64)
	actorId := args[1].(uint64)
	srvId := GetSrvIdFromGuildId(guildId)

	if _, err := db.OrmEngine.Exec("call deleteGuildMember(?,?)", guildId, actorId); nil != err {
		logger.LogError("Delete GuilldMember error!!! server_id:%d guild_id:%d actor_id:%d %v", srvId, guildId, actorId, err)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveGuildData, saveGuildData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeleteGuild, deleteGuild)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeleteGuildMember, deleteGuildMember)
	})
}
