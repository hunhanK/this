package actorsystem

import (
	"errors"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/inffairyplacemgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"

	wordmonitor2 "github.com/gzjjyz/wordmonitor"
)

/*
	desc:聊天系统
	author: ChenJunJi
*/

var WorldChatCache = make([]*pb3.S2C_5_1, 0, 20)

type ChatSys struct {
	Base
	ChannelCd map[uint32]uint32
	circle    uint32
	level     uint32
	isOpen    bool
}

func (sys *ChatSys) OnInit() {
	sys.ChannelCd = make(map[uint32]uint32)
}

func (sys *ChatSys) OnLogin() {
	binary := sys.GetBinaryData()
	if binary.HasNewBieChat == 1 {
		sys.owner.SetExtraAttr(attrdef.HasNewBieChat, 1)
	}
	manager.SendChatRule(sys.owner)
}

func (sys *ChatSys) OnAfterLogin() {
	chat := make([]*pb3.S2C_5_1, 0, 20)
	chat = append(chat, WorldChatCache...)
	if g := guildmgr.GetGuildById(sys.owner.GetGuildId()); nil != g {
		chat = append(chat, g.ChatCache...)
	}
	sys.SendProto3(5, 10, &pb3.S2C_5_10{ChatList: chat})
}

func (sys *ChatSys) OnReconnect() {
	chat := make([]*pb3.S2C_5_1, 0, 20)
	chat = append(chat, WorldChatCache...)
	if g := guildmgr.GetGuildById(sys.owner.GetGuildId()); nil != g {
		chat = append(chat, g.ChatCache...)
	}
	sys.SendProto3(5, 10, &pb3.S2C_5_10{ChatList: chat})
}

func (sys *ChatSys) Check(channel uint32, target iface.IPlayer) bool {
	conf := jsondata.GetChatConf(channel)
	if nil == conf {
		sys.LogWarn("%s 聊天频道不存在， channel：%d", sys.owner.GetName(), channel)
		sys.owner.SendTipMsg(tipmsgid.ChatChannelNotFound)
		return false
	}

	level := sys.owner.GetLevel()
	sec := time_util.NowSec()
	channelCd := sys.ChannelCd[channel]
	if channelCd > sec {
		sys.owner.SendTipMsg(tipmsgid.TpChatCd, channelCd-sec)
		return false
	}

	chatRule := manager.GetChatRule(engine.GetPfId(), channel)
	if chatRule != nil && !chatRule.Check(sys.owner) {
		return false
	}

	if level < conf.Level {
		sys.LogWarn("发送等级未达到，当前等级:%d, 限制等级:%d", level, conf.Level)
		sys.owner.SendTipMsg(tipmsgid.TpChatLimit, conf.Level)
		return false
	}

	owner := sys.GetOwner()

	var checkVipLvAndLv = func(chatRule *manager.PfChannelChatRule) bool {
		if chatRule != nil && chatRule.VipLevel != 0 && owner.GetVipLevel() < chatRule.VipLevel {
			owner.LogWarn("贵族等级未达到，当前:%d, 限制:%d", owner.GetVipLevel(), chatRule.VipLevel)
			owner.SendTipMsg(tipmsgid.TpChatLimitByVipLv, chatRule.VipLevel)
			return false
		}

		if chatRule != nil && chatRule.VipLevel != 0 && owner.GetVipLevel() < chatRule.VipLevel && chatRule.Level != 0 && owner.GetLevel() < chatRule.Level {
			owner.LogWarn("等级未达到，当前:%d, 限制:%d", owner.GetLevel(), chatRule.Level)
			owner.SendTipMsg(tipmsgid.TpChatLimitByLv, chatRule.Level)
			return false
		}

		if level < conf.ChatLevel && owner.GetVipLevel() < conf.ChatVip {
			sys.owner.SendTipMsg(tipmsgid.TpChatLimit, conf.Level)
			return false
		}
		return true
	}
	if !checkVipLvAndLv(chatRule) {
		return false
	}

	switch channel {
	case chatdef.CIPrivate:
		if nil == target || target.GetLost() {
			sys.LogWarn("发送目标不在线")
			sys.owner.SendTipMsg(tipmsgid.TpPlayerOfflineChat)
			return false
		}
		if !checkTarget(target, 0, level) {
			sys.LogWarn("不满足对方聊天限制")
			sys.owner.SendTipMsg(tipmsgid.TpTargetChatLimit)
			return false
		}
		if target.GetLevel() < conf.Level {
			sys.LogWarn("对方聊天等级未达到，当前等级:%d, 限制等级:%d", target.GetLevel(), conf.Level)
			sys.owner.SendTipMsg(tipmsgid.TpChatLimit, conf.Level)
			return false
		}
	//case custom_id.CITeam:
	//	if teamIdInfo := sys.owner.GetTeamId(); teamIdInfo.TeamId == 0 {
	//		sys.owner.SendTipMsg(tipmsgid.TpStr, "您还没有队伍!")
	//		return false
	//	}
	case chatdef.CIGuild:
		sys.LogInfo("玩家仙盟ID:%d", sys.owner.GetGuildId())
		if sys.owner.GetGuildId() <= 0 { // 玩家还没有加入仙盟
			sys.owner.SendTipMsg(tipmsgid.TpGuildNotContainSelf)
			return false
		}
	default:
		return true
	}
	return true
}

