/**
 * @Author: beiming
 * @Desc: 仙盟宴会
 * @Date: 2023/11/28
 */

package activity

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/activitydef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"math"

	"github.com/gzjjyz/logger"
)

var guildParty *GuildParty

func init() {
	guildParty = newGuildParty()
	guildParty.init()
}

type GuildParty struct{}

func newGuildParty() *GuildParty {
	return &GuildParty{}
}

func (g *GuildParty) init() {
	event.RegActorEventL(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		g.c2sPersonalInfo(player, nil)
		g.c2sGlobalInfo(player, nil)
	})

	event.RegActorEventL(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		g.c2sPersonalInfo(player, nil)
		g.c2sGlobalInfo(player, nil)
	})

	net.RegisterProto(29, 139, g.c2sSendGuildPartyTeachInvite)
	net.RegisterProto(29, 140, g.c2sRevGuildPartyTeachInvite)

	engine.RegisterActorCallFunc(playerfuncid.GuildPartyAddExp, g.addExp)
	engine.RegisterActorCallFunc(playerfuncid.GuildPartyEnter, g.onEnter)
	engine.RegisterActorCallFunc(playerfuncid.GuildPartyGatherFinish, g.onGatherFinish)

	engine.RegisterMessage(gshare.OfflineGuildPartyTeach, func() pb3.Message {
		return &pb3.CommonSt{}
	}, g.offlineGuildPartyTeach)
}

func (g *GuildParty) isInParty(player iface.IPlayer) bool {
	cfg, ok := jsondata.GetGuildPartyConf()
	if !ok {
		return false
	}

	if cfg.FubenId != player.GetFbId() {
		return false
	}

	return true
}

func (g *GuildParty) c2sSendGuildPartyTeachInvite(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_29_139
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	if !g.canParty(player) {
		return nil
	}

	cfg, ok := jsondata.GetGuildPartyConf()
	if !ok {
		return neterror.ConfNotFoundError("guild party conf is nil")
	}

	playerData := g.getPersonalData(player)
	if len(playerData.TeachNeiGongSender) >= int(cfg.TeachInviteTimes) {
		player.SendTipMsg(tipmsgid.TimesNotEnough)
		return nil
	}

	if playerData.TeachRewardActorId > 0 {
		player.SendTipMsg(tipmsgid.TimesNotEnough)
		return nil
	}

	if pie.Uint64s(playerData.TeachNeiGongSender).Contains(req.GetPlayerId()) {
		return neterror.ParamsInvalidError("guild party teach repeated")
	}

	target := manager.GetPlayerPtrById(req.GetPlayerId())
	if nil == target {
		player.SendTipMsg(tipmsgid.TpTargetOffline)
		return nil
	}

	if !g.isInParty(player) || !g.isInParty(target) {
		return neterror.ParamsInvalidError("not part guild party")
	}

	if player.GetId() == req.GetPlayerId() {
		return neterror.ParamsInvalidError("cant self")
	}

	if target.GetGuildId() != player.GetGuildId() {
		return neterror.ParamsInvalidError("not same guild")
	}

	playerData.TeachNeiGongSender = append(playerData.TeachNeiGongSender, req.GetPlayerId())

	targetData := g.getPersonalData(target)
	targetData.TeachNeiGongReceiver = append(targetData.TeachNeiGongReceiver, &pb3.Key64Value{
		Key: player.GetId(),
	})

	player.SendProto3(29, 139, &pb3.S2C_29_139{PlayerId: req.GetPlayerId()})
	target.SendProto3(29, 141, &pb3.S2C_29_141{PlayerId: player.GetId()})

	return nil
}

const (
	guildPartyTeachReceived = 1
)

