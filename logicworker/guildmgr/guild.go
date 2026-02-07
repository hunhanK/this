package guildmgr

import (
	"fmt"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/functional"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type Guild struct {
	*pb3.GuildInfo
	*miscitem.Container
	ChatCache []*pb3.S2C_5_1
}

func GetLevelLimit() (uint32, uint32) {
	if conf := jsondata.GetGuildConf(); nil != conf && len(conf.AutoApproveLevelLimit) >= 2 {
		return conf.AutoApproveLevelLimit[0], conf.AutoApproveLevelLimit[1]
	}
	return 0, 0
}

func GetPowerLimit() (int64, int64) {
	if conf := jsondata.GetGuildConf(); nil != conf && len(conf.AutoApprovePowerLimit) >= 2 {
		return conf.AutoApprovePowerLimit[0], conf.AutoApprovePowerLimit[1]
	}
	return 0, 0
}

func NewGuild(name string, banner *pb3.GuildBanner) *Guild {
	guild := new(Guild)
	guild.ChatCache = make([]*pb3.S2C_5_1, 0, 20)
	info := new(pb3.GuildInfo)
	levelLimit, _ := GetLevelLimit()
	powerLimit, _ := GetPowerLimit()
	basic := &pb3.GuildBasicInfo{
		Id:         GenerateGuildId(),
		Name:       name,
		Level:      1,
		Notice:     jsondata.GlobalString("defaultAnnouncement"),
		Mode:       custom_id.GuildApplyMode_Cond, //默认为自动审批
		ApplyLevel: levelLimit,
		ApplyPower: powerLimit,
		Banner:     banner,
		CreateTime: time_util.NowSec(),
	}
	info.BasicInfo = basic
	info.Binary = new(pb3.GuildBinaryInfo)
	info.Binary.Banner = banner
	guild.GuildInfo = info
	guild.Init()
	return guild
}

func (guild *Guild) Init() {
	guild.initDepot()

	guild.BasicInfo.Total = uint32(len(guild.Members))
	guild.BasicInfo.Power = 0
	var power uint64
	for actorId, member := range guild.Members {
		if playerData := manager.GetSimplyData(actorId); nil != playerData {
			member.PlayerInfo = playerData
			if nil != manager.GetPlayerPtrById(actorId) {
				member.IsOnline = true
			}
			power += member.PlayerInfo.Power
		} else {
			member.PlayerInfo = &pb3.SimplyPlayerData{Id: actorId}
			gshare.SendDBMsg(custom_id.GMsgGetPlayerBasicInfo, guild.GetId(), actorId)
		}
	}
	guild.AddPower(power)
	guild.BasicInfo.Banner = guild.Binary.Banner
	if nil == guild.Binary.ApplyIds {
		guild.Binary.ApplyIds = make(map[uint64]uint32)
	}
	if nil == guild.Members {
		guild.Members = make(map[uint64]*pb3.GuildMemberInfo)
	}
}

func (guild *Guild) Save() {
	sData, err := pb3.Marshal(guild.GuildInfo)
	if nil != err {
		logger.LogError("guildId data to string error!!! guildId=%d, err=%v", guild.GetId(), err)
		return
	}
	gshare.SendDBMsg(custom_id.GMsgSaveGuildData, sData)
}

func (guild *Guild) OnNewDay() {
	nowSec := time_util.NowSec()
	cleanTime := jsondata.GlobalUint("guildApplyCleanTime")
	for applyId, createTime := range guild.GuildInfo.Binary.ApplyIds {
		if createTime+cleanTime < nowSec {
			delete(guild.GuildInfo.Binary.ApplyIds, applyId)
		}
	}

	// 清理仙盟宴会的活动数据
	guild.GuildInfo.Party = nil

	guild.AutoTransferLeader()
	guild.SetSaveFlag()
	guild.Save()
}

func (guild *Guild) addMember(member *pb3.GuildMemberInfo, pos uint32) {
	if nil == member || nil == member.GetPlayerInfo() {
		return
	}
	memberInfo := member.GetPlayerInfo()
	if nil != guild.GetMember(memberInfo.GetId()) {
		return
	}
	playerId := memberInfo.GetId()
	guild.Members[playerId] = member

	basic := guild.GetBasicInfo()
	basic.Total = uint32(len(guild.Members))

	if actor := manager.GetPlayerPtrById(playerId); nil != actor {
		member.IsOnline = true
		actor.SendProto3(29, 10, &pb3.S2C_29_10{BasicInfo: guild.BasicInfo})
	}

	guild.AddPower(memberInfo.GetPower())
	// 设置职位
	guild.SetGuildPos(member, pos)

	guild.BroGuildBasicInfo()

	guild.onJoinGuild(member)

	event.TriggerSysEvent(custom_id.SeGuildAddMember, guild.GetId(), playerId)
}

