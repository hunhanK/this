/**
 * @Author: lzp
 * @Date: 2025/7/30
 * @Desc:
**/

package yy

import (
	"errors"
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"unicode/utf8"
)

type YYGuildChangeName struct {
	YYBase
}

func (yy *YYGuildChangeName) OnOpen() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		yy.s2cPlayerInfo(player)
	})
}

func (yy *YYGuildChangeName) OnEnd() {
	data := yy.GetData()
	for k, v := range data.PlayerData {
		playerData, ok := manager.GetData(k, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			continue
		}
		guildId := playerData.GuildId
		guildData, ok := data.GuildData[guildId]
		if !ok {
			continue
		}

		yy.sendNotRecRewards(k, v, guildData.Count)
	}
}

func (yy *YYGuildChangeName) GetData() *pb3.YYGuildChangeName {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.GChangeNameData == nil {
		globalVar.YyDatas.GChangeNameData = make(map[uint32]*pb3.YYGuildChangeName)
	}

	tmp := globalVar.YyDatas.GChangeNameData[yy.Id]
	if tmp == nil {
		tmp = new(pb3.YYGuildChangeName)
	}

	if tmp.GuildData == nil {
		tmp.GuildData = make(map[uint64]*pb3.YYGuildPrefixNameData)
	}

	if tmp.PlayerData == nil {
		tmp.PlayerData = make(map[uint64]*pb3.YYPlayerPrefixNameData)
	}

	if tmp.GuildPrefixNames == nil {
		tmp.GuildPrefixNames = make(map[string]bool)
	}

	globalVar.YyDatas.GChangeNameData[yy.Id] = tmp
	return tmp
}

func (yy *YYGuildChangeName) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil || globalVar.YyDatas.GChangeNameData == nil {
		return
	}
	delete(globalVar.YyDatas.GChangeNameData, yy.GetId())
}

func (yy *YYGuildChangeName) PlayerLogin(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYGuildChangeName) PlayerReconnect(player iface.IPlayer) {
	yy.s2cPlayerInfo(player)
}

func (yy *YYGuildChangeName) s2cPlayerInfo(player iface.IPlayer) {
	data := yy.GetData()
	player.SendProto3(127, 180, &pb3.S2C_127_180{
		ActiveId:   yy.Id,
		GuildData:  data.GuildData[player.GetGuildId()],
		PlayerData: data.PlayerData[player.GetId()],
	})
}

func (yy *YYGuildChangeName) sendNotRecRewards(playerId uint64, data *pb3.YYPlayerPrefixNameData, count uint32) {
	conf := jsondata.GetGuildChangeNameConf(yy.ConfName, yy.ConfIdx)
	if conf == nil {
		return
	}

	var rewards jsondata.StdRewardVec

	// 参与奖励
	if data.IsChange && !data.HasFetched {
		rewards = append(rewards, conf.Rewards...)
		data.HasFetched = true
	}

	// 达标奖励
	for _, rConf := range conf.StandRewards {
		if count < rConf.Count {
			break
		}

		if utils.SliceContainsUint32(data.Ids, rConf.Id) {
			continue
		}

		rewards = append(rewards, rConf.Rewards...)
		data.Ids = append(data.Ids, rConf.Id)
	}

	if len(rewards) > 0 {
		mailmgr.SendMailToActor(playerId, &mailargs.SendMailSt{
			ConfId:  uint16(conf.MailId),
			Rewards: rewards,
		})
	}
}