func (sys *ChatSys) SetChatCd(channel uint32, conf *jsondata.ChannelConf) {
	sys.ChannelCd[channel] = time_util.NowSec() + conf.Cd
	rsp := &pb3.S2C_5_2{
		Channel:   channel,
		CdEndTime: sys.ChannelCd[channel],
	}
	sys.SendProto3(5, 2, rsp)
}

func (sys *ChatSys) consume(conf *jsondata.ChannelConf) bool {
	if len(conf.Consume) != 0 && !sys.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogChatConsume}) {
		return false
	}
	return true
}

func RegChatTitlePower(playerId uint64, conf *jsondata.ChatTitleActiveConf) uint32 {
	powerRank := manager.GRankMgrIns.GetRankByType(gshare.RankTypePower).GetRankById(playerId)
	if powerRank <= 0 {
		return 0
	}
	for _, v := range conf.Titles {
		if v.Param[0] == powerRank {
			return v.TitleId
		}
	}
	return 0
}

func RegChatTitleInfFairyPlace(playerId uint64, conf *jsondata.ChatTitleActiveConf) uint32 {
	var job, isCross uint32 = 0, 1
	if crossJobInfo := inffairyplacemgr.GetCrossInfFairyPlaceJobInfo(); nil != crossJobInfo {
		for k, v := range crossJobInfo {
			if v == playerId {
				isCross = 2
				job = k
				break
			}
		}
	}

	if job == 0 {
		info := inffairyplacemgr.GetLocalInfFairyPlaceMgr().GetLocalInfo()
		if nil != info && nil != info.JobInfo {
			for k, actorId := range info.JobInfo {
				if actorId == playerId {
					job = k
					break
				}
			}
		}
	}

	if job > 0 {
		for _, v := range conf.Titles {
			if v.Param[0] == isCross && v.Param[1] == job {
				return v.TitleId
			}
		}
	}

	return 0
}

func sendSysChatWordMsgDirectly(rsp *pb3.S2C_5_1, cache bool) {
	engine.Broadcast(chatdef.CIWorld, 0, 5, 1, rsp, 0)
	if cache {
		if len(WorldChatCache) >= 20 {
			WorldChatCache = WorldChatCache[1:]
		}
		WorldChatCache = append(WorldChatCache, rsp)
	}
}