func (guild *Guild) GetId() uint64 {
	return guild.BasicInfo.GetId()
}

// 获取会长
func (guild *Guild) GetLeader() *pb3.GuildMemberInfo {
	return guild.GetMember(guild.BasicInfo.GetLeaderId())
}

// 获取前缀名id
func (guild *Guild) GetPrefix() uint32 {
	return utils.Low32(guild.GetBinary().GetNameSplice())
}

func (guild *Guild) GetLeaderId() uint64 {
	return guild.BasicInfo.GetLeaderId()
}

func (guild *Guild) GetLevel() uint32 {
	return guild.BasicInfo.GetLevel()
}

func (guild *Guild) SetLevel(level uint32) {
	guild.BasicInfo.Level = level
	toUpGuildRank(guild.GetId(), guild.GetLevel(), guild.GetPower())
}

func (guild *Guild) UpdateMember(member *pb3.GuildMemberInfo) {
	if nil == member {
		return
	}
	guild.BroadcastProto(29, 7, &pb3.S2C_29_7{Member: member})
}

func (guild *Guild) GetPower() uint64 {
	return guild.GetBasicInfo().GetPower()
}

func (guild *Guild) OnLogin(actor iface.IPlayer) {
	member := guild.GetMember(actor.GetId())
	if nil == member {
		return
	}
	actor.SetExtraAttr(attrdef.GuildPos, int64(member.GetPosition()))
	member.IsOnline = true
	guild.sendGuildInfo(actor)
	guild.OnMemberDataBaseChange(manager.GetSimplyData(actor.GetId()))
}

func (guild *Guild) SendGuildPlayerInfo(actor iface.IPlayer) {
	if guildData := actor.GetBinaryData().GuildData; nil != guildData {
		actor.SendProto3(29, 110, &pb3.S2C_29_110{Donates: guildData.DonateList})
	}
}

func (guild *Guild) IsRobotFull() bool {
	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return true
	}

	var robotCount uint32
	for robotId := range guild.Members {
		if engine.IsRobot(robotId) {
			robotCount++
			//每个行会机器人数量上限为10个
			if robotCount >= conf.MaxRobotCount {
				return true
			}
			continue
		}

	}
	return false
}

func (guild *Guild) chat(memberName string) {
	leader := guild.GetLeader()
	if leader == nil {
		return
	}
	playerInfo := leader.GetPlayerInfo()
	if playerInfo == nil {
		return
	}
	chatPlayerData := &pb3.ChatPlayerData{
		PlayerId:        playerInfo.GetId(),
		Name:            playerInfo.GetName(),
		Sex:             playerInfo.GetSex(),
		Career:          playerInfo.GetJob(),
		PfId:            engine.GetPfId(),
		ServerId:        engine.GetServerId(),
		VipLv:           playerInfo.GetVipLv(),
		HeadFrame:       playerInfo.GetHeadFrame(),
		BubbleFrame:     playerInfo.GetBubbleFrame(),
		Head:            playerInfo.GetHead(),
		Level:           playerInfo.GetLv(),
		SmallCrossCamp:  engine.GetSmallCrossCamp(),
		ChatTitles:      jsondata.PackChatTitle(playerInfo.GetId()),
		FlyCamp:         playerInfo.FlyCamp,
		MediumCrossCamp: engine.GetMediumCrossCamp(),
		JinjieLevel:     playerInfo.Circle,
	}
	ft := jsondata.GlobalString("welcomeMessageByGuild")
	if len(ft) == 0 {
		ft = "欢迎 %s 加入仙盟，齐心协力，共展宏图！"
	}
	rsp := &pb3.S2C_5_1{
		Channel:    chatdef.CIGuild,
		SenderData: chatPlayerData,
		Msg:        fmt.Sprintf(ft, memberName),
	}
	guild.BroadcastProto(5, 1, rsp)
	// 行会聊天缓存
	if len(guild.ChatCache) >= 20 {
		guild.ChatCache = guild.ChatCache[1:]
	}
	guild.ChatCache = append(guild.ChatCache, rsp)
}