func (yy *YYGuildChangeName) c2sSetGuildPrefixName(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_127_181
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	if !player.IsGuildLeader() {
		return neterror.ParamsInvalidError("playerId:%d is not guild leader", player.GetId())
	}

	guildId := player.GetGuildId()
	data := yy.GetData()
	guildData, ok := data.GuildData[guildId]
	if ok && guildData.IsSet {
		return neterror.ParamsInvalidError("guild prefix name is set")
	}

	if _, ok := data.GuildPrefixNames[req.GuildPrefixName]; ok {
		player.SendTipMsg(tipmsgid.TpGuildNameExist)
		return nil
	}

	name := req.GetGuildPrefixName()
	nameLen := utf8.RuneCountInString(name)
	lenLimit := jsondata.GetCommonConf("guildNameLimit").U32
	if nameLen > int(lenLimit) || nameLen <= 0 {
		player.SendTipMsg(tipmsgid.TpGuildNameLenLimit)
		return nil
	}
	if !engine.CheckNameSpecialCharacter(name) {
		player.SendTipMsg(tipmsgid.TpGuildNameLenLimit)
		return nil
	}

	engine.SendWordMonitor(wordmonitor.GuildPrefixName, wordmonitor.ChangeGuildPrefixName, req.GetGuildPrefixName(),
		wordmonitoroption.WithPlayerId(player.GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(player.BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(player.GetExtraAttrU32(attrdef.DitchId)),
	)

	return nil
}

func (yy *YYGuildChangeName) c2sChangeRoleName(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_127_182
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	guildId := player.GetGuildId()
	if guildId == 0 {
		return neterror.ParamsInvalidError("player not join guild")
	}

	data := yy.GetData()
	playerData, ok := data.PlayerData[player.GetId()]
	if ok && playerData.IsChange {
		return neterror.ParamsInvalidError("player has changed name")
	}

	newName := req.GetName()
	nameLen := utf8.RuneCountInString(newName)
	nameLenLimit := jsondata.GetNameLenLimit()
	if nameLen > int(nameLenLimit) || nameLen <= 0 {
		return neterror.ParamsInvalidError("name len limit")
	}

	if !engine.CheckNameRepeat(newName) {
		player.SendTipMsg(tipmsgid.ChangeNameIsExist)
		return neterror.ParamsInvalidError("name repeated")
	}

	engine.AddPendingName(newName)

	engine.SendWordMonitor(wordmonitor.Name, wordmonitor.YYChangePlayerName, newName,
		wordmonitoroption.WithPlayerId(player.GetId()),
		wordmonitoroption.WithCommonData(player.BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(player.GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func (yy *YYGuildChangeName) c2sFetchRewards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_127_183
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetGuildChangeNameStandRewards(yy.ConfName, yy.ConfIdx, req.Id)
	if conf == nil {
		return neterror.ParamsInvalidError("id:%d config not found", req.Id)
	}

	data := yy.GetData()
	playerData, ok := data.PlayerData[player.GetId()]
	if !ok {
		data.PlayerData[player.GetId()] = &pb3.YYPlayerPrefixNameData{
			PlayerId: player.GetId(),
		}
		playerData = data.PlayerData[player.GetId()]
	}

	if utils.SliceContainsUint32(playerData.Ids, req.Id) {
		return neterror.ParamsInvalidError("id:%d rewards has fetched", req.Id)
	}

	playerData.Ids = append(playerData.Ids, req.Id)

	engine.GiveRewards(player, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGuildChangeNameStandRewards})
	yy.s2cPlayerInfo(player)
	return nil
}

func (yy *YYGuildChangeName) c2sFetchJoinRewards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_127_184
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetGuildChangeNameConf(yy.ConfName, yy.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("config not found")
	}

	data := yy.GetData()
	playerData, ok := data.PlayerData[player.GetId()]
	if !ok || !playerData.IsChange {
		return neterror.ParamsInvalidError("not participate")
	}
	if playerData.HasFetched {
		return neterror.ParamsInvalidError("rewards has fetched")
	}

	playerData.HasFetched = true
	if len(conf.Rewards) > 0 {
		engine.GiveRewards(player, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGuildChangeNameJoinRewards})
	}
	yy.s2cPlayerInfo(player)
	return nil
}

func (yy *YYGuildChangeName) onChangeGuildPrefixName(req *pb3.C2S_127_181, player iface.IPlayer) {
	data := yy.GetData()
	guildId := player.GetGuildId()
	data.GuildData[guildId] = &pb3.YYGuildPrefixNameData{
		GuildId:    guildId,
		IsSet:      true,
		PrefixName: req.GuildPrefixName,
	}
	data.GuildPrefixNames[req.GuildPrefixName] = true

	guild := guildmgr.GetGuildById(guildId)
	for _, mem := range guild.GetMembers() {
		memPlayer := manager.GetPlayerPtrById(mem.PlayerInfo.Id)
		if memPlayer == nil {
			continue
		}
		yy.s2cPlayerInfo(memPlayer)
	}

	conf := jsondata.GetGuildChangeNameConf(yy.ConfName, yy.ConfIdx)
	if conf != nil {
		playerInfo := guild.GetLeader().PlayerInfo
		guild.BroadcastProto(5, 1, &pb3.S2C_5_1{
			Channel: chatdef.CIGuild,
			Msg:     fmt.Sprintf(conf.Channel, req.GuildPrefixName),
			SenderData: &pb3.ChatPlayerData{
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
			},
		})
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogYYCampCheeringEventAddProcess, &pb3.LogPlayerCounter{
		NumArgs: uint64(yy.GetId()),
		StrArgs: fmt.Sprintf("%s", req.GuildPrefixName),
	})
}

func (yy *YYGuildChangeName) onChangePlayerName(newName string, player iface.IPlayer) {
	engine.DelPendingName(newName)
	engine.AddPlayerName(newName)
	engine.RemovePlayerName(player.GetName())

	player.SetName(newName)
	player.TriggerEvent(custom_id.AeChangeName)
	player.CallActorFunc(actorfuncid.ChangeName, &pb3.ChangeName{NewName: newName})

	data := yy.GetData()

	playerData, ok := data.PlayerData[player.GetId()]
	if !ok {
		playerData = &pb3.YYPlayerPrefixNameData{
			PlayerId: player.GetId(),
			IsChange: true,
		}
		data.PlayerData[player.GetId()] = playerData
	} else {
		playerData.IsChange = true
	}

	guildData := data.GuildData[player.GetGuildId()]
	if !playerData.HasChanged {
		guildData.Count++
		playerData.HasChanged = true
	}

	yy.s2cPlayerInfo(player)
	player.SendProto3(2, 11, &pb3.S2C_2_11{
		Code:    0,
		NewName: player.GetName(),
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogYYCampCheeringEventAddProcess, &pb3.LogPlayerCounter{
		NumArgs: uint64(yy.GetId()),
		StrArgs: fmt.Sprintf("%s", player.GetName()),
	})
}

func (yy *YYGuildChangeName) handleGuildMemberJoin(guildId, playerId uint64) {
	data := yy.GetData()
	if _, ok := data.PlayerData[playerId]; !ok {
		data.PlayerData[playerId] = &pb3.YYPlayerPrefixNameData{
			PlayerId: playerId,
		}
	}

	player := manager.GetPlayerPtrById(playerId)
	if player != nil {
		yy.s2cPlayerInfo(player)
	}
}

func (yy *YYGuildChangeName) handleGuildMemberExit(guildId, playerId uint64) {
	data := yy.GetData()
	playerData, ok := data.PlayerData[playerId]
	if ok {
		playerData.IsChange = false
	}

	player := manager.GetPlayerPtrById(playerId)
	if player != nil {
		yy.s2cPlayerInfo(player)
	}
}

func (yy *YYGuildChangeName) handleGuildDismiss(guildId uint64) {
	data := yy.GetData()
	guildData, ok := data.GuildData[guildId]
	if ok {
		if guildData.PrefixName != "" {
			delete(data.GuildPrefixNames, guildData.PrefixName)
		}
	}
	delete(data.GuildData, guildId)
}

func onChangeGuildPrefixNameMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}
	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.TpSensitiveWord)
		return nil
	}

	req, ok := word.Data.(*pb3.C2S_127_181)
	if !ok {
		return errors.New("not *pb3.C2S_127_181")
	}

	allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
	for _, iYY := range allYY {
		if yy, ok := iYY.(*YYGuildChangeName); ok && yy.IsOpen() {
			yy.onChangeGuildPrefixName(req, player)
		}
	}
	return nil
}

func onYYChangePlayerNameMonitorRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if player == nil {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.ChangeName)
		return nil
	}

	newName := word.Content
	allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
	for _, iYY := range allYY {
		if yy, ok := iYY.(*YYGuildChangeName); ok && yy.IsOpen() {
			yy.onChangePlayerName(newName, player)
		}
	}

	return nil
}

func init() {
	yymgr.RegisterYYType(yydefine.YYGuildChangeName, func() iface.IYunYing {
		return &YYGuildChangeName{}
	})

	net.RegisterGlobalYYSysProto(127, 181, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGuildChangeName).c2sSetGuildPrefixName
	})
	net.RegisterGlobalYYSysProto(127, 182, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGuildChangeName).c2sChangeRoleName
	})
	net.RegisterGlobalYYSysProto(127, 183, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGuildChangeName).c2sFetchRewards
	})
	net.RegisterGlobalYYSysProto(127, 184, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYGuildChangeName).c2sFetchJoinRewards
	})

	engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangeGuildPrefixName, onChangeGuildPrefixNameMonitorRet)
	engine.RegWordMonitorOpCodeHandler(wordmonitor.YYChangePlayerName, onYYChangePlayerNameMonitorRet)

	event.RegSysEvent(custom_id.SeGuildAddMember, func(args ...interface{}) {
		if len(args) < 2 {
			return
		}
		guildId, ok1 := args[0].(uint64)
		actorId, ok2 := args[1].(uint64)
		if !ok1 || !ok2 {
			return
		}

		allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
		for _, iYY := range allYY {
			if yy, ok := iYY.(*YYGuildChangeName); ok && yy.IsOpen() {
				yy.handleGuildMemberJoin(guildId, actorId)
			}
		}
	})

	event.RegSysEvent(custom_id.SeGuildRemoveMember, func(args ...interface{}) {
		if len(args) < 2 {
			return
		}
		guildId, ok1 := args[0].(uint64)
		actorId, ok2 := args[1].(uint64)
		if !ok1 || !ok2 {
			return
		}

		allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
		for _, iYY := range allYY {
			if yy, ok := iYY.(*YYGuildChangeName); ok && yy.IsOpen() {
				yy.handleGuildMemberExit(guildId, actorId)
			}
		}
	})

	event.RegSysEvent(custom_id.SeGuildDestroy, func(args ...interface{}) {
		if len(args) < 1 {
			return
		}
		guildId, ok := args[0].(uint64)
		if !ok {
			return
		}

		allYY := yymgr.GetAllYY(yydefine.YYGuildChangeName)
		for _, iYY := range allYY {
			if yy, ok := iYY.(*YYGuildChangeName); ok && yy.IsOpen() {
				yy.handleGuildDismiss(guildId)
			}
		}
	})
}
