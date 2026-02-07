package guildmgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/db"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/dbworker"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
)

var (
	guildIdSeries uint32 // 工会序列号
	//prefixUse     = make(map[uint32]bool)     // 已创建工会名
	allGuildName = make(map[string]struct{}) // 工会名字
	GuildMap     = make(map[uint64]*Guild)   // 工会列表
	SaveFlagMap  = make(map[uint64]struct{}) // 保存标志
)

// 生成仙盟id主键
func GenerateGuildId() uint64 {
	serverId := engine.GetServerId()
	pfId := engine.GetPfId()
	//16位平台id+16位服务器id+15位序列号
	preId := pfId<<16 | serverId
	guid := uint64(preId)<<20 | uint64(guildIdSeries)

	guildIdSeries += 1

	return guid
}

func loadGuildSeries(srvId uint32) error {
	var seriesArr []*struct{ Series uint32 }
	if err := db.OrmEngine.SQL("call loadGuildSeries(?)", srvId).Find(&seriesArr); nil != err {
		logger.LogError("loadGuildMgrData error : %s", err)
		return err
	}

	series := seriesArr[0].Series
	guildIdSeries = series + 1
	return nil
}

func loadAllGuilds() error {
	guildList, err := dbworker.LoadGuildDataFromMysql()
	if err != nil {
		return err
	}
	for _, guildData := range guildList {
		guildBasic := guildData.GetBasicInfo()
		guildId := guildBasic.GetId()

		if guild, ok := GuildMap[guildId]; ok || nil != guild {
			logger.LogError("LoadAllGuilds reload Error !!! guildId:%d", guildId)
			continue
		}

		guild := &Guild{GuildInfo: guildData}
		if len(guild.Members) == 0 {
			logger.LogError("guild(%d) member is zero", guild)
			continue
		}
		GuildMap[guildId] = guild

		guild.Init()

		allGuildName[guild.GetName()] = struct{}{}
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypeGuild)
	rankLine := rank.GetList(1, rank.GetRankCount())
	for _, line := range rankLine {
		if nil == GuildMap[line.GetId()] {
			toUpGuildRank(line.GetId(), 0, 0)
		}
	}
	return nil
}

func Load(srvId uint32) error {
	var err error
	if err = loadGuildSeries(srvId); nil != err {
		return err
	}
	if err = loadAllGuilds(); nil != err {
		return err
	}

	return err
}

func CreateGuild(actor iface.IPlayer, guildName string, banner *pb3.GuildBanner) *Guild {
	guild := NewGuild(guildName, banner)
	if nil == guild {
		return nil
	}
	guildId := guild.BasicInfo.GetId()

	allGuildName[guildName] = struct{}{}

	GuildMap[guildId] = guild

	if nil != actor {
		guild.BasicInfo.LeaderId = actor.GetId()
	}
	GuildAddMember(guild, manager.GetSimplyData(actor.GetId()), custom_id.GuildPos_Leader)
	toUpGuildRank(guild.GetId(), guild.GetLevel(), guild.GetPower())
	if nil != actor {
		engine.BroadcastTipMsgById(tipmsgid.TpGuildCreateInviteMsg, actor.GetId(), guild.GetId(), guild.GetLevel(), guildName)
	}
	guild.Save()
	return guild
}

func DelGuild(guildId uint64) {
	guild := GetGuildById(guildId)
	if guild == nil {
		return
	}
	guild.Destroy()
	delete(SaveFlagMap, guildId)
	delete(GuildMap, guildId)
	toUpGuildRank(guild.GetId(), 0, 0)
	// 数据库删除
	gshare.SendDBMsg(custom_id.GMsgDeleteGuild, guildId)
	delete(allGuildName, guild.GetName())
	event.TriggerSysEvent(custom_id.SeGuildDestroy, guildId)
}

func CheckApplyLv(lv uint32) bool {
	applyLv := jsondata.GlobalUint("guildApplyLevelLimit")
	if lv < applyLv {
		return false
	}
	return true
}

func CheckGuildName(name string) bool {
	if _, ok := allGuildName[name]; ok {
		return false
	}
	return true
}