func (guild *Guild) onJoinGuild(member *pb3.GuildMemberInfo) {
	basic := guild.BasicInfo
	if nil == basic {
		return
	}
	if nil == member || nil == member.GetPlayerInfo() {
		return
	}
	memberInfo := member.GetPlayerInfo()
	actorId := memberInfo.GetId()

	if member.GetPosition() == custom_id.GuildPos_Leader {
		guild.AddEvent(custom_id.GuildEvent_CreateGuild, memberInfo.GetName())
	} else {
		guild.AddEvent(custom_id.GuildEvent_JoinGuild, memberInfo.GetName())
		guild.chat(memberInfo.Name)
	}

	if engine.IsRobot(actorId) {
		if robot := engine.GetRobotById(actorId); nil != robot {
			robot.SetGuildId(basic.GetId())
		}
		if guild.IsRobotFull() {
			for applyId := range guild.GetBinary().ApplyIds {
				if engine.IsRobot(applyId) {
					guild.RemoveApply(applyId)
				}
			}
		}
		return
	}

	if actor := manager.GetPlayerPtrById(actorId); nil != actor {
		actor.SetGuildId(basic.GetId())
		actor.GetBinaryData().GuildData.GuildInviteList = nil
		guild.sendGuildInfo(actor)
		guild.CheckGuildTransfer(actor)
	}

	if engine.IsRobot(guild.GetLeaderId()) {
		if guildRobotConf := jsondata.GetMainCityRobotGuild(); nil != guildRobotConf {
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  common.Mail_GuildRobotTransferLeaader,
				Content: &mailargs.CommonMailArgs{Str1: guild.GetName(), Digit1: guildRobotConf.TransferFightVal},
			})
		}
	}

	engine.SendPlayerMessage(actorId, gshare.OfflineJoinGuild, &pb3.CommonSt{
		U64Param: guild.GetId(),
		U32Param: time_util.NowSec(),
	})
}

func (guild *Guild) sendGuildInfo(actor iface.IPlayer) {
	actor.SendProto3(29, 1, &pb3.S2C_29_1{
		BasicInfo: guild.BasicInfo,
		Binary:    guild.Binary,
		GuildRule: GetPfRule(),
	})
	actor.SendProto3(29, 120, &pb3.S2C_29_120{Members: functional.MapToSlice(guild.Members)})
	actor.SendProto3(29, 55, &pb3.S2C_29_55{DepotItems: guild.DepotItems})
	guild.SendGuildPlayerInfo(actor)
}

func (guild *Guild) GetName() string {
	return guild.GetBasicInfo().GetName()
}

func (guild *Guild) GetNameSplice() uint64 {
	return guild.GetBinary().GetNameSplice()
}

func (guild *Guild) AddEvent(eventId uint32, params ...interface{}) {
	info := &pb3.GuildEvent{
		EventId: eventId,
		Params:  make([]string, 0, len(params)),
		Time:    time_util.NowSec(),
	}
	for _, param := range params {
		info.Params = append(info.Params, fmt.Sprintf("%v", param))
	}

	binary := guild.GetBinary()
	eventConf := jsondata.GetGuildLogConf(eventId)
	if eventConf.LogType == custom_id.GuildEvent_TypeDepot {
		depotLimit := jsondata.GlobalUint("guildStorageLogNum")
		if uint32(len(binary.DepotRecords)) >= depotLimit {
			binary.DepotRecords = binary.DepotRecords[1:]
		}
		binary.DepotRecords = append(binary.DepotRecords, info)
	} else if eventConf.LogType == custom_id.GuildEvent_TypeRecord {
		limit := jsondata.GlobalUint("guildLogMaxNum")
		if uint32(len(binary.Records)) >= limit {
			binary.Records = binary.Records[1:]
		}
		binary.Records = append(binary.Records, info)
	}

	guild.SetSaveFlag()
	guild.BroadcastProto(29, 102, &pb3.S2C_29_102{Record: info})
}

func (guild *Guild) SetSaveFlag() {
	SetSaveFlag(guild.GetId())
}

