package logworker

import (
	"jjyz/base/cmd"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker/internal"
	"time"

	"github.com/gzjjyz/logger"
)

func Init() error {
	return internal.Init()
}

func Flush() {
	logger.LogInfo("wait log counter worker flush")
	internal.Flush()
	logger.LogInfo("log counter worker flush finish")
}

// LogItem 道具埋点
func LogItem(player iface.IPlayer, st *pb3.LogItem) {
	if nil == st {
		return
	}

	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	st.Timestamp = uint32(time.Now().Unix())

	if nil != player {
		st.ActorId = player.GetId()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
		st.Level = player.GetLevel()
		st.VipLevel = player.GetVipLevel()
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogItem, st)
}

// LogPlayerBehavior 玩家行为埋点
func LogPlayerBehavior(player iface.IPlayer, logId pb3.LogId, st *pb3.LogPlayerCounter) {
	if nil == st {
		return
	}

	st.LogId = uint32(logId)
	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	st.Timestamp = uint32(time.Now().Unix())
	if nil != player {
		st.ActorId = player.GetId()
		st.Level = player.GetLevel()
		st.VipLevel = player.GetVipLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogCounter, st)
}

func LogEconomy(player iface.IPlayer, st *pb3.LogEconomy) {
	if nil == st {
		return
	}

	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	st.Timestamp = uint32(time.Now().Unix())

	if nil != player {
		st.ActorId = player.GetId()
		st.Level = player.GetLevel()
		st.VipLevel = player.GetVipLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogEconomy, st)
}

func LogExpLv(player iface.IPlayer, logId pb3.LogId, st *pb3.LogExpLv) {
	if nil == st {
		return
	}

	st.LogId = uint32(logId)
	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	st.Timestamp = uint32(time.Now().Unix())

	if nil != player {
		st.ActorId = player.GetId()
		st.ToLevel = player.GetLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogExpLv, st)
}

func LogLogin(player iface.IPlayer, st *pb3.LogLogin) {
	if nil == st {
		return
	}

	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	if 0 == st.Timestamp {
		st.Timestamp = uint32(time.Now().Unix())
	}

	if nil != player {
		st.UserId = player.GetUserId()
		st.ActorId = player.GetId()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)

		if st.Type == common.LogLogout {
			st.CreateTime = player.GetLoginTime()
		} else {
			st.CreateTime = player.GetCreateTime()
		}
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogLogin, st)
}

func LogFightChange(player iface.IPlayer, st *pb3.LogFightValueChange) {
	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId
	st.Timestamp = uint32(time.Now().Unix())

	if nil != player {
		st.Level = player.GetLevel()
		st.VipLevel = player.GetVipLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogFight, st)
}

func LogDrop(player iface.IPlayer, st *pb3.LogBossDrop) {
	st.SrvId = gshare.GameConf.SrvId
	if player != nil {
		st.ActorId = player.GetId()
		st.ActorLevel = player.GetLevel()
		st.ActorFight = player.GetExtraAttr(attrdef.FightValue)
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogDrop, st)
}

func LogCompose(player iface.IPlayer, conf *jsondata.ComposeConf, st *pb3.LogCompose) {
	st.TargetId = conf.ItemId
	if st.TargetCount <= 0 {
		st.TargetCount = 1
	}
	st.ComposeId = conf.Id
	st.Type = conf.ComposeType
	st.Timestamp = uint32(time.Now().Unix())
	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId

	if nil != player {
		st.ActorId = player.GetId()
		st.ActorLevel = player.GetLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}

	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogCompose, st)
}

func LogComposeWithConsume(player iface.IPlayer, conf *jsondata.ComposeConf, consumes jsondata.ConsumeVec, rate uint32) {
	st := &pb3.LogCompose{
		TargetCount: rate,
		Items:       make(map[uint32]uint32),
		Moneys:      make(map[uint32]uint32),
	}

	for _, consume := range consumes {
		if consume.Type == custom_id.ConsumeTypeItem {
			st.Items[consume.Id] += consume.Count * rate
		} else {
			st.Moneys[consume.Id] += consume.Count * rate
		}
	}

	LogCompose(player, conf, st)
}

func LogActivity(player iface.IPlayer, st *pb3.LogActivity) {
	if nil == st {
		return
	}
	st.Timestamp = uint32(time.Now().Unix())
	if nil != player {
		st.ActorId = player.GetId()
		st.Level = player.GetLevel()
		st.DitchId = player.GetExtraAttrU32(attrdef.DitchId)
		st.SubDitchId = player.GetExtraAttrU32(attrdef.SubDitchId)
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogActivity, st)
}

func LogOnline(st *pb3.LogOnline) {
	if nil == st {
		return
	}

	st.PfId = gshare.GameConf.PfId
	st.SrvId = gshare.GameConf.SrvId

	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogOnline, st)
}

func LogTokenOrder(player iface.IPlayer, st *pb3.LogTokenOrder) {
	if nil == st {
		return
	}

	st.Pid = gshare.GameConf.PfId
	st.ServerId = gshare.GameConf.SrvId
	if 0 == st.Timestamp {
		st.Timestamp = uint32(time.Now().Unix())
	}

	if nil != player {
		st.UserId = player.GetUserId()
		st.ActorId = player.GetId()
		st.Cid = player.GetExtraAttrU32(attrdef.DitchId)
		st.ActorName = player.GetName()
		st.ActorLevel = uint64(player.GetLevel())
		st.ActorCCid = player.GetExtraAttrU32(attrdef.SubDitchId)
		st.ActorCreateTime = player.GetCreateTime()
		st.OldServerId = player.GetServerId()
	}

	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogTokenOrder, st)
}

func LogNetProtoStat(cmdId, size, pfId, srvId uint32) {
	if !gshare.GetStaticVar().OpenProtoStat {
		return
	}
	if size == 0 {
		return
	}
	protoH, protoL := cmdId>>8, cmdId&0xFF
	if protoH == 0 && protoL == 0 {
		return
	}
	var st = &pb3.LogNetProto{
		H:         protoH,
		L:         protoL,
		Size:      size,
		PfId:      pfId,
		SrvId:     srvId,
		Timestamp: uint32(time.Now().Unix()),
	}
	internal.PostLogDataWithSuffix(gshare.GameConf.PfId, cmd.LogNetProto, st)
}

func LogChat(st *pb3.LogChat) {
	if nil == st {
		return
	}

	internal.PostLogData(cmd.GLChat, st)
}