func (sys *ChatSys) ChannelChat(req *pb3.C2S_5_1) {
	channel := req.GetChannel()
	content := req.GetMsg()
	checkContent := req.GetParams()
	toId := req.GetToId()
	target := manager.GetPlayerPtrById(toId)

	conf := jsondata.GetChatConf(channel)

	player := sys.owner
	sys.SetChatCd(channel, conf)
	chatPlayerData := &pb3.ChatPlayerData{
		PlayerId:        player.GetId(),
		Name:            player.GetName(),
		Sex:             player.GetSex(),
		Career:          player.GetJob(),
		PfId:            engine.GetPfId(),
		ServerId:        engine.GetServerId(),
		VipLv:           player.GetBinaryData().GetVip(),
		HeadFrame:       player.GetHeadFrame(),
		BubbleFrame:     player.GetBubbleFrame(),
		Head:            player.GetHead(),
		Level:           player.GetLevel(),
		SmallCrossCamp:  uint32(player.GetSmallCrossCamp()),
		ChatTitles:      jsondata.PackChatTitle(player.GetId()),
		FlyCamp:         player.GetExtraAttrU32(attrdef.FlyCamp),
		MediumCrossCamp: player.GetMediumCrossCamp(),
	}

	jinjieSys := player.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if jinjieSys != nil && jinjieSys.IsOpen() {
		chatPlayerData.JinjieLevel = jinjieSys.GetData().GetLevel()
	}

	rsp := &pb3.S2C_5_1{
		Channel:     channel,
		SenderData:  chatPlayerData,
		Msg:         content,
		ItemIds:     req.GetItemIds(),
		ToId:        toId,
		Params:      checkContent,
		ContentType: req.ContentType,
	}

	// 后台返回1：包含敏感字
	if req.BackStageRet == uint32(wordmonitor2.Suspect) {
		sys.owner.SendProto3(5, 1, rsp)
		return
	}

	switch channel {
	case chatdef.CIWorld:
		sys.owner.TriggerQuestEvent(custom_id.QttWorldChat, 0, 1)
		sys.owner.TriggerQuestEvent(custom_id.QttWorldOrCrossChat, 0, 1)
		sendSysChatWordMsgDirectly(rsp, channel == chatdef.CIWorld)
	case chatdef.CIBroadcast:
		engine.Broadcast(chatdef.CIWorld, 0, 5, 1, rsp, 0)
		sys.owner.BroadcastCustomTipMsgById(tipmsgid.TpCustomVocalize, player.GetName(), rsp.Msg)
	case chatdef.CINear: // 场景内玩家广播
		player.CallActorFunc(actorfuncid.ChatNear, rsp)
	case chatdef.CITeamUp: // 组队信息广播
		engine.Broadcast(chatdef.CIWorld, 0, 5, 1, rsp, 0)
	case chatdef.CITeam: // 队伍内玩家广播
		teamId := player.GetTeamId()
		if _, err := teammgr.GetTeamState(teamId); err == nil {
			teammgr.BroadCastToMember(teamId, 5, 1, rsp, 0)
		}
	case chatdef.CIPrivate:
		// 非好友不能私聊
		if nil != target {
			if fSys, ok := target.GetSysObj(sysdef.SiFriend).(*FriendSys); ok {
				if fSys.IsExistFriend(sys.owner.GetId(), custom_id.FrFriend) {
					target.SendProto3(5, 1, rsp)
					sys.SendProto3(5, 1, rsp)
				}
			}
		}
	case chatdef.CIGuild:
		g := guildmgr.GetGuildById(sys.owner.GetGuildId())
		if nil == g {
			sys.LogWarn("actor not guild(id:%d)", sys.owner.GetGuildId())
			break
		}

		g.BroadcastProto(5, 1, rsp)

		// 行会聊天缓存
		if len(g.ChatCache) >= 20 {
			g.ChatCache = g.ChatCache[1:]
		}
		g.ChatCache = append(g.ChatCache, rsp)
	case chatdef.CICrossChat:
		sys.owner.TriggerQuestEvent(custom_id.QttWorldOrCrossChat, 0, 1)
		engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CCrossChat, rsp)
	case chatdef.CIMediumCrossChat:
		engine.CallFightSrvFunc(base.MediumCrossServer, sysfuncid.G2CMediumCrossChat, rsp)
	}

	sys.owner.TriggerQuestEvent(custom_id.QttChatTimes, channel, 1)
	chatReq := &pb3.LogChat{
		FromUserId:    sys.owner.GetUserId(),
		FromActorId:   sys.owner.GetId(),
		FromActorName: sys.owner.GetName(),
		Channel:       channel,
		Content:       content,
		FromIp:        sys.owner.GetRemoteAddr(),
		FromPf:        sys.owner.GetExtraAttrU32(attrdef.DitchId),
		Time:          uint32(time.Now().Unix()),
		PfId:          engine.GetPfId(),
		FromVip:       sys.owner.GetVipLevel(),
		FromLevel:     sys.owner.GetLevel(),
		ServerId:      engine.GetServerId(),
	}
	if target != nil {
		chatReq.ToVip = target.GetVipLevel()
		chatReq.ToLevel = target.GetLevel()
		chatReq.ToActorName = target.GetName()
		chatReq.ToActorId = target.GetId()
		chatReq.ToUserId = target.GetUserId()
	}
	switch req.ContentType {
	case chatdef.ContentAskHelp,
		chatdef.ContentGlobalCollectCardAskHelp,
		chatdef.ContentCollectCardAskHelp,
		chatdef.ContentUltimateBossRedPaper,
		chatdef.ContentDivineRealmDividendRank:
	default:
		logworker.LogChat(chatReq)
	}
}