func (guild *Guild) Run5Minutes() {
	checkRobotExit := func() {
		conf := jsondata.GetMainCityRobotGuild()
		if nil == conf {
			return
		}

		var (
			robots   []uint64
			robotNum uint32
		)

		leaderId := guild.GetLeaderId()
		for m := range guild.Members {
			memberId := m
			if !engine.IsRobot(memberId) {
				continue
			}
			robotNum++
			if memberId != leaderId {
				robots = append(robots, memberId)
			}
		}

		playerNum := uint32(len(guild.Members)) - robotNum

		mxLv := uint32(len(conf.TotalCountCond))
		index := utils.Ternary(guild.GetLevel() >= mxLv, mxLv-1, guild.GetLevel()).(uint32)
		if conf.TotalCountCond[index] <= playerNum {
			for _, robotId := range robots {
				guild.RemoveMember(robotId)
			}
			if len(robots) > 0 {
				guild.SetSaveFlag()
			}
		}
	}
	checkRobotExit()
}

func (guild *Guild) GetMember(actorId uint64) *pb3.GuildMemberInfo {
	if members, ok := guild.Members[actorId]; ok {
		return members
	}
	return nil
}

func (guild *Guild) SetGuildLeader(member *pb3.GuildMemberInfo) {
	basic := guild.GetBasicInfo()
	id := member.GetPlayerInfo().GetId()
	basic.LeaderId = member.GetPlayerInfo().GetId()
	basic.LeaderName = member.GetPlayerInfo().GetName()

	blv := manager.GetExtraAttr(id, attrdef.Circle)
	basic.LeaderBoundaryLv = uint32(blv)
	SetSaveFlag(guild.GetId())
}

func (guild *Guild) SetGuildPos(member *pb3.GuildMemberInfo, pos uint32) bool {
	if engine.IsRobot(member.GetPlayerInfo().GetId()) && (pos != custom_id.GuildPos_Common && pos != custom_id.GuildPos_Leader) {
		return false
	}
	member.Position = pos

	if custom_id.GuildPos_Leader == pos {
		guild.SetGuildLeader(member)
	}

	guild.BroadcastProto(29, 7, &pb3.S2C_29_7{
		Member: member,
	})

	memId := member.GetPlayerInfo().GetId()
	if p := manager.GetPlayerPtrById(memId); nil != p {
		p.SetExtraAttr(attrdef.GuildPos, int64(member.Position))
		p.CallActorFunc(actorfuncid.GuildShowNameChange, &pb3.UpdateGuildShowName{Name: guild.GetName(), Pos: member.GetPosition()})
	}

	if robot := engine.GetRobotById(memId); nil != robot {
		robot.SetAttr(attrdef.GuildPos, int64(member.GetPosition()))
	}

	SetSaveFlag(guild.GetId())

	return true
}

func (guild *Guild) BroGuildBasicInfo() {
	guild.BroadcastProto(29, 10, &pb3.S2C_29_10{BasicInfo: guild.BasicInfo})
}

// BroadcastProto 向盟内所有在线成员推送消息
func (guild *Guild) BroadcastProto(protoH, protoL uint16, msg pb3.Message) {
	logger.LogInfo("BroadcastProto to guild, %d_%d, guild member num:%d", protoH, protoL, len(guild.Members))
	for _, v := range guild.Members {
		if nil != v.GetPlayerInfo() {
			if actor := manager.GetPlayerPtrById(v.GetPlayerInfo().GetId()); nil != actor {
				actor.SendProto3(protoH, protoL, msg)
			}
		}
	}
}

func (guild *Guild) OnUpdateGuildName(newName string, newNameNo uint64) {
	delete(allGuildName, guild.GetName())
	guild.GetBasicInfo().Name = newName
	guild.GetBinary().NameSplice = newNameNo
	SetSaveFlag(guild.GetId())
	allGuildName[newName] = struct{}{}
	rsp := &pb3.S2C_29_105{GuildId: guild.GetId(), GuildName: guild.GetName(), NameSplice: guild.GetNameSplice()}
	for _, v := range guild.Members {
		if nil != v.GetPlayerInfo() {
			if actor := manager.GetPlayerPtrById(v.GetPlayerInfo().GetId()); nil != actor {
				actor.CallActorFunc(actorfuncid.GuildShowNameChange, &pb3.UpdateGuildShowName{Name: guild.GetName(), Pos: v.GetPosition()})
				actor.SendProto3(29, 105, rsp)
			}
		}
	}
}

// BroadcastProto 向盟内所有在线成员拥有某权限的人推送消息
func (guild *Guild) BroadcastProtoByPermission(protoH, protoL uint16, msg pb3.Message, permission int) {
	for _, v := range guild.Members {
		if nil != v.GetPlayerInfo() && guild.CheckPermission(v, permission) {
			if actor := manager.GetPlayerPtrById(v.GetPlayerInfo().GetId()); nil != actor {
				actor.SendProto3(protoH, protoL, msg)
			}
		}
	}
}