func GuildAddMember(guild *Guild, playData *pb3.SimplyPlayerData, pos uint32) {
	if nil == guild || nil == playData {
		return
	}
	member := &pb3.GuildMemberInfo{PlayerInfo: playData, JoinTime: time_util.NowSec()}
	guild.addMember(member, pos)
}

func GetGuildById(guildId uint64) *Guild {
	if guild, ok := GuildMap[guildId]; ok {
		return guild
	}

	return nil
}

func GetExitTimeOff(actorId uint64) uint32 {
	return manager.GetOnlineAttr(actorId, custom_id.OnlineAttrGuildExitTime)
}

func SetExitTimeOff(actorId uint64, exitTime uint32) {
	if exitTime == 0 {
		manager.DelOnlineAttr(actorId, custom_id.OnlineAttrGuildExitTime)
	} else {
		manager.SetOnlineAttr(actorId, custom_id.OnlineAttrGuildExitTime, exitTime, false)
	}
}

func Save() {
	if len(SaveFlagMap) <= 0 {
		return
	}

	for id := range SaveFlagMap {
		guild, ok := GuildMap[id]
		if !ok || nil == guild {
			continue
		}
		guild.Save()
		delete(SaveFlagMap, id)
	}
}

func Run5Minutes() {
	for _, g := range GuildMap {
		g.Run5Minutes()
	}
}

// 设置行会保存标志
func SetSaveFlag(id uint64) {
	SaveFlagMap[id] = struct{}{}
}

func HasGuild(actorId uint64) bool {
	for _, g := range GuildMap {
		if nil != g.GetMember(actorId) {
			return true
		}
	}
	return false
}

func onGuildNameChange(player iface.IPlayer, args ...interface{}) {
	if guild := GetGuildById(player.GetGuildId()); nil != guild {
		m := guild.GetMember(player.GetId())
		player.CallActorFunc(actorfuncid.GuildShowNameChange, &pb3.UpdateGuildShowName{Name: guild.GetName(), Pos: m.GetPosition(), Id: guild.GetId()})
	} else {
		player.CallActorFunc(actorfuncid.GuildShowNameChange, &pb3.UpdateGuildShowName{})
	}
}

func SendSpInviteInfo(player iface.IPlayer) (trGuildId uint64) {
	if player.GetGuildId() > 0 {
		return
	}

	conf := jsondata.GetGuildConf()

	if player.GetLevel() < conf.GMInviteLevel || len(conf.GMInviteTimeLimit) < 2 {
		return
	}

	serverDay := gshare.GetOpenServerDay()
	if serverDay < conf.GMInviteTimeLimit[0] || serverDay > conf.GMInviteTimeLimit[1] {
		return
	}

	var mxNumber uint32

	for guildId, guild := range GuildMap {
		if guild.IsSetProp(custom_id.GuildPropChangeSet) {
			continue
		}
		if guild.IsFull() {
			continue
		}
		gNumber := guild.GetMemberCount()
		if gNumber > mxNumber {
			mxNumber = gNumber
			trGuildId = guildId
		}
	}

	if trGuildId <= 0 {
		return
	}

	trGuild := GuildMap[trGuildId]

	engine.SendPlayerMessage(player.GetId(), gshare.OfflineInviteGuild, &pb3.OfflineGuildInvite{Guild: trGuildId})

	player.SendProto3(29, 135, &pb3.S2C_29_135{
		InviteName: trGuild.GetBasicInfo().GetLeaderName(),
		GuildName:  trGuild.GetName(),
		GuildId:    trGuildId,
	})
	return
}

func handleSeClearActorChat(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	st, ok := args[0].(*pb3.CommonSt)
	if !ok {
		return
	}
	for _, guild := range GuildMap {
		var newList []*pb3.S2C_5_1
		for _, result := range guild.ChatCache {
			if result.SenderData != nil && result.SenderData.PlayerId == st.U64Param {
				continue
			}
			newList = append(newList, result)
		}
		guild.ChatCache = newList
	}
}

func init() {
	event.RegActorEvent(custom_id.AeJoinGuild, onGuildNameChange)
	event.RegActorEvent(custom_id.AeLeaveGuild, onGuildNameChange)

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		for _, guild := range GuildMap {
			guild.OnNewDay()
		}
	})
	event.RegSysEvent(custom_id.SeClearActorChat, handleSeClearActorChat)
}