// 请求聊天
func (sys *ChatSys) c2sChat(msg *base.Message) {
	req := &pb3.C2S_5_1{}
	err := pb3.Unmarshal(msg.Data, req)
	if err != nil {
		return
	}

	forbidTime := sys.owner.GetBinaryData().JinYanTime
	nowSec := time_util.NowSec()
	if forbidTime > nowSec {
		return
	}

	channel := req.GetChannel()
	toId := req.GetToId()
	target := manager.GetPlayerPtrById(toId)
	if !sys.Check(channel, target) {
		sys.LogWarn("检查发送条件不通过")
		return
	}
	conf := jsondata.GetChatConf(channel)
	if !sys.consume(conf) {
		sys.LogWarn("消耗不足")
		return
	}

	// 设置第一次发言标志
	if sys.owner.GetExtraAttr(attrdef.HasNewBieChat) == 0 {
		sys.owner.GetBinaryData().HasNewBieChat = 1
		sys.owner.SetExtraAttr(attrdef.HasNewBieChat, 1)
	}

	switch req.ContentType {
	case chatdef.ContentEmoji:
		if sys.checkSendEmoji(req.Params) {
			sys.ChannelChat(req)
		}
		return
	default:
		commonData := sys.GetOwner().BuildChatBaseData(target)
		commonData.ChatChannel = channel
		engine.SendWordMonitor(wordmonitor.Chat, wordmonitor.PlayerChat, req.GetMsg(),
			wordmonitoroption.WithPlayerId(sys.GetOwner().GetId()),
			wordmonitoroption.WithRawData(req),
			wordmonitoroption.WithCommonData(commonData),
			wordmonitoroption.WithDitchId(sys.GetOwner().GetExtraAttrU32(attrdef.DitchId)),
		)
	}
}

// 设置聊天限制条件
func (sys *ChatSys) c2sSetLimitCond(msg *base.Message) {
}

func (sys *ChatSys) checkSendEmoji(params string) bool {
	eId := utils.AtoUint32(params)
	eConf := jsondata.GetChatExpressionConf(eId)
	if eConf != nil && eConf.IsLock == 1 {
		binary := sys.owner.GetBinaryData()
		eIds := binary.ChatEmojiIds
		if eIds == nil || !utils.SliceContainsUint32(eIds, eId) {
			sys.LogWarn("表情%d 未解锁", eId)
			return false
		}
	}
	return true
}

func checkTarget(target iface.IPlayer, circle, level uint32) bool {
	sys, ok := target.GetSysObj(sysdef.SiChat).(*ChatSys)
	if !ok {
		return false
	}
	if sys.isOpen {
		if sys.circle > circle {
			return false
		}
		if sys.circle == circle && (sys.level > level || sys.level == 0) {
			return false
		}
	}
	return true
}

func ClearWorldChat(playerId uint64) {
	newCache := make([]*pb3.S2C_5_1, 0, 20)
	for _, v := range WorldChatCache {
		if nil == v {
			continue
		}
		data := v.SenderData
		if nil != data && data.PlayerId != playerId {
			newCache = append(newCache, v)
		}
	}

	WorldChatCache = newCache
	engine.Broadcast(chatdef.CIWorld, 0, 5, 3, &pb3.S2C_5_3{PlayerId: playerId}, 0)
}

func onChatMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.ChatContentHasIllegalCharacter)
		return nil
	}
	req, ok := word.Data.(*pb3.C2S_5_1)
	if !ok {
		return errors.New("not *pb3.C2S_5_1")
	}

	sys, ok := player.GetSysObj(sysdef.SiChat).(*ChatSys)
	if !ok {
		return nil
	}

	if word.BackStageRet == wordmonitor2.Suspect {
		req.BackStageRet = uint32(word.BackStageRet)
	}

	sys.ChannelChat(req)
	return nil
}

func handleSeClearActorChat(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	st, ok := args[0].(*pb3.CommonSt)
	if !ok {
		return
	}
	var newList []*pb3.S2C_5_1
	for _, result := range WorldChatCache {
		if result.SenderData != nil && result.SenderData.PlayerId == st.U64Param {
			continue
		}
		newList = append(newList, result)
	}
	WorldChatCache = newList
}

func init() {
	RegisterSysClass(sysdef.SiChat, func() iface.ISystem {
		return &ChatSys{}
	})
	net.RegisterSysProto(5, 1, sysdef.SiChat, (*ChatSys).c2sChat)

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		engine.RegWordMonitorOpCodeHandler(wordmonitor.PlayerChat, onChatMonitorRet)
		jsondata.RegChatTitleCheck(chatdef.CTitlePower, RegChatTitlePower)
		jsondata.RegChatTitleCheck(chatdef.CTitleInfFairyPlace, RegChatTitleInfFairyPlace)
	})
	event.RegSysEvent(custom_id.SeClearActorChat, handleSeClearActorChat)
}