func (guild *Guild) IsApply(actorId uint64) bool {
	if _, ok := guild.GetBinary().ApplyIds[actorId]; ok {
		return true
	}
	return false
}

// 行会人数
func (guild *Guild) GetMemberCount() uint32 {
	return uint32(len(guild.Members))
}

func (guild *Guild) IsFull() bool {
	conf := jsondata.GetGuildConf()
	if nil == conf || nil == conf.Upgrade || nil == conf.Upgrade[guild.GetLevel()] {
		return false
	}
	full, existRule := GuildRuleCheck(guild.GetLevel(), guild.GetMemberCount())
	if existRule {
		return full
	}
	return conf.Upgrade[guild.GetLevel()].MaxMemberCount <= guild.GetMemberCount()
}

func (guild *Guild) checkApplyCond(actor iface.IPlayer) bool {
	if nil == actor {
		return false
	}

	minCombat := guild.BasicInfo.GetApplyPower()
	minLevel := guild.BasicInfo.GetApplyLevel()

	if actor.GetLevel() < minLevel {
		return false
	}
	if actor.GetExtraAttr(attrdef.FightValue) < minCombat {
		return false
	}
	return true
}

// ApplyJoin 申请加入
func (guild *Guild) ApplyJoin(actor iface.IPlayer) {
	if guild.IsFull() {
		return
	}

	if actor.GetGuildId() > 0 {
		return
	}
	mode := guild.GetBasicInfo().GetMode()
	switch mode {
	case custom_id.GuildApplyMode_Verify:
		guild.addToApply(actor)
	case custom_id.GuildApplyMode_Ban:
		return
	case custom_id.GuildApplyMode_Cond:
		if guild.checkApplyCond(actor) {
			GuildAddMember(guild, manager.GetSimplyData(actor.GetId()), custom_id.GuildPos_Common)
			logworker.LogPlayerBehavior(actor, pb3.LogId_LogJoinGuild, &pb3.LogPlayerCounter{
				NumArgs: guild.GetId(),
			})
		}
	}
	return
}

func (guild *Guild) getApplyInfo(actorId uint64) *pb3.GuildApplyInfo {
	if createTime, ok := guild.Binary.ApplyIds[actorId]; ok {
		if engine.IsRobot(actorId) {
			data := manager.GetRobotSimplyData(actorId)
			return &pb3.GuildApplyInfo{
				PlayerInfo: data,
				ApplyTime:  createTime,
			}
		}
		if data := manager.GetSimplyData(actorId); nil != data {
			return &pb3.GuildApplyInfo{
				PlayerInfo: data,
				ApplyTime:  createTime,
			}
		}
	}
	return nil
}

func (guild *Guild) addToApply(actor iface.IPlayer) {
	if nil == actor {
		return
	}

	actorId := actor.GetId()
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
		PlayerInfo: manager.GetSimplyData(actorId),
		ApplyTime:  nowSec,
	}

	// 通知加入列表
	guild.BroadcastProto(29, 3, &pb3.S2C_29_3{
		Member: member,
	})

	SetSaveFlag(guild.GetId())
}

// 删除玩家申请
func (guild *Guild) RemoveApply(actorId uint64) bool {
	binary := guild.GetBinary()

	if guild.IsApply(actorId) {
		delete(binary.ApplyIds, actorId)
		basic := guild.GetBasicInfo()
		basic.Total = uint32(len(guild.Members))
		SetSaveFlag(guild.GetId())
		guild.BroadcastProtoByPermission(29, 4, &pb3.S2C_29_4{
			GuildId: guild.GetId(),
			ActorId: actorId,
		}, custom_id.GuildPermission_CanManageRequest)
		return true
	}

	return true
}

func IsInCoolTime(actorId uint64) bool {
	var cTime uint32
	if player := manager.GetPlayerPtrById(actorId); nil != player {
		cTime = uint32(player.GetExtraAttr(attrdef.GuildCoolTime))
	} else {
		cTime = uint32(manager.GetExtraAttr(actorId, attrdef.GuildCoolTime))
		eTime := GetExitTimeOff(actorId)
		cd := jsondata.GlobalUint("guildExitCd")
		if eTime+cd > cTime {
			cTime = eTime + cd
		}
	}
	return cTime >= time_util.NowSec()
}