func (g *GuildParty) c2sRevGuildPartyTeachInvite(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_29_140
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	if !g.canParty(player) {
		return nil
	}

	if !g.isInParty(player) {
		return neterror.ParamsInvalidError("not part guild party")
	}

	idx := -1
	playerData := g.getPersonalData(player)
	for i, v := range playerData.TeachNeiGongReceiver {
		if v.Key == req.GetPlayerId() {
			idx = i
		}
	}

	if idx == -1 {
		return neterror.ParamsInvalidError("not receive invite")
	}

	if playerData.TeachNeiGongReceiver[idx].Value > 0 {
		return neterror.ParamsInvalidError("receive invite repeated")
	}

	playerData.TeachNeiGongReceiver[idx].Value = guildPartyTeachReceived

	getExp := func(receiverId, senderId uint64) uint32 {
		receiverData, ok := manager.GetData(receiverId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			return 0
		}
		senderData, ok := manager.GetData(senderId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			return 0
		}
		diff := uint32(math.Abs(float64(senderData.GetLv()) - float64(receiverData.GetLv())))
		exp := jsondata.GetGuildPartyTeachExp(receiverData.GetLv(), diff)
		return exp
	}
	myAddExp := getExp(player.GetId(), req.GetPlayerId())
	globalData := g.getGlobalData(player)

	rsp := &pb3.S2C_29_140{PlayerId: req.GetPlayerId()}
	if playerData.TeachRewardActorId == 0 {
		playerData.TeachRewardActorId = req.GetPlayerId()
		globalData.TeachReciver[player.GetId()] = req.GetPlayerId()
		player.AddExp(int64(myAddExp), pb3.LogId_LogActGuildPartyTeachNeiGong, false)
		rsp.Exp = myAddExp
	}

	if globalData.TeachReciver[req.GetPlayerId()] == 0 {
		engine.SendPlayerMessage(req.GetPlayerId(), gshare.OfflineGuildPartyTeach, &pb3.CommonSt{
			U32Param:  getExp(req.GetPlayerId(), player.GetId()),
			U64Param:  player.GetId(),
			U32Param2: time_util.NowSec(),
		})
		globalData.TeachReciver[req.GetPlayerId()] = player.GetId()
	}

	player.SendProto3(29, 140, rsp)
	return nil
}

func (g *GuildParty) c2sGlobalInfo(player iface.IPlayer, _ *base.Message) error {
	if !g.canParty(player) {
		return nil
	}
	player.SendProto3(29, 130, &pb3.S2C_29_130{State: g.getGlobalData(player)})
	return nil
}

func (g *GuildParty) c2sPersonalInfo(player iface.IPlayer, _ *base.Message) error {
	if !g.canParty(player) {
		return nil
	}
	player.SendProto3(29, 131, &pb3.S2C_29_131{Player: g.getPersonalData(player)})
	return nil
}

// canParty 是否可以参加仙盟宴会
func (g *GuildParty) canParty(player iface.IPlayer) bool {
	status := GetActStatus(activitydef.ActGuildParty)

	return status == activitydef.ActStart && player.GetGuildId() > 0
}

func (g *GuildParty) levelUp(globalData *pb3.GuildParty) bool {
	cfg, ok := jsondata.GetGuildPartyConf() // 配表固定一条记录
	if !ok {
		return false
	}

	var level uint32
	for _, l := range cfg.Levels {
		if globalData.Count >= l.Times && l.Level > globalData.Level {
			level = l.Level
		}
	}

	if globalData.Level >= level {
		return false
	}

	globalData.Level = level
	return true
}

func (g *GuildParty) getPersonalData(player iface.IPlayer) *pb3.GuildPartyPlayer {
	if player.GetBinaryData().GetGuildData().PartyPlayer == nil {
		player.GetBinaryData().GetGuildData().PartyPlayer = &pb3.GuildPartyPlayer{
			LastJoinTime: time_util.NowSec(),
		}
	} else {
		lastJoinTime := player.GetBinaryData().GetGuildData().PartyPlayer.LastJoinTime
		if !time_util.IsSameDay(time_util.NowSec(), lastJoinTime) {
			// 跨天需要重置数据
			player.GetBinaryData().GetGuildData().PartyPlayer = nil
			return g.getPersonalData(player)
		}
	}

	if player.GetBinaryData().GetGuildData().PartyPlayer.ExpPerMinute == 0 {
		player.GetBinaryData().GetGuildData().PartyPlayer.ExpPerMinute = g.expPerMinute(player, g.finalExpAdded(player))
	}

	return player.GetBinaryData().GetGuildData().PartyPlayer
}

func (g *GuildParty) getGlobalData(player iface.IPlayer) *pb3.GuildParty {
	guild := guildmgr.GetGuildById(player.GetGuildId())
	if guild.GuildInfo.Party == nil {
		guild.GuildInfo.Party = &pb3.GuildParty{}
	}
	if nil == guild.GuildInfo.Party.TeachReciver {
		guild.GuildInfo.Party.TeachReciver = make(map[uint64]uint64)
	}

	return guild.GuildInfo.Party
}

