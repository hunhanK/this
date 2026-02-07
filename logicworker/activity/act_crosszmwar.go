/**
 * @Author: zjj
 * @Date: 2023/12/6
 * @Desc:
**/

package activity

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/net"
)

func init() {
	net.RegisterProto(54, 8, c2sGetHistoryActCrossZmWarSettlement)
	event.RegSysEvent(custom_id.SeGuildSecretOver, handleSeGuildSecretOver)
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		syncGuildSecretLeaderId()
	})
	engine.RegisterSysCall(sysfuncid.SmallCrossSyncCrossZmWarSettlement, handleSmallCrossSyncCrossZmWarSettlement)
	gmevent.Register("handleSeGuildSecretOver", func(player iface.IPlayer, args ...string) bool {
		handleSeGuildSecretOver(player.GetGuildId())
		return true
	}, 1)
}

func c2sGetHistoryActCrossZmWarSettlement(player iface.IPlayer, msg *base.Message) error {
	globalVar := gshare.GetStaticVar()
	if globalVar == nil {
		return nil
	}
	player.SendProto3(54, 8, &pb3.S2C_54_8{
		HistoryActCrossZmWarRankInfos: globalVar.HistoryActCrossZmWarRankInfos,
	})
	return nil
}

func handleSmallCrossSyncCrossZmWarSettlement(buf []byte) {
	var req pb3.CrossZmWarSettlementList
	if err := pb3.Unmarshal(buf, &req); nil != err {
		logger.LogError("pb3.Unmarshal err:%v", err)
		return
	}
	var curRankInfo *pb3.ActCrossZmWarRankInfo
	for _, warSettlement := range req.List {
		if warSettlement.PfId != engine.GetPfId() || warSettlement.SvrId != engine.GetServerId() {
			continue
		}
		curRankInfo = &pb3.ActCrossZmWarRankInfo{
			GuildId:      warSettlement.GuildId,
			LeaderId:     warSettlement.LeaderId,
			Rank:         warSettlement.Rank,
			SettlementAt: warSettlement.SettlementAt,
		}
		break
	}
	if curRankInfo == nil {
		return
	}
	globalVar := gshare.GetStaticVar()
	if globalVar == nil {
		return
	}
	globalVar.HistoryActCrossZmWarRankInfos = append(globalVar.HistoryActCrossZmWarRankInfos, curRankInfo)
}

func syncGuildSecretLeaderId() {
	if !engine.FightClientExistPredicate(base.SmallCrossServer) {
		logger.LogWarn("syncGuildSecretLeaderId small cross srv not exist")
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncGuildSecretLeaderId, &pb3.CommonSt{
		U32Param:  gshare.GameConf.PfId,
		U32Param2: gshare.GameConf.SrvId,
		U64Param:  gshare.GetGuildSecretLeaderId(),
	})
	if err != nil {
		logger.LogError("err:%v", err)
	}
	logger.LogInfo("syncGuildSecretLeaderId %d", gshare.GetGuildSecretLeaderId())

}

func handleSeGuildSecretOver(args ...interface{}) {
	if len(args) < 1 {
		return
	}

	leaderGuildId := args[0].(uint64)

	guildInfo := guildmgr.GetGuildById(leaderGuildId)
	if guildInfo == nil {
		logger.LogError("guildInfo is nil")
		return
	}

	guildLeaderId := guildInfo.BasicInfo.LeaderId
	globalVar := gshare.GetStaticVar()
	if globalVar == nil {
		return
	}
	globalVar.GuildSecretLeaderId = guildLeaderId
	syncGuildSecretLeaderId()
}