// 审批全部
func (guild *Guild) ReplyApply(actor iface.IPlayer, applyId uint64, op uint32) {
	isAccept := op == custom_id.GuildApplyReply_Accept
	if nil == actor {
		return
	}
	member := guild.GetMember(actor.GetId())
	if nil == member {
		return
	}
	apply := guild.getApplyInfo(applyId)
	if nil == apply {
		return
	}
	lv := apply.GetPlayerInfo().GetLv()
	if !CheckApplyLv(lv) {
		actor.SendTipMsg(tipmsgid.TpPlayerNotUnlockGuild)
		guild.RemoveApply(applyId)
		return
	}
	if isAccept {
		if guild.IsFull() {
			return
		}
		if !engine.IsRobot(applyId) && IsInCoolTime(applyId) {
			actor.SendTipMsg(tipmsgid.TpJoinGuildCoolCd)
			return
		}
		if HasGuild(applyId) {
			guild.RemoveApply(applyId)
			actor.SendTipMsg(tipmsgid.TpGuildHasJoinOther)
			return
		}
		if engine.IsRobot(applyId) {
			if robot := engine.GetRobotById(applyId); nil != robot {
				GuildAddRobotMember(guild, robot, custom_id.GuildPos_Common)
			}
		} else {
			GuildAddMember(guild, apply.PlayerInfo, custom_id.GuildPos_Common)
			logworker.LogPlayerBehavior(actor, pb3.LogId_LogPassApply, &pb3.LogPlayerCounter{
				NumArgs: guild.GetId(),
				StrArgs: utils.Itoa(applyId),
			})
		}
	}

	guild.RemoveApply(applyId)
	actor.SendProto3(29, 6, &pb3.S2C_29_6{
		ActorId: applyId,
		Op:      op,
	})
}

func (guild *Guild) RemoveMember(actorId uint64) bool {
	member := guild.GetMember(actorId)
	if nil != member {
		delete(guild.Members, actorId)
		rsp := &pb3.S2C_29_8{ActorId: actorId}
		guild.BroadcastProto(29, 8, rsp)

		if actor := manager.GetPlayerPtrById(actorId); nil != actor {
			actor.SetGuildId(0)
			actor.SendProto3(29, 8, rsp)
		}

		gshare.SendDBMsg(custom_id.GMsgDeleteGuildMember, guild.GetId(), actorId)

		basic := guild.GetBasicInfo()
		total := basic.GetTotal() - 1
		if basic.GetTotal() == 0 {
			total = 0
		}
		basic.Total = total

		guild.ReducePower(member.GetPlayerInfo().GetPower())

		SetSaveFlag(guild.GetId())

		guild.BroGuildBasicInfo()

		event.TriggerSysEvent(custom_id.SeGuildRemoveMember, guild.GetId(), actorId)
		return true
	}
	return false
}

func (guild *Guild) SetMode(mode uint32, level uint32, power int64) {
	guild.BasicInfo.Mode = mode
	guild.BasicInfo.ApplyLevel = level
	guild.BasicInfo.ApplyPower = power

	SetSaveFlag(guild.GetId())

	guild.BroadcastProtoByPermission(29, 100, &pb3.S2C_29_100{
		Mode:  mode,
		Level: level,
		Power: power,
	}, custom_id.GuildPermission_CanAutoAgree)
}

func (guild *Guild) SetProp(prop uint32, isSet bool) {
	if isSet {
		guild.Binary.Prop = utils.SetBit64(guild.Binary.Prop, prop)
	} else {
		guild.Binary.Prop = utils.ClearBit64(guild.Binary.Prop, prop)
	}
}

func (guild *Guild) IsSetProp(prop uint32) bool {
	return utils.IsSetBit64(guild.Binary.Prop, prop)
}

func (guild *Guild) CheckPermission(member *pb3.GuildMemberInfo, permission int, args ...uint32) bool {
	if nil == member {
		return false
	}
	posConf := jsondata.GetGuildPositionConf(member.GetPosition())
	if nil == posConf {
		return false
	}
	permissionConf, ok := posConf.Permission[uint32(permission)]
	if !ok {
		return false
	}
	flag := false
	switch permission {
	case custom_id.GuildPermission_SetPosition, custom_id.GuildPermission_KickMember:
		if len(args) > 0 {
			for _, pos := range permissionConf.Cond1 {
				if pos == args[0] {
					flag = true
					break
				}
			}
		}
	default: //不需要其他条件的
		flag = true
	}
	return flag
}

