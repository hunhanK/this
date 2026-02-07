/**
 * @Author: lzp
 * @Date: 2023/11/29
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

type GuildSecretSys struct {
	Base
}

func guildSecretAddExp(player iface.IPlayer, buf []byte) {
	var msg pb3.CommonSt
	if err := pb3.Unmarshal(buf, &msg); nil != err {
		player.LogError("addExp unmarshal err:%v", err)
		return
	}

	awards := jsondata.GetGuildSecretExpAwards(player.GetLevel())
	if len(awards) == 0 {
		return
	}
	ok := engine.GiveRewards(player, awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogActGuildSecretExpRewards,
		NoTips: true,
	})
	if !ok {
		player.LogError("give rewards failed")
		return
	}

}

func getGuildSecretGuildLeader(player iface.IPlayer, buf []byte) {
	var msg pb3.CommonSt
	if err := pb3.Unmarshal(buf, &msg); nil != err {
		player.LogError("addExp unmarshal err:%v", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiGuild).(*GuildSys)
	if !ok {
		return
	}

	guild := sys.GetGuild()
	if guild == nil {
		return
	}

	leaderId := guild.GetLeader().PlayerInfo.GetId()
	leaderName := guild.GetLeader().PlayerInfo.GetName()

	err := sys.GetOwner().CallActorFunc(actorfuncid.GuildSecretLeaderDataRet, &pb3.CommonSt{
		U64Param: leaderId,
		StrParam: leaderName,
	})

	if err != nil {
		player.LogError("getGuildSecretGuildLeader err: %v", err)
		return
	}

}

func init() {
	engine.RegisterActorCallFunc(playerfuncid.GuildSecretAddExp, guildSecretAddExp)
	engine.RegisterActorCallFunc(playerfuncid.GuildSecretLeaderDataReq, getGuildSecretGuildLeader)
	engine.RegisterSysCall(sysfuncid.F2GGuildSecretLeaderIdRet, func(buf []byte) {
		var req pb3.GuildSecretSettlementRet
		if err := pb3.Unmarshal(buf, &req); nil != err {
			logger.LogError("pb3.Unmarshal err:%v", err)
			return
		}
		event.TriggerSysEvent(custom_id.SeGuildSecretOver, req.GuildId)
	})
}