func (g *GuildParty) addExp(player iface.IPlayer, _ []byte) {
	actorRate := uint32(player.GetFightAttr(attrdef.ExpAddRate))

	cfg, _ := jsondata.GetGuildPartyConf()
	var secondExp int64
	if cfg != nil {
		secondExp = int64(cfg.SecondExp)
	}
	baseExp := int64(player.GetLevel()/60) * secondExp

	globalData := g.getGlobalData(player)
	var levelRate uint32
	for _, v := range cfg.Levels {
		if v.Level == globalData.Level {
			levelRate = v.AddRate
			break
		}
	}

	finalExpAdded := player.AddExp(baseExp, pb3.LogId_LogActGuildPartyAddExp, true, actorRate, levelRate)
	personalData := g.getPersonalData(player)
	personalData.AddedExp += uint64(finalExpAdded)
	personalData.ExpPerMinute = g.expPerMinute(player, finalExpAdded)

	g.c2sPersonalInfo(player, nil)
}

func (g *GuildParty) onEnter(player iface.IPlayer, _ []byte) {
	if !g.canParty(player) {
		return
	}

	event.TriggerEvent(player, custom_id.AeCompleteRetrieval, sysdef.SiGuildParty, 1)

	g.c2sPersonalInfo(player, nil)
	g.c2sGlobalInfo(player, nil)
}

func (g *GuildParty) onGatherFinish(player iface.IPlayer, _ []byte) {
	if !g.canParty(player) {
		logger.LogError("onGatherFinish guild party is not start")
		return
	}

	personalData := g.getPersonalData(player)
	globalData := g.getGlobalData(player)
	if personalData == nil || globalData == nil {
		logger.LogError("onGatherFinish player not in guild")
		return
	}

	// 添柴次数固定允许参与一次
	if personalData.JoinCount >= 1 {
		logger.LogError("onGatherFinish add count limit")
		return
	}

	personalData.JoinCount++
	g.c2sPersonalInfo(player, nil)
	player.TriggerQuestEvent(custom_id.QttGuildPartyGatherFinish, 0, 1)

	globalData.Count++
	g.levelUp(globalData)

	guild := guildmgr.GetGuildById(player.GetGuildId())
	if guild != nil {
		guild.BroadcastProto(29, 130, &pb3.S2C_29_130{State: globalData})
	}
}

func (g *GuildParty) finalExpAdded(player iface.IPlayer) int64 {
	actorExpRate := uint32(player.GetFightAttr(attrdef.ExpAddRate))

	cfg, _ := jsondata.GetGuildPartyConf()
	var secondExp int64
	if cfg != nil {
		secondExp = int64(cfg.SecondExp)
	}
	baseExp := int64(player.GetLevel()/60) * secondExp

	globalData := g.getGlobalData(player)
	var guildPartyLevelRate uint32
	for _, v := range cfg.Levels {
		if v != nil && v.Level == globalData.Level {
			guildPartyLevelRate = v.AddRate
			break
		}
	}

	// 下面这里计算经验加成的逻辑,
	// 参考了 levelsys.go 的 CalcFinalExp
	var worldAddRate uint32
	worldLv := gshare.GetWorldLevel()
	if player.GetLevel() < worldLv {
		worldAddRate = jsondata.GetWorldAddRateByLv(worldLv - player.GetLevel())
	}

	exp := int64(float64(baseExp) * (1 + float64(worldAddRate)/10000))
	addRate := actorExpRate + guildPartyLevelRate + worldAddRate
	exp = int64(float64(exp) * (1 + float64(addRate)/10000))

	return exp
}

func (g *GuildParty) expPerMinute(player iface.IPlayer, perIntervalExpAdded int64) uint32 {
	cfg, _ := jsondata.GetGuildPartyConf()
	if cfg != nil {
		interval := int64(cfg.Interval)
		if interval == 0 {
			interval = 1
		}
		return uint32(int64(60/interval) * perIntervalExpAdded)
	}

	return 0
}

func (g *GuildParty) offlineGuildPartyTeach(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	playerData := g.getPersonalData(player)
	if playerData.TeachRewardActorId > 0 {
		return
	}
	actorId, addExp, sendTime := st.U64Param, st.U32Param, st.U32Param2
	if time_util.IsSameDay(time_util.NowSec(), sendTime) {
		playerData.TeachRewardActorId = actorId
		if g.canParty(player) {
			player.SendProto3(29, 140, &pb3.S2C_29_140{PlayerId: actorId, Exp: addExp})
		}
	}
	player.AddExp(int64(addExp), pb3.LogId_LogActGuildPartyTeachNeiGong, false)
	g.c2sPersonalInfo(player, nil)
}