func (guild *Guild) IsPositionFull(pos uint32) bool {
	conf := jsondata.GetGuildConf()
	if nil == conf || nil == conf.Upgrade || nil == conf.Upgrade[guild.GetLevel()] {
		return false
	}
	upConf := conf.Upgrade[guild.GetLevel()]
	var cnt uint32
	for _, member := range guild.Members {
		if member.GetPosition() == pos {
			cnt++
		}
	}
	switch pos {
	case custom_id.GuildPos_Elite:
		return upConf.MaxEliteCount <= cnt
	case custom_id.GuildPos_Manager:
		return upConf.MaxHallMasterCount <= cnt
	case custom_id.GuildPos_Elder:
		return upConf.MaxElderCount <= cnt
	case custom_id.GuildPos_DeputyLeader:
		return upConf.MaxVicePresidentCount <= cnt
	case custom_id.GuildPos_Leader:
		return upConf.MaxPresidentCount <= cnt
	case custom_id.GuildPos_Common:
		return false
	}
	return true
}

func (guild *Guild) AddMoney(addValue int64) bool {
	money := guild.BasicInfo.Money
	money += addValue
	lv := guild.GetLevel()
	confList := jsondata.GetGuildUpgradeConfList()
	lvMax := uint32(len(confList))
	for ntLv := lv + 1; ntLv <= lvMax; ntLv++ {
		conf := jsondata.GetGuildUpgradeConf(ntLv)
		if conf == nil || conf.Money > money {
			break
		}
		if money >= conf.Money {
			money -= conf.Money
			guild.SetLevel(ntLv)
		}
	}

	guild.BasicInfo.Money = money
	guild.BroadcastProto(29, 112, &pb3.S2C_29_112{Level: guild.GetLevel()})
	guild.BroadcastProto(29, 113, &pb3.S2C_29_113{Moneny: money})
	guild.SetSaveFlag()
	return true
}

func (guild *Guild) AutoTransferLeader() {
	leader := guild.GetLeader()
	if player := manager.GetPlayerPtrById(guild.GetLeaderId()); nil != player {
		return
	}
	logoutTime := leader.GetPlayerInfo().GetLastLogoutTime()
	loginTime := leader.GetPlayerInfo().GetLoginTime()

	if loginTime > logoutTime || logoutTime == 0 {
		return
	}
	offlineTime := jsondata.GlobalUint("offLineTime")
	if time_util.NowSec()-logoutTime < offlineTime {
		return
	}
	limit := jsondata.GlobalUint("guildATransferenceTime")
	var p1, p2, p3 *pb3.GuildMemberInfo
	var power1, power2, power3 uint64
	for pid, member := range guild.Members {
		if pid == guild.GetLeaderId() || engine.IsRobot(pid) {
			continue
		}
		player := member.GetPlayerInfo()
		if player.GetPower() > power3 { //盟内战力最高
			power3 = player.GetPower()
			p3 = member
		}
		if nil == manager.GetPlayerPtrById(player.GetId()) {
			if player.GetLastLogoutTime() > player.GetLoginTime() {
				if time_util.NowSec()-player.GetLastLogoutTime() >= limit {
					continue
				}
			}
		}
		if member.GetPosition() == custom_id.GuildPos_DeputyLeader { //在线时长内副盟战力最高
			if player.GetPower() > power1 {
				power1 = member.GetPlayerInfo().GetPower()
				p1 = member
			}
		} else if p1 == nil && player.GetPower() > power2 { //在线时长内成员战力最高
			power2 = member.GetPlayerInfo().GetPower()
			p2 = member
		}
	}
	if nil == p1 && nil != p2 {
		p1 = p2
	}
	if nil == p1 && nil != p3 {
		p1 = p3
	}
	if p1 != nil {
		guild.SetGuildPos(leader, custom_id.GuildPos_Common)
		guild.SetGuildPos(p1, custom_id.GuildPos_Leader)
		mailmgr.SendMailToActor(p1.GetPlayerInfo().GetId(), &mailargs.SendMailSt{ConfId: common.Mail_GuildTransferLeaderAuto})
	}
}

func (guild *Guild) AddPersonDonate(actorId uint64, value int64) {
	member := guild.GetMember(actorId)
	if nil == member {
		return
	}

	member.Donate += value

	guild.BroadcastProto(29, 111, &pb3.S2C_29_111{AcotrId: actorId, Donate: member.Donate})
}

func (guild *Guild) AddPower(value uint64) {
	guild.BasicInfo.Power += value
	toUpGuildRank(guild.GetId(), guild.GetLevel(), guild.GetPower())
}

func toUpGuildRank(gid uint64, gLevel uint32, gPower uint64) {
	event.TriggerSysEvent(custom_id.GuildLevelPowerChange, &pb3.CommonSt{U64Param: gid, U32Param: gLevel, U64Param2: gPower})
}

func (guild *Guild) ReducePower(value uint64) {
	if value > guild.BasicInfo.Power {
		guild.BasicInfo.Power = 0
		return
	}
	guild.BasicInfo.Power -= value
	toUpGuildRank(guild.GetId(), guild.GetLevel(), guild.GetPower())
}

func (guild *Guild) OnMemberDataBaseChange(newInfo *pb3.SimplyPlayerData) {
	if nil == newInfo {
		return
	}
	if member := guild.GetMember(newInfo.GetId()); nil != member {
		newPower := newInfo.GetPower()
		if member.GetPosition() == custom_id.GuildPos_Leader {
			if guild.BasicInfo.LeaderName != newInfo.GetName() {
				guild.BasicInfo.LeaderName = newInfo.GetName()
				guild.SetSaveFlag()
				guild.UpdateMember(member)
			}
			if guild.BasicInfo.LeaderBoundaryLv != member.GetPlayerInfo().GetCircle() {
				guild.BasicInfo.LeaderBoundaryLv = member.GetPlayerInfo().GetCircle()
				guild.SetSaveFlag()
			}
		}
		oldPower := member.GetPlayerInfo().GetPower()
		if newPower > oldPower {
			diff := newPower - oldPower
			guild.AddPower(diff)
		} else if newPower < oldPower {
			diff := oldPower - newPower
			guild.ReducePower(diff)
		}
		member.PlayerInfo = newInfo
	}
}

func (guild *Guild) CheckGuildTransfer(player iface.IPlayer) {
	if !engine.IsRobot(guild.GetLeaderId()) {
		return
	}
	conf := jsondata.GetMainCityRobotGuild()
	if nil == conf {
		return
	}

	robotLeaderId := guild.GetLeaderId()
	trActorId := player.GetId()

	fightValue := player.GetExtraAttr(attrdef.FightValue)
	if conf.TransferFightVal > fightValue {
		return
	}

	robotLeader := guild.Members[robotLeaderId]
	tMember := guild.Members[trActorId]
	if nil == tMember {
		return
	}

	if nil != robotLeader {
		guild.SetGuildPos(robotLeader, custom_id.GuildPos_Common)
	}

	guild.SetGuildPos(tMember, custom_id.GuildPos_Leader)
	guild.SetSaveFlag()

	mailmgr.SendMailToActor(trActorId, &mailargs.SendMailSt{ConfId: common.Mail_GuildTransferLeader, Content: &mailargs.PlayerNameArgs{Name: robotLeader.PlayerInfo.GetName()}})
	guild.AddEvent(custom_id.GuildEvent_Commission, robotLeader.PlayerInfo.GetName(), tMember.GetPlayerInfo().GetName(), tMember.GetPosition())

	guild.RemoveMember(robotLeaderId)
}

func (guild *Guild) Destroy() {
	for actorId := range guild.Members {
		if engine.IsRobot(actorId) { // 机器人
			if robot := engine.GetRobotById(actorId); nil != robot {
				robot.SetGuildId(0)
			}
		} else {
			if player := manager.GetPlayerPtrById(actorId); nil != player {
				player.SetGuildId(0)
			}
		}
	}

	guild.Members = nil
}

func queryPlayerBasicRet(args ...interface{}) {
	data, ok := args[0].(*pb3.SimplyPlayerData)
	if !ok {
		return
	}

	guild := GetGuildById(data.GetGuildId())
	if nil == guild {
		return
	}

	member := guild.GetMember(data.GetId())
	if nil == member {
		return
	}

	guild.OnMemberDataBaseChange(data)

}

func useAddGuildMoney(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	guildId := player.GetGuildId()
	var ok bool
	if guild := GetGuildById(guildId); nil != guild {
		ok = guild.AddMoney(param.Count * int64(conf.Param[0]))
		if ok {
			return true, true, param.Count
		}
	}
	return false, false, 0
}

func init() {
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddGuildMoney, useAddGuildMoney)

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgGetPlayerBasicInfoRet, queryPlayerBasicRet)
	})
}
